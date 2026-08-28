# 72 — Tanıtım Sitesi (website/)

> **Durum: UYGULANDI (iskelet + içerik, 2026-08-25).** Açık kaynak kullanıcıya yönelik
> statik tek-sayfa tanıtım sitesi. Kod `website/`, kullanım `website/README.md`.

## Neden

TionHarness'in anlatılması gereken bir hikâyesi var (tek binary, DB yok, anahtar yok,
ajanlar uygulamayı kendi yönetiyor) ama bunu yalnızca `README.md` anlatıyor — GitHub'da,
Türkçe olmayan bir kitleye, bayat bilgilerle. Site bu anlatıyı ürünün kendi görsel diliyle
sunar.

## Kararlar

| Konu | Karar | Gerekçe |
|---|---|---|
| Stack | Astro 5 + Tailwind v4 | Varsayılan sıfır JS, statik çıktı, ileride `/docs` eklenebilir |
| Konum | `website/` (repo içinde) | Özellik değişince site metni **aynı commit'te** güncellenir |
| Dil | EN | Hedef kitle açık kaynak geliştiricisi; TR faz 2 |
| Kapsam | Tek sayfa + `/releases` + `/404` | Doküman sitesi ayrı bir iş |
| Deploy | Yok (henüz) | `dist/` hazır; hosting kararı repo kararına bağlı |
| Alan adı | `tionharness.com` | Bir alan adı sağlayıcısında kayıtlı; DNS henüz bir yere yönlendirilmedi |
| Release verisi | **Build anında** çözülür, tarayıcıda tazelenir | Sürüm ve indirme linki JS'siz görünür; build hiçbir zaman canlı bir host'a bağımlı değil (aşağıya bak) |

Go tarafını etkilemez: `website/` modül dışıdır, `go:embed` ağacına girmez.

## Placeholder politikası — sitenin omurgası

Projede henüz **olmayan** her şey (release binary'leri, docs sitesi, tanıtım videosu,
DNS) tek dosyada toplanır: `website/src/site.config.ts`. Public repo, lisans ve issue
takibi artık var, dolayısıyla o alanlar dolduruldu.

Kural: **`null` = henüz yok.** Bileşenler bu durumu ölü linke çevirmez.

- `CTAButton` → pasif kontrol + "Coming soon" rozeti + açıklayıcı `title`
- `SmartLink` → soluk metin + "(soon)"
- `Screenshot` → build anında `public/` altında dosya var mı diye bakar; yoksa
  dosya adını ve `scripts\shots.ps1` ipucunu taşıyan **placeholder çerçeve** çizer

Böylece site bugün eksiksiz ve dürüst; repo açıldığında tek dosya düzenlenip canlıya
geçiyor. Yeni bir "henüz yok" alanı eklenirken **mutlaka** `site.config.ts`'e konur,
bileşenin içine gömülmez.

## Release feed — `feedUrl` / `PUBLIC_FEED_URL`

`site.config.ts` iki yeni **string** alan taşır (bunlar `null` değildir; alan adı
gerçekten kayıtlı, feed adresi gerçekten planlanmış):

| Alan | Değer | Anlamı |
|---|---|---|
| `url` | `https://tionharness.com` | Sitenin kanonik adresi; `<link rel="canonical">` ve `og:url` buradan üretilir. `astro.config.mjs`'teki `site` alanı **elle senkron** tutulur (config `.mjs`, TS dosyasını import edemez) |
| `feedUrl` | `https://tionharness.com` | Release feed'inin kökü — feed sitenin kendi kökünden (`website/public/latest.json` → `/latest.json`) sunulur. `import.meta.env.PUBLIC_FEED_URL` verilirse onunla ezilir |

```bash
cd website
PUBLIC_FEED_URL=http://localhost:8080 npm run build   # yerel feed'e karşı derle
```

`PUBLIC_` öneki zorunludur: Astro/Vite yalnız bu önekli değişkenleri istemci
paketine gömer. Tip tanımı `website/src/env.d.ts` içindedir.

### İki katman: build anında çözüm + tarayıcıda tazeleme

