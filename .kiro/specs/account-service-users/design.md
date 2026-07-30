# Account Service Users — Design

## Command surface

```text
dtctl account list service-user [-o table|wide|json|yaml|csv|toon]
dtctl account create service-user --name <name> [--description <text>] [--dry-run]
dtctl account delete service-user <userUuid> [--dry-run]
```

`service-users` is an alias on each resource command. The singular resource name matches the existing `dtctl account ... token` convention.

## Layering

### `sdk/api/serviceuser`

Owns API-only types and HTTP operations:

```go
type ServiceUser struct {
    UID         string `json:"uid"`
    Email       string `json:"email"`
    Name        string `json:"name"`
    Surname     string `json:"surname"`
    Description string `json:"description"`
    CreatedAt   string `json:"createdAt"`
    GroupUUID   string `json:"groupUuid,omitempty"`
}

type ServiceUserCreate struct {
    Name        string `json:"name"`
    Description string `json:"description,omitempty"`
}

type ServiceUserListResponse struct {
    Results     []ServiceUser `json:"results"`
    NextPageKey string        `json:"nextPageKey"`
    TotalCount  int           `json:"totalCount"`
}
```

`Handler` stores the shared `httpclient.Client` and account UUID. `basePath()` returns `/iam/v1/accounts/{accountUuid}/service-users`.

- `List(ctx)` starts with `page-size=100`; subsequent requests send only `page-key=<nextPageKey>`. It appends `results` and stops when `nextPageKey` is empty. It does not rely on `totalCount`, avoiding numeric-contract ambiguity and ensuring the API cursor determines completion.
- `Create(ctx, req)` POSTs JSON and returns `*ServiceUser`. Since official documentation describes an object but displays an array example, decoding should first accept the documented object and optionally accept a one-element array. Empty or multi-item arrays are malformed create responses.
- `Delete(ctx, userUUID)` sends DELETE to `basePath()+"/"+userUUID`. URL path construction mirrors the existing platform-token implementation.
- Every operation uses `httpclient.CheckResponse` and wraps transport, HTTP, and JSON errors with operation-specific context.

### `pkg/resources/serviceuser`

Provides a CLI output type with table tags and delegates to the SDK with `context.Background()`.

Suggested columns:

- default: `NAME`, `EMAIL`, `UID`, `CREATED`
- wide: `DESCRIPTION`, `GROUP-UUID`

The output struct retains `surname`, `description`, and `groupUuid` in structured formats. No secret field exists.

### `cmd/account_service_user.go`

Registers three Cobra commands onto the existing `accountListCmd`, `accountCreateCmd`, and `accountDeleteCmd` values defined by `cmd/account_token.go`.

- List calls `SetupAccount`, lists through the resource handler, then `NewPrinter().PrintList`.
- Create validates flags before setup. Dry-run exits after printing name/description. Real execution calls `SetupAccountWithSafety(safety.OperationCreate)`, creates the user, prints a success message to stderr via output helpers, and prints returned metadata via `NewPrinter().Print` so structured/agent output remains useful.
- Delete validates exactly one UUID argument. Dry-run exits before setup. Real execution calls `SetupAccountWithSafety(safety.OperationDelete)`, deletes by UUID, and prints success.

Create output deliberately differs from platform-token create: there is no one-time secret, so metadata goes through the standard printer rather than direct `fmt.Println` secret output.

## Safety

Create and delete are mutating operations. Safety checks occur only for real execution and before constructing/using an API handler. Dry-run performs no credential loading and no safety check, matching the workspace safety rule.

## Pagination

The API uses kebab-case query parameters and camelCase response keys:

```text
request 1: ?page-size=100
request 2+: ?page-key=<nextPageKey>
response:  {"nextPageKey":"..."}
```

A mock-server test explicitly returns HTTP 400 if both query parameters are present. This guards the Dynatrace pagination constraint.

## Error handling

No operation-specific error DTO is documented. Shared `httpclient.CheckResponse` remains authoritative, preserving typed `401`, `403`, and `404` errors. JSON decoding errors include operation context. The client does not retry POST because create is not documented as idempotent.

## Testing and validation

- SDK unit tests: list, cursor pagination + query guard, create request/response, optional array compatibility if implemented, delete, typed errors.
- Resource unit tests: SDK-to-CLI field conversion and delegation as feasible.
- Command tests: registration/aliases, required name, exact delete args, and dry-run not invoking setup where test seams permit.
- Golden tests: table, wide, JSON, YAML, CSV, TOON, and empty table using synthetic `@example.invalid` identities.
- Validation: `gofmt`, targeted package tests, golden tests, SDK module tests, root module tests/build as practical under WSL.

## Documentation

Update `docs/dev/IMPLEMENTATION_STATUS.md` with account service-user list/create/delete support and `sdk/README.md` if it inventories SDK API packages. No changelog is edited.

## External contract sources

- [Service User Management API overview](https://docs.dynatrace.com/docs/dynatrace-api/account-management-api/service-user-management-api)
- [List service users](https://docs.dynatrace.com/docs/dynatrace-api/account-management-api/service-user-management-api/get-all-service-users)
- [Create service user](https://docs.dynatrace.com/docs/dynatrace-api/account-management-api/service-user-management-api/post-service-user)
- [Delete service user](https://docs.dynatrace.com/docs/dynatrace-api/account-management-api/service-user-management-api/delete-service-user)

Content derived from the official sources was rephrased for compliance with licensing restrictions.
