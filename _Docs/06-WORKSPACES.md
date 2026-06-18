# SwarmGo — Workspace İzolasyonu

> Her workspace **tamamen bağımsızdır**: kendi dosya-tabanlı `store/` dizini + kendi agent runtime'ı. Bir workspace'in içeriği asla diğerine sızmaz.
> Depolama biçimi (JSON/JSONL) için: **`_Docs/08-DEPOLAMA.md`**.

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
├── workspaces.json              # [{id, name, createdAt}] kayıt defteri
├── credential-secret            # global şifreleme anahtarı
└── workspaces/
    ├── {id-1}/
    │   ├── store/               # Workspace 1'in TÜM verisi (JSON/JSONL dosyaları)
    │   ├── config/              # Editlenebilir config: prompts/*.md, instructions.md, README.md
    │   ├── ws-settings.json     # Workspace ayarları (override + instructions)
    │   └── workspace/           # Workspace 1'in görev dosyaları (ajan fs-sandbox kökü)
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
| `pauseAutonomy` | Sadece bu workspace'in otonomisini (heartbeat + scheduler) durdurur |
| `instructions` | **Bu workspace'teki tüm agent'lara eklenen serbest metin yönergeler** |

### Instructions enjeksiyonu

`instructions`, agent'ın **statik** system prompt önekine (`persona` → soul + identity'den
sonra) `# Workspace Instructions` başlığıyla eklenir. Statik prefix'te kaldığı için
prompt cache'i bozmaz.

- **İnteraktif chat:** `internal/api/chat.go` ve `chat_stream.go` → `ws(r).Settings().Instructions`
- **Otonom yollar** (heartbeat / task / flow / reflection): `internal/agent/runtime.go` →
  `Runtime.systemPrompt(agent)`. Runtime, değeri `SetInstructions` ile ayarlardan
  senkron tutar (ilk yükleme `loadSettings`, sonraki güncellemeler `UpdateSettings`).

## Notlar / Gelecek

- **Runtime izolasyonu:** Her workspace'in kendi agent runtime'ı var → bir workspace'in otonom ajanları diğerini etkilemez. Tüm workspace'lerin heartbeat'leri paralel çalışır.
- **Silme koruması:** En az bir workspace her zaman kalır.
- **Gelecek:** Workspace yeniden adlandırma, dışa/içe aktarma (export/import), workspace başına ayrı tema.
