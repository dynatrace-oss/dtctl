# Extensions Resource

## List
```bash
dtctl get extensions                                      # List all extensions (shows extension name and active version)
dtctl get extensions -o json                              # Output as JSON
```

## Get Versions
```bash
dtctl get extensions com.dynatrace.extension.host-monitoring # Get all versions of a specific extension
```

## Describe
```bash
dtctl describe extension com.dynatrace.extension.host-monitoring         # Show detailed info (active version by default)
dtctl describe extension com.dynatrace.extension.host-monitoring 1.2.3   # Show details for a specific version
dtctl describe extension com.dynatrace.extension.host-monitoring -o json # Output as JSON
```

## Get Monitoring Configurations
```bash
dtctl get extension-configs com.dynatrace.extension.host-monitoring                          # List monitoring configurations for an extension
dtctl get extension-config com.dynatrace.extension.host-monitoring --config-id <config-id>   # Get a specific monitoring configuration by ID
```

## Upgrade / Activate a Version

```bash
# Activate a specific already-uploaded version
dtctl update extension com.dynatrace.extension.host-monitoring --version 1.2.3

# Activate the highest installed version
dtctl update extension com.dynatrace.extension.host-monitoring --latest

# Install the latest Hub release and activate it
dtctl update extension com.dynatrace.extension.host-monitoring --hub-latest

# Upgrade + re-validate all monitoring configurations against the new schema
dtctl update extension com.dynatrace.extension.host-monitoring --hub-latest --with-configurations

# Preview what would happen without making changes
dtctl update extension com.dynatrace.extension.host-monitoring --latest --dry-run

# Bulk-upgrade all installed extensions to their highest installed versions
dtctl update extensions --all --latest

# Bulk-upgrade from Hub and refresh monitoring configurations
dtctl update extensions --all --hub-latest --with-configurations --dry-run
```

## Apply Monitoring Configuration
```bash
dtctl apply extension-config com.dynatrace.extension.host-monitoring -f config.yaml                    # Create new (no objectId in file)
dtctl apply extension-config com.dynatrace.extension.host-monitoring -f config.yaml --scope HOST-1234  # Create with scope
dtctl apply extension-config com.dynatrace.extension.host-monitoring -f config.yaml                    # Update existing (objectId in file)
dtctl apply extension-config com.dynatrace.extension.host-monitoring -f config.yaml --set env=prod     # Apply with template variables
dtctl apply extension-config com.dynatrace.extension.host-monitoring -f config.yaml --dry-run          # Dry run
```
