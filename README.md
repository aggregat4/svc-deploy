# svc-deploy

`svc-deploy` is a host-local deployment manager for Go services, with optional per-release SQLite handling, intended for a single Linux VPS that uses `systemd` instead of containers or an orchestrator: it downloads versioned release tarballs, verifies SHA256 checksums, unpacks immutable release directories under `/opt/a4-services/<service>/releases/<version>`, updates `current` and `previous` symlinks, restarts the service, waits for an HTTP health check to return `2xx`, rolls back automatically if startup fails, writes release metadata and deploy history on success, and can later report status, perform explicit rollbacks, and prune old releases.

The full operator guide lives in `cmd/svc-deploy/manual.md` and is embedded into the binary, so an installed copy can print it with `svc-deploy manual`.

## Build

`svc-deploy` requires Go 1.26 or newer.

```bash
make build
make test
```

The build artifact is written to `./build/svc-deploy`. For a direct build without the helper script:

```bash
go build -trimpath -o ./build/svc-deploy ./cmd/svc-deploy
```

Developer checks:

```bash
make fmt
make fmt-check
make vet
make lint
```

`make lint` and `make fmt` install pinned tool versions into `./.tools/bin` when needed.

Install it on the target host as root with the standard Unix `install` utility:

```bash
install -m 0755 ./build/svc-deploy /usr/local/bin/svc-deploy
```

That command copies the built binary to `/usr/local/bin/svc-deploy` and sets its mode to `0755` in one step.

## Releases

Release builds are handled by `release.yml` The intended flow is:

1. Create a new release in the GitHub UI.
2. Let GitHub create the new tag as part of that release.
3. Publish the release.
4. GitHub Actions runs the release workflow, tests the tagged code, builds Linux `amd64` and `arm64` tarballs, generates `checksums.txt`, and attaches those assets back to the published release.

Read the full manual in either place:

```bash
svc-deploy manual
svc-deploy help manual
```
