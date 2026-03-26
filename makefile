.PHONY: all build build-pii-resolve clean test test-unit test-integration test-all test-coverage test-update-golden install lint lint-strict fmt markdownlint markdownlint-fix security-scan check release release-snapshot

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Minimum test coverage percentage. Raise as coverage improves: 60 -> 70 -> 80 -> 85.
COVERAGE_THRESHOLD ?= 60

LDFLAGS = -ldflags "-X github.com/dynatrace-oss/dtctl/cmd.version=$(VERSION) -X github.com/dynatrace-oss/dtctl/cmd.commit=$(COMMIT) -X github.com/dynatrace-oss/dtctl/cmd.date=$(DATE) -s -w"
INSTALL_BIN_DIR ?= $(if $(GOBIN),$(GOBIN),$(shell go env GOPATH)/bin)

MD_LINT_CLI_IMAGE := "ghcr.io/igorshubovych/markdownlint-cli:v0.31.1"

all: build

# Build the binary
build:
	@echo "Building dtctl..."
	@go build $(LDFLAGS) -o bin/dtctl .

# Build the PII resolve tool (separate trust boundary — human-only)
build-pii-resolve:
	@echo "Building dtctl-pii-resolve..."
	@go build $(LDFLAGS) -o bin/dtctl-pii-resolve ./cmd/pii-resolve

# Build for macOS (arm64)
build-darwin-arm64:
	@echo "Building dtctl for darwin/arm64..."
	@env GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build $(LDFLAGS) -o bin/dtctl-darwin-arm64 .

# Convenience target: build for current host OS/ARCH
build-host:
	@echo "Building dtctl for host: $(shell go env GOOS)/$(shell go env GOARCH)..."
	@env GOOS=$(shell go env GOOS) GOARCH=$(shell go env GOARCH) CGO_ENABLED=0 go build $(LDFLAGS) -o bin/dtctl-host .

# Run unit tests (default test target)
test:
	@echo "Running unit tests..."
	@go test -v -race -coverprofile=coverage.out ./...

# Run only unit tests (excludes integration tests)
test-unit:
	@echo "Running unit tests..."
	@go test -v -race -coverprofile=coverage.out ./...

# Run integration tests (requires DTCTL_INTEGRATION_ENV and DTCTL_INTEGRATION_TOKEN)
# Environment variables can be set via:
#   1. .integrationtests.env file (recommended, gitignored)
#   2. Shell environment variables
#   3. Command line: DTCTL_INTEGRATION_ENV=... make test-integration
test-integration:
	@echo "Running integration tests..."
	@# Load .integrationtests.env if it exists
	@if [ -f .integrationtests.env ]; then \
		echo "Loading environment from .integrationtests.env"; \
		export $$(cat .integrationtests.env | grep -v '^#' | xargs); \
	fi; \
	if [ -z "$$DTCTL_INTEGRATION_ENV" ]; then \
		echo "Error: DTCTL_INTEGRATION_ENV not set."; \
		echo ""; \
		echo "Create .integrationtests.env with your credentials:"; \
		echo "  cp .integrationtests.env.example .integrationtests.env"; \
		echo "  # Edit .integrationtests.env with your environment URL and token"; \
		echo ""; \
		echo "Or set environment variables:"; \
		echo "  export DTCTL_INTEGRATION_ENV=https://your-env.apps.dynatrace.com"; \
		echo "  export DTCTL_INTEGRATION_TOKEN=dt0s16.XXX"; \
		exit 1; \
	fi; \
	if [ -z "$$DTCTL_INTEGRATION_TOKEN" ]; then \
		echo "Error: DTCTL_INTEGRATION_TOKEN not set."; \
		echo "See .integrationtests.env.example for setup instructions."; \
		exit 1; \
	fi; \
	go test -v -race -count=1 -tags integration ./test/e2e/...

# Regenerate golden files (run after intentional output changes)
test-update-golden:
	@echo "Updating golden files..."
	@go test ./... -update
	@echo "Golden files updated. Review changes with: git diff pkg/output/testdata/"

# Run all tests (unit + integration)
test-all: test-unit test-integration

# Run tests and enforce coverage threshold
test-coverage:
	@echo "Running tests with coverage..."
	@go test -race -coverprofile=coverage.out -covermode=atomic ./...
	@echo ""
	@echo "=== Package Coverage ==="
	@go tool cover -func=coverage.out | grep -E "^(total|.*\t)" | tail -30
	@echo ""
	@COVERAGE=$$(go tool cover -func=coverage.out | grep total | awk '{print $$3}' | sed 's/%//'); \
	COVERAGE_INT=$${COVERAGE%.*}; \
	echo "Total coverage: $${COVERAGE}% (threshold: $(COVERAGE_THRESHOLD)%)"; \
	if [ "$$COVERAGE_INT" -lt "$(COVERAGE_THRESHOLD)" ]; then \
		echo "FAIL: Coverage $${COVERAGE}% is below the $(COVERAGE_THRESHOLD)% threshold"; \
		exit 1; \
	fi; \
	echo "OK: Coverage meets threshold"

# Install locally
install:
	@echo "Installing dtctl to $(INSTALL_BIN_DIR)..."
	@mkdir -p "$(INSTALL_BIN_DIR)"
	@go build $(LDFLAGS) -o "$(INSTALL_BIN_DIR)/dtctl" .
	@if case ":$$PATH:" in *":$(INSTALL_BIN_DIR):"*) false;; *) true;; esac; then \
		echo ""; \
		echo "dtctl installed, but $(INSTALL_BIN_DIR) is not in PATH."; \
		echo "Add it with:"; \
		echo '  export PATH="$$PATH:$(INSTALL_BIN_DIR)"'; \
	fi

# Clean build artifacts
clean:
	@rm -rf bin/ dist/ coverage.out

# Run linter
lint:
	@golangci-lint run

# Run strict linter (matches CI behavior — zero tolerance)
lint-strict:
	@echo "Checking goimports formatting..."
	@goimports_output=$$(goimports -local github.com/dynatrace-oss/dtctl -l .); \
	if [ -n "$$goimports_output" ]; then \
		echo "The following files are not properly formatted:"; \
		echo "$$goimports_output"; \
		echo ""; \
		echo "Run 'make fmt' to fix formatting."; \
		exit 1; \
	fi
	@echo "Checking go mod tidy..."
	@go mod tidy && git diff --exit-code go.mod go.sum > /dev/null 2>&1 || \
		(echo "go.mod or go.sum is not tidy. Run 'go mod tidy' and commit the changes." && exit 1)
	@echo "Running golangci-lint (zero tolerance)..."
	@golangci-lint run

# Run security vulnerability scan
security-scan:
	@echo "Running govulncheck..."
	@govulncheck ./...

# Run all checks (lint-strict + security)
check: lint-strict security-scan

# Format code
fmt:
	@go fmt ./...
	@goimports -local github.com/dynatrace-oss/dtctl -w .

# Markdown linting
markdownlint:
	docker run -v $(CURDIR):/workdir --rm $(MD_LINT_CLI_IMAGE) "**/*.md"

markdownlint-fix:
	docker run -v $(CURDIR):/workdir --rm $(MD_LINT_CLI_IMAGE) "**/*.md" --fix

# Release (using goreleaser)
release:
	@goreleaser release --clean

# Release snapshot (local testing)
release-snapshot:
	@goreleaser release --snapshot --clean
