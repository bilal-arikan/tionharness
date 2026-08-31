# Faz — Zengin Sohbet Arayüzü (Chat UX)

> Sohbet ekranı, [external-agent-oss](https://github.com/external-agent-project/external-agent-oss)
> referans alınarak External Agent benzeri zengin bir render katmanına kavuşturuldu:
> markdown çıktı, tool kullanım kartları, düşünme adımları, dosya satır
> değişimleri (diff), tıklanabilir dosya yolları ve ekranda görsel gösterimi.

## Amaç

Eskiden asistan mesajı düz metin (`whitespace-pre-wrap`) olarak basılıyordu.
Artık bir asistan turu, arkasındaki **aktivite izini** (thinking + ara metin +
tool çağrıları) ve son cevabı zengin biçimde gösterir — tıpkı External Agent'ın
sohbet ekranı gibi.

## Backend

### Aktivite izi (TurnStep)
- `internal/agent/trace.go` — `TurnStep` tipi: `kind` ∈ `text | thinking | tool`,
  ayrıca `tool`, `input`, `output`, `isError` alanları.
- `internal/agent/toolloop.go` — `CompleteWithToolsTraced` eklendi: native agentic
  döngü her iterasyonda ara metni ve her `tool_use` çağrısını (girdi + sonuç)
  sıralı bir `[]TurnStep` izine kaydeder. Eski `CompleteWithTools` bunu çağırıp
  izi yutar (imza uyumluluğu korunur).
- İz **iki yoldan** da üretilir:
  - **Native (anthropic) döngü:** `CompleteWithToolsTraced` her tool çağrısını + ara metni kaydeder.
  - **claude-cli (anahtarsız):** `providers/claudecli.go` artık `--output-format stream-json --verbose`
    kullanır; CLI'ın kendi olay akışını (`tool_use`/`tool_result`/`thinking`/`text`) ayrıştırıp
    `Response.Trace`'e doldurur. `agent/trace.go:traceToSteps` bunu `[]TurnStep`'e çevirir.
    Böylece **API anahtarı olmadan** da tool kartları + ara adımlar görünür.

#### Adım Türleri — işaretler

Adım türlerinin tek kaynağı `frontend/src/shared/stepKinds.ts`'tir (Ayarlar →
**Adım Türleri** ekranını besler). `context_change` adımının diff işaretleri:

| İşaret | Anlamı |
|--------|--------|
| `+` | Snapshot'ta olmayan, canlı bağlamda olan blok (ya da yeni araç) |
| `-` | Snapshot'ta olup canlı bağlamdan düşen blok (ya da kalkan araç) |
| `~` | İki tarafta da olan ama **düzenlenmiş** blok; gövdesi unified diff'tir (` ` değişmeyen, `-` silinen, `+` eklenen satır, atlanan aralıklar `…`) |

Başlıktaki `+N -M` sayaçları da satır düzeyindedir: `+`/`-` blok tek birim sayar,
`~` blok kendi diff'indeki gerçek eklenen/silinen **satır** sayısını ekler — yani
sadece satır silen bir düzenleme `+1` göstermez (`_Docs\57-PROMPT-EPOCH.md`).

#### `compaction` adımı (2026-08-30)

Bağlam katlaması artık düz metin değil, kendi türü olan bir adımdır:
`agent.StepCompaction` (`kind="compaction"`). Yapısal alanlar `TurnStep`
üzerinde taşınır — `foldedMsgs`, `beforeTokens`, `afterTokens`, `trigger`
(`auto` = bütçe eşiği, `manual` = `/compact`, `reactive` = taşma kurtarması).
`text` alanı eski insan-okur satırı (`🗜 Bağlam otomatik sıkıştırıldı — N mesaj…`)
**aynen** korur; böylece eski oturumlar ve bu türü bilmeyen istemciler yine
anlamlı bir şey gösterir. Değerlerin kaynağı `conversation.Compaction`
(`Prepared.Fold` ve `Manager.ForceCompact` dönüşü) — aynı rakamlar debug
journal'ına da yazılır.

