package protocols

import (
	"errors"
	"fmt"
	"io"
)

// Virtual Modem Protocol (VMP) framing and command vocabulary.
//
// VMP is the binary protocol implemented by Ray Gwinn's OS/2 VMODEM.EXE, the
// software the IVM ("Internet VMODEM") nodelist flag actually announces. The
// wire details below were recovered from VMODEM.EXE (a 16-bit code object
// inside an OS/2 LX image) and confirmed against live nodes.
//
// Frame format (send routine at 0x2d60):
//
//	10 02            frame marker (DLE STX), literal, never stuffed
//	<len:16 BE>      payload length, DLE-stuffed
//	<payload>        `len` bytes, DLE-stuffed; starts with a 16-bit BE command
//
// Any 0x10 (DLE) byte inside the length or payload is escaped by doubling.
// Example disconnect frame (command 1, reason 8): 10 02 00 04 00 01 00 08.
//
// A VMP call uses TWO TCP connections. The caller opens the control channel to
// the answerer's IVM port and names its own listening port in the connect
// frame; the answerer then dials a second, raw-binary DATA connection back to
// the caller (VMODEM.EXE at 0x398e does socket()+connect() to the control
// channel's peer address). Only once that reverse connection is up does the
// answerer ring its mailer. The control channel carries nothing but frames —
// VMODEM's parser discards any other byte — and the data channel carries
// unescaped binary, which is what the manual means by "VMP is true binary".
const (
	vmpDLE = 0x10
	vmpSTX = 0x02
)

// VMP command words, from the dispatch table at 0x2ce0 (4 states x 8 commands;
// the dispatcher at 0x33cd clamps the command to 7, so anything >= 8 is NAKed).
const (
	vmpCmdConnect    = 0 // caller -> answerer: connect request; carries the caller's data-channel port
	vmpCmdDisconnect = 1 // either direction: hang up; carries a reason word
	vmpCmdRinging    = 2 // answerer -> caller: the remote is ringing its mailer
	vmpCmdConnected  = 3 // answerer -> caller: the mailer answered; the data channel is live
	vmpCmdNAK        = 4 // either direction: unrecognized command, echoing the offender
	vmpCmdBusy       = 5 // answerer -> caller: no free virtual COM port
	vmpCmdHello      = 6 // caller -> answerer: opening frame; protocol level + requested port
)

// vmpMaxAnswererCommand bounds what an answering VMODEM can put on the wire.
// Commands 0 and 6 only ever travel caller->answerer, so a reply frame carrying
// one of those is not VMODEM. Keeping this tight avoids classifying arbitrary
// DLE-STX-looking binary as a VMP responder.
const vmpMaxAnswererCommand = vmpCmdBusy

// vmpMaxPayload is the largest payload VMODEM's own parser will accept: it
// rejects any frame declaring more (`cmp word [0x2766], 0x100; ja reject`), so
// a longer one did not come from a VMODEM.
const vmpMaxPayload = 0x100

// Disconnect reason codes VMODEM.EXE emits (call sites of the sender at 0x30a6
// and 0x239c).
const (
	vmpReasonRingAborted   = 1 // the ring loop gave up on the local COM port
	vmpReasonLocalHangup   = 2 // local side dropped DTR — the normal hangup
	vmpReasonBadState      = 3 // command not valid in the current state
	vmpReasonNoDataChannel = 4 // the reverse data connection back to the caller failed
	vmpReasonAborted       = 5 // local abort
	vmpReasonShortConnect  = 6 // connect frame payload shorter than 4 bytes
	vmpReasonLoopback      = 7 // caller is this same machine
	vmpReasonNotVModem     = 8 // opening frame was not a well-formed VMP hello
)

// vmpProtocolLevel is the caller's protocol level, carried in the hello frame
// as a LITTLE-endian word (VMODEM writes it without the byte swap it applies to
// every other field, and the validator at 0x240b reads it back the same way and
// requires >= 1).
const vmpProtocolLevel = 1

// vmpAnyPort asks the answerer to assign whichever virtual COM port is free,
// rather than naming one. The validator passes this word to the port assigner,
// which treats -1 as "scan for any available port".
const vmpAnyPort = 0xFFFF

