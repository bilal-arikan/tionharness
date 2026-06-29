# 39 — Plan Modu (ExitPlanMode köprüsü)

> Durum: **Tamamlandı (2026-06-28).** claude-cli'nin yerleşik plan modunu SwarmGo'nun
> izin/onay katmanına bağlar. Ayrıca eski ölü `planningMode` alanı tamamen kaldırıldı.

## Sorun

İki ayrı "plan" kavramı karıştırılıyordu:

1. **`planningMode` ajan alanı** ("Standart"/"Derin") — veri modeli/API/market/UI'da
   duruyordu ama `internal/agent/` runtime'ında **hiç okunmuyordu** (ölü alan). "Derin"
   seçmek ile "Standart" arasında pratik fark yoktu.
2. **claude-cli'nin yerleşik `EnterPlanMode`/`ExitPlanMode` araçları** — model plan
   moduna girip planını sunduğunda, headless `-p` modunda `ExitPlanMode` onayı
   alınamıyordu: araç sonucu `"Exit plan mode?"` + `is_error` dönüyor, ajan bunu
   "plan reddedildi" sanıp duruyordu.

## Çözüm

### 1. Ölü `planningMode` alanı kaldırıldı (uçtan uca)
- Go: `db/models.go`, `db/store.go` (default + patch), `api/agents.go` (create/update DTO),
  `api/market.go`, `api/templates.go`, `market/pack.go` (iki struct).
- Frontend: `types/agent.ts`, `types/market.ts`, `agents/agentOptions.ts` (`PLANNING_OPTIONS`
  silindi), `agents/AgentSettingsForm.tsx` ("Planlama modu" picker'ı kaldırıldı).
- Migration gerekmez: diskteki eski ajan JSON'larında alan kalsa bile struct'tan
  kalktığı için unmarshal sessizce yok sayar (geriye uyumlu).

### 2. `ExitPlanMode` plan-onay köprüsü

claude-cli'nin native `ExitPlanMode` çağrısı `--permission-prompt-tool` üzerinden
SwarmGo'nun Interaction MCP `permission_prompt` handler'ına düşer. Orada özel ele
alınır (`api/mcp_interaction.go callExitPlan`):

- Plan markdown'ı `input.plan`'den çıkarılır.
- Canlı turda → `StepPlan` adımı yayılır → UI'da **plan onay kartı** (`PlanPrompt.tsx`,
  markdown render + "Planı onayla" / "Reddet").
- Kullanıcı kararı `run.answer` kanalından gelir (ask_user/permission ile aynı kanal):
  - **Onayla** → `{behavior:"allow"}` → CLI plan modundan çıkar ve devam eder.
  - **Reddet** → `{behavior:"deny", message:"...kullanıcı planı reddetti..."}` → model planı revize eder.
- **Otonom tur** (scheduler/spawn/flow) → canlı kullanıcı yok → plan **otomatik onaylanır**.

### 3. Onaylanan plan → artifact (birikme kontrollü)

Plan **onaylandığında** otomatik olarak bir artifact'a yazılır (`capturePlanArtifact`
→ `db.AppendPlanArtifact`). Hem manuel onayda hem otonom oto-onayda çalışır;
best-effort (yakalama hatası planın ilerlemesini engellemez).

**Birikme çözümü — session başına TEK rolling artifact:**
- Bir session'da kaç plan onaylanırsa onaylansın **tek** artifact tutulur
  (`origin="plan"`, başlık "📋 Onaylanan Planlar").
- Her yeni onaylanan plan, bu artifact'a `## Plan N — <tarih>` bölümü olarak **eklenir**
  (yeni artifact açılmaz). Böylece artifact sayısı plan sayısıyla değil, **session
  sayısıyla** sınırlı kalır.
- Artifact'lar session-kapsamlıdır: session silinince plan artifact'ı da gider (doğal budama).
- `origin="plan"` ayrımı sayesinde UI bu artifact'ları gerçek teslimat artifact'larından
  ayırabilir/filtreleyebilir (ileride toplu budama da kolaylaşır).
- **UI "📋 Plan" chip'i (2026-06-29):** `origin="plan"` artifact'lar Artifactlar
  ekranında (liste + detay) ve hızlı-önizleme modalında accent-renkli "📋 Plan"
  rozetiyle gösterilir; liste filtre çubuğuna **Plan** facet'i eklendi
  (`OriginBadge.ORIGIN_META.plan`, `frontend/src/components/panels/artifactMeta.tsx`).
