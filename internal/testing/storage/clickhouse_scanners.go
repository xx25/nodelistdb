package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/nodelistdb/internal/testing/models"
)

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
		node.ProtocolPorts = make(map[string]int)
		node.InternetHostnames = []string{} // Initialize hostnames array

		if configJSON != "" && configJSON != "{}" {
			var config map[string]interface{}
			if err := json.Unmarshal([]byte(configJSON), &config); err == nil {
				// Store full config for later use
				node.InternetConfig = config

				// Extract all addresses from all protocols (now supports arrays)
				if protocols, ok := config["protocols"].(map[string]interface{}); ok {
					for proto, protoData := range protocols {
						// Handle both old format (single object) and new format (array of objects)
						switch v := protoData.(type) {
						case []interface{}:
							// New format: array of protocol details
							for _, item := range v {
								if protoMap, ok := item.(map[string]interface{}); ok {
									// Extract address
									if addr, ok := protoMap["address"].(string); ok && addr != "" {
										node.InternetHostnames = append(node.InternetHostnames, addr)
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
							if addr, ok := v["address"].(string); ok && addr != "" {
								node.InternetHostnames = append(node.InternetHostnames, addr)
							}

							// Extract port
							if portFloat, ok := v["port"].(float64); ok {
								node.ProtocolPorts[proto] = int(portFloat)
							}
						}
					}
				}

				// Also check defaults for INA address
				if defaults, ok := config["defaults"].(map[string]interface{}); ok {
					if ina, ok := defaults["INA"].(string); ok && ina != "" {
						// Add INA if not already in hostnames
						found := false
						for _, h := range node.InternetHostnames {
							if h == ina {
								found = true
								break
							}
						}
						if !found {
							node.InternetHostnames = append(node.InternetHostnames, ina)
						}
					}
				}
			}
		}

		nodes = append(nodes, node)
	}
	return nodes, nil
}

// scanNodes scans rows from SQL database (legacy support)
func scanNodes(rows *sql.Rows) ([]*models.Node, error) {
	var nodes []*models.Node
	for rows.Next() {
		node := &models.Node{}
		var hostnames, protocols string

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

		nodes = append(nodes, node)
	}
	return nodes, nil
}
