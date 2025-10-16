package storage

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/nodelistdb/internal/database"
)

// detectInternetConfigChanges compares two JSON configs and returns detailed changes
func (so *SearchOperations) detectInternetConfigChanges(prev, curr json.RawMessage) map[string]string {
	changes := make(map[string]string)

	prevConfig, prevErr := so.parseInternetConfig(prev)
	currConfig, currErr := so.parseInternetConfig(curr)

	// Handle errors or nil configs
	if prevErr != nil || currErr != nil {
		if len(prev) > 0 && len(curr) == 0 {
			changes["internet_config"] = "Removed all internet configuration"
		} else if len(prev) == 0 && len(curr) > 0 {
			changes["internet_config"] = "Added internet configuration"
		}
		return changes
	}

	if prevConfig == nil && currConfig == nil {
		return changes
	}

	// Check if previous config is empty (no protocols, defaults, or email protocols)
	prevIsEmpty := prevConfig == nil || (len(prevConfig.Protocols) == 0 && len(prevConfig.Defaults) == 0 && len(prevConfig.EmailProtocols) == 0 && len(prevConfig.InfoFlags) == 0)
	currIsEmpty := currConfig == nil || (len(currConfig.Protocols) == 0 && len(currConfig.Defaults) == 0 && len(currConfig.EmailProtocols) == 0 && len(currConfig.InfoFlags) == 0)

	if prevIsEmpty && !currIsEmpty {
		// New config added (was empty or nil before)
		for proto, details := range currConfig.Protocols {
			// Handle array of protocol details
			var addresses []string
			for _, detail := range details {
				if detail.Address != "" && detail.Port > 0 {
					addresses = append(addresses, fmt.Sprintf("%s:%d", detail.Address, detail.Port))
				} else if detail.Address != "" {
					addresses = append(addresses, detail.Address)
				} else if detail.Port > 0 {
					addresses = append(addresses, fmt.Sprintf("port %d", detail.Port))
				}
			}
			if len(addresses) > 0 {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Added %s", strings.Join(addresses, ", "))
			} else {
				changes[fmt.Sprintf("inet_%s", proto)] = "Added"
			}
		}
		for key, val := range currConfig.Defaults {
			changes[fmt.Sprintf("inet_%s", key)] = fmt.Sprintf("Added %s", val)
		}
		for key, details := range currConfig.EmailProtocols {
			// Handle array of email protocol details
			var emails []string
			for _, detail := range details {
				if detail.Email != "" {
					emails = append(emails, detail.Email)
				}
			}
			if len(emails) > 0 {
				changes[fmt.Sprintf("inet_%s", key)] = fmt.Sprintf("Added %s", strings.Join(emails, ", "))
			} else {
				changes[fmt.Sprintf("inet_%s", key)] = "Added"
			}
		}
		return changes
	}

	if !prevIsEmpty && currIsEmpty {
		// Config removed (became empty or nil)
		for proto, details := range prevConfig.Protocols {
			// Handle array of protocol details
			var addresses []string
			for _, detail := range details {
				if detail.Address != "" && detail.Port > 0 {
					addresses = append(addresses, fmt.Sprintf("%s:%d", detail.Address, detail.Port))
				} else if detail.Address != "" {
					addresses = append(addresses, detail.Address)
				} else if detail.Port > 0 {
					addresses = append(addresses, fmt.Sprintf("port %d", detail.Port))
				}
			}
			if len(addresses) > 0 {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Removed %s", strings.Join(addresses, ", "))
			} else {
				changes[fmt.Sprintf("inet_%s", proto)] = "Removed"
			}
		}
		for key, val := range prevConfig.Defaults {
			changes[fmt.Sprintf("inet_%s", key)] = fmt.Sprintf("Removed %s", val)
		}
		for key, details := range prevConfig.EmailProtocols {
			// Handle array of email protocol details
			var emails []string
			for _, detail := range details {
				if detail.Email != "" {
					emails = append(emails, detail.Email)
				}
			}
			if len(emails) > 0 {
				changes[fmt.Sprintf("inet_%s", key)] = fmt.Sprintf("Removed %s", strings.Join(emails, ", "))
			} else {
				changes[fmt.Sprintf("inet_%s", key)] = "Removed"
			}
		}
		return changes
	}

	// Both configs exist and are non-empty, compare them
	if prevIsEmpty && currIsEmpty {
		return changes
	}

	// Compare protocols (now arrays)
	for proto, currDetails := range currConfig.Protocols {
		prevDetails, existed := prevConfig.Protocols[proto]
		if !existed {
			formatted := formatProtocolDetails(currDetails)
			if formatted != "" {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Added %s", formatted)
			} else {
				changes[fmt.Sprintf("inet_%s", proto)] = "Added"
			}
		} else if !compareProtocolArrays(prevDetails, currDetails) {
			// Format old and new values
			oldStr := formatProtocolDetails(prevDetails)
			newStr := formatProtocolDetails(currDetails)
			changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("%s → %s", oldStr, newStr)
		}
	}

	// Check for removed protocols
	for proto, prevDetails := range prevConfig.Protocols {
		if _, exists := currConfig.Protocols[proto]; !exists {
			formatted := formatProtocolDetails(prevDetails)
			if formatted != "" {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Removed %s", formatted)
			} else {
				changes[fmt.Sprintf("inet_%s", proto)] = "Removed"
			}
		}
	}

	// Compare defaults
	for key, currVal := range currConfig.Defaults {
		prevVal, existed := prevConfig.Defaults[key]
		if !existed {
			changes[fmt.Sprintf("inet_%s", key)] = fmt.Sprintf("Added %s", currVal)
		} else if prevVal != currVal {
			changes[fmt.Sprintf("inet_%s", key)] = fmt.Sprintf("%s → %s", prevVal, currVal)
		}
	}

	// Check for removed defaults
	for key, prevVal := range prevConfig.Defaults {
		if _, exists := currConfig.Defaults[key]; !exists {
			changes[fmt.Sprintf("inet_%s", key)] = fmt.Sprintf("Removed %s", prevVal)
		}
	}

	// Compare email protocols (now arrays)
	for proto, currDetails := range currConfig.EmailProtocols {
		prevDetails, existed := prevConfig.EmailProtocols[proto]
		if !existed {
			formatted := formatEmailProtocolDetails(currDetails)
			if formatted != "" {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Added %s", formatted)
			} else {
				changes[fmt.Sprintf("inet_%s", proto)] = "Added (uses default email)"
			}
		} else if !compareEmailArrays(prevDetails, currDetails) {
			oldStr := formatEmailProtocolDetails(prevDetails)
			newStr := formatEmailProtocolDetails(currDetails)
			if newStr != "" {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("%s → %s", oldStr, newStr)
			}
		}
	}

	// Check for removed email protocols
	for proto, prevDetails := range prevConfig.EmailProtocols {
		if _, exists := currConfig.EmailProtocols[proto]; !exists {
			formatted := formatEmailProtocolDetails(prevDetails)
			if formatted != "" {
				changes[fmt.Sprintf("inet_%s", proto)] = fmt.Sprintf("Removed %s", formatted)
			} else {
				changes[fmt.Sprintf("inet_%s", proto)] = "Removed"
			}
		}
	}

	// Compare info flags
	prevFlags := make(map[string]bool)
	currFlags := make(map[string]bool)

	for _, flag := range prevConfig.InfoFlags {
		prevFlags[flag] = true
	}
	for _, flag := range currConfig.InfoFlags {
		currFlags[flag] = true
	}

	for flag := range currFlags {
		if !prevFlags[flag] {
			changes[fmt.Sprintf("inet_flag_%s", flag)] = "Added"
		}
	}
	for flag := range prevFlags {
		if !currFlags[flag] {
			changes[fmt.Sprintf("inet_flag_%s", flag)] = "Removed"
		}
	}

	return changes
}

