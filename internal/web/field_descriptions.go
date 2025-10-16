package web

import (
	"strings"

	"github.com/nodelistdb/internal/flags"
)

// FieldDescription contains display information for a database field
type FieldDescription struct {
	Icon        string
	Description string
}

// GetFieldDescriptions returns descriptions for all database fields
func GetFieldDescriptions() map[string]FieldDescription {
	return map[string]FieldDescription{
		// Node identification fields
		"status":   {Icon: "📊", Description: "Status"},
		"name":     {Icon: "💻", Description: "System Name"},
		"location": {Icon: "🌍", Description: "Location"},
		"sysop":    {Icon: "👤", Description: "Sysop"},
		"phone":    {Icon: "📞", Description: "Phone"},
		"speed":    {Icon: "⚡", Description: "Speed"},
		"flags":    {Icon: "🏷️", Description: "Flags"},
		
		// Boolean connectivity fields
		"binkp":    {Icon: "🌐", Description: "Binkp Support"},
		"telnet":   {Icon: "📡", Description: "Telnet Support"},
		"has_inet": {Icon: "🌍", Description: "Internet Connectivity"},
		
		// Modem and protocol fields
		"modem_flags": {Icon: "📞", Description: "Modem Flags"},
		
		// Legacy internet fields
		"internet_protocols": {Icon: "🌐", Description: "Internet Protocols (Legacy)"},
		"internet_hostnames": {Icon: "🏠", Description: "Internet Hostnames (Legacy)"},
		"internet_ports":     {Icon: "🔌", Description: "Internet Ports (Legacy)"},
		"internet_emails":    {Icon: "📧", Description: "Internet Emails (Legacy)"},
		
		// Modern internet configuration
		"internet_config": {Icon: "🌐", Description: "Internet Configuration"},
	}
}

// GetFieldDescriptionWithFlag returns the complete description for any field including inet_* fields
func GetFieldDescriptionWithFlag(field string) FieldDescription {
	// Check static field descriptions first
	staticDescs := GetFieldDescriptions()
	if desc, exists := staticDescs[field]; exists {
		return desc
	}
	
	// Check if it's an inet_* field
	if strings.HasPrefix(field, "inet_") {
		// Extract flag name from field name (inet_IBN -> IBN, inet_flag_ICM -> ICM)
		flagName := strings.TrimPrefix(field, "inet_")
		flagName = strings.TrimPrefix(flagName, "flag_")
		
		// Get flag description
		flagDescs := flags.GetFlagDescriptions()
		if flagInfo, exists := flagDescs[flagName]; exists {
			// Map flags to appropriate icons
			icon := "🌐" // Default icon
			switch flagName {
			case "IBN":
				icon = "🌐" // BinkP
			case "IFC":
				icon = "📁" // EMSI over TCP
			case "ITN":
				icon = "📡" // Telnet
			case "IVM":
				icon = "📞" // VModem
			case "IFT":
				icon = "📁" // FTP
			case "INA":
				icon = "🏠" // Internet Address
			case "IEM":
				icon = "📧" // Email
			case "IMI":
				icon = "📧" // Internet Mail Interface
			case "ITX":
				icon = "📧" // TransX
			case "ISE":
				icon = "📧" // SendEmail
			case "IUC":
				icon = "📧" // UUencoded
			case "INO4":
				icon = "🚫" // No IPv4
			case "INO6":
				icon = "🚫" // No IPv6
			case "ICM":
				icon = "📞" // Internet CM
			}
			
			// Include flag name in parentheses for clarity
			return FieldDescription{
				Icon:        icon,
				Description: flagInfo.Description + " (" + flagName + ")",
			}
		}
	}
	
	// Default: return field name as-is
	return FieldDescription{
		Icon:        "",
		Description: field,
	}
}

// GetFieldIcon returns just the icon for a field
func GetFieldIcon(field string) string {
	desc := GetFieldDescriptionWithFlag(field)
	return desc.Icon
}

// GetFieldDescription returns just the description for a field
func GetFieldDescription(field string) string {
	desc := GetFieldDescriptionWithFlag(field)
	return desc.Description
}