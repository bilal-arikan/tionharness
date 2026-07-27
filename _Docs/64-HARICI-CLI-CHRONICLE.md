# 64 — an external CLI agent `/chronicle` Oturum İçgörü Ailesi

> *Numara notu: bu doküman 2026-07-27'de **59 → 64** olarak yeniden numaralandı
> (59, CLI steer planı dokümanıyla çakışıyordu).*

> **Salt REFERANS — talimat yürütmez.** Bu doküman an external CLI agent'nin
> `/chronicle` komut ailesini açıklar ve TionSwarm'ın mevcut yetenekleriyle
> kıyaslar. **Kod değişikliği tanımlamaz**; TionSwarm'a benzer bir özellik
> *eklemek* ayrı bir karttır (bkz. son bölüm "Boşluklar / Fikirler").
>
> **Durum notu (2026-07-11):** `/chronicle` GitHub tarafında **deneysel/aktif
> gelişen** bir özelliktir; buradaki komut isimleri ve davranışları yazıldığı
> tarihteki resmi dokümana ve changelog'a dayanır, ileride değişebilir. Kaynaklar
> dosyanın sonundadır.

## `/chronicle` Nedir?

`/chronicle`, an external CLI agent'nin **oturumlar-arası içgörü** komut ailesidir.
Amacı: kullanıcının geçmiş CLI oturumlarını analiz ederek özet çıkarmak, kişisel
çalışma alışkanlıklarını tespit etmek ve prompt/talimat kalitesini iyileştirecek
öneriler üretmektir. Tek tek oturum transkriptini okumaya gerek kalmadan "ne
üzerinde çalıştım, nerede takıldım, hangi özellikleri kaçırıyorum" sorularına
cevap verir.

### Yerel Session Store (SQLite)

external CLI agent CLI, oturum verisini makinede **yerel bir SQLite veritabanında**
("session store") saklar; varsayılan olarak ayrıca kullanıcının GitHub hesabına
**senkronize eder** (cihazlar arası erişim için). Session store, disk üzerindeki
tam oturum dosyalarının **yapılandırılmış bir alt kümesidir** ve `/chronicle`
komutlarını besler. Store bozulursa veya güncel değilse `/chronicle reindex` ile
disk dosyalarından yeniden inşa edilir.

Store'da tutulan başlıca alanlar (yazıldığı tarihteki gözleme göre):

| Kategori | İçerik |
|----------|--------|
| Metadata | Depo (repo), branch, çalışma dizini (cwd), timestamp |
| Konuşma turları | Kullanıcı ve asistan mesajları (özet/kesme uygulanır — kullanıcı mesajları ~1000, asistan mesajları ~5000 karaktere kadar) |
| Dokunulan dosyalar | Oturumda okunan/yazılan dosya yolları |
| Harici referanslar | Bağlanan pull request / issue / URL referansları |

> **Not:** Karakter limitleri ve alan listesi resmi dokümanda ince ayrıntısıyla
> yayımlanmaz; bu tablo kart araştırması + gözleme dayanır ve sürümle değişebilir.

## `/chronicle improve` — Prompt & Talimat İyileştirme

`improve`, oturum geçmişinde **external CLI agent'un zorlandığı yerleri** (tekrar eden
hatalar, yanlış araç seçimleri, kullanıcının defalarca verdiği aynı yönergeler)
tespit eder ve bunları kalıcı hâle getirmek için **custom instructions**
(`.github/copilot-instructions.md`) dosyasına eklenecek öneriler üretir. Böylece
her oturumda tekrarlanan düzeltmeler bir kez talimat dosyasına yazılır.

**Örnek akış:**

```
> /chronicle improve

Analiz: Son 12 oturumda tekrar eden desenler bulundu.

1. PR açıklaması formatı — 4 oturumda "başlığı Conventional Commits
   formatında yaz" düzeltmesini elle verdin.
   Öneri (.github/copilot-instructions.md):
     "Pull request başlıklarını Conventional Commits formatında yaz
      (feat:/fix:/docs:...)."

2. Import sırası — 3 oturumda standart kütüphane / üçüncü-parti / yerel
   ayrımını elle düzelttin.
   Öneri: "Go import bloklarını std / external / internal olarak grupla."
```

## `/chronicle tips` — Araç-Kullanım Analizi & Kaçırılan Özellikler

