# 73 — Lokalizasyon (UI i18n)

> **Durum:** Arayüz genelinde İngilizce/Türkçe katalog geçişi tamamlandı (2026-09-29).
> Her dilde 28 namespace ve 5.355 çeviri anahtarı bulunuyor.
> **Kapsam:** Yalnız **arayüz dili**. Ajan yanıt dili ve LLM'e giden metin ayrı eksenler (aşağıya bak).

## 1. Üç ayrı "dil" ekseni

Bu projede "dil" tek bir şey değil. Karıştırmak en pahalı hata olur:

| # | Eksen | Nerede yaşar | Kim değiştirir |
|---|-------|--------------|----------------|
| 1 | **Arayüz dili** (chrome: düğme, etiket, hata) | `Settings.UILanguage` + `frontend/src/i18n/` katalogları | Kullanıcı, Ayarlar → Profil |
| 2 | **Ajan yanıt dili** | `Settings.Language` → sistem promptundaki "reply in …" satırı | Kullanıcı, Ayarlar → Profil |
| 3 | **Backend'in ürettiği yarı-yapılı metin** (view projeksiyonları, tool sonuçları, `<task-notification>`) | `internal/view/*`, `internal/tools/*` | — (henüz Türkçe, bkz. §7) |

`UILanguage = ""` → **arayüz ajan dilini izler**. Mevcut kurulumlar bu durumda başlar,
yani i18n eklenmesi hiçbir kullanıcının gördüğü dili değiştirmedi.

Eksenlerin ayrı olmasının somut teknik gerekçesi: `Language` değişimi statik system
prompt'u yeniden dondurur ve **prompt cache'i soğutur** (bkz. [57](57-PROMPT-EPOCH.md)).
Arayüz dilini değiştirmek hiçbir prompt'a dokunmaz. Tek alan olsaydı, "menüler
İngilizce olsun" demek her oturumun cache'ini yakardı.

## 2. Teknoloji

| Katman | Seçim | Gerekçe |
|--------|-------|---------|
| Çevirmen | `i18next` + `react-i18next` | Çoğul/bağlam desteği, namespace, `i18next-parser` ile anahtar çıkarma |
| Katalog formatı | Namespace başına JSON | Standart; harici çeviri araçlarının doğrudan okuduğu format |
| Yükleme | **Eager** (`import.meta.glob`) | Tek binary'den servis edilen yerel uygulamada ağ gidiş-dönüşü yok; lazy yükleme yalnızca çevrilmemiş bir kare (flash) kazandırırdı |
| Biçimlendirme | `Intl.*` (`shared/lib/intl.ts`) | Tarih/sayı/sıralama; kelime değil |

**Kaynak dil = `en`.** Türkçe bir çeviridir. Bu, kod/yorum İngilizce + doküman Türkçe
kuralıyla ve `website/`'in İngilizce olmasıyla tutarlı. Eksik anahtar İngilizce'ye
düşer (ham anahtar yoluna değil).

## 3. Dosya haritası

```
internal/settings/language.go        # SupportedLanguages, LanguageDisplayName (UI dili çözümü bootLocale.ts'te)
frontend/src/i18n/
├── locales.ts                       # dil kaydı: kod, endonym etiket, Intl tag, dir (RTL hazır)
├── catalog.ts                       # import.meta.glob ile katalog toplama
├── bootLocale.ts                    # localStorage önbelleği + resolveUILocale (backend ile aynı mantık)
├── index.ts                         # i18next init, setLocale, <html lang/dir>
├── I18nRoot.tsx                     # dil değişiminde ağacı yeniden bağlar
├── catalog.test.ts                  # parite/boşluk/placeholder guard'ı
├── sourceCatalog.test.ts            # statik t/Trans anahtarlarının katalogda bulunması
└── locales/{en,tr}/*.json            # ekran ve ortak bileşen katalogları
frontend/src/shared/lib/intl.ts      # dateFormat/numberFormat/collator + formatDate/Time/DateTime/compareText
frontend/src/shared/lib/format.ts    # usd/count/decimal/percent/tokens/bytes (locale-duyarlı)
frontend/src/shared/lib/time.ts      # relativeTime/formatDuration/bucketLabel (katalog-tabanlı)
frontend/i18next-parser.config.js    # npm run i18n:extract
```

