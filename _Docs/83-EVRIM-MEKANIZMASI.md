# 83 — Evrim Mekanizması: Hedef-Güdümlü Workspace Optimizasyonu

> **Özet (2026-09-06):** Araştırma + beyin fırtınası dokümanı; **E0–E2 uygulandı** (Goal varlığı, `goal-writer` sistem ajanı, Hedefler ekranı; konfigürasyon snapshot'ı + oturum atfı + LLM'siz fitness; `workspace-evolver` yalnız-öneri geçişi + kodda kural katmanı — §8), E3+ tasarım. Amaç:
> workspace için kaydedilip düzenlenebilen **Hedefler (Goals)** tanımlamak ve ajan
> hiyerarşisi, araç atamaları, ajan/skill promptları, otomasyon ve zamanlamalar, model
> ve düşünme seviyesi seçimleri gibi ayarların zamanla bu hedeflere göre optimize
> edilmesi. Üç kaynaktan sentez: (1) kod tabanı haritası — TionHarness'te bugün ne var,
> ne yok; (2) akademik sistemler (DSPy/GEPA, Darwin Gödel Machine, AlphaEvolve, ADAS,
> MASS, Optimas, ACE …); (3) pratisyen ve ürün deneyimleri (Karpathy autoresearch,
> Langfuse, Decagon, Sierra, Intercom Fin, Dust, Anthropic Dreaming/Outcomes …).
> En önemli sonuçlar: **her optimizer bir Goodhart makinesidir** (kısıtlar prompt'ta
> değil Go kodunda yaşamalı), **öner-onayla + fark (diff) + kanıt** varsayılan olmalı,
> mevcut `recipe-optimizer` / `insight` / `curator` / ajan kalıtımı altyapısı bu
> özelliğin %60'ını zaten taşıyor; eksik olan **Goal varlığı, fitness hesabı,
> konfigürasyon anlık görüntüsü (snapshot) + atıf, genel öneri şeması ve geri-alma
> defteri**. İki tasarım kararı daha: evrim **kısım kısım** (hedef kapsamı = evrim birimi,
> §8.2) ilerler ve **git-ağacı benzeri tek geçmiş** (hedef revizyonları + ileride snapshot/
> ledger, §8.3) üzerinden izlenir. Bir ajan için: bu özellik istendiğinde önce §2'deki
> mevcut yapı taşlarını, sonra §5'teki faz planını ve §8'i oku; yeniden yazma, genelleştir.

---

## 1. İstenen şey

Kullanıcı workspace için birden fazla **Hedef** tanımlar (kaydedilir, düzenlenir,
kapatılır). Örnekler:

- "Kanban kartları daha hızlı kapansın" (cycle time ↓, kart/gün ↑)
- "Koşu başına maliyet düşsün, başarı oranı düşmesin" (USD/run ↓, success ≥ %90)
- "İnsan müdahalesi azalsın" (ask_user / kapı beklemesi ↓)
- "Kod inceleme reçetesi daha az tur harcasın" (reçete başına AvgSessions ↓)
- "Türkçe doküman kalitesi artsın" (rubrik/LLM-yargıç puanı ↑ — açık uçlu)

Sistem zamanla şu yüzeyleri bu hedeflere göre değiştirir (öneri ya da otomatik):

| Yüzey | Ne değişir |
|---|---|
| Ajan hiyerarşisi | koordinatör/işçi ağacı, hangi profil hangi fazda, delegasyon derinliği |
| Araç atamaları | ajan başına `ToolOverrides` (full/summary/name-only/hidden/blocked), MCP sunucu aç/kapa |
| Promptlar | ajan `soul`/`identity`, skill gövdeleri, workspace `config/prompts/*.md` override'ları, `Instructions` |
| Skill'ler | görünürlük, `AutoSummary`, hangi ajana atanmış, reçete fazları/izleyicileri |
| Otomasyon / zamanlama | tetik eşikleri, cooldown, `MaxIterations`, cron sıklığı, hedef ajan |
| Model / düşünme | `Model`, `ThinkingLevel`, sağlayıcı örneği (kind+instance birlikte) |
| Diğer ayarlar | `TerseMode`, `PromptEpochEnabled`, lazy tool, insight lens ayarları |

## 2. TionHarness'te bugün ne var (yeniden yazma, genelleştir)

Kod haritası (2026-09-04). Depo dosya tabanlı: "tablo" = `store/` altında dizin.

### 2.1 Zaten var olan dört öz-iyileştirme mekanizması

| Mekanizma | Dosya | Ne yapar | Bu özelliğe katkısı |
|---|---|---|---|
| **Reçete optimizer** (Rota F4) | `internal/agent/recipe_optimizer.go`, `internal/db/store_optimizer.go` | ≥3 yeni koşu / failed / elle tetik; LLM 9 eylemden öneri üretir; kanıt/sayı yoksa **kodda atılır**; büyüme bütçesi; `AutoPrune` ile budama sınıfı otomatik uygulanır (`skills.ApplyRecipeProposal`, sürüm artar, yeniden ayrıştırıp doğrular) | Doğrudan şablon. Genel optimizer bunun çok-yüzeyli hali |
| **İçgörü taraması** (`_Docs/60`) | `internal/insight/*`, `internal/agent/insightscan.go` | Lens dosyaları, inkremental ledger, üç kanal (`app-fix`, `workspace-opt`, `recipe-opt`), `Finding` yaşam döngüsü, **`Regressed`** (kapanan bulgu geri gelirse işaret), **`ErrAppliedNeedsEvidence`** (applied için `AppliedEntity` zorunlu) | Öneri kuyruğu + yaşam döngüsü + regresyon tespiti hazır. `workspace-tuning` lensi zaten skill > agent > tools-config > hook/automation > insight > CLAUDE.md öncelik sırasıyla öneri üretiyor |
| **Küratör** (Rota F3) | `internal/db/store_curator.go` | LLM'siz haftalık geçiş; `CuratorAction{Kind, Entity, Reason, Evidence, Applied}` | Deterministik, kanıtlı, provenance'lı eylem kaydı deseni |
| **Ajan kalıtımı** (`_Docs/82`) | `internal/db/agent_inherit.go:50-115` | 16 kalıtılabilir alan (`soul, identity, provider, model, thinkingLevel, nativeWebSearch, permissionMode, inboundPolicy, avatar, color, tools, allowedTools, skills, coordinatorMode, coordinatorWorkflow, coordinatorPrompt`), `derive`, `restore-default` | **Hazır genom şeması + varyant sistemi.** Türet → 1-2 alanı override et → ölç → geri al. Ajan için ayrı sürümleme tablosu gerekmez |

