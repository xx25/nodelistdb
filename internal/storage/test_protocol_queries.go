package storage

import (
	"fmt"
	"sync"

	"github.com/nodelistdb/internal/database"
)

// ProtocolQueryOperations handles protocol-specific test result queries
// Replaces 6 nearly identical methods with a single generic implementation
type ProtocolQueryOperations struct {
	db           database.DatabaseInterface
	queryBuilder *TestQueryBuilder
	resultParser ResultParserInterface
	mu           sync.RWMutex
}

// NewProtocolQueryOperations creates a new protocol query operations instance
func NewProtocolQueryOperations(db database.DatabaseInterface, queryBuilder *TestQueryBuilder, resultParser ResultParserInterface) *ProtocolQueryOperations {
	return &ProtocolQueryOperations{
		db:           db,
		queryBuilder: queryBuilder,
		resultParser: resultParser,
	}
}

// GetProtocolEnabledNodes retrieves nodes that have successfully tested with a specific protocol
// This is the generic implementation that replaces 6 duplicate methods
func (pq *ProtocolQueryOperations) GetProtocolEnabledNodes(protocol string, limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	conn := pq.db.Conn()

	// Build node filter condition
	nodeFilter := ""
	if !includeZeroNodes {
		nodeFilter = "AND node != 0"
	}

	query := pq.queryBuilder.BuildProtocolEnabledQuery(protocol, nodeFilter)

	rows, err := conn.Query(query, days, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search %s enabled nodes: %w", protocol, err)
	}
	defer rows.Close()

	var results []NodeTestResult
	for rows.Next() {
		var r NodeTestResult
		err := pq.resultParser.ParseTestResultRow(rows, &r)
		if err != nil {
			return nil, fmt.Errorf("failed to parse test result: %w", err)
		}
		results = append(results, r)
	}

	return results, nil
}

// GetBinkPEnabledNodes returns nodes that have been successfully tested with BinkP
func (pq *ProtocolQueryOperations) GetBinkPEnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	return pq.GetProtocolEnabledNodes("binkp", limit, days, includeZeroNodes)
}

// GetIfcicoEnabledNodes returns nodes that have been successfully tested with IFCICO
func (pq *ProtocolQueryOperations) GetIfcicoEnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	return pq.GetProtocolEnabledNodes("ifcico", limit, days, includeZeroNodes)
}

// GetTelnetEnabledNodes returns nodes that have been successfully tested with Telnet
func (pq *ProtocolQueryOperations) GetTelnetEnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	return pq.GetProtocolEnabledNodes("telnet", limit, days, includeZeroNodes)
}

// GetVModemEnabledNodes returns nodes that have been successfully tested with VModem
func (pq *ProtocolQueryOperations) GetVModemEnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	return pq.GetProtocolEnabledNodes("vmodem", limit, days, includeZeroNodes)
}

// GetFTPEnabledNodes returns nodes that have been successfully tested with FTP
func (pq *ProtocolQueryOperations) GetFTPEnabledNodes(limit int, days int, includeZeroNodes bool) ([]NodeTestResult, error) {
	return pq.GetProtocolEnabledNodes("ftp", limit, days, includeZeroNodes)
}
