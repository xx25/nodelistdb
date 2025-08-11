package parser

import (
	"fmt"
	"strconv"
	"time"
)

// ParseError represents a parsing error with detailed context
type ParseError struct {
	File     string    `json:"file"`
	Line     int       `json:"line"`
	Column   int       `json:"column,omitempty"`
	Field    string    `json:"field,omitempty"`
	Value    string    `json:"value,omitempty"`
	Cause    error     `json:"cause,omitempty"`
	Message  string    `json:"message"`
	Occurred time.Time `json:"occurred"`
}

func (e *ParseError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("parse error at %s:%d: %s", e.File, e.Line, e.Message)
	}
	return fmt.Sprintf("parse error in %s: %s", e.File, e.Message)
}

func (e *ParseError) Unwrap() error {
	return e.Cause
}

// NewParseError creates a new ParseError with context
func NewParseError(file string, line int, message string) *ParseError {
	return &ParseError{
		File:     file,
		Line:     line,
		Message:  message,
		Occurred: time.Now(),
	}
}

// NewParseErrorWithCause creates a new ParseError with an underlying cause
func NewParseErrorWithCause(file string, line int, message string, cause error) *ParseError {
	return &ParseError{
		File:     file,
		Line:     line,
		Message:  message,
		Cause:    cause,
		Occurred: time.Now(),
	}
}

// NewFieldParseError creates a ParseError for a specific field
func NewFieldParseError(file string, line int, field, value, message string) *ParseError {
	return &ParseError{
		File:     file,
		Line:     line,
		Field:    field,
		Value:    value,
		Message:  message,
		Occurred: time.Now(),
	}
}

// FileError represents file-level errors (opening, reading, etc.)
type FileError struct {
	Path     string    `json:"path"`
	Op       string    `json:"operation"` // "open", "read", "stat", etc.
	Cause    error     `json:"cause,omitempty"`
	Message  string    `json:"message"`
	Occurred time.Time `json:"occurred"`
}

func (e *FileError) Error() string {
	return fmt.Sprintf("file %s error: %s", e.Op, e.Message)
}

func (e *FileError) Unwrap() error {
	return e.Cause
}

// NewFileError creates a new FileError
func NewFileError(path, op, message string, cause error) *FileError {
	return &FileError{
		Path:     path,
		Op:       op,
		Message:  message,
		Cause:    cause,
		Occurred: time.Now(),
	}
}

// DateError represents date parsing/extraction errors
type DateError struct {
	Source   string    `json:"source"` // "filename", "header", etc.
	Value    string    `json:"value"`
	Expected string    `json:"expected,omitempty"`
	Cause    error     `json:"cause,omitempty"`
	Message  string    `json:"message"`
	Occurred time.Time `json:"occurred"`
}

func (e *DateError) Error() string {
	return fmt.Sprintf("date error from %s: %s", e.Source, e.Message)
}

func (e *DateError) Unwrap() error {
	return e.Cause
}

// NewDateError creates a new DateError
func NewDateError(source, value, message string) *DateError {
	return &DateError{
		Source:   source,
		Value:    value,
		Message:  message,
		Occurred: time.Now(),
	}
}

// ConversionError represents type conversion errors (optimized for common cases)
type ConversionError struct {
	Field    string `json:"field"`
	Value    string `json:"value"`
	TargetType string `json:"target_type"`
	Cause    error  `json:"cause,omitempty"`
}

func (e *ConversionError) Error() string {
	return fmt.Sprintf("cannot convert %s='%s' to %s", e.Field, e.Value, e.TargetType)
}

func (e *ConversionError) Unwrap() error {
	return e.Cause
}

// NewConversionError creates a ConversionError for common conversion failures
func NewConversionError(field, value, targetType string, cause error) *ConversionError {
	return &ConversionError{
		Field:      field,
		Value:      value,
		TargetType: targetType,
		Cause:      cause,
	}
}

// Pre-defined common conversion errors to avoid allocations
var (
	ErrInvalidZone   = &ConversionError{Field: "zone", TargetType: "int"}
	ErrInvalidNet    = &ConversionError{Field: "net", TargetType: "int"}
	ErrInvalidNode   = &ConversionError{Field: "node", TargetType: "int"}
	ErrInvalidSpeed  = &ConversionError{Field: "speed", TargetType: "uint32"}
	ErrInvalidRegion = &ConversionError{Field: "region", TargetType: "int"}
)

// WithValue returns a copy of the error with the problematic value set
func (e *ConversionError) WithValue(value string) *ConversionError {
	return &ConversionError{
		Field:      e.Field,
		Value:      value,
		TargetType: e.TargetType,
		Cause:      e.Cause,
	}
}

// Common parsing helper functions that return optimized errors

// ParseInt wraps strconv.Atoi with optimized error handling
func ParseInt(field, value string) (int, error) {
	if value == "" {
		return 0, &ConversionError{
			Field:      field,
			Value:      value,
			TargetType: "int",
			Cause:      fmt.Errorf("empty value"),
		}
	}
	
	result, err := strconv.Atoi(value)
	if err != nil {
		return 0, &ConversionError{
			Field:      field,
			Value:      value,
			TargetType: "int",
			Cause:      err,
		}
	}
	return result, nil
}

// ParseUint32 wraps strconv.ParseUint with optimized error handling
func ParseUint32(field, value string) (uint32, error) {
	if value == "" {
		return 0, &ConversionError{
			Field:      field,
			Value:      value,
			TargetType: "uint32",
			Cause:      fmt.Errorf("empty value"),
		}
	}
	
	result, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return 0, &ConversionError{
			Field:      field,
			Value:      value,
			TargetType: "uint32",
			Cause:      err,
		}
	}
	return uint32(result), nil
}