package tools

import (
	"encoding/json"
	"testing"
)

// TestFlexBool covers the shapes a model actually emits for a boolean tool
// argument. The quoted form is not hypothetical: on 2026-07-31 an agent was told
// by the optimizer note to re-run a failed command with `no_compress: true`, sent
// `"true"`, and had the call refused with "cannot unmarshal string into Go struct
// field shellArgs.no_compress of type bool" — the recovery path breaking at the
// one moment it mattered.
func TestFlexBool(t *testing.T) {
	truthy := []string{`true`, `"true"`, `"True"`, `"TRUE"`, `"1"`, `"yes"`, `"on"`, `1`}
	for _, in := range truthy {
		var b flexBool
		if err := json.Unmarshal([]byte(in), &b); err != nil {
			t.Errorf("%s must parse, got %v", in, err)
		} else if !bool(b) {
			t.Errorf("%s must be true", in)
		}
	}

	falsy := []string{`false`, `"false"`, `"False"`, `"0"`, `"no"`, `"off"`, `""`, `0`, `null`}
	for _, in := range falsy {
		var b flexBool
		if err := json.Unmarshal([]byte(in), &b); err != nil {
			t.Errorf("%s must parse, got %v", in, err)
		} else if bool(b) {
			t.Errorf("%s must be false", in)
		}
	}

	// Ambiguity stays an ERROR. Reading "maybe" as false would swap a visible
	// failure for an invisible one — the agent would believe it had opted out of
	// compression while still receiving the compressed output.
	for _, in := range []string{`"maybe"`, `"YES please"`, `2`, `[]`, `{}`} {
		var b flexBool
		if err := json.Unmarshal([]byte(in), &b); err == nil {
			t.Errorf("%s must be rejected, got %v", in, bool(b))
		}
	}
}

// TestShellArgsFlexBool pins the end-to-end decode: the tool's own argument struct
// must accept the quoted form, since that is where the failure actually happened.
func TestShellArgsFlexBool(t *testing.T) {
	var a shellArgs
	if err := json.Unmarshal([]byte(`{"command":"ls","no_compress":"true","run_in_background":"false"}`), &a); err != nil {
		t.Fatalf("quoted booleans must decode, got %v", err)
	}
	if !bool(a.NoCompress) {
		t.Error(`no_compress:"true" must opt out of compression`)
	}
	if bool(a.RunInBackground) {
		t.Error(`run_in_background:"false" must stay false`)
	}
	if a.Command != "ls" {
		t.Errorf("command mangled: %q", a.Command)
	}
}
