# 41 — Araç Boşlukları ve Yapılacaklar (the external agent project ↔ TionSwarm)

> **Amaç:** the external agent project (Claude Code tabanlı) araç envanteri ile TionSwarm builtin araçlarının
> karşılaştırmasından çıkan **eksik araçları** ve **mevcut araç iyileştirmelerini** açıklamalarıyla
> birlikte tek bir yapılacaklar listesinde toplamak.
>
> Kaynak analiz: [`analiz-craftagent-arac-eslestirme.md`](./analiz-craftagent-arac-eslestirme.md)
> (envanter eşleştirmesi). Bu doküman onun **aksiyon (backlog) karşılığıdır.**
>
> **Güncel not:** Analiz dosyasında "TionSwarm'da yok" denen `config_validate`, `skill_validate`,
> `mermaid_validate` araçları **bu tarihten sonra eklenmiştir** (`builtin_configvalidate.go`,
> `builtin_skillvalidate.go`, `builtin_mermaidvalidate.go`) → o boşluklar **KAPANDI**, aşağıda yer
> almazlar. Liste yalnızca **hâlâ açık** olan boşlukları içerir.

---

## Öncelik özeti

| # | Araç / İş | Tür | Öncelik | Durum |
|---|-----------|-----|---------|-------|
| 1 | `WebSearch` | Yeni araç | **P0** | ✅ Tamamlandı (2026-06-30) |
| 2 | `call_llm` (hafif LLM alt-görevi) | Yeni araç | **P1** | Açık |
| 3 | `transform_data` | Yeni araç | **P1** | ✅ Tamamlandı (2026-06-30) |
| 4 | `PowerShell` (ayrı shell) | Yeni araç | **P1** | ✅ Tamamlandı (2026-06-30) |
| 5 | `get_session_info` | Yeni araç | **P2** | ✅ Tamamlandı (2026-07-03) |
| 6 | `set_session_labels` / `set_session_status` | Yeni araç | **P2** | ❌ Kapsam dışı (2026-07-03 — `set_session_tags` + `archive_session`/Kanban karşılıyor) |
| 7 | `update_user_preferences` | Yeni araç | **P2** | ✅ Tamamlandı (2026-07-03) |
| 8 | `render_template` | Yeni araç | **P2** | ✅ Tamamlandı (2026-07-06 — _Docs/53) |
| 9 | `source_credential_prompt` benzeri güvenli credential UI | Yeni araç | **P3** | Açık |
| 10 | `Monitor` (koşul bekleme) | Yeni araç | **P3** | Açık |
| 11 | `EnterWorktree` / `ExitWorktree` (ajan-kontrollü) | Yeni araç | **P3** | Açık |
| 12 | `NotebookEdit` (Jupyter) | Yeni araç | **P4** | Açık (niş) |
| I1–I5 | Mevcut araç iyileştirmeleri | İyileştirme | P1–P3 | Açık |

---

## A. Eklenecek araçlar (açıklamalı)

### 1. `WebSearch` — **P0** — ✅ TAMAMLANDI (2026-06-30)

- **Durum:** ~~Ölü placeholder.~~ **Uygulandı.** Native Go `WebSearch` aracı `WebFetch` deseninde eklendi.
- **Neden önemli:** Ajan güncel bilgiye (dokümantasyon sürümü, hata mesajı, kütüphane API'si, güncel
  olaylar) erişemiyor. Model bilgi-kesim tarihiyle sınırlı kalıyor. Tek net **işlevsel** boşluk.
- **Uygulanan yaklaşım:**
  - **Provider-bağımsız**, vault'tan otomatik backend seçimi: `SEARXNG_URL` (self-host, anahtarsız) varsa
    o, yoksa `TAVILY_API_KEY` (Tavily, 1k ücretsiz/ay). İkisi de yoksa **sessizce yutmaz** — açık hata.
  - **claude-cli hariç:** Araç `WebFetch` gibi **koşulsuz** kaydedilir (workspace tools ekranında görünür),
    ama claude-cli'ye **bridge'lenmez** — TionSwarm built-in'leri CLI'ye yalnızca elle küratörlenen
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

