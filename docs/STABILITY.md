# dtctl stability manifest

<!-- GENERATED FILE — do not edit. Regenerate with: make stability-manifest -->

Every command and flag dtctl exposes, with the contract it carries. This file is
generated from the live command tree and checked in, so a change to the
contract shows up as a reviewable diff and a failing build rather than as a
silent break.

| Tier | Promise |
|---|---|
| `stable` | Invocation and output contract are additive-only. Removal or an incompatible change requires a deprecation cycle. |
| `experimental` | May change or be removed in any release. Not covered by dtctl's stability guarantees. |
| `development` | Unfinished, no guarantees. Not registered unless opted in via `dtctl config set development.<feature> on`. |

Deprecation is not a tier: a deprecated command is still stable in shape and is
merely scheduled for removal. It is recorded on the same line.

## Choosing what this environment accepts

The **stability floor** is the weakest contract a command or flag may offer and
still be usable. It defaults to `experimental`, so interactive use keeps
getting new (badged) surface. Automation should pin `stable`:

```bash
# Per context, persisted in the config file
dtctl config set-context prod-agent --min-stability stable

# Per process — this is what an embedding service sets, and it is deliberately
# not a command-line flag: the embedder controls argv, so a flag would hand the
# opt-in to exactly the party the floor exists to constrain.
DTCTL_MIN_STABILITY=stable dtctl get workflows
```

A single below-floor command or flag can be admitted without lowering the floor
for everything. Each entry is one audited risk acceptance:

```bash
dtctl config set-context prod-agent --min-stability stable \
  --stability-exception 'ingest' \
  --stability-exception 'query --spill'
```

Development-tier features are enabled individually and never by a floor:

```bash
dtctl config list-development                 # what this build carries
dtctl config set development.serve on         # persistent
DTCTL_DEVELOPMENT=serve dtctl serve http      # one process
```

> **Versioning caveat.** `release-please-config.json` sets
> `bump-minor-pre-major`, so pre-1.0 a breaking change currently ships as a
> minor bump. The `stable` tier is only as strong as the carve-out that
> exempts it from that policy — see the design doc's "Versioning policy"
> section. Until that is written down, treat this file as the inventory, not
> yet as the guarantee.

## Summary

- commands: 268 stable, 0 experimental, 11 development
- entries below (commands + flags): 766

## Surface

