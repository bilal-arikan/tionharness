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
  `diff` blokları `DiffView`'e gider.
- `DiffView.tsx` — unified diff'i satır bazlı +/- renkli ve `+N / −M` istatistik
  başlığıyla çizer (`lib/diff.ts` ayrıştırır).

### Sohbet bileşenleri (`components/chat/`)
- `TurnSteps.tsx` — bir turun iz listesini sırayla çizer; `parseSteps` JSON'u
  güvenli çözer.
- `ThinkingBlock.tsx` — model akıl yürütmesi: tool ActivityCard ile **aynı tek-satır
  açılır-kapanır kart** (💭 + "Düşünme" + truncate önizleme + chevron; açınca tam metin,
  dimmed/italik). Varsayılan kapalı.
- `ActivityCard.tsx` — tek tool çağrısı: ikon + etiket + tek satır niyet
  (başlıkta), açınca girdi/çıktı. Edit/Write çıktısı diff olarak. Hata kırmızı.
- `PathText.tsx` — düz metindeki dosya yollarını tıklanabilir çiplere çevirir
  (`lib/paths.ts` tespit eder).

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

### Composer — `@` ajan / `/` komut menüleri
- `Composer.tsx` otomatik-tamamlama menüsü: caret konumuna göre `detectTrigger`.
  - **`@`** (herhangi bir token başında) → **ajan seçici**; seçim `onPickAgent`
    (aktif ajanı değiştirir), `@token` metinden silinir. Ajanlar `name` ile filtrelenir.
  - **`/`** (girdinin başında, tek kelime) → **komut paleti**; seçim `SlashCommand.run()`,
    girdi temizlenir. Komutlar `App.tsx`'te `chatCommands` (useMemo): `/new`, `/title`,
    `/reflect`, `/memory`, `/tools`, `/board`, `/flows`.
  - Klavye: ↑/↓ gezinme, Enter/Tab seçim, Esc kapat (menü açıkken Enter göndermez).
- `SlashCommand` tipi `types.ts`'te (`name`/`description`/`icon`/`run`).

### Session-bazlı + çok-ajanlı sohbet (`@` yönlendirme)
- Sohbet **session-bazlı**: sol panel **"Tüm Oturumlar"** düz listesi (ajan altında
  gruplama yok); her oturumun bir **varsayılan ajanı** (`Session.AgentID`) vardır ve
  satırda o ajanın avatarı görünür. Roster "**Ajanlar · varsayılan**" = yeni sohbetlerin
  varsayılan ajan seçicisi. `POST /api/sessions` `agentId` opsiyonel (boş → ilk ajan).
- **`@` ile tur yönlendirme:** composer `@` menüsü metne `@Ad` mention'ı ekler.
  `App.sendMessage` mention'ları ajanlara çözüp `agentIds[]` üretir (yoksa oturum
  varsayılanı). `POST /api/chat/stream` `agentIds` alır.
- **Çok-ajan (sıralı):** birden çok `@` → her ajan **sırayla** yanıtlar, sonrakiler
  öncekilerin yanıtını görür. SSE: `meta` → (her ajan için) `agent {agentId,index}` →
  `step`* → `reply {replyMessage}` → `done`. Her asistan turu `Message.AgentID` ile
  kalıcılaşır; `MessageList` her turu **kendi ajanının avatar+adıyla** çizer.

### Yerleşim
- Sohbet **tam genişlik** kullanır (`MessageList`/`Composer`'daki `max-w-3xl` kaldırıldı).
- `Sidebar` (Ajanlar + Oturumlar) **sürüklenerek yeniden boyutlandırılır**: sağ kenardaki
  tutamak (200–560px), genişlik `localStorage` (`swarmgo.sidebarWidth`).
- Bir turdaki üç adım türü de **tek-satır açılır-kapanır kart**: 💭 Düşünme (`thinking`),
  💬 Düşünce (ara `text` — `TextStep`), 🛠️ Tool (`tool` — `ActivityCard`). Nihai
  cevap tam görünür kalır.

## Doğrulama

- `go build ./...` ve `tsc --noEmit` temiz.
- Chrome canlı testi: markdown (başlık/liste/tablo), `go` kod bloğu (renkli),
  unified diff (`+2 / −1`), düşünme bloğu, Read/Edit/Bash tool kartları
  (tıklanabilir yollar, kırmızı "hata" rozeti), inline görsel
  (`/api/files` → HTTP 200 image/png) DOM üzerinden doğrulandı.

## Doğrulama (anahtarsız claude-cli, uçtan uca)

- Ajan: `StepTest` (claude-cli, anahtarsız). Mesaj: "Bash aracıyla `echo hello-from-swarmgo`
  çalıştır, sonra çıktıyı tek cümlede söyle."
- Yanıt `steps`: `[{kind:text,"Komutu çalıştırıyorum."}, {kind:tool, tool:"Bash",
  input:{command,description}, output:"hello-from-swarmgo"}]` — dosya deposuna kalıcı yazıldı.
- Chrome DOM: ara metin → **▶️ Bash** tool kartı (açınca GIRDI/ÇIKTI: `hello-from-swarmgo`)
  → markdown cevap. **API anahtarı kullanılmadı.**

## Notlar / Sıradaki

- `thinking` adımları: **hem native (anthropic) hem claude-cli** yolunda gösterilir.
  Ajanın `ThinkingLevel`'i (low/medium/high) `thinkingBudgetForLevel` ile token bütçesine
  çevrilir ve **araçsız (MCP kapalı) turlarda** `Request.ThinkingBudget` olarak gönderilir.
  - **Native streaming** (`anthropic.Stream`): SSE `content_block_delta` artık `text_delta`
    **ve** `thinking_delta`'yı ayrıştırır. Tipli `providers.StreamDelta{Kind: text|thinking}`
    ile yayılır → `recordedStream` thinking parçalarını sabit `liveThinkingID` ile canlı
    `StepThinking` olarak akıtır (frontend `App.tsx` id'ye göre tek büyüyen düşünme bloğuna
    **merge** eder, tool_delta deseni). Tam thinking metni ayrıca `Response.Trace`'e konur →
    `traceToSteps` ile **kalıcı** `StepThinking` olarak mesaja yazılır (reload sonrası kalır).
  - **Native non-streaming** (`anthropic.Complete`): `thinking` content-block'u `Response.Trace`'e
    bir `thinking` adımı olarak parse edilir (otonom/heartbeat turları dâhil).
  - **claude-cli**: CLI stream-json `thinking` bloğunu zaten yayınlar.
  Not: native **tool** döngüsünde thinking kapalı tutulur — imzalı thinking bloklarını geri
  beslemek gerekir, provider soyutlaması bunu korumaz (bkz. `toolloop.go`).
- claude-cli tool kullanımı, kullanıcının yerel `~/.claude` izin ayarlarına tabidir
  (print modunda izin verilen araçlar çalışır).
