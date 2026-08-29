# 74 — Sistem Ajanları

> **Durum: UYGULANDI (2026-08-26).** Başlıklandırma, sıkıştırma, ders çıkarma ve
> içgörü analizi gibi uygulama içi LLM işleri, workspace içinde özelleştirilebilen
> fakat güvenli yerleşik tanımlara düşebilen sistem ajanlarıyla yürütülür.

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

`ResolveSystemAgent`, yalnız workspace'te bulunan **ve etkin** sistem ajanını
kullanır. Kayıt yoksa da kayıt bulunup `Disabled` ise de koşulsuz biçimde derlenmiş
tanıma düşer; fallback ad, prompt, açıklama, model, araç izinleri ve `SystemKey`
alanlarını kayıt defterinden kurar (`internal/agent/systemagent_resolve.go:9-30`).
Bu nedenle sistem ajanını devre dışı bırakmak ilgili altyapı işini kapatmaz:
özelleştirilmiş workspace profilini devreden çıkarır ve yerleşik davranışı etkinleştirir.
Frontend bunu devre dışı sistem ajanında **“yerleşik tanım etkin”** rozetiyle açıklar
(`frontend/src/features/agents/SystemAgentStatusBadge.tsx:7-17`).

Bilinmeyen bir `SystemKey` sessizce kabul edilmez; kayıt defterinde karşılığı yoksa
`unknown system agent key` hatası döner (`internal/agent/systemagent_resolve.go:16-19`).

Çağrı yerleri (`titler`, `overview-summarizer`, `compaction`, `lesson-extractor`, `insight`) önce bu
çözümleyiciyi kullanır. Çözümleyici hata döndürürse her çağrı yeri kendi eski gömülü
prompt/model davranışına geri döner; registry ve gömülü yollar aynı merkezi prompt
anahtarlarını kullandığı için varsayılan prompt içeriği değişmez
(`internal/agent/titler.go:83-95`, `internal/agent/summarizer.go:65-80`,
`internal/agent/lessons_systemagent.go:13-29`).

## Devre dışı bırakma, silme ve varsayılana döndürme

Sistem ajanı **silinemez**. Store `ErrSystemAgentDelete` döndürür
(`internal/db/store_agent_system.go:8-10`, `internal/db/store.go:111-120`); API bunu
HTTP 409 Conflict ve “disable it instead” mesajına çevirir
(`internal/api/agents.go:281-293`). Frontend sistem ajanlarını normal ajanlardan ayrı
bir **Sistem ajanları** bölümünde gösterir ve silme eylemini sağlamaz
(`frontend/src/features/agents/AgentsView.tsx:145-149`,
`frontend/src/features/agents/AgentsView.tsx:378-386`,
`frontend/src/features/agents/AgentsView.tsx:456-471`). Dolayısıyla `disable`, silmenin
eşanlamlısı değildir: kalıcı kimlik korunur ve çalışma yerleşik fallback ile sürer.

Özelleştirilmiş profili derlenmiş ayarlara döndürmek için
`POST /api/agents/{id}/restore-default` kullanılır
(`internal/api/server.go:413`). İşlem kayıt defterindeki ad, sistem promptu,
açıklama, model ve izinli araçları geri yazar; `Disabled` alanını bilerek değiştirmez
(`internal/api/agents.go:306-334`). Hedef sistem ajanı değilse veya geçerli bir
`SystemKey` varsayılanı yoksa HTTP 404 döner (`internal/api/agents.go:311-318`).
Etkinleştirme/devre dışı bırakma bu nedenle restore işleminden ayrı, açık bir eylemdir
(`internal/api/agents.go:306-308`).

## API yüzeyi

- `DELETE /api/agents/{id}` sistem ajanı için HTTP 409 döndürür.
- `POST /api/agents/{id}/restore-default` düzenlenebilir profil alanlarını derlenmiş
  varsayılana döndürür; `Disabled` değerini korur.
- `PUT /api/agents/{id}` ile `disabled` güncellenebilir. `system` veya `systemKey`
  mevcut değerden farklı gönderilirse HTTP 400 döner; sistem kimliği sonradan
  değiştirilemez (`internal/api/agents.go:340-411`).

Rotalar `internal/api/server.go:406-414` içinde kayıtlıdır.

## UI davranışı

Frontend sistem ajanlarını normal ajanlardan sonra ayrı **Sistem ajanları** bölümünde
gösterir. Sistem ajanı seçim ve toplu silme listesine girmez; ayar formunda **Sil**
butonu render edilmez. Bunun yerine **Varsayılana dön** ve **Etkinleştir / Devre dışı
bırak** eylemleri sunulur (`frontend/src/features/agents/AgentsView.tsx:145-175`,
`frontend/src/features/agents/AgentsView.tsx:378-471`,
`frontend/src/features/agents/AgentSettingsForm.tsx:303-321`). Devre dışı bir sistem
ajanında **yerleşik tanım etkin** rozeti görünür
(`frontend/src/features/agents/SystemAgentStatusBadge.tsx:7-17`).

## Sistem ajanı varsayılan ajan olamaz

Workspace'in `defaultAgentId` ayarı (yeni sohbetlerde önseçili ajan) bir sistem
ajanını gösteremez. Sistem ajanı çalışma zamanına hizmet eder (başlıklandırma,
compaction, worker profilleri) ve sohbet muhatabı değildir.

- **Backend kapısı:** `Manager.UpdateSettings` yamayı uygulamadan önce hedefi
  çözer ve sistem ajanıysa `ErrDefaultAgentSystem` döndürür
  (`internal/workspace/settings.go`). `defaultAgentId` için tek yazma yolu burasıdır,
  dolayısıyla HTTP handler'ı, şablon/market kurulumu ve workspace bridge aynı kapıdan
  geçer. HTTP tarafında bu hata 400'e eşlenir
  (`internal/api/workspace_settings.go`).
- **Boot onarımı:** workspace açılışında `sanitizeDefaultAgent` kapı eklenmeden önce
  yazılmış bir değeri temizler ve `Warn` loglar (`internal/workspace/manager.go`).
  Değer temizlenince yeni sohbetler roster'daki ilk ajana düşer.
- **UI:** sistem ajanının ayar formunda **Varsayılan yap** düğmesi hiç render
  edilmez (`frontend/src/features/agents/AgentSettingsForm.tsx`).

Kısıt yalnız "yeni sohbetlerin varsayılan ajanı" içindir; sistem ajanının kendi
işlevi (titler/compaction/worker olarak çağrılması) etkilenmez.

## Workspace seed

Workspace manager'ın ortak `open()` yolu derlenmiş tanımları `EnsureSystemAgents`
ile seed eder (`internal/workspace/manager.go:263-293`). Aynı yol yeni workspace
oluşturulurken ve kayıtlı workspace'ler boot sırasında açılırken çalıştığından eski
workspace'ler de açılışta backfill edilir (`internal/workspace/manager.go:523-537`).
Seed idempotenttir. Eski `compactor` kaydı `overview-summarizer` anahtarına aynı ID ile
yerinde taşınır; soul, model ve disabled dahil kullanıcı özelleştirmeleri korunur.
Aynı `SystemKey` için canlı kayıt varsa alanlarına dokunulmaz — tek istisna boş
`Avatar`/`Color` alanlarının kanonik değerle doldurulmasıdır (bkz. "Görsel kimlik") —,
yalnız eksik kayıt oluşturulur;
yeni kayda `System`, `SystemKey` ve varsayılan `Disabled` değeri dahil tüm tanım yazılır
(`internal/db/store_agent_system.go:38-58`).

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
