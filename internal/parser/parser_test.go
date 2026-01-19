package parser

import (
	"errors"
	"testing"
	"time"

	"github.com/nodelistdb/internal/database"
)

func TestSanitizeUTF8(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "valid UTF-8 string",
			input:    "Hello, World!",
			expected: "Hello, World!",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "UTF-8 with special characters",
			input:    "Café résumé",
			expected: "Café résumé",
		},
		{
			name:     "UTF-8 with emoji",
			input:    "Hello 👋 World",
			expected: "Hello 👋 World",
		},
		{
			name:     "cyrillic characters",
			input:    "Привет мир",
			expected: "Привет мир",
		},
		{
			name:     "invalid UTF-8 sequence",
			input:    "Hello\x80World",
			expected: "Hello?World",
		},
		{
			name:     "multiple invalid sequences",
			input:    "\x80\x81\x82",
			expected: "???",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeUTF8(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeUTF8(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		firstLine string
		expected  NodelistFormat
	}{
		{
			name:      "modern format with IBN",
			line:      "Hub,1,Test_Hub,City,User,123-4567,9600,CM,IBN:24554",
			firstLine: "",
			expected:  Format2020,
		},
		{
			name:      "modern format with ITN",
			line:      "Hub,1,Test_Hub,City,User,123-4567,9600,CM,ITN",
			firstLine: "",
			expected:  Format2020,
		},
		{
			name:      "modern format with INA",
			line:      "Hub,1,Test_Hub,City,User,123-4567,9600,CM,INA:host.example.com",
			firstLine: "",
			expected:  Format2020,
		},
		{
			name:      "2000s format with V34",
			line:      "Hub,1,Test_Hub,City,User,123-4567,9600,CM,V34",
			firstLine: "",
			expected:  Format2000,
		},
		{
			name:      "2000s format with V90",
			line:      "Hub,1,Test_Hub,City,User,123-4567,9600,CM,V90",
			firstLine: "",
			expected:  Format2000,
		},
		{
			name:      "2000s format with X75",
			line:      "Hub,1,Test_Hub,City,User,123-4567,9600,CM,X75",
			firstLine: "",
			expected:  Format2000,
		},
		{
			name:      "1990s format with XA",
			line:      "Hub,1,Test_Hub,City,User,123-4567,9600,CM,XA",
			firstLine: "",
			expected:  Format1990,
		},
		{
			name:      "1990s format with CM",
			line:      "Hub,1,Test_Hub,City,User,123-4567,9600,CM",
			firstLine: "",
			expected:  Format1990,
		},
		{
			name:      "1990s format with MO",
			line:      "Hub,1,Test_Hub,City,User,123-4567,9600,MO",
			firstLine: "",
			expected:  Format1990,
		},
		{
			name:      "1986 format with XP colon flag",
			line:      "Hub,1,Test_Hub,City,User,123-4567,9600,XP:",
			firstLine: "",
			expected:  Format1986,
		},
		{
			name:      "default to 1990s format",
			line:      "Hub,1,Test_Hub,City,User,123-4567,9600",
			firstLine: "",
			expected:  Format1990,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectFormat(tt.line, tt.firstLine)
			if result != tt.expected {
				t.Errorf("detectFormat(%q, %q) = %v, want %v", tt.line, tt.firstLine, result, tt.expected)
			}
		})
	}
}

func TestParseError(t *testing.T) {
	t.Run("Error with line number", func(t *testing.T) {
		err := NewParseError("test.txt", 42, "invalid field")
		expected := "parse error at test.txt:42: invalid field"
		if err.Error() != expected {
			t.Errorf("Error() = %q, want %q", err.Error(), expected)
		}
	})

	t.Run("Error without line number", func(t *testing.T) {
		err := NewParseError("test.txt", 0, "file-level error")
		expected := "parse error in test.txt: file-level error"
		if err.Error() != expected {
			t.Errorf("Error() = %q, want %q", err.Error(), expected)
		}
	})

	t.Run("Error with cause", func(t *testing.T) {
		cause := errors.New("underlying error")
		err := NewParseErrorWithCause("test.txt", 10, "wrapper", cause)
		if err.Unwrap() != cause {
			t.Errorf("Unwrap() = %v, want %v", err.Unwrap(), cause)
		}
	})

	t.Run("Field error", func(t *testing.T) {
		err := NewFieldParseError("test.txt", 5, "zone", "abc", "not a number")
		if err.Field != "zone" {
			t.Errorf("Field = %q, want %q", err.Field, "zone")
		}
		if err.Value != "abc" {
			t.Errorf("Value = %q, want %q", err.Value, "abc")
		}
	})
}

func TestFileError(t *testing.T) {
	t.Run("Basic error", func(t *testing.T) {
		cause := errors.New("permission denied")
		err := NewFileError("/path/to/file", "open", "cannot open file", cause)

		expected := "file open error: cannot open file"
		if err.Error() != expected {
			t.Errorf("Error() = %q, want %q", err.Error(), expected)
		}

		if err.Unwrap() != cause {
			t.Errorf("Unwrap() = %v, want %v", err.Unwrap(), cause)
		}

		if err.Path != "/path/to/file" {
			t.Errorf("Path = %q, want %q", err.Path, "/path/to/file")
		}
	})
}

