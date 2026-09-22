# 39 — Dizin-Sitesi Köprüsü (Search Connector)

> **Durum: UYGULANDI (canlı arama + önizleme, 2026-06-26).** crossaitools.com /
> skillsmp.com gibi **skill dizin sitelerini** TionHarness market'ine bağlar. Mevcut
> `internal/ingest` (SK-IMP3) + uzak registry (`harnessregistry/v1`, `21-MARKET.md` §3)
> üzerine kurulur.
>
> **Mimari (özet):**
> - **Kaynak-ref (source-ref):** `market.RegistryEntry.Source` / `Pack.SourceRef`
>   (`SourceRef{Type,URL,Keys}`) — bir kayıt pack indirme yerine bir **GitHub kaynağına**
>   işaret eder. Install yönlendirmesi `installSourceRefPack` → mevcut
>   `ingest.BuildPacks` + `installPackInto`. `Get` payload indirmeden manifest döner.
> - **Connector'lar SEARCH-ONLY (canlı):** siteler **binlerce** skill barındırdığından
>   toplu katalog YOK; market arama çubuğu connector'ları **canlı** sorgular
>   (`SearchConnectors`). `RefreshRemote`/`loadRemoteCache` connector registry'lerini
>   atlar; connector'lar registry olarak "eklenmez", `ListConnectors` ile bilinir.
> - **skillsmp:** gerçek arama API'si `/api/v1/skills/search?q=` (`githubUrl` doğrudan).
> - **crossaitools:** site `?q` yok sayar → tüm liste (~12 MB) **bir kez** indirilip
>   `<DataDir>/market/.remote-cache/crossaitools-lite.json`'a (24s TTL) cache'lenir,
>   arama lokal filtre + stars sıralaması (`filterCrossAITools`).
> - **Önizleme:** source-ref için `ingest.Preview` GitHub'dan SKILL.md çekip render eder;
>   `POST /api/ingest/preview`; UI detay modalında gösterir (`SourceRefPreview`).
> - **API:** `GET /api/market/connectors` (liste) + `GET /api/market/connectors/search?q=`
>   (canlı arama) + `POST /api/ingest/{preview,install}`.
> - **UI:** Market arama kutusu skill sekmesinde 2+ karakterde connector'ları debounced
>   sorgular → "İnternet sonuçları" bölümü; kart → detay (önizleme) → "Bu workspace'e
>   kur" (ingest). `RegistryManager` connector'ları yalnız bilgilendirme listeler.
> - Testler: `connectors_test.go` (decode/filter/sort/source-ref) + canlı
>   `connectors_live_test.go` (skillsmp search, crossaitools cache+filter).
>
> **Kalan (planlı):** source-ref "Kuruldu" cross-session işaretleme (ledger pack-id ile;
> `isInstalled` slug eşleşmesi eksik); claudeskillsmarket (JSON API yok → sitemap/scrape);
> harici statik köprü generator; arama sonuçlarında sayfalama.

## Kod

- Connector'lar, source-ref ve canlı arama: `internal/market/connectors.go`
  (+ `connectors_test.go`, `connectors_live_test.go`). Planda önerilen ayrı
  `internal/catalog` paketi kurulmadı.

> Uygulama öncesi plan gövdesi (problem, site veri erişimi yoklaması, seçenekler,
> fazlama, efor, açık kararlar) → [arsiv/39-DIZIN-SITE-REGISTRY-PLAN.md](arsiv/39-DIZIN-SITE-REGISTRY-PLAN.md).
