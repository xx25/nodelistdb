package flags

// FlagInfo contains metadata about flag types
type FlagInfo struct {
	Category    string `json:"category"`    // modem, internet, capability, filerequest, schedule, user
	HasValue    bool   `json:"has_value"`   // whether flag takes a parameter
	Description string `json:"description"` // human-readable description
}

// timeLetterToUTC maps time letters to UTC times according to FTS-5001
var timeLetterToUTC = map[byte]string{
	'A': "00:00", 'a': "00:30",
	'B': "01:00", 'b': "01:30",
	'C': "02:00", 'c': "02:30",
	'D': "03:00", 'd': "03:30",
	'E': "04:00", 'e': "04:30",
	'F': "05:00", 'f': "05:30",
	'G': "06:00", 'g': "06:30",
	'H': "07:00", 'h': "07:30",
	'I': "08:00", 'i': "08:30",
	'J': "09:00", 'j': "09:30",
	'K': "10:00", 'k': "10:30",
	'L': "11:00", 'l': "11:30",
	'M': "12:00", 'm': "12:30",
	'N': "13:00", 'n': "13:30",
	'O': "14:00", 'o': "14:30",
	'P': "15:00", 'p': "15:30",
	'Q': "16:00", 'q': "16:30",
	'R': "17:00", 'r': "17:30",
	'S': "18:00", 's': "18:30",
	'T': "19:00", 't': "19:30",
	'U': "20:00", 'u': "20:30",
	'V': "21:00", 'v': "21:30",
	'W': "22:00", 'w': "22:30",
	'X': "23:00", 'x': "23:30",
	// Special cases found in nodelists
	'Y': "23:59", 'y': "23:59", // End of day
	'Z': "23:59", 'z': "23:59", // Also end of day
}

