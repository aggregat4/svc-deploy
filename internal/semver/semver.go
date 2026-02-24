// Package semver provides semantic version comparison utilities.
package semver

import (
	"strconv"
	"strings"
)

// Compare compares two semantic versions.
// Returns >0 if v1 > v2, <0 if v1 < v2, 0 if equal.
//
// Supports versions with optional 'v' prefix (e.g., "v1.2.3" or "1.2.3").
// Non-semver versions are compared lexicographically as a fallback.
// Invalid versions sort before valid versions.
func Compare(v1, v2 string) int {
	// Parse both versions
	sv1, ok1 := parse(v1)
	sv2, ok2 := parse(v2)

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

// parse parses a semantic version string.
// Returns the parsed version and true if valid, or zero value and false if invalid.
// Supports optional 'v' or 'V' prefix.
func parse(v string) (semver, bool) {
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
