# TionHarness — İlerleme Takibi

> **Özet (2026-10-18):** 2026-09-28 ve sonrası için yakın dönem değişiklik günlüğüdür. En yeni tarih üstte, aynı günün kayıtları önceki sırasındadır. Önceki ayların kayıtları aşağıdaki arşivlere taşınmıştır; konu sözleşmeleri ilgili rehberlerde, açık işler [yol haritasında](03-YOL-HARITASI.md) tutulur.

Önceki kayıtlar:

- [Temmuz 2026](arsiv/05-ILERLEME-2026-07.md)
- [Ağustos 2026](arsiv/05-ILERLEME-2026-08.md)
- [Eylül 2026, 27 Eylül ve öncesi](arsiv/05-ILERLEME-2026-09.md)
- [Haziran 2026 ve öncesi](05-ARSIV.md)

## Otomasyon sadeleştirmesi: flow türü, tohumlama ve gereksiz kurallar kaldırıldı (2026-10-18)

Otomasyon türleri yalnız **tag** ve **board**: `flow` türü (`automation_trigger_flow.go`,
`automation_flow.go`, flow-run hook'ları) ile akışın `trigger` düğümü (`flowturn_trigger.go`,
`flow.Runner.Trigger`, `Node.AutomationID`) kaldırıldı; `flow` / `phase` / `trajectory_end`
türlü eski kurallar yüklenirken atlanır. Varsayılan tohumlama (`automation_defaults.go`,
`Automation.Seed`, `.seeded-automations.json`) ve boş şablonun PM pano kuralları + "CEO board
heartbeat" zamanlaması kaldırıldı (şablon zamanlama/otomasyon mekanizması market paketleri için
kalır). Hata: `spawnTags: []` `nil`'e indirgenip `[triggerTag]` varsayılıyordu → takılı-onarıcı
şablonu kendini yeniden tetikleyebiliyordu; `normalizeSpawnTags` boş dilimi korur. Ölü kod:
kullanılmayan atlama nedenleri (`disabled`, `autonomy_paused`, `target_missing`,
`empty_prompt`), hiç yazılmayan `TriggerSessionID`/`Driver` alanları, `launchDriver`, sabit
`LaunchResult.Driver`, kullanılmayan `api.archiveAutomation`, eski yorumlar. Frontend'de flow
şeridi/alanları, akış düzenleyicideki trigger düğümü ve ilgili i18n anahtarları silindi.

Veri: 4 workspace'te 19 kural (WS1/WS3/WS6'da 5'er, WS2'de 4), 4 "CEO board heartbeat" ve WS3'teki 10
eski FootballOracle (18 Ekim'e sabitli) zamanlaması silindi; yalnız WS2'de açık olan "Kartı
çalışmaya al" kuralı kaldı. Yedek scratchpad'de (`automations-schedules-backup-20261018.tgz`).

## Sessizlik zaman aşımları birleştirildi; eski sağlayıcı ayarları kaldırıldı (2026-10-18)

`spawnIdleTimeoutMin`, `chatTurnIdleTimeoutMin` ve `turnIdleWatchdogMin` tek
`turnIdleTimeoutMin` ayarında (varsayılan 10 dk, 1–1440) birleşti: sohbet, spawn, zamanlama
ve otomasyon turları aynı pencereyle kesilir; kuyruğun dış watchdog'u (`runQueuedTurn`) artık
ayrı bir ayar değil, `Tunables.QueueIdleBackstop` = 2 × pencere, böylece turun kendi kesmesi
her zaman önce gelir (önceki 10/20 sıralaması korunur). Sohbet için "0 = kapalı" seçeneği
kalktı. Ayarlar ekranında üç alan tek alana indi.

`settings.json`'daki eski sağlayıcı alanları (`claudeCliPath`, `claudeCliAuthKind`,
`claudeCliAuthTokenEnc`, `codexCliPath`, `codexConfigDir`) ve DTO/Patch karşılıkları
kaldırıldı; yalnız `providers.json`'a tek seferlik taşıma için okunuyorlardı.
`MigrateFromSettings`/`EnsureMigrated` → `EnsureSeeded` (`internal/settings/provider_migrate.go`):
`providers.json` yoksa varsayılan claude-cli + codex-cli örneklerini yazar. Katalogdaki Claude
abonelik etiketi auth türünü artık `claude-cli` örneğinin `authKind` alanından okur;
`ClaudeAuthGate` yalnız sağlayıcı örneklerine bakar.

## Bağlam bütçe tavanı 256K → 400K (2026-10-18)

`ContextBudgetCeil` varsayılanı ve canlı ayar 262144 → 400000 (`internal/settings/settings.go`,
`store.go` normalizasyonu, `internal/conversation/budget.go`, `ContextPanel.tsx` önizlemesi).
800K taban hatası düzelince 256K tavan uzun oturumları gereğinden sık katlıyordu. Etkin bütçeler:
Opus/Sonnet/Fable 400K, MiniMax/DeepSeek/Gemini 350K, gpt-6.1-sol 300K, 272K'lık Codex
modelleri ~163K (pencere sınırlı, değişmedi). Ayar denetimi: frontend ve backend ayar alanları
birebir eşleşiyor, diskte bayat anahtar yok; `ClaudeCLIToolAllowlist` yorumundaki kaldırılmış
`AuxNativeRouting` atfı silindi; `agent.DefaultChatTurnIdleTimeoutMinutes` 3 → 10 (ayar varsayılanıyla
eşitlendi, ayarlar uygulanmadan önceki boot penceresinde de codex turu kesilmesin). `_Docs/17` §12 tablosu güncellendi.

## Codex CLI bağlam pencereleri Codex kataloğuna eşitlendi (2026-10-18)

Codex 0.163.0'ın kendi kataloğu `gpt-6.1-sol`'u 500K ile sunuyor, TionHarness ise bütün
Codex GPT-6 modelleri için 272K varsayıyordu; Sol 6.1 oturumlarının bütçesi bu yüzden
163,2K'da kalıyordu (SES361). Ayrıca codex-cli üzerindeki `gpt-5.6-sol`/`terra` API'nin
1,05M penceresiyle bütçeleniyordu (CLI 272K sunuyor; katlama çok geç tetiklenirdi).
`codexCLIContextWindow` (`internal/providers/codex_gpt6.go`) artık codex-cli için CLI
değerlerini döndürüyor: Sol 6.1 → 500K (bütçe 256K tavanında, ~262K), diğer Codex
slug'ları → 272K. Testler güncellendi; ayrıntı `_Docs/69` §15.

## Codex resume home çöp toplayıcı (2026-10-18)

