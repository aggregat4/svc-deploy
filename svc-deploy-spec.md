# svc-deploy Specification

## Summary

Build a host-local deployment manager (written in Go) for Go + SQLite services on a single VPS with:

- immutable release directories per service
- per-release SQLite copy
- server-local git repo for non-secret config
- root-owned `EnvironmentFile` secrets injected by systemd
- atomic symlink switching + health checks + automatic rollback
- retention policy of 5 releases

This provides repeatable, versioned deploys and rollback without Docker/Kubernetes.

## Target Host Layout

Per service (`svc-a`, `svc-b`, `svc-c`, `oidc-idp`) under `/opt/a4-services/<service>`:

- `releases/<version>/`
- `current -> releases/<version>`
- `previous -> releases/<version>`
- `shared/`
- `shared/config/`
- `shared/data/`
- `shared/backups/`
- `shared/run/`
- `logs/`

Release contents:

- `bin/<service>`
- `metadata/release.json` (version, sha256, deployed_at, source URL)
- `migrations/` (if applicable)

SQLite strategy (per-release copy):

- active DB at `releases/<version>/data/<service>.db`
- on deploy, copy DB from current release to new release before startup
- keep compressed pre-deploy DB backup in `shared/backups/`
- rollback can restore code + DB pair by switching symlink

## Migration + Rollback Policy

- Schema down-migrations are not required and are out of scope for services.
- Forward-only schema changes are allowed.
- Normal rollback is release-based:
  - switch `current` back to the target prior release
  - restart service against that release's own DB copy
- Normal rollback does not attempt DB down-merge or in-place schema rewrite.
- Backups in `shared/backups/` are for emergency restore/disaster recovery, not the default rollback path.



## Systemd Contract

Each service unit:

- `WorkingDirectory=/opt/a4-services/<service>/current`
- `ExecStart=/opt/a4-services/<service>/current/bin/<service>`
- `EnvironmentFile=/etc/a4-services/<service>.env` (secrets only)
- optional runtime config file:
  - `EnvironmentFile=/opt/a4-services/<service>/shared/config/runtime.env`
- `Restart=on-failure`
- `TimeoutStartSec=30`

Each service defines a health URL (for example `http://127.0.0.1:<port>/healthz`).

## Config + Secrets Model

Non-secret config:

- local git repo on server: `/opt/config-repo`
- canonical deploy map path: `/opt/config-repo/a4-services/deploy-map.toml`
- optional compatibility symlink/path: `/etc/a4-services/deploy-map.toml`
- per-service runtime config, for example `/opt/config-repo/a4-services/services/<service>/runtime.env`
- deploy places/symlinks runtime config into `/opt/a4-services/<service>/shared/config/runtime.env`
- deploy stores config commit hash in release metadata

Secrets:

- `/etc/a4-services/<service>.env`
- owner `root:root`, mode `0600`
- never tracked in config git repo
- rotate by replacing file + restarting service

## Artifact + Versioning Rules

Release source:

- GitHub Releases tarball fetched by version
- checksum file fetched alongside artifact
- mandatory SHA256 verification before unpack

Version identity:

- semantic version (for example `v1.8.3`)
- deploy ID `<service>:<version>:<timestamp>`
- append-only history log at `/opt/a4-services/<service>/shared/deploy-history.log`

## CLI Interface

Binary: `/usr/local/bin/svc-deploy`

Commands:

1. `svc-deploy deploy <service> <version>`
2. `svc-deploy rollback <service> [target-version]`
3. `svc-deploy status <service>`
4. `svc-deploy prune <service> --keep 5`

Configuration map (canonical): `/opt/config-repo/a4-services/deploy-map.toml`

Optional compatibility symlink/path: `/etc/a4-services/deploy-map.toml`

Per-service fields:

- release URL template
- artifact filename template
- binary path inside artifact
- healthcheck URL
- systemd unit name
- db filename / relative path
- startup timeout
- rollback timeout

## Deploy Flow

`svc-deploy deploy <service> <version>`:

