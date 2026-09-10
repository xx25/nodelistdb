package protocols

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/xx25/fidomail/pkg/emsi"
	"github.com/xx25/fidomail/pkg/xfer"

	// The transfer drivers register themselves by their EMSI compatibility
	// code on import. ZMO and ZAP are the two we offer in EMSI_DAT.
	_ "github.com/xx25/fidomail/pkg/xfer/zedzap"
	_ "github.com/xx25/fidomail/pkg/xfer/zmodem"

	"github.com/nodelistdb/internal/testing/logging"
)

// emsiFinishTimeout bounds the empty transfer phase. Two ZMODEM turnarounds
// with nothing in them take well under a second on any link; the bound is
// for a peer that stalls.
const emsiFinishTimeout = 20 * time.Second

// finishEMSISession plays out the transfer phase of a session in which we
// have nothing to send, so the remote mailer sees a completed session rather
// than a caller that vanished.
//
// After EMSI the answerer starts the negotiated transfer protocol expecting
// the caller to send first. Closing the socket at that point made mbcico log
// "Zmodem: could not initiate receive" and count the session as failed
// (rc=124), and every other mailer records the equivalent. An empty batch is
// what a mailer with no mail sends: ZRINIT answered by ZFIN, the turnaround,
// and the remote's own batch, in which anything it offers is declined with
// ZSKIP — we advertise pickup, as a mailer does, but have nowhere to put a
// file.
//
// The error is informational: the handshake already proved the mailer, and
// how the session ended is logged, not held against the node.
func finishEMSISession(ctx context.Context, sess *emsi.Session, expectedAddress string) error {
	name := sess.GetSelectedProtocol()
	if name == "" {
		return fmt.Errorf("no transfer protocol negotiated")
	}
	drv, ok := xfer.ByName(name)
	if !ok {
		return fmt.Errorf("no driver for negotiated protocol %s", name)
	}
	conn := sess.HandoffConn()
	if conn == nil {
		return fmt.Errorf("session has no connection")
	}

	ctx, cancel := context.WithTimeout(ctx, emsiFinishTimeout)
	defer cancel()
	_ = conn.SetDeadline(time.Now().Add(emsiFinishTimeout))

	res, err := drv.Exchange(ctx, conn, xfer.RoleOriginator, nothingToSend{}, declineEverything{}, transferLogger())
	if err != nil {
		return fmt.Errorf("%s transfer phase: %w", name, err)
	}
	if res != nil && len(res.Received) > 0 {
		logging.Debugf("EMSI: %s offered %d file(s) during the empty session; all declined", expectedAddress, len(res.Received))
	}

	// Let the remote hang up first. The last thing a ZMODEM sender writes
	// is its "OO" after our ZFIN, and our receiver does not wait for it;
	// closing now would turn that write into a broken pipe on the mailer's
	// side. Reading until it closes, or for a moment, lets it finish.
	_ = conn.SetReadDeadline(time.Now().Add(emsiLingerWindow))
	_, _ = io.Copy(io.Discard, conn)
	return nil
}

// emsiLingerWindow bounds the wait for the remote to close after the
// transfer phase.
const emsiLingerWindow = 1500 * time.Millisecond

// transferLogger routes the transfer driver's slog output into our own
// logger at debug level. The alternative — discarding it — makes an
// interop failure in the data phase undiagnosable, which is exactly the
// class of bug this phase exists to avoid causing.
func transferLogger() *slog.Logger {
	if !logging.DebugEnabled() {
		return slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return slog.New(slog.NewTextHandler(logging.DebugWriter(), &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// nothingToSend is the send side of an empty batch.
type nothingToSend struct{}

func (nothingToSend) NextFile() *xfer.FileOffer { return nil }
func (nothingToSend) AcceptFile(xfer.FileInfo) (xfer.AcceptDecision, io.WriteCloser, int64) {
	return xfer.SkipRS, nil, 0
}
func (nothingToSend) Progress(xfer.FileInfo, int64)         {}
func (nothingToSend) Completed(xfer.FileInfo, int64, error) {}

// declineEverything is the receive side: every file the remote offers is
// skipped, which a ZMODEM sender takes as "not this time" and moves on.
type declineEverything struct{}

func (declineEverything) NextFile() *xfer.FileOffer { return nil }
func (declineEverything) AcceptFile(xfer.FileInfo) (xfer.AcceptDecision, io.WriteCloser, int64) {
	return xfer.SkipRS, nil, 0
}
func (declineEverything) Progress(xfer.FileInfo, int64)         {}
func (declineEverything) Completed(xfer.FileInfo, int64, error) {}