`tips`, son oturumları inceleyerek **nasıl çalıştığını** anlar (gerçek
prompt'ların, kullandığın araçlar, henüz denemediğin özellikler) ve mevcut CLI
yeteneklerinin tamamıyla çapraz-referans yaparak **3–5 kişisel öneri** üretir.
Öneriler jenerik değil, senin gerçek kullanım verine dayanır — örneğin bir URL
içeriğini yapıştırmak yerine `/research` (gelişmiş web+GitHub araması) kullanmayı
hatırlatabilir.

**Örnek çıktı:**

```
> /chronicle tips

Kullanımına göre 3 öneri:

1. Dosya içeriğini yapıştırmak yerine `@dosya` ile referans ver
   (son 8 oturumda 20+ kez içerik yapıştırdın → bağlam israfı).
2. Keşif işleri için `/research` dene — URL'leri elle açıp yapıştırmak
   yerine web+GitHub'da doğrudan arama yapar.
3. Tekrarlayan prompt'larını özel bir agent'a dönüştür.
```

## Diğer Alt Komutlar (bütünlük için)

| Alt komut | Ne yapar |
|-----------|----------|
| `/chronicle standup` | Son 24 saat (veya "last 3 days") çalışma özeti; branch bazında gruplar, tamamlanma durumuna göre ayırır, bağlı PR/issue'ların güncel durumunu kontrol eder. |
| `/chronicle cost-tips` | Token harcama desenlerini analiz eder + maliyet düşürme önerileri verir. |
| `/chronicle search` | Tüm oturum içeriğinde doğrudan anahtar-kelime araması yapar. |
| `/chronicle reindex` | Session store'u disk dosyalarından yeniden inşa eder (store bozulduğunda/eskidiğinde). |

**Önerilen günlük ritim:** Güne başlarken `/chronicle standup last 3 days` ile
son çalışmayı hatırla; iki haftada bir `/chronicle tips` ile kaçırdığın özellik
ve iş akışı iyileştirmelerini keşfet.

## TionSwarm Karşılığı

| external CLI agent `/chronicle` | TionSwarm muadili | Not |
|----------------------|-------------------|-----|
| Session store (SQLite) | `session.jsonl` + `debug.jsonl` (dosya-tabanlı; DB yok) | TionSwarm'da veri dosya-başına JSONL; ayrı bir "içgörü store"u yok — transkript + debug günlüğü ham veri. Detay `08`, `38`. |
| `improve` (tekrar eden hata → talimat önerisi) | Hata→ders döngüsü (`lessons.jsonl`, `read_lessons`/`delete_lesson`) | En yakın muadil: kötü tur → ucuz-model reflection → ders; en yeni 5 ders dinamik suffix'e enjekte edilir. Fark: TionSwarm dersi **otomatik** üretir ve **prompt'a** ekler; external CLI agent `improve` **kullanıcıya** talimat-dosyası **önerisi** sunar. Detay `56`. |
| `tips` (kaçırılan özellik/araç önerisi) | **Muadil yok (boşluk)** | TionSwarm'da kullanıcının araç-kullanım desenini analiz edip proaktif "şunu kullanmayı dene" önerisi üreten bir katman yok. |
| `standup` (dönemsel çalışma özeti) | **Kısmi:** Aktivite ekranı + `SessionsOverview` + kalıcı ilerleme (`progress.json`) | Ham malzeme var; otomatik "son N gün özeti" üreteci yok. Detay `36`. |
| `search` (oturum içeriğinde arama) | `conversation_search` (`builtin_conversation_search.go`) | Birebir muadil: oturumlar-arası tam-metin arama. Detay `27`. |
| `cost-tips` (token harcama analizi + öneri) | Tasarruf Merkezi (prompt-cache USD) + `sqz`/`rtk` | Ölçüm ve optimizasyon araçları var; otomatik "harcamayı şöyle azalt" öneri üreteci yok. Detay `17`. |

## Boşluklar / Fikirler (gelecek kart tohumu)

TionSwarm'da `chronicle`-benzeri **proaktif içgörü üreteci** eksik. Uygulanırsa:

1. **`session_tips` aracı** — kullanıcı/ajanın son N oturumundaki araç-kullanım
   dağılımını (`debug.jsonl` tool adımlarından) çıkarıp, hiç kullanılmayan mevcut
   araçları (registry ↔ kullanım farkı) 3–5 öneri olarak sunmak.
2. **`session_standup` aracı** — cwd/branch + tamamlanan görev/tur özetini
   (`progress.json` + `session.jsonl`) dönemsel raporlamak.
3. **`improve`-benzeri talimat önerisi** — mevcut ders döngüsünü (`lessons.jsonl`)
   tersine çevirip, tekrar eden dersleri kullanıcının **workspace config prompt**
   dosyasına önerilecek kalıcı kurallar hâline getirmek (otomatik enjeksiyon
   yerine kullanıcı-onaylı).
4. **`cost-tips` üreteci** — billing/aggregation verisinden pahalı desenleri
   (uzun context, düşük cache-hit) tespit edip somut aksiyon önerisi.
5. **İçgörü store'u** — bu üreteçler için `debug.jsonl` üstüne hafif bir
   indeks/özet katmanı (SQLite şart değil; dosya-tabanlı özet yeterli).

> Bu maddeler **fikir**dir; TSK30 yalnız bu dokümanı ister. Herhangi birini
> uygulamak ayrı bir karttır.

## Kaynaklar

- [Using an external CLI agent session data — GitHub Docs](https://docs.github.com/en/copilot/how-tos/copilot-cli/use-copilot-cli/chronicle)
- [About an external CLI agent session data — GitHub Docs](https://docs.github.com/en/copilot/concepts/agents/copilot-cli/chronicle)
- [Gain insights across your agent sessions with /chronicle — GitHub Changelog (2026-06-02)](https://github.blog/changelog/2026-06-02-gain-insights-across-your-agent-sessions-with-chronicle/)
- [Query session history with chronicle — VS Code Docs](https://code.visualstudio.com/docs/agents/sessions/session-insights)
- [an external CLI agent | /chronicle to Improve Your Prompting Style — rajeevpentyala.com](https://rajeevpentyala.com/2026/04/11/github-copilot-cli-chronicle-to-improve-your-prompting-style/)
