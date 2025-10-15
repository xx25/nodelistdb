package logging

import (
	"fmt"
	"log/slog"
	"time"
)

// Common field helpers for consistent structured logging

// Node creates node address fields
func Node(zone, net, node int) []any {
	return []any{
		slog.Int("zone", zone),
		slog.Int("net", net),
		slog.Int("node", node),
	}
}

// Address formats FidoNet address
func Address(zone, net, node int) slog.Attr {
	return slog.String("address", fmt.Sprintf("%d:%d/%d", zone, net, node))
}

// Duration logs duration in milliseconds
func Duration(name string, d time.Duration) slog.Attr {
	return slog.Int64(name+"_ms", d.Milliseconds())
}

// Err creates error field
func Err(err error) slog.Attr {
	if err == nil {
		return slog.String("error", "")
	}
	return slog.String("error", err.Error())
}

// Count creates count field
func Count(name string, count int) slog.Attr {
	return slog.Int(name+"_count", count)
}

// Database creates database operation fields
func Database(operation, table string) []any {
	return []any{
		slog.String("db_operation", operation),
		slog.String("db_table", table),
	}
}

// HTTP creates HTTP request fields
func HTTP(method, path string, status int) []any {
	return []any{
		slog.String("http_method", method),
		slog.String("http_path", path),
		slog.Int("http_status", status),
	}
}

// Protocol creates protocol testing fields
func Protocol(name string) slog.Attr {
	return slog.String("protocol", name)
}

// Hostname creates hostname field
func Hostname(hostname string) slog.Attr {
	return slog.String("hostname", hostname)
}

// IP creates IP address field
func IP(ip string) slog.Attr {
	return slog.String("ip", ip)
}

// File creates file path field
func File(path string) slog.Attr {
	return slog.String("file", path)
}

// Query creates query operation field
func Query(operation string) slog.Attr {
	return slog.String("query", operation)
}

// BatchSize creates batch size field
func BatchSize(size int) slog.Attr {
	return slog.Int("batch_size", size)
}

// Worker creates worker ID field
func Worker(id int) slog.Attr {
	return slog.Int("worker_id", id)
}
