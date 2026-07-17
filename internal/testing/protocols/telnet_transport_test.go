package protocols

import (
	"net"
	"testing"
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
