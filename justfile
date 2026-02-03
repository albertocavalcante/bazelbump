# bazelbump development commands
# Run `just --list` to see available commands

# Packages to exclude from tests/linting (vendored code, tools, data)
exclude_pattern := "third_party|tools/|data/"

# Default recipe: show help
default:
    @just --list

# === BUILD ===

# Build all packages
build:
    go build ./...

# Build CLI binary
build-cli:
    go build -o bin/bazelbump ./cmd/bazelbump

# === INSTALL ===

# Install CLI to ~/.local/bin (recommended)
install:
    #!/usr/bin/env bash
    mkdir -p ~/.local/bin
    echo "Building bazelbump..."
    go build -o ~/.local/bin/bazelbump ./cmd/bazelbump
    echo "✓ Installed to ~/.local/bin/bazelbump"
    if [[ ":$PATH:" != *":$HOME/.local/bin:"* ]]; then
        echo ""
        echo "⚠ Add ~/.local/bin to your PATH:"
        if [[ "$OSTYPE" == "darwin"* ]]; then
            echo "  echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.zshrc"
            echo "  source ~/.zshrc"
        else
            echo "  echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.bashrc"
            echo "  source ~/.bashrc"
        fi
    fi
    echo ""
    echo "Run: bazelbump --help"

# Install CLI to $GOPATH/bin
install-gopath:
    @echo "Installing bazelbump to \$GOPATH/bin..."
    go install ./cmd/bazelbump
    @echo "✓ Installed! Make sure \$GOPATH/bin is in your PATH"
    @echo "  Run: bazelbump --help"

# Install to /usr/local/bin (requires sudo)
install-system:
    #!/usr/bin/env bash
    echo "Building bazelbump..."
    go build -o /tmp/bazelbump ./cmd/bazelbump
    echo "Installing to /usr/local/bin (requires sudo)..."
    sudo mv /tmp/bazelbump /usr/local/bin/
    echo "✓ Installed to /usr/local/bin/bazelbump"
    echo ""
    echo "Run: bazelbump --help"

# Uninstall from common locations
uninstall:
    #!/usr/bin/env bash
    echo "Removing bazelbump..."
    rm -f "$GOPATH/bin/bazelbump" 2>/dev/null && echo "  Removed from \$GOPATH/bin"
    rm -f "$HOME/.local/bin/bazelbump" 2>/dev/null && echo "  Removed from ~/.local/bin"
    rm -f ./bin/bazelbump 2>/dev/null && echo "  Removed from ./bin"
    echo "✓ Done"
    echo ""
    echo "Note: /usr/local/bin/bazelbump requires: sudo rm /usr/local/bin/bazelbump"

# Quick setup: sync flags and install
setup: sync-flags install
    @echo ""
    @echo "✓ Setup complete! Try:"
    @echo "  bazelbump analyze --to 9.0.0"

# === TEST ===

# Run all tests
test:
    go list ./... | grep -v -E '{{exclude_pattern}}' | xargs go test

# Run tests with verbose output
test-v:
    go list ./... | grep -v -E '{{exclude_pattern}}' | xargs go test -v

# Run tests with race detector
test-race:
    go list ./... | grep -v -E '{{exclude_pattern}}' | xargs go test -race

# Run benchmarks
bench:
    go list ./... | grep -v -E '{{exclude_pattern}}' | xargs go test -bench=. -benchmem

# Run tests with coverage
test-cov:
    go list ./... | grep -v -E '{{exclude_pattern}}' | xargs go test -coverprofile=coverage.out
    go tool cover -html=coverage.out -o coverage.html
    @echo "Coverage report: coverage.html"

# === LINT ===

# Run golangci-lint
lint:
    go list ./... | grep -v -E '{{exclude_pattern}}' | xargs go run -modfile=tools/lint/go.mod github.com/golangci/golangci-lint/v2/cmd/golangci-lint run

# Run staticcheck
staticcheck:
    go list ./... | grep -v -E '{{exclude_pattern}}' | xargs go run -modfile=tools/lint/go.mod honnef.co/go/tools/cmd/staticcheck

# Run all lint checks
lint-all: lint staticcheck vet

# Run go vet
vet:
    go list ./... | grep -v -E '{{exclude_pattern}}' | xargs go vet

# === FORMAT ===

# Format all Go files
fmt:
    find . -name '*.go' -not -path './third_party/*' -not -path './tools/lint/*' -not -path './data/*' | xargs gofmt -w -s

# Check formatting
fmt-check:
    #!/usr/bin/env bash
    unformatted=$(find . -name '*.go' -not -path './third_party/*' -not -path './tools/lint/*' -not -path './data/*' -exec gofmt -l {} +)
    if [ -n "$unformatted" ]; then
        echo "Code is not formatted. Run 'just fmt'"
        echo "$unformatted"
        exit 1
    fi

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

# === MODULE MANAGEMENT ===

# Tidy all go.mod files
tidy:
    go mod tidy
    cd tools/lint && go mod tidy
    cd tools/sync-flags && go mod tidy

# Verify the module has no external dependencies
verify-deps:
    #!/usr/bin/env bash
    echo "Checking for direct dependencies..."
    deps=$(go list -m -f '{{"{{if not .Indirect}}{{.Path}}{{end}}"}}' all 2>/dev/null | grep -v "^github.com/albertocavalcante/bazelbump$" || true)
    if [ -z "$deps" ]; then
        echo "✓ No direct dependencies!"
    else
        echo "✗ Found direct dependencies:"
        echo "$deps"
        exit 1
    fi

# === DEV ===

# Development workflow: format, lint, test
dev: fmt lint test

# Full CI check
ci: build test-race lint-all vet fmt-check verify-deps

# Run bazelbump with arguments (e.g., just run analyze --to 9.0.0)
run *args:
    go run ./cmd/bazelbump {{args}}

# Show which packages will be tested/linted
show-packages:
    @echo "Packages included in test/lint:"
    @go list ./... | grep -v -E '{{exclude_pattern}}'

# === CLEAN ===

# Clean build artifacts
clean:
    rm -rf bin/ coverage.out coverage.html
    go clean -cache

# Clean all including synced data
clean-all: clean
    rm -rf data/flags/

# === RELEASE ===

# Build release binaries for all platforms
release version:
    #!/usr/bin/env bash
    mkdir -p dist
    for os in linux darwin windows; do
        for arch in amd64 arm64; do
            ext=""
            if [ "$os" = "windows" ]; then ext=".exe"; fi
            echo "Building bazelbump-${os}-${arch}${ext}..."
            GOOS=$os GOARCH=$arch go build -ldflags "-X main.version={{version}}" \
                -o "dist/bazelbump-${os}-${arch}${ext}" ./cmd/bazelbump
        done
    done
    cd dist && sha256sum * > checksums.txt
    echo "Release binaries in dist/"

# === HELP ===

# Show vendoring plan
vendoring-plan:
    @cat docs/VENDORING_PLAN.md

# Quick help for common tasks
help:
    @echo "Common commands:"
    @echo "  just build      - Build all packages"
    @echo "  just test       - Run tests"
    @echo "  just install    - Install bazelbump to \$GOPATH/bin"
    @echo "  just dev        - Format, lint, and test"
    @echo "  just ci         - Full CI check"
    @echo "  just run <args> - Run bazelbump with arguments"
    @echo "  just sync-flags - Sync Bazel flag data"
    @echo ""
    @echo "Run 'just --list' for all commands"