func TestDateError(t *testing.T) {
	t.Run("Basic error", func(t *testing.T) {
		err := NewDateError("filename", "nodelist.abc", "invalid day number")

		expected := "date error from filename: invalid day number"
		if err.Error() != expected {
			t.Errorf("Error() = %q, want %q", err.Error(), expected)
		}

		if err.Source != "filename" {
			t.Errorf("Source = %q, want %q", err.Source, "filename")
		}

		if err.Value != "nodelist.abc" {
			t.Errorf("Value = %q, want %q", err.Value, "nodelist.abc")
		}
	})
}

func TestConversionError(t *testing.T) {
	t.Run("Basic error", func(t *testing.T) {
		cause := errors.New("strconv error")
		err := NewConversionError("zone", "abc", "int", cause)

		expected := "cannot convert zone='abc' to int"
		if err.Error() != expected {
			t.Errorf("Error() = %q, want %q", err.Error(), expected)
		}

		if err.Unwrap() != cause {
			t.Errorf("Unwrap() = %v, want %v", err.Unwrap(), cause)
		}
	})

	t.Run("WithValue", func(t *testing.T) {
		orig := ErrInvalidZone
		withVal := orig.WithValue("999999")

		if withVal.Value != "999999" {
			t.Errorf("Value = %q, want %q", withVal.Value, "999999")
		}
		if withVal.Field != "zone" {
			t.Errorf("Field = %q, want %q", withVal.Field, "zone")
		}
	})
}

func TestParseInt(t *testing.T) {
	tests := []struct {
		name      string
		field     string
		value     string
		expected  int
		wantError bool
	}{
		{
			name:      "valid positive integer",
			field:     "zone",
			value:     "42",
			expected:  42,
			wantError: false,
		},
		{
			name:      "valid zero",
			field:     "node",
			value:     "0",
			expected:  0,
			wantError: false,
		},
		{
			name:      "valid negative integer",
			field:     "offset",
			value:     "-10",
			expected:  -10,
			wantError: false,
		},
		{
			name:      "empty string",
			field:     "zone",
			value:     "",
			expected:  0,
			wantError: true,
		},
		{
			name:      "non-numeric string",
			field:     "zone",
			value:     "abc",
			expected:  0,
			wantError: true,
		},
		{
			name:      "float value",
			field:     "zone",
			value:     "3.14",
			expected:  0,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseInt(tt.field, tt.value)

			if tt.wantError {
				if err == nil {
					t.Errorf("ParseInt(%q, %q) expected error, got nil", tt.field, tt.value)
				}
				var convErr *ConversionError
				if !errors.As(err, &convErr) {
					t.Errorf("ParseInt(%q, %q) error type = %T, want *ConversionError", tt.field, tt.value, err)
				}
			} else {
				if err != nil {
					t.Errorf("ParseInt(%q, %q) unexpected error: %v", tt.field, tt.value, err)
				}
				if result != tt.expected {
					t.Errorf("ParseInt(%q, %q) = %d, want %d", tt.field, tt.value, result, tt.expected)
				}
			}
		})
	}
}

func TestParseUint32(t *testing.T) {
	tests := []struct {
		name      string
		field     string
		value     string
		expected  uint32
		wantError bool
	}{
		{
			name:      "valid positive integer",
			field:     "speed",
			value:     "9600",
			expected:  9600,
			wantError: false,
		},
		{
			name:      "valid zero",
			field:     "speed",
			value:     "0",
			expected:  0,
			wantError: false,
		},
		{
			name:      "max uint32",
			field:     "speed",
			value:     "4294967295",
			expected:  4294967295,
			wantError: false,
		},
		{
			name:      "empty string",
			field:     "speed",
			value:     "",
			expected:  0,
			wantError: true,
		},
		{
			name:      "negative number",
			field:     "speed",
			value:     "-1",
			expected:  0,
			wantError: true,
		},
		{
			name:      "non-numeric string",
			field:     "speed",
			value:     "fast",
			expected:  0,
			wantError: true,
		},
		{
			name:      "overflow",
			field:     "speed",
			value:     "4294967296",
			expected:  0,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseUint32(tt.field, tt.value)

			if tt.wantError {
				if err == nil {
					t.Errorf("ParseUint32(%q, %q) expected error, got nil", tt.field, tt.value)
				}
				var convErr *ConversionError
				if !errors.As(err, &convErr) {
					t.Errorf("ParseUint32(%q, %q) error type = %T, want *ConversionError", tt.field, tt.value, err)
				}
			} else {
				if err != nil {
					t.Errorf("ParseUint32(%q, %q) unexpected error: %v", tt.field, tt.value, err)
				}
				if result != tt.expected {
					t.Errorf("ParseUint32(%q, %q) = %d, want %d", tt.field, tt.value, result, tt.expected)
				}
			}
		})
	}
}