## 4. Namespace ve anahtar sözleşmesi

- **Namespace normalde feature klasörü adıdır:** `chat`, `tasks`, `flows`… Büyük
  yüzeyler `chatControls`, `chatStatus`, `settingsMain` gibi alt kataloglara
  ayrılabilir. Uygulama kabuğu `common`, ortak bileşenler `sharedUi`, ortak
  yardımcılar `shared` kullanır.
- **Anahtar semantiktir**, metnin kendisi değil: `chat.composer.send`. Metin düzeltmek
  anahtarı kırmazsa çeviriler ayakta kalır.
- Katalog dosyası: `src/i18n/locales/<locale>/<namespace>.json`. Dosyayı bırakmak yeter,
  kayıt listesi yok.

### Çoğul (dikkat!)

`t(key, { count })` çağrısı i18next'te **çıplak anahtarı aramaz**, `_one`/`_other`
eklerini arar. Eksik ek, tip kontrolünden ve parite testinden geçip **çalışma anında ham
anahtar yolu** basar. Türkçe'de sayıdan sonra çoğul eki olmasa bile her iki biçim de
yazılmalıdır (ikisi de aynı metin). `time.test.ts` bunu render çıktısı üzerinden doğrular.

## 5. Guard'lar (çalışır durumda)

| Guard | Ne yakalar |
|-------|-----------|
| `src/i18n/catalog.test.ts` | Diller arası namespace/anahtar farkı, boş çeviri, `{{placeholder}}` uyuşmazlığı, kayıtsız katalog klasörü |
| `src/i18n/sourceCatalog.test.ts` | Statik olarak çözülebilen `t` ve açık namespace kullanan `Trans` çağrılarının iki dilde de bulunması; çoğul anahtar ailelerinin eksikliği |
| `src/shared/lib/time.test.ts` | Anahtarın gerçekten çözülmesi (çoğul eki dahil), locale'e göre sayı/yüzde biçimi |
| ESLint `i18next/no-literal-string` | Tüm `src` altında doğrudan JSX metninin katalogları atlaması; test verileri, marka adı, semboller ve kod örnekleri kapsam dışıdır. Attribute ve dinamik anahtarlar için ekran testleri de gerekir |
| Ekran yerelleştirme testleri | Masaüstü/mobil gezinme, başlıklar, bildirimler, seçiciler ve modül düzeyi metadata'nın canlı dil değişiminde güncellenmesi; makine değerleri ve kullanıcı içeriğinin korunması |
| `internal/api/settings_test.go` altın liste | `uiLanguage` alanının backend↔frontend tip sürüklemesi |
| `internal/settings/language_test.go` | `UILanguage` "" toleransı, bilinmeyen kodun düşürülmesi, çözümleme sırası, prompt adı tablosunun eksiksizliği |

## 6. Yeni dil ekleme (3 adım)

1. `internal/settings/language.go` → `SupportedLanguages` + `languageNames`
2. `frontend/src/i18n/locales.ts` → `LOCALES` girdisi (endonym etiket, Intl tag, `dir`)
3. `frontend/src/i18n/locales/<kod>/` altına katalogları kopyala ve çevir

Adım 3 eksikse `catalog.test.ts` kırmızı olur — yarım eklenmiş dil ship edilemez.

## 7. Kapsam ve kalan sınırlar

