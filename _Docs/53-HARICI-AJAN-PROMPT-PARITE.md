# 53 — the external agent project Sistem-Promptu Paritesi (karşılaştırma notu)

the external agent project'ın (`external-agent-oss`) tam sistem promptu referans alınarak TionHarness'ın
kendi prompt kurgusuyla kısa karşılaştırması: neyi aldık, neyi bilinçli almadık, neyi
farklı çözdük. İlgili: [50-CLAUDE-CODE-CACHE-PARITE.md](50-CLAUDE-CODE-CACHE-PARITE.md)
(statik/dinamik cache bölünmesi), [analiz-craftagent-arac-eslestirme.md](analiz-craftagent-arac-eslestirme.md)
(araç eşleştirme), [arsiv/13-CRAFT-AGENTS-INCELEME.md](arsiv/13-CRAFT-AGENTS-INCELEME.md).

## İlke

the external agent project "her şeyi prompta göm" (~28K karakter statik + dinamik bloklar). TionHarness
**lean prefix → skill'e devret** (`default-instructions.md` ~166 satır). Bu yüzden parite
= körü körüne kopyalama değil; **gerçek boşluk + TionHarness'da destekli + prefix'i
şişirmeyen** olanı almak. `internal/workspace/defaults_test.go` guard'ı desteklenmeyen
the external agent project-ism'lerin (datatable/html-preview/call_llm/render_template/_displayName) geri
sızmasını aktif engeller.

## Statik prompt — alınan / alınmayan

| the external agent project bölümü | TionHarness | Not |
|---|---|---|
| Web Search nudge · Diagrams · Diffs · Document Tools · Git · Permissions · User prefs · Interaction guidelines | **VAR** | Zaten karşılığı mevcut |
| Session self-management (labels/status) | **EKLENDİ** (özet + skill pointer) | Tam açıklama `tionharness-self-management` |
| "Confirm destructive" → geri-döndürülemez + dışa-dönük | **GENİŞLETİLDİ** | send/publish/push kapsandı |
| Environment marker (`<environment>` OS/arch/shell) | **EKLENDİ** (dinamik tarafta) | `agent.EnvironmentContextBlock()` |
| Shell-tool gate advertisement | **EKLENDİ** (dinamik tarafta) | `Runtime.ShellToolsContextBlock()` — **tek kaynak, hiç boş dönmez**: gate açık + backing shell varsa Bash/PowerShell'i adıyla duyurur; **kapalıysa "shell DISABLED" + ölü-araç kuralını** enjekte eder (bir araç "not enabled in this context" derse ölü kabul et, aynı çağrıyı tekrarlama, fs araçlarına geç ya da raporla — ayrıca terminal varsayan rehberlik `rtk`/`go test`/`npm` shell'siz oturumda geçersiz). Bu, ajanın bare `PowerShell` çağırıp tur zaman aşımına düşmesini keser (FND-9c9a52aa · FND-6095a777 · FND-e9c79d9a · FND-495575b8). Statik talimatlar shell'i "her zaman var" diye VAAT ETMEZ (yalnız fs araçları always-on). `tools.ShellToolNames()` kayıt koşullarıyla aynı resolver'ı paylaşır → prompt ↔ katalog drift yok. **Ajan filtresi (2026-08-17):** workspace gate'i tek başına yetmiyordu — allowlist'inde `Bash`/`PowerShell` olmayan bir ajana "ENABLED" deniyor, ajan Bash'i çağırıp her seferinde `No such tool available` alıyordu (makine genelinde 100 başarısız çağrı; Bash hata oranı %4.05'e karşı köprülü MCP shell'de %0.21). Blok artık host'un desteklediği shell adlarını **ajanın kendi tool filtresinden** geçirir (`availableShellToolNames` → `ToolAllowedFunc`, `ToolCatalog`'un kullandığı **aynı** kapı): ikisinden hiçbiri kalmıyorsa DISABLED varyantı, yalnız biri kalıyorsa **sadece o** duyurulur. Bu yüzden `ShellToolsContextBlock(ctx, agent, confined)` imzası ajan taşır. Testler: `shellcontext_test.go` (allowlist'siz / kısmi allowlist / denylist) |
| Structured Data (datatable/spreadsheet) · HTML/PDF/Markdown Preview · call_llm · render_template · Source Templates · Tool Metadata (`_displayName/_intent`) | **DIŞLANDI** | Frontend/araç desteği yok → prompta yazmak halüsinasyon; guard yasaklıyor |
| Configuration Documentation tablosu (`~/.external-agent/docs/*`) | **DIŞLANDI** | TionHarness docs yerine skill'e devreder |
| External Sources (config.json/guide.md) | **FARKLI** | TionHarness MCP-server modeli kullanır |

