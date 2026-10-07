# OmaStack 0.1.2 local acceptance

Date: 2026-10-07, Australia/Perth

The October audit fixes passed the automated release gate and isolated native
Docker/systemd acceptance. The candidate was installed into the existing
Omarchy desktop. This record is local engineering evidence, not marketplace
approval or a guarantee of safety.

## Automated checks

`scripts/check-release.sh` passed: complete race/coverage suite, vet,
Staticcheck 0.8.1, govulncheck 1.8.0 source and static-binary checks, ShellCheck
0.11.0, Bash syntax, native QML tests, plugin/JSON validation, preview presence
and Gitleaks 8.30.1 history/working-tree scans. Statement coverage was 72.4%.
The QML suite reported 23 passed,
0 failed and 0 skipped. The complete race suite also passed on Go 1.25.13, the
documented minimum, as well as the installed Go 1.27.0 toolchain.

The opt-in Docker acceptance passed stopped/running Recreate, managed start,
Stop All, Force kill, orphan deletion refusal, stopped reconciliation and
non-purge uninstall shutdown. It used an already-cached SHA-256-pinned Alpine
image, no published ports or image pulls, and unique temporary runtime units.
Only its fixtures were removed.

## Fresh profile and native QML connection

A temporary private HOME and separate XDG config/state/cache/runtime were used
with the actual candidate CLI and actual Quickshell `Service.qml`, not QML test
stubs. A narrow systemctl adapter translated daemon/template/one fixture ID to
unique runtime user units, rewrote `%h` to the profile HOME, and supplied its
environment. Unit sandbox settings, including `PrivateTmp`, were retained.
The real installed daemon and service template were never replaced by this
profile test.

Verified behavior:

- Manual setup installed executable/user-unit files and served an initially
  empty connected configuration.
- A fixture host service started under a real systemd user unit.
- Repeated panel copy/backend setup preserved the configuration checksum and
  running child PID across a daemon restart.
- An offscreen native Quickshell client connected and reported one project and
  one running service, without runtime binding/reference/type errors.
- Non-purge uninstall stopped its daemon/service, removed its binary, unit
  files and socket, and preserved its configuration checksum.
- Removing the explicitly scoped panel copy left no active fixture units.

An initial harness attempt under `/tmp` failed because `PrivateTmp` correctly
hid its executable. Repeating with the profile under the workspace passed;
this was a fixture-layout failure, not a production unit change.

This is not a separate Unix account, interactive desktop login, or an actual
`omarchy plugin add/update/remove` test. A clean Omarchy account or VM must still
exercise those documented commands, panel activation and removal before final
release sign-off. A marketplace review request must disclose that limitation.

## Existing desktop

The local installer rebuilt and validated 0.1.2, installed backend/user units
and enabled the existing widget. Plugin rescan and `scripts/check-live.sh`
passed: the actual widget was connected and shared both backend project IDs.
The user's configuration checksum was unchanged before/after installation,
and the existing host service kept its running child PID. No credentials
or private configuration are included in this repository or preview.

An existing Docker container was running outside an inactive OmaStack unit.
Read-only testing exposed its status alternating between warning and stopped
on cached polls. Four post-install live observations kept the warning consistent.
The candidate retains
inspection failures between polls, and never adopts or restarts that external
container. Regression tests cover both cached-display cases.

## Commit, archive and publication

Commit only reviewed source. Package from a clean checkout of that commit and
exclude old binaries, private configuration and agent-local settings. Verify
hosted CI for the exact pushed SHA. Publish source only with the owner's
permission and request the marketplace's exact-commit checks/maintainer review.
Do not label a plugin approved or verified yourself.

Hosted CI, public visibility and submission status are external mutable facts;
check the repository and submission issue rather than inferring them from this
dated document. Manual backend build/setup is required after library installation
and updates. Nothing downloads or executes an installer automatically.