Codex her chat scope'u (oturum + ajan + sağlayıcı + model + sistem promptu) için
`codex-home/resume-homes/<sha256>/` altında kalıcı bir home açıyor, ama hiçbir şey
silmiyordu; prompt/model değişikliği ve silinen oturumlar eski home'u sahipsiz
bırakıyordu. Diskte 180 home 12GB tutuyordu, yalnız 2'si bir oturuma bağlıydı;
178'i elle silindi. Kalıcı çözüm: `internal/providers/codexcli_resume_gc.go`
(`PruneCodexResumeHomes`; canlı thread'li, son 24 saatte yazılmış veya bir turun
tuttuğu home korunur) ve `internal/app/codexgc.go` (açılıştan 5 dk sonra, sonra
6 saatte bir; degraded workspace varsa atlanır; Shutdown'da durur).
`prepareCodexTurnHome` artık tur boyunca home'u tutan bir release döndürüyor.
Tasarım notu `_Docs/69` §4. Doğrulama: yeni testler `-race` ile geçti,
`scripts/test.sh full` geçti.

## Koordinatör/worker, peer mesajlaşma ve Rota grafı kaldırıldı; varsayılanlar ayarlandı (2026-10-18)

Sistem sadeleştirmesi (Kısıt Raporu önerileri). Kaldırılanlar: koordinatör modu ve worker
araçları (`set_coordinator_mode`, `spawn_worker`, `send_to_worker`, `stop_worker`,
`list_workers`, `report_to_coordinator`), worker oturumları, koordinatör el kitabı ve reçeteleri,
stall yargıcı + süpürücü, reçete optimizer'ı, `worker-review` / `phase-gate` karar mercileri,
`send_message` ve peer inbox oturumları, trajectory (Rota) grafı/faz kapıları/faz otomasyonları,
`/api/trajectories*` ve `/api/recipes*`, ilgili ayarlar (`coordinator*`, `agentMessageMaxKB`) ve
"Ürün Ekibi" şablonu. Kalanlar: `run_subagent` (profiller "Worker: X" sistem ajanları),
`spawn_session`, handoff, zamanlama/otomasyon/akış, Rota ekranı yalnız şerit zaman çizelgesi.
Eski `recipe-optimizer` / `stall-judge` sistem ajanları `RetireSystemAgents` ile emekliye ayrılır.
Frontend'de 44 dosya silindi; çalışma durdurma kontrolü `RunStatusStrip` olarak kaldı.

Varsayılan değerler (`internal/settings/settings.go`, global `settings.json` da güncellendi):
`maxContextTokens` 800000→64000 (800K taban, 256K tavanı ve aile oranını her modelde eziyordu;
Opus bütçesi 800K'ya çıkıyordu), gpt/codex oranı 0.35→0.6, `autoCompactMode` rolling→auto,
`spawnMaxConcurrent` 16→4, `spawnQueueMax` 16→8, `spawnIdleTimeoutMin` 5→10,
`chatTurnIdleTimeoutMin` 3→10, `codexStdoutIdleSec` 90→300, `shellDefaultTimeoutSec` 30→120,
`shellMaxTimeoutSec` 120→600 (Claude Code'un belgelenen Bash değerleri), `maxToolOutputKB` 100→50.
E1 ölçümü: `codex exec --json` muhakeme boyunca hiçbir olay yaymıyor (reasoning summary açıkken
de), bu yüzden codex sessizlik bekçisi uzun muhakemeyi kapsayacak kadar açıldı.

Rapor maddeleri: D1 (sıcak CLI izi fold kapısından çıkarıldı, `warmCLIStepTerm`), C1 (akış
gözlemcisi varsayılan `off`; mevcut 14 akış `off`), C2 (öneri motoruna `decider-not-ready` ve
`insight-scan-off` kuralları), A1/A2/A3 (varsayılan talimat ve `tionharness-autonomous-ops`:
commit SHA kanıtı, kart = güncel durum, kural önceliği kullanıcı > workspace > proje > ajan),
D2 (varsayılan talimat sadeleştirildi; WS1/WS2/WS3/WS6 talimatları yenisiyle değiştirildi),
E2 (`scripts/service.sh`: launchd kurulum + boşalt-sonra-dağıt).

Doğrulama: `scripts/test.sh full`.

## FootballOracle izlemesinden çıkan harness düzeltmeleri (2026-10-18)

WS3 (FootballOracle) PM/worker oturumları izlenirken her PM turunda tekrarlanan hatalar ve kendi kendini optimize etme kusurları düzeltildi:

- **Akış gözlemcisi onarım turu** (`internal/agent/flow_optimizer.go`, prompt `flow-optimizer`): WS3'te gözlemci önerilerinin çoğu `invalid` dosyalanıyordu (RE2'nin desteklemediği `(?=`/`(?!` regex kolları, "girdi düğümü tek çıkış kenarı" ihlali). Doğrulamadan geçemeyen yanıt artık bir kez hata metniyle modele geri gider; düzeltilmiş öneri dosyalanır, yine geçersizse iki hatayla `invalid` kalır. Yanıt ayrıştırma ve öneri kurma `parseFlowOptimizerReply` / `flowProposalFromReply` yardımcılarına ayrıldı. Test: `flow_optimizer_repair_test.go`.
- **Gözlemci kapıyı geçmeye optimize edip koordinatörü felç etti** (`rerunsAgentTurn`, `flow-optimizer` prompt'u): karar modeli anahtarı girilince ölçüt kapıları gerçekten değerlendirilmeye başladı ve otomatik kabul açık olduğundan gözlemci PM akışını bir saatte üç kez evirdi. Ret kolundaki istem "yeni test/görev/worker açma, yalnız 'doğrulama bekliyor' yaz" diyordu; PM iş başlatmayı bıraktı. Koordinatör akışlarında ajanın kendi turunu döngüye sokan öneri artık reddedilir; gözlemci prompt'u yeniden deneme isteminin işi yasaklayamayacağını ve bir ifadeyle karşılanan ölçütün iş ölçmediğini söyler. FLW2 ve FLW37 varsayılan akışa geri alındı.
- **Gözlemci backoff'u** (`flowObserverDueRuns`, `FlowStats.QuietPasses`, `DB.NoteFlowObserverPass`): değişiklik getirmeyen her geçiş sonraki otomatik bakışı iki kat erteler (en fazla 8×); dosyalanan öneri sıfırlar. WS3'te gözlemci her 5 koşuda aynı "karar modeli anahtarsız, değişiklik yok" cevabını üretiyordu.
- **Katalog bağlama** (`internal/db/agent_catalog.go`, `catalogLinkFits`): WS3'ün `Worker: Validator` satırı (AGT15), o ID'yi önceden taşımış CEO'nun merkezi kaydına bağlanmıştı; rol CEO profiliyle çözülüyor, `spawn_worker validator` her PM oturumunda "system agent is not seeded" veriyordu. Eşleştirici map'i rastgele sırayla dolaşıp kaynak (`WS3/AGT15`) eşleşmesini sistem anahtarı eşleşmesinden önce kabul edebiliyordu. Yerleşikler artık yalnız kendi rolünün kilitli kaydına bağlanır; açılışta rolüne uymayan bağ temizlenip yeniden kurulur. Test: `TestAgentCatalogBuiltinNeverLinksThroughAReusedOrigin` (düzeltme olmadan düşüyor).
- **Worker → koordinatör mesajı** (`internal/agent/agentmsg.go`, `messageOwnCoordinator`): worker'ın kendi koordinatör ajanına `send_message`'ı inbox yerine koordinatör oturumuna worker notu olarak düşer. Önceki yol PM'in bağlamsız inbox turunu, onun yanıtı da Coder inbox'ında yeni bir turu açıyordu.
- **Anlamsal idle bekçisi araç çalışırken bekler** (`internal/agent/activity_timeout.go`, `internal/providers/codexcli_events.go`): codex araç başlarken kimlikli `Running` karesi yayınlar; izleyici bitiş adımı gelene kadar pencereyi 4 katına çıkarır. Çıktısız uzun test komutu çalıştıran worker'lar 5 dk'da kesiliyordu (SES366).
- **CLI ajanlarına öz-yönetim araçlarını yükleme ipucu** (`internal/agent/toolsetup.go`, `renderLazyToolCatalog`): CLI dalı gizli öz-yönetim araçları için "`ToolSearch` (`select:`) ile bul, `load` et" diyordu; bunlar TionHarness yerleşikleri ve CLI'nin ToolSearch'ü reklam edilmemiş aracı göremez. Codex PM her turda "pano yazma aracı erişilemez" deyip kartları REST API'ye curl ile yazıyordu. Satır artık ağ geçidinin `mcp__tionharness_interaction__tool_search` / `activate_tools` araçlarını gösterir; ToolSearch yalnız harici MCP özetinde kalır.
- **Codex/opencode için gizli araç yolu** (`internal/api/mcp_interaction.go`, `providers.StaticToolList`): codex'in `enabled_tools` listesi gizli öz-yönetim araçlarını (update_task, move_task…) hiç içermiyordu; codex ajanları board'a yazamıyordu. Tam katman artık `tool_search`'ü (şemalarla) ve yeni `call_tool({name, arguments})` aracını ilan eder; dağıtım `dispatchTool`'a ayrıldı, prompt codex'e bu yolu anlatır. 56 aracı her tura eklemek (~15k token) yerine iki küçük araç.
- **Codex köprü ön kontrolünde açılış toleransı** (`internal/providers/codexcli_probe.go`): yeniden başlatmada geri alınan worker turları HTTP dinleyicisi açılmadan başlıyor, MCP ön kontrolü süreç içi `tionharness_interaction/extended` uç noktasını erişilemez bulup turu "required TionHarness bridge unavailable" ile düşürüyordu. Bu iki anahtar artık 15 sn'ye kadar yarım saniyede bir yeniden yoklanır; diğer sunucular tek yoklamada kalır.
- **Oturuma bağlı cron zamanlaması** (`create_schedule.sessionId`, `Scheduler.run` → `deliverWake`): koordinatör saatler sonrası bir pencere için kendi oturumunu uyandırabilir. `schedule_wake` bir saatle sınırlı, cron ise yalnız ajanın zamanlama dizisine teslim ediyordu; WS3 PM'inin maç penceresi tetikleyicileri bu yüzden koordinatöre ulaşmıyordu.
- **İstemcinin iptal ettiği istek 500 değil** (`internal/api/server.go`, `writeDBError`): oturum panelinin `AbortController`'ı yenilemede isteği iptal edince `ReadSessionDecisions`'ın `ctx.Err()` kontrolü `context.Canceled` dönüyor, bu da 500 + ERROR log satırı oluyordu. Artık 499 (client closed request), WARN seviyesinde.
- **Prompt düzeltmeleri** (`internal/workspace/defaults/default-instructions.md`, `internal/prompts/defaults/coordinator.md`): varsayılan workspace talimatından eskimiş iddialar çıkarıldı. Bunlar "OpenAI-compatible endpoint", "`tool_search` + `activate_tools`, bir sonraki turda görünür" (claude-cli'de aynı tur, codex'te `call_tool`; artık her turdaki Available Tools bloğuna yönlendiriyor), "kendini düzenleyen hafıza yok" (notes araçları var) ve kodda olmayan `set_session_labels` / `set_session_status` (yerine `update_session` etiketleri). Aynı düzeltmeler WS3'ün kayıtlı talimatına da uygulandı; WS1, WS2 ve WS6'da aynı satırlar duruyor. Koordinatör prompt'una saate bağlı uyanma (`create_schedule` + `sessionId`) ve worker mesajlarının oturuma gelişi eklendi.
- **Yeniden başlatmada worker bir kez sürdürülür** (`RecoverOrphanedTurns`, `workerResumeNote`): kesilen worker için koordinatöre "killed" bildirmek yerine worker kendi geçmişiyle `SendToWorker` üzerinden "kaldığın yerden devam et" notuyla sürdürülür; sürdürme turunda tekrar kesilirse ya da sürdürme başarısız olursa eskisi gibi "killed" bildirilir. WS3 PM'i 05:27 yeniden başlatmasından sonra "killed" bildirimlerini görüp iki doğrulamayı yeniden görevlendirmeden boşta kalmıştı.
- **"Kimse üretmezken bekliyorum" takılması** (`stall-judge` prompt'u, `decide_stall.go`, `coordinator.md`): WS3 PM'i art arda "doğrulama bekliyor … çalışan worker yok, yeni iş sıraya alınmadı" deyip turu bitirdi; hakem yalnız "yapılmamış devri anlatma"yı arıyordu, dürüst ama boş bir beklemeyi takılma saymıyordu. Hakem (LLM ve karar modeli sorusu) artık hiçbir çalışan işin üretmeyeceği bir sonucu beklemeyi ve kendi işindeki bir FAIL'i raporlayıp kimseyi görevlendirmeden bitirmeyi de takılma sayar. Koordinatör prompt'u da "bekliyorsan o işi bir worker çalıştırıyor olmalı; kesilen worker'ı yeniden görevlendir; boşta bitireceksen beklemenin sahibini adlandır" der. Sürdürme notu da kesilen komutların yeniden çalıştırılmasını açıkça ister (`workerResumeHint`).
- **`conversation_search`** boş sorgu + `session_id` ile o oturumun son turlarını döndürür (önceden "query is required").
- **PATH**: `~/.docker/bin` iyi bilinen bin dizinlerine eklendi (`internal/proc/path_augment.go`); Docker Desktop symlink'siz kurulduğunda ajan kabukları `docker`'ı bulamıyor, kayıtlara "Docker bu hostta bulunamadı" yazıyordu.
- WS3'te `flowAutoAccept` açıldı (kullanıcının 2026-10-17 talebi).

## Akış evrimleri için workspace geneli otomatik kabul (2026-10-17)

- **Neden:** ajan akışlarının gözlemci önerileri Evrim sekmesinde onay bekliyordu; tüm
  akışlar varsayılan `propose` politikasındaydı ve ajan başına `auto` modu akış akış
  açılmak zorundaydı, açılınca da bekleyen öneriler beklemede kalıyordu.
- **Ne değişti:** workspace ayarı `flowAutoAccept` (varsayılan kapalı) eklendi. Açıkken
  `propose` politikalı akışlar `auto` gibi davranır, `off` olanlar kapalı kalır. Otomatik
  kabul güven eşiği ve `flow-proposal-gate` kapısından geçer, sonra Uygula düğmesiyle aynı
  `ApplyFlowProposal` yolunu kullanır. Öneriye uygulayan ayar `autoApplied: flow | workspace`
  olarak yazılır. Anahtar açılınca ya da bir akış `auto`'ya alınınca bekleyen öneriler arka
  planda en eskiden başlayarak uygulanır (`SweepFlowProposals`). UI: Evrim ▸ politika
  kartında hemen kaydedilen anahtar, `propose` akışında "Otomatik gibi davranır" notu ve
  öneri listesinde "otomatik kabul" rozeti. Dosyalar: `internal/agent/flow_autoaccept.go`,
  `flow_optimizer.go`, `internal/workspace/settings.go`, `internal/api/flows.go`,
  `internal/api/workspace_settings.go`, `internal/db/models_flow.go`, `store_flow.go`,
  `frontend/src/features/flows/EvolutionTab.tsx`; ayrıntı [93](93-EVRILEN-AKISLAR.md) §3.
- **Doğrulama:** `flow_autoaccept_test.go` (workspace anahtarı, akış politikası, `off`,
  bekleyenleri tarama, eşik, kapı), `settings_test.go` (`flowAutoAccept` kalıcılığı) ve
  `EvolutionTab.test.tsx` eklendi; `LC_ALL=tr_TR.UTF-8 scripts/test.sh full` yeşil.

## "Bitti" sesi worker'lar çalışırken çalmıyor (2026-10-17)

- **Neden:** koordinatör turu, worker'ları hâlâ çalışırken bittiğinde "yanıt hazır"
  sesi çalıyordu; oturum ise bitmemişti.
- **Ne değişti:** sunucu `chat` tamamlanma olayına bekleyen worker/drain sayısını
  `target.liveWorkers` olarak ekler (`internal/agent/coordination.go`,
  `annotateChatCompletion`; `Runtime.publish` tek giriş noktası). Ön yüz bu sayı > 0
  iken sesi çalmaz, toast'ı gösterir (`notifyBus.emitToast` `silent`,
  `useAppEvents.chatTurnLeavesWorkersRunning`). Ses, oturum ve tüm worker'ları
  bittiğinde, yani son drain turu bitince çalar. Ayrıntı [29](29-BILDIRIM-SINYALLERI.md).
- **Doğrulama:** `chat_completion_annotate_test.go`, `notifyBus.test.ts`,
  `useAppEvents.test.ts`; `scripts/test.sh full` yeşil.

## Yarıda kalan oturumlar: CLI bekçisi uzun araç çağrısında turu öldürüyordu (2026-10-17)

- **Belirti:** bazı oturumlar "yarım kaldı" ya da "durduruldu" görünüyordu. Tarama
  sonucu üç neden çıktı: (1) 2–12 Ekim arası codex/claude CLI oturum açma hataları
  ve erişilemeyen köprü (ortam sorunu, bugün tekrarlanmıyor); (2) bugün 22:34'te
  canlı doğrulama için sunucunun yeniden başlatılması SES288, SES289 ve SES297'yi
  kesti; (3) asıl kod hatası: codex-cli köprü üzerinden `run_subagent` çağırıp
  alt ajanı beklerken 90 sn stdout'a yazmıyor, stdout bekçisi bunu takılma sanıp
  codex'i öldürüyor, iptal edilen köprü isteği de alt ajanı "killed" bırakıyordu
  (SES287 → SES294, SES255).
- **Düzeltme:** codex ve opencode ayrıştırıcıları açık araç öğelerini sayar; araç
  sürerken stdout bekçisi turu öldürmez. Turun anlamsal idle bekçisi (spawn 5 dk,
  sohbet 3 dk) alt ajanın ilerlemesini artık görür: köprü çağrısı turun
  izleyicisini taşır, otonom turlar izleyicilerini run'a kaydeder, alt ajan
  adımları bu izleyiciye düşer. Ayrıntı [69](69-CODEX-CLI-SAGLAYICI.md).
- **Canlı denemede bulunan ikinci neden:** alt koordinatör turu `incomplete` (teslim
  bekçisi: beklenen sonuç dosyası henüz yok) bitince `deferWorkerReport` bunu bozuk
  dal sayıp `stopSubtree` ile az önce başlattığı doğrulayıcıyı öldürüyordu
  (SES314 → SES317, iki kez). Artık `incomplete`, `completed` gibi ertelenir; cascade
  yalnız failed/killed/timeout'ta. Ayrıntı [47](arsiv/47-KOORDINATOR-COKLU-AJAN.md).
- **Kalan risk:** claude-cli kalıcı oturum bekçisi (15 dk) araç takibi yapmıyor;
  15 dakikadan uzun alt ajanlarda aynı kesinti olabilir.
- **Doğrulama:** `codexcli_toolidle_test.go`, `subagent_activity_test.go`,
  `TestAutonomousInteractionRegistersTurnTracker`; `scripts/test.sh full` yeşil.

## Sohbet ajan seçicilerinden worker ve sistem ajanları çıkarıldı (2026-10-17)

- **Neden:** yeni konuşma açarken ve composer'daki ajan listesinde `Worker: Coder`
  gibi worker profilleri ile servis sistem ajanları seçilebiliyordu.
- **Ne değişti:** `isStartableAgent` artık hiçbir sistem ajanını kabul etmiyor (worker
  profilleri için istisna kalktı). Composer'daki ajan seçici `selectableChatAgents`
  ile aynı listeyi gösterir; açık oturumun ajanı listede yoksa (Arka plan'dan açılmış
  worker/sistem oturumu) başa eklenir. "@" otomatik tamamlama tam listeyi korur.
  Dosyalar: `frontend/src/app/startableAgents.ts`, `features/chat/ChatView.tsx`,
  `features/chat/Composer.tsx`; ayrıntı [74](74-SISTEM-AJANLARI.md).
- **Doğrulama:** `startableAgents.test.ts` güncellendi ve genişletildi;
  `LC_ALL=tr_TR.UTF-8 scripts/test.sh full` yeşil.

## Oturumlar paneli iki sekme: Konuşmalar / Arka plan (2026-10-17)

- **Neden:** sohbet listesi worker, subagent, sistem ajanı ve otomasyon oturumlarıyla
  doluyor, kişinin kendi konuşmaları aralarında kayboluyordu.
- **Ne değişti:** kenar çubuğu çiplerin üstünde iki sekme gösterir. **Konuşmalar**
  kişinin başlattığı sohbetleri ve onlardan açılan `spawn_session` / `/handoff`
  sohbetlerini, **Arka plan** koordinatör worker'larını, subagent, görev, zamanlama,
  otomasyon, insight, ajanlar arası mesaj kutularını (`💬 … — Mesajlar`) ve `system:`
  etiketli sistem ajanı oturumlarını tutar. Spawn
  oturumları bilinçli olarak Konuşmalar'da kalır (worker ≠ spawn). Kural ve gerekçesi
  [07 §çipler](07-CHAT-UX.md)'de.
- **Sunucu:** `GET /api/sessions` `group=main|other` alır, sekmeyi sayfalamadan önce
  uygular ve zarfta `groupCounts` döner (`internal/api/sessions_groups.go`,
  `sessions.go`). Geçersiz değer 400.
- **Ön yüz:** `features/sessions/sessionGroup.ts` (istemci ikizi + kalıcı seçim),
  `SessionsSidebar.tsx` (sekme şeridi), `useSessionsController.ts` / `App.tsx`
  (`group` istek kimliğine ve üç liste isteğine eklendi), `api/sessions.ts`.
- **Doğrulama:** `sessions_groups_test.go` ve `sessionGroup.test.ts` eklendi;
  `LC_ALL=tr_TR.UTF-8 scripts/test.sh full` yeşil. Gerçek veride (FootballOracle):
  Konuşmalar 47 (8 kullanıcı + 39 spawn), Arka plan 54 (24 sistem ajanı, 23 subagent,
  2 mesaj kutusu, 2 worker, insight/otomasyon/zamanlama birer).

## opencode-cli örneği açılışta tohumlanıyor, eski API anahtarları opencode'a taşınıyor (2026-10-17)

- **Sağlayıcılar sayfası çöküyordu:** sunucu güncel Go koduyla 7 Ekim tarihli gömülü
  ön yüzü çalıştırıyordu; `npm --prefix frontend run build` sonrası düzeldi. Derleme
  `modelDraft.keyMissing` içindeki kullanılmayan `draft` parametresini yakaladı
  (vitest tip denetimi yapmadığı için `full` kapısı görmemişti); parametre kaldırıldı.
- **opencode-cli listede yoktu:** `providers.json` tek seferlik göçten önce oluştuğu
  için örnek yoktu. `ProviderStore.EnsureKindInstance` eklendi; `app.go` açılışta
  `opencode` ikilisi bulunursa varsayılan `opencode-cli` örneğini ekler.
- **Eski anahtarlar:** `ProviderStore.ExportLegacyKeysToOpenCode` kayıtsız kind
  örneklerinin API anahtarlarını `<dataDir>/opencode-home/data/opencode/auth.json`
  içine kopyalar; mevcut vendor girdisi korunur ([95 §7](95-OPENCODE-CLI-SAGLAYICI.md)).

## OpenCode CLI sağlayıcısı eklendi, native Anthropic kaldırıldı (2026-10-13)

Proje "üç CLI" modeline geçti: `claude-cli`, `codex-cli` ve yeni `opencode-cli`
([95](95-OPENCODE-CLI-SAGLAYICI.md)).

- **Yeni taşıyıcı:** `internal/providers/opencodecli*.go` + `kind_opencodecli.go`.
  Tur başına `opencode run --format json --agent tionharness` alt süreci; prompt
  stdin'den, sistem promptu ve MCP sunucuları tur dizinindeki `opencode.json`
  (`OPENCODE_CONFIG`) üzerinden, ev `XDG_*` ile izole (`<dataDir>/opencode-home` veya
  örneğin `configDir`'i). Olay ayrıştırıcı `step_*`/`text`/`reasoning`/`tool_use`/
  `error` olaylarını `TraceStep`'e çevirir; `todowrite` → `todo_list`, köprü araçları
  `mcp__<server>__<tool>`. Resume `ScopedCLIResumer` + `<home>/scopes` kaydıyla;
  `planCodexResume` paylaşılır. Model id'leri `vendor/model`; aile tabloları
  `BareModelForKind` ile çalışır. Canlı doğrulama (opencode 1.19.0, ücretsiz
  `opencode/big-pickle`): düz tur, köprülü `todo_write` turu, ikinci turda önbellek
  okuması; birim testleri `opencodecli_test.go`.
- **Ajan/API:** `PinOpenCodeHome`, `usesServerSpec` (codex + opencode aynı MCP spec
  yolunu kullanır), `provider_auth` durum satırı (giriş `opencode auth login` ile),
  katalog `cliVersion`/`subscription`, `chat_resume` eşlemesi, `debug_journal` ve
  thinking varsayılanı. Frontend: metadata/i18n/`ProviderInstanceList` durum satırı.
- **Kaldırılan:** `anthropic.go` ve Anthropic-protokollü kind'lar (`anthropic`,
  `anthropic-compat`, `minimax-anthropic`, `deepseek-anthropic`, `zai`),
  `TokenCounter`, registry betaları, Anthropic ayar anahtarları (`anthropicKey`,
  `extendedPromptCache`, `anthropicContextEditing`, `anthropicNativeToolSearch`,
  `anthropicProgrammaticTools`, `anthropicWebTools`, `anthropicServerCompaction`,
  `anthropicRefusalFallback`, `autonomousTaskBudgetTokens`), `auxNativeRouting` ve
  `systemagent_route.go`, `prefix_calibration.go`, `AnthropicOptions.tsx`,
  `ANTHROPIC_API_KEY` tohumlaması. Market "anthropic" türü sağlayıcı paketleri artık
  açıklayıcı hatayla reddedilir.
- **İkinci adım (aynı gün, [95 §7](95-OPENCODE-CLI-SAGLAYICI.md)):** karar katmanı
  sağlayıcı örneklerinden anahtar ödünç almayı bıraktı (her karar modeli kendi
  `baseUrl` + anahtarıyla; eski "provider" kayıtları `own` olarak yüklenir, şablonlar
  OpenRouter anahtarını doğrudan ister) ve OpenAI-uyumlu HTTP ailesi (`openrouter`,
  `deepseek`, `minimax`, `lmstudio`, `openai-compat`) istemcileriyle birlikte silindi.
  Kalan kind'lar yalnız `claude-cli`, `codex-cli`, `opencode-cli`. Market "provider"
  paketleri artık reddedilir; `/api/providers` `reachable` alanını bıraktı. Native
  tool loop test altyapısı nedeniyle duruyor (95 §6/2).
- **Ajan formu:** `ask` izin modunda codex-cli/opencode-cli için "köprü onayı" notu
  (`settings.bridgeApprovalNote`).
- **Testler:** kaldırılan kind'lara dayanan testler `openrouter`/`deepseek`'e
  taşındı; `TestInstallProviderPack_AnthropicKindRejected` eklendi; frontend
  `settingsFields`/`providerMetadata` testleri güncellendi.

## OpenPencil desteği kaldırıldı (2026-10-10)

OpenPencil (`op` CLI) ile ilgili her şey projeden çıkarıldı:

- **Harici araçlar:** `openpencil` katalog girdisi, `op`/1Password ad çakışması
  koruması (`openPencilExe`, `TIONHARNESS_OPENPENCIL`) ve testleri silindi. Panelde
  **Tasarım** kategorisi artık yok (`externalTools.categories.design` anahtarı da).
- **Prerelease akışı:** yalnız OpenPencil için var olan `Tool.PreRelease`,
  `LatestPreRelease` ve `/releases` listesine düşüş kaldırıldı; sürüm kontrolü
  yalnız `releases/latest` kullanır. Eski önbellekteki `repo#pre` kayıtları zararsızdır.
- **Varsayılan talimatlar:** `openpencil-design` skill'ini yükle notu çıkarıldı. Daha
  önce oluşturulmuş workspace'lerin kayıtlı talimatları değişmez.
- **Dokümanlar:** `_Docs/design/` ve arşivdeki `.op` belgeleri ile OpenPencil JS
  kaynakları silindi, PNG önizlemeleri duruyor; [54](54-CAPABILITY-PROBE.md)
  bölümü kaldırıldı.

## macOS/Linux uyumluluğu: Windows varsayımları temizlendi (2026-10-07)

Proje Windows'ta geliştirildiği için yol, kabuk ve araç bulma varsayımları
macOS/Linux'ta kırılıyordu. Kapsamlı tarama (Go, frontend, betikler) sonrası:

- **Yollar:** yeni `internal/fspath` paketi — Windows ve macOS (APFS) büyük/küçük
  harf duyarsız karşılaştırılır; macOS'ta katlanmış eşleşme `os.SameFile` ile
  doğrulanır. `/api/files` POSIX mutlak yolların başındaki `/`'ı artık silmiyor
  (`file:///C:/` hariç). Sandbox ve artifact sınırı `/tmp`, `/var` → `/private`
  sembolik bağlı köklerde doğru çalışıyor. Başka OS'tan gelen workspace yolu
  (`C:\...` Mac'te) bozuk (degraded) listelenir.
- **Kabuk:** Bash aracı ve hook'lar Unix'te gerçek `bash` (yoksa `/bin/sh` + ajana
  uyarı). Windows'ta yazılmış PowerShell hook'ları Mac/Linux'ta `pwsh` ile çalışır;
  `pwsh` yoksa net hata ve Hooks ekranında uyarı (`shellWarning`).
  `${CLAUDE_PLUGIN_ROOT}` boşluklu yolda kabuğa göre tırnaklanır.
- **Araç bulma:** başlangıçta login-shell PATH'i + Homebrew/`~/.local/bin`/npm/bun
  dizinleri eklenir (`proc.AugmentPATH`; Finder/launchd başlatmasında `claude`,
  `node`, `rg` bulunur). `claude`/`codex` CLI için yedek konum listesi; STT
  Homebrew whisper/ffmpeg'i bulur; macOS'ta süreç başlangıç zamanı `ps` ile.
- **Kimlik:** macOS'ta claude-cli girişi Keychain'den okunur ve yazılır
  (`internal/claudeauth/keychain.go`; servis adı `Claude Code-credentials[-sha256(dir)[:8]]`,
  canlı makinede doğrulandı).
- **Linux cgroup v2:** kısıtlı kabuk komutları kendi cgroup'unda; `pids.max`
  (ActiveProcesses × 16 iş parçacığı) ve `cgroup.kill` ile `setsid` kaçağı dahil
  tüm ağaç öldürülür; yazılamazsa süreç grubuna düşer (`proc.ContainmentStatus`).
- **UI:** `/api/version` `os`/`arch` döner; yol placeholder'ları sunucu OS'una,
  kısayol etiketleri tarayıcıya göre (⌘/Ctrl). `/Users/...` ile başlayan mesaj artık
  slash komutu sayılmıyor; Galeri POSIX görsel yolları; boşluklu POSIX yol çipleri.
- **Betikler:** `scripts/dev.sh` (dev.ps1 karşılığı), `scripts/lib/common.sh`,
  `serve.sh` sürüm ldflags; `launch.json` mac girdileri (`tionharness-mac-dev`,
  `tionharness-mac-scratch`); `.githooks/pre-commit` ve `.sh` dosyaları çalıştırılabilir.
- **Doğrulama:** macOS'ta `scripts/test.sh full` Go tarafı tam geçti (önceden 7 hata);
  vitest'te yalnız önceden var olan 20 hata. Linux cgroup entegrasyon testleri
  derleniyor ancak Linux ortamı olmadığından çalıştırılmadı. Keychain gizli veri biçimi
  (JSON/hex) iki biçimi de kabul edecek şekilde yazıldı; canlı okuma uygulamada doğrulanmalı.

## Sahipsiz iç işler sistem ajanı kimliğiyle (2026-10-07)

Denetimin ardından kullanıcı kararları (1, 2, 3 evet; 4 hayır; 5 olduğu gibi):
içgörü taraması (oturumu dahil) ve reçete iyileştirici artık `insight` /
`recipe-optimizer` sistem ajanı olarak koşup ona faturalanıyor; sahipsiz başlıklar
(otomasyon/zamanlayıcı/sahipsiz görev) en yeni ajan yerine `titler` sistem ajanıyla;
`run_subagent` profilleri, profilin sistem ajanı sağlayıcı (sabitlenmiş) veya model
belirtiyorsa onları kullanıyor. Sistem ajanının sağlayıcısı sunulamazsa yalnız
taşıyıcı varsayılan ajandan ödünç alınır. `/handoff` sıkıştırma rolünde, karar modeli
maliyeti çağıranda kaldı. Yeni dosya `internal/agent/systemagent_caller.go`; testler
`systemagent_caller_test.go`. Ayrıntı [74 § Sahipsiz işlerin kimliği](74-SISTEM-AJANLARI.md).

## Rol atamaları yalnız sistem ajanlarına (2026-10-07)

Kullanıcı kararıyla rol seçicileri yalnız rolün kendi sistem ajanlarını (yerleşik +
türetilen özelleştirme) sunar; normal ajan atamaları backend'de de reddedilir ve eski
kayıtlarda yok sayılır. "Normal ajandan model ödünç alma" ve "worker profilini normal
ajana yönlendirme" kaldırıldı. Seçim boşken yerleşik ajan avatar/ad/modelle "yerleşik"
rozetiyle görünür. İçgörü kenar çubuğundaki "Analiz ajanı" artık `insight` rol
seçicisidir (`insightAgentSelection.ts` silindi). Sözleşme
[74 § Workspace rol atamaları](74-SISTEM-AJANLARI.md).

Denetim (aynı gün): arka plan LLM işlerinin çoğu sistem ajanının prompt+modelini
kullanır ama kimlik/faturalama çağıran normal ajandadır; normal ajana bağlı kalan yerler
`pickInsightAgent` (içgörü taraması — tarama oturumu da o ajana ait — ve reçete
optimizer'ı), `TitleFor`/`resolveTitlerAgent` (otomasyon/zamanlayıcı/görev başlıkları,
en yeni ajanı seçer), `run_subagent` profilleri (yalnız prompt sistem ajanından; model
çağırandan), `/handoff` (rolü yok) ve karar modeli çağrıları (çağırana faturalanır).

## Sistem rolleri için workspace ajan atamaları (2026-10-07)

İçgörü'deki "Analiz ajanı" seçicisi genelleştirildi: her workspace, worker
profillerini ve sistem yardımcılarını (başlık, sıkıştırma, özet, içgörü, ders,
akış/reçete gözlemcisi, takılma hakemi) hangi ajanın çalıştıracağını seçebilir.
Sözleşme [74 § Workspace rol atamaları](74-SISTEM-AJANLARI.md).

- **Backend:** `ws-settings.json` → `systemAgentAssignments` (yama birleşir, boş id
  temizler); `internal/agent/systemagent_assign.go` doğrulama + çözümleme.
  Yardımcı rollerde atanan ajandan yalnız sağlayıcı/model ödünç alınıp sabitlenir;
  worker rollerinde profil adı atanan ajanı kendisi olarak çalıştırır.
  `insight-applier` atanamaz. Geçersiz atama API'de 400.
- **UI:** Workspace'e **Ajanlar** sekmesi (tüm roller, gruplu); İçgörü ▸ Ayarlar'a
  İçgörü ajanları kartı; Akışlar ▸ Evrim'e Gözlemci ajanı seçicisi. Ortak bileşen
  `shared/components/agents/SystemRolePickers.tsx`. İçgörü "Analiz ajanı" varsayılanı
  artık sistem ajanı seçmiyor.
- **Doğrulama:** `systemagent_assign_test.go`, `TestUpdateSettingsMergesRoleAssignments`,
  `systemRoles.test.ts`; scratch sunucuda (8091) atama/temizleme/400 reddi ve üç ekran
  elle denendi.

## Harici araçlar: codex tek tıkla güncellenir (2026-10-07)

Codex CLI, Ayarlar ▸ Harici araçlar panelinde artık "Güncelle" düğmesi ve
kopyalanabilir komut alır (önceden yalnız manuel nottu). Ayrıntı
[69](69-CODEX-CLI-SAGLAYICI.md).

- **Kurulum sahibine göre komut:** `internal/exttools/codex.go`
  `codexUpdateSpec` algılanan yolu symlink'lerden geçirip okur. npm global
  kurulumda `npm install -g --prefix <prefix> @openai/codex@latest` (prefix
  kurulumun kendisinden; PATH'teki npm başka bir Node'a ait olabilir), Homebrew
  cask'ta `brew upgrade --cask codex`, tanınmayan kurulumda manuel not.
- **Katalog:** `Tool.ResolveUpdate` + `Tool.EffectiveUpdate(path)`; API
  (`internal/api/external_tools.go`) hem listede hem güncelleme uç noktasında
  yola göre çözülmüş spec'i kullanır. `UpdateCommandLine` boşluklu argümanları
  tırnaklar.
- **Meşgul koruması:** `Tool.ProcLabelPrefix` ("codex") ve
  `exttools.RunningProcesses`; procwatch'ta çalışan bir codex sağlayıcı süreci
  varken `RunUpdate` `ErrToolBusy` döner, uç nokta 409 verir. Kullanıcının kendi
  terminalinde çalıştırdığı codex ledger'da görünmez.
- **Doğrulama:** `internal/exttools/codex_test.go` (npm symlink/shim, cask,
  bilinmeyen kurulum, tırnaklama, meşgul koruması). Bu Mac'te canlı:
  `/api/external-tools` codex için `npm install -g --prefix /opt/homebrew
  @openai/codex@latest` döndü, `POST /api/external-tools/codex/update` başarıyla
  koştu (0.160.1, güncel).

## Harita: zaman penceresi, ajan okuma defteri ve sadeleştirme (2026-10-05)

Harita (Explorer) artık Rota'daki gibi zamana göre süzülür ve ajanların neyi ne
zaman okuduğunu gösterir; araç çubuğu üç satırdan bire indi. Ayrıntı
[68 §7.3](68-OZET-HARITASI.md).

- **Zaman penceresi:** 1 sa · 6 sa · 24 sa · 3 gün · tümü (varsayılan 24 sa).
  Düğüm zamanı = oluşturulma, düzenlenme ve ajan okumasının en yenisi; pencere dışı
  gizlenir, pencere içi düğümün köke giden yolu ve canlı oturumlar kalır
  (`explorerFilter.ts` `recentWithAncestors`). Temizle pencereyi korur.
- **Damgalar:** `/api/views/graph` ve `/graph/live` yanıtında `times`
  (`view.GraphTimes`, `internal/view/graph_times.go`).
- **Ajan okuma defteri:** workspace store'da `view-reads.json`
  (`db.RecordViewRead`, 60 sn birleştirme, 5000 kayıt sınırı). Başarılı `get_view`,
  `read_artifact`, `use_skill`, `note_expand` çağrıları native döngüde
  (`toolloop_phases.go`) ve CLI köprüsünde (`mcp_interaction.go`)
  `Runtime.RecordEntityRead` ile kaydedilir (`tools.ReadRefFor`).
- **Yan panel:** "Son okuyan: <ajan> · N dk önce" ve "Düzenlendi · …"
  (`ExplorerNodeTimes.tsx`).
- **Sadeleştirme:** başlıktaki düğüm/bağlantı özeti kalktı, yenile yalnız ikon;
  durum şeridi + filtre satırı tek satır (yalnız sıfır olmayan sayaçlar, not/özet
  bilgisi ⓘ ipucunda); **Canlı** çipi kalktı ("çalışan" sayacı aynı süzgeç);
  katmanlar/tür/ajan/etiket ve yoğunluk varsayılan kapalı Filtreler panelinde.
- **Düzeltme:** TTS kaldırmasından `AssistantTurn.tsx`'te kalan tanımsız
  `canSpeak` referansı silindi (`tsc` kırıktı, balon çalışma anında patlıyordu).

Doğrulama: yeni Go testleri (`store_view_reads_test.go`, `view_reads_test.go`,
`graph_times_test.go`) ve vitest (zaman penceresi, varsayılan, temizle) geçti;
`scripts/test.sh full` vitest 1253/1253, depcheck ve `git diff --check` temiz;
`go test ./...` yalnız bu Mac'e özgü bilinen 7 testte ve temiz HEAD'de de aralıklı
düşen `workspace` `TestDeleteMarksAndClearsRemovalOnTheProductionPath`'te hata
veriyor. Scratch veriyle
canlı denemede 1 sa penceresi 65 düğümü 47'ye indirdi, yan panel kaydedilen okumayı
"Son okuyan: Titler · 7 dk önce" olarak gösterdi.

## Sesli okuma (TTS) desteği kaldırıldı (2026-10-05)

Ajan yanıtlarını sesli okuma özelliği motorlarıyla birlikte tamamen çıkarıldı;
sesle yazma (STT/whisper) ve ses efektleri olduğu gibi kaldı.

- Backend: `internal/tts` paketi (Piper CLI sarmalayıcısı) ve
  `internal/api/tts.go` (`GET /api/tts/status`, `POST /api/tts`) silindi.
  Harici araç kataloğundan `piper` girdisi ve yalnız onun için var olan
  `exttools/versionprobe.go` (venv python'ı ile sürüm sorma) kaldırıldı; sürüm
  probu artık her araçta `(path, VersionArgs)`.
- Frontend: `shared/lib/tts.ts`, `api/tts.ts`, `TtsSettings`,
  `TtsVolumeSlider`, asistan balonundaki 🔊 butonu, tur bitiminde otomatik okuma
  (`useAppEvents`), boot'taki `initServerTts`/`initTtsUnlock` ve ilgili i18n
  anahtarları (en/tr) silindi. Ayarlar ▸ Ses artık yalnız ses efektleri + STT;
  harici araç kategorisi etiketi "Ses (STT)".
- Dokümanlar 07, 32, 54 güncellendi. Cihazlarda kalmış TTS `localStorage`
  tercihleri artık okunmaz.

Doğrulama: `go build ./...`, `go vet`, `tsc --noEmit` temiz; vitest
(`LC_ALL=tr_TR.UTF-8`) 1249/1249 geçti; depcheck ve `git diff --check` temiz.
`go test ./...` yalnız bu Mac'e özgü, HEAD'de de tekrarlanan 7 testte
(`agent`, `api`, `tools` — `/private` symlink) hata veriyor.

## Btw yan sohbet özelliği kaldırıldı (2026-10-05)

Oturumlardaki "btw" yan soru özelliği (araçsız, transkript-dışı tek seferlik
danışma) backend'den ve UI'dan tamamen çıkarıldı.

- Backend: `POST /api/chat/btw` rotası ve `internal/api/chat_btw.go`,
  `internal/agent/btw.go` (`AskBtw`, `buildBtwRequest`) ve testleri silindi;
  `KindBtw` / `db.UsageKindBtw` ve debug journal'daki `btw` kind değeri
  kaldırıldı (`isAuxiliaryKind` artık title/summary/reflect/compact).
- Promptlar: `btw-system` / `btw-preamble` kayıt girdileri ve
  `internal/prompts/defaults/btw-*.md` silindi. Workspace'te kalmış
  `config/prompts/btw-*.md` override'ları artık hiçbir yerde okunmaz.
- Frontend: composer'daki btw butonu, `BtwPanel`, `chatApi.btw`,
  `BtwResponse` tipi ve `composer.btw*` / `btw.*` i18n anahtarları (en/tr)
  kaldırıldı.
- Etkin dokümanlar (07, 17, 38, 61, 74, 93, maliyet planı, analiz notu)
  güncellendi; arşiv kayıtlarına dokunulmadı. Eski usage/journal satırlarındaki
  `btw` kind string'i olduğu gibi kalır.

Doğrulama: `go build ./...`, `tsc --noEmit`, eslint temiz; vitest
`LC_ALL=tr_TR.UTF-8` ile 1249/1249 geçti. `go test ./...` bu Mac'te
değişiklikten bağımsız 3 pakette (`agent`, `api`, `tools` — `/private` symlink
kaynaklı) hata veriyor; aynı hatalar değişmemiş HEAD üzerinde de birebir
tekrarlandı. depcheck ve `git diff --check` temiz.

## Harita canlı durum katmanı: olay → delta, dikkat halkaları, durum şeridi (2026-10-04)

- Amaç: kullanıcı workspace'te şu an ne olduğunu haritadan okusun. Farkındalık
  katmanı sinyal kaynağı, harita yüzey. Tasarım: [68 §7.2](68-OZET-HARITASI.md),
  [94 §10](94-FARKINDALIK-VE-NOTLAR.md).
- Backend: `awareness.CollectOpenLoops` yapılandırılmış açık döngü taraması (brifing ve
  nabız aynı veriden metin üretir; koşu durumu `db.FlowFailure` ile de eşleşir);
  `api/views_attention.go` haritaya `attention` (ref → düzey + nedenler) ve `status`
  sayaçlarını ekler; yeni `GET /api/views/graph/live` yalnız oynak katmanları döner.
  Testler: `views_attention_test.go`, rota tablosu girişi.
- Frontend: `eventToRefreshSignals` `explorer` (yapı) / `explorer-live` (katman)
  ayrımı; `explorerAttention.ts` (facet eşlemesi, `mergeLive`, `changedKeys`, spot
  ışığı süresi); `useExplorerGraph.refreshLive`; `explorerVis` düzeye göre halka ve
  10 sn spot halesi; `ExplorerStatusStrip` sayaçları aynı zamanda süzgeç
  (`ExplorerFilter.attention`). i18n `explorer.status.*`, `tooltip.attention.*`.
- Canlı testte bulunan eski hata: `useAppEvents.onEvent` `session` dalı panel
  sinyallerini bump'lamadan dönüyordu; oturum olayları (oluştur/sil/etiket/durum)
  haritaya ve yürütme akışına hiç ulaşmıyordu. Dal artık workspace süzgeciyle
  `bumpSignalsForEvent` çağırır. Doğrulama: etiket değişimi `GET /api/views/graph`,
  pin değişimi `GET /api/views/graph/live` tetikledi; şerit ve halka olayla güncellendi.
- Canlı LLM testi (claude-cli ve codex-cli): tur boyunca çalışan sayacı ve avatar
  parıltısı, `remember` ile yeni not düğümü, başarısız kart halkası ve sayacı, iki araç
  hatası sonrası "notice" halkası, codex `spawn_session` ile yeni oturum düğümü
  doğrulandı. Bulgu: etkileşimli `ask_user` "seni bekleyen"e yansımıyordu (kalıcı
  `SessionAsk` yok); `interactionStore.pendingSessions` dikkat katmanına katıldı ve
  yeni `interaction` kontrol olayı (`open/resolve/cancel`) `explorer-live`'ı tetikler.

## Harita rotaları geri bağlandı, projeksiyon kopyaları kaldırıldı (2026-10-04)

- Hata: `f9ab586b` akış rotalarını `flows.go`'ya taşırken eski `registerFlowRoutes`
  içindeki `GET /api/views/{graph,{kind}/{id},…/children,…/neighborhood}` ve
  `GET /api/dashboard{,/commit-activity}` kayıtları silinmişti; istekler SPA
  catch-all'ına düşüp HTML dönüyordu, Harita ve Panel ekranları açılmıyordu.
  Kayıtlar `internal/api/views.go` → `registerViewRoutes`'a alındı; `Routes()`
  mux kurulumu `routeTable()` olarak ayrıldı ve `routes_test.go` frontend'in
  çağırdığı `/api` yollarının API kalıbına çözüldüğünü sabitler.
- Ham projeksiyon artık yalnız Harita yan panelinde: Görevler panosu başlığındaki
  `◱ Özet`, Panel eylem kuyruğu satırlarındaki `◱`, Panel'deki "Çalışma alanı
  özeti" bloğu, Rota yan panelindeki `ViewPanel` ve Oturum bilgisi panelinin
  katlanır "Özet" bölümü kaldırıldı. `ViewButton.tsx`, `view.json` `button.*`,
  `dashboard.json` `workspaceSummary.*`, `sessions.json` `detail.summary` silindi;
  Rota yan paneli seçimi kapatan bir `✕` (`panel.clearSelection`) ve seçimdeyken de
  etkinlik şeridini gösterir. `/api/dashboard` yanıtından `summary` alanı çıkarıldı
  (`TestDashboardSummaryMatchesWorkspaceView` kaldırıldı). Doküman: [66](66-VIEW-KATMANI.md).
- UI taramasında bulunan ek hata: `dashboard` görünümü `url.ts` `VIEWS` tablosunda
  yoktu; `#/w/WS/dashboard` derin bağlantısı ve sayfa yenileme sohbete düşüyordu.
  Eklendi; `url.test.ts` artık `VIEW_METADATA`'daki her görünümün derin bağlantısını
  sabitler.
- İkinci ek hata: `api.viewProjector` not deposunu (`ViewSources.Notes`) geçmiyordu;
  harita boş bir Notlar kovası çiziyor, `GET /api/views/note/{id}` "notes store
  unavailable" dönüyordu (ajanın `get_view`'ı notları listeliyordu). `rt.Notes()`
  bağlandı; `views_notes_test.go` gerçek rota tablosu üzerinden not projeksiyonunu ve
  harita düğümünü sabitler. Bu test üçüncü hatayı da açığa çıkardı: `structuralCache.children`
  `note` türünü tanımıyordu, tek bir not olan workspace'te tüm harita yürüyüşü
  "children unsupported" ile 500 dönüyordu; artık `noteChildren` (düzeltme ardılı +
  wikilink kenarları) üzerinden çözülür.

## Workspace farkındalığı ve hafıza notları (2026-10-04)

- Yeni `internal/notes`: Markdown + frontmatter not deposu (`store/notes/`), erişim
  (`workspace|agent|project`), güven, supersede zinciri, imza tekilleştirme, lexical arama,
  wikilink/backlink, arşiv; silme yalnız kullanıcıya. Yeni `internal/awareness`: üretici
  arayüzü, bütçe + sayaç + işaretçiye düşürme (`Compose`), oturum başına dondurulan
  **brifing**, nabız dedupe'lu **tur** bileşimi, LLM'siz **kapanış özeti** (`digest.json`,
  `awareness/digests.json`). Tasarım ve sözleşme: [94](94-FARKINDALIK-VE-NOTLAR.md).
- `composeTurnRequest` ve `autonomousDynamicSuffix` artık bölümleri `Runtime.TurnBlock`'a
  verir; brifing `buildStaticPrefix` / `autonomousSystemPrompt` ile statik öneğe girer,
  ilk tur ve compaction'da yenilenir. `sessionsContextBlock`, `todoContextBlock`,
  `artifactsContextBlock`, `LessonsContextBlock` kaldırıldı.
- Dersler nota taşındı: `lessons.jsonl`, `read_lessons`/`delete_lesson`, `/api/lessons`,
  `lessonMaxAgeDays` kaldırıldı; ders çıkarıcı ve insight `lessons-mining` `lesson` notu
  yazar. Yeni araçlar `remember`, `record_work`, `note_search` (eager), `note_expand`,
  `note_correct` (name-only). `get_view` `note` türü ve Explorer `Notlar` kovası.
- Dört karar mercii (gölge): `brief-relevance`, `pulse-urgency`, `wrap-up-memory`,
  `note-supersede`. Workspace ayarlarına `awareness` bölümü; REST `/api/notes*`,
  `/api/awareness/*`, `/api/sessions/{id}/{digest,awareness}`; olaylar `notes`,
  `awareness_digest`; `focus_view` `notes`. Skill `tionharness-notes`; guide, insight,
  progress ve autonomous-ops skill'leri güncellendi. `scripts/depcheck.sh` iki yeni leaf.
- Testler: `internal/notes`, `internal/awareness`, `internal/tools/builtin_notes_test.go`,
  `internal/agent/awareness_test.go`, `internal/api/{notes,awareness_delivery}_test.go`
  (teslimat kapısı: gerçek `/api/chat` ucu, kayıt eden sağlayıcı).
- Canlı test bulguları ve düzeltmeleri: eager hafıza araçları CLI köprüsüne
  (`tools.bridgeEager`) eklendi (claude-cli/codex-cli ajanları `remember`/`record_work`/
  `note_search`'ü göremiyordu); `/refresh-context` ve eşik tetikli epoch yenilemesi
  brifingi de yeniler; boş `expand` komşulukları `[]` döner (Notlar detay sayfası
  `null.length` ile çöküyordu); ders notu başlığı üç nokta yerine sözcük sınırında
  kesilir. Senaryo listesi [94 §8](94-FARKINDALIK-VE-NOTLAR.md).

## JEV karar mekanizmaları ve akış ↔ otomasyon bağlantısı (2026-10-04)

- `internal/flow`: `route.mode = criteria` (`criteria[]`, `pass`/`fail` kolları, adım
  `detail`) ve `trigger` düğümü (`automationId` + yük şablonu; geçiş düğümü). Motor
  `Runner`'a `Check` ve `Trigger` eklendi; `Event`/`Step` `detail` taşır.
- Üç yeni karar mercii (`decide_authorities.go`): `flow-criteria` (ölçüt başına noul, tek
  çağrı), `flow-grade` (her başarılı koşuyu 1..5 puanlar; `FlowRun.Grade`, `FlowStats.
  Graded/GradeSum`; varsayılan kapalı), `flow-proposal-gate` (auto politikada öneri
  uygulanmadan önce "iyileştirir mi" kapısı; varsayılan gölge; tutulan öneri `pending`
  kalır, `FlowOptimizeResult.Held`). Gözlemci kanıtına puanlar ve ölçüt ayrıntıları girdi;
  pencere sağlıklıysa (`flowRunsHealthy`) gözlemci çağrısı atlanır.
- Koşu sonrası arka plan kuyruğu `afterFlowRun`: puanla → `FireFlowRunFinished`
  (`AddFlowRunHook`) → gözlemci. Otomasyon motoruna `flow` tetik türü (`automation_flow.
  go`: `flowAgentId` / `flowStatus` / `flowMaxGrade` filtreleri, kendi açtığı oturumu
  yeniden ateşlememe) ve `trigger` düğümünden ateşleme (`fireFromFlowNode`); `RunTrigger`
  `automation:flow` ve `flow:node`. REST ve arayüz (Otomasyonlar ekranında sekizinci
  şerit, akış kuralı alanları; Akışlar inceleyicisinde ölçüt listesi ve otomasyon seçici;
  koşu listesinde puan rozeti; karar mercileri çevirileri) güncellendi.
- Testler: `flow_test.go` (ölçüt kapısı, tetikleyici), `flowturn_decider_test.go` (karar
  stub'ıyla beş senaryo), `automation_core_test.go` (`flow` şekli),
  `automationPayload.test.ts`. `scripts/depcheck.sh` paket listesi `flows` → `flow`.
  Beyin fırtınası ve uygulanmayan fikirler [93 §8](93-EVRILEN-AKISLAR.md).
- Düzeltmeler: `thread` düğümü, son mesajı bir eş ajanın yanıtı olan (çok ajanlı oturum)
  isteğe `{{input}}` promptunu yeniden eklemiyor (e2e `TestMultiAgent_SequentialReplies
  ShareHistory`; yeni `TestFlowTurnKeepsPeerReplyTail`). Akış kaldırmasından kalan
  `internal/tools` testleri (workspace 10 kova, `create_automation` demeti, görev `flowId`
  doğrulaması) güncellendi. Evrim sekmesi dar genişlikte taşmıyor.
- Canlı doğrulama (Flow Lab, OpenRouter gpt-4o-mini + Jev): ölçüt kapısı 1. turda
  "en fazla iki cümle" ölçütünü düşürdü → düzeltme düğümü → 2. turda pass; tetikleyici
  düğüm özet otomasyonunu ateşledi; koşu 2/5 puan aldı ve akış-türü kural eleştirmen
  oturumunu açtı; gözlemci puanı ve kapı ayrıntısını okuyup ölçütü kaldırmayı önerdi,
  öneri kapısı (JEV, %80) onayladı ve v3 kendiliğinden uygulandı.

## Evrilen akışlar: orkestrasyon akışları kaldırıldı, ajan-başına ana akış geldi (2026-10-04)

- Eski flow sistemi (`internal/orchestration`, `internal/flows`, `internal/agent/flow*.go`,
  adhoc flow, flow-backed schedule/automation/task, market flow paketi, `flow` /
  `flow-coordinator` oturum türleri, Rota ve görünüm katmanındaki flow koşusu düğümleri,
  eski `features/flows` ekranı) tamamen kaldırıldı. Dokümanlar `arsiv/15` ve `arsiv/62`'ye
  taşındı; tasarım ve sözleşme [93-EVRILEN-AKISLAR.md](93-EVRILEN-AKISLAR.md).
- Yeni `internal/flow` yaprak motoru: `input` / `llm` / `route` / `transform` / `output`
  düğümleri, açık kenarlar (döngü ve geri besleme serbest), `maxSteps` + route `maxVisits`
  ile kodda zorlanan sonlanma, `{{input}}`/`{{last}}`/`{{node.<id>}}` şablonları, `flow.Op`
  yama sözlüğü, `Diff` ve `Summary`.
- Her ajan için tek ana akış (`EnsureAgentFlow`, varsayılan `girdi → yanıt → çıktı` düz
  turla aynı); `completeTracedInner` her turu akıştan geçirir, `llm` düğümleri
  `executeTurn` ile sohbet turunun tüm yeteneklerini korur. Tur başına `FlowRun` kaydı,
  `flow_node` SSE çerçevesi ve transkripte `flow_node` adım kartı; önemsiz akışta kart yok.
- Sürümleme (`agent-flow-versions/`), yalnız yerleşim değişikliğinde sürüm açılmaması,
  büyüme bütçesi, geri dönüş; ajan araçları `get_flow` / `edit_flow` / `revert_flow` /
  `list_flow_runs` / `update_my_prompt`; prompt sürümleri (`agent-prompt-versions/`).
- `flow-optimizer` sistem ajanı (Flow Observer): `off` / `propose` / `auto` politikası,
  her N koşuda kanıt toplayıp STRICT JSON öneri üretir; op'lar kodda doğrulanır, öneri
  `FlowProposal` olarak dosyalanır, `auto` + güven eşiğinde kendiliğinden uygulanır.
- REST `/api/flows*`, `/api/flow-runs/{id}`, `/api/flow-proposals/{id}/apply|reject`,
  `/api/flows/{id}/test`, `/api/agents/{id}/prompt-versions*`. Akışlar ekranı yeniden
  yazıldı: React Flow kanvas + inceleyici, Koşular / Evrim / Test sekmeleri, dar / kare /
  dikey ekranda çekmece liste + alt sayfa inceleyici + yığılmış koşu ayrıntısı.
- Bu makinede ortam kaynaklı, değişiklikten bağımsız test hataları: `TestDeleteMarksAnd
  ClearsRemovalOnTheProductionPath` (workspace), `TestCreateDerivedArtifactFailureRemoves
  Staging`, `TestCodexHomeDirMatchesAgentResolution`, `TestDashboardCommitActivity*`
  (git/Xcode lisansı), `TestIsEphemeralWorkdir` (Windows yolu) — HEAD'de de kırmızı;
  vitest Türkçe locale ile yeşil (`LC_ALL=tr_TR.UTF-8`).

## Doküman ve tarihçe düzeni (2026-10-03)

- 583 eski ilerleme bölümü korunarak en yeni tarih üstte sıralandı; aynı günün
  kayıtları önceki sırasını korur. Dokuz yakın dönem kayıt ana dosyada,
  271 Temmuz, 144 Ağustos ve 159 Eylül kaydı tarihli arşivlerdedir.
- Tamamlanan yol haritası ayrıntıları tarihsel eke taşındı; 27 açık/yarım madde
  ana yol haritasında korunur. Eski Harita odak planı ve kişisel araç bakım raporu
  arşivlendi; Go hata yolu uyumluluğu teknoloji rehberine ayrıldı.
- Frontend README, çalıştırma/doküman becerileri, doküman indeksi ve on karar
  mercii anlatımı güncellendi. Tarihsel araçlar yeni arşiv yollarından bulunur.
- Bölüm içerikleri için kaynak/taşınmış SHA-256, eksik/çift bölüm, tarih sırası
  ve aynı-tarih sırası kontrolü geçti. Genel temizliğin kapsamı ve son doğrulaması
  [temizlik raporundadır](TEMIZLIK-2026-10-03.md).

## SES36: koordinasyon doğruluğu ve context maliyeti (2026-10-01)

- CLI araç köprüsü worker'ın tur sonu rapor bağlamını koruyor. Erken
  `report_to_coordinator` üst koordinatörü uyandırmıyor; başarısız terminal sonuç
  erken PASS iddiasını geçersiz kılıyor. Çalışan worker için dış bağlamdan rapor
  gönderimi reddediliyor. Otomatik alt-koordinatör raporu da tur sonunda iletiliyor.
- Worker yanıtının veya terminal durumunun kalıcı kaydı başarısızsa tamamlandı
  bildirimi üretilmiyor. Hiç araç izi olmayan `VERDICT: PASS`, eksik doğrulama olarak
  işaretleniyor; normal metin analizi tamamlanabiliyor.
- Worker bildirimi metni XML için kaçışlanıyor; sonuç gövdesi durum alanını
  değiştiremiyor. Arayüz runtime sonucu ile öz bildirimi ayırıyor, test kanıtının
  ayrıca gerektiğini açıklıyor ve aynı worker'ın sonraki raporuna işaret ediyor.
  Eski SES51 başarı kartında sonraki başarısızlık görünür; geçmiş silinmedi.
- Otomatik turlar normal sohbetin CLI devam planını kullanıyor. Devam kimliği ve
  mesaj sınırı yanıtla aynı kalıcı işlemde saklanıyor. Tur sırasında eklenen worker
  mesajları sayacı ileri atlatmıyor; sonraki turda mutlaka gönderiliyor.
- Geçici Codex çağrıları da rollout üretiyor; model çağrısı sayısı ve kullanım
  ölçümden sonra okunuyor, ardından geçici home siliniyor. Eski, rollout'u
  silinmiş çağrıların iç döngü sayısı tahminle onarılmadı.
- Compaction hedefi ve otomatik tur sağlayıcısı uygulamanın CLI home'una
  bağlanıyor. Ambient kullanıcı config'ine düşüş önleniyor; kullanıcıya ait
  global config değiştirilmedi.
- Sohbet ve otomatik turlar beceri defterini paylaşıyor. Fold sonrası güncel
  epoch aynı turda uygulanıyor. Tekrarlanan zorunlu yüklemede gerekçe yoksa
  mevcut metne kısa işaretçi dönüyor; fold sonrası gerçek yeniden yükleme açık.
- Varsayılan, sabitlenmemiş sistem sağlayıcısı kimlik doğrulamayı reddederse
  yardımcı çağrı çalışan çağıran sağlayıcısına dönebiliyor. Reddedilen sistem
  sağlayıcısı yapılandırma nesli değişene kadar tekrar seçilmiyor. Kullanıcının
  sabitlediği sağlayıcı seçimi korunuyor; CLI auth hataları doğru sınıflanıyor.
- Context paneli sıradaki isteğin tahmini büyüklüğü ile tüm oturum boyunca
  toplanan token kullanımının farklı olduğunu açıklıyor. Koordinatör promptu
  tekrarlanan tam belge/skill okumaları yerine ilgili bölümleri istemeye yönlendiriyor.
- Tarayıcıda SES36/SES51 eski bildirim uyarıları, sonuç aç/kapa, worker oturumuna
  geçiş ve gerçek timeout kaydı doğrulandı. Chrome mesaj kanalı hatalarının
  uygulama mı eklenti mi olduğu belirlenmedi; konsolun tamamen temiz olduğu iddia
  edilmiyor. Odaklı backend regresyonları ve bildirim arayüz testleri geçti.
- Son doğrulama: arayüz derlemesi başarılı; `scripts/test.sh full` bütün Go
  paketlerinde ve 168 dosyadaki 1.189 arayüz testinde geçti. Bağımlılık yönü ve
  `git diff --check` temiz. Derleme sırasında dosya silme ile Go embed taraması
  çakıştığı ilk koşu başarısız oldu; arayüz derlemesi ve tam geçit sırayla
  yeniden çalıştırılarak bu hata giderildi.
- Çalışan WS30 oturumları kesilmedi. Backend güncellemeleri başarıyla derlenen
  `bin/tionharness-ses36-fixed.exe` dosyasında hazır; etkinleşmeleri için backend'in yeniden başlatılması
  gerekir. Arayüz değişiklikleri geliştirme ekranında doğrulandı.
- Değişen arayüz bileşenlerinin ESLint kontrolü de geçti. Commit oluşturulmadı.
## Codex oturumları: sayaçlar, dosya araçları ve belge güncellemeleri (2026-09-30)

- Devam ettirilen Codex oturumlarında thread boyunca biriken kullanım her turda
  yeniden toplanmıyor. Yalnız denemenin rollout dosyasına eklediği model çağrıları
  sayılıyor; çağrı sayısı ve ilk çağrının girdi ölçümü de buradan alınıyor.
- CLI köprüsündeki dosya araçları oturumun çalışma dizinine bağlanıyor;
  dosya değişiklik koruması ve otonom çalışma dizini sınırları korunuyor.
- Uzun Bash komutları geçici betik dosyasından çalışıyor; Windows Python çıktısı
  UTF-8 kullanıyor. Toplam sınır içindeki JSON araç çıktıları satır sınırından
  dolayı bozulmuyor.
- `update_artifact`, `sourcePath` ile sohbet ve otonom görevlerde dosyadan
  güncelleyebiliyor. Metin güncellemesi diskteki gövdeyi de yeniliyor; dışarıdan
  içe aktarılan dosyalar benzersiz isimlerle birbirini ezmiyor.
- MCP ilk açılışta gecikirse araçlar kaldırılmadan bir kez daha deneniyor.
  Çalışmaya başlamış bir araç varsa yan etkileri tekrarlayan retry yapılmıyor.
- SES4'ün eski sayaçları yedekli ve kilitli onarımla 4.938.096 token / 48 model
  çağrısına düzeltildi; üç belge ekinin gösterilen eski kopyası son kaydedilmiş
  içerikle yenilendi. İkinci çalıştırma değişiklik yapmıyor.
- Onarım aracı ve testleri: `scripts/codex_usage/README.md`. Regresyonlar;
  devam ettirilen kullanım, eksik rollout, MCP retry, uzun Unicode heredoc,
  Python kodlaması, çalışma dizini, belge sahipliği ve tekrar onarımı kapsıyor.
- Doğrulama: `scripts/test.sh full` temiz; tüm Go testleri, 168 dosyada 1.186
  arayüz testi, bağımlılık yönleri ve diff kontrolü geçti. Altı Python onarım
  testi ayrıca geçti. Yeni derleme çalıştırıldı; sağlık, kullanım ve zvec MCP
  bağlantısı canlı uygulamada doğrulandı.

## CLI oturumlarında kuyruk yerine canlı yönlendirme (2026-09-28)

- **Yönlendir** ve kuyruktaki **Şimdi yönlendir**, Interaction MCP bağlı Claude ve
  Codex CLI turlarında tüm izin modlarında açıldı. Native teslim yolu korundu.
- Genel MCP araç yanıtları kullanıcı yönlendirmesini ayrı içerik blokları olarak
  taşır; soru cevabı aynı anda gelebilir. Claude izin yanıtlarının JSON biçimi korunur.
- Tek mesajlık CLI tamponu ortak FIFO ile değiştirildi. Mesajlar birbirini silmez;
  tur sonu yarışı, dolu tampon, durmuş tura gönderim ve kuyruktan dönüşüm korunur.
- Yanlış `ask` moduna geçme önerileri kaldırıldı. Ret halinde taslak/queued mesaj
  korunur. Kullanıcı durdurduğunda bekleyen yönlendirme oturumu yeniden başlatmaz.
- Teslim sonraki model/TionHarness araç sınırındadır; uzun CLI çağrısını anında
  kesmez. Güncel sözleşme ve sınırlar: [59-CLI-STEER-PLANI.md](59-CLI-STEER-PLANI.md).
- Doğrulama: tüm Go paketleri, 1044 frontend testi, TypeScript, değişen yönlendirme
  dosyalarında ESLint ve depcheck geçti. Canlı CLI/model deneyi yapılmadı.

## Akışı durdurmadan kullanıcıya soru sorma (2026-09-28)

- Resmi OpenAI/Codex ve Claude belgeleri incelendi; protokol düzeyinde asenkron araç
  çalıştırma ile soru/arka plan ajanı akışları ayrıştırıldı. Kaynaklar ve sınırlar:
  [89-ASENKRON-KULLANICI-SORULARI.md](89-ASENKRON-KULLANICI-SORULARI.md).
- `ask_user_async` soruyu kalıcı kaydedip hemen döner. Native döngü cevapları sonraki
  model isteğine, CLI köprüsü sonraki MCP aracının ayrı içerik bloğuna taşır.
  Tur bittiyse cevap mevcut seri oturum kuyruğundan devam eder.
- Birden fazla soru kartı kimlikle yönetilir; normal tur sonu kartları kapatmaz,
  ağ hatasında cevap taslağı kaybolmaz. İzin/plan ve `ask_user` bekleme davranışı korunur.
- Cevap/tur-sonu yarışı, tek tüketim, yanlış oturum, çift cevap, durdurma ve
  native/CLI devamı için regresyon testleri eklendi.
- Tam test kapısı geçti: Go paketleri, 147 dosyada 1042 frontend testi, depcheck
  ve diff kontrolü. Tarayıcıda gerçek bileşenlerle iki kart, taslak koruma ve dar
  ekran doğrulandı. Ek ESLint kontrolünde `ChatView.tsx` içindeki mevcut
  `currentTodo` memoization hatası kaldı; yeni soru akışından bağımsızdır.

## Ayrıntılı Codex araç karşılaştırması (2026-09-28)

- Önceki deneyden ayrı 12 canlı çağrı: 48 araç olayı, sıfır araç hatası; 96 test
  kontrolü geçti. Astra TionHarness/CLI 12/12, Sol 11/13 adım kullandı.
- Komut/girdi/çıktı, ölçülebilen süreler ve nihai çözüm kaydedildi; birleşik
  keşif+okuma çağrıları ile cache temizliği ayrı incelendi. Yama girdisi/süresi
  eksikse ölçüm varmış gibi sunulmaz.
- Astra'nın bağımlılık doğrulaması iki yolda da 512 grafiği kapsadı; Sol seçili
  örnekleri sınadı. Dört nihai çözüm sonradan bağımsız 512'şer grafik kontrolünü
  geçti (2048 ek kontrol). Bu ek kontrol model sayaçlarına dahil değil.
- Rapor: `69-CODEX-TOOL-BENCHMARK-2026-09-28.md`; ham izler aynı adlı JSON.
  `scripts/codexbench` iz ayrıştırma testleri, etiketleme ve ek doğrulama araçları
  içerir. Üretim sağlayıcı davranışı değiştirilmedi.
- Tam test kapısı geçti: Go testleri, 143 dosyada 1026 frontend testi, depcheck
  ve diff kontrolü. İz parser testleri canlı istek gerektirmez.

## TionHarness–Codex CLI karşılaştırmalı deney (2026-09-28)

- CLI 0.157.1 ve `high` seviyesinde Astra/Sol × iki taşıyıcı × üç Python görevi
  çalıştırıldı. 12 deneme ve toplam 96 değerlendirici kontrolü geçti.
- Ortalama süre (TionHarness / doğrudan CLI): Astra 49,07 / 47,70 sn;
  Sol 43,79 / 40,22 sn. Her hücre yalnız üç görevdir; genelleme yapılmaz.
- Tekrar üretilebilir araç `scripts/codexbench`, rapor ve ham JSON
  `69-CODEX-BENCHMARK-2026-09-28` dosyalarında. Desktop ve tam uygulama
  akışları ölçülmedi; araç gerçek hesap kotası kullanır.
- Doğrulama: tam Go testleri, 143 dosyada 1026 frontend testi, depcheck ve
  diff kontrolü geçti. İlk kapının bulduğu Windows konsol politikası ihlali,
  benchmark başlatıcısında ortak `proc` yardımcıları kullanılarak giderildi.

## Codex CLI güncellemesi ve Sol canlı doğrulaması (2026-09-28)

- CLI 0.157.1 kuruldu; yerel Codex sağlayıcısının `cliPath` ayarı yeni dosyaya
  yönlendirildi. Giriş ve diğer sağlayıcı ayarları korundu.
- Aynı TionHarness hesabıyla Astra ve Sol canlı testleri geçti. Aşağıdaki
  0.153.3 testinde görülen Sol erişim reddi yeni sürümde tekrarlanmadı.
- Bu kısa yanıt testi model erişimini doğrular; performans benchmark'ı değildir.

## Codex GPT-6 aile desteği ve Desktop farkları (2026-09-28)

- Sol 6 ve Luna 6 Codex model kataloğuna ve API eşdeğeri fiyat tablosuna eklendi;
  Astra 6 açıklaması güncellendi. Önceki model seçimleri korunur.
- Codex 0.153.3 model kataloğuna göre GPT-6 CLI bağlamı 272K, Astra/Sol düşünmesi
  low–ultra, Luna low–max. API'nin 1.05M kapasitesi CLI'ya mal edilmez; CLI'da
  sunulmayan `off` ve Luna `ultra` katalog/API doğrulamasından çıkarıldı.
- Gerçek TionHarness taşıyıcısıyla Astra kısa yanıt verdi. Sol, test edilen
  ChatGPT girişinde model erişimi nedeniyle reddedildi; katalogda görünmesi
  erişim garantisi değildir. Yeniden girişten sonra uygulamanın kendi
  `~/.tionharness/codex-home` hesabıyla da aynı sonuç doğrulandı (Astra başarılı,
  Sol 400 model erişim reddi). Luna için canlı istek yapılmadı. Benchmark yok.
- Ayrıntılar, kaynaklar, Desktop farkları ve isteğe bağlı canlı test komutu:
  `69-CODEX-CLI-SAGLAYICI.md` §14.
- Doğrulama: sağlayıcı regresyonları ve backend derlemesi başarılı; tam kapının
  ikinci koşusu geçti (Go + 143 dosyada 1026 frontend testi + depcheck + diff).
  İlk tam koşuda mevcut `TestSpawnWorkerIsCancellableAsSoonAsItReturns` düştü;
  tek başına 20 tekrarda ve ikinci tam koşuda geçti. Bu test/kod değiştirilmedi;
  ilk hata zamanlamaya bağlı olabilir. İsteğe bağlı canlı testte Sol erişim reddi
  ayrı bir başarısız sonuçtur, çevrimdışı kapının geçmesi onu ortadan kaldırmaz.

## Doküman doğrulama ve kullanıcı kılavuzları (2026-09-28) ✅

- README, mimari, veri modeli, teknoloji seçimleri ve ajan referansı kaynak koduyla
  karşılaştırıldı: 12 sağlayıcı, 10 renk ailesi, güncel frontend çıktı yolu, dosya
  göçleri, WebSocket monitor bağımlılığı ve bölünmüş store dosyaları düzeltildi.
- HTTP auth varsayılanı, opt-in bearer kapısı ve UI kısıtı açıklandı; etkisiz
  `ACCESS_KEY`, tüm JSON'un şifreli olduğu iddiası ve workspace'in dosya sistemi
  sınırı olduğu anlatımı düzeltildi. Arşivlerdeki tarihli kayıtlar korunur.
- Web sitesindeki yedi yer tutucu kılavuz Türkçe kurulum, ilk kullanım, ajan,
  oturum, workspace ve yapılandırma rehberleriyle değiştirildi. Site README'si,
  feed/placeholder açıklamaları ve derleme ön koşulları güncellendi.
- Doğrulama: `scripts/test.sh full` başarılı (Go testleri + 143 dosyada 1026
  frontend testi + depcheck + yama kontrolü); frontend build, site check ve site
  build başarılı. 95 dokümanda 290 yerel bağlantı hedefi mevcut; yedi kılavuzun
  üretilen HTML çıktısı kontrol edildi. Harici bağlantılar ve görsel UI akışları
  bu kontrolde doğrulanmadı. Frontend büyük parça boyutu uyarısı hâlâ mevcut.
