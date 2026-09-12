# Native acceptance: 2026-09-12

Verified locally on Omarchy 4.0.3 / Qt 6.11.2 with the user's replacement
`david.bar`, using the installed plugin and systemd user daemon.

## Root cause

The widget used `bar.shell.serviceFor("david.omastack")`, but replacement bars
receive a restricted shell facade which cannot return another plugin's service.
The persistent service's IPC status was healthy while the panel binding was
null. Backend tests, installed file timestamps and daemon status did not catch
this. Earlier suggestions involving PATH, stale QML and runtime-file visibility
were not demonstrated causes of this incident.

`BackendConnection.qml` now reuses a shared service where supplied, otherwise
creates a widget-owned client with service IPC registration disabled. The
backend process remains a single systemd service. All UI remains native
QML/Quickshell with Omarchy's Panel/KeyboardPanel and theme controls.

## Evidence

- All Go packages pass, including `go test -race ./...` and `go vet ./...`.
  Run socket/race checks from the host session; the agent sandbox cannot access
  all required IPC/runtime resources.
- QML regression runner: 19 passed, zero failed. Covers restricted-bar fallback,
  shared-service reuse, project retention on invalid/stale snapshots, native
  editors, keyboard controls, settings, logs and three theme layouts.
- `omarchy plugin validate` accepts the source and installed native manifest.
- The actual widget's IPC reports `client: local`, `connected: true`, the two
  original project names and `visibleProjectCount: 2` while its panel is open.
- During an intentional backend stop, the open panel reports disconnected and
  retains both project rows. Starting the backend reconnects automatically.
- A temporary real `/usr/bin/sleep` service passed project creation, systemd
  start, running observation, stop and deletion. The original config's SHA-256
  was identical before and after this test. AniMauth stayed running and Mauth
  Dev stayed stopped.
- No OmaStack QML errors were found in the live journal after the fix.

## Repeating the checks

```bash
scripts/install-local.sh   # Go + QML tests, build, manifest, install
omarchy restart shell
bash scripts/check-live.sh # compares widget project IDs with daemon IDs
go test -race ./...
go vet ./...
```

This is local acceptance, not a guarantee across other shell versions. Docker
and proxy behavior was not newly exercised live during this follow-up; see the
dated prior acceptance in testing.md and the current automated tests.

The tested source candidate includes the current checkout's changes and new
source/test files. It is not a tagged release. Normal `scripts/package.sh`
requires all intended sources to be committed, and rejects untracked files so
a release cannot silently omit required code.
