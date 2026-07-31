package cmd

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/safety"
	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

func TestAccountServiceUserCommandsRegistered(t *testing.T) {
	tests := []struct {
		name   string
		parent interface {
			Find([]string) (*cobra.Command, []string, error)
		}
	}{
		{name: "list", parent: accountListCmd},
		{name: "create", parent: accountCreateCmd},
		{name: "delete", parent: accountDeleteCmd},
	}
	for _, tt := range tests {
		command, _, err := tt.parent.Find([]string{"service-user"})
		if err != nil || command == nil || command.Name() != "service-user" {
			t.Errorf("%s registration: command=%v err=%v", tt.name, command, err)
			continue
		}
		if len(command.Aliases) != 1 || command.Aliases[0] != "service-users" {
			t.Errorf("%s aliases = %v", command.CommandPath(), command.Aliases)
		}
	}
}

func TestCreateServiceUserRequiresName(t *testing.T) {
	flag := accountCreateServiceUserCmd.Flags().Lookup("name")
	old := flag.Value.String()
	t.Cleanup(func() { _ = accountCreateServiceUserCmd.Flags().Set("name", old) })
	_ = accountCreateServiceUserCmd.Flags().Set("name", "")

	err := accountCreateServiceUserCmd.RunE(accountCreateServiceUserCmd, nil)
	if err == nil || err.Error() != "--name is required" {
		t.Errorf("error = %v, want --name is required", err)
	}
}

func TestDeleteServiceUserRequiresExactlyOneUUID(t *testing.T) {
	if err := accountDeleteServiceUserCmd.Args(accountDeleteServiceUserCmd, nil); err == nil {
		t.Error("expected missing argument error")
	}
	if err := accountDeleteServiceUserCmd.Args(accountDeleteServiceUserCmd, []string{"one", "two"}); err == nil {
		t.Error("expected too many arguments error")
	}
	if err := accountDeleteServiceUserCmd.Args(accountDeleteServiceUserCmd, []string{"00000000-0000-4000-8000-000000000101"}); err != nil {
		t.Errorf("valid UUID argument rejected: %v", err)
	}
}

func TestCreateServiceUserDryRunSkipsSetup(t *testing.T) {
	oldDryRun := dryRun
	oldSetup := setupServiceUserAccountWithSafety
	nameFlag := accountCreateServiceUserCmd.Flags().Lookup("name")
	descriptionFlag := accountCreateServiceUserCmd.Flags().Lookup("description")
	oldName, oldDescription := nameFlag.Value.String(), descriptionFlag.Value.String()
	t.Cleanup(func() {
		dryRun = oldDryRun
		setupServiceUserAccountWithSafety = oldSetup
		_ = accountCreateServiceUserCmd.Flags().Set("name", oldName)
		_ = accountCreateServiceUserCmd.Flags().Set("description", oldDescription)
	})

	dryRun = true
	_ = accountCreateServiceUserCmd.Flags().Set("name", "automation-alpha")
	_ = accountCreateServiceUserCmd.Flags().Set("description", "Synthetic integration identity")
	called := false
	setupServiceUserAccountWithSafety = func(safety.Operation) (*httpclient.Client, string, error) {
		called = true
		return nil, "", errors.New("setup must not run")
	}

	if err := accountCreateServiceUserCmd.RunE(accountCreateServiceUserCmd, nil); err != nil {
		t.Fatalf("RunE() error: %v", err)
	}
	if called {
		t.Fatal("dry-run loaded account credentials or ran safety setup")
	}
}

func TestDeleteServiceUserDryRunSkipsSetup(t *testing.T) {
	oldDryRun := dryRun
	oldSetup := setupServiceUserAccountWithSafety
	t.Cleanup(func() {
		dryRun = oldDryRun
		setupServiceUserAccountWithSafety = oldSetup
	})

	dryRun = true
	called := false
	setupServiceUserAccountWithSafety = func(safety.Operation) (*httpclient.Client, string, error) {
		called = true
		return nil, "", errors.New("setup must not run")
	}

	if err := accountDeleteServiceUserCmd.RunE(accountDeleteServiceUserCmd, []string{"00000000-0000-4000-8000-000000000101"}); err != nil {
		t.Fatalf("RunE() error: %v", err)
	}
	if called {
		t.Fatal("dry-run loaded account credentials or ran safety setup")
	}
}

func TestCreateServiceUserUsesCreateSafetyOperation(t *testing.T) {
	oldDryRun := dryRun
	oldSetup := setupServiceUserAccountWithSafety
	nameFlag := accountCreateServiceUserCmd.Flags().Lookup("name")
	oldName := nameFlag.Value.String()
	t.Cleanup(func() {
		dryRun = oldDryRun
		setupServiceUserAccountWithSafety = oldSetup
		_ = accountCreateServiceUserCmd.Flags().Set("name", oldName)
	})

	dryRun = false
	_ = accountCreateServiceUserCmd.Flags().Set("name", "automation-alpha")
	stop := errors.New("stop after safety setup")
	var got safety.Operation
	setupServiceUserAccountWithSafety = func(op safety.Operation) (*httpclient.Client, string, error) {
		got = op
		return nil, "", stop
	}

	if err := accountCreateServiceUserCmd.RunE(accountCreateServiceUserCmd, nil); !errors.Is(err, stop) {
		t.Fatalf("RunE() error = %v, want %v", err, stop)
	}
	if got != safety.OperationCreate {
		t.Errorf("safety operation = %v, want %v", got, safety.OperationCreate)
	}
}

func TestDeleteServiceUserUsesDeleteSafetyOperation(t *testing.T) {
	oldDryRun := dryRun
	oldSetup := setupServiceUserAccountWithSafety
	t.Cleanup(func() {
		dryRun = oldDryRun
		setupServiceUserAccountWithSafety = oldSetup
	})

	dryRun = false
	stop := errors.New("stop after safety setup")
	var got safety.Operation
	setupServiceUserAccountWithSafety = func(op safety.Operation) (*httpclient.Client, string, error) {
		got = op
		return nil, "", stop
	}

	err := accountDeleteServiceUserCmd.RunE(accountDeleteServiceUserCmd, []string{"00000000-0000-4000-8000-000000000101"})
	if !errors.Is(err, stop) {
		t.Fatalf("RunE() error = %v, want %v", err, stop)
	}
	if got != safety.OperationDelete {
		t.Errorf("safety operation = %v, want %v", got, safety.OperationDelete)
	}
}

func TestListServiceUserRejectsArguments(t *testing.T) {
	if err := accountListServiceUserCmd.Args(accountListServiceUserCmd, []string{"unexpected"}); err == nil {
		t.Error("expected positional argument validation error")
	}
}
