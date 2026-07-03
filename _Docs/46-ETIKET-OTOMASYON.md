# 46 — Etiketler + Etiket-Tetikleyicili Otomasyonlar

> Durum: **Uygulandı + canlı doğrulandı** (2026-07-02/03, WS2, sonnet/claude-cli).

Üç özellik: (1) oturum/flow/schedule kayıtlarına **etiket** (tag), (2) belirli bir
etikete sahip oturum bir turu bitirince o oturumun sonucunu alıp yeni bir oturum
başlatan **etiket-tetikleyicili otomasyonlar** — kendiliğinden süren döngüler,
(3) tur olaylarına göre **otomatik etiketleme** (§3).

## 1. Etiketler

`Session.Tags` / `Flow.Tags` / `Schedule.Tags` (hepsi `[]string`, omitempty;
`Task.Tags` zaten vardı). Serbest metin etiketler; hem kullanıcı (UI) hem ajan
(araçlar) düzenler. Session etiketleri ayrıca otomasyonları tetikler.

- **Store:** `SetSessionTags` / `SetFlowTags` / `SetScheduleTags`. Ortak
  `normalizeTags` (trim + boşları at + first-seen sıralı dedup, hepsi boşsa nil).
- **API:** `PUT /api/sessions/{id}/tags`, `PUT /api/flows/{id}/tags`,
  `PUT /api/schedules/{id}/tags` — body `{"tags":[...]}` (tam değiştirme). Session
  tag değişimi canlı UI refresh için `session` event'i yayınlar. `SessionInfo`
  yanıtına `tags` eklendi.
