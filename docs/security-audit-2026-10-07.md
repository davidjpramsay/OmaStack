# OmaStack security and bug audit

Date: 2026-10-07, Australia/Perth

Version: 0.1.1

Source commit: `91ce9fd49c922877f4f43647dac7a43fc0cc9bb0`

## Release decision

Hold the plugin-library release. Seven defects were confirmed: three high-priority lifecycle or data-integrity bugs and four medium-priority environment, secret-editing, proxy-shutdown and executable-resolution bugs. The normal test and security-tool gates pass, but they do not cover these scenarios. Fix the findings and add regression tests before treating this commit as release-ready.

The repository is currently private. GitHub's public API returns 404, and authenticated repository metadata confirms `isPrivate: true`. Marketplace validation cannot inspect it in that state. Its visibility was not changed during this audit.

This is a source and local integration audit, not a certification that no vulnerabilities exist. No cross-user control bypass, unintended shell injection or exploitable remote code execution was confirmed. OmaStack intentionally executes trusted user-defined commands and Compose files; it is not a sandbox. Ambient credential inheritance and incomplete connection revocation remain security-hardening defects within that model.

## High priority findings

### P1 Project metadata edits can discard active services

Locations: [handlers.go](/home/david/Projects/OmaStack/internal/daemon/handlers.go:428), [ProjectEditor.qml](/home/david/Projects/OmaStack/qml/components/ProjectEditor.qml:16).

The project editor submits the full captured project, including its service array. `updateProject` preserves the current services only when the submitted array is empty; otherwise it replaces the entire project. A project-name edit based on an older snapshot can therefore erase a newly added service or revert a service edited elsewhere. It bypasses the stopped-service checks used by normal deletion.

Reproduction: capture a project containing one service, create another service, start its mocked systemd unit, and submit the old project with a changed name. The request succeeds, the new service disappears from configuration, and the mocked unit remains active. Normal Stop All and uninstall enumerate saved definitions, so the forgotten unit is no longer controlled through those paths.

Fix: make project updates metadata-only and retain the authoritative current service array. Keep service mutations behind the dedicated service APIs. If full-project replacement is required, use revision checks and explicit lifecycle guards rather than accepting stale state.

Regression requirements: stale project rename preserves newly created services, concurrent service edits and secrets; project updates cannot delete a running service or bypass dependency checks.

### P1 Docker Recreate can launch a container outside supervision

Locations: [handlers.go](/home/david/Projects/OmaStack/internal/daemon/handlers.go:775), [docker.go](/home/david/Projects/OmaStack/internal/docker/docker.go:301), [handlers.go stopped check](/home/david/Projects/OmaStack/internal/daemon/handlers.go:664).

Recreate directly runs `docker compose up --detach --force-recreate --no-deps`. It can start a previously stopped container without starting its OmaStack supervisor or passing managed dependency/readiness checks. Stop only stops the systemd unit, and deletion checks only systemd state.

Reproduction used a real, isolated Alpine container with an inactive fake systemd boundary: Recreate starts the container; Stop returns successfully while the container stays running; Delete succeeds and removes its definition. No installed user service was stopped or modified.

Fix: route operations that change container lifecycle through managed stop/start orchestration. Define whether Recreate should preserve the previous stopped state. Check Docker state before deleting, importing over or uninstalling Docker definitions, and refuse ambiguous ownership or inspection failures.

Regression requirements: stopped and running Recreate cases; dependency/readiness handling; cancellation; Stop All; deletion/import refusal while an unmanaged matching container is running; confirmation that no matching container remains after stop/uninstall.

### P1 Docker Force kill does not kill the container

Locations: [handlers.go](/home/david/Projects/OmaStack/internal/daemon/handlers.go:260), [manager.go](/home/david/Projects/OmaStack/internal/systemd/manager.go:46).

Force kill sends SIGKILL to the systemd unit's processes. The supervisor and attached Compose client are in that unit, but the Docker daemon owns the container. Killing those client processes does not reliably terminate the container. Subsequent Docker reconciliation can report it as running even though managed supervision has ended.

Reproduction used the real supervisor `runOnce` path and attached Compose client with a real isolated Alpine container. Killing that exact test client's process group returned from supervision while Docker still reported the container running. This reproduces the process-ownership failure; the installed Force kill action was not exercised against a real user service.

Fix: explicitly kill the validated Compose service/container as well as terminating supervision, and verify the resulting container state. Preserve intentional lifecycle outcomes instead of allowing Docker reconciliation to obscure failed control.

Regression requirements: Force kill of host and Docker services, Docker kill failures/timeouts, stopped status after reconciliation, and absence of a surviving container.

## Medium priority findings

### P2 Docker actions and command probes bypass the environment policy

Locations: [docker.go](/home/david/Projects/OmaStack/internal/docker/docker.go:343), [Docker terminal](/home/david/Projects/OmaStack/internal/docker/docker.go:283), [health.go](/home/david/Projects/OmaStack/internal/health/health.go:139), [env.go](/home/david/Projects/OmaStack/internal/supervise/env.go:35).

