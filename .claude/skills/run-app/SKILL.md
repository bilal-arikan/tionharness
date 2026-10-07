---
name: run-app
description: >
  Bir değişikliği çalışan TionHarness API/UI üzerinde doğrulamak için yerel
  sunucuyu ve tarayıcıyı açar. "/run-app", "uygulamayı başlat", "çalıştır ve bak"
  veya canlı UI/API kontrolünde kullanılır. Başlatıcılar `.claude/launch.json` içindedir.
---

# Uygulamayı çalıştırma

Önce işletim sistemini belirleyin (`uname -s`: `Darwin`/`Linux` → macOS/Linux; Git Bash'te
`MINGW*` → Windows) ve yalnız o sütundaki yapılandırmayı kullanın. Windows girdileri
`powershell.exe` ister; macOS/Linux girdileri `bash` + PATH'teki `go` ile çalışır.

| İşlev | Windows | macOS / Linux | Adres |
|---|---|---|---|
| Temiz derlemeli Go sunucusu, gömülü arayüz, varsayılan veri dizini | `tionharness` (`scripts/serve.ps1`) | `tionharness-mac` (5173'te; arayüz `scripts/serve.sh --ui` ile derlenir) | Win: 127.0.0.1:8090 · mac: 127.0.0.1:5173 |
| Yalnız geliştirme backend'i, varsayılan veri dizini | `tionharness-dev` (`scripts/dev.ps1 -BackendOnly`) | `tionharness-mac-dev` (`bin/tionharness-dev`; `scripts/dev.sh --backend-only` eşdeğeri) | http://127.0.0.1:8090 |
| Geçici, ayrı veri dizinli Go sunucusu | `tionharness-scratch` (8090) | `tionharness-mac-scratch` (8091, `$TMPDIR/th-scratch-data`) | Win: 8090 · mac: 8091 |
| Vite geliştirme sunucusu; API proxy'si 8090'a bağlıdır | `frontend-dev` | `frontend-dev` | http://127.0.0.1:5173 |

1. **API veya gömülü UI:** Windows'ta `tionharness` yapılandırmasını başlatın;
   `scripts/serve.ps1` arayüzü ve Go binary'sini sıralı derler, ayrı `go run`
   sarmalayıcısı kullanılmaz. macOS/Linux'ta karşılığı `tionharness-mac`'tir; ama bu
   makinede kullanıcının sunucusu zaten 5173'tedir, aşağıdaki "macOS makinesi" bölümüne
   bakın.
2. **Arayüz geliştirme:** `tionharness-dev` (macOS/Linux: `tionharness-mac-dev`) ve
   `frontend-dev` yapılandırmalarını başlatıp Vite adresini açın. Backend ve proxy aynı
   8090 portunu kullanır. `frontend-dev` 5173'ü kullandığından macOS'taki
   `tionharness-mac` / `scripts/serve.sh` (varsayılan 5173) ile aynı anda çalışamaz.
   Terminalden tek komut: `scripts/dev.sh` (macOS/Linux) / `.\scripts\dev.ps1` (Windows).
3. Veri dizini varsayılan `~/.tionharness` yoludur; `TIONHARNESS_DATA_DIR` değiştirir.
   Kullanıcının çalışan örneği `instance.lock` tutuyorsa onu kapatmayın. Ayrı veri
   dizini için `tionharness-scratch` (macOS/Linux: `tionharness-mac-scratch`, 8091'de;
   kullanıcının sunucusuyla yan yana çalışabilir) kullanın. Windows scratch'i aynı 8090
   portunu kullanır; port doluysa onu da başlatmayın.
4. Başlatıcılar port doluyken süreç öldürmeden çıkar (`-NoKillPort` / `--no-kill-port`;
   `scripts/dev.sh` bu bayrakla kilit tutan başka bir örnek görürse de çıkar). Çalışan
   sunucuyu veya kullanıcı oturumlarını sırf doğrulama için kesmeyin. Binary'nin doğrudan
   varsayılanı 8080'dir; bu geliştirme yapılandırmaları portu açıkça seçer.
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
- `preview_start` burada `serve.sh` çalıştıramaz (macOS izinleri `bash`'in Masaüstü
  dizinindeki dosyaları okumasını engeller: `Operation not permitted`); bu yüzden Bash
  kullanılır. `npx vite` ile açılan Vite önizlemesi ise `preview_start` altında çalışır.
  Aynı nedenle `tionharness-mac*` girdileri script dosyası çağırmaz, `bash -c` içinde
  doğrudan `go build` + `exec bin/...` yapar (2026-10-07'de `tionharness-mac-scratch`
  ile doğrulandı). Bu ortamda git `.git`'i okuyamadığından `/api/version` commit'i
  `unknown` görünür; derleme tarihi doğrudur.
- `tionharness-mac-scratch` boş bir veri diziniyle açılır: workspace oluşturulana kadar
  API `409 no active workspace` döner.
- Yeniden başlatmadan sonra açık sekmeler eski chunk adlarını isteyip
  "Failed to fetch dynamically imported module" verebilir; sayfayı tam yenileyin.
