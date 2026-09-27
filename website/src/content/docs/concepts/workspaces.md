---
title: Workspace'ler
description: Depolama kapsamı, paylaşılan ayarlar ve çalışma dizini.
order: 3
---

Workspace; ajanları, oturumları, panoyu, akışları ve zamanlamaları bir araya getirir.
Her workspace'in ayrı store'u, runtime'ı ve scheduler'ı vardır.

## Ayrı ve ortak veriler

Workspace verileri kendi dizininde saklanır. Sağlayıcı örnekleri, genel ayarlar ve
global skill'ler uygulama düzeyinde paylaşılır; dolayısıyla workspace'ler tamamen
bağımsız kullanıcı hesapları değildir.

Workspace değiştirmek arayüzdeki veri kapsamını değiştirir. Arka planda çalışan
diğer workspace işleri sırf bu seçim değişti diye durmaz.

## Çalışma dizini

Proje klasörü araçların varsayılan çalışma dizinidir; oturum bunu değiştirebilir.
Bu klasör dosya sistemi erişim sınırı değildir. Özellikle shell komutları yalnız
workspace altında çalışmak zorunda değildir. İzin modunu ve araçları göreve göre seçin.

## Talimatlar ve taşıma

Workspace talimatlarına ortak proje kurallarını yazabilirsiniz. Şablon dışa aktarımı
bir tam yedek değildir; sırlar ve CLI oturumları gibi uygulama-geneli verilerin aynı
paketle taşınacağını varsaymayın. Kalıcı verileri taşırken uygulama veri dizinini,
workspace konumlarını ve şifreleme anahtarını birlikte değerlendirin.
