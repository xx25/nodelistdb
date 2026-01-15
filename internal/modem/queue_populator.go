package modem

import (
	"context"
	"fmt"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/logging"
	"github.com/nodelistdb/internal/storage"
)

// QueuePopulator handles populating the modem test queue from nodelist data
type QueuePopulator struct {
	db           database.DatabaseInterface
	queueStorage *storage.ModemQueueOperations
	assigner     *ModemAssigner
}

// NewQueuePopulator creates a new QueuePopulator instance
func NewQueuePopulator(
	db database.DatabaseInterface,
	queueStorage *storage.ModemQueueOperations,
	assigner *ModemAssigner,
) *QueuePopulator {
	return &QueuePopulator{
		db:           db,
		queueStorage: queueStorage,
		assigner:     assigner,
	}
}

// PopulateFromNodelist adds nodes with phone numbers to the modem test queue
// Returns the number of nodes added
func (p *QueuePopulator) PopulateFromNodelist(ctx context.Context) (int, error) {
	// Get nodes with valid phone numbers that are not already in the queue
	nodes, err := p.getNodesWithPhonesNotInQueue(ctx)
	if err != nil {
		return 0, err
	}

	if len(nodes) == 0 {
		return 0, nil
	}

	added := 0
	for _, node := range nodes {
		entry := &storage.ModemQueueEntry{
			Zone:             node.Zone,
			Net:              node.Net,
			Node:             node.Node,
			ConflictSequence: node.ConflictSequence,
			Phone:            node.Phone,
			PhoneNormalized:  NormalizePhone(node.Phone),
			ModemFlags:       node.ModemFlags,
			Flags:            node.Flags,
			IsCM:             node.IsCM,
			TimeFlags:        node.TimeFlags,
			Priority:         calculatePriority(node),
			RetryCount:       0,
			Status:           storage.ModemQueueStatusPending,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		}

		// Skip if phone normalization failed
		if entry.PhoneNormalized == "" {
			continue
		}

		if err := p.assigner.AssignModemTestNode(ctx, entry); err != nil {
			logging.Debug("failed to assign node to modem queue",
				"node", node.Address(),
				"phone", node.Phone,
				"error", err)
			continue
		}

		added++
	}

	return added, nil
}

// getNodesWithPhonesNotInQueue retrieves nodes with phones that aren't in the queue
func (p *QueuePopulator) getNodesWithPhonesNotInQueue(ctx context.Context) ([]NodeWithPhone, error) {
	// Query nodes from the latest nodelist that have valid phone numbers
	// This is a simplified implementation - in production you may want
	// to use a more efficient query with a LEFT ANTI JOIN

	// Get all nodes with phone numbers from the latest nodelist
	allNodes, err := p.queryNodesWithPhones(ctx)
	if err != nil {
		return nil, err
	}

	// Filter out nodes already in queue
	var result []NodeWithPhone
	for _, node := range allNodes {
		exists, err := p.queueStorage.NodeExistsInQueue(ctx, node.Zone, node.Net, node.Node, node.ConflictSequence)
		if err != nil {
			logging.Warn("failed to check queue existence",
				"node", node.Address(),
				"error", err)
			continue
		}
		if !exists {
			result = append(result, node)
		}
	}

	return result, nil
}

// NodeWithPhone represents a node with phone information for queue population
type NodeWithPhone struct {
	Zone             uint16
	Net              uint16
	Node             uint16
	ConflictSequence uint8
	Phone            string
	ModemFlags       []string
	Flags            []string
	IsCM             bool
	TimeFlags        []string
}

// Address returns a human-readable address string
func (n *NodeWithPhone) Address() string {
	return fmt.Sprintf("%d:%d/%d", n.Zone, n.Net, n.Node)
}

// queryNodesWithPhones retrieves nodes with phone numbers from the database
func (p *QueuePopulator) queryNodesWithPhones(ctx context.Context) ([]NodeWithPhone, error) {
	query := `
		SELECT
			zone, net, node, conflict_sequence,
			phone, modem_flags, flags, is_cm
		FROM nodes
		WHERE nodelist_date = (SELECT MAX(nodelist_date) FROM nodes)
		AND phone != ''
		AND phone != '-Unpublished-'
		AND phone NOT LIKE '%-000-NETWORK%'
	`

	rows, err := p.db.Conn().QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query nodes with phones: %w", err)
	}
	defer rows.Close()

	var nodes []NodeWithPhone
	for rows.Next() {
		var node NodeWithPhone
		if err := rows.Scan(
			&node.Zone,
			&node.Net,
			&node.Node,
			&node.ConflictSequence,
			&node.Phone,
			&node.ModemFlags,
			&node.Flags,
			&node.IsCM,
		); err != nil {
			logging.Warn("failed to scan node row", "error", err)
			continue
		}

		// Validate phone
		if NormalizePhone(node.Phone) == "" {
			continue
		}

		// Extract time flags from general flags for callable window calculation
		node.TimeFlags = extractTimeFlags(node.Flags)

		nodes = append(nodes, node)
	}

	return nodes, nil
}

// extractTimeFlags extracts time-related flags (T-flags) from general flags
func extractTimeFlags(flags []string) []string {
	var timeFlags []string
	for _, flag := range flags {
		// T-flags typically start with T followed by two letter codes
		// Examples: Txy (like T18, TNA, etc.)
		if len(flag) >= 2 && flag[0] == 'T' {
			timeFlags = append(timeFlags, flag)
		}
	}
	return timeFlags
}

// calculatePriority determines queue priority based on node attributes
func calculatePriority(node NodeWithPhone) uint8 {
	priority := uint8(50) // Base priority

	// Higher priority for CM nodes (continuous mail, always available)
	if node.IsCM {
		priority += 20
	}

	// Higher priority for nodes with fast modem flags
	for _, flag := range node.ModemFlags {
		switch flag {
		case "V34", "V32B", "V32":
			priority += 10
		case "HST", "ZYX":
			priority += 5
		}
	}

	// Cap at 100
	if priority > 100 {
		priority = 100
	}

	return priority
}

// GetQueuedCount returns the number of nodes currently in the queue
func (p *QueuePopulator) GetQueuedCount(ctx context.Context) (int, error) {
	stats, err := p.queueStorage.GetQueueStats(ctx)
	if err != nil {
		return 0, err
	}
	return stats.TotalNodes, nil
}
