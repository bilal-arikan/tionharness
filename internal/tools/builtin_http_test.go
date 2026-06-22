package tools

import (
	"net"
	"testing"
)

func TestIsBlockedIP(t *testing.T) {
	cases := []struct {
		ip      string
		blocked bool
	}{
		{"127.0.0.1", true},             // loopback
		{"::1", true},                   // loopback v6
		{"0.0.0.0", true},               // unspecified
		{"169.254.169.254", true},       // cloud metadata (link-local)
		{"10.0.0.5", true},              // private
		{"172.16.3.4", true},            // private
		{"192.168.1.1", true},           // private
		{"100.64.0.1", true},            // carrier-grade NAT (RFC 6598)
		{"fc00::1", true},               // unique-local v6
		{"fe80::1", true},               // link-local v6
		{"8.8.8.8", false},              // public
		{"1.1.1.1", false},              // public
		{"2606:4700:4700::1111", false}, // public v6
	}
	for _, c := range cases {
		ip := net.ParseIP(c.ip)
		if ip == nil {
			t.Fatalf("bad test ip %q", c.ip)
		}
		if got := isBlockedIP(ip); got != c.blocked {
			t.Errorf("isBlockedIP(%s) = %v, want %v", c.ip, got, c.blocked)
		}
	}
}

func TestWebFetchRejectsBadScheme(t *testing.T) {
	tool := NewWebFetchTool()
	if _, err := tool.Call(t.Context(), []byte(`{"url":"file:///etc/passwd"}`)); err == nil {
		t.Fatal("expected error for file:// scheme")
	}
}
