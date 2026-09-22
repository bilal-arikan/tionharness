# 52 — Go-Native MCP Gateway Entegrasyonu

> **Özet (2026-09-03):** MCP gateway desenini (dinamik araç aktivasyonu, `list_changed`
> push) TionHarness'in kendi Interaction MCP sunucusuna kazandırma planı — **büyük ölçüde
> uygulanmış** (Faz 0-4 tamamlandı: config kararlılığı, stateful streaming server, dinamik
> extended yüzeyi, meta-araç konsolidasyonu, token ölçümü). Hibrit MCP kapsamı (`scope=
> shared|scoped`) zaten koddadır (`internal/mcp/pool.go`, `manager.go`). Kritik bulgu:
> persistent CLI oturumu + MCP birlikteyken eskiden her tur cold-restart oluyordu (config
> temp-yol churn'ü); wildcard allowlist + içerik-hash fingerprint + session-ömürlü token ile
> düzeltildi. Salt-okur yüzeyler (oturum bilgisi paneli) registry'yi
> `agent.WithCatalogNoDial` ile kurar → `Pool.CatalogCached` yalnız canlı bağlantıları
> okur, soğuk sunucu için dial beklemez (2026-09-06, `_Docs/05`). Soğuk sunucuyu tur
> ortasında ısıtmak için `wait_for_mcp_servers` aracı vardır (2026-09-21): sunucuları
> **paralel** dialler (`Pool.EnsureServers`), araçlarını canlı registry'ye `AppendMCP` ile
> ekler ve aktive eder → aynı turda çağrılabilir; CLI backend'lerinde kapalıdır. Tersi
> yön de kapalı: bir bağlantı tur ortasında **beklenmedik** biçimde ölürse
> `Pool.SetOnDisconnect` → `ws:mcp_status` olayı (UI kartı) + sunucu başına turda bir
> kez ajana sistem notu (2026-09-22, "Tur ortasında bağlantı kopması" bölümü). Temiz kapanışlar (reaper, `CloseSession`,
> config re-dial, `Pool.Close`) yapısal olarak sessizdir. MCP'nin **veri yarısı** da
> artık var: `list_mcp_resources` / `read_mcp_resource` (2026-09-22, §15) —
> `resources/list` + `resources/templates/list` + `resources/read`, sunucu başına
> izole hata, `initialize` yeteneklerine göre geçitli (`SupportsResources`), ikili
> içerik scratchpad'e dosya olarak (context'e base64 **girmez**), metin 64 KB'de
> açıkça kırpılır; yalnız native döngüde. Harici
> `/mcp/gateway` sunumu (Faz 3) de uygulandı (§12). Dayandığı
> dosyalar: `internal/interaction/server.go`, `internal/climcp/climcp.go`,
> `internal/providers/claudecli_session.go`.

