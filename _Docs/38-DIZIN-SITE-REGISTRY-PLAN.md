# 38 — Dizin-Sitesi Köprüsü (Catalog Connector)

> **Durum: Faz A + B UYGULANDI (2026-06-25); Faz C-F planlı.** crossaitools.com /
> skillsmp.com / claudeskillsmarket.com gibi **skill dizin sitelerini** SwarmGo
> market'ine bağlama. Mevcut `internal/ingest` (SK-IMP3) ve uzak registry
> (`swarmregistry/v1`, `21-MARKET.md` §3) üzerine kurulur.
>
> **Yapıldı:** **Faz A** — `RegistryEntry.Source`/`Pack.SourceRef` (kaynak-ref) +
> install yönlendirmesi (`installSourceRefPack` → `ingest.BuildPacks` + `installPackInto`);
> `Get` kaynak-ref'i payload indirmeden döner; `loadRemoteCache` Source'lu entry kabul eder.
> **Faz B** — skillsmp connector (`market/connectors.go`: `/api/skills` → source-ref
> entry'leri), `Registry.Connector` + `RefreshRemote` connector dalı, API
> `GET/POST /api/market/connectors[/add]`, UI `RegistryManager` "Hazır kaynaklar"
> quick-add. Canlı: skillsmp 12 entry. Testler: `connectors_test.go` (+live).
>
> **Faz C** ✅ — crossaitools connector (`market/connectors.go::fetchCrossAITools`):
> ~12 MB tam liste tek çağrıda gelir (site `?q`/`?limit` yok sayar) → **popülerliğe göre
> (stars, installs) top-300** kırpılır (`crossaitoolsTopN`), `repo`+`path` → GitHub tree
> URL'i (`githubTreeURL`). `maxConnectorBytes=24MB`. Canlı: 21.7k → 300.
> **Faz D** ✅ — market **arama kutusu** (`MarketPanel` `query` state; name/description/
> author client-side filtre).
>
> **Kalan (planlı):** source-ref "Kuruldu" cross-session işaretleme (şu an ledger
> pack-id ile; `isInstalled` slug eşleşmesi MVP'de eksik); crossaitools **server-side
> live search** (cap yerine sorgu-bazlı, arch değişikliği gerektirir); Faz E —
> claudeskillsmarket sitemap/scrape; Faz F — harici statik köprü generator.

## 1. Problem

Bu siteler **HTML/JSON katalog**; her kayıt nihayetinde bir **GitHub repo/klasörüne**
işaret eder (önceden-derlenmiş `.swarmpack.json` DEĞİL). Mevcut uzak registry ise
`swarmregistry/v1` index'i bekler ve her `RegistryEntry.URL` bir **pack indirme**
adresidir. Yani **empedans uyumsuzluğu** var:

| | swarmregistry/v1 | Dizin siteleri |
|---|---|---|
| İçerik | native `.swarmpack.json` | GitHub repo/klasör pointer'ı |
| Kurulum | indir + decode | **ingest** (tarball → adapter → pack) |

**Köprü = bir kayıt "kaynak referansı" (GitHub URL) ise, kurulumda `ingest` çalıştır.**

### Sitelerin veri erişimi (2026-06-25 yoklaması)

| Site | API | Kritik alanlar | Hacim |
|------|-----|----------------|-------|
| crossaitools.com | `GET /api/skills` (JSON) | `repo` (`owner/repo`), `path`, `installCommand`, `stars`, `installs` | **~21.700** |
| skillsmp.com | `GET /api/skills` + `/api/v1/skills/search` (JSON) | `githubUrl` (tam tree URL), `route.sourceSkillPath`, `author` | küçük/sayfalı |
| claudeskillsmarket.com | JSON API yok; `sitemap.xml` var | — (sayfa scrape / sitemap) | orta |

Üçü de GitHub-tabanlı → **GitHub URL üretilebiliyor**, dolayısıyla mevcut `ingest`
hattına beslenebilir. Asıl iş: site cevabını **GitHub URL listesine** çevirmek (connector)
+ kuruluma **ingest** bağlamak.

## 2. İki mimari seçenek

### A. Harici statik köprü (external bridge generator)
Bir CI/script her sitenin API'sini çeker, **`swarmregistry/v1` `registry.json`** üretir
(kayıtlar GitHub URL'lerine işaret eden **kaynak-ref** entry'leri) ve GitHub Pages/gist'te
yayınlar. Kullanıcı bu URL'yi uzak kaynak olarak ekler.
- ✅ App değişimi minimal (yalnız "kaynak-ref entry → ingest" semantiği)
- ✅ Scrape/bakım app dışında; kırılganlık kullanıcıyı bloklamaz
- ❌ Ayrı barındırma + güncelleme pipeline'ı; gerçek-zamanlı değil (snapshot)

### B. Uygulama-içi connector (in-app catalog connector)
SwarmGo'ya **connector** kavramı eklenir: her site için bir connector site API'sini
**canlı** çeker, kayıtları katalogda gösterir; kurulumda ilgili GitHub URL'ini `ingest`'e
verir.
- ✅ Gerçek-zamanlı, tek üründe; arama/sayfalama app'te
- ✅ `ingest` tamamen yeniden kullanılır
- ❌ App'e site-özel HTTP + (claudeskillsmarket için) scrape kodu girer (kırılgan, ToS)

### Öneri: **Ortak dayanak + B'yi tercih et**
Her iki seçenek de **aynı küçük, dayanıklı app değişimini** ister: *"bir kayıt
GitHub kaynak-ref'i ise, kurulum = ingest"*. Bunu çekirdek yap; sonra connector'ları
in-app (B) yaz. Harici köprü (A) sonradan aynı çekirdeği kullanır.

## 3. Çekirdek değişiklik — "kaynak-ref" kayıt türü

`RegistryEntry`'ye (remote.go) opsiyonel **kaynak** alanı:

```go
type RegistryEntry struct {
    ... // mevcut alanlar
    // Source, set yerine URL'in pack olmadığı; bunun yerine ingest edilecek bir
    // GitHub kaynağı olduğunu belirtir. Boşsa klasik pack davranışı (geriye uyumlu).
    Source *EntrySource `json:"source,omitempty"`
}
type EntrySource struct {
    Type string `json:"type"` // "github"
    URL  string `json:"url"`  // owner/repo[/tree/<ref>/<path>]
    Keys []string `json:"keys,omitempty"` // ops. belirli artifact'lar (boş = hepsi)
}
```

Kurulum yönlendirmesi (`api/market.go::handleInstallMarketPack` / yeni dal):
- `entry.Source == nil` → mevcut `fetchPayload` + `installPackInto` (değişmez).
- `entry.Source != nil` → `ingest.BuildPacks(source.URL, source.Keys, opts)` →
  her pack için `installPackInto`. **Zaten var olan tek kurulum otoritesi** kullanılır.

Katalogda kaynak-ref item'ları "kurulumda indirilir/ingest edilir" rozetiyle gösterilir.

## 4. Connector katmanı (B)

Yeni `internal/catalog` (veya `market/connectors`): her site bir `Connector`:

```go
type Connector interface {
    Name() string
    // List, siteyi sorgulayıp swarmregistry kayıtlarına (kaynak-ref) çevirir.
    List(ctx, query string, page int) ([]RegistryEntry, error)
}
```

- **crossaitoolsConnector:** `GET /api/skills` → her kayıt `repo`+`path` →
  `EntrySource{github, "github.com/<repo>/tree/main/<path>"}`. 21.7k kayıt →
  **asla tümünü katalog index'ine yükleme**; `query`/`page` ile sunucu-tarafı arama
  (`/api/skills?q=`) veya client-side ilk-N + arama kutusu.
- **skillsmpConnector:** `githubUrl` doğrudan `EntrySource.URL`; `/api/v1/skills/search`
  ile arama.
- **claudeskillsmarketConnector:** API yok → `sitemap.xml` + sayfa scrape (en kırılgan;
  **son faz / opsiyonel**, site API açarsa öncelik).

Connector kayıtları, mevcut uzak-registry tier'ı gibi `market.List()`'e karışır
(`Source: SourceRemote`, `RegistryName: <site>`), `ledger` ile "Kuruldu/Güncelle".

## 5. UI

- `RegistryManager`'a (mevcut "Kaynaklar" modalı) **hazır connector'lar** sekmesi:
  crossaitools/skillsmp toggle'ları (URL elle girmeye gerek yok).
- Market Skills sekmesinde connector kaynaklı item'lar **arama kutusu** ile (21.7k
  için zorunlu); item detayında "Kaynak: GitHub <repo>", install → ingest akışı
  (Tara→Seç gerekmez; tek-skill ref'i doğrudan kurar, çok-skilli repo ref'i için
  mevcut `SkillImportDialog` akışına devredebilir).

## 6. Riskler / kısıtlar

- **Scrape kırılganlığı + ToS:** site API'leri resmi değil; şema değişebilir, rate-limit
  /401 gelebilir. Connector başına timeout + nazik hata + cache şart. claudeskillsmarket
  scrape'i en riskli.
- **Ölçek:** crossaitools 21.7k → index'i belleğe/diske komple çekme; **lazy + arama**.
- **Güven/güvenlik:** rastgele GitHub repo'su ingest etmek = rastgele içerik. İçe aktarma
  zaten yalnız metin/SKILL.md + bilinen adapter'lar üretir (kod çalıştırmaz), ama
  kullanıcıya kaynak repo + yıldız sayısı gösterilmeli; install öncesi onay.
- **Bütünlük:** foreign repo'larda sha256 yok; sürüm = repo `lastUpdated`/commit.
- **Branch tespiti:** crossaitools branch vermiyor → `main`→`master` fallback (fetch'te zaten var).
- **Çift kayıt:** aynı repo birden çok sitede → `EntrySource.URL` normalize + dedup.

## 7. Fazlama

1. **Çekirdek:** `RegistryEntry.Source` + install yönlendirmesi (kaynak-ref → ingest).
   Test: elle yazılmış kaynak-ref `registry.json` → install → ingest. (App içi, küçük.)
2. **skillsmp connector** (en temiz API, `githubUrl` hazır) → uçtan uca doğrulama.
3. **crossaitools connector** + arama/sayfalama (ölçek).
4. **UI:** RegistryManager hazır-connector toggle'ları + market arama kutusu.
5. **(Ops.) claudeskillsmarket** sitemap/scrape — site JSON API açarsa öne alınır.
6. **(Ops.) Harici köprü (A):** aynı çekirdeği kullanan statik `registry.json` generator
   (CI) — offline/paylaşılabilir snapshot isteyenler için.

## 8. Eforun kabaca büyüklüğü

- Çekirdek (Faz 1): küçük (~yarım gün) — tek yeni alan + bir install dalı, mevcut
  `ingest.BuildPacks` + `installPackInto` yeniden kullanılır.
- Connector başına (Faz 2-3): orta — HTTP istemci + şema eşleme + arama; site başına ~yarım gün.
- UI (Faz 4): orta.
- claudeskillsmarket scrape (Faz 5): yüksek belirsizlik (kırılgan).

## 9. Karar bekleyen noktalar

- Connector'lar **in-app** mı (canlı, B) yoksa **harici generator** mı (snapshot, A)?
  Öneri: çekirdek + in-app skillsmp/crossaitools; claudeskillsmarket'i ertele.
- Kaynak-ref tek-skill mi yoksa tüm-repo mu kurar? Öneri: site kaydı genelde tek skill
  (`path`/`sourceSkillPath`) → tek-skill; "tüm repoyu içe aktar" için mevcut import
  dialog'una "repoyu aç" kısayolu.
- Trust: install öncesi repo onayı/yıldız eşiği gösterimi.
