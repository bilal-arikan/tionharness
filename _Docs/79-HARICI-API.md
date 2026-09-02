# Harici API — dışarıdan proje geliştirme

**Durum:** Kimlik doğrulama + eksik CRUD kapatma turu uygulandı (2026-09-02).

Bu doküman TionHarness'in HTTP API'sini **dışarıdan** (script, CI işi, başka bir
araç — paketlenmiş React arayüzü değil) sürmek isteyen tüketici için yazıldı.
Rota tablosunun kaynağı `internal/api/server.go` içindeki `Routes()` ve
`register*Routes` yardımcılarıdır; bu doküman onun yerine geçmez, **sözleşmeyi**
anlatır.

## 1. Kimlik doğrulama

Kapı **opt-in**'dir: yalnız `TIONHARNESS_API_AUTH_TOKEN` ayarlandığında devreye
girer. Değişken yoksa API, kapı eklenmeden önceki gibi davranır — **hangi adrese
bağlanmış olursa olsun**.

| Durum | Davranış |
|-------|----------|
| `TIONHARNESS_API_AUTH_TOKEN=<gizli>` | Her `/api/*` çağrısında bearer zorunlu — loopback dahil. |
| Ayarlı değil | Açık. Loopback dışı bağlanmada açılışta **uyarı loglanır**, istek reddedilmez. |

```bash
curl -H "Authorization: Bearer $TIONHARNESS_API_AUTH_TOKEN" http://127.0.0.1:8080/api/sessions
```

**Neden loopback dışı bağlanmada otomatik devreye girmiyor?** `scripts/dev.ps1`
**varsayılan olarak** `0.0.0.0` bağlar — telefon/ikinci makineden LAN üzerinden
test etmek belgelenmiş, gündelik bir akış — ve paketlenmiş arayüz hiçbir
`Authorization` başlığı göndermez (`frontend/src/api/client.ts`). Kapı orada
kendiliğinden kapansaydı, arayüzün karşılayamayacağı bir `401` ile normal
geliştirme döngüsü kırılırdı: **workspace'ler yüklenmez olurdu.** Yalnız tek
istemcinin sağlayamadığı bir koşulda ateşlenen auth, koruma değil hatadır.

Dolayısıyla durumun dürüst özeti: **operatör açıkça istemedikçe API kimliksizdir**
ve güvenilmeyen bir ağda `0.0.0.0` bağlanması açıktır (`dev.ps1` kendi başlığında
bunu zaten söyler). Bunu gerçekten kapatmak için **önce arayüzün token taşıması**
gerekir; o gelene kadar buradaki mekanizma, güvenilir LAN dışına dağıtan için
mevcuttur ve riskli şekil tespit edildiğinde açılışta uyarı verilir
(`internal/app/app.go`).

- Karşılaştırma sabit zamanlıdır (`crypto/subtle`) — token paylaşılan bir sırdır
  ve bayt bazında erken çıkış, ön ekini zamanlama ile sızdırır.
- Muaf tek yol `/health`'tir (canlılık probu token taşımaz). `/api/version`
  **muaf değildir**: derleme bilgisi keşif yüzeyidir.
- `OPTIONS` (CORS preflight) token istemez.

**Not:** `config.Config.AccessKey` alanı `ACCESS_KEY`'den okunur ama **hiçbir yerde
kullanılmaz** — kimlik doğrulamanın konulmak istendiği, hiç uygulanmamış yerdir.
Yeni kapı ondan bağımsız olarak `TIONHARNESS_API_AUTH_TOKEN` üzerinden çalışır;
`AccessKey` hâlâ ölü koddur.

## 2. Workspace kapsamı

Her istek bir workspace'e bağlanır (`withWorkspace`, `server.go`):

- Başlık: `X-Workspace-Id: <id>`
- Sorgu: `?ws=` / `?workspace=` / `?workspaceId=` / `?workspace_id=`

Fark önemlidir: **başlıktaki** bilinmeyen id sessizce varsayılana düşer (eski
localStorage değeri uygulamayı kilitlemesin diye), **sorgudaki** bilinmeyen id
`400` döner — açıkça kapsamlanmış bir isteği başka bir workspace'ten yanıtlamak,
çağırana yanlış veriyi yetkili gibi verir. Yanıt her zaman `X-Workspace-Id`
başlığıyla **hangi** workspace'in cevapladığını yankılar.

Hiç workspace yoksa workspace kapsamlı yollar `409` döner; bootstrap yüzeyi
(workspace CRUD, şablonlar, klasör seçici, `/health`, `/api/version`) çalışmaya
devam eder.

## 3. Workspace'i dışarıdan kurmak

Tümü başsız (headless) çalışır:

