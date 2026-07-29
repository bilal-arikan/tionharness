# TionSwarm — İlerleme Takibi

> Bu dosya canlı tutulur; her oturumda güncellenir. Son güncelleme: **2026-07-28**

## Sohbet süre/zaman bilgileri artık sunucu-otoriter ✅ (2026-07-28)

Balon altbilgisindeki "⏱ süre" ve canlı sayaç frontend'de türetiliyordu; ikisi de
sunucuya taşındı.

- **Tamamlanmış tur süresi:** `MessageList` `asistan.createdAt − önceki kullanıcı
  mesajı.createdAt` hesaplıyordu. Oysa backend turu zaten ölçüp `Message.DurationMs`
  olarak kaydediyordu (`chat_stream.go` `agentStart`; otonom yollarda `turnmeta.apply`)
  ve frontend bu alanı **hiç kullanmıyordu**. Artık `durationMs` gösteriliyor; saniye
  yuvarlaması yerine ms hassasiyeti (`formatDurationMs`, <10 sn'de "3.4 sn"). Eski
  türetim yalnız bu alandan önce yazılmış mesajlar için fallback ve "~" ile
  **yaklaşık** işaretleniyor (sessizce doğruymuş gibi gösterilmiyor).
- **Canlı sayaç + tur başlangıcı:** `LiveTimer` istemci `Date.now()` kullanıyor,
  tur başlangıcı da istemcide damgalanıyordu (`chatStreamHub` `AgentStart`). Sonuç:
  (a) saati kaymış istemcide süre yanlış, (b) tur ortasında açılan/yenilenen pencere
  turu **gördüğü ilk adımdan** saymaya başlıyordu. Artık başlangıç `agent_start` hub
  olayının sunucu `time` damgası (durable/ringed → replay'de gerçek başlangıç gelir).
- **Yeni `shared/lib/serverClock.ts`:** hub frame'lerinin `time` alanı + `hello`
  frame'ine eklenen `now` alanından skew tahmini (≥2 sn farkta adopte → ağ jitter'ı
  sayacı zıplatmaz); `serverNow()` tek "şimdi" kaynağı. Müşteriler: `LiveTimer`,
  `WorkerWaitBanner`, `WakeWaitBanner`, prompt-cache sıcaklık geri sayımları
  (`SessionDetailPanel`/`SessionContextModal`). Mutlak saat etiketleri (`MessageTime`)
  bilerek yerel saat diliminde kalır. Test: `serverClock.test.ts` (6/6).

Detay `07-CHAT-UX.md` + `58-QUEUE-SENKRON.md`.

## Spawn/zamanlama süre ayarları gerçekten kaydediliyor + tavan 2 saat ✅ (2026-07-28)

**Bulgu (SES17):** bir worker turu tam `1.199.905 ms` = 20 dk'da kesildi. Sebep
`SpawnTimeout` hard-cap'i; idle watchdog (5 dk) tetiklenmemişti çünkü worker sürekli
araç çağırıyordu (68 tool call). Kök neden ayarın **hiç uygulanamaması**:

- **Plumbing açığı:** `spawnTimeoutMin` / `spawnIdleTimeoutMin` / `scheduleTimeoutMin`
  `Settings` ve `DTO`'da vardı, Ayarlar UI'ında alanları da vardı ve PUT gövdesinde
  gönderiliyordu — ama `settings.Patch` struct'ında **yoktu** → sunucu değeri sessizce
  yutuyordu. UI alanı boot'tan beri no-op'tu. Düzeltildi: `Patch` alanları +
  `applyPatch` `applyInt` çağrıları + `normalize` clamp'leri (süreler 1–1440 dk;
  `spawnIdleTimeoutMin` hard-cap'i **aşamaz**, yoksa watchdog hiç ateşlemez).
- **Yeni değer:** `spawnTimeoutMin` = **120 dk** (global `~/.tionswarm/settings.json`
  → tüm workspace'ler; `internal/settings` app-seviyesi, workspace-scoped değil).
  Kod default'u (`DefaultSpawnTimeoutMinutes = 20`) değişmedi — yalnız fallback.
## Kesilen worker turu artık "completed" diye raporlanmıyor ✅ (2026-07-28)

Yukarıdaki SES17'nin ikinci yarısı: tur 20 dk'da kesildiği hâlde koordinatöre
`<status>completed</status>` + yarım cümlelik `<result>` gitti, koordinatör işi
bitmiş sandı. **Sebep:** kesilme hiçbir yerde hata olmuyor — claude-cli
`salvage()` kısmi metni `err=nil` ile döndürüyor, native döngü de iterasyon
limiti/guardrail halt/bağlam tükenmesinde `StepRecovery` ekleyip `nil` dönüyor.

- **`withActivityTimeout` → `context.WithCancelCause`** (`activity_timeout.go`):
  yeni sentinel'ler `ErrTurnHardTimeout` / `ErrTurnIdleTimeout`. Öncesinde
  watchdog iptali ile kullanıcının "Durdur"u **ayırt edilemiyordu** (ikisi de
  `context.Canceled`), yani süre dolması "killed" olarak da raporlanabilirdi.
- **Yeni `internal/agent/turnoutcome.go`** — `classifyTurnOutcome(ctx, steps,
  hard, idle)`: ctx cause + trace'teki **terminal** `termReason` işaretlerini
  okur (mid-turn `contReason`'larla çakışmaz) → `timeout` / `incomplete` /
  `completed`. `applyTurnOutcome` kurtarılan metnin **önüne** ne olduğunu
  anlatan Türkçe not koyar (metin atılmaz — işin nereye kadar geldiğinin tek
  kaydı).
- **`runWorker`** artık bu verdikti kullanıyor; deadline kontrolü
  `context.Canceled` kontrolünden **önce** gelir. Kesik turda ayrıca
  `debug.jsonl`'e `error` olayı düşer ve log'a `worker: turn truncated` yazılır.
- **Kapsanan durumlar:** süre tavanı, boşta watchdog'u, **araç iterasyon limiti**
  (`maxToolIters`, vars. 500), guardrail halt, bağlam penceresi ve çıktı-token
  tükenmesi.
- **`runSpawn` de aynı verdikti kullanıyor** (worker'sız detached spawn'lar):
  kesik turda (a) kurtarılan metnin önüne not, (b) trace'e `turn_timeout`
  `StepRecovery` (`appendOutcomeStep` — watchdog döngünün DIŞINDA iptal ettiği
  için başka hiçbir şey iz bırakmıyordu; loop'un kendi `term*` işaretleri
  tekrarlanmaz), (c) `spawn` olayı **başarısız** seviyesinde yayınlanır,
  (d) `ClearParentTagsOnSuccess` **çalıştırılmaz** — yarıda kalan bir onarım
  ajanı ebeveynin `error` etiketini temizleyemez, sınırlı onarım döngüsü
  tekrar denesin. Hata dalında ham `context canceled` yerine hangi tavanın
  dolduğunu söyleyen not kaydedilir.
- **Testler:** `turnoutcome_test.go` (8 vaka: temiz tur, hard/idle timeout,
  kullanıcı-stop'un timeout sayılmaması, 4 terminal işaret, mid-turn retry'ın
  turu bozmaması, iz adımı tekrarlanmaması, not birleştirme). Full suite +
  `go vet` yeşil.

Detay `_Docs\47` ▸ "task-notification formatı".

## Transkript açılış maliyeti: iz kırpma + off-screen render atlama ✅ (2026-07-28)

**Şikâyet:** bir worker oturumunu her açtığında adımlar/tool kullanımları "baştan
yükleniyor" gibi geliyordu. **Ölçüm sonucu varsayım yanlıştı:** sunucu zaten
cache'liyor — `db.loadSessions` boot'ta tüm `session.jsonl`'leri belleğe alıyor,
`ListMessages` disk'e hiç gitmiyor. Gerçek maliyet (a) wire payload'ı, (b) React
render'ıydı. İkisi de hedeflendi:

- **Sunucu-tarafı iz kırpma** — yeni `internal/api/steps_trim.go`: okuma yolunda
  `output`/`text`/`patch` + `input` yaprakları 2 KB'a kesilir, `*Truncated`/`*Len`
  bayrağıyla işaretlenir, `subSteps` özyinelemeli. `input` anahtarları korunur
  (diff sentezi + tool etiketi onlara bağlı). Disk ve model bağlamı **tam kalır**.
  `handleListMessages` + `publishHub(KindReply)`'a bağlandı. Gerçek 11-oturumlu
  workspace'te iz payload'ı **~%45** küçüldü.
- **Talep üzerine tam iz** — `GET /api/sessions/{id}/messages/{msgId}/steps` +
  turun 🔧 satırındaki **"⤓ tam iz"** çipi.
- **Off-screen render atlama** — `MessageList` satırlarına `content-visibility:auto`.
  Virtualizer bilerek kullanılmadı: scroll/deep-link mantığı `data-msg-id` ile
  gerçek DOM'u sorguluyor, unmount hepsini bozardı. 30 satırdan kısa transkriptte
  devre dışı; son 3 satır eager; atlamalar rAF'ta ikinci kez hizalanır.
- **Memoizasyon** — `AssistantTurn`/`UserTurn`/`PeerTurn`/`TurnSteps`/`ActivityCard`
  `React.memo`, `parseSteps` `useMemo`'ya alındı (her delta'da yüzlerce KB JSON.parse
  ediyordu), handler kimlikleri `shared/lib/useStableCallback.ts` ile sabitlendi.

Detay `_Docs\07-CHAT-UX.md` ▸ "Transkript yükleme maliyeti".

Yan bulgu: `SkillImportDialog.tsx`'te `KIND_LABEL` haritası `hook` ingest türünü
kaçırdığı için `npm run build` kırıktı (bu değişiklikle ilgisiz) — eklendi.

## Oturum-hedefi (goal) mekanizması tamamen kaldırıldı ✅ (2026-07-28)

Kaldırıldı çünkü **yarım bir özellikti**: bir kuzey-yıldızı metnini her turun
dinamik suffix'ine enjekte ediyordu, ama "bitti"ye işi yapan modelin kendisi karar
veriyordu (`complete_goal`) ve hiçbir şeyi sürmüyordu — `autocontinue.go` goal'den
tamamen habersizdi. Claude Code'un `/goal`'ü ise bir **kontrol akışıdır**: ayrı bir
değerlendirici model her turdan sonra transkripti okuyup durma koşulunu yargılar ve
karşılanmadıysa yeni tur başlatır. İleride bu şekilde ayrı bir oturumda yeniden
kurulacak; o zamana kadar yarı-uygulanmış hali beklenti yaratıp karşılamıyordu.

- **Silinen dosyalar:** `internal/agent/goal.go` (+test), `internal/api/goal.go`,
  `internal/tools/goalsink.go`, `frontend/src/features/sessions/SessionGoalSection.tsx`.
- **Veri modeli:** `db.Session.Goal` / `GoalDone` + `SetSessionGoal` kalktı. Diskteki
  eski `goal` alanları JSON'da öylece kalır ve **yok sayılır** (migration gerekmez).
- **Prompt:** `goalContextBlock` enjeksiyonu (chat + otonom yol) ve statik prefix'teki
  `GoalUsageHint` kalktı — `BuildSystemPrompt` artık salt persona (soul + identity).
- **Araç yüzeyi:** `update_session`'ın `goal` / `goal_done` alanları kalktı;
  `tools.SessionSink` artık `GoalSink`'i gömmüyor; `get_session_info` goal satırı yok.
- **Yan sistemler:** `PUT /api/sessions/{id}/goal` route'u, `SessionInfo.goal/goalDone`,
  bağlam ölçerdeki "Hedef" kovası, handoff'un `HandoffEnv.Goal` alanı ve
  **`goal` / `goal-done` auto-tag'leri** kalktı → bu etiketlere bağlı bir otomasyon
  varsa artık tetiklenmez (detay `46`).
- **Seed skill'ler:** `tionswarm-guide` ve `tionswarm-progress` artık ölü API'yi
  öğretmiyor; hedefin yeri `todo_write`/progress olarak yazıldı.
- Doğrulama: `go vet ./...` temiz; `go test` agent/tools/api/db/conversation/skills
  paketleri yeşil. `npx tsc --noEmit` bu değişikliklerde temiz.

  worker onu alınca boyanıyordu; yukarı kaydırmış kullanıcı o ana kadar hiçbir tepki
  görmüyordu.
- **Bekleme göstergesi:** bağımsız `WorkingDots` balonu artık `pending`'in yanı sıra
  `streaming` ile de çıkar, ve `App` oturum açılışında `GET /api/sessions/active`'ten
  o oturumu `markPending` ile tohumlar (yalnız EKLER — kuyruğa yeni atılmış tur
  sunucuda henüz kayıtlı olmayabilir). Çalışan bir oturum açıldığında transkript artık
  kendi mesajımızda bitip ajan susmuş gibi görünmüyor.
- **Coordinator workflow picker → satır başına (ⓘ) + 📖:** `<select>` bir radio
  listesine çevrildi (`<option>` buton taşıyamaz). Her recipe satırında kendi
  açıklama balonu (`recipeHelp` — frontmatter'daki `pattern`/`workerTargets`/
  `stopCondition`/`maxTurns`'ten üretilir) ve kendi skill'ini açan 📖 butonu var;
  etiket yanındaki (ⓘ) genel "workflow nedir" notu olarak kalır. Skills'e geçiş
  yeni `setSessionState` seam'i ile — seçim tohumlanır, Skills ekranı mount olurken
  okur → URL şeması değişmedi. Balonlar `fixed` modda: oturum paneli
  `overflow-hidden` olduğu için `absolute` balon kırpılıyordu; ayrıca
  `InfoPopover`'ın fixed dikey clamp'i artık uydurma 180px yükseklik yerine gerçek
  bir tavan (`max-h` + scroll) kullanıyor, uzun not viewport dışına taşmıyor.

## `shellOutputCompression=on` sessiz kalıyordu: DTO + ajan uyarısı ✅ (2026-07-28)

WS16'da sıkıştırmayı açarken iki ayrı sessiz boşluk çıktı:

- **API DTO'su alanı hiç taşımıyordu.** `PUT` diske doğru yazıyordu ama `GET` boş
  dönüyordu → Ayarlar ▸ Workspace seçici (`WorkspacePanel.tsx`, `ws.shellOutputCompression || ''`)
  gerçek değer `on`/`off` olsa bile **daima "auto"** gösteriyordu. `workspaceSettingsDTO`'ya
  alan eklendi.
- **Ajan `on` modunda bilgilendirilmiyordu.** `tokenOptimizerCapability` yalnız hook
  tarıyordu; `on` hook gerektirmediği için sıkıştırma çalışıp ajan uyarısız kalıyordu —
  sqz'nin kısaltmalı çıktısını truncation sanma riski. `Runtime.effectiveTokenOptimizers`
  artık in-process filtreyi de sayıyor (`*` matcher, daraltıcı kapsam notu yok); `off` ise
- `ShellTool` artık `preArgs` taşıyor, böylece `-c`'den önce ek argüman verilebiliyor.
- Git-bash tercih ediliyor çünkü Windows dosya sistemi + ağ yığınını paylaşır; diğer
  araçların (Read/Write/cwd) Windows yollarıyla tutarlı kalır.
- Testler: `builtin_shell_posix_test.go` — launcher tespiti, resolver'ın launcher'ı
  reddetmesi ve seçilen kabukta `X=..; Y=$(..)` genişletmesinin gerçekten çalışması.

## Bash aracı WSL launcher'ına düşünce `$VAR` sessizce boşalıyordu ✅ (2026-07-28)

Windows'ta `resolvePOSIXShell` PATH'teki ilk `bash`'i alıyordu; bu çoğu makinede
`C:\Windows\System32\bash.exe`, yani WSL launcher'ı. Launcher `-c` payload'ını gerçek
bash'e vermeden önce **bir dış kabukta expand ediyor** → script'in kendi atadığı
değişkenler ve `$(...)` sonuçları boş geliyor. Pratikte `T=$(cat tok.txt)` sonrası
`curl -H "Bearer $T"` boş kimlik gönderip 401 alıyordu; ajan bunu "kabuk değişkeni
genişletmesi bozuk" diye raporlayıp Python'a kaçıyor, orada da WSL'in ayrı network
namespace'i yüzünden Windows `127.0.0.1`'ine ulaşamıyordu (`Errno 111`).

- Resolver ayrı dosyaya alındı: `internal/tools/builtin_shell_posix.go`. Sıra artık
  PATH'teki bash (launcher değilse) → git-bash (PATH'te olmasa da git.exe'den ve standart
  kurulum yollarından bulunur) → son çare `wsl.exe -e bash` (argv'yi bozmadan geçirir).
  altına taşındı** → dolu şeritte yanlış tıklamayla kural silinmiyor.
- `data-testid`'ler korundu (`schedule-row`, `schedule-enable-toggle`,
  `schedule-run-now`, `schedule-edit`, `schedule-delete` — sonuncusu artık popup
  içinde, `schedule-create-*`); kartlara `automation-row`/`data-automation-id` eklendi.
- Detay: `46-ETIKET-OTOMASYON.md` ▸ "UI — 3 SÜTUNLU PANO".
## Frontend format otomasyonu (Prettier + pre-commit hook) ✅ (2026-07-29)

Frontend'in tek bir format otoritesi yoktu; elle tutarlı yazılıyordu ve config'siz bir
`npx prettier` çağrısı dosyaları çift tırnak/noktalı virgüle çevirip sahte diff üretebiliyordu.

- `frontend/.prettierrc.json` — `singleQuote` · `semi:false` · `printWidth:100` ·
  `trailingComma:all` · **`endOfLine:auto`** (CRLF çalışma kopyası + LF depo; `lf` deseydik
  106 dosya yalnızca satır-sonu yüzünden "farklı" görünürdü). `prettier` devDependency,
  `.prettierignore` (dist/node_modules/public/lock).
- Script: `npm run format` / `npm run format:check`.
- `.githooks/pre-commit` — **yalnız stage'lenmiş** dosyaları formatlar (frontend→prettier,
  `*.go`→gofmt) ve yeniden stage'ler; prettier **mutlak yolla** çağrılır (npm `.bin` hook'un
  düz `sh`'ında PATH'te değil). Etkinleştirme: `git config core.hooksPath .githooks` (yapıldı).
- `.vscode/settings.json` — kaydet-formatla + prettier/gopls eşlemesi.
- **Toplu format ATILMADI:** ağaçta 111 commit'lenmemiş dosya vardı, repo-geneli yeniden
  format gerçek diff'i gömerdi. `format:check` şu an ~248 dosyada uyarıyor; dokunulan dosya
  hook ile kendiliğinden dönüşür. Toplu geçiş temiz ağaçta kendi commit'inde yapılmalı.
- Kural CLAUDE.md'ye yazıldı ("config'siz prettier çalıştırma").

## Composer'da araç müfettişi — "ajan neyi kullanabiliyor?" ✅ (2026-07-28)

Sohbetten çıkmadan görülemiyordu: ajanın o an hangi araçlara sahip olduğu, hangilerinin
şemasının her tur gönderildiği (pahalı), hangilerinin yalnız katalogda durup gerektiğinde
açıldığı ve MCP gateway'de nelerin bağlı olduğu. Composer toolbar'ına 🔧 butonu eklendi;
açtığı panel **salt bilgi** — hiçbir ayarı değiştirmez (değişiklik yine Araçlar ekranından).

- Backend: `GET /api/agents/{id}/tool-access` (`internal/api/agent_tool_access.go`) —
  eager/lazy katalog + tier + ajan `blocked` listesi + MCP sunucu envanteri
  (etkin mi, transport/kapsam, canlı bağlantı sayısı, sunucu başına aktif/hazır adet).
- Frontend: `features/chat/composer/ToolAccessPanel.tsx` (üç sekme: Aktif · Talep üzerine ·
  MCP, arama kutusu), `ToolAccessList.tsx` (gruplu satırlar + sunucu listesi),
  `toolAccessGroups.ts` (saf grup/filtre). Built-in'ler kategoriye, MCP araçları sunucuya
## Otomasyon ekranı 3 sütunlu panoya çevrildi ✅ (2026-07-28)

Sekmeli görünüm (⏰ Zamanlamalar / 🏷 Etiket / 🗂 Pano) yerine **üç şeritli pano**:
üç kural türü artık aynı anda yan yana görünüyor, sekme değiştirmeye gerek yok.
Her şeridin başlığında sayaç + kısa açıklama + **`+`** butonu var; `+` o türün
**oluşturma popup'ını** açar. Kurallar salt-okunur kart; düzenleme de aynı
- Doğrulama: `go build ./...` + `go vet` + `go test ./internal/market/... ./internal/ingest/...`
  (19 test) yeşil, `npx tsc --noEmit` yeşil.
- **Kalan (içerik kararı):** şablonlar hâlâ yalnız agents + lineer flow + schedule taşıyor;
  automations (etiket/pano) için payload alanı bile yok, koordinatör rolü ve
  `await-input`/`subflow`/`spawn-join` düğümleri şablonla dağıtılamıyor.

## Git worktree izolasyonu kaldırıldı ✅ (2026-07-28)

Otonom oturuma per-session git worktree + dal veren `gitWorktreeIsolation`
çalışma-zamanı özelliği tamamen kaldırıldı — ileride kapsamlı biçimde yeniden
eklenecek (bkz. `41` madde 11: `EnterWorktree`/`ExitWorktree` ile birleşik tasarım).

- **Kaldırılanlar:** `internal/agent/worktree.go` (dosya silindi: `ensureWorktree` /
  `RemoveSessionWorktree` / `worktreesDir` / `gitRepoToplevel`); `Tunables`
  `gitWorktreeIsolation` alanı + `GitWorktreeIsolation()` getter; `SetWorkdirGuards`
  imzasından parametre düştü; `toolloop.go` otonom worktree dallanması; `sessions.go`
  oturum-silme temizlik çağrısı; `settings.go` (Settings/View/Patch) + `store.go`
  gerçek bir sqz hook'unu gizlemiyor. Testler: `capabilities_tokenopt_test.go`.

Ölçüm notu: WS16'nın üç worker oturumunda shell çıktısı toplamı ~40K char, sqz kazancı
**%24.8** (~2.5K token). Bağlamın asıl yükü `Write` (%39–44) ve `Read` (%16–34) — sqz
ikisini de kapsamıyor, yani shell sıkıştırma tek başına belirleyici değil.

  artık paketle taşınabiliyor** (önceden sessizce düşüyordu).
- **`NODE_ICON` 5 → 12 node tipi** (start/end/loop/await-input/subflow/spawn/join eklendi);
  workspace önizlemesindeki akış çipleri tüm non-lineer tipleri gösterir.
- **Ölü kod/tip temizliği** — kaldırılmış `memory` türüne ait yorumlar, `AgentPayload.capabilities`,
  kullanılmayan `SourceWorkspace` tier'ı (frontend `PackSource` + `SOURCE_LABEL` dâhil).
  Eklenenler: `toolOverrides` (TS), `prompts`/`readme` (TS + workspace önizlemesinde bölüm).
- **Gömülü 5 şablona `version: 1.0.0`** → `updateAvailable()` artık gömülülerde de çalışır.
- **Varsayılan kategori `skill` → `workspace`** (gömülü skill paketi 0 olduğu için market
  ilk açılışta boş ızgara gösteriyordu).
  `applyBool` alanları; frontend `types/settings.ts` + `AppToolsPanel` toggle'ı +
  `SettingsPanel` gönderimi.
- **Korunanlar:** `autonomousConfine` freni ve **geliştiriciye ait** `scripts\worktree.ps1`
  yardımcısı (tamamen ayrı) yerinde.
- Doğrulama: `go build ./...` yeşil, `npx tsc --noEmit` yeşil.

## Sohbet gezinme + bekleme geri bildirimi ✅ (2026-07-28)

Transkriptte dört küçük ama sürtünme yaratan boşluk kapatıldı (detay `07`, `47`).

- **Yapışkan soru başlığı tıklanabilir:** üstte sabitlenen kullanıcı sorusuna
  tıklayınca o mesaja dönülür. Sarmalayıcı `pointer-events-none` kalır (gradyan
  üzerinden scroll geçmeye devam eder), yalnız balon `pointer-events-auto` olur;
  overlay scroll konteynerinin dışında olduğu için wheel elle iletilir. Satır üstten
  8px aşağıya oturur — aksi halde sticky başlık jump ettiğimiz mesajın üstüne binerdi.
- **Gönderimde dibe in:** `ChatView` composer submit'inde (send + queue)
  `scrollBottomSignal`'ı bump eder. Gönderim yalnız kuyruğa aldığı için mesaj ancak
- Doğrulama: `npx vitest run` 42 test yeşil. `npx tsc --noEmit` bu değişikliklerde
  temiz; repoda o sırada **başka bir çalışmanın** yarım kalan `IngestKind` düzenlemesi
  vardı (`SkillImportDialog.tsx` güncellenmemiş) — dokunulmadı.

## Navbar'da hayalet "Sohbet çalışıyor" noktası ✅ (2026-07-28)

Boştaki bir workspace'in nav rail'inde `Sohbet` nabız noktası (ve yan etkileri:
sidebar spinner'ı, composer'daki "Durdur" butonu) hiçbir tur çalışmadığı halde
yanıp kalıyordu; yalnız sayfa yenilemek geçiriyordu. Backend suçsuzdu —
`GET /api/activity` ilgili workspace için `chat:false` dönüyordu.

- **Kök neden:** `useChatStream.streamingSessions`, poll gecikmesini gizleyen
  istemci-tarafı bir mandal. Yalnız olayla temizleniyor, tamamlanma olayları ise
  `useAppEvents`'te `e.workspaceId === aktif workspace` koşuluyla filtreleniyor.
  Kullanıcı tur bitmeden başka workspace'e geçerse temizleme olayı düşüyor ve
  mandal sekme ömrü boyunca asılı kalıyor. Oturum ID'leri **store başına**
  sayaçla üretildiğinden (`db.nextID` → her workspace kendi `SES1`, `SES2`…
  serisini verir) bu yetim ID, ekrandaki workspace'in gerçek bir oturumuyla
  çakışıyor ve `App.tsx`'teki `chatBusyLocal` kesişim testini yanlış yere
  geçiriyordu.
- **Düzeltme:** kurtarma yolu artık **mutabakat** yapıyor, yalnız eklemiyor.
  `useChatStream.reconcileActive(ids)` pending'i `GET /api/sessions/active`
  listesiyle değiştirir, streaming'i o listeye daraltır (yeni saf yardımcı
  göre gruplanır. Panel akış sürerken de açılabilir (read-only), ajan değişince remount.
- Not: `/tools` slash komutu (LLM'e özet yazdıran, token harcayan yol) duruyor; bu buton
  aynı bilgiyi **sıfır token** ile verir.
- Ek tur: panel **boşluğa tıklayınca** kapanır (mousedown; 🔧 toggle'ı `data-tool-access-toggle`
  ile hariç tutulur, aksi halde kapat→anında-aç yanıp sönmesi olurdu) ve MCP sekmesindeki her
  sunucu **"bağlamda mı"** verdikti taşır (`in-context` / `hidden-only` / `disabled` /
  `agent-mcp-off` / `no-tools`) + aktif/katalog/gizli sayaçları. Özel (custom) MCP'lerin
  bağlama girip girmediği artık tek bakışta görünüyor.
- Terminoloji düzeltmesi: `hidden-only` rozeti **"bağlam dışı" değil "katalog dışı"**.
  Gizli tier promptta ~40 token'lık bir işaretçi bırakır ("N araç daha var, `tool_search`
  ile bul"), yani araçlar bilinmez değil — yalnız isim listesi çıkarılmıştır (49 self-
  management aracı için ~800–1000 token/tur tasarruf). Gizli sayaç rozeti tooltip'inde
  kabaca tasarruf gösteriliyor. Gerekçe: `_Docs\19` son bölüm.

Detay: `_Docs\19-LAZY-TOOL-LOADING.md` + `_Docs\07-CHAT-UX.md`.

## Akış `coordinator` node tipi — dinamik worker fan-out ✅ (2026-07-28)

`parallel`/`spawn` fan-out genişliği tasarım anında sabitti; "bulunan her bulgu için bir
worker" gibi sayısı **çalışma anında** belli olan işler ifade edilemiyordu. Yeni
`coordinator` node'u seçilen ajanı kendi koordinatör oturumunda çalıştırır, ajan kaç
worker açacağına anlık karar verir, düğüm hepsi bitene kadar bloklar ve son yanıtı
`{{last}}`'e koyar. Graf deterministik motorda kalır — düğüm motor açısından atomiktir,
yarıda kalan koşu resume'da düğümü baştan (yeni oturumla) çalıştırır.

- Motor: `orchestration.NodeCoordinator` + opsiyonel `CoordinatorRunner` arayüzü; panic-safe,
  accumulate modunda sonucu `parallel` gibi tek sentetik user/assistant çifti olarak katlar.
- Runtime: `internal/agent/flow_coordinator.go` — `Kind="flow-coordinator"` +
  `Role="coordinator"` oturum, `enqueueCoordinatorTurn` (slot'u senkron claim eder → erken
  "boşta" okuması imkansız), `waitCoordinatorIdle` yoklaması, timeout'ta `StopWorker`.
- `RecoverOrphanedTurns` bu oturumları **atlar** (ve bağlı yetim worker'lar için
  `NotifyCoordinator` yapmaz), yoksa `ResumeRunningFlows`'un yeniden çalıştırmasıyla yarışıp
  işi iki kez yapardı.
- **Koordinasyon türü (recipe) düğümden seçilebilir:** node'un `workflow` alanı Oturum
  Bilgisi panelindeki listenin aynısını sunar; slug oturumun `CoordinatorWorkflow`'una
  yazılır (reçete gövdesi normal yoldan prompt'a girer), reçetenin `max_turns`'ü düğüm
  kendi tavanını vermediyse uygulanır. Doğrulama `skills.ResolveCoordinatorWorkflow`'a
  (leaf paket; `api.ResolveCoordinatorRecipe` artık alias) taşındı ve hem precheck'te hem
  düğümde koşar — bilinmeyen reçete sessizce serbest koordinasyona düşmez.
- UI: `CoordinatorNode` (pusula, `#ea580c`), palet + (ⓘ), inspector formu
  (ajan/görev/workflow/maxTurns/timeoutSec), koşu görüntüleyicide hedef + rapor kartı,
  Oturumlar'da "Akış Koordinatörü" kind rozeti. Workflow seçici tek paylaşılan bileşen
  (`shared/components/CoordinatorWorkflowPicker`); `CoordinatorSection` de ona taşındı.
- Not: bu iş sırasında `internal/agent` test paketi zaten derlenmiyordu — `bootseq_test.go`
popup'tan (kalem ikonu) yapılır — ekranın yarısını yiyen satır-içi form ve
satır-içi editör kaldırıldı.

- `Schedules.tsx` (795 satır) + `Automations.tsx` (911 satır) ikilisi silindi; yerine
  12 odaklı dosya: `AutomationBoard.tsx` (ekran + veri + mutasyonlar),
  `BoardColumn.tsx` (şerit kabuğu), `ScheduleCard.tsx`/`AutomationCard.tsx` (kartlar),
  `FormModal.tsx` (ortak popup kabuğu), `ScheduleModal.tsx`/`AutomationModal.tsx`
  (oluştur+düzenle), `AutomationFields.tsx` (`BoardTriggerFields`+`PromptVarsField`),
  `pickers.tsx` (`TargetModeToggle`/`FlowPicker`/`Field`), `automationMeta.ts`,
  `cronPresets.ts`, `timeUtils.ts`.
- `App.tsx` artık `AutomationBoard`'u render ediyor (`Schedules` yerine).
- Etiket otomasyonu modalına **Ad** alanı eklendi (eskiden yalnız şablon set ediyordu).
- **Dikey/dar ekran:** `md` altında üç şerit `snap-x snap-mandatory` karuseli
  (`w-[85vw]`, şerit başına bir ekran, her şeridin kendi dikey scroll'u); `md`+ üçü
  yan yana `flex-1`. Popup'lar `ModalOverlay` sayesinde telefonda bottom-sheet.
- Kart aksiyonları çerçeveli ikon buton (`CardAction`): ▶ çalıştır + ✏️ düzenle
  (+ limit dolduysa ↺ sıfırla). **Sil karttan kaldırılıp düzenleme popup'ının sol
  kaldırılmış `gitWorktreeIsolation` parametresini geçiyordu; çağrı 2 argümanlı yeni imzaya
  güncellendi.

Detay: `_Docs
-FLOW-CANVAS.md` + `_Docs'-KOORDINATOR-COKLU-AJAN.md`.

## Market ekranı + item'ları bayatlık tazelemesi ✅ (2026-07-28)

Market, son alt sistemlerin gerisinde kalmıştı. Detay tablo: **`_Docs\21-MARKET.md` §8**.

- **`hook` türü UI'a bağlandı** — backend'de kurulabiliyordu ama `PackKind`/`IngestKind`'da
  yoktu: sekme yok, `KIND_LABEL['hook']` boş (İçe Aktar diyaloğunda başlıksız grup),
  önizleme yok. Artık **Hooks** kategorisi + hook önizleme kartı (olay/matcher/timeout +
  komut + güvenlik uyarısı) var; kurulum dedup yapmadığı için "zaten kurulu" işareti yok.
- **MCP paketi hibrit kapsama uyduruldu** — `MCPPayload`'a `description`/`headersConfig`/
  `scope`; `installMCPPack` sabit `Scope:"shared"` yerine payload'ı aktarır (allow-list'li).
  `mcp_adapter` `.mcp.json`'daki `headers` bloğunu okur → **auth başlıklı HTTP MCP sunucusu
## Flow paleti: node butonlarında (ⓘ) açıklama balonu ✅ (2026-07-27)

Flow editöründe sol paletteki node tipleri yalnız ad + ikon gösteriyordu; ne işe
yaradıkları görünmüyordu. Her palet satırına bir **(ⓘ) bilgi butonu** eklendi;
tıklayınca o node tipinin ne yaptığını anlatan balon açılıyor.

- Metinler: yeni `frontend/src/features/flows/nodeTypeHelp.ts` (`NODE_TYPE_HELP`,
  12 node tipinin tamamı; `internal/orchestration/model.go` yorumlarıyla hizalı).
- `shared/components/InfoPopover`'a opsiyonel **`fixed`** modu: balon viewport
  koordinatlarında çizilir (kenarlara clamp'li). Palet kolonu `overflow-y-auto`
  olduğu için mutlak konumlu balon kırpılıyordu. Varsayılan kapalı → mevcut
  kullanım yerleri etkilenmedi.
- `FlowEditorView` palet satırı `flex` sarmalayıcıya alındı (buton `flex-1`,
  etiket `truncate`); sürükle-bırak + tıkla-ekle davranışı korundu.
- Doğrulama: `npx tsc --noEmit` yeşil.

## Per-ajan araç override'ları — "Yasaklı Araçlar" 5. tier oldu ✅ (2026-07-27)

Araç görünürlük tier'ları (Tam/Özet/İsim/Gizli) yalnız **workspace** seviyesinde
ayarlanabiliyordu; ajan seviyesinde ise ayrı bir **yasaklı araç** denylist'i vardı —
iki ayrı model, iki ayrı UI, ajan başına ince ayar imkânsız. Artık tek bir 5 değerli
ajan override haritası var: `full | summary | name-only | hidden | **blocked**`.

- **Öncelik zinciri:** kod default < workspace `ToolVisibility` < ajan `ToolOverrides`.
  `blocked` registry'ye **girmez** — bir katman yukarıda `toolFilter`'da çözülür ve
  aracı katalogdan tamamen düşürür; görünürlük semantiği kirlenmez.
- **Persistans:** yeni `Agent.ToolOverrides` (JSON object; anahtar tam ad **veya**
  `prefix*` deseni). `BlockedTools` silinmedi, **türetilmiş ayna**'ya dönüştü —
  `UpdateAgentTools` her yazışta `blocked` girdilerinden sıralı üretip yazar, böylece
  market paketleri/şablonlar/eski ajan dosyaları bozulmaz.
- **Migrasyon okuma tarafında:** `agent.ParseToolOverrides` iki kaynağı birleştirir
  (legacy denylist → `blocked`), açık override daima kazanır. Bozuk JSON fatal değil →
  override'sız duruma düşer, ajan çalışmaz hale gelmez.
- **Desen genişletmesi:** `SetVisibility` tek isim aldığı için `prefix*` anahtarları
  katalog üzerinde genişletilir; anahtarlar sıralı işlenir → daha özel (uzun) desen kazanır.
- **UI = DIFF:** ajan Araçlar ekranı ikinci bir katalog değil; yalnız override'lı araçlar
  `varsayılan → seçili` rozet çiftiyle listelenir (+5'li tier seçici, "Varsayılana dön",
  toplu uygulama). Varsayılana eşit seçim override'ı **siler**. Katalogda karşılığı
  olmayan anahtarlar (desen / o an kapalı araç) kesikli çerçevede korunur — sessizce
  düşürmek bir yasağı kaldırırdı.
- Kod: `agent/tooloverrides.go` (yeni), `agent/toolsetup.go` (`buildRegistry` iki-katmanlı
  zincir, `blockFunc`, `ActiveToolCatalogWithState`), `db/models.go`+`db/store_mcp.go`,
  `api/agent_tools.go`, frontend `AgentToolsSection.tsx`+`AgentToolOverrideRow.tsx`+
  `VisibilityControls.tsx`+`toolMeta.ts`. Testler: `tooloverrides_test.go` (6) +
  `tooloverrides_integration_test.go` (4). Detay: **`_Docs/19`**.

## SSE kopması toleransı: yeniden-bağlanınca resync ✅ (2026-07-27)

Poll'ler SSE'ye taşındıkça (bir alttaki giriş) yeni bir kırılganlık doğdu: **feed
kopup geri geldiğinde arada yayınlanan frame'ler kalıcı olarak kayıp** — backend bus'ı
fire-and-forget, replay yok, `Last-Event-ID` cursor'ı yok. Yalnız event'lerle render
eden bir görünüm bunu **kendi başına fark edemez**; en kötü hâli: outage sırasında biten
worker yüzünden banner çoktan rapor vermiş bir worker'da asılı kalır.

`api/system.ts` zaten CLOSED olan kaynağı 2sn backoff'la yeniden kuruyordu, ama kimseye
"kaçırdın, tazele" demiyordu. Eklenenler:
- **`sharedES.onopen`** → ilk açılış değilse `reconnectSubs`'a sinyal (`everConnected`
  bayrağı ilk açılışı ayırt eder; feed idle'dan kapanınca sıfırlanır → sonraki açılış
  yine "ilk" sayılır, çünkü yeni aboneler kendi başlangıç fetch'ini yapar).
- **`subscribeReconnect(cb)`** — `subscribeEvents`/`subscribeLogs` ile aynı desen
  (multiplex + `closeIfIdle` muhasebesine dahil).
- **`useAppEvents.onReconnect`** — SSE-only kümeyi tazeler: `refreshSessions()` +
  `publishWorkerChangeAll()` (yeni; hangi koordinatörün etkilendiği bilinemediği için
  tüm roster aboneleri) + açık oturumun transkriptini yeniden yükler.

Tarayıcının kendi auto-reconnect'i (readyState CONNECTING) ve modül-seviyesi rebuild
(CLOSED → backoff) **ikisi de** aynı `onopen` yolundan geçtiği için tek kanca yeterli.
`tsc --noEmit` temiz, `npm run build` + vitest (38) yeşil.

## Worker banner'ı: canlı süre + poll→SSE ✅ (2026-07-27)

Banner'ın (bir alttaki giriş) iki eksiği kapatıldı.

**1) Canlı süre.** `WorkerInfo`'ya `StartedAt` (unix sn) eklendi; kaynak
`workerCtl.startedAt` (runWorker turu başlatırken damgalar), API'de `startedAt`
alanı. Banner her çipte 1sn tick ile `Xsn` / `Xdk Ysn` yazar. Start zamanı
bilinmediğinde (ctl yok: worker oturumunda doğrudan açılmış tur veya restart'ı
atlatmış tur) alan 0 kalır ve UI süreyi **gizler** — uydurma süre göstermez.

**2) Poll kaldırıldı, SSE geldi.** `runWorker` başında yeni `emitWorkerStartEvent`
bir `worker` event'i yayınlar (`Target.phase="start"`; spawn + `send_to_worker`
ikisini de kapsar). `useAppEvents` her `worker` event'ini `coordinatorId` anahtarıyla
yeni `shared/lib/workerBus.ts`'e fanlar; `useRunningWorkers` **ve**
`CoordinatorSection` abone olup roster'ı tazeler — ikisindeki 3sn poll silindi.
Boşta trafik sıfır; worker başlayınca/bitince banner anında güncellenir.

**Kritik ayrıntı — `phase="start"` gate'i.** `useAppEvents`'in otonom-tamamlanma dalı
`worker` event'ini "tur bitti" sayıyor: ghost balonu siler, transkripti yeniden yükler,
`publishTurnEnd` fanlar ve masaüstü toast'ı atar. Bunların hepsi yeni **başlayan** bir
tur için yanlış olurdu (üstelik 8'li fan-out 8 toast demekti), o yüzden start event'i
hem o daldan hem toast funnel'ından hariç tutuldu; `refreshSessions`/panel sinyalleri
akmaya devam eder (yeni worker oturumu sidebar'a düşsün).

Yeni: `frontend/src/shared/lib/workerBus.ts`. `go build` + `go test ./...` (1029)
yeşil, `tsc --noEmit` temiz, `npm run build` başarılı.

## Sohbette çalışan worker banner'ı ✅ (2026-07-27)

**Belirti:** Bir koordinatör oturumu `spawn_worker` ile worker başlatıp turunu bitirdiğinde
sohbet **bitmiş gibi** görünüyordu — ilk `<task-notification>` düşene kadar hiçbir işaret yok.
Canlı worker roster'ı yalnız sağ **SessionDetailPanel ▸ Koordinasyon** bölümünde vardı, yani
panel kapalıysa konuşmanın worker sonucu beklediği belli olmuyordu.

**Düzeltme:** composer'ın üstündeki alt-yığına `WorkerWaitBanner` eklendi (`WakeWaitBanner` ile
aynı desen): "N worker çalışıyor — sonuçları bekleniyor · M/T bitti" + her çalışan worker için
tıklanabilir çip (worker oturumunu açar; worker'lar birinci-sınıf oturum). Veri `useRunningWorkers`
hook'undan — `GET /api/sessions/{id}/workers`; **yalnız** `role==='coordinator'` oturumlarda etkin,
poll da yalnız (tur streaming || en az bir worker çalışıyor) iken 3sn'de bir; boşta oturum başına
tek fetch. Oturum değişiminde roster anında boşaltılır (başka koordinatörün listesi yanıltmasın).

Yeni: `frontend/src/features/chat/WorkerWaitBanner.tsx`, `useRunningWorkers.ts`;
`ChatView` yeni `sessionRole`/`onSelectSession` prop'ları, `App.tsx` ikisini de besler.
Yeni backend/API yok. `tsc --noEmit` temiz.

## Navbar kayboldu: Tailwind utilities cascade layer'dan çıkarıldı ✅ (2026-07-27)

**Belirti:** Sol `NavRail` masaüstünde hiç görünmüyordu; alt `MobileNavBar` de gizliydi, yani
1920px'te iki gezinme çubuğu birden yoktu. Kod tarafında hiçbir değişiklik yoktu (`tsc` temiz,
`App.tsx` her ikisini de koşulsuz render ediyor) — bozulan CSS'ti.

**Kök neden — Chrome 150 regresyonu.** Tarayıcıda ölçüldü: `hidden md:flex` sınıf çiftinde
`display` `none` kalıyordu, oysa `.md\:flex` kuralı üretilen CSS'te `.hidden`'dan ~700 kural
SONRA geliyor (CSSOM'da doğrulandı: `@layer utilities` içinde sırasıyla #111 ve #807), aynı
specificity, `!important` yok. Tailwind v4 responsive utility'leri **nested** yazar
(`.md\:flex { @media (width >= 48rem) { display: flex } }`); Chrome 150 bu nested `@media`
declaration'larının kaynak sırasını **büyük bir `@layer` bloğu içinde** kaybediyor. Kesin kanıt:
aynı CSS'te tek kelimeyi değiştirip (`@layer utilities {` → `@media all {`) ölçüm `flex`'e döndü.
Küçük bir layer'da tekrarlanmıyor, yani kural sayısına bağlı bir Blink hatası. Tailwind 4.2 de
aynı nested çıktıyı ürettiği için sürüm düşürmek çözmüyordu.

**Etki alanı navbar'dan genişti:** `hidden sm:inline` / `hidden md:block` gibi TÜM
"mobilde gizle, geniş ekranda göster" kalıpları (AppHeader etiketleri, ~25 dosya) ölüydü.

**Düzeltme (`frontend/src/index.css`):** tek satırlık `@import 'tailwindcss'` üç parçaya bölündü —
`theme.css layer(theme)` + `preflight.css layer(base)` + `utilities.css` **layer'sız**. Layer'sız
utility'ler kaynak sırasını koruyor, kalıp yeniden çalışıyor. Cascade açısından güvenli: utilities
zaten en yüksek layer'daydı, layer dışına çıkınca yalnızca daha güçlü oluyor. Dev + prod build
doğrulandı (rail 208px `flex`, mobil bar `md`'de gizli). Chrome hatası düzelince tek-satır
import'a dönülebilir — gerekçe index.css'teki yorumda duruyor.

## Test altyapısı: CI tam kapsam + frontend testleri ✅ (2026-07-27)

Test **kodlarının** durumu iyiydi (kaldırılan özelliklerin testleri de silinmiş — `tools/compact`,
memory, günlük limitler için sıfır artık referans), asıl boşluk **koşturma** tarafındaydı. Dört düzeltme:

1. **CI hiç koşmuyormuş — crabbox kaldırıldı, gate Gitea'ya taşındı.** İki katmanlı sorun: (a) mevcut
   `ci` job'ı 30 paketten yalnız 4'ünü (`conversation/billing/orchestration/skills`) koşturuyordu —
   `agent`/`api`/`tools`/`db`/`insight`/`providers`, yani testlerin ~%85'i, kapsam dışıydı; (b) daha
   kötüsü, o job `.github/workflows/` altındaydı ama **repo'nun tek remote'u Gitea** — GitHub'a hiç
   push edilmiyor, dolayısıyla workflow hiç çalışmıyordu. Crabbox tamamen kaldırıldı (`.crabbox.yaml`,
   `.github/`, harici-araç kataloğu satırı); GitHub runner'ının içinde ikinci bir Docker katmanı zaten
   gereksiz dolaylılıktı ve `slug=` stdout-parse'ı kırılgandı. Yerine **`.gitea/workflows/ci.yml`**:
   native `setup-go`/`setup-node` (deploy.yml'in zaten kanıtladığı desen), `go vet` + `go build` +
   `go test ./... -race` (`TIONSWARM_ENABLE_SHELL=1`) + frontend `npm test` + `npm run build`.
   `-race` yalnız burada gerçekten koşar — Windows geliştirme makinesinde CGO kapalı. Ayrıca
   `deploy.yml` artık `needs: test` ile geçide bağlı: kırmızı build deploy edilemiyor.
2. **BOM fix.** `internal/tools/builtin_workspacemgmt.go` UTF-8 BOM ile başlıyordu; `go build`/`go vet`
   tolere ediyor ama cover instrumentation dosyayı yeniden yazınca BOM ortada kalıp
   `invalid BOM in the middle of the file` ile **tüm `internal/tools` paketinin coverage ölçümünü**
   kırıyordu. BOM kaldırıldı → paket %57.8 ile ölçülebiliyor (repodaki tek BOM'lu `.go` dosyasıydı).
3. **Bildirim sözleşmesine drift testi.** `events.NotifyKinds` ↔ `frontend/.../notifyTypes.ts` senkronu
   yalnız iki yorum satırıyla korunuyordu. `internal/events/notifyparity_test.go` TS dosyasını parse edip
   iki listeyi karşılaştırır: backend-only kind = Ayarlar'da susturulamaz bildirim, frontend-only kind =
   ölü toggle. `prompt` bilerek frontend-only (backend olayı yok). Yanında kontrol/stream tiplerinin
   `NotifyKinds`'e sızmadığı testi (sızarsa her log satırı toast olurdu).
4. **Frontend testleri sıfırdan.** Vitest kuruldu (`vitest.config.ts`, node env, `@` alias; `npm test` /
   `npm run test:watch`); kullanılmayan `@playwright/test` devDependency'si kaldırıldı (config yok,
   script yok, tek test yok). İlk 38 test: `notifyTypes` (cue/badge eşlemesi + `task`→`board` legacy
   alias'ı) ve `recommendations` (öneri kural motoru: token-conflict önceliği, sqz/rtk hook tespiti,
   `shellOutputCompression` explicit-tercih saygısı, cbm add/enable ayrımı, kart sırası). `ci.yml`'de
   ayrı bir `frontend` job'ı olarak `npm test` + `npm run build` koşar (Go gate'ine paralel).

Ek olarak `internal/tools/readtracker_test.go`: dosya-tazelik guard'ının unit sözleşmesi — nil tracker
no-op, mtime-değişti-içerik-aynı (guard tripmemeli) vs içerik-değişti-mtime-aynı (tripmeli), never-read
ile stale hata mesajlarının ayrışması, `recordWritten` sonrası ardışık yazım, path-scope, eşzamanlılık.
Araç seviyesindeki happy-path'ler zaten `builtin_fs_test.go`/`builtin_patch_test.go`'daydı.

Durum: **937 test yeşil, 0 fail, 11 skip** (harici araç/ağ geçitli) + 38 frontend testi.

Bilinen kalan boşluklar (öncelik sırasıyla): `internal/api` %17 — `chat_stream.go` (37KB),
`chat_control.go` (27KB), `session_context.go` (26KB), `session_stream.go` testsiz;
`internal/workspace` %4.3 (`manager.go` 23KB); `internal/app` %15.5.

## Flow: async spawn/join + subflow await-propagasyonu ✅ (2026-07-27)

İki yeni yürütme yeteneği (temiz kurulum; `flow.go` tek elden). **Async spawn/join:** `spawn`
node child flow'ları bloklamadan başlatır (`State.Spawned`), `join` node bariyer olarak block-poll
ile bekleyip çıktıları birleştirir; interaktif çocuk join'i fail eder → hep sonlanır. **Subflow
await-propagasyonu:** subflow çocuğu `await-input`'a düşerse parent da askıya alınır (`State.SubflowRun`),
parent'a input verilince child sync resume edilir. `AsyncFlowRunner` + `SuspendableChildFlowRunner`
arayüzleri; frontend palet "Spawn"/"Join" + inspector. **Genişletme:** join'e `joinTimeoutSec` +
`joinPartial` (kısmi mod: fail/suspend/timeout çocuğu düşür), ve subflow/spawn/join için **flow-picker
UI** (id metni yerine seçici). Ayrıca continuation turn'lerinin (scheduler `deliverPrompt`/`deliverWake`)
ortak reply/error kaydı `recordAssistantReply`/`recordTurnError`'a çıkarıldı (god-function'dan bilinçli
kaçınıldı). **Devam:** paylaşım 5 continuation sitesine yayıldı (`agentmsg`/`spawn`/`coordination` +
`recordAssistantMessage` çekirdeği); gallery'ye **Async Fan-out (Spawn/Join)** örnek şablonu (companion
alt-akışlarla runnable); **join canlı ilerleme** (`onProgress` → `progress` NodeEvent → RunView "N/M").
Backend 1029 test yeşil; canlı E2E hepsi (spawn/join, propagasyon, partial-drop, SSE progress `0/2→2/2`).
Detay: [62-BIRLESIK-RUN-AWAIT.md](62-BIRLESIK-RUN-AWAIT.md).

## Claude Opus 5 model desteği ✅ (2026-07-27)

Anthropic **Opus 5** (`claude-opus-5`, 24 Tem 2026; 1M bağlam, Opus fiyatı sabit
$5/$25) katalog + fiyat tablolarına eklendi. Açık giriş gereken 3 yer:
`kind_anthropic.go` katalog (yeni "en yetenekli", Opus 4.8 → "önceki nesil"),
`pricing.go` (anthropic + openrouter tabloları), `kind_openrouter.go`
(`anthropic/claude-opus-5` önerisi). `context_window.go`/`maxoutput.go` model-ailesi
("opus") eşleştiği için 1M pencere + çıktı tavanını otomatik verir; claude-cli `opus`
alias'ı CLI güncellenince otomatik çözülür (değişiklik yok). Frontend model dropdown'ı
API-güdümlü → değişiklik gerekmedi. Varsayılan model değişmedi (anthropic hâlâ Sonnet 5).
Backend `go build` + `internal/providers` 107 test yeşil.

## Flow Start / End node'ları ✅ (2026-07-27)

İlk-sınıf **Start** (zorunlu giriş markeri; per-node "başlangıç işaretle" kalktı) + **End**
(opsiyonel terminal; `Template` çıktıyı şekillendirir, `OutputSchema` nihai çıktıyı JSON-Schema'ya
karşı doğrular → uymuyorsa `failure` = **çıktı sözleşmesi**). Temiz kurulum: geri uyumluluk yok,
eski flow'lar `MigrateAddStart` + `MigrateFlowsStartEnd` ile workspace açılışında migrate edildi;
default flow + gallery/swarmpack templates + frontend `ensureStartNode` yeni formatta. Detay:
[15-FLOW-CANVAS.md](15-FLOW-CANVAS.md). Backend 1004 test yeşil; canlı: start→agent→end +
eski FLW15 migrate doğrulandı.

## Birleşik Run (C+D): await-input keystone + genişletmeler ✅ (2026-07-26)

Session ⇄ flow birleşiminin yürütülebilir çekirdeği ve çevresi. Detay: [62-BIRLESIK-RUN-AWAIT.md](62-BIRLESIK-RUN-AWAIT.md).

- **`await-input` keystone (durable suspend/resume):** flow bir node'da **durup girdi bekleyebilir**.
  `orchestration.State.WaitingAt` + `db.FlowWaiting` statüsü; suspend `Run`'dan `(st,nil)` ile döner
  (Current park), `MarkFlowRunWaiting` persist; `ClaimWaitingFlowRun` **CAS** çift-resume korur;
  waiting'ler boot-resume dışı (orphan yok). `ResumeWaitingFlow` + `POST /api/flow-runs/{id}/input`
  (409 guard). Girdi `{{last}}` ile devam eder. UI: RunView waiting composer + sarı ring. Canlı
  gerçek-LLM E2E ✅.
- **`LaunchRun` (Faz 3):** tetik-launcher'ların `FlowID?flow:session` dalı tek seam'de
  (`internal/agent/launch.go`, `RunTrigger`/`RunSpec`); `automation.fire`/`fireBoard` + `scheduler.deliverFlow`
  buradan geçer, `fireFlow` silindi. Reuse-continuation launcher'ları (`deliverPrompt`/wake) **tasarımca
  dışında** (continuation ≠ fresh launch).
- **Peer-bridge (#1):** `list_flow_runs` (status='waiting' → bekleyeni bul) + `deliver_flow_input`
  (resume köprüsü) araçları → peer ajan/koordinatör bekleyen flow'u besler.
- **await timeout/GC (#3):** node `TimeoutSec` + 30s sweeper (`StartWaitingFlowSweeper`) deadline
  geçeni CAS-claim + `failure`.
- **`subflow` node (#2):** bir flow başka flow'u baştan sona koşup çıktısını yakalar (kompozisyon);
  `RunChildFlow` + recursion depth guard (5). Canlı E2E ✅.
- Backend 1000 test yeşil; frontend `tsc`+`vite build` yeşil; backend restart + canlı doğrulama.

## Flow motoru: accumulate cache + loop + session↔flow köprüsü ✅ (2026-07-25/26)

Detay: [15-FLOW-CANVAS.md](15-FLOW-CANVAS.md).

- **Accumulate (cache'li bağlam):** `Graph.Accumulate` (flows ekranında default açık) — ardışık agent
  node'ları büyüyen tek konuşma thread'ini paylaşır (`State.Thread` + `ThreadAgentRunner`) → prompt-cache
  düğümler arası; `Node.Fresh` opt-out; paralel copy-on-fork + join sentetik-turn katlama.
- **`loop` node:** `Body`/`LoopNext`/`MaxIters`/`Until` — gövde alt-zincirini yineler (`{{iteration}}`),
  global `maxSteps` frenler.
- **Session → Flow:** chat header "Akış" toggle → oturumu **tamamlanmış bir koşu** olarak inline RunView'de
  gösterir (flow-run oturumunun gömülü step'leri çok-node'a açılır); node inline çıktı önizlemesi; dikey
  auto-layout.
- **Per-run flow oturumu:** her koşu kendi session'ı (`CreateSession`, `SourceID=flow.ID`).
- **Editörden çalıştır → Koşular tab'ına yönlendir** (editör canvas'ı değişmez).

## Kuyruk mesajı iptal edilince gözlemci kapanışı ✅ (2026-07-25)

`/chat` + `/chat/stream` kendi turunun terminal olayını bekler; mesaj çalışmadan başka
pencereden (`DELETE .../queue/{id}` veya `.../queue`) silinirse terminal hiç gelmez →
gözlemci asılırdı. `queue_update` payload'ına `inflightClientMsgId` eklendi (dispatched↔
cancelled ayrımı); gözlemci mesajını kuyrukta **canlı gördükten sonra** kaybolursa iptal
sayar → stream `error{reason:"cancelled"}`, non-stream **409** döner. Enqueue-öncesi yarışa
karşı "önce canlı görülmeli" guard'ı. Test `TestQueueHasMsg`; app+api+agent **348 test** yeşil.
Detay: `_Docs/58`.

## Tek-instance DataDir kilidi (multi-process guard) ✅ (2026-07-25)

Doc 58'in tüm serileştirmesi (hub/inbox-worker/coordSlot/interaction-CAS/scheduler)
**tek process belleğinde**; aynı store'a iki server process = cross-process eşzamanlı
tur + çift schedule + boot çift re-dispatch + entity ezmesi. Masaüstü `:0` portu
bağladığından double-launch'ta port çakışması yok, store kilidi de yoktu → iki birincil
aynı store'u bozardı. **Sert ret eklendi:** `app.Bootstrap` store'a girmeden DataDir'de
process-ömürlü exclusive advisory kilit alır (`internal/app/instancelock*.go`; Windows
`CreateFile` share=0, Unix `flock` — yeni bağımlılık yok, OS process çıkışında bırakır →
stale kilit yok). Tutuluysa net hatayla reddeder; connect-only ikincil pencereler
etkilenmez. Test `instancelock_test.go`; app+api+agent **341 test** yeşil. Detay: `_Docs/30`.

## Kuyruk cutover — nadir-senaryo sağlamlaştırması ✅ (2026-07-25)

Cutover sonrası adversarial gözden geçirmede 3 nadir boşluk kapatıldı: (1) **düşen
terminal frame → asılma** — hub fan-out'u terminal `turn_done`'u düşürürse gözlemci
sonsuza bekliyordu → her ping tick'te `drainReplay` ile ring reconcile; (2)
**`handleChat` yanlış/boş yanıt** — ardışık turlarda "son assistant" yarışı → gördüğü
hub `KindReply` payload'ını kullanır (DB fallback sondan geriye tarar); (3) **asılan
wake/scheduled tur → kuyruk head-of-line bloğu** — `deliverWake`/`deliverPrompt` slotu
alıyor ama `withActivityTimeout`'u yoktu → spawn/worker paritesiyle sarıldı. Yeni test
`TestDrainReplayRecoversDroppedTerminal`; agent+api **340 test** yeşil. Detay: `_Docs/58`.

## Legacy `/chat/stream` + `/chat` durable kuyruğa taşındı ✅ (2026-07-25)

Bu iki endpoint (frontend hiçbirini çağırmıyor — yalnız dış otomasyon/eski istemci)
turu **inline** koşup `inbox.json`'a yazmıyordu → mid-turn crash'te mesaj kayboluyor
+ kuyruğu atlıyordu. Artık ikisi de serial send-queue'ya enqueue eder (kalıcı,
crash-recoverable, tek-tur garantili) ve per-session hub'ı gözler: `handleChatStream`
hub'ı legacy SSE frame şekline çevirip relay eder (streaming sözleşmesi korunur),
`handleChat` terminal olayı bekleyip kalıcı yanıtı DB'den döndürür. Korelasyon: taze
`clientMsgId` → `runChatTurn` + kuyruk bariyerleri terminal hub olaylarına (turn_done/
turn_error) damgalar (öndeki turun terminal'i erken kapatmaz); slow-drop'a karşı
`Replay` gap-fill. `failTurn` artık hub'a da turn_error yayınlıyor (önceden yalnız SSE
sink → hub istemcileri reload'da görüyordu). Yeni dosya `internal/api/chat_queue.go`
(iki handler + relay helper'ları); eski inline gövdeler `chat.go`/`chat_stream.go`'dan
silindi (`inflightRecorder` testte kullanıldığı için korundu). `go build`/`go vet`
temiz, agent+api **339 test** yeşil. Detay: `_Docs/58`.

## Per-session tur kilidi TÜM oturumlara genelleştirildi ✅ (2026-07-25)

**Sorun:** "tek oturumda tek tur" garantisi yalnız koordinatör oturumlarındaydı;
düz oturumda `claimTurnSlotIfCoordinator` no-op dönüyordu. Kullanıcı chat yazarken
(inbox worker) aynı oturuma zamanlanmış **wake** / **peer teslimi** düşerse ya da
legacy `/chat/stream`·`/chat` inbox worker koşarken çağrılırsa **eşzamanlı iki tur**
açılabiliyordu (doc 58'in kapatmayı hedeflediği yarış, düz oturumda açıktı).

**Ne yapıldı:** koordinatör-only geçit kaldırıldı, `coordSlot` her oturumun tek tur
kilidi oldu. `BeginCoordinatorUserTurn`→`BeginSessionUserTurn` (chat_stream.go +
chat.go koşulsuz claim); `claimTurnSlotIfCoordinator`→`claimSessionTurnSlot` (her
zaman claim, resetCap=false) → wake/scheduled/peer (scheduler.go + agentmsg.go) aynı
slotta serileşir. Düşük seviye `claimCoordinatorSlot` korundu. Testler:
`TestClaimSessionTurnSlot` (yeniden yazıldı) + yeni
`TestPlainSessionSerializesConcurrentTurns`. Ayrıca `runWorker` +
`runSpawn` kendi oturum slotlarını almıyordu ve `SendToWorker` eşzamanlı turu yalnız
`isSessionActive` (UI göstergesi, kilit değil → TOCTOU) ile kontrol ediyordu; ikisi
de `claimSessionTurnSlot`'a bağlandı → worker/spawn turları da her turla serileşir.
`go build`/`go vet` temiz, agent+api **333 test** yeşil. Detay: `_Docs/47` §13, `_Docs/58`.

## Workspace oluşturmada "veri klasörü" → "proje dizini" ✅ (2026-07-25)

**İstek:** Workspace oluştururken "Veri klasörü" seçimi kalksın (hep app default kullanılsın);
yerine opsiyonel "Proje dizini (path)" girilebilsin.

**Ne yapıldı:**
- **Backend:** `createWorkspaceReq.Path` → `ProjectDir`. Data dir daima app varsayılanı
  (`workspaces.Create(name, "", "")`). `ProjectDir` doluysa identity patch'iyle workspace'in
  `DefaultWorkingDir`'ine yazılır.
- **Frontend:** `WorkspaceCreateModal` — "Veri klasörü" alanı "Proje dizini (path)" oldu
  (state `path`→`projectDir`, Gözat/pickFolder korundu). `NewWorkspaceData.path`→`projectDir`,
  `api.createWorkspace` body alanı da `projectDir`.
- **Test:** `go build` + `go test ./internal/api ./internal/workspace` (120) yeşil, `tsc --noEmit` temiz.

## Soyut "varsayılan sağlayıcı/model/ajan" kaldırıldı ✅ (2026-07-25)

**İstek:** Workspace ayarlarında "varsayılan model/provider/ajan" diye bir özellik olmasın;
sağlayıcı/ajan net belirtilsin ya da mevcut ajanlardan ilki seçilsin. Aynısı İçgörü
ekranındaki ajan seçiminde de geçerli olsun.

**Ne yapıldı:**
- **Workspace ayarları:** `WSSettings.DefaultProvider/DefaultModel` (struct + patch + DTO +
  frontend `WorkspaceSettings`/patch) tamamen kaldırıldı; `WorkspacePanel`'deki
  "Varsayılan sağlayıcı + model" bloğu + banner metni silindi (`ProviderModelSelect` importu da).
- **App-geneli varsayılan da kaldırıldı:** `settings.Settings.DefaultProvider/DefaultModel`
  (struct + DTO + patch + `Default()` + normalize + `Validate`) ve `ProvidersPanel`'deki
  "Varsayılan sağlayıcı + model" kontrolü + `SettingsPanel` save payload'ı + `AppSettings`
  tipi silindi.
- **Registry temizliği:** `providers.Registry.defaultModel` alanı + `SetDefaultModel` metodu +
  `ResolvedConfig.Model` alanı tamamen söküldü (`server.go` çağrısı da). claude-cli artık modelini
  ajanın `req.Model`'inden alır (`kind_claudecli` `NewClaudeCLI(..., "", ...)`), boşsa CLI kendi
  oturum varsayılanını kullanır.
- **Yeni ajan çözümü:** `handleCreateAgent` boş sağlayıcı/modeli workspace'in **ilk (en yeni)
  ajanından** miras alır (`firstAgentProviderModel`) → `claude-cli` + provider yerleşik modeli.
  Şablon tohumlama (`defaultProviderModel`) → `claude-cli`, model "". `handleTestProvider`
  boş modeli provider'ın kendi varsayılanına bırakır.
- **İçgörü:** `SettingsTab` AgentPicker artık `clearable` değil; açılışta ajan seçili değilse
  **ilk ajan** otomatik seçilir. "Varsayılan (…)" placeholder'ı kalktı.
- **Migration:** `ws-settings.json` yüklemede eski `defaultProvider/defaultModel` anahtarları
  görülürse dosya bir kez temiz yeniden yazılır (`loadSettings` → `saveSettings`). App
  `settings.json`'daki dead anahtarlar unmarshal'da yok sayılır, sonraki kayıtta düşer.
- **config_validate:** `settings.json` için beklenen anahtar `defaultProvider` → `defaultPermissionMode`.
- **Test:** `go vet ./...` temiz, `go test ./...` (974) yeşil, `tsc --noEmit` temiz.

## Her workspace'e varsayılan flow tohumlama ✅ (2026-07-25)

**İstek:** Her workspace'te kullanılabilecek bir flow; yeni workspace'te otomatik eklensin,
istenirse silinebilsin, eski workspace'lere de eklensin, şablonlar arasına da eklensin.

**Ne yapıldı:**
- **Model:** `db.Flow` += `Seed string` (shipped default işaretçisi).
- **Seed mantığı:** `internal/agent/flow_defaults.go` — `defaultFlows` (tek doğruluk kaynağı;
  "Yanıtla & Doğrula" akışı) + `EnsureDefaultFlows(ctx, db, storeDir)`. İdempotent
  (DB'de aynı `Seed` varsa atlar) + **silme kalıcı** (store kökünde `.seeded-flows.json`
  ledger; silinen tohum geri gelmez). Agent node'lara ilk ajan atanır (yoksa boş).
- **Tetikleme:** `workspace.Manager.open()` her açılışta çağırır → yeni workspace kurulumda,
  eski workspace'ler bir sonraki başlangıçta backfill.
- **Şablon galerisi:** `flowTemplates.ts` += `default-starter` (aynı graph).
- **Test:** `flow_defaults_test.go` (4 vaka: seed/idempotens, silme kalıcılığı, ilk-ajan
  ataması, ajansız seed). `go build`/`vet` + `tsc -b` yeşil.

## Worker oturumunda "Koordinatöre dön" butonu ✅ (2026-07-24)

**İstek:** Worker oturumundan koordinatör oturumuna dönme kısayolu.

**Ne yapıldı (yalnız frontend):** `SessionInfo.coordinatorSessionId` back-link'i
`SessionDetailPanel`'den `CoordinatorSection`'a geçirildi; worker branch'indeki
pasif notun altına ArrowLeft ikonlu "Koordinatöre dön" butonu eklendi →
`onSelectSession(coordinatorSessionId)` ile koordinatör oturumunu açar. Yalnız
`onSelectSession` + back-link mevcutsa görünür. `tsc` yeşil. Detay `_Docs\47`.

## Koordinasyon roster'ı: worker satırına tıklayınca oturumu açılır ✅ (2026-07-24)

**İstek:** Oturum bilgisi ekranındaki worker'a tıklayınca o worker'ın oturumuna
gitsin.

**Ne yapıldı (yalnız frontend):** Her worker zaten kendi oturumu (`w.sessionId`).
`SessionDetailPanel` mevcut `onSelectSession`'ı `CoordinatorSection`'a zincirledi;
roster satırı `onSelectSession` verildiğinde tıklanabilir butona dönüşüp
`onSelectSession(w.sessionId)` ile o oturumu açar (hover accent kenarlık). Prop
yoksa satır eski düz div olarak kalır. `tsc` yeşil. Detay `_Docs\47`.

## Composer: dar ekranda tur-ayarı butonları toggle ile gizlenir ✅ (2026-07-24)

**İstek:** Sohbet input alanı dikey/dar ekrana geçince Düşünme seviyesi, İzin modu
ve Çalışma dizini butonları bir toggle ile gösterilip gizlenebilsin.

**Ne yapıldı (yalnız frontend, `Composer.tsx`):**
- Üç per-turn kontrolü (`ComposerPicker` düşünme + `ComposerPicker` izin +
  `WorkDirBadge`) `display:contents` sarmalayıcıya alındı → toolbar gap'i bozulmaz.
  Sarmalayıcı dar ekranda `hidden`, `md:contents` ile **`md:`'den itibaren daima
  görünür**.
- Yeni `SlidersHorizontal` toggle butonu (`md:hidden`, yalnız dar ekran) kontrolleri
  aç/kapat yapar; açıkken accent kenarlık. Tercih `localStorage`
  (`tionswarm.composerControlsOpen`) ile kalıcı; varsayılan gizli. `tsc` yeşil.

## Görev listesi UX: bilgi panelinden kaldırıldı + TodoPanel minimize/kapatılamaz ✅ (2026-07-24)

**İstek:** Sohbet bilgisi panelinde görev listesi görünmesin; sohbet sırasında
açılan görev listesi küçültülmüş başlasın ama kapatılamasın.

**Ne yapıldı (yalnız frontend):**
- **Bilgi paneli:** `SessionDetailPanel.tsx`'ten `ProgressCard` (Görev Listesi)
  render'ı + `progress` state + `sessionProgress` fetch effect'i (`executions`
  sinyaliyle tazeleme) + ilgili importlar kaldırıldı. `SessionProgressCard.tsx`
  artık kullanılmayan ölü bileşen; API/tip korunur.
- **`TodoPanel.tsx`:** varsayılan **küçültülmüş** (`open=false`) başlar; ✕ gizle
  (dismiss) butonu + `dismissedSig`/`sig` mantığı + auto-open `useEffect` kaldırıldı
  → panel composer üstünde iğneli kalır, yalnız başlıktan katla/aç, **asla
  kapatılamaz**. `tsc --noEmit` yeşil. Detay `_Docs\36`.

## 👍/👎 geri bildirimi tur bağlamına enjekte ✅ (2026-07-24)

Mesaj puanları saklanıyordu (`db.MessageFeedback`, `session.jsonl`) ama hiç geri
okunmuyordu — puan vermek sonraki cevabı değiştirmiyordu. Yeni
`internal/api/chat_feedback_summary.go` → `recentFeedbackBlock`: en yeni **6**
puanlı turdan kompakt bir `<user_feedback>` bloğu üretir ve **volatile dinamik
suffix'e** enjekte eder — history'ye katlanmaz, puan vermek rolling prompt
cache'i **bozamaz**. Tüm konuşma taranır (puan seyrek ama uzun ömürlü),
`rating: 0` (geri alınan) atlanır; 5 `composeTurnRequest` çağıranının hepsine
bağlı (stream, blocking, btw, wake, context-preview). Test:
`chat_feedback_summary_test.go`. Detay `_Docs\07`.

## Chat yüzey rötuşları: mesaj altbilgisi + bloke-tur uyarısı + canlı panel ✅ (2026-07-24)

Üç UI düzeltmesi: **(1)** Per-mesaj meta + aksiyonlar balon altında **footer
satırına** taşındı — solda pasif meta (saat/süre/model/token), sağda her zaman
görünür aksiyon çipleri (🔊 oku, 👍/👎, yeniden dene, sil; eskiden hover-only
ghost'tu). Ortak stil `chat/messageActions.ts`. **(2)** Tur kullanıcıya bloke
olunca (ask_user / izin / plan) **ayrı chime + masaüstü toast** — interaction
id başına bir kez, reconnect/replay güvenli (`chatStreamHub` + `sounds.ts`).
**(3)** Açık Oturum Bilgisi paneli mesaj girişinde canlı yenilenir —
`bumpMeter` nonce'u bağlanmamıştı, panel mount anında donuyordu; Faz 3 queue
cutover'dan kalan 11 ölü `SendContext` alanı da temizlendi. Ek: oturum
başlığındaki AI-başlık + yeniden adlandırma butonları artık hover'sız her zaman
görünür (`SessionTitleBlock`). Detay `_Docs\07`.

## Workspace listesinde çapraz-workspace "çalışıyor" nabzı ✅ (2026-07-24)

Session "devam ediyor/tamamlandı" göstergeleri sohbet listesinde (yeşil nabız +
`StatusPill`) ve navbar per-view `busy` noktasında vardı, ama bunlar yalnız
**aktif** workspace-scoped (`/api/activity`). Workspace **listesinde** aktif
olmayan workspace'lerde canlı koşu görünmüyordu. Yeni çapraz-workspace sinyal:
backend `GET /api/workspaces/activity` (`handleWorkspacesActivity` + `workspaceRunning`
— her workspace için tek `running` bayrağı; global route, aktif ws gerektirmez),
frontend `useWorkspaceActivity` hook (4sn poll + **iki SSE sinyali**: aktif ws için
`executions`, çapraz-ws için yeni `workspace-activity` — `useAppEvents` çapraz-ws
dalında run-lifecycle olaylarında `bumpWorkspaceActivityForEvent` ile bumlar, aktif
olmayan ws'te başlayan/biten koşu **anlık** yansır → `busyWorkspaceIds`).
`WorkspaceSwitcher`/`MobileWorkspaceButton` her
satırda yeşil nabız (oturum "yazıyor…" görseliyle aynı, `unread`'in önünde);
aktif olmayan bir workspace çalışıyorsa switcher trigger + collapsed rail ikonu
accent noktayı pulse eder. "Tamamlandı" ayrı sinyal değil — mevcut SSE `unread`
accent noktası. `App.tsx` → `NavRail`/`MobileNavBar` boyunca `busyWorkspaceIds`
taşındı. `go vet`, `go test ./internal/api`, `npx tsc --noEmit` yeşil. Detay `_Docs\29`.

## Pano otomasyonlarında sıralama + tek sahip (çift-tetik yarışı) ✅ (2026-07-24)

**TSK59:** Aynı sütunu izleyen iki pano otomasyonu (Kart Sınıflandırıcı AUT7 ve
Board Planner AUT4, ikisi de `move → todo`) aynı olayda ateşliyor, üstelik sıra
`ListEnabledAutomations`'ın map kaynaklı dönüş düzenine bağlı olduğu için
**belirsiz** kalıyordu → iki otonom oturum aynı kart üzerinde yarışıyordu.
Çakışma o güne dek Planner elle kapatılarak önlenmişti. `Automation`'a iki alan
eklendi: **`BoardPriority`** (aynı olaya uyanlar arasında ateşleme sırası, küçük
önce; eşitlikte `ID` ile stabil) ve **`BoardExclusive`** (eşleşen olayı tek
başına sahiplenir, diğer tüm eşleşmeler bastırılır = "sütun başına tek sahip").
Karar mantığı ayrı dosyada — `internal/agent/automation_board_order.go`
`selectBoardAutomations`; `OnBoardChange` artık eşleşmeleri doğrudan gezmek
yerine bu seçiciden geçirip **sırayla** ateşliyor (ikinci kural birincinin
bıraktığı kart durumunu görür). Alanlar db/API/araç/UI boyunca taşındı; API ve
araç tarafında **pointer** oldukları için kısmi patch saklı değeri korur. UI:
`BoardTriggerFields` içine "Sıra" + "Tek sahip" kontrolleri (hem oluşturma formu
hem satır-içi editör), liste satırında `🔒 tek sahip` / `sıra N` rozetleri.
7 yeni birim testi (sıralama, ID tie-break, exclusive bastırma, exclusive
kazanan, sütun-içi izolasyon, filtreleme, boş küme) + `go build/vet`,
`go test ./internal/...`, `npx tsc -b`, `npm run build` yeşil. Detay `_Docs\46`.

## PromptEditor içerik-boyutlu yükseklik (autoSize) ✅ (2026-07-23)

Promptlar & Dosyalar ekranındaki editörler sabit 26rem'lik kutu yerine **içeriğe
göre boyutlanıyor**: `PromptEditor`'a opsiyonel `autoSize` + `autoSizeMax`
(vars. 320px, min 72px) prop'ları eklendi. Edit görünümünde textarea scrollHeight
ölçümüyle, split'te satır yüksekliği imperatif set edilerek (iki pane eşit kalır),
tek-pane önizlemede max-height + shrink-wrap ile çalışır; tavana ulaşan içerik
içten kaydırılır, tam ekran etkilenmez. **Uygulama-geneli VARSAYILAN** (aynı gün
ikinci adım): `autoSize` default `true` — tüm PromptEditor yüzeyleri (workspace
promptları, ajan soul/identity, skill gövdesi `autoSizeMax=560`, flow node
promptu, profil notları, oturum hedefi) kurala uyar; taban `rows`-farkındalı
(`max(72, rows*20+18)` — 12 satırlık authoring alanı boşken 72px'e çökmez);
`autoSize={false}` ile eski sabit kutuya dönülebilir. Görsel doğrulama: kısa
promptlar 72–132px, uzunlar 320px tavanında; ajan formu 416→98/72px; skill
gövdesi içerikle 518px.
`tsc` + prod build + gömülü binary üzerinde Playwright kontrolü yeşil.

## Sesli giriş + sesli okuma (STT/TTS) ✅ (2026-07-23→24)

Composer'a **dikte** geldi: `MicButton` + `useSpeechToText` (Web Speech API,
Chromium-bağımlı; API yoksa gizli), mikrofon dili Ayarlar ▸ **Ses**'ten, aktif
dil tooltip'te. **UI sesleri** merkezi `shared/lib/sounds.ts` (mic blip + yanıt
bitiş chime'ı + onay-bekleyen cue). **TTS** (`shared/lib/tts.ts` + balonda 🔊 +
"Yanıtları sesli oku"): kod/tablo/link ayıklanır; iki motor — tarayıcı
`speechSynthesis` veya **sunucu Piper CLI** (`internal/tts` + `/api/tts`;
telefon/thin client'ta da çalar, yoksa tarayıcıya düşer). **STT'de de iki
motor:** tarayıcı Web Speech veya **sunucu whisper.cpp** (`internal/stt` +
ffmpeg + `/api/stt`; offline/Türkçe, yoksa Web Speech'e düşer). Tüm ses
ayarları tek alt-sayfada: Ayarlar ▸ **Ses** = `SoundPanel` (efektler + STT +
TTS). Detay `_Docs\07`.

## İçgörü kokpiti: kanban Bulgular + reset + Dersler sekmesi + büyüme sınırı ✅ (2026-07-23)

Dört adım: **(1)** Bulgular sekmesi görev panosu gibi **kanban** oldu — 5 sabit
yaşam-döngüsü sütunu, sürükle = statü, kart tıkla = `FindingModal` detay
popup'ı, Ctrl/Shift çoklu seçim + toplu bar (`useMultiSelect`/`SelectionBar`
reuse); bulgu **silme** eklendi (`FindingStore.Delete` +
`DELETE /api/insight/findings/{id}`). **(2)** **Insight reset, iki mod**
(`internal/insight/reset.go` + `POST /api/insight/reset` + Ayarlar'da tehlike
bölgesi): varsayılan ledger'ı korur (eski oturumlar yeniden taranmaz),
`deep=true` sıfırdan (uyarılı). **(3)** **Dersler sekmesi**: reaktif lesson
tarafı (toggle + `LessonsList`) kokpitte — Insight tek öz-iyileştirme ekranı.
**(4)** Tema hizalaması (tanımsız `--color-text-muted` → `--color-text-dim`) +
`FindingModal` opak yüzey. Ayrıca **birikme önleme**: `ledger.jsonl` her
taramada `Compact()`, `runs.jsonl` 1000 kayıtla cap'li. Detay `_Docs\60`.

## Araç güvenilirlik düzeltmeleri (Insight bulgularından) ✅ (2026-07-23)

Insight taramasının yüzeye çıkardığı, koda karşı doğrulanmış dört düzeltme:
**(1) CRLF eşleşmesi** — `Edit`/`apply_patch` çok-satırlı `old_string`'i CRLF
(Windows) dosyada hiç yakalayamıyordu (Read LF verir, disk CRLF); Edit iğneyi
dosyanın satır sonuna hizalar, apply_patch hunk eşleşmesinde `\r` soyar, ikisi
de yazarken CRLF'i korur (`builtin_crlf_test.go`). **(2) `activate_tools`
namespace toleransı** — uydurma `mcp__server__` önekli ad, son `__`-segmenti
katalogda tekil ise çözülür (belirsizse unknown kalır). **(3) bg-shell şema
kapısı** — `Bash`/`PowerShell` şeması `run_in_background`'ı yalnız arka-plan
shell yöneticisi bağlıyken ilan eder (reddedeceğini teklif etme). **(4)
Artifact yol hatası** — çözülmeyen mutlak yol için hata artık "MCP sunucusu
container/uzak host yolu döndürmüş olabilir; içerik döndürün" ipucunu verir
(`artifact_content.go`). Detay `_Docs\19` (2). Testler:
`builtin_crlf_test.go`, `builtin_activate_ns_test.go`,
`builtin_shell_bg_gate_test.go`, `artifact_media_test.go`.

Uygulamaya dağılmış 15 gömülü LLM promptu (summary/title/compact + btw×2, lesson,
insight-analyzer, auto-continue, handoff, continuation, coordinator, subagent×4)
yeni **leaf paket `internal/prompts`**'ta toplandı: default'lar `defaults/*.md`
(`//go:embed`), her anahtar Spec metadata'lı (Türkçe label/hint, zorunlu
`{{yerTutucu}}` listesi, `EpochAffecting`). Çözümleme her yerde tek disiplin:
workspace `config/prompts/<key>.md` override → gömülü default; boş/eksik-yer-tutuculu
dosya sessizce default'a düşer (eski compact-%s guard'ının genellenmişi).
Konumsal `%s` → adlandırılmış `{{...}}` geçişi yapıldı (legacy iki-%s compact
override'ı okuma anında otomatik dönüştürülür). **Seed politikası değişti:**
prompt default'ları artık dosya olarak seed edilmez; eski default-aynısı seed
artıkları temizlenir → default iyileştirmeleri edit'lenmemiş workspace'lere
otomatik ulaşır. Subagent allowlist'leri bilinçli olarak kodda kaldı (güvenlik
sözleşmesi); btw araçsızlığı yapısal zorlamada. **Prompt izi:** `WithPromptTrace`
→ `debug.jsonl` `llm_call` olaylarına `promptKey`+`promptHash` (düzenlenmiş
prompt ≠ default hash'i → etki ölçülebilir). API: `workspace-config` DTO'suna
`promptMeta`; UI: Promptlar & Dosyalar ekranı registry-güdümlü ("özelleştirildi",
"yeni oturumlarda etkili" epoch rozeti, eksik-yer-tutucu uyarısı). Drift guard:
`TestRegistryConsistency` (yetim dosya/anahtar = test kırılır). Detay `61`.
`go build`+`vet`+ilgili paket testleri + frontend `tsc` yeşil.

## claude-cli hook matcher'ı: bridged-shell genişletme (tüm mevcut workspace'lere uygulandı) ✅ (2026-07-14)

Bir önceki (2026-07-13) regex-çevirisi düzeltmesinin tamamlayıcısı. **Kalan boşluk:** CLI'da
built-in shell açıkken araç adı **köprülü** görünür (`mcp__tionswarm_interaction__PowerShell`),
düz `PowerShell` değil. Yani matcher'ı yalnız `Bash,PowerShell` olan hook'lar (regex-çevirisi
sonrası `^(Bash|PowerShell)$` bile) köprülü ismi kapsamadığından CLI'da **hâlâ ateşlenmezdi**.
Tüm workspace'ler tarandı: WS1/WS5/WS10'un enabled sqz hook'ları matcher'da köprülü isimleri
zaten taşıyordu (elle eklenmiş — çalışıyordu); **WS15'in enabled sqz hook'u yalnız
`Bash,PowerShell`** taşıyordu → açık kurbandı.

**Çözüm (kod, evrensel — veri düzenlemesi YOK):** `cliMatcherRegex` artık matcher'daki
`Bash`/`PowerShell` alternatiflerine köprülü formu (`interactionToolPrefix+ad`) **otomatik
ekler** (deduplu). Düz `Bash,PowerShell` → `^(Bash|mcp__tionswarm_interaction__Bash|PowerShell|
mcp__tionswarm_interaction__PowerShell)$`. Böylece WS15 + gelecekteki her workspace + tek-tık
"Bağla" şablonu CLI'da veri düzenlemeden ateşlenir; zaten köprülü ismi olanlar deduple aynı
kalır. Native yol etkilenmez (olmayan araç zaten eşleşmez).

Test: `climcp_matcher_test.go` güncellendi (WS15-şekli `Bash,PowerShell` düz matcher köprülü
tools'a ateşler; superset'e uymaz; entegrasyon: writeCLISettings çıktısı genişletilmiş regex).
`go build`+`vet`+`go test ./internal/agent` yeşil (200).

**Canlı E2E (uçtan uca kanıt):** WS15'e geçici bir marker PreToolUse hook'u (matcher düz
`PowerShell`) eklendi, AGT1'e (claude-cli) gerçek bir PowerShell turu tetiklendi. Ajan cevabı:
hook **`mcp__tionswarm_interaction__PowerShell` çağrısında ateşlendi** → düz `PowerShell`
matcher'ı köprülü CLI aracına eşleşti (fix'ten önce eşleşmezdi). **Bonus keşif:** Claude Code
PreToolUse hook'larını Windows'ta **bash/sh ile** çalıştırıyor (TionSwarm native yol PowerShell
ile) — marker ham-PS sözdizimindeydi, bash altında `syntax error` verdi. **Etki:** sqz hook'u
`powershell -NoProfile … -File sqz-bridge-hook.ps1` (bash-geçerli) olduğu için ateşlendiğinde
sorunsuz çalışır; ama **ham-PS sözdizimli rtk hook'ları** (`$j=[Console]::In.ReadToEnd()|…`,
şu an her workspace'te DISABLED) CLI'da bash altında kırılır — etkinleştirilirse sqz gibi
`powershell -File` sarmalayıcısına çevrilmeli. Marker+test-session temizlendi.

## claude-cli hook matcher'ı: virgül-glob → regex çevirisi (sqz/rtk CLI'da sessiz çalışmıyordu) ✅ (2026-07-13)

**Kök neden (bir oturum incelemesinde yakalandı):** TionSwarm'ın **native** hook matcher'ı
(`hookMatches`) virgül-ayrık **filepath.Match glob listesi** (`Bash,PowerShell`,
tam-eşleşme). Ama `climcp.go writeCLISettings` matcher'ı claude-cli'ın `--settings`'ine
**verbatim** yazıyordu. **Claude Code matcher'ı REGEX sayar** (alternation `|`, virgül
literal, ankraj yok) → `Bash,PowerShell,mcp__tionswarm_interaction__PowerShell,…` virgüller
dahil o literal diziyi arar, hiçbir tekil araç adına uymaz → **hook claude-cli turlarında
SESSİZCE hiç ateşlenmez** (ajanların çoğunun kullandığı yol). Sonuç: virgüllü matcher'lı
sqz/rtk optimizerları CLI ajanlarında ölüydü — bir WS10 oturumunda 23 PowerShell/git komutu
ham çalışmış, sqz sıfır optimizasyon yapmış (komut+çıktı ham, "sqz" 0 kez).

**Çözüm:** yeni `internal/agent/climcp_matcher.go` — `cliMatcherRegex` matcher'ı iki lehçe
arası köprüler: virgülle böl → her glob'u regex'e çevir (`*`→`.*`, `?`→`.`, diğer metachar'lar
escape) → `|` ile birleştir → `^…$` ankraj (native `filepath.Match`'in tam-eşleşme semantiğini
aynala, `Bash` artık `BashOutput`'a uymasın). `writeCLISettings` artık `cliMatcherRegex(h.Matcher)`
yazıyor. Boş matcher boş kalır (iki motor da "tüm araçlar" sayar). Native yol değişmedi.

Test: `climcp_matcher_test.go` — dönüştürme tablosu + davranışsal ateşleme (bridged
`mcp__…__PowerShell`'e uyar, superset'e uymaz) + **entegrasyon** (`writeCLISettings` çıktısında
dönüştürülmüş regex, verbatim virgül-liste YOK). `go build ./...` + `go vet` + `go test
./internal/agent` yeşil (203). Backend rebuild+restart edildi; artık her CLI turunda doğru
matcher yazılıyor → sqz/rtk gerçekten ateşlenir.

## Loglama revizyonu: kaynak alanları + SSE canlı akış + Loglar ekranı yükseltmesi ✅ (2026-07-13)

- **Entry modeli:** `logbuf.Entry`'ye birinci sınıf **`component` / `session` /
  `agent` / `workspace`** alanları eklendi — handler aynı-isimli slog attr'larını
  bu alanlara terfi ettirir (attrs map'inden çıkarır). Kablolama: runtime
  `component=agent`+`workspace=<id>`, scheduler/automation/api/backup/workspace/
  mcp-pool/cli-pool kendi component etiketlerini alır (`runtime.go`,
  `workspace/manager.go`, `app/app.go`).
- **SSE canlı log akışı:** `logbuf.Buffer.SetNotify` → her kayıt `events.Bus`'a
  `Type="log"` olarak yayınlanır (`app.Bootstrap`), `/api/events` `log` SSE
  event'iyle iletir. Loglar ekranı artık 2.5sn poll yerine **canlı tail** yapar
  (30sn'de bir mutabakat poll'u SSE kopmalarını kapatır).
- **API filtreleri:** `GET /api/logs`'a `component`, `session`, `since`, `until`
  (unix ms) parametreleri; `entryMatches` terfi eden alanlarda da arar.
- **`read_logs` aracı:** `q` artık attrs + kaynak alanlarında da arar (önceden
  yalnız mesajdı — API ile tutarsızdı); `component` ve `session` filtreleri eklendi.
- **Gürültü:** `toolloop.go` "tool call" logu INFO→**DEBUG** (yoğun oturumda tur
  başına 100+ satır tamponu domine ediyordu; block/deny INFO'da kaldı).
- **Sessiz bölge logları:** `db.atomicWriteBytes` (mkdir/tmp/rename) ve `nextID`
  best-effort counter yazımı hataları WARN (`component=db`, slog default tee'li
  olduğundan Loglar ekranına düşer); `tools/registry.go` MCP çağrı transport
  hatası WARN (`component=mcp`) — önceden yalnız model'e dönen IsError'dı.
- **Loglar ekranı (UI):** bileşen filtresi (satırdaki rozet tıklanabilir),
  zaman aralığı seçici (15dk/1sa/24sa), 300ms debounce'lu arama + `<mark>`
  vurgusu, satır kopyalama (hover), filtrelenmiş JSON indirme, takip kapalıyken
  "N yeni kayıt — Yenile" rozeti, `content-visibility:auto` ile ucuz
  virtualization, session/agent/workspace alanları ayrı gösterim. Gruplama
  imzasına kaynak alanları dahil edildi (`logGroup.ts`).
- **Testler:** `go build ./...` + 561 test (logbuf/api/tools/db/events/workspace/
  agent/app) ve frontend `npm run build` temiz.
- **Doküman:** `_Docs/12-LOGLAMA.md` güncellendi.

## `archive_sessions` tüm oturum tiplerini süpürebiliyor (`kinds`) ✅ (2026-07-13)

- **Sorun:** Araç `s.Kind != "chat"` ile sabit filtreliyordu → otonom çalıştırmaların
  ürettiği oturumlar (`spawned`, `worker`, `flow`, `task`, `schedule`, `inbox`)
  **hiçbir toplu araçla arşivlenemiyordu**. Board otomasyon zinciri her kart için
  `spawned` oturum üretiyor, bunlar birikiyordu. Geriye tek yol ham REST kalıyordu —
  ki bu workspace scope'unu kaybettiren footgun (bir kez yanlış workspace arşivlendi).
- **Çözüm:** Opsiyonel `kinds` alanı. Geçerli tipler `chat`/`spawned`/`worker`/`flow`/
  `task`/`schedule`/`inbox` + hepsi için `["*"]`. **Verilmezse varsayılan `["chat"]`** →
  mevcut çağrılar birebir aynı davranır (geri uyumluluk kritikti: varsayılan genişletilseydi
  eski "temizlik" çağrıları aniden flow/schedule/inbox'ı da süpürürdü).
- **Nasıl:** Tip mantığı ayrı dosyada — `internal/tools/builtin_sessionkinds.go`
  (`archivableSessionKinds` otoritatif liste, `resolveArchiveKinds` çözümleme +
  trim/lowercase). Bilinmeyen tip **hata döner, sessizce yutulmaz** (`"spawn"` gibi bir
  typo aksi halde "arşivlenecek bir şey yok" diye okunurdu). `dry_run` ve uygulanan
  çıktı her satırda tipi gösterir (`- SES12 · [flow] · "..."`), başlıkta süpürülen tip
  seti yazar. Mevcut korumalar (`currentSessionID` hariç tutma, `exclude`, `idle_days`,
  `title_contains`, `limit` 100) `["*"]` altında da geçerli.
- **Not:** `worker` tipi plandaki 6 tipe ek olarak dahil edildi — coordinator spawn'ları
  (`spawn.go:120`) bu tipi üretiyor, listede olmasa süpürülemez kalırdı.
- **Testler:** `builtin_sessionkinds_test.go` (7 yeni test: chat-only varsayılan,
  seçili tip, `["*"]`, bilinmeyen tip hatası, dry-run tip gösterimi, `["*"]` altında
  guard'lar, helper birim testi). Mevcut 4 test **değiştirilmeden** geçiyor → geri
  uyumluluk kanıtı. `go build ./...`, `go vet ./...`, `go test ./internal/...` temiz.
- **Doküman:** `_Docs/24-SELF-MANAGEMENT.md` satır 60 güncellendi.

## Global skill değişiminde "tüm workspace'leri etkiler" toast'ı ✅ (2026-07-13)

- **Ne:** Skills ekranında **global** tier bir skill'in görünürlüğü, erişimi (shared)
  veya gövdesi değiştiğinde sağ-altta bilgilendirici bir toast çıkar: değişikliğin
  tüm workspace'lerde geçerli olduğunu (workspace override'ları hariç) belirtir.
- **Neden:** Global skill dosyası paylaşımlı global dizinde (`~/.tionswarm/skills`) →
  bir workspace'te yapılan düzenleme sessizce diğerlerini de etkiliyordu; kullanıcı
  bunu görmüyordu.
- **Nasıl:** Yeni `shared/components/InfoToast.tsx` (ErrorToast'ın nötr/accent kardeşi,
  `bottom-20 right-4` → error toast'ıyla çakışmaz). `SkillsPanel` üç mutasyon yolunda
  (`toggleAccess`/`setVisibility`/`onEditorSaved`) `active.source === 'global'` ise
  tetikler. Yalnız frontend, self-contained (App error plumbing'e dokunulmadı).
  `npx tsc --noEmit` temiz.

## Yeni default skill `tionswarm-terse` (caveman-esinli terse mod) ✅ (2026-07-13)

- **Ne:** Gömülü skill `internal/skills/defaults/tionswarm-terse/SKILL.md` (`🪨 TionSwarm
  Terse Mode`, `access: shared`). Caveman skill'inin (github.com/JuliusBrussee/caveman)
  özünü TionSwarm'a uyarlar: dolgu/nezaket/hedge at, teknik özü koru; **kod/komut/yol/
  hata string'leri byte-for-byte aynen**. 3 seviye (`lite`/`full`/`ultra`), dil-koruyan
  (çeviri yok), oturum-sürekli, "normal mode" ile kapanır.
- **Neden:** Yalnız **output token** kısar (caveman ölçümü ort. %65). Prompt-seviyesi,
  opt-in, cache-dostu — TionSwarm'ın güvenlik/izin/araç davranışını değiştirmez.
- **Nasıl:** `//go:embed defaults` yeni klasörü otomatik seed eder (kod değişikliği yok).
  Caveman'in destructive/security kalıplarında caveman'i kapatan **auto-clarity** kuralı
  TionSwarm izin-gate'leriyle hizalı biçimde korundu. `go build/test ./internal/skills/`
  yeşil (37 test). **Not:** çalışan binary için yeniden derleme + restart gerekir.

## `list_sessions` artık tüm kind'leri listeler + `kind` filtresi ✅ (2026-07-13)

- **Ne:** `list_sessions` aracı varsayılan olarak **her kind'i** döndürüyor (chat +
  spawn/worker/flow/task/schedule). Eski sabit `Kind=="chat"` eleme kaldırıldı;
  opsiyonel `kind` argümanı tek kind'e daraltır. Her satır artık `[kind·state]`
  ön ekiyle başlar (ör. `[spawned·active]`).
- **Neden:** Otonom koşular (board otomasyon spawn'ları vb.) UI'nın sidebar +
  SessionsOverview'ında görünürken ajanın `list_sessions`'ında **hiç** görünmüyordu
  → ajan "session listesi eksik geliyor" durumu yaşıyordu (WS5/SES158 teşhisi).
- **Nasıl:** `internal/tools/builtin_sessions.go` — filtre `kindFilter != "" &&
  !sessKindMatches(...)` oldu; `sessKindMatches` (legacy `""`→chat) + `sessKindLabel`
  yardımcıları eklendi; şema/açıklama güncellendi. Test `builtin_sessions_test.go`
  yeni davranışa göre yazıldı; self-management SKILL.md + `27-CROSS-SESSION-SEARCH.md`
  senkron. `go build ./...` + `go test ./internal/tools/` yeşil. **Not:** çalışan
  binary'nin görmesi için yeniden derleme + restart gerekir.

## Otomatik artifact yakalama toggle'ı (`AutoCaptureArtifacts`) ✅ (2026-07-13)

- **Ne:** Yazılan dosyaların tur sonunda otomatik artifact yapılması artık **workspace
  ayarı** ile açılıp kapanabiliyor ve **varsayılan KAPALI**. Kapalıyken yalnız ajanın
  **bilerek** `create_artifact` çağırdığı içerikler artifact olur — proje kaynak
  dosyalarını düzenlemek Artifacts'ı kirletmez. Açıldığında eski davranış: ajanın
  `Write`/`create_file` ile yazdığı her dosya `captureFileArtifacts` ile Artifacts
  ekranına düşer.
- **Neden:** Kaynak-kodu düzenleyen ajan turları (ör. `ART30.cs`) istenmeden artifact
  üretiyordu. Varsayılan bilerek-artifact'a çekildi; isteyen workspace toggle'ı açar.
- **Nasıl:** `WSSettings.AutoCaptureArtifacts` (varsayılan `false`) + patch/DTO. `chat.go`
  ve `chat_stream.go`'daki 4 `captureFileArtifacts` çağrısı bu ayara koşullandı. Prompt
  yönlendirmesi de takip ediyor: `artifactGuidanceFor(auto)` — kapalıyken ajana "dosya
  yazımı artifact üretmez, deliverable'ı `create_artifact` ile kaydet" der
  (`artifactDeliverableGuidanceManual`). Frontend WorkspacePanel'de "Artifact yakalama"
  toggle'ı.
- **Doğrulama:** `go build`/`go test ./internal/api ./internal/workspace` temiz (default
  testi genişletildi); frontend `tsc --noEmit` temiz. WS10 için ayar `false`'a alındı.

## Shell adımında program ikonunun yanına program adı ✅ (2026-07-13)

- **Ne:** Sohbetteki Bash/PowerShell (ve `transform_data`/`run_code`) araç adımında,
  marka ikonunun yanında artık programın **adı** da yazıyor: `<ikon> (curl) Bash`.
  Böylece adımın hangi programı çalıştırdığı ikonu tanımadan da okunabiliyor.
- **Nasıl:** `shared/lib/programIcons.ts`'e `resolveProgram(command)` eklendi — ilk
  tanınan programın **adını + ikonunu birlikte** döndürür (yalnız-ikon döndüren
  `resolveProgramIcon` kaldırıldı). Yeni `features/chat/CommandProgramTag.tsx` ikonu +
  `(ad)` etiketini render eder; `ActivityCard` eski `CommandProgramIcon` yerine bunu
  kullanır (eski bileşen dosyası silindi).
- **Sınır:** Ad yalnız program **tanındığında** (ikonu varsa) gösterilir; tanınmayan
  komutlarda (PowerShell cmdlet'i, düz `ls`…) davranış eskisi gibi — ne ikon ne ad.
- **Doğrulama:** `go build ./...` + `go vet ./...` + `go test ./internal/...` temiz;
  frontend `npx tsc -b` + `npm run build` temiz.

## Shell adımında komut-programı marka ikonu ✅ (2026-07-12)

- **Ne:** Bir Bash/PowerShell araç adımında, komuttaki programı (git/npm/docker/python
  /go/cargo/kubectl…) tespit edip tool ikonunun yanında küçük **marka SVG'si** gösterir
  (external-agent-oss'taki "hangi programı kullandığına göre ikon" davranışının frontend-only
  uyarlaması).
- **Nasıl:** `shared/lib/commandProgram.ts` komutu parse eder (env/`sudo`/pipe/chain/
  path/`bash -c`+`pwsh -Command` sarmalayıcıları). `shared/lib/programIcons.ts`
  program adını (**~90 alias**, ~75 marka) `simple-icons` glyph'ine eşler; ilk eşleşen
  program kazanır. `features/chat/CommandProgramIcon.tsx` 13px SVG'yi brand-hex + `title`
  ile render eder; eşleşmezse (cmdlet/`ls`…) hiçbir şey.
- **Kapsam:** git/gh, npm/pnpm/yarn/bun/node/deno, tsc/vite/webpack/esbuild/rollup/turbo/
  next/astro/prisma/eslint/prettier/biome/jest/vitest/cypress, python/pip/poetry/uv/pytest/
  conda, go/cargo, docker/podman/kubectl/helm/terraform/pulumi/ansible/vagrant, java/dotnet/
  gradle/maven/ant/scala/kotlin, ruby/php/composer, dart/flutter/swift/elixir/julia/r/perl/lua,
  clang/cmake/make, psql/mysql/mongo/redis/sqlite, nginx, gcloud/vercel/netlify/supabase/
  firebase/wrangler, nvim/vim/brew/pacman/wasmer, curl. (aws/azure/playwright simple-icons'ta yok.)
- **Bonus:** `ActivityCard`'ta yalnız Bash/PowerShell (`input.command`) **ve** `transform_data`
  /`run_code` (`input.language` → python/node/bun…) adımlarında `<meta.icon>` yanına eklenir.
- **Bağımlılık/bundle:** `simple-icons` (named import → tree-shake doğrulandı, yalnız
  kullanılan ikonlar bundle'a girer). `tsc --noEmit` + `vite build` temiz; parser 12
  örnek vaka ile node sanity-check'ten geçti (frontend'de unit-test runner yok).

## TurnStep ikonları tek kaynağa çekildi (sohbet ↔ ayar ekranı) ✅ (2026-07-12)

- **Sorun:** `stepKinds.ts` "tek doğruluk kaynağı" olduğunu iddia etse de yalnız ayar
  ekranı (`StepKindsPanel`) onu tüketiyordu (emoji); sohbet step bileşenleri ikonları
  ayrı ayrı hardcode ediyordu (lucide SVG). Drift vardı (ör. `thinking` ayarda 🧠,
  sohbette 💭).
- **Çözüm:** Metadata `@/shared/stepKinds.ts`'e taşındı; her kind artık bir **lucide
  `Icon`** taşır (+ `STEP_KIND_MAP` O(1) lookup). Ayar ekranı ve **tüm** sohbet step
  bileşenleri (TextStep/ThinkingBlock/ErrorStep/RecoveryStep/SteerStep/HookStep/
  SubagentStep/ContextChangeCard/TodoCard/DiffCard/ToolDeltaStep) aynı ikonu oradan
  render eder → görsel birebir aynı, drift imkânsız. `tsc --noEmit` + `vite build` temiz.

- **Teşhis:** Aynı tura düşen birden fazla `<task-notification>`'dan biri koordinatör
  LLM'i tarafından gözden kaçırılınca (biten worker'ı "hâlâ çalışıyor" sanması), o tur
  `pending` olmadan bitip drain döngüsü çıktığı için atlanan completion bir daha
  ziyaret edilmiyordu → koordinatör zaten biten bir worker'ı sonsuza dek bekliyordu.
  Kuyruk mekanizması bildirimi kaybetmiyor; açık **reasoning + liveness** katmanında.
- **Fix 1 — otoriter worker-state bloğu:** `coordinatorWorkerStatusBlock` her koordinatör
  turunun dinamik system suffix'ine (`autonomousDynamicSuffix`, coordinator-only) canlı
  `ListWorkers` durumunu enjekte eder → model biten worker'ı "çalışıyor" sanamaz.
- **Fix 2 — idle reconciliation sweep:** `drainCoordinator` çıkışta, koordinatör worker
  spawn etmişse (`hadWorkers`) ve **tüm** worker'lar bitmişse (`workers==0`) ve bu batch
  için henüz yapılmadıysa (`ackedIdle`), tek-seferlik `<coordination-status>All workers
  finished…</coordination-status>` notu ekleyip bir otoriter tur daha koşar. `ackedIdle`
  bir sonraki bildirimde re-arm olur; `CoordinatorMaxTurns` cap'i sınırlar → sonsuz döngü
  yok. LLM bir bildirimi atlasa bile stall imkânsız.
- **Testler:** `TestIdleReconcileSweepRunsFinalTurn` (process+reconcile, one-shot, re-arm),
  `TestIdleReconcileSkippedWithoutWorkers`; mevcut coalesce/user-turn testleri yeşil.
  (`coordination.go`, `runtime.go`, `coordination_test.go`.)

## MCP sunucu satırına "Kopyala" butonu ✅ (2026-07-12)

- **İstek:** Araçlar & MCP ekranında custom MCP eklendikten sonra Test/Aç-Kapat/Sil
  yanına, sunucu yapılandırmasını başka yerlere yapıştırabilmek için bir **Kopyala**
  butonu.
- **Çözüm:** `ServerManagement.tsx`'e Test'ten hemen sonra **Kopyala** butonu — sunucuyu
  standart `mcpServers` JSON belgesi (Claude Code / .mcp.json biçimi; "JSON ile içe aktar"
  kutusunun kabul ettiği aynı şekil) olarak panoya kopyalar → başka workspace/araca
  yapıştırıp içe aktarılabilir. Tıklayınca 1.5sn "Kopyalandı" geri bildirimi verir.
  `navigator.clipboard` yoksa (güvensiz bağlam) gizli textarea + `execCommand('copy')`
  fallback'i. transport'a göre yalnız ilgili alanlar yazılır (stdio → command/args/env,
  http → url/headers).
- **Değişiklikler:** `toolMeta.ts` — `serverToImportJson(server)` + `parseJsonObject(raw)`
  yardımcıları (env/headers JSON string'lerini güvenli parse). `ServerManagement.tsx` —
  `copiedId` state + `copyServer`, `data-testid="mcp-server-copy"`.
- Doğrulama: `tsc --noEmit` temiz.

## archive_sessions: kendi oturumunu da arşivleyebilir (include_current) ✅ (2026-07-11)

- **Karar:** `archive_sessions` artık mevcut oturumu **yalnız varsayılan olarak** hariç
  tutuyor; yeni `include_current: true` argümanıyla ajan **kendi çalıştığı oturumu da**
  arşivleyebilir. Arşivleme soft/geri-alınabilir ve çalışan turu durdurmaz (sadece
  aktif listeden çıkar). `exclude` listesi include_current ile de geçerli.
- **Değişiklikler:** `builtin_sessionarchive.go` — args'a `IncludeCurrent bool`, keep-set'e
  ekleme artık `!args.IncludeCurrent` koşullu; Def açıklaması + şema (`include_current`),
  struct/constructor yorumları ve "nothing to archive" mesajı güncellendi (koşullu ipucu).
  Not: şema açıklamasındaki `` `exclude` `` backtick'leri raw-string literal'i erken
  kapatıyordu → düz metne çevrildi. Test: `builtin_sessionarchive_test.go` →
  `TestArchiveSessionsIncludeCurrent`.
- Doğrulama: `go build ./...` + `go test ./internal/tools -run TestArchiveSessions` yeşil.

## Otonom-tur boot kurtarması + SpawnIdle Settings UI ✅ (2026-07-13)

- **Sorun:** Worker/spawn/koordinatör turları fire-and-forget goroutine; crash/restart
  onları sessizce öldürüyor → session yanıtsız `state=active` kalıyor VE worker'ın
  koordinatörü hiç bildirim almadığı için sonsuza dek bekliyor (SES28 donması). Inbox
  turları kurtarılıyordu ama otonom turlar için kurtarma YOKtu.
- **Çözüm — boot kurtarması** (`Runtime.RecoverOrphanedTurns`, `coordination.go`;
  boot'ta `server.go` → `recoverAutonomousTurns` her workspace runtime için çağırır):
  son mesajı **user** olan (yani yarıda kalmış) otonom session'lar için:
  - **worker** → interrupted assistant reply yaz + koordinatöre sentetik
    `<task-notification status="killed">` enjekte et → koordinatör beklemeyi bırakır,
    yeniden görevlendirebilir/sonuçlandırabilir;
  - **koordinatör** (yarıda ölmüş) → bir koordinatör turu re-enqueue → geçmişten devam;
  - **düz spawn** → interrupted reply (donuk görünmesin).
  Idempotent (reply eklenince son mesaj assistant olur, ikinci boot atlar), archived
  session'lara dokunmaz.
- **Testler:** `internal/agent/recover_orphan_test.go` (worker kurtarma + koordinatör
  bildirimi + idempotent + tamamlanmış worker'a dokunmama).
- **SpawnIdleTimeoutMin Settings UI:** AppToolsPanel'e "Spawn boşta süresi (dk)" alanı
  eklendi (spawn üst-sınır alanının yanına); tip + save payload + backend round-trip
  `spawnTimeoutMin` ile birebir. "Spawn süresi" etiketi "üst sınır" olarak netleştirildi.
- **Doğrulama:** full suite **847 test** yeşil, vet temiz, frontend tsc temiz.
- **Kalan:** durable spawn KUYRUĞU (N-bounded, "limit reached" yerine sıraya al) —
  kullanıcı isteğiyle şimdilik ertelendi.

## Otonom tur timeout'u: hard-cap tunable + idle watchdog ✅ (2026-07-13)

- **Sorun:** Worker + koordinatör turları **hardcoded 10 dk `spawnTimeout` const**'una
  takılıydı (sıradan spawn zaten 20 dk tunable `r.tun.SpawnTimeout()` kullanıyordu) →
  ağır keşif worker'ları (72-101 tool çağrısı) tam 600sn'de tur ortasında kesiliyordu
  (SES33). Ayrıca mutlak duvar-saati "asılı" ile "uzun ama üretken"i ayırmıyordu.
- **Çözüm — iki katmanlı süre sınırı** (`internal/agent/activity_timeout.go`
  `withActivityTimeout`): (1) **hard-cap** = `r.tun.SpawnTimeout()` (default 20 dk),
  (2) **idle watchdog** = yeni tunable `r.tun.SpawnIdleTimeout()` (default **5 dk**).
  Tur her adım yaydığında (`SessionStepEmitter` → `activityTouchFrom` → touch) idle
  timer resetlenir; adım akmayan (gerçekten asılı) tur idle penceresinde iptal edilir,
  üretken uzun tur hard-cap'e kadar koşar. Hangisi önce dolarsa ctx iptal.
- **Kapsam:** worker, koordinatör, spawn ve inbox-delivery **iş turları** artık
  hard-cap + idle kullanıyor (10 dk const yalnız turn-finished/failed **hook**
  dispatch'inde kaldı — iş turu değil).
- **Ayarlanabilir:** `SpawnIdleTimeoutMin` settings alanı (default 5) — `SpawnTimeoutMin`
  ile birebir aynı plumbing (settings.go + defaults + server.go applySettings).
- **Testler:** `internal/agent/activity_timeout_test.go` (idle iptal, touch canlı
  tutar, hard-cap, tunable). Full suite **844 test** yeşil, vet temiz.
- **Not:** Bu, süreç **restart**'ında öksüz kalan turları çözmez (o hâlâ ayrı bir iş:
  otonom-tur boot kurtarması). Bu değişiklik yalnız **asılı/uzun** turların timeout
  davranışını düzeltir.

## Otonom turlar için "çalışıyor" göstergesi + gerçek Durdur ✅ (2026-07-12)

- **İhtiyaç:** Canlı adım köprüsü düzeldikten sonra ghost balonu akıyordu ama otonom
  turlarda (koordinatör/scheduler/spawn) "çalışıyor" göstergesi + composer Durdur/
  Kes/Yönlendir kümesi çıkmıyordu — çünkü frontend `streamingSessions`'ı yalnız
  `KindUserMessage`'da işaretliyor, otonom tur bunu yayınlamıyor.
- **Frontend:** `chatStreamHub.ts` — ilk hub aktivitesinde (`KindAgentStart` ve
  `KindStep`) oturum `streamingSessions`'a eklenir → transkript göstergesi +
  composer busy-durumu interaktifle simetrik çıkar. `turn_done`/`turn_error` (ve
  global tamamlanma feed'i) temizler.
- **Backend:** `autonomousInteraction` (`autonomous_interaction.go`) artık **gerçek
  bir cancel** kaydediyor (eskiden no-op): `ctx, cancel := context.WithCancel(ctx)`
  → dönen iptal-edilebilir ctx CLI provider çağrısına akar, cleanup `cancel()`+
  `unregister`. Böylece izleyicinin "Durdur"/"Kes"i otonom claude-cli turunu
  gerçekten durdurur. `coordination.go` `runCoordinatorTurn`/`runWorker`:
  `errors.Is(err, context.Canceled)` → temiz "⏹️ … durduruldu" mesajı (hata değil).
- **Doğrulama:** api+agent **274 test** yeşil, vet temiz, frontend tsc temiz.

## Bug fix — otonom tur canlı adımları hub'a köprülenmiyordu ✅ (2026-07-11)

- **Teşhis:** Koordinatör (ve scheduler/spawn/worker) otonom turlarında ajanın
  düşünce/tool adımları canlı görünmüyor, yalnız tur bitince toptan geliyordu.
- **Kök neden:** `autonomousInteraction` her otonom claude-cli turu için Interaction
  MCP Bearer token'ını eşlemek üzere **token-only bir chatRun** kaydediyor
  (`run.autonomous=true`). `bridgeBusToHub` `session_step` guard'ı ise
  `if _, live := sessionRunInfo(sid); live { continue }` ile "canlı run'ı olan
  oturumu atla" yapıyordu — bu guard **interaktif** turlar için doğru (runChatTurn
  adımları zaten doğrudan hub'a yayınlar, çift yayını önler), ama otonom token-only
  run hub'a hiçbir şey yayınlamadığından adımlar **düşüyordu**. Tur bitince run
  unregister → completion event `turn_done`+reload → her şey birden.
- **Çözüm:** guard yalnız **interaktif** run'ı atlasın:
  `if info, live := sessionRunInfo(sid); live && !info.Autonomous { continue }`
  (`internal/api/session_stream.go`). Otonom run'ların adımları artık köprüleniyor.
- **Testler:** `internal/api/bridge_autonomous_test.go` — otonom adımlar köprüleniyor,
  interaktif adımlar çift yayınlanmıyor. api paketi 96 test yeşil.

## claude-cli canlı steer (Yönlendir) ✅ (2026-07-11)

- **Sorun:** "Yönlendir" yalnız native provider'da çalışıyordu; claude-cli için
  backend `unsupported` dönüp mesajı kuyruğa düşürüyordu (mid-turn rehberlik yok).
- **Çözüm (external-agent muadili, Doc 59):** claude-cli için steer mesajı
  `chatRun.pendingSteer`'a saklanır (`setSteer`/`takeSteer`, `chat_control.go`);
  `handleSessionControl` claude-cli → stash + `"steered"` (`inbox.go`);
  `callPermission` her **allow** (auto-allow RiskRead + prompt sonrası) sınırında
  mesajı `additionalContext` olarak enjekte eder (`permDecisionCtx`/`steerContext`,
  `mcp_interaction_tools.go`) → rehberlik bir sonraki **tool sınırında** turu
  yeniden başlatmadan bağlama girer. Tur tool'suz (yalnız metin) biterse
  `runChatTurn` bekleyen mesajı **sonraki tura enqueue** eder (steer_undelivered
  fallback, `chat_stream.go`).
- **Testler:** `internal/api/steer_cli_test.go` (stash consume-once, additionalContext
  enjeksiyonu, steer yokken plain karar). api paketi 94 test yeşil; frontend tsc temiz.
- **Açık doğrulama:** `additionalContext`'in claude-cli permission cevabında modele
  gerçekten girip girmediği CLI sürümüne bağlı (canlı test). Girmezse Doc 59 Faz 2
  seçenek (B) PreToolUse hook kanalına geçilir; fallback her hâlükârda güvenli.

## Shell sağlamlaştırma: non-interactive git + süreç-ağacı reap ✅ (2026-07-11)

- **Teşhis:** Bir ajan shell aracıyla `git commit` çalıştırınca tur tamamen
  kilitleniyordu. İki kök neden: (1) `core.editor=notepad` → git editör açmak
  isteyince headless/stdin'siz alt-süreçte **notepad GUI'si sonsuza dek bloke**
  oluyordu (env'de `GIT_EDITOR` yoktu); (2) timeout'ta `exec.CommandContext`
  yalnız doğrudan çocuğu (bash/powershell) öldürüp **spawn edilen `git.exe`+editör
  torununu zombi bırakıyordu** → `.git/index.lock` kalıyor, sonraki commit'ler
  bloke. Gözlemde ~28 zombi git süreci + 10 dakikalık boşa tur.
- **Çözüm (uygulama geneli):** `internal/proc` katmanına iki primitive:
  - `HardenedEnv(base)` → non-interactive guard'lar (`GIT_TERMINAL_PROMPT=0`,
    `GIT_EDITOR=true`, `GIT_SEQUENCE_EDITOR=true`, `GIT_PAGER=cat`/`PAGER=cat`,
    `GIT_OPTIONAL_LOCKS=0`, `GCM_INTERACTIVE=never`). Guard'lar **en sona
    eklenir** → os/exec last-wins dedup'ı ile miras `notepad` editörünü ezer.
  - `TreeKill(cmd)` → ctx iptal/timeout'ta **tüm çocuk ağacını** öldürür (Windows
    `taskkill /F /T`; Unix `Setpgid` + negatif-pid `SIGKILL`) + `WaitDelay` (5sn)
    ile takılan doğrudan çocuğu force-kill eder.
  - Bağlanan yerler: `builtin_shell.go` Bash+PowerShell `build` closure'ı ortak
    `hardenShellCmd` (foreground + `run_in_background` ikisi de) ve claude-cli
    `cliBaseEnv` (CLI ajanının kendi git'i de non-interactive).
- **Testler:** `internal/proc/env_test.go` — guard override (notepad→true),
  TreeKill Cancel/WaitDelay set; build+vet+tools/providers testleri yeşil.
- **Confined imza kapatma:** `DisableGitSigningEnv()` → `commit.gpgsign=false` +
  `tag.gpgsign=false` (git `GIT_CONFIG_COUNT/KEY/VALUE` env-injection). Yalnız
  **confined (otonom/spawn) shell'lerde** enjekte edilir (`hardenShellCmd(cmd,
  t.sb.Confined)`); interaktif turlar imzayı korur (insan passphrase girebilir).
  Böylece nezaretsiz `git commit` GPG pinentry'de asılamaz.

## Çapraz-session farkındalığı: tamamen ayarsız → her zaman açık + list_sessions sayfalama ✅ (2026-07-11)

- **Karar:** Çapraz-session farkındalığı artık diğer pull araçları gibi **her zaman
  aktif ve hiç ayarı yok**. Önce iki toggle (`Session bağlamı` = `SessionContextEnabled`,
  master; `Her turda ver` = `SessionContextEveryTurn`), ardından son kalan
  `Listelenecek geçmiş session sayısı` (`SessionContextRecentCount`) input'u da
  kaldırıldı. Pushed özet bloğu daima yalnız session'ın **ilk turunda**, sabit **5**
  geçmiş session ile verilir; `list_sessions`/`archive_sessions`/`conversation_search`
  daima sunulur.
- **list_sessions sayfalama:** Araç artık `offset` argümanı alıyor; yanıt toplam
  sayıyı ("Showing X–Y of Z") ve daha varsa bir sonraki sayfanın offset'ini bildiriyor
  → **tüm** sessionlar (aktif veya geçmiş) sayfa sayfa gezilebilir (varsayılan sayfa
  20). (`builtin_sessions.go` + `builtin_sessions_test.go`.)
- **Değişiklikler:** `WSSettings`/`WSSettingsPatch` + API DTO'dan üç alanın hepsi
  silindi, `clampRecent` kaldırıldı (`settings.go`, `workspace_settings.go`).
  `Runtime`'dan tüm `sessionCtx*` durumu + `SetSessionContext`/`SessionContextRecentCount`
  silindi, `DefaultSessionContextRecent` sabiti kaldırıldı (`runtime.go`, `tunables.go`).
  Gate'ler koşulsuz (`toolsetup.go`, `runtime.go` CLI bridge, `agent_context.go`);
  `sessionsContextBlock` artık parametresiz (sabit 5), `chat_turn.go` yalnız
  `freshSession` koşuluyla enjekte ediyor. Frontend: `WorkspacePanel.tsx`'ten
  "Çapraz-session farkındalığı" bölümü **tamamen kaldırıldı** (başlık + açıklama; hiç
  ayar yok), tipler + `WorkspaceView` payload'ı güncellendi. Testler (`settings_test.go`, `sessionctx_test.go`, `sessions_context_test.go`,
  `builtin_sessions_test.go`) yeni imzalara uyarlandı.
- Doğrulama: `go build ./...` + `go test ./internal/{workspace,agent,api,tools}` yeşil;
  frontend `npx tsc --noEmit` yeşil.

## Artifact arşivleme + "Kalıcı ilerleme" oturum sızıntısı düzeltmesi ✅ (2026-07-11)

- **TSK46 (iki parça):**
  1. **Artifact arşivleme** — Artifact'ler artık oturumlar gibi *yumuşak,
     geri-alınabilir* şekilde arşivlenebiliyor (silinmiyor). Model'e
     `Archived bool` alanı (`models_artifact.go`), store'a `SetArtifactArchived`
     (`store_artifact.go`, `SetArtifactGroup` desenini izler), API'ye
     `PUT /api/artifacts/{id}/archive` (`artifacts.go` + `server.go`) eklendi.
     Frontend: `setArtifactArchived` API çağrısı, `Artifact.archived` tipi,
     Artifacts ekranında filtre çubuğuna **Arşiv (N)** görünüm toggle'ı (aktif ⇄
     arşiv listeleri asla karışmaz), toplu **Arşivle/Arşivden çıkar** aksiyonu,
     detay toolbar'ında tekil arşiv butonu + "Arşivlendi" rozeti. Son arşivli
     artifact geri alınınca görünüm otomatik aktif listeye döner.
  2. **Bug** — "Kalıcı ilerleme · N/N" kartı bütün oturumlarda aynı görünüyor ve
     hiç kaybolmuyordu: `progress.json` **çalışma dizinine** göre anahtarlı
     (bkz. `36-KALICI-ILERLEME.md`), aynı proje dizinini paylaşan tüm oturumlar
     tek dosyayı okuyordu. Kayıt zaten **son yazan** oturumun `sessionId`'sini
     tutuyor (`todosink.go` `SaveTodos`); `SessionDetailPanel` artık kartı yalnız
     kaydı son yazan oturumda gösteriyor (sahipsiz legacy kayıt hâlâ görünür).
     `SessionProgressCard`'daki artık ulaşılamaz "foreign" uyarı bloğu kaldırıldı.
- Doğrulama: `go build ./...` + `go vet ./...` + `go test ./internal/...` yeşil;
  frontend `npx tsc -b` + `npm run build` yeşil.

## Canlı süre göstergesi her tool'da sıfırlanıyordu ✅ (2026-07-11)

- **Problem:** Sohbette asistan balonunun altındaki canlı süre (`LiveTimer`), agent'ın
  baştan beri çalışma süresini değil, **son tool'dan/step'ten beri geçen süreyi**
  gösteriyordu. Kök neden: `chatStreamHub.ts`'de `composeGhost()` her `syncGhost`
  çağrısında (her step/delta) `createdAt: Date.now()` ile **yeniden** damgalanıyordu;
  `LiveTimer startUnixSec={m.createdAt}` de bu sürekli-yenilenen zamandan sayıyordu.
  (SES162'de 5 dk görünmesinin sebebi: hung olduğu için hiç step gelmemiş, damga sabit
  kalmıştı — bug'ı maskeliyordu.)
- **Çözüm:** `ghostStartedAt` tur başında **bir kez** damgalanıp (`AgentStart`'ta;
  step'ler önce gelirse `syncGhost` fallback'iyle) tüm step/delta upsert'lerinde sabit
  tutuluyor, `dropGhost`'ta sıfırlanıyor. `composeGhost` artık `createdAt: ghostStartedAt`
  kullanıyor → canlı sayaç agent'ın tüm turunu sayıyor. Tamamlanan tur süresi
  (`workedSec = m.createdAt − tetikleyen user mesajı`) zaten doğruydu, dokunulmadı.
- Doğrulama: `tsc --noEmit` temiz. (Frontend değişikliği → gömülü dist rebuild gerekir.)

## sqz PreToolUse köprü uyumsuzluğu — teşhis + workaround (2026-07-11, kod değişikliği yok)

- **Bulgu:** WS5'te `sqz` (token sıkıştırıcı) HOK1 olarak **enabled** ve CLI
  `--settings`'ine doğru forward ediliyor, ama `sqz gain` → *son 7 günde 0 sıkıştırma*
  (tarihsel 25). Sebep: `sqz hook claude` **yalnız `tool_name == "Bash"`** olan çağrıları
  yeniden yazıyor; TionSwarm shell'i claude-cli'ye **MCP-namespaced** isimle köprülüyor
  (`mcp__tionswarm_interaction__PowerShell` — 463 çağrı — ve `__Bash` — 49), ayrıca
  CLI-native `Bash` gölgeleme korumasıyla deny listesinde. Sonuç: matcher `Bash,PowerShell`
  gerçek shell çağrılarını yakalamıyor → sqz hiç tetiklenmiyor. (Aynı sorun rtk/HOK2'de de
  var; HOK2 zaten disabled.)
- **Workaround (workspace config, TionSwarm source'a dokunmaz):** köprü scripti
  `Progs/sqz/sqz-bridge-hook.ps1` — köprülü `tool_name`'i `Bash`'e normalize edip sqz'ye
  **byte-temiz** (temp dosya + `cmd` redirection; WinPS 5.1 pipe UTF-16 bozuyor) devreder,
  çıktıyı BOM'suz yazar. HOK1 matcher'ı köprülü isimleri de içerecek şekilde genişletildi
  ve komutu bu scripte bağlandı. Test: köprülü PowerShell/Bash → `<cmd> 2>&1 | sqz compress`
  olarak yeniden yazılıyor (native Bash da bozulmadı). **Tüm sqz-hook'lu workspace'lere
  uygulandı:** WS5/HOK1, WS1/HOK2, WS10/HOK4 (WS10'unki bir otonom ajan tarafından çift-escape'li
  bozuk yazılmıştı — `-File \"C:\\...\"` — düzeltildi). WS2/WS8/WS9'da sqz hook yok. **Etkin
  olması için TionSwarm restart gerekir** (hook DB bellek-içi; dosya boot'ta yüklenir).
- **Olası kalıcı çözüm (gelecek kart):** ya sqz'nin köprülü tool-adı desteği, ya da
  TionSwarm'ın bridged-shell çıktısını doğrudan bir token-optimizer'dan geçiren native seam.

## claude-cli startup-hang watchdog ✅ (2026-07-11)

- **Problem:** Spawn edilen bir board-otomasyon turunun `claude.exe -p` subprocess'i
  MCP `initialize` handshake'inde kilitlenip **hiç stdout üretmeden ve çıkmadan**
  7+ dk askıda kaldı (WS5/SES162). `runAttempt`'in bloklu okuma döngüsü yalnız `ctx`
  iptaliyle biterdi; spawn/otonom turun ctx'inde deadline yoksa → `llm_call` yok,
  `error` yok, sohbetteki "yazıyor" göstergesi hiç temizlenmez, seri kuyruk kilitlenir.
- **Çözüm:** `runAttempt` okuma döngüsüne **startup-only watchdog** eklendi
  (`claudecli.go`, `cliStartupTimeout = 90s`). Reader goroutine + timer; **yalnız
  ilk-çıktıya-kadar** olan süre korunur — ilk stdout satırı gelince timer durur,
  sonraki uzun sessizlikler (meşru sync `run_subagent`, dakikalarca sessiz) CLI'nin
  `MCP_TOOL_TIMEOUT`'una + dış ctx'e bırakılır (yanlış-pozitif kill yok). Çıktısız
  hang'de subprocess öldürülür → `cmd.Wait` döner → **retryable** net hata döner →
  self-healing bir kez retry eder, ghost temizlenir, kuyruk açılır.
- Doğrulama: `go build ./...` + `go vet ./internal/providers` + `go test
  ./internal/providers -short` yeşil.

## `/compact` + `/handoff` claude-home seam düzeltmesi ✅ (2026-07-11)

- **Problem:** Manuel `/compact` (`api.compactSession`) ve `/handoff`
  (`Runtime.HandoffSession` → `conversation.BuildHandoff`) provider'ı
  `providers.Get` ile alıp **doğrudan** `provider.Complete` çağırıyordu —
  `guardedComplete` funnel'ını (dolayısıyla `SetConfigDir` seam'ini) atlayarak.
  claude-cli sağlayıcıda provider default `configDir` = **global**
  `~/.tionswarm/claude-home`; oranın credential'ı boşsa `authentication_failed`
  dönüyordu, workspace'in kendi `claude-home`'u login olsa bile (WS5/SES130'da
  görüldü).
- **Çözüm:** Ortak seam tek yere alındı — `Runtime.PinClaudeHome(provider)`
  (`budget.go`), claude-cli provider'ının config evini bu workspace'in
  `claude-home`'una sabitler (diğerlerinde no-op). `guardedComplete` artık bunu
  çağırıyor; iki out-of-loop yol (`compactSession`, `HandoffSession`) da ham
  `Complete` öncesi çağırıyor. Diğer yardımcı çağrılar (title/summary/reflect)
  zaten `guardedComplete`'ten geçtiği için etkilenmemişti.
- Doğrulama: `go build ./...` + `go vet` + `go test ./internal/agent` yeşil.

## an external CLI agent `/chronicle` referans dokümanı ✅ (2026-07-11)

- **TSK30 (doküman-only):** an external CLI agent'nin `/chronicle` oturum-içgörü
  ailesini (tips / improve / standup / cost-tips / search / reindex) + yerel
  SQLite session store mekaniğini açıklayan ve TionSwarm muadilleriyle
  (`session.jsonl`+`debug.jsonl`, ders döngüsü `lessons.jsonl`,
  `conversation_search`, Tasarruf Merkezi) kıyaslayan `_Docs/64-GITHUB-COPILOT-CHRONICLE.md`
  eklendi. Boşluk tespiti: proaktif `tips`/`standup` içgörü üreteci TionSwarm'da yok
  (gelecek kart tohumları dokümanda). Kaynaklar dipnotlandı (GitHub Docs + changelog).
  `00-GENEL-BAKIS.md` dizinine 58 + 59 satırları eklendi. Kod değişikliği yok.

## Sohbet kuyruğu + çoklu-ekran senkronizasyonu (event-sourcing cutover) ✅ (2026-07-11)

- **Amaç:** Sohbeti "owner window kendi SSE'sini stream'ler + non-owner'lar
  inflight snapshot + polling ile kurtarır" ikiliğinden çıkarıp **sunucu-otoriter,
  tek total-order'lı, cursor tabanlı event akışı** modeline taşımak. Her pencere
  sadece abone; "sahip pencere" öldü. Tam tasarım + faz planı: `_Docs/58-QUEUE-SENKRON.md`.
- **Faz 1 — SessionHub + cursor SSE:** `internal/sessionhub` (per-session monoton
  `seq` + ring buffer + epoch + gap-aware `Replay` + `Commit` boundary);
  `GET /api/sessions/{id}/stream?since=&epoch=` (hello/reset/hub, `Last-Event-ID`
  uyumlu). İnteraktif tur tüm dayanıklı olayları eş-sıralı hub'a yayınlıyor;
  autonomous turlar `bridgeBusToHub` ile bus→hub (steps + turn_done). **Replay
  optimizasyonu:** fresh abonelik (`since<=0`) yalnız `committed`'dan sonraki
  in-flight tail'i replay eder → tamamlanmış turlar tekrar oynatılmaz.
- **Faz 2 — Interaction CAS (resolve-once):** `ask_user`/`permission`/`plan` tek
  `pendingInteraction` primitive'ine; `interaction_open`/`interaction_resolved`
  tüm pencerelere yayılır; cevap `POST .../interactions/{iid}/answer` → compare-and-
  swap, ilk yazan kazanır (200), diğeri 409 + kart kapanır. Native + CLI yolu.
- **Faz 3 — Durable send-queue:** `handleChatStream` → ince wrapper + `runChatTurn`
  (HTTP'siz, worker'ın çağırdığı çekirdek). `db/inbox.go` (durable `inbox.json`) +
  `api/inbox.go`: `POST /sessions/{id}/messages` (enqueue, `clientMsgId` idempotency),
  `DELETE .../queue/{msgId}` (cancel), `POST .../control` (session-scoped stop/steer),
  `queue_update` broadcast, boot `recoverInboxes`. Frontend: send = enqueue
  (optimistic yok — kuyruktaysa tray, çalışınca chat balonu); client-side flush
  kaldırıldı, backend serialize ediyor.
- **Faz 4 — Presence:** `hub.SubscriberCount` → efemer `presence` yayını her abone
  giriş/çıkışında → ChatView "Bu oturum N pencerede açık" rozeti.
- **Frontend cutover:** `api/sessionStream.ts` (cursor+epoch+gap-detect+reconnect),
  `features/chat/chatStreamHub.ts` (aktif session'ın tek otoriter render'ı);
  `useChatStream` hub aboneliği; inflight polling + bus-ghost + recoverInflight
  kaldırıldı.
- **Doğrulama:** `go build`/`go vet` yeşil, **815 test / 35 paket** (3 interaction
  testi yeni CAS'a göre güncellendi); `tsc --noEmit` + `vite build` temiz; runtime
  smoke (headless boot + hub stream hello/presence + queue/control endpoint'leri).
- **Bilinen sınır:** autonomous turlar reply mesajını hub'a yayınlamıyor
  (tamamlanma `bridgeBusToHub`'ın turn_done'u + listMessages reload ile geliyor).

## Mobil: interaktif kartlar taşınca kaydırılabilir ✅ (2026-07-10)

- **Sorun:** Telefon ekranında `ask_user` (çok-soru), plan onayı, izin ve görev
  listesi kartları viewport'u aşınca üst kısımları görünmüyordu — dikey kaydırma yoktu.
- **Çözüm (yalnız frontend, mantık değişmedi):** ortak `ScrollableCard`
  (`shared/components/ScrollableCard.tsx`) sarmalayıcısı — viewport-oransal
  `maxH` (varsayılan `max-h-[55vh]`) + `overflow-y-auto`. Büyüyen içerik bölgesi bu
  sarmalayıcıya alındı; aksiyon butonları dışarıda bırakıldı → her zaman görünür.
  - Kullananlar: `AskPrompt` (SingleAsk soru/seçenek + MultiAsk soru listesi, 55vh),
    `TodoCard` (50vh), `TodoPanel` (45vh), `PermissionPrompt` (komut `<pre>`, 40vh),
    `PlanPrompt` (plan markdown'ı, 55vh).

## Bash öncelikli, PowerShell gerektiğinde ✅ (2026-07-10, TSK43)

- **Sorun:** Windows'ta ajan koşulsuz PowerShell'e yönlendiriliyordu — üç katman
  (ortam bloğu, Bash aracı açıklaması, claude-cli köprüsü). bash.exe zaten kuruluysa
  `Bash` aracı native döngüde kayıtlıydı; sorun araç eksikliği değil **önceliklendirmeydi**.
- **Çözüm (davranış/prompt-policy refactor, execution core değişmedi):**
  - `EnvironmentContextBlock()` artık `tools.ShellToolNames()`'in ilk girdisine göre
    tercih edilen shell'i ilan eder — Windows'ta bash.exe varsa `shell="Bash"` +
    "prefer Bash; use PowerShell only for Windows-native tasks", bash.exe yoksa
    PowerShell'e düşer (guard'a bağlı, sessiz yutma yok).
  - `ShellTool.Def()` "PREFERRED shell — reach for it first, including on Windows";
    `PowerShellTool.Def()` "use ONLY when the Bash tool cannot do the job".
  - claude-cli köprüsü: `interactionToolSpecs()` Windows'ta `ShellToolNames()`'e göre
    hem Bash (varsa) hem PowerShell'i ilan eder; `NewShellRunner()` closure'ı artık
    `toolName`'e göre dispatch eder (Bash→POSIX, PowerShell→PS; Windows'ta bash yoksa
    PowerShell'e düşer); `callShell()` dispatch'e tool adını (`bare`) geçirir.
  - `climcp.go` shadowing yorumu ve `default-instructions.md` shell cümlesi
    Bash-öncelikliye güncellendi. Native döngü zaten iki aracı da kaydediyordu.
- **Bash mevcudiyeti guard'ı:** Tüm Bash-öncelikli ilan `resolvePOSIXShell()`/`Available()`
  üzerinden `ShellToolNames()`'e bağlı — bash.exe olmayan makinede "Bash" iddia edilmez.
- **Not (kullanıcı CLAUDE.md çelişkisi):** Bu makinenin `CLAUDE.md`'si Windows/PowerShell
  tercihi belirtir (kullanıcının; dokunulmadı). Bu değişiklik uygulamanın **varsayılan
  ajan yönlendirmesini** Bash-öncelikliye çevirir; kullanıcı workspace talimatı/CLAUDE.md
  ile bunu ezebilir.
- **Doğrulama:** `go build ./...`, `go vet ./...`, `go test ./internal/...` — hepsi yeşil
  (agent/api/tools dahil; frontend'e dokunulmadı). Detay `_Docs\51`, `_Docs\17`.

## Artifact detayında tekil grup düzenleme ✅ (2026-07-10, TSK44)

- **Sorun:** Bir artifact açıkken grubunu değiştirmenin yolu yoktu — yalnız çoklu-seçim
  (bulk) grup atama ve sürükle-bırak vardı. Tek bir artifact'i gruplamak için kullanıcı
  çoklu-seçime girmek veya DnD yapmak zorundaydı.
- **Çözüm:** `ArtifactsPanel` detay editörüne doğrudan grup input'u eklendi
  (`data-testid="artifact-edit-group-input"`, mevcut grup adları `artifacts-group-names`
  datalist'iyle önerilir; boş = grupsuz). `Draft`'a `group` alanı; `createNew`/`startEdit`
  onu doldurur; `dirty` grup farkını da içerir.
- **Kayıt yolu:** `save` önce içerik/meta patch'ini (`api.updateArtifact`), grup değiştiyse
  ardından `api.setArtifactGroup(id, group)` çağırır — backend `updateArtifact` handler'ı
  `group`'u yok saydığı için mevcut `PUT /api/artifacts/{id}/group` endpoint'i reuse edildi.
  **Backend değişikliği yok.** Detay `_Docs\45`.
- **Doğrulama:** `go build/vet`, `go test ./internal/...`, `npx tsc -b`, `npm run build` — hepsi yeşil.

## Spawn süresi ayarlanabilir + otomatik-devam deadline koruması ✅ (2026-07-10)

- **Sorun:** Otomatik-devam turu (`maybeAutoContinue`), spawn work-turn'ünün context'ini
  paylaşıyordu. İlk tur 10 dk'lık sabit `spawnTimeout`'u tükettiğinde devam turu **süresi
  dolmuş** context'te 80 ms'de patlayıp kullanıcıya ham `context deadline exceeded`
  gösteriyordu (SES147).
- **Fix 1 — ayarlanabilir süre:** Spawn work-turn deadline'ı artık Ayarlar'dan
  (`SpawnTimeoutMin`, **default 20 dk**). `Tunables.SpawnTimeout()` → `runSpawn` kullanır;
  applySettings ile canlı uygulanır. Frontend: AppToolsPanel "Spawn süresi (dk)" alanı.
  Diğer ayrık yüzeyler (worker/inbox/hook firing) sabit `spawnTimeout`'ta kalır.
- **Fix 2 — deadline guard:** `maybeAutoContinue`, her iterasyonda `ctx.Err()` kontrol eder;
  deadline dolmuşsa devam turunu atlar ve ham Go hatası yerine anlamlı "⏱️ Süre doldu…"
  mesajı yazar (`context.WithoutCancel` ile kalıcılaştırır, `auto_continue_error` etiketler).

## Ek sabit değerler Ayarlar'a taşındı ✅ (2026-07-10)

Daha önce kod-sabiti olan 4 değer settings-driven yapıldı (applySettings ile canlı,
0 → yerleşik default). Ayarlar → App/Tools panelinde:

- **Zamanlama süresi** (`ScheduleTimeoutMin`, default 30 dk) — `scheduler.fire/fireWake`
  artık `s.rt.tun.ScheduleTimeout()` kullanır; spawn süresiyle aynı desen.
- **Kabuk varsayılan/maks. süre** (`ShellDefaultTimeoutSec`/`ShellMaxTimeoutSec`, 30/120 sn)
  — `tools.SetShellTimeouts`; per-call `timeout_sec` yine geçersiz kılar, maks. ile kırpılır.
- **Araç çıktı sınırı** (`MaxToolOutputKB`, default 100 KB) — `tools.SetMaxToolOutputBytes`;
  MCP dahil tüm araç çıktısının backstop kesme sınırı.
- **Maks. bağlam token** default `12000 → 800000`.

Not: `tools` paketinde bu değerler artık process-global `var` + setter (const değil).

## Executions ekranı kaldırıldı — birleşik Sohbet transkripti ✅ (2026-07-10, TSK45)

- **Karar:** Ayrı "Aktivite" (Executions) ekranı kaldırıldı. Executions ayrı bir veri
  kaynağı değildi — `GET /api/executions` ile `GET /api/sessions` **aynı** `DB.ListSessions`
  çağrısına dayanıyor; Executions yalnızca her satıra `running` + `lastStatus` ekliyordu.
  Artık **Sohbet** ekranı her oturum türü (chat/task/flow/schedule/spawned) için tek
  birleşik transkript görünümü.
- **Frontend — birleşik sidebar:** `SessionsSidebar` artık tüm oturum türlerini listeler:
  kind rozeti + ikon, kind filtre sekmeleri (`Tümü/Sohbet/Görev/Akış/Spawn/Zamanlama`,
  `localStorage`'da hatırlanır), canlı nabız noktası ve task/flow için pass/fail `StatusPill`.
  `running`/`lastStatus` bilgisi yeni `useExecutionRuntime` hook'undan (`/api/executions`
  poll + `executions` SSE sinyali) `runtimeById` haritası olarak beslenir.
- **Salt-okunur composer:** `ChatView`'e `readOnly` prop'u eklendi; task/flow/schedule
  transkriptlerinde composer + ask/todo/pending/wake yığını **hiç render edilmez**, yerine
  "Bu oturum salt-okunurdur" bandı gösterilir (`isWritableSessionKind` → `chat`/`spawned`
  yazılabilir). Böylece bağlam/debug/oturum-bilgisi panelleri **tüm** oturum türleri için
  açılabilir hâle geldi (önceden yalnız `view === 'chat'` altındaydı).
- **Toplu tablo:** `SessionsOverview` (`features/executions/`'tan `features/sessions/`'a
  taşındı) sidebar'daki "Oturumlar" (`Table2`) butonundan overlay olarak açılır; satıra
  tıkla → o oturumu Sohbet transkriptinde aç.
- **Silinen / taşınan dosyalar:** `features/executions/` klasörü tamamen kaldırıldı
  (`ExecutionsPanel.tsx`, `useLiveTranscript.ts` silindi; `executionsShared.tsx` →
  `features/sessions/sessionKindMeta.tsx`, `SessionsOverview.tsx` → `features/sessions/`).
  `NavRail`/`viewRegistry`/`url.ts`/`eventViews`/`useActivity`/`useDeepLinks`/
  `useAppNavigation`/`useSessionsController` içindeki `executions` referansları
  temizlendi; eski `#executions/<sid>` deep-link'i chat oturumuna düşer. `AgentActivityPanel`
  "yürütmeyi aç" bağlantısı artık chat transkriptine yönlenir.
- **Backend değişmedi:** `GET /api/executions` (`internal/api/executions.go`) korundu —
  `AgentActivityPanel` ve `useExecutionRuntime` onu tüketiyor; yalnız UI ekranı kaldırıldı.
- **Doğrulama:** `go build`/`go vet`/`go test ./internal/...` tümü yeşil; `npx tsc -b`
  temiz; `npm run build` başarılı.

## Spawn session'ı "çalışıyor" göstermiyordu 🐛 (2026-07-10)

- **Sorun:** `spawn` (veya `spawn_worker`) ile bir session oluşturulunca, sohbet ekranında o
  session "devam ediyor" (thinking) olarak gözükmüyordu; composer/input ajan çalışmıyormuş gibi
  görünüyordu.
- **Kök neden:** Spawn/worker turları runtime'da **detached** çalışır (`runSpawn`/`runWorker` →
  `trackSession` → `invokeTraced`); api sunucusunun `chatRuns` (`s.runs`) registry'sine **hiç
  kaydolmaz**. (a) Başlangıçta thinking göstergesini kaldıracak bir sinyal yoktu (scheduler'ın
  wake yolu `chat` phase=start olayı yayar, spawn yaymıyordu). (b) `handleActiveSessions`
  (`/api/active-sessions`, reload sonrası thinking restore kaynağı) yalnız `s.runs`'ı dönüyordu,
  `wsp.Runtime.ActiveSessionIDs()`'i kaçırıyordu → yenileme sonrası da restore edilemiyordu.
- **Çözüm:** (1) Yeni `Runtime.emitTurnStart(sessionID, title)` — wake desenini yansıtan `chat`
  phase=start olayı (frontend `markPending` → thinking + composer busy; `chat` olayları toast
  üretmez). `runSpawn` ve `runWorker` `trackSession`'dan hemen sonra çağırır; tamamlanma zaten
  `"spawned"`/`"worker"` olayıyla `clearPending` yapıyor. Flow/inbox/autocontinue completion'ları
  pending temizlemediği için `trackSession` **global** hook'lanmadı (cerrahi = yalnız spawn+worker).
  (2) `handleActiveSessions` artık `s.runs` **∪** `wsp.Runtime.ActiveSessionIDs()` (dedup) döner →
  reload sonrası spawn/worker/schedule/flow turları da restore edilir. Build + 266 test yeşil.

## Tur-ortası yenileme — eski araç adımları kurtarma yarışı 🐛 (2026-07-10)

- **Sorun:** Executions/Sohbet ekranında bir ajan turu akarken sayfa yenilenirse ajanın
  **reload öncesi** araç kullanımları listeden kayboluyor, yalnız reload **sonrası** yeni araç
  adımları büyümeye devam ediyordu.
- **Kök neden (frontend yarış koşulu):** Kurtarma mekanizması (`recoverInflightSnapshot`,
  `inflight.json` snapshot'ından adımları ghost bubble'a tohumlar) canlı `session_step` bus
  akışıyla yarışıyordu. (a) **Executions** (`useLiveTranscript.load`): snapshot tohumlaması
  `if (!autoLiveRef.has(sid))` ile korunuyordu; `listMessages` (async) çözülmeden önce bir bus
  adımı gelirse `foldAutoStep` boş entry (`steps:[]`) oluşturur → gate `true` olur → recover
  **hiç çağrılmaz** → tüm eski adımlar kaybolur. (b) **Chat** (`recoverInflightSnapshot`):
  mevcut entry'yi sorgusuz **overwrite** ediyordu → yarış anındaki adımlar kaybolur.
- **Çözüm:** (1) `recoverInflightSnapshot` artık **merge** ediyor — canlı entry zaten varsa
  snapshot adımlarını **başa ekler** (reload öncesi adımlar ⊕ reload sonrası bus adımları;
  bus geçmişi tekrar yollamadığı için çakışma yok), bubble id/text korunur. (2)
  `useLiveTranscript`'e `foldAutoStep`'ten bağımsız `seededRef` eklendi → snapshot seçim başına
  **tam bir kez** merge edilir (bus adımı yarışı kazansa bile). Seçim değişiminde
  `seededRef.clear()`. Backend zaten 600ms throttle ile `kept` (tool adımları dahil)
  snapshot'lıyor (`chat_stream.go` `snapshot()`), değişmedi. Frontend `tsc --noEmit` temiz.

## Sohbet & ekran geçişlerinde loading göstergesi ⏳ (2026-07-10, TSK41)

- **Sorun:** Açılışta bir an "Yeni sohbete başla" boş-durumu, sohbet değişince eski
  transkriptin ekranda kalması, panellerde ilk paint'te "boş" görünmesi.
- **Çözüm:** `useSessionsController`'a `bootstrapping` + `messagesLoading` bayrakları,
  transkript yüklemesine `msgSeqRef` in-flight guard'ı (hızlı A→B→A geçişinde geç gelen
  cevap ezmiyor) ve sessiz `.catch(() => {})` yerine `setError`. Yeni primitive'ler:
  `shared/components/Skeleton.tsx` (`Skeleton`/`LoadingState`), `shared/hooks/useDelayedFlag.ts`
  (≈140ms, iskelet titremesini önler), `features/chat/ChatSkeleton.tsx`. `useAsync`'te
  `loading` artık `enabled` ile başlıyor; `TaskBoard`/`Schedules`/`Automations`/`FlowsPanel`/
  `MarketPanel`/`ArtifactsPanel` bu desene taşındı. Lazy panel `Suspense` fallback'leri ve
  `SessionsSidebar` iskelete geçti. Detay → `07-CHAT-UX.md` "Loading & iskelet durumları".
- **Doğrulama:** `go build ./...`, `go vet ./...`, `go test ./internal/...`, `npx tsc -b`,
  `npm run build` temiz. Canlı UI doğrulaması review aşamasında.

## Debug Sankey — köprü ön ekleri temizlendi 🧹 (2026-07-10)

- **İstek:** "Debug / Gözlemlenebilirlik" popup'ındaki Araç yürütme akışı (Sankey)
  düğümlerinde araç adlarının başındaki `mcp__tionswarm_interaction__` /
  `mcp__tionswarm_extended__` ön ekleri okunurluğu bozuyordu.
- **Çözüm:** `flowVizData.ts`'e `toolDisplayName(name)` yardımcısı eklendi — yalnız bu iki
  **iç claude-cli köprü** namespace'ini soyar (araçlar zaten TionSwarm'ın kendi köprülü
  built-in'leri; ön ek bir transport detayı). `buildToolSankey` tool etiketini bundan geçirir;
  **"En yavaş araçlar" listesi** de (`SessionDebugCard.tsx` `topTools`) aynı yardımcıyı kullanır.
  Gerçek harici MCP sunucuları (`mcp__github__…`) ön eklerini korur (hangi sunucunun
  çağırdığını ayırt eder). Frontend `tsc --noEmit` temiz.

## Default skill seed'i frontmatter-farkındalı ✅ (2026-07-10)

**Sorun (global tier denetiminde bulundu):** `EnsureDefaults` sürüm-farkındalıydı ama
bütün-dosya hash'iyle çalışıyordu. Uygulama görünürlük frontmatter'ını yerinde yazdığı
için (`access`/`group`/`auto_summary`/`name_only`/`summary_only` — ör. Skills ekranında
tek bir paylaşım/tier değişikliği) dosya hem gömülüden hem son-sevk hash'inden ayrışıyor
→ "user edit" sayılıp **sonsuza dek donuyordu**; sevk edilen gövde güncellemeleri bir
daha ulaşmıyordu (canlı örnek: 12/12 global default donmuştu, `tionswarm-settings`
kaldırılmış compaction ayarlarını belgelemeye devam ediyordu).

**Çözüm:** SKILL.md dosyaları için **gövde-ayrı muhasebe**:

- `.shipped-versions.json` v2: `{files: {...}, bodies: {...}}` — `files` bütün-dosya
  (eski semantik, frontmatter değişikliklerini de sevk eden tam-tazeleme yolu önce
  denenir), `bodies` yalnız SKILL.md gövdesi (frontmatter hariç, `splitFrontmatter`).
  Legacy düz map `files`'a katlanarak okunur (`loadShippedManifest`).
- Yeni kural: bütün-dosya eşleşmezse gövde karşılaştırılır — gövde gömülüyle aynıysa
  yalnız kayda geçirilir (unfreeze bootstrap); gövde son-sevk gövdesiyle aynıysa
  **frontmatter verbatim korunarak** yeni gövde altına yazılır (`rebuildSkillFile`);
  ikisi de değilse gerçek kullanıcı düzenlemesi → dokunulmaz.
- Dosyalar: `internal/skills/defaults.go` (algoritma), yeni
  `internal/skills/defaults_manifest.go` (manifest v2 + `skillBody`/`rebuildSkillFile`).
- Testler: yeni `defaults_test.go` — gövde-tazeleme (kullanıcı frontmatter'ı altında),
  kullanıcı gövde düzenlemesi korunur, yalnız-frontmatter değişikliğinde unfreeze
  kaydı, legacy düz manifest yükleme, rebuild round-trip; mevcut `store_test.go`
  seed/pristine testleri yeni API'ye uyarlandı. `go test ./internal/...` 811 yeşil.
- Bayat `tionswarm-doc-improver` cümlesi ("EnsureDefaults never overwrites") repo
  default'unda + global kopyada düzeltildi.

**Etki:** Mevcut kurulumda gövdeler şu an gömülüyle eşit (elle senkronlandı) →
ilk çalıştırmada `bodies` kayıtları kendini tohumlar, sonraki her sevk gövdeyi
kullanıcı frontmatter'ına dokunmadan tazeler. Yeni davranış rebuild + restart ister.

## Doküman bakım turu ✅ (2026-07-10)

Git geçmişiyle (özellikle compactor kaldırma + memory kaldırma + debug-viz eklemeleri)
dokümanlar senkronlandı:

- **`38-SESSION-DEBUG.md`:** eksik "İş akışı görselleştirmeleri" bölümü eklendi
  (`viz/`: ToolSankey · ConcurrencyTimeline · PromptCacheEvents · SelfHealingEvents ·
  HookActivity); yeni olay tipleri (`repair`/`guardrail`/`lesson`/`epoch`) +
  `HookID`/`Calls` alanları; debug'ın ayrı `SessionDebugModal`'a taşındığı işlendi;
  kaldırılan dream-cycle reflektörü işaretlendi; "Sırada" listesi tazelendi.
- **`00-GENEL-BAKIS.md`:** dizine eksik 5 kayıt eklendi (50, 53-SOURCE-TEMPLATES,
  55, 57, MALIYET-DUSURME-PLANI); 17 açıklaması harici rtk/sqz devrini yansıtıyor;
  53 numara çakışması nota bağlandı; bayat "Sıradaki (07-03)" satırı 07-10 açık
  kalemleriyle yenilendi (P5 memory tool + 55 UI boşlukları, sqz byte-ölçümü,
  batching n≥5 kıyası, code-exec Faz 4/5, araç backlog'u).
- **Silinmiş compactor referansları:** `55-API-NATIVE` (P1 hedef listesi + uygulama
  notu) ve `50-CACHE-PARITE` (Sistem A/B "tamamlayıcı" cümlesi) kaldırma notuyla
  düzeltildi.
- **Kaldırılmış core-memory kalıntıları:** `11-INTERACTION-MCP` (CLI köprüsü bölümü)
  ve `41-ARAC-BOSLUKLARI` (madde 7 son cümle) tarihsel işaretlendi.
- **Skill `tionswarm-project`:** `compact/`+`compactor`+`reflector`+memory araç
  referansları temizlendi (yerine `lessons`); token-optimizasyon maddesi "built-in
  sıkıştırma kaldırıldı → harici rtk/sqz" olarak yeniden yazıldı.
- **Default skill `tionswarm-autonomous-ops`:** Guardrails bölümündeki bayat
  "per-agent budgets (`daily_call_limit`/`daily_token_limit`)" maddesi (limitler
  2026-07-01'de kaldırılmıştı) global otonomi-pause freni + kullanım ölçümü
  (`guardedComplete`) olarak düzeltildi. Not: `~/.tionswarm/skills`'teki global
  kopyalar `access/group/summary_only` frontmatter eklendiği için EnsureDefaults
  tarafından "user edit" sayılıp bir daha tazelenmiyor — düzeltme global kopyaya
  elle de uygulandı.

## Debug: Hook / token-optimizer aktivite göstergesi ✅ (2026-07-10)

"rtk/sqz tasarrufu Bütçe/Debug'da gözükür mü?" sorusunun ürün cevabı. **Gerçek tasarruf
(byte) gösterilemez** çünkü rtk/sqz **PreToolUse** hook'u — komutu yeniden yazıyorlar,
araç zaten sıkışmış çıktı üretiyor; TionSwarm sıkışmamış baseline'ı hiç görmüyor → delta
yok (built-in sıkıştırma muhasebesi de 2026-07-10'da kaldırıldı). Bunun yerine **aktivite
göstergesi** eklendi:

- **Backend:** `db.DebugEvent`'e `HookID` alanı; `hooks.go`'daki 6 hook emit noktası
  (pre/post/lifecycle · başarı+hata) artık ateşleyen hook'un id'sini yazıyor → `type=hook`
  debug olayları hook-başına atfedilebilir. Test: `hookdebug_test.go` (fire → journal → read).
- **Frontend:** yeni `sessions/viz/HookActivity.tsx` — `type=hook` olaylarını `hookId`'ye göre
  gruplayıp `\brtk\b`/`\bsqz\b` (backend probe aynası) ile rtk/sqz/other sınıflar; hook başına
  ateşleme sayısı + araç dağılımı (`tool×N`) + hata sayısı gösterir. `SessionFlowViz`'e
  "Hook / token-optimizer aktivitesi" bölümü olarak bağlandı (hooks listesini `/api/hooks`'tan
  çekip atıf için geçiriyor).
- **Dürüstlük:** UI açıkça "byte tasarrufu değil, aktivite" der; ayrıca **yalnız native turlar
  sayılır** (claude-cli turlarında hook'lar CLI içinde çalışır, journal'a düşmez) uyarısını taşır.

Canlı doğrulandı: yeni binary'de SES130 debug görünümünde bölüm render oldu, claude-cli
oturumu olduğu için dürüst empty-state gösterdi. `go test ./internal/agent ./internal/db
./internal/api` yeşil (316), `npx tsc -b` temiz. Not: gerçek byte tasarrufu istenirse yol,
sqz'yi PostToolUse output-rewrite moduna alıp `runPostToolHooks`'ta `len(önce)−len(sonra)`
ölçmek — ayrı iş.

## Token-optimizer UI callout + hook matcher düzeltmesi + API GET no-store ✅ (2026-07-10)

Token-optimizer capability'sinin (rtk/sqz) devamı — UI tarafı + bir gerçek matcher bug'ı
+ canlı testte yakalanan bir cache bug'ı.

- **UI callout (`ExternalToolsPanel.tsx`):** "Token / bağlam optimizasyonu" grubunun altına
  codebase-memory callout'unun eşdeğeri "⚡ Token optimizasyonu entegrasyonu" bloğu eklendi —
  rtk/sqz hook olarak bağlıysa ajanın promptuna bilgi bloğu enjekte edildiğini, çıktının otomatik
  kısaltıldığını (kayıp değil) ve matcher'ın `Bash,PowerShell` olması gerektiğini açıklar. **Matcher
  onarımı:** bağlı bir token hook'unun matcher'ı PowerShell'i kapsamıyorsa (`coversPowerShell`,
  backend `hookMatches` aynası: boş/`*`/`PowerShell` → kapsar) ⚠ rozeti + tek-tık **"Matcher'ı
  düzelt"** butonu (`fixMatcher` → mevcut matcher'a `PowerShell`'i merge eder, diğer alanları korur,
  `updateHook` PUT). `data-testid="fix-matcher"`.
- **Gerçek matcher bug'ı (WS5 + WS1):** rtk/sqz hook'ları `matcher=Bash` ile kayıtlıydı; ajanlar
  Windows'ta `PowerShell` aracını kullandığı için hook'lar **hiç ateşlenmiyordu**. Canlı UI butonuyla
  düzeltildi → ikisi de `Bash,PowerShell`.
- **API GET no-store (`api/client.ts`):** canlı testte yakalandı — `req()` `fetch`'i default cache
  ile çağırıyordu, tarayıcı `/api/hooks` gibi GET'leri **stale servis edebiliyordu** (panel bir
  değişiklikten sonra eski veriyi gösteriyordu). `fetch(path, { cache: 'no-store', … })` eklendi;
  çağıran `init.cache` ile hâlâ override edebilir. Tüm GET domain modülleri için canlı-veri tazeliği.

Doğrulama: `npx tsc -b` temiz; Playwright ile WS5'te callout + ⚠ uyarı + "Matcher'ı düzelt" butonu
uçtan uca test edildi (tık → WS5 hook `Bash,PowerShell` → uyarı sıfırlandı). Not: canlı backend eski
binary — token-optimizer **prompt bloğunun** enjekte olması için backend yeniden derlenip başlatılmalı.

## todo_write kompakt `set` formu: durum güncellemesi tüm listeyi yeniden göndermiyor ✅ (2026-07-10)

AlgoBench dökümünde todo_write çağrıları koşu başına ~1.2k output-char tutuyordu — her
güncellemede tüm liste yeniden gönderiliyordu. Artık iki form var (tam olarak biri):

- **`todos`** — tam liste (oluşturma / metin değişikliği; eskisi gibi replace).
- **`set`** — `{"set":{"1":"completed","2":"in_progress"}}`: 1-tabanlı indeks → status.
  Sunucu önceki listeyi **todo sink'ten** yükler (`TodoSink.LoadTodos`, progress
  dosyası), birleştirir, geri persist eder. Sink yoksa veya henüz liste yoksa **yüksek
  sesle hata** verir ("send the full todos array"). Tipik güncelleme ~400 → ~40 char.
- **Trace/UI:** `set` çağrısının input'unda liste olmadığından birleşik tam liste tool
  SONUCUNDA JSON olarak döner; `todoStepItems` (trace.go + toolloop.go) input boşsa
  output'tan parse edip StepTodo kartına terfi ettirir — CLI ve native yol aynı.
- **Model tarafı:** dinamik prompt'taki "Active todo list" bloğu artık **numaralı**
  (`1. [x] …`) ve `set` formunu öğretiyor; araç açıklaması da güncellendi.
- Test: builtin_todo_test (merge + alan koruma + 6 hata senaryosu), todos_test numaralı
  format. `go build ./...` + `go test ./internal/...` yeşil (799 test).

## AlgoBench ~1.7× output farkı ayrıştırıldı: %81'i Write payload'ı, kaynak ajan personası ✅ (2026-07-10)

v6/v7 transkriptlerinde TS vs CA output-token dökümü (dedup usage; karakter bazında kategori):

- **Fark narasyon değil, üretilen dosya içeriği.** TS v7 38.0k out-char / CA v7 22.2k.
  Delta 15.8k char'ın dağılımı: **Write %81** (+12.8k), text +1.1k, `todo_write` +1.2k
  (CA hiç todo kullanmadı), shell +0.6k. Her iki tarafta da out-char'ın %94-96'sı tool_use.
- **TS aynı görevde ~%64 daha uzun kod yazıyor** (iki koşuda da tutarlı → sistematik, sampling değil):
  6 algoritma dosyası 9.9k vs 5.8k · testler 7.7k vs 4.6k (30 test vs 22) · Go portu 4 dosyaya
  bölünmüş 3.8k vs tek main.go 2.4k · compute.py 2.4k vs 1.3k (bağımsız çapraz-doğrulama kodu) ·
  REPORT.md 4.6k vs 2.6k.
- **Kök neden: benchmark ajanı AGT10'un personası** ("focused implementation engineer …
  running code to verify") + TR yanıt tercihi. Görev metinleri birebir aynı (diff'lendi);
  TS statik prefix'i (5k char, epoch sidecar'dan okundu) verbosity talimatı içermiyor.
  Runtime'ın kendi output ek yükü küçük: todo_write ~1.2k char (~%3).
- **Sonuç:** 1.7× fark büyük ölçüde persona-kaynaklı titizlik (daha çok test, cross-check,
  dosya bölme) — kalite/maliyet dengesi, runtime bug'ı değil. Adil kıyas için TS koşusu
  boş-persona ajanla (AGT9 CoderEmpty benzeri, thinking-off) tekrarlanmalı.
- **Adil tekrar koşu (aynı gece, AGT11 "BenchBare": boş soul/identity + thinking-off,
  v7 ile aynı sunucu binary'si, taze `bench-tionswarm-bare`):** toplam 16.4k out-tok
  (0 thinking; batching var: [5,3,2,2,2,2,2]). Persona hipotezi **kısmen** doğrulandı:
  REPORT.md 4.6k→2.8k (CA seviyesi), compute.py 2.4k→2.1k, algoritma dosyaları kısaldı
  (örn. binary_search 1601→898) — ama **30 test yine yazıldı** ve Go yine dosyalara
  bölündü → bunlar persona değil. Koşu ortada bir kez kesildi (non-stream istemci koptu;
  **inflight paritesi çalıştı**, 32 adım kayıpsız persist edildi) ve devam turu ~3-4k
  char yeniden-bağlam maliyeti ekledi (8 Read + ekstra shell). Düzeltilmiş tahmin
  ~14.5-15k tok → CA'nın hâlâ ~1.4×'i. **Net:** persona ~%15-20'lik kısmı açıklıyor;
  kalan fark TS runtime append'i / model davranışı / örneklem gürültüsü — kesinleşme
  n≥5 istatistiksel seriye kaldı.

## Token-optimizer capability probe: rtk/sqz kuruluysa prompta bilgi bloğu ✅ (2026-07-10)

codebase-memory capability'siyle aynı desen: workspace'te `rtk`/`sqz` **hook ile bağlı**
ise ajanın statik prompt prefix'ine "# Token optimization active" bloğu enjekte edilir.

- **Yeni:** `internal/agent/capabilities_tokenopt.go` — `tokenOptimizerCapability`
  (registry'ye eklendi). `detectTokenOptimizers` enabled Pre/PostToolUse hook
  komutlarını `\brtk\b` / `\bsqz\b` ile tarar (kelime-sınırlı → `quirtky`/`sqzip`
  false-positive vermez). **Tespit hook komutundan**, PATH'ten değil (bağlanmamış ikili
  hiçbir şey optimize etmez → over-claim yok). Hata yutulmaz (loglanır, atlanır).
- **Blok içeriği:** hangi optimizer aktifse onu anlatır (rtk = komut proxy'si, sqz =
  çıktı sıkıştırıcı) + "kendisi çağırma, otomatik" + "çıktı kısaltılmış olabilir, kayıp
  değil" + **matcher kapsam notu**.
- **Kritik gerçek (SES130 kök nedeni):** WS5'in iki hook'u da `matcher=Bash`; ajanlar
  Windows'ta `PowerShell` aracını kullandığı için ikisi de **hiç ateşlenmiyordu**. Blok
  bunu açıkça söyler ("eşleşen `Bash` aracını token-ağır komutlar için tercih et").
- Test: `capabilities_tokenopt_test.go` (WS5-şekli iki-hook, kelime-sınırı, wildcard-no-scope).

Doğrulama: `go build ./...` + `go vet ./internal/agent` + `go test ./internal/agent ./internal/tools ./internal/api` yeşil (449 test). Detay `54-CAPABILITY-PROBE.md`.

## Oturum incelemesi düzeltmeleri: PowerShell UTF-8 + lazy-tool uyarısı + codebase-memory hint ✅ (2026-07-10)

Bir board-otomasyon oturumunun (SES130) debug/transkript analizinden çıkan üç somut sürtünme
noktası düzeltildi:

- **PowerShell 5.1 mojibake (kök neden + fix).** `internal/tools/builtin_shell.go` PowerShell
  aracını `powershell.exe -NoProfile -NonInteractive -Command …` ile hiçbir kodlama zorlaması
  yapmadan çalıştırıyordu. Windows PowerShell 5.1 (pwsh **yoksa** kullanılan fallback) konsol
  çıktısını + dosya-okuma cmdlet'lerini (Get-Content/Select-String/Import-Csv) legacy ANSI/OEM
  code-page'iyle çözer → **UTF-8-BOM'suz** bir Türkçe dosya cp1254 olarak okunur ve baytlar
  Go'ya geçersiz UTF-8 olarak ulaşır (ör. `talimatlar��`). Ajan bu oturumda ws-settings ve
  agent-soul'ları **bozuk** okumuştu. **Yeni:** `internal/tools/builtin_shell_encoding.go` —
  yalnızca legacy host'a (pwsh'e değil) bir UTF-8 prelude enjekte eder: `[Console]::OutputEncoding`
  + `$OutputEncoding` = UTF-8 (BOM'suz) ve `$PSDefaultParameterValues` ile Get-Content/Select-String/
  Import-Csv okuma default'u `utf8`. **Yazma davranışına dokunulmaz** (global `*:Encoding` set
  edilmez → BOM regresyonu yok). Canlı doğrulandı: prelude'suz mojibake, prelude'lu çıktı orijinal
  UTF-8 baytlarıyla birebir. Test: `builtin_shell_encoding_test.go`.
- **Lazy-tool same-batch reddi (#1).** İki-tier claude-cli köprüsünde extended (deferred) tier
  boş başlar, `activate_tools`'tan **sonraki** turda büyür; ajan `activate_tools`'u aktive edilen
  aracı **aynı yanıtta** çağırırsa CLI `No such tool available` döner. Araçları eager CORE'a taşımak
  bilinçli gateway-bütçe tasarımını (ve `TestInteractionTierSplit` sözleşmesini) bozacağı için
  **bütçe-nötr** çözüm: `activate_tools` açıklaması + çalıştırma sonucu artık "aktive edilenler
  yalnız **SONRAKI** adımda gelir, bu yanıtta çağırma" diye açıkça uyarır (`builtin_activate.go`).
- **codebase-memory hint güçlendirildi (#4).** `capabilities.go codebaseMemoryGuidance` artık raw
  shell grep'lerini (Select-String / Get-Content -Recurse / grep / findstr) ilk hamle olarak
  **yasaklar**, "yalnız indeks cevap veremezse son çare" der. (SES130'da ajan ~13 ardışık
  Select-String taraması yapıp indeksi az kullanmıştı.)

Ayrıca aynı analizde netleşen **sqz/rtk sorusu** (kod değişikliği gerektirmez): sqz TionSwarm'a
gömülü değil, `token` kategorisinde bir **PostToolUse hook** olarak Ayarlar→Hooks'tan bağlanır;
rtk ise ajanın Bash ile komutu sarmalamasıyla çalışır. İkisi de bu workspace'te bağlı/çağrılmadığı
için devreye girmiyordu; ayrıca claude-cli per-workspace `claude-home` kullandığından kullanıcının
global `~/.claude` rtk hook'u **devralınmaz** (CLI hook'ları yalnız TionSwarm'ın kendi hook DB'sinden
`--settings`'e yazılır). Detay `17-TOKEN-OPTIMIZASYON.md`.

Doğrulama: `go build ./...` + `go test ./internal/tools ./internal/agent ./internal/api` yeşil (445 test).

## Skills + Artifacts: sürükle-bırak ile grup değiştirme ✅ (2026-07-10)

Skills ve Artifacts listelerinde bir kartı başka bir grubun üzerine sürükleyip bırakmak,
o kartın `group` alanını kalıcı olarak değiştiriyor. Backend'e **sıfır** değişiklik: mevcut
`PUT /api/skills/{slug}/group` ve `PUT /api/artifacts/{id}/group` uçları (ve `Artifact.Group`
alanı) zaten vardı — kart notundaki "veri modeline group eklenmesi gerekebilir" varsayımı
yanlış çıktı.

**Yeni:** `frontend/src/shared/hooks/useGroupDnD.ts` — iki panelin paylaştığı **native HTML5
DnD** hook'u (`useGroupedList` ile aynı desen; `dnd-kit`/`react-dnd` gibi yeni bağımlılık
**eklenmedi**, TaskBoard'ın kanban sürüklemesiyle aynı yaklaşım). Hook `itemProps(item)` +
`groupProps(groupName)` prop-paketleri döndürür; paneller bunları karta ve grup sarmalayıcısına
yayar.

Davranış kararları:

- **No-op garantisi:** sürüklenen kartların hepsi zaten hedef gruptaysa `dragover` üzerinde
  `preventDefault()` **çağrılmaz** → tarayıcı bırakmayı reddeder → `drop` olayı hiç doğmaz →
  API isteği de gitmez. (Bayrakla bastırmak yerine tarayıcıya reddettiriyoruz.)
- **Dosya yükleme regresyonu yok:** sürükleme kendi MIME tipini taşır
  (`application/x-tionswarm-group-item`), ArtifactsPanel kökündeki upload drop zone ise
  `Files` tipine bakar. Ayrıca kart `onDrop`'u `stopPropagation()` çağırır → kart bırakması
  asla upload zone'una sızmaz.
- **Seçim farkındalığı:** sürüklenen kart mevcut çoklu seçimin içindeyse **tüm seçim** taşınır,
  değilse yalnız o kart.
- **Hata yutulmaz:** `onMove` hatasında `onError(...)` gösterilir ve her iki yolda `reload()`
  çalışır → liste yarım uygulanmış bir taşımada kalmaz, sunucu gerçeğine döner.
- **Katlı grup** başlığına bırakma çalışır (drop hedefi başlık + gövdeyi kapsayan sarmalayıcı).
- **Grupsuz** bucket'ına bırakmak `group=""` gönderir.

Test kancaları: grup sarmalayıcısında `data-drop-active`, kartta `data-dragging`
(mevcut `data-testid="skills-group-header"` / `artifacts-group-header` korundu).

Doğrulama: `go build ./...` + `go vet ./...` + `go test ./internal/...` temiz (backend'e
dokunulmadı, regresyon güvencesi); `npx tsc -b` + `npm run build` temiz. Not: repoda frontend
test altyapısı (vitest) **yok**, bu yüzden birim testi eklenmedi; `npm run lint` repo genelinde
zaten kırmızı (86 hata) — yeni hook sıfır bulgu veriyor, dokunulan iki panelin bulguları
değişikliğimizden önce de vardı.

## Built-in araç-çıktısı sıkıştırması TAMAMEN kaldırıldı → harici hook'lara devredildi ✅ (2026-07-10)

Az önce Sistem B (LLM özeti) kaldırılmıştı; bu adımda **Sistem A (deterministik) de
kaldırıldı** — TionSwarm artık **hiçbir built-in araç-çıktısı sıkıştırması içermez**.
Sıkıştırma tamamen **harici araçlara** devredildi: `sqz` (PostToolUse hook,
`agent/toolloop.go`'daki tek shrink yolu) ve `rtk` (komut-katmanında agent tarafından
`rtk <cmd>`). Gerekçe: server-tarafı `clear_tool_uses`/API-native compaction + retrieval
transcript baskısını zaten karşılıyor; ham çıktı kırpmasını bakımı-kolay harici bir
hook/CLI katmanına taşımak built-in bir alt sistem tutmaktan temiz. **Uyarı:** artık
kullanıcı bir hook bağlamazsa araç çıktıları sıkıştırılmadan modele gider (tek koruma
tool'ların kendi 64 KB hard-cap'i; hata/boş sonuçlar aynen geçer).

**Silinenler:** `internal/tools/compact` paketi (compact.go + test); `agent/compactor.go`
tümüyle + `toolloop.go` çağrısı; tunables `compactDeterministic/MaxLines/MaxBytes` +
`SetToolCompaction`/`CompactDeterministic`/`CompactMaxLines`/`CompactMaxBytes(For)` +
tüm **bütçe-ölçekleme aparatı** (`contextBudgetTokens`/`SetContextBudget`/
`ContextBudgetTokens`/`budgetScaleFor`/`budgetScaleLocked`/`defaultContextBudgetTokens` +
`DefaultCompactMax*`) + `server.go` iki çağrı; settings `CompactToolOutput/MaxLines/MaxBytes`
(struct/DTO/patch/default/clamp); DB `Usage.CompactSavedBytes` + `SessionUsage.CompactSavedBytes`
+ `AddCompactionSavings`/`AddSessionCompactionSavings`; API `budget/usage/session_usage`
`compactSavedBytes`; frontend `ContextPanel` sıkıştırma bölümü + `BudgetPanel` Tasarruf Merkezi
(tek Prompt-cache hücresi kaldı, "Sıkıştırma" trend metriği düştü) + `SessionUsageCard`/
`SessionDetailPanel`/`SettingsPanel` + `types/{settings,usage,agent}.ts`; testler.
`KindCompact`/`UsageKindCompact` **paylaşımlı** (ana konuşma compaction'ı) → korundu.
Doküman `17-TOKEN-OPTIMIZASYON.md` harici-delegasyon anlatısına dönüştürüldü, `tionswarm-settings`
skill'i güncellendi. `go build ./...` + `go test` (agent/db/settings/api **312 test**) +
frontend `tsc --noEmit` temiz.

## Araç-çıktısı LLM-özeti ("Sistem B") tamamen kaldırıldı ✅ (2026-07-10)

Araç çıktısı token optimizasyonundaki opt-in **LLM intent-aware özet** geçişi uçtan uca
silindi; yalnız **deterministik (Sistem A, ücretsiz kural tabanlı)** sıkıştırma kaldı.
Gerekçe: server-tarafı `clear_tool_uses` / API-native compaction (eskiyen/taşan sonuçlar)
+ retrieval katmanı bu ihtiyacı zaten karşılıyordu; ekstra ucuz-model çağrısı + gecikme +
tek-sağlayıcı sınırı değmiyordu (Sistem A ise sağlayıcıdan bağımsız giriş-noktası kalkanı,
kalıyor). Silinenler: `compactor.go` B dalı + `summarizeToolOutput`/`toolSummarySystemPrompt`;
tunables `CompactLLM*`/`CompactModel` + `SetToolCompaction` imzası (artık 3 arg); settings
`CompactLlmSummary`/`CompactLLMThreshold`/`CompactModel` (struct/DTO/patch/clamp/default);
DB `Usage.CompactSavedBytesLLM` + `AddLLMCompactionSavings`/`AddSessionLLMCompactionSavings`;
API `budget.go`/`usage.go`/`session_usage.go` `compactSavedBytesLLM` alanları; frontend
`ContextPanel` "Sistem B" bölümü + `BudgetPanel` Tasarruf Merkezi 3→2 hücre + trend tek seri
+ `SessionUsageCard`/`SessionDetailPanel`/`SettingsPanel` + `types/{settings,usage,agent}.ts`.
`KindCompact`/`UsageKindCompact` **paylaşımlı** (ana konuşma compaction'ı) → dokunulmadı.
Doküman: `17-TOKEN-OPTIMIZASYON.md` tek-sisteme indirildi, `tionswarm-settings` skill güncellendi.
`go build ./...` + `go test` (agent/db/settings 226 test) + frontend `tsc --noEmit` temiz.

## Cross-session bloğu artık aktif oturumları OTOMATİK göndermiyor ✅ (2026-07-10)

Dinamik bağlama enjekte edilen "Other sessions in this workspace" bloğu
(`internal/api/sessions_context.go` `sessionsContextBlock`) artık **yalnız geçmiş
(non-active) chat oturumlarını** listeliyor. **Aktif (canlı) oturumlar otomatik
gönderilmiyor** — sürekli değişip gürültü ekledikleri için ajan bunları **kendi
inisiyatifiyle** `list_sessions` aracıyla çeker (manuel kontrol). Blok başlığı
buna göre güncellendi ("Active (live) sessions are NOT listed here; call the
list_sessions tool..."), `Active:` bölümü ve kullanılmayan `maxActiveSessionsInBlock`
sabiti kaldırıldı. Tek kaynak: chat yolu (`composeTurnRequest`) + headless
(`buildAgentDynamicPrompt`) aynı fonksiyonu çağırdığı için ikisi de otomatik
hizalı. Workspace ayarları (`SessionContextEnabled`/`EveryTurn`/`RecentCount`)
aynen geçerli — yalnız aktif oturum satırları düştü. Test `TestSessionsContextBlock`
güncellendi (aktif oturum yok, yalnız `Recent:`).

**Manuel kontrol için statik ipucu + frontend temizliği:** `list_sessions` aracının
açıklaması (`internal/tools/builtin_sessions.go`, statik/cache'li şema) artık aktif
oturumların bağlama otomatik enjekte EDİLMEDİĞİNİ ve devam eden işi görmek için bu
aracın çağrılması gerektiğini söylüyor. Frontend `WorkspacePanel` "Session bağlamı"
toggle hint'i "aktif + son" yerine "son (geçmiş) sessionlar; aktif olanları
list_sessions ile kendi çeker" olarak güncellendi.

## Batching bulgusu REVİZE: deterministik değil, olasılıksal ✅ (2026-07-10, v6/v7)

v6/v7 koşuları önceki iki keskin hipotezi (yalnız-sürüm, katı "think XOR batch")
**olasılıksal** bir resme çevirdi: TS v6 (2.1.205, 8 thinking bloğu) **batch'ledi**;
CA v6 (2.1.197, 0 thinking) **serial** koştu. 16 koşu/probe'luk toplam örneklem
yine güçlü eğilimler gösteriyor — 2.1.197: 6/7 batch · 2.1.203/205+thinking: 1/5
batch · 2.1.205+thinking-off: 2/2 batch — yani sürüm ve thinking batching
OLASILIĞINI kuvvetle etkileyen kovaryatlar, ama garanti değil. Maliyetin gerçek
belirleyicisi **batch⇔ucuz / serial⇔pahalı**; ikinci kalıcı bulgu: TS aynı görevde
CA'dan **~1.7× fazla output token** üretiyor (18k vs 10k — prompt/verbosite
kaynaklı, ayrı optimizasyon adayı). Kesinleşen tek şey: v7'de `thinkingLevel`
Kapalı → **0 thinking** (aşağıdaki DisableThinking bağlaması canlıda doğrulandı).
Sağlıklı kıyas için sıradaki adım: koşul başına n≥5 tekrarla batching-oranı +
maliyet dağılımı.

## ThinkingLevel "Kapalı" → CLI'da MAX_THINKING_TOKENS=0 (batch dönüşü) ✅ (2026-07-10)

"Think XOR batch" bulgusunun ürün çözümü: `Request.DisableThinking` (yeni alan) —
`completeTracedInner` `thinkingBudgetForLevel(agent.ThinkingLevel)==0` ise set
eder (UI'daki **Kapalı** = `""`, per-turn `off` dahil); claude-cli **her iki
yolda** (one-shot `Complete` + persistent-session launcher) subprocess'e
`MAX_THINKING_TOKENS=0` env'i enjekte eder → thinking tamamen kapanır, Claude
Code ≥2.1.203'te paralel araç batch'leri geri gelir (probe kanıtı: gerçek
AlgoBench 2.1.205'te `[6,6,1,1,4,1]`). Native sağlayıcılar alanı yok sayar
(orada budget 0 zaten kapalı) — yani bu aynı zamanda bir **parite düzeltmesi**:
"Kapalı" seçimi artık CLI'da da gerçekten kapalı (eskiden CLI kendi adaptive
default'una düşüyordu). `cliEffortLevel("off")` low→high düzeltildi (thinking
zaten kapalı; düşük effort basit-görev batch'ini riske atar). **Frontend:**
Ajan ayarlarında thinking seçicisinin altı güncellendi — seviyenin artık CLI
`effortLevel`'ına eşlendiği yazıyor; **claude-cli + Kapalı** seçiliyken ⚡'lı
bilgi kutusu "think XOR batch" davranışını, maliyet kazancını ve takası anlatır
(`AgentSettingsForm.tsx`). Test: `TestCLIEffortLevel` güncel; suite 791/29
paket + tsc yeşil. Benchmark ajanını **Kapalı**'ya alıp v6'yı 2.1.205'te koşmak
artık pin gerektirmiyor.

## Paralel araç batch'leri chat'te gruplu görünüyor ✅ (2026-07-10)

Tek provider cevabında gelen çoklu paralel tool çağrıları artık UI'da tek küme
olarak render ediliyor: `TurnStep.Batch` (tur içi 1-tabanlı grup id, `omitempty`)
— **native döngü** her çok-çağrılı cevaba grup id atar (`toolloop.go` `batchSeq`;
tool kartı + izin/guardrail hataları dahil), **claude-cli yolu** stream-json'da
aynı API mesaj id'sini paylaşan `tool_use` event'lerini gruplar (`cliMessage.ID`
+ parser `curMsgID/curMsgTools/batchSeq`; CLI bir mesajı bloklara bölerek
yayınladığı için 2. çağrıda geriye dönük damgalama). Frontend: `TurnSteps.tsx`
ardışık aynı-batch adımları "⚡ N paralel araç çağrısı (tek istekte)" başlıklı
vurgulu çerçevede kümeler (tekil render'a düşen grup düz satıra döner);
`renderStep` yardımcıya çıkarıldı. Test: `claudecli_batch_test.go` (bölünmüş
mesaj + tekil + tek-event'te 3'lü senaryosu); suite 801/30 paket + tsc yeşil.
effortLevel düzeltmesiyle birlikte batching görünürlüğü de tamam — v4
benchmark'ta gruplar chat'te doğrudan izlenebilir.

## Benchmark serileşmesinin GERÇEK kök nedeni: `effortLevel` ✅ (2026-07-09, akşam)

Kontrollü probe deneyleri (aynı CLI 2.1.205, aynı mini görev "4 dosya yaz"):
global `~/.claude` home altında **4'lü paralel Write batch**, WS1 workspace
claude-home altında **[1,1,1,1] serileşme**; WS1 settings'e `"effortLevel":"high"`
eklenince **batch geri geldi**. Yani fail sürümün kendisi değil: **2.1.203+
`effortLevel` ayarlanmamışken (default effort) opus-4-8 tool çağrılarını
serileştiriyor**; kullanıcının global home'unda `effortLevel: high` olduğu için
kendi Claude Code kullanımı etkilenmiyordu, TionSwarm workspace home'ları minimal
settings ile default'a düşüyordu. `--disallowedTools`/`--allowedTools`/
`--dangerously-skip-permissions`/`stream-json` tek tek elendi (hepsi batch'li).
the external agent projects 2.1.197'ye pinli olduğu için hiç etkilenmedi. Maliyet zinciri:
default effort → serileşme → çağrı başına prefix re-read + cache-write primi →
$0.65→$1.51. **Uygulanan katman + SONRADAN DÜZELTİLEN beklenti (v5 sonrası,
2026-07-10 gece):** `ensureClaudeHomeEffortLevel` (workspace açılışında claude-home
settings'e eksikse `effortLevel`, kullanıcı `~/.claude`'undan kopya/`high`) +
`writeCLISettings` per-turn `effortLevel` (`cliEffortLevel`: ThinkingLevel eşlemesi)
uygulandı ve kalıcı (zararsız + ThinkingLevel'a CLI karşılığı kazandırır). ANCAK
v5 koşusu + belirleyici probe (gerçek AlgoBench promptu + `effortLevel:high` +
2.1.205, `--max-turns 6`) gösterdi ki **effortLevel yalnız basit/thinkingsiz
görevlerdeki serileşmeyi düzeltiyor; karmaşık (thinking tetikleyen) görevlerde
2.1.203+ effort değerinden bağımsız serileştiriyor** (ayarlar/hook/deny/MCP/
append-system-prompt tek tek probe'la aklandı; medium bile trivial görevde
batch'liyor). **NİHAİ KÖK NEDEN (aynı gece, probe'la kanıtlı):** 2.1.203+
**thinking aktifken paralel tool çağrısı yapmıyor** (think XOR batch; eski
CLI'lar — CA 2.1.197, TS 2.1.202 — hem düşünüp hem batch'liyordu).
`CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING=1` YETMİYOR (yine thinking üretti,
serial); **`MAX_THINKING_TOKENS=0`** thinking'i tamamen kapatınca gerçek
AlgoBench 2.1.205'te **[6,6,1,1,4,1] batch imzasına döndü** (v1 paritesi). Yani
iki geçerli çözüm: (a) CLI'ı ≤2.1.202'ye pinle (thinking + batch birlikte), veya
(b) 2.1.205'te `MAX_THINKING_TOKENS=0` env'i (batch var, thinking yok —
kalite/maliyet takası ajan bazında seçilmeli; TionSwarm'a ThinkingLevel="off" →
env enjeksiyonu olarak bağlanabilir, henüz bağlanmadı). Kaynaklar: claude-code
issue #24131 (paralel Write sınırı, closed-not-planned), #65785 (`--thinking
disabled` dokümantasyonu), topluluk: MAX_THINKING_TOKENS/effort yazıları.
Test: `claudehome_effort_test.go`; suite 800/30 paket yeşil.

## Benchmark v1↔v2 maliyet farkı analizi: fail CLI sürümüydü ✅ (2026-07-09)

AlgoBench v2'de (SES125) maliyet artışının nedeni "file-mutation verifier yazımları
serileştirdi" sanılıyordu — **yanlış atıf**. CLI transkript karşılaştırması
(claude-home): v1'de 6+7 Write **tek API cevabında paralel tool_use** (usage
değerleri özdeş), v2'de her Write ayrı çağrı (usage monoton artan). Tek değişen:
**claude-cli 2.1.202 → 2.1.203** (otomatik güncellenmişti); verifier CLI-native
`Write`'ları hiç görmez (delegasyon modunda fs araçları köprülenmez), lessons boştu,
epoch stale notu yoktu. Cache tarafı kusursuz doğrulandı: read monoton 28k→46k,
sıfır break (Prompt Epoch CLI yolunda da çalışıyor). Yan bulgular: (a) v2 turu
TionSwarm'a persist edilmedi — dev rebuild'i tam tur biterken geldi (kasıtlı
restart) ve **non-stream `/api/chat` inflight sidecar yazmıyordu** → süreç
ölümünde tur sessizce kaybolur; (b) makinedeki CLI 2.1.205'e güncellendi —
benchmark tekrarı aynı sürümle koşulmalı.

## Non-stream chat'e inflight paritesi ✅ (2026-07-09)

Yukarıdaki (a) bulgusunun düzeltmesi: `/api/chat` (non-stream) artık streaming
yoluyla aynı crash-korumasını taşıyor (`internal/api/chat.go` `inflightRecorder`):
reply id önceden ayrılır (`WithTurnID` debug paritesi dahil), tur
`CompleteWithToolsStream` ile koşup adımlar throttle'lı (600ms) `inflight.json`
snapshot'ına düşer → süreç ölümünde boot recovery kısmi turu materyalize eder.
Provider hatasında da stream paritesi: kısmi iz **interrupted mesaj** olarak
persist edilir (üretilmiş araç adımları/text kaybolmaz; HTTP sözleşmesi
değişmedi, 502), sidecar temizlenir; persist hatasında sidecar bilerek bırakılır
(recovery ağı). Test: `chat_inflight_test.go`; suite 797/30 paket yeşil.

## Board kartı: çoklu artifact referansı + dosya-drop ile ekleme ✅ (2026-07-09)

Kartlar artık workspace artifact'larına **birden fazla referans** taşıyabiliyor
(`Task.ArtifactIDs []string`, `models_task.go`; API create/update thread'lendi,
`store_task.go` UpdateTask kopyalar). Artifact yaşam döngüsünden bağımsız — kart
silmek referansı düşürür, artifact'ı silmez; çözülemeyen id'ler UI'da atlanır.
- **Frontend:** yeni `TaskArtifactRefs.tsx` bileşeni (chip listesi + "Var olan"
  arama-picker'ı + dosya sürükle-bırak alanı) TaskFormModal'a "Ekler" bölümü olarak
  eklendi. Dosya bırakınca: `/api/uploads` (bucket = kart id / `board`) → `createArtifact`
  (`sourcePath` + `origin:manual`) → id karta eklenir. Mevcut artifact upload→artifact
  akışı (`artifactKindForUpload`, ArtifactsPanel) yeniden kullanıldı.
- **Board kartı:** OS'ten dosya sürükleyip **doğrudan kart üzerine bırakma** → aynı
  upload+artifact+link akışı (`attachFilesToTask`, `TaskBoard.tsx`; `dataTransfer.files`
  olan drop dosya-ekleme, olmayan drop hâlâ sütuna taşıma). Kartta `📎 N` ek sayacı rozeti.
- **E2E doğrulama (canlı 8090):** mevcut-referanslı task create + multipart upload→artifact→link
  reload sonrası `artifactIds=['ART34','ART44']` olarak kalıcı.
- **Not:** frontend dist build'i şu an **ilgisiz** iki kullanıcı-WIP hatasıyla bloklu
  (`useAppEvents.ts:214`, `RunView.tsx:89`); bu özelliğin dosyaları tip-temiz.

## Navbar "çalışıyor" göstergesi workspace'ler arası sızıyordu 🐛 (2026-07-09)

- **Sorun:** Bir workspace'te oturum çalışırken navbar'daki "çalışıyor" (Aktivite/
  Executions) göstergesi doğru şekilde yanıyordu; ancak başka bir (boşta) workspace'e
  geçilince orada da iş devam ediyormuş gibi gösterge yanıyordu.
- **Kök neden:** `internal/api/activity.go` `handleActivity`, in-flight chat turlarını
  **server-geneli** `s.runs` registry'sinden (tüm workspace'ler ortak) topluyor. Döngüde
  `st.Executions = true`, session'ın bu workspace'e ait olup olmadığı `wsp.DB.GetSession`
  (workspace-scoped, izole store) ile doğrulanmadan **önce** set ediliyordu. Yabancı bir
  workspace'in akan turu global registry'de bulunduğundan, GetSession başarısız olsa (`continue`)
  bile bayrak zaten yanmış oluyordu.
- **Çözüm (kaynakta workspace-scope):** `chatRun`'a `workspaceID` alanı eklendi
  (`register` artık `id, sessionID, workspaceID, cancel` alır; chat yolu `ws(r).ID`, otonom
  yol `rt.WorkspaceID()` geçer). `chatRuns.activeSessionIDs(workspaceID)` verilen workspace'e
  filtreler (`""` = filtresiz, yalnız legacy/test). Böylece server-geneli registry'nin sızıntısı
  **kaynağında** kesilir; 4 çağıran (`activity`/`executions`/`graph`/`sessions.handleActiveSessions`)
  artık `wsp.ID` geçer → yabancı workspace'in turları hiçbir görünürlükte (busy dot, reload
  "thinking" restore, ağ grafiği, executions feed) görünmez. Diğer bayraklar (`Task`/`Flow`/
  `Schedule`) zaten `wsp.DB` üzerinden scoped'du. Kilit testi: `activity_scope_test.go`
  (`TestActiveSessionIDsWorkspaceScope`); tüm api paketi (90 test) yeşil.
- **Panel gözden geçirmesi — 2 ek frontend sızıntısı (aynı sınıf):** backend fix'i poll
  yolunu kapatsa da frontend'de iki yol daha aynı semptomu üretiyordu.
  (1) `App.tsx` `busyViews = useActivity(activeWorkspaceId, chat.streamingSessions.size > 0)`:
  `useChatStream` App seviyesinde yaşar, workspace switch'te **remount olmaz** → A'da başlayan
  stream'in session'ı Set'te kalır, B'ye geçince B'nin chat noktası yanardı. Düzeltme:
  `chatBusyLocal = ctl.chatSessions.some(s => chat.streamingSessions.has(s.id))` (yalnız aktif
  workspace'in oturumları; diğer aynı-workspace stream'leri zaten poll kapsar).
  (2) `useAppEvents.onEvent` → `bumpSignalsForEvent(e)` yorumu "workspace-match gate'inde" dese de
  koda göre gate'in **dışındaydı** → yabancı workspace olayı `activity`/`executions`/`network`
  panellerini boşuna re-fetch'e zorlar + göstergeyi kısa süre yakabilirdi. Düzeltme:
  `if (!e.workspaceId || e.workspaceId === getActiveWorkspace())` ile sarıldı (badge/toast yolu
  gate dışında kalır). **Kasıtlı global kaldı:** Logs ekranı (`logs.go` "application + all
  workspaces"), `events` SSE feed'i (her olay `workspaceId` taşır, frontend filtreler),
  `grants`/`sessionRunInfo` (session-ID anahtarlı). Frontend `tsc --noEmit` temiz.

## Flow-run görüntüleyicisi (RunView) canlı node ilerlemesi ✅ (2026-07-09)

- **Bağlam:** Aktivite ekranı canlı transkript aldıktan sonra "aynı canlılığı akış
  koşu görüntüleyicisine de" istendi. **Mimari bulgu:** akış node'ları session turu
  DEĞİL — her agent node'u `f.rt.complete(...)` ile session'sız tek provider çağrısı
  yapar (`session_step` yaymaz, tool-adım granülerliği yoktur); flow session'ı transkript
  turunu koşu **bitince** post-hoc yazar. Dolayısıyla stepBus/`FlowRun.SessionID` yolu
  buraya oturmuyordu. Doğru canlılık sinyali: engine'in zaten yaydığı `orchestration.NodeEvent`
  (per-node `start`/`done`+çıktı/`error`). RunView bunu kullanmıyordu → yalnız 3sn
  `listAllFlowRuns` polling ile canlı-kördü; ayrıca node olayları sadece koşuyu başlatan
  HTTP istemcisine gidiyordu (otonom/scheduled koşularda `obs=nil` → hiç canlı olay yok).
- **Çözüm (session_step deseninin flow eşi):** (a) `events.Event`'e `Node json.RawMessage`
  alanı; (b) `driveFlow` artık observer'ı **her zaman** sarmalar → her `NodeEvent`'i
  `emitFlowNode(runID, flowID, ev)` ile global bus'a yayınlar (`flow_node` tipi, `Target.flowRunId`),
  başlatıcının `obs`'u varsa yine çağrılır → **tüm** koşular (UI/otonom/scheduled) canlı olay
  yayar; (c) API SSE `flow_node` → ayrı `flownode` kanalı (notify/badge/toast yolunu kirletmez);
  (d) frontend: `AppEvent.node` + `FlowNodeEvent` tipi, `shared/lib/flowNodeBus.ts` (runId-keyed
  pub/sub), `useAppEvents.onFlowNode` → bus'a fan-out, `system.ts` üçüncü SSE callback'i;
  (e) `RunView` `run.id`'ye abone → canvas node durumu (asla geri sarmaz: pending→running→done/error)
  + "Adım izi" panelinde biten node çıktıları anında + çalışan node için "…çalışıyor" satırı;
  3sn poll caught-up olunca canlı ekler dedupe olur. `go build ./...` + flow/engine testleri
  (3/3) + `tsc` + `vite build` temiz.

## Aktivite (Executions) ekranı canlı transkript ✅ (2026-07-09)

- **Sorun:** Executions/Aktivite ekranı seçili yürütmenin transkriptini yalnız
  `api.listMessages` polling'iyle çekiyordu → **çalışan turda** araç adımları henüz
  diske yazılmadığından sadece "çalışıyor" noktaları görünüyordu; chat ekranı ise aynı
  turu SSE `session_step` bus'ından canlı gösteriyordu. Render katmanı zaten ortaktı
  (`MessageList`); ayrışma **canlı-veri katmanındaydı** (tek SSE beslemesi frame'leri
  yalnız `chat.applyAutoStep`'e veriyordu).
- **Çözüm — canlı-transkript katmanı birleştirildi:** (a) yeni `shared/lib/stepBus.ts`
  = `session_step` frame'lerini + `turn-end` sinyalini taşıyan session-scoped pub/sub
  (`refreshSignals.ts` desenini yansıtır, **ikinci EventSource açmaz**); (b) `useAppEvents`
  tek SSE beslemesinden `publishStep`/`publishTurnEnd` ile bus'a fan-out yapar (chat yolu
  aynen korunur); (c) yeni `features/executions/useLiveTranscript.ts` = chat'le **aynı**
  `chatStreamAutoLive` helper'larını (`foldAutoStep`/`recoverInflightSnapshot`/
  `clearAutoLiveEntry`) kullanan salt-okunur canlı-transkript hook'u — ghost balon + mid-turn
  inflight kurtarma dahil; (d) `ExecutionsPanel` kendi polling/tick transkript mantığını bu
  hook'la değiştirdi (liste polling'i kaldı). Sonuç: tek SSE → tek stepBus → N transkript
  görünümü aynı canlı kodu paylaşır. `tsc` + `vite build` temiz.
- **Executions canlı-transkript flicker fix (2026-07-10):** `useLiveTranscript.load`
  arka-plan poll'de (`RUNNING_POLL_MS` 2sn) + her `executions` sinyalinde `setMessages(m)`
  ile tüm listeyi kalıcı mesajlarla değiştirip **canlı ghost balonu düşürüyor**, ardından
  `recoverInflightSnapshot`'ı **her yüklemede** çağırıp akümülatörü snapshot'ın (≤600ms
  throttle) daha az adımına resetliyor + balon id'sini `live-auto-*`↔`messageId` arası
  değiştiriyordu → çalışan turda saniyede birkaç kez titreme ("N araç" oynaması, balonun
  çıplak working-dots'a düşüp geri gelmesi). Düzeltme: (a) ghost balonu kalıcı-liste
  takasında **koru** (id ile, yalnız `running` iken ve henüz persist olmamışsa); (b)
  `recoverInflightSnapshot`'ı yalnız **ghost yokken** çağır (gerçek mid-turn reload); (c)
  ghost persist olunca **veya** tur bitince akümülatörü sil (bir sonraki turun stale
  girdiye eklemesini önler). `runningRef` ile `load` kimliği stabil kalır. `tsc` temiz.

## Board otomasyon değişkenleri: `{{owner}}` + `{{priority}}` ✅ (2026-07-09)

`{{tags}}`'in ardından iki değişken daha: `BoardChangeEvent`'e `Priority` alanı
(4 fire noktası doldurur, `store_task.go`); `boardVars` artık ctx alıp `{{owner}}`'ı
`e.db.GetAgent` ile ajan **adına** çözer + `{{priority}}`'yi render eder
(`automation.go`). Araç şeması + frontend `BOARD_PROMPT_VARS` + testler güncellendi
(`TestBoardVarsSubstitution` tags/priority/owner kapsar). Canlı E2E: owner+priority+tags'li
kart → prompt `OWNER=[BenchOpus48] PRIO=[high] TAGS=[urgent]` render etti.

## Board UX: kart edit popup'ında kart ID'si (kopyalanabilir) ✅ (2026-07-10)

- **Card edit popup — kart ID'si (başlığın solunda)** (`TaskFormModal.tsx`): edit modunda
  header'da başlık input'unun **solunda** `tsk_…` chip'i; tıklayınca panoya kopyalar
  (Copy→✓ 1.5sn geri bildirim, `copyToClipboard`, `data-testid="task-id-copy"`). Amaç:
  kartı bağımlılık/otomasyon/prompt içinde referanslarken id'yi UI'dan almak. Yalnız edit
  modunda (create'te id yok).

## Board UX: create'te description focus + otomasyon `{{tags}}` ✅ (2026-07-09)

- **Create popup → description'a otomatik focus** (`TaskFormModal.tsx`): create modunda
  modal açılınca `descRef` ile açıklama alanına focuslanır (başlık zaten ondan üretiliyor
  → kullanıcı hemen yazmaya başlar). Edit modunda focus yok.
- **Board otomasyon prompt değişkeni `{{tags}}`**: kanban kartının etiketleri artık
  otomasyon promptuna parametre. `BoardChangeEvent`'e `Tags` alanı eklendi + dört fire
  noktası (Create/Move/Update/Delete, `store_task.go`) doldurur; `boardVars`
  (`automation.go`) `{{tags}}`'i virgülle-ayrık render eder. Araç şeması
  (`builtin_automationmgmt.go`) + frontend `BOARD_PROMPT_VARS` (`Automations.tsx`)
  güncellendi. Canlı E2E: board 'create' otomasyonu, etiketli kart → spawn oturumun
  prompt'u `CARD_TAGS=[urgent, backend]` olarak render etti.

## Prompt Epoch: oturum-başı donmuş bağlam snapshot'ı ✅ (2026-07-08)

external-context-agent "frozen snapshot" deseninin genellenmesi (yeni doküman
`57-PROMPT-EPOCH.md`): statik system promptu + araç şemaları per (session,
agent) oturum başında donar (`internal/agent/promptepoch.go` +
`prompt_epoch.json` sidecar'ı, restart-safe, fail-open) → oturum-ortası
skill/ayar/MCP-katalog/capability değişiklikleri prompt cache'ini artık kırmaz.
Adopt yalnız zaten-bust anlarında: compaction fold, 1h TTL soğuması,
model/workdir/katılımcı değişimi, `/refresh-context` komutu + `update_session
{refresh_context}` alanı. Tur içi `activate_tools` bilinçli kast: donmuş baza
canlı formuyla merge edilir (`mergeFrozenToolDefs`); kapatılan araç yürütmede
fail-closed kalır. Stale drift dinamik suffix notu + `epoch` debug olaylarıyla
görünür; workspace toggle `PromptEpochEnabled` (default açık) → Ayarlar →
Workspace paneli. Test: `promptepoch_test.go` (10 senaryo); suite 796/30 paket
+ tsc yeşil. **Ek (aynı gün): Debug viz** — "Prompt-cache olayları" kartı
(`PromptCacheEvents.tsx` + `buildPromptCacheSummary`): `epoch` + `cache_break`
olayları rozetli zaman çizelgesinde (donduruldu/adopte/stale/yenilendi/kırılım);
ham olay filtresi `epoch` tipini aldı; epoch emit'leri `WithSessionID` damgalı
(damgasız ctx'te olaylar düşüyordu — düzeltildi). **Canlı E2E doğrulandı**
(izole :8099, gerçek fable-5): `created → stale → refreshed → created` dizisi,
donmuş sidecar drift'i almadı → refresh sonrası aldı, kararlı turda olay yok,
`cache_break` 0. Yan bulgu: izole claude-home'da aralıklı `authentication_failed`
(token rotasyonu şüphesi, retry'la geçiyor) — epoch-dışı, ayrı araştırma.

## Board kartı: collapsible bağımlılıklar + pbi-kayması kök-neden ✅ (2026-07-08)

- **Card edit popup — bağımlılıklar collapsible** (`TaskFormModal.tsx`):
  "Bağımlılıklar — önce tamamlanması gereken görevler" bölümü artık chevron'lu
  açılır/kapanır başlık; görevde bağımlılık varsa açık, yoksa kapalı başlar +
  kapalıyken sayaç rozeti. Yer kaplamayı azaltır.
- **"pbi sütununa taşınan kartlar todo'ya kayıyor" — kök neden TionSwarm DIŞINDA:**
  harici `tionswarm-obsidian-sync` aracı (`Progs\tionswarm-obsidian-sync`,
  `watch --interval 30`) `status_map`'te olmayan custom board anahtarını her 30s'de
  `todo`'ya çeviriyordu (`obsidian.py` `_card_to_pm`/`_pm_to_card` `get(..,"todo")`
  fallback'i → conflict ping-pong). Düzeltme: bilinmeyen board anahtarı **pass-through**
  (todo'ya çevrilmez); custom sütun iki yönde kayıpsız round-trip eder. Canlı doğrulama:
  WS5'teki gerçek "Memory Provider Plugins Comparison" görevi 2+ sync döngüsü boyunca
  `pbi`'de sabit kaldı. **TionSwarm çekirdeği bu konuda zaten doğruydu** (`IsValidBoardKey`
  custom anahtarları kabul eder, store coercion yapmaz) — TionSwarm kodunda değişiklik yok.

## Board kartı: asenkron başlık + optimistic oluşturma ✅ (2026-07-08)

Board'da kart oluşturma iki iyileştirme aldı:
- **Asenkron AI başlık** (`internal/api/tasks.go` `handleCreateTask`): başlık boş
  bırakıldığında create artık LLM'i **beklemez**. Önce içerikten türetilmiş anlık
  bir placeholder (`placeholderTitle`: ilk satır, 60 rune + `…`) damgalanır ve kart
  hemen döner (~0.003s); gerçek AI başlığı **arka plan goroutine**'de üretilip
  (`TitleFor`, detached 60s ctx) yere iner ve `board` SSE olayıyla açık pencereler
  tazelenir. Guard: kart silinmiş veya kullanıcı başlığı elle değiştirmişse
  (placeholder ≠ mevcut başlık) AI sonucu **uygulanmaz** (kullanıcı düzenlemesi
  ezilmez). Başlık verilmişse davranış eskisi gibi.
- **Optimistic + seçili sütun düzeltmesi** (`TaskFormModal.tsx` + `TaskBoard.tsx`):
  create artık kartı **anında seçili sütunda** render eder (temp `temp-…` id →
  "başlık üretiliyor…" ipucu), sonra sunucu satırıyla `onReplaceTemp` ile
  upsert-uzlaştırır (eşzamanlı SSE reload'da tekilleştirir). Seçilen board sütunu
  (ToDo dışı sütunlar dahil) uçtan uca korunur. Placeholder başlık = açıklamanın
  kısaltılmış hali (`excerpt`).
- **Doğrulama (canlı 8090):** boş-başlık + `boardState=review` create → yanıt
  0.003s, doğru sütun, placeholder `"…"` ile; ~5s sonra AI başlığı
  ("TionSwarm 2FA TOTP Doğrulama Akışı Ekleme") yerine indi.

## Prompt-cache denetimi: hedge breakpoint + blok-bazlı coalesce ✅ (2026-07-08)

Cache-kırılım denetiminin iki düzeltmesi (`internal/providers/anthropic.go`):
- **Hedge breakpoint:** kayan history breakpoint'ine ek, bir önceki taşıyabilen
  mesajın son bloğuna 4. breakpoint — >20 bloklu paralel tool batch'lerinde
  Anthropic'in ~20-blok lookback ufku yüzünden tüm geçmişin miss olmasını önler
  (Raw mesajlar atlanır, en yeni Raw-olmayan öncüle düşer). Bütçe 4/4 dolu.
- **Blok-bazlı coalesce:** çok-ajanlı art arda aynı-rol düz mesajlar artık önceki
  mesajın text'ine değil **ayrı text bloğu** olarak eklenir → cached prefix'in son
  mesajının baytları değişmez. `coalescePlainSameRole` yalnız minimax'ta kaldı.
- Test: 5 yeni/güncel `TestToAnthropicMessages_*` + `TestCacheBreakpointStability`;
  tüm suite 787/787 yeşil. Detay `17-TOKEN-OPTIMIZASYON.md` §Hedge breakpoint.

## Self-healing: dosya-mutasyon verifier'ı ✅ (2026-07-08)

the external agent analizindeki "gereksiz adım optimizasyonu" maddesinin son boşluğu:
`Write`/`Edit`/`apply_patch` başarılı yazım sonrası diski geri okuyup içeriğin
gerçekten yere indiğini doğrular (`internal/tools/verifymutation.go`; ≤1MB
bayt-bayt, üstü SHA-256; dosya-başına tek ekstra okuma). Uymuyorsa çağrı hatalı
tool_result'a döner → mevcut self-healing zinciri (guardrail, tool-error tag,
ders) devralır. Detay `56-SELF-HEALING.md`.

## Self-healing devam turu: 8 adım + canlı E2E doğrulaması ✅ (2026-07-08)

Detay: `_Docs/56-SELF-HEALING.md` "Devam turu" bölümü. Özet:
`read_lessons`/`delete_lesson` araçları · ders yaşlandırma (45g) + signature
normalizasyonu (yol/sayı) · ajan-öncelikli enjeksiyon · Retry-After hint'i
turn-retry backoff'unda · CLI turlarına tur-sonu guardrail analizi (`cli_warn`) ·
hatalı-tur dispatch'i (`FireTurnFailed`) + tek-tık "Stuck oturum onarıcısı"
otomasyon şablonu · Debug viz'e Self-healing olayları bölümü. **Canlı E2E**
(izole instance, gerçek fable-5): tool-error tag → guardrail → stuck zinciri →
otomasyon → fixer → etiket/sayaç temizliği → ders doğumu → sonraki tura
enjeksiyon uçtan uca doğrulandı; E2E'nin yakaladığı 4 bug düzeltildi
(non-stream chat parity, CLI aux claude-home, SpawnTags omitempty, NONE artefaktı).

## Bütçe/maliyet ekranı hesaplama düzeltmeleri (4 bulgu) ✅ (2026-07-08)

Bütçe · oturum bilgisi · sohbet-debug · debug popup'larının hesap denetimi. Dört düzeltme, hepsi ortak `billing.PriceStat` + fiyat tablosu + `session_info` filler yolunda (tek nokta → dört ekran):
- **claude-cli tahmini cache tasarrufu $0 idi → düzeltildi** (`billing.PriceStat`): estimated dalı maliyeti tahmin edip tasarrufu `0` bırakıyordu; artık `ep.CacheSavings(cacheRead)` da tahmin ediliyor (`priced=false` korunur). Varsayılan anahtarsız sağlayıcıda Tasarruf Merkezi/oturum kazancı/mesaj-debug artık gerçek cache ROI'yi gösteriyor.
- **claude-cli cache-write primi 2× → 1.25×** (`providers.EstimateFor`): 1s-TTL primi (`CacheWrite1hMult`) yalnız native anthropic client'a özgü; Claude Code CLI 5dk TTL kullanır. Override sıfırlandı → ~%60 fazla fiyatlandırma giderildi.
- **Bağlam penceresi çubuğu segment↔toplam** (`session_info.go`): "kullanılan" başlığı `+MsgOverhead` sayıyor, segmentler saymıyordu → çubuk %100'e ulaşmıyordu. `buildFillers` artık mesaj başına `conversation.MsgOverhead` ekliyor + `ContextTokens` filler toplamından türetiliyor (birebir). `MsgOverhead` dışa açıldı.
- **"Tasarrufsuz maliyet" kartı tam-doğru baseline'a çevrildi** (`Price.CostNoCaching` + `billing.NoCacheCost` + `cumulative.noCacheCostUSD`): eski `cost + savings` cache-write primini içeride bırakıp senaryoyu şişiriyordu; artık cacheRead+cacheWrite tümü taban girdi fiyatından hesaplanan gerçek "caching yokmuş" tutarı gösteriliyor.
- 260707-quick-pass benchmark verisi aritmetik olarak iç-tutarlı doğrulandı (1.25× ile); tek çelişki kod tarafındaki 2× regresyonuydu → yukarıda giderildi.
- Regresyon: `billing_test.go` + `pricing_test.go`; backend build + test + frontend tsc yeşil. Detay `17-TOKEN-OPTIMIZASYON.md` §Hesaplama düzeltmeleri.

## Shell-araç promptu artık gate'e dinamik bağlı (prompt ↔ katalog drift'i giderildi) ✅ (2026-07-08)

**Sorun:** Statik workspace talimatları (`default-instructions.md` → `config/instructions.md`)
`Bash`/`PowerShell`'i "core, always-available" diye koşulsuz reklam ediyordu. Ama shell
araçları `buildRegistry`'de `r.tun.ShellEnabled()` gate'inin (+ backing shell varlığının)
arkasında — gate kapalıyken hiç register edilmez. Sonuç: model çıplak `PowerShell` çağırıp
`No such tool available: PowerShell … not enabled in this context` alıyordu (autotag bunu
zaten self-recovered permission-deny sayıyor; fonksiyonel bug değil ama modeli yanıltıyordu).

**Çözüm — dinamik render (tek-kaynak):**
- Statik talimatlardan shell vaadi çıkarıldı; yalnız fs araçları (`Read/Write/Edit/LS/Glob/Grep`)
  "always-on" kaldı. Shell satırı "gated — enabled olunca environment context söyler" notuna dönüştü.
  (Hem embedded `internal/workspace/defaults/default-instructions.md` hem canlı WS10 `config/instructions.md`.)
- Yeni `tools.ShellToolNames()` — `resolvePOSIXShell`/`resolvePowerShell` (araçların kendi resolver'ları)
  ile hangi shell'lerin register edileceğini adıyla döner → advertised isim, kayıtla asla drift etmez.
- Yeni `Runtime.ShellToolsContextBlock()` — gate açık + backing shell varsa Bash/PowerShell'i adıyla
  duyurur, aksi halde boş string. **Volatile → dinamik suffix** (gate tur-ortası değişebilir):
  chat yolu `composeTurnRequest` (EnvironmentContextBlock'tan hemen sonra) + headless
  `autonomousDynamicSuffix`, `EnvironmentContextBlock` ile aynı OS/shell kimliğini paylaşır.
- Build + vet + `internal/tools`/`internal/agent` testleri yeşil. Detay `53-CRAFTAGENT-PROMPT-PARITE.md`.

## Self-healing: Lessons UI + guardrail eşikleri ayarlara açıldı ✅ (2026-07-07)

- **Lessons API/UI:** `GET /api/lessons` + `DELETE /api/lessons/{id}`
  (`internal/api/lessons.go`, workspace-scoped) → Ayarlar → Bağlam → Self-healing
  bölümünde `LessonsList` bileşeni (araç rozeti, görülme sayısı, tarih, satır-başı
  silme, yenile). Frontend: `api/lessons.ts` + `types/lesson.ts` barrel'lara eklendi.
- **Guardrail eşikleri:** 6 eşik ayara açıldı (`guardExactWarn/Block`,
  `guardSameToolWarn/Halt`, `guardNoProgressWarn/Block`; 0 = varsayılan 2/5, 3/8,
  2/5; clamp 0..50). `toolGuardConfig` eşikleri taşır, `newToolGuard` <=0'ı
  varsayılana çözer; UI'da Self-healing bölümünde 3'lü grid. Detay `56` (ayarlar tablosu).

## Sohbet boş-durum ekranı: "Yeni sohbete başla" ✅ (2026-07-07)

**İstek:** Sohbet ekranını ilk açtığımızda ve hiç sohbet yokken devre-dışı bir input alanı
görünüyordu ama ajan bile seçili olmuyordu. Bunun yerine düzgün bir "Yeni sohbete başla"
ekranı çıksın.

- Yeni `features/chat/ChatEmptyState.tsx`: `ChatView` artık `activeSessionId` yokken (aktif
  oturum yok) transcript+devre-dışı composer yerine ortalanmış bir başlangıç kartı gösterir.
  - Ajan varsa: başlık "Yeni sohbete başla" + (birden fazla ajan varsa) ajan seçici (default'u
    `pickDefaultAgent` ile ayarlar) + **"Yeni sohbet"** butonu (`ctl.newSession` → seçili
    ajanla taze oturum açar). Tek ajan varsa hangi ajanın kullanılacağı rozet olarak gösterilir.
  - Ajan yoksa: "Önce bir ajan oluştur" + Ajanlar ekranına kısayol (`selectView('agents')`).
- `ChatView` yeni prop'lar alır: `defaultAgentId`/`onNewSession`/`onSelectDefaultAgent`/
  `onGoToAgents`; erken-return tüm hook'lardan SONRA (rules-of-hooks güvenli).
- `App`, `ctl.defaultAgentId`/`ctl.newSession`/`ctl.pickDefaultAgent`/`selectView('agents')`
  ile bağlar.

## Frontend feature-bazlı refactor (3 aşama) ✅ (2026-07-07)

**İstek:** Frontend kodlarının daha düzenli bir yapıya refactor edilmesi.
Davranış eşdeğeri, 3 commit: yapı taşıma → App.tsx decompose → dev dosya bölme.
Her aşamada `tsc -b` + `vite build` yeşil; eslint problem sayısı 116 → 102
(yeni ihlal sıfır, kalanlar refactor öncesinden).

- **Aşama 1 — feature klasörleri + `@/` alias:** 223 dosya taşındı (git rename
  olarak). Yeni yerleşim: `app/` (kabuk: App, NavRail, MobileNavBar, url/event
  hook'ları), `features/<domain>/` (chat, sessions, agents, flows, tasks,
  schedules, executions, artifacts, skills, tools, market, budget, logs,
  network, settings, workspace), `shared/` (components [eski common + markdown
  + çapraz-feature agent widget'ları], hooks, lib). `api/` + `types/` barrel'ları
  değişmedi. `tsconfig.app.json` `paths` + vite `resolve.alias` ile `@/*` = `src/*`.
- **Aşama 2 — App.tsx decompose:** 1.446 → ~630 satır kompozisyon kökü. Yeni
  `app/` modülleri: `viewRegistry` (VIEW_TITLE/HEADERLESS + `isChatKind`),
  `lazyPanels`, `useAppearance`, `useDeepLinks`, `useSessionsController`
  (agents/sessions/transcript state + tüm aksiyonlar), `useAppEvents` (SSE
  dağıtımı), `useAppNavigation` (URL↔state), `AppHeader`, ve
  `features/chat/ChatView` (transcript + alt yığın).
- **Aşama 3 — dev dosya bölmeleri (saf kod taşıma):**
  `useChatStream` 1006→375 + 7 modül (`chatStreamSend/AutoLive/Commands/
  Interventions/History/Types/Helpers`); `FlowsPanel` 1143→~450 +
  `FlowsListPane/FlowsHeader/FlowEditorView/flowActions/flowGraphOps/
  flowsPanelShared`; `MarketPanel` 1089→370 + `MarketGrid/PackDetailModal/
  PackPreview/previewParts/marketHelpers`; `ToolsPanel` 1033→314 +
  `useToolsPanelState/ServerManagement/ToolDetail/VisibilityControls`;
  `SessionDetailPanel` 897→429 + 9 bölüm modülü.
- **Eski→yeni yol eşlemesi (eski dokümanlardaki atıflar için):**
  `components/panels/X` → `features/<domain>/X` · `components/chat|sessions|
  agents|flow|artifacts|workspace|settings/` → `features/<domain>/` ·
  `components/common/` → `shared/components/` · `components/markdown/` →
  `shared/components/markdown/` · `hooks/`+`lib/` → feature'a aitse
  `features/<domain>/`, genel ise `shared/hooks|lib/`, app-kabuğuysa `app/`.
  05 ve öncesi tarihli dokümanlardaki eski yollar bu tabloyla okunmalı.
- Ölü dosyalar silindi (0 import): `features/agents/AgentRoster.tsx`,
  `features/sessions/SpawnSessionModal.tsx`, `shared/hooks/useResizableWidth.ts`,
  `app/App.css` — gerekirse git geçmişinden geri alınabilir.

## Self-healing Faz F: hata→ders döngüsü + Ayarlar UI ✅ (2026-07-07)

**İstek:** Self-healing ayarlarının frontend'e eklenmesi + hermes `background_review`
karşılığı hata→ders döngüsü. Detay: `_Docs/56-SELF-HEALING.md` (Faz F bölümü).

- **Lesson reflector** (`internal/agent/lessons.go`): kötü biten tur → arka-plan ucuz
  model çağrısı (`KindReflect`) → tek genellenebilir ders; `AutoTagTurn` hunisinden
  tetiklenir, `lessonReflect` ayarıyla (vars. açık) gate'li. Policy denial/guardrail
  coaching/iptal/stuck-gate reddi kanıta girmez; `NONE` cevabı kaydedilmez.
- **Lessons store** (`internal/db/store_lessons.go`): workspace-geneli `lessons.jsonl`,
  signature dedupe (tekrar → `Count++` + metin tazelenir), cap 200, kendi mutex'i.
- **Enjeksiyon**: `Runtime.LessonsContextBlock` — en yeni 5 ders chat + headless
  dinamik suffix'ine girer (cache'li prefix bozulmaz). Debug olayı: `lesson`.
- **Frontend**: `ContextPanel` "Self-healing" bölümü (guardrail uyarı/devre kesici
  toggle'ları, stuck eşiği, lesson toggle) + "Tur kurtarma" grid'ine sağlayıcı retry
  bütçesi; `types/settings.ts` + `SettingsPanel.saveApp` patch'i. **Bugfix:** saveApp
  patch'inde handoff/progress/autoTag/debugJournal alanları eksikti (bu toggle'lar
  hiç kaydedilmiyordu) — eklendi.
- Testler: `store_lessons_test.go` + `lessons_test.go`; `tsc --noEmit` + vite build temiz.

## Fable 5 uyumluluk paketi (6 madde) ✅ (2026-07-07)

**İstek:** Fable 5 uyumluluk denetiminde bulunan 6 boşluğun kapatılması.

1. **Thinking-blok echo'su (kritik):** Fable/Mythos'ta thinking HER yanıtta var (araç
   döngüsü dahil) ve bloklar aynen geri gönderilmek zorunda. `rawEcho` kapısına
   `providers.AlwaysOnThinking(agent.Model)` eklendi — Fable'lı ajanlar hiçbir toggle'a
   bağlı olmadan verbatim echo alır.
2. **Refusal + server-side fallback:** `AnthropicRefusalFallback` ayarı (**varsayılan
   AÇIK**, Anthropic'in Fable rehberi) → Fable-sınıfı isteklere `fallbacks:
   [claude-opus-4-8]` + `server-side-fallback-2026-06-01` beta; sınıflandırıcı reddi aynı
   çağrıda Opus 4.8'le karşılanır. `stop_details` parse edilir (`Response.StopDetails`);
   araç döngüsünde `refusal` artık boş balon yerine kategori+açıklamalı hata kartı üretir.
   Mid-output fallback echo kuralı için `sanitizeFallbackEcho` (sınır öncesi thinking/
   tool_use blokları düşülür). UI toggle + `fallback` iz adımı.
3. **Zaman aşımları:** model-sınıflı istek bütçesi — adaptive sınıf (Fable/4.7+/Sonnet 5)
   600s, eskiler 120s (`requestCtx`; client transport tavanı 630s). `scheduleTimeout`
   120s → **30 dk** (Fable'ın dakikalarca süren tek istekleri + uzun araç döngüleri;
   kaçak koruması iterasyon/bütçe guard'larında).
4. **`model_context_window_exceeded` stop reason:** `decideRecovery`'ye dal eklendi —
   hata-şekilli taşmayla aynı tek-atım compact-and-retry; tekrarında tur artık
   "completed" değil `context_window_exhausted` olarak işaretlenir (iz kartı + kırpılma
   notu). Araç döngüsünün yanıt-tarafına compact uygulaması eklendi.
5. **Prompt ince ayarı:** `autonomousBootReminder` de-prescribe edildi ve genişletildi —
   "Autonomous operation": izin sorma/planla bitirme yok (aksiyon al), baseline doğrula,
   TEK iş, ilerleme iddiaları bu oturumdaki araç sonuçlarına dayansın, kapanışta kayıt.
   `notify` açıklamasına birebir-iletim (verbatim) tetiği eklendi (send_to_user deseni).
6. **Veri saklama notu:** Fable katalog açıklamasına 30-gün saklama + fallback notu;
   anthropic 400 hatasında "retention" geçiyorsa eyleme dönük ipucu ekleniyor
   (ZDR org'da isteğin değil org ayarının sorun olduğu).

Testler: `TestSanitizeFallbackEcho`, `TestRequestCtx`, `TestDecideRecovery_ContextWindowStop`,
güncellenen boot-reminder pinleri. ✅ build/vet temiz; 764 test / 35 paket; frontend `tsc` temiz.

## Kendi kendini onaran oturum akışları (self-healing, Faz A–D) ✅ (2026-07-07)

**İstek:** external-context-agent incelemesinden çıkan self-healing desenlerinin TionSwarm'a
uyarlanması: tool hatalarını çözen, oturum akışını onaran, döngüleri kesen ve stuck
oturumları işaretleyen katman. Detay: `_Docs/56-SELF-HEALING.md`.

- **Faz A** `internal/agent/errclass.go`: provider hata taksonomisi
  (`rate_limit/overloaded/server_error/timeout` retry-edilebilir; `auth/billing/
  cancelled/unknown` terminal) + `decideRecovery`'de `contProviderRetry` — jitter'lı
  backoff'la sınırlı retry (`maxProviderRetries` ayarı, vars. 2). İptal artık
  `termCancelled`.
- **Faz C** `internal/conversation/repair.go` `RepairSequence`: her provider çağrısı
  öncesi tur-içi tool_use↔tool_result eşleşme onarımı (orphan drop / duplicate dedup /
  eksik sonuç sentezi / bölünmüş assistant batch merge). Saf + idempotent; onarımlar
  `debug.jsonl` `repair` olayı.
- **Faz B** `internal/agent/toolguard.go`: tur-başına döngü tespiti (exact-failure 2/5,
  same-tool 3/8, no-progress 2/5; idempotent = `RiskRead`). Uyarı hint'i başarısız
  tool_result'a eklenir (`toolGuardWarnings` vars. açık); hard stop (`toolGuardHardStop`
  vars. kapalı) blok/`termGuardrailHalt`. `debug.jsonl` `guardrail` olayı.
- **Faz D** `db.Session.StuckTurns` + `stuck` auto-tag + `stuckGate`: ardışık kötü tur
  eşiği (`stuckTurnThreshold` vars. 3) aşınca otonom turlar reddedilir (manuel chat
  serbest); `stuck` etiketi kaldırılınca sayaç sıfırlanır. Etiket-otomasyonla onarım
  ajanına bağlanabilir.
- Testler: `errclass_test` / `toolguard_test` / `repair_test` / `stuck_test` (tablo
  testleri). Sıradaki: hata→ders döngüsü (background-review karşılığı, ayrı plan).

## Workspace oluşturunca claude-cli hazırlık kapısı (auth popup / Sağlayıcılar yönlendirme) ✅ (2026-07-07)

**İstek:** Yeni bir workspace oluşturulduğunda, claude-cli kuruluysa ama bu workspace'in
claude-home'u yetkilendirilmemişse bir bildirim çıksın ve oradan "claude-cli kimlik
doğrulama" popup'ı açılabilsin; claude-cli hiç yoksa kullanıcı Sağlayıcılar ekranına
yönlendirilsin.

**Backend:**
- `providers.ClaudeCLI.Installed()` (yeni): yapılandırılmış `binPath`'i `exec.LookPath` ile
  çözer — CLI binary'si mevcut mu (login'den bağımsız). "CLI yok" ile "CLI var ama login yok"
  ayrımını sağlar.
- `claudeAuthDTO`'ya `installed bool` alanı; `handleWorkspaceClaudeAuth` artık probe'dan
  ÖNCE `cli.Installed()` kontrolü yapıyor — binary yoksa `installed:false` + açıklayıcı detay
  döner (login probe'u boşa çalıştırmaz).

**Frontend:**
- `api.checkWorkspaceClaudeAuth` dönüş tipine `installed` eklendi.
- `useWorkspaces.createWorkspace` artık oluşturulan workspace'i (veya hatada `undefined`)
  döndürüyor → çağıran taraf oluşturma-sonrası kapı çalıştırabiliyor.
- Yeni `features/workspace/ClaudeAuthGate.tsx`: `App` bir `claudeGateNonce` sayacıyla her
  başarılı oluşturmadan sonra tetikler; gate yeni (aktif) workspace'in login durumunu
  problar. Sonuç: login var → sessiz; CLI var + login yok → eyleme dönüştürülebilir bildirim
  (buton `ClaudeAuthDialog`'u açar, kimlik doğrudan bu workspace'in claude-home'una yazılır);
  CLI yok → `onNavigateProviders` ile Sağlayıcılar ekranı (`setSettingsCat('providers')`).
- `App` tüm oluşturma yollarını (onboarding + rail + mobil) tek `handleCreateWorkspace`
  sarmalayıcısından geçirir. **Mevcut klasör bağlama** (`attachWorkspace`, onboarding) da
  aynı kapıyı tetikler (`handleAttachWorkspace`) — bağlanan workspace'in kendi
  yetkilendirilmemiş claude-home'u olabilir; `attachWorkspace`'in hata-fırlatma sözleşmesi
  korunur (inline doğrulama hataları).
- **Sohbet-açılışında tekrar-hatırlatma (`requireNoProvider`):** Kapı iki nedenle tetiklenir
  — (a) oluştur/bağla (`requireNoProvider=false`, her zaman probe/popup); (b) her sohbet
  ekranı girişinde + workspace değişiminde (`requireNoProvider=true`). (b) yalnızca
  workspace'te **hiçbir kullanılabilir provider yoksa** iş yapar: `hasNoUsableProvider()` =
  hiçbir built-in anahtar (anthropic/minimax/openrouter) + claude-cli token + custom provider
  yok. Bu **ucuz** ön-kontrol, **pahalı** `claude -p` login probe'undan ÖNCE çalışır →
  provider'ı olan kullanıcı asla rahatsız edilmez, hiç kurulumu olmayan kullanıcı her sohbet
  açılışında popup'ı yeniden görür (CLI kuruluysa) veya Sağlayıcılar'a yönlendirilir (CLI
  yoksa). `App`'te `claudeGate={nonce,requireNoProvider}` durumu + `view==='chat'` effect'i.

## API-native P2+P3: sunucu web search/fetch + server-side compaction (toggle'lı) ✅ (2026-07-07)

**İstek:** _Docs/55 P2 ve P3'ün eklenmesi, her ikisi de ayarlardan açılıp kapanabilir.
İkisi de ek sunucu/süreç GEREKTİRMEZ — mevcut /v1/messages isteğinin alanlarıdır.

**P2 — Sunucu-tarafı web search + web fetch (`AnthropicWebTools`, varsayılan kapalı):**
- `Request.WebTools` → `toAnthropicTools` (yeni `serverToolOpts` yapısı): `web_search` +
  `web_fetch` sunucu araçları isteğe eklenir; aramayı Anthropic yürütür, alıntılı sonuçlar
  aynı yanıtta döner. 4.6+ modellerde `_20260209` dinamik-filtreli sürüm
  (`SupportsDynamicWebTools`); eski modellerde VE PTC açıkken temel sürümler
  (`web_search_20250305`/`web_fetch_20250910`) — dinamik sürüm kendi code-execution
  ortamını taşıdığından PTC'yle çifte ortam oluşmaz.
- Maliyet freni: tur başına `max_uses` tavanları (arama 8, çekme 12, sabit).
- Sonuç blokları iz adımı olarak UI'a düşer (`web_*_tool_result` → sonuç sayısı/hata kodu);
  RawContent verbatim echo web modunda da açık (şifreli alıntı içeriği tur içinde korunur).
- Yalnız `provider.Name()=="anthropic"` + native tool loop (araçları açık ajanlar).

**P3 — Server-side compaction (`AnthropicServerCompaction`, varsayılan kapalı):**
- `WithBetas` üçüncü parametre → `compact-2026-01-12` beta başlığı +
  `context_management.edits`'e `{type:"compact_20260112"}` (sunucu-varsayılan ~150K tetik).
  Context-editing ile bağımsız; ikisi açıkken iki edit birden gönderilir.
- Compaction blokları TUR İÇİNDE RawContent verbatim echo ile aynen geri gönderilir
  (API şartı) — uzun tek turların (araç döngüsü) taşma sigortası. Turlar-arası transkripti
  istemci-tarafı compaction yönetmeye devam eder (çifte özetleme çakışması yok; tam
  turlar-arası server compaction, compaction bloklarının db persist'ini gerektirir — P3'ün
  ileride derinleştirilecek kısmı olarak _Docs/55'te not edildi).
- Kayıt zinciri: settings → `SetAnthropicBetas(cache, ctxEdit, serverCompact)` (registry →
  kind cfg → `WithBetas`) + tunables aynası (rawEcho kapısı için).

**UI:** Ayarlar → Bağlam → "Anthropic beta" altında iki yeni toggle (web araçları +
API-native compaction), Türkçe ipuçlarıyla.

Testler: `TestToAnthropicTools_WebTools` (dinamik/temel sürüm seçimi, PTC çakışma kuralı,
max_uses, breakpoint yerleşimi), `TestContextMgmt_ServerCompaction` (edit + beta başlığı,
bağımsızlık). ✅ build/vet temiz; `go test ./...` 734 test / 35 paket; frontend `tsc` temiz.

## API-native P1+P6+P7+P4: structured outputs, system mesajları, strict/effort, PTC ✅ (2026-07-07)

**İstek:** _Docs/55 yol haritasının P1, P6, P7, P4 maddelerinin uygulanması.

**P1 — Structured Outputs (`output_config.format`):**
- `Request.OutputSchema` → `applyOutputSchema` (yalnız destekleyen modeller: Fable/Mythos,
  Opus 4.8, Sonnet 5, Haiku 4.5, legacy 4.5/4.1 — Opus 4.6/4.7 ve Sonnet 4.6 matriste YOK).
- **Titler**: `{"title": string}` şeması + JSON-parse-else-fallback (claude-cli serbest metin
  dönmeye devam eder, sanitizer korunur).
- **Orchestration**: agent node `outputSchema` alanı (yeni `SchemaAgentRunner` opsiyonel
  arayüzü — mevcut runner mock'ları değişmedi); branch node `jsonField` alanı — son çıktı
  JSON'ından üst-düzey alan çekilip eşleştirilir (`{"verdict":"SHIP"}` → parse-proof karar).

**P6 — Mid-conversation system messages (Opus 4.8):**
- `toAnthropicMessages` artık RoleSystem'ı düşürmüyor: Opus 4.8'de `{"role":"system"}` olarak
  geçer (cache-safe operatör kanalı); diğer modellerde `foldSystemMessages` metni ÖNCEKİ user
  mesajına `<system-reminder>` bloğu olarak katlar (rol alternasyonu korunur — steer'in
  user-user 400 riskini de çözer). Tool-loop steer mesajları Opus 4.8 + anthropic'te
  system rolüyle gönderilir.

**P7 — küçükler:**
- **Strict tool use**: `ToolDef.Strict` → `strict:true` (fs süiti Read/Write/Edit/LS/Glob +
  Grep); registry `foldStrict` şemaya `additionalProperties:false` + `required:[]` enjekte
  eder; PTC'li (allowed_callers) araçlarda otomatik düşer (uyumsuz).
- **xhigh/max thinking**: ThinkingLevel'a iki yeni seviye (32K/64K bütçe eşlemesi) →
  adaptive sınıfta `effort: xhigh|max`; legacy enabled+budget yolunda 16384'e kırpılır.
  Ajan formu + composer picker seçenekleri eklendi.
- **CountTokens**: `providers.TokenCounter` + `Anthropic.CountTokens`
  (/v1/messages/count_tokens); oturum bağlam önizlemesi `?accurate=1` ile gerçek sayımı
  `accurateTokens` alanında döner (sezgisel tahmin compaction'ı sürmeye devam eder —
  davranış değişmez, yalnız drift görünür olur).

**P4 — Programmatic Tool Calling (`code_execution_20260120`):**
- `AnthropicProgrammaticTools` ayarı (varsayılan kapalı) → tunables → tool loop: uygun
  builtinler (`CodeModeEligible`, MCP hariç) `allowed_callers:["code_execution_20260120"]`
  ile işaretlenir; code-execution sunucu aracı listeye eklenir.
- `ToolCall.Caller` parse edilir; programatik batch'in yanıt mesajı `OnlyToolResults` —
  dinamik sonek o mesaja binmez (API şartı: saf tool_result). Steer, programatik sonuç
  beklerken ERTELENIR (sıradaki normal iterasyonda teslim edilir).
- Container zinciri: yanıttaki `container.id` sonraki isteklere `container` olarak taşınır
  (bekleyen programatik çağrı varken zorunlu). RawContent verbatim echo PTC modunda da açık.
- `code_execution_tool_result` stdout/stderr'ı iz adımı olarak UI'a düşer.
- UI: Ayarlar → Bağlam → "Programatik araç çağrısı (code execution)" toggle'ı.

Testler: `anthropic_native_test.go` (output schema, system fold/native, saf tool_result,
caller sınıflandırma, effort/clamp), PTC tool-marshal testleri. ✅ build/vet temiz;
`go test ./...` 732 test / 35 paket; frontend `tsc` temiz.

## API-native Task Budgets + Tool Search (beta) ✅ (2026-07-07)

**İstek:** Anthropic'in güncel API özelliklerinden Task Budgets ve native (sunucu-tarafı)
Tool Search'ün TionSwarm'a eklenmesi (optimizasyon araştırması madde 1-2).

**Task Budgets (`task-budgets-2026-03-13` beta):**
- `providers.Request.TaskBudgetTokens` + `anthropic.go applyTaskBudget`: adaptive-sınıf
  modellerde (Opus 4.7/4.8, Sonnet 5, Fable 5 — `SupportsTaskBudget`) isteğe
  `output_config.task_budget {type:"tokens", total:N}` + beta başlığı eklenir; API
  minimumu 20K'nın altı otomatik yükseltilir. Model tüm ajan döngüsü için geri sayım
  görür ve kendini ona göre ayarlar — sert iterasyon caplerinin yumuşak, model-farkındalı
  tamamlayıcısı.
- Ayar: `AutonomousTaskBudgetTokens` (0 = kapalı, varsayılan) → tunables →
  `completeTracedInner` yalnız OTONOM turlarda `req.TaskBudgetTokens` doldurur
  (etkileşimli sohbet bütçesiz kalır). UI: Ayarlar → Bağlam → "Otonom görev bütçesi".

**Native Tool Search (`tool_search_tool_regex_20251119`):**
- `ToolDef.DeferLoading` + `tools.Registry.DeferredDefs`: TAM katalog gönderilir —
  eager araçlar normal, lazy/hidden/MCP araçları `defer_loading:true` ile; aktive
  edilenler sıcak kalır (deferred değil). Herhangi bir deferred def varsa Anthropic
  istemcisi arama sunucu-aracını listeye başa ekler (breakpoint asla sunucu araca
  binmez). Keşfedilen şemalar sunucuda EKLENEREK yüklenir → cache öneki bozulmaz;
  araç bloğu iterasyonlar arası bayt-stabil.
- **Raw passthrough:** `Response.RawContent` / `Message.RawContent` — native-search
  modunda asistan turları sunucu bloklarını (tool_search_tool_result / server_tool_use)
  bire bir geri yansıtır (`anthropicMessage.Raw` + özel MarshalJSON). Raw mesajlar
  coalesce edilmez, breakpoint/dinamik binmez. `pause_turn` stop-reason'ı döngüde
  otomatik devam ettirilir (asistan içerik verbatim eklenir, ek user mesajı yok).
- Kapı: `AnthropicNativeToolSearch` ayarı (varsayılan kapalı) + `provider.Name() ==
  "anthropic"` (minimax-anthropic/custom uçlar sunucu araç tipini reddeder). Mevcut
  activate_tools/tool_search builtinleri yanında çalışmaya devam eder.

Testler: `anthropic_toolsearch_test.go` (task budget resolver, defer serileştirme,
raw marshal/passthrough), `deferred_defs_test.go`. ✅ build/vet temiz; 725 test yeşil;
frontend `tsc` temiz.

## Token/prompt optimizasyon turu: adaptive thinking + cache varsayılanı + headless split ✅ (2026-07-07)

**İstek:** Kapsamlı optimizasyon denetiminin 1,2,3,4,6,7,9 numaralı maddelerinin uygulanması.

**Ne yapıldı (backend):**
- **Adaptive thinking (kritik düzeltme):** `providers/anthropic.go` — `thinking:{type:"enabled",budget_tokens}`
  formatı Opus 4.7/4.8, Sonnet 5 ve Fable 5'te **400 döndürüyordu** (kataloğun tamamı). `thinkingFor`
  artık model-sınıf farkındalı: adaptive sınıfta `{type:"adaptive", display:"summarized"}` +
  `output_config.effort` (bütçe→low/medium/high eşlemesi); Fable/Mythos'ta "off" alanı tamamen
  atlar (explicit disabled da 400); eski modeller + MiniMax `enabled+budget` şeklinde kalır.
  `providers/thinking.go` yeniden yazıldı (`UsesAdaptiveThinking`/`AlwaysOnThinking`/
  `EffortForThinkingBudget`); `agent.resolveThinkingBudget` Fable tabanı kaldırıldı (çeviri
  provider'a taşındı). Testler güncellendi.
- **Prompt caching varsayılan AÇIK:** `settings.Default()` → `ExtendedPromptCache: true`
  (mevcut settings.json dosyaları kayıtlı değerini korur; yeni kurulum cache'li başlar).
- **Headless statik/dinamik ayrımı:** `autonomousSystemPrompt`'tan uçucu tarih/saat çıkarıldı
  (env satırı bayt-stabil olduğundan statikte kaldı); yeni `autonomousDynamicSuffix(ctx)`
  (saat + hedef bloğu) scheduler/spawn/subagent/flow yollarında `SystemDynamic`'e bağlandı →
  otonom turlar da artık statik prefix cache'inden yararlanır. `DateTimeContextBlock`
  agent paketine alındı; chat yolu aynı kaynağı kullanır.
- **Fiyat düzeltmeleri:** Fable 5 $3/$15 → **$10/$50** (anthropic + openrouter tabloları);
  native anthropic girdilerine `CacheWrite1hMult=2.0` override'ı (client daima 1h TTL ister —
  yazma primi 1.25× değil 2×).
- **`<recent_tool_activity>` dinamik soneke taşındı:** recap artık geçmiş asistan mesajlarına
  gömülmüyor (pencereden düşen turun baytları değişip rolling cache breakpoint'ini kırıyordu);
  `recentToolActivityBlock(history)` tek birleşik blok üretir, `composeTurnRequest` yeni
  `toolRecap` parametresiyle dinamik tarafa ekler (chat/stream/wake/preview 4 çağrı yolu).
- **Eager araç tanımları küçültüldü:** `run_code` 1577→~600 karakter (kullanım detayı keşif
  çıktısına taşındı), `run_subagent` açıklama+şema sadeleşti, `Grep` şemasındaki bayrak
  açıklamaları kaldırıldı (anlamları tanımda tek yerde). Tur başına ~1.5-2K token kazanç.
- **Tekilleştirme/temizlik:** `agent.BuildSystemPrompt` tek persona kaynağı (api kopyası
  delegasyona döndü); ölü `default-instructions_old.md` (~40KB) silindi; software swarmpack
  akışının Execute node'u artık `{{input}}` + `{{node.search}}` bulgularını da alıyor.

✅ `go build`/`vet` temiz; `go test ./...` 720 test / 35 paket yeşil.

## Agent config: yasaklı araç + atanan skill "chip"leri tıkla-kaldır ✅ (2026-07-07)

**İstek:** (1) Yasaklı araç chip'inde ayrı çarpı yerine chip'in kendisine tıklayınca
yasak kalksın. (2) Ajana atanan skiller liste değil chip olarak görünsün.

**Ne yapıldı (yalnız frontend):**
- `AgentToolsSection.tsx`: yasaklı araç `<li>` içindeki ayrı `X` butonu kaldırıldı;
  chip'in tamamı artık `unblock` butonu (`data-testid="agent-tool-blocked"` üstünde
  onClick). Hover'da `Ban` ikonu `X`'e döner + kırmızı vurgu. Eski `agent-tool-unblock`
  testid'i kaldırıldı (dış referansı yok).
- `AgentSkillsSection.tsx`: seçili skiller `<ul>` liste yerine `flex-wrap` **chip**
  (aşağıdaki ekleme picker'larıyla aynı görsel dil). Chip'in tamamı `remove` kontrolü
  (`data-testid="skill-remove"` korundu); hover'da kırmızı vurgu + `X`. Bulunamayan
  slug kırmızı chip. Frontend `tsc` yeşil.

## Sohbet iz kartları arka-plansız + gönderince en-alta kay ✅ (2026-07-07)

**İstek:** (1) Yeni mesaj gönderince transkript en alta kaysın. (2) Sohbetteki
"Görev Listesi" bubble'ının arka-plan dolgusu kalksın; benzer UI bileşenlerinde de.

**Ne yapıldı (yalnız frontend):**
- **Auto-scroll:** `MessageList.tsx` — kaydırma yalnız kullanıcı en-alta "pinned"
  iken çalışıyordu; yukarı kaydırıp mesaj gönderince gönderilen mesaj görünmüyordu.
  `prevLastId` ref'i eklendi: en yeni tur **insanın kendi** turu (role `user`,
  `authorKind !== 'agent'`) ise `pinnedRef` zorla `true` → en alta kayar. Peer/inbox
  mesajları + streaming asistan delta'ları eski pin davranışına saygı gösterir.
- **Pinned `TodoPanel` — kutu yok ama opak:** composer üstündeki pinned "Görev Listesi"
  tepsisinden **iç bubble-kutusu** kaldırıldı (`bg-surface-2`+`border` yok → düz görünür),
  ama **dış tray opak** tutuldu (`bg-[var(--color-bg)]`+`border-t`) → arkasındaki transkript
  **sızmaz** (şeffaf yapılınca içerik geçiyordu; opak zemin bunu keser). Transkript içindeki
  katlanabilir iz kartları (`TodoCard`/`DiffCard`/`ThinkingBlock`/`TextStep`) arka-planını
  korur + **`shadow-sm` gölge** eklendi. `tsc` + prod build yeşil.

## Artifact gruplama: Skills paritesi (katlanabilir gruplar + toplu grup atama) ✅ (2026-07-07)

**İstek:** Artifactları da skiller gibi gruplayabilelim.

**Ne yapıldı:**
- **Backend:** `Artifact` modeline first-class `Group string` alanı (`models_artifact.go`);
  `DB.SetArtifactGroup(ctx, id, group)` yalnız `group`'u yazar (`store_artifact.go`);
  `handleSetArtifactGroup` + rota `PUT /api/artifacts/{id}/group` (`artifacts.go`,
  `server.go`). Skill gruplamasından fark: skill'de grup frontmatter'da, artifact'ta
  entity JSON alanında (artifact'lar dosya-tabanlı entity).
- **Frontend:** `Artifact.group?` tipi + `api.setArtifactGroup`. `ArtifactsPanel`
  `useGroupedList` ile kovalanır (arama+origin filtresinden **sonra**): katlanabilir
  grup başlıkları, tümünü katla/aç, SelectionBar'da grup input'u (`datalist` önerili)
  + "Ata"/"Grupsuz" (Enter da uygular). `orderedIds` görünür (katlanmamış) sırayı
  izler → shift-aralık folded grupları atlar. Detay başlığında grup rozeti. Collapse
  durumu `tionswarm.artifactsCollapsedGroups`'ta kalıcı.
- **Doğrulama:** `go build ./...` + `go test ./internal/db ./internal/api` (135 passed)
  + frontend `tsc` yeşil. Docs: `45-COKLU-SECIM.md` (tablo + not), `SKILL.md` artifact satırı.

## Composer odağı: yalnız yeni sohbet açılınca input'a odaklan ✅ (2026-07-07)

**İstek:** Bir sohbet penceresi açınca input'a odaklanma **yalnız yeni sohbet
oturumu açılınca** olsun; mevcut oturumlara tıklayınca input otomatik odaklanmasın.

**Ne yapıldı (yalnız frontend):**
- **Kök neden:** `Composer.tsx` odak effect'i `[sessionId, disabled]`'e bağlıydı →
  Composer oturum geçişinde remount olmadığı için (`composerKey` yalnız rewind'de artar)
  **her** oturum değişiminde `taRef.focus()` çalışıyordu (mevcut oturuma tıklayınca da).
- **Çözüm:** `App.tsx`'e `focusSessionId` state'i eklendi; **yalnız** `newSession()`
  bunu yeni oturum id'sine set eder. Composer'a `focusSessionId` prop'u geçildi; odak
  effect'i artık yalnız `sessionId === focusSessionId` iken odaklanır. Mevcut oturum
  seçimi (sidebar) bu sinyali değiştirmediğinden odak çalınmaz. İlk açılışta (mevcut
  oturum gösterilir) `focusSessionId=null` → odak yok.
- **Rewind korundu:** `handleRewind` de `setFocusSessionId(activeSessionId)` yaparak
  remount sonrası imleci input'a bırakır (eski davranış). Frontend `tsc` yeşil.

## Skills ekranı: son düzenleme tarihi + grup içi recency sıralaması ✅ (2026-07-06)

**İstek:** Skills ekranında her becerinin son düzenleme tarihi görünsün ve skill
grupları içinde güncelden eskiye doğru sıralansın.

**Ne yapıldı:**
- **Backend:** `skills.Skill`'e `ModifiedAt int64` (`json:"modifiedAt"`, Unix saniye)
  alanı eklendi; `store.go scanDir` her `SKILL.md`'yi `os.Stat` ile damgalar (best-effort,
  stat başarısızsa 0). API `Skill`'i doğrudan serialize ettiği için ek endpoint gerekmedi.
- **Frontend:** `types/skill.ts` `modifiedAt?: number`; `SkillsPanel.tsx` listeyi
  `modifiedAt` DESC sıralayıp `useGroupedList`'e verir (hook grup-içi giriş sırasını korur →
  her grup en yeni düzenlenenden eskiye sıralanır). Liste öğesinde "Düzenlendi: <relatif>"
  satırı (`relativeTime`, hover'da tam tarih), detay panelinde "Son düzenleme: <tam tarih>".
- **Not:** `store.List()` prompt kataloğu için hâlâ ada göre sıralı — yalnız UI sunum
  sırası değişti. `go build` + frontend `tsc` yeşil.

## Pano (kart) tetikleyicili otomasyonlar ✅ (2026-07-06)

**İstek:** Otomasyonlara cron + etiket türlerine ek olarak, **Board'daki kart
değişimlerinde** çalışan bir tetik türü ekle — genel kart değişimlerini dinlesin;
bir kart bir board'a taşınınca ajan veya flow çalışabilsin.

**Ne yapıldı:** Aynı `Automation` entity'sine ikinci bir tetik türü eklendi
(`TriggerKind`: `""`/`tag` varsayılan, `board` yeni). Board türü alanları:
`BoardOp` (`move` vars./`create`/`update`/`delete`/`any`), `BoardFromState`,
`BoardToState` (sütun filtreleri).

- **Tek nokta tetik (db hook):** UI ve ajan araçları kart mutasyonlarını hep
  `db` katmanından (`CreateTask`/`MoveTask`/`UpdateTask`/`DeleteTask`) geçirdiği için
  gözlemci `DB.SetBoardHook`/`BoardChangeEvent` ile **db seviyesine** kondu; kilit
  bırakıldıktan sonra çağrılır, manager onu ayrı goroutine'de `OnBoardChange`'e bağlar
  (mutasyon bloklanmaz). Move yalnız sütun **gerçekten** değişince ateşler; update
  sütun değiştiyse `move` aksi halde `update`.
- **Motor:** `AutomationEngine.OnBoardChange` + `boardMatches` + `fireBoard` +
  `boardVars` (`{{taskId}}/{{title}}/{{op}}/{{from}}/{{to}}/{{toLabel}}/{{board}}` …).
  Guardrail bloğu `guardsPass`'e çıkarılıp tag/board yolları paylaşır. Board otomasyonu
  kendini döngülemez (spawn'lanan oturum etiket taşımaz); MaxIterations/Cooldown sınırlar.
- **API + araçlar:** `automationReq` + `create/update/list_automation` yeni alanları alır;
  board türü `triggerTag` istemez, `boardOp` doğrulanır; update kısmi patch'te `triggerKind`
  verilmezse dokunulmaz (board→tag kazara dönüşümü önlenir).
- **UI — birleşik 3 SEKME (2026-07-07 güncelleme):** Otomasyon ekranı tek bir tab bar altında:
  **⏰ Zamanlamalar (cron)** · **🏷 Etiket otomasyonları** · **🗂 Pano otomasyonları** (canlı sayaç
  rozetleri). Tab state `Schedules.tsx`'te; `schedules` sekmesinde cron başlık+form+liste, diğer
  sekmelerde `Automations`. `Automations` **kontrollü** hâle geldi (`activeKind: 'tag'|'board'|null`;
  `null` → render yok ama mount kalır → `onCounts` ile sayaçlar canlı). Her `AutomationSection`'ın
  kendi formu/listesi/edit state'i; tür toggle'ı yok. Board bölümünde olay + kaynak/hedef sütun
  seçicileri; ortak `PromptVarsField`; board satırında `🗂 <op> (…→…)` çipi.
- **Test:** `boardMatches`/`boardVars` (agent) + `store_task_hook_test.go` (db hook 5 olay).
  `go build ./...` + `go test ./internal/db ./internal/agent` (175) + frontend `tsc` + prod build yeşil.
- **Dok:** `_Docs/46-ETIKET-OTOMASYON.md` §2.5 + intro; SKILL.md otomasyon maddesi.

## Dar ekranda liste drawer'ı seçim yoksa otomatik açılır (Aktivite + Akışlar) ✅ (2026-07-07)

**İstek:** Activities ekranına girince hiçbir aktivite seçili değilse dar
ekranlarda aktivite listesi paneli otomatik açılsın; aynısı Akışlar ekranında
flow listesi için.

**Ne yapıldı:** `ExecutionsPanel` ve `FlowsPanel`'e mount-once `useEffect` — seçim
yoksa (`!selectedId`) `useCollapsibleList.setOpen(true)` ile liste drawer'ı açılır.
`useCollapsibleList.open` yalnız dar ekranlarda etkili (md+ CSS ile hep görünür) →
otomatik açılma sadece dar ekranı etkiler, geniş ekranda no-op. Paneller view'e
göre remount olduğundan effect her girişte çalışır. `frontend tsc --noEmit` temiz.

## Ajanlar kullanıcının görevlerini de silebilir (delete_task köken kısıtı kaldırıldı) ✅ (2026-07-06)

**İstek:** Boards (kanban) ekranındaki görevleri ajanlar da silebilsin —
kullanıcının oluşturdukları dahil.

**Ne yapıldı:** `builtin_taskmgmt.go` `delete_task` aracındaki köken (provenance)
kısıtı kaldırıldı. Önceden `cur.CreatedBy == ""` (kullanıcı görevi) ise silme
reddediliyordu; artık **her görev** silinebilir (yalnız var-olma kontrolü kalır;
yok id → net hata, sessiz no-op değil). Açıklamalar + dosya-başı güvenlik yorumu +
`list_tasks` açıklaması güncellendi (`CreatedBy` yalnız provenance/gösterim için
damgalanmaya devam eder). Test `TestDeleteTaskGuard` → `TestDeleteTaskAny` (kullanıcı
+ ajan görevi silinebilir + yok-id hatası). Self-management default SKILL + proje
skill notu güncellendi. `go build ./...` + `go test ./internal/tools -run Task` (4)
yeşil.

## Chat başlığı = oturum title + ayrı "Debug" paneli ✅ (2026-07-06)

**İstek:** (1) Chat header'ında "Sohbet · Manager" yerine oturumun **title**'ı
yazsın. (2) Sağdaki "Bağlam" butonunun yanına **"Debug"** butonu. (3) "Oturum
bilgisi" içindeki Debug parçalarını **yeni bir panele** taşı; title'daki Debug
butonuyla açılsın.

**Ne yapıldı (frontend):**
- **Başlık:** `App.tsx` chat header artık `VIEW_TITLE`+ajan yerine `sessions.find(
  ...).title` gösterir (fallback: ajan adı → "Yeni sohbet"). Diğer view'ler
  değişmedi.
- **Debug butonu:** header'da "Bağlam" ile "Detay" arasına `Bug` ikonlu buton →
  `setDebugOpen(true)`.
- **Yeni panel:** `SessionDebugModal.tsx` — `ModalOverlay` içinde başlık + kapat;
  gövdede `SessionDebugCard`'ı **`alwaysOpen`** modunda render eder (iç katlama yok,
  chrome'u modal verir). `SessionDebugCard`'a `alwaysOpen` prop'u eklendi (açık
  başlar, katlama başlığı gizlenir).
- **Taşıma:** `SessionDetailPanel`'den `SessionDebugCard` (+import) kaldırıldı;
  Debug artık yalnız modalda. Modal `agents`'tan agentId→name map'i alır (viz
  şerit/düğüm etiketleri). `frontend tsc --noEmit` temiz.

## "Oturum bilgisi" paneli genişletilebilir (drag-resize) ✅ (2026-07-06)

**İstek:** Sağdaki "Oturum bilgisi" paneli (SessionDetailPanel) sabit `w-80`
genişlikteydi; sürükleyerek genişletilebilir olsun.

**Ne yapıldı:**
- **`useResizableSidebar` hook'una `invert` seçeneği:** sağ-taraf paneli sol
  kenardan sürüklendiğinde (clientX azalırken) **büyümesi** için delta ters
  çevrilir. Deps'e eklendi.
- **`ResizeHandle`'a `side` prop'u:** `'right'` (varsayılan, sol-liste kolonları)
  veya `'left'` (sağ panel). `left-0`/`right-0` konumlandırma.
- **SessionDetailPanel:** sabit `w-80` → `useResizableSidebar({storageKey:
  'tionswarm.sessionInfoWidth', default 320, min 280, max 640, invert:true})` +
  inline `style.width`. Sol kenarda `ResizeHandle side="left"`. Handle içerik
  kaydırılınca kaymasın diye aside `overflow-hidden` yapıldı, scroll **iç
  sarmalayıcıya** taşındı (ListPane deseni). Mobil çekmece için genişlik
  `max-md:!w-[85vw] max-md:!max-w-sm` ile bağlandı (inline stili `!important`
  ezsin diye). Genişlik `localStorage`'da kalıcı. `frontend tsc --noEmit` temiz.

## İş akışı görselleştirmeleri: Araç Sankey + Eşzamanlılık zaman çizelgesi ✅ (2026-07-06)

**İstek:** CCAM'in Workflows ekranındaki "Tool execution Sankey" ve "Concurrency
timeline" görselleştirmelerini TionSwarm'a ekle.

**Ne yapıldı (yalnız frontend; mevcut `debug.jsonl` verisinden, ekstra backend yok):**
- **Veri katmanı (saf):** `frontend/src/components/sessions/viz/flowVizData.ts` —
  `buildToolSankey` (tool olaylarını `Ajan → Araç → Tamam|Hata` mermaid `sankey-beta`
  koduna toplar; hata dalı yalnız hata varsa) + `buildConcurrencyTimeline` (zaman
  damgalı `llm_call`/`turn` olaylarını ajan-şeritli bar modeline; olay COMPLETION'da
  damgalandığı için bar = `[ts−durMs, ts]`; sweep-line ile şeritler-arası çakışma =
  `hasOverlap`). Yan-etkisiz → test edilebilir.
- **Sankey bileşeni:** `viz/ToolSankey.tsx` — mevcut `MermaidDiagram`'ı (lazy mermaid,
  tema-duyarlı, expand) `sankey-beta` koduyla besler.
- **Zaman çizelgesi:** `viz/ConcurrencyTimeline.tsx` — bağımlılıksız SVG Gantt
  (Sparkline desenine uygun); ajan başına şerit, zaman-eksenli barlar (hata=kırmızı),
  3 eksen tick'i, seri/eşzamanlı rozeti, `<title>` tooltip.
- **Kapsayıcı:** `viz/SessionFlowViz.tsx` — katlanabilir "İş akışı görselleştirmeleri"
  bölümü; açılınca olayları lazy çeker (limit 500, truncation notu), iki görseli üst
  üste render eder. Durum `localStorage`'da (`tionswarm.flowVizOpen`).
- **Bağlama:** `SessionDebugCard`'a `agentNames` opsiyonel prop + model dökümünden
  sonra `SessionFlowViz` render; `SessionDetailPanel` `info.agents`'tan agentId→name
  map'i geçirir. Yer: **oturum detay panelinin Debug bölümü**.
- **Not:** Tek-ajan seri oturumda zaman çizelgesi "seri" görünür; koordinatör/çok-ajan
  oturumlarda şerit-çakışması gerçek eşzamanlılığı gösterir. `frontend tsc --noEmit`
  temiz (lint `set-state-in-effect` kuralı repo-genelinde mevcut, idioma uygun).

## Fix: Yürütme/oturum listesi sırası her poll'de değişiyordu ✅ (2026-07-06)

**Belirti:** Aktivite ekranındaki yürütme listesi (ve sol oturum listesi) "durduk
yere" sürekli yeniden sıralanıyordu.

**Kök neden:** `DB.ListSessions` kaynak `d.sessions` **map**'i üzerinde dönüyor →
Go map iterasyon sırası her çağrıda rastgele. Sıralama yalnız `(Pinned, UpdatedAt
desc)` idi; **eşit `UpdatedAt`** (saniye-hassasiyetli damga, aynı anda güncellenen
oturumlar) olan satırlar `SliceStable`'da rastgele gelen map sırasını koruyordu →
her 5 sn'lik poll'de yer değiştirme.

**Düzeltme:** İki sıralamaya da kararlı **ID tie-break** eklendi:
`store.go ListSessions` (`ID` desc) + `api/executions.go handleListExecutions`
(`SessionID` desc). Artık eşit-zamanlı satırlar sabit sırada. `go build ./...` yeşil.

## Oturumlar toplu tablo görünümü (Aktivite ekranı) ✅ (2026-07-06)

**İstek:** Claude-Code-Agent-Monitor'ün "Sessions" ekranı gibi tüm oturumları
tek tabloda topluca (aranabilir/filtrelenebilir/sıralanabilir) görebilmek. Aktivite
ekranında yürütme listesinin en üstünde bu tabloyu açan bir buton olsun.

**Ne yapıldı:**
- **Paylaşılan meta çıkarıldı:** `frontend/src/components/panels/executionsShared.tsx`
  — `KIND_META`/`FILTERS`/`kindMeta`/`shortId`/`StatusPill` `ExecutionsPanel`'den
  buraya taşındı; hem panel listesi hem yeni tablo aynı kaynağı kullanır (kind rozeti/
  filtre/kimlik-kısaltma/durum pill'i birebir aynı).
- **Yeni bileşen:** `SessionsOverview.tsx` — `ModalOverlay` içinde geniş tablo.
  Kolonlar: Başlık (canlı/okunmadı noktası + kopyalanabilir kısa kimlik), Tür, Ajan
  (avatar), Mesaj, Durum (çalışıyor/başarılı/hata), Süre (bitmiş=created→updated,
  canlı=created→now), Oluşturma, Güncelleme. Kolon başlığına tıkla → sırala (aynı
  kolon yön çevirir; sayısal/tarih kolonları azalan varsayılan). Arama başlık/kimlik/
  ajan üzerinde; kind filtre sekmeleri. Ekstra fetch YOK — panelin zaten yüklü
  `listExecutions()` verisini kullanır. Satıra tıkla → o yürütmeyi seç + overlay kapan.
- **Panel entegrasyonu:** `ExecutionsPanel` "Yürütmeler" başlığına `Table2` ikonlu
  buton (`data-testid="sessions-overview-open"`) + `overviewOpen` state + koşullu
  overlay render. `frontend tsc --noEmit` temiz.

## Capability Probe → Context Genişletme + per-workspace codebase-memory store ✅ (2026-07-06)

**İstek:** Cihazda `codebase-memory-mcp` **mevcutsa** yeni oturumların sistem
promptuna **kısa + cachelenebilir** bir bilgi bloğu enjekte et → ajan varlığını
bilsin ve workspace indeksleme/arama araçlarını kullansın. Tespit + genişletme
katmanı **generic** olsun (ileride başka tool'lar tek kayıtla eklenebilsin). Ayrıca
her workspace kendi **izole** codebase-memory store'unu kullansın. Tam tasarım: **_Docs/54**.

**Ne yapıldı:**
- **Generic katman:** `internal/agent/capabilities.go` — `Capability{ID, Detect, Context}`
  + `[]capabilities` kaydı + `(*Runtime).CapabilityContext(ctx, cwd)`. `Detect` MCP-sunucu
  varlığı / on-PATH binary / settings-flag olabilir → yeni tool = slice'a bir kayıt.
- **İlk müşteri codebase-memory:** enabled stdio MCP sunucusunun `Command`'ında
  `codebase-memory-mcp` işareti aranarak tespit; kısa statik blok (araç-tercihi + izole
  store + cwd'den türetilen `project` id, `projectIDForPath`). Path→id kuralı
  codebase-memory ile birebir (iki gerçek örnekle test edildi), ıskalarsa `list_projects`
  hedge'i.
- **Enjeksiyon (iki senkron assembler):** chat `api.composeTurnRequest` statik prefix'e
  (cwd/project id'li) + headless `agent.autonomousSystemPrompt` (cwd-siz farkındalık).
  Cache breakpoint bozulmaz (presence sabit; sunucu yokken blok "" → no-op).
- **Per-workspace store (§C):** `(*Runtime).CBMStoreDir()` = `<workspace-container>/cbm-store`
  (skills/hook-scripts kardeşi). `toolsetup.go` codebase-memory sunucusunun stdio env'ine
  `CBM_CACHE_DIR`'i enjekte eder (user-set kazanır) → store = workspace sınırı, indeksler
  karışmaz.
- **Auto-index (§C):** `(*Runtime).EnsureCodebaseIndexed(ctx, cwd)` — best-effort, arka
  plan, `(cwd,store)` başına süreç-içi tek sefer (`cbmIndexed sync.Map`); `command cli
  index_repository` + `CBM_CACHE_DIR`. Hata **loglanır** (yutulmaz), guard silinip sonraki
  tur retry olur.
- **Test:** `capabilities_test.go` — `projectIDForPath` (gerçek örnekler), `codebaseMemoryCommand`
  (stdio-match/http-skip/absent), `CBMStoreDir`. `go build ./...` + `go test ./internal/agent`
  yeşil.
- **Takip (aynı gün):** (1) **Headless project id** — `autonomousSystemPrompt(ctx,a)` +
  `sessionCwd(ctx)` → headless turlar da cwd/project id + auto-index alır (executor+subagent
  ctx'li çağrı). (2) **Workspace-geneli arama** — `codebase_workspace_search`
  (`builtin_codebase_search.go`): `list_projects`→her project'e `search_code` fan-out,
  project'e göre birleşik sonuç (`limit` 30 / `maxCodebaseProjects` 40); `RiskRead` +
  `CategorySearch`; yalnız enabled codebase-memory sunucusu varsa kayıtlı. (3) **Frontend** —
  `ToolsPanel.tsx` MCP sunucu satırında **"izole store"** rozeti+tooltip
  (`data-testid="mcp-server-isolated-store"`). `go build`/`go test ./internal/agent
  ./internal/tools` + `npm run build` yeşil.
- **Aç/kapa toggle + canlı duman testi (2026-07-07):** Tüm yetenek workspace ayarıyla
  açılıp kapanır (default açık): `WSSettings.CodebaseMemoryEnabled` → `Runtime` atomic gate →
  `codebaseMemoryCmd` tek choke-point (kapalı → hint/auto-index/tool/env hepsi kaybolur).
  UI: `WorkspacePanel` Toggle + `ExternalToolsPanel` callout ("Ayarlar ▸ Bu Workspace"). **Smoke:**
  CLI kontratı (izole store index→list→search fan-out) + binary boot (panic yok) + canlı toggle
  round-trip (default true→PUT false→re-GET false→PUT true). Test, `workspaceSettingsDTO`'da
  **eksik alan** bug'ını yakaladı → GET her zaman boş dönüyordu; DTO'ya eklenip giderildi.
  `go test ./internal/{agent,tools,workspace,api}` + `npm run build` yeşil.

## `archive_sessions` (workspace-scoped toplu oturum arşivleme) ✅ (2026-07-06)

**İstek:** Bir oturum incelemesinde ajanın "başka oturumları temizle" isteğinde
`update_session` yalnız **mevcut** oturumu arşivleyebildiği için **ham REST**'e
(`Invoke-RestMethod /api/sessions/{id}/state`) düştüğü, `X-Workspace-Id` header'ı
verilmeyince **yanlış (Default) workspace'i** arşivlediği tespit edildi. Bu boşluğu
kapatan workspace-scoped toplu-arşiv aracı gerekiyordu.

**Ne yapıldı:**
- **Araç:** `internal/tools/builtin_sessionarchive.go` `archive_sessions` — `r.db`'ye
  bağlı (fiziksel workspace scope), **mevcut oturumu daima hariç tutar**
  (`SessionIDFrom(ctx)` build anında). Filtreler: `idle_days` (N günden eski),
  `title_contains`; `exclude` (ek koru), `dry_run` (önizleme), `limit` (vars. 100
  güvenlik tavanı). Yalnız `Kind=="chat"` + `State=="active"` hedefler; arşiv
  soft/geri alınabilir (`SetSessionState(...,"archived")`).
- **Kayıt:** `toolsetup.go` — `list_sessions`'ın yanına, `SessionContextEnabled()`
  gate'i altında eager. `categories.go` → `CategoryAgents`. Risk sınıfı listelenmedi
  → varsayılan `RiskWrite` (mutasyon; "ask" modda onay ister).
- **Test:** `builtin_sessionarchive_test.go` (3 test): current+non-chat+archived
  hariç tutma & scope; `dry_run` değiştirmez; `title_contains` + `idle_days` filtre.
  `go build ./...` + `go test ./internal/agent ./internal/tools` yeşil.
- **Doküman/skill:** `24-SELF-MANAGEMENT.md` yeni "Oturum yönetimi (workspace-scoped)"
  satırı; `tionswarm-project` + `tionswarm-session-debug` skill'leri (yeni §7 kök-neden).

## `render_template` + `html-preview` (şablonlu HTML render) ✅ (2026-07-06)

**İstek:** the external agent project'ın "Source Templates / `render_template`" özelliğini TionSwarm'a taşı —
motor markalı HTML şablonu doldurur, modele **yalnız dosya yolu** döner (HTML değil → token
tasarrufu), sohbette **inline izole iframe**'de gösterilir. Tam tasarım: **_Docs/53**.

**Ne yapıldı:**
- **Motor:** Go `html/template` (auto-escape/XSS-güvenli). `internal/tools/render_template.go`
  (saf: render + sidecar `.meta.json` oku + missing-field), `builtin_render_template.go` (Tool),
  `builtin_render_template_test.go` (8 test: escape, soft-warn, hard-fail'ler, traversal, no-session).
- **Session çıktı dizini:** `internal/agent/renderdir.go` `SessionRenderDir` = `<db.Root>/render/<sid>`
  (`progress/`'in kardeşi). Session yoksa boş → araç hard-fail.
- **Kayıt:** `toolsetup.go` builtins + `MarkNameOnly("render_template")`. Shell gate GEREKTİRMEZ (saf render).
- **Servis:** `internal/api/files.go` `serveTextFile` — `GET /api/files?path&as=text` yalnız render
  kökü altını (`underDir` whitelist) `text/plain`+`nosniff` ile döndürür (asla `text/html`).
- **Inline UI:** `HtmlPreview.tsx` (```html-preview``` → `<iframe srcDoc sandbox="allow-scripts">`,
  opaque origin, tab desteği) + `CodeBlock.tsx` dispatch + `attachments.tsx` `fileTextURL`.
- **Şablonlar:** skill-bundled `tionswarm-templates` (report + email şablonu + meta). "Template store"
  alt-sistemi KURULMADI (source kavramı yok; `${SKILL_DIR}` yeterli).
- **Soft vs hard:** eksik `requiredField` → render + warning; bozuk template/JSON/sidecar → hata.
- **Prompt + guard:** "## Rendering"e `html-preview`/`render_template` eklendi; `defaults_test.go`
  bu ikisini yasak listesinden çıkardı (artık TionSwarm-native).
- **Ömür/temizlik (aynı gün eklendi):** render çıktısı iki katmanda toplanır —
  `deleteSessionFilesLocked` session silmede `<render>/<sid>`'i siler; `DB.Open` → `cleanupRenders`
  startup'ta orphan dizinleri + `renderTTL` (14g) üstü eski dosyaları temizler, boş dizini budar.
  Yol tek kaynak `db.RenderDir(sid)`. Dosyalar: `internal/db/render_cleanup.go` + 3 test.

**Doğrulama:** `go build ./...` ✓, `go vet` ✓, tools+agent 304 test ✓, db+agent 169 test ✓ (render sweep 3/3),
workspace+skills 35 test ✓, `tsc` ✓.
**AÇIK:** UI'da gerçek bir render'ın görsel doğrulaması (iframe + `as=text` fetch) canlı denemeyle yapılmalı.

## Uygulama-içi claude-cli OAuth login (tarayıcıyla, popup senkron) ✅ (2026-07-06)

**İstek:** Terminal açmadan, popup'tan tıklayarak Max/Pro girişi — link verilir, tarayıcı
açılır, popup'a koddan yapıştırılır, arka planda kimlik senkron yazılır.

**Yaklaşım B (native PKCE):** `claude setup-token`/`auth login` interaktif TUI'sini sürmek
yerine OAuth authorization-code + PKCE akışını **Go'da kendimiz** kurduk. Sabitler kurulu
CLI binary'sinden (v2.1.201) + public client-metadata dokümanından çıkarıldı (domain'ler
`platform.claude.com`/`claude.com/cai`'ye taşınmış — eski `console.anthropic.com` değil):
- client_id `9d1c250a-e61b-44d9-88ed-5944d1962f5e`, authorize `claude.com/cai/oauth/authorize`,
  token `platform.claude.com/v1/oauth/token`, redirect `platform.claude.com/oauth/code/callback`, S256.

**Parçalar:**
- `internal/claudeauth/oauth.go` — `Begin()` (PKCE verifier/state + authorize URL),
  `Exchange()` (kod→credential, state doğrulama, `code#state` parse), `WriteCredentials()`
  (`<home>/.credentials.json`'a `claudeAiOauth{...}` atomik yaz, .bak yedek). Birim test 3/3.
- API: `POST /api/workspace-settings/claude-auth/oauth/{start,complete}` — start URL+flowId
  döner (verifier server-side stash, 10dk TTL), complete kodu exchange edip **aktif
  workspace'in claude-home'una** yazar (refresh token'lı → CLI kendi tazeler).
- Frontend: `ClaudeAuthDialog`'a **"Tarayıcıyla giriş"** sekmesi (varsayılan) — "Giriş başlat"
  → URL aç → `kod#state` yapıştır → "Girişi tamamla" → ✓. Manuel token-paste + API-key
  sekmeleri yedek kaldı. `api.startClaudeOAuth`/`completeClaudeOAuth`.

**AÇIK DOĞRULAMA:** Canlı token-exchange (platform.claude.com'a gerçek POST) yalnız gerçek
bir login ile doğrulanabilir — sabitler doğru ama scope/param ince ayarı gerekirse tek dosya
(`oauth.go` const bloğu). Go build+vet+78 test yeşil, tsc temiz.

**Loopback (paste'siz) varyant ✅ (2026-07-06):** İkinci akış eklendi —
`http://localhost:<port>/callback` redirect'ini kullanır (`LoopbackConfig`). Backend efemeral portta yerel dinleyici açar (`claude_oauth_loopback.go`);
tarayıcı yetkilendirmeden sonra doğrudan geri döner, callback handler kodu exchange edip
credential'ı yazar, tarayıcıya HTML başarı sayfası basar. Popup `.../oauth/loopback/status`'ı
poll eder → paste GEREKMEZ. `oauth.go` `Begin`→`BeginWith(FlowConfig)` refaktörüyle iki akış
tek çekirdeği paylaşır (`PendingLogin` client/redirect taşır). Popup'ta "Otomatik (önerilen)"
vs "Elle kod" alt-modu; otomatik varsayılan. Yalnız tarayıcı backend ile aynı makinedeyken
(masaüstü/yerel) çalışır — uzak/LAN'da manuel-paste'e düşülür.

**Loopback client_id fix ✅ (2026-07-06):** Otomatik (loopback) akış tarayıcıda "OAuth Request
Failed — client_id: Input should be a valid UUID, found `h` at 1" hatası veriyordu. Sebep:
`LoopbackConfig` client_id olarak metadata-doküman URL'i (`https://claude.ai/oauth/
claude-code-client-metadata`) gönderiyordu, ama `claude.com/cai/oauth/authorize` endpoint'i
client_id'yi **UUID** olarak doğrular ve URL'i (`https`'in `h`'sinden) reddeder. Düzeltme:
loopback artık manuel akışla **aynı public UUID client**'ı (`9d1c250a-…`) kullanır. Ayrıca
redirect_uri `127.0.0.1` → **`localhost`** yapıldı: authorize endpoint 127.0.0.1'i localhost'a
normalize ediyor; token-exchange redirect_uri'si normalize edilmiş biçimle eşleşmezse
"redirect mismatch" olurdu. Yerel dinleyici hâlâ 127.0.0.1'e bind (tarayıcı localhost'u ona
çözer). Canlı doğrulama: UUID+localhost authorize URL'i artık UUID hatası vermiyor (yalnız
oturumsuz 403). Tek dosya: `internal/claudeauth/oauth.go` (`loopbackClientID` const kaldırıldı).

**Rebuild+test (2026-07-06):** `go build ./cmd/tionswarm` (binary + gömülü dist) ✓,
`go vet ./...` ✓, `go test ./...` **673 test / 34 paket** ✓, frontend `npm run build` ✓, tsc temiz.

## Oturum bilgisi panelinde arka-plan süreç kontrolü (gör/durdur/yeniden başlat/tazele) ✅ (2026-07-06)

**İstek:** Bir sohbetin arkasında çalışan claude-cli/provider işlemini "Oturum bilgisi"
panelinde görebilmek ve durdurma/yeniden başlatma yapabilmek. 3 tier uygulandı:

**Tier 1 — Gör + Durdur (backend-driven, detached/autonomous turlar için de sağlam):**
- `chatRun`'a `startedAt` + `provider` (setter `setProvider`, `chat_stream` agents[0]'dan
  doldurur); `chatRuns.sessionRunInfo(sid)` snapshot döndürür.
- `sessionInfoResp`'e `running {runId, startedAt, autonomous, provider}` + `warmCliProcess`.
- Panelde canlı "Tur çalışıyor" kartı (geçen süre sayacı, provider rozeti) + **Durdur** →
  mevcut `POST /api/chat/control {action:stop}` → `run.cancel()` → `exec.CommandContext`
  claude.exe'yi öldürür. Panel `running`/`warmCliProcess` varken 3sn'de bir poll eder.

**Tier 2 — Yeniden başlat:** chat hook'una `rerunLast()` (son asistan turunu `retryMessage`
ile tekrar; yoksa son user mesajını yeniden gönder) → App `onRerun` ile panele bağlar.
Panel önce backend runId ile durdurur, sonra yeniden gönderir (tek gerçek tur yolu korunur).

**Tier 3 — Persistent süreç tazeleme:** `CLISessionPool`'a `AliveForSession`/`DropSession`
(pool anahtarı `<sid>|<agentID>` prefix eşleşmesi) + Runtime `HasWarmCLISession`/
`DropWarmCLISession` + `DELETE /api/sessions/{id}/cli-process`. Panelde "Sıcak claude-cli
süreci" satırı + **Tazele** → sonraki tur cold-restart (konuşma korunur). Yalnız
persistent-pool modunda görünür.

**Dosyalar:** `internal/api/{chat_control,chat_stream,session_info,sessions,server}.go`,
`internal/agent/runtime.go`, `internal/providers/claudecli_session.go`,
`frontend/src/{types/session.ts, api/sessions.ts, hooks/useChatStream.ts,
components/sessions/SessionDetailPanel.tsx, App.tsx}`. Go build+vet+159 test yeşil, tsc temiz.

## claude-cli auth hatası: sınıflandırma + ön-uçuş probe + terminal etiket ✅ (2026-07-06)

**Problem:** claude-cli sağlayıcılı bir ajanın workspace claude-home'u giriş yapmamışsa
tur ilk LLM çağrısında `authentication_failed` / "Not logged in · Please run /login"
ile reddediliyordu. Bu hata **jenerik `claude CLI failed: exit status 1`'e** düşüyor,
`retryable=true` ile **boşuna 2. kez** deneniyor, kullanıcıya hangi claude-home'un login
gerektirdiği söylenmiyordu. (Rate-limit için çözülmüştü, auth atlanmıştı.)

**5 maddelik çözüm:**
1. **Sınıflandırma (retry-EDİLMEZ):** `claudecli.go` yeni `isAuthErrorText` +
   parser `notLoggedIn`/`authMsg` alanları; `feed()` hem standalone
   `{"error":"authentication_failed"}` satırını (yeni `cliEvent.Error`) hem result-error'ı
   yakalar. `runAttempt` rate-limit dalının üstünde net, retry-edilmez auth hatası döndürür.
2. **Aksiyon mesajı:** Hata metni ilgili `CLAUDE_CONFIG_DIR=<claude-home>` yolunu + "`claude
   /login` çalıştır veya API-key sağlayıcıya al" önerisini gömer.
3. **Ön-uçuş probe:** `ClaudeCLI.ProbeAuth(ctx)` (araçsız minimal `claude -p`, aktif
   workspace'in claude-home'unu test eder) + `GET /api/workspace-settings/claude-auth`
   (`handleWorkspaceClaudeAuth`) + Ayarlar→Sağlayıcılar'da **"Bu workspace login doğrula"**
   butonu (`ProvidersPanel.tsx`, `api.checkWorkspaceClaudeAuth`). Not: jenerik "Test et"
   global config dir'i dener; bu probe workspace-özeldir.
4. **Terminal etiket:** `autotag.go` yeni `TagAuthError = "auth-error"` — auth hatası
   turlarına `error`'a ek olarak eklenir; auto-repair otomasyonu bunu **dışlamalı**
   (login onarılamaz, aksi halde MaxIterations'a kadar boşuna döner).
5. **Doküman:** `tionswarm-session-debug` skill'ine "I) authentication_failed" deseni +
   ön-uçuş probe reçetesi eklendi.

**Dosyalar:** `internal/providers/claudecli.go`, `internal/agent/autotag.go`,
`internal/api/workspace_settings.go` + `server.go` (route), `frontend/src/api/workspaces.ts`,
`frontend/src/components/settings/ProvidersPanel.tsx`. Go `build ./...` + `tsc --noEmit` yeşil.

## Schedule manuel "Run" turu sayfa yenileyince kesiliyordu ✅ (2026-07-05)

**Bug:** Bir schedule'ı elle "Run" ile çalıştırıp (senkron `POST /api/schedules/{id}/run`)
tur devam ederken sayfayı yenileyince/başka yere gidince tur **yarıda kalıyordu**
(yarım asistan cevabı persist, `lastDeliveryStatus=success`, hata bayrağı yok).

**Kök neden:** `handleRunSchedule` → `Scheduler.RunNow(r.Context(), id)` turu **istek
context'ine** bağlıyordu. Tarayıcı yenileme POST'u abort eder → `r.Context()` iptal →
`deliverPrompt`/`invokeTraced`/claude-cli turu üretim ortasında iptal. (Cron-fire zaten
`context.Background()` ile detach; sadece manuel Run bağlıydı.)

**Fix:** `handleRunSchedule` artık turu istekten ayırıyor:
`context.WithTimeout(context.WithoutCancel(r.Context()), 10*dk)` — chat-stream detach'ı
gibi. Böylece yenileme/navigasyon turu kesmiyor; 10dk güvenlik timeout'u hung run'ı sınırlar.

**Doğrulama (empirik):** eski binary'de RunNow'ı 12sn'de abort → tur 12sn'de kesildi
(out=4, partial). Fix'li binary'de aynı abort → tur detached tamamlandı (2045 char, 197sn).
Kesintisiz RunNow zaten tam çalışıyordu (175sn, tam plan + artifact). Dosya:
`internal/api/schedules.go`.

**Flow tarafı (kontrol edildi, değişiklik gerekmedi):** 4 flow-run endpoint'i
(`handleRunFlow`/`handleRunFlowStream`/`handleSessionRunFlow`/`handleSessionRunFlowStream`)
zaten `context.WithoutCancel` ile detached (`flows.go`). **Flow-backed schedule** manuel
Run'ı aynı `Scheduler.run(ctx)` → `deliverFlow` yolundan geçtiği için bu fix onu da
kapsar. Cron/automation/agent-tool flow yolları zaten server-side (istemciye bağlı değil).
Tek boşluk prompt-backed schedule manuel Run'dı.

## İlk kurulum: "Mevcut Workspace Seç" butonu ✅ (2026-07-05)

Onboarding ekranına (hiç workspace yokken) "Workspace Oluştur"un yanına ikinci
buton eklendi: **Mevcut Workspace Seç** → native klasör seçici → seçilen klasör
geçerliyse (içinde `store/` var) taşınmadan kayıt defterine eklenip aktifleşir.

- **Backend:** `Manager.Attach(path)` + `isWorkspaceDir`/`workspaceNameFromDir`/`sameDir`
  yardımcıları (`internal/workspace/manager.go`); `POST /api/workspaces/attach`
  (`internal/api/workspaces.go` + route `server.go`). Geçersiz/zaten-ekli klasör 400 +
  Türkçe mesaj. Yeni `WS<n>` id, `Meta.Path` doğrudan seçilen klasör; `open()` mevcut
  içeriği yerinde kullanır. Ad klasör adından (`tionswarm-` öneki soyulur); ikon/renk
  `ws-settings.json`'dan.
- **Frontend:** `api.attachWorkspace` (`api/workspaces.ts`),
  `useWorkspaces.attachWorkspace` (hata fırlatır → satır-içi göster),
  `OnboardingScreen` iki-buton düzeni + inline hata (`data-testid`:
  `select-existing-workspace` / `onboarding-error`), `App.tsx` `onAttach` prop.
- **Doğrulama:** `go build ./...` ve `tsc --noEmit` temiz. Detay: `06-WORKSPACES.md`
  "İlk Kurulum: Oluştur veya Mevcut Klasör Seç".

## UI: composer max-yükseklik + Budget dar-ekran uyumu ✅ (2026-07-05)

- **Composer textarea max-yükseklik:** `max-h-[5.5rem]` (~3 satır) → `max-h-[12rem]`
  (~7 satır); aşınca iç kaydırma. Doğrulandı: clientH=192px, scroll aktif.
- **Budget ekranı responsive:** özet kart satırları `flex` → `grid grid-cols-2
  lg:grid-cols-4`; Tasarruf Merkezi `grid-cols-3` → `grid-cols-1 sm:grid-cols-3`
  (divide yönü de responsive); Köken+Trend `grid-cols-2` → `grid-cols-1
  lg:grid-cols-2`; provider/ajan tabloları `overflow-x-auto` + `min-w` ile yatay
  kaydırılır; içerik dolgusu `p-5` → `p-3 md:p-5`. Doğrulandı: 430px'de yatay taşma
  0, özet 2 sütun; 1280px'de 4 sütun. Tema-uyumlu gradient de eklendi (composer).

## Sohbet: yüzen composer overlay + opak input + son mesaj görünürlüğü ✅ (2026-07-05)

Composer artık transkriptin **üstüne yüzen bir overlay** (App.tsx chat view'i
`relative` sarmalayıcı + `absolute bottom-0` bottom-stack). Böylece:

- **Gradient arka plan (tema-uyumlu):** composer sarmalayıcısı
  `bg-gradient-to-t from-[var(--color-bg)] via-[color-mix(...var(--color-bg)_85%...)]
  to-transparent` — mesaj balonları alttan geçerken şeffaf üst kısımdan görünüp arka
  plana karışarak composer'ın **arkasına kayar** (açık/koyu temada tutarlı).
- **Opak input:** iç input kartı `bg-[var(--color-surface)]` + `shadow-lg` — okunur,
  gradient'in üstünde net durur.
- **Son mesaj görünürlüğü (bug):** overlay'in canlı yüksekliği `ResizeObserver` ile
  ölçülüp `MessageList`'e `bottomInset` (scroll padding) olarak verilir → en yeni
  kullanıcı/asistan mesajı **daima opak input'un üstünde** kalır, "agent bitene kadar
  son kullanıcı mesajı görünmüyor" belirtisi giderilir. Ask/todo/pending/wake
  banner'ları da bu yüzen yığına taşındı (ölçüme dahil, mesajları örtmez).
- **Doğrulama (standalone Playwright, sistem Chrome):** kısa+uzun cevap boyunca
  kullanıcı mesajı `everHidden=false`; seri tool (`Glob/Glob/PowerShell`), subagent
  (`run_subagent`), `schedule_wake` (kuruldu **ve tetiklendi**, takip turu üretti)
  turları geçti; hepsinde `userShown=true`. Detay: `07-CHAT-UX.md`.

## Sohbet: tur-ortası reload'da canlı cevap kaybolması düzeltildi ✅ (2026-07-05)

Başka sohbete geçip geri gelince (veya sayfa yenileyince) **kendi penceresinin**
stream ettiği asistan cevabının kaybolması giderildi. Kök neden: `App.tsx`
session-değişim effect'i `listMessages` ile in-memory `live-*` balonu siliyordu;
`recoverInflight` ise pencere turu sahiplenmeye devam ettiği için (`runsRef`) erken
dönüyordu ve yerel SSE handler'ları `prev.map(id===liveId)` ile güncellediğinden
balon silinince sonraki delta'lar + final `onReply` sessizce düşüyordu.

- **Fix A (self-heal / UPSERT):** `useChatStream.ts` canlı metin+iz artık closure
  var'larında biriktirilir; `syncLive` balonu yoksa tam içerikle yeniden yaratır
  (`onStep`/`delta`/`onReply` map-only yerine upsert). `onReply` kalıcı mesajı
  farklı id ile append eder (map yerine) → reload sonrası da hayatta kalır.
- **Fix B (reseed):** yeni `liveBubblesRef` (session→canlı balon) + `reseedLive`;
  App effect'i `listMessages` sonrası çağırır → dönüşte balon anında geri gelir.
- **Fix C (metin ilerletme):** sahiplenilmeyen/refresh sonrası ghost balonun cevap
  metni donuyordu (bus `delta`'yı düşürür). `useChatStream` artık aktif session'da
  sahiplenilmeyen tur pending iken `/inflight`'i saniyede bir çekip yalnız `text`'i
  ilerletir (iz bus'a ait kalır); tur bitince poll durur.
- **Temizlik:** bırakılmış `[DBG:delta]` debug console.log'u kaldırıldı.
- Mimari refactor **gerekmedi** — backend zaten detached tur + `inflight.json` +
  `session_step` bus ile tek doğru kaynak. Detay: `07-CHAT-UX.md` (tur-ortası
  reload/navigasyon kurtarma bölümü).

## Memory alt sistemi kaldırıldı ✅ (2026-07-05)

2026-07-05 — Memory alt sistemi (journal recall + core memory + hafıza grafiği +
ilgili tool/API/UI/veri) tamamen kaldırıldı. Çıkarılanlar: `internal/memory` paketi;
ajan araçları `Remember`/`Recall`/`core_memory_replace`/`core_memory_append`/`reflect`;
API uçları `/api/agents/{id}/memories`, `/core`, `/reflect`, `/recall`, `memory-graph`;
frontend Hafıza ekranı/MemoryPanel/CoreMemoryCard/hafıza grafiği; ayarlar
`journalMinLen`/`recallMinScore`/`journalCap`/`journalMaxLen`/`autoReflectThreshold`
(+ `memoryPressureWarn`). Workspace Ağı / İlişki Grafiği'nin workspace tarafı
(`/api/graph`) korundu. Doküman güncellemeleri: `31-MEMGPT-CORE-MEMORY.md` (kaldırıldı
notu), `23-ILISKI-GRAFIGI.md` (yalnız Hafıza Bilgi Grafiği bölümü çıkarıldı),
`17-TOKEN-OPTIMIZASYON.md` (recall/journal kapıları), `00-GENEL-BAKIS.md`.

## Claude Sonnet 5 desteği ✅ (2026-07-05)

Yeni **Claude Sonnet 5** (`claude-sonnet-5`) modeli katalog + fiyat tablosuna eklendi.

- **Katalog:** `kind_anthropic.go` (`claude-sonnet-5`) + `kind_openrouter.go`
  (`anthropic/claude-sonnet-5`) manifestlerine "dengeli" etiketiyle eklendi; Sonnet 4.6
  "önceki dengeli" olarak yeniden etiketlendi. Katalog context-window / max-output
  değerleri aile tablosundan otomatik türer (`sonnet` substring → 1M bağlam, 32K çıktı,
  0.45 adaptif oran) → `context_window.go`/`thinking.go` değişmedi.
- **Fiyat:** `pricing.go` anthropic + openrouter haritalarına **$3/$15** (standart liste;
  giriş fiyatı $2/$10, 31 Ağu 2026'ya kadar) eklendi. OpenRouter satırı Anthropic
  pass-through cache tier'ı (0.10× okuma / 1.25× yazma) taşır.
- **claude-cli:** ayrı işlem gerekmedi — `sonnet` alias'ı aboneliğin sunduğu güncel
  Sonnet'i zaten çözer.
- **Varsayılan model Sonnet 5'e taşındı:** `anthropic.go DefaultModel` ve
  `kind_openrouter.go openrouterDefaultModel` → `claude-sonnet-5`
  (`anthropic/claude-sonnet-5`). Ingest adapter `mapCCModel` `sonnet` ailesi →
  `claude-sonnet-5` (CC subagent import'u); örnek snippet'ler (Reviewer ajanı,
  settings patch örneği) da güncellendi.
- **Fast varyantı EKLENMEDİ (bilinçli):** Fast mode yalnızca Opus sınıfına özgü;
  Anthropic Sonnet 5 için Fast tarifesi yayınlamadı → uydurma model eklenmedi.
- `go build ./...` ✅ · providers/ingest/billing/tools/api/settings **349 test geçti**
  · yeni `TestPriceFor_Sonnet5` regresyon kilidi.

## Per-workspace claude-cli config evi (birleşik config) ✅ (2026-07-05)

claude-cli'nin config evi artık **per-workspace**: `<workspace>/claude-home` =
`CLAUDE_CONFIG_DIR`. Böylece TionSwarm ve driver ettiği CLI **aynı skill/settings/
login** setini kullanır. Detay `51-CLAUDE-CONFIG-BIRLESIK.md`.

- **Faz 1:** `providers.ClaudeCLI.SetConfigDir` (turluk override) + `Runtime.claudeHomeDir()`;
  `toolloop.go` choke point'te (`provider.(*providers.ClaudeCLI)` sonrası) her CLI turu
  için set edilir. `agent.EnsureWorkspaceClaudeHome` workspace açılışında global
  `~/.tionswarm/claude-home`'u per-workspace eve tohumlar (çalışma-anı dizinleri atlanır),
  idempotent.
- **Faz 2:** ~~`workspaceSkillsDir` → `<workspace>/claude-home/skills`~~ **GERİ ALINDI (2026-07-05)** —
  skill'ler `<workspace>/skills`'te kalıyor; taşınmış skill'ler (WS1:4, WS8:14) geri alındı,
  `claude-home/skills` silindi. claude-home skill içermez, yalnız login/settings.
- **Faz 3:** ~~native `Skill` disallow kaldırıldı~~ **GERİ ALINDI (2026-07-05)** — native `Skill`
  KAPALI kalıyor; tek skill yolu `use_skill` köprüsü (hem workspace hem global tier'ı sunar).
- Sınırda bırakılan (bilinçli): MCP/hooks/izinler/agent'lar DB'de kalır, `--mcp-config`/
  `--settings` ile enjekte edilmeye devam eder.
- `go build ./...` ✅ · etkilenen 4 paket testi 241 passed.

**Ek (UI + güvenlik):**
- Settings → Sağlayıcılar → "claude config dizini" alanı **salt-okunur** yapıldı
  (`ProvidersPanel.tsx endpoint2ReadOnly`; per-workspace türetiliyor, global yalnız
  fallback). `tsc --noEmit` temiz.
- **Yedek sızıntısı kapatıldı:** migration credential/login dosyalarını (`.claude.json`,
  `.credentials.json`) her workspace'e kopyaladığından, workspace dizinini tümüyle
  zip'leyen periyodik yedek bunları arşive sızdırabilirdi. `backup/archive.go`'ya
  `backupExcludeNames` eklendi → bu dosyalar hiçbir yedek zip'ine yazılmaz (credential
  diskte kalır, arşive girmez). Test `TestZipExcludesClaudeCredentials`. Mevcut zip'ler
  (2026-06-25, migration öncesi) tarandı — sızıntı yok.

## claude-cli kalıcı süreç varsayılan AÇIK + sistem-prompt teslim toggle'ı ✅ (2026-07-05)

İki değişiklik:

1. **`claudePersistentSession` deneysellikten çıkarıldı, varsayılan AÇIK.**
   `settings.Default()` → `ClaudePersistentSession: true`. `--resume` ile karşılıklı
   dışlar; ikisi de açıkken persistent süreç kazanır (tasarım gereği). Struct/DTO/UI
   yorumlarından "experimental/deneysel" ibaresi kaldırıldı; UI toggle etiketi
   "(deneysel)" → sade. Tunables/skill/17. dokümanı güncellendi.

2. **Yeni ayar `claudeSysPromptFile` (varsayılan KAPALI = doğrudan).** claude-cli'ye
   eklenen sistem promptunun nasıl verileceğini seçer:
   - **Kapalı (varsayılan):** `--append-system-prompt <metin>` — komut satırında doğrudan, geçici dosya yok.
   - **Açık:** temp dosya + `--append-system-prompt-file <yol>` — Windows ~32 KB komut satırı limitini (errno 206) aşan çok büyük promptlar için.

   Plumbing: `settings` (struct+DTO+Patch+default) → `store.applyBool` → `api/server.go`
   `SetClaudeSysPromptFile` → `agent.Tunables.cliSysPromptFile` (Set/Get) →
   `recordedComplete` `req.SysPromptFile` → `providers.Request.SysPromptFile` →
   `claudecli.go` (tek-atış) + `claudecli_session.go` (kalıcı) dallanır. Persistent
   fingerprint'e eklenmedi (teslim yöntemi değişse de CLI'ye giden metin aynı — sıcak
   süreç yeniden başlatma gerektirmez). Frontend `types/settings.ts` + `SettingsPanel`
   patch + `AppToolsPanel` toggle.

   ⚠️ **Not:** doğrudan mod, prompt ~32 KB'ı aşarsa süreci başlatmadan çöktürebilir —
   o durumda dosya toggle'ını açın. (Bilinçli tercih: sessiz guard eklenmedi; hata
   gerekiyorsa görünür versin.)

   `go build`/`go vet`/285 test yeşil; `tsc --noEmit` temiz.

## Fix: "claude-cli kalıcı süreç" toggle'ı kaydedilmiyordu 🐛 (2026-07-05)

Ayarlar ekranındaki **claude-cli kalıcı süreç (deneysel)** toggle'ı açık kalmıyordu.
Neden: `SettingsPanel.tsx` içindeki kaydetme (patch) yükü `claudeResume`'u gönderiyor
ama `claudePersistentSession` alanını atlıyordu → toggle draft state'i değişiyor,
kaydet'e basınca backend'e hiç ulaşmıyor, yeniden yüklemede eski (kapalı) değere dönüyordu.
Düzeltme: patch objesine `claudePersistentSession: draft.claudePersistentSession` eklendi
(backend `store.go`/`settings.go` alanı zaten kabul ediyordu). `tsc --noEmit` temiz.

## Claude Code cache paritesi: P2–P5 tamamlandı ✅ (2026-07-05)

`_Docs\50-CLAUDE-CODE-CACHE-PARITE.md` planının kalan tüm iş paketleri uygulandı
(P1+P6 önceki oturumda bitmişti):

- **P2 — Özet=compact-boundary mesajı:** Rolling özet volatile Dinamik'ten çıkıp
  `providers.Request.Summary` ile taşınıyor; native yollar (anthropic + openrouter) onu
  cache'li önekin başına **sentetik head mesajı** koyuyor (`prependSummaryMessage`) → iki
  katlama arası **cache-read**. Cache kapalıyken `system`'e katlanır. **claude-cli native-only
  kararı gereği değişmedi** (özet hâlâ `[Context]` tail'inde, her tur taze). Önizleme
  `SummaryCached` ile özet bölümünü yeşil gösterir.
- **P4 — Cache-break telemetrisi:** `internal/agent/cachebreak.go` — oturum-başına önek
  hash'i (statik System + araç şeması) + model + warmth; ana konuşma turlarında sıcak önek
  kaybını (warmed && cacheRead==0 && cacheWrite≥2000) `cache_break` debug olayı olarak sebep
  atıflı kaydeder (model / prompt-tools / TTL-server). Debug kartında "Cache kırılması" pill +
  anomali. Claude Code `promptCacheBreakDetection` muadili.
- **P3 — API-native context editing:** anthropic `context_management` beta (`clear_tool_uses_
  20250919`), ayar `anthropicContextEditing` (vars. kapalı), ContextPanel toggle'ı. Microcompact
  muadili; client-side fold'a ek.
- **P5 — TTL/breakpoint kararlılığı:** tüm anthropic breakpoint'leri tek `cacheTTL="1h"`
  sabitinden türer (karışık-TTL drift'i imkânsız); `TestCacheBreakpointStability` tek stabil
  rolling marker'ı (son persist blokta) kilitler.

**Doğrulama:** `go test ./...` **680 yeşil**, `go vet` temiz, `tsc` temiz, `dist` yeniden
build edildi (tek-binary'e gömülü). Dokümanlar: `_Docs\50` (P2–P5 ✅), `_Docs\38` (cache_break
olayı), default skill `tionswarm-settings`.

**Canlı doğrulandı ✅ (2026-07-05, OpenRouter `anthropic/claude-haiku-4.5`, gerçek API):**
- **P1** — statik System (~8.2k tok) + geçmiş, **dinamik her tur değişmesine rağmen** turn-2'de
  `cache_read=8197` HIT aldı (P1'den önce dinamik system'de olduğu için bu 0 olurdu).
- **P2** — stabil özet head'i turn-2'de `cache_read=8216` HIT → özet cache'li önekin parçası.
- **P4** — sistem öneki başından değişince `cache_read` 8216→0 çöktü → detektör kırılmayı yakalar.
  Canlı test bir eksik ortaya çıkardı ve düzeltildi: OpenRouter write sayacını raporlamadığından
  tetik `cacheWrite+input` üzerinden ölçülüyor (native Anthropic + OpenRouter ikisini de kapsar).

> Not: Native anthropic anahtarı store'da sahte (`asdfasdf…`) olduğu için doğrulama gerçek
> OpenRouter anahtarıyla yapıldı (aynı native cache kod yolu: OpenAICompat cacheSystem). Kullanıcının
> backend'i yeni kodu almak için `.\scripts\dev.ps1` ile yeniden başlatılmalı (şu an çalışan örnek yok).


## Ağ ekranı mobilde kasması giderildi (vis-network lite modu) ✅ (2026-07-05)

Neden: `vis-network` canvas'ında **node gölgeleri**, **eğri (continuous) kenarlar**,
**`improvedLayout`** ön-yerleşim geçişi ve yüksek stabilizasyon iterasyonu — mobil
GPU'da pan/zoom sırasında her karede yeniden çizim çok pahalı → kasma.

Çözüm: `VisNetworkGraph`'a `lite` prop'u eklendi; `NetworkPanel` bunu `useIsMobile()`
ile besliyor. Lite modda (`< md`): gölge kapalı, kenarlar düz (`smooth: false`),
`improvedLayout: false`, stabilizasyon 300→120, hover kapalı (touch'ta zaten yok).
Grafın kendisi (düğüm/kenar/fizik yerleşimi) aynı, sadece çizim ucuzladı. Masaüstünde
tam kalite korunur. Build temiz; mobil (390px) canvas temiz render ediyor.

> ⚠️ `VisNetworkGraph.tsx` bu oturumda eşzamanlı bir süreç tarafından bir kez geri
> alındı; değişiklikler yeniden uygulandı.
## Ağ ekranı: istatistik + yenile başlık çubuğuna taşındı ✅ (2026-07-05)

`NetworkPanel` App'in generic başlığını kullanıyordu ("Ağ"). Artık kendi başlık
çubuğu var: "Ağ" solda, **"9 ajan · 13 görev · 2 akış · 0 beceri · 2 MCP"** istatistiği
sağa dayalı (`ml-auto`), **Yenile** butonu en sağda. İstatistik + yenile eski
toolbar satırından kaldırıldı; `network` `HEADERLESS_VIEWS`'e eklendi (çift başlık
olmasın). Playwright: title "Ağ", stats sağda, refresh en sağda, App çift başlığı yok.
Build temiz.

## AgentPicker: seçili ajan artık kaldırılabilir ✅ (2026-07-05)

- `AgentPicker`'a `clearable` prop'u eklendi. Aktifken seçili bir ajanı temizlemek
  için iki yol var: **trigger'daki ✕** düğmesi (sağda, `▾` yerine) ve **açılır listenin
  başındaki "Seçimi kaldır"** seçeneği. İkisi de `onChange('')` çağırıp değeri boşaltır.
- Etkinleştirildiği yerler: **Board görev formu** (`TaskFormModal` — atama zaten
  opsiyonel) ve **Akış ajan node'u** (`flow/NodeInspector`). Kullanıcı her ikisinde de
  seçili ajanı silemiyordu; artık silinebiliyor.
- Dosyalar: `agents/AgentPicker.tsx`, `panels/TaskFormModal.tsx`, `flow/NodeInspector.tsx`.
  `tsc -b` temiz.

## Agents: aktivite paneli chat Detay gibi yan-drawer oldu ✅ (2026-07-05)

- `AgentsView` aktivite paneli artık `SessionDetailPanel` (chat "Detay") ile aynı
  desende açılıyor: **masaüstünde** sağ kolon (tam yükseklik), **mobilde** sağdan
  kayan drawer (`max-md:fixed inset-y-0 right-0 z-40 w-[85vw] max-w-sm shadow-xl`) +
  `md:hidden` karartma backdrop (tıklayınca kapatır).
- İçerik satırından `max-md:flex-col` kaldırıldı (panel artık mobilde altta yığılmıyor,
  drawer). Sarmalayıcı `flex` yapıldı ki `<aside>` (h-full'süz) tam yüksekliği doldursun.
- Doğrulama: masaüstünde panel sağ kenarda (x:1600/w:320/h:860), toggle `aria-pressed`
  çalışıyor. Mobil drawer CSS'i SessionDetailPanel ile birebir.
- Dosya: `agents/AgentsView.tsx`. `tsc -b` temiz.

## Modal (bottom-sheet) mobilde navbar arkasında kalması fix ✅ (2026-07-05)

`ModalOverlay` `z-50` → `z-[60]`. MobileNavBar de `z-50` ve DOM'da modaldan sonra geldiğinden,
mobilde bottom-sheet modalın (ör. Flows node-editör popup'ı) altı navbar'ın arkasında kalıyordu.
Modal artık navbar'ın **üstünde**; 13 `ModalOverlay` tüketicisinin hepsi tek yerden düzeldi.

## Skills/Artifacts başlığı: yan-bilgiler dar ekranda 2. satıra + min-h-0 fix ✅ (2026-07-05)

- **`PaneHeader`'a `secondary` prop'u:** üç-slot flex-wrap düzeni. Geniş ekranda tek
  satır (title · secondary · right); dar ekranda `secondary` grubu tam genişlikle
  (`order-last basis-full`) 2. satıra sarar, title + sağ eylemler 1. satırda kalır.
  Geniş: `md:order-2 md:flex-1 md:basis-auto`.
- **Skills:** chip'ler (grup/Kısıtlı/görünürlük/kaynak) + **Tam/Özet/İsim/Gizli**
  seçici `secondary`'ye taşındı (dar ekranda 2. satır). Title + Düzenle/Kısıtla/
  kopya/Aç/Sil 1. satırda.
- **Artifacts:** rozetler (origin/kind/creator) + **İçerik** (içerik-kopyala) butonu
  `secondary`'ye taşındı. Title + Düzenle/yol-kopyala/Aç/kaynak/Sil 1. satırda.
- **Navbar/yükseklik fix:** Skills + Artifacts içerik kolonuna (`flex flex-1 flex-col`)
  **`min-h-0`** eklendi — eksik olması `min-height:auto` yüzünden uzun içeriğin kolonu
  parent yüksekliğinin ötesine taşırıp body-scroll + navbar kaymasına yol açıyordu.
- Dosyalar: `common/PaneHeader.tsx`, `panels/SkillsPanel.tsx`, `panels/ArtifactsPanel.tsx`.
  `tsc -b` temiz; canlı DOM doğrulaması (secondary `basis-full order-last`, geniş inline).
- **Güncelleme:** `PaneHeader`'a `secondaryAlwaysWrap` prop'u eklendi — `secondary`
  grubu **her genişlikte** kendi 2. satırında kalır (`md:` inline override'ları
  kaldırılır). **Skills** bu modu kullanıyor (chip'ler + Tam/Özet/İsim/Gizli hep 2.
  satırda). Doğrulama: geniş ekranda header 2 satır, secondary title'ın altında.
- **Güncelleme 2:** **Artifacts** de artık `secondaryAlwaysWrap` (chip'ler + İçerik
  hep 2. satırda). Ayrıca 2. satırda hareketli kontrol **sağa dayandı** (`ml-auto`):
  Skills'te Tam/Özet/İsim/Gizli seçici, Artifacts'te İçerik butonu satırın sağ
  kenarında (chip'ler solda). Doğrulama: her ikisinde `rightGap: 0`.

## Çoklu seçim: Ctrl+Click ile aktif öğe de sete dahil ediliyor ✅ (2026-07-05)

Sorun: bir öğe açık/aktifken Ctrl+Click ile ikinci bir öğeye tıklanınca yalnız yeni
tıklanan seçime giriyor, ilk (aktif) öğe dahil edilmiyordu — çünkü multi-select seti
panelin "aktif detay" state'inden ayrı.

**Çözüm (`hooks/useMultiSelect.ts` — tek noktada, tüm ekranları kapsar):**
`handleClick` artık yeni bir çoklu seçim başlarken (set boş) aktif öğeyi **tohumluyor**:
Ctrl/Cmd+Click ikinci satırda → ikisi de seçilir. Aktif öğe kaynağı: yeni opsiyonel
`activeId` parametresi (varsa) → yoksa anchor (son düz-tıklanan satır). Böylece
tıklanmamış default-seçili satır bile dahil edilir.

**Çağrı yerleri:** paneller aktif id'lerini geçiriyor — Artifacts (`activeId`),
Executions (`selectedId`), Flows (`selectedId`), Skills (`activeSlug`), Tools
(`selectedName`), Sessions (`activeSessionId`), Agents (`selectedId`). Memory /
AgentTools / TaskBoard anchor fallback kullanır.

Playwright: Tools (tık→Ctrl+tık = 2 seçili), Artifacts (default-seçili + Ctrl+tık başka
satır = 2 seçili). Build temiz.
## Ekran seçimleri oturum boyunca korunuyor (reload'da sıfırlanır) ✅ (2026-07-05)

İstek: farklı ekranlar arasında gezerken seçili öğe (sohbet, aktivite, ajan, hafıza
ajanı, akış, artifact, skill, tool) korunsun; uygulama kapatılıp açılınca sıfırlansın.

**Mevcut zaten çalışanlar (App-seviyesi state):** Sohbet (`activeSessionId`),
Aktivite (`executionTarget` + `onSelectExecution`), Ajanlar/Hafıza (`activeAgentId`).
App unmount olmadığı için nav geçişinde korunuyorlardı.

**Kırık olanlar (panel-local `useState`, remount'ta sıfırlanıyordu):** Artifactlar,
Skills, Araçlar, Akışlar — paneller kendi seçimini tutup App'e geri yazmıyordu.

**Çözüm:** Yeni `useSessionState(key, initial)` hook'u (→ `hooks/useSessionState.ts`).
`useState` benzeri ama değeri **modül-içi bir Map**'te tutar → component unmount/remount
arası korunur, ama localStorage'a yazılmaz → tam sayfa reload / uygulama yeniden açılış
sıfırlar. Panellerde seçim state'i buna geçirildi:
- `ArtifactsPanel` → `artifacts.activeId`
- `SkillsPanel` → `skills.activeSlug`
- `ToolsPanel` → `tools.selectedName`
- `FlowsPanel` → `flows.selectedId` + `tab` + `templateId` + `selectedRunId`, ayrıca
  mount'ta seçili akışın editör state'ini yeniden yükleyen tek-seferlik restore effect.

Playwright: 8 ekranın hepsinde seç → başka ekrana git → geri dön → seçim korunuyor.
Build temiz.
## Hafıza (memory): bilgi inputu + Belge(ikon) + Yansıt üst-title'a ✅ (2026-07-04)

- `memory` `HEADERLESS_VIEWS`'e eklendi; `MemoryPanel` kendi `PaneHeader`'ını render
  ediyor: `titleSlot` = bilgi-girme inputu (büyüyen), `right` = **Belge** (yalnız
  `Paperclip` ikonu, metin kaldırıldı) + **Yansıt**. Eski "bilgi ekle + reflect" bar'ı
  ve `+ Belge` (`Button`) kaldırıldı. Kullanılmayan `Button` importu temizlendi.
- Memory chat-benzeri layout (sol AgentRoster sibling + App header) kullandığından,
  App header'ın mobil roster hamburger'ı kayboldu → `PaneHeader`'a `onToggleList`
  prop'u (`setMobileListOpen(true)`) App'ten geçirildi; null-ajan durumunda da
  PaneHeader ("Hafıza" + hamburger) render ediliyor.
- Dosyalar: `App.tsx`, `panels/MemoryPanel.tsx`. `tsc -b` temiz; canlı doğrulandı
  (input header'da, Belge ikon-only, Yansıt header'da).
- **Ek:** `titleSlot`'ta inputun soluna seçili ajanın `AgentAvatar`'ı (22px) eklendi.

## Dar ekranda sohbet mesaj listesi yatay padding azaltıldı (1px) ✅ (2026-07-04)

Sohbet mesaj kaydırma konteyneri (`MessageList` `role="log"`) **ve** üstte sabitlenen
(pinned) soru overlay'i yatay padding'i dar telefonlarda **1px**'e düşürüldü:
`px-6` → **`px-[1px] md:px-6`** (ikisinde de). Dar (`<md`) = 1px, geniş (`md+`) = 24px
(değişmedi). Playwright: 390px→1px, 1280px→24px. Build temiz.
## Flow: daraltılabilir "Node ekle" paleti + yukarı büyüyen run input ✅ (2026-07-04)

- "Node ekle" başlığı chevron'lu toggle (`paletteOpen`, localStorage kalıcı); daraltınca node
  tip butonları gizlenir, "Görünüm" kalır.
- Çalıştır girdisi auto-grow (`runInputRef` effect, `min(scrollHeight,160)`, `resize-none`); run
  paneli bottom-anchored olduğundan **yukarı doğru** genişler (alt kenar sabit). Detay `15-FLOW-CANVAS.md`.

## Dar ekranda composer ↔ alt navbar boşluğu near-flush yapıldı ✅ (2026-07-04)

Sohbet ekranında composer ile alttaki `MobileNavBar` arasındaki boşluk fazlaydı;
kullanıcı composer'ın navbar'a bitişik olmasını istedi. `main` alt padding'i
sabit `pb-16/pb-20` yerine **`max-md:pb-[calc(3.25rem+env(safe-area-inset-bottom))]`**
yapıldı → navbar yüksekliğiyle (safe-area dahil) eşleşiyor, boşluk **~1px** (bitişik),
taşma yok. Safe-area büyüyen cihazlarda da padding aynı `env()` değerini içerdiği için
bitişiklik korunur, composer navbar arkasına kaçmaz. Playwright (390px): composer alt
728, navbar üst 729, gap 1, overlap yok. Build temiz.

## Bağlam modal header + cache info + otomasyon formu sadeleştirme ✅ (2026-07-04)

1. **Cache notu → InfoPopover:** SessionContextModal cache legend'ında "cache'li…"
   chip'inin yanındaki uzun `data.cache.note` metni, chip içine (i) `InfoPopover`
   olarak alındı.
2. **Header aksiyonları title yanına döndü:** iki modalda header tekrar tek satır
   (`nowrap`); kopyala + X butonları başlığın yanında (BulkButtons zaten örnek-mesaj
   satırına taşınmıştı). Alt yazı `truncate`.
3. **Otomasyon formu:** "Ad (ops.)" isim inputu **create + edit**'ten kaldırıldı;
   kart görünümündeki isim (`{a.name || '(adsız)'}`) kaldırıldı (artık `#etiket`
   birincil). Ajan-Akış geçişi (`TargetModeToggle`) forma **en sola** alındı
   (zamanlamada zaten soldaydı).
4. **FlowPicker görünür ikon:** native `<select>` artık solunda seçili akışın
   ikonunu (mojibake-safe emoji, yoksa `Workflow` glyph) gösteriyor — AgentPicker
   avatar'ına paralel; hem schedule hem otomasyon formunda.

Playwright: isim inputu 0, toggle en solda, iki FlowPicker'da da ikon, header nowrap
+ copy/X title yanında, cache chip'inde (i). Build temiz.

## Bağlam modalı: token özeti + CLI ek yükü açılır-kapanır (default kapalı) ✅ (2026-07-04)

`SessionContextModal` ve `AgentContextModal`'da token istatistik satırı (Toplam,
Sistem, Skills, … ~token tahmini) ve "CLI ek yükü" bloğu tek bir açılır-kapanır
şeride alındı. **Default kapalı**; başlık çubuğu "Token özeti · Toplam N (~tahmini)"
+ varsa "CLI ek yükü" rozetini gösterir, `ChevronRight` 90° dönerek durumu belli
eder. Playwright: default `aria-expanded=false`, kapalıyken detay (`Mesajlar (…)`)
gizli, tıklayınca açılıyor. Build temiz.

## Flow değişken popover'ı kırpılma fix + run input info butonu ✅ (2026-07-04)

- **Kırpılma fix** — `FlowVarsButton` paylaşılan bileşene taşındı (`flow/FlowVarsButton.tsx`),
  absolute popover yerine satır-içi (`w-full basis-full`, ebeveyn `flex flex-wrap`) panel; modal
  gövdesinin `overflow-y-auto`'su artık kesmiyor.
- **Run input info butonu** — flow başlatma girdisine ℹ️ (`context="seed"`): yalnız
  `{{date}}/{{time}}/{{datetime}}` (motorun sıralı `render` ikamesiyle çözülür) + "{{input}} olur" notu.
- Yalnız frontend; `tsc`+build temiz. Detay `15-FLOW-CANVAS.md`.

## Bağlam modal aksiyonları + claude-cli cache notu + schedule kartı ✅ (2026-07-04)

1. **"Tümünü aç/kapat" ikon-only + Compact Simüle yanına taşındı:** `SessionContextModal`
   ve `AgentContextModal`'da header'daki metinli `BulkButtons` kaldırıldı; örnek-mesaj
   satırına (Compaction simüle / Simüle et düğmesinin yanına), yalnız ikon (30×30)
   olarak taşındı. Satır `flex-wrap` yapıldı. Playwright: ikon-only, input satırında,
   header'da yok.
2. **claude-cli soğuk-tur cache notu kaldırıldı:** backend `session_context.go` içindeki
   uzun "claude-cli ilk/soğuk tur…" metni `Note: ""` yapıldı; frontend cache-legend
   boş note'u artık render etmiyor (`{data.cache.note && …}`). ⚠️ görünür etki için
   backend restart gerekir.
3. **Schedule/Automation kartı dikey:** açma-kapama toggle'ı **üstte**, agent/flow
   ikonu **altta** olacak şekilde dikey sütuna alındı (`Schedules.tsx` + `Automations.tsx`).
   Playwright: stacked column, toggle üstte, icon altta.

Frontend + Go build temiz.

## Dar ekran düzeltmeleri: bağlam modal başlığı + ayarlar min-w + artifact buton metni ✅ (2026-07-04)

Üç dar-ekran sorunu giderildi:

1. **Bağlam pencereleri başlığı sığmıyordu** (`SessionContextModal`, `AgentContextModal`):
   sağdaki geniş aksiyon butonları (Tümünü aç/kapat/kopyala/X) başlık kolonunu ~0'a
   sıkıştırıyor, alt yazı dikey kelime-kelime sarıyordu. Header artık `flex-wrap`;
   dar ekranda başlık tam ilk satırı alıyor (`basis-full sm:basis-0`), aksiyonlar
   tek grup halinde alt satıra sarıyor. Playwright (360px): başlık kolonu 319px,
   alt yazı 2 satır (dikey sıkışma yok), modal 360'a sığıyor.

2. **Ayarlar Hooks & Harici Araçlar bir noktadan sonra daralmıyordu:** içerik kolonu
   (`SettingsPanel` sağ pane) `min-w-0` içermiyordu → içindeki geniş bir öğe (kod
   örneği vb.) min-content genişliğinin altına inmeyi engelliyor, yatay taşma
   oluşuyordu. `flex flex-1 flex-col` → `flex min-w-0 flex-1 flex-col`. Bu tek
   düzeltme tüm ayar kategorilerini kapsıyor. Playwright (360px): `docScrollW 360`,
   taşma yok (hem hooks hem exttools).

3. **Artifacts "içeriği kopyala" butonu metni** "İçerik" olarak kısaltıldı.

Build temiz.

## Flow tarih/saat değişkenleri + tag filtreleme ✅ (2026-07-04)

- **Yeni flow değişkenleri** — `orchestration.render` (engine.go) artık `{{date}}`/`{{time}}`/
  `{{datetime}}`'i de çözer (otomasyon formatıyla aynı). Node inspector ℹ️ popover'ına eklendi.
  Test `TestRenderVars`. Oturum-bağlamlı otomasyon değişkenleri (`{{result}}` vb.) flow'da yok.
- **Flow tag filtreleme** — `FlowsPanel` Akışlarım sekmesinde etiket chip'leri; ANY-eşleşme
  süzme, ad aramasıyla AND. Boş → "Eşleşen akış yok." Detay `15-FLOW-CANVAS.md`.

## Alt navbar'a workspace seçme butonu eklendi ✅ (2026-07-04)

Dar ekranlarda (`< md`) masaüstü NavRail'in tepesindeki workspace seçici yoktu.
Alt `MobileNavBar`'ın **en başına sabit** bir workspace-seçme butonu eklendi (mobil
karşılığı `MobileWorkspaceButton` → `components/MobileWorkspaceButton.tsx`).

- **Yukarı açılan menü:** bar ekranın altında olduğu için popup `bottom-full` ile
  **yukarı** açılıyor; workspace'ler arasında geçiş + "Yeni workspace" (mevcut
  `WorkspaceCreateModal` yeniden kullanıldı).
- **Kırpılma çözümü:** `<nav>` artık kaydırmıyor (overflow visible), yalnız içteki
  görünüm şeridi yatay kayıyor. Böylece yukarı açılan menü şeridin `overflow` 'una
  takılmıyor. `useDragScroll` ref'i içteki şeride taşındı (fare-sürükleme korundu).
- **Playwright doğrulaması (360px):** workspace butonu en solda (left 0); menü
  yukarı açılıyor (bottom 706 ≤ nav-top 709), kırpılmıyor (top ≥ 0), 5 workspace +
  create listeleniyor; şerit hâlâ fare-sürükleme ile kayıyor (200px, navigasyon yok).
  Build temiz.

## Node türü sabit + değişken info butonu + flow tag chip'leri ✅ (2026-07-04)

Flow editörü UX (yalnız frontend). Ayrıntı: `_Docs/15-FLOW-CANVAS.md`.

- **Node türü sabit** — `NodeInspector`'dan "Tür" select kaldırıldı; yerine salt-okunur tür
  başlığı (monokrom ikon + ad + "(tür sabit)"). Tür artık yalnız palet'ten oluşturulurken belirlenir.
- **Değişken info butonu** — Prompt/Şablon alanları yanına ℹ️ popover (`FlowVarsButton`).
  Flow motorunun gerçekten desteklediği değişkenler: `{{input}}`, `{{last}}`, `{{node.<id>}}`
  (engine.go `render`). Popover, akıştaki her diğer node için dinamik `{{node.<id>}}` satırı üretir;
  tıklayınca alana ekler. (Otomasyonların `{{date}}` vb. flow motorunda yok.)
- **Flow tag chip'leri** — etiketler zaten Görünüm > Etiket'ten atanabiliyordu; artık sol flow
  listesinde chip olarak da görünür (canlı senkron).

## Akış emojisi + monokrom node ikonları ✅ (2026-07-04)

Akışlar (Flow) için iki görsel iyileştirme. Ayrıntı: `_Docs/15-FLOW-CANVAS.md`.

- **`Flow.emoji`** — her akışa opsiyonel emoji. Bağımsız kalıcı: yeni store metodu
  `SetFlowEmoji` + `PUT /api/flows/{id}/emoji`; `UpdateFlow` emojiye dokunmaz (ad/graph
  kaydı emojiyi silmez). `create_flow`/`update_flow`/`get_flow`/`list_flows` araçlarına
  `emoji` eklendi. UI: `FlowsPanel` başlığında ortak `EmojiField` (seçince anında kalıcı).
  Emoji her akış-seçim/gösterim yerinde: flow listesi, Koşular listesi + başlık, `RunView`,
  `FlowPicker` (Zamanlama/Otomasyon), zamanlama/otomasyon satır rozetleri, TaskBoard +
  `TaskFormModal`. Hepsi `normalizeAvatar` ile mojibake-güvenli.
- **Monokrom node ikonları** — çok renkli emoji (🤖🔀⚡⏱️🧩) yerine temaya uygun tek renkli
  lucide ikonlar (`Bot/Split/Zap/Timer/Puzzle`, `nodeStyles.NODE_ICONS`). `NodeChrome.icon`
  → `Icon: LucideIcon`; `NodeShell` başlıkta + "Node ekle" paletinde (node ağacı) aynı
  ikon seti render eder.
- **Test:** `TestFlowEmojiRoundTrip` (create/UpdateFlow-koruma/SetFlowEmoji-değiştir-temizle).
  Go build+vet temiz, `tsc`+`npm run build` temiz.

## Görevler (board): toolbar üst-title'a taşındı ✅ (2026-07-04)

- `board` `HEADERLESS_VIEWS`'e eklendi; `TaskBoard` board kolonunun tepesine
  `PaneHeader` (title="Görevler", `right` = ⊞ Sütunlar + "+ Görev" + 🔗 Sırala)
  yerleştirildi. Eski `border-b` toolbar bar'ı kaldırıldı. PaneHeader, sol sütun
  editörü panelinin sağındaki içerik kolonunun tepesinde (diğer ekranlardaki
  ListPane+PaneHeader deseniyle tutarlı). Sütunlar butonu zaten editörü toggle
  ettiğinden ayrı liste-toggle'a gerek yok.
- Dosyalar: `App.tsx`, `panels/TaskBoard.tsx`. `tsc -b` temiz; canlı doğrulandı
  (header: "Görevler · ⊞ Sütunlar · + Görev · 🔗 Sırala").

## Alt navbar fare-sürükleme ile kaydırılabilir oldu ✅ (2026-07-04)

Dar ekranlarda (`< md`) alttaki yatay `MobileNavBar` dokunmatikle native kayıyordu
ama fareyle sürüklenemiyordu. Yeni `useDragScroll` hook'u (→ `hooks/useDragScroll.ts`)
sadece **fare** için tıkla-sürükle panning ekliyor (touch'a dokunulmuyor — zaten
momentumlu native kaydırma var). 4px'lik ölü bölge + capture-fazında click bastırma
ile bir buton üzerinde sürükleme yanlışlıkla o görünüme geçmiyor. `cursor-grab` /
`active:cursor-grabbing` + `select-none` görsel/etkileşim ipuçları eklendi.

- **Playwright doğrulaması (360px):** 200px sola sürükleme → `scrollLeft` 0→200
  (birebir), görünüm değişmedi; düz tıklama → görünüm değişiyor. Build temiz.

## OpenRouter duplicate-key React uyarısı giderildi (katalog id çakışması) ✅ (2026-07-04)

**Belirti:** Sağlayıcı/model seçicilerinde React "encountered two children with the
same key, `openrouter`" uyarısı.

**Kök neden (backend, frontend değil):** `GET /api/catalog` yerleşik katalogu
(`providers.Catalog()`) custom sağlayıcılarla (`CustomCatalog()`) **id kontrolü
olmadan** birleştiriyordu. Kullanıcı, yerleşik `openrouter` ile aynı id'de bir
custom sağlayıcı eklemişti → katalog aynı id'yi iki kez döndürüyordu (canlı API'de
doğrulandı: 26 modelli yerleşik + 3 modelli custom). Bu sadece bir uyarı değil,
gerçek belirsizlik: `provider: "openrouter"` hangisini kastediyor?

**Düzeltme:** Yeni `providers.MergeCatalog(builtin, custom)` (→ `internal/providers/merge.go`).
Custom sağlayıcı aynı id'li yerleşiği **yerinde override eder** ("kullanıcı config'i
kazanır"), eşi olmayan custom id'ler sona eklenir; girdi dilimleri değişmez.
`handleCatalog` artık bunu kullanıyor. Birim test: `merge_test.go` (çakışma → tek
kayıt, sıra korunur, custom kazanır, girdi mutasyonu yok) — geçti. `go build ./internal/...` temiz.

> ⚠️ Etki için backend restart gerekir — çalışan `go run ./cmd/tionswarm` (kullanıcı
> oturumu) yeniden başlayınca devreye girer.

## Bütçe: iç header üst-title'a (PaneHeader) taşındı ✅ (2026-07-04)

- `budget` `HEADERLESS_VIEWS`'e eklendi; `BudgetPanel` kendi `PaneHeader`'ını render
  ediyor (title="Bütçe", subtitle = gün, `right` = 7g/30g/90g aralık seçici + Yenile).
  Eski iç header (`Wallet` + "Bütçe" h1 + gün + kontroller) kaldırıldı → App header ile
  çift "Bütçe" başlığı sorunu giderildi. Kullanılmayan `Wallet` importu temizlendi.
  İçerik (özet kartlar + tablolar) `flex-1 overflow-y-auto p-5` gövdeye alındı.
- Dosyalar: `App.tsx`, `panels/BudgetPanel.tsx`. `tsc -b` temiz; canlı doğrulandı
  (header: "Bütçe · 2026-07-04 · 7g/30g/90g · Yenile").

## Schedule + Automation: boş oturum yerine bir Flow başlatma (`flowId`) ✅ (2026-07-04)

Kullanıcı isteği: zamanlamalar ve otomasyonlar tek ajana prompt teslim etmek yerine
seçilen bir **orkestrasyon akışını** (Flow) da başlatabilsin. Task'taki mevcut
`FlowID` deseni Schedule + Automation'a taşındı.

- **Model:** `db.Schedule.FlowID` + `db.Automation.FlowID` (`flowId,omitempty`); set
  ise prompt = akış girdisi, agent/targetAgent opsiyonel (karşılıklı dışlar). Store
  `UpdateSchedule`/`UpdateAutomation` round-trip eder.
- **Dispatch:** `scheduler.go` `run` → `deliverFlow` (`RunFlowRecorded`, akış kendi
  bildirim + transcript oturumunu yönetir, `FlowFailure`→delivery failure).
  `automation.go` `fire` → render sonrası `fireFlow` (spawn atlanır; akış per-tetik,
  kendini döngülemez — guardrail'ler tetik sıklığını sınırlar, SpawnTags yok sayılır).
- **API:** create/update (schedules+automations) `flowId` alır; doğrulama gevşetildi
  (cron/triggerTag + promptTemplate zorunlu; hedef = flowId **ya da** ajan). Automation
  update hedef-değiştirme mantığı flowId↔agent (kısmi güncelleme hedefi ellemez).
- **Araçlar:** `create/update_schedule` + `create/update_automation` + `list_*`
  `flowId` (flow varlığı `GetFlow` ile doğrulanır).
- **UI:** ortak `TargetModeToggle` (Ajan/Akış) + `FlowPicker` (Schedules.tsx export;
  Automations import); create/edit formları + liste satırları (`Workflow` ikonu +
  `🔀 <akış>`); akış otomasyonunda spawn-etiket editörü yerine bilgi notu.
- **Test:** `db/automation_test.go` `TestAutomationFlowIDRoundTrip` +
  `TestScheduleFlowIDRoundTrip`. `go build ./...` + `go test` (409) yeşil, `tsc` temiz.
- Dosyalar: `db/models_task.go`, `db/models_automation.go`, `db/store_schedule.go`,
  `db/store_automation.go`, `agent/scheduler.go`, `agent/automation.go`,
  `api/schedules.go`, `api/automations.go`, `tools/builtin_schedulemgmt.go`,
  `tools/builtin_automationmgmt.go`, `frontend/{types/task.ts, api/tasks.ts,
  panels/Schedules.tsx, panels/Automations.tsx}`. Detay `20-SCHEDULE-WAKE.md`,
  `46-ETIKET-OTOMASYON.md`.

## Logs: sayaç + Kopyala + Aç üst-title'a taşındı ✅ (2026-07-04)

- `logs` `HEADERLESS_VIEWS`'e eklendi; `LogsPanel` kendi `PaneHeader`'ını render ediyor
  (title="Loglar", `right` = "N satır · N kayıt" sayacı + `CopyPathButton` (ikon) +
  `RevealButton` **label="Aç"**). Bu üç öğe filtre toolbar'ından çıkarıldı; toolbar'da
  seviyeler/arama/grupla/canlı/Yenile kaldı.
- Reveal butonu artık "Aç" metnini gösteriyor (`labelClassName="hidden sm:inline"`).
- Dosyalar: `App.tsx`, `panels/LogsPanel.tsx`. `tsc -b` temiz; canlı doğrulandı
  (header: "Loglar · 35 satır · 36 kayıt · [kopya] · Aç").

## Schedules toggle title'a + Flows minimap toggle + Market butonları panele ✅ (2026-07-04)

Üç ayrı UI isteği. Hepsi canlı doğrulandı (mcp-chrome), `tsc -b` temiz.

- **Schedules — otonomi toggle başlığa taşındı:** `schedules` artık `HEADERLESS_VIEWS`
  içinde; `Schedules` kendi `PaneHeader`'ını render ediyor (title="Otomasyon",
  `right` = "Otonomiyi duraklat" switch). İçerikteki eski büyük duraklat kartı
  kaldırıldı. Dosyalar: `App.tsx`, `panels/Schedules.tsx`.
- **Flows — mini harita aç/kapa butonu:** `FlowCanvas` `CanvasInner`'a `showMinimap`
  state'i + `CanvasTools` içine "🗺 Mini harita" toggle butonu eklendi (`CanvasTools`
  artık salt-okunur modda da render oluyor; auto-layout yalnız editable). `<MiniMap>`
  koşullu. Dosya: `flow/FlowCanvas.tsx`.
- **Market — İçe Aktar + Kaynaklar sol panele:** iki buton üst header'dan çıkarılıp
  sol "Kategoriler" `ListPane`'inin altına (border-top'lu footer) taşındı. Header'da
  yalnız arama + Yenile kaldı. Dosya: `panels/MarketPanel.tsx`.

## Agents: aktivite paneli üst-title'daki butondan toggle ✅ (2026-07-04)

Kullanıcı isteği: ajan ekranında aktivite paneli, başlıktaki bir butonla açılıp
kapansın. Canlı doğrulandı (mcp-chrome).

- `AgentsView` `PaneHeader` `right`'ına **"Aktivite"** toggle butonu eklendi
  (`data-testid="agent-activity-toggle"`, `Activity` ikonu, chat'teki "Detay"
  butonu deseni; açıkken accent kenarlık, `aria-pressed`). Copy/Aç butonları yalnız
  ajan seçiliyken; Aktivite butonu her zaman görünür.
- Eski **slim dikey reopen-rail** (kapalı durumda sağ kenardaki şerit) kaldırıldı;
  panel artık yalnız başlıktaki butondan açılıp kapanıyor (`{activityOpen && <AgentActivityPanel/>}`).
  Panelin kendi X (onClose) butonu korundu.
- Dosya: `agents/AgentsView.tsx`. `tsc -b` temiz.

## Artifacts + Skills başlıkları da üst-title'a birleştirildi ✅ (2026-07-04)

Flows/Agents desenini diğer detay ekranlarına uygulama. Canlı doğrulandı (mcp-chrome).

- **Artifacts:** detay toolbar'ı (başlık + origin/kind/creator rozetleri + eylemler:
  Düzenle/İçerik-kopyala/Yol-kopyala/Aç/Kaynak-sohbet/Sil) tamamen üst `PaneHeader`'a
  taşındı → `titleSlot` = kimlik, `right` = eylemler. Düzenleme modunda `right` =
  Kaydet/İptal. "Artifactlar" başlığı + subtitle detay görünümünde kalktı.
- **Skills:** isim + rozetler (grup/kısıt/görünürlük/kaynak) `titleSlot`'a, eylem
  grubu (Düzenle/Kısıtla-Paylaş/Görünürlük-seçici/Yol-kopyala/Aç/Sil) `right`'a taşındı.
  Açıklama bloğu (slug, açıklama, ne-zaman, izinli araçlar, alt-beceriler) gövdede
  slim strip olarak kaldı. "Skills" başlığı detay görünümünde kalktı.
- **Uygulanmayan:** `Araçlar & MCP` (detay salt-içerik, ayrı toolbar/path yok) ve
  liste-ağırlıklı ekranlar (Market/Hafıza/Loglar/Bütçe) desene uymuyor — dokunulmadı.
- Dosyalar: `panels/ArtifactsPanel.tsx`, `panels/SkillsPanel.tsx`. `tsc -b` temiz.

## Fix: Schedules ekranında "Zamanlamalar" başlığı scroll dışında kalıyordu ✅ (2026-07-04)

- **Belirti:** Schedules ekranında "Zamanlamalar (cron / zaman tabanlı)" başlığı +
  yeni-zamanlama formu üstte sabit (pinli) kalıyor, yalnız alttaki liste (içine
  Automations da giriyor) kayıyordu → başlık/form dikey alan yiyor, scroll dışında.
- **Çözüm:** `Schedules.tsx` tek scroll kapsayıcısına alındı — kök `flex-col`
  (p-4'süz), içine `min-h-0 flex-1 overflow-y-auto p-4` sarmalayıcı; başlık + form +
  liste + `Automations` birlikte kayar. Liste div'i `flex-1 … overflow-y-auto` →
  `space-y-2`. Deep-link `scrollIntoView` çalışmaya devam eder. `tsc -b && vite build` temiz.

## Flows: Şablonlar + Koşular başlıkları da üst-title'a birleştirildi ✅ (2026-07-04)

Önceki birleştirmenin (flow editörü + agents) devamı — aynı desen Şablonlar ve
Koşular sekmelerine uygulandı. Canlı doğrulandı (mcp-chrome).

- **Şablonlar:** şablon önizleme üstündeki ayrı toolbar kaldırıldı; içeriği üst
  `PaneHeader`'a taşındı → `titleSlot` = şablon adı + açıklaması; `right` =
  "salt-okunur önizleme" + "+ Bu şablondan akış oluştur". "Akışlar" başlığı kalktı.
- **Koşular:** `RunView`'e `hideSummary` prop'u eklendi (üst özet satırını gizler,
  Girdi/Hata satırları kalır). `STATUS_LABEL` + `statusColor` export edildi;
  `FlowsPanel` üst `PaneHeader`'da `titleSlot` = akış adı + durum + tarih, `right` =
  "Tekrar çalıştır" butonu. "Akışlar" başlığı kalktı, özet çift render olmuyor.
- `title` artık üç detay görünümünde de (editor/template/run) gizli; yalnız
  boş/liste durumunda "Akışlar".
- Dosyalar: `flow/RunView.tsx`, `panels/FlowsPanel.tsx`. `tsc -b` temiz.

## Flows + Agents başlıkları tek üst-title'a birleştirildi ✅ (2026-07-04)

Kullanıcı isteği: detay başlıklarını tek üst-bar'a topla (sohbet başlığı deseni).
Canlı doğrulandı (mcp-chrome).

- **`PaneHeader` esnetildi:** `title` opsiyonel oldu + yeni `titleSlot?: ReactNode`
  (title/subtitle bloğunun yerine büyüyen özel içerik — ör. isim inputu). Sol
  konteyner `flex-1` aldı ki input genişleyebilsin. Hamburger zaten `md:hidden`
  (yalnız dar ekran).
- **Flows:** flow editöründe ayrı "meta toolbar" (alt title) kaldırıldı; içeriği üst
  `PaneHeader`'a taşındı → `titleSlot` = akış-adı inputu + ID chip; `right` =
  Yol-kopyala (ikon) + "Aç" + "Kaydet". "Akışlar" başlığı ve `· <akış adı>` subtitle
  editör görünümünde kaldırıldı (şablon/koşu/boş sekmelerde "Akışlar" korunur).
  Doğrulama: header'da input="akış adı", id="FLW…", butonlar [Listeyi göster, Yolu
  kopyala, Aç, Kaydet], "Akışlar" yok.
- **Agents:** `CopyPathButton` + `RevealButton` `AgentSettingsForm` header'ından
  `AgentsView` üst `PaneHeader`'ının `right`'ına taşındı; reveal etiketi "Klasörü aç"
  → **"Aç"**. Kullanılmayan importlar (`api`, `CopyPathButton`, `RevealButton`)
  AgentSettingsForm'dan temizlendi.
- Dosyalar: `common/PaneHeader.tsx`, `panels/FlowsPanel.tsx`, `agents/AgentsView.tsx`,
  `agents/AgentSettingsForm.tsx`. `tsc -b` temiz.

## Sol panellerdeki "listeyi gizle" butonları kaldırıldı (sohbet listesi gibi) ✅ (2026-07-04)

Sohbet oturum listesinde panel-kapat butonu yok; mobilde drawer'ı **boşluğa
(backdrop) tıklayarak** kapatıyorsun, masaüstünde ise sütun hep açık. Diğer liste
ekranlarındaki `PanelLeftClose` "Listeyi kapat" butonları bu davranışı gereksiz
kılıyordu — hepsi kaldırıldı.

- **`SidebarHeader` (`common/SidebarChrome.tsx`):** `onCollapse` prop'u ve kapat
  butonu tamamen kaldırıldı → Executions, Ajanlar, Artifactlar başlıklarındaki
  buton gitti (3 çağrı yeri güncellendi).
- **Liste-içi satır butonları kaldırıldı:** ToolsPanel, FlowsPanel, SkillsPanel,
  MarketPanel (`md:hidden` "Listeyi kapat" düğmeleri) + artık kullanılmayan
  `PanelLeftClose` importları temizlendi.
- **Korunan:** başlıktaki hamburger (Menu) açma butonu — mobilde drawer'ı açmak
  için gerekli (MarketPanel dahil), sohbetteki gibi.
- **Playwright doğrulaması (390px):** 7 ekranın hepsinde liste sütununda 0 gizle
  butonu; hamburger ile açılıyor, backdrop'a tıklayınca kapanıyor
  (drawer `left: 0 → -width`).

## Kopyala butonları sadece-ikon yapıldı ✅ (2026-07-04)

Kullanıcı isteği: kopyala butonlarındaki "Bağlamı kopyala" / "Copy" gibi metinler
kaldırılsın, yalnız kopyalama ikonu kalsın.

- **Metin→ikon (4 buton):** `markdown/CodeBlock` ("Copy/Copied" → `Copy`/`Check`),
  `markdown/MermaidDiagram` (aynı), `sessions/SessionContextModal` ("Bağlamı kopyala"
  → ikon), `agents/AgentContextModal` ("Promptu kopyala" → ikon). Erişilebilirlik
  için `title` + `aria-label` eklendi; CodeBlock/Mermaid'e `lucide-react` ikonları,
  Session/Agent modal'a `Check` importu geldi.
- **Zaten ikon-only:** tüm `CopyPathButton` örnekleri (`labelClassName="hidden"`),
  `SecretsPanel`, `PromptEditor`, `ClaudeAuthDialog`, `ArtifactsPanel` kopya butonları.
- **Bilinçli istisna (etiketli bırakıldı):** `SessionsSidebar` sağ-tık menüsündeki
  "Yolu kopyala" (menü öğesi — liste satırı, etiket gerekli) ve `ExecutionsPanel`
  seçim-çubuğundaki "Kimlikleri kopyala" (bulk-action; `SelectionBarButton` children
  zorunlu, ikon-only UX'i bozardı). İstenirse bunlar da dönüştürülebilir.
- `tsc -b` temiz; canlı doğrulama (mcp-chrome) sorunsuz.

## Liste ekranı başlıkları tam sohbet paritesi: başlık artık listenin üstüne gelmiyor ✅ (2026-07-04)

Sorun: liste ekranlarında (Aktivite, Ajanlar, Artifactlar, Skills, Araçlar, Akışlar)
`PaneHeader` tüm genişliğe yayılıyordu — başlık, soldaki listenin **üzerine** de
geliyordu. Sohbet ekranında ise başlık yalnız içerik alanının üstünde; oturum
listesinin üzerine gelmez.

**Kök neden:** panel yapısı `flex-col > [PaneHeader(tam genişlik)] > [row: liste | detay]`
şeklindeydi. Sohbet ise `flex-row > [liste (tam yükseklik)] | [main: header + içerik]`.

**Düzeltme:** her liste ekranı sohbet düzenine geçirildi →
`flex-row > [ListPane (tam yükseklik kardeş sütun)] | [içerik-kolonu: PaneHeader + detay]`.
Böylece başlık çubuğu yalnız listenin **sağındaki** içerik kolonunun üstünde durur.
Playwright ile doğrulandı: 6 ekranın hepsinde `header.left === list.right` (üst üste
binme yok), 1280px'de hamburger gizli, 390px'de hamburger görünür + drawer varsayılan
kapalı, yatay taşma yok.

- **Değişen dosyalar:** `ExecutionsPanel`, `AgentsView` (3 kolon korundu:
  roster | ayarlar | aktivite), `ArtifactsPanel` (drop-zone kök satır oldu),
  `SkillsPanel`, `ToolsPanel`, `FlowsPanel`.
- **Path butonları sohbet stiline getirildi:** başlıklardaki `Yolu kopyala`
  (`labelClassName="hidden"` → sadece ikon) ve `Aç` (`labelClassName="hidden sm:inline"`)
  artık sohbet başlığındakiyle birebir aynı. Executions'ta path butonları detay
  alt-başlığından `PaneHeader.right`'a taşındı (sohbetteki gibi sağ üstte).

## Tüm kopyalama butonları merkezi `copyToClipboard`'a taşındı ✅ (2026-07-04)

Önceki düzeltmenin devamı: uygulamadaki tüm doğrudan `navigator.clipboard.writeText`
kullanımları tek merkezi metoda migrate edildi (güvensiz LAN/HTTP bağlamında hepsi
sessizce bozuktu).

- **Merkezi metod:** `lib/clipboard.ts` → `copyToClipboard(text, promptLabel?)`.
  İç akış: `copyText` (Clipboard API → `execCommand` fallback) başarısızsa
  `window.prompt` ile elle-kopya. Programatik kopya başarılıysa `true` döner →
  çağıran "Kopyalandı" onayını yalnız bunda gösterir. `navigator.clipboard`'a
  doğrudan dokunan tek yer artık bu dosya.
- **Migrate edilen 9 dosya (10+ buton):** `markdown/CodeBlock`, `markdown/MermaidDiagram`,
  `agents/AgentContextModal`, `sessions/SessionContextModal`, `panels/ArtifactsPanel`,
  `settings/ClaudeAuthDialog`, `common/PromptEditor`, `panels/SecretsPanel`,
  `panels/ExecutionsPanel` (toplu-id + oturum-id kopyala). Ayrıca daha önce düzeltilen
  `CopyPathButton` + `App.tsx` (`openFile`, `copySessionPath`) de artık merkezi metodu
  kullanıyor (inline prompt kaldırıldı).
- `tsc -b` temiz; canlı smoke testi (mcp-chrome) sorunsuz.

## Claude Code cache paritesi P1+P6: native yolda dinamiği mesaj kuyruğuna taşı ✅ (2026-07-04)

`_Docs\50-CLAUDE-CODE-CACHE-PARITE.md` planının **P1** (en büyük kazanç) + **P6**'sı uygulandı.
**Teşhis:** native (anthropic + OpenAI-compat) yolda Tools + statik System zaten cache HIT
alıyordu, ama volatile **Dinamik `system` alanında** (tools+mesajların önünde) durduğu için
asıl büyüyen kısmın (mesaj geçmişi) rolling breakpoint'i **her tur ıskalıyordu** → pratikte
ölü. External Agents bunu yaşamıyor çünkü cache/compaction'ı native `claude` binary'ye (Claude
Agent SDK) devrediyor; TionSwarm kendi yazdığı için boşluk oluşmuş.

**Yapılan (yalnız `extendedCache`/`cacheSystem` açıkken; cache-kapalı yol birebir korundu):**
- `anthropic.go`: `systemField` artık **statik-only** (tam cache'lenebilir); `toAnthropicMessages`
  yeni imza `(msgs, extendedCache, dynamic)` — rolling breakpoint son **persist** mesaj bloğunda,
  volatile dinamik onun **gerisinde** trailing text-blok olarak (request-time, persist edilmez).
- `minimax.go`: `buildSystemMessage` statik-only; `attachHistoryBreakpoint` işaretlediği
  **indeksi döndürür**; dinamik o mesaja breakpoint'ten sonra eklenir. Fallback: uygun mesaj
  yoksa trailing user mesajı.
- **Invariant:** cache öneki yalnız immutable içerik barındırır → dinamik hiç persist edilmez,
  sonraki tur önek byte-aynı kalır → HIT. (claude-cli'nin `withDynamic(lastUserText…)` deseninin
  native'e genellenmesi.)
- `session_context.go` `computeCachePreview` anthropic dalı: `CachedMsgCount=msgCount-1` (rolling),
  Araçlar+Sistem+geçmiş cache'li, dinamik "tail/taze" notu.
- Testler: `anthropic_test.go` + `minimax_test.go` yeni yerleşimi kilitler (dinamik breakpoint'in
  gerisinde, cache-kapalıyken system'de kalır). **`go test` 289 yeşil**, `tsc` temiz.
- **Kalan:** canlı `cache_read>0` ölçümü (anthropic/openrouter anahtarı + gerçek tur). Sıradaki:
  **P2** (özeti compact-boundary mesajına çevir → özet de cache'lensin), sonra P4 (cache-break
  telemetri), P3/P5 (opsiyonel). Detay: `_Docs\50`.

## claude-cli ek yükü: ajan bağlam önizlemesinde de + buton sohbet başlığına ✅ (2026-07-04)

CLI ek-yük bilgisi artık **iki bağlam penceresinde de** görünür ve "Bağlam önizle"
butonu sohbet başlığına taşındı. `go build/test` + `tsc -b --force` temiz.

- **Ajan bağlam önizlemesi (`AgentContextModal`):** `handleAgentContext` artık
  `computeCLIOverhead`'i **boş sessionID** ile çağırır → predicted-only (ölçüm yok,
  ajanın oturumu yok). `computeCLIOverhead` boş sessionID'de debug/usage okumasını
  atlar. UI'da "Beklenen (CLI, tahmini)" chip'i + "CLI ek yükü" uyarı kutusu. Canlı:
  AGT4 → predicted 27.275 (eager=5). `agentContextPreview.cliOverhead` alanı eklendi.
- **Buton taşındı:** `SessionDetailPanel` "Araçlar" kartındaki "Bağlam önizle (debug)"
  → sohbet başlığına (`App.tsx` header, `ScanEye` ikon). `SessionContextModal` artık
  App.tsx'ten render edilir; detay panelini açmadan erişilir. İlgili import/state/modal
  detay panelinden temizlendi.

## claude-cli ek yükü: ölçülmüş generic referans + önceden tahmin ✅ (2026-07-04)

Kullanıcı "Tahmin ↔ Gerçek" farkının (claude-cli vergisi) nereden geldiğini sordu →
bileşenler **empirik ölçüldü** ve uygulama geneli generic bir referansa dönüştürüldü.
`go build ./...` + `go test ./internal/conversation ./internal/api` temiz (95 test).

- **Ölçüm (claude-cli 2.1.201, gerçek API `usage`):** saf sistem promptu 0 araç =
  **17.067**; +dahili araçlar (~15) = **26.265** (dahili ≈ 9.198); köprülü araç başına
  ort. şema **~215** (42–710). Doğrulama: SES104 Tahmin 29.573 → Gerçek 88.425.
- **Generic kaynak `internal/conversation/clioverhead.go`:** `CLIBaseSystemTokens`,
  `CLIBuiltinToolsTokens`, `CLIBaseTokens`, `CLIAvgBridgedToolTokens` sabitleri +
  `PredictCLIOverhead(loadedTools)` = `26.200 + yüklü×215`. Token-hesap yapan her yer
  bu tek kaynaktan okur (native paket, import döngüsü yok).
- **`api/session_context.go`:** `computeCLIOverhead` artık `eagerTools` alır (call-site
  `interactionTier(d.Name)=="core"` sayımı — gerçek eager/deferred ayrımı, fs built-in'ler
  tabanda); ölçüm yokken (`measured==0`) 0 yerine `PredictCLIOverhead` ile **önceden
  tahmin (cold-start floor)** verir. `cliOverheadPreview.predictedOverhead` alanı (JSON)
  eklendi. Ölçüm-yok notu artık taban rakamları içerir.
- **Frontend (`SessionContextModal.tsx` + `types/session.ts`):** ölçüm yokken başlıkta
  "Beklenen (CLI, tahmini) = Tahmin + predictedOverhead" Stat'ı + "CLI ek yükü" kutusunda
  "Tahmin → beklenen ~X (+Y tahmini ek yük, henüz ölçülmedi)" satırı. Ölçüm gelince eski
  "Gerçek" görünümü. Benim dosyalarım `tsc` temiz.
- **Docs/skill:** `_Docs\17` "claude-cli ek yükü — ölçülmüş referans" bölümü;
  `tionswarm-session-debug` skill'ine "Tahmin↔Gerçek farkı" ölçüm-referansı + tekrar-
  ölçüm komutu eklendi. `clioverhead_test.go` regresyon kilidi.

## Liste panelleri tam sohbet paritesi: masaüstü hep açık + hamburger sadece mobil ✅ (2026-07-04)

Kullanıcı: liste ekranları (Aktivite vb.) sohbet ekranı gibi olmalı — geniş ekranda
**sol panel hep açık**, **hamburger geniş ekranda gizli**, daralt-butonu yok. Önceki
davranış (masaüstünde de daraltılabilir + toggle her boyutta) sohbetten farklıydı.
Sohbet sessions-drawer modeline geçirildi. `tsc -b && vite build` temiz; Playwright
ile masaüstü (1280) + mobil (390) doğrulandı.

- **`common/CollapsibleListShell.tsx` yeniden yazıldı:** liste artık DAİMA DOM'da —
  `md+` statik sütun (hep görünür, daralma yok), `< md` sola kayan drawer (`open`
  yalnız mobil translate'i sürer) + backdrop. Eski "kapalıyken null / ince ray"
  mantığı kaldırıldı.
- **`useCollapsibleList` sadeleşti:** artık ephemeral `useState(false)` (persist yok)
  — sohbetin `mobileListOpen`'ı gibi; mobilde her açılışta drawer kapalı gelir
  (eski persist, drawer'ı açık açıyordu).
- **Tüm toggle'lar `md:hidden`:** PaneHeader hamburger, Market inline, App
  workspace/settings toggle + liste-içi daralt butonları (SidebarHeader onCollapse,
  Skills/Tools/Market/Flows) → geniş ekranda görünmez, sadece mobil drawer'ı sürer.
- **Executions başlığı bağlamlı:** `Aktivite · {seçili çalıştırma}` (sohbetteki
  "Sohbet · Manager" gibi). Doğrulama: masaüstü hamburger gizli + aside statik
  görünür; mobil hamburger görünür + drawer default kapalı, tıklayınca dolu-surface
  drawer + backdrop, taşma yok.

## Flow açıklaması (description) uygulamadan tamamen kaldırıldı ✅ (2026-07-04)

Akışların `description` alanı uçtan uca kaldırıldı. `go build/test` (413) + `tsc -b &&
vite build` temiz.

- **Backend model/store/API:** `db.Flow.Description` alanı silindi; `store_flow.UpdateFlow`,
  `api.flows` (`flowReq` + create/update), `summarizer` (SummaryFlows artık `Ad (id)`),
  `api/graph.go` (node `Sub` = flow id) güncellendi.
- **Agent araçları (`builtin_flowmgmt.go`):** `create_flow`/`update_flow` şemalarından
  `description` prop'u; `list_flows`/`get_flow` çıktılarından `description` alanı; ilgili
  tool açıklama metinleri temizlendi.
- **Market/şablon (tam temizlik — 2026-07-04):** SwarmPack format tiplerinden
  `FlowPayload.Description` ve `WorkspaceTemplateFlow.Description` alanları **da silindi**
  (Go `market/pack.go` + frontend `types/market.ts`); install/publish/seed zaten db.Flow
  ile bağ kurmuyordu. `MarketPanel` flow açıklaması render'ı kaldırıldı. `gen_examples.py`
  `flow_pack` artık flow'a description koymaz (pack-seviye `description` korunur).
  **Gömülü paketler temizlendi:** `internal/market/defaults/workspace.*.swarmpack.json`
  içindeki 5 flow-description anahtarı silindi (4 dosya; `blank`'te yoktu). Ev dizininde
  başka `.swarmpack` yok. Not: çalışan workspace store'larındaki `FLW*.json` dosyalarında
  kalan eski `description` anahtarları zararsız (yükte yok sayılır, ilk kayıtta düşer);
  uygulama çalışırken canlı store'a dokunulmadı.
- **Frontend:** `types/flow.ts` + `api/flows.ts` (createFlow/updateFlow imzaları)
  `description`'sız; `FlowsPanel` state/dirty/kaydet/şablon çağrıları temizlendi;
  `useChatStream` slash-komut açıklaması sadeleşti; `WorkspaceExportPanel` sub → flow id.
- **Sol flow listesi:** açıklama satırı yerine **flow id (mono) · N node** meta satırı
  (`flowNodeCount` helper). Detay `_Docs\15`.

## Yol kopyala butonu güvensiz bağlamda (LAN IP/HTTP) düzeltildi ✅ (2026-07-04)

Kullanıcı "Copy path / Open path çalışmıyor" bildirdi. Canlı tarayıcıda (mcp-chrome,
`http://192.168.1.4:5173`) teşhis edildi:

- **Kök neden (Copy):** `window.isSecureContext === false` → `navigator.clipboard`
  **undefined**. Eski kod `navigator.clipboard?.writeText` (optional chaining) ile
  sessizce hiçbir şey yapmıyordu. Async Clipboard API yalnız güvenli origin'de
  (HTTPS veya `localhost`) açık; düz-HTTP LAN IP'de yok. Test: bu Chrome güvensiz
  bağlamda `document.execCommand('copy')`'yi de (gerçek tıklama gesture'ında bile)
  **false** döndürüyor.
- **Çözüm:** yeni `lib/clipboard.ts` → `copyText()` (Clipboard API → execCommand
  fallback, boolean döner). `CopyPathButton` ve `App.tsx` (`openFile`,
  `copySessionPath`) bunu kullanır. Her ikisi de başarısızsa **son çare**:
  `window.prompt(...)` yolu seçili gösterip kullanıcının Ctrl+C ile elle
  kopyalamasını sağlar → hiçbir bağlamda sessiz başarısızlık kalmaz.
- **Open (Aç) aslında çalışıyor:** backend `/api/sessions/{id}/reveal` →
  `explorer.exe <path>` host'ta klasörü açıyor (Shell.Application ile doğrulandı).
  Uzak cihazdan erişimde host'ta açılır (tasarım gereği), istemcide değil. Önceki
  manuel test 404'leri geçiciydi (hash chat route'unda değilken `sessionId=undefined`
  gidiyordu) — endpoint sağlam.
- Not: uygulamada ~13 yerde daha doğrudan `navigator.clipboard` kullanımı var;
  bunlar da güvensiz bağlamda çalışmaz — ileride `copyText`'e migrate edilebilir.
- Dosyalar: `frontend/src/lib/clipboard.ts` (yeni), `frontend/src/components/CopyPathButton.tsx`,
  `frontend/src/App.tsx`.

## Chat başlığından bütçe-yönlendiren harcama pill'i kaldırıldı ✅ (2026-07-04)

- **`ChatMeters` kaldırıldı:** chat üst-bar'ındaki "BUGÜNKÜ harcama" pill'i
  (`N çağrı · ~$X`, tıklayınca Bütçe ekranına gidiyordu) `App.tsx` başlığından
  silindi; import ve artık öksüz kalan `components/panels/ChatMeters.tsx` dosyası
  tamamen kaldırıldı. `meterRefresh` state'i korundu (artifact yenileme +
  `SessionDetailPanel` hâlâ kullanıyor). Bütçe verisi Bütçe ekranı + oturum detay
  panelinde zaten mevcut.
- Dosyalar: `frontend/src/App.tsx`, `frontend/src/components/panels/ChatMeters.tsx` (silindi).

## Ajan-yanı model etiketi + isim hizalaması ✅ (2026-07-04)

Kullanıcı isteğiyle 2 küçük UI düzeltmesi (canlı tarayıcıda mcp-chrome ile doğrulandı).

- **Model etiketinden açıklama eki kırpıldı:** `resolveModelLabel` (`lib/catalog.ts`)
  artık `stripTagline` ile katalog label'ındaki boşlukla ayrılmış tire sonrası eki
  atar → "Sonnet — dengeli" yerine "Sonnet", "MiniMax M3 - guncel amiral" yerine
  "MiniMax M3". Regex `/\s+[—–-]\s+.*$/` boşluksuz tireleri ("GPT-5.5") ve parantezli
  varyantları ("Opus 4.8 (Fast)") korur. Tam açıklamalı label'lar model seçicilerde
  (`ProviderModelSelect`/`ProvidersPanel`) aynen kalır; yalnız ajan-yanı gösterim
  sadeleşir.
- **Ajan ismi hizalaması standartlaştı:** `AgentIdentity` kök span'ine `text-left`
  eklendi. `<button>` varsayılan `text-align:center` taşıdığından composer'ın
  `AgentSelect` tetikleyicisinde (ve `AgentPicker` trigger'ında — `text-left`
  class'ı yoktu) ajan ismi ortalanıyordu; artık her yerde sola hizalı.
- **Dar telefonda sadece avatar:** `AgentIdentity`'ye `mobileIconOnly` prop'u eklendi
  → metin sütunu `md` altında `hidden` (yalnız avatar). Composer `AgentSelect`
  tetikleyicisi bu prop'u kullanır (`md:max-w-[180px]`), böylece dar ekranda ajan
  seçici kompakt kalır.
- Dosyalar: `frontend/src/lib/catalog.ts`, `frontend/src/components/agents/AgentIdentity.tsx`,
  `frontend/src/components/chat/composer/AgentSelect.tsx`.

## Liste-toggle butonu tüm ekranlarda sohbet hamburger'ıyla eşitlendi ✅ (2026-07-04)

Kullanıcı geri bildirimi: Executions/Agents/… başlığındaki liste aç/kapa butonu
(turuncu çerçeveli `PanelLeft` kutu) sohbet başlığındaki hamburger'dan farklı
görünüyordu. Hepsi sohbetteki **çerçevesiz `Menu` (hamburger), dim renk** stiline
alındı. `tsc -b && vite build` temiz; Playwright ile doğrulandı (border 0px, dim renk).

- `common/PaneHeader.tsx` (Agents/Artifacts/Tools/Skills/Executions/Flows),
  `MarketPanel` inline toggle, `App.tsx` header workspace/settings toggle → hepsi
  `Menu` + `flex h-8 w-8 rounded-lg text-dim hover:bg-surface-2` (sohbet hamburger'ının
  birebir sınıfları). Eski `border-accent` (kapalıyken) / bordered kutu kaldırıldı.

## Flow editörü: popup boyutu + palet sürükle-bırak ✅ (2026-07-04)

- **Popup boyutu board popup'ına eşitlendi** (`TaskFormModal`): `max-h-[90vh] w-full
  max-w-2xl rounded-xl` (önceki `w-80 max-h-[85vh]` yerine).
- **Palet sürükle-bırak ile node ekleme:** node listesindeki tipler `draggable`;
  `FlowCanvas` `CanvasInner`'a ayrıldı (ReactFlowProvider altında `screenToFlowPosition`
  erişimi için), `onDrop` bırakma noktasını graf uzayına çevirip `onDropNode` →
  `FlowsPanel.addNodeAt` ile node'u **o konumda** oluşturur. Tık ile ekleme korundu.
  MIME: `FLOW_NODE_DND_MIME`. `tsc -b && vite build` temiz.

## Flow editörü UX düzeni: popup node editörü + sadeleşmiş toolbar ✅ (2026-07-04)

Kullanıcı isteğiyle 4 UI değişikliği. `tsc -b && vite build` temiz.

- **Meta toolbar sadeleşti:** açıklama (`description`) alanı kaldırıldı; "yolu kopyala"
  **icon-only** (`CopyPathButton` `label` prop'u kaldırıldı); ad girişi genişledi.
- **Görünüm ayarları sol palete taşındı:** etiket (`TagEditor`), "Kablo" edge-style
  seçici ve "Animasyon" toggle artık "Node ekle" paletinin altında **Görünüm** bölümünde
  (palet `w-40`, mobil `w-32`).
- **Node editörü popup oldu:** sabit sağ panel → `ModalOverlay`. Node'a **tıklayınca**
  açılır; `FlowCanvas`'a `onNodeClick` prop'u + `nodeDragThreshold={4}` eklendi →
  sürükleme/tıklama karışmaz. Boş canvas/Escape/backdrop kapatır.
- **Node üstü toolbar kaldırıldı:** `NodeToolbar` yerine Başlangıç/Çoğalt/Sil eylemleri
  popup içindeki `NodeInspector` başlığında (`onDuplicate` prop'u eklendi). FlowsPanel
  artık `nodeActions` geçmiyor → `NodeActionsContext` null.
- Dosyalar: `FlowsPanel.tsx`, `flow/FlowCanvas.tsx`, `flow/NodeInspector.tsx`,
  `CopyPathButton.tsx` (değişmedi — zaten opsiyonel label). Detay `_Docs\15`.

## Portrait UI testi (Playwright) + taşma düzeltmeleri ✅ (2026-07-04)

Gerçek tarayıcıda (Playwright, 360px & 390px) 16 view tarandı; yatay-taşma
(docW > viewport) tespiti için clip-farkında JS detektörü kullanıldı. 4 gerçek
taşma bulundu ve düzeltildi; tümü "dar ekranda sarmalanmayan/`shrink-0` buton
satırı" desenindeydi. Yeniden tarama: 16/16 view temiz (docW=360), drawer açıkken
dolu surface + taşma yok.

- **Chat composer:** `px-6→max-md:px-3`, toolbar satırı `flex-wrap`, `AgentSelect`
  ad genişliği mobilde `max-w-[120px]` → "Gönder" artık taşmıyor.
- **Skills detay eylem çubuğu:** başlık + toolbar `flex-wrap` (eski `shrink-0`
  kaldırıldı) → ~238px taşma giderildi.
- **Artifacts viewer başlığı:** `flex-wrap` (başlık + aksiyon toolbar).
- **Memory ekle satırı:** `flex-wrap` + input `min-w-[10rem]`.
- Not: layout-dışı, pre-existing bir React uyarısı gözlendi — Sağlayıcılar
  listesinde çift `key="openrouter"` (veri kaynaklı; ayrı ele alınmalı).

## Standart ekran başlığı: PaneHeader + başlıktan liste aç/kapa (9 ekran) ✅ (2026-07-04)

Tüm liste ekranları sohbet ekranı gibi bir **başlık çubuğu + tıklanabilir liste
aç/kapa butonu** kazandı; kapalıyken liste tamamen gizlenir (sohbet gibi, ince ray
yok), başlıktan yeniden açılır. `tsc -b && vite build` temiz.

- **Yeni `common/PaneHeader.tsx`:** standart ekran başlığı (sol toggle + başlık +
  ops. subtitle + sağ aksiyonlar). **`CollapsibleListShell`/`ListPane` `hideRail`:**
  kapalıyken null döner (ray yerine başlık toggle'ı açar).
- **PaneHeader'lı 7 ekran:** Agents (roster→ListPane, subtitle = seçili ajan / "Ajan
  seçilmedi", boş-durum korunur), Artifacts, Tools, Market (toggle mevcut katalog
  başlığına), Skills, Executions, Flows.
- **App-header toggle'lı 2 ekran:** Workspace + Settings — kategori/sekme `<aside>`'ı
  `CollapsibleListShell hideRail` ile sarıldı; collapse state App'te
  (`workspaceNav`/`settingsNav` = useCollapsibleList), App header'ında PanelLeft
  toggle. Detay: `_Docs\49` §7.6.

## Refactor: iki-panelli liste ekranları tek `ListPane` standardında ✅ (2026-07-04)

Her iki-panelli ekran kendi liste-kolonu çözümünü uyguluyordu (kimi
`useResizableSidebar`, Skills özel resize, Tools/Market sabit genişlik; farklı
bg/border/handle; kimi CollapsibleListShell'li kimi değil). Tek standart bileşene
indirgendi. `tsc -b && vite build` temiz.

- **Yeni `common/ListPane.tsx`:** tek standart sol liste kolonu — `CollapsibleListShell`
  (daralt/rail + mobil drawer) + dolu surface `<aside>` + sağ border + `useResizableSidebar`
  (kalıcı sürükle-genişlet) + `ResizeHandle`. Ekran yalnız header + gövdeyi `children`
  olarak verir; genişlik/collapse/tema/handle ListPane'de.
- **Taşınan 6 panel:** Artifacts, Skills, Tools, Market, Flows, Executions. Kazanımlar:
  Skills'in **özel resize kodu silindi**; Tools/Market **artık resizable**; Executions
  **artık daraltılabilir**; hepsi aynı bg/border/genişlik/drawer davranışı.
- **Kapsam dışı:** `AgentsView` (roster | ayarlar | aktivite = 3-panel özel; mobil
  flex-col stack + aktivite paneliyle rail/drawer çakışması) mevcut paylaşılan
  primitiflerde bırakıldı. Detay: `_Docs\49` §7.5.

## Fix: Akış editörü yanlış "kaydedilmemiş değişiklik" ✅ (2026-07-04)

- **Belirti:** Flows ekranında bir akışa tıklayınca hiçbir düzenleme yapılmasa
  bile "değişiklik var" algılanıyor; ayrılırken/kapatırken uyarı çıkıyor
  (nav amber nokta + `beforeunload`).
- **Kök neden:** `FlowsPanel.flowDirty` canvas'tan yeniden kurulan graph'ı
  (`reactFlowToGraph` her zaman `next:""`, `x`, `y` üretir) backend'in stored
  JSON'u ile karşılaştırıyordu. Go `orchestration` modeli neredeyse tüm alanlarda
  `omitempty` kullandığından (`next`, `x`, `y`, `prompt`…) kayıtlı JSON boş alanları
  düşürüyor → iki taraf **her açılışta** farklı → sürekli dirty.
- **Çözüm:** normalize mantığı `flowGraph.ts` içinde tek `canonicalGraphKey()`
  helper'ına çıkarıldı — bir graph'ı editörün yüklemede kullandığı aynı round-trip'ten
  (`graphToReactFlow → reactFlowToGraph`) geçirip kararlı bir karşılaştırma anahtarı
  döndürür (cosmetic `edgeStyle`/`animated` hariç). `flowDirty` hem canlı canvas'ı
  hem stored graph'ı bu helper'dan geçirir → simetrik, yalnız gerçek düzenlemeler
  fark yaratır (`FlowsPanel.tsx` + `flowGraph.ts`). `tsc --noEmit` temiz.

## Sohbet UX küçük rötuşlar ✅ (2026-07-04)

- **Sohbet header "yolu kopyala" icon-only** (`App.tsx`, `labelClassName="hidden"`).
- **Sohbet listesinde oturum ID'si alt satıra taşındı** (`SessionsSidebar` — başlık
  satırından çıkıp meta satırında sağa hizalı; başlık artık daha geniş).
- **Koordinatör worker listesi daraltılabilir** (`CoordinatorSection` — "Worker'lar · N"
  başlığı + chevron, kalıcı `tionswarm.coordWorkersOpen`).

`tsc -b && vite build` temiz.

## Panel UX: daraltılabilir listeler + otomasyon renk ayrımı ✅ (2026-07-04)

Kullanıcı isteğiyle 5 UI iyileştirmesi. `tsc -b && vite build` temiz.

- **Yeniden kullanılabilir daraltılabilir liste:** `hooks/useCollapsibleList.ts`
  (kalıcı, masaüstü açık / telefon kapalı) + `common/CollapsibleListShell.tsx`
  (açık: sütun / mobil drawer+backdrop; kapalı: ince yeniden-açma rayı) +
  `SidebarHeader` `onCollapse` (◀). Sessions sidebar deseninin genelleştirmesi.
- **Uygulandığı paneller:** Artifacts, Skills, Tools, Market (kategori rayı), Flows
  (sol akış listesi). Bu panellerde F3 mobil top-bottom stack **geri alındı** → drawer.
- **Flows node inspector:** editördeki sağ node paneli daraltılabilir
  (`PanelRightClose/Open`).
- **Agents aktivite paneli:** sohbet DetayPaneli gibi aç/kapa (X + "Aktivite" rayı).
- **Agents "yolu kopyala":** icon-only (`labelClassName="hidden"`).
- **Otomasyon ekranı renk ayrımı:** Zamanlamalar → sky sol şerit + "Zamanlamalar
  (cron)" başlığı; Otomasyonlar → violet sol şerit + violet başlık ikonu (form +
  satır + edit kartı). Hangisi ne, bir bakışta belli. Detay: `_Docs\49` §7.4.

## Üç optimizasyon: default-skill re-seed + skill sadeleştirme + run_code built-in binding'leri ✅ (2026-07-04)

1. **`EnsureDefaults` sürüm-farkında re-seed** (`internal/skills/defaults.go`): eskiden
   mevcut dosyanın üzerine hiç yazmıyordu → gömülü skill güncellemeleri mevcut
   kurulumlara yansımıyordu. Artık root'ta `.shipped-versions.json` sidecar'ı her
   default dosyanın son-shipped sha256'sını tutar; boot'ta: **eksik**→yaz, **gömülüyle
   aynı**→bırak+kaydet, **son-shipped ile aynı (kullanıcı dokunmamış)**→**tazele**,
   **ikisinden de farklı (kullanıcı edit'i)**→koru. İlk boot'ta manifest kendini
   seed'ler (senkronladığımız kopyalar gömülüyle eşit → hepsi kaydolur). Testler:
   `TestEnsureDefaultsSeeds` (edit korunur) + yeni `TestEnsureDefaultsRefreshesPristine`.
2. **`tionswarm-project` workspace skill'i sadeleştirildi** (~194→~150 satır): "Mevcut
   Yetenekler" bölümü changelog seviyesi tarih/commit/test-ismi/env-minutiae'den
   arındırılıp "ne var + hangi `_Docs\NN`" özet haritasına indirildi (her-tur cache'li
   prefix küçüldü). Yetenek bilgisi korundu.
3. **`run_code`'a built-in araç binding'leri** (`_Docs\44`): code-execution artık yalnız
   MCP'yi değil, **TionSwarm built-in araçlarını** da `tionswarm` Python modülü olarak sunar
   (`from tionswarm import <tool>`). `codemode.WriteBindings(dir, entries, builtins, allow)`
   + `Config.Builtin` dispatcher (reserved `tionswarm__<tool>` namespace, bare-name
   allow/gate/dispatch); built-in'ler **tur ctx**'iyle `reg.Call`'a gider → sink/oturum
   davranışı direkt-çağrıyla birebir. Eligibility hard-exclude (`CodeModeEligible`:
   interaktif/exec-in-exec/delegasyon/meta/worker araçları hariç) + ajan tool-filter.
   run_code artık **MCP'siz** de kullanılabilir (built-in'ler yeter; `toolsetup.go`'da
   `len(entries)>0` koşulu kaldırıldı). Testler: bindings (built-in modül + collision
   guard), builtin_runcode (dispatch + discovery), bridge (routing). Tam suite 660 yeşil.

## Mobil/dikey ekran uyumu — F3 (liste panelleri tek-sütun) ✅ (2026-07-04)

İki-sütunlu paneller portrait telefonda **dikey stack** (üstte liste 45vh tavanlı,
altta içerik); memory roster sohbet gibi drawer. Masaüstü birebir korunur. `tsc -b
&& vite build` temiz.

- **Memory roster → drawer:** `mobileSessionsOpen` → `mobileListOpen` genellendi
  (sohbet+memory ortak); header hamburger `chat||memory`'de; `AgentRoster` sohbet
  sidebar'ıyla aynı drawer deseni.
- **6 headerless panel dikey stack:** kök `max-md:flex-col` + sol liste
  `max-md:!w-full max-md:max-h-[45vh] max-md:border-b` (`!important` inline
  resizable width'i ezer) — Executions/Agents/Tools/Skills/Artifacts/Market
  (Market kategori rayı `flex-row flex-wrap` chip). Detay: `_Docs\49` §7.3.

## Otonom self-completion: oto-devam (lazy-tool aktivasyon tuzağı) ✅ (2026-07-04)

**Sorun (canlı SES6/schedule):** Otonom (scheduler/spawn/wake) tek-atımlık tur,
ajan `ToolSearch`/`activate_tools` ile araç aktive edip todo yazdıktan sonra
`end_turn` ile bitiyordu. claude-cli'nin araç seti süreç başında sabit olduğundan
aktive edilen araçlar **ancak bir sonraki turda** kullanılabilir — ama otonom turda
sonraki tur yok → görev yarıda kalıyor, kullanıcı müdahale edemediği için asla
tamamlanmıyor. Error de yok (temiz `end_turn`, `result.subtype=success`).

**Çözüm — `internal/agent/autocontinue.go`:** Otonom tur bittikten sonra
`maybeAutoContinue` trace'i inceler; **tamamlanmamış iş sinyali** varsa
(`needsAutoContinue`: son anlamlı eylem lazy-tool aktivasyonu **veya** en güncel
`todo_write`'ta açık `pending`/`in_progress` madde) aynı oturumda history-aware bir
**devam turu** (`runSessionTurn` + Türkçe nudge, `Origin="auto-continue"`) tetikler,
yanıtı persist eder ve tekrarlar — **maks 10** (`DefaultAutoContinueMax`, ayar
`autonomousAutoContinueMax`). Devam turunun kalıcı MCP pool'u sayesinde aktive edilen
araçlar artık hazırdır. Duruş koşulları: iş bitti (`!needsAutoContinue`), tur araç
ilerlemesi yapmadı (`hasToolStep`=false → no-progress guard), sağlayıcı hatası veya
**günlük bütçe** stop'u (guardedComplete zaten enforce eder). Çağrı noktaları:
`runSpawn` (spawn.go) + scheduler deliver (scheduler.go), `FireTurnFinished`'ten önce.
Ayar `autonomousAutoContinue` (vars. açık) + `autonomousAutoContinueMax` (vars. 10) —
settings→tunables köprüsü `SetAutoContinue` (server.go applySettings), Tunables
`AutoContinue()`/`AutoContinueMax()`. settings paketi build+test yeşil; agent/api
derlemesi paralel codemode WIP'i (`builtin_runcode.go` ↔ yeni `WriteBindings` imzası)
yüzünden geçici bloke — kendi dosyalar gofmt-temiz, imzalar doğrulandı.

## Koordinatör: interaktif tur ↔ oto-tur kilit birleştirme ✅ (2026-07-04)

Stream/non-stream kullanıcı turu artık koordinatör oto-turlarıyla aynı kilidi
paylaşıyor: `BeginCoordinatorUserTurn` (`coordination.go`, sync.Cond'lu coordSlot)
interaktif tur boyunca slotu tutar; bu sırada gelen worker bildirimleri pending'e
düşüp release'te TEK coalesced oto-tur olarak koşar; kullanıcı turu cap'i
(`turns`/`capWarn`) resetler. Wiring: `chat_stream.go` + `chat.go`
(`Role=="coordinator"` → claim + defer release). Otonom yollar da kapsandı:
`claimTurnSlotIfCoordinator` (no-op release non-coordinator'da, cap RESETLEMEZ)
→ wake + scheduled prompt (`scheduler.go`) + inbox (`agentmsg.go`). 3 yeni test;
agent+api 191 test yeşil (tools/codemode test derlemesi paralel oturumun devam
eden run_code imza değişikliğinden kırık — bu işten bağımsız). Detay: `_Docs/47` §10.

## Koordinatör: canlı coalescing testi ✅ + stream/oto-tur kilit bulgusu (2026-07-04)

SES104'te 3 hızlı worker (ALPHA/BETA/GAMMA) aynı turda spawn edildi: 3 bildirim
→ **2 otomatik tur** (SES113+SES112 tek turda birleşti) — `coordSlot` coalescing
canlıda doğrulandı. Aynı testte bulgu: `handleChatStream`/`wake_turn` oturum
kilidi kullanmıyor, `drainCoordinator` da `isSessionActive`'e bakmıyor → kullanıcı
stream turu ile koordinatör oto-turu aynı oturumda **paralel** koşabiliyor
(canlıda gözlendi, zararsızdı; tasarım kararı bekliyor). Detay + zaman çizelgesi
+ çözüm seçenekleri: `_Docs/47-KOORDINATOR-COKLU-AJAN.md` §10.

## Araç konsolidasyonu — birleşik built-in araçlar ✅ (2026-07-04)

Fazla/parçalı built-in araçlar tek çok-amaçlı araçlara indirildi (per-tur bağlam
+ şema tekrarı azaldı, yetenek aynı). CRUD aileleri zaten standarttı, dokunulmadı.

- **`update_session`** (yeni, `tools/builtin_sessionupdate.go`): altı ayrı aracı
  birleştirir — `set_session_title` / `set_working_dir` / `archive_session` /
  `set_session_goal` / `complete_goal` / `set_session_tags` **kaldırıldı**. Tek
  çağrıda title/working_dir/goal/goal_done/tags(add,remove veya replace)/archive
  alanlarından verilenleri uygular; mutasyondan önce hepsini doğrular (yarım
  güncelleme yok). `sessionFrom` (SessionSink ⊇ GoalSink) tek sink'ten okur →
  CLI köprüsünde tek `sessionAttach`. Görünürlük: name-only.
- **`secret`** (birleşik, `tools/builtin_secret.go`): `secret_list`/`_get`/`_set`/
  `_delete` **kaldırıldı** → tek araç `action: list|get|set|delete`. `builtin_secretmgmt.go`
  silindi; artık yalnız `buildRegistry`'de vault varken kayıtlı (hidden). RiskWrite
  (eskiden de reads RiskWrite idi → regresyon yok).
- **`shell_manage`** (birleşik, `tools/builtin_shell_bg.go`): `shell_output`/`_kill`/
  `_list` **kaldırıldı** → tek araç `action: output|kill|list`. Görünürlük: name-only.
- **Tag editörleri folded**: `set_flow_tags`/`set_schedule_tags` **kaldırıldı** →
  `update_flow`/`update_schedule` artık `tags` alanı alıyor (ayrı `SetFlowTags`/
  `SetScheduleTags` ile persist, `SetScheduleEnabled` deseni gibi). Session tag'leri
  `update_session`'da.
- Wiring: `toolsetup.go`+`toolsetup_selfmanage.go` (kayıt+MarkNameOnly/MarkHidden),
  `mcp_interaction.go` (spec+sinkToolTable), `categories.go`, frontend `toolIcons.ts`,
  `default-instructions.md`. Tüm testler yeşil (650 passed / 34 paket). Detay:
  `_Docs\24-SELF-MANAGEMENT.md`.
- **Gömülü default skiller güncellendi (2026-07-04, ayrı tur):** 3 skill (`tionswarm-guide`,
  `tionswarm-progress`, `tionswarm-self-management`) yeni araç isimlerine (`update_session`,
  `secret`) göre düzeltildi. **Bulgu:** `skills.EnsureDefaults` diske seed ederken
  **mevcut dosyanın üzerine yazmıyor** → önceden çalışmış kurulumlarda global skills
  dizini (`~/.tionswarm/skills`, tüm workspace'ler paylaşır) **genel olarak bayat**
  kalmış (11 default skill'in hepsi farklı: self-management 361, settings 306, guide
  303 satır). On-disk kopyalar güncel gömülü içerikle **elle senkronlandı** (yedek:
  `~/.tionswarm/skills-backup-20260704-preconsolidation`; `otonom-dispatch` gibi kullanıcı
  skill'lerine dokunulmadı). **Açık gap:** `EnsureDefaults` sürüm-farkında değil →
  ileride gömülü skill güncellemeleri mevcut kurulumlara otomatik yansımıyor; içerik-hash
  ile "kullanıcı düzenlememişse tazele" mantığı eklenebilir (ileride).

## Mobil/dikey ekran uyumu — F2 (modallar + header) ✅ (2026-07-04)

Modallar portrait telefonda **bottom-sheet**; header taşması giderildi. Masaüstü
birebir korunur (mobil sınıflar `max-md:`/`md:hidden` altında). `tsc -b && vite
build` temiz.

- **`common/ModalOverlay.tsx` (tek kaldıraç → 13 modal):** `< md`'de `items-end` +
  `p-0` + `max-md:[&>*]:!w-full !max-w-none !max-h-[92dvh] !rounded-b-none` → her
  modal tam-genişlik, düz-alt-köşe, 92dvh iç-scrolllu sheet. `!important` çocuğun
  sabit genişlik/yuvarlamasını ezer.
- **Elle yazılmış overlay'ler:** `ArtifactPreviewModal` (48-F2 önizleme ile
  örtüşür) + `RewindDialog` aynı desene; `PromptEditor` tam-ekran editör mobilde
  kenardan-kenara (`p-0` + çocuk `!rounded-none !max-w-none`).
- **Header (`App.tsx`):** `max-md:px-3`; sol grup `min-w-0` + ajan adı `truncate`;
  **ChatMeters `hidden md:flex`** (mobilde gizli). Detay: `_Docs\49` §7.2.

## Mobil/dikey ekran uyumu — F1 (shell + sohbet) ✅ (2026-07-04)

UI artık portrait telefonda (`< md` = 768px altı) kullanılabilir. Masaüstü
düzeni birebir korunur (tüm mobil sınıflar `max-md:`/`md:hidden` altında).

- **Yeni:** `hooks/useMediaQuery.ts` (`useIsMobile`, `max-width:767px`) +
  `components/MobileNavBar.tsx` — altta `fixed bottom-0` **yatay-kaydırılabilir**
  nav bar; tüm view'lar + Workspace/Ayarlar tek şeritte (taşanlar scroll ile),
  busy/unread/dirty noktaları, safe-area padding, `md:hidden`.
- **NavRail:** `NAV` export edildi (tek kaynak → mobil bar da tüketir); kök
  `hidden md:flex` (mobilde gizli).
- **App.tsx:** chat header'ında mobil hamburger → **SessionsSidebar** soldan
  slide-in drawer (backdrop + `translate-x`, seçimde kapanır); **SessionDetailPanel**
  sağdan slide-in drawer (`detailOpen` sürer); `<main>` `max-md:pb-16`; `<MobileNavBar>`
  render.
- Navigasyon deseni **kararlaştı**: 4-tab+drawer hibriti yerine tam yatay-scroll bar.
- `tsc -b && vite build` temiz. Kalan: F2 modallar (full-screen sheet) · F3 liste
  panelleri · F4 grafik/canvas · mobil workspace switcher. Detay: `_Docs\49` §7.1.

## Chat: worker task-notification'a özel katlanabilir kart ✅ (2026-07-04)

`Origin=worker-note` mesajlar (koordinatöre enjekte edilen `<task-notification>`
blokları) artık ham XML yerine özel bir kartla çiziliyor: yeni
`frontend/src/components/chat/TaskNotificationNote.tsx` + `MessageList`'te
worker-note dalı. Kart başlığı worker ajan adı, oturum id ve durum rozeti
(tamamlandı yeşil / başarısız kırmızı / durduruldu sarı), altında araç sayısı +
süre; result gövdesi varsayılan **katlı**, tıklayınca açılır. Parse edilemeyen
format ham metniyle katlı gösterilir (sessizce gizleme yok); DB metni değişmez.
SES104'te canlı doğrulandı (`_Docs/gorseller/coord-06-notification-card.png`),
`npx tsc --noEmit` temiz. Detay: 47 §10.

## Bağlam önizleme modalları: katlanabilir bölümler + ayrık Skills segmenti + tümünü aç/kapat ✅ (2026-07-04)

Bağlam önizleme pencerelerindeki (Oturum + Ajan) tüm bağlam segmentleri artık
tek tek **fold in/out** (katla/aç) edilebilir; ayrıca **Skills** kendi segmenti
olarak sistem promptundan ayrıldı ve header'a **Tümünü aç / Tümünü kapat**
eklendi. Canlı doğrulandı (Playwright, WS2/AGT9 + WS1/SES89).

- **Yeni primitif `common/CollapsibleSection.tsx`:** chevron + başlık gövdeyi
  açar/kapar, opsiyonel `right` node (cache etiketi, sayaç, Markdown/Ham geçişi)
  toggle düğmesinin DIŞINDA kalır (buton-içinde-buton geçersiz HTML'den kaçınır —
  kendi kontrolleri tıklanabilir), vars. açık. Yanında `useBulkToggle` hook'u +
  `BulkToggle` tipi: `{all,nonce}` sinyalini bump'layıp tüm abone bölümleri aynı
  anda açar/kapar (sonra tek tek toggle serbest); `useEffect([nonce])` ile senkron.
- **Backend — Skills segment ayrımı (`session_context.go` + `agent_context.go`):**
  önizleme yanıtına `skills`/`skillsTokens` alanları eklendi.
  `SkillsCatalogBlockForAgent(agent)` bloğu composeTurnRequest/
  buildAgentStaticPrompt'un ürettiği sistem promptundan `stripBlock` yardımcısıyla
  (tek verbatim occurrence + ayraç temizliği) çıkarılıp ayrı alana taşınır; toplam
  token korunur (sys+skills+dyn+msg+tools). Blok bulunamazsa duplikasyon yerine
  skills boş bırakılır. `List()/SharedList()` `s.order` slice tabanlı → deterministik,
  recompute-strip güvenli. Canlı: AGT9 systemTokens 11762→10689 + skillsTokens 973,
  `systemHasSkillsHeader=false`.
- **`SessionContextModal`:** bölümler katlanabilir + **bölüm sırası** (kullanıcı
  isteği, 2026-07-04): **Mesaj dizisi (modele gidecek)** en üstte → **Dinamik
  bağlam** → Sistem promptu → **Skills** (varsa) → Cache'li mesaj dizisi → Araçlar
  (değişken/model-bağlı içerik üstte, stabil cache'li prefix altta). Mesaj dizisi
  ikiye bölündü — cache öneki varsa **"Cache'li mesaj dizisi (sıcak önek)"** ayrı
  grup (vars. KAPALI, artık alt tarafta) + **"Mesaj dizisi (taze)"**; mesaj kartı
  `MessageCard`'a çıkarıldı, eski inline cache-sınırı çizgisi kaldırıldı. `Section`
  helper'ı `CollapsibleSection` sarar + `bulk` iletir. Token chip'e Skills +
  copy()'ye `# Skills` bölümü eklendi.
- **`AgentContextModal`:** bölüm sırası (kullanıcı isteği, 2026-07-04): **Dinamik
  bağlam** en üstte → Sistem promptu (Markdown/Ham `right`'ta) → **Skills** (varsa,
  Markdown/Ham'a saygılı) → lazy araçlar; hepsi katlanabilir + Skills token chip.
- Doğrulama: `go build ./...` + `go test ./internal/api/...` (73) + `npx tsc
  --noEmit` temiz. Canlı: tek-toggle (aria-expanded true→false, 5→4), Tümünü
  kapat→0, Tümünü aç→geri; Skills bölümü AGT9 modalında chip 973 + başlık render.
- Not: kullanıcının 8090'daki backend'i (dün başlatılmış eski binary) bu
  değişiklikleri içermez → yeni davranış için backend yeniden başlatılmalı
  (`.\scripts\dev.ps1`).
- **Doğruluk uyarıları (`SessionContextModal`, 2026-07-04):** önizlemenin gerçek
  wire-payload'dan bilinçli saptığı yerler için yeni `HintNote` (soft-amber callout):
  (1) Mesaj dizisi başlığında — önizleme **bu-turun compaction'ını uygulamaz**
  (yan-etkisiz; bütçeye yakın oturumda modele gerçekte gidenden fazla mesaj
  görünebilir); (2) claude-cli'da (`data.cliOverhead != null`) Dinamik bölümünde —
  dinamik ayrı system bloğu değil **son kullanıcı mesajına dokunularak** gider;
  (3) claude-cli'da Araçlar bölümünde — araçlar TionSwarm isteğinde şema olarak değil
  **CLI built-in + MCP köprüsüyle** iletilir, token yaklaşık. `npx tsc --noEmit` temiz.
- **Provider alanı + "Compaction'ı simüle et" toggle + AgentContextModal cli notu
  (2026-07-04):** her iki önizleme yanıtına `provider` alanı eklendi
  (`session_context.go`/`agent_context.go` → `agent.Provider`); `AgentContextModal`
  Dinamik bölümüne de #3 notu (`data.provider === 'claude-cli'`) taşındı.
  **Compaction simülasyonu:** yeni yan-etkisiz `conversation.Manager.SimulateCompaction`
  (Prepare'ın ön yarısı — aynı bütçe matematiği + `foldBoundary`, ama **LLM summarize
  YOK, persist YOK**) → API `?compact=1` (`compactionSimulated`/`foldedCount` alanları,
  `strconv.ParseBool`). UI'da input satırında **"Compaction simüle/açık"** toggle
  (`FoldVertical`), açıkken mesaj dizisi bu-turun katlamasını yansıtır ve #1 notu
  duruma göre değişir (kaç mesaj katlanırdı / katlanacak yok). `load(msg, compact)` +
  `simulate` state; `useEffect([simulate])` toggle'da anında refetch.
- **Canlı doğrulandı (Playwright, WS1, kendi backend 8095 + Vite 5174 → 8095):**
  SES89 (claude-cli) modalında üç not da render (`compactionOff`/`cliDinamik`/`cliAraclar`
  = true), toggle → "Compaction açık" + "simülasyon açık" + "katlanacak mesaj yok"
  (foldedCount 0, oturum küçük). AgentContextModal Holly (AGT8, claude-cli): Dinamik
  en üstte + cli notu true. Negatif: Minimax3 (minimax-anthropic) → cli notu gizli.
  Backend `go test ./internal/api/... ./internal/conversation/...` (94) + `tsc` temiz.
  Görseller: `session-ctx-cli-notes-compaction.png`, `agent-ctx-cli-dynamic-note.png`.
  Doğrulama sonrası verify-backend/Vite kapatıldı, `vite.config.ts` 8090'a geri alındı.
- **Katlanmış mesajlar ayrı grup + Özet kategorisi + varsayılan tutarlılık fix'i
  (2026-07-04):** `SessionContextModal` artık `/compact` sonrası gerçekle tutarlı.
  **Backend (`session_context.go`):** önizleme mesaj dizisi **her zaman** kalıcı özet
  sınırını (`session.SummaryMsgCount`) uygular → `liveHistory = history[start:]`
  (gerçekten gönderilen) ve `droppedHistory = history[:start]` (özete katlanmış, artık
  gönderilmeyen) ayrılır; `?compact=1` ile bu-turun ek bütçe katlaması da düşülür.
  Rolling summary `conversationSummaryBlock` `stripBlock` ile Dinamik'ten çıkarılıp
  ayrı `summary`/`summaryTokens` alanına taşınır (Dinamik'te çift sayım yok). Yeni
  alanlar: `summary`/`summaryTokens` (toplama DAHİL — katlananların yerine geçer),
  `droppedMessages`/`droppedTokens` (toplama DAHİL DEĞİL — wire'da yok). `authorsFor`
  helper'ı iki dilime de yazar rozeti verir.
  **UI:** yeni **"Özet (katlanmış mesajların yerine geçer)"** katlanabilir bölümü +
  **"Artık gönderilmeyen (özete katlanmış)"** açık-turuncu grup (vars. KAPALI);
  `MessageCard`'a `dropped` varyantı (turuncu border/bg + `DroppedTag "katlandı ·
  gönderilmiyor"`), `Stat`'a `dropped` (turuncu chip), token chip'lerine Özet +
  Katlanmış, `copy()`'ye `# Summary (folded)`. **Bulk fix:** `CollapsibleSection`
  mount'ta (nonce değişmeden) artık `bulk`'u uygulamıyor (`seenNonce` ref) →
  `defaultOpen` korunuyor (dropped/cache grupları kapalı açılır), Tümünü aç/kapat
  hâlâ çalışıyor.
  **Canlı doğrulandı (Playwright, WS2/SES2, özetli oturum SummaryMsgCount=20):**
  20 canlı + 20 katlanmış mesaj, Özet 1130 tok (Dinamik'te yok), Katlanmış 3691 tok
  (toplama dahil değil), turuncu grup vars. kapalı → açınca 20 turuncu kart +
  conversation_search ipucu; Tümünü kapat→0/aç→7. `go test` (94) + `tsc` temiz.
  Görsel: `session-ctx-dropped-summary.png`.

## M2 koordinatör/worker: LLM-in-the-loop canlı görsel deneme ✅ + non-stream CLI köprü fix'i (2026-07-03)

`_Docs/47` §10: gerçek modelle (claude-cli/opus) koordinatör oturumu (WS5/SES104)
uçtan uca doğrulandı — `spawn_worker` ×2 tek turda, koordinatör turu bloklanmadan
bitti; Koordinasyon roster'ı UI'da canlı doldu (ÇALIŞIYOR→BITTI, 3 sn poll);
`<task-notification>`'lar otomatik koordinatör turlarını tetikledi ve sentez
yazıldı (çift tur yok). Görseller: `_Docs/gorseller/coord-0*.png`.

- **Bulgu+fix:** non-stream `/api/chat` + claude-cli turunda Interaction MCP hiç
  kurulmuyordu (`interaction=false`) → köprü araçları (spawn_worker dahil) yok ve
  CLI'nin native `Agent`/`Task`'ı disallow edilmiyordu; model kendi Agent'ıyla
  fan-out yapıp M2'yi bypass etti. `toolloop.go` on-demand `autoInteract`
  kurulumundaki `autonomous` şartı kaldırıldı (endpoint'siz her CLI turu sarılır;
  stream yolu etkilenmez). `go build ./...` + agent/tools 296 test yeşil.
- Not: model, prompt'taki meşru seçenek gereği ilk istekte M1'i (`run_subagent`
  sync) seçebiliyor; denemede async M2, "use spawn_worker (NOT run_subagent)"
  yönlendirmesiyle tetiklendi.

## Otonom turların canlı adım akışı — session-step bus ✅ (2026-07-03)

**Sorun:** Chat turunda ajanın adımları (thinking/tool) canlı görünüyordu; otonom
turlarda (scheduler/spawn/worker/wake/peer) **kalıcı olarak kaydediliyordu** ama
canlı görünmüyordu — çünkü chat'in canlı akışı `chat_stream`'in **isteğe-özel** SSE'si
(`sse("step")`) üzerindendi, otonom yollar ise `onStep=nil` ile
`CompleteWithToolsTraced` çağırıyordu (trace toplanır+persist edilir, ama hiçbir yere
yayınlanmaz). Süreç-geneli `/api/events` bus'ı yalnız kaba bildirim taşıyordu.

**Yapılan — süreç-geneli canlı adım köprüsü:**
- `events.Event`'e `Step json.RawMessage` alanı (opaque TurnStep JSON; events paketi
  agent'ı import etmez — `db.Message.Steps` deseni). `handleEvents` `Type=="session_step"`
  frame'lerini ayrı SSE event adı **`step`** ile yazar (bildirim/badge yolu `notify`
  dinler → step'ler oraya karışmaz).
- `internal/agent/sessionstep.go`: `emitSessionStep`/`EmitSessionStep` +
  `SessionStepEmitter(ctx)` (ctx'te session id yoksa nil → eski yol). `busForwardable`
  yüksek-frekanslı (delta/tool_delta) ve etkileşimli (ask/permission/plan/tombstone —
  yalnız turu **sahiplenen** pencere yanıtlayabilir) adımları eler; kalan anlamlı
  aktiviteyi (thinking/tool/todo/diff/recovery/error/subagent) yayınlar.
- Choke point'ler `CompleteWithToolsTraced`→`CompleteWithToolsStream(..., emitter)`
  ile değişti: `invokeTraced` (scheduler/spawn/peer-fallback) + `wakeTurnRunner`
  (worker/coordinator/wake/peer history-aware). Dönen `steps` slice'ı **değişmedi** →
  persistence birebir aynı; yalnız canlı yayın eklendi.
- `chat_stream` onStep'i de `EmitSessionStep` ile bus'a aynalanır → **çok-pencere**
  senkronu: aynı chat turunu başka pencerede izleyen de canlı görür.

**Frontend:** `subscribeEvents(onEvent, onStep?)` tek EventSource'ta `step` frame'lerini
de dinler. `useChatStream.applyAutoStep` aktif oturum için bir **ghost asistan balonu**
(`live-auto-<sid>`) büyütür (thinking merge + tombstone); turu bu pencere sahipleniyorsa
(`runsRef`) atlar (yerel SSE zaten render eder → çift balon yok), ekran-dışı oturumda
yalnız "düşünüyor" göstergesi. Tur bitince tamamlanma event'i (`chat`/`spawned`/`worker`/
`schedule`) ghost'u temizler + transcript'i reload eder → yetkili kalıcı mesaj yerine
geçer. `go build`+vet+agent testleri yeşil, frontend `tsc --noEmit` temiz.

**Ek — tur ortasında UI yenilenince adım/agent kaybı düzeltildi:** Yenileme
sırasında tur sunucuda detached sürüyor ama taze sayfa yalnız **kalıcı** mesajları
yüklüyordu → o ana kadarki adımlar + agent adı kayboluyor, yalnız son cevapla geri
geliyordu (asistan mesajı yalnız tur bitince persist edilir). `inflight.json` sidecar'ı
(partial text+steps+agentId, throttle'lı yazılır) zaten vardı ama yalnız **boot**'ta
crash kurtarma için okunuyordu (`recoverInflight`); canlı yenilemeye açık değildi.
Eklendi: `db.ReadInflight` (exported) + `GET /api/sessions/{id}/inflight` (snapshot
veya null). Frontend: mesaj-yükleme effect'i `chat.recoverInflight(sid, msgs)` çağırır →
snapshot varsa (ve henüz persist edilmemişse) `id=messageId` ghost balonu seed eder
(agent adı + o ana kadarki adımlar geri gelir); session-step bus'ı bu balonu **canlı
büyütmeye devam eder**, tur bitince aynı id'li kalıcı mesaj yerine geçer. Turu bu
pencere sahipleniyorsa (yerel SSE) no-op. `chatRef` (App'te chat hook'una canlı handle)
mesaj-yükleme effect'i chat tanımından ÖNCE geldiği için deps-dizisi TDZ'sini atlar.
`go build`+db+api testleri yeşil, `tsc --noEmit` temiz.

## Workspace default promptu TionSwarm-native yeniden yazıldı ✅ (2026-07-03)

`internal/workspace/defaults/default-instructions.md` hâlâ the external agent project sistem
promptunun mekanik "the external agent project→TionSwarm" kopyasıydı — TionSwarm'da **olmayan**
onlarca yeteneği öğretiyor (`datatable`/`spreadsheet`, `html/pdf/markdown-preview`,
`render_template`, `call_llm`, `~/.external-agent/docs/*`, `_displayName` MCP meta,
External Sources+`guide.md` modeli), **gerçek** yüzeyi (run_subagent, use_skill,
set_session_goal, flows/self-management/handoff/plan modu, gerçek render seti) hiç
anlatmıyordu. Ayrıca ajana talimat olmayan the external agent project iç dokümantasyonu (Dynamic
context / Complete user message / SDK config bölümleri + mini-agent promptu) ve
makineye özel sızıntı (gömülü Bilal tercihleri + sabit `C:/Users/user/...` yolları)
içeriyordu.

**Yapılan:** dosya sıfırdan TionSwarm-native olarak yeniden yazıldı (~750 → ~150
satır). Tasarım ilkesi the external agent project'ın "her şeyi inline et" (~37K token) yaklaşımı
yerine TionSwarm'nun **küçük cache'li prefix + skill'e devret** felsefesi (`_Docs/17`,
`_Docs/19`): skill kataloğu + `GoalUsageHint` zaten prefix'te enjekte edildiği için
prompt artık ansiklopedi değil, doğru araç yüzeyi + skill pointer'ları. İçerik
İngilizce (kod/prompt kuralı). Render fence'leri gerçek koda göre doğrulandı
(`frontend/.../CodeBlock.tsx`: yalnız `diff`/`mermaid`/`gallery`+`image-preview`).
Document Tools bölümü **kullanıcı kararıyla korundu** ("ileride eklenecek" notuyla).

**Regresyon kilidi:** `internal/workspace/defaults_test.go` —
`TestDefaultInstructionsAreTionSwarmNative` embed'in yasak the external agent project-ism string'leri
(`datatable`/`call_llm`/`render_template`/`~/.external-agent/docs`/`_displayName`/
`html-preview`…) içermemesini ve gerçek TionSwarm terimlerini (`run_subagent`/
`use_skill`/`set_session_goal`/`mermaid`) içermesini garanti eder. Enjeksiyon yolu
`TestSystemPromptInjectsWorkspaceInstructions` ile zaten kilitli. `go build ./...` +
118 test (agent+workspace) yeşil. Not: yalnız **yeni** workspace'leri etkiler;
persisted `instructions` taşıyan mevcut workspace'ler seed'i override eder.

## Otomasyon UX: nav rename + inline edit + opsiyonel son tarih ✅ (2026-07-03)

`_Docs/46` devamı. (1) **NavRail "Zamanlamalar" → "Otomasyon"** (`NavRail.tsx` +
`App.tsx` başlık; ekran cron Zamanlamalar + Otomasyonlar'ı birlikte tutar). (2)
**Otomasyonlara inline düzenleme** (`Automations.tsx` kalem butonu → ad/tetik/hedef/
maks-iter/bekleme/son-tarih/prompt + ℹ️ değişken popover; `updateAutomation` API zaten
vardı). (3) **Opsiyonel son tarih** `Automation.ExpiresAt` (unix sn): `fire` başında
`time.Now >= ExpiresAt` → otomatik pasifle (Schedule `expiresAt` deseninin eşi);
create+update API + `create/update_automation` tool + UI datetime-local. Canlı
doğrulandı (create round-trip expiresAt saklandı; PUT edit name/expiresAt/cooldown
güncelledi). `go build ./...` + 406 test + tsc yeşil. **Ayrıca** WS5'te gerçek
**otomatik-onarım otomasyonu** kuruldu+test edildi (AGT24 Repairer sonnet, triggerTag
`tool-error`, spawnTags `["repair"]` loop-kırıcı): induced tool-error → Repairer spawn →
"false positive, no changes" doğru teşhis.

## Araç backlog P2 dalgası: `get_session_info` + `update_user_preferences` ✅ / labels-status ❌ kapsam dışı (2026-07-03)

`_Docs/41` madde 5-6-7 kapatıldı (kullanıcı kararı: 6'yı yapma, 5+7'yi yap):

- **`get_session_info` (YENİ, `tools/builtin_sessioninfo.go`):** ajan kendi oturumunun
  metadata'sını okur — id/title/state/kind/agent(ad+id)/mesaj sayısı/tags/goal/
  working_dir/role/coordinator_session/parent_session; `session_id?` ile başka oturum.
  Ctx'teki mevcut oturum (`CurrentSessionID`) default; oturumsuz turda zarif mesaj.
  Session-edit araçlarının (title/tags/goal) okuma eşi. Koşulsuz kayıt, `MarkNameOnly`
  tier, `RiskRead`, kategori `agents`; claude-cli köprüsü `BridgeTools` `extra`.
- **`update_user_preferences` (YENİ, `tools/builtin_userprefs.go`):** kullanıcıdan
  öğrenilen kalıcı bilgileri (ad/saat dilimi/şehir/ülke/tercih notları) mevcut
  **Settings ▸ Profil** alanlarına yazar (`SettingsBridge.Apply`, yalnız 5 profil
  alanı — dar sarmalayıcı). `notes` REPLACE / `notes_append` satır ekler (ikisi
  birlikte → hata; append Snapshot'tan mevcut notu okur). Profil zaten her turda
  "About the user" bloğu olarak enjekte → yeni prompt katmanı gerekmedi. Bridge
  varken kayıt, `MarkNameOnly`, `RiskWrite` (default), kategori `config`; CLI köprülü.
- **`set_session_labels`/`set_session_status` KAPSAM DIŞI:** etiketleri
  `set_session_tags` + etiket-otomasyonları (`_Docs/46`) zaten karşılıyor; durum için
  `State`+`archive_session`+Kanban yeterli. `_Docs/41` §6 gerekçesiyle işaretlendi.
- **Test:** `builtin_sessioninfo_test.go` (explicit id / ctx default / no-session /
  unknown id) + `builtin_userprefs_test.go` (alan patch, append/replace, guard'lar,
  nil bridge). `go build ./...` + tools+agent 289 test yeşil.

## Kaydedilmemiş-değişiklik belirteci: agents/artifacts kapsam + sayfa-değiştirme uyarısı ✅ (2026-07-03)

Ayarlar/Workspace/Flows ekranlarında zaten var olan "kaydedilmemiş değişiklik"
(dirty) nav belirteci **Ajanlar** ve **Artifactlar** ekranlarına da genişletildi;
ayrıca kirli bir ekrandan ayrılmaya çalışınca uyarı gösterilir.

- **Ortak altyapı (`lib/dirtySignals.ts`):** `useRegisterDirty(view, isDirty)` artık
  `view: View | undefined` kabul eder (undefined → no-op). Böylece aynı editör bir
  modalda tekrar kullanıldığında nav'ı yanlış ekrandan kirletmez.
- **Ajanlar (`agents/AgentSettingsForm.tsx`):** form alanları (name/avatar/color/soul/
  identity/provider/model/thinkingLevel/permissionMode/skills) ajanın kalıcı değerleriyle
  karşılaştırılıp `dirty` hesaplanır; yeni opt-in `dirtyView?: View` prop'u ile
  `AgentsView` `dirtyView="agents"` geçer (modal reuse geçmez). Tools bölümü anında
  kaydettiği için dirty'e dahil değil.
- **Artifactlar (`panels/ArtifactsPanel.tsx`):** açık `draft` kalıcı artifact'tan
  (title/kind/language/content) farklıysa `useRegisterDirty('artifacts', dirty)`.
- **Sayfa-değiştirme uyarısı (`App.tsx`):** `selectView` guard'ı — mevcut ekran dirty
  iken başka nav view'ine geçişte `window.confirm` onayı ister (NavRail `onSelectView`
  artık `selectView`). Ayrıca herhangi bir ekran dirty iken sekme kapatma/yenilemede
  `beforeunload` tarayıcı uyarısı. Not: workspace switch bu guard'ın dışında (kapsam
  yalnız nav view değişimi).

## Araç boşluk kapatma: arka-plan shell + apply_patch + CLI tool latency + built-in hook görünürlüğü ✅ (2026-07-03)

claude-cli built-in araç yüzeyi ile TionSwarm native araçları arasındaki boşlukların
kapatılması (the external agent project↔TionSwarm backlog `_Docs/41`).

- **Arka-plan / uzun-süren shell (`BashOutput`/`KillShell` paritesi):** `Bash`/`PowerShell`
  araçlarına `run_in_background` argümanı → detached süreç başlatıp shell id döner
  (`internal/tools/builtin_shell_bg.go`: `ShellManager` süreç kayıt defteri + `bgWriter`
  rolling ring 256KB + offset-takipli **destructive drain**; `bgShellMaxLive=16`,
  `bgShellKeepDone=16` budama). Üç yönetim aracı: `shell_output` (son okumadan beri YENİ
  çıktı + durum satırı; ring taşarsa "rolled off" uyarısı), `shell_kill`, `shell_list`
  (name-only tier). Yönetici **session-scoped** (`Runtime.shellMgrs sync.Map`,
  `shellMgrFor`), turlar arası yaşar; catalog/preview build'de nil → arka-plan devre dışı.
  Shell tool'lar `WithManager` ile bağlanır (value-type copy; eski call-site'lar
  değişmedi). Confined-git brake arka-planda da geçerli. Dev server/watcher senaryosu.
- **`apply_patch` (unified-diff, çok-hunk/çok-dosya):** `Edit`'in batch kardeşi
  (`internal/tools/builtin_patch.go`). `diff -u`/`git diff` çıktısını **context
  eşleştirmeyle** uygular (@@ satır no'ları ipucu, güvenilmez) → alakasız edit'ler satırı
  kaydırsa bile tutar; hunk eşleşmezse **o dosyanın tamamı reddedilir** (yarım uygulama
  yok). **İki-fazlı** (tüm dosyalar önce validate+freshness, sonra commit) → dosyalar
  arası all-or-nothing. `--- /dev/null` create, `+++ /dev/null` delete; `a/`,`b/` prefix
  ve `diff -u` tab-timestamp temizlenir. Freshness guard entegre (read-only ajanda lazy).
- **CLI tool latency → debug.jsonl (#3):** `emitCLIToolDebug` zaten tool olaylarını
  besliyordu ama `DurMs=0` idi. `cliStreamParser`'a `toolStart map[id]time.Time` eklendi:
  `tool_use` görülünce saat başlar, `tool_result` gelince `TraceStep.DurMs` = gerçek
  wall-clock (stream satır-satır `ReadString` ile real-time beslendiği için CLI-içi araç
  gecikmesi doğru). `provider.TraceStep.DurMs` yeni alan; native/CLI arası tek-tip tool
  metriği tamamlandı.
- **Built-in/auto-injected hook görünürlüğü (salt-okunur):** yeni `GET /api/hooks/builtins`
  (`api/hooks.go handleListBuiltinHooks`) app-settings snapshot'ından hesaplanan 8 yerleşik
  davranışı döner (freshness guard, CLI native-tool bridging, Bash→PowerShell, CLI hook
  passthrough, permission deny-list, plan-mode approval, autonomous git brake, hook
  fail-open) — `enabled` toggle'lanabilirler için `FileFreshnessGuard`/`EnableShell`/
  `EnableCLIHooks`/`AutonomousConfine`'dan. Frontend: `HooksPanel.tsx` altına dashed-border
  read-only bölüm (scope/event badge + ayar anahtarı + Aktif/Pasif); `types/hook.ts
  BuiltinHook` + `api/hooks.ts listBuiltinHooks`.
- **#2 (freshness-guard'ı CLI yoluna taşı) YAPILMADI — bilinçli:** CLI'nin **native**
  Read/Edit/Write'ı zaten kendi read-before-write guard'ını uyguluyor (TionSwarm'nun
  `readtracker.go`'su bunu "mirrors Claude Code's readFileState guard" diye kopyaladı).
  Hook tabanlı ikinci guard redundant + kırılgan olurdu (hook executor'ı değiştiremez,
  yalnız deny/observe/updatedInput; `updatedInput` Edit'te bug'lı #47853).
- **Test/derleme:** `internal/tools/builtin_patch_test.go` (update/create/delete/mismatch/
  freshness/multi-hunk), `builtin_shell_bg_test.go` (ring drain/overflow, unknown-id, nil
  manager, gerçek arka-plan echo). `go build ./...` + 430 test (4 paket) + `tsc --noEmit` yeşil.

## Etiket-Otomasyon: genişletilmiş değişkenler + info popover + olay-bazlı otomatik etiketleme ✅ (2026-07-03)

`_Docs/46`'nın devamı (etiket + otomasyon çekirdeği 2026-07-02).

- **PromptTemplate değişkenleri 4→13:** `renderAutomationPrompt` + yeni `turnVars`
  (`agent/automation.go`) → {{result}}/{{title}}/{{tag}}/{{sessionId}}/{{iteration}}/
  {{maxIterations}}/{{agent}}(+{{agentName}})/{{prevPrompt}}/{{automation}}/{{date}}/
  {{time}}/{{datetime}}. Bilinmeyen `{{...}}` aynen kalır; {{result}} yoksa sona eklenir.
- **UI info popover:** `panels/Automations.tsx` prompt alanına ℹ️ butonu → 13 değişkeni
  açıklamalı listeler; satıra tıkla → şablona ekler (`PROMPT_VARS`).
- **Olay-bazlı otomatik etiketleme** (yeni `agent/autotag.go`): tur olaylarına göre
  well-known etiketler (ADD-only): `tool-error` (gerçek tool hatası; claude-cli
  **disallowed-tool** reddi `isPermissionDenyError` ile HARİÇ), `error` (tur-seviyesi
  hata, "stopped" hariç), `goal`/`goal-done`/`archived` (durum). `Runtime.AutoTagTurn`
  chat(başarı+cerr)/spawn/schedule/wake yollarında; `archived` ayrıca arşiv mutasyonunda
  (`sessionSink.Archive` + API state handler, geri yüklemede silinir). Amaç: bir
  otomasyonla hataları tarayıp otomatik onarmak. **Ayar toggle'ı** `AutoTagSessions`
  (vars. açık; Ayarlar ▸ Bağlam) → `AutoTagTurn`/`AutoTagEnabled`/`sessionSink.autoTag`
  guard'ları; kapalıyken hiç otomatik etiket yazılmaz. Canlı: OFF→yazmıyor, ON→yazıyor.
- **Canlı doğrulama (WS2, sonnet/claude-cli):** loop (sayaç 10→11→12, story zinciri,
  2 senaryo paralel, maks-iter'de auto-disable); genişletilmiş değişkenler render;
  autotag: archived ekle/sil, goal, var-olmayan dosya Read → tool-error. Testler:
  `db/automation_test.go`, `agent/automation_test.go`, `agent/autotag_test.go`;
  spawn testlerine `drainSpawns` (fire-and-forget goroutine'i TempDir cleanup'tan önce
  beklet → Windows dosya-kilidi flakiness giderildi). `go build ./...` + 635 test + tsc yeşil.

## Fix: `run_subagent` hedef gölgeleme + sync timeout (WS8/SES1 teşhisinden) ✅ (2026-07-03)

**Belirti:** Superpowers pipeline'ında (Orchestrator=claude-cli) `run_subagent`
"Reviewer"/"Verifier" fazlarında tutarlı başarısız: async → `async subagents require
an existing agent target, not a profile`; sync → `The operation timed out.`

**Kök neden 1 (gölgeleme):** `resolveSubagentTarget` önce built-in profillere
(`explore`/`coder`/`reviewer`) bakıyordu → kullanıcının gerçek "Reviewer" (AGT6) ajanı
`reviewer` profiliyle gölgelenip ephemeral çözülüyordu; ephemeral async'i reddettiği için
hata. **Fix:** önce mevcut ajana bak, bulamazsa profile düş (gerçek ajan kazanır).
Async+ephemeral reddi provider/bütçe işinden **önce** açıklayıcı mesajla (Guard 4).

**Kök neden 2 (timeout):** sync `run_subagent`'ta alt-ajanın tüm işi tool çağrısında
koşuyor ve claude-cli'nin ~60 sn MCP araç-çağrısı timeout'unu aşıyor → CLI `The operation
timed out.` verir (TionSwarm işi arka planda bitirir). **Fix:** `claudecli.go runAttempt`
CLI process'ine `MCP_TOOL_TIMEOUT=600000` + `MCP_TIMEOUT=60000` ms enjekte eder (kullanıcı
override kazanır → `ensureEnvDefault`).

**Dokunulan:** `internal/agent/subagent.go` (çözümleme sırası + Guard 4), `internal/
providers/claudecli.go` (`ensureEnvDefault` + MCP timeout env), testler
`TestResolveSubagentAgentBeatsProfile`/`TestAsyncProfileRejected`. Doküman `_Docs\25` +
skill `tionswarm-session-debug` (desen G/H). `go build`/`go test ./internal/agent
./internal/providers` (186) yeşil.

## Feature: Read/Grep/Glob araç-paritesi — offset/limit + satır no, ripgrep-stili Grep, mtime Glob, .gitignore ✅ (2026-07-03)

**İstek:** Claude Code'un fs araçlarında olup bizde olmayan per-tool özellikler
(`_Docs/41` Bölüm E): Read satır-aralığı + numaralama, Grep output-mode/context/-i/
type/multiline/head_limit, Glob mtime sıralama + path, ve .gitignore farkındalığı.

**Çözüm:**
- **`Read`** (`builtin_fs.go` `renderNumbered`): çıktı artık **satır-numaralı** (`%6d\t…`,
  cat -n stili; Edit için "önek+tab'ı sıyır" notu açıklamaya eklendi). **`offset`**
  (1-tabanlı başlangıç) + **`limit`** (satır sayısı, vars. 2000) ile büyük dosyanın
  penceresi okunur; satır-başı karakter cap'i (2000) + 256KB çıktı cap'i + "devam:
  offset=N" ipuçları. Tazelik hash'i tam içerik üzerinden (pencere kısmi görünüm).
- **`Grep`** (yeni `builtin_grep.go`): `output_mode` (content/files_with_matches/count),
  context `-A`/`-B`/`-C` (bitişik pencereler birleşir, `--` ayraç), `-i`, `-n` (vars.
  açık), `-o` (yalnız eşleşen), `type` (dil→uzantı haritası), `multiline` (`(?s)`),
  `head_limit`, `path` (dosya/dizin), `no_ignore`. Eşleşen satır `path:line:text`,
  bağlam `path-line-text` (ripgrep konvansiyonu).
- **`Glob`** (yeni `builtin_glob.go`): sonuçlar **mtime'a göre** (en yeni önce) sıralı;
  `path` (arama kökü) + `no_ignore` argümanları.
- **`.gitignore` farkındalığı** (yeni `ignore.go` `IgnoreSet`): Grep+Glob kök+iç-içe
  `.gitignore`'ları (lazy) + daima `.git`'i atlar; dizin eşleşince `SkipDir` ile tüm
  alt-ağaç elenir. `*`/`**`/`?`, `!` negasyon, dir-only `/`, anchored `/` desteklenir
  (byte-perfect Git değil; node_modules/.git/dist gürültüsünü keser). `no_ignore` ile
  kapatılır. Yeni bağımlılık yok (ripgrep binary'sine bağlanmadan saf-Go).
- Eski `FSGlobTool`/`FSGrepTool` `builtin_fs.go`'dan yeni dosyalara taşındı;
  `globToRegexp`/`isBinary` paylaşımlı kaldı.
- **Grep `rg` hızlı yolu (opsiyonel, 2026-07-03):** PATH'te `rg` (ripgrep) varsa Grep
  otomatik ona delege eder (`grep_rg.go` `tryRG`) — daha hızlı + native tip/ignore. Bayrak
  eşlemesi: `--no-require-git --hidden` (Go `IgnoreSet` semantiğiyle eşleşir: repo olmadan
  `.gitignore`'a uy + dotfile'ları ara, `.git` daima atlanır), `--path-separator /` (Windows
  `\`→`/` normalizasyon), output_mode→`--no-heading`/`--files-with-matches`/`--count-matches`,
  `-A/-B/-C`, `--ignore-case`, `--only-matching`, `--multiline --multiline-dotall`,
  `--glob`, tip→uzantı-glob'ları, `--no-ignore`, `--regexp` (dash-güvenli). Çıktı Go
  motoruyla **aynı şekle** normalize edilir (lider `./` sıyrılır, head-limit uygulanır).
  **Herhangi bir belirsizlikte** (rg yok / bilinmeyen mode/tip / rg exit≠0/1 / timeout)
  sessizce **Go motoruna düşer** → davranış her iki yolda birebir. `TIONSWARM_GREP_NO_RG=1`
  ile kapatılır. rgExe env kontrolü `sync.Once` DIŞINDA (test-toggle edilebilir; LookPath cache'li).
- **Edit satır-no toleransı (Read numaralama davranış değişikliği için):** Read çıktısı artık
  `<no>\t<içerik>` numaralı; model bazen bu öneki `old_string`'e kopyalar. Doğrudan eşleşme
  0 olduğunda Edit, `old_string` VE `new_string`'den `cat -n` öneklerini (`^ *\d+\t`,
  `stripCatNPrefixes`) sıyırıp yeniden dener — eşleşirse temiz uygular (numaralı paste
  artefaktı dosyaya yazılmaz). Yalnız verbatim eşleşme başarısızsa devreye girer; başarılı
  eşleşmeyi asla değiştirmez.
- **Test:** `TestFSReadWindow`, `TestFSEditToleratesLineNumbers`, `TestGrepOutputModes/
  Context/OnlyMatching` (Go motoruna sabitli), `TestGrepRGFastPath` (rg yoksa skip),
  `TestGlobMtimeSortAndIgnore/PathArg`; mevcut Read-eşitlik testleri `Contains`'e
  güncellendi. `go build`/`vet`/`test` (366) yeşil. (`run_in_background` paralel bir çalışmada
  `builtin_shell_bg.go` `ShellManager` + `shell_output`/`shell_kill`/`shell_list` ile
  ayrıca eklendi — tazelik guard'ı eşzamanlı düzenlemede duplikat yazımı engelledi.)

## Feature: Dosya tazelik guard'ı — Edit/Write "read-before-write" (Claude Code paritesi) ✅ (2026-07-03)

**İstek:** Claude Code'un Edit toolundaki *"File has been modified since read… Read
it again before attempting to write it"* tespiti bizde yoktu; ekleyelim.

**Sorun:** TionSwarm'nun `Edit`/`Write` araçları önceki bir `Read`'i takip etmiyordu →
bir oturum dosyayı okuduktan sonra dosya dışarıdan (kullanıcı/linter/başka tool)
değişse bile edit **sessizce üzerine yazıyordu** (stale-write footgun).

**Çözüm:** Session-scoped **`ReadTracker`** (içerik-hash tabanlı tazelik temeli):
- **`internal/tools/readtracker.go`** (yeni): `ReadRecord{ModTime,Size,Sum(sha256),
  Partial}` + concurrency-safe `ReadTracker` (nil = no-op, guard kapalı). `Read`
  aracı okuma anında (truncation ÖNCESİ, tam içerik hash'i → >256KB dosyalar da
  kapsanır) temeli kaydeder. `checkFreshness` = kayıt yok → *"file has not been read
  yet"*, içerik hash'i uyuşmuyor → *"file has been modified since it was last read"*.
  Karşılaştırma mtime değil **içerik-hash** (cloud-sync/AV kaynaklı sahte mtime
  bump'larında yanlış-pozitif yok, mtime değişmeyen gerçek edit'i de yakalar).
- **`builtin_fs.go`:** `Read` kaydeder; `Edit` mutasyondan önce `checkFreshness`;
  `Write` yalnız **var-olan** dosyada guard (yeni dosya prior-read istemez); ikisi de
  yazımdan sonra temeli yeni içeriğe tazeler (`recordWritten`) → aynı dosyada edit
  zinciri araya Read istemez. Tool açıklamalarına "önce Read" notu eklendi.
- **Ayar/gating:** `Tunables.fileFreshnessGuard` (default **açık**) + `settings.
  FileFreshnessGuard` (Default/public/patch/store + `server.go applySettings`); default
  skill `tionswarm-settings`'e belgelendi. `buildRegistry` session-id'yi (`SessionIDFrom`)
  çözüp `Runtime.readTrackers` (sync.Map, session-başına kalıcı) üzerinden tracker'ı
  3 fs tool'a bağlar; katalog/preview build'lerinde (session yok) veya ayar kapalıysa
  **nil** → guard devre dışı. Yalnız **native** yol; claude-cli'nin kendi karşılığı var.
- **Test:** `TestFSFreshnessGuard` (edit-before-read / write-before-read / after-read
  ok / external-change → stale / new-file-ok / nil-guard-off) + mevcut `TestFSWriteReadEdit`
  tracker'lı güncellendi. `go build`/`vet`/`test` (273) yeşil.

## Feature: Skiller için toplu "Grup ata" (bulk set-group) ✅ (2026-07-03)

**İstek:** Skilleri toplu seçince topluca gruplarını setleyebilelim.

**Çözüm:** Skills ekranı zaten çoklu-seçim (`useMultiSelect`) + `SelectionBar`
(toplu görünürlük + sil) taşıyordu; buna **toplu grup atama** eklendi.
- **Backend:** `Store.SetGroup(slug, group)` — SKILL.md'nin yalnız `group`
  frontmatter'ını `setFrontmatterFields` ile yeniden yazar (gövdeye/diğer alanlara
  dokunmaz), `category` alias'ını düşürür (ikisi çelişmesin), boş grup → grupsuz.
  API `PUT /api/skills/{slug}/group` (`handleSetSkillGroup`). Diğer `Set*` toggle
  endpoint'leriyle aynı desen.
- **Frontend:** `api.setSkillGroup(slug, group)`; `SkillsPanel` `bulkSetGroup` seçili
  her slug için paralel çağırır (`Promise.all`), sonra listeyi tazeler ve seçimi
  korur (zincirleme aksiyon). `SelectionBar`'a datalist'li (mevcut grup adları öneri)
  grup input'u + "Ata"/"Grupsuz" butonu; Enter da uygular.
- **Test:** `store_test.go TestSetGroup` (ata / category-alias düşür / boş=grupsuz /
  gövde-korunur / eksik-skill hata). `go build`/`test` + `tsc` yeşil.

## Antigravity CLI (`agy`) provider'ı KALDIRILDI ✅ (2026-07-03)

Deneysel `antigravity-cli` provider'ı (2026-06-29'da eklenmişti; tarihsel kayıt
aşağıda) **tamamen kaldırıldı**: upstream non-TTY bug'ı (#76) düzelmedi ve ConPTY
workaround'u istenmedi → ölü deneysel kod taşımak yerine temizlendi. Silinen:
`providers/antigravitycli.go` + `kind_antigravity.go` + live test;
`kind.go`/`registry.go`/`kind_test.go` arındırıldı. Artık **5 provider kind'ı**
(claude-cli/anthropic/minimax/minimax-anthropic/openrouter). Dış-ajan adaptörü
olarak sırada **Codex** (MCP delegasyonlu) duruyor.

## Feature: Çok-Ajan Koordinasyonu — M2 Koordinatör/Worker ✅ (2026-07-03)

**İstek:** Bir ajanın paralelde 4-5 ajanı koordine etmesi (Claude Code'un
koordinatör modu gibi) + farklı koordinasyon yöntemleri.

**Çözüm (M2 koordinatör/worker + M1/M3/M4 birleşik çatı):** Bir oturum
`Role="coordinator"` yapılınca koordinatör sistem promptu + dört araç açılır:
`spawn_worker` (async worker = mevcut ajan hedefli arka-plan oturumu), `send_to_worker`
(yüklü bağlamla devam), `stop_worker` (iptal→killed), `list_workers`.

- **Geri bildirim halkası:** worker turu bitince (`runWorker`, başarı/başarısız/killed)
  sonuç `<task-notification>` olarak koordinatör oturumuna enjekte edilir
  (`NotifyCoordinator`, `Origin="worker-note"`) ve **per-session tur kuyruğu**
  (`coordSlot` + `enqueueCoordinatorTurn`/`drainCoordinator`) bir koordinatör turu
  tetikler. Eşzamanlı bitişler **serileşir**; koordinatör meşgulken biriken
  bildirimler tek turda **coalesce** olur (çift-tur yarışı yok — kritik test yeşil).
- **Recursion engeli:** araçlar context-injection (`tools.WithCoordination`) ile YALNIZ
  koordinatör oturumunda kayıtlı → worker worker spawn edemez.
- **Guard'lar:** `CoordinatorMaxWorkers` (8) + `CoordinatorMaxTurns` (50) +
  `SetCoordinatorLimits`.
- **Session modeli:** `Role` + `CoordinatorSessionID`; worker `Kind="worker"`.
  `turnHook` → çoklu `turnHooks` (`AddTurnHook`), otomasyonu ezmeden.
- **Prompt/skill:** `api/coordinator_prompt.go` (`composeTurnRequest`'te koşullu enjekte,
  wake yolunu da kapsar) + gömülü default skill `tionswarm-coordinator`.
- **API:** `session_info`'ya `role`+`coordinatorSessionId`; `PUT /api/sessions/{id}/role`
  + `GET /api/sessions/{id}/workers`.
- **UI:** `CoordinatorSection.tsx` (aç/kapa + canlı worker roster, running varken 3sn
  poll) SessionDetailPanel'de; `worker`/`coordination` SSE tipleri executions'a bağlı.
- **Sapma:** tasarımdaki ayrı `CoordinationEngine` turn-hook yerine geri bildirim
  `runWorker` içinden doğrudan (runSpawn başarısız turda FireTurnFinished çağırmıyor →
  hook yolu worker hatalarını iletemezdi).
- **Test:** `agent/coordination_test.go` (kuyruk serileştirme+coalescing, worker cap,
  coordinator-link, notification format). `go build ./...` + `tsc` yeşil.

### İkinci tur: kalan adımların tamamı ✅ (2026-07-03)

- **CLI köprüsü:** koordinasyon araçları `BridgeTools`'a eklendi → claude-cli
  koordinatör de `spawn_worker/...` sürebilir (advertise + `dispatchCoordinationBridge`;
  `autonomous_interaction` ctx `WithSessionID` stamp).
- **Ayar UI'si:** `settings.CoordinatorMaxWorkers`/`CoordinatorMaxTurns` (ana+maskeli+
  patch + clamp 1–64 / 1–500) → `applySettings`→`SetCoordinatorLimits`; frontend
  `AppToolsPanel` "Koordinatör limitleri". Canlı doğrulandı (8/50→5/42).
- **M3 scratchpad:** koordinatör + worker'lar için ortak dizin
  (`<SessionDir(coordID)>/scratchpad`), `coordinationScratchpadBlock` context'e enjekte.
- **Efemeral worker hedefi:** `spawn_worker` profil hedefi (explore/coder/reviewer)
  `resolveWorkerTarget` ile kalıcı `worker:<profile>` ajanına materyalize (base'den
  klon, `profileWorkerMu` dup guard). Test: `TestSpawnWorkerMaterializesProfile`.
- **Canlı doğrulama:** backend boot + API smoke (rol set/get, `/workers`, ayar
  round-trip) uçtan uca geçti. `go build ./...` + **624 test** + `tsc` yeşil.
- **Kalan (opsiyonel):** LLM-in-the-loop görsel deneme (dev'de). Detay: `_Docs\47`.

## Feature: İçe aktarılan skiller için "Grup" (import namespace) ✅ (2026-07-02)

**İstek:** Markette başka kaynaklardan/linklerden skiller indirilebiliyor; içe
aktarırken **bir grup içinde** aktaralım ki mevcut skiller'e karışmasın.

**Çözüm (ingest pipeline'a `Group` opsiyonu):**
- `ingest.Options.Group` eklendi — doluysa her içe aktarılan skill'in `group`
  frontmatter'ı olarak yazılır (Skills UI'da tek katlanabilir başlık altında toplanır;
  `Skill.Group` zaten vardı). Boşsa kaynağın kendi `group`'u **korunur** (eskiden
  tamamen düşüyordu), doluysa onu **ezer**.
- `skills.RenderImportedSkill`/`mapCCSkill` imzasına `group` parametresi; skill +
  command adapter'ları `opts.Group` geçiriyor. Tek-skill import (`ImportCCSkill`) yolu
  `""` geçerek kaynağın kendi grubunu korur.
- API: `POST /api/ingest/install` `group`, `installRequest.group` (source-ref/directory-
  site kurulumu) — her iki install yolu `Options.Group`'a bağlı.
- UI (`SkillImportDialog`): "Grup (opsiyonel)" alanı; tarama sonrası grup **kaynak
  adından otomatik ön-doldurulur** (`deriveGroup`: `owner/repo`/GitHub tree URL → repo,
  yerel yol → son klasör), kullanıcı düzenleyene kadar. Elle düzenlenince ön-doldurma
  durur.
- Test: `import_test.go TestMapCCSkillGroup` (açık grup yaz / kaynak grubu koru / açık
  grup ezer). `go build`/`test` + `tsc` yeşil.

## Fix: sqz/rtk hook'u PowerShell aracını kaçırıyordu ✅ (2026-07-02)

**Bulgu:** Ayarlar ▸ Dış Araçlar'daki tek-tık "Bağla", sqz/rtk hook'unu
`matcher: 'Bash'` ile kuruyordu. `shell` aracı 2026-07-01'de `Bash` + `PowerShell`
olarak bölününce, Windows'ta ajanlar shell komutlarını **`PowerShell`** aracıyla
çağırdığından hook **hiç eşleşmiyordu** → sqz/rtk sessizce devreye girmiyordu.
Örnek oturum (`WS6/SES1`) `debug.jsonl`'inde onlarca `PowerShell` çağrısı var ama
sıfır `hook` olayı; sqz/rtk'nin kendisi CLI testinde PowerShell komutlarını sorunsuz
sıkıştırıyor (sqz `tool_name`'i umursamıyor, rtk sadece `rtk ` ekliyor) — yani tek
kusur matcher'daydı.

**Çözüm:**
- `agent/hooks.go` `hookMatches` artık **virgülle ayrılmış alternatif** glob'ları
  destekliyor (`Bash,PowerShell` → herhangi biri eşleşirse tetiklenir; `filepath.Match`
  süslü parantez desteklemediği için). Boşluk-toleranslı.
- `ExternalToolsPanel.tsx` rtk+sqz template matcher'ları `Bash` → `Bash,PowerShell`.
- `_Docs/18-HOOKS.md` matcher bölümü + Windows uyarısı güncellendi.
- Regresyon: `hooks_test.go` `TestHookMatches`'e 5 virgül-alt vakası.

**Not:** Eski workspace'lerde matcher `Bash` kalmış hook'lar elle `Bash,PowerShell`
yapılmalı (veya kaldırıp yeniden "Bağla"). `go build`/`test` + `tsc` yeşil.

## Code Execution with MCP — Faz 3: canlı A/B ölçümü ✅ (2026-07-02)

**İstek:** `_Docs/44` §5 Faz 3 — klasik tool-loop vs kod-modu, gerçek LLM + gerçek
MCP sunucusuyla uçtan uca karşılaştırma.

**Düzenek:** Geçici ikinci TionSwarm instance'ı (ayrı port + temp data dir — canlı
örneğe dokunulmadı), native ajan **minimax-anthropic/MiniMax-M3** (anthropic
anahtarı geçersiz çıktı), gerçek `sqz-mcp`; görev: 5 dizinin girdi sayımı
(ground truth 336). Metrikler `debug.jsonl`.

**Sonuç (özet — tam tablo `_Docs/44` §12):**
- **A klasik:** ✅ 336 · 3 iterasyon · in+out 19.098 tok · bağlama 4.366 B araç çıktısı · 7,3 s
- **B1 kod (naif):** ❌ **533 — yanlış!** Script sıkıştırılmış dönüşü `len()` ile
  saydı; model veriyi görmediği için fark edemedi → **§7 doğruluk riski canlı
  doğrulandı** (ölçümün en değerli çıktısı). Doğru olsaydı: in+out −%13, bağlama
  giren araç verisi −%91 (373 B).
- **B2 kod (format-bilinçli):** ✅ 336 · 10 iterasyon · 7 run_code · 25 köprü çağrısı ·
  in+out 22.239 · 55 s — model sqz formatını script içinden keşfetti (`expand(hash)`),
  binding docstring'ini Read'le okudu; on-demand tanım okuma tasarımı sahada çalıştı.
- **Zincir uçtan uca doğrulandı:** Settings toggle canlı → run_code kaydı → binding +
  köprü + izin + `via run_code` debug olayları + katlanabilir trace kartı (5 alt satır).
- **Dürüst not:** sqz-mcp kod-moduna en aleyhte senaryo (çıktılar zaten sıkışık);
  büyük-çıktılı tekrar (mcp-chrome/playwright) sıradaki hedef. `run_code`
  açıklamasına "opak dönüşte önce küçük örnek print et" nudge'ı **eklendi**
  (2026-07-03, `builtin_runcode.go` "ACCURACY:" paragrafı).

## Etiketler + Etiket-Tetikleyicili Otomasyonlar ✅ (2026-07-02)

**İstek:** (1) Sohbet/flow/schedule kayıtlarına etiket (tag) ekleyebilmek — hem
kullanıcı UI'dan hem ajan araçlarla düzenleyebilsin. (2) Schedules ekranına yeni
"otomasyon" türü: belirli bir etikete sahip oturum bir turu **bitirince**, o
oturumun sonucunu alıp yeni bir oturum başlatan → kendiliğinden süren döngüler.

**Tasarım kararı (kullanıcıyla netleşti):** Tetikleyici = etiketli oturumda bir tur
`end_turn` ile bitince (araçlar kullanılıp son cevap verilince). Ham "her tur"
sohbet selini **tag-gating** (yalnız tetik etiketi taşıyan oturumlar) +
guardrail'ler (maks. iterasyon, cooldown, aç/kapa) ile önlenir. Otomasyon ayrı bir
`Automation` entity'sidir ama UI'da Schedules ekranında ayrı bölümde gösterilir.

**Yapılan (backend):**
- **Etiket alanları:** `Session.Tags` / `Flow.Tags` / `Schedule.Tags` (`[]string`,
  omitempty; `Task.Tags` zaten vardı). Store setter'ları `SetSessionTags` /
  `SetFlowTags` / `SetScheduleTags` + paylaşımlı `normalizeTags` (trim/dedup/boş-at).
- **`Automation` modeli** (`db/models_automation.go` + `store_automation.go`): CRUD +
  `SetAutomationEnabled` (aç→sayaç sıfır) + `RecordAutomationFire` + `ResetAutomationCount`.
  Alanlar: TriggerTag, TargetAgentID, PromptTemplate (13 değişken: {{result}}/
  {{title}}/{{tag}}/{{sessionId}}/{{iteration}}/{{maxIterations}}/{{agent}}/
  {{prevPrompt}}/{{automation}}/{{date}}/{{time}}/{{datetime}} — `turnVars`, 2026-07-03
  genişletildi + canlı doğrulandı), SpawnTags (nil→[TriggerTag]=döngü), Enabled, MaxIterations (vars.
  50, 0=sınırsız), CooldownSec, IterationCount/LastFiredAt/LastSessionID/LastError.
  Yeni id prefix `AUT`, dir `automations`, `load()`'a eklendi.
- **Tur-tamamlanma hook'u:** `Runtime.turnHook` + `SetTurnHook` + `FireTurnFinished`
  (detached goroutine → turu bloklamaz). Çağrı yerleri: chat_stream (her yanıt
  sonrası), spawn (`runSpawn`), scheduler (`deliverPrompt` + `deliverWake`).
- **`AutomationEngine`** (`agent/automation.go`): `OnTurnFinished` → biten oturumu
  yükler, etiketsizse hızlı döner; eşleşen enabled otomasyonlar için cooldown +
  maks-iterasyon (aşılırsa otomatik pasifle + event) kontrolü → `renderAutomationPrompt`
  → `SpawnSession(Tags=spawnTags)` → `RecordAutomationFire`. `SpawnOptions.Tags`
  eklendi (spawn'lanan oturum oluşturulurken etiketlenir → race yok).
- **API:** `PUT /api/sessions|flows|schedules/{id}/tags` + automations CRUD
  (`GET/POST /api/automations`, `PUT/POST toggle/POST reset/DELETE /{id}`).
  `SessionInfo`'ya `tags` eklendi.
- **Araçlar (self-management):** `set_session_tags` (sink, add/remove/replace),
  `set_flow_tags`, `set_schedule_tags`, `create/update/delete/list_automation`
  (provenance: ajan yalnız kendi oluşturduğunu düzenler/siler). `SessionSink`
  arayüzüne `Tags`/`SetTags` eklendi.

**Yapılan (frontend):**
- Types: `Session/Flow/Schedule.tags`, yeni `Automation`, `SessionInfo.tags`.
- API client: `setSessionTags/setFlowTags/setScheduleTags` + automation CRUD.
- Yeniden kullanılabilir `common/TagEditor.tsx` (chip editörü, Enter/virgül ekler,
  Backspace son etiketi siler). Bağlandı: SessionDetailPanel (Hedef altına
  "Etiketler" bölümü), FlowsPanel (meta toolbar), Schedules (satır içi).
- Yeni `panels/Automations.tsx` — Schedules ekranında "Otomasyonlar" bölümü:
  oluşturma formu (ad/tetik-etiket/hedef-ajan/prompt/maks-iter/cooldown) + liste
  (aç-kapa, iterasyon sayacı, spawn-etiket editörü, limit dolunca sıfırla, sil).

**Doğrulama:** `go build ./...` + 143 test (db+agent, yeni automation_test'ler dahil)
+ 73 api test + `npx tsc --noEmit` yeşil.

**Sıradaki:** Canlı loop doğrulaması (gerçek sağlayıcıyla uçtan uca); opsiyonel
`tionswarm-autonomous-ops` skill'ine "etiketle döngü kur" reçetesi.

## Code Execution with MCP — Settings toggle + UI trace kartları ✅ (2026-07-02)

**İstek:** Kod-modunu env-only olmaktan çıkarıp Settings'e almak + `run_code` içi
MCP çağrılarını sohbet trace'inde kart olarak göstermek (`_Docs/44` kalan işler).

**Yapılan:**
- **Settings toggle `enableCodeMode`:** `settings.go` (Settings/DTO/Patch/mapping) +
  `store.go` apply + `api/server.go` `applySettings → SetCodeMode` (canlı) +
  `app.go`'da `TIONSWARM_CODE_MODE` artık `EnableShell` gibi **tek seferlik boot seed**
  (doğrudan tunable set kaldırıldı; source of truth Settings ekranı).
- **Frontend:** `types/settings.ts` + `SettingsPanel` patch'i + `AppToolsPanel`'e
  toggle ("Kod-modu (run_code + MCP binding'leri)"); kabuk kapalıyken sarı uyarı
  kutusu (run_code kabuk yetkisi olmadan kaydedilmez).
- **Trace kartları:** `codemode.CallObservation`'a `Args` alanı (yalnız UI trace'i —
  model bağlamına girmez); `toolsetup` observer'ı her script-içi çağrıyı call-ctx
  sub-step sink'ine `StepTool` olarak ekler (mutex'li — çok-thread'li script
  eşzamanlı çağırabilir) → tool loop'un mevcut generic promotion'ı `run_code`
  kartını katlanabilir `StepSubagent` yapar. **Frontend değişikliği gerekmedi**
  (run_subagent kartıyla aynı render). Girdi 2KB cap (`capStepInput`), çıktı satırı
  "N KB in M ms"; red `permission_denied` reason'lı hata satırı.
- **Doğrulama:** `go build ./...` + 349 test (codemode/tools/agent/settings/api) +
  `npx tsc --noEmit` yeşil.

**Sıradaki:** Faz 3 A/B ölçümü (`_Docs/44` §11 metriğiyle).

## Code Execution with MCP — Faz 0: baseline ölçümü ✅ (2026-07-02)

**İstek:** `_Docs/44` §5 Faz 0 — kod-modu kazancını ölçebilmek için gerçek MCP
ortamında occupancy baseline'ı.

**Yapılan:**
- **Yeni ölçüm aracı `cmd/measure-codemode`** (main/servers/report): data dizinindeki
  tüm workspace'lerin etkin MCP sunucularına gerçekten bağlanır (`mcp.ListServerTools`),
  üç senaryoyu raporlar: eager-full / tier-lazy (bugünkü default) / code-mode.
  Token tahmini `conversation.EstimateText` (runtime'la aynı). Read-only, tekrar
  koşulabilir (Faz 3'te aynı araçla karşılaştırılacak).
- **Gerçek sonuçlar (4 sunucu, 72 araç):** eager-full **67.050 B ≈ 17.049 tok/tur** ·
  tier-lazy **≈69 tok/tur** (−%99,6; +~236 tok/aktive araç) · code-mode **≈374 tok/tur**
  (−%97,8; binding'ler diskte 101,7 KB = 0 bağlam). context-mode sunucusu bayat
  konfig (ulaşılamıyor).
- **Gerçek kullanım profili (28 oturum, 204 llm_call):** in p50 1.967 · cacheRead
  p50 68.299 · out p50 515 tok; 336 araç olayının yalnız ~5'i gerçek harici MCP.
- **Dürüst bulgu:** şema-occupancy'yi tier-lazy zaten çözmüş (69 tok < 374 tok!) —
  kod-modunun gerçek değeri **aktivasyon churn'ü + ara verinin bağlam dışında kalması
  + tur sayısı düşüşü**. Faz 3 ölçüm metriği buna göre güncellendi: görev-başına
  toplam token + tur sayısı + bağlama giren araç-çıktısı baytı (şema tokenı değil).
  Detay + tablolar: `_Docs/44` §11.

**Sıradaki:** Faz 3 — MCP-yoğun çok-adımlı senaryoda klasik vs kod-modu A/B
(`turn-debug` ile görev-başına toplam maliyet).

## Code Execution with MCP — Faz 2: per-call izin + gözlemlenebilirlik ✅ (2026-07-02)

**İstek:** `_Docs/44` §5 Faz 2 — kod-modu köprüsünde per-call izin kancası ("ask"
modda script içi mutasyon çağrıları onay UI'ına düşsün) + per-call gözlemlenebilirliğin
geri kazanılması (debug.jsonl).

**Yapılan:**
- **`codemode.Bridge` genişletildi** (`bridge.go`): `Start` artık `Config` alıyor
  (`Call`/`Allow`/`Gate`/`Observe`). `Gate` dispatch'ten önce çalışır; red, script'e
  loud `MCPError` (isError=true) olarak döner ve özet "(N denied by permission gate)"
  sayacı içerir. `Observe` dispatch edilen VE reddedilen her çağrı için
  `CallObservation{Tool, DurMs, OutBytes, IsError, Denied}` üretir.
- **`RunCodeTool`** (`builtin_runcode.go`): `RunCodeGate` + `RunCodeObserver` hook'ları;
  Call ctx'i closure'a bağlanıp köprüye verilir (prompter/grants/session-id ctx'te).
  Araç açıklamasına "ask modda onay beklemesi timeout'a sayılır" uyarısı eklendi.
- **Agent bağlantısı** (`toolsetup.go`): gate = native loop'un aynı çağrıya uygulayacağı
  **`permGate`'in birebir kendisi** (aynı mod, ctx prompter/grants, audit logger —
  ayrı politika kodu yok, tam parite; standing "always allow" grant'ları script içi
  çağrılarda da geçerli). Observer = her çağrı için `debug.jsonl`'a `DebugTool` olayı
  (`Detail: "via run_code"` / `"permission denied (via run_code)"`).
- **Bilinçli karar:** shell'deki otonom `git push` guard'ının MCP karşılığı eklenmedi
  (annotation yok, semantik opak) — parite kuralı yeterli: otonom tur native yolda
  neyi çağırabiliyorsa köprüden de onu çağırır. Gerekçe `_Docs/44` §10'da.
- **Testler (+4):** gate reddi (dispatcher çalışmaz, observer Denied kaydeder, özet
  sayar) + gate izni & observer dispatch kaydı (bridge_test) · python ile uçtan uca
  red → `MCPError` ve observer/özet doğrulaması + dispatch gözlemi (runcode_test).
  270 test yeşil (codemode/tools/agent) + api 73 yeşil.

**Sıradaki:** Faz 0 baseline ölçümü → Faz 3 çok-server ölçümü → Faz 5 UI trace kartları.

## Code Execution with MCP — Faz 1 PoC (`run_code`) ✅ (2026-07-02)

**İstek:** `_Docs/44-CODE-EXECUTION-MCP.md` planının ilk uygulama fazı — MCP araçlarını
şema olarak değil, üretilmiş Python binding'leri olarak sunmak (occupancy düşürme).

**Yapılan:**
- **Yeni paket `internal/codemode`:** `bridge.go` (per-execution loopback HTTP köprüsü —
  127.0.0.1 rastgele port, rastgele Bearer token, agent tool-filter parity, 200 çağrı/koşu
  tavanı, 120s per-call timeout, çağrı sayacı/özeti) + `bindings.go` (`.tionswarm/mcp/`
  altına `_bridge.py` + server-başına Python modülü; docstring = açıklama + input schema;
  Python identifier sanitizasyonu; her çağrıda sıfırdan regen — bayat stub kalmaz).
- **Yeni araç `run_code`** (`internal/tools/builtin_runcode.go`): boş script → binding
  regen + modül/fonksiyon listesi (discovery); script → stripped env
  (`minimalScriptEnv`) + `PYTHONPATH` + köprü URL/token env'i ile python çalıştırır;
  yalnız stdout/stderr (16KB cap) + MCP çağrı özeti döner — **veri context'e girmez**.
  RiskExec (`classify.go`), varsayılan 60s / max 300s timeout.
- **Gate'ler:** `Tunables.codeMode` (default KAPALI, `TIONSWARM_CODE_MODE=1` ile boot'ta
  açılır — `codemode_tunable.go` + `app.go`) VE `ShellEnabled` VE MCP kataloğu dolu.
  Kayıt `toolsetup.go` AttachMCP bloğunda; CLI köprüsüne verilmez (`bridgeExcluded` +
  `cliLazyBridgeExcluded`).
- **Plandan sapmalar (gerekçeli):** (1) "yalnız read-only MCP araçları" yerine **tool-filter
  parity** — MCP `Tool` yapısında `readOnlyHint` annotation'ı yok (doğrulandı), heuristik
  isim filtresi kırılgan; bunun yerine köprü ajanın kendi filtresini uygular + `run_code`
  RiskExec olduğundan ask modunda bütünüyle onaya düşer, read-only modda bloklanır.
  (2) Köprü token'ı subprocess'e **env ile** geçer — host secret değil, koşu-başına
  rastgele, köprüyle birlikte ölür; diske yazmaktan (worktree'ye sızma riski) daha güvenli.
- **Testler (12 yeni):** `codemode/bridge_test.go` (auth/403/400/429, dispatcher hatası
  loud, özet), `codemode/bindings_test.go` (sanitizasyon, allow filtresi, regen temizliği),
  `tools/builtin_runcode_test.go` (discovery, gerçek python ile MCP çağrısı + veri
  sızmaması, loud failure, script-içi bypass'ın köprüde reddi). `go build ./...` +
  codemode/tools/agent/api testleri yeşil.

**Sıradaki:** Faz 0 baseline ölçümü (turn-debug ile şema token payı, klasik vs kod-modu
A/B) → Faz 2 (per-call izin + trace olayları). Detay: `_Docs/44` §5.

## run_subagent: app-settings delegasyon master toggle'ı kaldırıldı ✅ (2026-07-02)

**İstek:** Ayarlar ▸ Geçişli yetenekler'deki "Ajan→ajan delegasyon (run_subagent)"
toggle'ını kaldır — araç zaten Araçlar ekranından (ajan denylist) aktif/deaktif
edilebiliyor; ikinci bir master toggle gereksiz.

**Yapılan (self-management master toggle'ının 2026-07-01'de kaldırılmasıyla aynı desen):**
- **Backend gate kaldırıldı:** `run_subagent` artık **daima kurulu** (native
  `toolsetup.go`, CLI köprüsü `mcp_interaction.go`, `subagent.go RunSubagentRunner`
  koşulsuz). `Tunables.delegation`/`SetDelegationEnabled`/`DelegationEnabled` silindi;
  `settings.EnableDelegation` (3 struct + patch pointer + `applyBool` + `server.go`
  push + `app.go` env seed `TIONSWARM_ENABLE_DELEGATION`) tamamen çıkarıldı. **Kalan
  frenler:** `delegationMaxDepth` (1–10, vars. 3) + `delegationMaxCalls` (1–100, vars. 8)
  — her delegasyon çağrısında geçerli güvenlik/bütçe guard'ları.
- **Frontend:** `AppToolsPanel` toggle'ı bilgi kartıyla değiştirildi (araç Araçlar
  ekranından yönetilir), derinlik/çağrı limitleri koşulsuz gösteriliyor.
  `types/settings.ts` + `SettingsPanel.tsx` payload'ından `enableDelegation` alanı kaldırıldı.
- **Testler:** `TestRunSubagentGate` → `TestRunSubagentAlwaysInstalled` (registry'de
  daima var); `TestDelegation_DisabledToolAbsent` → `TestDelegation_ToolAlwaysAvailable`
  (unknown-tool hatası ALMAZ); `store_test`/`subagent_test`/`delegation_e2e_test`
  `EnableDelegation`/`SetDelegationEnabled(true)` referanslarından temizlendi.
- **Dokümanlar:** `tionswarm-settings`/`tionswarm-self-management`/`tionswarm-autonomous-ops`
  skill'leri + `_Docs/11/22/24/25` "gated" ifadelerinden "daima kurulu, görünürlük
  araç-bazlı"ya güncellendi.
- **Doğrulama:** `go build` ✅, tüm backend testleri ✅, frontend `tsc --noEmit` ✅.

## Native anthropic: konuşma geçmişi kayan cache breakpoint'i ✅ (2026-07-02)

**İstek:** Fable 5 cache analizinde tespit edilen fırsat — native anthropic yolunda
prompt-cache yalnız statik prefix'i (tools + system) kapsıyordu; uzun oturumlarda baskın
maliyet olan ham transkript her turda tam ücretleniyordu (OpenRouter yolunda geçmiş
breakpoint'i zaten vardı).

**Yapılan:**
- `contentBlock`'a `CacheControl` alanı eklendi; `toAnthropicMessages` artık
  `extendedCache bool` parametresi alıyor ve caching açıkken **son mesajın son bloğuna**
  kayan bir breakpoint (1h TTL) koyuyor. `Complete` + `Stream` çağrıları güncellendi.
- Breakpoint muhasebesi: `tools(1) + system-static(1) + history(1) = 3` (Anthropic limiti 4).
- Semantik: tur N cache yazımı → tur N+1 cache okuması (0.10×), breakpoint en yeni mesaja kayar.
  Trailing `tool_result` dahil her blok türünde çalışır.
- **Testler:** `TestToAnthropicMessages_RollingHistoryBreakpoint` (yalnız son blok işaretli),
  `_BreakpointOnLastBlockAcrossKinds` (tool_result kuyruğu), `_NoCacheWhenDisabled`.
  `_Docs/17`'ye bölüm + thinking-cache-invalidation uyarısı eklendi.
- **Doğrulama:** `go build` ✅, providers testleri ✅ (12/12 anthropic testi PASS).

## Claude Fable 5 tam desteği ✅ (2026-07-02)

**İstek:** TionSwarm'ya Fable 5 (claude-fable-5) desteği ekle.

**Mevcut durum (kısmi destek vardı):** thinking resolver (`RequiresAdaptiveThinking` —
Fable/Mythos `thinking:disabled`'ı 400 ile reddeder, off/low → min adaptif 1024) ve
anthropic pricing (3/15) zaten vardı; ingest `agent_adapter` fable→claude-fable-5
eşliyordu. Eksikler tamamlandı:

- **Kataloglar:** `kind_claudecli.go`'ya `fable` alias'ı eklendi (listede yoktu);
  `kind_anthropic.go`'daki yanlış tanım düzeltildi ("yaratıcı yazım odaklı" →
  "en yeni nesil; 1M bağlam, adaptif düşünme (daima açık), ajan görevleri", listenin
  başına alındı); `kind_openrouter.go`'ya `anthropic/claude-fable-5` eklendi.
- **Aile tabloları (`context_window.go`):** Fable ayrı katman oldu —
  `windowFable=1M` (eskiden genel-Claude 200K'ya düşüyordu), `MaxOutputFor` →
  `maxOutClaudeCapable` 32K (eskiden 16K), `AdaptiveBudgetFraction` → 0.45
  (opus/sonnet sınıfı; eskiden 0.40). Genel-Claude fallback'i 200K/16K/0.40 kaldı.
- **Pricing:** OpenRouter tablosuna `anthropic/claude-fable-5` (3/15, Anthropic
  cache tier 0.10×/1.25×) eklendi.
- **Metinler:** ContextPanel çıktı-tavanı ipucu, `tionswarm-settings` default skill,
  `_Docs/17` adaptif-fraction tablosu ve `tionswarm-project` skill'i yeni aile
  sınıflamasına güncellendi (opus/sonnet/fable 32K · haiku 16K).
- **Testler:** `context_window_test`/`maxoutput_test` yeni beklentilere güncellendi
  (+`fable` alias satırları). Doğrulama: `go build` ✅, providers/agent/conversation/
  billing testleri ✅, frontend `tsc --noEmit` ✅.

## Doküman bakımı: 40 numara çakışması + eksik index satırları ✅ (2026-07-02)

**Tespit (proje incelemesi):** `_Docs` içinde iki dosya 40 numarasını paylaşıyordu
(`40-PLAN-MODE.md` + `40-COKLU-SECIM.md`) ve `00-GENEL-BAKIS.md` index tablosunda
37–43 arası dokümanlar hiç listelenmiyordu (36'dan 44'e atlıyordu).

**Yapılan:**
- `40-COKLU-SECIM.md` → **`45-COKLU-SECIM.md`** (git mv; dosya başlığına numara notu,
  `05-ILERLEME` içindeki referans güncellendi). Plan modu 40'ta kaldı.
- `00-GENEL-BAKIS.md` index'ine 37/38/39/40/41/42/43/45 + `analiz-craftagent-arac-eslestirme.md`
  satırları eklendi; "Numara notu" 40→45 taşınmasını belgeliyor.
- `tionswarm-project` skill'i (the external agent project workspace) düzeltildi: modül yolu
  `github.com/bilal-arikan/tionswarm` (yanlış `bilal/tionswarm` idi), go.mod'a `jchv/go-webview2`
  eklendiği bilgisi, klasör yapısına `cmd/tionswarm-desktop` + `internal/app`, doc index'e
  39=Dizin-Site-Registry / 40=Plan-Modu / 44 / 45, kırık `39-PLAN-MODE.md` referansı →
  `40-PLAN-MODE.md`, bayat "Sırada: Faz 9 Wails" → native pencere zaten yapıldı (Wails'siz),
  Bash+PowerShell ayrımı + WebSearch + transform_data araçları eklendi.
- **Not (bir önceki oturum, commit `7ce98a4`, 2026-07-02 01:39):** persistent-pool
  gözlemlenebilirliği (`CLISessionPool.SetLogger` yaşam-döngüsü logları) + `resumeGateEnabled`
  saf fonksiyon + `TestResumeGateEnabled` + AppToolsPanel çifte-toggle uyarısı + `_Docs/17`
  düzeltmesi (canlı 3-tur ölçüm: resume ≈−43%, persistent ≈−58% cacheWrite) ve birikmiş WIP
  (rewind, code-execution MCP planı, bridge filter, sidebar chrome, error toast) commit'lendi.
- **Doğrulama:** `go build ./...` + `go vet ./...` + `go test ./...` (22 paket) + frontend
  `tsc --noEmit` tamamı yeşil.

## Otonomi duraklatma → yalnız workspace-özel + Zamanlamalar ekranına taşındı ✅ (2026-07-01)

**İstek:** Workspace ayarları ekranındaki "Otonomiyi duraklat" seçeneğini **Zamanlamalar**
ekranına taşı; gelişmiş uygulama ayarlarındaki "Tüm otonomiyi duraklat (uygulama geneli)"
anahtarını tamamen kaldır — pause artık yalnız workspace-özel olsun, Zamanlamalar
ekranından açıp kapatmak yeterli.

**Yapılan:**
- **App-geneli pause tamamen kaldırıldı (backend):** `settings.Settings/snapshot/patch`'ten
  `PauseAutonomy` alanı, `store.go` patch-apply'ı, `api/server.go`'daki
  `tun.SetAutonomyPaused(...)` çağrısı silindi. `agent.Tunables`'tan `pauseAutonomy`
  alanı + `SetAutonomyPaused`/`AutonomyPaused` metotları kaldırıldı. Otonomi freni artık
  yalnız workspace-düzeyi `Runtime.Paused()` (ws-settings `pauseAutonomy` → `SetPaused`):
  `budget.go guardedComplete` ve `reflector.go maybeAutoReflect` kontrolleri
  `r.tun.AutonomyPaused() || r.Paused()` → sadece `r.Paused()`.
- **Frontend:** Ayarlar ▸ Gelişmiş'ten "Otonomi" bölümü + `AutonomyPanel` bileşeni ve
  `settings.pauseAutonomy` tipi kaldırıldı. Workspace ▸ Genel'deki workspace pause toggle'ı
  kaldırılıp yerine Zamanlamalar'a yönlendiren not kondu. **Zamanlamalar ekranının üstüne**
  workspace-özel pause toggle'ı eklendi (`Schedules.tsx`: `getWorkspaceSettings` ile yüklenir,
  `updateWorkspaceSettings({pauseAutonomy})` ile optimistic toggle; `data-testid=workspace-pause-autonomy`).
- **Dokümanlar/skill:** `tionswarm-settings` + `tionswarm-autonomous-ops` skill'leri güncellendi
  (pause artık app-settings key'i değil, workspace-özel + Zamanlamalar ekranı); `06-WORKSPACES.md`
  tablosu not düştü. `update_settings` araç örnekleri `pauseAutonomy` yerine `autoTitleEnabled` kullanıyor.
- **Durum:** `go build ./...` ✅, ilgili paket testleri ✅ (agent/tools/settings 256 test), frontend `tsc` temiz.

## `/rewind` — sohbet checkpoint geri sarma (yalnız-sohbet MVP) ✅ (2026-07-01)

**İstek:** Sohbet ekranına Claude Code'daki `/rewind` benzeri bir komut ekle — bir tur
kodu bozunca ajanla tartışıp bağlamı kirletmek yerine, hatadan önceki temiz checkpoint'e
dönüp çarkı yeniden çevirmek için. Kapsam kullanıcı kararıyla **yalnız-sohbet** (dosya
geri-yükleme yok; kod için git zaten var).

**Yapılan:**
- **Backend truncate:** `db.DeleteMessagesFrom(sessionID, msgID)` — verilen mesaj + sonrasını
  siler, JSONL'i yeniden yazar; silinen sayıyı döndürür. Truncation noktası özet sınırından
  önceyse (`SummaryMsgCount > idx`) **artık geçersiz rolling-summary sessizce sıfırlanır**
  (dangling özet bırakılmaz). Test: `db/rewind_test.go` (removed sayısı + özet reset + reopen kalıcılığı).
- **API:** `POST /api/sessions/{id}/rewind` body `{messageId}` → `handleRewindSession`
  (`bindJSON` + `DeleteMessagesFrom`, Logs'a `session rewound` satırı). server.go route kaydı.
- **Frontend:** `/rewind` slash komutu (`useChatStream.chatCommands`, ⟲) → `RewindDialog`
  picker açar. Dialog oturumun kullanıcı promptlarını (checkpoint) yeniden-eskiye listeler
  (her satır: "Prompt #N" + "M mesaj silinir" + önizleme); seçilen checkpoint'e geri sarar.
  `rewindTo(msgId)` görünüm + sunucu tarafını atomik siler ve **silinen promptu composer
  draft'ına geri koyar** (düzenleyip yeniden göndermek için) — `writeSessionDraft` +
  Composer `key` bump ile remount. Yerel-only (persist edilmemiş) anchor'da sunucu çağrısı atlanır.
- **Balon hover aksiyonu (2026-07-02):** her kullanıcı balonunun altında ⟲ "Buraya geri sar"
  butonu (`RewindButton.tsx`, iki-adımlı onay) → picker açmadan doğrudan o mesaja geri sarar
  (`UserTurn`→`MessageList` `onRewind`→`App.handleRewind`, dialog ile ortak yol).
- **Sınır (Claude Code ile aynı):** yalnız transcript geri alınır; `Bash` yan etkileri
  (`git push`/`npm install`/`rm`) ve dosya değişiklikleri geri **gelmez**.
- **Durum:** db+api derlenir + testler geçer (db 35, api 66), frontend `tsc` temiz.
  Dosya-restore modu (kod geri-yükleme) ileride eklenebilir — snapshot altyapısı gerektirir.

## Compact (compaction) promptu editlenebilir + tüm workspace'lerde default ✅ (2026-07-01)

**İstek:** Compaction (bağlam sıkıştırma) promptunu da editlenebilir yap ve bütün
workspace'lerin default (seed) promptu yap — summary/reflect/title gibi.

**Yapılan:**
- **Editlenebilir 4. runtime prompt:** `agent.PromptKeys`'e `"compact"` eklendi →
  config API (`GET/PUT /api/workspace-config`) ve WorkspaceFilesPanel bu key üzerinden
  döndüğü için **otomatik editlenebilir** oldu (`config/prompts/compact.md`). Default
  `promptDefaults["compact"] = conversation.CompactPromptDefault()` (ham template, iki
  `%s` slotlu).
- **Per-workspace enjeksiyon (paylaşılan global Manager'a rağmen):** `conversation`
  paketine `WithCompactPrompt(ctx, tmpl)` + `compactPromptFromCtx(ctx)` eklendi;
  `summarizeRendered` template'i ctx'ten alır. **Güvenlik:** ctx template'i yalnız
  **tam iki `%s` ve başka `%` verb'ü yoksa** kullanılır — bozuk edit'te `fmt.Sprintf`'in
  `%!`-işaretli çıktısı yerine sessizce gömülü default'a düşer. Enjeksiyon 5 çağrı
  yerinde: chat / chat_stream / wake_turn (Prepare) + summary (ForceCompact) +
  toolloop (reaktif mid-loop, `r.CompactPromptTemplate()`).
- **Tüm workspace'lerde default:** yeni `"compact"` PromptKey olduğu için boot'ta
  `syncConfigFiles → SeedWorkspaceConfig → writeIfAbsent` her workspace'e
  `compact.md`'yi **otomatik** yazar (restart sonrası). README template'i iki-`%s`
  uyarısıyla güncellendi.
- **Temizlik:** eski read-only `wsConfigDTO.CompactionPrompt` alanı kaldırıldı (compact
  artık editlenebilir promptKeys'te; duplikasyon önlendi). `CompactionPromptText()`
  export'u referans için korundu. Frontend'de karşılığı salt-okunur textarea bloğu +
  `WorkspaceConfig.compactionPrompt` tipi de kaldırıldı; `PROMPT_LABELS`'a `compact` eklendi.
- **UI overflow fix (workspace prompt ekranı):** uzun promptlar (compact + 40KB instructions)
  ekran dışına taşıyordu — kök neden `WorkspaceView` sağ-içerik `flex-1` sütununda **`min-w-0`
  yokluğu** (flex item min-content genişliğinin altına küçülemiyordu). `min-w-0` eklendi
  (WorkspaceView sütunu + içerik container), `PromptEditor` root `w-full min-w-0` + split
  yarımları `min-w-0` + preview `break-words`. `<pre>` zaten `overflow-x-auto` taşıyordu.
  Frontend tam build ✅ (dist yazıldı).
- **Durum:** çekirdek paketler (`conversation`+`agent`) derlenir + `go vet` temiz + test
  yazılacak. **api paketi build'i, ilgisiz market_publish/export granular-selection
  WIP'i (`publishInclude` bool→ID-list refaktörü) nedeniyle bloklu** — o WIP tamamlanınca
  api tarafı + binary derlenir.

## Skill 4-tier görünürlük + default workspace prompt ✅ (2026-07-01)

**İstek:** (1) Skilleri de araçlardaki gibi 4 görünürlük kategorisinden birine
ayarlanabilir yap. (2) Yeni workspace'lerin default prompt'unu the external agent project'ın tam
sistem promptu gibi yap (TionSwarm'da monolitik sistem promptu yok — workspace prompt
onun yerini tutar). (3) `ClaudeResume`'u default açık yap.

**Yapılan:**
- **Skill 4-tier görünürlük (araç muadili tek seçici):** skiller artık `full` /
  `summary` / `name-only` / `hidden` tier'larından **tam birini** taşır. Önceden 3
  durum vardı (full / name-only / `auto_summary:false`≈hidden); eksik **summary**
  (slug + açıklama, when bastırılır) eklendi. Türetilmiş `Skill.Visibility`
  (`skillVisibility()`) + `Store.SetVisibility(slug,tier)` üç frontmatter flag'ini
  tek yazımda kurar. Yeni `SummaryOnly` alanı + `isSummaryOnly` +
  `setFrontmatterSummaryOnly`; `renderCatalog` summary'de when'i atlar. API
  `PUT /api/skills/{slug}/visibility` (geçersiz tier 400). UI: `SkillVisibilitySelector`
  eski Özet/NameOnly toggle çiftini değiştirir, araçların `VISIBILITY_TIERS`'ini
  paylaşır. Detay: `_Docs\19` §Skill 4-tier.
- **Default workspace prompt:** `internal/workspace/defaults/default-instructions.md`
  (the external agent project tam sistem promptu, ~40KB) `//go:embed` ile `defaultWSSettings().Instructions`
  seed'ine bağlandı → talimatı olmayan (yeni) workspace'ler bu baseline'la açılır.
  Persisted `instructions` bunu override eder (mevcut workspace'ler etkilenmez).
- **ClaudeResume:** zaten default açıktı (kod `settings.go` DefaultSettings + canlı
  `claudeResume=true`); değişiklik gerekmedi, doğrulandı.
- **Yan düzeltme:** `RevealButton.onReveal` tipi `() => void | Promise<unknown>`'a
  genişletildi (reveal endpoint'leri `{path}` döndürüyor; çağrı yerleri tek noktadan
  tip-uyumlu oldu).

**Cross-runtime cache benchmark (claude-cli resume vs the external agent project SDK):** aynı 3-mesajlık
konuşma iki runtime'da ölçüldü. TionSwarm (claude-cli, resume açık) ilk turları **soğuk**
yazıp tur-başı ~70-90K cache **yeniden yazıyor** (warm-read tutarsız, yalnız bazı
turlarda); the external agent project SDK 1. turdan **istikrarlı sıcak** cache okuyor (cR≫cW) → aynı
konuşmada ~3× ucuz. Ağır (~40KB) workspace prompt eklemek TionSwarm'da cache-write'ı
+42K büyüttü (input değişmez — prompt cache'e gider). Sonuç: darboğaz claude-cli'nin
sıcak prefix'i turlar arası **tutarlı** koruyamaması.

## Dışa aktarım: promptlar/README + flows/skills/schedules tek tek seçilebilir ✅ (2026-07-01)

**İstek:** Export'a "Promptlar & Dosyalar" ekranındaki diğer promptları da dahil et (varsayılan
değilse); Flows, Workspace skill'leri ve Schedules'ı ajanlar gibi **tek tek** seçilebilir yap.

**Yapılan:**
- **Payload:** `market.WorkspacePayload`'a `Prompts map[string]string` (yalnız varsayılandan
  farklı runtime prompt override'ları) + `Readme string` eklendi (`internal/market/pack.go`).
- **Export builder** (`buildWorkspaceTemplatePayload`): her `agent.PromptKeys` anahtarını okur,
  `PromptDefault` ile karşılaştırır, **yalnız farklı olanları** taşır; README boşsa atlanır.
  `Include` boolean yerine **id/slug set**'leriyle çalışır — `wantSet(all, ids)` (nil=tümü,
  boş=hiçbiri) + `sliceOrNil` ile agents/flows/skills/schedules bağımsız filtrelenir.
- **Include şeması:** `publishInclude` = `AgentIDs/FlowIDs/SkillSlugs/ScheduleIDs []string`
  (tri-state) + `Instructions/Prompts/BoardColumns bool`. Frontend `WorkspaceExportInclude` aynen.
- **Seed/install:** `seedTemplateConfigFiles` (`seedWorkspaceTeam` 5. adım) install'da non-default
  promptları `config/prompts/`, README'yi `config/README.md`'ye yazar. Dokunulmamış promptlar
  hedefteki güncel varsayılanı korur (`internal/api/templates.go`).
- **UI:** yeni yeniden kullanılabilir `ExportPickList.tsx` (checkbox liste) ile Ajanlar/Akışlar/
  Skill'ler/Zamanlamalar dört ayrı seçim listesi; Talimatlar/Promptlar&README/Pano toggle kaldı.
  Promptlar toggle'ı `getWorkspaceConfig`'ten non-default prompt + README sayısını gösterir.
  Önizleme + bağımlılık uyarısı seçili öğelere göre güncellendi.
- `go build ./...` ✅ · `npx tsc --noEmit` ✅. (Not: bu refactor, paralel "Compact prompt" WIP'inin
  beklediği api-build blokerini de çözer.)

## Dışa aktarıma canlı önizleme ✅ (2026-07-01)

**İstek:** Dışa aktarım paneline **canlı önizleme** ekle.

**Yapılan:**
- `WorkspaceExportPanel`'e, kategori toggle'larının altında **Önizleme** kartı eklendi:
  mevcut seçimin tam çıktısını pill'lerle gösterir (N ajan / akış / zamanlama / skill /
  talimat / pano sütunu). Kapalı veya 0 olan kalemler soluk + üstü çizili.
- Sayımlar backend kurallarını yansıtır: **zamanlama yalnızca seçili ajana bağlıysa** sayılır
  (orphan düşer); akış/skill/talimat/pano ilgili toggle'a uyar.
- Kart üstünde çözümlenen **ad · sürüm · pack id** ve aynı slug'a yeniden yayında
  **üzerine yazma** notu. Frontend `slugify`, Go `slugify` ile birebir eşleşir →
  önizleme sunucuyla aynı `workspace-<slug>` id'sini verir.
- `npx tsc --noEmit` ✅. Dosya: `frontend/src/components/workspace/WorkspaceExportPanel.tsx`.

## Dışa aktarıma metadata alanları + bağımlılık uyarısı ✅ (2026-07-01)

**İstek:** Dışa aktarıma **isim/açıklama/sürüm** alanı desteği ve **ajan bağımlılık uyarısı** ekle.

**Yapılan:**
- **Metadata alanları:** `WorkspaceExportPanel`'in üstüne **Şablon adı / Açıklama / Sürüm**
  girişleri kondu. Ad `ws.name`'den seed edilir; açıklama/sürüm boşsa sunucu varsayılan üretir.
  Frontend `WorkspaceExportMeta` (`name?/description?/version?`) `api.publishPack(...)`'ın yeni
  4. argümanı olarak geçer; boş alanlar düşürülür.
- **Backend:** `publishRequest`'e `Name/Description/Version` (omitempty) eklendi;
  `handlePublishMarket` workspace kind'ında bunları trim edip `BuildWorkspacePack(slug, name,
  desc, version, …)`'e verir. `BuildWorkspacePack` imzasına `version` parametresi eklendi
  (boş → `1.0.0`). Slug artık kullanıcı adından türetilir → farklı ad = farklı pack id.
- **Bağımlılık uyarısı:** panel, hariç bırakılan ajanlara bağlı akış/zamanlamaları uyarı
  kutusunda listeler. Flow bağımlılığı `flow.graph` JSON'u client'ta parse edilip
  `type==='agent'` düğümlerinin `agentId`'leri toplanarak hesaplanır. Etki net: akış → kopuk
  referans, zamanlama → dışa aktarımdan düşer.
- `go build ./...` ✅ · `npx tsc --noEmit` ✅. Dosyalar: `internal/market/publish.go`,
  `internal/api/market.go`, `internal/api/market_publish.go`, `frontend/src/api/market.ts`,
  `frontend/src/components/workspace/WorkspaceExportPanel.tsx`.

## Workspace dışa aktarımı ayrı sekmeye taşındı + seçilebilir içerik ✅ (2026-07-01)

**İstek:** Workspace ayarlarındaki "export aldığımız kısım" (Şablon olarak yayınla) ayrı bir
alt-panele taşınsın (feature detaylandırılacak); export alırken **neyin dahil edileceği** seçilebilsin.

**Yapılan:**
- **Yeni alt-sekme:** `WorkspaceView.tsx`'e `export` sekmesi ("Dışa Aktar", `PackageCheck` ikonu)
  eklendi (TAB_KEYS + TABS + render dalı). Kendi publish aksiyonu olduğu için header "Kaydet"
  butonu bu sekmede gizli (appearance gibi). Genel (`WorkspacePanel`) tab'ındaki eski
  "Şablon olarak yayınla" butonu **kaldırıldı**, yerine yeni sekmeye yönlendiren not kondu.
- **Yeni panel** `frontend/src/components/workspace/WorkspaceExportPanel.tsx`: ajanları
  (listAgents), akışları, workspace-tier skill'leri, zamanlamaları çeker; **ajan seçim listesi**
  (checkbox, varsayılan tümü seçili, ≥1 zorunlu) + kategori toggle'ları (Akışlar / Zamanlamalar /
  Skill'ler / Talimatlar / Pano sütunları, sayı rozetli, varsayılan açık). "Şablon olarak dışa aktar"
  → `api.publishPack('workspace', ws.id, include)`.
- **Backend:** `publishRequest`'e opsiyonel `Include *publishInclude` alanı (`AgentIDs []string`
  (nil=tümü) + `Flows/Schedules/Skills/Instructions/BoardColumns bool`). `buildWorkspaceTemplatePayload`
  artık `inc *publishInclude` alıyor — **nil = her şey** (geriye uyumlu), aksi halde ajanları filtreler
  ve kategori flag'lerini birebir onurlandırır (`market_publish.go`). En az bir ajan hâlâ zorunlu.
- **API tipi:** `market.ts`'e `WorkspaceExportInclude` tipi + `publishPack(kind, sourceId, include?)`.

**Doğrulama:** `go build ./...` ✅ · `npx tsc --noEmit` ✅.

## Harici araçlar listesine `codebase-memory-mcp` eklendi ✅ (2026-07-01)

**Yapılan:** Ayarlar ▸ Harici Araçlar ekranının kaynağı olan `knownExternalTools` slice'ına
(`internal/api/external_tools.go`) yeni entry: **codebase-memory-mcp** (DeusData) — kod tabanını
kalıcı bilgi grafiğine indeksleyen stdio MCP sunucusu (158 dil, sub-ms sorgu, ~%99 daha az token).
`category=dev`, `wire=mcp` (Market'te "Codebase Memory MCP" paketiyle kurulur). Tespit PATH'te
`exec.LookPath` ile; program `C:\Users\user\Desktop\Progs\codebase-memory-mcp\` altında ve PATH'te
olduğundan ekran **Found** gösteriyor. `go build ./internal/api` ✅. Not: yeni entry PATH'e o dizini
içeren bir süreçten görünür — backend yeni PATH ile yeniden başlatıldı.

## Seçili dil sohbet bağlamına enjekte ediliyor (profil zaten ediliyordu) ✅ (2026-07-01)

**Soru:** Profil bilgilerim ve seçtiğim dil bağlama ekleniyor mu?

**Bulgu:** Profil (Ad/Konum/Saat dilimi/Notlar) zaten **sohbet** turlarında "## About the
user" bloğu olarak enjekte ediliyordu (`api.userContextBlock` → `composeTurnRequest`).
Ama **dil (tr/en) hiçbir yere enjekte edilmiyordu** — blok yalnız profil alanlarını
içeriyordu.

**Yapılan:** `userContextBlock`'a **dil yönergesi** eklendi (`languageName` yardımcısı) —
"Preferred language: reply in Turkish (Türkçe) by default…". Profil alanı boş olsa bile
dil satırı çıkar (dil varsayılanı `tr`), yani her sohbet turu artık dili onurlandırıyor.

**Bilinen sınır:** Enjeksiyon **yalnız sohbet yolunda** — otonom/zamanlanmış/flow turları
(`agent.autonomousSystemPrompt`, farklı paket) bu bloğu hâlâ almıyor. Utility promptları
(reflect/title/summary) zaten "reply in the same language as the data" diyor. Otonom yola
taşımak istenirse Tunables köprüsü gerekir (backlog). `go build`/`api test` ✅.

## "Özet promptu" netleştirildi + asıl compaction promptu salt-okunur gösteriliyor ✅ (2026-07-01)

**Soru:** Promptlar & Dosyalar ekranındaki "Özet promptu" kullanılıyor mu? Özetleme
kapsamlı olmalı; gereksizse sil, veya asıl promptu göster.

**Bulgu:** "summary" runtime promptu **kullanılıyor ama konuşma özetlemesi değil** —
yalnız `/memory · /board · /flows` slash-komutlarının anlık genel-bakış sistem promptu
(`agent/summarizer.go`, composer'da hâlâ bağlı: `useChatStream.ts`). Asıl konuşma
özetlemesi ayrı ve **zaten kapsamlı**: `conversation/manager.go compactPrompt` (8 bölüm +
anti-decay), düzenlenemez.

**Yapılan:**
- Etiket "Özet promptu" → **"Genel bakış promptu"**, hint bunun slash-komut özeti olduğunu
  ve konuşma özetlemesi olmadığını açıkça belirtiyor (`WorkspaceFilesPanel.tsx`).
- **Asıl compaction promptu salt-okunur gösteriliyor:** `conversation.CompactionPromptText()`
  (yeni exported erişimci; `%s` slotları etiketle doldurulmuş) → `wsConfigDTO.compactionPrompt`
  (`api/workspace_config.go`) → panelde read-only textarea (`WorkspaceConfig.compactionPrompt`).
- Silinmedi (slash komutları hâlâ kullanıyor). `go build`/`tsc` temiz.

**İstek:** Ayarlardan self-management aç/kapa silinsin (araçlar diğerleri gibi olsun);
her araca skill'lerdeki gibi görünürlük seçilebilsin — **4 tier, biri seçili**:
`Tam` (context'in tamamı) / `Özet` / `İsim` / `Gizli`. Ayrıca araç bilgisinde olup
UI'da görünmeyenler (örnekler, when-to-use) gösterilsin.

**Yapılan:**
- **Backend:** `tools.Visibility{Full,Summary,NameOnly,Hidden}` + `SetVisibility`/
  `VisibilityOf` (registry). `WorkspaceToolConfig.ToolVisibility map[string]string`
  (eski `HiddenTools`/`ShownTools` listeleri yüklemede map'e migrate). `toolsetup`
  override'ları en son `SetVisibility` ile uygular. `workspace-tools` API `visibility`
  + `examples` döndürür, PUT `toolVisibility` map'i alır (`validVisibility` doğrular).
- **Self-manage:** master toggle kaldırıldı → paket daima kurulu, varsayılan `hidden`.
  **Tam sökme (aynı gün):** `settings.EnableSelfManage` alanı + `TIONSWARM_ENABLE_SELFMANAGE`
  env + `Tunables.SelfManageEnabled`/`selfManage` + ayar UI toggle'ı **tamamen silindi**;
  `spawn_session` CLI köprüsünde koşulsuz ilan edilir. `TestInteractionAdvertisedNames`
  + `store_test` güncellendi.
- **UI:** `ToolsPanel` per-tool 4'lü segment (Tam/Özet/İsim/Gizli) + tier-renkli
  `VisibilityBadge`; toplu + per-server hızlı eylem 4 tier'a genişledi. Detay görünümü
  **örnek çağrıları** + tam (çok-paragraflı) açıklamayı gösterir.
- **CLI uyumu:** Native tüm tier'ları tam uygular; CLI'da tier katalog-bloğu metnini
  etkiler, gerçek yükleme CLI'nin ToolSearch/`alwaysLoad`'ıyla — `full` CLI'da "en
  fazla ilan", dış MCP'de "kesin eager" garantisi vermez (mimari sınır, dokümante).
- `go build ./...` + tüm testler yeşil, `tsc --noEmit` temiz. Detay: `_Docs/19`.

## Görünüm sadeleştirme: accent + temel mod kaldırıldı, renk=açık/koyu varyant ✅ (2026-07-01)

**İstek:** Görünüm ekranında "Vurgu rengi (accent)" ve "Temel mod (koyu/açık/sistem)"
kaldırılsın; onun yerine her tema **renginin** açık ve koyu karşılığı olsun.

**Yeni model:** Tek kontrol = `themePreset`. Bir preset id'i hem rengi hem modu kodlar
(`violet-dark` / `violet-light`). 6 renk ailesi (Mor/Mavi/Zümrüt/Gül/Kehribar/Nord) ×
2 varyant = 12 preset. Ayrı base-mode ve accent picker yok.

**Frontend:**
- `lib/themePresets.ts` yeniden yazıldı: `COLORS` (aile tanımı, dark+light accent/soft) →
  `THEME_PRESETS` (düz, aile başına 2) + `THEME_COLORS` (picker için aile+varyant) +
  `DEFAULT_PRESET='violet-dark'`. Neutrals (bg/surface/text) mod başına sabit, yalnız
  accent değişir.
- `lib/theme.ts` sadeleşti: `Appearance={themePreset}`, `applyTheme(preset)` (accent
  override + legacy dark/light/system yolu kaldırıldı; boş/bilinmeyen id → default preset).
- `AppearancePanel` (`settings/appPanels.tsx`): "Temel mod" + "Vurgu rengi" alanları
  kaldırıldı; tema paleti grid'i yerine **renk satırları** (her aile: Koyu + Açık swatch).
  Load/save yalnız `themePreset`.
- `App.tsx`: appearance ref/`applyClientPrefs`/ws-override yalnız `themePreset`.

**Backend:**
- `settings.Default().ThemePreset` `"midnight-violet"` → `"violet-dark"`; alan yorumu
  güncellendi. `Theme`/`Accent` alanları geriye-uyum için kaldı (artık UI'yı etkilemiyor).
- `cmd/tionswarm-desktop/titlebar_windows.go`: preset→renk haritası kaldırıldı; titlebar
  artık id son-ekine (`-light`/`-dark`) göre mod-neutrals seçiyor (accent kullanılmıyor).

**Doğrulama:** `go build ./...` + desktop build ✅, frontend `tsc --noEmit` temiz ✅.
`tionswarm-settings` skill'i güncellendi.

## Journal gürültü filtresi + recall eşiği ayarlanabilir ✅ (2026-07-01)

**İstek:** Context-payload optimizasyonu (4 paralel görevden 4.). Her turun **dinamik**
(cache-dışı) bağlamına enjekte edilen "Relevant memory" bölümü, önemsiz/düşük-bilgili journal
kayıtlarıyla kirleniyordu (ör. `Q: 2+2 kaç eder? A: 4 eder.`). Bu trivial turlar her tur taze token
harcatıyor ve dikkati dağıtıyordu.

**Çözüm — iki ayarlanabilir mekanizma:**

1. **Yazma-tarafı düşük-bilgi kapısı (`journalMinLen`)** — `Journal()` artık içeriği
   `journalMinLen` rune'dan kısa olan turları **hiç saklamadan** atar (boş-check'in yanında, uzunluk
   cap'inden önce). Böylece gürültü daha kaynakta, recall havuzuna girmeden kesilir.
   - Yeni tunable + settings alanı `journalMinLen`. **Default 40 rune** (muhafazakâr: kısa ama
     anlamlı notlar korunur). **0 = kapalı** (filtre devre dışı; geriye dönük tam uyum).
   - `0` anlamlı bir değer olduğu için cap deseninden farklı: getter `0`'ı default'a çevirmez,
     yalnızca negatifi 0'a normalize eder. Clamp: `0 ≤ minLen ≤ journalMaxLen`.
   - Test runtime'ları (`NewTunables`, `tun==nil`) kapıyı **kapalı** tutar — mevcut testler
     etkilenmez.

2. **Recall eşiği artık ayarlanabilir (`recallMinScore`)** — eskiden `memory.go` içinde
   sert-kodlu `const minScore = 0.04` idi. Artık `memory.Store` eşiği bir **canlı provider**
   üzerinden okur (`SetMinScoreProvider`); runtime bunu workspace'in `Tunables.RecallMinScore`'una
   bağlar, böylece ayar değişikliği **restart'sız** bir sonraki recall'da geçerli olur (import döngüsü
   yok — `memory` paketi `agent`'ı import etmez).
   - `recallMinScore` ayarı zaten settings/frontend'de **vardı ama ölü konfigdi** (hiçbir yere bağlı
     değildi, default 0.05). Artık gerçekten bağlandı; default `0.05 → **0.04**` düzeltildi (gerçekte
     yürürlükteki sabit değer 0.04'tü — davranış korunur). Kullanıcı gürültüyü kesmek için
     yükseltebilir.

**Etki:** Default 40-rune kapısı `Q: kısa? A: tek kelime` türü ultra-trivial turları kaynakta eler;
daha agresif filtreleme için `journalMinLen` yükseltilir (ör. 60–80) ve/veya `recallMinScore`
artırılır (ör. 0.08–0.12) — ikisi de dinamik segmentin token + dikkat maliyetini düşürür. Her iki
default da mevcut davranışı bozmaz (recall 0.04 sabit; kapı yalnızca en kısa turları eler).

**Değişen dosyalar:** `internal/agent/tunables.go` (+`journalMinLen`, +`recallMinScore`,
`SetJournalLimits` 3 parametre), `internal/agent/reflector.go` (`Journal()` kapı + `journalMinLen()`
helper), `internal/agent/runtime.go` (provider bağlama), `internal/memory/memory.go`
(`DefaultMinScore` + provider + `minScore()`), `internal/memory/graph.go` (floor provider),
`internal/settings/{settings,store}.go` (+alan, applyInt, clamp, default 0.04), `internal/api/server.go`
(applySettings wiring), `internal/agent/tunables_test.go` (yeni testler), frontend
(`types/settings.ts`, `SettingsPanel.tsx`, `appPanels.tsx`).

**Doğrulama:** `go build ./...` temiz; `go test ./internal/{memory,agent,settings,api,e2e}/...` →
201 test geçti; `tsc --noEmit` temiz.

## Statik prefix sadeleştirme — Deliverables skill'e taşındı + deferred-not tek yerde ✅ (2026-07-01)

**İstek:** Context-payload optimizasyonu (4 paralel görevden 3.). Sistem prompt'un statik
prefix'inde iki şişkinlik: (1) "deferred / ToolSearch ile yükle / unloaded ad → No such tool
available" açıklaması hem Skills hem Tools bloğunda tekrar ediyordu; (2) "Deliverables →
Artifacts" bloğu (inline media + gallery JSON + `![alt]` kuralları) her tur statik prefix'te —
token'dan çok dikkat/context-rot maliyeti.

**Yapılan:**
- **Adım 1 — Boilerplate birleştirme:** `internal/skills/store.go` `renderCatalog` içindeki
  skills `deferNote` tek kısa cümleye indirildi (`ToolSearch select:...` + "Available Tools
  notuna bak"). Mekanizmanın tam açıklaması (DEFERRED'ın anlamı, `"No such tool available"`
  cümlesi) artık **yalnız** `internal/agent/toolsetup.go` `renderLazyToolCatalog` CLI intro'sunda
  (tek canonical yer). Native vs CLI varyant farkı korundu (native eager → not yok).
- **Adım 2 — Deliverables skill'e taşındı:** Yeni shipped skill
  `internal/skills/defaults/tionswarm-deliverables/SKILL.md` (`access: shared`, on-demand). Tüm
  detaylı kurallar (binary `sourcePath`, inline media, gallery, `update_artifact` by id) skill
  body'sine taşındı. `internal/api/artifacts.go` `artifactDeliverableGuidance` 2 satırlık özet +
  `use_skill tionswarm-deliverables` pointer'ına indirildi. Bilgi **kaybolmadı** — sadece prefix'ten
  skill'e taşındı; `//go:embed defaults` deseni otomatik gömüyor (ek kayıt gerekmedi).

`go build ./...` temiz; `go test ./internal/skills ./internal/agent ./internal/api` (197) temiz.
Yeni skill'in seed + parse + shared-yüklenebilir olduğu geçici testle doğrulandı (sonra silindi).
Tahmini statik-prefix kazancı: deliverables ~250-300 token + her CLI tur deferred-tekrar ~40-60
token ≈ **~300-360 token/tur**; asıl kazanç deliverables bloğunun her turdan kalkmasıyla
**dikkat/context-rot azalması**.

## Eager araç şemalarını sadeleştirme (run_subagent / create_artifact / core_memory) ✅ (2026-07-01)

**İstek:** Context-payload optimizasyonu. Araç JSON şemaları en ağır segment (~%56).
Bilinçli **eager** (her tur tam şemayla giden, "behavioral nudge") üç araç şişkin:
`run_subagent` (şemada 3 örnek + uzun field açıklamaları), `create_artifact` (uzun açıklama),
`core_memory_append`/`replace` (neredeyse aynı uzun açıklamayı tekrarlıyor).

**Yapılan (Strateji A — eager kalır, sıfır davranış riski):**
- `internal/tools/subagent.go`: `run_subagent` açıklaması ~yarıya, field açıklamaları kısaltıldı;
  `Examples` **3 → 1** (örnekler `foldExamples` ile her tur eager şemaya katlanıyordu → en büyük kalem).
- `internal/tools/builtin_artifact.go`: `create_artifact` açıklaması kısaltıldı (image/binary + base64-etme uyarısı korundu).
- `internal/tools/builtin_memory_core.go`: ortak core-memory tanımı tek `coreMemoryDesc` const'una çıkarıldı; her açıklama ortak cümle + role özgü satıra indi. **Lazy yapılmadı** (bağlam-basıncı anında lazım).

`go build ./internal/tools/...` + `go test ./internal/tools/...` (152) temiz; şema/örnek JSON geçerliliği doğrulandı.
Tahmini tasarruf: ~1.3 KB / eager tur ≈ **~300-350 token**. Strateji B (lazy + prompt nudge) öneri olarak bırakıldı.
Detay: `_Docs/19-LAZY-TOOL-LOADING.md`.

## Dış MCP araçları katalogda yalnız-ad (NameOnly) ✅ (2026-07-01)

**İstek:** Context-payload optimizasyonu. "Available Tools (load on demand)" bloğunda
dış MCP araçları (ör. `mcp__mcp-chrome__*`) tam açıklamalarıyla dökülüyordu; TionSwarm'nun
kendi `tionswarm_extended` araçları ise zaten yalnız-ad. Tutarsızlık + her tur ölü token.

**Yapılan:** `internal/tools/registry.go` `AttachMCP` artık her MCP aracını `lazy` **VE**
`nameOnly` işaretliyor (tek satırlık ekleme: `r.nameOnly[e.NamespacedName] = true`).
Mevcut `VisibleLazyCatalog`/`writeLazyToolLine` mekanizması açıklamayı boşaltıp yalnız
`- \`mcp__server__tool\`` basıyor. İsimler listede kaldığı için `tool_search`/`activate_tools`
ve CLI `ToolSearch select:<name>` ile araçlar hâlâ keşfedilip yüklenir. `go build ./internal/tools/...`
+ `go test ./internal/tools/...` temiz. Tahmini tasarruf: chrome ~30 araç için ~800–1200 token/tur.
Detay: `_Docs/19-LAZY-TOOL-LOADING.md`.

**Manuel override (UI):** MCP araçları artık varsayılan NameOnly olduğundan, kullanıcının
bunu sunucu bazında geri alabilmesi için `ToolsPanel` MCP sunucu kartına her satırda
**"Tümü NameOnly" / "Tümü Göster"** hızlı eylemi (+ eager/total sayacı) eklendi. Backend
değişmedi — mevcut `setWorkspaceToolsVisibility` override'ı kullanılıyor. `tsc --noEmit` temiz.

## Built-in araçlar fonksiyonel kategorilere gruplandı ✅ (2026-07-01)

**İstek:** Tools ekranında ~85 built-in araç tek "Yerleşik" grubunda akıyordu (MCP'ler
sunucu başına gruplanırken). Tutarsız ve taranması zor.

**Yapılan (tek-kaynak, backend → frontend):**
- **Backend:** `internal/tools/categories.go` — `CategoryOf(name) string`, isim→kategori
  açık eşlemesi (10 fonksiyonel anahtar: `files`, `search`, `memory`, `agents`,
  `automation`, `interaction`, `artifacts`, `skills-mcp`, `config`, `diagnostics`;
  eşlenmemiş araç `other`). `internal/api/workspace_tools.go` her built-in'e `category`
  alanını basar; MCP araçlarında boş (onlar sunucuya göre gruplanır).
- **Frontend:** `WorkspaceTool.category` tipi; `toolMeta.ts`'te `toolCategory` +
  `CATEGORY_LABELS` (TR etiket) + `CATEGORY_ORDER` (sabit sıra). `ToolsPanel` `groups`
  memo'su built-in'leri kategoriye göre (sabit sırada), MCP'leri sunucuya göre
  (alfabetik) böler. Bilinmeyen kategori anahtarı ham haliyle sona düşer (graceful).
- `go build ./internal/tools/... ./internal/api/...` + `tsc --noEmit` temiz. Yeni araç
  eklenince `categories.go`'ya bir satır eklenmeli (yoksa "Diğer" altında görünür).

## Tam temizlik: bütçe/limit sistemi + ölü ayarlar backend'den söküldü ✅ (2026-07-01)

UI kaldırıldıktan sonra backend kalıntıları da tamamen temizlendi.

**Bütçe/limit sistemi (tamamen kaldırıldı — ajanlar artık koşulsuz sınırsız):**
- `db.Agent.DailyCallLimit`/`DailyTokenLimit` alanları (`db/models.go`) + `db.UpdateBudget`
  (`db/store_usage.go`) silindi.
- `agent/budget.go`: `ErrBudgetExceeded`, `ensureBudget`, `billableTokens` silindi;
  `guardedComplete` artık yalnız global `pauseAutonomy`/`Paused` frenini uyguluyor.
  `agent/toolloop.go`'daki iki per-iterasyon bütçe kapısı + `recovery.go termBudget`
  sabiti + `budget_test.go TestBillableTokens` kaldırıldı.
- API: `POST /api/agents/{id}/budget` endpoint'i + `handleSetBudget`/`setBudgetReq`
  (`api/usage.go`, route `server.go`) silindi; `handleAgentUsage` + `agentBudgetRow`
  (`api/budget.go`) artık limit alanı döndürmüyor; `api/agents.go` varsayılan-bütçe
  tohumlaması kaldırıldı; `api/templates.go` + `api/market.go` + `market/pack.go` template
  agent limit alanları söküldü, `subagent_test.go` güncellendi.
- Settings: `DefaultDailyCallLimit`/`DefaultDailyTokenLimit` (struct/Default/DTO/Patch +
  store apply/clamp) kaldırıldı.
- Frontend: `types/agent.ts` (AgentUsage), `types/usage.ts` (BudgetAgentRow),
  `types/market.ts`, `types/settings.ts` limit alanları + `ChatMeters.tsx` overBudget
  mantığı temizlendi (spend pill yalnız çağrı+maliyet gösteriyor). **Spend takibi
  (RecordUsage + Bütçe ekranı) korunur** — yalnız *limit* kavramı gitti.

**Ölü ayarlar (`mcpGatewayUrl`, `logLevel`):** settings.go (struct/Default/DTO/Patch),
store.go (apply + logLevel clamp), validate.go (+ validate_test.go logLevel case) ve
frontend `types/settings.ts` + save payload'undan tamamen kaldırıldı.

**Doğrulama:** `go build ./...` ✅, `go vet ./...` temiz ✅, `go test` 208 test ✅,
frontend `tsc --noEmit` temiz ✅. Skill dökümanları (`tionswarm-settings`,
`tionswarm-autonomous-ops`) güncellendi.

## Antigravity CLI + Gemini CLI provider'ları tamamen kaldırıldı (2026-07-03)

Deneysel Google dış-ajan adaptörleri (Antigravity CLI `agy` ve daha önce denenen
Gemini CLI) **projeden ve dokümanlardan tamamen kaldırıldı.** Antigravity CLI, agy'nin
doğrulanmış non-TTY stdout bug'ı (google-antigravity/antigravity-cli#76 — pipe/subprocess
altında yanıtı sessizce düşürüyordu) nedeniyle hiçbir zaman üretim-hazır olamadı; Gemini
CLI ise daha önce OAuth yönlendirmesi yüzünden bırakılmıştı.

**Kaldırılanlar:**
- Dosyalar: `providers/antigravitycli.go`, `antigravitycli_live_test.go`, `kind_antigravity.go`.
- `registry.go`: `antigravityCLIPath`/`antigravityKey` alanları, `findAgy`,
  `SetAntigravityCLIPath`/`SetAntigravityKey`/`AntigravityCLIAvailable`, `NewRegistry`
  seeding'i ve `resolve()` doldurma; kullanılmayan `os` importu düştü.
- `kind.go`: `ResolvedConfig`'ten `AntigravityCLIPath`/`AntigravityKey`.
- `api/session_context.go`: CLI-overhead önizlemesi yalnız `claude-cli`'ye daraltıldı.
- `kind_test.go`: katalog artık **5 kind** (`claude-cli`/`anthropic`/`minimax`/
  `minimax-anthropic`/`openrouter`); yorumlar (`builtin_websearch.go`, `toolsetup.go`,
  `interaction/server.go`) ve frontend yorumları (`session.ts`, `SessionContextModal.tsx`)
  temizlendi.

**Doğrulama:** `go build ./...` ✅, `go test ./internal/providers` ✅ (72 test).
Not: OpenRouter üzerinden erişilen Gemini **modelleri** (pricing/context-window/katalog)
bir CLI provider'ı değil — meşru model referansları olarak korundu.