- **Hızlı önizleme modalı (2026-06-29):** sohbet/aktivite içindeki bir artifact
  chip'ine/kartına tıklamak artık Artifactlar ekranına gitmeden ortada bir
  önizleme modalı (`ArtifactPreviewModal`) açar; modal `getArtifact` ile içeriği
  çekip `ArtifactView` ile render eder, "Ekranda aç" kısayolu tam ekrana geçirir.
  `App.openArtifact` artık modalı açar (`previewArtifactId`); tam ekran navigasyonu
  `openArtifactFull`'a taşındı.
- Not: artifact'lar sürümlenmez (`UpdateArtifactContent` yerinde üzerine yazar); bu yüzden
  append çekirdeği içeriği elle birleştirip `ContentFile`'ı sıfırlayarak içerik dosyasını
  yeniden yazar.

> İleride istenirse: app-geneli `PlanArtifactCapture` ayarı (varsayılan açık) ile tamamen
> kapatma, veya global bir "en fazla N plan artifact" budama eklenebilir. Şimdilik
> session-başına-tek-artifact bounding yeterli görüldü.

### Mod davranışı

| Mod | CLI bayrağı | ExitPlanMode |
|-----|-------------|--------------|
| **ask** | `--permission-prompt-tool` (varsayılan CLI modu) | Köprüden geçer → plan onay kartı |
| **read-only** | `--permission-prompt-tool` **+** `--permission-mode plan` | CLI tüm yazmaları bloklar; yalnız ExitPlanMode köprüye düşer → plan onay kartı |
| **auto** | `--dangerously-skip-permissions` (bypass) | `EnterPlanMode`/`ExitPlanMode` **disallow** edilir (onaylayıcı yok; ajan doğrudan yürütür) |

## İlgili dosyalar

- `internal/agent/climcp.go` — `promptToolForMode` (ask+read-only), `writeCLIMCPConfig`
  (mode parametresi; auto'da plan araçları disallow).
- `internal/providers/claudecli.go` — read-only'de `--permission-prompt-tool` ile birlikte
  `--permission-mode plan`.
- `internal/api/mcp_interaction.go` — `callPermission` içinde `ExitPlanMode` özel dalı +
  `callExitPlan`.
- `internal/tools/permission.go` — `PlanOptions` / `PlanApproved`.
- `internal/db/store_artifact.go` — `AppendPlanArtifact` (session-başına rolling plan artifact, `origin="plan"`).
- `internal/api/artifacts.go` — `artifactSink.AppendPlanArtifact` (duck-typed yetenek).
- `internal/api/mcp_interaction.go` — `capturePlanArtifact` (onayda best-effort yakalama).
- `internal/tools/classify.go` — `EnterPlanMode` = RiskRead (izin kapısına düşerse oto-onay).
- `internal/agent/trace.go` — `StepPlan` adım türü (transient, live-only).
- `frontend/src/components/panels/artifactMeta.tsx` — `ORIGIN_META.plan` ("📋 Plan" chip).
- `frontend/src/components/artifacts/ArtifactPreviewModal.tsx` — hızlı önizleme modalı.
- `frontend/src/App.tsx` — `openArtifact` (modal) / `openArtifactFull` (tam ekran).
- Frontend: `types/message.ts` (`plan` kind), `chat/AskPrompt.tsx` (`PendingAsk.kind`),
  `hooks/useChatStream.ts` (plan adımı), `chat/PlanPrompt.tsx` (kart), `App.tsx` (render),
  `lib/stepKinds.ts` (referans girdisi).

## Notlar / sınırlar

- Köprü yalnız **claude-cli** sağlayıcısı içindir (native anthropic/minimax yolunda plan
  modu kavramı yoktur).
- `EnterPlanMode` zararsızdır (CLI'ı salt-okunur planlama durumuna alır); ask/read-only'de
  serbest, auto'da disallow.
- MCP/interaction tamamen kapalı bir turda (nadir) köprü kurulmadığından plan araçları da
  etkin olmaz — pratikte tüm sohbet turlarında interaction açıktır.
