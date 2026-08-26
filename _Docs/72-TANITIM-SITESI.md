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
| Kapsam | Tek sayfa + `/404` | Doküman sitesi ayrı bir iş |
| Deploy | Yok (henüz) | `dist/` hazır; hosting kararı repo kararına bağlı |

Go tarafını etkilemez: `website/` modül dışıdır, `go:embed` ağacına girmez.

## Placeholder politikası — sitenin omurgası

Projede henüz **olmayan** her şey (public repo, release binary'leri, docs sitesi, lisans,
sürüm, alan adı) tek dosyada toplanır: `website/src/site.config.ts`.

Kural: **`null` = henüz yok.** Bileşenler bu durumu ölü linke çevirmez.

- `CTAButton` → pasif kontrol + "Coming soon" rozeti + açıklayıcı `title`
- `SmartLink` → soluk metin + "(soon)"
- `Screenshot` → build anında `public/` altında dosya var mı diye bakar; yoksa
  dosya adını ve `scripts\shots.ps1` ipucunu taşıyan **placeholder çerçeve** çizer

Böylece site bugün eksiksiz ve dürüst; repo açıldığında tek dosya düzenlenip canlıya
geçiyor. Yeni bir "henüz yok" alanı eklenirken **mutlaka** `site.config.ts`'e konur,
bileşenin içine gömülmez.

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

## İçerik kaynağı kuralı

Site metni yazılırken/güncellenirken **kök `README.md` kaynak alınmaz.** Bayat: kaldırılmış
Hafıza alt sistemini anlatıyor (2026-07-05'te silindi), provider listesi 3 diyor (gerçekte
7 kind), tema sayısını 8 sanıyor (gerçekte 6 renk × 2 mod). Doğru kaynak sırasıyla:
`00-GENEL-BAKIS.md`, `05-ILERLEME.md`, `tionharness-project` skill'i.

> **Açık iş:** kök `README.md`'nin de aynı gerekçeyle tazelenmesi gerekiyor.

## Sıradaki adımlar

- Repo/lisans/release kararı → `site.config.ts` doldur
- Gerçek screenshot'ları üret (`shots.ps1`)
- Hosting bağla (Cloudflare Pages veya `deploy/` VPS + Caddy)
- Faz 2: TR çevirisi, `/docs`, `05-ILERLEME.md`'den türetilen `/changelog`
