package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/nodelistdb/internal/database"
)

// PSTNDeadOperations handles PSTN dead node database operations
type PSTNDeadOperations struct {
	db database.DatabaseInterface
}

// NewPSTNDeadOperations creates a new PSTNDeadOperations instance
func NewPSTNDeadOperations(db database.DatabaseInterface) *PSTNDeadOperations {
	return &PSTNDeadOperations{db: db}
}

// MarkDead marks a node's PSTN number as dead/disconnected
func (p *PSTNDeadOperations) MarkDead(zone, net, node int, reason, markedBy string) error {
	ctx := context.Background()
	conn := p.db.Conn()

	query := `INSERT INTO pstn_dead_nodes (zone, net, node, reason, marked_by, marked_at, is_active, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, true, ?)`

	now := time.Now()
	_, err := conn.ExecContext(ctx, query, zone, net, node, reason, markedBy, now, now)
	if err != nil {
		return fmt.Errorf("failed to mark PSTN node dead: %w", err)
	}
	return nil
}

// UnmarkDead revives a previously dead PSTN node by inserting an is_active=false row
func (p *PSTNDeadOperations) UnmarkDead(zone, net, node int, markedBy string) error {
	ctx := context.Background()
	conn := p.db.Conn()

	query := `INSERT INTO pstn_dead_nodes (zone, net, node, reason, marked_by, marked_at, is_active, updated_at)
		VALUES (?, ?, ?, '', ?, ?, false, ?)`

	now := time.Now()
	_, err := conn.ExecContext(ctx, query, zone, net, node, markedBy, now, now)
	if err != nil {
		return fmt.Errorf("failed to unmark PSTN node dead: %w", err)
	}
	return nil
}

// GetAllDeadNodes returns all currently dead PSTN nodes
func (p *PSTNDeadOperations) GetAllDeadNodes() ([]PSTNDeadNode, error) {
	ctx := context.Background()
	conn := p.db.Conn()

	query := `SELECT zone, net, node, reason, marked_by, marked_at
		FROM pstn_dead_nodes FINAL
		WHERE is_active = true
		ORDER BY zone, net, node`

	rows, err := conn.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query PSTN dead nodes: %w", err)
	}
	defer rows.Close()

	var results []PSTNDeadNode
	for rows.Next() {
		var n PSTNDeadNode
		if err := rows.Scan(&n.Zone, &n.Net, &n.Node, &n.Reason, &n.MarkedBy, &n.MarkedAt); err != nil {
			return nil, fmt.Errorf("failed to scan PSTN dead node row: %w", err)
		}
		results = append(results, n)
	}

	return results, rows.Err()
}

// GetDeadNodeSet returns a map of dead nodes for in-memory enrichment.
// Key is [zone, net, node], value is reason.
func (p *PSTNDeadOperations) GetDeadNodeSet() (map[[3]int]string, error) {
	nodes, err := p.GetAllDeadNodes()
	if err != nil {
		return nil, err
	}

	deadSet := make(map[[3]int]string, len(nodes))
	for _, n := range nodes {
		deadSet[[3]int{n.Zone, n.Net, n.Node}] = n.Reason
	}
	return deadSet, nil
}
