package exec

import "testing"

func TestApplyGuardrailDefaults_FillsZeroValues(t *testing.T) {
	saved := DefaultGuardrails
	defer SetDefaultGuardrails(saved)
	SetDefaultGuardrails(GuardrailDefaults{ScanLimitGbytes: 500})

	opts := DQLExecuteOptions{}
	applyGuardrailDefaults(&opts)
	if opts.DefaultScanLimitGbytes != 500 {
		t.Errorf("ScanLimit not filled: got %v", opts.DefaultScanLimitGbytes)
	}
}

func TestApplyGuardrailDefaults_PreservesCallerValues(t *testing.T) {
	saved := DefaultGuardrails
	defer SetDefaultGuardrails(saved)
	SetDefaultGuardrails(GuardrailDefaults{ScanLimitGbytes: 500})

	opts := DQLExecuteOptions{DefaultScanLimitGbytes: 50}
	applyGuardrailDefaults(&opts)
	if opts.DefaultScanLimitGbytes != 50 {
		t.Errorf("caller ScanLimit overwritten: got %v", opts.DefaultScanLimitGbytes)
	}
}

func TestApplyGuardrailDefaults_HonorsDisable(t *testing.T) {
	saved := DefaultGuardrails
	defer SetDefaultGuardrails(saved)
	SetDefaultGuardrails(GuardrailDefaults{ScanLimitGbytes: 500})

	opts := DQLExecuteOptions{DisableGuardrails: true}
	applyGuardrailDefaults(&opts)
	if opts.DefaultScanLimitGbytes != 0 {
		t.Errorf("DisableGuardrails ignored: %+v", opts)
	}
}

func TestApplyGuardrailDefaults_LeavesMaxRecordsZero(t *testing.T) {
	// Client-side should NOT auto-inject MaxResultRecords — server default wins.
	saved := DefaultGuardrails
	defer SetDefaultGuardrails(saved)
	SetDefaultGuardrails(GuardrailDefaults{ScanLimitGbytes: 500, MaxResultRecords: 0})

	opts := DQLExecuteOptions{}
	applyGuardrailDefaults(&opts)
	if opts.MaxResultRecords != 0 {
		t.Errorf("MaxResultRecords should stay zero: got %v", opts.MaxResultRecords)
	}
}

func TestApplyGuardrailDefaults_NilSafe(t *testing.T) {
	applyGuardrailDefaults(nil) // must not panic
}
