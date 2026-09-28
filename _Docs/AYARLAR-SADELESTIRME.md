# Ayarlar düzeni ve kayıt davranışı

> **Özet (2026-09-28):** Ayarlar işlevlerine göre ayrıldı; referans içeriği ayrı
> Yardım grubuna taşındı. Kayıt ve değişiklik göstergeleri kategoriye özeldir.
> Ayar anahtarları, varsayılanlar ve çalışma zamanı sınırları korunmuştur.

## Bölümler

| Bölüm | İçerik |
|---|---|
| General | Ekranı açık tutma, otomatik başlık ve etiketleme |
| Sound & notifications | Genel bildirim anahtarı, cihazın bildirim türleri ve ses tercihleri |
| Providers | Sağlayıcı bağlantıları ve açılır Anthropic API / CLI seçenekleri |
| Tool permissions | Kabuk erişimi ve yerel kod modu |
| Agent execution | Delegasyon, arka plan, araç, koordinatör ve otonomi sınırları |
| Context & memory | Bağlam bütçesi; açılır handoff, kalıcı ilerleme ve kurtarma bölümleri |
| Diagnostics | Oturum tanılama günlüğü ve saklanan olay sayısı |
| Help → Reference | Komutlar ve aktivite adımı türleri; salt okunur |

Profil, sırlar, ajan kütüphanesi, karar mercileri, hooks, harici araçlar ve yedekleme
işlevleri korunur. Hakkında sayfası Yardım grubundadır.

## Kayıt kuralları

- Genel Kaydet yalnız düzenlenebilir taslak sahibi kategorilerde görünür.
- Kaydet yalnız o kategoride değişen alanları gönderir. Başka kategorilerdeki
  bekleyen düzenlemeler ve kayıt sırasında yapılan yeni düzenlemeler korunur.
- Sağlayıcı bağlantı formları kendi kayıt işlemlerini yürütür. İleri sağlayıcı
  seçeneklerinin kendi bölümünde ayrı bir Kaydet alanı vardır.
- Ses ve bildirimlerde genel Kaydet yoktur. Genel bildirim anahtarı anında
  sunucuya kaydedilir; hata halinde önceki değer korunur ve hata gösterilir.
- Bildirim türleri ve ses tercihleri cihazda anında uygulanır. Genel/cihaz
  kapsamları ekranda ayrı açıklanır; mevcut depolama kapsamı değiştirilmez.
- Salt okunur ve kendi kaydını yöneten bölümler başka kategorideki değişiklikler
  yüzünden değişmiş olarak işaretlenmez.
- Kapalı açılır bölümlerdeki geçersiz sayı girdileri de kaydı engeller.

## Eski bağlantılar

`settings/advanced` Genel'i, `settings/commands` Referans'ın Komutlar bölümünü,
`settings/stepkinds` Referans'ın Adım Türleri bölümünü açmaya devam eder.

## Uygulama sınırı

Sunucu ayar şeması ve çalışma zamanı davranışları değiştirilmedi. Sağlayıcı
ekranına artık aktarılmayan eski test/taslak/yol parametreleri ve bunları besleyen
gereksiz istekler kaldırıldı. Alt formlar ayrı dosyalardadır; kategori alan
sahipliği `settingsFields.ts` içinde tanımlanır.
