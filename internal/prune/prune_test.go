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

func TestCompareVersions_SemverOrdering(t *testing.T) {
	tests := []struct {
		name     string
		v1       string
		v2       string
		expected int // >0 if v1 > v2, <0 if v1 < v2, 0 if equal
	}{
		// Basic semver ordering
		{"v1.10.0 > v1.9.0", "v1.10.0", "v1.9.0", 1},
		{"v1.9.0 < v1.10.0", "v1.9.0", "v1.10.0", -1},
		{"v2.0.0 > v1.10.0", "v2.0.0", "v1.10.0", 1},
		{"v1.0.0 < v2.0.0", "v1.0.0", "v2.0.0", -1},

		// Patch version comparison
		{"v1.0.10 > v1.0.9", "v1.0.10", "v1.0.9", 1},
		{"v1.0.1 > v1.0.0", "v1.0.1", "v1.0.0", 1},

		// Minor version comparison
		{"v1.10.0 > v1.9.5", "v1.10.0", "v1.9.5", 1},
		{"v1.2.0 > v1.1.100", "v1.2.0", "v1.1.100", 1},

		// Equal versions
		{"v1.0.0 == v1.0.0", "v1.0.0", "v1.0.0", 0},
		{"v2.5.3 == v2.5.3", "v2.5.3", "v2.5.3", 0},

		// With and without 'v' prefix
		{"v1.0.0 == 1.0.0", "v1.0.0", "1.0.0", 0},
		{"1.0.0 == v1.0.0", "1.0.0", "v1.0.0", 0},

		// Pre-release versions (versions without pre-release sort after)
		{"v1.0.0 > v1.0.0-rc1", "v1.0.0", "v1.0.0-rc1", 1},
		{"v1.0.0-rc1 < v1.0.0", "v1.0.0-rc1", "v1.0.0", -1},
		{"v1.0.0-rc2 > v1.0.0-rc1", "v1.0.0-rc2", "v1.0.0-rc1", 1},

		// Two-component versions (major.minor)
		{"v1.10 > v1.9", "v1.10", "v1.9", 1},
		{"v2.0 > v1.99", "v2.0", "v1.99", 1},
		{"v1.0 == v1.0", "v1.0", "v1.0", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareVersions(tt.v1, tt.v2)
			if tt.expected > 0 && result <= 0 {
				t.Errorf("compareVersions(%q, %q) = %d, want > 0", tt.v1, tt.v2, result)
			}
			if tt.expected < 0 && result >= 0 {
				t.Errorf("compareVersions(%q, %q) = %d, want < 0", tt.v1, tt.v2, result)
			}
			if tt.expected == 0 && result != 0 {
				t.Errorf("compareVersions(%q, %q) = %d, want 0", tt.v1, tt.v2, result)
			}
		})
	}
}

func TestCompareVersions_InvalidVersions(t *testing.T) {
	tests := []struct {
		name     string
		v1       string
		v2       string
		expected int
	}{
		// Invalid versions fall back to lexicographic compare
		{"invalid vs invalid - lexicographic", "abc", "def", -1},
		{"invalid vs invalid - equal", "abc", "abc", 0},
		{"invalid vs invalid - reversed", "xyz", "abc", 1},

		// Valid semver sorts after invalid
		{"valid > invalid", "v1.0.0", "abc", 1},
		{"invalid < valid", "abc", "v1.0.0", -1},

		// Empty strings are invalid
		{"empty vs valid", "", "v1.0.0", -1},
		{"valid vs empty", "v1.0.0", "", 1},

		// Mixed invalid patterns
		{"latest vs v1.0.0", "latest", "v1.0.0", -1},
		{"stable vs v1.0.0", "stable", "v1.0.0", -1},
		{"v1.0 == v1.0.0 (2-component)", "v1.0", "v1.0.0", 0}, // 2-component equals 3-component with patch=0
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareVersions(tt.v1, tt.v2)
			if tt.expected > 0 && result <= 0 {
				t.Errorf("compareVersions(%q, %q) = %d, want > 0", tt.v1, tt.v2, result)
			}
			if tt.expected < 0 && result >= 0 {
				t.Errorf("compareVersions(%q, %q) = %d, want < 0", tt.v1, tt.v2, result)
			}
			if tt.expected == 0 && result != 0 {
				t.Errorf("compareVersions(%q, %q) = %d, want 0", tt.v1, tt.v2, result)
			}
		})
	}
}

