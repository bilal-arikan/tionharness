---
name: "Generator↔Evaluator Loop (GAN)"
description: "Contract-driven generate→evaluate→refine loop with a separate skeptical evaluator, real Playwright testing, code-located bug reports, and a score-trend driven refine-or-pivot decision. Solves the self-evaluation problem (agents praising their own work)."
when_to_use: "When you want high-quality, verified output on an open-ended build/design task and want an independent agent to judge the work instead of the generator grading itself"
icon: "🧬"
color: "#8b5cf6"
access: shared
auto_summary: false
---
# Generator↔Evaluator Loop (GAN-benzeri) — Detaylı Kılavuz

Bu desen, Anthropic'in *harness design* makalesindeki **self-evaluation problemi**ne
çözümdür: ajanlar kendi ürettikleri işi değerlendirmeleri istendiğinde, kalite vasat olsa
bile **körü körüne överler**. Çözüm mimari ayrımdır — işi yapan ajanı (**generator**) işi
yargılayan ayrı, **şüpheci** ajandan (**evaluator**) tamamen ayırmak. GAN'lardaki
üretici↔ayırt-edici dinamiğine benzer bir geri-besleme döngüsü kurulur.

> Hazır flow: **"Generator↔Evaluator (GAN)"** şablonu (Akışlar ▸ Şablonlar) ya da market
> `flow.gan-generator-evaluator` paketi. Şüpheci persona: `agent.skeptical-evaluator`.
> Gerçek test için `mcp.playwright` MCP paketi.

## Döngünün şekli (graf)

```
contract(gen) → review-contract(eval) → generate(gen) → evaluate(eval) → decide(branch)
                                              ▲                               │
                                              │  REFINE (default)             │ SHIP → finalize(transform) → bitiş
                                              ├───────────────────────────────┤
                                              └── pivot(gen) ◄── PIVOT ────────┘
```

- **Döngü kasıtlıdır.** `evaluate → decide → generate` bir **geri-kenar (cycle)**'dır.
  SwarmGo orchestration motoru döngülere **izin verir** ve `maxSteps` (50) ile sınırlar —
  yani graf *acyclic olmak zorunda değildir*. İterasyon başına ~3 adım → ~15 iterasyon sert
  tavan; pratikte döngü evaluator SHIP dediğinde biter.

## İki ayrı ajan ata (kritik)

Desenin tüm değeri ayrımdadır. Şablonu kurduktan sonra:

- **Generator** rolü → `contract`, `generate`, `pivot` node'ları (üreten ajanın; coder/planner).
- **Evaluator** rolü → `review-contract`, `evaluate` node'ları (**ayrı, şüpheci** ajan;
  `agent.skeptical-evaluator` veya bir reviewer persona). Aynı ajanı iki role atamak deseni
  bozar — self-evaluation problemine geri dönersin.

## Sprint contract (sözleşme)

Uygulamadan **önce** generator ne inşa edeceğini ve başarının nasıl doğrulanacağını yazar;
evaluator bunu gözden geçirir ("doğru şeyi mi inşa ediyoruz?"). Sözleşme iki çekirdek bellek
bloğunda yaşar (yeni veri tipi gerekmez):

- `core:sprint-contract` — generator yazar (`core_memory_replace`), her turda bağlama enjekte
  edilir, evaluator okur.
- `core:sprint-scorelog` — evaluator her iterasyonda **ekler** (`core_memory_append`); skor
  **trend'i** buradan okunur (döngüde graf çıktıları üzerine yazıldığı için trend orada tutulamaz).

Sözleşme gövdesi (bloğa markdown/JSON olarak yazılır):

```json
{
  "deliverable": "Tek cümle: ne inşa edilecek",
  "successCriteria": [
    { "id": "C1", "text": "Ölçülebilir, test edilebilir kriter",
      "weight": "high|med|low", "verify": "Playwright: ... | API: ... | code: ..." }
  ],
  "gradingWeights": { "designQuality": 0.4, "originality": 0.3, "craft": 0.15, "functionality": 0.15 }
}
```

Score-log satır formatı (append-only):
```
ITER <n> | SCORE <passed>/<total> (<weightedPct>%) | TREND <up|flat|down> | VERDICT <SHIP|REFINE|PIVOT>
```

## Evaluator davranışı

- **Şüpheci ol:** işi sen yazmadın; öveni değil hatayı ara. Bir kriter ancak **gözlemlenebiliyorsa**
  PASS'tır; "kod doğru görünüyor" yetmez.
- **Gerçekten test et:** UI/E2E kriterleri için **Playwright MCP** araçlarıyla çalışan uygulamayı
  kullanıcı gibi tıkla (navigate/click/type/snapshot); API kriterleri için endpoint'i çağır;
  diğer durumda ilgili kod yolunu oku.
- **Kod-konumlu bug raporu:** her FAIL için `file:line` (veya fonksiyon), gözlenen vs beklenen,
  ve somut düzeltme.
- **Verdict üret:** yanıtın **son satırı** tam olarak `VERDICT: SHIP` / `VERDICT: REFINE` /
  `VERDICT: PIVOT` olmalı — `decide` branch'i buna göre yönlendirir.

## Skor → pivot mantığı (motor kısıtı)

Branch yalnız string eşleştirir (sayısal eşik yok). Bu yüzden **karar evaluator'ın
muhakemesinde** verilir ve tek satırlık verdict token'ı olarak dışa vurulur:

- Tüm high/med kriterler PASS → **SHIP**
- Skorlar yükseliyor, açıklar düzeltilebilir → **REFINE** (default arm → `generate`)
- Skorlar 2+ iterasyon düz/düşüş → **PIVOT** (yaklaşımı değiştir → `pivot` → `generate`)

`decide` branch'i `matchMode: regex` ile `(?m)^VERDICT:\s*SHIP` / `...PIVOT` desenlerini son
satıra demirler; eşleşmezse default = REFINE.

## Playwright bağlama (opsiyonel)

Evaluator'a gerçek tarayıcı testi kazandırmak için `mcp.playwright` paketini kur ve evaluator
ajanında `mcpEnabled: true` yap. Kalıcı MCP havuzu sayesinde tarayıcı oturumu iterasyonlar
arası korunur. UI/E2E olmayan hedeflerde (saf metin/kod) Playwright gereksizdir — sözleşmenin
`verify` alanı hangi doğrulamanın kullanılacağını belirler; evaluator kod okuma + `Bash` ile
test çalıştırmaya düşer.

## İpuçları

- Sözleşmeyi küçük, ölçülebilir kriterlere böl — belirsiz kriter = güvenilmez verdict.
- Generator her turda **önceki değerlendirmenin tüm FAIL'lerini** ele almalı.
- Bütçe guard'ları otonom koşularda geçerli; iterasyon sayısını hedefle orantılı tut.
- Bu desen `[[swarmgo-flows]]` üzerine kuruludur (flow node tipleri/şablon değişkenleri orada).
