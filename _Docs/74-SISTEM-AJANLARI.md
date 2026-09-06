# 74 — Sistem Ajanları

> **Durum: UYGULANDI (2026-08-26; kilitli yerleşik + kalıtım modeli 2026-09-03).**
> Başlıklandırma, sıkıştırma, ders çıkarma ve içgörü analizi gibi uygulama içi LLM işleri
> sistem ajanlarıyla yürütülür. Her rolün **kilitli yerleşik** satırı (koddan dayatılır,
> düzenlenemez) uygulamanın varsayılanıdır; özelleştirme, ondan kalıtım alan ve yalnız
> istenen alanları override eden bir çocukla yapılır (`82-AJAN-KALITIMI.md`).

## Kavram ve kayıt defteri

Sistem ajanı, normal ajan profilindeki `System: true` ve kararlı `SystemKey` ile
işaretlenen özel ajandır. Ayrı bir oturum türü veya `Session` alanı yoktur; bir
oturumun sistem ajanına ait olup olmadığı ajan kaydından çözülür
(`internal/agent/systemsession.go:10-21`).

Derlenmiş kayıt defteri altı altyapı rolü ve altı yerleşik worker profili tanımlar:

| `SystemKey` | Varsayılan durum | Prompt anahtarı | Görev |
|---|---|---|---|
| `titler` | Etkin | `title` | İstek ve konuşmalar için kısa başlık üretir (`internal/agent/systemagents.go:13-18`, `internal/agent/titler.go:83-95`). |
| `overview-summarizer` | Etkin | `summary` | İstek üzerine board ve flow genel bakışlarını özetler (`internal/agent/systemagents.go`, `internal/agent/summarizer.go`). |
| `compaction` | Etkin | `compact` | Bağlam sınırında konuşma geçmişini yapılandırılmış özete sıkıştırır (`internal/agent/systemagents.go`, `internal/agent/wsconfig.go`). |
| `lesson-extractor` | Etkin | `lesson` | Başarısız ajan turlarından yeniden kullanılabilir dersler çıkarır (`internal/agent/systemagents.go:29-34`, `internal/agent/lessons_systemagent.go:5-7`). |
| `insight` | **Devre dışı** | `insight-analyzer` | Oturum kanıtlarında tekrarlanan, eyleme dönük bulguları analiz eder (`internal/agent/systemagents.go:37-43`, `internal/agent/lessons_systemagent.go:9-10`). |
| `insight-applier` | Etkin | `insight-applier` | İçgörü taramasının `workspace-opt` bulgularını workspace varlıklarına uygular (`internal/agent/systemagents.go`, `internal/prompts/defaults/insight-applier.md`). |
| `stall-judge` | Etkin | `stall-judge` | Koordinatörün son mesajının gerçekte yapılmamış bir worker spawn'ını anlatıp anlatmadığını sınıflandırır; araçsız, tek satır JSON (`internal/agent/coordination_stall.go`, 2026-09-03). |
| `goal-writer` | Etkin | `goal-writer` | Evrim hedefleri (`_Docs/83`): kullanıcının kendi sözleriyle yazdığı hedefi kapalı metrik kataloğuna, guardrail'lere, kapsama ve propose-only politikaya oturtan taslak yazıcı; araçsız, STRICT JSON; doğrulama ve kayıt kodda (`internal/agent/goal_writer.go`, `internal/goals`, 2026-09-05). |
| `workspace-evolver` | Etkin | `workspace-evolver` | Evrim E2 (`_Docs/83`): bir hedefin snapshot başına fitness'inden, yalnız izin verilen yüzeylerde (`goals.ProposalRules`) ölçülü değişiklik önerileri üretir; araçsız, STRICT JSON; kanıt/kapsam/bütçe/çatışma kuralları kodda, `evolution` kanalına bulgu olarak düşer, hiçbir şey uygulamaz (`internal/agent/goal_evolver.go`, 2026-09-05). |

