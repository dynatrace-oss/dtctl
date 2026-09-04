# TOKEN_SCOPES

Generated per-safety-level scope reference, from `resource_scopes`.

## delete

| Resource | Scope |
| --- | --- |
| app | `app-engine:apps:delete` |
| breakpoint | `dev-obs:breakpoints:set` |
| dashboard | `document:documents:delete` |
| document | `document:documents:delete` |
| edgeconnect | `app-engine:edge-connects:delete` |
| lookup | `storage:files:delete` |
| notebook | `document:documents:delete` |
| segment | `storage:filter-segments:delete` |
| trash | `document:trash.documents:delete` |

## read

| Resource | Scope |
| --- | --- |
| analyzer | `davis:analyzers:read` |
| anomaly-detector | `settings:objects:read` |
| app | `app-engine:apps:run` |
| aws | `extensions:configurations:read` |
| aws | `settings:objects:read` |
| azure | `extensions:configurations:read` |
| azure | `settings:objects:read` |
| breakpoint | `dev-obs:breakpoints:set` |
| bucket | `storage:buckets:read` |
| classic-pipelines | `settings:objects:read` |
| copilot-skill | `davis-copilot:conversations:execute` |
| dashboard | `document:documents:read` |
| document | `document:documents:read` |
| edgeconnect | `app-engine:edge-connects:read` |
| extension | `extensions:definitions:read` |
| extension-config | `extensions:configurations:read` |
| function | `app-engine:apps:run` |
| gcp | `extensions:configurations:read` |
| gcp | `settings:objects:read` |
| group | `iam:groups:read` |
| hub-extension | `hub:catalog:read` |
| hub-extension-release | `hub:catalog:read` |
| intent | `app-engine:apps:run` |
| lookup | `storage:files:read` |
| lql-to-dql | `openpipeline:configurations:read` |
| notebook | `document:documents:read` |
| notification | `notification:notifications:read` |
| openpipeline-dql-processor | `openpipeline:configurations:read` |
| openpipeline-matcher | `openpipeline:configurations:read` |
| preview-processor | `openpipeline:configurations:read` |
| sdk-version | `app-engine:apps:run` |
| segment | `storage:filter-segments:read` |
| setting | `app-settings:objects:read` |
| setting | `settings:objects:read` |
| settings-schema | `settings:schemas:read` |
| slo | `slo:objective-templates:read` |
| slo | `slo:slos:read` |
| slo-template | `slo:objective-templates:read` |
| snapshot | `dev-obs:breakpoints:set` |
| trash | `document:trash.documents:read` |
| user | `iam:users:read` |
| wfe-task-result | `automation:workflows:read` |
| workflow | `automation:workflows:read` |
| workflow-execution | `automation:workflows:read` |

## run

| Resource | Scope |
| --- | --- |
| analyzer | `davis:analyzers:execute` |
| copilot | `davis-copilot:conversations:execute` |
| function | `app-engine:functions:run` |
| workflow | `automation:workflows:run` |

## write

| Resource | Scope |
| --- | --- |
| anomaly-detector | `settings:objects:write` |
| app | `app-engine:apps:install` |
| aws | `extensions:configurations:write` |
| aws | `settings:objects:write` |
| azure | `extensions:configurations:write` |
| azure | `settings:objects:write` |
| breakpoint | `dev-obs:breakpoints:set` |
| bucket | `storage:buckets:write` |
| dashboard | `document:documents:write` |
| document | `document:documents:write` |
| edgeconnect | `app-engine:edge-connects:write` |
| extension | `extensions:definitions:write` |
| extension-config | `extensions:configurations:write` |
| gcp | `extensions:configurations:write` |
| gcp | `settings:objects:write` |
| lookup | `storage:files:write` |
| notebook | `document:documents:write` |
| notification | `notification:notifications:write` |
| segment | `storage:filter-segments:write` |
| setting | `settings:objects:write` |
| slo | `slo:slos:write` |
| trash | `document:trash.documents:restore` |
| workflow | `automation:workflows:write` |

