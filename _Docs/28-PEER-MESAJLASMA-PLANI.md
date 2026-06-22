# SwarmGo — Ajanlar-Arası Peer Mesajlaşma Planı (SendMessage / Mailbox)

> **Durum:** **Faz 1 UYGULANDI (2026-06-23).** Claude Code `swarm/teammate`
> incelemesinden (`observed-behavior`) çıkarılan **adresli mailbox** deseninin
> SwarmGo'ya uyarlanması. Kavramsal arka plan: [[10-KAVRAMSAL-TASARIM-NOTLARI]] §10.
>
> **Faz 1 (tamam):** `send_message({to, message, summary?})` built-in (self-manage
> gated); teslim = alıcının kalıcı **inbox** oturumuna (`GetOrCreateKindSession`,
> kind="inbox") `<agent_message from="…" summary="…">…</agent_message>` etiketli
> kullanıcı mesajı + **arka planda** alıcının geçmiş-duyarlı turu (`runSessionTurn` →
> wake-runner; SpawnMaxConcurrent guard). Kendine-mesaj reddi. Dosyalar:
> `internal/agent/agentmsg.go`, `internal/tools/builtin_sendmessage.go`,
> `internal/agent/toolsetup.go`; testler: `sendmessage_test.go`. **Kalan: Faz 2–4.**

## 1. Neden / Bağlam

Çok-ajanlı "kim ne dedi" sorunu (SES29) iki yolla çözülebilir:

