#!/usr/bin/env bash
set -euo pipefail

omastack uninstall
omarchy plugin disable david.omastack || true
echo "Definitions are preserved. Remove the plugin code with: omarchy plugin remove david.omastack"
echo "To purge definitions explicitly: omastack uninstall --purge"