func TestParserParseMonth(t *testing.T) {
	// Create a parser for testing methods
	p := New(false)

	tests := []struct {
		name     string
		month    string
		expected int
	}{
		{"january full", "January", 1},
		{"january abbrev", "Jan", 1},
		{"february full", "February", 2},
		{"february abbrev", "Feb", 2},
		{"march full", "March", 3},
		{"march abbrev", "Mar", 3},
		{"april full", "April", 4},
		{"april abbrev", "Apr", 4},
		{"may", "May", 5},
		{"june full", "June", 6},
		{"june abbrev", "Jun", 6},
		{"july full", "July", 7},
		{"july abbrev", "Jul", 7},
		{"august full", "August", 8},
		{"august abbrev", "Aug", 8},
		{"september full", "September", 9},
		{"september abbrev", "Sep", 9},
		{"october full", "October", 10},
		{"october abbrev", "Oct", 10},
		{"november full", "November", 11},
		{"november abbrev", "Nov", 11},
		{"december full", "December", 12},
		{"december abbrev", "Dec", 12},
		{"lowercase", "january", 1},
		{"uppercase", "JANUARY", 1},
		{"mixed case", "JaNuArY", 1},
		{"invalid month", "NotAMonth", 0},
		{"empty string", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.parseMonth(tt.month)
			if result != tt.expected {
				t.Errorf("parseMonth(%q) = %d, want %d", tt.month, result, tt.expected)
			}
		})
	}
}

func TestParserExtractYearFromPath(t *testing.T) {
	p := New(false)

	tests := []struct {
		name     string
		path     string
		expected int
	}{
		{"4-digit year in path", "/data/nodelists/1989/nodelist.001", 1989},
		{"4-digit year 2024", "/data/nodelists/2024/nodelist.001", 2024},
		{"no year in path", "/data/nodelists/nodelist.001", 0},
		{"2-digit year 89", "nodelist.89", 1989},
		{"2-digit year 99", "nodelist.99", 1999},
		{"2-digit year 00", "nodelist.00", 2000},
		{"2-digit year 24", "nodelist.24", 2024},
		{"year in filename only", "1995/nodelist.001", 1995},
		{"multiple years takes first", "/1990/2000/nodelist.001", 1990},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.extractYearFromPath(tt.path)
			if result != tt.expected {
				t.Errorf("extractYearFromPath(%q) = %d, want %d", tt.path, result, tt.expected)
			}
		})
	}
}

func TestParseNodeFields(t *testing.T) {
	p := New(false)

	tests := []struct {
		name        string
		line        string
		wantType    string
		wantNum     string
		wantSystem  string
		wantLoc     string
		wantSysop   string
		wantPhone   string
		wantSpeed   uint32
		wantFlags   string
		wantError   bool
	}{
		{
			name:       "standard node line",
			line:       "Hub,100,Test_BBS,New_York_NY,John_Doe,1-212-555-1234,9600,CM,IBN",
			wantType:   "Hub",
			wantNum:    "100",
			wantSystem: "Test_BBS",
			wantLoc:    "New_York_NY",
			wantSysop:  "John_Doe",
			wantPhone:  "1-212-555-1234",
			wantSpeed:  9600,
			wantFlags:  "CM,IBN",
			wantError:  false,
		},
		{
			name:       "node with negative number (legacy format)",
			line:       ",-100,Legacy_BBS,City,Sysop,000-000-000,2400",
			wantType:   "",
			wantNum:    "100",
			wantSystem: "Legacy_BBS",
			wantLoc:    "City",
			wantSysop:  "Sysop",
			wantPhone:  "000-000-000",
			wantSpeed:  2400,
			wantFlags:  "",
			wantError:  false,
		},
		{
			name:       "node without flags",
			line:       ",1,Simple_BBS,Location,Operator,123-456-7890,14400",
			wantType:   "",
			wantNum:    "1",
			wantSystem: "Simple_BBS",
			wantLoc:    "Location",
			wantSysop:  "Operator",
			wantPhone:  "123-456-7890",
			wantSpeed:  14400,
			wantFlags:  "",
			wantError:  false,
		},
		{
			name:       "zone coordinator",
			line:       "Zone,2,Zone2_HQ,Europe,Coordinator,00-000-000,9600,CM",
			wantType:   "Zone",
			wantNum:    "2",
			wantSystem: "Zone2_HQ",
			wantLoc:    "Europe",
			wantSysop:  "Coordinator",
			wantPhone:  "00-000-000",
			wantSpeed:  9600,
			wantFlags:  "CM",
			wantError:  false,
		},
		{
			name:      "insufficient fields",
			line:      "Hub,100,Test",
			wantError: true,
		},
		{
			name:       "whitespace handling",
			line:       " Hub , 100 , Test_BBS , Location , Sysop , Phone , 9600 , CM ",
			wantType:   "Hub",
			wantNum:    "100",
			wantSystem: "Test_BBS",
			wantLoc:    "Location",
			wantSysop:  "Sysop",
			wantPhone:  "Phone",
			wantSpeed:  9600,
			wantFlags:  " CM ", // flags string preserves the joined remainder
			wantError:  false,
		},
		{
			name:       "invalid speed (non-numeric)",
			line:       ",1,BBS,City,Op,Phone,FAST,CM",
			wantType:   "",
			wantNum:    "1",
			wantSystem: "BBS",
			wantLoc:    "City",
			wantSysop:  "Op",
			wantPhone:  "Phone",
			wantSpeed:  0, // Invalid speed results in 0
			wantFlags:  "CM",
			wantError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeType, nodeNum, system, loc, sysop, phone, speed, flags, err := p.parseNodeFields(tt.line)

			if tt.wantError {
				if err == nil {
					t.Errorf("parseNodeFields(%q) expected error, got nil", tt.line)
				}
				return
			}

			if err != nil {
				t.Errorf("parseNodeFields(%q) unexpected error: %v", tt.line, err)
				return
			}

			if nodeType != tt.wantType {
				t.Errorf("nodeType = %q, want %q", nodeType, tt.wantType)
			}
			if nodeNum != tt.wantNum {
				t.Errorf("nodeNum = %q, want %q", nodeNum, tt.wantNum)
			}
			if system != tt.wantSystem {
				t.Errorf("system = %q, want %q", system, tt.wantSystem)
			}
			if loc != tt.wantLoc {
				t.Errorf("location = %q, want %q", loc, tt.wantLoc)
			}
			if sysop != tt.wantSysop {
				t.Errorf("sysop = %q, want %q", sysop, tt.wantSysop)
			}
			if phone != tt.wantPhone {
				t.Errorf("phone = %q, want %q", phone, tt.wantPhone)
			}
			if speed != tt.wantSpeed {
				t.Errorf("speed = %d, want %d", speed, tt.wantSpeed)
			}
			if flags != tt.wantFlags {
				t.Errorf("flags = %q, want %q", flags, tt.wantFlags)
			}
		})
	}
}

