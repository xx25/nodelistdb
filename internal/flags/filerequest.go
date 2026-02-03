package flags

// FileRequestCapabilities represents what file/update request types a node supports
// based on its file request flag (XA, XB, XC, XP, XR, XW, XX).
//
// FidoNet File Request Flags (from FTS-5001):
//
//	+--------------------------------------------------+
//	|      |         Bark        |        WaZOO        |
//	|      |---------------------|---------------------|
//	|      |   File   |  Update  |   File   |  Update  |
//	| Flag | Requests | Requests | Requests | Requests |
//	|------|----------|----------|----------|----------|
//	| XA   |    Yes   |    Yes   |    Yes   |    Yes   |
//	| XB   |    Yes   |    Yes   |    Yes   |    No    |
//	| XC   |    Yes   |    No    |    Yes   |    Yes   |
//	| XP   |    Yes   |    Yes   |    No    |    No    |
//	| XR   |    Yes   |    No    |    Yes   |    No    |
//	| XW   |    No    |    No    |    Yes   |    No    |
//	| XX   |    No    |    No    |    Yes   |    Yes   |
//	| none |    No    |    No    |    No    |    No    |
//	+--------------------------------------------------+
type FileRequestCapabilities struct {
	Flag               string // The source flag (XA, XB, etc.) or empty if none
	BarkFileRequest    bool   // Supports Bark file requests
	BarkUpdateRequest  bool   // Supports Bark update requests
	WaZOOFileRequest   bool   // Supports WaZOO file requests
	WaZOOUpdateRequest bool   // Supports WaZOO update requests
}

// HasAnyCapability returns true if the node supports any file/update requests
func (c FileRequestCapabilities) HasAnyCapability() bool {
	return c.BarkFileRequest || c.BarkUpdateRequest || c.WaZOOFileRequest || c.WaZOOUpdateRequest
}

// HasBarkSupport returns true if the node supports any Bark requests
func (c FileRequestCapabilities) HasBarkSupport() bool {
	return c.BarkFileRequest || c.BarkUpdateRequest
}

// HasWaZOOSupport returns true if the node supports any WaZOO requests
func (c FileRequestCapabilities) HasWaZOOSupport() bool {
	return c.WaZOOFileRequest || c.WaZOOUpdateRequest
}

// HasFullSupport returns true if the node supports all request types (XA flag)
func (c FileRequestCapabilities) HasFullSupport() bool {
	return c.BarkFileRequest && c.BarkUpdateRequest && c.WaZOOFileRequest && c.WaZOOUpdateRequest
}

// fileRequestFlags maps file request flag names to their capabilities
var fileRequestFlags = map[string]FileRequestCapabilities{
	"XA": {
		Flag:               "XA",
		BarkFileRequest:    true,
		BarkUpdateRequest:  true,
		WaZOOFileRequest:   true,
		WaZOOUpdateRequest: true,
	},
	"XB": {
		Flag:               "XB",
		BarkFileRequest:    true,
		BarkUpdateRequest:  true,
		WaZOOFileRequest:   true,
		WaZOOUpdateRequest: false,
	},
	"XC": {
		Flag:               "XC",
		BarkFileRequest:    true,
		BarkUpdateRequest:  false,
		WaZOOFileRequest:   true,
		WaZOOUpdateRequest: true,
	},
	"XP": {
		Flag:               "XP",
		BarkFileRequest:    true,
		BarkUpdateRequest:  true,
		WaZOOFileRequest:   false,
		WaZOOUpdateRequest: false,
	},
	"XR": {
		Flag:               "XR",
		BarkFileRequest:    true,
		BarkUpdateRequest:  false,
		WaZOOFileRequest:   true,
		WaZOOUpdateRequest: false,
	},
	"XW": {
		Flag:               "XW",
		BarkFileRequest:    false,
		BarkUpdateRequest:  false,
		WaZOOFileRequest:   true,
		WaZOOUpdateRequest: false,
	},
	"XX": {
		Flag:               "XX",
		BarkFileRequest:    false,
		BarkUpdateRequest:  false,
		WaZOOFileRequest:   true,
		WaZOOUpdateRequest: true,
	},
}

// FileRequestFlagList contains all valid file request flags
var FileRequestFlagList = []string{"XA", "XB", "XC", "XP", "XR", "XW", "XX"}

// IsFileRequestFlag returns true if the given flag is a file request flag
func IsFileRequestFlag(flag string) bool {
	_, ok := fileRequestFlags[flag]
	return ok
}

// GetFileRequestCapabilities returns the capabilities for a specific file request flag.
// Returns the capabilities and true if the flag is valid, or empty capabilities and false otherwise.
func GetFileRequestCapabilities(flag string) (FileRequestCapabilities, bool) {
	caps, ok := fileRequestFlags[flag]
	return caps, ok
}

// GetFileRequestCapabilitiesFromFlags scans a slice of flags and returns the file request
// capabilities based on the first file request flag found (XA, XB, XC, XP, XR, XW, XX).
// If no file request flag is found, returns empty capabilities (no support).
func GetFileRequestCapabilitiesFromFlags(flags []string) FileRequestCapabilities {
	for _, flag := range flags {
		if caps, ok := fileRequestFlags[flag]; ok {
			return caps
		}
	}
	// No file request flag found - no capabilities
	return FileRequestCapabilities{}
}

// GetFileRequestDescription returns a human-readable description of a file request flag.
// Derives from GetFlagDescriptions to avoid description drift.
func GetFileRequestDescription(flag string) string {
	if !IsFileRequestFlag(flag) {
		return ""
	}
	descriptions := GetFlagDescriptions()
	if info, ok := descriptions[flag]; ok {
		return info.Description
	}
	return ""
}

// GetSoftwareForFlag returns a list of software packages known to use a specific flag.
// Returns empty slice (not nil) for unknown flags to ensure safe JSON encoding.
func GetSoftwareForFlag(flag string) []string {
	switch flag {
	case "XA":
		return []string{
			"Frontdoor 1.99b and lower",
			"Frontdoor 2.01 and higher",
			"Dutchie 2.90c",
			"Binkleyterm 2.1 and higher",
			"D'Bridge 1.2 and lower",
			"Melmail",
			"TIMS",
			"ifcico",
			"mbcico 0.60.0+ (via modem)",
		}
	case "XB":
		return []string{
			"Binkleyterm 2.0",
			"Dutchie 2.90b",
		}
	case "XC":
		return []string{
			"Opus 1.1",
		}
	case "XP":
		return []string{
			"Seadog",
		}
	case "XR":
		return []string{
			"Opus 1.03",
			"Platinum Xpress",
		}
	case "XW":
		return []string{
			"Fido 12N and higher",
			"Tabby",
			"TrapDoor (no update processor)",
			"binkd w/SRIF FREQ processor",
		}
	case "XX":
		return []string{
			"Argus 2.00 and higher",
			"D'Bridge 1.30 and higher",
			"Frontdoor 1.99c/2.00",
			"InterMail 2.01",
			"McMail 1.00",
			"T-Mail",
			"TrapDoor (with update processor)",
			"mbcico 0.60.0+ (via IP)",
		}
	default:
		return []string{}
	}
}
