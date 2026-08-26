# 74 — Sistem Ajanları

> **Durum: UYGULANDI (2026-08-26).** Başlıklandırma, sıkıştırma, ders çıkarma ve
> içgörü analizi gibi uygulama içi LLM işleri, workspace içinde özelleştirilebilen
> fakat güvenli yerleşik tanımlara düşebilen sistem ajanlarıyla yürütülür.

## Kavram ve kayıt defteri

Sistem ajanı, normal ajan profilindeki `System: true` ve kararlı `SystemKey` ile
işaretlenen özel ajandır. Ayrı bir oturum türü veya `Session` alanı yoktur; bir
oturumun sistem ajanına ait olup olmadığı ajan kaydından çözülür
(`internal/agent/systemsession.go:10-21`).

Derlenmiş kayıt defteri dört rol tanımlar:

| `SystemKey` | Varsayılan durum | Prompt anahtarı | Görev |
|---|---|---|---|
| `titler` | Etkin | `title` | İstek ve konuşmalar için kısa başlık üretir (`internal/agent/systemagents.go:13-18`, `internal/agent/titler.go:83-95`). |
| `compactor` | Etkin | `summary` | Yapılandırılmış workspace verisini özetler (`internal/agent/systemagents.go:21-26`, `internal/agent/summarizer.go:65-80`). |
| `lesson-extractor` | Etkin | `lesson` | Başarısız ajan turlarından yeniden kullanılabilir dersler çıkarır (`internal/agent/systemagents.go:29-34`, `internal/agent/lessons_systemagent.go:5-7`). |
| `insight` | **Devre dışı** | `insight-analyzer` | Oturum kanıtlarında tekrarlanan, eyleme dönük bulguları analiz eder (`internal/agent/systemagents.go:37-43`, `internal/agent/lessons_systemagent.go:9-10`). |

Kayıt defteri promptları `prompts.Default(<anahtar>)` ile alır. Rol çözümlemesi
başarısız olduğunda kullanılan `readPrompt(<anahtar>)` aynı anahtara gider; böylece
özelleştirme yokken yerleşik ve eski gömülü davranış aynı promptu kullanır. Örneğin
compactor için iki yol da `summary` anahtarındadır
(`internal/agent/systemagents.go:21-24`, `internal/agent/summarizer.go:65-80`).

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

Çağrı yerleri (`titler`, `compactor`, `lesson-extractor`, `insight`) önce bu
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

## Workspace seed

Workspace manager'ın ortak `open()` yolu dört derlenmiş tanımı `EnsureSystemAgents`
ile seed eder (`internal/workspace/manager.go:263-293`). Aynı yol yeni workspace
oluşturulurken ve kayıtlı workspace'ler boot sırasında açılırken çalıştığından eski
workspace'ler de açılışta backfill edilir (`internal/workspace/manager.go:523-537`).
Seed idempotenttir: aynı `SystemKey` için canlı kayıt varsa alanlarına dokunmaz, yalnız
eksik kaydı oluşturur. Böylece yeniden açılış kullanıcı özelleştirmelerini ezmez;
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
çağrısı `system:titler:title`, compactor'ın sıkıştırma çağrısı
`system:compactor:summary` olarak kaydedilir (`internal/agent/callkind.go:114-126`,
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
