#!/usr/bin/env bash
set -euo pipefail

# Read-only acceptance of the widget binding, not just its background service.
backend="$("$HOME/.local/bin/omastack" status --json)"
widget="$(omarchy shell david.omastack.widget status)"
jq -en --argjson backend "$backend" --argjson widget "$widget" '
  ($backend.connected == true) and ($widget.connected == true) and
  (([$backend.projects[].id] | sort) == ($widget.projectIds | sort))
' >/dev/null || {
  echo "OmaStack panel is not connected to the current backend projects." >&2
  exit 1
}
jq -n --argjson widget "$widget" '$widget | {connected,client,projects,visibleProjectCount,opened,error}'
