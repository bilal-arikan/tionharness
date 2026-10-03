# 93 — Evrilen Akışlar (Ajan-Başına Ana Akış)

> **Özet (2026-10-04):** Orkestrasyon akışları (bağımsız flow varlığı, 13 node tipi, koşu
> ağacı, flow-backed schedule/automation/task, market flow paketleri) **kaldırıldı**; yerine
> **her ajanın tek bir ana akışı** var ve ajanın **her turu** o akıştan geçer. Varsayılan akış
> `girdi → yanıt → çıktı`'dır ve düz sohbet turuyla bire bir aynıdır. Akış sürümlüdür; kullanıcı
> (kanvas), ajanın kendisi (`edit_flow`) ve **Flow Observer** sistem ajanı (koşu-sonu
> gözlemci) yeni sürümler açar; ajan promptları (soul/identity) da aynı şekilde sürümlenir.
> Motor `internal/flow` (yaprak paket), depo `internal/db/store_flow.go`, tur entegrasyonu
> `internal/agent/flowturn.go`, gözlemci `internal/agent/flow_optimizer.go`, araçlar
> `internal/tools/builtin_flow.go`, REST `internal/api/flows.go`, ekran
> `frontend/src/features/flows/`. Kaldırılan sistemin tarihçesi `arsiv/15-FLOW-CANVAS.md`
> ve `arsiv/62-BIRLESIK-RUN-AWAIT.md`. **Ek (2026-10-04, aynı gün):** JEV karar
> mekanizmaları — `criteria` yönlendirme modu (`flow-criteria`), koşu puanlama
> (`flow-grade`), öneri kapısı (`flow-proposal-gate`) — ve otomasyon bağlantısı:
> `trigger` düğümü (akış içinden herhangi bir otomasyonu ateşler) + `flow` türü otomasyon
> (koşu bitince ateşlenir). Beyin fırtınası ve uygulanmayan fikirler §8.

## 1. Model

- **Flow** (`db.Flow`): `AgentID` (sahibi), `Graph` (baş sürümün JSON'u), `Version`,
  `Policy` (gözlemci politikası), `Stats` (koşu sayaçları), `Note`. Her ajan için en fazla bir
  tane; yoksa ilk ihtiyaçta varsayılan akış **v1** olarak yaratılır (`EnsureAgentFlow`).
  Sistem ajanlarının akışı yoktur.
- **Graf** (`flow.Graph`): `nodes[]` + `edges[]` + `maxSteps`. Düğüm tipleri:

  | Tip | Alanlar | Anlamı |
  |---|---|---|
  | `input` | — | turun girdisi (`{{input}}`); tam bir tane |
  | `llm` | `prompt`, `context` (`thread` / `fresh`), `tools` (`inherit` / `none`), `outputSchema`, `model`, `agentId` | bir model çağrısı; `thread` = oturum geçmişi + prompt yeni kullanıcı mesajı, `fresh` = yalnız sistem promptu + prompt |
  | `route` | `mode` (`contains` / `equals` / `regex` / `json` / `judge` / `criteria`), `jsonField`, `question`, `criteria[]`, `maxVisits` | son çıktıyı kenar etiketleriyle eşleyip bir çıkış kenarı seçer; etiketsiz kenar varsayılan koldur. `judge`: karar modeli etiketi seçer; `criteria`: karar modeli her ölçütü tek çağrıda sorar, hepsi sağlanırsa `pass`, aksi halde `fail` etiketli kol (adım kaydına hangi ölçütün düştüğü yazılır) |
  | `transform` | `template` | model çağrısız şablon |
  | `trigger` | `automationId`, `template` (yük; varsayılan `{{last}}`) | seçili otomasyonu (her tür) render edilmiş yükle `{{result}}` olarak ateşler; akışın son çıktısı **değişmeden** geçer, ateşlenemeyen (kapalı, bekleme, limit, fren) durum adım ayrıntısına yazılır, tur düşmez |
  | `output` | `template` (varsayılan `{{last}}`) | turun yanıtı; tam bir tane |

  Şablon yer tutucuları: `{{input}}`, `{{last}}`, `{{node.<id>}}`, `{{visit}}`, `{{step}}`.
- **Sonlanma kodda zorlanır:** `Validate` tek giriş/çıkış, erişilebilirlik, doğrusal düğümde
  tek çıkış kenarı, **her döngünün varsayılan kollu bir route'tan geçmesi** ve `HardMaxNodes`
  sınırını denetler. Motor `maxSteps` (varsayılan 24, üst sınır 100) ve route başına
  `maxVisits` (varsayılan 3; aşılınca varsayılan kol) uygular. Yargıç modu `flow-judge`,
  ölçüt modu `flow-criteria` karar merciini kullanır; yanıtsız/kararsız karar varsayılan
  kola düşer. `criteria` modunda en az bir ölçüt (en çok 12) ve `pass` ya da `fail`
  etiketli bir kol zorunludur.
- **Önemsiz akış** (`IsTrivial`): `input → llm(thread, {{input}}) → output({{last}})` — davranış
  düz turla aynıdır; düğüm kartları gösterilmez, yalnız koşu kaydı tutulur.

## 2. Tur entegrasyonu

`Runtime.completeTracedInner` kapı ve kararlardan sonra `flowForTurn` ile ajanın akışını
yükler ve `runFlowTurn`'a girer; her `llm` düğümü `executeTurn` (düz tamamlama, claude-cli
delege döngüsü ya da native araç döngüsü) ile koşar, yani aşamalar sohbet turunun tüm
yeteneklerini (araçlar, izinler, prompt cache, resume) korur. Yardımcı çağrılar (başlık,
özet, compaction, ders, btw) ve sistem ajanları doğrudan yola gider.

