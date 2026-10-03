---
name: run-app
description: >
  Bir değişikliği çalışan TionHarness API/UI üzerinde doğrulamak için yerel
  sunucuyu ve tarayıcıyı açar. "/run-app", "uygulamayı başlat", "çalıştır ve bak"
  veya canlı UI/API kontrolünde kullanılır. Başlatıcılar `.claude/launch.json` içindedir.
---

# Uygulamayı çalıştırma

| Yapılandırma | İşlev | Adres |
|---|---|---|
| `tionharness` | Temiz derlemeli Go sunucusu, gömülü arayüz, varsayılan veri dizini | http://127.0.0.1:8090 |
| `frontend-dev` | Vite geliştirme sunucusu; API proxy'si 8090'a bağlıdır | http://127.0.0.1:5173 |
| `tionharness-scratch` | 8090'da, geçici ve ayrı veri dizinli Go sunucusu | http://127.0.0.1:8090 |
| `tionharness-dev` | Yalnız geliştirme backend'i, varsayılan veri dizini | http://127.0.0.1:8090 |

1. **API veya gömülü UI:** `preview_start` ile `tionharness` yapılandırmasını
   başlatın. `scripts/serve.ps1` arayüzü ve Go binary'sini sıralı derler; ayrı
   `go run` sarmalayıcısı kullanılmaz.
2. **Arayüz geliştirme:** `tionharness-dev` ve `frontend-dev` yapılandırmalarını
   başlatıp Vite adresini açın. Backend ve proxy aynı 8090 portunu kullanır.
3. Veri dizini varsayılan `~/.tionharness` yoludur; `TIONHARNESS_DATA_DIR` değiştirir.
   Kullanıcının çalışan örneği `instance.lock` tutuyorsa onu kapatmayın. Ayrı veri
   dizini için `tionharness-scratch` kullanın; aynı 8090 portu doluysa onu da başlatmayın.
4. Başlatıcılar port doluyken süreç öldürmeden çıkar (`-NoKillPort`). Çalışan sunucuyu
   veya kullanıcı oturumlarını sırf doğrulama için kesmeyin. Binary'nin doğrudan
   varsayılanı 8080'dir; bu geliştirme yapılandırmaları 8090'ı açıkça seçer.
5. İlgili ekranı tarayıcı araçlarıyla veya API yanıtını `curl` ile kontrol edin.
   Bearer auth isteğe bağlıdır; günlükleri `preview_logs` ile okuyun.

İş sonunda yalnız bu kontrol için başlattığınız sunucuları `preview_stop` ile kapatın.
