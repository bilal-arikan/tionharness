package api

import "testing"

func TestStrictUnknownField(t *testing.T) {
	cases := []struct {
		msg      string
		wantOK   bool
		wantName string
	}{
		{`json: unknown field "workingDir"`, true, "workingDir"},
		{`json: unknown field "foo"`, true, "foo"},
		{`json: cannot unmarshal string into Go value of type int`, false, ""},
		{``, false, ""},
	}
	for _, tc := range cases {
		name, ok := strictUnknownField(tc.msg)
		if ok != tc.wantOK || name != tc.wantName {
			t.Errorf("strictUnknownField(%q) = (%q, %v), want (%q, %v)", tc.msg, name, ok, tc.wantName, tc.wantOK)
		}
	}
}
