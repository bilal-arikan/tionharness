# TionHarness — Ajanlar-Arası Peer Mesajlaşma Planı (SendMessage / Mailbox)

> **Durum:** **Faz 1 UYGULANDI (2026-06-23).** Claude Code'un gözlemlenen
> swarm/teammate davranışından çıkarılan **adresli mailbox** deseninin
> TionHarness'e uyarlanması.
>
> **Faz 1–2 + Faz 3-broadcast (tamam):** `send_message({to, message, summary?})`
> built-in (self-manage gated); teslim = alıcının kalıcı **inbox** oturumuna
> (`GetOrCreateKindSession`, kind="inbox") `<agent_message from="…" summary="…">…</agent_message>`
> etiketli mesaj + **arka planda** alıcının geçmiş-duyarlı turu (`runSessionTurn` →
> wake-runner; SpawnMaxConcurrent guard). **Yanıt:** `to`=gönderenin `from` adı.
> **Broadcast:** `to="*"` → tüm diğer ajanlar (`broadcastAgentMessage`, best-effort).
> Kendine-mesaj reddi. Dosyalar: `internal/agent/agentmsg.go`,
> `internal/tools/builtin_sendmessage.go`, `internal/agent/toolsetup.go`; testler:
> `sendmessage_test.go`. **Kalan:** Faz 3 UI inbox göstergesi + grafik kenarı, Faz 4 (ertelendi).

## 1. Neden / Bağlam

Çok-ajanlı "kim ne dedi" sorunu (SES29) iki yolla çözülebilir:

