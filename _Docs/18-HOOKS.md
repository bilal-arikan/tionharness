# 18 — Hooks (Araç + Yaşam Döngüsü) — Faz P4 + Lifecycle Parite

> Kullanıcı-tanımlı dış komutların, native araç döngüsünde her araç çağrısının
> **ve** turun/oturumun yaşam-döngüsü noktalarında çalışması. Claude Code'un **tam
> 9-olaylık** hook sözleşmesiyle uyumlu — aynı script'ler (ör. **sqz**, **caveman**)
> değiştirilmeden çalışır.

## Olay kümesi (9 — Claude Code paritesi)

| Olay | Ne zaman | Kapsam | Etki |
|------|----------|--------|------|
| `PreToolUse` | araç çağrısı öncesi | yalnız native | girdi rewrite / onayla / **engelle** |
| `PostToolUse` | araç çağrısı sonrası | yalnız native | çıktı dönüştür / bağlam ekle / engelle |
| `UserPromptSubmit` | her kullanıcı promptu öncesi | **native + claude-cli** | bağlam enjekte / **turu engelle** |
| `SessionStart` | oturumun ilk turu | **native + claude-cli** | bağlam enjekte (matcher = kaynak) |
| `Stop` | ana ajan turu bitti | native + claude-cli | audit + **block→devam** (sınırlı yeniden-tur) |
| `SubagentStop` | `run_subagent` bitti | native + claude-cli | audit + alt-ize bağlam ekler |
| `PreCompact` | özetleme (fold) öncesi | — | audit (matcher = tetik `auto`/`manual`) |
| `Notification` | ajan bildirim yükseltti | — | audit (her `notify` çağrısı) |
| `SessionEnd` | oturum silindi/arşivlendi | — | temizlik (silmeden önce, fire-and-forget) |

**Neden yaşam-döngüsü olayları claude-cli'da DA çalışır:** araç hook'ları (Pre/Post)
native döngüye özeldir çünkü claude-cli kendi tool loop'unu sürer. Ama
`UserPromptSubmit`/`SessionStart` **araç değil bağlam enjeksiyonudur** — dönen
`additionalContext` turun `SystemDynamic`'ine katlanır ve bu claude-cli'ya da geçer.
Bir **caveman**-tarzı paketin "mesaj birden itibaren otomatik" davranışı böylece
native olarak desteklenir.

### Motor (generic runner)

`Runtime.RunLifecycleHooks(ctx, sessionID, event, LifecycleExtras)` — `execHook`
üzerine ince bir generic katman (agent/hooks.go). Enabled hook'ları olay bazında
listeler, matcher'ı olayın **seçicisine** göre test eder (`SessionStart`→kaynak,
`PreCompact`→tetik; diğerleri seçicisiz, boş matcher = tümü), her hook'un
`additionalContext`'ini (üst-seviye **veya** `hookSpecificOutput.additionalContext`)
toplar ve ilk `decision:"block"`'ta durur. Fail-open. API tur orkestratöründen
(`chat_stream`/`subagent`/`sessions`) çağrılır — bu yüzden **exported**.

**Düz-stdout paritesi (kritik):** Claude Code, `SessionStart` ve `UserPromptSubmit`
hook'larının **düz (JSON-olmayan) stdout**'unu doğrudan enjekte edilen bağlam sayar —
caveman'in `caveman-activate.js`'i gibi birçok gerçek hook kuralları JSON değil düz
metin basar. `execHook` non-JSON çıktıyı `rawStdout`'a taşır; `RunLifecycleHooks` bu
iki olay için `additionalContext` yoksa `rawStdout`'u bağlam olarak kullanır (araç
hook'ları `rawStdout`'u yok sayar → "izin ver, değişiklik yok" davranışı korunur).

**Uçtan uca doğrulandı (2026-07-05):** caveman reposu `ingest` ile hook'larıyla
(`SessionStart`×2 + `UserPromptSubmit`) WS9'a kuruldu; **Soul'unda caveman OLMAYAN**
düz bir claude-cli ajanı, yalnız hook enjeksiyonuyla caveman-full stilinde yanıtladı
(611→281 kelime, −%54). Materyalize node script'i (`caveman-activate.js`) bağımlılıksız
çalıştı; düz stdout paritesi olmadan enjekte edilmezdi.

### Tetikleme noktaları (kod)