> **UYGULANDI (2026-07-13) — Hibrit MCP kapsamı.** Havuz artık per-sunucu `scope`
> alanını gerçekten kullanıyor: `scope="shared"` (varsayılan) eski davranış — workspace
> geneli tek paylaşımlı bağlantı; `scope="scoped"` her `(session,agent)` için ayrı canlı
> bağlantı (havuz anahtarı `ServerConfig.ScopeKey`, `toolsetup.go` `SessionIDFrom(ctx)+"|"+agent.ID`
> ile damgalar; session yoksa shared'e düşer). Boşta kalan scoped bağlantılar `pool.go`
> reaper'ıyla kapatılır (`TIONHARNESS_MCP_SCOPED_IDLE_SEC`, vars. 300s; 0=kapalı); shared
> bağlantılar hiç reap edilmez. `ScopeKey` dial-fingerprint'ten hariç. Create API + Araçlar
> formunda "Bağlantı kapsamı" seçici. Ayrıca sunucu **düzenleme** (`PATCH /api/mcp-servers/{id}`,
> `DB.UpdateMCPServer` — kimlik/enabled korunur, spec değişince re-dial) ve canlı havuz
> gözlemi (`GET /api/mcp-servers/pool` → `Pool.Stats()`; UI'da per-sunucu 🔗 live/reaper
> rozeti, 5sn poll). Testler: `pool_scoped_test.go`. **Bu, aşağıdaki
> §11-A "token'ı per-(session,agent) yap" fikrinin transport-seviyesi karşılığıdır**;
> kimlik/token seviyesi de §11-A'da ayrıca uygulandı.
>
> İlgili: `11-INTERACTION-MCP.md`, `19-LAZY-TOOL-LOADING.md`.

---

## 0–10. Uygulama öncesi analiz ve faz planı (arşivde)

Yönetici özeti, doğrulanan mimari, Faz 0 spike bilinmeyenleri, mimari kritiği, faz
planı, riskler ve açık kararlar → `arsiv/52-MCP-GATEWAY-PLANLAMA.md`. Kararlar §11'de,
uygulama durumu §12'dedir. Kodda atıflı olan bulgular (§3-D, §3-E, §7-15) aşağıda
yerinde tutuldu.

### 3-D. YENİ BULGU — persistent+MCP bugün her tur cold-restart oluyor

> **Düzeltildi (2026-07-06, §12 Faz 0-b):** wildcard allowlist + içerik-hash fingerprint +
> session-ömürlü token. Aşağıdaki metin bulgu anındaki durumu anlatır; `writeCLIMCPConfig`
> bugün `internal/climcp` `WriteConfig`'tir.

`toolloop.go:253` her tur `writeCLIMCPConfig` çağırır → `os.CreateTemp(... "tionharness-mcp-*.json")`
**her tur YENİ yol** üretir, `defer cleanup()` **tur sonunda siler**. Sonra
`ConfigureMCP(path,...)` → `c.mcpConfigPath = <yeni yol>`. `persistentFingerprint` bu yolu
içerdiğinden → **her tur fingerprint değişir → cold restart**. Ek olarak persistent process
hâlâ o dosyaya referans verirken dosya siliniyor (Windows'ta açık handle sorunu).

> **Sonuç:** Bugün `ClaudePersistentSession` + `MCPEnabled` birlikteyken warm-reuse
> pratikte hiç gerçekleşmiyor — her tur tam prefix yeniden gönderiliyor. Bu **mevcut,
> muhtemelen fark edilmemiş bir regresyon** ve gateway işinin ilk düzeltmesi olmalı:
> config dosyasını **session-ömürlü, kararlı bir yola** yaz (temp churn yok), token'ı
> session-ömürlü yap, allowlist'i wildcard'a çevir. Ancak o zaman list_changed ile
> tur-içi büyütme cache'i bozmadan çalışabilir.

**Faz 0 çıktısı:** Q0/Q1/Q2/Q3 için evet/hayır + token sayıları tablosu; `52`'ye işlenir.

### 3-E. Faz 0 spike SONUÇLARI (2026-07-06, gerçek claude-cli 2.1.201 + claude-fable-5)

Harness: `_spikes/52-gateway/` — stateful streamable-HTTP Go MCP server (`server/main.go`)
GET SSE akışını açık tutuyor, `spike_grow` çağrılınca `spike_secret`'i `registerTool` edip
**`notifications/tools/list_changed`** push ediyor. claude tek `-p` turunda, allowlist
**yalnız `mcp__spike`** wildcard'ı ile sürüldü.

| Soru | Sonuç | Kanıt |
|---|---|---|
| **Q1** — mid-turn list_changed + AYNI turda çağrı | ✅ **EVET** | Tek turda: `spike_grow` → server push (`pushed list_changed`, `flushed`) → claude `spike_secret`'i çağırıp **`SECRET=GATEWAY_OK_42`** aldı. |
| **Q2** — wildcard allowlist sonradan gelen aracı kapsıyor | ✅ **EVET** | Allowlist spawn'da `mcp__spike` sabit; `spike_secret` yalnız grow'dan SONRA belirdi, yine de restart'sız çağrılabildi. |
| **Q3** — mid-turn list_changed cache prefix'i siliyor mu | ✅ **HAYIR (marjinal)** | list_changed sonrası model çağrıları `cacheRead≈29360–29570`, yalnız `cacheCreate 56–210` delta. Tam prefix rebuild YOK. Final: cacheRead 79456 > cacheCreate 63778. |
| **Q0** — persistent+MCP config kararsızlığı (§3-D) | ⚠️ **KOD-DOĞRULANDI, ampirik bekliyor** | `writeCLIMCPConfig` temp yol churn'ü + `defer cleanup` + fingerprint(mcpConfigPath) kesin. Ampirik teyit TionHarness'i çalıştırmayı gerektirir; düzeltme unit-testlenebilir (içerik-hash fingerprint). |

**Beklenmedik + kritik gözlem — ToolSearch aracılığı.** claude, spike araçlarını **inline
ETMEDİ**; her birini çağırmadan önce `ToolSearch select:mcp__spike__<tool>` ile şemasını
**on-demand yükledi**. Yani #40314 (HTTP-transport defer edilmez) claude 2.1.201'de artık
geçerli değil gibi görünüyor — HTTP MCP araçları da deferral'a giriyor. Bu, gateway
modelini claude-cli'nın kendi ToolSearch'iyle **uyumlu** kılar: extended yüzeyi başta
BOŞ/az ilan et → şemalar bağlama hiç girmez → list_changed ile büyüt → ToolSearch select
ile yüklenir → wildcard izin verir → aynı turda çağrılır. **name-only'nin yapamadığı
gerçek tasarruf budur.**

**Karar:** Faz 0 **YEŞİL** (Q1/Q2/Q3 olumlu, Q0 kod-net). Uygulamaya geçilir.

### 7-15. CLI projeksiyonu 2-durum (§7 madde 15)

Gateway gelince CLI yolunda `hidden` = "tools/list'te yok ama list_changed ile
çağrılabilir" = gerçek deferred-usable. summary/name-only CLI'da anlamsız kalır (yalnız
native). Son model: CLI projeksiyonu **2-durum** (core=alwaysLoad · extended=gateway-managed,
gerçekten token-free), native 4-tier kalır. Uygulaması §12 Faz 2.

---

## 11. ONAYLANAN KARARLAR (Bilal, 2026-07-06)

1. ✅ **Mimari: S2** (gateway desenini TionHarness içine kat). Faz 3 opsiyonel/sonraki.
2. ✅ **Kod yazımından önce Q0-Q3 spike** çalıştırıldı → §3-E: **YEŞİL**.
3. ✅ **§3-D düzeltmesi öneri gibi uygulanacak:** session-ömürlü kararlı config yolu,
   içerik-hash fingerprint, session-ömürlü token, wildcard allowlist.
4. ✅ **Extended tier → `mcp__tionharness_extended` wildcard** allowlist'ine geçecek.
5. ✅ **summary/name-only emekliye ayrılacak** — CLI projeksiyonu 2-durum (core /
   gateway-managed), native 4-tier korunur (bkz. §7-15).
6. ✅ **Interaction token → per-(session,agent)** (aşağıda detaylı sonuçlar).
7. ✅ **Faz 3 kapsamı: auth (loopback+token) + VPS zincir göçü dahil** (aşağıda detay).

### 11-A. Token: per-run → per-(session,agent) — ✅ UYGULANDI

**Sorun:** Interaction MCP Bearer token'ı bir *turn* ömürlüydü. Persistent CLI
process'inde token spawn'da sabitlenir ama process birçok turn'ü kapsar → sonraki
turn'lerde `byToken` ölü run'a çözer (401). Config churn bunu maskeliyordu.

**Çözüm:** token `(session,agent)` kimliğine bağlandı; `byToken` o anahtarın **aktif**
run'ına çözer (`chatRuns.active` + `bindActive`, `internal/api/chat_control.go`).
Kazanç: process ömrü boyunca tek kararlı token → mcp-config değişmez → fingerprint
değişmez → **warm-reuse korunur** (§3-D'nin diğer yarısı).

**Kalıcı ödünleşim (bilinmesi gereken):** token artık turn değil **session+agent
ömürlü** → sızma penceresi geniş. Azaltmalar: yüksek-entropi token, **loopback-only**
endpoint, process ölünce geçersiz kılma. Ayrıca iki turn arası boşlukta (aktif run
yokken) gelen çağrı **açıkça reddedilir** — sessiz kabul yok. Anahtar `session|agent`
olduğu için çapraz-agent token karışması yapısal olarak imkânsız.

### 11-B. Faz 3 — harici sunum: auth (loopback+token) + VPS zincir göçü DETAY

**Kapsam onaylandı** ama **opsiyonel/sonraki faz** (S2 iç-fix'ten bağımsız değer).

**Auth (loopback + token).**
- TionHarness API'sinde bugün **auth YOK + CORS wildcard** (§7-9). `/mcp/gateway` dış
  client'a açılırsa bu kabul edilemez.
- Model: gateway'in `SISTEM.md` kuralı — **default `127.0.0.1` (loopback)**; ağa/Tailscale'e
  açmak için explicit `0.0.0.0` + **`GATEWAY_AUTH_TOKEN` muadili ŞART**. Token setliyse
  `/health` hariç her istek `Authorization: Bearer <token>` ister.
- Interaction endpoint'in mevcut Bearer middleware'i (`bearer()`/`Valid`) **yeniden
  kullanılır** (YENİ-F) → ayrı auth yazma yükü düşük.

**VPS zincir göçü (gateway-of-gateways).**
- TS gateway bugün `vps-*` sunucuları streamable-http URL'li backend olarak zincirliyor
  (uzak makinede özel ağ üzerinden yayınlanan `/servers/{name}/mcp` uçları). TionHarness `internal/mcp` **zaten
  streamable-http backend destekliyor** → `vps-*` sadece URL'li MCP kaynağı olarak eklenir
  (§7-8). Zincirleme neredeyse bedava.
- Göç adımları: (a) TS `config.json`'daki 18 server + 7 vps girişini TionHarness MCP-server
  kayıtlarına aktar (transport/headers/env korunarak), (b) `autoActivate`/preset ↔ TionHarness
  tier/görünürlük eşle, (c) auth token + loopback default, (d) audit paritesi (gateway-audit.jsonl
  ↔ debug.jsonl), (e) uçtan uca doğrula (Craft/harici Claude Code → TionHarness `/mcp/gateway`
  → vps zincir → backend).
- **ROI notu:** TS gateway çalışıyor; göç faydası = tek Go binary + tek pool + TS runtime
  (Bun) bağımlılığının kalkması. İç CLI-fix (Faz 0-2) bundan **bağımsız** değerli; Faz 3
  ayrı tetiklenir.

**✅ Göç aracı (2026-07-06): `_spikes/52-gateway/migrate-vps.py`.** TS `config.json`'ı
TionHarness'in **mevcut** `POST /api/mcp-servers/import` endpoint'inin kabul ettiği standart
`{"mcpServers":{...}}` formatına dönüştürür (yeni endpoint gerekmedi). Dönüşümler:
`transportType`/`url` → `type:"http"`; `${VAR}` placeholder'ları `secrets.json`/env'den çözer;
`options.disabled` işaretlenir (entry yine yazılır, UI'dan kapatılır); gateway-only alanlar
düşürülür. Masked template üzerinde doğrulandı (18 server: stdio/http doğru çıkarıldı).

**Uygulama prosedürü (canlı, elle — secrets içerir):**
```bash
# 1) TS config'i normalize et (gerçek config.json + secrets.json ile)
python _spikes/52-gateway/migrate-vps.py \
  <projects>/mcp-server/config.json \
  --secrets <projects>/mcp-server/secrets.json > import.json
# 2) default workspace'e toplu import et
curl -X POST http://127.0.0.1:8090/api/mcp-servers/import \
  -H "X-Workspace-Id: <default-ws-id>" --data-binary @import.json
# 3) TS'de disabled olanları TionHarness UI'dan kapat (özet import.json summary'sinde işaretli)
# 4) harici client'ı TionHarness /mcp/gateway + TIONHARNESS_GATEWAY_AUTH_TOKEN'a yönelt
# 5) TS gateway-manager'ı durdur (emekli). vps-* URL'li girişler zaten çalışır (ServersFunc canlı okur).
```
> **Not:** vps-* girişleri gerçek `config.json`'da (template'te değil); script onları `url`
> ile http olarak çıkarır. Adım 2 secrets içerdiğinden **elle** çalıştırılır (bu oturumda
> uygulanmadı — canlı config/secrets'a dokunulmadı).

---

## 12. Uygulama durumu

### ✅ Faz 0-b — config kararlılığı (UYGULANDI, 2026-07-06)

§3-D'nin üç kaynağı da kapatıldı; `go test ./...` **693 passed**. Değişiklikler:

1. **Extended tier wildcard allowlist** — `internal/climcp/climcp.go`: extended tier artık
   per-tool (`mcp__tionharness_extended__<tool>` × N) yerine **tek sunucu-seviyesi wildcard
   `mcp__tionharness_extended`** (dış MCP'lerin `mcp__<key>` deseniyle aynı). Sonradan
   list_changed ile gelen araç zaten izinli + allowlist turn-arası sabit. Core tier
   per-tool kaldı (küçük/stabil). Test: `climcp_test.go` güncellendi.
2. **İçerik-hash fingerprint** — `internal/providers/claudecli_session.go`:
   `persistentFingerprint` artık `mcpConfigPath`/`settingsPath` **yollarını** değil dosya
   **içeriğini** hash'liyor (`hashFileContent`). Aynı içerik farklı temp yolda → aynı
   fingerprint → warm reuse. Test: `claudecli_fingerprint_test.go`.
3. **Stable per-(session,agent) token** — `internal/api/chat_control.go` +
   `chat_stream.go` + `autonomous_interaction.go`: Bearer token artık per-run uuid değil,
   **(session,agent) başına kararlı sır** (`interactionToken`); `byToken` bunu `active`
   map ile in-flight run'a çözer (per-run fallback korundu). Token turn-arası sabit →
   mcp-config byte-identical → fingerprint churn yok. Test: `interaction_token_test.go`.

**Birlikte etki:** wildcard (allowlist sabit) + stable token (config içeriği sabit) +
içerik-hash (yol churn'ü önemsiz) → persistent+MCP artık turn-arası **warm** kalır.
Ampirik warm-reuse oranı ölçümü Faz 4'e bırakıldı (çalışan TionHarness gerektirir).

> **Kalan minör:** `writeCLIMCPConfig` hâlâ her tur temp dosya yazıp `defer cleanup` ile
> siliyor (israf, correctness değil — claude config'i yalnız spawn'da okur). Faz 1'de
> session-ömürlü config dosyasına geçilebilir.

### ✅ Faz 1-a — stateful streaming server (UYGULANDI, 2026-07-06)

`interaction/server.go` POST-only stateless'ten **stateful + streaming**'e yükseltildi
(`go test ./...` **694 passed**). Spike server (`_spikes/52-gateway/server`) referans alındı.

- `initialize` → `capabilities.tools.listChanged:true` (eskiden `{}`).
- `GET` → gerçek server→client **SSE stream** açar + tutar (eskiden 405). Tool sonuçları
  hâlâ POST'ta inline; SSE yalnız bildirim (`notifications/tools/list_changed`) taşır.
- `Server.PushToolsChanged(token)` + `HasStream(token)` — push kanalı (non-blocking,
  stream yoksa güvenli no-op). `api.Server.interactionSrv` concrete alanı push'u erişilebilir kılar.
- `interaction.Handler` compat shim korundu. Test: `TestInteraction_GetStreamReceivesPush`,
  `TestInteraction_PushNoStreamIsNoop`. Doc 11 güncellendi.
- **Davranış:** araç yüzeyi DEĞİŞMEDİ (extended hâlâ tam set); yalnız push altyapısı kuruldu
  → güvenli/inert increment. Dinamik büyütme Faz 1-b'de.

### ✅ Faz 1-b — dinamik extended yüzeyi + tek activate semantiği (UYGULANDI, 2026-07-06)

extended `Tools(token,"extended")` artık (flag ON) **boş başlar**; model `activate_tools`
çağırınca backend aracı aktif sete ekler + `PushToolsChanged(token)` ile list_changed push
eder → claude re-list → ToolSearch select ile yükler → wildcard allowlist izin verir → aynı
turda çağırır (spike Q1/Q2 mekaniği). `go test ./...` **701 passed**, flag **default OFF**.

**Uygulanan parçalar:**
1. **Feature-flag `GatewayDynamicExtended`** (`gateway_tunable.go`, `tunables.go` alanı,
   default OFF) + boot env seed `TIONHARNESS_GATEWAY_DYNAMIC_EXTENDED` (`app.go`).
2. **Per-token aktif-extended durumu** — `interactionBackend.activated map[token]map[string]bool`
   + mutex; `Tools(token,"extended")` flag ON iken bunu filtreler (`isActivated`).
3. **Core meta-tools `activate_tools`/`deactivate_tools`** — yalnız flag ON iken ilan edilir
   (`interactionToolSpecs`), core (alwaysLoad) tier. `Call` → `callActivate` → aktif seti
   mutate + `srv.PushToolsChanged(token)`. Bilinmeyen ad → temiz hata (`extendedCandidates`
   ile doğrulama). Namespaced ad da kabul (`bareToolName` ile normalize).
4. **Pusher referansı** — `interactionBackend.setServer(srv)` (server construct sonrası,
   `api/server.go`); nil-safe (stream yoksa sonraki tools/list'te converge).
5. **Keşif nudge'ı** — `renderLazyToolCatalog` artık `gateway` parametresi alır; CLI+gateway
   modunda "yüklemek için `activate_tools` çağır (ToolSearch değil)" der. Harici MCP araçları
   hâlâ ToolSearch ile.
6. **YENİ-D sıralama** — activate push'u sonuç dönmeden ÖNCE queue'lanır (SSE flush arası).
7. **Yanlış-yükleyici yönlendirmesi (2026-08-28)** — iki yükleyici ayrı kalır, ama yanlış
   olanı seçen model artık **sessiz boş sonuç** yerine yön alır. `activate_tools`'a harici
   bir `mcp__<server>__…` adı verilirse cevap onu "unknown" kovasına atmaz; ayrı satırda
   `ToolSearch` + hazır `select:<ad>` sorgusunu yazar. Gateway `tool_search`'e `select:`
   önekli ya da TionHarness namespace'li bir sorgu gelirse sonucun başına — **eşleşme
   olmasa bile** — `activate_tools` notu eklenir. Ortak metin: `internal/tools/loader_guidance.go`.

**Testler:** `gateway_dynamic_test.go` (boş başlama, activate→görünür, deactivate,
bilinmeyen ad, namespaced ad, flag-OFF tam yüzey), `tooltier_test.go`
(`TestLazyCatalogGatewayFormUsesActivateTools`), `interaction/server_test.go` (push).

### ✅ Faz 1-b canlı validation (2026-07-06, gerçek claude-cli 2.1.201 + claude-fable-5)

Harness: `internal/interaction/live_gateway_test.go` (`TestLiveGatewayActivate`, gate
`TIONHARNESS_LIVE_CLI=1`). **GERÇEK `interaction.Server`** (fake dinamik backend) +
`writeCLIMCPConfig` birebir yapısı (core=alwaysLoad `/core`, extended `/extended`, extended
**wildcard `mcp__gwext`** allowlist) + gerçek `claude -p` turu.

**Sonuç: ✅ tek turda uçtan uca çalıştı.** claude-fable-5:
1. `activate_tools(tools=["gizmo_secret"])` → "activated: … now callable"
2. Server `tools/list_changed` push etti → `gizmo_secret` **extended (`gwext`)** bağlantısında belirdi
3. `mcp__gwext__gizmo_secret` çağrıldı → **`SECRET=GATEWAY_LIVE_OK_77`** alındı (num_turns=4, tek `-p`)
4. Cache: `cache_read=176137`, `cache_creation=9685` — büyük ölçüde warm.

**Validation'ın yakaladığı GERÇEK bug (düzeltildi):** core ve extended **iki ayrı MCP
bağlantısı** ama **tek token** paylaşıyor. `streams` map'i token'la anahtarlanınca bir GET
stream diğerini eziyordu → push yanlış bağlantıya gidip claude **extended'i hiç re-list
etmeyebilirdi**. Fix: `streams` artık **token+tier** ile anahtarlanır; `PushToolsChanged`
token'ın **tüm** stream'lerine broadcast eder (doğru tier re-list olur, diğeri ucuz no-op).
Model bunu doğruladı: "aktive edilen araç `activate_tools`'un olduğu `gwcore`'da değil
`gwext`'te yayınlandı" — **beklenen/doğru davranış** (activate_tools=core, aktive edilen=extended).

**KALAN (default-on ÖNCESİ):**
- **Tam-yol validation:** gerçek `api.interactionBackend` + canlı chatRun ile (fake backend
  yerine) — mekanik unit+live ile kanıtlandı; tam-yol Faz 4 ölçümüyle birlikte.
- permission_prompt yeni-aktive aracı ask modunda gate'liyor mu (YENİ-A).
- Öncesi/sonrası token ölçümü: boş-extended gerçekten name-only'den fazla kazandırıyor mu (Faz 4).

### ✅ Faz 2 — meta-araç konsolidasyonu + CLI 2-durum projeksiyonu (UYGULANDI, 2026-07-06)

**Birleşik meta-araç seti (gateway ↔ TionHarness eşlemesi):**

| Gateway (TS) | TionHarness karşılığı | Seviye | Nerede |
|---|---|---|---|
| `activate_tools` | `activate_tools` | session | core interaction (Faz 1-b, flag) |
| `deactivate_tools` | `deactivate_tools` | session | core interaction (Faz 1-b, flag) |
| `active_tools` | **`active_tools`** (YENİ, Faz 2) | session | core interaction (flag) |
| `list_servers` | `list_mcp_servers` | config | self-management (bridged extended) |
| `enable_server`/`disable_server` | `toggle_mcp_server` | config | self-management (bridged extended) |
| — | `create_mcp_server` | config | self-management (gateway'de yok, ekstra) |

Model tek tutarlı yüzey görür: **session-seviyesi** activate/deactivate/active (core, eager,
flag ON iken), **config-seviyesi** list/toggle/create MCP server (self-management). İki
`activate_tools` çakışması yok — CLI'da tek semantik (interaction meta-tool → aktif set +
`PushToolsChanged`); native path kendi registry `activate_tools`'unu kullanır (ayrı yol).

- **`active_tools`** — `callActiveTools`: session'ın aktive edilmiş extended araçlarını
  listeler (`activeExtended` sıralı snapshot). Test: `gateway_dynamic_test.go`.

**CLI 2-durum projeksiyonu (summary/name-only "emekliliği"):** `cliTier` **zaten**
summary ve name-only'yi tek `extended` durumuna indiriyordu (`full→core`,
`summary|name-only→extended`, `hidden→absent`) — yani CLI teli **zaten 2+1 durum**. Faz 1-b
katalog nudge'ı (activate_tools) bunu UX'te tamamladı: CLI'da model için tek anlamlı ayrım
**core (eager) vs extended (activate ile gelir)**. Native yol 4-tier'ı korur (token farkı
gerçek). **Kod değişikliği gerekmedi** — projeksiyon mevcut; Faz 2 bunu belgeliyor.

**Bilinçli DEFER (ayrı iş):** `hidden→deferred-usable` (native `tool_search`+activate
muadili CLI'da). Bugün `hidden→absent` (katalogda yok, `cliBridgeSkipHidden` ile köprülenmez).
Gateway'de hidden'ı da aktive edilebilir yapmak, CLI için **hidden araçları arayan bir
`tool_search` meta-tool** + hidden'ı köprüleme gerektirir → görünürlük postürünü değiştirir,
gateway default-on kararıyla (Faz 4) birlikte ele alınmalı.

### ✅ Faz 4 — token ölçümü (2026-07-06, gerçek claude-cli 2.1.201 + claude-fable-5)

Harness: `internal/interaction/measure_gateway_test.go` (`TestMeasureGatewaySavings`,
gate `TIONHARNESS_LIVE_CLI=1`). Gerçek `interaction.Server` + fake backend, extended tier
**30 gerçekçi self-management-tarzı araç** (ad+özet+küçük şema). Trivial, araç-gerektirmeyen
prompt (`"reply DONE"`) → tek fark: extended kaç araç ilan ediyor. Her senaryo benzersiz
marker ile (cache paylaşımı yok, `cacheRead=0` → temiz soğuk ölçüm).

| Extended tier | Soğuk prefix (input + cacheCreate) | input | cacheCreate |
|---|---|---|---|
| **FULL (30 araç)** | **52.269** | 3.977 | 48.292 |
| **EMPTY (gateway)** | **46.373** | 3.509 | 42.864 |
| **TASARRUF** | **5.896 token** | — | — |

**Sonuç — gateway'in ana tezi DOĞRULANDI.** 30 "deferred" extended araç ilan etmek CLI'da
**yine de ~5.9K token** yiyor (≈197 token/araç) — çünkü claude bunları %10 eşiği altında
**inline ediyor** (brief §1'in tam bulgusu: name-only/deferral CLI'da kazandırmıyor).
Gateway'in **boş-başlaması** bu maliyeti araç gerçekten aktive edilene kadar **tamamen
ortadan kaldırıyor**. Gerçek extended yüzeyi (self-management + name-only, ~30-50 araç,
daha zengin şemalar) için tasarruf muhtemelen **daha yüksek** (~6-12K token/tur, soğuk prefix).

**Model/pay-for-use:** gateway "kullandığın kadar öde" — çoğu turda extended kullanılmaz →
büyük tasarruf; bir araç aktive edilince maliyeti o an yayılır. Persistent+warm (Faz 0-b) ile
prefix bir kez yazılır, sonra `cacheRead` ile okunur → küçük prefix warm turda da ucuz.

**DEFAULT-ON KARARI (Bilal onayına):** Sinyal güçlü ve tek yönlü pozitif; mekanizma
unit+canlı+ölçüm ile kanıtlı; risk flag'le izole. Ön koşullar:
- ✅ **permission_prompt (YENİ-A):** namespaced araç bare risk'iyle gate'lenir (`callPermission`
  namespace'i soyar; `TestPermissionPromptStripsNamespace`). Kapatıldı.
- ⏳ **Tam-yol canlı tur** (gerçek `api.interactionBackend` + canlı chatRun): mekanizma
  `TestLiveGatewayActivate` (gerçek `interaction.Server` + fake backend) ile kanıtlı; tam
  runtime turu hâlâ manuel/QA adımı.

**✅ KARAR (Bilal, 2026-07-06): flag KALDIRILDI — dinamik extended TEK davranış.** "Geri
dönük uyum yok, temiz kurulum" onayıyla `GatewayDynamicExtended` tunable + env seed + OFF-yolu
**tamamen silindi** (`gateway_tunable.go` silindi, `tunables.go`/`app.go` temizlendi,
`mcp_interaction.go` koşulsuzlaştı, `toolsetup.go` CLI katalogu daima activate_tools formu).
Extended tier artık her CLI turunda boş başlar ve activate_tools ile büyür. Obsolete testler
(`...OffIsFullSurface`, `...GatewayFormUsesActivateTools`) kaldırıldı; `TestInteractionTierSplit`
sınıflandırma-partisyonuna güncellendi. `go test ./...` **705 passed**.

### ✅ Faz 3 — harici `/mcp/gateway` endpoint (UYGULANDI, 2026-07-06)

TionHarness artık MCP havuzunu **dış client'lara** (harici Claude Code / External Agent) tek
endpoint arkasında sunabiliyor — TS `gateway-manager`'ın Go-native muadili. **Opt-in**
(`TIONHARNESS_GATEWAY_EXTERNAL=1`), `go test ./...` **706 passed**.

**Yeni paket `internal/gateway`** (interaction'dan AYRI — auth ve session modeli farklı):
- `server.go` — streaming MCP-over-HTTP: `initialize`'da **session id MİNTLER** (bearer'dan
  türetmez), `listChanged:true`, GET SSE per-session, `PushToolsChanged`. Auth **ayrı**
  bearer kontrolü (dış client'lar tek paylaşılan token, her bağlantı izole session).
- `backend.go` — `mcp.Pool` üzerinde meta-araçlar: **`list_servers` / `activate_tools` /
  `deactivate_tools` / `active_tools`**. `activate_tools(["docker"])` → `pool.Catalog` ile
  bağlan + araçları namespaced (`server__tool`) kaydet + `list_changed` push. Proxied çağrı
  → `pool.Call`. Servers **canlı** okunur (`ServersFunc`, config değişince restart yok).

**API entegrasyonu** (`api/server.go`): opt-in iken dedicated `mcp.Pool` + default
workspace'in enabled MCP server'ları (`workspaces.Default().DB.ListEnabledMCPServers` →
`agent.ToServerConfig`) + mount `/mcp/gateway` (+subtree).

**Güvenlik (§7-9, §11-B):**
- **Token yoksa → loopback-only** (`loopbackGuard`: non-loopback RemoteAddr → 403). Ağa
  açmak için **`TIONHARNESS_GATEWAY_AUTH_TOKEN` ŞART** (set edilince bearer zorunlu, guard pass-through).
- Default **KAPALI** — harici sunum explicit tercih.
- Test: `TestLoopbackGuard`, `TestIsLoopbackAddr` (IPv4+IPv6 loopback), `TestGatewayAuth`
  (401 token yok/yanlış, 200+session-id doğru).

**VPS zincir göçü (§11-B):** TionHarness `internal/mcp` zaten streamable-http backend
destekliyor → `vps-*` sunucular yalnız **URL'li MCP server satırları** (transport `http`,
`headers` ile auth). Göç = TS `config.json`'daki 18+7 server'ı default workspace'e MCP
server olarak ekle (`create_mcp_server`/UI/API) → dış client'ı `/mcp/gateway` + token'a
yönelt. `ServersFunc` canlı okuduğu için server ekleme **restart gerektirmez**. Böylece
gateway-of-gateways (yerel → VPS zincir) neredeyse bedava.

**Testler:** `internal/gateway/gateway_test.go` — `TestGatewayActivateAndProxy` (gerçek
`mcp.Pool` + fake backend MCP: meta-only → activate → namespaced araç görünür → proxied
`echo` round-trip → deactivate), `TestGatewayAuth`.

**✅ Canlı dış-client validation (2026-07-06):** `internal/gateway/live_gateway_test.go`
(`TestLiveExternalGateway`, gate `TIONHARNESS_LIVE_CLI=1`). Gerçek claude-cli, token'lı
`/mcp/gateway`'e (gerçek `gateway.Server` + gerçek `mcp.Pool` + fake backend MCP) bağlandı;
**tek turda** `activate_tools(servers=["fake"])` → gateway pool ile backend'e bağlandı +
`list_changed` push → claude re-list → proxied `get_secret` → **`SECRET=GW_EXT_OK_88`**.
Tam zincir **claude → gateway → pool → backend MCP** doğrulandı (num_turns=3, tek `-p`).

**KALAN (Faz 3 tamamlama):**
- ✅ **Pool yaşam döngüsü:** `api.Server.Close()` eklendi (dedicated gateway pool'u kapatır,
  stdio child'ları orphan bırakmaz); `App.Shutdown` çağırır. Ref-count paylaşımı (iç ajan +
  dış client aynı backend, #11) — hâlâ follow-up.
- **Workspace seçimi:** MVP default workspace'e bağlı; header/token→workspace eşlemesi follow-up.

### ✅ Follow-up'lar (2026-07-06)

- ✅ **Ref-count paylaşımı (#11):** harici gateway dedicated pool yerine default workspace'in
  pool'unu paylaşır (`Runtime.MCPPool()` + `PoolFunc` canlı çözüm) → iç ajan + dış client aynı
  backend bağlantısını kullanır. `api.Server.Close()`/`gatewayPool` kaldırıldı.
- ✅ **hidden→deferred-usable + `tool_search`:** gateway advertise-on-demand olduğundan hidden
  köprüleme sıfır token → `cliBridgeSkipHidden` POC **kaldırıldı** (`skipHidden=false`). Yeni
  core `tool_search` meta-tool katalogda görünmeyen hidden dahil tüm aktive-edilebilir araçları
  arar; `Tools("extended")` aktive edilmiş non-core (extended+hidden) ilan eder. CLI'da native
  hidden-tier'ın tam muadili. Test: `TestGatewayHiddenActivatableAndToolSearch`.
- ✅ **`tool_search` render'ı tek kaynak (2026-08-28):** iki elle-senkron kopya
  (`internal/tools/builtin_activate.go` + `callToolSearch`) tek saf yardımcıya indi:
  `tools.RenderToolSearch` (`internal/tools/toolsearchrender.go`). Gerçek yol farkları
  kod kopyası değil **seçenek**: başlık metni, gateway'in 100 baytlık açıklama kırpması,
  taşma notundaki boş demet listesinin basılıp basılmayacağı. `BundleOf` nil ise panic —
  satırların sessizce etiketsiz render edilmesi bu yardımcının önlediği hatanın ta kendisi.
  Refactor öncesi yakalanmış golden'lar iki yolu da bayt bayt sabitler
  (`internal/tools/testdata/toolsearch_native_*.txt`,
  `internal/api/testdata/toolsearch_gateway_*.txt`).
- ✅ **Demet (bundle) listeleme render'ı tek kaynak (2026-08-28):** aynı desen bu
  yüzeye de uygulandı — `ActivateToolsTool.openBundles` ve
  `interactionBackend.listBundles` içindeki kopya render `tools.RenderBundleList`
  yardımcısına indi (`internal/tools/bundlelistrender.go`), `bundleListLimit` +
  `gatewayBundleListLimit` tek `tools.BundleListLimit` sabitine düştü. Gerçek yol
  farkları seçenek olarak taşınır: başlık metni (native "Opened … no schema
  loaded" — durum tutar; gateway "… nothing was activated" — tutmaz), taşma notu
  metni ve **basılan ad biçimi** (native bare ad, gateway `extendedNSPrefix`'li
  çağrılabilir ad). `NameOf` nil ise panic — CLI'a çağrılamaz bir ad basmak
  yardımcının önlediği hatanın ta kendisi. Bilinmeyen anahtar raporu paylaşılmadı:
  iki yol farklı yerde ve farklı sözcüklerle raporlar. Durum yönetimi
  (`ActiveTools.OpenBundle`/`CloseBundle`) kapsam dışı bırakıldı; yardımcı saf.
  Karakterizasyon golden'ları refactor öncesi yakalandı
  (`internal/tools/bundlelistrender_test.go`,
  `internal/api/gateway_bundlelist_test.go`).
- ✅ **Aktivasyon SONUÇ metni — birleştirilmedi, kilitlendi (2026-08-28):** aynı
  disiplin üçüncü yüzeye uygulandı ama sonuç ters çıktı: `activate_tools` /
  `deactivate_tools` / `active_tools` sonuç metinlerinde native ile gateway
  arasında **kopya yok**. İki yol tek bir bayt bile paylaşmıyor ve bölüm dilbilgisi
  de örtüşmüyor — native adları açıklamalarıyla satır satır basar
  (`Activated %d tool(s)…` + `- ad — açıklama`), gateway adları tek satırda
  namespace'li ve virgülle basar (`activated: mcp__tionharness_extended__…` +
  `Call each by this exact (namespaced) name.`); native'in `Already active: %s`
  bölümü adları sayar, gateway'in `no new tools activated (already active or none
  valid)` karşılığı saymaz; native'de her bölüm `\n` ile biter ve sonuç
  `TrimSpace`'lenir, gateway'de tutkal bölüm başına değişir (`\n`, `; `) ve trim
  yoktur. Native'de always-on ve açık-demet notları vardır, gateway'de karşılığı
  yoktur; gateway'de push notu ve `IsError` bayrağı vardır, native'de yoktur. Geriye
  ortak olarak yalnız `strings.Join` kalıyor — onu bir yardımcıya sarmak her yolun
  metnini üretildiği yerden koparıp ikinci bir dosyaya dağıtacağı için drift riskini
  **artırır**. Bunun yerine iki sözleşme bağımsız birer karakterizasyon testiyle
  bayt bayt sabitlendi: `internal/tools/builtin_activate_result_test.go`,
  `internal/api/gateway_activate_result_test.go`. Böylece `tool_search` ve demet
  listeleme birleştirmeleriyle açılan "yarım-port render kopyası" sınıfı **kapandı**.

### ✅ VPS göç aracı (2026-07-06)

`_spikes/52-gateway/migrate-vps.py` — TS `config.json` → `/api/mcp-servers/import` formatı.
Prosedür §11-B'de. Masked template'te doğrulandı (18 server). Canlı import elle (secrets).

### Tam-runtime QA — durum + manuel checklist

**Otomatik kapsam (yeterli kanıt):** mekanizma uçtan uca kanıtlı — `TestLiveGatewayActivate`
(gerçek `interaction.Server` + gerçek claude, aynı-tur activate→çağrı), `TestLiveExternalGateway`
(gerçek `gateway.Server`+pool+claude), `TestMeasureGatewaySavings` (token), ve `interactionBackend`
mantığı gerçek `chatRun`'larla unit-testli (`gateway_dynamic_test.go`: activate/deactivate/
active/tool_search/hidden). Tam app boot + canlı chat testi orantısız ağır/kırılgan olacağından
(her CI'da token harcar) yazılmadı; onun yerine **manuel QA checklist** (Bilal, canlı TionHarness):

1. Boot: izole `TIONHARNESS_DATA_DIR` + ayrı port ile `tionharness` başlat; claude-cli agent oluştur
   (`MCPEnabled`, persistent session açık), birkaç extended/hidden görünürlüklü araç ayarla.
2. Bir chat turu at: modelden bir extended aracı (ör. `notify`) veya hidden aracı **kullanmasını**
   iste. Beklenen: model `activate_tools` (ya da hidden için önce `tool_search`) çağırır → araç
   aynı turda çalışır. UI/Logs'ta `tools/list_changed` push + tool çağrısı görünür.
3. Ask modda: activate edilen mutating bir araç çağrılınca **permission kartı** çıkar (YENİ-A —
   bare risk sınıfıyla). Read-only araç sormadan geçer.
4. Persistent warm: 2. turda config değişmediyse `cold start` **yok** (Logs'ta warm-reuse);
   `debug.jsonl`'de `cache_read > 0`.
5. Token: aynı senaryoyu ölç (öncesi=full-extended yoktu; şimdi boş-extended) → taze input düşük.

> Bu checklist geçerse gateway üretimde tam doğrulanmış sayılır; mekanizma zaten otomatik kanıtlı.

### ✅ İleri opsiyoneller (2026-07-06)

- ✅ **Per-workspace routing:** harici client `X-Workspace-Id` header'ıyla workspace seçer
  (`gateway.WorkspaceHeader`); server `initialize`'da `OpenSession(sid, wsID)` ile session'ı
  workspace'e bağlar; `ServersFunc`/`PoolFunc` artık `workspaceID` alır (boş/bilinmeyen →
  default). Böylece tek gateway çok workspace sunar. Test: `TestGatewayPerWorkspaceRouting`.
- ✅ **Audit paritesi:** proxied backend tool çağrıları `AuditEntry{ts,server,tool,ok,error}`
  ile kaydedilir (TS `gateway-audit.jsonl` muadili). Opt-in `TIONHARNESS_GATEWAY_AUDIT_LOG=<path>`
  (JSONL append, fire-and-forget; meta-araçlar denetlenmez). Test: `TestGatewayAuditRecordsProxiedCalls`.
- ✅ **Canlı VPS göç uygulaması — YAPILDI (2026-07-06):** kullanıcı onayıyla (tüm workspace'ler,
  23 server, çakışanların üstüne). Masaüstü app kapalıyken `cmd/tionharness` geçici olarak
  gerçek data-dir'e karşı ayrı bir portta başlatıldı; `apply-migration.py`
  (delete-colliding + import) 4 workspace'e uygulandı → **WS1: 2 overwrite +23, WS5: 1
  overwrite +23 (non-colliding `codebase-memory` korundu → 24), WS8: +23, WS9: +23**;
  0 hata, **dupe yok** (doğrulandı). Geçici server durduruldu, secret taşıyan ara çıktı +
  binary silindi. Sonraki masaüstü açılışında WS1/5/8/9'da 23 server hazır. Araçlar:
  `_spikes/52-gateway/migrate-vps.py` + `apply-migration.py`.
  > **Ek düzeltme (2026-07-06):** Import her server'ı **enabled** oluşturduğundan (import
  > endpoint'i `disabled` alanı taşımıyor), TS'de disabled olan 13 server TionHarness'te açık
  > geldi → app açılışta backend'i çalışmayanlara eager dial → `dial failed`/`context canceled`
  > log spam'i. Çözüm: `migrate-vps.py` artık çıktıya `_disabled: [...]` ekler; `apply-migration.py`
  > import sonrası bunları `toggle {enabled:false}` ile kapatır. Canlıda 4 workspace'te 13'er
  > server disable edildi → enabled set TS ile eşleşti (10 server). Kalan enabled http backend'leri
  > (yerel ve `vps-*` backend'ler) çalışmadıkça hâlâ warn verebilir — bu TS'nin enabled setiyle aynı.

- ⛔ **VPS göçü GERİ ALINDI (2026-07-06):** Bilal netleştirdi — asıl istek dış `mcp-server`
  gateway'inin server'larını TionHarness'e **import etmek değildi**; istek, TionHarness'in *kendi*
  built-in tool'larını + kullanıcının TionHarness'e **kendi eklediği** harici MCP'leri gateway-benzeri
  yüzeyle yönetmesiydi (bu zaten `internal/gateway` + iki-katmanlı interaction ile mevcut).
  Dolayısıyla göç bir yanlış-anlama ürünüydü. **Temizlik:** uzak gateway'e işaret eden
  **tüm `vps-*` server'lar** 4 workspace'ten silindi (WS1:1, WS5/8/9:7'şer
  = 22 toplam, 0 leftover doğrulandı). Kullanıcının gerçek local tool'ları korundu
  (hepsi stdio/local). `migrate-vps.py`/`apply-migration.py` araçları
  `_spikes`'te referans olarak duruyor ama **canlıya artık uygulanmıyor**. Not: import'tan kalan
  config-only **disabled** stdio server'lar da kullanıcının isteğiyle silindi (**32 disabled server**,
  4 workspace). Nihai temiz durum — yalnız kullanıcının gerçek tool'ları:
  **WS1:** 5 server · **WS5:** 8 server · **WS8/WS9:** 7'şer server.
  Gateway mekanizmasının kendisi (asıl hedef) değişmedi.

- 🩹 **activate_tools/active_tools çıktısı namespaced ad döndürüyor (2026-07-06):** Semptom
  (SES125): ajan `activate_tools({tools:['list_agents']})` çağırıyor, çıktı **bare** `"activated:
  list_agents"` diyor; ama claude-cli'de deferred tool YALNIZ namespaced adla çağrılabilir
  (`mcp__tionharness_extended__list_agents`). Model çıplak `list_agents` çağırıp `"No such tool
  available: list_agents"` alıyor, sonra doğru adla yeniden deneyip başarıyor — boş round-trip
  (+ autotag fix'inden önce sahte `tool-error`). Fix (`mcp_interaction.go`): `callActivate` +
  `callActiveTools` artık **namespaced çağrılabilir adı** raporluyor (`extendedNSPrefix` sabiti) +
  "Call each by this exact (namespaced) name." Test: `TestGatewayActivateAcceptsNamespacedName`
  namespaced çıktıyı da assert ediyor. İlgili: `PowerShell`/`list_agents` çıplak-ad reddi artık
  `tool-error` almıyor (`internal/agent/autotag.go` `permissionDenyMarkers`, bkz. _Docs/46).

- 🩹 **Ad-alanı öneki idempotent (2026-08-24):** Gateway/CLI katalog adları tek
  kaynak olan `mcp.NamespaceTool` ile üretilir. Girdi istenen `<server>__` veya
  CLI'ın `mcp__` önekini zaten taşıyorsa fonksiyon adı değiştirmeden döndürür;
  yeniden işleme çift `mcp__` öneki oluşturmaz. Ayrı
  `NamespaceToolIdempotent` yardımcı fonksiyonu kaldırılmıştır.

- 🩹 **recent_tool_activity recap tam (namespaced) adı gösteriyor (2026-07-06):** Bilal'in tespiti —
  bare-name alışkanlığının asıl kaynağı buymuş. `traceStepToTurnStep` (`internal/agent/trace.go`)
  CLI tool adından namespace'i soyup **bare** saklıyor (UI'da temiz kart için doğru). Ama bu bare ad
  `<recent_tool_activity>` recap'ine de gidiyordu → model bir sonraki turda "list_tasks kullandım"
  görüp **bare** çağırıyor → CLI reddediyor. Fix: `TurnStep`'e `CallName` alanı eklendi — soyulduğunda
  orijinal namespaced ad orada saklanır (native/bare tool'larda boş). Recap (`chat_tool_summary.go`
  `formatToolRecapLine`) artık `CallName` varsa onu gösteriyor → model gerçek çağrılabilir adı görüyor.
  UI hâlâ bare `Tool`'u kullanıyor (kartlar değişmedi). Test: `TestToolRecapBlockUsesCallName`.
  Ayrıca **activate→call race** (SES125 `list_tasks`): model doğru namespaced adı çağırsa bile
  `activate_tools` sonrası CLI `tools/list`'i henüz yenilememişse "No such tool available" gelir.

- ✅ **activate→call race ÇÖZÜLDÜ — `PushToolsChangedAndWait` (2026-07-08):** Önce canlı ölçtük
  (`internal/interaction/probe_relist_test.go`, `TIONHARNESS_LIVE_CLI=1`): activate cevabı tutulurken
  CLI'nin `tools/list(extended)` yeniden-çekmesi gelip gelmediğini gözledik. Sonuç claude-cli 2.1.203
  / fable-5'te **2/2 "A"** — CLI bildirimi **eşzamanlı** işliyor, activate PENDING iken ~10-16ms'de
  re-list ediyor (hold sırasında race hiç tetiklenmedi, 0/2). Yani bloklama güvenli+etkili.
  Uygulama: `interaction.Server`'a `relistWaiters` + `PushToolsChangedAndWait(token, timeout)` +
  `signalRelist` (tools/list handler sinyaller); `callActivate` artık `PushToolsChanged` yerine bunu
  çağırıyor (`activateRelistTimeout = 1s`, normalde ~15ms'de döner). Güvenlik: açık stream yoksa hiç
  bloklamaz, her zaman timeout'lu → client re-list etmezse en fazla +1s, sonra tarihsel retry.
  Testler: `TestProbeRelistOrdering` (canlı, gated), `TestPushToolsChangedAndWait{Signalled,NoStream,Timeout}`.

- ✅ **Dosya-yazan MCP → oturum scratchpad izinli kökü (Playwright, 2026-08-04):** Playwright MCP
  (`browser_take_screenshot` / PDF), dosya yazımını **izinli köklerine** — bu client hiç MCP root
  ilan etmediği için de yalnız **cwd**'sine — kısıtlar. TionHarness ise ajana çıktı yolu olarak oturum
  scratchpad'ini (`<store>/sessions/<SID>/scratchpad`) veriyordu; iki küme kesişmediği için her
  dosya-yazan çağrı `File access denied: outside allowed roots` ile reddediliyordu (ek olarak paylaşılan
  havuz bağlantısı bayat bir oturum kimliğine çözülüyordu — SES4'te SES1 scratchpad'i). **Fix:**
  `mcp.ServerConfig`'e `Dir` alanı (stdio alt-sürecin cwd'si; `DialStdio` `cmd.Dir`'e yazar) +
  `internal/agent/mcp_playwright.go` (`applyMCPScratchpadRoot`): dosya-yazan sunucu (bugün Playwright,
  `command`+`args`'ta "playwright") için scratchpad **her build'de aktif oturumdan yeniden çözülür**
  (`sessionScratchpad`), `cfg.Dir` = scratchpad + `--output-dir=<scratchpad>` (operatör verdiyse
  saygı) + sunucu (session,agent) başına **scope**'lanır. `Dir` bağlantı parmak izinde olduğundan yeni
  oturum bayat kök yerine yeniden dial eder. Çözülemezse (canlı oturumda) sessiz yutmaz — uyarı loglar.
  Çağrı `internal/agent/toolsetup.go` sunucu döngüsünde tek satır (`applyMCPScratchpadRoot`).
  Testler: `TestIsFileWritingMCP`, `TestEnsureOutputDirArg`.

- ✅ **CLI sağlayıcılarında ajan araç kısıtı → MCP sunucu kapısı (2026-08-21):** `claude-cli` ve
  `codex-cli` harici MCP sunucularını **CLI sürecine** mount eder; araç döngüsünü CLI kendi koşturduğu
  için ne ilan edilen katalog ne de çağrı dispatch'i `toolFilter`'dan geçer. Canlı yakalandı (SES948):
  allowlist'i `["Read","LS","Glob","Grep","Write","Edit","Bash"]` olan `worker:coder` ajanı bir tur
  boyunca Playwright ve codebase-memory sunucularını sürdü — araçlar sistem promptunda **hiç geçmiyordu**
  (yani TionHarness filtresi doğru çalışmıştı), ama `writeCLIMCPConfig`/`codexMCPSpec` **enabled olan her
  sunucuyu** mount edip `mcp__<key>` sunucu-düzeyi joker'i ile toptan izinliyordu. Ajan araç ekranı bu
  süre boyunca aynı araçları "blocked" gösteriyordu. **Fix:** `internal/agent/mcpservergate.go`
  (`mcpServerGate`) — ajanın blocked/allowed desenlerinden **önek tabanlı** bir sunucu kapısı üretir ve
  iki CLI yolu da sunucuyu mount etmeden önce ona sorar (`ag db.Agent` parametresi eklendi).
  - Neden önek tabanlı: CLI yolunda sunucuya bağlanmadan araç listesi yok; bağlanmak her stdio
    sunucunun **ikinci** bir kopyasını TionHarness sürecinde açardı. Namespaced araç adı daima
    `<key>__<tool>` olduğundan sunucu kararı yalnız desenlerden verilebilir.
  - Blocked: sunucunun tamamını kapsayan desen (`playwright*`, `playwright__*`, çıplak `playwright`)
    sunucuyu düşürür; **tek bir aracı** bloklamak düşürmez (diğer araçlar meşru kalır).
  - Allowlist (legacy, boş değilse): sunucu ancak bir desen onu hedeflerse hayatta kalır — tam ad
    (`playwright__browser_click`), önek veya çıplak key. `group:<kategori>` anahtarları **built-in**
    sınıflandırmasıdır, hiçbir MCP aracıyla eşleşmez → sadece built-in içeren allowlist tüm sunucuları
    düşürür (native yolun davranışıyla birebir aynı).
  - Sınır: sunucu içi **araç-başına** hassasiyet hâlâ CLI'ın kendi izin katmanındadır. codex'te
    `--disallowedTools` karşılığı olmadığından **sunucuyu hiç mount etmemek oradaki tek yaptırımdır**.
  - Testler: `internal/agent/mcpservergate_test.go` (SES948 regresyonu, allowlist'in adlandırdığı
    sunucu kalır, blocked desen matrisi, kısıtsız ajan her şeyi mount eder, `group:` anahtarları
    yok sayılır).

  **Bozuk izin belgesi → fail-closed (2026-09-01).** `mcpServerGate` imzası artık
  `(func(string) bool, error)`. **Boş** `AllowedTools` hâlâ "kısıt yok" demektir (nil kapı,
  davranış değişmedi); ama `AllowedTools` / `ToolOverrides` / `BlockedTools` **çözülemezse**
  fonksiyon **deny-all** kapı + hata döner. Eskiden `json.Unmarshal` hatası `_ =` ile
  yutuluyordu → iki liste de boş kalıyor, boş liste "kısıtsız" sayılıyor ve bozuk bir
  allowlist **her MCP sunucusunu** CLI sürecine mount ediyordu; yani kapının kapatmak için
  var olduğu delik, tam da belgenin okunamadığı anda açılıyordu.
  - Çağıran iki yol (`internal/climcp/climcp.go` → `WriteConfig`,
    `internal/agent/codexmcp.go` → `codexMCPSpec`) hatayı `logger.Error` ile basar,
    `db.DebugError` tipinde **`mcp_server_gate_malformed`** debug olayı yazar ve **erken
    döner** — hiçbir MCP sunucusu mount edilmez, tur yaptırımsız bir araç yüzeyiyle başlamaz.
  - Ayrıştırma `agent.ParseToolOverridesErr` üzerinden yapılır; lenient `ParseToolOverrides`
    yalnız görüntüleme yollarına aittir (kural ve tablo: `_Docs/19-LAZY-TOOL-LOADING.md`
    "Bozuk izin belgesi = fail-closed").

  **Politika kararı (Bilal, 2026-08-21) — "sözleşmeyi kabul et":** yerleşik worker
  profilleri (`explore`/`planner`/`coder`/`reviewer`/`validator`/`config`,
  `internal/agent/subagent.go`) **built-in-only** kalır; hiçbirine MCP deseni
  eklenmedi. Gerekçe: allowlist'ler kodda duran bir güvenlik sözleşmesidir ve
  native yol bunu zaten yıllardır uyguluyordu — CLI yolundaki MCP erişimi kazaydı,
  kabiliyet değil. Sonuçlar:
  - Etkilenen mevcut ajanlar: 28 legacy allowlist'li `worker:*` (14 codex-cli,
    12 claude-cli, 2 native). Hepsi `resolveWorkerTarget` tarafından otomatik
    materyalize edilmiş; **hiçbiri elle düzenlenmemiş**, bu yüzden migration
    yapılmadı — CLI olanlar artık native olanlarla aynı davranıyor.
  - `subagent.go`'daki `validator` yorumu düzeltildi: "drive a browser for e2e"
    iddiası kaldırıldı. Prompt katmanı zaten dürüsttü ("(when available) a
    browser", `e2e: n/a`); yanlış olan yalnız kod yorumuydu.
  - `coordination.go`'da profil worker'ının `MCPEnabled: true` değeri korundu ve
    "grant değil, ana şalter" olduğu yorumlandı — operatör UI'dan allowlist'i
    genişletirse MCP ayrıca bir bayrak aramadan açılır.
  - Sözleşme teste bağlandı: `TestProfileAllowlistsNameNoMCPServer` — bir profile
    MCP deseni eklenirse test kırılır, yani genişletme bilinçli bir politika
    değişikliği olur.

- ✅ **codebase-memory = allowlist'ten muaf altyapı (Bilal, 2026-08-21):** Yukarıdaki
  "sözleşmeyi kabul et" kararının bilinçli ve TEK istisnası. Worker profilleri built-in-only
  kaldığı için kod grafına da ulaşamıyordu; oysa statik prompt her ajana "graf araçlarını ÖNCE
  kullan, grep son çare" diyor — yani araç olmadan prompt yalan söylüyordu.
  **Fix:** `internal/agent/mcpservergate.go` → `allowlistExemptServer` + `isExemptTool`.
  - Emsal: `toolFilter` zaten koordinasyon araçlarını allowlist'ten muaf tutuyor
    ("allowlist personanın İŞ araçlarını tarif eder"). Kod grafı bir seviye aşağıda aynı şey:
    ajanın repoyu OKUMA biçimi — `Read`/`Glob`/`Grep` ile aynı rol, ve onlar zaten her
    profilin allowlist'inde. Grafı esirgemek kabiliyet kaybı değil, token israfı.
  - Kapsam: yalnız **ALLOWLIST**. Workspace anahtarı ve ajanın **AÇIK** denylist'i hâlâ
    kaldırır — operatör tek bir ajandan grafı bilerek alabilir ve bu karara saygı duyulur.
  - Tek sunucuya kilitli: TionHarness'in altyapı saydığı tek MCP sunucusu bu (yetenek probu +
    prompt bloğu yalnız onun için var). İkincisi açık bir karar gerektirir.
  - Uygulanan iki yol: `toolFilter` (native, `isExemptTool`) ve `mcpServerGate(ag, exempt)`
    (claude-cli + codex-cli mount'u).
  - **Yan düzeltme — yetenek probu artık ajan-farkında.** `Capability.Detect` imzasına
    `agent db.Agent` eklendi. Eskiden yalnız workspace'e bakıyordu: SES948'de built-in-only
    worker'ın promptuna tam codebase-memory bloğu giriyor ("greps are the LAST resort"),
    araçlar ise tool listesinde yok — ajan uyamayacağı bir emri izleyip kaçınması söylenen
    grep'e düşüyordu. Artık blok, ajan araçları gerçekten çağırabiliyorsa basılır; açık
    denylist ile kapatılmışsa susar.
  - Testler: `TestCodebaseMemoryIsExemptFromAllowlist` (native + CLI, sızıntı yok),
    `TestCodebaseMemoryExemptionYieldsToExplicitDenylist` (açık denylist kazanır, prompt susar).
# Validator Unity MCP sözleşmesi (2026-08-23, TSK101)

`validator` profili `unity-mcp` sunucu anahtarını allowlist'inde taşır. Native,
Claude CLI ve Codex CLI yolları yalnız bu dış MCP sunucusunu açar; `Write`, `Edit`
ve Playwright dahil diğer MCP sunucuları kapalı kalır. Profil worker'ları artık
`subagent-validator` sistem ajanına çözülür ve allowlist her spawn'da koddan
yeniden uygulanır (bkz. `_Docs/74-SISTEM-AJANLARI.md`); `ToolOverrides` gibi
diğer ajan özelleştirmeleri ezilmez.

---

## 13. Bundle (demet) listeleme — CLI/gateway yolu (2026-08-28)

`activate_tools` artık araç adının yanında **bundle anahtarı** da kabul eder:
`group:<kategori>` (built-in fonksiyonel kategorileri, `internal/tools/categories.go`)
ve `mcp:<sunucu>` (`internal/tools/bundles.go`). Gateway (claude-cli) tarafındaki
uygulama `internal/api/mcp_interaction.go` içindedir.

### Akış — hangi fonksiyon

1. `callActivate` girdiyi ikiye ayırır: `tools.SplitBundleKey` bir anahtarı tanırsa
   `bundleKeys`'e gider (`bareToolName` normalizasyonuna **sokulmaz**), aksi halde
   araç adı olarak `names`'e gider.
2. Anahtar varsa `listBundles(run, keys)` çalışır; üyeler `bundleIndex(run)` ile
   `candidateDefs(run)` üzerinden gruplanır — bu küme ajanın tool filtresini zaten
   geçmiştir, dolayısıyla workspace'te kapalı bir araç listeye sızamaz.
3. Aynı çağrıda hem ad hem anahtar geldiyse adlar normal yolla
   (`callActivateNames`) aktive edilir, listeleme sonucun sonuna eklenir.
4. codex-cli (`-full` sağlayıcı) dalında her non-core araç zaten ilan edildiğinden
   listeleme anlamsızdır: `"all on-demand tools are already advertised on this
   provider"` döner (savunma amaçlı; `gatewayMetaTools` orada `activate_tools`'u
   zaten ilan etmez).

### Neden push yok

`listBundles` bilinçli olarak **`activateExtended` ÇAĞIRMAZ** ve
**`PushToolsChangedAndWait` ÇAĞIRMAZ**. Üyeleri aktive etmek onların tam şemasını
`tionharness_extended` üzerinde ilan etmek demektir — yani lazy loading'in tam
olarak kaçındığı token patlaması. Demet açmak bu yüzden:

- `Tools(token,"extended")` uzunluğunu **değiştirmez** (regresyon testi:
  `TestGatewayBundleActivateListsWithoutAdvertising`),
- CLI'da `tools/list_changed` bildirimi üretmez (re-list turu harcanmaz),
- `cliTier`/`splitInteractionTiers` sınıflandırmasına hiç uğramaz — bundle anahtarı
  hiçbir zaman bir araç adı değildir.

### Limit 40

Tek bir demet listelemesi `tools.BundleListLimit = 40` üyede kesilir — iki yol da
**aynı sabiti** kullanır, ayrı kopya yoktur. Kesilirse kaç üyenin gizlendiği ve
`tool_search` ile daraltma yönlendirmesi yazılır. Amaç: 300 araçlı bir MCP
sunucusunun tek çağrıda on binlerce token'lık sonuç döndürmesini engellemek.

### Ad ile anahtar ayrımı (sözleşme)

| Girdi | Etki |
|---|---|
| Araç adı (`list_agents` veya `mcp__tionharness_extended__list_agents`) | **Şema yüklenir** — araç aktif sete girer, list_changed push edilir, aynı turda çağrılabilir. |
| Bundle anahtarı (`group:diagnostics`) | **Şema yüklenmez** — yalnız `- ad — özet` satırları döner. Hiçbir şey aktive edilmez. |

Yani akış iki adımlıdır: **demeti aç (isimleri gör) → istediğin adı/adları
`activate_tools`'a ver (şema gelir)**. Üyeler listede doğrudan
**namespace'li çağrılabilir adla** (`mcp__tionharness_extended__<ad>`) yazılır,
çünkü CLI bir aracı yalnız bu adla çağırabilir. Geçersiz anahtar
`unknown bundle: …; known bundles: …` olarak raporlanır — bulanık ad eşleştirmeye
düşmez. `tool_search` sonuç satırları üyenin demet anahtarını `[group:…]`
etiketiyle gösterir.

> **Gateway'de yalnız `group:` demetleri vardır.** `mcp:<sunucu>` anahtarı bu yola
> özgü DEĞİL, native yola özgüdür: gateway'in demet evreni `bundleIndex(run)` →
> `candidateDefs`'tir ve bu küme yalnız built-in araçlardan oluşur — köprü MCP
> araçlarını dışarıda bırakır (`internal/tools/bridge_filter.go`,
> `BridgeableDefsFiltered`), çünkü harici MCP sunucularını claude-cli kendisi
> mount eder. `mcp:*` demetleri yalnız native yolda üretilir
> (`internal/tools/bundles.go`, `Registry.BundleIndex`).

> **Gateway'de demet durumu tutulmaz.** Demet "açmak" yalnız bir listeleme
> render'ıdır; `listBundles` hiçbir durum yazmaz, dolayısıyla kapatılacak bir şey de
> yoktur. `deactivate_tools` bir demet anahtarını **no-op olarak yutar** (anahtar
> `bundleKeys`'e ayrılır ve deactivate dalında kullanılmaz). `OpenBundle` /
> `CloseBundle` yalnız native yolda anlamlıdır
> (`internal/tools/builtin_activate.go`).

> **Katalog farkı:** native (claude-cli olmayan) yolda lazy katalog bloğunun sonuna
> `Bundles: group:… (n), mcp:… (n)` satırı basılır; **CLI formunda bilerek
> basılmaz**. Sebep: native sayım `Registry.BundleIndex`'ten, gateway üyeliği ise
> run'a özel `candidateDefs`'ten çözülür — iki farklı kaynak, sayılar örtüşmezdi.
> Detay: `_Docs/19-LAZY-TOOL-LOADING.md`.

## 14. Aktivasyon kalıcılığı — `activated-tools.json` (2026-08-28)

Aktive edilmiş extended araç kümesi artık **oturuma ait kalıcı bir sidecar'da**
tutulur; RAM'deki `interactionBackend.activated` haritası yalnız bir **önbellek**tir.

- **Dosya:** `<workspace>/store/sessions/<SESID>/activated-tools.json`,
  gövde `{"tools":["notify","focus_view"]}` (sıralı + tekilleştirilmiş).
  Sahibi: `internal/db/activatedtools.go` (`WriteActivatedTools` /
  `ReadActivatedTools` / `ClearActivatedTools`), `inbox.json` ve
  `prompt_epoch.json` ile aynı atomik tmp→rename deseni.
- **Neden:** küme daha önce yalnız RAM'de ve **Bearer token** anahtarıyla
  duruyordu. Uygulama yeniden başlarsa ya da claude-cli alt süreci yeni bir
  token'la yeniden bağlanırsa küme sıfırlanıyor, daha önce aktive edilen araçlar
  CLI'ın listesinden düşüyor ve çağrı
  `No such tool available: mcp__tionharness_extended__<ad>` ile hata veriyordu
  (SES79). Kalıcı kopya **oturum** anahtarlı olduğu için token değişse de küme korunur.
- **Ne zaman yüklenir:** token için küme ilk kez istendiğinde (lazy hydrate,
  `hydrateActivated`). Disk okuma **mutex dışında** yapılır, birleştirme kilit
  altındadır; her token için tek okuma (`hydrated` haritası).
- **Ne zaman yazılır:** `activate_tools` / `deactivate_tools` gerçekten bir şey
  değiştirdiğinde (`persistActivated`, yine kilit dışında).
- **Ne zaman sıfırlanır:** son araç da deaktive edildiğinde sidecar **silinir**
  (boş yazım = temizlik); oturum silinince oturum dizini ile birlikte gider.
  Bunun dışında ne yeniden başlatma ne de token değişimi kümeyi sıfırlar.
- **Bozuk sidecar yutulmaz:** `ReadActivatedTools` geçersiz JSON'da hata döner;
  api katmanı bunu `logger.Error` ile raporlar ve token'ı hydrate edilmiş sayar
  (her araç çağrısında tekrar disk okumamak için).
- **codex-cli (`-full`) yolu etkilenmez:** orada extended tier zaten koşulsuz
  yayınlanır, aktivasyon kapısı hiç çalışmaz.

Testler: `internal/db/activatedtools_test.go` (round-trip, yok-dosya, üzerine
yazma, bozuk JSON) ve `internal/api/gateway_activation_persist_test.go`
(aynı oturum + YENİ token → araç hâlâ aktif — asıl regresyon).

## `wait_for_mcp_servers` — tur ortasında sunucu ısıtma (2026-09-21)

MCP bağlantıları tembeldir: tur başladığında dial edilmemiş bir sunucu o tura hiç
araç katmaz. Bir ajan sunucuyu yeni etkinleştirdiyse/yeniden başlattıysa, eskiden
turu bitirip bir sonraki katalog kurulumunun onu yakalamasını ummaktan başka yolu
yoktu. `wait_for_mcp_servers` bu beklemeyi **açık ve sınırlı** hale getirir.

**Akış.** `internal/tools/builtin_mcp_wait.go` (araç yüzeyi) →
`internal/agent/mcpwait.go` (politika) → `Pool.EnsureServers`
(`internal/mcp/ensure.go`).

- **Paralel dial, tek zaman aşımı.** `EnsureServers` her sunucuyu kendi
  goroutine'inde dialler; maliyet en yavaş sunucu kadardır, toplamı kadar değil.
  Seri olsaydı 5 ölü sunucu 5 × `DefaultDialTimeout` (100 sn) ederdi.
  `timeoutSeconds` (vars. 30, üst sınır 120) tüm kümeyi kapsar.
- **Aynı turda çağrılabilirlik.** Ayağa kalkan sunucuların araçları
  `Registry.AppendMCP` ile **canlı** registry'ye eklenir (AttachMCP ile aynı
  lazy + name-only damgaları) ve ardından aktive edilir. Aktivasyon şart: MCP
  araçları lazy olduğu için `ActiveDefs` onları aksi halde atlar, dahası **donmuş
  prompt epoch'unda** (`mergeFrozenToolDefs`) aktif olmayan ad gönderilen araç
  bloğundan tamamen düşer. Tur döngüsü her iterasyonda `shipFor()` ile yeniden
  hesapladığı için araçlar modelin bir sonraki adımında hazırdır.
- **Devre kesici (circuit breaker) politikası.** Açık bir breaker bu araç
  tarafından **bir kez** bilerek baypas edilir — breaker otomatik katalog
  kurulumlarını korumak içindir, açık bir "bu sunucuyu bekle" isteği ise tam
  tersi durumdur. Sonuç normal şekilde işlenir: başarıda `Clear()` (bir sonraki
  sıradan kurulum cooldown'ı beklemeden dialler), başarısızlıkta `Note()`.
- **Scoped sunucular.** Çağıranın **kendi** `(session, agent)` slotu ısıtılır;
  scope anahtarı `toolsetup.go` ile birebir aynı kurulur (`sid + "|" + agent.ID`),
  yoksa ısıtılan bağlantı sonraki çağrıların kullandığı bağlantı olmazdı.
- **CLI backend'lerinde yok.** claude-cli / codex-cli kendi MCP istemcilerini
  yönetir; havuz onlar için hiç dial etmez ve her sunucuya `ServerUnknown` der.
  Uydurma bir durum raporlamak yerine araç `cliLazyBridgeExcluded` ile köprüden
  çıkarılır (her iki lehçe için).
- **Hatalar yutulmaz ama yükseltilmez.** Bilinmeyen sunucu adı **hatadır**
  (model yazım hatası yapmıştır; "hazır değil" demek onu var olmayan bir bağlantıyı
  ayıklamaya yollardı). Ölü sunucu ise hata değil **bulgudur**: sorunun cevabı
  "hayır"dır ve dört canlı sunucunun haberi bir ölü yüzünden silinmemelidir.

Testler: `internal/mcp/ensure_test.go` (paralellik ölçümü, sunucu başına sonuç,
slot yeniden kullanımı, scope anahtarı), `internal/agent/mcpwait_test.go`
(aynı turda `ActiveDefs`'e girme, donmuş-epoch merge'ünden sağ çıkma, bilinmeyen
sunucu, ölü sunucu + breaker `Note`, açık breaker baypası + `Clear`, scoped slot,
CLI köprüsünden dışlanma), `internal/tools/registry_appendmcp_test.go`.

## Tur ortasında bağlantı kopması — bildirim (TSK915, 2026-09-22)

`wait_for_mcp_servers` soğuk sunucuyu **ısıtma** yönünü kapatır; bu bölüm ters
yönü kapatır: canlı bir bağlantının tur ortasında **ölmesi**. Eskiden bunu yalnız
`internal/mcp/client.go` `failAll` bilirdi ve tek yaptığı log yazmaktı — kullanıcı
araçların sessizce kaybolduğunu görür, model ise az önce kullandığı aracın artık
var olmadığını sanırdı.

**Akış.** `StdioClient.failAll` → `Pool.noteDisconnect`
(`internal/mcp/disconnect.go`) → `Runtime.handleMCPDisconnect`
(`internal/agent/mcpdisconnect.go`) → `ws:mcp_status` olayı (UI) + `mcpDisconnectLog`
(ajan notu).

- **Yalnız beklenmedik ölüm.** Geri çağrı `failAll`'daki mevcut `wasClosed`
  geçidinin içindedir. Havuzdaki her temiz kapanış yolu (`reapScoped` boşta
  toplama, `CloseSession`, config değişiminde re-dial, `Pool.Close`)
  `Client.Close()` üzerinden geçer ve `closed=true`'yu okuma döngüsü çözülmeden
  önce set eder; yani kasıtlı kapanış **yapısal olarak** olay üretemez. Geri
  çağrı `closed` ile aynı kilit altında okunur, böylece eşzamanlı bir `Close()`
  geçidin iki yanına birden düşemez. Her temiz yol için ayrı test vardır
  (`internal/mcp/disconnect_test.go`).
- **Yük ve `scoped` ayrımı.** `DisconnectEvent`: `server`, `scoped`, `scopeKey`,
  `error`, `pendingCalls`. Scoped bir bağlantı tek bir `(oturum, ajan)` çiftine
  aittir; ölümü "sunucu düştü" diye **workspace geneline** yansıtılmamalıdır —
  başka oturumlar aynı sunucuya canlı bağlantı tutuyor olabilir. `SessionID()`
  scope anahtarından oturumu çıkarır.
- **Olay tipi.** `ws:mcp_status` bir **control** tipidir (durum değişimi, sonuç
  değil): sıralı/replay edilebilir workspace akışına biner, `NotifyKinds`'a
  **girmez**, toast çıkarmaz. Dolayısıyla frontend bildirim kaydında eşlenecek
  bir giriş gerekmez. `op` alanı ileride bir "reconnected" durumunun ikinci bir
  olay tipi açmadan aynı kanaldan akmasına yer bırakır.
- **Ajan notu: sunucu başına turda bir kez.** Native döngü her yinelemenin
  başında `foldMCPDisconnects()` çağırır (aynen `foldSteer` gibi): not hem
  konuşmaya eklenir (model görür) hem `StepRecovery` kartı olarak yayınlanır
  (kullanıcı görür). Dedupe ayrı bir bayrak değil, `take()`'in kendisidir —
  rapor edilen kayıt log'dan silinir. Not, araçların **turun geri kalanında**
  kullanılamaz olduğunu açıkça söyler; bu doğrudur, çünkü katalog tur ortasında
  yeniden kurulmaz. Shared bağlantının ölümü her oturuma, scoped ölüm yalnız
  sahibi oturuma gider; `disconnectNoteTTL` (10 dk) sonrası rapor edilmemiş
  kayıt düşürülür (havuz o arada çoktan re-dial etmiş olabilir, bayat not
  yanıltır).
- **Kapsam: yalnız native döngü.** `claude-cli` ve `codex-cli` kendi MCP
  istemcilerini CLI süreci içinde açar; TionHarness o bağlantıları hiç tutmaz,
  dolayısıyla ölümlerini gözlemleyemez. O yolda kayıp yalnız ilgili CLI'ın kendi
  araç hatası olarak görünür. HTTP taşıması da olay üretmez: Streamable HTTP
  çağrılar arasında bağlantısızdır, kaybedilecek bir okuma döngüsü yoktur —
  erişilemeyen uç nokta çağrı başına hata olarak yüzeye çıkar.
- **UI.** Araçlar ekranı `ws:mcp_status`'a abone olur, kartı
  `MCPDisconnectNotices` ile gösterir (sebep + yarıda kalan çağrı sayısı) ve
  havuz anlık görüntüsünü hemen tazeler. Sunucu yeniden canlı göründüğünde kart
  kendiliğinden kalkar (`clearRecovered`) — `Pool.Call` bir sonraki kullanımda
  zaten şeffaf re-dial yapar. i18n `en`/`tr`: `src/i18n/locales/*/tools.json`.
- **Uygulanmadı: otomatik yeniden bağlanma döngüsü.** Sınırlı retry + eşli
  "recovered" olayı bu kartın kapsamı dışında bırakıldı. Kurtarma yolu zaten
  vardır (`Pool.Call` şeffaf re-dial); arka plan retry ayrı bir iştir ve
  yapıldığında `op: "reconnected"` ile aynı olay tipinden akabilir.

Testler: `internal/mcp/disconnect_test.go` (beklenmedik ölümde yük, scoped/shared
ayrımı, dört temiz yolun sessizliği, `splitEntryKey`),
`internal/agent/mcpdisconnect_test.go` (shared/scoped oturum kapsamı, `take()`
dedupe'u, tekrarlı ölümlerin tek kayda inmesi, TTL, not metni),
`frontend/src/features/tools/mcpDisconnects.test.ts` (kart indirgeyicisi).

## 15. MCP kaynakları — `list_mcp_resources` / `read_mcp_resource` (TSK909, 2026-09-22)

MCP'nin iki yüzü vardır: **araçlar** (yan etkili çağrılar) ve **kaynaklar**
(adreslenebilir içerik — doküman, şema, veri kümesi, üretilmiş dosya). TionHarness
bugüne dek yalnız araçları konuşuyordu; değeri kaynaklarında olan bir sunucu
buradaki ajanlar için tamamen görünmezdi. Bu bölüm kaynak yüzeyini anlatır.

### Protokol

Üç metot elle implemente edildi (`internal/mcp/resources.go`), deponun geri
kalanıyla aynı SDK'sız stilde:

- `resources/list` — sabit URI'li somut kaynaklar.
- `resources/templates/list` — **parametreli** URI'ler (RFC 6570, `db://{table}`).
  Alan adı burada `uri` değil `uriTemplate`'tir; `Resource.URI`'ye eşlenip
  `Template=true` ile işaretlenir.
- `resources/read` — bir URI'nin arkasındaki içerik blokları.

İki taşıma da (stdio `client.go`, Streamable HTTP `http.go`) aynı üç `Client`
metodunu uygular: `SupportsResources()`, `ListResources()`, `ReadResource()`.

### Yetenek geçidi — `initialize` sonucu artık saklanıyor

Kaynak desteği MCP'de **opsiyoneldir**. Eskiden `initialize` **sonucu** tamamen
atılıyordu; şimdi `ServerCapabilities` olarak ayrıştırılıp istemcide tutuluyor
(`parseInitializeResult`). `capabilities.resources` ilan etmeyen bir sunucuya
kaynak metodları **hiç sorulmaz** — aksi halde modele JSON-RPC `-32601`
("method not found") ulaşır ve bu, eksik bir opsiyonel özellikten çok
TionHarness'te bir bug gibi okunur. Liste çıktısında o sunucu
"no resource support" satırıyla **açıkça** görünür: sessizce atlanmaz.

### Hata izolasyonu — sunucu başına

`Pool.Resources` her sunucu için bir `ServerResources` döndürür (katalog
kurulumundaki `errs` haritasının aynı şekli). Dört ayrı durum ayırt edilir ve
hiçbiri diğerini silmez:

| Durum | Alan | Çıktı |
| --- | --- | --- |
| Kaynakları var | `Resources` | listelenir |
| Kaynağı yok | — | **boş liste**, hata değil |
| Kaynak desteği yok | `Unsupported` | "no resource support" |
| Bağlanamadı / liste patladı | `Err` | `ERROR — <sebep>` |

`resources/templates/list` hatası `resources/list`'i **iptal etmez**: somut
kaynaklar döner, şablon hatası `Note` alanında raporlanır. Tersi geçerli değildir
— `resources/list` hatası o sunucu için ölümcüldür, geriye raporlanacak bir şey
kalmaz.

Şekli bozuk bir liste yanıtı **görünür hatadır**, boş liste değil: bozuk sunucu
ile kaynağı olmayan sunucu ayırt edilebilir kalmalıdır.

### İkili içerik asla context'e girmez

`read_mcp_resource` metin içeriği satır içi döndürür; **ikili** içerik oturum
scratchpad'i altındaki `mcp-resources/` dizinine dosya olarak yazılır ve
yol + mimeType + boyut döndürülür. Gerekçe: base64 bağlam penceresinde saf
israftır (2 MB'lık bir görsel ~700k token) ve model onunla zaten bir şey yapamaz;
bir yol ise `Read`'e, kabuk komutuna veya başka bir araca verilebilir.

Oturum bağlı değilse yazacak yer yoktur — bu **hata** olarak döner, satır içine
düşülmez. Satır içine düşmek, bu yolun önlemek için var olduğu şeyin ta kendisi
olurdu.

Metin içerik `maxResourceTextBytes` (64 KB) ile sınırlıdır ve kırpma **açıkça**
raporlanır (`TRUNCATED: showing the first N of M bytes`) — model bir önekin
üzerinde tüm belgeye sahipmiş gibi akıl yürütmez.

### Kapsam ve kayıt

Araçlar `wait_for_mcp_servers` ile **aynı koşulda** kurulur: havuz var **ve** en
az bir yapılandırılmış sunucu (`toolsetup.go`). `scoped` bir sunucu çağıranın
kendi `(session, agent)` havuz yuvasından okunur (`mcpScopeKey`) — ikinci bir
bağlantı açılmaz. Görünürlük kademesi `summary`: isim ne yaptıklarını söyler ama
"resource"ın araç değil sunucu tarafı belge/şema demek olduğunu söylemez.

### Backend kararı — yalnız native

- **Native Go döngüsü:** buna ihtiyacı olan tek backend. Burada uygulandı.
- **claude-cli:** MCP istemcilerini `--mcp-config`'ten kendi başlatır ve
  kaynakları zaten native destekler (`ListMcpResources` / `ReadMcpResource`).
- **codex-cli:** istemcileri yine kendi sahiplenir, kaynak desteği yoktur.
  Köprülemek aynı sunucuya **ikinci bir stdio süreci** açardı — yinelenen
  bağlantı, yinelenen oturum durumu.

İkisi de `cliLazyBridgeExcluded`'dadır (her iki lehçe için de).

Testler: `internal/mcp/resources_test.go` (yetenek geçidi ve "hiç sorulmadı"
kanıtı, şablon birleştirme, şablon hatasının somut kaynakları düşürmemesi, liste
hatasının ölümcüllüğü, bozuk yükün görünür hata olması, boş listenin hata
olmaması, text/blob ayrımı, havuz düzeyinde sunucu başına sonuç),
`internal/agent/mcpresources_test.go` (kayıt koşulu + kademe, metadata ve şablon
etiketi, desteklemeyen/boş/bozuk sunucu raporu, bilinmeyen sunucu hatası, ikili
içeriğin dosyaya yazılması + base64'ün context'e girmediğinin iki yönlü kanıtı,
kırpma raporu, CLI köprüsünden dışlanma, scoped yuva kullanımı).