// vmpDisconnectReasonText renders a disconnect reason for humans.
func vmpDisconnectReasonText(reason int) string {
	switch reason {
	case vmpReasonRingAborted:
		return "remote stopped ringing its mailer"
	case vmpReasonLocalHangup:
		return "remote hung up"
	case vmpReasonBadState:
		return "remote rejected the command for its current state"
	case vmpReasonNoDataChannel:
		return "remote could not open the data channel back to us"
	case vmpReasonAborted:
		return "remote aborted the call"
	case vmpReasonShortConnect:
		return "remote rejected the connect frame as too short"
	case vmpReasonLoopback:
		return "remote refused a call from its own address"
	case vmpReasonNotVModem:
		return "remote rejected us as not a current VMODEM"
	default:
		return fmt.Sprintf("reason %d", reason)
	}
}

// vmpFrame is a decoded control frame.
type vmpFrame struct {
	Command int
	Payload []byte // payload after the 2-byte command word
}

// Arg returns the i-th 16-bit big-endian word after the command word, or 0.
func (f vmpFrame) Arg(i int) int {
	off := i * 2
	if off+1 >= len(f.Payload) {
		return 0
	}
	return int(f.Payload[off])<<8 | int(f.Payload[off+1])
}

// vmpEncodeFrame builds a complete frame around payload: the literal DLE STX
// marker, then the DLE-stuffed big-endian length and payload.
func vmpEncodeFrame(payload []byte) []byte {
	out := make([]byte, 0, 2*len(payload)+8)
	out = append(out, vmpDLE, vmpSTX)
	stuff := func(b byte) {
		if b == vmpDLE {
			out = append(out, vmpDLE)
		}
		out = append(out, b)
	}
	stuff(byte(len(payload) >> 8))
	stuff(byte(len(payload)))
	for _, b := range payload {
		stuff(b)
	}
	return out
}

// vmpCommandFrame builds a frame carrying a command word followed by args, each
// a 16-bit big-endian word.
func vmpCommandFrame(command int, args ...int) []byte {
	payload := make([]byte, 0, 2+2*len(args))
	payload = append(payload, byte(command>>8), byte(command))
	for _, a := range args {
		payload = append(payload, byte(a>>8), byte(a))
	}
	return vmpEncodeFrame(payload)
}

// vmpHelloFrame builds the caller's opening frame. The answerer's validator at
// 0x240b reads exactly 10 bytes and requires: the DLE STX marker, a big-endian
// command word of 6, a little-endian protocol level >= 1, and a port word it
// passes to its port assigner (0xFFFF = any).
//
// The protocol-level word is the one field VMODEM does not byte-swap, so it is
// written little-endian here on purpose.
func vmpHelloFrame(requestedPort int) []byte {
	payload := []byte{
		byte(vmpCmdHello >> 8), byte(vmpCmdHello),
		byte(vmpProtocolLevel), byte(vmpProtocolLevel >> 8), // little-endian, deliberately
		byte(requestedPort >> 8), byte(requestedPort),
	}
	return vmpEncodeFrame(payload)
}

// vmpConnectFrame builds the caller's connect request. dataPort is the TCP port
// the caller is listening on for the reverse data channel, in network byte
// order like every other word. The trailing dword is VMODEM's version stamp; it
// is only inspected for payloads of 24 bytes or more (handler at 0x3252), so a
// short frame like this one skips the check entirely.
func vmpConnectFrame(dataPort int) []byte {
	payload := []byte{
		byte(vmpCmdConnect >> 8), byte(vmpCmdConnect),
		byte(dataPort >> 8), byte(dataPort),
		0, 0, 0, 0,
	}
	return vmpEncodeFrame(payload)
}

// errVMPNotAFrame reports that the stream carried bytes that are not VMP framing.
var errVMPNotAFrame = errors.New("vmp: not a frame")

// vmpFrameReader decodes control frames from a stream, mirroring VMODEM's own
// parser: bytes outside a frame are discarded, and a frame body carries DLE only
// as a doubled pair, so an unpaired DLE resynchronizes rather than corrupting.
type vmpFrameReader struct {
	r    io.Reader
	buf  []byte
	pos  int
	back []byte // bytes handed back for resynchronization
	junk int    // count of bytes discarded outside frames, for diagnostics
}

func newVMPFrameReader(r io.Reader) *vmpFrameReader {
	return &vmpFrameReader{r: r}
}