- `thread` düğümü taban isteğin son kullanıcı mesajını render edilmiş promptla değiştirir;
  `fresh` düğümü geçmişi, özeti ve CLI `ResumeSessionID`'yi atar. Başka ajanı adlandıran
  düğüm o ajanın statik önekiyle `fresh` koşar.
- Sentezlenen yanıt: `output` metni, toplam kullanım, son model, **son thread düğümünün** CLI
  oturum kimliği (resume defteri bozulmasın), `ProviderCalls`.
- Canlı: her düğüm için `flow_node` SSE çerçevesi (`events.TypeFlowNode`, hedef
  `flowRunId/flowId/agentId/sessionId`) ve önemsiz olmayan akışlarda transkripte yazılan
  `flow_node` adım kartı (`StepFlowNode`; başlarken açılır, bitince aynı ID ile kapanır, düğümün
  kendi araç adımlarından ÖNCE durur). Önemsiz olmayan akışta ara `delta` akışı bastırılır.
- **Koşu** (`db.FlowRun`): tur başına bir satır — girdi/çıktı, sürüm, tetik (çağrı türü),
  oturum + mesaj kimliği, düğüm izi (`flow.Step[]`; route/trigger adımlarında `detail`),
  süre, token, kullanıcı 👍/👎 (`SetMessageFeedback` aynalar) ve **puan** (`Grade` 1..5 +
  güven; `flow-grade` mercii açıkken). Boot'ta yarım kalanlar `failure` olur
  (`FailOrphanedFlowRuns`); `flowRunRetention` ayarı ajan başına saklanacak biten koşuyu sınırlar.
- **Koşu sonrası kuyruk** (`afterFlowRun`, arka plan turu; yanıtı bekletmez): puanlama →
  `FireFlowRunFinished` (akış-türü otomasyonlar) → `maybeObserveFlow`.

## 3. Evrim

- **Sürümler** (`FlowVersion`, `agent-flow-versions/<FLW>/<n>.json`): değişmez; yazar
  (`user` / `agent` / `observer` / `system`), gerekçe, öneri kimliği, `Diff` özeti. Kanvas
  kaydı, `edit_flow`, uygulanan öneri ve geri dönüş yeni sürüm açar; yalnız konum değişikliği
  sürüm açmaz (`UpdateFlowLayout`).
- **Op sözlüğü** (`flow.Op`): `insert_between`, `add_node`, `remove_node` (doğrusal düğüm
  köprülenir), `update_node` (`id`/`type` değişmez), `add_edge`, `update_edge`,
  `remove_edge`, `set_max_steps`. Ajanlar, gözlemci ve API aynı sözlüğü kullanır; sonuç her
  yolda `Validate`'ten geçer.