```
account                              development  (opt-in key: account)
account create                       development
account create token                 development
  --expires                          development
  --expires-at                       development
  --name                             development
  --resource                         development
  --scope                            development
  --tag                              development
  --user-uuid                        development
account delete                       development
account delete token                 development
account list                         development
account list token                   development
account login                        development
  --account-uuid                     development
  --timeout                          development
account status                       development
alias                                stable
alias delete                         stable
alias export                         stable
  --file                             stable
alias import                         stable
  --file                             stable
  --overwrite                        stable
alias list                           stable
alias set                            stable
apply                                stable
  --create-snapshot                  stable
  --dry-run                          stable
  --file                             stable
  --id                               stable
  --label                            stable
  --no-hooks                         stable
  --set                              stable
  --share-environment                stable
  --show-diff                        stable
  --snapshot-description             stable
  --type                             stable
  --write-id                         stable
apply extension-config               stable
  --file                             stable
  --scope                            stable
  --set                              stable
auth                                 stable
auth login                           stable
  --context                          stable
  --environment                      stable
  --safety-level                     stable
  --timeout                          stable
  --token-name                       stable
auth logout                          stable
  --remove-context                   stable
auth refresh                         stable
auth status                          stable
auth whoami                          stable
  --id-only                          stable
  --refresh                          stable
commands                             stable
  --brief                            stable
  --full                             stable
  --required-scopes                  stable
commands howto                       stable
completion                           stable
config                               stable
config current-context               stable
config delete-context                stable
config describe-context              stable
config get-contexts                  stable
config init                          stable
  --context                          stable
  --force                            stable
config list-development              stable
config migrate-tokens                stable
config set                           stable
config set-context                   stable
  --description                      stable
  --environment                      stable
  --min-stability                    stable
  --profile                          stable
  --safety-level                     stable
  --stability-exception              stable
  --token-ref                        stable
config set-credentials               stable
  --token                            stable
config use-context                   stable
config view                          stable
create                               stable
create anomaly-detector              stable
  --file                             stable
  --set                              stable
create aws                           stable
create aws connection                stable
  --name                             stable
  --roleArn                          stable
create aws monitoring                stable
  --credentials                      stable
  --featureSets                      stable
  --name                             stable
  --regions                          stable
create azure                         stable
create azure connection              stable
  --applicationId                    stable
  --clientSecret                     stable
  --directoryId                      stable
  --issuer                           stable
  --name                             stable
  --type                             stable
create azure monitoring              stable
  --credentials                      stable
  --featureSets                      stable
  --featuresets                      stable
  --locationFiltering                stable
  --name                             stable
create breakpoint                    stable
  --filters                          stable
  --yes                              stable
create bucket                        stable
  --display-name                     stable
  --file                             stable
  --name                             stable
  --retention                        stable
  --table                            stable
create dashboard                     stable
  --description                      stable
  --file                             stable
  --id                               stable
  --name                             stable
  --set                              stable
create document                      stable
  --description                      stable
  --file                             stable
  --id                               stable
  --label                            stable
  --name                             stable
  --set                              stable
  --type                             stable
create edgeconnect                   stable
  --file                             stable
  --host-patterns                    stable
  --name                             stable
create extension                     stable
  --file                             stable
  --hub-extension                    stable
  --version                          stable
create gcp                           stable
create gcp connection                stable
  --name                             stable
  --serviceAccountId                 stable
  --serviceaccountid                 stable
create gcp monitoring                stable
  --credentials                      stable
  --featureSets                      stable
  --featuresets                      stable
  --locationFiltering                stable
  --name                             stable
create lookup                        stable
  --description                      stable
  --display-name                     stable
  --file                             stable
  --locale                           stable
  --lookup-field                     stable
  --parse-pattern                    stable
  --path                             stable
  --skip-records                     stable
  --timezone                         stable
create notebook                      stable
  --description                      stable
  --file                             stable
  --id                               stable
  --name                             stable
  --set                              stable
create segment                       stable
  --file                             stable
create settings                      stable
  --file                             stable
  --schema                           stable
  --scope                            stable
  --set                              stable
  --validate-only                    stable
create slo                           stable
  --file                             stable
  --set                              stable
create workflow                      stable
  --file                             stable
  --set                              stable
ctx                                  stable
ctx current                          stable
ctx delete                           stable
ctx describe                         stable
ctx set                              stable
  --description                      stable
  --environment                      stable
  --min-stability                    stable
  --profile                          stable
  --safety-level                     stable
  --stability-exception              stable
  --token-ref                        stable
ctx token                            stable
delete                               stable
delete anomaly-detector              stable
  --yes                              stable
delete app                           stable
  --yes                              stable
delete aws                           stable
delete aws connection                stable
delete aws monitoring                stable
delete azure                         stable
delete azure connection              stable
delete azure monitoring              stable
delete breakpoint                    stable
  --all                              stable
  --yes                              stable
delete bucket                        stable
  --confirm                          stable
  --yes                              stable
delete dashboard                     stable
  --yes                              stable
delete document                      stable
  --yes                              stable
delete edgeconnect                   stable
  --yes                              stable
delete gcp                           stable
delete gcp connection                stable
delete gcp monitoring                stable
delete lookup                        stable
  --yes                              stable
delete notebook                      stable
  --yes                              stable
delete notification                  stable
  --yes                              stable
delete segment                       stable
  --confirm                          stable
  --yes                              stable
delete settings                      stable
  --yes                              stable
delete slo                           stable
  --yes                              stable
delete trash                         stable
  --permanent                        stable
  --yes                              stable
delete workflow                      stable
  --yes                              stable
describe                             stable
describe analyzer                    stable
  --doc                              stable
describe anomaly-detector            stable
describe api                         stable
  --operation                        stable
  --raw                              stable
  --spill                            stable
  --spill-format                     stable
  --spill-threshold                  stable
  --spill-to                         stable
describe app                         stable
describe aws                         stable
describe aws connection              stable
describe aws monitoring              stable
describe azure                       stable
describe azure connection            stable
describe azure monitoring            stable
describe breakpoint                  stable
describe bucket                      stable
describe dashboard                   stable
describe document                    stable
describe edgeconnect                 stable
describe extension                   stable
  --active-gate-groups               stable
  --assets                           stable
  --feature-set-metrics              stable
  --full                             stable
  --monitoring-configuration-schema  stable
  --no-fluff                         stable
  --version                          stable
describe extension-config            stable
  --config-id                        stable
describe function                    stable
describe gcp                         stable
describe gcp connection              stable
describe gcp monitoring              stable
describe group                       stable
describe hub-extensions              stable
describe intent                      stable
describe lookup                      stable
describe notebook                    stable
describe segment                     stable
describe settings                    stable
describe settings-schema             stable
describe slo                         stable
describe trash                       stable
describe user                        stable
describe workflow                    stable
describe workflow-execution          stable
diff                                 stable
  --color                            stable
  --context                          stable
  --file                             stable
  --format                           stable
  --ignore-metadata                  stable
  --ignore-order                     stable
  --output                           stable
  --quiet                            stable
  --semantic                         stable
  --side-by-side                     stable
disable                              stable
disable aws                          stable
disable aws monitoring               stable
  --name                             stable
disable azure                        stable
disable azure monitoring             stable
  --name                             stable
disable gcp                          stable
disable gcp monitoring               stable
  --name                             stable
doctor                               stable
download                             stable
download extension                   stable
  --version                          stable
edit                                 stable
edit anomaly-detector                stable
edit aws                             stable
edit aws monitoring                  stable
  --format                           stable
  --name                             stable
edit azure                           stable
edit azure monitoring                stable
  --format                           stable
  --name                             stable
edit dashboard                       stable
  --create-snapshot                  stable
  --format                           stable
  --snapshot-description             stable
edit document                        stable
  --create-snapshot                  stable
  --format                           stable
  --snapshot-description             stable
edit gcp                             stable
edit gcp monitoring                  stable
  --format                           stable
  --name                             stable
edit notebook                        stable
  --create-snapshot                  stable
  --format                           stable
  --snapshot-description             stable
edit segment                         stable
  --format                           stable
edit setting                         stable
  --format                           stable
  --validate-only                    stable
edit workflow                        stable
  --format                           stable
enable                               stable
enable aws                           stable
enable aws monitoring                stable
  --name                             stable
  --roleArn                          stable
enable azure                         stable
enable azure monitoring              stable
  --applicationId                    stable
  --directoryId                      stable
  --name                             stable
enable gcp                           stable
enable gcp monitoring                stable
  --name                             stable
  --serviceAccountId                 stable
exec                                 stable
exec analyzer                        stable
  --file                             stable
  --input                            stable
  --query                            stable
  --timeout                          stable
  --validate                         stable
  --wait                             stable
exec copilot                         stable
  --context                          stable
  --file                             stable
  --instruction                      stable
  --no-docs                          stable
  --stream                           stable
exec copilot document-search         stable
  --collections                      stable
  --exclude                          stable
exec copilot dql2nl                  stable
  --file                             stable
exec copilot nl2dql                  stable
  --file                             stable
exec function                        stable
  --code                             stable
  --data                             stable
  --defer                            stable
  --file                             stable
  --method                           stable
  --payload                          stable
exec preview-processor               stable
  --config-id                        stable
  --file                             stable
exec slo                             stable
  --timeout                          stable
exec workflow                        stable
  --input                            stable
  --params                           stable
  --show-results                     stable
  --timeout                          stable
  --wait                             stable
find                                 stable
find intents                         stable
  --data                             stable
  --data-file                        stable
  --limit                            stable
get                                  stable
get analyzers                        stable
  --filter                           stable
get anomaly-detectors                stable
  --enabled                          stable
get apis                             stable
  --ops-count                        stable
  --uncovered                        stable
get apps                             stable
get aws                              stable
get aws connections                  stable
get aws monitoring                   stable
get aws monitoring-feature-sets      stable
get aws monitoring-regions           stable
get azure                            stable
get azure connections                stable
get azure monitoring                 stable
get azure monitoring-feature-sets    stable
get azure monitoring-locations       stable
get breakpoints                      stable
get buckets                          stable
get copilot-skills                   stable
get dashboards                       stable
  --add-fields                       stable
  --admin-access                     stable
  --filter                           stable
  --interval                         stable
  --mine                             stable
  --name                             stable
  --sort                             stable
  --watch                            stable
  --watch-only                       stable
get documents                        stable
  --add-fields                       stable
  --admin-access                     stable
  --filter                           stable
  --interval                         stable
  --mine                             stable
  --name                             stable
  --sort                             stable
  --type                             stable
  --types                            stable
  --watch                            stable
  --watch-only                       stable
get edgeconnects                     stable
get extension-configs                stable
  --config-id                        stable
  --version                          stable
get extensions                       stable
  --name                             stable
get functions                        stable
  --app                              stable
get gcp                              stable
get gcp connections                  stable
get gcp connections principal        stable
get gcp monitoring                   stable
get gcp monitoring-feature-sets      stable
get gcp monitoring-locations         stable
get groups                           stable
  --filter                           stable
get hub-extension-releases           stable
get hub-extensions                   stable
  --filter                           stable
get intents                          stable
  --app                              stable
get lookups                          stable
get notebooks                        stable
  --add-fields                       stable
  --admin-access                     stable
  --filter                           stable
  --interval                         stable
  --mine                             stable
  --name                             stable
  --sort                             stable
  --watch                            stable
  --watch-only                       stable
get notifications                    stable
  --type                             stable
get sdk-versions                     stable
get segments                         stable
get settings                         stable
  --schema                           stable
  --scope                            stable
get settings-schemas                 stable
get slo-templates                    stable
  --filter                           stable
get slos                             stable
  --filter                           stable
get snapshots                        stable
  --decode-snapshots                 stable
  --default-timeframe-end            stable
  --default-timeframe-start          stable
  --limit                            stable
  --max-result-records               stable
  --metadata                         stable
  --no-progress                      stable
get trash                            stable
  --deleted-after                    stable
  --deleted-before                   stable
  --deleted-by                       stable
  --interval                         stable
  --type                             stable
  --watch                            stable
  --watch-only                       stable
get users                            stable
  --filter                           stable
get wfe-task-result                  stable
  --task                             stable
get workflow-executions              stable
  --limit                            stable
  --started-since                    stable
  --started-until                    stable
  --state                            stable
  --trigger                          stable
  --workflow                         stable
get workflows                        stable
  --filter                           stable
  --interval                         stable
  --limit                            stable
  --mine                             stable
  --trigger                          stable
  --type                             stable
  --watch                            stable
  --watch-only                       stable
history                              stable
history dashboard                    stable
history document                     stable
history notebook                     stable
history workflow                     stable
inspect                              stable
  --fields                           stable
  --head                             stable
  --limit                            stable
  --list                             stable
  --offset                           stable
  --page                             stable
  --sample                           stable
  --schema                           stable
  --spill                            stable
  --spill-format                     stable
  --spill-threshold                  stable
  --spill-to                         stable
  --stats                            stable
  --tail                             stable
inventory                            stable
  --budget-queries                   stable
  --budget-seconds                   stable
  --definitions                      stable
  --no-builtin-definitions           stable
  --scan-limit-gbytes                stable
logs                                 stable
logs workflow-execution              stable
  --all                              stable
  --follow                           stable
  --task                             stable
  --tasks                            stable
open                                 stable
open intent                          stable
  --browser                          stable
  --data                             stable
  --data-file                        stable
plugin                               stable
plugin list                          stable
query                                stable
  --client-context                   stable
  --decode-snapshots                 stable
  --default-sampling-ratio           stable
  --default-scan-limit-gbytes        stable
  --default-timeframe-end            stable
  --default-timeframe-start          stable
  --dql                              stable
  --enable-preview                   stable
  --enforce-query-consumption-limit  stable
  --fetch-timeout-seconds            stable
  --file                             stable
  --fullscreen                       stable
  --height                           stable
  --include-contributions            stable
  --include-types                    stable
  --interval                         stable
  --live                             stable
  --locale                           stable
  --max-result-bytes                 stable
  --max-result-records               stable
  --metadata                         stable
  --no-progress                      stable
  --segment                          stable
  --segment-var                      stable
  --segments-file                    stable
  --set                              stable
  --spill                            stable
  --spill-format                     stable
  --spill-threshold                  stable
  --spill-to                         stable
  --timezone                         stable
  --typed                            stable
  --width                            stable
restore                              stable
restore dashboard                    stable
  --force                            stable
restore document                     stable
  --force                            stable
restore notebook                     stable
  --force                            stable
restore trash                        stable
  --force                            stable
  --new-name                         stable
restore workflow                     stable
  --force                            stable
serve                                development  (opt-in key: serve)
serve http                           development
  --addr                             development
  --idle-timeout                     development
  --max-request-bytes                development
  --read-timeout                     development
  --write-timeout                    development
share                                stable
share dashboard                      stable
  --access                           stable
  --group                            stable
  --user                             stable
share document                       stable
  --access                           stable
  --group                            stable
  --user                             stable
share notebook                       stable
  --access                           stable
  --group                            stable
  --user                             stable
skills                               stable
skills install                       stable
  --cross-client                     stable
  --for                              stable
  --force                            stable
  --global                           stable
  --list                             stable
skills status                        stable
  --for                              stable
skills uninstall                     stable
  --cross-client                     stable
  --for                              stable
token-scopes                         stable
translate                            stable
translate classic-pipelines          stable
  --include-sample-data              stable
  --skip-builtin-processing-rules    stable
  --skip-disabled-rules              stable
translate lql-to-dql                 stable
  --file                             stable
unshare                              stable
unshare dashboard                    stable
  --access                           stable
  --all                              stable
  --group                            stable
  --user                             stable
unshare document                     stable
  --access                           stable
  --all                              stable
  --group                            stable
  --user                             stable
unshare notebook                     stable
  --access                           stable
  --all                              stable
  --group                            stable
  --user                             stable
update                               stable
update aws                           stable
update aws connection                stable
  --name                             stable
  --roleArn                          stable
update aws monitoring                stable
  --featureSets                      stable
  --name                             stable
  --regions                          stable
update azure                         stable
update azure connection              stable
  --aplicationID                     stable
  --applicationID                    stable
  --applicationId                    stable
  --clientSecret                     stable
  --directoryID                      stable
  --directoryId                      stable
  --name                             stable
update azure monitoring              stable
  --featureSets                      stable
  --featuresets                      stable
  --locationFiltering                stable
  --name                             stable
update breakpoint                    stable
  --condition                        stable
  --enabled                          stable
  --filters                          stable
  --log-message                      stable
  --yes                              stable
update document                      stable
  --create-snapshot                  stable
  --dry-run                          stable
  --file                             stable
  --id                               stable
  --label                            stable
  --set                              stable
  --show-diff                        stable
  --snapshot-description             stable
  --type                             stable
update gcp                           stable
update gcp connection                stable
  --name                             stable
  --serviceAccountId                 stable
  --serviceaccountid                 stable
update gcp monitoring                stable
  --featureSets                      stable
  --featuresets                      stable
  --locationFiltering                stable
  --name                             stable
verify                               stable
verify analyzer                      stable
  --file                             stable
  --input                            stable
  --query                            stable
verify openpipeline-dql-processor    stable
  --config-id                        stable
  --file                             stable
verify openpipeline-matcher          stable
  --config-id                        stable
  --context                          stable
  --file                             stable
verify query                         stable
  --canonical                        stable
  --client-context                   stable
  --fail-on-warn                     stable
  --file                             stable
  --locale                           stable
  --set                              stable
  --timezone                         stable
version                              stable
wait                                 stable
wait query                           stable
  --backoff-multiplier               stable
  --default-sampling-ratio           stable
  --default-scan-limit-gbytes        stable
  --default-timeframe-end            stable
  --default-timeframe-start          stable
  --fetch-timeout-seconds            stable
  --file                             stable
  --for                              stable
  --initial-delay                    stable
  --locale                           stable
  --max-attempts                     stable
  --max-interval                     stable
  --max-result-bytes                 stable
  --max-result-records               stable
  --min-interval                     stable
  --quiet                            stable
  --set                              stable
  --timeout                          stable
  --timezone                         stable
  --verbose                          stable
```
