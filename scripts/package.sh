#!/usr/bin/env bash
set -euo pipefail

project_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
version="$(jq -er '.version' "$project_dir/manifest.json")"
archive_name="OmaStack-$version.tar.gz"
output="${1:-$project_dir/dist/$archive_name}"

git -C "$project_dir" rev-parse --verify HEAD >/dev/null
if [[ -n $(git -C "$project_dir" status --porcelain --untracked-files=all) ]]; then
  echo "package: refusing to archive a dirty working tree" >&2
  exit 1
fi

mkdir -p "$(dirname -- "$output")"
temporary="$(mktemp -d)"
trap 'rm -rf -- "$temporary"' EXIT

git -C "$project_dir" archive --format=tar --prefix="OmaStack-$version/" HEAD >"$temporary/source.tar"
gzip -n -9 <"$temporary/source.tar" >"$output"
sha256sum "$output" >"$output.sha256"

echo "Packaged $output"
echo "Checksum $output.sha256"
