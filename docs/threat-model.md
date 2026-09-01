# OmaStack threat model

Date: 2026-09-02

Scope: OmaStack 0.1.0 plus Unreleased hardening; backend, CLI, QML plugin, user units,
configuration, monitoring, logs, Docker integration and loopback proxy

## Security posture and scope

OmaStack is a local development orchestrator. It intentionally executes commands
chosen by the signed-in user and gives those commands that user's normal Linux
permissions. It is not a sandbox, multi-user service, secrets vault or remote
control plane. The main security objective is narrower: untrusted field values,
service output, project files and local traffic must not accidentally gain shell
semantics, cross the local-user control boundary, escape defined resource bounds,
leak configured secrets through OmaStack-owned surfaces, or cause silent
privileged system changes.

In scope are a malicious or malformed configuration/backup, hostile process and
log output, a local process under another UID, hostile HTTP requests to the
loopback proxy, failed/racing systemd operations, and unexpectedly large local
inputs. A process already running as the OmaStack user's UID is the same security
principal: it can read that user's files, replace user-owned executables and
invoke `systemctl --user` without OmaStack. Protecting the user from a fully
compromised same-UID process is therefore out of scope, although bounds and
fail-closed checks still reduce accidental damage.

OmaStack 0.1.0 has no root component. DNS, certificate trust, local HTTPS and a
privileged helper are explicitly deferred, so this model does not claim controls
for an implementation that is not present.

## Assets and security objectives

| Asset | Objective |
|---|---|
| Project definitions and environment values | Confidentiality from other UIDs; integrity across parsing, migration, updates and backup import. |
| Lifecycle authority | Only the owning user can start, stop, kill, edit or purge; ambiguous or unverifiable state fails closed. |
| User session and resources | Bounded request, process, health, log, monitoring and proxy work; no implicit shell interpretation. |
| Journald, snapshots, notifications and diagnostics | Configured secrets are masked; unclassified subprocess output is not promoted into trusted UI surfaces. |
| Local routes | Bind only to loopback, accept only configured local hostnames, target only a running configured service and do not trust client forwarding headers. |
| Host system | No silent root execution or edits to packaged Omarchy, resolver, trust store, firewall or `/etc/hosts`. |

## Architecture and trust boundaries

OmaStack's shell-facing manifest exposes a bar widget, panel and persistent QML
service (`manifest.json:9-17`). The QML service reads a redacted runtime snapshot
and invokes the CLI with an argument array rather than a shell
(`qml/Service.qml:21-23`, `qml/Service.qml:112-123`). Its shell IPC queues only
the documented local lifecycle methods (`qml/Service.qml:204-215`).

The CLI crosses into the backend over an AF_UNIX socket. The socket is mode
`0600`, an existing non-socket/symlink path is refused, concurrent clients are
capped at 32, and every connection is checked with `SO_PEERCRED`
(`internal/daemon/daemon.go:97-111`, `internal/daemon/daemon.go:134-175`,
`internal/control/peer_linux.go:12-35`). Requests are strict, versioned JSON with
a 1 MiB envelope, bounded parameters, method syntax checks and deadlines;
responses are capped at 8 MiB (`internal/control/protocol.go:16-20`,
`internal/control/protocol.go:53-87`, `internal/control/protocol.go:100-139`).

Configuration mutations and lifecycle actions cross a second boundary into
systemd and project code. They share an interruptible operation gate, with stop,
kill and restart able to cancel a long active operation
(`internal/daemon/handlers.go:149-205`). Stable UUIDs are validated before they
become unit instances (`internal/validate/validate.go:16-24`); dependencies are
topologically ordered with explicit cycle detection and reverse shutdown
(`internal/deps/order.go:13-65`). Services execute under `systemd --user`, not as
children of the shell. The service unit applies `NoNewPrivileges=yes` and
control-group killing (`internal/install/assets/omastack-service@.service:5-12`).

The supervisor is the code-execution boundary. Normal services use an
executable plus opaque argument array; shell mode alone uses `shell -c`, and
Docker mode constructs a fixed Compose verb array
(`internal/supervise/supervise.go:154-170`). It creates a process group, forwards
only an allow-listed stop signal, enforces the graceful timeout and then kills
the group (`internal/supervise/supervise.go:129-148`,
`internal/supervise/supervise.go:184-207`). This is supervision, not confinement:
managed development commands retain the user's filesystem and network access.
Only a baseline allowlist of execution/session variables is inherited from the
systemd user manager; service-specific values must be configured explicitly or
provided by a private environment file (`internal/supervise/env.go`).

