# TionSwarm — Workspace İzolasyonu

> Her workspace **tamamen bağımsızdır**: kendi dosya-tabanlı `store/` dizini + kendi agent runtime'ı. Bir workspace'in içeriği asla diğerine sızmaz.
> Depolama biçimi (JSON/JSONL) için: **`_Docs/08-DEPOLAMA.md`**.

## Başlangıç (favori) workspace (2026-06-23)

Workspace switcher dropdown'ında her satırda bir **yıldız** ile bir workspace
"başlangıç favorisi" olarak işaretlenebilir. Soğuk açılışta (URL'de açık bir
workspace deep-link'i yokken) favori workspace açılır; deep-link verildiğinde
route makinesi onu sonradan uygular, yani açık linkler favoriyi ezer. Cihaz-yerel
saklanır (`localStorage: tionswarm.favoriteWs`), aktif-workspace işaretçisiyle aynı
desen. Seçim sırası: `(işaretçi yoksa) favori → son-aktif → favori → ilk`.
Kod: `hooks/useWorkspaces.ts`, `WorkspaceSwitcher.tsx`.

## Workspace'e Özel Görünüm/Tema (2026-06-25)

Görünüm ayarları artık **uygulama-geneli Ayarlar'da değil**, NavRail ▸ **Workspace**
penceresinin **Görünüm** sekmesindedir (`WorkspaceView.tsx` sekmeleri: Genel ▸
**Görünüm** ▸ Proje ▸ Promptlar & Dosyalar). Ayar **aktif workspace'e özeldir**:
her workspace kendi tema paletini, temel modunu (koyu/açık/sistem) ve vurgu
rengini (accent) saklar; **workspace değiştirince arayüz teması da değişir**.

- **Saklama:** `ws-settings.json` içinde `theme`/`accent`/`themePreset` alanları
  (`internal/workspace/settings.go` → `WSSettings`). Boş alan = **uygulama-geneli
  görünümü miras alır** (`settings.json`'daki `theme`/`accent`/`themePreset`
  varsayılan rolünü sürdürür).
- **Çözümleme:** `frontend/src/lib/theme.ts::resolveAppearance(ws, global)` her
  alanı tek tek birleştirir (boş → global). `applyAppearance` belgeye uygular.
- **Uygulama akışı:** `App.tsx` global görünümü ve aktif workspace override'ını
  iki ref'te tutar; workspace değişiminde `GET /api/workspace-settings` ile
  override çekilip `applyResolvedTheme()` çağrılır. Global ayar kaydı (örn. Profil)
  workspace seçimini ezmez.
- **Düzenleme:** `AppearancePanel` (self-contained; global+workspace ayarını kendi
  çeker) override'ı doğrudan `PUT /api/workspace-settings` ile kaydeder, düzenlerken
  **canlı önizleme** uygular, kaydedilmeden çıkılırsa önizlemeyi geri alır. "Genele
  sıfırla" butonu alanları temizleyip workspace'i tekrar global görünüme döndürür.
  Panel `WorkspaceView`'in Görünüm sekmesinde gömülüdür (kendi Kaydet/Sıfırla
  butonlarını taşır → ortak header Kaydet'i bu sekmede gizli).
- **Dil seçeneği** Görünüm'den çıkarılıp uygulama-geneli **Profil** kategorisine
  (Ayarlar) taşındı; dil uygulama-geneli kalır (workspace'e özel değildir).

## Workspace Dışa Aktarımı (Şablon) — "Dışa Aktar" Sekmesi (2026-07-01)

Workspace'i taşınabilir bir **şablon paketine** dönüştürme (eski "Şablon olarak yayınla")
artık `WorkspaceView`'in kendi **Dışa Aktar** sekmesindedir (Genel ▸ Görünüm ▸ Proje ▸
Promptlar & Dosyalar ▸ **Dışa Aktar**). Genel tab'ından çıkarıldı çünkü içerik seçimi
detaylandırıldı. Panel: `frontend/src/components/workspace/WorkspaceExportPanel.tsx`.

- **Öğe-bazlı seçim (2026-07-01):** Ajanlar, Akışlar, Workspace skill'leri ve Zamanlamalar
  artık **tek tek** seçilir (dört ayrı checkbox listesi — yeniden kullanılabilir
  `ExportPickList.tsx`). Her liste varsayılan tümü-seçili; ajanlarda ≥1 zorunlu. Dosya
  kategorileri (Talimatlar / Promptlar & README / Pano sütunları) toggle olarak kalır.
- **Promptlar & README (2026-07-01):** "Promptlar & Dosyalar" ekranındaki runtime promptları
  (summary/reflect/title/…) ve README de dışa aktarılabilir — **yalnız varsayılandan farklı
  olanlar**. Panel `getWorkspaceConfig`'ten non-default prompt + README sayısını gösterir.
  Backend `WorkspacePayload.Prompts/Readme` taşır; install'da `config/prompts/` + `config/README.md`
  olarak seed edilir (`seedTemplateConfigFiles`). Dokunulmamış promptlar hedefte güncel varsayılanı korur.
- **Şablon metadata (2026-07-01):** panelin üstünde **Şablon adı / Açıklama / Sürüm**
  alanları vardır. Ad workspace adından seed edilir; açıklama/sürüm boş bırakılırsa
  sunucu varsayılan üretir (açıklama → otomatik satır, sürüm → `1.0.0`). Ad slug'ı
  belirlediği için farklı ad = farklı pack id (`workspace-<slug>`).
- **Bağımlılık uyarısı (2026-07-01):** bir ajan hariç bırakıldığında panel, o ajana
  bağlı **akış** ve **zamanlamaları** listeler. Etki net gösterilir: akışlar dışa
  aktarımda **kopuk referans** taşır (backend yalnız dışa aktardığı id'leri yeniden
  yazabilir), zamanlamalar **tamamen düşer** (orphan atlanır). Flow bağımlılığı,
  `flow.graph` JSON'u client'ta parse edilip `type==='agent'` düğümlerinin `agentId`'leri
  toplanarak hesaplanır.
- **Canlı önizleme (2026-07-01):** kategori toggle'larının altında, mevcut seçimin **tam
  çıktısını** gösteren bir kart. Sayımlar backend kurallarını birebir yansıtır: akış/skill/
  talimat/pano toggle'ına uyar; **zamanlama yalnızca seçili ajana bağlıysa** sayılır (orphan
  düşer). Kart ayrıca çözümlenen **ad · sürüm · pack id**'yi (`workspace-<slug>`) ve aynı
  slug'a yeniden yayında **üzerine yazma** uyarısını gösterir. Slug, Go `slugify`'ın frontend
  kopyasıyla hesaplanır (Türkçe/ASCII-dışı harfler düşer → önizleme sunucuyla aynı id'yi verir).
- **API:** `api.publishPack('workspace', ws.id, include, meta)` → `POST /api/market/publish`.
  `include` = `WorkspaceExportInclude`: `agentIds/flowIds/skillSlugs/scheduleIds: string[]|null`
  (**tri-state**: null=tümü, `[]`=hiçbiri, liste=tam üyeler) + `instructions/prompts/boardColumns`
  boolean'ları; `meta` = `WorkspaceExportMeta` (`name?/description?/version?`, boş alanlar düşürülür).
  `include`/`meta` gönderilmezse davranış eskisi gibi "her şey" (geriye uyumlu).
- **Backend:** `publishRequest.{Name,Description,Version,Include}`; workspace kind'ında
  bunlar `market.BuildWorkspacePack(slug, name, desc, version, …)`'e geçirilir (boş =
  türetilmiş varsayılan; `BuildWorkspacePack` boş sürümü `1.0.0`'a düşürür).
  `buildWorkspaceTemplatePayload(ctx, wsp, inc)` — `inc==nil` iken her şey, aksi halde
  `wantSet(all, ids)` (nil=tümü / `[]`=hiçbiri) ile agents/flows/skills/schedules bağımsız
  filtreler ve dosya flag'lerini onurlandırır (`internal/api/market_publish.go`). **Sırlar ve
  oturum geçmişi asla dahil edilmez** (sunucu-zorunlu). Yayınlanan şablon market + workspace
  oluşturma ekranında görünür (`s.market.Reload()`).
- Panel kendi publish aksiyonunu taşır → ortak header "Kaydet" bu sekmede gizli (Görünüm gibi).

## Neden Ayrı Store Dizini?

Tek depo + `workspace_id` ayrımı yerine **her workspace için ayrı `store/` dizini** seçildi:

| Yaklaşım | İzolasyon | Risk |
|----------|-----------|------|
| Tek depo + workspace_id ayrımı | Mantıksal (her erişim filtrelemeli) | Bir erişim filtreyi unutursa **sızıntı** |
| **Ayrı store dizini (seçilen)** | **Fiziksel** | Sıfır — erişim değişmez, karışma imkânsız |

## Mimari

```mermaid
graph TD
    REQ[HTTP İstek<br/>X-Workspace-Id header] --> MW[withWorkspace<br/>middleware]
    MW --> RES{Workspace çöz}
    RES -->|id| WS[İlgili Workspace]
    RES -->|boş| DEF[Varsayılan Workspace]
    WS --> H[Handler<br/>ws.DB + ws.Runtime]
    MGR[Manager] --> W1[Workspace 1<br/>DB + Runtime]
    MGR --> W2[Workspace 2<br/>DB + Runtime]
```

## Disk Yapısı

```
DATA_DIR/
├── workspaces.json              # [{id, name, createdAt}] kayıt defteri (id artık WS<n>)
├── ws-counter.json              # workspace id sayacı ({"n":N}) — tekrar-kullanımsız
├── credential-secret            # global şifreleme anahtarı
└── workspaces/
    ├── {id-1}/                   # örn. WS1/ (eski kayıtlar UUID kalabilir; bkz. cmd/migrate-ids)
    │   ├── store/               # Workspace 1'in TÜM verisi (JSON/JSONL dosyaları)
    │   ├── config/              # Editlenebilir config: prompts/*.md, instructions.md, README.md
    │   ├── ws-settings.json     # Workspace ayarları (override + instructions)
    │   └── workspace/           # Workspace 1'in görev dosyaları (ajan fs/shell çalışma dizini tabanı; artık KİLİT değil — bkz. not)
    └── {id-2}/
        ├── store/
        ├── config/
        └── workspace/
```

> **`config/` klasörü (2026-06-17):** runtime yardımcı promptları (summary/reflect/title),
> workspace talimatları ve README **editlenebilir dosyalar** olarak burada tutulur. Hem
> kullanıcı (diskten) hem uygulama (Ayarlar ▸ Bu Workspace ▸ Promptlar & Dosyalar) düzenler.
> Bir prompt dosyası boş/yoksa uygulama gömülü varsayılana düşer. Detay: `agent/wsconfig.go`,
> `api/workspace_config.go`.

## Çalışma Şekli

1. **Manager** (`internal/workspace/manager.go`) açılışta `workspaces.json`'ı okur, her workspace için `store/` dizinini açar (diskten belleğe yükler) + runtime başlatır.
2. Hiç workspace yoksa **"Varsayılan"** otomatik oluşturulur.
3. Her HTTP isteği `X-Workspace-Id` header'ı taşır; `withWorkspace` middleware'i doğru workspace'i çözüp context'e koyar.
4. Handler'lar `ws(r).DB` ve `ws(r).Runtime` ile yalnızca o workspace'in verisine erişir.
5. Frontend aktif workspace id'sini **URL hash'inde** (`#/w/{id}/{view}`) kaynak-doğru olarak tutar; `localStorage` yalnızca hash'siz açılışta tohum (fallback) olarak okunur. Switcher'dan geçince tüm liste (ajan/oturum/mesaj) sıfırlanıp yeniden yüklenir.

## Çoklu Pencere / Derin Bağlantı (Deep-Link)

Tüm navigasyon durumu URL hash'inde adreslenir → **her workspace ayrı bir tarayıcı
penceresinde/sekmesinde eşzamanlı açılabilir** (paylaşılan localStorage'a rağmen
çapraz-sızıntı yok).

```
#/w/{workspaceId}/{view}[/{entityId}]
   workspaceId → isteği izole backend DB'sine kapsar (X-Workspace-Id header'ına dönüşür)
   view        → NavRail görünümü (chat/board/agents/…)
   entityId    → görünüme göre: chat→sessionId, agents/memory/tools→agentId,
                 artifacts→artifactId, schedules→scheduleId
```

**Nasıl çalışır:**
- `lib/url.ts` (`parseRoute`/`buildRoute`/`routeIdForView`) + `hooks/useUrlSync.ts`
  (state↔URL iki-yönlü senkron: ilk yazım `replaceState`, sonrası `pushState` →
  geri/ileri tuşları çalışır).
- `App.tsx` modül yüklenirken `INITIAL_ROUTE = parseRoute(hash)`'i okur; hash bir
  workspace adresliyorsa **hemen** `setActiveWorkspace` ile api istemcisine pinler.
- Aktif workspace `api/client.ts`'te **modül-düzeyi değişkende** tutulur; runtime'da
  yalnızca buradan okunur (localStorage runtime'da yeniden okunmaz). Her pencere kendi
  JS realm'ine sahip olduğundan, bir penceredeki workspace değişimi diğerini etkilemez.
- **WorkspaceSwitcher** her satırda **"Ayrı pencerede aç"** (↗ `ExternalLink`) butonu
  sunar → `window.open(origin+pathname+#/w/{id}/chat)` ile o workspace'e pinli yeni
  pencere açar; mevcut pencerenin seçimini bozmaz.

> Not: Bu bir **web uygulaması** (Electron/native değil) — "ayrı pencere" tarayıcı
> penceresi/sekmesi demektir. Görev çubuğunda bağımsız uygulama penceresi istenirse
> ileride Electron/Tauri sarmalayıcı veya tarayıcının PWA modu gerekir.

### Çapraz-pencere "okunmadı" rozet senkronu

Workspace etkinlik rozetleri (`unreadWs`) **tüm pencereler arasında paylaşılır**
(`localStorage` anahtarı `tionswarm.unreadWs` + `storage` event). TionSwarm tek-kullanıcılı
olduğundan ilke: **"herhangi bir pencerede görüldü = her yerde okundu"**.

- `hooks/useWorkspaces.ts` paylaşılan ham seti `localStorage`'da tutar; her yazımda
  (`writeSharedUnread`) diğer pencereler `storage` event'iyle anında senkron olur
  (`storage` yazan dökümanda tetiklenmez, yalnız diğerlerinde → yazım/okuma döngüsü yok).
- `markWorkspaceUnread(id)` rozet ekler (kendi aktif workspace'i hariç),
  `markWorkspaceRead(id)` her yerden temizler. `App.tsx` olay feed'inde: başka
  workspace'te etkinlik → `markWorkspaceUnread`; **aktif workspace'te canlı etkinlik veya
  o workspace'e geçiş** → `markWorkspaceRead` (görüldü kabul edilir).
- **Görüntülenen** set her pencerede ham setten **kendi aktif workspace'i çıkarılarak**
  türetilir (aktif olan asla rozetlenmez). Aktif-workspace değişiminde bir effect onu
  paylaşılan setten de siler → yeni açılan pencere mevcut rozetleri **devralır** ve
  baktığı workspace'i her yerde okundu işaretler.

**Playwright doğrulaması (2026-06-18):** 3 workspace (X/Y/Z), 2 pencere (aktif X, aktif Y).
Paylaşılan set `[X,Y,Z]` yazıldığında pencere-X `{Y,Z}`, pencere-Y `{X,Z}` gösterdi
(her biri kendi aktifini filtreledi) ✅. Pencere-X Z'ye geçince Z her iki pencereden
düştü ✅. Z'siz durumda açılan **yeni** pencere (aktif Y) `{X}` rozetini devraldı ✅.

## API Uçları

| Metod | Yol | Açıklama |
|-------|-----|----------|
| GET | `/api/workspaces` | Workspace listesi |
| POST | `/api/workspaces` | Yeni workspace oluştur |
| DELETE | `/api/workspaces/{id}` | Workspace sil (son workspace silinemez) |

Diğer tüm uçlar (`/api/agents`, `/api/chat`, `/api/runtime` ...) `X-Workspace-Id` header'ına göre çalışır.

## Test Sonucu (2026-06-15)

- WS1 "Varsayılan" → sadece Ajan-A görüyor ✅
- WS2 "İş" → sadece Ajan-B görüyor ✅
- Ayrı klasör + ayrı `store/` dizinleri ✅
- UI switcher ile geçiş: liste anında o workspace'e göre değişiyor ✅

### Çoklu pencere doğrulaması (2026-06-18, Playwright)

- İki sekme iki farklı workspace hash'iyle açıldı (`#/w/A/chat`, `#/w/B/chat`) → her
  biri kendi workspace adını + oturum listesini gösterdi ✅
- Tab B yüklenince paylaşılan `localStorage` B'ye döndü; **Tab A'ya geri dönüldüğünde
  hâlâ A workspace'inde** (hash A, ad "Varsayilan") → çapraz-sızıntı yok ✅
- Sonuç: pencere-başına workspace izolasyonu URL pinning ile uçtan uca çalışıyor.

## Workspace Ayarları (`ws-settings.json`)

Her workspace, ayarlar ekranındaki **"Genel"** kategorisinden düzenlenen kendi
override'larını `store/` yanındaki `ws-settings.json` dosyasında tutar
(`internal/workspace/settings.go` → `WSSettings`).

| Alan | Açıklama |
|------|----------|
| `icon`, `color` | Switcher/rail'de görsel kimlik |
| `defaultProvider`, `defaultModel` | Boş = uygulama varsayılanı |
| `pauseAutonomy` | Sadece bu workspace'in otonomisini (scheduler) durdurur — anahtar **Zamanlamalar** ekranının üstünde (2026-07-01: app-geneli pause kaldırıldı, pause artık yalnız workspace-özel) |
| `instructions` | **Bu workspace'teki tüm agent'lara eklenen serbest metin yönergeler** |

### Instructions enjeksiyonu

`instructions`, agent'ın **statik** system prompt önekine (`persona` → soul + identity'den
sonra) `# Workspace Instructions` başlığıyla eklenir. Statik prefix'te kaldığı için
prompt cache'i bozmaz.

- **İnteraktif chat:** `internal/api/chat.go` ve `chat_stream.go` → `ws(r).Settings().Instructions`
- **Otonom yollar** (schedule / task / flow / reflection): `internal/agent/runtime.go` →
  `Runtime.systemPrompt(agent)`. Runtime, değeri `SetInstructions` ile ayarlardan
  senkron tutar (ilk yükleme `loadSettings`, sonraki güncellemeler `UpdateSettings`).

## Notlar / Gelecek

- **Runtime izolasyonu:** Her workspace'in kendi agent runtime'ı var → bir workspace'in otonom ajanları diğerini etkilemez. Tüm workspace'lerin zamanlayıcıları paralel çalışır.
- **Silme koruması:** En az bir workspace her zaman kalır.
- **fs/shell artık kilitli DEĞİL (2026-06-22):** `workspace/` dizini eskiden built-in
  `Read`/`Write`/`Edit`/`LS`/`Glob`/`Grep`/`Bash` araçları için
  bir **güvenlik kilidiydi** (mutlak yol yasak, `..` kaçışı reddedilir). Bu kilit
  kullanıcı kararıyla **kaldırıldı**: artık bu araçlar makinedeki herhangi bir yolu
  okuyup yazabilir ve her yerde komut çalıştırabilir. `Sandbox.Root` yalnızca göreli
  yolların tabanı (varsayılan çalışma dizini) — bir sınır değil. Güvenlik halkası
  artık tek başına **izin modu** (salt-okunur / sor / otomatik). Tek istisna:
  **config araçları** (`read_config`/`write_config`/`list_config`) hâlâ
  `<workspace>/config/` içine **kilitli** (`Sandbox.Confined=true`, the external agent project benzeri
  ayrım). Kod: `internal/tools/sandbox.go` (`NewSandbox` kilitsiz / `NewConfinedSandbox`
  kilitli), kablolama `internal/agent/toolsetup.go`.
- **Oturum-başına çalışma dizini (2026-06-22):** `workspace/` artık yalnızca
  **varsayılan** çalışma dizini. Her oturum kendi `Session.WorkingDir`'ini
  belirleyebilir (Composer'daki klasör rozeti) → fs/shell o dizinden çalışır, ajan
  cwd'sini + git branch'ini bağlamda görür. Otonom turlar için `autonomousConfine`
  (varsayılan açık) bu dizine yeniden kilitler; `gitWorktreeIsolation` (kapalı) ise
  otonom oturuma `<workspace>/worktrees/<sessionID>` altında ayrı git worktree
  verir. Detay: `_Docs/26-CALISMA-DIZINI.md`.
- **Gelecek:** Workspace yeniden adlandırma, dışa/içe aktarma (export/import), workspace başına ayrı tema.
