# TionSwarm — Ilerleme Arsivi (2026-06-19 ve oncesi)

> Bu dosya [05-ILERLEME.md](05-ILERLEME.md) ana ilerleme gunlugunden arsivlenen, tamamlanmis faz/is kayitlaridir.
> Canli/guncel durum icin ana dosyaya bakin. Buradaki en yeni kayit 2026-06-19 tarihlidir.

---

<!-- 2026-07-27'de 05-ILERLEME.md'den arsivlendi: 2026-06-22 .. 2026-06-30 kayitlari. -->

## Ayarlar ▸ Gelişmiş'ten ölü alt-bölümler kaldırıldı ✅ (2026-06-30)

**İstek:** Gelişmiş ekranındaki "MCP & Araçlar" ve "Tanılama" bölümleri gereksizse
kalksın; loglar zaten Logs ekranına gidiyor, oradan filtrelenebiliyor.

**Teşhis (ikisi de ölü ayar):**
- `mcpGatewayUrl` ("MCP & Araçlar"): tüm repoda yalnız kaydedilip yükleniyor, runtime
  hiç **tüketmiyor**. Gerçek MCP sunucu yönetimi ayrı "Araçlar & MCP" (`mcptools`)
  kategorisinde → alt-bölüm gereksiz.
- `logLevel` ("Tanılama"): `cmd/tionswarm`'da hiç okunmuyor → logger'a **uygulanmıyor**.
  Ayrıca `LogsPanel.tsx` zaten seviye filtresi + metin araması sunuyor → gereksiz.

**Yapılan (frontend):** `SettingsPanel.tsx`'ten iki `AdvSection` (MCP & Araçlar,
Tanılama) + `McpPanel`/`DiagnosticsPanel` import'ları + kullanılmayan `Plug`/`Activity`
ikonları kaldırıldı; `settings/appPanels.tsx`'ten `McpPanel` ve `DiagnosticsPanel`
fonksiyonları silindi. Backend alanları (`mcpGatewayUrl`/`logLevel`) geriye-uyum için
settings.json'da kalır (zararsız). `tionswarm-settings` skill'inde ikisi deprecated/unused
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

**Teşhis (ampirik, kapatıldı):** Kök neden TionSwarm'da **DEĞİL**. Kanıtlar:
(1) Go I/O uçtan uca UTF-8 — `internal/db/store.go` (JSONL) `encoding/json` + atomik
bayt yazımı, `internal/memory` + `agent/reflector.go` (journal=`"Q: "+req.Message+...`)
saf Go string; tüm `internal`'da **tek `DecodeString` base64**, hiçbir charset decoder
(`x/text/charmap`/CP125x) yok. (2) `claude-cli` provider'ı TionSwarm'nun bire-bir byte
yoluyla (stdin raw UTF-8 / stdout raw) Türkçe'yi **doğru** round-trip eder (probe ile
doğrulandı). (3) Diskteki yer-gerçeği: asistan cevapları `×`/`÷`/`−` çok-baytlı Unicode'u
**kusursuz** saklamış; yalnız **user mesajları** bozuk → bozulma `req.Message`
TionSwarm'ya gelmeden, **gönderen istemcide** (double-encode: UTF-8 bayt CP1254 çözülüp
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

**Hedef:** İzole config dizini için **TionSwarm UI'ından** login akışı — bir popup'ta
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

**Default (2026-06-30):** Artık **izole-by-default** → `~/.tionswarm/claude-home`
(`settings.defaultClaudeConfigDir()`, home çözülemezse `""`=ortak `~/.claude` fallback).
Yani claude-cli kutudan çıktığı gibi temiz bir config evinden çalışır; **tek seferlik
`claude` login** o dizinde gerekir. Anahtarsız-mevcut-login davranışı istenirse alan
boşaltılır (UI placeholder `otomatik (~/.claude)`). Bilinçli ürün kararı: TionSwarm runtime'ı
kullanıcının kişisel `~/.claude` skill/tool kirliliğinden ayrışır. (TionSwarm zaten
`--strict-mcp-config` ile MCP'leri izole ediyordu; bu, eksik olan skill/ayar/CLAUDE.md
katmanını da kapatır.) CLI bu env'e saygı duyar (doğrulandı: Claude Code env-vars docs +
issue #25762); TionSwarm CLI'ı doğrudan subprocess çağırdığı için VS Code eklentisindeki
bug yolu etkilemiyor.

**Dokunulan katmanlar (mevcut `claudeCliPath` aynalandı):** `settings.go`
(Settings+DTO+Patch+default `claudeConfigDir`), `store.go` (applyString),
`providers/kind.go` (`ResolvedConfig.CLIConfigDir`), `registry.go`
(`claudeConfigDir` alanı + `SetClaudeConfigDir` + resolve), `kind_claudecli.go`
(`NewClaudeCLI` 3. param), `claudecli.go` (`configDir` alanı + `runAttempt`'ta
`cmd.Env` enjeksiyonu), `api/server.go applySettings` (canlı uygula), frontend
(`settings.ts` tip, `SettingsPanel` save, `ProvidersPanel` Anthropic kartında opsiyonel
2. endpoint alanı "claude config dizini"). Default skill `tionswarm-settings` belgelendi.
`go build`/`go vet`/`tsc` ✅.

## Token/bütçe muhasebesi denetimi + CLI ek-yükü düzeltmesi ✅ (2026-06-30)

**Hedef:** Token ve bütçe hesaplamalarında yanlışlık var mı? (TionSwarm↔the external agent project
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
CacheWrite` ile sahte ~5–7× ek-yük üretiyordu (kıyas oturumunda TionSwarm'yu olduğundan
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
  boşken):** Settings override (>0) → env `TIONSWARM_MAX_OUTPUT_TOKENS` (>0) → aile
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
  <br>*(2026-07-27: `crabbox` katalogdan kaldırıldı — CI'daki crabbox tabanlı iş
  akışı da kaldırılıp gate `.gitea/workflows/ci.yml`'e taşındı. `dev` kategorisinde
  yalnız `codebase-memory-mcp` kaldı. Detay: `05-ILERLEME`.)*
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
deneyimine yaklaştırmak; TionSwarm board'unu Obsidian'da Kanban/Tablo/Gantt olarak
görüp **çift yön** senkronlamak.

> **Güncelleme (2026-06-30):** `type` (task/subtask/milestone) ve `parentId`
> TionSwarm'dan kaldırıldı (ihtiyaç yok) — `Task` modeli, API, araçlar ve modal
> Tür alanı temizlendi. Subtask hiyerarşisi yalnız Obsidian (PM) tarafında yaşar;
> köprü onu PM-only tutar. TionSwarm'da kalan zengin alanlar: **priority + tags**.

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

**Obsidian-pm köprüsü (harici, `Desktop\Progs\tionswarm-obsidian-sync`, Python):**
- TionSwarm REST API (`/api/tasks`) ↔ obsidian-pm projesi (düz Markdown: `<proje>.md`
  + `<proje>_tasks/*.md`, `pm-project`/`pm-task` frontmatter). Sunucu/DB yok.
- **Tam çift-yön:** başlık/açıklama/durum/owner/deps + **priority/tags/type/parentId**
  + subtask hiyerarşisi (subtask'lar proje `taskIds`/gövdeden hariç, parent `subtaskIds`
  yeniden hesaplanır). `parentId` link tablosundan PM-id↔TS-id çevrilir.
- Çakışma: *boş tarafı doldur; ikisi de doluysa son-yazan-kazanır* (LWW **tur başındaki
  orijinal** zaman damgalarıyla — çekirdek push'un PM dosyasını yeniden yazıp updatedAt
  bumplaması kaynaklı yanlış-yön hatası bu şekilde giderildi). İdempotent.
- Durum eşlemesi: TS `failed`↔PM `blocked`, `review`↔`review`, gerisi birebir.
  Bağ + son-senkron anlık görüntüsü sidecar `state/<proje>.json`'da; `tionswarmId`
  PM frontmatter'ına gömülü (sidecar kaybolsa bağ kurtarılır).
- **`watch` modu:** periyodik otomatik senkron; Progs altında arka plan servisi olarak
  Windows zamanlanmış görevle (logon'da) çalışır.
- Canlı backend'de uçtan uca doğrulandı (TS↔PM priority/tags/type/parent, idempotent).

### Köprü → çoklu-provider mimarisi (kanban soyutlaması) ✅ (2026-06-30)

**Hedef:** Kanban kontrolünü tek bir platforma (obsidian-pm) sabitlemek yerine
**değiştirilebilir provider** arkasına almak; ileride Trello/Asana/WeKan'a yalnız
yeni bir dosya yazarak geçebilmek (`mermaid-cli` benzeri soyutlama). TionSwarm task
store'u **canonical kaynak** olarak kalır (ajan orkestrasyonu onun üstünde).

- **Faz 1 — soyutlama (davranış değişmedi):** `sgsync/providers/` paketi eklendi.
  `base.py` provider sözleşmesi (`Card` canonical model — status'u TionSwarm board
  sözlüğünde; `ProjectInfo`; `Caps` yetenek bayrakları; `KanbanProvider` ~5 metot).
  Tüm obsidian-pm mantığı engine'den `providers/obsidian.py`'ye taşındı
  (status/hiyerarşi map, `tionswarmId` gömme, `subtaskIds` roll-up). `engine.py` artık
  **provider-bağımsız** — yalnız `Card` + `KanbanProvider` konuşur; çakışma çözümü,
  kimlik eşleme, çoklu-workspace fan-out, watch kilidi aynen korundu.
- **Capability modeli:** provider tutamadığı alanı bildirir (`Caps`); engine zorla
  map etmez, **atlar** (ör. Trello'da priority/deps/hierarchy yok → es geçilir).
- **Faz 2 — Trello provider:** `providers/trello.py` (REST: list↔status, label↔tag,
  arşiv↔close, TS id desc marker'ına gömülü; key+token auth). `config.json`'a
  `_trello_example` mapping bloğu + factory (`make_provider`, mapping `provider`
  anahtarı, varsayılan `obsidian`, geriye uyumlu).
- **State göçü:** snapshot `pm_status` artık canonical tutulduğu için tek-seferlik
  `migrate_state_v2.py` eski native değerleri çevirdi (10 link); böylece refactor
  sonrası ilk sync'te sahte churn olmadı.
- **Doğrulama:** obsidian yolu canlı backend'de **dry 0/0 stabil** (davranış birebir);
  `tests/test_providers.py` → Trello map mantığı + factory hataları + **engine'in
  board-bağımsızlığı** (sahte provider/TS ile iki-yön kart üretimi) yeşil. Watch
  servisi yeni kodla yeniden başlatıldı, idle teyit edildi. Detay: köprü README.

### Köprü sadeleştirme: Trello provider kaldırıldı + dosyalar workspace içine (2026-06-30)

**Hedef:** Generic board yönetim sistemini korumak ama Trello provider'ını sökmek;
obsidian-pm dosyalarını harici tek vault yerine **her TionSwarm workspace'inin kendi
klasörüne** taşımak (Obsidian kullanıcısı o klasörü manuel vault olarak açar).

- **Trello kaldırıldı:** `providers/trello.py` silindi, registry'den çıkarıldı,
  `config.json`'daki `_trello_example` ve testteki Trello map kaldırıldı. Generic
  seam (`base.py`/`obsidian.py`/factory) **aynen duruyor** — registry'de tek slug
  (`obsidian`); yeni provider eklemek hâlâ tek dosya + tek satır.
- **Workspace-içi yerleşim:** yol artık mapping'in `workspace_id`'sinden türüyor →
  `<tionswarm.data_dir>/workspaces/<WS_ID>/<obsidian.subdir>/`. `_resolve_projects_dir`
  önceliği: `obsidian_dir` override → `data_dir`+workspace → eski `projects_dir`
  (geriye uyumlu). `data_dir` var ama `workspace_id` yoksa **sessizce yanlış yere
  yazmaz, hata fırlatır.** Backend değişikliği gerekmedi (yol bridge tarafında türetiliyor).
- **Veri göçü:** mevcut 3 proje (`TionSwarm`/`DenemeBilimsel`/`TionSwarmRepo` ↔ WS1/WS2/WS5)
  eski vault'tan `…/.tionswarm/workspaces/<WS>/obsidian/`'e taşındı; state link'leri
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

## claude-cli cache sıcaklığı — 4 fazlı çözüm ✅ (2026-06-29)

**Sorun (ölçüldü):** Taze claude-cli session'ı tam soğuk başlıyor ve **tur 2 de soğuk** (cache_read=0). Kök neden: `claudecli.go` statik sistem promptu + **volatil** dinamik bloğu (saniye-hassas saat + bellek recall + özet) TEK `--append-system-prompt`'a birleştiriyordu → cache'lenen ~30K prefix her tur değişiyor.

**Faz 1 — Cache prefix stabilizasyonu (keystone).** `ClaudeCLI.buildSystemAndPrompt`: `--append-system-prompt`'a YALNIZ statik `req.System` gider; volatil `req.SystemDynamic` konuşma prompt'una (stdin) `[Context]…[/Context]` bloğu olarak taşınır → append-system byte-stabil → Claude Code auto-cache sıcak kalır. İzin+MCP arg üretimi `permissionArgs`/`mcpArgs` helper'larına çıkarıldı. **Canlı kanıt:** resume turn 3, volatil dinamikle **cache_read=54.932, cacheWrite=61**.

**Faz 2 — `ClaudeResume` varsayılan açık.** `settings.Default()` → `ClaudeResume: true`. Faz 1 sayesinde resume artık gerçekten ısıtıyor: **turn 2 cache_read=45.420** (canlı). Live test `TestLiveClaudeCLIResume` cache_read>0 assert'i ile genişletildi.

**Faz 3 — Statik prefix cross-session deterministik.** Denetlendi: `composeTurnRequest` zaten temiz statik/dinamik ayrımı yapıyor (saniye-hassas saat dinamikte), tool+skill katalogları zaten Name'e göre sort'lu (`registry.go`/`store.go`). Değişmezlik birim testi `TestBuildSystemAndPrompt` ile kilitlendi (sys yalnız statik, deterministik, dinamik kuyrukta).

**Faz 4 — Kalıcı claude-cli süreci (opsiyonel, deneysel, default off).** `providers.CLISession` + `CLISessionPool` (`claudecli_session.go`): session başına uzun-ömürlü `claude --input-format stream-json` süreci; soğuk başta tam transkript, sıcakta yalnız son kullanıcı mesajı (süreç gerisini hatırlar). `Runtime.cliSessions` pool'u (CloseMCP'de kapanır, 30dk idle eviction), tek-huni `recordedComplete`'te session-id ile devreye girer (hata → tek-atış fallback); `ClaudePersistentSession` açıkken `planClaudeResume` trim'i devre dışı. Ayar tüm sitelere bağlandı (Settings/DTO/Patch/Apply/tunable/server apply). **Canlı:** context korunuyor (ZEBRA-9) + cache ısındı (turn 2 cache_read=87.672); warmth TTL'e bağlı, default off. Live test `TestLivePersistentSession`.

Doğrulama: `go build ./...` + `go vet` + paket testleri temiz; `TIONSWARM_LIVE_CLI=1` ile iki live test PASS.

## Context-preview "CLI ek yükü" satırı ✅ (2026-06-29)

**Sorun:** `GET /api/sessions/{id}/context-preview` çıktısı (`systemTokens`/
`toolTokens`/`totalTokens`) yalnızca TionSwarm'nun **kendi** enjekte ettiği katmanı
sayar. `claude-cli` sağlayıcısında alttaki CLI **kendi sistem
promptu + araç şemaları + MCP köprüsünü** modele ekler — TionSwarm bunu hiç
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
TionSwarm `recordDiff` çağrılmıyor → düzenleme generic `tool` adımı olarak **ActivityCard**
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
TionSwarm ise `[]string` bekliyordu → `json: cannot unmarshal array into ... askInput.options
of type string` → 3 ardışık `ask_user` hatası, model 4. turda options'ı bırakıp düz metinle sordu.

**Çözüm:** `ask_user` şeması native ile uyumlu hale getirildi (`internal/tools/builtin_ask.go`):
- `options` öğesi artık **string VEYA obje** olabilir (`{label|content|value|text|description}`
  → görünen metne normalize; `askOption` + `flexOptions`).
- Native **`questions[]` wrapper** da tolere edilir (ilki kullanılır — TionSwarm tek soru sorar).
- Ortak `tools.ParseAskInput` hem native tool yolunda hem claude-cli Interaction MCP köprüsünde
  (`callAsk`) kullanılır → iki yol birebir aynı çözümler. Şema `oneOf` (string|object), ekstra
  native alanlar (header/multiSelect) yok sayılır. Test: `TestParseAskInput` (7 şekil) geçti.

## Doğrulama araçları (skill/config/mermaid) + interaction köprü konsolidasyonu ✅ (2026-06-29)

External Agent `session-tools-core` ↔ TionSwarm araç eşleştirmesindeki boşluk analizinden
(bkz. `_Docs/analiz-craftagent-arac-eslestirme.md`) çıkan üç **salt-okuma doğrulama
aracı** eklendi:

- **`skill_validate`** — bir skill'in SKILL.md'sini doğrular (slug hijyeni, frontmatter
  `name`/`description`, boş gövde). Mantık tek kaynak: `skills.Store.ValidateSkill`
  (`internal/skills/validate.go`); tool `internal/tools/builtin_skillvalidate.go`
  (`SkillValidator` arayüzü, agent tarafı `agentSkillWriter.ValidateSkill` adaptörü).
- **`config_validate`** — TionSwarm JSON config dosyalarını doğrular (geçerli JSON +
  tanınan şekiller için beklenen alanlar: settings.json / tools-config.json / agent /
  mcp-server). `internal/tools/builtin_configvalidate.go`, çalışma dizini sandbox'ı.
- **`mermaid_validate`** — saf-Go hafif lint (tanınan diyagram tipi + denge kontrolü;
  tam parser DEĞİL, sınır belgelendi). `internal/tools/builtin_mermaidvalidate.go`.

Üçü de native builtin + **NameOnly (lazy)** → claude-cli'da `tionswarm_extended`
köprüsünden `BridgeableDefs` ile otomatik gelir. Kayıt: `toolsetup.go` (mermaid base,
skill_validate `r.skills!=nil`, config_validate `sb.Ready()`; üçü NameOnly tier'a eklendi).

**Köprü konsolidasyonu (#2):** `mcp_interaction.go`'da delege eden 6 `callXxx`
(todo/artifact/notify/focus/goal/sessionEdit) tek tablo-güdümlü `sinkToolTable` +
`callViaSink` yardımcısına indirildi — davranış birebir korundu, ~120 satır boilerplate
kalktı. (Not: bu, "tek Context, çok-backend" refactor taslağının düşük-riskli/kozmetik
parçası; büyük `ToolContext` arayüzü gereksiz bulunup uygulanmadı — TionSwarm deseni zaten
ctx-value injection ile gerçekliyor.)

**Test:** `builtin_validate_test.go` (mermaid/config/skill_validate), `skills/validate_test.go`
(ValidateSkill) + tüm suite **510 test** yeşil. Doküman/skill: `_Docs/19` (NameOnly seti),
`tionswarm-self-management` (skill_validate notu), `tionswarm-guide` (mermaid_validate notu).

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
available` veriyordu. Kök neden: TionSwarm tüm bridged araçları (eager + tüm
self-management suite) **tek** `tionswarm_interaction` MCP sunucusuna full-şema koyuyor;
toplam şema bağlam penceresinin %10'unu aşınca claude-cli 2.1.x **hepsini erteliyordu**
(Bash dahil). UI'daki "her tur şema gönderilen · 19" metriği **native** yola aitti;
CLI yolunda lazy-loading kazanımı gerçekleşmiyordu.

**Çözüm:** Interaction MCP'yi **iki sunucuya** böldük (claude-cli'ın native Tool
Search mekanizmasını doğru kullanarak):
- `tionswarm_interaction` (CORE, `alwaysLoad: true`) → eager tier, tool-search'ten muaf
  → Bash/ask_user/use_skill ilk turdan hazır. Eski anahtar korundu (namespaced
  referanslar bozulmadı).
- `tionswarm_extended` (EXTENDED) → self-management + NameOnly oturum araçları;
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
`mcp__tionswarm_extended__` namespace'inden ToolSearch ile lazy yüklendi. Detay: `_Docs/19`.

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
  Geliştirme" → geniş popup; meta (Gömülü·TionSwarm + #template #workspace), stat (4 ajan/2 akış/0 zam/1 skill),
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
- **Self-mgmt** (gri) → hidden tier: katalogda ismi bile yok, `tionswarm-self-management`
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
  getiri. Kategori-pointer + `tionswarm-self-management` skill + tool_search zaten keşfi
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
olarak (`mcp__tionswarm_interaction__*`) görür ve kendi ToolSearch'üyle yükler — yani blok
yanıltıcıydı (skills bloğu zaten doğru namespaced biçimi kullanıyordu, tools bloğu değil).
Default-NameOnly değişikliği 10 built-in'i daha bu bloğa eklediği için fark belirginleşti.

- **Düzeltme:** `LazyToolsCatalogBlock` artık ajanın `provider`'ına göre dallanır.
  claude-cli formunda: built-in → `mcp__tionswarm_interaction__<ad>`, MCP → `mcp__<server>__<tool>`,
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
  `OnboardingScreen`. `tionswarm.hasSetup` localStorage bayrağı bir workspace var olunca
  set edilir → splash yalnız **taze kurulumda** (kurulum yapılmamışken) görünür, dönen
  kullanıcı doğrudan uygulamaya girer.
- **Yeni bileşenler:** `components/SplashScreen.tsx` (self-contained, tema-değişkenli),
  `components/workspace/OnboardingScreen.tsx` (karşılama kartı + mevcut
  `WorkspaceCreateModal`'ı açar; popup başta açık, kapatılırsa kurulum yapılmaz).
- **Splash marka + min-süre (2026-06-26):** Splash gerçek logoyu (`/favicon.svg` — mor gradyan
  swarm markası, hem Vite dev hem embed binary'de servis ediliyor) dönen aksan halkası +
  "TionSwarm" wordmark + "Yükleniyor…" ile gösterir (logoda pulse + ekran fade-in keyframe'leri
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
- **Doğrulama:** `go build ./...` + frontend `tsc --noEmit` yeşil. Taze `TIONSWARM_DATA_DIR`
  ile canlı test (8091): `GET /api/workspaces` → `count=0` + `workspaces.json` yok (otomatik
  oluşturma gerçekten kalktı), ardından `POST /api/workspaces` → WS1 oluştu, `count=1`.
- **Canlı UI smoke testi (2026-06-26, tek-binary 8091 + Playwright):** taze örnekte tarayıcı
  `http://127.0.0.1:8091` → **onboarding + create popup** render edildi (snapshot doğrulandı);
  ad girip **Oluştur** → URL `#/w/WS1/chat`, uygulama yüklendi; Workspace ▸ "Workspace'i sil"
  → yeni "son workspace" onay metni çıktı → kabul → **onboarding canlı geri döndü** (popup
  yeniden açıldı), `GET /api/workspaces` → `[]`. Gerçek veri (`~/.tionswarm`, 4 ws) izole tutuldu.

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
- **Etki (canlı ölçüm, WS5):** `tionswarm-autonomous-ops` NameOnly → satır **839→26
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
   workspace-default `<cwd>/.tionswarm/progress.json`'u paylaşıyordu → detay-panelde
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
- **Canlı sonuç:** ajan `mcp__tionswarm_interaction__read_session_debug`'i araç
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
  ve API özeti otomatik içerir. Skill `tionswarm-self-debug` + `_Docs\38` güncellendi.
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
- **Default skill** `tionswarm-self-debug` (ajana metriklerini optimize için nasıl
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
`tionswarm-guide` güncellendi. Playwright doğrulaması: 2 görsel+1 video ardışık → tek
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
`images` → `Gallery`. Ajan guidance'ı (`artifactDeliverableGuidance` + `tionswarm-guide`
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

- **CC model → TionSwarm provider/model eşleme** ✅: `agentAdapter` artık CC subagent
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
- **Skill:** `tionswarm-progress` north-star satırı + `tionswarm-guide` interaction bölümü güncellendi.
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

## Default skill: `tionswarm-doc-improver` (6-ölçütlü doküman denetimi) ✅ (2026-06-25)

**Ne:** Yeni gömülü default skill — TionSwarm'nun kendi bağlam dokümanlarını (skill'ler,
workspace CLAUDE.md/AGENTS.md kuralları, `_Docs`) denetleyip iyileştiren tekrarlanabilir
iş akışı. Anthropic'in resmi `claude-md-improver` skill'inden ilham; TionSwarm'nun daha
geniş doküman yüzeyine genelleştirildi.

**İçerik:** 6 ölçüt (komutlar / mimari açıklığı / açık-olmayan gotcha'lar / kısalık /
güncellik / uygulanabilirlik, her biri /5) + puanlı rapor formatı. TionSwarm'ya özgü
çekirdek içgörü: **changelog-leak anti-pattern** — referans dokümanların (`tionswarm-project`,
`_Docs` mekanik bölümleri) tarih damgalı geçmişi biriktirmesi; çözüm "1 cümle güncel durum
+ → `_Docs/NN`" kalıbı. **Doc-type kalibrasyonu:** referans/her-tur-yüklenen dokümanda
kısalık sert, on-demand action skill gövdesinde işlevsel yoğunluk normal → sağlıklı
dokümanı zorla kesme.

**Gömme:** Go değişikliği **gerekmedi** — `internal/skills/defaults.go` `//go:embed defaults`
tüm ağacı gömer ve slug'ları alt-dizinlerden türetir; yalnız `defaults/tionswarm-doc-improver/SKILL.md`
eklendi. Build + 29 test yeşil.

**Yan iş (aynı oturum, davranışsız doküman temizliği):** `tionswarm-project` referans skill'i
~%11 kısaltıldı (changelog-leak temizlendi + PowerShell çalıştırma komut bloğu eklendi);
`tionswarm-guide` Memory maddesi okunabilirlik için alt-maddelere bölündü; default-skill listesi
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
- **Ajan farkındalığı:** `tionswarm-guide` default skill'ine "Rich replies"
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
  Binary yeniden derlenip :8090'da restart edildi (eski binary `tionswarm.bak.exe`).
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
  doğrudan kurulabiliyor. (Eski `tionswarm-flows` skill'i "must be acyclic" diyordu — **yanlıştı**,
  düzeltildi.)
- **Kısıt → karar:** branch yalnız string eşler (sayısal eşik yok) → skor→pivot kararı
  **keyword verdict** (`VERDICT: SHIP|REFINE|PIVOT`, `decide` `matchMode:regex` son satıra
  demirli); döngüde graf çıktısı üzerine yazıldığından skor **trend'i** `core:sprint-scorelog`
  çekirdek belleğe append edilir, sözleşme `core:sprint-contract`'ta yaşar.
- **Dağıtım:** gömülü gallery şablonu `gan-loop` (`frontend/src/lib/flowTemplates.ts`) +
  market paketleri `flow.gan-generator-evaluator` / `agent.skeptical-evaluator` /
  `mcp.playwright` (global market dizinine yazıldı) + yeni default skill
  `tionswarm-gan-loop`. Şablon agent-bağımsız → kurulumdan sonra **iki ayrı ajan** atanır.
- **Test:** `engine_test.go` — `TestValidate_AllowsCyclicGraph`, `TestRun_GANLoop_RefinesThenShips`
  (2× REFINE → SHIP → finalize), `TestRun_GANLoop_StepCapBackstop` (hiç ship etmeyen →
  `step cap` hatası). `go build`/`vet`/`test ./internal/orchestration` ✅; `tsc -b`/`vite build` ✅.
- Detay: [`15-FLOW-CANVAS.md`](15-FLOW-CANVAS.md) §Generator↔Evaluator döngü şablonu;
  kullanım kılavuzu: `tionswarm-gan-loop` skill.

## Yapılandırılmış subagent görev sözleşmesi ✅ (2026-06-25)

Anthropic *"Multi-agent research system"* rehberi: her subagent'a **objective +
output format + tool/source guidance + boundaries** verilmezse iş tekrarı/boşluk
oluşur. TionSwarm'da `run_subagent` yalnız serbest-metin `task` alıyordu; bu 4 alanı
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
`feature_list.json` `passes` boolean) konvansiyonu TionSwarm'ya getirildi. `todo_write`
listesi artık **diske kalıcı**: oturumlar arası kaybolmuyor, yeni oturum devralıyor.

- **Sorun:** `todo_write` stateless'tı; liste yalnız oturum-içi (mesaj trace'inden
  `todoContextBlock` ile yeniden inşa) yaşıyordu. Oturum restart/yeni oturum/ajan
  değişiminde kayboluyordu. Core memory (serbest persona/human) bunu karşılamıyor.
- **Çözüm:** Liste, çalışma dizinine bağlı **`<cwd>/.tionswarm/progress.json`**'a
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
- **`tionswarm-progress` default skill'i:** ajana otomatik progress.json + insan-okunur
  `PROGRESS.md` konvansiyonunu öğretir (`internal/skills/defaults/tionswarm-progress/`;
  `//go:embed` ile otomatik, baseline skill setine girer).
- **UI görüntüleyici:** `GET /api/sessions/{id}/progress` (`api/progress.go`) +
  `SessionDetailPanel` "Kalıcı ilerleme" salt-okunur kartı (`ProgressCard` — statü
  işaretçili maddeler + category + son log satırları). `go test ./...` 438 ✅.
- Detay: [`36-KALICI-ILERLEME.md`](36-KALICI-ILERLEME.md).

## Otonom turda boot-verification sırası ✅ (2026-06-25)

Anthropic *"Effective harnesses for long-running agents"* makalesindeki **standart
oturum açılış sırası** (yönelim → hatırlama → tek görev seç → temel testi doğrula →
işi yap → döngüyü kapat) TionSwarm'nun otonom turlarına getirildi. Kayıp bağlamı telafi
eden, düşük-riskli, çoğunlukla skill+doküman değişikliği.

- **Skill reçetesi (ana iş):** `tionswarm-autonomous-ops/SKILL.md` → yeni **§10 "The
  autonomous boot sequence"** (Step 0 Orient → Step 5 Close); referans setup'a `0.` adımı
  ve Pitfalls'a "Skipping the boot sequence" maddesi. Reçete `.tionswarm/progress.json`
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

Anthropic *Effective context engineering* makalesi: token arttıkça recall hassasiyeti düşer ("context rot", `n²` dikkat ilişkisi → **performans gradyanı**, uçurum değil). TionSwarm'nun önceki "her şeyi ham tut" bahsi (512K/0.6) bu rot ile bilinçli bir takastı. Dayanıklılığın aslında **retrieval katmanında** (memory/`conversation_search`/core blocks) olduğu, ham pencere boyutunda olmadığı tespit edildi → ham pencere küçültülebilir, recall kaybetmeden.

- **Adaptif fraction:** `providers.AdaptiveBudgetFraction(provider, model)` — `ContextWindowFor`'un aile sınıflamasını yeniden kullanır; Opus/Sonnet 0.45, Haiku/Fable 0.40, MiniMax/DeepSeek/Gemini 0.35, bilinmeyen 0 (caller fallback).
- **Yeni semantik:** `ContextBudgetFraction = 0` → **otomatik/adaptif** (pozitif = manuel sabit). `EffectiveBudget` `fraction<=0`'da adaptif tabloyu kullanır; `Manager.SetBudgetShape` artık 0'ı (auto) saklar; `store.go` validate 0'ı korur (negatif → 0).
- **Yeni varsayılanlar:** `ContextBudgetCeil` 512K→**256K** (`262144`), `ContextBudgetFraction` 0.6→**0 (auto)**, `memoryPressureWarn` 0.75→**0.70** (`settings.go`+`tunables.go`). Eski `0.6` persisted değer manuel sabit olarak yaşar; yeni kurulum adaptif başlar.
- **Frontend:** Ayarlar▸Bağlam "Pencere oranı"/"Bütçe tavanı" hint'leri auto+rot açıklamasıyla güncellendi (`appPanels.tsx`).
- **Test:** `budget_test.go` (`TestEffectiveBudgetAdaptive`) + `context_window_test.go` (`TestAdaptiveBudgetFraction`); `go build ./...` + `go test ./internal/conversation ./internal/providers ./internal/settings` yeşil (83 test).
- **Doküman:** `_Docs\17` yeni **§12** (takas analizi + strateji + tablolar + mermaid) + §7 çapraz-referans; `tionswarm-settings` skill + `tionswarm-project` skill güncellendi.

## Context Reset + Handoff Artifact ✅ (2026-06-25)

Anthropic "harness design for long-running apps" bulgusu: in-place compaction tek başına **"context anxiety"**yi (model limite yaklaşınca erken toparlama) çözmez. Çözüm = **context reset** + **handoff artifact**: pencereyi özetlemek yerine, devamı taşıyan bir handoff dosyası yazıp **temiz bir oturumda** sürdür. TionSwarm'da önceden yalnız in-place rolling-summary vardı; bu, onun opt-in tamamlayıcısı.

- **Çekirdek:** `internal/conversation/handoff.go` (`handoffPrompt` 10-bölüm + DONE/TODO + Next Step, `HandoffEnv`, `BuildHandoff` — compaction çekirdeğini `KindCompact` ile yeniden kullanır) + `internal/agent/handoff.go` (`HandoffSession`: üret→artifact yaz→(ops.) `<workdir>/.tionswarm/handoff.md`→`SpawnSession(ParentSessionID)` ile taze oturum→tombstone; `maybeAutoHandoff`/`handoffChainDepth`/`handoffEnv`/`buildContinuationPrompt`).
- **Üç tetik:** manuel `/handoff` (`POST /api/sessions/{id}/handoff` → `summary.go handleSessionHandoff`); ajan aracı `handoff_session` (`tools/builtin_handoff.go`, self-manage gated, `toolsetup.go` kapanışı); **otomatik** (yalnız otonom tur — `runSpawn`/`deliverPrompt` tur-sonu; overflow sinyali `callkind.go withOverflowFlag`/`markContextOverflow`, tetik `toolloop.go` reactive compaction'da).
- **Ayarlar:** `HandoffAuto` (vars. **kapalı**) / `HandoffPressure` (0.90) / `HandoffMaxChain` (20) / `HandoffWriteFile` (kapalı) — `settings.go`+`store.go` clamp + `server.go applySettings → tun.SetHandoff`; UI Ayarlar▸Bağlam "Context reset (handoff)" bölümü. Tunables `DefaultHandoffPressure`/`DefaultHandoffMaxChain`.
- **Soyağacı:** `db.Session.ParentSessionID`/`HandoffArtifactID` (+`SetSessionHandoffArtifact`); `GET .../info` döner; UI `SessionDetailPanel` "↩ Devraldığı oturum" tıklanır link + `useChatStream` `/handoff` komutu yeni oturuma geçer.
- **Test:** `conversation/handoff_test.go` (env+transcript enjeksiyonu, boş-transcript guard) + `agent/handoff_test.go` (chain-depth, `maybeAutoHandoff` no-op yolları, continuation prompt, title). `go build`/`vet` temiz; `go test ./internal/conversation ./internal/agent` yeşil (mevcut paralel `TestEffectiveBudget` WIP'i hariç). Frontend `tsc`+`vite build` yeşil.
- claude-cli `--resume` ile uyumlu (reset zaten cold yeni oturum açar). Detay: **`_Docs\35-CONTEXT-RESET-HANDOFF.md`**.

## Market — katman sadeleştirme: bundled + workspace tier'ları kaldırıldı ✅ (2026-06-25)

Market pack katmanları üçten (bundled/global/workspace) **bir yerele** (global) indirildi; uzak registry 4. kaynak olarak kalır.

- **Kaldırılanlar:** `internal/market/defaults.go` (`//go:embed defaults`) + `internal/market/defaults/` klasörü + `EnsureDefaults` çağrısı (runtime.go) + workspace pack tier'ı. Binary artık market item taşımaz, workspace'te `market/` klasörü oluşmaz.
- **`market.New(globalDir, ledgerDir)`:** tek yerel tier = global; `Publish` global dizine yazar; install ledger (`installed.json`) per-workspace **kökte** (eski `<workspace>/market/` yerine; `workspaceLedgerDir`). Store'da `writeDir` → `globalDir`+`ledgerDir` ayrımı.
- **Mevcut paketler korundu:** 28 başlangıç paketi zaten global dizinde (`~/.tionswarm/market`); silinmedi. Yeni kurulumlarda market boş başlar → global'e elle paket konur veya uzak registry eklenir.
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
  başlık. Katlı gruplar `localStorage` (`tionswarm.skillsCollapsedGroups`) ile kalıcı. `SkillEditor`
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
- **Git hijyeni:** `.gitignore`'a `*.log.err`/`*.err`; sızan `vite-run.log.err`/`tionswarm-run.log.err`
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
  TionSwarm'ya eşler — name/description/when_to_use→aynı, `allowed-tools`→`always_allow`, `paths`→koşullu,
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

Sorun: self-management araçları (ajanın TionSwarm'yu kontrol eden tool'ları) kodda
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
- Doc: `tionswarm-self-management` SKILL.md güncellendi. Test: `builtin_skillmgmt_test.go`
  (partial forwarding + validation). Toplam testler yeşil.

## Dış-ajan otomasyon dostluğu — UI seçicileri + API rehberi ✅ (2026-06-23)

Soru: "TionSwarm'yu dışarıdan ajanlar (chrome-mcp/playwright-mcp) baştan sona kullanabilir mi,
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

**Bilinçli ertelendi:** fork/cache (TionSwarm özetleyiciye yalnız katlanan dilimi yollar →
çağrı zaten ucuz, fork'un çözeceği pahalılık yok; claude-cli cache paylaşımını kontrol edemez).

## Post-compact kurtarma işaretçisi — Claude Code parite 2. faz ✅ (2026-06-23)

CC compact sonrası transcript pointer + son okunan dosya re-injection yapar. TionSwarm'ya
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

> Sıradaki kesin adım: TionSwarm'yu yeniden derleyip flow'u tekrar çalıştır →
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
Hata "ne yapmalı" demediği ve `tionswarm-flows` skill'i node JSON şemasını hiç
belgelemediği (sadece soyut "steps/edges" anlatıyordu) + `create_flow` örneklerinde
paralel örnek olmadığı için agent doğru şemayı bulamadı, sıralı flow'a düştü.

**Çözüm — kendini düzelten tool hataları:** `validGraphJSON` artık her graph
hatasına kısa, eyleme dönük bir ipucu ekliyor (`graphSchemaHint` + `nodeSchemaCheat`):
hatadaki imzaya göre ("Node.nodes.branches", "has no children", "must be an agent
node" vb.) doğru alanı 5-6 kelimeyle söyler — ör. *"parallel fan-out uses
parallel:[...],joinNext — not branches/next"*. Ayrıca `create_flow`'a paralel
fan-out+join örneği ve `tionswarm-flows` skill'ine node-tipi/alan tablosu + paralel
örnek eklendi. (`builtin_flowmgmt.go`, `skills/defaults/tionswarm-flows/SKILL.md`.)
Doğru paralel şema: `{type:"parallel","parallel":["a","b"],"joinNext":"merge"}`
(çocuklar agent node id'leri; `branches`/`next` DEĞİL).

> Not: SES2'de turn 19'daki `provider error: claude CLI failed: exit status 1`
> ayrı bir provider/CLI çöküşüdür (tool hatası değil, detay loglanmadı) — bu fix
> kapsamı dışında.

## 3 self-management iyileştirmesi: create_agent skills + run_schedule + autonomous artifacts ✅ (2026-06-23)

Üç kullanıcı isteği tek turda:

**1) `create_agent` artık skill atayabiliyor.** Yeni opsiyonel `skills` (slug dizisi)
parametresi; verilmezse yeni agent **default TionSwarm skill seti** ile tohumlanır
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
build+vet+gofmt temiz. Skill `tionswarm-self-management` güncellendi (+ on-disk senkron).

---

## Self-management araçları prompt'tan gizlendi → skill katalog oldu (hidden-lazy tier) ✅ (2026-06-23)

**Sorun:** Self-management araçları zaten lazy'di (şema yok), ama ~40+ aracın **isim+özet
satırı** her turun "Available Tools (load on demand)" bloğunda (cached prefix) yer alıyordu —
gereksiz token. **Çözüm:** lazy araçlara **hidden** alt-katmanı eklendi; self-management suite
artık blokta **listelenmez**, yerine `tionswarm-self-management` skill'ine yönlendiren tek satır
durur. Araçlar aktive-edilebilir ve aranabilir kalır.

- **`internal/tools/registry.go`:** yeni `hidden map[string]bool` (hidden ⊆ lazy) +
  `MarkHidden(names…)`; `VisibleLazyCatalog(allow)` (= LazyCatalog − hidden, blok için);
  `HiddenLazyCount(allow)`. `LazyCatalog` (activate_tools/tool_search kaynağı) **tüm** lazy'yi
  döndürmeye devam eder → hidden araçlar aktive/aranabilir.
- **`internal/agent/toolsetup.go`:** self-management suite (`builtins[selfManageStart:]`)
  `MarkLazy` yerine **`MarkHidden`**. `LazyToolsCatalogBlock` artık `VisibleLazyCatalog` +
  `HiddenLazyCount` kullanır; `renderLazyToolCatalog(visible, hiddenCount)` hiddenCount>0 ise
  "**N self-management tools … not listed here … load the `tionswarm-self-management` skill … or
  `tool_search`**" pointer satırını basar.
- **Keşif yolu:** Available Skills bloğu `tionswarm-self-management` skill'ini zaten ilan ediyor
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
- **Pencere dışı**: tab başlığı `(N) TionSwarm` (odak dışıyken) + taskbar/dock
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
   stdout + `io.MultiWriter` ile `<dataDir>/logs/tionswarm.log` (append, best-effort).
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
131 test yeşil (cmd/tionswarm-desktop WIP hariç). Detay: `_Docs\07-CHAT-UX.md`,
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
241 test yeşil (cmd/tionswarm-desktop'taki ilgisiz WIP hariç). Detay:
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
  (`TIONSWARM_MCP_POOL_TTL_SEC`, vars. 60sn) — listChanged göndermeyen sunucular için.
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

TionSwarm iki modeli birden taşıyor: paylaşılan-thread (etiketleme+coalesce ile sağlamlaştırıldı)
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
   TionSwarm `tools/list`'i yeniden çağırmaz.
2. **Dial-per-operation (havuzsuz):** `BuildCatalog`/`CallNamespaced` her işlemde **yeni
   session** açıp kapatır. Gateway'in `activate_tools`'u **oturum-kapsamlıdır** → araçları
   o anlık session'a ekler, session `Close()` ile kapanınca kaybolur. Eklenen araçlar
   TionSwarm'nun kataloğuna hiç girmez → çağrılamaz.

→ Sonuç: **runtime'da araç ekleyen/çıkaran MCP sunucularıyla TionSwarm uyumsuz.**

**Geçici çözüm (uygulandı):** İstenen araçlar sunucunun bağlantı URL'indeki **preset'e**
konur; preset her taze session'da başlangıçta yüklendiği için dial-per-operation modeliyle
sorunsuz çalışır. MCP Gateway `tionswarm` preset'ine `mcp-chrome` eklendi
(`mcp-server/config.json`: `tionswarm: [<remote-service>, mcp-chrome]`); `?preset=tionswarm` artık
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
   (`tionswarm:draft:<sessionId>`). Oturum değiştirip dönünce ve sayfa yenilenince korunur;
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
İzolasyon: `TIONSWARM_DATA_DIR` temp'e yönlendirilir → gerçek `~/.tionswarm` skill/market seed'ine dokunulmaz.
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
çözülen `ThemePreset`/`Theme`/`Accent`'i verir; `cmd/tionswarm-desktop/titlebar_windows.go` 8 curated paletin
bg/text/border'ını (`themePresets.ts` ile elle senkron) COLORREF'e (`0x00BBGGRR`) çevirir. Bilinmeyen
preset → yalnız dark/light frame (caption rengi atlanır); eski Windows'ta desteklenmeyen attribute'lar
sessizce yok sayılır (pencere yine çalışır). **Canlı güncelleme:** `watchTitleBar` 1.5sn poll ile tema
değişince `w.Dispatch` üzerinden yeniden uygular (Ayarlar'dan palet değiştirince başlık anında uyar).
✅ `go build ./...`/`vet` yeşil; desktop canlı (varsayılan midnight-violet, pencere açıldı, health 200,
panik/hata yok). Detay: [32-NATIVE-PENCERE.md](32-NATIVE-PENCERE.md).

## Native masaüstü penceresi — WebView2 (CGO'suz) ✅ (2026-06-22)

TionSwarm artık tarayıcı yerine **kendi masaüstü penceresinde** açılabiliyor. Plan:
[32-NATIVE-PENCERE.md](32-NATIVE-PENCERE.md). **Ön koşul refactor (davranış-korumalı):**
`cmd/tionswarm/main.go`'nun boot dizisi yeni **`internal/app`** paketine taşındı
(`SetupLogging` + `Bootstrap`/`Serve`/`Shutdown`/`Addr`/`URL`); `Bootstrap` artık listener'ı
önden açar (`net.Listen`, `:0` boş port desteği) ve `SetBaseURL`'i çözülen adresle çağırır.
`main.go` ~130→~50 satır. **Yeni giriş noktası** `cmd/tionswarm-desktop` (`//go:build windows`,
[`jchv/go-webview2`](https://github.com/jchv/go-webview2) — **saf Go, CGO yok**; Win11'de
yerleşik WebView2 runtime): sunucuyu `127.0.0.1:0`'da başlatır, `waitForHealth` ile hazır olunca
1280×800 WebView2 penceresi açar, pencere kapanınca graceful `Shutdown`. WebView2 yoksa →
varsayılan tarayıcıya fallback (`rundll32 url.dll`). `!windows` stub mevcut. `scripts/build.ps1`
`-Desktop` bayrağı (`-H windowsgui` → konsolsuz). Bağımlılık: `go-webview2` (direct) +
`go-winloader`/`x/sys` (indirect) — yalnız desktop hedefinde derlenir; **başsız `tionswarm`
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

**Sorun:** Yerel MCP Gateway'de saniyeler içinde 4 ayrı `tionswarm` session açılıyordu
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
- **TTL:** varsayılan **60 sn** (`TIONSWARM_MCP_CATALOG_TTL_SEC` ile override; `0` =
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
  ajanlarda araç gerçekten `use_skill`; ama **claude-cli** ajanlarında TionSwarm built-in'leri
  Interaction MCP köprüsünden **namespaced** geliyor: `mcp__tionswarm_interaction__use_skill`.
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
- **Phase 2 (tokenLimitFor) bilinçle ertelendi:** model penceresine ölçekleme TionSwarm'nun
  12K transcript bütçesiyle çelişir (bir tool sonucu tüm bütçeyi aşar); doğru hamle
  "modele göre akıllı varsayılan bütçe". Detay: `17-TOKEN-OPTIMIZASYON.md` §6.

---

## CG-9 ikinci yarı — bütçe-orantılı tool eşikleri ✅ (2026-06-22)

**Hedef:** the external agent project'ın `tokenLimitFor` (tool-result eşiği context window'a göre)
deseninin TionSwarm karşılığı. Model context-window metadata'sı yok (`ModelInfo`
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
- **Doğrulama:** `tsc --noEmit` temiz. Skill `tionswarm-settings` zaten tüm alanları
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

- **`tionswarm-guide` SKILL.md → "Before you build: discover first" bölümü:** uygulamadan
  ya da "bu yok" demeden önce **search → read → confirm → extend** disiplini;
  absence iddiası ancak gerçekten arandıktan sonra ("Y ve Z için grepledim, bulamadım"),
  ve sıfırdan yazmak yerine mevcudu genişletme kuralı. Kod/konfig/agent/flow/skill/
  memory — hepsine uygulanır.
- **`tionswarm-self-management` → "Prefer reading first" güçlendirildi:** entity
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
- **Canlı duman testi (8088):** `GET /api/fs/browse` ✓; bir oturuma TionSwarm deposu
  set edildi → `{exists:true,isGitRepo:true,branch:"main"}` ✓ (git branch tespiti),
  sonra sıfırlandı. **Not:** varsayılan 8080 portu mcp-for-unity backend'iyle
  çakıştığı için bu örnek `TIONSWARM_ADDR=127.0.0.1:8088` ile çalışıyor.

## fs/shell sandbox kilidi kaldırıldı (kilitsiz dosya/komut erişimi) ✅ (2026-06-22)

**Hedef:** Built-in dosya/komut araçlarının workspace dizinine kilitli olma
güvenlik kısıtını kaldırmak — the external agent project (external-agent-oss) gibi araçların makinedeki
herhangi bir yola erişebilmesi, güvenliği tek başına **izin moduna** bırakmak.

Yapılan:


---

> **Daha eski kayitlar (2026-06-19 ve oncesi) arsivlendi:** [05-ARSIV.md](05-ARSIV.md).

- **`internal/tools/sandbox.go`** — `Sandbox`'a `Confined bool` eklendi.
  - `NewSandbox(dir)` artık **kilitsiz**: mutlak yollar olduğu gibi kabul edilir,
    göreli yollar `Root`'a (boşsa süreç cwd'sine) göre çözülür, `..` kaçışı serbest.
  - `NewConfinedSandbox(dir)` eski **katı** davranışı korur (mutlak + `..` reddi).
  - `Resolve` iki kola ayrıldı; `Root` artık bir sınır değil, sadece göreli yol tabanı.
- **`builtin_fs.go` / `builtin_shell.go`** — araç açıklamaları "mutlak yol veya çalışma
  dizinine göreli" olacak şekilde güncellendi; shell'in `Ready()` kapısı kaldırıldı
  (Root boş olsa bile süreç cwd'sinden çalışır).
- **`internal/agent/toolsetup.go`** — fs/shell sandbox'ı `NewSandbox` (kilitsiz);
  **config araçları** bilinçli olarak `NewConfinedSandbox` ile `<workspace>/config/`
  içine **kilitli kaldı** (ajan yalnızca kendi config'ini düzenler).
- **Testler** — `builtin_fs_test.go` (kaçış/mutlak artık serbest), `builtin_config_test.go`
  (kilitli sandbox kullanır) güncellendi. `go build` + `go test ./...` yeşil.

**Güvenlik notu:** Artık tek koruma katmanı izin modu (salt-okunur / sor / otomatik).
Detay: `_Docs/06-WORKSPACES.md` "fs/shell artık kilitli DEĞİL" notu.

## Faz A2 — Generic ajan yürütme çekirdeği + alt-ajan izolasyonu ✅ (2026-06-19)

**Hedef:** Ana bağlamı kirletmeden izole, tipli, paralel alt-ajanlar başlatabilmek;
eski çok-primitifli yapıyı tek generic çekirdeğe indirmek.

Yapılan (A2.0 → A2.4 tamamlandı):

- **`run_subagent` aracı** (`internal/tools/subagent.go`) — tek generic agent-to-agent
  primitifi. Parametreler: `target` (profil id ya da mevcut ajan adı/id), `task`,
  `wait` (`"sync"` varsayılan | `"async"` detached), `context` (`"isolated"` varsayılan |
  `"inherited"`), `model` (opsiyonel). `enableDelegation` ile gated; native-only (CLI
  ajanları async için köprülenmiş `SpawnSession` kullanır).
- **Profiller** — `explore` (salt-okunur arama), `coder` (kod yaz/düzenle), `reviewer`
  (salt-okunur inceleme). Ephemeral geçici worker; kalıcı session açmaz.
- **Paralel fan-out** — tek turda birden çok `run_subagent` çağrısı goroutine + WaitGroup
  ile eşzamanlı koşar; ortak atomik bütçe sayacı ve eşzamanlılık slotları paylaşılır.
- **`subagent` StepKind + UI** — `agent/trace.go`'da `StepSubagent`; `TurnStep.SubSteps`
  ile alt-ajan izi parent'a gömülü; `SubagentStep.tsx` katlanabilir iç içe ajan kartı.
- **Birleştirme/temizlik (A2.4):**
  - `call_agent` aracı **kaldırıldı**; işlevselliği `run_subagent` (`wait:sync,
    context:inherited`) ile karşılanır.
  - `send_agent_message` aracı + `SendAgentMessage` metodu **tamamen kaldırıldı**
    (heartbeat gidince inbox'ı işleyecek mekanizma kalmamıştı → ölü mektup).
  - `spawn_session` **native ajan tool'u olarak kaldırıldı** (yerine `run_subagent`
    `wait:async`); `SpawnSession` runtime metodu + `POST /api/sessions/spawn` HTTP
    uç noktası + Interaction MCP CLI köprüsü + UI "Başlat" butonu **korundu**.
  - Heartbeat kaldırılmasının doküman izleri temizlendi (`24-SELF-MANAGEMENT`,
    `22-SPAWN-SESSION`, skill'ler).
- **Guard'lar:** depth (varsayılan 3), tur-başı bütçe (varsayılan 8), cycle/visited,
  eşzamanlılık (`SpawnMaxConcurrent`).
- `go build`/`go vet`/`go test ./...` + frontend `tsc -b` yeşil. Detay: `_Docs/25-SUBAGENT-ISOLATION.md`.

## Heartbeat (ajan-başına otonom wake ticker) tamamen kaldırıldı ✅ (2026-06-19)

Heartbeat alt sistemi **schedules** (cron) ile fazlalık olduğu için uygulamadan
tamamen çıkarıldı. `internal/agent/worker.go` (worker/WorkerStatus/tick/`runHeartbeat`
döngüsü) ve `internal/api/runtime.go` (worker status/wake/heartbeat endpoint'leri)
**silindi**; `Runtime.Start/Stop/Wake/Status/StartConfigured/StopAll`,
`emitHeartbeatFailure`, `KindHeartbeat`, `UsageKindHeartbeat`, `db.UpdateHeartbeat`,
`GetOrCreateHeartbeatSession`, `Agent.Heartbeat*` alanları, `settings.DefaultHeartbeatSec`
ve `create_agent` heartbeat girdileri kaldırıldı. `delegate.go`'daki best-effort
`r.Wake` dürtme çağrısı düştü (mesaj inbox'ta sıraya kalır). Frontend: relation-graph /
execution / budget / notify kind'leri ve "Nabız"/"heartbeat" etiketleri sadeleşti;
`defaultHeartbeatSec` ayarı UI'dan çıkarıldı. **Korunanlar:** schedules (cron),
`schedule_wake` (sohbet-içi self-wake), `send_agent_message`/inbox, `pauseAutonomy`
(artık yalnız zamanlamalara işaret eder). DB güvenliği: diskteki ajan JSON'larında kalan
`heartbeat*` alanları zararsız (bilinmeyen alanlar unmarshal'da yok sayılır, migrasyon
gerekmez). Kapılar: `go build`/`go vet`/`go test`/`tsc --noEmit` hepsi yeşil.

## schedule_wake UX: bekleme banner'ı + async ask_user + iterasyon limiti ✅ (2026-06-19)

**İstek:** "ScheduleWake denerken agent beklerken UI işlemi bitmiş gibi görünüyor —
Durdur/Sıraya al/Steer gibi komutlar gözükse iyi olur. Ayrıca agent `ask_user` denerken
'only available in interactive chat sessions' hatası alıyor. Araç döngüsü limitini 3 katına çıkaralım."

Üç değişiklik (backend `go build`+testler yeşil, frontend `tsc -b` yeşil):

1. **Bekleme durumu UI'ı + Durdur** — `schedule_wake` arming sonrası `Runtime.ScheduleWake`
   artık `phase=armed` (`reason`+`fireAt` taşıyan) bir `chat` olayı yayar; frontend bir
   **bekleme banner'ı** (canlı geri sayım) + **Durdur** kontrolü gösterir → oturum artık
   "bitti" gibi görünmez. Durdur `POST /api/chat/wake/cancel` → `Runtime.CancelWake` bekleyen
   one-shot satırı siler, timer'ı iptal eder, `phase=cancelled` yayar. Yaşam döngüsü:
   armed→start→done / cancelled. (Bekleme sırasında çalışan tur olmadığından steer/queue
   uygulanmaz; kullanıcı mesaj yazarsa yeni tur olur, banner düşer.)
   Yeni: `WakeWaitBanner.tsx`, `useChatStream` `wakeWaits`/`cancelWake`, `emitWakePhase`/`CancelWake`.
2. **Async sohbette `ask_user`/`request_confirmation`** — wake turu `tools.WithAsyncChat` ile
   işaretlenir; bu araçlar artık "proceed without asking" yerine modele **"sorunu yanıtın olarak
   yaz, turu bitir; kullanıcı sohbette yanıtlar"** der. Headless koşular eski davranışı korur.
   (`tools/ask.go` `WithAsyncChat`/`IsAsyncChat`, `builtin_ask.go`, `builtin_confirm.go`, `scheduler.go`.)
3. **Araç döngüsü limiti 8 → 24** (3 kat) — `toolloop.go` `maxToolIters`; `TIONSWARM_MAX_TOOL_ITERS`
   env ile override edilebilir.

Detay: `_Docs\20-SCHEDULE-WAKE.md`.

## Tasarım tutarlılığı turu — tema token'ları + DRY (4 faz) ✅ (2026-06-19)

**İstek:** "Uygulamaya birçok yeni feature eklendi; tema tutarlılığı ve tasarım kodunu daha düzenli yapabileceğin yerleri tara, hepsini adım adım yap."

İki Explore ajanıyla frontend denetlendi; bulgular 4 faza bölünüp uygulandı (her faz ayrı commit, yalnız ilgili dosyalar — eşzamanlı WIP'e dokunulmadı):

- **Faz 1 — Semantik durum renkleri token'a çekildi** (`b94e16d`): Sabit Tailwind sınıfları (`text-green-400`/`red-400`/`amber-500`/`emerald-500` vb.) `var(--color-success/danger/warning)`'a çevrildi → açık tema dahil doğru re-theme. 12 dosya (RunView, FlowsPanel, TaskDetailPanel, TaskBoard, DependencyPicker, PermissionPrompt, SessionDetailPanel, appPanels, SecretsPanel, MemoryPanel, BoardColumnEditor, WorkspaceCreateModal). Kasıtlı kontrast/içerik `white`/`black` kullanımları bilerek bırakıldı.
- **Faz 2 — Ortak UI bileşenleri** (`62570a3`): `components/common/` eklendi — `Button` (primary/secondary/danger × sm/md/lg), `IconButton`, `Card`, `Badge` (tone-eşlemeli pill), `SectionHead`, `EmptyState`. İlk adoption: MemoryPanel + SecretsPanel. Geniş adoption kademeli.
- **Faz 3 — Kategorik renkler `lib/palette.ts`'te toplandı** (`36762b8`): Dağınık "kategori → renk" haritaları (Budget çağrı-türü, oturum bağlam-rolü) tek dosyaya alındı. Bunlar tema-bağımsız *veri* renkleri. `relationGraph` renkleri eşzamanlı WIP olduğundan dokunulmadı.
- **Faz 4 — Büyük panellerden ayrıştırma** (`13dfcfe`): Kendi kendine yeten parçalar ayrı dosyaya alındı — ArtifactsPanel meta/yardımcılar → `artifactMeta.tsx` (602→532), ToolsPanel ad-çözümleme/şema-düzleştirme → `toolMeta.ts` (580→534). FlowsPanel/Composer'ın derin yapısal bölünmesi (state çözme + görsel QA gerektirir) ertelendi.

✅ Her faz `tsc -b` + `vite build` yeşil. (Görsel Chrome testi bu turda yapılamadı.)

## Workspace WS-prefix + eski UUID migration aracı ✅ (2026-06-19)

İnsan-okunabilir kimlik şemasının devamı (aynı gün, ikinci tur):

- **Workspace ID → `WS<n>`** (`internal/workspace/manager.go` + yeni `id.go`).
  Yeni workspace'ler `WS1`/`WS2` alır. Sayaç `dataDir/ws-counter.json`'da kalıcı;
  `NewManager` sayaç + mevcut `WS<n>` maks'ını alıp rewind'i önler. ID dizin
  adıdır, entity'lere gömülü değildir (izolasyon). `uuid` importu kaldırıldı.
- **Migration aracı `cmd/migrate-ids`** (4 dosya: main/scan/rewrite/ws). Tek-seferlik,
  idempotent. **Token-replacement** stratejisi: eski→yeni ID haritası kurar, tüm
  metin dosyalarında birebir değiştirir (Flow.Graph gömülü ID'leri, Dependencies,
  content-file yolları dahil — alan-alan saymadan), sonra ID'li dosya/klasörleri
  (entity JSON, session klasörleri, artifact içerik dosyaları, uploads, usage)
  yeniden adlandırır, `counters.json`/`ws-counter.json`'ı ileri taşır.
  **Varsayılan dry-run**, `-apply` ile uygular (önce tam yedek). Mesaj + task Run
  ID'leri UUID kalır; referansları yine düzeltilir. Detay: `_Docs\08-DEPOLAMA.md`.
- **Geriye uyum:** araç çalıştırılmadan da eski UUID kayıtlar çalışmaya devam eder
  (opak string lookup). Araç istendiğinde tek seferde tümünü yeni şemaya çevirir.
- **UI/URL:** frontend ID formatı varsayımı **yok** (ID'ler opak) → kısa kodlar
  otomatik görünür; ekstra değişiklik gerekmedi.
- **Gerçek veride uygulandı (2026-06-19):** `~/.tionswarm` (4 workspace) migrate edildi
  → `WS1:MINIMAX`, `WS2:DenemeBilimsel`, `WS3:TionSwarm`, `WS4:OtonomOps`; toplam
  130 entity. Yedek: `~/.tionswarm-idbackup` (junction içeriği hariç). **Bulgu:**
  `OtonomOps` workspace'inin `workspace/` dizini `Desktop\Projects\url-shortener`'a
  bir **junction**'dı → araç junction-güvenli yapıldı (yedek atlar, rewrite yalnız
  `store/`). Çalıştırma için **uygulama kapatıldı**, sonra tek-binary build başlatıldı.
- **Doğrulama:** sentetik fixture'da dry-run+apply+idempotent re-run; gerçek veride
  her store `db.Open` ile temiz yüklendi, referans bütünlüğü tam (yetim ref=0,
  kalan UUID ref=0, 160+ mesaj tutarlı). `go build`/`go test ./...` yeşil.

## İnsan-okunabilir kimlik (ID) şeması — prefix + artan sayı ✅ (2026-06-19)

**İstek:** UUID yerine `D24` gibi harf-prefix + artan sayı kimlik mekanizması.

**Yapılan:** `internal/db` kimlik üretimi UUID'den **prefix'li monoton sayaca** geçti.
Yeni entity'ler `AGT3`/`SES42`/`TSK17`/`FLW5`/`RUN128`/`ART9`/`MEM88`/`MCP4`/`HOK2`/`SCH7`
biçiminde kimlik alır (prefix haritası + mekanik: `_Docs\08-DEPOLAMA.md`).
- **Merkez:** `DB.nextID(prefix)` (`db.go`) — prefix-başına sayaç, her tahsiste
  `counters.json`'a atomik yazılır; kendi mutex'i (`countersMu`) sayesinde hem
  `mu` öncesi hem de `mu` tutulurken güvenli.
- **Tekrar-kullanımsız:** sayaç yalnız artar; silme/restart numarayı geri vermez
  (`load()`→`loadCounters()`).
- **Geriye uyum:** eski UUID kimlikler dokunulmadan çalışır (opak string lookup,
  iki şema yan yana). Eski id'ler taranmaz; prefix'li id'ler bir UUID ile çakışamaz.
- **İstisna:** mesajlar + task `Run` ID'leri UUID'de kaldı (hot-path / legacy,
  UI'da görünmez); geçici tanımlayıcılar (chat runID/replyID, spawn SourceID,
  upload) de UUID. (Workspace ID'leri sonraki turda `WS<n>`'e geçti — yukarı bakın.)
- **Test:** `internal/db/id_test.go` (`TestNextIDMonotonicAndNoReuse`) — monotonluk +
  sil/yeniden-aç sonrası reuse olmadığını doğrular. `go build`/`go test ./internal/db ./internal/tools` yeşil.

## Tek binary — frontend `//go:embed` ile gömüldü ✅ (2026-06-19)

Proje artık **tek executable** olarak dağıtılabiliyor (projenin "tek binary, çapraz
platform" hedefiyle birebir uyumlu). Yeni `internal/web` paketi (`embed.go`) Vite build'ini
`//go:embed all:dist` ile gömer ve **SPA fallback'li** bir handler sunar (gerçek dosyalar
yol bazlı; kök + bilinmeyen yollar → `index.html`). `server.go` `registerWebRoutes` ile köke
(`/`) bağlar — `/api/*`·`/health`·`/mcp/*` Go mux'ında daha spesifik olduğundan önceliği
korur. Build bundle edilmemişse (`dist` yalnız `.gitkeep`) `Handler` `ok=false` döner →
binary yine derlenir, UI sunulmaz, log "frontend not bundled" der (dev/Vite-proxy akışı
bozulmaz). Vite `outDir` → `../internal/web/dist` + `emptyOutDir:true`; `.gitignore`
`/internal/web/dist/*` (placeholder hariç). Yeni `scripts/build.ps1`: UI build + UI gömülü
`go build -trimpath -ldflags "-s -w"` → `tionswarm.exe` (~12 MB), build sonrası `.gitkeep`
geri konur. ✅ `go build`/`vet` yeşil; **canlı smoke** (izole instance, port 8097):
`/`→HTML 200, `/health`→JSON 200, `/api/version`→200, `/agents`→index.html fallback 200,
`/favicon.svg`→200. README "Tek Binary (üretim)" bölümü eklendi.

**Repo temizliği (aynı oturum):** kök + frontend'deki ~43 MB yerel artefakt
(eski `tionswarm*.exe`, `*.log`, `_agentid.txt`, vite logları) silindi — tamamı zaten
`.gitignore`'da, git'te izlenmiyordu. Kök artık yalnız kaynak + `_Docs` içerir.

## Otonom Ops skill'i + Link Kısaltma workspace template'i ✅ (2026-06-19)

**İstek:** Uzman "vibe coding" otomasyon sistemini (video altyazısı) TionSwarm'ya uyarlayan detaylı bir skill; ve bu skill'i somutlaştıran, basit bir uygulama (URL kısaltma sitesi) üzerinde çalışmaya hazır bir workspace.

**Yapılan:**
- **Yeni default skill** — `internal/skills/defaults/tionswarm-autonomous-ops/SKILL.md` (`♻️ TionSwarm Autonomous Ops`, `access: shared`). Videodaki uzman desenlerini TionSwarm primitiflerine eşler: agent/provider çok-modelliği, kurallar (workspace config), skills, automations (**Schedules** = zaman trigger + **Hooks** = olay trigger), loops (otonom heartbeat runtime + Flows), paralellik (`spawn_session` + workspace izolasyonu), quality gates (izin katmanı + hooks) ve test/döküman/log "flywheel"ı. Worktree/git-merge sınırları dürüstçe belirtildi. `//go:embed defaults` ile otomatik seed olur (kod değişikliği gerekmez).
- **Yeni workspace template** — `internal/api/templates.go` → `linkshortener` ("Link Kısaltma (Otonom Ops)", 🔗). 4 ajan (Mimar/Plan, Geliştirici/Write, İnceleyici/Review, Bakım/Ops), **Plan → Yaz → İncele** çok-modelli flow'u, ve 3 **disabled** starter schedule (gece 01:00 docs sweep, 02:00 test coverage, 03:00 production error sweep). Ajan ruhları `tionswarm-autonomous-ops` skill'ini yüklemeye yönlendirir.
- **Doğrulama:** `go build ./...` + `go vet ./internal/api ./internal/skills` yeşil.
- **Not:** Template ajanları aynı default provider/model ile seed olur (`tmplAgent`'ta per-agent model alanı yok); gerçek çok-modelli pipeline için kullanıcı ajanlara UI'dan farklı model pinler.

## Ağ Canlı mod — flow run'ları run-tracker'a kaydedildi ✅ (2026-06-19)

**İstek:** Task/flow run'larını run-tracker'a kaydedip `runTarget` (aktif task/flow bağı) gerçekten canlansın.

**Yapılan:**
- **`graph.go`** çalışan-oturum kümesine `Runtime.ActiveSessionIDs()` eklendi (executions feed'iyle aynı; önceden yalnız `s.runs` = chat vardı). Böylece otonom (schedule/heartbeat/spawn) + flow run'ları grafikte "running" olarak görünür.
- **`flow.go RunFlowRecorded`**: flow oturumu çalışmadan önce `GetOrCreateSourceSession` ile çözülüp `trackSession`/`defer untrackSession` ile sarmalandı → flow çalışırken oturum aktif işaretlenir.
- **Sonuç:** flow çalışınca ilgili ajan `runKind=flow`, `runTarget=flow:<id>` raporlanır → Canlı modda **flow düğümüne aktif accent bağ + glow**, bitince temizlenir.
- **Doğrulama (uçtan uca):** DenemeBilimsel "Araştırma Akışı" çalıştırıldı; çalışırken `/api/graph` ajanı `running:true, runKind:'flow', runTarget:'flow:…'` raporladı; run bitince (lastStatus=success) running temizlendi. `go build`/`vet` yeşil.
- **Not:** Repoda doğrudan **task çalıştırma** yolu yok (task'lar flow-backed/otonom çalışır); doğrudan run path eklenirse aynı tracker'la otomatik kapsanır. Test, "deneme" amaçlı DenemeBilimsel workspace'inde bir başarılı flow run bıraktı.

## Silme — cascade temizliği (agent + skill) ✅ (2026-06-19)

**İstek:** Bir şey silindiğinde bağlı olduğu yerler de temizlensin (orphan
referans kalmasın).

**Tespit:** `DeleteAgent` yalnızca ajanı + sahip olduğu session'ları siliyordu;
ajana bağlı **schedule/task** ile run'lar orphan kalıyordu. **Skill** silinince
ajanların `Skills` listesindeki slug referansı duruyordu.

**Yapılan (`go build`/`vet`/`test` yeşil):**
- **`db/store.go` → `DeleteAgent` cascade'i genişletildi:** ajana bağlı
  schedule'lar (`AgentID`), ajanın sahip olduğu task'lar (`OwnerAgentID`) ve
  bunların run'ları da silinir. (Hook/Flow'da doğrudan `AgentID` alanı yok —
  bilerek dokunulmadı; flow ajanları graph içinde referanslanır.)
- **`db/store.go` → `RemoveSkillFromAgents(slug)` (yeni):** silinen skill slug'ını
  her ajanın `Skills` listesinden düşürür, değişen ajanları persist eder, güncellenen
  ajan sayısını döner.
- **Scheduler tazeleme:** `api.handleDeleteAgent` ve self-management `delete_agent`
  tool'u silmeden sonra `Scheduler.Reload` çağırır → kaldırılan schedule'lar canlı
  cron registry'sinden de düşer. `NewDeleteAgentTool` artık `reloadSchedules` parametresi
  alır; `agentDeps`'e eklendi, `toolsetup.go`'da `r.reloadSchedules` ile bağlandı.
- **Skill silme köprüsü:** `api.handleDeleteSkill` ve `agentSkillWriter.DeleteSkill`
  (artık DB taşır) skill silindikten sonra `RemoveSkillFromAgents` çağırır.
- **Testler (yeni):** `db/delete_cascade_test.go` — agent silmede schedule/task/run
  cascade + başka ajanın kayıtlarının korunması; `RemoveSkillFromAgents` slug strip.

## Düzeltme — workspace değişiminde panellerin bayat veri göstermesi ✅ (2026-06-19)

**Bug:** Sol üstten workspace değiştirince (veya yeni workspace oluşturup ona
geçince) Flows/Tasks/Schedules/Executions/Network/Artifacts/Memory/Secrets/Skills/
Market/Budget/Logs panelleri **eski workspace'in verisini** gösteriyordu; sayfa
yenileyince düzeliyordu.

**Kök neden:** Bu paneller verisini **mount anında** çekiyor (`useEffect([])`).
Workspace değişince `activeWorkspaceId` değişiyor ama `view` aynı kaldığından panel
remount olmuyor → mount-effect tekrar çalışmıyor → bayat veri. (App seviyesindeki
agents/sessions zaten `activeWorkspaceId` effect'iyle yenileniyordu; sorun yalnız
kendi verisini çeken panellerde.)

**Çözüm (`App.tsx`):** `<main>` öğesine `key={activeWorkspaceId}` verildi → workspace
değişince tüm panel ağacı **remount** olur ve verisini yeni workspace başlığıyla
yeniden çeker. Mevcut "paneller kendi mount'unda yüklenir" tasarımıyla uyumlu;
gelecekteki workspace-scoped panelleri de otomatik kapsar. `tsc`/`vite build` yeşil.

## Sağlayıcılar — her sağlayıcıya bağlantı testi butonu ✅ (2026-06-19)

**İstek:** Bağlantı testi butonlarını ekle (önce yalnız varsayılan sağlayıcıda vardı).

**Yapılan (`go build` + `tsc`/`vite build` yeşil):**
- **Per-provider test:** `ProvidersPanel`'de Anthropic, MiniMax ve OpenRouter
  bölümlerinin her birine "Bağlantıyı test et" butonu + canlı sonuç rozeti
  (yeniden kullanılabilir `TestConnection` bileşeni). Anahtar yoksa buton pasif.
- **Doğru model ile prob:** `test-provider` ucu artık opsiyonel `model` alır
  (`testProviderReq.Model`); boşsa global `DefaultModel`'e düşer. Per-provider
  butonlar temsilî bir model gönderir (MiniMax → `MiniMax-M3`, OpenRouter →
  `anthropic/claude-sonnet-4.6`) çünkü global varsayılan model genelde başka bir
  sağlayıcıya aittir (Anthropic id'si MiniMax/OpenRouter'ı test edemez).
- **Frontend:** `api.testProvider(provider, model?)`, `SettingsPanel.runTest`
  imzası model parametresiyle genişletildi; varsayılan sağlayıcı butonu aynen çalışır.

## Sağlayıcılar — OpenRouter built-in + MiniMax model listesi genişletildi ✅ (2026-06-19)

**İstek:** Ajan ayarlarında model seçerken MiniMax modelleri eksik; OpenRouter
hiç yok. OpenRouter'ı ekle (~25 güncel popüler model yeterli).

**Yapılan (`go build`/`vet`/`test ./...` + `tsc`/`vite build` yeşil):**
- **OpenRouter yeni built-in kind** (`internal/providers/kind_openrouter.go`):
  plugin desenini izleyen self-registering `kind_*.go` (Order 4). OpenAI-uyumlu
  `OpenAICompat` istemcisini `https://openrouter.ai/api/v1` ile sarar → tool-use +
  streaming + native ajan döngüsü. **25 küratörlü model** (Claude Opus 4.8/4.7,
  Sonnet 4.6, GPT-5.5, Gemini 3.5 Flash, DeepSeek V4, Grok 4.3, Kimi K2.6, Qwen3.7,
  Nemotron 3 Ultra, GLM 5.2, MiniMax M3, MiMo V2.5 …) — OpenRouter'ın canlı
  `/models` kataloğuna karşı **doğrulanmış** slug'lar. `AllowCustomModel=true`.
- **Kendi API anahtarı:** `settings` (`openrouterKeyEnc` AES-GCM + `openrouterBaseUrl`),
  DTO (`openrouterKeySet`), Patch (`openrouterKey` write-only), `store.go` Apply +
  `OpenRouterKey()` getter, `registry.go` (`openrouterKey`/`SetOpenRouter`/`resolve`),
  `api applySettings` → `SetOpenRouter`. `reservedProviderIDs`'e `openrouter` eklendi.
- **MiniMax listesi genişletildi** (`kind_minimax.go` + `kind_minimax_anthropic.go`):
  güncel **M3 (amiral)**, M2.7(+highspeed), M2.5(+highspeed) eklendi; M2.1/M2 korundu.
- **Frontend:** `ProvidersPanel`'e OpenRouter anahtar + base URL bölümü; `SettingsPanel`
  state/dirty/save + `clearKey`/`applyKey` union'ları `'openrouter'` ile genişletildi;
  `types/settings.ts` DTO+Patch alanları. Model seçici katalog-tabanlı → OpenRouter
  otomatik görünür.
- **Testler:** `kind_test.go` (4→5 entry + sıra), `customprovider_test.go` (id
  çakışması: `openrouter`→`myrouter`) güncellendi.
- **Doküman:** `tionswarm-settings` skill (yeni alanlar), `tionswarm-project` referans
  skill (sağlayıcı satırı).

## Ajan kontrol-yüzeyi — workspace CRUD araçları ✅ (2026-06-19)

**İstek:** Ajan kendi kendine uygulamayı kullanabiliyor ama **workspace
düzenlemeleri eksikti**; onları da ekle.

**Yapılan (`go build`/`vet`/`test` + `tsc`/`vite build` yeşil):**
- **4 yeni self-management aracı** (`internal/tools/builtin_workspacemgmt.go`):
  `list_workspaces` / `create_workspace` / `rename_workspace` / `delete_workspace`.
- **Çapraz-workspace köprüsü** — diğer self-manage araçları mevcut DB'de çalışır;
  workspace araçları workspace sınırını aştığından ayarlar gibi bir köprüden geçer:
  `tools.WorkspaceBridge` (arayüz) → `api.workspaceBridge` (manager + seedTemplate +
  `publishWorkspacesChanged`'i sarar) → `Manager.SetWorkspaceBridge` (tüm Runtime'lara
  dağıtır) → `Runtime.workspaceBridge`. `main.go`'da `SetSettingsBridge` yanında bağlanır.
- **Provenance + guard'lar:** `Meta.CreatedBy` alanı eklendi (ajan-oluşturduğu
  workspace). `list/create/rename` her workspace'te; **delete yalnız ajan-oluşturduğu**,
  ayrıca **mevcut çalıştığı** ve **son kalan** workspace silinemez. `Manager.Create`
  imzası `createdBy` parametresi aldı (UI yolu `""` geçer).
- **Canlı UI:** her değişiklik `workspaces` SSE event'i yayar → `App.tsx` switcher
  listesini canlı tazeler (`refreshWorkspaces`). create blank şablonla tohumlanır.
- **Testler (yeni):** `builtin_workspacemgmt_test.go` — provenance stamp + delete
  guard'ları (current/user/unknown/agent-created).
- **Doküman:** `24-SELF-MANAGEMENT.md` (workspace satırı + köprü wiring + guard notu),
  `tionswarm-self-management` skill kataloğu, `tionswarm-project` referans skill güncellendi.

## Ağ — Canlı mod: çalışan-run (Session) bağı + glow ✅ (2026-06-19)

**İstek:** Ajanın şu an çalıştırdığı oturum/run'ı (Session) gösterme — gerçek "şu an ne yapıyor".

**Yapılan:**
- **Backend** (`graph.go`): ajan düğümüne `running`+`runKind`+`runTarget` eklendi. `s.runs.activeSessionIDs()` (process-wide çalışan oturumlar) bu workspace'in oturumlarına join edilip her ajanın o anki aktivitesi (kind + type-prefixed task/flow hedefi) çıkarılır.
- **Frontend** (Canlı mod): çalışan ajan **parlak "live" glow** alır; `runTarget` (task/flow) varsa o düğüme **aktif accent bağ** kurulur. Aktif bağ ayrıca `in_progress` görev sahipliğiyle (fallback) de kurulur. Idle/busy hesabı running'i kapsar (çalışan ajan lobiye çekilmez).
- **Doğrulama:** `go build`/`tsc`/`vite build` yeşil. Canlı chat stream tetiklenip `/api/graph` poll'landı → akış sürerken ajan `running:true, runKind:'chat'` raporlandı (uçtan uca); test oturumu silindi (gerçek veri korundu).
- **Sınır:** `s.runs` yalnız chat-stream turn'lerini izlediğinden task/flow `runTarget` bağı şimdilik `in_progress` fallback'iyle gelir; run-tracker eklenince otomatik yanar.

## Ağ — Canlı mod: Boşta lobisi + ajanın akışı ✅ (2026-06-19)

**İstek:** Aktif görevi olmayan ajanlar için bir "boşta" çekim alanı; ayrıca ajanın kullandığı Flow/Session gibi şeyleri göstermek.

**Yapılan (Canlı mod):**
- **Boşta lobisi:** alt-ortada sabit `idle` çekirdeği; aktif (`in_progress`) görevi olmayan ajanlar zayıf yayla buraya çekilir, görev alınca güçlü aktif bağ onları kartına çeker.
- **Ajanın akışı:** Canlı modda `uses` (akış→ajan) kenarları + akış düğümleri gösterilir (flow katman chip'i ile); ajan bağlı olduğu akışla birlikte hareket eder. Skill/MCP de korunur.
- `lib/relationGraph.ts`: `IDLE_ID` anchor + busy-agent hesabı; `show()` Canlı modda flow'a izin verir; NetworkPanel katman chip'lerine flow eklendi.
- **Session/run (gerçek "şu an çalışıyor")**: ayrı adıma bırakıldı — backend'de executions `running`+kind+sourceId join'i gerekiyor.
- **Doğrulama:** `tsc`/`go build` yeşil; canlı Chrome (DenemeBilimsel): 4 ajan "Araştırma Akışı"na (uses) ve "Boşta" lobisine bağlı render oldu.

## Ağ — "Canlı" sütun-akışı modu ✅ (2026-06-19)

**İstek:** Ağda sütunlar sabit, görevler sütun altlarında; bir ajan göreve başlayınca o karta çekilsin, kart sütun değişince bağ kopsun, ajan başka görev alınca yeni bağ kursun — canlı akış. Skill/MCP ajanla bağlı.

**Yapılan:** Ağ paneline **İlişki | Canlı** mod geçişi eklendi.
- **Canlı mod (`workspaceToVis(graph, visible, 'live')`):** 5 sabit board-durumu sütun başlığı (`fixed`+`physics:false`), her görev `task→col` kenarıyla durum sütununa yaylanır. **Aktif bağ** = `owns` + görev `in_progress` (parlak accent kenar + gölge); diğer owns/created/runs/uses bağları canlı modda gizli. Skill/MCP bağları ajanla kalır.
- **Gerçek zamanlı:** `NetworkPanel` canlı modda `/api/events` SSE'ye abone olur (600ms debounce) → grafiği yeniden çeker. `VisNetworkGraph` DataSet'i **artımlı** (diff, konum sıfırlamadan) günceller → fizik motoru ajanı yeni bağına kaydırarak animasyon yapar.
- `VisNetworkGraph`'a `mode` prop'u (live'da düşük centralGravity + avoidOverlap); fit stabilizasyon sonrası yapılır.
- **Doğrulama:** `go build`/`tsc`/`vite build` yeşil. Canlı Chrome (MINIMAX ws): 5 sütun başlığı + görevler durum renklerine göre sütun altlarında kümelendi; geçici olarak bir in_progress göreve sahip atayınca **ajan→aktif görev parlak bağı** render oldu (sonra sahip `""`'a geri alındı — gerçek veri korundu). Detay: `_Docs/23-ILISKI-GRAFIGI.md`.

## Ajan kontrol-yüzeyi genişletme — hooks/mcp/secret-write/skill araçları ✅ (2026-06-19)

**Hedef:** "Ajanlar TionSwarm'yu her şekilde kontrol edebilsin." Ajanların araçla
dokunamadığı kontrol yüzeyleri kapatıldı (insan API/UI'da yapılabilen ama ajan
tool'u olmayanlar). `builtin_taskmgmt.go` desenini (provenance + `SelfManageEnabled`
gating + lazy) tekrarlayan 11 yeni araç:

- **Hooks** (`builtin_hookmgmt.go`): `list_hooks`/`create_hook`/`delete_hook` —
  delete yalnız ajan-oluşturduğu (`Hook.CreatedBy`).
- **MCP sunucuları** (`builtin_mcpmgmt.go`): `list_mcp_servers`/`create_mcp_server`/
  `toggle_mcp_server`/`delete_mcp_server` — yeni/etkin sunucu sonraki turda
  kataloğa girer (toolsetup `ListEnabledMCPServers`'ı her tur canlı okur);
  provenance için **`MCPServer`'a `CreatedBy` eklendi** (additive JSON alanı).
- **Secret yazma** (`builtin_secretmgmt.go`): `secret_set`/`secret_delete` —
  `secret_list`/`secret_get` okuma tarafının tamamlayıcısı (vault, AES-GCM).
- **Skills** (`builtin_skillmgmt.go`): `create_skill`/`delete_skill` — `tools`
  paketi `skills`'i import etmesin diye `SkillWriter` arayüzü + `runtime.go`
  `agentSkillWriter` adaptörü.

Hepsi `toolsetup.go` self-manage bloğuna eklendi (lazy işaretli). Testler
`builtin_controlgaps_test.go` (hook/mcp create + provenance + validation). Canlı
doğrulama: self-manage açık sunucuda ajan oluşturup `/api/agents/{id}/tools`
kataloğunda 11 aracın hepsi göründü. Default skill `tionswarm-self-management` +
`tionswarm-guide` ve `SKILL.md` (proje skill'i) güncellendi.

**Kalan boşluklar (bilinçli):** workspace CRUD (tools paketi `workspace.Manager`'a
erişmiyor — köprü gerek), market install (tür-özel install switch'i tool'a
taşınmalı), özel sağlayıcı (ayrı araç yok ama `update_settings` ile dolaylı).

**Not — Otomasyon motoru (event→action) prototiplendi ve KALDIRILDI:** kısa süre
`internal/agent/automation.go` + `Automation` entity + UI paneli denendi; ancak
spawn/schedule/flow ile benzer tetikleme zaten yapılabildiğinden ve panel ajan
kontrolü tezini ilerletmediğinden tümü geri alındı. Repoda iz yok.

## input_examples (tool kullanım örnekleri) ✅ (2026-06-19)

Anthropic "advanced tool use" üçüncü tekniği eklendi. `ToolDef.Examples
[]json.RawMessage`: şemanın söyleyemediği konvansiyonları (cron/tarih formatı, ID
deseni, opsiyonel alan kombinasyonu) gösteren somut örnek çağrılar.

- **Mimari:** örnekler **yalnız tam şemaya** katlanır — `registry.foldExamples`
  InputSchema'ya JSON Schema `examples` dizisi olarak gömer; `Defs`/`ActiveDefs`/
  `BridgeableDefs`'te uygulanır, `LazyCatalog`'a **değil**. Böylece lazy araçta
  örnek katalogu şişirmez, yalnız `activate_tools` sonrası (kullanılacağı an) token
  harcar. Provider'lara dokunulmadı (hepsi InputSchema okur).
- **Pilot (1. dalga):** `create_schedule` (5-alan cron + enabled) ve `create_flow`
  (graf'ın **escaped JSON string** oluşu + `{{input}}`/`{{node.id}}` şablonları +
  `next:""`=son). İkisi de lazy → eager bütçeye sıfır etki.
- **Pilot (2. dalga, 2026-06-19):** `create_hook` (matcher glob + command'in
  stdin/stdout JSON sözleşmesi), `update_settings` (`patch` opak
  `additionalProperties:true` → en güçlü aday), `create_mcp_server` (stdio vs
  sse/http; `args`/`env` escaped JSON string).
- **Pilot (3. dalga, 2026-06-19) — edit/create araçları:** `update_flow`,
  `update_schedule`, `update_task` (`dependencies` escaped JSON array + `flowId:""`
  =unlink), `create_agent` (provider/model eşleşmesi + heartbeat). Edit aracında
  örnek **kısmi-güncelleme** konvansiyonunu öğretir (id + yalnız değişen alan).
  **Silme araçları ve `move_task` (enum) bilinçli atlandı** — şema zaten
  belirsizliksiz. `TestPilotToolExamplesAreValid` artık **dokuz** aracın örneklerini
  doğrular (stringify alanların iç JSON'u dahil).
- **Maliyet:** örnek küçük sabit token; hatalı-çağrı + hata + retry turunu
  önlediğinden pratikte **net negatif** (token kazandırır).

Testler: `TestExamplesFoldIntoSchemaNotCatalog`, `TestPilotToolExamplesAreValid`
(gerçek pilot örneklerin JSON geçerliliği). `go vet`/`test ./...` yeşil. Detay:
`19-LAZY-TOOL-LOADING.md`.

## tool_search rename + MCP-ağır katalog kısaltma ✅ (2026-06-19)

Lazy keşfini Anthropic "advanced tool use" modeline yaklaştıran iki değişiklik:

1. **`find_tools` → `tool_search` rename** (`builtin_activate.go`): tip
   `ToolSearchTool`, ctor `NewToolSearchTool`, araç adı `tool_search`. Tüm
   referanslar (toolsetup wiring, katalog blok metni, activate_tools hata mesajı,
   testler, doc/skill) güncellendi. Frontend'de referans yok.
2. **MCP-ağır katalog kısaltma** (`renderLazyToolCatalog`, `toolsetup.go`):
   "Available Tools (load on demand)" bloğu built-in lazy araçları **tam** listeler;
   namespaced MCP araçları `lazyCatalogMCPListLimit=30`'a kadar tek tek listelenir,
   üstünde **sunucu başına özet** (ad + sayı) verilip gerisi `tool_search`'e
   bırakılır. Bir MCP sunucusu yüzlerce araç açabildiğinden bu, cache'lenen sistem-
   prompt prefix'ini yalın tutar (Anthropic'in "ara, sıralama" deseni).

Testler: `TestLazyCatalogSummarisesManyMCPTools` (agent), `tool_search` adı +
arama (`lazyload_test.go`). `go build`/`vet`/`test` yeşil. Detay:
`19-LAZY-TOOL-LOADING.md`. Not: bu turda PowerShell `Get-Content|Set-Content`
round-trip'i 3 dokümanı cp1254 çift-kodlamayla bozdu; cp1254 ters çevirimle
kayıpsız onarıldı (U+FFFD=0). Doküman düzenlemede artık Edit aracı kullanılmalı.

## Bridge alt-küme + rol-bazlı eager + call_agent lazy ✅ (2026-06-19)

Eager küçültmenin üç takip adımı:

1. **Bridge alt-küme sınırı** (`tools.bridgeExcluded`, `registry.go`): CLI köprüsü
   tam şema ilan ettiğinden yüzeyi budandı — `http_get` (CLI'de WebFetch var) ve
   `call_agent` (dispatch native-loop `DelegationFrom(ctx)` ister, bridge ctx'inde
   yok) artık köprülenmiyor. Native ajanlar etkilenmez.
2. **Rol-bazlı eager** (`toolsetup.go`): `PermissionMode == "read-only"` ajanda
   `write_file`/`edit_file` eager'dan lazy'ye düşer (yazma zaten onaylanmaz → şema
   israfı). "ask"/"auto" eager tutar.
3. **call_agent lazy**: senkron delegasyon artık lazy (native `activate_tools`;
   CLI'ye köprülenmez — bkz. madde 1).

Testler: `TestBridgeableDefsExcludesCLINative` (tools),
`TestReadOnlyAgentDemotesWriteTools` (agent). `go test ./...` tamamı yeşil.
Detay: `19-LAZY-TOOL-LOADING.md`, `11-INTERACTION-MCP.md`.

## Eager çekirdek küçültme — 8 araç lazy ✅ (2026-06-19)

**Her tur gönderilen eager araç yüzeyi daraltıldı.** Self-management + MCP zaten
lazy'ydi; ek olarak her zaman kurulan ama turların azında kullanılan 8 araç da
lazy'ye indirildi (`toolsetup.go`, `reg.MarkLazy(...)`): `read_config`/
`write_config`/`list_config`, `secret_list`/`secret_get`, `list_sessions`,
`memory_recall`, `http_get`. Gerekçe: config editing nadir; secret yalnız
kimlik-bilgili görevlerde; recall `ContextBlock` ile zaten otomatik enjekte;
çoğu tur dış istek yapmıyor. UX-hassas etkileşim primitifleri (`ask_user`,
`request_confirmation`, `schedule_wake`) ve artifact çıktı yolu eager bırakıldı.

**Sinerji:** CLI-3 köprüsü tüm lazy built-in'leri `BridgeableDefs` ile claude-cli'ye
bridge ettiğinden, bu indirme aynı zamanda `memory_recall`/`secret_list`/
`secret_get`/`list_sessions` araçlarını **CLI ajanlarına da otomatik açar** —
aşağıdaki "sıradaki köprü adayları" listesinin bir kısmı bu değişiklikle kapandı.
`MarkLazy` builtins'te olmayan ada no-op olduğundan gate'li araçlar için ek koruma
yok. `go build`/`vet`/`test` (tools/agent/api) yeşil. Detay: `19-LAZY-TOOL-LOADING.md`.

## use_skill — CLI köprüsü ✅ (2026-06-19)

**Skill'ler artık claude-cli ajanları tarafından da kullanılabiliyor.** Sorun:
CLI ajanları sistem promptunda `# Available Skills` kataloğunu görüyordu ama
gövdeyi yükleyecek `use_skill` aracı native bir Go aracıydı ve CLI'nin
`--mcp-config`'inde yer almıyordu → katalog görünüyor, hiçbir skill okunamıyordu.
Çözüm `spawn_session` köprüsüyle aynı desende: `use_skill` Interaction MCP'ye
eklendi. Değişen dosyalar: `runtime.go` (`LoadSkillForAgent` — native `agentSkillLib`
allowlist'ini dışa açar), `chat_control.go` (`chatRun.skill` + `setSkillLoader`/
`skillLoaderFor`), `chat_stream.go` (her turda yanıtlayan ajanın yükleyicisini kurar),
`mcp_interaction.go` (`Tools()` ilanı + `callUseSkill` dispatch, çıktı native
`UseSkillTool.Call` ile birebir), `climcp.go` (`interactionToolNames` allowlist).
Erişim kontrolü, sub-skill footer'ı, lazy disk okuma native yolla tam parite;
tek kaynak TionSwarm skill store'u (dosya kopyası/symlink yok). Build + `go test`
(api/agent/skills) yeşil. Detay: `11-INTERACTION-MCP.md §8`.

> Sıradaki köprü adayları (native'de var, CLI'de yok): `memory_recall`/`memory_add`
> (TionSwarm hafızası — CLI tamamen kör), `secret_list`/`secret_get` (kasa),
> `list_sessions`, `call_agent` (delegation açıkken) ve self-manage ailesinin
> tamamı (yalnız `spawn_session` köprülü). FS/shell/http→WebFetch CLI'de native
> karşılığı olduğu için köprü gerektirmez.

## Spawn — CLI köprüsü + tier/log fix + UI temizliği ✅ (2026-06-19)

**spawn_session → claude-cli ajanlarına açıldı.** Daha önce `spawn_session` yalnız
native-API provider'larının (anthropic/minimax-anthropic) Go tool-loop'unda vardı;
claude-cli ajanları (Coder/Fasty) araçlara Interaction MCP köprüsüyle ulaştığından
bulamıyordu. Köprüye eklendi: `mcp_interaction.go` (`interactionBackend.tun` ile
self-manage gating, `Tools()` ilanı, `callSpawn` dispatch), `chat_control.go`
(`chatRun.spawn` + set/get), `chat_stream.go` (her ajan turunda taze
`SpawnSessionTool` örneği → per-turn bütçe), `server.go` (köprüye `tun`). Hem
Minimax3 (native) hem Coder (claude-cli) canlı doğrulandı → Fasty için bağımsız
oturum açtılar. Detay: `22-SPAWN-SESSION.md` + `11-INTERACTION-MCP.md §8`.

**Tier→provider bug'ı (çalışan binary bayattı):** Eski binary `agent.Tier` etiketini
ayar tier-provider'ına (`tierMedium/Smart`=anahtarsız `anthropic`) yönlendirip
"anthropic provider not configured" veriyordu; güncel kaynakta bu bağ zaten yok →
backend güncel kaynakla yeniden başlatılınca düzeldi (Coder/Minimax3 PONG).

**Log açığı:** `chat_stream.go::failTurn` artık başarısız turu loglar
(`"turn failed"` reason+detail) — provider-unavailable gibi çıktısız hatalar
sunucu log'unda görünmüyordu.

**UI:** ExecutionsPanel'den "✨ Başlat" butonu kaldırıldı (`SpawnSessionModal`
artık öksüz). `tsc` yeşil.

## A3 + B2 + P3-Aşama4 — İptal hiyerarşisi & arg-bazlı izin ✅ (2026-06-19)

Üç P1 izin/iptal maddesi tek oturumda kapatıldı. **P3 Aşama 4** kodda zaten
tamdı (callPermission + StepPermission + ortak grant); roadmap durumu güncellendi.

**A3 — İptal hiyerarşisi (`agent/toolloop.go`):** Tur-içi iptal dalı (`ctx.Err()`)
artık dönmeden önce yarım kalan batch'in TÜM tool_call'larına sentetik `cancelled`
tool_result basar (`fillCancelledResults` + `cancelledToolMsg`) ve user turunu
ekler → assistant'ın tool_use turu hiç dangling kalmaz (provider'ın tool_use↔
tool_result eşleme kuralı korunur; reaktif compaction / replay güvenli). Not:
kalıcı geçmiş zaten yalnız metin turlarını saklıyor (`toProviderMessages`), bu
fix tur-içi `req.Messages` bütünlüğü + ileriye dönük güvence içindir. `cancel_test.go`.

**B2 — Arg-bazlı izin deseni (`Bash(git *)`):** Yeni `tools/permpattern.go`:
`PermRule` (`Tool` + opsiyonel `ArgGlob`), `globMatch` (`*` joker), `RepresentativeArg`
(exec araçlarda komut satırını çıkarır), `DeriveGrantRule` ("git status -s" →
`shell(git *)`, env-prefix atlar). `tools/grants.go` kural deposuyla genişledi
(`Matches`/`GrantRule`; `Grant`/`Granted` geriye-uyumlu sarmalayıcı). `permGate`
(native) ve `callPermission` (CLI) artık çağrı argümanını çıkarıp eşleştiriyor;
"Her zaman izin ver" exec araçta **komut ailesine daraltılır** (git'e izin →
git'ler sorusuz, ama `rm` yine sorar) — diğer araçlarda eski "tüm-araç" davranışı.
`PermissionFunc` imzasına `arg` eklendi; izin kartı (`PermissionPrompt.tsx`) artık
gated komutu gösterir (`StepPermission.Text`→`PendingAsk.cmd`). `permpattern_test.go`
+ `permission_test.go` (komut-ailesi daraltma testi).

**P3 Aşama 4 (durum güncellemesi):** claude-cli "ask" → `--permission-prompt-tool`
→ Interaction MCP `permission_prompt` (`mcp_interaction.go callPermission`):
risk sınıflar, read/granted otomatik geçer, aksi halde `StepPermission` kartı
basıp kullanıcı kararını bekler; native yol ile **aynı oturum grant setini**
paylaşır (`run.setGrants` + `tools.WithGrants`). Zaten implementeydi.

**Doğrulama:** `go build ./...` + `go vet` + `go test ./internal/tools ./internal/agent ./internal/api` + `tsc --noEmit` yeşil.

## C4 — Caching ROI: oturumlar arası kümülatif maliyet ✅ (2026-06-19)

**Bağlam:** C4'ün ilk yarısı (`cache_creation` vs `cache_read` ayrımı) zaten
uçtan uca yapılmıştı — `anthropic.go` API parse → `db.Usage` ayrı CacheRead/Write
→ `pricing.go` (`CostDetailed`/`CacheSavings`) → Bütçe ekranı "bugün" rozeti.
Eksik olan ikinci yarıydı: **"oturumlar arası toplam (caching ROI)"** — her şey
yalnız *bugün* kapsamlıydı.

**Backend (`api/budget.go`):** `dayPoint`'e `cacheReadTokens`/`cacheWriteTokens`/
`costUSD`/`savingsUSD` eklendi → trend artık token hacmi değil **caching ROI**'yi
de zaman ekseninde taşır (her gün `modelRowsFor(u.ByModel)` ile bugünkü ekranla
birebir aynı maliyet mantığı). Yanıta yeni **`cumulative`** bloğu: seçili pencere
(7/30/90g) genelinde toplam çağrı/token/maliyet/tasarruf + **`cacheHitRate`**
(`cacheRead / (cacheRead + input + cacheWrite)`) — tek bakışta ROI sinyali.

**Frontend (`BudgetPanel.tsx` + `types/usage.ts`):** "bugün" kartlarının altına
**pencere-kümülatif kart sırası**: Toplam maliyet (son Ng), Cache tasarrufu (son
Ng), Cache isabet oranı (%), Tasarrufsuz maliyet (caching olmasaydı = maliyet +
tasarruf). Trend tooltip'ine günlük maliyet/tasarruf eklendi. `BudgetTrendPoint`
genişledi + yeni `BudgetCumulative` tipi.

**Sınır (B5'ten devam):** Hâlâ per-sohbet maliyet YOK — usage `agent+gün` anahtarlı;
kümülatif "oturumlar arası" toplam pencere genelinde agregadır, `SessionID`
boyutu eklenmedi (bilinçli ertelendi).

**Doğrulama:** `go build ./...` + `go test ./internal/db ./internal/providers` +
`tsc --noEmit` yeşil.

## Faz B5 — Yüzeyler Arası Tutarlılık (Motor B paylaşımı) ✅ (2026-06-19)

**Karar:** İki veri motoru ayrı kalır — **Motor A** (canlı bağlam tahmini,
`conversation.EstimateTokens`) ve **Motor B** (kayıtlı kullanım defteri,
`RecordUsage`→`db.Usage`→`pricing.go`). Birleştirme yanlış soyutlama olurdu
(farklı ömür/kapsam/birim). Bunun yerine üç yüzey aynı motorları **tutarlı**
okusun diye yüzey düzeyinde hizalama yapıldı.

**Backend:** `/api/agents/{id}/usage` artık Bütçe ekranıyla **aynı Motor-B
yardımcısından** (`modelRowsFor`) geçer → `costUSD`, `savingsUSD`, `byModel[]`,
cache token'ları, `priced`/`estimated` döner. `budget.go`'ya paylaşılan
`modelRowsFor` eklendi (PriceFor→EstimateFor fallback; budget ekranıyla bire bir
aynı maliyet mantığı).

**Frontend:**
- `ChatMeters` (sohbet sağ-üst): harcama göstergesi artık **maliyet** de gösterir;
  tooltip netleşti ("Bu ajanın BUGÜNKÜ toplam harcaması — tüm oturumlar"); tıklayınca
  **Bütçe ekranına** gider (`onOpenBudget`). Bağlam göstergesinin tooltip'i de
  "bu oturumun doluluğu" diye netleşti (Motor A vs B karışmasın).
- **Limit düzenleme** ChatMeters'tan **Bütçe ekranına taşındı** (asıl yeri orası):
  ajan tablosunda "limit" butonu → çağrı+token limiti düzenler (`api.setBudget`).
- `SessionDetailPanel`: yeni **"Ajanın bugünkü harcaması"** bölümü — Motor-B'yi
  olduğu gibi (ajan+gün) gösterir, **açıkça "bu sohbete özel değil" etiketiyle**
  (usage session bazlı değil); maliyet + ilk 4 model + cache tasarrufu + "Bütçe
  ekranı →" linki. `agentId` + `onOpenBudget` prop'ları App'ten geçer.

**Sınır (bilinçli):** Per-sohbet maliyet YOK — usage `agent+gün` ile anahtarlı,
`SessionID` boyutu yok. Gerçek per-sohbet maliyet istenirse usage kaydına
`SessionID` eklenmesi gerekir (ayrı, daha büyük iş — bilinçli ertelendi).

**Doğrulama:** `go build`/`vet`/`test` + `tsc`/`vite` yeşil. **Canlı API smoke:**
`/api/agents/{id}/usage` opus 120k girdi + 400k cache-oku + 24k çıktı →
**$4.20** maliyet + **$5.40** tasarruf + model detayı, Bütçe ekranıyla birebir
aynı hesap.

## Interaction MCP: self-management CLI köprüsü (CLI-3) ✅ (2026-06-19)

**İstek:** claude-cli ajanları lazy self-management ailesini kullanamıyor (`activate_tools` döngüsü CLI'de yok).

**Yapılan:** Lazy built-in'ler CLI'ye önden advertise edilip native registry üzerinden dispatch edilen generic köprü.
- `Registry.BridgeableDefs(allow)` — lazy built-in'lerin tam şeması (MCP hariç). `Runtime.BridgeTools(ctx, agent)` — registry kurar, def'ler + dispatcher döner (per-agent `toolFilter`).
- `chatRun.setBridge(defs, call)` + stream handler her ajan turunda kurar; endpoint `mergeInteractionToolNames` ile birleşik allowlist (statik + köprü, dedup) ile yeniden bağlanır.
- `interaction.Backend.Tools()` → **`Tools(token)`** (per-run): token→run, statik spec'lere `bridgeDefs` eklenir (ad-dedup). `Call()` default → `run.bridgeCallFor()` native registry dispatch'i.
- Self-manage kapalıyken katalog boş → no-op. Test: `mcp_interaction_test.go TestInteractionBridge` (advertise + dispatch + token izolasyonu). **Durum:** `go build/vet/test ./...` yeşil; claude-cli canlı uçtan-uca doğrulama beklemede. Detay: `_Docs/11-INTERACTION-MCP.md`.

## Interaction MCP: CLI araç köprüsü generic'leştirildi (CLI-2) ✅ (2026-06-19)

**Sorun:** claude-cli ajanları skill kataloğunu görüyor ama gövde yükleyecek `use_skill` aracı CLI'nin `--mcp-config`'inde yoktu. Ayrıca her Interaction MCP aracı **üç yerde** elle kablolu (advertise `Tools()` + dispatch `Call()` + `climcp.go` allowlist'i `interactionToolNames`) — yeni araç eklemek kırılgan.

**Yapılan:**
- **CLI-1 (paralel oturum, bu sırada uygulandı):** `use_skill` Interaction MCP'ye köprülendi — `Tools()` spec + `Call()` → `callUseSkill` + stream handler'ın `setSkillLoader(LoadSkillForAgent)` (per-agent `AllowedFor` allowlist'i korur). Detay: `_Docs/11-INTERACTION-MCP.md` §8.
- **CLI-2 (bu oturum):** advertise + allowlist **tek kaynağa** indirildi. `interactionToolSpecs(tun)` tek üretici → hem `Tools()` hem yeni `interactionAdvertisedNames(tun)`. Stream handler isimleri `InteractionEndpoint.ToolNames`'e koyar (`tools/interaction.go` yeni alan + `WithInteractionEndpoint` imzası); `climcp.go` allowlist'i bundan türetir, sabit `interactionToolNames` **kaldırıldı**. Artık araç eklemek = `interactionToolSpecs`'e `Def()` + `Call()`'a `case`; allowlist otomatik. `spawn_session` self-manage gating'i tek yerde. Test: `mcp_interaction_test.go TestInteractionAdvertisedNames` (advertise == allowlist değişmezi).
- **Durum:** `go build`/`go vet`/`go test ./internal/{api,agent,tools,interaction}/...` yeşil.
- **Not:** CLI-1, ben CLI-2'yi analiz ederken paralel bir oturumca aynı dosyalara yazıldı (mcp_interaction.go 286→320 satır). Çakışmadan kaçınmak için CLI-1 bitene kadar bekleyip CLI-2'yi taze taban üstüne uyguladım — bu oturumun belgelediği RG-1/RG-2 yarışının canlı örneği.

## Backlog: Faz R — çok-ajan yarış & kurtarma guard'ları (2026-06-19)

**İstek:** Bir dev-oturumu (`260617-gentle-coyote`) analizinden çıkan sürtünme noktaları (paralel-commit yarışı, port çakışması, elle temizlik, varlık-kontrolsüz "özellik yok" kararı) TionSwarm task'ı olarak dokümanlara işlensin.

**Yapılan:** `_Docs/03-YOL-HARITASI.md`'ye yeni **Faz R** bölümü eklendi — 6 aday task (RG-1..RG-6) gerçek dosya dayanaklarıyla:
- **RG-1** entity versioning + CAS (data-loss kapatır), **RG-2** workspace git-lock + provenance-scoped staging, **RG-3** kaynak kira registry, **RG-4** tur yan-etki defteri → auto-teardown, **RG-5** boot orphan reconcile, **RG-6** implement-öncesi keşif guard'ı (skill, kod yok).
- Önceliklendirme: 1. dalga RG-6+RG-1+RG-4; 2. dalga RG-2+RG-3+RG-5. Ürün/skill ayrımı belirtildi.
- Not: ekleme sırasında 03 dosyasının paralel oturumca güncellendiği görüldü (B2 + P3 Aşama-4 artık tamamlanmış) — tam da RG-1/RG-2'nin hedeflediği eşzamanlı-yazım durumu.

## Doküman bakımı: kod ↔ doküman senkronu ✅ (2026-06-19)

**İstek:** Git'e eklenmiş commit'ler + commit'lenmemiş dosyalar incelensin; dokümanlarda eksik/yanlış/fazlalık varsa düzeltilsin.

**Yapılan (kod değişikliği yok, yalnız doküman + `.gitignore`):**
- **`_Docs/02-VERI-MODELI.md`** — yeni `MCPServer.CreatedBy` + `flows.created_by` tabloya işlendi; tüm self-management entity'leri için ortak **provenance (`created_by`) konvansiyonu** notu + şema-evrim maddesi eklendi.
- **`_Docs/18-HOOKS.md`** — yeni ajan araçları (`list/create/delete_hook`, provenance guard'lı) için "Self-management (ajan araçları)" bölümü + Dosyalar satırı.
- **`_Docs/24-SELF-MANAGEMENT.md` (YENİ)** — tüm öz-yönetim araç yüzeyi (agents/flows/schedules/tasks/hooks/MCP/secret/skill/artifact/memory/logs/settings) + gating/lazy davranışı + provenance guard'ı + **ayarlar alt sistemi** (validate→clamp, bridge wiring, `settings` SSE çok-pencere senkronu) tek dokümanda toplandı.
- **`_Docs/03-YOL-HARITASI.md`** — CG-7 `[ ]`→`[~]` (komut hook'ları Faz P4 ile yapıldı); Tamamlananlar listesine self-management genişlemesi + ayarlar canlı-uygulama + İlişki Grafiği eklendi.
- **Numara çakışması düzeltildi:** iki `18-` önekli dosya vardı → `18-SPAWN-SESSION.md` boş `22` slotuna taşındı (`22-SPAWN-SESSION.md`), başlık + tek çapraz-referans güncellendi. `_Docs/` artık 00–24 kesintisiz.
- **İçerik doğruluğu:** `05` grafik yerleşim özeti (Fizik/Küme) ile koda göre düzeltildi.
- **`.gitignore`** — build artıfaktı `*.exe~` + scratch manuel test `frontend/tionswarm_title_test.mjs` ignore'a eklendi.
- **Workspace referans skill'i** (`tionswarm-project/SKILL.md`) — doküman aralığı 00–24'e genişletildi, İlişki Grafiği yeteneği + 24 referansı eklendi.

## "Araçlar" NavRail görünümü → Ayarlar kategorisine taşındı ✅ (2026-06-19)

**İstek:** NavRail'deki **Araçlar** butonu, Ayarlar ekranının soldaki kategori paneline taşınsın.

**Yapılan:** Çalışma-alanı araç kataloğu + MCP sunucu yönetimi (`components/panels/ToolsPanel.tsx`) artık ayrı bir top-level görünüm değil; Ayarlar ekranının sol panelinde **"Araçlar & MCP"** kategorisi olarak açılıyor. Panel kendi master-detail düzenini taşıdığı için Ayarlar içinde **tam genişlikte** (merkezi `max-w-2xl` sütununun dışında) render edilir; bu kategoride "Kaydet" butonu/durum metni gizlidir (araçlar toggle ile anında kaydeder).
- `components/settings/primitives.tsx` — yeni `Cat` anahtarı `mcptools` + `APP_CATS`'a `{ key: 'mcptools', label: 'Araçlar & MCP', icon: Plug }` ('tools'/"Yetenekler" hemen ardından).
- `components/SettingsPanel.tsx` — `ToolsPanel`'i `ToolsCatalogPanel` olarak içe aktarır; `cat === 'mcptools'` özel-durumu tam genişlik render eder; header'da Kaydet + durum metni `mcptools` için gizli.
- `components/NavRail.tsx` — `NAV`'dan `tools` kaldırıldı; `View` birleşiminden `'tools'` çıkarıldı; kullanılmayan `Plug` import'u silindi.
- `App.tsx` — `ToolsPanel` import'u, `VIEW_TITLE.tools` ve `view === 'tools'` render satırı kaldırıldı; ilgili eski yorum sadeleştirildi.
- `lib/url.ts` — `VIEWS` listesinden `'tools'` çıkarıldı (artık geçerli bir deep-link view'i değil).
- **Not:** Ayarlar'daki mevcut **"Yetenekler (Araçlar)"** kategorisi (`tools`, env toggle'ları) ayrı bir şeydir; ona dokunulmadı. Sohbet `/tools` özet komutu (summarize 'tools') de ayrıdır, korundu.
- **Durum:** `tsc --noEmit` yeşil (EXIT=0).

## Masaüstü bildirimlerinde çoklu-sekme tekilleştirme (`tag`) ✅ (2026-06-19)

**İstek:** Zamanlama (schedule) her tamamlandığında "bir fazla bildirim" geliyordu.

**Teşhis:** Backend doğru — `scheduler.go` her tur için **tam bir** event yayınlıyor; çift-emit yok, frontend tek SSE abonesi. Çift bildirimin kaynağı: uygulamanın **birden fazla sekme/pencerede** açık olması; her sekme aynı SSE olayını alıp ayrı bir OS toast'u gösteriyordu. (Ayrıca önceden aynı prompt'un iki ajana zamanlanmış olması da ayrı bir çift kaynağıydı; o veri zaten temizlenmişti.)

**Yapılan:** Bildirimlere olay-türevli **stabil `tag`** eklendi; tarayıcı aynı tag'li bildirimleri **değiştirir (yığmaz)** → çok sekmede bile tek toast.
- `lib/clientPrefs.ts` — `notify(..., tag?)` → `new Notification(title, { body, tag })`.
- `App.tsx` — otonom olaylar için `tag = workspaceId:type:sessionId|agentId:time`.
- `hooks/useChatStream.ts` — sohbet yanıt/hata bildirimleri için `chat-reply:sid:msgId` / `chat-error:sid:liveId`.
- **Durum:** `tsc -b` yeşil.

## İlişki Grafiği — Workspace Ağı + Hafıza Bilgi Grafiği ✅ (2026-06-19)

**İstek:** Agent-MCP'deki "Multi-Agent Collaboration Network" tarzı ilişki görselleştirmesini TionSwarm'da kullan.

**Yapılan:** Mevcut React Flow altyapısını yeniden kullanan iki salt-okunur ağ görünümü eklendi.
- **Workspace Ağı** (NavRail → "Ağ", `Share2` ikonu): ajanlar hub, görevler ışın, akışlar çok-ajanlı bağlayıcı. Kenar türleri: `owns`/`created`/`runs`/`uses` (renk+lejant). İki deterministik yerleşim modu (toolbar geçişi): **Fizik** (varsayılan, `forcePositions` — Fruchterman–Reingold eşit dağılım) ve **Küme** (`clusterPositions` — hub-and-spoke).
- **Hafıza Bilgi Grafiği** (Hafıza → Liste/Ağ geçişi): bir ajanın hafızalarının lexical-cosine benzerlik grafiği; tür-renkli düğümler, degree ile boyut, benzerlik eşiği kaydırıcısı.
- **Backend:** `internal/memory/graph.go` (`Store.Graph`, pairwise cosine + cap, `graph_test.go`), `internal/api/graph.go` (`GET /api/graph`, `GET /api/agents/{id}/memory-graph`).
- **Frontend:** `types/graph.ts`, `api/graph.ts`, `lib/relationGraph.ts`, `components/graph/{VisNetworkGraph,MemoryGraphView}.tsx`, `components/panels/NetworkPanel.tsx`. Grafik motoru ayrı lazy chunk.
- **vis-network'e geçiş (2026-06-19):** İlk React Flow + saf-TS force simülasyonu homojen dağılım vermedi (mesafe/yoğunluk kırılgan). Agent-MCP'nin de **`vis-network` (vis.js)** kullandığı `package.json`'dan doğrulanınca **`vis-network` v10.1.0 + `vis-data` v8.0.4**'e geçildi. Gerçek fizik motoru (`forceAtlas2Based` çözücü) bağsız/seyrek graflarda bile **homojen dağılım** veriyor. (Not: kısa süre denenen "Ağaç"/hiyerarşik mod bu döngüsel+bağsız veride bozuk göründüğü için kaldırıldı — hiyerarşi için Akışlar ekranı zaten gerçek DAG'dır; ağ yalnız fizik düzeni kullanır.) Eski `RelationGraph.tsx`/`EntityNode.tsx` + force layout fn'leri silindi; `lib/relationGraph.ts` artık DTO→vis eşleyici (`workspaceToVis`/`memoryToVis`), yeni `VisNetworkGraph.tsx` sarmalayıcı. vis-network ~515KB ayrı lazy chunk (ana bundle değişmedi).
- **Yoğunluk kaydırıcısı + yeni katmanlar (2026-06-19):** Toolbar'a **Yoğunluk** kaydırıcısı (0.4×–2×) eklendi — `VisNetworkGraph` `density` prop'u forceAtlas2 itme/yay uzunluğunu canlı ölçekler. Ağa iki yeni düğüm türü eklendi: **beceri/skill** (sarı altıgen, `Agent.Skills`'ten, `skill` kenarı) ve **MCP sunucusu** (teal kare, etkin sunucular + `Agent.MCPEnabled`, `mcp` kenarı). Toolbar'da **katman chip'leri** (Görevler/Akışlar/Beceriler/MCP) ile her tür açılıp kapatılır; backend `/api/graph` skills/mcp düğüm+kenarlarını ve `stats`'a sayıları döndürür. Hafıza bilinçli olarak workspace ağına eklenmedi (yüzlerce düğüm → ayrı Hafıza→Ağ grafiği kapsar). Canlı doğrulama: MINIMAX ws'de teal "gateway" MCP düğümü + ajanlara teal kenarlar render oldu.
- **Düğüm şekilleri + tooltip (2026-06-19):** Görevler artık **durum-renkli kare** (başlık altında etiket) + **hover açıklama tooltip'i** (vis `title`=HTMLElement; backend `graphNode.Desc`=`Task.Description` eklendi). Beceriler **yıldız**, MCP **üçgen** (kareyle çakışmasın diye). `lib/relationGraph.ts`'e `tip()`/`esc()` tooltip yardımcıları. Canlı doğrulama: MINIMAX ws'de kare görevler (durum renkli) + teal üçgen "gateway" render oldu; `desc` 23 görevde mevcut.
- **Durum:** `go build/vet/test ./internal/...` + `tsc`/`vite build` yeşil; canlı API smoke + **canlı Chrome görsel doğrulaması** (MINIMAX ws, Fizik homojen yayılım; kare görevler, üçgen MCP, yoğunluk kaydırıcısı çalışıyor). Detay: `_Docs/23-ILISKI-GRAFIGI.md`.

## Ayarlar skill'i + canlı ayar tool'ları (`get_settings`/`update_settings`) ✅ (2026-06-19)

**İstek:** TionSwarm'ya, uygulamanın tüm ayarlarını bilen bir **default skill** eklensin; ayarları/configleri dosya yolundan değiştirip **aktifleştiren** bir **tool** da eklensin; skill tool'a referans versin.

**Yapılan:**
- **Default skill `tionswarm-settings`** (`internal/skills/defaults/tionswarm-settings/SKILL.md`, `access: shared`): `settings.json` içindeki tüm uygulama ayarlarını gruplandırılmış olarak belgeler (görünüm, sağlayıcılar/model, kullanıcı profili, bağlam & hafıza, tur kurtarma, araç-çıktısı sıkıştırma, bütçe & otonomi, MCP + gated yetenekler, tanılama) — her alanın anlamı + geçerli aralık/varsayılan. `tionswarm-guide`'a subskill olarak eklendi. Gömülü defaults `EnsureDefaults` ile her workspace'e seed'lenir.
- **İki built-in tool** (`internal/tools/builtin_settings.go`, self-management gated, lazy):
  - `get_settings` → `settings.json` dosya yolu + güncel ayarları **maskeli** JSON döner (secret key'ler yalnız "set mi" olarak görünür).
  - `update_settings` → yalnızca değişen alanları içeren bir `patch` alır, diske yazar **ve canlı uygular** (restart yok); sayısal alanlar clamp'lenir.
- **Köprü (bridge) wiring'i:** `tools.SettingsBridge` arayüzü → `api.settingsBridge` (settings store + `applySettings` hook'unu sarar) → `Server.SettingsBridge()`; `Manager.SetSettingsBridge` mevcut + sonradan açılan tüm workspace `Runtime`'larına dağıtır (`Runtime.settingsBridge` + `SetSettingsBridge`); `main.go` server kurulumundan sonra bağlar. `settings.Store.Path()` eklendi.
- **Test:** `internal/tools/builtin_settings_test.go` (path raporu, patch uygulama, boş-patch reddi). `go build/vet/test ./...` yeşil.

**Sağlamlaştırma (validation) + canlı frontend yenileme (aynı gün):**
- **Validation (`internal/settings/validate.go`):** `Validate(Patch)` enum/format alanlarını denetler (theme, language, defaultProvider, defaultPermissionMode, logLevel, accent hex) ve geçersizleri **açık hata mesajıyla reddeder** — `Apply` en başta çağırır, yani hatalı değişiklik canlı alt sistemlere hiç ulaşmaz. Sayısal alanlar reddedilmez, `normalize` tarafından güvenli aralığa **clamp** edilir. `normalize`'a ek güvenlik ağı: `accent` (geçersiz hex → varsayılan) + `defaultPermissionMode` (bilinmeyen → `auto`) coercion'ı — elle bozulmuş bir `settings.json` bile yüklendiğinde uygulama çökmez. HTTP `PUT /api/settings` artık validation hatasında **400** döner (encryption hatası 500 kalır). Test: `internal/settings/validate_test.go` (kötü enum reddi, geçersiz patch state'i değiştirmez, bozuk değer coercion'ı, sayısal clamp).
- **Frontend canlı yenileme:** bir ajan `update_settings` ile ayar değiştirince `api.settingsBridge.Apply` `/api/events` üzerinden bir **`settings`** SSE event'i yayınlar (app-global → workspace rozeti/toast yok). `App.tsx` `onEvent`'te bu event: client-side prefs'i (tema/accent/bildirim) **canlı uygular** (`applyClientPrefs`) + `settingsNonce`'u artırır. `SettingsPanel` yeni `reloadNonce` prop'u ile — **yalnız kaydedilmemiş düzenleme yoksa** (dirty değilse) formu yeniden yükler, böylece eşzamanlı ajan değişikliği kullanıcının yazdığını ezmez. `tsc -b` + `vite build` yeşil.
- **Not (ilgisiz düzeltme):** `vite build`'i tıkayan, devam eden market/hooks WIP'ine ait iki TS hatası giderildi — `App.tsx` `VIEW_TITLE` haritasına `market: 'Market'` eklendi; `HooksPanel.tsx` `displayPath(t.path ?? '')`.
- **Default skill `tionswarm-self-management` (2026-06-19):** öz-yönetim tool ailesini (agents/flows/schedules/tasks/automations CRUD + `spawn_session`/`send_agent_message`/`run_flow` + artifact/memory/log + `get_settings`/`update_settings`) kataloglayan ve **lazy tool'ları `activate_tools` ile kendi-aktivasyon** akışını öğreten gömülü skill (`access: shared`). Ajan, "Available Tools (load on demand)" listesinden gerekeni `activate_tools`/`find_tools` ile kendisi yükler. `tionswarm-guide`'a subskill + body referansı; provenance/guard notları (silme yalnız ajan-oluşturduğu entity). `internal/skills/defaults/tionswarm-self-management/SKILL.md`. `go build/test ./internal/skills/` yeşil.
- **Çok-pencere senkronu (2026-06-19):** `settings` SSE event'i artık **UI'dan yapılan değişikliklerde de** yayılıyor — `handleUpdateSettings` (HTTP `PUT /api/settings`) ortak `Server.publishSettingsChanged` helper'ını çağırır (bridge de aynı helper'ı kullanır → tekrar yok). Böylece bir pencerede (veya ajan tarafından) yapılan ayar değişikliği **diğer tüm açık pencerelerde** canlı yansır (tema + form, dirty değilse). `go build/vet/test ./...` + `vite build` yeşil.

## Zamanlamalara opsiyonel son tarih (`expiresAt`) ✅ (2026-06-19)

**İstek:** Schedules ekranına opsiyonel bir "son tarih" seçimi eklensin.

**Yapılan:** Tekrarlayan cron zamanlamalarına opsiyonel `Schedule.ExpiresAt` (unix saniye, `0` = süresiz) eklendi. Son tarih geçince o ana denk gelen cron tick'i **atlanır**, zamanlama **otomatik pasifleşir** ve bir `Reload` ile cron tablosundan düşer; scheduler yeniden kurulurken süresi geçmiş satır hiç eklenmez (restart-safe). **Manuel "▶ Çalıştır" etkilenmez** — kullanıcı süresi dolmuş bir zamanlamayı elle bir kez daha koşturabilir. Saf karar katmanı `scheduleExpired(sc)` hem `rebuildLocked` hem `fire` tarafından kullanılır. API `POST/PUT /api/schedules` `expiresAt` alır; `UpdateSchedule` `ExpiresAt`'i persist eder (ama `enabled`'a dokunmaz → süresi dolmuşu tekrar açmak: düzenle→kaydet→toggle). Frontend `Schedules.tsx` oluşturma+düzenleme formlarında **"Son tarih (ops.)"** `datetime-local` alanı (✕ temizleme + gelecekte-olma doğrulaması); liste satırı son tarihi gösterir, geçmişse kırmızı **"(süresi doldu)"**. Test: `TestScheduleExpired` + `TestExpiredScheduleSkippedOnReload`. Detay: `_Docs/20-SCHEDULE-WAKE.md` ("Tekrarlayan Zamanlamalarda Son Tarih"). `go build/test ./internal/...` + frontend `tsc` yeşil.

## Market — dört türde kurulum + 20 örnek paket ✅ (2026-06-18)

**İstek:** Markete daha çok örnek (≈20, farklı türlerde) ekle.

**Yapılan (örneklerin gerçekten işe yaraması için kurulum da tamamlandı):**
- **20 yeni gömülü örnek paket** (toplam 23): 10 skill (web-research, code-review + technical-writing, data-analysis, debugging, prompt-engineering, sql-expert, git-workflow, api-design, summarization), 5 agent (researcher/coder/editor/planner/support), 4 provider (openrouter/groq/ollama/deepseek), 4 flow (research-synthesis/review-and-fix/parallel-brainstorm/draft-edit-finalize). Üreteç: `internal/market/gen_examples.py` (in-tree authoring helper; ürettiği JSON'lar `//go:embed` ile gömülür).
- **Dört türde de install** (önce yalnız skill, diğerleri 501'di): `api/market.go`'ya `installAgentPack` (`db.CreateAgent`, bilinmeyen skill slug'ları elenir, CreatedBy=""), `installFlowPack` (`db.CreateFlow`; agent-agnostik graph'ın boş `agentId` slotları workspace'in ilk ajanına atanır → hemen çalışır), `installProviderPack` (`settings.UpsertCustomProvider` + `applySettings` canlı push; API key gövdeden gelir, AES-GCM, pakette taşınmaz; provider id = `provider.<slug>`→`<slug>`) eklendi. Skill yolu market paketinde (dosya), agent/provider/flow API handler'ında (db/settings) — market paketini db/settings bağımlılığından uzak tutar.
- **Frontend MarketPanel**: artık her tür için "Kur" butonu (provider'da API-key input'u), tür-özel önizleme `PackPreview` (skill→markdown, agent→persona+sağlayıcı/model/beceriler, provider→baseUrl+modeller, flow→düğüm listesi). `types/market.ts` agent/provider/flow payload tipleriyle genişletildi.
- **Test güncellemesi:** `store_test.go` artık çok-türlü bundled set'i doğrular (her paketin bilinen bir kind'ı var; kind-listeleri toplamı = toplam).

**Durum:** `go build/vet` + `go test ./internal/market/... ./internal/api/...` + frontend `tsc` yeşil. **Canlı API E2E** (port 8090, gerçek workspace): agent→CreateAgent (UUID döndü), flow→CreateFlow (2 düğüm, ilk-ajan otomatik atandı), provider→Upsert (`keySet:true` AES-GCM key ile), skill→workspace+reload — dördü de 200. Test sırasında kullanıcının workspace'ine eklenen örnekler sonrasında temizlendi (market kataloğundaki 23 paket kalıcı). **Boot notu:** ilk seed-boot'unda EnsureDefaults dosyaları yazarken katalog tek seferlik 2 paket önbellekleyebilir; `reload`/yeniden başlatma çözer (skills sistemiyle aynı kalıp, diskte kalıcı).

**Zombi süreç temizliği:** 8090'ı tutan eski `tionswarm-dev.exe` (başka oturumdan) ve takılı `go run`/`vite` süreçleri sonlandırıldı; taze `tionswarm.exe` (8090) + tek `vite` (5173) çalışır durumda.

### Market — provider API anahtarı secret vault'tan seçim (commit `6e96e87`)
Provider kurulumunda API anahtarı artık **serbest metin değil**, Ayarlar→Sağlayıcılar paneliyle aynı politikayla **secret kasasından seçilir**: `MarketPanel` provider detayında `listSecrets` dropdown'u gösterir, seçilince `revealSecret(name)` ile değer çözülüp install gövdesine `apiKey` olarak gider (UI'da plaintext tutulmaz). "Sırlar →" butonu (`onManageSecrets` prop'u, App `setView('secrets')`) Sırlar ekranına atlar; sır yoksa anahtarsız kurulur. Playwright ile doğrulandı (Groq → `GROQ_API_KEY` seç → kur → `keySet:true`).

### Market — dedup + kurulum sonrası tazeleme (commit `29d1123`)
İki UX düzeltmesi: **(1) Tekrar ekleme koruması:** agent/flow kurulumu aynı **ada** sahip varlık varsa **409** + net mesaj (`nameExists`+`agentNames`/`flowNames`); skill dosya çakışmasında 409; provider id-keyed olduğundan çoğaltmaz, günceller. UI: kart "Kuruldu" rozeti + detay butonu disabled "Zaten kurulu" (provider'da "Güncelle"), `loadExisting`+`packTargetKey` ile hesaplanır. **(2) Tazeleme:** `MarketPanel onInstalled(kind)` → App `kind==='agent'` olunca `listAgents()`→`setAgents`, yeni ajan **Ajanlar ekranında manuel yenileme olmadan** görünür (flow/skill/provider panelleri zaten mount'ta yüklenir). Playwright: Coder kur→buton "Zaten kurulu", Ajanlar'da yenilemesiz göründü; ikinci API install→409.

### Market — veri kaybı tanısı (kod değişikliği yok)
"Marketten agent ekleyince eski agentler silindi" raporu araştırıldı. **Bulgu: market install ajan SİLMEZ** — `installAgentPack` yalnız `db.CreateAgent` çağırır; reproduction (2 mevcut ajan + market ajanı kur = üçü de sağ kaldı) ve kod bunu doğrular. Backend access-log'u silmenin install'dan **15 sn sonra gelen ayrı bir `DELETE /api/agents/{id}`** olduğunu gösterdi; bu uç yalnızca UI "Sil" butonundan (confirm'li) ya da agent `delete_agent` tool'undan (yalnız ajan-oluşturduğu ajanlar — kullanıcı ajanı korumalı) tetiklenir. Sonuç: silme açık bir delete eyleminden kaynaklandı, install'dan değil. Olası UX tuzağı: kurulum sonrası Ajanlar ekranında **yanlış (eski) ajan seçiliyken** "Sil"e basılması. Öneri (beklemede): kurulan ajanı otomatik seç / ad yazdırarak silme onayı / soft-delete.

## claude-cli için bütçe/maliyet tahmini ✅ (2026-06-18)

**İstek:** Bütçe ekranında `claude-cli` (AnthropicCli) sağlayıcısı için de maliyet hesaplaması yapılsın — mevcut `anthropic` ile aynı model fiyat tablosu kullanılarak eşdeğer API maliyeti tahmin edilsin; "abonelik / fiyatsız" yerine `~$X.XX` gösterilsin.

**Token kaydı:** `claudecli.go` zaten `cliUsage.InputTokens/OutputTokens`'ı stream-json olaylarından yakalıyordu; `RecordUsage` bunları `provider="claude-cli"` + model adıyla `ByModel` haritasına yazıyordu. Veri mevcut, yalnızca fiyatlandırma eksikti.

**Backend (`internal/providers/pricing.go`):**
- `EstimateFor(provider, model string) (Price, bool)` eklendi: `claude-cli` için anthropic fiyat tablosundan aynı model id'si ile liste fiyatı döndürür (abonelik/OAuth sağlayıcısı için "eşdeğer API maliyeti" tahmini). `PriceFor` değişmedi — `claude-cli` hâlâ `ok=false` döndürür (gerçek faturalandırma yok).

**Backend (`internal/api/budget.go`):**
- `modelStat`, `providerStat`, `agentBudgetRow` tiplerine `Estimated bool` alanı eklendi.
- `costOf` artık 3 değer döndürüyor: `(cost, priced, estimated)`. `PriceFor` başarısız olunca `EstimateFor` denenir; bulunursa maliyet hesaplanır, `estimated=true` işaretlenir.
- Provider ve model detay döngüsü de `EstimateFor` yolu eklendi.
- `totals` yanıt nesnesine `estimated` bayrağı eklendi.

**Frontend (`frontend/src/types/usage.ts`):** `ModelStat`, `ProviderStat`, `BudgetAgentRow` ve `totals` tipine `estimated?: boolean` alanı eklendi.

**Frontend (`frontend/src/components/panels/BudgetPanel.tsx`):**
- `costText(costUSD, priced, estimated?)` imzası genişletildi; `estimated=true` olan girişlerde tooltip "Abonelik (claude-cli) — eşdeğer API maliyeti tahmini; gerçek faturalandırma değil" olarak güncellendi.
- Agent tablosundaki maliyet hücresi `costText` kullanımına geçirildi (önceki satır-içi `~` mantığı kaldırıldı).
- Summary card sub-metni: `priced=false, estimated=true` durumunda "claude-cli: eşdeğer API maliyeti dahil".

**Test:** `TestEstimateFor_ClaudeCLI` (`pricing_test.go`): bilinen model `ok=true` + doğru fiyat, bilinmeyen model `ok=false`, metered `anthropic` sağlayıcısı `ok=false`. `go test ./internal/...` tam yeşil. **Playwright canlı doğrulama** (MINIMAX workspace, 8090+5174): Summary card `~$4.24` + "claude-cli: eşdeğer API maliyeti dahil", Claude CLI satırı `~$4.16`, Reminder `~$2.42`, StepTest `~$1.75`. Commit: bkz. git log.

## Beceri yazımı — UI'dan oluştur/düzenle/sil + emoji ikon ✅ (2026-06-18)

**İstek:** Beceri (skill) ikonlarını değiştirebilelim; ayrıca **yeni skill ekleyip mevcutları düzenleyebilelim**. Önceki Beceriler ekranı salt-okunurdu (yalnız paylaş/kısıtla, klasörü aç, diskten tara).

**Backend (`internal/skills`):**
- `frontmatter.go`: `setFrontmatterFields` + `quoteYAML` — sıralı, genel frontmatter yazıcısı: skaler anahtarları upsert/siler, yönetilmeyen anahtarları + blok listeleri (subskills vb.) **yerinde korur**, opsiyonel olarak gövdeyi değiştirir. İki nokta/YAML göstergesi içeren değerler güvenle tırnaklanır (`hasYAMLIndicatorPrefix`).
- `store.go`: `Store.Create` (workspace tier'a yazar; slug addan türetilir, **Türkçe harf transliterasyonu** ile), `Store.Update` (yerinde yeniden yazım, slug değişmez), `Store.Delete` (bilinen tier dizini altında guard'lı `RemoveAll`). `SkillInput`, `slugify`, `oneLine` yardımcıları.
- API (`skills.go`+`server.go`): `POST /api/skills`, `PUT /api/skills/{slug}`, `DELETE /api/skills/{slug}`; create/update beceriyi **gövdesiyle** döndürür → UI tek round-trip'te yenilenir.

**Frontend:**
- `api/skills.ts`: `createSkill`/`updateSkill`/`deleteSkill` + `SkillInput` tipi.
- `panels/SkillEditor.tsx` (yeni): oluştur+düzenle diyaloğu; **ikon için ortak `EmojiField`** (merkezi emoji seçici), ad, slug (yalnız oluşturmada), açıklama, ne-zaman, paylaşımlı toggle, Markdown gövde.
- `panels/SkillsPanel.tsx`: list başlığında **Yeni** butonu, detayda **Düzenle**/**Sil** (onaylı), editör bağlantısı + kaydetme sonrası listeyi/seçimi yeniler.

**Doğrulama:** `go test ./internal/skills,api` + `go build ./...` yeşil; yeni testler `store_test.go` (Create/Update/Delete round-trip, `slugify`, `setFrontmatterFields` koruma/tırnak). **API E2E** (canlı 8090): create→get→update→delete (açıklamadaki iki nokta round-trip, ikon **UTF-8 emoji** olarak `icon: 🧠` diske doğru yazılır, silme sonrası 404). **Playwright** (5173→8090): editör açılır, ortak `EmojiField` picker render olur, oluşturma workspace becerisi ekler (liste 3→4, slug `ui-test-beceri`, workspace badge). Not: curl/Git Bash emojiyi `??`'e bozuyordu (shell UTF-8 artefaktı, backend değil) — Python/fetch yolu emojiyi düzgün saklar. Commit `6dcbbb2` (9 dosya). Push yok. Frontend tsc kalan hataları paralel oturum WIP'i (App.tsx `market`, TaskDetailPanel, HooksPanel) — bu özellikle ilgisiz.

## Spawn Session — fire-and-forget paralel işçi ✅ (2026-06-18)

**İstek:** `spawn_session` deseni: bir prompt'tan **yeni, bağımsız bir oturum** başlatıp **beklemeden** bırakmak (paralel otonom işçi). Mevcut tetiklemeler (flow/schedule/`call_agent`/`send_agent_message`) bunu karşılamıyordu — spawn, FRESH bir oturum açıp turu arka planda koşar. Çıktı Faz U Aktivite feed'inde canlı görünür. Tasarım: `_Docs/22-SPAWN-SESSION.md`.

**Mimari (iki katman):**
- **Çekirdek `Runtime.SpawnSession` (`internal/agent/spawn.go`, yeni):** `scheduler.deliverPrompt` kalıbını genelleştirir — `resolveAgent` (id/isim, workspace-scoped) → `kind:"spawned"` + taze `sourceID` ile **bağımsız** session (GetOrCreate değil) → `AddMessage(user)` → **fire-and-forget goroutine** (`runSpawn`): `trackSession` (feed'de canlı "running") → `invokeTraced(KindSpawn, autonomous=true)` → `AddMessage(assistant, steps)` → tamamlanma event'i. Çağıranın ctx'i goroutine'i iptal etmez (`context.WithoutCancel`+10dk timeout) — HTTP/tur kapanınca spawn ölmesin. `SpawnOptions{ModelOverride,Title,CreatedBy}`.
- **Guard'lar:** eşzamanlı spawned-session üst sınırı (`spawnActive` atomik sayaç, `Tunables.SpawnMaxConcurrent` vars. 16) + tur başına spawn sayısı (`SpawnMaxPerTurn` vars. 4, tool örneği başına sayaç). Otonomi günlük bütçesi `invokeTraced(autonomous)` ile geçerli. Yeni `db.UsageKindSpawn`/`agent.KindSpawn`.

**Yüzeyler:**
- **HTTP:** `POST /api/sessions/spawn` (`internal/api/spawn.go`, yeni) — body `{agentId,prompt,modelOverride?}` → `{sessionId,agentName}`; `registerSessionRoutes`'a eklendi.
- **Ajan aracı:** `spawn_session` (`internal/tools/builtin_spawn.go`, yeni) — built-in, **`SelfManageEnabled`** ile gated (self-manage suite, lazy); `agent`/`prompt`/`modelOverride?` parametreleri, per-tur budget + provenance (`CreatedBy`). `toolsetup.go` self-manage bloğuna kaydedildi (send_agent_message yanına).
- **Ayarlar:** `settings.SpawnMaxConcurrent`/`SpawnMaxPerTurn` (DTO+Patch+defaults+clamp 1–128 / 1–64) → `applySettings`→`tun.SetSpawnLimits`.
- **UI:** `ExecutionsPanel` `spawned` kind metadata (✨ "Spawn") + filtre sekmesi + başlıkta **"✨ Başlat"** butonu → `SpawnSessionModal.tsx` (yeni: AgentPicker + prompt + opsiyonel model); başarıda spawned filtresine geçer + yeni yürütmeyi seçer. `api.spawnSession`.

**Durum:** `go build`/`go vet`/`go test ./internal/...` + frontend `tsc`/`vite build` yeşil. Yeni testler `internal/agent/spawn_test.go` (bağımsız session + isimle çözümleme + boş-prompt reddi + eşzamanlılık cap). **Canlı API E2E** (izole instance, gerçek claude-cli): spawn anında `sessionId` döndü → feed'de `spawned` yürütmesi (✨ başlık, SpawnWorker ajanı) → arka plan turu koştu, asistan "PONG" yanıtı ~2s'de transkripte düştü. Not: Playwright UI smoke yapılamadı (8080 portunu Unity MCP işgal ediyor, dev proxy backend'e ulaşamıyor); UI build-doğrulandı.

## Uygulama İçi Market — tasarım + MVP ilk dilim ✅ (2026-06-18)

**İstek:** Uygulama içi bir market sistemi: Skiller, Agentlar, Providerlar ve Flow taslakları paylaşılıp kurulabilsin. Büyük özellik → önce kapsamlı tasarım, sonra MVP'nin ilk dikey dilimi (yerel skill listeleme/içe aktarma).

**Tasarım dokümanı:** `_Docs/21-MARKET.md` — paket formatı (**SwarmPack v1**: manifest zarfı + tür-özel payload), çok-katmanlı (bundled→global→workspace) dosya-tabanlı registry (`skills.Store` kardeşi), tür-başına install/publish/sanitize akışı, API yüzeyi, UI sekmeleri, MVP kapsamı ve sonraki adımlar.

**Mimari kararlar:**
- **Dosya-tabanlı, bağımlılıksız registry** — `*.swarmpack.json` dosyaları; manifest ucuz taranır, payload yalnız detay/kurulum anında lazy okunur (skills body-lazy kalıbı).
- **Sır sızdırmaz** — provider paketi `keyEnc` taşımaz (kurulumda kullanıcı kendi anahtarını girer); agent paketi ID/CreatedBy/secret taşımaz; flow/agent paketleri agent-agnostik.
- **Üç katman** — bundled (`//go:embed defaults`) → global (`<DataDir>/market`) → workspace (`<workspace>/market`); workspace > global > bundled çakışmada kazanır. Publish workspace tier'a yazar.

**MVP dikey dilim (skill türü uçtan uca):**
- **Backend `internal/market` (yeni):** `pack.go` (SwarmPack + 4 tür payload tipi, schema sabitleri), `store.go` (tier tarama + lazy `Get` + `Publish`/`Import` + `ListKind`), `install.go` (`InstallSkill`: SKILL.md'yi workspace skills dizinine yazar, overwrite guard), `publish.go` (`BuildSkillPack`), `defaults.go` (`//go:embed defaults` + `EnsureDefaults`, skills aynası), `defaults/` (2 gömülü başlangıç paketi: web-research, code-review), `store_test.go` (defaults→list→get→install→conflict→overwrite→publish E2E).
- **`agent/runtime.go`:** `market *market.Store` alanı + `Market()`/`WorkspaceSkillsDir()` accessor'ları + `marketGlobalDir()`/`workspaceMarketDir()` dizin yardımcıları + boot'ta `market.EnsureDefaults`.
- **API `internal/api/market.go` (yeni):** `GET /api/market` (`?kind=` filtresi, manifestler), `GET /api/market/{id}` (payload dâhil), `POST /api/market/{id}/install` (skill → workspace + skills.Reload; diğer türler 501), `POST /api/market/publish` (skill slug → ham SKILL.md kayıpsız paketlenir), `POST /api/market/import` (ham JSON), `POST /api/market/reload`.
- **Frontend:** `types/market.ts` (Pack/Payload/InstallResult), `api/market.ts` (`marketApi`, barrel'a eklendi), NavRail `market` görünümü (Store ikonu), `components/panels/MarketPanel.tsx` (tür sekmeleri Tümü/Beceri/Ajan/Sağlayıcı/Akış + kart ızgarası + detay çekmecesi: skill body markdown önizleme + "Bu workspace'e kur"; diğer türler "yakında" rozetli), `App.tsx` wiring.

**Durum:** `go build ./...` + `go vet` + `go test ./internal/market/... ./internal/api/... ./internal/agent/...` + frontend `tsc`/`vite build` yeşil. **Canlı API E2E** (izole instance): list→get(payload lazy)→install→skiller arasında görünür, publish→workspace tier dosyası yazıldı, re-install çakışması 409 / overwrite 200 doğrulandı. Sonraki dilimler: agent/provider/flow install+publish, import/export UI, Playwright UI smoke.

## Sohbete Hedef (Goal) mekanizması ✅ (2026-06-18)

**İstek:** Sohbet/oturum detay ekranına kalıcı bir **Hedef (Goal)** alanı eklensin; ajanlar ve context bunu kullanabilsin. Claude Code'un `/goal` mekanizması (kalıcı, ölçülebilir bir "kuzey yıldızı" hedefi; her turda korunur) örnek alındı — otonom döngü/checker kısmı kapsam dışı, yalnız **kalıcı-hedef-enjeksiyonu** uyarlandı.

**Mimari kararlar:**
- Hedef **oturum-kapsamlı** (`db.Session.Goal`), ajan-kapsamlı değil — her sohbet kendi hedefini taşır.
- Enjeksiyon noktası `composeTurnRequest` **dinamik (cache'siz) suffix'inin EN BAŞI** — hedef, statik persona'dan hemen sonra okunan ilk şey (memory/summary/artifact bloklarından önce). Tek kaynak: `goalContextBlock` (`api/goal.go`).
- `UpdatedAt` bump edilmez → hedef düzenlemek oturum listesini yeniden sıralamaz (başlık `SetSessionTitle` aksine, ama `MarkSessionRead` kalıbı gibi).
- Otonom **heartbeat** turu da hedefi kullanır: `runtime.runHeartbeat` heartbeat oturumunun `Goal`'ını `SystemDynamic`'e enjekte eder (`heartbeatGoalBlock`; `api` paketine import bağımlılığı olmadan yerel ayna). Böylece "ajanlar bunu kullanabilsin" hem manuel hem otonom yolda sağlanır.

**Değişiklikler (backend):**
- `internal/db/models.go`: `Session.Goal string` (`json:"goal,omitempty"`).
- `internal/db/store.go`: `SetSessionGoal(ctx, id, goal)` (`mutateSessionLocked`, UpdatedAt bump yok, boş = temizler).
- `internal/api/goal.go` (yeni): `goalContextBlock` (north-star prompt bloğu, ≤`maxGoalLen=2000`), `handleSetSessionGoal` (`PUT /api/sessions/{id}/goal`, trim + uzunluk guard + 404).
- `internal/api/chat_turn.go`: `composeTurnRequest` dynamic suffix'ine hedef bloğu **en başa** eklendi (chat + chat_stream ortak).
- `internal/api/session_info.go`: `sessionInfoResp.Goal` + `systemFillers`'a `role:"goal"` ("Hedef") filler'ı → bağlam metresi hedefin token ayak izini gösterir.
- `internal/api/server.go`: `PUT /api/sessions/{id}/goal` rotası.
- `internal/agent/runtime.go`: `runHeartbeat` `SystemDynamic: heartbeatGoalBlock(session.Goal)` + `heartbeatGoalBlock` helper.

**Değişiklikler (frontend):**
- `types/session.ts`: `SessionInfo.goal`.
- `api/sessions.ts`: `setSessionGoal(id, goal)` (`PUT`).
- `components/sessions/SessionDetailPanel.tsx`: başlık altında **Hedef** bölümü — boş durumda "Bu sohbet için bir hedef belirle" CTA, dolu durumda accent kutuda metin, `Target` ikonu + kalem düzenle; `textarea` editör (⌘/Ctrl+Enter kaydet, Esc iptal, maxLength 2000); kaydedince local `info.goal` günceller + `localRefresh` bump (metre anında yenilenir). Bağlam metresi lejandına `goal` rengi (rose `#f43f5e`).

**Doğrulama:** `go build`/`vet`/`test ./internal/api,db,agent` + frontend `tsc --noEmit` yeşil. **API E2E** (izole 8090 instance, gerçek backend): session oluştur → `PUT goal` → `/info` goal round-trip ✅, "goal" filler `/info`'da görünür ✅, boş body ile temizleme ✅. **Playwright canlı test** (5174 → 8090): detay panelinde Hedef bölümü render ✅, boş-durum CTA → textarea → metin yaz → Kaydet → metin paragraf olarak görünür + "Hedefi düzenle"ye döner ✅, ↻ sonrası bağlam metresinde "Hedef: ~107" bucket'ı çıkar ✅ (enjeksiyon kanıtı).

### Hedef "tamamlandı" durumu ✅ (2026-06-18, ek dilim)

Claude `/goal` **checker yakınsaması**: hedef tamamlanınca metni korunur ama **context enjeksiyonu durur** (achieved → artık turları yönlendirmez); yeniden açılabilir.

- **Backend:** `db.Session.GoalDone` (+`SetSessionGoal(ctx,id,goal,done)` — boş hedefte done daima false). `goalContextBlock(goal, done)` ve `heartbeatGoalBlock(goal, done)` done iken `""` döner → ne chat ne heartbeat turuna enjekte edilir. `PUT /api/sessions/{id}/goal` artık `{goal, done}` alır (boş hedefte done normalize edilir); `/info` `goalDone` döner ve done iken "Hedef" filler'ı düşer.
- **Frontend:** `SessionInfo.goalDone`, `api.setSessionGoal(id, goal, done=false)`. `SessionDetailPanel`: tamamlanmış hedef üstü-çizili + yeşil tonlu kutu + "Tamamlandı · enjekte edilmiyor" rozeti; **Tamamlandı / Yeniden aç** toggle butonu (`CheckCircle2`/`Circle`); hedef metni düzenlemek hedefi **yeniden açar** (done=false). Toggle sonrası metre yenilenir (bucket düşer/geri gelir).
- **Doğrulama:** `go build`/`test` + `tsc` yeşil. **API E2E** (izole 8091 instance): aktif→enjekte ✅, tamamlandı→enjekte yok + metin korunur ✅, yeniden aç→enjekte ✅, boş hedefte done=false'a normalize ✅. UI: tsc + temel akışın Playwright ile zaten doğrulanmış render/handler kalıbı (toggle aynı kalıp).

---

## Sağlayıcı/model seçimi + emoji picker standardizasyonu ✅ (2026-06-18)

**İstek:** Workspace ayar ekranındaki model/sağlayıcı seçimi standart değildi (her yerde farklı UI). `ProviderModelSelect` bileşeni her yerde tutarlı kullanılsın; ikon seçiminde emoji listesinden seçilebilsin (emoji picker).

**Tutarsızlıklar (önce):**
- `WorkspacePanel` (workspace ayarları): sağlayıcı için elle yazılmış `<select>` (yalnız claude-cli + anthropic — **minimax ve özel sağlayıcılar eksik**) + model için düz metin input. Diğer her yer (`AgentSettingsForm`, `AgentRoster`/sidebar, `ProvidersPanel`) zaten katalog-temelli `ProviderModelSelect` kullanıyordu.
- Emoji/ikon seçimi 3 yerde 3 farklı yöntem: agent avatarı sabit glyph listesi, workspace create sabit 10-emoji satırı, workspace ayarları düz metin input.

**Değişiklikler:**
- `ProviderModelSelect.tsx`: opsiyonel `allowInherit` modu eklendi — boş sağlayıcı = "(uygulama varsayılanı)"; bu modda model serbest metin alanı olur. Workspace override formu artık aynı katalog-temelli picker'ı kullanır (minimax + özel sağlayıcılar seçilebilir).
- `lib/emojiData.ts` + `agents/EmojiPicker.tsx`: bağımlılıksız (emoji-mart yok), kategorili + aranabilir, paylaşılan emoji seçici (agent avatar editörü buna geçirildi).
- `common/EmojiField.tsx` (yeni): kendi tetik butonu olan, `EmojiPicker` popover'ını saran tek-satırlık yeniden kullanılabilir kontrol.
- `WorkspacePanel.tsx`: sağlayıcı/model → `ProviderModelSelect allowInherit`; ikon → `EmojiField`.
- `WorkspaceCreateModal.tsx`: sabit ikon satırı → `EmojiField`.

**Durum:** `tsc -b` benim dosyalarımda temiz (kalan tek hata paralel oturumun ilgisiz `HooksPanel` WIP'inde). Commit `e29d725` (7 dosya). Push yok. Not: oturumda paralel başka bir TionSwarm session'ı aktif (session-goals + market WIP) — yalnız bu özelliğin dosyaları seçilerek commit edildi; canlı E2E, çoklu vite instance + 110+ sekme ortam gürültüsü nedeniyle yapılmadı.

**Konsolidasyon — tek merkezi ikon seçici (2026-06-18, sonradan):** `AgentSettingsForm` hâlâ `EmojiPicker`'ı doğrudan sarıp kendi tetik butonu + `emojiPickerOpen` state'ini elle yazıyordu (workspace picker'larının kullandığı `EmojiField`'ı çoğaltarak). Ajan avatarı da `EmojiField`'a geçirildi → **tüm ikon-seçimi yerleri tek kontrolden geçer** (`AgentSettingsForm` + `WorkspaceCreateModal` + `WorkspacePanel` → `EmojiField` → `EmojiPicker`). `EmojiField` opsiyonel `label` prop'u kazandı (`string | (value) => string`) — ajan formu açıklayıcı tetik metnini korur ("Emoji seç (varsayılan: baş harf)" / "Emojiyi değiştir"), workspace picker'ları kompakt `Smile` ikonunda kalır. `EmojiPicker` artık yalnız `EmojiField`'ın iç popover'ı. **Doğrulama:** `tsc -b` benim dosyalarımda temiz (kalan App.tsx `market` + `HooksPanel` hataları paralel oturum WIP'i); **Playwright canlı test** (5173→8090): ajan formunda `EmojiField` tetiği + paylaşılan picker (arama/sekme/ızgara) açılışı doğrulandı; daha önce uçtan uca seç→kaydet→`avatar` kalıcılığı da doğrulanmıştı. Commit `2dd73f4` (AgentSettingsForm + EmojiField). Ayrıca dev-ortam fix'i: `vite.config.ts` proxy `:8080`→`:8090` (8080 unity-mcp ile çakışıp tüm `/api`'yi 404'lıyordu) — commit `56d3c20`. Push yok.

## Board arası görev bağımlılığı (Task Dependencies) ✅ (2026-06-18)

**İstek:** Görevler arasında "önce şu tamamlanmalı" bağımlılığı tanımlanabilsin; kart oluşturma/güncelleme bu bilgiyi kabul etsin; kart detay panelinde bağımlılık chip'lerine tıklayınca ilgili göreve geçilsin; Board ekranında bağımlılık sırasına göre kartları sıralayan buton eklensin.

**Mimari kararlar:**
- `Task.Dependencies` alanı modelde zaten mevcuttu (JSON string array of task IDs); yalnız API/UI katmanlarına açılması gerekiyordu.
- Sıralama için topological sort (`topoLevels`): bağımlısı olmayan görevler level-0 (en üste), zincirleme bağımlılıklar artan level. Döngüler cycle guard ile kırılıyor (tekrar edilen node'a level-0 atanır).
- `DependencyPicker` ayrı component dosyasında — `TaskDetailPanel`'den bağımsız, yeniden kullanılabilir.

**Değişiklikler:**
- `internal/api/tasks.go`: `createTaskReq.Dependencies` + `updateTaskReq.Dependencies` alanları eklendi; `handleCreateTask`/`handleUpdateTask` bu alanı okuyup `db.Task.Dependencies`'e yazar.
- `internal/tools/builtin_taskmgmt.go`: `create_task`/`update_task` agent araçlarına `dependencies` alanı schema'ya eklendi; `Call()` metotlarında işleniyor.
- `frontend/src/api/tasks.ts`: `createTask` params'a `dependencies?: string`, `updateTask` Pick tipine `'dependencies'` eklendi.
- `frontend/src/components/panels/DependencyPicker.tsx` (yeni): Checkbox listesi — tüm görevleri gösterir, seçilenler bağımlılık olur, `boardState` renk kodu (done=yeşil).
- `frontend/src/components/panels/TaskDetailPanel.tsx`: `tasks?: Task[]` + `onSelectTask?: (id: string) => void` props; `DependencyPicker` entegrasyonu; bağımlılık chip'leri (done=yeşil, pending=turuncu) tıklayınca `onSelectTask` çağrılır; kaydet `dependencies: JSON.stringify(depIds)` gönderir.
- `frontend/src/components/panels/TaskBoard.tsx`: `parseDeps()` + `topoLevels()` helper fonksiyonları; `depSort` state + "🔗 Sırala" toggle butonu; kolon içi topological sıralama; kart üstünde `🔗 N` rozeti (tamamlanmamış bağımlılık varsa turuncu, hepsi bittiyse yeşil); `TaskDetailPanel`'e `tasks` ve `onSelectTask` props iletiliyor.

**Doğrulama:** `go build ./...` + `tsc --noEmit` yeşil. Playwright canlı testi: dependency chip click-to-navigate ✅, DependencyPicker checkbox seçimi ✅, kart rozeti ✅, "🔗 Sırala" butonu görünür ✅.

## Bağımlılık ekleme → Sürükle-Bırak (drag-drop) ✅ (2026-06-18)

**İstek:** Detay panelinde checkbox listesi yerine, kartı sürükleyip detay paneline bırakarak bağımlılık eklenebilsin.

**Değişiklikler:**
- `frontend/src/components/panels/TaskBoard.tsx`: `onDragStart` handler'a `e.dataTransfer.setData('application/x-tionswarm-task', t.id)` + `effectAllowed = 'link'` eklendi — kartlar artık task ID'lerini taşır.
- `frontend/src/components/panels/TaskDetailPanel.tsx`:
  - `DependencyPicker` (checkbox liste) kaldırıldı; yerine kesik-çizgili **drag-drop zone** eklendi (`🔗 Kartı buraya sürükle`).
  - Drop sırasında `dataTransfer.getData('application/x-tionswarm-task')` okunup `depIds`'e ekleniyor (self-dep, duplicate ve geçersiz ID koruması mevcut).
  - `onDragEnter`/`onDragLeave` (currentTarget.contains guard ile) → accent rengi hover feedback.
  - Dependency chip'lerine `×` kaldırma butonu eklendi — tıklama hâlâ navigate ediyor, × kaldırıyor.

**Doğrulama:** `tsc --noEmit` yeşil. Playwright canlı: "Sürükle Bırak Testi" kartından drop zone'a drag simüle edildi → `⏳ Sürükle Bırak Testi` chip'i ve `×` butonu detay panelinde göründü ✅.

## Dependency döngüsü uyarısı ✅ (2026-06-18)

**İstek:** A→B→C zincirinde C'ye A'yı sürükleyince döngü engellenip uyarı gösterilsin.

**Değişiklikler (`frontend/src/components/panels/TaskDetailPanel.tsx`):**
- `hasCycle(startId, targetId, allTasks)` helper: DFS ile bağımlılık grafiğini dolaşarak döngü varlığını kontrol eder (visited set ile sonsuz döngü korumalı).
- `cycleWarning: boolean` state + `cycleTimerRef` — drop sonrası 3 saniye kırmızı uyarı, sonra otomatik sıfır.
- `handleDepDrop`: ekleme öncesi `hasCycle` çağrılır; döngü varsa uyarı gösterilip erken dönülür.
- Drop zone: üç durum — normal (kesik çizgi), hover (accent mavi), cycle (kırmızı `🔄 Döngü — bu görev zaten sizi bekliyor`).

**Doğrulama:** `tsc --noEmit` yeşil. Playwright evaluate: `Temel → İkinci → Üçüncü` zincirinde "Üçüncü iş" kartı "Temel görev" detay paneline drop edildi → zone metni `🔄 Döngü — bu görev zaten sizi bekliyor`, `color-danger` class true, bağımlılık chip'i eklenmedi ✅.

---

## Board sütun düzenleme (BoardColumnEditor) ✅ (2026-06-18)

**İstek:** Kanban board'undaki sütunlar yeniden adlandırılabilsin, renklendirilsin, sıralanabilsin, eklenip silinebilsin. Sol tarafta panel ile yönetim.

**Mimari kararlar:**
- Sütun tanımları `ws-settings.json`'a `boardColumns []BoardColumnDef` olarak kaydedildi — yeni endpoint gerekmedi, mevcut workspace settings altyapısı kullanıldı.
- `ValidBoardState` (5 sabit durum) linter tarafından korunduğundan yeni `IsValidBoardKey()` fonksiyonu ayrı `db/board_columns.go` dosyasında eklendi; lowercase+rakam+alt-çizgi desenini kabul eder, 5 sabitin ötesinde özel anahtarlara izin verir.
- API katmanı (`handleCreateTask`, `handleUpdateTask`) `IsValidBoardKey` kullanır → özel sütun anahtarları HTTP üzerinden kabul edilir.
- Agent araçları (`builtin_taskmgmt.go`) linter nedeniyle `ValidBoardState` ile kalmaya devam eder — agent araçları hâlâ yalnız 5 sabit durumu kabul eder.

**Değişiklikler:**
- `internal/db/board_columns.go` (yeni): `IsValidBoardKey()`
- `internal/db/models_task.go`: `BoardColumnDef` struct + `DefaultBoardColumns()`
- `internal/workspace/settings.go`: `WSSettings.BoardColumns` + `WSSettingsPatch.BoardColumns`
- `internal/api/workspace_settings.go`: `workspaceSettingsDTO.BoardColumns` + fallback
- `internal/api/tasks.go`: validasyon `IsValidBoardKey`'e geçildi
- `frontend/src/types/workspace.ts`: `BoardColumnDef` interface + `WorkspaceSettings.boardColumns`
- `frontend/src/types/task.ts`: `BoardState` özel string'e açıldı
- `frontend/src/components/panels/BoardColumnEditor.tsx` (yeni): sol panel — sürükle-sırala, etiket düzenle, renk seçici (9 preset + özel hex + native color input), sil (görev varsa korumalı), ekle, kaydet
- `frontend/src/components/panels/TaskBoard.tsx`: sütunları workspace settings'ten yükle/kaydet, `⊞ Sütunlar` butonu, sütun başlıklarına renk uygulama
- `frontend/src/components/panels/TaskDetailPanel.tsx`: `columns` prop, durum pill seçici renk desteği

**Doğrulama:** `go build ./...` + `tsc --noEmit` yeşil. Playwright canlı: Board ekranı açıldı, sütunlar doğru görüntülendi, `⊞ Sütunlar` butonu editor paneli açtı.

## Aktivite paneli — canlı çalışma durumu göstergesi ✅ (2026-06-18)

**Sorun:** Bir schedule tetiklenip ajan yanıt üretirken aktivite listesindeki execution'da `running: false` görünüyordu. Detay panelinde de "çalışıyor" bilgisi yalnızca chat streaming sessionlarında vardı; schedule/heartbeat otonom invockelarda yoktu.

**Çözüm:**
- **`internal/agent/runtime.go`:** `activeSessions sync.Map` alanı + `trackSession`/`untrackSession`/`ActiveSessionIDs()` metotları eklendi. Otonom çalışmalar (schedule/heartbeat) için session takibi sağlar.
- **`internal/agent/scheduler.go`:** `deliverPrompt`'ta `invokeTraced` çağrısı öncesi `rt.trackSession(session.ID)`, sonrasında `rt.untrackSession(session.ID)` eklendi.
- **`internal/agent/runtime.go`** (`runHeartbeat`): `CompleteWithTools` çağrısı etrafına aynı track/untrack sarıldı.
- **`internal/api/executions.go`:** `handleListExecutions`'da `chatRuns.activeSessionIDs()` + `wsp.Runtime.ActiveSessionIDs()` birleştirildi → schedule ve heartbeat sessionları da `running:true` gösterir.
- **`frontend/src/components/panels/ExecutionsPanel.tsx`:** Seçili execution `running:true` iken mesajlar her 2 saniyede bir (`RUNNING_POLL_MS`) yeniden çekilir (chat streaming beklemeden transkript güncellenir). Detay panelinde animasyonlu "Yanıt hazırlanıyor…" banner'ı (Loader2 spinner + nabız eden noktalar) eklendi.

**Playwright doğrulaması (2026-06-18):** Aktivite paneli hatasız açıldı, "Henüz yürütme yok" mesajı doğru görüntülendi. `go build ./internal/...` + `tsc --noEmit` yeşil.

## `schedule_wake` — Ajanın kendi sohbetine geri dönmesi ✅ (2026-06-18)

**Sorun:** claude-cli sağlayıcısı `claude -p` (one-shot print mode) ile çalışır; alt süreç her turda ölür. Claude Code'un yerleşik `ScheduleWakeup` aracı yalnızca `/loop` harness içinde anlamlıdır — TionSwarm'da harness olmadığından ajan "bekliyorum" diyip hiçbir şey gelmeden askıda kalıyordu.

**Çözüm — TionSwarm'ya özel `schedule_wake`:**
- **`internal/db/models_task.go`:** `Schedule` struct'ına `OneShot`, `FireAt`, `SessionID`, `Reason` alanları eklendi.
- **`internal/agent/scheduler.go`:** `wakeTimers map[string]*time.Timer` alanı; `rebuildLocked` one-shot satırları cron tablosuna eklemiyor, bunun yerine `armWakeLocked` ile zamanlıyor; `fireWake` → `deliverWake` → orijinal sohbet oturumuna prompt enjeksiyonu; `emitWakeEvent` → frontend'e `phase=start/done` gönderir; `Stop()` timer'ları iptal eder. `maxWakeDelay = 1 saat`.
- **`internal/agent/runtime.go`:** `ScheduleWake(ctx, sessionID, agentID, prompt, reason string, delaySeconds int)` — one-shot schedule satırı oluşturur, `reloadSchedules`'ı tetikler, 5–3600s sıkıştırır.
- **`internal/tools/builtin_wake.go`** (yeni): `WakeFunc` tipi, `WithWakeScheduler`/`wakeFrom` context bağlantısı, `ScheduleWakeTool` (araç adı: `schedule_wake`).
- **`internal/agent/toolsetup.go`:** `NewScheduleWakeTool()` built-in listesine eklendi (her ajanda, ask_user/todo_write ile aynı seviyede).
- **`internal/api/chat_stream.go`:** Her yanıt turunda `wakeFn` closure oluşturulup `tools.WithWakeScheduler(turnCtx, wakeFn)` ve `run.setWakeScheduler(wakeFn)` ile bağlandı.
- **`internal/api/chat_control.go`:** `chatRun`'a `wake WakeFunc` alanı + `setWakeScheduler`/`wakeScheduler` metotları.
- **`internal/api/mcp_interaction.go`:** `schedule_wake` Interaction MCP araç listesine eklendi; `callWake` dispatch handler; claude-cli'ın kırık yerleşik `ScheduleWakeup`'u devre dışı listesine alındı (`climcp.go`).
- **`internal/api/schedules.go` + `internal/tools/builtin_schedulemgmt.go`:** One-shot wake satırları liste endpointlerinden filtrelendi (UI'da gereksiz gürültü önlenir).
- **`frontend/src/App.tsx`:** `chat` tipi event'lerde `phase=start` → `markPending([sid])`, `phase=done` → `clearPending(sid)` — wake beklerken thinking göstergesi.
- **Test:** `internal/agent/wake_test.go` — 3 test: `TestScheduleWake_ArmsOneShot` (DB satırı + delay sıkıştırma), `TestDeliverWake_TargetsOriginalSession` (wake orijinal sohbet oturumuna enjekte edilir), `TestScheduler_StartSkipsOneShotCron` (cron tablosuna eklenmez, timer kurulur).

**Canlı doğrulama (Playwright, 2026-06-18):**
1. TionSwarm arayüzünde yeni oturum açıldı.
2. Ajana "10 saniyede `schedule_wake` kullan" mesajı gönderildi.
3. Agent `mcp__tionswarm_interaction__schedule_wake` çağırdı, "kurdum, bekliyorum" yanıtı verdi.
4. **10 saniye sonra** wake prompt (`Wake up! Say hello...`) **aynı sohbet oturumuna** otomatik enjekte edildi.
5. Agent yeniden yanıt verdi — sohbet akışı ekranda görünür şekilde devam etti. ✅

✅ `go test ./internal/agent/ ./internal/db/ ./internal/api/` + `go vet ./internal/...` + `npm run build` yeşil. Detay: **`_Docs/20-SCHEDULE-WAKE.md`**.

## Kanban → pasif durum panosu dönüşümü (2026-06-18, COMMITSİZ)

Pano artık bir **çalıştırma yüzeyi değil**, pasif bir durum/bilgi panosu. İş flow/schedule/agent oturumlarında yapılır; kartlar yalnızca durumu yansıtır.

- **Karttan kaldırılanlar (UI):** ▶ çalıştırma, ⏰ cron bağlama, çalıştırma geçmişi, canlı etkinlik ve **prompt** alanı. Kartta artık çıktı/son-durum rozeti yok.
- **Oluşturma:** prompt yerine **açıklama**; başlık açıklamadan otomatik üretilir (`handleCreateTask` artık `description || prompt`'tan üretir). Ajan + Flow kart üstünde **opsiyonel, bilgi amaçlı** etiket.
- **Panel:** başlık (⟳ açıklamadan), açıklama (birincil), ajan (info), flow (info), durum (kolon), sil. Sürükle-genişlet (resize) `useResizableWidth` hook'u + localStorage.
- **Backend temizliği (`kaldır temizle`):**
  - **`run_task` agent tool kaldırıldı** — `builtin_taskmgmt.go` artık 5 araç (list/create/update/move/delete); ajan görevi çalıştırmaz, durumu günceller. `toolsetup.go` kaydı + `TestRunTaskInvokesRunner` silindi.
  - **Schedule↔task (karttan-cron) bağlama kaldırıldı** — `db.Schedule.TaskID` alanı, `scheduler.run`'ın `if sc.TaskID != "" { RunTask }` dalı, `api/schedules.go` `taskId` alanları/validasyonu (artık `prompt` zorunlu), `store_schedule.go` kopyalaması, frontend `Schedule.taskId` + `createSchedule/updateSchedule` `taskId` paramı. Schedule artık yalnız prompt teslim eder.
- **Yetim run yolu da temizlendi (2026-06-18):** pano pasif olduğundan tüm görev-çalıştırma yolu kaldırıldı —
  - **executor.go:** `RunTask`/`RunTaskStream`/`runTaskFlow` + yalnız bunların kullandığı `invoke`/`invokeWithMemory`/`invokeWithMemoryStream`/`recordRunReply`/`taskSession`/`TaskSession` silindi. Kalan: `renderFlowTranscript` (flow.go kullanıyor), `invokeTraced` (scheduler), `complete` (flow.go). `errors`+`events` import'ları düştü.
  - **api:** `handleRunTask` + `handleListTaskRuns` (tasks.go) + `tasks_stream.go` dosyası tamamen silindi; `server.go`'dan `/run`·`/run-stream`·`/runs` rotaları kaldırıldı.
  - **db:** `store_run.go` yalnız `ListRunningRuns`'a indi (activity.go kullanıyor); `CreateRun/FinishRun/SetRunSession/ListRuns/GetRun/persistRunLocked` + `store_task.go SetTaskLastRun` silindi. `db.Run` modeli + `Task.LastRun*` korundu (executions feed + `list_tasks` okuyor).
  - **frontend:** `api/tasks.ts`'den `runTask/runTaskStream/streamRunTask/listTaskRuns/TaskStreamHandlers` + `prompt` paramları kaldırıldı; import'lar sadeleşti.
- **Durum:** `go build`/`go test ./internal/...` + frontend `tsc`/`npm run build` yeşil. **Commit beklemede** (smart-surge reconcile).

## Executions ekranı — session ID gösterimi + bildirim deep-link'leri (2026-06-18)

**İstek:** Her yürütme satırında session ID göster; zamanlama ve akış bildirimlerine tıklanınca Executions ekranında ilgili oturum açılsın.

### Session ID görünürlüğü (`ExecutionsPanel.tsx`)
- **Liste satırı:** Her öğenin altına `#shortId(sessionId)` — son 8 karakter, `font-mono text-[10px] opacity-60`.
- **Detay başlığı:** Tıklanabilir kopyalama butonu `<Copy size={10} /> #shortId(sessionId)` — hover'da accent rengi, `navigator.clipboard.writeText(fullId)` tam ID'yi panoya yazar.
- **`shortId` yardımcısı:** Kompakt liste görünümü için; tam ID detay başlığında (title attribute) ve clipboard'da mevcut.
- **Deep-link props:** `focusId?: string | null` (route'dan gelen sessionId) + `onSelectExecution?: (sessionId: string) => void` (URL sync için yukarı bildirir). `useEffect` focusId değişince selectedId'yi günceller.

### Zamanlama bildirimleri → Executions (`scheduler.go`)
- `emitPromptDelivery` her iki yol (başarı + oturumlu-hata) `{"view": "chat"}` → `{"view": "executions", "sessionId": sessionID}` olarak değiştirildi.
- Artık zamanlamadan tetiklenen oturumun transkribi bildirime tıklayınca Executions ekranında doğrudan açılır. (Session'sız hata fallback'i `{"view": "logs", "agentId": ...}` olarak korundu.)

### Akış bildirimleri → Executions (`flow.go`)
- `RunFlowRecorded`'a `autonomous` parametresi guard'ı + `emitFlowDelivery` yeni metodu eklendi.
- Akışlar önceden **hiç bildirim göndermiyordu**; artık otonom çalıştırmalarda başarı veya hata bildirimi gönderilir.
- Her iki bildirim `{"view": "executions", "sessionId": sessionID}` hedefler → tıklama Executions'ta ilgili akış oturumunu açar.

### URL routing (`url.ts`, `App.tsx`)
- `routeFromEvent`: `view === 'executions'` → `t.sessionId` işlendi (yalnız `chat` işleniyordu).
- `routeIdForView`: `executionId: string | null` state parametresi + `case 'executions': return state.executionId`.
- `App.tsx`: `executionTarget` state + `applyRoute` `executions` dalı + `pendingRouteRef` workspace-yükleme handler'ı; `ExecutionsPanel`'e `focusId={executionTarget}` + `onSelectExecution={setExecutionTarget}`.

✅ `go build ./...` + `npx tsc -b` yeşil. Chrome canlı testi bu turda gateway bağlantısı kopuk olduğundan yapılamadı; sunucular sağlıklı (Go + Vite HMR).

## Ayarlar yeniden düzenleme — bölüm taşımaları (2026-06-18)
İki ayar bölümü daha mantıksal olarak ait oldukları ekrana taşındı:
- **"Anthropic beta"** (1M token bağlam + uzatılmış prompt cache) → **Sağlayıcılar**'dan **Bağlam & Bellek**'e alındı (`appPanels.tsx ContextPanel`, `FlaskConical` başlık ikonu). Bu seçenekler bağlam penceresi/cache davranışını etkilediği için Bağlam ekranıyla daha uyumlu. `ProvidersPanel` artık `Toggle`/`FlaskConical` kullanmıyor (import temizlendi).
- **"Harici token araçları"** (PATH'te token-optimizasyon CLI'ları tespiti) → **Gelişmiş → Tanılama**'dan **Hooks**'a alındı (`HooksPanel.tsx`, kendi `tools/checking/toolsErr` state'i + `systemApi.externalTools`). Bu araçlar (ör. `sqz`) hook komutlarında kullanıldığı için Hooks ekranıyla daha alakalı. `DiagnosticsPanel` artık yalnız "Log seviyesi" alanını içeriyor.

✅ `tsc -b` yeşil. (Chrome canlı testi gateway bağlantısı kopuk olduğundan bu turda yapılamadı; sunucular sağlıklı, Vite HMR yansıtır.)

## Ara özellik — Tur-içi crash kurtarma (inflight sidecar) (2026-06-18)

**İstek:** "bu sessionda network-error hatası verdi … büyük ihtimalle TionSwarm yeniden başladı, ve ekranı yenileyince agentın yarım konuşması kayboldu" → **restart-dayanıklılığını çöz.**

**Teşhis:** Asistan yanıtı `session.jsonl`'e yalnızca stream **bitince** (`chat_stream.go` AddMessage) persist ediliyordu. Süreç tam stream sırasında ölünce (dev rebuild/OOM) yanıt hiç yazılmamış oluyor → yenilemede tur kayboluyordu. Mevcut "detach from client" koruması yalnız **sayfa yenilemesini** kurtarıyordu, **süreç restart'ını değil**.

**Çözüm — inflight sidecar:** Her stream'lenen tur, oturum dizinine throttle'lı (~600ms) atomik `inflight.json` anlık görüntüsü yazar (kısmi metin + kalıcı iz). Normal her çıkışta silinir (`ClearInflight` defer + persist sonrası); yalnız gerçek crash sidecar'ı bırakır. Boot'ta `recoverInflight` orphan sidecar'ı `Interrupted=true` asistan mesajı olarak materialize eder — `MessageID` paylaşımı ile **idempotent**.

- **Backend:** `db/inflight.go` (`InflightTurn`/`WriteInflight`/`ClearInflight`/`recoverInflight`), `db.Message.Interrupted`, `AddMessage` boş olmayan ID'yi korur, `db.go` load() → recovery, `api/chat_stream.go` tur başına snapshot closure + temizlik.
- **Frontend:** `types/message.ts` `interrupted?`, `chat/MessageList.tsx` ⚠ "Bu yanıt yarıda kesildi" banner'ı.
- **Test:** `db/inflight_test.go` (materialize + idempotent + already-persisted skip) ✅.

✅ `go build/vet` + `go test ./internal/db ./internal/api` + frontend `tsc --noEmit` yeşil. **Canlı E2E** (izole instance, gerçek binary + HTTP API, port 18099): session oluştur → diske `inflight.json` yaz (crash simülasyonu) → süreç öldür → restart → `GET messages` `interrupted:true` kurtarılmış mesajı döndü + sidecar temizlendi. Detay: **`_Docs/08-DEPOLAMA.md`** (Tur-içi crash kurtarma + external-agent-oss karşılaştırması). **Kalan (ops.):** (1) gerçek sağlayıcılı canlı turda stream sırasında crash + UI banner'ının görsel (Playwright) doğrulaması; (2) external-agent'tan devşirilebilecek **stale-session watchdog** (olay düşünce "düşünüyor…"da takılan oturumu sunucudan tazele) ve **`preserved_stale_messages`** kuralı (yeniden yükleme istemcideki mesajları silmesin) — şu an TionSwarm'da yok.

## Faz P4 — Hooks (PreToolUse / PostToolUse) (2026-06-18)

**İstek:** "Projeye Pre-Post hookları ekleyeceğiz (yapılacaklar listesinde mevcuttu) nasıl ekleyebiliriz" + "https://github.com/ojuschugh1/sqz bunu kullanabilmek için pre-post hook mu gerekiyor".

Kullanıcı-tanımlı dış komutların **native (anthropic/minimax) araç döngüsünde** her araç çağrısının etrafında çalışması. **Claude Code hook sözleşmesi** (stdin JSON → stdout JSON, `exit 2`=engelle) ile uyumlu — aynı script'ler (ör. **sqz**) değişmeden çalışır.

- **PreToolUse:** araç öncesi — girdiyi yeniden yazar (`updatedInput`), otomatik onaylar (izin kapısını atlar) veya engeller.
- **PostToolUse:** araç + token sıkıştırması sonrası — çıktıyı dönüştürür (`updatedOutput`, dış sıkıştırma), bağlam ekler (`additionalContext`) veya engeller.
- **Kapsam:** yalnız native yol. claude-cli kendi `~/.claude/settings.json` hook'larını okur (P3 CLI-vs-native deseni). **sqz cevabı:** sqz'in kendisi bir PreToolUse hook'tur; claude-cli ajanlarında `sqz init --global` yeterli (TionSwarm'da bir şey gerekmez), native ajanlarda TionSwarm PostToolUse hook'u olarak `sqz` tanımlanır.
- **Veri:** `db.Hook` (`models_hook.go`/`store_hook.go`, per-ws `store/hooks/*.json`, `ListEnabledHooksByEvent`).
- **Motor:** `agent/hooks.go` — `runPreToolHooks`/`runPostToolHooks`/`execHook` (subprocess timeout 30s clamp 1–120 + 64KB çıktı cap + **fail-open**; matcher=araç-adı glob, boş=hepsi; oluşturma sırası zincir, ilk block kazanır).
- **Entegrasyon:** `toolloop.go` → Pre (permGate öncesi) + Post (compactToolResult sonrası); iz kartı `StepHook` (`trace.go`).
- **API:** `GET/POST/PUT/DELETE/toggle /api/hooks` (`api/hooks.go` + `server.go` route).
- **Frontend:** Ayarlar → **Hooks** (`settings/HooksPanel.tsx`), sohbet `chat/HookStep.tsx` (🪝), `types/hook.ts`+`api/hooks.ts`+`stepKinds.ts`.
- **Test:** `store_hook_test.go` (CRUD+reopen), `hooks_test.go` (matcher+execHook block/modify/allow).

✅ `go build/vet/test` + `tsc -b`/`vite build` yeşil. **Canlı API round-trip** (izole instance, port 8099): create (type/createdAt varsayılanları) → geçersiz event/boş komut 400 → update → toggle → kalıcı liste → delete uçtan uca doğrulandı. Detay: **`_Docs/18-HOOKS.md`**. **Kalan (ops.):** gerçek sağlayıcılı uçtan uca tur testi; claude-cli için `settings.json` hook üretimi.

## NavRail canlı iş belirteçleri (busy indicators) (2026-06-18)

**İstek:** "Sohbet/Görevler/Zamanlamalar/Akışlar ekranında bir iş devam ediyorsa soldaki navbar'da bunu belirten bir belirteç ekleyebilir miyiz?"

Sol nav item'larında o görünümde **canlı iş** varsa nabız atan (animate-pulse) accent nokta gösterilir.

- **Backend:** yeni `GET /api/activity` (`internal/api/activity.go`) → `{chat,task,flow,schedule}` boolean'ları. Kaynaklar kalıcı durumdan türetilir: chat = in-flight streaming turn (chat-kind session, `chatRuns.activeSessionIDs`), task = çalışan run (`db.ListRunningRuns` — yeni; her trigger), flow = çalışan flow run (`db.ListRunningFlowRuns`), schedule = trigger==schedule olan çalışan run. Hem otonom (heartbeat/cron) hem interaktif koşuları kapsar.
- **Frontend:** `hooks/useActivity.ts` 3sn'de bir poll → `Set<View>` (chat/board/flows/schedules); bu pencerenin canlı chat stream'i (`chat.streamingSessions`) anlık merge edilir (poll gecikmesi yok). `NavRail` `busyViews` prop'u ile item'a nabız noktası basar (daraltılmış rail'de köşe noktası).

✅ `go build`/`go vet`/`go test ./internal/...` + `tsc -b` yeşil. **Playwright canlı (izole 8091 backend + test vite):** `/api/activity` baseline hepsi-false → gerçek task koşarken `task=true` → bitince false (lastRunStatus=success); fetch-override ile `task/flow=true` döndüğünde Görevler+Akışlar item'larında nokta render doğrulandı.

## URL routing — bildirim tıklaması deep-link'e bağlandı (2026-06-18)

**İstek:** "Deep-link URL'lerini bildirim tıklamalarına bağlamayı ekleyebilir misin."

Otonom olay (task/schedule/heartbeat) masaüstü bildirimine tıklayınca artık **deep-link URL'ine** gidilir.

- Yeni `lib/url.ts` `routeFromEvent(e)`: olay `target` hint'lerini (`view` + `sessionId`/`agentId`) `Route`'a çevirir (`chat`→sessionId, `agents/memory`→agentId, diğerleri view-level; geçerli view yoksa `null`). + `isView` helper.
- `App.tsx` `notify(... onClick)` artık `window.location.hash = buildRoute(routeFromEvent(e))` yapar. Eski elle `switchWorkspace`+`setView`+`selectSession` üçlüsü **kaldırıldı** — çapraz-workspace'te eski ws'in oturum listesine bakıp ajanı yanlış set ediyordu. Hash ataması → `useUrlSync` hashchange → `applyRoute` zinciri workspace geçişini (`pendingRouteRef`) ve entity seçimini doğru yürütür.

✅ `tsc -b`/`vite build` yeşil; `routeFromEvent` **14/14** birim testi (tsx, koşuldu+silindi). **Playwright canlı:** `window.location.hash` ataması (tıklama handler'ının birebir eylemi) chat→board geçişini doğru yaptı.

## URL routing — yeni ekranlar + ayarlar alt panelleri (2026-06-18)

**İstek:** "yeni ekranlar ve ayarlar ekranında alt paneller geldi, onlara da [URL routing] ekleyebilir misin."

Deep-link routing (bkz. aşağıdaki *URL deep-link routing* girdisi), araya eklenen ekranları ve ayarlar kategorilerini de kapsayacak şekilde genişletildi.

- **Yeni görünümler** (`lib/url.ts` `VIEWS`): `executions`/Aktivite, `secrets`/Sırlar, `skills`/Beceriler, `budget`/Bütçe eklendi — yoksa `parseRoute` bunları `chat`'e düşürüyordu. Hepsi `#/w/{ws}/{view}` ile adreslenir.
- **Ayarlar alt panelleri**: `#/w/{ws}/settings/{kategori}` (ör. `settings/providers`, `settings/context`). `routeIdForView` `settings`→`settingsCat` döndürür; App `settingsCat` state + `applyRoute` `settings` dalı. `SettingsPanel` **kontrollü kategori** kazandı (`cat`/`onCatChange`, `ALL_CATS`/`isCat` doğrulaması, bilinmeyen→`profile`). Kategori tıklaması URL'i günceller; URL ilgili kategoriyi açar.
- **Geri/ileri düzeltmesi** (`useUrlSync`): `firstWrite` artık ilk *yazımda* değil, app hazır olduktan sonraki **ilk effect koşusunda** koşulsuz kapanır. Önceki davranışta ilk gerçek yazım (kategori tıklaması) `replaceState` ile gerçek bir geçmiş girdisini eziyor, geri tuşu yanlış sayfaya atlıyordu.
- `tools` görünümü artık entity taşımaz (workspace-scoped — ajan değil).

✅ `tsc -b`/`vite build` yeşil. **Playwright canlı test geçti:** `settings/providers` + `budget` deep-link'leri doğru render; "Bağlam & Bellek" tıklaması → `settings/context`; geri tuşu → `settings/providers` + providers içeriği (replaceState fix doğrulandı).

## Ara özellik — Araç Çıktısı Token Optimizasyonu (2 bağımsız sistem) ✅ (2026-06-18)

**İstek:** Ajan araç çıktıları (shell/dosya/MCP) modele dönmeden önce küçültülsün; iki yöntem
**bağımsız + paralel** çalışabilsin ve Ayarlar'dan konfigüre edilebilsin. İlham: `rtk-ai/rtk`
(deterministik) + `external-agent-oss` Large Response Handling (LLM özeti). Detay: `_Docs\17-TOKEN-OPTIMIZASYON.md`.

- **Sistem A — Deterministik compactor (`internal/tools/compact`):** Bağımsız, dep-siz, ücretsiz.
  Ardışık tekrar satırlarını `(×N)` ile birleştirir, boş satır bloklarını sadeleştirir, satır/bayt
  sınırını aşan çıktının ortasını UTF-8 güvenli kırpar (baş+son korunur). `Compact(output, Options)`
  → `(string, Stats)`. Tablo testleri (`compact_test.go`). Varsayılan **açık**. **Tasarruf kalıcı (2026-06-18):**
  `Stats.Saved()` → `db.AddCompactionSavings` → `Usage.CompactSavedBytes` (ajan+gün, token/maliyetten bağımsız
  ölçer; UI bağlama sonraya). `store_usage_test.go::TestAddCompactionSavings` (reload dahil).
- **Sistem B — LLM intent-aware özet (`agent/compactor.go`):** Bağımsız. A sonrası çıktı hâlâ eşik
  üstündeyse niyet-farkında özetler. **Model çözüm zinciri:** adanmış `CompactModel` → başlık modeli →
  ajan modeli; **sağlayıcı daima ajanın sağlayıcısı** (ayrı seçilemez). Özet modeli (`compactModel`)
  Ayarlar → Bağlam'dan girilebilir (boşsa zincire düşer, trim'lenir).
  `KindCompact` ile usage'a işlenir; bütçeyi gate'lemez (titler/summary/reflect ile aynı). Hata/boş
  dönüşte A çıktısına düşer (tur asla bozulmaz). Maliyetli → varsayılan **kapalı** (opt-in).
- **Entegrasyon (tek nokta):** `agent/toolloop.go` — `res` üretilip iptal kontrolünden sonra
  `r.compactToolResult(...)`; tüm araçları (built-in + MCP) kapsar. Hata sonuçları sıkıştırılmaz.
- **Ayarlar (canlı):** `settings` → `CompactToolOutput`/`CompactMaxLines`/`CompactMaxBytes` (Sistem A),
  `CompactLLMSummary`/`CompactLLMThreshold` (Sistem B), clamp'li; `applySettings`→`SetToolCompaction`,
  `Tunables` get/set. UI: Ayarlar → **Bağlam** → "Araç çıktısı sıkıştırma (Sistem A)" + "Araç çıktısı
  özeti (Sistem B)" bölümleri.
- **Doğrulama:** `go build`/`vet`/`test ./...` yeşil; canlı smoke — `/api/settings` yeni alanları doğru
  varsayılanlarla döndü (A açık, B kapalı), PUT round-trip + clamp doğrulandı (`9999999`→`262144`, `-5`→`0`).

## Faz B4 — Model Detayı + Prompt-Cache Maliyet Modellemesi ✅ (2026-06-18)

**İstek:** Bütçe ekranına model-bazlı detay satırı + prompt-cache indirimini maliyete katma.

**Prompt-cache token yakalama (provider katmanı):**
- `providers.Usage` += `CacheReadTokens`, `CacheWriteTokens` (Anthropic'in `input_tokens`'ı
  cache'i HARİÇ tutar → gerçek girdi = input + cacheRead + cacheWrite).
- `anthropic.go`: hem `Complete` hem `Stream` (`message_start`) yolunda
  `cache_creation_input_tokens` (write) + `cache_read_input_tokens` (read) parse edilir.
  Cache zaten aktifti (statik prefix'te 1h ephemeral breakpoint).

**Fiyatlama (`providers/pricing.go`):** cache çarpanları `CacheReadMult=0.10`,
`CacheWriteMult=1.25`. `Price.CostDetailed(in,out,cacheRead,cacheWrite)` her sınıfı
doğru oranla fiyatlar; `Price.CacheSavings(cacheRead)` cache okumanın tam girdi
fiyatına kıyasla sağladığı tasarrufu (×0.90) verir.

**Depolama:** `db.KindStat`+`db.Usage` += cache sayaçları; param patlamasını önlemek
için `db.UsageDelta` struct'ı eklendi, `AddUsageKind(...,delta)` imzasına geçildi.
`RecordUsage` + `recordCompaction` cache token'larını da geçirir.

**API (`/api/usage`):** `byProvider[]` artık her provider altında **`models[]`**
detayı taşır (model, çağrı, token, cache oku/yaz, maliyet, tasarruf, priced) +
provider düzeyinde cache/tasarruf; `totals` += `cacheReadTokens/cacheWriteTokens/
savingsUSD`. Maliyet `CostDetailed` ile cache-bilinçli hesaplanır.

**Frontend (`BudgetPanel`):** "Provider / model" tablosu artık **açılır satırlı**
(chevron → model detayı), **Cache (oku/yaz)** + **Tasarruf** sütunları, başlıkta
yeşil "cache tasarrufu $X" rozeti.

**Doğrulama:** `go build`/`vet`/`test` + `tsc`/`vite` yeşil; `providers/pricing_test.go`
(cache katman maliyeti + tasarruf + abonelik). **Canlı API smoke:** opus
(100k girdi + 500k cache-oku + 50k cache-yaz + 20k çıktı) → **$4.6875**, tasarruf
**$6.75**; haiku → **$0.40**; provider toplamı **$5.0875** — hepsi tam doğru.

**Sıradaki:** fiyatların Ayarlar'dan düzenlenmesi; 1h cache TTL için write çarpanı
(şu an 1.25; 1h = 2.0); TL kuru.

## Takip edilmeyen hataları yakalama — son savunma hattı ✅ (commit sonrası, 2026-06-18)

"Kimsenin takip etmediği hataları yakalayan bir sistem var mı?" sorusu üzerine
iki eksik güvenlik ağı eklendi (detay: `_Docs/12-LOGLAMA.md`):
- **HTTP panic recovery** (`api/middleware_recover.go` `withRecover`): handler
  panic'i artık stderr yerine log akışına (`Error` + stack) yazılır + temiz 500
  döner. Zincire `withRequestLog → withRecover → withWorkspace` olarak eklendi.
- **Frontend hata köprüsü**: `POST /api/logs` (`handleClientLog`) + `lib/reportError.ts`
  (`window.onerror`/`unhandledrejection` + throttle/keepalive) + `ErrorBoundary.tsx`
  (render çökmesi → rapor + kurtarılabilir fallback), `main.tsx`'te kurulu. Beyaz
  ekran ve sessiz JS hataları artık Loglar ekranında görünür.
- Testler: `middleware_recover_test.go` (panic→500+log, passthrough, client-log
  kayıt/boş-düşürme). `12-LOGLAMA.md` "Gelecek" madde 2 ✅ işaretlendi.

✅ `go build ./...` + `go test ./internal/...` + frontend `tsc -b`/`vite build` yeşil.

## Profilleme altyapısı — pprof (env-gate'li, loopback-only) (2026-06-18)

`net/http/pprof` ile profilleme eklendi: **varsayılan kapalı**, `TIONSWARM_PPROF=1` ile
açılır, yalnız loopback dinler (`TIONSWARM_PPROF_ADDR`, vars. `127.0.0.1:6060`). Uçlar
`http.DefaultServeMux`'ta yayınlanır; ana sunucu kendi mux'ını (`server.Routes()`)
kullandığından profiler uygulama rotalarından **tamamen izole**. Wiring: yeni
`cmd/tionswarm/pprof.go` (`startPprof`) + `main.go`'da config sonrası çağrı.
✅ `go build`/`vet` yeşil; **canlı smoke** (izole veri dizini, port 6061): `/debug/pprof/`
ve `/debug/pprof/heap` HTTP 200, log "pprof profiling server enabled" doğrulandı.
Kullanım rehberi (heap/CPU/goroutine toplama, `go tool pprof` komutları, sıcak-yol
adayları, before/after karşılaştırma): yeni doküman [16-PROFILLEME.md](16-PROFILLEME.md).

## Performans sağlamlaştırma turu — Go best-practice incelemesi sonrası (2026-06-18)

Go performans araştırması (Go 1.26 GC, JSON kütüphaneleri, RWMutex vs sync.Map, SSE,
allocation) sonrası **davranış-korumalı** 4 düzeltme uygulandı (commit `ad36ee9` —
not: bir auto-commit, bu hunk'ları eşzamanlı flows WIP'iyle aynı commit'e topladı):

1. **HTTP `IdleTimeout: 120s`** (`cmd/tionswarm/main.go`) — boşta keep-alive bağlantıları
   reap edilir. `WriteTimeout` **bilerek** koyulmadı (SSE akışlarını keserdi).
2. **`estimateText` → `utf8.RuneCountInString`** (`conversation/tokens.go`) ve
   **`tokenize` uzunluk kontrolü** (`memory/vector.go`) — niyet netliği + ufak hız
   (not: Go derleyicisi `len([]rune(s))` kalıbını zaten 0-alloc'a optimize ediyordu,
   dolayısıyla allocation kazancı değil, mikro hız + okunabilirlik).
3. **Recall query-norm tek hesap** (`memory/memory.go` + `vector.go`) — `cosine` artık
   `cosineNorm(a,b,anorm)` üzerine kurulu; `Recall` `norm(qv)`'i döngü öncesi **bir kez**
   hesaplar (eskiden her aday için yeniden). Her sohbet turu + her görevde çalışan sıcak
   yol. Eski `cosine` wrapper olarak korundu (davranış özdeş).
4. **RWMutex korundu** — incelemede `sync.Map`'e geçiş bilinçli olarak reddedildi
   (mixed read/write + sık yeni-key senaryosunda `map+RWMutex` daha hızlı).

**SSE'de değişiklik yok:** `events/chat_stream/tasks_stream/flows` handler'ları zaten
her event'te `Flush()` + `X-Accel-Buffering: no` + ping ticker ile doğru kurulmuş.
✅ `go build`/`vet` + `go test ./internal/...` yeşil. **Sıradaki (opsiyonel):** recall
vektör+norm bellek cache'i (`unmarshalVector` JSON parse'ı her recall'da tekrar ediyor);
pprof'u `TIONSWARM_PPROF=1` env-gate ile ekleyip gerçek yük altında baseline profil.

## Provider mimarisi incelemesi → gelecek plan (2026-06-18)

İncelenen TS referans projesinin ~70 provider'ı
nasıl düşük eforla eklediği incelendi: **"metadata'yı protokolden ayır"** deseni — ~25
OpenAI-uyumlu API tek `streamOpenAiChat` handler'ını paylaşıyor (fark sadece baseURL),
~30 CLI 4'lü diziden üretilip tek `streamGenericCliChat`'i kullanıyor, yalnız ~10 yapısal
CLI bespoke parser alıyor. Tam analiz + TionSwarm çıkarımları yeni dokümanda:
[14-PROVIDER-MIMARISI-INCELEME.md](arsiv/14-PROVIDER-MIMARISI-INCELEME.md). Yol haritasına iki
plan maddesi eklendi: **SC-1** (built-in API preset kataloğu — düşük efor) ve **SC-2**
(generic CLI factory — CLI fazı). **Yalnız plan; uygulamaya geçilmedi.**

## Reconcile durumu — "COMMITSİZ" notları çözüldü ✅ (2026-06-18)

> Aşağıdaki günlük girdilerinde **COMMITSİZ / iç içe / reconcile'a bırakıldı** olarak
> işaretlenmiş işlerin tümü artık `main` HEAD'inde commitli. Tarihli girdiler o anki
> durumu yansıttığı için olduğu gibi bırakıldı; güncel gerçek durum:

- **Özel sağlayıcılar (data-instance) — backend wiring:** ✅ Commitli. `server.go`
  `SetCustomProviders` (applySettings) + `GET/PUT/DELETE /api/providers` rotaları +
  `customProviderSpecs` HEAD'de mevcut (commit `35ec373` / `79da15d`).
- **Özel sağlayıcılar — frontend:** ✅ Commitli. `api/providers.ts` (CRUD client) +
  `ProvidersPanel` "Özel sağlayıcılar" bölümü (`CustomProviders` bileşeni) HEAD'de.
- **Provider key'leri yalnız Sır kasasından (frontend):** ✅ `ProviderKeyField`
  seçim-only akışı HEAD'de.
- **Kanban flow-backed task (executor):** ✅ Reconcile tamam — `RunFlow`/`RunFlowRecorded`
  artık 5-arg `Observer` imzasında (`flow.go:32`), beklenen reconcile gerçekleşti.

- **Frontend build (`ArtifactsPanel` WIP):** ✅ Çözüldü. `frontend` dizininde
  `npx tsc -b` + `npx vite build` (2026-06-18) temiz geçiyor; önceki günlük girdilerinde
  derlemeyi tıkadığı belirtilen `ArtifactsPanel` hatası artık yok.

## Sessizce yutulan hata denetimi + panic kurtarma ✅ (2026-06-18)

Scheduler loglama düzeltmesinin devamı olarak kod tabanı "sessizce yutulan
hatalar" için tarandı (Explore ajanı + elle inceleme). İki sınıf ele alındı
(commit `a128362`):

**1) Loglanmadan yutulan hatalar → artık `Warn`:**
- `agent/executor.go` — task transcript prompt yazımı + panoyu `in_progress`'e taşıma
  (her ikisi `_, _ =` / `_ =` ile yutuluyordu). Not: `taskSession`/`recordRunReply`
  zaten içeride logluyordu, dokunulmadı.
- `agent/flow.go` — flow transcript input/reply yazımları; **resume yolundaki
  `ParseGraph` ve state-restore hataları** (öncesi: hiç log yok, sessizce flow'u
  failed işaretliyor veya sıfırdan başlatıyordu).

**2) Panic güvenliği (yeni):** Bir node panic ederse tüm TionSwarm süreci (tüm
workspace'ler) çöküyor ve yalnız stderr'e Go stack trace düşüyordu — uygulama-içi
loglara/logbuf'a yansımıyordu. Goroutine köklerine recover + log eklendi:
- `orchestration/engine.go` — paralel child panic'i normal flow hatasına çevrilir
  (goroutine/süreç çökmez); `engine_test.go` (yeni: panic kurtarma + happy-path).
- `agent/flow.go` `driveFlow` — recover + `Error("flow run panicked")` + run'ı
  failed işaretle (UI'da "running"da asılı kalmaz).
- `agent/worker.go` `tick` — heartbeat panic'i recover + `Error` + tick hatası
  sayılır (backoff/auto-disable normal işler).

> Bilerek dokunulmayanlar (gürültü/tasarım): `json.Marshal` (string struct'ları —
> pratikte hata vermez), `mcp/client.go` non-JSON satır atlama (MCP sunucuları
> stdout'a log basar), `os.RemoveAll`/`os.Remove` best-effort temizlikler.

✅ `go build ./...` + `go test ./internal/...` yeşil. Commit edildi (push edilmedi).

### Devamı — sıralı node panic + node "error" olayı (commit `d7931c1`, 2026-06-18)

Panic-güvenliği ve gözlemlenebilirlik flow motorunda tamamlandı:
- `orchestration/engine.go` — sıralı agent node'lar artık ortak `runAgentNodeSafe`
  üzerinden koşar (paralel yolla paylaşımlı): node panic'i süreç çökmesi yerine
  normal flow hatasına çevrilir.
- **Yeni `error` Observer fazı** (`NodeEvent.Error`): hem sıralı hem paralel yol
  node başarısız olunca yayar → canlı UI spinner'ı durdurup nedenini gösterir
  (önceden "done" gelmediği için sonsuza kadar pending kalıyordu).
- Frontend: `FlowNodeEvent`'e `error` fazı + alanı; chat-tetiklemeli flow transcript
  (`useChatStream`) node hatasını satır içinde gösterir. (`FlowsPanel.tsx` canvas
  hata-halkası + canlı hata satırı eş-zamanlı "animated edges" WIP'iyle aynı
  dosyada iç içe olduğundan reconcile'a bırakıldı; `NodeStatus`/`statusRing`
  zaten 'error'ü destekliyordu.)
- Testler: `engine_test.go` sıralı panic kurtarma + error-olay yayımı.

✅ `go build ./...` + `go test ./internal/orchestration/` + frontend `tsc -b` yeşil.

## Scheduler hata loglama düzeltmesi ✅ (2026-06-18)

**Sorun (kullanıcı raporu):** Zamanlamadan (scheduler) gelen bir mesaj
başarısız olduğunda masaüstü bildirimi + oturum içi hata mesajı görünüyor ama
**loglar boş kalıyordu**. Kök neden: `scheduler.go` `run()` sonunda tek log
satırı `Info("schedule fired", status)` idi — yani başarısızlıkta bile **Info**
seviyesinde ve **hata metnini içermeden** yazılıyordu; `deliverPrompt`'taki
provider hatası ve `RunTask`'taki başarısızlık hiç loglanmıyordu.

**Düzeltme (davranış-korumalı, yalnız gözlemlenebilirlik):**
- `agent/scheduler.go` — `run()`: her tetiklemenin başında `Info("schedule fire: begin")`
  (schedule/trigger/kind/agent/task/cron); sonuçta seviye eşleşmeli loglama →
  başarısızlıkta `Error("schedule fire: failed")` **tam hata + agent/task/session**
  ile, başarıda `Info("schedule fire: ok")`. `SetScheduleDelivery` hatası da artık
  loglanıyor. `deliverPrompt`: agent lookup / session open hataları `Warn`'la,
  provider invoke hatası `Error("schedule deliver: agent invoke failed")`
  (provider/model dahil) ile loglanıyor.
- `agent/executor.go` — `RunTask`: sonuç logu seviye eşleşmeli; başarısızlıkta
  `Error("task run failed")` tam hata + provider/model ile.
- `agent/toolloop.go` — `recordedComplete` (tüm tool-loop yollarının tek geçtiği
  provider çağrı noktası): provider hatası `Warn("provider complete failed")` ile
  agent/provider/model/**callKind** etiketleriyle loglanıyor → chat/task/schedule/
  heartbeat tüm yollar bedavaya kapsanır.
- Test: `scheduler_test.go` `TestRun_LogsFailureAtErrorLevel` (capturingHandler ile
  Error-seviyeli kaydın varlığını doğrular) — regresyon koruması.
- **Tamamlayıcı (commit `799a6e9`):** `agent/budget.go` `ensureBudget` — bütçe
  kapısındaki `GetUsageToday` DB hatası sessizce yukarı yayılıp gerçek
  `ErrBudgetExceeded`'dan ayırt edilemiyordu. Artık `Warn("budget check failed",
  agent/callKind)` ile loglanıyor → bozuk bütçe kontrolü loglarda görünür.
- **Tamamlayıcı (commit `122ba2a`):** `agent/budget.go` `guardedComplete` — tool'suz
  otonom çağrıların (reflect/summary/title) huni noktası. Provider resolve / `Complete`
  hatası artık burada da `Warn` ile (agent/provider/model/callKind) loglanıyor;
  `recordedComplete` desenini aynalar, sessiz yukarı-yayılma kapandı. Test:
  `budget_test.go` `TestGuardedComplete_LogsProviderResolveFailure` (anahtarsız
  anthropic ajanı → resolve hatası → `Warn` kaydı doğrulanır).
- **Tamamlayıcı (commit `a04fc57`):** `api/summary.go` `handleSessionSummary` — talep-üzerine
  özet üretimi başarısız olunca 500 dönerken **loglama yoktu**; title endpoint'leri
  (`sessions.go`/`tasks.go`) "degraded" `Warn`'ı her zaman yazarken bu yol sessizdi.
  Artık 500 öncesi `Warn("summary generation failed", session/kind/error)`.

✅ `go build ./...` + `go test ./internal/agent/` + `go vet ./internal/api/` yeşil. Commit edildi (push edilmedi).

## Özel sağlayıcılar (data-instance) + `<think>` ayıklama ✅ (2026-06-17)

**(2) `<think>` ayıklama (commit `7273a62`):** MiniMax (ve bazı OpenAI-uyumlu
modeller) görünür metne `<think>…</think>` gömüyor. Yeni `providers/think.go`
`thinkFilter` (chunk-sınırı toleranslı durum makinesi) reasoning'i ayırır →
`Complete`'te thinking TraceStep'e, `Stream`'de `DeltaThinking`'e yönlendirir;
tag içermeyen modeller değişmeden geçer. `think_test.go` (chunk-bölünmüş tag dahil).

**(1) Özel sağlayıcılar — data-instance modeli (backend commit `35ec373`):**
Built-in'lere dokunmadan, migration'sız, kullanıcı **OpenAI- veya Anthropic-uyumlu
herhangi bir ucu** (OpenRouter/Gemini/Kimi/Ollama…) ekleyip ajan sağlayıcısı
olarak seçebilir. **settings:** `CustomProvider{id,label,kind,baseUrl,defaultModel,
models,keyEnc}` + maskeli DTO; store CRUD (`UpsertCustomProvider`/`DeleteCustomProvider`/
`CustomProviderKey`) — id doğrulama (`providerIDRe`), reserved-id guard, write-only
key. **registry:** `SetCustomProviders` + `Get`/`Available` custom dalı;
`buildCustom` transport'u kind'a göre seçer (openai→`OpenAICompat`, anthropic→
`Anthropic.WithEndpoint`); `CustomCatalog` katalogla birleşir. **api:**
`GET/PUT/DELETE /api/providers` + `customProviderSpecs`; catalog handler merge.
**Canlı doğrulandı** (registry→custom spec→client) iki kind için MiniMax `/v1` ve
`/anthropic/v1`'e karşı: tool_use round-trip. Store CRUD birim testi (doğrulama,
key koruma, reopen kalıcılığı). **server.go wiring (route + applySettings) COMMITSİZ**
— başka oturum WIP'iyle iç içe; reconcile'da gidecek.

**Frontend (COMMITSİZ — intertwined):** `api/providers.ts` (CRUD client) + barrel;
`ProvidersPanel`'e **Özel sağlayıcılar** bölümü (liste + ekle/düzenle/sil formu:
id/label/kind/baseURL/model/models/key) **ve** (önceki tur) API key alanlarında
Secrets içe-aktarma. `tsc` benim dosyalarımda temiz (`ArtifactsPanel` hatası başka
oturuma ait). `App.tsx`/`SettingsPanel.tsx`/`ProvidersPanel.tsx` başka WIP ile iç içe
→ reconcile'a bırakıldı.

**Provider key'leri yalnız Sır kasasından (2026-06-17, frontend COMMITSİZ):** Kullanıcı
isteğiyle ayarlarda provider anahtarları **artık textfield ile girilemez** — yalnız
sır kasasından seçilir. Yeni `ProviderKeyField` (seçim-only dropdown + Sil + "Sırlar →"),
Anthropic/MiniMax için kullanılır; seçim `applyKey`(SettingsPanel, yeni) ile **anında**
`updateSettings`'e yazılır. Custom provider formunda key alanı da seçim-only (`SecretSource`).
`keyInput`/`minimaxKeyInput` textfield'ları kaldırıldı. **Canlı (8090, yeni binary):**
OpenRouter token'ı sır kasasına eklendi (`OPENROUTER_API_KEY`), oradan değer alınıp
`openrouter` custom provider'ı (`kind=openai`, `openrouter.ai/api/v1`, `gpt-4o-mini`)
PUT edildi; `test-provider`→`{ok:true, model:gpt-4o-mini, sample:OK}`. Frontend UI
binary'e gömülü bundle rebuild gerektirir (şu an `ArtifactsPanel` WIP hatası `vite build`'i tıkıyor).

## Faz B3 — Provider/Model Bazlı Kullanım + Ücretlendirme ✅ (2026-06-17)

**İstek:** Bütçe ekranına provider'a göre token kullanımı ve **maliyet hesabı**.

**Veri boşluğu:** Kullanım kayıtları model/provider tutmuyordu. Eklendi:
- `db.Usage.ByModel map[string]KindStat` — `"<provider>|<model>"` anahtarlı
  (`db.ModelKey`); `AddUsageKind` artık `provider, model` parametreleri alıyor ve
  ByKind'in yanında ByModel'i de günceller. `RecordUsage(ctx, agent, model, usage)`
  (model boşsa `agent.Model`), compaction `recordCompaction` provider+model damgalı.
- **Fiyat tablosu** `providers/pricing.go`: `Price{InputPerMTok, OutputPerMTok}` (USD),
  `PriceFor(provider, model)`. anthropic (opus 5/25, sonnet 3/15, haiku 1/5, fable
  3/15) + minimax (M2.1 0.30/1.20, lightning 0.20/0.80, M2 0.30/1.20); `minimax-anthropic`
  minimax tablosunu paylaşır. **claude-cli kasıtlı yok** → abonelik (OAuth), token
  başına ücret yok; `PriceFor` `ok=false` döner → UI "abonelik / fiyatsız" gösterir.
  Fiyatlar **liste-fiyatı tahmini** (prompt-cache/batch indirimi modellenmez).

**API** (`/api/usage`): yanıt artık `byProvider[]` (provider başına çağrı/token/
`costUSD`/`priced`), `totals.costUSD`+`totals.priced`, ajan satırında `provider`+
`costUSD`+`priced`. `priced=false` = harcamanın bir kısmı fiyatsız (abonelik/özel).

**Frontend:** `types/usage.ts` (`ProviderStat`, totals/agent cost alanları);
`BudgetPanel`: "Tahmini maliyet (bugün)" özet kartı, **"Provider'a göre" tablosu**
(maliyet veya "abonelik / fiyatsız"), ajan tablosuna **Maliyet** sütunu.

**Doğrulama:** `go build`/`vet`/`test` + `tsc`/`vite` yeşil; `store_usage_test`'e
ByModel iddiası eklendi. **Canlı API smoke:** anthropic opus 100k/20k → `$3.00`
(0.1×15 + 0.02×75) doğru hesaplandı; claude-cli → `priced=false` abonelik; toplam +
byProvider + ajan maliyeti tutarlı.

**Sıradaki:** fiyatların Ayarlar'dan düzenlenebilmesi; TL kuru; model-bazlı detay
satırı; prompt-cache indirimini modelleme.

## external-agent incelemesi P0+P1 fix'leri (CG-1…CG-5) ✅ (2026-06-17)

`_Docs/arsiv/13-CRAFT-AGENTS-INCELEME.md`'deki 4 P0 + 1 P1 boşluğu kapatıldı
(davranış-koruyucu, geriye-uyumlu):

- **CG-1 — Tool çıktısı boyut sınırı:** `tools/registry.go` `capToolOutput()`
  (100K bayt, UTF-8 sınırında trunc + `…[truncated N bytes]`); `Registry.Call`
  ve `CallStream`'de built-in **ve** MCP tüm başarılı çıktılarına uygulanır →
  kontrolsüz tool çıktısının bağlam/JSONL şişmesi/OOM riski kapandı.
- **CG-2 — http_get SSRF:** `builtin_http.go` artık özel `net.Dialer.Control`
  guard'lı transport kullanır — çözülen IP **her dial'da** denetlenir (loopback,
  unspecified, link-local, RFC1918 private, ULA `fc00::/7`, CGNAT `100.64/10`,
  multicast; `169.254.169.254` cloud-metadata link-local'e dahil). Redirect ve
  DNS-rebind hop'ları da kapsanır; ayrıca http/https dışı şema reddedilir.
- **CG-3 — Thinking resolver model-sınıf:** `providers.RequiresAdaptiveThinking
  (model)` (Fable/Mythos 5 sınıfı) + `agent.resolveThinkingBudget(model,level)`;
  bu modellerde "off"/"low" istek `MinAdaptiveThinkingBudget=1024`'e taban'lanır
  (aksi halde `thinking:disabled` → API 400). Opus/Sonnet/Haiku değişmez.
  Native tool-path thinking'i hâlâ kapalı (imzalı blokları echo edemiyor) —
  resolver yalnız plain path'te uygulanır; Fable 5 eklenince genişletilecek.
- **CG-4 — MCP şema normalizasyonu:** `mcp.NormalizeSchema()` `Registry.Defs`'te
  her dış MCP şemasına uygulanır — `$schema`/`$id`/`$ref`/`$defs`/`definitions`
  recursive strip (Anthropic 400 sebebi), kök object garanti; `additional
  Properties`/`required`/`oneOf`/`anyOf`/`allOf` korunur.
- **CG-5 — `send_agent_message` (P1):** `tools/builtin_agentmsg.go` yeni built-in
  (self-manage gate'li) — hedef ajanın `agent-inbox` oturumuna **user-mesaj**
  append + `Runtime.Wake` ile uyandırır; **fire-and-forget** (senkron `call_agent`
  delegasyonunun async tamamlayıcısı). `agent.SendAgentMessage` ismi/id'yi çözer,
  inbox thread'ini `GetOrCreateSourceSession("agent-inbox","inbox:<id>",…)` ile
  tek tutar. Native (anthropic/minimax) yolunda; claude-cli kendi döngüsünü sürer.

**Testler:** `registry_cap_test.go`, `builtin_http_test.go`, `normalize_test.go`,
`thinking_test.go`, `agent/agentmsg_test.go` (resolveThinkingBudget + delivery +
inbox reuse + unknown-recipient). ✅ `go build`/`vet`/`test ./internal/...` yeşil.
**Commit edildi** (push yok, kullanıcı tercihi).

## Generic OpenAI-compat tool-use + Ayarlarda Secrets'tan key ✅ (2026-06-17)

**(1) Generic `OpenAICompat` + tool-use (commit `cf7d718`):** `minimax.go`'daki
`Minimax` tipi generic **`OpenAICompat`**'e dönüştürüldü (name/baseURL/
defaultModel-parametrik; `NewMinimax` ince alias korundu → testler ve
`kind_minimax` değişmeden çalışır). **OpenAI-tarzı tool-use** eklendi:
`Complete` artık `req.Tools`'u `tools` formatında gönderir, asistan `tool_calls`
ve `tool`-rol sonuçları round-trip eder, `finish_reason:tool_calls`→`StopToolUse`.
Agent native loop tool turlarını `Complete`'e yönlendirdiğinden MiniMax (ve
ileride OpenRouter/Gemini/Kimi gibi OpenAI-uyumlu kind'lar) artık **ajan** olur.
`Stream` metin-only (tool turları akmaz). **Canlı doğrulandı** (MiniMax `/v1`,
registry→minimax kind→client): r1 `get_weather` tool_use, r2 tool sonucu
beslenince `end_turn` metin cevap. *Nüans:* MiniMax görünür metne `<think>…</think>`
gömüyor (mevcut davranış; ayıklama kapsam dışı). *Sıradaki:* OpenRouter/Gemini/Kimi'yi
**seçilebilir** kılmak için data-instance modeli (per-id credential store) gerekir.

**(2) Ayarlarda API key alanlarında Secrets kaynağı (frontend, COMMITSİZ):**
Sağlayıcı key alanlarının (Anthropic + MiniMax) altına **`SecretSource`** kontrolü
eklendi: (a) workspace **sır kasasından seç** dropdown'ı → `revealSecret` ile değeri
alana içe aktarır (kaydedince settings'e şifreli yazılır), (b) **"Sırları yönet →"**
butonu `onNavigate('secrets')` ile Sırlar ekranına atlar, (c) mevcut **Sil** korundu.
`ProvidersPanel` yeni prop'lar (`secrets`/`onImportSecret`/`onManageSecrets`),
`SettingsPanel` `api.listSecrets()` yükler + `onNavigate` (App `setView`) thread'ler.
**Mimari not:** settings app-global, secret kasası per-workspace, registry tek paylaşımlı
→ referans-bazlı çözüm uygun değil; bu yüzden **değeri içe aktarma** (kopya) yaklaşımı
seçildi. `tsc -b` temiz. **Commit edilmedi:** `App.tsx`+`SettingsPanel.tsx` zaten başka
oturum WIP'i içeriyor (`ProvidersPanel.tsx` tamamen yeni); reconcile'da gidecek.

## Provider kind/manifest/factory temeli — plugin'e çevrilebilir ✅ (2026-06-17)

**İstek:** ClaudeCli/AnthropicAPI/MinimaxAPI sağlayıcılarını ve Provider/Model
seçimini daha **generic**, ileride **plugin gibi eklenip çıkarılabilir** yapmak.
Bu tur kapsamı: mevcut 3 sağlayıcıyı koruyup (davranış/ayar/ID aynı) registry
switch'ini **self-registering kind katmanı**na taşımak (Katman 2 temeli). Data
instance UI + migration (Katman 1) ve subprocess plugin (Katman 3) sonraya.

**Değişiklik (`internal/providers`):**
- `kind.go`: `Manifest` (Kind/Label/NeedsKey/NeedsBaseURL/AllowCustomModel/Order/
  Models), `ResolvedConfig` (Key/BaseURL/Model/CLIPath/beta bayrakları),
  `ProviderKind` arayüzü (`Manifest`/`Available`/`Build`), package-level
  `RegisterKind`/`lookupKind`/`Kinds()` (Order'a göre sıralı).
- `kind_anthropic.go` / `kind_claudecli.go` / `kind_minimax.go`: 3 built-in,
  model listeleri buraya taşındı, her biri `init()` ile self-register.
- `catalog.go`: `Catalog()` artık manifest'lerden türetilir (elle liste gitti).
- `registry.go`: `Get` switch → `lookupKind`+`resolve(id)`→`kind.Build`; yeni
  `resolve()` (kimlik→creds eşlemesi, Faz 2'nin tek kalan id-aware dikişi) +
  generic `Available(id)`. Eski `*Configured`/`ClaudeCLIAvailable` korundu (main.go).
- `api/catalog.go`: per-id switch → `s.providers.Available(e.ID)`.

**Plugin'e çevrilebilirlik:** 4. transport eklemek = yeni `kind_*.go` + `RegisterKind`;
registry/catalog/api'ye **dokunmadan**. Manifest, UI'ın da generic render
edebileceği yetenek tanımı (NeedsKey/NeedsBaseURL şimdiden var, frontend Faz 2).

**Doğrulama:** `go build ./...` + `go vet` + `go test ./internal/...` **yeşil**;
yeni `kind_test.go` (katalog 3 giriş+sıra, Get kind-dağıtımı + "" → claude-cli,
bilinmeyen hata, Available gating, unconfigured Build hataları). Commit `7ce50f8`
(yalnız provider dosyaları; ağaçtaki diğer WIP'e dokunulmadı).

**Faz 0 + MiniMax-Anthropic kind ✅ (aynı gün, commit `5ee514c`):** Gerçek MiniMax
key ile `/anthropic/v1/messages` canlı smoke yapıldı → uç **yüksek uyumlu**:
native Messages formatı (`thinking`+signature, `text`, `tool_use` blokları),
`usage.cache_creation/read_input_tokens` (caching var), `x-api-key` **ve** Bearer
auth çalışıyor, `anthropic-beta` header + `cache_control` breakpoint **kabul
ediliyor** (HTTP 200), tool-use **tam çalışıyor** (`stop_reason:tool_use`).
Bunun üzerine: `anthropic.go` baseURL/defaultModel/name-parametrik yapıldı
(`WithEndpoint`; varsayılanlar değişmedi) + yeni **`kind_minimax_anthropic.go`**
(4. self-registering kind, MiniMax key'ini Anthropic transport'undan geçirir →
OpenAI yolunun veremediği **tool-use + thinking**). `registry.resolve` "minimax-
anthropic"→MiniMax key. **Uçtan uca canlı doğrulandı**: registry→kind→client→
`/anthropic` ile `get_weather` tool çağrısı (`stop_reason:tool_use`). Plugin
temelinin ilk faydası: 4. transport = tek dosya + `RegisterKind`, başka yere
dokunmadan.

**Sıradaki:** generic `openaicompat` (OpenAI-tarzı tool-use ekleyerek OpenRouter/
Gemini/Kimi'yi ajan yapmak); ardından data-instance modeli (ProviderConfig
listesi + migration + frontend instance UI) ve `ThinkingCapable` arayüzü.
Codex/Gemini **CLI** kapsam dışı (bespoke protokol). **Frontend notu:** yeni kind
katalogda otomatik görünür (generic render); özel UI gerekmedi.

## Faz B2 — Bütçe Ekranı (Budget Screen) ✅ (2026-06-17)

**İstek:** Faz B'nin ürettiği köken-etiketli kullanım verisini görebileceğimiz bir
ekran. Önceden bütçe yalnızca sohbet başlığındaki minik `ChatMeters` rozetiydi
(tek ajan, tek gün, yalnız çağrı sayısı); `byKind` hiçbir yerde görünmüyordu.

**Backend:**
- `db/store_usage.go`: `Today()` (exported gün), `UsageForDay(day)`,
  `UsageHistory(sinceDay)` — bellek-içi usage map üzerinden, ek disk okuması yok.
- `api/budget.go::handleWorkspaceUsage` → `GET /api/usage?days=` (1–90, vars. 7):
  bugünkü **workspace toplamı + köken kırılımı**, **ajan başına satır** (ajan
  kimliğiyle join: ad/avatar/renk + limitler), ve **son N gün trendi** döner.
  Ajanlar token harcamasına göre azalan sıralı. Rota `registerUsageRoutes`'a eklendi.

**Frontend:**
- `types/usage.ts` (`WorkspaceUsage`/`BudgetAgentRow`/`KindStat`/`BudgetTrendPoint`),
  `api/system.ts::workspaceUsage(days)`.
- NavRail yeni **"Bütçe"** görünümü (Wallet ikonu) + `App.tsx` render + `VIEW_TITLE`.
- `components/panels/BudgetPanel.tsx`: (1) gün seçici (7/30/90) + yenile, (2) 4
  özet kartı (toplam token, çağrı, girdi, çıktı), (3) **köken kırılımı** yüzde
  çubukları (her köken sabit renk + TR etiket: Sohbet/Görev/Zamanlama/Akış/Nabız/
  Delegasyon/Başlık/Özet/Yansıma/**Sıkıştırma**/Diğer), (4) **trend** bar grafiği,
  (5) **ajan tablosu** — avatar + çağrı/token + token limiti doluluk çubuğu
  (accent→warning≥%80→danger aşımda) + durum rozeti (Normal/Limit yakın/Aşıldı).

**Doğrulama:** `go build`/`vet`/`test` + `tsc -b`/`vite build` yeşil. **Canlı API
smoke** (scratch instance): `/api/usage` doğru şekil; ajan-join (ad/avatar/renk/
limit) doğrulandı; store'a enjekte edilen örnek kullanımla `byKind` (chat/compact/
task/title) hem ajan satırında hem toplamda hem trendde uçtan uca doğru toplandı.
(Not: PS 5.1 `Out-File -Encoding utf8` BOM'u JSON yüklemesini bozuyor → testte .NET
`WriteAllText(UTF8 no-BOM)` kullanıldı; CLAUDE.md uyarısıyla aynı tuzak.) UI'ın
görsel Playwright testi gateway/Chrome dalgalı olduğundan yapılmadı; derleme yeşil.

**Sıradaki:** maliyet sütunu (model→fiyat tablosu), ajan satırından inline limit
düzenleme, kind kırılımının `ChatMeters`'a da düşmesi.

## Faz B — Köken-Etiketli Kullanım Muhasebesi (Usage Attribution) ✅ (2026-06-17)

**İstek:** Tüm agent sorgularını tek bir noktadan geçirip daha doğru bağlam
harcaması ve bütçe planlaması yapmak — schedules/flows/kanban dahil **her kökeni**
ve **sayılmayan yardımcı çağrıları** (özet, başlık, yansıma, **compaction**)
kapsayacak şekilde.

**Tespit:** LLM çağrıları aslında üç funnel + iki bypass'tan akıyordu:
- `agent/budget.go::guardedComplete` (title/summary/reflect) — sayıyor.
- `agent/toolloop.go::recordedComplete/recordedStream` — chat/task/schedule/flow/
  heartbeat/delegate **hepsi buraya iner** (fiili tek boğaz) — sayıyor.
- **Bypass:** `conversation/manager.go::summarize` (rolling summary) +
  `conversation/reactive.go` (in-flight compaction) doğrudan `provider.Complete`
  çağırıyordu → **token yakıyor ama hiç sayılmıyordu** (bütçeyi en çok bozan nokta).

**Çözüm — iki katman:**
1. **Köken etiketi (CallKind, ctx üzerinden):** `agent/callkind.go` — `WithCallKind`/
   `callKindFrom`; taksonomi tek kaynağı `db.UsageKind*` (chat/task/schedule/flow/
   heartbeat/delegate/title/summary/reflect/compact/other). `RecordUsage` artık
   ctx'ten kind okuyup `db.AddUsageKind` ile yazıyor; üç funnel de buradan geçtiği
   için **damgayı her köken giriş noktasında bir kez basmak yeterli**: `RunTaskStream`
   (task), `deliverPrompt` (schedule), `RunFlow` (flow), `runHeartbeat` (heartbeat),
   `delegate` (call_agent), `GenerateTitle`/`Summarize`/`reflect` (meta).
2. **Bypass'ları sayma:** `db.AddUsageKind` + `db.Usage.ByKind map[string]KindStat`
   (toplam + kırılım senkron). `conversation` paketi `agent`'ı import edemediğinden
   compaction çağrıları zaten ellerindeki `*db.DB` ile `recordCompaction(...,
   UsageKindCompact)` çağırır — `Prepare`/`ForceCompact` imzaları değişmedi (api
   katmanı dokunulmadı); `CompactInFlightMessages` yalnız `*db.DB` parametresi kazandı
   (çağıran: `toolloop.go` → `r.db`).

**Yüzey:** `GET /api/agents/{id}/usage` artık `byKind` döner → metre harcamanın
nereye gittiğini gösterebilir.

**Dosyalar:** `db/store_usage.go` (kind consts + ByKind + AddUsageKind),
`agent/callkind.go` (yeni), `agent/budget.go`, `titler.go`, `summarizer.go`,
`reflector.go`, `runtime.go`, `executor.go`, `scheduler.go`, `flow.go`,
`delegate.go`, `conversation/manager.go`+`reactive.go`, `api/usage.go`.
Test: `db/store_usage_test.go` (kırılım↔toplam senkron + "other" fallback).
✅ `go build`/`vet`/`test ./internal/...` yeşil.

**Sıradaki (opsiyonel):** (a) her çağrıda `events.Bus`'a `usage` olayı → canlı
çapraz-workspace metre; (b) **ön-uçuş token tahmini** (`conversation.EstimateTokens`)
ile bütçeyi *çağrıdan önce* engelleyen rezervasyon; (c) per-kind bütçe limiti;
(d) frontend `ChatMeters`'a kind kırılımı (compaction payı uyarısı).

## Faz U — Birleşik Yürütme/Çıktı Katmanı (Unified Executions) ✅ (2026-06-17)

**İstek:** Uygulamadaki farklı çıktı üreten yolları (Flows, Schedules, Kanban
görevleri, manuel sessionlar) daha generic hale getirmek — hepsinin çıktısını
streaming şekilde okuyup hepsini **Session gibi** görebilmek.

**Tasarım kararı:** Tetikleyici yüzeyleri (Board, cron, flow builder, sohbet)
ayrı kalır; **altlarındaki transkript + akış katmanı birleşir.** Birleştirme
primitifi: her yürütme çıktısını bir **Session**'a (Message + `TurnStep` izi)
döker. `TurnStep` zaten hem native hem claude-cli yolunda üretilen evrensel birim
olduğundan iş çoğunlukla *bağlama*, yeniden yazım değil.

**Faz U1 — Görev koşuları → transkript session'ı:**
- `db.Session`'a `SourceID` (kaynağa bağlama), `db.Run`'a `SessionID`+`MessageID`.
- `db.GetOrCreateSourceSession(kind, sourceID, …)` — (kind, sourceID) başına tek
  session (görev başına bir "task" thread'i, akış başına bir "flow" thread'i).
- `agent/executor.go`: `RunTask`/`runTaskFlow` artık `invokeWithMemoryTraced` ile
  iz üreterek her koşuyu task session'ına **user turn (prompt) + assistant turn
  (aktivite izi)** olarak yazar; `SetRunSession` ile run↔session bağlanır. Tek
  çalıştırma noktası `RunTask` olduğundan manuel ▶ / cron / dispatcher hepsi
  bedavaya transkript üretir.

**Faz U2 — Akış koşuları → transkript session'ı + canlı node akışı:**
- `agent/flow.go`: `flowStateToSteps` (node izi → `TurnStep`), `RunFlowRecorded`
  (akışı koşup flow session'ına turu yazar), `finalFlowAgentID`/`firstFlowAgentID`.
- `orchestration.Graph.NodeByID` (exported lookup).
- API: `handleRunFlow` artık `{run, sessionId}` döner ve flow session'ına kaydeder;
  yeni `POST /api/flows/{id}/run-stream` (SSE node akışı + transkript kaydı).

**Faz U3 — Birleşik "Aktivite" feed'i:**
- `GET /api/executions` (`api/executions.go`): tüm ajanların session'larını
  kind + canlı `running` (akıştaki turlar) + `lastStatus` (task/flow son koşu) ile
  zenginleştirip döner; `?kind=` filtresi.
- Frontend: NavRail **"Aktivite"** görünümü (`ExecutionsPanel.tsx`) — master-detail:
  kind filtre sekmeleri (Tümü/Sohbet/Görev/Akış/Zamanlama/Nabız), canlı durum/okunmadı
  rozetleri, seçilen yürütmenin **salt-okunur transkripti** (`MessageList` yeniden
  kullanılır), 5sn poll ile canlı tazeleme. Chat sidebar artık yalnız `kind==='chat'`
  gösterir (task/flow/schedule transkriptleri Aktivite'de) → sohbet listesi kirlenmez.

**Durum:** `go build`/`vet`/`test` + `tsc -b`/`vite build` yeşil. Yeni testler:
`db/source_session_test.go` (kaynak-session idempotent + reload), `agent/flow_session_test.go`
(flowStateToSteps). **Canlı API E2E** (izole instance, gerçek claude-cli):
görev koşusu → "task" execution + 2-turlu transkript (prompt→PONG); standalone akış
koşusu → "flow" execution + transkript (go→FLOWPONG); kind filtreleri doğrulandı.
Not: schedule/heartbeat session'ları zaten Session kullandığından bedavaya feed'de görünür.

> **Güncelleme — Canlı streaming (2026-06-17):** İki yürütme yolu da artık adım-adım
> canlı akıyor. **Görev koşusu:** `RunTask` → `RunTaskStream(…, onStep)` (eski imza
> `nil` ile delege eder); plain görev `invokeWithMemoryStream` (= `CompleteWithToolsStream`)
> ile token/araç adımlarını canlı yayar, flow-backed görev her node'u observer'dan
> `StepText` olarak yayar. Yeni `POST /api/tasks/{id}/run-stream` (`api/tasks_stream.go`):
> SSE `meta`/`step`/`reply`/`error`; run, task session id'sine **kaydedilerek `s.runs`'a
> register edilir** → executions feed'inde canlı `running=true` + iptal edilebilir.
> `TaskDetailPanel`'in ▶ butonu artık `api.runTaskStream` ile koşar ve canlı adımları
> `TurnSteps` ile gösterir (geçici delta/tool_delta/tombstone/ask/steer filtrelenir).
> **Standalone akış:** `RunFlowRecorded` zaten observer alıyordu; `handleRunFlowStream`
> bağlandı, frontend `api.runFlowStreamStandalone` (`POST /api/flows/{id}/run-stream`,
> `node`/`reply`/`error`) + `FlowsPanel` `doRun` artık node-node canlı ilerleme gösterir
> (her node start'ta spinner'lı pending, done'da çıktı dolar; final run gelince kalıcı
> trace'e geçer). `api/flows.ts`'te ortak `pumpSSE`/`dispatchSSE` yardımcıları çıkarıldı.
> ✅ build/test + tsc/vite yeşil; **canlı E2E** (izole, claude-cli): task run-stream
> `meta→reply`, flow run-stream `node×4 (start/done×2)→reply` doğrulandı. (Not: eşzamanlı
> başka oturum aynı ağaca call-kind attribution + agent Skills WIP'i eklemiş — çakışma yok.)


## Ara özellik — Çapraz-Session Farkındalığı (push block + `list_sessions` tool) ✅ (2026-06-17)

> **Güncelleme (2026-06-17):** Ayar kapsamı **app-global'dan workspace'e özele** taşındı.
> Artık her workspace kendi `sessionContextEnabled`/`sessionContextEveryTurn`/
> `sessionContextRecentCount` değerine sahip (`WSSettings` + Ayarlar → **Bu Workspace → Genel**).
> Backend: `workspace/settings.go` (alanlar + `defaultWSSettings` seed: açık/first-turn/5 +
> `clampRecent` 1–20) → `loadSettings`/`UpdateSettings` `Runtime.SetSessionContext` push;
> `Runtime`'da per-workspace atomic state (`sessionCtx*` + getter'lar); `composeTurnRequest`
> artık `wsp.Settings()` okur; `buildRegistry` tool gating `r.SessionContextEnabled()`.
> App-global `settings`/`Tunables`/applySettings + frontend app-paneli **kaldırıldı**;
> `api/workspace_settings.go` DTO + frontend `WorkspacePanel`/`types/workspace.ts`'e eklendi.
> Testler: `workspace/settings_test.go` (default+clamp), `agent/sessionctx_test.go` (Runtime
> getter + tool gating). Canlı doğrulandı (izole instance): workspace-settings varsayılan
> açık/first-turn/5, kapatınca `list_sessions` katalogdan düşer, recent 999→20 clamp, app
> settings'te artık alan yok.

**İstek:** Ajanlara her oturum başında workspace'in **aktif sessionlarını** ve **geçmiş 5
sessionunu** kısa bir özet olarak vermek. Kararlar: aktif = `State=="active"`, filtre
`Kind=="chat"`, mevcut session hariç; geçmiş = aktif-olmayan, `UpdatedAt` desc ilk N; **yeni
LLM çağrısı yok** (mevcut `Title`+`Summary` kullanılır); ilk-tur/her-tur Ayarlar'dan seçilir;
**`list_sessions` pull tool**'u da eklendi; tümü **Ayarlar toggle**'lı.

**İki kanal — push (enjeksiyon) + pull (tool):**
- **Push** (`api/sessions_context.go`, yeni): `sessionsContextBlock(ctx, db, currentID, recentCount)`
  → `ListSessions("")` (ws-geneli, UpdatedAt desc) → `Kind=="chat"` + mevcut hariç → aktif
  (cap 12) / geçmiş (recentCount) ayrımı → kompakt satır (başlık·mesaj·göreli yaş·özet snippet,
  kırpmalı). `composeTurnRequest`'in **dinamik** (cache-dışı) suffix'ine eklenir; `freshSession`
  (ilk tur) bayrağı `chat.go`/`chat_stream.go`'da user-mesajı eklenmeden **önce** yakalanır.
- **Pull** (`tools/builtin_sessions.go`, yeni): `list_sessions` built-in tool (`{state?:active|all,
  limit?}`) — ajan ihtiyaç duyunca çeker. `buildRegistry`'de aynı master toggle ile gated.

**Ayarlar (Bağlam & Bellek paneli):** `SessionContextEnabled` (vars. **açık**),
`SessionContextEveryTurn` (vars. kapalı = yalnız ilk tur), `SessionContextRecentCount`
(vars. 5, clamp 1–20). `settings`→`applySettings`→`tun.SetSessionContext` canlı push;
`Tunables.SessionContext*` getter'ları. Frontend `ContextPanel`'e "Çapraz-session farkındalığı"
bölümü (toggle + everyTurn + sayı, koşullu).

**Test:** `settings/store_test.go` (round-trip+clamp), `api/sessions_context_test.go` (aktif/geçmiş
ayrımı, mevcut-hariç, chat-only filtre, boş), `tools/builtin_sessions_test.go` (active-only/all/
chat-only). **Canlı doğrulama** (izole instance, port 8099): varsayılan açık/first-turn/5 →
`list_sessions` katalogda → disable edince kaybolur → recentCount 999→20 clamp. `go test ./...` +
`tsc` yeşil.

## Faz S1 — Skill sistemi (dosya-tabanlı, 2 katman, lazy, subskills, varsayılan seeding) ✅ (2026-06-17 → 2026-06-18)

Ajanlara **yeniden kullanılabilir talimat setleri** (skill) eklendi — Claude Code /
external-agent-oss desenleri incelenip TionSwarm'ya uyarlandı. **flat skill + 2 katman +
lazy gövde + subskills (aşamalı yükleme) + varsayılan skill seeding** ile tamamlandı.

**Format:** klasör-başına `<slug>/SKILL.md` (YAML-ish frontmatter + markdown gövde).
Frontmatter alanları: `name`, `description`, `when_to_use`, `icon`, `color`,
`alwaysAllow[]`, `requiredSources[]`, `subskills[]`. Bağımlılıksız küçük frontmatter
parser (`internal/skills/frontmatter.go`) — go.mod minimal kalsın diye yaml lib yok.

**2 katman (öncelik: workspace > global):**
- global: `~/.tionswarm/skills/` (TionSwarm'nun kendi data dizini — `TIONSWARM_DATA_DIR` onurlandırır)
- workspace: `<workspace>/skills/` (store/·config/·workspace/ kardeşi)
Aynı slug workspace'te override eder (`internal/skills/store.go`, `New`+`Reload`+`List`).

**Lazy:** katalog taranırken **sadece frontmatter** okunur (token-dostu). Tam gövde
diskte kalır; ajan `use_skill(slug)` çağırınca `Store.Body` ile okunur. Katalog
(slug+özet+ne-zaman) **statik sistem promptuna** "# Available Skills" bloğu olarak
girer (`SkillsCatalogBlock` → `composeTurnRequest`). `use_skill` tool'u sadece
workspace'te ≥1 skill varken kaydedilir (`builtin_skill.go`, `toolsetup.go`).

**API:** `GET /api/skills` (katalog), `GET /api/skills/{slug}` (+gövde, lazy),
`POST /api/skills/reload` (diskten yeniden tara) — `internal/api/skills.go`,
`registerSkillRoutes`. **Frontend:** NavRail "✨ Beceriler" → `SkillsPanel.tsx`
(iki panel: solda liste + tier rozeti, sağda gövde markdown render + meta);
`types/skill.ts`, `api/skills.ts`.

✅ `go build`/`vet`/`test` + `tsc -b`/`vite build` yeşil. Yeni testler:
`skills/store_test.go` (frontmatter parse, tier override, catalog block).
**Canlı API testi** (scratch instance :8099, temp data dir): global `code-review`
skill'i `/api/skills` katalogda frontmatter-only, `/api/skills/{slug}` gövdeyi lazy
döndü, `/api/skills/reload` ok, Türkçe karakterler round-trip. Demo skill
`~/.agents/skills/code-review/` bırakıldı (özellik anında görünür). **Not:**
mcp-chrome bu oturumda bağlı olmadığından Playwright görsel testi yapılamadı.
**Kalan (v2):** namespacing (`parent:child`), `paths:` koşullu otomatik aktivasyon,
skill'in `alwaysAllow`/`requiredSources` alanlarının runtime'da enforce edilmesi.

### Faz S1 — Güncellemeler (2026-06-18)

Yukarıdaki ilk sürüm üzerine yapılan değişiklikler (skill sistemi son hali):

- **Global tier izole edildi:** cross-tool `~/.agents/skills` yerine **`<DataDir>/skills`**
  (varsayılan `~/.tionswarm/skills`, `TIONSWARM_DATA_DIR`'a saygılı). Sebep: ilk sürümde global
  skill'ler External Agent gibi `~/.agents/skills` okuyan diğer araçlara **sızıyordu**. `globalSkillsDir`
  artık TionSwarm'nun kendi data dizinine bakar.
- **Proje katmanı KALDIRILDI → 2 katman (workspace > global):** TionSwarm'da `workDir` zaten
  workspace'e ait tek sandbox olduğundan `<workDir>/.agents/skills` "proje" tier'ı workspace
  tier'la örtüşüyordu. `SourceProject` + `projectSkillsDir` silindi; `skills.New(global, workspace)`.
  Frontend `SkillSource` → `'global' | 'workspace'`; rozetler/label/boş-durum güncellendi.
- **Per-agent seçim + sıralama:** Skill'ler **paylaşımlı havuz**; ajan **yazmaz, seçer**.
  `db.Agent.Skills []string` (sıralı slug listesi), ajan ayarlarından çoklu seçim + ↑/↓
  (`AgentSkillsSection.tsx`, `PUT /api/agents/{id}` `skills`). Ajanın gördüğü katalog = atanmışlar
  (sırasıyla) + tüm shared'ler. `use_skill` `agentSkillLib` ile bu kümeye kısıtlı; `composeTurnRequest`
  artık `SkillsCatalogBlockForAgent(agent)` çağırır.
- **Erişim modu `access: shared` (gerektiğinde) vs restricted (atanınca):** shared skill'ler atama
  gerekmeden **tüm** ajanların promptunda görünür/kullanılır (`Shared` alanı, `SharedList`/`AllowedFor`/
  `CatalogBlockForAgent`). SkillsPanel'de **Gerektiğinde/Atanınca** rozeti + **Paylaş/Kısıtla** toggle
  (`PUT /api/skills/{slug}/access` → `setFrontmatterAccess` frontmatter'ı yeniden yazar).
- **UI iyileştirmeleri:** `POST /api/skills/{slug}/reveal` (Klasörü aç), SkillsPanel sol liste
  **sürükle-genişlet** (localStorage'da kalıcı genişlik).
- **Sağlamlaştırma:** dosya dışarıdan silinince `Body`/`SetAccess` katalogu **self-heal** reload edip
  net hata döner (kriptik OS hatası yerine).
- **Örnek skill'ler (son):** `commit` (global), `tionswarm-project` + `web-research` (workspace;
  `web-research` `access: shared`). Eski `~/.agents/skills` ve proje-tier demo'ları temizlendi.
- **`subskills:` frontmatter alanı (aşamalı yükleme):** Skill `subskills: [slug1, slug2]` ile daha
  ayrıntılı alt-skill'lere işaret edebilir. `use_skill` çağrıldığında gövde sonuna "## Related skills"
  footer'ı eklenir — ajan bilgi derinleştirme yolu olarak `use_skill` ile alt-skill'i yükleyebilir.
  `internal/skills/store.go` → `UseSkillBody` + `subskillFooter`. Frontend `SkillsPanel.tsx` detail
  header'ında alt-skill chip'leri (bilinene tıklanabilir, bilinmeyene strikethrough).
- **Varsayılan skill seeding (`EnsureDefaults`):** Her `NewRuntime` başlangıcında TionSwarm'nun kendi
  kılavuz skill'leri `~/.tionswarm/skills`'e **idempotent** olarak kopyalanır (mevcut olanı asla ezmez).
  `internal/skills/defaults.go` (`//go:embed defaults`); seeded skill'ler: `tionswarm-guide` (genel
  bakış, `access: shared`) + `tionswarm-flows` (orkestrasyon detayı, `access: shared`, `tionswarm-guide`
  subskill'i olarak işaret eder).
- ✅ build/vet/test + tsc/vite yeşil; **canlı :8090 restart** ile uçtan uca doğrulandı.

## Ara özellik — Ajan bağlam önizleme (fresh-start context) ✅ (2026-06-17/18)

Ajan ayarlarında (sağ üst, **👁 Bağlam**) bir pencere açıp ajanın **bir tura sıfırdan
başlarken aldığı bağlamı** gösterir. `GET /api/agents/{id}/context[?message=...]`
(`internal/api/agent_context.go`):
- **Statik sistem promptu** (`buildAgentStaticPrompt`, `composeTurnRequest` statik yarısının
  aynası): ajan-adı notu + kullanıcı profili + persona/soul/identity + workspace yönergeleri +
  deliverable rehberi + ajanın gördüğü beceri kataloğu.
- **Araç kataloğu** (`ToolCatalog`) ad+açıklama + token tahmini.
- **Dinamik kısım (opsiyonel `?message=`)**: o mesaj için **hafıza recall** + **çapraz-oturum
  bloğu** (`buildAgentDynamicPrompt`). Oturuma özel parçalar (özet/artifact/todo) canlı oturum
  gerektirdiğinden hariç (notla belirtilir). Token özeti: Toplam = Sistem + Araçlar + Dinamik.
- **Frontend:** `AgentContextModal.tsx` — sistem promptu **markdown render** (Markdown/Ham toggle),
  örnek-mesaj input'u, dinamik bölüm, kopyala. `api.agentContext(id, message?)`, `AgentContextPreview`.
  Ayrıca ajan formundaki **Sil/Kaydet** aksiyonları panel **footer'ından sağ-üst başlığa** taşındı.
- ✅ build/tsc/vite yeşil; canlı :8090 (68 araç, çapraz-oturum bloğu dinamikte göründü).

## Ara özellik — Akış node-node SSE streaming + multimodal flow input ✅ (2026-06-17)

Sohbetten tetiklenen akışlar artık node-node canlı akıyor ve dosya/görsel eki kabul ediyor.

- **SSE streaming:** `orchestration.Engine`'e opsiyonel `Observer` eklendi — her node'un `start`/`done` anında `NodeEvent` yayımlar (paralel çocuklar eşzamanlı; observer concurrency-safe olmalı). `RunFlow(..., obs)` observer'ı geçirir; `driveFlow`/`ResumeRunningFlows` nil ile çağırır. Yeni `POST /api/sessions/{id}/run-flow-stream` (`api/flows.go` `handleSessionRunFlowStream`) akışı SSE üzerinde çalıştırır: `meta` → `node`* → `reply`/`error`. Yazıcı `sync.Mutex` ile korunur (paralel emisyon). Frontend `api.runFlowStream` (chat SSE parser ikizi) + `useChatStream.runFlow` canlı transcript'i node geldikçe kurar — **`nodeId` ile key'lenir** (paralel node'lar aynı execution index'i paylaşır).
- **Multimodal input:** `conversation.InlineAttachments` (eski `withAttachments` mantığı export edildi) — text/code inline, binary/image `read_file` yoluyla. run-flow endpoint'leri `attachments` alır, flow `{{input}}`'una katlar; agent node'ları ek içeriği görür. `SlashCommand.run(input?, attachments?)` + `Composer.send` "/" flow komutuna composer'daki ekleri geçirir.
- **Sınırlama:** node-içi token akışı yok (node tamamlanınca çıktısı gelir); granülerlik node düzeyinde. flow_run trace + non-stream `run-flow` endpoint'i korundu.
- ✅ Curl ile uçtan uca doğrulandı: paralel akış (pros/cons eşzamanlı start→done→conclude→reply); metin attachment'ı boş input'la NEGATIVE sınıflandı (ek `{{input}}`'a ulaştı). `go build` + `tsc --noEmit` yeşil.

> ⚠️ Not: bu oturumda repo'da **eşzamanlı başka bir düzenleme süreci** aktifti (commit `8d54448` "style(ui)…" `git add -A` ile backend dosyalarımı da kapsadı); fonksiyonel olarak tüm değişiklikler HEAD'de ve build yeşil, ancak commit ayrımı ideal değil.

## Faz A1 — Agent loop recovery + `continuationReason` ✅ (2026-06-17)

Native tool döngüsü (`agent/toolloop.go`) "happy-path" odaklıydı; max-token / bağlam-taşması gibi durumlarda yapısal kurtarma yoktu. `observed-behavior`'in gerçek query-loop implementasyonu (`src/query/transitions.ts` + `src/query.ts`) referans alınarak kurtarma yolları yapısal hale getirildi. **Tasarım ilkesi (audit'ten):** kurtarma *kararı* (saf, I/O'suz, test edilebilir) yürütmeden ayrıldı.

**Faz 1 — saf karar katmanı + max-token kurtarma:**
- **`agent/recovery.go` (yeni):** `loopState` (iterasyonlar arası tek-atımlık guard'lar: `maxTokenRetries`/`compacted`/`lastContinue`) + `decideRecovery(resp, callErr, st) decision` saf fonksiyonu. `contReason`/`termReason` makine etiketleri (audit'in `Continue`/`Terminal` transition'larının Go karşılığı). `lastContinue` State'te tutulur → test mesaj içeriğine bakmadan kurtarma yolunun tetiklendiğini assert eder (audit deseni).
- **Max-token resume:** model `StopMaxTok` ile yarıda kesilince (guard limit `maxTokenRetryLimit=3`) İngilizce "resume directly" meta-mesajı enjekte edilip tur sürdürülür; ara parçalar `partial strings.Builder` ile birleştirilip tam cevap döndürülür (truncation kaybı yok).
- **Withhold deseni:** kurtarma turunda ara hata `emit` edilmez — yalnız `StepRecovery` (Adım Türleri ekranı zaten render eder) yayılır; hata sadece guard tükenince yüzeye çıkar.
- **`providers/minimax.go`:** `finish_reason:"length"` → `StopMaxTok` map'i (`oaiStopReason`, Complete + Stream). Anthropic `stop_reason`'ı zaten ham geçiriyordu.

**Faz 2 — reaktif compaction:**
- **`conversation/reactive.go` (yeni):** `CompactInFlightMessages(ctx, provider, agent, msgs, keepRecent)` paket-fonksiyonu — DB'ye dokunmadan (in-flight/transient) eski mesajları özetler. **Fold sınırı assistant mesajında** seçilir → summary(user)→assistant tail ile rol-alternasyonu korunur ve hiçbir `tool_use`/`tool_result` çifti bölünmez; güvenli sınır yoksa `ok=false` (no-op, orijinal hata yüzeye çıkar). Katmanlama: `agent → conversation` (cycle yok), `Runtime`'a wiring/arayüz gerekmez.
- **Döngü entegrasyonu:** provider hatası `isContextOverflow` pattern'ine uyuyor ve `!compacted` ise compaction çağrılır, başarılıysa `reactive_compact_retry` ile tur yeniden denenir; aksi halde `provider_error` terminal.

**Testler:** `agent/recovery_test.go` (6-vakalı `decideRecovery` tablo testi + `isContextOverflow` + toggle-kapalı/0-deneme vakaları), `conversation/reactive_test.go` (assistant-sınır fold + güvenli-sınır-yok no-op), `agent/recovery_loop_test.go` (scripted fake provider ile tam-döngü entegrasyon: max-token resume stitch / reaktif compaction retry / sınır-yok terminal). ✅ `go build`/`vet`/`test ./...` tümü yeşil.

**Ayarlanabilir (2026-06-17):** kurtarma parametreleri artık **Ayarlar → Bağlam**'tan canlı yönetiliyor (sabit değildi). `settings.ReactiveCompact` (vars. açık) + `MaxTokenRetries` (vars. 3, clamp 0–10; **0 = resume kapalı**) + `ReactiveKeepRecent` (vars. 6, clamp 2–50); `applySettings`→`Tunables.SetRecoveryLimits` ile push; `toolloop.go` döngü başında `r.tun`'dan `recoveryConfig`+`keepRecent` çözer, `decideRecovery(resp, err, ls, cfg)` saf imzası policy'yi parametre alır (test edilebilirlik korunur). `NewTunables` recovery knob'larını default-on kurar (applySettings'i atlayan test runtime'ları sağlam davranır). UI: `ContextPanel`'e açıklayıcı metin bloğu ("Tur kurtarma (A1)" — neyi neden yaptığını anlatan kısa metin) + "Reaktif sıkıştırma" toggle + 2 sayı alanı. **Canlı API round-trip** (scratch instance :8097): defaults `true/3/6`, clamp `99→10`/`1→2`/`80→50`, `0` korunur (disable), persist+GET doğrulandı. tsc + vite build yeşil. **Not:** çalışan eski binary yeni alanları görmek için restart ister; UI görsel testi (Chrome) restart sonrası yapılabilir.

**Kalan (sonraki adımlar):** A3 ile birleştirme (iptalde yarım `tool_call`'lara sentetik `cancelled` sonucu), max-token escalation merdiveni (8k→64k, provider'a `max_tokens` ayarı gerekir), claude-cli yolu kendi döngüsünü sürdüğünden bu kurtarmaları kullanmaz (SDK parite deseni — not).

## Ara özellik — Kanban yenileme turu: avatar → panel → cron → flow-backed task ⏳ (2026-06-17, son madde COMMITSİZ)

Kanban panosu (Faz 5) bir dizi kademeli iyileştirmeden geçti. İlk dördü commit'lendi, sonuncusu (flow-backed task) kod olarak hazır ama paralel oturumla iç içe olduğu için commit beklemede.

1. **Ajan avatarları** (commit `05742be`): yeni-görev formunda düz `<select>` → avatarlı `AgentPicker`; her kartta owner ajan `AgentAvatar` ikonuyla.
2. **Karttan cron'a bağlama** (commit `d64f0d0`): kartta **⏰ Zamanla** ile görevi doğrudan bir cron zamanlamasına bağlama (`taskId` set → scheduler `RunTask`).
3. **Detay paneli** (commit `a97eaed`): karta tıklayınca sağdan açılan `TaskDetailPanel` — başlık/prompt/açıklama/owner/durum görüntüle+düzenle.
4. **Aksiyonlar panele taşındı** (commit `4139a1b`): kart sade bir özet oldu; ▶ Çalıştır, ⏰ Zamanla, Geçmiş, ⟳ başlık, 🗑 Sil hepsi panele alındı.
5. **Flow-backed task** (⏳ **commit edilmedi**): chat'teki "flow'u mesajdan tetikleme" mantığının Kanban karşılığı. Göreve opsiyonel `Task.FlowID`; doluysa `RunTask` → `runTaskFlow`, prompt'u ajana göndermek yerine o orchestration akışını koşar (`RunFlow(...,nil)`), düğüm transkriptini (`renderFlowTranscript`) Run çıktısı olarak kaydeder. Owner ajan flow varken zorunlu değil. Tek çalıştırma noktası `RunTask` olduğundan **manuel ▶ / cron / ileride dispatcher hepsi flow'u destekler** — ⏰ Zamanla flow görevini bedavaya periyodik koşar. API `createTask/updateTask` `flowId` alır; frontend: yeni-görev formunda 🔀 Akış seçici, kartta flow rozeti, panelde flow seçici + "▶ Akışı çalıştır".
   - **Dosyalar:** `db/models_task.go`+`store_task.go` (FlowID), `agent/executor.go` (`runTaskFlow`/`renderFlowTranscript`), `api/tasks.go`, `frontend types/task.ts`+`api/tasks.ts`+`TaskBoard.tsx`+`TaskDetailPanel.tsx`.
   - **Durum:** `go build` + `go test ./internal/...` + frontend `tsc` yeşil. **Commit beklemede** — çalışma ağacı smart-surge oturumunun "flow attachment + tema refactor" WIP'iyle iç içe; `executor.go` onun 5-arg `RunFlow(...,Observer)` imzasına bağımlı (HEAD'de 4-arg). İki oturum reconcile edilince commit edilecek. (Derlemeyi tıkayan `flows.go` eksik `conversation` import'u eklendi — salt import.)
6. **Ajanlar panoyu yönetebiliyor** (⏳ **commit edilmedi**): `internal/tools/builtin_taskmgmt.go` — `SelfManageEnabled` ile gated 6 tool: `list_tasks` (oku), `create_task` (prompt ve/veya `flowId`, `CreatedBy` damgalı), `update_task`, `move_task` (kolon), `run_task` (RunTask, trigger `"agent"`, flow-backed dahil), `delete_task`. **Güvenlik sınırı:** oku/oluştur/düzenle/taşı/çalıştır her görevde serbest; **delete yalnız ajan-oluşturduğu görevde** (`Task.CreatedBy` provenance — schedule/agent/artifact kalıbı). `db.Task` += `CreatedBy`; `toolsetup.go` self-manage bloğuna eklendi. Testler `builtin_taskmgmt_test.go` (createdBy, move doğrulama, delete provenance, flow doğrulama, runner çağrısı) — `go build`+`vet`+`test ./internal/...` yeşil. **Görev dispatcher'ının ön koşulu** (ajan artık todo'yu okuyup `run_task` ile koşabilir).

## Ara özellik — Sohbetten akış tetikleme + sonucu session'a yazma ✅ (2026-06-17)

Akışlar (flows) artık sohbet composer'ından "/" komutuyla tetiklenebiliyor ve çıktı kalıcı bir sohbet turu olarak session'a yazılıyor.

- **Backend:** `POST /api/sessions/{id}/run-flow` (`api/flows.go` `handleSessionRunFlow`) — flow'u çalıştırır (`Runtime.RunFlow`, manuel/bütçesiz), session'a **user mesajı** (input) + **assistant mesajı** ekler. Assistant gövdesi `flowRunMarkdown` ile run trace'inden üretilir (her node = başlık + çıktı bölümü; branch "→ etiket"; hata durumu notu); mesaj **son agent node'unun ajanına** atfedilir (`finalAgentID`). `flow_run` satırı yine oluşur → trace geçmişte kalır. `handleSessionSummary` kalıbının ikizi.
- **Frontend:** Her flow sohbet "/" menüsünde bir komut olur (🔀 + slug ad, `useChatStream.ts` `flowSlug`+`chatCommands`); seçince composer'a `/slug ` yazılır, satırın geri kalanı flow input'u olur. `SlashCommand` artık `run(input?)` + `takesInput` taşıyor; `Composer.tsx` gönderimde `/ad argüman` ayrıştırıp eşleşen komutu çalıştırır. `runFlow` runner'ı `summarize` gibi optimistic user+placeholder gösterip API sonucuyla değiştirir.
- **Tasarım kararı:** flow_run trace paneli korundu (anlık teknik görünüm); session turu kalıcı + zengin + devam edilebilir kayıt. Sınırlama: flow'lar sunucuda senkron çalışır → node-node canlı token akışı yok ("⏳ çalışıyor…" → bitince transcript).
- ✅ Backend curl ile uçtan uca doğrulandı (Geri Bildirim Yönlendirici akışı: NEGATIVE → Özür Dile, assistant agentId = son node). `go build` + frontend `tsc --noEmit` yeşil. (Görsel "/" menü testi mcp-chrome kırılganlığı nedeniyle yapılamadı.)

## Ara özellik — Akış açıklaması + trace markdown render (FlowsPanel) ✅ (2026-06-17)

- FlowsPanel'de akış **açıklaması** artık düzenlenebilir (editörde textarea, sol listede ad altında özet); alan API'de zaten saklanıyordu ama UI yüzeyi yoktu.
- Trace çıktısı artık chat ile aynı `Markdown` bileşeniyle render ediliyor (agent/parallel node'ları; branch düz metin kalır).

## Ara özellik — Gated Yetenekler Ayarlar Ekranına Taşındı ✅ (2026-06-17)

**İstek:** `call_agent` (ve kardeşleri shell / self-manage) yalnızca env değişkeniyle
açılabiliyordu; **Ayarlar ekranından** yönetilebilsin.

**Çözüm — `applySettings` tek doğruluk kaynağı:**
- **`settings` paketi:** `Settings`/`DTO`/`Patch`/`Default`/`ToDTO`'ya 5 alan: `EnableShell`,
  `EnableSelfManage`, `EnableDelegation` (bool) + `DelegationMaxDepth` (vars. 3) /
  `DelegationMaxCalls` (vars. 8). `store.go` `Apply` (applyBool/applyInt) + `normalize`
  clamp: depth 1–10, calls 1–100. `Open` mevcut `settings.json`'ı `Default()` üzerine
  overlay ettiği için eski kurulumlar 3/8 alır (0 sorunu yok).
- **`agent/tunables.go`:** `SetDelegationLimits` + `DelegationMaxDepth()`/`DelegationMaxCalls()`
  (0 → `Default*` sabiti). `agent/delegate.go` runner artık sabit yerine `r.tun` limitlerini
  okur.
- **`api/server.go` `applySettings`:** `SetShellEnabled`/`SetSelfManageEnabled`/
  `SetDelegationEnabled`/`SetDelegationLimits` canlı push (boot + her kayıtta).
- **`main.go`:** `TIONSWARM_ENABLE_*` env değişkenleri artık **tek seferlik boot seed**'i —
  truthy ise ilgili yeteneği settings'e **açar** (asla kapatmaz), sonra Ayarlar tek doğruluk
  kaynağı. Eski dev akışları çalışmaya devam eder.
- **Frontend:** `types/settings.ts` (5 alan), yeni **"Yetenekler (Araçlar)"** kategorisi
  (`primitives.tsx` Cat + `Wrench` ikonu), `appPanels.tsx` `ToolsPanel` (3 toggle + delegation
  açıkken depth/calls sayı alanları), `SettingsPanel.tsx` wiring + save patch.

**Test:** `internal/settings/store_test.go` (round-trip + clamp + reload + DTO). **Canlı
doğrulama** (izole instance, port 8099, model çağrısı yok): GET varsayılan 3/8 → PUT enable +
depth 99→10 / calls 0→1 clamp + persist → `call_agent` **canlı araç kataloğunda belirir**
(applySettings→tun→buildRegistry) → disable edince **kaybolur**. `go test ./...` + `tsc` yeşil.

## Ara özellik — Ajan→Ajan Delegasyonu (`call_agent` tool) ✅ (2026-06-17)

**İstek:** Bir sohbet sırasında bir ajanın başka bir ajanı **etiketleyerek/çağırarak**
ona alt-görev devredebilmesi. Beyin fırtınası sonrası kararlar: **senkron** (çağıran
bekler), **bağlam mirası** (çağrılan ajan, çağıranın gördüğü tüm geçmişi + çağıranın
yazdıklarını görür), **3 döngü koruması birden**, çağıran sırası **önce kullanıcı→ajan**
(zaten `@mention` ile mevcut) **sonra ajan→ajan**, mekanizma **tool** (yapılandırılmış,
güvenli) ama mesaj akışında bozuk görüntü oluşturmadan.

**Tasarım — bir built-in tool olarak (`call_agent`):**
- **`internal/tools/delegate.go` (yeni):** `CallAgentTool` (`{agent, task}` şeması) +
  context köprüsü (`WithDelegation`/`DelegationFrom` + `DelegateRunner`/`DelegateResult`).
  `WithAsker` kalıbını birebir izler — built-in tool, `agent` paketini import etmez
  (döngü yok); runner context üzerinden enjekte edilir.
- **`internal/agent/delegate.go` (yeni):** `Runtime.withDelegation` — çağrı-grafı konumunu
  (`delegState`: depth + visited-set + paylaşılan call-budget sayacı) context'te taşır,
  runner'ı kurar. Runner **3 korumayı** uygular:
  1. **Derinlik (depth):** zincir `DefaultMaxDelegationDepth=3`'e ulaştıysa reddeder.
  2. **Döngü (visited-set):** zincirde zaten olan (veya çağıranın kendisi) bir ajan tekrar
     çağrılamaz → tüm a→b→a döngüleri kapanır.
  3. **Bütçe (budget):** tur başına toplam `DefaultMaxDelegationCalls=8` delegasyon.
- **Bağlam mirası:** `inheritedMessages` çağıranın **canlı** isteğini (`&req`) okur — alt-ajan,
  araç bağlantısı (tool_use/tool_result) temizlenmiş, okunabilir geçmişi + çağıranın bu turda
  yazdıklarını + delegasyon görevini görür (dangling tool_use riski yok). Alt-ajan kendi
  persona/model/araçlarıyla **tam bir tur** koşar (`completeTraced` özyinelemesi).
- **Wiring:** `toolloop.go` native döngüde `ctx = r.withDelegation(...)`; `toolsetup.go`
  tool'u **gated** ekler (`r.tun.DelegationEnabled()`); `tunables.go` `DelegationEnabled`;
  `main.go` `TIONSWARM_ENABLE_DELEGATION` env bayrağı (varsayılan kapalı — her çağrı tam bir
  ajan turu = token maliyeti).
- **Mesaj akışı:** Delegasyon, çağıranın izinde standart bir **tool kartı** olarak görünür
  (input=`{agent, task}`, output=alt-ajanın cevabı). Frontend `lib/tools.ts`'e `call_agent`
  ikonu (🤝) + özet anahtarı eklendi. Akış bozulmaz.

**Kapsam notu:** `call_agent` yalnızca **native tool yolu** (anthropic) için çalışır;
claude-cli MCP delegasyon yolunda built-in tool'lar TionSwarm tarafından koşulmaz.

**Test:** `internal/agent/delegate_test.go` — gate (açık/kapalı, gerçek registry yolu),
3 korumanın da reddi (cycle/self, depth, budget), bilinmeyen ajan, `inheritedMessages`
temizliği. `go build`/`go vet`/`go test ./...` + frontend `tsc --noEmit` yeşil. Gerçek iki-
ajan E2E (model çağrısı) manuel doğrulamaya bırakıldı (API kredisi + canlı yığın gerekir).

## Ara özellik — Tur Hatalarını Sohbet Hiyerarşisinde Gösterme ✅ (2026-06-17)

**İstek:** Bir mesaj sonucu hata oluşursa (server taraflı veya client taraflı), bunu
köşedeki bir banner yerine **sohbet mesaj hiyerarşisinde hata detayıyla** gösterelim.

**Önceki davranış:** Hata olunca `onError`/`catch` canlı balonu **ve kullanıcı mesajını
siliyordu**; yalnızca sol üstte global bir hata banner'ı çıkıyordu (sayfa yenilenince hata
tamamen kayboluyordu). Backend ise hata anında hiçbir şey kalıcılaştırmıyordu.

**Çözüm — mevcut `StepError` altyapısını hata yoluna bağlama:**
- **Backend (`internal/api/chat_stream.go`):** Yeni `failTurn(...)` yardımcısı — tur düzeyi
  her hatada bir `error` adımı (text=detay, reason=makine etiketi) içeren **assistant mesajı
  kalıcılaştırır** (hiyerarşide görünür + reload'da kalır), sonra `error` SSE event'ini bu
  mesajla (`replyMessage`) birlikte gönderir. Döngüdeki hata noktaları buna bağlandı:
  `provider_unavailable`, `history_error`, `compaction_failed`, `provider_error`,
  `persist_error`.
- **Frontend (`api/chat.ts`):** `onError(err, replyMessage?)` — `error` event'i artık
  opsiyonel kalıcı mesajı taşıyor.
- **Frontend (`hooks/useChatStream.ts`):** `renderTurnError(detail, replyMessage?)` — hatayı
  transcript'e yazar (banner yerine). Kullanıcı mesajı korunur; canlı (kaydedilmemiş) balon
  hata balonuyla değişir. Server kalıcı mesaj gönderdiyse o gösterilir; client/transport
  hatasında yerel bir `error` balonu sentezlenir (`reason: client_error`).
- Render zaten mevcut `ErrorStep` bileşeniyle yapılıyor (⛔ + detay + reason rozeti).

**Test:** İzole instance'ta (port 8099) bozuk-provider ajanıyla deterministik hata tetiklendi.
Doğrulandı: SSE `error` event'i `{error, reason, replyMessage}` taşıyor; kalıcı mesajlar
`[user] 'selam'` + `[assistant] steps=[{kind:error, text:"anthropic provider not configured…",
reason:"provider_unavailable"}]`. `go build`/`vet` + frontend `tsc`/`vite build` yeşil.

## Faz SM — Ajan Self-Management Araçları ✅ (2026-06-17)

**İstek:** Ajan, sohbet esnasında TionSwarm'nun kendisini yönetebilsin — yeni ajan/flow/
schedule/artifact oluştur-sil-düzenle, hafızaya ekle, logları oku. (Built-in in-process
tool olarak; MCP/REST katmanı **değil** — tek binary felsefesi + bedava workspace izolasyonu.)

**Temel İlke — "kim oluşturdu" tag sistemi:** Ajan yalnızca **bir ajan tarafından
oluşturulmuş** kaynakları silebilir/düzenleyebilir; kullanıcının elle yaptıklarına dokunamaz.
- `db.Agent`/`db.Flow`/`db.Schedule`'a yeni `CreatedBy string` alanı (`omitempty`, geriye
  uyumlu — eski JSON'da yok = boş). `Artifact` ve `KnowledgeSource` zaten `AgentID` taşıyor.
- `CreatedBy == ""` → kullanıcı/sistem (korumalı). `CreatedBy != ""` → ajan-oluşturma
  (değer = oluşturan ajan ID). Guard hatası: "created by the user and cannot be edited…".

**Araçlar (16 yeni, `internal/tools/`):**
- `builtin_agentmgmt.go` — `create_agent` / `update_agent` / `delete_agent` / `list_agents`
  (delete kendini reddeder; create heartbeat'li ajanın worker'ını anında başlatır).
- `builtin_flowmgmt.go` — `create_flow` / `update_flow` / `delete_flow` / `list_flows` /
  `get_flow` / `run_flow`. Graph doğrulaması artık **derin**: `validGraphJSON` boş olmayan
  grafiği `orchestration.ParseGraph` + `Graph.Validate` ile geçirir (start node, benzersiz id,
  çözülebilir referanslar, atanmış agent node) → bozuk grafik create/update anında reddedilir
  (eskiden yalnız "geçerli JSON mu" bakılıyordu, hata çalışınca patlıyordu). `get_flow` flow'u
  **graph dahil** tam döndürür (list_flows graph'ı atlar) → ajan grafiği okuyup düzenleyip
  update_flow ile geri yazabilir. `run_flow` herhangi bir akışı `{{input}}` ile çalıştırır
  (otonom, bütçe-kapılı), `RunFlowRecorded` üzerinden çalıştırmalar akışına kaydeder; edit/delete
  provenance-kapılı (yalnız ajan-yapımı), get/run ise kapısız (okuma/çalıştırma yıkıcı değil).
- `builtin_schedulemgmt.go` — `create_schedule` / `update_schedule` / `delete_schedule` /
  `list_schedules` (her değişimde `Scheduler.Reload` → cron anında etkili).
- `builtin_artifactmgmt.go` — `delete_artifact` / `list_artifacts` (create/update zaten
  per-turn sink ile var).
- `builtin_memory_add.go` — `memory_add` (document/reflection; ajanın kendi belleğine yazar).
- `builtin_logs.go` — `read_logs` (logbuf ring buffer'dan, level/q filtresi).

**Wiring:** Kimlik `buildRegistry(ctx, agent)`'ten gelir (`agent.ID`) — context bridge yok.
`Runtime`'a `logs *logbuf.Buffer` + `reloadSched` callback (`SetScheduleReloader`, manager
`sched.Reload`'u bağlar) eklendi. `NewRuntime`/`NewManager` imzaları + `main.go` güncellendi.

**Gate:** Self-management paketi native kataloğu **19 → 37**'ye çıkarır (~+2000 tok/tur), ve
ajanın workspace'i değiştirmesine izin verir → **varsayılan KAPALI**, `TIONSWARM_ENABLE_SELFMANAGE=1`
ile açılır (shell gate deseni; `Tunables.SelfManageEnabled`). Per-agent allowlist + workspace
denylist yine geçerli.

**Test:** `builtin_selfmanage_test.go` (provenance damgalama, user-created reddi, self-delete
reddi, schedule reload çağrısı, flow graph doğrulama, memory_add, artifact guard) ✅.
**Canlı smoke** (`/api/workspace-tools`): gate açık → 35 araç/16 self-manage; kapalı → 19/0
doğrulandı. `go build`/`vet`/`test ./...` yeşil.

**Kalan (v2):** Settings UI toggle (şu an env-var, shell ile aynı), UI'da "ajan oluşturdu"
rozeti, `request_confirmation` ile riskli işlemlere onay gate'i.

## Ara özellik — Workspace Şablonları (Templates) ✅ (2026-06-17)

**İstek:** Yeni workspace'ler boş tek-ajan yerine, belirli bir iş türüne (araştırma,
yazılım, günlük rutin) hazır gelsin.

**Çözüm:** Workspace oluşturmada **şablon seçimi**. Her şablon; ajan kadrosu + onları
sırayla bağlayan bir **orchestration akışı** + (opsiyonel) **devre dışı başlangıç
zamanlamaları** tohumlar.

- **`internal/api/templates.go`** (yeni) — `workspaceTemplates` kayıt defteri (4 şablon):
  - `blank` (Boş): eski tek "Asistan" davranışı + saatlik devre dışı görev-özeti zamanlaması.
  - `research` (Bilimsel Araştırma): Literatür Tarayıcı → Metodolog → Analist → Hakem +
    uçtan uca araştırma akışı.
  - `software` (Yazılım Geliştirme): Search → Plan → Execute → Verify ajanları + akışı.
  - `daily` (Günlük Rutin): Planlayıcı/Koç/Hatırlatıcı + sabah(08:00)/akşam(20:00) devre
    dışı zamanlamalar.
  - `seedTemplate` ajanları oluşturur (key→gerçek ID eşler), `seedTemplateFlow` ile lineer
    grafiği kurup `Graph.Validate()` sonrası flow olarak saklar, zamanlamaları **disabled**
    ekler. Tüm hatalar loglanır ama **non-fatal** (workspace yine kullanılır).
- **API:** `GET /api/workspace-templates` (katalog: id/ad/açıklama/ikon/ajan sayısı/akış var mı).
  `POST /api/workspaces` artık `template` alanı alır; bilinmeyen/boş → `blank` fallback
  (`templateByID`). Eski `seedDefaultAgent`/`seedDefaultSchedule` kaldırıldı, yerini
  `seedTemplate` aldı (`workspaces.go` sadeleşti).
- **UI** (`WorkspaceCreateModal.tsx`): ad alanının üstünde **şablon seçici** kart listesi
  (ikon + ad + "N ajan · akış" rozeti + açıklama). Şablon seçince ikon otomatik adapte olur.
  `listWorkspaceTemplates()` + `createWorkspace({...template})` (`api/workspaces.ts`),
  `WorkspaceTemplate` tipi (`types/workspace.ts`), `NewWorkspaceData.template`.

**Test:** `go build ./...` + `go test ./internal/api` yeşil (`templates_test.go`: şablon
bütünlüğü — benzersiz step id, çözülen ajan key'leri, geçerli grafik; `templateByID`
fallback). Frontend `tsc --noEmit` temiz.

## Ara özellik — Sır Kasası (Secret Vault) ✅ (2026-06-17)

**İstek:** "Uygulamaya secret/şifre tutabileceğimiz bir ekran ekleyelim; workspace'teki
ajanlar da onlara erişebilsin."

**Çözüm:** Workspace-izolasyonuna uygun, **her workspace'in kendi şifreli kasası**.

- **`internal/secrets`** (yeni paket) — `Vault`: per-workspace `store/secrets.json`,
  her değer **AES-GCM** ile şifreli (mevcut `config.Secret` cipher'ı `secrets.Cipher`
  arayüzünü karşılar). `Open/List/Get/Set/Delete` + isim validasyonu
  (`^[A-Za-z][A-Za-z0-9_.-]*$`, ≤128) + değer ≤64 KiB; atomik (temp+rename) persist.
  Değer asla düz diske yazılmaz, `List()` (Meta) değeri asla döndürmez. Round-trip +
  validasyon testleri (`vault_test.go`).
- **Wiring** — `workspace.Manager` artık `secrets.Cipher` alır (`main.go`'da `config.Secret`
  geçilir); her `Workspace`'e `Secrets *secrets.Vault` açılır ve `agent.NewRuntime`'a verilir
  (`Runtime.vault`). nil-güvenli (test/araçsız yol).
- **Ajan araçları** (`internal/tools/builtin_secret.go`) — `secret_list` (sadece
  isim+açıklama, değer yok) ve `secret_get` (isimle değeri döner). `buildRegistry`'e
  eklendi → mevcut workspace/ajan allow-deny sistemine tabi (istenirse kapatılabilir).
- **API** (`internal/api/secrets.go`, workspace-scoped, X-Workspace-Id):
  `GET /api/secrets` (maskeli liste), `POST /api/secrets` (`{name,value,description}`,
  write-only değer), `GET /api/secrets/{name}/reveal` (sahip-tetikli tek değer),
  `DELETE /api/secrets/{name}`.
- **UI** — Sol rayda yeni **"Sırlar"** ekranı (`SecretsPanel.tsx`, KeyRound ikonu):
  ekle/güncelle formu (değer `type=password`), liste (değer `••••` maskeli), göz ikonuyla
  iste-üzerine göster/gizle, kopyala, düzenle, sil. `types/secret.ts` + `api/secrets.ts`
  + barrel'lar + `App.tsx`/`NavRail.tsx` view wiring.

**Test:** `go build/vet/test` yeşil; tsc + vite temiz. **Canlı API smoke** (ayrı port 8099,
geçici data dir): set/list/reveal/delete + invalid-name 400 + **diskte şifreli** (`valueEnc`,
plaintext yok) doğrulandı; `secret_get`/`secret_list` workspace tool katalogunda görünüyor.

**Güvenlik notu:** Ajanlar `secret_get` ile değerleri okuyabildiğinden, UI'daki reveal
(sahip aksiyonu) ek bir risk getirmez. Bir workspace'te ajanların sırlara erişmesini
istemiyorsan **Araçlar** ekranından `secret_get`/`secret_list`'i kapat.

## UI fix — Edit/Write kartında diff +/- input'tan sentezleniyor (2026-06-17)

**Sorun:** `TurnStep` Edit/Write kartlarında yeşil/kırmızı diff değerleri
gözükmüyordu. Kök neden: claude-cli `Edit`/`Write` araçlarının **çıktısı bir
diff değil**, sadece onay metni ("The file … has been updated" / "File created
successfully…"). `parseDiff(output)` +/- bulamadığından rozet de DiffView
yeşil/kırmızısı da boştu.

**Çözüm:** Diff'i **çıktıdan değil tool input'undan** sentezle. `lib/diff.ts`
`synthDiff(toolBase, input)`: **Write** → `content` tüm satırları `+` (yeni
dosya); **Edit** → `old_string` satırları `-`, `new_string` satırları `+`.
`ActivityCard`: `diffText = output diff gibiyse output, değilse synthDiff(input)`
→ hem **başlık rozeti** (`+X −Y`) hem gövdedeki **DiffView** (artık "Değişiklik"
başlığıyla) bunu kullanır. ✅ tsc + vite temiz. **Playwright canlı**: Write kartı
**+83 −0** (fetch_weather.py), Edit kartı **+1 −4** (CSV son 3 satır silme)
rozetleri doğru render etti. Native yol zaten `diff` step → `DiffCard` ile gerçek
sayıları gösteriyordu; bu fix claude-cli (output=onay metni) yolunu eşitledi.

## Faz A1.2 — Artifact: versiyonlamayı kaldır + dosya çıktısını otomatik yakala ✅ (2026-06-17)

**İstek:** "artifact sisteminden versiyonlamayı kaldır; ve bir session'da dosya çıktısı istediğimde otomatik olarak artifact'a atsın — default olarak yapsın (önceki denememde yapmadı)."

**1) Versiyonlama kaldırıldı**
- `db.Artifact`'tan `Version` + `Revisions[]` çıkarıldı; `ArtifactRevision` silindi. Yeni alan `SourcePath` (otomatik yakalanan dosya yolu, dedup için). Güncelleme artık **yerinde overwrite** (`UpdateArtifactContent(id, content)`), revizyon arşivlemesi yok.
- Tool: `update_artifact` `note` parametresi kaldırıldı; sonuç ref'i `{id,title,kind,action}` (artık `version` yok). `ArtifactSink.UpdateArtifact(ctx,id,content)`.
- Frontend: `types/artifact.ts` `version`/`revisions` çıktı + `sourcePath?` eklendi; `ArtifactsPanel` sürüm seçici + not alanı + `viewVersion` kaldırıldı (yalnız Düzenle→overwrite, Kopyala, Kaynağa git, Sil); `ArtifactCard` sürüm satırı kaldırıldı; liste satırı `Kind · zaman`.

**2) Dosya çıktısı → otomatik artifact (varsayılan)**
- **Tur-sonu trace taraması** (`api/artifacts_auto.go` `captureFileArtifacts`): asistan turu bitince trace'teki dosya-yazan araç çağrıları (`write_file` native + `Write` claude-cli; `{path}`/`{file_path}`+`{content}`) yakalanır → `db.SaveFileArtifact` ile **session+sourcePath'e göre upsert** (tekrar yazımda kopya değil güncelleme). Uzantıdan tür çıkarımı (`.md→markdown`, `.html→html`, `.csv/.txt→text`, kod uzantıları→`code`+dil). `chat.go` + `chat_stream.go` reply sonrası çağrılır. **Sağlayıcıdan bağımsız** (her iki yol da trace üretir).
- **Varsayılan prompt yönlendirmesi** (`artifactDeliverableGuidance`, statik prefix, `chat_turn.go`): "dosya/doküman/dataset/rapor üretirken dosya-yazma aracıyla yaz (otomatik artifact olur) ya da `create_artifact` çağır; ad-hoc shell/script ile üretip artifact yakalamayı atlama." → ajan python-to-disk yerine Write aracını tercih eder, o da yakalanır.

**CANLI TEST (gerçek claude-cli E2E + unit):**
- [x] Gerçek tur: ajan `greeting.md`'yi **Write** aracıyla yazdı → otomatik **markdown** artifact oluştu (title=greeting.md, sourcePath dolu, `version` alanı yok). Kullanıcının senaryosu birebir doğrulandı.
- [x] Dedup: aynı dosyayı 2. turda yeniden yazdırınca artifact **tek kaldı**, içerik güncellendi.
- [x] Manuel düzenleme: `PUT {content}` yerinde overwrite (`version`/`revisions` alanı yok).
- [x] Unit: `TestParseFileWrite` (native/cli/non-write/empty), `TestArtifactKindForPath`, `TestCaptureFileArtifacts_DedupByPath` (dedup + errored-write skip), `TestArtifactsContextBlock` — hepsi PASS.
- [x] `go build/vet/test ./...` + frontend `tsc --noEmit` + `vite build` temiz. UI: panel sürümsüz layout render etti (liste `Kind · zaman`, sürüm seçici yok).

> Not: claude-cli artık Interaction MCP üzerinden `create_artifact`/`update_artifact`'a da erişiyor (paralel iş); native yol context köprüsüyle. İki mekanizma birlikte: model dosya yazarsa otomatik, içerik doğrudan üretirse create_artifact.

## Ara özellik — URL deep-link routing (uygulama içi URL ile gezme, 2026-06-17)

**İstek:** "uygulama içi url ile gezmeyi etkinleştir, mesela istediğimiz workspace'in istediğimiz session'una, agent'ına, zamanlayıcısına vs gidebilelim."

Uygulama tamamen state-tabanlıydı (URL routing yoktu). Artık navigasyon durumu **URL hash'inde** adreslenir → paylaşılabilir/yenilemede geri yüklenir, ileri/geri tuşları çalışır.

- **Şema:** `#/w/{workspaceId}/{view}[/{entityId}]`. `entityId` görünüme göre: chat→sessionId, agents/memory/tools→agentId, artifacts→artifactId, schedules→scheduleId; diğerleri yok sayar. Hash-tabanlı seçildi: tek-binary serve + Vite dev proxy'de sunucu route config gerektirmez.
- **Saf yardımcılar** `frontend/src/lib/url.ts`: `parseRoute` (bilinmeyen view→`chat`, malformed→default), `buildRoute`, `routeIdForView`.
- **Hook** `frontend/src/hooks/useUrlSync.ts`: state→URL (ilk yazım `replaceState`, sonrası `pushState`); URL→state (`popstate`+`hashchange`→`applyRoute`). `pushState`/`replaceState` `hashchange`/`popstate` tetiklemediğinden ve `applyRoute` yalnız fark olunca state'i değiştirdiğinden besleme döngüsü yok.
- **App.tsx wiring:** modül-yükünde `INITIAL_ROUTE` parse → `setActiveWorkspace` (geçersiz id `useWorkspaces`'te ilk ws'e düşer); workspace-yükleme effect'i `Promise.all([listAgents, listSessions])` sonrası `pendingRouteRef`'i bir kez tüketerek deep-link entity'sini seçer (ilk yük + çapraz-ws nav). `applyRoute` (aynı ws→entity'yi hemen uygula; ws değişimi→`pendingRouteRef`+`switchWorkspace`). `focusAgent` (agent-scoped görünümlerde aktif ajanı, varsayılanı bozmadan seçer).
- **Kontrollü bileşenler:** `AgentsView` `selectedId`/`onSelectAgent` prop'ları (verilmezse iç state); `Schedules` `focusId` → deep-link satırına scroll + 2.5sn highlight ring.

✅ `tsc -b`/`vite build` yeşil; `url.ts` saf fonksiyonları **25/25** round-trip/parse testinden geçti (geçici tsx test koşuldu+silindi). **Not:** gateway-manager MCP bu oturuma tool olarak gelmediğinden (gateway 9091 + mcp-chrome 12306 ayakta olsa da) canlı Chrome görsel testi yapılmadı — sıraya alındı.

## Bugfix — Zamanlanmış prompt artık gerçek sohbet turu olarak görünür (2026-06-17)

**Şikâyet:** "Saat başına eklediğim bir zamanlayıcı `failure ... (claude CLI failed: exit status 1)` hatası verdi, ve istediğim ui görüntüsü olmamış."

**1) `exit status 1` hatası — geçici (transient):** İnceleme: claude-cli düz headless çağrısı bu ortamda sorunsuz (`exit 0`); otonom (zamanlama) yolu da canlı testte **başarılı** oldu (aynı ajan/prompt). Kök neden: hata anında backend defalarca kapatılıp yeniden derleniyordu (geliştirme); claude-cli süreç ortasında **ctx iptaliyle** kesildiğinde **boş stderr ile `exit status 1`** döner — kullanıcının gördüğü mesaj tam da budur (stderr boş). Yani normal çalışmada kalıcı bir bug değil, kesinti artığı. (Sağlamlaştırma fikri: iptal kaynaklı hataları `context.Canceled` olarak ayırıp "kesildi" diye göstermek — ileride.)

**2) UI görüntüsü — düzeltildi:** `agent/scheduler.go` `deliverPrompt` zamanlanmış cevabı tek bir `"[schedule] …"` **assistant** balonu olarak, üstelik **`AgentID` boş** yazıyordu → schedule oturumunda ajan avatarı/kimliği render olmuyordu ve **gönderilen prompt hiç görünmüyordu** (user mesajı yok). Düzeltme: (a) prompt önce **user mesajı** olarak yazılır (thread gerçek sohbet gibi okunur), (b) cevap **`AgentID` damgalı** + **aktivite izi (`Steps`)** ile yazılır (avatar + thinking/araç adımları normal sohbet turu gibi görünür). Yeni `agent/executor.go` `invokeTraced` (CompleteWithToolsTraced sarmalayıcı) izle birlikte döner; `encodeSteps` ile serileştirilir. `"[schedule]"` ön eki kaldırıldı.

✅ `go build`/`vet` yeşil; commit `127fa5e`. Canlı test: dakikalık test zamanlaması ile schedule oturumunda user+assistant turu, ajan avatarı ve adımlar doğrulanıyor.

## Sohbet — Tek mesaj silme (2026-06-17)

**İstek:** Bir sohbette tek bir mesajı (ör. hatalı/test mesajı) silebilmek.

- **Backend:** `db.DeleteMessage(ctx, sessionID, messageID)` — bellekten çıkarır,
  `MessageCount`'u düşürür ve session JSONL'ini **tam yeniden yazar**
  (`writeSessionFileLocked`; append-only depo olduğundan satır-içi silme yok).
  Yeni uç **`DELETE /api/sessions/{id}/messages/{msgId}`** (`handleDeleteMessage`,
  `writeDBError` → bilinmeyen mesaj 404 JSON). Test `delete_message_test.go`
  (ortadaki mesaj silinir, sıra korunur, reopen sonrası kalıcı, bilinmeyen id →
  ErrNotFound).
- **Frontend:** `api.deleteMessage(sid, mid)`; `App.tsx` `deleteMessage`
  (API → `setMessages` filtre → `refreshSessions`); `MessageList` her mesaj
  satırına **hover'da 🗑** butonu (`group-hover`, user+assistant; akıştaki canlı
  balon hariç). **Onay native dialog değil, inline iki-adımlı**: 🗑 → kırmızı
  **"Sil" / ✕** belirir → "Sil" siler (`DeleteButton` `armed` state). Bu, akıcı
  bir UI silme sağlar ve CDP/otomasyonda native confirm'in oto-iptalini de önler.

✅ go build/vet + `delete_message_test` yeşil; tsc + vite temiz. **Playwright
canlı E2E** (8090'a güncel binary alınarak): son mesajın 🗑 → "Sil" → mesaj
silindi (**28 → 27**, JSONL'e kalıcı, konsol temiz). Test sırasında `MessageList`
`useState` import eksiği (DeleteButton'da kullanılıyordu ama import yoktu →
runtime crash) yakalanıp düzeltildi. **Not:** Çalışma ağacında eşzamanlı
attachments (`store.go` upload temizliği) + url-sync (`App.tsx`) işi bu
değişikliklerle iç içe → ayrı temiz commit çıkarılamadığından **commit kullanıcıya
bırakıldı**.

## Bağlam — Aktif todo listesi sistem promptuna enjekte (2026-06-17)

**İstek:** Agent, `todo_write` mesajı sohbette geride kalsa / compaction ile
bağlamdan çıksa bile oturumun **aktif** todo listesini görebilsin.

**Çözüm:** Artifacts-context enjeksiyonuyla aynı desen. `internal/api/todos.go`
`todoContextBlock(ctx, db, sessionID)` — oturumun **tüm** mesajlarını (compaction
penceresinden bağımsız) yeniden-eskiye tarayıp en son todo step'ini bulur
(`latestSessionTodos`/`stepTodos`: kind `todo` veya eski `todo_write` tool input);
tümü `completed` ise boş döner (takip edilecek bir şey yok), aksi halde
`## Active todo list (this session)` + `- [x]/[~]/[ ]` maddeleri olarak
formatlanır (`renderTodoBlock`). `chat_turn.go` `composeTurnRequest` bunu
`SystemDynamic` (cache'siz, her tur gönderilen suffix) sonuna ekler — artifacts
bloğunun hemen ardına. Native + claude-cli (System+SystemDynamic'i birleştirir)
ikisi de alır. Test `todos_test.go` (en-yeni seçim, eski tool-input parse,
format + all-done baskılama). ✅ go build/vet/test yeşil. **Playwright canlı**:
aktif liste (Alfa=completed, Beta=in_progress, Gama=pending) oluşturuldu →
ayrı bir turda "todo_write KULLANMADAN aktif listeni yaz" sorusuna agent listeyi
**doğru durumlarıyla** üretti ("hafızamdan cevapladım"). Compaction dayanıklılığı
yapısal: her tur ham geçmişten yeniden türetilip enjekte edilir.

## UI — Sabit görev listesi paneli (TodoPanel) (2026-06-17)

**İstek:** Sohbette "todo listesi oluştur" denince agent `todo_write` ile liste
yapıyor ama tek bir tura gömülü kalıyor (sıraya alınan mesaj gibi kayboluyor);
sabit bir yerde görünsün ve agent tamamlayıp güncelleyebilsin.

**Gerçek:** Todo verisi zaten kalıcı — `todo_write` her çağrıda tam listeyi
`StepTodo` (kind `todo`, `Todos[]`) olarak yayar, mesaj `steps`'ine yazılır
(reload'da kalır). Eksik olan tek şey **sabit/evrilen görünüm**.

**Çözüm (frontend-only):** Aktif oturumun mesajlarındaki **en son** todo
step'inden güncel liste türetilip composer üstünde (PendingTray yanında) sabit,
katlanabilir bir panelde gösterilir.
- `lib/todos.ts` `latestTodos(messages)` — mesajları yeniden-eskiye tarayıp son
  todo step'ini bulur (kind `todo` veya eski `todo_write` tool step'i).
- `components/chat/TodoPanel.tsx` — 📋 başlık + ince progress bar + `done/total`
  + katlanabilir checklist (✓ completed üstü-çizili, ◐ in_progress accent, ○
  pending). `App.tsx` `useMemo(latestTodos(messages))` → `<TodoPanel>` AskPrompt
  ile PendingTray arasında.
- Backend değişikliği yok: agent yeni `todo_write` çağırınca (durum güncelleme ya
  da yeni liste) en-son-kazanır mantığıyla panel otomatik güncellenir.

✅ tsc + vite build temiz (döngü yok). **Playwright canlı E2E** (claude-cli
`mcp__tionswarm_interaction__todo_write` yolu): 3 maddelik liste oluşturuldu →
panel composer üstünde **0/3** ile belirdi (önceki 4 maddelik listeyi geçersiz
kıldı = en-son-kazanır); ardından "1=completed, 2=in_progress" güncellemesi →
panel **1/3**, madde-1 ✓ üstü-çizili yeşil, madde-2 ◐ accent olarak **canlı
güncellendi**. Kalıcılık: persisted `steps`'ten türetildiği için reload'da kalır.

**İnline TodoCard collapsible (2026-06-17):** Tur izindeki `todo_write` kartı
(`TodoCard`) de diğer tool kartları gibi **başlık + chevron** ile açılıp kapanır
oldu (✅ + "Görev Listesi" + `done/total`). **Varsayılan kapalı** (sabit TodoPanel
zaten güncel listeyi gösterdiğinden inline kart uzun izlerde yer kaplamasın;
başlığa tıklayınca açılır). Sabit TodoPanel ayrı bileşen, etkilenmez. ✅
Playwright: 6 inline kartın hepsi ▸ kapalı (liste gizli) render oldu.

**Cila (2026-06-17):** (a) **Otomatik küçülme** — liste tamamen tamamlanınca
panel collapsed açılır (`useState(()=>!allDone)` + sig değişiminde
`setOpen(!allDone)`). (b) **✕ gizle butonu** — kullanıcı paneli kapatabilir;
liste değişince (yeni `todo_write`, imza `sig` farklı) otomatik geri gelir
(`dismissedSig`). (b2) **Tamamlanınca + kullanıcı mesajı → gizle** — liste tümü
`completed` olduktan **sonra** kullanıcı yeni mesaj gönderince panel kendiliğinden
gizlenir (`latestTodos`: son todo all-done ve sonrasında `role:user` mesajı varsa
`[]` döner); yeni `todo_write` daha yeni step ürettiğinden tekrar belirir. ✅
canlı: 3/3 collapsed → "tamam, teşekkürler" mesajı → panel gizlendi. (c)
**Nested-button fix** — `PathText` ve `DiffCard`'taki
tıklanabilir yol `<button>` yerine `<span role="button" tabIndex>` +
`stopPropagation` oldu; ActivityCard/DiffCard başlığı zaten `<button>` olduğundan
"button-in-button" geçersiz HTML / hydration uyarısı çıkıyordu → **giderildi**
(yola tıklayınca artık kart da toggle olmuyor, UX iyileşti). ✅ Playwright:
nested-button uyarısı konsoldan kalktı, ✕ ile panel gizlendi, tsc+vite temiz.

## Ara fix — Context metre gerçek footprint'i sayar (2026-06-17)

Oturum bilgisi panelindeki **Bağlam penceresi** ölçeri eskiden yalnızca özet +
sohbet mesajlarını sayıyordu; **sistem promptu, araç/MCP şemaları ve artifact
bloğu** (her tura giden ama mesaj olmayan içerik) hesaba katılmıyordu → panel
gerçek doluluğu olduğundan düşük gösteriyordu (kullanıcı tespiti).

- **Backend (`api/session_info.go`):** yeni `systemFillers` —
  `composeTurnRequest`'i birebir aynalayarak sistem promptunu (persona + kullanıcı
  profili + workspace talimatları), ajanın **efektif araç kataloğunu**
  (`Runtime.ToolCatalog`, built-in + MCP) ve session artifact bloğunu tahmin eder;
  `estimateToolCatalog` araç başına ad+açıklama+JSON şema (+ çerçeve) maliyetini
  toplar. `ContextTokens` artık bu ekstrayı içerir; `Fillers`'a **Sistem promptu**
  (`system`), **Araçlar** (`tools`), **Artifactlar** (`artifacts`) kovaları eklenir
  ve tümü token ağırlığına göre sıralanır.
- **Frontend (`SessionDetailPanel.tsx`):** `fillerColor`'a `tools` (mor) ve
  `artifacts` (pembe) renkleri; panel zaten fillers'ı role göre generic render
  ettiğinden başka değişiklik gerekmedi. "Boş alan" otomatik küçülür.
- **Test:** `api/session_info_test.go::TestEstimateToolCatalog` (boş=0, monotonik).
  **Canlı test:** geçici sunucu + boş oturum → `Araçlar ~1942 tok / 15 araç` +
  `Sistem promptu ~38 tok` (eskiden ~0 gösterirdi). ✅ build/vet/test + tsc yeşil.

> Cevap: tool/MCP şemaları **artık** Context hesabına dahil ve ayrı kalem olarak
> gösteriliyor. Not: hafıza recall bloğu sorgu-bağımlı olduğundan kasıtlı olarak
> hariç bırakıldı (yanıltıcı olmasın); özet zaten ayrı kovada sayılıyor.

## Faz A2 — Mesaj ekleri (attachments, 2026-06-17)

Kullanıcı sohbet turuna **çoklu dosya** ekleyebilir; ekler input üstünde tip
ikonlu chip olarak görünür (x ile iptal), gönderince kullanıcı balonunda kalır.
external-agent-oss attachment sistemi referans alındı.

- **Yükleme:** `POST /api/uploads` (multipart, `api/uploads.go`) → dosyayı
  workspace sandbox'ında `uploads/<sessionId>/<id>-<ad>` altına yazar (ajan
  `read_file` ile okur). Kaba `kind` tespiti (image/text/code/pdf/office/
  archive/audio/video); küçük (≤100KB) text/code dosyaları içeriğini
  `Attachment.TextContent`'e inline taşır. 25MB cap.
- **Kalıcılık:** `db.Attachment` (`models_attachment.go`) + `db.Message.Attachments`;
  `chat.go` + `chat_stream.go` user mesajına yazar. Metin boşsa **ek varken**
  gönderime izin.
- **Provider:** `conversation.toProviderMessages` → `withAttachments` ekleri user
  metnine katar: text/code verbatim inline (```...```), binary/görsel `read_file`
  path listesi olarak. Test: `conversation/attachments_test.go`.
- **Inline görsel servis:** `GET /api/files` workspace-göreli **`?rel=`** formu
  kazandı; `<img>` header gönderemediğinden `?ws=<id>` ile scope alır.
  Path-traversal guard. `withWorkspace` header yoksa `?ws=` query'sini okur.
- **Frontend:** `Composer` ek tepsisi (📎 buton + gizli multi-input + drag-drop +
  **paste**: >2000 karakter pano metni `.txt` ekine, yapıştırılan görsel
  yüklenir), dosya-başına yükleme durumu + **x**; `AttachmentChip` (görsel
  thumbnail / dosya kartı) tepside + `UserBubble`'da; `api/uploads.ts`,
  `lib/attachments.tsx` (ikon/etiket/`imageURL`), `types/attachment.ts`.

✅ go build/vet/test + tsc -b/vite build yeşil. **API canlı round-trip** (geçici
:8091 scratch instance): text+görsel upload → doğru kind/rel/size, `?rel=` görsel
servis 200 image/png, traversal `../../` → 400, diske kalıcılık, `TextContent`
inline doğrulandı.

**Fix (9044e0b):** claude-cli ajanı eki okuyamıyordu — `Read` aracı
`uploads/<sid>/<dosya>` göreli yolunu backend'in başlatma dizinine göre
çözüyordu. CLI alt sürecinin çalışma dizini **workspace sandbox köküne**
ayarlandı (`providers.Request.WorkDir` → `toolloop.completeTraced` tek noktada
`req.WorkDir = r.workDir` → `claudecli.cmd.Dir`). Artık yol hem native hem
claude-cli için doğru çözülür.

**Şişme/temizlik (a7116a2):** oturum silinince upload dizini de silinir —
`db.sessionUploadsDir(sid)` (`<storeRoot>/../workspace/uploads/sid`) hem
`DeleteSession` hem `DeleteAgent` oturum-kaskadında `RemoveAll` edilir
(`uploads_cleanup_test.go`; canlı doğrulandı: yükle→sil→dizin yok).

**Şişme önlemleri (ee99e9b):** (1) tray'de x ile **gönderilmeden iptal** edilen
ek diskten silinir — `DELETE /api/uploads?rel=` (uploads alt-ağacına sınırlı,
traversal-guard) + frontend `uploadsApi.deleteFile` → `Composer.removePending`.
(2) `maxInlineTextBytes` **100KB→16KB**: büyük text/code ekleri artık her turda
bağlama inline edilmez (diskte kalır, `read_file` ile okunur). Canlı doğrulandı:
500B inline, 20KB inline değil, DELETE 204, uploads-dışı path 400. **Kalan
(v2):** sekme-kapatma/yenileme ile gönderilmeden bırakılan ekler yetim kalabilir
(yaş/kota bazlı GC); native görsel multimodal.

**Not:** mcp-chrome bu oturumda bağlı olmadığından tarayıcı görsel testi
yapılamadı. **v2:** görsel **multimodal** (model görseli görür — provider
katmanına anthropic image content-block eklenmeli; claude-cli kendi Read'i ile
görebilir), "Artifact'a dönüştür" butonu, drag-drop cilası.

## UI — Yenileme sonrası "düşünüyor" göstergesinin geri yüklenmesi (2026-06-17)

**Sorun:** Mesaj gönderdikten hemen sonra sayfa yenilenince "agent düşünüyor"
göstergesi kayboluyordu. Turlar istemci bağlantısından **detached** olduğundan
sunucuda çalışmaya devam eder; ama reload sonrası client'ın `pending` state'i
sıfırlandığından gösterge gider, yanıt ancak tur bitip `chat` event'i gelince
görünür — arada "boşluk" oluşuyordu.

**Çözüm:** Sunucu, hangi oturumların **uçuşta** turu olduğunu bildirir; frontend
reload'da bunu sorgulayıp göstergeyi geri yükler.
- **Backend:** `chatRun`'a `sessionID` alanı + `chatRuns.activeSessionIDs()`
  (distinct, in-flight oturumlar). Yeni uç **`GET /api/sessions/active`** →
  `{sessionIds:[]}` (`handleActiveSessions`, `server.go` route). `register`
  imzası `(id, sessionID, cancel)`. Ayrıca `chat_stream.go`'da **tur bitişinde
  hata yolunda da** terminal `chat` event'i yayan guard'lı defer (`turnStarted`
  +`emitted`) — önceden event yalnız başarı yolunda yayılıyordu, hata olunca
  gösterge takılı kalabilirdi.
- **Frontend:** `api.activeSessions()`; `useChatStream` `markPending(ids)` /
  `clearPending(sid)` (yalnız `pendingSessions` — `streaming` değil, böylece
  reload sonrası composer "Gönder"de kalır, kırık Durdur yok). `App.tsx`
  boot/workspace-değişiminde aktif turları seed'ler; her `chat` event'inde ilgili
  oturumu temizler (+ açık transcript'i yeniden yükler).

✅ go build/vet/test + tsc + vite build yeşil; geçici instance'ta
`GET /api/sessions/active` → `{"sessionIds":[]}` (200) smoke doğrulandı.
**Not (1):** çalışan 8090 backend eski binary — uç etkin olması için **backend
restart** gerekir (eski binary'de uç 404 döner, `api.activeSessions()` sessizce
yutar → regresyon yok). **Not (2):** çalışma ağacında eşzamanlı **attachments**
özelliği paylaşılan dosyalara (server.go/App.tsx/useChatStream.ts) iç içe
girdiğinden bu değişikliklerin commit'i kullanıcıya bırakıldı.

## UI — Açıklayıcı HTTP hata mesajları (2026-06-17)

Header'daki kırmızı hata pill'i (sol-üst) artık çıplak **"HTTP 502"** yerine
**eyleme dönük** mesaj gösterir. Kök neden: `api/client.ts` `req()` ve
`api/chat.ts` non-OK yanıtta gövdeyi JSON parse edip `{error}` arıyor;
**502/503/504** Vite dev-proxy'den gelir (Go backend ulaşılamıyor) ve gövde
JSON olmadığından çıplak `HTTP <status>` kalıyordu. **Çözüm:** `client.ts`'e
`describeHttpError(status)` (status→Türkçe açıklama; 502/503/504 → "Sunucuya
ulaşılamıyor… `go run ./cmd/tionswarm` çalışıyor mu", 500/404/401/403/400/408/429
özel) + `errorFromResponse(res)` (backend `{error}` öncelikli, yoksa status
açıklaması) yardımcıları eklendi; `req()` ayrıca **fetch reddini** (sunucu hiç
yanıt vermiyor) yakalayıp "Sunucuya bağlanılamadı…" döndürür. `chat.ts` aynı
yardımcıyı kullanır. ✅ tsc + vite build temiz; saf eşleme node ile doğrulandı.
(Canlı 502: çalışan backend'i durdurmamak için tetiklenmedi.)

## Ara özellik — Zamanlama formu: ikonlu ajan seçici + zorunlu prompt (2026-06-17)

**İstek:** "Ajan seçerken ikonunu da görelim, görev bağlama olmasın onun yerine doğrudan zorunlu prompt girelim."

- **İkonlu ajan seçici:** Native `<select>` avatar render edemediğinden yeni **`frontend/src/components/agents/AgentPicker.tsx`** özel açılır listesi eklendi. Tetik butonu + her seçenek, ajanın `AgentAvatar` dairesini (özel emoji ya da baş harf, `lib/avatar.ts` deterministik renk) adıyla gösterir; dışarı-tıkla-kapat (`rootRef` + `mousedown`). `Schedules.tsx` hem oluştur hem düzenle formunda bunu kullanır.
- **Görev bağlama kaldırıldı:** `taskId` select'i, `tasks` state'i ve `api.listTasks` çağrısı `Schedules.tsx`'ten silindi. **Prompt artık zorunlu** tek girdi (oluştur + düzenle doğrulaması "Prompt zorunlu"; eski "task veya prompt" mantığı gitti). Liste satırı her zaman `Prompt: …` gösterir.
- **Backend dokunulmadı:** `POST/PUT /api/schedules` prompt-only'i zaten kabul ediyordu; `taskId` API alanları geriye-dönük uyumluluk için korundu (eski schedule'lar bozulmaz).

✅ `tsc -b`/`vite build` yeşil. **Chrome canlı test** (`localhost:5173`): açılır listede 3 ajan renkli avatarlarıyla (Thinker/Reminder/StepTest) listelendi, Thinker seçilince trigger'da TH avatarı + ad gösterildi; formda görev alanı yok, prompt placeholder'ı "Prompt (zorunlu)".

## Ara özellik — Yeni workspace'te varsayılan deaktif zamanlama (2026-06-17)

**İstek:** "Yeni workspace oluşturunca default olarak saatte 1 çalışan ama deaktif bir zamanlama ekleyebilir misin? 'yarıda kalan taskları bildirim göndersin'."

`api/workspaces.go` `seedDefaultAgent` artık oluşturduğu ajanı döndürür ve yeni `seedDefaultSchedule`'ı çağırır: her yeni workspace'e **saatlik** (`0 * * * *`) ama **`Enabled:false`** bir zamanlama eklenir; prompt yarıda kalan/takılı görevleri özetleyip bildirim göndermeyi ister (`"Review the task board for unfinished or stuck tasks and send a notification summarizing them."`). Deaktif olduğundan cron tablosuna girmez, kendiliğinden ateşlenmez — kullanıcı Zamanlamalar ekranından toggle ile açtığında `Scheduler.Reload` çalışır.

✅ `go build`/`vet` yeşil. **API canlı test** (`127.0.0.1:8090`): `POST /api/workspaces` → yeni ws'de `GET /api/agents` = 1 (Asistan), `GET /api/schedules` = 1 (`cron=0 * * * *`, `enabled=false`, ajana bağlı, prompt doğru) → workspace silindi.

## Ara özellik — Zamanlama düzenleme (schedule edit) (2026-06-17)

**İstek:** "Zamanlamalar ekranında eklenen zamanlamaları liste halinde görebilelim, tıklayıp aktif/deaktif etme, silme veya editleme yapabilelim."

Liste + toggle (aktif/pasif) + sil zaten vardı (`components/panels/Schedules.tsx`); eksik olan tek şey **düzenleme** idi. Eklenenler:

- **Backend `db.UpdateSchedule`** (`store_schedule.go`): id ile bulup `AgentID`/`CronExpr`/`TaskID`/`Prompt` alanlarını günceller; `Enabled` + teslimat alanları (`LastDeliveryStatus`/`LastRunAt`/`NextRunAt`) **korunur** (yalnızca tanım düzenlenir).
- **`PUT /api/schedules/{id}`** (`api/schedules.go` `handleUpdateSchedule`): create ile aynı doğrulama (`agentId`+`cronExpr` zorunlu, `taskId` **veya** `prompt` gerekli, bilinmeyen ajan reddi). Başarıda `Scheduler.Reload(ctx)` çağırır (cron tablosu yeni ifadeyle yeniden yüklenir) ve güncel satırı döner. Route `server.go` `registerScheduleRoutes`'a eklendi.
- **Frontend `api.updateSchedule`** (`api/tasks.ts`) + `Schedules.tsx` **satır-içi düzenleme modu**: her satırda ✎ düğmesi → o satır ajan/preset/cron/görev/prompt formuna dönüşür (**Kaydet**/**İptal**). Kaydedince listedeki satır sunucudan dönen güncel veriyle değiştirilir; tek seferde bir zamanlama düzenlenir.

✅ `go build`/`vet` + `tsc -b`/`vite build` yeşil. **API canlı test** (yeni binary, `127.0.0.1:8090`): schedule create → `PUT` (cron `*/5 * * * *`→`0 9 * * *` ve prompt değişti, `enabled=true` **korundu**) → list ile diskte kalıcılık doğrulandı → delete ile temizlik. Not: gateway/mcp-chrome bu oturumda bağlı değil → tarayıcı görsel testi yapılmadı (API round-trip kanıt).

## Bugfix — claude-cli transcript'ine sızan harness markup'ı (system-reminder balonu) (2026-06-17)

**Belirti:** `claude-cli` sağlayıcısıyla çalışan bir ajan, çok-turlu bir sohbette araç kullandıktan (Edit/Read) sonra gelen bir kullanıcı isteğine cevap yerine `<system-reminder>The assistant message ... is malformed ...</system-reminder>` metnini **bir sohbet balonu olarak** üretiyordu.

**Kök neden:** `providers/serializeTranscript`, çok-turlu geçmişi düz metne çevirirken önceki asistan turunun `Text`'ini **olduğu gibi** CLI'ye geri besliyordu. Önceki turun metnine sızmış tool-call markup'ı (`</parameter></parameter></function_results>` gibi) içeren bir transcript, claude CLI'nin girdi-onarım harness'ini tetikliyor; o da `<system-reminder>` enjekte ediyor ve model bunu geri tükürüyor.

**Düzeltme (`internal/providers/sanitize.go` — yeni):**
1. **Girdi temizleme (kök neden):** `sanitizeTranscriptText` — `<system-reminder>…</system-reminder>`, `<function_calls|function_results>` blokları ve başıboş `invoke`/`parameter`/`function_*` etiketlerini regex ile strip eder; `serializeTranscript` artık her **asistan** turunu bununla geçirir (kullanıcı metnine dokunulmaz). Sıradan düzyazı/kod korunur (yalnız bu spesifik harness etiket adları hedeflenir).
2. **Çıktı koruması (savunma):** `cliStreamParser.finish()` artık `isRepairArtifact` ile çıktının bir onarım-reminder'ı olup olmadığını kontrol eder; öyleyse temizler, geriye gerçek cevap kalmazsa turu hata ile başarısız sayar (reminder asla kaydedilmez/gösterilmez).

**Testler:** `providers/sanitize_test.go` (`TestSanitizeTranscriptText_StripsHarnessMarkup`, `TestIsRepairArtifact`, `TestSerializeTranscript_SanitizesAssistantTurns`). ✅ `go build`/`vet`/`test ./internal/...` yeşil. Not: native (anthropic) yol yapısal content-block kullandığından bu sızıntıya açık değil; düzeltme claude-cli düz-metin transcript yoluna özgüdür.

## UI — Akış scroll fix + tool kartı başlık rozetleri (2026-06-17)

İki kullanıcı geri bildirimi (yalnız frontend, düşük risk):

1. **Bug — uzun sohbette canlı cevap "kayboluyor", tur bitince görünüyor.**
   Kök neden: `components/MessageList.tsx` her `messages` değişiminde (akışta her
   token bir delta → `setMessages`) `endRef.scrollIntoView({behavior:'smooth'})`
   çağırıyordu. Uzun transcript'te (büyük scroll yüksekliği) her delta bir önceki
   smooth animasyonu yarıda kesip yeniden başlatıyor → animasyon hiç oturmuyor,
   viewport canlı baloncuğu gösteremiyor; akış bitince son scroll oturunca cevap
   "gözüküyor". **Düzeltme:** sticky-bottom + **anlık** scroll. Scroll konteynerine
   `ref`+`onScroll` eklendi; `pinnedRef` kullanıcının dibe sabitli olup olmadığını
   izler (eşik 80px) → yukarı kaydırıp geçmiş okurken deltalar artık dibe çekmiyor.
   Pinned iken `el.scrollTop = el.scrollHeight` (smooth değil → delta thrash'i yok).
   Oturum değişiminde (ilk mesaj id'si değişince) yeniden dibe sabitlenir.

2. **Feature — tool kartı başlığında (collapse halinde) özet rozet.** Native yol
   zaten `diff` step → `DiffCard`'da `+eklendi/−silindi` gösteriyordu; **claude-cli**
   yolunda Read/Edit/Write `tool` step → `ActivityCard`'a gidiyor ve sayı yalnız
   kart açılınca (DiffView) görünüyordu. `ActivityCard` başlığına rozet eklendi:
   Edit/Write için çıktıyı `parseDiff` ile çözüp **`+X −Y`** (added>0||removed>0),
   Read/`read_file` için çıktı satır sayısı **`N satır`**. `lib/tools.ts`'e
   `toolBase`/`isReadTool` export'ları eklendi. ✅ tsc + vite build temiz.
   (Chrome canlı testi: gateway-manager/mcp-chrome bu oturumda bağlı değil — yapılamadı.)

## Faz P3 — İzin/onay katmanı (Aşama 1: claude-cli izin modu, 2026-06-16)

**Sorun (kök neden):** claude-cli ajanları **hiçbir dosya editleme işlemini yapamıyordu.**
TionSwarm `claude -p` (headless) ile shell-out yapıyor ama `--permission-mode` /
`--dangerously-skip-permissions` bayraklarını **hiç geçmiyordu**. Headless modda
varsayılan izin modu Edit/Write/Bash için onay ister; soracak arayüz olmadığından bu
araçlar **sessizce reddediliyordu**. (external-agent-oss'un 3-modlu izin sistemine bakıldı.)

**Aşama 1 — ajan-bazlı izin modu → CLI bayrağı eşlemesi:**
1. `db.Agent.PermissionMode` (`"read-only" | "ask" | "auto"`, boş = `auto`) — yeni ajanda
   varsayılan `auto`; mevcut ajanlar (alan boş) da `auto`'ya düşer → anında çözülür.
   `AgentProfilePatch.PermissionMode` ile kısmi güncelleme (`store.go`).
2. `providers.Request.PermissionMode` alanı; `agent/toolloop.go` `completeTraced` ajanın
   modunu `req.PermissionMode`'a basar (hem native hem CLI yolu için).
3. `providers/claudecli.go` `permissionModeArgs(mode)` → bayrak eşlemesi:
   `read-only`→`--permission-mode plan`, `ask`→`--permission-mode acceptEdits`,
   `auto`/boş/bilinmeyen→`--dangerously-skip-permissions`. `Complete`'te `--model`'den
   hemen sonra eklenir.
4. API: `createAgentReq`/`updateAgentReq`'e `permissionMode` alanı (`api/agents.go`).
5. Test: `providers/permission_test.go::TestPermissionModeArgs` (5 mod → bayrak eşlemesi).

✅ `go build`/`vet`/`test ./internal/...` yeşil. **Canlı uçtan-uca kanıt:** headless `claude -p`
ile temp dizinde dosya yazdırma — **bayrak yokken dosya OLUŞMADI** (eski hata doğrulandı),
**`--permission-mode acceptEdits` ile dosya OLUŞTU** (düzeltme doğrulandı).

**Aşama 2 — native (anthropic/minimax) yolda risk-sınıflı gate (2026-06-17):**
1. `tools/classify.go` (yeni) — `Risk` (`read`/`write`/`exec`) + `Classify(name)`. Bilinen
   built-in araçlar eşlenir; etkileşim/in-app araçlar (`ask_user`/`todo_write`/artifacts) =
   `read` (Explore ajanı da kullanabilsin); `write_file`/`edit_file` = `write`; `shell` = `exec`;
   **bilinmeyen + namespaced MCP araçları = `write`** (varsayılan, güvenli taraf).
2. `tools/ask.go` — `AskerFrom(ctx)` exported (agent paketinin ask kanalını onay için
   yeniden kullanması için).
3. `agent/permission.go` (yeni) — `permGate(ctx, mode, call, granted)`:
   `auto`→hepsi; `read-only`→yalnız `read`, write/exec **bloklanır**; `ask`→`read` serbest,
   write/exec **`WithAsker` ile onay sorar** (`Allow once`/`Always allow`/`Deny`; "Always allow"
   tur boyunca `granted`'da hatırlanır → tekrar sormaz). Asker yoksa (otonom koşu) write/exec
   reddedilir. `normalizePermAnswer` serbest-metin/tık yanıtını normalize eder.
4. `agent/toolloop.go` — native döngüde `reg.Call` **öncesi** `permGate`; bloklanan çağrı
   modele `IsError` tool result olarak döner (model uyum sağlar, tur düşmez) + `StepError`
   (`reason:"permission_denied"`) adımı yayılır. Tur-ömürlü `granted` haritası.
5. Testler: `agent/permission_test.go` (auto/read-only/ask×asker-yok/Always-allow-hatırlama/deny).

✅ `go build`/`vet`/`test ./internal/agent ./internal/tools ./internal/providers` yeşil. "ask"
onay istemi mevcut `StepAsk`→`AskPrompt` UI'ını yeniden kullanır (3 tıklanabilir seçenek);
mod şu an API ile (`permissionMode`) ayarlanır — composer mod seçici Aşama 3.

**Aşama 3 — UI (görsel kontrol + tur-bazlı override + varsayılan, 2026-06-17):**
1. **Ajan formu** (`AgentSettingsForm.tsx` + `types/agent.ts`): "İzin modu (araç kullanımı)"
   seçici (auto/ask/read-only) + açıklama → ajanın **kalıcı** modu (PUT `permissionMode`).
   "Nerede görürüm" sorusunun birincil cevabı.
2. **Composer** (`Composer.tsx` + `App.tsx` + `useChatStream.ts` + `api/chat.ts`): 🛡 tur-bazlı
   mod seçici + **Shift+Tab** döngü (external-agent-oss imzası); seçim localStorage'da kalıcı;
   `/api/chat(/stream)` gövdesine `permissionMode`. Backend `chat.go`+`chat_stream.go` tur başına
   ajanın yerel kopyasının `PermissionMode`'unu override eder (ThinkingLevel deseni).
3. **Settings** (`ProvidersPanel.tsx` + `types/settings.ts` + `internal/settings`): "Yeni ajan
   varsayılan izin modu" → `Settings.DefaultPermissionMode` (DTO/Patch/Default=`auto`/store);
   `api/agents.go` create önceliği: istek → uygulama varsayılanı → `auto`.

✅ `go build`/`vet`/`test` + frontend `tsc`/`vite build` yeşil. **Playwright canlı:** Ajan formu
İzin modu seçici+açıklama (Part A ✓), Composer 🛡 seçici (Part B ✓), kaydetme backend'e gitti.
Settings paneli tarayıcı aracı kararsızlığıyla bu turda görsel doğrulanamadı (kod/tsc doğrulandı).

**Aşama 4 — claude-cli gerçek onay + StepPermission kartı + oturum-ömürlü grants (2026-06-18):**
1. **CLI permission-prompt aracı** — claude-cli "ask" modunda artık `acceptEdits` (sessiz
   onay) yerine **gerçek tur-içi onay** sorar. `mcp_interaction.go` `callPermission`: CLI'nin
   `--permission-prompt-tool` sözleşmesini uygular (tool_name+input → risk sınıfla; read +
   "Always allow" otomatik izin; aksi halde `StepPermission` kartı yayıp `run.answer`'da
   bloklar) → `{"behavior":"allow","updatedInput":..}` / `{"behavior":"deny","message":..}`
   döner. `climcp.go` `permission_prompt`'u interaction araçlarına + `permissionPromptToolID`
   ekler; `promptToolForMode(mode,inter)` yalnız ask+endpoint varken döndürür. `claudecli.go`
   `ConfigureMCP`'ye `permissionPromptTool` param; set ise `--permission-prompt-tool` + default
   mod (yoksa eski `--permission-mode` eşlemesi). `toolloop.go` çağrıya geçirir. **Not:** flag
   v2.1.181'de `--help`'te gizli ama **çalışıyor** (ampirik doğrulandı, exit 0). `classify.go`'ya
   CLI yerleşik araç adları eklendi (Edit/Write/MultiEdit→write, Bash→exec, Read/Glob/Grep/LS/
   WebFetch→read) — CLI kendi tool adlarını gönderdiği için.
2. **Ayrı `StepPermission` kartı** — `agent/trace.go` yeni `permission` kind. Native gate
   (Aşama 2) artık `StepAsk` yerine bunu yayar: `tools/permission.go` `PermissionFunc` +
   `WithPermissionPrompter` köprüsü; `chat_stream.go` prompter'ı `StepPermission` (tool+risk+
   options) yayıp `run.answer`'da bekler. Frontend: `types` `permission` kind, `PendingAsk`'e
   kind/tool/risk, yeni `PermissionPrompt.tsx` (amber kart: araç+risk + İzin ver/Her zaman/
   Reddet, kırmızı Reddet), `useChatStream` `permission` adımını ayrı state'e koyar, `App.tsx`
   kind'a göre `PermissionPrompt` vs `AskPrompt`, `lib/stepKinds.ts` açıklaması.
3. **Oturum-ömürlü "Always allow"** — `tools/grants.go` `PermissionGrants` (sessiz, mutex,
   `WithGrants`/`GrantsFrom` köprüsü); `api/permgrants.go` `permGrantStore` (session→grants,
   Server alanı). Native gate (`agent/permission.go`) tur-ömürlü harita yerine `GrantsFrom(ctx)`
   kullanır; CLI `callPermission` `run.grantStore()` kullanır — **ikisi aynı per-session
   instance'ı paylaşır** (chat.go + chat_stream.go `WithGrants`; run'a `setGrants`). "Her zaman
   izin ver" oturum boyunca hatırlanır (sunucu restart'ında sıfırlanır). Ortak sabitler/normalize
   `tools/permission.go`'da (`PermissionOptions`, `NormalizePermission` → always/allow/deny;
   TR+EN kabul).

✅ `go build`/`vet`/`test ./internal/...` (tümü) + frontend `tsc`/`vite build` yeşil. `claude
--permission-prompt-tool` ampirik kabul testi geçti. **Kalan canlı test:** backend restart +
claude-cli ask-mode ajanla bir Edit tetikleyip onay kartını tıklamak (running backend eski binary;
restart kullanıcıya bırakıldı). **Faz P3 (4 aşama) TAMAMLANDI.**

## Kararlar (2026-06-15)
- **İlk LLM sağlayıcısı:** Anthropic (Claude) ✅
- **Frontend:** React ✅

## Mevcut Durum: Faz 0–8 + Faz 7 + SDK Paritesi P2/P1 TAMAMLANDI ✅ → Backlog (Wails dahil)

> Not: Faz 8 (MCP/Tools) kullanıcı talebiyle Faz 7'den önce yapıldı; ardından Faz 7 tamamlandı.
> Sonrasında **SDK Paritesi Faz P2** (builtin fs/shell araçları) + **Faz P1** (todo_write/ask_user) + Trace `StepKind` genişletme (ask/todo/recovery) ve çok sayıda ara özellik (streaming, MiniMax, workspace switcher, otonom olay akışı) tamamlandı.
> Kalan sıra: **SDK Paritesi P3/P4 · Faz 9 Wails** ve diğer backlog kalemleri — bkz. [03-YOL-HARITASI.md](03-YOL-HARITASI.md) "Yapılacaklar / Backlog". (Connectors fazı 2026-06-16'da kapsamdan çıkarıldı.)

### `/reflect` düzeltmesi + `/compact` komutu ✅ (2026-06-16)
İki sohbet "/" komutu eklendi/düzeltildi (backend `internal/api/summary.go` + `internal/conversation/manager.go`):
1. **`/reflect` (düzeltme):** Önceden `POST /api/agents/{id}/reflect` çağırıp **hiçbir görsel sonuç** vermiyordu (kullanıcı "çalışmıyor" dedi). Artık `handleSessionSummary` `reflect` kind'ını işliyor → `Runtime.Reflect` (dream cycle) sonucu **sohbete asistan mesajı** olarak yazılıyor ("✦ Yansıma" başlığıyla). `/memory` vb. ile aynı UX.
2. **`/compact` (yeni):** Sohbeti **şimdi** sıkıştırır. `conversation.Manager.ForceCompact` token bütçesine bakmadan en eski mesajları (son `keepRecent` hariç) yuvarlanan özete katlar, kalıcılaştırır ve "N mesaj katlandı + güncel özet" raporu döner ("🗜 Sohbet sıkıştırma").
3. **Frontend:** `App.tsx` `summarize(kind)` artık serbest string alır + kind'a göre placeholder ("Yansıma üretiliyor…"/"Sıkıştırılıyor…"); `chatCommands`'a `/reflect` (düzeltildi) + `/compact` (yeni) eklendi.

> Test: go build yeşil; dev binary yeniden başlatıldı; **API canlı**: 64-mesajlı oturumda `/compact` → 56 mesaj katlandı; `/reflect` → dream-cycle yansıması döndü; Chrome: "/" menüsünde `/reflect`+`/compact` üstte listelendi. **Not:** Composer'daki bekleyen-kuyruğu (queue/steer tray, silinebilir) eşzamanlı bir çalışmayla birlikte aynı turda eklendi.

### Bugfix — HTTP access-log middleware (Loglar ekranı canlı) ✅ (2026-06-16)
**Sorun:** Kullanıcı "loglar ekranı bayadır kullanıyorum ama yeni log oluşmuyor" dedi. Ekran ve `GET /api/logs` doğru çalışıyordu (HTTP 200 + gerçek kayıtlar), ama yalnızca açılış logları + birkaç "agent reflected" görünüyordu. **Kök neden:** kod sadece **hata** durumlarında (`s.logger.Error/Warn`) ve birkaç otonom olayda slog kaydı üretiyordu; başarılı istek/sohbet/görev hiçbir log basmıyordu → aktif kullanımda yeni kayıt düşmüyordu (beklenen davranış, eksik enstrümantasyon).

**Çözüm:** `internal/api/middleware_log.go` — `withRequestLog` her API isteğini loglar (`"http request"`, attrs: `method`/`path`/`status`/`dur`); 2xx→INFO, 4xx→WARN, 5xx→ERROR. `statusRecorder` durum kodunu yakalar **ve** `Flush()`+`Unwrap()` ile `http.Flusher`'ı yeniden açar (SSE `chat/stream`+`events` `w.(http.Flusher)` assertion'ı sarmalayıcıdan geçsin diye — yoksa akış kırılırdı). `skipRequestLog` `/api/logs` (kendi buffer'ını sel etmesin) + `/api/events` (uzun-ömürlü SSE) + `/health`'i atlar. `Routes()` zinciri: `withCORS → withRequestLog → withWorkspace` (OPTIONS preflight CORS'ta erken döndüğünden loglanmaz). ✅ (`go build`/`vet` yeşil; **canlı test**: dev binary yeniden başlatıldı, `/api/agents`→INFO 200, `/api/sessions`→INFO 200, `/api/nonexistent`→WARN 404, `/api/logs` kendini loglamadı).

**Ek (aynı gün): İş-seviyesi INFO loglar.** Access-log "hangi uç çağrıldı"yı verir; üstüne "ne oldu" anlamı eklendi: `chat turn completed` (`chat.go`+`chat_stream.go` — agent/provider/model/in-out token/steps/süre, stream yolunda `stream:true`), `agent created/updated/deleted` (`agents.go`), `session created/deleted` (`sessions.go`), `task created` (`tasks.go`), `schedule created/toggled` (`schedules.go`), `mcp server toggled/tested` (`mcp.go`; başarısız test→WARN), `memory added` (`memory.go`). (Zaten vardı: heartbeat `agent tick` `worker.go`, `task run finished` `executor.go`, `flow run started/finished` `flow.go`, `agent reflected` `reflector.go`.) ✅ canlı test: agent+session create/delete dört olay da loglandı.

### Interaction MCP — Faz 2 (request_confirmation + artifacts + CLI kart paritesi) ✅ (2026-06-16)
Faz 1 üstüne araç seti genişletildi ve CLI iz paritesi sağlandı. Detay: [11-INTERACTION-MCP.md](11-INTERACTION-MCP.md) (§16).

1. **`request_confirmation`** (yeni `internal/tools/builtin_confirm.go`): riskli/geri-dönülemez eylem öncesi evet/hayır onayı; cevap `confirmed`/`denied` normalize (`NormalizeConfirmation`, TR+EN). Native registry'ye de eklendi (`toolsetup.go`); `ask_user` bloklama deseni (ortak `blockForAnswer`).
2. **`create_artifact`/`update_artifact` CLI yolunda:** `chatRun`'a per-agent **artifact sink** (`setArtifacts`, chat_stream her ajan turunda kurar); interaction backend sink'i context'e koyup mevcut tool'u çağırır (tek-kaynak).
3. **CLI iz paritesi (`agent/trace.go`):** `traceStepToTurnStep` interaction namespace'ini (`mcp__tionswarm_interaction__`) **soyar** (kartlar bare adla eşleşir) + `todo_write`'ı **`StepTodo`** kartına yükseltir → CLI yolunda da TodoCard/ArtifactCard kalıcı izde doğru render (çift kart yok, canlı emit yerine izden).
4. **Wiring:** `climcp.go` interaction allow-listesi 5 araca (`interactionToolNames`); `Tools()` + backend dispatch eşitlendi. **Keep-alive gereksiz çıktı:** claude `MCP_TOOL_TIMEOUT` varsayılanı ~28 saat → bloklayan araç pratikte timeout olmaz (+15 dk sunucu backstop).

> Doğrulama: `go build`/`vet`/`test` yeşil (yeni: confirm normalize, artifact dispatch, todo no-emit). **Canlı claude-cli testi:** todo_write→request_confirmation→(onay "Onayla")→create_artifact promptu → SSE'de TodoCard `[in_progress,pending]`, ASK `[Onayla,İptal]` → `request_confirmation→confirmed` → `create_artifact→{id}` → TodoCard `[completed,completed]` → "DONE"; `GET /api/artifacts` → `('Faz2 Test','markdown',1)` kalıcı; tool adları izde namespace'siz. **Sıradaki: Faz 3** (notify) + **Faz 4** (Codex/Gemini/Mistral Vibe adaptör — ilgili CLI'lar kurulmalı). **Not:** eşzamanlı frontend (PendingTray) + `withRequestLog` başka oturumda; commit yalnız backend Faz 2 dosyaları.

### Interaction MCP — Faz 1 (ask_user + todo_write claude-cli'ye) ✅ (2026-06-16)
Kendi agentic döngüsünü süren CLI ajanlarının (ilk hedef claude-cli) TionSwarm'nun insan-etkileşimli araçlarını kullanıp TionSwarm UI'ında yüzeyleyebilmesi için **in-process MCP-over-HTTP** sunucusu eklendi. Önceden claude-cli `-p` modunda kendi `AskUserQuestion`'ını cevaplayamıyordu → "soru penceresi hiç çıkmıyordu". Tasarım+detay: [11-INTERACTION-MCP.md](11-INTERACTION-MCP.md) (§15).

1. **Yeni `internal/interaction`** (saf protokol): MCP-over-HTTP server (`initialize`/`tools/list`/`tools/call`), bearer auth, protokol `2025-06-18`; `Backend` arayüzü token→run çözer. Birim testli.
2. **`internal/tools/interaction.go`:** `WithInteractionEndpoint`/`InteractionFrom` context köprüsü (WithAsker deseni).
3. **`internal/api/mcp_interaction.go`:** `interactionBackend` — `ask_user` (bloklayan, `run.answer` bekler) + `todo_write` (bloklamayan, canlı kart) dispatch; tool şemaları `tools` paketi tanımlarından **tek-kaynak** enumerate. `/mcp/interaction` route + `Server.SetBaseURL` (loopback self-URL). `chatRun`'a per-run **bearer token** + mutex'li `emit` (handler + MCP goroutine yarışmaz) + `done` kanalı; `byToken` lookup.
4. **CLI wiring:** `climcp.go` interaction entry'sini (`type:http`, `headers: Bearer`) **MCP kapalı olsa bile** yazar; `toolloop.go` claude-cli'yi interaction varsa MCP-delegasyon yoluna sokar; `claudecli.go` `--disallowedTools AskUserQuestion TodoWrite` + sistem-prompt notu.

> Doğrulama: `go build`/`vet`/`test` yeşil (interaction + api birim testleri: bearer reddi, initialize/list/call, ask round-trip, turn-ended, todo). **Canlı claude-cli testi** (MCP kapalı yeni ajan, ayrı port/data-dir): "önce ask_user ile renk sor" → SSE'de `ask` adımı (`[RED,BLUE,GREEN]`) → `POST /api/chat/control {answer:"BLUE"}` → CLI sonucu `BLUE` aldı → final "Your color is BLUE." Backend log: `cli mcp config written servers=1 interaction=true`. Faz 0 spike (handshake) önce ayrıca doğrulanmıştı. **Not:** eşzamanlı frontend çalışması (PendingTray vb.) başka oturumda; commit yalnız backend Faz 1 dosyalarını kapsar.

### Komut çalıştırınca komut balonu + bağlam penceresi göstergesi ✅ (2026-06-16)
1. **Komut balonu** (`api/summary.go` + `App.tsx`): `/memory`·`/board`·`/flows`·`/tools` paletten çalışınca, komutun kendisi de sohbete **kullanıcı mesajı** (`/kind`, `UserBubble` komut stilinde) olarak yazılır; ardından sonuç asistan mesajı gelir. `POST /api/sessions/{id}/summary` artık `{userMessage, replyMessage}` döndürür (ikisi de kalıcı → reload-safe). Composer her iki balonu da iyimser (optimistic) gösterir.
2. **Bağlam penceresi göstergesi** (`api/session_info.go` + `SessionDetailPanel.tsx`): `/info` artık `contextWindow` (= `maxContextTokens` sıkıştırma eşiği) döndürür. Oturum bilgisi paneline `/context` tarzı bir bölüm eklendi: kullanılan/pencere (%) başlığı + kategori-renkli **yığılmış kullanım barı** + her kategori (token+%) + **boş alan** satırı; pencere aşılırsa sıkıştırma uyarısı.

> Doğrulama: go/tsc build + API (`/summary` `{userMessage,replyMessage}`, `/info` `contextWindow=32000`) + Chrome canlı (`/board` → "⌘/board" komut balonu + özet; panel `2.5k/32.0k %8`, boş alan %92). Commit: `d03483d` + worker sweep `7f432be`.

### Composer'da tur-bazlı düşünme seviyesi seçici ✅ (2026-06-16)
Mesaj gönderme alanına, o tur için **düşünme (reasoning) seviyesini** seçtiren bir menü eklendi.
1. **UI (`Composer.tsx`):** Textarea'nın solunda `🧠` butonu (mevcut seviyeyi gösterir, seçiliyse accent). Tıklayınca üstte açılan menü: **Oto** (ajanın kendi ayarı), **Kapalı**, **Düşük**, **Orta**, **Yüksek**. Dışarı tıkla-kapat. Seçim `App.tsx`'te `thinkingLevel` state'inde tutulur ve **localStorage**'a yazılır (mesajlar/yenileme arası kalıcı). Props: `thinkingLevel` + `onThinkingLevelChange`.
2. **Akış (`api.ts`):** `streamChat`/`chatStream` artık `thinkingLevel` parametresi alıp `/api/chat/stream` gövdesine ekler.
3. **Backend (`chat.go` + `chat_stream.go`):** `chatReq.ThinkingLevel` (`"low"|"medium"|"high"|"off"`, boş = ajan ayarı). Handler, yanıtlayan ajanın **yerel kopyasının** `ThinkingLevel`'ini tur başına override eder (kalıcı değil) → mevcut `thinkingBudgetForLevel` → `req.ThinkingBudget`. Yalnız **anthropic araçsız yolda** etki eder; claude-cli/minimax `ThinkingBudget`'i yok sayar.

> Test: tsc + go build yeşil; **Chrome canlı**: seçici açıldı, "Yüksek" seçildi (accent aktif), yenileme sonrası korundu (localStorage), mesaj hatasız gönderildi. Not: backend override'ı etkin olması için sunucu yeniden başlatılmalı; görsel düşünme etkisi yalnız anthropic anahtarlı ajanlarda görülür.

### Sohbette mesaj zamanı + agent çalışma süresi ✅ (2026-06-16)
Sohbet ekranında her mesajın **gönderilme saati** ve her asistan turunun **çalışma süresi** gösterilir.
1. **Zaman yardımcıları (`lib/time.ts`):** `clockTime` ("22:48"), `fullDateTime` (hover title), `formatDuration` ("45 sn" / "2 dk 15 sn" / "1 sa 5 dk").
2. **Meta bileşenleri (`components/chat/MessageMeta.tsx`, yeni):** `MessageTime` (mesaj saati + tam tarih hover), `TurnDuration` (tamamlanan tur süresi "⏱ 2 dk 15 sn"), `LiveTimer` (akış sürerken her saniye tıklayan canlı sayaç "⏱ 0:45").
3. **MessageList bağlama (`MessageList.tsx`):** Her mesajın altına rolüne göre hizalı meta satırı. Asistan çalışma süresi ≈ `mesaj.createdAt (tur sonu) − önceki mesaj.createdAt (tur başı)`; **yalnız önceki mesaj kullanıcıysa** gösterilir (enjekte edilen /summary özetleri ve ardışık asistan turları yanıltıcı boşta-süre vermesin). Akıştaki son asistan balonunda `LiveTimer` (createdAt = tur başı), tamamlanınca `TurnDuration`. Yeni prop `streaming` (`App.tsx`'ten `streaming && streamingSessionId === activeSessionId`).

> Test: tsc temiz; **Chrome canlı**: mevcut oturumda tüm mesajlarda saat + doğru süreler (⏱ 6 sn…44 sn); enjekte özet mesajlarında süre gizlendi; yeni turda canlı sayaç 7 sn→31 sn tıkladı, bitince ⏱ 10 sn — doğrulandı.

### Oturum-bazlı akış göstergesi + buton kapsamı ✅ (2026-06-16)
Sohbet akışı (streaming) durumu artık **oturuma bağlı** — önceden tek global `streaming` bayrağı tüm oturumları etkiliyordu.
1. **Buton kapsamı fix'i (`App.tsx`):** Yeni `streamingSessionId` state'i akışın hangi oturuma ait olduğunu izler. Composer'a geçen prop `streaming && streamingSessionId === activeSessionId` oldu; `AskPrompt` de aynı koşula bağlandı. Böylece A oturumunda yanıt üretilirken B oturumuna geçince buton yanlışça "Durdur/Kes/Yönlendir" yerine doğru şekilde **"Gönder"** gösterir. (Akış başlangıcında set, `finally`/`stopTurn`'de temizlenir.)
2. **Sidebar canlı göstergesi (`SessionsSidebar.tsx`):** Akışı süren oturum satırında **nabız atan nokta** (`animate-ping`) + alt satırda **"yazıyor…"** etiketi gösterilir (zaman/mesaj sayısı yerine); başlık kalınlaşır. Gösterge yalnız ilgili oturumda görünür, oturum değiştirince akıştaki oturumda kalır. Prop: `streamingSessionId={streaming ? streamingSessionId : null}`.
3. **Renk iyileştirmesi (sonradan, aynı gün):** Gösterge daha görünür olsun diye nokta + "yazıyor…" etiketi koyu mor `--color-accent` yerine **parlak açık yeşil `emerald-400`** yapıldı; etiketteki `opacity-60` soluklaştırması bu durumda kaldırıldı.

> Test: tsc + go build/vet yeşil; **Chrome canlı**: A oturumunda akış → satırda "yazıyor…" + ping nokta (emerald-400); B'ye geçince buton "Gönder", A bitince gösterge kayboldu — doğrulandı.

### Komutlar ekranında salt-okunur prompt görüntüleyici ✅ (2026-06-16)
Ayarlar → **Komutlar** kategorisine, komutların arkasındaki gömülü promptları **salt-okunur** gösteren bir bölüm + **klasörü aç** butonu eklendi:

1. **Backend** (`internal/agent/prompts.go`): `Prompts()` summary/reflect/title promptlarını (system + user-turn şablonu + not + kaynak dosya adı) döndürür; `PromptsDir()` `runtime.Caller` ile prompt kaynak klasörünü (yerel derlemede `internal/agent`) verir. `GET /api/prompts` (`internal/api/prompts.go`) bunları + `dir`'i döner; `POST /api/prompts/reveal` klasörü Explorer'da açar (session-reveal ile aynı `explorer.exe` deseni). Route'lar `registerSettingsRoutes`'ta.
2. **Frontend** (`SettingsPanel.tsx`): Komutlar kategorisi eski komut-kartı görünümünü **korur**; her kart artık **açılır-kapanır** (▸/▾). Açılınca o komuta ait prompt (System/User turn `<pre>` blokları, salt-okunur), kaynak dosya rozeti ve **📂 Klasörü aç** butonu (`api.revealPrompts()`) kartın içinde görünür. `/tools` deterministik olduğundan "prompt yok" notu gösterir. Auto-title komut olmadığından en altta ayrı (kesik çizgili) açılır kart olarak durur. Ortak `PromptDetails` bileşeni. Düzenleme yok — promptlar binary'ye gömülü.

> Doğrulama: `go build ./...` + `tsc` yeşil; `GET /api/prompts` 3 prompt + doğru `dir` döndü; Chrome canlı: Komutlar ekranında promptlar + klasör yolu + "Klasörü aç" butonu render, butona basınca backend hatasız Explorer açtı.

### Extended thinking native parite (streaming + Complete) ✅ (2026-06-16)
Extended reasoning artık **native (anthropic) yolda** da uçtan uca yüzeyleniyor — önceden yalnız claude-cli `thinking` bloğunu dolduruyordu; native `Stream` sadece `text_delta` ayrıştırdığından düşünme kayboluyordu.

1. **`providers/provider.go`:** `Streamer` callback'i `func(string)` → tipli `func(StreamDelta)` (yeni `StreamDelta{Kind,Text}` + `DeltaText`/`DeltaThinking` sabitleri).
2. **`providers/anthropic.go`:** `Stream` SSE `content_block_delta`'da artık `text_delta` **ve** `thinking_delta` parse eder, tipli delta ile yayar; tam thinking metni `Response.Trace`'e `thinking` adımı olarak konur. `Complete` da `thinking` content-block'unu `Response.Trace`'e parse eder.
3. **`providers/minimax.go`:** `Stream` yeni imzaya uyduruldu (thinking yok → hep `DeltaText`).
4. **`agent/toolloop.go`:** `recordedStream` thinking'i sabit `liveThinkingID` ile canlı `StepThinking` akıtır; stream yolu artık `traceToSteps(resp.Trace)` döndürür → thinking **kalıcılaşır** (önce `nil` dönüp reload'da kayboluyordu).
5. **`frontend/src/App.tsx`:** canlı thinking delta'larını `id`'ye göre tek büyüyen `ThinkingBlock`'a merge eder (tool_delta deseni).
6. **Test (yeni):** `providers/stream_test.go` → `TestAnthropic_StreamSurfacesThinking` (thinking+text delta ayrımı + `Response.Trace` kalıcılığı); mevcut stream testleri yeni imzaya güncellendi.

Bütçe ajanın `ThinkingLevel`'inden (`thinkingBudgetForLevel`: low=2048/medium=8192/high=16384) yalnız **araçsız (MCP kapalı)** turlarda gönderilir. Native **tool** döngüsünde thinking hâlâ kapalı (imzalı blok geri-besleme gerektirir). ✅ `go build`/`vet`/`test ./...` + frontend `tsc --noEmit` temiz. Detay: [07-CHAT-UX.md](07-CHAT-UX.md).

### D2 — Provider retry middleware + C1 — Sistem-prompt cache sınırı ✅ (2026-06-16)

İki backlog maddesi tamamlandı: geçici hatalara dayanıklı sağlayıcı çağrıları (D2) ve
prompt-cache'i etkili kılan statik/dinamik sistem-prompt bölümlemesi (C1).

**D2 — Retry middleware (`internal/providers/transport.go`)**
- [x] `retryPolicy` (varsayılan 4 deneme, 500ms taban, 8s tavan) + `httpRetry` paket var'ı
  (testler hızlı/no-retry politikayla değiştirir).
- [x] `doWithRetry(ctx, client, prefix, build)` — her denemede isteği yeniden kurar (gövde
  tüketildiği için), **üstel backoff + ±%25 jitter**, `Retry-After` başlığına saygı,
  context iptaline duyarlı. Son denemede yanıtı (retryable status olsa bile) ya da sarılmış
  ağ hatasını **olduğu gibi** döndürür → çağıran sağlayıcının kendi mesajını/gövdesini sunar.
- [x] Retryable: ağ hatası + `408/429/500/502/503/504/529` (529 = Anthropic "overloaded").
- [x] **Retry görünürlüğü (2026-06-16):** her retry öncesi `slog.Warn("provider transport retry", provider/attempt/maxAttempts/reason/backoff)`; `main.go` artık `slog.SetDefault(logger)` çağırdığından bu uyarılar logbuf ring buffer'ına düşer → **Loglar ekranında** (`/api/logs`) görünür. Canlı doğrulandı: MiniMax flaky sunucuya (503→503→200) yönlendirildi, sohbet başarıyla döndü, loglarda `attempt:1 backoff:589ms` + `attempt:2 backoff:801ms reason:HTTP 503` yakalandı.
- [x] `postJSON` **ve** `postSSE` artık `doWithRetry`'den geçer (SSE yalnız ilk bağlantıyı
  yeniden dener → akış başladıktan sonra kısmi çıktı tekrarlanmaz). İmzalar değişmedi →
  anthropic/minimax çağrıları dokunulmadan retry kazandı.

**C1 — Statik/dinamik sistem-prompt bölümleme**
- [x] `providers.Request`: `System` (STATİK prefix: persona + kullanıcı profili — turlar arası
  sabit) + yeni `SystemDynamic` (VOLATİL suffix: recall edilen bellek + konuşma özeti — her tur değişir).
- [x] `anthropic.go` `systemField(static, dynamic)`: extended-cache açıkken `cache_control`
  breakpoint'i **yalnız statik blokta** (1h TTL) → araç tanımları + statik prefix cache'lenir,
  her tur değişen dinamik suffix cache'i geçersiz kılmaz. Cache kapalıyken düz birleştirme.
  (Statik boşsa breakpoint dinamiğe düşer → eski tek-blok davranışı korunur.)
- [x] `minimax.go` + `claudecli.go`: cache breakpoint'i olmadığından `System`+`SystemDynamic`
  birleştirilir. **Önemli:** claude-cli `--append-system-prompt` artık her ikisini birleştirir
  → belleğin anahtarsız yolda düşmesi (regresyon) engellendi.
- [x] Çağıranlar (`api/chat.go`, `chat_stream.go`, `agent/executor.go`, `flow.go`): bellek + özet
  artık `System`'e değil `SystemDynamic`'e konur; persona + profil statik kalır. `Runtime.complete`
  imzası `(system, systemDynamic, prompt, ...)` oldu.

**CANLI TEST (unit + API):**
- [x] `transport_test.go`: 503→503→200 retry başarısı (3 çağrı), kalıcı 503'te denemeler bitince
  son yanıt+gövde döner, non-retryable 400 hiç denenmez, `retryableStatus`/`parseRetryAfter` tablo.
- [x] `anthropic_test.go`: cache breakpoint sadece statik blokta; statik-yok→dinamik cache'lenir;
  cache kapalı→düz birleşim; boş→nil.
- [x] **Canlı API (claude-cli, anahtarsız):** ajana "BLUEFALCON" hafıza belgesi eklendi →
  "What is the secret project codename?" sorusuna **"BLUEFALCON"** yanıtı → recall'ın `SystemDynamic`
  üzerinden claude-cli'a ulaştığı uçtan uca doğrulandı (C1 anahtarsız yolu bozmuyor).
- [x] `go build/vet/test ./...` temiz.

> Not: 1M-context + uzatılmış cache yalnız **anthropic**'te etkili; retry üç sağlayıcıyı da kapsar
> (ortak transport). claude-cli kendi HTTP'sini yaptığından retry yalnız native (anthropic/minimax)
> çağrıları + MCP listelemeyi etkiler.

### Talep-üzerine özetler + "/" komut paleti ✅ (2026-06-16)

Sohbet composer'ındaki `/` komut paletine **workspace verisini özetleyen** dört komut
eklendi: ajan hafızası, görev panosu, akışlar ve araç listesi. Sonuç, oturuma normal bir
asistan mesajı olarak (başlıkla) yazılır.

**Backend**
- [x] `agent/summarizer.go` (yeni): `Runtime.Summarize(ctx, agentID, kind)` — `memory`/`board`/
  `flows` türleri ilgili veriyi toplar (`gatherSummaryData`, her tür max **40 kayıt**, kayıt
  başına 200 rune cap) ve **ucuz bir modele** özetletir (yapılandırılmışsa başlık-modeli override'ı,
  yoksa ajanın kendi modeli; `guardedComplete` ile bütçeye dahil). `tools` türü **deterministik**
  (`toolsOverview`, model çağrısı yok) — ajanın efektif araç kataloğunu açıklamalarıyla listeler.
- [x] `api/summary.go` (yeni): `POST /api/sessions/{id}/summary` (`{kind}`) → `Summarize` çağırır,
  türe özel başlık ekler (🧠 Hafıza / 🗂 Görev panosu / 🔀 Akışlar / 🔌 Araçlar), sonucu
  `AddMessage` ile oturuma yazıp mesajı döner. Bilinmeyen tür → 400.
- [x] `server.go`: `POST /api/sessions/{id}/summary` route'u kaydedildi.

**Frontend**
- [x] `App.tsx` `summarize(kind)` callback'i: backend'i çağırır, üretim sırasında geçici placeholder
  gösterir, dönen asistan mesajını sohbete ekler. `/` komut paletine 4 komut (`api.ts`/`types.ts`).

**CANLI TEST (API):**
- [x] `tools` türü: ajanın efektif araç kataloğu deterministik markdown listesi olarak döndü (model
  çağrısı yok); MCP kapalı ajanda "araçlar kapalı" mesajı.
- [x] `memory`/`board`/`flows`: boş veride "_(boş — özetlenecek … yok)_"; dolu veride ucuz model özeti.
- [x] `go build/vet/test ./...` + frontend `tsc --noEmit` temiz.

### İki-seviyeli araç yönetimi (workspace + ajan) ✅ (2026-06-16)

Araç (tool) yönetimi ajan-bazlıdan **iki seviyeli** bir modele çevrildi: workspace
geneli aktivasyon + ajan-bazlı seçim.

**Model**
- **Workspace seviyesi**: `db.WorkspaceToolConfig{DisabledTools []string}` — store kökünde
  `tools-config.json` singleton (denylist; listelenmeyen araç = aktif → yeni MCP araçları
  otomatik aktif gelir). `store_tools.go` (`GetWorkspaceToolConfig`/`SetWorkspaceToolConfig`/
  `loadToolConfig`), `db.go` `toolConfig` alanı + load.
- **Ajan seviyesi**: mevcut `allowed_tools` allowlist korundu (boş = tüm **workspace-aktif** araçlar).

**Backend**
- [x] `agent/toolsetup.go`: `workspaceDisabledSet` + `toolFilter(ctx,agent)` (workspace denylist ∩
  ajan allowlist birleşik predicate). Katalog metodları: `WorkspaceToolCatalog` (tümü, ajan-agnostik),
  `ActiveToolCatalog` (workspace-aktif = ajan seçeneği), `ToolCatalog(agent)` (efektif). `toolloop.go`
  `req.Tools = reg.Defs(r.toolFilter(ctx,agent))`.
- [x] `api/workspace_tools.go` (yeni): `GET /api/workspace-tools` (tüm katalog + `enabled` bayrağı +
  `disabledTools`), `PUT /api/workspace-tools` (`disabledTools` denylist'i yaz). `agent_tools.go`
  katalog kaynağı `ActiveToolCatalog` (ajanın seçebileceği = workspace-aktif).

**Frontend**
- [x] `ToolsPanel.tsx` artık **workspace-geneli**: tüm araçlar aç/kapat toggle'larıyla (anında kaydeder)
  + MCP sunucu yönetimi; ajan bölümü kaldırıldı. App'te `<ToolsPanel onError>` (ajan prop'u yok).
- [x] `AgentToolsSection.tsx` (yeni): ajanın kullanacağı araçları workspace-aktif kataloğundan seçer
  (master "araç kullan" + per-tool checkbox + "Hepsi"/"Hiçbiri"; anında kaydeder). `AgentSettingsForm`'a
  gömüldü → hem Ajanlar ekranı sağ paneli hem roster ⚙ modalı paylaşır.
- [x] `types.ts`/`api.ts`: `WorkspaceTool`/`WorkspaceTools` + `workspaceTools()`/`setWorkspaceTools()`.

**CANLI TEST (API + unit):**
- [x] `GET /api/workspace-tools`: 14 araç (shell dahil, `TIONSWARM_ENABLE_SHELL=1`); `PUT` ile shell+http_get
  deaktif → re-fetch `enabled` bayrakları doğru; `store/tools-config.json` diske yazıldı.
- [x] Ajan kataloğu workspace-aktif **12 araç** döndü (deaktif shell/http_get hariç); alt küme allowlist
  (`read_file,grep,get_current_time`) kaydedildi.
- [x] Unit (`tooltier_test.go`): workspace denylist tam kataloğu etkilemeden aktif kataloğu filtreler;
  ajan efektif seti = workspace-aktif ∩ ajan allowlist (workspace-deaktif araç allowlist'te olsa bile düşer).
- [x] `go build/vet/test ./...` + frontend `tsc --noEmit` temiz.

> Not: claude-cli yolu sunucu-bazlı `--allowedTools` ile kendi döngüsünü sürdüğünden per-tool filtre
> native (anthropic/minimax) yola + katalog/MCP listesine uygulanır (SDK parite deseni). Tarayıcı DOM
> testi sıradaki doğrulama adımı (API + unit ile çekirdek mantık doğrulandı).

### Faz A1.1 — Artifact: manuel düzenleme + sohbet bağlamına enjeksiyon ✅ (2026-06-16)

Artifact sistemine iki ek yetenek:

1. **Panelden manuel düzenleme / yeni sürüm / yeni oluşturma** (`ArtifactsPanel.tsx`): görüntüleyici header'ında **✎ Düzenle** → içerik `textarea` + başlık/tür/dil alanları + değişiklik notu, **Kaydet/İptal**. İçerik değişince `PUT /api/artifacts/{id} {content,note}` yeni **sürüm** açar (eski sürüm `revisions`'a arşivlenir); yalnız başlık/tür değişince meta güncellenir, **sürüm açılmaz** (seçici patch). Liste header'ında **+ Yeni** → boş artifact oluşturup doğrudan düzenleme moduna girer.
2. **Sohbet bağlamına artifact enjeksiyonu** (`api/artifacts.go` `artifactsContextBlock` → `chat.go` + `chat_stream.go` `dynamic` suffix): oturumdaki mevcut artifactlar (id/başlık/tür/sürüm) sistem promptunun dinamik (cache'siz) kısmına "## Artifacts in this session" bloğu olarak eklenir → ajan `update_artifact` ile **id üzerinden** revize eder, kopya oluşturmaz. Boş oturumda blok yok; session-scoped (başka oturumun artifactları sızmaz).

**CANLI TEST (API + unit):**
- [x] UI API yolu: Yeni→v1(boş içerik), içerik düzenle→**v2**+1 revizyon, yalnız-başlık→**v2 kalır** (sürüm açmadı), delete=200.
- [x] `artifacts_test.go::TestArtifactsContextBlock`: blok id/başlık/`update_artifact` içerir, boş oturumda boş, başka oturumu sızdırmaz — **PASS**.
- [x] `go build/vet/test ./...` + frontend `tsc --noEmit` + `vite build` temiz.

> Not: Chrome/Playwright canlı UI testi bu turda yapılamadı (mcp-gateway bağlantısı kopuktu); doğrulama UI'ın yaptığı **tam API çağrıları** + Go unit testi + temiz derleme üzerinden yapıldı. UI bileşeni tsc/build'den temiz geçiyor.

### Faz A1 — Artifact sistemi ✅ (2026-06-16)

Ajanların ürettiği önemli, bağımsız içerik (doküman/kod/HTML/SVG/Mermaid) **versiyonlanarak** saklanır ve ayrı bir ekranda görüntülenir — Claude.ai artifacts benzeri.

**Backend**
- [x] **Depolama** (`internal/db`): `models_artifact.go` (`Artifact`: `SessionID`/`AgentID` köken, `Kind`, `Language`, `Content`, `Version`, inline `Revisions[]` geçmiş), `store_artifact.go` (Create / Get / List(session filtreli) / `UpdateArtifactContent`[eski sürümü `Revisions`'a arşivler + `Version++`] / `UpdateArtifactMeta` / Delete), `db.go` (`artifacts` map + `dirArtifacts` + load).
- [x] **Üretim araçları** (`internal/tools`): native (anthropic/minimax) yola `create_artifact` + `update_artifact` (`builtin_artifact.go`); `artifact.go` `ArtifactSink` arayüzü + `WithArtifacts`/`artifactsFrom` context köprüsü (ask_user deseni, import-cycle'dan kaçınmak için `tools` paketinde). Araç çıktısı JSON ref (`{id,title,kind,version,action}`).
- [x] **Wiring** (`internal/agent/toolsetup.go` + `internal/api`): `buildRegistry` iki aracı kaydeder; api `artifactSink` (session+agent damgalı) `chat.go` + `chat_stream.go`'da `WithArtifacts` ile turuna bağlanır. İnteraktif olmayan koşularda sink yok → araç hata döner.
- [x] **API** (`api/artifacts.go` + `registerArtifactRoutes`): `GET/POST /api/artifacts`, `GET/PUT/DELETE /api/artifacts/{id}`.

**Frontend**
- [x] `types.ts`/`api.ts`: `Artifact`/`ArtifactKind`/`ArtifactRevision`/`ArtifactRefResult` + artifact CRUD metodları.
- [x] NavRail "Artifactlar" view (`FileCode` ikonu) + `App.tsx` `artifactTarget` deep-link state + `openArtifact`.
- [x] `ArtifactsPanel.tsx`: liste (tür rozeti + sürüm + zaman) + sürüm seçicili görüntüleyici + kopya/sil/kaynağa-git.
- [x] `artifacts/ArtifactView.tsx`: kind→renderer (markdown→`Markdown`, code→`CodeBlock`, html→sandbox'lı `iframe srcDoc`, svg→inline, mermaid→kaynak, text→`pre`).
- [x] `chat/ArtifactCard.tsx`: `create_artifact`/`update_artifact` tool adımı → tıklanabilir kart (`TurnSteps` özel-durumu) → `onOpenArtifact` ile artifact ekranına atlar.

**CANLI TEST (API + Playwright):**
- [x] API CRUD: manuel create → list/get/filtre(sessionId) → update (içerik) **v1→v2**, eski sürüm `revisions`'a arşivlendi; 404 doğru.
- [x] Tool kataloğu: `mcpEnabled` ajanda **13 araç** (`create_artifact`+`update_artifact` dahil).
- [x] Kalıcılık: tek backend restart'ında **3 artifact diskten reload** edildi (round-trip).
- [x] Playwright (5174→test backend :8095 proxy): "Artifactlar" view, 3 tür liste, markdown/kod render, **HTML sandbox iframe** (`srcDoc`), sürüm v2/v1 seçici — DOM doğrulandı.
- [x] `go build/vet/test ./...` + frontend `tsc --noEmit` temiz.

> Not: claude-cli yolu kendi tool döngüsünü `--mcp-config` ile sürdüğünden bu built-in araçları kullanmaz (SDK parite deseniyle bilinçli). Gerçek LLM-tetikli artifact üretimi anthropic/minimax anahtarı gerektirir; API + sink yolu doğrudan doğrulandı.

### Tırnakla komut kaçışı + Komutlar referans ekranı ✅ (2026-06-16)
İki küçük UX iyileştirmesi:

1. **Tırnak içine alınan komut = düz metin** (`components/chat/UserBubble.tsx`): Bir mesaj `/` ile başlayan bir komutu **tırnak içinde** içeriyorsa (`"/komut"`, `'/komut'`, `` `/komut` ``, akıllı tırnaklar dahil), komut stili (mono + ⌘ + kenarlık) **uygulanmaz**; tırnaklar soyulur ve içerik normal balonda düz metin olarak gösterilir. Böylece bir komuttan *bahsetmek* mümkün. `quotedCommand()` yardımcısı + `QUOTE_PAIRS` haritası. Composer paleti zaten `/` ile başlamayan girişlerde açılmadığından tırnaklı giriş paleti de tetiklemez.
2. **"Komutlar" ayar ekranı** (`SettingsPanel.tsx`): Ayarlar'da yeni `commands` kategorisi (⌘ "Komutlar") tüm slash komutlarını ikon + `/ad` + açıklama ile **salt-okunur** listeler. Komut listesi App'ten `commands={chatCommands}` prop'u ile geçirilir; bilgilendirme kutusunda tırnakla kaçış da anlatılır. Bu kategori için Kaydet butonu gizli (referans ekranı).

> Doğrulama: kendi dosyalarımda tsc temiz. Not: Aynı ağaçta paralel **artifacts** WIP'i (ArtifactsPanel) henüz bağlanmadığından `tsc -b` o dosyalarda kırmızı; commit ağaç yeşile dönünce yapılacak.

### Sohbet sidebar = oturum listesi + okundu/okunmadı ✅ (2026-06-16)
Sohbet sol paneli ajanlardan arındırıldı; oturum-merkezli hale geldi (Playwright ile canlı test edildi):

1. **Sadece oturumlar**: Sohbet sidebar yalnızca oturumları gösterir (`SessionsSidebar.tsx`). Ajan roster'ı `AgentRoster.tsx`'e taşındı (Hafıza/Araçlar sidebar'ı). **Ajanlar** NavRail view'i **iki-panelli** (`AgentsView.tsx`): solda roster (★ ile varsayılan seç), ajana tıklayınca sağda ayarları düzenlenir; form ortak `AgentSettingsForm.tsx`'e çıkarıldı (`AgentsView` sağ paneli + `AgentSettingsModal` roster ⚙'i paylaşır). Eski `Sidebar.tsx` silindi. **Ajan silme** (2026-06-16): sağ panel footer'ında 🗑 Sil → `DELETE /api/agents/{id}` (`db.DeleteAgent` ajan + sahip oturumları siler, `Runtime.Stop` worker'ı durdurur), onaylı; Playwright ile test edildi (geçici "SilTest" ajanı oluşturulup silindi, roster + backend'den kalktı).
2. **Zaman gruplama + sıralama**: oturumlar `updatedAt` desc (backend zaten böyle) + **Bugün / Dün / Geçen hafta / Geçen ay / Daha eski** kovaları (`lib/time.ts`: `bucketOf` takvim-günü bazlı, `relativeTime`). Her satırda relative zaman + mesaj sayısı.
3. **`updatedAt`**: `AddMessage` her mesajda (kullanıcı turu başı + ajan yanıtı) `UpdatedAt` basıyor → oturum otomatik en üste.
4. **Oturum ⚙ menüsü**: Başlığı düzenle (inline → `POST /api/sessions/{id}/title {title}`), AI ile başlık, Yolu kopyala (`GET .../path`), Klasörü aç (`POST .../reveal` → Explorer), Sil (`DELETE /api/sessions/{id}`, onaylı).
5. **Okundu/okunmadı** (`Session.Unread`): `AddMessage` assistant'ta `Unread=true`; `MarkSessionRead` temizler. Oturum açılınca + aktif tur bitince okundu; açık değilken gelen yanıt **nokta + kalın** ile okunmadı. Olay feed'i aktif workspace'te listeyi tazeler.

> Doğrulama: build/vet + tsc yeşil; Playwright: sessions-only + Bugün/Dün gruplama + zamanlar, ⚙ menü (manuel rename uygulandı), Ajanlar view; API: path doğru + `/api/chat` sonrası `unread=true` + en üste taşındı + frontend nokta/kalın render.
> Not: Aynı çalışma ağacında eşzamanlı **artifacts** + **tema presetleri** çalışması var; commit ayrımı kullanıcı onayına bırakıldı (bkz. oturum sonu özeti).

### Workspace switcher iyileştirmeleri ✅ (2026-06-16)
Sol-üst workspace seçici elden geçirildi (Playwright ile canlı test edildi):

1. **Dışarı tıkla-kapat** (`WorkspaceSwitcher.tsx`): `mousedown` dinleyicisi + `rootRef`; panel dışına tıklayınca kapanır.
2. **Boş-isim sağlamlığı**: ad `||` ile gösterilir (önceki `??` boş string'i geçiriyordu); listede fallback "İsimsiz".
3. **Popup ile oluşturma** (`WorkspaceCreateModal.tsx`): ad + emoji simge paleti + opsiyonel **veri klasörü**. Klasör: `POST /api/pick-folder` (Windows native FolderBrowserDialog) "Gözat" butonu **veya** elle yol. Backend `Manager.Create(name, parentPath)` + `Meta.Path` (boş=varsayılan; özel yolda `{path}/tionswarm-{id}` alt klasörü → silme komşu içeriği bozmaz).
4. **Varsayılan ajan**: `handleCreateWorkspace`→`seedDefaultAgent` yeni workspace'e "Asistan" ajanı ekler (provider önceliği ws→app→claude-cli).
5. **Çapraz-workspace etkinlik rozeti**: aktif olmayan workspace'te olay (görev/zamanlama/heartbeat **+ sohbet tamamlanma**) olunca switcher'da nokta belirir. `chat_stream` `done`'da `Runtime.Emit` (yeni exported) `chat` olayı yayar (yalnız rozet). `App.tsx` `unreadWs` Set olay feed'inden dolar, geçince temizlenir; `NavRail` collapsed + switcher tetik/satır nokta.
6. **Sağlamlık fix**: `withWorkspace` bilinmeyen `X-Workspace-Id`'de 400 yerine Varsayılan'a düşer (silinmiş id uygulamayı kilitlemez).

> Doğrulama: build/vet/test + frontend tsc yeşil; Playwright canlı: dışarı-tıkla-kapat, modal oluşturma (ikon 🚀 + ad), varsayılan ajan beliriş, çapraz-ws rozet beliriş→geçişte temizleniş, konsol temiz.

### Tıklanabilir bildirimler + otonom olay akışı ✅ (2026-06-16)
Masaüstü bildirimleri artık hedefe **deep-link**'lenir; otonom olaylar (heartbeat/task/schedule) backend'den frontend'e akar.

1. **Frontend bildirim navigasyonu** (`frontend/src/lib/clientPrefs.ts`): `notify(...)` opsiyonel `onClick` alır → `window.focus()` + navigasyon. Sohbet yanıt-hazır bildirimi kaynak sohbete (`setView('chat')`+`selectSession(sid)`), sohbet hatası loglara (`setView('logs')`) atlar (`App.tsx`).
2. **`internal/events` (yeni):** `Event` (type/level/workspaceId/title/body/target/time) + `Bus` (süreç-geneli pub/sub, nil-safe, slow-subscriber drop).
3. **`GET /api/events` SSE** (`internal/api/events.go`): aboneye `notify` olayları akıtır; 25sn ping keep-alive; global (workspace-scoped değil).
4. **Yayın noktaları:** `Runtime` artık `bus`+`wsID`+`wsName` taşır (`NewRuntime`/`NewManager`/`NewServer` imzaları güncellendi, `main.go` `events.NewBus()` enjekte eder). Heartbeat hatası/auto-disable (`worker.tick`→`Runtime.emitHeartbeatFailure`, target=logs), görev bitti/başarısız (`executor.RunTask`→`publish`, target=board+taskId), zamanlanmış prompt teslimi (`scheduler.emitPromptDelivery`: başarı→chat+sessionId, hata→logs). Görev olayları tüm tetikleyiciler için yayılır (frontend görünürken bastırır).
5. **Frontend abonelik** (`api.subscribeEvents`, EventSource oto-reconnect; `App.tsx` `onEventRef` taze closure + tek-mount `useEffect`): olay gelince `notify` ile bildirim, tıklayınca gerekirse workspace değiştirip `target.view`/`sessionId`'e gider.
6. **Doğrulama:** `go build`/`vet`/`test ./...` yeşil + frontend `tsc --noEmit` temiz + **canlı SSE smoke**: anahtarsız görev çalıştırma → `event: notify` `{type:task,level:error,target{view:board,taskId},workspaceId,...}` uçtan uca yakalandı.

### Kullanıcı mesajında @mention / komut stili ✅ (2026-06-16)

Gönderilen kullanıcı mesajları artık `@mention` ve `/komut` içerdiğinde farklı render edilir.

- [x] `components/chat/UserBubble.tsx` (yeni): kullanıcı balonunu render eder. `@AjanAdı` token'ları **çip** olarak vurgulanır (eşleşen ajanın avatar rengiyle; eşleşmezse yarı-saydam beyaz). Mention içeren balona `ring-1 ring-white/40` halka. `/` ile başlayan komut-mesajı **mono yazı tipi + ⌘ + accent-soft kenarlık** ile ayrı stilde.
- [x] `MessageList.tsx`: kullanıcı dalı düz `{m.text}` yerine `<UserBubble text agents>` kullanır.

**CANLI TEST (Chrome DOM):**
- [x] "@Reminder kısa selam ver" → balon `ring-1`, `@Reminder` çip span'i; mention Reminder'a yönlendi ("Selam! 👋").
- [x] "/deneme …" → `font-mono` + ⌘ + accent-soft kenarlıklı komut balonu.
- [x] `tsc + vite build` temiz.

### Akış müdahalesi: Durdur / Sıraya / Kes / Yönlendir (steering) ✅ (2026-06-16)

Token akarken kullanıcı turu **canlı kontrol edebiliyor**. Composer butonları streaming durumuna göre değişir.

**Backend (kontrol kanalı):**
- [x] `api/chat_control.go` (yeni): `chatRuns` kayıt defteri (runId → {cancel, steer chan}); `POST /api/chat/control` (`action: stop|steer`).
- [x] `api/chat_stream.go`: tur başına `runId` üretir, **cancelable ctx** + steer chan kaydeder, `meta` event'ine `runId` ekler, ctx'i `agent.WithSteer` ile zenginleştirir.
- [x] `agent/steer.go` (yeni): `WithSteer` ctx + `drainSteer`. `toolloop.go`: native tool döngüsünde her iterasyon başında steer mesajlarını çeker → `[Canlı kullanıcı yönlendirmesi] …` user mesajı olarak ekler + iz adımı (`↪ Yönlendirme`). (Düz/araçsız turda mid-turn etkisizdir; tool döngüsünde etkili.)
- [x] `server.go`: `runs *chatRuns` alanı + route.

**Frontend:**
- [x] `api.ts`: `chatStream` `onMeta`'ya `runId`; `chatControl(runId, action, text)`.
- [x] `App.tsx`: `streaming` state + `AbortController` (stop/interrupt) + `runIdRef` (steer) + `queuedRef` (done'da otomatik gönder); `stopTurn`/`interruptTurn`/`queueMessage`/`steerTurn`. Stop'ta kısmi balon korunur (abort error gizlenir).
- [x] `Composer.tsx`: **akış yok** → Gönder; **akış + boş input** → 🔴 Durdur; **akış + dolu input** → Sıraya / Kes / Yönlendir. Akarken Enter = Sıraya.

**CANLI TEST (API + Chrome):**
- [x] Stop deterministik: SSE'den `runId` alındı → `POST /api/chat/control stop` → tur **+1.5sn**'de ctx-iptal ile sonlandı (`error` event).
- [x] Control validasyon: bilinmeyen run → 404.
- [x] Chrome: mesaj gönderildi → akış + yanıt geldi (uçtan uca streaming UI çalışıyor).
- [x] `go build/vet ./...` + `tsc + vite build` temiz.

> Not: Yönlendir (steer) anlamlı etkiyi **araç kullanan (agentic) turlarda** gösterir; düz tek-atış sohbette mid-turn enjekte edilemez. Stop/Interrupt/Queue her ajanda çalışır.

### Faz P2 — Built-in dosya/shell araçları (SDK paritesi) ✅ (2026-06-16)

Native (anthropic/minimax) tool-use yolundaki ajanlara **yerleşik dosya sistemi araçları** ve (opsiyonel, varsayılan kapalı) **shell** aracı eklendi. Tümü workspace'in `workspace/` alt dizinine **sandbox**'lanır (path-traversal koruması). Detaylı tasarım: [09-CLAUDE-AGENT-SDK.md](09-CLAUDE-AGENT-SDK.md) (Faz P2).

- [x] **Sandbox** (`internal/tools/sandbox.go`): `Sandbox{Root}` + `Resolve` — mutlak yol reddi, `..` kaçış reddi, kök-altı doğrulaması; `Rel` (görüntüleme için).
- [x] **FS araçları** (`internal/tools/builtin_fs.go`): `read_file` (256KB cap), `write_file` (dizin oluşturur), `edit_file` (tam string değişimi; tekil/`replace_all`), `list_dir` (dizinler önce), `glob` (`**`/`*`/`?` → RE2; 500 cap), `grep` (RE2 + opsiyonel glob filtre; ikili dosya atlama; 200 cap).
- [x] **Shell aracı** (`internal/tools/builtin_shell.go`): Windows'ta `powershell.exe -NoProfile -NonInteractive`, diğerinde `/bin/sh -c`; cwd = sandbox kökü; timeout (varsayılan 30s, max 120s); çıktı 64KB cap. **Yüksek riskli → varsayılan kapalı**, `TIONSWARM_ENABLE_SHELL=1` ile açılır (permission katmanı P3 gelene dek opt-in).
- [x] **Wiring**: `Runtime.workDir` alanı (`NewRuntime` parametresi); `workspace.Manager.open` her workspace için `filepath.Join(dir,"workspace")` geçirir; `Tunables.shellEnabled` (+ `SetShellEnabled`/`ShellEnabled`); `main.go` env'den okur; `agent/toolsetup.go buildRegistry` sandbox hazırsa fs araçlarını, gate açıksa shell'i kaydeder.
- [x] **Test** (`internal/tools/builtin_fs_test.go`): sandbox resolve (in-bounds/escape/absolute/not-ready), write→read→edit round-trip, non-unique edit reddi, list/glob/grep, glob-restricted grep, `globToRegexp` tablo testi.

**CANLI TEST (build + unit + API):**
- [x] `go build/vet ./...` + `go test ./...` tamamı temiz (yeni fs testleri dahil).
- [x] API: ajan oluştur → `mcpEnabled=true` → `GET /api/agents/{id}/tools` kataloğu **10 araç** döndü (3 built-in + 6 fs + shell, `TIONSWARM_ENABLE_SHELL=1` ile).
- [x] Shell gate doğrulandı: env olmadan katalogda `shell` yok.

> Mimari not: Bu araçlar **native** tool-use döngüsünde (`agent/toolloop.go`) çalışır. **claude-cli** yolu kendi döngüsünü `--mcp-config` ile sürdüğünden (ve kendi Read/Write/Bash araçları olduğundan) bu built-in'leri kullanmaz — SDK parite tasarımıyla bilinçli uyum. Gerçek LLM-tetikli yürütme anthropic/minimax anahtarı gerektirir.

### Session-bazlı sohbet + çok-ajanlı `@` yönlendirme ✅ (2026-06-16)

Sohbet ekranı **ajan-bazlıdan session-bazlıya** çevrildi; her oturumun bir **varsayılan
ajanı** var ve `@` ile başka ajanlar aynı sohbete dahil edilebiliyor.

- [x] **Backend:** `db.Message.AgentID` (turu üreten ajan). `chatReq.AgentIDs []string`;
  `chat_stream.go` çoklu-ajan döngüsü (`resolveTurnAgents`, boş→oturum varsayılanı), her ajan
  sırayla yanıtlar (sonrakiler öncekini görür); SSE `meta`→`agent {agentId,index}`→`step`*→`reply`→`done`;
  her tur `Message.AgentID` ile kalıcı. `POST /api/sessions` `agentId` opsiyonel (boş→ilk ajan).
- [x] **Frontend:** `App` tüm oturumları yükler; `Sidebar` düz **"Tüm Oturumlar"** + roster
  "Ajanlar · varsayılan" seçici (avatarlı); `Composer` `@` menüsü metne `@Ad` mention ekler;
  `sendMessage` mention'ları `agentIds[]`'e çözer + çoklu-ajan canlı balonları (`onAgentStart`/`onReply`);
  `MessageList` mesaj başına ajan avatar+adı. `defaultAgentId` localStorage'da.
- **CANLI TEST (API + Chrome):** `agentIds=[Reminder,StepTest]` → iki asistan mesajı sırayla,
  biri Reminder biri StepTest etiketli (kalıcı); UI "Tüm Oturumlar" iki ajanın oturumlarını tek
  listede avatarlarıyla, mesajlar 🤖 Reminder / ST StepTest başlıklarıyla; `@Rem`+Enter →
  `@Reminder ` eklendi. `go build` + `tsc` temiz.

#### Tamamlama — @mention farkındalığı + HEAD onarımı ✅ (2026-06-17)

Önceki turda `chat_stream.go` çoklu-ajan döngüsü commit'lenmiş ama destekleyen
tanımlar commit dışı kalmıştı → **HEAD derlenmiyordu** (commit `013a16a`,
`adoptMentionedAgent`/`SetSessionAgent`/7-arg `composeTurnRequest` çağrılıyor ama
tanımları yok). Eksik parçalar tamamlanıp commit'lendi (`7127aef`):

- `db.SetSessionAgent` — oturumun varsayılan ajanını yeniden sabitler.
- `adoptMentionedAgent` — **yeni** bir oturum `@X` ile açılınca tüm thread'i X'e
  sabitler (`MessageCount==0` + mention varsa; yerleşik/mention'sız turlarda no-op,
  zaten varsayılansa gereksiz yazma yok). `chat_stream.go:110`'dan çağrılır.
- `composeTurnRequest(... turnAgents []db.Agent ...)` — her ajana **kendi adını** ve
  `@mention` mekaniğini anlatan sistem-prompt notu (baştaki `@Ad` = ajanı çağırma,
  dosya/skill değil); çok-ajanlı turda diğer adreslenmiş ajanları da bildirir
  ("sırayla yanıtlıyorsunuz, başkası adına konuşma").
- `chat.go` (blocking yol) tek-ajan dilimi `[]db.Agent{agent}` geçirir.
- Testler: `chat_turn_test.go::TestResolveTurnAgents` (sıra/dedup/fallback) +
  `TestAdoptMentionedAgent` (adopt/yerleşik-no-op/mention'sız/zaten-varsayılan).

✅ `go build`/`vet`/`test` + frontend `tsc -b`/`vite build` yeşil. Frontend tarafı
(Composer `@` menüsü, `useChatStream` mention→`agentIds[]` çözümü, çoklu balon) zaten
commit'liydi. **Kalan:** iki gerçek ajan + canlı provider gerektiren uçtan uca
çoklu-ajan Playwright testi (sunucu+frontend ayağa kalkmalı) sıraya alındı.

### Detaylı model etiketleri + ajan thinking seviyesi ✅ (2026-06-16)

- [x] **Detaylı model isimleri** (`internal/providers/catalog.go`): her modele açıklayıcı `Label` (ör. "Claude Opus 4.8 — en yetenekli") + `Description` (tek satır not). `ProviderModelSelect` seçili modelin açıklamasını dropdown altında ipucu olarak gösterir.
- [x] **Thinking (uzatılmış akıl yürütme) seviyesi** — ajan başına: `db.Agent.ThinkingLevel` ("" / off / low / medium / high) + `AgentProfilePatch`/`UpdateAgent`; `createAgentReq`/`updateAgentReq` (`PUT/POST /api/agents`). `providers.Request.ThinkingBudget`; `anthropic.go` `thinkingFor` → `thinking:{type:enabled,budget_tokens}` + gereğinde `max_tokens` yükseltme (Complete + Stream). `agent/toolloop.go` `thinkingBudgetForLevel` (low=2048/medium=8192/high=16384) **yalnız `!MCPEnabled` (araçsız) dalında** enjekte edilir — native tool döngüsü imzalı thinking bloğu gerektirdiğinden tool turlarında kapalı. claude-cli/minimax param'ı yok sayar (yalnız anthropic etkili).
- [x] Frontend: `Agent.thinkingLevel` + `AgentPatch`; `AgentSettingsModal`'da "Düşünme (thinking) seviyesi" dropdown'ı (Kapalı/Düşük/Orta/Yüksek) + "yalnız anthropic & araçsız" notu.

**CANLI TEST (API):**
- [x] `/api/catalog`: tüm modeller detaylı label + description ile döndü (Claude/MiniMax).
- [x] Ajan create `thinkingLevel=high` → kalıcı; update `medium` → kalıcı.
- [x] `go build/vet ./...` temiz. (Frontend tsc bu sırada paralel **Composer.tsx** WIP'i yüzünden kırıktı — benim dosyalarımda hata yok; gözcü yeşili bekliyor.)

### Model kataloğu + provider/model seçici + MiniMax sağlayıcı ✅ (2026-06-15)

Ajan için provider+model artık **listeden** seçilebiliyor (serbest girişe de izin var) ve yeni bir **MiniMax** sağlayıcı (OpenAI-uyumlu) eklendi.

- [x] **Katalog** (`internal/providers/catalog.go`): provider+model listesi (claude-cli aliasları, Anthropic Claude modelleri, MiniMax M2.1/M2.1-lightning/M2); her provider `allowCustomModel`. `GET /api/catalog` canlı `available` bayrağıyla (claude-cli PATH'te mi, anthropic/minimax anahtarı var mı).
- [x] **MiniMax provider** (`internal/providers/minimax.go`): OpenAI-uyumlu `POST {base}/chat/completions` (Bearer), `base_resp`/`error` zarfı işlenir; varsayılan `https://api.minimax.io/v1`. `registry` → `SetMinimax(key, baseURL)` + Get("minimax"); `settings` → şifreli `MinimaxKeyEnc` + `MinimaxBaseURL`, DTO `minimaxKeySet`/`minimaxBaseUrl`, `applySettings` push.
- [x] **Frontend** `ProviderModelSelect.tsx` (yeniden kullanılabilir, katalogu modül-düzeyinde cache'ler): provider dropdown + model dropdown + "Özel…" serbest giriş. AgentSettingsModal, Sidebar **ajan oluşturma** (artık model de seçiliyor → `onCreateAgent(name,soul,provider,model)`), SettingsPanel varsayılan provider/model'e entegre. SettingsPanel "Sağlayıcılar"a **MiniMax API anahtarı + base URL** alanları.

**CANLI TEST (API + Chrome):**
- [x] `GET /api/catalog`: 3 provider modelleriyle döndü; anahtarsız anthropic/minimax `available=false`.
- [x] MiniMax key PUT → `minimaxKeySet=true`, baseUrl persist; katalogda minimax `available=true` oldu (sonra sıfırlandı).
- [x] Chrome: ajan oluşturma formunda Sağlayıcı dropdown'ı **Claude CLI · Anthropic (anahtar gerek) · MiniMax (anahtar gerek)** seçeneklerini gösterdi (HTML doğrulandı).
- [x] `go build/vet ./...` + `tsc + vite build` temiz.

> Not: MiniMax (OpenAI-uyumlu istemci) tool-use'suz düz completion yapar; aynı istemci başka OpenAI-uyumlu uçlar için de base URL ile kullanılabilir. Model ID'leri sık değiştiğinden her provider serbest model girişine izin verir.

### Ajan görseli (yuvarlak avatar) + ajan ayar modalı ✅ (2026-06-15)

Ajan listesindeki her ajana **yuvarlak görsel** (avatar) eklendi ve satır kenarındaki **⚙ ayar butonundan** ajan değerleri düzenlenebilir hale geldi.

- [x] **Backend:**
  - `db.Agent`'a `avatar` (emoji/glif) + `color` (hex accent) alanları.
  - `db.UpdateAgent(id, AgentProfilePatch)` — pointer alanlı kısmi güncelleme (nil = dokunma), atomik diske yazar.
  - `PUT /api/agents/{id}` (`handleUpdateAgent`) — name/soul/identity/provider/model/planningMode/avatar/color kısmi patch. `handleCreateAgent` de opsiyonel avatar/color kabul eder.
- [x] **Frontend:**
  - `lib/avatar.ts`: id'den **deterministik renk** türetme (djb2 hash → palet), baş harf çıkarımı, glif/renk paletleri (`AVATAR_COLORS`, `AVATAR_GLYPHS`).
  - `components/AgentAvatar.tsx`: yuvarlak gradient disk — özel emoji ya da baş harf; aktif ajanda halka.
  - `components/AgentSettingsModal.tsx`: canlı avatar önizlemeli editör (ad, emoji seçici, renk paleti, sağlayıcı/model, planlama modu, soul, identity).
  - `Sidebar.tsx`: ajan satırı avatar + hover'da görünen ⚙ butonu (oturum ⟳ deseniyle aynı); modal render.
  - `types.ts` `Agent.avatar/color` + `AgentPatch`; `api.ts` `updateAgent`; `App.tsx` `updateAgent` callback (liste canlı güncellenir).

**CANLI TEST (API + Chrome):**
- [x] `PUT /api/agents/{id}` avatar/color/soul patch'ledi; dosyada `avatar` 🤖 (U+1F916), `color` `#7c3aed` doğru UTF-8 kalıcılaştı.
- [x] Chrome: roster yuvarlak avatarlar (RE/ST deterministik renk); ⚙ → modal açıldı; emoji+renk seçip Kaydet → roster anında 🤖 mor daireye döndü (DOM doğrulandı).
- [x] `go build ./...` + `tsc --noEmit` + `vite build` temiz.

### Workspace geniş düzenleme + paralel çalışma ✅ (2026-06-15)

Workspace düzenleme seçenekleri genişletildi ve paralel çalışma netleştirildi.

- [x] **Görsel kimlik**: `WSSettings`'e `Icon` (emoji) + `Color` (hex). `/api/workspaces` listesi artık icon/color ile zengin; WorkspaceSwitcher + NavRail daraltılmış rozet ikon/renk gösterir. Emoji uçtan uca round-trip doğrulandı (disk codepoint `D83D DE80`).
- [x] **İstatistikler**: `workspace-settings` DTO'ya `createdAt` + `agentCount`/`sessionCount`/`taskCount` (canlı sayım). Ayar ekranında rozet kartları.
- [x] **Silme**: "Bu Workspace" kategorisinde tehlikeli-bölge sil butonu (`onDeleteWorkspace`, App'te confirm + başka workspace'e geçiş; backend son workspace'i silmeyi reddeder).
- [x] **Paralel çalışma (zaten var, doğrulandı)**: `workspace.Manager.open()` boot'ta **her** workspace için `Runtime.StartConfigured` (heartbeat ajanları) + `Scheduler.Start` (cron) + `ResumeRunningFlows` çağırır → tüm workspace'lerin otonom ajanları/zamanlamaları **aynı süreçte eşzamanlı** koşar (UI yalnız aktif olanı gösterir). Per-workspace pause (`Runtime.SetPaused`) ile biri diğerlerini etkilemeden durdurulabilir.

**CANLI TEST (API + Chrome):**
- [x] workspace-settings GET: icon/color + stats (2 ajan/3 oturum/0 görev/createdAt) döndü.
- [x] PUT icon=🚀 color=#22c55e → kaydedildi; disk codepoint D83D DE80 (emoji bozulmadan); `/api/workspaces` zenginleşti.
- [x] Chrome: "Bu Workspace" kategorisi — istatistik kartları, ikon/renk alanları, 🚀 önizleme, sil bölümü render (DOM doğrulandı). Sonra varsayılana sıfırlandı.
- [x] `go build/vet ./...` + `tsc + vite build` temiz.

### Loglar ekranı (uygulama + tüm workspace) ✅ (2026-06-15)

En sol NavRail'e **📜 Loglar** görünümü eklendi; tüm uygulama ve workspace logları tek ekranda.

- [x] `internal/logbuf` (yeni): `Buffer` (sabit kapasiteli ring, 2000) + `slog.Handler` (kayıtları buffer'a yakalar **ve** stdout text handler'a delege eder). `main.go` logger'ı bu handler ile kurar → tüm workspace runtime'ları aynı logger'dan geçtiği için **çapraz-workspace** tüm akış tek buffer'da.
- [x] `api/logs.go` (yeni): `GET /api/logs?limit=&level=&q=` — seviye (min) + substring filtre; uygulama-geneli (workspace-scoped değil). `Server`'a `*logbuf.Buffer` alanı + `NewServer` imzası.
- [x] Frontend: `types.ts`/`api.ts` `LogEntry` + `getLogs`; `NavRail` "📜 Loglar"; `LogsPanel.tsx` (canlı poll 2.5sn "Canlı" toggle, seviye filtre chip'leri, arama, renkli seviye rozeti, zaman+mesaj+alanlar, oto-scroll); `App.tsx` render.

**CANLI TEST (API + Chrome):**
- [x] `/api/logs` boot loglarını döndürdü (config/providers/scheduler/workspace opened + attrs).
- [x] Filtre: `level=error`→0, `q=workspace`→2; yeni workspace oluştur → log anında yakalandı (`name=LogTest WS`).
- [x] Chrome: 📜 Loglar render — zaman/seviye/mesaj/attrs satırları, 2 workspace'in açılış logları görünür (DOM doğrulandı).
- [x] `go build/vet ./...` + `tsc + vite build` temiz.

### Depolama: SQLite → Dosya Sistemi ✅ (2026-06-15)

Kalıcılık katmanı **tamamen dosya-tabanlıya** çevrildi (External Agents OSS tarzı).
SQLite (`modernc.org/sqlite`), migration runner ve `migrations/*.sql` kaldırıldı.
Detay: **`_Docs/08-DEPOLAMA.md`**.

- [x] `internal/db` **yerinde** dosya-store'a çevrildi: bellek-içi maps + diske
  atomik write-through (`*.tmp`→`rename`), tek `sync.RWMutex`. **Tüm metod imzaları,
  model struct'ları, sabitler, `ErrNotFound` korundu** → onu kullanan 22 dosya
  (`agent`/`api`/`conversation`/`memory`/`workspace`) **değişmedi**.
- [x] Disk yapısı: `{wsID}/store/` altında entity-başına JSON + `sessions/{id}/session.jsonl`
  (satır 1 header, satır 2+ mesajlar). Embedding base64 gömülü; usage gün-bazlı dosya.
- [x] `tionswarm.db` yolu → `store/` dizini (`workspace/manager.go`); ölü `Config.DBPath()` silindi.
- [x] `go.mod` temizlendi: yalnız `google/uuid` + `robfig/cron/v3` kaldı (sqlite + ~8 dolaylı dep gitti).
- [x] `internal/db/filestore_test.go` round-trip testi (create→reopen→reload + UTF-8/HTML/embedding/cascade) **PASS**.

**CANLI TEST (API + restart):**
- [x] Backend temiz veri diziniyle ayağa kalktı; `store/` boş oluştu.
- [x] API ile ajan/oturum/görev/MCP oluşturuldu → doğru disk dosyaları yazıldı.
- [x] **Restart** sonrası 2 ajan + 2 oturum + 1 görev + 1 MCP **diskten reload** edildi; Türkçe başlık (`İlk sohbet — ğüşıöç`) round-trip korundu.
- [x] `go build/vet ./...` temiz.

> Not: Otomatik SQLite→dosya migration'ı yok (kullanıcı talebiyle SQLite tamamen kaldırıldı); yeni kurulum `store/`'da sıfırdan başlar.

### Ayarlar ekranı — genişletme: 2 panel + workspace + profil + beta'lar ✅ (2026-06-15)

Ayarlar ekranı **sohbet gibi 2 panele** dönüştürüldü (sol kategori rayı, sağ içerik) ve kapsam genişletildi.

- [x] **2 panel UI** (`SettingsPanel.tsx` yeniden): sol rayda **Uygulama** grubu (Profil · Görünüm · Bildirimler & Ekran · Sağlayıcılar · Bağlam & Bellek · Bütçe · Otonomi · Otomatik Başlık · MCP · Tanılama · Hakkında) + **Bu Workspace** grubu; kaydedilmemiş kategoride nokta göstergesi; kategoriye-özel Kaydet (app/workspace ayrı scope).
- [x] **Workspace'e özel ayarlar** (`internal/workspace/settings.go`): her workspace'in `ws-settings.json`'u — ad (rename), açıklama, **bu workspace'e özel** varsayılan sağlayıcı/model, **bu workspace'te otonomiyi duraklat**. `Manager.Rename`/`UpdateSettings`; `agent.Runtime` per-workspace `SetPaused/Paused` (guardedComplete'te global pause ile birlikte kontrol); `GET/PUT /api/workspace-settings` (X-Workspace-Id). Ajan oluşturmada öncelik: istek → workspace → uygulama → varsayılan.
- [x] **Kullanıcı profili** (resimdeki gibi): Ad · Saat dilimi · Şehir · Ülke · Notlar — `settings` alanları; chat sistem promptuna `## About the user` bloğu olarak enjekte edilir (`userContextBlock`).
- [x] **Masaüstü bildirimleri** + **Ekranı açık tut**: istemci-tarafı (`lib/clientPrefs.ts` — Notification API + Wake Lock, visibility-change'te yeniden alır); pencere arkadayken yanıt gelince bildirim.
- [x] **Anthropic beta'ları** (yalnız anthropic sağlayıcı): **1M token bağlam** (`context-1m-2025-08-07`) + **uzatılmış prompt cache 1 saat** (`extended-cache-ttl-2025-04-11` + system'e `cache_control` ttl=1h). `anthropic.go` `WithBetas`/`betaHeader`/`systemField`; `registry.SetAnthropicBetas`; `applySettings` push.

**CANLI TEST (API + Chrome):**
- [x] settings yeni alanlar GET/PUT: profil (Bilal/Türkiye/İstanbul), 1M+cache+notif+awake → kaydedildi.
- [x] workspace-settings: rename "Ana Workspace" + açıklama + pause=true persist; `/api/workspaces` rename'i yansıttı; sonra varsayılana sıfırlandı.
- [x] Chrome: ⚙ Ayarlar → **Profil** kategorisi (Ad/Saat dilimi/Şehir/Ülke/Notlar) ve **Bu Workspace > Genel** kategorisi (ad/açıklama/sağlayıcı-model override) DOM ile doğrulandı; 0 konsol hatası.
- [x] `go build/vet ./...` + `tsc + vite build` temiz.

> Not: 1M context / uzatılmış cache yalnız **anthropic** sağlayıcıda etkilidir (claude-cli yok sayar). Db katmanı bu sırada paralel olarak SQLite'tan dosya-store'a taşındı; eklemeler birleşik derlemede temiz.

---

### Ayarlar ekranı (uygulama geneli) ✅ (2026-06-15)

Uygulama genelinde tek bir ayar belgesi (`settings.json`, data dizininde) + Ayarlar UI eklendi.
Hassas Anthropic API anahtarı AES-GCM ile şifreli saklanır, istemciye asla düz dönmez
(`anthropicKeySet` boolean). Ayarlar canlı olarak alt sistemlere uygulanır.

**Backend**
- [x] `internal/settings/` (yeni): `settings.go` (Settings + DTO maskeli + Patch), `store.go` (thread-safe JSON deposu, atomik yazım, normalize/clamp, `Cipher` arayüzü ile şifreli secret).
- [x] `providers/registry.go`: mutable + `SetAnthropicKey/SetClaudeCLIPath/SetDefaultModel`, RWMutex; `Get` canlı değerleri okur.
- [x] `conversation/manager.go`: `SetLimits` (canlı maxTokens/keepRecent), mutex.
- [x] `agent/tunables.go` (yeni): süreç-geneli `SetAutonomyPaused`/`SetTitleModel`; `budget.go` otonom çağrıda `ErrAutonomyPaused`; `titler.go` başlık-modeli override.
- [x] `api/settings.go` (yeni): `GET/PUT /api/settings`, `POST /api/settings/test-provider` (gerçek minimal completion ile bağlantı testi).
- [x] `api/server.go`: `settings` alanı + `NewServer` imzası + `applySettings()` (boot + her güncellemede push).
- [x] `api/agents.go`: yeni ajanlarda varsayılan provider/model + günlük bütçe ayarlardan.
- [x] `api/chat.go`: auto-title `autoTitleEnabled` ayarına bağlı.
- [x] `cmd/tionswarm/main.go`: settings store + env `ANTHROPIC_API_KEY`'i tek seferlik şifreli store'a migrate.

**Frontend**
- [x] `types.ts`/`api.ts`: `AppSettings`/`SettingsPatch`/`ProviderTestResult` + `getSettings/updateSettings/testProvider`.
- [x] `components/SettingsPanel.tsx` (yeni): 9 bölüm (Görünüm · Sağlayıcılar · Bağlam & Bellek · Bütçe · Otonomi · Otomatik Başlık · MCP · Tanılama · Hakkında), dirty-takip + tek Kaydet, API key maskeli/sil, provider test rozeti, toggle/sayı/select alanları.
- [x] `components/NavRail.tsx`: en altta sabit ⚙ Ayarlar görünümü.
- [x] `lib/theme.ts` (yeni) + `index.css`: açık tema (`[data-theme=light]`) + accent CSS değişkeni; `App.tsx` boot'ta + kayıtta `applyTheme`.

**CANLI TEST (API + Chrome):**
- [x] GET/PUT settings round-trip; `theme/maxContextTokens/pauseAutonomy` güncellendi.
- [x] Anthropic key: set → `anthropicKeySet=true` (değer dönmüyor), temizle → false.
- [x] test-provider claude-cli → `ok=true model=claude-opus-4-8 sample="OK"`.
- [x] Ajan varsayılanları: `defaultModel=sonnet` + `callLimit=50` ayarıyla yeni ajan o değerlerle oluştu.
- [x] Auto-title kapısı: `autoTitleEnabled=false` iken ilk mesajda başlık üretilmedi.
- [x] Chrome: uygulama 0 konsol hatasıyla yüklendi; ⚙ Ayarlar tıklandı, 9 bölüm render oldu (DOM doğrulandı). Görsel ekran görüntüsü 87 sekmeli ortamda `image readback` ile alınamadı (guide'da bilinen sorun).

### Sohbet: adım-adım akış (SSE streaming) ✅ (2026-06-15)

Sohbet artık tüm tur bitince değil, **her adım bittikçe** UI'a akıtılıyor.
- `providers.Request.OnEvent func(TraceStep)` + `claudecli.go` stream-json'u **satır
  satır** (`bufio` + `cliStreamParser`) okuyup olayları anında yayınlar (thinking
  hemen, ara metin flush'ta, tool adımı sonucu gelince). `agent/toolloop.go`
  `CompleteWithToolsStream(... onStep)` köprüler; native döngü de her adımı yayınlar.
- `internal/api/chat_stream.go` — `POST /api/chat/stream` (SSE): `meta`→`step`*→`done`.
  Tur sonunda mesaj + tam iz kalıcılaştırılır.
- Frontend: `api.streamChat` (`fetch`+`ReadableStream` SSE ayrıştırma), `App.sendMessage`
  canlı asistan balonu büyütür, `MessageList` boş balonda "çalışıyor" noktaları.
- **CANLI TEST:** `curl -N /api/chat/stream` zaman damgaları artımlı geldi
  (`meta` 08.9s → text 14.7s → Bash step-one 37.4s → Bash step-two 47.8s → `done` 56.7s);
  Chrome'da canlı asistan balonu akış sırasında "çalışıyor" göstergesiyle yakalandı.
  `go build ./...` + `tsc --noEmit` temiz.

### Zengin Sohbet Arayüzü (Chat UX) ✅ (2026-06-15)

Sohbet ekranı [external-agent-oss](https://github.com/external-agent-project/external-agent-oss)
referans alınarak External Agent benzeri zengin render katmanına kavuşturuldu: markdown
çıktı, tool kullanım kartları, düşünme adımları, dosya satır değişimi (diff),
tıklanabilir dosya yolları ve inline görsel. Asistan turu artık **aktivite izini**
(thinking + ara metin + tool çağrıları) ve son cevabı zengin biçimde gösterir.
Detaylı doküman: [07-CHAT-UX.md](07-CHAT-UX.md).

**Backend**
- [x] `internal/agent/trace.go` (yeni): `TurnStep` (kind=text|thinking|tool, tool/input/output/isError).
- [x] `internal/agent/toolloop.go`: `CompleteWithToolsTraced` — native agentic döngü ara metni + her `tool_use` çağrısını (girdi+sonuç) sıralı izle kaydeder. Eski `CompleteWithTools` bunu çağırıp izi yutar (imza uyumlu). İz **hem native hem claude-cli** (stream-json) yolunda üretilir — bkz. aşağıdaki "Anahtarsız adım izi" notu.
- [x] `db.Message.Steps` alanı (oturum JSONL'inde JSON `[]TurnStep`) — yeniden yüklemede tur yeniden çizilir; `AddMessage`/`ListMessages` korur. (O dönem SQLite `0007_message_steps.sql` migration'ıydı; depolama dosya-tabanlıya taşınınca alana dönüştü — bkz. `08-DEPOLAMA.md`.)
- [x] `internal/api/chat.go`: yanıt `steps` döndürür ve izi mesaja yazar.
- [x] `internal/api/files.go` (yeni): `GET /api/files?path=` — inline görsel için salt-okunur akış (görsel uzantı allowlist'i).

**Frontend** (yeni bağımlılıklar: react-markdown, remark-gfm, highlight.js)
- [x] `components/markdown/`: `Markdown.tsx` (GFM, özel kod/link/görsel render, `urlTransform` kapalı → yerel `C:\` yolları korunur, Windows ters-bölü normalize), `CodeBlock.tsx` (dil etiketi + kopya + highlight.js, diff→DiffView), `DiffView.tsx` (+N/−M satır renkli).
- [x] `components/chat/`: `TurnSteps.tsx` (iz çizimi + parseSteps), `ThinkingBlock.tsx` (açılır akıl yürütme), `ActivityCard.tsx` (tool kartı: ikon+etiket+niyet, açınca girdi/çıktı, Edit/Write→diff, hata kırmızı), `PathText.tsx` (metindeki yolları tıklanır çip).
- [x] `lib/`: `tools.ts` (tool meta), `paths.ts` (yol tespiti/görsel url), `diff.ts` (diff ayrıştırma).
- [x] `MessageList.tsx`: asistan turu ThinkingBlock → TurnSteps → markdown cevap; `App.tsx` `onOpenFile` (görsel→yeni sekme, diğer→pano); `index.css` `.sg-markdown` tipografisi + github-dark tema.

**CANLI TEST (Chrome DOM + API):**
- [x] Markdown: h2/liste/tablo, `go` kod bloğu highlight.js renkli span'larla, unified diff `+2 / −1` satır renkli — DOM doğrulandı.
- [x] Aktivite izi (DB'ye enjekte edilen örnek tur): düşünme bloğu, ara metin, Read/Edit/Bash tool kartları (tıklanabilir yol + kırmızı "hata" rozeti), Edit çıktısı diff olarak render.
- [x] Inline görsel: markdown `![](C:\…\hero.png)` → `src=/api/files?path=…`; uç **HTTP 200 image/png 13KB** döndürdü.
- [x] `go build ./...` + `tsc --noEmit` temiz.

**Anahtarsız adım izi (güncelleme):** `providers/claudecli.go` `--output-format stream-json --verbose`'a geçirildi; CLI'ın kendi olay akışı (`tool_use`/`tool_result`/`thinking`/`text`) ayrıştırılıp `Response.Trace` → `traceToSteps` ile `[]TurnStep`'e çevrilir. Böylece **API anahtarı olmadan** da tool kartları + ara adımlar görünür. Canlı test: claude-cli ajanı "Bash ile `echo hello-from-tionswarm`" → yanıt `steps`=[text, tool(Bash, output=hello-from-tionswarm)]; Chrome DOM'da ▶️ Bash kartı GIRDI/ÇIKTI ile render oldu. Native (anthropic) yol da kendi izini üretmeye devam eder.

---

### Faz 7 — Orchestration ✅ (2026-06-15)

Yapılandırılmış **çok-ajanlı akışlar**: bir akış = node grafiği (agent / branch / parallel).
Şablon sistemi + **restart-safe run state** (her node sonrası DB'ye yazılır, çökme sonrası kaldığı yerden devam).

**1) Graf modeli + motor (`internal/orchestration`)**
- [x] `model.go`: `Graph`/`Node`/`Branch`; node tipleri `agent`/`branch`/`parallel`; `Validate` (start/uniq/ref/agent kontrolleri)
- [x] `engine.go`: `Engine.Run` — start'tan yürür; `agent` node `AgentRunner` ile çalışır, çıktı saklanır; `branch` son çıktıya göre yönlendirir (case-insensitive substring, boş = varsayılan); `parallel` çocukları goroutine ile eşzamanlı koşar + join birleştirir; `maxSteps=50` döngü guard; her node sonrası `SaveFunc` ile state persist
- [x] Şablon: `{{input}}`, `{{last}}`, `{{node.<id>}}` prompt yer tutucuları

**2) DB (migration `0006_flows.sql`)**
- [x] `flows` (graph JSON) + `flow_runs` (restart-safe `state` JSON, status, input, output, error)
- [x] `models_flow.go` + `store_flow.go`: Flow CRUD + FlowRun (Create/Get/List/SetState/Finish + `ListRunningFlowRuns`)

**3) Agent entegrasyonu (`internal/agent/flow.go`)**
- [x] `flowRunner` → `orchestration.AgentRunner`: her node ajanın tam hattından geçer (memory recall + tools + budget via `complete`)
- [x] `RunFlow` (yeni run aç + motoru sür) + `driveFlow` (per-node persist + terminal status)
- [x] `ResumeRunningFlows` (boot'ta `running` kalan run'ları persist state'ten devam ettirir) — workspace açılışına bağlandı (restart-safe)

**4) API + Frontend**
- [x] `api/flows.go`: GET/POST `/api/flows`, PUT/DELETE `/api/flows/{id}`, POST `/api/flows/{id}/run`, GET `/api/flow-runs`, GET `/api/flow-runs/{id}`
- [x] `components/FlowsPanel.tsx`: görsel protokol builder — akış listesi, node editörü (agent/branch/parallel form kartları, başlangıç seçimi, sonraki/dal/paralel yönlendirme), çalıştır + trace görüntüleyici; NavRail "🔀 Akışlar"

**CANLI TEST (API + Chrome):**
- [x] **Flow A (agent→branch→agent)** "Sentiment Router": pozitif girdi → n1 "POSITIVE" → branch doğru şekilde n3 (Cheerful) → neşeli yanıt; trace state'e kalıcı yazıldı
- [x] **Flow B (parallel→join→agent)** "Pros & Cons": a1+a2 eşzamanlı koştu, çıktılar `[Advantage]…[Disadvantage]…` birleşti, s1 birleşik `{{last}}`'i özetledi
- [x] Chrome (DOM): "🔀 Akışlar" sekmesi; iki akış listelendi; node editörü 4 node'u tip/ajan/yönlendirme ile render etti; çalıştır bölümü göründü

> Bilinen sınır: `branch` substring eşleşmesi LLM çıktısının temizliğine duyarlı — claude-cli gevezelik ekleyince ("POSITIVE? No. NEGATIVE") yanlış dal seçilebilir. Motor doğru; sınıflandırma node'larında prompt katı tutulmalı veya varsayılan dal dikkatli sıralanmalı.

---

## ESKİ: FAZ 8 MCP + ARAÇLAR TAMAMLANDI ✅ (CANLI TEST GEÇTİ)

### Faz 8 — Tool-use + MCP ✅ (2026-06-15)

Ajanlara **araç kullanımı** kazandırıldı: hem yerleşik (built-in) hem **MCP sunucu** araçları.
Desen: built-in + MCP tek katalogda, ajan başına atanır. İki yol birlikte kuruldu.

**1) Provider native tool-use protokolü (gerçek motor)**
- [x] `providers/provider.go`: `ToolDef`/`ToolCall`/`ToolResult`; `Request.Tools`, `Message.ToolCalls`/`ToolResults`, `Response.ToolCalls`+`StopReason`
- [x] `providers/anthropic.go`: content-block modeli (text/tool_use/tool_result); `tools` gönderimi + `tool_use` ayrıştırma
- [x] `agent/toolloop.go`: `CompleteWithTools` — native agentic döngü (provider → tool çalıştır → tool_result → tekrar, `maxToolIters=8`), bütçe guardrail entegre

**2) claude-cli MCP delegasyonu (anahtarsız, canlı test edilen yol)**
- [x] `providers/claudecli.go`: `ConfigureMCP` — `--mcp-config` + `--strict-mcp-config` + `--allowedTools`; CLI tool döngüsünü kendi içinde çalıştırır (API anahtarı gerekmez)
- [x] etkin MCP sunucularından geçici `--mcp-config` JSON üretimi + temizleme

**3) MCP istemcisi (`internal/mcp`) — SDK'sız elle JSON-RPC 2.0**
- [x] `client.go`: stdio; `initialize` → `notifications/initialized` → `tools/list` / `tools/call`
- [x] `manager.go`: `ServerConfig`, namespace (`<server>__<tool>`), `BuildCatalog`, `CallNamespaced`, `ListServerTools`; sse/http → net "henüz desteklenmiyor"

**4) Tool kayıt defteri (`internal/tools`)**
- [x] `registry.go`: `Tool` arayüzü + birleşik `Registry` (built-in + MCP); `Defs(allow)` allowlist, `Call` (hata → IsError)
- [x] Built-in: `get_current_time`, `http_get` (64KB sınır), `memory_recall`

**5) DB (migration `0005_mcp_tools.sql`)**
- [x] `mcp_servers` += `command`/`args`/`url`/`enabled`/`scope`; `agents` += `mcp_enabled`/`allowed_tools`
- [x] `models_mcp.go` + `store_mcp.go`: CRUD + `UpdateAgentTools`

**6) API + Frontend**
- [x] `api/mcp.go`: GET/POST `/api/mcp-servers`, `/toggle`, `/test`, DELETE
- [x] `api/agent_tools.go`: GET/POST `/api/agents/{id}/tools` (mcpEnabled + allowlist + canlı katalog)
- [x] chat/executor/heartbeat → `CompleteWithTools` (usage tek yerde)
- [x] `components/ToolsPanel.tsx` + NavRail "🔌 Araçlar" + types/api

**CANLI TEST (Go test + API + Chrome):**
- [x] `internal/mcp` live test: gerçek `@modelcontextprotocol/server-filesystem` (npx) → 14 araç, `list_directory` → `[FILE] note.txt`
- [x] API: MCP oluştur → `/test` ok=true 14 araç; ajan tools aç → katalog **17 araç** (14 MCP + 3 built-in)
- [x] API: claude-cli delegasyonu — "note.txt oku" → araçla okudu, **"hello from tionswarm"** (BOM + CRLF dahil → gerçekten araçla, tahmin değil)
- [x] Chrome (DOM): "🔌 Araçlar" sekmesi; 17 araç tam açıklamayla; filesystem MCP sunucusu Test/Kapat/Sil; ekleme formu render

> Not: `chrome_screenshot` odaktaki başka sekmeyi yakaladı (bilinen mcp-chrome sorunu); doğrulama DOM (`chrome_get_web_content`) ile yapıldı.

---

## ESKİ: Otomatik başlık üretimi (auto-title) ✅ (2026-06-15)

Görev ve sohbetlere otomatik başlık üreten ortak bir sistem eklendi. Kanban'da artık
yalnızca prompt girilir; başlık prompt'tan üretilir. Sohbette ilk mesaj gönderilince
başlık otomatik oluşur. Her ikisi de istenildiğinde ⟳ ile yeniden üretilebilir.

**Backend**
- [x] `internal/agent/titler.go` (yeni): `GenerateTitle` (ajan provider'ı ile kısa başlık),
  `TitleFor` (tercih edilen ajan yoksa ilk ajana düşer; üretim hatasında prompt'tan
  `FallbackTitle`), `SanitizeTitle` (tek satır, tırnak/noktalama temizliği, 60 karakter sınırı).
  Başlık talimatı hem system hem **user turn** içine gömülü — claude-cli'ın büyük taban
  promptunun appended system'i bastırmasını önler.
- [x] `db/store.go`: `SetSessionTitle`.
- [x] `api/chat.go`: ilk turda (kind=chat, başlık boş, messageCount=0) başlık otomatik üretilir,
  `sessionTitle` alanı ile döner; best-effort (hata yanıtı bozmaz).
- [x] `api/sessions.go`: `POST /api/sessions/{id}/title` — sohbet geçmişinden (veya `source`) yeniden üret.
- [x] `api/tasks.go`: görev oluşturmada `title` opsiyonel (prompt'tan üretilir); `POST /api/tasks/{id}/title` yeniden üret.

**Frontend**
- [x] `types.ts`/`api.ts`: `ChatResponse.sessionTitle`; `generateSessionTitle`, `generateTaskTitle`; `createTask.title` opsiyonel.
- [x] `TaskBoard.tsx`: form'dan başlık alanı kaldırıldı (yalnız prompt + ajan); her kartta ⟳ "başlığı yeniden oluştur".
- [x] `Sidebar.tsx`: her oturumda hover'da ⟳ başlık yenileme.
- [x] `App.tsx`: chat yanıtındaki `sessionTitle` ile oturum başlığı güncellenir; `regenerateSessionTitle`.

**CANLI TEST (API):**
- [x] Görev: yalnız prompt → başlık "Veritabanı yedekleme cron görevi kurulumu" otomatik üretildi.
- [x] Görev yeniden başlık: ⟳ → "Veritabanı yedekleme gece cron ve hata raporu".
- [x] Sohbet: ilk mesaj → `sessionTitle` "Python liste tuple farkı", DB'ye yazıldı.
- [x] Frontend: `tsc -b && vite build` temiz; uygulama tarayıcıda render oldu (DOM doğrulandı).
- Not: Chrome görsel testi 80+ sekmeli ortamda kararsız (image readback) — doğrulama API + DOM metni üzerinden yapıldı.

### UI yeniden düzenleme — 3 kolonlu yerleşim ✅ (2026-06-15)
- [x] `components/NavRail.tsx` (yeni): en solda **daraltılabilir** nav rail — marka + workspace switcher + görünüm geçişleri (💬 Sohbet · 🗂 Görevler · ⏰ Zamanlamalar · ⛁ Hafıza). Daraltınca ikon-only (w-14 ↔ w-52); tercih `localStorage`'da.
- [x] `components/Sidebar.tsx`: artık orta kolon — yalnızca Ajanlar + Oturumlar listesi (logo/workspace NavRail'e taşındı). **Yalnızca ajan-bazlı görünümlerde (Sohbet, Hafıza) gösterilir;** Görevler/Zamanlamalar workspace-bazlı olduğundan orada gizli.
- [x] `App.tsx`: `NavRail → Sidebar → main` 3 kolon; görünüm sekmeleri header'dan NavRail'e taşındı, header artık görünüm başlığı + (chat'te) meter'ları gösterir.
- [x] **Chrome canlı test:** 3 kolon render, daralt/genişlet çalışıyor, meter'lar header'da.



### Faz 6.5 — Sağlamlaştırma ✅ (2026-06-15)

Kıyaslamada öne çıkan iki kritik açık kapatıldı: **bağlam (context) yönetimi** ve **otonom döngü maliyet guardrail'i**.

**1) Bağlam yönetimi + compaction (`internal/conversation`)**
- [x] `tokens.go`: tokenizer-bağımsız token tahmini (~4 char/token)
- [x] `manager.go`: `Manager.Prepare` — pending geçmiş bütçeyi aşınca eski turları **rolling summary**'ye katlar (compaction), sadece özet + son N tur gönderilir
- [x] Migration `0004`: `sessions.summary` + `summary_msg_count`; `db.SetSessionSummary`
- [x] `api/chat.go`: her turda Prepare çağrılır; özet sistem promptuna enjekte edilir; yanıt `contextTokens` + `compacted` döner
- [x] Env: `TIONSWARM_MAX_CONTEXT_TOKENS` (varsayılan 12000), `TIONSWARM_KEEP_RECENT_MSGS` (8)
- [x] **Önceki bug:** chat her turda TÜM geçmişi gönderiyordu → uzun oturumda context taşması + artan maliyet. Artık sınırlı.

**2) Maliyet / bütçe guardrail'i (otonom döngü)**
- [x] Migration `0004`: `agents.daily_call_limit` + `daily_token_limit` (0=sınırsız); `agent_usage(agent_id, day, calls, in/out tokens)` tablosu
- [x] `db/store_usage.go`: `AddUsage` (upsert), `GetUsageToday`, `UpdateBudget`
- [x] `agent/budget.go`: `guardedComplete` — **tek provider çağrı hunisi**; otonom çağrıda `ensureBudget` (limit aşılırsa `ErrBudgetExceeded`, provider'a gitmeden), her çağrıda usage kaydı
- [x] Tüm otonom yollar guardedComplete'e bağlandı: heartbeat (otonom), scheduler prompt/task (otonom), reflect (kullanıcı tetikli=false ama usage kaydı var). Manuel chat + run-now budget'a takılmaz.
- [x] API: GET `/api/agents/{id}/usage`, POST `/api/agents/{id}/budget`, GET `/api/sessions/{id}/context`

**Frontend**
- [x] `components/ChatMeters.tsx`: chat başlığında **⛁ context meter** (token + ⧉ özet göstergesi) ve **◷ bütçe meter** (bugünkü çağrı/limit, tıkla→limit ayarla)
- [x] `App.tsx`: chat header'a meter'lar; her turdan sonra `meterRefresh`

**CANLI TEST (API + Chrome):**
- [x] Compaction: maxctx=120 ile 4 mesaj → 4.'te `compacted=true`, summary_msg_count=5, özet kalıcı bilgileri yakaladı (Go, Türkiye, oyun, SQLite)
- [x] Budget: limit=4 (mevcut kullanım) → heartbeat wake → "failure: daily budget exceeded", **token harcanmadı** (provider'a gitmeden bloklandı); manuel chat etkilenmedi
- [x] Chrome: chat başlığında ⛁ 362 ⧉ ve ◷ 4 çağrı göstergeleri render edildi

> Not: claude-cli compaction özetine kendi persona tonunu katıyor (anthropic provider'da daha temiz). İşlevsel olarak doğru.

---

## ESKİ: FAZ 6 TAMAMLANDI ✅ (CHROME'DA CANLI TEST GEÇTİ)

### Faz 6 — Memory ✅ (2026-06-15)

Ajan hafızası: belge + günlük (journal) + yansıma (reflection). Recall **saf Go sözcüksel benzerlik** (token-frekans cosine) — harici embedding API yok, CGO yok, çevrimdışı. Tüm anılar workspace-scoped.

**Mimari karar:** Anahtarsız/CGO-free felsefeye uygun olarak gerçek semantik embedding yerine lexical cosine kullanıldı; term-vektörü `knowledge_sources.embedding` BLOB'unda cache'lenir. İleride aynı `memory.Store` arkasına gerçek embedder takılabilir (`0001`'deki şema yeterli, migration gerekmedi).

**Memory paketi (`internal/memory`)**
- [x] `vector.go`: Unicode-aware tokenizasyon (TR+EN stopword), tf-vektör, cosine, JSON marshal/unmarshal
- [x] `memory.go`: `Store` (db sarmalayıcı) — Remember / Recall (top-N, minScore eşiği) / List / Delete / **ContextBlock** (sistem promptuna enjekte edilecek blok)

**DB**
- [x] `models_memory.go`: `KnowledgeSource` + kind sabitleri (document/journal/reflection)
- [x] `store_memory.go`: CreateKnowledge / GetKnowledge / ListKnowledge(kind filtreli) / DeleteKnowledge + `placeholders` helper

**Agent paketi**
- [x] `runtime.go`: Runtime'a `mem *memory.Store` alanı + `Memory()` erişimcisi
- [x] `reflector.go`: `Journal` (aktiviteyi kaydet) + `Reflect` (dream cycle — son ~20 journal'ı provider'a özetletip reflection olarak sakla)
- [x] `executor.go`: `invokeWithMemory` (recall→sistem promptuna enjekte) + başarılı task sonrası journal; `complete` ortak helper

**Enjeksiyon noktaları**
- [x] `api/chat.go`: her turda recall→enjekte + tur sonrası journal
- [x] `agent/executor.go`: task çalıştırmada recall→enjekte + journal

**API uçları**
- [x] GET/POST `/api/agents/{id}/memories` (liste, ?kind= filtreli / belge ekle)
- [x] POST `/api/agents/{id}/reflect` (yansıma üret)
- [x] POST `/api/agents/{id}/recall` (recall önizleme — skorlu, debug)
- [x] DELETE `/api/memories/{id}`

**Frontend**
- [x] `types.ts`/`api.ts`: Memory/RecallHit + uç metodları
- [x] `components/MemoryPanel.tsx`: belge ekle, ✦ Yansıt, tür filtreleri (Tümü/Belgeler/Günlük/Yansımalar), rozetli liste, sil
- [x] `App.tsx`: 4. görünüm "Hafıza" (aktif ajana göre)

**CANLI TEST (API + Chrome):**
- [x] API: 3 belge eklendi; recall "hangi veritabani" → SQLite anısı skor 0.236 ile döndü (diğerleri eşik altı)
- [x] API: chat "hangi veritabani?" → ajan enjekte edilen anıdan **"SQLite"** cevapladı (başka bilme yolu yok); tur journal'landı
- [x] API: reflect → ajan journal üzerine birinci şahıs yansıma yazdı, kullanıcının kısa cevap tercihini bile not etti
- [x] Chrome: Hafıza sekmesinde belge/günlük/yansıma rozetli olarak göründü

---

## ESKİ: FAZ 5 TAMAMLANDI ✅ (CHROME'DA CANLI TEST GEÇTİ)

### Faz 5 — Tasks + Schedules ✅ (2026-06-15)

Görev panosu (kanban) + cron zamanlama. Hepsi **workspace-scoped** (her workspace kendi scheduler'ı).

**DB (migration `0003_tasks_schedules.sql`)**
- [x] `tasks`/`schedules`/`runs` zaten 0001'de vardı; Faz 5 kolonları eklendi: tasks → `prompt`, `last_run_id/status/at`; schedules → `task_id`, `prompt`, `last_run_at`; runs → `output`, `trigger` + indeksler
- [x] `internal/db/models_task.go`: Task / Schedule / Run tipleri + board state sabitleri + `ValidBoardState`
- [x] `internal/db/store_task.go`: CreateTask/GetTask/ListTasks/UpdateTask/MoveTask/SetTaskLastRun/DeleteTask + `nullable`/`mustAffect` helper'ları
- [x] `internal/db/store_run.go`: CreateRun/FinishRun/ListRuns/GetRun
- [x] `internal/db/store_schedule.go`: CRUD + ListEnabledSchedules + SetScheduleEnabled/Delivery + GetOrCreateKindSession

**Çalıştırma + zamanlama (agent paketi)**
- [x] `internal/agent/executor.go`: `Runtime.RunTask` — run aç → board `in_progress` → provider çağrısı → çıktı + done/failed; `invoke` helper'ı (tek prompt)
- [x] `internal/agent/scheduler.go`: `robfig/cron/v3` ile workspace başına Scheduler; Start/Reload/Stop; fire → task çalıştır veya prompt teslim et; `nextRunAt` senkronu; geçersiz cron yakalama
- [x] `internal/workspace/manager.go`: Workspace'e `Scheduler` alanı; open'da Start, Delete/Close'da Stop

**API uçları**
- [x] `internal/api/tasks.go`: GET/POST `/api/tasks`, PUT/DELETE `/api/tasks/{id}`, POST `/api/tasks/{id}/run`, GET `/api/tasks/{id}/runs`
- [x] `internal/api/schedules.go`: GET/POST `/api/schedules`, POST `/api/schedules/{id}/toggle`, DELETE `/api/schedules/{id}` (her yazımda Scheduler.Reload)

**Frontend**
- [x] `types.ts`/`api.ts`: Task/Run/Schedule tipleri + tüm uç metodları
- [x] `components/TaskBoard.tsx`: 5 sütunlu kanban, HTML5 sürükle-bırak ile taşıma, görev oluştur, ▶ Çalıştır, run geçmişi, sil
- [x] `components/Schedules.tsx`: cron preset'leri + serbest cron, görev/prompt seçimi, aç-kapa toggle, sil, sonraki/son çalışma
- [x] `App.tsx`: üstte Sohbet / Görevler / Zamanlamalar görünüm değiştirici

**CANLI TEST (API + Chrome):**
- [x] API: ajan→görev→çalıştır (claude-cli "Merhaba!"), board done'a geçti, run geçmişi yazıldı
- [x] API: cron `*/1 * * * *` schedule gerçekten tetiklendi, ajan prompt'a cevap verdi, schedule oturumuna kaydedildi
- [x] Chrome: Görevler sekmesinde form ile görev oluşturuldu → ▶ Çalıştır → claude-cli "4" → kart **Bitti** sütununa otomatik geçti, yeşil `● success` rozeti
- [x] Chrome: Zamanlamalar görünümü cron preset'leriyle render edildi

> Not: Cron 5-alanlı standart format (dk sa gün ay haftagünü). Çalıştırma claude-cli üzerinden anahtarsız.

---

## ESKİ: WORKSPACE İZOLASYONU TAMAMLANDI ✅ (CHROME'DA CANLI TEST GEÇTİ)

### Ara Faz — Workspace İzolasyonu ✅ (kullanıcı talebiyle Faz 5 öncesi eklendi)
Detaylı tasarım: [06-WORKSPACES.md](06-WORKSPACES.md)

- [x] `internal/workspace/manager.go`: her workspace = ayrı DB + ayrı runtime; workspaces.json kayıt defteri; Create/Delete/Get/List/Default/Close
- [x] `internal/api/server.go`: `withWorkspace` middleware (X-Workspace-Id header → context); workspace CRUD uçları
- [x] Tüm handler'lar workspace-scoped (`ws(r).DB`, `ws(r).Runtime`)
- [x] `main.go`: tek db/runtime yerine Manager
- [x] Frontend: `WorkspaceSwitcher.tsx`, api.ts header injection + localStorage, App.tsx workspace state + geçişte tam reset
- [x] **CANLI TEST (API + Chrome):** WS1=Ajan-A, WS2=Ajan-B; her workspace yalnızca kendi verisini görüyor; UI switcher ile geçiş çalışıyor; ayrı .db dosyaları ✅

### Mimari karar
Tek DB + workspace_id kolonu yerine **fiziksel ayrım** (workspace başına ayrı tionswarm.db) → sıfır sızıntı riski.

---

## ESKİ: Faz 4 TAMAMLANDI ✅ (OTONOM HEARTBEAT CANLI TEST GEÇTİ)

### Faz 4 — Agent Runtime ✅
- [x] Migration `0002_heartbeat.sql`: agents'a heartbeat_enabled/interval/prompt sütunları
- [x] `internal/db/store.go`: scanAgent/scanSession refactor, UpdateHeartbeat, GetOrCreateHeartbeatSession
- [x] `internal/agent/worker.go`: per-agent goroutine, heartbeat ticker, wake/stop channel, exponential backoff, 10 hatada otomatik devre dışı
- [x] `internal/agent/runtime.go`: yaşam döngüsü yöneticisi (Start/Stop/Wake/Status/StartConfigured/StopAll) + heartbeat eylemi (LLM çağrısı → heartbeat oturumuna yaz)
- [x] `internal/api/runtime.go`: GET /api/runtime, POST /api/agents/{id}/heartbeat, POST /api/agents/{id}/wake
- [x] `main.go`: boot'ta StartConfigured, shutdown'da StopAll
- [x] **CANLI TEST:** ajan 5sn aralıkla kendi kendine uyandı, Claude'u çağırdı, "Sen yapabilirsin, asla pes etme!" cevabını heartbeat oturumuna yazdı; runtime durumu success ✅

### Yeni API uçları
| Metod | Yol | Açıklama |
|-------|-----|----------|
| GET | `/api/runtime` | Tüm worker'ların durumu |
| POST | `/api/agents/{id}/heartbeat` | Heartbeat aç/kapat + ayarla |
| POST | `/api/agents/{id}/wake` | Anlık uyandır |

### Runtime mekanikleri
- Her ajan = 1 goroutine; kontrol kanalları (wake/stop) ile yönetilir
- Outcome classification: success → failures sıfırlanır; failure → artar, backoff uygulanır
- Exponential backoff: `base * 2^failures` (maxBackoffSteps ile sınırlı)
- 10 ardışık hata → ajan otomatik devre dışı (Disabled=true)

### ⏳ Sonraki (Faz 4 artıkları, opsiyonel)
- [ ] Heartbeat durumunu UI'da göster (runtime paneli)
- [ ] claude-cli için araç kısıtlama (`--allowedTools ""`) — saf chat güvenliği

---

## ESKİ: Faz 3 TAMAMLANDI ✅ (CHROME'DA CANLI TEST GEÇTİ)

### Faz 3 — React Web UI ✅
- [x] `frontend/`: Vite + React + TypeScript + Tailwind v4 kuruldu
- [x] `vite.config.ts`: Tailwind eklentisi + backend proxy (:8090)
- [x] `src/index.css`: özel koyu tema (@theme değişkenleri)
- [x] `src/types.ts`, `src/api.ts`: tip güvenli API istemcisi
- [x] `src/components/Sidebar.tsx`: ajan listesi + oluşturma formu + oturumlar
- [x] `src/components/MessageList.tsx`: mesaj balonları + typing animasyonu + auto-scroll
- [x] `src/components/Composer.tsx`: mesaj girişi (Enter ile gönder)
- [x] `src/App.tsx`: durum yönetimi, optimistic UI, hata gösterimi
- [x] `npm run build`: TypeScript temiz derlendi
- [x] **CHROME CANLI TEST:** ajan seç → geçmiş yüklendi → yeni mesaj "3+7" → cevap "10." ✅
- [x] Türkçe karakterler UI'da kusursuz (PowerShell mojibake'si sadece konsoldaymış)

### Çalıştırma (geliştirme)
```powershell
# Terminal 1 - backend
$env:TIONSWARM_ADDR=":8090"; go run ./cmd/tionswarm
# Terminal 2 - frontend
cd frontend; npm run dev   # http://localhost:5173
```

### ⏳ Sonraki iyileştirmeler
- [ ] Streaming (WebSocket ile token token akış)
- [ ] Frontend build'i Go binary'sine embed (tek dosya dağıtım)

---

## ESKİ: Faz 2 TAMAMLANDI ✅ (CANLI TEST GEÇTİ)

### 🎉 Önemli: API anahtarı OLMADAN çalışıyor (claude-cli provider)
"CLI provider" yaklaşımı eklendi. Yerel `claude` CLI'ı
kullanıcının OAuth/abonelik girişiyle çalışır — **API anahtarı gerekmez.**

- [x] `internal/providers/claudecli.go`: `claude -p --output-format json` ile shell-out
- [x] `internal/providers/registry.go`: claude CLI otomatik tespit (exec.LookPath)
- [x] Yeni ajanların varsayılan provider'ı: `claude-cli`
- [x] **CANLI TEST:** Çok turlu sohbet çalıştı, hafıza korundu, mesajlar DB'ye yazıldı ✅
- [x] Çözülen sorun: `--bare` flag'i keychain okumasını atlayıp "Not logged in" veriyordu → kaldırıldı

### İki provider seçeneği (ajan başına seçilebilir)
| Provider | Auth | Maliyet | Persona kontrolü |
|----------|------|---------|------------------|
| `claude-cli` | OAuth/abonelik (anahtarsız) | Abonelik dahili | Claude Code kimliği taban + append |
| `anthropic` | API anahtarı | Token başına ücret | Tam temiz kontrol |

### ⚠️ Bilinen kısıtlar / sonraki iyileştirmeler
- claude-cli, Claude Code'un taban sistem prompt'unu (≈23k token) yükler → her çağrı bu yükü taşır
- claude-cli print modunda araç (Bash/Edit) erişimi olabilir → saf chat için `--allowedTools ""` ile kısıtlanmalı
- Streaming henüz yok (Faz 3'te WebSocket)

---

## ESKİ: Faz 2 KOD TAMAM

### Faz 2 — Anthropic Provider + Chat MVP ✅ (kod)
- [x] `internal/db/models.go`: Agent, Session, Message tipleri
- [x] `internal/db/store.go`: CRUD (CreateAgent/GetAgent/ListAgents, sessions, messages, transaction'lı AddMessage)
- [x] `internal/providers/provider.go`: ortak `Provider` arayüzü + Message/Request/Response
- [x] `internal/providers/anthropic.go`: ince HTTP istemci (Messages API, SDK'sız)
- [x] `internal/providers/registry.go`: sağlayıcı seçimi (anthropic)
- [x] `internal/api/server.go`: router + CORS + health
- [x] `internal/api/agents.go`, `sessions.go`, `chat.go`: HTTP handler'ları
- [x] `main.go` API sunucusuyla bağlandı
- [x] Test: ajan/oturum/mesaj akışı ✅; chat anahtarsız güvenli hata veriyor ✅

### ⏳ Açık iş
- [ ] **Canlı chat testi:** `ANTHROPIC_API_KEY` verilince gerçek Claude cevabı doğrulanacak
- [ ] Streaming (Faz 3'te WebSocket ile)

### API Uçları (mevcut)
| Metod | Yol | Açıklama |
|-------|-----|----------|
| GET | `/health` | Sağlık + db |
| GET/POST | `/api/agents` | Ajan listele/oluştur |
| GET/POST | `/api/sessions` | Oturum listele/oluştur |
| GET | `/api/sessions/{id}/messages` | Mesaj geçmişi |
| POST | `/api/chat` | Sohbet turu (Claude) |

---

## ESKİ: Faz 1 TAMAMLANDI ✅

### Faz 1 — Veritabanı ve Config ✅
- [x] `internal/config/config.go`: env okuma, dizin çözümleme, DBPath
- [x] `internal/config/secret.go`: AES-GCM credential şifreleme (env→dosya→üretim)
- [x] `internal/db/db.go`: SQLite (modernc.org/sqlite, CGO yok), WAL + busy_timeout + FK
- [x] `internal/db/migrate.go`: embed.FS migration runner + schema_migrations takibi
- [x] `internal/db/migrations/0001_init.sql`: 11 tablo (agents, sessions, messages, tasks, schedules, runs, connectors, knowledge_sources, skills, mcp_servers, provider_configs)
- [x] `main.go` config+db ile bağlandı; `/health` artık `db:true` döndürüyor
- [x] `.gitignore` eklendi
- [x] Test: DB oluştu, migrate geçti, health `db:true` ✅

---

## ESKİ: Faz 0 TAMAMLANDI ✅

### Tamamlananlar ✅
- [x] Go 1.26.4 kuruldu ve doğrulandı
- [x] Node.js v24 + npm 11 mevcut (frontend için hazır)
- [x] Proje klasör yapısı oluşturuldu (`C:\Users\user\Desktop\Projects\TionSwarm`)
- [x] `go mod init github.com/bilal/tionswarm`
- [x] `_Docs` plan dokümanları yazıldı (00-05)
- [x] `cmd/tionswarm/main.go`: HTTP sunucu + `/health` ucu (graceful shutdown, slog)
- [x] `go build` + `go vet` temiz
- [x] Çalıştırma testi: `/health` → `{"status":"ok"}` ✅
- [x] Kök `README.md` (Türkçe)

### Sıradaki Adımlar ⏳ (Faz 1)
- [ ] Wails v2 CLI kurulumu (Faz 9'a kadar ertelenebilir)
- [ ] `internal/db`: SQLite (modernc.org/sqlite) bağlantısı
- [ ] Migration runner + `0001_init.sql`
- [ ] `internal/config`: env + credential secret (AES-GCM)

### Sonraki Faz: Faz 1 (Veritabanı ve Config)
Detaylar için bkz. [03-YOL-HARITASI.md](03-YOL-HARITASI.md)

---

## Karar Bekleyen Konular

Kullanıcıyla netleştirilecek:
1. **İlk LLM sağlayıcısı:** Anthropic (Claude) mı, OpenAI mı, yoksa yerel Ollama mı?
2. **Frontend framework:** React (varsayılan) mı, Svelte mi?
3. **MVP kapsamı:** Tek ajan + chat ile mi başlayalım?

---

## Oturum Günlüğü

### 2026-06-18 — Lazy tool loading (3 faz: katalog + activate_tools + prune)
Araç şemalarına skill `subskills` deseninin araç karşılığı uygulandı (bkz. `19-LAZY-TOOL-LOADING.md`):
- **Faz 1** — `providers.ToolDef.Lazy` alanı; `tools.Registry` lazy seti (`MarkLazy`, `Add`, `ActiveDefs`, `LazyCatalog`). **Self-management suite + tüm MCP araçları lazy** (`AttachMCP` MCP'yi otomatik lazy işaretler), çekirdek araçlar eager. Sistem promptuna **"Available Tools (load on demand)"** bloğu (`Runtime.LazyToolsCatalogBlock`, `chat_turn.go`'da skills'ten sonra enjekte). Eager meta-araç **`activate_tools`** + per-turn **aktif set** (`tools.ActiveTools`, context ile `buildRegistry`'ye taşınır). Tool loop her iterasyonda `ActiveDefs(filter, active.Snapshot())` ile gönderilen şemayı yeniden hesaplar.
- **Faz 2** — **`deactivate_tools`** + **`find_tools`** (katalog anahtar-kelime arama). `internal/tools/builtin_activate.go`.
- **Faz 3** — `ActiveTools.Prune` + `activeToolMaxIdle=3`: kullanılmayan aktif araçlar uzun turda düşürülür (loop `MarkUsed`/`Prune` çağırır).
- Bağlam önizlemesi: gönderilen araçlar artık eager (`ShippedToolCatalog`); lazy'ler sistem bloğunda → dürüst token ayrımı.
- Testler: `lazyload_test.go` (`ActiveDefs`/`LazyCatalog`/`Prune`/`activate_tools`/`find_tools`). `go build ./...` + `go test ./...` temiz. `:8090` doğrulandı: agent context'te MCP araçları lazy blokta, eager listede yalnızca çekirdek + meta-araçlar (24 araç).

### 2026-06-18 — Skill `subskills` (progressive disclosure) + default app-flow skiller
Skill sistemine üç ekleme yapıldı:
- **`subskills:` frontmatter alanı** (`skill.go` `SubSkills`, `store.go` `scanDir` parse). Bir skill, daha detaylı alt skill'lerin slug'larını listeleyebiliyor. `use_skill` ile gövde yüklenince sonuna **"Related skills"** footer'ı ekleniyor (`Store.UseSkillBody` + `subskillFooter`): bilinmeyen/öz-referans/izinsiz slug'lar elenir, model gerektiğinde alt skill'i ayrıca `use_skill` ile yükler. Ham detay görünümü (`Body`) değişmeden kaldı; footer yalnızca araç yolunda. `agentSkillLib.Body` artık `UseSkillBody(slug, allow)` çağırıyor.
- **Default skill seeding** (`defaults.go` + `//go:embed defaults`): TionSwarm ile gelen baseline skill'ler global dizine (`~/.tionswarm/skills`) açılışta yazılıyor — `EnsureDefaults` idempotent, mevcut dosyayı **ezmez** (kullanıcı düzenlemesi ve access toggle korunur; silinen default bir sonraki başlangıçta geri gelir). `NewRuntime` içinde çağrılıyor → her workspace miras alır.
- **İki built-in skill**: `tionswarm-guide` (tüm uygulama akışı — agents/sessions/tasks/flows/schedules/skills/memory/MCP/secrets; `access: shared`, `subskills: [tionswarm-flows]`) ve `tionswarm-flows` (orkestrasyon detayları, shared). Progressive disclosure örneği: önce genel rehber, gerektiğinde flows detayı.
- Frontend `skill.ts`'e `subSkills?: string[]` eklendi.
- Testler: `TestUseSkillBodySubskillFooter`, `TestEnsureDefaultsSeeds` (ezme-yok + idempotent). `go build ./...` + `go test ./internal/skills/...` temiz; :8090 yeniden derlenip başlatıldı, `/api/skills` doğrulandı (guide: `shared=true`, `subSkills=[tionswarm-flows]`).
- **Plan**: built-in/MCP araçları için lazy yükleme tasarımı `19-LAZY-TOOL-LOADING.md`'ye yazıldı (skill progressive-disclosure deseninin araç karşılığı: hafif katalog + `activate_tools`).

### 2026-06-17 — Ekran yüksekliği/scroll fix (panel kökleri)
"Adım Türleri" (ve aynı kalıptaki diğer ekranlar) tarayıcı yüksekliğini aşıyordu: `<main>` (flex-col, header + panel) içinde panel kökleri `h-full` (= main'in TAM yüksekliği) kullanıyordu → header yüksekliği kadar taşıyor, iç scroll'un altı ekran dışına itiliyordu. Kök yükseklikleri **`min-h-0 flex-1`**'e çevrildi (yeni Artifacts/Tools/Skills panelleriyle aynı kalıp): `SettingsPanel`, `MemoryPanel`, `LogsPanel`, `FlowsPanel`, `TaskBoard`, `Schedules`, `AgentsView`; `SecretsPanel`+`MessageList`'e `min-h-0` eklendi. Artık header sabit, içerik panel içinde scroll. ✅ `tsc` temiz; Chrome canlı doğrulandı.

### 2026-06-17 — Ayarlar: "Gelişmiş" birleşik kategori
Ayarlar kategori rayı sadeleştirildi: **Bildirimler & Ekran + Otonomi + Otomatik Başlık + MCP & Araçlar + Tanılama** ayrı kategorileri tek **"Gelişmiş"** (`advanced`, `SlidersHorizontal` ikonu) alt-ekranında toplandı. `settings/primitives.tsx` `Cat` union + `APP_CATS` güncellendi (5 giriş → 1); `SettingsPanel.tsx` `advanced` branch'i beş paneli (`NotificationsPanel`/`AutonomyPanel`/`AutoTitlePanel`/`McpPanel`/`DiagnosticsPanel`) yeni `AdvSection` (alt-başlık + ayraç) sarmalı içinde dikey istifler; tek "Kaydet" hepsini kaydeder. Her `AdvSection` başlığında **accent-soft ikon rozeti** (Bell/Bot/Tag/Plug/Activity) — daha canlı görünüm. ✅ `tsc` temiz; Chrome canlı doğrulandı (rail tek "Gelişmiş", ekran 5 ikonlu bölüm). (Not: "Bağlam & Bellek" kısa süre Gelişmiş'e taşındı, ardından kullanıcı isteğiyle yine **ayrı sayfaya** alındı — Brain ikonlu standalone kategori.)

### 2026-06-17 — Tema tutarlılık denetimi (yeni özellikler sonrası)
Yeni gelen özellikler (Ajanlar/Artifactlar/Sırlar görünümleri, sessions sidebar bölme) tema açısından denetlendi; tespit edilen tutarsızlıklar giderildi (`go build`/`vet` + `tsc -b` temiz; Chrome canlı doğrulandı):

1. **Emoji → lucide-react** (uygulama geneli ikon dili birleştirildi): Ayarlar kategori rayı (`settings/primitives.tsx` — 15 kategori), sohbet adımları (`TextStep`/`DiffCard`/`RecoveryStep`/`SteerStep`/`ErrorStep`/`AskPrompt`/`TodoCard`/`ActivityCard` — ikon + ▾/▸ chevron'lar), `Composer` düşünme çipi (🧠→Brain), `ChatMeters` (⛁/⧉/◷→Database/Layers/Clock), `AgentRoster`/`SessionsSidebar`/`SessionDetailPanel` aksiyon menüleri (⚙/✏️/✨/📋/📂/🗑/↻→Settings/Pencil/Sparkles/ClipboardCopy/FolderOpen/Trash2/Loader2), `Schedules` (▶/✎/✕→Play/Pencil/X), `TaskDetailPanel` (⟳→RefreshCw), `MessageList` (🗑/✕), `PendingTray` (⏱/⏳/✕), `FlowsPanel` node etiketleri, `AgentSettingsForm` (🗑). MenuItem/ActionBtn/CatMeta tipleri `icon: LucideIcon` aldı.
2. **Yeşil aykırı buton düzeltildi:** `MemoryPanel` "Yansıt" `bg-emerald-600/80` dolgu → accent-outline + Sparkles ikonu (ekrandaki tek yeşil birincil buton sorunu).
3. **Semantic durum renkleri token'a bağlandı** (`--color-success/warning/danger`): başarı/uyarı/hata renkleri artık sabit Tailwind paleti yerine token kullanır — `TaskBoard`/`TaskDetailPanel` (status), `LogsPanel` (level), `Schedules`, `DiffView`/`DiffCard`/`ActivityCard` (+/−), `ErrorStep`/`RecoveryStep`, `StepKindsPanel`, `ProvidersPanel`, `ToolsPanel`, `ChatMeters`, `PendingTray`, `MemoryPanel`, `FlowsPanel`, `SessionsSidebar` (typing), `WorkspacePanel` (tehlike bölgesi), `App.tsx` (hata rozeti), `MessageList`/`SessionDetailPanel`/`AgentSettingsForm`/`ArtifactCard`/`ArtifactsPanel` (sil). Soft arka planlar `color-mix(... transparent)` ile.
4. **Elevation + empty state:** NavRail + kanban kartlarına `--shadow-sm`/hover `--shadow-md`; `ArtifactsPanel` boş durumu ikonlu hale getirildi.

> Not: Semantic durum renkleri (kırmızı=hata/yeşil=başarı) palet değişse de **sabit kalır** (status göstergesi) — token'a almak tek-noktadan ayar + tutarlılık içindir, accent paletinden bağımsızdır. Eşzamanlı diğer iş ile aynı ağaçta; commit kullanıcı onayına bırakıldı.

### 2026-06-16 — tool_delta/tombstone gerçek üreticileri (tool-streaming + iptal)
`tool_delta` ve `tombstone` artık altyapı değil, **canlı üreticili** (`go build`/`vet`/`test ./...` yeşil):

1. **`tools.StreamingTool`** arayüzü (`CallStream(ctx,input,onChunk)`) + `Registry.CanStream`/`CallStream` (`registry.go`). Akan araç yoksa düz `Call`'a düşer.
2. **`shell` aracı akıyor** (`builtin_shell.go`): `Call` → `CallStream`'e taşındı; `shellStreamWriter` stdout+stderr'i (tek writer, exec serialize eder) hem 64KB cap'li buffer'a yazar hem `onChunk`'a iletir.
3. **`toolloop.go` bağladı:** akan araç + canlı sink varsa `StepToolDelta` (`ID`=call.ID) yayılır; tamamlanınca `StepTombstone` (`Ref`=call.ID) placeholder'ı geri çeker; tool sonrası `ctx.Err()` varsa `fail("cancelled")` → `StepError` ve tur temiz biter (yarım sonuç modele beslenmez).
4. **Frontend** (geçen turda hazırdı): App.onStep `tool_delta`'yı `ID` ile merge, `tombstone`'u filtreler; `ToolDeltaStep.tsx` canlı çıktı kartı. `stepKinds.ts` durumları `infra`→`active`.
5. **Test:** `builtin_shell_test.go` — `CanStream("shell")` + `CallStream` onChunk parça + tam çıktı.

> Doküman: `10-KAVRAMSAL` E3 (artık tüm kind'lar üreticili) + SKILL güncellendi. Mekanizma genel: uzun MCP çağrıları da aynı `StreamingTool` yoluna takılabilir.

### 2026-06-16 — Trace StepKind genişletme #2: error/steer/tool_delta/tombstone + Ayarlar referans ekranı
4 yeni `StepKind` eklendi (`go build`/`vet`/`test ./...` + frontend `tsc`/`build` yeşil; canlı UI testi mcp-chrome stale-sekme/screenshot kırılganlığı nedeniyle güvenilir alınamadı, otomatik kontroller esas):

1. **`error`** (`StepError`) — tur düzeyinde hata (sağlayıcı/bütçe/iptal); `toolloop.go` `fail()` budget_exceeded/provider_error yollarında yayar → `ErrorStep.tsx` (kırmızı + reason rozeti). Araç hatasından (tool+isError) ayrı.
2. **`steer`** (`StepSteer`) — canlı yönlendirme artık `StepText` "↪" prefix'i yerine ayrı tip → `SteerStep.tsx`.
3. **`tool_delta`** (`StepToolDelta`) — akan tool çıktısı (aynı `ID` birleşir, yalnız-canlı) → `ToolDeltaStep.tsx`; App.onStep `id` ile merge eder. **Altyapı hazır, üretici yok** (tool-streaming gelince).
4. **`tombstone`** (`StepTombstone`) — `Ref` ile canlı bir adımı geri çeker (render edilmez); App.onStep filtreler. **Altyapı hazır.**
- `TurnStep`'e `ID`/`Ref` alanları. Tek-kaynak referans `frontend/src/lib/stepKinds.ts` (kind/etiket/ikon/kalıcı?/durum/açıklama) → **Ayarlar ▸ Adım Türleri** read-only ekranı (`SettingsPanel.tsx` yeni `stepkinds` kategorisi).
- Testler: `trace_test.go` error/steer/tool_delta/tombstone JSON round-trip.

> Doküman: `10-KAVRAMSAL` E3 + SKILL güncellendi.

### 2026-06-16 — UI tema yenileme (Design Refresh)
Arayüzün görsel dili cilalandı (`go build`/`vet` yeşil; backend `themePreset` kalıcılığı API round-trip ile, yeni tema + preset grid Chrome'da canlı doğrulandı):

1. **Token genişletme** (`frontend/src/index.css`): semantic renkler (`--color-success/warning/danger`), elevation (`--shadow-sm/md/lg`), `--radius`, klavye için tutarlı `:focus-visible` ring. Light tema bu token'ları kendi değerleriyle ezer. **Inter** (gövde) + **JetBrains Mono** (kod/yol) yerel `@fontsource-variable` fontları import edildi; `body::before` ile köşelerde `radial-gradient` + `color-mix` tabanlı **hafif accent glow**.
2. **Hazır tema paletleri** (`frontend/src/lib/themePresets.ts` — yeni): 8 küratörlü palet — Gece Moru (varsayılan), Arduvaz, Zümrüt, Gül, Kehribar, Nord, Gün Işığı, Solarized Açık. Her preset tam token seti (bg/surface/surface2/border/accent/accentSoft/text/textDim + opsiyonel semantic) + `dark` bayrağı taşır.
3. **`applyTheme` yeniden yazıldı** (`frontend/src/lib/theme.ts`): imza `applyTheme(theme, accent, preset)`. Preset seçiliyse tüm paleti `<html>` inline style'a basar (stylesheet'i ezer) ve dark/light'ı `data-theme`'den ayarlar; preset yoksa eski theme(dark/light/system)+accent yoluna düşer. Her iki yolda da boş olmayan `accent` accent token'ını override eder. `App.tsx` `applyClientPrefs` artık `themePreset`'i geçirir.
4. **Backend** (`internal/settings/settings.go` + `store.go`): `ThemePreset` alanı — Settings/DTO/Patch/Apply/Default(`midnight-violet`). `GET/PUT /api/settings` ile kalıcı (yeni uç yok, mevcut DTO genişletildi).
5. **SettingsPanel** (`frontend/src/components/SettingsPanel.tsx`): Görünüm sekmesine **swatch'lı palet seçici grid** (`grid-cols-2 sm:grid-cols-3`); bir preset tıklanınca `themePreset` + `accent` o paletin rengine senkronlanır. Eski tema dropdown'ı "Temel mod" olarak kaldı (preset yokken/sistem için).
6. **NavRail** (`frontend/src/components/NavRail.tsx`): emoji ikonlar → **lucide-react**; aktif öğe dolgu-mor yerine `accent-soft` tint + 3px sol indicator (`navItemClass`/`ActiveBar`); gradient marka rozeti (`color-mix`). *(Aynı dosya eşzamanlı "Ajanlar/Artifactlar görünümü" refactor'uyla `agents`/`artifacts` nav öğeleri eklenerek birleşti.)*

> Yeni bağımlılıklar: `lucide-react`, `@fontsource-variable/inter`, `@fontsource-variable/jetbrains-mono`. **Not:** Bu iş, eşzamanlı yürüyen "Ajanlar görünümü + Sessions sidebar bölme + Artifactlar" refactor'uyla aynı çalışma ağacında; o iş yarımken `tsc -b` geçici hata verir. Commit kullanıcı onayına bırakıldı.

### 2026-06-16 — Trace StepKind genişletme: `todo` + `recovery` (E3 düşük-efor)
`StepKind` ilk-sınıf hale getirildi (`go build`/`vet`/`test ./...` + frontend `tsc` yeşil):

1. **`todo` kind** (`agent/trace.go` `StepTodo` + `TurnStep.Todos []TodoItem` + `parseTodos`): `todo_write` aracı artık generic tool kartı yerine `StepTodo` adımı yayar (`toolloop.go` çağrı sonrası dönüştürür) → frontend `TodoCard.tsx` `step.todos`'u önceler (eski trace'ler için `step.input` fallback'i + tool-adı geriye-dönük render korunur).
2. **`recovery` kind** (`StepRecovery` + `TurnStep.Reason`): tool döngüsü iterasyon limitine (`maxToolIters`) ulaşınca `reason:"max_tool_iterations"` adımı yayılır → frontend `RecoveryStep.tsx` (amber uyarı satırı + reason rozeti). Tur neden erken bittiğini açıklar.
3. **Frontend:** `types.ts` `StepKind` union + `TodoItem` tipi; `TurnSteps.tsx` `todo`/`recovery` yönlendirmesi.
4. **Testler:** `agent/trace_test.go` (`parseTodos` geçerli/bozuk + todo/recovery JSON round-trip).

> Kalan StepKind adayları (sonraki): `subagent` (A2 ile), `tombstone`/`tool_delta` (canlı güncelleme altyapısı, P3). Bkz. `10-KAVRAMSAL-TASARIM-NOTLARI.md` E3.

### 2026-06-16 — Etkileşim araçları: `todo_write` + `ask_user` (E3/E2/E1)
`observed-behavior` mimari incelemesinden (`_Docs/10-KAVRAMSAL-TASARIM-NOTLARI.md`) çıkan **etkileşim katmanı** ilk iş paketi uygulandı (`go build`/`vet`/`test ./...` + frontend `tsc`/`build` + canlı tool-katalog smoke testi yeşil):

1. **E3 — Trace modeli genişletildi** (`internal/agent/trace.go`): yeni `StepAsk` (`"ask"`) kind'ı (delta gibi geçici, kalıcı değil) + `TurnStep.Options []string` (tıklanabilir öneri yanıtlar).
2. **E2 — `todo_write` aracı + UI** (`internal/tools/builtin_todo.go`): ajan tam görev listesini her seferinde yayınlar (`pending|in_progress|completed`); araç çağrısı yalnızca özet metin döner, listeyi frontend `TodoCard.tsx` canlı checklist olarak render eder (tool adına göre özel-durum, `TurnSteps.tsx`). Sunucu tarafında durum tutulmaz.
3. **E1 — `ask_user` (suspend/resume)** (`internal/tools/builtin_ask.go` + `ask.go`): ajan soruyu sorar ve **açık SSE stream üzerinden bloklar**. Tam suspend/resume yerine context-kanal deseni: `tools.WithAsker(ctx, fn)` → `chat_stream.go` geçici `StepAsk` yayar, `run.answer` kanalında bekler; kullanıcı `POST /api/chat/control {action:"answer"}` ile yanıtlar (`chat_control.go`). İnteraktif olmayan (heartbeat/scheduler) koşularda asker yok → araç hata döndürür, model kendi devam eder.
4. **Frontend:** `AskPrompt.tsx` (soru + tıklanabilir seçenekler + serbest metin) composer üstünde gösterilir; `App.tsx` `pendingAsk` state'i + `answerAsk` callback'i. `lib/tools.ts` ikonları (✅ todo_write, 💬 ask_user).
5. **Testler (yeni):** `internal/tools/builtin_interaction_test.go` — todo_write geçerli/geçersiz-durum/boş, ask_user asker-yok/asker-var/boş-soru.

> Doküman: `10-KAVRAMSAL-TASARIM-NOTLARI.md` güncellendi (B-Ek + D2-streaming "yapıldı" işaretlendi; E1/E2 tamamlandı). Kalan etkileşim işi: `ask_user` kalıcı tool kartı için özel render (şu an generic ActivityCard) ve `EnterPlanMode`/`ExitPlanMode` (plan modu).

### 2026-06-16 — Modülerlik refactor'ları (davranış değişmedi)
Dört adet düşük-riskli, davranış-korumalı refactor uygulandı (`go build`/`go vet`/`go test ./...` + canlı `/health` smoke testi yeşil):

1. **`providers/transport.go` (yeni):** Ortak `postJSON` HTTP yardımcısı. `anthropic.go` ve `minimax.go`'daki tekrar eden marshal → request → header → `Do` → `ReadAll` → unmarshal iskeleti tek noktaya alındı.
2. **`api/server.go` → `writeDBError`:** Her handler'da tekrar eden `db.ErrNotFound → 404 / diğer → 500` eşlemesi merkezîleştirildi; `agents/sessions/tasks/schedules/memory/usage` handler'ları sadeleşti. (`handleRunTask` 400 semantiği korunarak hariç tutuldu.)
3. **`agent/tunables.go` global state → `Tunables` struct:** Süreç-geneli paket globalleri kaldırıldı; tek `*Tunables` örneği `main.go`'da oluşturulup `NewManager`/`NewRuntime`/`NewServer` üzerinden enjekte ediliyor. `budget.go`/`titler.go` artık `r.tun` kullanıyor.
4. **`db/store.go` → `mutateAgentLocked`/`mutateSessionLocked`:** Kilitle→bul→değiştir→persist kalıbı generic yardımcılara alındı; `UpdateAgent`, `UpdateHeartbeat`, `UpdateBudget`, `UpdateAgentTools`, `SetSessionSummary`, `SetSessionTitle` sadeleşti.

> Not: `tunables.go` artık `Set*` yerine `*Tunables` metotları sunuyor (önceki satır 133'teki paket-fonksiyon imzaları değişti).

#### Devam refactor'ları (aynı gün)
5. **`agent/climcp.go` (yeni):** claude-cli `--mcp-config` üretimi (`cliMCPConfig`/`cliMCPServer` + `writeCLIMCPConfig`) `toolloop.go`'dan ayrı, kohezyonlu bir dosyaya taşındı. `ToolCatalog` registry'nin yanına (`toolsetup.go`) alındı. `toolloop.go` artık yalnızca completion/agentic-loop mantığına odaklı. (Saf kod taşıma — davranış değişmedi.)
6. **`api/server.go` → `register*Routes`:** Tek `Routes()` bloğu domain-bazlı 13 yardımcıya bölündü (`registerAgentRoutes`, `registerTaskRoutes`, …).
7. **Testler (yeni):** `providers/transport_test.go` (`postJSON`: başlık/gövde, non-200, decode hatası, transport hatası), `providers/minimax_test.go` (httptest ile `Complete` başarı/hata/varsayılan baseURL), `agent/tunables_test.go` (`*Tunables` get/set + eşzamanlı erişim), `api/server_test.go` (rota kaydı panik regresyon koruması).

> Repo hijyeni: `internal/` altında 36 kaynak dosyası henüz versiyon kontrolüne hiç girmemiş durumda (HEAD tek başına derlenmez). Tüm kaynak ağacını tek seferlik "track existing sources" commit'iyle eklemek önerilir.

### 2026-06-15
- Proje başlatıldı.
- Referans mimari analiz edildi, Go karşılıkları belirlendi.
- Klasör yapısı + go.mod + plan dokümanları oluşturuldu.
