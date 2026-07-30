# Account Service Users — Implementation Tasks

- [ ] 1. Add the SDK service-user API package
  - [ ] Define service-user, create-request, and list-response models.
  - [ ] Implement account-scoped base path and cursor-paginated list.
  - [ ] Implement create and delete operations using shared response handling.
  - [ ] Add SDK unit tests, including the pagination query-constraint guard and typed errors.

- [ ] 2. Add the CLI resource wrapper
  - [ ] Define the real output struct and table/wide tags.
  - [ ] Convert all API identity fields without introducing secret handling.
  - [ ] Delegate list, create, and delete to the SDK handler.
  - [ ] Add focused conversion/delegation tests.

- [ ] 3. Add account commands
  - [ ] Register `list service-user` and `service-users` alias.
  - [ ] Register `create service-user` with required `--name` and optional `--description`.
  - [ ] Register `delete service-user <userUuid>`.
  - [ ] Use create/delete safety operations for real mutations and bypass setup/safety/API calls for dry-run.
  - [ ] Print list/create metadata through the standard printer and mutation status through output helpers.
  - [ ] Add focused command tests for validation, aliases/registration, and dry-run behavior where supported by existing test seams.

- [ ] 4. Add output snapshots and documentation
  - [ ] Add service-user fixtures using real production structs and synthetic identities.
  - [ ] Generate and review table, wide, JSON, YAML, CSV, TOON, and empty golden files.
  - [ ] Update implementation status and SDK package inventory where applicable.

- [ ] 5. Validate and review
  - [ ] Run formatting and targeted SDK/resource/command/output tests under WSL.
  - [ ] Run relevant root and SDK module test suites or document any environment limitation.
  - [ ] Review the final diff for API fidelity, safety, sensitive output, pagination, and architecture.
  - [ ] Resolve all actionable review findings and repeat review until approved.
