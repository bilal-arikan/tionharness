# 87 — Karar Katmanı (Decider) ve Jev

> **Özet (2026-09-22):** Uygulamanın belirli karar noktalarında metin üretmeyen,
> yalnız tipli soruları (evet/hayır, birini seç, puanla) kalibre olasılıkla
> cevaplayan bir **karar modeli** kullanmasını sağlayan ayrı katman:
> `internal/decider`. İlk backend OpenRouter'ın alpha **Decisions API**'si
> üzerinden TypeSafe **Jev** (`typesafe/jev-1.13`); yeni karar modelleri chat
> sağlayıcılarına dokunmadan yeni bir backend dosyasıyla eklenir. Dört karar
> noktası (site) bağlandı: koordinatör takılma yargıcı, shell komutu risk
> kontrolü, flow `judge` eşleşme modu, Rota `judge` kapısı. Her site
> `off / shadow / on` modunda çalışır. Varsayılan olarak her şey kapalıdır (ana
> anahtar). Durum: **uygulandı (2026-09-22)**, ayar ekranı Ayarlar → Karar Modeli.

## 1. Neden ayrı bir katman

- **Chat provider değil.** `providers.Provider` arayüzü yalnız chat'tir
  (`Complete(ctx, Request) (*Response, error)`); bir kind'ın `Build`'i chat
  provider döndürmek zorundadır ve kind'ın `Manifest.Models` listesi her ajan
  model seçicisinde görünür. Jev ise `/chat/completions`'ı reddeder
  (`typesafe/jev-1.13 is a decisions model and cannot be used with the
  chat/completions endpoint`), metin ya da tool call üretmez.