- **Büyüme bütçesi:** `Policy.MaxNodes` (varsayılan 16) ajan ve gözlemci için zorlanır;
  kullanıcıyı yalnız `HardMaxNodes` (48) sınırlar.
- **Ajan araçları** (`internal/tools/builtin_flow.go`, öz-yönetim demeti): `get_flow`,
  `edit_flow`, `revert_flow`, `list_flow_runs`, `update_my_prompt`. Prompt değişikliği
  `agent-prompt-versions/<AGT>/<n>.json`'a sürüm olarak yazılır (ilk değişiklikte v1 = evrim
  öncesi metin), `RestoreAgentPromptVersion` ile geri yüklenir. Rehber skill:
  `tionharness-flows`.
- **Flow Observer** (`flow-optimizer` sistem ajanı, prompt `flow-optimizer`):
  `Policy.Mode` `off` / `propose` / `auto`; `EveryRuns` (varsayılan 5) yeni koşuda bir kez
  bakar (`maybeObserveFlow`, arka plan turu). Kanıt: baş graf, soul/identity, istatistik,
  son 8 koşunun düğüm izi ve geri bildirimi, önceki öneriler. Yanıt STRICT JSON
  (`noChange / reason / expected / confidence / ops / prompt`); kodda doğrulanır (op
  sayısı ≤ 8, graf, bütçe) ve `FlowProposal` olarak dosyalanır (`pending` / `applied` /
  `rejected` / `invalid`). `auto` modda `confidence ≥ MinConfidence` ise **öneri kapısından**
  (`flow-proposal-gate` mercii: off → uygula, shadow → uygula ama kaydet, on → "hayır"
  derse `pending` kalır, sonuç `FlowOptimizeResult.Held`) geçip uygulanır; aksi halde
  kullanıcı Evrim sekmesinden uygular/reddeder. Her geçiş `flow-optimizer` etiketli bir
  sistem oturumuna kaydedilir. "Değişiklik yok" iyi bir cevaptır. Kanıtta koşu puanları ve
  ölçüt kapısı ayrıntıları da vardır; pencere içindeki tüm koşular başarılı, puanı ≥ 4 ve
  👎'siz ise geçiş atlanır ve işaret ilerletilir (`flowRunsHealthy`).

## 4. API

`GET /api/flows` (ajan başına satır: ajan adı/avatarı, şekil, önemsiz mi, düğüm ve bekleyen
öneri sayısı) · `GET /api/flows/{id}` · `PUT /api/flows/{id}` (`{graph, reason}` → yeni sürüm;
yalnız yerleşim değiştiyse `changed:false`) · `PUT /api/flows/{id}/meta` (ad/not/politika) ·
`POST /api/flows/{id}/validate` · `GET /api/flows/{id}/versions[/{n}]` ·
`POST /api/flows/{id}/revert` · `GET /api/flows/{id}/runs` · `GET|DELETE /api/flow-runs/{id}` ·
`POST /api/flows/{id}/optimize` · `GET /api/flows/{id}/proposals` ·
`POST /api/flow-proposals/{id}/apply|reject` · `POST /api/flows/{id}/test` (girdiyi
`flow-test` etiketli yeni sohbet oturumunda bir tur olarak kuyruğa alır) ·
`GET /api/agents/{id}/flow` · `GET /api/agents/{id}/prompt-versions` ·
`POST /api/agents/{id}/prompt-versions/{n}/restore`. Otomasyon API'si (`/api/automations`)
`triggerKind: "flow"` ile `flowAgentId` / `flowStatus` / `flowMaxGrade` alanlarını alır.

## 5. Ekran (`frontend/src/features/flows/`)

