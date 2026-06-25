# SwarmGo — İlerleme Takibi

> Bu dosya canlı tutulur; her oturumda güncellenir. Son güncelleme: **2026-06-25**

## Generic import pipeline — internal/ingest (SK-IMP3) ✅ (2026-06-25)

İçe aktarma **skill-özel olmaktan çıkıp jenerik, çok-türlü** bir boru hattına dönüştü.
Tek hat artık GitHub repo / plugin / local klasörden **skill + agent + command + MCP**
keşfedip hepsini market ile **aynı kurulum otoritesine** bağlıyor. Beş fazda yapıldı:

- **Faz 1 — `internal/fetch` (edinme):** tarball indirme + local tree walk + GitHub URL
  ayrıştırma tek pakete çıkarıldı (`TreeFrom`/`GroupByMarker`/`FindFiles`). `skills` ve
  `ingest` ortak kullanır; eski `skills/github.go` (contents-API) silindi.
- **Faz 2 — `market.Pack.Files`:** pack envelope'una `Files map[string][]byte` eklendi;
  `InstallSkill` nested kaynakları (`safeRelPath`) yazar, publish (`collectSkillFiles`)
  toplar → yayınla/kur kaybsız.
- **Faz 3 — `internal/ingest` + Adapter:** `Adapter{Kind,Scan}` arayüzü + `Discovered`
  (preview + `build` closure) + orchestrator (`Scan`/`BuildPacks`). Skill adapter
  `skills.RenderImportedSkill`'i sarmalar; `skills`'ten helper'lar export edildi
  (`RenderImportedSkill`/`Slugify`/`SuggestSlug`/`FrontmatterField|List|Body`).
- **Faz 4 — tek install otoritesi + kind-agnostik UI:** `api/market.go` kind switch'i
  `installPackInto(...) (InstallResult, error)`'a çıkarıldı (alt-installer'lar `httpErr`
  ile HTTP kodu taşır); market install **ve** yeni `POST /api/ingest/{scan,install}`
  ortak çağırır. `SkillImportDialog` kind-agnostik. Eski `/api/skills/import/{scan,bulk}`
  kaldırıldı; `/api/skills/import` (tek) korundu.
- **Faz 5 — agent/command/mcp adapter'ları:** `**/agents/*.md`→AgentPayload (body→Soul,
  tools→AllowedTools; CC model uyarıyla eşlenmez), `**/commands/*.md`→loadable skill
  (`.toml` atlanır), `.mcp.json`→MCPPayload. **`(kind,slug)` dedup** (repo'nun `plugins/`/
  `dist/` aynası → en sığ yol).

**İlke:** import market'in **içine taşınmadı**; ayrı `internal/ingest` paketi market'i
bağımlılık olarak kullanır (döngüsüz: `ingest→{fetch,skills,market}`, yalnız `api→ingest`).
**Canlı:** caveman **10** (3 agent + 7 skill; `plugins/` aynası dedup'landı), taste-skill
**13**, marketingskills **45**. Go vet temiz, **460 test**, tsc + prod build temiz.
Detay: **`_Docs\37-INGEST-MIMARISI.md`**, `21-MARKET.md` §4.1.

## Generic AgentIdentity bileşeni + agent ayar pill seçicileri ✅ (2026-06-25)

**AgentIdentity (`components/agents/AgentIdentity.tsx`):** Uygulama genelinde agent
gösterimini (avatar + ad + opsiyonel alt satır) tek bileşene topladı. `size`
(sm/md/lg), `subtitle` ('model' → katalogdan çözülmüş model etiketi · 'none' ·
özel ReactNode), `active` (avatar halkası), `dim`, `nameSuffix`, `trailing`
props'ları. `AgentAvatar` düşük seviyeli primitif olarak kalır; AgentIdentity onu
+ `resolveModelLabel`'i sarmalar. Taşınan yerler: composer **AgentSelect**
dropdown'ı, **AgentHeader** (sohbet balonu), **SessionDetailPanel** "Konuşmadaki
ajanlar", **TaskBoard** kart sahibi, **AgentPicker** (board kart-detay sahip
seçici — trigger+seçenekler, artık model alt satırı), **AgentRoster**,
**AgentsView**. (Avatar-only ikon kullanımları — SessionsSidebar/Executions/
Schedules/Artifacts/Budget/Autocomplete/flow AgentNode — primitif AgentAvatar'da
bırakıldı; isim ayrı/karmaşık satırda.)

**Agent ayar formu pill seçiciler:** Planlama/Düşünme/İzin `<select>`'leri ikon+
etiketli `OptionPills` (radiogroup) ile değiştirildi; ikon dili composer ile
ortak (`components/agents/agentOptions.ts`). Düşünme "Kapalı" = boş string
(depolama korunur).

## Default skill: `swarmgo-doc-improver` (6-ölçütlü doküman denetimi) ✅ (2026-06-25)

**Ne:** Yeni gömülü default skill — SwarmGo'nun kendi bağlam dokümanlarını (skill'ler,
workspace CLAUDE.md/AGENTS.md kuralları, `_Docs`) denetleyip iyileştiren tekrarlanabilir
iş akışı. Anthropic'in resmi `claude-md-improver` skill'inden ilham; SwarmGo'nun daha
geniş doküman yüzeyine genelleştirildi.

**İçerik:** 6 ölçüt (komutlar / mimari açıklığı / açık-olmayan gotcha'lar / kısalık /
güncellik / uygulanabilirlik, her biri /5) + puanlı rapor formatı. SwarmGo'ya özgü
çekirdek içgörü: **changelog-leak anti-pattern** — referans dokümanların (`swarmgo-project`,
`_Docs` mekanik bölümleri) tarih damgalı geçmişi biriktirmesi; çözüm "1 cümle güncel durum
+ → `_Docs/NN`" kalıbı. **Doc-type kalibrasyonu:** referans/her-tur-yüklenen dokümanda
kısalık sert, on-demand action skill gövdesinde işlevsel yoğunluk normal → sağlıklı
dokümanı zorla kesme.

**Gömme:** Go değişikliği **gerekmedi** — `internal/skills/defaults.go` `//go:embed defaults`
tüm ağacı gömer ve slug'ları alt-dizinlerden türetir; yalnız `defaults/swarmgo-doc-improver/SKILL.md`
eklendi. Build + 29 test yeşil.

**Yan iş (aynı oturum, davranışsız doküman temizliği):** `swarmgo-project` referans skill'i
~%11 kısaltıldı (changelog-leak temizlendi + PowerShell çalıştırma komut bloğu eklendi);
`swarmgo-guide` Memory maddesi okunabilirlik için alt-maddelere bölündü; default-skill listesi
güncellendi (progress/gan-loop/doc-improver eklendi).

## Agent avatar mojibake onarımı + model adı gösterimi ✅ (2026-06-25)

**Sorun:** claude-cli `create_agent` yoluyla (bir agent'ın başka agent oluşturması)
yaratılan agent'larda emoji avatar + soul/identity metinleri **mojibake**'ye
dönüşüyordu (UTF-8 baytları Latin-1 olarak yanlış çözülüyor; ör. 🗺️ → `ðºï¸`).
UI'da ikon bozuk kutucuklar olarak görünüyordu (AGT1–5 UI'dan temiz, AGT6–9
agent-üretimi bozuk). CLI/MCP transport katmanından geliyor.

**Üç katmanlı çözüm:**
1. **Frontend (görüntü):** `lib/avatar.ts` → `normalizeAvatar` + `repairMojibake`;
   gerçek emoji aynen, mojibake onarılır, onarılamazsa **baş harflere** düşer
   (asla çöp glyph). `AgentAvatar` `isEmoji` boyutlandırması normalize'a bağlandı.
2. **Backend (kök neden):** `tools/builtin_agentmgmt.go` → `repairMojibake`
   (Türkçe ş/ğ/ı korunur; rastgele Latin-1 katı UTF-8 re-decode'da elenir);
   `create_agent` + `update_agent`'ta `name`/`soul`/`identity`/`avatar` kaydedilmeden
   onarılır. Test `builtin_agentmgmt_test.go` (9 vaka).
3. **Veri:** WS2/AGT6–9 dosyaları onarıldı (✈️🏨📍🗺️), `.bak` yedekleriyle.

**Model adı:** Agent gösterilen yerlerde ad altında **soluk** model adı (boşsa
provider): composer `AgentSelect` dropdown'ı + `AgentRoster`. Composer tetik butonu
avatar + **ajan adı** gösteriyor.

**Yan onarım (build kıran önceki sorun):** `providers/pricing.go` mükerrer `"minimax"`
map anahtarı tek bloğa birleştirildi (M2.1/lightning/M2/M1/Text-01, 0.25× cache).

## Mermaid diyagram render desteği (chat) ✅ (2026-06-25)

Sohbet markdown'ında ```` ```mermaid ```` fenced blokları artık tema-duyarlı SVG
olarak çizilir (akış/sıra/durum/sınıf/ER/gantt vb.). `diff` bloklarının
`DiffView`'a yönlenmesiyle aynı kalıp.

- **Bağımlılık:** `mermaid@^11` (`frontend/package.json`). Ağır (~3MB) →
  **dinamik `import()`** ile lazy yüklenir; `vite.config.ts` `manualChunks`'a
  `vendor-mermaid` (mermaid + d3/dagre/cytoscape/khroma/elkjs) eklendi → ana
  bundle'a binmez (yalnız bir diyagram render edilince iner; `relationGraph`
  kalıbı). Build doğrulandı: `vendor-mermaid` ≈ 3.1MB ayrı chunk, `index` sabit.
- **Bileşen:** `frontend/src/components/markdown/MermaidDiagram.tsx` — tema base'i
  `<html data-theme>`'ten (`light`→`default`, yok→`dark`) seçilir, `themeVariables`
  CSS değişkenlerinden (`--color-accent`/`-surface`/`-text`/…) türetilir;
  `MutationObserver` ile tema değişiminde yeniden çizer. **Akış-dayanıklı:** 120ms
  debounce + render hatasında ham kaynağa düşer (yarım kalan diyagram
  patlatmaz); ayrıca geçersiz sözdiziminde mermaid'in `<body>`'ye iliştirdiği
  hata/"bomba" SVG'si `finally`'de id ile temizlenir (orphan leak yok). Toolbar:
  Source/Diagram toggle, Expand (tam-ekran overlay), Copy. `securityLevel: 'strict'`.
- **Entegrasyon:** `CodeBlock.tsx` `lang === 'mermaid'` → `MermaidDiagram`.
- **Test (Playwright, 2026-06-25):** geçici harness ile flowchart/sequence/state
  render, dark+light tema geçişinde yeniden renklenme ve geçersiz blok → kaynak
  fallback (crash yok, bomba leak yok) gerçek tarayıcıda doğrulandı.
- **Ajan farkındalığı:** `swarmgo-guide` default skill'ine "Rich replies"
  bölümü eklendi (mermaid/diff/kod render edildiğini ajana öğretir).
- Detay: `_Docs\07-CHAT-UX.md`.

## Workspace'e özel görünüm/tema + Dil → Profil ✅ (2026-06-25)

Ayarlar ▸ **Görünüm** kategorisi uygulama-geneli olmaktan çıkıp **aktif
workspace'e özel** hâle getirildi: her workspace kendi tema paleti / temel mod /
accent'ini saklar ve **workspace değiştirilince UI teması da değişir**.

- **Backend:** `WSSettings`'e `Theme`/`Accent`/`ThemePreset` alanları (+ patch +
  `UpdateSettings`) ve `workspaceSettingsDTO`'ya aynı alanlar eklendi. Boş alan =
  uygulama-geneli görünümü miras alır. `ws-settings.json`'da kalıcı.
- **Frontend:** `lib/theme.ts`'e `Appearance` tipi + `resolveAppearance`
  (workspace override'ı global üstüne katmanlar) + `applyAppearance`. `App.tsx`
  global+workspace görünümünü iki ref'te tutar, workspace değişiminde
  `GET /api/workspace-settings` çekip `applyResolvedTheme()` çağırır; global
  ayar kaydı workspace seçimini ezmez. `AppearancePanel` self-contained yeniden
  yazıldı: doğrudan `PUT /api/workspace-settings`, canlı önizleme, unmount'ta
  kaydedilmemiş önizlemeyi geri alma, "Genele sıfırla" butonu.
- **Dil** seçeneği Görünüm'den **Profil** kategorisine taşındı (uygulama-geneli
  kalır). Detay: `_Docs/06-WORKSPACES.md`.

## Composer otomatik-büyüyen giriş + araç çubuğu düzeni ✅ (2026-06-25)

