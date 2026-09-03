# 82 — Ajan Kalıtımı ve Kilitli Yerleşik Ajanlar

> **Özet (2026-09-03):** Ajanlar artık başka bir ajandan **kalıtım** alabilir
> (`parentId`): override edilmeyen her alan ebeveynin etkin değerinden okunur, `overrides`
> listesi hangi alanların bu ajana özel olduğunu söyler. Uygulamanın kendi işleri için
> kullandığı sistem ajanları (titler, compaction, worker profilleri…) her workspace'te
> **kilitli** (`locked`) yerleşik satırlar olarak tutulur: düzenlenemez, silinemez, her
> açılışta koddan yeniden dayatılır. Özelleştirmek için "Özelleştir" ile rolü devralan bir
> çocuk türetilir; çocuk etkinken rol ondan, değilse yerleşikten çözülür. UI'da ajan
> listesi soldaki renkli şeritlerle kalıtım zincirini, ayar formu alan alan
> "devralındı / override" durumunu ve tek tıkla "devral" (sıfırlama) eylemini gösterir.
> Durum: **uygulandı**. Dayandığı dosyalar: `internal/db/agent_inherit.go`,
> `internal/db/store_agent_system.go`, `internal/db/store_agent_derive.go`,
> `internal/api/agent_inherit.go`, `frontend/src/features/agents/*`.

## 1. Neden

Önceki modelde (bkz. `74-SISTEM-AJANLARI.md`, 2026-08-26 sürümü) her workspace'te sistem
ajanı tek bir **düzenlenebilir** satırdı: kullanıcı titler'ın promptunu değiştirdiğinde
yerleşik tanım kayboluyor, "varsayılana dön" için ayrı bir endpoint gerekiyor, yeni
sürümde iyileşen bir yerleşik prompt zaten dokunulmuş satıra hiç ulaşmıyordu. Ajanlar
arasında ortak ayar paylaşmanın da yolu yoktu: aynı sağlayıcı/model/araç ayarını on ajana
uygulamak on ayrı düzenleme demekti.

İki ihtiyaç aynı mekanizmayla çözüldü: **alan bazlı kalıtım**.

## 2. Veri modeli

`db.Agent` üç alan kazandı (`internal/db/models.go`):

| Alan | Anlam |
|---|---|
| `parentId` | Kalıtım alınan ajan. Boş = kök ajan (her değeri kendine ait). Zincir derinliği sınırsız; döngü yazımda reddedilir (`ErrAgentParentCycle`). |
| `overrides` | Bu ajanın **kendi değerini** kullandığı alan anahtarları (`InheritableFieldKeys`). Listede olmayan alan ebeveynden devralınır. Kök ajanda anlamsız, daima boş yazılır. |
| `locked` | Kilitli yerleşik ajan: kayıt defterinden (`agent.SystemAgentDefaults`) dayatılır; `UpdateAgent`/`UpdateAgentTools`/`DeleteAgent` `ErrAgentLocked` / `ErrSystemAgentDelete` döner. |

Kalıtım birimleri (`internal/db/agent_inherit.go`, `inheritableFields` tablosu tek kaynaktır):
`soul`, `identity`, `provider` (kind + instance birlikte), `model`, `thinkingLevel`,
`nativeWebSearch`, `permissionMode`, `inboundPolicy`, `avatar`, `color`, `tools`
(`mcpEnabled` + `toolOverrides` + türetilmiş `blockedTools` birlikte), `allowedTools`,
`skills`, `coordinatorMode`, `coordinatorWorkflow`, `coordinatorPrompt`.

**Devralınmayan** kimlik alanları: `id`, `name`, `system`/`systemKey`, `disabled`,
`locked`, `parentId`, `overrides`, `createdBy`, `deleted`, zaman damgaları.

### Okuma anında çözümleme

