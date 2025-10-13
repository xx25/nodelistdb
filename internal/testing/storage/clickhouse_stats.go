package storage

import (
	"context"
	"fmt"

	"github.com/nodelistdb/internal/testing/models"
)

// GetStatistics returns basic statistics about nodes
func (s *ClickHouseStorage) GetStatistics(ctx context.Context) (map[string]int, error) {
	query := `
		SELECT
			count(*) as total_nodes,
			countIf(has_inet = 1) as nodes_with_inet,
			countIf(has(internet_protocols, 'IBN')) as nodes_with_binkp,
			countIf(has(internet_protocols, 'IFC')) as nodes_with_ifcico,
			countIf(has(internet_protocols, 'ITN')) as nodes_with_telnet,
			countIf(has(internet_protocols, 'IFT')) as nodes_with_ftp
		FROM nodes
	`

	var stats struct {
		Total      int
		WithInet   int
		WithBinkP  int
		WithIfcico int
		WithTelnet int
		WithFTP    int
	}

	row := s.db.QueryRowContext(ctx, query)
	err := row.Scan(
		&stats.Total,
		&stats.WithInet,
		&stats.WithBinkP,
		&stats.WithIfcico,
		&stats.WithTelnet,
		&stats.WithFTP,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get statistics: %w", err)
	}

	return map[string]int{
		"total_nodes":       stats.Total,
		"nodes_with_inet":   stats.WithInet,
		"nodes_with_binkp":  stats.WithBinkP,
		"nodes_with_ifcico": stats.WithIfcico,
		"nodes_with_telnet": stats.WithTelnet,
		"nodes_with_ftp":    stats.WithFTP,
	}, nil
}

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
