package vroot

import (
	"path/filepath"
	"runtime"
	"testing"
)

// TestCleanToSlash covers platform-independent expectations for cleanToSlash.
// Backslash-input cases that only behave correctly on Windows live in
// TestCleanToSlash_Windows.
func TestCleanToSlash(t *testing.T) {
	for _, tt := range []struct {
		in, want string
	}{
		{"", "."},           // filepath.Clean("") == "."
		{".", "."},          // "." stays "."
		{"./", "."},         // trailing slash removed
		{"./a", "a"},        // leading "./" removed by Clean
		{"a", "a"},          // single element
		{"a/b", "a/b"},      // passthrough
		{"a/b/", "a/b"},     // trailing slash dropped
		{"a//b", "a/b"},     // duplicate slash collapsed
		{"a/./b", "a/b"},    // inner "./" removed
		{"a/b/../c", "a/c"}, // ".." resolved
	} {
		if got := cleanToSlash(tt.in); got != tt.want {
			t.Errorf("cleanToSlash(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCleanFromSlash(t *testing.T) {
	join := filepath.Join // alias for readability in the table

	for _, tt := range []struct {
		in, want string
	}{
		{"", "."},
		{".", "."},
		{"./", "."},
		{"./a", "a"},
		{"a", "a"},
		{"a/b", join("a", "b")},
		{"a/b/", join("a", "b")},
		{"a//b", join("a", "b")},
		{"a/./b", join("a", "b")},
		{"a/b/../c", join("a", "c")},
	} {
		if got := cleanFromSlash(tt.in); got != tt.want {
			t.Errorf("cleanFromSlash(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestCleanToSlash_Windows pins behavior for backslash inputs. On Linux,
// filepath.Clean treats "\" as a regular byte so these expectations don't
// hold; the test is therefore Windows-only.
func TestCleanToSlash_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific separator handling")
	}
	for _, tt := range []struct {
		in, want string
	}{
		{`.\a`, "a"},
		{`a\b`, "a/b"},
		{`a\b\..\c`, "a/c"},
	} {
		if got := cleanToSlash(tt.in); got != tt.want {
			t.Errorf("cleanToSlash(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestCleanFromSlash_Windows checks that forward-slash inputs end up in
// backslash form after cleanFromSlash on Windows.
func TestCleanFromSlash_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific separator handling")
	}
	for _, tt := range []struct {
		in, want string
	}{
		{"./a", "a"},
		{"a/b", `a\b`},
		{"a/b/../c", `a\c`},
	} {
		if got := cleanFromSlash(tt.in); got != tt.want {
			t.Errorf("cleanFromSlash(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
