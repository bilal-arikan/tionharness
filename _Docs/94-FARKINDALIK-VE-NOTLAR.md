# 94 — Workspace Farkındalığı ve Hafıza Notları

> **Özet (2026-10-04):** **Uygulandı.** Ajanın oturum başında, her turda ve tur sonunda
> workspace'i nasıl gördüğünü tek bir sözleşmeye bağlayan katman (`internal/awareness`)
> ve bunun yazılabilir hafızası (`internal/notes`). Üç an: **brifing** (oturumun ilk
> turunda derlenir, prompt epoch'un donmuş öneğine girer, compaction'da yenilenir),
> **tur** (volatile suffix; bölümler önceliklerine göre bütçeye sığdırılır, her tur sonu
> satırı bir **sayaç**tır, workspace **nabzı** yalnız değişince basılır) ve **kapanış
> özeti** (her tamamlanan turda LLM'siz hesaplanan `digest.json`, bir sonraki oturumun
> brifinginde "son bitirilen işler"). Notlar Markdown + frontmatter'dır (`store/notes/`),
> yazılırken **erişim** (`workspace|agent|project`), **güven**
> (`verified|inferred|unverified`) ve **supersede** zinciri taşır; silinmez, arşivlenir
> ya da düzeltilir. Dersler (`lessons.jsonl`) ve `read_lessons/delete_lesson`
> kaldırıldı; ders çıkarıcı ve insight `lesson` notu yazar. Araçlar: `remember`,
> `record_work`, `note_search`, `note_expand`, `note_correct`. Dört karar mercii
> (`brief-relevance`, `pulse-urgency`, `wrap-up-memory`, `note-supersede`) varsayılan
> gölgede. Esin kaynağı: [breferrari/obsidian-mind](https://github.com/breferrari/obsidian-mind).
> Dayandığı dosyalar: `internal/notes/*`, `internal/awareness/*`,
> `internal/agent/{awareness,awareness_preview,decide_awareness,lessons}.go`,
> `internal/tools/builtin_notes.go`, `internal/view/note.go`, `internal/api/{notes,chat_turn}.go`,
> `frontend/src/features/notes/*`.

## 1. Neden

[66-VIEW-KATMANI](66-VIEW-KATMANI.md) "büyük durumu küçült" işinin dört ayrı yerde
birbirinden habersiz yapıldığını saptamış ve projeksiyon katmanını kurmuştu; ama hiçbir
eski özetleyici göç etmemişti. `composeTurnRequest` on iki bloğu ortak bütçesiz art arda
ekliyordu; oturumun ne gördüğü ölçülmüyordu; oturum bittiğinde workspace'e ne yaptığı
hiçbir yerde kalmıyordu; ajanın "şunu hatırla" diyebileceği bir araç yoktu (çekirdek
bellek 2026-07-05'te kaldırılmıştı, [arsiv/31](arsiv/31-MEMGPT-CORE-MEMORY.md)).

Obsidian Mind'dan alınan üç ders:

1. **Üç teslim anı, tek bütçe, tek sayaç.** Hook çıktısının üstünde bir tavan var;
   tavana vurulduğunda en ucuz bölümler işaretçiye düşer ve son satır neyi düşürdüğünü
   söyler. Sessiz kayıp yanıltır; adlandırılmış kayıp sinyaldir.
2. **Hafıza bilgi notudur, kendi promptunu düzenleyen bellek değil.** Ekle-yalnız
   Markdown, erişim yazım anında bildirilir, okuyan genişletemez; düzeltme üzerine
   yazmaz, geçersiz kılar.
3. **Yasak yayılır, yönlendirme yayılmaz.** Bu yüzden brifing "şu araçlar var" demekle
   kalmaz, notların kendisini (bütçe içinde) taşır ve dijest'ler bir sonraki oturuma
   kendiliğinden ulaşır; danışmak niyete bırakılmaz.

## 2. Paketler ve import yönü

```
notes      ← leaf (yalnız stdlib + archive sözlüğü)
awareness  ← notes, view, progress, db (modeller)
view       ← notes (KindNote, CategoryNotes)
tools      ← notes, awareness (bridge arayüzü), view
agent      ← awareness, notes, tools, decider
api        ← agent, awareness, notes
workspace  ← awareness (Settings)
```

`scripts/depcheck.sh` `notes` ve `awareness`'ın `internal/agent`'ı import etmediğini
denetler.

## 3. Not sözleşmesi (`internal/notes`)

Dosya: `<store>/notes/NOTE<n>.md`, frontmatter + gövde. Alanlar:

| Alan | Anlam |
|---|---|
| `kind` | `lesson` · `decision` · `work` · `gotcha` · `pattern` · `profile` · `reference` |
| `scope` + `agents[]` / `projects[]` | Erişim. `workspace` herkese; `agent` listelenen ajanlara; `project` çalışma dizini listelenen köklerden birinin altında olan oturumlara. Workspace'ler fiziksel izole olduğu için daha geniş kapsam **yoktur**. |
| `confidence` + `verification` | `verified` için doğrulama zorunlu. |
| `supersedes` / `superseded_by` | Düzeltme zinciri. Geçersiz kılınan not yalnız düzeltmesiyle birlikte servis edilir; başlık çözümlemesi aktif notu seçer. |
| `source_session` / `source_agent` / `source` | Runtime damgalar; model iddia edemez. |
| `signature` + `occurrences` | Makine yazımı notların tekilleştirilmesi (ders şekli). |
| `private` / `archived` | Ajana asla servis edilmez / varsayılan listeden çıkar. Silme yalnız kullanıcıya ve yalnız bağlantısız, zincir dışı notlar için. |

Doğrulayıcı reddeder (kesmez): geçersiz enum, eksik başlık/gövde, kapsamın listesi,
doğrulamasız `verified`, 20 KB üstü gövde, köşeli parantezli başlık. Wikilink
(`[[Başlık]]` / `[[NOTE12]]`) ileri bağlantı olabilir; çözülemeyenler uyarıdır.

Arama: her sözcük zorunlu (AND), başlık ve etiket vuruşu ağırlıklı, yenilik ve
`verified` küçük bonus. Semantik katman yok; zvec-grep ile sonradan yeniden
sıralanabilir, hiçbir şey kaybolmaz.

## 4. Üç an (`internal/awareness`)

**Üretici arayüzü.** `Producer{Key, Moments, Produce(ctx, Input) Section}`;
`Section{Key, Text, Pointer, Priority, Volatile}`. `Priority 0` sabitlenmiş: hiç
düşmez. `Compose(moment, sections, budget)` bütçe aşımında en yüksek önceliği (eşitlikte
en büyüğü) işaretçisine düşürür, işaretçisi olmayanı atar, yalnız sabitlenmişler
kaldıysa son bölümü satır sınırında keser ve işaretler. Son satır sayaç:
`[context meter · turn · 4.1KB of 16.0KB · 9 sections · pointer: tool-recap · dropped: -]`.
Bir bileşimin `Hash`'i sayaçtan bağımsızdır.

**Brifing (`MomentBrief`).** Üreticiler sırasıyla: giriş (p0), workspace kartı
(`view.ProjectWorkspace`, p2; işaretçisi tiny), açık döngüler (bekleyen sorular,
stuck/blocked oturumlar, 24s'te düşen koşular, bayat ve düşen kartlar; p1), devralınan
progress dosyası (p0), erişen notlar (kural sırası: ders/gotcha → karar → örüntü →
iş; önce kendi ajanının; `verified` önce; p3), son dijest'ler (p4), diğer oturumlar
(p5). `Service.Brief` sonucu oturum başına dondurur; `InvalidateBrief` yalnız epoch'un
zaten adapte olduğu yerde çağrılır (ilk tur, compaction fold). Volatile bir bölüm
brifinge girmeye kalkarsa reddedilir.

**Tur (`MomentTurn`).** API yolu (`composeTurnRequest`) ve headless yol
(`autonomousDynamicSuffix`) kendi bölümlerini verir: hook bağlamı, saat, oturum kimliği,
araç özeti (p7, işaretçili), geri bildirim (p6), çalışma dizini, ortam, kabuk, scratchpad
(hepsi p0), koordinatör durumu (p1, işaretçili), epoch notu (p0). Katman ekler: yapılacak
listesi (p0), oturum artifact'leri (p6), nabız (p2). Nabız içerik hash'i önceki turla
aynıysa basılmaz; acil (bekleyen/stuck/failed ya da `pulse-urgency` kararı) ise
sabitlenir ve dikkat satırı eklenir. Varsayılan bütçe 16 KB.

**Kapanış (`MomentWrap`).** `BuildDigest` oturum başlığı + son 200 mesajın araç
histogramı (çağrı/hata), yapılacak listesi, artifact'ler, bu oturumun yazdığı notlar,
çocuk oturumlar (koordinatör/handoff), bekleyen soru, son hata, açık döngüler ve
`SuggestNote` (kural: ≥2 araç hatası ya da stuck ya da düşen çocuk ve hiç not yok).
`AutoTagTurn` kanalında her tur sonunda çağrılır; hash değişmediyse yazılmaz. Dosyalar:
`sessions/<id>/digest.json`, `sessions/<id>/awareness.json` (brifing/son tur/nabız
aynası), `awareness/digests.json` (200 satır indeks). Değişince `awareness_digest`
olayı yayımlanır.

## 5. Teslimat yolları

| Yol | Brifing | Tur | Kapanış |
|---|---|---|---|
| native (anthropic / openai-compat) | `buildStaticPrefix` → `System` | `SystemDynamic` | `AutoTagTurn` |
| claude-cli | `--append-system-prompt(-file)` | son kullanıcı mesajı önünde `[Context]` | aynı |
| codex-cli | `developer_instructions` | `[Context]` | aynı |
| headless (schedule/spawn/wake) | `autonomousSystemPrompt` | `autonomousDynamicSuffix` | aynı |

Prompt epoch statik sistemi merkezi derlediği için bağlam teslimi için ek sağlayıcı kodu
yok. Brifing önekte durduğu sürece cache korunur; nabız dedupe'u volatile tarafı küçültür.
Brifing, öneğin yeniden dondurulduğu her noktada yeniden derlenir: ilk tur, compaction
fold'u (`prep.Compacted`), `/refresh-context` ve eşik tetikli epoch yenilemesi
(`refreshPromptEpochLocked` → `InvalidateBrief`).

**CLI köprüsü.** claude-cli ve codex-cli yerleşik araçları Interaction MCP üzerinden alır
ve eager yerleşikler varsayılan olarak köprülenmez (CLI'nın kendi karşılığı olduğu
varsayılır). `remember`, `record_work` ve `note_search` bu yüzden `get_view` gibi
`tools.bridgeEager` listesindedir; `note_expand` / `note_correct` name-only olduğu için
zaten extended katalogda. Canlı doğrulama (2026-10-04): claude-cli ve codex-cli
oturumları beş aracı da çağırabildi, brifing bölüm başlıklarını gördü.

## 6. Karar mercileri (`decide_awareness.go`)

| Merci | Desen | Kural tabanı | Varsayılan |
|---|---|---|---|
| `brief-relevance` | Select | kural sırası (bkz. §4) | gölge, 0.6 |
| `pulse-urgency` | Gate | bekleyen/stuck/failed | gölge, 0.7 |
| `wrap-up-memory` | Gate | `SuggestNoteRule` | gölge, 0.7 |
| `note-supersede` | Gate | başlık Jaccard ≥ 0.8 (araç ≥ 0.5 için sorar) | gölge, 0.75 |

Hepsi `off / shadow / on`; model yokken kural çalışır. Kancalar `awareness.Input`
üzerinden geçer (`RankNotes`, `JudgeUrgent`, `SuggestNote`); `tools.NotesBridge.
Supersedes` runtime'da `decideNoteSupersedes`'e bağlanır.

## 7. Araçlar ve yüzeyler

- Ajan: `remember`, `record_work`, `note_search` (eager), `note_expand`, `note_correct`
  (name-only). `get_view note:<id>`, `expand category:notes`. Erişim bilgisi (ajan kimliği,
  çalışma dizini) runtime'dan gelir; model kapsam listesini yazamaz.
- REST: `/api/notes[...]` (liste, arama, oluştur, güncelle, arşiv, düzelt, genişlet, sil),
  `/api/awareness/digests`, `/api/awareness/settings`, `/api/sessions/{id}/digest`,
  `/api/sessions/{id}/awareness` ("ajan ne gördü": brifing, son tur, nabız, dijest).
- UI: **Notlar** ekranı (Notlar · Dijestler · Bağlam), Explorer'da `Notlar` kovası ve
  `note` yaprağı, Oturum bilgisi panelinde farkındalık bölümleri, workspace ayarlarında
  `awareness` bölümü (`enabled`, bütçeler, sayılar, bayat kart günü).
- Olaylar: `notes`, `awareness_digest`.

## 8. Teslimat kapısı ve canlı test

Canlı test (2026-10-04, ayrı "Awareness Test" workspace'i, claude-cli ve codex-cli
ajanları) ile doğrulananlar: hata senaryoları (doğrulamasız `verified`, `remember` ile
`work`, olmayan not id'si, boş hafızada arama), supersede algılama ve `note_correct`
zinciri, erişim kuralları (özel not `all=true` ile bile servis edilmez; ajan ve proje
kapsamlı notlar brifinge girmez), iki araç hatasının dijest'te `suggestNote`
üretmesi ve ders çıkarıcının arka planda `lesson` notu yazması, nabzın bir kez
basılıp değişmeyince susması ve sorun çözülünce kaybolması, bütçe sıkışmasında
dört bölümün işaretçiye düşmesi, katman kapalıyken brifing/nabız/dijest'in
üretilmemesi, `spawn_session` ile açılan headless oturumun brifing alması, oturum
silmede dijest satırının düşmesi, `/refresh-context` sonrası brifingin yeniden
derlenmesi. Elle `compact-custom` ortamdaki geçersiz Anthropic anahtarı yüzünden
denenemedi; compaction yolu birim testlerle kapalı.

`internal/api/awareness_delivery_test.go` gerçek `/api/chat` ucunu kayıt eden bir
sağlayıcıyla sürer: ilk turda brifing `System`'da ve sayaçla, nabız `SystemDynamic`'te;
ikinci turda `System` byte-aynı, nabız yok, sayaç var; dijest yazılmış;
`/awareness` ucu bunları gösteriyor. Birim testleri: `internal/notes`,
`internal/awareness` (bütçe/işaretçi/kesme/sayaç, dondurma, dedupe, dijest),
`internal/tools/builtin_notes_test.go`, `internal/agent/awareness_test.go`,
`internal/api/notes_test.go`.

## 9. Kaldırılanlar

`internal/db/store_lessons*.go` (+ `lessons.jsonl`), `internal/tools/builtin_lessons.go`,
`internal/api/lessons.go` ve rotaları, `LessonsContextBlock`, `lessonMaxAgeDays` ayarı,
`internal/api/{todos,sessions_context}.go`, `artifactsContextBlock`, frontend ders
sekmeleri. `lessonReflect` anahtarı kaldı: ders çıkarıcı artık not yazar.

## 10. Açık işler

- Semantik not araması (zvec-grep ile `store/notes` indeksi) ve L2 anlatı (dijest'e
  "neden" paragrafı; sayı üretmeden).
- Hijyen bulgularının insight workspace-opt kanalına otomatik yönlendirilmesi.
- `Running` kümesi yalnız otonom çağrıları görür; etkileşimli turların canlılığı API
  katmanında kalır.