Supervised launches filter ambient environment variables and merge explicitly configured service values. Docker administrative subprocesses and command health probes leave `exec.Cmd.Env` unset, inheriting the daemon's complete environment instead. Service-specific environment values are not supplied to these paths. Command probes also do not use the service's working directory.

Reproduction: a fake Docker executable invoked by Recreate receives a fake ambient `AWS_SECRET_ACCESS_KEY` that the supervisor allowlist would exclude. No real credential was displayed, sent or used. The health and terminal paths have the same unset-environment behavior in source.

This is an accidental credential-exposure risk and a functional inconsistency, not a new privilege boundary or proof of credential theft. Configured Compose interpolation and command probes can also behave differently from the service they are supposed to manage.

Fix: use a shared, explicit environment policy for service-scoped subprocesses, including action, inspection/readiness and probe paths. Merge the service's environment file and inline values where appropriate; apply the service working directory to command probes. Keep interactive session requirements explicit rather than restoring blanket inheritance.

Regression requirements: excluded fake ambient credentials stay excluded in every subprocess path; configured variables reach all relevant Compose/probe operations; probes resolve relative files in the service directory; errors remain redacted.

### P2 Changing an existing secret to plain text corrupts its value

Locations: [ServiceEditor.qml](/home/david/Projects/OmaStack/qml/components/ServiceEditor.qml:468), [handlers.go](/home/david/Projects/OmaStack/internal/daemon/handlers.go:757).

Snapshots replace stored secrets with `••••••••`. The editor's secret toggle changes the boolean but retains that placeholder. `mergeMaskedSecrets` preserves the old value only if the submitted field is still marked secret. Toggle an existing stored secret to plain and save, and the literal mask replaces the original credential.

Reproduction submitted the redacted editor state with `Secret=false`; the stored value became the mask. The original value was a synthetic test credential.

Fix: separate unchanged-value intent from the displayed mask, preferably with an explicit keep/replace operation. Preserve the existing value when only classification changes, or require a replacement before accepting the change. Treat renames and a literal mask value unambiguously.

Regression requirements: unchanged secret save, secret-to-plain transition, plain-to-secret transition, secret rename, explicit replacement/removal, and a legitimate value equal to the mask.

### P2 Disabling the proxy leaves an established upgrade tunnel open

Location: [proxy.go](/home/david/Projects/OmaStack/internal/proxy/proxy.go:252).

Proxy close stops the listener with `http.Server.Shutdown`, but does not track and close hijacked connections. A valid upgraded connection can continue forwarding after the setting is disabled and routes are advertised as inactive. Continued traffic can keep it alive despite the existing inactivity deadline.

