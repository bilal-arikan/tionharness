# 87 — Karar Katmanı (Decider), Karar Modelleri ve Karar Mercileri

> **Özet (2026-10-03):** Uygulamanın belirli noktalarında metin üretmeyen, yalnız
> tipli soruları (evet/hayır, birini seç, puanla) olasılıkla cevaplayan **karar
> modelleri** kullanmasını sağlayan ayrı katman: `internal/decider`. **Karar
> modelleri** sağlayıcı örnekleri gibi eklenip düzenlenir (kendi uç noktası +
> şifreli anahtar ya da bir sağlayıcı hesabını ödünç alma); üç backend var:
> OpenRouter Decisions (Jev), System One API (TypeSafe · OpenJev) ve logprobs
> üzerinden **herhangi bir yerel LLM** (Ollama, LM Studio, llama.cpp, vLLM).
> **Karar mercileri** (`Authority`) kararı modele devreden noktalardır; kendi
> paketlerinden kayıt olur ve her biri `off / shadow / on`, eşik, kendi modeli,
> **yedek** ve **rakip** model alır. On üç merci bağlıdır: tool-risk, stall-judge,
> flow-judge, flow-criteria, flow-grade, flow-proposal-gate ([93 §8](93-EVRILEN-AKISLAR.md)),
> phase-gate ve [JEV iş akışlarındaki](92-JEV-IS-AKISLARI.md) altı
> oturum/bağlam/işbirliği noktası. Ortak `Pick` yardımcısı ve merciye özel soru
> kurucuları kullanılır. Durum: **uygulandı**; karar sağlayıcıları Ayarlar →
> Sağlayıcılar, merciler Ayarlar → Karar Mercileri.

## 1. Neden ayrı bir katman

- **Chat provider değil.** `providers.Provider` yalnız chat'tir; bir kind'ın
  `Manifest.Models` listesi her ajan model seçicisinde görünür. Jev ise
  `/chat/completions`'ı reddeder, metin ya da tool call üretmez. Karar modelleri
  bu yüzden kendi kayıt defterinde tutulur, hiçbir chat seçicide görünmez.
- **Birden çok model, birden çok merci.** Aynı anda hem barındırılan Jev hem
  yerel bir OpenJev ya da küçük bir Ollama modeli tanımlanabilir; her karar
  mercii hangisinin cevap vereceğini seçer.
- **Ölçmeden açma, ölçmeden değiştirme.** Merci önce `shadow` modunda mevcut
  mantığın yanında ölçülür; yeni bir model önce **rakip** olarak aynı soruları
  arka planda cevaplar, uyumu görülmeden devralmaz.

## 2. Kavramlar

| Kavram | Kod | Ne |
|---|---|---|
| Backend | `decider.Backend`, `Register` | Bir karar API'si ailesi (wire formatı, URL türetme, faturalama). `init()` ile kayıt olur. |
| Karar modeli | `ModelInstance`, `ModelStore` | Kullanıcının eklediği giriş: backend + uç nokta + kimlik bilgisi + model id + zaman aşımı + bağlam + backend alanları. `DM<n>` id'si. |
| Karar mercii | `Authority`, `RegisterAuthority` | Kararı modele devreden nokta. Grup, desen, modlar, varsayılan eşik, `Explicit`, `FailClosed`. |
| Merci ayarı | `AuthorityConfig` | `mode`, `threshold`, `model` (boş = varsayılan), `fallback`, `challenger`. |
| Hub | `decider.Hub` | Uygulama çapında servis: model başına istemci önbelleği ve sağlık, fallback, challenger, ledger. |
| Desen | `Pick`, `Hub.Decide` ve merciye özel soru kurucuları | Ortak tek-seçim yorumlayıcısı; çoklu seçim/sınıflandırma girdisini ilgili çalışma noktası hazırlar. |

## 3. Backend'ler

### 3.1 OpenRouter Decisions (`openrouter`)

`POST {base}/../alpha/decisions` (`…/api/v1` → `…/api/alpha/decisions`), TypeSafe
Jev. Canlı doğrulama 2026-09-21 (9 çağrı):