// nextByte returns the next raw byte, refilling from the underlying reader.
func (fr *vmpFrameReader) nextByte() (byte, error) {
	if len(fr.back) > 0 {
		b := fr.back[0]
		fr.back = fr.back[1:]
		return b, nil
	}
	for fr.pos >= len(fr.buf) {
		tmp := make([]byte, 512)
		n, err := fr.r.Read(tmp)
		if n > 0 {
			fr.buf = tmp[:n]
			fr.pos = 0
			break
		}
		if err != nil {
			return 0, err
		}
	}
	b := fr.buf[fr.pos]
	fr.pos++
	return b, nil
}

// pushBack returns bytes to the head of the stream so the resynchronizing hunt
// can reconsider them. Without this, abandoning a malformed frame on an
// unpaired DLE would swallow the very marker that starts the next good one.
func (fr *vmpFrameReader) pushBack(b ...byte) {
	fr.back = append(append([]byte{}, b...), fr.back...)
}

// ReadFrame returns the next complete control frame, skipping any non-frame
// bytes ahead of it.
func (fr *vmpFrameReader) ReadFrame() (vmpFrame, error) {
	for {
		frame, err := fr.readOne()
		if errors.Is(err, errVMPNotAFrame) {
			continue // resynchronize on the next marker
		}
		return frame, err
	}
}

func (fr *vmpFrameReader) readOne() (vmpFrame, error) {
	// Hunt for the DLE STX marker.
	for {
		b, err := fr.nextByte()
		if err != nil {
			return vmpFrame{}, err
		}
		if b != vmpDLE {
			fr.junk++
			continue
		}
		b, err = fr.nextByte()
		if err != nil {
			return vmpFrame{}, err
		}
		if b == vmpSTX {
			break
		}
		fr.junk += 2
	}

	// Inside a frame every DLE is doubled, so an unpaired one is a desync.
	unstuffed := func() (byte, error) {
		b, err := fr.nextByte()
		if err != nil {
			return 0, err
		}
		if b != vmpDLE {
			return b, nil
		}
		b2, err := fr.nextByte()
		if err != nil {
			return 0, err
		}
		if b2 != vmpDLE {
			// Not a stuffed DLE, so this frame is malformed. Both bytes may
			// belong to the next frame's marker — hand them back rather than
			// eating it.
			fr.pushBack(b, b2)
			return 0, errVMPNotAFrame
		}
		return vmpDLE, nil
	}

	hi, err := unstuffed()
	if err != nil {
		return vmpFrame{}, err
	}
	lo, err := unstuffed()
	if err != nil {
		return vmpFrame{}, err
	}
	length := int(hi)<<8 | int(lo)
	if length < 2 || length > vmpMaxPayload {
		return vmpFrame{}, errVMPNotAFrame
	}

	body := make([]byte, 0, length)
	for len(body) < length {
		b, err := unstuffed()
		if err != nil {
			return vmpFrame{}, err
		}
		body = append(body, b)
	}
	return vmpFrame{
		Command: int(body[0])<<8 | int(body[1]),
		Payload: body[2:],
	}, nil
}

// unstuffDLE collapses doubled DLE (0x10 0x10 -> 0x10). A trailing lone DLE
// (an escape whose second half hasn't arrived yet) is dropped.
func unstuffDLE(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); i++ {
		if b[i] == vmpDLE {
			if i+1 < len(b) && b[i+1] == vmpDLE {
				out = append(out, vmpDLE)
				i++
				continue
			}
			if i+1 >= len(b) {
				break // incomplete escape at end
			}
			// A lone DLE not followed by DLE shouldn't occur inside a valid
			// frame body; keep it so a malformed frame fails validation below.
			out = append(out, vmpDLE)
			continue
		}
		out = append(out, b[i])
	}
	return out
}

// looksLikeVMP reports whether b begins with a well-formed VMP frame from an
// answering VMODEM and, if so, the command word carried in its payload. It
// tolerates a truncated read as long as the marker, length and command word are
// present.
func looksLikeVMP(b []byte) (ok bool, command int) {
	if len(b) < 2 || b[0] != vmpDLE || b[1] != vmpSTX {
		return false, 0
	}
	rest := unstuffDLE(b[2:])
	if len(rest) < 4 {
		return false, 0 // need length word + command word
	}
	length := int(rest[0])<<8 | int(rest[1])
	command = int(rest[2])<<8 | int(rest[3])
	if length < 2 || length > vmpMaxPayload {
		return false, 0
	}
	if command < vmpCmdDisconnect || command > vmpMaxAnswererCommand {
		return false, 0
	}
	return true, command
}
