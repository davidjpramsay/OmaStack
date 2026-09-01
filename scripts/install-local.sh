#!/usr/bin/env bash
set -euo pipefail

project_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
plugin_dir="${XDG_CONFIG_HOME:-$HOME/.config}/omarchy/plugins/david.omastack"

"$project_dir/scripts/build.sh"
install -d -m 700 "$plugin_dir"
install -d -m 700 "$plugin_dir/qml/components"
install -m 600 "$project_dir/manifest.json" "$plugin_dir/manifest.json"
install -m 600 "$project_dir/marketplace.json" "$plugin_dir/marketplace.json"
install -m 600 "$project_dir/LICENSE" "$plugin_dir/LICENSE"
install -m 600 "$project_dir/qml/BarWidget.qml" "$project_dir/qml/CompactPanel.qml" "$project_dir/qml/Panel.qml" "$project_dir/qml/Service.qml" "$plugin_dir/qml/"
install -m 600 "$project_dir"/qml/components/*.qml "$plugin_dir/qml/components/"
"$project_dir/bin/omastack" setup
omarchy plugin enable david.omastack right
echo "Installed OmaStack at $plugin_dir"
