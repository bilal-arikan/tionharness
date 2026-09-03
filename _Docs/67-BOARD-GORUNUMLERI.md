# 67 — Board Görünümleri: filtre çubuğu, gruplama ekseni ve kayıtlı görünümler

> **Özet (2026-09-03):** Uygulanmış bir özelliktir — Kanban panosuna client-side filtre çubuğu (facet'ler AND/OR karışık), türetilmiş gruplama eksenleri (status/agent/priority/tag/due) ve workspace başına kayıtlı görünümler ekler; büyük panolarda (300+ kart) kullanılabilirliği hedefler. Önemli kararlar: filtreleme sunucu tarafında değil client tarafında (`filterTasks.ts`), aktif görünüm seçimi pencereye özel `localStorage`'da tutulur (ayarlarda değil, çoklu pencere çakışmasın diye), Türkçe arama katlaması `İ/I` sorununu özel `foldForSearch()` ile çözer. Ayrıca kart görsel önizlemesi (TSK437), açılışta değişen kart vurgusu (TSK461) ve doğrulama turu rozeti (review bounces, TSK'lar) gibi sonradan eklenen alt özellikleri kapsar. Ana dosyalar: `internal/db/models_board_view.go`, `frontend/src/features/tasks/views/*`.

> **Uygulandı.** Kanban panosunun üstüne bir _görünüm katmanı_ eklendi: tek veri
> kümesi, çok eksende gruplanabilen sütunlar, facet filtreleri ve workspace
> başına kaydedilen görünüm önayarları.

## Neden

Kanban 30 kartta iyi, 300 kartta çöker. Sebep panonun kendisi değil, panonun
**tek eksenli ve filtresiz** olmasıydı:

| Eksik                                          | Sonuç                                            |
| ---------------------------------------------- | ------------------------------------------------ |
| Filtre yok                                     | Her açılışta tüm workspace tek ekranda           |
| Tek gruplama ekseni (`boardState`)             | "Hangi ajan neyle meşgul" sorusu panoda cevapsız |
| Tarih / yüzde ilerleme alanları gereksizdi     | Kart modeli ve görünüm eksenleri şişiyordu       |
| `dependencies` yalnızca rozet                  | Bloke işler görünmez                             |

Veri modeli kart önceliği, etiket ve bağımlılık alanlarını taşır. Eksik olan tek şey onları **eksen** olarak
kullanan bir katmandı.

## Mimari

```
Task[] ──filterTasks()──► visible[] ──deriveColumns(groupBy)──► DerivedColumn[]
                              │                                      │
                              └──────► cardsByColumn ◄───sortTasks()──┘
```

Üç saf fonksiyon, tek render kodu. Pano render'ı hiç değişmedi — yalnızca
kendisine verilen sütun listesi ve bir sürüklemenin yazdığı alan değişti.

### Filtreleme neden client-side?

`GET /api/tasks` zaten tüm panoyu döndürüyor ve `TaskBoard` onu state'te
tutuyor. Depolama dosya-tabanlı, ölçek workspace başına yüzlerce kart. Sunucu
tarafı sorgu eklemek yeni bir endpoint, yeni bir cache tutarlılığı problemi ve
sıfır kazanç demekti. Filtreleme tamamen `filterTasks.ts` içinde.

Yine de `BoardFilter` **backend'de tipli** tanımlı. Sebebi bugün değil yarın:
ajanların self-management araçlarıyla görünüm üretebilmesi ve
[66-VIEW-KATMANI](66-VIEW-KATMANI.md) projeksiyonunun aynı yapıyı board görünümü
olarak kullanabilmesi. Opak JSON blob saklamak bugün daha kolaydı, o iki kapıyı
kapatırdı.

## Facet semantiği

Facet'ler arası **AND**, facet içi **OR**. Boş dizi / boş string = facet pasif.
`BoardFilter{}` her kartı eşler.

| Facet        | Tip       | Not                           |
| ------------ | --------- | ----------------------------- |
| `text`       | string    | başlık + açıklama, alt-dize   |
| `priorities` | çoklu     | `critical\|high\|medium\|low` |
| `tags`       | çoklu     | serbest                       |
| `agentIds`   | çoklu     | `"-"` = atanmamış (sentinel)  |
| `columns`    | çoklu     | `boardState` anahtarları      |
| `dues`       | çoklu     | `overdue\|today\|week\|none`  |
| `dep`        | **tekil** | `blocked\|ready`              |

`dep` bilinçli olarak tekil: bir kartın "bloke" ve "hazır" olması aynı anda
mümkün değil, dolayısıyla radyo düğmesi — çoklu seçim yanlış bir zihinsel model
kurardı. `dues` ise çoklu, çünkü en faydalı kombinasyon ("bugün **veya** zaten
gecikmiş") tekil enum ile ifade edilemiyordu.

`review` de tekil ve aynı gerekçeyle: `exhausted`, `bounced`'ın **öz alt
kümesidir** (bütçesi dolan kart zaten en az bir kez geri dönmüştür), yani ikisini
birlikte seçmenin anlamı olmazdı. Değerler `Task.reviewBounces` üzerinden okunur:
`bounced` ≥ 1, `exhausted` ≥ `REVIEW_ROUND_BUDGET`.

### Türkçe arama katlaması

Metin araması `toLocaleLowerCase('tr')` **kullanmaz**. Türkçe küçültme `I`→`ı`
yaptığı için "LOGIN" yazıldığında "Login" başlığı eşleşmiyordu. Değişmez
küçültme ise ters yönde bozuk: `İ`→`i`+birleşik nokta, bu da "istanbul"u
kaçırıyor. `foldForSearch()` her iki formu düz `i`'ye katlar — arama için
noktalı/noktasız ayrımı anlamsız. Regresyon testi:
`filterTasks.test.ts` › _treats dotted and dotless I as interchangeable_.

## Gruplama ekseni

Asıl kaldıraç. Sütunlar sabit okunmak yerine eksenden **türetilir**:

| Eksen      | Sütunlar                                    | Sürükleme neyi yazar |
| ---------- | ------------------------------------------- | -------------------- |
| `status`   | workspace'in `boardColumns` listesi (aynen) | `boardState`         |
| `agent`    | kart sahibi olan ajanlar + "Atanmamış"      | `ownerAgentId`       |
| `priority` | Kritik→Düşük + "Önceliksiz"                 | `priority`           |
| `tag`      | kullanılan etiketler + "Etiketsiz"          | `tags` (ekler)       |
| `due`      | Gecikmiş / Bugün / Bu hafta / Tarihsiz      | — (reddedilir)       |

Türetilmiş eksenler yalnızca **dolu** sütunları üretir: 30 ajanlı bir
workspace'te 4'ü kart sahibiyse 26 boş sütun çizilmez. "Değer yok" sütunu
(`__none__`) yalnızca gerçekten değersiz kart varsa eklenir; üzerine bırakmak
alanı **temizler**.

`due` ekseninde sürükleme reddedilir ve gerekçesi ipucu satırında gösterilir.
"Bu hafta yap" iyi tanımlı bir tarih değil; tahmin etmek bir teslim tarihini
sessizce uydurmak olurdu.

## Kayıtlı görünümler

Tanımlar `WSSettings.BoardViews` içinde, `BoardColumns` ile aynı desende: aynı
`GET/PUT /api/workspace/settings`, aynı `board` SSE kanalı. Yeni endpoint yok.

**Hazır görünümler kaydedilmez** — istemci tarafında sabittir:
Tümü · Bugün · Bloke · Ajansız · Gecikmiş. Bir hazır görünümü düzenleyip
"Kaydet"e basmak her zaman _yeni_ bir görünüm üretir.

### Aktif görünüm neden ayarlarda değil?

Çoklu pencere ([30-COKLU-PENCERE](30-COKLU-PENCERE.md)) destekleniyor. Aktif
görünüm ayarlarda saklansaydı iki pencere birbirinin seçimini ezerdi. Tanım
paylaşılır (`ws-settings.json`), **seçim pencereye özeldir**
(`localStorage: tionharness.board.viewId`).

Başka pencerede silinen bir görünüm bu pencereyi boşluğa bakar durumda bırakır;
bu durumda seçim sessizce "Tümü"ye düşer — türetilmiş, effect'siz.

## Sessiz veri kaybı riskleri ve alınan önlem

Üçü de "kullanıcı gördüğünün her şey olduğunu sanır" sınıfından:

1. **Toplu seçim** — `orderedIds` artık yalnızca _görünen_ kartlardan kurulur.
   Aksi hâlde Shift+Click aralığı ekranda olmayan kartları seçer ve toplu silme
   görünmeyen işi silerdi.
2. **Sayaç** — filtre aktifken `12 / 87` gösterilir, sadece `12` değil.
   Özellikle ajanlar arka planda kart eklerken kritik.
3. **Filtreden düşen kart** — sürükleme sonrası kart filtreye uymuyorsa anında
   kaybolur. Doğru ama sessiz; ipucu satırı + **Geri al** eklendi.

## Kart görsel önizlemesi (TSK437)

Bir karta eklenen (`artifactIds`) **görsel** artifact varsa, kartın en üstünde —
başlığın da üstünde — önizlemesi çizilir. Ek yoksa kart eskisi gibi kalır; boş
kutu veya placeholder yok.

- **Hangi görsel:** her zaman karta **en son eklenen** görsel. Seçim
  `frontend/src/features/tasks/cardImage.ts` içindeki saf `pickCardImage`
  fonksiyonunda; `artifactIds` dizisi **sondan başa** taranır ve ilk `image`
  türü artifact seçilir. Sıra ölçütü bilinçli olarak artifact `createdAt`
  **değil**: hem dosya bırakma hem "mevcut artifact'ı bağla" akışı yeni id'yi
  dizinin **sonuna** ekler, dolayısıyla dizinin kuyruğu "bu karta en son eklenen"
  demektir; `createdAt` ise çok önce yüklenip bugün bağlanan bir görseli öne
  çıkarırdı. Çözülemeyen id'ler (başka yerde silinmiş artifact) atlanır.
- **4:3 sınırı:** kutu sabit `aspect-[4/3]` + `overflow-hidden`, görsel
  `object-cover`. Böylece önizleme yüksekliği her zaman kart genişliğinin tam
  3/4'ü olur, asla aşmaz; farklı orandaki görsel kutunun içine sıkışır, letterbox
  bandı bırakmaz ve kart yerleşimini bozmaz.
- **Veri yolu:** `TaskBoard` yalnızca `kind=image` artifact'larını çeker
  (`api.listArtifacts({ kind: 'image' })`), `cardMeta` içinde id ile indeksler,
  `pickCardImage` ile kart başına seçer ve `fileURL(sourcePath)` ile servis
  URL'ine çevirir. `sourcePath` yoksa (medya baytları diskte değilse) önizleme
  çizilmez. Liste açılışta, `board` SSE tick'inde ve karta dosya bırakıldığında
  (iyimser ekleme) tazelenir.

Test: `frontend/src/features/tasks/cardImage.test.ts`.

## Açılışta değişen kart vurgusu (TSK461)

İlgili workspace'in Boards ekranı açık değilken SSE üzerinden gelen `board`
olaylarının `taskId` değerleri workspace bazlı pending listede tutulur. Başka bir
workspace'in Boards ekranının açık olması olayı görülmüş saymaz. Yalnız `create`, `update` ve `move`
işlemleri kaydedilir; `delete` ile `columns_changed` kart vurgusu üretmez.

`TaskBoard`, ilk başarılı task listesi yüklendiğinde kendi workspace'inin pending
ID'lerini tek sefer tüketir ve yalnız hâlâ mevcut olan kartlarla kesişimi glow
olarak gösterir. Vurgu pano açık kaldığı sürece korunur; ekrandan çıkıp Boards'u
ikinci kez açınca pending liste boş olduğundan kaybolur. Tüketim ref ile korunur;
React StrictMode'un çift effect kurulumu ilk sonucu boş kümeyle ezmez. Eski global
`tionharness:board-last-seen-at` timestamp karşılaştırması kaldırıldı; böylece aynı
saniyede gerçekleşen değişiklikler kaçmaz ve workspace'ler birbirini etkilemez.

Kart glow'u ve normal kart gölgesi karşılıklı dışlayan tek bir `box-shadow`
utility'si olarak uygulanır. İki Tailwind shadow utility'sini aynı elementte
birleştirmek class sırasına göre cascade önceliği vermez; üretilen CSS sırası normal
gölgeyi kazandırıp glow'u görünmez yapabilir.

İlgili saf store ve regresyon testleri:
`frontend/src/features/tasks/boardChangeHighlights.ts` ve
`boardChangeHighlights.test.ts`.

## Dosyalar

| Dosya                                                 | Rol                                                  |
| ----------------------------------------------------- | ---------------------------------------------------- |
| `internal/db/models_board_view.go`                    | `BoardFilter`, `BoardViewDef`, doğrulama             |
| `internal/workspace/settings.go`                      | `BoardViews` alanı + patch + apply                   |
| `internal/api/workspace_settings.go`                  | DTO + 400 doğrulama + SSE yayını                     |
| `frontend/src/features/tasks/views/boardViewTypes.ts` | hazır görünümler, etiketler, slug                    |
| `.../views/filterTasks.ts`                            | facet değerlendirme, arama katlaması, sıralama, topo |
| `.../views/deriveColumns.ts`                          | eksen → sütun, sürükleme → patch                     |
| `.../views/useBoardView.ts`                           | seçim + düzenleme + kalıcılık                        |
| `.../views/BoardFilterBar.tsx`                        | çubuk                                                |
| `.../views/FacetDropdown.tsx`                         | tek facet menüsü                                     |
| `.../views/SavedViewMenu.tsx`                         | görünüm seçici                                       |
| `.../cardImage.ts`                                    | kartın önizleyeceği görsel artifact'ın seçimi (saf)  |

Testler: `filterTasks.test.ts`, `deriveColumns.test.ts` (saf fonksiyonlar),
`models_board_view_test.go` (doğrulama).

## Klavye

`/` aramaya odaklanır (bir alana yazarken yok sayılır), `Esc` aramayı temizler.

## Sıradaki adımlar

- **Hiyerarşi (`parentID`)** — üst düzey kartlar + `3/7` rozeti; alt kartlara
  zoom. Kalabalığa karşı filtreden sonraki en büyük kazanç.
- **Ajan tarafı görünüm üretimi** — `BoardViewDef` zaten tipli; bir
  self-management aracı "bloke işlerimi göster" görünümünü kendi kaydedebilir.

## Doğrulama turu rozeti (2026-09-01)

Bir kart `review` sütunundan çalışma sütununa her düştüğünde `Task.ReviewBounces`
artar (`review → done` bir PASS'tir ve sayılmaz). Kural `db.countReviewBounce`
içindedir ve board state'i değiştirebilen **her iki** yazma yolu onu çağırır:
`MoveTask` (move_task aracı) ve `UpdateTask` (kart sürükleme / kart formunun
`PUT /api/tasks/{id}` isteği). Alan sunucu-sahiplidir — istemcinin gönderdiği
`reviewBounces` değeri yok sayılır. Bu sayaç
koordinatörün doğrulama koşu bandına girdiğini gösteren tek dayanıklı kayıttır —
gerekçe ve backend tarafı: `_Docs\47-KOORDINATOR-COKLU-AJAN.md` §19.5.

Panoda üç yerde görünür:

| Yer | Ne gösterir |
|-----|-------------|
| `TaskCard` rozeti | `↻ N/3`; ilk geri dönüşten itibaren sarı, bütçe dolunca kırmızı |
| `TaskFormModal` bandı | Salt-okunur açıklama + ne yapılacağı (daralt / kullanıcıya sor) |
| `Doğrulama` facet'i | `bounced` / `exhausted` ile panoyu daraltır |

Ajan tarafındaki `get_view board` projeksiyonu aynı durumu kendi sinyal satırıyla
bildirir (`↻ N kart doğrulama bütçesini doldurdu`), kart drill-down'ı ise ilk
geri dönüşten itibaren `↻ doğrulama turu: N/3 başarısız` satırını ekler.

**Bütçe iki yerde yazılıdır ve elle senkron tutulur:** `db.ReviewRoundBudget`
(Go, tek kaynak — runtime + view onu okur) ve `REVIEW_ROUND_BUDGET`
(`frontend/src/features/tasks/reviewGate.ts`). Frontend'in Go sabitini okuma yolu
yok; "3/3" yazarken backend'in 4'te eskale etmesi rozetin hiç olmamasından
kötüdür, o yüzden ikisi birlikte değiştirilir.

Rozet **sıfır turda hiç çıkmaz**: pano zaten yoğun ve "0 başarısız tur" her
kartın normal hâli.