Sol liste ajan akışları (avatar, sürüm, düğüm/koşu sayısı, bekleyen öneri rozeti); başlıkta
sekmeler **Kanvas · Koşular · Evrim · Test** (`#/w/{ws}/flows/{tab}`). Kanvas React Flow
üzerinde: tipli düğüm kartları (prompt özeti, bağlam/araç çipleri, route kolları), etiketli
kenarlar (geri besleme kenarı kesikli/sarı), düğüm ekleme/otomatik dizme/sığdır araç çubuğu,
seçili düğüm/kenar için inceleyici, istemci tarafı lint, canlı düğüm durumu (çalışıyor/bitti/hata
halkası). İnceleyici `criteria` modunda satır başına bir ölçüt alanı, `trigger` düğümünde
otomasyon seçici + yük şablonu sunar; koşu listesi ve ayrıntısı puan rozetini (★ n/5) ve
adım `detail` satırını gösterir. Kaydet gerekçe ister ve yeni sürüm açar; yalnız yerleşim
değiştiyse sürüm açılmaz.
**Yerleşim:** geniş yatay ekranda liste · kanvas · sağda sabit inceleyici; dar / kare / dikey
ekranda liste çekmece, inceleyici alt sayfa (bottom sheet), Koşular sekmesi liste → ayrıntı
yığını, minimap gizli (`useViewport` tier + aspect). Sohbette `flow_node` kartı aşamayı
gösterir (`FlowNodeStep`).

## 6. Kaldırılanlar

`internal/orchestration`, `internal/flows`, `internal/agent/flow*.go` + `adhocflow*`,
`run_adhoc_flow` ve `create/update/delete/list/run_flow`, `list_flow_runs`
(eski anlam), `deliver_flow_input` araçları; `FlowID` alanları (schedule / automation / task)
ve flow-backed sürücü (`LaunchRun` yalnız oturum sürücüsü); market `flow` paketi ve
workspace şablonlarındaki `flows`; `flow` / `flow-coordinator` oturum türleri; rota ve
görünüm katmanındaki flow koşusu düğümleri; `tionharness-gan-loop` skill'i. Eski
`flows/` ve `flow-runs/` dizinleri diskte durur ama okunmaz; yeni dizinler
`agent-flows/`, `agent-flow-runs/`, `agent-flow-versions/`, `agent-flow-proposals/`,
`agent-prompt-versions/`.

## 7. Test ve doğrulama

