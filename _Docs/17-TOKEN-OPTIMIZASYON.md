# 17 — Araç Çıktısı Token Optimizasyonu

> Ajan araç çıktılarının (shell, dosya, MCP) LLM context'ine girmeden önce küçültülmesi.
> **İki bağımsız sistem**, paralel veya tek başına çalışabilir; her biri Ayarlar'dan ayrı konfigüre edilir.

## Neden?

SwarmGo'nun native agentic döngüsünde (`agent/toolloop.go`) her araç çağrısının çıktısı bir
`ToolResult` olarak konuşmaya eklenir ve sonraki model çağrısında **girdi token'ı** olarak ücretlenir.
`shell` gibi araçlar 64 KB'ye kadar ham çıktı döndürebilir. `git status`, test runner, `ls -R`, `grep`
gibi komutlar context'i hızla şişirir. Bu katman, çıktı transcript'e *girmeden önce* onu kırpar — mevcut
`internal/conversation` compaction'ı (transcript bütçesi) ve prompt-cache'i tamamlar.

## İki Sistem

| | **Sistem A — Deterministik** | **Sistem B — LLM Özeti** |
|---|---|---|
| Paket | `internal/tools/compact` | `internal/agent/compactor.go` |
| İlham | `rtk-ai/rtk` (Rust Token Killer) | `external-agent-oss` (Large Response Handling) |
| Yöntem | Kural tabanlı: dedupe + boş-satır sadeleştirme + ortadan kırpma | Ucuz modelle niyet-farkında özet |
| Maliyet | Sıfır (yerel) | Ekstra model çağrısı (`KindCompact`) |
| Tetik | Her başarılı araç çıktısı | Yalnız (A sonrası) eşik üstü çıktı |
| Varsayılan | **Açık** | **Kapalı** (opt-in, maliyetli) |

İkisi de açıksa **ardışık**: önce ücretsiz A, kalan hâlâ eşik üstündeyse B. Biri açıksa yalnız o çalışır.
İkisi de kapalıysa eski davranış (sadece tool'un kendi 64 KB hard-cap'i) korunur.

```mermaid
graph TD
    A["Tool çıktısı (res.Content)"] --> E{"IsError / boş?"}
    E -->|evet| OUT["dokunulmaz → context"]
    E -->|hayır| CA{"Sistem A açık?"}
    CA -->|evet| RA["compact.Compact:<br/>dedupe / boş-satır / ortadan kırp"]
    CA -->|hayır| SB
    RA --> SB{"Sistem B açık<br/>& boyut > eşik?"}
    SB -->|evet| LB["summarizeToolOutput<br/>(cheap model, intent-aware)"]
    SB -->|hayır| OUT2["context'e + persisted step"]
    LB -->|başarı| OUT2
    LB -->|hata/boş| OUT2
    style RA fill:#2d6,stroke:#093
    style LB fill:#69d,stroke:#036
```

## Sistem A — `internal/tools/compact`

`Compact(output string, opts Options) (string, Stats)` — dep-siz, hata döndürmez (en kötü ihtimalle
orijinali verir).

- **Dedupe:** Ardışık aynı satırlar `satır  (×N)` olarak birleşir (trailing whitespace yok sayılır).
- **Boş satır blokları:** 2+ ardışık boş satır → tek boş satır.
- **Satır eleme:** `MaxLines` aşılırsa baş + son yarı korunur, ortaya `… N satır atlandı …` işareti.
- **Bayt kırpma:** `MaxBytes` aşılırsa baş (2/3) + son (1/3) korunarak ortaya `… [çıktı N bayt kırpıldı] …`;
  kesim **rune sınırında** yapılır (UTF-8 bozulmaz — Türkçe karakterler güvenli).
- `Stats{BeforeBytes, AfterBytes, Applied}` + `Saved()` — log ve tasarruf ölçümü.

## Sistem B — `agent/compactor.go`

- `compactToolResult(ctx, agent, toolName, input, res)` — A'yı uygular, sonra B eşiğini kontrol eder.
  `IsError` veya boş sonuçlar **hiç dokunulmadan** geçer.
- `summarizeToolOutput(...)` — `guardedComplete` + `WithCallKind(ctx, KindCompact)`. Model: başlık-modeli
  override → yoksa ajan modeli. **Niyet** = tool adı + (varsa) input özeti (`intentInputRunes=300`).
  Sistem prompt: olguları (yol/kimlik/hata/sayı/sonuç) koru, uydurma yapma, sadece sonucu döndür.
- Bütçeyi **gate'lemez** (autonomous=false) — titler/summary/reflect ile aynı politika; usage yine işlenir.
- Yalnız **native döngüde** (Anthropic/MiniMax) etkilidir; claude-cli delegasyonu kendi döngüsünü sürdürür
  (çıktıları SwarmGo'nun `ToolResult` katmanından geçmez).

## Ayarlar

`settings.Settings` / `DTO` / `Patch` (clamp'ler `store.go::validate`'de):

| Alan | Sistem | Vars. | Clamp |
|------|--------|-------|-------|
| `compactToolOutput` | A aç/kapa | `true` | — |
| `compactMaxLines` | A satır sınırı | `200` | 0 (=default) – 5000 |
| `compactMaxBytes` | A bayt sınırı | `12288` | 0 (=default) – 262144 |
| `compactLlmSummary` | B aç/kapa | `false` | — |
| `compactLlmThreshold` | B eşik (bayt) | `8192` | 0 (=default) – 262144 |

Canlı push: `api/server.go::applySettings` → `Tunables.SetToolCompaction(...)`. 0 değerleri Tunables
getter'larında built-in default'a (`DefaultCompact*`) çevrilir. UI: **Ayarlar → Bağlam** içinde iki
ayrı bölüm (`frontend/.../settings/appPanels.tsx` `ContextPanel`).

## Sınırlar / Notlar

- Sıkıştırma hem modele giden `ToolResult`'a **hem de** UI'da gösterilen/persist edilen `TurnStep.Output`'a
  uygulanır → kullanıcı, modelin gördüğü çıktıyı görür (tutarlılık).
- Komut-özel akıllı kısaltıcılar (git/test/grep'e özgü) henüz yok; A jeneriktir. Gelecek iş.
- claude-cli delegasyon yolu kapsam dışıdır (yukarıdaki sebep).
