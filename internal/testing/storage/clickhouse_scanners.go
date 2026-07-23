package storage

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/nodelistdb/internal/testing/models"
)

// applyInternetConfig fills a node's hostnames, protocol ports and info flags
// from its stored internet_config JSON. Every address the config carries is
// collected: a node is reachable at each of them, and the daemon tests them all.
func applyInternetConfig(node *models.Node, configJSON string) {
	node.ProtocolPorts = make(map[string]int)
	node.InternetHostnames = []string{}

	if configJSON == "" || configJSON == "{}" {
		return
	}

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return
	}

	// Store full config for later use
	node.InternetConfig = config

	// Extract all addresses from all protocols (now supports arrays).
	// Protocol keys are walked in sorted order so the hostname list - and with
	// it hostname_index and GetPrimaryHostname() - stays stable across test
	// cycles instead of following map iteration order.
	if protocols, ok := config["protocols"].(map[string]interface{}); ok {
		for _, proto := range sortedKeys(protocols) {
			// Handle both old format (single object) and new format (array of objects)
			switch v := protocols[proto].(type) {
			case []interface{}:
				// New format: array of protocol details
				for _, item := range v {
					if protoMap, ok := item.(map[string]interface{}); ok {
						// Extract address
						if addr, ok := protoMap["address"].(string); ok {
							node.InternetHostnames = appendHostname(node.InternetHostnames, addr)
						}

						// Extract port (store first non-default port found)
						if _, exists := node.ProtocolPorts[proto]; !exists {
							if portFloat, ok := protoMap["port"].(float64); ok {
								node.ProtocolPorts[proto] = int(portFloat)
							}
						}
					}
				}
			case map[string]interface{}:
				// Old format: single protocol detail object
				// Extract address
				if addr, ok := v["address"].(string); ok {
					node.InternetHostnames = appendHostname(node.InternetHostnames, addr)
				}

				// Extract port
				if portFloat, ok := v["port"].(float64); ok {
					node.ProtocolPorts[proto] = int(portFloat)
				}
			}
		}
	}

	// Also check defaults for INA addresses. A nodelist line may repeat the flag
	// ("INA:a,INA:b") and each value is a hostname the node answers on, so all
	// of them are tested. Rows written before INA became a list hold a bare
	// string.
	if defaults, ok := config["defaults"].(map[string]interface{}); ok {
		switch ina := defaults["INA"].(type) {
		case string:
			node.InternetHostnames = appendHostname(node.InternetHostnames, ina)
		case []interface{}:
			for _, item := range ina {
				if addr, ok := item.(string); ok {
					node.InternetHostnames = appendHostname(node.InternetHostnames, addr)
				}
			}
		}
	}

	// Extract info_flags array (INO4, INO6, ICM)
	if infoFlags, ok := config["info_flags"].([]interface{}); ok {
		node.InfoFlags = make([]string, 0, len(infoFlags))
		for _, flag := range infoFlags {
			if flagStr, ok := flag.(string); ok {
				node.InfoFlags = append(node.InfoFlags, flagStr)
			}
		}
	}
}

// sortedKeys returns the map's keys in a stable order.
func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// appendHostname adds a hostname unless it is empty or already present.
func appendHostname(hostnames []string, hostname string) []string {
	if hostname == "" {
		return hostnames
	}
	for _, existing := range hostnames {
		if existing == hostname {
			return hostnames
		}
	}
	return append(hostnames, hostname)
}

// scanNodesNative scans rows from native ClickHouse driver
func scanNodesNative(rows driver.Rows) ([]*models.Node, error) {
	var nodes []*models.Node
	for rows.Next() {
		node := &models.Node{}
		var hostnames, protocols string
		var zone, net, nodeNum int32
		var configJSON string

		err := rows.Scan(
			&zone,
			&net,
			&nodeNum,
			&node.SystemName,
			&node.SysopName,
			&node.Location,
			&hostnames,
			&protocols,
			&node.HasInet,
			&configJSON,
			&node.Domain,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Convert int32 values to int
		node.Zone = int(zone)
		node.Net = int(net)
		node.Node = int(nodeNum)

		// Convert hostname string to array and handle addresses with embedded ports
		if hostnames != "" {
			// Check if the hostname contains a port (e.g., "185.22.236.179:2030" for IVM)
			if strings.Contains(hostnames, ":") {
				// Split hostname and port
				parts := strings.SplitN(hostnames, ":", 2)
				if len(parts) == 2 {
					node.InternetHostnames = []string{parts[0]}
					// Store the port for IVM protocol if it's the only protocol
					if len(node.InternetProtocols) == 1 && node.InternetProtocols[0] == "IVM" {
						if port, err := strconv.Atoi(parts[1]); err == nil {
							node.ProtocolPorts["IVM"] = port
						}
					}
				} else {
					node.InternetHostnames = []string{hostnames}
				}
			} else {
				node.InternetHostnames = []string{hostnames}
			}
		} else {
			node.InternetHostnames = []string{}
		}

		// Parse protocols from comma-separated string
		if protocols != "" {
			node.InternetProtocols = strings.Split(protocols, ",")
			// Trim spaces from each protocol
			for i := range node.InternetProtocols {
				node.InternetProtocols[i] = strings.TrimSpace(node.InternetProtocols[i])
			}
		} else {
			node.InternetProtocols = []string{}
		}

		// Parse internet_config JSON to extract ALL addresses and custom ports
		applyInternetConfig(node, configJSON)

		nodes = append(nodes, node)
	}
	return nodes, nil
}

