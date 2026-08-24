# Çoklu Provider Mimarisi İncelemesi (gelecek plan)

> Kaynak: bir TypeScript / Next.js / Electron referans projesi. İnceleme tarihi: **2026-06-18**.
> Amaç: o projenin ~70 provider'ı nasıl düşük eforla eklediğini anlamak ve TionHarness'e
> taşınabilir desenleri çıkarmak.
> **Durum: yalnız plan — uygulamaya geçilmedi.**

## Ana bulgu: "metadata'yı protokolden ayır"

Referans projede ~70 provider var ama **sadece 4 paylaşılan handler** üzerine oturuyor.
Provider eklemek çoğunlukla kod yazmak değil, **bir satır/blok metadata** eklemek.
Tanımlar `src/lib/providers/index.ts` `PROVIDERS` map'inde ve
`src/lib/providers/cli-provider-metadata.ts`'te.

### 1. OpenAI-uyumlu API ailesi (~25 provider → tek handler)

OpenRouter, DeepSeek, Groq, Together, Mistral, xAI, Fireworks, Nebius, DeepInfra,
Google (OpenAI-compat ucu), TokenMix, LM Studio, the external agent… **hepsi `streamOpenAiChat`'i
paylaşıyor.** Aralarındaki tek fark `defaultEndpoint` (baseURL) + model listesi:

```ts
deepseek: {
  id: 'deepseek', name: 'DeepSeek',
  models: ['deepseek-chat', 'deepseek-reasoner'],
  requiresApiKey: true, requiresEndpoint: false,
  defaultEndpoint: 'https://api.deepseek.com/v1',
  handler: {                                   // ← yeni protokol kodu YOK
    streamChat: (opts) => streamOpenAiChat({
      ...opts,
      session: { ...opts.session, apiEndpoint: opts.session.apiEndpoint || '<baseURL>' },
    }),
  },
},
```

Gerçekten özel protokolü olan yalnızca **Anthropic** ve **Ollama** kendi handler'ında.

### 2. Generic CLI factory (~30 CLI → tek handler, satır-başına bir provider)

aider, amp, augment, cline, continue, kimi, openhands, replit, warp, windsurf… bir
**4'lü diziden** üretiliyor (`[id, görünenAd, binaryAdı, yetenek]`), `.map()` ile metadata
nesnesine dönüşüyor, `buildGenericCliEntries()` `PROVIDERS`'a yayıyor. Hepsi tek handler
`streamGenericCliChat`'i kullanıyor: binary'i `spawn` et, prompt'u argv olarak ver,
stdout satırlarını delta olarak stream et. **Bir CLI = bir satır.**

### 3. Bespoke CLI handler'ları (~10 CLI → gerçek kod)

Claude Code, Codex, Gemini, external CLI agent, Cursor, OpenCode, Goose gibi **yapısal JSON çıktısı
olan** CLI'ler kendi stream parser'ına sahip. Geri kalan generic factory'ye düşüyor.

### 4. Custom + extension provider'lar

Runtime'da kullanıcının eklediği `custom-*` (type=custom) provider'lar da
`streamOpenAiChat`'i depolanmış `baseUrl` ile çağırıyor; eklenti sistemi
(`getExtensionManager`) ekstra provider register edebiliyor. Hepsi `getProviderList()`'te
birleşiyor; UI, model-discovery, health-check, kimlik bilgisi eşleme aynı `id`'den türüyor.

### Özet tablo

| Katman | Adet | Efor |
|--------|------|------|
| OpenAI-uyumlu API | ~25 | her biri ~12 satır veri, **handler ortak** |
| Generic CLI | ~30 | her biri **1 satır** veri, handler ortak |
| Bespoke CLI | ~10 | gerçek kod (özel parser) |
| Custom/extension | ∞ | kod yok, runtime |

---

## TionHarness için çıkarımlar (gelecek plan)

> TionHarness zaten aynı mimaride: `kind_*.go` + generic `OpenAICompat` + data-instance
> custom providers (CG-19, 2026-06-18 tamamlandı). Aşağıdakiler **opsiyonel genişletmeler**.

### SC-1 — Built-in API provider preset kataloğu (CLI değil)

> **Durum (2026-06-19): Kısmen hayata geçirildi.** `internal/providers/kind_openrouter.go`
> eklendi: `openrouter` kind, OpenRouter'ın OpenAI-uyumlu ucu üzerinden yüzlerce modeli
> tek API key ile sunar; ~25 model önerisi kataloğa dahil. Bu, aşağıdaki planın ilk
> somut adımıdır.

**Fikir:** `OpenAICompat` handler'ı zaten hazır. Referans projenin `PROVIDERS` map'indeki
OpenAI-uyumlu girişleri (DeepSeek, Groq, Together, xAI, Fireworks, Nebius, DeepInfra,
OpenRouter, Mistral, Google-compat…) TionHarness'te **önceden-tanımlı preset katalog** girişi
olarak eklemek = yalnız `{id, label, baseURL, defaultModel, models}` verisi, **sıfır yeni
protokol kodu**. Kullanıcı yalnız anahtarını (sır kasası) seçer.

**Dokunulacak yerler (tahmini):** `providers/catalog.go` `CustomCatalog` veya yeni bir
`presetCatalog`; UI'da "Hazır sağlayıcılar" listesi (tek tıkla ekle). Migration gerekmez.
**Risk:** düşük — mevcut OpenAICompat yolunu kullanır. Referans projenin `PROVIDERS` map'i birebir
şablon.

### SC-2 — Generic CLI factory (CLI ailesi)

**Fikir:** Şu an TionHarness'te yalnız `claude-cli` var. Referans projenin `streamGenericCliChat`
deseni (binary spawn + stdout satır-stream, JSON parse yok) ile yapısal çıktısı olmayan
onlarca coding-CLI'yi **tek handler + veri listesiyle** eklenebilir hale getirmek.

**Dokunulacak yerler (tahmini):** yeni `providers/kind_genericcli.go` + bir
`[]genericCLI{id, label, binary}` tablosu; `registry.go` katalog birleştirme.
**Risk:** orta — alt-süreç yönetimi, PATH çözümü, timeout, abort, çalışma dizini
sandbox'ı (mevcut claude-cli `cmd.Dir` deseni yeniden kullanılabilir).
**Not:** Bu bir **CLI işi** — kullanıcı talebiyle şimdilik kapsam dışı; yalnız plan olarak
kaydedildi.

### Öncelik notu

SC-1 düşük-efor/yüksek-değer (native API ekosistemi genişler, abonelik gerektirmez).
SC-2 daha büyük ve CLI ailesine ait → SC-1'den sonra ve CLI fazı açıldığında ele alınmalı.
