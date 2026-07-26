package daemon

import (
	"context"
	"net"
	"testing"
)

// A run-once cycle must not bind the CLI port. On the host where such a run is
// most useful the installed service already holds it, and binding would kill
// the hand-run before it tested anything.
func TestStartCLIServer_RunOnceDoesNotBind(t *testing.T) {
	// Hold the port the way the service would.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to take a port to stand in for the service: %v", err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port

	d := &Daemon{config: &Config{
		CLI: CLIConfig{Enabled: true, Host: "127.0.0.1", Port: port},
	}}
	d.config.Daemon.RunOnce = true

	if err := d.StartCLIServer(context.Background()); err != nil {
		t.Errorf("run-once start should skip the CLI server, got: %v", err)
	}
}

// -cli-only is the opposite case: the CLI is the only way to reach the daemon,
// so it must still come up even when the run is nominally a single cycle.
func TestStartCLIServer_CLIOnlyStillBinds(t *testing.T) {
	d := &Daemon{config: &Config{
		CLI: CLIConfig{Enabled: true, Host: "127.0.0.1", Port: 0},
	}}
	d.config.Daemon.RunOnce = true
	d.config.Daemon.CLIOnly = true

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := d.StartCLIServer(ctx); err != nil {
		t.Errorf("cli-only start should bring the CLI server up, got: %v", err)
	}
}

// Disabled in config stays disabled whatever the run mode.
func TestStartCLIServer_Disabled(t *testing.T) {
	d := &Daemon{config: &Config{CLI: CLIConfig{Enabled: false}}}

	if err := d.StartCLIServer(context.Background()); err != nil {
		t.Errorf("disabled CLI should be a no-op, got: %v", err)
	}
}
