# TionHarness Tanıtım Sitesi

Statik tek-sayfa tanıtım sitesi. **Astro + Tailwind CSS v4**, sıfır runtime framework —
`npm run build` düz HTML/CSS üretir, herhangi bir statik sunucuda çalışır.

Tasarım kararları ve içerik kuralları: [`_Docs/72-TANITIM-SITESI.md`](../_Docs/72-TANITIM-SITESI.md).

## Çalıştırma

```powershell
cd website
npm install
npm run dev      # http://localhost:4321
npm run build    # -> website/dist/
npm run preview  # dist/ çıktısını yerelde sun
npm run check    # TypeScript + Astro tip kontrolü
```

Go build'i etkilemez: `website/` Go modülünün dışındadır, `go:embed` ile hiçbir ilgisi yoktur.

## Placeholder'ları doldurma

Projede **henüz var olmayan** her şey tek dosyada toplanmıştır:
[`src/site.config.ts`](src/site.config.ts).

Değeri `null` olan bir bağlantı **ölü link üretmez** — `CTAButton` pasif bir kontrol +
"Coming soon" rozeti, `SmartLink` ise soluk metin + "(soon)" gösterir. Repo yayına
alındığında yalnızca bu dosyayı doldurmak yeterlidir; başka hiçbir yeri değiştirmeye
gerek yoktur.

Doldurulacaklar:

| Alan | Ne zaman |
|---|---|
| `repoUrl`, `issuesUrl` | Public repo açıldığında |
| `releasesUrl`, `downloads.*` | İlk release binary'leri yayınlandığında |
| `docsUrl` | Doküman sitesi (faz 2) yayına girdiğinde |
| `license`, `version` | Lisans seçildiğinde / ilk sürüm etiketlendiğinde |
| `astro.config.mjs` → `site` | Alan adı alındığında (canonical + og:url için) |

`quickstart.ts` içindeki `<REPO_URL>` yer tutucusu da repo açılınca gerçek adresle değişir.

## Ekran görüntüleri

Görseller repoda tutulmaz (`.gitignore`), çalışan bir uygulamadan üretilir:

```powershell
# Önce uygulamayı başlat
.\scripts\dev.ps1

# Sonra (ilk seferde Chromium indirir, ~150 MB)
.\scripts\shots.ps1 -InstallDeps
```

Script `website/public/shots/` altına `coordinator.png`, `flows.png`, `board.png`,
`agents.png`, `insights.png` yazar. Görsel yoksa `Screenshot.astro` **placeholder çerçeve**
render eder — site screenshot olmadan da eksiksiz çalışır.

## Klasör yapısı

```
website/
├── src/
│   ├── site.config.ts      # TÜM placeholder'lar burada
│   ├── content/            # metin/veri (features, deepdives, themes, quickstart, scope, stats)
│   ├── components/         # her bölüm ayrı .astro dosyası
│   ├── layouts/Base.astro  # <head>, nav, footer
│   └── pages/              # index.astro, 404.astro
├── scripts/shots.mjs       # Playwright screenshot sürücüsü
└── public/                 # favicon + shots/
```

## Tema

`src/styles/theme.css`, uygulamanın `frontend/src/index.css` dosyasındaki token'larını
(varsayılan "midnight-violet" preset'i) birebir yansıtır. `src/content/themes.ts` ise
`frontend/src/shared/lib/themePresets.ts`'in kopyasıdır (6 renk ailesi × açık/koyu).
**Uygulamada palet değişirse bu iki dosya da güncellenmelidir.**

Tailwind, uygulamadaki gibi üç parçalı import ile alınır (utilities cascade layer dışında
kalsın diye) — gerekçe `theme.css` başındaki yorumda.

## İçerik kaynağı

Site metni yazılırken **`README.md` kaynak alınmaz** — bayat bilgiler içerir (kaldırılmış
"Hafıza" alt sistemi, eksik provider listesi, yanlış tema sayısı). Doğru kaynak:
`_Docs/00-GENEL-BAKIS.md` + `_Docs/05-ILERLEME.md` + `tionharness-project` skill'i.

## Dağıtım

Şu an bağlı bir hosting yok. `dist/` klasörü olduğu gibi Cloudflare Pages, GitHub Pages
veya `deploy/` altındaki VPS'te Caddy ile sunulabilir. Statik sitede auth sorunu yoktur —
uygulamanın aksine internete açmak güvenlidir.
