# TionHarness Tanıtım ve Dokümantasyon Sitesi

Astro 5 + Tailwind CSS v4 ile üretilen statik site; ana sayfa, kullanıcı kılavuzu,
sürüm notları ve sürüm akışını sunar. Uygulamanın Go binary'sine gömülmez.
Tasarım kararları: [72-TANITIM-SITESI.md](../_Docs/72-TANITIM-SITESI.md).

## Çalıştırma ve doğrulama

Depo kökünden:

```powershell
cd website
npm ci
npm run dev      # http://localhost:4321
npm run check
npm run build    # website/dist/
npm run preview
```

`frontend/` ayrı npm projesidir. Siteyi derlemek uygulama arayüzünü derlemez.

## İçerik ve bağlantılar

`src/site.config.ts` repo, issue, dokümantasyon ve alan adı bağlantılarını içerir.
`null` değerler `CTAButton` ve `SmartLink` tarafından pasif gösterilir. Repo, lisans
ve dokümantasyon adresleri zaten tanımlıdır; bunları yeniden doldurmak gerekmez.

Sürüm ve indirme adreslerinin kaynağı `public/latest.json` dosyasıdır.
`src/lib/releaseFeedBuild.ts` bu dosyayı derleme sırasında okur; `PUBLIC_FEED_URL`
verilirse harici akıştan okur. Kullanılabilir artifact yoksa indirme yerine
"Coming soon" görünür. Yalnız `site.config.ts` içindeki `downloads` alanlarını
doldurmak sürüm yayımlamaz. Yayın akışı için
[75-YAYIN-SURECI.md](../_Docs/75-YAYIN-SURECI.md).

`src/content/quickstart.ts` içindeki `<REPO_URL>`, sayfa oluşturulurken yapılandırılmış
repo adresiyle değiştirilir; elle sabit bir adresle değiştirilmesi gerekmez.

## Klasör yapısı

| Yol | İçerik |
|---|---|
| `src/content/docs/` | Markdown kullanıcı kılavuzları; `title`, `description`, `order` ön bilgisi |
| `src/content.config.ts` | Doküman koleksiyonu ve ön bilgi şeması |
| `src/content/` | Tanıtım metinleri, tema ve hızlı başlangıç verileri |
| `src/components/`, `src/layouts/` | Sayfa bileşenleri ve ortak yerleşimler |
| `src/pages/docs/` | İçerik koleksiyonundan üretilen doküman rotaları |
| `src/pages/releases.astro`, `src/pages/releases.xml.ts` | Sürüm notları ve akış |
| `public/` | Favicon, ekran görüntüleri ve `latest.json` |
| `scripts/shots.mjs` | Playwright ekran görüntüsü sürücüsü |

## Ekran görüntüleri

Aşağıdaki komutları **depo kökünden**, ayrı terminallerde çalıştırın:

```powershell
.\scripts\dev.ps1
.\scripts\shots.ps1 -InstallDeps
```

Görüntüler `public/shots/` altında üretilir ve gitignore kapsamındadır. Dosya yoksa
`Screenshot.astro` yer tutucu çerçeve gösterir.

## Tema ve içerik bakımı

`src/styles/theme.css` uygulamanın `frontend/src/index.css` token'larıyla,
`src/content/themes.ts` ise `frontend/src/shared/lib/themePresets.ts` ile elle
senkronlanır. Güncel palet 10 renk ailesi × açık/koyu moddur; varsayılan
`violet-dark` değeridir.

İçerik, ilgili kaynak kodu ve `_Docs` içindeki güncel özellik dokümanlarıyla
karşılaştırılır. README özet sunar; tarihli ilerleme kayıtları geçmişi anlatır ve
tek başına güncel davranış kanıtı sayılmaz. Tanıtım metinleri İngilizce,
doküman dosyaları Türkçe tutulur.

## Dağıtım

`.github/workflows/pages.yml` GitHub Pages dağıtımını yapar. `dist/` başka bir statik
sunucuda da sunulabilir. `deploy/release-host/` yerel önizleme içindir.
