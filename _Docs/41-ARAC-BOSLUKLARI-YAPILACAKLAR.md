# 41 — Araç Boşlukları ve Yapılacaklar (the external agent project ↔ SwarmGo)

> **Amaç:** the external agent project (Claude Code tabanlı) araç envanteri ile SwarmGo builtin araçlarının
> karşılaştırmasından çıkan **eksik araçları** ve **mevcut araç iyileştirmelerini** açıklamalarıyla
> birlikte tek bir yapılacaklar listesinde toplamak.
>
> Kaynak analiz: [`analiz-craftagent-arac-eslestirme.md`](./analiz-craftagent-arac-eslestirme.md)
> (envanter eşleştirmesi). Bu doküman onun **aksiyon (backlog) karşılığıdır.**
>
> **Güncel not:** Analiz dosyasında "SwarmGo'da yok" denen `config_validate`, `skill_validate`,
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
| 5 | `get_session_info` | Yeni araç | **P2** | Açık |
| 6 | `set_session_labels` / `set_session_status` | Yeni araç | **P2** | Açık |
| 7 | `update_user_preferences` | Yeni araç | **P2** | Açık (kısmen core_memory) |
| 8 | `render_template` | Yeni araç | **P2** | Açık |
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
    ama claude-cli'ye **bridge'lenmez** — SwarmGo built-in'leri CLI'ye yalnızca elle küratörlenen
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
- **Neden önemli:** SwarmGo'nun **token optimizasyon** felsefesiyle (`_Docs/17`) birebir uyumlu: büyük
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
  kaynak-sınırlama sarmalayıcısı. (Not: the external agent project bunu Explore'da da sunar; SwarmGo dürüst risk modeli
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

### 5. `get_session_info` — tekil oturum metadata — **P2**

- **Durum:** Yok. Yalnızca `list_sessions` var (tüm oturumlar). Tek oturumun etiket/durum/cwd/goal/izin
  modunu dönen ucuz "kendini incele" yolu yok.
- **Neden önemli:** Ajanın kendi oturum bağlamını (özellikle `set_session_labels`/`status` eklenince)
  okuması için `list_sessions`'tan daha ucuz ve nokta-atışı. Otomasyon/koşullu mantık için temel.
- **Yaklaşım:** `sessionId?` (boşsa mevcut oturum) → metadata objesi. `currentsession.go` zaten mevcut
  oturum referansını taşıyor.
- **Dosyalar:** yeni `internal/tools/builtin_sessioninfo.go` (+ test), `registry.go`.
- **Risk sınıfı:** `RiskRead`.

### 6. `set_session_labels` / `set_session_status` — **P2**

- **Durum:** Yok. SwarmGo'da `set_session_goal`/`complete_goal`/`set_session_title`/`archive_session`
  var ama **etiket** ve **durum (todo/in_progress/done)** kavramı oturum seviyesinde yok (benzeri kanban
  task'larında `move_task`).
- **Neden önemli:** (a) Oturumları sınıflandırma/filtreleme (`list_sessions` filtresi güçlenir);
  (b) **Otomasyon tetikleyici** — etiket/durum değişimi hook/schedule tetikleyebilir → kendi-kapanan
  iş akışları (iş biter → status=done → downstream webhook). the external agent project'ta tam bu desen var.
- **Yaklaşım:** `db.Session`'a `Labels []string` + `Status string` alanları (zaten `archive` benzeri
  durum var; genelleştir). İki builtin + `list_sessions` filtre güncellemesi + (varsa) hook event'i.
- **Dosyalar:** `internal/db/models_*` (Session alanları), yeni `builtin_sessionstate.go` (+ test),
  `builtin_sessions.go` filtre, hook entegrasyonu.
- **Risk sınıfı:** `RiskWrite`.

### 7. `update_user_preferences` — yapısal kullanıcı profili — **P2**

- **Durum:** Kısmen `core_memory` "human" bloğu karşılıyor ama **yapısal değil** (timezone/city/country/
  name serbest metinde). the external agent project'ta ayrı, alanları belli bir araç var.
- **Neden önemli:** Yapısal tercihler (ad, saat dilimi, dil, konum, co-author tercihi) tutarlı biçimde
  her ajana/oturuma enjekte edilebilir; serbest-metin core memory'den daha güvenilir okunur.
- **Yaklaşım:** `name?`, `timezone?`, `city?`, `region?`, `country?`, `language?`, `notes?`,
  `includeCoAuthoredBy?`. Workspace/global ayar dosyasına yazar; sistem promptuna yapısal blok olarak
  girer. core_memory "human" ile çakışmayı önlemek için: bu yapısal alan, human bloğu serbest-metin kalır.
- **Dosyalar:** `settings`/`wsconfig` alanı, yeni `builtin_userprefs.go` (+ test), prompt enjeksiyonu.
- **Risk sınıfı:** `RiskWrite`.

### 8. `render_template` — şablonlu çıktı render — **P2**

- **Durum:** Yok. Artifact sistemi var ama kaynak/veriden **Mustache/HTML şablonu** ile tutarlı,
  markalı çıktı (rapor/önizleme/e-posta) üretecek araç yok.
- **Neden önemli:** Ajanların ürettiği raporları/önizlemeleri her seferinde elle HTML yazmak yerine
  şablonla render etmek tutarlılık + token tasarrufu sağlar; artifact olarak saklanır.
- **Yaklaşım:** `source/template/data` → render → artifact. Go tarafında hafif bir template motoru
  (`text/template` veya bir Mustache kütüphanesi — go.mod minimalizmi gözetilerek). Şablonlar workspace
  altında saklanır.
- **Dosyalar:** yeni `internal/tools/builtin_rendertemplate.go` (+ test), şablon depolama konvansiyonu,
  artifact entegrasyonu.
- **Risk sınıfı:** `RiskWrite`.

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
  SwarmGo'nun scheduler'ı zaten var → üstüne ince bir "until-condition" sarmalayıcı.
- **Dosyalar:** yeni `builtin_monitor.go` (+ test), scheduler entegrasyonu.
- **Risk sınıfı:** `RiskRead`.

### 11. `EnterWorktree` / `ExitWorktree` — ajan-kontrollü worktree — **P3**

- **Durum:** SwarmGo'da `gitWorktreeIsolation` **ayarı** var (otonom oturuma ayrı worktree) ama
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

## C. Bilinçli kapsam-dışı (eklenmeyecek)

the external agent project'ta olup SwarmGo'nun **kapsam/felsefe farkı** nedeniyle eklenmeyenler:

- `source_oauth_trigger` ve Google/Slack/Microsoft OAuth varyantları → SwarmGo source modeli MCP-sunucu +
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
2. **Dalga 2 (P2):** `get_session_info`, `set_session_labels`/`set_session_status`,
   `update_user_preferences`, `render_template`.
3. **Dalga 3 (P3–P4):** `Monitor`, `EnterWorktree`/`ExitWorktree`, güvenli credential UI, `NotebookEdit`.

Her araç eklemesinde ortak kontrol listesi:
- [ ] `internal/tools/builtin_*.go` + birim test
- [ ] `registry.go` kaydı + lazy/eager tier kararı (`_Docs/19`)
- [ ] `classify.go` risk sınıfı
- [ ] Self-management/gating gerekiyorsa `_Docs/24` güncellemesi
- [ ] İlgili default skill + `SKILL.md` "Mevcut Yetenekler" güncellemesi
- [ ] `_Docs/05-ILERLEME.md` changelog girdisi
