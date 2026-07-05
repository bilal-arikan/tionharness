# 18 — Hooks (PreToolUse / PostToolUse) — Faz P4

> Kullanıcı-tanımlı dış komutların, native araç döngüsünde her araç çağrısının
> etrafında çalışması. Claude Code hook sözleşmesiyle uyumlu — aynı script'ler
> (ör. **sqz**) değiştirilmeden çalışır.

## Ne işe yarar?

- **PreToolUse:** araç çalışmadan **önce** çalışır. Komut, çağrının girdisini
  yeniden yazabilir (`updatedInput`), çağrıyı otomatik onaylayabilir (izin
  kapısını atlar), veya **engelleyebilir** (modele hata sonucu döner).
- **PostToolUse:** araç çalıştıktan **sonra** (ve token sıkıştırmasından sonra)
  çalışır. Komut, çıktıyı dönüştürebilir (`updatedOutput` — ör. dış sıkıştırma),
  bağlam ekleyebilir (`additionalContext`), veya sonucu engelleyebilir.

Tipik kullanım: denetim/audit log, güvenlik politikası (tehlikeli komutu blokla),
otomatik onay kuralları, dış araç-çıktısı sıkıştırma (sqz).

## Kapsam: yalnız native yol

Kancalar **yalnız native (anthropic/minimax) araç döngüsünde** uygulanır
(`agent/toolloop.go`). **claude-cli** kendi döngüsünü sürer ve kendi
hook'larını `~/.claude/settings.json`'dan okur — TionSwarm hook'ları oraya yazılmaz.
Bu, Faz P3'teki (permission) "CLI vs native ayrımı" deseninin aynısıdır.

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

> **sqz/rtk uyarısı (Windows):** Ayarlar ▸ Dış Araçlar'daki tek-tık "Bağla" artık
> `Bash,PowerShell` matcher'ıyla hook kurar. Eski kurulumlarda matcher `Bash` kalmışsa
> PowerShell komutları sıkıştırılmadan geçer — hook'u düzenleyip matcher'ı
> `Bash,PowerShell` yapın (veya kaldırıp yeniden bağlayın).

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
