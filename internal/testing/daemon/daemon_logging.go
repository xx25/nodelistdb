package daemon

import (
	"fmt"
	"strings"

	"github.com/nodelistdb/internal/testing/logging"
	"github.com/nodelistdb/internal/testing/models"
)

// logProtocolResult logs the dual-stack test results for a protocol
func (d *Daemon) logProtocolResult(nodeAddr, protocol string, result *models.ProtocolTestResult) {
	if result == nil || !result.Tested {
		return
	}

	var status string
	if result.IPv4Success && result.IPv6Success {
		status = fmt.Sprintf("✓ Dual-stack (IPv4=%dms, IPv6=%dms)",
			result.IPv4ResponseMs, result.IPv6ResponseMs)
	} else if result.IPv6Success {
		status = fmt.Sprintf("✓ IPv6-only (%dms)", result.IPv6ResponseMs)
	} else if result.IPv4Success {
		status = fmt.Sprintf("✓ IPv4-only (%dms)", result.IPv4ResponseMs)
	} else {
		var tried []string
		if result.IPv4Tested {
			tried = append(tried, "IPv4")
		}
		if result.IPv6Tested {
			tried = append(tried, "IPv6")
		}
		status = fmt.Sprintf("✗ Failed (tried: %s)", strings.Join(tried, ", "))
	}

	logging.Debugf("[%s]   %s: %s", nodeAddr, protocol, status)
}

// logConnectivitySummary logs a summary of the node's connectivity
func (d *Daemon) logConnectivitySummary(nodeAddr string, node *models.Node, result *models.TestResult) {
	var summary []string

	// Count dual-stack protocols
	dualStackCount := 0
	ipv4OnlyCount := 0
	ipv6OnlyCount := 0

	protocols := []struct {
		name   string
		result *models.ProtocolTestResult
	}{
		{"BinkP", result.BinkPResult},
		{"IFCICO", result.IfcicoResult},
		{"Telnet", result.TelnetResult},
		{"FTP", result.FTPResult},
		{"VModem", result.VModemResult},
	}

	for _, p := range protocols {
		if p.result != nil && p.result.Tested {
			connType := p.result.GetConnectivityType()
			switch connType {
			case "dual-stack":
				dualStackCount++
			case "ipv6-only":
				ipv6OnlyCount++
			case "ipv4-only":
				ipv4OnlyCount++
			}
		}
	}

	if dualStackCount > 0 {
		summary = append(summary, fmt.Sprintf("%d dual-stack", dualStackCount))
	}
	if ipv6OnlyCount > 0 {
		summary = append(summary, fmt.Sprintf("%d IPv6-only", ipv6OnlyCount))
	}
	if ipv4OnlyCount > 0 {
		summary = append(summary, fmt.Sprintf("%d IPv4-only", ipv4OnlyCount))
	}

	if len(summary) > 0 {
		logging.Infof("[%s] connectivity: %s protocols", nodeAddr, strings.Join(summary, ", "))
	}
}
