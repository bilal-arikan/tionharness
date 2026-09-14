# Codex Plugin Desteği

> **Özet (2026-09-14):** `codex exec` turlarında Codex plugin'lerinin (skill, MCP
> sunucusu, hook taşıyan paketler) kullanılabilmesi. Workspace ayarı
> `codexPluginsEnabled` (varsayılan **açık**), marketplace + plugin listeleri
> workspace başına tutulur, her turun `config.toml`'una yazılır ve her kalıcı
> sohbet evine bir kez kurulur. Kutudan çıktığında hiçbir marketplace gelmez:
> kullanıcı ya kendi kaynağını ekler ya da makinesindeki Codex marketplace'lerini
> kopyalar. Durum: **uygulandı**. Bir ajan için: codex tarafında plugin/skill
> görünürlüğünü değiştirecek her iş burayı okumadan yapılmamalı — iki yarımın
> (config anahtarı + kurulu cache) ikisi birden gerekir.

## 1. Neden iki yarım birden gerekir

Codex 0.153.3 üzerinde ölçüldü. Bir plugin'in gerçekten etkin olması için
**ikisi birden** şart:

| Yarım | Nerede | Olmazsa |
|-------|--------|---------|
| Config anahtarları | `[marketplaces.<ad>]` + `[plugins."<seçici>"]` | `codex plugin list` "not installed" der, skill modele hiç ulaşmaz |
| Kurulu cache | `<CODEX_HOME>/plugins/cache/<marketplace>/<plugin>/<sürüm>/` | Anahtarlar dursa da plugin yok sayılır |

Ölçüm: config anahtarları elle yazılıp `codex exec` koşulduğunda model
"NONE" dedi; aynı evde `codex plugin add` çalıştırılınca (config değişmeden)
`visualize:visualize` göründü. Tersi de doğru: cache kurulduktan sonra
`config.toml` anahtarsız olarak üzerine yazılınca `plugin list` çıktısı boşaldı
(cache diskte kalmasına rağmen).

Bu ikinci durum TionHarness için kritik: `writeCodexConfig` **her turda**
`config.toml`'u sıfırdan yazar. Bu yüzden anahtarları her turda yeniden
yazmak bir iyileştirme değil, zorunluluktur.

## 2. Akış

```
WSSettings (codexPluginsEnabled + codexMarketplaces + codexPlugins)
  → Workspace.codexPluginSpecLocked()      (kapalıysa BOŞ spec)
  → Runtime.SetCodexPlugins()              (atomic.Pointer)
  → Runtime.applyCodexPlugins(provider)    (her turun başında, PinCodexHome yanında)
  → CodexCLI.SetCodexPlugins()
      ├─ buildConfig() → renderCodexConfig()   → config.toml anahtarları (her tur)
      └─ ensureCodexPlugins()                  → codex plugin add (ev başına bir kez)
```

Kapalı ayar **boş spec** üretir; yapılandırılmış listeler gizlenir. Böylece
aşağıdaki her tüketici tek bir "kapalı" durumu görür, ayrı ayrı bayrak kontrol
etmez.

## 3. Kurulum nereye yapılır

`prepareCodexTurnHome` iki farklı ev verir:

- **Kalıcı sohbet evi** (`CLIResumeScope` dolu): `<codex-home>/resume-homes/<sha256>`.
  Plugin **buraya** kurulur. İlk turda ~600 ms (bir marketplace + bir plugin,
  ölçüldü), sonraki turlar bedava.
- **Tek kullanımlık gölge ev** (scope boş): başlık, özet, içgörü gibi yardımcı
  çağrılar. Tur sonunda silinir, bu yüzden plugin **kurulmaz** — her yardımcı
  çağrıda kurulum maliyeti ödemek, o çağrıların hiç kullanmayacağı bir yetenek
  için anlamsız olurdu.

`ensureCodexPlugins` eve bir damga dosyası (`.tionharness-plugins`) yazar;
yapılandırma parmak izi değişmedikçe tekrar kurulum yapılmaz. Kısmi başarısızlıkta
da damga yazılır: bozuk bir marketplace her turda timeout eklemesin diye. Listeyi
düzenlemek parmak izini değiştirir — yeniden denemenin kullanıcıya görünen yolu
budur.

## 4. Rezerve adlar ve içe aktarma

Codex kendi marketplace adlarını korur: `openai-bundled`,
`openai-primary-runtime`, `openai-curated-remote`, `created-by-me-remote`.
Bunları başka bir kaynaktan eklemek reddedilir:

```
Error: marketplace `openai-bundled` is reserved and cannot be added from this source
```

Bu yüzden makinedeki bundled plugin'leri kullanmanın tek yolu **yeniden
adlandırılmış bir kopya**dır. `POST /api/codex-plugins/import` bunu yapar:
marketplace dizinini `<workspace>/codex-marketplaces/<ad>` altına kopyalar ve
kopyanın `marketplace.json`'undaki `name` alanını yeni adla değiştirir.