Reproduction opened a loopback HTTP 101 upgrade, disabled the proxy, and successfully exchanged additional data with the upstream through the existing tunnel. No external network listener was used. This behavior follows the documented [Go shutdown semantics](https://pkg.go.dev/net/http#Server.Shutdown), which exclude hijacked connections.

Fix: track and close upgraded upstream/downstream connections on disable and listener reconfiguration. Cancel active proxy work and hard-close ordinary connections when the graceful timeout expires. Calling `Server.Close` alone also does not close hijacked connections.

Regression requirements: upgraded tunnel closes on disable, address/port change and daemon shutdown; connection tracking and concurrency permits are released; normal shutdown and timeout paths do not leak active streams.

### P2 A configured PATH does not resolve the service executable

Location: [supervise.go](/home/david/Projects/OmaStack/internal/supervise/supervise.go:100).

The supervisor constructs the service environment, but calls `exec.Command` before assigning it to `cmd.Env`. Go resolves a bare executable name using the supervisor's ambient PATH, not the explicitly configured service PATH. A command available only on the configured PATH fails to launch.

Reproduction created a harmless executable in a temporary directory, added that directory only to the service's PATH, and launched it by basename. Launch failed with `executable file not found in $PATH`.

Fix: resolve executable basenames using the constructed environment, retaining protection against implicit current-directory lookup, or clearly require and validate absolute executable paths.

Regression requirements: service-specific PATH, environment-file PATH, absolute executable paths, missing executables and unsafe current-directory lookup.

## Fresh verification results

These results apply to the audited commit, not to fixes that have yet to be implemented.

| Gate | Result |
| --- | --- |
| Complete Go race suite with coverage | Pass; no reported races; 68.0% statement coverage |
| Go vet | Pass |
| Staticcheck v0.8.1 | Pass |
| govulncheck v1.8.0 source scan | No vulnerabilities found |
| govulncheck existing Go 1.25.13 binary scan | No vulnerabilities found |
| ShellCheck 0.11.0 and Bash syntax checks | Pass for all repository shell scripts |
| QML regression suite | 20 passed, 0 failed, 0 skipped |
| Omarchy plugin validation | Pass |
| Isolated stripped static build | Pass with host Go 1.27.0 |
| Gitleaks 8.30.1 history and working-tree scans | No leaks detected |
| Read-only live widget/backend check | Pass; connected widget sees the backend's projects |
| Clean-clone source packaging | Pass; root build binary, bin output, Git metadata and local .codex data excluded |
| Focused audit reproductions | Seven defects confirmed, including two real-container ownership tests |

The existing suite has no statement coverage for `updateProject`, `deleteService`, `ensureStopped` or `dockerAction`. Overall coverage is useful, but these missing lifecycle scenarios are more important than reaching an arbitrary percentage.

Diagnostic tests were added only to a temporary source copy. Some assert the observed faulty behavior so the reproduction is repeatable; their passing status is not evidence that the defects are fixed. Application source, installed binaries, user configuration and repository history were left unchanged. Temporary Docker fixtures were removed, and a final name check found no remaining `omastackaudit` containers.

## Installation and marketplace checks

### Public access and verification

The official public baseline preflight failed because this repository is private. Make it public only after fixes and a final secret/history review, or choose a supported public distribution repository. That is a publication decision requiring the owner's authorization.

A local run of the marketplace analysis rules from marketplace commit `c40a8a83aabd13790ff114d5980a5c3b6f1348fc` examined 34 selected committed text files. It returned `review-required`, no unsafe-execution findings, and package-management, service-management and installer capabilities. Package-management evidence includes the documented Go installation command; it is not evidence that OmaStack silently installs packages. This local analysis does not replace the full public snapshot/compatibility workflow and is not marketplace approval. The [official verification policy](https://github.com/omacom/omarchy-plugin-marketplace/blob/main/VERIFICATION.md) describes the exact-commit review process and its limits.

### Backend setup is manual

The installed Omarchy 4.0.4 add/update commands clone or pull and validate the QML plugin; they do not execute OmaStack's build/setup scripts. The repository ships source, not a tracked backend executable. QML expects the backend at `$HOME/.local/bin/omastack`.

The README correctly documents the additional build/setup steps for installation and updates. This is not an undisclosed code bug, but a standard plugin-library Install action alone will not produce a functioning fresh OmaStack installation. Use an explicit manual-setup listing and clear first-run instructions, or implement a supported, consent-based backend setup workflow before advertising one-click installation. Test the chosen workflow on a clean Omarchy account, including removal and an update that changes both QML and backend code.

### Preview and maintenance

The repository has three screenshot assets but no supported root `preview` image. The marketplace's default preview discovery looks for root `preview.png`, `preview.webp`, `preview.jpg`, `preview.jpeg` or `preview.avif`, rather than this repository's custom `marketplace.json` screenshot list. Add a crisp root preview or arrange an explicit supported preview override to avoid a fallback card. The [publish guide](https://plugins.omarchy.org/publish.html) covers submission assets.

No CI workflow is tracked. Add reproducible Go/race/vet/staticcheck/govulncheck/ShellCheck gates, plus an Omarchy-compatible QML job or a clearly documented native acceptance gate. Keep historical September verification records dated; this October audit must not be presented as a successful release sign-off.

## Layout and usability

Fresh offscreen renders of the current panel were inspected in Everforest, Tokyo Night and Catppuccin Latte. CPU/memory charts fit the expanded row, logs retain a readable full-width message area, and the 400-pixel settings viewport contains its scroll region. Existing tests verify keyboard-focus visibility of service controls, browser route ports, draft preservation after failed saves and stop interruption of pending starts. The offscreen runner uses Quickshell stubs, so these renders do not prove every live compositor or input behavior.

The most valuable usability improvements before release are clearer missing-backend onboarding and safe, predictable Docker actions. The existing plain/secret eye toggle also needs wording and behavior that distinguish changing secret classification from revealing a value; the actual stored value is not present in the redacted snapshot.

## Required sign off checklist

- [ ] Fix the seven confirmed defects and add permanent tests for the cases above.
- [ ] Run the full race/static/security/QML gates on the fixed commit and newly built release binary.
- [ ] Exercise normal stop and Force kill through real temporary systemd-managed Docker fixtures, including Recreate, Stop All, deletion and uninstall. The audit reproduced container ownership separately; this final end-to-end gate remains required.
- [ ] Verify proxy upgrade revocation and environment consistency with synthetic credentials.
- [ ] Test fresh install, update and non-purge removal on a clean Omarchy account using the documented marketplace installation mode.
- [ ] Finalize a supported preview and make public source available with explicit owner approval.
- [ ] Run public marketplace validation/baseline against the exact release SHA and obtain required maintainer review.
- [ ] Package only committed files, record the SHA/checksum, and update the verification record with actual post-fix results.

Local audit fixtures, coverage data, screenshots and scanner reports are retained under `/tmp/omastack-audit.edCtSD`. That directory is temporary evidence, not a release artifact or permanent regression suite.
