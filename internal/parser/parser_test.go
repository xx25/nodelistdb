package parser

import (
	"errors"
	"testing"
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
