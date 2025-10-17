package daemon

import (
	"testing"

	"github.com/nodelistdb/internal/testing/models"
)

func TestDetermineOperationalStatus_BinkPSuccess(t *testing.T) {
	te := &TestExecutor{}

	result := &models.TestResult{
		BinkPResult: &models.ProtocolTestResult{
			Tested:  true,
			Success: true,
		},
	}

	status := te.determineOperationalStatus(result)
	if !status {
		t.Error("Expected operational status true for successful BinkP")
	}
}

func TestDetermineOperationalStatus_IfcicoSuccess(t *testing.T) {
	te := &TestExecutor{}

	result := &models.TestResult{
		IfcicoResult: &models.ProtocolTestResult{
			Tested:  true,
			Success: true,
		},
	}

	status := te.determineOperationalStatus(result)
	if !status {
		t.Error("Expected operational status true for successful Ifcico")
	}
}

func TestDetermineOperationalStatus_TelnetSuccess(t *testing.T) {
	te := &TestExecutor{}

	result := &models.TestResult{
		TelnetResult: &models.ProtocolTestResult{
			Tested:  true,
			Success: true,
		},
	}

	status := te.determineOperationalStatus(result)
	if !status {
		t.Error("Expected operational status true for successful Telnet")
	}
}

func TestDetermineOperationalStatus_FTPSuccess(t *testing.T) {
	te := &TestExecutor{}

	result := &models.TestResult{
		FTPResult: &models.ProtocolTestResult{
			Tested:  true,
			Success: true,
		},
	}

	status := te.determineOperationalStatus(result)
	if !status {
		t.Error("Expected operational status true for successful FTP")
	}
}

func TestDetermineOperationalStatus_VModemSuccess(t *testing.T) {
	te := &TestExecutor{}

	result := &models.TestResult{
		VModemResult: &models.ProtocolTestResult{
			Tested:  true,
			Success: true,
		},
	}

	status := te.determineOperationalStatus(result)
	if !status {
		t.Error("Expected operational status true for successful VModem")
	}
}

func TestDetermineOperationalStatus_MultipleProtocols_OneSuccess(t *testing.T) {
	te := &TestExecutor{}

	result := &models.TestResult{
		BinkPResult: &models.ProtocolTestResult{
			Tested:  true,
			Success: false,
			Error:   "Connection failed",
		},
		IfcicoResult: &models.ProtocolTestResult{
			Tested:  true,
			Success: true,
		},
		TelnetResult: &models.ProtocolTestResult{
			Tested:  true,
			Success: false,
			Error:   "Timeout",
		},
	}

	status := te.determineOperationalStatus(result)
	if !status {
		t.Error("Expected operational status true when at least one protocol succeeds")
	}
}

func TestDetermineOperationalStatus_AllProtocolsFail(t *testing.T) {
	te := &TestExecutor{}

	result := &models.TestResult{
		BinkPResult: &models.ProtocolTestResult{
			Tested:  true,
			Success: false,
			Error:   "Connection failed",
		},
		IfcicoResult: &models.ProtocolTestResult{
			Tested:  true,
			Success: false,
			Error:   "Connection failed",
		},
		TelnetResult: &models.ProtocolTestResult{
			Tested:  true,
			Success: false,
			Error:   "Connection failed",
		},
	}

	status := te.determineOperationalStatus(result)
	if status {
		t.Error("Expected operational status false when all protocols fail")
	}
}

func TestDetermineOperationalStatus_NoProtocolsTested(t *testing.T) {
	te := &TestExecutor{}

	result := &models.TestResult{}

	status := te.determineOperationalStatus(result)
	if status {
		t.Error("Expected operational status false when no protocols tested")
	}
}

func TestDetermineOperationalStatus_ProtocolTestedButNotSuccess(t *testing.T) {
	te := &TestExecutor{}

	result := &models.TestResult{
		BinkPResult: &models.ProtocolTestResult{
			Tested:  true,
			Success: false,
		},
	}

	status := te.determineOperationalStatus(result)
	if status {
		t.Error("Expected operational status false when protocol tested but failed")
	}
}

func TestDetermineOperationalStatus_ProtocolNotTested(t *testing.T) {
	te := &TestExecutor{}

	result := &models.TestResult{
		BinkPResult: &models.ProtocolTestResult{
			Tested:  false,
			Success: false,
		},
	}

	status := te.determineOperationalStatus(result)
	if status {
		t.Error("Expected operational status false when protocol not tested")
	}
}

