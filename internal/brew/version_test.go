package brew

import (
	"testing"
)

func TestVersionCompare(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int
	}{
		{"1.0", "1.0", 0},
		{"1.0", "1.1", -1},
		{"1.1", "1.0", 1},
		{"2.0.0", "1.9.9", 1},
		{"1.9.9", "2.0.0", -1},
		{"1.0.0", "1.0", 0},
		{"1.0", "1.0.0", 0},
		{"1.0_1", "1.0", 0},
		{"1.0_2", "1.0_1", 0},
		{"3.2.1", "3.2.1", 0},
		{"3.2.1", "3.2.2", -1},
		{"10.0", "9.9", 1},
		{"0.9.0", "1.0.0", -1},
	}

	for _, tt := range tests {
		result := versionCompare(tt.v1, tt.v2)
		if result != tt.expected {
			t.Errorf("versionCompare(%q, %q) = %d, want %d", tt.v1, tt.v2, result, tt.expected)
		}
	}
}

func TestIsOutdated(t *testing.T) {
	tests := []struct {
		installed string
		latest    string
		expected  bool
	}{
		{"1.0", "1.0", false},
		{"1.0", "1.1", true},
		{"1.0", "2.0", true},
		{"2.0", "1.0", false},
		{"1.0_1", "1.0", false},
		{"1.0_2", "1.0_1", false},
		{"1.0.0", "1.0.1", true},
		{"10.0", "9.9", false},
	}

	for _, tt := range tests {
		result := isOutdated(tt.installed, tt.latest)
		if result != tt.expected {
			t.Errorf("isOutdated(%q, %q) = %v, want %v", tt.installed, tt.latest, result, tt.expected)
		}
	}
}

func TestStripRevision(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"1.0", "1.0"},
		{"1.0_1", "1.0"},
		{"1.0_2", "1.0"},
		{"2.3.4", "2.3.4"},
		{"2.3.4_1", "2.3.4"},
		{"abc", "abc"},
		{"abc_123", "abc"},
	}

	for _, tt := range tests {
		result := stripRevision(tt.input)
		if result != tt.expected {
			t.Errorf("stripRevision(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
