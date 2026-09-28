# 89 — Akışı durdurmadan kullanıcıya soru sorma

> **Özet (2026-09-28):** `ask_user_async`, native ve Claude/Codex CLI sohbetlerinde
> bir veya birkaç soruyu kalıcı olarak açar ve hemen döner. Ajan cevaptan bağımsız
> işleri sürdürebilir. Cevap sonraki uygun model/araç sınırında iletilir; tur bittiyse
> aynı oturumun seri mesaj kuyruğuna girer. Arayüz birden fazla soruyu ayrı ayrı
> gösterir. `ask_user`, işlem izinleri ve plan onayları mevcut bekleme davranışını korur.

## Güncel sistemler hakkında araştırma

28 Eylül 2026 tarihinde resmi belgeler kontrol edildi:

| Sistem | Belgelenmiş davranış | TionHarness açısından sonuç |
|---|---|---|
| OpenAI Responses | Function/custom araçlarda `async: true`; sonuç orijinal `call_id` ile daha sonra gönderilir. Model bağımsız muhakeme ve araç çağrılarını sürdürebilir. Kullanıcı sorusu ve `wait_for_tasks` örnekleri uygulama tarafından tanımlanır. Belgede GPT-6 Astra ve sonrası desteği belirtiliyor. | Protokol düzeyindeki destek sağlayıcıya/model sürümüne bağlıdır. Tek başına arayüz kartı göstermek gerçek asenkron araç sonucunun yerini tutmaz. |
| Codex App Server | İstek, cevap ve `serverRequest/resolved` yaşam döngüsü; soru isteklerinde isteğe bağlı `autoResolutionMs`; tur sürerken yönlendirme (`turn/steer`). | Soru kimliği, kapanma bildirimi ve tur yaşam döngüsü birlikte yönetilmelidir. App Server özelliği `codex exec` entegrasyonuna otomatik taşınmaz. |
| Claude Agent SDK | `AskUserQuestion`, `canUseTool` üzerinden soru/seçenekleri istemciye taşır ve cevabı bekler. | Callback'in asenkron olması, ana ajanın aynı soru çağrısını beklemeden ilerlediği anlamına gelmez. |
| Claude Code alt ajanları | Arka planda paralel işler ve araç filtreleri bulunur. Normal alt ajanlarda `AskUserQuestion` sunulmaz; fork davranışı ayrıca tanımlanmıştır. | Arka plan ajanı ile kullanıcı sorusunun yaşam döngüsü ayrı kavramlardır. |

Kaynaklar:

