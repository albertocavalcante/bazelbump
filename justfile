# bazelbump justfile
# Run `just` to see available commands

set shell := ["bash", "-cu"]

# Default: show help
default:
    @just --list

# === BUILD ===

# Build all packages
build:
    go build ./...

# Build CLI binary
build-cli:
    go build -o bin/bazelbump ./cmd/bazelbump

# === TEST ===

# Run all tests
test:
    go test ./...

# Run tests with verbose output
test-v:
    go test -v ./...

# Run tests with race detector
test-race:
    go test -race ./...

# Run benchmarks
bench:
    go test -bench=. -benchmem ./...

# Run tests with coverage
test-cov:
    go test -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html
    @echo "Coverage report: coverage.html"

# === LINT ===

# Run all linters
lint:
    go run -modfile=tools/lint/go.mod github.com/golangci/golangci-lint/v2/cmd/golangci-lint run

# Run staticcheck
staticcheck:
    go run -modfile=tools/lint/go.mod honnef.co/go/tools/cmd/staticcheck ./...

# Run all lint checks
lint-all: lint staticcheck vet

# Run go vet
vet:
    go vet ./...

# === FORMAT ===

# Format all Go files
fmt:
    gofmt -w -s .
    go run -modfile=tools/lint/go.mod golang.org/x/tools/cmd/goimports -w -local github.com/albertocavalcante/bazelbump .

# Check formatting
fmt-check:
    @gofmt -l . | grep -v third_party | grep -v tools/ | { read -r line && echo "Files need formatting:" && echo "$line" && exit 1 || true; }

# === SYNC/VENDOR ===

# Sync flag data from registry.build
sync-flags:
    cd tools/sync-flags && go run . -dest ../../data/flags

# Sync flag data for specific version
sync-flags-version version:
    cd tools/sync-flags && go run . -dest ../../data/flags -version {{version}}

# List available flag versions
sync-flags-list:
    cd tools/sync-flags && go run . -list

# Vendor Bazel protos (bazel_flags.proto, option_filters.proto)
vendor-protos:
    go run ./tools/vendor-protos

# Vendor Bazel protos from specific tag
vendor-protos-tag tag:
    go run ./tools/vendor-protos -tag {{tag}}

# === CODEGEN ===

# Generate Go code from protos
proto-gen:
    go run ./tools/proto-gen

# Generate all (protos + other generated code)
generate: proto-gen

# === DEV ===

# Development workflow: format, lint, test
dev: fmt lint test

# Full CI check
ci: build test-race lint-all vet fmt-check

# Verify no unexpected dependencies
verify-deps:
    @echo "Checking for unexpected direct dependencies..."
    @deps=$$(go list -m -f '{{{{if not .Indirect}}{{{{.Path}}{{{{end}}' all | grep -v "^github.com/albertocavalcante/bazelbump$$" | grep -v "^$$"); \
    if [ -n "$$deps" ]; then \
        echo "ERROR: Found direct dependencies (should vendor instead):"; \
        echo "$$deps"; \
        exit 1; \
    else \
        echo "OK: No unexpected direct dependencies"; \
    fi

# === CLEAN ===

# Clean build artifacts
clean:
    rm -rf bin/ coverage.out coverage.html
    go clean -cache

# Clean all including vendored data
clean-all: clean
    rm -rf data/flags/

# === HELP ===

# Show vendoring plan
vendoring-plan:
    @cat docs/VENDORING_PLAN.md
