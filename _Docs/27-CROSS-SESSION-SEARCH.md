# 27 — Oturumlar-Arası Tam-Metin Arama (CG-16)

> **Güncelleme (2026-07-11):** Çapraz-session farkındalığı artık **her zaman açık ve
> hiç ayarı yok** (diğer pull araçları gibi). `Session bağlamı` (master), `Her turda ver`
> ve `Listelenecek geçmiş session sayısı` (RecentCount) — hepsi kaldırıldı. Pushed özet
> bloğu daima yalnız session'ın **ilk turunda**, sabit 5 geçmiş session ile verilir.
> `list_sessions` artık **sayfalanır**: `offset` argümanı + yanıtta "Showing X–Y of Z"
> ile tüm sessionlar (aktif/geçmiş) gezilebilir; `archive_sessions`/`conversation_search`
> daima sunulur.
>
> **Durum (2026-06-22): Parça 1, 2, 3 TAMAMEN UYGULANDI — görsel arama dahil.**
> Sidebar arama kutusu artık yüklenen oturumlarda başlık + session ID'yi yerel,
> mesaj-içeriğini backend üzerinden arıyor; sonuca tıklayınca
> oturum açılıp ilgili mesaja kaydırılıp flash'lanıyor. Uygulama özeti dosyanın sonunda.
>
> **CLI köprüsü (2026-06-22):** `conversation_search` artık claude-cli ajanlarına da
> Interaction MCP üzerinden sunuluyor (eager built-in, native'de değişmeden kalır).
> Bkz. `11-INTERACTION-MCP.md`.
>
> **Roadmap maddesi:** `03-YOL-HARITASI.md` → **CG-16** (external-agent-oss P4).
> **Zemin olduğu iş:** **N5** (`conversation_search` aracı — bu araçla kapandı). Hafıza
> alt sistemi (HA-1 / MemGPT core memory) sonradan kaldırıldı; tarihsel plan:
> [arsiv/31-MEMGPT-CORE-MEMORY.md](arsiv/31-MEMGPT-CORE-MEMORY.md).
> İnceleme/karar: 2026-06-22.

## Amaç

Bir workspace'teki **tüm oturumların mesaj geçmişinde** anahtar-kelime/ifade
araması: hem kullanıcı için global arama kutusu, hem ajan için `conversation_search`
aracı. Bugün yalnız oturum **başlığı + rolling summary** üzerinden farkındalık var
(`list_sessions` tool + `sessionsContextBlock`); **mesaj gövdelerinde arama yok**.

## Kilit gerçek — neden ripgrep/indeks gerekmez

TionHarness dosya-tabanlı ama **boot'ta tüm oturumları belleğe yükler**:
`internal/db/store.go` içinde `d.messages map[sessionID][]Message` (her oturumun
`session.jsonl`'i `loadSessions` ile okunup RAM'e alınır, bkz. `08-DEPOLAMA.md`).
Yani aranacak veri **zaten bellekte**. Sonuç:

- **Birincil tasarım = saf-Go bellek-içi tarama.** Yeni indeks, yeni dosya okuma,
  yeni bağımlılık yok. En basit yol aynı zamanda yeterli yol.
- **ripgrep yalnızca** ileride *lazy oturum yükleme*ye geçilirse (oturumlar RAM'de
  değilse) anlam kazanır → şimdilik **kapsam dışı**, sadece not. Başlıktaki
  "ripgrep/Go" ikileminde **Go kazanıyor** çünkü veri hâlihazırda yüklü.
- FTS5 (HA-1) de aynı sebeple **şart değil**; ters-indeks ancak corpus RAM'e
  sığmayacak kadar büyürse gerekir. CG-16 onsuz teslim edilir, HA-1 üzerine
  ters-indeks **opsiyonel hızlandırma** olarak gelebilir.

```mermaid
graph TD
    BOOT["loadSessions (boot)<br/>session.jsonl → d.messages (RAM)"] --> MEM["d.messages<br/>map[sessionID]Message"]
    MEM --> SEARCH["db.SearchMessages<br/>(saf-Go tarama)"]
    SEARCH --> TOOL["conversation_search aracı<br/>(ajan — N5)"]
    SEARCH --> API["GET /api/sessions/search<br/>(kullanıcı — global arama)"]
    style SEARCH fill:#2d6,stroke:#093
```

---

> Parça 1–3 tasarım gövdesi, testler, fazlama ve doğrulama planı uygulandı (aşağıdaki "Uygulama notu" ile birebir) → [arsiv/27-CROSS-SESSION-SEARCH-PLAN.md](arsiv/27-CROSS-SESSION-SEARCH-PLAN.md).

## `conversation_search` ve `list_sessions` ayrıntıları

**Güçlendirme — birebir kurtarma (2026-06-24):** compact sonrası **kelime kelime** geri-getirme için araca
üç parametre eklendi (snippet tek başına kırpık olduğundan §17.10):

| Parametre | Etki |
|---|---|
| `full: bool` | Eşleşen mesajın **tam metni** birebir döner (snippet yerine). |
| `context: int` (0–5) | Her isabetin **N tur öncesi+sonrası** birebir eklenir; isabet `»»` ile işaretlenir. |
| `session_id: string` | Aramayı tek oturuma daraltır (`db.SearchOpts.OnlyID`). |

Tam/çevre metni `db.MessagesAround(sid, mid, before, after)` ile bellekteki transkriptten çekilir (LLM'siz).
Çıktı çok-satırlı: başlıkta `session_id` de var (ajan yeniden daraltabilsin). Header'da yaş hâlâ gösterilir.

**Kayıt:** `internal/agent/toolsetup.go` — `NewListSessionsTool`'un yanında,
**her zaman aktif** (cross-session context / `list_sessions` ile birlikte; toggle yok).
Lazy-load kataloğuna girebilir (`19-LAZY-TOOL-LOADING.md` deseni) — `activate_tools`
ile çekilir; sürekli prompt'ta durmasına gerek yok.

**`list_sessions` kapsam + sayfalama:** Araç **varsayılan olarak TÜM kind'leri**
listeler (chat + spawn/worker/flow/task/schedule) — eski `Kind=="chat"` sabit
filtresi kaldırıldı (2026-07-13), çünkü otonom koşular UI'nın sidebar/Overview'ında
görünürken ajanın `list_sessions`'ında görünmüyordu. Args: `state` (`active`|`all`,
vars. `active`) + **`kind`** (tek kind'e daralt; boş = hepsi) + `limit` (vars. 20) +
`offset` (vars. 0). Her satır `[kind·state]` ön ekiyle başlar. Yanıt sonunda
`Showing X–Y of Z` ve daha varsa `… pass offset:Y for the next page` — böylece tüm
sessionlar sayfa sayfa okunur.

## Kapsam ve sınırlar

- **Workspace-scoped** — `db` zaten per-workspace; çapraz-workspace arama yok
  (izolasyon felsefesi, `06-WORKSPACES.md`).
- **Eşleşme:** substring + AND (case-insensitive). Fuzzy/stemming/dizimsel arama
  **yok** (gerekirse HA-1 ters-indeksiyle gelir).
- **Alanlar:** `Message.Text` (birincil) + opsiyonel `ReasoningContent`. `ToolCalls`/
  `Steps` JSON'u **hariç** (gürültü); istenirse opsiyon eklenir.
- **Snippet** salt-okunur; arama sonucu mesajı değiştirmez.

## Uygulama notu (2026-06-22)

Parça 1, 2 ve Parça 3'ün backend/contract'ı sevk edildi. Plandan sapma yok.

- **Çekirdek:** `db.SearchMessages` (`internal/db/store_search.go`) — saf-Go RAM-içi
  tarama, AND-terim eşleşme, `score = matchCount + recency` (C5 felsefesi),
  rune-sınırlı snippet. `SearchHit`/`SearchOpts` tipleri. Test: `store_search_test.go`
  (AND, rol filtresi, ExcludeID, limit, Türkçe snippet UTF-8 güvenliği).
- **Ajan aracı (N5):** `tools.ConversationSearchTool` (`builtin_conversation_search.go`),
  `conversation_search` — `ListSessionsTool` deseni. `toolsetup.go`'da artık
  **her zaman aktif** (list_sessions ile aynı; 2026-07-11'de toggle kaldırıldı). Test:
  `builtin_conversation_search_test.go`. **Not:** plandaki `exclude_current`
  düşürüldü — `tools`→`agent` import döngüsü olurdu; API tarafında `exclude` query
  param'ı ile karşılanıyor.
- **API:** `GET /api/sessions/search?q=&limit=&role=&exclude=`
  (`api/sessions_search.go`), `server.go`'da `active`'in yanında kayıtlı (literal
  segment, `{id}` ile çakışmaz). `[]SearchHit` döner.
- **Frontend contract:** `SearchHit` tipi (`types/session.ts`) +
  `sessionApi.searchMessages(q, {limit,role,exclude})` (`api/sessions.ts`).
  `tsc --noEmit` temiz.
- **Doğrulama:** `go build ./...` + db/tools/api/agent testleri (200) yeşil.
- **Görsel arama (2026-06-22, ID filtresi 2026-08-31):** `SessionsSidebar` arama kutusu
  artık çift işlevli — yüklenenlerde case-insensitive başlık/session ID substring filtresi
  + `api.searchMessages` ile mesaj-içeriği araması (≥2 char,
  250ms debounce, `cancelled` guard'lı). "Mesajlarda (N)" bölümü rol-rozeti + snippet +
  yaş ile listeler; tıklayınca `onSelectSession(sessionId, messageId)`. App `selectSession`
  artık opsiyonel `messageId` taşır → `scrollToMsgId` state → `MessageList`. `MessageList`
  her satırı `data-msg-id` ile sarmalar; `highlightMessageId` değişince hedefe
  `scrollIntoView({block:'center'})` + 1.6sn accent-ring flash, sonra `onHighlightConsumed`
  ile temizlenir. `tsc --noEmit` + `npm run build` + `go build ./...` yeşil.

## Ayrıca bakınız

- **`03-YOL-HARITASI.md`** — CG-16 (bu plan), HA-1 (FTS/kullanıcı modelleme),
  CG-18 (session labels/batch — arama ile bütünleşir).
- **`arsiv/31-MEMGPT-CORE-MEMORY.md`** — tarihsel: N5 (`conversation_search`) kökeni;
  hafıza alt sistemi kaldırıldı.
- **`08-DEPOLAMA.md`** — `session.jsonl` + boot `loadSessions` → `d.messages` (RAM).
- **`19-LAZY-TOOL-LOADING.md`** — aracın talep-üzerine yüklenmesi.
