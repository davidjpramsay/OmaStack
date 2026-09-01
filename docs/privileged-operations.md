# Privileged operations and local-domain roadmap

The current release performs **no privileged operation**. It never invokes
`sudo`, `pkexec` or Polkit, and never edits `/etc/hosts`, resolver settings,
certificate stores, firewall rules or packaged Omarchy files.

## What works without elevation

- HTTP reverse proxy on a configurable loopback port.
- Routes using an explicit `Host` value.
- `*.localhost` names on resolvers and browsers that implement localhost
  subdomain resolution.
- `.test` route definitions for users who already provide their own DNS.
- Conflict detection for duplicate hostnames and occupied proxy ports.

Use a URL such as `http://frontend.myapp.localhost:8088` with the default proxy
port. `.test` names are intentionally not claimed to resolve automatically.

## Deferred privileged milestone

A future helper may add automatic `.test` DNS and local HTTPS only after the
following design is implemented and reviewed:

1. The unprivileged daemon produces an exact preview containing every path,
   hostname, certificate fingerprint and command.
2. The reviewed preview is bound to the apply request with a digest and
   single-use nonce, so the helper cannot authorize one plan and execute a
   different or stale plan.
3. A separately packaged, root-owned helper accepts a tiny versioned request;
   it does not accept shell commands or arbitrary paths.
4. Polkit policy authorizes one operation at a time through `pkexec`.
5. Files live under narrowly scoped OmaStack names, use atomic replacement,
   reject symlinks and record hashes/backups before change.
6. Install, repair and uninstall are idempotent and reverse only files whose
   current hashes match an OmaStack receipt.
7. CA private keys never enter QML, notifications, diagnostics or logs.
8. Certificate renewal is bounded and requires the helper only for trust-store
   updates that genuinely need elevation.

Arch/Omarchy installations can use different resolver and trust-store stacks.
Shipping a helper before those variants and rollback paths are exercised would
violate the project's safety requirements, so the current UI disables HTTPS
onboarding and the backend rejects an HTTPS route instead of faking support.