- **Araçlar:** `set_session_tags` (mevcut oturum, `SessionSink` üzerinden;
  `tags` tam-değiştir **veya** `add`/`remove` artımlı), `set_flow_tags`,
  `set_schedule_tags` (id ile, db üzerinden — tag'leme yıkıcı değil, provenance yok).
- **UI:** `common/TagEditor.tsx` — chip editörü (Enter/virgül ekler, × kaldırır,
  boş kutuda Backspace son etiketi siler). Bağlı: SessionDetailPanel (Hedef altında
  "Etiketler"), FlowsPanel (meta toolbar; `setFlowTags` ile anında kalıcı), Schedules
  (satır içi).

## 2. Otomasyonlar (`Automation`)

Ayrı entity (`db/models_automation.go`, id prefix `AUT`, dir `automations`). Cron
Schedule'dan **ayrı** tutuldu (Schedule zaten cron + one-shot wake ile yüklü) ama
UI'da aynı Schedules ekranında ayrı bölümde gösterilir.

### Alanlar
| Alan | Anlam |
|------|-------|
| `TriggerTag` | İzlenen session etiketi. Bu etiketi taşıyan oturum bir turu bitirince tetiklenir. |
| `TargetAgentID` | Spawn'lanan oturumu çalıştıracak ajan. |
| `PromptTemplate` | Yeni oturumun promptu. Placeholder'lar aşağıda. |
| `SpawnTags` | Spawn'lanan oturuma uygulanan etiketler. `nil` → `[TriggerTag]` (döngü). `[]` → döngüyü kırar. |
| `Enabled` | Kill-switch. Aç→iterasyon sayacı sıfırlanır. |
| `MaxIterations` | Toplam tetik üst sınırı (0=sınırsız — dikkat). Vars. 50. Aşılınca otomatik pasifle. |
| `CooldownSec` | İki tetik arası min. saniye. |
| `IterationCount` / `LastFiredAt` / `LastSessionID` / `LastError` | Çalışma-zamanı defteri (`RecordAutomationFire`). |

### PromptTemplate değişkenleri
`renderAutomationPrompt` + `AutomationEngine.turnVars` (`agent/automation.go`) şu
`{{...}}` placeholder'larını ikame eder (biten tur başına):

| Değişken | Değer |
|----------|-------|
| `{{result}}` | Biten oturumun son yanıtı (asistan metni) |
| `{{title}}` | Biten oturumun başlığı |
| `{{tag}}` | Tetikleyici etiket |
| `{{sessionId}}` | Biten oturumun ID'si |
| `{{iteration}}` | Bu ateşlemenin sıra no'su (1-tabanlı, `IterationCount+1`) |
| `{{maxIterations}}` | Üst sınır (`0` → `∞`) |
| `{{agent}}` / `{{agentName}}` | Sonucu üreten ajanın adı |
| `{{prevPrompt}}` | Biten oturumun son **user** mesajı (sonucu üreten girdi) |
| `{{automation}}` | Otomasyonun adı |
| `{{date}}` / `{{time}}` / `{{datetime}}` | `2006-01-02` / `15:04` / `2006-01-02 15:04` |

Şablonda hiç `{{result}}` yoksa, sonuç yine de "--- Önceki sonuç ---" başlığıyla
sona eklenir (döngü çıktıyı taşımadan kalmasın). Bilinmeyen `{{...}}` aynen kalır.
Yeni değişken eklemek = `turnVars` haritasına bir satır. **Canlı doğrulandı**
(2026-07-03, WS2): `{{iteration}}/{{agent}}/{{date}}/{{prevPrompt}}/{{result}}` doğru
render oldu (sayaç 10→11→12, AUT4 2/2'de otomatik durdu).

### Tetikleyici: tur bitişi
Karar: tetik = **etiketli oturumda bir tur `end_turn` ile bitince** (araçlar
kullanılıp son cevap verilince). Bu, `Runtime.FireTurnFinished(sessionID, agentID,
output)` ile sinyallenir — **detached goroutine**, turu asla bloklamaz/iptal etmez.

Çağrı yerleri (tüm tamamlanma yolları):
- `api/chat_stream.go` — her yanıt sonrası (`resp.Text`).
- `agent/spawn.go` `runSpawn` — spawn turu bitince (döngünün doğal adımı).
- `agent/scheduler.go` `deliverPrompt` (zamanlanmış) + `deliverWake` (uyandırma).

### Motor (`agent/automation.go`)
`AutomationEngine.OnTurnFinished`:
1. Biten oturumu yükle; **etiketsizse hızlı dön** (etiketsiz sohbet trafiği neredeyse
   bedava).
2. Enabled otomasyonlardan `TriggerTag ∈ session.Tags` olanlar için `fire`:
   - **Cooldown:** `now - LastFiredAt < CooldownSec` → atla (logla).
   - **Maks. iterasyon:** `IterationCount >= MaxIterations` (>0) → **otomatik pasifle**
     + `automation` event, dur.
   - `renderAutomationPrompt` (placeholder ikamesi; `{{result}}` yoksa sonucu
     "--- Önceki sonuç ---" ile ekler).
   - `SpawnSession(TargetAgentID, prompt, {Tags: spawnTags, ParentSessionID: biten,
     CreatedBy: "automation:"+id})`. **`SpawnOptions.Tags`** eklendi → spawn'lanan
     oturum **oluşturulurken** etiketlenir (arka plan turu tag konmadan bitse bile
     race yok).
   - `RecordAutomationFire` (sayaç++, spawned session id, hata).

Döngü: A(#loop) biter → B(#loop) spawn → B biter → C spawn … MaxIterations'a kadar.
Sayaç tek otomasyon üzerinde birikir (tüm spawn'lar aynı etiketi → aynı kural).

### API + Araçlar + UI
- **API:** `GET/POST /api/automations`, `PUT /api/automations/{id}`,
  `POST /api/automations/{id}/toggle`, `POST /api/automations/{id}/reset`,
  `DELETE /api/automations/{id}`. Create'te hedef ajan doğrulanır; vars. maks=50.
- **Araçlar:** `create/update/delete/list_automation` (self-management suite,
  provenance: ajan yalnız kendi oluşturduğunu düzenler/siler).
- **UI:** `panels/Automations.tsx` — Schedules ekranında "Otomasyonlar" bölümü:
  oluşturma formu + liste (aç-kapa, iterasyon sayacı `n/max`, spawn-etiket editörü,
  limit dolunca "sıfırla", sil). Prompt şablonu alanında **ℹ️ info butonu** →
  13 değişkeni açıklamalı listeleyen popover (satıra tıkla → şablona ekle;
  click-away ile kapanır; `PROMPT_VARS` sabiti `turnVars` ile senkron).
  Etiket-tetikleyici event'leri `automation` tipiyle
  yayınlanır (deep-link: başarı→executions, limit/hata→schedules).

## 3. Otomatik Etiketleme (olay → etiket)

Tur olaylarına ve oturum durumuna göre well-known etiketler otomatik atanır
(`agent/autotag.go`), böylece bir otomasyon (veya kullanıcı) bunları tarayıp
işleyebilir — hedef akış: "tool-error"/"error" etiketli oturumları bulup onaran
tag-tetikleyicili otomasyon. **ADD-only**: etiket, bir onarıcı `set_session_tags`/
API ile silene kadar kalır (onarım sinyali budur).

Atanan etiketler (`agent/autotag.go` sabitleri):
| Etiket | Ne zaman |
|--------|----------|
| `tool-error` | Turda **gerçek** bir tool hatası (`StepTool.IsError`) |
| `error` | Tur-seviyesi hata (provider/aksiyon hatası; kullanıcı "stopped" hariç) |
| `goal` | Oturumda kalıcı hedef var |
| `goal-done` | Hedef tamamlandı |
| `archived` | Oturum arşivlendi |

**"disallowed tool" istisnası:** claude-cli izin verilmeyen bir tool'u denerse
`is_error` sonucu döner ama bu bir **politika reddi**, tool hatası değil — `tool-error`
atanmaz. Native yolda red zaten `StepError{Reason:"permission_denied"}` (StepTool
değil) olduğu için sayılmaz; claude-cli yolunda `isPermissionDenyError` metin
işaretleriyle (`permission_denied`, "requested permissions", "haven't granted",
"not allowed", "disallowed" …) hariç tutar (`permissionDenyMarkers`).

**Tetik noktaları:** `Runtime.AutoTagTurn(ctx, sessionID, steps, turnErr)` — chat
(başarı + cerr hata yolu), spawn (başarı + hata), scheduler `deliverPrompt` +
`deliverWake` (başarı + hata). `archived` ayrıca mutasyon anında: `sessionSink.Archive`
+ API `handleSetSessionState` (arşivde ekle, geri yüklemede sil). Yazım add-only +
değişiklik varsa persist + `session` event (canlı UI refresh). Şu an daima açık
(ayar yok; istenirse Tunables flag'i eklenebilir).

**Canlı doğrulama (2026-07-03, WS2):** archived ekle/sil ✅, goal→`['goal']` ✅,
var-olmayan dosya Read → `is_error` → `tool-error` ✅. Birim testi:
`autotag_test.go` (`isPermissionDenyError` gerçek-hata vs politika-reddi ayrımı).

## Güvenlik / Runaway Freni
- Etiketsiz sohbet asla tetiklenmez (tag-gating).
- Cooldown + MaxIterations + per-otomasyon Enabled + workspace "otonomi duraklat"
  (mevcut) + per-ajan günlük bütçe (`ensureBudget`, spawn autonomous).
- Limit dolunca otomasyon otomatik pasifleşir (sonsuz döngü imkânsız, 0=sınırsız
  hariç — UI'da uyarı title'ı).

## Test
- `internal/db/automation_test.go` — CRUD, sayaç, aç/kapa-sıfırla, reset, reload
  kalıcılığı; `SetSessionTags` normalizasyonu.
- `internal/agent/automation_test.go` — `renderAutomationPrompt` (ikame + append +
  no-op), `containsTag`.

## Sıradaki
- Canlı loop doğrulaması (gerçek sağlayıcıyla uçtan uca; token maliyeti nedeniyle
  unit testlerle ayrıldı).
- Opsiyonel: `swarmgo-autonomous-ops` skill'ine "etiketle döngü kur" reçetesi;
  flow/schedule etiketlerini de tetikleyiciye açma (şimdilik yalnız session).
