package cmd

import "testing"

func TestActivateCommandRegistration(t *testing.T) {
	activateCmd, _, err := rootCmd.Find([]string{"activate"})
	if err != nil {
		t.Fatalf("expected activate command to exist, got error: %v", err)
	}
	if activateCmd == nil || activateCmd.Name() != "activate" {
		t.Fatalf("expected activate command to exist")
	}
}

func TestActivateGCPMonitoringCommandRegistration(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"activate", "gcp", "monitoring"})
	if err != nil {
		t.Fatalf("expected activate gcp monitoring command to exist, got error: %v", err)
	}
	if cmd == nil || cmd.Name() != "monitoring" {
		t.Fatalf("expected activate gcp monitoring command to exist")
	}
}

func TestActivateGCPMonitoringFlags(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"activate", "gcp", "monitoring"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, flag := range []string{"name", "serviceAccountId"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag --%s to be registered on activate gcp monitoring", flag)
		}
	}
}

func TestActivateAzureMonitoringCommandRegistration(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"activate", "azure", "monitoring"})
	if err != nil {
		t.Fatalf("expected activate azure monitoring command to exist, got error: %v", err)
	}
	if cmd == nil || cmd.Name() != "monitoring" {
		t.Fatalf("expected activate azure monitoring command to exist")
	}
}

func TestActivateAzureMonitoringFlags(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"activate", "azure", "monitoring"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, flag := range []string{"name", "directoryId", "applicationId"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag --%s to be registered on activate azure monitoring", flag)
		}
	}
}