func TestParseNodeType(t *testing.T) {
	tests := []struct {
		name       string
		nodeType   string
		nodeNum    string
		initZone   int
		initNet    int
		wantType   string
		wantZone   int
		wantNet    int
		wantNode   int
		wantError  bool
	}{
		{
			name:      "empty type (normal node)",
			nodeType:  "",
			nodeNum:   "100",
			initZone:  1,
			initNet:   5001,
			wantType:  "Node",
			wantZone:  1,
			wantNet:   5001,
			wantNode:  100,
			wantError: false,
		},
		{
			name:      "zone coordinator",
			nodeType:  "Zone",
			nodeNum:   "2",
			initZone:  1,
			initNet:   1,
			wantType:  "Zone",
			wantZone:  2,
			wantNet:   2,
			wantNode:  0,
			wantError: false,
		},
		{
			name:      "region coordinator",
			nodeType:  "Region",
			nodeNum:   "17",
			initZone:  1,
			initNet:   1,
			wantType:  "Region",
			wantZone:  1,
			wantNet:   17,
			wantNode:  0,
			wantError: false,
		},
		{
			name:      "host (network coordinator)",
			nodeType:  "Host",
			nodeNum:   "5020",
			initZone:  2,
			initNet:   1,
			wantType:  "Host",
			wantZone:  2,
			wantNet:   5020,
			wantNode:  0,
			wantError: false,
		},
		{
			name:      "hub node",
			nodeType:  "Hub",
			nodeNum:   "100",
			initZone:  1,
			initNet:   5001,
			wantType:  "Hub",
			wantZone:  1,
			wantNet:   5001,
			wantNode:  100,
			wantError: false,
		},
		{
			name:      "pvt node",
			nodeType:  "Pvt",
			nodeNum:   "50",
			initZone:  1,
			initNet:   5001,
			wantType:  "Pvt",
			wantZone:  1,
			wantNet:   5001,
			wantNode:  50,
			wantError: false,
		},
		{
			name:      "hold node",
			nodeType:  "Hold",
			nodeNum:   "25",
			initZone:  1,
			initNet:   5001,
			wantType:  "Hold",
			wantZone:  1,
			wantNet:   5001,
			wantNode:  25,
			wantError: false,
		},
		{
			name:      "down node",
			nodeType:  "Down",
			nodeNum:   "10",
			initZone:  1,
			initNet:   5001,
			wantType:  "Down",
			wantZone:  1,
			wantNet:   5001,
			wantNode:  10,
			wantError: false,
		},
		{
			name:      "case insensitive zone",
			nodeType:  "ZONE",
			nodeNum:   "3",
			initZone:  1,
			initNet:   1,
			wantType:  "Zone",
			wantZone:  3,
			wantNet:   3,
			wantNode:  0,
			wantError: false,
		},
		{
			name:      "invalid node number",
			nodeType:  "",
			nodeNum:   "abc",
			initZone:  1,
			initNet:   5001,
			wantError: true,
		},
		{
			name:      "unknown node type",
			nodeType:  "Unknown",
			nodeNum:   "1",
			initZone:  1,
			initNet:   1,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(false)
			p.Context.CurrentZone = tt.initZone
			p.Context.CurrentNet = tt.initNet

			nodeType, zone, net, node, _, err := p.parseNodeType(tt.nodeType, tt.nodeNum)

			if tt.wantError {
				if err == nil {
					t.Errorf("parseNodeType(%q, %q) expected error, got nil", tt.nodeType, tt.nodeNum)
				}
				return
			}

			if err != nil {
				t.Errorf("parseNodeType(%q, %q) unexpected error: %v", tt.nodeType, tt.nodeNum, err)
				return
			}

			if nodeType != tt.wantType {
				t.Errorf("nodeType = %q, want %q", nodeType, tt.wantType)
			}
			if zone != tt.wantZone {
				t.Errorf("zone = %d, want %d", zone, tt.wantZone)
			}
			if net != tt.wantNet {
				t.Errorf("net = %d, want %d", net, tt.wantNet)
			}
			if node != tt.wantNode {
				t.Errorf("node = %d, want %d", node, tt.wantNode)
			}
		})
	}
}