| İş | Uç nokta |
|----|----------|
| Workspace oluştur (+`projectDir`, +`gitInit`) | `POST /api/workspaces` |
| Var olan klasörü bağla | `POST /api/workspaces/attach` `{"path": "..."}` |
| Sağlayıcı tanımla | `PUT /api/providers` |
| Sağlayıcı API anahtarı | `POST /api/providers/{id}/auth/api-key` |
| Sır yaz | `POST /api/secrets` |
| Ajan tanımla | `POST /api/agents` |
| Ajan araç erişimi | `POST /api/agents/{id}/tools` |
| MCP sunucusu kaydet | `POST /api/mcp-servers` |
| Skill oluştur/içe aktar | `POST /api/skills`, `POST /api/skills/import` |
| Oturum çalışma dizini | `PUT /api/sessions/{id}/workdir` |

**`POST /api/pick-folder` yerel bir diyalogdur** (`powershell.exe -STA`,
WinForms) ve yalnızca bir yol *döndürür* — hiçbir yerde zorunlu değildir. Dışarıdan
çağıran yolu doğrudan JSON olarak verir.

**İki gerçek kısıt:**

1. **Yollar sunucu makinesinde çözülür** (mutlak olmalı, `os.Stat` ile
   doğrulanır). API uzak dosya sistemi sunmaz; uzak istemci sunucunun dizin
   düzenini bilmek zorundadır.
2. **Anthropic/claude-cli için başsız kimliklendirme yoktur.** `auth/api-key`
   yolu yalnız `codex-cli` içindir (`provider_auth.go`); Claude tarafı ya
   etkileşimli OAuth loopback'i ister ya da kimlik dosyalarının API dışından
   yazılmasını.

## 4. İş başlatmak ve izlemek

### Bloklayan (senkron) uçlar

- **`POST /api/chat`** — SSE gerekmez. Hub'a abone olur, mesajı kuyruğa alır ve
  kendi `clientMsgId`'sine ait `KindTurnDone` gelene kadar bloklar; kalıcılaşmış
  yanıtı JSON döner. Tur hatası `502`, başka pencereden iptal `409`.
- **`POST /api/flows/{id}/run`** — akışı satır içi koşturur ve **bittikten sonra**
  `{run, sessionId}` döner (düğüm başına durum `run` içindedir).
- **`POST /api/schedules/{id}/run`** — `RunNow`'u satır içi çağırır.

Üçü de `context.WithoutCancel` kullanır: istemci bağlantısı kopsa bile iş
tamamlanır ve kalıcılaşır. HTTP istemcisi zaman aşımına uğrarsa sonuç
`GET /api/flow-runs/{id}` ile geri okunur.

### Ateşle-unut uçlar + terminal sinyali

`POST /api/sessions/spawn` ve `POST /api/sessions/{id}/messages` hemen döner
(`{sessionId, queued, queuePosition}` / `{queued, clientMsgId}`). Bittiğini
anlamak için `GET /api/sessions/{id}/info`:

- **`terminal`** (bool) — yoklanacak alan budur.
- **`runState`** — `completed | failed | killed | timeout | incomplete`.
  `Session.State` ile **karıştırma**: o, görünürlük alanıdır (active/archived).
- `running` — tur uçuştayken doludur ve mutlak son-etkinlik damgası taşır, yani
  "çalışıyor" ile "takılmış" ayırt edilebilir.

Filo görünümü: `GET /api/executions` (oturum başına `lastStatus`).

### Akış (stream) tercihi

**`GET /api/workspace/stream`** imleç tabanlıdır ve **tekrar oynatılabilir**
(`since` + `epoch`; `hello` / `reset` / `hub` çerçeveleri). Bağlantı koparsa
boşluksuz devam eder. Dışarıdan tüketim için doğru akış budur.

**`GET /api/events`** imleçsizdir ve tekrar oynatmaz — kopan bağlantı olayları
kalıcı olarak kaybeder. Yalnız arayüz rozet/toast yolu içindir.

## 5. İş takibi (kanban)

Pano **görev çalıştırmaz**: dispatcher yoktur (`internal/db/models_task.go`).
Dışarıdan sürücü için akış şudur:

1. `GET /api/tasks?boardState=todo` — kartları oku (filtreler: `boardState`,
   `priority`, `ownerAgentId`, `tags`; sayfalama: `limit`/`offset`/`sort`).
2. `GET /api/tasks/{id}` — tek kartı geri oku.
3. `POST /api/sessions/spawn` — kartın `prompt`'u ile oturumu **sen** aç.
4. **`POST /api/tasks/{id}/sessions`** `{"sessionId": "SES..."}` — oturumu karta
   bağla.
5. `PUT /api/tasks/{id}` — `boardState`'i ilerlet.

