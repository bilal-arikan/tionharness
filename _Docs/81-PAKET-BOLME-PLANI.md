# 81 — Paket Bölme Planı (`internal/agent`, `internal/api`)

> **Özet (2026-09-03):** Kısmen uygulandı — adım 1/2/3/5 tamam (dört yeni paket, ~4,5k
> satır `internal/agent`'tan çıktı: 62.603 → 58.412), adım 4/6/7 tasarım gerektirdiği için
> bekliyor (aşağıda §6). `internal/agent` 323 dosya / 62.603
> satır, `internal/api` 269 / 46.351, `internal/tools` 190 / 31.820. Bu boyut ajan
> aramalarını (grep, codebase-memory), derleme ve `go test ./internal/agent/` süresini
> (65–78 sn) doğrudan uzatıyor. Bölme, `Runtime` üzerindeki yöntem kümelerinin ayrı
> paketlere taşınmasıyla değil, **önce bağımsız veri/algoritma katmanlarının**
> (trajectory grafı, coordination policy, flow state) çıkarılmasıyla yapılmalı; `Runtime`
> yöntemleri arayüzler üzerinden bu paketlere delege eder. Sıra ve kabul ölçütleri aşağıda.

## 1. Ölçüm (2026-09-03)

`internal/agent` içindeki dosya-önek kümeleri (test dahil satır):

| Küme | Satır | Dosya | Bağımlılık karakteri |
|---|---:|---:|---|
| `coordination*` | 8.866 | 27 | `Runtime` slot/turn/spawn ile sıkı bağlı; policy (stall, situation, tree) ayrılabilir |
| `flow*` | 3.496 | 18 | `db` + `Runtime.completeTraced`; state-delta ve precheck saf |
| `trajectory*` | 2.935 | 13 | Büyük ölçüde saf graf/geçiş mantığı (`trajectory_graph`, `_transitions`, `_summary`, `_recipe_stats`) |
| `automation*` | 2.733 | 13 | Tetik registry `db`'de; deliver/ledger `Runtime` |
| `mcp*` | 2.048 | 13 | args/repair/escalate/notice saf-ya-yakın |
| `spawn*` | 1.985 | 9 | `Runtime` çekirdeği |
| `insight*` | 1.923 | 13 | Zaten `internal/insight` var; analyzer/cron/session köprüleri burada |
| `subagent*` | 1.753 | 11 | Profil sözleşmesi saf, runner `Runtime` |
| `climcp*` | 1.276 | 7 | Saf: config/hook/matcher üretimi |
| `lessons*` | 1.059 | 4 | `db` + sistem ajanı çözümlemesi |
| `codex*` / `claudehome*` | 1.446 | 8 | Sağlayıcı ev/klasör yönetimi, saf-ya-yakın |

`internal/api`'de benzer kümeler: `chat_turn*`, `session*`, `trajector*`, `flow*`,
`automation*`, `settings*`, `mcp_interaction*`.

## 2. İlke

- **Bağımlılık yönü:** yeni paketler `agent`'a bağlanamaz (döngü). Bu yüzden `Runtime`
  yöntemleri yerinde kalır; taşınan şey "Runtime'a ihtiyaç duymayan" katmandır. Runtime'a
  ihtiyaç duyan kısım, yeni paketin küçük bir arayüzünü (`Sink`, `Store`, `Clock`) alır.
- **Adım başına tek küme, tek PR;** her adımın sonunda `scripts/test.sh full` yeşil, test
  dosyaları taşındıkları paketle birlikte gider, `_Docs` indeksi güncellenir.
- **Dışa açık API sabit:** `internal/api` bugün `agent.X` adıyla ne çağırıyorsa, ilk
  aşamada `agent` içinde ince bir delege (`func (r *Runtime) X = pkg.X`) kalır; çağıran
  taraf ikinci aşamada güncellenir.

## 3. Sıra (kolaydan zora)

1. **`internal/agent/climcp` → `internal/climcp`** (saf; girdi `db.Agent`,
   `tools.InteractionEndpoint`, tunable değerleri). Çıktı: config dosyası, allow/deny,
   native allowlist. Testleri hazır (`climcp_*_test.go`, `climcp_allowlist_test.go`).
2. **`trajectory_graph/_transitions/_summary/_recipe_stats` → `internal/trajectory`**
   (`db.Trajectory` modelini alır; `Runtime` yalnız kuyruk + bağlayıcı tutar).
3. **`mcpargs/mcprepair/mcpescalate/mcpnotice` → `internal/mcp/repair`** (arayüz:
   indeks tetikleyici + günlük).
4. **`coordination_situation/_stall/_tree/_observer` → `internal/coordination`**
   (policy katmanı; `coordSlot` durumu bir arayüz arkasına: `SlotView`).
5. **`flow_state_delta/flow_precheck/flow_defaults` → `internal/flow`**.
6. **`subagent` profil sözleşmesi + allowlist → `internal/subagent`**; runner
   `agent`'ta kalır.
7. `internal/api`: `chat_turn*` (tur kompozisyonu) → `internal/api/turn`; `trajector*`
   → `internal/api/trajectories`; handler kayıtları `server.go`'da tek yerde kalır.

## 4. Kabul ölçütleri

- `internal/agent` ≤ 35k satır, `internal/api` ≤ 30k satır (ilk hedef).
- `go test ./internal/agent/` ≤ 40 sn (bugün 65–78 sn).
- Hiçbir yeni paket `internal/agent`'ı import etmez (`go list -deps` ile CI'da kontrol).
- `_Docs/01-MIMARI.md` paket haritası güncellenir; `_Docs/80` §8 tablosuna yeni paketler
  eklenir.