| Olay | Tetik yeri |
|------|-----------|
| `SessionStart` + `UserPromptSubmit` | `api/chat_stream.go` — ajan döngüsünden önce; context `composeTurnRequest`'in `lifecycleContext` parametresiyle `dynamic`'e katlanır; `UserPromptSubmit` block → tur kısa-devre (reason asistan mesajı olarak kalıcı) |
| `Stop` | `api/chat_stream.go` — ajan döngüsü **continuation loop ile sarılı**: her geçiş sonrası Stop tetiklenir; `block` dönerse reason bir sonraki geçişe `[A Stop hook asked you to keep working…]` bağlamı olarak enjekte edilir. `StopHookActive` (geçiş>0) hook'a zorlanmış devam olduğunu bildirir; `maxStopPasses=3` sert tavan. Hook yoksa tek geçiş. |
| `SubagentStop` | `agent/subagent.go` — sync `runAgent` tamamlanınca |
| `PreCompact` | `conversation.Manager.Prepare` — `summarize` (fold) öncesi `WithPreCompact(ctx, fn)` callback'i (import döngüsünü kırar); `api/chat_stream.go` fn'i PreCompact hook'unu tetikler |
| `Notification` | `api/notifysink.go` — `notifySink.onNotify` callback'i her `notify` çağrısında Notification hook'unu tetikler (`api/chat_stream.go`'da bağlanır) |
| `SessionEnd` | `api/sessions.go` — `DeleteSession`'dan **önce** |

### Generic hook import (caveman gibi paketler)

Bir Claude Code plugin'inin **hook'ları da** tek importla kurulur (`ingest`
pipeline'ı, 5. adapter):
- **`ingest/hook_adapter.go`** — `.claude-plugin/plugin.json` / `hooks.json` /
  `settings.json` içindeki `hooks` bloğunu keşfeder, her komutu bir `market.KindHook`
  pack'ine çevirir; `${CLAUDE_PLUGIN_ROOT}/...` ile atıfta bulunulan script'leri (+
  kardeş dosyaları) `Pack.Files`'a bundle'lar, placeholder'ı **korur**.
- **`api/market_install_hook.go`** — script'leri `<workspace>/hook-scripts/<slug>/`'a
  materyalize eder, `${CLAUDE_PLUGIN_ROOT}`'u o dizine rewrite eder, `db.Hook` oluşturur
  (path-traversal guard'lı). Tek-install-otoritesi (`installPackInto`) üzerinden.
- **Sınırlama:** `fetch` `node_modules/`'ı eler → dış runtime bağımlılığı olan script
  bundle edilmez (uyarı verilir); saf-stdlib script'ler doğrudan çalışır, aksi halde
  kullanıcı materyalize dizinde paket yöneticisini çalıştırır.

## Ne işe yarar?

- **PreToolUse:** araç çalışmadan **önce** çalışır. Komut, çağrının girdisini
  yeniden yazabilir (`updatedInput`), çağrıyı otomatik onaylayabilir (izin
  kapısını atlar), veya **engelleyebilir** (modele hata sonucu döner).
- **PostToolUse:** araç çalıştıktan **sonra** (ve token sıkıştırmasından sonra)
  çalışır. Komut, çıktıyı dönüştürebilir (`updatedOutput` — ör. dış sıkıştırma),
  bağlam ekleyebilir (`additionalContext`), veya sonucu engelleyebilir.

Tipik kullanım: denetim/audit log, güvenlik politikası (tehlikeli komutu blokla),
otomatik onay kuralları, dış araç-çıktısı sıkıştırma (sqz).

## Kapsam: araç hook'ları native, yaşam-döngüsü hook'ları her ikisi

**Araç** hook'ları (`PreToolUse`/`PostToolUse`) **yalnız native (anthropic/minimax)
araç döngüsünde** uygulanır (`agent/toolloop.go`). **claude-cli** kendi döngüsünü
sürer ve kendi hook'larını `~/.claude/settings.json`'dan okur — TionSwarm araç
hook'ları oraya yazılmaz (Faz P3 "CLI vs native ayrımı" deseni). **Yaşam-döngüsü**
hook'ları (`UserPromptSubmit`/`SessionStart`/…) ise turun `SystemDynamic`'ine bağlam
enjekte ettiği veya turu gözlemlediği için **hem native hem claude-cli** yolunda
çalışır (üstteki tabloya bak).

> sqz'i claude-cli ajanlarında kullanmak için: `sqz init --global` (sqz kendini
> Claude Code'un PreToolUse hook'u olarak kurar; TionSwarm'da bir şey gerekmez).
> Native ajanlarda kullanmak için: TionSwarm'da bir PostToolUse hook olarak `sqz`
> komutunu tanımlayın.

## Sözleşme (Claude Code paritesi)

Komut, **stdin'den JSON** alır, **stdout'a JSON** döner. `exit 2` = engelle.

**PreToolUse stdin:**
```json
{ "session_id": "", "cwd": "<workspace>", "hook_event_name": "PreToolUse",
  "tool_name": "Bash", "tool_input": { "command": "..." } }
```
**PostToolUse stdin:** yukarıdakine ek `"tool_response": { "content": "...", "isError": false }`.

**stdout (tümü opsiyonel; boş stdout + exit 0 = "izin ver, değişiklik yok"):**
```json
{ "decision": "block" | "approve",
  "reason": "...",
  "hookSpecificOutput": { "permissionDecision": "allow" | "deny" | "ask" },
  "updatedInput":  { ... },   // PreToolUse: tool_input'u değiştir
  "updatedOutput": "...",      // PostToolUse: çıktıyı değiştir
  "additionalContext": "..." } // PostToolUse: modele ek mesaj
```

- `exit 2` → engelle; **stderr** sebep olarak modele beslenir.
- JSON olmayan temiz-çıkış stdout'u → yok sayılır ("izin ver").
- Komut **workspace sandbox kökünde** (`r.workDir`) çalışır.
- **Güvenlik:** timeout (vars. 30s, clamp 1–120), çıktı cap'i (64 KB), hook
  hatası/timeout **fail-open** (çağrı devam eder — bozuk hook turu kilitlemez).

> **Interpreter hatası ≠ engelle (2026-08-17):** POSIX sh/bash bir **sözdizimi
> hatasında da `exit 2`** döner — yani Claude Code sözleşmesinin "engelle" için
> ayırdığı kodun aynısı. Bu yüzden yanlış lehçede yazılmış bir hook, kasıtlı bir
> deny'den ayırt edilemiyor ve eşleştiği aracı **oturum boyunca sessizce bloke
> ediyordu** (canlı bulgu: rtk PreToolUse hook'u `syntax error near unexpected
> token '|'` ile ölüp Bash'i devre dışı bırakıyordu).
>
> `execHook` artık exit kodunu yorumlamadan **önce** stderr'i inceler
> (`interpreterFailure`): parse/başlatma imzası varsa (`syntax error`,
> `unexpected token`, `command not found`, `ParserError`, …) sonuç bir **karar
> değil hatadır** — fail-open akar, araç çağrısı geçer. Sıradan bir stderr
> sebebiyle gelen `exit 2` **hâlâ engeller**. Hata mesajı beklenen lehçeyi
> (`PowerShell` / `POSIX sh`) ve suçlu satırı taşır; çağıran taraf bunu
> `debug.jsonl`'e `<tool>:error:<mesaj>` olarak yazar, böylece bozuk hook turu
> yeniden koşturmadan teşhis edilir.

## Veri modeli + depolama

`db.Hook` (`internal/db/models_hook.go`): `ID, Event, Matcher, Type("command"),
Command, TimeoutSec, Enabled, CreatedBy, CreatedAt`. Per-workspace
`store/hooks/<id>.json` (atomik write-through, schedule/mcp store deseni).
CRUD: `internal/db/store_hook.go` (+ `ListEnabledHooksByEvent` filtresi).

**Matcher:** araç adı glob'u (`filepath.Match`: `*`, `?`). Boş = tüm araçlar.
**Virgülle ayrılmış alternatifler** desteklenir (`Bash,PowerShell`) — herhangi biri
eşleşirse hook tetiklenir (Go'nun `filepath.Match`'i süslü parantez `{}` desteklemez;
`hookMatches` virgülde bölüp her alternatifi dener). Bu, `shell` aracının `Bash` +
`PowerShell` olarak bölünmesinden (2026-07-01) sonra tek hook'un iki shell aracını da
kapsaması için gerekir — Windows'ta ajan `PowerShell` aracını kullanır, yalnız `Bash`
matcher'ı **hiç eşleşmezdi**. Birden çok eşleşen hook **oluşturma sırasına göre**
zincirlenir; ilk `block` kazanır.

> **claude-cli köprüsü (2026-07-13):** Virgül-glob **TionSwarm'ın native** sözdizimidir.
> Claude Code matcher'ı **REGEX** sayar (alternation `|`, virgül literal), o yüzden
> `writeCLISettings` matcher'ı `cliMatcherRegex` ile çevirir: virgül→`|`, glob→regex
> (`*`→`.*`), `^…$` ankraj (`climcp_matcher.go`). **Önceden verbatim yazılıyordu → virgüllü
> matcher CLI turlarında sessizce hiç ateşlenmiyordu** (sqz/rtk CLI ajanlarında ölüydü).
> **Bridged-shell genişletme (2026-07-14):** built-in shell açıkken CLI, `Bash`/`PowerShell`
> yerine köprülü `mcp__tionswarm_interaction__Bash`/`__PowerShell`'i görür — o yüzden
> `cliMatcherRegex`, matcher'daki `Bash`/`PowerShell` alternatiflerine köprülü formu da
> **otomatik ekler** (deduplu). Böylece düz `Bash,PowerShell` matcher'ı (tek-tık şablonu +
> tüm mevcut/gelecek workspace'ler) CLI'da veri düzenlemeden ateşlenir; aşağıdaki manuel
> `Bash,PowerShell` uyarısı artık yalnız **native** yol için geçerli. **Canlı E2E ile
> doğrulandı (2026-07-14):** düz `PowerShell` matcher'lı bir hook gerçek claude-cli turunda
> `mcp__tionswarm_interaction__PowerShell` çağrısında ateşlendi.
>
> **Hook yürütme kabuğu farkı (ÖNEMLİ):** Native yol hook'u **PowerShell** ile çalıştırır
> (`execHook` → `powershell.exe -Command`), Claude Code ise Windows'ta **bash/sh** ile. Yani
> ham-PowerShell sözdizimli bir hook komutu (`$j=[Console]::In.ReadToEnd()|…`) native'de çalışır
> ama CLI'da bash altında `syntax error` verir (canlı E2E'de görüldü). **Kural:** CLI'da da
> çalışması gereken hook'u kabuk-bağımsız yaz — açık yorumlayıcı çağır (`powershell -NoProfile
> -File script.ps1`, sqz hook'unun yaptığı) veya POSIX sözdizimi kullan. Ham-PS rtk hook'ları
> bu yüzden CLI'da kırılır.
>
> **Kapatıldı (2026-07-31):** Bu uyarı yazılıydı ama **bozuk rtk şablonu gönderilmeye devam
> ediyordu**; WS10/SES63'te ajanın her Bash çağrısı bloke oldu (başarısız PreToolUse hook'u
> aracı durdurur). rtk hook şablonu **tamamen kaldırıldı** — rtk artık hook ile değil,
> `shellCommandRewrite` **ayarıyla** bağlanıyor (in-process filtre; ölçülmüş beyaz liste +
> `Degraded` koruması). Regresyon testi: `recommendations.test.ts` →
> *"never offers an rtk HOOK…"*. Detay: [17-TOKEN-OPTIMIZASYON.md](17-TOKEN-OPTIMIZASYON.md).
>
> Ders: bir riski **dokümante etmek yetmiyor** — riski üreten şablon kodda durdukça
> kullanıcı ona tek tıkla ulaşıyor. Uyarıyı yazarken şablonu da düzeltmek gerekirdi.
>
> **Lehçe köprüsü (2026-08-17):** Şablon kaldırıldı ama **eski hook kayıtları
> workspace store'larında duruyor** (5 workspace, 11 dosya) — yani "yaz-ve-uyar"
> hâlâ yetmiyordu. `writeCLISettings` artık komutu da çevirir: `cliHookCommand`
> (`climcp_hookcmd.go`), Windows'ta PowerShell **kaynak kodu** olan bir komutu
> (`$` ataması, `[Type]::Üye`, `&`/`.` çağrı operatörü, `Verb-Noun` cmdlet)
> `powershell.exe -NoProfile -NonInteractive -Command '<gövde>'` içine sarar —
> gövde POSIX tek-tırnakla kaçırılır, yani bash için tek kelime, PowerShell için
> orijinal metin. Yorumlayıcısını **zaten açıkça çağıran** komutlar
> (`sqz hook claude`, `powershell -File …`, `node hook.js`) **aynen geçer**;
> sarmalamak kendi tırnaklamalarını bozardı. stdin miras alındığı için payload
> sözleşmesi değişmez. Test: `climcp_hookcmd_test.go`.
>
> Matcher köprüsü (lehçe #1) ile birlikte bu, native↔CLI arasındaki **ikinci**
> sessiz lehçe farkını kapatır. Çalışma zamanı tarafı da sertleştirildi — bkz.
> yukarıdaki *"Interpreter hatası ≠ engelle"*: artık bozuk bir hook aracı
> susturamaz, yalnızca `debug.jsonl`'e hata yazar.



> **sqz uyarısı (Windows):** Ayarlar ▸ Dış Araçlar'daki tek-tık "Bağla" `Bash,PowerShell`
> matcher'ıyla hook kurar. Eski kurulumlarda matcher `Bash` kalmışsa PowerShell komutları
> sıkıştırılmadan geçer — hook'u düzenleyip matcher'ı `Bash,PowerShell` yapın (veya
> kaldırıp yeniden bağlayın). **rtk artık burada değil** — ayarla bağlanıyor (yukarı bak).

## Akış (toolloop.go)

```
model tool_use → [PreToolUse hooks] → permission gate → reg.Call(araç)
              → compactToolResult → [PostToolUse hooks] → modele tool_result
```

Her hook eylemi izde bir **`StepHook`** kartı olarak görünür (kalıcı):
`hook_block` / `hook_modify` / `hook_allow` / `hook_context`.

## API

Workspace-scoped (`X-Workspace-Id` header):
- `GET /api/hooks` — liste
- `POST /api/hooks` — oluştur (`event` PreToolUse|PostToolUse, `command` zorunlu)
- `PUT /api/hooks/{id}` — güncelle
- `POST /api/hooks/{id}/toggle` — aç/kapat
- `DELETE /api/hooks/{id}` — sil

## Frontend

- **Ayarlar → Hooks** (`settings/HooksPanel.tsx`): liste + ekle/düzenle
  (olay/eşleşme/komut/timeout/aktif) + toggle/sil. Kendi yükleme/kaydetme
  (global Kaydet çubuğundan muaf).
- Sohbet izi: `chat/HookStep.tsx` (🪝 / engelde ⛔ tonlu tek-satır kart).
- Adım türü referansı: Ayarlar → **Adım Türleri** → "Hook".

## Self-management (ajan araçları)

Ajan, hook'ları kendi de yönetebilir (self-management gated, lazy yüklenir;
`internal/tools/builtin_hookmgmt.go`):
- `list_hooks` — workspace'teki hook'ları döner (id/event/matcher/command/enabled +
  `createdByAgent` = silebilir mi).
- `create_hook` — yeni PreToolUse/PostToolUse hook (komut Claude Code hook
  sözleşmesini konuşur); `CreatedBy = actorID` ile etkin oluşturulur.
- `delete_hook` — **yalnız ajan-oluşturduğu** hook'u siler; `CreatedBy == ""`
  (kullanıcı tanımlı) ise reddedilir (provenance guard'ı).

## Dosyalar

| Katman | Dosya |
|--------|-------|
| Model | `internal/db/models_hook.go` |
| Store | `internal/db/store_hook.go` (+ `db.go` map/load/dir) |
| Motor | `internal/agent/hooks.go` |
| İz | `internal/agent/trace.go` (`StepHook`) |
| Entegrasyon | `internal/agent/toolloop.go` (pre/post çağrıları) |
| API | `internal/api/hooks.go` (+ `server.go` route) |
| Ajan araçları | `internal/tools/builtin_hookmgmt.go` (`list/create/delete_hook`, self-manage gated) |
| Test | `internal/db/store_hook_test.go`, `internal/agent/hooks_test.go` |
| Frontend | `types/hook.ts`, `api/hooks.ts`, `components/settings/HooksPanel.tsx`, `components/chat/HookStep.tsx`, `lib/stepKinds.ts` |

## Durum

✅ `go build/vet/test` + `tsc -b`/`vite build` yeşil. **Canlı API round-trip**
(izole instance, port 8099): create (type/createdAt varsayılanları) → geçersiz
event/boş komut 400 → update → toggle → kalıcı liste → delete uçtan uca
doğrulandı. Motor birim testleri (block/modify/allow/matcher) geçti.

**Kalan (ops.):** gerçek sağlayıcılı uçtan uca tur (bir araç çağrısını native
döngüde gerçekten engelleyen/dönüştüren canlı test); claude-cli için
`settings.json` hook üretimi (parite genişlemesi).