Sohbet giriş alanı (`Composer.tsx`) yeniden düzenlendi. Eskiden tüm kontroller
(ajan seçici, düşünme/izin picker'ları, çalışma dizini, ekle, textarea, gönder)
**tek satırda** `items-end` ile yan yana duruyordu; textarea büyüdüğünde yanındaki
butonlar da uzayıp düzeni bozuyordu. Ayrıca textarea otomatik büyümüyordu
(`rows=1` + `max-h-40`, JS yok).

Yeni tasarım **kart + iki satır**: (1) tam-genişlik **textarea** üstte, (2) altta
sabit yükseklikli **araç çubuğu** (sol = ajan/picker/dizin/ekle, sağ = gönder
kümesi, arada `flex-1` boşluk). Kenarlık/odak halkası artık kartta
(`focus-within:border-accent`). **Otomatik büyüme:** `useLayoutEffect` her
`text` değişiminde `height='auto'`→`scrollHeight` ile textarea'yı içeriğe göre
büyütür; `max-h-[5.5rem]` (~3 satır) sınırlar, üstünde iç kaydırma açılır →
butonlar **asla** itelenmez/uzamaz. Dosya: `frontend/src/components/chat/Composer.tsx`.

## Skill koleksiyon içe aktarma (SK-IMP2) ✅ (2026-06-25)

İçe aktarma artık **tek skill klasörü** yerine **çok-skilli koleksiyonları** (GitHub repo /
Claude Code plugin / `skills/` klasörü) destekler. Hedef: `juliusbrussee/caveman`,
`leonxlnx/taste-skill`, `coreyhaines31/marketingskills` gibi repoların ve
crossaitools.com / skillsmp.com / claudeskillsmarket.com dizinlerinin işaret ettiği
GitHub-tabanlı skill'lerin toplu eklenmesi.

- **Tek-tarball indirme:** `internal/skills/collection.go` repo'yu `codeload.github.com/
  …/tar.gz/<ref>` üzerinden **bir** istekle çeker (`main`→`master` fallback), `archive/tar`
  +`compress/gzip` ile bellek-içi ayıklar (**yeni bağımlılık yok**, go.mod hâlâ uuid+cron).
  Contents-API klasör-gezmesine göre anonim rate-limit'e çok daha dostu (1 istek = keşif+içerik).
- **Keşif** (`discoverInTree`): her `SKILL.md`'nin ebeveyni skill klasörü; dosyalar **en derin**
  ata-skill'e atanır → nested sub-skill kendi kaynaklarını korur. Kök-skill + URL alt-yol
  prefix filtresi desteklenir. `skipDirs` ile repo gürültüsü (.git/.github/dist/…) elenir;
  4 MB/dosya, 64 MB/toplam cap.
- **Nested kaynaklar korunur:** `ImportCCSkill` artık `references/`/`evals/`/`scripts/` alt
  klasörlerini yazar (`safeBundledPath` mutlak yol + `..` reddeder, nested'a izin verir);
  `readLocalSkillDir` tek-skill local import'ta da ağacı `WalkDir` ile toplar.
- **YAML block-scalar açıklama** (`description: >` folded / `|` literal) parse edilir
  (`frontmatter.go::collectBlockScalar`) — caveman vb. community skill'lerinde yaygındı, eskiden `>` çıkıyordu.
- **API:** `POST /api/skills/import/scan` (önizleme) + `POST /api/skills/import/bulk`
  (seçili `paths[]` + `slugPrefix` + `shared`; çakışma batch'i durdurmaz, `Skipped`'a düşer).
  `owner/repo` kısayolu kabul edilir. Eski `/api/skills/import` (tek) korundu.
- **UI:** `SkillImportDialog` üç adımlı (**Tara → Seç → İçe aktar**): keşfedilen skill listesi,
  çoklu seçim + tümü/hiçbiri, "zaten var" rozeti, slug öneki, sonuç (içe aktarılan + atlanan + uyarılar).
- **Test:** `collection_test.go` (gruplama/prefix/kök-skill/çakışma/block-scalar; 29 birim test),
  `collection_live_test.go` (network-gated). **Canlı doğrulama:** caveman 11, taste-skill 13,
  marketingskills **45 skill + 155 nested dosya** sorunsuz içe aktarıldı.
- **Detay:** `_Docs\21-MARKET.md` §4.1.

## Generator↔Evaluator (GAN-benzeri) flow şablonu ✅ (2026-06-25)

Anthropic *"harness design"* makalesindeki **self-evaluation problemi** (ajan kendi işini
körü körüne över) için sözleşmeli **generator↔evaluator** iterasyon döngüsü: işi yapan
generator ile yargılayan **ayrı, şüpheci** evaluator; sprint contract + gerçek Playwright
testi + kod-konumlu bug raporu + skora göre **refine/pivot**. **Yeni motor kodu yok** —
mevcut node tipleriyle, döngünün (cycle) bilinçli kullanımıyla kuruldu.

- **Motor teyidi:** `orchestration.Validate()` acyclicity kontrol **etmiyor**, engine döngüye
  izin verip `maxSteps=50` ile sınırlıyor → `evaluate → decide → generate` geri-kenarı
  doğrudan kurulabiliyor. (Eski `swarmgo-flows` skill'i "must be acyclic" diyordu — **yanlıştı**,
  düzeltildi.)
- **Kısıt → karar:** branch yalnız string eşler (sayısal eşik yok) → skor→pivot kararı
  **keyword verdict** (`VERDICT: SHIP|REFINE|PIVOT`, `decide` `matchMode:regex` son satıra
  demirli); döngüde graf çıktısı üzerine yazıldığından skor **trend'i** `core:sprint-scorelog`
  çekirdek belleğe append edilir, sözleşme `core:sprint-contract`'ta yaşar.
- **Dağıtım:** gömülü gallery şablonu `gan-loop` (`frontend/src/lib/flowTemplates.ts`) +
  market paketleri `flow.gan-generator-evaluator` / `agent.skeptical-evaluator` /
  `mcp.playwright` (global market dizinine yazıldı) + yeni default skill
  `swarmgo-gan-loop`. Şablon agent-bağımsız → kurulumdan sonra **iki ayrı ajan** atanır.
- **Test:** `engine_test.go` — `TestValidate_AllowsCyclicGraph`, `TestRun_GANLoop_RefinesThenShips`
  (2× REFINE → SHIP → finalize), `TestRun_GANLoop_StepCapBackstop` (hiç ship etmeyen →
  `step cap` hatası). `go build`/`vet`/`test ./internal/orchestration` ✅; `tsc -b`/`vite build` ✅.
- Detay: [`15-FLOW-CANVAS.md`](15-FLOW-CANVAS.md) §Generator↔Evaluator döngü şablonu;
  kullanım kılavuzu: `swarmgo-gan-loop` skill.

## Yapılandırılmış subagent görev sözleşmesi ✅ (2026-06-25)

Anthropic *"Multi-agent research system"* rehberi: her subagent'a **objective +
output format + tool/source guidance + boundaries** verilmezse iş tekrarı/boşluk
oluşur. SwarmGo'da `run_subagent` yalnız serbest-metin `task` alıyordu; bu 4 alanı
yapısal teşvik etmiyordu.

- **Şema:** `run_subagent` input'una **üç opsiyonel alan** eklendi — `objective`,
  `output_format`, `boundaries` (`tools/subagent.go`: `runSubagentInput` + `RunAgentSpec`
  + JSON şema + `Call()` parse). `target`+`task` hâlâ tek zorunlu çift.
- **Enjeksiyon:** `agent/subagent.go::delegationContract()` dolu alanlardan bir
  **"Task contract"** bloğu üretir, `runAgent()` bunu subagent system-prompt'una
  persona'dan **sonra** ekler (`req.System`). Hiçbir alan yoksa blok boş → eski düz-`task`
  davranışı bayt-bazında korunur. Native + CLI köprüsü ikisi de `runAgent`'tan geçtiği
  için tek nokta yeterli. Açık karar: sözleşme şu an yalnız **sync** dalda (async →
  `SpawnSession`, ileride genişletilebilir).
- **Teşvik:** tool description + `input_examples`'a yapılandırılmış örnek eklendi →
  model 4 alanı doldurmaya yönlendirilir.
- **Test** `agent/subagent_test.go::TestDelegationContract` (boş→"", dolu→satırlar,
  tek-alan izolasyonu). `go build`/`vet`/`test ./...` ✅.
- Detay: [`25-SUBAGENT-ISOLATION.md`](25-SUBAGENT-ISOLATION.md) §Yapılandırılmış görev sözleşmesi.

## Kalıcı todo / PROGRESS dosyası ✅ (2026-06-25)

Anthropic *"Effective harnesses for long-running agents"* + *"Effective context
engineering"* makalelerindeki **kalıcı not dosyası** (`claude-progress.txt` +
`feature_list.json` `passes` boolean) konvansiyonu SwarmGo'ya getirildi. `todo_write`
listesi artık **diske kalıcı**: oturumlar arası kaybolmuyor, yeni oturum devralıyor.

- **Sorun:** `todo_write` stateless'tı; liste yalnız oturum-içi (mesaj trace'inden
  `todoContextBlock` ile yeniden inşa) yaşıyordu. Oturum restart/yeni oturum/ajan
  değişiminde kayboluyordu. Core memory (serbest persona/human) bunu karşılamıyor.
- **Çözüm:** Liste, çalışma dizinine bağlı **`<cwd>/.swarmgo/progress.json`**'a
  yazılır (cwd yoksa `<store>/progress/<agentID>/`); fresh oturum açılışında
  geri yüklenip "Resumed progress" bloğu olarak `SystemDynamic`'e enjekte edilir.
  `completed` ≡ Anthropic `passes:true`. Rolling `log` = `claude-progress.txt`.
- **Yeni paket** `internal/progress` (atomik JSON oku/yaz). **Sink** ArtifactSink
  deseninin ikizi: `tools/todosink.go` (ctx) + `agent/todosink.go` (`NewTodoSink`).
  Native (`toolloop.go` fallback) + chat (`chat_stream.go`) + CLI
  (`mcp_interaction.go callTodo` + `chat_control.go` run sink + autonomous) yolları.
- **Geri yükleme** `api/todos.go::todoContextBlock` (cwd+agentID+resume); fresh
  oturumda diskten devralır (`renderResumedBlock`). `agent/workdir_ctx.go` →
  `SessionWorkdir`; `db.DB.Root()`.
- **Ayar** `progressPersist`/`progressResume` (vars. açık) — settings/DTO/Patch +
  `Tunables.SetProgress` + `applySettings` + UI (Ayarlar ▸ Bağlam ▸ "Kalıcı
  ilerleme"). Migration yok, opt-in (kapalı → eski efemeral davranış).
- **Test** `progress` (round-trip/trim/missing), `agent` (cwd+store fallback),
  `tools` (sink çağrısı/no-op/hata-yutma), `api` (resumed block/progressDir). `go
  test ./...` ✅, `go vet` ✅, frontend build ✅.

**İkinci tur (aynı gün) — zenginleştirmeler:**
- **`feature_list` zenginliği:** `todo_write` öğelerine opsiyonel `category` +
  `steps` (Anthropic feature_list paritesi); şema + `TodoSinkItem` +
  `progress.TodoItem` + sink mapping uçtan uca taşır (`omitempty`).
- **`swarmgo-progress` default skill'i:** ajana otomatik progress.json + insan-okunur
  `PROGRESS.md` konvansiyonunu öğretir (`internal/skills/defaults/swarmgo-progress/`;
  `//go:embed` ile otomatik, baseline skill setine girer).
- **UI görüntüleyici:** `GET /api/sessions/{id}/progress` (`api/progress.go`) +
  `SessionDetailPanel` "Kalıcı ilerleme" salt-okunur kartı (`ProgressCard` — statü
  işaretçili maddeler + category + son log satırları). `go test ./...` 438 ✅.
- Detay: [`36-KALICI-ILERLEME.md`](36-KALICI-ILERLEME.md).

## Otonom turda boot-verification sırası ✅ (2026-06-25)

Anthropic *"Effective harnesses for long-running agents"* makalesindeki **standart
oturum açılış sırası** (yönelim → hatırlama → tek görev seç → temel testi doğrula →
işi yap → döngüyü kapat) SwarmGo'nun otonom turlarına getirildi. Kayıp bağlamı telafi
eden, düşük-riskli, çoğunlukla skill+doküman değişikliği.

- **Skill reçetesi (ana iş):** `swarmgo-autonomous-ops/SKILL.md` → yeni **§10 "The
  autonomous boot sequence"** (Step 0 Orient → Step 5 Close); referans setup'a `0.` adımı
  ve Pitfalls'a "Skipping the boot sequence" maddesi. Reçete `.swarmgo/progress.json`
  (progressPersist) + `list_tasks` (append-only board) + git log'u "hafıza" olarak
  okur; kapanışta git commit + append-only not. One-task-per-run + append-only kullanıcı
  tercihiyle hizalı.
- **Minimal kod kancası:** `runtime.go autonomousSystemPrompt` artık `autonomousBootReminder`
  (6 satırlık, skill'e yönlendiren pointer) enjekte eder — tek noktadan **dört otonom yol**
  (scheduler/spawn/flow/subagent; `executor.go`+`subagent.go` ortak kurucu). Chat turları
  (composeTurnRequest) etkilenmez. Pointer-only → cache'li statik prefix şişmez.
- **Gate:** `autonomousBootSeq` ayarı (vars. **true**) — `settings.go` (struct+default+DTO+patch)
  + `store.go` apply + `tunables.go` (alan/default/`SetWorkdirGuards`/`AutonomousBootSeq()`)
  + `server.go applySettings`. Eski config'lerde `Default()` backfill'i ile true kalır.
  Frontend: `types/settings.ts` + `SettingsPanel.tsx` payload + `appPanels.tsx` toggle
  ("Otonom boot doğrulama sırası").
- **Test:** `bootseq_test.go` (gate default+SetWorkdirGuards köprüsü + reminder içeriği);
  `go build ./...` + `go test` (159 vaka: agent/settings/api) + `tsc --noEmit` yeşil.
- İlişki: Görev #2 kalıcı PROGRESS (`progressPersist`/`progressResume` zaten mevcut) bu
  reçetenin Recall/Close adımlarını besler; dosya yoksa git log + board'a düşer.
- Detay: `_Docs\33-DIS-AJAN-OTOMASYONU.md` §Otonom Boot Sırası.

## Context-rot farkındalığı + adaptif bütçe stratejisi ✅ (2026-06-25)

Anthropic *Effective context engineering* makalesi: token arttıkça recall hassasiyeti düşer ("context rot", `n²` dikkat ilişkisi → **performans gradyanı**, uçurum değil). SwarmGo'nun önceki "her şeyi ham tut" bahsi (512K/0.6) bu rot ile bilinçli bir takastı. Dayanıklılığın aslında **retrieval katmanında** (memory/`conversation_search`/core blocks) olduğu, ham pencere boyutunda olmadığı tespit edildi → ham pencere küçültülebilir, recall kaybetmeden.

- **Adaptif fraction:** `providers.AdaptiveBudgetFraction(provider, model)` — `ContextWindowFor`'un aile sınıflamasını yeniden kullanır; Opus/Sonnet 0.45, Haiku/Fable 0.40, MiniMax/DeepSeek/Gemini 0.35, bilinmeyen 0 (caller fallback).
- **Yeni semantik:** `ContextBudgetFraction = 0` → **otomatik/adaptif** (pozitif = manuel sabit). `EffectiveBudget` `fraction<=0`'da adaptif tabloyu kullanır; `Manager.SetBudgetShape` artık 0'ı (auto) saklar; `store.go` validate 0'ı korur (negatif → 0).
- **Yeni varsayılanlar:** `ContextBudgetCeil` 512K→**256K** (`262144`), `ContextBudgetFraction` 0.6→**0 (auto)**, `memoryPressureWarn` 0.75→**0.70** (`settings.go`+`tunables.go`). Eski `0.6` persisted değer manuel sabit olarak yaşar; yeni kurulum adaptif başlar.
- **Frontend:** Ayarlar▸Bağlam "Pencere oranı"/"Bütçe tavanı" hint'leri auto+rot açıklamasıyla güncellendi (`appPanels.tsx`).
- **Test:** `budget_test.go` (`TestEffectiveBudgetAdaptive`) + `context_window_test.go` (`TestAdaptiveBudgetFraction`); `go build ./...` + `go test ./internal/conversation ./internal/providers ./internal/settings` yeşil (83 test).
- **Doküman:** `_Docs\17` yeni **§12** (takas analizi + strateji + tablolar + mermaid) + §7 çapraz-referans; `swarmgo-settings` skill + `swarmgo-project` skill güncellendi.

## Context Reset + Handoff Artifact ✅ (2026-06-25)

Anthropic "harness design for long-running apps" bulgusu: in-place compaction tek başına **"context anxiety"**yi (model limite yaklaşınca erken toparlama) çözmez. Çözüm = **context reset** + **handoff artifact**: pencereyi özetlemek yerine, devamı taşıyan bir handoff dosyası yazıp **temiz bir oturumda** sürdür. SwarmGo'da önceden yalnız in-place rolling-summary vardı; bu, onun opt-in tamamlayıcısı.

- **Çekirdek:** `internal/conversation/handoff.go` (`handoffPrompt` 10-bölüm + DONE/TODO + Next Step, `HandoffEnv`, `BuildHandoff` — compaction çekirdeğini `KindCompact` ile yeniden kullanır) + `internal/agent/handoff.go` (`HandoffSession`: üret→artifact yaz→(ops.) `<workdir>/.swarmgo/handoff.md`→`SpawnSession(ParentSessionID)` ile taze oturum→tombstone; `maybeAutoHandoff`/`handoffChainDepth`/`handoffEnv`/`buildContinuationPrompt`).
- **Üç tetik:** manuel `/handoff` (`POST /api/sessions/{id}/handoff` → `summary.go handleSessionHandoff`); ajan aracı `handoff_session` (`tools/builtin_handoff.go`, self-manage gated, `toolsetup.go` kapanışı); **otomatik** (yalnız otonom tur — `runSpawn`/`deliverPrompt` tur-sonu; overflow sinyali `callkind.go withOverflowFlag`/`markContextOverflow`, tetik `toolloop.go` reactive compaction'da).
- **Ayarlar:** `HandoffAuto` (vars. **kapalı**) / `HandoffPressure` (0.90) / `HandoffMaxChain` (20) / `HandoffWriteFile` (kapalı) — `settings.go`+`store.go` clamp + `server.go applySettings → tun.SetHandoff`; UI Ayarlar▸Bağlam "Context reset (handoff)" bölümü. Tunables `DefaultHandoffPressure`/`DefaultHandoffMaxChain`.
- **Soyağacı:** `db.Session.ParentSessionID`/`HandoffArtifactID` (+`SetSessionHandoffArtifact`); `GET .../info` döner; UI `SessionDetailPanel` "↩ Devraldığı oturum" tıklanır link + `useChatStream` `/handoff` komutu yeni oturuma geçer.
- **Test:** `conversation/handoff_test.go` (env+transcript enjeksiyonu, boş-transcript guard) + `agent/handoff_test.go` (chain-depth, `maybeAutoHandoff` no-op yolları, continuation prompt, title). `go build`/`vet` temiz; `go test ./internal/conversation ./internal/agent` yeşil (mevcut paralel `TestEffectiveBudget` WIP'i hariç). Frontend `tsc`+`vite build` yeşil.
- claude-cli `--resume` ile uyumlu (reset zaten cold yeni oturum açar). Detay: **`_Docs\35-CONTEXT-RESET-HANDOFF.md`**.

## Market — katman sadeleştirme: bundled + workspace tier'ları kaldırıldı ✅ (2026-06-25)

Market pack katmanları üçten (bundled/global/workspace) **bir yerele** (global) indirildi; uzak registry 4. kaynak olarak kalır.

- **Kaldırılanlar:** `internal/market/defaults.go` (`//go:embed defaults`) + `internal/market/defaults/` klasörü + `EnsureDefaults` çağrısı (runtime.go) + workspace pack tier'ı. Binary artık market item taşımaz, workspace'te `market/` klasörü oluşmaz.
- **`market.New(globalDir, ledgerDir)`:** tek yerel tier = global; `Publish` global dizine yazar; install ledger (`installed.json`) per-workspace **kökte** (eski `<workspace>/market/` yerine; `workspaceLedgerDir`). Store'da `writeDir` → `globalDir`+`ledgerDir` ayrımı.
- **Mevcut paketler korundu:** 28 başlangıç paketi zaten global dizinde (`~/.swarmgo/market`); silinmedi. Yeni kurulumlarda market boş başlar → global'e elle paket konur veya uzak registry eklenir.
- **Test:** `store_test.go` `EnsureDefaults`'tan arındırıldı (global'e elle pack yazıp test eder) + ledger/semver testleri eklendi; `go build ./...` + `go test` (145, market/api/agent) yeşil. Çalışan instance global'den 28 paket (`source=global`) döndürüyor.
- Detay: `_Docs\21-MARKET.md` §3.1.

## Skill grupları — katlanabilir (fold in/out) gruplama ✅ (2026-06-25)

Skiller artık serbest-metin bir **`group`** etiketiyle organize edilebiliyor; Skills
ekranında aynı gruptaki beceriler **katlanabilir başlık** altında toplanıyor. Tamamen
kozmetik — çözümleme/reklam/yükleme davranışını etkilemez.

- **Frontmatter (`group`/`category` alias):** `internal/skills/skill.go` `Skill.Group`,
  `store.go scanDir` parse eder; `SkillInput.Group` + `fields()` → `Create`/`Update` diske
  yazar. Import yolu dosyayı verbatim kopyaladığından grup korunur.
- **API:** `skillInputReq.Group` (`internal/api/skills.go`) → create/update gövdesinde geçer;
  `Skill` JSON'una `group,omitempty` eklendi.
- **Self-management araçları:** `create_skill`/`update_skill` artık `group` parametresi alır
  (`builtin_skillmgmt.go`); `SkillWriter` arayüzü + `agentSkillWriter` adapter (`runtime.go`)
  güncellendi (partial update'te `cur.Group` korunur, aksi halde silinirdi). Mock + test güncellendi.
- **UI (`SkillsPanel.tsx`):** `groupSkills()` listeyi gruba göre kovalar (adlandırılmış gruplar
  alfabetik, "Grupsuz" en sonda); her grup `ChevronDown/Right`'lı, sayaç rozetli katlanabilir
  başlık. Katlı gruplar `localStorage` (`swarmgo.skillsCollapsedGroups`) ile kalıcı. `SkillEditor`
  grup input'u + mevcut gruplardan `datalist` önerisi; detay başlığında grup rozeti.
- **Build/test:** `go build ./...` + `go test ./internal/skills ./internal/tools` ✅, frontend `tsc` ✅.

### Takip iyileştirmeleri (aynı gün)

- **İlişki grafiğinde grup tinti:** `api/graph.go` skill düğümüne `Sub = sk.Group` ekler (store'dan
  bakılır); `relationGraph.ts` `groupHue()` (etiket→deterministik HSL) ile aynı gruptaki skill
  yıldızlarını **ortak renkle** boyar, grupsuzlar sarı kalır; tooltip "Skill · <grup>" gösterir.
- **"Tümünü katla/aç":** SkillsPanel header'ında (>1 grup varken) `ChevronsDownUp`/`ChevronsUpDown`
  butonu — tüm grupları tek tıkla katlar/açar (`toggleAll`, `allCollapsed` türetimi).
- **Market:** skill paketi zaten tam `SKILL.md` gövdesini (`SkillPayload.Body`) taşıdığından
  `group` install/publish ile **kendiliğinden korunuyor** — değişiklik gerekmedi.
- **Git hijyeni:** `.gitignore`'a `*.log.err`/`*.err`; sızan `vite-run.log.err`/`swarmgo-run.log.err`
  izlemeden çıkarıldı (`git rm --cached`).

## Bütçe refactor faz 3 — RollupOf birleştirmesi (costOf+modelRowsFor → billing) ✅ (2026-06-25)

Pricing aggregation tümüyle billing'e taşındı, iki fonksiyon tek primitife indi.

- **`billing.RollupOf(byModel) Rollup`:** eski `costOf` + `modelRowsFor` çiftinin birleşik çekirdeği —
  her satırı fiyatlar (`PriceStat`), maliyete göre sıralar (input+output tiebreak) ve toplam
  cost/savings/cache + priced/estimated bayraklarını döner. Yeni `billing.Row` (provider/model/Stat +
  cost/save/priced/estimated) DTO-bağımsız.
- **api sadeleşti:** `costOf` silindi; `modelRowsFor` artık `RollupOf` çıktısını `modelStat` DTO'ya
  haritalayan ince adaptör. Cost-only çağıranlar (per-agent satır, günlük trend) doğrudan `RollupOf`
  okur. Per-agent provider/model aggregation `roll.Rows`'u tekrar kullanır → **çift fiyatlama kalktı**
  (eskiden u.ByModel ikinci kez PriceStat'tan geçiyordu). `strings` importu budget.go'dan düştü.
- **Test/build:** `billing_test.go`'ya `TestRollupOf` (sıralama + toplam + priced bayrağı); `go build
  ./...` + `go vet` + `go test ./...` → **407 passed (29 paket)**.

## Bütçe refactor faz 2 — UsageDelta helper + tokenTotals embed + billing paketi ✅ (2026-06-25)

Önceki refactor'un devamı; 3 ek sadeleştirme, davranış korundu.

- **`db.DeltaFromUsage(calls, providers.Usage)`:** `providers.Usage → db.UsageDelta` dönüşümü
  `RecordUsage` (agent) ve `recordCompaction` (conversation)'da elle kuruluyordu → tek mapping
  noktası `db`'de (yeni `db→providers` importu; döngü yok, providers leaf). Yeni token sınıfı
  eklenince tek yer güncellenecek.
- **`tokenTotals` gömülü struct (`api/budget.go`):** `modelStat`/`providerStat`/`dayPoint` aynı 5
  sayaç alanını (Calls/In/Out/CacheR/CacheW) tekrar ediyordu → anonim embed; alanlar promote olup
  inline marshal edilir, **JSON şekli birebir aynı** (alan adları korundu). `kindStat` ayrı kaldı
  (cache alanı yok). modelRowsFor literal'i `tokenTotals: {...}` biçimine güncellendi.
- **`internal/billing` paketi:** pricing domain matematiği (`priceStat` → `billing.PriceStat`) api'den
  çıkarıldı → api yalnız JSON şekillendirir, billing maliyet/tahmin mantığını taşır (db+providers
  import eder). Test `billing/billing_test.go`'ya taşındı (4 vaka).
- **Build/test:** `go build ./...` + `go vet` + `go test ./...` → **405 passed (29 paket; +billing)**.

## Bütçe + context refactor — fiyatlama/summarize/fold tekrarları sadeleştirildi ✅ (2026-06-25)

Davranış değiştirmeden 3 tekrar noktası generic'leştirildi (golden testlerle kilitlendi).

- **Tek fiyatlama primitifi (`api/budget.go priceStat`):** "PriceFor→CostDetailed/CacheSavings,
  yoksa EstimateFor" dallanması 3 yerde (costOf, modelRowsFor, handleWorkspaceUsage inline döngü)
  kopyalanmıştı → tek `priceStat(provider, model, st) (cost, save, priced, estimated)` çekirdeği;
  hepsi buradan geçiyor (~60 satır tekrar gitti). Harmonizasyon: sıfır-token slice artık `priced=true`
  (eskiden inline döngüde `false` olabiliyordu — gerçek harcaması olmayan satır "unpriced" işaretlenmez).
- **Tek summarize çekirdeği (`conversation/`):** `Manager.summarize` (db.Message) ve reactive'in
  `summarizeProviderMessages` (providers.Message) aynı provider-call+recordCompaction+trim bloğunu
  taşıyordu → ortak `summarizeRendered(existing, rendered)` + iki ince renderer
  (`renderDBMessages`/`renderProviderMessages`). `reactive.go`'dan `fmt` importu düştü.
- **Fold-sınırı yardımcısı (`conversation/manager.go`):** `Prepare` ve `ForceCompact`'taki
  `start/pending/fold` deseni → `foldBoundary(history, start, keepRecent) (fold, keepTail, newCount, ok)`
  + `clampStart`; `ForceCompact` artık "bütçesiz Prepare".
- **Test/build:** `conversation/manager_test.go` (foldBoundary/clampStart) + `api/budget_test.go`
  (priceStat 4 vaka) eklendi; `go build ./...` + `go test ./...` → **405 passed (28 paket)**.

## OpenRouter geçmiş cache breakpoint'i + per-model cache fiyatlandırması ✅ (2026-06-25)

Önceki cache çalışmasının üstüne iki iyileştirme.

- **Mesaj geçmişi breakpoint'i (`minimax.go attachHistoryBreakpoint`):** OpenRouter'da System
  prefix'inin yanına **2.** bir `cache_control` transkriptin sonundaki son düz-metin mesaja konuyor
  (tool_call taşıyan ve system mesajı atlanır). Böylece System + **tüm sohbet geçmişi** cache'lenir,
  yalnız en yeni mesaj taze gider (2/4 breakpoint). Uzun sohbetlerde token maliyetini ciddi düşürür;
  ilk tur cache-write, sonraki turlar cache-read.
- **Per-model cache çarpanı + OpenRouter fiyatları (`pricing.go`):** `Price`'a
  `CacheReadMultOverride`/`CacheWriteMultOverride` eklendi (0 → paket varsayılanı 0.10/1.25).
  `CostDetailed`/`CacheSavings` artık `p.cacheReadMult()`/`cacheWriteMult()` kullanıyor. `priceTable`'a
  **`openrouter`** bölümü: anthropic-routed modeller (opus/sonnet/haiku) Anthropic pass-through fiyatı +
  0.10/1.25 cache tier'ı ile; OpenAI/DeepSeek-routed için 0.25 override eklenebilir (yorumda örnek).
  Listelenmeyen openrouter modelleri unpriced (ekran ballpark).
- **Önizleme:** `computeCachePreview` openrouter modunda `cachedMsgCount = msgCount-1` (geçmiş yeşil,
  son mesaj taze) + güncel note.
- **Test/build:** `pricing_test.go` (override + openrouter fiyat) + `minimax_test.go` (geçmiş
  breakpoint) eklendi; `go test ./internal/{providers,api}/` 120 passed, `go build ./...` + frontend
  `tsc -b && vite build` yeşil.

## OpenAI-uyumlu yol prompt-cache — minimax/openrouter cache ölçümü + OpenRouter breakpoint ✅ (2026-06-25)

OpenAI-uyumlu sağlayıcılar (minimax, openrouter, özel OpenAI-compat) artık cache'i hem **ölçüyor**
hem de OpenRouter'da **istiyor**.

- **Ölçüm (`oaiUsage.toUsage()`):** usage objesinden prompt-cache token'ları ayrıştırılıyor —
  `prompt_tokens_details.cached_tokens` (OpenAI/OpenRouter) + `prompt_cache_hit_tokens` (MiniMax) →
  `Usage.CacheReadTokens`. Cached token `prompt_tokens`'in alt kümesi olduğu için **InputTokens'tan
  düşülüyor** (Anthropic konvansiyonu; `pricing.CostDetailed` cache read'i input üstüne ekler →
  çift sayım önlendi). `cache_creation_input_tokens` → `CacheWriteTokens` (ayrı sayım, düşülmez).
  Hem `Complete` hem `Stream` yolunda. → Usage/Budget ekranı minimax/openrouter cache tasarrufunu
  gösterir (eskiden hep 0'dı).
- **OpenRouter breakpoint (`buildSystemMessage` + `cachesSystem()`):** OpenRouter'da statik System
  prefix'ine `cache_control: {type: ephemeral}` konuyor (system content artık parça-dizisi:
  statik+breakpoint, dinamik breakpoint'siz — native anthropic `systemField` ile birebir). Anthropic/
  Gemini backend'lerinde cache'li; OpenAI/DeepSeek zaten otomatik. `oaiMessage.Content` `any` oldu;
  diğer endpoint'ler (MiniMax/Groq/Ollama) düz string content korur (array-form reddini önler).
- **Önizleme:** `computeCachePreview`'a `openrouter` modu eklendi → System/Tools cache'li gösterilir.
- **Test/build:** `minimax_test.go`'ya cache-parse (4 vaka: openai-details/minimax-toplevel/write/none) +
  OpenRouter breakpoint + MiniMax string-content testleri; `go test ./internal/{providers,api}/` 117 passed,
  `go build ./...` yeşil.

## Yedek arşivlerini UI'dan listeleme + tek-tık geri yükleme ✅ (2026-06-25)

Yedekleme özelliğine **geri yükleme** eklendi (önce yalnız alma vardı).

- **Backend liste/çözümleme (`internal/backup/restore.go`):** `Manager.ListArchives(targets)`
  (workspace başına arşiv, en yeni önce — ad/bayt/mtime), `Manager.ResolveArchive(wsID, name)`
  (ad doğrulama: yol ayıracı/`..` reddi → path-traversal guard). `Unzip` (`archive.go`,
  zip-slip korumalı, exported).
- **Lifecycle restore (`workspace.Manager.RestoreFromArchive(id, path, extract)`):** workspace'i
  ayır (scheduler `Stop`/runtime `CloseMCP`/DB `Close`) → arşivi **staging**'e aç → üst-düzey
  girdileri (store/config/workspace + ws-settings.json) `rename` ile **swap**'le → `open(meta)`
  ile diskten yeniden aç. Açma hatasında rollback (workspace dokunulmadan reopen). `extract`
  enjekte (`backup.Unzip`) → `workspace` paketi backup formatına bağımsız.
- **API:** `GET /api/backups/archives` + `POST /api/backups/restore` (`{workspaceId, archive}`;
  başarıda `publishWorkspacesChanged` → UI listeleri tazelenir).
- **UI (`BackupPanel`):** "Mevcut yedekler (N)" bölümü — workspace başına gruplu arşiv listesi
  (ad + tarih + boyut) + her arşivde **iki-adımlı onaylı "Geri yükle"** + yıkıcı işlem uyarısı.
  Tipler `BackupArchiveFile`/`WorkspaceArchives`, api `listBackupArchives`/`restoreBackup`.
- **Koşullu otomatik yenileme:** geri yüklenen workspace o pencerede **aktifse** sayfa otomatik
  yenilenir (`getActiveWorkspace()===id` → `window.location.reload()`); başka workspace'te yenileme
  yapılmaz (o workspace'e geçince zaten taze yüklenir → alakasız reload yok).
- **Ayrı "Yedekleme" sayfası (2026-06-25):** önce Ayarlar ▸ Gelişmiş altında alt-bölümdü; ayarlar +
  arşiv/geri-yükleme tek yerde toplansın diye **kendi kategori sayfasına** alındı (`primitives.tsx`
  `APP_CATS`'e `backup` katı + `Archive` ikonu, "Gelişmiş" ile "Komutlar" arası; `SettingsPanel.tsx`
  `cat==='backup'`). Üstteki ortak Kaydet butonu config'i yazar.
- **Arşiv silme (2026-06-25):** `Manager.DeleteArchive(wsID, name)` (ResolveArchive guard + `os.Remove`),
  `DELETE /api/backups/archives`, api `deleteBackupArchive`; UI her arşiv satırında **iki-adımlı onaylı**
  🗑 "Sil" butonu (Geri yükle'nin yanında). Test `TestDeleteArchive` (geçerli sil + traversal reddi);
  canlı API+UI: WS1/WS2 5→4 (disk+liste). `go test ./internal/backup` 5/5 yeşil.
- **Test/doğrulama:** `go test ./internal/backup` (4: +round-trip, +traversal reddi) yeşil; canlı
  WS1 (5 ajan/26 oturum) ve WS2 (9 ajan/11 oturum) API+UI'dan geri yüklendi → veri korundu,
  server sağlıklı. Detay: `_Docs\34-YEDEKLEME.md`.

## Workspace yedekleme — periyodik zip snapshot + saklama + manuel tetik ✅ (2026-06-25)

Her workspace'in tüm veri dizini (`store/`, `config/`, `workspace/`, `ws-settings.json`)
belirli aralıklarla bir zip arşivine alınır; saklanan sayı aşılınca eskiler budanır.
Süreç-geneli tek `backup.Manager` (workspace'ten bağımsız), ayarlardan canlı yapılandırılır.

- **Yeni paket `internal/backup`:** `backup.go` (Manager: `Configure`/`RunOnce`/`Stop`/`Status`,
  ticker döngüsü + run-mutex ile çakışma engeli + per-workspace saklama budama `prune`),
  `archive.go` (`zipDir` — göreli yol korumalı, backups kökünü dışlayıp özyinelemeyi önler).
  İlk otomatik yedek bir **aralık sonra** alınır (restart başına yedek patlaması yok).
- **Workspace köprüsü:** `workspace.Manager.BackupTargets()` her workspace'in mutlak veri
  dizinini döndürür (`open()` ile aynı yol mantığı: kullanıcı `Path`'i ya da varsayılan
  `rootDir/workspaces/<id>`).
- **Ayarlar (`settings`):** `backupEnabled` (vars. false), `backupIntervalHours` (≥1, vars. 24),
  `backupRetain` (≥1, vars. 7), `backupDir` (boş → `<dataDir>/backups`). Settings struct/DTO/Patch/
  Default/ToDTO/Apply + `normalize` clamp'leri eklendi.
- **Wiring:** `app.Bootstrap` manager'ı kurup `server.SetBackupManager` ile bağlar; `applySettings`
  her ayar değişiminde `backups.Configure(...)` çağırır (canlı başlat/durdur/yeniden-yapılandır);
  `App.Shutdown` döngüyü durdurur.
- **API:** `GET /api/backups` (durum: config + son koşu) + `POST /api/backups/run` (anında yedek,
  zamanlama açık olmasa da çalışır). Ajan aracı **değil** — yalnız kullanıcı/UI.
- **UI:** Ayarlar ▸ Gelişmiş ▸ **Yedekleme** bölümü (`BackupPanel`, `appPanels.tsx`): aç/kapat +
  aralık/saklama/klasör alanları + canlı durum kartı + **"Şimdi yedekle"** butonu. Tipler
  `types/settings.ts` (`BackupStatus`/`BackupResult`), api `system.ts` (`getBackupStatus`/`runBackup`).
  Tek manager **tüm** workspace'leri yedeklediği için app-geneli ayardır (workspace'e özel değil).
- **Build/test:** `go build ./...` + `go test ./internal/backup` (2: arşivle+budama, backups-kökü
  dışlama) yeşil; frontend `npm run build` (dist gömüldü). Detay: `_Docs\34-YEDEKLEME.md`.

## Market — uzak kayıt defteri (remote registry) + sürüm/güncelleme + detay popup ✅ (2026-06-25)

Market harici sunuculardan paket çekebilen 4. tier'a kavuştu (Faz 1-4, doğrulama opsiyonel).

- **Index formatı `swarmregistry/v1`:** uzak sunucu tek `registry.json` sunar (manifest + payload `url` + opsiyonel `sha256` + `minAppVersion`). Payload kurulum anında `url`'den lazy indirilir; sha256 verildiyse doğrulanır, yoksa atlanır.
- **Backend:** `internal/market/remote.go` (fetchIndex/fetchPayload, http(s)-only, boyut limiti 8/4 MiB, 20sn timeout, `compareVersions` semver-lite), `registry_store.go` (kaynak config `registries.json` global + index cache `.remote-cache/` restart-safe + per-workspace install ledger `installed.json`), `store.go` List/Get yerel+uzak birleştirir (`Source=remote`, id çakışmasında yerel gölgeler), `New` artık globalDir saklar. Pack'e `RegistryName`/`InstalledVersion` (+ transient `remoteURL`/`remoteSHA`).
- **API:** `GET/POST /api/market/registries`, `POST .../delete`, `POST .../refresh`; `GET /api/market` her pack'i ledger'dan `installedVersion` ile dekore eder; install başarısında `statusCaptureWriter` ile ledger'a `version` yazılır (tüm türler için tek nokta).
- **UI:** `RegistryManager.tsx` modal (kaynak ekle/sil/yenile, `market-registries-modal`); `MarketPanel.tsx`'e "Kaynaklar" butonu, kaynak rozeti (Yerel/registry adı), **"Güncelle (vX→vY)"** rozeti + detay popup'ında güncelleme butonu (overwrite). "Yenile" artık önce uzak index'leri çeker. Ayrıca **detay paneli yan-panelden ortada popup'a** çevrildi (`market-detail-modal`).
- **Doğrulama (uçtan uca):** yerel HTTP'de registry yayınlandı → ekle → uzak pack `source=remote` listede → kur (indirildi, skill oluştu) → `installedVersion=2.0.0` → registry 3.0.0 + refresh → "güncelleme var"=true. Backend `go build`+`go test` (59) yeşil, `tsc --noEmit` temiz, `npm run build` + binary derlendi, çalışan instance'ta test edildi.
- Detay: `_Docs\21-MARKET.md` §7.

## Market — 3 yeni paket türü (workspace/memory/mcp) + sol kategori menüsü + import taşıma ✅ (2026-06-24)

Market 4 türden 7 türe çıkarıldı ve ekran yeniden düzenlendi. (Aynı gün kısa süre `board` türü de
eklendi ama **kaldırıldı** — workspace şablonu zaten opsiyonel kanban düzeni taşıyor; ayrı board paketi
gereksiz bulundu.)

- **Yeni türler (install çalışır):** `workspace` (yeni workspace oluştur: `Manager.Create`+`UpdateSettings`,
  opsiyonel kanban düzeni dahil), `memory` (**seçilen ajana** tohum: `Runtime.Memory().Remember`),
  `mcp` (`db.CreateMCPServer`, enabled → sonraki turda araçlar görünür). Backend: `internal/market/pack.go`'ya
  kind sabitleri + payload struct'ları (`WorkspacePayload`/`MemoryPayload`/`MCPPayload` + `BoardColumn`
  [workspace şablonunun kanban düzeni için] + `MemoryEntry`; market paketi db'ye bağımlı kalmasın diye
  `BoardColumn` ayrı, API katmanı `db.BoardColumnDef`'e map'liyor); install handler'ları
  `internal/api/market.go` (`installMCPPack`/`installWorkspacePack`/`installMemoryPack` +
  `toBoardColumnDefs`/`mcpNames` helper'ları).
- **Memory ajan seçimi:** install body'sine `agentId` eklendi; `installMemoryPack` verilen ajanı hedefler,
  boşsa ilk ajana düşer, bilinmeyen id → hata. UI'da detay panelinde **hedef ajan dropdown'u**
  (`market-memory-agent`); ajan yoksa kur butonu pasif.
- **Gömülü örnekler (+5):** `mcp.filesystem`, `mcp.fetch`, `workspace.software-project`,
  `workspace.research`, `memory.coding-standards` (`internal/market/defaults/`, `//go:embed` ile gömülü;
  `EnsureDefaults` eksikleri global market dizinine yazar). Board örnekleri (scrum/bug-triage) eklenip geri
  silindi; global market dizinindeki kalıntılar da temizlendi.
- **UI (`MarketPanel.tsx`):** üst sekmeler → **sol dikey kategori menüsü** (7 kategori + paket sayacı);
  "Tümü" kaldırıldı, ilk kategori (Skills) varsayılan. Yeni türler için `PackPreview` (workspace yönergeleri +
  kanban chip'leri, MCP komut/args, bellek girdileri), `INSTALL_LABEL`, `KIND_LABEL`. memory "zaten kurulu"
  işaretlenmez (eylem); workspace/mcp ad-bazlı dedup (`api.listWorkspaces`/`listMCPServers`).
- **Import taşıma:** Claude Code skill içe aktarma (`SkillImportDialog`) **Skills ekranından markete taşındı**
  (Skills kategorisi başlığındaki "İçe Aktar" butonu). `SkillsPanel.tsx`'ten buton+dialog+state kaldırıldı.
- **Built-in tools sorusu:** yerleşik araçlar binary'e derili → markete eklenemez; araç paylaşımının doğru
  karşılığı **mcp** türü (cevap dokümana da işlendi).
- **Build:** `go build ./...` + `go test ./internal/market ./internal/api` (59) yeşil; frontend `tsc --noEmit`
  temiz + `npm run build` (dist gömüldü).
- Detay: `_Docs\21-MARKET.md`.

## Bütçe geliştirmeleri — Tasarruf Merkezi + session bazlı kullanım ✅ (2026-06-24)

Bütçe sistemi 3 fazda genişletildi: tasarruf görünürlüğü, oturum-başına atıf, birleşik kazanç paneli.

- **Faz 1 — Sıkıştırma tasarrufu görünür:** Sistem B'nin kırptığı bayt artık ölçülüyor
  (`Usage.CompactSavedBytesLLM` + `db.AddLLMCompactionSavings`; Sistem A'nın `CompactSavedBytes`'ı zaten vardı).
  `GET /api/usage` totals/cumulative/trend + `GET /api/agents/{id}/usage` bu alanları taşıyor.
- **Faz 2 — Session bazlı kullanım/maliyet:** yeni `db.SessionUsage` rollup (`internal/db/store_session_usage.go`,
  **sessionID anahtarlı ömür-boyu**, gün-reset yok; `store/session-usage/<sid>.json`). `RecordUsage` +
  `compactToolResult` ctx'teki `SessionIDFrom` ile ajan kaydının yanında session'a da yazıyor. Yeni endpoint
  `GET /api/sessions/{id}/usage-detail` (cost helper'ları workspace ekranıyla paylaşılır). `SessionDetailPanel`
  "Bu oturumun harcaması" kartı (maliyet + kazanç/tasarruf kırılımı).
- **Faz 3 — Tasarruf Merkezi:** Bütçe ekranında tüm tasarruf kaynaklarını birleştiren panel (cache USD +
  Sistem A/B bayt + ~token eşdeğeri).
- **Notlar:** hook'lar tasarruf ölçmez (CC sözleşmesi); `context-mode`/`rtk`/`sqz` presence-only → ölçülen kazanç yok,
  yerel eşdeğer Sistem A. Sistem A/B yalnız bayt+~token gösterir (USD'ye çevrilmez — uydurma sayı olmaması için);
  gerçek USD yalnız prompt-cache'te. Bu cache USD'si az önceki claude-cli cache muhasebesi düzeltmesinden de
  beslenir (claude-cli turları artık cache read/write raporladığından session rollup'a da yansır).
- **Build/test:** `go build ./...` + `go vet` yeşil; `internal/db` (`store_session_usage_test.go` +
  `AddLLMCompactionSavings`), `internal/api`, `internal/agent` testleri (168) geçti; frontend `tsc --noEmit` temiz.
- Detay: `_Docs\17-TOKEN-OPTIMIZASYON.md` §Bütçe görünürlüğü.

## Bağlam önizleme cache haritası — cache dışı segmentler yeşil + cache sınırı ✅ (2026-06-24)

Önizleme ekranı (`SessionContextModal`) artık isteğin hangi parçasının **sıcak prompt-cache**'ten,
hangisinin her tur **taze** gittiğini gösterir.

- **Backend (`internal/api/session_context.go`):** `sessionContextPreview`'a `cache cachePreview`
  alanı + `computeCachePreview(provider, session, …)`:
  - **anthropic** (ExtendedPromptCache açık): breakpoint statik System bloğunda →
    `toolsCached`+`systemCached=true`, `dynamicCached=false`; kapalıysa `mode=none`.
  - **claude-cli** (ClaudeResume warm, `CLISessionID`+`CLISentMsgCount`>0): `systemCached=true`,
    `cachedMsgCount=CLISentMsgCount` → ilk N mesaj sıcak, delta taze; soğuk/ilk tur `mode=none`.
  - diğer sağlayıcılar `mode=none`. Her mod insan-okunur `note` taşır.
- **Frontend (`SessionContextModal.tsx` + `types/session.ts`):** cache dışı segmentler **hafif
  yeşil** (`text-emerald-500/75`, Markdown düz metni `currentColor`'dan miras alır); her bölümde
  `cache'li`/`cache dışı` pill (`CacheTag`), üstte yeşil legend (`note` ile), mesaj dizisinde
  ilk taze mesajdan önce **"cache sınırı — buradan sonrası taze gönderilir"** ayıracı.
- **Build/test:** `go build ./...` + `go vet` + `go test ./internal/{api,providers}/` (110 passed);
  frontend `tsc -b && vite build` yeşil, binary'e gömüldü.

## claude-cli cache muhasebesi — Usage ekranında resume tasarrufu görünür ✅ (2026-06-24)

`--resume` (ClaudeResume) modunun asıl faydası **prompt-cache hit**'idir, ama claude-cli
usage ayrıştırıcısı cache alanlarını okumuyordu → Usage/Budget ekranı her claude-cli turu için
**0 cache** gösteriyor, resume'un değeri görünmüyordu.

- **Düzeltme (`internal/providers/claudecli.go`):** `cliUsage` struct'ına
  `cache_read_input_tokens` + `cache_creation_input_tokens` alanları eklendi; `assistant`
  event'inde max-bağlama (per-message), `result` envelope'unda authoritative aggregate ile
  `resp.Usage.CacheReadTokens`/`CacheWriteTokens`'e yazılıyor.
- **Zincir doğrulandı:** `resp.Usage` → `RecordUsage` (budget.go:57) → `AddUsageKind` →
  `store_usage` aggregation → `GET /api/usage` (`cacheReadTokens`/`cacheWriteTokens`). Anthropic
  native ile aynı yoldan akar; artık claude-cli turları da cache read/write raporlar.
- **Build:** `go build ./...` + `go vet ./internal/providers/` yeşil.
- **Kapsam dışı (sıradaki):** bağlam önizleme ekranında (`context/preview`) cache-breakpoint
  etiketi + resume modunda "N mesaj CLI cache'inde sıcak, yalnız delta gönderilecek" satırı —
  henüz eklenmedi.

## SK-IMP UI — Skills panelinde içe-aktarma akışı ✅ (2026-06-24)

SK-IMP'in son parçası: importer artık UI'dan kullanılıyor (SK-IMP tamamen tamamlandı).

- **Frontend:** `SkillImportDialog.tsx` (kaynak seçici GitHub/yerel, location, opsiyonel slug, paylaşımlı
  toggle) → `api.importSkill` → sonuç kartı (slug + kopyalanan dosyalar + uyarı listesi). Skills panelinde
  ("İçe Aktar" butonu, `skills-import` testid) açılır; başarıda liste yenilenir + içe-aktarılan skill seçilir.
- **API client:** `skillApi.importSkill` + `SkillImportResult`/`SkillImportResponse`/`SkillImportInput`
  tipleri (`api/skills.ts`).
- **Doğrulama:** `tsc --noEmit` + `vite build` yeşil; full build (frontend gömülü) 8090'a deploy edildi.
  Çağrılan endpoint (`POST /api/skills/import`) zaten SK-IMP.2/3'te canlı doğrulanmıştı.
- Not: `Github` ikonu lucide sürümünde yok → `Globe` kullanıldı (build hatası giderildi).

## SK-IMP.3 — Importer: GitHub kaynağı + import_skill aracı ✅ (2026-06-24)

- **GitHub kaynağı (`internal/skills/github.go`):** `parseGitHubURL` (tree/blob → owner/repo/ref/dir,
  blob+SKILL.md → parent), `fetchGitHubSkill` GitHub contents API ile SKILL.md + top-level bundled
  dosyaları çeker (host-allowlist: api.github.com/github.com/raw.githubusercontent.com/codeload; 8MB cap,
  30sn timeout, anon). `Store.ImportFromSource(source, location, slug, shared)` local|github ayrımını yapar;
  `readLocalSkillDir` api'den buraya taşındı. API `POST /api/skills/import` artık `url` alanını da kabul eder.
- **`import_skill` self-management aracı:** `SkillWriter.ImportSkill` + `tools.SkillImportResult` +
  `ImportSkillTool` (`builtin_skillmgmt.go`); `agentSkillWriter.ImportSkill` → `ImportFromSource`;
  `toolsetup`'ta create/update/delete yanında kayıtlı (CLI bridge üzerinden de erişilir).
- **Testler:** `import_test.go` yeşil; `builtin_skillmgmt_test.go` fake'e `ImportSkill` eklendi.
  `go build ./...` + `go test ./internal/{skills,tools,agent,api}/` yeşil.
- **Canlı doğrulandı (8090):** (1) GitHub `anthropics/skills/.../skill-creator` → source_url + bundled
  LICENSE.txt kopyalandı, sıfır uyarı; (2) claude-cli ajanı `import_skill`(local `gsd-capture`) → slug +
  `$ARGUMENTS` uyarısı birebir döndü. Test artefaktları silindi.
- **Kalan:** Market/Skills UI içe-aktarma akışı.

## SK-IMP — Claude Code skill importer: çekirdek + local API ✅ (2026-06-24)

Seviye 2 importer'ın ilk iki increment'i. Önkoşullar SK-1..SK-4 hazırdı.

- **Çekirdek (`internal/skills/import.go`):** `mapCCSkill(raw, sourceURL, shared)` CC frontmatter'ını
  SwarmGo'ya eşler — name/description/when_to_use→aynı, `allowed-tools`→`always_allow`, `paths`→koşullu,
  version/license→aynı, source_url=import kaynağı (provenance), `disable-model-invocation:true`→shared
  değil, `user-invocable`→`user_invocable`. Uyumsuzu (`context:fork`, `hooks`, `model`/`agent`/`effort`,
  slash-arg `$ARGUMENTS`/`$1`, inline-shell `` !` ``) ayıklayıp **warning** döndürür. `Store.ImportCCSkill`
  rendered SKILL.md + bundled dosyaları (yalnız düz dosya adı; path-traversal reddi; SKILL.md hariç)
  workspace tier'a yazıp reload eder, slug çakışmasında hata verir.
- **API:** `POST /api/skills/import` (`source:"local"`, `path`, opsiyonel `slug`/`shared`) →
  `readLocalSkillDir` (SKILL.md + sibling dosyalar) → `ImportCCSkill` → `{result, skill}` döner
  (`handleImportSkill`, `server.go` route).
- **Testler:** `import_test.go` (mapping + disable-invocation downgrade + write/resolve + path-traversal
  reddi + duplicate). `go build ./...` + `go test ./internal/{skills,api}/` yeşil.
- **Canlı doğrulandı:** gerçek `~/.claude/skills/gsd-add-tests` 8090'a import edildi →
  `always_allow=Read;Write;Edit;Bash;Glob;Grep;Agent;AskUserQuestion`, source_url set, `$ARGUMENTS` uyarısı,
  shared=false. Test artefaktı silindi.
- **Kalan (SK-IMP.3):** GitHub kaynağı (URL→fetch), `import_skill` self-management aracı (agent-usable),
  Market/Skills UI akışı. Detay: `03-YOL-HARITASI.md` SK-IMP.

## Skill sistemi geliştirmeleri SK-1..SK-4 (CC skill importer önkoşulları) ✅ (2026-06-23)

Claude Code skill importer'a (Seviye 2, `03-YOL-HARITASI.md` SK-IMP) hazırlık olarak, kendi skill
sistemimizde 4 önkoşul uygulandı. Kaynak desen: `observed-behavior/src/skills/loadSkillsDir.ts`.

- **SK-1 — Çok-dosyalı skill:** `Store.UseSkillBody` gövdede `${SKILL_DIR}` (+ CC `${CLAUDE_SKILL_DIR}`)
  ikamesi yapıyor → skill kendi klasöründeki dosyalara atıf verir; **sibling dosyalar** "## Bundled files"
  footer'ıyla ajana ilan edilir (Read ile on-demand). Raw `Body` (edit/detay) literal kalır.
  `substituteSkillVars`/`bundledFilesFooter` (`store.go`).
- **SK-2 — Ölçeklenebilir keşif:** `Skill.Paths` (`paths:`) → **koşullu skill** auto-advertise'dan çıkar
  (prompt şişmez), loadable kalır; `Store.Search` + yeni **`skill_search`** aracı (`builtin_skillsearch.go`,
  native `toolsetup.go` + CLI bridge `mcp_interaction.go`). fs-touch OTOMATİK aktivasyon ertelendi
  (session-scoped state gerekir).
- **SK-3 — `allowed_tools` enforcement:** `use_skill` yüklenince skill'in `always_allow` desenlerini
  `ParsePermRule` ile oturum grant'larına (`GrantsFrom(ctx)`) ekler → araçlar re-prompt'suz; çıktıya
  şeffaflık notu. `SkillLibrary.AllowedTools` + `agentSkillLib.AllowedTools`.
- **SK-4 — Provenance frontmatter:** `Skill.Version/SourceURL/License/UserInvocable` parse + serialize
  (importer'da köken/güncellik izi). `user-invocable` default true.
- **CLI bridge (canlı testte bulunup tamamlandı):** `skill_search` ve SK-3 auto-grant ilk uygulamada
  yalnız native yoldaydı; claude-cli ajanlarının Interaction MCP köprüsünde eksikti. Eklendi:
  `Runtime.SearchSkillsForAgent`/`SkillAllowedToolsForAgent`, `chatRun.skillSearch`/`skillAllow`
  setter'ları (chat_stream + autonomous_interaction'da kurulur), `mcp_interaction.go` dispatch
  case `skill_search` + `callSkillSearch` + `grantSkillToolsCLI`. `renderCatalog` ajana skill_search'ü
  hatırlatan satır ekler (koşullu skill'ler katalogda yok). **Not:** CLI bridge yalnız streaming
  `/api/chat/stream` yolunda kurulur; non-streaming `/api/chat` interaction tool'larını bağlamaz.
- **Doğrulama:** `go build ./...` + `go test ./internal/{skills,tools,agent,api}/` yeşil; yeni testler
  `TestUseSkillBodySK1`/`TestSearchAndConditionalSK2`/`TestUseSkillGrantsToolsSK3`/`TestRichFrontmatterSK4`.
  **Canlı (claude-cli, /api/chat/stream):** SK-1 `${SKILL_DIR}`+bundled path, SK-2 `skill_search`→koşullu
  `sk-demo` (katalogda yok), SK-3 "_auto-allowed: Bash(git *)_", SK-4 API parse — hepsi doğrulandı.
  Sırada: **SK-IMP** (gömülü importer).

## Vite bundle temizliği + API E2E smoke testi ✅ (2026-06-23)

**Vite:** Production build'in "chunks larger than 500 kB" uyarısı temizlendi.
`vite.config.ts`'e `manualChunks` eklendi — ağır vendor kütüphaneleri ana
bundle'dan ayrıldı: `vendor-highlight` (highlight.js ~152 kB), `vendor-markdown`
(react-markdown/remark/rehype ~161 kB), `vendor-react` (~359 kB). Ana `index`
chunk'ı **969 kB → 472 kB**'a indi (artık limitin altında). Tek kalan büyük chunk
`relationGraph` (vis-network ~522 kB, zaten lazy) için `chunkSizeWarningLimit: 600`
ayarlandı — bilinçli lazy vendor chunk'ı için dürüst susturma. Build temiz, uyarı yok.

**E2E smoke testi:** Repodaki ilk otomatik test — `scripts\e2e-smoke.ps1` (**12 adım**).
Doc 33'teki HTTP API yolunu baştan sona doğrular: health → **CORS preflight (`*`+auth-yok)**
→ **hata sözleşmesi (400/404 `{error}`)** → workspaces → agents → session →
**workdir round-trip (cwd set→git→reset)** → **gerçek LLM chat turu (SSE `done`+yanıt)**
→ **çok-turlu bağlam sürekliliği (codeword recall)** → **`permissionMode=read-only`
override turu** → kalıcılık (8 mesaj/4 tur) → cleanup. Canlı sunucuya karşı `AGT3`
(claude-cli/haiku) ile **12/12 geçti** (LLM `ZEPHYR-7` codeword'ünü 2. turda hatırladı);
`-SkipLLM` ile 9/9 (smoke-only, ucuz). CI dostu exit kodu. Detay: `_Docs/33` §A.6.

## Lazy araç kataloğu MCP açıklamalarını kısaltıyor (bağlam şişmesi fix) ✅ (2026-06-23)

Sorun: Sistem promptundaki **"Available Tools (load on demand)"** bloğu, lazy MCP
araçlarının **tam, çok-paragraflı açıklamasını** (`e.Tool.Description`) basıyordu. Gateway
gibi sunucuların açıklamaları "When to use / When NOT to use" rehberleriyle dolu →
her lazy araç için bu metin bağlama giriyor, sistem promptu şişiyordu (ve Windows'ta
claude-cli komut satırını 32K limitine itiyordu — yukarıdaki fix'in tetikleyicisiyle
aynı kök bloat).

Çözüm (`internal/tools/registry.go` `LazyCatalog`):
- Lazy blok bir **isim + kısa özet** teaser'ıdır; tam açıklama araç `activate_tools`
  ile yüklenince zaten şemada (`Defs`) geliyor. Yeni `lazyDescription` helper'ı her
  açıklamayı **ilk anlamlı satıra** indirip `lazyCatalogDescMaxChars=200` ile kapıyor
  (UTF-8 sınırında). Hem native hem MCP lazy araçlarına uygulanıyor.
- Sonuç: gateway gibi yüzlerce araçlı sunucularda load-on-demand bloğu dramatik küçülür.
- Build + `internal/tools` testleri yeşil.

## claude-cli sistem promptu artık dosyadan veriliyor (Windows 32K arg limiti fix) ✅ (2026-06-23)

Sorun: Otonom/flow turlarında claude.exe başlatılırken
`provider error: fork/exec ...claude.exe: Dosya adı veya uzantısı çok uzun.`
(Windows hata 206 / `ERROR_FILENAME_EXCED_RANGE`). Kök neden: `claudecli.go`
sistem promptunu `--append-system-prompt <sys>` ile **komut satırı argümanı** olarak
geçiriyordu. Sistem promptu (skills + core memory blokları + dinamik bağlam) büyüyünce
Windows'un ~32.767 karakterlik komut satırı limiti aşılıp süreç hiç başlamadan çöküyordu.
Belirti flow'larda ardıl `node "<X>" (agent): context canceled` olarak da görünüyordu
(paralel düğüm çökünce ortak context iptal edilir).

Çözüm (`internal/providers/claudecli.go` `Complete`):
- Sistem promptu artık `os.CreateTemp` ile bir temp dosyaya yazılıp
  **`--append-system-prompt-file <path>`** bayrağıyla veriliyor → komut satırında yalnız
  kısa bir yol taşınıyor, limit aşımı imkânsız. (Sohbet promptu zaten stdin'den gidiyordu.)
- Temp dosya `defer os.Remove` ile her iki retry denemesi bitince siliniyor.
- Gereksinim: claude CLI'nin `--append-system-prompt-file` desteği (2.1.186'da mevcut;
  `--bare` yardımında `--append-system-prompt[-file]` belgeli).
- Build + `go vet` + `internal/providers` testleri yeşil.

## "Gizli" çip artık gerçek context durumunu yansıtıyor + self-management'ı kapsıyor ✅ (2026-06-23)

Sorun: self-management araçları (ajanın SwarmGo'yu kontrol eden tool'ları) kodda
zorla `MarkHidden` olduğu için context'te görünmüyordu, ama Araçlar ekranındaki
"Gizli" çip yalnızca kullanıcının `HiddenTools` listesini yansıtıyordu → bu araçlar
çipsiz "normal" görünüyordu (yanıltıcı) ve kullanıcı bunları context'e alamıyordu.

Çözüm — çift yönlü görünürlük override'ı:
- **Çip artık efektif lazy durumunu gösteriyor.** API `hidden` alanı registry'nin
  gerçek `IsLazy` durumundan geliyor (`WorkspaceToolCatalogWithState`): kod-default
  lazy (self-management/MCP/read_config/WebFetch…) + kullanıcı override'ları. Yani
  context'e her tur gitmeyen her araç "Gizli" rozeti alır.
- **`ShownTools` override'ı eklendi** (`WorkspaceToolConfig`): default gizli bir aracı
  (özellikle self-management) **zorla context'e** geri alır. `registry.Unlazy` lazy+hidden
  işaretlerini siler; toolsetup'ta **en son** uygulanır (tüm default + MCP lazy'yi ezer).
- **Toggle çift yönlü:** "Göster" → `ShownTools`'a ekle / `HiddenTools`'tan çıkar;
  "Gizle" → tersi. Her araç için çalışır (kod-gizli self-management dahil).
- API PUT üç listeyi de per-field merge eder (`disabled`/`hidden`/`shown`).
  Frontend `setWorkspaceToolsVisibility(hidden, shown)`.
- Test: `TestUnlazyOverridesHidden` (registry), db round-trip'e ShownTools. 238 test yeşil.

## Araçlar ekranında "Gizli" (load-on-demand) çip + toggle ✅ (2026-06-23)

Skills ekranındaki "Gizli" (auto-summary off) deseninin **araçlara** karşılığı eklendi.
Bir araç "Gizli" işaretlenince **aktif kalır** ama şeması her tur ajana gönderilmez —
ajan gerektiğinde `tool_search`/`activate_tools` ile çeker (= `MarkLazy`). `enabled`
(devre dışı) toggle'ından bağımsız, ikinci bir eksen.

- **db** (`store_tools.go`): `WorkspaceToolConfig.HiddenTools []string` (DisabledTools'tan
  ayrı, persist + reload). 
- **API** (`workspace_tools.go`): per-tool `hidden` flag + yanıtın `hiddenTools` dizisi;
  `PUT /api/workspace-tools` artık **per-field merge** (pointer'lı req → yalnız gelen
  liste değişir, diğerine dokunmaz; enable & hide toggle'ları çakışmaz).
- **toolsetup** (`toolsetup.go`): tur kurulumunda `reg.MarkLazy(cfg.HiddenTools...)` —
  builtin + MCP araçlarında çalışır (MarkLazy sıra-bağımsız isim seti).
- **Frontend** (`ToolsPanel.tsx`): listede ve detayda **"Gizli" çipi** (SkillsPanel'in
  `SummaryOffBadge` stiliyle aynı warning rengi), detayda **Gizle/Göster** (Eye/EyeOff)
  toggle'ı (optimistic + revert). `api.setWorkspaceToolsHidden`, tipler `hidden`/`hiddenTools`.
- Test: `store_tools_test.go` (hidden round-trip + disabled'dan bağımsızlık). Tüm testler yeşil.

## Self-correcting hata kapsamı taraması — kalan boşluklar kapatıldı ✅ (2026-06-23)

242 builtin tool hata mesajı tarandı. Çoğu zaten iyiydi (`use list_X`, geçerli
değer listeleri, argErr/enumErr/cron/graph hint'leri). Atlanan grup: meta/interaction
araçları `invalid <tool> input: %w` (argErr'in eşleştiği `invalid arguments: %w`'den
farklı kelime → toplu değişimde kaçmış), eyleme dönük "fix:" eki yoktu.

- Yeni `argErrFor(tool, err)` (`builtin_errhints.go`): tool adını korur + aynı şema
  ipucunu ekler. **12 site** dönüştürüldü: activate/deactivate/tool_search,
  create/update_artifact, ask_user, request_confirmation, send_message, use_skill,
  spawn_session, todo_write, run_subagent (`subagent.go`).
- Edit araçları (`builtin_fs.go`): `old_string not found` → "Read the file first,
  copy exact text incl. whitespace (no line-number prefixes)"; `identical` →
  "make new_string differ". Kör retry'ı önler.
- Bilinçli dokunulmayanlar: terse-ama-net `X is required` (çözüm zaten örtük) ve
  `… not available in this context` (ortam hatası, input'la düzeltilemez).
- Test: `argErrFor` için case eklendi. Tüm tool/provider testleri yeşil.

## Hooks paneli: harici araç oto-tespit + tek-tıkla bağla toggle'ı ✅ (2026-06-23)

Ayarlar ▸ Hooks ekranı (`HooksPanel.tsx`) iki iyileştirme aldı:
- **Oto-tespit:** Harici token araçları (`rtk`/`sqz`/`context-mode`)
  artık **ekran açılır açılmaz** otomatik kontrol ediliyor (`useEffect`'e `checkTools()`
  eklendi); eski "Kurulu mu kontrol et" butonu yeniden-tarama için korundu. Tespit
  hâlâ presence-only (`/api/external-tools` → `exec.LookPath`, çalıştırma/kurulum yok).
- **Tek-tıkla bağla toggle'ı:** Bulunan her hook-tabanlı araç için **Bağla / Aktif /
  Pasif** düğmesi. `TOOL_HOOK_TEMPLATES` şablonundan ilgili hook'u oluşturur
  (`rtk`→PreToolUse/`Bash` PowerShell rewrite adapter; `sqz`→PreToolUse/`Bash`,
  komut `sqz hook claude` — rtk gibi bash-rewrite, ikisini aynı anda Bash'te açma),
  tekrar tıklayınca `toggleHook` ile aç/kapat (silmez).
  `context-mode` MCP tabanlı (sandbox + FTS5 KB) olduğu için toggle yerine **MCP**
  rozeti gösterilir (hook değil; Ayarlar ▸ MCP'den eklenir). `wiredHook()` eşlemeyi
  komut içeriğinden yapar. Otomasyon için `data-testid="tool-toggle"` + `data-tool` eklendi.
- Doğrulama: frontend `tsc --noEmit` yeşil. Not: prod embed için `npm run build`
  + Go yeniden derleme gerekir (dev'de Vite HMR yeterli).

## `update_skill` self-management aracı ✅ (2026-06-23)

Ajanlar bir skill'i değiştirmek için `delete_skill`+`create_skill` yapmak zorundaydı
(SES2'de tam bunu yaptı — riskli, dangling-reference doğurabilir). Artık **`update_skill`**
var: slug ile in-place düzenleme, **partial** semantik (yalnız değişen alanları geç —
name/description/whenToUse/body/shared; verilmeyen alan korunur). Yalnız **workspace-tier**
skill düzenlenebilir (global/bundled korunur). Mimari:
- `tools.SkillWriter` arayüzüne `UpdateSkill(slug, name,desc,when,body *string, shared *bool)`
  eklendi (pointer = nil → değişme). `UpdateSkillTool` (`builtin_skillmgmt.go`),
  örnekli şema + boş-güncelleme/slug guard'ları.
- `agentSkillWriter.UpdateSkill` (`runtime.go`): `store.Get`+`store.Body` ile mevcut
  değerleri okuyup merge eder, tier guard (`Source==workspace`), `store.Update` çağırır.
- `toolsetup.go`: create/delete arasına eklendi → self-management aralığında olduğu için
  otomatik **hidden-lazy** (cached prefix'i şişirmez; `tool_search`/`activate_tools` ile erişilir).
- Doc: `swarmgo-self-management` SKILL.md güncellendi. Test: `builtin_skillmgmt_test.go`
  (partial forwarding + validation). Toplam testler yeşil.

## Dış-ajan otomasyon dostluğu — UI seçicileri + API rehberi ✅ (2026-06-23)

Soru: "SwarmGo'yu dışarıdan ajanlar (chrome-mcp/playwright-mcp) baştan sona kullanabilir mi,
eksik/iyileştirilecek yer var mı?" İki yol değerlendirildi:

- **HTTP API yolu zaten eksiksiz (9/10):** 138+ endpoint tüm alt sistemleri kapsıyor, **auth yok**
  (`server.go:withCORS`), **CORS wildcard açık**, SSE `chat/stream` net terminal sinyali veriyor
  (`meta→agent→step→reply→done|error`, `chat_stream.go`). Dış ajan API ile uçtan uca sürebilir.
- **UI tıklama yolu kırılgandı (6/10):** `data-testid` yok, DOM'da tamamlanma sinyali yok,
  modaller `role=dialog` taşımıyordu.

**Uygulanan (UI otomasyon dostluğu, additive — yalnız attribute):**
- **`data-testid` haritası:** NavRail (`nav-{view}`/`nav-workspace`/`nav-settings`), Composer
  (`composer-input`/`-send`/`-stop`/`-queue`/`-interrupt`/`-steer`/`-attach`), AgentSelect
  (`agent-select`/`-menu`/`-option`+`data-agent-id`), WorkspaceSwitcher (`workspace-switcher`/
  `-menu`/`-row`+`data-workspace-id`/`-switch`/`-create`), MessageList (`chat-transcript`/`chat-message`).
- **Tamamlanma sinyali:** `chat-message` satırında `data-role` + **`data-streaming="true|false"`**
  (MessageList'te `rowLive` hoisted) → otomasyon turun bittiğini DOM'dan okur, polling yok.
- **Erişilebilirlik:** `chat-transcript` `role=log`+`aria-live=polite`; AgentSelect/WorkspaceSwitcher
  `role=listbox`/`option`+`aria-expanded`; 6 modal (`AgentSettings`/`AgentContext`/`SessionContext`/
  `SkillEditor`/`WorkspaceCreate`/`SpawnSession`) `role=dialog`+`aria-modal`+`aria-label`+testid;
  NavRail butonları `aria-label`+`aria-current`.
- `npx tsc --noEmit` yeşil (EXIT=0). Hiç davranış değişmedi, yalnız işaretleme eklendi.

**Yeni doküman:** `_Docs\33-DIS-AJAN-OTOMASYONU.md` — iki yol karşılaştırması, API uçtan-uca akış,
SSE tüketim örnekleri, testid haritası, playwright/chrome-mcp reçeteleri, bilinen sınırlar.

## Panel-içi form testid'leri — Faz 1+2+3 ✅ (2026-06-23)

Önceki fazda yalnız çekirdek sohbet/nav/workspace/modal akışları testid taşıyordu; panellerin iç
formları taşımıyordu (dış ajan API'den yönetebiliyor ama UI'dan kırılgan). Tüm panel formlarına
`data-testid` eklendi — 4 paralel subagent ile (dosyalar disjoint, çakışma yok), additive (yalnız
attribute, davranış/stil değişmedi).

**Konvansiyon:** statik = `{panel}-{eylem}`; dinamik liste öğesi = sabit `data-testid` + ayrı
`data-{entity}-id` (nth yerine id ile hedefleme). Özel bileşenlere (`<Button>`/`<AgentPicker>`/
`<ProviderModelSelect>`) gerekince layout-nötr saran `<div data-testid>` (ör. `*-wrap`).

**Kapsam (~146 yeni, toplam ~183 testid / 32 dosya):**
- **Agents:** AgentRoster (7), AgentSettingsForm (14), ProviderModelSelect (5), AgentToolsSection (4),
  AgentSkillsSection (5), AgentPicker (2), EmojiPicker (4).
- **Tasks:** TaskBoard (7), TaskDetailPanel (9), BoardColumnEditor (10).
- **Schedules+Settings:** Schedules (15), ProvidersPanel (14), ToolsPanel/MCP (14), HooksPanel (8).
- **Diğer paneller:** SkillsPanel (8), MarketPanel (3), SecretsPanel (7), MemoryPanel (6),
  ArtifactsPanel (12), FlowCanvas (2).

`npx tsc --noEmit` yeşil (EXIT=0). Seçici haritası `_Docs\33-DIS-AJAN-OTOMASYONU.md` §B.2'ye işlendi.

**Gerçek E2E doğrulama (2026-06-23):** headless Playwright (kurulu Chrome) ile canlı uygulama
(Vite :5173 → backend :8090) sürüldü; MCP köprüsü o an dalgalandığı için doğrudan Playwright
kullanıldı. 16/16 kontrol geçti: NavRail 15/15, chat composer (input/send/agent-select), tüm panel
CRUD testid'leri ve **gerçek etkileşim** (ajan oluştur formuna ad yazıp geri okuma). Test bir boşluk
yakaladı: "Ajanlar" ekranı `AgentRoster` değil **`AgentsView`** render ediyor; bu bileşen testid'siz
kalmıştı → AgentsView roster/create formuna testid eklendi (commit eafe718). **Sıradaki (ertelendi):**
ağa açılırsa API auth katmanı.

## Yapılandırılmış konuşma-özeti — Claude Code parite 1. faz (compact decay fix) ✅ (2026-06-23)

WS2/SES2'de kullanıcı compact'in "çok kısa özet" ürettiğini ve "compact sonrası hâlâ
mesaj eklendiğini" bildirdi. Teşhis: ikincisi tasarım (kayan pencere, `keepRecent=8` +
yeni turlar birikir — normal); birincisi `conversation/manager.go` `compactPrompt`'undaki
**"under 200 words"** cap'i + her katlamada **özetin özetini** alan rolling-merge →
**decay**.

Claude Code compaction motoru incelendi (`Desktop/Projects/observed-behavior/src/services/
compact/`: `prompt.ts` 9-bölümlü + `<analysis>` scratchpad, `compact.ts`, `autoCompact.ts`,
~20K output rezervi, fork+prompt-cache paylaşımı, post-compact dosya/skill re-injection).

**1. faz uygulandı (düşük risk, en yüksek etki):**
- `compactPrompt` → sabit **8 bölümlü** yapı + **anti-decay talimatı** ("önceki özetteki her
  kalıcı gerçeği taşı, kısaltma"). 200-kelime cap kaldırıldı. İki `%s` korundu → `reactive.go`
  aynı sabiti kullanmaya devam.
- `compactMaxOutputTokens = 8192`; `summarize` + reactive yol `Request.MaxTokens` ile geçiyor
  → uzun özet anthropic 4096 default'unda kesilmiyor.
- `go build ./...` + `internal/conversation` testleri yeşil. Detay: `_Docs/17` §8.

**Bilinçli ertelendi:** fork/cache (SwarmGo özetleyiciye yalnız katlanan dilimi yollar →
çağrı zaten ucuz, fork'un çözeceği pahalılık yok; claude-cli cache paylaşımını kontrol edemez).

## Post-compact kurtarma işaretçisi — Claude Code parite 2. faz ✅ (2026-06-23)

CC compact sonrası transcript pointer + son okunan dosya re-injection yapar. SwarmGo'ya
**birebir port mimariye ters:** turlar arası yalnız `role+text` taşınır (`toProviderMessages`)
→ tool sonuçları/dosya okumaları zaten cross-turn context'te değil; ajan serbest fs ile
istediğinde yeniden okur. Kalıcı durum (artifacts/todos/core-memory/goal/summary) zaten her
tur re-inject ediliyor.

**Uygulanan:** özet bloğu `conversationSummaryBlock(summary)` ile sarıldı (`api/chat_turn.go`)
→ özetin altına **kurtarma notu**: "önceki turlar katlandı, tam metni yok; kesin detay lazımsa
tahmin etme — `conversation_search` ile ara veya dosyaları fs araçlarıyla yeniden aç". Compact
sonrası ajan körleşmez. `readFileState` tracker bilinçle eklenmedi (mimariye gereksiz). `go build`
+ `internal/api` + `internal/conversation` testleri yeşil. Detay: `_Docs/17` §9.

**Sırada (opsiyonel 3. faz):** partial compact (`from`/`up_to`) + boundary UI; veya tam decay-sıfır
için merge yerine `history` prefix'inden sıfırdan özetleme (fork tartışmasına bağlı).

## SES5 flow çöküşü teşhisi: crash-tail kuyruğu + create_agent provider default ✅ (2026-06-23)

WS2/SES5'te "Seyahat Planlama Akışı" flow'u node'larda `claude CLI failed: exit
status 1` veriyordu (sıralıda flight başarılı → hotel hata; paralelde flight hemen
hata — yani **intermittent**, deterministik değil).

**Eleme:** gsd `SessionStart` hook'ları (`gsd-check-update.js`/`gsd-session-state.sh`)
exit 0 ile bitiyor → sebep değil. 4× eşzamanlı **düz** claude-cli hepsi exit 0 →
ham eşzamanlılık/`~/.claude.json` çakışması da değil.

**Asıl yön:** Flow agent'ları (AGT6–9) `mcpEnabled=true` (create_agent'ta
**hardcoded**) + workspace `enableCliHooks=true` → claude-cli'a fazladan
`--mcp-config` (interaction MCP) + `--settings` (hook'lar) + `--permission-prompt-tool`
geçiliyor; çöküş bu kırılgan yolda. Tam başarısız bileşen crash-tail'de gizliydi.

**İki düzeltme:**
1. **crash-tail artık ölümcül SONU gösteriyor** (`stdoutCrashTail`, `claudecli.go`).
   Önceki sürüm baştan 600 char kırpıyordu → yalnız hook startup gürültüsü
   görünüyordu. Yeni sürüm stream-json `result`/`is_error`/`error` event'lerini ve
   plain panic satırlarını öne çıkarır, kırparken **son 800 char**'ı korur. Böylece
   "mcp server failed" gibi gerçek sebep mesaja düşer.
2. **`create_agent` boş provider bırakmıyor** (`builtin_agentmgmt.go`). AGT6–9
   `provider=""` ile kaydedilmişti (örtük fallback'e bağımlı, teşhisi zor); artık
   boşsa `claude-cli`'a default'lanır.

> Sıradaki kesin adım: SwarmGo'yu yeniden derleyip flow'u tekrar çalıştır →
> geliştirilen crash-tail tam başarısız bileşeni (hangi MCP/permission) yazacak.

## Self-correcting tool hataları (yayma) + claude-cli çöküş teşhisi ✅ (2026-06-23)

Bir önceki flow-graph fix'inin desenini tüm tool yüzeyine yaydık + SES2 turn 19
CLI çöküşünün teşhis kara-deliğini kapattık.

**1) Ortak hata-ipucu yardımcıları (`builtin_errhints.go`).**
- `argErr(err)` — 49 tool çağrı-yerindeki tek-tip `fmt.Errorf("invalid arguments:
  %w", err)` bununla değişti: Go'nun alan/tip hatasını korur + *"fix: match this
  tool's input schema (required fields, exact types); see its examples"* ekler.
- `enumErr(field, got, allowed...)` — geçersiz değeri ve **izinli seti** listeler;
  task `boardState` kontrollerinde (create/update/move) kullanıldı.
- `cronHint` — schedule reload hatasına 5-alan cron formatı + örnekler
  (`"0 * * * *"`=saatlik, `"*/15 * * * *"`, `"@daily"`); `create/update_schedule`'da.

**2) claude-cli çöküşü artık teşhis taşıyor (`claudecli.go`).** Önceden CLI exit 1
+ boş stderr ile öldüğünde agent'a yalnız `claude CLI failed: exit status 1`
gidiyordu (SES2 turn 19). Artık stdout'un son satırları bounded ring'de tutuluyor;
stderr boşsa `stdoutCrashTail` JSON-olmayan (gerçek hata/panic) satırları tercih
edip mesaja ekliyor → *"claude CLI failed: exit status 1 stdout-tail: panic: …"*.

Test: `builtin_errhints_test.go` (graph hint SES2'nin gerçek hatasını eşliyor +
argErr/enumErr) ve `claudecli_crashtail_test.go`. Toplam 120 test geçer.

## Flow graph hata mesajlarına "ne yapmalı" ipucu + paralel node belgelenmesi ✅ (2026-06-23)

**Sorun (WS2/SES2):** Bir agent paralel flow kurarken `parallel` node'unu yanlış
şemayla (`branches:["id"]` + `next`) kurdu → ham Go hatası
`cannot unmarshal string into ... Node.nodes.branches of type orchestration.Branch`.
Hata "ne yapmalı" demediği ve `swarmgo-flows` skill'i node JSON şemasını hiç
belgelemediği (sadece soyut "steps/edges" anlatıyordu) + `create_flow` örneklerinde
paralel örnek olmadığı için agent doğru şemayı bulamadı, sıralı flow'a düştü.

**Çözüm — kendini düzelten tool hataları:** `validGraphJSON` artık her graph
hatasına kısa, eyleme dönük bir ipucu ekliyor (`graphSchemaHint` + `nodeSchemaCheat`):
hatadaki imzaya göre ("Node.nodes.branches", "has no children", "must be an agent
node" vb.) doğru alanı 5-6 kelimeyle söyler — ör. *"parallel fan-out uses
parallel:[...],joinNext — not branches/next"*. Ayrıca `create_flow`'a paralel
fan-out+join örneği ve `swarmgo-flows` skill'ine node-tipi/alan tablosu + paralel
örnek eklendi. (`builtin_flowmgmt.go`, `skills/defaults/swarmgo-flows/SKILL.md`.)
Doğru paralel şema: `{type:"parallel","parallel":["a","b"],"joinNext":"merge"}`
(çocuklar agent node id'leri; `branches`/`next` DEĞİL).

> Not: SES2'de turn 19'daki `provider error: claude CLI failed: exit status 1`
> ayrı bir provider/CLI çöküşüdür (tool hatası değil, detay loglanmadı) — bu fix
> kapsamı dışında.

## 3 self-management iyileştirmesi: create_agent skills + run_schedule + autonomous artifacts ✅ (2026-06-23)

Üç kullanıcı isteği tek turda:

**1) `create_agent` artık skill atayabiliyor.** Yeni opsiyonel `skills` (slug dizisi)
parametresi; verilmezse yeni agent **default SwarmGo skill seti** ile tohumlanır
(`skills.DefaultSkillSlugs()` — embed'deki `defaults/` alt-dizinlerinden türetilir).
Sağlanan slug'lar skill store'a karşı doğrulanır (`r.skillExists`); bilinmeyenler atlanır
ve sonuçta `skippedUnknownSkills` olarak raporlanır. (`builtin_agentmgmt.go`,
`skills/defaults.go`, `toolsetup.go`.)

**2) `run_schedule` aracı eklendi — "şu schedule'ı şimdi fırlat".** Agent bir routine'i
cron zamanını/enabled durumunu beklemeden **manuel tetikler** (UI "Run now" eşleniği,
`Scheduler.RunNow`). Yıkıcı olmadığı için provenance aranmaz (herhangi bir schedule).
Wiring: `Runtime.runSched` + `SetScheduleRunner` (manager `sched.RunNow` bağlar) →
`tools.NewRunScheduleTool` (self-management). (`builtin_schedulemgmt.go`, `runtime.go`,
`toolsetup.go`, `workspace/manager.go`.)

**3) Artifact'lar her turda oluşturulabilir.** Önceki hata: autonomous turlarda
(scheduler/spawn/flow) artifact sink kurulmadığı için `create_artifact` →
*"artifacts are not available for this turn"*. Çözüm iki yolu da kapsar:
- **Native:** `completeTraced` chokepoint'inde, ctx'te sink yoksa ve sessionID varsa
  fallback sink kurulur (`tools.HasArtifactSink` + `Runtime.NewArtifactSink`).
- **claude-cli:** `api/autonomous_interaction.go` artık run'a `setArtifacts` çağırıyor
  (Interaction MCP köprüsünün gördüğü sink).
- Yeni emit'li sink `internal/agent/artifactsink.go`'da (db'ye yazar + `artifact`
  event'i yayar; chat yolundaki api sink'inin aynası). Artifact'lar artık otonom
  ajanların ürettiğinde de UI'da bildirilir.

Test: `tools` (create_agent skills + run_schedule), `agent` (NewArtifactSink persist),
mevcutlar uyarlandı. **`tools`+`agent`+`skills`+`api`+`workspace` 221 test yeşil**,
build+vet+gofmt temiz. Skill `swarmgo-self-management` güncellendi (+ on-disk senkron).

---

## Self-management araçları prompt'tan gizlendi → skill katalog oldu (hidden-lazy tier) ✅ (2026-06-23)

**Sorun:** Self-management araçları zaten lazy'di (şema yok), ama ~40+ aracın **isim+özet
satırı** her turun "Available Tools (load on demand)" bloğunda (cached prefix) yer alıyordu —
gereksiz token. **Çözüm:** lazy araçlara **hidden** alt-katmanı eklendi; self-management suite
artık blokta **listelenmez**, yerine `swarmgo-self-management` skill'ine yönlendiren tek satır
durur. Araçlar aktive-edilebilir ve aranabilir kalır.

- **`internal/tools/registry.go`:** yeni `hidden map[string]bool` (hidden ⊆ lazy) +
  `MarkHidden(names…)`; `VisibleLazyCatalog(allow)` (= LazyCatalog − hidden, blok için);
  `HiddenLazyCount(allow)`. `LazyCatalog` (activate_tools/tool_search kaynağı) **tüm** lazy'yi
  döndürmeye devam eder → hidden araçlar aktive/aranabilir.
- **`internal/agent/toolsetup.go`:** self-management suite (`builtins[selfManageStart:]`)
  `MarkLazy` yerine **`MarkHidden`**. `LazyToolsCatalogBlock` artık `VisibleLazyCatalog` +
  `HiddenLazyCount` kullanır; `renderLazyToolCatalog(visible, hiddenCount)` hiddenCount>0 ise
  "**N self-management tools … not listed here … load the `swarmgo-self-management` skill … or
  `tool_search`**" pointer satırını basar.
- **Keşif yolu:** Available Skills bloğu `swarmgo-self-management` skill'ini zaten ilan ediyor
  (giriş noktası). Skill **kataloğun kendisi** oldu; metni güncellendi ("bu skill araçların
  listesidir; isimleri buradan/`tool_search`'ten al, `activate_tools` et"). On-disk seed kopya
  da güncel kaynakla senkronlandı (EnsureDefaults üzerine yazmadığı için).
- **Kapsam dışı (şimdilik):** secret_*/list_sessions/WebFetch/*_config hâlâ görünür-lazy
  (self-management değil, az sayıda, çapraz-kesen). claude-cli Interaction MCP köprüsü
  (`BridgeableDefs`) tam şema göndermeye devam ediyor (ayrı yol) — istenirse ayrıca kısılır.
- Test: `tools` (MarkHidden/VisibleLazyCatalog/HiddenLazyCount + aktive-edilebilirlik) +
  `agent` (render pointer + boş durum). **`tools`+`agent` 150 test yeşil**, build temiz.

---

## Generic bildirim sinyalleri: busy / unread / dirty (nav + workspace) ✅ (2026-06-23)

"Haber verme" parçaları (flow/schedule/sohbet/board/artifact değişimleri +
kaydedilmemiş ayar) tek bir generic sisteme toplandı. Her nav görünümü için 3 dik
sinyal: **busy** (accent nabız), **unread** (accent dolu), **dirty** (amber). Hepsi
workspace etiketine yukarı toplanır.

- **Backend**: `api/notify.go` `publishEntityChange` + yeni `board` (task taşıma)
  ve `artifact` (agent sink + UI) event'leri — chat/flow ile aynı SSE borusu.
- **Frontend**: `lib/eventViews.ts` (event→view), `hooks/useUnreadViews.ts`
  (workspace-başına, cross-window persist), `lib/dirtySignals.ts`
  (`useSyncExternalStore` modül store + `useRegisterDirty`). `NavRail` `NavDots`
  ile 3 durumu çizer; `WorkspaceSwitcher`/collapsed ikon aktif workspace'i toplar.
- **Kayıtlı dirty ekranlar**: Settings, WorkspaceView, FlowsPanel.
- **Pencere dışı**: tab başlığı `(N) SwarmGo` (odak dışıyken) + taskbar/dock
  rozeti (`navigator.setAppBadge`, Edge/WebView2'de native taskbar). Toplam
  görülmemiş sayısı `App.tsx` `unreadTotal`. `lib/appBadge.ts`,
  `hooks/useUnreadBadge.ts`.

Detay: `_Docs/29-BILDIRIM-SINYALLERI.md`. Build + tsc yeşil.

## Fix: "Aktivite" nav göstergesi arka plan oturumlarında yanmıyordu ✅ (2026-06-23)

**Sorun:** Bir flow/schedule/agent **ayrı bir oturum** başlattığında (spawn, inbox
teslimi, schedule wake, flow node'ları) sol navbar'daki **Aktivite** öğesinde "işlem
sürüyor" göstergesi çıkmıyordu. Çünkü `handleActivity` yalnızca chat-stream registry'sini
(`s.runs.activeSessionIDs()`) + çalışan task/flow run'larını sayıyordu; otonom invoke'ları
izleyen `Runtime.ActiveSessionIDs()`'i (executions feed'in kullandığı sinyal) **hiç
kullanmıyordu**. Ayrıca `executions` görünümü için bir bayrak yoktu.

**Çözüm:** `activityState`'e `executions` bayrağı eklendi. `handleActivity` artık
chat-stream + `Runtime.ActiveSessionIDs()` oturumlarını birleştiriyor; **herhangi** biri
varsa `executions` yanıyor, ek olarak oturum kind'ı (`chat`/`flow`/`schedule`) ilgili
görünümü de yakıyor. Frontend: `useActivity` → `executions` → `'executions'` View;
`getActivity` tipi güncellendi. `api/activity.go`, `useActivity.ts`, `api/system.ts`.

## UI: ID görünürlüğü + disk yolu erişimi (sohbet / akış / zamanlama / log) ✅ (2026-06-23)

Ajan ekranındaki "ID + klasörü aç/kopyala" deseni diğer ekranlara da yayıldı:

1. **Sohbet listesi (SessionsSidebar)** — her oturum başlığının yanında küçük
   mono **oturum ID'si** (ajanlardaki gibi).
2. **Akışlar (FlowsPanel)** — editör araç çubuğunda **akış ID'si** + **yolu kopyala**
   (`CopyPathButton`) + **klasörü aç** (Explorer `/select`). Yeni backend:
   `GET /api/flows/{id}/path`, `POST /api/flows/{id}/reveal`, `db.FlowPath`.
3. **Zamanlamalar (Schedules)** — her satırda cron ifadesinin yanında mono
   **zamanlama ID'si**.
4. **Loglar (LogsPanel)** — kontrol çubuğunda **log dosyası yolunu kopyala** +
   **klasörü aç**. Loglar artık disk dosyasına da yazılıyor: `SetupLogging`
   stdout + `io.MultiWriter` ile `<dataDir>/logs/swarmgo.log` (append, best-effort).
   Yeni: `config.DefaultDataDir()`, `config.LogFilePath()`, `GET /api/logs/path`,
   `POST /api/logs/reveal` (`api/logs_path.go`).

**Not (workspace rengi):** "kullanılmıyorsa kaldır" istendi ama renk **kullanılıyor** —
NavRail (daraltılmış workspace ikonu) ve WorkspaceSwitcher ikon arkaplan tonu. O yüzden
ayar korundu. API: `api/flows.ts`, `api/system.ts`. Build + tsc yeşil.

## Sıradaki-tur bağlam önizleme (debug) + peer mesajlaşma Faz 2–3 ✅ (2026-06-23)

1. **Sıradaki-tur bağlam önizleme** — Agent ekranındaki bağlam önizlemesinin oturum
   karşılığı: SessionDetailPanel'de **"Bağlam önizle (debug)"** → `SessionContextModal`,
   `GET /api/sessions/{id}/context-preview?message=`. Ajanın bu oturumda sonraki turda
   alacağı tam isteği (sistem + dinamik + **mesaj dizisi** + araçlar, ~token'larla) gösterir.
   **Yan etkisiz** (compaction/persist/provider çağrısı yok; `Prepared` elle kurulur).
   `api/session_context.go`, `SessionContextModal.tsx`, `types/session.ts`, `api/sessions.ts`.
2. **Peer mesajlaşma Faz 2** — yanıt ergonomisi: alıcı `from` adını `to` yapıp yanıtlar
   (araç açıklamasında talimat). Faz 1'de inbox turu zaten geçmiş-duyarlıydı.
3. **Peer mesajlaşma Faz 3 (kısmi)** — **broadcast `"*"`** (`broadcastAgentMessage`,
   best-effort + slot guard, test). Kalan (UI inbox göstergesi + grafik `messaged` kenarı)
   ve **Faz 4** (yapısal protokol) ertelendi.

Testler: `sendmessage_test.go` (+broadcast), `chat_tool_summary_test.go`. Build +
131 test yeşil (cmd/swarmgo-desktop WIP hariç). Detay: `_Docs\07-CHAT-UX.md`,
`_Docs\28-PEER-MESAJLASMA-PLANI.md`.

## Medya/binary artifact desteği (create_artifact sourcePath + auto-capture) ✅ (2026-06-23)

**Sorun (SES30'da görüldü):** Ajan Chrome MCP ile ekran görüntüsü aldı ama PNG'yi
artifact yapamadı; `create_artifact` yalnız inline metin `content` kabul ediyordu,
ajan da binary'yi base64 olarak context'ten geçirmeye zorlanıp token sınırına çarptı
ve "yapısal engel, çözülemez" sonucuna vardı. Oysa depolama (`models_artifact.go`
`image/video/audio/file` kind'ları, `SourcePath`) ve frontend (`ArtifactView`
medya render) zaten hazırdı — eksik olan **ajana açık araç** + **auto-capture'da
medya farkındalığı**ydı.

**Çözüm — iki parça:**

1. **`create_artifact` genişletildi** — `kind` enum'una `image/video/audio/file`
   eklendi + yeni `sourcePath` parametresi. Medya kind'larında bytes context'e hiç
   girmez: dosya yolu verilir, `ArtifactSink.CreateArtifact` artık `CreateArtifactSpec`
   struct'ı alır, sink `db.ImportMediaSource` ile yolu workspace-göreli hale getirir
   (workspace dışındaki dosyayı — ör. Downloads'taki screenshot — `artifacts/<session>/`
   altına **kopyalar**, içindekini olduğu yerden referanslar). Doğrulama: text kind →
   `content` zorunlu, media kind → `sourcePath` zorunlu.
2. **Auto-capture genişletildi** (`artifacts_auto.go`) — (a) `artifactKindForPath`
   medya uzantılarını (`.png/.jpg/.gif/.webp/.mp4/.mp3/.pdf/.zip/...`) doğru media
   kind'a eşler (artık `text`'e düşmez); (b) `Write`/`create_file` medya dosyası
   yazarsa binary-as-text yerine `SourcePath` ile yakalanır; (c) **herhangi bir aracın
   çıktısı** taranır (`extractProducedMediaPaths` + regex) — screenshot/export araçları
   kaydettikleri dosya yolunu döndürünce o dosya da medya artifact'ı olarak yakalanır.
   Var olmayan yol stat'ta elenir → sahte artifact üretilmez.

Her zaman enjekte edilen `artifactDeliverableGuidance` promptu, ajana "binary dosyayı
`sourcePath` ile ver, base64 gömme" talimatıyla güncellendi.

Dosyalar: `tools/artifact.go` (`CreateArtifactSpec`), `tools/builtin_artifact.go`,
`db/artifact_content.go` (`ImportMediaSource`+`copyFileContents`), `api/artifacts.go`
(sink + guidance), `api/artifacts_auto.go`. Testler: `db/artifact_media_test.go`,
`api/artifacts_auto_media_test.go` (+ güncellenen `mcp_interaction_test.go` fakeSink).
Build + tüm api/db/tools testleri yeşil.

## Peer mesajlaşma Faz 1 (send_message/mailbox) + araç I/O geçmiş özeti ✅ (2026-06-23)

İki iş birlikte yapıldı:

1. **`send_message` (Faz 1)** — Claude Code mailbox deseninin uyarlaması: bir ajan
   başka ajana **adresli, kimlikli** mesaj atar; mesaj alıcının kalıcı **inbox**
   oturumuna (`GetOrCreateKindSession` kind="inbox") `<agent_message from="…">`
   etiketiyle düşer ve alıcının **geçmiş-duyarlı turu** arka planda çalışır
   (fire-and-forget, `SpawnMaxConcurrent` guard, kendine-mesaj reddi). self-manage
   gated. `run_subagent` (izole görev) ile birlikte durur; bu "süregelen peer
   işbirliği" yolu. Dosyalar: `agentmsg.go`, `builtin_sendmessage.go`, `toolsetup.go`;
   `runSessionTurn` ile wake/inbox ortak geçmiş-duyarlı runner. Plan/detay:
   `_Docs\28-PEER-MESAJLASMA-PLANI.md`.
2. **Araç I/O geçmiş özeti (#5)** — geçmişte araç çağrı/sonuçları düşüyordu; artık
   son N=4 asistan turunun `Steps` izinden kompakt `<recent_tool_activity>` bloğu
   (kopya üzerinde) eklenir → ajan "az önce ne yaptın / ne döndü"yü yanıtlar
   (`api/chat_tool_summary.go`). Wake/inbox turları da dahil.

Testler: `sendmessage_test.go`, `chat_tool_summary_test.go` (+ mevcutlar). Build +
241 test yeşil (cmd/swarmgo-desktop'taki ilgisiz WIP hariç). Detay:
`_Docs\07-CHAT-UX.md`, `_Docs\28-PEER-MESAJLASMA-PLANI.md`.

## MCP kalıcı bağlantı havuzu (persistent pool) ✅ (2026-06-23)

Daha önce belgelenen 🔴 kısıt (dinamik MCP araç ekleme / `tools.listChanged` yok +
dial-per-operation) **kalıcı olarak çözüldü**. Önceki ara çözüm (60sn katalog TTL cache,
`mcpcatalog.go`) **kaldırıldı**, yerini sunucu başına **canlı oturum havuzu** aldı.

**`internal/mcp/client.go` (yeniden yazıldı):** stdio istemci artık **kalıcı + eşzamanlı
kullanıma güvenli**. Tek arka-plan **read loop** yanıtları JSON-RPC `id`'ye göre per-call
kanallara demux eder; sunucu bildirimleri (`notifications/tools/list_changed`) bir
callback'e yönlenir. `initialize` artık `capabilities.tools.listChanged=true` ilan eder.
Yeni API: `SetOnToolsChanged`, `Alive`. (`proc.Command` süreç-grubu kill korundu.)

**`internal/mcp/pool.go` (yeni):** `Pool` her enabled MCP server için **tek canlı
`StdioClient`** tutar (sanitized ada göre).
- **Catalog reuse:** tur-başı tekrarlı `buildRegistry` çağrıları aynı canlı oturumu
  kullanır → gateway'de **session churn yok** (eski burst sorunu da tamamen biter).
- **Oturum-durumu korunur:** gateway'in `activate_tools` etkisi oturum kapanmadığı için
  **sonraki çağrıda da yaşar** → dinamik araç ekleme artık çalışır (tur-ötesi).
- **listChanged → invalidate:** sunucu araç listesi değişince entry stale işaretlenir,
  sonraki `Catalog` aynı canlı oturumda yeniden listeler. Ek emniyet: TTL
  (`SWARMGO_MCP_POOL_TTL_SEC`, vars. 60sn) — listChanged göndermeyen sunucular için.
- Config (command/args/url/env fingerprint) değişiminde veya bağlantı ölümünde şeffaf
  re-dial; çağrı ölü bağlantıda bir kez retry eder.

**Wiring:** `Runtime.mcpPool *mcp.Pool` (+ `CloseMCP()`, workspace teardown'da çağrılır:
`workspace/manager.go` delete + Close). `buildRegistry` → `pool.Catalog`. `Registry`
artık `mcpCaller` taşır (`AttachMCP(..., caller)`); MCP çağrıları pool üzerinden gider,
nil ise dial-per-call'a düşer (testler). Tek-seferlik test endpoint'i
(`api/mcp.go handleTestMCPServer`) + `live_test.go` hâlâ dial-per-op `ListServerTools`/
`CallNamespaced` kullanır (kasıtlı).

**Kalan nüans:** Aynı tur içinde `activate_tools` sonrası yeni araçlar **bir sonraki turda**
çağrılabilir (tur başı `reg` sabit). Pre-load için preset hâlâ en pürüzsüz yol; ama artık
zorunlu değil. Test: `internal/mcp/pool_test.go` (sahte stdio MCP sunucusu: persistent
reuse, listChanged refresh, config-change re-dial, Close, hata yolları). **`internal/mcp`
+ `internal/agent` + `internal/tools` + `internal/workspace` 153 test yeşil**; build+vet
temiz (ilgisiz `internal/e2e` MemGPT WIP build hatası hariç).

---

## Bilinen kısıt — dinamik MCP araç ekleme (`tools.listChanged`) desteklenmiyor 🔴→✅ (2026-06-23)

Çok-ajan "kim ne dedi" çözümünü iki referansla karşılaştırdık:
- **external-agent-oss:** sorunu *yaşamıyor* — bir oturum = tek ajan; çok-ajan ayrı oturum.
  Mesajlarda per-mesaj yazar alanı yok; Claude Agent SDK döngüyü sürüyor.
- **Claude Code (`observed-behavior` swarm/teammate):** çok-ajanı **izole bağlam + adresli
  mailbox** ile çözüyor — `SendMessage({to,message,summary})`, alıcının inbox'ına `from`
  kimliğiyle `<teammate_message teammate_id>` etiketiyle düşer; plain çıktı diğer ajana
  görünmez. Kimlik **doğuştan**; ardışık-rol çakışması hiç oluşmaz.

SwarmGo iki modeli birden taşıyor: paylaşılan-thread (etiketleme+coalesce ile sağlamlaştırıldı)
ve izole `run_subagent`. Eksik olan "akran ajana adresli DM" için **uyarlama planı** yazıldı:
`_Docs\28-PEER-MESAJLASMA-PLANI.md` (mevcut `GetOrCreateKindSession` inbox + `SpawnSession`
üzerine). Kavramsal not: `_Docs\10-KAVRAMSAL-TASARIM-NOTLARI.md` §10. **Uygulama kullanıcı
onayı bekliyor** (tetik/inbox modeli/ayrı-araç kararları planda).

## Çok-ajanlı bağlam sağlamlığı: yazar kimliği + ardışık-rol + geçmiş-duyarlı wake ✅ (2026-06-23)

**Bağlam:** SES29'da iki ajana soru soruldu ama ajanlar "kim ne dedi"yi göremedi.
Kök neden: `toProviderMessages` geçmişi çevirirken `AgentID`'yi düşürüyordu → tüm
asistan turları tek ayrımsız "assistant" sesine karışıyordu. Düzeltme + aynı sınıftan
diğer kusurlar tarandı; en kritik 3'ü (+ kullanıcı-hedefi) kapatıldı:

1. **Yazar etiketleme** (önceki tur) — çok-yazarlı geçmişte her asistan turu yazarıyla
   ön-eklenir (`api/chat_authors.go`); kendi turlarına `(you)` markerı. **Güncelleme
   (2026-06-23):** etiketleme eşiği "2+ farklı yazar"dan "geçmişte **yanıtlayan ajandan
   farklı** bir yazar var mı"ya genişletildi → ajanı değiştirilmiş (devredilen) oturumda
   da (tek önceki yazar A, şimdi B yanıtlıyor) A'nın turları etiketlenir, B onları
   kendisininki sanmaz. Saf tek-ajan oturumu (yalnız yanıtlayan konuşmuş) hâlâ etiketsiz
   (doğal transkript + prompt cache korunur).
2. **Geçmiş-duyarlı wake** — `schedule_wake` ile uyanan ajan eskiden yalnız wake
   prompt'unu görüyordu (`invokeTraced`, geçmiş yok). Artık `WakeTurnFunc` hook'u
   (`api/wake_turn.go`) tam sohbet turunu (geçmiş+özet+hafıza+goal) kurar. Detay:
   `_Docs\20-SCHEDULE-WAKE.md`.
3. **Ardışık aynı-rol birleştirme** — bir kullanıcı mesajına 2 ajan ardışık yanıtlarsa
   `user→assistant→assistant` oluşuyor, Anthropic "roles must alternate" ile reddediyordu.
   `providers/coalesce.go` ardışık aynı-rol düz-metin turları birleştirir (araç turlarına
   dokunmaz); anthropic + minimax çeviricilerinde uygulanır.
4. **Kullanıcı mesajının hedef ajanı** — kullanıcı mesajı artık yönlendirildiği ajanla
   (`AgentID = agents[0]`) damgalanır; geçmişte `"[User → Ada]: …"` etiketlenir → "hangi
   soru kime" de görünür. `@Ad` yalnız bilgi amaçlı (yönlendirme değil).

Testler: `chat_authors_test.go`, `providers/coalesce_test.go` (+ mevcutlar). Tüm
build + 167 test yeşil. Detay: `_Docs\07-CHAT-UX.md`, `_Docs\20-SCHEDULE-WAKE.md`.

## Bilinen kısıt — dinamik MCP araç ekleme (`tools.listChanged`) desteklenmiyor 🔴 (2026-06-23)

**Bulgu (gerçek vaka):** Bir ajan MCP Gateway üzerinden `mcp-chrome`'u kullanmak istedi.
Gateway'in `activate_tools('mcp-chrome')` çağrısı **"✅ 29 tools activated"** döndü ama
ardından `chrome_navigate` çağrısı **`No such tool available`** verdi. Gateway'in kendisi
uyardı: *"Your client did not advertise tools.listChanged support… reconnect with a preset."*

**Kök neden — iki birleşen mimari gerçek:**
1. **`tools.listChanged` yok:** istemci `initialize`'da `capabilities:{}` gönderir
   (`internal/mcp/client.go`), yani sunucu "araç listem değişti" bildirimini gönderse bile
   SwarmGo `tools/list`'i yeniden çağırmaz.
2. **Dial-per-operation (havuzsuz):** `BuildCatalog`/`CallNamespaced` her işlemde **yeni
   session** açıp kapatır. Gateway'in `activate_tools`'u **oturum-kapsamlıdır** → araçları
   o anlık session'a ekler, session `Close()` ile kapanınca kaybolur. Eklenen araçlar
   SwarmGo'nun kataloğuna hiç girmez → çağrılamaz.

→ Sonuç: **runtime'da araç ekleyen/çıkaran MCP sunucularıyla SwarmGo uyumsuz.**

**Geçici çözüm (uygulandı):** İstenen araçlar sunucunun bağlantı URL'indeki **preset'e**
konur; preset her taze session'da başlangıçta yüklendiği için dial-per-operation modeliyle
sorunsuz çalışır. MCP Gateway `swarmgo` preset'ine `mcp-chrome` eklendi
(`mcp-server/config.json`: `swarmgo: [<remote-service>, mcp-chrome]`); `?preset=swarmgo` artık
47→**76 araç** döndürüyor. Doğrulandı.

**Kalıcı çözüm (Sırada / öneri):** ya (a) `initialize`'da `tools.listChanged` ilan edip
**kalıcı session** tut + bildirimde `tools/list`'i yenile, ya da (b) gateway gibi
dinamik sunucular için kalıcı bağlantı havuzu (Seçenek 2) — böylece `activate_tools`
etkisi sonraki çağrıda da yaşar. İkisi de aynı `internal/mcp` yeniden tasarımına bağlanır.

---

## Reflection budama + auto-reflect eşiği 30→20 ✅ (2026-06-23)

Dream cycle ayarları ince ayarlandı: ham journal gürültüsü daha erken damıtılsın diye
**`autoReflectThreshold` default 30 → 20**; ve reflection'lar (journal'ların aksine
budanmıyordu → süresiz birikip recall havuzunu kirletiyordu) artık **bounded**.

- **Yeni `reflectionCap`** (default 20, clamp 1–1000): her dream cycle sonunda
  `reflect()` en yeni N reflection'ı tutup eskileri budar (`PruneKind(reflection)`).
- Wiring: `Tunables.SetReflectionCap/ReflectionCap` + `DefaultReflectionCap=20`;
  `settings` (Settings/DTO/Patch/Default + clamp) + `applySettings`; frontend
  "Yansıma limiti" alanı. `reflector.go`'da budama + `reflectionCap()` helper.
- `go build` ✅, **148 test** ✅ (tunable default + reflection-prune), `tsc` ✅.

## HA-1: human bloğu otomatik kullanıcı modelleme (MemGPT Parça 4b) ✅ (2026-06-23)

`human` çekirdek bloğu artık dream-cycle ile **otomatik** doldurulur. `reflect()`
her çalıştığında (manuel/auto), yansımadan sonra journal'lar silinmeden önce
`updateUserModel` çağrılır: model mevcut profili + journal'ı alıp kullanıcı
hakkındaki kalıcı çıkarımları kısa satırlar olarak merge eder, `human`'a yazar.

- **Yeni dosya** `internal/agent/user_model.go` (`updateUserModel`/`writeUserModel`
  + prompt); `reflector.go`'ya tek `if r.tun.UserModel()` satırı. `runtime.go`
  **dokunulmadı**. Usage `KindReflect`'e yazılır (dream-cycle maliyeti).
- **Best-effort**: değişiklik yoksa no-op, limit aşılırsa truncate, hata yansımayı
  bozmaz. **Ayar** `AutoUserModel` (varsayılan açık) — settings + Tunables +
  applySettings + frontend toggle.
- `go build` ✅, **206 test** ✅, `tsc` ✅. Detay: `31-MEMGPT-CORE-MEMORY.md` Parça 4b.

## Çekirdek bellek: adlandırılmış bloklar + karakter limiti (MemGPT Parça 5) ✅ (2026-06-23)

Letta'nın **memory blocks** modeli native getirildi. Sabit persona/human ikilisi,
ajanın istediği etikette tanımlayabildiği **dinamik bloklar**a genelleşti; her blok
**karakter limiti + açıklama + salt-okunur** taşır.

- **Encoding** `kind="core:"+label` (`db.CoreKind/IsCoreKind`); tanım ajan dosyasında
  (`Agent.CoreBlocks`), içerik `knowledge_sources`'ta. `CoreBlocks` boşsa varsayılan
  persona+human (2000 char) → migration yok.
- **Store**: `WriteCore/ReadCore/AppendCore(label)` + limit (`*CoreBlockFullError`),
  `ErrUnknownCoreBlock`, `ReadCoreBlocks→[]BlockView`, `DefineCoreBlock`/`DeleteCoreBlock`.
- **Araç**: `section`→`label` (alias korundu); bilinmeyen/read-only/limit hatası ajana
  net döner. Constructor imzaları sabit → `runtime.go`/`toolsetup.go` **dokunulmadı**.
- **API**: `GET /core→{blocks}`, `PUT /core {blocks:{label:content}}`, `POST/DELETE
  /core/blocks[/{label}]`. **Frontend**: `CoreMemoryCard` dinamik + limit çubuğu +
  blok ekle/sil + read-only kilit.
- Doğrulama: `go test` 141 ✅, `tsc` ✅. Detay: `31-MEMGPT-CORE-MEMORY.md` Parça 5.

## Composer: oturum-başına taslak + ikon-tabanlı kontroller ✅ (2026-06-23)

İki UX iyileştirmesi (kullanıcı isteği). Yalnız frontend, `tsc --noEmit` temiz.

1. **Oturum-başına taslak.** Yeni `useSessionDraft` hook'u (`hooks/useSessionDraft.ts`):
   composer'a yazılıp **gönderilmeyen** metin `localStorage`'da oturum-id ile saklanır
   (`swarmgo:draft:<sessionId>`). Oturum değiştirip dönünce ve sayfa yenilenince korunur;
   gönderme/temizleme taslağı siler (boş taslak saklanmaz). Composer `useState('')` yerine
   bu hook'u kullanır — tüm mevcut `setText` çağrıları otomatik kalıcı. **Yan fayda:** eskiden
   metin oturumlar arası sızıyordu (Composer `key`'siz, monte kalıyor); artık her oturum kendi taslağını taşır.
2. **İkon-tabanlı composer kontrolleri.** Ajan seçici yalnız avatar (ad tooltip'te); düşünme
   seviyesi yoğunluk ikonuyla (◌○◔◑●); izin modu emoji ikonuyla (🛡🔒✋⚡). `ComposerPicker`'a
   `iconOnly` prop'u eklendi; `THINKING_OPTIONS`'a seviye ikonları eklendi.

---

## E2E test paketi — uçtan uca ajan davranışları ✅ (2026-06-23)

Yeni **`internal/e2e`** test paketi: tam kablolu bir `agent.Runtime` (gerçek dosya-store,
memory, skills, sandbox, `conversation.Manager`) `api/chat_stream`'in sürdüğü tur hattının
**aynısıyla** sürülür; yalnız LLM, ağsız-deterministik bir **`scriptedProvider`** ile
değiştirilir. Provider sıraya konmuş yanıtları kuyruktan tüketir (metin turu veya `tool_use`
turu → native araç döngüsü gerçek araçları çalıştırır), `conversation.Manager`'ın rolling-summary
özetleme çağrısını ise script'i bozmadan yakalayıp yanıtlar. Harness (`harness_test.go`) her turda
kullanıcı mesajını kalıcılaştırır, geçmişi bütçeleyici üzerinden tekrar oynatır, sistem+memory
bağlamını dizer, akışlı araç döngüsünü koşar, yanıtı kalıcılaştırır + journal'lar.

**Kapsam (16 test, hepsi yeşil):**
- **Konuşma:** çok-turlu geçmiş kalıcılığı + ikinci tura ilk alışverişin tekrar oynatılması (`conversation_e2e_test.go`).
- **Araç kullanımı:** tek turda `Write`→`Read` çok-adımlı döngü (dosya gerçekten diske düşer, sonuç cevaba akar, `StepDiff` izi), shell-gate (`tools_e2e_test.go`).
- **Skill kullanımı:** workspace skill oluştur → katalogta slug+özet (gövde lazy) → `use_skill` ile gövde yükleme; ayrıca per-agent allowlist ile kısıtlı skill erişilemezliği (`skills_e2e_test.go`).
- **Hafıza:** `core_memory_replace` ile kalıcı çekirdek bellek + sonraki tura tekrar enjeksiyon; `memory_recall` ile uzun-dönem hatırlama (`memory_e2e_test.go`).
- **Uzun oturum:** bütçe aşımında compaction tetiklenmesi (özet kalıcı, yalnız son tur'lar verbatim) (`longsession_e2e_test.go`).
- **İzin modu:** read-only modda yazma engellenir/okuma serbest; ask modunda interaktif prompter ile onay/red (yazma diske düşer ya da engellenir, `permission_denied` izi) (`permission_e2e_test.go`).
- **Self-wake:** `schedule_wake` ctx'teki scheduler'a doğru delay/prompt ile ulaşır; scheduler yokken (headless) net hata (`wake_e2e_test.go`).
- **Delegasyon:** delegasyon kapalıyken `run_subagent` kayıtlı değil (unknown tool); açıkken bilinmeyen hedef target-çözümleme guard'ıyla reddedilir (`delegation_e2e_test.go`). Not: alt-ajan provider'ı registry'den çözüldüğü için (anahtarsız) mutlu-yol e2e'si üretim kodu değişmeden test edilemez; gate+guard yolları kapsandı.
- **Çok-ajanlı tur:** tek kullanıcı mesajı, iki ajan sırayla yanıtlar; ikinci ajan birincinin cevabını geçmişte görür (`multiagent_e2e_test.go`).

Harness genişletildi: `decorate` ctx-kancası (prompter/grants/wake enjeksiyonu) + `sendMulti` (çok-ajanlı tur sürücüsü).
İzolasyon: `SWARMGO_DATA_DIR` temp'e yönlendirilir → gerçek `~/.swarmgo` skill/market seed'ine dokunulmaz.
✅ `go test ./internal/e2e/` 16/16 yeşil, `go vet` temiz.

## Native pencere — konsol penceresi yanıp sönmesi düzeltildi ✅ (2026-06-23)

Konsolsuz desktop binary (`-H windowsgui`) bir konsol alt-süreci başlattığında Windows'un
çocuk için açtığı terminal penceresi yanıp sönüyordu (claude CLI/PowerShell/git/MCP). Yeni
**`internal/proc`** paketi (`Hide(cmd)` → Windows'ta `CREATE_NO_WINDOW`, diğer platformlarda
no-op) konsol açan tüm `exec.Command` çağrılarına eklendi: `claudecli.go` (ana suçlu),
`builtin_shell.go`, `agent/hooks.go`, `agent/worktree.go`, `mcp/client.go`, `api/git.go`,
`workdir_context.go`, `workspaces.go`. `explorer.exe` (GUI) dokunulmadı. ✅ build/vet +
305 test yeşil. Detay: [32-NATIVE-PENCERE.md](32-NATIVE-PENCERE.md).

## Native pencere — başlık çubuğu temaya uyumlu ✅ (2026-06-23)

WebView2 penceresinin native başlık çubuğu (caption + küçült/büyüt/kapat butonları + kenarlık)
artık uygulama temasına boyanıyor — beyaz Windows frame'i koyu temayla çelişmiyor. **DWM** ile
(`dwmapi.dll` `DwmSetWindowAttribute`, salt `syscall`, yeni bağımlılık yok): `DWMWA_USE_IMMERSIVE_DARK_MODE`
(Win10 1809+) + `DWMWA_CAPTION_COLOR`/`TEXT_COLOR`/`BORDER_COLOR` (Win11 22000+). `app.App.Appearance()`
çözülen `ThemePreset`/`Theme`/`Accent`'i verir; `cmd/swarmgo-desktop/titlebar_windows.go` 8 curated paletin
bg/text/border'ını (`themePresets.ts` ile elle senkron) COLORREF'e (`0x00BBGGRR`) çevirir. Bilinmeyen
preset → yalnız dark/light frame (caption rengi atlanır); eski Windows'ta desteklenmeyen attribute'lar
sessizce yok sayılır (pencere yine çalışır). **Canlı güncelleme:** `watchTitleBar` 1.5sn poll ile tema
değişince `w.Dispatch` üzerinden yeniden uygular (Ayarlar'dan palet değiştirince başlık anında uyar).
✅ `go build ./...`/`vet` yeşil; desktop canlı (varsayılan midnight-violet, pencere açıldı, health 200,
panik/hata yok). Detay: [32-NATIVE-PENCERE.md](32-NATIVE-PENCERE.md).

## Native masaüstü penceresi — WebView2 (CGO'suz) ✅ (2026-06-22)

SwarmGo artık tarayıcı yerine **kendi masaüstü penceresinde** açılabiliyor. Plan:
[32-NATIVE-PENCERE.md](32-NATIVE-PENCERE.md). **Ön koşul refactor (davranış-korumalı):**
`cmd/swarmgo/main.go`'nun boot dizisi yeni **`internal/app`** paketine taşındı
(`SetupLogging` + `Bootstrap`/`Serve`/`Shutdown`/`Addr`/`URL`); `Bootstrap` artık listener'ı
önden açar (`net.Listen`, `:0` boş port desteği) ve `SetBaseURL`'i çözülen adresle çağırır.
`main.go` ~130→~50 satır. **Yeni giriş noktası** `cmd/swarmgo-desktop` (`//go:build windows`,
[`jchv/go-webview2`](https://github.com/jchv/go-webview2) — **saf Go, CGO yok**; Win11'de
yerleşik WebView2 runtime): sunucuyu `127.0.0.1:0`'da başlatır, `waitForHealth` ile hazır olunca
1280×800 WebView2 penceresi açar, pencere kapanınca graceful `Shutdown`. WebView2 yoksa →
varsayılan tarayıcıya fallback (`rundll32 url.dll`). `!windows` stub mevcut. `scripts/build.ps1`
`-Desktop` bayrağı (`-H windowsgui` → konsolsuz). Bağımlılık: `go-webview2` (direct) +
`go-winloader`/`x/sys` (indirect) — yalnız desktop hedefinde derlenir; **başsız `swarmgo`
hâlâ saf-Go/çapraz-derlenebilir**. ✅ `go build ./...`/`vet` yeşil; başsız smoke (refactor sonrası
`/`+`/health` 200, boot logları aynı); desktop canlı (rastgele port 60385'te boot, `/health`+`/`
200, pencere açıldı); `-H windowsgui` build 12 MB. README "Native Masaüstü Uygulaması" eklendi.

## Prompt saatine saniye eklendi (SES28 zaman-ölçüm hatası) ✅ (2026-06-22)

**Sorun:** SES28'de bir ajan "30 sn bekle, farkı ölç" görevinde ilk saati **uydurdu**
(`18:18:48`). Kök neden: sistem-prompt saati yalnız **dakika** hassasiyetindeydi
(`15:04`), `get_current_time` aracı da kaldırılmıştı → saniye gereken ölçümde ajanın
gerçek saati yok, uyduruyor. (Not: `schedule_wake` timer'ı doğru — tam 30 sn tetikledi;
hata zamanlayıcıda değil, uydurmadaydı.)

- **Çözüm (geçici):** `dateTimeContextBlock` (chat) + `autonomousSystemPrompt` (headless)
  artık `15:04:05` (saniyeli) yazıyor ve satır "tur başında yakalandı, tur içinde ilerlemez"
  diye etiketli — ajan ölçüm görevinde bunu **baseline** alsın, uydurmasın.
- **Kalıcı (sırada):** hafif, her-zaman-açık `get_current_time` aracı (saniye + unix epoch)
  — bilerek kaldırılmıştı; kullanıcı onayı bekliyor.
- **Doğrulama:** `go build ./...` + `go vet` temiz.

---

## Ajan path/reveal + oturum yolu ~ gösterimi + context-mode dedektörü ✅ (2026-06-22)

Üç küçük UI/UX iyileştirmesi (kullanıcı isteği). Build + `tsc --noEmit` temiz.

1. **Ajan dosya yolu / klasör aç.** Ajan ayarları formuna (`AgentSettingsForm`) sağ
   üstte **Yolu kopyala** + **Klasörü aç** butonları eklendi. Backend: `db.AgentPath`
   (ajanın `agents/<id>.json` mutlak yolu) + `GET /api/agents/{id}/path` +
   `POST /api/agents/{id}/reveal` (`explorer.exe /select,<path>` ile dosyayı vurgular).
   Frontend api: `agentApi.agentPath`/`revealAgent`. Session reveal deseninin ajan eşleniği.
2. **Oturum yolu `~` gösterimi.** `SessionDetailPanel` "Klasör" bölümü `info.path`'i ham
   gösteriyordu; artık `displayPath()` ile `~\...` kısaltmasıyla gösterir (title'da tam yol;
   "Yolu kopyala" hâlâ tam yolu kopyalar).
3. **context-mode dedektörü.** Hooks "Harici token araçları" listesine (`external_tools.go`)
   `context-mode` eklendi (rtk/sqz yanında).

---

## MCP katalog önbelleği — gateway'de session birikmesi düzeltildi ✅ (2026-06-22)

**Sorun:** Yerel MCP Gateway'de saniyeler içinde 4 ayrı `swarmgo` session açılıyordu
(her biri `requestCount:3`). Kök neden: native MCP istemcisi **havuzsuz** (`manager.go`
dial-per-operation) ve `buildRegistry` tek bir sohbet turunda birden çok kez çağrılıyor
(tur girişi `runtime.go`, native döngü `toolloop.go`, UI/araç önizleme endpoint'leri).
Her çağrı `BuildCatalog` ile **her enabled MCP server'ı yeniden dial ediyordu**
(`initialize` + `notifications/initialized` + `tools/list` = requestCount 3), ardından
`Close()`. Gateway HTTP-köprülü olduğu için her dial yeni bir session açıyor; stdio
istemci HTTP `DELETE` göndermediğinden gateway session'ları idle olarak birikiyordu.

**Çözüm — workspace başına katalog önbelleği** (`internal/agent/mcpcatalog.go`):
- `Runtime.mcpCat *mcpCatalog` — dial edilmiş katalogu (entries) bellekte tutar.
- **Fingerprint-tabanlı geçersizleme:** anahtar = enabled server config'lerinin
  SHA-256 fingerprint'i (sıra-bağımsız). Server toggle/ekle/sil/düzenle → fingerprint
  değişir → otomatik rebuild. **Ayrı invalidation hook'u gerekmez.**
- **TTL:** varsayılan **60 sn** (`SWARMGO_MCP_CATALOG_TTL_SEC` ile override; `0` =
  önbellek kapalı, eski davranış). Config'in göremediği dış değişiklikleri (server
  farklı tool sunması) sınırlar.
- Sadece pahalı dial sonucu (entries) önbelleklenir; ucuz dispatch haritası
  (`cfgByServer`) her çağrıda canlı server listesinden yeniden hesaplanır → asla
  drift etmez. Nil-receiver toleranslı (bare `&Runtime{}` testleri önbeklsiz çalışır).
- **Etki:** tur-başı dial burst'ü her server için **1**'e iner → gateway session
  birikmesi ~%90 azalır.
- Test: `mcpcatalog_test.go` (cache hit/miss, fingerprint geçersizleme, TTL süresi,
  ttl=0, nil-receiver, env parse). `internal/agent` + `internal/mcp` **81 test** yeşil.

> **Kalan (tam çözüm değil):** `CallNamespaced` (gerçek tool çağrısı) hâlâ dial-per-call.
> Asıl tool kullanımı seyrek olduğu için burst kaynağı değil; kalıcı bağlantı havuzu
> (Seçenek 2) veya native HTTP transport + `Mcp-Session-Id` reuse ileride değerlendirilebilir.

---

## Bütçe ceil/fraction artırıldı + eskimiş 1M-token ayarı kaldırıldı ✅ (2026-06-22)

**Hedef:** (1) 1M modelleri daha çok kullan, (2) eskimiş 1M-token beta ayarını temizle.

- **Ceil/fraction (1M kullanımını artır):** `budgetWindowFraction 0.10→0.20`,
  `budgetAutoCeil 32K→128K`, tool-eşik scale clamp `[1,5]→[1,12]`. Artık 1M modeller
  **128K** transcript bütçesi (≈%12.8), Haiku **40K**. 1M'de ceil operatif knob.
  `budget_test.go` + `tunables_compact_test.go` güncellendi. **124 test** yeşil.
- **Eskimiş 1M-token ayarı kaldırıldı:** Web doğrulaması — Anthropic 1M'i **13 Mart 2026**
  GA yaptı (header gerekmez), `context-1m-2025-08-07` beta header'ı **30 Nisan 2026**
  kapatıldı. Bugün 22 Haziran → tamamen işlevsiz.
  - **Provider:** `anthropic.go` artık header'ı **göndermiyor** (const + field + append
    silindi; `WithBetas(_, extendedCache)`). 43 test + vet temiz.
  - **Frontend:** Ayarlar → Anthropic beta'daki "1 milyon token bağlam" toggle'ı +
    `oneMillionContext` tipi/payload referansları kaldırıldı (appPanels + types +
    SettingsPanel). `npm run build` yeşil.
  - **Kalan (entangle):** backend `settings.OneMillionContext` alanı + `server.go`
    `SetAnthropicBetas` argümanı + registry/ResolvedConfig plumbing vestigial kaldı
    (zararsız, header gitmiyor). `api` paketi paralel MemGPT WIP'inden kurtulunca
    tek temiz commit'te purge edilecek (server.go o dosyada entangle).

---

## `http_get` → `WebFetch` zengin fetch ✅ (2026-06-22)

`http_get` (düz GET, 64KB) **`WebFetch`'e yükseltildi** — claude-cli'nin WebFetch'inin
native eşleniği, son web-parite boşluğu kapandı. Build+vet temiz, **300 test** yeşil,
`tsc --noEmit` temiz, backend rebuild+restart.

- **HTML→Markdown converter** (`internal/tools/htmltomarkdown.go`, **stdlib-only** —
  go.mod minimal kalır, `x/net/html` yok): regexp geçişleriyle script/style/nav/form
  blokları atılır; başlık/link/liste/bold/italic/code/pre → Markdown; göreli linkler
  base URL'e çözülür; `html.UnescapeString` ile entity çözme; whitespace temizliği.
- **`WebFetchTool`** (`builtin_http.go`): SSRF-guard'lı dialer korunur (loopback/
  private/link-local + 169.254 metadata + CGNAT engeli, redirect/DNS-rebind kapsanır).
  HTML→Markdown; metinsel içerik (md/plain/json/xml) verbatim; ikili içerik özetlenir
  (dump edilmez). `raw=true` ham gövde döndürür. Ham indirme 3MB, çıktı 96KB cap.
  Yönlendirme sonrası final URL başlıkta.
- **İsim:** `http_get`→`WebFetch` (classify zaten `WebFetch=RiskRead` taşıyordu;
  `bridgeExcluded` anahtarı güncellendi — CLI kendi WebFetch'ini kullandığından bizimki
  köprülenmez). Referanslar: `toolsetup`/`subagent`/`registry`/frontend `tools.ts` +
  default skill + testler (`tooltier`/`lazyload`/`builtin_http`). Yeni test:
  `htmltomarkdown_test.go` (4 senaryo). Dokümanlar: `09-SDK`, `11-INTERACTION`, `19-LAZY`.

## Tool eşikleri per-model bütçeye hizalandı + model picker rozeti ✅ (2026-06-22)

**Hedef:** (1) §5 tool eşiklerini de per-model bütçeye bağla (zinciri tutarlı kıl),
(2) frontend model picker'da context-window'u göster.

- **Tool eşikleri per-model:** `Tunables`'a `CompactMaxBytesFor(budget)` /
  `CompactLLMThresholdFor(budget)` + `ContextBudgetTokens()` getter + saf
  `budgetScaleFor(budget)`. Compactor her tur `conversation.EffectiveBudget(
  provider, model, ContextBudgetTokens())` hesaplayıp For-varyantlarına geçiyor.
  No-arg getter'lar process-geneli bütçeyle geriye-uyumlu. Zincir: model penceresi
  → EffectiveBudget → eşik ölçeği. `tunables_compact_test.go` (For). Agent **71 test**.
- **Model picker rozeti:** `CatalogModel.contextWindow?` (TS) + `ProviderModelSelect`
  `formatContextWindow` (200000→"200K", 1000000→"1M"); dropdown option'larında
  "· 200K" + seçili modelde "200K bağlam" rozeti. `tsc` + `npm run build` yeşil.
- **Not:** Hepsi agent/conversation/providers/frontend'de — `api` paketine dokunulmadı.

---

## claude-cli `use_skill` isim uyuşmazlığı düzeltmesi ✅ (2026-06-22)

**Hedef:** SES21'de açık kalan konu — claude-cli ajanı ilk turda `use_skill`'i çağırınca
"No such tool available: use_skill" alıyordu (ikinci turda kendini toparlıyordu).

- **Kök neden (yarış değil, isim uyuşmazlığı):** "# Available Skills" prompt bloğu modele
  **çıplak** `use_skill` adını söylüyordu (`skills/store.go renderCatalog`). Native
  ajanlarda araç gerçekten `use_skill`; ama **claude-cli** ajanlarında SwarmGo built-in'leri
  Interaction MCP köprüsünden **namespaced** geliyor: `mcp__swarmgo_interaction__use_skill`.
  Model prompt'u harfiyen izleyip çıplak adı deniyor → CLI reddediyor. (`trace.go` namespace'i
  soyduğu için başarılı 2. çağrı izde yine `use_skill` görünüyor — kafa karıştırıcı.)
- **Çözüm:** Katalog bloğu artık aracı **ajanın göreceği adla** yazıyor. `renderCatalog`
  skill-araç-adı parametresi aldı; `skills.DefaultSkillTool` sabiti + yeni
  `CatalogBlockForAgentTool(assigned, skillTool)`. `runtime.go skillToolNameFor(provider)`:
  provider `""`/`claude-cli` → namespaced, diğerleri (anthropic/minimax/openrouter/custom)
  → çıplak. `SkillsCatalogBlockForAgent` bunu kullanıyor (chat + autonomous yolları).
- **Doğrulama:** `TestSkillToolNameFor` (6 vaka) + `TestCatalogBlockForAgentTool`;
  `internal/agent` & `internal/skills` **88 test** yeşil, `go vet` temiz.

---

## SES21 hava-durumu oturumu hata düzeltmeleri ✅ (2026-06-22)

**Hedef:** Bir kullanıcı oturumunda (SES21, claude-cli ajanı) çıkan üç somut hatayı gider.

- **Shell `$` değişkeni bozulması (kök neden):** Ajan komutunu zaten-PowerShell olan
  `Bash` aracının içinde tekrar `powershell -Command "..."` ile sarmaladı → dış kabuk
  `$geo` vb. değişkenleri iç kabuğa geçmeden boşaltıp sildi ("An empty pipe element").
  Çözüm: (1) araç açıklamasına "tekrar `powershell -Command` ile sarmalama" uyarısı,
  (2) `unwrapRedundantPowershell` ile gereksiz dış sarmalı savunmacı olarak açma (yalnız
  tüm komut sarmalsa ve iç tarafta kaçışlı tırnak yoksa). `builtin_shell.go` + test.
- **Artifact geri okunamıyor:** `list_artifacts` yalnız id/başlık veriyordu; ajan
  içeriği göstermek için dosya yolunu (`.artifacts\...`) tahmin edip "File does not exist"
  aldı. Çözüm: yeni **`read_artifact`** aracı (id → içerik, bellekten; yol tahmini yok) +
  `list_artifacts` artık `contentFile` yolunu da döndürüyor. İkisi de `RiskRead`.
  `builtin_artifactmgmt.go`, `classify.go`, `toolsetup.go`.
- **Veri uydurma (skill davranışı):** `daily-weather-report` skill'i WebFetch başarısız
  olunca uydurma tablo yazıyordu. Skill'e eklendi: çok-şehir için Open-Meteo geocoding,
  weather.com kazımayı yasakla, **asla veri uydurma**, forecast≠iklim ortalaması,
  alt-ajana da aynı kuralı geçir.
- **Doğrulama:** `go build ./...` + `internal/tools` & `internal/db` **85 test** yeşil.
- **Açık kalan:** claude-cli köprüsünde `use_skill` ilk çağrıda "No such tool available"
  (MCP aracı ilk turda çözülmüyor) — ayrı, daha derin bir köprü konusu; not edildi.

---

## Modele göre akıllı varsayılan bütçe — Option B ✅ (2026-06-22)

**Hedef:** Flat 12K transcript bütçesi büyük modelin (200K–1M) penceresini boşa
harcıyordu. Pencere metadata'sını (önceki commit) gerçekten kullan.

- **`conversation.EffectiveBudget(provider, model, configured)`**: pencere biliniyorsa
  bütçe = `clamp(window × 0.10, configured, 32K)` — yapılandırılmış değer **taban**
  (asla altına inmez), 32K **tavan** (1M modelde maliyet guard'ı), bilinmeyen → değişmez.
- **`Manager.Prepare`** artık compaction tetiğini + pressure oranını model-aware bütçeyle
  hesaplıyor. `maxTokens≤0` (bütçe kapalı) dokunulmaz — `TestPrepareZeroPressure...`
  semantiği korundu (ilk denemede bu testi kırdım, `if maxTokens>0` guard'ıyla düzelttim).
- **Sonuç:** Opus 4.8/Sonnet 4.6 1M → 32K · Haiku 4.5 200K → 20K · MiniMax/DeepSeek/Gemini 1M → 32K · bilinmeyen → 12K.
- **Doğrulama:** `budget_test.go` + conversation/providers **51 test** yeşil, `go vet` temiz.
- **Follow-up:** §5 tool eşikleri hâlâ process-geneli bütçeyle (dormant); per-model
  `EffectiveBudget`'a bağlamak temiz sonraki adım. Detay: `17-TOKEN-OPTIMIZASYON.md` §7.

---

## Per-model context-window metadata ✅ (2026-06-22)

**Hedef:** Modellerin context-window boyutunu metadata olarak taşı (UI + gelecekteki
tokenLimitFor zemini). Kullanıcı isteği.

- **`ModelInfo.ContextWindow int`** (token, `contextWindow,omitempty`). `Catalog()`
  build-time'da merkezi **`ContextWindowFor(provider, model)`** aile-tablosundan
  doldurur → manifest'ler churn'den uzak kalır, yine her modelde değer görünür.
- **Aile-bazlı, muhafazakâr:** Opus 4.8/Sonnet 4.6 **1M**, Haiku 4.5 **200K**,
  MiniMax/DeepSeek/Gemini 1M (web'le doğrulandı: M3 = 1,048,576; Opus/Sonnet 1M,
  Haiku 200K), gerisi 0 = "bilinmiyor" → fallback. 40+ third-party OpenRouter
  modelini elle yanlış doldurmaktansa emin olunanlar.
  > **Düzeltme (2026-06-22):** İlk sürümde Claude ailesine düz 200K verilmişti; Opus
  > 4.8 ve Sonnet 4.6 aslında **1M**, sadece Haiku 200K. Per-tier eşlemeyle düzeltildi.
- **Test:** `context_window_test.go` (aile eşleme + Catalog dolduruyor mu) — providers
  paketi **43 test** yeşil, `go vet` temiz. `api`'ye dokunulmadı (JSON tag otomatik akar).
- **Phase 2 (tokenLimitFor) bilinçle ertelendi:** model penceresine ölçekleme SwarmGo'nun
  12K transcript bütçesiyle çelişir (bir tool sonucu tüm bütçeyi aşar); doğru hamle
  "modele göre akıllı varsayılan bütçe". Detay: `17-TOKEN-OPTIMIZASYON.md` §6.

---

## CG-9 ikinci yarı — bütçe-orantılı tool eşikleri ✅ (2026-06-22)

**Hedef:** the external agent project'ın `tokenLimitFor` (tool-result eşiği context window'a göre)
deseninin SwarmGo karşılığı. Model context-window metadata'sı yok (`ModelInfo`
sadece ID/Label), o yüzden mevcut **transcript bütçesine** (`MaxContextTokens`)
orantıladım — kullanıcının zaten modeline göre ayarladığı knob.

- **`Tunables.budgetScaleLocked()`** = `budget/12000`, clamp **[1×,5×]**.
  `CompactMaxBytes()` ve `CompactLLMThreshold()` artık base × scale döndürüyor.
  **Aynı faktör** → A-cap(16384) > B-eşik(12288) değişmezi her ölçekte korunur.
- **5× tavan** → B≈60KB, the external agent project'ın ~60KB özet tavanıyla örtüşür.
- **Default bütçe (12000) → 1×** → değerler birebir mevcut → **regresyon yok**.
- **`SetContextBudget`** setter + `applySettings` wiring (`s.convo.SetLimits` yanında).
- **Doğrulama:** `tunables_compact_test.go` + agent paketi **71 test** yeşil, `go vet` temiz.
- **Not:** wiring satırı (`server.go`) paralel oturumun MemGPT Parça-4 `/api/agents/{id}/core`
  route'larıyla aynı dosyada uncommitted → o paket bütünleşince commit'lenecek.
  Wiring olmadan `contextBudgetTokens=0` → 1× → güvenli no-op (dormant).

---

## Ayarlar ekranı kaydetme tutarsızlığı düzeltildi ✅ (2026-06-22)

**Hedef:** Ayarlar ekranında gösterilen ama Kaydet'e basınca diske yazılmayan
("sessizce kaybolan") alanları onar — UI/kaydetme tutarsızlığı denetimi.

- **Kök neden:** `SettingsPanel.tsx` `saveApp()` patch nesnesini elle alan-alan
  kuruyordu; panellerde render edilen 10 kontrol bu listede yoktu. Kullanıcı
  değiştirip Kaydet'e basınca patch alanı içermiyor, backend değişmemiş değeri
  döndürüyor ve `setOriginal(updated)` kontrolü eski haline geri alıyordu (hatasız).
- **Onarılan 10 alan:** `defaultPermissionMode` (Sağlayıcılar) + `reactiveCompact`,
  `maxTokenRetries`, `reactiveKeepRecent`, `compactToolOutput`, `compactMaxLines`,
  `compactMaxBytes`, `compactLlmSummary`, `compactLlmThreshold`, `compactModel`
  (Bağlam — Tur kurtarma + Sistem A/B sıkıştırma bölümlerinin tamamı). Hepsi
  `saveApp()` patch'ine eklendi.
- **İkincil:** `spawnMaxConcurrent`/`spawnMaxPerTurn` backend (Patch+store clamp+
  server canlı uygulama) ve skill dokümanında vardı ama frontend `AppSettings`
  tipinde, UI'da ve patch'te **yoktu**. Tipe eklendi, Tools paneline kontrol
  (1–128 / 1–64) eklendi, patch'e eklendi → uçtan uca bağlandı.
- **Doğrulama:** `tsc --noEmit` temiz. Skill `swarmgo-settings` zaten tüm alanları
  doğru belgeliyordu (değişiklik gerekmedi).

**Diğer düzenleme ekranlarının denetimi (aynı tur):** Workspace (`WorkspaceView`),
Ajan (`AgentSettingsForm`), Hooks (`HooksPanel`), Skill (`SkillEditor`), Görev
(`TaskDetailPanel`) ve Akış (`FlowsPanel`) ekranları tek tek denetlendi — **hepsi
temiz**: her biri render ettiği tüm alanları kendi create/update payload'una
gönderiyor (elle-omit yok). Workspace'te `instructions`/`boardColumns`,
SkillEditor'da `color` bilinçli/dökümante şekilde ayrı yüzeyde. Asıl hata yalnız
App Settings'teydi.

**Ölü alan temizliği:** `db.Agent.Capabilities` (`models.go`) kaldırıldı — hiçbir
yerde okunmuyordu (yalnız `store.go`'da `"[]"` default'lanıp market install'da
yazılıyordu, geri-publish yolu yok). 3 nokta: `models.go` alan, `store.go` default
bloğu, `api/market.go` atama. Ardından pack formatı
`market.AgentPayload.Capabilities` (SwarmPack v1) alanı da kaldırıldı — `omitempty`
olduğu için eski pack JSON'ları sorunsuz parse olur (alan varsa yok sayılır). Eski
agent JSON'larında migrasyon gerekmez. `go build` (db/api/market) temiz, db testleri
20/20, market testi geçti.

---

## CG-9 density-aware estimator + Sistem B varsayılan açık ✅ (2026-06-22)

**Hedef:** the external agent project kıyaslamasında çıkan iki açığı kapat — (1) yoğun içerikte token
undercount ("session poisoning"), (2) büyük araç-sonucu özetinin kutudan-kapalı olması.

- **CG-9 (birinci yarı):** `conversation/tokens.go` `estimateText` density-aware oldu.
  Tek geçişte rune+whitespace sayar; uzun & `<%3` boşluklu (≥256 rune) içerik **~1.5
  chars/token** (`runes*2/3`), düz metin **~4**. `utf8` importu düştü. `tokens_test.go`.
  Transcript bütçesi (12K) ve UI meter artık base64/hex'i doğru sayıyor. **Kalan:**
  tool-result eşiğini context window'a göre ölçekleme (CG-9 ikinci yarı).
- **Sistem B varsayılan açık:** `settings.Default()` + `tunables` sabitleri —
  `CompactLLMSummary: false→true`, `CompactLLMThreshold: 8192→12288`,
  `CompactMaxBytes: 12288→16384`. **Kritik:** A'nın cap'i B eşiğinin üstüne çıkarıldı,
  yoksa A çıktıyı B eşiğinin altına kırpıp B'yi pre-empt ediyordu. Doc 17 güncellendi.
- **Doğrulama:** conversation/settings/agent **89 test** yeşil; `TestEstimateTextDensity`
  ayrıca tek tek geçti. **Not:** `go build ./...` şu an paralel oturumun yarım MemGPT
  Parça-4 işinden (`ReadCore`/`WriteCore` 3-arg, `coreMemoryBlock`) **kırık** — benim
  paketlerim (api'ye bağımsız) izole derlenip test edildi; bozuk dosyalara dokunulmadı.

---

## Araç hizalama + tarih enjeksiyonu + bellek/arama CLI köprüsü ✅ (2026-06-22)

Üç bağımsız iyileştirme (kullanıcı isteği). Build+vet temiz, **289 test** yeşil,
`tsc --noEmit` temiz, backend rebuild+restart (127.0.0.1:8090).

1. **`get_current_time` kaldırıldı → tarih sistem prompt'unda.** Ajan saati artık
   tool round-trip yerine bağlamdan okur. `composeTurnRequest` dinamik bloğuna ve
   `autonomousSystemPrompt`'a tek satır eklendi (`dateTimeContextBlock`,
   `"Current date and time: Monday, 2006-01-02 15:04 (-07:00)"`). `builtin_time.go`
   + testi silindi. Bu, claude-cli'nin zaten yaptığının native eşleniği.

2. **Çekirdek araç isimleri claude-cli ile hizalandı.** `read_file→Read`,
   `write_file→Write`, `edit_file→Edit`, `list_dir→LS`, `glob→Glob`, `grep→Grep`,
   `shell→Bash` (`builtin_fs.go`, `builtin_shell.go`). Model bu isimlere yoğun
   eğitimli → daha güvenilir tool-use. `classify.go`/`permpattern.go` zaten her iki
   isim setini taşıyordu → tek sete indirgendi (`execArgTools={"Bash"}`). Yan
   referanslar güncellendi: subagent profilleri, `artifacts_auto.fileWriteTools`,
   hook şablonları, MarkLazy, bridge dispatch case, frontend (`DiffCard`/`tools.ts`/
   `stepKinds`). `http_get` kasıtlı korundu (CLI WebFetch'ten farklı; düz GET).

3. **`core_memory_replace/append` + `conversation_search` CLI'ye köprülendi.** Bu
   eager built-in'ler native-loop ctx bağımlılığı taşımadığından `BridgeTools` def
   listesine doğrudan eklendi (gate'leri `CoreMemoryTools()`/`SessionContextEnabled()`);
   dispatch zaten `bridgeCallFor()`→`reg.Call` ile çalışır, ekstra case yok. Native'de
   eager kalırlar. Artık CLI ajanı gördüğü core-memory bloğunu **düzenleyebilir** ve
   geçmişte derin arama yapabilir. (Stale `bridgeExcluded` run_subagent yorumu da
   düzeltildi.) Dokümanlar: `11-INTERACTION-MCP`, `26-MEMGPT`, `27-CROSS-SESSION`,
   `19-LAZY`, `09-SDK`, `25-SUBAGENT`, `06-WORKSPACES`.

## CG-16 — Oturumlar-arası tam-metin arama ✅ TAM (çekirdek+araç+API+UI, 2026-06-22)

**Hedef:** Workspace'in tüm oturum mesaj geçmişinde anahtar-kelime araması (bugün
yalnız başlık+summary üzerinden farkındalık vardı). Plan: `_Docs/27-CROSS-SESSION-SEARCH.md`.

**Kilit karar (RG-6 disiplininin ürünü):** Oturumlar boot'ta `d.messages`'a (RAM)
yükleniyor → aranacak veri zaten bellekte. Varsaymak yerine depolama modelini
okuyup **ripgrep/FTS5'i eledim**; saf-Go tarama hem en basit hem yeterli.

Yapılan:

- **`db.SearchMessages`** (`internal/db/store_search.go`): saf-Go RAM-içi tarama,
  boşlukla bölünmüş terimler AND-eşleşir (case-insensitive), `score = matchCount +
  recency` (C5 felsefesi), rune-sınırlı Türkçe-güvenli snippet. `SearchHit`/`SearchOpts`.
- **`conversation_search` aracı (N5)** (`tools/builtin_conversation_search.go`):
  `ListSessionsTool` deseni; `toolsetup.go`'da `SessionContextEnabled()` gate'i.
  `exclude_current` plandan düşürüldü (tools→agent import döngüsü); API'de `exclude`
  query param'ı karşılıyor.
- **API** `GET /api/sessions/search?q=&limit=&role=&exclude=`
  (`api/sessions_search.go` + `server.go` route, `active` yanında).
- **Frontend contract:** `SearchHit` tipi + `sessionApi.searchMessages(...)`.
- **Doğrulama:** `go build ./...` + db/tools/api/agent testleri **200** yeşil;
  `tsc --noEmit` temiz. Yeni testler: `store_search_test.go`,
  `builtin_conversation_search_test.go`.
- **Görsel arama ✅ (aynı gün):** `SessionsSidebar` arama kutusu çift işlevli —
  başlık filtresi + `api.searchMessages` mesaj araması (≥2 char, 250ms debounce,
  `cancelled` guard). "Mesajlarda (N)" bölümü rol-rozeti+snippet+yaş; tıkla →
  `onSelectSession(sessionId, messageId)`. `selectSession` opsiyonel `messageId` →
  `scrollToMsgId` → `MessageList`: her satır `data-msg-id`, hedefe `scrollIntoView`
  (center) + 1.6sn accent-ring flash, sonra `onHighlightConsumed`. `tsc` + `npm run
  build` + `go build` yeşil. **CG-16 tam kapandı.**

---

## RG-6 — Implement-öncesi keşif guard'ı ✅ (2026-06-22)

**Hedef:** Ajanın var olan bir özelliği "yok" sanıp sıfırdan yeniden yazma (veya
çalışan bir uygulamanın üzerine yazma) riskini önle. Faz R oturum-analizindeki en
büyük yanlış kararın mekanizma karşılığı; kod değil, skill/system-prompt düzeyi.

Yapılan:

- **`swarmgo-guide` SKILL.md → "Before you build: discover first" bölümü:** uygulamadan
  ya da "bu yok" demeden önce **search → read → confirm → extend** disiplini;
  absence iddiası ancak gerçekten arandıktan sonra ("Y ve Z için grepledim, bulamadım"),
  ve sıfırdan yazmak yerine mevcudu genişletme kuralı. Kod/konfig/agent/flow/skill/
  memory — hepsine uygulanır.
- **`swarmgo-self-management` → "Prefer reading first" güçlendirildi:** entity
  (agent/flow/skill/schedule/hook/MCP) oluşturmadan önce mevcudu kontrol et,
  duplicate yerine genişlet; guide bölümüne çapraz-referans.
- **Doğrulama:** `go build ./...` ✅ + `go test ./internal/skills/...` (14 test) yeşil.
  Skill testleri yalnız seed varlığı/non-overwrite kontrol ediyor; içerik değişimi
  güvenli.

---

## MemGPT/Letta Tarzı Self-Editing Bellek — Parça 1–3 ✅ (2026-06-22)

**Hedef:** Letta'yı (Docker+Postgres+Python) koşmadan, fikirlerini native Go'da:
bağlam-basıncı sinyali + ajanın in-place düzenlediği kalıcı **çekirdek bellek**.
Tek binary / offline / dosya-tabanlı kimliği korunur, migration yok. Plan +
sapmalar: `_Docs/31-MEMGPT-CORE-MEMORY.md`.

Yapılan (3 parça):

- **(1) Bağlam-basıncı sinyali:** `conversation.Prepared.Pressure`
  (`ContextTokens/maxTokens`, `maxTokens<=0 → 0`); `composeTurnRequest` eşik
  (`memoryPressureWarn`, vars. 0.75) aşılınca `SystemDynamic` başına "önemliyi
  şimdi yaz" uyarısı koyar — sessiz compaction'dan *önce*. Sıfır yeni model çağrısı.
- **(2) `core` bellek türü + Store API:** `db.MemoryCore` + `db.UpsertKnowledgeByKind`
  (ajan başına tek satır, ID korunur); `memory.Store.WriteCore/ReadCore/AppendCore`.
  Recall artık `recallKinds` allowlist'iyle core'u hariç tutar (her turda zaten
  sabit enjekte edildiğinden tekrar çıkmaz).
- **(3) `core_memory_*` araçları:** `tools.CoreMemoryTool` (replace/append) — eager,
  `coreMemoryTools` ayarıyla (vars. açık) gated; core bloğu her turda recall'ın
  üstünde `SystemDynamic`'e enjekte edilir. Letta'nın "core memory always in
  context" davranışı.
- **Ayar/UI:** `memoryPressureWarn` + `coreMemoryTools` → Settings/DTO/Patch/
  Tunables + `applySettings` canlı push; frontend ContextPanel "Çekirdek bellek
  (MemGPT)" bölümü.
- **Doğrulama:** `go build ./...` + ilgili paket testleri (conversation/memory/
  tools/api/agent/db/settings) yeşil; frontend `tsc --noEmit` temiz.
- **UI/API genişletmesi (ikinci tur):** Ayarlarda basınç eşiği **sürgü** + canlı
  yüzde + tetikleme-token'ı + "kapalı" durumu + canlı durum kartı (yeni `Slider`
  primitifi). Hafıza panelinde **çekirdek bellek kartı** (`CoreMemoryCard` —
  göster/düzenle) + `GET|PUT /api/agents/{id}/core` uç noktaları.
- **Parça 4a — persona/human ayrımı ✅ (2026-06-22):** çekirdek bellek iki bağımsız
  bölüme ayrıldı — persona + human, her biri ajan başına tek satır.
  > ⚠️ **2026-06-23'te Parça 5 ile geçersiz kılındı:** `core_persona`/`core_human`,
  > `section` parametresi, `ReadCoreSections` ve `{persona,human}` API'si artık
  > yok → `core:<label>`, `label`, `ReadCoreBlocks`, `{blocks}`. Bu girişin üstündeki
  > **"adlandırılmış bloklar"** ve **"HA-1"** girişlerine bak.
- **Devamı:** Parça 5 (adlandırılmış bloklar + limit) ve Parça 4b (HA-1
  oto-modelleme) **2026-06-23'te tamamlandı** — dosyanın başındaki güncel girişler.

## Oturum-başına Çalışma Dizini (cwd) + otonomi frenleri ✅ (2026-06-22)

**Hedef:** the external agent project'taki "working directory" mekaniği — her oturumun, ajanın
dosya/kabuk araçlarının çalışacağı bir cwd'si olsun; UI'dan değiştirilebilsin;
bağlama enjekte edilsin; ajan git ile repo editleyebilsin. Kilitsiz fs/shell'in
otonom yolda güvenli kalması için frenler.

Yapılan (4 adım uçtan uca):

- **(1) cwd çözümleme:** `Session.WorkingDir` (db) + `SetSessionWorkingDir`;
  `agent/workdir_ctx.go` (`effectiveWorkDir` + ctx taşıyıcı); `completeTraced`
  her turda çözüp `req.WorkDir` + ctx'e koyar; `buildRegistry` sandbox kökünü
  ctx'ten alır. Chat yollarına `WithSessionID` eklendi.
- **(2) Bağlam enjeksiyonu:** `api/workdir_context.go` — "Working directory"
  bloğu (cwd + git branch + CLAUDE.md ipucu) dinamik bağlama (chat_turn.go).
- **(3) Otonom fren:** `autonomousConfine` ayarı (varsayılan açık) — otonom
  turlar `NewConfinedSandbox` ile çalışma dizinine kilitlenir; `shell` confined
  modda `git push`'u reddeder (`isNetworkMutatingGit`).
- **(4) Worktree izolasyonu:** `gitWorktreeIsolation` ayarı (varsayılan kapalı) —
  `agent/worktree.go` otonom oturuma `<workspace>/worktrees/<sid>` worktree+dal
  verir; oturum silinince `RemoveSessionWorktree` temizler.
- **API:** `GET/PUT /api/sessions/{id}/workdir`, `GET /api/fs/browse` (`api/workdir.go`).
- **Frontend:** `chat/WorkDirBadge.tsx` (klasör rozeti + dizin gezgini); Ayarlar ▸
  iki yeni toggle; types/api alanları.
- **Doğrulama:** `go build ./...` + `go test ./...` + frontend `tsc -b` + `npm run
  build` yeşil. Detay: `_Docs/26-CALISMA-DIZINI.md`.

### Workspace varsayılan çalışma dizini + canlı doğrulama (2026-06-22)
- `WSSettings.DefaultWorkingDir` (ws-settings.json) + Runtime `SetDefaultWorkDir`/
  `WorkspaceDefaultDir`; `effectiveWorkDir` artık oturum → workspace-default →
  fiziksel workDir sırasıyla çözüyor. Ayarlar ▸ Bu Workspace ▸ "Varsayılan çalışma
  dizini" alanı (`WorkspacePanel.tsx`, `workspace_settings.go` DTO/patch).
- **Canlı duman testi (8088):** `GET /api/fs/browse` ✓; bir oturuma SwarmGo deposu
  set edildi → `{exists:true,isGitRepo:true,branch:"main"}` ✓ (git branch tespiti),
  sonra sıfırlandı. **Not:** varsayılan 8080 portu mcp-for-unity backend'iyle
  çakıştığı için bu örnek `SWARMGO_ADDR=127.0.0.1:8088` ile çalışıyor.

## fs/shell sandbox kilidi kaldırıldı (kilitsiz dosya/komut erişimi) ✅ (2026-06-22)

**Hedef:** Built-in dosya/komut araçlarının workspace dizinine kilitli olma
güvenlik kısıtını kaldırmak — the external agent project (external-agent-oss) gibi araçların makinedeki
herhangi bir yola erişebilmesi, güvenliği tek başına **izin moduna** bırakmak.

Yapılan:


---

> **Daha eski kayitlar (2026-06-19 ve oncesi) arsivlendi:** [05-ARSIV.md](05-ARSIV.md).
