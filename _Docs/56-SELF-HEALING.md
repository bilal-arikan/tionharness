# 56 — Kendi Kendini Onaran Oturum Akışları (Self-Healing)

> Durum: **Tamamlandı** (2026-07-07). İlham: external-context-agent projesinin
> `error_classifier` / `tool_guardrails` / `message_sanitization` / stuck-loop
> desenleri, TionSwarm'ın mevcut saf-`decideRecovery` mimarisine uyarlandı.
> Kapsam: **native tool-use döngüsü** (anthropic/minimax). claude-cli kendi
> döngüsünü sürer — Faz A/B/C ona uygulanmaz; Faz D (oturum-seviyesi) her iki
> yol için geçerlidir.

## Bileşenler

### Faz A — Provider hata sınıflandırıcı + sınırlı retry
- `internal/agent/errclass.go`: `errClass` taksonomisi — `rate_limit`,
  `overloaded`, `server_error`, `timeout` (retry-edilebilir); `context_overflow`
  (compact yolu); `auth`, `billing`, `cancelled`, `unknown` (terminal).
  `classifyProviderError` muhafazakâr string/sentinel eşleme yapar; emin
  olunamayan her hata `unknown` = terminal (asla körlemesine retry edilmez).
- `decideRecovery` yeni `contProviderRetry` continuation'ı döner: aynı istek,
  jitter'lı üstel backoff (1s→30s cap, ±%25), `sleepCtx` ile iptal-duyarlı.
- Bütçe: `loopState.providerRetries` < `maxProviderRetries`
  (ayar `maxProviderRetries`, varsayılan 2, clamp 0..5, 0 = kapalı).
- İptal (`context.Canceled`) artık `termCancelled` ile biter (önceden
  `provider_error` görünüyordu).
- **Terminal hit-limit mesajı** (2026-08-04): retry bütçesi tükenip tur
  `provider_error` ile bitince, hata `rate_limit`/`overloaded`/`billing`
  sınıfındaysa `fail()` (toolloop.go) ham "HTTP 429…" metnini net Türkçe bir
  açıklamayla değiştirir (`limitErrorText`) ve error step'in `Reason`'ını
  spesifik sınıf tag'ine (`rate_limit`/`overloaded`/`billing`) çeker — ham detay
  açıklamanın altında kalır. Sınıflandırma muhafazakâr olduğundan guardrail/
  max-iters hataları etkilenmez. Frontend `ErrorStep` bu tag'leri tanıyıp
  belirgin başlık + "Yeniden dene ile sürdür" ipucu gösterir; salt-okunur
  oturumlarda (görev/akış/zamanlama günlüğü) retry **yıkıcı olmayan** varyantı
  kullanır (`retryMessagePreserve` — transkripti silmeden tetikleyici prompt'u
  yeniden kuyruğa alır; backend enqueue her oturum türünü kabul eder).

### Faz B — Tool-loop guardrail (döngü tespiti)
- `internal/agent/toolguard.go`: yan-etkisiz saf kontrolcü (`check`/`observe`),
  tur başına `completeTracedInner`'da kurulur. Üç sayaç:
  | Sayaç | Uyarı | Blok/Halt |
  |---|---|---|
  | Exact-failure (aynı tool + aynı arg-hash hatası) | 2 | 5 (blok) |
  | Same-tool-failure (aynı tool ardışık hata) | 3 | 8 (tur halt) |
  | No-progress (idempotent aynı çağrı başarılı tekrar) | 2 | 5 (blok) |
- İdempotentlik hardcoded liste değil: `tools.Classify(name) == RiskRead`
  (MCP araçları otomatik kapsanır).
- **Uyarılar** (`toolGuardWarnings`, vars. açık): eyleme dönük İngilizce hint
  başarısız tool_result'ın SONUNA eklenir ("diagnose before retrying, try a
  different tool, don't fall back to text-only").
- **Hard stop** (`toolGuardHardStop`, vars. KAPALI): blok → tool çalışmadan
  sentetik `IsError` result (`StepError(reason:guardrail_block)`); halt →
  batch'in sonuçları yazıldıktan sonra kontrollü tur sonu
  (`StepRecovery(reason:guardrail_halt)`, hata DEĞİL).
- Hook/izin redleri sayaçlara girmez (hermes paritesi: policy denial ≠ tool
  failure).

