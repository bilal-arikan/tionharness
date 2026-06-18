# 18 — Ajan Seviyeleri ve Dinamik Geçişler (Tasarım Notu)

> **Durum:** **UYGULANDI (basitleştirilmiş model)** — backend yeşil; commit eşzamanlı WIP reconcile sonrası  
> **Tarih:** 2026-06-18 (revize: 2026-06-19)  
> **İlgili dosyalar:** `internal/db/models.go` (`Agent.Tier`), `internal/agent/delegate.go`,
> `internal/tools/delegate.go`, `internal/api/agents.go`, `frontend/.../AgentSettingsForm.tsx`

---

## 0. REVİZYON (2026-06-19) — tier = saf etiket

İlk tasarım (aşağıdaki §4'ün eski hali) **workspace düzeyinde Ucuz/Orta/Zeki provider+model
konfigürasyonu** + ajanın efektif modelini bu konfigürasyondan çözme + `escalate_tier` ile
session-içi model yükseltme öneriyordu. Kullanıcı geri bildirimiyle bu **kaldırıldı**.

**Yeni (uygulanan) model — çok daha basit:**

- `Agent.Tier` sadece bir **etiket**tir (`cheap`/`medium`/`smart` veya boş) — ajanın
  **sağlayıcı/modelini DEĞİŞTİRMEZ**. Her ajan her zaman kendi `Provider`/`Model`'iyle çalışır.
- Workspace'te ayrı tier provider/model konfigürasyonu **yok** (Settings'ten kaldırıldı).
- `escalate_tier` aracı, session `TierOverride`, `StepTier` ve tüm model-çözüm katmanı **kaldırıldı**.
- Etiketin tek işlevi **delegasyon yönlendirmesi**: `call_agent(tier:"smart")` → "smart" etiketli
  bir ajana görevi devreder (orijinal hedef: "session içinde ajanlar arası görev aktarımı").

> Aşağıdaki §4.1 (workspace config), §4.2 (efektif provider çözümü), §4.3 (escalate_tier /
> session override) bölümleri **artık geçersiz** — tarihsel bağlam için bırakıldı. Geçerli olan:
> §4.4 (call_agent tier delegasyonu, etiket-bazlı) + §11 (Güncel Uygulama).

---

## 1. Problem

Şu an her ajan `Provider` + `Model` alanlarıyla statik olarak konfigüre edilir. Bir görev
başladığında hangi modelin kullanılacağına önceden karar verilmiştir; zor bir soruyu çözemeyip
duran ucuz bir ajan daha yetenekli bir modele geçemez. Bu kısıt:

- Basit görevlerde akıllı (pahalı) model boşa harcar.
- Karmaşık görevlerde ucuz model yetersiz kalır ve kullanıcı müdahalesi gerekir.

**Hedef:** Workspace düzeyinde üç kalite katmanı (Ucuz / Orta / Zeki) tanımlayabilelim;
ajanlar katmanla oluşturulsun ve bir session içinde görevin zorluğuna göre daha yüksek
katmana geçiş yapılabilsin — hem ajan inisiyatifiyle (araç), hem de sistem otomasyonuyla.

---

## 2. Hedefler ve Kapsam Dışı

### Hedefler

- Workspace'e **üç katman konfigürasyonu** ekle (provider + model + opsiyonel sistem-prompt eki).
- Ajana **`Tier`** alanı ekle; bu alandan çözülen provider/model, `Agent.Provider/Model` alanlarının
  önüne geçsin.
- `call_agent`'ı **tier-bazlı** delegasyona genişlet (`agent` adı yerine `tier` verilebilsin).
- Ajan, kendi session'ında **`escalate_tier`** aracıyla model yükseltmesi talep edebilsin.
- Yeni ajanlar için workspace'in **varsayılan tier**'ı geçerli olsun.
- **Kapsam dışı (şimdilik):** otomatik yükseltme (fail-count tabanlı), tier düşürme,
  çapraz-workspace tier erişimi.

---

## 3. Mevcut Altyapı (Özet)

