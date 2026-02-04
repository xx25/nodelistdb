// Package main provides tests for modem worker and pool functionality.
package main

import (
	"testing"
	"time"
)

// Test WorkerResult structure
func TestWorkerResult_Fields(t *testing.T) {
	result := WorkerResult{
		WorkerID:       1,
		WorkerName:     "modem1",
		Phone:          "79001234567",
		OperatorName:   "VoIP-A",
		OperatorPrefix: "1#",
		NodeAddress:    "2:5001/100",
		NodeSystemName: "Test BBS",
		NodeLocation:   "Moscow",
		NodeSysop:      "Test Sysop",
		TestNum:        5,
		Timestamp:      time.Now(),
		Result: testResult{
			success: true,
			message: "Test passed",
		},
	}

	if result.WorkerID != 1 {
		t.Errorf("WorkerID = %d, want 1", result.WorkerID)
	}
	if result.WorkerName != "modem1" {
		t.Errorf("WorkerName = %q, want %q", result.WorkerName, "modem1")
	}
	if result.Phone != "79001234567" {
		t.Errorf("Phone = %q, want %q", result.Phone, "79001234567")
	}
	if result.OperatorName != "VoIP-A" {
		t.Errorf("OperatorName = %q, want %q", result.OperatorName, "VoIP-A")
	}
	if result.OperatorPrefix != "1#" {
		t.Errorf("OperatorPrefix = %q, want %q", result.OperatorPrefix, "1#")
	}
	if result.NodeAddress != "2:5001/100" {
		t.Errorf("NodeAddress = %q, want %q", result.NodeAddress, "2:5001/100")
	}
	if !result.Result.success {
		t.Error("Result.success = false, want true")
	}
}

// Test WorkerStats structure
func TestWorkerStats_Fields(t *testing.T) {
	stats := WorkerStats{
		Total:         10,
		Success:       7,
		Failed:        3,
		TotalDialTime: 5 * time.Minute,
		TotalEmsiTime: 30 * time.Second,
	}

	if stats.Total != 10 {
		t.Errorf("Total = %d, want 10", stats.Total)
	}
	if stats.Success != 7 {
		t.Errorf("Success = %d, want 7", stats.Success)
	}
	if stats.Failed != 3 {
		t.Errorf("Failed = %d, want 3", stats.Failed)
	}

	// Verify consistency
	if stats.Success+stats.Failed != stats.Total {
		t.Errorf("Success + Failed = %d, want %d (Total)", stats.Success+stats.Failed, stats.Total)
	}
}

// Test RetryAttemptCallback type
func TestRetryAttemptCallback(t *testing.T) {
	// Test callback invocation
	var called bool
	var gotAttempt int
	var gotDialTime time.Duration
	var gotReason, gotOpName, gotOpPrefix string

	callback := func(attempt int, dialTime time.Duration, reason, operatorName, operatorPrefix string) {
		called = true
		gotAttempt = attempt
		gotDialTime = dialTime
		gotReason = reason
		gotOpName = operatorName
		gotOpPrefix = operatorPrefix
	}

	callback(5, 30*time.Second, "BUSY", "TestOp", "99")

	if !called {
		t.Error("callback was not called")
	}
	if gotAttempt != 5 {
		t.Errorf("attempt = %d, want 5", gotAttempt)
	}
	if gotDialTime != 30*time.Second {
		t.Errorf("dialTime = %v, want %v", gotDialTime, 30*time.Second)
	}
	if gotReason != "BUSY" {
		t.Errorf("reason = %q, want %q", gotReason, "BUSY")
	}
	if gotOpName != "TestOp" {
		t.Errorf("operatorName = %q, want %q", gotOpName, "TestOp")
	}
	if gotOpPrefix != "99" {
		t.Errorf("operatorPrefix = %q, want %q", gotOpPrefix, "99")
	}
}

// Test testResult structure
func TestTestResult_Fields(t *testing.T) {
	// Success result
	successResult := testResult{
		success:       true,
		message:       "Test 1 [modem1] 79001234567: OK - CONNECT 31200, EMSI 2.5s, Test BBS",
		dialTime:      45 * time.Second,
		emsiTime:      2500 * time.Millisecond,
		connectSpeed:  31200,
		connectString: "CONNECT 31200/ARQ/V34/LAPM/V42BIS",
		emsiInfo: &EMSIDetails{
			SystemName: "Test BBS",
			Sysop:      "Test Sysop",
		},
	}

	if !successResult.success {
		t.Error("success result should have success=true")
	}
	if successResult.connectSpeed != 31200 {
		t.Errorf("connectSpeed = %d, want 31200", successResult.connectSpeed)
	}
	if successResult.emsiInfo == nil {
		t.Error("emsiInfo should not be nil for success")
	}
	if successResult.emsiError != nil {
		t.Error("emsiError should be nil for success")
	}

	// Failed result
	failedResult := testResult{
		success:  false,
		message:  "Test 1 [modem1] 79001234567: DIAL FAILED - NO CARRIER",
		dialTime: 60 * time.Second,
	}

	if failedResult.success {
		t.Error("failed result should have success=false")
	}
	if failedResult.emsiInfo != nil {
		t.Error("emsiInfo should be nil for failed dial")
	}
}

