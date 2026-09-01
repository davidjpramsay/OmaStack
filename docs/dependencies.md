# Dependencies

## Build

- Go 1.25.13 or newer. Earlier Go 1.25 patch releases contain standard-library
  vulnerabilities that are reachable from OmaStack's HTTP and certificate
  handling paths.
- `make` is optional; the documented build uses `go build` directly.

The backend uses only the Go standard library. There are no Go module downloads
and no vendored third-party runtime libraries.

Release verification additionally uses ShellCheck, Staticcheck and govulncheck;
they are development tools and are not runtime dependencies.

## Required runtime

- Omarchy Quattro shell with manifest schema 1, `qs.Commons` and `qs.Ui`.
- Quickshell/Qt 6 QML modules supplied by Omarchy.
- systemd user manager (`systemctl --user`, `journalctl --user-unit`).
- Linux `/proc` and Unix sockets with `SO_PEERCRED`.
- `notify-send` for desktop notifications.
- `xdg-open` for service URLs.

## Optional runtime

- Docker CLI and Docker Compose v2 for Compose services.
- `xdg-terminal-exec` and, when available, `uwsm-app` for interactive
  container terminals.

Docker is never required for host-process projects. OpenSSL is not used by the
current release because trusted local HTTPS is deferred.
