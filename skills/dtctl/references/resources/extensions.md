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

## Apply Monitoring Configuration
```bash
dtctl apply extension-config com.dynatrace.extension.host-monitoring -f config.yaml                    # Create new (no objectId in file)
dtctl apply extension-config com.dynatrace.extension.host-monitoring -f config.yaml --scope HOST-1234  # Create with scope
dtctl apply extension-config com.dynatrace.extension.host-monitoring -f config.yaml                    # Update existing (objectId in file)
dtctl apply extension-config com.dynatrace.extension.host-monitoring -f config.yaml --set env=prod     # Apply with template variables
dtctl apply extension-config com.dynatrace.extension.host-monitoring -f config.yaml --dry-run          # Dry run
```
