# Account Service Users — Requirements

## Overview

Add service-user management to `dtctl account`, covering list, create, and delete operations through the Dynatrace Account Management Service User Management API. The commands follow the existing platform-token command hierarchy and delegate HTTP behavior through the SDK.

## Requirements

### 1. List service users

**User story:** As an account administrator, I want to list service users so that I can inspect non-interactive identities in an account.

#### Acceptance criteria

1. WHEN a user runs `dtctl account list service-user`, THE CLI SHALL list all service users for the configured account.
2. THE command SHALL accept `service-users` as an alias.
3. THE SDK SHALL call `GET /iam/v1/accounts/{accountUuid}/service-users`.
4. WHEN the API returns `nextPageKey`, THE SDK SHALL request subsequent pages using `page-key` until no key remains.
5. THE SDK SHALL send `page-size` only on the initial request and SHALL NOT combine it with `page-key`.
6. THE CLI SHALL support the existing global output formats through `NewPrinter().PrintList`.
7. THE default table SHALL show useful identity fields without exposing credentials; structured formats SHALL preserve API identity metadata.
8. WHEN the API returns an error, THE command SHALL preserve the shared typed HTTP error behavior.

### 2. Create a service user

**User story:** As an account administrator, I want to create a service user so that automation can have a dedicated identity.

#### Acceptance criteria

1. WHEN a user runs `dtctl account create service-user --name <name>`, THE CLI SHALL create a service user in the configured account.
2. THE command SHALL accept `service-users` as an alias.
3. THE command SHALL require a non-empty `--name` and accept an optional `--description`.
4. THE SDK SHALL call `POST /iam/v1/accounts/{accountUuid}/service-users` with JSON fields `name` and, when supplied, `description`.
5. THE command SHALL perform `safety.OperationCreate` before a real mutation.
6. WHEN `--dry-run` is set, THE command SHALL print the intended operation without loading credentials, running a safety check, or calling the API.
7. AFTER successful creation, THE command SHALL print the returned service-user metadata using the standard printer; it SHALL NOT claim that a credential or secret was created.
8. THE SDK decoder SHALL follow the documented object response and MAY tolerate the official API example's one-element-array discrepancy without changing the public result type.

### 3. Delete a service user

**User story:** As an account administrator, I want to delete a service user by UUID so that obsolete automation identities can be removed.

#### Acceptance criteria

1. WHEN a user runs `dtctl account delete service-user <userUuid>`, THE CLI SHALL delete that exact service user.
2. THE command SHALL accept `service-users` as an alias and require exactly one argument.
3. THE SDK SHALL call `DELETE /iam/v1/accounts/{accountUuid}/service-users/{userUuid}`.
4. THE command SHALL perform `safety.OperationDelete` before a real mutation.
5. WHEN `--dry-run` is set, THE command SHALL print the intended deletion without loading credentials, running a safety check, or calling the API.
6. THE command SHALL report success only after a successful API response.
7. THE CLI SHALL not attempt name resolution because service-user names are not guaranteed unique.

### 4. Architecture and quality

1. HTTP models and calls SHALL live in `sdk/api/serviceuser` with no CLI output or file-I/O concerns.
2. CLI display types and SDK delegation SHALL live in `pkg/resources/serviceuser`.
3. Cobra registration SHALL live under the existing `dtctl account list|create|delete` hierarchy.
4. Mutating commands SHALL comply with repository safety requirements.
5. Pagination tests SHALL include a mock guard rejecting simultaneous `page-size` and `page-key`.
6. Tests SHALL cover SDK list pagination, create payload/response, delete method/path, and typed HTTP errors; resource conversion and command validation/dry-run behavior SHALL be covered where practical.
7. Golden tests SHALL use the real production service-user struct and synthetic data only.
8. The implementation status documentation and SDK package inventory SHALL be updated if their current structure requires it.

## API authorization

The external API requires `account-idm-read` for list and `account-idm-write` for create/delete. These are bearer-token requirements of the account API; existing account client configuration remains responsible for authentication.