| Bileşen | Nerede | Ne yapıyor |
|---|---|---|
| `db.Agent.Provider/Model` | `internal/db/models.go` | Ajan başına statik provider+model |
| `Settings.DefaultProvider/Model` | `internal/settings/settings.go` | Yeni ajanlar için workspace varsayılanı |
| `providers.ProviderKind` | `internal/providers/kind.go` | Plugin-tabanlı provider kaydı |
| `Runtime.guardedComplete` | `internal/agent/budget.go` | Tüm provider çağrılarının tek kapısı (bütçe + kayıt) |
| `call_agent` (DelegateRunner) | `internal/tools/delegate.go` | Ajan→ajan senkron delegasyon; context köprüsü |
| `send_agent_message` | `internal/tools/builtin_agentmsg.go` | Ajan→ajan async inbox |
| `completeTraced` | `internal/agent/toolloop.go` | Araçlı tur motoru; provider'ı `agent.Provider`'dan alır |

**Kritik nokta:** `Runtime.completeTraced` şu an `r.providers.Get(agent.Provider)` çağrısını
doğrudan `db.Agent` alanından yapar. Tier sistemi buraya en az dokunarak entegre edilmeli.

---

## 4. Önerilen Tasarım

### 4.1 Veri Modeli Değişiklikleri

#### 4.1.1 Settings — Tier Konfigürasyonları

```go
// AgentTierConfig, bir kalite katmanının provider + model + opsiyonel sistem prompt ekini tanımlar.
type AgentTierConfig struct {
    Provider    string `json:"provider"`    // "claude-cli" | "anthropic" | "minimax" | custom id
    Model       string `json:"model"`       // boş = provider varsayılanı
    SystemBoost string `json:"systemBoost"` // opsiyonel: soul'a eklenen kısa yönerge
}

// Settings'e yeni alanlar:
TierCheap   AgentTierConfig `json:"tierCheap"`
TierMedium  AgentTierConfig `json:"tierMedium"`
TierSmart   AgentTierConfig `json:"tierSmart"`
DefaultTier string          `json:"defaultTier"` // "cheap" | "medium" | "smart" | "" (custom)
```

**Varsayılan değerler** (Default() fonksiyonunda):
- `TierCheap`  → `{Provider: "claude-cli", Model: ""}` (anahtarsız, ücretsiz)
- `TierMedium` → `{Provider: "anthropic", Model: "claude-haiku-4-5-20251001"}`
- `TierSmart`  → `{Provider: "anthropic", Model: "claude-sonnet-4-6"}`
- `DefaultTier` → `"cheap"`

#### 4.1.2 db.Agent — Tier Alanı

```go
// Tier is the quality-tier assignment for this agent.
// "cheap" | "medium" | "smart" → effective provider+model resolved from workspace tier settings.
// "" means the agent uses its own Agent.Provider + Agent.Model fields directly (custom/manual).
// Tier takes precedence; Agent.Provider/Model serve as the fallback when Tier == "".
Tier string `json:"tier,omitempty"`
```

**Önemli:** `Agent.Provider/Model` alanları silinmez — `Tier == ""` olan ajanlar için (veya
tier sisteminden önce oluşturulan eski ajanlar için) fallback işlevi görür.

### 4.2 Runtime — Efektif Provider Çözümü

`guardedComplete` ve `completeTraced` çağrılarında `agent.Provider` doğrudan kullanılmak yerine
yeni bir `resolveEffectiveProvider(agent, settings)` yardımcısına devredilir:

```go
// resolveEffectiveProvider, bir ajanın efektif provider+model çiftini döndürür.
// Tier atanmışsa workspace tier konfigürasyonundan, aksi halde agent alanlarından çözülür.
func resolveEffectiveProvider(agent db.Agent, cfg tierConfig) (provider string, model string) {
    switch agent.Tier {
    case "cheap":
        return cfg.Cheap.Provider, cfg.Cheap.Model
    case "medium":
        return cfg.Medium.Provider, cfg.Medium.Model
    case "smart":
        return cfg.Smart.Provider, cfg.Smart.Model
    }
    return agent.Provider, agent.Model
}
```

`Tunables` bu konfigürasyonu `SetTierConfig(...)` ile çalışma zamanında güncellenebilir hale getirir
(tıpkı diğer ayarlanabilir parametreler gibi).

### 4.3 İntra-Session Tier Geçişi

#### 4.3.1 Session-Tier Context Key

