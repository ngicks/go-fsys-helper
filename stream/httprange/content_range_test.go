package httprange

import "testing"

func TestParseContentRange(t *testing.T) {
	for _, tc := range []struct {
		name    string
		in      string
		start   int64
		end     int64
		total   int64
		wantErr bool
	}{
		{name: "head", in: "bytes 0-499/1234", start: 0, end: 499, total: 1234},
		{name: "middle", in: "bytes 500-999/1234", start: 500, end: 999, total: 1234},
		{name: "tail", in: "bytes 1000-1233/1234", start: 1000, end: 1233, total: 1234},
		{name: "whole", in: "bytes 0-1233/1234", start: 0, end: 1233, total: 1234},
		{name: "single_byte", in: "bytes 0-0/1234", start: 0, end: 0, total: 1234},
		{name: "single_byte_object", in: "bytes 0-0/1", start: 0, end: 0, total: 1},
		{name: "unsatisfied", in: "bytes */1234", start: -1, end: -1, total: 1234},
		{name: "unsatisfied_zero_size", in: "bytes */0", start: -1, end: -1, total: 0},
		{name: "unit_upper", in: "BYTES 0-499/1234", start: 0, end: 499, total: 1234},
		{name: "unit_mixed", in: "Bytes 0-499/1234", start: 0, end: 499, total: 1234},
		{name: "surrounding_space", in: " bytes 0-499/1234 ", start: 0, end: 499, total: 1234},
		{name: "empty", in: "", wantErr: true},
		{name: "unit_only", in: "bytes", wantErr: true},
		{name: "no_unit", in: "0-499/1234", wantErr: true},
		{name: "wrong_unit", in: "items 0-499/1234", wantErr: true},
		{name: "range_header_syntax", in: "bytes=0-499/1234", wantErr: true},
		{name: "unknown_total", in: "bytes 0-499/*", wantErr: true},
		{name: "unknown_everything", in: "bytes */*", wantErr: true},
		{name: "no_complete_length", in: "bytes 0-499", wantErr: true},
		{name: "empty_range", in: "bytes /1234", wantErr: true},
		{name: "no_last_byte_pos", in: "bytes 0-/1234", wantErr: true},
		{name: "no_first_byte_pos", in: "bytes -499/1234", wantErr: true},
		{name: "negative_first_byte_pos", in: "bytes -1-5/10", wantErr: true},
		{name: "signed_first_byte_pos", in: "bytes +0-499/1234", wantErr: true},
		{name: "non_numeric_total", in: "bytes 0-499/abc", wantErr: true},
		{name: "non_numeric_range", in: "bytes a-b/1234", wantErr: true},
		{name: "spaced_out", in: "bytes 0 - 499 / 1234", wantErr: true},
		{name: "last_before_first", in: "bytes 499-0/1234", wantErr: true},
		{name: "last_equals_total", in: "bytes 0-1234/1234", wantErr: true},
		{name: "last_beyond_total", in: "bytes 0-5000/1234", wantErr: true},
		{name: "multipart_ranges", in: "bytes 0-499/1234, 800-899/1234", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start, end, total, err := parseContentRange(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf(
						"parseContentRange(%q) = (%d, %d, %d, <nil>), want error",
						tc.in, start, end, total,
					)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseContentRange(%q) returned error: %v", tc.in, err)
			}
			if start != tc.start || end != tc.end || total != tc.total {
				t.Fatalf(
					"parseContentRange(%q) = (%d, %d, %d), want (%d, %d, %d)",
					tc.in, start, end, total, tc.start, tc.end, tc.total,
				)
			}
		})
	}
}