Configuration, imports and environment files cross from user-selected paths
into trusted parsing. Sensitive files are opened with `O_NOFOLLOW`, checked
after open for regular-file type, private permissions and maximum size
(`internal/securefile/read_linux.go:12-45`). Current v1 configuration decoding
rejects unknown fields and validates the complete document before an atomic
mode-`0600` replacement (`internal/store/config.go:30-46`, `internal/store/config.go:79-124`,
`internal/store/atomic.go:9-45`). Configurations are capped at 64 projects, 128
services and 512 KiB after encoding (`internal/validate/validate.go:45-97`).
Setup verifies that the binary and user-unit directories are owned by the
current UID and are not writable by group or other before atomically installing
artifacts (`internal/install/install.go:41-72`,
`internal/install/install.go:150-166`).

Project output crosses into journald and back into the panel. Every non-empty
environment-file value and marked inline secret is collected in the same read
that builds the process environment (`internal/supervise/env.go:23-63`), then
exact matches are masked before stdout/stderr reaches journald
(`internal/supervise/supervise.go:124-127`,
`internal/supervise/supervise.go:172-181`). Journal requests have line, byte,
message and time bounds (`internal/logs/journal.go:18-71`), and the redacted
snapshot is atomically written mode `0600` (`internal/daemon/daemon.go:350-359`).
Every repo-owned QML `Text` sink explicitly uses `Text.PlainText`, including
imported labels, previews, diagnostics and operational errors, so display data
cannot be reinterpreted as rich text or load inline network resources.

Health checks and Docker are local subprocess/network boundaries. Health work
has a maximum concurrency of 32, a per-check deadline and a 1 MiB response-body
read limit; TLS verification cannot be disabled
(`internal/health/health.go:46-80`, `internal/health/health.go:84-124`,
`internal/validate/validate.go:264-288`). Command-check output is deliberately
discarded from diagnostics (`internal/health/health.go:138-147`). Compose
discovery reads a normalized regular non-symlink file once, feeds those verified
bytes to `docker compose config` over standard input with interpolation disabled,
and imposes size/time/output bounds without starting containers
(`internal/docker/docker.go:74-134`). Docker action verbs are allow-listed and
deadline-bound, while raw Docker error streams are withheld because Compose may
interpolate secrets (`internal/docker/docker.go:253-292`).

The optional HTTP proxy is a separate loopback listener. Configuration permits
only `127.0.0.1` or `::1` (`internal/validate/validate.go:39-43`), routes require
unique `.localhost`/`.test` hostnames (`internal/proxy/proxy.go:43-58`), and the
server caps concurrency, headers, bodies and connection durations
(`internal/proxy/proxy.go:79-107`). Requests are routed only to a currently
running service on `127.0.0.1`; untrusted forwarding headers are removed and
recreated, including extension `X-Forwarded-*` and `X-Original-*` fields
(`internal/proxy/proxy.go:108-158`,
`internal/daemon/daemon.go:500-512`). HTTPS routes are rejected
(`internal/validate/validate.go:219-228`).

## Threats, controls and residual risk