### Faz C — Mesaj dizisi onarımı
- `internal/conversation/repair.go` `RepairSequence(msgs)`: her provider çağrısı
  öncesi (toolloop iterasyon başı) tur-içi geçmişte tool_use↔tool_result
  eşleşme değişmezlerini zorlar; ihlaller provider 400'ü olarak turu öldürmek
  yerine onarılır:
  1. duplicate tool_result → ilki kalır;
  2. sahipsiz tool_result → düşer (mesajda başka şey yoksa mesaj düşer);
  3. cevapsız assistant tool_use → RawContent yoksa komşu assistant batch ile
     birleştirilir, yoksa sentetik "cancelled" tool_result üretilir;
  4. kısmi batch (bazı sonuçlar kayıp) → eksikler sentetik doldurulur.
- Saf + idempotent; sağlam dizi aynı backing array ile döner (bedava).
  **Sessiz düzeltme yok**: her onarım log + `debug.jsonl`'e `repair` olayı.
- Not: kalıcı geçmiş (`session.jsonl` → `toProviderMessages`) yalnız Role+Text
  taşır; tool blokları yalnız tur-içi yaşar — onarımın doğru yeri bu yüzden
  toolloop'tur, `Prepare` değil.

### Faz D — Kalıcı stuck sayacı + otonom askıya alma
- `db.Session.StuckTurns`: ardışık kötü turlar (tur hatası VEYA guardrail
  halt); temiz tur sıfırlar. Restart'a dayanıklı (session JSON'unda).
- Eşik (`stuckTurnThreshold`, vars. 3, clamp 0..20, 0 = kapalı) aşılınca:
  - otomatik **`stuck` etiketi** (`agent.TagStuck`, autotag add-only) →
    etiket-otomasyonla onarım ajanı bağlanabilir (46-ETIKET-OTOMASYON);
  - `completeTracedInner` başındaki `stuckGate` sonraki **otonom** turları
    reddeder (manuel chat hiç kapılanmaz — kurtarma yolu tam da odur).
- Gate'in kendi reddi sayacı BÜYÜTMEZ (`stuckGuardMarker` dışlaması).
- `RemoveSessionTags` ile `stuck` etiketi kaldırılınca sayaç da sıfırlanır
  (fixer çözdü → otonomi geri açılır).

### Faz E — Etkinlik gözcüsü + kesinti kurtarma (idle watchdog)
- `internal/agent/activity_timeout.go`: her arka-plan turu HEM mutlak süre tavanı
  (`SpawnTimeout`) HEM boşta penceresi (`SpawnIdleTimeout`) ile sınırlıdır; step
  emitter her adımda `touch()` ile boşta sayacını sıfırlar.
- **Heartbeat** (`startActivityHeartbeat`, interval = idle/2): streaming OLMAYAN
  provider tamamlaması ve tek-seferlik araç çağrısı (büyük `write_file`, uzun
  shell, alt-ajan beklemesi) adım üretmez — bunlar sırasında gözcü periyodik
  touch'lanır, böylece meşru uzun-ama-sessiz iş "asılı" sanılıp kesilmez. Streaming
  yollar (token/düşünce/tool_delta) zaten her parçada touch ettiğinden heartbeat
  ALMAZ; böylece takılan bir akış hâlâ idle ile geri alınır. Wedge olan tek işlem
  yine sert tavanla sınırlıdır.
- **Kesinti kurtarma**: `classifyTurnOutcome` iptal nedenini (`ErrTurnIdleTimeout`/
  `ErrTurnHardTimeout`) + loop terminal işaretlerini okuyup "iş BİTMİŞ DEĞİL" notu
  üretir. `reconcileTurnOutcome` bu notu kurtarılan kısmi metnin başına ekler, iz'e
  işaret bırakır ve turu temiz kapatır; kesilmiş tur `FireTurnFinished`/başarı
  sinyali tetiklemez (fragmanla otomasyon zincirlenmez). Spawn/worker'a ek olarak
  wake/schedule/inbox **ve koordinatör** turları da bu kurtarmayı kullanır;
  koordinatörde kesilmiş tur ayrıca spawn-narration stall kontrolünü atlar (yarıda
  kesilen tur bu heuristikle yargılanmaz).
