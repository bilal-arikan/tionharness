# 41 — Araç Boşlukları ve Yapılacaklar (the external agent project ↔ TionHarness)

> **Özet (2026-09-03):** the external agent project (Claude Code tabanlı) ile TionHarness builtin araç
> envanterinin karşılaştırmasından çıkan **backlog** dokümanıdır — kısmen uygulanmış,
> kısmen açık. Dalga 1-2 tamamlandı (`WebSearch`, `transform_data`, `PowerShell`,
> `get_session_info`, `update_user_preferences`, `render_template`); açık kalanlar
> `call_llm` (P1), güvenli credential-giriş UI'ı, `Monitor`, `EnterWorktree`/`ExitWorktree`
> ve `NotebookEdit` (P3-P4). Ayrıca bilinçli kapsam-dışı bırakılanlar (OAuth trigger'lar,
> messaging kanalları, `browser_tool`) ve mevcut araçlara eklenen özellikler (Edit/Write
> tazelik guard'ı, Read offset/limit, Grep/Glob genişletmeleri) listelenir. Dayandığı
> dosyalar: `internal/tools/builtin_*.go`, `internal/tools/classify.go`, `toolsetup.go`.

> **Amaç:** the external agent project (Claude Code tabanlı) araç envanteri ile TionHarness builtin araçlarının
> karşılaştırmasından çıkan **eksik araçları** ve **mevcut araç iyileştirmelerini** açıklamalarıyla
> birlikte tek bir yapılacaklar listesinde toplamak.
>
> Kaynak analiz: [`analiz-harici-ajan-arac-eslestirme.md`](./analiz-harici-ajan-arac-eslestirme.md)
> (envanter eşleştirmesi). Bu doküman onun **aksiyon (backlog) karşılığıdır.**
>
> **Güncel not:** Analiz dosyasında "TionHarness'te yok" denen `config_validate`, `skill_validate`,
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
| 8 | `render_template` | Yeni araç | **P2** | ✅ Tamamlandı (2026-07-06 — _Docs/63) |
| 9 | `source_credential_prompt` benzeri güvenli credential UI | Yeni araç | **P3** | Açık |
| 10 | `Monitor` (koşul bekleme) | Yeni araç | **P3** | Açık |
| 11 | `EnterWorktree` / `ExitWorktree` (ajan-kontrollü) | Yeni araç | **P3** | Açık |
| 12 | `NotebookEdit` (Jupyter) | Yeni araç | **P4** | Açık (niş) |
| I1–I4 | Mevcut araç iyileştirmeleri | İyileştirme | P1–P2 | Açık |
| I5 | `activate_tools` direct-fetch | İyileştirme | P3 | ✅ Tamamlandı |

---

## A. Eklenecek araçlar (açıklamalı)

> **Tamamlanan maddeler** (1 `WebSearch`, 3 `transform_data`, 4 `PowerShell`,
> 5 `get_session_info`, 7 `update_user_preferences`, 8 `render_template`) ve Bölüm E
> (mevcut araçlara eklenen özellikler: Edit/Write tazelik guard'ı, Read, Grep, Glob,
> Bash/PowerShell arka plan, `.gitignore` farkındalığı) gövdeleri →
> [arsiv/41-ARAC-BOSLUKLARI-TAMAMLANANLAR.md](arsiv/41-ARAC-BOSLUKLARI-TAMAMLANANLAR.md).
> `render_template` ayrıntısı: [63-SOURCE-TEMPLATES-RENDER.md](63-SOURCE-TEMPLATES-RENDER.md).

### `WebSearch` — SearXNG kurulumu (operasyonel not)

- **Operatör kurulumu:** `secret_set` ile `TAVILY_API_KEY` *veya* `SEARXNG_URL` ekle.
- **Bu makinedeki kurulum (2026-08-11):** Anahtarsız yol seçildi — yerel SearXNG,
  `<progs>/searxng` (Docker Compose, `searxng/searxng:latest`,
  `127.0.0.1:8484` → container 8080, `restart: unless-stopped`). `settings.yml` içinde
  `search.formats` listesine `json` eklendi (araç `GET /search?format=json` çağırır;
  varsayılan imajda JSON kapalıdır) ve `server.limiter: false` (yerel, tek tüketici).
  Vault kaydı: `SEARXNG_URL = http://127.0.0.1:8484` — **workspace başına izole**, yani
  her workspace'e ayrı eklenir (şu an WS5 ve WS1'de var).
  Yeniden başlatma: `cd <progs>/searxng; docker compose up -d`.
  Docker Desktop kapalıysa arama `connection refused` verir — önce onu başlat.
  **Konteyner `restart: unless-stopped` olsa da elle durdurulduğunda geri gelmez** — 2026-08-16'da
  tam olarak bu oldu ve zamanlanmış bir oturum (WS1/SES286) aramayı hiç yapamadı. Sağlık kontrolü:
  `curl "http://127.0.0.1:8484/search?q=test&format=json"` → HTTP 200 + JSON gövde beklenir.
  Git-Bash'ten `docker run -v` kullanılacaksa `MSYS_NO_PATHCONV=1` şart, yoksa konteyner içi hedef yol
  (`/etc/searxng`) `C:/Program Files/Git/etc/searxng` olarak bozulur ve settings.yml sessizce bağlanmaz.

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