### 2. `call_llm` — hafif ikincil LLM alt-görevi — **P1**

- **Durum:** Yok. En yakın karşılık `run_subagent` ama o **tam bir ajan** (araçlı, multi-turn, ağır).
  Ucuz tek-atışlık (özet/sınıflandırma/yapısal çıkarım) için doğru maliyet katmanı yok.
- **Neden önemli:** (a) **Maliyet** — basit işlere (özetleme, etiketleme) Haiku gibi ucuz modeli tek
  completion ile koşar; (b) **Paralellik** — N dosyayı aynı anda işler; (c) **Bağlam izolasyonu** —
  büyük dosyayı ana bağlamı şişirmeden işler; (d) **Yapısal çıktı** — `outputSchema` ile garantili JSON.
- **Yaklaşım:** Araçsız, tek-completion bir builtin. Parametreler: `prompt`, `attachments?` (dosya
  yolları → tool içerik yükler), `model?`, `systemPrompt?`, `maxTokens?`, `temperature?`, `thinking?`,
  `outputFormat?` (text|json), `outputSchema?`. `providers` katmanını doğrudan kullanır; tool-loop'a
  girmez. Alternatif: `run_subagent`'a "no-tools + single-shot + outputSchema" modu olarak eklemek
  (daha az yeni yüzey) — kararı uygulama anında ver.
- **Dosyalar:** yeni `internal/tools/builtin_calllm.go` (+ `_test.go`), `registry.go`. Bütçe entegrasyonu:
  otonom turda `guardedComplete`/`ensureBudget` ile sayaca dahil edilmeli.
- **Risk sınıfı:** `RiskRead`.

### 3. `transform_data` — izole script — **P1** — ✅ TAMAMLANDI (2026-06-30)

- **Durum:** ~~Yok.~~ **Uygulandı** (`transform_data` adıyla). `script_sandbox` ayrı bir araç olarak
  uygulanmadı — `transform_data` zaten striplenmiş-env + timeout + yapısal çıktı sözleşmesini karşılıyor.