## 5. Uygulama günlüğü (2026-09-03)

| Adım | Paket | Satır | Commit | Not |
|---|---|---:|---|---|
| 1 | `internal/climcp` | 1.289 | `415b1647` | `Host` arayüzü (`EnabledServers`, `ServerGate`, `EnabledHooks`, tunable'lar, logger, debug); `agent.cliHost` adaptörü; saf testler sahte host ile, db-bağımlı olanlar `agent`'ta kaldı |
| 2 | `internal/trajectory` | 1.324 | `75f27660` | graf/özet/geçiş differ'ı/reçete istatistikleri; `agent` tarafında `TrajectoryPlanPhase`/`TrajectoryTransition`/`RecipeStats` alias'ları, `api`/`workspace` dokunulmadı |
| 3 | `internal/mcp/repair` | 1.312 | `0e7e84e1` | `Guard`, `FailStreaks`, `FailureCollector`, `CallKey`; proje-id kuralı `mcp.ProjectIDForPath` oldu (capabilities + repair tek kaynak) |
| 5 | `internal/flows` | 586 | `067de343` | state-delta yazıcısı + varsayılan flow tohumlama/migrasyon; `workspace` yeni paketi çağırır |

Bağımlılık yönü kapısı: `scripts/depcheck.sh` (`scripts/test.sh full` içinde) yeni paketlerin
`internal/agent`'ı import etmediğini `go list -deps` ile doğrular.

Ölçülen gerçek: `internal/agent`'ın kalan 58k satırının büyük kısmı `func (r *Runtime)`
yöntemleridir (koordinasyon 4.386 satırda 94 Runtime yöntemi, yalnız ~313 satır saf yardımcı;
otomasyon motoru `Runtime`, `LaunchRun`, tetikleyici sabitleri ve tur slotlarına doğrudan
bağlı; subagent runner `Runtime`). Bunlar dosya taşımakla değil, önce bir arayüz
(`SlotView`/`RunHost`) tasarlayıp yöntemleri o arayüz üzerinden yeniden yazmakla çıkar.

## 6. Kalan adımlar (tasarım gerektirir)

- **Adım 4 — `coordination`:** saf kısım küçük (`formatWorkerTree`, `parseStallVerdict`,
  `slotIsStallCandidate`, `spawnclaim.go`); `coordSlot` ve `TurnStep` tipleri ile 94 yöntem
  `Runtime`'a bağlı. Önce `SlotView` (salt-okunur slot görünümü) + `WorkerNotifier`
  arayüzleri; sonra stall/situation/tree politikaları taşınır. Riskli bölge (stall
  guard olayları), ayrı bir görev olmalı.
- **Adım 6 — `subagent`:** profil sözleşmesi (`defaultSubagentProfiles`, allowlist)
  `systemagents.go` ve `subagent_allowlist.go` ile iç içe; runner `Runtime`. Profil +
  allowlist saf katmanı ~250 satır; kazancı küçük, `systemagents` ile birlikte ele alınmalı.
- **Adım 7 — `internal/api` bölme:** 271 handler tek `server.go`'da kayıtlı; `chat_turn*`
  (1,9k) ve `session*` (6k) kümeleri `Server` alanlarına bağlı. Alt paket için önce
  `Server`'ın tur-kompozisyon bağımlılıklarının (`convo`, `providers`, `tun`, `hub`)
  bir arayüze indirgenmesi gerekir.

## 7. Yapılmayan

Adım 4/6/7 bu turda uygulanmadı: mekanik taşıma değil arayüz tasarımı gerektiriyor ve
koordinasyon kodu en çok olay üreten bölge. Her biri ayrı, testleri yeşil bir PR olmalıdır.