```go
// sessionTierKey, mevcut session'a özel tier override'ı context'te tutar.
// Bir escalate_tier çağrısı buraya yazar; completeTraced buradan okur.
type sessionTierKey struct{}

type sessionTierOverride struct {
    Tier string // "medium" | "smart"
}
```

`completeTraced`, provider'ı çözerken önce context'teki session-tier override'a bakar;
varsa onu kullanır, yoksa `resolveEffectiveProvider`'a düşer. Bu şekilde hiçbir kalıcı
agent verisi değişmez — yükseltme yalnız mevcut tur döngüsünde etkilidir.

> **Neden context?** Session içindeki bütün tur-çağrıları (tool loop iterasyonları dahil)
> aynı `ctx` zincirini paylaşır. Context key'i tier'ı güvenli biçimde tur sonuna kadar
> taşır ve "tur bitti, bir sonraki tur da yükseltilmiş modelle mi çalışacak?" sorusunu
> açık bırakır (bkz. §4.3.3).

#### 4.3.2 `escalate_tier` Aracı

```go
// Yeni araç: escalate_tier
// Gate: EnableDelegation (mevcut delegation guard'ını yeniden kullanır)
type escalateTierInput struct {
    To     string `json:"to"`     // "medium" | "smart"
    Reason string `json:"reason"` // opsiyonel, trace'e yazılır
}
```

**Davranış:**
1. `To` tier'ı mevcut ajandan yüksek mü kontrol et (`cheap → medium → smart`).
   Düşürme veya aynı tier → hata (model kendiyle daha az verimli kalabilir).
2. Workspace'te ilgili tier konfigürasyonu doluysa context'e `sessionTierOverride{Tier: to}` yaz.
3. Araç cevabı: `"Tier yükseltildi: cheap → medium (claude-haiku-4-5-20251001). Kalan turlar bu modelle çalışacak."`
4. Araç trace'ine `StepTierEscalation` kaydı düşer → UI'da görsel rozet.

#### 4.3.3 Yükseltmenin Kapsamı — İki Seçenek

| Seçenek | Kapsam | Uygulama |
|---|---|---|
| **A — Sadece bu tur** | Mevcut tool-loop iterasyonu | `ctx` zaten turla sınırlı |
| **B — Session sona erene dek** | Tüm sonraki turlar | `chatRun` veya `invokeTraced` üst bağlamına yaz |

**Öneri: B seçeneği** (session-kalıcı). Gerekçe: ajan zor bir soruyla karşılaştıktan sonra
yükseltme yapar; kullanıcı birkaç mesaj daha gönderiyor — modelin session boyunca akıllı kalması beklenir.
Uygulama: `chat_stream.go`'daki `chatRun` struct'ına `tierOverride string` eklenir; `escalate_tier`
çağrısı SSE event olarak `tier_escalation` yayar ve `chatRun.tierOverride` güncellenir; sonraki
turlar bu değeri `ctx`'e enjekte eder.

### 4.4 `call_agent` — Tier-bazlı Delegasyon

Mevcut `callAgentInput`:
```go
type callAgentInput struct {
    Agent string `json:"agent"` // isim veya id
    Task  string `json:"task"`
}
```

Yeni `callAgentInput`:
```go
type callAgentInput struct {
    Agent string `json:"agent,omitempty"` // isim veya id (Tier ile birlikte opsiyonel)
    Tier  string `json:"tier,omitempty"`  // "cheap" | "medium" | "smart"
    Task  string `json:"task"`
}
```