- **Paylaşılan-thread + etiketleme** (TionHarness'in seçtiği yol): birden çok ajan tek
  sohbete yazar; geçmişte her tur yazarıyla etiketlenir (`chat_authors.go`) ve ardışık
  aynı-rol turlar birleştirilir (`providers/coalesce.go`). ✅ Yapıldı.
- **İzole bağlam + adresli mailbox** (Claude Code'un yolu): her ajan kendi
  transcript'inde çalışır; iletişim **`SendMessage({to, message})`** ile, alıcının
  inbox'ına `from` kimliğiyle düşer. Plain çıktı diğer ajana görünmez. Kimlik doğuştan
  vardır → ne etiketleme ne rol-çakışması sorunu olur.

**Bugünkü TionHarness durumu:** Eski `send_agent_message` / `call_agent` / `spawn_session`
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

- `formatAgentMessage` → `internal/agent/agentmsg.go` (uygulandı; ayrı `tools/agentmsg_format.go` yok):
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
2. ✅ **Faz 2 (2026-06-23)** — yanıt ergonomisi: alıcı, mesajdaki `from` adını `to` yapıp
   `send_message` ile yanıtlar (araç açıklamasında açık talimat); inbox turu zaten
   geçmiş-duyarlı (`runSessionTurn`, Faz 1).
3. ⏳ **Faz 3 (kısmi, 2026-06-23)** — ✅ **broadcast `"*"`** (`broadcastAgentMessage`,
   en iyi-çaba + slot guard, test geçti). ⏳ **kalan:** UI inbox göstergesi +
   ilişki grafiğine `messaged` kenarı (frontend-ağırlıklı; ertelendi — inbox oturumları
   şimdilik Aktivite feed'inde görünür).
4. ⏳ **Faz 4** — yapısal protokol mesajları (görev atama/durum) — **ertelendi** (opsiyonel;
   şimdilik `run_task`/Kanban yeterli).
5. ✅ **Faz 5 (TSK340, 2026-08-28)** — **alıcı tarafı politikası + teslim makbuzu +
   byte sınırı**. Ayrıntı: §9.

## 7. Açık kararlar (kullanıcı onayı bekliyor)

1. **Tetik:** teslimde **hemen tur** mu (heartbeat yok → en pratik), yoksa kuyruk +
   scheduled işleme mi?
2. **Inbox modeli:** per-ajan **tek** "inbox" oturumu mu, yoksa gönderen başına ayrı
   thread mi? (Tek inbox + from-tag daha basit.)
3. **Ayrı araç mı, `run_subagent`'a mod mu?** Öneri: **ayrı `send_message`** (semantik
   net; run_subagent sonuç-odaklı kalır).

## 8. Tahmini dosya dokunuşları

- `internal/tools/builtin_sendmessage.go` (yeni araç) + `internal/agent/agentmsg.go` (from-tag biçimleme)
- `internal/agent/toolsetup.go` (self-manage bloğuna ekle)
- `internal/agent/agentmsg.go` → `DeliverAgentMessage(ctx, fromAgentID, toRef, summary, message)` (inbox + spawn)
- `internal/tools/builtin_sendmessage_test.go`
- Docs: bu plan + [[05-ILERLEME]] + [[23-ILISKI-GRAFIGI]]

---
*Desen kaynağı: Claude Code'un dışarıdan gözlemlenen mesajlaşma davranışı (kod kopyalanmadı, yalnız desen).
İlgili: [[25-SUBAGENT-ISOLATION]] · [[22-SPAWN-SESSION]] · [[07-CHAT-UX]].*

---

## 9. Alıcı tarafı: inbound politikası, makbuz ve byte sınırı (Faz 5)

Faz 1–3'te gönderen her zaman kazanıyordu: mesaj ya teslim ediliyor ya da yalnız
geçici bir araç hatasıyla düşüyordu; alıcının söz hakkı yoktu ve düşen mesajdan
kalıcı bir iz kalmıyordu. Faz 5 üç boşluğu kapatır.

### 9.1 Inbound politikası — `accept | hold | refuse`

- Saklandığı yer: **ajanda** `Agent.InboundPolicy`, **oturumda**
  `Session.InboundPolicy` (`internal/db/models.go`). Oturum ayarı **varsa** o kazanır,
  yoksa ajanınki, o da boşsa `accept`.
- **Boş değer = "ayarlanmamış" = `accept`.** Bu yüzden mevcut ajan/oturum satırları
  hiç dokunulmadan eski davranışı sürdürür — geriye dönük uyum bozulmaz.
- Bilinmeyen bir değer **sessizce accept'e düşmez**: `ValidateInboundPolicy` hata
  döndürür, `UpdateAgent` / `SetSessionInboundPolicy` yazmayı reddeder ve teslim
  sırasında çözümleme hata verir (`internal/db/models_agentmsg.go`).
- Etkisi:
  - `accept` → eskisi gibi teslim + arka plan turu.
  - `hold` → mesaj **gövdesiyle birlikte** park edilir, tur başlatılmaz; onay bekler.
  - `refuse` → teslim reddedilir, gönderen hatayı görür, red kalıcı olarak yazılır.
- Inbox (`send_message`) yolunda oturum, teslim anında yaratıldığı için politika
  **ajan** düzeyinde okunur; oturum override'ı zaten var olan oturumlar için (worker
  oturumları, `send_to_worker`) geçerlidir.

### 9.2 Teslim makbuzu — `accepted | held | refused | dropped`

`db.AgentMessage` (`internal/db/models_agentmsg.go`, store:
`internal/db/store_agentmsg.go`, disk: `<store>/agent-messages/AMS<n>.json`) her
teslim **denemesi** için yazılır. Sessiz düşme yoktur:

| Durum | Anlamı |
|---|---|
| `accepted` | Politika kabul etti, teslim yapıldı |
| `held` | Politika `hold`; gövde saklandı, teslim edilmedi, onay bekliyor |
| `refused` | Politika `refuse` (ya da tutulan mesaj elle reddedildi) — gerekçe makbuzda |
| `dropped` | Kabul edildi ama teslim **sonradan** başarısız oldu (slot tükendi, kuyruk dolu, DB hatası) |

- `accepted → dropped` düşürmesi `dropDelivery` ile yapılır; asıl hata olduğu gibi
  gönderene döner, üstüne makbuz kimliği eklenir (`internal/agent/inbound.go`).
- `held → accepted/refused` geçişi **CAS**'tır (`ResolveHeldAgentMessage`): aynı mesaj
  iki kez serbest bırakılıp iki kez çalıştırılamaz.
- Makbuzlar diskte durduğundan onay bekleyen mesaj **yeniden başlatmayı da atlatır**.

### 9.3 Tutulan mesajları görme / serbest bırakma

HTTP (`internal/api/agent_messages.go`):

- `GET  /api/agent-messages/held?agentId=…` — onay bekleyenler (eskiden yeniye).
- `POST /api/agent-messages/{id}/release` — onayla ve **şimdi** teslim et.
- `POST /api/agent-messages/{id}/refuse` — reddet (`{"reason": "..."}` opsiyonel).

Serbest bırakma, mesajın yakalandığı kanala göre yeniden teslim eder: `inbox` →
alıcının inbox oturumu, `worker` → worker oturumu (meşgulse worker kuyruğuna girer).

### 9.4 Byte üst sınırı ve `message_too_large`

Ayar: `agentMessageMaxKB` (varsayılan **64 KB**, `internal/settings/settings.go`);
çalışma zamanı karşılığı `Tunables.AgentMessageMaxBytes()`
(`DefaultAgentMessageMaxBytes`). Üç yolda da uygulanır:

| Yol | Nerede | Davranış |
|---|---|---|
| `send_message` | `gateInbound` → `checkMessageSize` (`internal/agent/inbound.go`) | Sınır aşılırsa **hata**: `message_too_large`; makbuz yazılmaz, teslim denenmez |
| `send_to_worker` | `SendToWorker` içindeki `gateInbound` (`internal/agent/coordination.go`) | Aynı: kuyruk slotu bile alınmaz |
| Worker bildirimi | `NotifyCoordinator` → `capNotification` | **Kırpılır**, atılmaz: turu biten worker'a hata döndürecek kimse yok; kırpma `message_too_large` işaretiyle açıkça yazılır (rune-güvenli kesim) |

Testler: `internal/agent/inbound_test.go` (politika matrisi, byte sınırı, makbuz
durumları, CAS'lı serbest bırakma, bildirim kırpması) ve
`internal/db/store_agentmsg_test.go` (politika doğrulama + makbuz yaşam döngüsü +
yeniden açılışta kalıcılık).
