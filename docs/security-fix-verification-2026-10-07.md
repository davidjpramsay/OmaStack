# OmaStack October security fix verification

Date: 2026-10-07, Australia/Perth

The seven defects confirmed in the [October audit](security-audit-2026-10-07.md)
are fixed in the 0.1.2 candidate based on `91ce9fd49c922877f4f43647dac7a43fc0cc9bb0`.
Permanent regressions and isolated native Docker/systemd acceptance pass. This
is a local engineering verification, not marketplace approval or a guarantee
that no further bugs exist. Local install/profile acceptance is recorded in
[release-acceptance-2026-10-07.md](release-acceptance-2026-10-07.md). Hosted CI and
marketplace decisions must be checked against the current exact commit; they
are not implied by this local report.

## Fixed behavior

| Finding | Fixed behavior and regression |
| --- | --- |
| Stale project edits discard services | Backend updates metadata only; the editor submits no service array. Tests retain active/new services, concurrent edits, secrets and order. |
| Recreate bypasses supervision | Stopped Recreate creates without starting; running Recreate stops and starts through managed orchestration. Native tests verify both states. |
| Force kill leaves containers running | Stop and Force kill explicitly stop Docker-owned containers and verify every returned replica. Delete/import require stopped unit and container state; uninstall retains control files if shutdown fails. |
| Subprocesses inherit ambient credentials | Shared execution policy filters ambient variables and merges explicit service/environment-file values for actions, inspections, readiness, probes and terminal helpers. Tests use synthetic credentials, configured PATH and working-directory checks. |
| Secret classification corrupts values | Explicit `keepFrom` references preserve unchanged stored values, including renames and secret-to-plain changes; literal mask values remain unambiguous. Invalid references fail transactionally and cannot be persisted/imported. |
| Disabled proxy retains tunnels | Each listener generation tracks downstream/upstream connections, cancels active work and closes ordinary/upgraded connections on disable, route/listener change or shutdown. Tests verify revocation and released permits. |
| Configured PATH is ignored | Executable basenames resolve against the constructed child environment. Tests cover inline/file PATH, missing binaries, explicit relative commands and rejection of implicit relative PATH lookup. |

Related hardening requires a Docker service to be stopped before changing its
Compose identity, environment or working directory, preserving control of the
old container. New execution settings are validated before invoking Docker.
Lifecycle actions invalidate display polling; stale container observations
cannot turn a clean stop back into running, and a surviving container cannot
hide a failed supervisor. Successful intentional kills reconcile as stopped.
Orphan-container warnings remain visible between Docker polls, and inspection
errors remain stale/error indicators until a successful poll. A live read-only
check exposed this cached-display edge and permanent regressions cover it.

Terminal brokers can restore user-manager environment variables. The terminal
now launches an OmaStack helper with only the service ID and exact private
config path, then rebuilds the filtered execution environment inside that
terminal. Secret values do not travel in terminal command arguments.

## Verification results

| Gate | Post-fix result |
| --- | --- |
| Complete Go race suite on Go 1.27.0 | Pass, no reported races; 72.4% total statement coverage |
| Complete Go race suite on documented minimum Go 1.25.13 | Pass |
| Go vet | Pass |
| Staticcheck v0.8.1 | Pass |
| govulncheck v1.8.0 source and newly built static binary scans | No vulnerabilities found |
| Gitleaks v8.30.1 history and working-tree scans | No leaks detected; ignored old build binaries excluded from source packaging |
| ShellCheck 0.11.0 and Bash syntax | Pass |
| Native Omarchy QML regression suite | 23 passed, 0 failed, 0 skipped; runtime reference/type/binding errors also fail the runner |
| Omarchy 4.0.4 plugin validation and JSON parsing | Pass |
| Root marketplace preview | Real QML panel rendered at 2x with synthetic data; inspected for clipping and readability |
| Native Docker/systemd lifecycle acceptance | Pass; fixture unit/container removed; installed daemon remains active |

The native test uses a cached Alpine image pinned by its local SHA-256 ID, no
network/ports or image pulls, one unique runtime user unit and an isolated
configuration. It exercises stopped/running Recreate, managed startup, Stop
All, Force kill, orphan deletion refusal, stopped-state reconciliation and
non-purge uninstall shutdown. Uninstall's temporary HOME contains no daemon
unit, so it does not disable the installed daemon. The test is opt-in and its
cleanup targets only its own unit and Compose project.

The automated gate is reproducible with `bash scripts/check-release.sh`; native
acceptance and preview regeneration commands are in [testing.md](testing.md).
The added GitHub workflow pins actions/tool versions and uses read-only token
permissions. Its commands were checked locally; the actual hosted result is
available under the repository's Actions tab after pushing. Native QML/Omarchy
acceptance remains separate from hosted CI.

## Setup and publication requirements

The plugin panel now clearly explains missing-backend setup and recovery, even
when saved projects are visible. Commands are selectable and manual; nothing
is downloaded or executed automatically. README instructions explicitly state
that a library Install action installs the panel only and that backend rebuild
and setup are also required after plugin updates. The root preview and its
native capture fixture are included for supported marketplace discovery.

Publication boundaries:

- A fresh isolated profile passed manual backend setup, native QML connection,
  repeated setup preserving config/service PID, and non-purge removal. Its
  systemctl adapter remapped only fixture units. It is not a fresh desktop login
  or end-to-end plugin-library installation; that acceptance remains pending.
- Package committed source only, rerun gates on the exact SHA, and inspect the
  archive. Old root-level/build binaries and local agent settings are excluded.
- The owner has authorized commit/push, public repository visibility and a
  marketplace review request. Permission is not evidence of publication.
- Submit the manual-setup installation mode and run public marketplace
  verification against the exact release SHA, including maintainer review.
  Any review request must disclose the remaining clean-desktop acceptance gap;
  do not represent this candidate as final marketplace-approved software.

The historical audit retains its original hold verdict for commit `91ce9fd`.
This dated verification records the fixes without retroactively turning the
unfixed commit into an approved release.
