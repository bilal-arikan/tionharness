---
title: Giriş
description: TionHarness kapsamı ve temel bileşenleri.
order: 1
---

TionHarness, kendi makinenizde çalışan çoklu ajan ortamıdır. Tek çalıştırılabilir
dosya HTTP API ve web arayüzünü sunar; kayıtlar JSON/JSONL dosyalarında tutulur.
Windows'ta WebView2 kullanan ayrı masaüstü derlemesi de vardır.

## Neler yapabilirsiniz?

Ajanlara sağlayıcı, model, talimat ve araçlar atayabilir; sohbet oturumları açabilir;
görevleri panoda izleyebilir; akış, zamanlama ve olay tetikli otomasyonlarla işleri
birleştirebilirsiniz. Koordinatör ajanlar işçi ajanlara görev devredebilir.

CLI sağlayıcıları kendi oturum açma mekanizmalarını kullanır. API sağlayıcılarında
anahtar gerekir; LM Studio gibi yerel bir sunucu da bağlanabilir. Uygulamayı açmak
ücretli bir model çağrısı gerektirmez, ajan çalıştırmak uygun sağlayıcı gerektirir.

## Veri ve erişim

Geçmiş yerel disktedir; uzak sağlayıcıya veya harici araca gönderilen içerik makineden
çıkabilir. Workspace, dosya sistemi güvenlik sınırı değildir. Normal HTTP erişimi
varsayılan olarak loopback üzerindedir ve kimlik doğrulama varsayılan kapalıdır.

[Kurulum](/docs/getting-started/installation) ile derleyin,
[hızlı başlangıç](/docs/getting-started/quickstart) ile ilk oturumu açın.