```
{"model":"typesafe/jev-1.13","state":<metin | JSON>,"questions":{
  "k1":{"type":"noul","instructions":"…","criteria":{"true":"…","false":"…"}},
  "k2":{"type":"choice","instructions":"…","criteria":{"a":"…","b":"…"}},
  "k3":{"type":"score","instructions":"…","criteria":["düşük","orta","yüksek"]}}}
→ {"id":"gen-dec-…","model":"typesafe/jev-1.13-20260917","provider":"TypeSafe",
   "answers":{"k1":{"type":"noul","noul":0.96}, "k2":{"choice":"a","probabilities":{…},"confidence":0.66},
              "k3":{"score":2,"legend":{…},"probabilities":{…},"confidence":1}},
   "usage":{"input_tokens":606,"output_tokens":92,"cost":2.5452e-05}}
```

| Gözlem | Sonuç |
|---|---|
| 4 soruluk istek | 0,80 sn, $0.0000255 (yalnız girdi: 606 × $0.042/M) |
| Tek soru ×5 | 0,30–0,71 sn, medyan 0,35 sn |
| noul `criteria` tek taraflı | **400**; `Request.Normalized` eksik tarafı doldurur |
| `/chat/completions`, `/api/v1/models` | Jev reddedilir / listelenmez; id'ler elle tanımlı |

Her çağrı `openrouter` altında faturalanır (Decisions API yalnız orada vardır).

### 3.2 System One API (`systemone`) — TypeSafe ve OpenJev

