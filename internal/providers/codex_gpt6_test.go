package providers

import (
	"slices"
	"strings"
	"testing"
)

func TestCodexGPT6CatalogAndWire(t *testing.T) {
	for _, model := range []string{"gpt-6-astra", "gpt-6-sol", "gpt-6-luna"} {
		t.Run(model, func(t *testing.T) {
			var found *ModelInfo
			for _, entry := range Catalog() {
				if entry.ID != "codex-cli" {
					continue
				}
				for _, m := range entry.Models {
					if m.ID == model {
						found = &m
						break
					}
				}
			}
			if found == nil {
				t.Fatalf("%s missing from Codex catalog", model)
			}
			want := []string{"low", "medium", "high", "xhigh", "max"}
			if model != "gpt-6-luna" {
				want = append(want, "ultra")
			}
			if !slices.Equal(found.ThinkingTiers, want) {
				t.Errorf("tiers = %v, want %v", found.ThinkingTiers, want)
			}
			if found.ContextWindow != 272_000 {
				t.Errorf("CLI context = %d, want 272000", found.ContextWindow)
			}
			if found.ThinkingClass != "adaptive" {
				t.Errorf("class = %s", found.ThinkingClass)
			}
			cli := NewCodexCLI("codex", "", t.TempDir())
			for _, effort := range want {
				req := Request{Model: model, CLIEffortLevel: effort, PermissionMode: "read-only"}
				if err := ValidateThinkingLevelForProvider("codex-cli", model, effort); err != nil {
					t.Fatal(err)
				}
				args := cli.buildArgs(req, model)
				i := slices.Index(args, "-m")
				if i < 0 || i+1 >= len(args) || args[i+1] != model {
					t.Fatalf("model not forwarded: %v", args)
				}
				config := renderCodexConfig(cli.buildConfig(req))
				if !strings.Contains(config, `model_reasoning_effort = "`+effort+`"`) {
					t.Fatalf("effort %s not forwarded", effort)
				}
			}
			if err := ValidateThinkingLevelForProvider("codex-cli", model, "off"); err == nil {
				t.Error("unsupported CLI off level accepted")
			}
		})
	}
	if err := ValidateThinkingLevelForProvider("codex-cli", "gpt-6-luna", "ultra"); err == nil {
		t.Error("Luna must reject ultra")
	}
}

func TestGPT6TransportWindows(t *testing.T) {
	for _, model := range []string{"gpt-6-astra", "gpt-6-sol", "gpt-6-luna"} {
		for _, alias := range []string{model, " " + strings.ToUpper(model) + " "} {
			if got := ContextWindowFor("codex-cli", alias); got != 272_000 {
				t.Errorf("CLI %q: %d", alias, got)
			}
			if got := ContextWindowFor("openai", alias); got != windowGPTLarge {
				t.Errorf("API %q: %d", alias, got)
			}
			if got := MaxOutputFor("codex-cli", alias); got != maxOutGPT {
				t.Errorf("output %q: %d", alias, got)
			}
		}
	}
	if got := ContextWindowFor("openai", "gpt-5.6-luna"); got != windowGPTLuna {
		t.Errorf("legacy Luna window changed: %d", got)
	}
}

func TestGPT6EquivalentAPICost(t *testing.T) {
	for _, tc := range []struct {
		model         string
		input, output float64
	}{
		{"gpt-6-astra", 10, 50}, {"gpt-6-sol", 2, 10}, {"gpt-6-luna", 0.1, 0.5},
	} {
		t.Run(tc.model, func(t *testing.T) {
			p, ok := EstimateFor("codex-cli", tc.model)
			if !ok {
				t.Fatal("missing equivalent API estimate")
			}
			if p.InputPerMTok != tc.input || p.OutputPerMTok != tc.output {
				t.Fatalf("wrong rates: %+v", p)
			}
			if p.CacheReadMultOverride != 0.1 || p.CacheWriteMultOverride != 1.25 {
				t.Fatalf("wrong cache rates: %+v", p)
			}
			if _, billed := PriceFor("codex-cli", tc.model); billed {
				t.Error("subscription must not become API billing")
			}
			const in, out, read, write = 200_000, 1000, 80_000, 10_000
			want := (float64(in)*tc.input*2 + float64(read)*tc.input*0.1*2 + float64(write)*tc.input*1.25*2 + float64(out)*tc.output*1.5) / 1_000_000
			if got := p.CostDetailed(in, out, read, write); !approx(got, want) {
				t.Errorf("long context cost = %v, want %v", got, want)
			}
		})
	}
}
