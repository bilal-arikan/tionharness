package api

import "testing"

// TestSetBaseURL_IPv6WildcardNormalizesToLoopback locks in the fix for the bug
// where strings.Cut(addr, ":") split "[::]:8090" on the colon INSIDE the
// brackets, producing host "[" and leaving the wildcard normalization dead.
// net.SplitHostPort must be used instead so "[::]:8090" resolves the same way
// "0.0.0.0:8090" does: to a connectable loopback URL.
func TestSetBaseURL_IPv6WildcardNormalizesToLoopback(t *testing.T) {
	s := &Server{}
	s.SetBaseURL("[::]:8090")
	if want := "http://127.0.0.1:8090"; s.selfURL != want {
		t.Fatalf("SetBaseURL([::]:8090) = %q, want %q", s.selfURL, want)
	}
}

func TestSetBaseURL_Table(t *testing.T) {
	cases := []struct {
		name string
		addr string
		want string
	}{
		{"ipv4 wildcard", "0.0.0.0:8090", "http://127.0.0.1:8090"},
		{"ipv6 wildcard bracketed", "[::]:8090", "http://127.0.0.1:8090"},
		{"ipv4 loopback", "127.0.0.1:8090", "http://127.0.0.1:8090"},
		{"ipv6 loopback bracketed", "[::1]:8090", "http://[::1]:8090"},
		{"ipv4 lan address", "192.168.1.5:8090", "http://192.168.1.5:8090"},
		{"empty host", ":8090", "http://127.0.0.1:8090"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &Server{}
			s.SetBaseURL(tc.addr)
			if s.selfURL != tc.want {
				t.Fatalf("SetBaseURL(%q) = %q, want %q", tc.addr, s.selfURL, tc.want)
			}
		})
	}
}