TypeSafe'in kendi API'si `POST https://api.typesafe.ai/v1/systemone`; aynı wire
formatını OpenRouter `…/api/v1/systemone` altında ve açık **OpenJev** sunucuları da
konuşur (TypeSafe SDK'ları onlara değişmeden bağlanır). Tek backend hepsini kapsar;
fark yalnız taban URL, anahtar ve model id'sidir. URL kuralı: `…/v1` → `+/systemone`,
tam `…/systemone` olduğu gibi, sürümsüz taban (`https://api.typesafe.ai`) →
`+/v1/systemone`. Cevap çözümü toleranslıdır: `type` alanı yoksa da, score dağılımı
dizi (`[0.1,0.2,0.7]`) gelse de okunur.

Araştırma (2026-09-22) — OpenJev ailesi:

| Proje | Nasıl | Arayüz |
|---|---|---|
| `razorback16/openjev` | DiffusionGemma 26B, vLLM (NVIDIA ≥24 GB) ya da MLX | `POST /v1/systemone`, port 8080, model `openjev-latest` (`jev-latest` takma adı da) |
| `openjev/openjev` (HF) | 27B, vLLM + karar shim'i | `POST /v1/systemone`; ≤52 seçenek, 16k prompt, H100'de ~80–210 ms |
| `zefan-cai/open-jev` | Qwen tabanlı 2B/9B/27B | `POST /v1/systemone` |
| `zhihz/openjev`, OpenJevPro, poorjev, von, laya … | Kendi formatları / kütüphane | Doğrudan desteklenmez; logprobs backend'i aynı işi görür |

Faturalama host'a göre: loopback / özel ağ / `.local` → `local` (bilinen sıfır),
`openrouter.ai` → `openrouter` (çıplak id `typesafe/` önekiyle), `api.typesafe.ai` →
`typesafe`. Limitler: seçenek ≤52, score seviyesi ≤10.

### 3.3 Herhangi bir LLM — logprobs (`llm-logprobs`)

Sıradan bir sohbet modelini karar modeline çevirir. Her soru tek etiketli çoktan
seçmeli olarak sorulur (noul: `A) Yes / B) No`, choice: anahtara göre sıralı `A…Z,
a…z`, score: `0…9`), `logprobs: true, top_logprobs: 20` ile OpenAI uyumlu
`/chat/completions`'a gider; cevap pozisyonundaki alternatiflerin olasılıkları
etiketler üzerinde normalize edilir (`" A"` ile `"A"` aynı cevaptır). Boş
`<think></think>` bloğu, baştaki boşluk ya da `Answer:` öneki atlanır; açık bir
düşünme bloğu içindeki harfler cevap sayılmaz. Sorular paralel koşar (varsayılan 4).

- Destek: Ollama ≥0.12.11, LM Studio ≥0.3.39, llama.cpp server, vLLM, OpenRouter
  (logprobs veren modeller).
- Logprobs dönmezse metinden okunur, cevap kesin 0/1 olur ve `warnings`'e yazılır.
- Backend alanları: `suffix` (Qwen3 için `/no_think`), `extraBody` (JSON, ör.
  `{"chat_template_kwargs":{"enable_thinking":false}}`), `topLogprobs`, `maxTokens`
  (varsayılan 8), `parallel`.
- `DecisionOnly=false`: bu modeller chat için de kullanılabilir; chat koruması onları
  reddetmez. `Calibrated=false`: arayüz "yaklaşık olasılıklar" rozeti gösterir.
- **Canlı doğrulama 2026-09-22**: LM Studio + Qwen3-8B (`/no_think`),
  `git push --force` → P(onay)=1,00 · risk 2 (800 ms); `git status` → 0,00 · risk 0
  (419 ms). Ham cevapta `B 0,99996 / A 0,00004` — model gerçekten emin; kalibrasyon
  Jev düzeyinde değildir.

### 3.4 Ödünç alınan uç nokta kuralı

Bir model sağlayıcı hesabını ödünç alırsa anahtar yalnız o sağlayıcının kendi uç
noktasına gider: formda taban URL alanı gizlenir ve sunucu da temizler. Sağlayıcı
örneği taban URL'yi boş bıraktıysa kind'ın varsayılanı kullanılır (`openrouter` →
`https://openrouter.ai/api/v1`, `lmstudio` → `:1234`), **backend varsayılanı asla**:
aksi hâlde boş alanlı bir OpenRouter hesabının anahtarı TypeSafe'e giderdi
(`endpoint_base.go`, test `TestBorrowedEndpointNeverFallsBackToTheBackendDefault`).

## 4. Karar modelleri (kayıt defteri)

- Dosya `<dataDir>/decider/models.json`; kendi anahtarı sağlayıcı sırlarıyla aynı
  şifreleyiciyle (`ProviderStore.Cipher()`) saklanır, API yalnız `secretsSet` döner.
  Sır kuralı sağlayıcılarla aynı: alan yok = koru, boş = sil, değer = değiştir.
- Bozuk dosya `models.json.corrupt-<unix>` olarak kenara alınır, açılış engellenmez.
- İlk çalıştırmada (dosya yokken) bir model tohumlanır: eski v1 `decider.json`'daki
  bağlantı (backend, sağlayıcı örneği, model, timeout) ya da hiç yapılandırma yoksa
  "Jev · OpenRouter" (ilk OpenRouter hesabını otomatik ödünç alır). Silinmiş bir
  liste yeniden tohumlanmaz.
- Doğrulama (`normalizeModelInput`): backend kayıtlı olmalı ve düzenlemede
  değişemez; `KeyRequired` backend kendi uç noktasında anahtar ister; bilinmeyen
  alan/sır reddedilir; timeout 500–60000 ms, bağlam 1024–1.000.000 token
  (0 = backend'in); backend alanları `ValidateConfig` ile denetlenir.
- Hazır şablonlar (manifest `Presets`): Jev · OpenRouter, Jev · TypeSafe,
  Jev · OpenRouter (System One), OpenJev · bu makine, Ollama · bu makine,
  LM Studio · bu makine.

API: `POST /api/decider/models`, `PUT|DELETE /api/decider/models/{id}`,
`POST /api/decider/models/{id}/test` (kapalı modeli de dener, faturalamaz).
Her değişiklik tüm görünümü döner; silme, modele bağlı olanları (`usedBy`:
`default` ve merci id'leri) raporlar ve referansları temizler.

## 5. Karar mercileri

### 5.1 Kayıt ve ayarlar

Merci kendi paketinden kayıt olur (`decider.RegisterAuthority` bir `init()` içinde;
bkz. `internal/agent/decide_authorities.go`). Descriptor: `ID`, `Group`
(`safety`, `coordination`, `flows`, `routing`, `context`, `housekeeping`),
`Pattern` (`gate`, `pick`, `rate`, `select`, `triage`), `Modes`, `DefaultMode`,
`DefaultThreshold` (0,50–0,99), `Explicit` (gölge modu yok), `FailClosed`.
Hatalı descriptor `init()`'te panikler.

`<dataDir>/decider.json` (sürüm 2):

| Alan | Varsayılan | Anlam |
|---|---|---|
| `enabled` | `false` | Ana anahtar; kapalıyken hiçbir yere bir şey gönderilmez |
| `defaultModel` | `""` | Kendi modelini seçmemiş mercilere cevap veren model; boş = ilk etkin model |
| `authorities.<id>.mode/threshold` | merci varsayılanı | `off / shadow / on`, eşik |
| `authorities.<id>.model` | `""` | Bu merciin modeli; boş = varsayılan |
| `authorities.<id>.fallback` | `""` | Model cevap veremezse (ulaşılamaz, circuit/karantina, timeout, 5xx, 401) sorulan model |
| `authorities.<id>.challenger` | `""` | Her cevaptan sonra aynı soruyu arka planda cevaplayan rakip |

Sürüm 1 dosyası (`backend/providerInstanceId/model/timeoutMs/sites`) açılışta
dönüştürülür: `sites` → `authorities`, bağlantı → `DM1`; dosya sürüm 2 olarak
yeniden yazılır. Kayıtlı olmayan merciin ayarı düşer; olmayan modele referans
temizlenir.

### 5.2 Çağrı akışı (`Hub.Decide`)

1. Ana anahtar kapalı → `ErrDisabled`; merci `off` → `ErrSiteOff` (sessiz).
2. Etkin model: merciinki → varsayılan → ilk etkin model. Yoksa `ErrNoModel`.
3. `ask`: model kapalı mı, genel doğrulama (`ErrInvalidRequest`), model sağlığı,
   istemci (önbellek anahtarı: modelin revizyonu + ödünç sağlayıcı + sağlayıcı
   `Generation`'ı), backend limitleri, redaksiyon + bağlam bütçesi, çağrı.
4. Hata ve `fallback` varsa ve hata "başka model deneyebilir" türündeyse
   (geçersiz istek ve iptal hariç) yedek sorulur; cevap `Fallback=true` taşır.
   İkisi de düşerse iki hata birleşik döner.
5. Cevaptan sonra `challenger` varsa arka planda sorulur (en çok 4 eşzamanlı; yer
   yoksa atlanır, kuyruğa girmez; 30 sn sınır). Ledger'a `role: challenger` kaydı:
   `Outcome` = rakibin kararı, `Baseline` = cevap veren modelinki. Karar sözlüğü
   `WithOutcome` ile merciinkidir ("stalled"/"ok"), verilmezse genel `Verdict`
   (`yes/no`, seçenek, `L<seviye>`).
6. Faturalama `WithBilling` ile çağırana: birincil, yedek ve rakip çağrılarının
   hepsi. `WithBackground` rakibi çağıranın yaşam döngüsünde koşturur (agent'ta
   `startBackgroundTurn` bariyeri).

Sağlık **model başına**: 3 ardışık geçici hata → 60 sn circuit; 401/402/403 →
10 dk karantina (ödünç anahtarda sağlayıcılar yeniden kaydedilince, kendi
anahtarında model güncellenince kalkar). Bir modelin dinlenmesi diğerini
etkilemez: yerel sunucu kapalıyken barındırılan yedek çalışmaya devam eder.

### 5.3 Bağlı merciler

| Merci | Grup · desen | Varsayılan | Soru | Cevap yoksa |
|---|---|---|---|---|
| `tool-risk` | safety · gate | shadow, 0,80 | noul `needs_approval` + score `risk` | Komut eskisi gibi çalışır |
| `stall-judge` | coordination · gate | shadow, 0,70 | noul `stalled` (stall-judge prompt'unun karşılığı) | LLM yargıcına döner |
| `flow-judge` | flows · pick | on (açık), 0,60 | choice `arm` | Varsayılan dal; döngüde sınıra kadar |
| `flow-criteria` | flows · select | on (açık), 0,60 | ölçüt başına noul `c1..cN` (tek çağrı); hepsi ≥ eşik → `pass` | Varsayılan dal; adım ayrıntısına "checker unavailable" |
| `flow-grade` | flows · rate | off, 0,50 | score `grade` (useless … excellent → 1..5) | Koşu puansız kalır; gözlemci ve akış otomasyonları puana bakmaz |
| `flow-proposal-gate` | flows · gate | shadow, 0,70 | noul `improves` (akış + istatistik + öneri + diff) | Öneri eskisi gibi uygulanır (auto politika) |
| `phase-gate` | flows · gate | on (açık, fail-closed), 0,80 | noul `holds` (kök oturumun son 40 mesajı) | **Kapı kapalı kalır** |
| `session-setup` | session · select | off | İzinli skill ve araç adaylarına noul | Mevcut hazırlık/keşif yolu |
| `model-router` | session · pick | off | Kullanılabilir model adaylarına choice | Ajanın/kullanıcının seçimi |
| `clarification` | session · gate | off | İsteğe bağlı soruların zaten cevaplanıp cevaplanmadığı | Soru kullanıcıya gider |
| `compact-retention` | context · triage | off | Bağlam parçalarına koru/özetle/çıkar choice | Mevcut compact girdisi |
| `context-reminder` | context · select | off | Kayıtlı ilgili bağlam parçalarına noul | Yeni model hatırlatması eklenmez; kullanıcı pinleri korunur |
| `worker-review` | collaboration · select | off | Worker sonucu için en fazla üç inceleme bakışı | Sonuç normal biçimde teslim edilir |

Altı oturum/bağlam/işbirliği iş akışının tetikleyicileri, modları, bütçeleri ve
CLI sınırları [92-JEV-IS-AKISLARI.md](92-JEV-IS-AKISLARI.md) içindedir.

Ayrıntılar (değişmedi): tool-risk salt-okunur komutları sormaz, `on`'da riskli
komutu onay istemine çevirir (`exec:decider`), gözetimsiz turu asla bloklamaz,
işaretli çağrıda "her zaman izin ver" tam komutu onaylar; claude-cli izin aracı da
aynı kontrolü kullanır. flow-judge: route düğümünde `mode: "judge"`; flow-criteria:
`mode: "criteria"` + `criteria[]`; flow-grade her başarılı akış koşusundan sonra
arka planda; flow-proposal-gate yalnız `auto` politikalı akışlarda uygulanmadan önce
([93 §8](93-EVRILEN-AKISLAR.md)). phase-gate: kapı türü `judge`, değeri çıkış koşulu.

## 6. Desenler (yeni merci yazarken)

| Desen | API | Kullanım |
|---|---|---|
| gate | `Noul` + `Answer.Yes(th)` | Bir koşul sağlanıyor mu |
| pick | `Choice` + `decider.Pick(resp, key, th)` | N seçenekten biri; eşik altında "emin değil" (eğilim yine döner) |
| rate | `Score` + `Answer.Level()` | Sıralı ölçekte yer |
| select | Merciye özel `Request` + `Hub.Decide` | Aday başına noul; aday/seçim/bütçe sınırları iş akışında uygulanır. `internal/agent/decision_policy.go` içindeki `selectionRequest` ve `selectedVerdict` mevcut oturum akışlarını destekler. |
| triage | Merciye özel choice soruları + `Hub.Decide` | Öğe başına koru/özetle/çıkar gibi etiket; kompakt akışının bağlam ve parça sınırları korunur. Genel amaçlı `Hub.Select`/`Hub.Triage` API'si yoktur. |

İstekler normal `Decide` yolundan geçtiği için mod, yedek, rakip ve faturalama
aynı merkezden uygulanır. Protokol isteği en çok 64 soru içerir; mevcut iş akışları
kendi daha dar aday ve kanıt bütçelerini ayrıca uygular. Kısmi/başarısız cevapta
ne yapılacağı ilgili merciin deterministik geri dönüş sözleşmesidir; genel bir
paralel parçalama servisi varmış gibi varsayılmaz.

## 7. Yeni karar mercii ekleme

1. Descriptor: `decider.RegisterAuthority(decider.Authority{…})` merciin paketinde
   (`init()`); grup ve desen seç, `Modes`/`DefaultMode`/eşik ver.
2. Çağrı: `r.decide(ctx, id, caller, req, decider.WithOutcome(…))` (agent içi) ya da
   `hub.Decide/Select/Triage(…, decider.WithBilling(…), decider.WithBackground(…))`.
3. `decider.NewRecord` ile ledger kaydı (`Outcome`, `Strength`, gölgede `Baseline`,
   uygulandıysa `Applied`).
4. Hata hâlinde eski mantığa dön (fail-open) ya da güvenlik kapısıysa kapalı kal
   (`FailClosed: true`).
5. `frontend/src/i18n/locales/{en,tr}/decider.json` → `authority.<id>.*`.

### 7.1 Uygulanan fikirler ve kalan öneri

Model yönlendirme, başlangıç skill/araç seçimi, compact koruması ve bağlam
hatırlatması artık kayıtlı mercilerdir; bunlar yeni taslak gibi ele alınmaz.
Güncel sözleşmeleri [92-JEV-IS-AKISLARI.md](92-JEV-IS-AKISLARI.md) tutar.

**Oturum/görev temizleyici** henüz öneridir: boşta kalan oturumlara
`keep / archive / review` gibi etiketler veren merciye özel choice soruları
hazırlanabilir. Deterministik aktif-oturum korumaları önce gelir; görevlerde
durum değiştirmek yerine öneri üretmek tercih edilir. Bu belge böyle bir
bakım işlemini uygulamaz veya yetkilendirmez.

## 8. Yeni backend ekleme

1. `internal/decider/<ad>.go`: `Backend` — `Manifest()` (id, etiket,
   `ProviderKinds`, `DefaultBaseURL`, `KeyRequired`, `Fields`, `Models`,
   `DefaultModel`, `ContextTokens`, `DefaultTimeoutMs`, `Limits`, `ModelPrefixes`,
   `DecisionOnly`, `Calibrated`, `Presets`), `Accepts(kind, baseURL)`,
   `New(Endpoint, ClientOptions)`; gerekirse `ValidateConfig`.
2. `init()` içinde `Register(...)`.
3. URL'yi `endpointBase(ep)` üzerinden türet (ödünç uç nokta kuralı).
4. Cevabı nötr `Answer`'a çevir, `BillingProvider`/`BillingModel` doldur; fiyat
   `providers/pricing.go`'da ya da manifest modelinde (servis maliyet bildirmezse
   `fillCost` manifest fiyatını kullanır).
5. `httptest` ile istek/yanıt şekli testi.

## 9. Arayüz

İki ekran (`frontend/src/features/decider/`, namespace `decider`, `I18N_MIGRATED`):

- **Ayarlar → Sağlayıcılar → "Karar sağlayıcıları"** (`DeciderProviders`): sağlayıcı
  örneklerinin hemen altında, onlarla aynı kart/form tasarımında (`providerStyles.ts`
  sağlayıcı ekranının sınıflarını paylaşır). "+ Yeni karar sağlayıcısı" → form:
  isteğe bağlı şablon, tür (backend; düzenlemede kilitli) + etiket, Etkin, model
  (öneri listesi ya da özel id), kimlik bilgisi (kendi uç noktası + şifreli API
  anahtarı ya da sağlayıcı hesabını ödünç alma — hesap listesi form açılırken
  tazelenir), zaman aşımı, bağlam, backend alanları. Kart: etiket, tür, durum
  (hazır / hazır değil / duraklatıldı / devre dışı), yerel ve yaklaşık rozetleri,
  id/model/uç nokta/kullanan, istatistik, Dene / Düzenle / Sil. Değişiklikler anında
  kaydedilir. Ajan model seçicilerinde görünmezler.
- **Ayarlar → Karar Mercileri** (`DeciderPanel`): başlık kartı (durum, varsayılan
  sağlayıcı), ana anahtar, varsayılan karar sağlayıcısı, sağlayıcı sayısı ve
  Sağlayıcılar'a geçiş bağlantısı, gruplu merciler (mod, eşik, sağlayıcı / yedek /
  rakip — cevap veren sağlayıcı yedek/rakip listesinde çıkmaz —, istatistik, gölge ve
  rakip önerileri), Kaydet, varsayılan sağlayıcıyı deneme, son kararlar (rakip ve yedek
  rozetleri).

**Kurulum akışı** (`DeciderSetupGuide`, `setupSteps`): iki ekranın başında, üç adım
bitene kadar görünen rehber; her adım tamamlanınca işaretlenir:

1. **Sağlayıcı hesabı** — Sağlayıcı örneklerine bir OpenRouter hesabı ekle ve API
   anahtarını gir (tür "OpenRouter (OpenAI-uyumlu)" → "API Anahtarı"). Yerel bir model
   (LM Studio, Ollama, OpenJev) ya da kendi anahtarıyla bir karar sağlayıcısı
   kullanılacaksa gerekmez. Tamam sayılır: ödünç alınabilir bir hesap ya da kendi
   anahtarı (gerekiyorsa) olan bir karar sağlayıcısı var.
2. **Karar sağlayıcısı** — "Jev · OpenRouter ekle" düğmesi şablonla formu açar (hesabı
   ödünç alır, token ikinci kez girilmez) → Ekle → kartta Dene. Tamam sayılır: etkin ve
   hazır bir karar sağlayıcısı var.
3. **Karar mercileri** — Karar Mercileri'nde "Karar katmanını kullan"ı aç; merciler önce
   Gölge'de, uyum yükselince Açık. Tamam sayılır: ana anahtar açık.

Rehber (`deciderModel.ts`): en az 50 karşılaştırmadan sonra uyum ≥ %90 "açmaya
hazır" / "rakip devralabilir", < %80 "gölgede tut" / "rakip çok ayrışıyor".

## 10. Güvenlik, gizlilik, maliyet

- Ödünç anahtar `providers.HTTPAccess` kapanışında kalır; kendi anahtarı şifreli
  saklanır, API ve log'a düşmez (test: `TestDeciderModelsCRUD`).
- Gönderilen her state maskelenir (bilinen anahtar biçimleri, `Bearer`, `password=…`,
  private key blokları, URL içi parolalar); metin state bağlama göre ortadan kırpılır,
  sığmayan yapılandırılmış state gönderilmez. Yerel modele de aynı maskeleme uygulanır.
- Kullanım `decide` çağrı türüyle çağıran ajana yazılır; sağlayıcı sütunu modelin
  faturalama sağlayıcısıdır (`openrouter`, `typesafe`, `local`). Fiyat tablosu:
  `openrouter` altında `typesafe/jev-1.13`, `~typesafe/jev-latest`,
  `typesafe/jev-latest`; `typesafe` altında `jev-latest`, `jev-1.13` (girdi
  $0.042/M, çıktı $0); `local` ve `lmstudio` bilinen sıfır.
- Chat koruması: `IsDecisionModel` yalnız `DecisionOnly` backend'lerin model ve
  öneklerini (`typesafe/`, `~typesafe/`, `jev-`, `openjev`) reddeder.

## 11. Test haritası

2026-10-01 ekleri: [Karar debug ve inceleme](87-KARAR-DEBUG.md),
[JEV ile 20 kullanım fikri](87-JEV-FIKIRLERI.md). Karar Mercileri ekranındaki
debug zaman çizelgesi ve `read_decider_debug` yeni kararların model/HTTP
denemelerini, oturum/tur bağlantısını ve uygulanmış sonuçlarını izler.
Eski kayıtların girdileri yeniden oluşturulmaz; uyum doğruluk değildir.

- `internal/decider`: `hub_test` (anahtarlar, ödünç/kendi kimlik bilgisi, önbellek,
  model başına karantina ve circuit, yedek, rakip + faturalama + sözlük, redaksiyon,
  kalıcılık, v1 dönüşümü, maliyet doldurma), `modelstore_test`, `config_test`,
  `ledger_test` (rakip/yedek istatistikleri, eski `site` satırları), `authority_test`,
  `patterns_test` (Pick, Verdict, Select/Triage parçalama, uçtan uca Select),
  `openrouter_test`, `systemone_test` (tolerant cevap, URL kuralı, host'a göre
  faturalama, ödünç uç nokta kuralı), `logprobs_test` (dağılım, think bloğu, logprobs
  yok, akıl yürüten model uyarısı, paralellik, etiketler); canlı testler
  `TestLiveOpenRouterDecisions` (`OPENROUTER_LIVE_KEY`) ve
  `TestLiveLocalLLMDecisions` (`DECIDER_LLM_LIVE_URL`, `DECIDER_LLM_LIVE_MODEL`,
  `DECIDER_LLM_LIVE_SUFFIX`).
- `internal/agent/decide*_test.go`: faturalama, rakibin faturası ve sözlüğü,
  stall-judge, tool-risk, flow judge, phase-gate.
- `internal/api/decider_test.go`: ayar turu, model CRUD (anahtar sızmaz, `usedBy`,
  silmede referans temizliği, ulaşılamayan model testi), chat koruması.
- Frontend: `features/decider/deciderModel.test.ts`, `modelDraft.test.ts`, i18n
  katalog paritesi.