1. Acquire lockfile (`/var/lock/svc-deploy-<service>.lock`).
2. Validate service exists in deploy map.
3. Fetch tarball + checksum from GitHub Releases.
4. Verify checksum.
5. Create `releases/<version>`.
6. Extract artifact.
7. Copy DB from current release into new release DB path (including WAL/SHM handling).
8. Create compressed pre-deploy backup in `shared/backups/`.
9. Place/symlink runtime config.
10. Update `previous` symlink to old current.
11. Atomically switch `current` symlink to new release.
12. `systemctl restart <unit>`.
13. Poll health endpoint until timeout.
14. If health fails, switch `current` back to `previous`, restart, and mark failed deploy.
15. On success, write metadata + deploy history and prune old releases.

## Rollback Flow

`svc-deploy rollback <service> [target-version]`:

1. Acquire lock.
2. Resolve target release (`previous` or explicit version).
3. Validate release exists and contains expected binary + DB.
4. Switch `current` to target.
5. Restart service.
6. Health-check until timeout.
7. On failure, restore prior `current` and restart.
8. Record rollback result in history.

## Failure Handling

- artifact fetch/checksum failure: abort before symlink switch
- DB copy failure: abort before symlink switch
- health check failure: automatic rollback
- rollback target unhealthy: revert to prior current
- concurrent deploys: lockfile prevents overlap
- low disk: preflight free-space check fails deploy

## Public Artifacts Added

- CLI: `svc-deploy`
- config (canonical): `/opt/config-repo/a4-services/deploy-map.toml`
- config (optional compatibility path): `/etc/a4-services/deploy-map.toml`
- secrets: `/etc/a4-services/<service>.env`
- metadata: `/opt/a4-services/<service>/releases/<version>/metadata/release.json`
- history: `/opt/a4-services/<service>/shared/deploy-history.log`

## Testing Strategy

All tests must be runnable inside this repository with no dependency on external services.

Test command baseline:

- `go test ./...` runs unit tests and local integration tests
- optional heavier suites may use a build tag (for example `-tags=integration`) but still run fully local

Architecture for testability:

- the deploy engine is built around interfaces for all side effects (`ArtifactFetcher`, `FS`, `SymlinkManager`, `ServiceManager`, `HealthChecker`, `Locker`, `Clock`)
- tests replace real implementations with in-process fakes or local mock servers
- tests run in temp directories and never require real `/opt/a4-services`, `/opt/config-repo`, `/etc`, or systemd

Local-only test fixtures:

- mock artifact HTTP server started in tests via `httptest.Server`
- generated tarballs + checksum files under `testdata/`
- fake service manager (no real `systemctl`) with scripted success/failure outcomes
- local health mock server with controllable behavior (healthy, unhealthy, timeout, delayed healthy)

Required test categories:

1. Config parsing and validation for `deploy-map.toml`.
2. Deploy state-machine/unit tests with fake dependencies.
3. Rollback state-machine/unit tests with fake dependencies.
4. Prune/retention logic tests.
5. Locking and concurrent command behavior tests.
6. Integration-style workflow tests over temp filesystem + local mock servers.
7. CLI contract tests (exit codes, stderr/stdout, optional `--json` output).

## Test Scenarios

1. Fresh deploy on empty temp host layout creates expected release directories and symlinks.
2. Normal upgrade switches `current`, updates `previous`, and records metadata/history.
3. New version fails to start (service manager restart error) and triggers automatic rollback.
4. New version starts but health endpoint remains unhealthy/timeout and triggers automatic rollback.
5. Explicit rollback to known older version succeeds and records rollback event.
6. Explicit rollback to missing/invalid version fails without mutating active release.
7. Concurrent deploy attempts for the same service are rejected by locking behavior.
8. Prune keeps exactly configured N releases and never removes `current` or `previous`.
9. Artifact download/checksum mismatch aborts before any symlink switch.
10. Secret-file preflight failures abort deploy before cutover (without validating secret contents).
11. Restart/recovery simulation: status resolves active release from existing symlinks + metadata.

Out of scope for this tool's tests:

- application-specific SQLite schema/content correctness
- service business-logic validation beyond process start + health endpoint behavior

## Defaults Chosen

- keep 5 releases per service
- per-release SQLite copy
- config versioned in local server git repo
- secrets via root-owned systemd `EnvironmentFile`
- health-gated cutover with automatic rollback
- no GitHub-triggered deploy; VPS manually pulls release artifacts

