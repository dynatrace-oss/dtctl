# Enterprise Architecture Evaluation

Generated on 2026-07-07 for `my-env` against `https://mjs70956.apps.dynatrace.com/`.

## Executive Summary

This tenant assessment completed 36 probes with grade **B** and score **89/100**. It combines direct dtctl inventories, DQL-backed discovery, and sampled deep checks for workflows, dashboards, notebooks, and extensions.

## Scorecard

| Dimension | Value |
|---|---:|
| Success rate | 83% |
| Domain coverage | 100% |
| Deep check rate | 86% |
| Discovery signals | 646 |

## Domain Coverage

| Domain | Probes | Success | Failed | Discovered Items | Representative Signals |
|---|---:|---:|---:|---:|---|
| Alerting | 4 | 3 | 1 | 111 | vu9U3hXa3q0AAAABAB9idWlsdGluOmRhdmlzLmFub21hbHktZGV0ZWN0b3JzAAZ0ZW5hbnQABnRlbmFudAAkZGZmYmE5YmEtYzgwOS0zMDFlLWFlYzctZWMyMDE4ODU2NTllvu9U3hXa3q0 |
| Automation | 3 | 3 | 0 | 2 | d7a196db-d664-4a9b-b7bb-c75b0049e61f |
| Cloud | 6 | 3 | 3 | 3 | - |
| Configuration | 2 | 2 | 0 | 2 | - |
| Content | 7 | 6 | 1 | 288 | 033954cf-181c-4dca-b8bf-f0608c0c3ad6, dynatrace.experience.vitals.core-web-vitals |
| Deployment | 1 | 1 | 0 | 4 | PROCESS_GROUP_INSTANCE-B8BF271E48818128 |
| Extensions | 3 | 3 | 0 | 31 | com.dynatrace.custom.python-certificate-monitor, 47894a5f-4703-3ed3-9813-cd772e919205, 3049a1d5-180b-3003-af09-50238e41fc4e |
| Governance | 3 | 3 | 0 | 72 | HOST-718FF4B38DF3FF1A |
| Kubernetes | 2 | 2 | 0 | 0 | - |
| Platform | 2 | 2 | 0 | 33 | 7PUw6cM11rJ |
| Telemetry | 2 | 1 | 1 | 100 | - |
| Topology | 1 | 1 | 0 | 0 | - |

## Top Risks

- **HIGH**: notifications inventory probe failed. Evidence: exit status 1
- **HIGH**: spans query probe failed. Evidence: exit status 1
- **MEDIUM**: aws monitoring inventory probe failed. Evidence: exit status 1
- **MEDIUM**: azure monitoring inventory probe failed. Evidence: exit status 1
- **MEDIUM**: gcp monitoring inventory probe failed. Evidence: [Preview] GCP commands are in Preview and may change in future releases.
- **MEDIUM**: dashboard history 033954cf 181c 4dca b8bf f0608c0c3ad6 probe failed. Evidence: exit status 1

## Probe Details