## Dinamik bağlam blokları (the external agent project user-tail vs TionHarness `SystemDynamic`)

the external agent project volatil blokları **user-mesaj kuyruğuna** koyar; TionHarness **`SystemDynamic`**
suffix'ine (cache breakpoint'ten sonra). İkisi de cache'li prefix'i sabit tutar.

| the external agent project bloğu | TionHarness | Mekanizma |
|---|---|---|
| Date/Time | **VAR** | `dateTimeContextBlock()` (saniye hassas, tur başında sabit) |
| Working Directory | **VAR (daha zengin)** | `workdirContextBlock()`: cwd + git branch + CLAUDE.md; + env marker |
| Session State (`<session_state>`) | **EKLENDİ** | `sessionStateBlock()`: sessionId + permissionMode (read-only/ask/auto) + workspace id/isim/path |
| Source State (`<sources>`) | **YOK** | MCP tool kataloğu karşılıyor; ayrı durum bloğu ~tekrar |
| Workspace Capabilities | **YOK** | Runtime içi bilgi; düşük değer |
| Recovery Context (`<recovery_context>`) | **GEREKSİZ** | Aşağıya bakınız |

### TionHarness'a özel ek dinamik bloklar (the external agent project'ta yok)
Goal · Coordination scratchpad (M2) · Artifacts listesi · Todo/progress (kalıcı, resume) ·
Cross-session özeti · Lifecycle-hook context · Memory-pressure uyarısı.
*(Core memory + recall vardı → hafıza alt sistemi kaldırılınca gitti — `7849daf`.)*

## Recovery Context neden gereksiz

the external agent project Claude Agent SDK'nın **sunucu-tarafı oturumuna** yaslanır; native `resume`
başarısız olursa bir sonraki user mesajına önceki konuşmanın **özetini** enjekte eder
(son güvenlik ağı). TionHarness **dosya-tabanlıdır**: her tur `session.jsonl`'den **tam
transkript** yüklenir → "resume başarısızlığı" durumu yoktur, özet fallback'ine gerek
kalmaz. Daha dar olan tur-ortası süreç-ölümü riski için `inflight.json` crash-recovery
sidecar'ı vardır (`chat_stream.go`: ~600ms snapshot, reply-id paylaşımı → idempotent
kurtarma; tur normal biterse silinir). Yani muadili özet değil **ham transkript** korur.

## Bu oturumda uygulanan değişiklikler (commit izi)

- `feat(prompt): environment marker + self-mgmt/destructive prompt polish`
- `docs(prompt): add tionharness-deliverables to key skills + drop stale "memory"`
- `feat(subagent): add "config" mini-agent profile` (the external agent project `getMiniAgentSystemPrompt`
  karşılığı → `run_subagent` profili, config araçlarına sandbox'lı)
- `feat(prompt): inject <session_state> block (session + workspace identity + mode)`

## Açık (bilinçli ertelenen)
- **Tool Metadata (`_displayName/_intent`)** — her çağrıya token vergisi + guard yasağı;
  eklenirse settings-gated olmalı.
- **Source Templates / `render_template`** — asıl blokör `html-preview` inline render;
  ayrı planlama session'ında ele alınıyor.