### Yardımcı çağrılar nerede koşar — `auxNativeRouting` (2026-09-03)

Çözümleyiciler (`resolveTitleConfig`, `resolveCompactorConfig`,
`resolveAnalysisSystemAgent`, `resolveFoldAgent`) sistem ajanından yalnız prompt +
model alır; kimlik ve **kimlik bilgileri çağıran ajandan** gelir. Çağıran ajan
claude-cli/codex-cli'daysa bu, her başlık/özet/yargı için taze bir `claude -p` =
Claude Code'un ~36k token'lık taban promptu (2.1.259 ölçümü, `_Docs/17`) demekti.
`Runtime.routeAuxAgent` (`internal/agent/systemagent_route.go`) artık üç koşul
sağlanınca kopyayı anahtarlı ilk `anthropic` instance'ına çevirir: ayar
`auxNativeRouting` açık (varsayılan), çağıran CLI türünde, `Registry.
FirstAvailableOfKind("anthropic")` boş değil. Model, sistem ajanının alias'ından
API id'sine çevrilir (`providers.NativeClaudeModel`: haiku →
`claude-haiku-4-5-20251001`). Koşullar sağlanmazsa davranış eskisi gibidir;
native anthropic bir çağıranda yalnız alias çevrilir. Faturalama çağıran ajanın
ID'sinde kalır, usage satırının provider sütunu değişir — bu çağrılar CLI
aboneliğinden değil **API anahtarından** ödenir.

**Compaction katlaması** da bu yoldan geçer: önceden `Manager.Prepare`/`ForceCompact`
oturum ajanının sağlayıcı+modeliyle (ör. claude-cli opus, tam taban) doğrudan
`provider.Complete` çağırıyordu; `compaction` sistem ajanının modeli yalnız
belgede geçerliydi. `Runtime.FoldContext(ctx, agent)` (`internal/agent/
fold_target.go`) beş katlama giriş noktasında (`chat_turn_phases`, `chat_btw`,
`summary`, `wake_turn`, `toolloop_phases.compactAndRetry`) + handoff'ta
`conversation.WithFoldTarget` damgalar: aynı sağlayıcıda yalnız model değişir
(provider nesnesi elde kalır), yönlendirmede sağlayıcı da değişir. Katlama
istekleri araçsız olduğundan `CLIRestrictNativeTools` taşır. Testler:
`systemagent_route_test.go`, `conversation/foldtarget_test.go`.

### `insight-applier` — dar allowlist bir güvenlik sözleşmesidir

Ajan yalnız `channel:workspace-opt` + `status:new` bulgularını uygular, `app-fix` kanalına
dokunmaz ve uyguladığı bulguyu `insight_apply_finding` ile `applied` işaretler. `AllowedTools`
yedi giriş taşır: `group:automation`, `group:agents`, `group:skills-mcp`, `group:artifacts`,
`insight_list_findings`, `insight_apply_finding`, `todo_write`.

**`group:files` ve `group:config` bilinçli olarak YOKTUR** → Read/Write/Edit/Bash ve
ayar/secret/workspace araçlarına erişemez. Böylece düzeltmeyi yalnız workspace store
varlıklarında (skill, ajan, hook, otomasyon) yapabilir; repo dosyalarına ve uygulama
ayarlarına ulaşamaz. Kısıtı prompt değil, **allowlist** uygular.

Ajanın kendisi etkin gelir; onu çağıran shipped otomasyon (`insight-apply-workspace-opt`,
etiket `insight-scan`) ise **varsayılan kapalıdır** — zincir kullanıcı Otomasyon ekranından
kuralı açana kadar çalışmaz. Zincirin tamamı: `_Docs/60-RETROSPEKTIF-TARAMA.md` §9.2,
otomasyon alanları: `_Docs/46-ETIKET-OTOMASYON.md`.

### Yerleşik worker profilleri (`subagent-*`)

