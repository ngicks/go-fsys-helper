package httprange

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func TestRedactURL(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{
			name: "plain",
			in:   "https://example.com/bucket/obj",
			want: "https://example.com/bucket/obj",
		},
		{
			name: "userinfo",
			in:   "https://user:hunter2@example.com/obj",
			want: "https://example.com/obj",
		},
		{
			name: "username_only",
			in:   "https://user@example.com/obj",
			want: "https://example.com/obj",
		},
		{
			name: "query",
			in:   "https://example.com/obj?token=s3cr3t&x=1",
			want: "https://example.com/obj",
		},
		{
			name: "empty_query",
			in:   "https://example.com/obj?",
			want: "https://example.com/obj",
		},
		{
			name: "fragment",
			in:   "https://example.com/obj#s3cr3t",
			want: "https://example.com/obj",
		},
		{
			name: "everything",
			in:   "https://user:hunter2@example.com:8443/a/b?token=s3cr3t#frag",
			want: "https://example.com:8443/a/b",
		},
		{
			name: "escaped_path_preserved",
			in:   "https://example.com/a%2Fb?token=s3cr3t",
			want: "https://example.com/a%2Fb",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u, err := url.Parse(tc.in)
			if err != nil {
				t.Fatalf("url.Parse(%q) returned error: %v", tc.in, err)
			}
			if got := redactURL(u); got != tc.want {
				t.Fatalf("redactURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if got := u.String(); got != tc.in {
				t.Fatalf("redactURL mutated its argument: %q, want %q", got, tc.in)
			}
		})
	}
}

func TestRedactRawURL(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{
			name: "parsable",
			in:   "https://user:hunter2@example.com/obj?token=s3cr3t",
			want: "https://example.com/obj",
		},
		{
			name: "unparsable",
			in:   "https://%zz@example.com/obj?token=s3cr3t",
			want: redactedURLPlaceholder,
		},
		{
			name: "empty",
			in:   "",
			want: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := redactRawURL(tc.in)
			if got != tc.want {
				t.Fatalf("redactRawURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if strings.Contains(got, "s3cr3t") || strings.Contains(got, "hunter2") {
				t.Fatalf("redactRawURL(%q) = %q, leaked a secret", tc.in, got)
			}
		})
	}
}

func TestRedactURLError(t *testing.T) {
	underlying := errors.New("connection reset by peer")

	for _, tc := range []struct {
		name    string
		err     error
		wantMsg string
	}{
		{
			name: "userinfo",
			err: &url.Error{
				Op:  "Get",
				URL: "https://user:hunter2@example.com/obj",
				Err: underlying,
			},
			wantMsg: `Get "https://example.com/obj": connection reset by peer`,
		},
		{
			name: "query_and_fragment",
			err: &url.Error{
				Op:  "Get",
				URL: "https://example.com/obj?token=s3cr3t#hunter2",
				Err: underlying,
			},
			wantMsg: `Get "https://example.com/obj": connection reset by peer`,
		},
		{
			name: "unparsable",
			err: &url.Error{
				Op:  "Get",
				URL: "https://%zz@example.com/obj?token=s3cr3t",
				Err: underlying,
			},
			wantMsg: fmt.Sprintf(
				"Get %q: connection reset by peer", redactedURLPlaceholder,
			),
		},
		{
			name: "wrapped_deeper",
			err: fmt.Errorf("probe: %w", &url.Error{
				Op:  "Get",
				URL: "https://user:hunter2@example.com/obj?token=s3cr3t",
				Err: underlying,
			}),
			wantMsg: `Get "https://example.com/obj": connection reset by peer`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := redactURLError(tc.err)

			if got.Error() != tc.wantMsg {
				t.Fatalf("redactURLError(...).Error() = %q, want %q", got.Error(), tc.wantMsg)
			}
			for _, secret := range []string{"hunter2", "s3cr3t", "token"} {
				if strings.Contains(got.Error(), secret) {
					t.Fatalf("redactURLError(...).Error() = %q, leaked %q", got.Error(), secret)
				}
			}
			if !errors.Is(got, underlying) {
				t.Fatalf("errors.Is(redactURLError(...), underlying) = false, want true")
			}
			ue, ok := errors.AsType[*url.Error](got)
			if !ok {
				t.Fatalf("errors.AsType[*url.Error](redactURLError(...)) = false, want true")
			}
			for _, secret := range []string{"hunter2", "s3cr3t", "token"} {
				if strings.Contains(ue.URL, secret) {
					t.Fatalf("redacted url.Error.URL = %q, leaked %q", ue.URL, secret)
				}
			}
		})
	}
}

func TestRedactURLError_passthrough(t *testing.T) {
	if got := redactURLError(nil); got != nil {
		t.Fatalf("redactURLError(nil) = %v, want nil", got)
	}

	plain := errors.New("no url here")
	if got := redactURLError(plain); got != plain {
		t.Fatalf("redactURLError(plain) = %v, want the argument unchanged", got)
	}

	wrapped := fmt.Errorf("probe: %w", plain)
	if got := redactURLError(wrapped); got != wrapped {
		t.Fatalf("redactURLError(wrapped) = %v, want the argument unchanged", got)
	}
}
