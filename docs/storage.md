# Files and directories

OmaStack follows XDG locations and never stores user projects inside the
plugin checkout.

| Path | Mode | Purpose | Removal behaviour |
|---|---:|---|---|
| `$XDG_CONFIG_HOME/omastack/config.json` | `0600` | Projects, services, secrets and settings | Preserved by disable/uninstall; removed only by confirmed purge |
| `$XDG_CONFIG_HOME/omastack/exports/*.json` | `0600` | User-requested configuration backups | Preserved unless purged |
| `$XDG_RUNTIME_DIR/omastack/control.sock` | `0600` | Same-user local control API | Removed when the daemon stops |
| `$XDG_RUNTIME_DIR/omastack/state.json` | `0600` | Redacted shell snapshot | Removed when the daemon stops |
| `$XDG_STATE_HOME/omastack/services/*.json` | `0600` | Durable supervisor PID/exit/restart records | Cleaned explicitly or purged |
| `$XDG_CACHE_HOME/omastack/` | `0700` directory | Reserved bounded cache | Purged on request |
| `~/.local/bin/omastack` | `0755` | Native CLI/backend binary | Removed by `omastack uninstall` |
| `~/.config/systemd/user/omastackd.service` | `0644` | Backend user service | Removed by `omastack uninstall` |
| `~/.config/systemd/user/omastack-service@.service` | `0644` | Stable per-service template | Removed by `omastack uninstall` |
| `~/.config/omarchy/plugins/david.omastack/` | user-owned | QML and manifest | Removed separately by `omarchy plugin remove` |

Defaults expand to `~/.config`, `~/.local/state`, `~/.cache` and
`/run/user/$UID`; absolute XDG overrides are honoured. Directories are mode
`0700`. OmaStack refuses symlink directories, symlink config targets,
non-normalized import paths, non-regular import files and backups readable by
group or other users.

User-selected environment files are not created or owned by OmaStack. At
service start, the final path component is opened with `O_NOFOLLOW`; it must be
a regular file no larger than 1 MiB and must not grant group/other permissions.
Its values are parsed once, passed directly as the child environment, and added
to the same run's redaction set.

Setup also verifies that existing `~/.local/bin` and
`~/.config/systemd/user` directories are owned by the current user and are not
group/world writable before installing the binary or user-unit definitions.
