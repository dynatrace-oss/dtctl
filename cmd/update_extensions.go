package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtctl/pkg/output"
	"github.com/dynatrace-oss/dtctl/pkg/resources/extension"
	"github.com/dynatrace-oss/dtctl/pkg/safety"
)

// updateExtensionCmd activates a version of an Extensions 2.0 extension as the
// environment-wide active version.
var updateExtensionCmd = &cobra.Command{
	Use:     "extension <name>",
	Aliases: []string{"ext"},
	Short:   "Activate a version of an Extensions 2.0 extension",
	Long: `Activate a specific version of an Extensions 2.0 extension as the
environment-wide active version.

Exactly one of --version, --latest, or --hub-latest must be provided:

  --version <ver>   Activate a specific already-uploaded version.
  --latest          Pick the highest installed version and activate it.
  --hub-latest      Install the newest release from the Hub catalog (if not
                    already present) and then activate it.

When --with-configurations is set, every existing monitoring configuration for
the extension is re-PUT against the API after version activation, causing the
API to validate each configuration against the new version's schema.

Examples:
  # Activate a specific installed version
  dtctl update extension com.dynatrace.extension.host-monitoring --version 1.2.3

  # Activate the highest installed version
  dtctl update extension com.dynatrace.extension.host-monitoring --latest

  # Install the latest Hub release and activate it
  dtctl update extension com.dynatrace.extension.host-monitoring --hub-latest

  # Upgrade to latest and refresh monitoring configurations
  dtctl update extension com.dynatrace.extension.host-monitoring --latest --with-configurations

  # Preview what would happen
  dtctl update extension com.dynatrace.extension.host-monitoring --latest --dry-run
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runUpdateOneExtension(cmd, args[0])
	},
}

// updateExtensionsCmd bulk-upgrades all installed Extensions 2.0 extensions.
var updateExtensionsCmd = &cobra.Command{
	Use:   "extensions",
	Short: "Bulk-upgrade all installed Extensions 2.0 extensions",
	Long: `Upgrade all Extensions 2.0 extensions installed in the Dynatrace environment.

--all is required to prevent accidental bulk mutations.

One of --latest or --hub-latest must be provided to select the target version:

  --latest      Activate the highest already-installed version for each extension.
  --hub-latest  Install the newest Hub release (if not present) and activate it.

When --with-configurations is set, each extension's monitoring configurations
are re-PUT after version activation to validate them against the new schema.

Failures for individual extensions are reported but do not abort the bulk run;
the command exits non-zero if any extension could not be upgraded.

Examples:
  # Upgrade all extensions to their highest installed versions
  dtctl update extensions --all --latest

  # Upgrade all extensions from the Hub and refresh their monitoring configs
  dtctl update extensions --all --hub-latest --with-configurations

  # Preview what would happen
  dtctl update extensions --all --latest --dry-run
`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		all, _ := cmd.Flags().GetBool("all")
		if !all {
			return fmt.Errorf("--all is required to prevent accidental bulk mutations; run with --all to confirm")
		}

		if dryRun {
			fmt.Println("Dry run: would upgrade all installed extensions")
			return nil
		}

		_, c, err := SetupWithSafety(safety.OperationUpdate)
		if err != nil {
			return err
		}

		handler := extension.NewHandler(c)
		list, err := handler.List(cmd.Context(), "", 0)
		if err != nil {
			return fmt.Errorf("list extensions: %w", err)
		}

		if len(list.Items) == 0 {
			output.PrintInfo("No extensions installed.")
			return nil
		}

		withConfigs, _ := cmd.Flags().GetBool("with-configurations")
		hubLatest, _ := cmd.Flags().GetBool("hub-latest")
		latest, _ := cmd.Flags().GetBool("latest")

		// Validate flags (same rules as for a single extension, sans --version)
		if !latest && !hubLatest {
			return fmt.Errorf("one of --latest or --hub-latest is required")
		}
		if latest && hubLatest {
			return fmt.Errorf("--latest and --hub-latest are mutually exclusive")
		}

		var (
			upgraded int
			failed   int
		)

		for _, ext := range list.Items {
			name := ext.ExtensionName
			activatedVersion, err := resolveAndActivate(handler, name, "", latest, hubLatest, withConfigs)
			if err != nil {
				output.PrintHumanError("  %-60s  FAILED: %v", name, err)
				failed++
				continue
			}

			output.PrintInfo("  %-60s  → %s", name, activatedVersion)
			upgraded++
		}

		output.PrintSuccess("Upgraded %d extension(s), %d failed", upgraded, failed)
		if failed > 0 {
			return fmt.Errorf("%d extension(s) could not be upgraded", failed)
		}
		return nil
	},
}

// runUpdateOneExtension is the RunE body for updateExtensionCmd, also reused by
// the bulk command.
func runUpdateOneExtension(cmd *cobra.Command, name string) error {
	version, _ := cmd.Flags().GetString("version")
	latest, _ := cmd.Flags().GetBool("latest")
	hubLatest, _ := cmd.Flags().GetBool("hub-latest")
	withConfigs, _ := cmd.Flags().GetBool("with-configurations")

	// Validate: exactly one version source must be set.
	set := 0
	if version != "" {
		set++
	}
	if latest {
		set++
	}
	if hubLatest {
		set++
	}
	if set == 0 {
		return fmt.Errorf("one of --version, --latest, or --hub-latest is required")
	}
	if set > 1 {
		return fmt.Errorf("--version, --latest, and --hub-latest are mutually exclusive")
	}

	if dryRun {
		switch {
		case version != "":
			fmt.Printf("Dry run: would activate extension %q version %s\n", name, version)
		case latest:
			fmt.Printf("Dry run: would activate the highest installed version of %q\n", name)
		case hubLatest:
			fmt.Printf("Dry run: would install the latest Hub release of %q and activate it\n", name)
		}
		return nil
	}

	_, c, err := SetupWithSafety(safety.OperationUpdate)
	if err != nil {
		return err
	}

	handler := extension.NewHandler(c)

	activatedVersion, err := resolveAndActivate(handler, name, version, latest, hubLatest, withConfigs)
	if err != nil {
		return err
	}

	output.PrintSuccess("Extension %q activated", name)
	output.PrintInfo("  Active version: %s", activatedVersion)
	return nil
}

// resolveAndActivate determines the target version (from flag values), handles
// monitoring configuration updates in the correct order (configs-first for
// downgrades, activation-first for upgrades), and returns the activated version.
func resolveAndActivate(handler *extension.Handler, name, version string, latest, hubLatest, withConfigs bool) (string, error) {
	// Step 1: resolve the target version string.
	switch {
	case version != "":
		// Use the caller-supplied version directly.

	case latest:
		v, err := handler.LatestInstalledVersion(name)
		if err != nil {
			return "", fmt.Errorf("resolve latest installed version for %q: %w", name, err)
		}
		version = v

	case hubLatest:
		// Install the latest Hub release (empty version = Hub picks latest).
		result, err := handler.InstallFromHub(name, "")
		if err != nil {
			return "", fmt.Errorf("install Hub-latest for %q: %w", name, err)
		}
		version = result.Version
		// The Hub API may return an empty version string when the extension is
		// already at the latest available release. Fall back to the highest
		// installed version in that case.
		if version == "" {
			v, err := handler.LatestInstalledVersion(name)
			if err != nil {
				return "", fmt.Errorf("resolve installed version after Hub install for %q: %w", name, err)
			}
			version = v
		}
	}

	// Step 2 (downgrade path): when --with-configurations and the target version
	// is lower than the current active version, the API refuses to activate
	// because monitoring configs reference the higher version. Update configs to
	// the target version first so the activation succeeds.
	if withConfigs {
		currentActive, err := handler.GetActiveVersion(name)
		if err == nil && extension.SemverGreater(currentActive, version) {
			// Downgrade: configs must be updated before activation.
			if cfgErr := refreshMonitoringConfigurations(handler, name, version); cfgErr != nil {
				return "", fmt.Errorf("update monitoring configurations before downgrade: %w", cfgErr)
			}
		}
	}

	// Step 3: activate the target version.
	if _, err := handler.ActivateVersion(name, version); err != nil {
		return "", fmt.Errorf("activate %q version %s: %w", name, version, err)
	}

	// Step 4 (upgrade path): update configs after activation.
	if withConfigs {
		currentActive, err := handler.GetActiveVersion(name)
		if err == nil && !extension.SemverGreater(currentActive, version) {
			// Upgrade or same version: configs updated after activation.
			if cfgErr := refreshMonitoringConfigurations(handler, name, version); cfgErr != nil {
				output.PrintWarning("version activated but monitoring configuration refresh failed: %v", cfgErr)
			}
		}
	}

	return version, nil
}

// refreshMonitoringConfigurations re-PUTs every monitoring configuration for the
// given extension, updating the "version" field inside each config's value to
// newVersion. This causes the API to validate each configuration against the new
// active version's schema. Per-config errors are aggregated and returned as a
// single error; the caller decides whether to treat this as fatal.
func refreshMonitoringConfigurations(handler *extension.Handler, extensionName, newVersion string) error {
	configs, err := handler.ListMonitoringConfigurations(extensionName, "", 0)
	if err != nil {
		return fmt.Errorf("list monitoring configurations: %w", err)
	}

	var errs []string
	for _, cfg := range configs.Items {
		// Unmarshal the raw JSON value into a map so it can be re-sent via
		// UpdateMonitoringConfiguration (which takes map[string]any).
		var valueMap map[string]any
		if len(cfg.Value) > 0 {
			if err := json.Unmarshal(cfg.Value, &valueMap); err != nil {
				errs = append(errs, fmt.Sprintf("  config %s: parse value: %v", cfg.ObjectID, err))
				continue
			}
		}
		// Update the "version" field inside the value to the new active version
		// so the API validates the config against the correct schema.
		if valueMap == nil {
			valueMap = make(map[string]any)
		}
		valueMap["version"] = newVersion
		body := extension.MonitoringConfigurationCreate{
			Scope: cfg.Scope,
			Value: valueMap,
		}
		if _, err := handler.UpdateMonitoringConfiguration(extensionName, cfg.ObjectID, body); err != nil {
			errs = append(errs, fmt.Sprintf("  config %s: %v", cfg.ObjectID, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%d configuration(s) failed to refresh:\n%s", len(errs), joinErrors(errs))
	}
	return nil
}


// joinErrors concatenates error strings with newlines.
func joinErrors(errs []string) string {
	result := ""
	for i, e := range errs {
		if i > 0 {
			result += "\n"
		}
		result += e
	}
	return result
}

func init() {
	updateCmd.AddCommand(updateExtensionCmd)
	updateCmd.AddCommand(updateExtensionsCmd)

	// Flags for a single extension
	updateExtensionCmd.Flags().String("version", "", "specific version to activate (already uploaded to the environment)")
	updateExtensionCmd.Flags().Bool("latest", false, "activate the highest installed version")
	updateExtensionCmd.Flags().Bool("hub-latest", false, "install the latest Hub release and activate it")
	updateExtensionCmd.Flags().Bool("with-configurations", false, "re-validate monitoring configurations against the new version after activation")

	// Flags for bulk upgrade
	updateExtensionsCmd.Flags().Bool("all", false, "upgrade all installed extensions (required to prevent accidental bulk mutations)")
	updateExtensionsCmd.Flags().Bool("latest", false, "activate the highest installed version for each extension")
	updateExtensionsCmd.Flags().Bool("hub-latest", false, "install the latest Hub release for each extension and activate it")
	updateExtensionsCmd.Flags().Bool("with-configurations", false, "re-validate monitoring configurations against the new version after activation")
}
