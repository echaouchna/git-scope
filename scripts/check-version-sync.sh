#!/usr/bin/env bash
set -euo pipefail

changelog_version="$(
  awk '
    /^## \[/ {
      line = $0
      sub(/^## \[/, "", line)
      sub(/\].*$/, "", line)
      print line
      exit
    }
  ' CHANGELOG.md
)"

app_version="$(
  awk -F'"' '/^var Version = "/ { print $2; exit }' internal/app/version.go
)"

if [[ -z "${changelog_version}" ]]; then
  echo "error: could not read latest version from CHANGELOG.md"
  exit 1
fi

if [[ -z "${app_version}" ]]; then
  echo "error: could not read Version from internal/app/version.go"
  exit 1
fi

if [[ "${changelog_version}" != "${app_version}" ]]; then
  echo "error: version mismatch"
  echo "  CHANGELOG.md latest version: ${changelog_version}"
  echo "  internal/app/version.go:    ${app_version}"
  exit 1
fi

echo "ok: version sync (${app_version})"
