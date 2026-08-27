// Package prompts is the CENTRAL REGISTRY for every embedded LLM prompt in the
// runtime (_Docs/61). Each prompt is one key with an embedded default (a .md
// file under defaults/, //go:embed'ed into the binary) plus metadata: a Turkish
// UI label/hint, the named {{placeholder}} slots an override MUST preserve, and
// whether the prompt rides the cached static system prefix (epoch-affecting).
//
// Resolution order everywhere is: workspace override file
// (<workspace>/config/prompts/<key>.md) → embedded default. An override that is
// blank or drops a required placeholder falls back to the default, so a bad
// edit can never break a turn (the same guarantee the old compact-%s guard
// gave, generalized). The agent package's Runtime.readPrompt and the api layer
// both resolve through ResolveFile.
//
// This package is a LEAF: it imports nothing from internal/, so agent,
// conversation and api can all depend on it without cycles.
package prompts

import (
	"embed"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

//go:embed defaults/*.md
var defaultsFS embed.FS

// Spec describes one registered prompt: identity, UI metadata, and the named
// placeholders a workspace override must keep for the runtime to fill.
type Spec struct {
	Key              string   `json:"key"`
	Label            string   `json:"label"`                      // Turkish UI label
	Hint             string   `json:"hint"`                       // Turkish UI hint (where it is used, what to preserve)
	Placeholders     []string `json:"placeholders"`               // required {{name}} slots; empty = free-form text
	EpochAffecting   bool     `json:"epochAffecting"`             // true → enters the cached static prefix; edits apply to NEW sessions/epochs
	OwnedBySystemKey string   `json:"ownedBySystemKey,omitempty"` // system agent whose Soul supplies the effective prompt when resolution succeeds
}

// specs is the registry, in display order. Adding a prompt = one entry here +
// one defaults/<key>.md file; prompts_test.go enforces they stay in sync.
var specs = []Spec{
	{
		Key:              "summary",
		Label:            "Genel bakış promptu",
		Hint:             "Yalnızca /board · /flows komutlarının anlık genel-bakış sistem promptu (kısa liste özeti). Konuşma özetlemesi (compaction) DEĞİL.",
		OwnedBySystemKey: "overview-summarizer",
	},
	{
		Key:              "title",
		Label:            "Başlık promptu",
		Hint:             "Otomatik başlık üretimi sistem promptu (sohbet ilk turu + görev oluşturma).",
		OwnedBySystemKey: "titler",
	},
	{
		Key:              "compact",
		Label:            "Compaction promptu",
		Hint:             "Bağlam sınırına yaklaşınca geçmişi tek bir yapılandırılmış özete katlayan prompt. {{summary}} (mevcut özet) ve {{messages}} (yeni mesajlar) yer tutucuları KORUNMALI — bozarsan gömülü varsayılana düşer.",
		Placeholders:     []string{"summary", "messages"},
		OwnedBySystemKey: "compaction",
	},
	{
		Key:          "handoff",
		Label:        "Handoff promptu",
		Hint:         "Context reset öncesi devir dokümanını üreten prompt. {{summary}}, {{transcript}}, {{environment}} yer tutucuları korunmalı.",
		Placeholders: []string{"summary", "transcript", "environment"},
	},
	{
		Key:          "continuation",
		Label:        "Devam (continuation) promptu",
		Hint:         "Handoff sonrası taze oturumun açılış kullanıcı mesajı. {{handoff}}, {{oldSession}}, {{artifact}} korunmalı; {{fileNote}} opsiyoneldir (dosya yazıldıysa dolar).",
		Placeholders: []string{"handoff", "oldSession", "artifact"},
	},
	{
		Key:   "btw-system",
		Label: "Yan soru (btw) sistem promptu",
		Hint:  "Araçsız, transkript-dışı yan soru danışmanının kuralları. Araçsızlık yapısal olarak da zorlanır (prompt tek güvence değildir).",
	},
	{
		Key:   "btw-preamble",
		Label: "Yan soru (btw) kullanıcı önsözü",
		Hint:  "Yan sorunun kullanıcı mesajının başına eklenen sözleşme tekrarı (agentic CLI sağlayıcıları için).",
	},
	{
		Key:              "lesson",
		Label:            "Ders çıkarma (reflection) promptu",
		Hint:             "Kötü biten turdan tek, genellenebilir ders damıtan arka plan reflection çağrısının sistem promptu.",
		OwnedBySystemKey: "lesson-extractor",
	},
	{
		Key:              "insight-analyzer",
		Label:            "İçgörü analiz promptu",
		Hint:             "Retrospektif taramada bir lens talimatını oturum kanıtına uygulayan analizörün sistem promptu.",
		OwnedBySystemKey: "insight",
	},
	{
		Key:   "auto-continue",
		Label: "Otomatik devam dürtmesi",
		Hint:  "Otonom turda iş yarım kaldığında oturuma kullanıcı mesajı olarak yazılan devam talimatı.",
	},
	{
		Key:            "coordinator",
		Label:          "Koordinatör el kitabı",
		Hint:           "Koordinatör oturumlarının statik prefix'ine eklenen işletme talimatı. Değişiklik YENİ oturum/epoch'larda etkili olur.",
		EpochAffecting: true,
	},
	{
		Key:              "subagent-explore",
		Label:            "Subagent: explore",
		Hint:             "run_subagent 'explore' profilinin (salt-okunur araştırmacı) sistem promptu.",
		EpochAffecting:   true,
		OwnedBySystemKey: "subagent-explore",
	},
	{
		Key:              "subagent-planner",
		Label:            "Subagent: planner",
		Hint:             "run_subagent/spawn_worker 'planner' profilinin (kodu okuyup uygulanabilir plan üreten, düzenleme yapmayan) sistem promptu.",
		EpochAffecting:   true,
		OwnedBySystemKey: "subagent-planner",
	},
	{
		Key:              "subagent-coder",
		Label:            "Subagent: coder",
		Hint:             "run_subagent 'coder' profilinin (kod yazan/düzenleyen) sistem promptu.",
		EpochAffecting:   true,
		OwnedBySystemKey: "subagent-coder",
	},
	{
		Key:              "subagent-reviewer",
		Label:            "Subagent: reviewer",
		Hint:             "run_subagent 'reviewer' profilinin (salt-okunur eleştirmen) sistem promptu.",
		EpochAffecting:   true,
		OwnedBySystemKey: "subagent-reviewer",
	},
	{
		Key:              "subagent-validator",
		Label:            "Subagent: validator",
		Hint:             "run_subagent/spawn_worker 'validator' profilinin (kodu düzenlemeden test/typecheck/build/e2e ile doğrulayan, kompakt PASS/FAIL verdict dönen) sistem promptu.",
		EpochAffecting:   true,
		OwnedBySystemKey: "subagent-validator",
	},
	{
		Key:              "subagent-config",
		Label:            "Subagent: config",
		Hint:             "run_subagent 'config' profilinin (config/ editörü) sistem promptu.",
		EpochAffecting:   true,
		OwnedBySystemKey: "subagent-config",
	},
	{
		Key:            "terse",
		Label:          "Terse (caveman) yanıt stili",
		Hint:           "Workspace ▸ Genel ▸ 'Terse mod' anahtarı AÇIKKEN her ajanın statik system prefix'ine eklenen yanıt-stili talimatı. Kapalıyken hiç gönderilmez. Statik prefix'te olduğu için prompt-cache penceresi başına bir kez ödenir; düzenlemesi YENİ oturum/epoch'ta etkili olur.",
		EpochAffecting: true,
	},
}

// defaults maps key → embedded default text, loaded once at init. A missing
// file panics at startup (a build-time invariant, also locked by tests).
var defaults = func() map[string]string {
	m := make(map[string]string, len(specs))
	for _, s := range specs {
		data, err := defaultsFS.ReadFile("defaults/" + s.Key + ".md")
		if err != nil {
			panic(fmt.Sprintf("prompts: embedded default missing for key %q: %v", s.Key, err))
		}
		m[s.Key] = strings.TrimSpace(string(data))
	}
	return m
}()

// Specs returns the registry in display order (copy — callers cannot mutate it).
func Specs() []Spec {
	out := make([]Spec, len(specs))
	copy(out, specs)
	return out
}

// Keys returns every registered prompt key in display order.
func Keys() []string {
	out := make([]string, len(specs))
	for i, s := range specs {
		out[i] = s.Key
	}
	return out
}

// Get returns the Spec for key.
func Get(key string) (Spec, bool) {
	for _, s := range specs {
		if s.Key == key {
			return s, true
		}
	}
	return Spec{}, false
}

// Default returns the embedded default text for key ("" for an unknown key).
func Default(key string) string { return defaults[key] }

// SourceDir returns the absolute directory holding the embedded default .md
// sources, as recorded at build time via runtime.Caller. On a locally-built
// binary this resolves to the repository's internal/prompts/defaults folder, so
// the UI's "open folder" button lands on the actual sources.
func SourceDir() string {
	if _, file, _, ok := runtime.Caller(0); ok {
		return filepath.Join(filepath.Dir(file), "defaults")
	}
	return ""
}
