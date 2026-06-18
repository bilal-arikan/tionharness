# 18 — Ajan Seviyeleri ve Dinamik Geçişler (Tasarım Notu)

> **Durum:** Faz T1 **uygulandı** (backend yeşil; commit eşzamanlı WIP reconcile sonrası) — T2/T3 beklemede  
> **Tarih:** 2026-06-18  
> **İlgili dosyalar:** `internal/settings/settings.go`, `internal/db/models.go`,
> `internal/agent/toolloop.go`, `internal/tools/delegate.go`, `internal/providers/kind.go`

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

## 8. Uygulama Fazları

### Faz T1 — Veri Modeli + Konfigürasyon (Temel) — ✅ UYGULANDI (2026-06-18)

**Backend:**
- `settings.AgentTierConfig` (provider/model/systemBoost) + `Settings.TierCheap/Medium/Smart` + `DefaultTier`;
  `Default()` varsayılanları (cheap→claude-cli, medium→haiku, smart→sonnet, defaultTier="" ), DTO + Patch + Apply
  + `normalize` (slot trim + DefaultTier enum doğrulama) — `settings.go`, `store.go`.
- `db.Agent.Tier` alanı + `AgentProfilePatch.Tier` (kısmi patch) — `db/models.go`, `db/store.go`.
- `agent.TierSlot` tipi + `Tunables.SetTierConfig/TierSlot` (canlı push) — `tunables.go`.
- **Çözüm seam'i** `agent/tier.go`: `resolveTier` (tier varsa+slot doluysa provider/model döner, yoksa
  ajanın kendi alanlarına düşer) + `effectiveProvider` (Provider örneği + model) + exported `ResolveTier`.
- Çağrı noktaları seam'e geçirildi: heartbeat (`runtime.go`), flow/task (`executor.go`), delegasyon
  (`delegate.go`), reflect/summary/title/compact funnel'i (`budget.go::guardedComplete` — açık model
  override'ı korunur), chat (`api/chat.go`, `chat_stream.go`, `chat_turn.go` → `ResolveTier`).
- `applySettings` → `SetTierConfig` push (`api/server.go`); `POST/PUT /api/agents` `tier` alanı + create'te
  `DefaultTier` seed (`api/agents.go`).

**Frontend:**
- `types/settings.ts` `AgentTierConfig`/`AgentTier` + 4 alan; `types/agent.ts` `Agent.tier`/`AgentPatch.tier`;
  `api/agents.ts` createAgent `tier`.
- `AgentSettingsForm`: "Seviye" dropdown'u (Özel/Ucuz/Orta/Zeki) — Özel'de ProviderModelSelect görünür,
  tier'da workspace ayarından geldiği notu.
- `settings/ProvidersPanel`: "Ajan Seviyeleri" bölümü (3 slot için ProviderModelSelect + "Yeni ajan varsayılan
  seviyesi" seçici); `SettingsPanel` save patch'ine tier alanları eklendi.

**Test/Build:** `agent/tier_test.go` (4 senaryo: tiersiz fallback, tier override, ayarsız tier fallback,
bilinmeyen tier fallback). `go build`/`vet`/`test ./internal/...` **yeşil**. Frontend: tier kodu `tsc` temiz
(ağaçtaki 2 hata eşzamanlı `market`/`hooks` WIP'inden — bu çalışmadan bağımsız).

**Not — commit:** çalışma ağacı eşzamanlı bir oturumun WIP'iyle iç içe (`runtime.go`'da
`heartbeatGoalBlock`+`GoalDone` değişikliği benim tier hunk'ımla aynı bölgede; frontend `App.tsx`/`HooksPanel`
WIP'i derlemiyor). Bu yüzden T1 kodu **commitsiz** — iki oturum reconcile edilince commit'lenecek (bu projede
yerleşik kalıp). Bu doküman güncellemesi ayrı commit'lenir.

### Faz T2 — `escalate_tier` Aracı

Değiştirilen: `internal/tools/` (yeni `builtin_tiertool.go`), `toolsetup.go`, `chat_stream.go`  
Eklenen: `sessionTierOverride` context key, SSE `tier_escalation` olayı  
Frontend: TurnSteps tier eskalasyon kartı, chat başlık rozeti  
**Sonuç:** Ajan, session içinde kendini akıllı tier'a yükseltebilir.

### Faz T3 — `call_agent` Tier Delegasyonu

Değiştirilen: `internal/tools/delegate.go`, `internal/agent/runtime.go` (DelegateRunner)  
Eklenen: Tier-bazlı ajan çözümü + ephemeral çağrı  
Frontend: `call_agent` araç kartında tier bilgisi  
**Sonuç:** Ajan, `call_agent(tier="smart", task="...")` ile doğrudan tier'a delege edebilir.

### Faz T4 (Opsiyonel) — Otomatik Yükseltme

`toolloop.go` içinde N ardışık hata veya boş cevap → otomatik `escalate_tier("medium")`.  
Ayarlar: `AutoEscalateOnFailure bool`, `AutoEscalateThreshold int`.

---

## 9. Alternatifler ve Değiş-Tokuşlar

### A. "Her ajan kendi modeline sahip" (mevcut durum)
Artısı: Basit. Eksisi: Dinamik uyum yok.

### B. Session içi model swap (bu tasarım, §4.3)
Artısı: Context korunur, kullanıcı kesintiye uğramaz.  
Eksisi: Yükseltme sonrası "ucuz" geçmişi "akıllı" modele görünür → akıllı modelin verimliliği
biraz düşebilir. Kabul edilebilir: akıllı modeller büyük context'i zaten iyi yönetir.

### C. Tam agent değişimi (call_agent ile)
Artısı: Temiz ayrışım, uzmanlaşmış persona.  
Eksisi: Context kopyalanmaz; delegated ajan soruyu kısa bir `task` ile alır.  
**Öneri:** İki yaklaşım birlikte kullanılabilir: `escalate_tier` (session swap) basit
durum için, `call_agent(tier=...)` (delegation) uzmanlaşmış iş bölümü için.

### D. Tier'ı sadece katalog metaverisi olarak tut (kod değişikliği en az)
Sadece `Agent.Tier` etiket olarak UI'da görünür, runtime'da etki etmez.  
Eksisi: Asıl değeri (dinamik geçiş) sunmaz.

---

## 10. Sıradaki Adımlar

1. `Settings.TierConfig` + `db.Agent.Tier` + `resolveEffectiveProvider` → **Faz T1**
2. UI'da WorkspacePanel + AgentSettingsForm güncellemeleri
3. `escalate_tier` tool + SSE olay → **Faz T2**
4. `call_agent` tier delegasyonu → **Faz T3**