**Kopyalama asla kendiliğinden olmaz.** Bundled plugin'ler OpenAI'ın tescilli
dosyalarıdır (`"license": "Proprietary"`); kopyalanıp kopyalanmayacağı
kullanıcının kararıdır, ürünün varsayılanı değil. TionHarness kendi
marketplace'i ile gelmez.

`GET /api/codex-plugins/discover` makinedeki marketplace'leri listeler. Yalnız
bilinen marketplace konumlarını yoklar ve her birinin `marketplace.json`'undan
başka **hiçbir şey okumaz** — bir codex home'u `auth.json`, `secrets/` ve canlı
oturum veritabanları da barındırır.

## 5. `--strict-config` riski

Turlar `--strict-config` ile koşar: tanınmayan bir anahtar config yüklemesini
düşürür, yani **o plugin değil, bütün tur** başarısız olur. İki sonuç:

1. `[marketplaces]` ve `[plugins]` anahtarlarının şemaya uygunluğu canlı
   doğrulandı (codex 0.153.3, kabul edildi).
2. Ad ve seçici doğrulaması API kenarında yapılır
   (`internal/api/workspace_codexplugins.go`): rezerve ad, geçersiz karakter,
   bozuk seçici, yinelenen giriş 400 döner. Böyle bir değer hiç kaydedilemez.

**Windows yolu tuzağı:** `source` alanı TOML *literal* string (`'...'`) olarak
yazılır. Basic string'de `C:\Users\...` içindeki `\U` unicode kaçışı sanılır ve
config "too few unicode value digits" ile reddedilir — ölçüldü.

## 6. Sohbet akışında görünürlük

İki plugin cinsi akışta farklı davranır:

- **MCP sunucusu sunanlar** (`unified-computer-use`, `codex-app-tools`): çağrıları
  JSONL'de `mcp_tool_call` item'ı olarak gelir, mevcut parser bunu zaten
  `Kind:"tool"` adımına çevirir. Ek iş gerekmez.
- **Skill sunanlar** (`visualize`, `chrome`, `sites`): **hiçbir iz bırakmaz.**
  Ölçüldü — skill kullanılan bir turda JSONL'de yalnız `agent_message` vardı;
  skill'i adlandıran bir item tipi yok.

Bu yüzden trace, turun neyle **donatıldığını** yazar ("bu turda yüklü
plugin'ler: …"), kullanımı değil. Script yollarından kullanım çıkarımı yapmak
doğru görünen ama yanlış olabilecek bir sinyal üretirdi: model skill'i script
çağırmadan da kullanabilir, ya da hiç kullanmayabilir.

## 7. Dosyalar

| Dosya | Sorumluluk |
|-------|-----------|
| `internal/db/models_codexplugin.go` | `CodexMarketplace` tipi, rezerve ad listesi, seçici ayrıştırma |
| `internal/workspace/settings.go` | `CodexPluginsEnabled` + iki liste, varsayılan açık |
| `internal/workspace/codexplugins.go` | Ayarlardan runtime spec'i (kapalı = boş) |
| `internal/agent/codexplugins.go` | `CodexPluginSpec`, db→providers çevrimi, provider'a uygulama |
| `internal/providers/codexcli_config.go` | `[marketplaces]`/`[plugins]` render + literal string |
| `internal/providers/codexcli_plugins.go` | Ev başına kurulum, damga, hata notları |
| `internal/api/workspace_codexplugins.go` | Kenar doğrulaması |
| `internal/api/codexplugin_discovery.go` | Keşif + içe aktarma (kopyala + yeniden adlandır) |
| `internal/api/codexplugin_roots.go` | Nerelere bakılacağı (dar kapsam) |
| `frontend/src/features/settings/CodexPluginsSection.tsx` | Marketplace/plugin yönetimi + içe aktarma |

## 8. Bilinmeyenler

- `unified-computer-use` ve `codex-app-tools` **kurulur** ama çalıştırılmadı.
  `.mcp.json`'larında `"enabled": false` var ve env'leri Desktop'a bağlı yollar
  istiyor (`CODEX_APP_TOOLS_PIPE_PATH`, `CODEX_ELECTRON_RESOURCES_PATH`). Desktop
  olmadan ayağa kalkıp kalkmadıkları ölçülmedi.
- Rezerve ad kısıtını kopyalayarak aşmak Codex'in bilerek koyduğu bir engeli
  dolaşır; ileride kapatılabilir.
- `plugins.<plugin>.mcp_servers.<server>.enabled` anahtarı (plugin'in MCP
  sunucusunu açmak için) dokümantasyondan biliniyor, canlı denenmedi.
