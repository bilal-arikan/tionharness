# 61 — Merkezi Prompt Registry

> **Durum:** Tamamlandı (2026-07-15). Uygulamaya dağınık gömülü LLM promptları tek
> kayıt defterinde (`internal/prompts`) toplandı; tümü workspace-başına
> düzenlenebilir hale geldi.

## Sorun

Prompt metinleri kod tabanına dağılmış Go sabitleriydi (summarizer, titler, btw,
lessons, insight-analyzer, auto-continue, subagent profilleri, handoff/continuation,
compaction, koordinatör el kitabı). Yalnızca 3'ü (`summary`/`title`/`compact`)
workspace `config/prompts/*.md` mekanizmasına bağlıydı; gerisi düzenlenemiyordu ve
tek tip bir doğrulama/fallback disiplini yoktu.

## Tasarım

**Paket:** `internal/prompts` — LEAF (internal'dan hiçbir şey import etmez; agent,
conversation ve api çevrim olmadan bağımlı olabilir).

- **Default'lar dosya:** `internal/prompts/defaults/<key>.md`, `//go:embed` ile
  gömülü. Prompt düzenlemek = markdown düzenlemek (Go bilgisi gerekmez).
- **Spec metadata:** her anahtar için `Label`/`Hint` (Türkçe UI metni),
  `Placeholders` (zorunlu `{{ad}}` yuvaları) ve `EpochAffecting` (statik prefix'e
  giren promptlarda düzenleme YENİ oturum/epoch'ta etkili olur).
- **Çözümleme sırası:** `<workspace>/config/prompts/<key>.md` override → gömülü
  default. Boş veya zorunlu yer tutucusu eksik dosya **sessizce default'a düşer**
  (eski compact-%s guard'ının genellenmişi) — bozuk bir edit turu asla kıramaz.
  UI eksik yer tutucuyu kırmızı rozetle gösterir.
- **Adlandırılmış yer tutucular:** konumsal `%s` yerine `{{summary}}` gibi adlı
  yuvalar (`prompts.Render`). Flows'un `{{...}}` sözdizimiyle tutarlı ve
  sıra-bağımsız. **Legacy geçiş:** eski iki-`%s`'li compact.md override'ı okuma
  anında otomatik dönüştürülür (`normalizeLegacy`).
- **Seed politikası değişti:** `SeedWorkspaceConfig` artık prompt default'larını
  dosya olarak YAZMAZ (dosya = yalnız gerçek kullanıcı editi). Eski build'lerin
  default-aynısı seed artıkları seed sırasında temizlenir → gelecekteki default
  iyileştirmeleri edit'lenmemiş workspace'lere otomatik ulaşır (skills seeder'ının
  çözdüğü drift problemi, daha ucuz yolla).

## Anahtarlar (15)

| Anahtar | Kullanım yeri | Yer tutucular | Epoch? |
|---|---|---|---|
| `summary` | `/board` · `/flows` özetleri (summarizer.go) | — | — |
| `title` | otomatik başlık (titler.go) | — | — |
| `compact` | konuşma compaction'ı (conversation/manager.go) | `{{summary}}` `{{messages}}` | — |
| `handoff` | context-reset devir dokümanı (conversation/handoff.go) | `{{summary}}` `{{transcript}}` `{{environment}}` | — |
| `continuation` | handoff sonrası açılış mesajı (agent/handoff.go) | `{{handoff}}` `{{oldSession}}` `{{artifact}}` (+ ops. `{{fileNote}}`) | — |
| `btw-system` / `btw-preamble` | yan soru danışmanı (btw.go) — araçsızlık yapısal, prompt değil | — | — |
| `lesson` | hata→ders reflection (lessons.go) | — | — |
| `insight-analyzer` | retrospektif tarama analizörü (insightanalyzer.go) | — | — |
| `auto-continue` | otonom devam dürtmesi (autocontinue.go) | — | — |
| `coordinator` | koordinatör el kitabı (api/chat_turn.go statik prefix) | — | ✅ |
| `subagent-explore/coder/reviewer/config` | run_subagent profilleri (subagent.go) — allowlist'ler kodda kaldı (güvenlik sözleşmesi) | — | ✅ |

Kayıt dışı bırakılanlar (bilinçli): structured-output şemaları (parser sözleşmesi),
dinamik context blokları (lessons/todo/env — veri, prompt değil), tool
description'ları, `readmeTemplate` (insan dokümanı), `instructions.md` (ayrı akış,
WSSettings ile senkron).

## Gözlemlenebilirlik (prompt izi)

`WithPromptTrace(ctx, key, resolvedText)` bağlama anahtar + 8-hex içerik hash'i
damgalar; `RecordUsage` bunu `debug.jsonl`'deki `llm_call` olayına `promptKey` /
`promptHash` alanları olarak yazar. Düzenlenmiş prompt default'tan farklı
hash'lenir → "kötü tur hangi prompt sürümüyle koştu" sorusu debug journal'dan
cevaplanır. Damgalı yollar: summary, title, lesson, insight-analyzer, btw-system.

## API / UI

- `GET/PATCH /api/workspace-config` — `promptMeta` (label/hint/placeholders/
  epochAffecting) eklendi; `promptKeys` artık 15 anahtar. `""` yazmak dosyayı
  temizler (default devralır).
- Ayarlar → Promptlar & Dosyalar: tüm anahtarlar registry metadata'sıyla render
  edilir; "özelleştirildi" etiketi, "yeni oturumlarda etkili" epoch rozeti ve
  eksik-yer-tutucu uyarısı eklendi.
- `GET /api/prompts` (salt-okunur vitrin) artık tüm registry'yi listeler;
  `POST /api/prompts/reveal` `internal/prompts/defaults` klasörünü açar.
- Market publish + workspace şablonları `agent.PromptKeys` üzerinden döndüğü için
  özelleştirilmiş TÜM promptları otomatik taşır.

## Drift koruması

`internal/prompts/prompts_test.go::TestRegistryConsistency`: her anahtarın gömülü
default'ı boş olamaz, kendi zorunlu yer tutucularını içermek zorunda ve
`defaults/` klasöründeki her dosya kayıtlı olmalı (yetim dosya = test kırılır).
Yeni prompt eklemek = 1 Spec girdisi + 1 `.md` dosyası.

## Dosyalar

- `internal/prompts/{prompts,render,resolve}.go` + `defaults/*.md` + test
- `internal/agent/wsconfig.go` (readPrompt/WorkspacePrompt/seed), `prompttrace.go`,
  `prompts.go` (vitrin), summarizer/titler/btw/lessons/insightanalyzer/
  autocontinue/subagent/coordination/handoff rewiring
- `internal/conversation/manager.go` (compact Render + ctx validasyonu),
  `handoff.go` (BuildHandoff `promptTmpl` parametresi)
- `internal/api/chat_turn.go` (coordinator `WorkspacePrompt`),
  `coordinator_prompt.go` (sabit silindi), `workspace_config.go` (promptMeta)
- `internal/db/debug_journal.go` (`promptKey`/`promptHash`)
- frontend: `types/workspace.ts`, `types/settings.ts`,
  `features/settings/WorkspaceFilesPanel.tsx`
