package emsi

import (
	"fmt"
	"strings"
	"time"
)

// EMSI protocol constants
const (
	EMSI_INQ = "**EMSI_INQC816"
	EMSI_REQ = "**EMSI_REQA77E"
	EMSI_ACK = "**EMSI_ACKA490"
	EMSI_NAK = "**EMSI_NAKEEC3"
	EMSI_CLI = "**EMSI_CLIFA8C"
	EMSI_HBT = "**EMSI_HBTEAEE"
	EMSI_DAT = "**EMSI_DAT"
)

// EMSIData represents the EMSI handshake data
type EMSIData struct {
	// System information
	SystemName   string
	Location     string
	Sysop        string
	Phone        string
	Speed        string
	Flags        string
	
	// Mailer information
	MailerName   string
	MailerVersion string
	MailerSerial string
	
	// Addresses
	Addresses    []string
	
	// Capabilities
	Protocols    []string // ZMO, ZAP, DZA, JAN, HYD
	Capabilities []string // Additional capabilities
	
	// Password (for authentication)
	Password     string
}

// CreateEMSI_DAT creates an EMSI_DAT packet
func CreateEMSI_DAT(data *EMSIData) string {
	var packet strings.Builder
	
	// Start with EMSI_DAT header (length will be filled later)
	packet.WriteString("**EMSI_DAT0000")
	
	// Build the data section
	var dataPart strings.Builder
	
	// {EMSI} identifier
	dataPart.WriteString("{EMSI}")
	
	// {addresses}
	dataPart.WriteString("{")
	dataPart.WriteString(strings.Join(data.Addresses, " "))
	dataPart.WriteString("}")
	
	// {password}
	dataPart.WriteString("{")
	if data.Password != "" {
		dataPart.WriteString(data.Password)
	}
	dataPart.WriteString("}")
	
	// {link_codes} - link capabilities
	dataPart.WriteString("{8N1,PUA}")  // 8N1, Pick Up All
	
	// {compatibility_codes} - protocol capabilities
	dataPart.WriteString("{")
	if len(data.Protocols) > 0 {
		dataPart.WriteString(strings.Join(data.Protocols, ","))
	} else {
		dataPart.WriteString("NCP")  // No Compatible Protocols
	}
	dataPart.WriteString("}")
	
	// {mailer_code}{mailer_name}{mailer_version}{mailer_serial}
	dataPart.WriteString("{")
	dataPart.WriteString(fmt.Sprintf("PID,%s,%s,%s",
		data.MailerName,
		data.MailerVersion,
		data.MailerSerial))
	dataPart.WriteString("}")
	
	// {extra_data} - OHFR, TRAF, etc.
	dataPart.WriteString("{")
	
	// System name
	if data.SystemName != "" {
		dataPart.WriteString(fmt.Sprintf("SNAME:%s,", data.SystemName))
	}
	
	// Location
	if data.Location != "" {
		dataPart.WriteString(fmt.Sprintf("LOC:%s,", data.Location))
	}
	
	// Sysop
	if data.Sysop != "" {
		dataPart.WriteString(fmt.Sprintf("SYSOP:%s,", data.Sysop))
	}
	
	// Phone
	if data.Phone != "" {
		dataPart.WriteString(fmt.Sprintf("PHONE:%s,", data.Phone))
	}
	
	// Flags
	if data.Flags != "" {
		dataPart.WriteString(fmt.Sprintf("FLAGS:%s,", data.Flags))
	}
	
	// Time
	dataPart.WriteString(fmt.Sprintf("TIME:%s,", time.Now().Format("20060102T150405")))
	
	dataPart.WriteString("[NodelistDB Test Daemon]")
	dataPart.WriteString("}")
	
	// Get the data part
	dataStr := dataPart.String()
	
	// Calculate CRC16 of the data part
	crc := CalculateCRC16([]byte(dataStr))
	
	// Build the final packet
	finalPacket := fmt.Sprintf("**EMSI_DAT%04X%s%04X",
		len(dataStr),  // Length in hex
		dataStr,       // Data part
		crc)          // CRC16 in hex
	
	return finalPacket
}

// ParseEMSI_DAT parses an EMSI_DAT packet
func ParseEMSI_DAT(packet string) (*EMSIData, error) {
	// Remove the header if present
	if strings.HasPrefix(packet, "**EMSI_DAT") {
		packet = packet[14:] // Skip **EMSI_DATxxxx
	}
	
	// Find the data between braces
	data := &EMSIData{
		Addresses:    []string{},
		Protocols:    []string{},
		Capabilities: []string{},
	}
	
	// Simple parser - look for address patterns
	parts := strings.Split(packet, "}")
	for i, part := range parts {
		part = strings.TrimPrefix(part, "{")
		
		switch i {
		case 0: // EMSI identifier
			// Skip
		case 1: // Addresses
			addrs := strings.Fields(part)
			for _, addr := range addrs {
				if strings.Contains(addr, ":") && strings.Contains(addr, "/") {
					data.Addresses = append(data.Addresses, addr)
				}
			}
		case 2: // Password
			data.Password = part
		case 3: // Link codes
			// Parse link codes if needed
		case 4: // Compatibility codes (protocols)
			protocols := strings.Split(part, ",")
			for _, proto := range protocols {
				proto = strings.TrimSpace(proto)
				if proto != "" && proto != "NCP" {
					data.Protocols = append(data.Protocols, proto)
				}
			}
		case 5: // Mailer info
			// Parse PID,name,version,serial
			if strings.HasPrefix(part, "PID,") {
				mailerParts := strings.Split(part[4:], ",")
				if len(mailerParts) > 0 {
					data.MailerName = mailerParts[0]
				}
				if len(mailerParts) > 1 {
					data.MailerVersion = mailerParts[1]
				}
			}
		case 6: // Extra data
			// Parse SNAME, LOC, SYSOP, etc.
			extras := strings.Split(part, ",")
			for _, extra := range extras {
				if strings.HasPrefix(extra, "SNAME:") {
					data.SystemName = strings.TrimPrefix(extra, "SNAME:")
				} else if strings.HasPrefix(extra, "LOC:") {
					data.Location = strings.TrimPrefix(extra, "LOC:")
				} else if strings.HasPrefix(extra, "SYSOP:") {
					data.Sysop = strings.TrimPrefix(extra, "SYSOP:")
				} else if strings.HasPrefix(extra, "PHONE:") {
					data.Phone = strings.TrimPrefix(extra, "PHONE:")
				} else if strings.HasPrefix(extra, "FLAGS:") {
					data.Flags = strings.TrimPrefix(extra, "FLAGS:")
				}
			}
		}
	}
	
	return data, nil
}

// CalculateCRC16 calculates CRC16-CCITT for EMSI
func CalculateCRC16(data []byte) uint16 {
	var crc uint16 = 0
	
	for _, b := range data {
		crc ^= uint16(b) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	
	return crc
}