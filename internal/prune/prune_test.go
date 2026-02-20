package prune

import (
	"context"
	"testing"

	"github.com/a4/svc-deploy/internal/config"
	"github.com/a4/svc-deploy/internal/testutil"
)

func TestPruneKeepsCurrentAndPrevious(t *testing.T) {
	ctx := context.Background()

	fs := testutil.NewMockFS()
	symlinkMgr := testutil.NewMockSymlinkManager()

	// Setup service and releases
	fs.AddDir("/opt/a4-services/svc-a")
	fs.AddDir("/opt/a4-services/svc-a/releases")
	for _, v := range []string{"v1.0.0", "v1.1.0", "v1.2.0", "v1.3.0", "v1.4.0", "v1.5.0", "v1.6.0"} {
		fs.AddDir("/opt/a4-services/svc-a/releases/" + v)
	}

	// Current is v1.6.0, previous is v1.5.0
	symlinkMgr.SetCurrentDirect("/opt/a4-services/svc-a", "/opt/a4-services/svc-a/releases/v1.6.0")
	symlinkMgr.SetPreviousDirect("/opt/a4-services/svc-a", "/opt/a4-services/svc-a/releases/v1.5.0")

	// Also set up symlinks in the mock filesystem for the test
	fs.AddSymlink("/opt/a4-services/svc-a/current", "/opt/a4-services/svc-a/releases/v1.6.0")
	fs.AddSymlink("/opt/a4-services/svc-a/previous", "/opt/a4-services/svc-a/releases/v1.5.0")

	svcCfg := config.ServiceConfig{
		KeepReleases: 5,
	}

	deps := Deps{
		FS: fs,
	}

	op := New(svcCfg, "svc-a", 5, deps)
	result, err := op.Run(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// We have 7 releases, keep 5, but current and previous are protected
	// So we should remove: 7 - 5 = 2 releases
	// But protected: v1.5.0 (previous), v1.6.0 (current)
	// We keep 3 more: v1.4.0, v1.3.0, v1.2.0
	// Remove: v1.0.0, v1.1.0

	if len(result.Removed) != 2 {
		t.Errorf("Removed = %d, want 2", len(result.Removed))
	}

	// Should have removed oldest versions
	for _, v := range []string{"v1.0.0", "v1.1.0"} {
		found := false
		for _, r := range result.Removed {
			if r == v {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %s to be removed", v)
		}
	}

	// Remaining should be 5
	if result.Remaining != 5 {
		t.Errorf("Remaining = %d, want 5", result.Remaining)
	}
}

func TestPruneDoesNotRemoveProtected(t *testing.T) {
	ctx := context.Background()

	fs := testutil.NewMockFS()
	symlinkMgr := testutil.NewMockSymlinkManager()

	// Setup service and only 3 releases
	fs.AddDir("/opt/a4-services/svc-a")
	fs.AddDir("/opt/a4-services/svc-a/releases")
	for _, v := range []string{"v1.0.0", "v1.1.0", "v1.2.0"} {
		fs.AddDir("/opt/a4-services/svc-a/releases/" + v)
	}

	// Current is v1.2.0, previous is v1.1.0
	symlinkMgr.SetCurrentDirect("/opt/a4-services/svc-a", "/opt/a4-services/svc-a/releases/v1.2.0")
	symlinkMgr.SetPreviousDirect("/opt/a4-services/svc-a", "/opt/a4-services/svc-a/releases/v1.1.0")
	fs.AddSymlink("/opt/a4-services/svc-a/current", "/opt/a4-services/svc-a/releases/v1.2.0")
	fs.AddSymlink("/opt/a4-services/svc-a/previous", "/opt/a4-services/svc-a/releases/v1.1.0")

	svcCfg := config.ServiceConfig{
		KeepReleases: 5,
	}

	deps := Deps{
		FS: fs,
	}

	op := New(svcCfg, "svc-a", 5, deps)
	result, err := op.Run(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// We only have 3 releases, all should be kept
	if len(result.Removed) != 0 {
		t.Errorf("Removed = %d, want 0", len(result.Removed))
	}

	if result.Remaining != 3 {
		t.Errorf("Remaining = %d, want 3", result.Remaining)
	}
}

func TestPruneCustomKeepCount(t *testing.T) {
	ctx := context.Background()

	fs := testutil.NewMockFS()
	symlinkMgr := testutil.NewMockSymlinkManager()

	// Setup service and 10 releases
	fs.AddDir("/opt/a4-services/svc-a")
	fs.AddDir("/opt/a4-services/svc-a/releases")
	for _, v := range []string{"v1.0.0", "v1.1.0", "v1.2.0", "v1.3.0", "v1.4.0",
		"v1.5.0", "v1.6.0", "v1.7.0", "v1.8.0", "v1.9.0"} {
		fs.AddDir("/opt/a4-services/svc-a/releases/" + v)
	}

	symlinkMgr.SetCurrentDirect("/opt/a4-services/svc-a", "/opt/a4-services/svc-a/releases/v1.9.0")
	symlinkMgr.SetPreviousDirect("/opt/a4-services/svc-a", "/opt/a4-services/svc-a/releases/v1.8.0")
	fs.AddSymlink("/opt/a4-services/svc-a/current", "/opt/a4-services/svc-a/releases/v1.9.0")
	fs.AddSymlink("/opt/a4-services/svc-a/previous", "/opt/a4-services/svc-a/releases/v1.8.0")

	svcCfg := config.ServiceConfig{
		KeepReleases: 5, // This will be overridden by explicit keep=3
	}

	deps := Deps{
		FS: fs,
	}

	// Keep only 3 releases
	op := New(svcCfg, "svc-a", 3, deps)
	result, err := op.Run(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// We have 10 releases, keep 3, but current and previous are protected
	// Effective keep = 3 - 2 = 1 additional release
	// Total kept: 3, removed: 7

	if len(result.Removed) != 7 {
		t.Errorf("Removed = %d, want 7", len(result.Removed))
	}

	if result.Remaining != 3 {
		t.Errorf("Remaining = %d, want 3", result.Remaining)
	}
}

func TestPruneUsesConfigDefault(t *testing.T) {
	ctx := context.Background()

	fs := testutil.NewMockFS()
	symlinkMgr := testutil.NewMockSymlinkManager()

	// Setup service and 7 releases
	fs.AddDir("/opt/a4-services/svc-a")
	fs.AddDir("/opt/a4-services/svc-a/releases")
	for _, v := range []string{"v1.0.0", "v1.1.0", "v1.2.0", "v1.3.0", "v1.4.0", "v1.5.0", "v1.6.0"} {
		fs.AddDir("/opt/a4-services/svc-a/releases/" + v)
	}

	symlinkMgr.SetCurrentDirect("/opt/a4-services/svc-a", "/opt/a4-services/svc-a/releases/v1.6.0")
	symlinkMgr.SetPreviousDirect("/opt/a4-services/svc-a", "/opt/a4-services/svc-a/releases/v1.5.0")
	fs.AddSymlink("/opt/a4-services/svc-a/current", "/opt/a4-services/svc-a/releases/v1.6.0")
	fs.AddSymlink("/opt/a4-services/svc-a/previous", "/opt/a4-services/svc-a/releases/v1.5.0")

	svcCfg := config.ServiceConfig{
		KeepReleases: 3, // Custom default
	}

	deps := Deps{
		FS: fs,
	}

	// Pass 0 for keep, should use config default
	op := New(svcCfg, "svc-a", 0, deps)
	result, err := op.Run(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 7 releases, keep 3, 2 protected -> remove 7 - 3 = 4
	if len(result.Removed) != 4 {
		t.Errorf("Removed = %d, want 4", len(result.Removed))
	}
}

func TestPruneEmptyReleases(t *testing.T) {
	ctx := context.Background()

	fs := testutil.NewMockFS()
	symlinkMgr := testutil.NewMockSymlinkManager()

	// Setup service but no releases (only symlinks pointing to non-existent dirs)
	fs.AddDir("/opt/a4-services/svc-a")
	fs.AddDir("/opt/a4-services/svc-a/releases")
	// Note: we intentionally don't add the v1.0.0 directory to simulate empty state
	symlinkMgr.SetCurrentDirect("/opt/a4-services/svc-a", "/opt/a4-services/svc-a/releases/v1.0.0")
	fs.AddSymlink("/opt/a4-services/svc-a/current", "/opt/a4-services/svc-a/releases/v1.0.0")

	svcCfg := config.ServiceConfig{
		KeepReleases: 5,
	}

	deps := Deps{
		FS: fs,
	}

	op := New(svcCfg, "svc-a", 5, deps)
	result, err := op.Run(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Removed) != 0 {
		t.Errorf("Removed = %d, want 0", len(result.Removed))
	}

	// No releases exist in the directory (even though current symlink points to one)
	if result.Remaining != 0 {
		t.Errorf("Remaining = %d, want 0", result.Remaining)
	}
}