| Threat or abuse path | Main controls | Residual risk | Severity |
|---|---|---|---|
| Another local UID invokes lifecycle/config API | Runtime directory `0700`, socket `0600`, `SO_PEERCRED`, strict protocol | Root and the owning UID remain authoritative by design. | Low |
| Malformed or oversized request/config exhausts the daemon | Request/response/config limits, 32 workers, strict JSON, operation deadlines | The owning UID can still intentionally queue work or repeatedly restart units. | Low |
| Argument field becomes shell injection | Direct executable/argument execution; explicit, visibly warned shell mode; validated lengths | Shell mode and health commands are intentionally executable code. A trusted backup can define them. | High if an untrusted definition is approved; expected by product design |
| Import swaps or corrupts definitions while services run | Private `O_NOFOLLOW` read, complete validation, explicit replace flag, serialized mutation, every current unit must be proven stopped | Import itself does not start code, but imported `autostart` executes on a later daemon start. Backups must be treated as executable policy. | Medium |
| Symlink/path attack reads or overwrites an unintended file | Private XDG directories, final-component `O_NOFOLLOW`, regular-file checks, normalized absolute imports, atomic replacement, install-directory ownership/write checks, purge basename/symlink checks | A compromised same-UID process can alter user-owned parent directories and is outside the isolation claim. | Low within scope |
| Service or tool output leaks secrets | Ambient environment allowlist; exact configured/env-file values masked before journal and snapshots; Docker/health diagnostic output suppressed; bounded messages | Derived, encoded, split, command-argument, URL, note, application-generated or previously journaled secrets cannot be identified reliably. Journald remains a sensitive user-owned data source. | Medium |
| Imported label is interpreted as QML rich text | Every repo-owned production `Text` node is pinned to `Text.PlainText`; the policy is regression-tested | External UI components remain an Omarchy compatibility dependency; currently used controls also render button text as plain text. | Low |
| Lifecycle requests race deletion/import/uninstall | Interruptible operation gate; fail-closed systemd state check; uninstall stops daemon before reverse-order services (`internal/daemon/handlers.go:658-715`, `internal/install/install.go:84-128`) | External manual `systemctl --user` actions can still race OmaStack; reconciliation reports authoritative systemd state afterward. | Low |
| Host-header/forwarding spoof reaches the wrong upstream | Local hostname validation, exact configured route lookup, forwarding-header replacement, active-service port resolution | Any local user able to reach the loopback port can send requests; the proxy provides routing, not application authentication. Services must enforce their own auth where needed. | Medium on a multi-user host; Low on the target single-user desktop |
| Health checks provide SSRF or command execution | Only owning user can configure; URLs/ports/commands validated and deadline-bound; output discarded | A trusted definition can deliberately access resources reachable by the user. Do not import untrusted backups. | Medium if trust guidance is ignored |
| Docker integration increases privilege | Docker optional, explicit service/action, fixed argument arrays and bounded calls | Membership in a Docker-equivalent control group is commonly root-equivalent. OmaStack cannot reduce Docker daemon authority and must not be used to grant Docker access. | High environmental risk |
| Purge removes unintended data | Interactive `PURGE`, explicit `--yes` for automation, exact `omastack` basename and symlink refusal (`cmd/omastack/main.go:316-355`, `internal/install/install.go:131-147`) | Confirmed purge is intentionally destructive and cannot restore data without an external backup. | Low |

## Security guidance for users

- Treat project definitions, Compose files and OmaStack backups like scripts.
  Review them before import; do not enable shell mode merely to avoid forming an
  argument array.
- Keep environment files mode `0600`. Avoid printing secrets at all: redaction
  is a last-line exact-match control, not a general data-loss-prevention system.
- Define application credentials and tool-specific variables explicitly. OmaStack
  deliberately does not copy the complete user-manager environment into every
  service.
- Managed commands are supervised but not filesystem/network sandboxed. Run only
  development code you would otherwise trust with your user account.
- On a multi-user machine, assume other local users can reach the loopback HTTP
  proxy. Put authentication in sensitive development services or leave the
  proxy disabled.
- Docker access is optional and should be granted independently of OmaStack.
  Docker daemon policy determines its effective privilege.
- Use non-purge uninstall to preserve definitions. Back up configuration before
  confirmed purge.

## Deferred privileged design

No code in this release invokes `sudo`, `pkexec` or Polkit, and no code edits
DNS, host, trust-store, firewall or packaged Omarchy files. A future privileged
helper changes the trust model materially and requires a new review covering a
root-owned narrow protocol, authenticated requests, exact change previews,
transactional binding of the reviewed preview to one apply request, certificate
private-key handling, path allow-lists, receipts, rollback,
idempotent repair and distribution-specific resolver/trust-store behaviour.
Those requirements are tracked in `docs/privileged-operations.md`.

## Independent review and maintenance

An independent architecture/threat review was performed against the complete
source tree. It identified fail-open systemd checks, symlink/private-file gaps,
operation races, unbounded outputs, unsafe TLS opt-out, proxy forwarding-header
trust, install-directory permissions, Compose interpolation/file races and
uninstall ordering. Those findings were corrected before this model was
finalized and covered by regression tests. Remaining same-UID, trusted
backup, Docker-authority, loopback multi-user and best-effort-redaction risks are
documented above rather than hidden behind a broad “local only” assumption.

Revisit this model whenever the protocol is exposed beyond the Unix socket,
service sandboxing changes, remote Docker is supported, credential storage is
added, HTTPS/DNS onboarding ships, a privileged helper appears, or a new import
format can execute or resolve external content.

Repository version: OmaStack 0.1.1, reviewed 2026-09-02. CMDHub was research
input only; its separately recorded reference commit is
`8e8615e1d95742ad706d52d1075e03fc2d6dab2c`.