Feed **iki kez** okunur ve iki okuma da aynı `null` politikasına uyar:

1. **Build anında** — `website/src/lib/releaseFeedBuild.ts`. Kaynak, bu deploy'un
   kendi yayınladığı dosyadır: `website/public/latest.json` (Astro onu olduğu gibi
   `/latest.json` olarak kopyalar). Diskten okumak hem en taze hem de tek
   döngüsüz kaynaktır; canlı siteyi fetch etmek **bir önceki** deploy'un feed'ini
   döndürürdü. `PUBLIC_FEED_URL` verilirse kaynak gerçek bir HTTP fetch'e
   (6 sn timeout) döner — yerel `deploy/release-host` konteynerini test etmenin yolu
   budur. Sonuç HTML'e yazılır: sürüm rozeti ve indirme linki **JS olmadan** görünür.
2. **Tarayıcıda** — `releaseFeed.ts`, sayfa yüklendikten sonra `<feedUrl>/latest.json`
   çeker. Ziyaretçinin platformuna göre birincil butonu yeniden seçer ve site
   yeniden derlenmeden yeni bir sürüm yayınlanmışsa onu gösterir.

Build fetch'i **asla derlemeyi düşürmez**: erişilemeyen host, 200 dışı yanıt, bozuk
JSON veya eksik dosya `null`'a iner ve site "Coming soon" durumunda kalır. Ancak
sessizce yutulmaz — her başarısızlık build log'una `[release-feed] …` satırı basar.
Sessiz yanlış sürüm, sürüm göstermemekten kötüdür.

Feed `Access-Control-Allow-Origin: *` ile servis edildiği için tarayıcıdan okumak
serbesttir. Şema:

```json
{ "version": "0.1.0", "released_at": "<RFC3339>", "notes_url": "...",
  "artifacts": [ { "os": "linux|windows|darwin", "arch": "amd64|arm64",
                   "file": "...", "url": "...", "sha256": "...", "size": 123 } ] }
```

### Bozulmadan geri düşme (graceful degradation)

`website/src/lib/releaseFeed.ts` tek giriş noktasıdır ve **her** başarısızlık yolunu —
host kapalı, 6 sn timeout, 200 dışı yanıt, bozuk/eksik JSON, `http(s)` olmayan `url` —
`null`'a indirger. Bileşenler yalnızca `null` **olmayan** sonuçta DOM'u değiştirir:

- placeholder durumu **build çıktısına yazılıdır**, JS onu yalnız başarıda gizler;
- dolayısıyla ilk boyama (first paint) fetch'i beklemez, script `type=module` (defer);
- feed erişilemezken kırık buton, hata diyaloğu veya uydurma sürüm numarası **çıkmaz**.

Tarayıcının kendi ağ hatası (`ERR_NAME_NOT_RESOLVED`) konsolda görünür; bu, runtime
fetch yapan her sitede olur ve JS hatası değildir. Kod bilerek ek log basmaz: bugün
feed'in erişilemez olması beklenen durumdur, kullanıcının yapabileceği bir şey yoktur.

### İlgili bileşenler

- `components/Downloads.astro` — ana sayfadaki `#download` bölümü. Ziyaretçinin
  platformunu `navigator.userAgent`'tan tahmin eder, eşleşen artifact'ı birincil
  butona koyar, kalanları ikincil link olarak listeler; sürüm + tarih gösterir.
  Mimari tahmini güvenilir olmadığı için varsayılan `amd64`'tür (macOS'ta `arm64`)
  ve diğer tüm artifact'lar zaten listelenir. `Hero`'daki Download butonu bu bölüme
  çapa (`#download`) atar.
