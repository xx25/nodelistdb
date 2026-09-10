package protocols

import (
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestTelnetProcess(t *testing.T) {
	tests := []struct {
		name        string
		in          []byte
		wantDecoded []byte
		wantReplies []byte
		wantLeft    []byte
	}{
		{
			name:        "DO BINARY accepted",
			in:          []byte{tnIAC, tnDO, tnOptBinary},
			wantReplies: []byte{tnIAC, tnWILL, tnOptBinary},
		},
		{
			name:        "DO ECHO refused",
			in:          []byte{tnIAC, tnDO, 1},
			wantReplies: []byte{tnIAC, tnWONT, 1},
		},
		{
			name:        "WILL SGA answered with DO",
			in:          []byte{tnIAC, tnWILL, tnOptSGA},
			wantReplies: []byte{tnIAC, tnDO, tnOptSGA},
		},
		{
			name:        "data around negotiation",
			in:          []byte{'H', 'I', tnIAC, tnDO, tnOptBinary, 'J'},
			wantDecoded: []byte("HIJ"),
			wantReplies: []byte{tnIAC, tnWILL, tnOptBinary},
		},
		{
			name:        "escaped 0xFF is literal data",
			in:          []byte{tnIAC, tnIAC, 'x'},
			wantDecoded: []byte{0xff, 'x'},
		},
		{
			name:        "incomplete IAC carried as leftover",
			in:          []byte{'a', tnIAC},
			wantDecoded: []byte("a"),
			wantLeft:    []byte{tnIAC},
		},
		{
			name:        "subnegotiation discarded",
			in:          []byte{'a', tnIAC, tnSB, 24, 'x', 'y', tnIAC, tnSE, 'b'},
			wantDecoded: []byte("ab"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &telnetBinaryConn{}
			decoded, replies, left := c.process(tt.in)
			if string(decoded) != string(tt.wantDecoded) {
				t.Errorf("decoded = % x, want % x", decoded, tt.wantDecoded)
			}
			if string(replies) != string(tt.wantReplies) {
				t.Errorf("replies = % x, want % x", replies, tt.wantReplies)
			}
			if string(left) != string(tt.wantLeft) {
				t.Errorf("leftover = % x, want % x", left, tt.wantLeft)
			}
		})
	}
}

// captureConn records everything written to it.
type captureConn struct {
	net.Conn
	written []byte
}

func (c *captureConn) Write(p []byte) (int, error) {
	c.written = append(c.written, p...)
	return len(p), nil
}

func TestTelnetWriteEscapesIAC(t *testing.T) {
	cc := &captureConn{}
	tn := newTelnetBinaryConn(cc)
	n, err := tn.Write([]byte{'a', tnIAC, 'b'})
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("Write returned n=%d, want 3 (logical bytes)", n)
	}
	want := []byte{'a', tnIAC, tnIAC, 'b'}
	if string(cc.written) != string(want) {
		t.Fatalf("wire bytes = % x, want % x", cc.written, want)
	}
}

// failingWriteConn delivers one read and refuses every write.
type failingWriteConn struct {
	net.Conn
	data []byte
}

func (c *failingWriteConn) Read(p []byte) (int, error) {
	if len(c.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, c.data)
	c.data = c.data[n:]
	return n, nil
}
func (c *failingWriteConn) Write([]byte) (int, error)        { return 0, errors.New("write refused") }
func (c *failingWriteConn) SetWriteDeadline(time.Time) error { return nil }

// A mailer's greeting shares a segment with its option requests. If our
// reply to those cannot be written, the greeting must still be delivered;
// the write failure is reported on the read after it.
func TestTelnetBinaryConnKeepsDecodedBytesWhenReplyWriteFails(t *testing.T) {
	greeting := "**EMSI_REQA77E\r"
	c := &failingWriteConn{data: append([]byte{tnIAC, tnWILL, tnOptBinary}, greeting...)}
	tn := newTelnetBinaryConn(c)

	buf := make([]byte, 64)
	n, err := tn.Read(buf)
	if err != nil {
		t.Fatalf("first read returned %v; the decoded greeting should come first", err)
	}
	if string(buf[:n]) != greeting {
		t.Fatalf("read %q, want the greeting", buf[:n])
	}
	if _, err := tn.Read(buf); err == nil || err.Error() != "write refused" {
		t.Fatalf("second read err = %v, want the deferred write failure", err)
	}
}