func TestParseProtocolValue(t *testing.T) {
	p := New(false)

	tests := []struct {
		name     string
		value    string
		wantAddr string
		wantPort int
	}{
		{
			name:     "port only",
			value:    "24554",
			wantAddr: "",
			wantPort: 24554,
		},
		{
			name:     "hostname only",
			value:    "bbs.example.com",
			wantAddr: "bbs.example.com",
			wantPort: 0,
		},
		{
			name:     "hostname with port",
			value:    "bbs.example.com:24555",
			wantAddr: "bbs.example.com",
			wantPort: 24555,
		},
		{
			name:     "IPv4 only",
			value:    "192.168.1.1",
			wantAddr: "192.168.1.1",
			wantPort: 0,
		},
		{
			name:     "IPv4 with port",
			value:    "192.168.1.1:24554",
			wantAddr: "192.168.1.1",
			wantPort: 24554,
		},
		{
			name:     "bracketed IPv6 only",
			value:    "[::1]",
			wantAddr: "[::1]",
			wantPort: 0,
		},
		{
			name:     "bracketed IPv6 with port",
			value:    "[::1]:24554",
			wantAddr: "[::1]",
			wantPort: 24554,
		},
		{
			name:     "bracketed full IPv6 with port",
			value:    "[2001:db8::1]:8080",
			wantAddr: "[2001:db8::1]",
			wantPort: 8080,
		},
		{
			name:     "unbracketed IPv6",
			value:    "2001:db8::1",
			wantAddr: "2001:db8::1",
			wantPort: 0,
		},
		{
			name:     "invalid port number (too high)",
			value:    "host:99999",
			wantAddr: "host:99999",
			wantPort: 0,
		},
		{
			name:     "zero port",
			value:    "0",
			wantAddr: "0",
			wantPort: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, port := p.parseProtocolValue(tt.value)

			if addr != tt.wantAddr {
				t.Errorf("address = %q, want %q", addr, tt.wantAddr)
			}
			if port != tt.wantPort {
				t.Errorf("port = %d, want %d", port, tt.wantPort)
			}
		})
	}
}

