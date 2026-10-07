# Changelog

All notable changes to OmaStack are recorded here. The project follows
Semantic Versioning.

## [0.1.2] - 2026-10-07

- Made project updates metadata-only so stale snapshots cannot discard services.
- Kept stopped Docker Recreate stopped and running Recreate under managed
  dependency/readiness orchestration; Stop/Force kill/uninstall explicitly stop
  Docker-owned containers, and deletion/import/execution edits fail closed.
- Shared filtered environment, working-directory and PATH resolution across
  supervised commands, Docker helpers and command health probes. Terminal
  helpers reload the exact private configuration without secrets in argv.
- Preserved unchanged secret values across classification changes and renames
  using explicit keep references instead of interpreting a mask as a value.
- Revoked active proxy streams and upgraded tunnels on disable, route changes,
  listener changes and shutdown.
- Added Docker/systemd native acceptance, permanent audit regressions, pinned
  CI security gates, manual backend onboarding and a reproducible root preview.
- Recorded post-fix verification on 2026-10-07 separately from the original audit.
- Kept orphan-container warnings and Docker inspection errors visible between
  polls, without letting invalidated pre-stop caches undo intentional shutdown.

- Fixed empty panels under replacement bars whose scoped shell cannot return
  OmaStack's shared service; the widget now owns a client when needed.
- Added widget-specific live diagnostics and a build gate for QML regressions.
- Retained offline project snapshots and removed misleading setup instructions
  during disconnection.
- Fixed project creation/reset failing because the arguments field shadowed
  JavaScript's `arguments` object.
- Packaging now rejects untracked source files as well as modified tracked files.

- Added persistent browser/stop/overflow controls. Browser access uses the
  configured URL, an active route, or an explicit port picker.
- Made rejected configuration updates transactional, settings edits atomic,
  and route uniqueness consistent across whitespace/case/trailing-dot aliases.
- Bound oversized log-line draining, journal merging and queued health work;
  old process/container generations cannot supply current readiness results.
- Fixed targeted shutdown, dependency-aware session autostart and cancellation
  of queued aggregate starts; shared Compose prerequisites have separate owners.
- Prevented duplicate Compose stop signals with systemd `KillMode=mixed` and
  allowed CLI completion time after the container's configured stop grace.
- Preserved hidden editor fields, zero values, opaque arguments and IPv6;
  failed saves retain the draft and successful saves wait for acknowledgment.
- Improved narrow-panel scrolling, log readability, history settings, route
  URLs/activity, recovery notification settings and bounded stale-state polling.
- Added regression coverage and recorded final verification on 2026-09-12.

## [0.1.1] - 2026-09-02

- Preserved clean operator stops for Docker services even when Compose reports
  the stopped container with exit code 137, while retaining real crash reports.
- Wired the Settings `logBufferLines` value through the QML log request and
  aligned the bounded journal reader with the validated 50,000-line maximum.
- Forced all production QML text and log-message rendering to plain text so
  imported labels and operational errors cannot be interpreted as markup or
  initiate inline resource requests.
- Replaced full user-manager environment inheritance with a documented baseline
  allowlist; explicit service and private environment-file values still layer on
  top.
- Suppressed the expected closed-pipe diagnostic during clean supervisor
  shutdown while retaining genuine stream-read errors.
- Added regression coverage for Docker reconciliation, QML text policy, log
  settings, environment inheritance, CLI, paths, systemd, control, install,
  logs, configuration mutations and reproducible source packaging.
- Increased project/service-card contrast with native selected-surface tokens,
  clearer stopped/secondary content and stronger compact graph rendering.
- Sized the stack popup directly from its content so it ends just below the
  footer instead of reserving unused vertical space.
- Replaced the tiled two-pane workbench with a narrow, bar-anchored Omarchy
  `KeyboardPanel`: one column, collapsible project groups, 38-unit service rows
  and in-place metric expansion.
- Added an enabled-by-default `showStatusCounts` bar setting for the four
  running, stopped, unhealthy and crashed totals beside the OmaStack icon.
- Exposed that setting as an Appearance toggle inside the OmaStack panel.
- Upgraded CPU and memory sparklines with smooth cubic curves and a subtle
  theme-derived gradient fill while retaining bounded histories.
- Matched Omarchy's standard 380-unit companion-panel width across the daily
  view, logs, settings, wizards and editors; height still follows content.
- Prevented the panel from restoring open after a shell/plugin reload.
- Stopped the internal panel-visibility request from flashing a misleading
  `Working…`/`Done` toast.
- Added a compact actionable empty state and right-click service editing.
- Fixed project disclosure so an intentionally collapsed group stays collapsed,
  and added keyboard cursor/Enter/Space disclosure control.
- Rebuilt the service editor for the standard 380-unit panel: short fixed tabs,
  mode-specific Host/Shell/Compose fields, compact copy and no page-level
  scrolling. Only unbounded variable and dependency collections scroll inside
  their own fixed regions.
- Corrected project/service grouping and low-contrast copy across dark themes:
  child services now use an indented status rail, expanded rows use restrained
  native control surfaces, and essential captions derive from popup foreground
  instead of the decorative `muted` palette slot.
- Compressed the daily disclosure stack: 38-unit project headers, 34-unit
  service rows and 82-unit expanded metric cards with 30-unit sparklines.

## [0.1.0] - 2026-08-31

### Added

- Omarchy schema-v1 bar widget, large panel and persistent service entry
  points, themed through `qs.Commons` and `qs.Ui`.
- Project/service wizard and editors for commands, explicit shell mode,
  environment, dependencies, health, routes, Docker and lifecycle policies.
- Stable CLI and same-user bounded Unix-socket protocol.
- UUID-backed `systemd --user` supervision with dependency ordering, graceful
  stops, restart policy, force-kill and restart reconciliation.
- CPU, memory, TCP port and bounded sparkline history monitoring.
- HTTP(S), TCP and executable health checks with bounded concurrency and
  transition notifications.
- Redacted journald logs, per-service/combined project views, search and follow.
- Optional Docker Compose import, lifecycle, stats, logs, rebuild/recreate and
  terminal support, including visual no-start discovery into a selected project.
- Loopback HTTP routing with strict local hostname/Host validation and route
  conflict diagnostics.
- Mode-`0600` validated export/import, doctor, cleanup, non-destructive disable,
  uninstall and confirmation-gated purge workflows.
- Architecture, threat model, storage/dependency/privilege documentation,
  CMDHub reference research, three-theme design review and full-stack example.

### Security

- Executable/argument arrays avoid implicit shell parsing; shell mode is
  explicit and visibly marked.
- Secrets are redacted before journald and again when read, and mutations never
  echo secret values.
- User data directories are mode `0700`; sensitive files and the control socket
  are mode `0600`; symlink and unsafe purge targets are refused.
- Config/environment/import reads use `O_NOFOLLOW`, regular-file and permission
  checks; lifecycle/config operations are serialized and interruptible.
- Setup refuses install directories owned by another UID or writable by group/
  other, and Compose discovery uses verified stdin with interpolation disabled.
- Control, log, Docker, health, proxy and monitoring reads have explicit size,
  concurrency and time limits; all forwarded/original identity headers are
  replaced at the proxy.
- TLS certificate verification cannot be disabled, and non-redactable
  subprocess diagnostics are not surfaced through UI/API/notifications.
- Automatic DNS and trusted local HTTPS remain explicitly disabled until a
  narrowly scoped privileged onboarding/helper can be implemented safely.