Diskteki satır ham değerleri tutar; override edilmemiş bir alanın ham değeri yalnız
**önbellektir** ve doğrudan okunmaz. `GetAgent`, `ListAgents`, `FindAgentBySystemKey` ve
her mutatorun döndürdüğü değer `resolveAgentLocked` ile katlanmış **etkin** ajandır: zincir
kökten başlayarak katman katman uygulanır (`applyInheritance`). Eksik ebeveyn veya elle
yazılmış döngü okumayı asla kırmaz — yürüyüş bulunabilen son satırda durur
(`maxInheritanceDepth = 32`). Oturum oluşturma (`CreateSession`) model anlık görüntüsünü ve
koordinatör varsayılanını da etkin satırdan alır.

### Yazma kuralları (`UpdateAgent`)

- Kilitli satır → `ErrAgentLocked`.
- Çocukta patch'in dokunduğu her alan `overrides`'a eklenir; `ResetFields` listesindekiler
  çıkarılır ve ham değer ebeveynin etkin değerinden tazelenir (disk okunabilir kalsın).
- `ParentID` geçişleri: `"" → X` kök ajanı **davranışı değişmeden** çocuğa çevirir (tüm
  alanlar override sayılır, tek tek "devral" ile bırakılır); `X → ""` etkin değerleri
  dondurup kök yapar (`materialize`); `X → Y` override kümesini korur, gerisi Y'ye göre
  yeniden çözülür.
- `Disabled: false` yazımı, aynı `SystemKey`'i etkin olarak sağlayan başka bir özelleştirme
  varsa `ErrSystemRoleTaken` döner (rol başına en fazla bir etkin özelleştirme).
- `UpdateAgentTools` çocukta `tools` + `allowedTools` birimini birlikte override eder.

### Silme ve yeniden bağlama

Bir ajan silinince (yumuşak) doğrudan çocukları `reparentChildrenLocked` ile büyükebeveyne
bağlanır; silinen katmanın katkısı çocuğun override'larına katlanır, böylece **etkin
değerleri bayt bayt aynı kalır**. Büyükebeveyn yoksa çocuk kök olur (materialize).

## 3. Kilitli yerleşik ajanlar ve rol çözümleme

`EnsureSystemAgents` (`internal/db/store_agent_system.go`) artık her tanım için **kilitli**
satırı garanti eder ve mevcut kilitli satıra kanonik değerleri her açılışta yeniden yazar
(`canonicalAgent` + `imposeCanonical`; değişiklik yoksa disk yazılmaz). Kilitli satır
hiçbir zaman `disabled` olmaz — o zaten koşulsuz fallback'tir; tanımlardaki eski
`Disabled` alanı kaldırıldı.

Rol çözümleme sırası (`FindAgentBySystemKey`):

1. aynı `SystemKey`'li **etkin, kilitsiz** özelleştirme (varsa en eski),
2. kilitli yerleşik satır,
3. (yalnız göç öncesi depolar için) devre dışı özelleştirme.

`ResolveSystemAgent` değişmedi: bulduğu satır devre dışı değilse onu kullanır. Yerleşik
satır artık gerçek bir ID taşıdığından oturumlar/usage ona bağlanabilir; derlenmiş
tanımdan ID'siz ajan kurma yolu yalnız tohumlanmamış depolar için kaldı.
`FindBuiltinAgentBySystemKey` doğrudan kilitli satırı verir.

### Göç (eski tek satırlı model → kilitli + çocuk)

Her tanım için, kilitli satır yoksa **en eski** eski satır **yerinde** kilitli yerleşiğe
çevrilir: ID'si korunur (worker oturumları ona bağlıdır), değerleri kanonik olana döner —
taşıdığı eski düzenlemeler **atılır**, çünkü bu modelde rol yalnız türetilmiş çocukla
özelleştirilir. Aynı anahtarın diğer kilitsiz satırları (devre dışı bırakılmış
`worker:<profil>` kopyaları) yerleşiğin altına bağlanır. `compactor → overview-summarizer`
anahtar göçü ve `worker:<profil>` benimseme bu adımdan önce çalışır.