- `pages/releases.astro` — `/releases`: sürüm, tarih, `notes_url` linki ve platform
  başına dosya/boyut/**sha256** tablosu (elle doğrulama için `sha256sum` /
  `certutil` ipucuyla). Footer'daki "Releases" linki artık bu iç sayfaya gider.
- `pages/releases.xml.ts` — `/releases.xml`: RSS 2.0 kanalı. Feed yalnız **güncel**
  sürümü tanımladığı için kanalda en fazla **tek item** olur; sürüm yokken kanal
  boş ama geçerli kalır (uydurma geçmiş üretilmez). `Base.astro` içinde
  `<link rel="alternate">`, footer'da "RSS" linki ile duyurulur.

### Yerel feed'e karşı önizleme

`deploy/release-host/` aynı feed'i yerelde servis eder:

```bash
cd deploy/release-host && cp .env.example .env
docker compose up -d
bash sync-release.sh 0.0.1-test        # dist/release/<sürüm>/ içeriğini yayınlar
cd ../../website && PUBLIC_FEED_URL=http://localhost:8080 npm run build && npm run preview
# temizlik: docker compose down; .env ve srv/dl/* silinir (.gitkeep kalır)
```

## Sayfa akışı

`Hero` → `StatsBar` → `NoDocker` → `FeatureGrid` → `DeepDives` (3 şerit) →
`ThemeShowcase` → `Quickstart` → `HonestScope` → `Footer`.

Öne çıkan iki bölüm:

- **`NoDocker`** — solda tipik bir agent platformunun compose dosyası, sağda tek satır
  `.\tionharness.exe`. Farkı anlatmanın en kısa yolu.
- **`HonestScope`** — "bu ne DEĞİLDİR". Auth yokluğu + wildcard CORS **saklanmaz**,
  local-first tasarım kararı olarak açıkça yazılır. Aksi hâlde kullanıcı servisi
  `0.0.0.0`'a açar. Bu bölüm pazarlama değil, güvenlik gereğidir.

`Hero` bir screenshot beklemez: `AgentTreeGraphic` koordinatör/worker panelini CSS ile
çizer, dolayısıyla site sıfır görselle de tam görünür.

## Tema senkronu (kırılgan nokta)

İki dosya uygulamadan **elle kopyalanır** ve uygulama değişince güncellenmelidir:

| Site dosyası | Kaynağı |
|---|---|
| `src/styles/theme.css` | `frontend/src/index.css` (`@theme` token'ları) |
| `src/content/themes.ts` | `frontend/src/shared/lib/themePresets.ts` (6 renk × açık/koyu) |

Tailwind importu da uygulamadaki gibi **üç parçalıdır** (`theme.css` + `preflight.css` +
`utilities.css`), çünkü Chrome 150 büyük bir `@layer` içindeki `@media` bildirimlerini
yanlış sıralayıp `hidden md:flex` kalıbını bozuyor. İkisi birlikte değişmeli.

## Ekran görüntüleri

`scripts\shots.ps1` (ASCII-only, WinPS 5.1) → `website/scripts/shots.mjs` (Playwright).
Çalışan bir örneğe bağlanır, `#/w/{workspaceId}/{view}` hash rotalarını gezer, 2560×1440
koyu temada `website/public/shots/`'a yazar. Görseller **gitignore'da** — üretilen çıktı,
kaynak değil.

Playwright bilerek `package.json`'a konmadı (Chromium ~150 MB); `-InstallDeps` ile talep
üzerine kurulur. Böylece sitenin normal kurulumu hafif kalır.

## Docs bölümü (`/docs`)

Site içi dokümantasyon, ana sayfadan ayrı bir rota ağacıdır. Şu an **iskelet**:
7 sayfanın hepsi placeholder, gerçek `_Docs` içeriği bilerek taşınmadı.

| Dosya | Rolü |
|---|---|
| `src/content.config.ts` | `docs` koleksiyonu — `glob` loader, `src/content/docs/**/*.md`, şema: `title`, `description`, `order` |
| `src/content/docs/<bölüm>/<sayfa>.md` | Placeholder markdown. Koleksiyon id'si `<bölüm>/<sayfa>`, URL'i `/docs/<bölüm>/<sayfa>` |
| `src/content/docsNav.ts` | **Sıra ve etiketlerin tek kaynağı**; sidebar da prev/next de bundan türer |
| `src/layouts/Docs.astro` | Sol sidebar + breadcrumb + sağda TOC (h2/h3) + altta prev/next |
| `src/pages/docs/[...slug].astro` | `getStaticPaths` ile koleksiyondan sayfa üretir |
| `src/pages/docs/index.astro` | Bölümleri listeleyen giriş sayfası |

Bölümler: `getting-started` (introduction, installation, quickstart),
`concepts` (agents, sessions, workspaces), `reference` (configuration).

Dikkat edilecekler:

- **Yeni sayfa iki yere birden eklenir**: markdown dosyası + `docsNav.ts` girdisi.
  `assertNavMatchesCollection` iki yönlü kontrol eder — nav'da olup dosyası olmayan
  da, dosyası olup nav'da olmayan da **build'i patlatır**. Sessizce atlanmaz;
  ilki ölü sidebar linki, ikincisi erişilemeyen sayfa demektir.
- Tailwind v4 kurulumunda **typography eklentisi yok**; markdown gövdesinin
  (`h2`, `p`, `pre`, `code`, `table`…) stilleri `Docs.astro` içindeki scoped
  `<style>` bloğunda elle yazılıdır. Yeni bir element tipi kullanılacaksa oraya
  eklenir.
- `src/content/` altında hem bu koleksiyon hem de eski **düz TS veri modülleri**
  (`features.ts`, `themes.ts`…) yaşar. Loader deseni `docs/**` ile sınırlıdır,
  ikisi çakışmaz.
- `site.config.ts` → `docsUrl` artık `null` değil; Nav ve Footer'daki `SmartLink`
  bu yüzden otomatik olarak canlı link render eder.
- Uygulama tarafındaki **Ayarlar → Hakkında** paneli
  (`frontend/src/features/settings/AboutPanel.tsx`, `PROJECT_LINKS`) siteye,
  docs'a ve depoya link verir. Docs rotası taşınırsa orası da güncellenmelidir.

Arama kutusu, tema seçici ve TR çevirisi kapsam dışı bırakıldı.

## İçerik kaynağı kuralı

Site metni yazılırken/güncellenirken **kök `README.md` kaynak alınmaz.** Bayat: kaldırılmış
Hafıza alt sistemini anlatıyor (2026-07-05'te silindi), provider listesi 3 diyor (gerçekte
7 kind), tema sayısını 8 sanıyor (gerçekte 6 renk × 2 mod). Doğru kaynak sırasıyla:
`00-GENEL-BAKIS.md`, `05-ILERLEME.md`, `tionharness-project` skill'i.

> **Açık iş:** kök `README.md`'nin de aynı gerekçeyle tazelenmesi gerekiyor.

## Sıradaki adımlar

- `repoUrl` / `issuesUrl` / `license` **dolduruldu** (depo public:
  `github.com/bilal-arikan/tionharness`). `releasesUrl` (henüz tag yok, GitHub
  releases sayfası boş) ve `demoVideoUrl` bilerek `null` — dolduğunda otomatik
  canlıya geçerler. `docsUrl` artık dolu (`/docs/getting-started/introduction`)
- `version` artık `site.config.ts`'ten değil feed'den gelir; alan yalnızca
  hiç feed olmayan bir build için fallback olarak duruyor
- Site GitHub Pages'e `.github/workflows/pages.yml` ile deploy edilir; feed aynı
  deploy'la `/latest.json` olarak gider. DNS + Pages ayarları için
  `_Docs\75-YAYIN-SURECI.md` → "Depo ayarları"
- `site.downloads` alanı artık feed tarafından ikame edildi; ilk yayından sonra
  bu alanın tamamen kaldırılması değerlendirilecek
- Gerçek screenshot'ları üret (`shots.ps1`)
- Pages custom domain'i (`tionharness.com`) doğrula ve HTTPS'i zorunlu kıl
- `/docs` iskeleti kuruldu; sıradaki iş placeholder sayfaları gerçek içerikle
  doldurmak (kaynak: `00-GENEL-BAKIS.md` + `tionharness-project` skill'i)
- Faz 2: TR çevirisi, `05-ILERLEME.md`'den türetilen `/changelog`, docs araması
