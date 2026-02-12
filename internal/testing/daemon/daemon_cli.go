package daemon

import (
	"context"
	"fmt"

	"github.com/nodelistdb/internal/testing/cli"
	"github.com/nodelistdb/internal/testing/logging"
	"github.com/nodelistdb/internal/testing/models"
)

// GetNodeInfo retrieves detailed information about a node from the database
func (d *Daemon) GetNodeInfo(ctx context.Context, zone, net, node uint16) (*cli.NodeInfo, error) {
	// Try to find the node in our storage
	nodes, err := d.storage.GetNodesByZone(ctx, int(zone))
	if err != nil {
		return &cli.NodeInfo{
			Address:      fmt.Sprintf("%d:%d/%d", zone, net, node),
			Found:        false,
			ErrorMessage: fmt.Sprintf("Failed to query database: %v", err),
		}, nil
	}

	// Find the specific node
	for _, n := range nodes {
		if n.Zone == int(zone) && n.Net == int(net) && n.Node == int(node) {
			return &cli.NodeInfo{
				Address:           fmt.Sprintf("%d:%d/%d", zone, net, node),
				SystemName:        n.SystemName,
				SysopName:         n.SysopName,
				Location:          n.Location,
				HasInternet:       n.HasInet,
				InternetHostnames: n.InternetHostnames,
				InternetProtocols: n.InternetProtocols,
				Found:             true,
			}, nil
		}
	}

	return &cli.NodeInfo{
		Address:      fmt.Sprintf("%d:%d/%d", zone, net, node),
		Found:        false,
		ErrorMessage: "Node not found in database",
	}, nil
}

// TestNodeDirect tests a specific node directly (for CLI)
func (d *Daemon) TestNodeDirect(ctx context.Context, zone, net, node uint16, hostname string) (*models.TestResult, error) {
	var testNode *models.Node

	// If no hostname provided, try to look up the node from database
	if hostname == "" {
		// Try to find the node in our storage
		nodes, err := d.storage.GetNodesByZone(ctx, int(zone))
		if err == nil {
			// Find the specific node
			for _, n := range nodes {
				if n.Zone == int(zone) && n.Net == int(net) && n.Node == int(node) {
					testNode = n
					break
				}
			}
		}

		if testNode == nil {
			// Node not found in database, create a minimal node
			return nil, fmt.Errorf("node %d:%d/%d not found in database and no hostname provided", zone, net, node)
		}
	} else {
		// Hostname was provided, create node with that hostname
		testNode = &models.Node{
			Zone:              int(zone),
			Net:               int(net),
			Node:              int(node),
			InternetHostnames: []string{hostname},
			HasInet:           true,
			// Enable all protocols for CLI testing when hostname is manually provided
			InternetProtocols: []string{"IBN", "IFC", "ITN", "IFT", "IVM"},
		}
	}

	result := d.testExecutor.TestNode(ctx, testNode)
	if result == nil {
		return nil, fmt.Errorf("test returned no result for node %d:%d/%d", zone, net, node)
	}

	// Store result if not in dry-run mode
	if !d.config.Daemon.DryRun {
		if err := d.storage.StoreTestResult(ctx, result); err != nil {
			logging.Infof("Failed to store test result: %v", err)
		}
	}

	return result, nil
}
