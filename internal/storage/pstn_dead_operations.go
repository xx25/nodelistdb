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

// MarkDead marks a node's PSTN number as not to be dialled: dead/disconnected,
// or (mark.Kind = PSTNDeadKindVoice) answered by a person. An empty kind is
// "dead". mark.MarkedAt is ignored; the mark is stamped now.
//
// The two writers take the caller's context like the readers do. Each is a
// single-row INSERT whose only observer is the HTTP client that asked for it,
// so a client that disconnected mid-flight cannot tell whether the row landed
// either way, and the modem tester re-sends on its next cycle.
func (p *PSTNDeadOperations) MarkDead(ctx context.Context, mark PSTNDeadNode) error {
	kind := mark.Kind
	if kind == "" {
		kind = PSTNDeadKindDead
	}
	if !ValidPSTNDeadKind(kind) {
		return fmt.Errorf("invalid PSTN mark kind %q", kind)
	}

	conn := p.db.Conn()

	query := `INSERT INTO pstn_dead_nodes (zone, net, node, reason, kind, phone, marked_by, marked_at, is_active, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, true, ?)`

	now := time.Now()
	_, err := conn.ExecContext(ctx, query, mark.Zone, mark.Net, mark.Node, mark.Reason, kind, PhoneDigits(mark.Phone), mark.MarkedBy, now, now)
	if err != nil {
		return fmt.Errorf("failed to mark PSTN node dead: %w", err)
	}
	return nil
}

// UnmarkDead revives a previously marked PSTN node, whatever the mark's kind,
// by inserting an is_active=false row
func (p *PSTNDeadOperations) UnmarkDead(ctx context.Context, zone, net, node int, markedBy string) error {
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

// GetAllDeadNodes returns all currently marked PSTN nodes, of both kinds
func (p *PSTNDeadOperations) GetAllDeadNodes(ctx context.Context) ([]PSTNDeadNode, error) {
	conn := p.db.Conn()

	query := `SELECT zone, net, node, reason, kind, phone, marked_by, marked_at
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
		if err := rows.Scan(&n.Zone, &n.Net, &n.Node, &n.Reason, &n.Kind, &n.Phone, &n.MarkedBy, &n.MarkedAt); err != nil {
			return nil, fmt.Errorf("failed to scan PSTN dead node row: %w", err)
		}
		if n.Kind == "" {
			n.Kind = PSTNDeadKindDead
		}
		results = append(results, n)
	}

	return results, rows.Err()
}

// GetDeadNodeSet returns a map of marked nodes for in-memory enrichment.
// Key is [zone, net, node], value is the mark. Whether a mark still covers a
// node depends on the number the node lists now: see PSTNDeadNode.AppliesTo.
func (p *PSTNDeadOperations) GetDeadNodeSet(ctx context.Context) (map[[3]int]PSTNDeadNode, error) {
	nodes, err := p.GetAllDeadNodes(ctx)
	if err != nil {
		return nil, err
	}

	deadSet := make(map[[3]int]PSTNDeadNode, len(nodes))
	for _, n := range nodes {
		deadSet[[3]int{n.Zone, n.Net, n.Node}] = n
	}
	return deadSet, nil
}
