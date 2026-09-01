# OmaStack architecture

## Scope and decisions

OmaStack is a native Omarchy Quattro shell plugin. QML renders the bar widget,
panel and persistent shell bridge; it does not supervise project processes or
parse untrusted command strings. A single Go binary supplies the CLI, local
daemon and per-service supervisor mode. Long-running project commands are
children of stable `systemd --user` template units, never children of
`omarchy-shell`.

Go was selected after inspecting this machine. Neither Go nor Rust was
installed initially. Go's standard library covers Unix sockets, HTTP proxying,
TLS clients, bounded JSON, process groups and testing without adding a runtime
dependency; the result is one small native binary. Rust would provide a finer
type system but would require a larger build bootstrap and more third-party
crates for this particular plugin. The build requires Go 1.25.13 or newer.

## Components

| Component | Responsibility | Lifetime |
|---|---|---|
| `qml/BarWidget.qml` | Compact global counts and panel launcher | Omarchy shell |
| `qml/CompactPanel.qml` | Native single-column project/service panel, logs, routes, settings and editors | On demand |
| `qml/Panel.qml` | Compatibility wrapper for older local installs | On demand |
| `qml/Service.qml` | Read-only snapshot watcher, serialized CLI requests and shell IPC | Omarchy shell |
| `omastack daemon` | Config, reconciliation, monitoring, health, proxy, notifications and control API | `omastackd.service` |
| `omastack supervise <id>` | Resolve a stable service ID, start one command, forward stop signals, redact output and apply restart policy | `omastack-service@<id>.service` |
| `omastack` CLI | Scriptable lifecycle, status, logs, doctor, backup and setup | Per invocation |

The QML-to-daemon boundary is intentionally narrow. The shell reads a mode
`0600` runtime snapshot and sends validated JSON requests by invoking the CLI.
The CLI connects to a mode `0600` Unix socket; the daemon also verifies Linux
`SO_PEERCRED` and rejects peers whose UID is not the daemon's UID.

Configuration and lifecycle mutations share one interruptible operation gate.
This prevents imports, edits, deletion and lifecycle requests from racing each
other; stop, kill and restart can cancel a long dependency-health wait. The
socket server caps concurrent clients, request/response sizes and method
deadlines so a same-user client cannot grow backend memory without bound.

## Service lifecycle

Each project and service receives a UUIDv4. Unit names use only service IDs,
so renaming or reordering a project cannot orphan a unit. Dependency startup
uses a topological ordering and supports `started` and `healthy` conditions;
shutdown reverses the dependency order. Systemd prevents duplicate template
unit activation. The supervisor places the command in a new process group,
forwards the configured stop signal, waits for the grace period and escalates
to `SIGKILL`.

The backend reconstructs state from systemd and durable supervisor records on
every start. Shell reloads have no effect on service units. The panel tells the
daemon when it is visible: CPU/memory history remains bounded, but port and
Docker polling slow down while the panel is closed.

## Commands and environment

Ordinary commands are stored as an executable plus an argument array and are
passed directly to `execve` semantics. Shell mode is explicit, visually
labelled as less safe, and uses the configured shell with `-c`. Environment
files are parsed as simple assignments with no interpolation or command
substitution. Configured secret values and every value read from an environment
file are redacted before output is written to journald. Environment files are
opened once with `O_NOFOLLOW`, must be private regular files, and are never
interpreted as shell syntax.

The supervisor does not forward the complete systemd user-manager environment.
It inherits only baseline execution/session variables (`PATH`, locale, XDG,
display, D-Bus and terminal values); explicit service variables and private
environment-file values are layered afterward. This prevents unrelated ambient
credentials and loader/injection variables from reaching every managed command.

## Logs and monitoring

The supervisor prefixes output with `stdout` or `stderr`; journald supplies the
bounded persistent store. The daemon reads a maximum of 50,000 entries per
request, strips the stream prefix, redacts again, and merges project logs by
timestamp. The QML view has a bounded query, pause/resume polling, selectable
text and a clear-visible-buffer action that never deletes journal data.

Host metrics are aggregated across the service process tree from `/proc`.
Listening TCP sockets are matched by inode only every few seconds. Docker
Compose services use `compose ps` and `docker stats` on a slower bounded poll
for state, health, CPU, memory and published ports.

## Health and routing

HTTP(S), TCP and executable health checks have hard timeouts and a global
concurrency cap. State changes pass through retry thresholds and start grace
periods before notifications fire.

The first production milestone includes an HTTP reverse proxy bound only to
`127.0.0.1` or `::1`, strict `.localhost`/`.test` hostname validation,
duplicate-route detection and dynamic configured/detected ports. The proxy
removes `Forwarded`, every `X-Forwarded-*`, and every `X-Original-*` identity
header before creating canonical loopback forwarding metadata. It creates its
own loopback target and caps header/body sizes, concurrent requests and
read/write/idle durations.

Compose discovery opens the selected file once with `O_NOFOLLOW`, supplies the
verified bytes to `docker compose config` over standard input and disables
environment interpolation. Discovery cannot expose interpolated `${SECRET}`
values through its command preview and never starts a container.

Configuration backups are private, validated data files, but remain executable
policy: they may define host, shell, health-check and Docker commands. Import is
therefore explicit, replacement-only, symlink-safe and refused while any
currently managed service cannot be proven stopped. A successful import never
starts services by itself; later autostart still follows the imported policy.

Trusted local HTTPS and automatic `.test` DNS are not represented as complete.
They require privileged, distribution-specific trust and resolver changes;
the UI keeps those controls disabled and the validator rejects HTTPS routes.
See [privileged-operations.md](privileged-operations.md).

## Native Omarchy integration

The manifest declares `bar-widget` and `service` entry points. The bar widget
owns an Omarchy `KeyboardPanel`, matching the first-party layer-shell panel
lifecycle: it anchors to the bar, dismisses on outside click, participates in
panel switching and scales to the available output. All UI surfaces import
`qs.Commons` and `qs.Ui`, use `Style` spacing/type/radius roles, and derive
colours from `Color`. There is no browser, Electron process or standalone GTK
main interface.