// GetFlagDescriptions returns the complete flag documentation map
// with dynamic generation of T-prefixed time availability flags
func GetFlagDescriptions() map[string]FlagInfo {
	return map[string]FlagInfo{
		// Modem flags
		"V21":  {Category: "modem", HasValue: false, Description: "ITU-T V.21 (300 bps, full-duplex)"},
		"V22":  {Category: "modem", HasValue: false, Description: "ITU-T V.22 (1200 bps, full-duplex)"},
		"V23":  {Category: "modem", HasValue: false, Description: "CCITT V.23 (1200/75 bps, full-duplex)"},
		"V29":  {Category: "modem", HasValue: false, Description: "ITU-T V.29 (9600 bps, half-duplex)"},
		"V32":  {Category: "modem", HasValue: false, Description: "ITU-T V.32 (9600 bps, full-duplex)"},
		"V32B": {Category: "modem", HasValue: false, Description: "ITU-T V.32bis (14400 bps, full-duplex)"},
		"V32T": {Category: "modem", HasValue: false, Description: "V.32 Terbo mode"},
		"V33":  {Category: "modem", HasValue: false, Description: "ITU-T V.33"},
		"V34":  {Category: "modem", HasValue: false, Description: "ITU-T V.34 (28800 bps, full-duplex)"},
		"V42":  {Category: "modem", HasValue: false, Description: "LAP-M error correction w/fallback to MNP 1-4"},
		"V42B": {Category: "modem", HasValue: false, Description: "LAP-M error correction w/fallback to MNP 1-5"},
		"V90C": {Category: "modem", HasValue: false, Description: "ITU-T V.90 (56 kbps, client side, analog download)"},
		"V90S": {Category: "modem", HasValue: false, Description: "ITU-T V.90 (56 kbps, server side, digital upload)"},
		"X2C":  {Category: "modem", HasValue: false, Description: "US Robotics x2 client (56 kbit/s from X2S to X2C, V.34-type reverse)"},
		"X2S":  {Category: "modem", HasValue: false, Description: "US Robotics x2 server (digital interface, 64kbit/s X2S-X2S or 56kbit/s to X2C)"},
		"Z19":  {Category: "modem", HasValue: false, Description: "Zyxel 19.2 Kbps (implies V.32Bis & V.42Bis & ZYX)"},
		"X75":  {Category: "modem", HasValue: false, Description: "ITU-T X.75 (ISDN B-channel protocol, 64 kbps)"},
		"HST":  {Category: "modem", HasValue: false, Description: "USR Courier HST"},
		"H96":  {Category: "modem", HasValue: false, Description: "Hayes V9600"},
		"H14":  {Category: "modem", HasValue: false, Description: "USR Courier HST up to 14.4Kbps"},
		"H16":  {Category: "modem", HasValue: false, Description: "USR Courier HST up to 16.8Kbps"},
		"MAX":  {Category: "modem", HasValue: false, Description: "Microcom AX/96xx series"},
		"PEP":  {Category: "modem", HasValue: false, Description: "Packet Ensemble Protocol"},
		"CSP":  {Category: "modem", HasValue: false, Description: "Compucom Speedmodem"},
		"ZYX":  {Category: "modem", HasValue: false, Description: "Zyxel (implies V.32Bis & V.42Bis)"},
		"VFC":  {Category: "modem", HasValue: false, Description: "Rockwell's V.Fast Class"},
		"MNP":  {Category: "modem", HasValue: false, Description: "Microcom Networking Protocol error correction"},

		// Internet flags
		"IBN": {Category: "internet", HasValue: true, Description: "BinkP"},
		"IFC": {Category: "internet", HasValue: true, Description: "EMSI over TCP"},
		"ITN": {Category: "internet", HasValue: true, Description: "Telnet"},
		"IVM": {Category: "internet", HasValue: true, Description: "VModem"},
		"IFT": {Category: "internet", HasValue: true, Description: "FTP"},
		"INA": {Category: "internet", HasValue: true, Description: "Internet address"},
		"IP":  {Category: "internet", HasValue: true, Description: "General IP"},

		// Email protocols
		"IEM": {Category: "internet", HasValue: true, Description: "Email"},
		"IMI": {Category: "internet", HasValue: true, Description: "Internet Mail Interface"},
		"ITX": {Category: "internet", HasValue: true, Description: "TransX"},
		"IUC": {Category: "internet", HasValue: true, Description: "UUencoded"},
		"ISE": {Category: "internet", HasValue: true, Description: "SendEmail"},
		
		// Internet information flags
		"ICM":  {Category: "internet", HasValue: false, Description: "Internet CM"},
		"INO4": {Category: "internet", HasValue: false, Description: "No IPv4 incoming connections (FTS-1038)"},

		// Capability flags
		"CM": {Category: "capability", HasValue: false, Description: "Continuous Mail"},
		"MO": {Category: "capability", HasValue: false, Description: "Mail Only"},
		"LO": {Category: "capability", HasValue: false, Description: "Local Only"},
		"MN": {Category: "capability", HasValue: false, Description: "No compression supported"},

		// File/Update Request Flags (FTS-5001 Section 5.4)
		// These indicate file request capabilities via Bark and WaZOO protocols
		"XA": {Category: "filerequest", HasValue: false, Description: "Supports Bark and WaZOO file/update requests"},
		"XB": {Category: "filerequest", HasValue: false, Description: "Supports Bark file/update and WaZOO file requests"},
		"XC": {Category: "filerequest", HasValue: false, Description: "Supports Bark file and WaZOO file/update requests"},
		"XP": {Category: "filerequest", HasValue: false, Description: "Supports Bark file/update requests only"},
		"XR": {Category: "filerequest", HasValue: false, Description: "Supports Bark file and WaZOO file requests"},
		"XW": {Category: "filerequest", HasValue: false, Description: "Supports WaZOO file requests only"},
		"XX": {Category: "filerequest", HasValue: false, Description: "Supports WaZOO file/update requests only"},

		// Schedule flags
		"U":  {Category: "schedule", HasValue: true, Description: "Availability"},
		"T":  {Category: "schedule", HasValue: true, Description: "Time zone"},
		"DA": {Category: "schedule", HasValue: true, Description: "Daily hours of operation"},
		"WK": {Category: "schedule", HasValue: true, Description: "Week days hours of operation"},
		"WE": {Category: "schedule", HasValue: true, Description: "Week ends hours of operation"},
		"SU": {Category: "schedule", HasValue: true, Description: "Sundays hours of operation"},
		"SA": {Category: "schedule", HasValue: true, Description: "Saturdays hours of operation"},

		// User flags
		"ENC":   {Category: "user", HasValue: false, Description: "Encrypted"},
		"NC":    {Category: "user", HasValue: false, Description: "Network Coordinator"},
		"NEC":   {Category: "user", HasValue: false, Description: "Net Echomail Coordinator"},
		"REC":   {Category: "user", HasValue: false, Description: "Region Echomail Coordinator"},
		"ZEC":   {Category: "user", HasValue: false, Description: "Zone Echomail Coordinator"},
		"PING":  {Category: "user", HasValue: false, Description: "Ping OK"},
		"TRACE": {Category: "user", HasValue: false, Description: "Network trace capability - notifies sender when PING messages pass through this node"},
		"RPK":   {Category: "user", HasValue: false, Description: "Regional Pointlist Keeper"},
		"RE":    {Category: "user", HasValue: false, Description: "Node exercises some access restrictions"},

		// Mail hour flags (dedicated mail periods)
		"#01": {Category: "schedule", HasValue: false, Description: "Zone 5 mail hour (01:00-02:00 UTC)"},
		"#02": {Category: "schedule", HasValue: false, Description: "Zone 2 mail hour (02:30-03:30 UTC)"},
		"#08": {Category: "schedule", HasValue: false, Description: "Zone 4 mail hour (08:00-09:00 UTC)"},
		"#09": {Category: "schedule", HasValue: false, Description: "Zone 1 mail hour (09:00-10:00 UTC)"},
		"#18": {Category: "schedule", HasValue: false, Description: "Zone 3 mail hour (18:00-19:00 UTC)"},
		"#20": {Category: "schedule", HasValue: false, Description: "Zone 6 mail hour (20:00-21:00 UTC)"},
	}
}

// GetTFlagInfo generates FlagInfo for T-prefixed time availability flags
func GetTFlagInfo(flag string) (FlagInfo, bool) {
	if len(flag) != 3 || flag[0] != 'T' {
		return FlagInfo{}, false
	}

	startTime, startOk := timeLetterToUTC[flag[1]]
	endTime, endOk := timeLetterToUTC[flag[2]]

	if !startOk || !endOk {
		return FlagInfo{}, false
	}

	return FlagInfo{
		Category:    "schedule",
		HasValue:    false,
		Description: "Available " + startTime + "-" + endTime + " UTC",
	}, true
}

// GetParserFlagMap returns a simplified map for parser use (without descriptions)
func GetParserFlagMap() map[string]ParserFlagInfo {
	descriptions := GetFlagDescriptions()
	parserMap := make(map[string]ParserFlagInfo)

	for flag, info := range descriptions {
		parserMap[flag] = ParserFlagInfo{
			Category: info.Category,
			HasValue: info.HasValue,
		}
	}

	return parserMap
}

// ParserFlagInfo is the simplified version used by the parser
type ParserFlagInfo struct {
	Category string
	HasValue bool
}
