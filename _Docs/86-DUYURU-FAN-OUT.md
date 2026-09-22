# 86 — Duyuru fan-out (release announcement)

> **Özet (2026-09-22):** Bir sürüm yayımlandıktan sonra duyuruyu **CI botu
> değil, TionHarness'in kendisi** yapar: bir schedule bir ajanı uyandırır, ajan
> `announce_release` aracını çağırır, araç `_Docs/release.json`'u okuyup Discord
> ve/veya Telegram'a kısa bir duyuru atar. Kendi otomasyonumuzu dogfood ederiz.
> Webhook adresleri **secret vault**'ta durur, araç argümanında değil. Durum:
> **uygulandı** (araç + render + teslim); schedule'ı kullanıcı kendi
> kanallarıyla kurar. Bir ajan için: duyuru zincirini (girdi dosyası, gizli
> anahtarlar, hedefler, hata davranışı) bu dosya tanımlar.

## 1. Neden CI botu değil

Yayın hattı (`_Docs/75`) zaten GitHub Actions'ta koşuyor; duyuruyu oraya bir
adım olarak eklemek en kısa yol olurdu. Tercih edilmedi:

- TionHarness'in **kendi** schedule + agent + tool zinciri tam olarak bu iş için
  var. Duyuruyu dışarıda bir bot yapsaydı, ürünün en görünür otomasyon
  senaryosunu ürünün kendisi çalıştırmıyor olurdu.
- Webhook adresleri birer **kimlik bilgisidir** (adresi olan proje adına mesaj
  atar). Vault'ta şifreli durmaları, CI secret'ı olarak durmalarından daha
  yakındır ve yerel/kendi barındırılan kurulumlarda da çalışır.
- Duyuru **yayından ayrı** tetiklenebilmeli: bir sürüm çıkıp duyurusu bir gün
  sonra atılabilir, ya da hiç atılmayabilir. Hattın içine gömülü bir adım bu
  esnekliği vermez.

## 2. Zincir

```
etiket → release.yml → _Docs/release.json (cmd/changelog write, _Docs/75)
                              │
                              ▼
                 schedule (cron) ──► agent ──► announce_release
                                                    │
                                        ┌───────────┴───────────┐
                                        ▼                       ▼
                                Discord webhook        Telegram Bot API
```

Girdi sözleşmesi **dosyadır**, Go tipi değil: `internal/announce`, changelog
üreticisinin tiplerini import etmez, `_Docs/release.json`'u kendi okur. İki
alt sistem böylece birbirinden bağımsız değişebilir.

## 3. `announce_release` aracı

| Alan | Anlamı |
|------|--------|
| `targets` | `discord` / `telegram`. Verilmezse **yapılandırılmış olan her kanal**. |
| `notes_url` | Tam sürüm notlarının adresi; mesajın sonuna eklenir. |
| `dry_run` | Mesajı üretip döndürür, **göndermez**. |

Kurallar:

- **Metin argüman değildir.** Araç `_Docs/release.json`'u okur. Bir modele
  changelog'u yeniden yazdırmak, er ya da geç bir sürüm numarasının
  başkalaştırılması demektir; duyuru gerçekten çıkanı anlatmalı.
- **Adresler vault'tan gelir**, argümandan değil: `ANNOUNCE_DISCORD_WEBHOOK`,
  `ANNOUNCE_TELEGRAM_API` + `ANNOUNCE_TELEGRAM_CHAT_ID`. Bir webhook adresi
  argüman olsaydı model bağlamına ve transkripte düşerdi.
- **Açıkça istenen ama yapılandırılmamış kanal hatadır**, atlanmaz. Sessizce
  daha az kanala duyurmak, bir sürümün kimse fark etmeden duyurusuz kalma yolu.
- **İdempotent değildir.** İki kez çağırmak iki kez duyurur; araç açıklaması
  bunu söyler. Kısmi başarıda hata mesajı **hangi kanallara çıktığını** söyler,
  böylece körlemesine yeniden deneme çift gönderime yol açmaz.