func TestParseFlagsWithConfig(t *testing.T) {
	p := New(false)

	tests := []struct {
		name           string
		flagsStr       string
		wantFlagsLen   int
		wantHasConfig  bool
		wantFlagsCheck []string // flags that should be in the result
	}{
		{
			name:           "empty flags",
			flagsStr:       "",
			wantFlagsLen:   0,
			wantHasConfig:  false,
			wantFlagsCheck: nil,
		},
		{
			name:           "simple flags",
			flagsStr:       "CM,MO,XA",
			wantFlagsLen:   3,
			wantHasConfig:  false,
			wantFlagsCheck: []string{"CM", "MO", "XA"},
		},
		{
			name:           "IBN protocol without value",
			flagsStr:       "IBN,CM",
			wantFlagsLen:   1, // Only CM goes to flags, IBN goes to config
			wantHasConfig:  true,
			wantFlagsCheck: []string{"CM"},
		},
		{
			name:           "IBN protocol with port",
			flagsStr:       "IBN:24555,CM",
			wantFlagsLen:   1,
			wantHasConfig:  true,
			wantFlagsCheck: []string{"CM"},
		},
		{
			name:           "IBN with hostname",
			flagsStr:       "IBN:bbs.example.com,CM",
			wantFlagsLen:   1,
			wantHasConfig:  true,
			wantFlagsCheck: []string{"CM"},
		},
		{
			name:           "INA default address",
			flagsStr:       "INA:node.fido.net,IBN,CM",
			wantFlagsLen:   1,
			wantHasConfig:  true,
			wantFlagsCheck: []string{"CM"},
		},
		{
			name:           "multiple protocols",
			flagsStr:       "IBN:24554,ITN:23,IFC,CM,MO",
			wantFlagsLen:   2, // CM and MO
			wantHasConfig:  true,
			wantFlagsCheck: []string{"CM", "MO"},
		},
		{
			name:           "user flags with values",
			flagsStr:       "U:ESTaBcD,CM",
			wantFlagsLen:   2,
			wantHasConfig:  false,
			wantFlagsCheck: []string{"U:ESTaBcD", "CM"},
		},
		{
			name:           "BND alternative for IBN",
			flagsStr:       "BND,CM",
			wantFlagsLen:   1,
			wantHasConfig:  true,
			wantFlagsCheck: []string{"CM"},
		},
		{
			name:           "TEL alternative for ITN",
			flagsStr:       "TEL,CM",
			wantFlagsLen:   1,
			wantHasConfig:  true,
			wantFlagsCheck: []string{"CM"},
		},
		{
			name:           "info flags INO4 ICM",
			flagsStr:       "INO4,ICM,CM",
			wantFlagsLen:   1,
			wantHasConfig:  true,
			wantFlagsCheck: []string{"CM"},
		},
		{
			name:           "email protocol IEM",
			flagsStr:       "IEM:sysop@example.com,CM",
			wantFlagsLen:   1,
			wantHasConfig:  true,
			wantFlagsCheck: []string{"CM"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags, config := p.parseFlagsWithConfig(tt.flagsStr)

			if len(flags) != tt.wantFlagsLen {
				t.Errorf("flags length = %d, want %d (flags: %v)", len(flags), tt.wantFlagsLen, flags)
			}

			hasConfig := len(config) > 0
			if hasConfig != tt.wantHasConfig {
				t.Errorf("hasConfig = %v, want %v", hasConfig, tt.wantHasConfig)
			}

			for _, wantFlag := range tt.wantFlagsCheck {
				found := false
				for _, f := range flags {
					if f == wantFlag {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected flag %q not found in %v", wantFlag, flags)
				}
			}
		})
	}
}

func TestHasFlag(t *testing.T) {
	p := New(false)

	tests := []struct {
		name   string
		flags  []string
		flag   string
		expect bool
	}{
		{
			name:   "flag exists",
			flags:  []string{"CM", "MO", "IBN"},
			flag:   "CM",
			expect: true,
		},
		{
			name:   "flag not exists",
			flags:  []string{"CM", "MO"},
			flag:   "IBN",
			expect: false,
		},
		{
			name:   "case insensitive match",
			flags:  []string{"cm", "mo"},
			flag:   "CM",
			expect: true,
		},
		{
			name:   "empty flags",
			flags:  []string{},
			flag:   "CM",
			expect: false,
		},
		{
			name:   "nil flags",
			flags:  nil,
			flag:   "CM",
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.hasFlag(tt.flags, tt.flag)
			if result != tt.expect {
				t.Errorf("hasFlag(%v, %q) = %v, want %v", tt.flags, tt.flag, result, tt.expect)
			}
		})
	}
}

func TestConvertLegacyFlags(t *testing.T) {
	p := New(false)

	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "XP: to XA",
			input:  "XP:CM",
			expect: "XACM",
		},
		{
			name:   "MO: to MO",
			input:  "MO:LO",
			expect: "MOLO",
		},
		{
			name:   "multiple conversions",
			input:  "XP:MO:CM:",
			expect: "XAMOCM",
		},
		{
			name:   "no legacy flags",
			input:  "CM,MO,IBN",
			expect: "CM,MO,IBN",
		},
		{
			name:   "empty string",
			input:  "",
			expect: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.convertLegacyFlags(tt.input)
			if result != tt.expect {
				t.Errorf("convertLegacyFlags(%q) = %q, want %q", tt.input, result, tt.expect)
			}
		})
	}
}

func TestParseAdvancedFlags(t *testing.T) {
	p := New(false)

	tests := []struct {
		name           string
		flagsStr       string
		wantAllLen     int
		wantModemLen   int
		wantProtoLen   int
	}{
		{
			name:         "empty flags",
			flagsStr:     "",
			wantAllLen:   0,
			wantModemLen: 0,
			wantProtoLen: 0,
		},
		{
			name:         "modem flags only",
			flagsStr:     "V32B,V34,HST",
			wantAllLen:   3,
			wantModemLen: 3,
			wantProtoLen: 0,
		},
		{
			name:         "internet protocols",
			flagsStr:     "IBN:24554,ITN:23",
			wantAllLen:   2,
			wantModemLen: 0,
			wantProtoLen: 2,
		},
		{
			name:         "mixed flags",
			flagsStr:     "V34,IBN:24554,HST,CM",
			wantAllLen:   4,
			wantModemLen: 2, // V34, HST
			wantProtoLen: 1, // IBN
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allFlags, internetProtos, _, _, _, modemFlags := p.parseAdvancedFlags(tt.flagsStr)

			if len(allFlags) != tt.wantAllLen {
				t.Errorf("allFlags length = %d, want %d (flags: %v)", len(allFlags), tt.wantAllLen, allFlags)
			}
			if len(modemFlags) != tt.wantModemLen {
				t.Errorf("modemFlags length = %d, want %d (flags: %v)", len(modemFlags), tt.wantModemLen, modemFlags)
			}
			if len(internetProtos) != tt.wantProtoLen {
				t.Errorf("internetProtocols length = %d, want %d (protos: %v)", len(internetProtos), tt.wantProtoLen, internetProtos)
			}
		})
	}
}

