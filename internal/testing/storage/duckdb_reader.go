package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/marcboeker/go-duckdb"
	"github.com/nodelistdb/internal/testing/models"
)

// DuckDBReader reads node data from DuckDB
type DuckDBReader struct {
	db   *sql.DB
	path string
}

// NewDuckDBReader creates a new DuckDB reader
func NewDuckDBReader(path string) (*DuckDBReader, error) {
	// Open in read-only mode
	connStr := fmt.Sprintf("%s?access_mode=read_only", path)
	db, err := sql.Open("duckdb", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open DuckDB: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping DuckDB: %w", err)
	}

	return &DuckDBReader{
		db:   db,
		path: path,
	}, nil
}

// Close closes the database connection
func (r *DuckDBReader) Close() error {
	if r.db != nil {
		return r.db.Close()
	}
	return nil
}

// GetNodesWithInternet retrieves all nodes with internet connectivity
func (r *DuckDBReader) GetNodesWithInternet(ctx context.Context, limit int) ([]*models.Node, error) {
	query := `
		SELECT DISTINCT
			zone, 
			net, 
			node,
			system_name,
			sysop_name,
			location,
			internet_hostnames,
			internet_protocols,
			has_inet
		FROM nodes
		WHERE has_inet = true
			AND array_length(internet_protocols) > 0
			AND nodelist_date = (SELECT MAX(nodelist_date) FROM nodes)
		ORDER BY zone, net, node
	`
	
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query nodes: %w", err)
	}
	defer rows.Close()

	var nodes []*models.Node
	for rows.Next() {
		node := &models.Node{}
		var hostnames, protocols sql.NullString

		err := rows.Scan(
			&node.Zone,
			&node.Net,
			&node.Node,
			&node.SystemName,
			&node.SysopName,
			&node.Location,
			&hostnames,
			&protocols,
			&node.HasInet,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Parse arrays (DuckDB returns as strings like ['host1', 'host2'])
		node.InternetHostnames = parseArray(hostnames.String)
		node.InternetProtocols = parseArray(protocols.String)

		nodes = append(nodes, node)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return nodes, nil
}

// GetNodesByZone retrieves nodes from a specific zone
func (r *DuckDBReader) GetNodesByZone(ctx context.Context, zone int) ([]*models.Node, error) {
	query := `
		SELECT DISTINCT
			zone, 
			net, 
			node,
			system_name,
			sysop_name,
			location,
			internet_hostnames,
			internet_protocols,
			has_inet
		FROM nodes
		WHERE zone = ?
			AND has_inet = true
			AND array_length(internet_protocols) > 0
			AND nodelist_date = (SELECT MAX(nodelist_date) FROM nodes)
		ORDER BY net, node
	`

	rows, err := r.db.QueryContext(ctx, query, zone)
	if err != nil {
		return nil, fmt.Errorf("failed to query nodes by zone: %w", err)
	}
	defer rows.Close()

	var nodes []*models.Node
	for rows.Next() {
		node := &models.Node{}
		var hostnames, protocols sql.NullString

		err := rows.Scan(
			&node.Zone,
			&node.Net,
			&node.Node,
			&node.SystemName,
			&node.SysopName,
			&node.Location,
			&hostnames,
			&protocols,
			&node.HasInet,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		node.InternetHostnames = parseArray(hostnames.String)
		node.InternetProtocols = parseArray(protocols.String)

		nodes = append(nodes, node)
	}

	return nodes, nil
}

// GetNodesByProtocol retrieves nodes that support a specific protocol
func (r *DuckDBReader) GetNodesByProtocol(ctx context.Context, protocol string, limit int) ([]*models.Node, error) {
	query := `
		SELECT DISTINCT
			zone, 
			net, 
			node,
			system_name,
			sysop_name,
			location,
			internet_hostnames,
			internet_protocols,
			has_inet
		FROM nodes
		WHERE has_inet = true
			AND list_contains(internet_protocols, ?)
			AND nodelist_date = (SELECT MAX(nodelist_date) FROM nodes)
		ORDER BY zone, net, node
	`
	
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := r.db.QueryContext(ctx, query, protocol)
	if err != nil {
		return nil, fmt.Errorf("failed to query nodes by protocol: %w", err)
	}
	defer rows.Close()

	var nodes []*models.Node
	for rows.Next() {
		node := &models.Node{}
		var hostnames, protocols sql.NullString

		err := rows.Scan(
			&node.Zone,
			&node.Net,
			&node.Node,
			&node.SystemName,
			&node.SysopName,
			&node.Location,
			&hostnames,
			&protocols,
			&node.HasInet,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		node.InternetHostnames = parseArray(hostnames.String)
		node.InternetProtocols = parseArray(protocols.String)

		nodes = append(nodes, node)
	}

	return nodes, nil
}

// GetStatistics returns basic statistics about nodes
func (r *DuckDBReader) GetStatistics(ctx context.Context) (map[string]int, error) {
	query := `
		SELECT 
			COUNT(*) as total_nodes,
			COUNT(CASE WHEN has_inet THEN 1 END) as nodes_with_inet,
			COUNT(CASE WHEN list_contains(internet_protocols, 'IBN') THEN 1 END) as nodes_with_binkp,
			COUNT(CASE WHEN list_contains(internet_protocols, 'IFC') THEN 1 END) as nodes_with_ifcico,
			COUNT(CASE WHEN list_contains(internet_protocols, 'ITN') THEN 1 END) as nodes_with_telnet,
			COUNT(CASE WHEN list_contains(internet_protocols, 'IFT') THEN 1 END) as nodes_with_ftp
		FROM nodes
		WHERE nodelist_date = (SELECT MAX(nodelist_date) FROM nodes)
	`

	var stats struct {
		Total         int
		WithInet      int
		WithBinkP     int
		WithIfcico    int
		WithTelnet    int
		WithFTP       int
	}

	row := r.db.QueryRowContext(ctx, query)
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
		"total_nodes":      stats.Total,
		"nodes_with_inet":  stats.WithInet,
		"nodes_with_binkp": stats.WithBinkP,
		"nodes_with_ifcico": stats.WithIfcico,
		"nodes_with_telnet": stats.WithTelnet,
		"nodes_with_ftp":   stats.WithFTP,
	}, nil
}

// parseArray parses DuckDB array string format to Go slice
// Example: ['host1', 'host2'] -> []string{"host1", "host2"}
func parseArray(s string) []string {
	if s == "" || s == "[]" || s == "NULL" {
		return []string{}
	}

	// Remove brackets
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")

	// Split by comma and clean up
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	
	for _, part := range parts {
		// Trim spaces and quotes
		part = strings.TrimSpace(part)
		part = strings.Trim(part, "'\"")
		if part != "" {
			result = append(result, part)
		}
	}

	return result
}