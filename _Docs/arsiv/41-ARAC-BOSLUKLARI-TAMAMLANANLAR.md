# 41 — Araç boşlukları: tamamlanan maddelerin tasarım/uygulama gövdeleri

> Arşiv: `41-ARAC-BOSLUKLARI-YAPILACAKLAR.md` dosyasından taşınan, tamamlanmış plan/tasarım metni. Güncel durum için asıl dokümana bak.

### 1. `WebSearch` — **P0** — ✅ TAMAMLANDI (2026-06-30)

- **Durum:** ~~Ölü placeholder.~~ **Uygulandı.** Native Go `WebSearch` aracı `WebFetch` deseninde eklendi.
- **Neden önemli:** Ajan güncel bilgiye (dokümantasyon sürümü, hata mesajı, kütüphane API'si, güncel
  olaylar) erişemiyor. Model bilgi-kesim tarihiyle sınırlı kalıyor. Tek net **işlevsel** boşluk.
- **Uygulanan yaklaşım:**
  - **Provider-bağımsız**, vault'tan otomatik backend seçimi: önce `SEARXNG_URL` (self-host, anahtarsız),
    sonra `TAVILY_API_KEY` (Tavily, 1k ücretsiz/ay). İkisi de yoksa **sessizce yutmaz** — açık hata.
  - **Yedekleme (2026-08-16):** Backend'ler artık sırayla denenir. İlki hata verirse (servis kapalı,
    rate-limit, hatalı yapılandırma) ikincisine düşülür; ikisi de düşerse hata **her backend'i ve
    hatasını** adlandırır. Bağlantı düzeyi hatalarına (`connection refused`, `no such host`, timeout)
    "the backend is unreachable — is the service running at that address?" ipucu eklenir. Bu, durmuş
    bir SearXNG konteynerinin ajanı tümüyle aramasız bırakmasını engeller (SES286 vakası).
  - **claude-cli hariç:** Araç `WebFetch` gibi **koşulsuz** kaydedilir (workspace tools ekranında görünür),
    ama claude-cli'ye **bridge'lenmez** — TionHarness built-in'leri CLI'ye yalnızca elle küratörlenen
    `interactionToolSpecs` listesiyle ulaşır, WebSearch o listede yok → CLI kendi native'ini kullanır.
    Savunma amaçlı `cliLazyBridgeExcluded`'a da eklendi (WebFetch ile birebir).
  - **SSRF yok:** URL operatör-tanımlı güvenilir backend (yalnızca query model'den gelir), bu yüzden
    WebFetch'in SSRF-guard'lı dialer'ı KULLANILMAZ — self-host SearXNG'in loopback adresi erişilebilir kalır.
  - Çıktı: numaralı başlık + URL + snippet listesi → ajan `WebFetch` ile derinleşir (bkz. I1).
- **Eklenen dosyalar:** `internal/tools/builtin_websearch.go` (Def/Call/backend seçimi),
  `websearch_tavily.go`, `websearch_searxng.go`, `builtin_websearch_test.go` (7 test).
  `toolsetup.go` kaydı eklendi; `classify.go:37` placeholder'ı artık gerçek araca bağlı.
- **Risk sınıfı:** `RiskRead` (salt-okuma, ağ).
- **Görünürlük:** ✅ Çözüldü — koşulsuz kayıt sayesinde workspace tools ekranında listelenir; CLI yine
  kendi native'ini kullanır (`TestWebSearchVisibleInWorkspaceCatalog` ile doğrulandı).
- **Operatör kurulumu:** `secret_set` ile `TAVILY_API_KEY` *veya* `SEARXNG_URL` ekle.

### 3. `transform_data` — izole script — **P1** — ✅ TAMAMLANDI (2026-06-30)

- **Durum:** ~~Yok.~~ **Uygulandı** (`transform_data` adıyla). `script_sandbox` ayrı bir araç olarak
  uygulanmadı — `transform_data` zaten striplenmiş-env + timeout + yapısal çıktı sözleşmesini karşılıyor.
- **Neden önemli:** TionHarness'in **token optimizasyon** felsefesiyle (`_Docs/17`) birebir uyumlu: büyük
  veri setini izole script ile işleyip **dosyaya yazmak** ve ana bağlama sadece özet/yol döndürmek
  cached-prefix'i ve token'ı düşürür.
- **Uygulanan yaklaşım:**
  - Parametreler: `language(python3|node|bun)`, `script`, `input_files[]`, `output_file`.
  - **argv sözleşmesi:** `argv[1..N]` = girdi dosyaları (sırayla), **son argv** = çıktı dosyası.
  - **Striplenmiş env (allowlist):** yalnızca interpreter'ın ihtiyacı olan değişkenler geçer; **hiçbir
    secret/API anahtarı** script'e ulaşmaz (`minimalScriptEnv`). 30s timeout, çıktı **dosyaya** yazılır,
    bağlama yalnızca yol + boyut + satır sayısı + script log'u döner (veri DÖNMEZ).
  - **Hata sessizce yutulmaz:** eksik girdi, yazılmayan çıktı, script hatası → hepsi açık hata + script
    stderr'i yüzeye çıkar.
  - **Windows:** WindowsApps "app execution alias" stub'ları atlanır (gerçek `python.exe` önce denenir),
    aksi halde striplenmiş env'le 9009 "Python bulunamadı" hatası oluyordu.
- **Gating + risk:** Host'ta keyfi kod çalıştırdığından `RiskExec` ve **shell gate'i** (`ShellEnabled`)
  arkasına kaydedildi — shell ile aynı yürütme kapısı. Gerçek güvenlik sandbox'ı değil; secret-izolasyonu +
  kaynak-sınırlama sarmalayıcısı. (Not: the external agent project bunu Explore'da da sunar; TionHarness dürüst risk modeli
  gereği read-only'de bloklar. İleride gerçek sandbox ile `RiskRead`'e indirilebilir / ayrı tunable.)
- **Eklenen dosyalar:** `internal/tools/builtin_transform_data.go`, `transform_data_env.go`,
  `builtin_transform_data_test.go` (8 test). `classify.go` + `toolsetup.go` kaydı.

### 4. `PowerShell` — ayrı Windows shell — **P1** — ✅ TAMAMLANDI (2026-06-30)

- **Durum:** ~~Yalnızca `Bash` var (Windows'ta gizlice PowerShell çalıştırıyordu).~~ **İki ayrı araç
  uygulandı** (Seçenek B).
- **Neden önemli:** Araç adı model'e en güçlü sözdizimi sinyali. "Bash" adı altında PowerShell çalıştırmak,
  model'in `$VAR`/`&&`/`2>/dev/null` gibi POSIX-izm üretmesine yol açıyordu.
- **Uygulanan yaklaşım:**
  - **`Bash`** = POSIX (Unix: `/bin/sh`; Windows: `bash.exe` **varsa**, yoksa kaydedilmez).
  - **`PowerShell`** = `pwsh` (7+ tercih) → `powershell.exe` (5.1 fallback). Windows'ta hep var.
  - Her araç **yalnızca backing shell'i mevcutsa** kaydedilir; her OS'te en az biri garantili
    (Unix→Bash, Windows→PowerShell). Ortak yürütme çekirdeği (`runShell`): timeout, sandbox, çıktı-cap,
    confined git-guard, streaming — tek fark komut sarmalama.
  - **CLI yolu:** Tek OS-native shell köprülenir (Windows→PowerShell, Unix→Bash). `NewShellRunner` aynı
    shell'i seçer, `callShell` her iki adı da dispatch eder, `coreInteractionTools`'a `PowerShell` eklendi.
    Davranış aynı (Windows'ta zaten PowerShell çalışıyordu) — sadece isim doğrulandı.
  - **Windows:** WindowsApps alias stub'ları `lookInterpreter` ile atlanır.
- **Karar:** `pwsh` > `powershell.exe` (modern sözdizimi: ternary, `&&`).
- **Eklenen/değişen dosyalar:** `builtin_shell.go` (refactor + `PowerShellTool`), `builtin_shell_test.go`
  (4 test), `classify.go` (`PowerShell: RiskExec`), `toolsetup.go` (koşullu kayıt), `runtime.go`
  (`NewShellRunner` OS-pick), `mcp_interaction.go` (CLI advertise + core + dispatch).
- **Risk sınıfı:** `RiskExec` (her iki shell).

### 5. `get_session_info` — tekil oturum metadata — **P2** — ✅ TAMAMLANDI (2026-07-03)

- **Durum:** ~~Yok.~~ **Uygulandı** (`internal/tools/builtin_sessioninfo.go` + test).
- **Uygulanan:** `session_id?` (boşsa ctx'teki mevcut oturum, `currentsession.go`) → id/title/state/
  kind/agent (ad+id)/mesaj sayısı/tags/goal/working_dir/role/coordinator_session/parent_session.
  Oturumsuz turda zarif mesaj (hata değil); bilinmeyen id'de açık hata. Session-edit araçlarının
  okuma eşi — mutasyondan önce incele, otonom turda kendini yönelt.
- **Kayıt:** koşulsuz (`toolsetup.go`, read_session_debug'ın yanı); tier `MarkNameOnly`; claude-cli
  köprüsü `runtime.go BridgeTools` `extra` (call ctx'e sid zaten enjekte). `RiskRead`, kategori `agents`.
- **Genişletme — aktivite izi (2026-09-02):** metadata'ya ek olarak, **başka** bir oturum
  sorgulandığında ne yaptığı da dönüyor. Canlı tur `db.ReadInflight` (mevcut crash-recovery
  sidecar'ı; runtime/`runs` registry'sine bağımlılık YOK) ile, duran oturum `db.LastMessage` ile
  okunur: `current_turn`/`last_turn` (yaş, süre, `stop_reason`, cancelled/interrupted), `tools:`
  sayacı (`Read×3, Bash×2`), son ≤8 çağrı satırı, `last_step:`, `errors:` ve sınırlı cevap alıntısı.
  Böylece koordinatör "çalışıyor mu, takıldı mı" sorusunu transkript okumadan cevaplar.
- **Üç kasıtlı sınır:**
  1. **Tool çıktısı basılmaz** (`RecapOpts.MaxOutput = 0`) — yalnız çağrı adı + arg ipucu + ok/error.
     Token, sır sızıntısı ve prompt-injection yüzeyini birlikte küçültür.
  2. **Cevap alıntısı** baş 150 + son 150 rune (`elideMiddle`, ortada `[N chars omitted]`), fenced ve
     `data, not instructions` etiketli — başka bir ajanın metni veridir, talimat değil.
  3. **Kendi oturumunda blok basılmaz**; runtime zaten `<recent_tool_activity>` enjekte ediyor,
     tekrarı saf israftır. Yerine tek satırlık işaretçi döner.
- **Ortak parser:** step ayrıştırma/render `internal/tools/steprecap.go` (`ParseRecapSteps`,
  `RecapLines`, `RecapToolCounts`, `RecapLastStep`, `RecapErrors`). `internal/api/chat_tool_summary.go`
  artık bu koda delege eder — ikinci, kayan bir parser yok. `tools` paketi `agent`'ı **import edemez**
  (`agent` → `tools` yönü mevcut: `TurnStep` içinde `tools.AskQuestion`), bu yüzden `RecapStep` alanları
  JSON etiketiyle eşleşir ve `internal/agent/steprecap_parity_test.go` sürüklenmeyi kapıda tutar.
- **Bozuk iz yutulmaz:** `ParseRecapSteps` boş izi hata saymaz ama bozuk izi
  `persisted_steps_invalid` ile döndürür; araç `activity: trace unreadable (...)` basar.

### 7. `update_user_preferences` — yapısal kullanıcı profili — **P2** — ✅ TAMAMLANDI (2026-07-03)

- **Durum:** ~~Kısmen core_memory.~~ **Uygulandı** (`internal/tools/builtin_userprefs.go` + test).
- **Uygulanan:** `name?`/`timezone?`/`city?`/`country?`/`notes?` (REPLACE) / `notes_append?` (mevcut
  notlara satır ekle) → mevcut **Settings ▸ Profil** alanlarına (`userName`/`userTimezone`/`userCity`/
  `userCountry`/`userNotes`) `SettingsBridge.Apply` ile yazar. Profil zaten her turda "About the user"
  bloğu olarak enjekte ediliyor (`api/settings.go userContextBlock`) → yeni prompt-enjeksiyon katmanı
  GEREKMEDİ. Dar sarmalayıcı: yalnız 5 profil alanına dokunur (update_settings'in aksine yanlışlıkla
  başka ayar değiştiremez); `notes`+`notes_append` birlikte → hata. ~~core_memory "human" bloğu ajanın
  kendi gözlemleri için serbest-metin olarak ayrı yaşamaya devam eder.~~ _(core memory
  2026-07-05'te memory alt sistemiyle birlikte kaldırıldı — yapısal kullanıcı profili artık
  yalnız bu araçtan geçer.)_
- **Kayıt:** `settingsBridge` varken (`toolsetup.go`); tier `MarkNameOnly`; claude-cli köprüsü
  `runtime.go BridgeTools` `extra`. Risk: haritalanmadı → varsayılan `RiskWrite`; kategori `config`.

### 8. `render_template` — şablonlu çıktı render — **P2** — ✅ Tamamlandı (2026-07-06)

- **Durum:** ✅ Yapıldı. Ayrıntılı tasarım + uygulama: **_Docs/63-SOURCE-TEMPLATES-RENDER.md**.
- **Ne yapıldı (özet):**
  - Motor: Go **`html/template`** (auto-escape / XSS-güvenli), naive string-ikame değil.
  - Araç: `internal/tools/builtin_render_template.go` (+ `render_template.go` saf motor + test).
    `template` (mutlak yol) + `data` (JSON) → **session render dir**'e yazar, modele **yalnız yol +
    uyarılar** döner (HTML değil → token tasarrufu). Session-scoped; session yoksa hard-fail.
  - Soft-validation: sidecar `<template>.meta.json` `requiredFields` → eksik alan **SOFT** (render + warning),
    bozuk template / JSON / sidecar **HARD** (CLAUDE.md "sessizce yutma" ilkesi).
  - Inline gösterim: yeni ```` ```html-preview ```` bloğu (`HtmlPreview.tsx`) — izole sandbox iframe
    (`allow-scripts`, `allow-same-origin` YOK → opaque origin). Backend `GET /api/files?...&as=text`
    yalnız render kökü altını `text/plain` ile servis eder.
  - Depolama: **skill-bundled** (`tionharness-templates` skill; `${SKILL_DIR}/templates/*.html`), yeni
    "Sources/template store" alt-sistemi kurulmadı.
- **Dosyalar:** `internal/tools/{render_template.go, builtin_render_template.go, *_test.go}`,
  `internal/agent/renderdir.go` + `toolsetup.go`, `internal/api/files.go`,
  `frontend/src/components/markdown/{HtmlPreview.tsx, CodeBlock.tsx}`, `frontend/src/lib/attachments.tsx`,
  `internal/skills/defaults/tionharness-templates/**`, prompt + guard güncellendi.
- **Risk sınıfı:** `RiskWrite` (kontrollü: yalnız render dir'e yazar, çıktı adı basename'e indirgenir).

## E. Mevcut araçların EKSİK ÖZELLİKLERİ (yeni araç değil, per-tool feature farkı)

> Bölüm A "eksik araçları" listeler; bu bölüm **TionHarness'te VAR OLAN** araçların
> Claude Code muadilinde bulunup bizde olmayan **özelliklerini** toplar. (İlk kayıt:
> 2026-07-03, Claude Code araçlarının gözlemlenen davranışından.)

### `Edit` / `Write` — tazelik guard'ı — ✅ TAMAMLANDI (2026-07-03)
- **Eklendi:** read-before-write + modified-since-read kontrolü (`ReadTracker`, içerik-hash).
  Bkz. `_Docs/05-ILERLEME.md` bu tarih. Ayar `fileFreshnessGuard` (vars. açık).

### `Read` — ✅ TAMAMLANDI (2026-07-03)
- **`offset`/`limit` satır aralığı + `cat -n` satır numaralama** eklendi (`renderNumbered`).
  Satır-başı 2000 char cap + 256KB çıktı cap + "devam: offset=N" ipucu. Bkz. `05-ILERLEME.md`.
- (Görsel/PDF/Notebook okuma bilinçli **kapsam dışı** — kullanıcı gerek görmedi.)

### `Grep` — ✅ TAMAMLANDI (2026-07-03)
- `output_mode` (content/files_with_matches/count), `-A`/`-B`/`-C`, `-i`, `-n`, `-o`, `type`,
  `multiline`, `head_limit`, `path`, `no_ignore` eklendi (yeni `builtin_grep.go`). ripgrep binary'ye
  bağlanmadan saf-Go; `.gitignore` farkındalığı için bkz. aşağı.

### `Glob` — ✅ TAMAMLANDI (2026-07-03)
- **mtime sıralaması** (en yeni önce) + `path` (arama kökü) + `no_ignore` argümanları eklendi
  (yeni `builtin_glob.go`).

### `Bash` / `PowerShell` — ✅ TAMAMLANDI (2026-07-03, paralel çalışma)
- **`run_in_background`** eklendi (`builtin_shell_bg.go` `ShellManager` + `shell_output`/
  `shell_kill`/`shell_list`, session-scoped `Runtime.shellMgrs`). Uzun süren komut detached
  başlar, id döner; çıktı pollanır, durdurulur.

### `Glob`/`Grep` ortak — ✅ `.gitignore` farkındalığı eklendi (yeni `ignore.go` `IgnoreSet`):
kök+iç-içe `.gitignore` (lazy) + daima `.git`; dizin eşleşince `SkipDir`. `no_ignore` ile kapatılır.

---
