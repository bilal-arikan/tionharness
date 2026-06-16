# SwarmGo — İlerleme Takibi

> Bu dosya canlı tutulur; her oturumda güncellenir. Son güncelleme: **2026-06-17**

## Ara fix — Context metre gerçek footprint'i sayar (2026-06-17)

Oturum bilgisi panelindeki **Bağlam penceresi** ölçeri eskiden yalnızca özet +
sohbet mesajlarını sayıyordu; **sistem promptu, araç/MCP şemaları ve artifact
bloğu** (her tura giden ama mesaj olmayan içerik) hesaba katılmıyordu → panel
gerçek doluluğu olduğundan düşük gösteriyordu (kullanıcı tespiti).

- **Backend (`api/session_info.go`):** yeni `systemFillers` —
  `composeTurnRequest`'i birebir aynalayarak sistem promptunu (persona + kullanıcı
  profili + workspace talimatları), ajanın **efektif araç kataloğunu**
  (`Runtime.ToolCatalog`, built-in + MCP) ve session artifact bloğunu tahmin eder;
  `estimateToolCatalog` araç başına ad+açıklama+JSON şema (+ çerçeve) maliyetini
  toplar. `ContextTokens` artık bu ekstrayı içerir; `Fillers`'a **Sistem promptu**
  (`system`), **Araçlar** (`tools`), **Artifactlar** (`artifacts`) kovaları eklenir
  ve tümü token ağırlığına göre sıralanır.
- **Frontend (`SessionDetailPanel.tsx`):** `fillerColor`'a `tools` (mor) ve
  `artifacts` (pembe) renkleri; panel zaten fillers'ı role göre generic render
  ettiğinden başka değişiklik gerekmedi. "Boş alan" otomatik küçülür.
- **Test:** `api/session_info_test.go::TestEstimateToolCatalog` (boş=0, monotonik).
  **Canlı test:** geçici sunucu + boş oturum → `Araçlar ~1942 tok / 15 araç` +
  `Sistem promptu ~38 tok` (eskiden ~0 gösterirdi). ✅ build/vet/test + tsc yeşil.

> Cevap: tool/MCP şemaları **artık** Context hesabına dahil ve ayrı kalem olarak
> gösteriliyor. Not: hafıza recall bloğu sorgu-bağımlı olduğundan kasıtlı olarak
> hariç bırakıldı (yanıltıcı olmasın); özet zaten ayrı kovada sayılıyor.

## Faz A2 — Mesaj ekleri (attachments, 2026-06-17)

Kullanıcı sohbet turuna **çoklu dosya** ekleyebilir; ekler input üstünde tip
ikonlu chip olarak görünür (x ile iptal), gönderince kullanıcı balonunda kalır.
external-agent-oss attachment sistemi referans alındı.

- **Yükleme:** `POST /api/uploads` (multipart, `api/uploads.go`) → dosyayı
  workspace sandbox'ında `uploads/<sessionId>/<id>-<ad>` altına yazar (ajan
  `read_file` ile okur). Kaba `kind` tespiti (image/text/code/pdf/office/
  archive/audio/video); küçük (≤100KB) text/code dosyaları içeriğini
  `Attachment.TextContent`'e inline taşır. 25MB cap.
- **Kalıcılık:** `db.Attachment` (`models_attachment.go`) + `db.Message.Attachments`;
  `chat.go` + `chat_stream.go` user mesajına yazar. Metin boşsa **ek varken**
  gönderime izin.
- **Provider:** `conversation.toProviderMessages` → `withAttachments` ekleri user
  metnine katar: text/code verbatim inline (```...```), binary/görsel `read_file`
  path listesi olarak. Test: `conversation/attachments_test.go`.
- **Inline görsel servis:** `GET /api/files` workspace-göreli **`?rel=`** formu
  kazandı; `<img>` header gönderemediğinden `?ws=<id>` ile scope alır.
  Path-traversal guard. `withWorkspace` header yoksa `?ws=` query'sini okur.
- **Frontend:** `Composer` ek tepsisi (📎 buton + gizli multi-input + drag-drop +
  **paste**: >2000 karakter pano metni `.txt` ekine, yapıştırılan görsel
  yüklenir), dosya-başına yükleme durumu + **x**; `AttachmentChip` (görsel
  thumbnail / dosya kartı) tepside + `UserBubble`'da; `api/uploads.ts`,
  `lib/attachments.tsx` (ikon/etiket/`imageURL`), `types/attachment.ts`.

✅ go build/vet/test + tsc -b/vite build yeşil. **API canlı round-trip** (geçici
:8091 scratch instance): text+görsel upload → doğru kind/rel/size, `?rel=` görsel
servis 200 image/png, traversal `../../` → 400, diske kalıcılık, `TextContent`
inline doğrulandı.

**Not:** mcp-chrome bu oturumda bağlı olmadığından tarayıcı görsel testi
yapılamadı (çalışan :8090 backend de eski binary — özellik için yeniden başlatma
gerek). **v2:** görsel **multimodal** (model görseli görür — provider katmanına
anthropic image content-block eklenmeli), "Artifact'a dönüştür" butonu, drag-drop
cilası.

## UI — Yenileme sonrası "düşünüyor" göstergesinin geri yüklenmesi (2026-06-17)

**Sorun:** Mesaj gönderdikten hemen sonra sayfa yenilenince "agent düşünüyor"
göstergesi kayboluyordu. Turlar istemci bağlantısından **detached** olduğundan
sunucuda çalışmaya devam eder; ama reload sonrası client'ın `pending` state'i
sıfırlandığından gösterge gider, yanıt ancak tur bitip `chat` event'i gelince
görünür — arada "boşluk" oluşuyordu.

**Çözüm:** Sunucu, hangi oturumların **uçuşta** turu olduğunu bildirir; frontend
reload'da bunu sorgulayıp göstergeyi geri yükler.
- **Backend:** `chatRun`'a `sessionID` alanı + `chatRuns.activeSessionIDs()`
  (distinct, in-flight oturumlar). Yeni uç **`GET /api/sessions/active`** →
  `{sessionIds:[]}` (`handleActiveSessions`, `server.go` route). `register`
  imzası `(id, sessionID, cancel)`. Ayrıca `chat_stream.go`'da **tur bitişinde
  hata yolunda da** terminal `chat` event'i yayan guard'lı defer (`turnStarted`
  +`emitted`) — önceden event yalnız başarı yolunda yayılıyordu, hata olunca
  gösterge takılı kalabilirdi.
