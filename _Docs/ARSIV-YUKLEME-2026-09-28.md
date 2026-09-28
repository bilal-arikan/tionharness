# Arşivin isteğe bağlı yüklenmesi

> **Özet:** Normal liste istekleri arşiv kayıtlarını döndürmez. Arşiv görünümü açılınca ilgili kayıtlar istenir; doğrudan kayıt bağlantıları ve konuşma yazarları tekil sorgularla çözümlenir. Görev panosunun açılışındaki üç görev isteği bire indirilmiştir.

## Kapsam

- Ajan, skill, artifact ve hedef liste uçlarının varsayılanı aktif kayıtlardır. `archived=true` yalnız arşivi, `archived=all` iki tarafı döndürür.
- Oturum listesi, filtre verilmediğinde arşivi dışlar. Arşiv çipi, `state=archived`, `state=all` veya belirli `ids` sorgusu açık taleptir. Çip sayıları mevcut sözleşmesini korur.
- Görev panosu aktif/arşiv tarafını sunucudan seçer. `archived=only` yalnız arşiv içindir; eski istemcilerin `archived=1/true` ile tüm listeyi istemesi korunur.
- Ajan ve skill arşivleri ilk açılışta çekilmez. Artifact ekranı arşiv sayacını öğrenmek için arka planda arşiv kaydı istemez; sayaç arşiv açıldığında öğrenilir.
- Konuşmada gereken arşivlenmiş ajan bilgisi tekil okunur. Bu ek kayıtlar seçim listesine katılmaz, görünümün referansları değişince bırakılır.
- Görev, ajan ve hedef ayrıntılarına doğrudan bağlantılar tüm arşivi çekmeden çalışır. Geç gelen liste yanıtları, daha yeni aktif/arşiv seçimini değiştiremez.

## WS5 gözlemi

İnceleme sırasında WS5 `store/tasks` altında 332 görev bulundu; 247'si arşivlenmişti. Aktif görev API'si bunları zaten dışlıyordu. Bulunan ek maliyet, panonun aynı aktif listeyi açılışta üç ayrı efektten istemesiydi. Bu yüzden değişiklik için süre kazancı iddiası yerine istek sayısı regresyon testi kullanıldı.

## Sınır

Bu çalışma arayüze gönderilen listeleri ve isteklerin zamanlamasını değiştirir. Dosya deposunun açılışta tuttuğu kayıt/başlık haritalarını diskten tembel yükleyen yeni bir mimari kurmaz; arşiv dosyalarını silmez veya taşımaz.

## Doğrulama

API regresyonları varsayılan listede arşiv bulunmamasını, açık arşiv sorgularını, tekil ayrıntı erişimini ve geri yüklemeyi denetler. Arayüz testleri görev panosunun tek açılış isteğini, arşivin kullanıcı seçimiyle yüklenmesini, eski yanıtların reddedilmesini ve konuşma yazarlarının yalnız gerektiğinde okunmasını kapsar. Teslim kapısı `scripts/test.sh full`; ayrıca TypeScript denetimi çalıştırılır.
