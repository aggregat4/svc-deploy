# svc-deploy Remediation Plan

Purpose: close correctness, spec-compliance, architecture, and test gaps found in the first implementation review.

How to use: mark each checkbox when complete. Each item has explicit acceptance criteria so progress can be audited.

## P0 - Blockers

[x] R1 - Fix CLI argument parsing hang and command/flag routing
Scope: `cmd/svc-deploy/main.go`
Changes:
- Parse global flags before command selection.
- Ensure parser always advances index for positional args.
- Support both `svc-deploy --version` and `svc-deploy --help` without config loading.
- Keep subcommand usage errors deterministic.
Acceptance criteria:
- `svc-deploy deploy svc-a v1.0.0` does not hang.
- `svc-deploy --version` exits `0` without trying to load config.
- `svc-deploy --help` exits `0` without trying to load config.
- CLI tests added and passing.

[x] R2 - Add CLI contract test suite
Scope: `cmd/svc-deploy/*_test.go`
Changes:
- Add table-driven tests for exit codes/stdout/stderr for:
  - `--version`, `--help`
  - unknown command
  - missing args per command
  - missing config
  - `--json` error output shape
Acceptance criteria:
- Tests run under `go test ./...`.
- Output/exit assertions cover required CLI contract scenarios from spec.

[x] R3 - Fix real artifact extraction (`.tar.gz`) and harden archive extraction
Scope: `cmd/svc-deploy/impl.go`
Changes:
- Detect/decode gzip before tar read.
- Reject path traversal (`..`) and absolute target paths during extraction.
- Ensure parent dirs exist for symlink extraction and reject unsafe symlink targets if needed.
Acceptance criteria:
- Unit/integration tests cover valid `.tar.gz` extraction and traversal rejection.
- Deploy path uses hardened extraction in real implementation.

## P1 - Spec compliance and correctness

[x] R4 - Add deploy post-success prune step
Scope: `internal/deploy/deploy.go`
Changes:
- Invoke prune logic after successful health check and metadata/history writes.
- Respect configured keep count and protect `current`/`previous`.
Acceptance criteria:
- Successful deploy prunes old releases automatically.
- Test verifies prune is called as part of deploy success flow.

[x] R5 - Place runtime config at shared path required by systemd contract
Scope: `internal/deploy/deploy.go`, `internal/config/config.go`
Changes:
- Deploy runtime config to `/opt/a4-services/<service>/shared/config/runtime.env`.
- If symlink strategy is chosen, ensure final path matches spec contract.
- Keep config commit in release metadata.
Acceptance criteria:
- Runtime env file exists at shared config path after deploy.
- Systemd contract path in spec is satisfied.
- Tests verify location and content.

[x] R6 - Add required deploy preflights (low disk + secrets file existence)
Scope: `internal/deploy/deploy.go`
Changes:
- Add free-space preflight using `FS.DiskFree`.
- Add secret-file preflight for `/etc/a4-services/<service>.env` existence and basic file checks required by scope.
- Abort before symlink switch on preflight failures.
Acceptance criteria:
- Low disk and missing secret file both fail deploy before cutover.
- Tests cover both failures.

[x] R7 - Fix rollback history target version logging
Scope: `internal/rollback/rollback.go`
Changes:
- Persist resolved target version (not raw user arg) in rollback history.
Acceptance criteria:
- `rollback <service>` (implicit previous) writes correct target version in history.
- Regression test added.

[ ] R8 - Fix prune semver ordering
Scope: `internal/prune/prune.go`
Changes:
- Replace lexicographic compare with semantic version compare.
- Define behavior for invalid/non-semver tags and test it.
Acceptance criteria:
- `v1.10.0` sorts newer than `v1.9.0`.
- Tests cover ordering edge cases.

[ ] R9 - Store effective source artifact URL in release metadata
Scope: `internal/deploy/deploy.go`
Changes:
- Write rendered artifact URL (the actual fetched source) into `metadata/release.json`.
Acceptance criteria:
- Metadata `source_url` equals concrete URL used during deploy.
- Test verifies metadata fields.

[ ] R10 - Fix status metadata path resolution for relative symlink targets
Scope: `internal/status/status.go`
Changes:
- Resolve release metadata path relative to service root when symlink target is relative.
Acceptance criteria:
- Status works for both absolute and relative `current` symlink targets.
- Tests added for both modes.

[ ] R11 - Stop silently dropping metadata/history write errors
Scope: `internal/deploy/deploy.go`, `internal/rollback/rollback.go`
Changes:
- Handle write errors explicitly.
- Decide policy: fail operation or return success with warning and structured result flag.
Acceptance criteria:
- Error policy is documented in code and tests.
- No ignored metadata/history write failures in deploy/rollback paths.

[ ] R12 - Harden checksum parsing
Scope: `cmd/svc-deploy/impl.go`
Changes:
- Validate checksum file format before indexing tokens.
- Return clear error for empty/malformed checksum files.
Acceptance criteria:
- Malformed checksum never panics.
- Tests cover empty and invalid checksum payloads.

## P2 - Architecture and test quality upgrades

[ ] R13 - Separate production and test-only implementations cleanly
Scope: `cmd/svc-deploy/impl.go`, `internal/testutil/*`
Changes:
- Remove unused mock types from production package.
- Keep all mocks in `internal/testutil`.
Acceptance criteria:
- Production binary package contains only runtime implementations.
- No dead mock code in `cmd/svc-deploy`.

[ ] R14 - Expand integration-style tests to use realistic local fixtures
Scope: `internal/*`, `testdata/`
Changes:
- Add local fixture generation for real `.tar.gz` artifact and checksum.
- Add `httptest.Server` artifact serving for deploy integration path.
- Add local health mock behaviors: healthy/unhealthy/timeout/delayed healthy.
Acceptance criteria:
- Integration-style tests exercise real fetch/verify/extract path.
- No external service dependency.

[ ] R15 - Add missing scenario tests from spec
Scope: `internal/deploy/deploy_test.go`, `internal/status/status_test.go`, `cmd/svc-deploy/*_test.go`
Changes:
- Add tests for:
  - deploy restart failure triggers rollback
  - deploy success writes metadata + history
  - secret preflight failure before cutover
  - status reads active release from symlink + metadata
Acceptance criteria:
- All missing scenarios have direct tests.
- Scenario list in spec can be mapped one-to-one to test coverage.

## Suggested execution order

[ ] E1 - Complete all P0 items (R1-R3)
[ ] E2 - Complete P1 items impacting deploy flow (R4-R6, R9, R11, R12)
[ ] E3 - Complete P1 items impacting rollback/status/prune (R7, R8, R10)
[ ] E4 - Complete P2 cleanup and test hardening (R13-R15)
[ ] E5 - Run full verification and capture evidence

## Verification checklist

[ ] V1 - `go test ./...` passes
[ ] V2 - `go vet ./...` passes
[ ] V3 - Manual smoke:
- `svc-deploy --version` works without config
- Deploy command no longer hangs
- Prune ordering validated with `v1.9.0` vs `v1.10.0`
[ ] V4 - Spec compliance review updated with pass/fail per requirement

## Definition of done

[ ] D1 - No P0 or P1 items remain unchecked
[ ] D2 - Required spec behaviors are implemented and tested
[ ] D3 - Test suite contains CLI contract, state-machine, and integration-style coverage per spec
[ ] D4 - Final review shows no known critical or high-severity gaps