// Test EMSIDetails structure
func TestEMSIDetails_Fields(t *testing.T) {
	details := &EMSIDetails{
		SystemName:    "Test BBS",
		Location:      "Moscow, Russia",
		Sysop:         "Test Sysop",
		Addresses:     []string{"2:5001/100", "2:5001/100.1"},
		MailerName:    "binkd",
		MailerVersion: "1.1a",
		Protocols:     []string{"ZAP", "ZMO"},
		Capabilities:  []string{"PUA", "NPU"},
	}

	if details.SystemName != "Test BBS" {
		t.Errorf("SystemName = %q, want %q", details.SystemName, "Test BBS")
	}
	if details.MailerName != "binkd" {
		t.Errorf("MailerName = %q, want %q", details.MailerName, "binkd")
	}
	if len(details.Addresses) != 2 {
		t.Errorf("Addresses len = %d, want 2", len(details.Addresses))
	}
	if len(details.Protocols) != 2 {
		t.Errorf("Protocols len = %d, want 2", len(details.Protocols))
	}
}

// Test that phoneJob accepts multiple operators
func TestPhoneJob_OperatorsField(t *testing.T) {
	operators := []OperatorConfig{
		{Name: "Direct", Prefix: ""},
		{Name: "VoIP-A", Prefix: "1#"},
		{Name: "VoIP-B", Prefix: "2#"},
	}

	job := phoneJob{
		phone:     "79001234567",
		operators: operators,
		testNum:   1,
	}

	if len(job.operators) != 3 {
		t.Errorf("operators len = %d, want 3", len(job.operators))
	}

	// Verify operators are stored correctly
	for i, op := range job.operators {
		if op.Name != operators[i].Name {
			t.Errorf("operators[%d].Name = %q, want %q", i, op.Name, operators[i].Name)
		}
		if op.Prefix != operators[i].Prefix {
			t.Errorf("operators[%d].Prefix = %q, want %q", i, op.Prefix, operators[i].Prefix)
		}
	}
}

// Test backward compatibility with legacy single-operator fields
func TestPhoneJob_LegacyFields(t *testing.T) {
	// Legacy job without operators list
	legacyJob := phoneJob{
		phone:          "79001234567",
		operatorName:   "Legacy Operator",
		operatorPrefix: "99#",
		testNum:        1,
	}

	// Legacy fields should work
	if legacyJob.operatorName != "Legacy Operator" {
		t.Errorf("operatorName = %q, want %q", legacyJob.operatorName, "Legacy Operator")
	}
	if legacyJob.operatorPrefix != "99#" {
		t.Errorf("operatorPrefix = %q, want %q", legacyJob.operatorPrefix, "99#")
	}

	// operators should be nil/empty
	if len(legacyJob.operators) != 0 {
		t.Errorf("operators len = %d, want 0 for legacy job", len(legacyJob.operators))
	}
}

// Test that phoneJob stores node info from API
func TestPhoneJob_NodeInfo(t *testing.T) {
	job := phoneJob{
		phone:          "79001234567",
		testNum:        1,
		nodeAddress:    "2:5001/100",
		nodeSystemName: "Test_BBS",
		nodeLocation:   "Moscow_Russia",
		nodeSysop:      "Test_Sysop",
	}

	if job.nodeAddress != "2:5001/100" {
		t.Errorf("nodeAddress = %q, want %q", job.nodeAddress, "2:5001/100")
	}
	if job.nodeSystemName != "Test_BBS" {
		t.Errorf("nodeSystemName = %q, want %q", job.nodeSystemName, "Test_BBS")
	}
	if job.nodeLocation != "Moscow_Russia" {
		t.Errorf("nodeLocation = %q, want %q", job.nodeLocation, "Moscow_Russia")
	}
	if job.nodeSysop != "Test_Sysop" {
		t.Errorf("nodeSysop = %q, want %q", job.nodeSysop, "Test_Sysop")
	}
}

// Test empty phoneJob
func TestPhoneJob_Empty(t *testing.T) {
	job := phoneJob{}

	if job.phone != "" {
		t.Errorf("empty job phone = %q, want empty", job.phone)
	}
	if len(job.operators) > 0 {
		t.Error("empty job operators should be nil or empty")
	}
	if job.testNum != 0 {
		t.Errorf("empty job testNum = %d, want 0", job.testNum)
	}
	if job.nodeAddress != "" {
		t.Errorf("empty job nodeAddress = %q, want empty", job.nodeAddress)
	}
}