func TestParseLine(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		initZone  int
		initNet   int
		wantZone  int
		wantNet   int
		wantNode  int
		wantType  string
		wantCM    bool
		wantMO    bool
		wantInet  bool
		wantError bool
	}{
		{
			name:      "simple node",
			line:      ",100,Test_BBS,New_York,John_Doe,1-212-555-1234,9600,CM",
			initZone:  1,
			initNet:   5001,
			wantZone:  1,
			wantNet:   5001,
			wantNode:  100,
			wantType:  "Node",
			wantCM:    true,
			wantMO:    false,
			wantInet:  false,
			wantError: false,
		},
		{
			name:      "node with IBN",
			line:      ",200,Internet_BBS,City,Sysop,000-000-000,9600,CM,IBN",
			initZone:  2,
			initNet:   5020,
			wantZone:  2,
			wantNet:   5020,
			wantNode:  200,
			wantType:  "Node",
			wantCM:    true,
			wantMO:    false,
			wantInet:  true,
			wantError: false,
		},
		{
			name:      "node with INA and IBN",
			line:      ",300,Full_Internet,City,Op,Phone,9600,CM,INA:bbs.example.com,IBN:24554",
			initZone:  1,
			initNet:   5001,
			wantZone:  1,
			wantNet:   5001,
			wantNode:  300,
			wantType:  "Node",
			wantCM:    true,
			wantMO:    false,
			wantInet:  true,
			wantError: false,
		},
		{
			name:      "hub node",
			line:      "Hub,50,Hub_BBS,Location,Admin,Phone,9600,CM,MO",
			initZone:  1,
			initNet:   5001,
			wantZone:  1,
			wantNet:   5001,
			wantNode:  50,
			wantType:  "Hub",
			wantCM:    true,
			wantMO:    true,
			wantInet:  false,
			wantError: false,
		},
		{
			name:      "zone coordinator",
			line:      "Zone,2,Zone2_HQ,Europe,ZC,Phone,9600,CM,IBN",
			initZone:  1,
			initNet:   1,
			wantZone:  2,
			wantNet:   2,
			wantNode:  0,
			wantType:  "Zone",
			wantCM:    true,
			wantMO:    false,
			wantInet:  true,
			wantError: false,
		},
		{
			name:      "host (network coordinator)",
			line:      "Host,5020,Net_5020_HQ,Moscow,NC,Phone,9600,CM",
			initZone:  2,
			initNet:   1,
			wantZone:  2,
			wantNet:   5020,
			wantNode:  0,
			wantType:  "Host",
			wantCM:    true,
			wantMO:    false,
			wantInet:  false,
			wantError: false,
		},
		{
			name:      "pvt node",
			line:      "Pvt,999,Private_BBS,City,Op,000-000-000,2400",
			initZone:  1,
			initNet:   5001,
			wantZone:  1,
			wantNet:   5001,
			wantNode:  999,
			wantType:  "Pvt",
			wantCM:    false,
			wantMO:    false,
			wantInet:  false,
			wantError: false,
		},
		{
			name:      "down node",
			line:      "Down,123,Offline_BBS,City,Op,Phone,9600",
			initZone:  1,
			initNet:   5001,
			wantZone:  1,
			wantNet:   5001,
			wantNode:  123,
			wantType:  "Down",
			wantCM:    false,
			wantMO:    false,
			wantInet:  false,
			wantError: false,
		},
		{
			name:      "insufficient fields",
			line:      ",100,BBS",
			initZone:  1,
			initNet:   1,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(false)
			p.Context.CurrentZone = tt.initZone
			p.Context.CurrentNet = tt.initNet

			node, err := p.parseLine(tt.line, time.Now(), 1, "test.txt")

			if tt.wantError {
				if err == nil {
					t.Errorf("parseLine(%q) expected error, got nil", tt.line)
				}
				return
			}

			if err != nil {
				t.Errorf("parseLine(%q) unexpected error: %v", tt.line, err)
				return
			}

			if node.Zone != tt.wantZone {
				t.Errorf("Zone = %d, want %d", node.Zone, tt.wantZone)
			}
			if node.Net != tt.wantNet {
				t.Errorf("Net = %d, want %d", node.Net, tt.wantNet)
			}
			if node.Node != tt.wantNode {
				t.Errorf("Node = %d, want %d", node.Node, tt.wantNode)
			}
			if node.NodeType != tt.wantType {
				t.Errorf("NodeType = %q, want %q", node.NodeType, tt.wantType)
			}
			if node.IsCM != tt.wantCM {
				t.Errorf("IsCM = %v, want %v", node.IsCM, tt.wantCM)
			}
			if node.IsMO != tt.wantMO {
				t.Errorf("IsMO = %v, want %v", node.IsMO, tt.wantMO)
			}
			if node.HasInet != tt.wantInet {
				t.Errorf("HasInet = %v, want %v", node.HasInet, tt.wantInet)
			}
			if node.RawLine != tt.line {
				t.Errorf("RawLine = %q, want %q", node.RawLine, tt.line)
			}
		})
	}
}

