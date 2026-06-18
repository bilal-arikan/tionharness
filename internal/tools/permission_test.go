package tools

import "testing"

func TestNormalizePermission(t *testing.T) {
	cases := map[string]string{
		PermAllowOnce:        "allow",  // "İzin ver" — Turkish dotted-İ must still match
		PermAllowAlways:      "always", // "Her zaman izin ver" — always wins over izin
		PermDeny:             "deny",
		"allow":              "allow",
		"Allow once":         "allow",
		"yes":                "allow",
		"evet":               "allow",
		"always":             "always",
		"reddet":             "deny",
		"no":                 "deny",
		"":                   "deny",
		"something unrelated": "deny",
	}
	for in, want := range cases {
		if got := NormalizePermission(in); got != want {
			t.Errorf("NormalizePermission(%q) = %q, want %q", in, got, want)
		}
	}
}
