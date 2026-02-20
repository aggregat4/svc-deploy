// Package prune implements the prune command for cleaning up old releases.
package prune

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/a4/svc-deploy/internal/config"
	"github.com/a4/svc-deploy/internal/interfaces"
)

// Deps contains all external dependencies for prune operations.
type Deps struct {
	FS interfaces.FS
}

// Operation performs pruning.
type Operation struct {
	cfg     config.ServiceConfig
	service string
	keep    int
	deps    Deps
}

// Result contains the outcome of pruning.
type Result struct {
	Removed   []string
	Remaining int
}

// New creates a new prune operation.
func New(cfg config.ServiceConfig, service string, keep int, deps Deps) *Operation {
	return &Operation{
		cfg:     cfg,
		service: service,
		keep:    keep,
		deps:    deps,
	}
}

// Run executes the prune operation.
func (op *Operation) Run(_ context.Context) (*Result, error) {
	servicePath := config.ServicePath(op.service)
	releasesPath := filepath.Join(servicePath, "releases")

	// Get current and previous versions (protected)
	protected := make(map[string]bool)

	currentPath := config.CurrentPath(op.service)
	if target, err := op.deps.FS.Readlink(currentPath); err == nil {
		protected[filepath.Base(target)] = true
	}

	previousPath := config.PreviousPath(op.service)
	if target, err := op.deps.FS.Readlink(previousPath); err == nil {
		protected[filepath.Base(target)] = true
	}

	// List all releases
	entries, err := op.deps.FS.ListDirectory(releasesPath)
	if err != nil {
		return nil, fmt.Errorf("listing releases: %w", err)
	}

	// Collect all release versions
	type releaseInfo struct {
		version string
	}

	var releases []releaseInfo
	for _, entry := range entries {
		if !entry.IsDir {
			continue
		}
		releases = append(releases, releaseInfo{version: entry.Name})
	}

	// Sort by version (descending - newest first)
	sort.Slice(releases, func(i, j int) bool {
		return compareVersions(releases[i].version, releases[j].version) > 0
	})

	toKeep := op.keep
	if toKeep < 1 {
		toKeep = op.cfg.KeepReleases
	}
	if toKeep < 1 {
		toKeep = config.DefaultKeepReleases
	}

	result := &Result{
		Removed: []string{},
	}

	// Count how many we're keeping (protected + most recent non-protected)
	kept := 0

	for _, rel := range releases {
		if protected[rel.version] {
			// Always keep protected releases
			kept++
			continue
		}

		if kept < toKeep {
			// Keep this release
			kept++
		} else {
			// Remove this release
			releasePath := filepath.Join(releasesPath, rel.version)
			if err := op.deps.FS.RemoveAll(releasePath); err != nil {
				// Log but continue
				continue
			}
			result.Removed = append(result.Removed, rel.version)
		}
	}

	result.Remaining = kept

	return result, nil
}

// compareVersions compares two semantic versions.
// Returns >0 if v1 > v2, <0 if v1 < v2, 0 if equal.
//
// Supports versions with optional 'v' prefix (e.g., "v1.2.3" or "1.2.3").
// Non-semver versions are compared lexicographically as a fallback.
// Invalid versions sort before valid versions.
func compareVersions(v1, v2 string) int {
	// Parse both versions
	sv1, ok1 := parseSemver(v1)
	sv2, ok2 := parseSemver(v2)

	// If both are valid semver, compare semantically
	if ok1 && ok2 {
		return sv1.compare(sv2)
	}

	// If only one is valid semver, valid sorts after invalid
	if ok1 && !ok2 {
		return 1
	}
	if !ok1 && ok2 {
		return -1
	}

	// Both invalid: fall back to lexicographic compare
	return strings.Compare(v1, v2)
}

// semver represents a parsed semantic version.
type semver struct {
	major int
	minor int
	patch int
	rest  string // pre-release/build metadata (e.g., "-rc1+build123")
}

// parseSemver parses a semantic version string.
// Returns the parsed version and true if valid, or zero value and false if invalid.
// Supports optional 'v' or 'V' prefix.
func parseSemver(v string) (semver, bool) {
	// Strip optional 'v' or 'V' prefix
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")

	// Split into version core and pre-release/build metadata
	parts := strings.SplitN(v, "-", 2)
	core := parts[0]
	rest := ""
	if len(parts) == 2 {
		rest = "-" + parts[1]
	}

	// Parse core version (major.minor.patch)
	coreParts := strings.Split(core, ".")
	if len(coreParts) < 2 || len(coreParts) > 3 {
		return semver{}, false
	}

	var s semver
	var err error
	if s.major, err = strconv.Atoi(coreParts[0]); err != nil {
		return semver{}, false
	}
	if s.minor, err = strconv.Atoi(coreParts[1]); err != nil {
		return semver{}, false
	}
	if len(coreParts) == 3 {
		if s.patch, err = strconv.Atoi(coreParts[2]); err != nil {
			return semver{}, false
		}
	}
	s.rest = rest

	return s, true
}

// compare compares two semvers.
// Returns >0 if s > other, <0 if s < other, 0 if equal.
func (s semver) compare(other semver) int {
	if s.major != other.major {
		if s.major > other.major {
			return 1
		}
		return -1
	}
	if s.minor != other.minor {
		if s.minor > other.minor {
			return 1
		}
		return -1
	}
	if s.patch != other.patch {
		if s.patch > other.patch {
			return 1
		}
		return -1
	}
	// Core version equal, compare pre-release metadata
	// Versions without pre-release sort after versions with pre-release
	if s.rest == "" && other.rest != "" {
		return 1
	}
	if s.rest != "" && other.rest == "" {
		return -1
	}
	return strings.Compare(s.rest, other.rest)
}
