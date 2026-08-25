# 73 — Lokalizasyon (UI i18n)

> **Durum:** Altyapı UYGULANDI (2026-08-25). Kelime çevirileri kademeli devam ediyor.
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
internal/settings/language.go        # SupportedLanguages, EffectiveUILanguage, LanguageDisplayName
frontend/src/i18n/
├── locales.ts                       # dil kaydı: kod, endonym etiket, Intl tag, dir (RTL hazır)
├── catalog.ts                       # import.meta.glob ile katalog toplama
├── bootLocale.ts                    # localStorage önbelleği + resolveUILocale (backend ile aynı mantık)
├── index.ts                         # i18next init, setLocale, <html lang/dir>
├── I18nRoot.tsx                     # dil değişiminde ağacı yeniden bağlar
├── catalog.test.ts                  # parite/boşluk/placeholder guard'ı
└── locales/{en,tr}/common.json
frontend/src/shared/lib/intl.ts      # dateFormat/numberFormat/collator + formatDate/Time/DateTime/compareText
frontend/src/shared/lib/format.ts    # usd/count/decimal/percent/tokens/bytes (locale-duyarlı)
frontend/src/shared/lib/time.ts      # relativeTime/formatDuration/bucketLabel (katalog-tabanlı)
frontend/scripts/codemod-intl.mjs    # tek-seferlik migrasyon aracı (SKIP listesi = bilerek locale-bağımsız yerler)
frontend/i18next-parser.config.js    # npm run i18n:extract
```

## 4. Namespace ve anahtar sözleşmesi

- **Namespace = feature klasörü adı.** `chat`, `tasks`, `flows`… Paylaşılanlar `common`.
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
| `src/shared/lib/time.test.ts` | Anahtarın gerçekten çözülmesi (çoğul eki dahil), locale'e göre sayı/yüzde biçimi |
| ESLint `i18next/no-literal-string` | Migrasyonu bitmiş klasörlerde yeni hardcoded metin. **Allowlist** (`eslint.config.js` → `I18N_MIGRATED`), her feature migre edildikçe büyür |
| `internal/api/settings_test.go` altın liste | `uiLanguage` alanının backend↔frontend tip sürüklemesi |
| `internal/settings/language_test.go` | `UILanguage` "" toleransı, bilinmeyen kodun düşürülmesi, çözümleme sırası, prompt adı tablosunun eksiksizliği |

## 6. Yeni dil ekleme (3 adım)

1. `internal/settings/language.go` → `SupportedLanguages` + `languageNames`
2. `frontend/src/i18n/locales.ts` → `LOCALES` girdisi (endonym etiket, Intl tag, `dir`)
3. `frontend/src/i18n/locales/<kod>/` altına katalogları kopyala ve çevir

Adım 3 eksikse `catalog.test.ts` kırmızı olur — yarım eklenmiş dil ship edilemez.

## 7. Bilinen açıklar / sıradaki iş

- **Kelime çevirileri:** ~2.2k Türkçe literal 450+ dosyada duruyor. Feature-feature
  taşınacak; her feature bitince yolu `I18N_MIGRATED` allowlist'ine eklenmeli.
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
- **Hardcoded `'tr-TR'`:** 60+ çağrı yeri Intl katmanına taşındı. Bilerek dışarıda
  bırakılanlar `codemod-intl.mjs` içindeki `SKIP` listesinde gerekçesiyle duruyor
  (URL parametre sırası, BCP-47 kod sıralaması — bunlar makine verisi, kullanıcı metni değil).
