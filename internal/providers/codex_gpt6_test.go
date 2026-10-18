package providers

import (
	"slices"
	"strings"
	"testing"
)

func TestCodexGPT6CatalogAndWire(t *testing.T) {
	for _, model := range []string{"gpt-6.1-sol", "gpt-6-astra", "gpt-6-sol", "gpt-6-luna"} {
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
			if want := codexCLIContextWindow(model); found.ContextWindow != want {
				t.Errorf("CLI context = %d, want %d", found.ContextWindow, want)
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
	for _, model := range []string{"gpt-6.1-sol", "gpt-6-astra", "gpt-6-sol", "gpt-6-luna"} {
		for _, alias := range []string{model, " " + strings.ToUpper(model) + " "} {
			if got := ContextWindowFor("codex-cli", alias); got != codexCLIContextWindow(model) {
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

// TestCodexCLIWindowsFollowCodexCatalog pins the windows Codex itself serves
// (models_cache.json context_window): only gpt-6.1-sol is 500K, and the API's
// 1.05M Sol/Terra windows must never leak into the codex-cli budget.
func TestCodexCLIWindowsFollowCodexCatalog(t *testing.T) {
	for model, want := range map[string]int{
		"gpt-6.1-sol":   500_000,
		"gpt6.1-sol":    500_000,
		"gpt-6-sol":     272_000,
		"gpt-6-astra":   272_000,
		"gpt-6-luna":    272_000,
		"gpt-5.6-sol":   272_000,
		"gpt-5.6-terra": 272_000,
		"gpt-5.6-luna":  272_000,
		"gpt-reserve":   0, // not a GPT-5/6 slug: unknown, the conservative fallback applies
	} {
		if got := ContextWindowFor("codex-cli", model); got != want {
			t.Errorf("codex-cli %s window = %d, want %d", model, got, want)
		}
	}
	if got := ContextWindowFor("openai", "gpt-5.6-sol"); got != windowGPTLarge {
		t.Errorf("API gpt-5.6-sol window = %d, want %d", got, windowGPTLarge)
	}
}

func TestGPT6EquivalentAPICost(t *testing.T) {
	for _, tc := range []struct {
		model         string
		input, output float64
		cacheRead     float64
	}{
		{"gpt-6.1-sol", 2, 10, 0.05},
		{"gpt-6-astra", 10, 50, 0.1}, {"gpt-6-sol", 2, 10, 0.1}, {"gpt-6-luna", 0.1, 0.5, 0.1},
	} {
		t.Run(tc.model, func(t *testing.T) {
			p, ok := EstimateFor("codex-cli", tc.model)
			if !ok {
				t.Fatal("missing equivalent API estimate")
			}
			if p.InputPerMTok != tc.input || p.OutputPerMTok != tc.output {
				t.Fatalf("wrong rates: %+v", p)
			}
			if p.CacheReadMultOverride != tc.cacheRead || p.CacheWriteMultOverride != 1.25 {
				t.Fatalf("wrong cache rates: %+v", p)
			}
			if _, billed := PriceFor("codex-cli", tc.model); billed {
				t.Error("subscription must not become API billing")
			}
			const in, out, read, write = 200_000, 1000, 80_000, 10_000
			want := (float64(in)*tc.input*2 + float64(read)*tc.input*tc.cacheRead*2 + float64(write)*tc.input*1.25*2 + float64(out)*tc.output*1.5) / 1_000_000
			if got := p.CostDetailed(in, out, read, write); !approx(got, want) {
				t.Errorf("long context cost = %v, want %v", got, want)
			}
		})
	}
}
