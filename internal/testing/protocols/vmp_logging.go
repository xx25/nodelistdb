package protocols

import (
	"context"
	"log/slog"
	"strings"

	"github.com/nodelistdb/internal/testing/logging"
)

// vmpDebugLogger returns the logger fidomail/pkg/vmp writes its per-frame
// trace to, or nil when debug output is off.
//
// The VMP library takes an *slog.Logger because it depends on nothing but the
// standard library; this package logs through zerolog. The adapter below is
// the whole of the bridge, so the trace a VMP call emits lands in the same
// place as every other line the tester prints instead of on stderr.
func vmpDebugLogger(debug bool) *slog.Logger {
	if !debug {
		return nil
	}
	return slog.New(vmpLogHandler{})
}

type vmpLogHandler struct {
	prefix string
}

func (vmpLogHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h vmpLogHandler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(h.prefix)
	b.WriteString(r.Message)
	r.Attrs(func(a slog.Attr) bool {
		b.WriteString(" ")
		b.WriteString(a.Key)
		b.WriteString("=")
		b.WriteString(a.Value.String())
		return true
	})
	logging.Debugf("%s", b.String())
	return nil
}

func (h vmpLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	var b strings.Builder
	b.WriteString(h.prefix)
	for _, a := range attrs {
		b.WriteString(a.Key)
		b.WriteString("=")
		b.WriteString(a.Value.String())
		b.WriteString(" ")
	}
	return vmpLogHandler{prefix: b.String()}
}

func (h vmpLogHandler) WithGroup(name string) slog.Handler {
	return vmpLogHandler{prefix: h.prefix + name + "."}
}