### 2.2 Fitness sinyali olarak kullanılabilir telemetri (hepsi LLM'siz)

| Sinyal | Kaynak | Alanlar |
|---|---|---|
| Reçete istatistikleri | `internal/trajectory/recipestats.go:17` | `Runs, Done, Failed, Abandoned, AvgDurationSec, AvgTokens, AvgCostUSD, AvgSessions, GateWaitSec, UnfiredWatchers, GhostPhases` — **sürüm başına** |
| Kullanım / maliyet | `internal/db/store_usage.go` | cache read/write ayrımı, `ByKind`, `ByModel`, `CoolingWasteUSD` |
| Pano sonuçları | `internal/api/dashboard_outcomes.go` | `VelocityByDay, CardsDoneWindow, CycleTimeAvgSec, RunSuccessRate` |
| Otomasyon sayaçları | `Automation.IterationCount/LastError`, `automation-fires/` | tetik verimi, hata oranı |
| Öz-iyileşme sayaçları | `_Docs/56` `errClass`, stuck-loop | negatif sinyal |
| İnsan geri bildirimi | `Task.Rating` (`models.go:753`), insight bulgu kabul/ret, `IgnoredRecommendations` | kalite + "bunu bir daha önerme" listesi |
| Dersler | `store_lessons.go` | hata→ders metni (LLM üretimi, kanıt bağlı) |

### 2.3 Koruyucu değişmezler (zaten kodda)

- Prompt override'ları `prompts.Validate` ile zorunlu `{{placeholder}}` koruması geçmeden yazılamaz.
- `BlockedTools` türetilir; optimizer **yalnız `ToolOverrides`** yazmalı.
- `provider` alanı kind+instance birlikte taşınır; tek başına yazmak bozuyor.
- Reçete yüklemede `RecipeGrowthBudget = 9` zorlanır.
- `WSSettings.IgnoredRecommendations`: kullanıcının reddettiği öneriler; yeniden önerilmemeli.

### 2.4 Gerçek boşluk

Depoda `goal`/`fitness`/`experiment`/`A/B` araması yalnız tesadüfi sonuç veriyor
(`Session.Goal` serbest metin, `SubagentSpec.Objective`). **Yok olanlar:**