func TestDetermineOperationalStatus_NilProtocolResults(t *testing.T) {
	te := &TestExecutor{}

	result := &models.TestResult{
		BinkPResult:   nil,
		IfcicoResult:  nil,
		TelnetResult:  nil,
		FTPResult:     nil,
		VModemResult:  nil,
	}

	status := te.determineOperationalStatus(result)
	if status {
		t.Error("Expected operational status false when all protocol results are nil")
	}
}

func TestDetermineOperationalStatus_MixedNilAndFailed(t *testing.T) {
	te := &TestExecutor{}

	result := &models.TestResult{
		BinkPResult: nil,
		IfcicoResult: &models.ProtocolTestResult{
			Tested:  true,
			Success: false,
		},
		TelnetResult: nil,
	}

	status := te.determineOperationalStatus(result)
	if status {
		t.Error("Expected operational status false with mix of nil and failed protocols")
	}
}

func TestDetermineOperationalStatus_AllProtocolsSuccess(t *testing.T) {
	te := &TestExecutor{}

	result := &models.TestResult{
		BinkPResult: &models.ProtocolTestResult{
			Tested:  true,
			Success: true,
		},
		IfcicoResult: &models.ProtocolTestResult{
			Tested:  true,
			Success: true,
		},
		TelnetResult: &models.ProtocolTestResult{
			Tested:  true,
			Success: true,
		},
		FTPResult: &models.ProtocolTestResult{
			Tested:  true,
			Success: true,
		},
		VModemResult: &models.ProtocolTestResult{
			Tested:  true,
			Success: true,
		},
	}

	status := te.determineOperationalStatus(result)
	if !status {
		t.Error("Expected operational status true when all protocols succeed")
	}
}

func TestDetermineOperationalStatus_FirstProtocolWins(t *testing.T) {
	te := &TestExecutor{}

	// BinkP succeeds, so should return true immediately
	result := &models.TestResult{
		BinkPResult: &models.ProtocolTestResult{
			Tested:  true,
			Success: true,
		},
		// Other protocols don't matter
	}

	status := te.determineOperationalStatus(result)
	if !status {
		t.Error("Expected operational status true when first protocol succeeds")
	}
}

func TestDetermineOperationalStatus_LastProtocolWins(t *testing.T) {
	te := &TestExecutor{}

	// All protocols fail except the last one
	result := &models.TestResult{
		BinkPResult: &models.ProtocolTestResult{
			Tested:  true,
			Success: false,
		},
		IfcicoResult: &models.ProtocolTestResult{
			Tested:  true,
			Success: false,
		},
		TelnetResult: &models.ProtocolTestResult{
			Tested:  true,
			Success: false,
		},
		FTPResult: &models.ProtocolTestResult{
			Tested:  true,
			Success: false,
		},
		VModemResult: &models.ProtocolTestResult{
			Tested:  true,
			Success: true, // Only this succeeds
		},
	}

	status := te.determineOperationalStatus(result)
	if !status {
		t.Error("Expected operational status true when last protocol succeeds")
	}
}

func TestDetermineOperationalStatus_SuccessFalseButNotTested(t *testing.T) {
	te := &TestExecutor{}

	// Edge case: Success is false but Tested is also false
	result := &models.TestResult{
		BinkPResult: &models.ProtocolTestResult{
			Tested:  false,
			Success: true, // This should be ignored because Tested is false
		},
	}

	status := te.determineOperationalStatus(result)
	if status {
		t.Error("Expected operational status false when protocol not tested (even if Success is true)")
	}
}

func TestDetermineOperationalStatus_RealWorldScenario1(t *testing.T) {
	te := &TestExecutor{}

	// Typical scenario: BinkP succeeds, Ifcico not tested
	result := &models.TestResult{
		BinkPResult: &models.ProtocolTestResult{
			Tested:  true,
			Success: true,
		},
		IfcicoResult: nil,
	}

	status := te.determineOperationalStatus(result)
	if !status {
		t.Error("Expected operational status true for typical BinkP-only success")
	}
}

func TestDetermineOperationalStatus_RealWorldScenario2(t *testing.T) {
	te := &TestExecutor{}

	// Typical failure scenario: DNS succeeded but all protocols failed
	result := &models.TestResult{
		BinkPResult: &models.ProtocolTestResult{
			Tested:  true,
			Success: false,
			Error:   "Connection timeout",
		},
	}

	status := te.determineOperationalStatus(result)
	if status {
		t.Error("Expected operational status false when protocol connection fails")
	}
}

func TestNewTestExecutor(t *testing.T) {
	daemon := &Daemon{} // Minimal daemon for testing
	te := NewTestExecutor(daemon)

	if te == nil {
		t.Fatal("Expected non-nil TestExecutor")
	}

	if te.daemon != daemon {
		t.Error("Expected daemon to be set correctly")
	}
}
