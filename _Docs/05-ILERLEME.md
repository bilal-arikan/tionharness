# SwarmGo — İlerleme Takibi

> Bu dosya canlı tutulur; her oturumda güncellenir. Son güncelleme: **2026-07-04**

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

> ⚠️ Etki için backend restart gerekir — çalışan `go run ./cmd/swarmgo` (kullanıcı
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
Agent SDK) devrediyor; SwarmGo kendi yazdığı için boşluk oluşmuş.

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
  `swarmgo-session-debug` skill'ine "Tahmin↔Gerçek farkı" ölçüm-referansı + tekrar-
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
  başlığı + chevron, kalıcı `swarmgo.coordWorkersOpen`).

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
2. **`swarmgo-project` workspace skill'i sadeleştirildi** (~194→~150 satır): "Mevcut
   Yetenekler" bölümü changelog seviyesi tarih/commit/test-ismi/env-minutiae'den
   arındırılıp "ne var + hangi `_Docs\NN`" özet haritasına indirildi (her-tur cache'li
   prefix küçüldü). Yetenek bilgisi korundu.
3. **`run_code`'a built-in araç binding'leri** (`_Docs\44`): code-execution artık yalnız
   MCP'yi değil, **SwarmGo built-in araçlarını** da `swarmgo` Python modülü olarak sunar
   (`from swarmgo import <tool>`). `codemode.WriteBindings(dir, entries, builtins, allow)`
   + `Config.Builtin` dispatcher (reserved `swarmgo__<tool>` namespace, bare-name
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
- **Gömülü default skiller güncellendi (2026-07-04, ayrı tur):** 3 skill (`swarmgo-guide`,
  `swarmgo-progress`, `swarmgo-self-management`) yeni araç isimlerine (`update_session`,
  `secret`) göre düzeltildi. **Bulgu:** `skills.EnsureDefaults` diske seed ederken
  **mevcut dosyanın üzerine yazmıyor** → önceden çalışmış kurulumlarda global skills
  dizini (`~/.swarmgo/skills`, tüm workspace'ler paylaşır) **genel olarak bayat**
  kalmış (11 default skill'in hepsi farklı: self-management 361, settings 306, guide
  303 satır). On-disk kopyalar güncel gömülü içerikle **elle senkronlandı** (yedek:
  `~/.swarmgo/skills-backup-20260704-preconsolidation`; `otonom-dispatch` gibi kullanıcı
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
  (3) claude-cli'da Araçlar bölümünde — araçlar SwarmGo isteğinde şema olarak değil
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

## Workspace default promptu SwarmGo-native yeniden yazıldı ✅ (2026-07-03)

`internal/workspace/defaults/default-instructions.md` hâlâ the external agent project sistem
promptunun mekanik "the external agent project→SwarmGo" kopyasıydı — SwarmGo'da **olmayan**
onlarca yeteneği öğretiyor (`datatable`/`spreadsheet`, `html/pdf/markdown-preview`,
`render_template`, `call_llm`, `~/.external-agent/docs/*`, `_displayName` MCP meta,
External Sources+`guide.md` modeli), **gerçek** yüzeyi (run_subagent, use_skill,
set_session_goal, flows/self-management/handoff/plan modu, gerçek render seti) hiç
anlatmıyordu. Ayrıca ajana talimat olmayan the external agent project iç dokümantasyonu (Dynamic
context / Complete user message / SDK config bölümleri + mini-agent promptu) ve
makineye özel sızıntı (gömülü Bilal tercihleri + sabit `C:/Users/user/...` yolları)
içeriyordu.

**Yapılan:** dosya sıfırdan SwarmGo-native olarak yeniden yazıldı (~750 → ~150
satır). Tasarım ilkesi the external agent project'ın "her şeyi inline et" (~37K token) yaklaşımı
yerine SwarmGo'nun **küçük cache'li prefix + skill'e devret** felsefesi (`_Docs/17`,
`_Docs/19`): skill kataloğu + `GoalUsageHint` zaten prefix'te enjekte edildiği için
prompt artık ansiklopedi değil, doğru araç yüzeyi + skill pointer'ları. İçerik
İngilizce (kod/prompt kuralı). Render fence'leri gerçek koda göre doğrulandı
(`frontend/.../CodeBlock.tsx`: yalnız `diff`/`mermaid`/`gallery`+`image-preview`).
Document Tools bölümü **kullanıcı kararıyla korundu** ("ileride eklenecek" notuyla).

**Regresyon kilidi:** `internal/workspace/defaults_test.go` —
`TestDefaultInstructionsAreSwarmGoNative` embed'in yasak the external agent project-ism string'leri
(`datatable`/`call_llm`/`render_template`/`~/.external-agent/docs`/`_displayName`/
`html-preview`…) içermemesini ve gerçek SwarmGo terimlerini (`run_subagent`/
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

claude-cli built-in araç yüzeyi ile SwarmGo native araçları arasındaki boşlukların
kapatılması (the external agent project↔SwarmGo backlog `_Docs/41`).

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
  Read/Edit/Write'ı zaten kendi read-before-write guard'ını uyguluyor (SwarmGo'nun
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
timed out.` verir (SwarmGo işi arka planda bitirir). **Fix:** `claudecli.go runAttempt`
CLI process'ine `MCP_TOOL_TIMEOUT=600000` + `MCP_TIMEOUT=60000` ms enjekte eder (kullanıcı
override kazanır → `ensureEnvDefault`).

**Dokunulan:** `internal/agent/subagent.go` (çözümleme sırası + Guard 4), `internal/
providers/claudecli.go` (`ensureEnvDefault` + MCP timeout env), testler
`TestResolveSubagentAgentBeatsProfile`/`TestAsyncProfileRejected`. Doküman `_Docs\25` +
skill `swarmgo-session-debug` (desen G/H). `go build`/`go test ./internal/agent
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
  sessizce **Go motoruna düşer** → davranış her iki yolda birebir. `SWARMGO_GREP_NO_RG=1`
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

**Sorun:** SwarmGo'nun `Edit`/`Write` araçları önceki bir `Read`'i takip etmiyordu →
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
  skill `swarmgo-settings`'e belgelendi. `buildRegistry` session-id'yi (`SessionIDFrom`)
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
  wake yolunu da kapsar) + gömülü default skill `swarmgo-coordinator`.
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

**Düzenek:** Geçici ikinci SwarmGo instance'ı (ayrı port + temp data dir — canlı
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
`swarmgo-autonomous-ops` skill'ine "etiketle döngü kur" reçetesi.

## Code Execution with MCP — Settings toggle + UI trace kartları ✅ (2026-07-02)

**İstek:** Kod-modunu env-only olmaktan çıkarıp Settings'e almak + `run_code` içi
MCP çağrılarını sohbet trace'inde kart olarak göstermek (`_Docs/44` kalan işler).

**Yapılan:**
- **Settings toggle `enableCodeMode`:** `settings.go` (Settings/DTO/Patch/mapping) +
  `store.go` apply + `api/server.go` `applySettings → SetCodeMode` (canlı) +
  `app.go`'da `SWARMGO_CODE_MODE` artık `EnableShell` gibi **tek seferlik boot seed**
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
  tavanı, 120s per-call timeout, çağrı sayacı/özeti) + `bindings.go` (`.swarmgo/mcp/`
  altına `_bridge.py` + server-başına Python modülü; docstring = açıklama + input schema;
  Python identifier sanitizasyonu; her çağrıda sıfırdan regen — bayat stub kalmaz).
- **Yeni araç `run_code`** (`internal/tools/builtin_runcode.go`): boş script → binding
  regen + modül/fonksiyon listesi (discovery); script → stripped env
  (`minimalScriptEnv`) + `PYTHONPATH` + köprü URL/token env'i ile python çalıştırır;
  yalnız stdout/stderr (16KB cap) + MCP çağrı özeti döner — **veri context'e girmez**.
  RiskExec (`classify.go`), varsayılan 60s / max 300s timeout.
- **Gate'ler:** `Tunables.codeMode` (default KAPALI, `SWARMGO_CODE_MODE=1` ile boot'ta
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
  push + `app.go` env seed `SWARMGO_ENABLE_DELEGATION`) tamamen çıkarıldı. **Kalan
  frenler:** `delegationMaxDepth` (1–10, vars. 3) + `delegationMaxCalls` (1–100, vars. 8)
  — her delegasyon çağrısında geçerli güvenlik/bütçe guard'ları.
- **Frontend:** `AppToolsPanel` toggle'ı bilgi kartıyla değiştirildi (araç Araçlar
  ekranından yönetilir), derinlik/çağrı limitleri koşulsuz gösteriliyor.
  `types/settings.ts` + `SettingsPanel.tsx` payload'ından `enableDelegation` alanı kaldırıldı.
- **Testler:** `TestRunSubagentGate` → `TestRunSubagentAlwaysInstalled` (registry'de
  daima var); `TestDelegation_DisabledToolAbsent` → `TestDelegation_ToolAlwaysAvailable`
  (unknown-tool hatası ALMAZ); `store_test`/`subagent_test`/`delegation_e2e_test`
  `EnableDelegation`/`SetDelegationEnabled(true)` referanslarından temizlendi.
- **Dokümanlar:** `swarmgo-settings`/`swarmgo-self-management`/`swarmgo-autonomous-ops`
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

**İstek:** SwarmGo'ya Fable 5 (claude-fable-5) desteği ekle.

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
- **Metinler:** ContextPanel çıktı-tavanı ipucu, `swarmgo-settings` default skill,
  `_Docs/17` adaptif-fraction tablosu ve `swarmgo-project` skill'i yeni aile
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
- `swarmgo-project` skill'i (the external agent project workspace) düzeltildi: modül yolu
  `github.com/bilal-arikan/swarmgo` (yanlış `bilal/swarmgo` idi), go.mod'a `jchv/go-webview2`
  eklendiği bilgisi, klasör yapısına `cmd/swarmgo-desktop` + `internal/app`, doc index'e
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
- **Dokümanlar/skill:** `swarmgo-settings` + `swarmgo-autonomous-ops` skill'leri güncellendi
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
sistem promptu gibi yap (SwarmGo'da monolitik sistem promptu yok — workspace prompt
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
konuşma iki runtime'da ölçüldü. SwarmGo (claude-cli, resume açık) ilk turları **soğuk**
yazıp tur-başı ~70-90K cache **yeniden yazıyor** (warm-read tutarsız, yalnız bazı
turlarda); the external agent project SDK 1. turdan **istikrarlı sıcak** cache okuyor (cR≫cW) → aynı
konuşmada ~3× ucuz. Ağır (~40KB) workspace prompt eklemek SwarmGo'da cache-write'ı
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
  **Tam sökme (aynı gün):** `settings.EnableSelfManage` alanı + `SWARMGO_ENABLE_SELFMANAGE`
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
- `cmd/swarmgo-desktop/titlebar_windows.go`: preset→renk haritası kaldırıldı; titlebar
  artık id son-ekine (`-light`/`-dark`) göre mod-neutrals seçiyor (accent kullanılmıyor).

**Doğrulama:** `go build ./...` + desktop build ✅, frontend `tsc --noEmit` temiz ✅.
`swarmgo-settings` skill'i güncellendi.

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
  `internal/skills/defaults/swarmgo-deliverables/SKILL.md` (`access: shared`, on-demand). Tüm
  detaylı kurallar (binary `sourcePath`, inline media, gallery, `update_artifact` by id) skill
  body'sine taşındı. `internal/api/artifacts.go` `artifactDeliverableGuidance` 2 satırlık özet +
  `use_skill swarmgo-deliverables` pointer'ına indirildi. Bilgi **kaybolmadı** — sadece prefix'ten
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
dış MCP araçları (ör. `mcp__mcp-chrome__*`) tam açıklamalarıyla dökülüyordu; SwarmGo'nun
kendi `swarmgo_extended` araçları ise zaten yalnız-ad. Tutarsızlık + her tur ölü token.

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
frontend `tsc --noEmit` temiz ✅. Skill dökümanları (`swarmgo-settings`,
`swarmgo-autonomous-ops`) güncellendi.

## Ayarlar ▸ Gelişmiş'ten ölü alt-bölümler kaldırıldı ✅ (2026-06-30)

**İstek:** Gelişmiş ekranındaki "MCP & Araçlar" ve "Tanılama" bölümleri gereksizse
kalksın; loglar zaten Logs ekranına gidiyor, oradan filtrelenebiliyor.

**Teşhis (ikisi de ölü ayar):**
- `mcpGatewayUrl` ("MCP & Araçlar"): tüm repoda yalnız kaydedilip yükleniyor, runtime
  hiç **tüketmiyor**. Gerçek MCP sunucu yönetimi ayrı "Araçlar & MCP" (`mcptools`)
  kategorisinde → alt-bölüm gereksiz.
- `logLevel` ("Tanılama"): `cmd/swarmgo`'da hiç okunmuyor → logger'a **uygulanmıyor**.
  Ayrıca `LogsPanel.tsx` zaten seviye filtresi + metin araması sunuyor → gereksiz.

**Yapılan (frontend):** `SettingsPanel.tsx`'ten iki `AdvSection` (MCP & Araçlar,
Tanılama) + `McpPanel`/`DiagnosticsPanel` import'ları + kullanılmayan `Plug`/`Activity`
ikonları kaldırıldı; `settings/appPanels.tsx`'ten `McpPanel` ve `DiagnosticsPanel`
fonksiyonları silindi. Backend alanları (`mcpGatewayUrl`/`logLevel`) geriye-uyum için
settings.json'da kalır (zararsız). `swarmgo-settings` skill'inde ikisi deprecated/unused
not edildi. `tsc --noEmit` temiz.

## Bütçe/limit UI kaldırıldı — ajanlar daima sınırsız ✅ (2026-06-30)

**İstek:** Ayarlardaki "Bütçe" sekmesi kalksın; "Günlük çağrı limiti" ve "Günlük
token limiti" olmasın, hep sınırsız olsun.

**Yapılan (frontend):**
- Ayarlar kategorisi `budget` kaldırıldı: `settings/primitives.tsx` (`Cat` union +
  `APP_CATS`), `SettingsPanel.tsx` (import + `cat === 'budget'` render),
  `settings/appPanels.tsx` (`BudgetPanel` fonksiyonu silindi).
- NavRail **Bütçe** ekranı (`panels/BudgetPanel.tsx`) korunur (harcama görünümü) ama
  limit yüzeyi söküldü: inline `editLimit` prompt'u, "Token limiti doluluk" + "Durum"
  sütunları, `/limit` çağrı göstergesi, over/near durum rozeti, "limit" düzenle butonu.
- Kullanılmayan `api.setBudget` (`api/agents.ts`) kaldırıldı.

**Backend:** `defaultDailyCallLimit`/`defaultDailyTokenLimit` zaten 0 (sınırsız);
alanlar geriye-uyum + self-management için settings.json'da kalır ama artık UI'dan
düzenlenemez → yeni ajanlar daima sınırsız. Enforcement (budget.go) 0'da no-op.
`tsc --noEmit` temiz.

## Türkçe karakter (UTF-8) mojibake — teşhis + onarım ✅ (2026-06-30)

**Şikâyet:** Bellek/recall'a giren Türkçe metin bozuluyor (`Kısaca`→`KÄ±saca`,
`kaç`→`kaÃ§`); uyarı `internal/memory` (journal) + `conversation/db` JSONL byte
handling'i işaret ediyordu.

**Teşhis (ampirik, kapatıldı):** Kök neden SwarmGo'da **DEĞİL**. Kanıtlar:
(1) Go I/O uçtan uca UTF-8 — `internal/db/store.go` (JSONL) `encoding/json` + atomik
bayt yazımı, `internal/memory` + `agent/reflector.go` (journal=`"Q: "+req.Message+...`)
saf Go string; tüm `internal`'da **tek `DecodeString` base64**, hiçbir charset decoder
(`x/text/charmap`/CP125x) yok. (2) `claude-cli` provider'ı SwarmGo'nun bire-bir byte
yoluyla (stdin raw UTF-8 / stdout raw) Türkçe'yi **doğru** round-trip eder (probe ile
doğrulandı). (3) Diskteki yer-gerçeği: asistan cevapları `×`/`÷`/`−` çok-baytlı Unicode'u
**kusursuz** saklamış; yalnız **user mesajları** bozuk → bozulma `req.Message`
SwarmGo'ya gelmeden, **gönderen istemcide** (double-encode: UTF-8 bayt CP1254 çözülüp
tekrar UTF-8). Klasik **Windows PowerShell 5.1** `Invoke-RestMethod` string-gövde /
BOM'suz-UTF-8-dosya-ANSI-okuma hatası (`dev.ps1` ASCII-only kuralının aynısı).

**Karar:** Sunucu tarafına otomatik onarım **eklenmedi** (double-encoded girdi geçerli
UTF-8'dir, meşru Latin-1'den ayırt edilemez → sessiz yanlış-pozitif riski; sınır
istemcidir).

**Yapılanlar:** (1) `scripts\repair-encoding.ps1` — bir-seferlik güvenli onarım: span
bazlı (ftfy-benzeri) ters-çevirme, yalnız geçerli-UTF-8 oluşturan + lead-bayt `C2-C5/E2`
koşuları (temiz Türkçe harf/sembolde yanlış pozitif yok; karışık bozuk-Q+temiz-A
satırları da düzelir), knowledge'da stale `embedding` null'lanır (Go `unmarshalVector(nil)`
→ içerikten yeniden hesaplar), session.jsonl ham-satır onarımı (JSON round-trip riski yok),
`.bak-encfix` yedek + dry-run varsayılan. WS5'te **21 alan / 16 dosya** düzeltildi, 0
kalıntı, 0 geçersiz JSON. (2) `scripts\e2e-smoke.ps1` `Api-Post`/`Api-Put` sertleştirildi
(gövde artık UTF-8 byte[]). (3) Doküman: `33-DIS-AJAN-OTOMASYONU.md §A.1.1`.

## claude-cli kimlik popup'ı (Max OAuth / API token) ✅ (2026-06-30)

**Hedef:** İzole config dizini için **SwarmGo UI'ından** login akışı — bir popup'ta
ya Max/Pro hesabı ya API anahtarı eklenebilsin; dizinde ayrı `claude login` gerekmesin.

**Mekanik:** Claude CLI auth-precedence'ı token'ı env'den kabul ediyor
(`ANTHROPIC_API_KEY` > `CLAUDE_CODE_OAUTH_TOKEN` > config-dir `.credentials.json`).
Yani token'ı subprocess env'ine enjekte etmek izole/boş dizini login'siz yetkilendirir
(token inference için yeterli). İki yöntem: **Max/Pro** → kullanıcı `claude setup-token`
ile 1 yıllık OAuth token üretir (tarayıcı, tek sefer), popup'a yapıştırır →
`CLAUDE_CODE_OAUTH_TOKEN` (abonelik, API faturası yok); **API** → `sk-ant-…` →
`ANTHROPIC_API_KEY`.

**Katmanlar:** `settings.go` (`ClaudeCliAuthKind` + şifreli `ClaudeCliAuthTokenEnc`;
DTO `claudeCliAuthKind`/`claudeCliAuthSet`; Patch write-only `claudeCliAuthToken`),
`store.go` (write-only encrypt + `ClaudeCliAuthToken()` decrypt accessor),
`providers/kind.go` (`CLIAuthKind`/`CLIAuthToken`), `registry.go`
(`SetClaudeAuth` + resolve), `kind_claudecli.go`+`claudecli.go` (`NewClaudeCLI` 5 param +
`runAttempt`'ta kind'e göre env enjeksiyonu; ANTHROPIC_API_KEY öncelikli olduğu için
tek env set edilir), `api/server.go applySettings` (`SetClaudeAuth`). Frontend: yeni
`ClaudeAuthDialog.tsx` modal (2 sekme + setup-token komutu kopyalama + paste +
sil), `ProvidersPanel` Anthropic kartında "claude-cli kimlik" butonu + durum rozeti,
`settings.ts` tipler. Token şifreli saklanır, API'ye asla dönmez (`claudeCliAuthSet`
bool). `go build`/`go vet`/`tsc` ✅.

**Ek (test butonu):** Anthropic kartındaki mevcut "Test et" **anthropic HTTP API**
anahtarını dener; yalnız Max/Pro OAuth token varken `invalid x-api-key` döner. Bu yüzden
karta ayrı **"claude-cli'yi test et"** butonu eklendi → `runTest('claude-cli')` (backend
test endpoint'i zaten herhangi bir provider'ı `s.providers.Get` ile kurup gerçek `claude`
turu atıyor; config dir + enjekte token ile). Bağımsız rozet (`test['claude-cli']`).
Backend değişmedi; yalnız `ProvidersPanel` extra slot.

## claude-cli izole config dizini (CLAUDE_CONFIG_DIR) ✅ (2026-06-30)

**Hedef:** Kullanıcının mevcut `~/.claude`'una (dolu skill/tool/MCP/global CLAUDE.md/login)
**dokunmadan** claude-cli'yi **temiz** bir config evi ile çalıştırabilmek. İkinci binary
kurmak çözüm değil — tüm claude binary'leri aynı `~/.claude`'u okur; ayrım **config
dizininde**.

**Çözüm:** Yeni `claudeConfigDir` ayarı → claude-cli subprocess'ine `CLAUDE_CONFIG_DIR`
env olarak enjekte edilir. Boş = ortak `~/.claude`; bir yol verilince CLI
skill/ayar/komut/global CLAUDE.md/login'i o izole dizinden okur.

**Default (2026-06-30):** Artık **izole-by-default** → `~/.swarmgo/claude-home`
(`settings.defaultClaudeConfigDir()`, home çözülemezse `""`=ortak `~/.claude` fallback).
Yani claude-cli kutudan çıktığı gibi temiz bir config evinden çalışır; **tek seferlik
`claude` login** o dizinde gerekir. Anahtarsız-mevcut-login davranışı istenirse alan
boşaltılır (UI placeholder `otomatik (~/.claude)`). Bilinçli ürün kararı: SwarmGo runtime'ı
kullanıcının kişisel `~/.claude` skill/tool kirliliğinden ayrışır. (SwarmGo zaten
`--strict-mcp-config` ile MCP'leri izole ediyordu; bu, eksik olan skill/ayar/CLAUDE.md
katmanını da kapatır.) CLI bu env'e saygı duyar (doğrulandı: Claude Code env-vars docs +
issue #25762); SwarmGo CLI'ı doğrudan subprocess çağırdığı için VS Code eklentisindeki
bug yolu etkilemiyor.

**Dokunulan katmanlar (mevcut `claudeCliPath` aynalandı):** `settings.go`
(Settings+DTO+Patch+default `claudeConfigDir`), `store.go` (applyString),
`providers/kind.go` (`ResolvedConfig.CLIConfigDir`), `registry.go`
(`claudeConfigDir` alanı + `SetClaudeConfigDir` + resolve), `kind_claudecli.go`
(`NewClaudeCLI` 3. param), `claudecli.go` (`configDir` alanı + `runAttempt`'ta
`cmd.Env` enjeksiyonu), `api/server.go applySettings` (canlı uygula), frontend
(`settings.ts` tip, `SettingsPanel` save, `ProvidersPanel` Anthropic kartında opsiyonel
2. endpoint alanı "claude config dizini"). Default skill `swarmgo-settings` belgelendi.
`go build`/`go vet`/`tsc` ✅.

## Token/bütçe muhasebesi denetimi + CLI ek-yükü düzeltmesi ✅ (2026-06-30)

**Hedef:** Token ve bütçe hesaplamalarında yanlışlık var mı? (SwarmGo↔the external agent project
token kıyası oturumunun ardından). **Denetim sonucu:** Çekirdek muhasebe **doğru** —
OpenAI-uyumlu yol `prompt_tokens`'tan `cached`'i çıkarıyor (çift-sayım yok,
`minimax.go toUsage`), Anthropic native ayrık sayaçlar, fiyat kademeleri
(`pricing.go CostDetailed`: input + cacheRead×0.10 + cacheWrite×1.25 + output),
günlük/oturum toplama her sağlayıcı çağrısı başına doğru topluyor (`RecordUsage`).

**Tek gerçek kusur:** `api/session_context.go computeCLIOverhead`. claude-cli'nin
`result` zarfındaki `cache_read_input_tokens` **tek tur içindeki iç tool-loop
adımlarının KÜMÜLATİF** toplamıdır (tek-geçiş bağlamını kat kat aşar — bir API
çağrısı cache'ten yazılandan fazlasını okuyamaz; kanıt: SES5 tur 2 `cacheRead=269485`
iken yazılan ≤ ~86K → ~5 iç çağrının toplamı). **Maliyet için doğru** ama **bağlam
boyutu değil**. Eski kod bunu "çağrı başına gerçek girdi" sayıp `In+CacheRead+
CacheWrite` ile sahte ~5–7× ek-yük üretiyordu (kıyas oturumunda SwarmGo'yu olduğundan
ağır gösteren rakam buydu).

**Değişiklik:** `computeCLIOverhead` artık cacheRead katkısını çağrı başına bağlam
tahminiyle **sınırlıyor** (`capRead = min(cacheRead, estimated)`), hem debug-event
hem lifetime-fallback yolunda. Soğuk ilk turda (cacheRead≈0) ölçülen≈tahmin → gerçek
yük ~0; ısınınca ~1.5× düzeyinde gerçekçi kalır. Tip dokümanları (`session.ts`
`CLIOverhead`, `session_context.go cliOverheadPreview`) + `_Docs/17` güncellendi.
Build + `billing/providers/conversation/db` testleri ✅. Parser'a (kümülatif kayıt
billing için doğru) dokunulmadı.

**Takip düzeltmeleri (aynı gün):**
1. **Bütçe kapısı cache'i sayıyor (`agent/budget.go`):** `ensureBudget` artık
   `daily_token_limit`'i `input+output` yerine **`billableTokens(u)`** ile
   kıyaslıyor — cache token'ları fiyat çarpanlarıyla katlıyor (read×0.10,
   write×1.25). claude-cli/Anthropic'te tüketimin çoğu cache trafiği olduğundan eski
   `input+output` kapısı neredeyse hiç tetiklenmiyordu; ağırlıklı sayım kapıyı
   gerçek harcamayla orantılı yapar.
2. **Native turda mesaj-balonu tur-toplamı (`agent/toolloop.go` + `turnmeta.go`):**
   Native çok-adımlı tool-loop artık her sağlayıcı çağrısının usage'ını
   **`sumUsage`** ile biriktirip dönen yanıta yazıyor (`turnUsage`). Eskiden balon
   yalnız **son** iterasyonun token'ını gösteriyordu → balonların toplamı oturum
   ömür-boyu toplamından az çıkıyordu (RecordUsage zaten çağrı-başı topluyordu). Artık
   tutarlı. claude-cli yolu etkilenmez (tek Complete; usage zaten tur-aggregate).
   Birim testler: `budget_test.go` `TestBillableTokens` + `TestSumUsage` ✅.

**CLI-overhead num_turns düzeltmesi (aynı gün, takip-2):** İlk düzeltmede konan
`min(cacheRead, estimated)` sınırı **ters yönde** hata yaptı — başka bir oturumda
fark edildi: sıcak turda gerçek çağrı-başı ~53K iken preview ~12K gösteriyordu (~34K
eksik). **Ham claude-cli stream'i ile kesin semantik doğrulandı:** `result.num_turns`
= turdaki iç tool-loop API çağrı sayısı; result `usage` o çağrıların **toplamı**
(`cacheRead=46658 = 21628+25030`, num_turns=2). Doğru çağrı-başı bağlam =
`(in+cacheRead+cacheWrite)/num_turns` (= 30.692 ≈ gerçek 28.9K/32.4K). **Zincir:**
`providers/claudecli.go` parser `num_turns`'ü `Response.ProviderCalls`'a yakalar →
`Response.ProviderCalls` alanı + `db.DebugEvent.Calls` alanı eklendi → `RecordUsage`
artık `providerCalls` parametresi alıp debug `llm_call`'a `Calls` yazar (4 çağrı yeri
güncellendi) → `api/session_context.go computeCLIOverhead` `capRead` hack'i kaldırılıp
`/num_turns` bölmesi kondu. Lifetime-fallback (rollup num_turns saklamaz) yaklaşık
kalır — debug-journal yolu (vars. açık) kesin. Tip dokümanları + `_Docs/17`
güncellendi. Test: `claudecli_usage_test.go TestCLIParserNumTurns` ✅; tüm suite ✅.

**Lifetime-fallback kesinleştirme (aynı gün, takip-3):** Yukarıdaki yaklaşıklık
giderildi — `db.UsageDelta`'ya `ProviderCalls` taşıyıcı alanı + `db.Usage` ve
`db.SessionUsage`'a kümülatif **`ProviderCalls`** sayacı (Σ num_turns) eklendi.
`AddUsageKind`/`AddSessionUsageKind` artık her delta'nın round-trip'ini fold ediyor
(`providerCallsOf`: bildirilmeyen 0 → `Calls`'a, yani 1'e tabanlanır; native+compaction
doğru sayılır, claude-cli num_turns). `RecordUsage` `delta.ProviderCalls`'ı set ediyor.
`computeCLIOverhead` fallback'i artık `(in+cacheRead+cacheWrite)/u.ProviderCalls` ile
debug-journal yolu kadar kesin (eski sessionlarda sayaç 0 → turn-count'a geriler).
Test: `store_session_usage_test.go TestSessionUsageProviderCalls` ✅.

## Model-farkında çıktı tavanı (max_tokens) ✅ (2026-06-29)

**Hedef:** Tüm sağlayıcı çağrılarında `req.MaxTokens` boş bırakılıyordu →
sağlayıcıların sabit `defaultMaxTokens=4096` fallback'ine düşüyordu. Bu, modelin
gerçek kapasitesinden bağımsız yapay düşük bir çıktı tavanı demekti: yanıtlar
ortada kesilip turn-recovery (A1) `max_output_tokens_recovery` döngüsünü gereksiz
sıklıkta tetikliyordu (ör. SES16'da MiniMax-M3 tek turda 4096'ya **iki kez** çarptı;
M3'ün gerçek sınırı ~512K).

**Değişiklikler:**
- `providers/context_window.go`: **`MaxOutputFor(provider, model)`** aile-bazlı tablo
  (`ContextWindowFor` kardeşi, aynı muhafazakâr felsefe — değerler ailenin en zayıf
  üyesi için bile geçerli, 4096'nın çok üstünde): opus/sonnet + minimax **32K**,
  haiku/fable/generic-claude **16K**, deepseek/gemini **8K**, bilinmeyen **0** →
  sağlayıcı fallback. `max_tokens` yükseltmenin maliyet/rate-limit dezavantajı yok
  (gerçek üretilen token başına ödenir), tek risk gerçek sınırı aşıp 400 → muhafazakâr.
- `providers/catalog.go`: `ModelInfo.MaxOutput` alanı + `Catalog()` build'inde
  `MaxOutputFor`'dan doldurulur (manifest'ler churn'süz; tıpkı `ContextWindow`).
- `agent/maxoutput.go`: **`(r *Runtime) withMaxOutput(provider, req)`** yardımcısı —
  yalnız `MaxTokens==0` iken doldurur (compaction/summary/title açık değerleri
  korunur). Üç huniye bağlı: `recordedComplete` + `recordedStream` +
  `guardedComplete`.
- **Runtime ayarı (Ayarlar ▸ Bağlam):** `settings.MaxOutputTokens` (default 0=auto,
  clamp 256–512000) → `Tunables.SetMaxOutputTokens`/`MaxOutputTokens()` →
  `api/server.go applySettings`. UI: "Çıktı token tavanı" alanı (recovery grid'i,
  `appPanels.tsx` + `types/settings.ts` + `SettingsPanel.tsx`). **Öncelik (MaxTokens
  boşken):** Settings override (>0) → env `SWARMGO_MAX_OUTPUT_TOKENS` (>0) → aile
  tablosu → sağlayıcı fallback (4096).
- Testler: `providers/maxoutput_test.go` (aile + katalog), `agent/maxoutput_test.go`
  (fill/explicit-korunur/override). Doğrulama: `go build ./...` ✅, `go test` 166 ✅.
- Detay/kavram: `_Docs\10` §A1 (eski "max-token escalation merdiveni" boşluğu kapandı).

## Harici araç dedektörü genelleştirildi (kategori + wire) ✅ (2026-06-29)

**Hedef:** PATH'te kurulu harici araçları tarayan presence-only mekanizmanın
(`rtk`/`sqz`/`context-mode`) kapsamını token araçlarının ötesine genişletmek.

**Değişiklikler:**
- `api/external_tools.go`: `knownExternalTools` girişlerine **`Category`** (gruplama:
  `token`/`dev`/`render`) + **`Wire`** (`hook`/`mcp`/`cli`) alanları eklendi; yanıt
  struct'ı (`externalToolStatus`) bunları JSON'a yansıtıyor. İki yeni araç: **`crabbox`**
  (`dev`/`cli` — uzak yürütme/test control-plane'i; geliştirmede Bash ile çağrılır) ve
  **`mmdc`** (`render`/`cli` — mermaid-cli, yerelde mermaid→SVG/PNG dosya çıktısı).
  Hâlâ `exec.LookPath` ile **presence-only** — kurulum/çalıştırma/değişiklik yok.
- `frontend/types/settings.ts`: `ExternalToolStatus`'a `category`+`wire` eklendi.
- `frontend/components/settings/HooksPanel.tsx`: araçlar **kategoriye göre gruplanır**
  (`TOOL_CATEGORY_LABELS`), rozet/buton **`wire`'a göre** gösterilir — `hook`→Bağla/Aktif
  toggle (`TOOL_HOOK_TEMPLATES`, artık yalnız hook araçları için), `mcp`→MCP rozeti,
  `cli`→CLI rozeti. Bölüm başlığı "Harici token araçları" → "Harici araçlar".
- **Genişletilebilirlik:** yeni araç = `knownExternalTools`'a tek giriş; `wire="hook"`
  olduğunda ayrıca frontend `TOOL_HOOK_TEMPLATES`'e şablon. Detay: `_Docs\17` §Harici araç tespiti.
- Doğrulama: `go build ./internal/api` ✅, frontend `tsc --noEmit` ✅.
- **Kapsam dışı (kullanıcı kararı):** Kroki / Mermaid Live Editor HTTP render yeteneği
  (built-in `render_mermaid` aracı) bu turda yapılmadı — bunlar PATH CLI'ı değil, ayrı feature.

## Zengin görev alanları + PM-benzeri kart modalı + Obsidian-pm köprüsü ✅ (2026-06-29)

**Hedef:** Board kartlarını obsidian-pm (Obsidian "Project Manager" eklentisi)
deneyimine yaklaştırmak; SwarmGo board'unu Obsidian'da Kanban/Tablo/Gantt olarak
görüp **çift yön** senkronlamak.

> **Güncelleme (2026-06-30):** `type` (task/subtask/milestone) ve `parentId`
> SwarmGo'dan kaldırıldı (ihtiyaç yok) — `Task` modeli, API, araçlar ve modal
> Tür alanı temizlendi. Subtask hiyerarşisi yalnız Obsidian (PM) tarafında yaşar;
> köprü onu PM-only tutar. SwarmGo'da kalan zengin alanlar: **priority + tags**.

**Zengin görev modeli (backend):**
- `db.Task` yeni opsiyonel alanlar: `priority` (critical/high/medium/low),
  `tags []string`, `type` (task/subtask/milestone), `parentId`, `progress`,
  `startDate`, `dueDate` — hepsi `omitempty`, eski task JSON'ları zero-value alır
  (migration yok). `ValidPriority`/`ValidTaskKind` doğrulayıcıları.
- `store_task.go UpdateTask` yeni alanları kopyalar; `api/tasks.go` create+update
  request'leri + validasyon (geçersiz priority/type → 400), `clampProgress` (0–100).

**PM-benzeri kart modalı (frontend):**
- Yeni `TaskFormModal.tsx` — ortalanmış popup, **oluşturma + düzenleme** tek bileşen.
  Alanlar: Başlık (AI retitle), Açıklama, Durum (kolon), **Öncelik**, **Tür** (+subtask'ta
  üst-görev seçici), Ajan, Akış, **Etiketler** (chip+input), Bağımlılıklar (`DependencyPicker`).
- `TaskBoard.tsx`: satır-içi oluşturma formu ve eski sağ panel (`TaskDetailPanel`,
  **silindi**) kaldırıldı → "+ Görev" modalı açar, karta tıklayınca edit modalı; kartlarda
  priority/tür/etiket rozetleri. (İlerleme barı + tarih alanları kullanıcı isteğiyle
  modalden çıkarıldı; backend alanları omitempty olarak duruyor.)
- `types/task.ts`/`api/tasks.ts` yeni alanlarla genişledi.

**Obsidian-pm köprüsü (harici, `Desktop\Progs\swarmgo-obsidian-sync`, Python):**
- SwarmGo REST API (`/api/tasks`) ↔ obsidian-pm projesi (düz Markdown: `<proje>.md`
  + `<proje>_tasks/*.md`, `pm-project`/`pm-task` frontmatter). Sunucu/DB yok.
- **Tam çift-yön:** başlık/açıklama/durum/owner/deps + **priority/tags/type/parentId**
  + subtask hiyerarşisi (subtask'lar proje `taskIds`/gövdeden hariç, parent `subtaskIds`
  yeniden hesaplanır). `parentId` link tablosundan PM-id↔SG-id çevrilir.
- Çakışma: *boş tarafı doldur; ikisi de doluysa son-yazan-kazanır* (LWW **tur başındaki
  orijinal** zaman damgalarıyla — çekirdek push'un PM dosyasını yeniden yazıp updatedAt
  bumplaması kaynaklı yanlış-yön hatası bu şekilde giderildi). İdempotent.
- Durum eşlemesi: SG `failed`↔PM `blocked`, `review`↔`review`, gerisi birebir.
  Bağ + son-senkron anlık görüntüsü sidecar `state/<proje>.json`'da; `swarmgoId`
  PM frontmatter'ına gömülü (sidecar kaybolsa bağ kurtarılır).
- **`watch` modu:** periyodik otomatik senkron; Progs altında arka plan servisi olarak
  Windows zamanlanmış görevle (logon'da) çalışır.
- Canlı backend'de uçtan uca doğrulandı (SG↔PM priority/tags/type/parent, idempotent).

### Köprü → çoklu-provider mimarisi (kanban soyutlaması) ✅ (2026-06-30)

**Hedef:** Kanban kontrolünü tek bir platforma (obsidian-pm) sabitlemek yerine
**değiştirilebilir provider** arkasına almak; ileride Trello/Asana/WeKan'a yalnız
yeni bir dosya yazarak geçebilmek (`mermaid-cli` benzeri soyutlama). SwarmGo task
store'u **canonical kaynak** olarak kalır (ajan orkestrasyonu onun üstünde).

- **Faz 1 — soyutlama (davranış değişmedi):** `sgsync/providers/` paketi eklendi.
  `base.py` provider sözleşmesi (`Card` canonical model — status'u SwarmGo board
  sözlüğünde; `ProjectInfo`; `Caps` yetenek bayrakları; `KanbanProvider` ~5 metot).
  Tüm obsidian-pm mantığı engine'den `providers/obsidian.py`'ye taşındı
  (status/hiyerarşi map, `swarmgoId` gömme, `subtaskIds` roll-up). `engine.py` artık
  **provider-bağımsız** — yalnız `Card` + `KanbanProvider` konuşur; çakışma çözümü,
  kimlik eşleme, çoklu-workspace fan-out, watch kilidi aynen korundu.
- **Capability modeli:** provider tutamadığı alanı bildirir (`Caps`); engine zorla
  map etmez, **atlar** (ör. Trello'da priority/deps/hierarchy yok → es geçilir).
- **Faz 2 — Trello provider:** `providers/trello.py` (REST: list↔status, label↔tag,
  arşiv↔close, SG id desc marker'ına gömülü; key+token auth). `config.json`'a
  `_trello_example` mapping bloğu + factory (`make_provider`, mapping `provider`
  anahtarı, varsayılan `obsidian`, geriye uyumlu).
- **State göçü:** snapshot `pm_status` artık canonical tutulduğu için tek-seferlik
  `migrate_state_v2.py` eski native değerleri çevirdi (10 link); böylece refactor
  sonrası ilk sync'te sahte churn olmadı.
- **Doğrulama:** obsidian yolu canlı backend'de **dry 0/0 stabil** (davranış birebir);
  `tests/test_providers.py` → Trello map mantığı + factory hataları + **engine'in
  board-bağımsızlığı** (sahte provider/SG ile iki-yön kart üretimi) yeşil. Watch
  servisi yeni kodla yeniden başlatıldı, idle teyit edildi. Detay: köprü README.

### Köprü sadeleştirme: Trello provider kaldırıldı + dosyalar workspace içine (2026-06-30)

**Hedef:** Generic board yönetim sistemini korumak ama Trello provider'ını sökmek;
obsidian-pm dosyalarını harici tek vault yerine **her SwarmGo workspace'inin kendi
klasörüne** taşımak (Obsidian kullanıcısı o klasörü manuel vault olarak açar).

- **Trello kaldırıldı:** `providers/trello.py` silindi, registry'den çıkarıldı,
  `config.json`'daki `_trello_example` ve testteki Trello map kaldırıldı. Generic
  seam (`base.py`/`obsidian.py`/factory) **aynen duruyor** — registry'de tek slug
  (`obsidian`); yeni provider eklemek hâlâ tek dosya + tek satır.
- **Workspace-içi yerleşim:** yol artık mapping'in `workspace_id`'sinden türüyor →
  `<swarmgo.data_dir>/workspaces/<WS_ID>/<obsidian.subdir>/`. `_resolve_projects_dir`
  önceliği: `obsidian_dir` override → `data_dir`+workspace → eski `projects_dir`
  (geriye uyumlu). `data_dir` var ama `workspace_id` yoksa **sessizce yanlış yere
  yazmaz, hata fırlatır.** Backend değişikliği gerekmedi (yol bridge tarafında türetiliyor).
- **Veri göçü:** mevcut 3 proje (`SwarmGo`/`DenemeBilimsel`/`SwarmGoRepo` ↔ WS1/WS2/WS5)
  eski vault'tan `…/.swarmgo/workspaces/<WS>/obsidian/`'e taşındı; state link'leri
  pm_id bazlı olduğundan korundu (`pull` yeni konumdan aynı kartları/id'leri okudu).
  `ikariam` örnek projesi (mapping'siz) eski vault'ta bırakıldı.
- **Doğrulama:** `tests/test_providers.py` (yol çözümü + factory + board-bağımsızlık)
  yeşil; `pull` yeni konumdan çalışıyor. Watch yeni config'le yeniden başlatıldı.
  Tam sync 0-churn teyidi **backend açıkken** yapılacak (test sırasında dev backend
  kapalıydı). Obsidian'da `…/workspaces/<WS_ID>/obsidian` → "Open folder as vault".

## Dış MCP için Streamable HTTP transport + `mcpServers` JSON içe aktarma ✅ (2026-06-29)

**Hedef:** Native MCP istemcisi şimdiye dek yalnız **stdio** konuşuyordu (sse/http
`dial()`'de "not yet supported" hatasıyla reddediliyordu). Uzak/hosted MCP
sunucuları için **Streamable HTTP** (MCP spec 2025-03-26/2025-06-18) transport'u
eklendi; ayrıca standart `mcpServers` JSON belgesiyle **toplu içe aktarma**.

**Transport soyutlaması (refaktör):**
- `internal/mcp/client.go` — yeni `Client` arayüzü (`ListTools`/`CallTool`/`Alive`/
  `Close`/`SetOnToolsChanged`/`SetLogger`); `StdioClient` ve `httpClient` ikisi de
  uygular (compile-time assert). JSON-RPC çözümleme mantığı transport-bağımsız serbest
  fonksiyonlara çıkarıldı: `parseToolsList`/`parseCallResult`/`callToolParams`.
- `pool.go` `poolEntry.client` ve `ensure()` artık somut `*StdioClient` yerine
  `Client` arayüzü tutar → pool transport'tan habersiz, havuzlama/TTL/re-dial aynen
  her iki transport için çalışır.
- `manager.go` `dial()` → `Client` döner; `http` → `DialHTTP`, `sse` → açık
  "deprecated, use http" hatası.

**Streamable HTTP istemcisi (`internal/mcp/http.go`, yeni):**
- Tek endpoint'e POST; her istek bağımsız (HTTP yanıtı isteğe doğal eşlenir → stdio'daki
  gibi read-loop/demux gerekmez). `Accept: application/json, text/event-stream`.
- Yanıt **JSON** (tekil) **veya** **SSE stream** (çoklu event; isteğin yanıtı id ile
  bulunur, aradaki `notifications/tools/list_changed` callback'e dispatch edilir) — ikisi
  de ele alınır.
- `Mcp-Session-Id` handshake'te yakalanır, sonraki her istekte echo edilir;
  `MCP-Protocol-Version` header'ı; Close'da best-effort session DELETE.
- Statik `Headers` (örn. `Authorization`) her isteğe uygulanır. Yeni bağımlılık YOK
  (`net/http` + manuel SSE parse). Testler `http_test.go` (JSON + SSE + session echo +
  list_changed callback + hata durumu) — 4 vaka ✅.

**Veri modeli + API:**
- `db.MCPServer` yeni `HeadersConfig` alanı (JSON object, http header'ları);
  `mcp.ServerConfig.Headers`; `toServerConfig`/`mcpServerConfig`/`climcp` köprülerinde
  taşınır (CLI köprüsü zaten `type:http`+url+headers iletiyordu).
- `POST /api/mcp-servers/import` — yapıştırılan `mcpServers` JSON'undan toplu oluşturma;
  hem `{"mcpServers":{...}}` hem çıplak `{name:spec}` kabul; transport `type`'tan, yoksa
  command→stdio / url→http çıkarımı; entry-başına hata (biri bozuksa batch düşmez).
  Tekil create ile ortak `createMCPReq.toMCPRow()` doğrulaması (http→url zorunlu,
  sse→reddedilir).

**Frontend (`ToolsPanel.tsx` + `types/mcp.ts` + `api/mcp.ts`):**
- `MCPServer.headersConfig`, `MCPImportResult` tipleri; `mcpApi.importMCPServers(json)`.
- ServerManagement'a **"JSON ile içe aktar"** kutusu (textarea + placeholder örnek +
  sonuç/hata mesajı). Transport dropdown'dan `sse` kaldırıldı (`http (Streamable HTTP)`).
- **Opsiyonel header alanı:** http transport seçiliyken URL altında textarea — her satır
  `Anahtar: Değer` (ilk `:` böler, sonrakiler değerde kalır → `Bearer x:y` korunur);
  yalnız http'de gönderilir, boşsa `undefined`.

**Durum:** build ✅, `go test ./...` 544 ✅, `tsc --noEmit` temiz ✅. Canlı uzak-sunucu
doğrulaması kullanıcı tarafında yapılabilir (httptest ile transport doğrulandı).

## Sohbette ajan başlığına tıklayınca ajan ayar sayfası ✅ (2026-06-29)

**Hedef:** Sohbet detay ekranında asistan balonunun üstündeki ajan adı/avatarına
tıklayınca o ajanın ayar sayfasına (Ajanlar görünümü, ajan seçili) gidilsin.

**Yapılanlar (frontend-only):**
- `App.tsx` — `openAgentSettings(id)` callback'i (`setActiveAgentId(id)` + `setView('agents')`);
  `MessageList`'e `onOpenAgent={openAgentSettings}` olarak geçer.
- `MessageList.tsx` — `onOpenAgent` prop'u; hem `AssistantTurn`'e hem standalone
  pending balonundaki `AgentHeader`'a iletilir.
- `AssistantTurn.tsx` — `onOpenAgent` prop'u eklendi, `AgentHeader`'a geçer.
- `AgentHeader.tsx` — `onOpenAgent` verildiğinde kimlik satırı bir `<button>`'a sarılır
  (hover vurgusu + "Ajan ayarlarını aç" tooltip); yoksa eski salt-görüntü davranışı.
- Doğrulama: `tsc` temiz, `npm run build` başarılı.

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

## claude-cli cache sıcaklığı — 4 fazlı çözüm ✅ (2026-06-29)

**Sorun (ölçüldü):** Taze claude-cli session'ı tam soğuk başlıyor ve **tur 2 de soğuk** (cache_read=0). Kök neden: `claudecli.go` statik sistem promptu + **volatil** dinamik bloğu (saniye-hassas saat + bellek recall + özet) TEK `--append-system-prompt`'a birleştiriyordu → cache'lenen ~30K prefix her tur değişiyor.

**Faz 1 — Cache prefix stabilizasyonu (keystone).** `ClaudeCLI.buildSystemAndPrompt`: `--append-system-prompt`'a YALNIZ statik `req.System` gider; volatil `req.SystemDynamic` konuşma prompt'una (stdin) `[Context]…[/Context]` bloğu olarak taşınır → append-system byte-stabil → Claude Code auto-cache sıcak kalır. İzin+MCP arg üretimi `permissionArgs`/`mcpArgs` helper'larına çıkarıldı. **Canlı kanıt:** resume turn 3, volatil dinamikle **cache_read=54.932, cacheWrite=61**.

**Faz 2 — `ClaudeResume` varsayılan açık.** `settings.Default()` → `ClaudeResume: true`. Faz 1 sayesinde resume artık gerçekten ısıtıyor: **turn 2 cache_read=45.420** (canlı). Live test `TestLiveClaudeCLIResume` cache_read>0 assert'i ile genişletildi.

**Faz 3 — Statik prefix cross-session deterministik.** Denetlendi: `composeTurnRequest` zaten temiz statik/dinamik ayrımı yapıyor (saniye-hassas saat dinamikte), tool+skill katalogları zaten Name'e göre sort'lu (`registry.go`/`store.go`). Değişmezlik birim testi `TestBuildSystemAndPrompt` ile kilitlendi (sys yalnız statik, deterministik, dinamik kuyrukta).

**Faz 4 — Kalıcı claude-cli süreci (opsiyonel, deneysel, default off).** `providers.CLISession` + `CLISessionPool` (`claudecli_session.go`): session başına uzun-ömürlü `claude --input-format stream-json` süreci; soğuk başta tam transkript, sıcakta yalnız son kullanıcı mesajı (süreç gerisini hatırlar). `Runtime.cliSessions` pool'u (CloseMCP'de kapanır, 30dk idle eviction), tek-huni `recordedComplete`'te session-id ile devreye girer (hata → tek-atış fallback); `ClaudePersistentSession` açıkken `planClaudeResume` trim'i devre dışı. Ayar tüm sitelere bağlandı (Settings/DTO/Patch/Apply/tunable/server apply). **Canlı:** context korunuyor (ZEBRA-9) + cache ısındı (turn 2 cache_read=87.672); warmth TTL'e bağlı, default off. Live test `TestLivePersistentSession`.

Doğrulama: `go build ./...` + `go vet` + paket testleri temiz; `SWARMGO_LIVE_CLI=1` ile iki live test PASS.

## Context-preview "CLI ek yükü" satırı ✅ (2026-06-29)

**Sorun:** `GET /api/sessions/{id}/context-preview` çıktısı (`systemTokens`/
`toolTokens`/`totalTokens`) yalnızca SwarmGo'nun **kendi** enjekte ettiği katmanı
sayar. `claude-cli` sağlayıcısında alttaki CLI **kendi sistem
promptu + araç şemaları + MCP köprüsünü** modele ekler — SwarmGo bunu hiç
görmediği için `totalTokens` gerçek faturalanan girdiyi ciddi şekilde **az
raporlar**. Canlı ölçüm (AGT1 Coder, opus, SES75, 3 çağrı ort.): tahmin **6.832**
→ gerçek **49.844** token (~**7,3×**, +43.012 ek yük).

**Çözüm:** `sessionContextPreview`'a opsiyonel `cliOverhead` alanı eklendi
(`internal/api/session_context.go` → `computeCLIOverhead`). Yalnız CLI-wrapper
sağlayıcılarda (`claude-cli`) dolar; native (anthropic/minimax/
openrouter) ajanlarda `nil` → segment tahmini zaten doğru. Gerçek girdi, session'ın
kayıtlı lifetime usage'ından ölçülür: `measuredTokens = (input + cacheRead +
cacheWrite) / calls`; `overheadTokens = max(0, measured − estimated)`. Henüz tur
gönderilmemişse `measured=0` + "ölçülmedi" notu.

**UI:** `SessionContextModal.tsx` token özetine "Gerçek (CLI, ölçülen)" rozeti +
altına sarı **CLI ek yükü** uyarı bandı (tahmin→gerçek, ×kat, çağrı sayısı, not).
Tip: `types/session.ts` `CLIOverhead`. Doğrulama: `go build ./internal/api` +
`tsc --noEmit` temiz; canlı SES75 (cliOverhead dolu) + SES74/Minimax3 (nil) test.

**Yan bulgu (cache stabilitesi):** SES75 ardışık turlarda cache_read = 0 (tur1),
**0 (tur2)**, 45.920 (tur3) ölçüldü → claude-cli'da tur-arası prompt-cache sıcaklığı
garanti değil; araç/prompt değişimi prefix'i bozabiliyor.

## Ctrl/Cmd+Click ile çoklu seçim + toplu eylemler ✅ (2026-06-29)

**Hedef:** Listelerde birden fazla öğe seçip tek seferde toplu işlem yapabilmek
(sohbet, ajanlar, board kartları, hafıza, artifact, skill, flow, araç, aktivite).

**Ortak altyapı (frontend-only):**
- `hooks/useMultiSelect.ts` — liste-agnostik seçim çekirdeği: `selected` Set'i +
  shift-aralık `anchor` ref'i. `handleClick(e, id, ordered)` modifier yorumlar
  (Ctrl/Cmd=toggle, Shift=aralık, plain=temizle+anchor) ve **seçim jesti mi**
  döndürür → çağıran normal navigasyonu bastırır. `selectAll`/`clear`/`toggle`/
  `replace`. ≥1 seçili iken **Escape** global temizler. Çapraz-platform
  (`ctrlKey || metaKey`).
- `components/common/SelectionBar.tsx` — seçim ≥1 olunca beliren sticky toplu
  eylem çubuğu (sayaç + filtre-dışı ipucu + "Tümü" + temizle); `SelectionBarButton`
  kompakt eylem butonu. `common/index.ts`'ten export edilir.

**Bağlanan listeler + toplu eylemler:**
- **Sohbet** (`SessionsSidebar`): AI başlık · Sabitle · Arşivle/çıkar · Sil
  (bucket'lar arası shift-aralık; filtre-dışı seçili sayısı gösterilir).
- **Ajanlar** (`AgentsView`): Sil.
- **Board kartları** (`TaskBoard`): Sütuna taşı · Ajan ata · Sil (sütunlar arası
  düz render sırasıyla shift-aralık).
- **Hafıza** (`MemoryPanel`): Sil — yalnız **modifier-click** seçer (plain-click
  kart genişletmeyi korur; kart-içi handler'lar modifier'ı yutar).
- **Artifact** (`ArtifactsPanel`): Sil.
- **Skills** (`SkillsPanel`): Sil (katlanmış grupları atlayan shift-aralık).
- **Flows** (`FlowsPanel`, Akışlarım sekmesi): Çalıştır · Sil.
- **Araçlar — ajan** (`AgentToolsSection`): plain-click anında yasaklar; modifier-click
  çoklu seçer → "Seçilenleri yasakla" tek `setAgentTools` PATCH'i.
- **Araçlar — workspace** (`ToolsPanel`, Ayarlar): plain-click detay açar; modifier-click
  çoklu seçer → Etkinleştir · Devre dışı (`setWorkspaceTools`) · NameOnly · Göster
  (`setWorkspaceToolsVisibility`); gruplu (yerleşik + MCP) shift-aralık katlanmışları atlar.
- **Aktivite** (`ExecutionsPanel`): salt-okunur → "Kimlikleri kopyala".

**Desen:** Toplu eylemler mevcut tekil API'leri döngü/`Promise.all` ile kullanır
(yeni backend yok); optimistic state + hata halinde reload. Yıkıcı eylemler tek
`confirm("N öğe…")` ile. Detay: `_Docs\45-COKLU-SECIM.md`. Doğrulama: `tsc
--noEmit` temiz.

## Son kullanıcı mesajı sohbette üstte sticky (ChatGPT/Claude tarzı) ✅ (2026-06-29)

**Hedef:** Transkripti kaydırırken **en son yazdığımız kullanıcı mesajı** ekranın
üstüne yapışsın (aktif soru, altındaki yanıt kaydırılırken görünür kalsın).

**Yapılanlar (frontend-only):**
- `MessageList.tsx` — en son **gerçek** (typed; `origin` set olmayan, yani
  AutoPromptNote olmayan) kullanıcı mesajının index'i (`lastUserIndex`) hesaplanır;
  o satırın wrapper'ına `sticky -top-2 z-10 bg-[var(--color-bg)] pb-2` eklenir.
  Solid arka plan + alt padding, altından kayan satırların sızmasını engeller.
  Flash-highlight ring class'ı ile birleşik className üretilir.
- Konteyner üst padding'i azaltıldı (`py-6` → `pb-6 pt-2`) ve sticky offset `-top-2`
  yapıldı → mesaj viewport'un en tepesine (flush) yapışır, üstte boşluk kalmaz.
- **Yapışıkken kompakt:** sticky satır artık **şeffaf** (balonun kendi accent dolgusu
  yeterli; `bg-[var(--color-bg)]` kaldırıldı). Yalnızca **yapışık durumdayken** mesaj
  2 satıra kırpılır (`line-clamp-2`) — normal akışta tam metin görünür. "Stuck" tespiti
  `onScroll`/messages-effect içinde: `data-pinned-user` satırının rect.top'ı konteyner
  üstüne değince (`scrollTop>4` korumalı) `stuckPinned=true`. `clamp` prop'u
  MessageList→UserTurn→UserBubble zinciriyle taşınır.
- Yalnız tek bir kullanıcı mesajı sticky'dir (diğer turlar normal akışta) → üst üste
  yığılma olmaz. Scroll-anchoring (bottom-pin) mantığı değişmedi.
- **Dinamik section-header pinleme (sabit index değil):** sabit pin yerine kaydırma
  sırasında **viewport üstüne çıkmış en son typed-user mesajı** dinamik olarak pinlenir
  (`activePinnedIndex`). `onScroll`/messages-effect → `updateActivePinned`: tüm
  `[data-user-row]` satırlarını tarar, `rect.top ≤ konteyner üstü` olanlardan **en büyük
  index**'i (DOM sırasında sonuncu) seçer. Yeni bir soru üste değince pin ona devreder;
  yukarı kaydırınca öncekine geri döner. Hiçbiri üste çıkmamışsa `-1` (ilk mesaj artık
  boşuna pinlenmez — önceki şikâyet giderildi). **Yalnız aktif satır** sticky olduğu için
  başlıklar üst üste yığılmaz.
- **Aktif başlık görünümü:** aktif satır `sticky -top-2 z-10 bg-gradient-to-b
  from-black to-transparent pb-6` (yukarıdan aşağı siyah→şeffaf gradient, altından kayan
  metin okunabilirlik için solar) + balon 2 satıra `line-clamp-2`. `clamp` prop'u
  `isActivePinned` ile MessageList→UserTurn→UserBubble zinciriyle taşınır.
- Doğrulama: `tsc --noEmit` temiz, `npm run build` başarılı.

## claude-cli dosya düzenlemeleri için diff paneli (Edit/Write tutarlılığı) ✅ (2026-06-29)

**Hedef:** Sohbette dosya-düzenleme araçlarına (Edit/Write/MultiEdit) tıklayınca
the external agent project'taki gibi silinen/eklenen satırların görüldüğü belirgin bir diff paneli
açılsın. Sorun: native tool-loop düzenlemeleri zaten `diff` adımı → güzel **DiffCard**
veriyordu; ama asıl provider olan **claude-cli** düzenlemeyi kendi uyguladığı için
SwarmGo `recordDiff` çağrılmıyor → düzenleme generic `tool` adımı olarak **ActivityCard**
ile (yalnızca açınca, sönük) gösteriliyordu → iki yol arasında tutarsızlık.

**Yapılanlar (frontend-only; Go değişmedi):**
- **`synthDiffData` yardımcısı** (`frontend/src/lib/diff.ts`): bir düzenleme aracının
  girdisinden (`old_string`/`new_string`, `content` veya MultiEdit `edits[]`) tam
  unified-patch + `+added/−removed` sayımı + hedef yol üretir. `synthDiff` MultiEdit'i
  artık her edit için ayrı patch'i istifleyerek işliyor; `isEditToolBase` predicate'i
  eklendi.
- **DiffCard iki yolu da besliyor** (`DiffCard.tsx`): `step.patch` varsa (native) onu,
  yoksa (claude-cli `tool` adımı) `synthDiffData` ile sentezler → her iki yol da aynı
  paneli render eder. `actionLabel` MultiEdit'i de "Düzenle" sayar.
- **TurnSteps yönlendirmesi** (`TurnSteps.tsx`): hatasız + düzenleme-aracı + sentezlenebilir
  bir `tool` adımı artık **DiffCard**'a gider. Hatalı düzenlemeler ActivityCard'da kalır
  (hata çıktısı + uygulanmamış sönük diff görünür) — native yolun `!res.IsError` koşuluyla
  aynı davranış.
- Doğrulama: `tsc --noEmit` temiz, `npm run build` başarılı.

## Plan artifact "📋 Plan" chip'i + artifact hızlı-önizleme modalı ✅ (2026-06-29)

**Hedef:** (1) Agent plan oluşturup artifact olarak kaydedince o artifact'a "plan"
chip'i de eklensin. (2) Bir artifact'a tıklayınca Artifactlar ekranına gitmeden
önizlemesi açılsın.

**Yapılanlar (frontend-only; backend zaten `origin="plan"` set ediyordu):**
- **"📋 Plan" chip'i:** `OriginBadge`'e `plan` girişi eklendi (accent-renkli);
  `Artifact.origin` tipine `'plan'` eklendi; Artifactlar liste filtre çubuğuna
  **Plan** facet'i eklendi. Dosyalar: `frontend/src/components/panels/artifactMeta.tsx`,
  `frontend/src/types/artifact.ts`, `frontend/src/components/panels/ArtifactsPanel.tsx`.
- **Hızlı önizleme modalı:** yeni `ArtifactPreviewModal` bileşeni — sohbet/aktivite
  içindeki artifact chip/kartına tıklanınca `getArtifact` ile içeriği çekip
  `ArtifactView` ile ortada bir overlay'de gösterir (Esc/backdrop kapatır, "Ekranda
  aç" kısayolu tam Artifactlar ekranına geçirir). `App.openArtifact` artık modalı
  açar (`previewArtifactId`); tam ekran navigasyonu `openArtifactFull`'a taşındı.
  Dosyalar: `frontend/src/components/artifacts/ArtifactPreviewModal.tsx`, `frontend/src/App.tsx`.
- `tsc --noEmit` temiz. Detay: `_Docs\40-PLAN-MODE.md`.

## Dokümante özelliklerin test kapsamı tamamlandı ✅ (2026-06-29)

**Hedef:** Dokümanlarda anlatılan ama test edilmemiş davranışları tespit edip
testlerini eklemek (eksik/yanlış test denetimi). Tüm suite zaten yeşildi (524
test); odak, dokümante-edildiği halde unit-test kapsamı olmayan davranışlar.

**Eklenen testler (524 → 539):**
- **Orchestration (`internal/orchestration/branch_delay_test.go`, _Docs/15):**
  - `branchArmMatches` üç eşleşme modu (`contains`/`equals`/`regex` + boş→contains
    fallback) — önceden yalnız `regex` GAN-loop üzerinden dolaylı test ediliyordu.
  - `evalBranch` default-arm önceliği + default-yok-eşleşme-yok ("no match") yolu.
  - **Delay node** (hiç testi yoktu): gerçekten bekler + trace yazar + `Next`'e
    geçer; context-iptali "delay" hatasına döner; `sleepCtx` sıfır/negatif no-op.
- **Backup (`internal/backup/backup_test.go`, _Docs/34):** `Unzip` **zip-slip**
  guard'ı — kötücül `../` zip *girdisi* reddedilir ve destDir dışına yazılmaz
  (önceden yalnız API-katmanı dosya-adı traversal'ı test ediliyordu).
- **Memory graph (`internal/memory/graph_test.go`, _Docs/23):** kenar `Score`
  alanı gerçek lexical-cosine benzerliğini taşıyor (eşik..1 aralığı + cosine
  eşitliği) — önceki test yalnız kenar *sayısını* doğruluyordu.

## Yeni agent'lar varsayılan olarak araç kullanabilir ✅ (2026-06-29)

**Hedef:** UI/API'den oluşturulan yeni agent'ların `mcpEnabled` alanı varsayılan
olarak **açık** gelsin — minimax/anthropic gibi native sağlayıcılarda araç
döngüsü `MCPEnabled`'a bağlı olduğundan, kapalı default ile yeni agent'lar
araçsız başlıyordu (SES8 / AGT7 belirtisi). Aynı kural `false`→`true` flip'i
olarak **tüm agent-oluşturan call site'larına** yayıldı (market, workspace
template, ingest).

**Çözüm (per-creation-path default-on flip):**
- **DB seviyesi (`internal/db/store.go`):** `CreateAgent` `MCPEnabled`'a default
  koymaz (Go bool "unset" ile "explicit false" ayırt edemez); her caller flip'ten
  sorumlu — merkezi "default true" override'ı kasıtlı `false`'yu da ezerdi.
- **HTTP create (`internal/api/agents.go`):** `createAgentReq.MCPEnabled *bool`
  → `nil` default on, `false` opt-out (tek gerçek opt-out kanalı).
- **Market install (`internal/api/market.go` `installAgentPack`):** payload
  `mcpEnabled` yoksa (Go zero value `false`) → `true` flip.
- **Workspace template seeding (`internal/api/templates.go` `seedWorkspaceTeam`):**
  template agent `MCPEnabled` yoksa → `true` flip (ek olarak `BlockedTools` zaten
  template'da tanımlıysa onu olduğu gibi taşır).
- **Self-management `create_agent` (`internal/tools/builtin_agentmgmt.go`):**
  zaten önceden `MCPEnabled: true` literal'ı vardı.
- **E2E harness (`internal/e2e/harness_test.go`):** zaten açıkça `MCPEnabled: true`.

**Chat-only ajan isteyen (escape hatch):** yeni ajan oluşturduktan sonra
`POST /api/agents/{id}/tools` body `{"mcpEnabled":false}` (UI'da ajan detay
▸ Araçlar ▸ ana switch) ile kapatabilir. `BlockedTools` daha ince tanelidir.

**Test:** `TestCreateAgentPerCallerFlipDefault` (`internal/api/agents_default_tools_test.go`)
iki flip'i (HTTP `*bool` + bool pass-through) + DB round-trip ile sabitler.
Tam suite **524 test** yeşil.

## `ask_user` şeması claude-cli native AskUserQuestion'a hizalandı ✅ (2026-06-29)

**Sorun (SES73):** claude-cli modeli köprülü `ask_user`'ı native `AskUserQuestion`
şemasıyla çağırdı — `options`'ı **obje dizisi** (`[{"content":"..."}]`) olarak yolladı;
SwarmGo ise `[]string` bekliyordu → `json: cannot unmarshal array into ... askInput.options
of type string` → 3 ardışık `ask_user` hatası, model 4. turda options'ı bırakıp düz metinle sordu.

**Çözüm:** `ask_user` şeması native ile uyumlu hale getirildi (`internal/tools/builtin_ask.go`):
- `options` öğesi artık **string VEYA obje** olabilir (`{label|content|value|text|description}`
  → görünen metne normalize; `askOption` + `flexOptions`).
- Native **`questions[]` wrapper** da tolere edilir (ilki kullanılır — SwarmGo tek soru sorar).
- Ortak `tools.ParseAskInput` hem native tool yolunda hem claude-cli Interaction MCP köprüsünde
  (`callAsk`) kullanılır → iki yol birebir aynı çözümler. Şema `oneOf` (string|object), ekstra
  native alanlar (header/multiSelect) yok sayılır. Test: `TestParseAskInput` (7 şekil) geçti.

## Doğrulama araçları (skill/config/mermaid) + interaction köprü konsolidasyonu ✅ (2026-06-29)

External Agent `session-tools-core` ↔ SwarmGo araç eşleştirmesindeki boşluk analizinden
(bkz. `_Docs/analiz-craftagent-arac-eslestirme.md`) çıkan üç **salt-okuma doğrulama
aracı** eklendi:

- **`skill_validate`** — bir skill'in SKILL.md'sini doğrular (slug hijyeni, frontmatter
  `name`/`description`, boş gövde). Mantık tek kaynak: `skills.Store.ValidateSkill`
  (`internal/skills/validate.go`); tool `internal/tools/builtin_skillvalidate.go`
  (`SkillValidator` arayüzü, agent tarafı `agentSkillWriter.ValidateSkill` adaptörü).
- **`config_validate`** — SwarmGo JSON config dosyalarını doğrular (geçerli JSON +
  tanınan şekiller için beklenen alanlar: settings.json / tools-config.json / agent /
  mcp-server). `internal/tools/builtin_configvalidate.go`, çalışma dizini sandbox'ı.
- **`mermaid_validate`** — saf-Go hafif lint (tanınan diyagram tipi + denge kontrolü;
  tam parser DEĞİL, sınır belgelendi). `internal/tools/builtin_mermaidvalidate.go`.

Üçü de native builtin + **NameOnly (lazy)** → claude-cli'da `swarmgo_extended`
köprüsünden `BridgeableDefs` ile otomatik gelir. Kayıt: `toolsetup.go` (mermaid base,
skill_validate `r.skills!=nil`, config_validate `sb.Ready()`; üçü NameOnly tier'a eklendi).

**Köprü konsolidasyonu (#2):** `mcp_interaction.go`'da delege eden 6 `callXxx`
(todo/artifact/notify/focus/goal/sessionEdit) tek tablo-güdümlü `sinkToolTable` +
`callViaSink` yardımcısına indirildi — davranış birebir korundu, ~120 satır boilerplate
kalktı. (Not: bu, "tek Context, çok-backend" refactor taslağının düşük-riskli/kozmetik
parçası; büyük `ToolContext` arayüzü gereksiz bulunup uygulanmadı — SwarmGo deseni zaten
ctx-value injection ile gerçekliyor.)

**Test:** `builtin_validate_test.go` (mermaid/config/skill_validate), `skills/validate_test.go`
(ValidateSkill) + tüm suite **510 test** yeşil. Doküman/skill: `_Docs/19` (NameOnly seti),
`swarmgo-self-management` (skill_validate notu), `swarmgo-guide` (mermaid_validate notu).

## Plan modu bağlandı + ölü `planningMode` alanı kaldırıldı ✅ (2026-06-28)

İki iş tek oturumda: (a) eski **ölü `planningMode`** ajan alanı ("Standart"/"Derin")
uçtan uca silindi — runtime'da hiç okunmuyordu (Go: models/store/agents/market/
templates/pack; frontend: types + `agentOptions` `PLANNING_OPTIONS` + AgentSettingsForm
picker'ı). Migration gerekmez (struct'tan kalkınca eski JSON alanı sessizce yok sayılır).

(b) **claude-cli plan modu (`ExitPlanMode`) gerçekten çalışır hale getirildi.** Önceden
headless `-p` modunda `ExitPlanMode` onayı alınamıyor, araç `"Exit plan mode?"` +
`is_error` dönüyor, ajan planı "reddedildi" sanıyordu. Artık native `ExitPlanMode`
çağrısı `--permission-prompt-tool` köprüsünden `permission_prompt`'a düşürülüp özel
ele alınıyor (`callExitPlan`): plan markdown'ı **plan onay kartı**na (`StepPlan` →
`PlanPrompt.tsx`, "Planı onayla"/"Reddet") dönüşür, karar `run.answer` kanalından gelir.
Mod davranışı: **ask** → her tur köprüden; **read-only** → `--permission-mode plan` +
köprü (CLI tüm yazmaları bloklar, yalnız ExitPlanMode düşer); **auto** → plan araçları
disallow (onaylayıcı yok); **otonom** → otomatik onay.

**Onaylanan plan → artifact (birikme kontrollü, eklendi):** plan onaylandığında otomatik
olarak **session başına TEK rolling artifact**'a yazılır (`db.AppendPlanArtifact`,
`origin="plan"`, başlık "📋 Onaylanan Planlar"); her yeni plan `## Plan N` bölümü olarak
**eklenir** (yeni artifact açılmaz) → artifact sayısı plan sayısıyla değil session sayısıyla
sınırlı. Hem manuel onayda hem otonom oto-onayda; best-effort. Doğrulandı: `go build`/`vet`
temiz, 92 db/api testi + `TestAppendPlanArtifact` geçti, `npm run build` temiz. Detay:
`_Docs/40-PLAN-MODE.md`.

## Log boşlukları kapatıldı: MCP client + compaction + izin denetim izi ✅ (2026-06-28)

Önceki oturumda listelenen kalan log boşluklarının **üçü de** in-app Logs ring
buffer'ına bağlandı (hepsi nil-safe logger, sıcak yolda maliyetsiz):

**(1) `internal/mcp/client.go` — stdio JSON-RPC client artık logger taşıyor.**
Pool yalnız "connection_dead" semptomunu görüyordu; client read-loop'unun **neden**
öldüğü görünmezdi. Eklenen: `SetLogger(l, server)` + nil-safe `log`. Loglanan:
read-loop çıkışı (`read loop exited (connection lost)`, kendi `Close()`'umuz hariç —
`wasClosed` ayrımı; EOF=Info, diğer=Warn, stranded pending sayısı dahil), non-JSON
inbound satır (Debug), bilinmeyen/geç id (Debug). Pool dial sonrası
`client.SetLogger(p.logger, cfg.Name)` ile bağlar.

**(2) Bütçeli compaction / rolling-summary fold loglanıyor.** `conversation/
manager.go`'ya `SetLogger` eklendi; `Prepare` rutin fold'da `context compacted
(rolling summary fold)` (session/agent/folded_msgs/before_tokens/after_tokens/
budget), `ForceCompact` manuel `/compact`'te `context compacted (manual /compact)`
logluyor. `api/server.go` `s.convo.SetLogger(logger)` ile bağladı → chat (sync+
stream) + wake/otonom yolların **hepsi** tek noktadan kapsanır. Önceden yalnız
`api/chat.go` yanıtında (`prep.Compacted`) görünüyordu, Logs'a hiç düşmüyordu.

**(3) İzin onayları / "her zaman izin ver" grant'ları — denetim izi.** `permission.go`
saf iki-değer imzasını korudu; logger **ctx üzerinden** taşınır (`withPermLogger`/
`logPerm`, testlerde `context.Background()` → no-op). `permGate` artık logluyor:
standing-rule auto-allow (Debug), onay-bir-kez (`granted (once)`, Info), her-zaman
grant (`granted (always) — standing rule recorded` + `rule=Bash(git *)` gibi
`PermRule.String()`, Info), kullanıcı reddi (Info). `toolloop.go` çağrıda
`withPermLogger(ctx, r.logger)` enjekte eder (reddetme zaten `tool blocked` ile
loglanıyordu). Böylece neyin yetkilendirildiği Logs'ta izlenebilir.

**Doğrulama:** `go build ./...` + `go test ./...` (**505 test, 32 paket**) yeşil.

## Logs ekranı: kaydırma-aware otomatik takip + MCP pool yaşam-döngüsü logları ✅ (2026-06-27)

**(1) Logs ekranı — yukarı kaydırınca otomatik dibe inmesin.** Önceden `follow`
(Canlı) açıkken her yeni satırda viewport koşulsuz en alta zıplıyordu; kullanıcı
eski logları okumak için yukarı kaydırsa bile yeni log gelince aşağı çekiliyordu.
`LogsPanel.tsx`'e **dibe-sabitlenme takibi** eklendi: `onScroll` viewport'un dibe
yakın olup olmadığını (`scrollHeight - scrollTop - clientHeight < 24px`) ölçüp
`atBottomRef`'i günceller; otomatik kaydırma efekti yalnız `follow && atBottom`
iken çalışır. Kullanıcı yukarıdayken yeni satırlar görünümü bozmaz. Yukarı
kaydırılmışken sağ-alt köşede **"En alta in"** (ArrowDown) floating butonu görünür
→ tıklanınca tekrar dibe sabitlenir. Frontend `tsc --noEmit` yeşil.

**(2) MCP kalıcı havuzu (pool) yaşam-döngüsü logları.** `internal/mcp/pool.go`
tamamen sessizdi — bağlantı dial/yeniden-dial/ölüm ve `tools/list_changed`
olayları Logs ekranına hiç düşmüyordu (churn'e duyarlı yeni özellik için kör
nokta). Pool'a **opsiyonel/nil-safe logger** (`SetLogger`) eklendi; loglanan
olaylar: `connected` (reason=first_connect/config_changed/connection_dead),
`dial failed`, `tool list invalidated (list_changed)`, `call hit dead connection,
re-dialing`. Runtime kuruluşunda `r.mcpPool.SetLogger(logger)` ile bağlandı →
loglar in-app ring buffer'a akar. `NewPool()` imzası korundu (testler kırılmadı).
`go build ./...` + `go test ./internal/mcp ./internal/agent` (106 test) yeşil.

**Diğer log boşlukları (sonraki adım önerisi):** `internal/agent/debugjournal.go`
(yalnız hata-debug), `internal/ingest/ingest.go` (tek log), `permission.go` (izin
ver/reddet izlenmiyor) — istenirse benzer şekilde enstrümante edilebilir.

## Otonom turlarda per-message metadata + label/status geri çekildi ✅ (2026-06-27)

**Otonom turlarda per-message metadata.** Önceden yalnız chat (sync+stream)
asistan mesajları `model/stopReason/usage/durationMs` taşıyordu; otonom turlar
(scheduler/spawn/inbox/wake) bunu atıyordu çünkü sarmalayıcılar (`invokeTraced`/
`runSessionTurn`/`complete`) provider `Response`'unu düşürüyor. **ctx-tabanlı
turn-meta yakalama** eklendi (imza değişmeden): `internal/agent/turnmeta.go`
(`WithTurnMeta(ctx)→*turnMeta`, `capture(resp)`, `apply(&msg,durMs)` + `usageMsg`);
`completeTraced` her başarılı completion'da ctx'teki sink'e yazar (tüm yollar
oradan geçer). Bağlanan persist siteleri: spawn (başarı+hata), inbox-delivery
(başarı+hata), schedule-delivery (başarı+hata), schedule-wake (başarı+hata) —
hepsi süre + model/stop/usage stamp'ler. Flow (kompozit transkript, per-node model
değişir) ve handoff (tombstone) bilinçli atlandı.

**Session `labels` + `status` GERİ ÇEKİLDİ (kullanıcı talebi).** Önceki gün eklenen
oturum etiket/durum alanları + editör + sidebar chip/filtre tamamen kaldırıldı
(model alanları, db setter'ları `SetSessionLabels/Status`, API `PUT .../labels|status`,
`SessionDetailPanel` "Etiketler & Durum" editörü, sidebar badge/chip/filtre). **Kalan
session.jsonl zenginleştirmesi:** per-message (`model/stopReason/usage/durationMs/
cancelled/feedback`) + session header `pinned` + `v` (SchemaVersion).

Build+vet+505 test + frontend `tsc -b` temiz.

## "Çalışıyor" (zıplayan nokta) indikatörü tüm tur boyunca kalıcı ✅ (2026-06-26)

**Sorun:** Sohbette ajan yanıtı başlamadan önce zıplayan 3-nokta ("yazıyor")
gösteriliyordu, ama **ilk tool adımı gelir gelmez kayboluyordu** —
`AssistantTurn.tsx`'te WorkingDots yalnız `steps.length === 0 && !text &&
!reasoning` iken çiziliyordu. Kullanıcı, indikatörün **tur tamamen bitene
kadar** (tool kullanımları + kısmi metin dahil) ve **scheduled_task/flow/spawn
koşularında da** görünmesini istedi.

**Çözüm:**
- `AssistantTurn.tsx`: WorkingDots koşulu `isLastLive && !interrupted &&
  !cancelled` oldu → tur canlı olduğu sürece (adımlar/metin olsa bile) balonun
  altında kalır, içerik varsa `mt-1.5` ile aralıklı; tur bitince (`isLastLive`
  false) kaybolur. Tamamlanmış boş balon artık sonsuza dek zıplamaz.
- `ExecutionsPanel.tsx`: `MessageList`'e `streaming={selected.running}`
  eklendi → koşan scheduled_task/flow/spawn'ın son asistan balonu "canlı"
  sayılır ve indikatör orada da tur bitene kadar devam eder.
- `tsc --noEmit` temiz.

## session.jsonl zenginleştirme: per-turn metadata + oturum etiket/durum/pin + feedback ✅ (2026-06-26)

`session.jsonl`'ye kullanıcıya-görünür, kalıcı alanlar eklendi (debug.jsonl ≠ bu;
gözlemlenebilirlik orada kalır):

**Message (asistan turu):** `model` (cevaplayan gerçek model), `stopReason`
(kesilme/red), `usage{in,out,cacheRead,cacheWrite}` (per-balon maliyet),
`durationMs`, `cancelled` (kullanıcı durdurması — `interrupted` crash'ten ayrı),
`feedback{rating±1,note,at}` (👍/👎). chat (sync+stream) yollarında populate;
otonom turlar `omitempty` boş. UI: balon-altı `model · ↑in ↓out ⚡cache` rozeti +
stopReason uyarısı + cancelled banner + hover'da 👍/👎 thumbs (optimistic).

**Session header:** `v` (SchemaVersion=1, ileri-migration), `pinned` (ListSessions
öne alır). UI: sidebar pin menüsü + 📌 gösterge. _(Not: aynı gün eklenen `labels`+
`status` ertesi gün kullanıcı talebiyle geri çekildi — yukarıdaki 2026-06-27 girdisi.)_

**Backend:** db setter'ları (SetSessionPinned, SetMessageFeedback) +
API `PUT /api/sessions/{id}/pin` + `.../messages/{msgId}/feedback`.
Test `db/session_meta_test.go` (round-trip + clear). Build+vet+test + frontend
`tsc -b` temiz. Detay: `_Docs\08-DEPOLAMA.md`.

## claude-cli 2.1.x+ iki-tier araç köprüsü: eager-core + lazy-extended ✅ (2026-06-26)

**Sorun:** claude-cli ajanlarında (ör. WS5/AGT4) `Bash` ilk turda `No such tool
available` veriyordu. Kök neden: SwarmGo tüm bridged araçları (eager + tüm
self-management suite) **tek** `swarmgo_interaction` MCP sunucusuna full-şema koyuyor;
toplam şema bağlam penceresinin %10'unu aşınca claude-cli 2.1.x **hepsini erteliyordu**
(Bash dahil). UI'daki "her tur şema gönderilen · 19" metriği **native** yola aitti;
CLI yolunda lazy-loading kazanımı gerçekleşmiyordu.

**Çözüm:** Interaction MCP'yi **iki sunucuya** böldük (claude-cli'ın native Tool
Search mekanizmasını doğru kullanarak):
- `swarmgo_interaction` (CORE, `alwaysLoad: true`) → eager tier, tool-search'ten muaf
  → Bash/ask_user/use_skill ilk turdan hazır. Eski anahtar korundu (namespaced
  referanslar bozulmadı).
- `swarmgo_extended` (EXTENDED) → self-management + NameOnly oturum araçları;
  `ENABLE_TOOL_SEARCH=auto` (CLI env) ile lazy keşfedilir.

**Dosyalar:** `internal/tools/interaction.go` (Core/ExtendedToolNames), `internal/
interaction/server.go` (`Tools(token,tier)` + `tierFromPath`), `internal/api/
mcp_interaction.go` (`coreInteractionTools`/`interactionTier`/`splitInteractionTiers`,
çift-prefix `bareToolName`), `internal/agent/climcp.go` (`AlwaysLoad` + iki anahtar),
`internal/agent/trace.go` + `toolsetup.go` (extended prefix), `internal/providers/
claudecli.go` (`ENABLE_TOOL_SEARCH=auto` env), `internal/api/server.go` (subtree mount),
çağıranlar `chat_stream.go`/`autonomous_interaction.go`.

**Kapsam:** yalnız claude-cli **2.1.x ve üzeri** (kurulu: 2.1.186). Sürüm guard'ı yok.
**Test:** `TestWriteCLIMCPConfigTwoTierInteraction`, `TestInteractionTierSplit`,
`TestLazyCatalogCLIFormNamespacesNames` + tüm suite (504 test) yeşil. Canlı doğrulandı
(WS5/AGT4, Playwright): `Bash` tek adımda (ToolSearch'süz), `list_agents` ise
`mcp__swarmgo_extended__` namespace'inden ToolSearch ile lazy yüklendi. Detay: `_Docs/19`.

## Oturum bilgisi panelinden "Ajanın bugünkü harcaması" kaldırıldı ✅ (2026-06-26)

Kullanıcı isteğiyle, oturum detay (Oturum bilgisi) panelindeki **"Ajanın bugünkü
harcaması"** bölümü (Motor B — ajanın gün içi tüm-oturum toplamı + model kırılımı
+ "Bütçe ekranı →" linki) kaldırıldı.

- `SessionDetailPanel.tsx`: ilgili `Section` bloğu + `agentUsage` state + onu
  besleyen `useEffect` + `agentId`/`onOpenBudget` prop'ları + kullanılmayan
  `AgentUsage` type importu silindi. (Bu oturumun kendi harcaması "Bu oturumun
  harcaması" bölümü **korundu**.)
- `App.tsx`: `SessionDetailPanel`'e geçilen `agentId` ve `onOpenBudget`
  prop'ları kaldırıldı.
- Ajanın günlük harcaması zaten **Bütçe** ekranında tam haliyle duruyor;
  `api.agentUsage` ve `AgentUsage` tipi orada kullanıldığı için korundu.
- `tsc --noEmit` temiz.

## spawned/handoff oturumları sohbet ekranından devam ettirilebilir ✅ (2026-06-26)

Bağlam-reset (handoff) ile açılan oturumlar artık sohbet ekranından konuşulabiliyor.

**Sorun:** handoff komutu yeni session'ı `kind:"spawned"` ile açar (`spawn.go:94`,
`ParentSessionID` ile eski oturuma bağlı). Ama sol sohbet listesi yalnız
`chat`/boş kind'ı gösterdiği için (`App.tsx isChatKind`), handoff/spawn
oturumları yalnız Aktivite (Executions) ekranında salt-okunur görünüyordu —
kullanıcı devam ettiremiyordu. Backend `chat/stream`'de **kind kontrolü yok**;
AgentID dolu olan her oturuma tur çalıştırabiliyor (spawned'da AgentID dolu),
yani tıkanma tamamen frontend kapısındaydı.

**Çözüm (minimal, frontend-only):**
- `App.tsx`: `isChatKind` artık `spawned`'ı da kabul ediyor → spawn ve handoff
  (context-reset) çocukları sohbet listesinde görünür ve composer ile devam
  ettirilebilir. Tek-ajanlı doğrusal transcript oldukları için sohbete uygun.
- `SessionsSidebar.tsx`: spawned oturumlara ayırt edici rozet — `↩ handoff`
  (ParentSessionID varsa) veya `✦ spawn`.
- `schedule`/`flow`/`task` **bilinçli olarak dışarıda** kaldı: bunlar
  paylaşılan/çok-koşulu/çok-ajanlı birikimli loglar; sohbete yazmak otonom
  turlarla karışma + ajan belirsizliği yaratır. Onlar için ileride "Sohbete
  fork" yaklaşımı düşünülebilir (bkz. tasarım tartışması).
- `tsc --noEmit` temiz.

**Not / sıradaki olası iyileştirmeler:** (a) tur-devam-ediyor kilidi (otonom +
manuel tur aynı session'a çakışmasın), (b) manuel devralınan turun bütçe/confine
muafiyetinin netleştirilmesi, (c) spawned oturumların Executions'tan
kaldırılıp kaldırılmayacağı (şimdilik her iki yerde de görünüyor).

## Oturum detay panelinden özet butonları kaldırıldı ✅ (2026-06-26)

Kullanıcı isteğiyle, sohbet detay (oturum bilgisi) panelindeki "Araçlar"
bölümünden dört özet butonu (`Hafıza özeti` / `Görev panosu özeti` /
`Akışlar özeti` / `Araçlar özeti`) kaldırıldı.

- `SessionDetailPanel.tsx`: `SUMMARY_KINDS` sabiti + butonları render eden blok,
  `onSummarize` prop'u ve artık kullanılmayan importlar (`Database`,
  `Workflow`, `Wrench`) silindi.
- `App.tsx`: `SessionDetailPanel`'e geçilen `onSummarize` prop'u kaldırıldı.
- Aynı özetleri tetikleyen **`/` slash komutları** (`/memory`, `/board`,
  `/flows`, `/tools`) `useChatStream.ts` içinde **korundu** — yalnız panel
  butonları kaldırıldı. `tsc --noEmit` temiz.

## Tier ince ayarı: 6 aracın yeniden atanması ✅ (2026-06-26)

Kullanıcı isteğiyle altı aracın tier'ı değişti:

- **eager → NameOnly:** `update_artifact` (create_artifact eager kalır), `deactivate_tools`
- **NameOnly → eager:** `WebFetch` (artık tam şema her tur)
- **Self-mgmt (hidden) → NameOnly:** `handoff_session`, `memory_add`, `send_message`
  (artık katalogda adıyla görünür; pointer 51→48)

- **Registry:** `MarkNameOnly`/`MarkHidden` artık karşılıklı dışlıyor (`delete` ile;
  disjointness, son işaret kazanır) — self-mgmt→NameOnly flip'i için gerekliydi.
- **deactivate_tools incelik:** meta-tool buildRegistry sonunda eklendiği için
  `activate_tools`'un bildiği lazyCat'e elle eklendi (yoksa aktive edilemezdi). CLI'de
  bridge'lenmediğinden `cliLazyBridgeExcluded`'a kondu (CLI katalogunda görünmez).
- **Canlı doğrulama (WS5+WS2):** 6 atama hem native hem CLI yolunda doğru; CLI'de
  handoff/send_message/update_artifact namespaced, deactivate_tools dışlanmış, WebFetch
  eager. Go build+test ✅, tsc ✅.

## Bağlam önizlemesi: eager araçların tam şeması (açılır-kapanır) ✅ (2026-06-26)

Session bağlam önizlemesinde "Araçlar — her tur şema gönderilen" bölümü artık düz isim
chip'leri yerine **açılır-kapanır** (`<details>`) öğeler gösteriyor: ad → genişletince
**tam açıklama + tam JSON input şeması** (her tur gönderilen gerçek payload). Böylece bir
eager aracın her tur ne kadar yer kapladığı birebir görülebiliyor.

- **Backend:** `toolSummary`'ye `inputSchema` alanı eklendi; `handleAgentContext` ve
  `handleSessionContextPreview` eager araçlar için `d.InputSchema`'yı (folded examples dahil)
  doldurur. Lazy araçlarda boş (şema tura girmez).
- **Frontend:** `SessionContextPreview.tools[].inputSchema`; `SessionContextModal` her aracı
  `<details>` ile render eder (ad/summary + açıklama + `JSON.stringify(schema, null, 2)`).
- AgentContextModal eager araçları bilinçli listelemez (değişmedi).
- Canlı (WS5/AGT1): 20/20 eager araç inputSchema döndürüyor. Go build+test ✅, tsc ✅.

## Market detay popup'ı genişledi: zengin pack önizlemesi ✅ (2026-06-26)

Market item'ına tıklayınca açılan detay popup'ı genişletildi (`max-w-lg`→`max-w-2xl`) ve özellikle
**workspace pack'leri** için çok daha fazla detay gösteriyor (yeni payload alanları artık görünür).

- **Genel meta satırı** (her kind): kaynak (📦 Gömülü / 💾 Yerel / 🌐 Uzak + registry adı), kurulu sürüm,
  oluşturma tarihi, `#tag`'ler — açıklamanın altında çip olarak (`PackMeta`).
- **Zengin workspace önizlemesi** (`WorkspacePackPreview`): stat şeridi (Ajan/Akış/Zamanlama/Skill
  sayıları) + **Ajanlar** (avatar, isim, izin/thinking/MCP çipleri, sağlayıcı·model, soul, skill çipleri)
  + **Akışlar** (node-tipi zinciri + branch/parallel rozetleri; lineer `steps` veya tam `graph`'tan)
  + **Zamanlamalar** (cron + agent-key + prompt) + **Gömülü skill'ler** (frontmatter'dan ad/açıklama) +
  board kolonları + yönergeler. Frontend tipleri (`types/market.ts`) yeni payload alanlarıyla genişletildi.
- **Doğrulama:** `tsc` + `vite build` yeşil. Canlı (8090, tarayıcı): Market ▸ Workspaces ▸ "Yazılım
  Geliştirme" → geniş popup; meta (Gömülü·SwarmGo + #template #workspace), stat (4 ajan/2 akış/0 zam/1 skill),
  ajan kartları (read-only/auto + thinking + sw-conventions çipleri), akış zincirleri render edildi.

## Workspace'i şablon olarak publish (seeding'in tersi) ✅ (2026-06-26)

Mevcut bir workspace artık tek tıkla bir **market workspace-pack'ine** dönüştürülebiliyor; paket
hem markette hem workspace oluşturma picker'ında belirir ve ondan yeni workspace üretilebilir
(tam round-trip).

- **Backend:** `handlePublishMarket`'e `KindWorkspace` case + `buildWorkspaceTemplatePayload`
  (seeding'in TERSİ): agent'ları stabil local key'lere eşler, flow graph'larındaki gerçek agent id'lerini
  `tmpl:<key>`'e yeniden yazar (non-lineer yapı korunur), schedule'ları key'e bağlar, **workspace-tier**
  skill'leri (SKILL.md + nested files) gömer, identity/instructions/board'u taşır. **Sırlar/oturum/runtime
  verisi DAHİL EDİLMEZ.** `market.BuildWorkspacePack` (id=`workspace-<slug>`), `slugify`/`uniqueAgentKey`
  yardımcıları. Publish sonrası `s.market.Reload()` (picker server-store'u bayat kalmasın). (`internal/api/market.go`, `internal/market/publish.go`)
- **Frontend:** `WorkspacePanel`'e "Şablon olarak yayınla" bölümü (`api.publishPack('workspace', ws.id)`),
  başarı/hata geri bildirimi. Workspace ▸ Genel sekmesinde, silme danger-zone'unun üstünde.
- **Doğrulama:** `go build/vet/test` + `tsc` yeşil. Canlı round-trip (fresh 8091): `workspace-software`'tan
  oluştur → **publish** → picker'da `workspace-mydevteam` belirdi (6 template) → ondan yeni workspace üret
  → agent richness (read-only/auto/thinking/skills), **2 flow** (branch dahil, `tmpl:` key'leri çözülmüş) ve
  gömülü `sw-conventions/SKILL.md` birebir geri geldi.

## Prompt editörlerine Markdown önizleme + kopyalama ✅ (2026-06-26)

Uygulamadaki prompt/talimat girilen tüm metin alanlarına ortak bir Markdown-farkında
editör eklendi. Tek yeniden kullanılabilir bileşen `common/PromptEditor.tsx`:

- **Toolbar:** Düzenle/Önizleme geçiş düğmeleri (`Pencil`/`Eye`) + tek tık **Kopyala**
  (`Copy`→`Check` 1.5s geri-bildirim, `navigator.clipboard`, güvensiz bağlamda sessiz düşer) +
  **Tam ekran** büyüteç (`Maximize2`/`Minimize2`).
- **Tam ekran modu:** büyüteç tıklanınca `fixed inset-0` overlay'de aynı editör büyür (toolbar +
  önizleme/düzenleme paylaşılır; overlay'de içerik `flex-1` ile ekranı doldurur). Esc veya backdrop
  tıklaması kapatır; açıkken `body` scroll kilitlenir. Hem önizleme hem düzenleme tam ekranda çalışır.
- **Önizleme:** mevcut `markdown/Markdown` bileşeniyle render (GFM, kod blokları, görseller);
  boşsa "Önizlenecek içerik yok." Düzenleme modu bare `<textarea>` korur (resize-y, outline).
- **API:** `value/onChange` + tüm `<textarea>` attribute'leri passthrough (`rows`, `placeholder`,
  `maxLength`, `autoFocus`, `data-testid`). `mono` (monospace), `textareaClassName` (min-height vb.),
  `className` (kapsayıcı). Kendi border/bg/focus stilini taşır → call-site sade.
- **Entegre alanlar (9 alan / 9 call-site):** agent soul + identity (`AgentSettingsForm`), yeni-agent
  soul (`AgentRoster`/`AgentsView`), skill markdown body (`SkillEditor`), flow agent prompt + transform
  template (`NodeInspector`), workspace runtime promptları + instructions (`WorkspaceFilesPanel`),
  background spawn prompt (`SpawnSessionModal`), **core memory blokları** (`CoreMemoryCard` — karakter
  sayacı/usage-bar korunur, `maxLength` passthrough), **oturum hedefi** (`SessionDetailPanel` — accent
  kenarlık `className` ile, Ctrl/Cmd+Enter kaydet & Escape `onKeyDown` passthrough, `maxLength=2000`),
  **kullanıcı tercih notları** (`appPanels` Profil).
- **Doğrulama:** `tsc -b` temiz; eklediğim alanlarda yeni lint hatası yok (dosyalardaki mevcut
  ref/useEffect uyarıları ilgisiz/dokunulmadı). (`frontend/src/components/common/PromptEditor.tsx` + 9 call-site)

## Workspace template kapsamı genişledi: zengin agent + gömülü skill + çoklu/non-lineer flow ✅ (2026-06-26)

`market.WorkspacePayload` artık tam bir başlangıç ekosistemi taşıyor (önceden yalnız identity +
name/soul agent + tek lineer flow):

- **Agent zenginliği:** `WorkspaceTemplateAgent` `AgentPayload` ile hizalandı — provider/model,
  planning/thinking/permission modu, `MCPEnabled`, `AllowedTools`/`BlockedTools`, `Skills[]` atamaları,
  günlük bütçe. Boş provider/model seed'de workspace/app default'una düşer. Seeding tek `db.CreateAgent`
  çağrısında tüm alanları persist eder.
- **Gömülü skill'ler:** `WorkspacePayload.Skills []WorkspaceTemplateSkill{Slug,Body,Files}` — seed'de
  **agent'lardan ÖNCE** workspace skills dizinine yazılır (`market.InstallSkill` sentetik pack ile) ve
  katalog reload edilir, böylece agent `Skills[]` referansları çözülür.
- **Çoklu + non-lineer flow:** tekil `Flow` → `Flows []WorkspaceTemplateFlow`. Her flow ya lineer
  (`Steps`) ya da tam **orchestration graph** (`Graph`, branch/parallel/delay/transform). Graph'ta agent
  düğümleri `agentId="tmpl:<key>"` taşır; seed'de gerçek id'ye çevrilir (`TemplateAgentKeyPrefix`).
  Saf `resolveTemplateFlowGraph` (lineer + graph) birim-test edilir.
- **Seed sırası:** skills → agents → flows → schedules. `seedWorkspaceTeam` hem create-picker hem
  market-install yolunda ortak. (`internal/api/templates.go`, `internal/market/pack.go`)
- **Bundled paketler:** 5 template `flow`→`flows` migrate edildi; `workspace-software` üç yeteneği de
  sergiliyor (gömülü `sw-conventions` skill'i + read-only/auto + thinking=medium agent'lar + ikinci
  **branch'li** flow "Verify → FIX→Execute | SHIP").
- **Doğrulama:** `go build/vet/test` + integrity testi (branch graph dahil) yeşil. Canlı (fresh 8091):
  `workspace-software`'tan oluşturma → agent permission/thinking/skill alanları, `sw-conventions/SKILL.md`
  diske yazıldı, **2 flow** seed edildi (biri `[agent,branch,agent]`, tmpl-key'leri çözülmüş).

## Tool tier UI chip'leri: Self-mgmt vs NameOnly ayrımı ✅ (2026-06-26)

Workspace Tools ekranı artık üç tier'ı ayrı chip ile gösteriyor (önceden hidden ve
nameOnly ikisi de "NameOnly" görünüyordu):

- **eager** → chip yok (her tur tam şema)
- **NameOnly** (amber) → lazy + isimle listelenir (örn. set_session_goal, notify)
- **Self-mgmt** (gri) → hidden tier: katalogda ismi bile yok, `swarmgo-self-management`
  skill pointer'a katlanır, tool_search ile keşfedilir (örn. create_agent… + Part A'da
  taşınan read/write/list_config, secret_list/get)

- **Backend:** `Registry.IsHidden`; `WorkspaceToolCatalogWithState` → `(defs, lazy, hidden)`;
  `workspaceTool.selfManaged` (json) = hidden[name]. **Frontend:** `WorkspaceTool.selfManaged`,
  `SelfMgmtBadge` (ToolsPanel liste + detay).
- **Canlı (WS5):** eager 20 · NameOnly 13 · Self-mgmt 51. Go build+test ✅, tsc ✅.
- Detay: `_Docs\19-LAZY-TOOL-LOADING.md`.

## Workspace template'leri markete taşındı (bundled tier) ✅ (2026-06-26)

Workspace oluşturma picker'ı artık **market'in workspace-pack'lerini** gösteriyor; 5 built-in
template (Boş/Bilimsel Araştırma/Yazılım Geliştirme/Günlük Rutin/Link Kısaltma) hard-coded Go
listesinden çıkarılıp **gömülü market paketlerine** taşındı.

- **Pack şeması genişledi:** `market.WorkspacePayload` artık opsiyonel `agents[] + flow + schedules[]`
  taşıyor (`WorkspaceTemplateAgent/Step/Flow/Schedule`). Önceden yalnız identity+instructions+columns
  vardı → zengin template'leri temsil edemiyordu. (`internal/market/pack.go`)
- **Bundled tier geri geldi (sadece template'ler için):** `internal/market/embed.go` `//go:embed
  defaults/*.swarmpack.json` → `SourceBundled` (en düşük öncelik, global/remote override eder).
  `store.go`: `tier.fsys` + `scanDir`/`Get` embed-FS okuma. 5 paket `internal/market/defaults/`.
- **Picker kaynağı market:** `/api/workspace-templates`'in JSON şekli **aynı kaldı** (frontend modal
  değişmedi) ama kaynağı Server-seviyesi workspace-bağımsız market store (`s.market = market.New(
  agent.MarketGlobalDir(), "")`) — onboarding'de sıfır workspace'te de çalışır. Blank ilk sıraya pinli.
- **Seeding birleşti:** `seedWorkspaceFromTemplate` (picker) + `installWorkspacePack` (market install)
  ortak `seedWorkspaceTeam` ile agent+flow+schedule seed eder. Market'ten kurulan workspace artık
  takımıyla geliyor. Eski `workspaceTemplates`/`templateByID`/`seedTemplate` kaldırıldı.
- **Frontend:** `WorkspaceCreateModal` yüklemede blank'i (market id `workspace-blank`) auto-select eder.
- **Doğrulama:** `go build/vet/test ./...` + `tsc` yeşil; bundled-pack integrity testi (eski in-code
  test yerine). Canlı: fresh 8091 → picker 5 bundled template gösterdi; `workspace-research`'ten
  oluşturma → 4 agent + 1 flow seed; tarayıcıda create-modal picker market template'lerini render etti.

## Tier rafine: admin-nadir araçlar hidden gruba taşındı ✅ (2026-06-26)

NameOnly seti gözden geçirildi. Admin/nadir araçlar her turdan enumerate edilmek yerine
**hidden self-management** grubuna (pointer + skill + tool_search) taşındı:
`read_config`/`write_config`/`list_config` (ajanın kendi promptunu düzenler) +
`secret_list`/`secret_get` (kasa okuma — yazma kardeşleri zaten hidden'dı, tutarlılık).
Pointer metnine "your own prompts/config" eklendi.

- **Karar (Part B = HAYIR):** self-management ailesinin tamamını (46) name-only enumerate
  ETMEDİK. Patlamalı/nadir admin araçları; her tur 46 satır (CLI'de ~600 token) düşük
  getiri. Kategori-pointer + `swarmgo-self-management` skill + tool_search zaten keşfi
  sağlıyor. **Kural:** NameOnly = "var olduğunu bil, ara sıra kullan"; hidden = "toplu/
  nadir admin, per-turn ödeme yok".
- **Kod:** `toolsetup.go` — config+secret-read'ler `MarkNameOnly`'den `MarkHidden`'a.
- `internal/agent` + `internal/tools` derleniyor, testleri geçiyor.
- ⚠️ **Canlı doğrulama bekliyor:** `internal/api` şu an dışarıda süren market/templates
  refactor'ü yüzünden derlenmiyor (`workspace_bridge.go` → `seedTemplate`/`templateByID`
  tanımsız), bu yüzden tam backend rebuild + canlı kontrol o refactor bitince yapılacak.

## CLI-uyumlu "Available Tools" kataloğu (claude-cli namespaced adlar) ✅ (2026-06-26)

SES12 (WS2, claude-cli ajan) incelemesinde fark edildi: "# Available Tools (load on
demand)" bloğu built-in araçları **bare adlarla** (`set_session_goal`, `notify`…) +
native `activate_tools` yönergesiyle listeliyordu. Ama claude-cli bu araçları MCP aracı
olarak (`mcp__swarmgo_interaction__*`) görür ve kendi ToolSearch'üyle yükler — yani blok
yanıltıcıydı (skills bloğu zaten doğru namespaced biçimi kullanıyordu, tools bloğu değil).
Default-NameOnly değişikliği 10 built-in'i daha bu bloğa eklediği için fark belirginleşti.

- **Düzeltme:** `LazyToolsCatalogBlock` artık ajanın `provider`'ına göre dallanır.
  claude-cli formunda: built-in → `mcp__swarmgo_interaction__<ad>`, MCP → `mcp__<server>__<tool>`,
  yönerge `ToolSearch` (native `activate_tools` değil), CLI-native built-in'ler (WebFetch)
  düşürülür. Native (anthropic/minimax) form **değişmedi** (bare ad + activate_tools).
- **Kod:** `catalogDisplayName` ad eşlemesi + `renderLazyToolCatalog(..., cli bool)` +
  `writeLazyToolLine(name, desc)`. `cliLazyBridgeExcluded` = tools.bridgeExcluded ayna.
- **Not (önemli):** Bu davranış değişikliği değil bir **netleştirme** — eskiden de
  çalışıyordu (model namespaced adı kendi çıkarıp ToolSearch'lüyordu, SES12'de görüldü),
  ama artık blok doğru adları + doğru yöntemi söylüyor.
- **Canlı doğrulama:** WS2/AGT4 (cli) → namespaced + ToolSearch, WebFetch yok; WS1/AGT2
  (minimax/native) → bare + activate_tools, WebFetch var. Test:
  `TestLazyCatalogCLIFormNamespacesNames`. Go build+test ✅. Detay: `_Docs\19-LAZY-TOOL-LOADING.md`.

## İlk-yükleme (onboarding) akışı + splash ekranı ✅ (2026-06-26)

Taze kurulumda (hiç workspace yokken) artık **otomatik "Varsayılan" workspace
oluşturulmuyor**; bunun yerine kullanıcı bir splash + workspace-oluşturma popup'ı
ile karşılanıyor. Popup kapatılırsa **hiçbir varsayılan kurulum yapılmaz** — uygulama
boş karşılama ekranında bekler.

- **Backend:** `workspace.NewManager` içindeki "len(order)==0 → Create(\"Varsayılan\")"
  bloğu kaldırıldı; manager sıfır workspace ile açılabilir. Sıfır workspace'te API'nin
  `ws(r)` çözümlemesi workspace-scoped rotalarda `nil` döner; frontend bunları aktif
  workspace arkasına gate'ler, `withRecover` stray çağrıyı 500'e çevirir (panik yok).
  (`internal/workspace/manager.go`)
- **Frontend gate:** `useWorkspaces` artık `loading` bayrağı taşır (ilk liste çözülene
  kadar `true`). `App.tsx` tüm hook'lardan SONRA gate'ler:
  `loading && !hadSetupAtBoot` → `SplashScreen`; `!loading && workspaces.length===0` →
  `OnboardingScreen`. `swarmgo.hasSetup` localStorage bayrağı bir workspace var olunca
  set edilir → splash yalnız **taze kurulumda** (kurulum yapılmamışken) görünür, dönen
  kullanıcı doğrudan uygulamaya girer.
- **Yeni bileşenler:** `components/SplashScreen.tsx` (self-contained, tema-değişkenli),
  `components/workspace/OnboardingScreen.tsx` (karşılama kartı + mevcut
  `WorkspaceCreateModal`'ı açar; popup başta açık, kapatılırsa kurulum yapılmaz).
- **Splash marka + min-süre (2026-06-26):** Splash gerçek logoyu (`/favicon.svg` — mor gradyan
  swarm markası, hem Vite dev hem embed binary'de servis ediliyor) dönen aksan halkası +
  "SwarmGo" wordmark + "Yükleniyor…" ile gösterir (logoda pulse + ekran fade-in keyframe'leri
  bileşene gömülü). **Minimum görünme süresi** `SPLASH_MIN_MS=1100` (App.tsx): liste anında
  çözülse bile splash en az bu kadar kalır → tek-kare flaş olmaz. Gate:
  `!hadSetupAtBoot && (wsLoading || !minSplashElapsed)`; dönen kullanıcı min-süreyi de atlar.
  Canlı doğrulandı: min geçici 4s'ye çekilip Playwright ekran görüntüsüyle logo+halka+wordmark
  teyit edildi, sonra 1100'e döndürüldü.
- **Son workspace silinince onboarding'e dönüş (2026-06-26):** `Manager.Delete`'teki
  "son workspace silinemez" guard'ı **kaldırıldı** — silince manager sıfır workspace'e
  düşer ve `persist()` `workspaces.json`'a `[]` yazar (reboot'ta yine onboarding). Frontend:
  yeni `clearActiveWorkspace()` (api/client) aktif işaretçiyi temizler; `useWorkspaces`'teki
  `deleteWorkspace`/`deleteActiveWorkspace` "son" guard'larını kaldırdı, kalan yoksa
  `activeWorkspaceId=null` → App gate `workspaces.length===0` ile **canlı (reload'suz)**
  onboarding'e döner. Son workspace için onay metni farklı: "… son workspace — silinince ilk
  kurulum ekranına dönersin."
- **Doğrulama:** `go build ./...` + frontend `tsc --noEmit` yeşil. Taze `SWARMGO_DATA_DIR`
  ile canlı test (8091): `GET /api/workspaces` → `count=0` + `workspaces.json` yok (otomatik
  oluşturma gerçekten kalktı), ardından `POST /api/workspaces` → WS1 oluştu, `count=1`.
- **Canlı UI smoke testi (2026-06-26, tek-binary 8091 + Playwright):** taze örnekte tarayıcı
  `http://127.0.0.1:8091` → **onboarding + create popup** render edildi (snapshot doğrulandı);
  ad girip **Oluştur** → URL `#/w/WS1/chat`, uygulama yüklendi; Workspace ▸ "Workspace'i sil"
  → yeni "son workspace" onay metni çıktı → kabul → **onboarding canlı geri döndü** (popup
  yeniden açıldı), `GET /api/workspaces` → `[]`. Gerçek veri (`~/.swarmgo`, 4 ws) izole tutuldu.

## Skill NameOnly: skill'ler için slug-only katman ✅ (2026-06-26)

Araçlardaki NameOnly mekaniğinin skill muadili. Frontmatter `name_only: true` ile
işaretlenen skill, "# Available Skills" bloğunda **yalnız slug** olarak listelenir
(`- \`slug\``); açıklama+when bastırılır. Skill **listede kalır** — model varlığını
görür, detayı `skill_search` ile keşfeder, `use_skill` ile yükler. "Tam özet" ile
"tamamen düşür" (`auto_summary:false`/`paths`) arasındaki eksik orta katman.

- **Default KAPALI, skill-başına opt-in.** Araçlardaki gibi küratörlü default-açık
  set YOK: skill'lerde açıklama ana tetikleme sinyali olduğundan toptan kaldırmak
  keşfi zayıflatır (bilinçli tasarım kararı).
- **Backend:** `Skill.NameOnly` alanı; `isNameOnly` parse (`name_only`/`nameonly`);
  `renderCatalog` slug-only satır + footer `skill_search` yönlendirmesi;
  `Store.SetNameOnly` + `setFrontmatterNameOnly` (`auto_summary` desenini yansıtır);
  `PUT /api/skills/{slug}/name-only` (`handleSetSkillNameOnly`).
- **Frontend:** `Skill.nameOnly` tipi, `api.setSkillNameOnly`, SkillsPanel "NameOnly"
  badge + toggle butonu (liste + detay).
- **Etki (canlı ölçüm, WS5):** `swarmgo-autonomous-ops` NameOnly → satır **839→26
  karakter**, Available Skills bloğu **3376→2563** (~813 karakter ≈ ~200 token, tek skill).
- Test: `TestNameOnlySkillRendersSlugOnly`. Go build+test ✅, `tsc` ✅. Canlı
  toggle on/off doğrulandı, config geri alındı. Detay: `_Docs\19-LAZY-TOOL-LOADING.md`.

## Default NameOnly seti: built-in araçlar için varsayılan ✅ (2026-06-26)

Kullanıcı isteğiyle projedeki built-in araçlar tarandı ve **küçük, kendini açıklayan,
turların azınlığında kullanılan** bir grup, **kod varsayılanı** olarak NameOnly yapıldı
(`toolsetup.go`'daki eski `MarkLazy` bloğu `MarkNameOnly` ile değiştirildi). Kod
varsayılanı olduğu için **tüm workspace'lere (mevcut WS1/2/4/5 + yeni) otomatik** uygulanır;
per-workspace config yazmaya gerek yok.

- **Önceden eager → NameOnly (10 araç, her turdan şema kalktı):** `set_session_goal`,
  `complete_goal`, `set_session_title`, `set_working_dir`, `archive_session`, `notify`,
  `focus_view`, `schedule_wake`, `list_sessions`, `conversation_search`, `read_session_debug`.
- **Önceden lazy+özet → NameOnly (özet satırı kalktı):** `read_config`, `write_config`,
  `list_config`, `secret_list`, `secret_get`, `WebFetch`, `memory_recall`.
- **Eager kalan** (davranışsal/sık): `todo_write`, `ask_user`, `request_confirmation`,
  `create_artifact`/`update_artifact`, `core_memory_*`, `use_skill`/`skill_search`,
  `run_subagent`, `Read`/`Write`/`Edit`/`list_dir`/`Glob`/`Grep`, `shell`. Self-management
  ailesi `MarkHidden` kalır.
- **Çalıştırma etkilemez:** `Registry.Call` aracı lazy/nameOnly'den bağımsız çalıştırır;
  aktivasyon yalnız şema gönderimini etkiler (schedule_wake e2e testi bozulmadı).
- **Ölçüm (WS5/AGT1):** eager 30→20 araç, eager şema **~5931→3857 token** (≈ turn/agent
  başına **~2074 token** tasarruf). 4 workspace'te de canlı doğrulandı.
- Testler: `go test ./internal/agent ./internal/tools ./internal/e2e` geçti.
  Detay: `_Docs\19-LAZY-TOOL-LOADING.md`.

## Tool görünürlüğü: "NameOnly" katmanı (Claude Code deferred-tool stili) ✅ (2026-06-26)

Kullanıcı isteğiyle workspace Tools ekranındaki **"Gizle" çipi "NameOnly" oldu** ve
davranışı değişti. Eskiden "Gizli" işareti aracı `MarkLazy` ile **ad+özet** satırı olarak
gösteriyordu; artık `MarkNameOnly` ile **yalnız adıyla** listeleniyor (özet bastırılır) —
Claude Code'un "deferred tool" mekaniğinin muadili. Model adı görür, ne yaptığını
`tool_search` ile keşfeder, şemayı `activate_tools` ile çeker.

- **Registry:** yeni `nameOnly` set + `MarkNameOnly` (implies lazy). `nameOnly ⊆ lazy`,
  `hidden`'dan ayrık. `VisibleLazyCatalog` NameOnly araçların `Description`'ını boşaltır;
  `LazyCatalog` (activate/search kaynağı) açıklamayı korur. `Unlazy` ("Göster") nameOnly'yi
  de temizler. (`internal/tools/registry.go`)
- **Render:** `renderLazyToolCatalog` boş özetli satırı `- \`ad\`` olarak basar
  (`writeLazyToolLine`). Workspace `HiddenTools` → `MarkNameOnly` (`toolsetup.go`).
  Veri modeli (`HiddenTools`/`hidden`) **değişmedi** — yalnız sunum + UI etiketi.
- **UI:** `ToolsPanel.tsx` çip "NameOnly", buton/tooltip metinleri güncellendi.
- **Kapsam:** native yolda (anthropic/minimax) token kazandırır; **claude-cli yolunda
  etkisiz** (bridge tam şema ilan eder, MCP'ler zaten CLI deferral'ına tabi).
- Test: `TestMarkNameOnlyKeepsNameDropsSummary`. Detay: `_Docs\19-LAZY-TOOL-LOADING.md`.

## Ajan araç erişimi: allowlist → denylist + skill sıralaması kaldırıldı ✅ (2026-06-26)

Kullanıcı geri bildirimiyle ajan-düzeyi araç yönetimi modeli sadeleşti:

1. **Ajan araçları artık denylist.** Eskiden ajan-başına **allowlist** (`Agent.AllowedTools`,
   boş = hepsi) vardı; kısmî seçim yapınca sonradan workspace'e eklenen araçlar ajana
   gelmiyordu. Yeni model: **her ajan varsayılan olarak TÜM workspace-aktif araçlara erişir**;
   agent detayından istenen araçlar **engellenir** (`Agent.BlockedTools`, JSON denylist,
   boş = hiçbiri engelli). Sonradan eklenen araçlar otomatik erişilebilir kalır.
   - `toolFilter` artık üç katmanı besteliyor: workspace denylist ∪ ajan denylist çıkarılır,
     ardından (varsa) legacy allowlist daraltır.
   - **Legacy allowlist korunuyor** ama yalnız built-in **subagent profilleri**
     (explore/coder/reviewer izolasyonu, `subagent.go`) için — kullanıcı ajanları boş bırakır.
   - **Otomatik migrasyon:** eski allowlist'i olan bir ajanda `GET /api/agents/{id}/tools`
     allowlist'i eşdeğer denylist'e çevirip döner (UI doğru efektif seti gösterir); ilk
     kaydetmede (`UpdateAgentTools`) allowlist temizlenir, ajan tamamen denylist ile tanımlanır.
   - UI (`AgentToolsSection`): checkbox işaretli = araç açık; işareti kaldır = bu ajanda engelle.
     "Hepsi" = denylist'i temizle, "Hiçbiri" = tümünü engelle.

2. **Skill sıralaması kaldırıldı.** `AgentSkillsSection`'daki yukarı/aşağı (move) butonları ve
   sıra numarası kaldırıldı — skill seçimi artık sırasız bir **küme**. `Agent.Skills` listesi
   ekleme sırasını korur ama kullanıcı sıralamaz; ilgili metinler güncellendi.

**UI rötuşu (2. tur):** Araçlar bölümü tamamen denylist odaklı yeniden tasarlandı
(`AgentToolsSection`) — eskiden 118 aracı işaretli liste yerine artık başlık "Yasaklı Araçlar";
yasaklı araçlar kırmızı çip listesi (kaldır = X), altında yasaklamak için aranabilir ekleme listesi;
"Yasakları temizle" / "Tümünü yasakla" kısayolları. **Yeni ajan butonu** sohbet ekranındaki
"Yeni Sohbet" butonu gibi belirgin tam-genişlik `+ Yeni Ajan` butonu oldu (`AgentRoster` +
`AgentsView`; eski küçük "+" toggle kaldırıldı). Frontend `tsc --noEmit` + `npm run build` temiz.

Backend `go build`/`vet` + agent/db/api testleri (185) + frontend `tsc --noEmit` temiz.

## Oturum detayı UI rötuşları + per-session kalıcı ilerleme ✅ (2026-06-26)

Kullanıcı geri bildirimiyle 4 iyileştirme:
1. **Kalıcı ilerleme artık oturum-başına.** Eskiden proje dizini olmayan oturumlar
   workspace-default `<cwd>/.swarmgo/progress.json`'u paylaşıyordu → detay-panelde
   "ajan-bazlı" görünüyordu. Tek resolver **`Runtime.ProgressDir(sessionID)`**:
   explicit proje dizini varsa onu (cross-session paylaşım korunur), yoksa
   per-session fallback `<store>/progress/<sessionID>`. Yaz (NewTodoSink, artık
   2-arg) + oku (resume bloğu + detay-panel) hepsi tek resolver'dan; api'deki
   `progressDir` kaldırıldı. Test güncellendi (`todosink_test` per-session izolasyon).
2. **Debug / Gözlemlenebilirlik kartı katlanabilir** (`SessionDebugCard`): başlık
   tıkla-aç/kapa (chevron), kapalıyken tek-satır özet (tur + uyarı sayısı), seçim
   localStorage'da kalıcı (vars. kapalı).
3. **"Özete çevir" komutları "Araçlar" butonları gibi** — grid yerine tam-genişlik
   `ActionBtn` satırları (ikon + etiket: Hafıza/Görev panosu/Akışlar/Araçlar özeti).
4. **Klasör kartı kaldırıldı; "Yolu kopyala" + "Aç" butonları sohbet header'ına**
   "Detay"nın yanına taşındı (App.tsx; path metni artık panelde yazmıyor).

Build + vet + 161 test (agent/api/progress) + frontend `tsc -b` temiz.

## Dizin-sitesi: canlı arama + source-ref önizleme ✅ (2026-06-26)

Connector'lar "statik katalog"tan **canlı arama**ya geçti + source-ref önizleme eklendi
(`_Docs\39`):

- **Canlı arama:** siteler binlerce skill barındırıyor → toplu yükleme kaldırıldı.
  `market.SearchConnectors(q)`: **skillsmp** gerçek arama API'si (`/api/v1/skills/search`),
  **crossaitools** tüm listeyi (~12MB) bir kez cache'leyip (`crossaitools-lite.json`, 24s TTL)
  lokal filtre (`filterCrossAITools`). `RefreshRemote`/`loadRemoteCache` connector'ları atlar.
  API `GET /api/market/connectors/search?q=`. UI: Market arama kutusu 2+ karakterde debounced
  sorgular → "İnternet sonuçları" bölümü.
- **Önizleme:** `ingest.Preview` source-ref için GitHub'dan SKILL.md çekip render eder;
  `POST /api/ingest/preview`; detay modalında `SourceRefPreview` (kaynak linki + gövde).
  Install yine `ingest.BuildPacks` ile.
- **Bugfix:** `req()` istemcisi 204/boş gövdeyi artık tolere ediyor (eskiden "Unexpected
  end of JSON input" → katalog yenilenmiyordu).
- **Doğrulama:** my-paketler **47 test** + tsc temiz; canlı skillsmp/crossaitools arama OK.
  Not: çalışma ağacında **paralel iş** (`App.tsx`, `todos_test.go`) yarım olduğundan tam
  prod build/`go test ./...` o dosyalarda kırık — benim değişikliklerim ayrık ve yeşil.
- **Kalan:** source-ref "Kuruldu" cross-session; claudeskillsmarket scrape; sayfalama. `_Docs\39`.

## Debug günlüğü — claude-cli köprü düzeltmesi + canlı E2E doğrulama ✅ (2026-06-26)

Canlı test (gerçek claude-cli LLM, izole datadir, 8097) sırasında bulunan ve
düzeltilen gerçek bir eksik: `read_session_debug` aracı yalnız native registry'de
kayıtlıydı; **claude-cli ajanı interaction köprüsünde göremiyordu** (köprü yalnız
`reg.BridgeableDefs` + birkaç açık `extra` builtin sunar). Düzeltme:
- `Runtime.BridgeTools`'ta `extra` listesine `read_session_debug` eklendi
  (`conversation_search` gibi, yalnız `r.db` gerektiren eager builtin; gated
  `DebugJournalEnabled`). Köprü çağrı closure'ı build ctx'inden yakalanan oturum
  id'yi `tools.WithCurrentSession` ile enjekte eder (interaction request ctx'inde yok).
- Araç hidden-lazy self-manage tier'ından **çıkarılıp** core/her-zaman-açık yapıldı
  (toolsetup, `conversation_search` yanında, `DebugJournalEnabled` gated) → native
  yolda da eager.
- **Canlı sonuç:** ajan `mcp__swarmgo_interaction__read_session_debug`'i araç
  listesinde gördü, çağırdı ve `turns=2, llmCalls=2, anomalies=0` raporladı; API
  ground-truth tur bitince `turns=3` (fark beklenen: araç tur-içinde çağrıldı).
  Özet/seri/cache muhasebesi (read 41956 / write 67146) gerçek veriyle doğrulandı.

## Goal mekanizması — otonom enjeksiyon + cached ipucu ✅ (2026-06-26)

Goal'ün iki eksiği kapatıldı:
- **Otonom turlar artık goal'ü görüyor:** `goalContextBlock` yalnız chat'te enjekte oluyordu;
  scheduler/spawn/peer turları görmüyordu. Renderer agent paketine taşındı
  (`agent.GoalContextBlock`, api ince alias). Yeni `Runtime.autonomousGoalBlock(ctx)` ctx'teki
  sessionID'den (WithSessionID) goal'ü çözüp `invokeTraced`'de `SystemDynamic`'e enjekte eder
  (goal yoksa/spawn'da boş = no-op).
- **Cached prefix ipucu:** her ajan (chat+otonom) statik prefix'inde tek satır
  `agent.GoalUsageHint` ("uzun çok-turlu işte `set_session_goal` ile north-star koy…") →
  proaktif goal kullanımını dürtüyor (araçlar zaten eager; ipucu skill yüklemeden görünür).
  **Kök neden notu:** **iki** `buildSystemPrompt` var (`api/chat.go` chat+preview,
  `agent/runtime.go` otonom/flow); ikisi de aynı export `agent.GoalUsageHint`'i ekler (drift yok).
- **Test:** `TestAutonomousGoalBlock` (goal→render, done→boş, session-yok→boş) + `TestSystemPrompt…`
  (ipucu var); izole instance'ta chat preview'de ipucu + goal bloğu canlı doğrulandı.
  `go test` (agent/api/db) yeşil.

## Debug günlüğü Faz 3 — anomali + zaman serisi + reflektör self-improvement ✅ (2026-06-26)

Oturum debug günlüğünün üstüne üç yetenek:
- **Anomali tespiti:** `db.computeDebugAnomalies` (saf/no-I/O) `GetDebugSummary`'ye
  `anomalies []DebugAnomaly` ekler — `tool_time_dominant` (≥%60 araç süresi),
  `tool_failing` (>%30 hata), `tool_large_output` (>64KB ort.), `frequent_compaction`
  (≥3), `error_burst` (≥3), `slow_turns` (>45s ort.). severity+code+Türkçe mesaj.
- **Zaman serisi:** `turnDurSeries` + `tokenSeries` (en yeni 40 nokta) → UI'da
  bağımlılıksız SVG sparkline (`Sparkline` bileşeni `SessionDebugCard`'da).
- **Reflektör entegrasyonu:** `reflect()` → `r.debugPerfNotes(agentID)` ajanın son
  ≤5 oturumunun dedup'lı anomalilerini reflect prompt'una "Performance observations"
  olarak ekler → kalıcı reflection belleğine ders olarak yedirilir (gated, best-effort).
- UI: kartta anomaliler (warn=kırmızı/info=gri) + sparkline'lar; tool `read_session_debug`
  ve API özeti otomatik içerir. Skill `swarmgo-self-debug` + `_Docs\38` güncellendi.
- Test: `db/debug_journal_test.go` +2 (`AnomaliesAndSeries`, `NoAnomaliesOnHealthy`).
  Build + vet + frontend `tsc` temiz.

## Oturum debug günlüğü (paralel gözlemlenebilirlik akışı) ✅ (2026-06-26)

Her oturum için `session.jsonl`'in yanına append-only **`debug.jsonl`** yazılır:
yapılandırılmış olaylar — `turn` (süre+stop+hata), `llm_call` (model+in/out/cache
token), `tool` (gecikme+boyut+hata), `hook` (karar+süre), `error`, `compaction`,
`recovery`. Amaç: debug, token/gecikme optimizasyonu ve **ajanın kendi kendini
geliştirmesi** (kendi metriklerini okuyup davranış ayarı).

- **DB katmanı** (`internal/db/debug_journal.go`): `DebugEvent` + `AppendDebugEvent`
  (inflight deseni, store-kilitsiz O(1) append, tembel cap budama `cap+cap/4` →
  en yeni `cap` atomik rewrite; satır sayacı `DB.debugCount` kendi mutex'iyle) +
  `ReadDebugEvents` (tip filtresi) + `GetDebugSummary` (turlar/token/`byTool`/
  `byModel`/`topTools`/hata/compaction). `DeleteSession` RemoveAll'ı dosyayı da siler.
- **Emit hunisi** (`internal/agent/debugjournal.go` `emitDebug`): oturum+köken
  ctx'ten; `toolloop.go` (`completeTraced` wrapper turu zamanlar → tüm yollar;
  native döngü tool timing + compaction/recovery + `fail` error), `budget.go
  RecordUsage` (her provider çağrısı), `hooks.go` (her Pre/Post hook + `hookDecisionLabel`).
- **Okuma:** ajan aracı **`read_session_debug`** (özet/ham, self-management suite,
  `read_logs` yanında; mevcut oturum `tools.CurrentSessionID` ile) · API
  `GET /api/sessions/{id}/debug` (`?summary=0&type=&limit=`) · UI ayrı bileşen
  `SessionDebugCard.tsx` (metrik ızgarası + sağlık rozetleri + en yavaş araçlar +
  modele göre token + tembel ham olay log'u), oturum detayında harcama kartından sonra.
- **Ayar:** `debugJournalEnabled` (vars. açık) + `debugJournalCap` (vars. 5000);
  `Tunables.SetDebugJournal`, `applySettings` canlı uygular; UI Ayarlar ▸ Uygulama.
- **Default skill** `swarmgo-self-debug` (ajana metriklerini optimize için nasıl
  okuyacağını öğretir). **Test** `db/debug_journal_test.go` (round-trip + cap budama).
- Detay: `_Docs\38-SESSION-DEBUG.md`. Build + `go vet` + 311 test (5 paket) + frontend `tsc` temiz.

## Otomatik galeri gruplama + video ✅ (2026-06-26)

İki ekleme: (1) **Ardışık `![]()` otomatik gruplama** — `Markdown.tsx::groupMediaRuns`
art arda gelen (aralarında boş satır olabilen) tam-satır `![]()` medya satırlarını
**tek ```gallery bloğuna** dönüştürür (fence-farkında: kod blokları korunur; tek
medya/satır-içi medya inline kalır). (2) **Video desteği** — `lib/paths.ts`'e
`isVideoPath`/`isMediaPath` (mp4/webm/ogg/mov/m4v); tekil `![alt](klip.mp4)` inline
`<video controls>` olur, galeri item'ı video ise thumbnail `<video>` + **play ikonu**
ve Lightbox'ta `<video autoPlay controls>` (kendi kontrolleri pan'ı çalmaz). `Lightbox`
`LightboxImage.type` ('image'|'video'), `Gallery` ext'ten tip türetir. Guidance +
`swarmgo-guide` güncellendi. Playwright doğrulaması: 2 görsel+1 video ardışık → tek
galeri (1 video thumbnail), tek video satırı → inline player, metinle ayrılmış tek
görsel → gruplanmadı. Binary :8090 restart.

## Sohbette görsel + galeri (the external agent project tarzı) ✅ (2026-06-26)

Mesaj akışında görsel gösterimi: (1) tekil `![alt](yol)` markdown görseli zaten
inline render olur (yerel yollar `/api/files` ile, tıkla→Lightbox zoom); (2) **çoklu
görsel için ```gallery bloğu** (`Gallery.tsx`, alias `image-preview`/`images`) —
gövde JSON `{"title","images":[{"src","alt"}]}` veya düz satır/virgül-ayrık yol
listesi → **thumbnail grid**, tık→**Lightbox o index'te** açılır, ←/→ + ok butonları +
"n / N" sayaç ile gezinilir. `Lightbox` `images[]`+`index` desteğiyle genişletildi
(`go(delta)` sarmalı navigasyon, ArrowLeft/Right). `CodeBlock` `gallery`/`image-preview`/
`images` → `Gallery`. Ajan guidance'ı (`artifactDeliverableGuidance` + `swarmgo-guide`
Rich replies) inline görsel + galeri bloğunu öğretecek şekilde güncellendi. Playwright
ile grid + index'li açılış + ileri-geri navigasyon doğrulandı; binary :8090 restart.

## Paylaşılan Lightbox (zoom + pan) ✅ (2026-06-26)

Görsel/diyagram önizlemeleri için tek paylaşılan **tam-ekran zoom+pan** bileşeni
(`frontend/src/components/common/Lightbox.tsx`): tekerlekle imlece-doğru zoom
(translate düzeltmeli), sürükle-pan, toolbar (uzaklaştır/%/yakınlaştır/sıfırla/kapat),
çift-tık toggle, Escape + `0` reset, temiz-tık backdrop kapatma (pan'dan sonra kapanmaz).
`imageSrc` ya da `children` (mermaid SVG) alır. **Kullananlar:** mermaid Expand (eski
statik overlay yerine), Markdown inline görselleri (tıkla→zoom, `cursor-zoom-in`),
`UserBubble` attachment önizlemesi (yerel `ImageLightbox` kaldırıldı → DRY),
`ArtifactView` görsel artifact (`ImageArtifact` alt-bileşeni). Playwright ile görsel
zoom (%125) + mermaid Expand gerçek tarayıcıda doğrulandı; binary :8090'da restart edildi.

## Dizin-sitesi köprüsü: crossaitools connector + market arama ✅ (2026-06-26)

`_Docs\38` Faz C+D:

- **Faz C — crossaitools connector:** `market/connectors.go::fetchCrossAITools`.
  crossaitools.com `/api/skills` tüm kataloğu (**~12 MB**, site `?q`/`?limit` yok sayar)
  tek çağrıda döner → **popülerliğe göre (stars, sonra installs) top-300** kırpılır
  (`crossaitoolsTopN`); `repo`+`path` → GitHub tree URL'i (`githubTreeURL`, root path →
  bare repo). `maxConnectorBytes=24MB`. Canlı: 21.7k → 300 source-ref entry.
- **Faz D — market arama kutusu:** `MarketPanel` header'ına `query` state + client-side
  filtre (name/description/author); büyük kataloglar (crossaitools 300) için kullanılabilirlik.
- **Test:** `connectors_test.go` (sort/cap/skip + githubTreeURL) + canlı `TestLiveCrossAITools`
  (21.7k→300). **480 test**, vet/tsc/prod build temiz.
- **Kalan (planlı):** source-ref "Kuruldu" cross-session işaretleme; crossaitools server-side
  live search; claudeskillsmarket scrape; harici köprü generator. Detay: `_Docs\38`.

## Dizin-sitesi köprüsü: kaynak-ref + skillsmp connector ✅ (2026-06-25)

Dizin-sitelerini market'e bağlama (`_Docs\38`) Faz A+B uygulandı:

- **Faz A — kaynak-ref (source-ref) çekirdeği:** `market.RegistryEntry.Source`
  (`SourceRef{Type,URL,Keys}`) + `Pack.SourceRef`; bir kayıt pack indirme yerine bir
  **GitHub kaynağına** işaret eder. Install yönlendirmesi (`installSourceRefPack`):
  kaynak-ref ise **mevcut `ingest.BuildPacks` + `installPackInto`** çalışır (import
  dialog'uyla aynı hat). `Store.Get` kaynak-ref'i payload indirmeden manifest olarak
  döner; `loadRemoteCache` URL'siz ama Source'lu entry kabul eder.
- **Faz B — skillsmp connector:** `market/connectors.go` skillsmp.com `/api/skills`
  cevabını **source-ref entry'lerine** çevirir (her skill'in `githubUrl`'i). `Registry`
  artık `Connector` taşır; `RefreshRemote` connector dalı site API'sini swarmregistry
  index'i gibi cache'ler. API `GET /api/market/connectors` + `POST .../connectors/add`;
  UI `RegistryManager`'da "Hazır kaynaklar" quick-add (skillsmp tek tıkla eklenir).
- **Canlı:** skillsmp 12 source-ref entry. **478 test**, vet/tsc/prod build temiz.
- **Kalan (planlı):** crossaitools connector (21.7k, arama/sayfalama), market arama
  kutusu + cross-session "Kuruldu", claudeskillsmarket scrape, harici köprü. Detay: `_Docs\38`.

## Ingest: CC model eşleme + dizin-sitesi köprü planı (2026-06-25)

- **CC model → SwarmGo provider/model eşleme** ✅: `agentAdapter` artık CC subagent
  `model:` değerini eşliyor (`mapCCModel`): aile anahtar kelimesi → keysiz `claude-cli`
  provider + kanonik model id (`opus`→`claude-opus-4-8`, `sonnet`→`claude-sonnet-4-6`,
  `haiku`→`claude-haiku-4-5-20251001`, `fable`→`claude-fable-5`). `inherit`/boş → workspace
  varsayılanı; tanınmayan (gpt/gemini vb.) → boş + uyarı. İçe aktarılan ajan artık **kutudan
  çıktığı gibi** çalışır (eskiden model uyarıyla atlanıyordu). Test: `TestMapCCModel`,
  `TestAgentModelMappedIntoPack`.
- **Dizin-sitesi köprüsü (PLAN)** 📋: crossaitools/skillsmp/claudeskillsmarket'i market'e
  bağlama tasarımı `_Docs\38-DIZIN-SITE-REGISTRY-PLAN.md`'ye yazıldı. Bulgu: üçü de
  GitHub-tabanlı (crossaitools `/api/skills` ~21.7k `repo`+`path`; skillsmp `githubUrl`
  doğrudan; claudeskillsmarket yalnız sitemap). Çekirdek değişiklik: `RegistryEntry.Source`
  (kaynak-ref) → kurulum = mevcut `ingest.BuildPacks` + `installPackInto`. Henüz uygulanmadı.

## Interaction MCP Faz 3 KAPANDI — oturum metadata araçları + goal canlı-refresh ✅ (2026-06-26)

Faz 3'ün son artığı bitti: ajan **kendi oturumunu** düzenleyebiliyor + agent-set goal/metadata
UI'da **canlı** yansıyor.

- **Yeni araçlar (bloklamayan, oturum-bağlı):** `set_session_title(title*)` (yeniden adlandır,
  maxlen 200), `set_working_dir(path*)` (cwd; `os.Stat` var-olan-dizin doğrulaması, boş=reset,
  sonraki tur), `archive_session()` (aktif listeden düşür, silme yok).
- **Konsolide sink:** `tools.SessionSink` = `GoalSink` superset'i; tek `agent/sessionSink`
  (goalsink.go→sessionsink.go) hepsini uygular, `Runtime.NewSessionSink`. Goal araçları dokunulmadı.
- **Canlı refresh:** her mutasyon (goal dahil) `events.Event{Type:"session"}` yayınlar →
  `App.tsx` `session` event'i: `refreshSessions` + aktif oturumsa `meterRefresh` bump (detay panel
  goal/başlık kartı canlı). Toast yok. → önceki fazın "goal kartı güncellenmiyor" eksiği de kapandı.
- **DB:** `SetSessionState` (archive) eklendi.
- **Test:** 7 yeni birim test (sessionedit); tools/agent/api/db + tsc/vite yeşil. Canlı claude-cli
  doğrulandı. Detay: `_Docs\11` §20.

**FAZ 3 TAMAMEN KAPANDI** — notify + focus_view + goal + oturum metadata + canlı-refresh hepsi
native+CLI+otonom. Kalan: Faz 4 (Codex/Gemini).

### Arşiv UI (2026-06-26)
`archive_session`'ın UI karşılığı: sidebar artık state'e göre filtreliyor. **Backend:**
`PUT /api/sessions/{id}/state` (`handleSetSessionState`, geçersiz state 400). **Frontend:**
`SessionsSidebar`'da **Aktif / Arşiv (n)** toggle + satır menüsünde Arşivle/Arşivden-çıkar;
`App.setSessionArchived` → `api.setSessionState`. Canlı: PUT state (active↔archived, 400-reddi)
doğrulandı. `tsc`/`go test` yeşil. Detay: `_Docs\11` §20.1.

## Interaction MCP Faz 3 — `set_session_goal` / `complete_goal` (oturum hedefi) ✅ (2026-06-26)

Ajan artık oturumun kalıcı **north-star hedefini** kendisi koyabilir/tamamlayabilir —
**mevcut `db.Session.Goal`/`GoalDone` paylaşımlı** (ayrı ajan-goal açılmadı; kullanıcı UI'da
aynı alanı düzenliyor). Daha önce ajan hedefi `goalContextBlock` ile **görüyor** ama yazamıyordu.

- **Araçlar (`tools/builtin_goal.go` + `goalsink.go`):** `set_session_goal(goal*)` (Goal yaz,
  done=false; üzerine yazınca "replaced previous goal" şeffaf bildirimi; maxlen 2000) +
  `complete_goal()` (metni koru, done=true; hedef yok/zaten-done graceful). Sink yoksa no-op.
- **Concrete sink (`agent/goalsink.go`):** `Runtime.NewGoalSink` → `db.GetSession`/`SetSessionGoal`
  (`artifactsink.go` deseni). chat_stream + autonomous_interaction `setGoal`; CLI köprüsü
  `mcp_interaction.go` `callGoal`. Native registry'ye iki eager built-in.
- **Skill:** `swarmgo-progress` north-star satırı + `swarmgo-guide` interaction bölümü güncellendi.
- **Test:** 7 yeni birim test; tools/agent/api `build`/`vet`/`test` yeşil. Detay: `_Docs\11` §19.
- **Not:** Goal kartı canlı-refresh event'i bu fazda yok (panel yeniden açılınca tazelenir;
  kalıcılık+context enjeksiyonu anında). Faz 3 kalan: `set_session_title`/cwd/archive.

## Interaction MCP Faz 3 — `focus_view` (UI navigasyon) ✅ (2026-06-25)

`notify`'ın tamamlayıcısı: ajan UI'ı **aktif olarak** bir ekrana/entity'ye sürer
("yaptığım artifact'ı aç"). `notify` pasif (toast→tıkla→git); `focus_view` anında navige
eder. Mevcut deep-link makinesini yeniden kullanır: araç `events.Event{Type:"navigate"}`
yayınlar → SSE → `App.tsx onEvent` `navigate` tipini anında uygular (`routeFromEvent`→
`buildRoute`→`window.location.hash`). Toast/rozet yok (eylemin kendisi).

- **Araç (`tools/builtin_focusview.go` + `navigatesink.go`):** `focus_view(view*, sessionId?,
  agentId?)`; view enum doğrulama; sink yoksa graceful no-op. `NavigateSink` köprüsü.
- **Concrete sink:** `api/notifysink.go`'daki `notifySink` artık hem `NotifySink` hem
  `NavigateSink` (tek instance, iki arayüz) → wiring çoğaltılmadı.
- **Wiring:** native registry + chat_stream + autonomous_interaction `setNav`; CLI köprüsü
  `mcp_interaction.go` spec+`callFocus`; frontend `App.tsx` navigate handler.
- **Test:** 5 yeni birim test; `go build`/`vet`/`test`+`tsc`/`vite` yeşil. Canlı claude-cli
  (Fasty `focus_view{view:artifacts}`) uçtan uca doğrulandı. Detay: `_Docs\11` §18.

## Interaction MCP Faz 3 — `notify` (masaüstü bildirim) ✅ (2026-06-25)

Interaction MCP'ye **bloklamayan masaüstü bildirimi** aracı (`notify`) eklendi; native
ajanlar (anthropic/minimax) ve CLI ajanları (claude-cli) artık kullanıcı uygulamaya
bakmıyorken haber verebilir ("uzun iş bitti", "dikkat gerek", "hata oluştu"). Mevcut
bildirim altyapısı (event bus → SSE → OS toast → per-tip susturma) zaten hazır olduğundan
yalnız aracı + iki yola wiring gerekti; **yeni event tipi/transport yok** (mevcut `agent`
tipi). Tek-şema kuralı korundu — tanım `tools` paketinde tek yerde, iki adaptör enumerate eder.

- **Araç (`internal/tools/builtin_notify.go` + `notifysink.go`):** `notify(title*, body, level)`,
  bloklamaz; `level∈{info,success,error}` (boş/geçersiz → info). `NotifySink` context köprüsü
  (`WithNotify`/`notifyFrom`, `WithArtifacts` deseni). Sink yoksa **graceful no-op** (tur bozulmaz).
- **Native:** `agent/toolsetup.go` registry'ye eager built-in olarak eklendi.
- **Concrete sink (`api/notifysink.go`):** `events.Event{Type:"agent"}` yayını
  (Target=`{view:chat, sessionId, agentId}` → tıklayınca sohbete deep-link).
- **Chat wiring (`api/chat_stream.go`):** her turda `run.setNotify` + `tools.WithNotify`.
- **CLI köprüsü (`api/mcp_interaction.go`):** spec + `Call` case + `callNotify`; `interactiveOnlyTools`
  **değil** → otonom turlarda da advertise edilir (bloklamaz). Allowlist tek-kaynaktan otomatik (CLI-2).
- **Otonom wiring (`api/autonomous_interaction.go`):** scheduler/spawn/flow CLI ajanları da
  `setNotify` alır (UI açıkken toast düşer).
- **Test:** 5 yeni notify birim testi (dispatch/level default/bilinmeyen level/title zorunlu/sink-yok);
  `go build`/`vet`/`go test ./...` yeşil (**468 test, 32 paket**). Canlı claude-cli testi beklemede.

Detay: `_Docs\11-INTERACTION-MCP.md` §17. Kalan Faz 3: workspace/oturum etkileşimleri (ileride).

## Ingest: TOML command + marketplace.json keşfi ✅ (2026-06-25)

Generic import boru hattına iki ekleme:

- **`.toml` slash-command desteği:** `commandAdapter` artık `commands/*.toml`'u da okur
  (önceden yalnız `.md`). Yeni bağımlılık yok — `ingest/toml.go` mini parser'ı
  `description`/`prompt` (tek + `"""`/`'''` çok-satır) çıkarır; loadable skill'e çevrilir.
- **marketplace.json güdümlü keşif:** `ingest/marketplace.go` repo kökündeki
  `.claude-plugin/marketplace.json`'u okur; `plugins[].source` yolları **gerçek plugin
  kökleri** olarak hedeflenir (kullanıcı alt-yol vermediyse) → kör tarama yerine isabet,
  aynalar/ilgisiz alt-ağaçlar baştan elenir. En sığ manifest seçilir. `Scan` artık kök
  listesi üzerinde döner.

Canlı: caveman **11** (3 agent + 8 skill; `.toml` komutlardan `caveman-init` eklendi,
kalan 3'ü skill-folder'larla dedup'landı), taste-skill 13, marketingskills 45. vet temiz,
**463 test**. Detay: `_Docs\37-INGEST-MIMARISI.md` §4, §9.

## Generic import pipeline — internal/ingest (SK-IMP3) ✅ (2026-06-25)

İçe aktarma **skill-özel olmaktan çıkıp jenerik, çok-türlü** bir boru hattına dönüştü.
Tek hat artık GitHub repo / plugin / local klasörden **skill + agent + command + MCP**
keşfedip hepsini market ile **aynı kurulum otoritesine** bağlıyor. Beş fazda yapıldı:

- **Faz 1 — `internal/fetch` (edinme):** tarball indirme + local tree walk + GitHub URL
  ayrıştırma tek pakete çıkarıldı (`TreeFrom`/`GroupByMarker`/`FindFiles`). `skills` ve
  `ingest` ortak kullanır; eski `skills/github.go` (contents-API) silindi.
- **Faz 2 — `market.Pack.Files`:** pack envelope'una `Files map[string][]byte` eklendi;
  `InstallSkill` nested kaynakları (`safeRelPath`) yazar, publish (`collectSkillFiles`)
  toplar → yayınla/kur kaybsız.
- **Faz 3 — `internal/ingest` + Adapter:** `Adapter{Kind,Scan}` arayüzü + `Discovered`
  (preview + `build` closure) + orchestrator (`Scan`/`BuildPacks`). Skill adapter
  `skills.RenderImportedSkill`'i sarmalar; `skills`'ten helper'lar export edildi
  (`RenderImportedSkill`/`Slugify`/`SuggestSlug`/`FrontmatterField|List|Body`).
- **Faz 4 — tek install otoritesi + kind-agnostik UI:** `api/market.go` kind switch'i
  `installPackInto(...) (InstallResult, error)`'a çıkarıldı (alt-installer'lar `httpErr`
  ile HTTP kodu taşır); market install **ve** yeni `POST /api/ingest/{scan,install}`
  ortak çağırır. `SkillImportDialog` kind-agnostik. Eski `/api/skills/import/{scan,bulk}`
  kaldırıldı; `/api/skills/import` (tek) korundu.
- **Faz 5 — agent/command/mcp adapter'ları:** `**/agents/*.md`→AgentPayload (body→Soul,
  tools→AllowedTools; CC model uyarıyla eşlenmez), `**/commands/*.md`→loadable skill
  (`.toml` atlanır), `.mcp.json`→MCPPayload. **`(kind,slug)` dedup** (repo'nun `plugins/`/
  `dist/` aynası → en sığ yol).

**İlke:** import market'in **içine taşınmadı**; ayrı `internal/ingest` paketi market'i
bağımlılık olarak kullanır (döngüsüz: `ingest→{fetch,skills,market}`, yalnız `api→ingest`).
**Canlı:** caveman **10** (3 agent + 7 skill; `plugins/` aynası dedup'landı), taste-skill
**13**, marketingskills **45**. Go vet temiz, **460 test**, tsc + prod build temiz.
Detay: **`_Docs\37-INGEST-MIMARISI.md`**, `21-MARKET.md` §4.1.

## Generic AgentIdentity bileşeni + agent ayar pill seçicileri ✅ (2026-06-25)

**AgentIdentity (`components/agents/AgentIdentity.tsx`):** Uygulama genelinde agent
gösterimini (avatar + ad + opsiyonel alt satır) tek bileşene topladı. `size`
(sm/md/lg), `subtitle` ('model' → katalogdan çözülmüş model etiketi · 'none' ·
özel ReactNode), `active` (avatar halkası), `dim`, `nameSuffix`, `trailing`
props'ları. `AgentAvatar` düşük seviyeli primitif olarak kalır; AgentIdentity onu
+ `resolveModelLabel`'i sarmalar. Taşınan yerler: composer **AgentSelect**
dropdown'ı, **AgentHeader** (sohbet balonu), **SessionDetailPanel** "Konuşmadaki
ajanlar", **TaskBoard** kart sahibi, **AgentPicker** (board kart-detay sahip
seçici — trigger+seçenekler, artık model alt satırı), **AgentRoster**,
**AgentsView**. (Avatar-only ikon kullanımları — SessionsSidebar/Executions/
Schedules/Artifacts/Budget/Autocomplete/flow AgentNode — primitif AgentAvatar'da
bırakıldı; isim ayrı/karmaşık satırda.)

**Agent ayar formu pill seçiciler:** Planlama/Düşünme/İzin `<select>`'leri ikon+
etiketli `OptionPills` (radiogroup) ile değiştirildi; ikon dili composer ile
ortak (`components/agents/agentOptions.ts`). Düşünme "Kapalı" = boş string
(depolama korunur).

## Default skill: `swarmgo-doc-improver` (6-ölçütlü doküman denetimi) ✅ (2026-06-25)

**Ne:** Yeni gömülü default skill — SwarmGo'nun kendi bağlam dokümanlarını (skill'ler,
workspace CLAUDE.md/AGENTS.md kuralları, `_Docs`) denetleyip iyileştiren tekrarlanabilir
iş akışı. Anthropic'in resmi `claude-md-improver` skill'inden ilham; SwarmGo'nun daha
geniş doküman yüzeyine genelleştirildi.

**İçerik:** 6 ölçüt (komutlar / mimari açıklığı / açık-olmayan gotcha'lar / kısalık /
güncellik / uygulanabilirlik, her biri /5) + puanlı rapor formatı. SwarmGo'ya özgü
çekirdek içgörü: **changelog-leak anti-pattern** — referans dokümanların (`swarmgo-project`,
`_Docs` mekanik bölümleri) tarih damgalı geçmişi biriktirmesi; çözüm "1 cümle güncel durum
+ → `_Docs/NN`" kalıbı. **Doc-type kalibrasyonu:** referans/her-tur-yüklenen dokümanda
kısalık sert, on-demand action skill gövdesinde işlevsel yoğunluk normal → sağlıklı
dokümanı zorla kesme.

**Gömme:** Go değişikliği **gerekmedi** — `internal/skills/defaults.go` `//go:embed defaults`
tüm ağacı gömer ve slug'ları alt-dizinlerden türetir; yalnız `defaults/swarmgo-doc-improver/SKILL.md`
eklendi. Build + 29 test yeşil.

**Yan iş (aynı oturum, davranışsız doküman temizliği):** `swarmgo-project` referans skill'i
~%11 kısaltıldı (changelog-leak temizlendi + PowerShell çalıştırma komut bloğu eklendi);
`swarmgo-guide` Memory maddesi okunabilirlik için alt-maddelere bölündü; default-skill listesi
güncellendi (progress/gan-loop/doc-improver eklendi).

## Agent avatar mojibake onarımı + model adı gösterimi ✅ (2026-06-25)

**Sorun:** claude-cli `create_agent` yoluyla (bir agent'ın başka agent oluşturması)
yaratılan agent'larda emoji avatar + soul/identity metinleri **mojibake**'ye
dönüşüyordu (UTF-8 baytları Latin-1 olarak yanlış çözülüyor; ör. 🗺️ → `ðºï¸`).
UI'da ikon bozuk kutucuklar olarak görünüyordu (AGT1–5 UI'dan temiz, AGT6–9
agent-üretimi bozuk). CLI/MCP transport katmanından geliyor.

**Üç katmanlı çözüm:**
1. **Frontend (görüntü):** `lib/avatar.ts` → `normalizeAvatar` + `repairMojibake`;
   gerçek emoji aynen, mojibake onarılır, onarılamazsa **baş harflere** düşer
   (asla çöp glyph). `AgentAvatar` `isEmoji` boyutlandırması normalize'a bağlandı.
2. **Backend (kök neden):** `tools/builtin_agentmgmt.go` → `repairMojibake`
   (Türkçe ş/ğ/ı korunur; rastgele Latin-1 katı UTF-8 re-decode'da elenir);
   `create_agent` + `update_agent`'ta `name`/`soul`/`identity`/`avatar` kaydedilmeden
   onarılır. Test `builtin_agentmgmt_test.go` (9 vaka).
3. **Veri:** WS2/AGT6–9 dosyaları onarıldı (✈️🏨📍🗺️), `.bak` yedekleriyle.

**Model adı:** Agent gösterilen yerlerde ad altında **soluk** model adı (boşsa
provider): composer `AgentSelect` dropdown'ı + `AgentRoster`. Composer tetik butonu
avatar + **ajan adı** gösteriyor.

**Yan onarım (build kıran önceki sorun):** `providers/pricing.go` mükerrer `"minimax"`
map anahtarı tek bloğa birleştirildi (M2.1/lightning/M2/M1/Text-01, 0.25× cache).

## Mermaid diyagram render desteği (chat) ✅ (2026-06-25)

Sohbet markdown'ında ```` ```mermaid ```` fenced blokları artık tema-duyarlı SVG
olarak çizilir (akış/sıra/durum/sınıf/ER/gantt vb.). `diff` bloklarının
`DiffView`'a yönlenmesiyle aynı kalıp.

- **Bağımlılık:** `mermaid@^11` (`frontend/package.json`). Ağır (~3MB) →
  **dinamik `import()`** ile lazy yüklenir; `vite.config.ts` `manualChunks`'a
  `vendor-mermaid` (mermaid + d3/dagre/cytoscape/khroma/elkjs) eklendi → ana
  bundle'a binmez (yalnız bir diyagram render edilince iner; `relationGraph`
  kalıbı). Build doğrulandı: `vendor-mermaid` ≈ 3.1MB ayrı chunk, `index` sabit.
- **Bileşen:** `frontend/src/components/markdown/MermaidDiagram.tsx` — tema base'i
  `<html data-theme>`'ten (`light`→`default`, yok→`dark`) seçilir, `themeVariables`
  CSS değişkenlerinden (`--color-accent`/`-surface`/`-text`/…) türetilir;
  `MutationObserver` ile tema değişiminde yeniden çizer. **Akış-dayanıklı:** 120ms
  debounce + render hatasında ham kaynağa düşer (yarım kalan diyagram
  patlatmaz); ayrıca geçersiz sözdiziminde mermaid'in `<body>`'ye iliştirdiği
  hata/"bomba" SVG'si `finally`'de id ile temizlenir (orphan leak yok). Toolbar:
  Source/Diagram toggle, Expand (tam-ekran overlay), Copy. `securityLevel: 'strict'`.
- **Entegrasyon:** `CodeBlock.tsx` `lang === 'mermaid'` → `MermaidDiagram`.
- **Test (Playwright, 2026-06-25):** geçici harness ile flowchart/sequence/state
  render, dark+light tema geçişinde yeniden renklenme ve geçersiz blok → kaynak
  fallback (crash yok, bomba leak yok) gerçek tarayıcıda doğrulandı.
- **Ajan farkındalığı:** `swarmgo-guide` default skill'ine "Rich replies"
  bölümü eklendi (mermaid/diff/kod render edildiğini ajana öğretir).
- **Davranış fix'i (2026-06-26):** Ajan "diyagram çiz" deyince mermaid'i mesaja
  gömmek yerine `create_artifact(kind=mermaid)` yapıyordu (ART11/ART12). Kök neden:
  her turda enjekte edilen `artifactDeliverableGuidance` sabiti (`api/artifacts.go`)
  "produce a ...**diagram**... → create_artifact" diyordu → skill notunu eziyordu.
  Düzeltme: "diagram" deliverable listesinden çıkarıldı + açık **istisna** eklendi
  ("diyagram istenince ```mermaid bloğunu mesaja inline koy, sadece artifact yapma");
  hem chat (`chat_turn.go:60`) hem claude-cli (`agent_context.go:120`) yolunu kapsar.
  Ayrıca default skill'ler diske bir kez **seed** edildiğinden (`skills.EnsureDefaults`,
  "existing files never overwritten") disk kopyası eski kalıyordu → disk kopyası elle
  güncellendi (skill gövdesi her `use_skill`'de diskten okunur → anında geçerli).
  Binary yeniden derlenip :8090'da restart edildi (eski binary `swarmgo.bak.exe`).
- Detay: `_Docs\07-CHAT-UX.md`.

## Workspace'e özel görünüm/tema + Dil → Profil ✅ (2026-06-25)

Görünüm ayarları uygulama-geneli Ayarlar'dan çıkıp **NavRail ▸ Workspace ▸
Görünüm sekmesine** taşındı ve **aktif workspace'e özel** hâle getirildi: her
workspace kendi tema paleti / temel mod / accent'ini saklar ve **workspace
değiştirilince UI teması da değişir**. (`AppearancePanel` artık tamamen
self-contained — global+workspace ayarını kendi çekiyor — ve `WorkspaceView`
sekmelerinde gömülü; APP_CATS'ten `appearance` kaldırıldı.)

- **Backend:** `WSSettings`'e `Theme`/`Accent`/`ThemePreset` alanları (+ patch +
  `UpdateSettings`) ve `workspaceSettingsDTO`'ya aynı alanlar eklendi. Boş alan =
  uygulama-geneli görünümü miras alır. `ws-settings.json`'da kalıcı.
- **Frontend:** `lib/theme.ts`'e `Appearance` tipi + `resolveAppearance`
  (workspace override'ı global üstüne katmanlar) + `applyAppearance`. `App.tsx`
  global+workspace görünümünü iki ref'te tutar, workspace değişiminde
  `GET /api/workspace-settings` çekip `applyResolvedTheme()` çağırır; global
  ayar kaydı workspace seçimini ezmez. `AppearancePanel` self-contained yeniden
  yazıldı: doğrudan `PUT /api/workspace-settings`, canlı önizleme, unmount'ta
  kaydedilmemiş önizlemeyi geri alma, "Genele sıfırla" butonu.
- **Dil** seçeneği Görünüm'den **Profil** kategorisine taşındı (uygulama-geneli
  kalır). Detay: `_Docs/06-WORKSPACES.md`.

## Composer otomatik-büyüyen giriş + araç çubuğu düzeni ✅ (2026-06-25)

Sohbet giriş alanı (`Composer.tsx`) yeniden düzenlendi. Eskiden tüm kontroller
(ajan seçici, düşünme/izin picker'ları, çalışma dizini, ekle, textarea, gönder)
**tek satırda** `items-end` ile yan yana duruyordu; textarea büyüdüğünde yanındaki
butonlar da uzayıp düzeni bozuyordu. Ayrıca textarea otomatik büyümüyordu
(`rows=1` + `max-h-40`, JS yok).

Yeni tasarım **kart + iki satır**: (1) tam-genişlik **textarea** üstte, (2) altta
sabit yükseklikli **araç çubuğu** (sol = ajan/picker/dizin/ekle, sağ = gönder
kümesi, arada `flex-1` boşluk). Kenarlık/odak halkası artık kartta
(`focus-within:border-accent`). **Otomatik büyüme:** `useLayoutEffect` her
`text` değişiminde `height='auto'`→`scrollHeight` ile textarea'yı içeriğe göre
büyütür; `max-h-[5.5rem]` (~3 satır) sınırlar, üstünde iç kaydırma açılır →
butonlar **asla** itelenmez/uzamaz. Dosya: `frontend/src/components/chat/Composer.tsx`.

## Skill koleksiyon içe aktarma (SK-IMP2) ✅ (2026-06-25)

İçe aktarma artık **tek skill klasörü** yerine **çok-skilli koleksiyonları** (GitHub repo /
Claude Code plugin / `skills/` klasörü) destekler. Hedef: `juliusbrussee/caveman`,
`leonxlnx/taste-skill`, `coreyhaines31/marketingskills` gibi repoların ve
crossaitools.com / skillsmp.com / claudeskillsmarket.com dizinlerinin işaret ettiği
GitHub-tabanlı skill'lerin toplu eklenmesi.

- **Tek-tarball indirme:** `internal/skills/collection.go` repo'yu `codeload.github.com/
  …/tar.gz/<ref>` üzerinden **bir** istekle çeker (`main`→`master` fallback), `archive/tar`
  +`compress/gzip` ile bellek-içi ayıklar (**yeni bağımlılık yok**, go.mod hâlâ uuid+cron).
  Contents-API klasör-gezmesine göre anonim rate-limit'e çok daha dostu (1 istek = keşif+içerik).
- **Keşif** (`discoverInTree`): her `SKILL.md`'nin ebeveyni skill klasörü; dosyalar **en derin**
  ata-skill'e atanır → nested sub-skill kendi kaynaklarını korur. Kök-skill + URL alt-yol
  prefix filtresi desteklenir. `skipDirs` ile repo gürültüsü (.git/.github/dist/…) elenir;
  4 MB/dosya, 64 MB/toplam cap.
- **Nested kaynaklar korunur:** `ImportCCSkill` artık `references/`/`evals/`/`scripts/` alt
  klasörlerini yazar (`safeBundledPath` mutlak yol + `..` reddeder, nested'a izin verir);
  `readLocalSkillDir` tek-skill local import'ta da ağacı `WalkDir` ile toplar.
- **YAML block-scalar açıklama** (`description: >` folded / `|` literal) parse edilir
  (`frontmatter.go::collectBlockScalar`) — caveman vb. community skill'lerinde yaygındı, eskiden `>` çıkıyordu.
- **API:** `POST /api/skills/import/scan` (önizleme) + `POST /api/skills/import/bulk`
  (seçili `paths[]` + `slugPrefix` + `shared`; çakışma batch'i durdurmaz, `Skipped`'a düşer).
  `owner/repo` kısayolu kabul edilir. Eski `/api/skills/import` (tek) korundu.
- **UI:** `SkillImportDialog` üç adımlı (**Tara → Seç → İçe aktar**): keşfedilen skill listesi,
  çoklu seçim + tümü/hiçbiri, "zaten var" rozeti, slug öneki, sonuç (içe aktarılan + atlanan + uyarılar).
- **Test:** `collection_test.go` (gruplama/prefix/kök-skill/çakışma/block-scalar; 29 birim test),
  `collection_live_test.go` (network-gated). **Canlı doğrulama:** caveman 11, taste-skill 13,
  marketingskills **45 skill + 155 nested dosya** sorunsuz içe aktarıldı.
- **Detay:** `_Docs\21-MARKET.md` §4.1.

## Generator↔Evaluator (GAN-benzeri) flow şablonu ✅ (2026-06-25)

Anthropic *"harness design"* makalesindeki **self-evaluation problemi** (ajan kendi işini
körü körüne över) için sözleşmeli **generator↔evaluator** iterasyon döngüsü: işi yapan
generator ile yargılayan **ayrı, şüpheci** evaluator; sprint contract + gerçek Playwright
testi + kod-konumlu bug raporu + skora göre **refine/pivot**. **Yeni motor kodu yok** —
mevcut node tipleriyle, döngünün (cycle) bilinçli kullanımıyla kuruldu.

- **Motor teyidi:** `orchestration.Validate()` acyclicity kontrol **etmiyor**, engine döngüye
  izin verip `maxSteps=50` ile sınırlıyor → `evaluate → decide → generate` geri-kenarı
  doğrudan kurulabiliyor. (Eski `swarmgo-flows` skill'i "must be acyclic" diyordu — **yanlıştı**,
  düzeltildi.)
- **Kısıt → karar:** branch yalnız string eşler (sayısal eşik yok) → skor→pivot kararı
  **keyword verdict** (`VERDICT: SHIP|REFINE|PIVOT`, `decide` `matchMode:regex` son satıra
  demirli); döngüde graf çıktısı üzerine yazıldığından skor **trend'i** `core:sprint-scorelog`
  çekirdek belleğe append edilir, sözleşme `core:sprint-contract`'ta yaşar.
- **Dağıtım:** gömülü gallery şablonu `gan-loop` (`frontend/src/lib/flowTemplates.ts`) +
  market paketleri `flow.gan-generator-evaluator` / `agent.skeptical-evaluator` /
  `mcp.playwright` (global market dizinine yazıldı) + yeni default skill
  `swarmgo-gan-loop`. Şablon agent-bağımsız → kurulumdan sonra **iki ayrı ajan** atanır.
- **Test:** `engine_test.go` — `TestValidate_AllowsCyclicGraph`, `TestRun_GANLoop_RefinesThenShips`
  (2× REFINE → SHIP → finalize), `TestRun_GANLoop_StepCapBackstop` (hiç ship etmeyen →
  `step cap` hatası). `go build`/`vet`/`test ./internal/orchestration` ✅; `tsc -b`/`vite build` ✅.
- Detay: [`15-FLOW-CANVAS.md`](15-FLOW-CANVAS.md) §Generator↔Evaluator döngü şablonu;
  kullanım kılavuzu: `swarmgo-gan-loop` skill.

## Yapılandırılmış subagent görev sözleşmesi ✅ (2026-06-25)

Anthropic *"Multi-agent research system"* rehberi: her subagent'a **objective +
output format + tool/source guidance + boundaries** verilmezse iş tekrarı/boşluk
oluşur. SwarmGo'da `run_subagent` yalnız serbest-metin `task` alıyordu; bu 4 alanı
yapısal teşvik etmiyordu.

- **Şema:** `run_subagent` input'una **üç opsiyonel alan** eklendi — `objective`,
  `output_format`, `boundaries` (`tools/subagent.go`: `runSubagentInput` + `RunAgentSpec`
  + JSON şema + `Call()` parse). `target`+`task` hâlâ tek zorunlu çift.
- **Enjeksiyon:** `agent/subagent.go::delegationContract()` dolu alanlardan bir
  **"Task contract"** bloğu üretir, `runAgent()` bunu subagent system-prompt'una
  persona'dan **sonra** ekler (`req.System`). Hiçbir alan yoksa blok boş → eski düz-`task`
  davranışı bayt-bazında korunur. Native + CLI köprüsü ikisi de `runAgent`'tan geçtiği
  için tek nokta yeterli. Açık karar: sözleşme şu an yalnız **sync** dalda (async →
  `SpawnSession`, ileride genişletilebilir).
- **Teşvik:** tool description + `input_examples`'a yapılandırılmış örnek eklendi →
  model 4 alanı doldurmaya yönlendirilir.
- **Test** `agent/subagent_test.go::TestDelegationContract` (boş→"", dolu→satırlar,
  tek-alan izolasyonu). `go build`/`vet`/`test ./...` ✅.
- Detay: [`25-SUBAGENT-ISOLATION.md`](25-SUBAGENT-ISOLATION.md) §Yapılandırılmış görev sözleşmesi.

## Kalıcı todo / PROGRESS dosyası ✅ (2026-06-25)

Anthropic *"Effective harnesses for long-running agents"* + *"Effective context
engineering"* makalelerindeki **kalıcı not dosyası** (`claude-progress.txt` +
`feature_list.json` `passes` boolean) konvansiyonu SwarmGo'ya getirildi. `todo_write`
listesi artık **diske kalıcı**: oturumlar arası kaybolmuyor, yeni oturum devralıyor.

- **Sorun:** `todo_write` stateless'tı; liste yalnız oturum-içi (mesaj trace'inden
  `todoContextBlock` ile yeniden inşa) yaşıyordu. Oturum restart/yeni oturum/ajan
  değişiminde kayboluyordu. Core memory (serbest persona/human) bunu karşılamıyor.
- **Çözüm:** Liste, çalışma dizinine bağlı **`<cwd>/.swarmgo/progress.json`**'a
  yazılır (cwd yoksa `<store>/progress/<agentID>/`); fresh oturum açılışında
  geri yüklenip "Resumed progress" bloğu olarak `SystemDynamic`'e enjekte edilir.
  `completed` ≡ Anthropic `passes:true`. Rolling `log` = `claude-progress.txt`.
- **Yeni paket** `internal/progress` (atomik JSON oku/yaz). **Sink** ArtifactSink
  deseninin ikizi: `tools/todosink.go` (ctx) + `agent/todosink.go` (`NewTodoSink`).
  Native (`toolloop.go` fallback) + chat (`chat_stream.go`) + CLI
  (`mcp_interaction.go callTodo` + `chat_control.go` run sink + autonomous) yolları.
- **Geri yükleme** `api/todos.go::todoContextBlock` (cwd+agentID+resume); fresh
  oturumda diskten devralır (`renderResumedBlock`). `agent/workdir_ctx.go` →
  `SessionWorkdir`; `db.DB.Root()`.
- **Ayar** `progressPersist`/`progressResume` (vars. açık) — settings/DTO/Patch +
  `Tunables.SetProgress` + `applySettings` + UI (Ayarlar ▸ Bağlam ▸ "Kalıcı
  ilerleme"). Migration yok, opt-in (kapalı → eski efemeral davranış).
- **Test** `progress` (round-trip/trim/missing), `agent` (cwd+store fallback),
  `tools` (sink çağrısı/no-op/hata-yutma), `api` (resumed block/progressDir). `go
  test ./...` ✅, `go vet` ✅, frontend build ✅.

**İkinci tur (aynı gün) — zenginleştirmeler:**
- **`feature_list` zenginliği:** `todo_write` öğelerine opsiyonel `category` +
  `steps` (Anthropic feature_list paritesi); şema + `TodoSinkItem` +
  `progress.TodoItem` + sink mapping uçtan uca taşır (`omitempty`).
- **`swarmgo-progress` default skill'i:** ajana otomatik progress.json + insan-okunur
  `PROGRESS.md` konvansiyonunu öğretir (`internal/skills/defaults/swarmgo-progress/`;
  `//go:embed` ile otomatik, baseline skill setine girer).
- **UI görüntüleyici:** `GET /api/sessions/{id}/progress` (`api/progress.go`) +
  `SessionDetailPanel` "Kalıcı ilerleme" salt-okunur kartı (`ProgressCard` — statü
  işaretçili maddeler + category + son log satırları). `go test ./...` 438 ✅.
- Detay: [`36-KALICI-ILERLEME.md`](36-KALICI-ILERLEME.md).

## Otonom turda boot-verification sırası ✅ (2026-06-25)

Anthropic *"Effective harnesses for long-running agents"* makalesindeki **standart
oturum açılış sırası** (yönelim → hatırlama → tek görev seç → temel testi doğrula →
işi yap → döngüyü kapat) SwarmGo'nun otonom turlarına getirildi. Kayıp bağlamı telafi
eden, düşük-riskli, çoğunlukla skill+doküman değişikliği.

- **Skill reçetesi (ana iş):** `swarmgo-autonomous-ops/SKILL.md` → yeni **§10 "The
  autonomous boot sequence"** (Step 0 Orient → Step 5 Close); referans setup'a `0.` adımı
  ve Pitfalls'a "Skipping the boot sequence" maddesi. Reçete `.swarmgo/progress.json`
  (progressPersist) + `list_tasks` (append-only board) + git log'u "hafıza" olarak
  okur; kapanışta git commit + append-only not. One-task-per-run + append-only kullanıcı
  tercihiyle hizalı.
- **Minimal kod kancası:** `runtime.go autonomousSystemPrompt` artık `autonomousBootReminder`
  (6 satırlık, skill'e yönlendiren pointer) enjekte eder — tek noktadan **dört otonom yol**
  (scheduler/spawn/flow/subagent; `executor.go`+`subagent.go` ortak kurucu). Chat turları
  (composeTurnRequest) etkilenmez. Pointer-only → cache'li statik prefix şişmez.
- **Gate:** `autonomousBootSeq` ayarı (vars. **true**) — `settings.go` (struct+default+DTO+patch)
  + `store.go` apply + `tunables.go` (alan/default/`SetWorkdirGuards`/`AutonomousBootSeq()`)
  + `server.go applySettings`. Eski config'lerde `Default()` backfill'i ile true kalır.
  Frontend: `types/settings.ts` + `SettingsPanel.tsx` payload + `appPanels.tsx` toggle
  ("Otonom boot doğrulama sırası").
- **Test:** `bootseq_test.go` (gate default+SetWorkdirGuards köprüsü + reminder içeriği);
  `go build ./...` + `go test` (159 vaka: agent/settings/api) + `tsc --noEmit` yeşil.
- İlişki: Görev #2 kalıcı PROGRESS (`progressPersist`/`progressResume` zaten mevcut) bu
  reçetenin Recall/Close adımlarını besler; dosya yoksa git log + board'a düşer.
- Detay: `_Docs\33-DIS-AJAN-OTOMASYONU.md` §Otonom Boot Sırası.

## Context-rot farkındalığı + adaptif bütçe stratejisi ✅ (2026-06-25)

Anthropic *Effective context engineering* makalesi: token arttıkça recall hassasiyeti düşer ("context rot", `n²` dikkat ilişkisi → **performans gradyanı**, uçurum değil). SwarmGo'nun önceki "her şeyi ham tut" bahsi (512K/0.6) bu rot ile bilinçli bir takastı. Dayanıklılığın aslında **retrieval katmanında** (memory/`conversation_search`/core blocks) olduğu, ham pencere boyutunda olmadığı tespit edildi → ham pencere küçültülebilir, recall kaybetmeden.

- **Adaptif fraction:** `providers.AdaptiveBudgetFraction(provider, model)` — `ContextWindowFor`'un aile sınıflamasını yeniden kullanır; Opus/Sonnet 0.45, Haiku/Fable 0.40, MiniMax/DeepSeek/Gemini 0.35, bilinmeyen 0 (caller fallback).
- **Yeni semantik:** `ContextBudgetFraction = 0` → **otomatik/adaptif** (pozitif = manuel sabit). `EffectiveBudget` `fraction<=0`'da adaptif tabloyu kullanır; `Manager.SetBudgetShape` artık 0'ı (auto) saklar; `store.go` validate 0'ı korur (negatif → 0).
- **Yeni varsayılanlar:** `ContextBudgetCeil` 512K→**256K** (`262144`), `ContextBudgetFraction` 0.6→**0 (auto)**, `memoryPressureWarn` 0.75→**0.70** (`settings.go`+`tunables.go`). Eski `0.6` persisted değer manuel sabit olarak yaşar; yeni kurulum adaptif başlar.
- **Frontend:** Ayarlar▸Bağlam "Pencere oranı"/"Bütçe tavanı" hint'leri auto+rot açıklamasıyla güncellendi (`appPanels.tsx`).
- **Test:** `budget_test.go` (`TestEffectiveBudgetAdaptive`) + `context_window_test.go` (`TestAdaptiveBudgetFraction`); `go build ./...` + `go test ./internal/conversation ./internal/providers ./internal/settings` yeşil (83 test).
- **Doküman:** `_Docs\17` yeni **§12** (takas analizi + strateji + tablolar + mermaid) + §7 çapraz-referans; `swarmgo-settings` skill + `swarmgo-project` skill güncellendi.

## Context Reset + Handoff Artifact ✅ (2026-06-25)

Anthropic "harness design for long-running apps" bulgusu: in-place compaction tek başına **"context anxiety"**yi (model limite yaklaşınca erken toparlama) çözmez. Çözüm = **context reset** + **handoff artifact**: pencereyi özetlemek yerine, devamı taşıyan bir handoff dosyası yazıp **temiz bir oturumda** sürdür. SwarmGo'da önceden yalnız in-place rolling-summary vardı; bu, onun opt-in tamamlayıcısı.

- **Çekirdek:** `internal/conversation/handoff.go` (`handoffPrompt` 10-bölüm + DONE/TODO + Next Step, `HandoffEnv`, `BuildHandoff` — compaction çekirdeğini `KindCompact` ile yeniden kullanır) + `internal/agent/handoff.go` (`HandoffSession`: üret→artifact yaz→(ops.) `<workdir>/.swarmgo/handoff.md`→`SpawnSession(ParentSessionID)` ile taze oturum→tombstone; `maybeAutoHandoff`/`handoffChainDepth`/`handoffEnv`/`buildContinuationPrompt`).
- **Üç tetik:** manuel `/handoff` (`POST /api/sessions/{id}/handoff` → `summary.go handleSessionHandoff`); ajan aracı `handoff_session` (`tools/builtin_handoff.go`, self-manage gated, `toolsetup.go` kapanışı); **otomatik** (yalnız otonom tur — `runSpawn`/`deliverPrompt` tur-sonu; overflow sinyali `callkind.go withOverflowFlag`/`markContextOverflow`, tetik `toolloop.go` reactive compaction'da).
- **Ayarlar:** `HandoffAuto` (vars. **kapalı**) / `HandoffPressure` (0.90) / `HandoffMaxChain` (20) / `HandoffWriteFile` (kapalı) — `settings.go`+`store.go` clamp + `server.go applySettings → tun.SetHandoff`; UI Ayarlar▸Bağlam "Context reset (handoff)" bölümü. Tunables `DefaultHandoffPressure`/`DefaultHandoffMaxChain`.
- **Soyağacı:** `db.Session.ParentSessionID`/`HandoffArtifactID` (+`SetSessionHandoffArtifact`); `GET .../info` döner; UI `SessionDetailPanel` "↩ Devraldığı oturum" tıklanır link + `useChatStream` `/handoff` komutu yeni oturuma geçer.
- **Test:** `conversation/handoff_test.go` (env+transcript enjeksiyonu, boş-transcript guard) + `agent/handoff_test.go` (chain-depth, `maybeAutoHandoff` no-op yolları, continuation prompt, title). `go build`/`vet` temiz; `go test ./internal/conversation ./internal/agent` yeşil (mevcut paralel `TestEffectiveBudget` WIP'i hariç). Frontend `tsc`+`vite build` yeşil.
- claude-cli `--resume` ile uyumlu (reset zaten cold yeni oturum açar). Detay: **`_Docs\35-CONTEXT-RESET-HANDOFF.md`**.

## Market — katman sadeleştirme: bundled + workspace tier'ları kaldırıldı ✅ (2026-06-25)

Market pack katmanları üçten (bundled/global/workspace) **bir yerele** (global) indirildi; uzak registry 4. kaynak olarak kalır.

- **Kaldırılanlar:** `internal/market/defaults.go` (`//go:embed defaults`) + `internal/market/defaults/` klasörü + `EnsureDefaults` çağrısı (runtime.go) + workspace pack tier'ı. Binary artık market item taşımaz, workspace'te `market/` klasörü oluşmaz.
- **`market.New(globalDir, ledgerDir)`:** tek yerel tier = global; `Publish` global dizine yazar; install ledger (`installed.json`) per-workspace **kökte** (eski `<workspace>/market/` yerine; `workspaceLedgerDir`). Store'da `writeDir` → `globalDir`+`ledgerDir` ayrımı.
- **Mevcut paketler korundu:** 28 başlangıç paketi zaten global dizinde (`~/.swarmgo/market`); silinmedi. Yeni kurulumlarda market boş başlar → global'e elle paket konur veya uzak registry eklenir.
- **Test:** `store_test.go` `EnsureDefaults`'tan arındırıldı (global'e elle pack yazıp test eder) + ledger/semver testleri eklendi; `go build ./...` + `go test` (145, market/api/agent) yeşil. Çalışan instance global'den 28 paket (`source=global`) döndürüyor.
- Detay: `_Docs\21-MARKET.md` §3.1.

## Skill grupları — katlanabilir (fold in/out) gruplama ✅ (2026-06-25)

Skiller artık serbest-metin bir **`group`** etiketiyle organize edilebiliyor; Skills
ekranında aynı gruptaki beceriler **katlanabilir başlık** altında toplanıyor. Tamamen
kozmetik — çözümleme/reklam/yükleme davranışını etkilemez.

- **Frontmatter (`group`/`category` alias):** `internal/skills/skill.go` `Skill.Group`,
  `store.go scanDir` parse eder; `SkillInput.Group` + `fields()` → `Create`/`Update` diske
  yazar. Import yolu dosyayı verbatim kopyaladığından grup korunur.
- **API:** `skillInputReq.Group` (`internal/api/skills.go`) → create/update gövdesinde geçer;
  `Skill` JSON'una `group,omitempty` eklendi.
- **Self-management araçları:** `create_skill`/`update_skill` artık `group` parametresi alır
  (`builtin_skillmgmt.go`); `SkillWriter` arayüzü + `agentSkillWriter` adapter (`runtime.go`)
  güncellendi (partial update'te `cur.Group` korunur, aksi halde silinirdi). Mock + test güncellendi.
- **UI (`SkillsPanel.tsx`):** `groupSkills()` listeyi gruba göre kovalar (adlandırılmış gruplar
  alfabetik, "Grupsuz" en sonda); her grup `ChevronDown/Right`'lı, sayaç rozetli katlanabilir
  başlık. Katlı gruplar `localStorage` (`swarmgo.skillsCollapsedGroups`) ile kalıcı. `SkillEditor`
  grup input'u + mevcut gruplardan `datalist` önerisi; detay başlığında grup rozeti.
- **Build/test:** `go build ./...` + `go test ./internal/skills ./internal/tools` ✅, frontend `tsc` ✅.

### Takip iyileştirmeleri (aynı gün)

- **İlişki grafiğinde grup tinti:** `api/graph.go` skill düğümüne `Sub = sk.Group` ekler (store'dan
  bakılır); `relationGraph.ts` `groupHue()` (etiket→deterministik HSL) ile aynı gruptaki skill
  yıldızlarını **ortak renkle** boyar, grupsuzlar sarı kalır; tooltip "Skill · <grup>" gösterir.
- **"Tümünü katla/aç":** SkillsPanel header'ında (>1 grup varken) `ChevronsDownUp`/`ChevronsUpDown`
  butonu — tüm grupları tek tıkla katlar/açar (`toggleAll`, `allCollapsed` türetimi).
- **Market:** skill paketi zaten tam `SKILL.md` gövdesini (`SkillPayload.Body`) taşıdığından
  `group` install/publish ile **kendiliğinden korunuyor** — değişiklik gerekmedi.
- **Git hijyeni:** `.gitignore`'a `*.log.err`/`*.err`; sızan `vite-run.log.err`/`swarmgo-run.log.err`
  izlemeden çıkarıldı (`git rm --cached`).

## Bütçe refactor faz 3 — RollupOf birleştirmesi (costOf+modelRowsFor → billing) ✅ (2026-06-25)

Pricing aggregation tümüyle billing'e taşındı, iki fonksiyon tek primitife indi.

- **`billing.RollupOf(byModel) Rollup`:** eski `costOf` + `modelRowsFor` çiftinin birleşik çekirdeği —
  her satırı fiyatlar (`PriceStat`), maliyete göre sıralar (input+output tiebreak) ve toplam
  cost/savings/cache + priced/estimated bayraklarını döner. Yeni `billing.Row` (provider/model/Stat +
  cost/save/priced/estimated) DTO-bağımsız.
- **api sadeleşti:** `costOf` silindi; `modelRowsFor` artık `RollupOf` çıktısını `modelStat` DTO'ya
  haritalayan ince adaptör. Cost-only çağıranlar (per-agent satır, günlük trend) doğrudan `RollupOf`
  okur. Per-agent provider/model aggregation `roll.Rows`'u tekrar kullanır → **çift fiyatlama kalktı**
  (eskiden u.ByModel ikinci kez PriceStat'tan geçiyordu). `strings` importu budget.go'dan düştü.
- **Test/build:** `billing_test.go`'ya `TestRollupOf` (sıralama + toplam + priced bayrağı); `go build
  ./...` + `go vet` + `go test ./...` → **407 passed (29 paket)**.

## Bütçe refactor faz 2 — UsageDelta helper + tokenTotals embed + billing paketi ✅ (2026-06-25)

Önceki refactor'un devamı; 3 ek sadeleştirme, davranış korundu.

- **`db.DeltaFromUsage(calls, providers.Usage)`:** `providers.Usage → db.UsageDelta` dönüşümü
  `RecordUsage` (agent) ve `recordCompaction` (conversation)'da elle kuruluyordu → tek mapping
  noktası `db`'de (yeni `db→providers` importu; döngü yok, providers leaf). Yeni token sınıfı
  eklenince tek yer güncellenecek.
- **`tokenTotals` gömülü struct (`api/budget.go`):** `modelStat`/`providerStat`/`dayPoint` aynı 5
  sayaç alanını (Calls/In/Out/CacheR/CacheW) tekrar ediyordu → anonim embed; alanlar promote olup
  inline marshal edilir, **JSON şekli birebir aynı** (alan adları korundu). `kindStat` ayrı kaldı
  (cache alanı yok). modelRowsFor literal'i `tokenTotals: {...}` biçimine güncellendi.
- **`internal/billing` paketi:** pricing domain matematiği (`priceStat` → `billing.PriceStat`) api'den
  çıkarıldı → api yalnız JSON şekillendirir, billing maliyet/tahmin mantığını taşır (db+providers
  import eder). Test `billing/billing_test.go`'ya taşındı (4 vaka).
- **Build/test:** `go build ./...` + `go vet` + `go test ./...` → **405 passed (29 paket; +billing)**.

## Bütçe + context refactor — fiyatlama/summarize/fold tekrarları sadeleştirildi ✅ (2026-06-25)

Davranış değiştirmeden 3 tekrar noktası generic'leştirildi (golden testlerle kilitlendi).

- **Tek fiyatlama primitifi (`api/budget.go priceStat`):** "PriceFor→CostDetailed/CacheSavings,
  yoksa EstimateFor" dallanması 3 yerde (costOf, modelRowsFor, handleWorkspaceUsage inline döngü)
  kopyalanmıştı → tek `priceStat(provider, model, st) (cost, save, priced, estimated)` çekirdeği;
  hepsi buradan geçiyor (~60 satır tekrar gitti). Harmonizasyon: sıfır-token slice artık `priced=true`
  (eskiden inline döngüde `false` olabiliyordu — gerçek harcaması olmayan satır "unpriced" işaretlenmez).
- **Tek summarize çekirdeği (`conversation/`):** `Manager.summarize` (db.Message) ve reactive'in
  `summarizeProviderMessages` (providers.Message) aynı provider-call+recordCompaction+trim bloğunu
  taşıyordu → ortak `summarizeRendered(existing, rendered)` + iki ince renderer
  (`renderDBMessages`/`renderProviderMessages`). `reactive.go`'dan `fmt` importu düştü.
- **Fold-sınırı yardımcısı (`conversation/manager.go`):** `Prepare` ve `ForceCompact`'taki
  `start/pending/fold` deseni → `foldBoundary(history, start, keepRecent) (fold, keepTail, newCount, ok)`
  + `clampStart`; `ForceCompact` artık "bütçesiz Prepare".
- **Test/build:** `conversation/manager_test.go` (foldBoundary/clampStart) + `api/budget_test.go`
  (priceStat 4 vaka) eklendi; `go build ./...` + `go test ./...` → **405 passed (28 paket)**.

## OpenRouter geçmiş cache breakpoint'i + per-model cache fiyatlandırması ✅ (2026-06-25)

Önceki cache çalışmasının üstüne iki iyileştirme.

- **Mesaj geçmişi breakpoint'i (`minimax.go attachHistoryBreakpoint`):** OpenRouter'da System
  prefix'inin yanına **2.** bir `cache_control` transkriptin sonundaki son düz-metin mesaja konuyor
  (tool_call taşıyan ve system mesajı atlanır). Böylece System + **tüm sohbet geçmişi** cache'lenir,
  yalnız en yeni mesaj taze gider (2/4 breakpoint). Uzun sohbetlerde token maliyetini ciddi düşürür;
  ilk tur cache-write, sonraki turlar cache-read.
- **Per-model cache çarpanı + OpenRouter fiyatları (`pricing.go`):** `Price`'a
  `CacheReadMultOverride`/`CacheWriteMultOverride` eklendi (0 → paket varsayılanı 0.10/1.25).
  `CostDetailed`/`CacheSavings` artık `p.cacheReadMult()`/`cacheWriteMult()` kullanıyor. `priceTable`'a
  **`openrouter`** bölümü: anthropic-routed modeller (opus/sonnet/haiku) Anthropic pass-through fiyatı +
  0.10/1.25 cache tier'ı ile; OpenAI/DeepSeek-routed için 0.25 override eklenebilir (yorumda örnek).
  Listelenmeyen openrouter modelleri unpriced (ekran ballpark).
- **Önizleme:** `computeCachePreview` openrouter modunda `cachedMsgCount = msgCount-1` (geçmiş yeşil,
  son mesaj taze) + güncel note.
- **Test/build:** `pricing_test.go` (override + openrouter fiyat) + `minimax_test.go` (geçmiş
  breakpoint) eklendi; `go test ./internal/{providers,api}/` 120 passed, `go build ./...` + frontend
  `tsc -b && vite build` yeşil.

## OpenAI-uyumlu yol prompt-cache — minimax/openrouter cache ölçümü + OpenRouter breakpoint ✅ (2026-06-25)

OpenAI-uyumlu sağlayıcılar (minimax, openrouter, özel OpenAI-compat) artık cache'i hem **ölçüyor**
hem de OpenRouter'da **istiyor**.

- **Ölçüm (`oaiUsage.toUsage()`):** usage objesinden prompt-cache token'ları ayrıştırılıyor —
  `prompt_tokens_details.cached_tokens` (OpenAI/OpenRouter) + `prompt_cache_hit_tokens` (MiniMax) →
  `Usage.CacheReadTokens`. Cached token `prompt_tokens`'in alt kümesi olduğu için **InputTokens'tan
  düşülüyor** (Anthropic konvansiyonu; `pricing.CostDetailed` cache read'i input üstüne ekler →
  çift sayım önlendi). `cache_creation_input_tokens` → `CacheWriteTokens` (ayrı sayım, düşülmez).
  Hem `Complete` hem `Stream` yolunda. → Usage/Budget ekranı minimax/openrouter cache tasarrufunu
  gösterir (eskiden hep 0'dı).
- **OpenRouter breakpoint (`buildSystemMessage` + `cachesSystem()`):** OpenRouter'da statik System
  prefix'ine `cache_control: {type: ephemeral}` konuyor (system content artık parça-dizisi:
  statik+breakpoint, dinamik breakpoint'siz — native anthropic `systemField` ile birebir). Anthropic/
  Gemini backend'lerinde cache'li; OpenAI/DeepSeek zaten otomatik. `oaiMessage.Content` `any` oldu;
  diğer endpoint'ler (MiniMax/Groq/Ollama) düz string content korur (array-form reddini önler).
- **Önizleme:** `computeCachePreview`'a `openrouter` modu eklendi → System/Tools cache'li gösterilir.
- **Test/build:** `minimax_test.go`'ya cache-parse (4 vaka: openai-details/minimax-toplevel/write/none) +
  OpenRouter breakpoint + MiniMax string-content testleri; `go test ./internal/{providers,api}/` 117 passed,
  `go build ./...` yeşil.

## Yedek arşivlerini UI'dan listeleme + tek-tık geri yükleme ✅ (2026-06-25)

Yedekleme özelliğine **geri yükleme** eklendi (önce yalnız alma vardı).

- **Backend liste/çözümleme (`internal/backup/restore.go`):** `Manager.ListArchives(targets)`
  (workspace başına arşiv, en yeni önce — ad/bayt/mtime), `Manager.ResolveArchive(wsID, name)`
  (ad doğrulama: yol ayıracı/`..` reddi → path-traversal guard). `Unzip` (`archive.go`,
  zip-slip korumalı, exported).
- **Lifecycle restore (`workspace.Manager.RestoreFromArchive(id, path, extract)`):** workspace'i
  ayır (scheduler `Stop`/runtime `CloseMCP`/DB `Close`) → arşivi **staging**'e aç → üst-düzey
  girdileri (store/config/workspace + ws-settings.json) `rename` ile **swap**'le → `open(meta)`
  ile diskten yeniden aç. Açma hatasında rollback (workspace dokunulmadan reopen). `extract`
  enjekte (`backup.Unzip`) → `workspace` paketi backup formatına bağımsız.
- **API:** `GET /api/backups/archives` + `POST /api/backups/restore` (`{workspaceId, archive}`;
  başarıda `publishWorkspacesChanged` → UI listeleri tazelenir).
- **UI (`BackupPanel`):** "Mevcut yedekler (N)" bölümü — workspace başına gruplu arşiv listesi
  (ad + tarih + boyut) + her arşivde **iki-adımlı onaylı "Geri yükle"** + yıkıcı işlem uyarısı.
  Tipler `BackupArchiveFile`/`WorkspaceArchives`, api `listBackupArchives`/`restoreBackup`.
- **Koşullu otomatik yenileme:** geri yüklenen workspace o pencerede **aktifse** sayfa otomatik
  yenilenir (`getActiveWorkspace()===id` → `window.location.reload()`); başka workspace'te yenileme
  yapılmaz (o workspace'e geçince zaten taze yüklenir → alakasız reload yok).
- **Ayrı "Yedekleme" sayfası (2026-06-25):** önce Ayarlar ▸ Gelişmiş altında alt-bölümdü; ayarlar +
  arşiv/geri-yükleme tek yerde toplansın diye **kendi kategori sayfasına** alındı (`primitives.tsx`
  `APP_CATS`'e `backup` katı + `Archive` ikonu, "Gelişmiş" ile "Komutlar" arası; `SettingsPanel.tsx`
  `cat==='backup'`). Üstteki ortak Kaydet butonu config'i yazar.
- **Arşiv silme (2026-06-25):** `Manager.DeleteArchive(wsID, name)` (ResolveArchive guard + `os.Remove`),
  `DELETE /api/backups/archives`, api `deleteBackupArchive`; UI her arşiv satırında **iki-adımlı onaylı**
  🗑 "Sil" butonu (Geri yükle'nin yanında). Test `TestDeleteArchive` (geçerli sil + traversal reddi);
  canlı API+UI: WS1/WS2 5→4 (disk+liste). `go test ./internal/backup` 5/5 yeşil.
- **Test/doğrulama:** `go test ./internal/backup` (4: +round-trip, +traversal reddi) yeşil; canlı
  WS1 (5 ajan/26 oturum) ve WS2 (9 ajan/11 oturum) API+UI'dan geri yüklendi → veri korundu,
  server sağlıklı. Detay: `_Docs\34-YEDEKLEME.md`.

## Workspace yedekleme — periyodik zip snapshot + saklama + manuel tetik ✅ (2026-06-25)

Her workspace'in tüm veri dizini (`store/`, `config/`, `workspace/`, `ws-settings.json`)
belirli aralıklarla bir zip arşivine alınır; saklanan sayı aşılınca eskiler budanır.
Süreç-geneli tek `backup.Manager` (workspace'ten bağımsız), ayarlardan canlı yapılandırılır.

- **Yeni paket `internal/backup`:** `backup.go` (Manager: `Configure`/`RunOnce`/`Stop`/`Status`,
  ticker döngüsü + run-mutex ile çakışma engeli + per-workspace saklama budama `prune`),
  `archive.go` (`zipDir` — göreli yol korumalı, backups kökünü dışlayıp özyinelemeyi önler).
  İlk otomatik yedek bir **aralık sonra** alınır (restart başına yedek patlaması yok).
- **Workspace köprüsü:** `workspace.Manager.BackupTargets()` her workspace'in mutlak veri
  dizinini döndürür (`open()` ile aynı yol mantığı: kullanıcı `Path`'i ya da varsayılan
  `rootDir/workspaces/<id>`).
- **Ayarlar (`settings`):** `backupEnabled` (vars. false), `backupIntervalHours` (≥1, vars. 24),
  `backupRetain` (≥1, vars. 7), `backupDir` (boş → `<dataDir>/backups`). Settings struct/DTO/Patch/
  Default/ToDTO/Apply + `normalize` clamp'leri eklendi.
- **Wiring:** `app.Bootstrap` manager'ı kurup `server.SetBackupManager` ile bağlar; `applySettings`
  her ayar değişiminde `backups.Configure(...)` çağırır (canlı başlat/durdur/yeniden-yapılandır);
  `App.Shutdown` döngüyü durdurur.
- **API:** `GET /api/backups` (durum: config + son koşu) + `POST /api/backups/run` (anında yedek,
  zamanlama açık olmasa da çalışır). Ajan aracı **değil** — yalnız kullanıcı/UI.
- **UI:** Ayarlar ▸ Gelişmiş ▸ **Yedekleme** bölümü (`BackupPanel`, `appPanels.tsx`): aç/kapat +
  aralık/saklama/klasör alanları + canlı durum kartı + **"Şimdi yedekle"** butonu. Tipler
  `types/settings.ts` (`BackupStatus`/`BackupResult`), api `system.ts` (`getBackupStatus`/`runBackup`).
  Tek manager **tüm** workspace'leri yedeklediği için app-geneli ayardır (workspace'e özel değil).
- **Build/test:** `go build ./...` + `go test ./internal/backup` (2: arşivle+budama, backups-kökü
  dışlama) yeşil; frontend `npm run build` (dist gömüldü). Detay: `_Docs\34-YEDEKLEME.md`.

## Market — uzak kayıt defteri (remote registry) + sürüm/güncelleme + detay popup ✅ (2026-06-25)

Market harici sunuculardan paket çekebilen 4. tier'a kavuştu (Faz 1-4, doğrulama opsiyonel).

- **Index formatı `swarmregistry/v1`:** uzak sunucu tek `registry.json` sunar (manifest + payload `url` + opsiyonel `sha256` + `minAppVersion`). Payload kurulum anında `url`'den lazy indirilir; sha256 verildiyse doğrulanır, yoksa atlanır.
- **Backend:** `internal/market/remote.go` (fetchIndex/fetchPayload, http(s)-only, boyut limiti 8/4 MiB, 20sn timeout, `compareVersions` semver-lite), `registry_store.go` (kaynak config `registries.json` global + index cache `.remote-cache/` restart-safe + per-workspace install ledger `installed.json`), `store.go` List/Get yerel+uzak birleştirir (`Source=remote`, id çakışmasında yerel gölgeler), `New` artık globalDir saklar. Pack'e `RegistryName`/`InstalledVersion` (+ transient `remoteURL`/`remoteSHA`).
- **API:** `GET/POST /api/market/registries`, `POST .../delete`, `POST .../refresh`; `GET /api/market` her pack'i ledger'dan `installedVersion` ile dekore eder; install başarısında `statusCaptureWriter` ile ledger'a `version` yazılır (tüm türler için tek nokta).
- **UI:** `RegistryManager.tsx` modal (kaynak ekle/sil/yenile, `market-registries-modal`); `MarketPanel.tsx`'e "Kaynaklar" butonu, kaynak rozeti (Yerel/registry adı), **"Güncelle (vX→vY)"** rozeti + detay popup'ında güncelleme butonu (overwrite). "Yenile" artık önce uzak index'leri çeker. Ayrıca **detay paneli yan-panelden ortada popup'a** çevrildi (`market-detail-modal`).
- **Doğrulama (uçtan uca):** yerel HTTP'de registry yayınlandı → ekle → uzak pack `source=remote` listede → kur (indirildi, skill oluştu) → `installedVersion=2.0.0` → registry 3.0.0 + refresh → "güncelleme var"=true. Backend `go build`+`go test` (59) yeşil, `tsc --noEmit` temiz, `npm run build` + binary derlendi, çalışan instance'ta test edildi.
- Detay: `_Docs\21-MARKET.md` §7.

## Market — 3 yeni paket türü (workspace/memory/mcp) + sol kategori menüsü + import taşıma ✅ (2026-06-24)

Market 4 türden 7 türe çıkarıldı ve ekran yeniden düzenlendi. (Aynı gün kısa süre `board` türü de
eklendi ama **kaldırıldı** — workspace şablonu zaten opsiyonel kanban düzeni taşıyor; ayrı board paketi
gereksiz bulundu.)

- **Yeni türler (install çalışır):** `workspace` (yeni workspace oluştur: `Manager.Create`+`UpdateSettings`,
  opsiyonel kanban düzeni dahil), `memory` (**seçilen ajana** tohum: `Runtime.Memory().Remember`),
  `mcp` (`db.CreateMCPServer`, enabled → sonraki turda araçlar görünür). Backend: `internal/market/pack.go`'ya
  kind sabitleri + payload struct'ları (`WorkspacePayload`/`MemoryPayload`/`MCPPayload` + `BoardColumn`
  [workspace şablonunun kanban düzeni için] + `MemoryEntry`; market paketi db'ye bağımlı kalmasın diye
  `BoardColumn` ayrı, API katmanı `db.BoardColumnDef`'e map'liyor); install handler'ları
  `internal/api/market.go` (`installMCPPack`/`installWorkspacePack`/`installMemoryPack` +
  `toBoardColumnDefs`/`mcpNames` helper'ları).
- **Memory ajan seçimi:** install body'sine `agentId` eklendi; `installMemoryPack` verilen ajanı hedefler,
  boşsa ilk ajana düşer, bilinmeyen id → hata. UI'da detay panelinde **hedef ajan dropdown'u**
  (`market-memory-agent`); ajan yoksa kur butonu pasif.
- **Gömülü örnekler (+5):** `mcp.filesystem`, `mcp.fetch`, `workspace.software-project`,
  `workspace.research`, `memory.coding-standards` (`internal/market/defaults/`, `//go:embed` ile gömülü;
  `EnsureDefaults` eksikleri global market dizinine yazar). Board örnekleri (scrum/bug-triage) eklenip geri
  silindi; global market dizinindeki kalıntılar da temizlendi.
- **UI (`MarketPanel.tsx`):** üst sekmeler → **sol dikey kategori menüsü** (7 kategori + paket sayacı);
  "Tümü" kaldırıldı, ilk kategori (Skills) varsayılan. Yeni türler için `PackPreview` (workspace yönergeleri +
  kanban chip'leri, MCP komut/args, bellek girdileri), `INSTALL_LABEL`, `KIND_LABEL`. memory "zaten kurulu"
  işaretlenmez (eylem); workspace/mcp ad-bazlı dedup (`api.listWorkspaces`/`listMCPServers`).
- **Import taşıma:** Claude Code skill içe aktarma (`SkillImportDialog`) **Skills ekranından markete taşındı**
  (Skills kategorisi başlığındaki "İçe Aktar" butonu). `SkillsPanel.tsx`'ten buton+dialog+state kaldırıldı.
- **Built-in tools sorusu:** yerleşik araçlar binary'e derili → markete eklenemez; araç paylaşımının doğru
  karşılığı **mcp** türü (cevap dokümana da işlendi).
- **Build:** `go build ./...` + `go test ./internal/market ./internal/api` (59) yeşil; frontend `tsc --noEmit`
  temiz + `npm run build` (dist gömüldü).
- Detay: `_Docs\21-MARKET.md`.

## Bütçe geliştirmeleri — Tasarruf Merkezi + session bazlı kullanım ✅ (2026-06-24)

Bütçe sistemi 3 fazda genişletildi: tasarruf görünürlüğü, oturum-başına atıf, birleşik kazanç paneli.

- **Faz 1 — Sıkıştırma tasarrufu görünür:** Sistem B'nin kırptığı bayt artık ölçülüyor
  (`Usage.CompactSavedBytesLLM` + `db.AddLLMCompactionSavings`; Sistem A'nın `CompactSavedBytes`'ı zaten vardı).
  `GET /api/usage` totals/cumulative/trend + `GET /api/agents/{id}/usage` bu alanları taşıyor.
- **Faz 2 — Session bazlı kullanım/maliyet:** yeni `db.SessionUsage` rollup (`internal/db/store_session_usage.go`,
  **sessionID anahtarlı ömür-boyu**, gün-reset yok; `store/session-usage/<sid>.json`). `RecordUsage` +
  `compactToolResult` ctx'teki `SessionIDFrom` ile ajan kaydının yanında session'a da yazıyor. Yeni endpoint
  `GET /api/sessions/{id}/usage-detail` (cost helper'ları workspace ekranıyla paylaşılır). `SessionDetailPanel`
  "Bu oturumun harcaması" kartı (maliyet + kazanç/tasarruf kırılımı).
- **Faz 3 — Tasarruf Merkezi:** Bütçe ekranında tüm tasarruf kaynaklarını birleştiren panel (cache USD +
  Sistem A/B bayt + ~token eşdeğeri).
- **Notlar:** hook'lar tasarruf ölçmez (CC sözleşmesi); `context-mode`/`rtk`/`sqz` presence-only → ölçülen kazanç yok,
  yerel eşdeğer Sistem A. Sistem A/B yalnız bayt+~token gösterir (USD'ye çevrilmez — uydurma sayı olmaması için);
  gerçek USD yalnız prompt-cache'te. Bu cache USD'si az önceki claude-cli cache muhasebesi düzeltmesinden de
  beslenir (claude-cli turları artık cache read/write raporladığından session rollup'a da yansır).
- **Build/test:** `go build ./...` + `go vet` yeşil; `internal/db` (`store_session_usage_test.go` +
  `AddLLMCompactionSavings`), `internal/api`, `internal/agent` testleri (168) geçti; frontend `tsc --noEmit` temiz.
- Detay: `_Docs\17-TOKEN-OPTIMIZASYON.md` §Bütçe görünürlüğü.

## Bağlam önizleme cache haritası — cache dışı segmentler yeşil + cache sınırı ✅ (2026-06-24)

Önizleme ekranı (`SessionContextModal`) artık isteğin hangi parçasının **sıcak prompt-cache**'ten,
hangisinin her tur **taze** gittiğini gösterir.

- **Backend (`internal/api/session_context.go`):** `sessionContextPreview`'a `cache cachePreview`
  alanı + `computeCachePreview(provider, session, …)`:
  - **anthropic** (ExtendedPromptCache açık): breakpoint statik System bloğunda →
    `toolsCached`+`systemCached=true`, `dynamicCached=false`; kapalıysa `mode=none`.
  - **claude-cli** (ClaudeResume warm, `CLISessionID`+`CLISentMsgCount`>0): `systemCached=true`,
    `cachedMsgCount=CLISentMsgCount` → ilk N mesaj sıcak, delta taze; soğuk/ilk tur `mode=none`.
  - diğer sağlayıcılar `mode=none`. Her mod insan-okunur `note` taşır.
- **Frontend (`SessionContextModal.tsx` + `types/session.ts`):** cache dışı segmentler **hafif
  yeşil** (`text-emerald-500/75`, Markdown düz metni `currentColor`'dan miras alır); her bölümde
  `cache'li`/`cache dışı` pill (`CacheTag`), üstte yeşil legend (`note` ile), mesaj dizisinde
  ilk taze mesajdan önce **"cache sınırı — buradan sonrası taze gönderilir"** ayıracı.
- **Build/test:** `go build ./...` + `go vet` + `go test ./internal/{api,providers}/` (110 passed);
  frontend `tsc -b && vite build` yeşil, binary'e gömüldü.

## claude-cli cache muhasebesi — Usage ekranında resume tasarrufu görünür ✅ (2026-06-24)

`--resume` (ClaudeResume) modunun asıl faydası **prompt-cache hit**'idir, ama claude-cli
usage ayrıştırıcısı cache alanlarını okumuyordu → Usage/Budget ekranı her claude-cli turu için
**0 cache** gösteriyor, resume'un değeri görünmüyordu.

- **Düzeltme (`internal/providers/claudecli.go`):** `cliUsage` struct'ına
  `cache_read_input_tokens` + `cache_creation_input_tokens` alanları eklendi; `assistant`
  event'inde max-bağlama (per-message), `result` envelope'unda authoritative aggregate ile
  `resp.Usage.CacheReadTokens`/`CacheWriteTokens`'e yazılıyor.
- **Zincir doğrulandı:** `resp.Usage` → `RecordUsage` (budget.go:57) → `AddUsageKind` →
  `store_usage` aggregation → `GET /api/usage` (`cacheReadTokens`/`cacheWriteTokens`). Anthropic
  native ile aynı yoldan akar; artık claude-cli turları da cache read/write raporlar.
- **Build:** `go build ./...` + `go vet ./internal/providers/` yeşil.
- **Kapsam dışı (sıradaki):** bağlam önizleme ekranında (`context/preview`) cache-breakpoint
  etiketi + resume modunda "N mesaj CLI cache'inde sıcak, yalnız delta gönderilecek" satırı —
  henüz eklenmedi.

## SK-IMP UI — Skills panelinde içe-aktarma akışı ✅ (2026-06-24)

SK-IMP'in son parçası: importer artık UI'dan kullanılıyor (SK-IMP tamamen tamamlandı).

- **Frontend:** `SkillImportDialog.tsx` (kaynak seçici GitHub/yerel, location, opsiyonel slug, paylaşımlı
  toggle) → `api.importSkill` → sonuç kartı (slug + kopyalanan dosyalar + uyarı listesi). Skills panelinde
  ("İçe Aktar" butonu, `skills-import` testid) açılır; başarıda liste yenilenir + içe-aktarılan skill seçilir.
- **API client:** `skillApi.importSkill` + `SkillImportResult`/`SkillImportResponse`/`SkillImportInput`
  tipleri (`api/skills.ts`).
- **Doğrulama:** `tsc --noEmit` + `vite build` yeşil; full build (frontend gömülü) 8090'a deploy edildi.
  Çağrılan endpoint (`POST /api/skills/import`) zaten SK-IMP.2/3'te canlı doğrulanmıştı.
- Not: `Github` ikonu lucide sürümünde yok → `Globe` kullanıldı (build hatası giderildi).

## SK-IMP.3 — Importer: GitHub kaynağı + import_skill aracı ✅ (2026-06-24)

- **GitHub kaynağı (`internal/skills/github.go`):** `parseGitHubURL` (tree/blob → owner/repo/ref/dir,
  blob+SKILL.md → parent), `fetchGitHubSkill` GitHub contents API ile SKILL.md + top-level bundled
  dosyaları çeker (host-allowlist: api.github.com/github.com/raw.githubusercontent.com/codeload; 8MB cap,
  30sn timeout, anon). `Store.ImportFromSource(source, location, slug, shared)` local|github ayrımını yapar;
  `readLocalSkillDir` api'den buraya taşındı. API `POST /api/skills/import` artık `url` alanını da kabul eder.
- **`import_skill` self-management aracı:** `SkillWriter.ImportSkill` + `tools.SkillImportResult` +
  `ImportSkillTool` (`builtin_skillmgmt.go`); `agentSkillWriter.ImportSkill` → `ImportFromSource`;
  `toolsetup`'ta create/update/delete yanında kayıtlı (CLI bridge üzerinden de erişilir).
- **Testler:** `import_test.go` yeşil; `builtin_skillmgmt_test.go` fake'e `ImportSkill` eklendi.
  `go build ./...` + `go test ./internal/{skills,tools,agent,api}/` yeşil.
- **Canlı doğrulandı (8090):** (1) GitHub `anthropics/skills/.../skill-creator` → source_url + bundled
  LICENSE.txt kopyalandı, sıfır uyarı; (2) claude-cli ajanı `import_skill`(local `gsd-capture`) → slug +
  `$ARGUMENTS` uyarısı birebir döndü. Test artefaktları silindi.
- **Kalan:** Market/Skills UI içe-aktarma akışı.

## SK-IMP — Claude Code skill importer: çekirdek + local API ✅ (2026-06-24)

Seviye 2 importer'ın ilk iki increment'i. Önkoşullar SK-1..SK-4 hazırdı.

- **Çekirdek (`internal/skills/import.go`):** `mapCCSkill(raw, sourceURL, shared)` CC frontmatter'ını
  SwarmGo'ya eşler — name/description/when_to_use→aynı, `allowed-tools`→`always_allow`, `paths`→koşullu,
  version/license→aynı, source_url=import kaynağı (provenance), `disable-model-invocation:true`→shared
  değil, `user-invocable`→`user_invocable`. Uyumsuzu (`context:fork`, `hooks`, `model`/`agent`/`effort`,
  slash-arg `$ARGUMENTS`/`$1`, inline-shell `` !` ``) ayıklayıp **warning** döndürür. `Store.ImportCCSkill`
  rendered SKILL.md + bundled dosyaları (yalnız düz dosya adı; path-traversal reddi; SKILL.md hariç)
  workspace tier'a yazıp reload eder, slug çakışmasında hata verir.
- **API:** `POST /api/skills/import` (`source:"local"`, `path`, opsiyonel `slug`/`shared`) →
  `readLocalSkillDir` (SKILL.md + sibling dosyalar) → `ImportCCSkill` → `{result, skill}` döner
  (`handleImportSkill`, `server.go` route).
- **Testler:** `import_test.go` (mapping + disable-invocation downgrade + write/resolve + path-traversal
  reddi + duplicate). `go build ./...` + `go test ./internal/{skills,api}/` yeşil.
- **Canlı doğrulandı:** gerçek `~/.claude/skills/gsd-add-tests` 8090'a import edildi →
  `always_allow=Read;Write;Edit;Bash;Glob;Grep;Agent;AskUserQuestion`, source_url set, `$ARGUMENTS` uyarısı,
  shared=false. Test artefaktı silindi.
- **Kalan (SK-IMP.3):** GitHub kaynağı (URL→fetch), `import_skill` self-management aracı (agent-usable),
  Market/Skills UI akışı. Detay: `03-YOL-HARITASI.md` SK-IMP.

## Skill sistemi geliştirmeleri SK-1..SK-4 (CC skill importer önkoşulları) ✅ (2026-06-23)

Claude Code skill importer'a (Seviye 2, `03-YOL-HARITASI.md` SK-IMP) hazırlık olarak, kendi skill
sistemimizde 4 önkoşul uygulandı. Kaynak desen: `observed-behavior/src/skills/loadSkillsDir.ts`.

- **SK-1 — Çok-dosyalı skill:** `Store.UseSkillBody` gövdede `${SKILL_DIR}` (+ CC `${CLAUDE_SKILL_DIR}`)
  ikamesi yapıyor → skill kendi klasöründeki dosyalara atıf verir; **sibling dosyalar** "## Bundled files"
  footer'ıyla ajana ilan edilir (Read ile on-demand). Raw `Body` (edit/detay) literal kalır.
  `substituteSkillVars`/`bundledFilesFooter` (`store.go`).
- **SK-2 — Ölçeklenebilir keşif:** `Skill.Paths` (`paths:`) → **koşullu skill** auto-advertise'dan çıkar
  (prompt şişmez), loadable kalır; `Store.Search` + yeni **`skill_search`** aracı (`builtin_skillsearch.go`,
  native `toolsetup.go` + CLI bridge `mcp_interaction.go`). fs-touch OTOMATİK aktivasyon ertelendi
  (session-scoped state gerekir).
- **SK-3 — `allowed_tools` enforcement:** `use_skill` yüklenince skill'in `always_allow` desenlerini
  `ParsePermRule` ile oturum grant'larına (`GrantsFrom(ctx)`) ekler → araçlar re-prompt'suz; çıktıya
  şeffaflık notu. `SkillLibrary.AllowedTools` + `agentSkillLib.AllowedTools`.
- **SK-4 — Provenance frontmatter:** `Skill.Version/SourceURL/License/UserInvocable` parse + serialize
  (importer'da köken/güncellik izi). `user-invocable` default true.
- **CLI bridge (canlı testte bulunup tamamlandı):** `skill_search` ve SK-3 auto-grant ilk uygulamada
  yalnız native yoldaydı; claude-cli ajanlarının Interaction MCP köprüsünde eksikti. Eklendi:
  `Runtime.SearchSkillsForAgent`/`SkillAllowedToolsForAgent`, `chatRun.skillSearch`/`skillAllow`
  setter'ları (chat_stream + autonomous_interaction'da kurulur), `mcp_interaction.go` dispatch
  case `skill_search` + `callSkillSearch` + `grantSkillToolsCLI`. `renderCatalog` ajana skill_search'ü
  hatırlatan satır ekler (koşullu skill'ler katalogda yok). **Not:** CLI bridge yalnız streaming
  `/api/chat/stream` yolunda kurulur; non-streaming `/api/chat` interaction tool'larını bağlamaz.
- **Doğrulama:** `go build ./...` + `go test ./internal/{skills,tools,agent,api}/` yeşil; yeni testler
  `TestUseSkillBodySK1`/`TestSearchAndConditionalSK2`/`TestUseSkillGrantsToolsSK3`/`TestRichFrontmatterSK4`.
  **Canlı (claude-cli, /api/chat/stream):** SK-1 `${SKILL_DIR}`+bundled path, SK-2 `skill_search`→koşullu
  `sk-demo` (katalogda yok), SK-3 "_auto-allowed: Bash(git *)_", SK-4 API parse — hepsi doğrulandı.
  Sırada: **SK-IMP** (gömülü importer).

## Vite bundle temizliği + API E2E smoke testi ✅ (2026-06-23)

**Vite:** Production build'in "chunks larger than 500 kB" uyarısı temizlendi.
`vite.config.ts`'e `manualChunks` eklendi — ağır vendor kütüphaneleri ana
bundle'dan ayrıldı: `vendor-highlight` (highlight.js ~152 kB), `vendor-markdown`
(react-markdown/remark/rehype ~161 kB), `vendor-react` (~359 kB). Ana `index`
chunk'ı **969 kB → 472 kB**'a indi (artık limitin altında). Tek kalan büyük chunk
`relationGraph` (vis-network ~522 kB, zaten lazy) için `chunkSizeWarningLimit: 600`
ayarlandı — bilinçli lazy vendor chunk'ı için dürüst susturma. Build temiz, uyarı yok.

**E2E smoke testi:** Repodaki ilk otomatik test — `scripts\e2e-smoke.ps1` (**12 adım**).
Doc 33'teki HTTP API yolunu baştan sona doğrular: health → **CORS preflight (`*`+auth-yok)**
→ **hata sözleşmesi (400/404 `{error}`)** → workspaces → agents → session →
**workdir round-trip (cwd set→git→reset)** → **gerçek LLM chat turu (SSE `done`+yanıt)**
→ **çok-turlu bağlam sürekliliği (codeword recall)** → **`permissionMode=read-only`
override turu** → kalıcılık (8 mesaj/4 tur) → cleanup. Canlı sunucuya karşı `AGT3`
(claude-cli/haiku) ile **12/12 geçti** (LLM `ZEPHYR-7` codeword'ünü 2. turda hatırladı);
`-SkipLLM` ile 9/9 (smoke-only, ucuz). CI dostu exit kodu. Detay: `_Docs/33` §A.6.

## Lazy araç kataloğu MCP açıklamalarını kısaltıyor (bağlam şişmesi fix) ✅ (2026-06-23)

Sorun: Sistem promptundaki **"Available Tools (load on demand)"** bloğu, lazy MCP
araçlarının **tam, çok-paragraflı açıklamasını** (`e.Tool.Description`) basıyordu. Gateway
gibi sunucuların açıklamaları "When to use / When NOT to use" rehberleriyle dolu →
her lazy araç için bu metin bağlama giriyor, sistem promptu şişiyordu (ve Windows'ta
claude-cli komut satırını 32K limitine itiyordu — yukarıdaki fix'in tetikleyicisiyle
aynı kök bloat).

Çözüm (`internal/tools/registry.go` `LazyCatalog`):
- Lazy blok bir **isim + kısa özet** teaser'ıdır; tam açıklama araç `activate_tools`
  ile yüklenince zaten şemada (`Defs`) geliyor. Yeni `lazyDescription` helper'ı her
  açıklamayı **ilk anlamlı satıra** indirip `lazyCatalogDescMaxChars=200` ile kapıyor
  (UTF-8 sınırında). Hem native hem MCP lazy araçlarına uygulanıyor.
- Sonuç: gateway gibi yüzlerce araçlı sunucularda load-on-demand bloğu dramatik küçülür.
- Build + `internal/tools` testleri yeşil.

## claude-cli sistem promptu artık dosyadan veriliyor (Windows 32K arg limiti fix) ✅ (2026-06-23)

Sorun: Otonom/flow turlarında claude.exe başlatılırken
`provider error: fork/exec ...claude.exe: Dosya adı veya uzantısı çok uzun.`
(Windows hata 206 / `ERROR_FILENAME_EXCED_RANGE`). Kök neden: `claudecli.go`
sistem promptunu `--append-system-prompt <sys>` ile **komut satırı argümanı** olarak
geçiriyordu. Sistem promptu (skills + core memory blokları + dinamik bağlam) büyüyünce
Windows'un ~32.767 karakterlik komut satırı limiti aşılıp süreç hiç başlamadan çöküyordu.
Belirti flow'larda ardıl `node "<X>" (agent): context canceled` olarak da görünüyordu
(paralel düğüm çökünce ortak context iptal edilir).

Çözüm (`internal/providers/claudecli.go` `Complete`):
- Sistem promptu artık `os.CreateTemp` ile bir temp dosyaya yazılıp
  **`--append-system-prompt-file <path>`** bayrağıyla veriliyor → komut satırında yalnız
  kısa bir yol taşınıyor, limit aşımı imkânsız. (Sohbet promptu zaten stdin'den gidiyordu.)
- Temp dosya `defer os.Remove` ile her iki retry denemesi bitince siliniyor.
- Gereksinim: claude CLI'nin `--append-system-prompt-file` desteği (2.1.186'da mevcut;
  `--bare` yardımında `--append-system-prompt[-file]` belgeli).
- Build + `go vet` + `internal/providers` testleri yeşil.

## "Gizli" çip artık gerçek context durumunu yansıtıyor + self-management'ı kapsıyor ✅ (2026-06-23)

Sorun: self-management araçları (ajanın SwarmGo'yu kontrol eden tool'ları) kodda
zorla `MarkHidden` olduğu için context'te görünmüyordu, ama Araçlar ekranındaki
"Gizli" çip yalnızca kullanıcının `HiddenTools` listesini yansıtıyordu → bu araçlar
çipsiz "normal" görünüyordu (yanıltıcı) ve kullanıcı bunları context'e alamıyordu.

Çözüm — çift yönlü görünürlük override'ı:
- **Çip artık efektif lazy durumunu gösteriyor.** API `hidden` alanı registry'nin
  gerçek `IsLazy` durumundan geliyor (`WorkspaceToolCatalogWithState`): kod-default
  lazy (self-management/MCP/read_config/WebFetch…) + kullanıcı override'ları. Yani
  context'e her tur gitmeyen her araç "Gizli" rozeti alır.
- **`ShownTools` override'ı eklendi** (`WorkspaceToolConfig`): default gizli bir aracı
  (özellikle self-management) **zorla context'e** geri alır. `registry.Unlazy` lazy+hidden
  işaretlerini siler; toolsetup'ta **en son** uygulanır (tüm default + MCP lazy'yi ezer).
- **Toggle çift yönlü:** "Göster" → `ShownTools`'a ekle / `HiddenTools`'tan çıkar;
  "Gizle" → tersi. Her araç için çalışır (kod-gizli self-management dahil).
- API PUT üç listeyi de per-field merge eder (`disabled`/`hidden`/`shown`).
  Frontend `setWorkspaceToolsVisibility(hidden, shown)`.
- Test: `TestUnlazyOverridesHidden` (registry), db round-trip'e ShownTools. 238 test yeşil.

## Araçlar ekranında "Gizli" (load-on-demand) çip + toggle ✅ (2026-06-23)

Skills ekranındaki "Gizli" (auto-summary off) deseninin **araçlara** karşılığı eklendi.
Bir araç "Gizli" işaretlenince **aktif kalır** ama şeması her tur ajana gönderilmez —
ajan gerektiğinde `tool_search`/`activate_tools` ile çeker (= `MarkLazy`). `enabled`
(devre dışı) toggle'ından bağımsız, ikinci bir eksen.

- **db** (`store_tools.go`): `WorkspaceToolConfig.HiddenTools []string` (DisabledTools'tan
  ayrı, persist + reload). 
- **API** (`workspace_tools.go`): per-tool `hidden` flag + yanıtın `hiddenTools` dizisi;
  `PUT /api/workspace-tools` artık **per-field merge** (pointer'lı req → yalnız gelen
  liste değişir, diğerine dokunmaz; enable & hide toggle'ları çakışmaz).
- **toolsetup** (`toolsetup.go`): tur kurulumunda `reg.MarkLazy(cfg.HiddenTools...)` —
  builtin + MCP araçlarında çalışır (MarkLazy sıra-bağımsız isim seti).
- **Frontend** (`ToolsPanel.tsx`): listede ve detayda **"Gizli" çipi** (SkillsPanel'in
  `SummaryOffBadge` stiliyle aynı warning rengi), detayda **Gizle/Göster** (Eye/EyeOff)
  toggle'ı (optimistic + revert). `api.setWorkspaceToolsHidden`, tipler `hidden`/`hiddenTools`.
- Test: `store_tools_test.go` (hidden round-trip + disabled'dan bağımsızlık). Tüm testler yeşil.

## Self-correcting hata kapsamı taraması — kalan boşluklar kapatıldı ✅ (2026-06-23)

242 builtin tool hata mesajı tarandı. Çoğu zaten iyiydi (`use list_X`, geçerli
değer listeleri, argErr/enumErr/cron/graph hint'leri). Atlanan grup: meta/interaction
araçları `invalid <tool> input: %w` (argErr'in eşleştiği `invalid arguments: %w`'den
farklı kelime → toplu değişimde kaçmış), eyleme dönük "fix:" eki yoktu.

- Yeni `argErrFor(tool, err)` (`builtin_errhints.go`): tool adını korur + aynı şema
  ipucunu ekler. **12 site** dönüştürüldü: activate/deactivate/tool_search,
  create/update_artifact, ask_user, request_confirmation, send_message, use_skill,
  spawn_session, todo_write, run_subagent (`subagent.go`).
- Edit araçları (`builtin_fs.go`): `old_string not found` → "Read the file first,
  copy exact text incl. whitespace (no line-number prefixes)"; `identical` →
  "make new_string differ". Kör retry'ı önler.
- Bilinçli dokunulmayanlar: terse-ama-net `X is required` (çözüm zaten örtük) ve
  `… not available in this context` (ortam hatası, input'la düzeltilemez).
- Test: `argErrFor` için case eklendi. Tüm tool/provider testleri yeşil.

## Hooks paneli: harici araç oto-tespit + tek-tıkla bağla toggle'ı ✅ (2026-06-23)

Ayarlar ▸ Hooks ekranı (`HooksPanel.tsx`) iki iyileştirme aldı:
- **Oto-tespit:** Harici token araçları (`rtk`/`sqz`/`context-mode`)
  artık **ekran açılır açılmaz** otomatik kontrol ediliyor (`useEffect`'e `checkTools()`
  eklendi); eski "Kurulu mu kontrol et" butonu yeniden-tarama için korundu. Tespit
  hâlâ presence-only (`/api/external-tools` → `exec.LookPath`, çalıştırma/kurulum yok).
- **Tek-tıkla bağla toggle'ı:** Bulunan her hook-tabanlı araç için **Bağla / Aktif /
  Pasif** düğmesi. `TOOL_HOOK_TEMPLATES` şablonundan ilgili hook'u oluşturur
  (`rtk`→PreToolUse/`Bash` PowerShell rewrite adapter; `sqz`→PreToolUse/`Bash`,
  komut `sqz hook claude` — rtk gibi bash-rewrite, ikisini aynı anda Bash'te açma),
  tekrar tıklayınca `toggleHook` ile aç/kapat (silmez).
  `context-mode` MCP tabanlı (sandbox + FTS5 KB) olduğu için toggle yerine **MCP**
  rozeti gösterilir (hook değil; Ayarlar ▸ MCP'den eklenir). `wiredHook()` eşlemeyi
  komut içeriğinden yapar. Otomasyon için `data-testid="tool-toggle"` + `data-tool` eklendi.
- Doğrulama: frontend `tsc --noEmit` yeşil. Not: prod embed için `npm run build`
  + Go yeniden derleme gerekir (dev'de Vite HMR yeterli).

## `update_skill` self-management aracı ✅ (2026-06-23)

Ajanlar bir skill'i değiştirmek için `delete_skill`+`create_skill` yapmak zorundaydı
(SES2'de tam bunu yaptı — riskli, dangling-reference doğurabilir). Artık **`update_skill`**
var: slug ile in-place düzenleme, **partial** semantik (yalnız değişen alanları geç —
name/description/whenToUse/body/shared; verilmeyen alan korunur). Yalnız **workspace-tier**
skill düzenlenebilir (global/bundled korunur). Mimari:
- `tools.SkillWriter` arayüzüne `UpdateSkill(slug, name,desc,when,body *string, shared *bool)`
  eklendi (pointer = nil → değişme). `UpdateSkillTool` (`builtin_skillmgmt.go`),
  örnekli şema + boş-güncelleme/slug guard'ları.
- `agentSkillWriter.UpdateSkill` (`runtime.go`): `store.Get`+`store.Body` ile mevcut
  değerleri okuyup merge eder, tier guard (`Source==workspace`), `store.Update` çağırır.
- `toolsetup.go`: create/delete arasına eklendi → self-management aralığında olduğu için
  otomatik **hidden-lazy** (cached prefix'i şişirmez; `tool_search`/`activate_tools` ile erişilir).
- Doc: `swarmgo-self-management` SKILL.md güncellendi. Test: `builtin_skillmgmt_test.go`
  (partial forwarding + validation). Toplam testler yeşil.

## Dış-ajan otomasyon dostluğu — UI seçicileri + API rehberi ✅ (2026-06-23)

Soru: "SwarmGo'yu dışarıdan ajanlar (chrome-mcp/playwright-mcp) baştan sona kullanabilir mi,
eksik/iyileştirilecek yer var mı?" İki yol değerlendirildi:

- **HTTP API yolu zaten eksiksiz (9/10):** 138+ endpoint tüm alt sistemleri kapsıyor, **auth yok**
  (`server.go:withCORS`), **CORS wildcard açık**, SSE `chat/stream` net terminal sinyali veriyor
  (`meta→agent→step→reply→done|error`, `chat_stream.go`). Dış ajan API ile uçtan uca sürebilir.
- **UI tıklama yolu kırılgandı (6/10):** `data-testid` yok, DOM'da tamamlanma sinyali yok,
  modaller `role=dialog` taşımıyordu.

**Uygulanan (UI otomasyon dostluğu, additive — yalnız attribute):**
- **`data-testid` haritası:** NavRail (`nav-{view}`/`nav-workspace`/`nav-settings`), Composer
  (`composer-input`/`-send`/`-stop`/`-queue`/`-interrupt`/`-steer`/`-attach`), AgentSelect
  (`agent-select`/`-menu`/`-option`+`data-agent-id`), WorkspaceSwitcher (`workspace-switcher`/
  `-menu`/`-row`+`data-workspace-id`/`-switch`/`-create`), MessageList (`chat-transcript`/`chat-message`).
- **Tamamlanma sinyali:** `chat-message` satırında `data-role` + **`data-streaming="true|false"`**
  (MessageList'te `rowLive` hoisted) → otomasyon turun bittiğini DOM'dan okur, polling yok.
- **Erişilebilirlik:** `chat-transcript` `role=log`+`aria-live=polite`; AgentSelect/WorkspaceSwitcher
  `role=listbox`/`option`+`aria-expanded`; 6 modal (`AgentSettings`/`AgentContext`/`SessionContext`/
  `SkillEditor`/`WorkspaceCreate`/`SpawnSession`) `role=dialog`+`aria-modal`+`aria-label`+testid;
  NavRail butonları `aria-label`+`aria-current`.
- `npx tsc --noEmit` yeşil (EXIT=0). Hiç davranış değişmedi, yalnız işaretleme eklendi.

**Yeni doküman:** `_Docs\33-DIS-AJAN-OTOMASYONU.md` — iki yol karşılaştırması, API uçtan-uca akış,
SSE tüketim örnekleri, testid haritası, playwright/chrome-mcp reçeteleri, bilinen sınırlar.

## Panel-içi form testid'leri — Faz 1+2+3 ✅ (2026-06-23)

Önceki fazda yalnız çekirdek sohbet/nav/workspace/modal akışları testid taşıyordu; panellerin iç
formları taşımıyordu (dış ajan API'den yönetebiliyor ama UI'dan kırılgan). Tüm panel formlarına
`data-testid` eklendi — 4 paralel subagent ile (dosyalar disjoint, çakışma yok), additive (yalnız
attribute, davranış/stil değişmedi).

**Konvansiyon:** statik = `{panel}-{eylem}`; dinamik liste öğesi = sabit `data-testid` + ayrı
`data-{entity}-id` (nth yerine id ile hedefleme). Özel bileşenlere (`<Button>`/`<AgentPicker>`/
`<ProviderModelSelect>`) gerekince layout-nötr saran `<div data-testid>` (ör. `*-wrap`).

**Kapsam (~146 yeni, toplam ~183 testid / 32 dosya):**
- **Agents:** AgentRoster (7), AgentSettingsForm (14), ProviderModelSelect (5), AgentToolsSection (4),
  AgentSkillsSection (5), AgentPicker (2), EmojiPicker (4).
- **Tasks:** TaskBoard (7), TaskDetailPanel (9), BoardColumnEditor (10).
- **Schedules+Settings:** Schedules (15), ProvidersPanel (14), ToolsPanel/MCP (14), HooksPanel (8).
- **Diğer paneller:** SkillsPanel (8), MarketPanel (3), SecretsPanel (7), MemoryPanel (6),
  ArtifactsPanel (12), FlowCanvas (2).

`npx tsc --noEmit` yeşil (EXIT=0). Seçici haritası `_Docs\33-DIS-AJAN-OTOMASYONU.md` §B.2'ye işlendi.

**Gerçek E2E doğrulama (2026-06-23):** headless Playwright (kurulu Chrome) ile canlı uygulama
(Vite :5173 → backend :8090) sürüldü; MCP köprüsü o an dalgalandığı için doğrudan Playwright
kullanıldı. 16/16 kontrol geçti: NavRail 15/15, chat composer (input/send/agent-select), tüm panel
CRUD testid'leri ve **gerçek etkileşim** (ajan oluştur formuna ad yazıp geri okuma). Test bir boşluk
yakaladı: "Ajanlar" ekranı `AgentRoster` değil **`AgentsView`** render ediyor; bu bileşen testid'siz
kalmıştı → AgentsView roster/create formuna testid eklendi (commit eafe718). **Sıradaki (ertelendi):**
ağa açılırsa API auth katmanı.

## Yapılandırılmış konuşma-özeti — Claude Code parite 1. faz (compact decay fix) ✅ (2026-06-23)

WS2/SES2'de kullanıcı compact'in "çok kısa özet" ürettiğini ve "compact sonrası hâlâ
mesaj eklendiğini" bildirdi. Teşhis: ikincisi tasarım (kayan pencere, `keepRecent=8` +
yeni turlar birikir — normal); birincisi `conversation/manager.go` `compactPrompt`'undaki
**"under 200 words"** cap'i + her katlamada **özetin özetini** alan rolling-merge →
**decay**.

Claude Code compaction motoru incelendi (`Desktop/Projects/observed-behavior/src/services/
compact/`: `prompt.ts` 9-bölümlü + `<analysis>` scratchpad, `compact.ts`, `autoCompact.ts`,
~20K output rezervi, fork+prompt-cache paylaşımı, post-compact dosya/skill re-injection).

**1. faz uygulandı (düşük risk, en yüksek etki):**
- `compactPrompt` → sabit **8 bölümlü** yapı + **anti-decay talimatı** ("önceki özetteki her
  kalıcı gerçeği taşı, kısaltma"). 200-kelime cap kaldırıldı. İki `%s` korundu → `reactive.go`
  aynı sabiti kullanmaya devam.
- `compactMaxOutputTokens = 8192`; `summarize` + reactive yol `Request.MaxTokens` ile geçiyor
  → uzun özet anthropic 4096 default'unda kesilmiyor.
- `go build ./...` + `internal/conversation` testleri yeşil. Detay: `_Docs/17` §8.

**Bilinçli ertelendi:** fork/cache (SwarmGo özetleyiciye yalnız katlanan dilimi yollar →
çağrı zaten ucuz, fork'un çözeceği pahalılık yok; claude-cli cache paylaşımını kontrol edemez).

## Post-compact kurtarma işaretçisi — Claude Code parite 2. faz ✅ (2026-06-23)

CC compact sonrası transcript pointer + son okunan dosya re-injection yapar. SwarmGo'ya
**birebir port mimariye ters:** turlar arası yalnız `role+text` taşınır (`toProviderMessages`)
→ tool sonuçları/dosya okumaları zaten cross-turn context'te değil; ajan serbest fs ile
istediğinde yeniden okur. Kalıcı durum (artifacts/todos/core-memory/goal/summary) zaten her
tur re-inject ediliyor.

**Uygulanan:** özet bloğu `conversationSummaryBlock(summary)` ile sarıldı (`api/chat_turn.go`)
→ özetin altına **kurtarma notu**: "önceki turlar katlandı, tam metni yok; kesin detay lazımsa
tahmin etme — `conversation_search` ile ara veya dosyaları fs araçlarıyla yeniden aç". Compact
sonrası ajan körleşmez. `readFileState` tracker bilinçle eklenmedi (mimariye gereksiz). `go build`
+ `internal/api` + `internal/conversation` testleri yeşil. Detay: `_Docs/17` §9.

**Sırada (opsiyonel 3. faz):** partial compact (`from`/`up_to`) + boundary UI; veya tam decay-sıfır
için merge yerine `history` prefix'inden sıfırdan özetleme (fork tartışmasına bağlı).

## SES5 flow çöküşü teşhisi: crash-tail kuyruğu + create_agent provider default ✅ (2026-06-23)

WS2/SES5'te "Seyahat Planlama Akışı" flow'u node'larda `claude CLI failed: exit
status 1` veriyordu (sıralıda flight başarılı → hotel hata; paralelde flight hemen
hata — yani **intermittent**, deterministik değil).

**Eleme:** gsd `SessionStart` hook'ları (`gsd-check-update.js`/`gsd-session-state.sh`)
exit 0 ile bitiyor → sebep değil. 4× eşzamanlı **düz** claude-cli hepsi exit 0 →
ham eşzamanlılık/`~/.claude.json` çakışması da değil.

**Asıl yön:** Flow agent'ları (AGT6–9) `mcpEnabled=true` (create_agent'ta
**hardcoded**) + workspace `enableCliHooks=true` → claude-cli'a fazladan
`--mcp-config` (interaction MCP) + `--settings` (hook'lar) + `--permission-prompt-tool`
geçiliyor; çöküş bu kırılgan yolda. Tam başarısız bileşen crash-tail'de gizliydi.

**İki düzeltme:**
1. **crash-tail artık ölümcül SONU gösteriyor** (`stdoutCrashTail`, `claudecli.go`).
   Önceki sürüm baştan 600 char kırpıyordu → yalnız hook startup gürültüsü
   görünüyordu. Yeni sürüm stream-json `result`/`is_error`/`error` event'lerini ve
   plain panic satırlarını öne çıkarır, kırparken **son 800 char**'ı korur. Böylece
   "mcp server failed" gibi gerçek sebep mesaja düşer.
2. **`create_agent` boş provider bırakmıyor** (`builtin_agentmgmt.go`). AGT6–9
   `provider=""` ile kaydedilmişti (örtük fallback'e bağımlı, teşhisi zor); artık
   boşsa `claude-cli`'a default'lanır.

> Sıradaki kesin adım: SwarmGo'yu yeniden derleyip flow'u tekrar çalıştır →
> geliştirilen crash-tail tam başarısız bileşeni (hangi MCP/permission) yazacak.

## Self-correcting tool hataları (yayma) + claude-cli çöküş teşhisi ✅ (2026-06-23)

Bir önceki flow-graph fix'inin desenini tüm tool yüzeyine yaydık + SES2 turn 19
CLI çöküşünün teşhis kara-deliğini kapattık.

**1) Ortak hata-ipucu yardımcıları (`builtin_errhints.go`).**
- `argErr(err)` — 49 tool çağrı-yerindeki tek-tip `fmt.Errorf("invalid arguments:
  %w", err)` bununla değişti: Go'nun alan/tip hatasını korur + *"fix: match this
  tool's input schema (required fields, exact types); see its examples"* ekler.
- `enumErr(field, got, allowed...)` — geçersiz değeri ve **izinli seti** listeler;
  task `boardState` kontrollerinde (create/update/move) kullanıldı.
- `cronHint` — schedule reload hatasına 5-alan cron formatı + örnekler
  (`"0 * * * *"`=saatlik, `"*/15 * * * *"`, `"@daily"`); `create/update_schedule`'da.

**2) claude-cli çöküşü artık teşhis taşıyor (`claudecli.go`).** Önceden CLI exit 1
+ boş stderr ile öldüğünde agent'a yalnız `claude CLI failed: exit status 1`
gidiyordu (SES2 turn 19). Artık stdout'un son satırları bounded ring'de tutuluyor;
stderr boşsa `stdoutCrashTail` JSON-olmayan (gerçek hata/panic) satırları tercih
edip mesaja ekliyor → *"claude CLI failed: exit status 1 stdout-tail: panic: …"*.

Test: `builtin_errhints_test.go` (graph hint SES2'nin gerçek hatasını eşliyor +
argErr/enumErr) ve `claudecli_crashtail_test.go`. Toplam 120 test geçer.

## Flow graph hata mesajlarına "ne yapmalı" ipucu + paralel node belgelenmesi ✅ (2026-06-23)

**Sorun (WS2/SES2):** Bir agent paralel flow kurarken `parallel` node'unu yanlış
şemayla (`branches:["id"]` + `next`) kurdu → ham Go hatası
`cannot unmarshal string into ... Node.nodes.branches of type orchestration.Branch`.
Hata "ne yapmalı" demediği ve `swarmgo-flows` skill'i node JSON şemasını hiç
belgelemediği (sadece soyut "steps/edges" anlatıyordu) + `create_flow` örneklerinde
paralel örnek olmadığı için agent doğru şemayı bulamadı, sıralı flow'a düştü.

**Çözüm — kendini düzelten tool hataları:** `validGraphJSON` artık her graph
hatasına kısa, eyleme dönük bir ipucu ekliyor (`graphSchemaHint` + `nodeSchemaCheat`):
hatadaki imzaya göre ("Node.nodes.branches", "has no children", "must be an agent
node" vb.) doğru alanı 5-6 kelimeyle söyler — ör. *"parallel fan-out uses
parallel:[...],joinNext — not branches/next"*. Ayrıca `create_flow`'a paralel
fan-out+join örneği ve `swarmgo-flows` skill'ine node-tipi/alan tablosu + paralel
örnek eklendi. (`builtin_flowmgmt.go`, `skills/defaults/swarmgo-flows/SKILL.md`.)
Doğru paralel şema: `{type:"parallel","parallel":["a","b"],"joinNext":"merge"}`
(çocuklar agent node id'leri; `branches`/`next` DEĞİL).

> Not: SES2'de turn 19'daki `provider error: claude CLI failed: exit status 1`
> ayrı bir provider/CLI çöküşüdür (tool hatası değil, detay loglanmadı) — bu fix
> kapsamı dışında.

## 3 self-management iyileştirmesi: create_agent skills + run_schedule + autonomous artifacts ✅ (2026-06-23)

Üç kullanıcı isteği tek turda:

**1) `create_agent` artık skill atayabiliyor.** Yeni opsiyonel `skills` (slug dizisi)
parametresi; verilmezse yeni agent **default SwarmGo skill seti** ile tohumlanır
(`skills.DefaultSkillSlugs()` — embed'deki `defaults/` alt-dizinlerinden türetilir).
Sağlanan slug'lar skill store'a karşı doğrulanır (`r.skillExists`); bilinmeyenler atlanır
ve sonuçta `skippedUnknownSkills` olarak raporlanır. (`builtin_agentmgmt.go`,
`skills/defaults.go`, `toolsetup.go`.)

**2) `run_schedule` aracı eklendi — "şu schedule'ı şimdi fırlat".** Agent bir routine'i
cron zamanını/enabled durumunu beklemeden **manuel tetikler** (UI "Run now" eşleniği,
`Scheduler.RunNow`). Yıkıcı olmadığı için provenance aranmaz (herhangi bir schedule).
Wiring: `Runtime.runSched` + `SetScheduleRunner` (manager `sched.RunNow` bağlar) →
`tools.NewRunScheduleTool` (self-management). (`builtin_schedulemgmt.go`, `runtime.go`,
`toolsetup.go`, `workspace/manager.go`.)

**3) Artifact'lar her turda oluşturulabilir.** Önceki hata: autonomous turlarda
(scheduler/spawn/flow) artifact sink kurulmadığı için `create_artifact` →
*"artifacts are not available for this turn"*. Çözüm iki yolu da kapsar:
- **Native:** `completeTraced` chokepoint'inde, ctx'te sink yoksa ve sessionID varsa
  fallback sink kurulur (`tools.HasArtifactSink` + `Runtime.NewArtifactSink`).
- **claude-cli:** `api/autonomous_interaction.go` artık run'a `setArtifacts` çağırıyor
  (Interaction MCP köprüsünün gördüğü sink).
- Yeni emit'li sink `internal/agent/artifactsink.go`'da (db'ye yazar + `artifact`
  event'i yayar; chat yolundaki api sink'inin aynası). Artifact'lar artık otonom
  ajanların ürettiğinde de UI'da bildirilir.

Test: `tools` (create_agent skills + run_schedule), `agent` (NewArtifactSink persist),
mevcutlar uyarlandı. **`tools`+`agent`+`skills`+`api`+`workspace` 221 test yeşil**,
build+vet+gofmt temiz. Skill `swarmgo-self-management` güncellendi (+ on-disk senkron).

---

## Self-management araçları prompt'tan gizlendi → skill katalog oldu (hidden-lazy tier) ✅ (2026-06-23)

**Sorun:** Self-management araçları zaten lazy'di (şema yok), ama ~40+ aracın **isim+özet
satırı** her turun "Available Tools (load on demand)" bloğunda (cached prefix) yer alıyordu —
gereksiz token. **Çözüm:** lazy araçlara **hidden** alt-katmanı eklendi; self-management suite
artık blokta **listelenmez**, yerine `swarmgo-self-management` skill'ine yönlendiren tek satır
durur. Araçlar aktive-edilebilir ve aranabilir kalır.

- **`internal/tools/registry.go`:** yeni `hidden map[string]bool` (hidden ⊆ lazy) +
  `MarkHidden(names…)`; `VisibleLazyCatalog(allow)` (= LazyCatalog − hidden, blok için);
  `HiddenLazyCount(allow)`. `LazyCatalog` (activate_tools/tool_search kaynağı) **tüm** lazy'yi
  döndürmeye devam eder → hidden araçlar aktive/aranabilir.
- **`internal/agent/toolsetup.go`:** self-management suite (`builtins[selfManageStart:]`)
  `MarkLazy` yerine **`MarkHidden`**. `LazyToolsCatalogBlock` artık `VisibleLazyCatalog` +
  `HiddenLazyCount` kullanır; `renderLazyToolCatalog(visible, hiddenCount)` hiddenCount>0 ise
  "**N self-management tools … not listed here … load the `swarmgo-self-management` skill … or
  `tool_search`**" pointer satırını basar.
- **Keşif yolu:** Available Skills bloğu `swarmgo-self-management` skill'ini zaten ilan ediyor
  (giriş noktası). Skill **kataloğun kendisi** oldu; metni güncellendi ("bu skill araçların
  listesidir; isimleri buradan/`tool_search`'ten al, `activate_tools` et"). On-disk seed kopya
  da güncel kaynakla senkronlandı (EnsureDefaults üzerine yazmadığı için).
- **Kapsam dışı (şimdilik):** secret_*/list_sessions/WebFetch/*_config hâlâ görünür-lazy
  (self-management değil, az sayıda, çapraz-kesen). claude-cli Interaction MCP köprüsü
  (`BridgeableDefs`) tam şema göndermeye devam ediyor (ayrı yol) — istenirse ayrıca kısılır.
- Test: `tools` (MarkHidden/VisibleLazyCatalog/HiddenLazyCount + aktive-edilebilirlik) +
  `agent` (render pointer + boş durum). **`tools`+`agent` 150 test yeşil**, build temiz.

---

## Generic bildirim sinyalleri: busy / unread / dirty (nav + workspace) ✅ (2026-06-23)

"Haber verme" parçaları (flow/schedule/sohbet/board/artifact değişimleri +
kaydedilmemiş ayar) tek bir generic sisteme toplandı. Her nav görünümü için 3 dik
sinyal: **busy** (accent nabız), **unread** (accent dolu), **dirty** (amber). Hepsi
workspace etiketine yukarı toplanır.

- **Backend**: `api/notify.go` `publishEntityChange` + yeni `board` (task taşıma)
  ve `artifact` (agent sink + UI) event'leri — chat/flow ile aynı SSE borusu.
- **Frontend**: `lib/eventViews.ts` (event→view), `hooks/useUnreadViews.ts`
  (workspace-başına, cross-window persist), `lib/dirtySignals.ts`
  (`useSyncExternalStore` modül store + `useRegisterDirty`). `NavRail` `NavDots`
  ile 3 durumu çizer; `WorkspaceSwitcher`/collapsed ikon aktif workspace'i toplar.
- **Kayıtlı dirty ekranlar**: Settings, WorkspaceView, FlowsPanel.
- **Pencere dışı**: tab başlığı `(N) SwarmGo` (odak dışıyken) + taskbar/dock
  rozeti (`navigator.setAppBadge`, Edge/WebView2'de native taskbar). Toplam
  görülmemiş sayısı `App.tsx` `unreadTotal`. `lib/appBadge.ts`,
  `hooks/useUnreadBadge.ts`.

Detay: `_Docs/29-BILDIRIM-SINYALLERI.md`. Build + tsc yeşil.

## Fix: "Aktivite" nav göstergesi arka plan oturumlarında yanmıyordu ✅ (2026-06-23)

**Sorun:** Bir flow/schedule/agent **ayrı bir oturum** başlattığında (spawn, inbox
teslimi, schedule wake, flow node'ları) sol navbar'daki **Aktivite** öğesinde "işlem
sürüyor" göstergesi çıkmıyordu. Çünkü `handleActivity` yalnızca chat-stream registry'sini
(`s.runs.activeSessionIDs()`) + çalışan task/flow run'larını sayıyordu; otonom invoke'ları
izleyen `Runtime.ActiveSessionIDs()`'i (executions feed'in kullandığı sinyal) **hiç
kullanmıyordu**. Ayrıca `executions` görünümü için bir bayrak yoktu.

**Çözüm:** `activityState`'e `executions` bayrağı eklendi. `handleActivity` artık
chat-stream + `Runtime.ActiveSessionIDs()` oturumlarını birleştiriyor; **herhangi** biri
varsa `executions` yanıyor, ek olarak oturum kind'ı (`chat`/`flow`/`schedule`) ilgili
görünümü de yakıyor. Frontend: `useActivity` → `executions` → `'executions'` View;
`getActivity` tipi güncellendi. `api/activity.go`, `useActivity.ts`, `api/system.ts`.

## UI: ID görünürlüğü + disk yolu erişimi (sohbet / akış / zamanlama / log) ✅ (2026-06-23)

Ajan ekranındaki "ID + klasörü aç/kopyala" deseni diğer ekranlara da yayıldı:

1. **Sohbet listesi (SessionsSidebar)** — her oturum başlığının yanında küçük
   mono **oturum ID'si** (ajanlardaki gibi).
2. **Akışlar (FlowsPanel)** — editör araç çubuğunda **akış ID'si** + **yolu kopyala**
   (`CopyPathButton`) + **klasörü aç** (Explorer `/select`). Yeni backend:
   `GET /api/flows/{id}/path`, `POST /api/flows/{id}/reveal`, `db.FlowPath`.
3. **Zamanlamalar (Schedules)** — her satırda cron ifadesinin yanında mono
   **zamanlama ID'si**.
4. **Loglar (LogsPanel)** — kontrol çubuğunda **log dosyası yolunu kopyala** +
   **klasörü aç**. Loglar artık disk dosyasına da yazılıyor: `SetupLogging`
   stdout + `io.MultiWriter` ile `<dataDir>/logs/swarmgo.log` (append, best-effort).
   Yeni: `config.DefaultDataDir()`, `config.LogFilePath()`, `GET /api/logs/path`,
   `POST /api/logs/reveal` (`api/logs_path.go`).

**Not (workspace rengi):** "kullanılmıyorsa kaldır" istendi ama renk **kullanılıyor** —
NavRail (daraltılmış workspace ikonu) ve WorkspaceSwitcher ikon arkaplan tonu. O yüzden
ayar korundu. API: `api/flows.ts`, `api/system.ts`. Build + tsc yeşil.

## Sıradaki-tur bağlam önizleme (debug) + peer mesajlaşma Faz 2–3 ✅ (2026-06-23)

1. **Sıradaki-tur bağlam önizleme** — Agent ekranındaki bağlam önizlemesinin oturum
   karşılığı: SessionDetailPanel'de **"Bağlam önizle (debug)"** → `SessionContextModal`,
   `GET /api/sessions/{id}/context-preview?message=`. Ajanın bu oturumda sonraki turda
   alacağı tam isteği (sistem + dinamik + **mesaj dizisi** + araçlar, ~token'larla) gösterir.
   **Yan etkisiz** (compaction/persist/provider çağrısı yok; `Prepared` elle kurulur).
   `api/session_context.go`, `SessionContextModal.tsx`, `types/session.ts`, `api/sessions.ts`.
2. **Peer mesajlaşma Faz 2** — yanıt ergonomisi: alıcı `from` adını `to` yapıp yanıtlar
   (araç açıklamasında talimat). Faz 1'de inbox turu zaten geçmiş-duyarlıydı.
3. **Peer mesajlaşma Faz 3 (kısmi)** — **broadcast `"*"`** (`broadcastAgentMessage`,
   best-effort + slot guard, test). Kalan (UI inbox göstergesi + grafik `messaged` kenarı)
   ve **Faz 4** (yapısal protokol) ertelendi.

Testler: `sendmessage_test.go` (+broadcast), `chat_tool_summary_test.go`. Build +
131 test yeşil (cmd/swarmgo-desktop WIP hariç). Detay: `_Docs\07-CHAT-UX.md`,
`_Docs\28-PEER-MESAJLASMA-PLANI.md`.

## Medya/binary artifact desteği (create_artifact sourcePath + auto-capture) ✅ (2026-06-23)

**Sorun (SES30'da görüldü):** Ajan Chrome MCP ile ekran görüntüsü aldı ama PNG'yi
artifact yapamadı; `create_artifact` yalnız inline metin `content` kabul ediyordu,
ajan da binary'yi base64 olarak context'ten geçirmeye zorlanıp token sınırına çarptı
ve "yapısal engel, çözülemez" sonucuna vardı. Oysa depolama (`models_artifact.go`
`image/video/audio/file` kind'ları, `SourcePath`) ve frontend (`ArtifactView`
medya render) zaten hazırdı — eksik olan **ajana açık araç** + **auto-capture'da
medya farkındalığı**ydı.

**Çözüm — iki parça:**

1. **`create_artifact` genişletildi** — `kind` enum'una `image/video/audio/file`
   eklendi + yeni `sourcePath` parametresi. Medya kind'larında bytes context'e hiç
   girmez: dosya yolu verilir, `ArtifactSink.CreateArtifact` artık `CreateArtifactSpec`
   struct'ı alır, sink `db.ImportMediaSource` ile yolu workspace-göreli hale getirir
   (workspace dışındaki dosyayı — ör. Downloads'taki screenshot — `artifacts/<session>/`
   altına **kopyalar**, içindekini olduğu yerden referanslar). Doğrulama: text kind →
   `content` zorunlu, media kind → `sourcePath` zorunlu.
2. **Auto-capture genişletildi** (`artifacts_auto.go`) — (a) `artifactKindForPath`
   medya uzantılarını (`.png/.jpg/.gif/.webp/.mp4/.mp3/.pdf/.zip/...`) doğru media
   kind'a eşler (artık `text`'e düşmez); (b) `Write`/`create_file` medya dosyası
   yazarsa binary-as-text yerine `SourcePath` ile yakalanır; (c) **herhangi bir aracın
   çıktısı** taranır (`extractProducedMediaPaths` + regex) — screenshot/export araçları
   kaydettikleri dosya yolunu döndürünce o dosya da medya artifact'ı olarak yakalanır.
   Var olmayan yol stat'ta elenir → sahte artifact üretilmez.

Her zaman enjekte edilen `artifactDeliverableGuidance` promptu, ajana "binary dosyayı
`sourcePath` ile ver, base64 gömme" talimatıyla güncellendi.

Dosyalar: `tools/artifact.go` (`CreateArtifactSpec`), `tools/builtin_artifact.go`,
`db/artifact_content.go` (`ImportMediaSource`+`copyFileContents`), `api/artifacts.go`
(sink + guidance), `api/artifacts_auto.go`. Testler: `db/artifact_media_test.go`,
`api/artifacts_auto_media_test.go` (+ güncellenen `mcp_interaction_test.go` fakeSink).
Build + tüm api/db/tools testleri yeşil.

## Peer mesajlaşma Faz 1 (send_message/mailbox) + araç I/O geçmiş özeti ✅ (2026-06-23)

İki iş birlikte yapıldı:

1. **`send_message` (Faz 1)** — Claude Code mailbox deseninin uyarlaması: bir ajan
   başka ajana **adresli, kimlikli** mesaj atar; mesaj alıcının kalıcı **inbox**
   oturumuna (`GetOrCreateKindSession` kind="inbox") `<agent_message from="…">`
   etiketiyle düşer ve alıcının **geçmiş-duyarlı turu** arka planda çalışır
   (fire-and-forget, `SpawnMaxConcurrent` guard, kendine-mesaj reddi). self-manage
   gated. `run_subagent` (izole görev) ile birlikte durur; bu "süregelen peer
   işbirliği" yolu. Dosyalar: `agentmsg.go`, `builtin_sendmessage.go`, `toolsetup.go`;
   `runSessionTurn` ile wake/inbox ortak geçmiş-duyarlı runner. Plan/detay:
   `_Docs\28-PEER-MESAJLASMA-PLANI.md`.
2. **Araç I/O geçmiş özeti (#5)** — geçmişte araç çağrı/sonuçları düşüyordu; artık
   son N=4 asistan turunun `Steps` izinden kompakt `<recent_tool_activity>` bloğu
   (kopya üzerinde) eklenir → ajan "az önce ne yaptın / ne döndü"yü yanıtlar
   (`api/chat_tool_summary.go`). Wake/inbox turları da dahil.

Testler: `sendmessage_test.go`, `chat_tool_summary_test.go` (+ mevcutlar). Build +
241 test yeşil (cmd/swarmgo-desktop'taki ilgisiz WIP hariç). Detay:
`_Docs\07-CHAT-UX.md`, `_Docs\28-PEER-MESAJLASMA-PLANI.md`.

## MCP kalıcı bağlantı havuzu (persistent pool) ✅ (2026-06-23)

Daha önce belgelenen 🔴 kısıt (dinamik MCP araç ekleme / `tools.listChanged` yok +
dial-per-operation) **kalıcı olarak çözüldü**. Önceki ara çözüm (60sn katalog TTL cache,
`mcpcatalog.go`) **kaldırıldı**, yerini sunucu başına **canlı oturum havuzu** aldı.

**`internal/mcp/client.go` (yeniden yazıldı):** stdio istemci artık **kalıcı + eşzamanlı
kullanıma güvenli**. Tek arka-plan **read loop** yanıtları JSON-RPC `id`'ye göre per-call
kanallara demux eder; sunucu bildirimleri (`notifications/tools/list_changed`) bir
callback'e yönlenir. `initialize` artık `capabilities.tools.listChanged=true` ilan eder.
Yeni API: `SetOnToolsChanged`, `Alive`. (`proc.Command` süreç-grubu kill korundu.)

**`internal/mcp/pool.go` (yeni):** `Pool` her enabled MCP server için **tek canlı
`StdioClient`** tutar (sanitized ada göre).
- **Catalog reuse:** tur-başı tekrarlı `buildRegistry` çağrıları aynı canlı oturumu
  kullanır → gateway'de **session churn yok** (eski burst sorunu da tamamen biter).
- **Oturum-durumu korunur:** gateway'in `activate_tools` etkisi oturum kapanmadığı için
  **sonraki çağrıda da yaşar** → dinamik araç ekleme artık çalışır (tur-ötesi).
- **listChanged → invalidate:** sunucu araç listesi değişince entry stale işaretlenir,
  sonraki `Catalog` aynı canlı oturumda yeniden listeler. Ek emniyet: TTL
  (`SWARMGO_MCP_POOL_TTL_SEC`, vars. 60sn) — listChanged göndermeyen sunucular için.
- Config (command/args/url/env fingerprint) değişiminde veya bağlantı ölümünde şeffaf
  re-dial; çağrı ölü bağlantıda bir kez retry eder.

**Wiring:** `Runtime.mcpPool *mcp.Pool` (+ `CloseMCP()`, workspace teardown'da çağrılır:
`workspace/manager.go` delete + Close). `buildRegistry` → `pool.Catalog`. `Registry`
artık `mcpCaller` taşır (`AttachMCP(..., caller)`); MCP çağrıları pool üzerinden gider,
nil ise dial-per-call'a düşer (testler). Tek-seferlik test endpoint'i
(`api/mcp.go handleTestMCPServer`) + `live_test.go` hâlâ dial-per-op `ListServerTools`/
`CallNamespaced` kullanır (kasıtlı).

**Kalan nüans:** Aynı tur içinde `activate_tools` sonrası yeni araçlar **bir sonraki turda**
çağrılabilir (tur başı `reg` sabit). Pre-load için preset hâlâ en pürüzsüz yol; ama artık
zorunlu değil. Test: `internal/mcp/pool_test.go` (sahte stdio MCP sunucusu: persistent
reuse, listChanged refresh, config-change re-dial, Close, hata yolları). **`internal/mcp`
+ `internal/agent` + `internal/tools` + `internal/workspace` 153 test yeşil**; build+vet
temiz (ilgisiz `internal/e2e` MemGPT WIP build hatası hariç).

---

## Bilinen kısıt — dinamik MCP araç ekleme (`tools.listChanged`) desteklenmiyor 🔴→✅ (2026-06-23)

Çok-ajan "kim ne dedi" çözümünü iki referansla karşılaştırdık:
- **external-agent-oss:** sorunu *yaşamıyor* — bir oturum = tek ajan; çok-ajan ayrı oturum.
  Mesajlarda per-mesaj yazar alanı yok; Claude Agent SDK döngüyü sürüyor.
- **Claude Code (`observed-behavior` swarm/teammate):** çok-ajanı **izole bağlam + adresli
  mailbox** ile çözüyor — `SendMessage({to,message,summary})`, alıcının inbox'ına `from`
  kimliğiyle `<teammate_message teammate_id>` etiketiyle düşer; plain çıktı diğer ajana
  görünmez. Kimlik **doğuştan**; ardışık-rol çakışması hiç oluşmaz.

SwarmGo iki modeli birden taşıyor: paylaşılan-thread (etiketleme+coalesce ile sağlamlaştırıldı)
ve izole `run_subagent`. Eksik olan "akran ajana adresli DM" için **uyarlama planı** yazıldı:
`_Docs\28-PEER-MESAJLASMA-PLANI.md` (mevcut `GetOrCreateKindSession` inbox + `SpawnSession`
üzerine). Kavramsal not: `_Docs\10-KAVRAMSAL-TASARIM-NOTLARI.md` §10. **Uygulama kullanıcı
onayı bekliyor** (tetik/inbox modeli/ayrı-araç kararları planda).

## Çok-ajanlı bağlam sağlamlığı: yazar kimliği + ardışık-rol + geçmiş-duyarlı wake ✅ (2026-06-23)

**Bağlam:** SES29'da iki ajana soru soruldu ama ajanlar "kim ne dedi"yi göremedi.
Kök neden: `toProviderMessages` geçmişi çevirirken `AgentID`'yi düşürüyordu → tüm
asistan turları tek ayrımsız "assistant" sesine karışıyordu. Düzeltme + aynı sınıftan
diğer kusurlar tarandı; en kritik 3'ü (+ kullanıcı-hedefi) kapatıldı:

1. **Yazar etiketleme** (önceki tur) — çok-yazarlı geçmişte her asistan turu yazarıyla
   ön-eklenir (`api/chat_authors.go`); kendi turlarına `(you)` markerı. **Güncelleme
   (2026-06-23):** etiketleme eşiği "2+ farklı yazar"dan "geçmişte **yanıtlayan ajandan
   farklı** bir yazar var mı"ya genişletildi → ajanı değiştirilmiş (devredilen) oturumda
   da (tek önceki yazar A, şimdi B yanıtlıyor) A'nın turları etiketlenir, B onları
   kendisininki sanmaz. Saf tek-ajan oturumu (yalnız yanıtlayan konuşmuş) hâlâ etiketsiz
   (doğal transkript + prompt cache korunur).
2. **Geçmiş-duyarlı wake** — `schedule_wake` ile uyanan ajan eskiden yalnız wake
   prompt'unu görüyordu (`invokeTraced`, geçmiş yok). Artık `WakeTurnFunc` hook'u
   (`api/wake_turn.go`) tam sohbet turunu (geçmiş+özet+hafıza+goal) kurar. Detay:
   `_Docs\20-SCHEDULE-WAKE.md`.
3. **Ardışık aynı-rol birleştirme** — bir kullanıcı mesajına 2 ajan ardışık yanıtlarsa
   `user→assistant→assistant` oluşuyor, Anthropic "roles must alternate" ile reddediyordu.
   `providers/coalesce.go` ardışık aynı-rol düz-metin turları birleştirir (araç turlarına
   dokunmaz); anthropic + minimax çeviricilerinde uygulanır.
4. **Kullanıcı mesajının hedef ajanı** — kullanıcı mesajı artık yönlendirildiği ajanla
   (`AgentID = agents[0]`) damgalanır; geçmişte `"[User → Ada]: …"` etiketlenir → "hangi
   soru kime" de görünür. `@Ad` yalnız bilgi amaçlı (yönlendirme değil).

Testler: `chat_authors_test.go`, `providers/coalesce_test.go` (+ mevcutlar). Tüm
build + 167 test yeşil. Detay: `_Docs\07-CHAT-UX.md`, `_Docs\20-SCHEDULE-WAKE.md`.

## Bilinen kısıt — dinamik MCP araç ekleme (`tools.listChanged`) desteklenmiyor 🔴 (2026-06-23)

**Bulgu (gerçek vaka):** Bir ajan MCP Gateway üzerinden `mcp-chrome`'u kullanmak istedi.
Gateway'in `activate_tools('mcp-chrome')` çağrısı **"✅ 29 tools activated"** döndü ama
ardından `chrome_navigate` çağrısı **`No such tool available`** verdi. Gateway'in kendisi
uyardı: *"Your client did not advertise tools.listChanged support… reconnect with a preset."*

**Kök neden — iki birleşen mimari gerçek:**
1. **`tools.listChanged` yok:** istemci `initialize`'da `capabilities:{}` gönderir
   (`internal/mcp/client.go`), yani sunucu "araç listem değişti" bildirimini gönderse bile
   SwarmGo `tools/list`'i yeniden çağırmaz.
2. **Dial-per-operation (havuzsuz):** `BuildCatalog`/`CallNamespaced` her işlemde **yeni
   session** açıp kapatır. Gateway'in `activate_tools`'u **oturum-kapsamlıdır** → araçları
   o anlık session'a ekler, session `Close()` ile kapanınca kaybolur. Eklenen araçlar
   SwarmGo'nun kataloğuna hiç girmez → çağrılamaz.

→ Sonuç: **runtime'da araç ekleyen/çıkaran MCP sunucularıyla SwarmGo uyumsuz.**

**Geçici çözüm (uygulandı):** İstenen araçlar sunucunun bağlantı URL'indeki **preset'e**
konur; preset her taze session'da başlangıçta yüklendiği için dial-per-operation modeliyle
sorunsuz çalışır. MCP Gateway `swarmgo` preset'ine `mcp-chrome` eklendi
(`mcp-server/config.json`: `swarmgo: [<remote-service>, mcp-chrome]`); `?preset=swarmgo` artık
47→**76 araç** döndürüyor. Doğrulandı.

**Kalıcı çözüm (Sırada / öneri):** ya (a) `initialize`'da `tools.listChanged` ilan edip
**kalıcı session** tut + bildirimde `tools/list`'i yenile, ya da (b) gateway gibi
dinamik sunucular için kalıcı bağlantı havuzu (Seçenek 2) — böylece `activate_tools`
etkisi sonraki çağrıda da yaşar. İkisi de aynı `internal/mcp` yeniden tasarımına bağlanır.

---

## Reflection budama + auto-reflect eşiği 30→20 ✅ (2026-06-23)

Dream cycle ayarları ince ayarlandı: ham journal gürültüsü daha erken damıtılsın diye
**`autoReflectThreshold` default 30 → 20**; ve reflection'lar (journal'ların aksine
budanmıyordu → süresiz birikip recall havuzunu kirletiyordu) artık **bounded**.

- **Yeni `reflectionCap`** (default 20, clamp 1–1000): her dream cycle sonunda
  `reflect()` en yeni N reflection'ı tutup eskileri budar (`PruneKind(reflection)`).
- Wiring: `Tunables.SetReflectionCap/ReflectionCap` + `DefaultReflectionCap=20`;
  `settings` (Settings/DTO/Patch/Default + clamp) + `applySettings`; frontend
  "Yansıma limiti" alanı. `reflector.go`'da budama + `reflectionCap()` helper.
- `go build` ✅, **148 test** ✅ (tunable default + reflection-prune), `tsc` ✅.

## HA-1: human bloğu otomatik kullanıcı modelleme (MemGPT Parça 4b) ✅ (2026-06-23)

`human` çekirdek bloğu artık dream-cycle ile **otomatik** doldurulur. `reflect()`
her çalıştığında (manuel/auto), yansımadan sonra journal'lar silinmeden önce
`updateUserModel` çağrılır: model mevcut profili + journal'ı alıp kullanıcı
hakkındaki kalıcı çıkarımları kısa satırlar olarak merge eder, `human`'a yazar.

- **Yeni dosya** `internal/agent/user_model.go` (`updateUserModel`/`writeUserModel`
  + prompt); `reflector.go`'ya tek `if r.tun.UserModel()` satırı. `runtime.go`
  **dokunulmadı**. Usage `KindReflect`'e yazılır (dream-cycle maliyeti).
- **Best-effort**: değişiklik yoksa no-op, limit aşılırsa truncate, hata yansımayı
  bozmaz. **Ayar** `AutoUserModel` (varsayılan açık) — settings + Tunables +
  applySettings + frontend toggle.
- `go build` ✅, **206 test** ✅, `tsc` ✅. Detay: `31-MEMGPT-CORE-MEMORY.md` Parça 4b.

## Çekirdek bellek: adlandırılmış bloklar + karakter limiti (MemGPT Parça 5) ✅ (2026-06-23)

Letta'nın **memory blocks** modeli native getirildi. Sabit persona/human ikilisi,
ajanın istediği etikette tanımlayabildiği **dinamik bloklar**a genelleşti; her blok
**karakter limiti + açıklama + salt-okunur** taşır.

- **Encoding** `kind="core:"+label` (`db.CoreKind/IsCoreKind`); tanım ajan dosyasında
  (`Agent.CoreBlocks`), içerik `knowledge_sources`'ta. `CoreBlocks` boşsa varsayılan
  persona+human (2000 char) → migration yok.
- **Store**: `WriteCore/ReadCore/AppendCore(label)` + limit (`*CoreBlockFullError`),
  `ErrUnknownCoreBlock`, `ReadCoreBlocks→[]BlockView`, `DefineCoreBlock`/`DeleteCoreBlock`.
- **Araç**: `section`→`label` (alias korundu); bilinmeyen/read-only/limit hatası ajana
  net döner. Constructor imzaları sabit → `runtime.go`/`toolsetup.go` **dokunulmadı**.
- **API**: `GET /core→{blocks}`, `PUT /core {blocks:{label:content}}`, `POST/DELETE
  /core/blocks[/{label}]`. **Frontend**: `CoreMemoryCard` dinamik + limit çubuğu +
  blok ekle/sil + read-only kilit.
- Doğrulama: `go test` 141 ✅, `tsc` ✅. Detay: `31-MEMGPT-CORE-MEMORY.md` Parça 5.

## Composer: oturum-başına taslak + ikon-tabanlı kontroller ✅ (2026-06-23)

İki UX iyileştirmesi (kullanıcı isteği). Yalnız frontend, `tsc --noEmit` temiz.

1. **Oturum-başına taslak.** Yeni `useSessionDraft` hook'u (`hooks/useSessionDraft.ts`):
   composer'a yazılıp **gönderilmeyen** metin `localStorage`'da oturum-id ile saklanır
   (`swarmgo:draft:<sessionId>`). Oturum değiştirip dönünce ve sayfa yenilenince korunur;
   gönderme/temizleme taslağı siler (boş taslak saklanmaz). Composer `useState('')` yerine
   bu hook'u kullanır — tüm mevcut `setText` çağrıları otomatik kalıcı. **Yan fayda:** eskiden
   metin oturumlar arası sızıyordu (Composer `key`'siz, monte kalıyor); artık her oturum kendi taslağını taşır.
2. **İkon-tabanlı composer kontrolleri.** Ajan seçici yalnız avatar (ad tooltip'te); düşünme
   seviyesi yoğunluk ikonuyla (◌○◔◑●); izin modu emoji ikonuyla (🛡🔒✋⚡). `ComposerPicker`'a
   `iconOnly` prop'u eklendi; `THINKING_OPTIONS`'a seviye ikonları eklendi.

---

## E2E test paketi — uçtan uca ajan davranışları ✅ (2026-06-23)

Yeni **`internal/e2e`** test paketi: tam kablolu bir `agent.Runtime` (gerçek dosya-store,
memory, skills, sandbox, `conversation.Manager`) `api/chat_stream`'in sürdüğü tur hattının
**aynısıyla** sürülür; yalnız LLM, ağsız-deterministik bir **`scriptedProvider`** ile
değiştirilir. Provider sıraya konmuş yanıtları kuyruktan tüketir (metin turu veya `tool_use`
turu → native araç döngüsü gerçek araçları çalıştırır), `conversation.Manager`'ın rolling-summary
özetleme çağrısını ise script'i bozmadan yakalayıp yanıtlar. Harness (`harness_test.go`) her turda
kullanıcı mesajını kalıcılaştırır, geçmişi bütçeleyici üzerinden tekrar oynatır, sistem+memory
bağlamını dizer, akışlı araç döngüsünü koşar, yanıtı kalıcılaştırır + journal'lar.

**Kapsam (16 test, hepsi yeşil):**
- **Konuşma:** çok-turlu geçmiş kalıcılığı + ikinci tura ilk alışverişin tekrar oynatılması (`conversation_e2e_test.go`).
- **Araç kullanımı:** tek turda `Write`→`Read` çok-adımlı döngü (dosya gerçekten diske düşer, sonuç cevaba akar, `StepDiff` izi), shell-gate (`tools_e2e_test.go`).
- **Skill kullanımı:** workspace skill oluştur → katalogta slug+özet (gövde lazy) → `use_skill` ile gövde yükleme; ayrıca per-agent allowlist ile kısıtlı skill erişilemezliği (`skills_e2e_test.go`).
- **Hafıza:** `core_memory_replace` ile kalıcı çekirdek bellek + sonraki tura tekrar enjeksiyon; `memory_recall` ile uzun-dönem hatırlama (`memory_e2e_test.go`).
- **Uzun oturum:** bütçe aşımında compaction tetiklenmesi (özet kalıcı, yalnız son tur'lar verbatim) (`longsession_e2e_test.go`).
- **İzin modu:** read-only modda yazma engellenir/okuma serbest; ask modunda interaktif prompter ile onay/red (yazma diske düşer ya da engellenir, `permission_denied` izi) (`permission_e2e_test.go`).
- **Self-wake:** `schedule_wake` ctx'teki scheduler'a doğru delay/prompt ile ulaşır; scheduler yokken (headless) net hata (`wake_e2e_test.go`).
- **Delegasyon:** delegasyon kapalıyken `run_subagent` kayıtlı değil (unknown tool); açıkken bilinmeyen hedef target-çözümleme guard'ıyla reddedilir (`delegation_e2e_test.go`). Not: alt-ajan provider'ı registry'den çözüldüğü için (anahtarsız) mutlu-yol e2e'si üretim kodu değişmeden test edilemez; gate+guard yolları kapsandı.
- **Çok-ajanlı tur:** tek kullanıcı mesajı, iki ajan sırayla yanıtlar; ikinci ajan birincinin cevabını geçmişte görür (`multiagent_e2e_test.go`).

Harness genişletildi: `decorate` ctx-kancası (prompter/grants/wake enjeksiyonu) + `sendMulti` (çok-ajanlı tur sürücüsü).
İzolasyon: `SWARMGO_DATA_DIR` temp'e yönlendirilir → gerçek `~/.swarmgo` skill/market seed'ine dokunulmaz.
✅ `go test ./internal/e2e/` 16/16 yeşil, `go vet` temiz.

## Native pencere — konsol penceresi yanıp sönmesi düzeltildi ✅ (2026-06-23)

Konsolsuz desktop binary (`-H windowsgui`) bir konsol alt-süreci başlattığında Windows'un
çocuk için açtığı terminal penceresi yanıp sönüyordu (claude CLI/PowerShell/git/MCP). Yeni
**`internal/proc`** paketi (`Hide(cmd)` → Windows'ta `CREATE_NO_WINDOW`, diğer platformlarda
no-op) konsol açan tüm `exec.Command` çağrılarına eklendi: `claudecli.go` (ana suçlu),
`builtin_shell.go`, `agent/hooks.go`, `agent/worktree.go`, `mcp/client.go`, `api/git.go`,
`workdir_context.go`, `workspaces.go`. `explorer.exe` (GUI) dokunulmadı. ✅ build/vet +
305 test yeşil. Detay: [32-NATIVE-PENCERE.md](32-NATIVE-PENCERE.md).

## Native pencere — başlık çubuğu temaya uyumlu ✅ (2026-06-23)

WebView2 penceresinin native başlık çubuğu (caption + küçült/büyüt/kapat butonları + kenarlık)
artık uygulama temasına boyanıyor — beyaz Windows frame'i koyu temayla çelişmiyor. **DWM** ile
(`dwmapi.dll` `DwmSetWindowAttribute`, salt `syscall`, yeni bağımlılık yok): `DWMWA_USE_IMMERSIVE_DARK_MODE`
(Win10 1809+) + `DWMWA_CAPTION_COLOR`/`TEXT_COLOR`/`BORDER_COLOR` (Win11 22000+). `app.App.Appearance()`
çözülen `ThemePreset`/`Theme`/`Accent`'i verir; `cmd/swarmgo-desktop/titlebar_windows.go` 8 curated paletin
bg/text/border'ını (`themePresets.ts` ile elle senkron) COLORREF'e (`0x00BBGGRR`) çevirir. Bilinmeyen
preset → yalnız dark/light frame (caption rengi atlanır); eski Windows'ta desteklenmeyen attribute'lar
sessizce yok sayılır (pencere yine çalışır). **Canlı güncelleme:** `watchTitleBar` 1.5sn poll ile tema
değişince `w.Dispatch` üzerinden yeniden uygular (Ayarlar'dan palet değiştirince başlık anında uyar).
✅ `go build ./...`/`vet` yeşil; desktop canlı (varsayılan midnight-violet, pencere açıldı, health 200,
panik/hata yok). Detay: [32-NATIVE-PENCERE.md](32-NATIVE-PENCERE.md).

## Native masaüstü penceresi — WebView2 (CGO'suz) ✅ (2026-06-22)

SwarmGo artık tarayıcı yerine **kendi masaüstü penceresinde** açılabiliyor. Plan:
[32-NATIVE-PENCERE.md](32-NATIVE-PENCERE.md). **Ön koşul refactor (davranış-korumalı):**
`cmd/swarmgo/main.go`'nun boot dizisi yeni **`internal/app`** paketine taşındı
(`SetupLogging` + `Bootstrap`/`Serve`/`Shutdown`/`Addr`/`URL`); `Bootstrap` artık listener'ı
önden açar (`net.Listen`, `:0` boş port desteği) ve `SetBaseURL`'i çözülen adresle çağırır.
`main.go` ~130→~50 satır. **Yeni giriş noktası** `cmd/swarmgo-desktop` (`//go:build windows`,
[`jchv/go-webview2`](https://github.com/jchv/go-webview2) — **saf Go, CGO yok**; Win11'de
yerleşik WebView2 runtime): sunucuyu `127.0.0.1:0`'da başlatır, `waitForHealth` ile hazır olunca
1280×800 WebView2 penceresi açar, pencere kapanınca graceful `Shutdown`. WebView2 yoksa →
varsayılan tarayıcıya fallback (`rundll32 url.dll`). `!windows` stub mevcut. `scripts/build.ps1`
`-Desktop` bayrağı (`-H windowsgui` → konsolsuz). Bağımlılık: `go-webview2` (direct) +
`go-winloader`/`x/sys` (indirect) — yalnız desktop hedefinde derlenir; **başsız `swarmgo`
hâlâ saf-Go/çapraz-derlenebilir**. ✅ `go build ./...`/`vet` yeşil; başsız smoke (refactor sonrası
`/`+`/health` 200, boot logları aynı); desktop canlı (rastgele port 60385'te boot, `/health`+`/`
200, pencere açıldı); `-H windowsgui` build 12 MB. README "Native Masaüstü Uygulaması" eklendi.

## Prompt saatine saniye eklendi (SES28 zaman-ölçüm hatası) ✅ (2026-06-22)

**Sorun:** SES28'de bir ajan "30 sn bekle, farkı ölç" görevinde ilk saati **uydurdu**
(`18:18:48`). Kök neden: sistem-prompt saati yalnız **dakika** hassasiyetindeydi
(`15:04`), `get_current_time` aracı da kaldırılmıştı → saniye gereken ölçümde ajanın
gerçek saati yok, uyduruyor. (Not: `schedule_wake` timer'ı doğru — tam 30 sn tetikledi;
hata zamanlayıcıda değil, uydurmadaydı.)

- **Çözüm (geçici):** `dateTimeContextBlock` (chat) + `autonomousSystemPrompt` (headless)
  artık `15:04:05` (saniyeli) yazıyor ve satır "tur başında yakalandı, tur içinde ilerlemez"
  diye etiketli — ajan ölçüm görevinde bunu **baseline** alsın, uydurmasın.
- **Kalıcı (sırada):** hafif, her-zaman-açık `get_current_time` aracı (saniye + unix epoch)
  — bilerek kaldırılmıştı; kullanıcı onayı bekliyor.
- **Doğrulama:** `go build ./...` + `go vet` temiz.

---

## Ajan path/reveal + oturum yolu ~ gösterimi + context-mode dedektörü ✅ (2026-06-22)

Üç küçük UI/UX iyileştirmesi (kullanıcı isteği). Build + `tsc --noEmit` temiz.

1. **Ajan dosya yolu / klasör aç.** Ajan ayarları formuna (`AgentSettingsForm`) sağ
   üstte **Yolu kopyala** + **Klasörü aç** butonları eklendi. Backend: `db.AgentPath`
   (ajanın `agents/<id>.json` mutlak yolu) + `GET /api/agents/{id}/path` +
   `POST /api/agents/{id}/reveal` (`explorer.exe /select,<path>` ile dosyayı vurgular).
   Frontend api: `agentApi.agentPath`/`revealAgent`. Session reveal deseninin ajan eşleniği.
2. **Oturum yolu `~` gösterimi.** `SessionDetailPanel` "Klasör" bölümü `info.path`'i ham
   gösteriyordu; artık `displayPath()` ile `~\...` kısaltmasıyla gösterir (title'da tam yol;
   "Yolu kopyala" hâlâ tam yolu kopyalar).
3. **context-mode dedektörü.** Hooks "Harici token araçları" listesine (`external_tools.go`)
   `context-mode` eklendi (rtk/sqz yanında).

---

## MCP katalog önbelleği — gateway'de session birikmesi düzeltildi ✅ (2026-06-22)

**Sorun:** Yerel MCP Gateway'de saniyeler içinde 4 ayrı `swarmgo` session açılıyordu
(her biri `requestCount:3`). Kök neden: native MCP istemcisi **havuzsuz** (`manager.go`
dial-per-operation) ve `buildRegistry` tek bir sohbet turunda birden çok kez çağrılıyor
(tur girişi `runtime.go`, native döngü `toolloop.go`, UI/araç önizleme endpoint'leri).
Her çağrı `BuildCatalog` ile **her enabled MCP server'ı yeniden dial ediyordu**
(`initialize` + `notifications/initialized` + `tools/list` = requestCount 3), ardından
`Close()`. Gateway HTTP-köprülü olduğu için her dial yeni bir session açıyor; stdio
istemci HTTP `DELETE` göndermediğinden gateway session'ları idle olarak birikiyordu.

**Çözüm — workspace başına katalog önbelleği** (`internal/agent/mcpcatalog.go`):
- `Runtime.mcpCat *mcpCatalog` — dial edilmiş katalogu (entries) bellekte tutar.
- **Fingerprint-tabanlı geçersizleme:** anahtar = enabled server config'lerinin
  SHA-256 fingerprint'i (sıra-bağımsız). Server toggle/ekle/sil/düzenle → fingerprint
  değişir → otomatik rebuild. **Ayrı invalidation hook'u gerekmez.**
- **TTL:** varsayılan **60 sn** (`SWARMGO_MCP_CATALOG_TTL_SEC` ile override; `0` =
  önbellek kapalı, eski davranış). Config'in göremediği dış değişiklikleri (server
  farklı tool sunması) sınırlar.
- Sadece pahalı dial sonucu (entries) önbelleklenir; ucuz dispatch haritası
  (`cfgByServer`) her çağrıda canlı server listesinden yeniden hesaplanır → asla
  drift etmez. Nil-receiver toleranslı (bare `&Runtime{}` testleri önbeklsiz çalışır).
- **Etki:** tur-başı dial burst'ü her server için **1**'e iner → gateway session
  birikmesi ~%90 azalır.
- Test: `mcpcatalog_test.go` (cache hit/miss, fingerprint geçersizleme, TTL süresi,
  ttl=0, nil-receiver, env parse). `internal/agent` + `internal/mcp` **81 test** yeşil.

> **Kalan (tam çözüm değil):** `CallNamespaced` (gerçek tool çağrısı) hâlâ dial-per-call.
> Asıl tool kullanımı seyrek olduğu için burst kaynağı değil; kalıcı bağlantı havuzu
> (Seçenek 2) veya native HTTP transport + `Mcp-Session-Id` reuse ileride değerlendirilebilir.

---

## Bütçe ceil/fraction artırıldı + eskimiş 1M-token ayarı kaldırıldı ✅ (2026-06-22)

**Hedef:** (1) 1M modelleri daha çok kullan, (2) eskimiş 1M-token beta ayarını temizle.

- **Ceil/fraction (1M kullanımını artır):** `budgetWindowFraction 0.10→0.20`,
  `budgetAutoCeil 32K→128K`, tool-eşik scale clamp `[1,5]→[1,12]`. Artık 1M modeller
  **128K** transcript bütçesi (≈%12.8), Haiku **40K**. 1M'de ceil operatif knob.
  `budget_test.go` + `tunables_compact_test.go` güncellendi. **124 test** yeşil.
- **Eskimiş 1M-token ayarı kaldırıldı:** Web doğrulaması — Anthropic 1M'i **13 Mart 2026**
  GA yaptı (header gerekmez), `context-1m-2025-08-07` beta header'ı **30 Nisan 2026**
  kapatıldı. Bugün 22 Haziran → tamamen işlevsiz.
  - **Provider:** `anthropic.go` artık header'ı **göndermiyor** (const + field + append
    silindi; `WithBetas(_, extendedCache)`). 43 test + vet temiz.
  - **Frontend:** Ayarlar → Anthropic beta'daki "1 milyon token bağlam" toggle'ı +
    `oneMillionContext` tipi/payload referansları kaldırıldı (appPanels + types +
    SettingsPanel). `npm run build` yeşil.
  - **Kalan (entangle):** backend `settings.OneMillionContext` alanı + `server.go`
    `SetAnthropicBetas` argümanı + registry/ResolvedConfig plumbing vestigial kaldı
    (zararsız, header gitmiyor). `api` paketi paralel MemGPT WIP'inden kurtulunca
    tek temiz commit'te purge edilecek (server.go o dosyada entangle).

---

## `http_get` → `WebFetch` zengin fetch ✅ (2026-06-22)

`http_get` (düz GET, 64KB) **`WebFetch`'e yükseltildi** — claude-cli'nin WebFetch'inin
native eşleniği, son web-parite boşluğu kapandı. Build+vet temiz, **300 test** yeşil,
`tsc --noEmit` temiz, backend rebuild+restart.

- **HTML→Markdown converter** (`internal/tools/htmltomarkdown.go`, **stdlib-only** —
  go.mod minimal kalır, `x/net/html` yok): regexp geçişleriyle script/style/nav/form
  blokları atılır; başlık/link/liste/bold/italic/code/pre → Markdown; göreli linkler
  base URL'e çözülür; `html.UnescapeString` ile entity çözme; whitespace temizliği.
- **`WebFetchTool`** (`builtin_http.go`): SSRF-guard'lı dialer korunur (loopback/
  private/link-local + 169.254 metadata + CGNAT engeli, redirect/DNS-rebind kapsanır).
  HTML→Markdown; metinsel içerik (md/plain/json/xml) verbatim; ikili içerik özetlenir
  (dump edilmez). `raw=true` ham gövde döndürür. Ham indirme 3MB, çıktı 96KB cap.
  Yönlendirme sonrası final URL başlıkta.
- **İsim:** `http_get`→`WebFetch` (classify zaten `WebFetch=RiskRead` taşıyordu;
  `bridgeExcluded` anahtarı güncellendi — CLI kendi WebFetch'ini kullandığından bizimki
  köprülenmez). Referanslar: `toolsetup`/`subagent`/`registry`/frontend `tools.ts` +
  default skill + testler (`tooltier`/`lazyload`/`builtin_http`). Yeni test:
  `htmltomarkdown_test.go` (4 senaryo). Dokümanlar: `09-SDK`, `11-INTERACTION`, `19-LAZY`.

## Tool eşikleri per-model bütçeye hizalandı + model picker rozeti ✅ (2026-06-22)

**Hedef:** (1) §5 tool eşiklerini de per-model bütçeye bağla (zinciri tutarlı kıl),
(2) frontend model picker'da context-window'u göster.

- **Tool eşikleri per-model:** `Tunables`'a `CompactMaxBytesFor(budget)` /
  `CompactLLMThresholdFor(budget)` + `ContextBudgetTokens()` getter + saf
  `budgetScaleFor(budget)`. Compactor her tur `conversation.EffectiveBudget(
  provider, model, ContextBudgetTokens())` hesaplayıp For-varyantlarına geçiyor.
  No-arg getter'lar process-geneli bütçeyle geriye-uyumlu. Zincir: model penceresi
  → EffectiveBudget → eşik ölçeği. `tunables_compact_test.go` (For). Agent **71 test**.
- **Model picker rozeti:** `CatalogModel.contextWindow?` (TS) + `ProviderModelSelect`
  `formatContextWindow` (200000→"200K", 1000000→"1M"); dropdown option'larında
  "· 200K" + seçili modelde "200K bağlam" rozeti. `tsc` + `npm run build` yeşil.
- **Not:** Hepsi agent/conversation/providers/frontend'de — `api` paketine dokunulmadı.

---

## claude-cli `use_skill` isim uyuşmazlığı düzeltmesi ✅ (2026-06-22)

**Hedef:** SES21'de açık kalan konu — claude-cli ajanı ilk turda `use_skill`'i çağırınca
"No such tool available: use_skill" alıyordu (ikinci turda kendini toparlıyordu).

- **Kök neden (yarış değil, isim uyuşmazlığı):** "# Available Skills" prompt bloğu modele
  **çıplak** `use_skill` adını söylüyordu (`skills/store.go renderCatalog`). Native
  ajanlarda araç gerçekten `use_skill`; ama **claude-cli** ajanlarında SwarmGo built-in'leri
  Interaction MCP köprüsünden **namespaced** geliyor: `mcp__swarmgo_interaction__use_skill`.
  Model prompt'u harfiyen izleyip çıplak adı deniyor → CLI reddediyor. (`trace.go` namespace'i
  soyduğu için başarılı 2. çağrı izde yine `use_skill` görünüyor — kafa karıştırıcı.)
- **Çözüm:** Katalog bloğu artık aracı **ajanın göreceği adla** yazıyor. `renderCatalog`
  skill-araç-adı parametresi aldı; `skills.DefaultSkillTool` sabiti + yeni
  `CatalogBlockForAgentTool(assigned, skillTool)`. `runtime.go skillToolNameFor(provider)`:
  provider `""`/`claude-cli` → namespaced, diğerleri (anthropic/minimax/openrouter/custom)
  → çıplak. `SkillsCatalogBlockForAgent` bunu kullanıyor (chat + autonomous yolları).
- **Doğrulama:** `TestSkillToolNameFor` (6 vaka) + `TestCatalogBlockForAgentTool`;
  `internal/agent` & `internal/skills` **88 test** yeşil, `go vet` temiz.

---

## SES21 hava-durumu oturumu hata düzeltmeleri ✅ (2026-06-22)

**Hedef:** Bir kullanıcı oturumunda (SES21, claude-cli ajanı) çıkan üç somut hatayı gider.

- **Shell `$` değişkeni bozulması (kök neden):** Ajan komutunu zaten-PowerShell olan
  `Bash` aracının içinde tekrar `powershell -Command "..."` ile sarmaladı → dış kabuk
  `$geo` vb. değişkenleri iç kabuğa geçmeden boşaltıp sildi ("An empty pipe element").
  Çözüm: (1) araç açıklamasına "tekrar `powershell -Command` ile sarmalama" uyarısı,
  (2) `unwrapRedundantPowershell` ile gereksiz dış sarmalı savunmacı olarak açma (yalnız
  tüm komut sarmalsa ve iç tarafta kaçışlı tırnak yoksa). `builtin_shell.go` + test.
- **Artifact geri okunamıyor:** `list_artifacts` yalnız id/başlık veriyordu; ajan
  içeriği göstermek için dosya yolunu (`.artifacts\...`) tahmin edip "File does not exist"
  aldı. Çözüm: yeni **`read_artifact`** aracı (id → içerik, bellekten; yol tahmini yok) +
  `list_artifacts` artık `contentFile` yolunu da döndürüyor. İkisi de `RiskRead`.
  `builtin_artifactmgmt.go`, `classify.go`, `toolsetup.go`.
- **Veri uydurma (skill davranışı):** `daily-weather-report` skill'i WebFetch başarısız
  olunca uydurma tablo yazıyordu. Skill'e eklendi: çok-şehir için Open-Meteo geocoding,
  weather.com kazımayı yasakla, **asla veri uydurma**, forecast≠iklim ortalaması,
  alt-ajana da aynı kuralı geçir.
- **Doğrulama:** `go build ./...` + `internal/tools` & `internal/db` **85 test** yeşil.
- **Açık kalan:** claude-cli köprüsünde `use_skill` ilk çağrıda "No such tool available"
  (MCP aracı ilk turda çözülmüyor) — ayrı, daha derin bir köprü konusu; not edildi.

---

## Modele göre akıllı varsayılan bütçe — Option B ✅ (2026-06-22)

**Hedef:** Flat 12K transcript bütçesi büyük modelin (200K–1M) penceresini boşa
harcıyordu. Pencere metadata'sını (önceki commit) gerçekten kullan.

- **`conversation.EffectiveBudget(provider, model, configured)`**: pencere biliniyorsa
  bütçe = `clamp(window × 0.10, configured, 32K)` — yapılandırılmış değer **taban**
  (asla altına inmez), 32K **tavan** (1M modelde maliyet guard'ı), bilinmeyen → değişmez.
- **`Manager.Prepare`** artık compaction tetiğini + pressure oranını model-aware bütçeyle
  hesaplıyor. `maxTokens≤0` (bütçe kapalı) dokunulmaz — `TestPrepareZeroPressure...`
  semantiği korundu (ilk denemede bu testi kırdım, `if maxTokens>0` guard'ıyla düzelttim).
- **Sonuç:** Opus 4.8/Sonnet 4.6 1M → 32K · Haiku 4.5 200K → 20K · MiniMax/DeepSeek/Gemini 1M → 32K · bilinmeyen → 12K.
- **Doğrulama:** `budget_test.go` + conversation/providers **51 test** yeşil, `go vet` temiz.
- **Follow-up:** §5 tool eşikleri hâlâ process-geneli bütçeyle (dormant); per-model
  `EffectiveBudget`'a bağlamak temiz sonraki adım. Detay: `17-TOKEN-OPTIMIZASYON.md` §7.

---

## Per-model context-window metadata ✅ (2026-06-22)

**Hedef:** Modellerin context-window boyutunu metadata olarak taşı (UI + gelecekteki
tokenLimitFor zemini). Kullanıcı isteği.

- **`ModelInfo.ContextWindow int`** (token, `contextWindow,omitempty`). `Catalog()`
  build-time'da merkezi **`ContextWindowFor(provider, model)`** aile-tablosundan
  doldurur → manifest'ler churn'den uzak kalır, yine her modelde değer görünür.
- **Aile-bazlı, muhafazakâr:** Opus 4.8/Sonnet 4.6 **1M**, Haiku 4.5 **200K**,
  MiniMax/DeepSeek/Gemini 1M (web'le doğrulandı: M3 = 1,048,576; Opus/Sonnet 1M,
  Haiku 200K), gerisi 0 = "bilinmiyor" → fallback. 40+ third-party OpenRouter
  modelini elle yanlış doldurmaktansa emin olunanlar.
  > **Düzeltme (2026-06-22):** İlk sürümde Claude ailesine düz 200K verilmişti; Opus
  > 4.8 ve Sonnet 4.6 aslında **1M**, sadece Haiku 200K. Per-tier eşlemeyle düzeltildi.
- **Test:** `context_window_test.go` (aile eşleme + Catalog dolduruyor mu) — providers
  paketi **43 test** yeşil, `go vet` temiz. `api`'ye dokunulmadı (JSON tag otomatik akar).
- **Phase 2 (tokenLimitFor) bilinçle ertelendi:** model penceresine ölçekleme SwarmGo'nun
  12K transcript bütçesiyle çelişir (bir tool sonucu tüm bütçeyi aşar); doğru hamle
  "modele göre akıllı varsayılan bütçe". Detay: `17-TOKEN-OPTIMIZASYON.md` §6.

---

## CG-9 ikinci yarı — bütçe-orantılı tool eşikleri ✅ (2026-06-22)

**Hedef:** the external agent project'ın `tokenLimitFor` (tool-result eşiği context window'a göre)
deseninin SwarmGo karşılığı. Model context-window metadata'sı yok (`ModelInfo`
sadece ID/Label), o yüzden mevcut **transcript bütçesine** (`MaxContextTokens`)
orantıladım — kullanıcının zaten modeline göre ayarladığı knob.

- **`Tunables.budgetScaleLocked()`** = `budget/12000`, clamp **[1×,5×]**.
  `CompactMaxBytes()` ve `CompactLLMThreshold()` artık base × scale döndürüyor.
  **Aynı faktör** → A-cap(16384) > B-eşik(12288) değişmezi her ölçekte korunur.
- **5× tavan** → B≈60KB, the external agent project'ın ~60KB özet tavanıyla örtüşür.
- **Default bütçe (12000) → 1×** → değerler birebir mevcut → **regresyon yok**.
- **`SetContextBudget`** setter + `applySettings` wiring (`s.convo.SetLimits` yanında).
- **Doğrulama:** `tunables_compact_test.go` + agent paketi **71 test** yeşil, `go vet` temiz.
- **Not:** wiring satırı (`server.go`) paralel oturumun MemGPT Parça-4 `/api/agents/{id}/core`
  route'larıyla aynı dosyada uncommitted → o paket bütünleşince commit'lenecek.
  Wiring olmadan `contextBudgetTokens=0` → 1× → güvenli no-op (dormant).

---

## Ayarlar ekranı kaydetme tutarsızlığı düzeltildi ✅ (2026-06-22)

**Hedef:** Ayarlar ekranında gösterilen ama Kaydet'e basınca diske yazılmayan
("sessizce kaybolan") alanları onar — UI/kaydetme tutarsızlığı denetimi.

- **Kök neden:** `SettingsPanel.tsx` `saveApp()` patch nesnesini elle alan-alan
  kuruyordu; panellerde render edilen 10 kontrol bu listede yoktu. Kullanıcı
  değiştirip Kaydet'e basınca patch alanı içermiyor, backend değişmemiş değeri
  döndürüyor ve `setOriginal(updated)` kontrolü eski haline geri alıyordu (hatasız).
- **Onarılan 10 alan:** `defaultPermissionMode` (Sağlayıcılar) + `reactiveCompact`,
  `maxTokenRetries`, `reactiveKeepRecent`, `compactToolOutput`, `compactMaxLines`,
  `compactMaxBytes`, `compactLlmSummary`, `compactLlmThreshold`, `compactModel`
  (Bağlam — Tur kurtarma + Sistem A/B sıkıştırma bölümlerinin tamamı). Hepsi
  `saveApp()` patch'ine eklendi.
- **İkincil:** `spawnMaxConcurrent`/`spawnMaxPerTurn` backend (Patch+store clamp+
  server canlı uygulama) ve skill dokümanında vardı ama frontend `AppSettings`
  tipinde, UI'da ve patch'te **yoktu**. Tipe eklendi, Tools paneline kontrol
  (1–128 / 1–64) eklendi, patch'e eklendi → uçtan uca bağlandı.
- **Doğrulama:** `tsc --noEmit` temiz. Skill `swarmgo-settings` zaten tüm alanları
  doğru belgeliyordu (değişiklik gerekmedi).

**Diğer düzenleme ekranlarının denetimi (aynı tur):** Workspace (`WorkspaceView`),
Ajan (`AgentSettingsForm`), Hooks (`HooksPanel`), Skill (`SkillEditor`), Görev
(`TaskDetailPanel`) ve Akış (`FlowsPanel`) ekranları tek tek denetlendi — **hepsi
temiz**: her biri render ettiği tüm alanları kendi create/update payload'una
gönderiyor (elle-omit yok). Workspace'te `instructions`/`boardColumns`,
SkillEditor'da `color` bilinçli/dökümante şekilde ayrı yüzeyde. Asıl hata yalnız
App Settings'teydi.

**Ölü alan temizliği:** `db.Agent.Capabilities` (`models.go`) kaldırıldı — hiçbir
yerde okunmuyordu (yalnız `store.go`'da `"[]"` default'lanıp market install'da
yazılıyordu, geri-publish yolu yok). 3 nokta: `models.go` alan, `store.go` default
bloğu, `api/market.go` atama. Ardından pack formatı
`market.AgentPayload.Capabilities` (SwarmPack v1) alanı da kaldırıldı — `omitempty`
olduğu için eski pack JSON'ları sorunsuz parse olur (alan varsa yok sayılır). Eski
agent JSON'larında migrasyon gerekmez. `go build` (db/api/market) temiz, db testleri
20/20, market testi geçti.

---

## CG-9 density-aware estimator + Sistem B varsayılan açık ✅ (2026-06-22)

**Hedef:** the external agent project kıyaslamasında çıkan iki açığı kapat — (1) yoğun içerikte token
undercount ("session poisoning"), (2) büyük araç-sonucu özetinin kutudan-kapalı olması.

- **CG-9 (birinci yarı):** `conversation/tokens.go` `estimateText` density-aware oldu.
  Tek geçişte rune+whitespace sayar; uzun & `<%3` boşluklu (≥256 rune) içerik **~1.5
  chars/token** (`runes*2/3`), düz metin **~4**. `utf8` importu düştü. `tokens_test.go`.
  Transcript bütçesi (12K) ve UI meter artık base64/hex'i doğru sayıyor. **Kalan:**
  tool-result eşiğini context window'a göre ölçekleme (CG-9 ikinci yarı).
- **Sistem B varsayılan açık:** `settings.Default()` + `tunables` sabitleri —
  `CompactLLMSummary: false→true`, `CompactLLMThreshold: 8192→12288`,
  `CompactMaxBytes: 12288→16384`. **Kritik:** A'nın cap'i B eşiğinin üstüne çıkarıldı,
  yoksa A çıktıyı B eşiğinin altına kırpıp B'yi pre-empt ediyordu. Doc 17 güncellendi.
- **Doğrulama:** conversation/settings/agent **89 test** yeşil; `TestEstimateTextDensity`
  ayrıca tek tek geçti. **Not:** `go build ./...` şu an paralel oturumun yarım MemGPT
  Parça-4 işinden (`ReadCore`/`WriteCore` 3-arg, `coreMemoryBlock`) **kırık** — benim
  paketlerim (api'ye bağımsız) izole derlenip test edildi; bozuk dosyalara dokunulmadı.

---

## Araç hizalama + tarih enjeksiyonu + bellek/arama CLI köprüsü ✅ (2026-06-22)

Üç bağımsız iyileştirme (kullanıcı isteği). Build+vet temiz, **289 test** yeşil,
`tsc --noEmit` temiz, backend rebuild+restart (127.0.0.1:8090).

1. **`get_current_time` kaldırıldı → tarih sistem prompt'unda.** Ajan saati artık
   tool round-trip yerine bağlamdan okur. `composeTurnRequest` dinamik bloğuna ve
   `autonomousSystemPrompt`'a tek satır eklendi (`dateTimeContextBlock`,
   `"Current date and time: Monday, 2006-01-02 15:04 (-07:00)"`). `builtin_time.go`
   + testi silindi. Bu, claude-cli'nin zaten yaptığının native eşleniği.

2. **Çekirdek araç isimleri claude-cli ile hizalandı.** `read_file→Read`,
   `write_file→Write`, `edit_file→Edit`, `list_dir→LS`, `glob→Glob`, `grep→Grep`,
   `shell→Bash` (`builtin_fs.go`, `builtin_shell.go`). Model bu isimlere yoğun
   eğitimli → daha güvenilir tool-use. `classify.go`/`permpattern.go` zaten her iki
   isim setini taşıyordu → tek sete indirgendi (`execArgTools={"Bash"}`). Yan
   referanslar güncellendi: subagent profilleri, `artifacts_auto.fileWriteTools`,
   hook şablonları, MarkLazy, bridge dispatch case, frontend (`DiffCard`/`tools.ts`/
   `stepKinds`). `http_get` kasıtlı korundu (CLI WebFetch'ten farklı; düz GET).

3. **`core_memory_replace/append` + `conversation_search` CLI'ye köprülendi.** Bu
   eager built-in'ler native-loop ctx bağımlılığı taşımadığından `BridgeTools` def
   listesine doğrudan eklendi (gate'leri `CoreMemoryTools()`/`SessionContextEnabled()`);
   dispatch zaten `bridgeCallFor()`→`reg.Call` ile çalışır, ekstra case yok. Native'de
   eager kalırlar. Artık CLI ajanı gördüğü core-memory bloğunu **düzenleyebilir** ve
   geçmişte derin arama yapabilir. (Stale `bridgeExcluded` run_subagent yorumu da
   düzeltildi.) Dokümanlar: `11-INTERACTION-MCP`, `26-MEMGPT`, `27-CROSS-SESSION`,
   `19-LAZY`, `09-SDK`, `25-SUBAGENT`, `06-WORKSPACES`.

## CG-16 — Oturumlar-arası tam-metin arama ✅ TAM (çekirdek+araç+API+UI, 2026-06-22)

**Hedef:** Workspace'in tüm oturum mesaj geçmişinde anahtar-kelime araması (bugün
yalnız başlık+summary üzerinden farkındalık vardı). Plan: `_Docs/27-CROSS-SESSION-SEARCH.md`.

**Kilit karar (RG-6 disiplininin ürünü):** Oturumlar boot'ta `d.messages`'a (RAM)
yükleniyor → aranacak veri zaten bellekte. Varsaymak yerine depolama modelini
okuyup **ripgrep/FTS5'i eledim**; saf-Go tarama hem en basit hem yeterli.

Yapılan:

- **`db.SearchMessages`** (`internal/db/store_search.go`): saf-Go RAM-içi tarama,
  boşlukla bölünmüş terimler AND-eşleşir (case-insensitive), `score = matchCount +
  recency` (C5 felsefesi), rune-sınırlı Türkçe-güvenli snippet. `SearchHit`/`SearchOpts`.
- **`conversation_search` aracı (N5)** (`tools/builtin_conversation_search.go`):
  `ListSessionsTool` deseni; `toolsetup.go`'da `SessionContextEnabled()` gate'i.
  `exclude_current` plandan düşürüldü (tools→agent import döngüsü); API'de `exclude`
  query param'ı karşılıyor.
- **API** `GET /api/sessions/search?q=&limit=&role=&exclude=`
  (`api/sessions_search.go` + `server.go` route, `active` yanında).
- **Frontend contract:** `SearchHit` tipi + `sessionApi.searchMessages(...)`.
- **Doğrulama:** `go build ./...` + db/tools/api/agent testleri **200** yeşil;
  `tsc --noEmit` temiz. Yeni testler: `store_search_test.go`,
  `builtin_conversation_search_test.go`.
- **Görsel arama ✅ (aynı gün):** `SessionsSidebar` arama kutusu çift işlevli —
  başlık filtresi + `api.searchMessages` mesaj araması (≥2 char, 250ms debounce,
  `cancelled` guard). "Mesajlarda (N)" bölümü rol-rozeti+snippet+yaş; tıkla →
  `onSelectSession(sessionId, messageId)`. `selectSession` opsiyonel `messageId` →
  `scrollToMsgId` → `MessageList`: her satır `data-msg-id`, hedefe `scrollIntoView`
  (center) + 1.6sn accent-ring flash, sonra `onHighlightConsumed`. `tsc` + `npm run
  build` + `go build` yeşil. **CG-16 tam kapandı.**

---

## RG-6 — Implement-öncesi keşif guard'ı ✅ (2026-06-22)

**Hedef:** Ajanın var olan bir özelliği "yok" sanıp sıfırdan yeniden yazma (veya
çalışan bir uygulamanın üzerine yazma) riskini önle. Faz R oturum-analizindeki en
büyük yanlış kararın mekanizma karşılığı; kod değil, skill/system-prompt düzeyi.

Yapılan:

- **`swarmgo-guide` SKILL.md → "Before you build: discover first" bölümü:** uygulamadan
  ya da "bu yok" demeden önce **search → read → confirm → extend** disiplini;
  absence iddiası ancak gerçekten arandıktan sonra ("Y ve Z için grepledim, bulamadım"),
  ve sıfırdan yazmak yerine mevcudu genişletme kuralı. Kod/konfig/agent/flow/skill/
  memory — hepsine uygulanır.
- **`swarmgo-self-management` → "Prefer reading first" güçlendirildi:** entity
  (agent/flow/skill/schedule/hook/MCP) oluşturmadan önce mevcudu kontrol et,
  duplicate yerine genişlet; guide bölümüne çapraz-referans.
- **Doğrulama:** `go build ./...` ✅ + `go test ./internal/skills/...` (14 test) yeşil.
  Skill testleri yalnız seed varlığı/non-overwrite kontrol ediyor; içerik değişimi
  güvenli.

---

## MemGPT/Letta Tarzı Self-Editing Bellek — Parça 1–3 ✅ (2026-06-22)

**Hedef:** Letta'yı (Docker+Postgres+Python) koşmadan, fikirlerini native Go'da:
bağlam-basıncı sinyali + ajanın in-place düzenlediği kalıcı **çekirdek bellek**.
Tek binary / offline / dosya-tabanlı kimliği korunur, migration yok. Plan +
sapmalar: `_Docs/31-MEMGPT-CORE-MEMORY.md`.

Yapılan (3 parça):

- **(1) Bağlam-basıncı sinyali:** `conversation.Prepared.Pressure`
  (`ContextTokens/maxTokens`, `maxTokens<=0 → 0`); `composeTurnRequest` eşik
  (`memoryPressureWarn`, vars. 0.75) aşılınca `SystemDynamic` başına "önemliyi
  şimdi yaz" uyarısı koyar — sessiz compaction'dan *önce*. Sıfır yeni model çağrısı.
- **(2) `core` bellek türü + Store API:** `db.MemoryCore` + `db.UpsertKnowledgeByKind`
  (ajan başına tek satır, ID korunur); `memory.Store.WriteCore/ReadCore/AppendCore`.
  Recall artık `recallKinds` allowlist'iyle core'u hariç tutar (her turda zaten
  sabit enjekte edildiğinden tekrar çıkmaz).
- **(3) `core_memory_*` araçları:** `tools.CoreMemoryTool` (replace/append) — eager,
  `coreMemoryTools` ayarıyla (vars. açık) gated; core bloğu her turda recall'ın
  üstünde `SystemDynamic`'e enjekte edilir. Letta'nın "core memory always in
  context" davranışı.
- **Ayar/UI:** `memoryPressureWarn` + `coreMemoryTools` → Settings/DTO/Patch/
  Tunables + `applySettings` canlı push; frontend ContextPanel "Çekirdek bellek
  (MemGPT)" bölümü.
- **Doğrulama:** `go build ./...` + ilgili paket testleri (conversation/memory/
  tools/api/agent/db/settings) yeşil; frontend `tsc --noEmit` temiz.
- **UI/API genişletmesi (ikinci tur):** Ayarlarda basınç eşiği **sürgü** + canlı
  yüzde + tetikleme-token'ı + "kapalı" durumu + canlı durum kartı (yeni `Slider`
  primitifi). Hafıza panelinde **çekirdek bellek kartı** (`CoreMemoryCard` —
  göster/düzenle) + `GET|PUT /api/agents/{id}/core` uç noktaları.
- **Parça 4a — persona/human ayrımı ✅ (2026-06-22):** çekirdek bellek iki bağımsız
  bölüme ayrıldı — persona + human, her biri ajan başına tek satır.
  > ⚠️ **2026-06-23'te Parça 5 ile geçersiz kılındı:** `core_persona`/`core_human`,
  > `section` parametresi, `ReadCoreSections` ve `{persona,human}` API'si artık
  > yok → `core:<label>`, `label`, `ReadCoreBlocks`, `{blocks}`. Bu girişin üstündeki
  > **"adlandırılmış bloklar"** ve **"HA-1"** girişlerine bak.
- **Devamı:** Parça 5 (adlandırılmış bloklar + limit) ve Parça 4b (HA-1
  oto-modelleme) **2026-06-23'te tamamlandı** — dosyanın başındaki güncel girişler.

## Oturum-başına Çalışma Dizini (cwd) + otonomi frenleri ✅ (2026-06-22)

**Hedef:** the external agent project'taki "working directory" mekaniği — her oturumun, ajanın
dosya/kabuk araçlarının çalışacağı bir cwd'si olsun; UI'dan değiştirilebilsin;
bağlama enjekte edilsin; ajan git ile repo editleyebilsin. Kilitsiz fs/shell'in
otonom yolda güvenli kalması için frenler.

Yapılan (4 adım uçtan uca):

- **(1) cwd çözümleme:** `Session.WorkingDir` (db) + `SetSessionWorkingDir`;
  `agent/workdir_ctx.go` (`effectiveWorkDir` + ctx taşıyıcı); `completeTraced`
  her turda çözüp `req.WorkDir` + ctx'e koyar; `buildRegistry` sandbox kökünü
  ctx'ten alır. Chat yollarına `WithSessionID` eklendi.
- **(2) Bağlam enjeksiyonu:** `api/workdir_context.go` — "Working directory"
  bloğu (cwd + git branch + CLAUDE.md ipucu) dinamik bağlama (chat_turn.go).
- **(3) Otonom fren:** `autonomousConfine` ayarı (varsayılan açık) — otonom
  turlar `NewConfinedSandbox` ile çalışma dizinine kilitlenir; `shell` confined
  modda `git push`'u reddeder (`isNetworkMutatingGit`).
- **(4) Worktree izolasyonu:** `gitWorktreeIsolation` ayarı (varsayılan kapalı) —
  `agent/worktree.go` otonom oturuma `<workspace>/worktrees/<sid>` worktree+dal
  verir; oturum silinince `RemoveSessionWorktree` temizler.
- **API:** `GET/PUT /api/sessions/{id}/workdir`, `GET /api/fs/browse` (`api/workdir.go`).
- **Frontend:** `chat/WorkDirBadge.tsx` (klasör rozeti + dizin gezgini); Ayarlar ▸
  iki yeni toggle; types/api alanları.
- **Doğrulama:** `go build ./...` + `go test ./...` + frontend `tsc -b` + `npm run
  build` yeşil. Detay: `_Docs/26-CALISMA-DIZINI.md`.

### Workspace varsayılan çalışma dizini + canlı doğrulama (2026-06-22)
- `WSSettings.DefaultWorkingDir` (ws-settings.json) + Runtime `SetDefaultWorkDir`/
  `WorkspaceDefaultDir`; `effectiveWorkDir` artık oturum → workspace-default →
  fiziksel workDir sırasıyla çözüyor. Ayarlar ▸ Bu Workspace ▸ "Varsayılan çalışma
  dizini" alanı (`WorkspacePanel.tsx`, `workspace_settings.go` DTO/patch).
- **Canlı duman testi (8088):** `GET /api/fs/browse` ✓; bir oturuma SwarmGo deposu
  set edildi → `{exists:true,isGitRepo:true,branch:"main"}` ✓ (git branch tespiti),
  sonra sıfırlandı. **Not:** varsayılan 8080 portu mcp-for-unity backend'iyle
  çakıştığı için bu örnek `SWARMGO_ADDR=127.0.0.1:8088` ile çalışıyor.

## fs/shell sandbox kilidi kaldırıldı (kilitsiz dosya/komut erişimi) ✅ (2026-06-22)

**Hedef:** Built-in dosya/komut araçlarının workspace dizinine kilitli olma
güvenlik kısıtını kaldırmak — the external agent project (external-agent-oss) gibi araçların makinedeki
herhangi bir yola erişebilmesi, güvenliği tek başına **izin moduna** bırakmak.

Yapılan:


---

> **Daha eski kayitlar (2026-06-19 ve oncesi) arsivlendi:** [05-ARSIV.md](05-ARSIV.md).
