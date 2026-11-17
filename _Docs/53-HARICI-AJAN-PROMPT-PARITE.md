# 53 — the external agent project Sistem-Promptu Paritesi (karşılaştırma notu)

> **Özet (2026-09-03, güncelleme 2026-11-17):** the external agent project'ın tam sistem promptu ile TionHarness'in kendi prompt
> kurgusunu karşılaştıran, **uygulanmış** bir parite dokümanıdır — TionHarness "lean prefix
> → skill'e devret" ilkesini izler (the external agent project'ın ~28K karakterlik statik promptunun
> aksine). Alınanlar: session self-management, environment marker (2026-11-17'den beri
> yalnız `<environment os=".." arch=".." />`; eski shell-tool gate duyurusu TionHarness'in
> kendi kabuğuyla birlikte kalktı); bilinçli dışlananlar: datatable/spreadsheet, `_displayName`/
> `_intent` metadata, `~/.external-agent/docs`. `render_template`/`html-preview` sonradan
> TionHarness-native karşılığıyla uygulandı. Guard testi (`internal/workspace/
> defaults_test.go`) desteklenmeyen the external agent project-ism'lerin geri sızmasını engeller.

the external agent project'ın (`external-agent-oss`) tam sistem promptu referans alınarak TionHarness'in
kendi prompt kurgusuyla kısa karşılaştırması: neyi aldık, neyi bilinçli almadık, neyi
farklı çözdük. İlgili: [50-CLAUDE-CODE-CACHE-PARITE.md](50-CLAUDE-CODE-CACHE-PARITE.md)
(statik/dinamik cache bölünmesi), [analiz-harici-ajan-arac-eslestirme.md](analiz-harici-ajan-arac-eslestirme.md)
(araç eşleştirme), [arsiv/13-HARICI-AJANLAR-INCELEME.md](arsiv/13-HARICI-AJANLAR-INCELEME.md).

## İlke

the external agent project "her şeyi prompta göm" (~28K karakter statik + dinamik bloklar). TionHarness
**lean prefix → skill'e devret** (`default-instructions.md` ~166 satır). Bu yüzden parite
= körü körüne kopyalama değil; **gerçek boşluk + TionHarness'te destekli + prefix'i
şişirmeyen** olanı almak. `internal/workspace/defaults_test.go` guard'ı desteklenmeyen
the external agent project-ism'lerin (datatable/spreadsheet/call_llm/`~/.external-agent/docs`/_displayName/
pdf-preview/markdown-preview/"the external agent project" adı — tam liste `defaults_test.go:19-28`) geri
sızmasını aktif engeller.

## Statik prompt — alınan / alınmayan

| the external agent project bölümü | TionHarness | Not |
|---|---|---|
| Web Search nudge · Diagrams · Diffs · Document Tools · Git · Permissions · User prefs · Interaction guidelines | **VAR** | Zaten karşılığı mevcut |
| Session self-management (labels/status) | **EKLENDİ** (özet + skill pointer) | Tam açıklama `tionharness-self-management` |
| "Confirm destructive" → geri-döndürülemez + dışa-dönük | **GENİŞLETİLDİ** | send/publish/push kapsandı |
| Environment marker (`<environment>` OS/arch) | **EKLENDİ** (dinamik tarafta) | `agent.EnvironmentContextBlock()` → `<environment os=".." arch=".." />` (2026-11-17'den beri shell bilgisi yok; kabuk lehçesini CLI'nin kendi kabuk aracı belgeler, Windows git-bash yol rehberi yalnız claude-cli ajanlarına `shellEnvironmentCapability` ile gider) |
| Shell-tool gate advertisement | **KALDIRILDI (2026-11-17)** | Eskiden `Runtime.ShellToolsContextBlock()` köprülü `Bash`/`PowerShell`'i adıyla duyuruyor ya da "Shell execution is DISABLED" + ölü-araç kuralını enjekte ediyordu (ajan-filtreli; FND-9c9a52aa · FND-6095a777 · FND-e9c79d9a · FND-495575b8). TionHarness'in kendi kabuğu kaldırılınca blok da silindi: ajan her zaman CLI'nin native kabuğunu kullanır; claude-cli'de etkin araç filtresi `Bash`'i kabul etmeyen ajanın tüm kabuk ailesi `--tools` menüsünden düşer, codex/opencode'da böyle ajan read-only izin modunda koşar. |
| Structured Data (datatable/spreadsheet) · PDF/Markdown Preview · call_llm · Tool Metadata (`_displayName/_intent`) | **DIŞLANDI** | Frontend/araç desteği yok → prompta yazmak halüsinasyon; guard yasaklıyor |
| Source Templates / `render_template` · `html-preview` | **UYGULANDI** (2026-07-06) | TionHarness-native karşılık: `internal/tools/builtin_render_template.go` + inline HTML önizleme (`frontend/src/shared/components/markdown/HtmlPreview.tsx`). Guard bu ikisini artık yasaklamaz. Detay: [63-SOURCE-TEMPLATES-RENDER.md](63-SOURCE-TEMPLATES-RENDER.md) |
| Configuration Documentation tablosu (`~/.external-agent/docs/*`) | **DIŞLANDI** | TionHarness docs yerine skill'e devreder |
| External Sources (config.json/guide.md) | **FARKLI** | TionHarness MCP-server modeli kullanır |