**Sağlayıcı açıkça tohumlanır (2026-08-29).** Yukarıdaki tablodaki altı yerleşik
tanım `Provider: "claude-cli"` taşır (`internal/agent/systemagents.go`) ve
`EnsureSystemAgents` boot'ta sağlayıcısı **boş** olan mevcut sistem ajanı
kayıtlarına bu değeri bir kez yazar (`internal/db/store_agent_system.go`,
idempotent: dolu bir sağlayıcı asla ezilmez). Böylece UI ajanın sağlayıcısını
"belirtilmemiş" diye göstermez. Model alanı boş kalmaya devam eder — o, sağlayıcının
oturum modelidir (bkz. `_Docs/05-ILERLEME.md`, "Model etiketlerinden 'Varsayılan'
kalktı").

`run_subagent` ve `spawn_worker` hedefi olan altı yerleşik profil de sistem ajanı
olarak tutulur: `subagent-explore`, `subagent-planner`, `subagent-coder`,
`subagent-reviewer`, `subagent-validator`, `subagent-config`. Tanımları
`buildSystemAgentDefaults` üretir; prompt `prompts.Default("subagent-<id>")`,
araç listesi ise `defaultSubagentProfiles[<id>].AllowedTools` alanından gelir
(`internal/agent/systemagents.go`, `internal/agent/subagent.go`).

Üç kural bu profilleri diğer sistem ajanlarından ayırır:

- **Araç listesi bir güvenlik sözleşmesidir, kullanıcı tercihi değil.** Ajan
  kaydındaki `AllowedTools`, koddaki profilin önbelleğidir: `resolveWorkerTarget`
  farklıysa kaydı geri yazar, `SpawnSession` ve `SendToWorker` ise o turun ajan
  kopyasına listeyi yeniden uygular (`applyProfileAllowlist`,
  `internal/agent/subagent_allowlist.go`). Böylece elle düzenlenmiş bir kayıt
  worker'ın erişimini genişletemez. UI'da bu ajanlarda araç bölümü salt-okunurdur
  (`frontend/src/features/agents/AgentToolsSection.tsx`, `locked` prop'u).
- **Model ve sağlayıcı dinamiktir.** Tanımda `SuggestedModel` boştur; worker,
  spawn anında koordinatörün provider/instance/model/permission değerlerini
  klonlar (`SpawnSession`, `RuntimeBaseAgentID`).
- **Eski `worker:<profil>` ajanları göç ettirilir.** `EnsureSystemAgents`, aynı
  adlı kalıcı ajanı bulursa onu yerinde sistem ajanına dönüştürür (id korunur,
  oturumları çözülmeye devam eder); sistem ajanı zaten varsa eski kaydı devre dışı
  bırakır, böylece çift hedef kalmaz (`internal/db/store_agent_system.go`).

Kayıt defteri promptları `prompts.Default(<anahtar>)` ile alır. Rol çözümlemesi
başarısız olduğunda kullanılan `readPrompt(<anahtar>)` aynı anahtara gider; böylece
özelleştirme yokken yerleşik ve eski gömülü davranış aynı promptu kullanır. Örneğin
`overview-summarizer` için iki yol da `summary` anahtarındadır. `compaction` ajanının
özelleştirilmiş promptu `{{summary}}` ve `{{messages}}` yer tutucularını korumazsa
çalışma zamanı doğrulaması promptu güvenli `compact` varsayılanına düşürür.

## Görsel kimlik (avatar + renk)

> **2026-09-03:** Aşağıdaki "yalnız seed edilir, dayatılmaz" kuralı kilitli yerleşik
> satır için kalktı — yerleşik her açılışta kanonik avatar/rengi geri alır. Kullanıcının
> seçtiği avatar/renk artık rolü devralan **özelleştirme çocuğunda** `avatar`/`color`
> override'ı olarak yaşar ve UI'da ebeveynin rengi soldaki kalıtım şeridinde görünür.

Her yerleşik ajanın avatarı (tek emoji) ve rengi (hex) kanoniktir: aynı sistem ajanı
her workspace'te aynı görünür. Tek kaynak `systemAgentVisuals` tablosudur
(`internal/agent/systemagents.go`); tablo `SystemKey` ile anahtarlanır ve
`applySystemAgentVisuals` her tanıma damgalar. Tabloda karşılığı olmayan bir
`SystemKey` panik üretir — görsel kimliksiz yerleşik ajan, bu tablonun ortadan
kaldırdığı "rastgele varsayılan görünüm" durumudur.

Renkler role göre ailelere ayrılır: analiz ajanları mor/kurşuni, salt-okunur worker
profilleri turkuaz/mavi, yazma yetkisi olan worker profilleri kehribar/kiremit.

| SystemKey | Avatar | Renk | Aile |
|-----------|--------|------|------|
| `titler` | 🔖 | `#7C6BE8` | analiz |
| `overview-summarizer` | 📋 | `#9C6BD8` | analiz |
| `compaction` | 📦 | `#6B5FA8` | analiz |
| `lesson-extractor` | 🎓 | `#B07AD0` | analiz |
| `insight` | 🔮 | `#5C6480` | analiz |
| `insight-applier` | 🛠️ | `#8A6BC8` | analiz |
| `subagent-explore` | 🔍 | `#17A2A2` | worker, salt-okunur |
| `subagent-planner` | 🧭 | `#2E86D8` | worker, salt-okunur |
| `subagent-reviewer` | 🧐 | `#4FBF8B` | worker, salt-okunur |
| `subagent-validator` | 🧪 | `#3FB8D8` | worker, salt-okunur |
| `subagent-coder` | 💻 | `#E08A2E` | worker, yazan |
| `subagent-config` | 🔧 | `#D2683C` | worker, yazan |

`Name`/`Soul`/`Model`'den farklı olarak avatar ve renk **yalnız seed edilir, her
boot'ta yeniden dayatılmaz**: `EnsureSystemAgents` mevcut kayıtta alan BOŞSA kanonik
değeri yazar (görsel kimlik eklenmeden önce oluşmuş workspace'ler böylece düzelir),
kullanıcının seçtiği bir değer varsa dokunmaz — aksi hâlde alan kalıcı olarak
düzenlenemez olurdu. **Varsayılana dön** (`POST /api/agents/{id}/restore-default`)
avatar ve rengi de tanımdan geri yükler.

## Çözümleme ve koşulsuz fallback

> **2026-09-03 — kilitli yerleşikler + kalıtım.** Ayrıntı `82-AJAN-KALITIMI.md`; burada
> yalnız bu dokümanın eski anlatımından değişen noktalar özetlenir.

Her sistem rolü artık workspace'te **iki tür satırla** temsil edilir:

- **Kilitli yerleşik** (`Locked: true`): derlenmiş tanımın birebir kopyası; her açılışta
  `EnsureSystemAgents` kanonik değerleri yeniden dayatır. Düzenlenemez, silinemez, devre dışı
  bırakılamaz. Uygulamanın **varsayılanı** budur.
- **Özelleştirme** (`System: true, Locked: false, ParentID = yerleşik`): "Özelleştir" ile
  türetilen çocuk; override etmediği her alanı yerleşikten devralır.

`FindAgentBySystemKey` sırası: etkin özelleştirme → kilitli yerleşik → (göç öncesi
depolar) devre dışı özelleştirme. `ResolveSystemAgent` değişmedi: bulduğu satır devre dışı
değilse onu kullanır; artık pratikte daima ID'li bir satır bulur. Derlenmiş tanımdan ID'siz
ajan kurma yolu yalnız tohumlanmamış depolar içindir. Bilinmeyen `SystemKey` yine
`unknown system agent key` hatasıdır.

Çağrı yerleri (`titler`, `overview-summarizer`, `compaction`, `lesson-extractor`, `insight`,
worker profilleri) değişmedi; hepsi etkin ajanı (kalıtım katlanmış) alır.

## Devre dışı bırakma, silme ve varsayılana döndürme

- **Yerleşik satır silinemez** (`ErrSystemAgentDelete` → HTTP 409, mesaj "built-in agent
  cannot be deleted; derive a copy to customise it") ve düzenlenemez (`ErrAgentLocked` →
  409). Frontend kilitli ajanda Kaydet/Sil/Devre dışı sunmaz.
- **Özelleştirme silinebilir**: rol yerleşiğe döner; yumuşak silme oturumları korur.
- **Devre dışı** yalnız özelleştirmede anlamlıdır: kapatınca rol yerleşiğe düşer
  ("yerleşik tanım etkin" rozeti). Yeniden etkinleştirme, aynı rolü etkin olarak sağlayan
  başka bir özelleştirme varsa `ErrSystemRoleTaken` (409) ile reddedilir.
- `POST /api/agents/{id}/restore-default` artık **"tüm override'ları kaldır"** demektir
  (`ClearAgentOverrides`): çocuk her alanı yeniden devralır. Kilitli → 409, kök ajan → 404.
  Eski "kayıt defterinden alanları geri yaz" davranışı gereksizleşti; yerleşik satır zaten
  hep kayıt defteridir.

## API yüzeyi

- `DELETE /api/agents/{id}` kilitli yerleşik için 409; özelleştirme için normal silme.
- `PUT /api/agents/{id}` kilitli için 409; `system`/`systemKey` değişimi yine 400.
  Yeni alanlar: `parentId`, `resetFields`.
- `POST /api/agents/{id}/derive` `{bindRole: true}` ile rolü devralan özelleştirme,
  `bindRole` olmadan sıradan türev oluşturur (201).
- `POST /api/agents/{id}/restore-default` override temizler (yukarı bakın).

Rotalar `internal/api/server.go` (`registerAgentRoutes`); hata eşlemesi
`internal/api/agent_inherit.go` (`writeAgentWriteError`).

## UI davranışı

Roster'da sistem ajanları **iki ayrı bölüme** ayrılır: "Sistem ajanları" (uygulamanın
kendi işleri — titler, compaction, insight …) ve "Sistem worker'ları"
(`subagent-` önekli profiller: explore, planner, coder, reviewer, validator, config).
Ayrım `systemKey`'in `subagent-` önekine bakar; bir özelleştirme bağlı olduğu yerleşikle
aynı `systemKey`'i taşıdığı için otomatik olarak aynı bölüme düşer
(`frontend/src/features/agents/agentRoster.ts`).

Her bölüm kendi içinde **gruplanır**: her kilitli yerleşik, hemen altında rolü devralan
özelleştirme(ler) ve soldaki kalıtım şeridiyle. Rozetler: 🔒 *yerleşik*, *rolü sağlıyor*
(etkin özelleştirme), *yerleşik tanım etkin* (devre dışı özelleştirme)
(`frontend/src/features/agents/SystemAgentStatusBadge.tsx`). Kilitli ajanın formu
salt-okunurdur ve **Özelleştir** / **Özelleştirmeyi aç** sunar; özelleştirmede alan alan
"devralındı / override" rozeti ve "devral" sıfırlaması vardır
(`AgentSettingsForm.tsx`, `FieldOverrideBadge.tsx`, `AgentLineageChips.tsx`).

### Ayarlar ▸ Sistem Ajanları ekranı (2026-09-04)

Aynı düzenleme, **Ayarlar** ekranında da bağımsız bir kategori olarak durur:
`Ayarlar ▸ Sistem Ajanları` (`frontend/src/features/settings/SystemAgentsPanel.tsx`).
Ajanlar ekranına gitmeden uygulamanın kendi davranışını (hangi model başlık üretir,
hangi worker profili ne yapar) buradan ayarlamak içindir.

Panel, Ajanlar ekranının iki bölmeli şeklini yeniden üretir — solda roster, sağda
seçili ajanın formu — ama **yalnız sistem ajanlarıyla** sınırlıdır: `groupSystemAgents`
ile aynı "Servisler" / "Worker'lar" ayrımını, `AgentSettingsForm` ile aynı düzenleme
formunu (Özelleştir, devre dışı bırak, override sıfırlama, özelleştirmeyi silme)
kullanır. Ortak parçaları paylaştığı için iki ekran arasında davranış farkı yoktur;
sıradan ajanlar bu panelde hiç görünmez.

Kaydetme app-settings taslağından bağımsızdır: her düzenleme kendi `/api/agents`
çağrısıyla anında yazılır, bu yüzden kategori `SettingsPanel.tsx` içindeki
`SELF_MANAGED_CATS` kümesindedir — başlıktaki "Kaydedilmemiş değişiklik" yazısı ve
**Kaydet** düğmesi bu ekranda gösterilmez (`secrets` ve `exttools` ile aynı davranış).

### Özelleştirme workspace'e özgüdür (2026-09-04)

Kilitli yerleşik tanımlar **koddan** gelir ve her workspace'te aynıdır; ondan kalıtım
alan özelleştirme ise **aktif workspace'in ajan deposuna** yazılır
(`<workspace>/store/agents`, `wsp.DB.DeriveAgent` — `internal/api/agent_inherit.go`).
Yani bir rolü özelleştirmek yalnız o workspace'i etkiler; diğer workspace'ler rolü
yerleşik tanımdan çözmeye devam eder. Bu, kalıtım modelinin doğal sonucudur — ayrı bir
kapsam alanı yoktur, kapsamı **hangi workspace'in deposuna yazıldığı** belirler.

Kapsam üç yerde görünür kılınır:

- **Varsayılan ad workspace'i taşır.** `bindRole` ile türetilen çocuğun adı artık
  `<Yerleşik adı> (<workspace adı>)` olur — ör. `Titler (TionHarnessRepo)`. Eski
  `(özel)` eki app genelinde bir değişiklik izlenimi veriyordu; workspace adı yoksa
  (isim zorunlu olmadan önce yazılmış kayıt) eski ek geri düşer
  (`internal/api/agent_derive_name.go`).
- **Panel başlığındaki şerit.** `Ayarlar ▸ Sistem Ajanları` ekranının üstünde, aktif
  workspace'in adıyla birlikte kapsamı söyleyen bir bilgi şeridi durur
  (`system-agents-scope-note`; ad `GET /api/workspace-settings`'ten okunur). Ayarlar
  ekranı app genelinde olduğu için bu şerit ekranın en gerekli parçasıdır.
- **Kilitli ajan notu.** Yerleşik ajanın salt-okunur formundaki açıklama, kopyanın
  yalnız bu workspace'e özgü olduğunu ve diğer workspace'lerin yerleşik tanımı
  kullanmaya devam ettiğini söyler (`agent-locked-note`).

## Sistem ajanı varsayılan ajan olamaz

Workspace'in `defaultAgentId` ayarı (yeni sohbetlerde önseçili ajan) bir sistem
ajanını gösteremez — kilitli yerleşik de, rolü devralan özelleştirme de (`System: true`).
Sistem ajanı çalışma zamanına hizmet eder ve sohbet muhatabı değildir. Yerleşikten
`bindRole` olmadan türetilen sıradan ajan bu kısıta girmez.

- **Backend kapısı:** `Manager.UpdateSettings` yamayı uygulamadan önce hedefi
  çözer ve sistem ajanıysa `ErrDefaultAgentSystem` döndürür
  (`internal/workspace/settings.go`). HTTP tarafında bu hata 400'e eşlenir
  (`internal/api/workspace_settings.go`).
- **Boot onarımı:** workspace açılışında `sanitizeDefaultAgent` kapı eklenmeden önce
  yazılmış bir değeri temizler ve `Warn` loglar (`internal/workspace/manager.go`).
- **UI:** sistem ajanının ayar formunda **Varsayılan yap** düğmesi hiç render edilmez.

## Workspace seed

Workspace manager'ın ortak `open()` yolu derlenmiş tanımları `EnsureSystemAgents`
ile seed eder (`internal/workspace/manager.go`); yeni workspace'te ve boot'ta açılan
kayıtlı workspace'lerde çalışır, idempotenttir. 2026-09-03 itibarıyla seed **kilitli**
satırı garanti eder ve kanonik değerleri her açılışta yeniden yazar (avatar/renk dahil —
"yalnız seed et, dayatma" kuralı yerleşik satır için kalktı; kullanıcı tercihi artık
özelleştirme çocuğunda yaşar). Eski tek satırlı depoların göçü (en eski satır **yerinde**
kilitlenir, eski düzenlemeleri atılır; `compactor` anahtar göçü ve `worker:<profil>`
benimseme önce çalışır) ve ilk göçün bıraktığı gereksiz çocukları katlayan açılış onarımı
`82-AJAN-KALITIMI.md` §3'te anlatılır (`internal/db/store_agent_system.go`).

## Özyineleme koruması

Sistem ajanlarının ürettiği oturumlar yeni ders veya içgörü analizine kaynak yapılmaz.
Ders çıkarma, oturumun sistem ajanı kullandığını çözdüğünde hemen döner
(`internal/agent/lessons.go:116-136`). İçgörü çalışma yolu oturum filtresini
`internal/agent/insightscan.go:135-144` aralığında enjekte eder; tarayıcı bu filtreyi
`internal/insight/scanner.go:174-181` aralığında çağırıp eşleşen oturumu analizden atlar.
Bu koruma, analiz işinin kendi
çıktısını yeniden analiz ederek zinciri büyütmesini ve yapay geri besleme üretmesini
önler. Doğruluk kaynağı oturumdaki yeni bir bayrak değil, ilişkili `Agent.System`
değeridir (`internal/agent/systemsession.go:10-21`).

## Usage taksonomisi

Sistem ajanı çağrıları usage sınırında
`system:<SystemKey>:<call-kind>` bileşik kind'ına çevrilir. Örneğin titler'ın başlık
çağrısı `system:titler:title`, genel bakış özeti
`system:overview-summarizer:summary` olarak kaydedilir (`internal/agent/callkind.go:114-126`,
`internal/db/store_usage.go:46-60`). Tek bileşik değer hem aktörü hem yapılan işi
korur; ayrı kind kayıtları üretmediği için `ByKind` toplamlarını çift saymaz. Boş veya
`:` içeren bir `SystemKey` hata verir; registry'de olmayan anahtar provider çağrısından
önce reddedilir (`internal/agent/budget.go:117-120`).

## İlgili dosya haritası

- Model ve kalıcılık: `internal/db/models.go`, `internal/db/store_agent_system.go`,
  `internal/db/store.go`, `internal/db/store_usage.go`
- Registry ve çözümleme: `internal/agent/systemagents.go`,
  `internal/agent/systemagent_resolve.go`, `internal/agent/lessons_systemagent.go`
- Çağrı yerleri: `internal/agent/titler.go`, `internal/agent/summarizer.go`,
  `internal/agent/lessons.go`, `internal/agent/insightscan.go`,
  `internal/agent/insightanalyzer.go`
- Özyineleme koruması: `internal/agent/systemsession.go`,
  `internal/insight/scanner.go`
- Usage: `internal/agent/callkind.go`, `internal/agent/budget.go`,
  `internal/db/store_usage.go`
- Workspace yaşam döngüsü: `internal/workspace/manager.go`
- HTTP API: `internal/api/agents.go`, `internal/api/server.go`
- Frontend: `frontend/src/features/agents/AgentsView.tsx`,
  `frontend/src/features/agents/AgentSettingsForm.tsx`,
  `frontend/src/features/agents/SystemAgentStatusBadge.tsx`,
  `frontend/src/api/agents.ts`, `frontend/src/types/agent.ts`
