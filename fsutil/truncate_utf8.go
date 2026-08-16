package fsutil

import "unicode/utf8"

// TruncateUTF8 returns the longest prefix of s that is valid UTF-8 and at most
// maxBytes bytes long, never splitting a multi-byte rune. If maxBytes <= 0 it
// returns "". If s is already <= maxBytes bytes (and the cut would not land
// mid-rune) the whole string is returned.
func TruncateUTF8(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}

	part := s
	off := 0
	for len(part) > 0 {
		_, n := utf8.DecodeRuneInString(part)
		if off+n > maxBytes {
			break
		}
		off += n
		part = part[n:]
	}
	return s[:off]
}
