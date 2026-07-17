package protocols

import (
	"bytes"
	"net"
)

// Telnet command/option bytes (RFC 854 / RFC 856) used by the IVM
// telnet-binary transport that some FidoNet "vmodem" endpoints run in front of
// an EMSI mailer session.
const (
	tnIAC  = 255
	tnDONT = 254
	tnDO   = 253
	tnWONT = 252
	tnWILL = 251
	tnSB   = 250
	tnSE   = 240

	tnOptBinary = 0
	tnOptSGA    = 3
)

// telnetBinaryConn wraps a net.Conn and transparently handles the telnet
// option layer. On Read it strips IAC command/subnegotiation sequences and
// auto-negotiates binary mode (accepts BINARY and SGA, refuses everything
// else); on Write it doubles any 0xFF byte as telnet binary mode requires.
// Deadlines and addresses delegate to the embedded connection.
//
// It is a stream filter, not a one-shot: an IAC sequence may straddle a Read
// boundary, so an incomplete tail is carried in `pending` between calls.
type telnetBinaryConn struct {
	net.Conn
	pending []byte // incomplete IAC sequence carried from a prior Read
	outbuf  []byte // decoded application bytes not yet returned to the caller
	sawIAC  bool   // true once any IAC byte has been seen (i.e. peer speaks telnet)
}

func newTelnetBinaryConn(c net.Conn) *telnetBinaryConn {
	return &telnetBinaryConn{Conn: c}
}

func (t *telnetBinaryConn) Read(p []byte) (int, error) {
	if len(t.outbuf) > 0 {
		n := copy(p, t.outbuf)
		t.outbuf = t.outbuf[n:]
		return n, nil
	}
	for {
		buf := make([]byte, 4096)
		n, err := t.Conn.Read(buf)
		if n > 0 {
			data := append(t.pending, buf[:n]...)
			t.pending = nil
			decoded, replies, leftover := t.process(data)
			t.pending = leftover
			if len(replies) > 0 {
				_, _ = t.Conn.Write(replies) // best-effort negotiation reply
			}
			if len(decoded) > 0 {
				m := copy(p, decoded)
				if m < len(decoded) {
					t.outbuf = append(t.outbuf, decoded[m:]...)
				}
				return m, nil
			}
			// Only negotiation this round produced no app data; keep reading
			// so the caller isn't handed a spurious 0,nil.
			if err == nil {
				continue
			}
		}
		if err != nil {
			return 0, err
		}
	}
}

// process consumes a telnet byte stream, returning decoded application data,
// negotiation reply bytes to send, and any trailing incomplete IAC sequence.
func (t *telnetBinaryConn) process(data []byte) (decoded, replies, leftover []byte) {
	i := 0
	for i < len(data) {
		b := data[i]
		if b != tnIAC {
			decoded = append(decoded, b)
			i++
			continue
		}
		t.sawIAC = true
		if i+1 >= len(data) {
			leftover = data[i:]
			return
		}
		switch cmd := data[i+1]; cmd {
		case tnIAC: // escaped literal 0xFF (binary mode)
			decoded = append(decoded, tnIAC)
			i += 2
		case tnWILL, tnWONT, tnDO, tnDONT:
			if i+2 >= len(data) {
				leftover = data[i:]
				return
			}
			replies = append(replies, t.negotiate(cmd, data[i+2])...)
			i += 3
		case tnSB: // subnegotiation: discard through IAC SE
			j := i + 2
			for j+1 < len(data) && !(data[j] == tnIAC && data[j+1] == tnSE) {
				j++
			}
			if j+1 >= len(data) {
				leftover = data[i:]
				return
			}
			i = j + 2
		default: // other 2-byte command (GA, NOP, ...) — skip
			i += 2
		}
	}
	return
}

// negotiate answers an option request: accept BINARY and SGA, refuse the rest.
func (t *telnetBinaryConn) negotiate(cmd, opt byte) []byte {
	switch cmd {
	case tnDO:
		if opt == tnOptBinary || opt == tnOptSGA {
			return []byte{tnIAC, tnWILL, opt}
		}
		return []byte{tnIAC, tnWONT, opt}
	case tnWILL:
		if opt == tnOptBinary || opt == tnOptSGA {
			return []byte{tnIAC, tnDO, opt}
		}
		return []byte{tnIAC, tnDONT, opt}
	case tnWONT:
		return []byte{tnIAC, tnDONT, opt}
	case tnDONT:
		return []byte{tnIAC, tnWONT, opt}
	}
	return nil
}

func (t *telnetBinaryConn) Write(p []byte) (int, error) {
	if bytes.IndexByte(p, tnIAC) < 0 {
		return t.Conn.Write(p)
	}
	esc := make([]byte, 0, len(p)+8)
	for _, b := range p {
		if b == tnIAC {
			esc = append(esc, tnIAC, tnIAC)
		} else {
			esc = append(esc, b)
		}
	}
	if _, err := t.Conn.Write(esc); err != nil {
		return 0, err
	}
	return len(p), nil
}

// prefixConn returns bytes from `prefix` before reading from the wrapped
// connection, replaying bytes consumed during protocol classification so a
// downstream reader (e.g. an EMSI session) sees the full stream.
type prefixConn struct {
	net.Conn
	prefix []byte
}

func newPrefixConn(c net.Conn, prefix []byte) net.Conn {
	if len(prefix) == 0 {
		return c
	}
	return &prefixConn{Conn: c, prefix: prefix}
}

func (p *prefixConn) Read(b []byte) (int, error) {
	if len(p.prefix) > 0 {
		n := copy(b, p.prefix)
		p.prefix = p.prefix[n:]
		return n, nil
	}
	return p.Conn.Read(b)
}