| Probe | Domain | Kind | Status | Items | Duration (ms) | Notes |
|---|---|---|---|---:|---:|---|
| workflows_inventory | Automation | command | ok | 1 | 954 | [   {     "id": "d7a196db-d664-4a9b-b7bb-c75b0049e61f",     "title": "Network Log Generator",     "isDeployed": false... |
| workflow_executions_inventory | Automation | command | ok | 0 | 820 | [] |
| dashboards_inventory | Content | command | ok | 110 | 1409 | [   {     "id": "033954cf-181c-4dca-b8bf-f0608c0c3ad6",     "name": "Log Monitoring Coverage Overview",     "type": "... |
| notebooks_inventory | Content | command | ok | 3 | 788 | [   {     "id": "dynatrace.experience.vitals.core-web-vitals",     "name": "Google Core Web Vitals analysis",     "ty... |
| documents_inventory | Content | command | ok | 172 | 1309 | [   {     "id": "033954cf-181c-4dca-b8bf-f0608c0c3ad6",     "name": "Log Monitoring Coverage Overview",     "type": "... |
| buckets_inventory | Platform | command | ok | 32 | 893 | [   {     "bucketName": "default_application_snapshots",     "table": "application.snapshots",     "displayName": "De... |
| segments_inventory | Platform | command | ok | 1 | 594 | [   {     "uid": "7PUw6cM11rJ",     "name": "ecom-prod-service-385017 ",     "isPublic": true,     "owner": "499e96b7... |
| extensions_inventory | Extensions | command | ok | 27 | 743 | [   {     "extensionName": "com.dynatrace.custom.python-certificate-monitor",     "version": "1.10.17"   },   {     "... |
| settings_objects_inventory | Configuration | command | ok | 0 | 1110 | [] |
| lookups_inventory | Configuration | command | ok | 2 | 632 | [   {     "path": "/lookups/aafes/payment_cart_bridge",     "displayName": "AAFES Payment ↔ Cart Bridge",     "desc... |
| slos_inventory | Governance | command | ok | 0 | 579 | [] |
| notifications_inventory | Alerting | command | error | 0 | 669 | exit status 1 |
| analyzers_inventory | Alerting | command | ok | 10 | 715 | [   {     "name": "dt.statistics.ui.anomaly_detection.AutoAdaptiveAnomalyDetectionAnalyzer",     "displayName": "Auto... |
| anomaly_detectors_inventory | Alerting | command | ok | 1 | 557 | [   {     "schemaId": "builtin:davis.anomaly-detectors",     "scope": "environment",     "objectId": "vu9U3hXa3q0AAAA... |
| aws_connections_inventory | Cloud | command | ok | 1 | 713 | null |
| aws_monitoring_inventory | Cloud | command | error | 0 | 705 | exit status 1 |
| azure_connections_inventory | Cloud | command | ok | 1 | 629 | null |
| azure_monitoring_inventory | Cloud | command | error | 0 | 769 | exit status 1 |
| gcp_connections_inventory | Cloud | command | ok | 1 | 599 | null |
| gcp_monitoring_inventory | Cloud | command | error | 0 | 569 | [Preview] GCP commands are in Preview and may change in future releases. |
| host_groups_query | Topology | query | ok | 0 | 767 | {   "records": [] } |
| kubernetes_clusters_query | Kubernetes | query | ok | 0 | 723 | {   "records": [] } |
| cloud_namespaces_query | Kubernetes | query | ok | 0 | 648 | {   "records": [] } |
| host_tags_query | Governance | query | ok | 36 | 632 | {   "records": [     {       "entity.name": " content-service-prod (001548f729d0361c8...)",       "id": "HOST-718FF4B... |
| management_zones_query | Governance | query | ok | 36 | 636 | {   "records": [     {       "entity.name": " content-service-prod (001548f729d0361c8...)",       "id": "HOST-718FF4B... |
| events_query | Alerting | query | ok | 100 | 867 | {   "records": [     {       "dt.active_gate.id": "0x12363",       "dt.entity.synthetic_location": "SYNTHETIC_LOCATIO... |
| logs_query | Telemetry | query | ok | 100 | 633 | {   "records": [     {       "cloud.platform": "gcp_kubernetes_engine",       "cloud.provider": "gcp",       "content... |
| spans_query | Telemetry | query | error | 0 | 567 | exit status 1 |
| activegate_otel_query | Deployment | query | ok | 4 | 649 | {   "records": [     {       "entity.name": "Dynatrace ActiveGate Extensions Controller",       "id": "PROCESS_GROUP_... |
| workflow_describe_d7a196db_d664_4a9b_b7bb_c75b0049e61f | Automation | command | ok | 1 | 783 | {   "id": "d7a196db-d664-4a9b-b7bb-c75b0049e61f",   "title": "Network Log Generator",   "isDeployed": false,   "descr... |
| dashboard_describe_033954cf_181c_4dca_b8bf_f0608c0c3ad6 | Content | command | ok | 1 | 545 | {   "id": "033954cf-181c-4dca-b8bf-f0608c0c3ad6",   "name": "Log Monitoring Coverage Overview",   "type": "dashboard"... |
| dashboard_history_033954cf_181c_4dca_b8bf_f0608c0c3ad6 | Content | command | error | 0 | 556 | exit status 1 |
| notebook_describe_dynatrace_experience_vitals_core_web_vitals | Content | command | ok | 1 | 580 | {   "id": "dynatrace.experience.vitals.core-web-vitals",   "name": "Google Core Web Vitals analysis",   "type": "note... |
| notebook_history_dynatrace_experience_vitals_core_web_vitals | Content | command | ok | 1 | 570 | No snapshots found for this notebook |
| extension_configs_com_dynatrace_custom_python_certificate_monitor | Extensions | command | ok | 3 | 593 | [   {     "type": "extension_monitoring_config",     "extensionName": "com.dynatrace.custom.python-certificate-monito... |
| extension_configs_com_dynatrace_extension_akamai_siem_fallback | Extensions | command | ok | 1 | 839 | [   {     "type": "extension_monitoring_config",     "extensionName": "com.dynatrace.extension.akamai-siem",     "obj... |

## Dashboard And Notebook Ownership / Change History

### Dashboard Sample

- Name: Log Monitoring Coverage Overview
- Owner: 0b6a9d8d-1861-4827-ac7a-466e68493e6a
- Private: false
- Access: read
- Created: 2026-06-11T16:10:44.913Z
- Last modified: 2026-06-11T16:11:42.34Z
- Last modified by: 0b6a9d8d-1861-4827-ac7a-466e68493e6a
- History signal: exit status 1

### Notebook Sample

- Name: Google Core Web Vitals analysis
- Owner: 50436aec-8901-4282-ae81-690bd6509b18
- Private: false
- Access: read
- Created: 2025-10-22T05:31:04.439Z
- Last modified: 2026-04-29T13:13:19.669Z
- Last modified by: 50436aec-8901-4282-ae81-690bd6509b18
- History signal: No snapshots found for this notebook


## Settings And Extension Configuration Samples

### Settings Objects

- Sample output: []

### Extension Config Sample

- Returned extension configs: 3
- 47894a5f-4703-3ed3-9813-cd772e919205 | version `` | id `47894a5f-4703-3ed3-9813-cd772e919205`
- 6ea7732c-09a5-3e3e-bf3c-5a4176a9b16a | version `` | id `6ea7732c-09a5-3e3e-bf3c-5a4176a9b16a`
- 85c854dd-f535-305f-ae59-ca4bbae4c4f2 | version `` | id `85c854dd-f535-305f-ae59-ca4bbae4c4f2`

### Extension Config Sample

- Returned extension configs: 1
- 3049a1d5-180b-3003-af09-50238e41fc4e | version `` | id `3049a1d5-180b-3003-af09-50238e41fc4e`

