---
title: Oturumlar
description: Konuşma geçmişi, çalışma dizini ve devam davranışı.
order: 2
---

Oturum, bir ajanla yürütülen çok turlu konuşmadır. Bir tur bittiğinde oturum sona
ermez; aynı geçmiş üzerinden yeni mesaj gönderilebilir.

## Kayıtlar

Workspace store'unda her oturumun `session.json` başlığı ve `messages.jsonl`
transkripti vardır. Başlık, etiket ve çalışma dizini gibi metadata transkriptten
ayrı yazılır. Mesaj ekleme JSONL kullanır; silme ve geri sarma gibi işlemler geçmişi
değiştirebildiğinden dosya değişmez bir günlük değildir.

Geçmiş kalıcıdır. Devam etme ve çökme kurtarması, tur durumu ve sağlayıcıya bağlıdır;
uygulamayı yeniden açmak yarım kalmış her harici işlemi otomatik tekrar çalıştırmaz.

## Çalışma dizini ve bağlam

Oturumun çalışma dizini Composer'dan seçilebilir; belirtilmezse workspace
varsayılanı kullanılır. Uzun geçmiş bağlam bütçesine göre sıkıştırılabilir.
CLI sağlayıcılarında devam kimliği ve native sıkıştırma ayrıca izlenir.

## Görünürlük ve kullanım

Arşiv durumu ile turun çalışma sonucu ayrı alanlardır. Arşivlemek, çalışan bir işi
durdurma komutunun yerine geçmez. Token kullanımı ve maliyet tahminleri oturum ve
ajan düzeyinde izlenir; bunlar sağlayıcının faturasının yerine geçen kayıtlar değildir.
