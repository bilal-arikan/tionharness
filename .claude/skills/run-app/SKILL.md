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

## macOS makinesi: kullanıcının terminalindeki sunucu

macOS'ta kullanıcı sunucuyu kendi terminalinden `scripts/serve.sh` ile çalıştırır
(`0.0.0.0:5173`, `TIONHARNESS_ENABLE_SHELL=1`, `bin/tionharness`). Arayüz binary'ye
gömülü olduğundan frontend değişikliği yalnız yeniden derlemeyle görünür.

- **Kullanıcı izni (2026-10-05):** Doğrulama gerekiyorsa bu sunucuyu durdurup aynı portta
  yeniden başlatabilirsiniz; yukarıdaki 4. maddedeki "çalışan sunucuyu kesmeyin" kuralının
  bu makinedeki istisnasıdır. Durdurduğunuzu yanıtta söyleyin.
- Yeniden başlatma: `scripts/serve.sh --ui` komutunu Bash'te arka planda
  (`run_in_background`) çalıştırın. Script arayüzü ve Go binary'sini derler, portu tutan
  eski süreci kendisi kapatır ve aynı adreste başlatır. Günlüğü scratchpad'e yönlendirin.
- `preview_start` burada `serve.sh` çalıştıramaz (macOS izinleri Masaüstü dizinine
  erişimi engeller: `Operation not permitted`); bu yüzden Bash kullanılır. `npx vite`
  ile açılan Vite önizlemesi ise `preview_start` altında çalışır.
- Yeniden başlatmadan sonra açık sekmeler eski chunk adlarını isteyip
  "Failed to fetch dynamically imported module" verebilir; sayfayı tam yenileyin.