**`Task.sessionIds`** sunucu sahipliğindedir: `UpdateTask` onu çağıranın
gövdesinden **kopyalamaz**, çünkü istemci en son okuduğunu PUT eder ve o sırada
eklenmiş bağlantıları sessizce düşürürdü. Tek yazar `LinkTaskSession`'dır ve
idempotenttir (aynı oturumu iki kez bağlamak no-op).

`LastRunID` / `LastRunStatus` / `LastRunAt` alanları **eskidir ve salt
okunurdur** — pano görev çalıştırmayı bıraktığından beri onları hiçbir şey
yazmaz. Canlı bağlantı için `sessionIds` kullan.

**Webhook yoktur.** Dışa doğru HTTP geri çağrısı hiçbir yerde kayıtlı değildir;
bildirim yalnız içeri bağlanan SSE ya da yoklamadır. Pano değişiklikleri artık
`ws:board` olarak **sıralı akışa da** yansıtılır (`internal/api/notify.go`), yani
kopan bir tüketici `since` ile boşluk doldurabilir.

## 6. Liste sözleşmesi (TSK68)

`limit` / `offset` / `sort` verildiğinde yanıt
`{items, total, offset, limit, hasMore}` zarfına girer; **hiçbiri verilmezse**
uç nokta eski davranışını korur (çıplak tam dizi). Bozuk değer `400`'dür, sessizce
yok sayılmaz.

Zarfı destekleyenler: agents, artifacts, automations, executions, flows,
schedules, sessions, tasks, workspaces.

Ayrı, dar sınırlayıcılar:

- `GET /api/sessions/{id}/messages?tail=N` — son N mesaj +
  `{items, offset, total, hasMore, truncated}`. Varsayılan **tam transkripttir**
  (arayüz ondan render eder); `tail` isteğe bağlı sınırlayıcıdır. Uzun bir
  oturumun tam transkripti megabaytlarca olabilir.
- `GET /api/lessons?limit=N` — en yeniden başlayarak sınırlar. Varsayılan 0
  (sınırsız), eski davranış.

Sınırsız kalanlar (bilinçli: sınırlı yapılandırma koleksiyonları):
`skills`, `hooks`, `mcp-servers`, `secrets`, `providers`.

## 7. Bilinen tutarsızlıklar

Kırıcı olduğu için **düzeltilmedi**, tüketicinin bilmesi gerekir:

- **`PUT /api/providers` yol parametresi almaz** — id gövdededir ve bilinmeyen id
  *oluşturur*. Yani yanlış yazılmış bir güncelleme, sessizce yeni bir sağlayıcı
  yaratır. Diğer tüm aileler id'yi yolda taşır.
- **`POST /api/market/registries/delete`** — silme için POST. Kalan tüm silmeler
  `DELETE` kullanır.
- **`DELETE /api/uploads` ve `DELETE /api/backups/archives`** hedefi gövdede
  taşır, yolda değil.
- **Arşivleme fiili üç türlü**: `PUT /api/artifacts/{id}/archive` (gövdeli),
  `POST /api/{tasks,automations,schedules,hooks}/{id}/archive`. `unarchive`
  yalnız tasks'ta ayrı uç noktadır.
- **Başlık üretme iki adla**: `.../title` (sessions, tasks) ve
  `.../generate-title` (schedules, automations).
- `PATCH /api/mcp-servers/{id}` artık `PUT` ile de çağrılabilir. **`PUT` doğru
  fiildir**: `UpdateMCPServer` her değiştirilebilir alanı atar, yani atlanan alan
  korunmaz, **temizlenir**. `PATCH` yalnız arayüz onu çağırdığı için kayıtlı
  kalmıştır.

## 8. Rota çakışması kuralı

Go 1.22 ServeMux'ta **düz segment, joker'i yener**. Depo id'leri ön ekli
üretildiği için (`SES`/`TSK`/`AUT`/`FLW`/`RUN`/`RTA`) oturum/görev/otomasyon
tarafında çakışma yapısal olarak imkânsızdır.

Tek gerçek risk **market paketi id'leriydi**: id manifestten gelir ve yalnız
boşluk kontrolü görürdü. `registries` id'li bir paket
`GET /api/market/registries` düz rotasının arkasında kalıcı olarak erişilemez
olurdu. `internal/market/store.go` artık `Publish` (ve dolayısıyla `Import`)
sırasında bunları reddeder: `registries`, `connectors`, `reload`, `publish`,
`import`. **Yeni bir düz `/api/market/<segment>` rotası eklersen
`reservedPackIDs` listesine de ekle.**

`POST /api/tasks/{id}/{subpath...}` bilinmeyen alt yolları `404`'e çevirir. Yeni
bir görev alt rotası eklerken **bu joker'den önce** kayıt olduğundan emin ol.
