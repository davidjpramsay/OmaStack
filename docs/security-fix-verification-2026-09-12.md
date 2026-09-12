# Security-fix verification — 2026-09-12

This record covers the combined source candidate, including the separately
verified replacement-bar/offline-state fix. It is not a vulnerability-free
certificate or a completed repository-wide Codex Security Deep Scan.

## Boundaries verified

- Rejected configuration mutations cannot change live nested configuration or
  its persisted copy; valid updates still work. `internal/store/rollback_test.go`
  covers validation, mutator and write failures plus copy isolation.
- Imported text remains plain text, opaque command arguments retain their
  boundaries, hidden editor settings survive saves, and invalid saves keep
  the draft. Existing QML policy checks and `tests/qml/tst_release.qml` cover
  these paths, including empty arguments, IPv6 and zero grace values.
- Startup expands prerequisites, but targeted stops do not stop shared
  prerequisites. Health results are tied to process/configuration and actual
  Docker container start identities. `internal/daemon/readiness_test.go` and
  `release_regression_test.go` reject stale success, native unhealthy/missing
  checks, unmanaged required Compose dependencies and dependency cycles, while
  accepting healthy/current and successfully completed dependency controls.
- Oversized log lines are drained and omitted whole rather than leaking a
  secret prefix or blocking the child. Log capture/merge/JSON-response sizes
  and outstanding health jobs are bounded. Tests include repeated generation
  changes, escaped JSON message sizes and newest-entry selection across sources.
- Canonical route aliases cannot bypass uniqueness; disabled listeners do not
  advertise active routes. Settings patches cannot overwrite unrelated edits.
- Poll timeouts retain explicitly stale last-known state instead of inventing
  stops; the timeout fixture contains 128 services across two valid projects.

The fixes live at the existing store, supervisor, readiness, settings, logging
and route boundaries, with their callers updated consistently. The intended
same-user model remains: explicitly imported commands and Compose files are
trusted executable policy, not sandboxed untrusted applications.

## Checks

Toolchain: official Go 1.25.13 linux-amd64, archive SHA-256
`39042a078ea9ceebe3ecda4a7188f0f5b96e14a071d27923ba7f40b456e85ae3`.
Qt 6.11.2 / Omarchy 4.0.3; Staticcheck v0.7.0, govulncheck v1.6.0,
ShellCheck v0.11.0.

| Gate | Result |
|---|---|
| `go test -count=1 ./...` | Pass in host context; sandbox lacks Unix sockets |
| `go test -race -count=1 -coverprofile=coverage.out ./...` | Pass with local socket access; no race reports |
| `go tool cover -func=coverage.out` | 65.2% total; daemon 61.0% |
| `go vet ./...` | Pass |
| `staticcheck ./...` | Pass |
| `govulncheck ./...` | No known vulnerabilities found in this code/toolchain |
| `bash scripts/test-qml.sh` | 19 passed, zero failed/skipped; no QML warnings |
| Production QML parsing with `qmlformat` | Pass |
| `shellcheck scripts/*.sh`, per-script `bash -n` | Pass |
| `omarchy plugin validate .`, JSON parsing, `git diff --check` | Pass |
| Static release-style Go build into an isolated temporary directory | Pass |
| `gitleaks git --redact --log-opts=--all` | No leaks found in existing Git history |
| `gitleaks dir --redact` | No leaks found in the working tree (Gitleaks 8.30.1) |

Early failures were not suppressed: two invalid test fixtures were corrected
(timeout exceeding interval and over 64 services in one project), an empty
shell assignment was quoted, Staticcheck error-message style was corrected,
and the QML route lookup was aligned with backend hostname canonicalization.
The offline-snapshot test needs actual Unix socket access, so the final race
run was outside the restricted sandbox using isolated fixtures.

## Real Docker/systemd shutdown reproduction

A disposable Compose project used two containers from an already-cached image,
with no host mounts, published ports or network, read-only root filesystems,
dropped capabilities and no-new-privileges. The web service declared a database
dependency. A uniquely named transient user unit ran the freshly built
OmaStack supervisor; no installed project definitions or service units were
changed by this fixture.

The initial production-policy reproduction failed: systemd's control-group
signal and the supervisor's forwarded signal could both reach Compose, and
the supervisor timeout raced Compose's container-stop timeout. The web
container remained running after the unit stopped.

The patched unit uses `KillMode=mixed`, so the supervisor owns initial signal
forwarding while systemd retains final cgroup cleanup. Docker's container grace
is unchanged; the supervisor allows five additional seconds for CLI completion.
Both fit the existing 310-second unit deadline. Attached Compose runs use
`--no-deps`, keeping each managed dependency under its own supervisor.

The same fixture then passed: web stopped, the database stayed running with
the same start timestamp, the runtime record reported exit zero/no signal,
and the journal had no closed-stream errors. Both disposable containers and
the transient unit were removed afterward; only test logs remain.

## Native UI and remaining release limits

The archived task “Diagnose missing OmaStack apps” independently tested the
replacement-bar integration and a real host-service lifecycle; see
[native acceptance](native-acceptance-2026-09-12.md). Its source changes were
preserved in this candidate. A fresh read-only `bash scripts/check-live.sh`
confirmed the installed widget was connected, retained both original projects,
and had no reported error.

The final Docker stop-policy change is source-tested, not deployed into the
user's live plugin by this verification. Install the committed candidate to
activate that update. Proxy networking was not re-exercised live in this pass;
its automated checks passed. The earlier Deep Scan has no completed canonical
result available here, so it is not used as evidence of security sign-off.
No plugin-store submission or new release tag was performed.
