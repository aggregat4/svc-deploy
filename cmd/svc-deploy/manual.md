# svc-deploy Manual

`svc-deploy` is a host-local deployment manager for Go services, with optional per-release SQLite handling, intended for a single Linux VPS that uses `systemd` instead of containers or an orchestrator: it downloads versioned release tarballs, verifies SHA256 checksums, unpacks immutable release directories under `/opt/a4-services/<service>/releases/<version>`, updates `current` and `previous` symlinks, restarts the service, waits for an HTTP health check to return `2xx`, rolls back automatically if startup fails, writes release metadata and deploy history on success, and can later report status, perform explicit rollbacks, and prune old releases.

## Commands

```text
svc-deploy deploy <service> <version>
svc-deploy rollback <service> [target-version]
svc-deploy status <service>
svc-deploy prune <service> [--keep N]
svc-deploy manual
```

Global flags:

```text
-c, --config <path>   Path to deploy-map.toml
-j, --json            JSON output for automation
-v, --version         Show binary version
-h, --help            Show usage
```

## Paths

Default config lookup:

```text
/opt/config-repo/a4-services/deploy-map.toml
/etc/a4-services/deploy-map.toml
```

Required host layout:

```text
/usr/local/bin/svc-deploy
/opt/a4-services/<service>/
/opt/a4-services/<service>/releases/
/opt/a4-services/<service>/current
/opt/a4-services/<service>/previous
/opt/a4-services/<service>/shared/config/runtime.env
/opt/a4-services/<service>/shared/backups/
/opt/a4-services/<service>/shared/deploy-history.log
/etc/a4-services/<service>.env
/opt/config-repo/a4-services/deploy-map.toml
/opt/config-repo/a4-services/services/<service>/runtime.env
```

Important implementation detail: the deploy-map path is configurable with `--config`, but runtime environment files are always read from `/opt/config-repo/a4-services/services/<service>/runtime.env` in the current implementation.

## Deploy Map

Example:

```toml
[service.svc-a]
release_url_template = "https://github.com/example/svc-a/releases/download/{{.Version}}/{{.Artifact}}"
artifact_filename_template = "svc-a-{{.Version}}-linux-amd64.tar.gz"
checksum_filename_template = "svc-a-{{.Version}}-linux-amd64.tar.gz.sha256"
binary_path = "bin/svc-a"
healthcheck_url = "http://127.0.0.1:8080/healthz"
systemd_unit = "svc-a.service"
db_filename = "svc-a.db"
startup_timeout = 30
rollback_timeout = 30
keep_releases = 5
min_disk_space = 1073741824
```

Field notes:

- `release_url_template` supports `{{.Version}}`, `{{.Service}}`, and `{{.Artifact}}`.
- `artifact_filename_template` supports `{{.Version}}` and `{{.Service}}`.
- `checksum_filename_template` is optional. If omitted, the tool fetches `<artifact-url>.sha256`.
- `db_filename` is optional. If omitted, no database copy or backup happens.
- Defaults are `startup_timeout = 30`, `rollback_timeout = 30`, `keep_releases = 5`, and `min_disk_space = 1073741824`.

## Config Model

Secrets are expected in:

```text
/etc/a4-services/<service>.env
```

Non-secret runtime settings are expected in:

```text
/opt/config-repo/a4-services/services/<service>/runtime.env
```

During deploy, `svc-deploy` copies the runtime file to:

```text
/opt/a4-services/<service>/shared/config/runtime.env
```

If the source runtime file does not exist, any existing shared `runtime.env` is removed.

## Systemd Contract

The unit should run from the `current` symlink and consume both environment files:

```ini
[Unit]
Description=svc-a
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=/opt/a4-services/svc-a/current
ExecStart=/opt/a4-services/svc-a/current/bin/svc-a
EnvironmentFile=/etc/a4-services/svc-a.env
EnvironmentFile=-/opt/a4-services/svc-a/shared/config/runtime.env
Restart=on-failure
RestartSec=2
TimeoutStartSec=30

[Install]
WantedBy=multi-user.target
```

`EnvironmentFile=/path/file` means the file is required. `EnvironmentFile=-/path/file` means the file is optional.

## Release Artifact Contract

The release tarball must unpack cleanly into the release directory and contain the binary at `binary_path`. For the example above, the archive must produce:

```text
bin/svc-a
```

