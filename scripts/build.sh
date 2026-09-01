#!/usr/bin/env bash
set -euo pipefail

project_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
mkdir -p "$project_dir/bin"
cd "$project_dir"
go test ./...
CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags="-s -w" -o "$project_dir/bin/omastack" ./cmd/omastack
omarchy plugin validate "$project_dir"
echo "Built $project_dir/bin/omastack"