- **Arayüz metinleri:** Ana gezinme, sohbet, ajanlar, oturumlar, görevler, akışlar,
  otomasyonlar, workspace, ayarlar, araçlar/MCP, skills, market, artifactlar,
  bütçe/panel, içgörü, Rota, harita, loglar, view ve ortak bileşenler katalogları
  kullanır. Düğmeler, yardım/açıklama, placeholder, erişilebilirlik, yerel hata,
  bildirim ve onay metinleri dahildir.
- **Canlı dil değişimi:** Modül yüklenirken çevrilmiş metin saklama. Etiket haritaları
  getter veya çağrı anında çalışan fonksiyon kullanır. Kimlik, enum, select değeri,
  komut ve depolanan kullanıcı verisi çevrilmez. Yerleşik görev sütunlarının
  görünen adları çevrilirken saklanan kanonik değerleri korunur. Yeni akış düğümü
  ve otomasyon şablonu adları oluşturma anındaki dili kullanır; kayıtlı özel
  adlar sonraki dil değişikliklerinde yeniden yazılmaz.
- **İçerik sınırı:** Kullanıcı/ajan metinleri, kaydedilmiş promptlar, akış şablonunun
  yürütülecek gövdesi, kod/komut örnekleri ve model kimlikleri arayüz dilinden bağımsızdır.
  Workspace, Skill, Workflow gibi mevcut ürün/alan terimleri bazı Türkçe etiketlerde
  korunur; bir metnin iki katalogda aynı olması tek başına eksik çeviri değildir.
- **Yerleşik sunucu metadata'sı:** Sağlayıcı/model açıklamaları ve sağlayıcı form
  alanları, bilinen kimlik ve özgün metin eşleşmesiyle çevrilir; özel etiketler ve
  bilinmeyen modeller korunur. Oturum bağlamı etiketleri yapılandırılmış rol ve
  kalibrasyon alanlarından çözülür; bilinmeyen roller özgün etiketi kullanır.
- **Backend hata mesajları:** API `message` alanı Türkçe. Hedef: makine-okunur `code` +
  UI tarafında çeviri.
- **LLM'e giden metin (eksen 3):** `internal/view/*` projeksiyonları Türkçe ve prompt'a
  Türkçe giriyor. Türkçe, Claude tokenizer'ında İngilizce'nin ~2 katı token yer kaplar →
  bunları İngilizce'ye sabitlemek i18n'den bağımsız bir **maliyet** işidir
  (bkz. [17](17-TOKEN-OPTIMIZASYON.md), `MALIYET-DUSURME-PLANI.md`).
- **RTL:** Altyapı hazır (`LocaleMeta.dir`, `<html dir>`), ama mevcut 73k satır fiziksel
  Tailwind yönü kullanıyor. Yeni kodda logical utility (`ps-*`/`pe-*`/`text-start`) tercih et.

## 8. Yol boyunca düzelen gerçek hatalar

- **Türkçe i/İ + CSS `text-transform`:** 59 dosya `uppercase`/`capitalize` kullanıyor.
  `<html lang>` set edilmediği için tarayıcı Türkçe casing kurallarını uygulamıyordu
  ("Sabitlenen" → "SABITLENEN"). `applyDocumentLocale` artık `lang`'ı yazıyor → doğru casing bedava geldi.
- **`toLowerCase()` ile arama:** `searchFold` bilerek `toLocaleLowerCase` KULLANMAZ —
  Türkçe locale'de 'I' → 'ı' eşlemesi "Insight" aramasını bozardı. Arama makine işidir.
- **Sabit `BUCKET_LABELS` map'i:** Modül seviyesindeki etiket sabiti, modül ilk
  yüklendiğindeki dilde donuyordu → `bucketLabel()` fonksiyonuna çevrildi.
- **Hardcoded `'tr-TR'`:** Arayüzde tarih, sayı, para ve süre biçimleri ortak Intl
  katmanını kullanır. URL parametre sırası ve BCP-47 kod sıralaması gibi makine
  işlemleri kullanıcı arayüzünün diline göre değişmez.
