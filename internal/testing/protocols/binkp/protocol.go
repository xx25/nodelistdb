package binkp

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

// BinkP frame types
const (
	M_NUL  = 0x00 // Information frame (M_NUL "KEY value")
	M_ADR  = 0x01 // Address announcement
	M_PWD  = 0x02 // Password
	M_FILE = 0x03 // File information
	M_OK   = 0x04 // Handshake successful
	M_EOB  = 0x05 // End of batch
	M_GOT  = 0x06 // File received
	M_ERR  = 0x07 // Error
	M_BSY  = 0x08 // Busy
	M_GET  = 0x09 // Request file
	M_SKIP = 0x0A // Skip file
)

// Frame type names for debugging
var frameTypeNames = map[uint8]string{
	M_NUL:  "M_NUL",
	M_ADR:  "M_ADR",
	M_PWD:  "M_PWD",
	M_FILE: "M_FILE",
	M_OK:   "M_OK",
	M_EOB:  "M_EOB",
	M_GOT:  "M_GOT",
	M_ERR:  "M_ERR",
	M_BSY:  "M_BSY",
	M_GET:  "M_GET",
	M_SKIP: "M_SKIP",
}

// Frame represents a BinkP protocol frame
type Frame struct {
	Type    uint8  // Frame type (M_NUL, M_ADR, etc.)
	Command bool   // True if command frame (bit 7 set)
	Data    []byte // Frame data
}

// String returns a human-readable representation of the frame
func (f *Frame) String() string {
	typeName := frameTypeNames[f.Type]
	if typeName == "" {
		typeName = fmt.Sprintf("0x%02X", f.Type)
	}
	
	if f.Command {
		typeName = "CMD:" + typeName
	}
	
	if f.Type == M_NUL && len(f.Data) > 0 {
		// Parse M_NUL data as "KEY value"
		dataStr := string(f.Data)
		return fmt.Sprintf("%s %s", typeName, dataStr)
	}
	
	return fmt.Sprintf("%s [%d bytes]", typeName, len(f.Data))
}

// ReadFrame reads a single BinkP frame from the connection
func ReadFrame(conn net.Conn) (*Frame, error) {
	// Set read timeout
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	
	// Read 2-byte header (network byte order)
	header := make([]byte, 2)
	n, err := io.ReadFull(conn, header)
	if err != nil {
		if err == io.EOF {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("failed to read header: %w", err)
	}
	if n != 2 {
		return nil, fmt.Errorf("short header read: %d bytes", n)
	}
	
	// Parse header (network/big-endian byte order)
	headerValue := binary.BigEndian.Uint16(header)
	isCommand := (headerValue & 0x8000) != 0
	dataLen := int(headerValue & 0x7FFF)
	
	// Read data
	var data []byte
	var frameType byte
	
	if dataLen > 0 {
		data = make([]byte, dataLen)
		n, err = io.ReadFull(conn, data)
		if err != nil {
			return nil, fmt.Errorf("failed to read data: %w", err)
		}
		if n != dataLen {
			return nil, fmt.Errorf("short data read: %d bytes, expected %d", n, dataLen)
		}
		
		// For command frames, first byte is the command type
		if isCommand && dataLen > 0 {
			frameType = data[0]
			data = data[1:] // Rest is command arguments
		}
	}
	
	return &Frame{
		Type:    frameType,
		Command: isCommand,
		Data:    data,
	}, nil
}

// WriteFrame writes a BinkP frame to the connection
func WriteFrame(conn net.Conn, frame *Frame) error {
	// Debug: log what we're sending
	if debug := os.Getenv("DEBUG_BINKP"); debug != "" {
		log.Printf("BinkP: Writing frame type=0x%02X command=%v len=%d", frame.Type, frame.Command, len(frame.Data))
	}
	
	// Set write timeout
	conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	
	dataLen := len(frame.Data)
	
	// BinkP header format (2 bytes, network byte order):
	// Bit 15: Command flag (1=command, 0=data)
	// Bits 14-0: Data length (up to 32767 bytes)
	// For command frames, data includes 1 byte command type + arguments
	
	var fullData []byte
	if frame.Command {
		// For command frames, prepend the command type to data
		fullData = make([]byte, 1+dataLen)
		fullData[0] = frame.Type
		copy(fullData[1:], frame.Data)
		dataLen = len(fullData)
	} else {
		fullData = frame.Data
	}
	
	if dataLen > 0x7FFF {
		return fmt.Errorf("data too large: %d bytes (max 32767)", dataLen)
	}
	
	// Create header with length and command flag (network/big-endian byte order)
	header := make([]byte, 2)
	headerValue := uint16(dataLen)
	if frame.Command {
		headerValue |= 0x8000 // Set command flag
	}
	binary.BigEndian.PutUint16(header, headerValue)
	
	// Write header
	if _, err := conn.Write(header); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}
	
	// Write data
	if dataLen > 0 {
		if _, err := conn.Write(fullData); err != nil {
			return fmt.Errorf("failed to write data: %w", err)
		}
	}
	
	return nil
}

// ParseM_NUL parses M_NUL frame data into key and value
// Format: "KEY value" or just "value" for some fields
func ParseM_NUL(data []byte) (key, value string) {
	dataStr := string(data)
	
	// Trim leading/trailing null bytes and spaces
	dataStr = strings.Trim(dataStr, "\x00 ")
	
	// Find first space
	spaceIdx := strings.Index(dataStr, " ")
	if spaceIdx == -1 {
		// No space, entire string is the value (some implementations do this)
		return "", dataStr
	}
	
	key = dataStr[:spaceIdx]
	value = dataStr[spaceIdx+1:]
	
	return key, value
}

// CreateM_NUL creates an M_NUL frame with the given key and value
func CreateM_NUL(key, value string) *Frame {
	data := fmt.Sprintf("%s %s", key, value)
	return &Frame{
		Type:    M_NUL,
		Command: true,
		Data:    []byte(data),
	}
}

// CreateM_ADR creates an M_ADR frame with the given addresses
func CreateM_ADR(addresses ...string) *Frame {
	data := strings.Join(addresses, " ")
	return &Frame{
		Type:    M_ADR,
		Command: true,
		Data:    []byte(data),
	}
}

// CreateM_PWD creates an M_PWD frame with the given password
func CreateM_PWD(password string) *Frame {
	return &Frame{
		Type:    M_PWD,
		Command: true,
		Data:    []byte(password),
	}
}

// CreateM_OK creates an M_OK frame
func CreateM_OK() *Frame {
	return &Frame{
		Type:    M_OK,
		Command: true,
		Data:    nil,
	}
}

// CreateM_ERR creates an M_ERR frame with error message
func CreateM_ERR(message string) *Frame {
	return &Frame{
		Type:    M_ERR,
		Command: true,
		Data:    []byte(message),
	}
}

// ParseAddresses parses address list from M_ADR frame
// Format: "2:5001/100@fidonet 2:5001/100.1@fidonet"
func ParseAddresses(data []byte) []string {
	addrStr := string(data)
	addrStr = strings.TrimSpace(addrStr)
	
	if addrStr == "" {
		return []string{}
	}
	
	// Split by space
	return strings.Fields(addrStr)
}