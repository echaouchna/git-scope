# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