- **Paylaşılan-thread + etiketleme** (SwarmGo'nun seçtiği yol): birden çok ajan tek
  sohbete yazar; geçmişte her tur yazarıyla etiketlenir (`chat_authors.go`) ve ardışık
  aynı-rol turlar birleştirilir (`providers/coalesce.go`). ✅ Yapıldı.
- **İzole bağlam + adresli mailbox** (Claude Code'un yolu): her ajan kendi
  transcript'inde çalışır; iletişim **`SendMessage({to, message})`** ile, alıcının
  inbox'ına `from` kimliğiyle düşer. Plain çıktı diğer ajana görünmez. Kimlik doğuştan
  vardır → ne etiketleme ne rol-çakışması sorunu olur.

**Bugünkü SwarmGo durumu:** Eski `send_agent_message` / `call_agent` / `spawn_session`
araçları kaldırılıp **`run_subagent`** altında birleştirildi (`toolsetup.go`).
`run_subagent` = *izole görev delege et + sonucu al* (sync) veya *arka plana detach et*
(async → `SpawnSession`). **Boşluk:** bir ajanın başka bir **bağımsız** ajana
**adresli, kimlikli bir mesaj** gönderip onu kendi oturumunda sürdürmesi — Claude
Code'un `SendMessage`'ı gibi — yok.

## 2. Hedef

`send_message` built-in aracı:

```json
{ "name": "send_message",
  "input": { "to": "Researcher", "summary": "1. görevi ata", "message": "task #1'e başla" } }
```

- **`to`**: ajan **adı** (UUID değil). İleride `"*"` = broadcast (maliyet uyarısıyla).
- **`summary`**: 5–10 kelimelik UI/önizleme metni.
- Teslim: alıcı ajanın **inbox oturumuna** kimlik etiketiyle yazılır —
  `<agent_message from="Gönderen" summary="…">…</agent_message>` — ve alıcının bir turu
  tetiklenir (wake/spawn deseni). Alıcı "kim, ne istedi" görür; yanıtı `to: <from>` ile
  döndürebilir.
- **Plain çıktı diğer ajana görünmez** (prompt notu) — iletişim için araç zorunlu.

## 3. Üzerine kurulacak mevcut altyapı

| Parça | Konum | Kullanım |
|---|---|---|
| Kalıcı per-ajan kind oturumu | `db.GetOrCreateKindSession(agentID,"inbox","📥 Inbox")` | Alıcının inbox thread'i |
| Bağımsız oturumda tur | `Runtime.SpawnSession` / `SpawnSessionTool` | Teslimde alıcıyı çalıştır (async, bütçe-gated) |
| Geçmiş-duyarlı tur | `WakeTurnFunc` / `invokeTraced` | Alıcı inbox geçmişiyle yanıtlasın |
| Yazar etiketleme | `api/chat_authors.go` | inbox'taki from-tag ile doğal birleşir |
| Spawn guard'ları | `SpawnMaxConcurrent` / `SpawnMaxPerTurn` | Mesaj-seli / döngü koruması |

## 4. Tasarım

### 4.1 Teslim akışı

```
send_message(to, message, summary)
  └─ hedef ajanı ADIYLA çöz (yoksa hata)
  └─ inbox = GetOrCreateKindSession(target, "inbox", "📥 Inbox")
  └─ AddMessage(inbox, role=user,
                text = formatAgentMessage(from=Self, summary, message))
  └─ SpawnSession/invoke(target, inbox)   # async, autonomous, budget-gated
  └─ return "X'e teslim edildi (inbox)"
```

- `formatAgentMessage` → `internal/tools/agentmsg_format.go`:
  `<agent_message from="Ada" summary="…">…</agent_message>` (Claude Code'un
  `<teammate_message teammate_id=…>` muadili).
- Teslim **fire-and-forget**: gönderen beklemez (heartbeat kalktığı için tetik =
  teslim anı). Yanıt, alıcının ayrı bir `send_message({to: <from>})` çağrısıyla geri
  gelir.

### 4.2 Kimlik / "kim ne dedi"

inbox oturumunda mesajlar zaten `from`-etiketli; ayrıca `labelMultiAgentHistory`
çoklu-yazar durumunda ek etiket sağlar. Yani peer yolu kimlik sorununu **doğuştan**
çözer (paylaşılan-thread'deki sonradan-etiketlemeye gerek kalmaz).

### 4.3 Guardrail

- **Self-manage gated** (diğer self-yönetim araçları gibi).
- **Sel/döngü:** mevcut `SpawnMaxConcurrent`/`SpawnMaxPerTurn` guard'ları teslimi
  sınırlar; A→B→A pinpon için basit derinlik/oran sayacı (delegation guard deseni).
- **Provenance:** teslim log'u (`from`, `to`, `summary`).
- Broadcast `"*"` ekip boyunda lineer → açık maliyet uyarısı, opsiyonel.

## 5. `run_subagent` ile ilişki (ne zaman hangisi)

| | `run_subagent` | `send_message` (yeni) |
|---|---|---|
| Model | İzole görev delege + sonuç dön | Akran ajana adresli DM; o kendi oturumunda sürer |
| Bağlam | isolated/inherited, geçici | alıcının **kalıcı inbox** oturumu |
| Dönüş | sync sonuç / async fire-forget | fire-forget; yanıt ayrı mesajla geri gelebilir |
| Tipik kullanım | "şunu araştır, bana getir" | "şu işi al, bağımsız ilerle, gerekirse bana yaz" |

İkisi **bir arada** durur: delegasyon (sonuç-odaklı) vs. peer mesajlaşma (süregelen
işbirliği).

## 6. Fazlar

1. ✅ **Faz 1 (2026-06-23)** — `send_message` aracı + inbox teslim (fire-on-deliver) +
   `from`-tag + geçmiş-duyarlı alıcı turu. Test geçti.
2. **Faz 2** — yanıt ergonomisi (`to: <from>`), inbox geçmiş-duyarlı tur (WakeTurnFunc deseni).
3. **Faz 3** — broadcast `"*"`, UI inbox göstergesi, ilişki grafiğine `messaged` kenarı.
4. **Faz 4** — yapısal protokol mesajları (görev atama / durum) — gerekirse; aksi halde
   `run_task`/Kanban ile yapılır.

## 7. Açık kararlar (kullanıcı onayı bekliyor)

1. **Tetik:** teslimde **hemen tur** mu (heartbeat yok → en pratik), yoksa kuyruk +
   scheduled işleme mi?
2. **Inbox modeli:** per-ajan **tek** "inbox" oturumu mu, yoksa gönderen başına ayrı
   thread mi? (Tek inbox + from-tag daha basit.)
3. **Ayrı araç mı, `run_subagent`'a mod mu?** Öneri: **ayrı `send_message`** (semantik
   net; run_subagent sonuç-odaklı kalır).

## 8. Tahmini dosya dokunuşları

- `internal/tools/builtin_sendmessage.go` (yeni araç) + `agentmsg_format.go` (from-tag)
- `internal/agent/toolsetup.go` (self-manage bloğuna ekle)
- `internal/agent/*` → `DeliverAgentMessage(target, from, summary, text)` (inbox + spawn)
- `internal/tools/builtin_sendmessage_test.go`
- Docs: bu plan + [[10-KAVRAMSAL-TASARIM-NOTLARI]] + [[05-ILERLEME]] + [[23-ILISKI-GRAFIGI]]

---
*Kaynak inceleme: `observed-behavior` (salt-okunur; kod kopyalanmadı, yalnız desen).
İlgili: [[25-SUBAGENT-ISOLATION]] · [[22-SPAWN-SESSION]] · [[07-CHAT-UX]].*
