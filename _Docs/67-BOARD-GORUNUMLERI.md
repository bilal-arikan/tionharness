# 67 — Board Görünümleri: filtre çubuğu, gruplama ekseni ve kayıtlı görünümler

> **Uygulandı.** Kanban panosunun üstüne bir *görünüm katmanı* eklendi: tek veri
> kümesi, çok eksende gruplanabilen sütunlar, facet filtreleri ve workspace
> başına kaydedilen görünüm önayarları.

## Neden

Kanban 30 kartta iyi, 300 kartta çöker. Sebep panonun kendisi değil, panonun
**tek eksenli ve filtresiz** olmasıydı:

| Eksik | Sonuç |
|-------|-------|
| Filtre yok | Her açılışta tüm workspace tek ekranda |
| Tek gruplama ekseni (`boardState`) | "Hangi ajan neyle meşgul" sorusu panoda cevapsız |
| `startDate` / `dueDate` / `progress` UI'da yok | Backend'in sakladığı alanlar ölü veri |
| `dependencies` yalnızca rozet | Bloke işler görünmez |

Veri modeli zaten hazırdı (`db.Task`: `priority`, `tags`, `dueDate`,
`dependencies`, `progress`). Eksik olan tek şey onları **eksen** olarak
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
[66-VIEW-KATMANI](66-VIEW-KATMANI.md) projeksiyonunun aynı yapıyı board lens'i
olarak kullanabilmesi. Opak JSON blob saklamak bugün daha kolaydı, o iki kapıyı
kapatırdı.

## Facet semantiği

Facet'ler arası **AND**, facet içi **OR**. Boş dizi / boş string = facet pasif.
`BoardFilter{}` her kartı eşler.

| Facet | Tip | Not |
|-------|-----|-----|
| `text` | string | başlık + açıklama, alt-dize |
| `priorities` | çoklu | `critical\|high\|medium\|low` |
| `tags` | çoklu | serbest |
| `agentIds` | çoklu | `"-"` = atanmamış (sentinel) |
| `columns` | çoklu | `boardState` anahtarları |
| `dues` | çoklu | `overdue\|today\|week\|none` |
| `dep` | **tekil** | `blocked\|ready` |

`dep` bilinçli olarak tekil: bir kartın "bloke" ve "hazır" olması aynı anda
mümkün değil, dolayısıyla radyo düğmesi — çoklu seçim yanlış bir zihinsel model
kurardı. `dues` ise çoklu, çünkü en faydalı kombinasyon ("bugün **veya** zaten
gecikmiş") tekil enum ile ifade edilemiyordu.

### Türkçe arama katlaması

Metin araması `toLocaleLowerCase('tr')` **kullanmaz**. Türkçe küçültme `I`→`ı`
yaptığı için "LOGIN" yazıldığında "Login" başlığı eşleşmiyordu. Değişmez
küçültme ise ters yönde bozuk: `İ`→`i`+birleşik nokta, bu da "istanbul"u
kaçırıyor. `foldForSearch()` her iki formu düz `i`'ye katlar — arama için
noktalı/noktasız ayrımı anlamsız. Regresyon testi:
`filterTasks.test.ts` › *treats dotted and dotless I as interchangeable*.

## Gruplama ekseni

Asıl kaldıraç. Sütunlar sabit okunmak yerine eksenden **türetilir**:

| Eksen | Sütunlar | Sürükleme neyi yazar |
|-------|----------|----------------------|
| `status` | workspace'in `boardColumns` listesi (aynen) | `boardState` |
| `agent` | kart sahibi olan ajanlar + "Atanmamış" | `ownerAgentId` |
| `priority` | Kritik→Düşük + "Önceliksiz" | `priority` |
| `tag` | kullanılan etiketler + "Etiketsiz" | `tags` (ekler) |
| `due` | Gecikmiş / Bugün / Bu hafta / Tarihsiz | — (reddedilir) |

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
"Kaydet"e basmak her zaman *yeni* bir görünüm üretir.

### Aktif görünüm neden ayarlarda değil?

Çoklu pencere ([30-COKLU-PENCERE](30-COKLU-PENCERE.md)) destekleniyor. Aktif
görünüm ayarlarda saklansaydı iki pencere birbirinin seçimini ezerdi. Tanım
paylaşılır (`ws-settings.json`), **seçim pencereye özeldir**
(`localStorage: tionharness.board.viewId`).

Başka pencerede silinen bir görünüm bu pencereyi boşluğa bakar durumda bırakır;
bu durumda seçim sessizce "Tümü"ye düşer — türetilmiş, effect'siz.

## Sessiz veri kaybı riskleri ve alınan önlem

Üçü de "kullanıcı gördüğünün her şey olduğunu sanır" sınıfından:

1. **Toplu seçim** — `orderedIds` artık yalnızca *görünen* kartlardan kurulur.
   Aksi hâlde Shift+Click aralığı ekranda olmayan kartları seçer ve toplu silme
   görünmeyen işi silerdi.
2. **Sayaç** — filtre aktifken `12 / 87` gösterilir, sadece `12` değil.
   Özellikle ajanlar arka planda kart eklerken kritik.
3. **Filtreden düşen kart** — sürükleme sonrası kart filtreye uymuyorsa anında
   kaybolur. Doğru ama sessiz; ipucu satırı + **Geri al** eklendi.

## Dosyalar

| Dosya | Rol |
|-------|-----|
| `internal/db/models_board_view.go` | `BoardFilter`, `BoardViewDef`, doğrulama |
| `internal/workspace/settings.go` | `BoardViews` alanı + patch + apply |
| `internal/api/workspace_settings.go` | DTO + 400 doğrulama + SSE yayını |
| `frontend/src/features/tasks/views/boardViewTypes.ts` | hazır görünümler, etiketler, slug |
| `.../views/filterTasks.ts` | facet değerlendirme, arama katlaması, sıralama, topo |
| `.../views/deriveColumns.ts` | eksen → sütun, sürükleme → patch |
| `.../views/useBoardView.ts` | seçim + düzenleme + kalıcılık |
| `.../views/BoardFilterBar.tsx` | çubuk |
| `.../views/FacetDropdown.tsx` | tek facet menüsü |
| `.../views/SavedViewMenu.tsx` | görünüm seçici |

Testler: `filterTasks.test.ts`, `deriveColumns.test.ts` (saf fonksiyonlar),
`models_board_view_test.go` (doğrulama).

## Klavye

`/` aramaya odaklanır (bir alana yazarken yok sayılır), `Esc` aramayı temizler.

## Sıradaki adımlar

- **Hiyerarşi (`parentID`)** — üst düzey kartlar + `3/7` rozeti; alt kartlara
  zoom. Kalabalığa karşı filtreden sonraki en büyük kazanç.
- **Timeline / Gantt** — `startDate`/`dueDate`/`progress` artık düzenlenebilir;
  `deriveColumns` altyapısının üstüne bir zaman ekseni oturur.
- **Ajan tarafı görünüm üretimi** — `BoardViewDef` zaten tipli; bir
  self-management aracı "bloke işlerimi göster" görünümünü kendi kaydedebilir.