- **Tek-atımlık boşta-resume** (`runTurnWithIdleResume`, `idleResumeMax=1`,
  FND-708844f8): boşta gözcüsü döngü DIŞINDA tetiklendiğinden döngü-içi kurtarma
  bütçelerine (`decideRecovery`) hiç girmez; bu yüzden bir kez sessizleşen tur (uzun
  streaming-olmayan araç çağrısı, alt-ajan beklemesi) hiçbir bütçeye girmeden yarım
  kalırdı. Artık tur **yalnız `ErrTurnIdleTimeout`** ile kesildiyse taze bir boşta
  penceresiyle **BİR KEZ** otomatik yeniden başlatılır; ikinci deneme, ilk denemenin
  kurtarılan fragmanını `resumeContinuationPrompt` ile alır (baştan başlamaz,
  kaldığı yerden sürer). **Sert tavan (`ErrTurnHardTimeout`) resume EDİLMEZ** —
  tekrar aynı tavana çarpar. İkinci deneme de boştaysa tur yine "timeout"/unfinished
  olarak raporlanır (Faz E) ve üst koordinatör onu yeniden görevlendirir (ÇİFT
  kurtarma değil: resume, koordinatör re-task'ından ÖNCEKİ yerinde ilk savunma).
  Worker yolunda `workerCtl` her denemede `setCancel` ile güncellenir, böylece
  koordinatörün `stop_worker`'ı daima uçuştaki denemeyi keser. Bağlı yollar:
  spawn / worker / inbox / wake / schedule (koordinatör turu hariç).

## Gözlemlenebilirlik
- Yeni `debug.jsonl` olay türleri: `repair` (rule `Name`'de) ve `guardrail`
  (`warn`/`block`/`halt`/`stuck_gate` `Name`'de, tool/detay `Detail`'de).
- Yeni adım reason'ları: `provider_retry` (StepRecovery), `guardrail_block`/
  `guardrail_halt`. UI mevcut StepRecovery/StepError kartlarıyla gösterir.

### Faz F — Hata→ders döngüsü (lesson reflect, 2026-07-07)
external-context-agent `background_review`'un TionSwarm uyarlaması (memory alt sistemi
kaldırıldığı için hedef store dar-kapsamlı yeni bir sidecar):

- **Reflector** `internal/agent/lessons.go`: kötü biten her tur
  (`AutoTagTurn` hunisi üzerinden, kendi gate'iyle) arka-plan goroutine'de
  ucuz bir model çağrısı yapar (başlık modeli varsa o, yoksa ajanın modeli;
  `KindReflect` olarak faturalanır, `guardedComplete` bütçe/pause guard'ından
  geçer). Kanıt: en fazla 3 gerçek tool hatası (policy denial + guardrail
  coaching metni dışlanır) + tur-seviyesi hata. Model tek, genellenebilir bir
  ders döner (`NONE` → kaydedilmez). Kullanıcı iptali ve stuck-gate reddi
  yansıtılmaz.
- **Store** `internal/db/store_lessons.go`: workspace-geneli `lessons.jsonl`
  (store kökünde, kendi mutex'i). **Signature dedupe**: aynı hata şekli
  (tool + normalize hata digest'i) tekrarında mevcut ders güncellenir
  (`Count++`, yeni metin kazanır) — yığılma yok. Cap 200 (en yeniler).
  `AddLesson`/`ListLessons`/`DeleteLesson`.
- **Enjeksiyon** `Runtime.LessonsContextBlock`: en yeni 5 ders "Lessons from
  past failures" bloğu olarak **hem** chat (`composeTurnRequest` dynamic)
  **hem** headless (`autonomousDynamicSuffix`) turlara girer — volatile suffix,
  cache'li prefix'e dokunmaz.
- Debug olayı: `lesson` (tool `Name`'de, ders özeti `Detail`'de).

## Ayarlar (settings.json → Tunables → Ayarlar UI "Bağlam" sekmesi)
| Ayar | Varsayılan | Etki |
|---|---|---|
| `maxProviderRetries` | 2 (0..5) | geçici provider hatası retry bütçesi |
| `toolGuardWarnings` | true | hint ekleme |
| `toolGuardHardStop` | false | blok/halt devre kesici |
| `stuckTurnThreshold` | 3 (0..20) | stuck etiketi + otonom gate eşiği |
| `lessonReflect` | true | hata→ders döngüsü (kötü tur başına 1 ucuz çağrı) |
| `guardExactWarn` / `guardExactBlock` | 2 / 5 | birebir aynı başarısız çağrı: uyarı / blok eşiği |
| `guardSameToolWarn` / `guardSameToolHalt` | 3 / 8 | aynı araç ardışık hata: uyarı / tur-durdurma eşiği |
| `guardNoProgressWarn` / `guardNoProgressBlock` | 2 / 5 | idempotent aynı çağrı tekrarı: uyarı / blok eşiği |

Eşiklerde 0 = yerleşik varsayılan (clamp 0..50); blok/halt yalnız devre kesici
(`toolGuardHardStop`) açıkken etkilidir.

## Lessons API + UI
- `GET /api/lessons` (workspace-scoped, tümü, en yeni önce) ·
  `DELETE /api/lessons/{id}` (stale ders budama). Yazma yolu YOK — dersleri
  yalnız reflector üretir.
- Ayarlar → Bağlam → Self-healing bölümünde `LessonsList` bileşeni: kayıtlı
  dersler (araç rozeti, görülme sayısı, tarih) + satır-başı silme + yenile.
  Silinen ders bir daha enjekte edilmez (aynı hata tekrar ederse yeniden doğar).

Frontend: `ContextPanel.tsx` "Self-healing (döngü koruması & ders çıkarma)"
bölümü (guardrail toggle'ları + stuck eşiği + lesson toggle) ve "Tur kurtarma"
grid'inde sağlayıcı retry bütçesi alanı. Not: `SettingsPanel.saveApp` patch'ine
eksik olan handoff/progress/autoTag/debugJournal alanları da eklendi (bu
toggle'lar daha önce kaydedilmiyordu — düzeltildi).

## Testler
- `errclass_test.go` (sınıflandırma/retryable/backoff/sleepCtx + provider-retry
  karar tabloları), `toolguard_test.go` (uyar/blok/halt/reset/no-progress),
  `conversation/repair_test.go` (9 onarım senaryosu + idempotentlik),
  `stuck_test.go` (sayaç/etiket/gate/untag-reset),
  `store_lessons_test.go` (round-trip/dedupe/sıralama/cap/delete),
  `lessons_test.go` (kanıt toplama/signature/gate'ler/context bloğu).

## Devam turu (2026-07-08): 8 adım + canlı E2E

- **Ajan araçları:** `read_lessons` (RiskRead) / `delete_lesson` — tam ders seti +
  id'lerle budama; `lessonReflect` gate'iyle kurulur.
- **Yaşlandırma:** `db.DefaultLessonMaxAge` (varsayılan **2 gün**; `Settings.lessonMaxAgeDays`
  ile ayarlanabilir, İçgörü ▸ Öz-iyileşme'den) — tekrar etmeyen ders sonraki
  AddLesson rewrite'ında düşer. **Signature normalizasyonu:** yol → `<path>`,
  sayı dizileri → `#` (aynı hata şekli farklı dosya/satırda tek derse deduplanır).
- **Ajan-öncelikli enjeksiyon:** `LessonsContextBlock(ctx, agentID)` — ajanın
  kendi dersleri önce, sonra workspace havuzu (overfetch ×10, cap 5).
- **Retry-After:** provider hata metnine ` (retry-after: Ns)` eki
  (`RetryAfterSuffix`/`ParseRetryAfterHint`); `decideRecovery` sunucu hint'ini
  hesaplanan backoff yerine kullanır (60s cap).
- **CLI görünürlük paritesi:** `analyzeCLIGuardrail` — CLI trace'i tur sonunda
  aynı sayaçlardan geçirilir, eşik aşan araç başına `guardrail/cli_warn` olayı
  (analiz, müdahale yok).
- **Stuck→onarım otomasyonu:** hatalı turlar artık ayrı `failedTurnHooks` ile
  YALNIZ automation engine'e dispatch edilir (`FireTurnFailed`, AutoTagTurn
  hunisinden; koordinasyon bilinçli hariç) — stuck etiketli oturumun BAŞARISIZ
  turu otomasyonu tetikler. `stuck` da clear-parent-tags setine girdi (fixer
  başarısı etiket+sayaç sıfırlar). Automations ekranında tek-tık **"🩹 Stuck
  oturum onarıcısı"** şablonu (spawnTags=[], döngüsüz).
- **Debug viz:** `SelfHealingEvents` — recovery/repair/guardrail/lesson
  olayları tür rozetli liste olarak İş Akışı Görselleştirmeleri altında.
- **Dosya-mutasyon verifier'ı** (`internal/tools/verifymutation.go`, hermes
  `_record_file_mutation_result` paritesi): `Write`/`Edit`/`apply_patch`
  başarılı yazım sonrası diski geri okuyup içeriği doğrular (≤1MB bayt-bayt,
  üstü SHA-256). "Yazıldı" denilen ama AV/eşzamanlı yazar/dolu disk yüzünden
  yere inmeyen mutasyon artık hatalı tool_result olur → guardrail sayaçları,
  `tool-error` etiketi ve ders döngüsü normal hata gibi tepki verir.
- **Edit eşleştirme sağlamlığı** (`internal/tools/builtin_fs.go`, `computeEdit`):
  `old_string` hafızadan yeniden yazıldığında (unicode « » ✅ ⏳, boşluk/hizalama
  farkı) birebir bayt eşleşmezse Edit sessizce düşmesin diye kademeli eşleştirme:
  (1) birebir → (2) cat -n satır-no ön eki / CRLF hizalama normalizasyonu →
  (3) **boşluğa toleranslı, satır-tabanlı fallback** (önce satır-sonu boşluk, sonra
  girinti; yalnız **tekil** konumda ateşler, `replace_all` yoksa çoklu adayda hata).
  Hiç eşleşme yoksa artık **en yakın dosya satırını ve ilk farklılaşan sütunu** gösteren
  tanılayıcı hata döner ("kısa BENZERSIZ ASCII parça hedefle" yönlendirmesiyle) — no-op
  yapmaz. Testler `builtin_edit_match_test.go`.
- **apply_patch eşleştirme sağlamlığı** (`internal/tools/builtin_patch.go`,
  `applyHunks`): hunk bağlamı birebir bulunamazsa aynı kademeli boşluk toleransı
  (`blockMatchesNorm`: satir-sonu boşluk → girinti) devreye girer, **yalnız cursor'dan
  sonra tekil konumda** kabul edilir (çoklu adayda hata). Hiç eşleşme yoksa
  `diagnoseHunkMismatch` en yakın satırı + ilk farklılaşan sütunu gösterir; dosya asla
  yarı-uygulanmaz. Testler `builtin_patch_match_test.go`.

### Canlı E2E doğrulaması (izole instance, gerçek claude-cli/fable-5)
Doğrulanan zincir: başarısız Read×3 turu → `tool-error` auto-tag ✅ →
`cli_warn` guardrail olayı ✅ → anomaly "Read 10/10 başarısız" ✅ → ölü
provider'lı oturumda 3 tur hata → `error`+`stuck` tag, `stuckTurns=3` ✅ →
failed-turn dispatch stuck-otomasyonunu ateşledi, fixer spawn ✅ → fixer
başarısı parent'ın stuck etiketi+sayacını temizledi ✅ → EPERM turundan
GERÇEK ders damıtıldı (`lesson recorded`, normalize signature) ✅ → sonraki
turda ajan "Lessons from past failures" bloğunu kelimesi kelimesine gördü ✅.

E2E'nin yakaladığı ve düzeltilen 4 bug: (1) non-stream `/api/chat`
AutoTagTurn/FireTurnFinished çağırmıyordu; (2) CLI yardımcı çağrıları
(title/summary/lesson) per-workspace claude-home seam'ini atlıyordu
(`guardedComplete`'e SetConfigDir eklendi); (3) `Automation.SpawnTags`
`omitempty` yüzünden kasıtlı `[]` persist'te kayboluyordu (fixer'lar kendini
döngülüyordu); (4) reflector "NONE\n…reconsider" cevabında NONE artefaktı
derse sızıyordu (`cleanLessonText`). Ayrıca NONE kararı artık `lesson/none`
debug olayı olarak günlüklenir.

## Kapsam dışı / sıradaki adımlar
- ~~Guardrail eşiklerinin settings'e açılması~~ ✅ (2026-07-07, 6 eşik ayarı).
- ~~Lessons için UI görünürlüğü~~ ✅ (2026-07-07, API + LessonsList).
- `read_lessons`/`delete_lesson` ajan araçları (store + REST hazır; ajan
  şimdilik dersleri yalnız enjekte edilen bloktan görür).
- Ders enjeksiyonunu ajan/tool bazında filtreleme (şimdilik workspace-geneli
  en yeni 5).