**Onarım (2026-09-04).** İlk sürüm, tanımdan farklı her eski satırı yeni bir yerleşiğin
altında çocuk olarak tutuyordu; farklar kullanıcı düzenlemesi değil, tanımın altından
kaymasıydı (eski prompt metni, eski spawn yolunun klonladığı codex sağlayıcı/model,
thinking). Bu da her workspace'te 8–12 gereksiz "özelleştirme" bıraktı. Artık her açılışta
`collapseLegacyChildLocked` çalışır: kilitli satırdan **daha eski** tek bir çocuk varsa
(bilinçli özelleştirme her zaman yerleşikten sonra türetilir, dolayısıyla daha yenidir) o
çocuk yerinde yerleşiğe çevrilir ve göçün yarattığı, hiçbir oturumun referans vermediği
kilitli satır silinir. Kilitli satıra bir oturum bağlanmışsa veya birden fazla eski çocuk
varsa dokunulmaz. WS5'in kopyası üzerinde kuru koşu: 29 → 18 ajan, 14 rolün tamamı
orijinal ID'leriyle kilitli.

## 4. Türetme (`DeriveAgent`)

`DeriveAgent(parentID, {Name, BindRole})` her alanı devralan (override'sız) bir çocuk
oluşturur; ham satırı ebeveynin etkin değerleriyle tohumlanır. `BindRole` yalnız sistem
ebeveynde geçerlidir: çocuk `System+SystemKey` taşır ve etkin olduğu sürece rolü sağlar;
etkin bir özelleştirme zaten varsa `ErrSystemRoleTaken`. `BindRole` olmadan yerleşikten
türetilen ajan **sıradan** bir ajandır (rolü almaz; ör. "Titler'dan türeyen Türkçe
özetleyici").

`POST /api/agents/{id}/duplicate` (klon) kilit/rol taşımaz: yerleşiğin klonu etkin
değerleri dondurulmuş bağımsız bir kök, çocuğun klonu aynı ebeveyn + aynı override kümesi.

## 5. API

| Uç | Davranış |
|---|---|
| `GET /api/agents`, `GET /api/agents/{id}` | Çözümlenmiş ajan; `parentId`, `overrides`, `locked` alanları dahil. |
| `POST /api/agents` | `parentId` verilirse `createDerivedAgent`: türet + yalnız ad ve (boş değilse) soul/identity/avatar/renk override. Sağlayıcı/model/thinking doğrulanmaz, devralınır. |
| `PUT /api/agents/{id}` | `parentId` (yeniden bağlama/ayırma) ve `resetFields` alanları eklendi. Kilitli → **409**; rol çakışması → 409; kötü ebeveyn/döngü/bilinmeyen anahtar → 400 (`writeAgentWriteError`). |
| `POST /api/agents/{id}/derive` | `{name?, bindRole?}` → 201 çocuk. `bindRole` için varsayılan ad `"<ebeveyn> (özel)"`, aksi halde `"<ebeveyn> (kopya)"`. |
| `POST /api/agents/{id}/restore-default` | Anlamı değişti: çocuğun **tüm override'larını** kaldırır (`ClearAgentOverrides`). Kilitli → 409, kök → 404. |
| `DELETE /api/agents/{id}` | Kilitli → 409 (`built-in agent cannot be deleted; derive a copy to customise it`). Sistem **özelleştirmesi silinebilir** (rol yerleşiğe döner). |
| `POST /api/agents/{id}/tools` | Kilitli → 409. |

## 6. UI (`frontend/src/features/agents`)

- **Ajan listesi:** her satırın solunda `AgentLineageStripes` — atalar için kökten
  ebeveyne birer dikey renk çubuğu (rengi atanın avatar rengi, hover'da adı). İki katmanlı
  kalıtım iki çubuk gösterir. Alt satır "← <ebeveyn>" ekler. "Sistem ajanları" bölümü
  **gruplanır**: kilitli yerleşik, hemen altında rolü devralan özelleştirme(ler).
  Rozetler (`SystemAgentStatusBadge`): 🔒 *yerleşik* / *rolü sağlıyor* / *yerleşik tanım
  etkin* (devre dışı özelleştirme).
- **Yeni Ajan formu:** "Kalıtım (ebeveyn ajan)" seçimi. Ebeveyn seçilince
  sağlayıcı/model/koordinatör alanları gizlenir (devralınır), yalnız ad + isteğe bağlı soul
  girilir.
- **Ayar formu (`AgentSettingsForm`):**
  - Başlıkta `AgentLineageChips`: "Kalıtım: Titler › Türkçe Titler › (bu ajan)" — atalara
    tıklanınca o ajana gidilir, kilitli atalarda kilit simgesi.
  - Kilitli yerleşikte tüm alanlar `<fieldset disabled>` içinde, Kaydet/Sil/Devre dışı
    yok; açıklayıcı not ve **Özelleştir** (türet + rolü bağla) düğmesi. Özelleştirme zaten
    varsa "Özelleştirmeyi aç" onu seçer.
  - Çocukta her kalıtımsal alanın etiketinde `FieldOverrideBadge`: gri **devralındı ←
    <ebeveyn>** ya da vurgulu **override** + **↺ devral**. Alanı düzenlemek onu override eder;
    "devral" yerel değeri ebeveynin değerine çevirir ve kaydette `resetFields`'a yazar.
    Kaydet yalnız **override edilen alanları** + adı gönderir (dokunulmayan alan devralmaya
    devam eder). Araçlar bölümü anında kaydettiği için override rozeti sunucu değerinden
    okunur, "devral" `resetFields:['tools','allowedTools']` ile anında gider.
  - "Kalıtım (ebeveyn ajan)" seçimi (sistem özelleştirmesi hariç) anında kaydedilir;
    onay metni geçişin ne yaptığını söyler. **Türet** her ajanda vardır; **Tümünü devral**
    (eski "Varsayılana dön") yalnız çocukta.
- Yardımcılar: `shared/lib/agentLineage.ts` (`lineageOf`, `eligibleParents`,
  `isSelfOrDescendant`, `OVERRIDE_LABELS`).

## 7. Testler

- `internal/db/agent_inherit_test.go`: türetme, override işaretleme/sıfırlama, tools birimi,
  ebeveyn geçişleri + döngü, silmede yeniden bağlama, `BindRole` kısıtları, oturumun etkin
  ajandan tohumlanması.
- `internal/db/store_agent_system_test.go`: kilitli tohum, kanonik yeniden dayatma, eski
  satırın yerinde çevrilmesi, özelleştirilmiş eski satırın çocuk olması, rol çözümleme
  sırası, silme kapısı.
- `internal/api/agents_system_test.go`: derive+bindRole, tek özelleştirme, override →
  restore, kilitli 409, reparent/reset/döngü durum kodları.
- `frontend/.../AgentSettingsForm.inherit.test.tsx`, `shared/lib/agentLineage.test.ts`.
- **Canlı (gerçek claude-cli, `TIONHARNESS_LIVE_INHERIT=1`):** `internal/agent/inherit_live_test.go` —
  `TestLiveInheritedTitler` yerleşik titler → soul override edilmiş özelleştirme (başlık zorunlu
  işaretle başlar) → özelleştirme devre dışı (işaret kaybolur) üçlüsünü haiku ile koşturur;
  `TestLiveInheritedSoulReachesCompletion` override'sız türetilmiş ajanın devraldığı soul'un gerçek
  tamamlamaya ulaştığını gösterir. 2026-09-03'te ikisi de geçti.

## 8. Bilinen sınırlar

- Market/şablon paketleri `parentId` taşımıyor; paketten kurulan ajan kök gelir.
- Self-management araçları (`create_agent`/`update_agent`) türetme sunmuyor; kilitli
  ajana yazma denemesi hata döner.
- `inboundPolicy` kalıtım birimi var ama ayar formunda editörü yok.
- Boot'ta tohumlanan otomasyonlar hedefi `FindAgentBySystemKey` ile o anki sağlayıcıya
  sabitler; sonradan türetilen özelleştirme mevcut otomasyonun hedefini değiştirmez.
