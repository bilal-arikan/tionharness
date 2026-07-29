package agent

import (
	"strings"
	"testing"
)

// TestParseSqzStats covers every stderr shape sqz actually emits. The two success
// modes look nothing alike, and treating a dedup hit as "no optimization" would
// hide the chip on exactly the card that most needs one (a full result replaced
// by a single §ref:…§ line).
func TestParseSqzStats(t *testing.T) {
	tests := []struct {
		name      string
		stderr    string
		wantNil   bool
		wantDedup bool
		wantIn    int
		wantOut   int
		wantPct   int
	}{
		{
			name:    "measured compression",
			stderr:  "[sqz] 57/841 tokens (93% reduction) [cargo test]\n[sqz] n-gram abbreviation: 21 tokens saved\n",
			wantIn:  841,
			wantOut: 57,
			wantPct: 93,
		},
		{
			name:      "dedup hit reports no token pair",
			stderr:    "[sqz] dedup hit: §ref:a8856afd1df33ecc§ (L2)\n",
			wantDedup: true,
		},
		{
			name:    "no sqz line at all (passthrough)",
			stderr:  "",
			wantNil: true,
		},
		{
			// sqz self-gates and returns precise/short output verbatim. Claiming a
			// "−0%" saving on an untouched result would misrepresent what the agent read.
			name:    "no actual saving",
			stderr:  "[sqz] 841/841 tokens (0% reduction) [git rev-parse HEAD]\n",
			wantNil: true,
		},
		{
			name:    "output larger than input",
			stderr:  "[sqz] 900/841 tokens [cat x]\n",
			wantNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseSqzStats(tc.stderr)
			if tc.wantNil {
				if got != nil {
					t.Fatalf("expected no optimization, got %+v", *got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected an optimization, got nil")
			}
			if got.Kind != "sqz" {
				t.Errorf("Kind = %q, want \"sqz\"", got.Kind)
			}
			if got.Dedup != tc.wantDedup {
				t.Errorf("Dedup = %v, want %v", got.Dedup, tc.wantDedup)
			}
			if got.InTokens != tc.wantIn || got.OutTokens != tc.wantOut {
				t.Errorf("tokens = %d/%d, want %d/%d", got.OutTokens, got.InTokens, tc.wantOut, tc.wantIn)
			}
			if got.Percent() != tc.wantPct {
				t.Errorf("Percent() = %d, want %d", got.Percent(), tc.wantPct)
			}
		})
	}
}

// TestShellEnvironmentGuidance locks the ONE fact each flavour must state — the
// mount prefix. Getting it backwards is worse than saying nothing: the agent
// would confidently use a path that cannot exist.
func TestShellEnvironmentGuidance(t *testing.T) {
	gitbash := shellEnvironmentGuidance("gitbash")
	if !strings.Contains(gitbash, "/c/") || !strings.Contains(gitbash, "NO `/mnt/c`") {
		t.Errorf("git-bash block must point at /c/ and rule out /mnt/c, got:\n%s", gitbash)
	}
	wsl := shellEnvironmentGuidance("wsl")
	if !strings.Contains(wsl, "/mnt/c/") {
		t.Errorf("WSL block must point at /mnt/c/, got:\n%s", wsl)
	}
	// Native Unix needs no block — the model already assumes that layout, so an
	// extra section would be pure token cost on every turn.
	if shellEnvironmentGuidance("unix") != "" || shellEnvironmentGuidance("") != "" {
		t.Error("native unix / no-shell must produce no prompt block")
	}
}