- **Frontend:** `api.activeSessions()`; `useChatStream` `markPending(ids)` /
  `clearPending(sid)` (yalnız `pendingSessions` — `streaming` değil, böylece
  reload sonrası composer "Gönder"de kalır, kırık Durdur yok). `App.tsx`
  boot/workspace-değişiminde aktif turları seed'ler; her `chat` event'inde ilgili
  oturumu temizler (+ açık transcript'i yeniden yükler).

✅ go build/vet/test + tsc + vite build yeşil; geçici instance'ta
`GET /api/sessions/active` → `{"sessionIds":[]}` (200) smoke doğrulandı.
**Not (1):** çalışan 8090 backend eski binary — uç etkin olması için **backend
restart** gerekir (eski binary'de uç 404 döner, `api.activeSessions()` sessizce
yutar → regresyon yok). **Not (2):** çalışma ağacında eşzamanlı **attachments**
özelliği paylaşılan dosyalara (server.go/App.tsx/useChatStream.ts) iç içe
girdiğinden bu değişikliklerin commit'i kullanıcıya bırakıldı.

## UI — Açıklayıcı HTTP hata mesajları (2026-06-17)

Header'daki kırmızı hata pill'i (sol-üst) artık çıplak **"HTTP 502"** yerine
**eyleme dönük** mesaj gösterir. Kök neden: `api/client.ts` `req()` ve
`api/chat.ts` non-OK yanıtta gövdeyi JSON parse edip `{error}` arıyor;
**502/503/504** Vite dev-proxy'den gelir (Go backend ulaşılamıyor) ve gövde
JSON olmadığından çıplak `HTTP <status>` kalıyordu. **Çözüm:** `client.ts`'e
`describeHttpError(status)` (status→Türkçe açıklama; 502/503/504 → "Sunucuya
ulaşılamıyor… `go run ./cmd/swarmgo` çalışıyor mu", 500/404/401/403/400/408/429
özel) + `errorFromResponse(res)` (backend `{error}` öncelikli, yoksa status
açıklaması) yardımcıları eklendi; `req()` ayrıca **fetch reddini** (sunucu hiç
yanıt vermiyor) yakalayıp "Sunucuya bağlanılamadı…" döndürür. `chat.ts` aynı
yardımcıyı kullanır. ✅ tsc + vite build temiz; saf eşleme node ile doğrulandı.
(Canlı 502: çalışan backend'i durdurmamak için tetiklenmedi.)

## Ara özellik — Zamanlama düzenleme (schedule edit) (2026-06-17)

**İstek:** "Zamanlamalar ekranında eklenen zamanlamaları liste halinde görebilelim, tıklayıp aktif/deaktif etme, silme veya editleme yapabilelim."

Liste + toggle (aktif/pasif) + sil zaten vardı (`components/panels/Schedules.tsx`); eksik olan tek şey **düzenleme** idi. Eklenenler:

- **Backend `db.UpdateSchedule`** (`store_schedule.go`): id ile bulup `AgentID`/`CronExpr`/`TaskID`/`Prompt` alanlarını günceller; `Enabled` + teslimat alanları (`LastDeliveryStatus`/`LastRunAt`/`NextRunAt`) **korunur** (yalnızca tanım düzenlenir).
- **`PUT /api/schedules/{id}`** (`api/schedules.go` `handleUpdateSchedule`): create ile aynı doğrulama (`agentId`+`cronExpr` zorunlu, `taskId` **veya** `prompt` gerekli, bilinmeyen ajan reddi). Başarıda `Scheduler.Reload(ctx)` çağırır (cron tablosu yeni ifadeyle yeniden yüklenir) ve güncel satırı döner. Route `server.go` `registerScheduleRoutes`'a eklendi.
- **Frontend `api.updateSchedule`** (`api/tasks.ts`) + `Schedules.tsx` **satır-içi düzenleme modu**: her satırda ✎ düğmesi → o satır ajan/preset/cron/görev/prompt formuna dönüşür (**Kaydet**/**İptal**). Kaydedince listedeki satır sunucudan dönen güncel veriyle değiştirilir; tek seferde bir zamanlama düzenlenir.

✅ `go build`/`vet` + `tsc -b`/`vite build` yeşil. **API canlı test** (yeni binary, `127.0.0.1:8090`): schedule create → `PUT` (cron `*/5 * * * *`→`0 9 * * *` ve prompt değişti, `enabled=true` **korundu**) → list ile diskte kalıcılık doğrulandı → delete ile temizlik. Not: gateway/mcp-chrome bu oturumda bağlı değil → tarayıcı görsel testi yapılmadı (API round-trip kanıt).

## Bugfix — claude-cli transcript'ine sızan harness markup'ı (system-reminder balonu) (2026-06-17)

**Belirti:** `claude-cli` sağlayıcısıyla çalışan bir ajan, çok-turlu bir sohbette araç kullandıktan (Edit/Read) sonra gelen bir kullanıcı isteğine cevap yerine `<system-reminder>The assistant message ... is malformed ...</system-reminder>` metnini **bir sohbet balonu olarak** üretiyordu.

**Kök neden:** `providers/serializeTranscript`, çok-turlu geçmişi düz metne çevirirken önceki asistan turunun `Text`'ini **olduğu gibi** CLI'ye geri besliyordu. Önceki turun metnine sızmış tool-call markup'ı (`</parameter></parameter></function_results>` gibi) içeren bir transcript, claude CLI'nin girdi-onarım harness'ini tetikliyor; o da `<system-reminder>` enjekte ediyor ve model bunu geri tükürüyor.

**Düzeltme (`internal/providers/sanitize.go` — yeni):**
1. **Girdi temizleme (kök neden):** `sanitizeTranscriptText` — `<system-reminder>…</system-reminder>`, `<function_calls|function_results>` blokları ve başıboş `invoke`/`parameter`/`function_*` etiketlerini regex ile strip eder; `serializeTranscript` artık her **asistan** turunu bununla geçirir (kullanıcı metnine dokunulmaz). Sıradan düzyazı/kod korunur (yalnız bu spesifik harness etiket adları hedeflenir).
2. **Çıktı koruması (savunma):** `cliStreamParser.finish()` artık `isRepairArtifact` ile çıktının bir onarım-reminder'ı olup olmadığını kontrol eder; öyleyse temizler, geriye gerçek cevap kalmazsa turu hata ile başarısız sayar (reminder asla kaydedilmez/gösterilmez).

**Testler:** `providers/sanitize_test.go` (`TestSanitizeTranscriptText_StripsHarnessMarkup`, `TestIsRepairArtifact`, `TestSerializeTranscript_SanitizesAssistantTurns`). ✅ `go build`/`vet`/`test ./internal/...` yeşil. Not: native (anthropic) yol yapısal content-block kullandığından bu sızıntıya açık değil; düzeltme claude-cli düz-metin transcript yoluna özgüdür.

## UI — Akış scroll fix + tool kartı başlık rozetleri (2026-06-17)

İki kullanıcı geri bildirimi (yalnız frontend, düşük risk):

1. **Bug — uzun sohbette canlı cevap "kayboluyor", tur bitince görünüyor.**
   Kök neden: `components/MessageList.tsx` her `messages` değişiminde (akışta her
   token bir delta → `setMessages`) `endRef.scrollIntoView({behavior:'smooth'})`
   çağırıyordu. Uzun transcript'te (büyük scroll yüksekliği) her delta bir önceki
   smooth animasyonu yarıda kesip yeniden başlatıyor → animasyon hiç oturmuyor,
   viewport canlı baloncuğu gösteremiyor; akış bitince son scroll oturunca cevap
   "gözüküyor". **Düzeltme:** sticky-bottom + **anlık** scroll. Scroll konteynerine
   `ref`+`onScroll` eklendi; `pinnedRef` kullanıcının dibe sabitli olup olmadığını
   izler (eşik 80px) → yukarı kaydırıp geçmiş okurken deltalar artık dibe çekmiyor.
   Pinned iken `el.scrollTop = el.scrollHeight` (smooth değil → delta thrash'i yok).
   Oturum değişiminde (ilk mesaj id'si değişince) yeniden dibe sabitlenir.

2. **Feature — tool kartı başlığında (collapse halinde) özet rozet.** Native yol
   zaten `diff` step → `DiffCard`'da `+eklendi/−silindi` gösteriyordu; **claude-cli**
   yolunda Read/Edit/Write `tool` step → `ActivityCard`'a gidiyor ve sayı yalnız
   kart açılınca (DiffView) görünüyordu. `ActivityCard` başlığına rozet eklendi:
   Edit/Write için çıktıyı `parseDiff` ile çözüp **`+X −Y`** (added>0||removed>0),
   Read/`read_file` için çıktı satır sayısı **`N satır`**. `lib/tools.ts`'e
   `toolBase`/`isReadTool` export'ları eklendi. ✅ tsc + vite build temiz.
   (Chrome canlı testi: gateway-manager/mcp-chrome bu oturumda bağlı değil — yapılamadı.)

## Faz P3 — İzin/onay katmanı (Aşama 1: claude-cli izin modu, 2026-06-16)

**Sorun (kök neden):** claude-cli ajanları **hiçbir dosya editleme işlemini yapamıyordu.**
SwarmGo `claude -p` (headless) ile shell-out yapıyor ama `--permission-mode` /
`--dangerously-skip-permissions` bayraklarını **hiç geçmiyordu**. Headless modda
varsayılan izin modu Edit/Write/Bash için onay ister; soracak arayüz olmadığından bu
araçlar **sessizce reddediliyordu**. (external-agent-oss'un 3-modlu izin sistemine bakıldı.)

**Aşama 1 — ajan-bazlı izin modu → CLI bayrağı eşlemesi:**
1. `db.Agent.PermissionMode` (`"read-only" | "ask" | "auto"`, boş = `auto`) — yeni ajanda
   varsayılan `auto`; mevcut ajanlar (alan boş) da `auto`'ya düşer → anında çözülür.
   `AgentProfilePatch.PermissionMode` ile kısmi güncelleme (`store.go`).
2. `providers.Request.PermissionMode` alanı; `agent/toolloop.go` `completeTraced` ajanın
   modunu `req.PermissionMode`'a basar (hem native hem CLI yolu için).
3. `providers/claudecli.go` `permissionModeArgs(mode)` → bayrak eşlemesi:
   `read-only`→`--permission-mode plan`, `ask`→`--permission-mode acceptEdits`,
   `auto`/boş/bilinmeyen→`--dangerously-skip-permissions`. `Complete`'te `--model`'den
   hemen sonra eklenir.
4. API: `createAgentReq`/`updateAgentReq`'e `permissionMode` alanı (`api/agents.go`).
5. Test: `providers/permission_test.go::TestPermissionModeArgs` (5 mod → bayrak eşlemesi).

✅ `go build`/`vet`/`test ./internal/...` yeşil. **Canlı uçtan-uca kanıt:** headless `claude -p`
ile temp dizinde dosya yazdırma — **bayrak yokken dosya OLUŞMADI** (eski hata doğrulandı),
**`--permission-mode acceptEdits` ile dosya OLUŞTU** (düzeltme doğrulandı).

**Aşama 2 — native (anthropic/minimax) yolda risk-sınıflı gate (2026-06-17):**
1. `tools/classify.go` (yeni) — `Risk` (`read`/`write`/`exec`) + `Classify(name)`. Bilinen
   built-in araçlar eşlenir; etkileşim/in-app araçlar (`ask_user`/`todo_write`/artifacts) =
   `read` (Explore ajanı da kullanabilsin); `write_file`/`edit_file` = `write`; `shell` = `exec`;
   **bilinmeyen + namespaced MCP araçları = `write`** (varsayılan, güvenli taraf).
2. `tools/ask.go` — `AskerFrom(ctx)` exported (agent paketinin ask kanalını onay için
   yeniden kullanması için).
3. `agent/permission.go` (yeni) — `permGate(ctx, mode, call, granted)`:
   `auto`→hepsi; `read-only`→yalnız `read`, write/exec **bloklanır**; `ask`→`read` serbest,
   write/exec **`WithAsker` ile onay sorar** (`Allow once`/`Always allow`/`Deny`; "Always allow"
   tur boyunca `granted`'da hatırlanır → tekrar sormaz). Asker yoksa (otonom koşu) write/exec
   reddedilir. `normalizePermAnswer` serbest-metin/tık yanıtını normalize eder.
4. `agent/toolloop.go` — native döngüde `reg.Call` **öncesi** `permGate`; bloklanan çağrı
   modele `IsError` tool result olarak döner (model uyum sağlar, tur düşmez) + `StepError`
   (`reason:"permission_denied"`) adımı yayılır. Tur-ömürlü `granted` haritası.
5. Testler: `agent/permission_test.go` (auto/read-only/ask×asker-yok/Always-allow-hatırlama/deny).

✅ `go build`/`vet`/`test ./internal/agent ./internal/tools ./internal/providers` yeşil. "ask"
onay istemi mevcut `StepAsk`→`AskPrompt` UI'ını yeniden kullanır (3 tıklanabilir seçenek);
mod şu an API ile (`permissionMode`) ayarlanır — composer mod seçici Aşama 3.

**Aşama 3 — UI (görsel kontrol + tur-bazlı override + varsayılan, 2026-06-17):**
1. **Ajan formu** (`AgentSettingsForm.tsx` + `types/agent.ts`): "İzin modu (araç kullanımı)"
   seçici (auto/ask/read-only) + açıklama → ajanın **kalıcı** modu (PUT `permissionMode`).
   "Nerede görürüm" sorusunun birincil cevabı.
2. **Composer** (`Composer.tsx` + `App.tsx` + `useChatStream.ts` + `api/chat.ts`): 🛡 tur-bazlı
   mod seçici + **Shift+Tab** döngü (external-agent-oss imzası); seçim localStorage'da kalıcı;
   `/api/chat(/stream)` gövdesine `permissionMode`. Backend `chat.go`+`chat_stream.go` tur başına
   ajanın yerel kopyasının `PermissionMode`'unu override eder (ThinkingLevel deseni).
3. **Settings** (`ProvidersPanel.tsx` + `types/settings.ts` + `internal/settings`): "Yeni ajan
   varsayılan izin modu" → `Settings.DefaultPermissionMode` (DTO/Patch/Default=`auto`/store);
   `api/agents.go` create önceliği: istek → uygulama varsayılanı → `auto`.

✅ `go build`/`vet`/`test` + frontend `tsc`/`vite build` yeşil. **Playwright canlı:** Ajan formu
İzin modu seçici+açıklama (Part A ✓), Composer 🛡 seçici (Part B ✓), kaydetme backend'e gitti.
Settings paneli tarayıcı aracı kararsızlığıyla bu turda görsel doğrulanamadı (kod/tsc doğrulandı).

**Kalan:** Aşama 4 — claude-cli "ask" için Interaction MCP permission-prompt aracı
(`acceptEdits` yerine gerçek tur-içi onay); (ops.) ayrı `StepPermission` kartı + oturum-ömürlü
"Always allow" kalıcılığı (şu an tur-ömürlü).

## Kararlar (2026-06-15)
- **İlk LLM sağlayıcısı:** Anthropic (Claude) ✅
- **Frontend:** React ✅

## Mevcut Durum: Faz 0–8 + Faz 7 + SDK Paritesi P2/P1 TAMAMLANDI ✅ → Backlog (Wails dahil)

> Not: Faz 8 (MCP/Tools) kullanıcı talebiyle Faz 7'den önce yapıldı; ardından Faz 7 tamamlandı.
> Sonrasında **SDK Paritesi Faz P2** (builtin fs/shell araçları) + **Faz P1** (todo_write/ask_user) + Trace `StepKind` genişletme (ask/todo/recovery) ve çok sayıda ara özellik (streaming, MiniMax, workspace switcher, otonom olay akışı) tamamlandı.
> Kalan sıra: **SDK Paritesi P3/P4 · Faz 9 Wails** ve diğer backlog kalemleri — bkz. [03-YOL-HARITASI.md](03-YOL-HARITASI.md) "Yapılacaklar / Backlog". (Connectors fazı 2026-06-16'da kapsamdan çıkarıldı.)

### `/reflect` düzeltmesi + `/compact` komutu ✅ (2026-06-16)
İki sohbet "/" komutu eklendi/düzeltildi (backend `internal/api/summary.go` + `internal/conversation/manager.go`):
1. **`/reflect` (düzeltme):** Önceden `POST /api/agents/{id}/reflect` çağırıp **hiçbir görsel sonuç** vermiyordu (kullanıcı "çalışmıyor" dedi). Artık `handleSessionSummary` `reflect` kind'ını işliyor → `Runtime.Reflect` (dream cycle) sonucu **sohbete asistan mesajı** olarak yazılıyor ("✦ Yansıma" başlığıyla). `/memory` vb. ile aynı UX.
2. **`/compact` (yeni):** Sohbeti **şimdi** sıkıştırır. `conversation.Manager.ForceCompact` token bütçesine bakmadan en eski mesajları (son `keepRecent` hariç) yuvarlanan özete katlar, kalıcılaştırır ve "N mesaj katlandı + güncel özet" raporu döner ("🗜 Sohbet sıkıştırma").
3. **Frontend:** `App.tsx` `summarize(kind)` artık serbest string alır + kind'a göre placeholder ("Yansıma üretiliyor…"/"Sıkıştırılıyor…"); `chatCommands`'a `/reflect` (düzeltildi) + `/compact` (yeni) eklendi.

> Test: go build yeşil; dev binary yeniden başlatıldı; **API canlı**: 64-mesajlı oturumda `/compact` → 56 mesaj katlandı; `/reflect` → dream-cycle yansıması döndü; Chrome: "/" menüsünde `/reflect`+`/compact` üstte listelendi. **Not:** Composer'daki bekleyen-kuyruğu (queue/steer tray, silinebilir) eşzamanlı bir çalışmayla birlikte aynı turda eklendi.

### Bugfix — HTTP access-log middleware (Loglar ekranı canlı) ✅ (2026-06-16)
**Sorun:** Kullanıcı "loglar ekranı bayadır kullanıyorum ama yeni log oluşmuyor" dedi. Ekran ve `GET /api/logs` doğru çalışıyordu (HTTP 200 + gerçek kayıtlar), ama yalnızca açılış logları + birkaç "agent reflected" görünüyordu. **Kök neden:** kod sadece **hata** durumlarında (`s.logger.Error/Warn`) ve birkaç otonom olayda slog kaydı üretiyordu; başarılı istek/sohbet/görev hiçbir log basmıyordu → aktif kullanımda yeni kayıt düşmüyordu (beklenen davranış, eksik enstrümantasyon).

**Çözüm:** `internal/api/middleware_log.go` — `withRequestLog` her API isteğini loglar (`"http request"`, attrs: `method`/`path`/`status`/`dur`); 2xx→INFO, 4xx→WARN, 5xx→ERROR. `statusRecorder` durum kodunu yakalar **ve** `Flush()`+`Unwrap()` ile `http.Flusher`'ı yeniden açar (SSE `chat/stream`+`events` `w.(http.Flusher)` assertion'ı sarmalayıcıdan geçsin diye — yoksa akış kırılırdı). `skipRequestLog` `/api/logs` (kendi buffer'ını sel etmesin) + `/api/events` (uzun-ömürlü SSE) + `/health`'i atlar. `Routes()` zinciri: `withCORS → withRequestLog → withWorkspace` (OPTIONS preflight CORS'ta erken döndüğünden loglanmaz). ✅ (`go build`/`vet` yeşil; **canlı test**: dev binary yeniden başlatıldı, `/api/agents`→INFO 200, `/api/sessions`→INFO 200, `/api/nonexistent`→WARN 404, `/api/logs` kendini loglamadı).

**Ek (aynı gün): İş-seviyesi INFO loglar.** Access-log "hangi uç çağrıldı"yı verir; üstüne "ne oldu" anlamı eklendi: `chat turn completed` (`chat.go`+`chat_stream.go` — agent/provider/model/in-out token/steps/süre, stream yolunda `stream:true`), `agent created/updated/deleted` (`agents.go`), `session created/deleted` (`sessions.go`), `task created` (`tasks.go`), `schedule created/toggled` (`schedules.go`), `mcp server toggled/tested` (`mcp.go`; başarısız test→WARN), `memory added` (`memory.go`). (Zaten vardı: heartbeat `agent tick` `worker.go`, `task run finished` `executor.go`, `flow run started/finished` `flow.go`, `agent reflected` `reflector.go`.) ✅ canlı test: agent+session create/delete dört olay da loglandı.

### Interaction MCP — Faz 2 (request_confirmation + artifacts + CLI kart paritesi) ✅ (2026-06-16)
Faz 1 üstüne araç seti genişletildi ve CLI iz paritesi sağlandı. Detay: [11-INTERACTION-MCP.md](11-INTERACTION-MCP.md) (§16).

1. **`request_confirmation`** (yeni `internal/tools/builtin_confirm.go`): riskli/geri-dönülemez eylem öncesi evet/hayır onayı; cevap `confirmed`/`denied` normalize (`NormalizeConfirmation`, TR+EN). Native registry'ye de eklendi (`toolsetup.go`); `ask_user` bloklama deseni (ortak `blockForAnswer`).
2. **`create_artifact`/`update_artifact` CLI yolunda:** `chatRun`'a per-agent **artifact sink** (`setArtifacts`, chat_stream her ajan turunda kurar); interaction backend sink'i context'e koyup mevcut tool'u çağırır (tek-kaynak).
3. **CLI iz paritesi (`agent/trace.go`):** `traceStepToTurnStep` interaction namespace'ini (`mcp__swarmgo_interaction__`) **soyar** (kartlar bare adla eşleşir) + `todo_write`'ı **`StepTodo`** kartına yükseltir → CLI yolunda da TodoCard/ArtifactCard kalıcı izde doğru render (çift kart yok, canlı emit yerine izden).
4. **Wiring:** `climcp.go` interaction allow-listesi 5 araca (`interactionToolNames`); `Tools()` + backend dispatch eşitlendi. **Keep-alive gereksiz çıktı:** claude `MCP_TOOL_TIMEOUT` varsayılanı ~28 saat → bloklayan araç pratikte timeout olmaz (+15 dk sunucu backstop).

> Doğrulama: `go build`/`vet`/`test` yeşil (yeni: confirm normalize, artifact dispatch, todo no-emit). **Canlı claude-cli testi:** todo_write→request_confirmation→(onay "Onayla")→create_artifact promptu → SSE'de TodoCard `[in_progress,pending]`, ASK `[Onayla,İptal]` → `request_confirmation→confirmed` → `create_artifact→{id}` → TodoCard `[completed,completed]` → "DONE"; `GET /api/artifacts` → `('Faz2 Test','markdown',1)` kalıcı; tool adları izde namespace'siz. **Sıradaki: Faz 3** (notify) + **Faz 4** (Codex/Gemini/Mistral Vibe adaptör — ilgili CLI'lar kurulmalı). **Not:** eşzamanlı frontend (PendingTray) + `withRequestLog` başka oturumda; commit yalnız backend Faz 2 dosyaları.

### Interaction MCP — Faz 1 (ask_user + todo_write claude-cli'ye) ✅ (2026-06-16)
Kendi agentic döngüsünü süren CLI ajanlarının (ilk hedef claude-cli) SwarmGo'nun insan-etkileşimli araçlarını kullanıp SwarmGo UI'ında yüzeyleyebilmesi için **in-process MCP-over-HTTP** sunucusu eklendi. Önceden claude-cli `-p` modunda kendi `AskUserQuestion`'ını cevaplayamıyordu → "soru penceresi hiç çıkmıyordu". Tasarım+detay: [11-INTERACTION-MCP.md](11-INTERACTION-MCP.md) (§15).

1. **Yeni `internal/interaction`** (saf protokol): MCP-over-HTTP server (`initialize`/`tools/list`/`tools/call`), bearer auth, protokol `2025-06-18`; `Backend` arayüzü token→run çözer. Birim testli.
2. **`internal/tools/interaction.go`:** `WithInteractionEndpoint`/`InteractionFrom` context köprüsü (WithAsker deseni).
3. **`internal/api/mcp_interaction.go`:** `interactionBackend` — `ask_user` (bloklayan, `run.answer` bekler) + `todo_write` (bloklamayan, canlı kart) dispatch; tool şemaları `tools` paketi tanımlarından **tek-kaynak** enumerate. `/mcp/interaction` route + `Server.SetBaseURL` (loopback self-URL). `chatRun`'a per-run **bearer token** + mutex'li `emit` (handler + MCP goroutine yarışmaz) + `done` kanalı; `byToken` lookup.
4. **CLI wiring:** `climcp.go` interaction entry'sini (`type:http`, `headers: Bearer`) **MCP kapalı olsa bile** yazar; `toolloop.go` claude-cli'yi interaction varsa MCP-delegasyon yoluna sokar; `claudecli.go` `--disallowedTools AskUserQuestion TodoWrite` + sistem-prompt notu.

> Doğrulama: `go build`/`vet`/`test` yeşil (interaction + api birim testleri: bearer reddi, initialize/list/call, ask round-trip, turn-ended, todo). **Canlı claude-cli testi** (MCP kapalı yeni ajan, ayrı port/data-dir): "önce ask_user ile renk sor" → SSE'de `ask` adımı (`[RED,BLUE,GREEN]`) → `POST /api/chat/control {answer:"BLUE"}` → CLI sonucu `BLUE` aldı → final "Your color is BLUE." Backend log: `cli mcp config written servers=1 interaction=true`. Faz 0 spike (handshake) önce ayrıca doğrulanmıştı. **Not:** eşzamanlı frontend çalışması (PendingTray vb.) başka oturumda; commit yalnız backend Faz 1 dosyalarını kapsar.

### Komut çalıştırınca komut balonu + bağlam penceresi göstergesi ✅ (2026-06-16)
1. **Komut balonu** (`api/summary.go` + `App.tsx`): `/memory`·`/board`·`/flows`·`/tools` paletten çalışınca, komutun kendisi de sohbete **kullanıcı mesajı** (`/kind`, `UserBubble` komut stilinde) olarak yazılır; ardından sonuç asistan mesajı gelir. `POST /api/sessions/{id}/summary` artık `{userMessage, replyMessage}` döndürür (ikisi de kalıcı → reload-safe). Composer her iki balonu da iyimser (optimistic) gösterir.
2. **Bağlam penceresi göstergesi** (`api/session_info.go` + `SessionDetailPanel.tsx`): `/info` artık `contextWindow` (= `maxContextTokens` sıkıştırma eşiği) döndürür. Oturum bilgisi paneline `/context` tarzı bir bölüm eklendi: kullanılan/pencere (%) başlığı + kategori-renkli **yığılmış kullanım barı** + her kategori (token+%) + **boş alan** satırı; pencere aşılırsa sıkıştırma uyarısı.

> Doğrulama: go/tsc build + API (`/summary` `{userMessage,replyMessage}`, `/info` `contextWindow=32000`) + Chrome canlı (`/board` → "⌘/board" komut balonu + özet; panel `2.5k/32.0k %8`, boş alan %92). Commit: `d03483d` + worker sweep `7f432be`.

### Composer'da tur-bazlı düşünme seviyesi seçici ✅ (2026-06-16)
Mesaj gönderme alanına, o tur için **düşünme (reasoning) seviyesini** seçtiren bir menü eklendi.
1. **UI (`Composer.tsx`):** Textarea'nın solunda `🧠` butonu (mevcut seviyeyi gösterir, seçiliyse accent). Tıklayınca üstte açılan menü: **Oto** (ajanın kendi ayarı), **Kapalı**, **Düşük**, **Orta**, **Yüksek**. Dışarı tıkla-kapat. Seçim `App.tsx`'te `thinkingLevel` state'inde tutulur ve **localStorage**'a yazılır (mesajlar/yenileme arası kalıcı). Props: `thinkingLevel` + `onThinkingLevelChange`.
2. **Akış (`api.ts`):** `streamChat`/`chatStream` artık `thinkingLevel` parametresi alıp `/api/chat/stream` gövdesine ekler.
3. **Backend (`chat.go` + `chat_stream.go`):** `chatReq.ThinkingLevel` (`"low"|"medium"|"high"|"off"`, boş = ajan ayarı). Handler, yanıtlayan ajanın **yerel kopyasının** `ThinkingLevel`'ini tur başına override eder (kalıcı değil) → mevcut `thinkingBudgetForLevel` → `req.ThinkingBudget`. Yalnız **anthropic araçsız yolda** etki eder; claude-cli/minimax `ThinkingBudget`'i yok sayar.

> Test: tsc + go build yeşil; **Chrome canlı**: seçici açıldı, "Yüksek" seçildi (accent aktif), yenileme sonrası korundu (localStorage), mesaj hatasız gönderildi. Not: backend override'ı etkin olması için sunucu yeniden başlatılmalı; görsel düşünme etkisi yalnız anthropic anahtarlı ajanlarda görülür.

### Sohbette mesaj zamanı + agent çalışma süresi ✅ (2026-06-16)
Sohbet ekranında her mesajın **gönderilme saati** ve her asistan turunun **çalışma süresi** gösterilir.
1. **Zaman yardımcıları (`lib/time.ts`):** `clockTime` ("22:48"), `fullDateTime` (hover title), `formatDuration` ("45 sn" / "2 dk 15 sn" / "1 sa 5 dk").
2. **Meta bileşenleri (`components/chat/MessageMeta.tsx`, yeni):** `MessageTime` (mesaj saati + tam tarih hover), `TurnDuration` (tamamlanan tur süresi "⏱ 2 dk 15 sn"), `LiveTimer` (akış sürerken her saniye tıklayan canlı sayaç "⏱ 0:45").
3. **MessageList bağlama (`MessageList.tsx`):** Her mesajın altına rolüne göre hizalı meta satırı. Asistan çalışma süresi ≈ `mesaj.createdAt (tur sonu) − önceki mesaj.createdAt (tur başı)`; **yalnız önceki mesaj kullanıcıysa** gösterilir (enjekte edilen /summary özetleri ve ardışık asistan turları yanıltıcı boşta-süre vermesin). Akıştaki son asistan balonunda `LiveTimer` (createdAt = tur başı), tamamlanınca `TurnDuration`. Yeni prop `streaming` (`App.tsx`'ten `streaming && streamingSessionId === activeSessionId`).

> Test: tsc temiz; **Chrome canlı**: mevcut oturumda tüm mesajlarda saat + doğru süreler (⏱ 6 sn…44 sn); enjekte özet mesajlarında süre gizlendi; yeni turda canlı sayaç 7 sn→31 sn tıkladı, bitince ⏱ 10 sn — doğrulandı.

### Oturum-bazlı akış göstergesi + buton kapsamı ✅ (2026-06-16)
Sohbet akışı (streaming) durumu artık **oturuma bağlı** — önceden tek global `streaming` bayrağı tüm oturumları etkiliyordu.
1. **Buton kapsamı fix'i (`App.tsx`):** Yeni `streamingSessionId` state'i akışın hangi oturuma ait olduğunu izler. Composer'a geçen prop `streaming && streamingSessionId === activeSessionId` oldu; `AskPrompt` de aynı koşula bağlandı. Böylece A oturumunda yanıt üretilirken B oturumuna geçince buton yanlışça "Durdur/Kes/Yönlendir" yerine doğru şekilde **"Gönder"** gösterir. (Akış başlangıcında set, `finally`/`stopTurn`'de temizlenir.)
2. **Sidebar canlı göstergesi (`SessionsSidebar.tsx`):** Akışı süren oturum satırında **nabız atan nokta** (`animate-ping`) + alt satırda **"yazıyor…"** etiketi gösterilir (zaman/mesaj sayısı yerine); başlık kalınlaşır. Gösterge yalnız ilgili oturumda görünür, oturum değiştirince akıştaki oturumda kalır. Prop: `streamingSessionId={streaming ? streamingSessionId : null}`.
3. **Renk iyileştirmesi (sonradan, aynı gün):** Gösterge daha görünür olsun diye nokta + "yazıyor…" etiketi koyu mor `--color-accent` yerine **parlak açık yeşil `emerald-400`** yapıldı; etiketteki `opacity-60` soluklaştırması bu durumda kaldırıldı.

> Test: tsc + go build/vet yeşil; **Chrome canlı**: A oturumunda akış → satırda "yazıyor…" + ping nokta (emerald-400); B'ye geçince buton "Gönder", A bitince gösterge kayboldu — doğrulandı.

### Komutlar ekranında salt-okunur prompt görüntüleyici ✅ (2026-06-16)
Ayarlar → **Komutlar** kategorisine, komutların arkasındaki gömülü promptları **salt-okunur** gösteren bir bölüm + **klasörü aç** butonu eklendi:

1. **Backend** (`internal/agent/prompts.go`): `Prompts()` summary/reflect/title promptlarını (system + user-turn şablonu + not + kaynak dosya adı) döndürür; `PromptsDir()` `runtime.Caller` ile prompt kaynak klasörünü (yerel derlemede `internal/agent`) verir. `GET /api/prompts` (`internal/api/prompts.go`) bunları + `dir`'i döner; `POST /api/prompts/reveal` klasörü Explorer'da açar (session-reveal ile aynı `explorer.exe` deseni). Route'lar `registerSettingsRoutes`'ta.
2. **Frontend** (`SettingsPanel.tsx`): Komutlar kategorisi eski komut-kartı görünümünü **korur**; her kart artık **açılır-kapanır** (▸/▾). Açılınca o komuta ait prompt (System/User turn `<pre>` blokları, salt-okunur), kaynak dosya rozeti ve **📂 Klasörü aç** butonu (`api.revealPrompts()`) kartın içinde görünür. `/tools` deterministik olduğundan "prompt yok" notu gösterir. Auto-title komut olmadığından en altta ayrı (kesik çizgili) açılır kart olarak durur. Ortak `PromptDetails` bileşeni. Düzenleme yok — promptlar binary'ye gömülü.

> Doğrulama: `go build ./...` + `tsc` yeşil; `GET /api/prompts` 3 prompt + doğru `dir` döndü; Chrome canlı: Komutlar ekranında promptlar + klasör yolu + "Klasörü aç" butonu render, butona basınca backend hatasız Explorer açtı.

### Extended thinking native parite (streaming + Complete) ✅ (2026-06-16)
Extended reasoning artık **native (anthropic) yolda** da uçtan uca yüzeyleniyor — önceden yalnız claude-cli `thinking` bloğunu dolduruyordu; native `Stream` sadece `text_delta` ayrıştırdığından düşünme kayboluyordu.

1. **`providers/provider.go`:** `Streamer` callback'i `func(string)` → tipli `func(StreamDelta)` (yeni `StreamDelta{Kind,Text}` + `DeltaText`/`DeltaThinking` sabitleri).
2. **`providers/anthropic.go`:** `Stream` SSE `content_block_delta`'da artık `text_delta` **ve** `thinking_delta` parse eder, tipli delta ile yayar; tam thinking metni `Response.Trace`'e `thinking` adımı olarak konur. `Complete` da `thinking` content-block'unu `Response.Trace`'e parse eder.
3. **`providers/minimax.go`:** `Stream` yeni imzaya uyduruldu (thinking yok → hep `DeltaText`).
4. **`agent/toolloop.go`:** `recordedStream` thinking'i sabit `liveThinkingID` ile canlı `StepThinking` akıtır; stream yolu artık `traceToSteps(resp.Trace)` döndürür → thinking **kalıcılaşır** (önce `nil` dönüp reload'da kayboluyordu).
5. **`frontend/src/App.tsx`:** canlı thinking delta'larını `id`'ye göre tek büyüyen `ThinkingBlock`'a merge eder (tool_delta deseni).
6. **Test (yeni):** `providers/stream_test.go` → `TestAnthropic_StreamSurfacesThinking` (thinking+text delta ayrımı + `Response.Trace` kalıcılığı); mevcut stream testleri yeni imzaya güncellendi.

Bütçe ajanın `ThinkingLevel`'inden (`thinkingBudgetForLevel`: low=2048/medium=8192/high=16384) yalnız **araçsız (MCP kapalı)** turlarda gönderilir. Native **tool** döngüsünde thinking hâlâ kapalı (imzalı blok geri-besleme gerektirir). ✅ `go build`/`vet`/`test ./...` + frontend `tsc --noEmit` temiz. Detay: [07-CHAT-UX.md](07-CHAT-UX.md).

### D2 — Provider retry middleware + C1 — Sistem-prompt cache sınırı ✅ (2026-06-16)

İki backlog maddesi tamamlandı: geçici hatalara dayanıklı sağlayıcı çağrıları (D2) ve
prompt-cache'i etkili kılan statik/dinamik sistem-prompt bölümlemesi (C1).

**D2 — Retry middleware (`internal/providers/transport.go`)**
- [x] `retryPolicy` (varsayılan 4 deneme, 500ms taban, 8s tavan) + `httpRetry` paket var'ı
  (testler hızlı/no-retry politikayla değiştirir).
- [x] `doWithRetry(ctx, client, prefix, build)` — her denemede isteği yeniden kurar (gövde
  tüketildiği için), **üstel backoff + ±%25 jitter**, `Retry-After` başlığına saygı,
  context iptaline duyarlı. Son denemede yanıtı (retryable status olsa bile) ya da sarılmış
  ağ hatasını **olduğu gibi** döndürür → çağıran sağlayıcının kendi mesajını/gövdesini sunar.
- [x] Retryable: ağ hatası + `408/429/500/502/503/504/529` (529 = Anthropic "overloaded").
- [x] **Retry görünürlüğü (2026-06-16):** her retry öncesi `slog.Warn("provider transport retry", provider/attempt/maxAttempts/reason/backoff)`; `main.go` artık `slog.SetDefault(logger)` çağırdığından bu uyarılar logbuf ring buffer'ına düşer → **Loglar ekranında** (`/api/logs`) görünür. Canlı doğrulandı: MiniMax flaky sunucuya (503→503→200) yönlendirildi, sohbet başarıyla döndü, loglarda `attempt:1 backoff:589ms` + `attempt:2 backoff:801ms reason:HTTP 503` yakalandı.
- [x] `postJSON` **ve** `postSSE` artık `doWithRetry`'den geçer (SSE yalnız ilk bağlantıyı
  yeniden dener → akış başladıktan sonra kısmi çıktı tekrarlanmaz). İmzalar değişmedi →
  anthropic/minimax çağrıları dokunulmadan retry kazandı.

**C1 — Statik/dinamik sistem-prompt bölümleme**
- [x] `providers.Request`: `System` (STATİK prefix: persona + kullanıcı profili — turlar arası
  sabit) + yeni `SystemDynamic` (VOLATİL suffix: recall edilen bellek + konuşma özeti — her tur değişir).
- [x] `anthropic.go` `systemField(static, dynamic)`: extended-cache açıkken `cache_control`
  breakpoint'i **yalnız statik blokta** (1h TTL) → araç tanımları + statik prefix cache'lenir,
  her tur değişen dinamik suffix cache'i geçersiz kılmaz. Cache kapalıyken düz birleştirme.
  (Statik boşsa breakpoint dinamiğe düşer → eski tek-blok davranışı korunur.)
- [x] `minimax.go` + `claudecli.go`: cache breakpoint'i olmadığından `System`+`SystemDynamic`
  birleştirilir. **Önemli:** claude-cli `--append-system-prompt` artık her ikisini birleştirir
  → belleğin anahtarsız yolda düşmesi (regresyon) engellendi.
- [x] Çağıranlar (`api/chat.go`, `chat_stream.go`, `agent/executor.go`, `flow.go`): bellek + özet
  artık `System`'e değil `SystemDynamic`'e konur; persona + profil statik kalır. `Runtime.complete`
  imzası `(system, systemDynamic, prompt, ...)` oldu.

**CANLI TEST (unit + API):**
- [x] `transport_test.go`: 503→503→200 retry başarısı (3 çağrı), kalıcı 503'te denemeler bitince
  son yanıt+gövde döner, non-retryable 400 hiç denenmez, `retryableStatus`/`parseRetryAfter` tablo.
- [x] `anthropic_test.go`: cache breakpoint sadece statik blokta; statik-yok→dinamik cache'lenir;
  cache kapalı→düz birleşim; boş→nil.
- [x] **Canlı API (claude-cli, anahtarsız):** ajana "BLUEFALCON" hafıza belgesi eklendi →
  "What is the secret project codename?" sorusuna **"BLUEFALCON"** yanıtı → recall'ın `SystemDynamic`
  üzerinden claude-cli'a ulaştığı uçtan uca doğrulandı (C1 anahtarsız yolu bozmuyor).
- [x] `go build/vet/test ./...` temiz.

> Not: 1M-context + uzatılmış cache yalnız **anthropic**'te etkili; retry üç sağlayıcıyı da kapsar
> (ortak transport). claude-cli kendi HTTP'sini yaptığından retry yalnız native (anthropic/minimax)
> çağrıları + MCP listelemeyi etkiler.

### Talep-üzerine özetler + "/" komut paleti ✅ (2026-06-16)

Sohbet composer'ındaki `/` komut paletine **workspace verisini özetleyen** dört komut
eklendi: ajan hafızası, görev panosu, akışlar ve araç listesi. Sonuç, oturuma normal bir
asistan mesajı olarak (başlıkla) yazılır.

**Backend**
- [x] `agent/summarizer.go` (yeni): `Runtime.Summarize(ctx, agentID, kind)` — `memory`/`board`/
  `flows` türleri ilgili veriyi toplar (`gatherSummaryData`, her tür max **40 kayıt**, kayıt
  başına 200 rune cap) ve **ucuz bir modele** özetletir (yapılandırılmışsa başlık-modeli override'ı,
  yoksa ajanın kendi modeli; `guardedComplete` ile bütçeye dahil). `tools` türü **deterministik**
  (`toolsOverview`, model çağrısı yok) — ajanın efektif araç kataloğunu açıklamalarıyla listeler.
- [x] `api/summary.go` (yeni): `POST /api/sessions/{id}/summary` (`{kind}`) → `Summarize` çağırır,
  türe özel başlık ekler (🧠 Hafıza / 🗂 Görev panosu / 🔀 Akışlar / 🔌 Araçlar), sonucu
  `AddMessage` ile oturuma yazıp mesajı döner. Bilinmeyen tür → 400.
- [x] `server.go`: `POST /api/sessions/{id}/summary` route'u kaydedildi.

**Frontend**
- [x] `App.tsx` `summarize(kind)` callback'i: backend'i çağırır, üretim sırasında geçici placeholder
  gösterir, dönen asistan mesajını sohbete ekler. `/` komut paletine 4 komut (`api.ts`/`types.ts`).

**CANLI TEST (API):**
- [x] `tools` türü: ajanın efektif araç kataloğu deterministik markdown listesi olarak döndü (model
  çağrısı yok); MCP kapalı ajanda "araçlar kapalı" mesajı.
- [x] `memory`/`board`/`flows`: boş veride "_(boş — özetlenecek … yok)_"; dolu veride ucuz model özeti.
- [x] `go build/vet/test ./...` + frontend `tsc --noEmit` temiz.

### İki-seviyeli araç yönetimi (workspace + ajan) ✅ (2026-06-16)

Araç (tool) yönetimi ajan-bazlıdan **iki seviyeli** bir modele çevrildi: workspace
geneli aktivasyon + ajan-bazlı seçim.

**Model**
- **Workspace seviyesi**: `db.WorkspaceToolConfig{DisabledTools []string}` — store kökünde
  `tools-config.json` singleton (denylist; listelenmeyen araç = aktif → yeni MCP araçları
  otomatik aktif gelir). `store_tools.go` (`GetWorkspaceToolConfig`/`SetWorkspaceToolConfig`/
  `loadToolConfig`), `db.go` `toolConfig` alanı + load.
- **Ajan seviyesi**: mevcut `allowed_tools` allowlist korundu (boş = tüm **workspace-aktif** araçlar).

**Backend**
- [x] `agent/toolsetup.go`: `workspaceDisabledSet` + `toolFilter(ctx,agent)` (workspace denylist ∩
  ajan allowlist birleşik predicate). Katalog metodları: `WorkspaceToolCatalog` (tümü, ajan-agnostik),
  `ActiveToolCatalog` (workspace-aktif = ajan seçeneği), `ToolCatalog(agent)` (efektif). `toolloop.go`
  `req.Tools = reg.Defs(r.toolFilter(ctx,agent))`.
- [x] `api/workspace_tools.go` (yeni): `GET /api/workspace-tools` (tüm katalog + `enabled` bayrağı +
  `disabledTools`), `PUT /api/workspace-tools` (`disabledTools` denylist'i yaz). `agent_tools.go`
  katalog kaynağı `ActiveToolCatalog` (ajanın seçebileceği = workspace-aktif).

**Frontend**
- [x] `ToolsPanel.tsx` artık **workspace-geneli**: tüm araçlar aç/kapat toggle'larıyla (anında kaydeder)
  + MCP sunucu yönetimi; ajan bölümü kaldırıldı. App'te `<ToolsPanel onError>` (ajan prop'u yok).
- [x] `AgentToolsSection.tsx` (yeni): ajanın kullanacağı araçları workspace-aktif kataloğundan seçer
  (master "araç kullan" + per-tool checkbox + "Hepsi"/"Hiçbiri"; anında kaydeder). `AgentSettingsForm`'a
  gömüldü → hem Ajanlar ekranı sağ paneli hem roster ⚙ modalı paylaşır.
- [x] `types.ts`/`api.ts`: `WorkspaceTool`/`WorkspaceTools` + `workspaceTools()`/`setWorkspaceTools()`.

**CANLI TEST (API + unit):**
- [x] `GET /api/workspace-tools`: 14 araç (shell dahil, `SWARMGO_ENABLE_SHELL=1`); `PUT` ile shell+http_get
  deaktif → re-fetch `enabled` bayrakları doğru; `store/tools-config.json` diske yazıldı.
- [x] Ajan kataloğu workspace-aktif **12 araç** döndü (deaktif shell/http_get hariç); alt küme allowlist
  (`read_file,grep,get_current_time`) kaydedildi.
- [x] Unit (`tooltier_test.go`): workspace denylist tam kataloğu etkilemeden aktif kataloğu filtreler;
  ajan efektif seti = workspace-aktif ∩ ajan allowlist (workspace-deaktif araç allowlist'te olsa bile düşer).
- [x] `go build/vet/test ./...` + frontend `tsc --noEmit` temiz.

> Not: claude-cli yolu sunucu-bazlı `--allowedTools` ile kendi döngüsünü sürdüğünden per-tool filtre
> native (anthropic/minimax) yola + katalog/MCP listesine uygulanır (SDK parite deseni). Tarayıcı DOM
> testi sıradaki doğrulama adımı (API + unit ile çekirdek mantık doğrulandı).

### Faz A1.1 — Artifact: manuel düzenleme + sohbet bağlamına enjeksiyon ✅ (2026-06-16)

Artifact sistemine iki ek yetenek:

1. **Panelden manuel düzenleme / yeni sürüm / yeni oluşturma** (`ArtifactsPanel.tsx`): görüntüleyici header'ında **✎ Düzenle** → içerik `textarea` + başlık/tür/dil alanları + değişiklik notu, **Kaydet/İptal**. İçerik değişince `PUT /api/artifacts/{id} {content,note}` yeni **sürüm** açar (eski sürüm `revisions`'a arşivlenir); yalnız başlık/tür değişince meta güncellenir, **sürüm açılmaz** (seçici patch). Liste header'ında **+ Yeni** → boş artifact oluşturup doğrudan düzenleme moduna girer.
2. **Sohbet bağlamına artifact enjeksiyonu** (`api/artifacts.go` `artifactsContextBlock` → `chat.go` + `chat_stream.go` `dynamic` suffix): oturumdaki mevcut artifactlar (id/başlık/tür/sürüm) sistem promptunun dinamik (cache'siz) kısmına "## Artifacts in this session" bloğu olarak eklenir → ajan `update_artifact` ile **id üzerinden** revize eder, kopya oluşturmaz. Boş oturumda blok yok; session-scoped (başka oturumun artifactları sızmaz).

**CANLI TEST (API + unit):**
- [x] UI API yolu: Yeni→v1(boş içerik), içerik düzenle→**v2**+1 revizyon, yalnız-başlık→**v2 kalır** (sürüm açmadı), delete=200.
- [x] `artifacts_test.go::TestArtifactsContextBlock`: blok id/başlık/`update_artifact` içerir, boş oturumda boş, başka oturumu sızdırmaz — **PASS**.
- [x] `go build/vet/test ./...` + frontend `tsc --noEmit` + `vite build` temiz.

> Not: Chrome/Playwright canlı UI testi bu turda yapılamadı (mcp-gateway bağlantısı kopuktu); doğrulama UI'ın yaptığı **tam API çağrıları** + Go unit testi + temiz derleme üzerinden yapıldı. UI bileşeni tsc/build'den temiz geçiyor.

### Faz A1 — Artifact sistemi ✅ (2026-06-16)

Ajanların ürettiği önemli, bağımsız içerik (doküman/kod/HTML/SVG/Mermaid) **versiyonlanarak** saklanır ve ayrı bir ekranda görüntülenir — Claude.ai artifacts benzeri.

**Backend**
- [x] **Depolama** (`internal/db`): `models_artifact.go` (`Artifact`: `SessionID`/`AgentID` köken, `Kind`, `Language`, `Content`, `Version`, inline `Revisions[]` geçmiş), `store_artifact.go` (Create / Get / List(session filtreli) / `UpdateArtifactContent`[eski sürümü `Revisions`'a arşivler + `Version++`] / `UpdateArtifactMeta` / Delete), `db.go` (`artifacts` map + `dirArtifacts` + load).
- [x] **Üretim araçları** (`internal/tools`): native (anthropic/minimax) yola `create_artifact` + `update_artifact` (`builtin_artifact.go`); `artifact.go` `ArtifactSink` arayüzü + `WithArtifacts`/`artifactsFrom` context köprüsü (ask_user deseni, import-cycle'dan kaçınmak için `tools` paketinde). Araç çıktısı JSON ref (`{id,title,kind,version,action}`).
- [x] **Wiring** (`internal/agent/toolsetup.go` + `internal/api`): `buildRegistry` iki aracı kaydeder; api `artifactSink` (session+agent damgalı) `chat.go` + `chat_stream.go`'da `WithArtifacts` ile turuna bağlanır. İnteraktif olmayan koşularda sink yok → araç hata döner.
- [x] **API** (`api/artifacts.go` + `registerArtifactRoutes`): `GET/POST /api/artifacts`, `GET/PUT/DELETE /api/artifacts/{id}`.

**Frontend**
- [x] `types.ts`/`api.ts`: `Artifact`/`ArtifactKind`/`ArtifactRevision`/`ArtifactRefResult` + artifact CRUD metodları.
- [x] NavRail "Artifactlar" view (`FileCode` ikonu) + `App.tsx` `artifactTarget` deep-link state + `openArtifact`.
- [x] `ArtifactsPanel.tsx`: liste (tür rozeti + sürüm + zaman) + sürüm seçicili görüntüleyici + kopya/sil/kaynağa-git.
- [x] `artifacts/ArtifactView.tsx`: kind→renderer (markdown→`Markdown`, code→`CodeBlock`, html→sandbox'lı `iframe srcDoc`, svg→inline, mermaid→kaynak, text→`pre`).
- [x] `chat/ArtifactCard.tsx`: `create_artifact`/`update_artifact` tool adımı → tıklanabilir kart (`TurnSteps` özel-durumu) → `onOpenArtifact` ile artifact ekranına atlar.

**CANLI TEST (API + Playwright):**
- [x] API CRUD: manuel create → list/get/filtre(sessionId) → update (içerik) **v1→v2**, eski sürüm `revisions`'a arşivlendi; 404 doğru.
- [x] Tool kataloğu: `mcpEnabled` ajanda **13 araç** (`create_artifact`+`update_artifact` dahil).
- [x] Kalıcılık: tek backend restart'ında **3 artifact diskten reload** edildi (round-trip).
- [x] Playwright (5174→test backend :8095 proxy): "Artifactlar" view, 3 tür liste, markdown/kod render, **HTML sandbox iframe** (`srcDoc`), sürüm v2/v1 seçici — DOM doğrulandı.
- [x] `go build/vet/test ./...` + frontend `tsc --noEmit` temiz.

> Not: claude-cli yolu kendi tool döngüsünü `--mcp-config` ile sürdüğünden bu built-in araçları kullanmaz (SDK parite deseniyle bilinçli). Gerçek LLM-tetikli artifact üretimi anthropic/minimax anahtarı gerektirir; API + sink yolu doğrudan doğrulandı.

### Tırnakla komut kaçışı + Komutlar referans ekranı ✅ (2026-06-16)
İki küçük UX iyileştirmesi:

1. **Tırnak içine alınan komut = düz metin** (`components/chat/UserBubble.tsx`): Bir mesaj `/` ile başlayan bir komutu **tırnak içinde** içeriyorsa (`"/komut"`, `'/komut'`, `` `/komut` ``, akıllı tırnaklar dahil), komut stili (mono + ⌘ + kenarlık) **uygulanmaz**; tırnaklar soyulur ve içerik normal balonda düz metin olarak gösterilir. Böylece bir komuttan *bahsetmek* mümkün. `quotedCommand()` yardımcısı + `QUOTE_PAIRS` haritası. Composer paleti zaten `/` ile başlamayan girişlerde açılmadığından tırnaklı giriş paleti de tetiklemez.
2. **"Komutlar" ayar ekranı** (`SettingsPanel.tsx`): Ayarlar'da yeni `commands` kategorisi (⌘ "Komutlar") tüm slash komutlarını ikon + `/ad` + açıklama ile **salt-okunur** listeler. Komut listesi App'ten `commands={chatCommands}` prop'u ile geçirilir; bilgilendirme kutusunda tırnakla kaçış da anlatılır. Bu kategori için Kaydet butonu gizli (referans ekranı).

> Doğrulama: kendi dosyalarımda tsc temiz. Not: Aynı ağaçta paralel **artifacts** WIP'i (ArtifactsPanel) henüz bağlanmadığından `tsc -b` o dosyalarda kırmızı; commit ağaç yeşile dönünce yapılacak.

### Sohbet sidebar = oturum listesi + okundu/okunmadı ✅ (2026-06-16)
Sohbet sol paneli ajanlardan arındırıldı; oturum-merkezli hale geldi (Playwright ile canlı test edildi):

1. **Sadece oturumlar**: Sohbet sidebar yalnızca oturumları gösterir (`SessionsSidebar.tsx`). Ajan roster'ı `AgentRoster.tsx`'e taşındı (Hafıza/Araçlar sidebar'ı). **Ajanlar** NavRail view'i **iki-panelli** (`AgentsView.tsx`): solda roster (★ ile varsayılan seç), ajana tıklayınca sağda ayarları düzenlenir; form ortak `AgentSettingsForm.tsx`'e çıkarıldı (`AgentsView` sağ paneli + `AgentSettingsModal` roster ⚙'i paylaşır). Eski `Sidebar.tsx` silindi. **Ajan silme** (2026-06-16): sağ panel footer'ında 🗑 Sil → `DELETE /api/agents/{id}` (`db.DeleteAgent` ajan + sahip oturumları siler, `Runtime.Stop` worker'ı durdurur), onaylı; Playwright ile test edildi (geçici "SilTest" ajanı oluşturulup silindi, roster + backend'den kalktı).
2. **Zaman gruplama + sıralama**: oturumlar `updatedAt` desc (backend zaten böyle) + **Bugün / Dün / Geçen hafta / Geçen ay / Daha eski** kovaları (`lib/time.ts`: `bucketOf` takvim-günü bazlı, `relativeTime`). Her satırda relative zaman + mesaj sayısı.
3. **`updatedAt`**: `AddMessage` her mesajda (kullanıcı turu başı + ajan yanıtı) `UpdatedAt` basıyor → oturum otomatik en üste.
4. **Oturum ⚙ menüsü**: Başlığı düzenle (inline → `POST /api/sessions/{id}/title {title}`), AI ile başlık, Yolu kopyala (`GET .../path`), Klasörü aç (`POST .../reveal` → Explorer), Sil (`DELETE /api/sessions/{id}`, onaylı).
5. **Okundu/okunmadı** (`Session.Unread`): `AddMessage` assistant'ta `Unread=true`; `MarkSessionRead` temizler. Oturum açılınca + aktif tur bitince okundu; açık değilken gelen yanıt **nokta + kalın** ile okunmadı. Olay feed'i aktif workspace'te listeyi tazeler.

> Doğrulama: build/vet + tsc yeşil; Playwright: sessions-only + Bugün/Dün gruplama + zamanlar, ⚙ menü (manuel rename uygulandı), Ajanlar view; API: path doğru + `/api/chat` sonrası `unread=true` + en üste taşındı + frontend nokta/kalın render.
> Not: Aynı çalışma ağacında eşzamanlı **artifacts** + **tema presetleri** çalışması var; commit ayrımı kullanıcı onayına bırakıldı (bkz. oturum sonu özeti).

### Workspace switcher iyileştirmeleri ✅ (2026-06-16)
Sol-üst workspace seçici elden geçirildi (Playwright ile canlı test edildi):

1. **Dışarı tıkla-kapat** (`WorkspaceSwitcher.tsx`): `mousedown` dinleyicisi + `rootRef`; panel dışına tıklayınca kapanır.
2. **Boş-isim sağlamlığı**: ad `||` ile gösterilir (önceki `??` boş string'i geçiriyordu); listede fallback "İsimsiz".
3. **Popup ile oluşturma** (`WorkspaceCreateModal.tsx`): ad + emoji simge paleti + opsiyonel **veri klasörü**. Klasör: `POST /api/pick-folder` (Windows native FolderBrowserDialog) "Gözat" butonu **veya** elle yol. Backend `Manager.Create(name, parentPath)` + `Meta.Path` (boş=varsayılan; özel yolda `{path}/swarmgo-{id}` alt klasörü → silme komşu içeriği bozmaz).
4. **Varsayılan ajan**: `handleCreateWorkspace`→`seedDefaultAgent` yeni workspace'e "Asistan" ajanı ekler (provider önceliği ws→app→claude-cli).
5. **Çapraz-workspace etkinlik rozeti**: aktif olmayan workspace'te olay (görev/zamanlama/heartbeat **+ sohbet tamamlanma**) olunca switcher'da nokta belirir. `chat_stream` `done`'da `Runtime.Emit` (yeni exported) `chat` olayı yayar (yalnız rozet). `App.tsx` `unreadWs` Set olay feed'inden dolar, geçince temizlenir; `NavRail` collapsed + switcher tetik/satır nokta.
6. **Sağlamlık fix**: `withWorkspace` bilinmeyen `X-Workspace-Id`'de 400 yerine Varsayılan'a düşer (silinmiş id uygulamayı kilitlemez).

> Doğrulama: build/vet/test + frontend tsc yeşil; Playwright canlı: dışarı-tıkla-kapat, modal oluşturma (ikon 🚀 + ad), varsayılan ajan beliriş, çapraz-ws rozet beliriş→geçişte temizleniş, konsol temiz.

### Tıklanabilir bildirimler + otonom olay akışı ✅ (2026-06-16)
Masaüstü bildirimleri artık hedefe **deep-link**'lenir; otonom olaylar (heartbeat/task/schedule) backend'den frontend'e akar.

1. **Frontend bildirim navigasyonu** (`frontend/src/lib/clientPrefs.ts`): `notify(...)` opsiyonel `onClick` alır → `window.focus()` + navigasyon. Sohbet yanıt-hazır bildirimi kaynak sohbete (`setView('chat')`+`selectSession(sid)`), sohbet hatası loglara (`setView('logs')`) atlar (`App.tsx`).
2. **`internal/events` (yeni):** `Event` (type/level/workspaceId/title/body/target/time) + `Bus` (süreç-geneli pub/sub, nil-safe, slow-subscriber drop).
3. **`GET /api/events` SSE** (`internal/api/events.go`): aboneye `notify` olayları akıtır; 25sn ping keep-alive; global (workspace-scoped değil).
4. **Yayın noktaları:** `Runtime` artık `bus`+`wsID`+`wsName` taşır (`NewRuntime`/`NewManager`/`NewServer` imzaları güncellendi, `main.go` `events.NewBus()` enjekte eder). Heartbeat hatası/auto-disable (`worker.tick`→`Runtime.emitHeartbeatFailure`, target=logs), görev bitti/başarısız (`executor.RunTask`→`publish`, target=board+taskId), zamanlanmış prompt teslimi (`scheduler.emitPromptDelivery`: başarı→chat+sessionId, hata→logs). Görev olayları tüm tetikleyiciler için yayılır (frontend görünürken bastırır).
5. **Frontend abonelik** (`api.subscribeEvents`, EventSource oto-reconnect; `App.tsx` `onEventRef` taze closure + tek-mount `useEffect`): olay gelince `notify` ile bildirim, tıklayınca gerekirse workspace değiştirip `target.view`/`sessionId`'e gider.
6. **Doğrulama:** `go build`/`vet`/`test ./...` yeşil + frontend `tsc --noEmit` temiz + **canlı SSE smoke**: anahtarsız görev çalıştırma → `event: notify` `{type:task,level:error,target{view:board,taskId},workspaceId,...}` uçtan uca yakalandı.

### Kullanıcı mesajında @mention / komut stili ✅ (2026-06-16)

Gönderilen kullanıcı mesajları artık `@mention` ve `/komut` içerdiğinde farklı render edilir.

- [x] `components/chat/UserBubble.tsx` (yeni): kullanıcı balonunu render eder. `@AjanAdı` token'ları **çip** olarak vurgulanır (eşleşen ajanın avatar rengiyle; eşleşmezse yarı-saydam beyaz). Mention içeren balona `ring-1 ring-white/40` halka. `/` ile başlayan komut-mesajı **mono yazı tipi + ⌘ + accent-soft kenarlık** ile ayrı stilde.
- [x] `MessageList.tsx`: kullanıcı dalı düz `{m.text}` yerine `<UserBubble text agents>` kullanır.

**CANLI TEST (Chrome DOM):**
- [x] "@Reminder kısa selam ver" → balon `ring-1`, `@Reminder` çip span'i; mention Reminder'a yönlendi ("Selam! 👋").
- [x] "/deneme …" → `font-mono` + ⌘ + accent-soft kenarlıklı komut balonu.
- [x] `tsc + vite build` temiz.

### Akış müdahalesi: Durdur / Sıraya / Kes / Yönlendir (steering) ✅ (2026-06-16)

Token akarken kullanıcı turu **canlı kontrol edebiliyor**. Composer butonları streaming durumuna göre değişir.

**Backend (kontrol kanalı):**
- [x] `api/chat_control.go` (yeni): `chatRuns` kayıt defteri (runId → {cancel, steer chan}); `POST /api/chat/control` (`action: stop|steer`).
- [x] `api/chat_stream.go`: tur başına `runId` üretir, **cancelable ctx** + steer chan kaydeder, `meta` event'ine `runId` ekler, ctx'i `agent.WithSteer` ile zenginleştirir.
- [x] `agent/steer.go` (yeni): `WithSteer` ctx + `drainSteer`. `toolloop.go`: native tool döngüsünde her iterasyon başında steer mesajlarını çeker → `[Canlı kullanıcı yönlendirmesi] …` user mesajı olarak ekler + iz adımı (`↪ Yönlendirme`). (Düz/araçsız turda mid-turn etkisizdir; tool döngüsünde etkili.)
- [x] `server.go`: `runs *chatRuns` alanı + route.

**Frontend:**
- [x] `api.ts`: `chatStream` `onMeta`'ya `runId`; `chatControl(runId, action, text)`.
- [x] `App.tsx`: `streaming` state + `AbortController` (stop/interrupt) + `runIdRef` (steer) + `queuedRef` (done'da otomatik gönder); `stopTurn`/`interruptTurn`/`queueMessage`/`steerTurn`. Stop'ta kısmi balon korunur (abort error gizlenir).
- [x] `Composer.tsx`: **akış yok** → Gönder; **akış + boş input** → 🔴 Durdur; **akış + dolu input** → Sıraya / Kes / Yönlendir. Akarken Enter = Sıraya.

**CANLI TEST (API + Chrome):**
- [x] Stop deterministik: SSE'den `runId` alındı → `POST /api/chat/control stop` → tur **+1.5sn**'de ctx-iptal ile sonlandı (`error` event).
- [x] Control validasyon: bilinmeyen run → 404.
- [x] Chrome: mesaj gönderildi → akış + yanıt geldi (uçtan uca streaming UI çalışıyor).
- [x] `go build/vet ./...` + `tsc + vite build` temiz.

> Not: Yönlendir (steer) anlamlı etkiyi **araç kullanan (agentic) turlarda** gösterir; düz tek-atış sohbette mid-turn enjekte edilemez. Stop/Interrupt/Queue her ajanda çalışır.

### Faz P2 — Built-in dosya/shell araçları (SDK paritesi) ✅ (2026-06-16)

Native (anthropic/minimax) tool-use yolundaki ajanlara **yerleşik dosya sistemi araçları** ve (opsiyonel, varsayılan kapalı) **shell** aracı eklendi. Tümü workspace'in `workspace/` alt dizinine **sandbox**'lanır (path-traversal koruması). Detaylı tasarım: [09-CLAUDE-AGENT-SDK.md](09-CLAUDE-AGENT-SDK.md) (Faz P2).

- [x] **Sandbox** (`internal/tools/sandbox.go`): `Sandbox{Root}` + `Resolve` — mutlak yol reddi, `..` kaçış reddi, kök-altı doğrulaması; `Rel` (görüntüleme için).
- [x] **FS araçları** (`internal/tools/builtin_fs.go`): `read_file` (256KB cap), `write_file` (dizin oluşturur), `edit_file` (tam string değişimi; tekil/`replace_all`), `list_dir` (dizinler önce), `glob` (`**`/`*`/`?` → RE2; 500 cap), `grep` (RE2 + opsiyonel glob filtre; ikili dosya atlama; 200 cap).
- [x] **Shell aracı** (`internal/tools/builtin_shell.go`): Windows'ta `powershell.exe -NoProfile -NonInteractive`, diğerinde `/bin/sh -c`; cwd = sandbox kökü; timeout (varsayılan 30s, max 120s); çıktı 64KB cap. **Yüksek riskli → varsayılan kapalı**, `SWARMGO_ENABLE_SHELL=1` ile açılır (permission katmanı P3 gelene dek opt-in).
- [x] **Wiring**: `Runtime.workDir` alanı (`NewRuntime` parametresi); `workspace.Manager.open` her workspace için `filepath.Join(dir,"workspace")` geçirir; `Tunables.shellEnabled` (+ `SetShellEnabled`/`ShellEnabled`); `main.go` env'den okur; `agent/toolsetup.go buildRegistry` sandbox hazırsa fs araçlarını, gate açıksa shell'i kaydeder.
- [x] **Test** (`internal/tools/builtin_fs_test.go`): sandbox resolve (in-bounds/escape/absolute/not-ready), write→read→edit round-trip, non-unique edit reddi, list/glob/grep, glob-restricted grep, `globToRegexp` tablo testi.

**CANLI TEST (build + unit + API):**
- [x] `go build/vet ./...` + `go test ./...` tamamı temiz (yeni fs testleri dahil).
- [x] API: ajan oluştur → `mcpEnabled=true` → `GET /api/agents/{id}/tools` kataloğu **10 araç** döndü (3 built-in + 6 fs + shell, `SWARMGO_ENABLE_SHELL=1` ile).
- [x] Shell gate doğrulandı: env olmadan katalogda `shell` yok.

> Mimari not: Bu araçlar **native** tool-use döngüsünde (`agent/toolloop.go`) çalışır. **claude-cli** yolu kendi döngüsünü `--mcp-config` ile sürdüğünden (ve kendi Read/Write/Bash araçları olduğundan) bu built-in'leri kullanmaz — SDK parite tasarımıyla bilinçli uyum. Gerçek LLM-tetikli yürütme anthropic/minimax anahtarı gerektirir.

### Session-bazlı sohbet + çok-ajanlı `@` yönlendirme ✅ (2026-06-16)

Sohbet ekranı **ajan-bazlıdan session-bazlıya** çevrildi; her oturumun bir **varsayılan
ajanı** var ve `@` ile başka ajanlar aynı sohbete dahil edilebiliyor.

- [x] **Backend:** `db.Message.AgentID` (turu üreten ajan). `chatReq.AgentIDs []string`;
  `chat_stream.go` çoklu-ajan döngüsü (`resolveTurnAgents`, boş→oturum varsayılanı), her ajan
  sırayla yanıtlar (sonrakiler öncekini görür); SSE `meta`→`agent {agentId,index}`→`step`*→`reply`→`done`;
  her tur `Message.AgentID` ile kalıcı. `POST /api/sessions` `agentId` opsiyonel (boş→ilk ajan).
- [x] **Frontend:** `App` tüm oturumları yükler; `Sidebar` düz **"Tüm Oturumlar"** + roster
  "Ajanlar · varsayılan" seçici (avatarlı); `Composer` `@` menüsü metne `@Ad` mention ekler;
  `sendMessage` mention'ları `agentIds[]`'e çözer + çoklu-ajan canlı balonları (`onAgentStart`/`onReply`);
  `MessageList` mesaj başına ajan avatar+adı. `defaultAgentId` localStorage'da.
- **CANLI TEST (API + Chrome):** `agentIds=[Reminder,StepTest]` → iki asistan mesajı sırayla,
  biri Reminder biri StepTest etiketli (kalıcı); UI "Tüm Oturumlar" iki ajanın oturumlarını tek
  listede avatarlarıyla, mesajlar 🤖 Reminder / ST StepTest başlıklarıyla; `@Rem`+Enter →
  `@Reminder ` eklendi. `go build` + `tsc` temiz.

### Detaylı model etiketleri + ajan thinking seviyesi ✅ (2026-06-16)

- [x] **Detaylı model isimleri** (`internal/providers/catalog.go`): her modele açıklayıcı `Label` (ör. "Claude Opus 4.8 — en yetenekli") + `Description` (tek satır not). `ProviderModelSelect` seçili modelin açıklamasını dropdown altında ipucu olarak gösterir.
- [x] **Thinking (uzatılmış akıl yürütme) seviyesi** — ajan başına: `db.Agent.ThinkingLevel` ("" / off / low / medium / high) + `AgentProfilePatch`/`UpdateAgent`; `createAgentReq`/`updateAgentReq` (`PUT/POST /api/agents`). `providers.Request.ThinkingBudget`; `anthropic.go` `thinkingFor` → `thinking:{type:enabled,budget_tokens}` + gereğinde `max_tokens` yükseltme (Complete + Stream). `agent/toolloop.go` `thinkingBudgetForLevel` (low=2048/medium=8192/high=16384) **yalnız `!MCPEnabled` (araçsız) dalında** enjekte edilir — native tool döngüsü imzalı thinking bloğu gerektirdiğinden tool turlarında kapalı. claude-cli/minimax param'ı yok sayar (yalnız anthropic etkili).
- [x] Frontend: `Agent.thinkingLevel` + `AgentPatch`; `AgentSettingsModal`'da "Düşünme (thinking) seviyesi" dropdown'ı (Kapalı/Düşük/Orta/Yüksek) + "yalnız anthropic & araçsız" notu.

**CANLI TEST (API):**
- [x] `/api/catalog`: tüm modeller detaylı label + description ile döndü (Claude/MiniMax).
- [x] Ajan create `thinkingLevel=high` → kalıcı; update `medium` → kalıcı.
- [x] `go build/vet ./...` temiz. (Frontend tsc bu sırada paralel **Composer.tsx** WIP'i yüzünden kırıktı — benim dosyalarımda hata yok; gözcü yeşili bekliyor.)

### Model kataloğu + provider/model seçici + MiniMax sağlayıcı ✅ (2026-06-15)

Ajan için provider+model artık **listeden** seçilebiliyor (serbest girişe de izin var) ve yeni bir **MiniMax** sağlayıcı (OpenAI-uyumlu) eklendi.

- [x] **Katalog** (`internal/providers/catalog.go`): provider+model listesi (claude-cli aliasları, Anthropic Claude modelleri, MiniMax M2.1/M2.1-lightning/M2); her provider `allowCustomModel`. `GET /api/catalog` canlı `available` bayrağıyla (claude-cli PATH'te mi, anthropic/minimax anahtarı var mı).
- [x] **MiniMax provider** (`internal/providers/minimax.go`): OpenAI-uyumlu `POST {base}/chat/completions` (Bearer), `base_resp`/`error` zarfı işlenir; varsayılan `https://api.minimax.io/v1`. `registry` → `SetMinimax(key, baseURL)` + Get("minimax"); `settings` → şifreli `MinimaxKeyEnc` + `MinimaxBaseURL`, DTO `minimaxKeySet`/`minimaxBaseUrl`, `applySettings` push.
- [x] **Frontend** `ProviderModelSelect.tsx` (yeniden kullanılabilir, katalogu modül-düzeyinde cache'ler): provider dropdown + model dropdown + "Özel…" serbest giriş. AgentSettingsModal, Sidebar **ajan oluşturma** (artık model de seçiliyor → `onCreateAgent(name,soul,provider,model)`), SettingsPanel varsayılan provider/model'e entegre. SettingsPanel "Sağlayıcılar"a **MiniMax API anahtarı + base URL** alanları.

**CANLI TEST (API + Chrome):**
- [x] `GET /api/catalog`: 3 provider modelleriyle döndü; anahtarsız anthropic/minimax `available=false`.
- [x] MiniMax key PUT → `minimaxKeySet=true`, baseUrl persist; katalogda minimax `available=true` oldu (sonra sıfırlandı).
- [x] Chrome: ajan oluşturma formunda Sağlayıcı dropdown'ı **Claude CLI · Anthropic (anahtar gerek) · MiniMax (anahtar gerek)** seçeneklerini gösterdi (HTML doğrulandı).
- [x] `go build/vet ./...` + `tsc + vite build` temiz.

> Not: MiniMax (OpenAI-uyumlu istemci) tool-use'suz düz completion yapar; aynı istemci başka OpenAI-uyumlu uçlar için de base URL ile kullanılabilir. Model ID'leri sık değiştiğinden her provider serbest model girişine izin verir.

### Ajan görseli (yuvarlak avatar) + ajan ayar modalı ✅ (2026-06-15)

Ajan listesindeki her ajana **yuvarlak görsel** (avatar) eklendi ve satır kenarındaki **⚙ ayar butonundan** ajan değerleri düzenlenebilir hale geldi.

- [x] **Backend:**
  - `db.Agent`'a `avatar` (emoji/glif) + `color` (hex accent) alanları.
  - `db.UpdateAgent(id, AgentProfilePatch)` — pointer alanlı kısmi güncelleme (nil = dokunma), atomik diske yazar.
  - `PUT /api/agents/{id}` (`handleUpdateAgent`) — name/soul/identity/provider/model/planningMode/avatar/color kısmi patch. `handleCreateAgent` de opsiyonel avatar/color kabul eder.
- [x] **Frontend:**
  - `lib/avatar.ts`: id'den **deterministik renk** türetme (djb2 hash → palet), baş harf çıkarımı, glif/renk paletleri (`AVATAR_COLORS`, `AVATAR_GLYPHS`).
  - `components/AgentAvatar.tsx`: yuvarlak gradient disk — özel emoji ya da baş harf; aktif ajanda halka.
  - `components/AgentSettingsModal.tsx`: canlı avatar önizlemeli editör (ad, emoji seçici, renk paleti, sağlayıcı/model, planlama modu, soul, identity).
  - `Sidebar.tsx`: ajan satırı avatar + hover'da görünen ⚙ butonu (oturum ⟳ deseniyle aynı); modal render.
  - `types.ts` `Agent.avatar/color` + `AgentPatch`; `api.ts` `updateAgent`; `App.tsx` `updateAgent` callback (liste canlı güncellenir).

**CANLI TEST (API + Chrome):**
- [x] `PUT /api/agents/{id}` avatar/color/soul patch'ledi; dosyada `avatar` 🤖 (U+1F916), `color` `#7c3aed` doğru UTF-8 kalıcılaştı.
- [x] Chrome: roster yuvarlak avatarlar (RE/ST deterministik renk); ⚙ → modal açıldı; emoji+renk seçip Kaydet → roster anında 🤖 mor daireye döndü (DOM doğrulandı).
- [x] `go build ./...` + `tsc --noEmit` + `vite build` temiz.

### Workspace geniş düzenleme + paralel çalışma ✅ (2026-06-15)

Workspace düzenleme seçenekleri genişletildi ve paralel çalışma netleştirildi.

- [x] **Görsel kimlik**: `WSSettings`'e `Icon` (emoji) + `Color` (hex). `/api/workspaces` listesi artık icon/color ile zengin; WorkspaceSwitcher + NavRail daraltılmış rozet ikon/renk gösterir. Emoji uçtan uca round-trip doğrulandı (disk codepoint `D83D DE80`).
- [x] **İstatistikler**: `workspace-settings` DTO'ya `createdAt` + `agentCount`/`sessionCount`/`taskCount` (canlı sayım). Ayar ekranında rozet kartları.
- [x] **Silme**: "Bu Workspace" kategorisinde tehlikeli-bölge sil butonu (`onDeleteWorkspace`, App'te confirm + başka workspace'e geçiş; backend son workspace'i silmeyi reddeder).
- [x] **Paralel çalışma (zaten var, doğrulandı)**: `workspace.Manager.open()` boot'ta **her** workspace için `Runtime.StartConfigured` (heartbeat ajanları) + `Scheduler.Start` (cron) + `ResumeRunningFlows` çağırır → tüm workspace'lerin otonom ajanları/zamanlamaları **aynı süreçte eşzamanlı** koşar (UI yalnız aktif olanı gösterir). Per-workspace pause (`Runtime.SetPaused`) ile biri diğerlerini etkilemeden durdurulabilir.

**CANLI TEST (API + Chrome):**
- [x] workspace-settings GET: icon/color + stats (2 ajan/3 oturum/0 görev/createdAt) döndü.
- [x] PUT icon=🚀 color=#22c55e → kaydedildi; disk codepoint D83D DE80 (emoji bozulmadan); `/api/workspaces` zenginleşti.
- [x] Chrome: "Bu Workspace" kategorisi — istatistik kartları, ikon/renk alanları, 🚀 önizleme, sil bölümü render (DOM doğrulandı). Sonra varsayılana sıfırlandı.
- [x] `go build/vet ./...` + `tsc + vite build` temiz.

### Loglar ekranı (uygulama + tüm workspace) ✅ (2026-06-15)

En sol NavRail'e **📜 Loglar** görünümü eklendi; tüm uygulama ve workspace logları tek ekranda.

- [x] `internal/logbuf` (yeni): `Buffer` (sabit kapasiteli ring, 2000) + `slog.Handler` (kayıtları buffer'a yakalar **ve** stdout text handler'a delege eder). `main.go` logger'ı bu handler ile kurar → tüm workspace runtime'ları aynı logger'dan geçtiği için **çapraz-workspace** tüm akış tek buffer'da.
- [x] `api/logs.go` (yeni): `GET /api/logs?limit=&level=&q=` — seviye (min) + substring filtre; uygulama-geneli (workspace-scoped değil). `Server`'a `*logbuf.Buffer` alanı + `NewServer` imzası.
- [x] Frontend: `types.ts`/`api.ts` `LogEntry` + `getLogs`; `NavRail` "📜 Loglar"; `LogsPanel.tsx` (canlı poll 2.5sn "Canlı" toggle, seviye filtre chip'leri, arama, renkli seviye rozeti, zaman+mesaj+alanlar, oto-scroll); `App.tsx` render.

**CANLI TEST (API + Chrome):**
- [x] `/api/logs` boot loglarını döndürdü (config/providers/scheduler/workspace opened + attrs).
- [x] Filtre: `level=error`→0, `q=workspace`→2; yeni workspace oluştur → log anında yakalandı (`name=LogTest WS`).
- [x] Chrome: 📜 Loglar render — zaman/seviye/mesaj/attrs satırları, 2 workspace'in açılış logları görünür (DOM doğrulandı).
- [x] `go build/vet ./...` + `tsc + vite build` temiz.

### Depolama: SQLite → Dosya Sistemi ✅ (2026-06-15)

Kalıcılık katmanı **tamamen dosya-tabanlıya** çevrildi (External Agents OSS tarzı).
SQLite (`modernc.org/sqlite`), migration runner ve `migrations/*.sql` kaldırıldı.
Detay: **`_Docs/08-DEPOLAMA.md`**.

- [x] `internal/db` **yerinde** dosya-store'a çevrildi: bellek-içi maps + diske
  atomik write-through (`*.tmp`→`rename`), tek `sync.RWMutex`. **Tüm metod imzaları,
  model struct'ları, sabitler, `ErrNotFound` korundu** → onu kullanan 22 dosya
  (`agent`/`api`/`conversation`/`memory`/`workspace`) **değişmedi**.
- [x] Disk yapısı: `{wsID}/store/` altında entity-başına JSON + `sessions/{id}/session.jsonl`
  (satır 1 header, satır 2+ mesajlar). Embedding base64 gömülü; usage gün-bazlı dosya.
- [x] `swarmgo.db` yolu → `store/` dizini (`workspace/manager.go`); ölü `Config.DBPath()` silindi.
- [x] `go.mod` temizlendi: yalnız `google/uuid` + `robfig/cron/v3` kaldı (sqlite + ~8 dolaylı dep gitti).
- [x] `internal/db/filestore_test.go` round-trip testi (create→reopen→reload + UTF-8/HTML/embedding/cascade) **PASS**.

**CANLI TEST (API + restart):**
- [x] Backend temiz veri diziniyle ayağa kalktı; `store/` boş oluştu.
- [x] API ile ajan/oturum/görev/MCP oluşturuldu → doğru disk dosyaları yazıldı.
- [x] **Restart** sonrası 2 ajan + 2 oturum + 1 görev + 1 MCP **diskten reload** edildi; Türkçe başlık (`İlk sohbet — ğüşıöç`) round-trip korundu.
- [x] `go build/vet ./...` temiz.

> Not: Otomatik SQLite→dosya migration'ı yok (kullanıcı talebiyle SQLite tamamen kaldırıldı); yeni kurulum `store/`'da sıfırdan başlar.

### Ayarlar ekranı — genişletme: 2 panel + workspace + profil + beta'lar ✅ (2026-06-15)

Ayarlar ekranı **sohbet gibi 2 panele** dönüştürüldü (sol kategori rayı, sağ içerik) ve kapsam genişletildi.

- [x] **2 panel UI** (`SettingsPanel.tsx` yeniden): sol rayda **Uygulama** grubu (Profil · Görünüm · Bildirimler & Ekran · Sağlayıcılar · Bağlam & Bellek · Bütçe · Otonomi · Otomatik Başlık · MCP · Tanılama · Hakkında) + **Bu Workspace** grubu; kaydedilmemiş kategoride nokta göstergesi; kategoriye-özel Kaydet (app/workspace ayrı scope).
- [x] **Workspace'e özel ayarlar** (`internal/workspace/settings.go`): her workspace'in `ws-settings.json`'u — ad (rename), açıklama, **bu workspace'e özel** varsayılan sağlayıcı/model, **bu workspace'te otonomiyi duraklat**. `Manager.Rename`/`UpdateSettings`; `agent.Runtime` per-workspace `SetPaused/Paused` (guardedComplete'te global pause ile birlikte kontrol); `GET/PUT /api/workspace-settings` (X-Workspace-Id). Ajan oluşturmada öncelik: istek → workspace → uygulama → varsayılan.
- [x] **Kullanıcı profili** (resimdeki gibi): Ad · Saat dilimi · Şehir · Ülke · Notlar — `settings` alanları; chat sistem promptuna `## About the user` bloğu olarak enjekte edilir (`userContextBlock`).
- [x] **Masaüstü bildirimleri** + **Ekranı açık tut**: istemci-tarafı (`lib/clientPrefs.ts` — Notification API + Wake Lock, visibility-change'te yeniden alır); pencere arkadayken yanıt gelince bildirim.
- [x] **Anthropic beta'ları** (yalnız anthropic sağlayıcı): **1M token bağlam** (`context-1m-2025-08-07`) + **uzatılmış prompt cache 1 saat** (`extended-cache-ttl-2025-04-11` + system'e `cache_control` ttl=1h). `anthropic.go` `WithBetas`/`betaHeader`/`systemField`; `registry.SetAnthropicBetas`; `applySettings` push.

**CANLI TEST (API + Chrome):**
- [x] settings yeni alanlar GET/PUT: profil (Bilal/Türkiye/İstanbul), 1M+cache+notif+awake → kaydedildi.
- [x] workspace-settings: rename "Ana Workspace" + açıklama + pause=true persist; `/api/workspaces` rename'i yansıttı; sonra varsayılana sıfırlandı.
- [x] Chrome: ⚙ Ayarlar → **Profil** kategorisi (Ad/Saat dilimi/Şehir/Ülke/Notlar) ve **Bu Workspace > Genel** kategorisi (ad/açıklama/sağlayıcı-model override) DOM ile doğrulandı; 0 konsol hatası.
- [x] `go build/vet ./...` + `tsc + vite build` temiz.

> Not: 1M context / uzatılmış cache yalnız **anthropic** sağlayıcıda etkilidir (claude-cli yok sayar). Db katmanı bu sırada paralel olarak SQLite'tan dosya-store'a taşındı; eklemeler birleşik derlemede temiz.

---

### Ayarlar ekranı (uygulama geneli) ✅ (2026-06-15)

Uygulama genelinde tek bir ayar belgesi (`settings.json`, data dizininde) + Ayarlar UI eklendi.
Hassas Anthropic API anahtarı AES-GCM ile şifreli saklanır, istemciye asla düz dönmez
(`anthropicKeySet` boolean). Ayarlar canlı olarak alt sistemlere uygulanır.

**Backend**
- [x] `internal/settings/` (yeni): `settings.go` (Settings + DTO maskeli + Patch), `store.go` (thread-safe JSON deposu, atomik yazım, normalize/clamp, `Cipher` arayüzü ile şifreli secret).
- [x] `providers/registry.go`: mutable + `SetAnthropicKey/SetClaudeCLIPath/SetDefaultModel`, RWMutex; `Get` canlı değerleri okur.
- [x] `conversation/manager.go`: `SetLimits` (canlı maxTokens/keepRecent), mutex.
- [x] `agent/tunables.go` (yeni): süreç-geneli `SetAutonomyPaused`/`SetTitleModel`; `budget.go` otonom çağrıda `ErrAutonomyPaused`; `titler.go` başlık-modeli override.
- [x] `api/settings.go` (yeni): `GET/PUT /api/settings`, `POST /api/settings/test-provider` (gerçek minimal completion ile bağlantı testi).
- [x] `api/server.go`: `settings` alanı + `NewServer` imzası + `applySettings()` (boot + her güncellemede push).
- [x] `api/agents.go`: yeni ajanlarda varsayılan provider/model + günlük bütçe ayarlardan.
- [x] `api/chat.go`: auto-title `autoTitleEnabled` ayarına bağlı.
- [x] `cmd/swarmgo/main.go`: settings store + env `ANTHROPIC_API_KEY`'i tek seferlik şifreli store'a migrate.

**Frontend**
- [x] `types.ts`/`api.ts`: `AppSettings`/`SettingsPatch`/`ProviderTestResult` + `getSettings/updateSettings/testProvider`.
- [x] `components/SettingsPanel.tsx` (yeni): 9 bölüm (Görünüm · Sağlayıcılar · Bağlam & Bellek · Bütçe · Otonomi · Otomatik Başlık · MCP · Tanılama · Hakkında), dirty-takip + tek Kaydet, API key maskeli/sil, provider test rozeti, toggle/sayı/select alanları.
- [x] `components/NavRail.tsx`: en altta sabit ⚙ Ayarlar görünümü.
- [x] `lib/theme.ts` (yeni) + `index.css`: açık tema (`[data-theme=light]`) + accent CSS değişkeni; `App.tsx` boot'ta + kayıtta `applyTheme`.

**CANLI TEST (API + Chrome):**
- [x] GET/PUT settings round-trip; `theme/maxContextTokens/pauseAutonomy` güncellendi.
- [x] Anthropic key: set → `anthropicKeySet=true` (değer dönmüyor), temizle → false.
- [x] test-provider claude-cli → `ok=true model=claude-opus-4-8 sample="OK"`.
- [x] Ajan varsayılanları: `defaultModel=sonnet` + `callLimit=50` ayarıyla yeni ajan o değerlerle oluştu.
- [x] Auto-title kapısı: `autoTitleEnabled=false` iken ilk mesajda başlık üretilmedi.
- [x] Chrome: uygulama 0 konsol hatasıyla yüklendi; ⚙ Ayarlar tıklandı, 9 bölüm render oldu (DOM doğrulandı). Görsel ekran görüntüsü 87 sekmeli ortamda `image readback` ile alınamadı (guide'da bilinen sorun).

### Sohbet: adım-adım akış (SSE streaming) ✅ (2026-06-15)

Sohbet artık tüm tur bitince değil, **her adım bittikçe** UI'a akıtılıyor.
- `providers.Request.OnEvent func(TraceStep)` + `claudecli.go` stream-json'u **satır
  satır** (`bufio` + `cliStreamParser`) okuyup olayları anında yayınlar (thinking
  hemen, ara metin flush'ta, tool adımı sonucu gelince). `agent/toolloop.go`
  `CompleteWithToolsStream(... onStep)` köprüler; native döngü de her adımı yayınlar.
- `internal/api/chat_stream.go` — `POST /api/chat/stream` (SSE): `meta`→`step`*→`done`.
  Tur sonunda mesaj + tam iz kalıcılaştırılır.
- Frontend: `api.streamChat` (`fetch`+`ReadableStream` SSE ayrıştırma), `App.sendMessage`
  canlı asistan balonu büyütür, `MessageList` boş balonda "çalışıyor" noktaları.
- **CANLI TEST:** `curl -N /api/chat/stream` zaman damgaları artımlı geldi
  (`meta` 08.9s → text 14.7s → Bash step-one 37.4s → Bash step-two 47.8s → `done` 56.7s);
  Chrome'da canlı asistan balonu akış sırasında "çalışıyor" göstergesiyle yakalandı.
  `go build ./...` + `tsc --noEmit` temiz.

### Zengin Sohbet Arayüzü (Chat UX) ✅ (2026-06-15)

Sohbet ekranı [external-agent-oss](https://github.com/external-agent-project/external-agent-oss)
referans alınarak External Agent benzeri zengin render katmanına kavuşturuldu: markdown
çıktı, tool kullanım kartları, düşünme adımları, dosya satır değişimi (diff),
tıklanabilir dosya yolları ve inline görsel. Asistan turu artık **aktivite izini**
(thinking + ara metin + tool çağrıları) ve son cevabı zengin biçimde gösterir.
Detaylı doküman: [07-CHAT-UX.md](07-CHAT-UX.md).

**Backend**
- [x] `internal/agent/trace.go` (yeni): `TurnStep` (kind=text|thinking|tool, tool/input/output/isError).
- [x] `internal/agent/toolloop.go`: `CompleteWithToolsTraced` — native agentic döngü ara metni + her `tool_use` çağrısını (girdi+sonuç) sıralı izle kaydeder. Eski `CompleteWithTools` bunu çağırıp izi yutar (imza uyumlu). İz **hem native hem claude-cli** (stream-json) yolunda üretilir — bkz. aşağıdaki "Anahtarsız adım izi" notu.
- [x] `db.Message.Steps` alanı (oturum JSONL'inde JSON `[]TurnStep`) — yeniden yüklemede tur yeniden çizilir; `AddMessage`/`ListMessages` korur. (O dönem SQLite `0007_message_steps.sql` migration'ıydı; depolama dosya-tabanlıya taşınınca alana dönüştü — bkz. `08-DEPOLAMA.md`.)
- [x] `internal/api/chat.go`: yanıt `steps` döndürür ve izi mesaja yazar.
- [x] `internal/api/files.go` (yeni): `GET /api/files?path=` — inline görsel için salt-okunur akış (görsel uzantı allowlist'i).

**Frontend** (yeni bağımlılıklar: react-markdown, remark-gfm, highlight.js)
- [x] `components/markdown/`: `Markdown.tsx` (GFM, özel kod/link/görsel render, `urlTransform` kapalı → yerel `C:\` yolları korunur, Windows ters-bölü normalize), `CodeBlock.tsx` (dil etiketi + kopya + highlight.js, diff→DiffView), `DiffView.tsx` (+N/−M satır renkli).
- [x] `components/chat/`: `TurnSteps.tsx` (iz çizimi + parseSteps), `ThinkingBlock.tsx` (açılır akıl yürütme), `ActivityCard.tsx` (tool kartı: ikon+etiket+niyet, açınca girdi/çıktı, Edit/Write→diff, hata kırmızı), `PathText.tsx` (metindeki yolları tıklanır çip).
- [x] `lib/`: `tools.ts` (tool meta), `paths.ts` (yol tespiti/görsel url), `diff.ts` (diff ayrıştırma).
- [x] `MessageList.tsx`: asistan turu ThinkingBlock → TurnSteps → markdown cevap; `App.tsx` `onOpenFile` (görsel→yeni sekme, diğer→pano); `index.css` `.sg-markdown` tipografisi + github-dark tema.

**CANLI TEST (Chrome DOM + API):**
- [x] Markdown: h2/liste/tablo, `go` kod bloğu highlight.js renkli span'larla, unified diff `+2 / −1` satır renkli — DOM doğrulandı.
- [x] Aktivite izi (DB'ye enjekte edilen örnek tur): düşünme bloğu, ara metin, Read/Edit/Bash tool kartları (tıklanabilir yol + kırmızı "hata" rozeti), Edit çıktısı diff olarak render.
- [x] Inline görsel: markdown `![](C:\…\hero.png)` → `src=/api/files?path=…`; uç **HTTP 200 image/png 13KB** döndürdü.
- [x] `go build ./...` + `tsc --noEmit` temiz.

**Anahtarsız adım izi (güncelleme):** `providers/claudecli.go` `--output-format stream-json --verbose`'a geçirildi; CLI'ın kendi olay akışı (`tool_use`/`tool_result`/`thinking`/`text`) ayrıştırılıp `Response.Trace` → `traceToSteps` ile `[]TurnStep`'e çevrilir. Böylece **API anahtarı olmadan** da tool kartları + ara adımlar görünür. Canlı test: claude-cli ajanı "Bash ile `echo hello-from-swarmgo`" → yanıt `steps`=[text, tool(Bash, output=hello-from-swarmgo)]; Chrome DOM'da ▶️ Bash kartı GIRDI/ÇIKTI ile render oldu. Native (anthropic) yol da kendi izini üretmeye devam eder.

---

### Faz 7 — Orchestration ✅ (2026-06-15)

Yapılandırılmış **çok-ajanlı akışlar**: bir akış = node grafiği (agent / branch / parallel).
Şablon sistemi + **restart-safe run state** (her node sonrası DB'ye yazılır, çökme sonrası kaldığı yerden devam).

**1) Graf modeli + motor (`internal/orchestration`)**
- [x] `model.go`: `Graph`/`Node`/`Branch`; node tipleri `agent`/`branch`/`parallel`; `Validate` (start/uniq/ref/agent kontrolleri)
- [x] `engine.go`: `Engine.Run` — start'tan yürür; `agent` node `AgentRunner` ile çalışır, çıktı saklanır; `branch` son çıktıya göre yönlendirir (case-insensitive substring, boş = varsayılan); `parallel` çocukları goroutine ile eşzamanlı koşar + join birleştirir; `maxSteps=50` döngü guard; her node sonrası `SaveFunc` ile state persist
- [x] Şablon: `{{input}}`, `{{last}}`, `{{node.<id>}}` prompt yer tutucuları

**2) DB (migration `0006_flows.sql`)**
- [x] `flows` (graph JSON) + `flow_runs` (restart-safe `state` JSON, status, input, output, error)
- [x] `models_flow.go` + `store_flow.go`: Flow CRUD + FlowRun (Create/Get/List/SetState/Finish + `ListRunningFlowRuns`)

**3) Agent entegrasyonu (`internal/agent/flow.go`)**
- [x] `flowRunner` → `orchestration.AgentRunner`: her node ajanın tam hattından geçer (memory recall + tools + budget via `complete`)
- [x] `RunFlow` (yeni run aç + motoru sür) + `driveFlow` (per-node persist + terminal status)
- [x] `ResumeRunningFlows` (boot'ta `running` kalan run'ları persist state'ten devam ettirir) — workspace açılışına bağlandı (restart-safe)

**4) API + Frontend**
- [x] `api/flows.go`: GET/POST `/api/flows`, PUT/DELETE `/api/flows/{id}`, POST `/api/flows/{id}/run`, GET `/api/flow-runs`, GET `/api/flow-runs/{id}`
- [x] `components/FlowsPanel.tsx`: görsel protokol builder — akış listesi, node editörü (agent/branch/parallel form kartları, başlangıç seçimi, sonraki/dal/paralel yönlendirme), çalıştır + trace görüntüleyici; NavRail "🔀 Akışlar"

**CANLI TEST (API + Chrome):**
- [x] **Flow A (agent→branch→agent)** "Sentiment Router": pozitif girdi → n1 "POSITIVE" → branch doğru şekilde n3 (Cheerful) → neşeli yanıt; trace state'e kalıcı yazıldı
- [x] **Flow B (parallel→join→agent)** "Pros & Cons": a1+a2 eşzamanlı koştu, çıktılar `[Advantage]…[Disadvantage]…` birleşti, s1 birleşik `{{last}}`'i özetledi
- [x] Chrome (DOM): "🔀 Akışlar" sekmesi; iki akış listelendi; node editörü 4 node'u tip/ajan/yönlendirme ile render etti; çalıştır bölümü göründü

> Bilinen sınır: `branch` substring eşleşmesi LLM çıktısının temizliğine duyarlı — claude-cli gevezelik ekleyince ("POSITIVE? No. NEGATIVE") yanlış dal seçilebilir. Motor doğru; sınıflandırma node'larında prompt katı tutulmalı veya varsayılan dal dikkatli sıralanmalı.

---

## ESKİ: FAZ 8 MCP + ARAÇLAR TAMAMLANDI ✅ (CANLI TEST GEÇTİ)

### Faz 8 — Tool-use + MCP ✅ (2026-06-15)

Ajanlara **araç kullanımı** kazandırıldı: hem yerleşik (built-in) hem **MCP sunucu** araçları.
SwarmClaw deseni: built-in + MCP tek katalogda, ajan başına atanır. İki yol birlikte kuruldu.

**1) Provider native tool-use protokolü (gerçek motor)**
- [x] `providers/provider.go`: `ToolDef`/`ToolCall`/`ToolResult`; `Request.Tools`, `Message.ToolCalls`/`ToolResults`, `Response.ToolCalls`+`StopReason`
- [x] `providers/anthropic.go`: content-block modeli (text/tool_use/tool_result); `tools` gönderimi + `tool_use` ayrıştırma
- [x] `agent/toolloop.go`: `CompleteWithTools` — native agentic döngü (provider → tool çalıştır → tool_result → tekrar, `maxToolIters=8`), bütçe guardrail entegre

**2) claude-cli MCP delegasyonu (anahtarsız, canlı test edilen yol)**
- [x] `providers/claudecli.go`: `ConfigureMCP` — `--mcp-config` + `--strict-mcp-config` + `--allowedTools`; CLI tool döngüsünü kendi içinde çalıştırır (API anahtarı gerekmez)
- [x] etkin MCP sunucularından geçici `--mcp-config` JSON üretimi + temizleme

**3) MCP istemcisi (`internal/mcp`) — SDK'sız elle JSON-RPC 2.0**
- [x] `client.go`: stdio; `initialize` → `notifications/initialized` → `tools/list` / `tools/call`
- [x] `manager.go`: `ServerConfig`, namespace (`<server>__<tool>`), `BuildCatalog`, `CallNamespaced`, `ListServerTools`; sse/http → net "henüz desteklenmiyor"

**4) Tool kayıt defteri (`internal/tools`)**
- [x] `registry.go`: `Tool` arayüzü + birleşik `Registry` (built-in + MCP); `Defs(allow)` allowlist, `Call` (hata → IsError)
- [x] Built-in: `get_current_time`, `http_get` (64KB sınır), `memory_recall`

**5) DB (migration `0005_mcp_tools.sql`)**
- [x] `mcp_servers` += `command`/`args`/`url`/`enabled`/`scope`; `agents` += `mcp_enabled`/`allowed_tools`
- [x] `models_mcp.go` + `store_mcp.go`: CRUD + `UpdateAgentTools`

**6) API + Frontend**
- [x] `api/mcp.go`: GET/POST `/api/mcp-servers`, `/toggle`, `/test`, DELETE
- [x] `api/agent_tools.go`: GET/POST `/api/agents/{id}/tools` (mcpEnabled + allowlist + canlı katalog)
- [x] chat/executor/heartbeat → `CompleteWithTools` (usage tek yerde)
- [x] `components/ToolsPanel.tsx` + NavRail "🔌 Araçlar" + types/api

**CANLI TEST (Go test + API + Chrome):**
- [x] `internal/mcp` live test: gerçek `@modelcontextprotocol/server-filesystem` (npx) → 14 araç, `list_directory` → `[FILE] note.txt`
- [x] API: MCP oluştur → `/test` ok=true 14 araç; ajan tools aç → katalog **17 araç** (14 MCP + 3 built-in)
- [x] API: claude-cli delegasyonu — "note.txt oku" → araçla okudu, **"hello from swarmgo"** (BOM + CRLF dahil → gerçekten araçla, tahmin değil)
- [x] Chrome (DOM): "🔌 Araçlar" sekmesi; 17 araç tam açıklamayla; filesystem MCP sunucusu Test/Kapat/Sil; ekleme formu render

> Not: `chrome_screenshot` odaktaki başka sekmeyi yakaladı (bilinen mcp-chrome sorunu); doğrulama DOM (`chrome_get_web_content`) ile yapıldı.

---

## ESKİ: Otomatik başlık üretimi (auto-title) ✅ (2026-06-15)

Görev ve sohbetlere otomatik başlık üreten ortak bir sistem eklendi. Kanban'da artık
yalnızca prompt girilir; başlık prompt'tan üretilir. Sohbette ilk mesaj gönderilince
başlık otomatik oluşur. Her ikisi de istenildiğinde ⟳ ile yeniden üretilebilir.

**Backend**
- [x] `internal/agent/titler.go` (yeni): `GenerateTitle` (ajan provider'ı ile kısa başlık),
  `TitleFor` (tercih edilen ajan yoksa ilk ajana düşer; üretim hatasında prompt'tan
  `FallbackTitle`), `SanitizeTitle` (tek satır, tırnak/noktalama temizliği, 60 karakter sınırı).
  Başlık talimatı hem system hem **user turn** içine gömülü — claude-cli'ın büyük taban
  promptunun appended system'i bastırmasını önler.
- [x] `db/store.go`: `SetSessionTitle`.
- [x] `api/chat.go`: ilk turda (kind=chat, başlık boş, messageCount=0) başlık otomatik üretilir,
  `sessionTitle` alanı ile döner; best-effort (hata yanıtı bozmaz).
- [x] `api/sessions.go`: `POST /api/sessions/{id}/title` — sohbet geçmişinden (veya `source`) yeniden üret.
- [x] `api/tasks.go`: görev oluşturmada `title` opsiyonel (prompt'tan üretilir); `POST /api/tasks/{id}/title` yeniden üret.

**Frontend**
- [x] `types.ts`/`api.ts`: `ChatResponse.sessionTitle`; `generateSessionTitle`, `generateTaskTitle`; `createTask.title` opsiyonel.
- [x] `TaskBoard.tsx`: form'dan başlık alanı kaldırıldı (yalnız prompt + ajan); her kartta ⟳ "başlığı yeniden oluştur".
- [x] `Sidebar.tsx`: her oturumda hover'da ⟳ başlık yenileme.
- [x] `App.tsx`: chat yanıtındaki `sessionTitle` ile oturum başlığı güncellenir; `regenerateSessionTitle`.

**CANLI TEST (API):**
- [x] Görev: yalnız prompt → başlık "Veritabanı yedekleme cron görevi kurulumu" otomatik üretildi.
- [x] Görev yeniden başlık: ⟳ → "Veritabanı yedekleme gece cron ve hata raporu".
- [x] Sohbet: ilk mesaj → `sessionTitle` "Python liste tuple farkı", DB'ye yazıldı.
- [x] Frontend: `tsc -b && vite build` temiz; uygulama tarayıcıda render oldu (DOM doğrulandı).
- Not: Chrome görsel testi 80+ sekmeli ortamda kararsız (image readback) — doğrulama API + DOM metni üzerinden yapıldı.

### UI yeniden düzenleme — 3 kolonlu yerleşim ✅ (2026-06-15)
- [x] `components/NavRail.tsx` (yeni): en solda **daraltılabilir** nav rail — marka + workspace switcher + görünüm geçişleri (💬 Sohbet · 🗂 Görevler · ⏰ Zamanlamalar · ⛁ Hafıza). Daraltınca ikon-only (w-14 ↔ w-52); tercih `localStorage`'da.
- [x] `components/Sidebar.tsx`: artık orta kolon — yalnızca Ajanlar + Oturumlar listesi (logo/workspace NavRail'e taşındı). **Yalnızca ajan-bazlı görünümlerde (Sohbet, Hafıza) gösterilir;** Görevler/Zamanlamalar workspace-bazlı olduğundan orada gizli.
- [x] `App.tsx`: `NavRail → Sidebar → main` 3 kolon; görünüm sekmeleri header'dan NavRail'e taşındı, header artık görünüm başlığı + (chat'te) meter'ları gösterir.
- [x] **Chrome canlı test:** 3 kolon render, daralt/genişlet çalışıyor, meter'lar header'da.



### Faz 6.5 — Sağlamlaştırma ✅ (2026-06-15)

SwarmClaw kıyaslamasında öne çıkan iki kritik açık kapatıldı: **bağlam (context) yönetimi** ve **otonom döngü maliyet guardrail'i**.

**1) Bağlam yönetimi + compaction (`internal/conversation`)**
- [x] `tokens.go`: tokenizer-bağımsız token tahmini (~4 char/token)
- [x] `manager.go`: `Manager.Prepare` — pending geçmiş bütçeyi aşınca eski turları **rolling summary**'ye katlar (compaction), sadece özet + son N tur gönderilir
- [x] Migration `0004`: `sessions.summary` + `summary_msg_count`; `db.SetSessionSummary`
- [x] `api/chat.go`: her turda Prepare çağrılır; özet sistem promptuna enjekte edilir; yanıt `contextTokens` + `compacted` döner
- [x] Env: `SWARMGO_MAX_CONTEXT_TOKENS` (varsayılan 12000), `SWARMGO_KEEP_RECENT_MSGS` (8)
- [x] **Önceki bug:** chat her turda TÜM geçmişi gönderiyordu → uzun oturumda context taşması + artan maliyet. Artık sınırlı.

**2) Maliyet / bütçe guardrail'i (otonom döngü)**
- [x] Migration `0004`: `agents.daily_call_limit` + `daily_token_limit` (0=sınırsız); `agent_usage(agent_id, day, calls, in/out tokens)` tablosu
- [x] `db/store_usage.go`: `AddUsage` (upsert), `GetUsageToday`, `UpdateBudget`
- [x] `agent/budget.go`: `guardedComplete` — **tek provider çağrı hunisi**; otonom çağrıda `ensureBudget` (limit aşılırsa `ErrBudgetExceeded`, provider'a gitmeden), her çağrıda usage kaydı
- [x] Tüm otonom yollar guardedComplete'e bağlandı: heartbeat (otonom), scheduler prompt/task (otonom), reflect (kullanıcı tetikli=false ama usage kaydı var). Manuel chat + run-now budget'a takılmaz.
- [x] API: GET `/api/agents/{id}/usage`, POST `/api/agents/{id}/budget`, GET `/api/sessions/{id}/context`

**Frontend**
- [x] `components/ChatMeters.tsx`: chat başlığında **⛁ context meter** (token + ⧉ özet göstergesi) ve **◷ bütçe meter** (bugünkü çağrı/limit, tıkla→limit ayarla)
- [x] `App.tsx`: chat header'a meter'lar; her turdan sonra `meterRefresh`

**CANLI TEST (API + Chrome):**
- [x] Compaction: maxctx=120 ile 4 mesaj → 4.'te `compacted=true`, summary_msg_count=5, özet kalıcı bilgileri yakaladı (Go, Türkiye, oyun, SQLite)
- [x] Budget: limit=4 (mevcut kullanım) → heartbeat wake → "failure: daily budget exceeded", **token harcanmadı** (provider'a gitmeden bloklandı); manuel chat etkilenmedi
- [x] Chrome: chat başlığında ⛁ 362 ⧉ ve ◷ 4 çağrı göstergeleri render edildi

> Not: claude-cli compaction özetine kendi persona tonunu katıyor (anthropic provider'da daha temiz). İşlevsel olarak doğru.

---

## ESKİ: FAZ 6 TAMAMLANDI ✅ (CHROME'DA CANLI TEST GEÇTİ)

### Faz 6 — Memory ✅ (2026-06-15)

Ajan hafızası: belge + günlük (journal) + yansıma (reflection). Recall **saf Go sözcüksel benzerlik** (token-frekans cosine) — harici embedding API yok, CGO yok, çevrimdışı. Tüm anılar workspace-scoped.

**Mimari karar:** Anahtarsız/CGO-free felsefeye uygun olarak gerçek semantik embedding yerine lexical cosine kullanıldı; term-vektörü `knowledge_sources.embedding` BLOB'unda cache'lenir. İleride aynı `memory.Store` arkasına gerçek embedder takılabilir (`0001`'deki şema yeterli, migration gerekmedi).

**Memory paketi (`internal/memory`)**
- [x] `vector.go`: Unicode-aware tokenizasyon (TR+EN stopword), tf-vektör, cosine, JSON marshal/unmarshal
- [x] `memory.go`: `Store` (db sarmalayıcı) — Remember / Recall (top-N, minScore eşiği) / List / Delete / **ContextBlock** (sistem promptuna enjekte edilecek blok)

**DB**
- [x] `models_memory.go`: `KnowledgeSource` + kind sabitleri (document/journal/reflection)
- [x] `store_memory.go`: CreateKnowledge / GetKnowledge / ListKnowledge(kind filtreli) / DeleteKnowledge + `placeholders` helper

**Agent paketi**
- [x] `runtime.go`: Runtime'a `mem *memory.Store` alanı + `Memory()` erişimcisi
- [x] `reflector.go`: `Journal` (aktiviteyi kaydet) + `Reflect` (dream cycle — son ~20 journal'ı provider'a özetletip reflection olarak sakla)
- [x] `executor.go`: `invokeWithMemory` (recall→sistem promptuna enjekte) + başarılı task sonrası journal; `complete` ortak helper

**Enjeksiyon noktaları**
- [x] `api/chat.go`: her turda recall→enjekte + tur sonrası journal
- [x] `agent/executor.go`: task çalıştırmada recall→enjekte + journal

**API uçları**
- [x] GET/POST `/api/agents/{id}/memories` (liste, ?kind= filtreli / belge ekle)
- [x] POST `/api/agents/{id}/reflect` (yansıma üret)
- [x] POST `/api/agents/{id}/recall` (recall önizleme — skorlu, debug)
- [x] DELETE `/api/memories/{id}`

**Frontend**
- [x] `types.ts`/`api.ts`: Memory/RecallHit + uç metodları
- [x] `components/MemoryPanel.tsx`: belge ekle, ✦ Yansıt, tür filtreleri (Tümü/Belgeler/Günlük/Yansımalar), rozetli liste, sil
- [x] `App.tsx`: 4. görünüm "Hafıza" (aktif ajana göre)

**CANLI TEST (API + Chrome):**
- [x] API: 3 belge eklendi; recall "hangi veritabani" → SQLite anısı skor 0.236 ile döndü (diğerleri eşik altı)
- [x] API: chat "hangi veritabani?" → ajan enjekte edilen anıdan **"SQLite"** cevapladı (başka bilme yolu yok); tur journal'landı
- [x] API: reflect → ajan journal üzerine birinci şahıs yansıma yazdı, kullanıcının kısa cevap tercihini bile not etti
- [x] Chrome: Hafıza sekmesinde belge/günlük/yansıma rozetli olarak göründü

---

## ESKİ: FAZ 5 TAMAMLANDI ✅ (CHROME'DA CANLI TEST GEÇTİ)

### Faz 5 — Tasks + Schedules ✅ (2026-06-15)

Görev panosu (kanban) + cron zamanlama. Hepsi **workspace-scoped** (her workspace kendi scheduler'ı).

**DB (migration `0003_tasks_schedules.sql`)**
- [x] `tasks`/`schedules`/`runs` zaten 0001'de vardı; Faz 5 kolonları eklendi: tasks → `prompt`, `last_run_id/status/at`; schedules → `task_id`, `prompt`, `last_run_at`; runs → `output`, `trigger` + indeksler
- [x] `internal/db/models_task.go`: Task / Schedule / Run tipleri + board state sabitleri + `ValidBoardState`
- [x] `internal/db/store_task.go`: CreateTask/GetTask/ListTasks/UpdateTask/MoveTask/SetTaskLastRun/DeleteTask + `nullable`/`mustAffect` helper'ları
- [x] `internal/db/store_run.go`: CreateRun/FinishRun/ListRuns/GetRun
- [x] `internal/db/store_schedule.go`: CRUD + ListEnabledSchedules + SetScheduleEnabled/Delivery + GetOrCreateKindSession

**Çalıştırma + zamanlama (agent paketi)**
- [x] `internal/agent/executor.go`: `Runtime.RunTask` — run aç → board `in_progress` → provider çağrısı → çıktı + done/failed; `invoke` helper'ı (tek prompt)
- [x] `internal/agent/scheduler.go`: `robfig/cron/v3` ile workspace başına Scheduler; Start/Reload/Stop; fire → task çalıştır veya prompt teslim et; `nextRunAt` senkronu; geçersiz cron yakalama
- [x] `internal/workspace/manager.go`: Workspace'e `Scheduler` alanı; open'da Start, Delete/Close'da Stop

**API uçları**
- [x] `internal/api/tasks.go`: GET/POST `/api/tasks`, PUT/DELETE `/api/tasks/{id}`, POST `/api/tasks/{id}/run`, GET `/api/tasks/{id}/runs`
- [x] `internal/api/schedules.go`: GET/POST `/api/schedules`, POST `/api/schedules/{id}/toggle`, DELETE `/api/schedules/{id}` (her yazımda Scheduler.Reload)

**Frontend**
- [x] `types.ts`/`api.ts`: Task/Run/Schedule tipleri + tüm uç metodları
- [x] `components/TaskBoard.tsx`: 5 sütunlu kanban, HTML5 sürükle-bırak ile taşıma, görev oluştur, ▶ Çalıştır, run geçmişi, sil
- [x] `components/Schedules.tsx`: cron preset'leri + serbest cron, görev/prompt seçimi, aç-kapa toggle, sil, sonraki/son çalışma
- [x] `App.tsx`: üstte Sohbet / Görevler / Zamanlamalar görünüm değiştirici

**CANLI TEST (API + Chrome):**
- [x] API: ajan→görev→çalıştır (claude-cli "Merhaba!"), board done'a geçti, run geçmişi yazıldı
- [x] API: cron `*/1 * * * *` schedule gerçekten tetiklendi, ajan prompt'a cevap verdi, schedule oturumuna kaydedildi
- [x] Chrome: Görevler sekmesinde form ile görev oluşturuldu → ▶ Çalıştır → claude-cli "4" → kart **Bitti** sütununa otomatik geçti, yeşil `● success` rozeti
- [x] Chrome: Zamanlamalar görünümü cron preset'leriyle render edildi

> Not: Cron 5-alanlı standart format (dk sa gün ay haftagünü). Çalıştırma claude-cli üzerinden anahtarsız.

---

## ESKİ: WORKSPACE İZOLASYONU TAMAMLANDI ✅ (CHROME'DA CANLI TEST GEÇTİ)

### Ara Faz — Workspace İzolasyonu ✅ (kullanıcı talebiyle Faz 5 öncesi eklendi)
Detaylı tasarım: [06-WORKSPACES.md](06-WORKSPACES.md)

- [x] `internal/workspace/manager.go`: her workspace = ayrı DB + ayrı runtime; workspaces.json kayıt defteri; Create/Delete/Get/List/Default/Close
- [x] `internal/api/server.go`: `withWorkspace` middleware (X-Workspace-Id header → context); workspace CRUD uçları
- [x] Tüm handler'lar workspace-scoped (`ws(r).DB`, `ws(r).Runtime`)
- [x] `main.go`: tek db/runtime yerine Manager
- [x] Frontend: `WorkspaceSwitcher.tsx`, api.ts header injection + localStorage, App.tsx workspace state + geçişte tam reset
- [x] **CANLI TEST (API + Chrome):** WS1=Ajan-A, WS2=Ajan-B; her workspace yalnızca kendi verisini görüyor; UI switcher ile geçiş çalışıyor; ayrı .db dosyaları ✅

### Mimari karar
Tek DB + workspace_id kolonu yerine **fiziksel ayrım** (workspace başına ayrı swarmgo.db) → sıfır sızıntı riski.

---

## ESKİ: Faz 4 TAMAMLANDI ✅ (OTONOM HEARTBEAT CANLI TEST GEÇTİ)

### Faz 4 — Agent Runtime ✅
- [x] Migration `0002_heartbeat.sql`: agents'a heartbeat_enabled/interval/prompt sütunları
- [x] `internal/db/store.go`: scanAgent/scanSession refactor, UpdateHeartbeat, GetOrCreateHeartbeatSession
- [x] `internal/agent/worker.go`: per-agent goroutine, heartbeat ticker, wake/stop channel, exponential backoff, 10 hatada otomatik devre dışı
- [x] `internal/agent/runtime.go`: yaşam döngüsü yöneticisi (Start/Stop/Wake/Status/StartConfigured/StopAll) + heartbeat eylemi (LLM çağrısı → heartbeat oturumuna yaz)
- [x] `internal/api/runtime.go`: GET /api/runtime, POST /api/agents/{id}/heartbeat, POST /api/agents/{id}/wake
- [x] `main.go`: boot'ta StartConfigured, shutdown'da StopAll
- [x] **CANLI TEST:** ajan 5sn aralıkla kendi kendine uyandı, Claude'u çağırdı, "Sen yapabilirsin, asla pes etme!" cevabını heartbeat oturumuna yazdı; runtime durumu success ✅

### Yeni API uçları
| Metod | Yol | Açıklama |
|-------|-----|----------|
| GET | `/api/runtime` | Tüm worker'ların durumu |
| POST | `/api/agents/{id}/heartbeat` | Heartbeat aç/kapat + ayarla |
| POST | `/api/agents/{id}/wake` | Anlık uyandır |

### Runtime mekanikleri
- Her ajan = 1 goroutine; kontrol kanalları (wake/stop) ile yönetilir
- Outcome classification: success → failures sıfırlanır; failure → artar, backoff uygulanır
- Exponential backoff: `base * 2^failures` (maxBackoffSteps ile sınırlı)
- 10 ardışık hata → ajan otomatik devre dışı (Disabled=true)

### ⏳ Sonraki (Faz 4 artıkları, opsiyonel)
- [ ] Heartbeat durumunu UI'da göster (runtime paneli)
- [ ] claude-cli için araç kısıtlama (`--allowedTools ""`) — saf chat güvenliği

---

## ESKİ: Faz 3 TAMAMLANDI ✅ (CHROME'DA CANLI TEST GEÇTİ)

### Faz 3 — React Web UI ✅
- [x] `frontend/`: Vite + React + TypeScript + Tailwind v4 kuruldu
- [x] `vite.config.ts`: Tailwind eklentisi + backend proxy (:8090)
- [x] `src/index.css`: özel koyu tema (@theme değişkenleri)
- [x] `src/types.ts`, `src/api.ts`: tip güvenli API istemcisi
- [x] `src/components/Sidebar.tsx`: ajan listesi + oluşturma formu + oturumlar
- [x] `src/components/MessageList.tsx`: mesaj balonları + typing animasyonu + auto-scroll
- [x] `src/components/Composer.tsx`: mesaj girişi (Enter ile gönder)
- [x] `src/App.tsx`: durum yönetimi, optimistic UI, hata gösterimi
- [x] `npm run build`: TypeScript temiz derlendi
- [x] **CHROME CANLI TEST:** ajan seç → geçmiş yüklendi → yeni mesaj "3+7" → cevap "10." ✅
- [x] Türkçe karakterler UI'da kusursuz (PowerShell mojibake'si sadece konsoldaymış)

### Çalıştırma (geliştirme)
```powershell
# Terminal 1 - backend
$env:SWARMGO_ADDR=":8090"; go run ./cmd/swarmgo
# Terminal 2 - frontend
cd frontend; npm run dev   # http://localhost:5173
```

### ⏳ Sonraki iyileştirmeler
- [ ] Streaming (WebSocket ile token token akış)
- [ ] Frontend build'i Go binary'sine embed (tek dosya dağıtım)

---

## ESKİ: Faz 2 TAMAMLANDI ✅ (CANLI TEST GEÇTİ)

### 🎉 Önemli: API anahtarı OLMADAN çalışıyor (claude-cli provider)
SwarmClaw'un "CLI provider" yaklaşımı eklendi. Yerel `claude` (Claude Code) CLI'ı
kullanıcının OAuth/abonelik girişiyle çalışır — **API anahtarı gerekmez.**

- [x] `internal/providers/claudecli.go`: `claude -p --output-format json` ile shell-out
- [x] `internal/providers/registry.go`: claude CLI otomatik tespit (exec.LookPath)
- [x] Yeni ajanların varsayılan provider'ı: `claude-cli`
- [x] **CANLI TEST:** Çok turlu sohbet çalıştı, hafıza korundu, mesajlar DB'ye yazıldı ✅
- [x] Çözülen sorun: `--bare` flag'i keychain okumasını atlayıp "Not logged in" veriyordu → kaldırıldı

### İki provider seçeneği (ajan başına seçilebilir)
| Provider | Auth | Maliyet | Persona kontrolü |
|----------|------|---------|------------------|
| `claude-cli` | OAuth/abonelik (anahtarsız) | Abonelik dahili | Claude Code kimliği taban + append |
| `anthropic` | API anahtarı | Token başına ücret | Tam temiz kontrol |

### ⚠️ Bilinen kısıtlar / sonraki iyileştirmeler
- claude-cli, Claude Code'un taban sistem prompt'unu (≈23k token) yükler → her çağrı bu yükü taşır
- claude-cli print modunda araç (Bash/Edit) erişimi olabilir → saf chat için `--allowedTools ""` ile kısıtlanmalı
- Streaming henüz yok (Faz 3'te WebSocket)

---

## ESKİ: Faz 2 KOD TAMAM

### Faz 2 — Anthropic Provider + Chat MVP ✅ (kod)
- [x] `internal/db/models.go`: Agent, Session, Message tipleri
- [x] `internal/db/store.go`: CRUD (CreateAgent/GetAgent/ListAgents, sessions, messages, transaction'lı AddMessage)
- [x] `internal/providers/provider.go`: ortak `Provider` arayüzü + Message/Request/Response
- [x] `internal/providers/anthropic.go`: ince HTTP istemci (Messages API, SDK'sız)
- [x] `internal/providers/registry.go`: sağlayıcı seçimi (anthropic)
- [x] `internal/api/server.go`: router + CORS + health
- [x] `internal/api/agents.go`, `sessions.go`, `chat.go`: HTTP handler'ları
- [x] `main.go` API sunucusuyla bağlandı
- [x] Test: ajan/oturum/mesaj akışı ✅; chat anahtarsız güvenli hata veriyor ✅

### ⏳ Açık iş
- [ ] **Canlı chat testi:** `ANTHROPIC_API_KEY` verilince gerçek Claude cevabı doğrulanacak
- [ ] Streaming (Faz 3'te WebSocket ile)

### API Uçları (mevcut)
| Metod | Yol | Açıklama |
|-------|-----|----------|
| GET | `/health` | Sağlık + db |
| GET/POST | `/api/agents` | Ajan listele/oluştur |
| GET/POST | `/api/sessions` | Oturum listele/oluştur |
| GET | `/api/sessions/{id}/messages` | Mesaj geçmişi |
| POST | `/api/chat` | Sohbet turu (Claude) |

---

## ESKİ: Faz 1 TAMAMLANDI ✅

### Faz 1 — Veritabanı ve Config ✅
- [x] `internal/config/config.go`: env okuma, dizin çözümleme, DBPath
- [x] `internal/config/secret.go`: AES-GCM credential şifreleme (env→dosya→üretim)
- [x] `internal/db/db.go`: SQLite (modernc.org/sqlite, CGO yok), WAL + busy_timeout + FK
- [x] `internal/db/migrate.go`: embed.FS migration runner + schema_migrations takibi
- [x] `internal/db/migrations/0001_init.sql`: 11 tablo (agents, sessions, messages, tasks, schedules, runs, connectors, knowledge_sources, skills, mcp_servers, provider_configs)
- [x] `main.go` config+db ile bağlandı; `/health` artık `db:true` döndürüyor
- [x] `.gitignore` eklendi
- [x] Test: DB oluştu, migrate geçti, health `db:true` ✅

---

## ESKİ: Faz 0 TAMAMLANDI ✅

### Tamamlananlar ✅
- [x] Go 1.26.4 kuruldu ve doğrulandı
- [x] Node.js v24 + npm 11 mevcut (frontend için hazır)
- [x] Proje klasör yapısı oluşturuldu (`C:\Users\user\Desktop\Projects\SwarmGo`)
- [x] `go mod init github.com/bilal/swarmgo`
- [x] `_Docs` plan dokümanları yazıldı (00-05)
- [x] `cmd/swarmgo/main.go`: HTTP sunucu + `/health` ucu (graceful shutdown, slog)
- [x] `go build` + `go vet` temiz
- [x] Çalıştırma testi: `/health` → `{"status":"ok"}` ✅
- [x] Kök `README.md` (Türkçe)

### Sıradaki Adımlar ⏳ (Faz 1)
- [ ] Wails v2 CLI kurulumu (Faz 9'a kadar ertelenebilir)
- [ ] `internal/db`: SQLite (modernc.org/sqlite) bağlantısı
- [ ] Migration runner + `0001_init.sql`
- [ ] `internal/config`: env + credential secret (AES-GCM)

### Sonraki Faz: Faz 1 (Veritabanı ve Config)
Detaylar için bkz. [03-YOL-HARITASI.md](03-YOL-HARITASI.md)

---

## Karar Bekleyen Konular

Kullanıcıyla netleştirilecek:
1. **İlk LLM sağlayıcısı:** Anthropic (Claude) mı, OpenAI mı, yoksa yerel Ollama mı?
2. **Frontend framework:** React (varsayılan) mı, Svelte mi?
3. **MVP kapsamı:** Tek ajan + chat ile mi başlayalım?

---

## Oturum Günlüğü

### 2026-06-16 — tool_delta/tombstone gerçek üreticileri (tool-streaming + iptal)
`tool_delta` ve `tombstone` artık altyapı değil, **canlı üreticili** (`go build`/`vet`/`test ./...` yeşil):

1. **`tools.StreamingTool`** arayüzü (`CallStream(ctx,input,onChunk)`) + `Registry.CanStream`/`CallStream` (`registry.go`). Akan araç yoksa düz `Call`'a düşer.
2. **`shell` aracı akıyor** (`builtin_shell.go`): `Call` → `CallStream`'e taşındı; `shellStreamWriter` stdout+stderr'i (tek writer, exec serialize eder) hem 64KB cap'li buffer'a yazar hem `onChunk`'a iletir.
3. **`toolloop.go` bağladı:** akan araç + canlı sink varsa `StepToolDelta` (`ID`=call.ID) yayılır; tamamlanınca `StepTombstone` (`Ref`=call.ID) placeholder'ı geri çeker; tool sonrası `ctx.Err()` varsa `fail("cancelled")` → `StepError` ve tur temiz biter (yarım sonuç modele beslenmez).
4. **Frontend** (geçen turda hazırdı): App.onStep `tool_delta`'yı `ID` ile merge, `tombstone`'u filtreler; `ToolDeltaStep.tsx` canlı çıktı kartı. `stepKinds.ts` durumları `infra`→`active`.
5. **Test:** `builtin_shell_test.go` — `CanStream("shell")` + `CallStream` onChunk parça + tam çıktı.

> Doküman: `10-KAVRAMSAL` E3 (artık tüm kind'lar üreticili) + SKILL güncellendi. Mekanizma genel: uzun MCP çağrıları da aynı `StreamingTool` yoluna takılabilir.

### 2026-06-16 — Trace StepKind genişletme #2: error/steer/tool_delta/tombstone + Ayarlar referans ekranı
4 yeni `StepKind` eklendi (`go build`/`vet`/`test ./...` + frontend `tsc`/`build` yeşil; canlı UI testi mcp-chrome stale-sekme/screenshot kırılganlığı nedeniyle güvenilir alınamadı, otomatik kontroller esas):

1. **`error`** (`StepError`) — tur düzeyinde hata (sağlayıcı/bütçe/iptal); `toolloop.go` `fail()` budget_exceeded/provider_error yollarında yayar → `ErrorStep.tsx` (kırmızı + reason rozeti). Araç hatasından (tool+isError) ayrı.
2. **`steer`** (`StepSteer`) — canlı yönlendirme artık `StepText` "↪" prefix'i yerine ayrı tip → `SteerStep.tsx`.
3. **`tool_delta`** (`StepToolDelta`) — akan tool çıktısı (aynı `ID` birleşir, yalnız-canlı) → `ToolDeltaStep.tsx`; App.onStep `id` ile merge eder. **Altyapı hazır, üretici yok** (tool-streaming gelince).
4. **`tombstone`** (`StepTombstone`) — `Ref` ile canlı bir adımı geri çeker (render edilmez); App.onStep filtreler. **Altyapı hazır.**
- `TurnStep`'e `ID`/`Ref` alanları. Tek-kaynak referans `frontend/src/lib/stepKinds.ts` (kind/etiket/ikon/kalıcı?/durum/açıklama) → **Ayarlar ▸ Adım Türleri** read-only ekranı (`SettingsPanel.tsx` yeni `stepkinds` kategorisi).
- Testler: `trace_test.go` error/steer/tool_delta/tombstone JSON round-trip.

> Doküman: `10-KAVRAMSAL` E3 + SKILL güncellendi.

### 2026-06-16 — UI tema yenileme (Design Refresh)
Arayüzün görsel dili cilalandı (`go build`/`vet` yeşil; backend `themePreset` kalıcılığı API round-trip ile, yeni tema + preset grid Chrome'da canlı doğrulandı):

1. **Token genişletme** (`frontend/src/index.css`): semantic renkler (`--color-success/warning/danger`), elevation (`--shadow-sm/md/lg`), `--radius`, klavye için tutarlı `:focus-visible` ring. Light tema bu token'ları kendi değerleriyle ezer. **Inter** (gövde) + **JetBrains Mono** (kod/yol) yerel `@fontsource-variable` fontları import edildi; `body::before` ile köşelerde `radial-gradient` + `color-mix` tabanlı **hafif accent glow**.
2. **Hazır tema paletleri** (`frontend/src/lib/themePresets.ts` — yeni): 8 küratörlü palet — Gece Moru (varsayılan), Arduvaz, Zümrüt, Gül, Kehribar, Nord, Gün Işığı, Solarized Açık. Her preset tam token seti (bg/surface/surface2/border/accent/accentSoft/text/textDim + opsiyonel semantic) + `dark` bayrağı taşır.
3. **`applyTheme` yeniden yazıldı** (`frontend/src/lib/theme.ts`): imza `applyTheme(theme, accent, preset)`. Preset seçiliyse tüm paleti `<html>` inline style'a basar (stylesheet'i ezer) ve dark/light'ı `data-theme`'den ayarlar; preset yoksa eski theme(dark/light/system)+accent yoluna düşer. Her iki yolda da boş olmayan `accent` accent token'ını override eder. `App.tsx` `applyClientPrefs` artık `themePreset`'i geçirir.
4. **Backend** (`internal/settings/settings.go` + `store.go`): `ThemePreset` alanı — Settings/DTO/Patch/Apply/Default(`midnight-violet`). `GET/PUT /api/settings` ile kalıcı (yeni uç yok, mevcut DTO genişletildi).
5. **SettingsPanel** (`frontend/src/components/SettingsPanel.tsx`): Görünüm sekmesine **swatch'lı palet seçici grid** (`grid-cols-2 sm:grid-cols-3`); bir preset tıklanınca `themePreset` + `accent` o paletin rengine senkronlanır. Eski tema dropdown'ı "Temel mod" olarak kaldı (preset yokken/sistem için).
6. **NavRail** (`frontend/src/components/NavRail.tsx`): emoji ikonlar → **lucide-react**; aktif öğe dolgu-mor yerine `accent-soft` tint + 3px sol indicator (`navItemClass`/`ActiveBar`); gradient marka rozeti (`color-mix`). *(Aynı dosya eşzamanlı "Ajanlar/Artifactlar görünümü" refactor'uyla `agents`/`artifacts` nav öğeleri eklenerek birleşti.)*

> Yeni bağımlılıklar: `lucide-react`, `@fontsource-variable/inter`, `@fontsource-variable/jetbrains-mono`. **Not:** Bu iş, eşzamanlı yürüyen "Ajanlar görünümü + Sessions sidebar bölme + Artifactlar" refactor'uyla aynı çalışma ağacında; o iş yarımken `tsc -b` geçici hata verir. Commit kullanıcı onayına bırakıldı.

### 2026-06-16 — Trace StepKind genişletme: `todo` + `recovery` (E3 düşük-efor)
`StepKind` ilk-sınıf hale getirildi (`go build`/`vet`/`test ./...` + frontend `tsc` yeşil):

1. **`todo` kind** (`agent/trace.go` `StepTodo` + `TurnStep.Todos []TodoItem` + `parseTodos`): `todo_write` aracı artık generic tool kartı yerine `StepTodo` adımı yayar (`toolloop.go` çağrı sonrası dönüştürür) → frontend `TodoCard.tsx` `step.todos`'u önceler (eski trace'ler için `step.input` fallback'i + tool-adı geriye-dönük render korunur).
2. **`recovery` kind** (`StepRecovery` + `TurnStep.Reason`): tool döngüsü iterasyon limitine (`maxToolIters`) ulaşınca `reason:"max_tool_iterations"` adımı yayılır → frontend `RecoveryStep.tsx` (amber uyarı satırı + reason rozeti). Tur neden erken bittiğini açıklar.
3. **Frontend:** `types.ts` `StepKind` union + `TodoItem` tipi; `TurnSteps.tsx` `todo`/`recovery` yönlendirmesi.
4. **Testler:** `agent/trace_test.go` (`parseTodos` geçerli/bozuk + todo/recovery JSON round-trip).

> Kalan StepKind adayları (sonraki): `subagent` (A2 ile), `tombstone`/`tool_delta` (canlı güncelleme altyapısı, P3). Bkz. `10-KAVRAMSAL-TASARIM-NOTLARI.md` E3.

### 2026-06-16 — Etkileşim araçları: `todo_write` + `ask_user` (E3/E2/E1)
`observed-behavior` mimari incelemesinden (`_Docs/10-KAVRAMSAL-TASARIM-NOTLARI.md`) çıkan **etkileşim katmanı** ilk iş paketi uygulandı (`go build`/`vet`/`test ./...` + frontend `tsc`/`build` + canlı tool-katalog smoke testi yeşil):

1. **E3 — Trace modeli genişletildi** (`internal/agent/trace.go`): yeni `StepAsk` (`"ask"`) kind'ı (delta gibi geçici, kalıcı değil) + `TurnStep.Options []string` (tıklanabilir öneri yanıtlar).
2. **E2 — `todo_write` aracı + UI** (`internal/tools/builtin_todo.go`): ajan tam görev listesini her seferinde yayınlar (`pending|in_progress|completed`); araç çağrısı yalnızca özet metin döner, listeyi frontend `TodoCard.tsx` canlı checklist olarak render eder (tool adına göre özel-durum, `TurnSteps.tsx`). Sunucu tarafında durum tutulmaz.
3. **E1 — `ask_user` (suspend/resume)** (`internal/tools/builtin_ask.go` + `ask.go`): ajan soruyu sorar ve **açık SSE stream üzerinden bloklar**. Tam suspend/resume yerine context-kanal deseni: `tools.WithAsker(ctx, fn)` → `chat_stream.go` geçici `StepAsk` yayar, `run.answer` kanalında bekler; kullanıcı `POST /api/chat/control {action:"answer"}` ile yanıtlar (`chat_control.go`). İnteraktif olmayan (heartbeat/scheduler) koşularda asker yok → araç hata döndürür, model kendi devam eder.
4. **Frontend:** `AskPrompt.tsx` (soru + tıklanabilir seçenekler + serbest metin) composer üstünde gösterilir; `App.tsx` `pendingAsk` state'i + `answerAsk` callback'i. `lib/tools.ts` ikonları (✅ todo_write, 💬 ask_user).
5. **Testler (yeni):** `internal/tools/builtin_interaction_test.go` — todo_write geçerli/geçersiz-durum/boş, ask_user asker-yok/asker-var/boş-soru.

> Doküman: `10-KAVRAMSAL-TASARIM-NOTLARI.md` güncellendi (B-Ek + D2-streaming "yapıldı" işaretlendi; E1/E2 tamamlandı). Kalan etkileşim işi: `ask_user` kalıcı tool kartı için özel render (şu an generic ActivityCard) ve `EnterPlanMode`/`ExitPlanMode` (plan modu).

### 2026-06-16 — Modülerlik refactor'ları (davranış değişmedi)
Dört adet düşük-riskli, davranış-korumalı refactor uygulandı (`go build`/`go vet`/`go test ./...` + canlı `/health` smoke testi yeşil):

1. **`providers/transport.go` (yeni):** Ortak `postJSON` HTTP yardımcısı. `anthropic.go` ve `minimax.go`'daki tekrar eden marshal → request → header → `Do` → `ReadAll` → unmarshal iskeleti tek noktaya alındı.
2. **`api/server.go` → `writeDBError`:** Her handler'da tekrar eden `db.ErrNotFound → 404 / diğer → 500` eşlemesi merkezîleştirildi; `agents/sessions/tasks/schedules/memory/usage` handler'ları sadeleşti. (`handleRunTask` 400 semantiği korunarak hariç tutuldu.)
3. **`agent/tunables.go` global state → `Tunables` struct:** Süreç-geneli paket globalleri kaldırıldı; tek `*Tunables` örneği `main.go`'da oluşturulup `NewManager`/`NewRuntime`/`NewServer` üzerinden enjekte ediliyor. `budget.go`/`titler.go` artık `r.tun` kullanıyor.
4. **`db/store.go` → `mutateAgentLocked`/`mutateSessionLocked`:** Kilitle→bul→değiştir→persist kalıbı generic yardımcılara alındı; `UpdateAgent`, `UpdateHeartbeat`, `UpdateBudget`, `UpdateAgentTools`, `SetSessionSummary`, `SetSessionTitle` sadeleşti.

> Not: `tunables.go` artık `Set*` yerine `*Tunables` metotları sunuyor (önceki satır 133'teki paket-fonksiyon imzaları değişti).

#### Devam refactor'ları (aynı gün)
5. **`agent/climcp.go` (yeni):** claude-cli `--mcp-config` üretimi (`cliMCPConfig`/`cliMCPServer` + `writeCLIMCPConfig`) `toolloop.go`'dan ayrı, kohezyonlu bir dosyaya taşındı. `ToolCatalog` registry'nin yanına (`toolsetup.go`) alındı. `toolloop.go` artık yalnızca completion/agentic-loop mantığına odaklı. (Saf kod taşıma — davranış değişmedi.)
6. **`api/server.go` → `register*Routes`:** Tek `Routes()` bloğu domain-bazlı 13 yardımcıya bölündü (`registerAgentRoutes`, `registerTaskRoutes`, …).
7. **Testler (yeni):** `providers/transport_test.go` (`postJSON`: başlık/gövde, non-200, decode hatası, transport hatası), `providers/minimax_test.go` (httptest ile `Complete` başarı/hata/varsayılan baseURL), `agent/tunables_test.go` (`*Tunables` get/set + eşzamanlı erişim), `api/server_test.go` (rota kaydı panik regresyon koruması).

> Repo hijyeni: `internal/` altında 36 kaynak dosyası henüz versiyon kontrolüne hiç girmemiş durumda (HEAD tek başına derlenmez). Tüm kaynak ağacını tek seferlik "track existing sources" commit'iyle eklemek önerilir.

### 2026-06-15
- Proje başlatıldı.
- SwarmClaw mimarisi analiz edildi, Go karşılıkları belirlendi.
- Klasör yapısı + go.mod + plan dokümanları oluşturuldu.
