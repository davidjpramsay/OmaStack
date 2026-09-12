# Four retained security findings — focused fix verification

Scope: the four distinct findings retained by the cancelled deep scan of
revision `4f02a6e755c36dac947af4b71164d434227dbbdd`.
The deep scan remains cancelled; this is not a repository-wide sign-off.

## Boundaries and fixes

1. **Inline secrets in process arguments.** Both queued and urgent QML requests
   now use `omastack request <method> -` and send compact JSON over stdin.
   The CLI reads one bounded line (512 KiB maximum), rejects malformed input
   without echoing it, and preserves default/positional JSON compatibility.
   QML clears the transport's retained payload after writing. Existing editor
   secret state is still needed while the user edits. External callers must
   also choose stdin rather than putting secrets in legacy positional JSON.
2. **Blocking special-file open.** `ReadRegular` opens with `O_NONBLOCK` before
   its existing descriptor-based regular-file, permission and size validation.
   FIFOs without writers are rejected immediately. This central helper covers
   Compose/config imports, Compose startup discovery, environment and state
   reads, and installation reads without weakening symlink protection.
3. **Proxy starvation.** A shared explicit upstream transport bounds header
   waiting to 30 seconds and connection inactivity to 60 seconds, including
   response bodies and upgraded connections. Activity in either direction
   refreshes both I/O deadlines. Admission is limited to 16 requests per stable
   service ID and 64 overall; aliases/reconfiguration do not reset active
   counts. Environment HTTP proxies cannot redirect loopback upstream traffic.
   Active streams remain supported; silent connections must reconnect.
4. **Redaction amplification.** The shared text redactor deduplicates and
   prioritizes longer patterns, replaces only original input, and caps retained
   transformed output at 1 MiB. It omits the entire message on overflow instead
   of exposing a partially transformed prefix. Generated masks cannot be
   processed again by subsequent patterns.

Implementation files: `qml/Service.qml`, `cmd/omastack/main.go`,
`internal/securefile/read_linux.go`, `internal/proxy/proxy.go`,
`internal/redact/redact.go`. README documents compatibility and timeout limits.
Regression tests were added to each owning package, daemon import integration,
and `tests/qml/tst_release.qml`; the QML Process stub models stdin writes.

## Ordered validation

- Syntax/build: `gofmt`, `git diff --check`, `go build ./cmd/omastack`, and
  `go vet ./...` passed with Go 1.25.13.
- Security triggers and alternate cases: FIFO/no-writer and directory rejection;
  Compose and backup import followed by a successful settings mutation;
  duplicate mask-character secrets, overlapping patterns and oversized output;
  malformed/oversized stdin; stalled proxy headers and bodies; same-service
  alias saturation while a healthy route succeeds; cancellation frees permits.
- Legitimate controls: regular private files and existing size/symlink checks;
  unchanged public log text and ordinary secret masking; default/legacy CLI
  JSON and Unicode/newline-bearing stdin; live HTTP streaming and bidirectional
  and one-way upgraded traffic beyond the injected inactivity interval.
- Full `go test -race -count=1 ./...` passed across 19 packages. Focused daemon
  and proxy race tests were rerun after the final added import regressions.
- `bash scripts/test-qml.sh`: 20 passed, zero failed/skipped. Both queued and
  urgent request commands exclude secret payloads and write exact JSON to stdin.
- Real offscreen Quickshell smoke test: native Process -> built CLI -> isolated
  Unix socket delivered a 300,000-character Unicode/newline-bearing payload
  and a concurrent urgent request exactly. The mock server checked the live
  CLI process arguments using its socket peer PID: neither contained the secret.
  Temporary fixture: `/tmp/omastack-stdin-smoke.rIj36d/`.
- Staticcheck passed with the matching Go toolchain and a writable temporary
  cache. Initial runs encountered a system-toolchain export-format mismatch
  and sandbox cache permissions; neither was a source finding.

The security fix workflow included one independent read-only investigation and
one candidate review. Review found a one-way upgraded-stream timeout regression;
the shared activity deadline and added one-way traffic test address it.
Verification did not modify production services or user configuration.
No live cross-user exploit or memory-exhaustion attack was performed;
bounded regression fixtures exercise those paths safely instead.

## Approved local installation

After verification, the user requested installation and push. The release build
and all 20 QML tests passed again. The new binary and `qml/Service.qml` were
installed, and only `omastackd.service` was restarted. The installed binary
matches the build (SHA-256
`b319208da6b4cf620f0079f1dc579b5cc6792c6094bb24c9b20a997cd067c146`).
`scripts/check-live.sh` confirms the panel is connected with both projects.
The user configuration hash is unchanged. The existing Studio supervisor PID
remained 1176: it must restart before its in-process redactor uses the new code.
Newly started supervisors use the fixed binary immediately.
Backups of the old binary and Service.qml are in
`/tmp/omastack-security-install.C1cf9j/`.