If `db_filename` is configured, rollback expects the target release to contain `data/<db_filename>`. On upgrades, `svc-deploy` copies the current release database, plus `-wal` and `-shm` files if present, into the new release before restart.

Important first-deploy behavior: if there is no current release yet, `svc-deploy` does not seed or create the SQLite database. Your service must be able to start with no existing database copy, or your artifact or startup path must create it.

## Host Preparation

Create the base directories once:

```bash
install -d -m 0755 /opt/a4-services
install -d -m 0755 /opt/config-repo/a4-services/services
install -d -m 0755 /etc/a4-services
```

For each service, create at least:

```bash
install -d -m 0755 /opt/a4-services/<service>/releases
install -d -m 0755 /opt/a4-services/<service>/shared/config
install -d -m 0755 /opt/a4-services/<service>/shared/backups
install -d -m 0755 /opt/a4-services/<service>/shared/data
install -d -m 0755 /opt/a4-services/<service>/shared/run
install -d -m 0755 /opt/a4-services/<service>/logs
install -m 0600 /dev/null /etc/a4-services/<service>.env
```

The deploy preflight requires `/etc/a4-services/<service>.env` to exist and be non-empty. In production, keep it `root:root` and mode `0600` even though the current code only checks for existence, readability, and non-zero size.

## First Deployment

1. Publish the service artifact and checksum for the target version.
2. Create `/etc/a4-services/<service>.env` with the required secrets.
3. Add the service entry to `/opt/config-repo/a4-services/deploy-map.toml`.
4. Optionally add `/opt/config-repo/a4-services/services/<service>/runtime.env`.
5. Install and enable the `systemd` unit.
6. Run `svc-deploy deploy <service> <version>`.

On success, `current` points to the new release, `previous` points to the prior release if one existed, the unit is restarted, and release metadata is written to `metadata/release.json`.

## Normal Deployment Workflow

For a routine upgrade:

```bash
svc-deploy deploy svc-a v1.2.3
```

What happens:

1. A lock file is taken at `/var/lock/svc-deploy-<service>.lock`.
2. The artifact and checksum are downloaded and verified.
3. `/opt/a4-services/<service>/releases/<version>` is created.
4. The tarball is extracted there.
5. If `db_filename` is set and a current release exists, the database is copied into the new release.
6. A compressed pre-deploy database backup is written under `shared/backups/` on a best-effort basis.
7. Runtime config is copied from the config repo into `shared/config/runtime.env`.
8. Free disk space and the secrets file are checked.
9. `current` is switched atomically and `previous` is updated.
10. `systemctl restart <unit>` is executed.
11. The health URL is polled every 500ms until the startup timeout expires.
12. If health never becomes good, the tool switches `current` back to `previous` and restarts again.

## Rollback

Rollback to the `previous` symlink target:

```bash
svc-deploy rollback svc-a
```

Rollback to a specific release:

```bash
svc-deploy rollback svc-a v1.2.1
```

Rollback changes the `current` symlink, restarts the unit, and waits for the health endpoint. If the rollback target fails health checks, the tool restores the release that was active before the rollback attempt.

## Status and Automation

Check a service:

```bash
svc-deploy status svc-a
svc-deploy status svc-a --json
```

Current CLI status output contains:

- Service name
- Current version
- Previous version, if known
- Whether `systemd` reports the unit as active

For automation, all commands can emit JSON with `--json`.

## Pruning

Prune manually:

```bash
svc-deploy prune svc-a --keep 5
```

The tool never deletes the releases referenced by `current` or `previous`. Because of that protection, the number of release directories left on disk can be higher than the nominal keep count.

## Operational Files

After successful use in production, expect these files and links to matter during incident response:

- `/opt/a4-services/<service>/current`
- `/opt/a4-services/<service>/previous`
- `/opt/a4-services/<service>/releases/<version>/metadata/release.json`
- `/opt/a4-services/<service>/shared/config/runtime.env`
- `/opt/a4-services/<service>/shared/backups/`
- `/opt/a4-services/<service>/shared/deploy-history.log`
- `/var/lock/svc-deploy-<service>.lock`

## Failure Modes

- Bad download or checksum mismatch: deploy aborts before cutover.
- Missing binary in the unpacked artifact: deploy aborts before cutover.
- Missing or empty secrets file: deploy aborts before cutover.
- Restart failure or unhealthy service after cutover: automatic rollback to `previous`.
- Concurrent deploy or rollback for the same service: lock acquisition fails.
