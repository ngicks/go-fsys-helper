package paths

import (
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

// collect drains a component iterator into a slice.
func collect(seq func(yield func(string) bool)) []string {
	var out []string
	seq(func(s string) bool {
		out = append(out, s)
		return true
	})
	return out
}

// fs rewrites a slash-form expectation to the host separator so the tables read
// naturally on every platform.
func fs(ss ...string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = filepath.FromSlash(s)
	}
	return out
}

func TestPathFromHead(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"simple", "a/b/c", fs("a", "a/b", "a/b/c")},
		{"single", "a", fs("a")},
		{"dot", ".", nil},
		{"trailing slash", "a/b/", fs("a", "a/b")},
		{"doubled separators", "a//b", fs("a", "a/b")},
		{"interior dotdot cleaned away", "a/../b", fs("b")},
		{"interior dot cleaned away", "a/./b", fs("a", "a/b")},
		{"leading dotdot kept", "../a", fs("..", "../a")},
		{"empty cleans to dot", "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := collect(PathFromHead(filepath.FromSlash(tc.in)))
			if !slices.Equal(got, tc.want) {
				t.Errorf("PathFromHead(%q) = %q, want %q", tc.in, got, tc.want)
			}
			assertNoEmptyTokens(t, got)
		})
	}
}

func TestPathFromTail(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"simple", "a/b/c", fs("a/b/c", "a/b", "a")},
		{"single", "a", fs("a")},
		{"dot", ".", nil},
		{"trailing slash", "a/b/", fs("a/b", "a")},
		{"doubled separators", "a//b", fs("a/b", "a")},
		{"interior dotdot cleaned away", "a/../b", fs("b")},
		{"leading dotdot kept", "../a", fs("../a", "..")},
		{"empty cleans to dot", "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := collect(PathFromTail(filepath.FromSlash(tc.in)))
			if !slices.Equal(got, tc.want) {
				t.Errorf("PathFromTail(%q) = %q, want %q", tc.in, got, tc.want)
			}
			assertNoEmptyTokens(t, got)
		})
	}
}

// TestAbsoluteInputs exercises the absolute-path rules; the bare filesystem root
// is never emitted as its own token. Unix-only (Windows volume handling differs
// and is covered implicitly by the relative tables).
func TestAbsoluteInputs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix absolute-path layout")
	}
	if got := collect(PathFromHead("/a/b")); !slices.Equal(got, []string{"/a", "/a/b"}) {
		t.Errorf("PathFromHead(/a/b) = %q, want [/a /a/b]", got)
	}
	if got := collect(PathFromTail("/a/b")); !slices.Equal(got, []string{"/a/b", "/a"}) {
		t.Errorf("PathFromTail(/a/b) = %q, want [/a/b /a]", got)
	}
	// Root itself yields nothing (no named components).
	if got := collect(PathFromHead("/")); got != nil {
		t.Errorf("PathFromHead(/) = %q, want nil", got)
	}
	if got := collect(PathFromTail("/")); got != nil {
		t.Errorf("PathFromTail(/) = %q, want nil", got)
	}
	if got := collect(PathFromHead("/a")); !slices.Equal(got, []string{"/a"}) {
		t.Errorf("PathFromHead(/a) = %q, want [/a]", got)
	}
}

// assertNoEmptytokens enforces the package invariant: no token is empty.
func assertNoEmptyTokens(t *testing.T, got []string) {
	t.Helper()
	for _, s := range got {
		if s == "" {
			t.Errorf("emitted an empty token in %q", got)
		}
	}
}

// TestEarlyStop confirms both iterators honor a false return from yield.
func TestEarlyStop(t *testing.T) {
	var got []string
	PathFromHead(filepath.FromSlash("a/b/c"))(func(s string) bool {
		got = append(got, s)
		return len(got) < 2
	})
	if !slices.Equal(got, fs("a", "a/b")) {
		t.Errorf("PathFromHead early stop = %q, want [a a/b]", got)
	}
	got = nil
	PathFromTail(filepath.FromSlash("a/b/c"))(func(s string) bool {
		got = append(got, s)
		return len(got) < 2
	})
	if !slices.Equal(got, fs("a/b/c", "a/b")) {
		t.Errorf("PathFromTail early stop = %q, want [a/b/c a/b]", got)
	}
}
