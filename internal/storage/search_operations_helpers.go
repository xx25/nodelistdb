package storage

import (
	"fmt"
	"strings"

	"github.com/nodelistdb/internal/database"
)

// formatProtocolDetails formats an array of InternetProtocolDetail for display
func formatProtocolDetails(details []database.InternetProtocolDetail) string {
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
	return strings.Join(addresses, ", ")
}

// formatEmailProtocolDetails formats an array of EmailProtocolDetail for display
func formatEmailProtocolDetails(details []database.EmailProtocolDetail) string {
	var emails []string
	for _, detail := range details {
		if detail.Email != "" {
			emails = append(emails, detail.Email)
		}
	}
	return strings.Join(emails, ", ")
}

// compareProtocolArrays checks if two protocol detail arrays are equal
func compareProtocolArrays(a, b []database.InternetProtocolDetail) bool {
	if len(a) != len(b) {
		return false
	}
	// Simple comparison - format both and compare strings
	return formatProtocolDetails(a) == formatProtocolDetails(b)
}

// compareEmailArrays checks if two email protocol detail arrays are equal
func compareEmailArrays(a, b []database.EmailProtocolDetail) bool {
	if len(a) != len(b) {
		return false
	}
	// Simple comparison - format both and compare strings
	return formatEmailProtocolDetails(a) == formatEmailProtocolDetails(b)
}