**Çözüm mantığı** (`DelegateRunner` içinde):
- `Agent` dolu → mevcut davranış (isme/id'ye göre bul).
- `Tier` dolu, `Agent` boş → workspace'te `agent.Tier == tier` olan ilk aktif ajanı bul.
  Yoksa `tier`'ın provider/model'i ile **ephemeral çağrı**: mevcut ajanın kopyası gibi davranır
  ama provider/model tier konfigürasyonundan gelir.
  > Not: Ephemeral çağrı yeni bir `db.Agent` oluşturmaz; `Runtime.completeTraced`'i
  > geçici bir `db.Agent{Provider: cfg.Provider, Model: cfg.Model, Soul: callerSoul}`
  > ile çağırır.
- Her ikisi de dolu → `Agent` önceliklidir.

### 4.5 Ajan Oluşturma — Tier Atama

`POST /api/agents` endpoint'i `tier` alanını kabul eder. Backend, `tier != ""` iken
`provider/model` alanlarını tier konfigürasyonundan doldurur (ön yüz: hem seçenek hem bilgi).

`seedDefaultAgent` (workspace oluşturmada), `DefaultTier` ayarını kullanarak yeni workspace
ajanını doğru tier ile oluşturur.

---

## 5. API Değişiklikleri

### Settings (`GET/PUT /api/settings`)

DTO'ya eklenir:
```json
{
  "tierCheap":   { "provider": "claude-cli", "model": "" },
  "tierMedium":  { "provider": "anthropic", "model": "claude-haiku-4-5-20251001" },
  "tierSmart":   { "provider": "anthropic", "model": "claude-sonnet-4-6" },
  "defaultTier": "cheap"
}
```

### Agents (`GET /api/agents`)

Agent DTO'ya `tier` eklenir. `tier != ""` durumunda frontend efektif model/provider'ı
tier konfigürasyonundan türetir; `tier == ""` → mevcut `provider/model` alanlarını kullanır.

### Chat SSE — Yeni Olay

```json
{ "type": "tier_escalation", "from": "cheap", "to": "smart", "model": "claude-sonnet-4-6", "reason": "..." }
```

Frontend bu olayı alır → chat başlık çubuğunda geçici rozet + trace kartı.

---

## 6. Frontend Değişiklikleri

### 6.1 Workspace Ayarları → "Ajan Seviyeleri" Paneli

`WorkspacePanel` (veya `SettingsPanel` → "Bu Workspace" sekmesi) yeni alt bölüm:

```
┌─ Ajan Seviyeleri ──────────────────────────────────────────┐
│ Ucuz   [Provider ▼] [Model ▼]  [Sistem eki ...]           │
│ Orta   [Provider ▼] [Model ▼]  [Sistem eki ...]           │
│ Zeki   [Provider ▼] [Model ▼]  [Sistem eki ...]           │
│ Yeni ajan varsayılanı: [Ucuz ▼]                           │
└────────────────────────────────────────────────────────────┘
```

### 6.2 Ajan Oluşturma / Ayarları

`AgentSettingsForm`'a `Tier` dropdown'u (`Ucuz | Orta | Zeki | Özel`).
`Özel` seçilince mevcut `Provider/Model` alanları görünür; diğerlerinde gri/salt-okunur
(efektif değer tier konfigürasyonundan türetilir).

### 6.3 Chat — Tier Rozeti

- Sohbet başlık çubuğunda ajan adının yanında tier rozeti (🟢 Ucuz / 🟡 Orta / 🔴 Zeki).
- `tier_escalation` SSE olayı gelince rozet canlanarak güncellenir.
- `TurnSteps` aktivite izinde `StepTierEscalation` → "⬆ Tier yükseltildi: Ucuz → Zeki" kartı.

---

## 7. Güvenlik ve Bütçe Notları

- `escalate_tier` aracı **`EnableDelegation`** gate'i arkasında — varsayılan kapalı;
  kullanıcı bilinçli açar.
- Tier yükseltmesi yalnız **yukarı** gider (cheap→medium veya cheap→smart veya medium→smart).
  Aynı veya daha düşük tier talebi `"already at or above requested tier"` hatasıyla reddedilir.
- Tier, ajan bütçe sayaçlarını (`DailyCallLimit/DailyTokenLimit`) değiştirmez —
  akıllı tier çağrıları daha pahalı olsa da mevcut `RecordUsage` ile izlenir.
- Workspace'te tier konfigürasyonu boşsa (`Provider == ""`) escalate hata döner;
  kullanıcı konfigürasyon eksikliğini düzeltebilir.

---

## 8. Uygulama Fazları (eski plan — yalnız tarihsel)

> ⚠️ Aşağıdaki T1/T2/T3 eski (provider/model konfigürasyonlu) tasarımı anlatır ve **uygulanıp
> sonra geri alındı**. Güncel, yürürlükteki uygulama için **§11**'e bak.

Eski plan: T1 = workspace tier config + efektif provider çözümü; T2 = `escalate_tier` (session
model yükseltme); T3 = `call_agent` tier delegasyonu (ephemeral ajan dahil). T1+T2 ve T3'ün
ephemeral kısmı **geri alındı** (kullanıcı geri bildirimi — bkz §0).

---

## 9. Alternatifler ve Değiş-Tokuşlar

### A. "Her ajan kendi modeline sahip" + tier etiketi (UYGULANAN)
Artısı: Çok basit, anlaşılır; ajanın modeli tek yerde (kendi alanı). Etiket sadece yönlendirme.  
Eksisi: Tek bir ajan kendi modelini dinamik değiştiremez (ama farklı modeldeki bir ajana delege edebilir).

### B. Workspace tier config + session içi model swap (ESKİ, GERİ ALINDI)
Artısı: Tek ajan model yükseltebilir.  
Eksisi: İki yerde model kaynağı (ajan + tier config) → kafa karıştırıcı; kullanıcı reddetti.

### C. call_agent ile agent değişimi (UYGULANAN — etiket bazlı)
Etiketli ajana delege: temiz ayrışım, uzmanlaşmış persona, "session içinde ajanlar arası görev
aktarımı" hedefini doğrudan karşılar.

---

## 11. Güncel Uygulama (yürürlükteki) — ✅ 2026-06-19

**Model:** `Agent.Tier` saf bir **etiket** (`cheap`/`medium`/`smart`/boş). Ajanın provider/model'ini
değiştirmez. Tek işlevi: `call_agent(tier:"…")` ile etiketli ajana delegasyon yönlendirmesi.

**Backend:**
- `db.Agent.Tier` (etiket) + `AgentProfilePatch.Tier` (kısmi patch) — `db/models.go`, `db/store.go`.
- `tools/delegate.go`: `DelegateRunner` imzası `(ctx, target, tier, task)`; `callAgentInput` += `Tier`;
  şema (`tier` enum cheap/medium/smart, `agent` opsiyonel, yalnız `task` zorunlu); `Call` doğrulaması
  (agent **veya** tier).
- `agent/delegate.go` `resolveDelegateTarget`: isimli hedef → `resolveAgent`; tier → o etikete sahip ilk
  uygun ajan (caller/visited hariç), kendi provider/model'iyle koşar; hiç etiketli ajan yoksa açık hata.
  Mevcut depth/budget/cycle guard'ları değişmeden uygulanır.
- `POST/PUT /api/agents` `tier` alanını kabul eder (etiket).

**Frontend:**
- `types/agent.ts` `Agent.tier`/`AgentPatch.tier`; `types/settings.ts` `AgentTier` tipi; `api/agents.ts`
  createAgent `tier`.
- `AgentSettingsForm`: ProviderModelSelect **her zaman** görünür + ayrı **"Seviye etiketi"** dropdown'u
  (Etiket yok / Ucuz / Orta / Zeki) + "yalnız etiket, modeli değiştirmez" notu.

**Kaldırılanlar (eski tasarımdan):** `settings` tier config (TierCheap/Medium/Smart/DefaultTier),
`Tunables.SetTierConfig/TierSlot`, `agent/tier.go` (resolveTier/effectiveProvider/escalator),
`tools/escalate.go` (`escalate_tier`), `db.Session.TierOverride`, `StepTier`+`TierStep.tsx`,
ProvidersPanel "Ajan Seviyeleri" bölümü.

**Test/Build:** `delegate_test.go` (etiketli-ajan yönlendirme + "no agent tagged" hata + guard'lar yeni
imzaya). `go build`/`vet`/`test ./internal/...` **yeşil** (16 paket ok); frontend tier kodu `tsc` temiz
(ağaçtaki 2 hata `market`/`hooks` WIP'inden — bağımsız).

---

## 10. Sıradaki Adımlar

1. **Canlı test** — gateway/Chrome bağlanınca: ajana "smart" etiketi ata → başka ajandan
   `call_agent(tier:"smart")` ile delegasyonu uçtan uca doğrula.
2. **Reconcile + commit** — eşzamanlı WIP çözülünce tier-etiket kodunu commitle.
3. (Ops.) Roster/chat'te ajan adı yanında küçük tier rozeti (🟢/🟡/🔴) — kozmetik.