func TestParseSemver(t *testing.T) {
	tests := []struct {
		input    string
		expectOK bool
		major    int
		minor    int
		patch    int
		rest     string
	}{
		// Valid versions
		{"v1.2.3", true, 1, 2, 3, ""},
		{"1.2.3", true, 1, 2, 3, ""},
		{"V1.2.3", true, 1, 2, 3, ""},
		{"v2.0.0", true, 2, 0, 0, ""},
		{"v0.0.1", true, 0, 0, 1, ""},
		{"v1.2.3-rc1", true, 1, 2, 3, "-rc1"},
		{"v1.2.3-alpha.1", true, 1, 2, 3, "-alpha.1"},

		// Two-component versions
		{"v1.2", true, 1, 2, 0, ""},
		{"1.2", true, 1, 2, 0, ""},

		// Invalid versions
		{"", false, 0, 0, 0, ""},
		{"v1", false, 0, 0, 0, ""},
		{"v1.2.3.4", false, 0, 0, 0, ""},
		{"abc", false, 0, 0, 0, ""},
		{"latest", false, 0, 0, 0, ""},
		{"v1.a.3", false, 0, 0, 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			sv, ok := parseSemver(tt.input)
			if ok != tt.expectOK {
				t.Errorf("parseSemver(%q) ok = %v, want %v", tt.input, ok, tt.expectOK)
				return
			}
			if !tt.expectOK {
				return
			}
			if sv.major != tt.major || sv.minor != tt.minor || sv.patch != tt.patch {
				t.Errorf("parseSemver(%q) = {%d,%d,%d}, want {%d,%d,%d}",
					tt.input, sv.major, sv.minor, sv.patch, tt.major, tt.minor, tt.patch)
			}
			if sv.rest != tt.rest {
				t.Errorf("parseSemver(%q).rest = %q, want %q", tt.input, sv.rest, tt.rest)
			}
		})
	}
}

func TestPruneSemverOrdering(t *testing.T) {
	ctx := context.Background()

	fs := testutil.NewMockFS()
	symlinkMgr := testutil.NewMockSymlinkManager()

	// Setup service with versions that would sort incorrectly lexicographically
	// v1.10.0 should be newer than v1.9.0, but lexicographically "v1.10.0" < "v1.9.0"
	fs.AddDir("/opt/a4-services/svc-a")
	fs.AddDir("/opt/a4-services/svc-a/releases")
	versions := []string{"v1.8.0", "v1.9.0", "v1.10.0", "v1.11.0"}
	for _, v := range versions {
		fs.AddDir("/opt/a4-services/svc-a/releases/" + v)
	}

	// Current is v1.11.0, previous is v1.10.0
	symlinkMgr.SetCurrentDirect("/opt/a4-services/svc-a", "/opt/a4-services/svc-a/releases/v1.11.0")
	symlinkMgr.SetPreviousDirect("/opt/a4-services/svc-a", "/opt/a4-services/svc-a/releases/v1.10.0")
	fs.AddSymlink("/opt/a4-services/svc-a/current", "/opt/a4-services/svc-a/releases/v1.11.0")
	fs.AddSymlink("/opt/a4-services/svc-a/previous", "/opt/a4-services/svc-a/releases/v1.10.0")

	svcCfg := config.ServiceConfig{
		KeepReleases: 3,
	}

	deps := Deps{
		FS: fs,
	}

	op := New(svcCfg, "svc-a", 3, deps)
	result, err := op.Run(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With semver ordering, we keep: v1.11.0 (current), v1.10.0 (previous), v1.9.0
	// We should remove: v1.8.0
	if len(result.Removed) != 1 {
		t.Errorf("Removed = %d, want 1", len(result.Removed))
	}

	// Verify v1.8.0 was removed (oldest when using semver)
	found := false
	for _, r := range result.Removed {
		if r == "v1.8.0" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected v1.8.0 to be removed, got: %v", result.Removed)
	}

	// Verify v1.9.0 was kept (not removed)
	for _, r := range result.Removed {
		if r == "v1.9.0" {
			t.Errorf("v1.9.0 should be kept, not removed")
		}
	}
}
