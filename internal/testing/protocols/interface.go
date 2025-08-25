package protocols

import (
	"context"
	"time"
)

// Tester defines the interface for protocol testers
type Tester interface {
	// Test performs a connectivity test
	Test(ctx context.Context, host string, port int, expectedAddress string) TestResult
	
	// GetProtocolName returns the protocol name
	GetProtocolName() string
}

// DebugSetter is an optional interface for testers that support debug mode
type DebugSetter interface {
	SetDebug(enabled bool)
}

// TestResult is the base interface for test results
type TestResult interface {
	IsSuccess() bool
	GetError() string
	GetResponseTime() uint32
}

// BaseTestResult contains common test result fields
type BaseTestResult struct {
	Success    bool
	Error      string
	ResponseMs uint32
	TestTime   time.Time
}

func (r *BaseTestResult) IsSuccess() bool {
	return r.Success
}

func (r *BaseTestResult) GetError() string {
	return r.Error
}

func (r *BaseTestResult) GetResponseTime() uint32 {
	return r.ResponseMs
}

// BinkPTestResult contains BinkP-specific test results
type BinkPTestResult struct {
	BaseTestResult
	SystemName   string
	Sysop        string
	Location     string
	Version      string
	Addresses    []string
	Capabilities []string
	AddressValid bool
	Port         int
}

// IfcicoTestResult contains IFCICO-specific test results
type IfcicoTestResult struct {
	BaseTestResult
	MailerInfo   string
	SystemName   string
	Addresses    []string
	ResponseType string // REQ/ACK/NAK/CLI/HBT
}

// TelnetTestResult contains Telnet-specific test results
type TelnetTestResult struct {
	BaseTestResult
	Banner string
}

// FTPTestResult contains FTP-specific test results
type FTPTestResult struct {
	BaseTestResult
	Banner string
}

// VModemTestResult contains VModem-specific test results
type VModemTestResult struct {
	BaseTestResult
	Banner string
}