package semver

import (
	"testing"
)

func TestCompare(t *testing.T) {
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
			result := Compare(tt.v1, tt.v2)
			if tt.expected > 0 && result <= 0 {
				t.Errorf("Compare(%q, %q) = %d, want > 0", tt.v1, tt.v2, result)
			}
			if tt.expected < 0 && result >= 0 {
				t.Errorf("Compare(%q, %q) = %d, want < 0", tt.v1, tt.v2, result)
			}
			if tt.expected == 0 && result != 0 {
				t.Errorf("Compare(%q, %q) = %d, want 0", tt.v1, tt.v2, result)
			}
		})
	}
}

func TestCompare_InvalidVersions(t *testing.T) {
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
			result := Compare(tt.v1, tt.v2)
			if tt.expected > 0 && result <= 0 {
				t.Errorf("Compare(%q, %q) = %d, want > 0", tt.v1, tt.v2, result)
			}
			if tt.expected < 0 && result >= 0 {
				t.Errorf("Compare(%q, %q) = %d, want < 0", tt.v1, tt.v2, result)
			}
			if tt.expected == 0 && result != 0 {
				t.Errorf("Compare(%q, %q) = %d, want 0", tt.v1, tt.v2, result)
			}
		})
	}
}

func TestParse(t *testing.T) {
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
			sv, ok := parse(tt.input)
			if ok != tt.expectOK {
				t.Errorf("parse(%q) ok = %v, want %v", tt.input, ok, tt.expectOK)
				return
			}
			if !tt.expectOK {
				return
			}
			if sv.major != tt.major || sv.minor != tt.minor || sv.patch != tt.patch {
				t.Errorf("parse(%q) = {%d,%d,%d}, want {%d,%d,%d}",
					tt.input, sv.major, sv.minor, sv.patch, tt.major, tt.minor, tt.patch)
			}
			if sv.rest != tt.rest {
				t.Errorf("parse(%q).rest = %q, want %q", tt.input, sv.rest, tt.rest)
			}
		})
	}
}
