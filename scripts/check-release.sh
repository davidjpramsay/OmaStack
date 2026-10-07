#!/usr/bin/env bash
set -euo pipefail

project_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$project_dir"
for tool in go staticcheck govulncheck shellcheck gitleaks omarchy jq rg; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "Release check requires $tool; see docs/testing.md." >&2
    exit 127
  fi
done

go test -race -count=1 -cover ./...
go vet ./...
staticcheck ./...
govulncheck ./...
shellcheck scripts/*.sh
for script in scripts/*.sh; do bash -n "$script"; done
bash scripts/test-qml.sh
omarchy plugin validate "$project_dir"
jq empty manifest.json marketplace.json examples/full-stack/config.json
test -s preview.png
gitleaks git --redact --no-banner .
gitleaks dir --redact --no-banner --max-target-megabytes 4 .
release_check_dir="$(mktemp -d /tmp/omastack-release-check.XXXXXX)"
trap 'rm -rf -- "$release_check_dir"' EXIT
CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags='-s -w' -o "$release_check_dir/omastack" ./cmd/omastack
govulncheck -mode=binary "$release_check_dir/omastack"
echo "Automated release gates passed. Native lifecycle and clean-account acceptance remain required."