- [OpenAI: Async tool calling](https://developers.openai.com/api/docs/guides/async-tool-calling)
- [Codex App Server](https://learn.chatgpt.com/docs/app-server)
- [Claude: Handle approvals and user input](https://code.claude.com/docs/en/agent-sdk/user-input)
- [Claude: Create custom subagents](https://code.claude.com/docs/en/sub-agents)

## Seçilen entegrasyon

TionHarness'in tüm sağlayıcılarına Responses protokolündeki `async` alanı eklenmedi.
Yeni araç normal bir çağrı olarak **soruyu yayınlama işlemini** tamamlar; gerçek
kullanıcı cevabı sonradan gelen ayrı bir girdidir. Böylece native sağlayıcılar ve
mevcut CLI Interaction MCP köprüsü aynı özelliği kullanır. Araç cevabı açıkça
`status: pending` ve `request_id` içerir; soru gösterildi diye cevap üretilmez.

```json
{
  "question": "Who is the report for?",
  "options": ["Engineers", "Executives"]
}
```

Aynı çağrıda `questions` dizisi ile birden fazla soru da sorulabilir. Ajan:

1. `ask_user_async` çağırır ve kimliği alır.
2. Cevaptan bağımsız çalışmayı sürdürür.
3. Gelen cevabı kimliği ve soru metniyle birlikte işler.
4. Sadece cevaba bağımlı işler kaldıysa turu bitirir; kullanıcı cevabı devamı başlatır.

Tekrar soru sorma veya periyodik durum sorgusu gerekmez. Sessizlik onay sayılmaz.
İşlem izni için `request_confirmation` / permission akışı kullanılmalıdır.

## Yaşam döngüsü ve dosyalar

- Araç ve bağlam sözleşmesi: `internal/tools/ask_async.go`, `builtin_ask_async.go`.
- Kalıcı kayıt: mevcut `SessionAsk` üzerinde `async` ve `answerDelivered` alanları;
  `store_session_ask_async.go` yalnız ilgili oturum/ajan cevaplarını tek tüketiciye verir.
  Eski kayıtların eksik alanları `false` okunur; mevcut suspend/resume değişmez.
- Native teslim: `internal/agent/steer.go`, bir sonraki sağlayıcı isteğinden önce
  cevabı kullanıcı girdisi olarak ekler; tur izinde de saklar.
- CLI teslim: Interaction MCP cevabında araç çıktısından ayrı metin blokları.
  İzin callback'inin JSON sözleşmesine ek yapılmaz. CLI kendi araçlarıyla çalışır ve
  TionHarness'e tekrar çağrı yapmazsa cevap tur sonunda sıraya alınır.
- Tur sonu: `internal/api/ask_async.go`, henüz tüketilmemiş cevapları aynı oturumun
  kuyruğuna ekler. Cevap/tur-sonu yarışı finalizasyon işareti ve atomik tüketimle korunur.
- Kullanıcı cevabı: mevcut `/sessions/{id}/interactions/{iid}/answer`; workspace ve
  oturum eşleşmesi doğrulanır, ilk cevap kazanır. Asenkron soru için eski snapshot
  yeniden oynatılmaz.
- Arayüz: `pendingInteractions.ts` ve `InteractionPrompts.tsx`; kartlar kimlikle
  eşlenir, bir cevabın kapanması diğer soruları silmez. Normal tur sonu soruyu
  kapatmaz. Ağ hatasında kart ve yazılan cevap yerinde kalır. Asenkron kartlar odağı
  yazı yazılan alandan çalmaz.

## Oturum sürerken genel yönlendirme

Soru cevabından bağımsız bir düzeltme veya yeni kısıt da **Yönlendir** ile mevcut
tura gönderilebilir. Claude/Codex CLI'nin `auto`, `ask` ve `read-only` modlarında,
Interaction MCP köprüsü bağlıysa bu destek açıktır. Mesajlar sırayla tüketilir;
asenkron soru cevaplarıyla aynı araç yanıtında ayrı metin blokları olarak iletilebilir.

**Sıraya** sonraki turu bekletir; **Yönlendir** sonraki uygun model/MCP sınırında
mevcut tura katılır; **Kes** turu durdurup yenisini başlatır. TionHarness'e başka araç
çağrısı gelmeden CLI turu biterse teslim edilemeyen yönlendirme sıraya aktarılır.
Ayrıntılar: [59 — Canlı yönlendirme](59-CLI-STEER-PLANI.md).

## Sınırlar

- Özellik interaktif sohbetler içindir. Başsız scheduler/flow/worker çağrılarına
  kullanıcı sorusu aracı açılmaz.
- Bekleyen sorular diskte kalır ve yeniden bağlantıda geri gösterilir. Açık sorular
  durdurma işlemiyle iptal edilir. Bu sürüm kendiliğinden varsayılan cevap seçmez.
- CLI'lerde teslim noktası sonraki **TionHarness MCP çağrısıdır**; uzun süren tek
  bir subprocess/model çağrısı anında kesilip yönlendirilmez.
- Kalıcı tüketim işareti ile modelin cevabı gerçekten işlemesi arasında dağıtık bir
  işlem yoktur. Tam o aralıkta süreç çökerse cevap kayıtta bulunur ama otomatik
  tekrar teslim garantisi yoktur. Mevcut durable resume da benzer bir sınır taşır.

## Doğrulama

Regresyon testleri; soru sonrası bağımsız native araç adımı, sonraki model isteğine
cevap enjeksiyonu, CLI köprüsünde ayrı cevap bloğu, eşzamanlı tek tüketim, yeniden
açılışta teslim işareti, yanlış oturuma cevap reddi, çift cevap, tur-sonu yarışı,
durdurma ve birden fazla kartın replay/tur sonu boyunca korunmasını kapsar.

Tam test kapısı (`scripts/test.sh full`) geçti: Go testleri, 147 dosyada 1042
frontend testi, depcheck ve diff kontrolü. Son backend yaşam döngüsü düzeltmesinden
sonra `internal/api` paketinin tamamı ayrıca geçti. TypeScript kontrolü ve frontend
üretim derlemesi başarılı. Gerçek React
bileşenleriyle hazırlanan yerel tarayıcı önizlemesinde iki kart, devam eden sayaç,
kimlikle cevap eşleşmesi, taslak koruma ve 375 px görünüm doğrulandı; bu önizleme
canlı bir model/CLI uçtan uca deneyi değildir. Ek ESLint kontrolü, değişiklik
öncesinde de aynı olan `ChatView.tsx` `currentTodo` memoization bağımlılığına takıldı.
