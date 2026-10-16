#!/usr/bin/env bash
set -euo pipefail

module="github.com/v2fly/v2ray-core/v5"
version="${1:-}"

if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$ ]]; then
  echo "Usage: $0 vMAJOR.MINOR.PATCH" >&2
  exit 2
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
third_party="$repo_root/third_party"
core_dir="$third_party/v2ray-core"
patch_file="$repo_root/patches/v2ray-core-xdrive.patch"
tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/update-v2ray-core.XXXXXX")"
stage_dir="$(mktemp -d "$third_party/.v2ray-core-stage.XXXXXX")"
backup_dir=""

cleanup() {
  if [[ -n "$backup_dir" && -d "$backup_dir" ]]; then
    if [[ ! -e "$core_dir" ]]; then
      mv "$backup_dir" "$core_dir"
    else
      rm -rf "$backup_dir"
    fi
  fi
  rm -rf "$stage_dir" "$tmp_dir"
}
trap cleanup EXIT

if [[ ! -f "$patch_file" ]]; then
  echo "Missing XDRIVE patch: $patch_file" >&2
  exit 1
fi

module_json="$(cd "$repo_root" && go mod download -json "$module@$version")"
module_dir="$(printf '%s\n' "$module_json" | sed -n 's/^[[:space:]]*"Dir": "\([^"]*\)",\{0,1\}$/\1/p')"
if [[ -z "$module_dir" || ! -f "$module_dir/go.mod" ]]; then
  echo "Could not locate downloaded module $module@$version" >&2
  exit 1
fi

mkdir -p "$stage_dir/core"
cp -R "$module_dir/." "$stage_dir/core/"
chmod -R u+w "$stage_dir/core"

if ! patch --directory="$stage_dir/core" --strip=1 --fuzz=0 --forward --batch < "$patch_file"; then
  echo "XDRIVE patch does not apply cleanly to $module@$version; core was not changed." >&2
  exit 1
fi

cp "$repo_root/go.mod" "$tmp_dir/go.mod"
backup_dir="$third_party/.v2ray-core-backup.$$"
if [[ -e "$backup_dir" ]]; then
  echo "Core backup path already exists: $backup_dir" >&2
  exit 1
fi
mv "$core_dir" "$backup_dir"
if ! mv "$stage_dir/core" "$core_dir"; then
  mv "$backup_dir" "$core_dir"
  backup_dir=""
  exit 1
fi

if ! (cd "$repo_root" && go mod edit -require="$module@$version"); then
  rm -rf "$core_dir"
  mv "$backup_dir" "$core_dir"
  backup_dir=""
  cp "$tmp_dir/go.mod" "$repo_root/go.mod"
  exit 1
fi

rm -rf "$backup_dir"
backup_dir=""
echo "Updated $module to $version and reapplied the XDRIVE patch."
