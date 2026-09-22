# 49 — Mobil / Dikey Ekran Uyumlu UI (Responsive)

> **Durum:** F1 + F2 + F3 + panel UX iyileştirmeleri UYGULANDI ✅ (2026-07-04);
> **dört katmanlı kabuk** (dar / kare / geniş / çok geniş + en-boy oranı, durum
> geçişleri) UYGULANDI ✅ (2026-09-05, §7.7); **tüm sol liste panelleri
> daraltılabilir** (varsayılan açık, yeniden-açma rayı, İçgörü standarda uydu)
> UYGULANDI ✅ (2026-09-05, §7.8) — kalan F4 tasarım 📐. Bkz. §7.
> **Amaç:** TionHarness web UI'ını **dikey (portrait) telefon ekranlarına** uyumlu
> hale getirmek. Bugün UI **masaüstü-sabit** (yatay çok-sütunlu düzen, sabit
> genişlikli raylar/sidebar'lar); telefonda kullanılamaz. Bu doküman kırılma
> noktalarını tespit edip mobile-first bir dönüşüm planı önermişti (analiz arşivde, §1–6).
> **Kapsam bağlamı:** `48-VPS-REMOTE-CLIENT` uzak mobil istemci (PWA/WebView APK)
> vizyonunun **F4 (mobil UI cilası)** fazının derinleştirilmiş hâlidir — uzak
> istemci ancak UI dikey ekrana uyunca kullanışlı olur.

İlgili dokümanlar: `48-VPS-REMOTE-CLIENT` (uzak mobil istemci + F4), `32-NATIVE-PENCERE`
(WebView2 masaüstü — bunun mobil eşi), `07-CHAT-UX` (chat düzeni), `30-COKLU-PENCERE`.

---

## 1–6. Analiz ve plan

> Uygulama öncesi analiz (portrait kırılma tespiti, `App.tsx:satır` referansları,
> tasarım yaklaşımı, bileşen listesi, faz planı, açık kararlar) →
> [arsiv/49-MOBIL-RESPONSIVE-UI-ANALIZ.md](arsiv/49-MOBIL-RESPONSIVE-UI-ANALIZ.md).
> Faz planından **açık kalanlar:** **F4** — grafik/canvas görünümleri (Flows / Network /
> TaskBoard) için "görüntüle + uyarı" (opsiyonel liste-fallback); **F5** — kalan
> sabit-genişlik denetimi, dokunmatik hedef boyutları (min 44px), erişilebilirlik,
> gerçek cihaz testi.

---

## 7. Uygulama Durumu

### 7.1–7.3 F1–F3 — kabuk, modallar, liste panelleri ✅ (2026-07-04)

Portrait telefon için tek-sütun kabuk, bottom-sheet modallar ve tek-sütun liste
panelleri sevk edildi. Dosya-dosya döküm **git geçmişindedir**; kalıcı olarak
bilinmesi gerekenler:

- **Breakpoint tek kaynak:** `shared/hooks/useMediaQuery.ts` → `useIsMobile()`
  (`max-width: 767px` = Tailwind `md` altı). Tailwind `md:` sınıflarıyla senkron
  kalması için breakpoint başka yerde tekrar edilmez.
- **Mobil navigasyon:** `app/MobileNavBar.tsx` (altta `fixed bottom-0`,
  `md:hidden`); `NavRail`'in `NAV` dizisi export edilip tek kaynak yapıldı.
- **Drawer deseni:** yan paneller (oturum listesi, ajan roster'ı, detay paneli)
  masaüstünde sütun (`md:static`), mobilde `max-md:fixed` + `translate-x` slide-in
  + backdrop; seçimden sonra kapanır.
- **Modal tek kaldıracı:** `shared/components/ModalOverlay.tsx` — `< md`'de overlay `items-end`
  ve `max-md:[&>*]:!w-full !max-w-none !max-h-[92dvh] !rounded-b-none` ile **13 modal**
  birden bottom-sheet olur. `!important`, çocuğun sabit `w-*`/`max-w-*` sınıflarını ezer.
- **Genel kural:** tüm mobil davranış `max-md:` / `md:hidden` altındadır →
  **`>= md`'de düzen birebir eski hâlidir** (masaüstü regresyonu yok).
- **`!w-full` neden `!important`:** resizable panellerin inline `style={{width}}`'ini
  ezmek için (inline stil > sınıf; `!important` > inline).

**Bu fazların dışında bırakılanlar:** grafik/canvas görünümleri (Flows, Network,
Board) → **F4**; dokunmatik ergonomi cilası, master-detail navigasyonu ve
`Schedules`/`Budget`/`Logs` kart taşma denetimi → **F5**.

### 7.4 Panel UX iyileştirmeleri — daraltılabilir listeler + otomasyon renkleri ✅ (2026-07-04)

Kullanıcı isteği üzerine panel listeleri **daraltılabilir** yapıldı (F3'ün mobil
top-bottom stack'i yerine masaüstü+mobil ortak **aç/kapa** deseni) + çeşitli
cilalar. `tsc -b && vite build` temiz.

- **Yeniden kullanılabilir daraltılabilir liste deseni (yeni):**
  - `shared/hooks/useCollapsibleList.ts` — kalıcı aç/kapa state; varsayılan masaüstü açık,
    telefon kapalı.
  - `shared/components/CollapsibleListShell.tsx` — açık: masaüstü sütun / mobil soldan **drawer**
    (backdrop); kapalı: ince dikey **yeniden-açma rayı** (`PanelLeftOpen` + dikey
    etiket). Sohbet sessions sidebar'ının genelleştirilmiş hâli. **Not (düzeltme
    2026-07-04):** açık sarmalayıcıya `bg-[var(--color-surface)]` + gölge eklendi —
    önceki hâli şeffaftı, mobil drawer koyu backdrop üstünde saydam bir çubuk gibi
    görünüyordu; artık sessions sidebar gibi dolu surface.
  - `shared/components/SidebarChrome.tsx` `SidebarHeader`'a opsiyonel `onCollapse` (◀ butonu).
- **Uygulandığı paneller (istek 4-5):** `ArtifactsPanel`, `SkillsPanel`,
  `ToolsPanel`, `MarketPanel` (kategori rayı) + `FlowsPanel` (sol akış listesi).
  F3'ün bu panellerdeki `max-md:flex-col` + `max-h-[45vh]` stack'i **geri alındı**
  (artık drawer). (Executions + Agents roster hâlâ F3 stack — istek dışıydı.)
- **Flows node inspector (istek 4):** editördeki sağ `w-72` node paneli de
  daraltılabilir (`PanelRightClose`/`PanelRightOpen`, `tionharness.flowInspectorOpen`).
- **Agents aktivite paneli (istek 1):** `AgentActivityPanel` sohbet DetayPaneli gibi
  aç/kapa (`onClose` X + kapalıyken sağda "Aktivite" rayı; `tionharness.agentActivityOpen`);
  mobilde tam-genişlik stack.
- **Agents "yolu kopyala" (istek 2):** `AgentSettingsForm`'da icon-only
  (`labelClassName="hidden"`).
- **Otomasyon ekranı renk ayrımı (istek 3):** `Schedules` → sky (mavi) sol şerit
  `border-l-sky-500` + "Zamanlamalar (cron)" başlığı (Clock ikonu sky); `Automations`
  → violet (mor) sol şerit `border-l-violet-500` + başlık ikonu violet. Oluşturma
  formları, satırlar ve inline-edit kartları renk kodlu → hangisi ne, bir bakışta belli.

### 7.5 Standardizasyon — `ListPane` (tek liste-kolonu bileşeni) ✅ (2026-07-04)

İki-panelli ekranlar kendi liste-kolonu çözümlerini uyguluyordu (resizable vs sabit
vs özel resize; farklı bg/border/handle; kimi collapsible kimi değil). Tek standarda
indirgendi. `tsc -b && vite build` temiz.

- **`shared/components/ListPane.tsx` (yeni):** standart sol liste kolonu = `CollapsibleListShell`
  (daralt→rail + mobil drawer) + dolu surface `<aside>` + sağ border + `useResizableSidebar`
  (kalıcı genişlik) + `ResizeHandle`. Ekran sadece header+gövdeyi `children` verir.
- **Taşınanlar:** Artifacts, Skills, Tools, Market, Flows, Executions. Skills'in özel
  resize kodu silindi; Tools/Market resizable oldu; Executions daraltılabilir oldu;
  hepsi aynı tema/genişlik/drawer.
- **Kapsam dışı:** `AgentsView` 3-panelli (roster | ayarlar | aktivite) özel layout —
  mobil `flex-col` stack + aktivite paneli, ListPane'in rail/drawer mantığıyla
  çakışıyor; paylaşılan primitiflerde (SidebarHeader + useResizableSidebar + ResizeHandle)
  bırakıldı. İstenirse ayrı ele alınabilir. **(Güncelleme: §7.6'da Agents da ListPane'e
  taşındı.)**

### 7.6 Standart ekran başlığı — `PaneHeader` + başlıktan liste aç/kapa ✅ (2026-07-04)

Tüm liste ekranları sohbet ekranı gibi bir **başlık çubuğu + tıklanabilir liste
aç/kapa butonu** kazandı. Kapalıyken liste tamamen gizlenir (ince ray yok — sohbet
sessions gibi), başlıktaki butondan yeniden açılır. `tsc -b && vite build` temiz.

- **`shared/components/PaneHeader.tsx` (yeni):** standart ekran başlığı — sol toggle butonu
  (PanelLeft, aktif/pasif accent) + başlık + opsiyonel subtitle + sağ aksiyon slotu.
- **`CollapsibleListShell` + `ListPane` `hideRail` (yeni prop):** kapalıyken ray yerine
  `null` döndürür; yeniden açma dış PaneHeader/başlık toggle'ıyla olur.
- **PaneHeader eklenen 7 ekran:** `AgentsView` (roster **ListPane'e taşındı**;
  subtitle = seçili ajan adı / "· Ajan seçilmedi"; boş-durum "soldan bir ajan seç"
  korunur → sohbet gibi başlık + boş ekran), `ArtifactsPanel`, `ToolsPanel`,
  `MarketPanel` (toggle mevcut katalog başlığına eklendi, çift başlık olmasın),
  `SkillsPanel`, `ExecutionsPanel`, `FlowsPanel`.
- **App-header toggle eklenen 2 ekran (Workspace + Ayarlar):** bunlar zaten App
  başlığıyla (VIEW_TITLE) geliyor → kategori/sekme `<aside>`'ları `CollapsibleListShell
  hideRail` ile sarıldı, collapse state App'te tutulur (`workspaceNav`/`settingsNav`),
  App header'ında `PanelLeft` toggle butonu render edilir.
- Böylece 9 liste ekranı da: başlık + başlıktan aç/kapa + mobil drawer + boş durum
  bakımından **tek standart**.

### 7.7 Dört katmanlı kabuk — kare ve çok geniş ekranlar + durum geçişleri ✅ (2026-09-05)

F1–F3 yalnız "md altı = telefon" ikilisini biliyordu; 768–1279px arası tablet /
yarım ekran pencereler (üç kolon sığmaz) ve ≥ 1920px monitörler (satırlar
okunamayacak kadar uzar) masaüstü gibi davranıyordu. Bu adım genişliği dört
katmana, en-boy oranını üç sınıfa ayırır ve her kolonun dock/drawer modunu buradan
türetir.

- **Tek kaynak:** `shared/lib/viewport.ts` — `classifyWidth` (narrow < 768, square
  768–1279, wide 1280–1919, ultra ≥ 1920; Tailwind `md`/`xl` ve `index.css`'teki
  `--breakpoint-3xl: 120rem` ile birebir), `classifyAspect` (portrait < 0.9,
  square ≤ 1.25, landscape), `capListColumnWidth` (kare katmanda liste kolonu
  ≤ 256px). `useIsMobile()` artık aynı tablodan okur.
- **Hook'lar:** `shared/hooks/useViewport.ts` (`useSyncExternalStore`, tek resize
  aboneliği, yalnız katman/oran değişince bildirim; `useViewportAttribute()`
  `<html data-viewport data-aspect>` damgalar) ve `app/useShellLayout.ts`
  (`computeShellLayout` saf, test edilir): rail `hidden|compact|full`, liste
  `drawer|docked`, detay `drawer|docked`, ölçü `fluid|centered`.
- **Kare katman:** `NavRail` ikon-raya zorlanır; workspace düğmesi rail'i içerik
  üstünde **peek** olarak açar (`absolute z-40`, akış yuvası 3.5rem sabit;
  `useOutsideClick`, görünüm seçimi ve katman değişimi kapatır, `data-rail-mode`
  ile gözlenir). Detay paneli `app/App.tsx`'te `shell.detail === 'drawer'` iken sağdan
  giren drawer + `Backdrop`; portrait monitörlerde de (ör. 1200×1600) drawer.
- **Ultra katman:** `.th-measure` (`styles/layout.css`) transcript / composer /
  `ComposerCard` / `SessionStartPanel` üzerinde; `--th-measure: 88rem` yalnız
  `[data-viewport='ultra']`'da, altında yalnız oluk (`--th-gutter`). Ayarlar ve
  Workspace form kolonları `3xl:max-w-4xl`; Market ızgarası ve pano sütunları
  `3xl:` ile genişler.
- **Durum geçişleri:** `.th-col` genişlik geçişi (sürükleme sırasında
  `body[data-th-resizing]` ile kapalı), `.th-drawer` transform, `.th-drawer-from-*`
  ve `.th-backdrop` `@starting-style` giriş animasyonu, `.th-column` max-width.
  `prefers-reduced-motion` altında hepsi anlık.
- **Paylaşılan drag:** `SessionsSidebar` kendi drag/persist kodunu bıraktı;
  `useResizableSidebar` (+ `ResizeHandle`) kullanır. Hook `capToTier` seçeneği
  aldı (detay paneli `false`), `localStorage` erişimi try/catch'li.
- **Izgaralar:** sabit `grid-cols-N` form ızgaraları `grid-cols-1 sm:grid-cols-N`
  oldu; `StatTiles` kare katmanda 3 sütun (`square:` custom variant).
- **Doğrulama:** `viewport.test.ts`, `useShellLayout.test.ts`; tarayıcıda 390×844
  (drawer + backdrop), 1024×768 (compact rail, peek, liste 256px, detay drawer
  320px), 1440×900 (tam dock), 2560×1440 (transcript 1768px, iki yanda 180px ölçü
  paddingi).

### 7.8 Tüm sol liste panelleri daraltılabilir — tek standart, varsayılan açık ✅ (2026-09-05)

§7.4–7.6'nın daraltma modeli bir ara "md+'da her zaman görünür kolon"a
indirgenmişti (`useCollapsibleList` yalnız mobil drawer bayrağı tutuyordu,
`hideRail` her yerde). Kullanıcı isteğiyle daraltma geri geldi ve **her** sol
liste paneline uygulandı; İçgörü'nün özel `aside`'ı da standarda taşındı.

- **Durum modeli** (`shared/hooks/useCollapsibleList.ts`): md+ → kalıcı dock
  aç/kapa (`storageKey`, varsayılan açık; yalnız `'0'` daraltır), narrow → kalıcı
  olmayan drawer (varsayılan kapalı). Katman `useViewport().tier` ile belirlenir.
- **Kabuk** (`shared/components/CollapsibleListShell.tsx`): kapalı+md+ → kolon
  unmount, ince `.th-rail` düğmesi (36px, ikon + `writing-mode: vertical-rl`
  etiket, `data-testid="<id>-rail"`); açık → `.th-pane-enter` keyframe. Narrow
  → önceki drawer davranışı (`th-drawer`, `Backdrop`). `data-list-open` ile
  gözlemlenebilir. `hideRail` kaldırıldı.
- **Kontroller:** `PaneHeader` liste düğmesi her genişlikte (`PanelLeft`,
  kapalıyken accent); `AppHeader` sohbet ve ayar/workspace düğmeleri her
  genişlikte; `SidebarHeader onCollapse` / `CollapseListButton` panel içinde.
- **Uygulanan paneller:** Sohbet oturumları (`app/App.tsx` → `CollapsibleListShell`,
  `tionharness.sessionsListOpen`), Ajanlar, Artifactlar, Skills, Araçlar,
  Market, Akışlar, Hedefler, Ayarlar ve Workspace kategori rayları, **İçgörü**
  (`ListPane` + `SidebarHeader` + sağ kolonda `PaneHeader`).
- **Sohbet özel durumu:** oturum seçimi / yeni sohbet / görünüm değişimi yalnız
  dar ekranda drawer'ı kapatır; masaüstü dock durumu korunur.