// parseInternetConfig unmarshals JSON into InternetConfiguration struct
func (so *SearchOperations) parseInternetConfig(data json.RawMessage) (*database.InternetConfiguration, error) {
	if len(data) == 0 {
		return nil, nil
	}

	// First try to unmarshal into a flexible structure that handles both string and int ports
	var rawConfig struct {
		Protocols      map[string]json.RawMessage `json:"protocols,omitempty"`
		Defaults       map[string]string          `json:"defaults,omitempty"`
		EmailProtocols map[string]json.RawMessage `json:"email_protocols,omitempty"`
		InfoFlags      []string                   `json:"info_flags,omitempty"`
	}

	if err := json.Unmarshal(data, &rawConfig); err != nil {
		// If parsing fails, return nil config (treated as empty)
		return nil, nil
	}

	// Convert to the proper InternetConfiguration structure
	config := &database.InternetConfiguration{
		Protocols:      make(map[string][]database.InternetProtocolDetail),
		Defaults:       rawConfig.Defaults,
		EmailProtocols: make(map[string][]database.EmailProtocolDetail),
		InfoFlags:      rawConfig.InfoFlags,
	}

	// Parse protocols with flexible port handling (now supports arrays)
	for proto, rawDetail := range rawConfig.Protocols {
		// Try to parse as array first
		var detailsArray []database.InternetProtocolDetail
		if err := json.Unmarshal(rawDetail, &detailsArray); err == nil {
			config.Protocols[proto] = detailsArray
		} else {
			// Try parsing as single object (backward compatibility)
			var flexDetail struct {
				Address interface{} `json:"address,omitempty"`
				Port    interface{} `json:"port,omitempty"`
			}
			if err := json.Unmarshal(rawDetail, &flexDetail); err != nil {
				continue
			}

			detail := database.InternetProtocolDetail{}

			// Handle address
			switch v := flexDetail.Address.(type) {
			case string:
				detail.Address = v
			}

			// Handle port (can be string or number)
			switch v := flexDetail.Port.(type) {
			case float64:
				detail.Port = int(v)
			case string:
				// Try to parse string as int
				if portNum, err := strconv.Atoi(v); err == nil {
					detail.Port = portNum
				}
			}

			// Store as single-element array for consistency
			config.Protocols[proto] = []database.InternetProtocolDetail{detail}
		}
	}

	// Parse email protocols (now supports arrays)
	for proto, rawDetail := range rawConfig.EmailProtocols {
		// Try to parse as array first
		var detailsArray []database.EmailProtocolDetail
		if err := json.Unmarshal(rawDetail, &detailsArray); err == nil {
			config.EmailProtocols[proto] = detailsArray
		} else {
			// Try parsing as single object (backward compatibility)
			var detail database.EmailProtocolDetail
			if err := json.Unmarshal(rawDetail, &detail); err != nil {
				continue
			}
			// Store as single-element array for consistency
			config.EmailProtocols[proto] = []database.EmailProtocolDetail{detail}
		}
	}

	return config, nil
}