- Risk katmanı varsayılan `RiskWrite` (bilinmeyen araçların varsayılanı): salt
  okunur modda bloke, "ask" modunda onay ister. Dışarıya mesaj atan bir araç
  için doğru kapı.
- Görünürlük `VisibilityHidden`: tek bir görevde (sürüm çıkarken) gerekir ve
  schedule onu adıyla çağırır, diğer her ajanın kataloğunu şişirmesi gereksiz.

## 4. Mesaj biçimi

`internal/announce.Render` bir **manşet** üretir, changelog'un tamamını değil:

- Yalnız `BREAKING CHANGES`, `Features`, `Bug Fixes` bölümleri taşınır. Chores /
  style / test satırları tam notlara aittir; duyuruya girseler ilginç satırları
  uzunluk sınırının dışına iterlerdi.
- Bölüm başına en çok **5** giriş; kalanı "…and N more" olarak sayılır.
- Gövde **1800 rune** ile sınırlı: Discord'un sınırı 2000, Telegram'ınki 4096
  karakter; düşük olanın altında kalmak tek bir render'ı iki hedef için de
  geçerli kılar. Kırpma satır sınırından yapılır ve **notlar linki korunur** —
  kırpılan metnin işaret ettiği yer odur.
- Hafif Markdown (`**kalın**`, `- ` listesi) kullanılır; Discord bunu doğrudan,
  Telegram `parse_mode=Markdown` ile render eder. Hedef başına ayrı biçimlendirici
  yok, dolayısıyla ikisi arasında senkronda tutulacak bir şey de yok.

## 5. Teslim

`internal/announce.Post`:

- **Discord:** endpoint tam webhook adresidir, gövde `{"content": "..."}`.
- **Telegram:** endpoint bot token tabanıdır
  (`https://api.telegram.org/bot<token>`), sonuna `/sendMessage` eklenir,
  `chat_id` gövdede gider.
- **2xx dışı yanıt hatadır** ve sağlayıcının kendi mesajını taşır. Sessiz başarı,
  bir sürümün duyurulmadığı halde duyuruldu raporlanması demek olurdu.
- İstemci, WebFetch ve monitör kaynaklarıyla **aynı** SSRF korumalı taşıyıcıdır
  (`internal/tools/egress_guard.go`): yanlış kurulmuş bir secret aracı iç ağ
  tarayıcısına çeviremez.

## 6. Kurulum (kullanıcı tarafı)

1. Vault'a adresleri koy (Ayarlar ▸ Secrets ya da `secret` aracı):
   - `ANNOUNCE_DISCORD_WEBHOOK` = kanalın webhook adresi, ve/veya
   - `ANNOUNCE_TELEGRAM_API` = `https://api.telegram.org/bot<token>`
   - `ANNOUNCE_TELEGRAM_CHAT_ID` = `-100…` kanal/grup kimliği
2. Önce **`dry_run`** ile mesajı gör.
3. Bir schedule kur: ajan + cron + prompt, örneğin haftalık bir kontrol —
   "`_Docs/release.json` içindeki sürüm henüz duyurulmadıysa `announce_release`
   ile duyur". Duyurunun bir kez atılmasını sağlamak **prompt'un ve ajanın**
   işidir; araç idempotent değildir (§3).

## 7. Açık uçlar

- **Tekrar koruması yok.** Araç, bir sürümün daha önce duyurulup duyurulmadığını
  bilmez; bunu bilen bir durum dosyası (son duyurulan etiket) doğal bir sonraki
  adım. Bugün bu sorumluluk schedule prompt'undadır.
- **Hedef kümesi kapalı** (discord + telegram). Slack/Matrix eklemek
  `announce.Target` ve `Post` içinde birer dal demek; render ortak kalır.
- RSS kanalı (`website/src/pages/releases.xml.ts`, `/releases.xml`) bu
  zincirden **bağımsızdır**: o `latest.json`'dan üretilir ve okuyucu tarafı
  primitifidir; bu doküman push tarafını anlatır.
