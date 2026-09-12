#!/usr/bin/env bash
set -euo pipefail
project_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
shell_dir="${OMASTACK_TEST_SHELL_DIR:-/usr/share/omarchy/shell}"
runner="${OMASTACK_QML_RUNNER:-/usr/lib/qt6/bin/qmltestrunner}"
if [[ ! -x "$runner" || ! -d "$shell_dir/Ui" ]]; then
  echo "QML tests require Qt Quick Test (qt6-declarative) and the installed Omarchy shell." >&2
  exit 1
fi
test_dir="$(mktemp -d /tmp/omastack-qml.XXXXXX)"
trap 'rm -rf -- "$test_dir"' EXIT
ln -s "$shell_dir" "$test_dir/qs"
ln -s "$project_dir/tests/qml/stubs/Quickshell" "$test_dir/Quickshell"
QT_QPA_PLATFORM=offscreen QT_QPA_PLATFORMTHEME='' QT_QUICK_BACKEND=software \
  "$runner" -input "$project_dir/tests/qml" -import "$test_dir" "$@" | tee "$test_dir/output.log"
# Some runner builds return zero despite reporting a failing QML test.
if rg -q '^(FAIL!|QFATAL)|Totals:.*[1-9][0-9]* failed' "$test_dir/output.log"; then
  exit 1
fi
