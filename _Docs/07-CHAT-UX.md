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

### Kalıcılık
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
- **Tur-ortası reload/navigasyon kurtarma (istemci tarafı, crash'ten AYRI):** tur
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

### Adım-adım akış (SSE streaming)
Sohbet artık **her adım bittikçe** UI'a akıtılır (tüm tur bitince değil).

İki streaming yolu vardır:
1. **claude-cli (trace tabanlı):** `providers.Request.OnEvent func(TraceStep)` —
   `claudecli.go` stream-json'u **satır satır** (`bufio`) okuyup olayları anında
   yayınlar: thinking hemen, ara metin flush'ta, tool adımı sonucu gelince.
   `cliStreamParser` (feed/finish) artımlı durumu tutar.
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
  `urlTransform` kimlik fonksiyonuyla devre dışı (aksi halde `C:` bir protokol
  sanılıp yerel yollar düşürülür); Windows ters-bölü yolları parse öncesi `/`'e
  normalize edilir.
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
  çevirir. `mermaid@^11` **dinamik `import()`** ile lazy yüklenir (`vite.config.ts`
  `vendor-mermaid` chunk'ı → ana bundle'a binmez). Tema base'i `<html data-theme>`'ten
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
- `common/Lightbox.tsx` — **paylaşılan tam-ekran zoom+pan** önizleyici (2026-06-26):
  tekerlek ile imlece-doğru zoom (translate düzeltmeli), sürükle-pan, toolbar
  (uzaklaştır/yüzde/yakınlaştır/sıfırla/kapat), çift-tık toggle, Escape + `0` (reset),
  temiz backdrop tıklamasıyla kapanır (pan'dan sonra kapanmaz). `imageSrc` (görsel) ya
  da `children` (mermaid SVG) **ya da `images[]` + `index` (galeri, ←/→ + ok
  butonları + "n / N" sayaç)** alır. Kullananlar: mermaid Expand, Markdown inline
  görselleri (tıkla→zoom), `Gallery` (çoklu görsel), `UserBubble` attachment
  önizlemesi (eski yerel ImageLightbox kaldırıldı), `ArtifactView` görsel artifact
  (`ImageArtifact`).

### Sohbet bileşenleri (`components/chat/`)
- `TurnSteps.tsx` — bir turun iz listesini sırayla çizer; `parseSteps` JSON'u
  güvenli çözer.
- `ThinkingBlock.tsx` — model akıl yürütmesi: tool ActivityCard ile **aynı tek-satır
  açılır-kapanır kart** (💭 + "Düşünme" + truncate önizleme + chevron; açınca tam metin,
  dimmed/italik). Varsayılan kapalı.
- `ActivityCard.tsx` — tek tool çağrısı: ikon + etiket + tek satır niyet
  (başlıkta), açınca girdi/çıktı. Edit/Write çıktısı diff olarak. Hata kırmızı.
  **`use_skill` (2026-07-02):** başlık özeti artık slug **değerini** gösterir
  (`lib/tools.ts summarize` `slug`/`skill` alanlarını da toplar; eskiden "slug"
  anahtar-adı yazıyordu); açınca gövde `<pre>` yerine `Markdown` ile biçimli
  render olur (`# Skill: <slug>` başlığı + md gövde), gereksiz "Girdi" (`{slug}`)
  bloğu skill'de gizlenir.
- `DiffCard.tsx` — `kind:diff` adımı için özel dosya-değişikliği kartı: ✏️ +
  eylem (Oluştur/Düzenle/Yaz) + tıklanabilir yol + `+N −M` satır sayıları
  (başlıkta), açınca `DiffView` ile birleşik patch. `Write`/`Edit`
  çağrıları `todo_write` gibi generic tool satırı yerine bu kart olur.
  **Yol katlanabilirliği (2026-07-02):** yol artık `shortPath` (…/dir/file) ile
  kısa gösterilir ve tüm satırı kaplamaz; yalnız yol metni dosyayı açar
  (stopPropagation), kalan `flex-1` alan + chevron başlık-butonun parçası kalıp
  diff'i katlar (eskiden `flex-1` tam-genişlik yol satırı katlamayı engelliyordu).
  Tam yol `title` ile hover'da görünür.
- `PathText.tsx` — düz metindeki dosya yollarını tıklanabilir çiplere çevirir
  (`lib/paths.ts` tespit eder).

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
  için döngü kırıldı; satırlar tam-yükseklikte kalır. Her satırı şu bileşenlere
  devreder:
  - `UserTurn.tsx` — gerçek kullanıcı mesajı (balon + sağ meta satırı).
  - `PeerTurn.tsx` — **gelen peer/inbox mesajı** (2026-07-06): başka bir ajanın
    yazdığı, `Role:"user"` ama `authorKind:"agent"` (+`authorId`) ile damgalı mesaj.
    Kullanıcının sağ-hizalı balonu yerine **sola-hizalı, gönderen ajanın avatar+adı**
    ile (AgentHeader) çizilir; altında `DirectionBadge` ile "→ alıcı" ve zaman/silme
    meta'sı gösterilir. `MessageList` `isPeer(m)` ile tespit edip bu bileşene devreder;
    peer satırları "typed user" pin/overlay'den de dışlanır. Gönderen roster'da yoksa
    ham `authorId` fallback olarak yazılır (atıf sessizce kaybolmaz).
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
    girdi temizlenir. Komutlar `App.tsx`'te `chatCommands` (useMemo): `/reflect` (yansıma),
    `/memory` · `/board` · `/flows` (talep-üzerine özet, `POST /api/sessions/{id}/summary`),
    `/tools` (deterministik araç listesi). Özet komutları çalışınca **komutun kendisi de**
    sohbete bir kullanıcı balonu (`/kind`, komut stilinde) olarak yazılır, ardından sonuç
    asistan mesajı gelir (ikisi de kalıcı; `{userMessage, replyMessage}`). Bir komutu
    **çalıştırmadan düz metin** göndermek için tırnak içine al: `"/komut"` (bkz. `chat/UserBubble.tsx`).
  - Klavye: ↑/↓ gezinme, Enter/Tab seçim, Esc kapat (menü açıkken Enter göndermez).
- `SlashCommand` tipi `types.ts`'te (`name`/`description`/`icon`/`run`).
- **Düşünme seviyesi seçici (`ThinkingPicker`):** textarea'nın solunda `🧠` butonu +
  üstte açılan menü (**Oto**=ajan ayarı / **Kapalı** / **Düşük** / **Orta** / **Yüksek**,
  dışarı-tıkla-kapat). Seçim `App.tsx` `thinkingLevel` state'inde + `localStorage`
  (`tionswarm.thinkingLevel`) ile kalıcı; `chatStream` gövdesine `thinkingLevel` olarak gider
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
- **Son turların araç I/O özeti (2026-06-23):** Geçmiş provider'a çevrilirken araç
  çağrı/sonuçları düşüyordu (`toProviderMessages` yalnız metin) → ajan "az önce ne
  yaptın / o komut ne döndü" diye soramıyordu. Artık `Prepare`'den önce son **N=4**
  asistan turunun `Steps` izinden kompakt bir `<recent_tool_activity>` bloğu
  (araç+kısa arg → kırpılmış çıktı; tur başına ≤10 araç, çıktı ≤240 rune) o turun
  metnine **kopya üzerinde** eklenir (`api/chat_tool_summary.go`). Token maliyeti
  son N turla sınırlı. Test: `chat_tool_summary_test.go`.
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
  tutamak (200–560px), genişlik `localStorage` (`tionswarm.sidebarWidth`).
- Bir turdaki üç adım türü de **tek-satır açılır-kapanır kart**: 💭 Düşünme (`thinking`),
  💬 Düşünce (ara `text` — `TextStep`), 🛠️ Tool (`tool` — `ActivityCard`). Nihai
  cevap tam görünür kalır.
- **Mesaj meta satırı (`chat/MessageMeta.tsx`):** her mesajın altında **gönderilme saati**
  (`MessageTime`, hover'da tam tarih; `lib/time.ts` `clockTime`/`fullDateTime`) ve her asistan
  turunda **çalışma süresi** (`TurnDuration` "⏱ 2 dk 15 sn" = asistan.createdAt − önceki
  **kullanıcı** mesajı.createdAt; yalnız önceki mesaj kullanıcıysa, enjekte özet/ardışık
  asistan turları yanıltmasın). Akış sürerken son balonda her saniye tıklayan **`LiveTimer`**;
  `formatDuration` ortak biçimleyici.

## Doğrulama

- `go build ./...` ve `tsc --noEmit` temiz.
- Chrome canlı testi: markdown (başlık/liste/tablo), `go` kod bloğu (renkli),
  unified diff (`+2 / −1`), düşünme bloğu, Read/Edit/Bash tool kartları
  (tıklanabilir yollar, kırmızı "hata" rozeti), inline görsel
  (`/api/files` → HTTP 200 image/png) DOM üzerinden doğrulandı.

## Doğrulama (anahtarsız claude-cli, uçtan uca)

- Ajan: `StepTest` (claude-cli, anahtarsız). Mesaj: "Bash aracıyla `echo hello-from-tionswarm`
  çalıştır, sonra çıktıyı tek cümlede söyle."
- Yanıt `steps`: `[{kind:text,"Komutu çalıştırıyorum."}, {kind:tool, tool:"Bash",
  input:{command,description}, output:"hello-from-tionswarm"}]` — dosya deposuna kalıcı yazıldı.
- Chrome DOM: ara metin → **▶️ Bash** tool kartı (açınca GIRDI/ÇIKTI: `hello-from-tionswarm`)
  → markdown cevap. **API anahtarı kullanılmadı.**

## Notlar / Sıradaki

- `thinking` adımları: **hem native (anthropic) hem claude-cli** yolunda gösterilir.
  Ajanın `ThinkingLevel`'i (low/medium/high) `thinkingBudgetForLevel` ile token bütçesine
  çevrilir ve **araçsız (MCP kapalı) turlarda** `Request.ThinkingBudget` olarak gönderilir.
  **Tur-bazlı override:** composer'daki `🧠` seçici (`chatReq.ThinkingLevel`) bu turun
  seviyesini ajan ayarının yerine geçirir — handler yanıtlayan ajanın **yerel kopyasının**
  `ThinkingLevel`'ini değiştirir (kalıcı değil); boş = ajan ayarı. claude-cli/minimax bütçeyi
  yok sayar.
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
