package storage

import (
	"context"
	"fmt"

	"github.com/nodelistdb/internal/testing/models"
)

// StoreDailyStats stores daily statistics
func (s *ClickHouseStorage) StoreDailyStats(ctx context.Context, stats *models.TestStatistics) error {
	// Use batch insert for proper Map type handling
	batch, err := s.conn.PrepareBatch(ctx, `INSERT INTO node_test_daily_stats`)
	if err != nil {
		return fmt.Errorf("failed to prepare batch: %w", err)
	}

	// Convert nil maps to empty maps to avoid ClickHouse parsing errors
	countries := stats.Countries
	if countries == nil {
		countries = make(map[string]uint32)
	}

	isps := stats.ISPs
	if isps == nil {
		isps = make(map[string]uint32)
	}

	protocolStats := stats.ProtocolStats
	if protocolStats == nil {
		protocolStats = make(map[string]uint32)
	}

	errorTypes := stats.ErrorTypes
	if errorTypes == nil {
		errorTypes = make(map[string]uint32)
	}

	// Append the row to batch
	err = batch.Append(
		stats.Date,
		stats.TotalNodesTested,
		stats.NodesWithBinkP,
		stats.NodesWithIfcico,
		stats.NodesOperational,
		stats.NodesWithIssues,
		stats.NodesDNSFailed,
		stats.AvgBinkPResponseMs,
		stats.AvgIfcicoResponseMs,
		countries,
		isps,
		protocolStats,
		errorTypes,
	)
	if err != nil {
		return fmt.Errorf("failed to append to batch: %w", err)
	}

	// Send the batch
	if err := batch.Send(); err != nil {
		return fmt.Errorf("failed to send batch: %w", err)
	}

	return nil
}