func TestTrackDuplicates(t *testing.T) {
	t.Run("first occurrence", func(t *testing.T) {
		p := New(false)
		nodes := []database.Node{}
		stats := struct {
			totalDuplicates int
			conflictGroups  int
		}{}

		node := database.Node{Zone: 1, Net: 5001, Node: 100}
		p.trackDuplicates(&node, &nodes, &stats, 1, "test.txt")

		if node.HasConflict {
			t.Error("first occurrence should not have conflict")
		}
		if node.ConflictSequence != 0 {
			t.Errorf("first occurrence ConflictSequence = %d, want 0", node.ConflictSequence)
		}
		if stats.totalDuplicates != 0 {
			t.Errorf("totalDuplicates = %d, want 0", stats.totalDuplicates)
		}
	})

	t.Run("duplicate occurrence", func(t *testing.T) {
		p := New(false)
		nodes := []database.Node{}
		stats := struct {
			totalDuplicates int
			conflictGroups  int
		}{}

		// Create and track first node
		firstNode := database.Node{Zone: 1, Net: 5001, Node: 100, HasConflict: false, ConflictSequence: 0}
		p.trackDuplicates(&firstNode, &nodes, &stats, 1, "test.txt")
		nodes = append(nodes, firstNode)

		// Now add duplicate
		duplicateNode := database.Node{Zone: 1, Net: 5001, Node: 100}
		p.trackDuplicates(&duplicateNode, &nodes, &stats, 10, "test.txt")

		if !duplicateNode.HasConflict {
			t.Error("duplicate should have conflict")
		}
		if duplicateNode.ConflictSequence != 1 {
			t.Errorf("duplicate ConflictSequence = %d, want 1", duplicateNode.ConflictSequence)
		}
		if !nodes[0].HasConflict {
			t.Error("original should be marked as having conflict")
		}
		if stats.totalDuplicates != 1 {
			t.Errorf("totalDuplicates = %d, want 1", stats.totalDuplicates)
		}
		if stats.conflictGroups != 1 {
			t.Errorf("conflictGroups = %d, want 1", stats.conflictGroups)
		}
	})

	t.Run("multiple duplicates", func(t *testing.T) {
		p := New(false)
		nodes := []database.Node{}
		stats := struct {
			totalDuplicates int
			conflictGroups  int
		}{}

		// Track first
		node1 := database.Node{Zone: 1, Net: 5001, Node: 100}
		p.trackDuplicates(&node1, &nodes, &stats, 1, "test.txt")
		nodes = append(nodes, node1)

		// Add second (first duplicate)
		dup1 := database.Node{Zone: 1, Net: 5001, Node: 100}
		p.trackDuplicates(&dup1, &nodes, &stats, 10, "test.txt")
		nodes = append(nodes, dup1)

		// Add third (second duplicate)
		dup2 := database.Node{Zone: 1, Net: 5001, Node: 100}
		p.trackDuplicates(&dup2, &nodes, &stats, 20, "test.txt")
		nodes = append(nodes, dup2)

		if nodes[2].ConflictSequence != 2 {
			t.Errorf("third occurrence ConflictSequence = %d, want 2", nodes[2].ConflictSequence)
		}
		if stats.totalDuplicates != 2 {
			t.Errorf("totalDuplicates = %d, want 2", stats.totalDuplicates)
		}
		if stats.conflictGroups != 1 {
			t.Errorf("conflictGroups = %d, want 1 (single node with duplicates)", stats.conflictGroups)
		}
	})

	t.Run("different nodes not duplicates", func(t *testing.T) {
		p := New(false)
		nodes := []database.Node{}
		stats := struct {
			totalDuplicates int
			conflictGroups  int
		}{}

		node1 := database.Node{Zone: 1, Net: 5001, Node: 100}
		node2 := database.Node{Zone: 1, Net: 5001, Node: 200}

		p.trackDuplicates(&node1, &nodes, &stats, 1, "test.txt")
		nodes = append(nodes, node1)
		p.trackDuplicates(&node2, &nodes, &stats, 2, "test.txt")

		if node1.HasConflict || node2.HasConflict {
			t.Error("different nodes should not have conflicts")
		}
		if stats.totalDuplicates != 0 {
			t.Errorf("totalDuplicates = %d, want 0", stats.totalDuplicates)
		}
	})
}
