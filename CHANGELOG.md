# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.2.0] - Unreleased

### Added
- Pre-commit configuration with recommended repository hygiene hooks.
- Local pre-commit hook to validate that `internal/app/version.go` matches the latest version entry in `CHANGELOG.md` (including unreleased versions).
- Dedicated non-tag GitHub Actions workflow to publish nightly/snapshot builds from `main`.
- Dedicated GoReleaser tip configuration for snapshot publishing.
- Homebrew tip cask publishing as `git-scope-tip` while keeping the installed binary name as `git-scope`.
- Added `justfile` with common developer commands (build, `/tmp` build, test, lint, release checks).
- Added strict cyclomatic-complexity repro config via `.golangci-gocyclo.yml` and `just cyclo`.

### Changed
- Open Project modal now uses a single input for both option search and custom command entry.
- When no open-option matches are found, pressing `Enter` now runs the typed command in the selected repository directory.
- Removed the explicit `Run command...` entry from the Open Project menu.
- Updated Open Project modal/help text to reflect the new search-or-run behavior.
- Main dashboard now shows a dedicated selected-repo path bar for easier identification when repositories share the same name.
- Repository search now matches full repository paths in addition to name/branch, with stricter multi-term matching to reduce irrelevant fuzzy results.
- Last Action Logs modal now colorizes log output for clearer scanability (`OK`/additions in green, `ERROR`/deletions in red).
- Last Action Logs modal scrolling now keeps the modal border stable and prevents offset/render desynchronization.
- CI and snapshot Homebrew publishing workflows were merged into a single `ci.yml`.
- Tip Homebrew publish now derives the version from the top (unreleased) version in `CHANGELOG.md`.
- Tip Homebrew publish no longer requires creating a git tag; it uses an ephemeral `GORELEASER_CURRENT_TAG`.
- Tip Homebrew publish skips GoReleaser git-tag validation to support no-tag snapshot publishing.
- Refactored TUI handlers/render helpers to reduce cyclomatic complexity and satisfy strict `>15` checks.
- Open-project menu now caches tool availability checks (`nvim`, `gitui`, `tig`) to speed up opening the menu.

## [1.1.0] - 2026-03-05

### Added
- Background filesystem watcher for real-time status refresh of dirty/staged/untracked repository state.

### Changed
- Batch git actions now run in parallel (bounded worker pool).
- Git actions modal keeps the last run status/progress summary visible after completion.
- Git actions modal now shows a live spinner while batch actions are running.
- Git actions completion summary is now colorized:
  - green when all repositories succeed
  - red when one or more repositories fail

### Fixed
- GoReleaser/Homebrew publishing configuration updated to current `homebrew_casks`.
- Release workflow now uses split tokens: default `GITHUB_TOKEN` for release assets and dedicated tap token for Homebrew repo updates.
- GoReleaser release retries now replace existing artifacts instead of failing on duplicate asset names.

## [1.0.0] - 2026-03-05

### Added
- TUI dashboard for scanning and managing multiple Git repositories.
- Configurable ignored folders during scan (for example: `.terraform`).
- In-TUI batch git actions:
  - `pull --rebase`
  - `switch branch`
  - `create branch`
  - `merge --no-ff`
- Branch autocomplete in git actions using common branches across selected targets.
- Repo multi-select workflow (`Space`, `Ctrl+A`) for batch operations.
- Command palette (`Ctrl+P`) with searchable actions.
- Keyboard shortcuts overlay (`?`) with scroll support.
- Last action logs viewer (`l`) with explicit open/close behavior.
- Workspace switch modal (`w`) with path autocomplete and default current workspace.
- Open project menu on `Enter` with Neovim (when installed) and VS Code options.
- Open-project menu now includes `gitui` and `tig`.
- GitHub Actions workflows for CI and release automation.
- GoReleaser configuration including Homebrew tap publishing.
- MIT license and refreshed README documentation.