- **Neden önemli:** TionSwarm'nun **token optimizasyon** felsefesiyle (`_Docs/17`) birebir uyumlu: büyük
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
  kaynak-sınırlama sarmalayıcısı. (Not: the external agent project bunu Explore'da da sunar; TionSwarm dürüst risk modeli
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

### 6. `set_session_labels` / `set_session_status` — **P2** — ❌ KAPSAM DIŞI (2026-07-03, kullanıcı kararı)

- **Gerekçe:** Etiket tarafını **`set_session_tags`** (2026-07-02, `_Docs/46-ETIKET-OTOMASYON.md`)
  zaten karşılıyor — UI ile paylaşımlı `Session.Tags` + etiket-tetikleyicili otomasyonlar, yani
  the external agent project'ın "label → automation → kendi-kapanan iş akışı" deseninin TionSwarm karşılığı kurulu.
  Durum tarafında da oturum `State` + `archive_session` + Kanban task'ları (`move_task`) mevcut
  akışları karşılıyor; ayrı bir `set_session_status` aracı eklenmeyecek (bkz. Bölüm C mantığı).

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
  - Depolama: **skill-bundled** (`tionswarm-templates` skill; `${SKILL_DIR}/templates/*.html`), yeni
    "Sources/template store" alt-sistemi kurulmadı.
- **Dosyalar:** `internal/tools/{render_template.go, builtin_render_template.go, *_test.go}`,
  `internal/agent/renderdir.go` + `toolsetup.go`, `internal/api/files.go`,
  `frontend/src/components/markdown/{HtmlPreview.tsx, CodeBlock.tsx}`, `frontend/src/lib/attachments.tsx`,
  `internal/skills/defaults/tionswarm-templates/**`, prompt + guard güncellendi.
- **Risk sınıfı:** `RiskWrite` (kontrollü: yalnız render dir'e yazar, çıktı adı basename'e indirgenir).

### 9. `source_credential_prompt` benzeri güvenli credential giriş UI — **P3**

- **Durum:** Kısmen `secret_set` (+ `ask_user`) karşılıyor ama ajan secret'ı **kendisi yazar**; güvenli,
  maskeli credential giriş UI'ı yok.
- **Neden önemli:** Kullanıcının hassas anahtarları ajan-transkriptine düşürmeden, maskeli bir alana
  girmesi güvenlik açısından daha doğru.
- **Yaklaşım:** `interaction` MCP üzerinden maskeli prompt kartı (`ask_user`'ın `secret=true` varyantı)
  → doğrudan `secret_set`'e yazar, transkriptte değer görünmez.
- **Dosyalar:** `builtin_ask.go`/`interaction.go` genişletmesi, UI prompt kartı.
- **Risk sınıfı:** `RiskWrite`.

### 10. `Monitor` — koşul bekleme — **P3**

- **Durum:** Yok. `schedule_wake` kısmen (zamanlı uyanış) ama bir **koşul sağlanana kadar** bloklayan
  bekleme yok.
- **Neden önemli:** Dış süreç/CI/uzak kuyruk gibi harness'in bildiremeyeceği durumları beklemek için.
- **Yaklaşım:** Periyodik kontrol + timeout'lu bir bekleme aracı; otonom turlarda bütçe-dostu aralık.
  TionSwarm'nun scheduler'ı zaten var → üstüne ince bir "until-condition" sarmalayıcı.
- **Dosyalar:** yeni `builtin_monitor.go` (+ test), scheduler entegrasyonu.
- **Risk sınıfı:** `RiskRead`.

### 11. `EnterWorktree` / `ExitWorktree` — ajan-kontrollü worktree — **P3**

- **Durum:** TionSwarm'da `gitWorktreeIsolation` **ayarı** var (otonom oturuma ayrı worktree) ama
  ajanın **açıkça** worktree'ye girip çıkabileceği bir araç yok.
- **Neden önemli:** Paralel/izole değişiklik (riskli refactor, paralel ajan çakışması) için ajanın
  kendi inisiyatifiyle izole worktree açıp kapatması.
- **Yaklaşım:** `git worktree add/remove` sarmalayan iki builtin; mevcut `autonomousConfine`/
  `gitWorktreeIsolation` mantığıyla uyumlu, çalışma dizinini geçici olarak yönlendirir.
- **Dosyalar:** yeni `builtin_worktree.go` (+ test), `agent/wsconfig` veya cwd yönetimi entegrasyonu.
- **Risk sınıfı:** `RiskExec`.

### 12. `NotebookEdit` — Jupyter — **P4 (niş)**

- **Durum:** Yok.
- **Neden (düşük):** Kullanıcının ana projesi Go/TS; Jupyter ihtiyacı sınırlı. Sadece veri-bilimi iş
  akışları gündeme gelirse değerlenir.
- **Yaklaşım:** `.ipynb` hücre-bazlı edit aracı (gerektiğinde).
- **Risk sınıfı:** `RiskWrite`.

---

## B. Mevcut araçlarda iyileştirmeler (açıklamalı)

### I1. `WebFetch` ↔ `WebSearch` zinciri — **P1**

- **Şu an:** `WebFetch` tek başına; arama yok. `htmltomarkdown.go` zaten mevcut.
- **İyileştirme:** `WebSearch` (madde 1) eklenince "ara → en iyi sonucu getir → markdown" akışını
  pürüzsüzleştir; `WebFetch` çıktısında token-tasarruflu özetleme opsiyonu.

### I2. `run_subagent` — hafif/yapısal mod — **P1**

- **Şu an:** Tam ajan (araçlı, multi-turn). `output_format`/`objective`/`boundaries` sözleşmesi var.
- **İyileştirme:** "no-tools + single-shot + `outputSchema`" hafif modu ekle. Bu, `call_llm` (madde 2)
  boşluğunu burada da kapatabilir → yeni araç yüzeyi yerine mevcut aracın bir modu (karar: madde 2).

### I3. Araç çıktısı → dosya deseni (`transform_data` bütünleşik) — **P1**

- **Şu an:** Büyük `Bash`/`Read` çıktıları doğrudan bağlama düşer.
- **İyileştirme:** `transform_data` (madde 3) ile "çıktıyı dosyaya yaz + yalnız özet/yol döndür" desenini
  standardize et; cached-prefix ve token yükünü düşürür (`_Docs/17` felsefesi).

### I4. `notify` — tür/öncelik/kanal alanları — **P2**

- **Şu an:** Masaüstü bildirimi (tür-bazlı toggle altyapısı mevcut).
- **İyileştirme:** the external agent project `PushNotification` benzeri öncelik/kanal alanları; mevcut bildirim
  altyapısına ince ekleme.

### I5. `tool_search` / `activate_tools` — direct-fetch kısayolu — **P3**

- **Şu an:** Lazy yükleme arama tabanlı.
- **İyileştirme:** the external agent project `ToolSearch`'ün `select:<name>,<name>` direct-fetch sözdizimine paralel
  bir kısayol; bilinen araçları aramadan, isimle yükleyip deferred şema yüklemeyi hızlandırır.

---

## E. Mevcut araçların EKSİK ÖZELLİKLERİ (yeni araç değil, per-tool feature farkı)

> Bölüm A "eksik araçları" listeler; bu bölüm **TionSwarm'da VAR OLAN** araçların
> Claude Code muadilinde bulunup bizde olmayan **özelliklerini** toplar. (İlk kayıt:
> 2026-07-03, `observed-behavior` `src/tools/*` incelemesinden.)

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

## C. Bilinçli kapsam-dışı (eklenmeyecek)

the external agent project'ta olup TionSwarm'nun **kapsam/felsefe farkı** nedeniyle eklenmeyenler:

- `source_oauth_trigger` ve Google/Slack/Microsoft OAuth varyantları → TionSwarm source modeli MCP-sunucu +
  secret-vault tabanlı; OAuth akışı kapsam dışı.
- `list_messaging_channels` / `unbind_messaging_channel` → Telegram/WhatsApp gateway entegrasyonu yok
  (Connectors fazı 2026-06-16'da kapsamdan çıkarıldı).
- `send_developer_feedback` → harici geri bildirim kanalı yok.
- `browser_tool` → natif tarayıcı aracı yerine playwright / mcp-chrome MCP sunucuları kullanılır.
- `SubmitPlan` → claude-cli plan modu (`ExitPlanMode` köprüsü, `_Docs/40-PLAN-MODE.md`) zaten karşılıyor.

---

## D. Önerilen uygulama dalgaları

1. **Dalga 1 (P0–P1):** `WebSearch`, `call_llm` (veya `run_subagent` hafif mod), `transform_data`/
   `script_sandbox`, `PowerShell`. En yüksek getiri/çaba.
2. **Dalga 2 (P2):** ~~`get_session_info`~~ ✅, ~~`set_session_labels`/`set_session_status`~~ ❌ kapsam
   dışı, ~~`update_user_preferences`~~ ✅ (üçü 2026-07-03'te kapandı), ~~`render_template`~~ ✅
   (2026-07-06, _Docs/53) — **Dalga 2 tamamlandı.**
3. **Dalga 3 (P3–P4):** `Monitor`, `EnterWorktree`/`ExitWorktree`, güvenli credential UI, `NotebookEdit`.

Her araç eklemesinde ortak kontrol listesi:
- [ ] `internal/tools/builtin_*.go` + birim test
- [ ] `registry.go` kaydı + lazy/eager tier kararı (`_Docs/19`)
- [ ] `classify.go` risk sınıfı
- [ ] Self-management/gating gerekiyorsa `_Docs/24` güncellemesi
- [ ] İlgili default skill + `SKILL.md` "Mevcut Yetenekler" güncellemesi
- [ ] `_Docs/05-ILERLEME.md` changelog girdisi
