#!/bin/sh
set -eu

version="$(tr -d '\r\n' < VERSION)"
frontend_version="$(node -p "require('./frontend/package.json').version")"
lockfile_version="$(node -p "require('./frontend/package-lock.json').version")"
tag="${1:-}"

if ! printf '%s\n' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
  printf 'VERSION 必须是稳定的 SemVer，例如 1.2.3；当前值：%s\n' "$version" >&2
  exit 1
fi

if [ "$frontend_version" != "$version" ]; then
  printf 'frontend/package.json (%s) 与 VERSION (%s) 不一致\n' "$frontend_version" "$version" >&2
  exit 1
fi

if [ "$lockfile_version" != "$version" ]; then
  printf 'frontend/package-lock.json (%s) 与 VERSION (%s) 不一致\n' "$lockfile_version" "$version" >&2
  exit 1
fi

if ! grep -Fq "## [$version] - " CHANGELOG.md; then
  printf 'CHANGELOG.md 缺少版本 %s 的正式发布记录\n' "$version" >&2
  exit 1
fi

if [ -n "$tag" ] && [ "$tag" != "v$version" ]; then
  printf 'Git Tag (%s) 与 VERSION (v%s) 不一致\n' "$tag" "$version" >&2
  exit 1
fi

printf '版本检查通过：%s%s\n' "$version" "${tag:+ ($tag)}"
