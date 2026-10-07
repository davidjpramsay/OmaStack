#!/usr/bin/env bash
set -euo pipefail
project_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$project_dir"
capture_stamp="$(mktemp /tmp/omastack-preview.XXXXXX)"
trap 'rm -f -- "$capture_stamp"' EXIT
OMASTACK_QML_TEST_DIR="$project_dir/tests/preview" bash scripts/test-qml.sh
if [[ ! -s preview.png || ! preview.png -nt "$capture_stamp" ]]; then
  echo "Preview capture did not produce a fresh preview.png." >&2
  exit 1
fi
echo "Captured preview.png from the real panel with synthetic sample data."
