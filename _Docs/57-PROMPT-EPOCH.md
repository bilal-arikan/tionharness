# 57 — Prompt Epoch: Oturum-Başı Donmuş Bağlam Snapshot'ı (2026-07-08)

> external-context-agent'ın "frozen snapshot" deseninin genellenmesi: bir oturumun cache'e
> giren prompt prefix'i (araç şemaları + statik sistem promptu) oturum başında
> **dondurulur**; oturum ortası konfigürasyon değişiklikleri prompt cache'i
> kıramaz. Değişiklikler yalnız cache'in zaten öldüğü/öleceği anlarda adopte edilir.

## Sorun

Anthropic prompt cache'i prefix tabanlıdır (`tools → system → messages`); tek
baytlık değişiklik o noktadan sonrasını cache-write primiyle (1h TTL'de 2×)
yeniden yazdırır. Statik prefix her turda **canlı** durumdan derlendiği için şu
oturum-ortası olaylar tam kırılım yaratıyordu:

- Araç şeması değişimi (skill kurulumu, MCP `tools/list_changed`, sunucu
  ölümü/re-dial, denylist/görünürlük düzenlemesi, shell gate) → **tools en önde,
  her şey düşer**
- Statik system değişimi (ajan promptu, user context/settings, workspace
  talimatları, skills/lazy-tools/capability katalog blokları) → system + history
- `multiAgent` flip'i (oturuma ikinci ajan yazınca) → system notu + tüm history
  etiketleri (`labelMultiAgentHistory`)
- `CodebaseMemoryEnabled` toggle, `working_dir` değişimi (capability bloğu)

Tespit katmanı zaten vardı (`cachebreak.go` → `cache_break` debug olayı); bu iş
**önleme** katmanıdır.

## Tasarım

### Snapshot birimi

`internal/agent/promptepoch.go` — `promptEpochEntry` per (session, agent):
`Model / WorkDir / MultiAgent / System / Tools / CreatedAt / LastUsedAt` +
bellek-içi `systemStale / toolsStale / staleNotified`.

- **Bellek:** `Runtime.epochCache` (`sessionID → agentID → entry`, `epochMu`).
- **Sidecar:** `<store>/sessions/<sid>/prompt_epoch.json`
  (`internal/db/promptepoch.go`, atomik tmp→rename; `inflight.json` deseni).
  Restart cache'i öldürmediği için snapshot da restart'ı atlatmak zorunda.
  Bozuk/eski-versiyon sidecar **fail-open** (canlı derleme, yeniden dondurma).
- `LastUsedAt` disk yazımı 5 dk taneciklikle throttle'lı (`epochUsePersistEvery`).

### Üretici choke-point'leri

| Yol | Fonksiyon | Nasıl |
|---|---|---|
| Chat statik prefix | `api.composeTurnRequest` → `buildStaticPrefix` (saf builder) | `Runtime.EpochStaticSystem` sarar |
| Headless statik prefix | `agent.autonomousSystemPrompt` (builder saflaştırıldı; `EnsureCodebaseIndexed` dışarı alındı) | aynı `EpochStaticSystem` |
| Araç şemaları | `toolloop.completeTracedInner` → `shipFor()` | `Runtime.EpochToolDefs` + `mergeFrozenToolDefs` |
| claude-cli | `req.System` zaten donmuş baytları taşır | ekstra iş yok |

**Builder saflığı şart:** donmuş turda builder yalnız drift tespiti için
çağrılır, çıktısı gönderilmez — yan etkiler (auto-index) çağrı yerinde durur.

### Araçlarda ajan-kastı istisnası

`EpochToolDefs` turn başında **baz** eager seti dondurur. Tur içi
`activate_tools`/`deactivate_tools` bilinçli kast sayılır: `mergeFrozenToolDefs`
aktive edilen ismi **canlı formuna** çevirir (native-search modda `DeferLoading`
flip'i dahil), snapshot'ın hiç görmediği aktive isimleri sona ekler; kalan tüm
frozen baytlar/sıra aynen korunur. Pasif katalog drift'i ise donmuş kalır.

**Güvenlik değişmezi:** yürütme daima CANLI registry'den — kapatılan araç,
şeması hâlâ reklamlıyken bile çağrıda kapalı döner (fail-closed); izin katmanı
(permGate) her zaman canlı.

### Adopt (yenileme) tetikleyicileri

| Tetik | Kaynak |
|---|---|
| Taze oturum / handoff | entry yok → ilk turda dondur (`created`) |
| Compaction fold | `prep.Compacted` → `forceAdopt` (`compaction`) — history cache'i zaten düştü |
| Model değişimi | `entry.Model != agent.Model` (`model-changed`) |
| working_dir değişimi | `entry.WorkDir != session.WorkingDir` (`workdir-changed`) — update_session "next turn" sözü korunur |
| Katılımcı değişimi | `entry.MultiAgent != multiAgent` (`participants-changed`) |
| TTL soğuması | `LastUsedAt` > 1h (`promptEpochAdoptAfter`; providers `cacheTTL` hizalı) — yenileme bedava (`ttl-cold`) |
| Açık istek | `/refresh-context` chat komutu · `update_session {refresh_context:true}` (`SessionSink.RefreshContext`) → `RefreshPromptEpoch` (`refreshed`) |

Adopt tüm entry'yi düşürür → hem system hem tools birlikte yeniden donar
(system compose turdaki ilk adım olduğundan sıra tutarlı).

### Stale görünürlüğü

- Drift varken **dinamik suffix'e** tek satır `PromptEpochStaleNote`
  (`<context_snapshot_note>…`) — chat (`composeTurnRequest`), headless
  (`autonomousDynamicSuffix`, `PromptEpochStale` sorgusuyla) ve toolloop (tools
  drift'i, marker'la çift eklenme korumalı). Not volatil tarafta olduğundan
  kendisi cache kıramaz.
- `debug.jsonl` `epoch` olayları (`db.DebugEpoch`): `created / compaction /
  model-changed / workdir-changed / participants-changed / ttl-cold / stale /
  refreshed`. Drift başına tek `stale` olayı (`staleNotified`, düzelince re-arm).
  Emit noktaları `WithSessionID` damgalar (compose çağrıları tur damgasından önce
  gelir; damgasız ctx'te `emitDebug` olayı düşürürdü).
- Mevcut `cache_break` olayı doğal doğrulayıcı: epoch açıkken
  `prompt-or-tools-changed` kırılımları kaybolmalı. **2026-08-11'den beri bu kural
  kullanıcıya da görünür:** böyle bir kırılım olursa sohbette `cache_break` adımı
  (`CacheBreakCard`) çıkar ve "epoch açıkken bu olmamalı" uyarısını + "Bağlamı
  yenile" aksiyonunu gösterir (`_Docs\50` P7). Yani epoch regresyonu artık Debug
  kartı açılmadan fark edilir.
- **Debug paneli görselleştirmesi:** "İş akışı görselleştirmeleri" bölümüne
  **"Prompt-cache olayları"** kartı eklendi (`sessions/viz/PromptCacheEvents.tsx`
  + `flowVizData.buildPromptCacheSummary`): `epoch` (önleme) + `cache_break`
  (tespit) olayları tek zaman çizelgesinde — rozetler `donduruldu / adopte /
  stale / yenilendi / kırılım`. Okuma kuralı: **kırılım yalnız bilinçli adopt
  anlarının yanında görünmeli**; adopt'suz kırılım araştırılacak sinyaldir.
  Ham olay listesi filtresine `epoch` tipi de eklendi (`SessionDebugCard`).

## Ayar / UI

- `WSSettings.PromptEpochEnabled` (default **açık**) → `Runtime.SetPromptEpoch`
  atomic gate → `workspaceSettingsDTO.promptEpochEnabled` → Ayarlar →
  Workspace panelinde "Prompt epoch" toggle'ı (codebase-memory paritesiyle).
- Kapalı = tamamen eski davranış (her tur canlı derleme; `EpochStaticSystem`
  doğrudan `build()` döner, `EpochToolDefs` nil/fail-open).
- Chat `/` menüsü: `refresh-context` komutu (🔄) — `POST /api/sessions/{id}/summarize`
  `kind=refresh-context` (compact ile aynı kanal).
- **Cache-warmth göstergesi:** `CacheWarmthBadge.tsx` yalnız `updatedAt` + canlı
  1sn tick'ten türeyen prompt-cache sıcaklığını gösterir — 🔥 **Sıcak · mm:ss
  kaldı** (TTL soğumasına geri sayım) veya ❄️ **Soğuk** (1sa TTL doldu → sonraki
  tur tam cache-miss). Backend alanı YOK (TTL sabit 1sa; `CACHE_TTL_SEC=3600`,
  `sessionDetailFormat.cacheRemaining`). İki yerde: **Oturum bilgisi** panelinin
  "Genel" bölümünde satır (`SessionDetailPanel`) + **"Sıradaki tur bağlam
  önizleme"** popup'ının cache legend'ında TTL geri sayımı (`SessionContextModal`,
  ayrıca 2026-08-11'den beri **chat composer'ının üstünde** `CacheWarmthStrip`,
  `updatedAt` App.tsx'ten geçer; per-segment cache'li/dışı bayrakları = token
  ekseni, TTL = zaman ekseni). Her iki yerde 1sn tick tur çalışırken **veya**
  cache sıcakken döner, soğuyunca kendini durdurur.

## Context-Change Diff Yüzeyleme (2026-07-10)

Stale notu artık jenerik tek satır değil: donmuş snapshot ile canlı prefix
arasındaki **gerçek farkı** hesaplayıp iki yerde yüzeyler (ikisi de volatil
tarafta → cache'i asla bozmaz).

- **Diff motoru** `internal/agent/contextdiff.go` — paragraf-seviyeli LCS
  (`diffSystemPrefix`) + araç-adı küme farkı (`diffToolNames`). Statik prefix
  bloklar `\n\n` ile birleştiği için diff **kendiliğinden blok-hizalı**: her
  değişen paragrafın ilk satırı doğal etiket olur (`# Workspace Instructions`,
  `<user_context>`, skill katalog başlığı…) — hardcoded marker gerekmez. Notun
  her stale turda ridee etmesi için cap'li (`maxContextAreas`/`maxAreaLines`).
  **Satır kesme rune-güvenli (fix 2026-07-12):** `paragraphArea` satırı
  `maxAreaLineLen`'e keserken **rune** sayısıyla guard'lar (`len([]rune(l))`), bayt
  değil. Eski bayt-guard (`len(l)>max`) çok-baytlı UTF-8'de (Türkçe ç/ğ/ı/ö/ş/ü,
  emoji, CJK) `string([]rune(l)[:max])`'i rune-slice kapasitesini aşırıp
  **panikletiyordu** (`slice bounds out of range [:400] with capacity 384`). Panik
  tur-setup'ında (epoch/debug olayından önce) olduğu için semptom "tur hiç
  başlamıyor, hata da yok"tu — kuyruk worker'ını da kilitliyordu (bkz. `_Docs\58`).
- **Hesaplama seam'i:** `EpochStaticSystem` zaten `build() != e.System`
  karşılaştırmasında canlı+donmuş metnin ikisini de elinde tutuyor → orada
  `e.systemChange` doldurulur; `EpochToolDefs` `e.toolsChange`'i doldurur;
  `combinedChange` ikisini birleştirir.
- **Ajana (suffix notu):** `PromptEpochContextNote` her stale turda `<context_
  snapshot_note>` içine "What changed since the snapshot: +/- <etiket>" listesi
  enjekte eder (chat + headless + toolloop, hepsi `suffixNoteMarker` guard'ıyla
  çift-eklenme korumalı). Böylece ajan davranışını cache adopte edilmeden **kısmen
  hemen** uyarlayabilir. Boş diff → jenerik `PromptEpochStaleNote`'a düşer.
- **Kullanıcıya (chat UI):** yeni `StepContextChange` (`kind="context_change"`)
  TurnStep — drift **başına bir kez** (`changePending` one-shot, `noteEpoch
  StaleLocked` arms/re-arms), `ConsumeContextChange` ile tüketilir; canlı SSE'ye
  yayılır + kalıcı `steps`'e prepend (reload'da kalır). Chat/non-stream yolları
  `consumeContextChangeLead` helper'ını paylaşır. Frontend `ContextChangeCard.tsx`:
  katlanabilir renkli +/- diff (blok başlığı → gövde), Ayarlar → Adım Türleri'nde
  belgeli. Not: değişiklik yine sadece bir sonraki refresh'te **tam** uygulanır.

## Bilinçli Takaslar

- Kullanıcı ajan promptunu/skill setini düzenleyince açık oturumlarda davranış
  **tam** hemen değişmez (frozen prefix refresh'e kadar sabit) — ama ajan artık
  suffix notundaki diff'ten NE değiştiğini görür, kullanıcı da context_change
  adımından. hermes'in bilinçli takası + değişiklik görünürlüğü.
- Donmuş MCP şeması sunucu tarafında değişmişse arg uyumsuzluğu hatalı
  tool_result üretir → self-healing (toolguard/lessons) yakalar; stale notu
  refresh'i işaret eder.
- Tur içi reactive compaction (bağlam taşması) epoch'u adopte ETMEZ (tur ortası
  system değişimi daha kötü); bir sonraki `Prepare` fold'u yakalar.

## Test

`internal/agent/promptepoch_test.go`: freeze/drift-stale/revert · 4 adopt
tetiği · TTL-cold · disabled pass-through · sidecar restart · tool freeze +
pasif drift · `mergeFrozenToolDefs` (aktivasyon canlı form + append, frozen
baytlar sabit) · refresh (bellek + sidecar) · per-agent izolasyon.
`fakeSessionSink.RefreshContext` ile update_session alanı. Suite: 796/30 paket
+ frontend tsc yeşil.

**Canlı E2E (2026-07-08, izole instance :8099 + geçici store, gerçek
fable-5/claude-cli):** tur 1 → `epoch created` + sidecar donmuş System; oturum
ortası workspace talimat değişikliği → tur 2 frozen baytlarla döndü, tek
`stale` olayı, sidecar'da drift YOK; `/refresh-context` → `refreshed` + sidecar
silindi; tur 3 → yeni `created`, yeni sidecar değişikliği İÇERİYOR; tur 4
(kararlı) → yeni olay yok (spam yok); `cache_break` = 0. Olay dizisi birebir:
`created → stale → refreshed → created`. Yan gözlem (epoch-dışı): izole
workspace claude-home'unda seed'lenmiş credential ile turlar arasında aralıklı
`authentication_failed` görüldü (token rotasyonu şüphesi; retry ile geçiyor) —
`51-CLAUDE-CONFIG-BIRLESIK` alanına ayrı araştırma konusu.

## Dosya Haritası

- `internal/agent/contextdiff.go` (+`_test.go`) — diff motoru + `ContextChange`/
  `ContextArea` + `SuffixNote`; `trace.go` — `StepContextChange`/`ContextChangeStep`;
  `promptepoch.go` — `systemChange`/`toolsChange`/`changePending` + `PromptEpoch
  ContextNote`/`ConsumeContextChange`; enjekte: `chat_turn.go`(+`consumeContextChangeLead`)/
  `chat.go`/`chat_stream.go`/`runtime.go autonomousDynamicSuffix`/`toolloop.go`;
  frontend `features/chat/ContextChangeCard.tsx` + `types/message.ts` + `settings/stepKinds.ts`
- `internal/agent/promptepoch.go` (+`_test.go`) — çekirdek
- `internal/db/promptepoch.go` — sidecar; `internal/db/debug_journal.go` — `DebugEpoch`
- `internal/api/chat_turn.go` — `buildStaticPrefix` + epoch + stale notu
- `internal/agent/runtime.go` — gate + `autonomousSystemPrompt`/`autonomousDynamicSuffix`
- `internal/agent/toolloop.go` — `shipFor` (frozen + aktivasyon merge)
- `internal/agent/sessionsink.go` + `internal/tools/sessionsink.go` + `builtin_sessionupdate.go` — `refresh_context`
- `internal/api/summary.go` — `/refresh-context`; `internal/api/workspace_settings.go` — DTO
- `internal/workspace/settings.go` — ayar; frontend: `WorkspacePanel.tsx`, `chatStreamCommands.ts`, `types/workspace.ts`, `WorkspaceView.tsx`
- Cache-warmth göstergesi (frontend): `features/sessions/CacheWarmthBadge.tsx` +
  `sessionDetailFormat.ts` (`CACHE_TTL_SEC`/`cacheRemaining`/`formatCountdown`),
  bağlayanlar `SessionDetailPanel.tsx` (Genel bölümü) + `SessionContextModal.tsx`
  (popup cache legend) + `app/App.tsx` (`updatedAt` prop'u)
