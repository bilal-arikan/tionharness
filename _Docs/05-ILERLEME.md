# TionHarness — İlerleme Takibi

> **Özet (2026-10-04):** 2026-09-28 ve sonrası için yakın dönem değişiklik günlüğüdür. En yeni tarih üstte, aynı günün kayıtları önceki sırasındadır. Önceki ayların kayıtları aşağıdaki arşivlere taşınmıştır; konu sözleşmeleri ilgili rehberlerde, açık işler [yol haritasında](03-YOL-HARITASI.md) tutulur.

Önceki kayıtlar:

- [Temmuz 2026](arsiv/05-ILERLEME-2026-07.md)
- [Ağustos 2026](arsiv/05-ILERLEME-2026-08.md)
- [Eylül 2026, 27 Eylül ve öncesi](arsiv/05-ILERLEME-2026-09.md)
- [Haziran 2026 ve öncesi](05-ARSIV.md)

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