`internal/flow/flow_test.go` (motor: döngü, tur sınırı, adım sınırı, yargıç, ölçüt kapısı,
tetikleyici düğüm, op'lar, diff), `internal/agent/flowturn_test.go` (önemsiz akış = düz tur,
eleştiri döngüsü uçtan uca, düğüm hatası, sürümleme + bütçe + geri dönüş + prompt sürümleri,
gözlemci öner/otomatik/geçersiz, geri bildirim aynası),
`internal/agent/flowturn_decider_test.go` (karar stub'ıyla ölçüt kapısı, puanlama + akış
otomasyonu + kendi oturumunu ateşlememe, tetikleyici düğüm + kapalı otomasyon, öneri kapısı
tut/geçir, sağlıklı pencere atlama), `internal/db/automation_core_test.go` (`flow` türü
şekil denetimi), `internal/api` (koşu/panel/görünüm fixture'ları),
`frontend/src/features/flows/flowGraph.test.ts`, `automationPayload.test.ts`. Canlı
doğrulama: 2026-10-04 "Flow Lab" workspace'inde gerçek sağlayıcılarla (bkz. `05-ILERLEME.md`).

## 8. JEV karar mekanizmaları ve otomasyon tetikleme (beyin fırtınası, 2026-10-04)

JEV (TypeSafe'in tipli karar modeli; `internal/decider`, [87](87-KARAR-KATMANI.md)) metin
üretmez, kalibre olasılıkla **evet/hayır · birini seç · puanla** cevaplar; bir çağrı ~0,3–0,8 sn
ve ~$0,00003. Akış sisteminde bunun değerli olduğu üç yer var: **koşu içinde** (bir düğümün
kararı), **koşu sonunda** (koşunun kalitesi) ve **evrimde** (bir değişikliğin kabulü).
Aşağıdaki tablo tartışılan fikirleri ve durumlarını toplar; her mekanizma kodda bir
yedekle (model yoksa eski davranış) bağlıdır ve `off / shadow / on` ile ölçülebilir.

| # | Mekanizma | Girdi → JEV sorusu | Nerede | Durum |
|---|---|---|---|---|
| 1 | **Yargıç kolu** (`route.mode=judge`) | son çıktı → choice: hangi kol etiketi uyuyor | `flow-judge` | uygulandı (ilk sürüm) |
| 2 | **Ölçüt kapısı** (`route.mode=criteria`) | son çıktı + N ölçüt → N noul tek çağrıda; hepsi ≥ eşik → `pass`, değilse `fail`; düşen ölçütler adım `detail`'ine | `flow-criteria` | **uygulandı** |
| 3 | **Koşu puanlama** | istek + yanıt → score 1..5 (`useless … excellent`) | `flow-grade`, `afterFlowRun` | **uygulandı**; varsayılan kapalı (tur başına bir çağrı) |
| 4 | **Öneri kapısı** | akış özeti + istatistik + öneri (ops, diff, gerekçe) → noul "iyileştirir mi" | `flow-proposal-gate`, `OptimizeFlow` auto yolu | **uygulandı**; varsayılan gölge |
| 5 | **Sağlıklı pencere atlama** | son N koşunun puan/durum/👎'si (deterministik, JEV çağrısı yok) | `maybeObserveFlow` | **uygulandı** |
| 6 | **Akış-türü otomasyon** | koşu bitti (ajan, sonuç, puan ≤ k filtreleri) → otomasyon ateşlenir; kendi açtığı oturumu yeniden ateşlemez | `automation_flow.go`, `FireFlowRunFinished` | **uygulandı** |
| 7 | **Tetikleyici düğüm** (`trigger`) | akış içinden herhangi bir otomasyonu render edilmiş yükle ateşler; geçiş düğümüdür | `flowturn_trigger.go` | **uygulandı** |
| 8 | Döngü devam kararı | kritik + taslak → noul "bir tur daha değer mi" (maliyet/fayda) | route judge ile yapılabilir; ayrı merci gereksiz | fikir (mevcut araçlarla) |
| 9 | Düğüm başına model kademesi (`model: auto`) | girdi zorluğu → choice ucuz/güçlü model | `RunLLM` öncesi, `model-router` mantığı | fikir; önce gölgede ölçülmeli |
| 10 | Düğüm başına araç kararı (`tools: auto`) | prompt → noul "araç gerekir mi"; gereksizse `none` (ucuz düz tamamlama) | `RunLLM` | fikir |
| 11 | Prompt değişikliği kimlik denetimi | eski/yeni soul → noul "rol ve kısıtlar korunuyor mu" | öneri kapısının bir alt sorusu | fikir (4'e ek soru olarak ucuz) |
| 12 | Gerileme tespiti / otomatik geri alma | sürüm N vs N-1 koşu puanları (deterministik) → düşüşte `RevertFlow` + bildirim | `afterFlowRun` | fikir; 3 açıkken kolay |
| 13 | Gözlemci ön-eleme | son koşuların kısa izi → triage "öneriye değer / değmez" (ucuz) → pahalı gözlemci çağrısı atlanır | `maybeObserveFlow` | fikir; 5 deterministik hâli |
| 14 | A/B sürüm meydan okuması | iki sürüm dönüşümlü koşar, puanlar karşılaştırılır | sürüm politikası | fikir; çok oturumlu ajanlarda |
| 15 | Araç çıktısı enjeksiyon kapısı | güvenilmeyen metin → noul "talimat saptırması var mı" | route öncesi / `transform` | fikir ([87-JEV-FIKIRLERI](87-JEV-FIKIRLERI.md) #19) |
| 16 | 👎'lerden ölçüt madenciliği | olumsuz geri bildirimli koşular → gözlemci yeni `criteria` önerir | optimizer prompt'u | kısmen (prompt yönlendirmesi var) |

**Tasarım ilkeleri:** JEV yetki vermez ve içerik yazmaz; seçer/puanlar, kod uygular. Her
merci kendi eşiğiyle ayarlanır; ölçüt kapısı ve yargıç kol seçimi "açıkça istenen" (explicit)
mercilerdir ve gölge modları yoktur, puanlama ve öneri kapısı gözlemlenebilir. Tetikleyici
düğüm ve akış-türü otomasyon JEV gerektirmez; puan filtresi (`flowMaxGrade`) yalnız
puanlama açıkken anlamlıdır. Döngü güvenliği: tetikleyici düğüm otomasyonun kendi
korumalarından (açık/kapalı, bekleme, iterasyon üst sınırı, otonomi freni) geçer; akış-türü
otomasyon kendi açtığı oturumun koşusunu yeniden ateşlemez ve `MaxIterations` zinciri keser.
