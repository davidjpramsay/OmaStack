# Verification record

Date: 2026-09-02

Machine: Omarchy 4.0.2-1, Quickshell 0.3.1, systemd 261.2,
Linux 7.1.9-arch1-2 x86-64

## Automated gates

The complete suite passed with the official Go 1.25.13 linux-amd64 archive.
Its SHA-256 was verified against Go's release JSON as
`39042a078ea9ceebe3ecda4a7188f0f5b96e14a071d27923ba7f40b456e85ae3`.
The suite also passed with Omarchy's Go 1.27.0 Arch package, exercised from a
temporary directory without installing a system package.

```text
go test -count=1 ./...
PASS (all packages)

go test -race -count=1 ./...
PASS (all packages; no data races)

go vet ./...
PASS

staticcheck ./...
PASS — Staticcheck 2026.1 (v0.7.0)

govulncheck ./...
PASS — govulncheck v1.6.0: No vulnerabilities found

shellcheck scripts/*.sh
PASS — ShellCheck 0.11.0
```

The minimum build version is Go 1.25.13. A comparison scan using Go 1.25.11
found reachable standard-library advisories in HTTP and certificate handling;
the patched minimum and release build eliminate those results.

Statement coverage increased from 28.4% to 51.4%. The areas called out in the
release review now have focused fake-process or fake-command tests:

| Package | Coverage |
|---|---:|
| control | 78.3% |
| daemon | 32.1% |
| Docker | 84.5% |
| install | 70.9% |
| logs | 89.6% |
| systemd | 88.0% |
| total | 51.4% |

Daemon orchestration remains the largest unit-test gap because its main loop
coordinates live systemd, Docker, health, proxy and notification boundaries.
The live acceptance checks below cover that integration rather than replacing
those boundaries with a broad in-process mock.

The suite specifically covers configuration migration and validation, atomic
private writes, dependency ordering, restart policies, health transitions,
process/runtime records, journald bounds and filtering, proxy headers, secret
masking, same-user Unix-socket calls, Docker discovery/inspection/actions,
systemd state parsing and lifecycle commands, install/setup/uninstall, safe
purge behavior and the checked-in full-stack example.

New regression checks verify that:

- an intentional systemd stop remains `stopped` even if Docker reports exit
  137, while an unexpected exit 137 remains `crashed`;
- `logBufferLines` flows from the snapshot settings into the UI log request
  and the backend accepts the validated 50,000-line maximum;
- all 78 repository-owned production QML `Text` nodes and the log `TextEdit`
  use plain-text rendering;
- ambient service inheritance excludes representative credentials and loader
  variables while explicit editor/environment-file values still work;
- clean closed streams are silent while genuine scanner errors are retained.

## QML and plugin validation

Every production QML file parsed successfully with Qt 6 `qmlformat` and
`qmllint` exited zero using an import shim for the installed Omarchy
`qs.Commons` and `qs.Ui` modules. Standalone lint still emits expected warnings
for dynamic Omarchy singleton properties, delegate qualification and
`QProcess::ExitStatus`; there were no syntax or missing-component errors.

```text
omarchy plugin validate .
PASS

jq empty manifest.json marketplace.json examples/full-stack/config.json
PASS

bash -n scripts/*.sh
PASS
```

The markup/network regression was also reproduced with an offscreen Qt 6
canary. The old `Text.AutoText` control requested an inline image from a
loopback HTTP server. The same crafted value under `Text.PlainText` made no
request. An independent post-patch review traced imported names, descriptions,
notes, diagnostics, logs and errors to the protected production text sinks and
found no remaining rich-text bypass or intended-markup regression.

## Release build

The release command produced a stripped, statically linked x86-64 ELF with
`CGO_ENABLED=0`; embedded build information records Go 1.25.13.

```text
CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags='-s -w' ./cmd/omastack
PASS

file bin/omastack
ELF 64-bit LSB executable, x86-64, statically linked, stripped
```

## Live Omarchy and Docker acceptance

The reviewed source was installed at
`~/.config/omarchy/plugins/david.omastack/`, the backend was restarted and the
Omarchy shell reloaded. The plugin remained enabled, its persistent shell IPC
returned a connected status, and the existing AniMauth service returned to
`running/healthy`.

An isolated temporary Compose project used the already-cached, digest-pinned
`manimcommunity/manim` image and exercised the complete live path:

1. create a project and Compose service through OmaStack's control API;
2. start it through the dependency-aware systemd user unit;
3. confirm the container and supervisor were running;
4. stop it through OmaStack;
5. observe Docker `exited (137)` alongside systemd `Result=success`,
   `ExecMainCode=0`, `ExecMainStatus=0`;
6. confirm the reconciled OmaStack result stayed `stopped`, exit code zero,
   with no stale signal/error;
7. inspect the journal and confirm clean shutdown emitted no harmless
   `file already closed` stream errors;
8. delete the temporary OmaStack project and remove its container and network.

The final live state contains only the user's original AniMauth project and no
acceptance fixture. The Docker daemon, shell IPC and `omastackd.service` were
all healthy after cleanup.

## Scope limits

Coverage is not a proof of correctness, and daemon/proxy main-loop behavior is
still more strongly supported by live acceptance than by statement coverage.
Automatic `.test` DNS and trusted local HTTPS remain intentionally unavailable;
OmaStack makes no privileged networking or trust-store changes. The release was
verified on this Omarchy/Arch machine, not across other distributions or shell
versions.