- **Alternatifler eklenebilsin.** Katman bir backend kaydı üzerine kurulu
  (`decider.Register`, provider kind'larıyla aynı `init()` deseni). TypeSafe'in
  kendi API'si, başka bir Decisions sağlayıcısı ya da yerel bir sınıflandırıcı
  yeni bir `Backend` implementasyonu olarak eklenir; çağıran siteler değişmez.
- **Ölçmeden açma.** Her site önce `shadow` modunda koşup mevcut mantığın
  kararıyla yan yana kaydedilebilir; uyum oranı görülmeden davranış değişmez.

## 2. Jev ve OpenRouter Decisions API

Kaynaklar: TypeSafe blog (System One models), OpenRouter Go SDK `Alpha.Decisions`,
`OpenRouterTeam/ai-sdk-provider#562`, bağımsız test harness'leri. **Canlı doğrulama
2026-09-21** (gerçek anahtar, 9 çağrı):

```
POST https://openrouter.ai/api/alpha/decisions      Authorization: Bearer <OpenRouter key>
{"model":"typesafe/jev-1.13","state":<metin | JSON>,"questions":{
  "k1":{"type":"noul","instructions":"…","criteria":{"true":"…","false":"…"}},
  "k2":{"type":"choice","instructions":"…","criteria":{"a":"…","b":"…"}},
  "k3":{"type":"score","instructions":"…","criteria":["düşük","orta","yüksek"]}}}
→ {"id":"gen-dec-…","model":"typesafe/jev-1.13-20260917","provider":"TypeSafe",
   "answers":{"k1":{"type":"noul","noul":0.96},
              "k2":{"type":"choice","choice":"a","probabilities":{…},"confidence":0.66},
              "k3":{"type":"score","score":2,"legend":{…},"probabilities":{…},"confidence":1}},
   "usage":{"input_tokens":606,"output_tokens":92,"cost":2.5452e-05}}
```

| Gözlem | Sonuç |
|---|---|
| 4 soruluk istek | 0,80 sn, $0.0000255 (yalnız girdi: 606 × $0.042/M) |
| Tek soruluk istek ×5 | 0,30–0,71 sn, medyan 0,35 sn |
| `~typesafe/jev-latest` | Aynı snapshot'a gider: `typesafe/jev-1.13-20260917` |
| noul `criteria` tek taraflı | **400** (`criteria.false` zorunlu); hiç verilmezse geçerli |
| `/chat/completions` | **400**, yukarıdaki mesaj |
| `/api/v1/models` | Jev **listelenmiyor** (443 model) — id'ler elle tanımlı |
| Hata gövdesi | `{"error":{"message","code"}}`; doğrulama mesajı string'lenmiş zod JSON'u |

Bilinen sınırlar: alpha uç nokta (şekil değişebilir), bağlam OpenRouter'da 32k,
metin/tool call/görsel yok, test-time reasoning yok (akıl yürütmesiz LLM düzeyi),
topluluk raporlarında çağrıların bir kısmı read timeout'ta asılı kalıyor.

## 3. Paket haritası

| Yer | Görev |
|---|---|
| `internal/decider/types.go` | `Request`/`Question`/`Answer`/`Response`/`Usage`, `Decider` arayüzü; `Answer.Yes/Level/Strength` |
| `internal/decider/question.go` | `Noul`/`Choice`/`Score` kurucuları, `Validate`, `Normalized` (tek taraflı noul kriterini tamamlar) |
| `internal/decider/backend.go` | Backend kaydı: `Register`, `Lookup`, `Manifests`, `IsDecisionModel`; `Endpoint` (anahtar yerine `Authorize` fonksiyonu) |
| `internal/decider/openrouter.go` | OpenRouter Decisions backend'i; `DecisionsURL` (`…/api/v1` → `…/api/alpha/decisions`, `/v1` ile bitmeyen tabanı reddeder) |
| `internal/decider/http.go` | Deneme başına zaman aşımı + tek hızlı retry (408/429/5xx/524/529, 200 ms), yanıt 1 MB sınırı, `"<ad> HTTP <kod>: …"` hata biçimi |
| `internal/decider/config.go`, `store.go` | `Config`/`Site`/`Mode`; `<dataDir>/decider.json` (settings.json'dan ayrı, atomik yazım) |
| `internal/decider/hub.go` | Uygulama çapında servis: istemci önbelleği (provider `Generation`'ına bağlı), otomatik hesap seçimi, circuit breaker, anahtar karantinası, state hazırlığı |
| `internal/decider/ledger.go` | `<dataDir>/decider/ledger.jsonl` (4 MB'ta döner, bellekte son 5000 kayıt), site başına istatistik |
| `internal/decider/redact.go` | Gönderilen state'teki sırların maskelenmesi, `TrimMiddle` |
| `internal/providers/registry_http.go` | `Registry.HTTPAccess` — anahtarı string olarak dışarı vermeden `Authorize` kapanışı; `InstanceBaseURL` |
| `internal/agent/decide*.go`, `flow_judge.go`, `trajectory_gate_judge.go` | Site entegrasyonları, usage kaydı, shadow arka plan çağrıları |
| `internal/orchestration/judge.go` | `MatchJudge`, opsiyonel `JudgeRunner` arayüzü |
| `internal/api/decider*.go` | `/api/decider` uç noktaları, kayıt defteri adaptörü, chat modeli koruması, claude-cli izin yolu |
| `frontend/src/features/decider/` | Ayarlar → Karar Modeli ekranı (i18n namespace `decider`, `I18N_MIGRATED`'da) |

`internal/decider` hiçbir internal paketi import etmez; `scripts/depcheck.sh`
`internal/agent`'ı import etmediğini doğrular.

## 4. Yapılandırma

`<dataDir>/decider.json` (API: `GET/PUT /api/decider`):

| Alan | Varsayılan | Anlam |
|---|---|---|
| `enabled` | `false` | Ana anahtar. Kapalıyken hiçbir yere bir şey gönderilmez. |
| `backend` | `openrouter` | Kayıtlı backend id'si |
| `providerInstanceId` | `""` | Anahtarı ödünç alınan sağlayıcı örneği; boş = backend'in kabul ettiği ilk etkin/kullanılabilir örnek (`openrouter` kind'ı, sonra openrouter.ai'ye bakan `openai-compat`) |
| `model` | `typesafe/jev-1.13` | Sabit snapshot; `~typesafe/jev-latest` kayan takma addır (gölge ölçümlerini karşılaştırılamaz kılar) |
| `timeoutMs` | `3000` | Deneme başına; 500–15000 arasında kıstırılır |
| `sites.<id>` | site varsayılanı | `mode` + `threshold` (0,50–0,99) |

Anahtar burada **saklanmaz**; seçilen örneğin şifreli anahtarı kullanılır. Market'teki
"openrouter" paketi `openai-compat` olarak kurulduğu için o örnekler de adaydır.

## 5. Karar noktaları (siteler)

| Site | Varsayılan mod | Varsayılan eşik | Soru | Cevap veremezse |
|---|---|---|---|---|
| `stall-judge` | shadow | 0,70 | noul `stalled` — stall-judge prompt'unun birebir karşılığı | LLM yargıcına döner |
| `tool-risk` | shadow | 0,80 | noul `needs_approval` + score `risk` (3 seviye) | Komut eskisi gibi çalışır |
| `flow-judge` | on (açık site) | 0,60 | choice `arm` (dallar) / noul `holds` (döngü) | Varsayılan dal; döngüde sınıra kadar devam, sınır yoksa hata |
| `phase-gate` | on (açık site) | 0,80 | noul `holds` — kök oturumun son 40 mesajı | **Kapı kapalı kalır** (fail-closed) |

- **stall-judge** (`decide_stall.go`): `on` modunda `judgeCoordinatorStalledUncached`
  Haiku çağrısı yerine karar modelini sorar; `shadow` modunda LLM kararından sonra
  arka planda sorar ve ikisini kaydeder. Test dikişi `stallJudgeFn` önceliklidir.
- **tool-risk** (`decide_toolrisk.go`, `permission.go`): insan kararı olmadan
  çalışacak exec çağrılarına ikinci bakış — auto moddaki her exec çağrısı ve ask
  modda bir aile kuralına (`Bash(git *)`) takılan çağrılar. Salt-okunur komutlar
  (`git status/diff/log`, `ls`, `go test` …, `decide_toolrisk_readonly.go`) hiç
  sorulmaz. `on` modunda onay gerekir denirse çağrı **izin istemine** döner (kart
  etiketi `exec:decider`: "riskli komut — karar modeli onay önerdi"). Soracak kimse
  yoksa (otonom tur) **asla bloklamaz**, yalnız arka planda ölçer. İşaretlenmiş bir
  çağrıya "Her zaman izin ver" o **tam komutu** onaylar (`tools.ExactGrantRule`),
  aile kuralı yazmaz. claude-cli ask modundaki izin-prompt aracı da aynı kontrolü
  kullanır (`api/decider_cli_permission.go`).
- **flow-judge** (`orchestration/judge.go`, `agent/flow_judge.go`): dallanma
  düğümünde `matchMode: "judge"` — her dalın `contains` metni bir seçeneği tarif
  eder; döngüde `untilMode: "judge"` — `until` düz cümleli koşuldur. İsteğe bağlı
  `judgeQuestion`. İz etiketi `"<dal> (judge 0.93)"` / `"default (judge unsure, 0.41)"`.
  Faturayı çıktısı yargılanan ajan öder.
- **phase-gate** (`trajectory_gate_judge.go`): kapı türü `judge`, değeri fazın
  çıkış koşulu. Doğrulayıcının kod bloğu içindeki ya da farklı sözcüklerle yazılmış
  onayını da tanır; şablon yankısı `VERDICT: PASS | FAIL` kapıyı açmaz.

## 6. Ledger, istatistik ve shadow → on geçişi

Her karar bir `Record` olarak yazılır: site, mod, model, gecikme, girdi token'ı,
maliyet, karar (`Outcome`) ve gücü, mevcut mantığın kararı (`Baseline`),
uygulanıp uygulanmadığı, kısa hata sınıfı (`timeout`, `http_401` …; ham metin
**asla**). Yargılanan içerik ledger'a yazılmaz. `GET /api/decider/stats?days=N` site
başına çağrı, hata, karşılaştırma, uyum, uygulanan, p50/p95 ve maliyet verir.

Ekran rehberi (`frontend/src/features/decider/deciderModel.ts`): en az **50**
karşılaştırmadan sonra uyum **≥ %90** ise "açmaya hazır", **< %80** ise "gölgede
tut". Açık siteler (flow-judge, phase-gate) kullanıcı zaten açıkça istediği için
gölge modu sunmaz.

## 7. Güvenlik, gizlilik, hata davranışı

- Anahtar `providers.HTTPAccess` içinde bir kapanışta kalır; katmana string olarak
  hiç girmez, log/hata mesajına düşemez.
- Gönderilen her state maskelenir (`sk-…`, `ghp_…`, `github_pat_…`, `AKIA…`,
  `AIza…`, Slack token'ları, `Bearer …`, `password/token/secret/api_key =…`,
  private key blokları, URL içi parolalar). Metin state bağlam bütçesine göre
  ortadan kırpılır; sığmayan yapılandırılmış state gönderilmez.
- 3 ardışık geçici hata → 60 sn circuit (siteler anında eski mantığa döner);
  401/402/403 → örnek 10 dk karantina ya da sağlayıcılar yeniden kaydedilene kadar.
  400/404/413 isteğe özgüdür, circuit'i tetiklemez.
- Shadow çağrıları çağıranı hiç bekletmez; `startBackgroundTurn` bariyeri altında
  koşar, workspace kapanışı onları bekler.

## 8. Maliyet ve faturalama

- Kullanım yeni `decide` çağrı türüyle (`db.UsageKindDecide`) **çağıran ajana**
  yazılır, sağlayıcı olarak backend'in faturalama sağlayıcısı (`openrouter`) ile.
- Fiyat tablosu `providers/pricing.go`: `typesafe/jev-1.13`, `~typesafe/jev-latest` =
  girdi $0.042/M, çıktı $0. Kayıt **istenen** model id'siyle yapılır; sunulan
  snapshot (`…-20260917`) tabloda yoktur.
- Ajansız çağrılar (claude-cli izin prompt'u) ajan bütçesine düşmez, ledger'da
  maliyetleriyle görünür. Bütçe ekranında tür etiketi "Karar".

## 9. Chat modeli koruması

`decider.IsDecisionModel` (manifest model id'leri + `typesafe/`, `~typesafe/`
önekleri) ajan create/update'te `model` alanını reddeder: "… is a decision model
… set it under Settings → Decision model instead". Jev hiçbir provider
manifest'ine konmadı, dolayısıyla hiçbir chat seçicide görünmez.

## 10. Yeni backend ekleme (alternatif karar modeli)

1. `internal/decider/<ad>.go`: `Backend` arayüzü — `Manifest()` (id, etiket,
   `ProviderKinds`, `BillingProvider`, `Models`, `DefaultModel`, `ContextTokens`,
   `ModelPrefixes`), `Accepts(kind, baseURL)`, `New(Endpoint, ClientOptions)`.
2. `init()` içinde `Register(...)`.
3. Yanıtı nötr `Answer` tiplerine çevir; istenmiş her soru için cevap yoksa hata.
4. `providers/pricing.go`'ya faturalama sağlayıcısı altında fiyat satırları.
5. `httptest` ile istek/yanıt şekli testi (bkz. `openrouter_test.go`).

Kendi anahtarını isteyen bir backend (ör. TypeSafe'in yerel API'si) için önce bir
provider kind'ı gerekir (anahtar oradan `HTTPAccess` ile ödünç alınır).

## 11. Yeni site ekleme

1. `config.go` `sites` listesine `Site` (id, modlar, varsayılan mod/eşik, açıklama).
2. `internal/agent/decide_<site>.go`: istek kurucu + `r.decide` / `r.shadowDecision`
   / `r.backgroundDecision` çağrıları, `decider.NewRecord` ile ledger kaydı.
3. Hata hâlinde sitenin eski mantığına dön (fail-open) ya da güvenlik kapısıysa
   kapalı kal (fail-closed) — hangisi olduğunu dokümana yaz.
4. `frontend/src/i18n/locales/{en,tr}/decider.json` → `site.<id>.*` metinleri.

## 12. Sonraki adaylar

Araştırmada belirlenen ama bu fazda bağlanmayan noktalar: auto-continue "iş bitti mi?",
lesson reflect ön-kapısı ve tur başı lesson alaka seçimi, worker brief "yazma
gerekir mi?" regex'lerinin yerine geçmek, fan-out `reviewer-selects`, insight
scanner ön elemesi, skill önerisi, kullanıcı tanımlı `decision` hook tipi, spawn
anında model kademe seçimi, WebFetch/MCP sonuçlarında prompt-injection taraması.

## 13. Test haritası

- `internal/decider/*_test.go`: istek/yanıt şekli (canlı yanıttan türetilmiş gövde),
  retry/timeout, `DecisionsURL`, `Accepts`, `IsDecisionModel`, config
  normalize/validate/round-trip, ledger istatistikleri + yeniden yükleme, hub
  (anahtarlar, otomatik hesap, önbellek, karantina, circuit, redaksiyon/bütçe,
  kalıcılık), redaksiyon kalıpları.
- `internal/agent/decide*_test.go`: faturalama (`openrouter|typesafe/jev-1.13`,
  `decide` türü), stall-judge on/shadow/fallback, salt-okunur komut tablosu,
  tool-risk (auto, ask + aile kuralı, gözetimsiz tur, shadow/off, tam komut onayı),
  flow judge dal/koşul, phase-gate judge (geçer/kalır/kapalı kalır).
- `internal/orchestration/judge_test.go`, `internal/api/decider_test.go`,
  `internal/providers/registry_http_test.go`, `internal/tools/grants_exact_test.go`.
- Frontend: `features/decider/deciderModel.test.ts`, `features/flows/judgeLabel.test.ts`,
  i18n katalog paritesi.
