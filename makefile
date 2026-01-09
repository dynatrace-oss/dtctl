.PHONY: all build clean test install lint fmt markdownlint markdownlint-fix security-scan check release release-snapshot

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS = -ldflags "-X github.com/dynatrace-oss/dtctl/cmd.version=$(VERSION) -X github.com/dynatrace-oss/dtctl/cmd.commit=$(COMMIT) -X github.com/dynatrace-oss/dtctl/cmd.date=$(DATE) -s -w"

MD_LINT_CLI_IMAGE := "ghcr.io/igorshubovych/markdownlint-cli:v0.31.1"

all: build

# Build the binary
build:
	@echo "Building dtctl..."
	@go build $(LDFLAGS) -o bin/dtctl .

# Build for macOS (arm64)
build-darwin-arm64:
	@echo "Building dtctl for darwin/arm64..."
	@env GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build $(LDFLAGS) -o bin/dtctl-darwin-arm64 .

# Convenience target: build for current host OS/ARCH
build-host:
	@echo "Building dtctl for host: $(shell go env GOOS)/$(shell go env GOARCH)..."
	@env GOOS=$(shell go env GOOS) GOARCH=$(shell go env GOARCH) CGO_ENABLED=0 go build $(LDFLAGS) -o bin/dtctl-host .

# Run tests
test:
	@echo "Running tests..."
	@go test -v -race -coverprofile=coverage.out ./...

# Install locally
install:
	@echo "Installing dtctl..."
	@go install $(LDFLAGS) .

# Clean build artifacts
clean:
	@rm -rf bin/ dist/ coverage.out

# Run linter
lint:
	@golangci-lint run

# Run security vulnerability scan
security-scan:
	@echo "Running govulncheck..."
	@govulncheck ./...

# Run all checks (lint + security)
check: lint security-scan

# Format code
fmt:
	@go fmt ./...
	@goimports -w .

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
