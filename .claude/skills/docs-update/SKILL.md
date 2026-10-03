---
name: docs-update
description: >
  Bu depodaki Türkçe dokümanları günceller: tarihli kaydı `_Docs/05-ILERLEME.md`
  dosyasının başına ekler, ilgili dokümanın Özet bloğunu ve
  `_Docs/00-GENEL-BAKIS.md` indeksini yeniler. Özellik/hata düzeltmesi sonrası,
  "/docs-update", "dokümanı güncelle" veya davranış değiştiren commit öncesinde kullanılır.
---

# Doküman güncelleme düzeni

Dokümanlar Türkçe; kod ve kod yorumları İngilizcedir. Numaralı `_Docs/NN-*.md`
dosyaları H1 başlığından sonra `> **Özet (YYYY-MM-DD):** ...` bloğu içerir.
Özet 3–5 cümlede konuyu, mevcut durumu, önemli kararları ve ilgili paketleri açıklar.
Değişen durum veya kararları gövdede ve özette birlikte düzeltin.

1. **İlerleme kaydı:** yeni bölümü `_Docs/05-ILERLEME.md` içindeki H1, Özet ve
   arşiv gezinmesinden sonra, mevcut tarihli kayıtların önüne ekleyin. En yeni
   tarih üstte kalır; aynı gün içindeki kayıtların mevcut sırasını koruyun.

   ```markdown
   ## Kısa başlık (YYYY-MM-DD)

   Ne değişti, neden, ilgili dosyalar ve doğrulama sonuçları.
   ```

2. **Konu dokümanı:** sahibi olan rehberi indeksten bulun; artık geçerli olmayan
   plan/durum metnini güncelleyin. Geçmiş ölçümleri yeni davranışın kanıtı gibi sunmayın.
3. **Yeni doküman:** mevcut numara ve konu ailesini kontrol edin. İlgisiz yeni konuya
   en yüksek numaradan sonrakini verin; aynı konu ailesinin alt rehberinde mevcut
   numara ve farklı dosya adı kullanılabilir. Dosyayı indeks tablosuna ekleyin.
4. **Uzun günlük:** eski ayları `arsiv/05-ILERLEME-YYYY-MM.md` dosyalarına taşıyın.
   Her tarihsel bölüm ve ilgili bağlantı korunur; ana günlük arşivlere bağlantı verir.
   Tarihsiz tarihsel kayıtların içeriğine tahmini tarih eklemeyin.
5. **Ajan kuralları:** her tur geçerli kısa kurallar `CLAUDE.md`'ye; ayrıntılı
   referans `_Docs/80-AJAN-REFERANSI.md`'ye gider.
6. **Son kontrol:** yerel bağlantıları ve tarih sırasını doğrulayın; `.md`/`.yml`
   dosyalarında satır sonu boşluk bırakmayın ve `git diff --check` çalıştırın.

Tarihler mutlak yazılır (`2026-10-03`); "bugün" ve "dün" kullanılmaz.
