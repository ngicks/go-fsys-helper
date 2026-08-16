package fsutil

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateUTF8(t *testing.T) {
	type testCase struct {
		name     string
		s        string
		maxBytes int
		expected string
	}
	tests := []testCase{
		{"empty string, maxBytes 0", "", 0, ""},
		{"empty string, maxBytes negative", "", -1, ""},
		{"empty string, maxBytes positive", "", 5, ""},
		{"ascii, maxBytes 0", "hello", 0, ""},
		{"ascii, maxBytes negative", "hello", -3, ""},
		{"ascii shorter than maxBytes", "hi", 5, "hi"},
		{"ascii equal to maxBytes", "hello", 5, "hello"},
		{"ascii longer than maxBytes", "hello", 3, "hel"},
		{"maxBytes larger than len(s)", "hello", 100, "hello"},

		// "héllo": h(1) é(2 bytes, U+00E9) l(1) l(1) o(1) = 6 bytes total.
		// maxBytes 2 lands in the middle of é, so only "h" fits.
		{"multibyte cut mid-rune keeps only leading ascii", "héllo", 2, "h"},
		// maxBytes 3 exactly fits "h" + "é".
		{"multibyte exact fit of two runes", "héllo", 3, "hé"},
		// maxBytes 1 fits just "h".
		{"multibyte only first ascii rune", "héllo", 1, "h"},

		// "こんにちは": each rune is 3 bytes, 15 bytes total.
		{"japanese cut between runes", "こんにちは", 6, "こん"},
		// maxBytes 7 still only fits 2 full runes (next would need 9 bytes).
		{"japanese cut mid-rune", "こんにちは", 7, "こん"},
		{"japanese cut before first full rune", "こんにちは", 2, ""},
		{"japanese fits exactly one rune", "こんにちは", 3, "こ"},

		// emoji "😀" is 4 bytes (U+1F600).
		{"emoji cut mid-rune yields empty", "😀", 3, ""},
		{"emoji exact fit", "😀", 4, "😀"},
		{"emoji prefix before second emoji", "😀😀", 5, "😀"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateUTF8(tt.s, tt.maxBytes)
			if got != tt.expected {
				t.Errorf(
					"TruncateUTF8(%q, %d): not equal: expected(%q) != actual(%q)",
					tt.s,
					tt.maxBytes,
					tt.expected,
					got,
				)
			}

			// Result must always be valid UTF-8.
			if !utf8.ValidString(got) {
				t.Errorf("TruncateUTF8(%q, %d) = %q is not valid UTF-8", tt.s, tt.maxBytes, got)
			}

			// Result must be a prefix of s.
			if !strings.HasPrefix(tt.s, got) {
				t.Errorf(
					"TruncateUTF8(%q, %d) = %q is not a prefix of input",
					tt.s,
					tt.maxBytes,
					got,
				)
			}

			// Result length must never exceed maxBytes when maxBytes > 0.
			if tt.maxBytes > 0 && len(got) > tt.maxBytes {
				t.Errorf(
					"TruncateUTF8(%q, %d) = %q has len %d > maxBytes",
					tt.s,
					tt.maxBytes,
					got,
					len(got),
				)
			}
		})
	}
}

// TestTruncateUTF8_LongestPrefix proves that adding the next rune to the
// returned prefix would exceed maxBytes (i.e. the result is the longest
// valid prefix that fits).
func TestTruncateUTF8_LongestPrefix(t *testing.T) {
	type testCase struct {
		name     string
		s        string
		maxBytes int
	}
	tests := []testCase{
		{"ascii", "hello", 3},
		{"héllo mid-rune", "héllo", 2},
		{"japanese mid-rune", "こんにちは", 7},
		{"emoji mid-rune", "😀😀", 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateUTF8(tt.s, tt.maxBytes)

			// got must fit.
			if len(got) > tt.maxBytes {
				t.Fatalf("TruncateUTF8(%q, %d) = %q exceeds maxBytes", tt.s, tt.maxBytes, got)
			}

			// There must be a remaining rune (otherwise it is not a meaningful
			// "longest prefix" assertion — the whole string would fit).
			rest := tt.s[len(got):]
			if rest == "" {
				t.Fatalf("expected leftover bytes for %q with maxBytes %d", tt.s, tt.maxBytes)
			}

			// Adding the next rune must exceed maxBytes.
			_, n := utf8.DecodeRuneInString(rest)
			if len(got)+n <= tt.maxBytes {
				t.Errorf(
					"TruncateUTF8(%q, %d) = %q not longest prefix: next rune (%d bytes) still fits",
					tt.s,
					tt.maxBytes,
					got,
					n,
				)
			}
		})
	}
}
