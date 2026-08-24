package billing

// TSK67 PoC — "Agent modeli session tamamlandıktan sonra değişirse cache & bütçe
// hesabı nasıl doğru kalır?"
//
// Bu dosya, mevcut tasarımın değişmez (invariant) kabulünü dışarıdan kanıtlar ve
// tasarım raporundaki eksikleri (Gap 3: bilinmeyen model → fiyatlandırılmamış
// kayıt) gözlemlenebilir kılar. Üretim koduna dokunmaz; yalnızca testtir.
//
// Kanıtlanan invariant:
//
//	Usage / SessionUsage kayıtları "<provider>|<model>" anahtarıyla, çağrıyı
//	GERÇEKTEN servis eden modelin adıyla tutulur (RecordUsage → resp.Model, boşsa
//	agent.Model fallback). Maliyet OKUMA anında, saklanan bu anahtara göre
//	hesaplanır (PriceStat). Dolayısıyla UpdateAgent ile agent.Model değişince
//	geçmiş kayıtlar NE yeniden anahtarlanır NE de yeniden fiyatlanır; yalnızca
//	yeni çağrılar yeni modelin anahtarına düşer.

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func strPtr(s string) *string { return &s }

// TestModelChangeDoesNotRepricePastUsage, bütçe tarafının doğru davrandığını uçtan
// uca gösterir: model değişimi ne günlük usage'ı ne de session usage'ı geriye
// dönük değiştirir; iki modelin kayıtları ByModel içinde ayrık ve doğru fiyatlı
// durur.
func TestModelChangeDoesNotRepricePastUsage(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agent, err := d.CreateAgent(ctx, db.Agent{
		Name: "poc", Provider: "anthropic", Model: "claude-opus-4-8",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Turn 1 — eski model (opus). Agent döngüsü RecordUsage'da resp.Model'i
	// yazar; burada aynısını birebir taklit ediyoruz.
	opusDelta := db.UsageDelta{Calls: 1, InputTokens: 1000, OutputTokens: 500, CacheReadTokens: 8000, CacheWriteTokens: 12000}
	if err := d.AddUsageKind(ctx, agent.ID, db.UsageKindChat, "anthropic", "claude-opus-4-8", opusDelta); err != nil {
		t.Fatalf("add usage: %v", err)
	}
	if err := d.AddSessionUsageKind(ctx, "session-1", agent.ID, db.UsageKindChat, "anthropic", "claude-opus-4-8", opusDelta); err != nil {
		t.Fatalf("add session usage: %v", err)
	}

	before, err := d.GetUsageToday(ctx, agent.ID)
	if err != nil {
		t.Fatalf("get usage: %v", err)
	}
	opusKey := db.ModelKey("anthropic", "claude-opus-4-8")
	opusStat, ok := before.ByModel[opusKey]
	if !ok {
		t.Fatalf("opus key %q missing before change", opusKey)
	}
	costBefore, saveBefore, pricedBefore, _ := PriceStat("anthropic", "claude-opus-4-8", opusStat)
	if !pricedBefore {
		t.Fatal("opus must be priced (real list price exists)")
	}

	// Model değişimi — update_agent'ın yaptığı tek şey bu patch'tir.
	newModel := "claude-sonnet-4-6"
	if _, err := d.UpdateAgent(ctx, agent.ID, db.AgentProfilePatch{Model: strPtr(newModel)}); err != nil {
		t.Fatalf("update agent: %v", err)
	}

	// Geçmiş usage satırına dokunulmamış olmalı: aynı anahtar, aynı sayaçlar,
	// aynı fiyat.
	after, err := d.GetUsageToday(ctx, agent.ID)
	if err != nil {
		t.Fatalf("get usage after: %v", err)
	}
	afterOpus, ok := after.ByModel[opusKey]
	if !ok {
		t.Fatal("past opus bucket disappeared after model change")
	}
	if afterOpus != opusStat {
		t.Fatalf("past opus bucket mutated by model change: %+v → %+v", opusStat, afterOpus)
	}
	costAfter, _, _, _ := PriceStat("anthropic", "claude-opus-4-8", afterOpus)
	if costAfter != costBefore {
		t.Fatalf("past cost repriced: before=%v after=%v", costBefore, costAfter)
	}
	_, saveAfter, _, _ := PriceStat("anthropic", "claude-opus-4-8", afterOpus)
	if saveAfter != saveBefore {
		t.Fatalf("past cache savings repriced: before=%v after=%v", saveBefore, saveAfter)
	}
	// Henüz yeni modelde çağrı olmadığı için yeni anahtar var olmamalı.
	if _, ok := after.ByModel[db.ModelKey("anthropic", newModel)]; ok {
		t.Fatal("new model key appeared before any call ran on it")
	}

	// Turn 2 — yeni model. Bir sonraki çağrı yeni anahtara düşer.
	if err := d.AddUsageKind(ctx, agent.ID, db.UsageKindChat, "anthropic", newModel, db.UsageDelta{Calls: 1, InputTokens: 100, OutputTokens: 50}); err != nil {
		t.Fatalf("add usage new model: %v", err)
	}
	final, err := d.GetUsageToday(ctx, agent.ID)
	if err != nil {
		t.Fatalf("get usage final: %v", err)
	}
	if got := final.ByModel[opusKey]; got != opusStat {
		t.Fatalf("opus bucket changed after new-model call: %+v", got)
	}
	sonnetStat, ok := final.ByModel[db.ModelKey("anthropic", newModel)]
	if !ok || sonnetStat.InputTokens != 100 || sonnetStat.Calls != 1 {
		t.Fatalf("new-model bucket wrong: %+v", sonnetStat)
	}
	// İki modelin fiyatları farklı olmalı (opus 5$/MTok, sonnet 3$/MTok input).
	opusCost, _, _, _ := PriceStat("anthropic", "claude-opus-4-8", final.ByModel[opusKey])
	sonnetCost, _, _, _ := PriceStat("anthropic", newModel, sonnetStat)
	if sonnetCost >= opusCost {
		t.Fatalf("expected sonnet cheaper than opus per token, opus=%v sonnet=%v", opusCost, sonnetCost)
	}

	// Session usage tarafı da aynı ayrımı korur.
	su, err := d.GetSessionUsage(ctx, "session-1")
	if err != nil {
		t.Fatalf("get session usage: %v", err)
	}
	expectedOpusStat := db.KindStat{Calls: 1, InputTokens: 1000, OutputTokens: 500, CacheReadTokens: 8000, CacheWriteTokens: 12000}
	if suStat, ok := su.ByModel[opusKey]; !ok || suStat != expectedOpusStat {
		t.Fatalf("session usage opus bucket lost: %+v (ok=%v)", suStat, ok)
	}
}

// TestUnknownModelIsFlaggedUnpriced, Gap 3'ü gösterir: update_agent bilinmeyen bir
// model adını sessizce kabul eder (ne API ne de tool fiyat tablosunu doğrular) ve
// o modeldeki her çağrı bütçe ekranında "0 USD, fiyatlandırılmamış" görünür —
// yanlışlıkla ücretsiz sanılabilir. Bu PoC, tasarım önerisindeki "update anında
// uyar" gereksinimini gerekçelendirir.
func TestUnknownModelIsFlaggedUnpriced(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agent, err := d.CreateAgent(ctx, db.Agent{Name: "poc2", Provider: "anthropic", Model: "claude-mega-9"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	// Turn — çağrıyı gerçekten bu (bilinmeyen) model servis etti.
	if err := d.AddUsageKind(ctx, agent.ID, db.UsageKindChat, "anthropic", "claude-mega-9", db.UsageDelta{Calls: 1, InputTokens: 10000, OutputTokens: 5000}); err != nil {
		t.Fatalf("add usage: %v", err)
	}
	u, err := d.GetUsageToday(ctx, agent.ID)
	if err != nil {
		t.Fatalf("get usage: %v", err)
	}
	cost, _, priced, estimated := PriceStat("anthropic", "claude-mega-9", u.ByModel[db.ModelKey("anthropic", "claude-mega-9")])
	if priced || estimated {
		t.Fatalf("unknown model must be unpriced & not estimated, got priced=%v estimated=%v", priced, estimated)
	}
	if cost != 0 {
		t.Fatalf("unknown model cost must be 0, got %v", cost)
	}
}