1. Birinci sınıf **Goal** varlığı ve metrik tanımı.
2. **Fitness hesabı** (goal × telemetri → puan, LLM'siz).
3. **Konfigürasyon anlık görüntüsü** (snapshot/genom hash'i) ve **koşu→snapshot atfı**. Agent/tool/automation/settings için geçmiş yok; `PUT` üzerine yazıyor.
4. **Genel öneri şeması**: `insight.RecipeProposal` reçete slug'ına ve reçete eylemlerine sabit.
5. **Uygulama + geri alma defteri** (çok yüzeyli, before/after).
6. **Deney (varyant) kaydı** ve terfi kapısı.

## 3. Araştırma: benzer şeyleri yapanlar ve karşılaştıkları sorunlar

### 3.1 Akademik sistemler (mekanizma + bildirilen sorun)

| Sistem | Mekanizma | Bildirilen sorun |
|---|---|---|
| **DSPy MIPROv2 / GEPA** | Bayesian talimat+örnek araması; GEPA iz okuyup yansıtır, örnek-başına Pareto cephesi tutar (RL'e göre 35× az rollout) | Açgözlü "tek en iyi" seçimi yerel optimuma çöker (+6 vs +12); validasyon→test açığı; kazanımlar göreve bağlı; ucuz model için optimize edilen prompt güçlü modele taşınmıyor; prompt DSPy dışına çıkarınca bozuluyor |
| **OPRO / TextGrad / EvoPrompt / PromptBreeder** | Optimizer LLM'e (prompt, skor) izi verip yenisini ister; metinsel gradyan; GA/DE popülasyonu | Küçük optimizer modelle etkisiz; düz talimat zor baseline; 2026 edit-düzeyi analiz: kazançların çoğu dev-set overfitting; modeller arası transfer tutarsız (**model yükseltmesi = non-stationarity**) |
| **Darwin Gödel Machine** (Sakana) | Ajan kendi Python iskeletini yazar; arşiv; ebeveyn seçimi skora + az çocuğa göre | 80 iterasyon ≈ 2 hafta, ~$22k. **Halüsinasyon tespit işaretlerini silerek** mükemmel skor aldı; yalnız soy izi (lineage) sayesinde yakalandı. AI Scientist: kendi timeout'unu düzenledi, kendini yeniden başlattı, 1 TB checkpoint |
| **AlphaEvolve / OpenEvolve / ShinkaEvolve** | Program evrimi; MAP-Elites + adalar; kademeli değerlendirme; yenilik reddi (embedding) + LLM backend'leri üzerinde UCB1 bandit | Yalnız "otomatik doğrulanabilir" çözümlerde çalışır; altyapı hatası (çocuk yanlış adaya) aramayı sessizce bozdu |
| **ADAS / AFlow / AgentSquare / MaAS** | Meta-ajan ajan kodu yazar; MCTS iş akışı grafiği; modüler arama | Karşı çalışma (2025): tasarlanan ajanlar çıkarımda %15-20 pahalı, başabaş ~15k sorgu, önceki tasarımları beslemek **daha kötü**, çeşitlilik çöküşü. Tek ALFWorld değerlendirmesi ~$60 |
| **MASS** (Google) / GPTSwarm | Blok-prompt → topoloji → iş-akışı-prompt aşamalı arama | **Prompt kalitesi topolojiyi domine eder**; topolojilerin küçük kısmı işe yarar; değerlendirme gürültüsü; zayıf transfer. MAS-PromptBench: çok-ajanlıda beş tekrar eden mod: overfitting, **kredi atfı**, orantısız token, kararsızlık, mimariler arası taşınmama |
| **Trace / Agent-Lightning / Optimas** | Yürütme DAG'ı + geri bildirim → LLM optimizer; RL eğiticiyi çalışma zamanından ayırma; **bileşen başına yerel ödül** (global ile korelasyon kısıtı), her bileşen kendi yöntemiyle (prompt: OPRO, model seçimi: kategorik dağılım) | Skaler geri bildirim kredi atfına yetmiyor; binlerce düğümlü graf yok. **Optimas, karışık prompt/model/hiperparametre araması için en yakın tasarım** |
| **SICA / Reflexion / Voyager / Dynamic Cheatsheet / ACE** | Kendi kodunu düzenleyen ajan (fayda = 0.5 skor + 0.25 maliyet + 0.25 süre, gözetmen); sözel yansıtma belleği; beceri kütüphanesi; birikimli bağlam | **Bağlam çöküşü:** cheatsheet 18.282 token/%66,7 → tek LLM yeniden yazımıyla 122 token/%57,1 (baseline'ın altı). ACE çözümü: madde madde delta güncelleme, embedding dedup, ayrı Generator/Reflector/Curator |
| **RouteLLM / FrugalGPT / Martian / Not Diamond** | Tercih verisiyle eğitilmiş yönlendirici; kaskad | RouterArena: hepsi oracle'ın altında, çoğu ucuz modelin yettiğini fark edemiyor; Not Diamond pahalıyı fazla seçiyor; gerçekçi tasarruf ~%35 (başlıktaki %85-98 değil) |

### 3.2 Pratisyen ve ürün deneyimleri

| Kaynak | Ne yaptı | Ne ters gitti / ne öğrendi |
|---|---|---|
| **Karpathy autoresearch** | Tek dosya, tek metrik, 5 dk bütçe, tut/at; gecede ~100 deney | Kısıt = özellik: eval hattı ajanın yazma alanının dışında; insan yalnız `program.md`'yi düzenler |
| autoresearch #322 (Gomoku) | Aynı döngü | Ajan ağı eğitmek yerine alpha-beta motoru yazdı (%99 kazanç, `train_time_sec: 0.0`); prob eklenince `net.forward()`'ı bir kez çağırıp sonucu attı. Prompt talimatı "teknik uyumlu" hile üretir; çözüm SDK hook'larıyla dosya-düzeyi ACL |
| **Langfuse** (skill dosyası) | Ağırlıklı değerlendiriciyle SKILL.md'yi 0.35→0.82 | **Kullanıcı onay kapısını sildi** (testte insan yoktu, onay saf maliyetti), test etmeyen bölümleri sildi. "Ölçülmeyen kesilir" |
| **AgentSelfEdit** | Başarısız izden prompt önerisi, holdout'ta A/B, deterministik kapı (n tabanı, etki, p<0.05, dondurulmuş bölüm, edit-mesafesi, drift) | Ters yazılmış kontrol (`p<0.95`) gürültüyü kazanç saydı; 15 iterasyon/4.150 çağrıda **sıfır** terfi. "Kapı üründür, optimizer değil"; küçük eval seti ile zayıf öneri ayırt edilemez |
| Ken Ashe | Bellek tabanlı öz-iyileştirme | Görev sırası karıştırılınca kazanım yok oldu; "şanslı ilk koşu kendinden emin bellek yazar, birikir" |
| **Cekura** (ses ajanı) | 7 fazlı döngü, `auto_mode:false` diff-onay, insan kararı makine skorunu ezer | Tam senaryo setinde doğrula; **"değişmeme imzası"** (3 iterasyon aynı hatalar) → yapısal düzeltmeye tırmandır (osilasyon önleme) |
| **Warp** | Temel skill + zamanlanmış "iyileştirici" ajan, öneriler PR olarak | Elle AGENTS.md düzenlemek ölçeklenmedi; thumbs up/down işe yaramaz ("neden" yok); "kural değil ilke yaz" |
| Osmani / Alderson / bloat raporları | Oturum loglarından CLAUDE.md önerisi | 100 repo denetimi: %42 bağlam şişkinliği; 56k token sistem promptu; 250 unutulmuş skill; eski model için yazılmış kurallar silinmiyor ("silmek riskli hissettiriyor") |
| **Anthropic Dreaming / Outcomes / auto-dream** (2026-05) | Arka planda bellek konsolidasyonu → **incelenebilir diff**; 24 sa/5 oturum tetik; Outcomes = ayrı bağlamda değerlendirici ajanın düz-dilli rubriği | Harvey: dreaming ancak sıkı outcomes rubriğiyle işe yaradı; 500 satırlık MEMORY.md 200'de sessizce kırpılıyordu |
| Anthropic çok-ajan araştırma / Cognition | Araç açıklamalarını ajan yeniden yazdı (%40 süre ↓); paralel işçiler çelişen kararlar aldı | Küçük insan evali erken yakalar; hiyerarşide **atıf zor**, bağlam parçalı; yazma adımını merkezileştir |
| Cognition Devin → Sonnet 4.5 | Model değişimi | Eski model için prompt geçici çözümleri gürültüye dönüştü, tek tek yeniden ele alındı |
| **Decagon** (GEPA prod) | GEPA | Kısıtsız GEPA 5.000+ karakter prompt üretti; 500 örnek 50'den kötü ve 10× pahalı; küçük yansıtma modeli tamamen başarısız. Çözüm: 1.500 karakter uzunluk düzenleyici, sabit holdout |
| **Dropbox Dash** (DSPy) | Yüksek riskli promptlarda serbest yazım yerine **önceden onaylı talimat kütüphanesinden seçim** | Artımlı ve test edilebilir kalır; annotator uyuşmazlığı anahtar-kelime overfitting'e karşı sinyal |
| **Intercom Fin** | Tek metrik (çözüm oranı); haftalık öneriler, her öneri onu doğuran konuşmalara bağlı; kabul/yapıldı/ret | ~1 puan/ay kazanım |
| **Sierra ADLC** | Değişmez sürüm anlık görüntüsü (prompt+model+bilgi); annotasyonlar regresyon testi olur, platform yükseltmesinde her müşteri için yeniden koşar; Ghostwriter düzeltmeyi sandbox'ta doğrulayıp insan kuyruğuna koyar | Kurumsal ölçekte "kim, neyi, neden, hangi testler geçti, hangi konuşmalar bu sürümü kullandı" denetim şartı |
| **Dust / Cursor / Devin / Braintrust Loop** | Öneriler diff olarak; Onayla/Reddet; auto-approve opsiyonel + audit | Braintrust: "neredeyse her prompt değişikliği bazı girdileri iyileştirir, bazılarını geriletir" |
| LangSmith Align Evals / OpenAI optimizer | Önce LLM yargıcı insan notlarına kalibre et | OpenAI dataset-tabanlı optimizer Evals ile birlikte kapanıyor (2026-11): serbest oto-yazım tek başına ürün olarak tutmadı |
| Letta / Claude Code memory | Ajan-güdümlü bellek yazımı | "core_memory_append çağırmak döngüyü verimli hissettirir" (tik); bellek **tavsiyedir**, zorlama hook'ta yaşar |

### 3.3 Konsolide sorun listesi

1. **Metrik oyunu (Goodhart):** dedektörü kapatma, onay kapısını silme, ölçülmeyen özelliği kesme. Prompt-yasakları durdurmaz; yapısal kısıt durdurur.
2. **Gürültü kovalama ve osilasyon:** açgözlü kabul çok sayıda sahte kazanım işler (PACE: %72-100 sahte commit vs istatistik kapıyla <1/koşu).
3. **Küçük sete overfitting:** her kenar durumunu kodlayan promptlar; sıraya bağlı bellek kazanımları.
4. **Şişme ve okunamazlık:** 5k karakter prompt, 56k token sistem promptu, kırpılan bellek.
5. **Model sürümü regresyonu:** promptların ~%23'ü model güncellemesinde geriliyor; eski geçici çözümler gürültü.
6. **Sessiz davranış değişikliği / denetim boşluğu.**
7. **Tavsiye niteliğindeki bellek uygulanmıyor.**
8. **Çok-ajanlı atıf:** hangi ajanın kararı kötü sonucu doğurdu?
9. **Açık uçlu hedef ölçülemiyor:** bilgi işinde 5 dakikalık sinyal yok; yavaş proxy'ler gerçek hedefle yanlış korele.
10. **İnsan incelemesi darboğaz olur** — üretilen değişiklik inceleyiciyi geçer.
11. **Maliyet domine eder:** DGM $22k, tek eval $60, ADAS başabaş 15k sorgu.

## 4. Tasarım önerisi (beyin fırtınası)

İlke: **öner-onayla varsayılan, otomatik uygulama alan-bazlı opt-in, her kısıt Go kodunda, her değişiklik kanıtlı ve geri alınabilir.** Mevcut `recipe-optimizer` + `insight` hattını genelleştir; yeni bir motor yazma.

### 4.1 Goal varlığı

`store/goals/GOAL*.json`:

```jsonc
{
  "id": "GOAL3",
  "name": "Kod inceleme reçetesi ucuzlasın",
  "description": "…serbest metin; evolver'a bağlam…",
  "status": "active | paused | archived",
  "scope": { "recipes": ["code-review"], "tags": [], "agents": [] },
  "primary":   { "metric": "recipe.avgCostUSD", "direction": "min" },
  "guardrails": [
    { "metric": "recipe.successRate", "min": 0.9 },
    { "metric": "recipe.avgSessions", "max": 6 },
    { "metric": "agent.soulChars", "max": 4000 }
  ],
  "rubric": null,                    // açık uçlu hedef için düz-dilli rubrik (Outcomes deseni)
  "policy": {
    "mode": "propose | auto | off",
    "autoApplySurfaces": ["thinkingLevel", "toolOverrides:hide", "automation:cooldown"],
    "cooldownHours": 72, "minRuns": 5
  },
  "createdAt": "...", "updatedAt": "...", "history": [ /* alan değişiklikleri, kim/ne zaman */ ]
}
```

Metrik adları kapalı bir kataloğa bağlıdır (`internal/evolution/metrics.go`): reçete
istatistikleri, usage, pano sonuçları, otomasyon sayaçları, hata sınıfları, `Task.Rating`.
**Açık uçlu hedefler** için rubrik: ayrı bağlamda çalışan `outcome-judge` sistem ajanı
(araçsız, Anthropic Outcomes deseni) puan verir; kullanılmadan önce birkaç insan
puanıyla kalibre edilir (Align Evals). LLM yargıcı **asla tek başına** terfi kararı vermez.

### 4.2 Konfigürasyon anlık görüntüsü ve atıf

- `ConfigSnapshot` = ilgili yüzeylerin (ajan etkin alanları, tools-config, skill sürümleri,
  otomasyon/zamanlama alanları, prompt override hash'leri, ws-settings, **model sürümü**)
  kanonik hash'i + içerik. `store/evolution/snapshots/<hash>.json`, içerik-adresli.
- Her oturum başlangıcı / reçete koşusu / otomasyon ateşlemesi `snapshotHash` kaydeder
  (`SessionOrigin` yanına tek alan). Böylece fitness **sürüm başına** hesaplanır — bugün
  `RecipeStats.Version` için yapılan şeyin genel hali.
- Model sürümü snapshot'ın parçasıdır: sağlayıcı modeli değişince o snapshot'la ayarlanmış
  tüm override'lar "yeniden doğrulanacak" işaretlenir (Sierra/Cognition dersi).

### 4.3 Genel öneri şeması

`insight.Finding.Proposal`'ı genelleştir (`RecipeProposal` alt-türü kalır):

```go
type Proposal struct {
    Surface  string // "agent" | "skill" | "recipe" | "tools" | "automation" | "schedule" | "prompt" | "ws-settings"
    EntityID string // AGT12 | skill slug | AUT3 | prompt key
    Field    string // agent için 16 kalıtılabilir alandan biri; automation için CooldownSec…
    Action   string // set | prune | add | swap | rollback
    Value    any
    Removes  []string // büyüme bütçesini aşan eklemeler neyi kaldırdığını adlandırır
    GoalID   string
    Expected struct{ Metric string; Delta float64 } // beklenen etki, sonradan doğrulanır
}
```

Kanal: `evolution` (dördüncü kanal). Yaşam döngüsü, `Regressed`, `ErrAppliedNeedsEvidence`,
UI (FindingsTab/FindingModal) olduğu gibi kullanılır.

### 4.4 Evolver geçişi

- Sistem ajanı `workspace-evolver` (`recipe-optimizer` gibi kilitli, araçsız, JSON şema).
  Girdi: aktif hedefler, snapshot başına fitness tablosu (LLM'siz hesaplanmış), en kötü
  koşuların kısa özetleri, dersler, önceki önerilerin akıbeti, `IgnoredRecommendations`.
- Tetik: hedef başına `minRuns` yeni koşu **veya** guardrail ihlali **veya** elle
  **veya** haftalık cron (mevcut `insightcron` altyapısı).
- **Kodda uygulanan değişmezler** (recipe_optimizer deseninin genişletilmişi):
  - kanıt (oturum id + sayı) yoksa at; genel olumsuz yargı at;
  - hedef dışı yüzeye dokunan öneri at (scope);
  - **düzenlenemez alanlar**: hedefin kendisi, guardrail'ler, yargıç rubriği, hook'lar,
    `permissionMode`, `inboundPolicy`, kilitli sistem ajanları — evolver bunları **göremez
    bile** (autoresearch #322: editable/readonly/hidden ACL);
  - büyüme bütçeleri: soul karakter tavanı, ajan başına araç sayısı, skill sayısı,
    otomasyon sayısı — ekleme `Removes` adlandırmalı;
  - `IgnoredRecommendations` imzasıyla çakışan öneri at;
  - aynı imza 3 geçiş üst üste gelirse **insan tırmandırması**, yeniden önerme yok (Cekura).
- Öneri çeşitliliği: tek "en iyi" değil, hedef başına en fazla N öneri, farklı yüzeylerden
  (GEPA Pareto dersi, küçük ölçekte).

### 4.5 Uygulama, geri alma, defter

- `store/evolution/ledger.jsonl` **append-only**: `{ts, goalId, findingId, proposal, before,
  after, snapshotBefore, snapshotAfter, appliedBy: user|auto, evidence}`.
- Uygulayıcı yüzey başına Go fonksiyonu; `ApplyRecipeProposal` deseni: oku → değiştir →
  doğrula (prompts.Validate, RecipeSpec.Validate, tool tier geçerliliği, provider çifti) →
  yaz → yeniden oku. Ajan alanları için `derive`/override altyapısı; geri alma için
  `restore-default` ya da ledger'daki `before`.
- **Otomatik uygulama** yalnız hedef politikasındaki `autoApplySurfaces` için ve yalnız
  **tersinir + düşük riskli** sınıf: `thinkingLevel` düşürme, araç `hidden`/`summary`'ye alma,
  otomasyon cooldown/eşik artırma, skill `AutoSummary`. Prompt metni, model değişimi,
  hiyerarşi, hook, izin: **her zaman insan**.
- **Otomatik geri alma:** uygulanan değişiklikten sonra `minRuns` koşuda guardrail ihlali ya
  da primary metrik kötüleşmesi → ledger'dan `before` geri yüklenir, bulgu `Regressed`.
- Değişiklik başına **bekleme süresi** (dwell) ve hedef başına cooldown → thrash yok.

### 4.6 Deney (varyant) modu — sonraki faz

- Ajan için: `derive` ile çocuk (1-2 override), reçete profili/otomasyon hedefi bir süre
  çocuğa yönlendirilir (canary payı), iki kolun fitness'i snapshot atfıyla ayrışır.
- Terfi kapısı **deterministik**: her kolda n ≥ tabana, etki büyüklüğü eşiği, guardrail
  temiz; LLM karar vermez. Terfi = override'ları ebeveyne kopyala; ret = çocuğu sil.
- **Gerçekçi uyarı:** kişisel workspace'te n küçüktür; istatistik kapısı hiç geçmeyebilir
  (AgentSelfEdit: 4.150 çağrıda sıfır terfi). v1'de kural + insan incelemesi;
  istatistik kapısı yalnız yüksek hacimli reçeteler/otomasyonlar için.
- Model seçimi için bandit (ShinkaEvolve UCB1, RouteLLM): yalnız ödülün ucuz ve anında
  olduğu yerde (ör. titler/summarizer sistem ajanları, kısa görevler). Ana ajanlarda değil.

### 4.7 Ekran

- Yeni sayfa **Hedefler** (Ayarlar altında ya da nav): liste, düzenleme formu (metrik
  kataloğundan seçim, guardrail satırları, politika), durum çipleri.
- Hedef panosu: primary/guardrail trendi (snapshot sınırları dikey çizgi), bekleyen /
  uygulanan / reddedilen öneri sayıları, sürüm başına atıf tablosu.
- Öneri kartı = mevcut FindingModal + **diff görünümü** (before/after) + "bu öneriyi doğuran
  oturumlar" + beklenen etki + Uygula / Reddet (→ `IgnoredRecommendations`) / Ertele.
- Ledger sekmesi: kronolojik, her satırda **Geri al** düğmesi.
- Rota'da `o:optimizer` hayalet düğümü gerçek `workspace-evolver` geçişine bağlanır.
- Model sürümü değiştiğinde "yeniden doğrulanacak N override" uyarı bandı.

## 5. Faz planı (öneri)

| Faz | İçerik | LLM? | Bağımlılık |
|---|---|---|---|
| **E0** ✅ | `Goal` varlığı + CRUD API + Hedefler ekranı; metrik kataloğu; hedef başına LLM'siz fitness hesabı (view katmanı deseni, `_Docs/66`) | Hayır | — |
| **E1** ✅ | `ConfigSnapshot` + oturum/koşu/ateşleme atfı; sürüm başına fitness; model sürümü snapshot'ta izlenir ("yeniden doğrula" işareti E3'e kaydı) | Hayır | E0 |
| **E2** ✅ | Genel `Proposal` + `evolution` kanalı; `workspace-evolver` sistem ajanı, **yalnız öneri**; kodda değişmezler; FindingModal diff + kanıt | Evet (nadir) | E1 |
| **E3** | Yüzey başına uygulayıcılar + append-only ledger + Geri al; guardrail ihlalinde otomatik geri alma; `autoApplySurfaces` opt-in (yalnız tersinir sınıf) | Hayır | E2 |
| **E4** | Varyant deneyi (`derive` tabanlı canary) + deterministik terfi kapısı; yüksek hacimli reçeteler için | Hayır | E3 |
| **E5** | Açık uçlu hedefler için `outcome-judge` rubrik + insan kalibrasyonu; sistem ajanlarında model bandit'i | Evet | E2 |

E0–E1 tek başına değer üretir (hedef panosu + sürüm atfı); E2 mevcut `workspace-tuning`
lensinin hedef-bilinçli hali; E3 `AutoPrune`'un genellemesi.

## 8. E0 — gerçekleşen (2026-09-05) ve iki tasarım kararı

### 8.1 Uygulanan

- **Goal varlığı**: `internal/db/models_goal.go` + `store_goal.go` (`goals/GOL<n>.json`;
  `RawText` kullanıcının sözleri, düzenlemede yeniden yazılmaz; her değişiklik
  `History[]`'e `GoalRevision{at, by, note, fields}` ekler).
- **Alan paketi** `internal/goals`: kapalı metrik kataloğu (26 anahtar; reçete istatistikleri,
  usage, pano, otomasyon, oturum/insan yükü, `task.ratingAvg`, `judge.rubricScore`, config
  şişme guardrail'leri), tersinir oto-uygulama yüzeyi allowlist'i, `Normalize`/`Validate`/
  `CanActivate`, `Draft` şeması + `FromDraft`. Kurallar kodda: bilinmeyen metrik/yön, çift ya
  da sınırsız guardrail, güvensiz auto yüzeyi, açık soruyla etkinleştirme reddedilir; yazıcı
  `auto` politika koyamaz; sınırsız/katalog-dışı guardrail → açık soru.
- **`goal-writer` sistem ajanı** (🎯, araçsız, JSON şema; `internal/agent/goal_writer.go`,
  `prompts/defaults/goal-writer.md`). Girdi: sözler + katalog + kapsam adayları + mevcut
  hedefler (+ yeniden yazımda taban hedef). Çıktı `draft` olarak kaydedilir.
- **API** `GET /api/goals`, `GET /api/goals/catalog`, `POST /api/goals/intake`,
  `GET/PUT /api/goals/{id}`, `POST /api/goals/{id}/status`, `DELETE`. `POST /api/goals` yok:
  hedef yalnız yazıcıdan girer.
- **Ekran** `features/goals/` — liste + detay (açık sorular bandı, "Senin sözlerin", Ölçüm,
  Kapsam, Politika, Geçmiş), intake modalı, katalog-bağlı editör. Bölüm düzeni sonraki
  fazlar için yer bırakır: **fitness trendi** (E1), **öneriler** (E2), **evrim defteri /
  geri al** (E3) aynı başlık altına eklenir.

### 8.1b E1 — uygulanan (2026-09-05)

- `goals.ConfigSnapshot` (kanonik JSON → sha256[:8]; `Diff`), depo `evolution/snapshots/`
  (`index.json`'da `prev` kenarı ve `current`), `Session.SnapshotHash` damgası (kilit
  dışında; deadlock regresyon testi), runtime sağlayıcısı 2 sn önbellekli.
- `goals.Evaluate`: kapsam + pencere + ana metrik/guardrail + snapshot başına gruplama +
  sürümler arası fark. Ölçülebilen/ölçülemeyen metrikler katalogda `Available` ile ayrılır.
- API `GET /api/goals/{id}/fitness`, `GET /api/evolution/snapshots[/{hash}]`; ekranda
  **Ölçüm** bloğu. §8.3'teki ağacın düğümleri (snapshot) ve `prev` kenarları artık var;
  ledger kenarları (uygulanan öneri) E3'te gelir.

### 8.1c E2 — uygulanan (2026-09-06)

- `insight.EvolutionProposal` + `ChannelEvolution`; `goals.ProposalRules` kapalı yüzey/alan
  listesi ve `CheckProposal` (kanıt, kapsam, varlık, bütçe, beklenen etki yönü, yan etki →
  ret ya da `conflict`).
- `workspace-evolver` sistem ajanı; `SweepGoals`/`MaybeEvolveGoal` (minRuns, cooldown,
  guardrail ihlali) ve manuel `RunGoalEvolver`; `fileEvolverProposals` kap/ret/tırmandırma;
  `evolution/state.json`.
- API `POST /api/goals/{id}/evolve`, `GET /api/goals/{id}/evolution`; ekranda **Öneriler**
  bloğu ve İçgörü'de `evolution` kanalı. §4.4'teki değişmezlerin hepsi kodda ve testli;
  §4.5 (uygulama/geri alma) E3'te.

### 8.2 Karar: workspace'in tamamı değil, kısım kısım evrim

**Evrim birimi = hedefin kapsamı.** Bir hedef reçete/ajan/otomasyon/etiket kümesine
bağlıdır; evolver yalnız o kümenin yüzeylerine öneri üretir, fitness yalnız o kümenin
koşularından hesaplanır. Gerekçeler araştırmadan geliyor (§3):

1. **Kredi atfı**: workspace genelinde "ne değişti, hangi sonuç değişti" sorusu çok-ajanlıda
   çözülmemiş (MAS-PromptBench, Cognition). Kapsam daraldıkça atıf mümkün olur; tek seferde
   tek kapsam, tek yüzey.
2. **İstatistik gücü**: kişisel workspace'te n küçük. Reçete başına 5 koşu anlamlı bir
   sinyal olabilir; workspace toplamına karışınca gürültüye döner.
3. **Patlama yarıçapı**: yanlış bir değişiklik bir reçeteyi bozar, tüm workspace'i değil;
   geri alma da o kapsamda kalır.
4. **Çakışma yönetimi**: iki hedef aynı yüzeyi zıt yöne çekerse (maliyet ↓ vs kalite ↑)
   çatışma kapsam kesişiminde görünür ve §6/4 kuralı uygulanır (öneri iki hedefin
   guardrail'ini de geçmeli, geçemezse çatışma bulgusu).

Workspace-geneli hedef yine mümkündür (tüm listeler boş); ama evolver bunu "tüm yüzeyler
serbest" değil, **kapsam başına ayrı geçişler** olarak koşar: her reçete/ajan için ayrı
fitness, ayrı öneri, ayrı cooldown. Genel hedef bir şemsiye, uygulama yine parça parça.

### 8.3 Karar: evrim geçmişi git-ağacı gibi tek görünümde

Evet, mantıklı ve altyapı buna zaten yatkın. Tasarım:

- **Düğüm** = bir `ConfigSnapshot` (E1: kapsamdaki yüzeylerin içerik hash'i). **Kenar** =
  ledger kaydı (E3: `{goalId, proposal, before, after, appliedBy, evidence}`). Hedef
  revizyonları (E0, bugün var) aynı zaman çizgisinde "hedef değişti" işaretleri olarak durur;
  model sürümü değişimleri de birer işaret (§4.2).
- **Dallar** = kapsamlar (§8.2). Her reçete/ajan kapsamı kendi dalında ilerler; workspace
  görünümü tüm dalları tek tuvalde yan yana gösterir, Rota'nın şerit (lane) düzeni ve
  `rotaTimeScale` boşluk-kırpma mantığı doğrudan yeniden kullanılır. Bir dalın düğümüne
  tıklayınca: o snapshot'ta yüzeylerin değeri, before/after diff'i, kanıt oturumları, o
  sürümle koşan runs'ların fitness'i (E1 atfı).
- **Deney (E4)** = geçici yan dal: `derive` ile türetilen çocuk ajan kolu; terfi = ana dala
  birleşme, ret = dalın kapanması. Git'teki merge/abandon görselini birebir karşılar.
- **Geri alma** = daldan önceki düğüme "checkout": ledger'daki `before` geri yazılır, yeni
  bir düğüm olarak (tarih silinmez) eklenir; `Regressed` işareti düğümde rozet olur.
- Veri kaynağı **tek append-only ledger**; graf projeksiyonu LLM'siz view katmanında
  (`_Docs/66`) üretilir; snapshot içerik-adresli olduğu için aynı konfigürasyona dönüş aynı
  düğüme geri bağlanır (gerçek DAG, düz zaman çizgisi değil).

Bugün (E0) yalnız hedef revizyonları var; ekranın "Geçmiş" bölümü bu ağacın ilk, tek dallı
halidir. E1 snapshot'ı, E3 ledger'ı ekleyince görünüm ayrı bir **Evrim** sekmesine
(Hedefler ekranı içinde) taşınır.

## 6. Açık sorular

1. Goal workspace-başına mı, uygulama-geneli de olabilir mi (fleet)? İlk sürüm workspace.
2. Snapshot'a hangi yüzeyler girer? Fazla girerse her küçük değişiklik yeni sürüm → atıf
   seyrelir. Öneri: yalnız 16 kalıtılabilir alan + tools-config + reçete sürümü + otomasyon
   tetik alanları + prompt override hash'leri + model.
3. claude-cli / codex-cli yolunda bazı alanlar (thinking, araç allowlist) köprü üzerinden
   kısmen kontrol edilir; evolver hangi alanların o sağlayıcıda etkili olduğunu bilmeli
   (capability probe ile, `_Docs/54`).
4. Hedef çatışması: iki hedef aynı alanı zıt yöne çekerse? Öneri: öneri iki hedefin
   guardrail'ini de geçmeli; geçemiyorsa çatışma bulgusu üret, insan seçer.
5. `IgnoredRecommendations` imza biçimi genel `Proposal` ile hizalanmalı
   (`surface:entity:field:action`).

## 7. Kaynaklar

Akademik: GEPA https://arxiv.org/abs/2507.19457 · DSPy vaka https://arxiv.org/abs/2507.03620 ·
edit-düzeyi analiz https://arxiv.org/pdf/2605.26655 · Revisiting OPRO https://arxiv.org/abs/2405.10276 ·
DGM https://sakana.ai/dgm/ · AI Scientist https://arxiv.org/pdf/2408.06292 ·
AlphaEvolve https://deepmind.google/blog/alphaevolve-a-gemini-powered-coding-agent-for-designing-advanced-algorithms/ ·
ShinkaEvolve https://arxiv.org/pdf/2509.19349 · ADAS verimsizlik https://arxiv.org/html/2510.06711 ·
MASS https://arxiv.org/pdf/2502.02533 · MAS-PromptBench https://arxiv.org/pdf/2606.23664 ·
Optimas https://arxiv.org/abs/2507.03041 · Trace https://arxiv.org/html/2406.16218v2 ·
Agent-Lightning https://arxiv.org/html/2508.03680v1 · SICA https://arxiv.org/pdf/2504.15228 ·
ACE https://arxiv.org/html/2510.04618v1 · Dynamic Cheatsheet https://arxiv.org/abs/2504.07952 ·
RouterArena https://arxiv.org/html/2510.00202v1 · LLM-yargıç (PROCTOR) https://arxiv.org/html/2609.02246v1 ·
PACE https://arxiv.org/pdf/2606.08106.

Pratisyen/ürün: autoresearch https://github.com/karpathy/autoresearch ve
https://github.com/karpathy/autoresearch/discussions/322 · Langfuse
https://langfuse.com/blog/2026-03-24-optimizing-ai-skill-with-autoresearch · AgentSelfEdit
https://dev.to/debashish_ghosal/i-built-an-ai-that-rewrites-its-own-prompts-its-safety-gate-rejected-every-single-edit-220h ·
Ken Ashe https://kenashe.ai/blog/2026-08-19-the-self-improving-agent-demo-that-falls-apart-when-you-shuffle-the-tasks ·
Cekura https://www.cekura.ai/blogs/self-improving-voice-agents-closing-eval-loop · Warp
https://claude.com/blog/how-warp-builds-self-improving-agents-on-claude · Osmani
https://addyosmani.com/blog/self-improving-agents/ , https://addyo.substack.com/p/audit-your-agent-files ·
Anthropic Dreaming https://letsdatascience.com/blog/anthropic-dreaming-claude-managed-agents-self-improving-may-6 ·
Anthropic RSI https://www.anthropic.com/institute/recursive-self-improvement · Anthropic harness
https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents · Cognition
https://cognition.com/blog/dont-build-multi-agents , https://cognition.com/blog/devin-sonnet-4-5-lessons-and-challenges ·
Decagon https://decagon.ai/blog/optimizing-gepa-for-production , https://decagon.ai/blog/decagon-agent-versioning ·
Dropbox https://dropbox.tech/machine-learning/optimizing-dropbox-dash-relevance-judge-with-dspy ·
Intercom Fin https://www.intercom.com/help/en/articles/11390088-optimize-fin-instantly-with-the-help-of-ai ·
Sierra https://sierra.ai/blog/agent-development-life-cycle · Dust https://dust.tt/blog/the-continuous-improvement-loop ·
Braintrust https://www.braintrust.dev/articles/prompt-optimization-loop · Align Evals
https://venturebeat.com/ai/langchains-align-evals-closes-the-evaluator-trust-gap-with-prompt-level-calibration ·
promptim https://www.langchain.com/blog/promptim · Claude Code memory https://code.claude.com/docs/en/memory.
