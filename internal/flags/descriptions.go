package flags

// FlagInfo contains metadata about flag types
type FlagInfo struct {
	Category    string `json:"category"`    // modem, internet, capability, schedule, user
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
		"V21":  {Category: "modem", HasValue: false, Description: "ITU-T V.21 (300 bps, full-duplex, FSK modulation)"},
		"V22":  {Category: "modem", HasValue: false, Description: "ITU-T V.22 (1200 bps, full-duplex, QAM modulation)"},
		"V29":  {Category: "modem", HasValue: false, Description: "ITU-T V.29 (9600 bps, half-duplex, used for fax and data)"},
		"V32":  {Category: "modem", HasValue: false, Description: "ITU-T V.32 (9600 bps, full-duplex, QAM modulation)"},
		"V32B": {Category: "modem", HasValue: false, Description: "ITU-T V.32bis (14400 bps, full-duplex, QAM modulation)"},
		"V33":  {Category: "modem", HasValue: false, Description: "ITU-T V.33 (14400 bps, half-duplex, data/fax transmission)"},
		"V34":  {Category: "modem", HasValue: false, Description: "ITU-T V.34 (up to 28800 bps, full-duplex, advanced QAM modulation)"},
		"V42":  {Category: "modem", HasValue: false, Description: "ITU-T V.42 (LAPM error correction protocol)"},
		"V42B": {Category: "modem", HasValue: false, Description: "ITU-T V.42bis (data compression, up to 4:1 ratio)"},
		"V90C": {Category: "modem", HasValue: false, Description: "ITU-T V.90 (56 kbps, client side, analog download)"},
		"V90S": {Category: "modem", HasValue: false, Description: "ITU-T V.90 (56 kbps, server side, digital upload)"},
		"X75":  {Category: "modem", HasValue: false, Description: "ITU-T X.75 (ISDN B-channel protocol, 64 kbps)"},
		"HST":  {Category: "modem", HasValue: false, Description: "USRobotics HST (High-Speed Transfer, proprietary, 9600-14400 bps)"},
		"H96":  {Category: "modem", HasValue: false, Description: "USRobotics HST 9600 (early HST modem, 9600 bps)"},
		"H14":  {Category: "modem", HasValue: false, Description: "USRobotics HST 14400 (improved speed variant, 14400 bps)"},
		"H16":  {Category: "modem", HasValue: false, Description: "USRobotics HST 16800 (advanced speed variant, 16800 bps)"},
		"MAX":  {Category: "modem", HasValue: false, Description: "Microcom AX/96xx series (proprietary modulation, 9600 bps+)"},
		"PEP":  {Category: "modem", HasValue: false, Description: "Packet Ensemble Protocol (proprietary error correction and modulation)"},
		"CSP":  {Category: "modem", HasValue: false, Description: "Compucom SpeedModem (CSP, proprietary protocol)"},
		"ZYX":  {Category: "modem", HasValue: false, Description: "ZyXEL modem (supports proprietary and standard protocols)"},
		"VFC":  {Category: "modem", HasValue: false, Description: "V.Fast Class (V.FC, pre-V.34 28800 bps, Rockwell standard)"},

		// Internet flags
		"IBN": {Category: "internet", HasValue: true, Description: "BinkP"},
		"IFC": {Category: "internet", HasValue: true, Description: "File transfer"},
		"ITN": {Category: "internet", HasValue: true, Description: "Telnet"},
		"IVM": {Category: "internet", HasValue: true, Description: "VModem"},
		"IFT": {Category: "internet", HasValue: true, Description: "FTP"},
		"INA": {Category: "internet", HasValue: true, Description: "Internet address"},
		"IP":  {Category: "internet", HasValue: true, Description: "General IP"},

		// Email protocols
		"IEM": {Category: "internet", HasValue: true, Description: "Email"},
		"IMI": {Category: "internet", HasValue: true, Description: "Mail interface"},
		"ITX": {Category: "internet", HasValue: true, Description: "TransX"},
		"IUC": {Category: "internet", HasValue: true, Description: "UUencoded"},
		"ISE": {Category: "internet", HasValue: true, Description: "SendEmail"},

		// Capability flags
		"CM": {Category: "capability", HasValue: false, Description: "Continuous Mail"},
		"MO": {Category: "capability", HasValue: false, Description: "Mail Only"},
		"LO": {Category: "capability", HasValue: false, Description: "Local Only"},
		"XA": {Category: "capability", HasValue: false, Description: "Extended addressing"},
		"XB": {Category: "capability", HasValue: false, Description: "Bark requests"},
		"XC": {Category: "capability", HasValue: false, Description: "Compressed mail"},
		"XP": {Category: "capability", HasValue: false, Description: "Extended protocol"},
		"XR": {Category: "capability", HasValue: false, Description: "Accepts file requests"},
		"XW": {Category: "capability", HasValue: false, Description: "X.75 windowing"},
		"XX": {Category: "capability", HasValue: false, Description: "No file/update requests"},

		// Schedule flags
		"U": {Category: "schedule", HasValue: true, Description: "Availability"},
		"T": {Category: "schedule", HasValue: true, Description: "Time zone"},

		// User flags
		"ENC":   {Category: "user", HasValue: false, Description: "Encrypted"},
		"NC":    {Category: "user", HasValue: false, Description: "Network Coordinator"},
		"NEC":   {Category: "user", HasValue: false, Description: "Net Echomail Coordinator"},
		"REC":   {Category: "user", HasValue: false, Description: "Region Echomail Coordinator"},
		"ZEC":   {Category: "user", HasValue: false, Description: "Zone Echomail Coordinator"},
		"PING":  {Category: "user", HasValue: false, Description: "Ping OK"},
		"TRACE": {Category: "user", HasValue: false, Description: "Network trace capability - notifies sender when PING messages pass through this node"},
		"RPK":   {Category: "user", HasValue: false, Description: "Regional Pointlist Keeper"},
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
