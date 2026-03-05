set shell := ["bash", "-euo", "pipefail", "-c"]

default:
  @just --list

# Build the project.
build:
  go build ./...

# Build the CLI binary at a fixed temp path.
build-tmp:
  go build -o /tmp/git-scope ./cmd/git-scope

# Run the CLI from source.
run *args:
  go run ./cmd/git-scope {{args}}

# Run all Go tests.
test:
  go test ./...

# Format all Go files.
fmt:
  gofmt -w .

# Verify Go formatting matches gofmt.
fmt-check:
  @if [ -n "$$(gofmt -l .)" ]; then \
    echo "The following files are not gofmt-formatted:"; \
    gofmt -l .; \
    exit 1; \
  fi

# Lint with golangci-lint via mise (project standard).
lint:
  GOCACHE=/tmp/go-build GOLANGCI_LINT_CACHE=/tmp/golangci-cache mise exec -- golangci-lint run --enable gocritic --enable gocyclo ./...

# Reproduce Go Report Card-style cyclomatic checks (threshold: >15).
cyclo:
  GOCACHE=/tmp/go-build GOLANGCI_LINT_CACHE=/tmp/golangci-cache mise exec -- golangci-lint run -c .golangci-gocyclo.yml ./...

# Run pre-commit hooks across all files.
pre-commit:
  pre-commit run --all-files

# CI-equivalent checks.
check: fmt-check build test lint cyclo

# Validate GoReleaser config without publishing.
release-check:
  mise exec -- goreleaser check --config .goreleaser.yml

# Build release artifacts locally without publishing.
release-dry-run:
  mise exec -- goreleaser release --clean --skip=publish --config .goreleaser.yml

# Build tip artifacts locally without publishing.
tip-dry-run:
  CHANGELOG_VERSION="$$(sed -n 's/^## \[\([^]]\+\)\].*/\1/p' CHANGELOG.md | head -n 1)" && \
  GORELEASER_CURRENT_TAG="v$${CHANGELOG_VERSION}-tip.local" \
  mise exec -- goreleaser release --clean --skip=validate --skip=publish --config .goreleaser-tip.yml