Frontend tarafında adım `CompactionCard` ile render edilir
(`frontend/src/features/chat/CompactionCard.tsx`, `TurnSteps.renderStep`
dispatch'i). Kapalı satır: katlanan mesaj sayısı + `öncesi → sonrası` token
rozeti; açıldığında tetikleyici ve (varsa) eski insan-okur satırı görünür.
Yapısal alanlar yoksa (bu türden önce yazılmış oturumlar) kart `text` alanına
düşer — boş kart göstermez.

Kart ayrıca katlamanın kaynağını ve CLI oturum sonucunu taşır: `source`
(`tionharness` | `cli-native`), `provider` (`claude-cli` | `codex-cli`) ve
`sessionAction` (`resume` | `native-compact` | `restart-summary`). Codex 0.148.0+
`exec --json`, snake_case `context_compaction` item'ını started/completed çiftiyle
taşır. App Server'ın camelCase `contextCompaction` item'ı ve
`thread/compact/start` RPC'si `/compact` tarafından doğrudan kullanılan native
transporttur. Claude Code 2.1.238+
`--include-hook-events` ile `PreCompact` başlangıcını; `compact_boundary`,
`system/status compact_result=success` ve `PostCompact` tamamlanma kanıtını taşır.
Completion sinyalleri deduplicate edilir.
Eksik/eski CLI sürümünde capability fail-closed kalır.

Komut sözleşmesi ikiye ayrılır:

- `/compact`, yalnız mevcut resumable `claude-cli` veya `codex-cli` oturumunun
  native compaction kontrolünü çağırır. Claude'da tek başına `/compact` girdisi,
  Codex'te App Server `thread/compact/start` RPC'si kullanılır. Native oturum veya
  desteklenen CLI sürümü yoksa açık hata döner; rolling-summary fallback yapılmaz.
- `/compact-custom`, önceki TionHarness davranışıdır: eski mesajları rolling
  summary'ye katlar, CLI resume durumunu temizler ve sonraki turu özet + son mesaj
  kuyruğuyla fresh CLI session olarak başlatır.

Her iki manuel komutun başarı mesajı aynı yapısal `compaction` adımını kendi
`steps` alanında kalıcı taşır. `/compact`: `trigger=manual`, `source=cli-native`,
`sessionAction=native-compact`; `/compact-custom`: `source=tionharness`, CLI ise
`sessionAction=restart-summary`. Custom fold sonrası hem DB'deki resume id/sınırı
temizlenir hem Claude persistent-pool süreci düşürülür. Auto, wake ve side-chat
fold yolları da persistent süreci provider çağrısından önce düşürür; özet eski
warm transcriptin üstüne eklenmez.

**Adımı yayan yollar.** Katlama nerede olursa olsun aynı kart çıkar:

| Yol | Tetikleyici | Kaynak |
|-----|-------------|--------|
| İnteraktif tur | `auto` | `api.compactionLeadStep(prep.Fold)` — `chat_stream.go` |
| Otonom tur (wake/spawn/worker) | `auto` | aynı yardımcı — `wake_turn.go` |
| `/compact` komutu | `manual` | Claude slash-control veya Codex `thread/compact/start` |
| `/compact-custom` komutu | `manual` | `Manager.ForceCompact` dönüşü |
| Tur içi taşma kurtarması | `reactive` | `agent.reactiveCompactionStep` — `toolloop.go`'daki iki kurtarma dalı |
| Yan sohbet (`btw`) | `auto` | `chat_btw.go`; `Prepare`'in paylaşılan katlaması |

`reactive` yol `conversation.CompactInFlightMessages`'ın döndürdüğü
`ReactiveFold`'u kullanır: tur içi mesaj dilimi katlandığından oturum özeti
yazılmaz, dolayısıyla debug journal kaydında **fold ordinali yoktur**
(`fold # n/a (in-flight)`); `savedBytes`/`summaryBytes` ve token rakamları
gerçek değerlerdir. Bu dalda `compaction` adımı `recovery` adımını **değiştirmez**,
ona eklenir: `recovery` turun neden yeniden denendiğini, `compaction` katlamanın
neye mal olduğunu söyler.

`btw` uç noktası tek bir JSON gövdesiyle yanıt verir (SSE yazıcısı yok) ve hiç
mesaj oluşturmaz (adımın kalıcılaşacağı `Steps` yok) — bu yüzden adım yalnız
session hub'ına yayınlanır: oturumu açık tutan tüm pencereler katlamayı canlı
görür.

### Kalıcılık

#### Canlı adım kartı sözleşmesi (2026-08-21)

Araçlar ve düşünme akışı tek generic canlı kart mekanizmasını kullanır:

- `ID`, canlı kartın kimliğidir; araç kartlarında araç çağrısının `call.ID`
  değeridir.
- `Running: true`, adımın kısmi olduğunu ve UI'da "çalışıyor…" gösterileceğini
  belirtir.
- `Append: true`, aynı ID'deki mevcut kartın `text`/`output` içeriğine parçayı
  ekler. Araç stdout/stderr parçaları artık ayrı `tool_delta` üretmez.
- Aynı ID ile gelen `Append` taşımayan adım mevcut kartı yerinde değiştirir.
  Final adım `Running` ve `Append` taşımaz; kalıcı mesaja yalnız bu kapanmış biçim
  girebilir.
- `tombstone`, yalnız iptal veya panic nedeniyle final kartı gelemeyen canlı kartı
  `Ref` ile geri çeker.

`tool_delta` kind sabiti ve frontend okuma dalı yalnız daha önce kalıcılaştırılmış
eski oturumlar için korunur; yeni üretim yolu değildir.

- `Message.Steps` alanı (`session.jsonl` mesaj satırında JSON dizisi). Tur yeniden
  yüklemede yeniden çizilebilsin diye iz JSON olarak saklanır. (Depolama dosya-tabanlı;
  bkz. `_Docs/08-DEPOLAMA.md` — eski SQLite `0007_message_steps.sql` migration'ının yerini bu alan aldı.)
- `db.Message.Steps` alanı + `AddMessage`/`ListMessages` güncellendi.
- `internal/api/chat.go`: yanıt `steps` alanı döndürür ve izi mesaja yazar.
- **Tur-içi crash kurtarma:** asistan yanıtı yalnız stream bitince persist edildiğinden,
  süreç stream sırasında ölünce tur kaybolurdu. `chat_stream.go` artık her turu
  throttle'lı `inflight.json` sidecar'ına snapshot'lar; boot'ta `db.recoverInflight`
  yarım yanıtı `Message.Interrupted=true` olarak kurtarır → frontend asistan
  balonunda **"Bu yanıt yarıda kesildi (sunucu yeniden başladı)"** banner'ı
  (`MessageList.tsx`). Mekanizma + external-agent karşılaştırması: `_Docs/08-DEPOLAMA.md`.
- **Tur-ortası reload/navigasyon kurtarma (istemci tarafı, crash'ten AYRI):**
  > ⚠️ **Kısmen süperseded (2026-07-11).** Aşağıdaki "sahip / sahip-olmayan pencere"
  > ikiliği **event-sourcing cutover'ıyla kaldırıldı**: her pencere artık
  > `GET /api/sessions/{id}/stream` (per-session `SessionHub`: seq + ring + cursor'lı
  > replay, `internal/sessionhub/hub.go`) üzerinden aynı şekilde render eder.
  > Güncel model: **[58-QUEUE-SENKRON.md](58-QUEUE-SENKRON.md)**. Buradaki inflight
  > sidecar/ghost-balon mekanizması kodda hâlâ vardır ama artık *crash kurtarma* rolündedir,
  > pencere-sahipliği ayrımı için değil. Aşağısı tarihsel bağlam olarak korunmuştur.

  Tur
  `context.WithoutCancel` ile client bağlantısından **detached** çalışır → sayfa
  yenileme/başka sohbete geçiş üretimi kesmez, yanıt yine persist olur. Yeniden
  girişte `App.tsx` `listMessages` ile transkripti kalıcı-mesajlarla yükler; canlı
  balon iki yoldan geri gelir:
  - **Sahip-olmayan pencere** (başka pencere / otonom tur): `recoverInflight`
    `GET /sessions/{id}/inflight` snapshot'ından ghost balon tohumlar, `session_step`
    bus'ı (`applyAutoStep`) canlı büyütür. Not: bus `delta/tool_delta`'yı düşürür
    (`sessionstep.go busForwardable`) → metin gövdesi snapshot throttle'ı (≤600ms)
    kadar tazedir, tam metin tur bitince gelir.
  - **Sahip pencere** (turu bu pencere stream'liyor): `listMessages` in-memory
    `live-*` balonunu siler; `runsRef` sahipliği sürdüğü için `recoverInflight`
    erken döner. `useChatStream` bunu iki savunmayla kurtarır: (1) yerel SSE
    handler'ları **UPSERT** (`syncLive` — balon yoksa biriktirilmiş tam
    metin/iz ile yeniden yaratır, `map`-only artık frame düşürmez); (2)
    `reseedLive` (App effect'inden) `liveBubblesRef`'teki en taze balonu dönüşte
    hemen geri enjekte eder. Böylece "başka sohbete gidip gelince cevap kayboluyor"
    sorunu çözülür.
  - **Metin ilerletme (inflight polling):** bus `delta`'yı düşürdüğü için ghost
    balonun **cevap metni** reload anındaki snapshot'ta donardı. `useChatStream`
    artık aktif session'da **sahiplenilmeyen** bir tur pending iken
    `GET /sessions/{id}/inflight`'i **saniyede bir** çekip yalnız `text` alanını
    ilerletir (iz `steps` bus'a ait kalır → çakışma yok). Tur bitince pending
    temizlenir → poll durur, transkript reload otoriter mesajı koyar.

### Yüzen composer + son mesaj görünürlüğü
- **Overlay yerleşim:** chat view'i `relative` sarmalayıcı; bottom-stack
  (ask/todo/pending/wake banner + `Composer`) `absolute inset-x-0 bottom-0 z-20`
  ile transkriptin **üstüne yüzer** (`pointer-events-none` + `[&>*]:pointer-events-auto`
  → şeffaf boşluklar scroll'a geçer). Mesaj balonları alttan composer'ın arkasına kayar.
- **Gradient + opak input:** composer sarmalayıcısı tema-uyumlu
  `bg-gradient-to-t from-[var(--color-bg)]
  via-[color-mix(in_srgb,var(--color-bg)_85%,transparent)] to-transparent`; iç input
  kartı `bg-[var(--color-surface)]` (opak) + `shadow-lg`. Şeffaf üst kısım balonların
  görünüp arka plana karışmasını sağlar (açık/koyu temada tutarlı).
- **`bottomInset` (bug fix):** overlay yüksekliği `ResizeObserver` ile ölçülüp
  `MessageList`'e scroll padding olarak verilir → en yeni mesaj **daima opak input'un
  üstünde** kalır. Bu, "agent yanıtı tamamlanana kadar son kullanıcı mesajı
  görünmüyor" belirtisini giderir (mesaj artık input'un arkasında saklanmaz).

### Inline görsel sunucu
- `internal/api/files.go` — `GET /api/files?path=<yol>`: sohbet içeriğinde
  referans verilen yerel görselleri inline göstermek için salt-okunur akış.
  Yalnızca görsel uzantıları allowlist'te (png/jpg/gif/webp/svg/bmp/ico/avif).
  `rel=<workspace-göreli yol>` biçimi ayrıca **sandbox içindeki metin dosyalarını**
  (`.md`, `.txt`, `.log`, `.json`, `.ts`, `.go`, … — `textServableExt`)
  `text/plain; charset=utf-8` + `nosniff` ile sunar; `file` türü artifact'ların
  önizlenip indirilebilmesi buna dayanır. Mutlak `path=` biçiminde bu geçerli
  değildir — orada yalnız medya allowlist'i çalışır.

### Adım-adım akış (SSE streaming)
Sohbet artık **her adım bittikçe** UI'a akıtılır (tüm tur bitince değil).

İki streaming yolu vardır:
1. **claude-cli (trace tabanlı):** `providers.Request.OnEvent func(TraceStep)` —
   `claudecli.go` stream-json'u **satır satır** (`bufio`) okuyup olayları anında
   yayınlar: thinking hemen, ara metin flush'ta, tool adımı sonucu gelince.
   `cliStreamParser` (feed/finish, `claudecli_stream.go`) artımlı durumu tutar. JSON ayrıştırması bozulan
   olaylar sessizce kaybolmaz: tur başına yalnız ilk hata, `len(line)` ve ayrıştırma
   hatasıyla birlikte en fazla 500 baytlık `[claude-cli parse drop]` notu olarak
   ize eklenir. Girdi satırına boyut sınırı uygulanmaz; 1 MiB üzerindeki satırlar da
   taranır.
2. **Native token streaming (`providers.Streamer`):** `anthropic` ve `minimax`
   artık birinci sınıf token akışı yapar — `Stream(ctx, req, onDelta)` SSE'yi
   ayrıştırıp her metin parçasını `onDelta`'ya verir. `toolloop.go` araçsız turda
   bir sağlayıcı `Streamer` ise akışı tercih eder ve her parçayı geçici bir
   `StepDelta` (kind `"delta"`) olarak yayınlar. Delta'lar **kalıcı değildir**
   (yalnız canlı UI); tam metin tur sonunda mesaja yazılır.
- `agent/toolloop.go` `CompleteWithToolsStream(... onStep)` — claude-cli yolunda
  `OnEvent`'i `onStep`'e köprüler; native loop her adımı kendisi yayınlar;
  araçsız streaming yolu `recordedStream` ile `StepDelta`'ları yayınlar.
- `internal/api/chat_stream.go` — `POST /api/chat/stream` (SSE):
  `meta` (userMessage) → `step` (her TurnStep) → `done` (replyMessage + sessionTitle).
  Tur sonunda mesaj + tam iz kalıcılaştırılır (yeniden yüklemede aynı görünür).
- Frontend `api.ts:streamChat` — `fetch` + `ReadableStream` ile SSE çerçevelerini
  ayrıştırır. `App.tsx` canlı bir asistan balonu ekler; `onStep`'te `kind:"delta"`
  parçaları **balon metnine eklenir** (token token büyür), diğer adımlar ize
  yazılır; `onReply`/`onDone`'da kanonik mesajla değişir. `MessageList` boş canlı
  balonda "çalışıyor" noktaları gösterir.

## Frontend

Yeni bağımlılıklar: `react-markdown`, `remark-gfm`, `highlight.js`.

### Markdown katmanı (`components/markdown/`)
- `Markdown.tsx` — GFM markdown: başlık, liste, tablo, görev listesi, satır-içi
  kod. Özel render'lar: kod blokları (`CodeBlock`), linkler (yerel yol → tıklanır
  `onOpenFile`, http → yeni sekme), görseller (yerel yol → `/api/files`).
  `urlTransform`, yalnız Windows dosya hedefleri ve inline görseller için özel
  geçiş verir; diğer hedeflerde `react-markdown` güvenli şema filtresi korunur.
  Ortak parser'ın ürettiği Windows link hedefleri ve açık Markdown linklerindeki
  ters bölüler parse/render öncesi `/`'e normalize edilir; görünen metin değişmez.
  Görsel yolu **göreli** de olabilir (`![a](output/images/a.png)`):
  backend `/api/files?path=` parametresi mutlak değilse yolu aktif workspace
  sandbox köküne göre çözer (`internal/api/files.go`); `..` ile kaçış 400 döner.
- `CodeBlock.tsx` — dil etiketi + kopyala düğmesi + `highlight.js` vurgusu;
  `diff` blokları `DiffView`'e, `mermaid` blokları `MermaidDiagram`'a, `gallery`/
  `image-preview` `Gallery`'ye, `html-preview` `HtmlPreview`'e gider.
- `HtmlPreview.tsx` — ```` ```html-preview ```` bloğunu **izole sandbox iframe**'de
  inline render eder (2026-07-06, _Docs/53). Gövde JSON `{"src":"<abs>.html","title"}`
  ya da çoklu-sekme `{"items":[{"src","label"}]}`. Dosya **metin olarak** `fileTextURL`
  (`/api/files?...&as=text`) ile çekilir → `<iframe srcDoc sandbox="allow-scripts">`
  (opaque origin: script çalışır ama parent DOM/cookie/API'ye erişemez). Backend
  `as=text` yalnız workspace **render kökü** (`<store>/render/`) altını `text/plain`
  ile servis eder; asla `text/html` değil. Kaynak: `render_template` aracı çıktısı.
- `DiffView.tsx` — unified diff'i satır bazlı +/- renkli ve `+N / −M` istatistik
  başlığıyla çizer (`lib/diff.ts` ayrıştırır).
- `MermaidDiagram.tsx` — ```` ```mermaid ```` blokunu **tema-duyarlı SVG**'ye
  çevirir. `mermaid@^11` **dinamik `import()`** ile lazy yüklenir; chunk bölmesini
  bundler'ın kendisi yapar (elle `manualChunks` grubu **verilmez** — verildiğinde
  paylaşılan preload helper'ı o dev chunk'ın içine düşüp mermaid'i entry'nin statik
  bağımlılığı hâline getiriyor ve lazy yüklemeyi iptal ediyordu). Sonuç: mermaid
  diyagram tipi başına ayrı chunk'a bölünür, ilk açılışta hiçbiri inmez.
  Tema base'i `<html data-theme>`'ten
  seçilir, renkler CSS değişkenlerinden türetilir, `MutationObserver` tema değişiminde
  yeniden çizer. **Akış-dayanıklı:** 120ms debounce + hatada ham kaynağa düşer →
  yarım kalan diyagram patlatmaz. Toolbar: Source/Diagram, Expand (tam-ekran), Copy.
  `securityLevel: 'strict'`.
- `Gallery.tsx` — ```` ```gallery ```` (alias `image-preview`/`images`) bloğunu
  **thumbnail grid + Lightbox** olarak çizer (the external agent project tarzı, 2026-06-26). Gövde
  JSON `{"title","images":[{"src","alt"}]}` ya da düz satır/virgül-ayrık yol listesi;
  yerel yollar `mediaUrl` (`/api/files`) ile çözülür. Thumbnail'a tık → Lightbox o
  index'te açılır, ok/←→ ile gezinilir. **Video** (mp4/webm/…) item'ı thumbnail'da
  `<video>` + play ikonu, Lightbox'ta oynatıcı olur (`LightboxImage.type`). Tekil
  `![alt](yol)` markdown medyası inline render olur (görsel→tıkla-zoom, video→inline
  player). **Ardışık `![]()` otomatik gruplama:** `Markdown.tsx::groupMediaRuns` art
  arda gelen tam-satır medya satırlarını tek ```gallery'ye çevirir (fence-farkında;
  tek/satır-içi medya inline kalır).
- `shared/components/Lightbox.tsx` — **paylaşılan tam-ekran zoom+pan** önizleyici (2026-06-26):
  tekerlek ile imlece-doğru zoom (translate düzeltmeli), sürükle-pan, **iki
  parmakla pinch-zoom + iki-parmak pan** (dokunmatik; pointer olayları
  `pointers` map'inde tutulur, ikinci parmak inince pinch temel çizgisi
  alınır — 2026-08-20), toolbar
  (uzaklaştır/yüzde/yakınlaştır/sıfırla/kapat), çift-tık toggle, Escape + `0` (reset),
  temiz backdrop tıklamasıyla kapanır (pan'dan sonra kapanmaz). `imageSrc` (görsel) ya
  da `children` (mermaid SVG) **ya da `images[]` + `index` (galeri, ←/→ + ok
  butonları + "n / N" sayaç)** alır. Kullananlar: mermaid Expand, Markdown inline
  görselleri (tıkla→zoom), `Gallery` (çoklu görsel), `UserBubble` attachment
  önizlemesi (eski yerel ImageLightbox kaldırıldı), `ArtifactView` görsel artifact
  (`ImageArtifact`).

### Transkript yükleme maliyeti (2026-07-28)

Bir oturumu her açtığında iz **sunucudan yeniden çekilir** — ama disk'ten değil:
`db.loadSessions` boot'ta tüm `session.jsonl`'leri belleğe alır, `ListMessages`
yalnız kopya döndürür. Maliyet **wire payload'ı + React render'ı**ydı. İkisi de
kırpıldı:

1. **Sunucu-tarafı iz kırpma** (`internal/api/steps_trim.go`) — transkript
   **okuma yolunda** her `TurnStep`'in `output`/`text`/`patch` alanları ve
   `input` içindeki uzun string yaprakları `stepFieldCap` (2 KB) ile kesilir;
   `subSteps` (subagent izi) özyinelemeli taranır. Kesilen alan
   `outputTruncated`/`inputTruncated`/… + `*Len` bayraklarıyla işaretlenir.
   `input`'un **anahtarları korunur** (tool etiketi, program rozeti ve
   sentezlenen Edit/Write diff'i onlardan türer). Disk'e yazılan iz ve **modele
   giden bağlam tam kalır** — kırpma yalnız HTTP kopyasıdır.
   Uygulandığı yerler: `handleListMessages` + `publishHub(KindReply)` (aynı tur
   canlıyken ve reload sonrası farklı görünmesin diye). Ölçüm: gerçek 11-oturumlu
   bir workspace'te iz payload'ı **~%45** küçüldü (cap 1 KB → ~%60 ama sıradan
   tool çıktısını kırpmaya başlar; 4 KB → ~%27).
2. **Tam izi talep üzerine getirme** — `GET /api/sessions/{id}/messages/{msgId}/steps`
   o turun izini kırpılmamış döndürür. UI'da turun 🔧 satırında **"⤓ tam iz"**
   çipi (yalnız kırpılmış tur + `sessionId` varken); `AssistantTurn` çekip
   `steps`'i değiştirir. `ActivityCard` kırpılmış çıktının altına "Sunucu bu
   çıktıyı kırptı (tamamı N KB)" notunu düşer.
3. **Off-screen render atlama** (`MessageList.tsx`) — görünüm dışı satırlara
   `content-visibility: auto` + `contain-intrinsic-size: auto 320px`.
   **Virtualizer DEĞİL** bilerek: transkriptin scroll mantığı (`scrollRowIntoView`,
   `updateActivePinned`, arama deep-link'i) `data-msg-id` ile gerçek DOM
   düğümlerini sorgular; satırları unmount etmek hepsini bozardı. Satır DOM'da
   kalır, yalnız alt-ağacının render'ı atlanır. Guard'lar: transkript
   < `SKIP_OFFSCREEN_MIN_ROWS` (30) ise hiç uygulanmaz, son 3 satır + flash'lanan
   satır daima eager, ve iki atlama yolu (`scrollRowIntoView` + deep-link) tahmini
   yükseklikle ıskalamasın diye **rAF'ta ikinci kez hizalanır**.
4. **Memoizasyon** — `AssistantTurn`/`UserTurn`/`PeerTurn`/`TurnSteps`/`ActivityCard`
   `React.memo`; `AssistantTurn` içinde `parseSteps` artık `useMemo` (yüzlerce KB'lık
   JSON.parse her delta'da koşuyordu) → `TurnSteps`'e referansı sabit dizi gider.
   Memo'nun tutması için `MessageList` gelen handler'ları
   `shared/lib/useStableCallback.ts` ile kimliği sabit hale getirir (ebeveyn inline
   arrow verse bile). Handler yoksa `undefined` kalır — çağıranlar `!!onRetry` ile
   affordance'a karar veriyor.

### Sohbet bileşenleri (`components/chat/`)
- `TurnSteps.tsx` — bir turun iz listesini sırayla çizer; `parseSteps` JSON'u
  güvenli çözer, `stepTruncated` sunucunun kırptığı adımı bildirir.
- `ThinkingBlock.tsx` — model akıl yürütmesi: tool ActivityCard ile **aynı tek-satır
  açılır-kapanır kart** (💭 + "Düşünme" + truncate önizleme + chevron; açınca tam metin,
  dimmed/italik). Varsayılan kapalı.
- `ActivityCard.tsx` — tek tool çağrısı: ikon + etiket + tek satır niyet
  (başlıkta), açınca girdi/çıktı. Edit/Write çıktısı diff olarak. Hata kırmızı.
  **Başlık özeti içerik-odaklı (2026-07-09):** `lib/tools.ts summarize` artık
  ham anahtar-listesi fallback'ini KALDIRDI — yan bilgi ya gerçek içeriği
  (komut/yol/mesaj/başlık/`question`/`reason`/`worker`/`template`) ya da birincil
  dizi alanının değerlerini gösterir (`activate_tools.names`, `todo_write.todos`
  → öğe etiketleri veya "N öğe"; ikincil/filtre dizileri
  `options`/`tags`/`exclude`/`args`/… atlanır); `move_task` → `→ <sütun>`;
  anlamlı bir şey çıkmazsa **boş** kalır (eskiden "names"/"questions" gibi
  anahtar adları yazıyordu). **Tool-özel şablonlar** (`RICH_TEMPLATES`, generic
  mantıktan önce, saf frontend — LLM/token yok): `send_message`/`send_to_worker`/
  `spawn_worker`/`spawn_session` → `aktör → yük`; `create_task`/`update_task` →
  `başlık [sütun]`; `create_schedule`/`update_schedule` → `prompt · cron`;
  `create_hook` → `event → komut`; `create_mcp_server` → `ad → komut`. **`use_skill`
  (2026-07-02):** başlık özeti slug **değerini** gösterir (`slug`/`skill`
  alanları); açınca gövde `<pre>` yerine `Markdown` ile biçimli
  render olur (`# Skill: <slug>` başlığı + md gövde), gereksiz "Girdi" (`{slug}`)
  bloğu skill'de gizlenir.
- `OptimizerChip.tsx` (2026-07-28) — shell adımının çıktısı **modele girmeden önce**
  bir token-optimize edici tarafından kısaltıldıysa başlığın sağında küçük çip:
  `sqz −%93` (hover: `841 → 57 token`), `sqz · yinelenen` (sqz dedup: tüm çıktı
  `§ref:…§` işaretçisiyle değişti — kart tek satır görünür ama komut düzgün çalıştı)
  veya yalnız `rtk` (komutu sarmalar, ölçülebilir öncesi/sonrası yok → yüzde
  gösterilmez). Veri `TurnStep.optimizer`; canlı UI stream'i (`onChunk`) **ham**
  kaldığı için çip yalnız modele giden değerin kısaltıldığını söyler.
  **`rtk özet — ham değil` (uyarı rengi):** yeniden yazılmış komut **başarısız**
  oldu; metin rtk'nın özeti ve gerçek hatayı kaybetmiş olabilir (ölçülen vaka:
  bozuk `go.mod` → "No tests found"). Bu durum diğer her şeyin önüne geçer —
  gizli bir hatanın yanında "−%93" tasarruf yazmak yanlış şeyi kutlamak olur.
  Komut yeniden yazıldıysa hover'da **çalıştırılan gerçek komut** görünür; sessiz
  komut değişimi kabul edilmez. Mekanizma + claude-cli yolu →
  [17-TOKEN-OPTIMIZASYON.md](17-TOKEN-OPTIMIZASYON.md).
- `DiffCard.tsx` — `kind:diff` adımı için özel dosya-değişikliği kartı: ✏️ +
  eylem (Oluştur/Düzenle/Yaz) + tıklanabilir yol + `+N −M` satır sayıları
  (başlıkta), açınca `DiffView` ile birleşik patch. `Write`/`Edit`
  çağrıları `todo_write` gibi generic tool satırı yerine bu kart olur.
  **Yol katlanabilirliği (2026-07-02):** yol artık `shortPath` (…/dir/file) ile
  kısa gösterilir ve tüm satırı kaplamaz; yalnız yol metni dosyayı açar
  (stopPropagation), kalan `flex-1` alan + chevron başlık-butonun parçası kalıp
  diff'i katlar (eskiden `flex-1` tam-genişlik yol satırı katlamayı engelliyordu).
  Tam yol `title` ile hover'da görünür.
- `ChangesButton.tsx` + `ChangesModal.tsx` (2026-08-01) — **toplu dosya-farkı
  görüntüleyici.** Ajan balonunun altındaki aksiyon satırında `⧉ N dosya +A −R`
  çipi; tıklayınca **master-detail** popup açılır: solda değişen dosyalar, sağda
  yalnız seçili dosyanın yaması. Tek uzun scroll yerine master-detail, çünkü o
  zaman aynı anda tek bir patch mount edilir — 20 dosya 1 dosya kadar maliyetli.
  İki sekme: **Bu tur** (turun izinden) ve **Tüm oturum**
  (`GET /api/sessions/{id}/changes`).
  - **Çıkarım tek kaynaktan:** `shared/lib/fileChanges.ts` — `kind:diff` adımı
    (native yol, sunucu `FileDiff`'i kaydetmiş) **veya** başarılı `tool` adımı +
    edit aracı (claude-cli yolu, yama `synthDiffData` ile girdiden sentezlenir).
    Kural birebir `TurnSteps`'in `DiffCard` seçimiyle aynı, yoksa popup'ın dosya
    sayısı görünen kartlarla çelişirdi. Hatalı adımlar atlanır (diske hiçbir şey
    yazılmadı); `subSteps` içine inilir (alt-ajan düzenlemeleri de gerçek).
  - **Kırpma farkındalığı:** transkript yolu yamaları `stepFieldCap`'e kırpar.
    Herhangi bir değişiklik kırpılmışsa çipte `≥` görünür ve popup açılışta o
    turun **tam izini** çeker (`getMessageSteps`). Sentezlenen yamalarda kırpılan
    şey **girdi** olduğu için satır sayıları da eksik kalır — bu yüzden
    `inputTruncated` de "kırpılmış" sayılır.
  - **Aynı dosyaya çoklu edit** grup içinde sıralı listelenir, **birleştirilmiş
    yama üretilmez**: TionHarness dosyanın öncesi/sonrası içeriğini saklamaz, tek
    tek edit farklarını saklar → gerçek birleşim hesaplanamaz, uydurmak yerine
    "sıralı değişiklikler" denir.
  - Yama başına `Kopyala` / `.patch indir` / `dosyayı aç`; altta sabit not:
    gösterilen fark değişiklik anındaki halidir, dosyanın şu anki içeriği değil.
- **Büyük dosya stratejisi** (`DiffView` `variant="panel"`) — sırayla:
  1. **Bağlam katlama** (`foldableRanges`/`diffRows`, `shared/lib/diff.ts`): 6+
     satırlık değişmemiş bloklar iki yanda 3 satır bağlam bırakılarak tek
     `⋯ N değişmeyen satır` satırına iner. İşin kendisini küçülten tek adım bu,
     o yüzden ilk sırada.
  2. **Satır sanallaştırma** (`shared/hooks/useVirtualRows.ts`, 400+ satırda
     devreye girer). Panelde satırlar **sarmaz** (`pre` + yatay kaydırma) —
     sarma satır yüksekliğini değiştirir, sabit yükseklik varsayımı bozulunca
     spacer'lar içerikten kayar.
  3. **Sert tavan:** 20.000 satır üstünde ilk 2.000 satır gösterilir, kalanı
     gerçek sayısıyla birlikte açık bir butonla yüklenir. Sessiz kırpma yok.
  4. **Yeni dosya özel durumu:** `Write`'ın sentezlenen "farkı" dosyanın tüm
     içeriğidir (binlerce `+` satırı, bilgi değeri düşük) → varsayılan kapalı,
     `Yeni dosya · N satır` rozetinin arkasında. Düzenlemeler açık başlar.
- `PathText.tsx` — düz metindeki dosya yollarını tıklanabilir çiplere, **URL'leri
  de gerçek `<a target="_blank">` linkine** çevirir (`lib/paths.ts` tespit eder).
  `splitPaths` artık `{ text, kind: 'text' | 'path' | 'url' }` segmentleri döner;
  URL deseni yol deseninden **önce** denenir — aksi hâlde `https://host/x` içindeki
  `//host/x` kuyruğu "yol" sanılıp şeması kesiliyor ve WebSearch/WebFetch adımının
  yanındaki link tıklanamaz sahte bir dosya çipine dönüşüyordu (TSK423). Cümle
  sonu noktalama (`.,;:!?`) ve dengesiz kapanış parantezi linkin dışında bırakılır;
  şemasız `www.host` linkleri `urlHref` ile `https://` alır. Tırnaklı yollar
  boşluklarıyla birlikte; tırnaksız Windows yolları ise açık `:satır[:sütun]`
  sonlandırıcısı bulunduğunda tek segment olarak linklenir. Aynı `splitPaths`
  parser'ı Markdown metin düğümlerinde de kullanılır; kod/link/görsel düğümlerine
  yeniden ayrıştırma uygulanmaz.
- `lib/paths.ts` → `isExternalUrl` tek kaynaktır; `Markdown.tsx` ve `Gallery.tsx`
  kendi kopya `isExternal` yardımcılarını kullanmaz. `mediaUrl` uzak (http/https)
  bir görsel adresini artık `/api/files?path=…` ile sarmalamaz, olduğu gibi geçirir.
- `WorkerWaitBanner.tsx` (2026-07-27) — koordinatör oturumunda **çalışan worker**
  varken composer'ın üstünde bekleme banner'ı (`WakeWaitBanner` deseni): "N worker
  çalışıyor — sonuçları bekleniyor · M/T bitti" + her worker için oturumunu açan çip.
  Koordinatör turu bitip ilk `<task-notification>` düşene kadar sohbetin bitmiş
  görünmesini engeller. Her çipte **canlı geçen süre** (`WorkerInfo.startedAt` + 1sn
  tick; start zamanı bilinmiyorsa süre gizlenir).
  **Ajan kimliği çipleri (2026-08-29):** çip artık düz metin değil, uygulamanın ortak
  `AgentIdentity` bileşeni (`size="sm"`, `showId`, `subtitle="model"`, geçen süre
  `trailing`'de) — avatar + ajan adı + ajan id'si + çözülmüş model etiketi. Veriyi
  `GET /api/sessions/{id}/workers` taşır (`agentId`, `agentAvatar`, `agentColor`,
  `agentProvider`, `agentModel`, `agentDeleted`; `WorkerInfo`'nun yeni alanları).
  Ajan satırı artık yoksa alanlar boş kalır ve çip `title`/`sessionId` fallback'iyle
  ad gösterir. Çip iki satır olduğu için `max-w-[240px]` ile sınırlanır ve geniş
  fan-out satırlara sarar. Aynı kimlik çipi sağ paneldeki koordinatör roster'ında da
  kullanılır (bkz. `_Docs/47`); eşleme `shared/lib/workerAgent.ts` içinde paylaşılır.
- **Composer üstü yüzen kartlar (2026-08-01):** composer'ın üstündeki **yedi** panel
  (`TodoPanel`, `PendingTray`, `WorkerWaitBanner`, `WakeWaitBanner`, `AskPrompt`,
  `PermissionPrompt`, `PlanPrompt`) geometriyi **tek sarmalayıcıdan** alır:
  `ComposerCard.tsx` — şeffaf kapsayıcı (`-mb-2 px-3 pt-2 md:px-6`) + `rounded-2xl
  rounded-b-lg` + `shadow-xl`; opak gri şerit yok, kartlar transkriptin üstünde yüzer ve
  composer balonuna yaslanır. Çağıran yalnız `tone` (renk) + `className` (kendi iç
  boşluğu/düzeni) verir. Ayırt edici olan **arka plan tonu** (hepsi `--color-surface`
  üzerine `color-mix`, tema-nötr; sınıf metinleri Tailwind tarayıcısı görsün diye
  `TONE` haritasında tam literal):

  | Panel | `tone` | Ton |
  |---|---|---|
  | `TodoPanel` | `plain` | düz `--color-surface` |
  | `PendingTray` | `muted` | `--color-text-dim` %10 |
  | `WorkerWaitBanner` | `worker` | `--color-accent-soft` |
  | `AskPrompt` | `ask` | `--color-accent` %8 |
  | `WakeWaitBanner` | `wake` | `--color-warning` %12 (ikon/başlık da warning) |
  | `PermissionPrompt` | `permission` | `--color-warning` %18 (onay kapısı daha acil okunsun) |
  | `PlanPrompt` | `plan` | `--color-success` %10 | Veri `useRunningWorkers.ts`
  (`GET /api/sessions/{id}/workers`, yalnız `role==='coordinator'`) — **poll yok**,
  tazeleme `worker` SSE event'i ile: `useAppEvents` → `shared/lib/workerBus.ts`
  (coordinatorId anahtarlı pub/sub; iç içe ağaçta event ayrıca
  `rootCoordinatorId` taşır ve kök anahtarına da fanlanır) → hook. Feed koparsa
  `api.subscribeReconnect` → `onReconnect` resync eder (kopma sırasındaki event'ler
  kalıcı kayıptır). Detay `_Docs/47`.

- **Todo tamamlanma/kapatma (2026-08-29):** `latestTodos` en yeni `todo_write`
  listesini, tamamlandıktan sonra kullanıcı yeni mesaj gönderse de korur. Bitmemiş
  listede kapatma düğmesi yoktur. Tüm maddeler tamamlanınca erişilebilir etiketli X
  görünür; X yalnız paneli kapatır, katla/aç davranışını tetiklemez. Kapatma
  `localStorage` içinde **session ID + todo occurrence ID + liste imzası** ile saklanır:
  reload'da aynı occurrence gizli kalır; aynı session'da aynı içerikle yeni bir
  `todo_write` occurrence'ı eski kapatma kaydından etkilenmeden görünür. Farklı liste ve
  başka oturum da otomatik görünür. Kapatma geçmişi en yeni 100 kayıtla sınırlıdır;
  limit aşılınca en eski kayıtlar atılır. Bozuk veya erişilemeyen storage paneli bozmaz.
  Bu davranış yalnız composer üstü
  `TodoPanel` içindir; transkriptteki inline `TodoCard` değişmez.

### Modüler yapı (büyük dosyaların bölünmesi)
İki büyük dosya tek-sorumluluklu küçük parçalara ayrıldı; davranış birebir korundu.
- `MessageList.tsx` artık yalnız **orkestratör**: scroll-pinleme + tool-izi katlama
  durumu. **Pinlenen soru başlığı = ayrı overlay (2026-07-02 fix):** üstten geçen
  son kullanıcı sorusu artık **flow-içi sticky satır değil**, scroll alanının
  üstünde `pointer-events-none` mutlak-konumlu overlay (`UserBubble ... clamp` +
  yukarıdan-aşağı `--color-bg` gradyanı). Eski yaklaşımda aktif satır hem `sticky`
  hem `line-clamp-2` alıyordu; uzun mesaj pinlenince yüksekliği düşüp `scrollHeight`'i
  değiştiriyor → scroll kayıyor → eşik tekrar geçiliyor → clamp↔unclamp titreşimi
  (flicker + aşağı kaydırma zorluğu). Overlay flow yüksekliğini hiç değiştirmediği
  için döngü kırıldı; satırlar tam-yükseklikte kalır. **Overlay flash fix
  (2026-07-10):** scroll-dibe-sabitle + `updateActivePinned` efekti `useEffect`
  yerine **`useLayoutEffect`** ile boyama ÖNCESİ çalışır; eski `useEffect`'te her
  streaming/tool-adımı `messages` değişimi bir kare **eski scroll konumu + eski
  pinned index** ile boyanıp overlay'i (tam-genişlik gradyanlı sticky soru) tek
  kare flash'lıyordu. Pre-paint çalışınca scroll ile overlay görünürlüğü aynı
  commit'e bağlanır → ara tutarsız kare yok.
  **Overlay tıklanabilir (2026-07-28):** sarmalayıcı `pointer-events-none` kalır
  (gradyan üzerinden scroll geçmeye devam eder), yalnız balon `pointer-events-auto`
  `role="button"` olur → tıklayınca o soruya döner (`scrollRowIntoView`, satırı üstten
  **8px aşağıya** oturtur ki sticky başlık kendi kopyasını örtmesin; + flash vurgusu).
  Overlay scroll konteynerinin DIŞINDA olduğu için balon üzerindeki wheel'in
  kaydıracağı ata yok → `onWheel` deltayı konteynere elle iletir.
  **Gönderimde dibe in (2026-07-28):** `scrollBottomSignal` prop'u — `ChatView`
  composer submit'inde (send **ve** queue) bump eder, `MessageList` anında dibe iner.
  Gerekli, çünkü gönderim yalnız **kuyruğa alır**: mesaj backend worker onu alınca
  boyanır, o yüzden yukarı kaydırmış kullanıcı aksi halde hiçbir tepki görmez.
  Mevcut "yeni kullanıcı turu gelince yeniden pinle" mantığı korunur.
  **Bekleme göstergesi (2026-07-28):** bağımsız `WorkingDots` balonu artık
  `(pending || streaming) && son mesaj kullanıcının` ile çıkar. Çalışan bir oturum
  açıldığında turun başlangıcı ile ilk asistan karesi arasında yalnız streaming
  mandalı set oluyordu → transkript kendi mesajımızda bitip ajan susmuş gibi
  görünüyordu. Ayrıca `App` oturum açılışında `GET /api/sessions/active`'ten o
  oturumu **markPending** ile tohumlar (yalnız EKLER; kuyruğa yeni atılmış tur
  sunucuda henüz kayıtlı olmayabilir, budamak göstergeyi söndürürdü).
  Her satırı şu bileşenlere devreder:
  - `UserTurn.tsx` — gerçek kullanıcı mesajı (balon + sağ meta satırı).
  - `PeerTurn.tsx` — **gelen peer/inbox mesajı** (2026-07-06): başka bir ajanın
    yazdığı, `Role:"user"` ama `authorKind:"agent"` (+`authorId`) ile damgalı mesaj.
    Kullanıcının sağ-hizalı balonu yerine **sola-hizalı, gönderen ajanın avatar+adı**
    ile (AgentHeader) çizilir; altında `DirectionBadge` ile "→ alıcı" ve zaman/silme
    meta'sı gösterilir. `MessageList` `isPeer(m)` ile tespit edip bu bileşene devreder;
    peer satırları "typed user" pin/overlay'den de dışlanır. Gönderen roster'da yoksa
    ham `authorId` fallback olarak yazılır (atıf sessizce kaybolmaz).
    **Gönderici balon rengi (2026-08-28):** balon artık asistan balonuyla aynı
    `surface-2` karışımını değil, kendi tema token'larını kullanır —
    `--color-sender-bubble` (dolgu), `--color-sender-bubble-border` (kenar),
    `--color-on-sender-bubble` (metin). Böylece "bu mesajı başkası gönderdi"
    bir bakışta ayrışır; kimin gönderdiği ise AgentHeader'daki ajana özel
    avatar renginden okunur. Token'lar üç yerde birlikte tutulur:
    `frontend/src/index.css` (dark `@theme` + `[data-theme='light']`),
    `frontend/src/shared/lib/themePresets.ts` (`DARK_NEUTRALS`/`LIGHT_NEUTRALS`,
    `theme.ts` ile CSS değişkenine yazılır) ve site aynası
    `website/src/styles/theme.css` + `website/src/content/themes.ts`.
    `themePresets.test.ts` her preset için token'ların dolu olduğunu ve
    dolgu↔metin kontrastının WCAG AA (≥ 4.5) kaldığını doğrular.
  - `AutoPromptNote.tsx` — `Message.origin` dolu olduğunda (`wake`/`schedule`)
    ortalanmış "⏰ Otomatik devam / Zamanlanmış görev" notu (kullanıcı balonu değil).
  - `AssistantTurn.tsx` — asistan turu (başlık + akıl yürütme + iz + cevap + meta).
  - `AgentHeader.tsx` (avatar+ad, 2 yerde paylaşılır), `WorkingDots.tsx`,
    `DeleteButton.tsx` — paylaşılan küçük parçalar.
- `Composer.tsx` artık yalnız input state + olay kablolaması. Yardımcılar
  `components/chat/composer/` altında:
  - `trigger.ts` — `detectTrigger` + `buildMenuItems` (saf mantık), `Trigger`/`MenuItem` tipleri.
  - `AutocompleteMenu.tsx` — `@`/`#`/`/` açılır menüsü.
  - `ComposerPicker.tsx` — düşünme + izin seçicisini birleştiren **tek generic** picker;
    seçenekler `pickerOptions.ts` (`THINKING_OPTIONS`/`PERMISSION_OPTIONS`).
  - `SendActions.tsx` — Gönder/Durdur/Sıraya/Kes/Yönlendir buton kümesi (tur yaşam
    döngüsüne göre tek dal seçer); stil sabitleri `buttonStyles.ts`.
  - **Araç müfettişi (`ToolAccessPanel.tsx` + `ToolAccessList.tsx` +
    `toolAccessGroups.ts`):** toolbar'daki 🔧 butonu seçili ajanın **şu an**
    kullanabildiği araçları **salt bilgi** olarak gösterir — üç sekme: *Bağlamda*
    (şeması her tur gönderilen eager set), *Talep üzerine* (katalogda isim/özet
    duran, `tool_search`/`activate_tools` ile açılabilen lazy set) ve *MCP*
    (tanımlı sunucular: etkin mi, transport/kapsam, canlı bağlantı sayısı, o
    sunucudan gelen aktif/hazır araç adedi). İki araç sekmesi de tek düz,
    alfabetik listedir — built-in ve MCP araçları ayrı gruplara bölünmez, kaynak
    satırdaki rozette durur; her satırda ayrıca görünürlük tier rozeti vardır.
    Sekme başlıklarında sayı rozeti; boş sekme kendi boş-durum metnini, sonuçsuz
    arama ayrı bir metni gösterir.
    Kaynak `GET /api/agents/{id}/tool-access` (ajan-kapsamlı, hiçbir şeyi
    değiştirmez); ayar değişikliği yine Araçlar ekranından yapılır. Panel akış
    sürerken de açılabilir, ajan seçimi değişince `key={agentId}` ile remount olur.
    **Kapanış:** Esc, ✕ **ve boşluğa tıklama** (mousedown; `data-tool-access-toggle`
    taşıyan 🔧 butonu hariç tutulur — yoksa mousedown kapatır, butonun click'i anında
    geri açardı). **Bağlam durumu:** MCP sekmesinde her sunucu (özel/custom dâhil)
    bir **verdict rozeti** taşır — `bağlamda` · `katalog dışı` (araçlarının hepsi
    Gizli tier) · `kapalı` (workspace'te devre dışı) · `ajanda MCP kapalı` ·
    `araç yok` (bağlanamamış / tümü yasaklı) — yanında aktif/katalog/gizli araç
    sayıları (gizli rozetin tooltip'inde kabaca kaç token tasarruf edildiği).
    Araç satırlarında `inContext` alanı aynı ayrımı taşır. **Sunucu satırı açılır**
    (`tool-access-server-toggle`): genişletilince o harici MCP'nin **alt araçları**
    namespace'siz ad + tier rozeti + açıklama ile listelenir (önce eager, sonra lazy,
    her kova alfabetik — saf yardımcı `toolsForServer`). Arama kutusu MCP sekmesinde de
    çalışır. Hiç araç yoksa kutu boş bırakılmaz: sunucunun verdict tooltip'i (kapalı /
    ajanda MCP kapalı / araç yok) satır olarak basılır. **Sözcük seçimi bilinçli:**
    Gizli tier "bağlam dışı" DEĞİL — promptta "N araç daha var, `tool_search` ile bul"
    işaretçisi durur; kaybolan tek şey isim listesi (detay `19`).
  - **Sesli girdi (`MicButton.tsx` + `useSpeechToText.ts` + `sttLanguages.ts`):**
    tarayıcı **Web Speech API** ile dikte. Toolbar'da yalnız **mikrofon toggle**
    (dropdown YOK — sadeleşti). Tanıma dili artık **Ayarlar ▸ Ses ▸ Sesli giriş (STT)
    ▸ Mikrofon dili**'nden seçilir (`SttSettings.tsx` → `setSttLang`; çok-dilli liste
    `sttLanguages.ts`, `tr-TR` varsayılan, `localStorage`'a kalıcı). Seçim same-window
    custom event (`onSttLangChange`) ile mikrofona canlı yansır; **aktif dil butonun
    tooltip'inde** görünür (`sttLangLabel`). `useSpeechToText` hook'u tanıma
    oturumunu sürer: **final** parçalar `appendTranscript` ile drafta eklenir (trigger
    tespiti + typing sinyali tetiklenir), **interim** metin mikrofonun üstünde canlı
    önizleme. Başlat/durdur'da kısa **blip** sesi (merkezi `shared/lib/sounds.ts`,
    Web Audio ile sentez — dosya yok; `listening` geçişine bağlı → kendi kapanışta
    da çalar). API yoksa (çoğu WebView2 masaüstü build'i, Firefox) `supported=false`
    → kümenin tamamı gizlenir.
- **Ajan yanıtı bitiş sesi + bildirimi:** chat turu tamamlanınca (`useAppEvents.ts`
  `chat` completion dalı, wake fazları hariç) `playTurnDone()` chime'ı çalar ve
  pencere arka plandaysa tıklayınca oturuma deep-link eden OS bildirimi gösterilir.
  Tüm UI sesleri **tek cihaz-yerel tercihe** bağlı (`shared/lib/sounds.ts`
  `soundEffectsEnabled`) → Ayarlar ▸ **Ses** ▸ **Ses efektleri** toggle'ı
  (`SoundPanel`); açınca örnek chime çalar.
- **Yanıtı sesli okuma (TTS, `shared/lib/tts.ts`):** tarayıcı `speechSynthesis` ile
  ajan yanıtını sesli okur — **güvenli bağlam/izin gerektirmez** (mikrofonun aksine
  HTTP/LAN'da da çalışır). `stripForSpeech` yalnız düz metni bırakır: fenced/inline
  kod (fenced **bloklar**), datatable/mermaid/html-preview blokları, tablolar, görsel/link,
  çıplak URL ve Windows dosya yolları ayıklanır. **Inline kod** (tek-backtick) ise cümlenin
  parçası olduğu için **okunur** — sadece backtick'ler atılır, içindeki kelime kalır.
  **Manuel:** her asistan balonunda 🔊
  buton (`AssistantTurn`, play/stop). **Otomatik:** tur bitince aktif oturumda okur
  (`useAppEvents` completion dalı → `speakLatestReply`; id-dedupe + interrupted/cancelled
  atlanır). Dil `ttsLang()` → açık TTS seçimi, yoksa composer ses dili (`stt.lang`),
  yoksa motor varsayılanı. **Ayarlar ▸ Ses** (yeni özel alt-sayfa `SoundPanel` —
  ses efektleri + STT + TTS bir arada; NotificationsPanel'den ayrıldı)
  + `TtsSettings.tsx`): "Yanıtları sesli oku" toggle'ı (cihaz-yerel, varsayılan kapalı)
  + **ses seçimi** (`speechSynthesis.getVoices()`, `voiceschanged` ile tazelenir,
  voiceURI'ye göre) + **hız** (0.5–2×) + **ton** (0–2) slider'ları + "Sesi dene" butonu.
  Tüm bu tercihler hem otomatik okuma hem balon 🔊 butonunu etkiler; `speak` rate/pitch/
  seçili sesi uygular (`resolveVoice`: voiceURI › dile göre eşleşen ses).
- **Uzun metin okuma (tarayıcı motoru):** Chrome/Edge `speechSynthesis` uzun
  utterance'ı ~15sn/birkaç yüz karakterde cümle ortasında keser + sekme blur'unda
  stall eder. `speak` metni **cümlelere bölüp** (`splitForSpeech`, ≤180 char) zincirleme
  kuyrukta seslendirir + 10sn'de bir `pause()/resume()` keep-alive. Cümle sınırı =
  **ardından boşluk gelen** `.!?…` → `file.ts`, `127.0.0.1`, `3.14`, `v1.2.0`, `Node.js`
  gibi kod/sayı token'ları bölünmez. `stopSpeaking` kuyruğu temizler. (Sunucu Piper
  motorunda bu sorunlar yok — tek kesintisiz WAV.)
- **Global ses seviyesi slider'ı (`TtsVolumeSlider.tsx`):** her asistan balonundaki 🔊
  butonunun yanında kompakt bir volume slider'ı. **Tek global değer** (`ttsVolume`/
  `setTtsVolume`, 0–1) — biri değişince `onTtsVolumeChange` yayınıyla **tüm balonların
  slider'ları + Ayarlar ▸ Ses'teki slider** anında güncellenir. Volume iki motora da
  uygulanır (`speechSynthesis` `utterance.volume`; sunucu `<audio>.volume`, çalan ses
  canlı güncellenir).
- **Sunucu TTS motoru (Piper, harici CLI):** tarayıcı sesleri yerine sunucuda üretilen
  **doğal Piper** sesi — böylece **telefon/thin client** da okur (sesi sunucu üretir,
  cihaz sadece çalar). Backend `internal/tts` (piper.exe tespiti: `TIONHARNESS_PIPER` env
  › `Progs\piper` layout › PATH; `voices/*.onnx` tarar; `os/exec`+60s timeout, `--model`
  + stdin metin → WAV). **Kurulum şekli değişti (2026-08-20):** upstream
  (`OHF-Voice/piper1-gpl`) Windows'a standalone arşiv yayınlamayı bıraktı, yerine
  Python wheel veriyor → kurulum artık `Progs\piper\.venv` ve aranan ilk aday
  `.venv\Scripts\piper.exe` (eski standalone layout listede kaldı, bozulmaz).
  CLI sözleşmesi **aynı**: piper1-gpl `--model`/`--output_file` alt-çizgili
  yazımları takma ad olarak koruyor, bu yüzden `Synthesize` sürüme göre
  dallanmıyor. Ses modelleri venv dışında (`Progs\piper\voices`) durduğu için
  `voiceDirs` iki seviye yukarıyı da tarar + `internal/api/tts.go` (`GET /api/tts/status`, `POST /api/tts`
  → `audio/wav`). Frontend `api/tts.ts` + `shared/lib/tts.ts` motor katmanı: `resolveEngine`
  (`auto`/`browser`/`server`; auto Piper varsa onu), server yolunda `/api/tts` → paylaşımlı
  `<audio>`; hata/yoksa **browser speechSynthesis'e düşer**. **Mobil autoplay:** ilk
  jestte sessiz-WAV ile `initTtsUnlock`, boot'ta `initServerTts` (App.tsx). Ayarlar'da
  motor seçici (Segmented) + sunucu ses listesi (`TtsSettings`). Piper yoksa hiçbir şey
  değişmez. **Kurulum-bağımsız** (tek-binary'e gömülü değil).
- **Sunucu STT motoru (whisper.cpp, harici CLI):** tarayıcı Web Speech yerine sunucuda
  transkripsiyon — offline, Türkçe, WebView2/thin client'ta da çalışır. Backend
  `internal/stt` (whisper-cli + **ffmpeg** tespiti: env › `Progs\whisper` layout › PATH;
  `models/ggml-*.bin` tarar; `Transcribe`: ffmpeg ile ses→16kHz mono WAV → `whisper-cli
  -otxt` → metin; 120s timeout) + `internal/api/stt.go` (`GET /api/stt/status`,
  `POST /api/stt?lang=&model=` raw audio → `{text}`). Frontend `api/stt.ts` +
  `shared/lib/stt.ts` (`resolveSttEngine` auto/browser/server) + `useServerStt.ts`
  (`MediaRecorder` → kayıt → stop'ta yükle → transcribe; ara sonuç YOK, "yazıya
  çevriliyor…" spinner'ı). `MicButton` iki motoru da bağlar, resolver'a göre yönlendirir;
  server yoksa/başarısızsa **Web Speech'e düşer**. Boot'ta `initServerStt` (App.tsx).
  Ayarlar ▸ Ses'te motor seçici + whisper model listesi (`SttSettings`). **Uyarı:**
  `getUserMedia` yine güvenli bağlam ister → LAN-IP+HTTP telefonda mikrofon bloklu
  (localhost/HTTPS gerekir); sunucu motoru bu sınırı kaldırmaz, sadece tanımayı yerelleştirir.
- **Onay bekleyen tool sesi + bildirimi (2026-07-23):** Tur kullanıcıya **bloke**
  olduğunda (`ask_user` sorusu, izin onayı, plan onayı) dikkat çekilir:
  `chatStreamHub.ts` `openInteraction` (hub `interaction_open`) `playAskPrompt()`
  çalar ve pencere arka plandaysa OS bildirimi gösterir (`ask:{sid}:{id}` tag'i ile
  çoklu pencerede tek toast). Ses `playTurnDone`'dan **bilerek farklı**: yükselip
  geri düşen "soru" şekli — "bitti" ile karışmasın. Ses her durumda çalar (pencere
  önde olsa da; kullanıcı başka yere bakıyor olabilir), OS toast'ı ise `notify()`
  gereği yalnız arka planda. Aynı prompt **bir kez** duyurulur: interaction id'leri
  `cuedInteractions` setinde tutulur (hub replay / oturum değişimi / reconnect
  tekrar çalmaz), `interaction_resolved` ile set'ten düşer. Gate'ler: ses →
  `soundEffectsEnabled`, toast → `settings.desktopNotifications` + tarayıcı izni.
  **Sınır:** `openInteraction` sunucuda yalnız oturum hub'ına yayınlanır
  (`publishHub`, global bus event'i YOK) → ipucu sadece **aktif oturum** için
  çalışır. Ekranda olmayan bir oturumdaki `ask_user` sessizdir; kapsamak için
  backend'in interaction'ı process-wide bus'a da yayınlaması gerekir.
- `hooks/useOutsideClick.ts` — dışarı-tıklama efekti tek hook'a çıkarıldı ve **7
  bileşende** (Composer pickerları, WorkDirBadge, AgentPicker, FolderPickerButton,
  WorkspaceSwitcher, SessionsSidebar, EmojiPicker) tekrar yerine kullanıldı.

### Yardımcılar (`lib/`)
- `tools.ts` — tool adı → ikon/etiket/özet/`isDiff` meta verisi (MCP namespace'i
  `server · tool` olarak ayrıştırılır).
- `paths.ts` — dosya yolu tespiti, görsel `mediaUrl`, yol kısaltma.
- `diff.ts` — unified diff ayrıştırma + +/- istatistik.

### Entegrasyon
- `MessageList.tsx` — kullanıcı balonu sağda; asistan turu: `ThinkingBlock`
  (varsa reasoning) → `TurnSteps` → markdown cevap.
- `App.tsx` — `onOpenFile`: görseli yeni sekmede açar (`/api/files`), diğer
  yolları panoya kopyalar.
- `index.css` — `.sg-markdown` tipografisi + `github-dark` highlight teması.

### Composer — ajan seçici / `@` referans / `#` artifact / `/` komut menüleri
- **Ajan seçimi (`composer/AgentSelect.tsx`):** mesajın gönderileceği ajan **her zaman**
  dropdown'dan seçilir (zorunlu) — **`@` ile yönlendirme YOK**. Textarea'nın solunda
  avatar+ad gösteren, yukarı açılan seçici. Seçim oturuma kalıcı yazılır
  (`PUT /api/sessions/{id}/agent` → `db.SetSessionAgent`) ve sonraki her tur o ajana
  gider. Ajan seçili değilse **Gönder kilitli** (seçici kırmızı kenarlık).
- `Composer.tsx` otomatik-tamamlama menüsü (`composer/AutocompleteMenu.tsx`): caret
  konumuna göre `detectTrigger` (`@`, `#`, `/`).
  - **`@`** (token başında) → **ajan adı referansı**; seçim metne `@Ad` ekler ve
    `UserBubble`'da çip olarak vurgulanır. **Bu yalnız bir isim referansıdır — turu o
    ajana YÖNLENDİRMEZ** (alıcı yine dropdown ajanıdır). `@Ad` düz metin olarak mesajla
    gider; alıcı ajan, sistem-prompt notu sayesinde bunu "başka ajana yapılan isim
    referansı" (handoff/çağırma değil) olarak yorumlar ve gerekirse yanıtında ona
    hitap edebilir/iletebilir — ama otomatik bağlantı yoktur (`chat_turn.go` notu).
  - **`#`** (token başında) → **artifact seçici**; **workspace'teki tüm artifact'lar**
    listelenir (yalnız bu oturumunkiler değil — `App.tsx` `listArtifacts()`'i sessionId'siz
    çağırır, aksi halde elle/başka-oturumda oluşturulan sessionId'siz artifact'lar hiç
    görünmezdi). Seçim `#sorgu` token'ını siler ve artifact'ı içerik eki olarak ekler
    (içerik satır-içine alınır, `ARTIFACT_INLINE_CAP`).
  - **`/`** (girdinin başında, tek kelime) → **komut paleti**; seçim `SlashCommand.run()`,
    girdi temizlenir. Komutlar `features/chat/chatStreamCommands.ts`'teki
    `buildChatCommands()`'ten gelir: `/compact` (sohbeti şimdi özete sıkıştır),
    `/refresh-context` (donmuş bağlam snapshot'ını yenile), `/handoff` (context
    reset), `/rewind` (checkpoint'e geri sar), `/tools` · `/board` · `/flows`
    (talep-üzerine özet, `POST /api/sessions/{id}/summary`). Özet komutları çalışınca **komutun kendisi de**
    sohbete bir kullanıcı balonu (`/kind`, komut stilinde) olarak yazılır, ardından sonuç
    asistan mesajı gelir (ikisi de kalıcı; `{userMessage, replyMessage}`). Bir komutu
    **çalıştırmadan düz metin** göndermek için tırnak içine al: `"/komut"` (bkz. `chat/UserBubble.tsx`).
  - Klavye: ↑/↓ gezinme, Enter/Tab seçim, Esc kapat (menü açıkken Enter göndermez).
- `SlashCommand` tipi `types.ts`'te (`name`/`description`/`icon`/`run`).
- **Düşünme seviyesi seçici (`ThinkingPicker`):** textarea'nın solunda `🧠` butonu +
  üstte açılan menü (**Oto**=ajan ayarı / **Kapalı** / **Düşük** / **Orta** / **Yüksek**,
  dışarı-tıkla-kapat). Seçim `App.tsx` `thinkingLevel` state'inde + `localStorage`
  (`tionharness.thinkingLevel`) ile kalıcı; `chatStream` gövdesine `thinkingLevel` olarak gider
  ve o turun reasoning bütçesini **ajan ayarından bağımsız** belirler (bkz. Notlar).

### Session-bazlı sohbet (varsayılan ajan + çok-katılımcılı thread)
> Bir oturum **çok-katılımcılı** olabilir (aşağıdaki "Generic participant modeli"):
> `AgentID` varsayılan yanıtlayıcıdır, dropdown'dan başka bir ajan seçmek turu ona
> yönlendirir ve o ajanı thread'e katar. "Her seferinde tek ajan" akışı budur.
- Sohbet **session-bazlı**: sol panel (`SessionsSidebar.tsx`) oturumları **zaman
  kovalarına** gruplar (Bugün/Dün/Geçen hafta/Geçen ay/Daha eski; ajan altında gruplama
  yok), `updatedAt` desc; her oturum **tek bir ajana bağlıdır** (`Session.AgentID`) ve
  satırda o ajanın avatarı görünür. Ajan roster'ı ayrı **Ajanlar** view'inde
  (`AgentRoster`/`AgentsView`) = yeni sohbetlerin varsayılan ajan seçicisi.
  `POST /api/sessions` `agentId` opsiyonel (boş → ilk ajan).
- **Ajan seçimi dropdown ile (zorunlu):** composer'daki `AgentSelect` oturumun ajanını
  gösterir; değiştirince `PUT /api/sessions/{id}/agent` ile kalıcı olur ve `activeAgentId`
  + sessions listesi güncellenir. `sendMessage` her zaman **oturumun ajanını** tek
  elemanlı `agentIds=[sessAgent]` olarak gönderir. `POST /api/chat/stream` `agentIds` alır.
  - **`@` = isim referansı, yönlendirme DEĞİL.** Composer `@` menüsü metne `@Ad`
    ekler ve `UserBubble` çip olarak vurgular; `useChatStream` bunu **parse etmez** —
    mesaj yine yalnız oturumun ajanına gider (`agentIds=[sessAgent]`). `@Ad` düz
    metindir; alıcı ajan `chat_turn.go` sistem-prompt notuyla onu "başka ajana isim
    referansı (handoff/çağırma değil)" olarak yorumlar. Bir tur tek ajana gider.
    Backend `agentIds`'i hâlâ dizi olarak kabul eder (geriye dönük uyumlu); yeni boş
    oturumda ilk ajan `adoptMentionedAgent` ile oturuma yazılır.
- Her asistan turu `Message.AgentID` ile kalıcılaşır; `MessageList` her turu **kendi
  ajanının avatar+adıyla** çizer.
- **Çok-ajanlı geçmişte yazar etiketleme (2026-06-23):** Birden fazla ajanın yanıt
  verdiği bir oturumda, geçmiş provider'a aktarılırken her asistan turu **yazarının
  adıyla** ön-eklenir (`api/chat_authors.go` → `labelMultiAgentHistory`): başka ajanın
  turu `"[Ada]: …"`, yanıtlayan ajanın kendi eski turları `"[Kai (you)]: …"`. Önceden
  `toProviderMessages` yalnız `Role`+`Text` taşıyıp `AgentID`'yi düşürüyordu → tüm
  asistan turları tek ayrımsız "assistant" sesine karışıyor, ajan **kimin ne dediğini
  göremiyordu** (hatta diğer ajanın sözlerini kendi sanıyordu). Etiketleme yalnız
  **2+ farklı yazar** varken devreye girer (1:1 sohbet ve prompt cache etkilenmez),
  **kopya** üzerinde çalışır (kayıt değişmez) ve compaction özetine de yansır. Çok-yazarlı
  oturumda `chat_turn.go` sistem promptuna kısa bir not ekler (`multiAgentHistoryNote`):
  köşeli-parantez etiketlerinin okuma amaçlı olduğunu ve ajanın **kendi yanıtını
  ön-eksiz** yazması gerektiğini açıklar. Test: `chat_authors_test.go`.
- **Kullanıcı mesajının hedef ajanı (2026-06-23):** Kullanıcı mesajı kaydedilirken
  yönlendirildiği ajan `Message.AgentID`'ye damgalanır (`chat_stream.go`/`chat.go` →
  `agents[0]`); metindeki `@Ad` yalnız bilgi amaçlıdır, yönlendirme yapmaz. Çok-yazarlı
  geçmişte kullanıcı turları `"[User → Ada]: …"` olarak etiketlenir → ajan **hangi
  sorunun kime sorulduğunu** da görür. (1:1 oturumda etiket çıkmaz.)
- **Generic participant modeli (2026-07-06):** Oturum artık **çok-katılımcılı bir
  thread** olarak modellenir: örtük **`user`** (insan, en üst yetkili principal) +
  bir veya daha fazla ajan. Her mesaj `Message.AuthorKind` (`user`|`agent`|`system`)
  + `AuthorID` (ajan id / `user`) + `RecipientID` (ajan id / `*` broadcast / boş =
  thread geneli) taşır (`db/models.go`). Alanlar her yazımda `NormalizeParticipants`
  ile Role+legacy `AgentID`'den türetilir (assistant→author=AgentID, user→
  author=`user` & recipient=AgentID) → **eski `session.jsonl` migrationsuz** okunur.
  `Session.Participants` roster'ı, bir ajan yazdıkça/adres alındıkça büyür; append
  hot-path header'ı tazelemediği için **reload'da mesaj satırlarından yeniden
  hesaplanır** (MessageCount gibi self-healing; `user` ve `*` roster'a yazılmaz).
  `AgentID` **varsayılan yanıtlayıcı** olarak kalır; `Participants` composer'ın
  yönlendirebileceği tam kümedir (`SessionParticipants` legacy boş roster'da
  `[AgentID]`'e düşer). `labelMultiAgentHistory` artık bu alanlardan çalışır:
  `"[Author → Recipient]: …"` (yön yoksa oksuz, broadcast `→ all`), yanıtlayanın
  kendi turu `(you)`. Sistem notu (`multiAgentHistoryNote`) **yetki sırasını** da
  belirtir: `User` insan principal'dir, çelişkide ajan yerine User izlenir. Not:
  provider rol üçlüsü (`system`/`user`/`assistant`) sabit olduğundan katılımcılar
  **rol-flip edilmez**, yalnız metin etiketlenir → 1:1 için prompt-cache korunur.
  Frontend: `Session.participants` tipe eklendi; her balon yazarını zaten
  `m.agentId`'den çizer (`MessageList`). Testler: `db/participants_test.go`,
  `chat_authors_test.go` (directed/broadcast).
- **RecipientID yön rozeti (frontend, 2026-07-06):** `Message` tipine `authorKind`/
  `authorId`/`recipientId` eklendi. `DirectionBadge.tsx` bir turun `recipientId`'sinden
  **"→ &lt;ad&gt;"** ipucu çizer (broadcast `*` → "herkes"; boş = thread geneli, rozet yok).
  `MessageList` yalnız **çok-katılımcılı** thread'de (2+ ayrı ajan yazar/adres) gösterir —
  1:1 sohbet temiz kalır: asistan balonunda `AgentHeader` yanında, user balonunda meta
  satırında. Asistan yanıtları normal sohbette thread-geneli (RecipientID boş); yönlü
  atıf şimdilik **inbox/peer** mesajlarında görünür (`agentmsg.go` katılımcı damgası).
- **Inbox mesajı genericleştirildi (2026-07-06):** `agent/agentmsg.go: deliverOne` inbox
  mesajına `AuthorKind=agent`/`AuthorID=fromAgentID`/`RecipientID=target.ID` damgalar →
  gönderen ajan inbox thread'inin katılımcısı olur, `labelMultiAgentHistory` (inbox turu
  `wake_turn.go` üzerinden bundan geçer) mesajı `[Gönderen → Alıcı (you)]` atfeder.
  Detay `_Docs/47`.
- **Ajan yazdığı her enjekte tur atfedilir (TSK507):** aynı katılımcı damgası artık
  yalnız peer teslimlerinde değil, **bir ajanın yazdığı her enjekte user-turunda**
  uygulanır — spawn açılış promptu (`spawn.go`, yazar = `SpawnOptions.CreatedBy`) ve
  koordinatörün worker'a follow-up'ı (`coordination.go: dispatchWorkerTurn`, yazar =
  koordinatör oturumunun ajanı). Ortak yardımcı:
  `Runtime.recordAgentAuthoredNote`. Böylece bu mesajlar `MessageList`'te
  `isPeer` → **`PeerTurn`** (soldan gelen balon) olarak çizilir; öncesinde insanın
  kendi turu gibi görünüyorlardı.
  **Atıf yalnız gerçek bir ajana çözülürse damgalanır:** `CreatedBy` ajan olmayan
  köken de taşır (`"automation:<id>"`), frontend ise `authorId`'yi ajan listesinde
  arayıp isim yazar — çözülemeyen id isimsiz bir peer balonu üretirdi. Çözülemeyen
  yazar düz balona düşer (insan spawn'ı da öyle). Testler:
  `TestSpawnSession_AttributesPromptToSpawningAgent`,
  `TestSpawnSession_HumanSpawnStaysUnattributed`,
  `TestSendToWorkerAttributesFollowUpToCoordinator`.
- **Sıradaki-tur bağlam önizleme (debug, 2026-06-23):** Agent ekranındaki bağlam
  önizlemesinin oturum karşılığı. SessionDetailPanel → **"Bağlam (debug)"** →
  `SessionContextModal`, `GET /api/sessions/{id}/context-preview?message=`. Ajanın bu
  oturumda **bir sonraki turda alacağı tam isteği** gösterir: composed sistem promptu +
  dinamik suffix + **modele gidecek mesaj dizisi** (yazar etiketleri + araç özeti folded)
  + şema araç kataloğu, her biri ~token tahminiyle. Opsiyonel örnek "sıradaki mesaj"
  bekleyen kullanıcı turu olarak eklenir. **Yan etkisiz:** `Prepare`'i atlar (compaction/
  özet persist YOK, provider çağrısı YOK) — `Prepared` elle kurulur (`api/session_context.go`).
  **Per-mesaj yazar rozeti (2026-06-23):** her mesaj satırı rolün yanında **hangi ajana
  ait** olduğunu gösterir — asistan turu yazan ajan adı (kendisi ise `(siz)`), kullanıcı
  turu yönlendirildiği ajan (`→ Ad`). Tek-ajan oturumunda da görünür (metinde `[Ad]:`
  prefix'i yokken bile). `previewMessage.author/self` alanları, modele giden user/assistant
  turlarıyla 1:1 hizalı kurulur (`composeTurnRequest` `prep.Messages`'i değiştirmez).
- **Arka-plan süreç kontrolü (SessionDetailPanel, 2026-07-06):** Oturum bilgisi paneli,
  o oturum için **uçuştaki turu** (background provider/claude-cli süreci) ve varsa
  **sıcak persistent-pool sürecini** gösterir + kontrol ettirir. Kaynak `GET /info` yeni
  `running {runId, startedAt, autonomous, provider}` + `warmCliProcess` alanları
  (`chatRuns.sessionRunInfo` + `Runtime.HasWarmCLISession`). Butonlar: **Durdur**
  (`POST /api/chat/control {action:stop}` → `run.cancel()` → subprocess ölür),
  **Yeniden başlat** (chat hook `rerunLast` — backend runId ile durdur, son promptu
  yeniden gönder), **Tazele** (`DELETE /api/sessions/{id}/cli-process` →
  `CLISessionPool.DropSession`, sonraki tur cold-restart, konuşma korunur). Kart, süreç
  canlıyken 3sn'de bir poll eder + geçen-süre sayacı gösterir; detached/otonom turlar için
  de çalışır (runId backend'den gelir, pencere sahipliğine bağlı değil).
- **Oturum bilgisi paneli artık konuşmayı takip ediyor (2026-07-23):** Panel bir
  `refreshKey` (`meterRefresh` nonce) ile yenileniyordu ve kodda "her turdan sonra
  tazelenir" yazıyordu — ama bunu artıran `bumpMeter` **hiçbir yerden çağrılmıyordu**
  (`chatStreamSend.ts`'e prop olarak geçiyor, `performSend` onu destructure bile
  etmiyor). Nonce'u yalnızca `useAppEvents`'teki `session_change` olayları artırıyordu,
  yani **ajan araçlarıyla** metadata değişince (goal/title/tags/workdir…). Normal sohbette
  hiçbir `session_change` yayılmadığı için panel mount anındaki fotoğrafta donuyordu —
  mesaj sayısı, boyut, bağlam kullanımı ve harcama güncellenmiyordu.
  Çözüm: `chatStreamHub.ts` artık `bumpMeter`'ı **transkripte mesaj girdiğinde** çağırıyor:
  `KindUserMessage` (bizim mesajımız) ve `KindTurnDone`/`KindTurnError` (ajanın turu
  bitti, maliyet/boyut kesinleşti). Tur **sonunda** (her `Reply`'de değil) → çok-ajanlı
  bir tur yine **tek** yenileme eder. Panel kapalıyken zaten mount edilmediği için
  ekstra istek doğurmaz. **Yan fayda:** aynı nonce `sessionArtifacts` listesini de
  besliyordu; o da sessizce ölüydü, artık tur sonunda gerçekten tazeleniyor
  (yorumun baştan beri iddia ettiği davranış).
- **Gönderim yolu ölü kablolarından temizlendi (2026-07-23):** Yukarıdaki hatanın
  kök nedeni `SendContext`'ti: Faz 3 kuyruk geçişi `performSend`'i ince bir
  **enqueue**'ya indirdi, ama arayüz eski "turu boyayan" hâlinden kalma 19 alanı
  taşımaya devam etti. `performSend` bunların yalnız **8**'ini destructure ediyordu;
  kalan 11'i (run handle'ları, canlı balon ref'i, transkript setter'ları, navigasyon,
  `bumpMeter`) sessizce ölüydü — ve "bağlı görünen ama çağrılmayan" `bumpMeter` tam
  da panelin donmasına yol açmıştı. `SendContext` gerçekten kullanılan 8 alana
  indirildi; zincirleme olarak `useChatStream`'de `runsRef` ile `setView` de ölü
  kaldı ve silindi, `RunHandle` tipi tamamen kaldırıldı (tek kullanıcısı `runsRef`'ti),
  `ChatStreamDeps`'ten `setView` çıkarıldı (App.tsx çağrı yeri güncellendi).
  `sendMessage`'ın `useCallback` bağımlılık dizisi 12 → 5 girdiye indi.
  Kural: bu arayüz **minimal** kalmalı — kullanılmayan bir prop burada "bağlı"
  görünür ve bir sonraki geliştiriciyi (ve bu bug'da olduğu gibi UI'ı) yanıltır.
- **Başlık kontrolleri hep görünür (2026-07-23):** `SessionTitleBlock`'taki **AI ile
  başlık üret** (✨) ve **Başlığı düzenle** (✏️) butonları `opacity-0
  group-hover:opacity-100` hayaletiydi → satırın üzerine gelmeyen kullanıcı bu iki
  özelliğin varlığını göremiyordu. Hover kapısı kaldırıldı (artık `opacity: 1`), sarmalayıcı
  `group` sınıfı ölü kaldığı için silindi, `aria-label`'lar eklendi. Devre dışı
  durum (`messageCount === 0` / üretim sürerken) `disabled:opacity-30` ile korunuyor.
- **Son turların araç I/O özeti (2026-06-23):** Geçmiş provider'a çevrilirken araç
  çağrı/sonuçları düşüyordu (`toProviderMessages` yalnız metin) → ajan "az önce ne
  yaptın / o komut ne döndü" diye soramıyordu. Artık `Prepare`'den önce son **N=4**
  asistan turunun `Steps` izinden kompakt bir `<recent_tool_activity>` bloğu
  (araç+kısa arg → kırpılmış çıktı; tur başına ≤10 araç, çıktı ≤240 rune) o turun
  metnine **kopya üzerinde** eklenir (`api/chat_tool_summary.go`). Token maliyeti
  son N turla sınırlı. Test: `chat_tool_summary_test.go`.
- **👍/👎 geri bildirimi artık bağlama enjekte ediliyor (2026-07-23):** Puan zaten
  mesajda saklanıyordu (`db.MessageFeedback`, `session.jsonl`'e yazılır) ama **hiçbir
  yer okumuyordu** → 👎'ye basmak bir sonraki cevabı hiç etkilemiyordu (tek dokunan
  yer yazma fonksiyonunun kendisiydi). Okuma tarafı eklendi:
  `api/chat_feedback_summary.go` → `recentFeedbackBlock(history)` kompakt bir
  `<user_feedback>` bloğu üretir ve `composeTurnRequest` bunu **volatile dinamik
  soneke** ekler (tıpkı `recentToolActivityBlock` gibi).
  - **Neden history'ye katlanmıyor:** puan her an eklenebilir/çevrilebilir/temizlenebilir;
    geçmiş bir turun baytlarını değiştirmek **rolling prompt-cache breakpoint'ini
    bozardı**. Dinamik blok breakpoint'ten sonra gittiği için puan vermek cache'i bozmaz.
  - **Sınırlar:** en yeni **6** puanlı tur, cevap alıntısı ≤200 rune, not ≤300 rune.
    Puanlar seyrek ama uzun ömürlü olduğundan tool recap'ten farklı olarak **tüm
    geçmiş** taranır (20 tur önceki bir 👎 hâlâ en değerli sinyal olabilir).
  - `rating: 0` (kullanıcı puanı geri aldı) **sinyal değildir**, atlanır.
  - **İfade:** girdiler "senin cevapların" değil "bu sohbetteki cevaplar" diye
    tanımlanır — çok-ajanlı bir thread'de puanlanan tur bir **başka ajana** ait olabilir,
    ona ait olmayan bir eleştiriyi üstlenmesi yanlış olurdu.
  - Prompt, modele bunu **sessizce uygulamasını** söyler: puanı gündeme getirme,
    teşekkür etme, 👎 için özür dileme. Not varsa alıntıdan **üstündür**.
  - 5 çağıranın hepsine bağlandı: `chat_stream.go` (akış), `chat.go` (bloklayan),
    `chat_btw.go` (yan sohbet), `wake_turn.go` (otomatik uyanma), `session_context.go`
    (bağlam önizlemesi — gerçek turu birebir yansıtması için).
  - Test: `chat_feedback_summary_test.go` (boş/temizlenmiş puan, 👍/👎 + tur-yaşı
    etiketleri, sıralama, üst sınır, kırpma).
- **Ardışık aynı-rol birleştirme (2026-06-23):** Bir kullanıcı mesajına iki ajan
  ardışık yanıt verirse geçmiş `user → assistant → assistant` olur; Anthropic katı
  şekilde rol-değişimi ister ("roles must alternate") → istek reddedilirdi. `providers`
  katmanına `coalescePlainSameRole` eklendi: ardışık aynı-rol **düz metin** turlarını
  tek mesajda birleştirir (araç çağrı/sonucu taşıyan turlara dokunmaz — tool_use↔tool_result
  eşleşmesi korunur). Hem `anthropic.go` hem OpenAI-uyumlu `minimax.go` çeviricilerinde
  uygulanır. Test: `providers/coalesce_test.go`.
- **Oturum-bazlı akış durumu:** akış (streaming) artık **oturuma bağlı** — `App.tsx`
  `streamingSessionId` akışın sahibi oturumu izler. Composer'ın akış aksiyonları
  (Durdur/Kes/Yönlendir) ve `AskPrompt` yalnız `streamingSessionId === activeSessionId`
  iken görünür (başka oturuma geçince normal "Gönder"). `SessionsSidebar` akıştaki oturum
  satırında **nabız atan nokta** (`animate-ping`, **`emerald-400`**) + **"yazıyor…"** etiketi
  gösterir (başlık kalın); gösterge oturum değişse de akıştaki oturumda kalır.

### Boş yeni-sohbet otomatik temizliği (2026-06-23)

"Yeni Sohbet" ile açılan ama **hiç mesaj gönderilmemiş** oturum, kullanıcı ondan
ayrılınca (başka oturum seçince veya bir başka yeni sohbet açınca) otomatik silinir
— boş, terk edilmiş sohbetler birikmesin. `App.tsx`: `freshEmptyRef` yeni-oluşturulan
boş oturumu izler; `discardEmptyFresh(leavingId)` ayrılırken canlı transcript boşsa
(`messagesRef.length === 0`) `api.deleteSession` ile siler. Bir mesaj gönderilmişse
oturum gerçek konuşma sayılır, korunur. (Sidebar'daki yenile butonu silmeyi tetiklemez.)

### Anlık başlık kesiti (2026-07-23)

Yeni bir oturumun title'ı boştur ve arayüzde **"Yeni sohbet"** placeholder'ıyla gösterilir
(`SessionsSidebar` / `AppHeader` / `SessionTitleBlock`). İlk mesaj gönderilince ilk turda
backend LLM ile bir başlık üretir (`maybeAutoTitle` → `Runtime.TitleFor`), ama bu bir model
round-trip'idir; o pencerede başlık hâlâ "Yeni sohbet" kalırdı (ve sağlayıcı hatalıysa hiç
değişmeyebilirdi). Artık mesaj **kuyruğa alınır alınmaz** (LLM'i beklemeden) başlık, promptun
kısa bir **kesitine** set edilir: `handleEnqueueMessage` (`internal/api/inbox.go`), enqueue
`queued==true` döndükten sonra `maybeSnippetTitle` (`internal/api/chat_turn.go`) çağırır. Bu
yardımcı yalnızca title'ı **hâlâ boş** olan oturuma dokunur (mevcut başlığı asla ezmez),
`titleSnippet(message)` ile kesiti üretir (ilk satır → whitespace tek boşluğa → ~48 rune,
rune-güvenli kırpma + `…`), `SetSessionTitle` ile yazar ve `emitSessionChange(..., "title")`
yayar — böylece "Yeni sohbet" SSE üzerinden **anında** kaybolur. Kesit hem kalıcıdır (reload'da
da görünür) hem de ilk-tur LLM auto-title'ı için **fallback**tir: LLM başarısız olursa/boş
dönerse kesit kalır, başarılıysa daha temiz bir başlıkla üzerine yazar (refine). Tamamen
best-effort — hiçbir hata enqueue/reply akışını bozmaz. Frontend'e dokunulmaz; mevcut
`session_change("title")` eventi UI'ı günceller.

### Hata kurtarma ve yeniden deneme

- **Hata adımı:** Bir tur ağ/sağlayıcı hatasıyla sonuçlanırsa `useChatStream` asistan
  balonuna `{ kind: "error", text: ..., reason: "client_error" }` adımlı bir sentez mesajı
  ekler (`useChatStream.ts` satır ~203). Kalıcı bir `TurnStep` olarak değil, yalnız o
  oturumun canlı görünümüne yerel mesaj olarak eklenir.
- **"Yeniden dene" butonu:** Asistan balonunda en az bir `kind === "error"` adımı varsa ve
  akış sürmüyorsa `MessageList.tsx` balonun altında kırmızı **Yeniden dene** butonu gösterir
  (`canRetry` mantığı). Tıklanınca `onRetry(m.id)` → `useChatStream.retryMessage` çağrılır:
  başarısız tur + tetikleyen kullanıcı mesajı çifti (kalıcıysa sunucudan da) silinir, ardından
  aynı metin yeniden gönderilir. Akış sürerken (`isLastLive`) buton gösterilmez.

### Yerleşim
- Sohbet **tam genişlik** kullanır (`MessageList`/`Composer`'daki `max-w-3xl` kaldırıldı).
- `Sidebar` (Ajanlar + Oturumlar) **sürüklenerek yeniden boyutlandırılır**: sağ kenardaki
  tutamak (200–560px), genişlik `localStorage` (`tionharness.sidebarWidth`).
- Bir turdaki üç adım türü de **tek-satır açılır-kapanır kart**: 💭 Düşünme (`thinking`),
  💬 Düşünce (ara `text` — `TextStep`), 🛠️ Tool (`tool` — `ActivityCard`). Nihai
  cevap tam görünür kalır.
- **Mesaj meta satırı (`chat/MessageMeta.tsx`):** her mesajın altında **gönderilme saati**
  (`MessageTime`, hover'da tam tarih; `lib/time.ts` `clockTime`/`fullDateTime`) ve her asistan
  turunda **çalışma süresi** (`TurnDuration` "⏱ 2 dk 15 sn"). Akış sürerken son balonda her
  saniye tıklayan **`LiveTimer`**; `formatDuration`/`formatDurationMs` ortak biçimleyici.
- **Süreler SUNUCUDAN gelir (2026-07-28):** tamamlanmış turun süresi artık frontend'de
  `createdAt` farkından türetilmez — backend turu bizzat ölçüp `Message.DurationMs` olarak
  kaydeder (`chat_stream.go` `agentStart`; otonom yollarda `turnmeta.apply`) ve `TurnDuration`
  bu ms değerini gösterir (<10 sn'de tek ondalık: "3.4 sn"). Yalnız bu alandan ÖNCE yazılmış
  eski mesajlarda eski türetim (asistan.createdAt − önceki **kullanıcı** mesajı.createdAt, yalnız
  önceki mesaj kullanıcıysa) devreye girer ve "~" ile **yaklaşık** işaretlenir.
- **Sunucu saati (`shared/lib/serverClock.ts`):** geçen-süre sayan her gösterge sunucunun
  saatine göre ölçer. Hub akışı her frame'de (+ `hello` frame'i `now` alanıyla) sunucu unix
  saniyesini taşır → `noteServerTime` skew tahminini günceller (≥2 sn fark olunca adopte edilir,
  ağ jitter'ı sayacı zıplatmaz), `serverNow()` de "şimdi"yi verir. Müşteriler: `LiveTimer`,
  `WorkerWaitBanner`, `WakeWaitBanner`, prompt-cache sıcaklık geri sayımları
  (`SessionDetailPanel`/`SessionContextModal`/`CacheWarmthStrip`). Tur başlangıcı da sunucudan: **`agent_start`**
  hub olayının `time` alanı kullanılır — durable/ringed olduğu için
  tur ortasında açılan/yenilenen pencere turu gerçek başlangıcından sayar, bağlandığı andan
  değil. Mutlak saat etiketleri (`MessageTime`) bilerek yerel saat diliminde kalır.
- **Tur altbilgisi: meta solda, aksiyon çipleri sağda — balonun DIŞINDA (2026-07-23):**
  Her mesajın altında, **balonun dışında** tek bir satır var:
  - **Sol:** pasif meta — saat, süre, model, token, cache sıcaklık noktası
    (`00:35 · ⏱ 4 dk 32 sn · claude-opus-4-8 · ↑34 ↓9.8k ⚡6.5M 🔥`); kullanıcı turunda
    saat + yönlendirme rozeti.
  - **Sağ:** aksiyon çipleri — asistanda 🔊 sesli oku + 👍/👎 puan + "Yeniden dene" +
    🗑 sil; kullanıcıda ⟲ geri sar + 🗑 sil.
  - **Görünürlük düzeltmesi (asıl kazanım):** butonlar eskiden
    `opacity-0 group-hover:opacity-100` hayaletiydi → fark edilmiyorlardı. Artık
    **durağan hâlde görünür** çipler (kenarlıklı, hover'da renk alan).
  - **Konum geçmişi:** kısa süre balonun *içine* alındı (meta sol alt / butonlar sağ alt),
    sonra tekrar **dışarı** çıkarıldı; nihai hâl budur. Balon dışı zemin nötr olduğu için
    yüzeye göre değişen **`accent` tonu tamamen kaldırıldı** — `MessageTime`,
    `DirectionBadge`, `DeleteButton`, `RewindButton` artık `tone` prop'u almıyor ve
    `messageActions.ts` tek paletli. (Butonlar tekrar balon içine alınırsa beyaz-üstü-accent
    varyantının geri gelmesi gerekir.)
  - Stil tek yerde: **`chat/messageActions.ts`** — `actionChip(intent)` /
    `actionChipActive(intent)` + `TURN_FOOTER` / `TURN_FOOTER_END` / `META_CLUSTER` /
    `ACTION_CLUSTER`. Her durum **eksiksiz** sınıf kümesi döndürür (üstüne "override"
    sınıfı eklenmez): Tailwind çakışmasını **stylesheet sırası** çözer, class-string
    sırası değil. `intent`: `default` | `danger` | `positive`.
  - **İki ayrı hizalama sabiti, bilerek:** `TURN_FOOTER` (`justify-between`) tam genişlikteki
    **asistan** balonu için; `TURN_FOOTER_END` (`justify-end`) sağa yaslı **kullanıcı**
    balonu için — orada `justify-between` meta'yı sütunun en soluna atıp balondan koparırdı.
    `TURN_FOOTER + 'justify-end'` **yapılmaz**: aynı öğede iki `justify-*` sınıfını
    stylesheet sırası çözer, class-string sırası değil.
  - `AutoPromptNote`, `PeerTurn`, `TaskNotificationNote` de aynı görünür çipi kullanır.

### Loading & iskelet durumları (2026-07-10)

Açılışta ve sohbet geçişlerinde **yanlış içerik** (boş-durum ekranı ya da önceki
sohbetin transkripti) gösterilmemesi için iki ayrı yükleme bayrağı vardır. İkisi de
`useSessionsController`'da tutulur ve `ChatView`'a prop olarak geçer:

- **`bootstrapping`** — workspace aktifleştiği andan `listAgents()`+`listSessions()`
  çözülene kadar `true`. Bu süre boyunca `ChatEmptyState` **hiç** render edilmez:
  dönen kullanıcı splash'i atladığı için, oturum listesi gelmeden önce bir an
  "Yeni sohbete başla" görünüyordu. Yükleme bitince (`!bootstrapping`) ve gerçekten
  sıfır oturum varsa boş-durum **yine gösterilir** — bastırma kalıcı değildir.
- **`messagesLoading`** — açık oturumun transkripti çekilirken `true`. Effect'in
  başında `setMessages([])` çağrılır, böylece eski sohbet beklerken ekranda kalmaz.

**Delayed-flag sözleşmesi.** Yerel backend `listMessages`'ı çoğu zaman <50ms
döndürür; gecikmesiz iskelet tek-frame'lik titreme yaratır. Bu yüzden her iki bayrak
da `shared/hooks/useDelayedFlag.ts` (≈140ms) üzerinden geçirilir: bayrak `true`
olduktan `delayMs` sonra iskelet açılır, `false` olunca **anında** kapanır. Hook'lar
erken-return'lerden **önce** çağrılır (`react-hooks/rules-of-hooks`).

**In-flight guard.** Transkript yüklemesi `msgSeqRef` sayacıyla korunur: yalnız en
yeni çağrı `setMessages` commit eder. Hızlı `A → B → A` geçişinde B'nin geç gelen
cevabı A'nın transkriptini ezmez. `recoverInflight`/`reseedLive` commit'ten **sonra**
ve yalnız güncel seq'te çalışır; böylece canlı (streaming) balon silinmez. Yükleme
hatası artık sessizce yutulmaz (`.catch(() => {})` yerine `setError`).

**Ortak primitive'ler.** `shared/components/Skeleton.tsx` → `Skeleton` (pulse bar) +
`LoadingState` (ortalanmış spinner + etiket). `features/chat/ChatSkeleton.tsx`
transkript iskeletidir (`data-testid="chat-skeleton"`). `SessionsSidebar` yüklenirken
iskelet satırlar, lazy panellerin `Suspense` fallback'leri ise düz metin yerine
`LoadingState` gösterir. `shared/hooks/useAsync.ts` `loading` bayrağı artık `enabled`
ile başlar — ilk paint "boş" değil "yükleniyor" olur; `TaskBoard`, `Schedules`,
`Automations`, `FlowsPanel`, `MarketPanel`, `ArtifactsPanel` bu desene taşındı.

### Prompt-cache görünürlüğü (2026-08-11)

Cache kırılımı artık yalnız Debug kartında değil, **sohbetin kendisinde** görünür.
Beş yüzey, kasıtlı olarak farklı sorulara cevap verir — hiçbiri diğerini tekrar etmez.
Tespit/atıf katmanı değişmedi (`internal/agent/cachebreak.go`, `_Docs\50` P4).

| Yüzey | Soru | Kaynak |
|---|---|---|
| `CacheWarmthStrip` (composer üstü) | "Şimdi göndersem ucuz mu?" | son mesajın `createdAt` + 1sn tick; ek istek yok |
| `ColdCacheDivider` (transkript ayracı) | "Bu turu ne pahalılaştırdı?" | iki mesaj arası boşluk > `CACHE_TTL_SEC` |
| `CacheWarmthDot` (tur altbilgisi 🔥/❄) | "Hangi turlar soğuk koştu?" | `Message.usage.cacheRead/cacheWrite` |
| `CacheBreakCard` (`cache_break` adımı) | "Neden kırıldı, ne yapmalıyım?" | backend atıflı `cache_break` olayı |
| `MessageDebugPanel` cache bölümü | "Sebep + kaçınılabilir fazla ödeme?" | `TurnDebug.cacheBreak*`/`coolingWaste*` |

Kritik ayrımlar:

- **Yalnız "bir şey değişti" kırılımı kart olur** (`model-changed`, `prompt-or-tools-changed`;
  `agent.inlineCacheBreak`). TTL soğuması normaldir → kart yerine ayraç + panel. Her molada
  kart basmak kullanıcıyı karta kör ederdi.
- **Kart canlı SSE ile yayılmaz**, yalnız kalıcı ize **başa** eklenir
  (`api.consumeCacheBreakLead`, `chat_stream.go`). Kırılım turun başında ödenir ama ancak
  provider yanıtından *bilinebilir*; geç yayınlamak kartı akışın altına çizip reload'da yukarı
  zıplatırdı. Non-stream `/api/chat` yolunda kart yoktur (`context_change` ile aynı kapsam).
- **Kanıt yoksa iddia yok:** 🔥/❄ noktası ve "soğuk tur" sayacı yalnız `cacheRead>0` ya da
  `cacheWrite>0` varken konuşur. OpenRouter soğuk öneki düz `input` olarak faturalar (write
  sayacı yok) → orada gösterge sessiz kalır, tahmin üretmez.
- **İlk tur soğuk sayılmaz** — oturum başlatmanın kaçınılmaz bedelidir; backend detektörünün
  `warmed` koşuluyla aynı mantık.
- `prompt-or-tools-changed` kartı **şüpheli** tonda: prompt epoch açıkken (varsayılan) bu
  kırılım oturum ortasında olmamalı (`_Docs\57`) → kart bunu söyler ve "Bağlamı yenile"
  (`/refresh-context`) aksiyonunu sunar.

### Yazılabilir oturum türleri + "Salt okunur" rozeti (2026-08-24)

Composer her oturumda görünmez: yeni bir **kullanıcı turu** yalnız belirli
oturum türlerinde başlatılabilir. Kural tek yerde tanımlıdır ve üç katman onu
aynen yansıtır.

- **Tek doğruluk kaynağı:** `writableSessionKindList` / `IsWritableSessionKind`
  (`internal/db/models.go`). Liste: `""` (manuel sohbet), `"chat"`, `"spawned"`,
  `"schedule"`, `"automation-run"`, `"schedule-run"`, `"worker"`. Geri kalan her
  tür (task, flow, automation, flow-coordinator, insight) orkestratörün yazdığı
  koşu kaydıdır — tam okunur ama yeni bir kullanıcı turunun bağlanacağı koşu
  yoktur.
- **`"worker"` yazılabilir (TSK507).** Worker transkripti bitmiş bir koşu kaydı
  değil, koordinatörün **zaten içine yazdığı** canlı bir konuşmadır
  (`SendToWorker` user-rolünde tur enjekte eder). Transkripti izleyen insanın da
  aynısını yapabilmesi gerekir: worker'ın sorduğunu yanıtlamak, rotasını
  düzeltmek, eksik bağlamı vermek. Çakışma riski `schedule` ile aynı şekilde
  sınırlıdır — insan turu ile koordinatörün enjekte ettiği tur aynı per-session
  turn slot'unu talep eder, sıraya girer.
- **`"inbox"` türü kaldırıldı (TSK507).** Ajanlar arası mesajlar artık alıcının
  **sıradan `chat` thread'ine** düşer (`agent/agentmsg.go`), o da sohbet olduğu
  için doğal olarak yazılabilir — yani insan o konuşmaya da katılabilir. Thread
  `(kind, sourceID)` ile aranır (`GetOrCreateSourceSession`, sourceID =
  `agent-messages:<agentID>`), **`(agentID, kind)` ile değil**: `"chat"` aynı
  zamanda insanın açtığı her ad-hoc oturumun türü olduğundan, ikinci arama
  kullanıcının kendi sohbetini bulup peer mesajlarını oraya teslim ederdi.
  Regresyon: `TestDeliverAgentMessage_DoesNotHijackExistingChat`.
- **Göç (migration):** eski build'lerin yazdığı `inbox` oturumları açılışta
  otomatik `chat`'e çevrilir — `db.migrateLegacyInboxSessions`, `Open` içinde
  `migrateLegacyInboxSessions` fazı. Her oturuma `PeerThreadSourceID(agentID)`
  damgalanır; **bu şart**, çünkü sourceId'siz kalan oturum teslim tarafındaki
  `GetOrCreateSourceSession` aramasına görünmez ve bir sonraki mesaj yanına
  **ikinci bir thread** açarak ajanın geçmişini ikiye bölerdi. Idempotent (her
  boot koşar, ikinci geçiş no-op) ve oturum başına best-effort: yazılamayan tek
  bir header yüzünden boot düşmez, sayılıp loglanır. Dolu bir `sourceId`'ye
  dokunulmaz. Testler: `internal/db/session_inbox_migrate_test.go` (5 test,
  `TestOpenRunsLegacyInboxMigration` boot yolunu da kapsar).
  Sidebar çipi ve `graph.go` etiketi yine de korunur: göç edemeyen bir oturum
  kalırsa "Diğer"e düşmesin.
- **`"schedule"` bilinçli istisnadır.** Zamanlayıcı da oraya yazar, ama o bir
  koşu-başına log değil, ajanın uzun ömürlü cron thread'idir; kullanıcı tikler
  arasında konuşmaya devam edebilmelidir (sorulanı yanıtlamak, düzeltmek, bağlam
  eklemek). Çakışma riski yok: kullanıcı turu ile zamanlanmış tur aynı
  per-session turn slot'unu (`turnqueue`) talep ettiğinden iç içe geçmez,
  sıraya girer.
- **Frontend aynası:** `isWritableSessionKind`
  (`frontend/src/shared/lib/sessionKind.ts`) — backend listesiyle **birlikte**
  değiştirilmelidir, yoksa composer ile API aynı oturum hakkında farklı şey
  söyler. Kullanıcıları: `pickInitialSession.ts`, `useSessionsController.ts`
  (composer kapısı) ve `SessionDetailPanel.tsx` (rozet).
- **Rozet:** `SessionDetailPanel.tsx`, "Oturum bilgisi" başlığının yanına
  `<Badge tone="muted">Salt okunur</Badge>` çizer — koşul tam olarak
  `info && !isWritableSessionKind(info.kind)`. Yani rozet **yalnız** composer'ın
  gizlendiği oturumlarda görünür; amacı eksik composer'ın hata gibi görünmesini
  engellemektir. `schedule` ve `worker` oturumlarında rozet **çıkmaz**.
- **API karşılığı:** `rejectNonWritableSession`
  (`internal/api/session_readonly.go`) aynı kapıyı sunucuda uygular ve `403`
  döner. Yanına iki kural daha oturur: `rejectReadOnlySession` (rewind — canlı
  turu yönlendirmez, geçmişi keser, bu yüzden zayıf "yazılamaz" kapısıyla
  korunur) ve `rejectImmutableSession` (stop/steer + `ask_user` yanıtı; yalnız
  `IsImmutableSessionKind` = makine transkriptleri, bugün `insight`). Bu kapılar
  yalnız HTTP uçlarındadır; süreç-içi üreticiler (`send_message`, otomasyon
  teslimi, koordinatör→worker) kasıtlı olarak dışarıdadır.

Test: `frontend/src/shared/lib/sessionKind.test.ts`,
`internal/db/models_session_kind_test.go`,
`internal/api/session_writable_test.go`, `internal/api/session_readonly_test.go`.

## Doğrulama

- `go build ./...` ve `tsc --noEmit` temiz.
- Chrome canlı testi: markdown (başlık/liste/tablo), `go` kod bloğu (renkli),
  unified diff (`+2 / −1`), düşünme bloğu, Read/Edit/Bash tool kartları
  (tıklanabilir yollar, kırmızı "hata" rozeti), inline görsel
  (`/api/files` → HTTP 200 image/png) DOM üzerinden doğrulandı.

## Doğrulama (anahtarsız claude-cli, uçtan uca)

- Ajan: `StepTest` (claude-cli, anahtarsız). Mesaj: "Bash aracıyla `echo hello-from-tionharness`
  çalıştır, sonra çıktıyı tek cümlede söyle."
- Yanıt `steps`: `[{kind:text,"Komutu çalıştırıyorum."}, {kind:tool, tool:"Bash",
  input:{command,description}, output:"hello-from-tionharness"}]` — dosya deposuna kalıcı yazıldı.
- Chrome DOM: ara metin → **▶️ Bash** tool kartı (açınca GIRDI/ÇIKTI: `hello-from-tionharness`)
  → markdown cevap. **API anahtarı kullanılmadı.**

## Notlar / Sıradaki

- `thinking` adımları: **hem native (anthropic) hem claude-cli** yolunda gösterilir.
  Ajanın `ThinkingLevel`'i (low/medium/high/**xhigh/max**) `thinkingBudgetForLevel` ile token
  bütçesine çevrilir ve **araçsız (MCP kapalı) turlarda** `Request.ThinkingBudget` olarak gönderilir.
  **Derin-çalışma tiyerleri (claude-cli):** `cliEffortLevel` artık `xhigh`/`max`'i CLI'ye **geçirir**
  (önceden `high`'a kırpılıyordu). `max`, Claude Code'un `settings.json` enum'unun reddettiği tek
  değer (sessizce `high`'a düşürür), bu yüzden `--settings` dosyasına `xhigh` (taban) yazılır ve
  provider (`runAttempt`) turu `CLAUDE_CODE_EFFORT_LEVEL=max` env'i ile `max`'a yükseltir
  (`Request.CLIEffortLevel`). Bu tiyerlerde thinking açık kaldığından paralel araç batch'i kapanır
  ("think XOR batch") — derin akıl yürütme için kabul edilen takas.
  **Tur-bazlı override:** composer'daki `🧠` seçici (`chatReq.ThinkingLevel`) bu turun
  seviyesini ajan ayarının yerine geçirir — handler yanıtlayan ajanın **yerel kopyasının**
  `ThinkingLevel`'ini değiştirir (kalıcı değil); boş = ajan ayarı. claude-cli/minimax bütçeyi
  yok sayar.
  **Model-farkında tiyer butonları (2026-08-11):** hangi seviyelerin **aktif** olacağı seçili
  modele göre değişir. Tek doğruluk kaynağı `providers.ThinkingClass(model)` (→ `ThinkingTiersFor`);
  `Catalog()` build'inde her `ModelInfo.ThinkingTiers` (`off/low/medium/high/xhigh/max`) + `ThinkingClass`
  doldurulup `/api/catalog` ile taşınır. **Beş sınıf:** `always-on` (Fable/Mythos → `off` yok, daima
  düşünür); `adaptive` (Opus 4.7/4.8, Sonnet 5 → tam rampa); `non-thinking` (**DeepSeek V4 Flash** →
  yalnız `off`); `legacy` (somut eski Claude/MiniMax — reasoning_effort tavanı `high` — DeepSeek Pro →
  `xhigh/max` yok); `alias` (claude-cli `opus`/boş "claude oturum modeli"/özel → tam rampa, provider kırpar).
  **Gizleme değil pasifleştirme:** desteklenmeyen tiyer butonu gizlenmez, **soluk+disabled** gösterilir
  ve tooltip sebebini yazar (`thinkingTierDisabledReason(cls, tier)` — "her zaman düşünür — kapatılamaz"
  / "düşünmez" / "\"Yüksek\"e düşer"). Composer (`ComposerPicker`) ve ajan formu (`OptionPills`) artık
  `disabled` opsiyonlarını destekler; kaynak `thinkingInfoForModel(catalog, provider, model)`. Model
  kataloğda yoksa (özel id) tüm tiyerler aktif; mevcut seçili seviye ve "Oto" her zaman tıklanabilir
  kalır. Backend kırpma güvenlik ağı yerinde durur.
  - **Native streaming** (`anthropic.Stream`): SSE `content_block_delta` artık `text_delta`
    **ve** `thinking_delta`'yı ayrıştırır. Tipli `providers.StreamDelta{Kind: text|thinking}`
    ile yayılır → `recordedStream` thinking parçalarını sabit `liveThinkingID` ile canlı
    `StepThinking` olarak akıtır (frontend `App.tsx` id'ye göre tek büyüyen düşünme bloğuna
    **merge** eder, tool_delta deseni). Tam thinking metni ayrıca `Response.Trace`'e konur →
    `traceToSteps` ile **kalıcı** `StepThinking` olarak mesaja yazılır (reload sonrası kalır).
  - **Native non-streaming** (`anthropic.Complete`): `thinking` content-block'u `Response.Trace`'e
    bir `thinking` adımı olarak parse edilir (otonom turlar dâhil).
  - **claude-cli**: CLI stream-json `thinking` bloğunu zaten yayınlar.
  Not: native **tool** döngüsünde thinking kapalı tutulur — imzalı thinking bloklarını geri
  beslemek gerekir, provider soyutlaması bunu korumaz (bkz. `toolloop.go`).
- claude-cli tool kullanımı, kullanıcının yerel `~/.claude` izin ayarlarına tabidir
  (print modunda izin verilen araçlar çalışır).

### CLI-native compaction kartı (2026-08-30)

- Codex `context_compaction` ve Claude `status=compacting`/`PreCompact` başlangıcı aynı ID'li
  `running` kart gösterir; running frame kalıcı mesaja veya inflight sidecar'a yazılmaz.
- Codex `item.completed`, Claude `compact_result=success`, `compact_boundary` veya `PostCompact` kanıtı
  kalıcı `source=cli-native`, `sessionAction=native-compact` kart üretir.
- Claude `compact_result=failed` çalışan kartı tombstone ile kapatır; tamamlanmış
  compaction üretmez. Aynı lifecycle içindeki başarı/boundary/PostCompact sinyalleri
  tek tamamlanmış karta deduplicate edilir.
- Claude completion sinyali eksikse çalışan kart iki dakika sonra tombstone ile
  kapanır; tamamlanmış compaction uydurulmaz.
- Provider event'i token ölçümü vermiyorsa kart token rozeti göstermez.
- Bu kart TionHarness rolling-summary kartından ayrıdır ve `SummaryMsgCount`
  değerini değiştirmez.

### Codex collab kartı (2026-08-30)

- `collab_tool_call` kartı ham prompt yerine yapısal `operation`, alıcı thread
  kimlikleri (`target`), `status`, ölçülen `durationMs` ve güvenli `summary` taşır.
- Kart başlığı ve açılmış gövde işlem, hedef, durum ve süreyi gösterir. Eski
  kayıtlarda alanlar yoksa işlem adı `collab_tool_call` olarak kalır.
- Güvenli özet yalnız işlem + alıcılar + durumdan üretilir; delegasyon prompt'u
  kullanıcı sunumuna veya debug günlüğüne yazılmaz.
