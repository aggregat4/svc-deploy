# svc-deploy Remediation Plan (Open Items)

Purpose: track only unresolved findings from the 2026-02-20 remediation review.

How to use:
1. Work findings top-to-bottom by severity.
2. Mark each checkbox only when acceptance criteria and verification commands pass.
3. Keep this file synchronized with actual test intent and behavior.

## P1 - Medium

[ ] F5 - Missing true end-to-end deploy success coverage in integration tests
Severity: Medium
Scope:
- `internal/integrationtest/integration_test.go:23`
- `internal/integrationtest/integration_test.go:30`
- `internal/integrationtest/integration_test.go:131`
- `internal/integrationtest/integration_test.go:136`
- `internal/integrationtest/integration_test.go:139`

Current behavior:
- `TestFullDeployFlow` is now explicitly a component test and still allows deploy errors.
- `TestEndToEndDeploySuccess` fails on deploy error, but still uses a full mock stack (`MockFS`, `MockArtifactFetcher`) and does not exercise the real fetch+verify+extract path.

Gap:
- The suite still lacks one deploy test that validates real artifact fetch, checksum verification, tar extraction, and deployment success in a single flow.

Fix guidance:
1. Keep `TestFullDeployFlow` as component scope (that part is now aligned).
2. Add or convert one test to use real artifact plumbing end-to-end:
   - real `tar.gz` fixture generation,
   - real HTTP artifact+checksum serving,
   - real fetcher implementation,
   - real filesystem extraction path (no post-extract mock callback),
   - hard failure on any deploy error.
3. Assert concrete deployment outcomes (not only lack of error):
   - deployed version,
   - metadata/history artifacts,
   - current/previous release pointers.

Acceptance criteria:
- Integration test suite contains at least one deploy flow test that fails if real fetch/verify/extract/deploy does not succeed.
- Test naming matches test behavior (component vs end-to-end).
- No test named as end-to-end relies entirely on mock fetch/extract pipeline.

## Verification Checklist (for open items)

[ ] V1 - `go test ./internal/integrationtest -run 'Test.*Deploy.*'`
[ ] V2 - `go test ./...`

## Definition of Done

[ ] D1 - F5 is closed with real deploy success coverage
[ ] D2 - Verification checklist for open items is fully green
