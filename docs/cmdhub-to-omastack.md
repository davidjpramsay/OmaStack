# CMDHub-to-OmaStack feature mapping

The reference version is CMDHub `v1.0.0`, commit
`8e8615e1d95742ad706d52d1075e03fc2d6dab2c`. Research details and the complete
screen/component inventory are in [cmdhub-reference.md](cmdhub-reference.md).

| CMDHub reference concept | OmaStack translation | Linux/Omarchy difference |
|---|---|---|
| macOS menu-bar popover | Quattro bar widget plus anchored Omarchy `KeyboardPanel` | Compact layer-shell card follows Omarchy panel placement, focus and dismissal conventions |
| Global service summary | Theme-derived count pills and bar counts | Omarchy `accent`, `muted`, `urgent` roles replace fixed traffic-light colours |
| Grouped projects and dense service rows | Single-column disclosure groups, 38-unit service rows and expandable inline metrics | Original Omarchy controls and geometry rather than CMDHub trade dress |
| Start/stop/restart controls | Per-row, per-project and global lifecycle actions | Stable `systemd --user` units replace macOS process ownership/helper paths |
| CPU/memory sparklines | Two bounded 60-sample native QML Canvas series | Updates slow while the panel is closed |
| Detected ports and URL opening | `/proc` socket-inode matching or Docker published ports | Explicit loopback URLs and `xdg-open` |
| Health badges/checks | HTTP(S), TCP and direct command checks | Concurrency/timeouts are enforced by the Go daemon |
| Service and combined logs | Journald-backed service/project viewer | Journald remains authoritative; clear affects only the visible QML buffer |
| Project/service wizard | Native multi-step project wizard and six-section service editor | Executable and arguments are separate; shell mode is explicit and warned |
| Environment editor | Dynamic key/value rows with per-value secret masking | Files are mode `0600`; values are redacted before journald |
| Dependency editor | Visual picker with started/healthy conditions | DAG is validated before lifecycle actions |
| Reverse proxy/local domains | Loopback HTTP proxy and active route view | `.localhost` is the zero-config recommendation; automatic `.test` DNS is deferred |
| Local HTTPS | Disabled and rejected in the current release | Trust-store changes need a future narrow Polkit helper and reversible receipts |
| Docker Compose | Import parser, lifecycle, state/health/ports/stats, rebuild/recreate/terminal | Docker is fully optional for host projects |
| Privileged helper architecture | No privileged helper shipped yet | OmaStack does not elevate its daemon or QML; safety requirements are documented before implementation |

## Preserved workflow principles

OmaStack keeps compact grouping, state-first rows, adjacent lifecycle actions,
metrics close to the service they describe, a bounded log drill-down and a
wizard-first setup path. It intentionally changes the visual composition,
copy, icons, palette, typography and component shapes to follow Omarchy rather
than reproducing a macOS popover or CMDHub trade dress.
