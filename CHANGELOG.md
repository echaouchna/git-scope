# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
