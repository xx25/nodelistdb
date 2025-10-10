package daemon

import (
	"time"

	"github.com/nodelistdb/internal/testing/models"
)

// StatisticsCollector handles calculation and aggregation of test statistics
type StatisticsCollector struct{}

// NewStatisticsCollector creates a new statistics collector
func NewStatisticsCollector() *StatisticsCollector {
	return &StatisticsCollector{}
}

// CalculateStatistics calculates statistics from a set of test results
func (sc *StatisticsCollector) CalculateStatistics(results []*models.TestResult) *models.TestStatistics {
	stats := &models.TestStatistics{
		Date:             time.Now().Truncate(24 * time.Hour),
		TotalNodesTested: uint32(len(results)),
		Countries:        make(map[string]uint32),
		ISPs:             make(map[string]uint32),
		ProtocolStats:    make(map[string]uint32),
		ErrorTypes:       make(map[string]uint32),
	}

	// Protocol-specific counters
	var binkpCount uint32
	var ifcicoCount uint32
	var binkpResponseTotal float64
	var ifcicoResponseTotal float64
	var binkpResponseCount uint32
	var ifcicoResponseCount uint32

	for _, result := range results {
		if result == nil {
			continue
		}

		// Count operational nodes
		if result.IsOperational {
			stats.NodesOperational++
		} else {
			stats.NodesWithIssues++
		}

		// Check DNS failures
		if result.DNSError != "" {
			stats.NodesDNSFailed++
			stats.ErrorTypes["DNS_FAIL"]++
		}

		// Geographic statistics
		if result.Country != "" {
			stats.Countries[result.Country]++
		}
		if result.ISP != "" {
			stats.ISPs[result.ISP]++
		}

		// Protocol statistics
		if result.BinkPResult != nil {
			if result.BinkPResult.Tested {
				binkpCount++
				stats.ProtocolStats["BinkP_Tested"]++
				if result.BinkPResult.Success {
					stats.ProtocolStats["BinkP_Success"]++
					// Track response time if available
					if result.BinkPResult.ResponseMs > 0 {
						binkpResponseTotal += float64(result.BinkPResult.ResponseMs)
						binkpResponseCount++
					}
				} else {
					stats.ProtocolStats["BinkP_Failed"]++
					if result.BinkPResult.Error != "" {
						stats.ErrorTypes["BinkP_"+result.BinkPResult.Error]++
					}
				}
			}
		}

		if result.IfcicoResult != nil {
			if result.IfcicoResult.Tested {
				ifcicoCount++
				stats.ProtocolStats["Ifcico_Tested"]++
				if result.IfcicoResult.Success {
					stats.ProtocolStats["Ifcico_Success"]++
					// Track response time if available
					if result.IfcicoResult.ResponseMs > 0 {
						ifcicoResponseTotal += float64(result.IfcicoResult.ResponseMs)
						ifcicoResponseCount++
					}
				} else {
					stats.ProtocolStats["Ifcico_Failed"]++
					if result.IfcicoResult.Error != "" {
						stats.ErrorTypes["Ifcico_"+result.IfcicoResult.Error]++
					}
				}
			}
		}

		if result.TelnetResult != nil {
			if result.TelnetResult.Tested {
				stats.ProtocolStats["Telnet_Tested"]++
				if result.TelnetResult.Success {
					stats.ProtocolStats["Telnet_Success"]++
				} else {
					stats.ProtocolStats["Telnet_Failed"]++
				}
			}
		}

		if result.FTPResult != nil {
			if result.FTPResult.Tested {
				stats.ProtocolStats["FTP_Tested"]++
				if result.FTPResult.Success {
					stats.ProtocolStats["FTP_Success"]++
				} else {
					stats.ProtocolStats["FTP_Failed"]++
				}
			}
		}

		if result.VModemResult != nil {
			if result.VModemResult.Tested {
				stats.ProtocolStats["VModem_Tested"]++
				if result.VModemResult.Success {
					stats.ProtocolStats["VModem_Success"]++
				} else {
					stats.ProtocolStats["VModem_Failed"]++
				}
			}
		}
	}

	// Set protocol node counts
	stats.NodesWithBinkP = binkpCount
	stats.NodesWithIfcico = ifcicoCount

	// Calculate average response times
	if binkpResponseCount > 0 {
		stats.AvgBinkPResponseMs = float32(binkpResponseTotal / float64(binkpResponseCount))
	}
	if ifcicoResponseCount > 0 {
		stats.AvgIfcicoResponseMs = float32(ifcicoResponseTotal / float64(ifcicoResponseCount))
	}

	return stats
}