#!/usr/bin/env bash
set -euo pipefail

project_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"

if ! command -v go >/dev/null 2>&1; then
  echo "OmaStack requires Go 1.25.13 or newer to build." >&2
  echo "Install it on Omarchy with: omarchy pkg add go" >&2
  exit 127
fi

mkdir -p "$project_dir/bin"
cd "$project_dir"
go test ./...
bash "$project_dir/scripts/test-qml.sh"
CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags="-s -w" -o "$project_dir/bin/omastack" ./cmd/omastack
omarchy plugin validate "$project_dir"
echo "Built $project_dir/bin/omastack"
