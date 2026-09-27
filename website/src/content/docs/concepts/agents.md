---
title: Ajanlar
description: Kimlik, sağlayıcı, araç erişimi ve delegasyon.
order: 1
---

Ajan; adı, talimatları, sağlayıcı örneği, modeli, düşünme seviyesi ve araç
politikası olan kalıcı bir yapılandırmadır. Aynı ajanla birden çok oturum açılabilir.

## Sağlayıcı ve talimatlar

Sağlayıcı örnekleri uygulama genelinde tanımlanır; ajan hangi örneği kullanacağını
seçer. Soul alanı davranış talimatlarını taşır. Workspace talimatları ve oturumun
çalışma dizini de yürütme bağlamına katılır.

Yerleşik sistem ajanları kilitlidir. Özelleştirme için türetilmiş ajan kullanılır;
devralınan alanlar ile açıkça değiştirilmiş alanlar ayrı tutulur.

## Araçlar ve izinler

Yerleşik araçlar ve harici MCP sunucuları ayrı ayrı etkinleştirilebilir. Bazı araç
şemaları talep üzerine yüklenir. `auto`, `ask` ve `read-only` izin modları işlem
politikasını belirler; işletim sistemi sandbox'ı oluşturmaz. CLI sağlayıcılarının
native araç/onay davranışları ayrıca kendi taşıyıcısına bağlıdır.

## Delegasyon ve arşiv

Koordinatör işçi oturumları başlatıp sonuçlarını toplayabilir. Bu oturumlar aynı
sohbetin mesajları olmak yerine kendi geçmişi ve köken bilgisi olan kayıtlardır.
Arşivli ajan çalıştırılamaz ve yeni görev, zamanlama veya otomasyon hedefi olarak
atanamaz; yeniden kullanmak için arşivden çıkarılmalıdır.
