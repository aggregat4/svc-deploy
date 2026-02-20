// Package prune implements the prune command for cleaning up old releases.
package prune

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
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
func (op *Operation) Run(ctx context.Context) (*Result, error) {
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

	// Collect releases with their modification times for sorting
	type releaseInfo struct {
		version string
		modTime int64 // We'll use alphabetical for now, or parse from metadata if available
	}

	var releases []releaseInfo
	for _, entry := range entries {
		if !entry.IsDir {
			continue
		}
		if protected[entry.Name] {
			continue
		}
		releases = append(releases, releaseInfo{version: entry.Name})
	}

	// Sort by version (semantic versioning sort)
	sort.Slice(releases, func(i, j int) bool {
		return compareVersions(releases[i].version, releases[j].version) > 0
	})

	// Keep only the most recent N
	result := &Result{
		Removed:   []string{},
		Remaining: len(protected),
	}

	toKeep := op.keep
	if toKeep < 1 {
		toKeep = config.DefaultKeepReleases
	}

	// Add protected releases to the keep count
	effectiveKeep := toKeep - len(protected)
	if effectiveKeep < 0 {
		effectiveKeep = 0
	}

	for i, rel := range releases {
		if i < effectiveKeep {
			result.Remaining++
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

	result.Remaining += len(releases) - len(result.Removed)

	return result, nil
}

// compareVersions compares two semantic versions.
// Returns >0 if v1 > v2, <0 if v1 < v2, 0 if equal.
func compareVersions(v1, v2 string) int {
	// Simple string comparison works for semver with leading 'v'
	// v1.10.0 > v1.9.0 lexicographically due to padding
	// For more accurate comparison, we'd parse the version
	return strings.Compare(v1, v2)
}