## Dinamik bağlam blokları (the external agent project user-tail vs TionHarness `SystemDynamic`)

the external agent project volatil blokları **user-mesaj kuyruğuna** koyar; TionHarness **`SystemDynamic`**
suffix'ine (cache breakpoint'ten sonra). İkisi de cache'li prefix'i sabit tutar.

| the external agent project bloğu | TionHarness | Mekanizma |
|---|---|---|
| Date/Time | **VAR** | `dateTimeContextBlock()` (saniye hassas, tur başında sabit) |
| Working Directory | **VAR (daha zengin)** | `workdirContextBlock()`: cwd + git branch + talimat dosyası (`CLAUDE.md` önce, `AGENTS.md` fallback — `agent.ResolveInstructionFile`, yalnız ad, içerik inline DEĞİL); + env marker. Headless turlar `workdirConfineBlock()` ile aynı probe'u alır — native Go araç döngüsü talimat dosyasını natively yüklemeyen tek backend olduğu için asıl ihtiyaç orada. Detay: [26-CALISMA-DIZINI.md](26-CALISMA-DIZINI.md) |
| Session State (`<session_state>`) | **EKLENDİ** | `sessionStateBlock()`: sessionId + permissionMode (read-only/ask/auto) + workspace id/isim/path |
| Source State (`<sources>`) | **YOK** | MCP tool kataloğu karşılıyor; ayrı durum bloğu ~tekrar |
| Workspace Capabilities | **YOK** | Runtime içi bilgi; düşük değer |
| Recovery Context (`<recovery_context>`) | **GEREKSİZ** | Aşağıya bakınız |

### TionHarness'e özel ek dinamik bloklar (the external agent project'ta yok)
Goal · Coordination scratchpad (M2) · Artifacts listesi · Todo/progress (kalıcı, resume) ·
Cross-session özeti · Lifecycle-hook context · Memory-pressure uyarısı.

## Recovery Context neden gereksiz

the external agent project Claude Agent SDK'nın **sunucu-tarafı oturumuna** yaslanır; native `resume`
başarısız olursa bir sonraki user mesajına önceki konuşmanın **özetini** enjekte eder
(son güvenlik ağı). TionHarness **dosya-tabanlıdır**: her tur `session.jsonl`'den **tam
transkript** yüklenir → "resume başarısızlığı" durumu yoktur, özet fallback'ine gerek
kalmaz. Daha dar olan tur-ortası süreç-ölümü riski için `inflight.json` crash-recovery
sidecar'ı vardır (`chat_stream.go`: ~600ms snapshot, reply-id paylaşımı → idempotent
kurtarma; tur normal biterse silinir). Yani muadili özet değil **ham transkript** korur.

## Açık (bilinçli ertelenen)
- **Tool Metadata (`_displayName/_intent`)** — her çağrıya token vergisi + guard yasağı;
  eklenirse settings-gated olmalı.