### 6. `set_session_labels` / `set_session_status` — **P2** — ❌ KAPSAM DIŞI (2026-07-03, kullanıcı kararı)

- **Gerekçe:** Etiket tarafını **`set_session_tags`** (2026-07-02, `_Docs/46-ETIKET-OTOMASYON.md`)
  zaten karşılıyor — UI ile paylaşımlı `Session.Tags` + etiket-tetikleyicili otomasyonlar, yani
  the external agent project'ın "label → automation → kendi-kapanan iş akışı" deseninin TionHarness karşılığı kurulu.
  Durum tarafında da oturum `State` + `archive_session` + Kanban task'ları (`move_task`) mevcut
  akışları karşılıyor; ayrı bir `set_session_status` aracı eklenmeyecek (bkz. Bölüm C mantığı).

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
  TionHarness'in scheduler'ı zaten var → üstüne ince bir "until-condition" sarmalayıcı.
- **Dosyalar:** yeni `builtin_monitor.go` (+ test), scheduler entegrasyonu.
- **Risk sınıfı:** `RiskRead`.

### 11. `EnterWorktree` / `ExitWorktree` — ajan-kontrollü worktree — **P3**

- **Durum:** Otonom oturuma per-session worktree veren `gitWorktreeIsolation` ayarı 2026-07-28'de
  **kaldırıldı** (ileride kapsamlı yeniden ekleme planlı). Ajanın **açıkça** worktree'ye girip
  çıkabileceği bir araç da yok. Bu iki iş birlikte, kapsamlı bir worktree katmanı olarak tasarlanacak.
- **Neden önemli:** Paralel/izole değişiklik (riskli refactor, paralel ajan çakışması) için ajanın
  kendi inisiyatifiyle izole worktree açıp kapatması.
- **Yaklaşım:** `git worktree add/remove` sarmalayan iki builtin; yeniden eklenecek worktree
  izolasyon mantığı + mevcut `autonomousConfine` ile uyumlu, çalışma dizinini geçici olarak yönlendirir.
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

### I5. `tool_search` / `activate_tools` — direct-fetch kısayolu — **P3** — ✅ TAMAMLANDI

- `activate_tools` araçları doğrudan isimle yükler (`internal/tools/builtin_activate.go`);
  `select:<name>,<name>` sözdizimi `internal/tools/loader_guidance.go` içinde tanınıp
  yönlendirilir.

---

## C. Bilinçli kapsam-dışı (eklenmeyecek)

the external agent project'ta olup TionHarness'in **kapsam/felsefe farkı** nedeniyle eklenmeyenler:

- `source_oauth_trigger` ve Google/Slack/Microsoft OAuth varyantları → TionHarness source modeli MCP-sunucu +
  secret-vault tabanlı; OAuth akışı kapsam dışı.
- `list_messaging_channels` / `unbind_messaging_channel` → Telegram/WhatsApp gateway entegrasyonu yok
  (Connectors fazı 2026-06-16'da kapsamdan çıkarıldı).
- `send_developer_feedback` → harici geri bildirim kanalı yok.
- `browser_tool` → natif tarayıcı aracı yerine playwright vb. tarayıcı MCP sunucuları kullanılır.
- `SubmitPlan` → claude-cli plan modu (`ExitPlanMode` köprüsü, `_Docs/40-PLAN-MODE.md`) zaten karşılıyor.

---

## D. Önerilen uygulama dalgaları

1. **Dalga 1 (P0–P1):** `WebSearch`, `call_llm` (veya `run_subagent` hafif mod), `transform_data`/
   `script_sandbox`, `PowerShell`. En yüksek getiri/çaba.
2. **Dalga 2 (P2):** ~~`get_session_info`~~ ✅, ~~`set_session_labels`/`set_session_status`~~ ❌ kapsam
   dışı, ~~`update_user_preferences`~~ ✅ (üçü 2026-07-03'te kapandı), ~~`render_template`~~ ✅
   (2026-07-06, _Docs/63) — **Dalga 2 tamamlandı.**
3. **Dalga 3 (P3–P4):** `Monitor`, `EnterWorktree`/`ExitWorktree`, güvenli credential UI, `NotebookEdit`.

Her araç eklemesinde ortak kontrol listesi:
- [ ] `internal/tools/builtin_*.go` + birim test
- [ ] `registry.go` kaydı + lazy/eager tier kararı (`_Docs/19`)
- [ ] `classify.go` risk sınıfı
- [ ] Self-management/gating gerekiyorsa `_Docs/24` güncellemesi
- [ ] İlgili default skill + `SKILL.md` "Mevcut Yetenekler" güncellemesi
- [ ] `_Docs/05-ILERLEME.md` changelog girdisi
