---
name: "Koordinatör (Çoklu-Ajan)"
description: "MEKANİK (nasıl çağrılır), politika DEĞİL: koordinatör/worker araçlarının somut kullanımı — spawn_worker ile fan-out, <task-notification> ile geri bildirim, send_to_worker ile devam, stop_worker ile durdurma, guardrail'ler (worker/derinlik sınırları) ve alt-koordinatör derinliği. M1 (run_subagent sync), M3 (send_message peer), M4 (flow) arasından hangi yöntemi seçeceğini de açıklar. Kartın ne zaman done olacağına dair KARAR kuralları için: orchestrator-doctrine."
when_to_use: "Araçların kendisini kullanman gerektiğinde: koordinatör modunu açmak, worker spawn etmek/durdurmak, bildirimleri işlemek, hangi koordinasyon yönteminin (M1–M4) uygun olduğunu seçmek, bir guardrail hatasını çözmek. Yalnız 'kartı done'a taşıyabilir miyim / doğrulamayı kime veririm' gibi disiplin sorusu için bunu YÜKLEME."
icon: "🧭"
color: "#0ea5e9"
access: shared
auto_summary: false
---
# Koordinatör Modu — Çoklu-Ajan Koordinasyonu

Bu skill, TionHarness'teki **koordinatör/worker** desenini (M2) ve onunla birlikte
kullanılabilen diğer koordinasyon yöntemlerini öğretir. Ayrıntılı tasarım:
`_Docs/47-KOORDINATOR-COKLU-AJAN.md`.

> **Koordinatör olmak için** oturumun koordinatör modu açık olmalıdır — oturum
> panelindeki toggle, `spawn_worker(coordinator: true)` ile açılmış olmak, ya da
> kendi `set_coordinator_mode(enabled: true)` çağrın. Yalnız o zaman `spawn_worker` /
> `send_to_worker` / `stop_worker` / `list_workers` araçları görünür. Kendi kendine
> açtığında araçlar **bir sonraki turda** gelir (bu turun araç seti donmuştur).

## 1. Dört koordinasyon yöntemi — hangisi ne zaman?

| Yöntem | Araç | Ne zaman |
|--------|------|----------|
| **M1 — Parallel fan-out (sync)** | `run_subagent` ×N (tek turda) | Kısa, bağımsız alt-görevler; cevabı **bu turda** istiyorsun (araştırma taraması). Sonuç anında döner. |
| **M2 — Koordinatör/İşçi (async)** | `spawn_worker` / `send_to_worker` / `stop_worker` / `list_workers` | Uzun/çok-fazlı iş; turlar boyunca canlı kalıp fan-out + sentez + doğrulama. Sonuç `<task-notification>` ile geri gelir, yeni koordinatör turu **otomatik** başlar. |
| **M3 — Takım/Peer** | `send_message` (+ inbox) | Merkezî koordinatör yok; eşdüzey ajanlar birbirine mesaj atarak işbirliği yapar. |
| **M4 — Flow** | Akışlar (graf) | LLM koordinatörü değil, **deterministik** sabit graf: paralel + branch node'ları. |

Aynı koordinatör oturumunda M1 (hızlı senkron bakış) ile M2 (uzun async iş) **birlikte**
kullanılabilir.

## 2. M2 çalışma döngüsü

```
1. spawn_worker ×N   → bağımsız worker'ları TEK turda fan-out et, turu bitir
2. <task-notification> gelir → yeni koordinatör turu otomatik başlar
3. Bulguları SEN sentezle (dosya:satır içeren spesifik spec yaz)
4. send_to_worker (bağlam örtüşüyorsa) veya yeni spawn_worker (temiz bağlam)
5. Doğrulama worker'ı ile kanıtla → kullanıcıya özetle
```

## 3. Altın kurallar

- **Araç çağrısı = tek gerçeklik.** Bir worker'dan bahsetmeden ÖNCE o tur `spawn_worker`'ı ÇAĞIRMIŞ ol; mevcut worker'lara atıf yapmadan önce `list_workers` çağır. Düz metinde "worker başlattım / 3 worker açtım / round 2 açıldı" demek — aynı turda eşleşen araç çağrısı olmadan — HİÇBİR ŞEY yaratmaz: worker yoktur ve gelmeyecek bir sonucu bekleyerek donarsın (stall). Spawn'ı anlatmak spawn etmek değildir.
- **Her mesajın kullanıcıya.** `<task-notification>`'lar iç sinyaldir; onlara teşekkür etme.
- **Fan-out süper gücün.** Bağımsız worker'ları tek turda başlat, sonra turu bitir. Sonuçları **tahmin etme/uydurma** — bildirim gelince yeni tur açılır.
- **Sentezi SEN yap.** "Based on your findings" YASAK — bulguları oku, dosya:satır içeren net spec yaz.
- **Yazma-ağır işleri sıraya koy.** Aynı dosya kümesine aynı anda iki worker yazmasın; araştırma paralel serbest.
- **Worker görevleri self-contained olmalı** — worker senin konuşmanı görmez; dosya yolu, satır, hata mesajı, "bitti" tanımı ver.
- **Çalışma dizini argümandır, prose değil.** Worker senin cwd'ini miras alır; başka bir depoda çalışması gerekiyorsa `spawn_worker(cwd: "C:\\...\\Repo")` ver. Görev metnine "Depo: C:\\..." yazmak worker'ı oraya taşımaz — yanlış dizinde `go build` "does not contain main module" ile patlar.
- **Continue vs. spawn:** bağlam örtüşmesi yüksek → `send_to_worker`; düşük/temiz gerek → `spawn_worker`; doğrulama → her zaman taze `spawn_worker`.
- **`send_to_worker` sırası:** worker boştaysa mesaj hemen teslim edilir. Worker hâlâ önceki turunu işliyorsa mesaj **tek-slotluk kuyruğa** alınır (`queued`) ve tur biter bitmez otomatik teslim edilir — kaybolmaz. Ama **worker başına yalnız bir bekleyen mesaj** tutulur; ikinci bir mesaj gönderirsen **reddedilir**. Meşgul diye `stop_worker` **çağırma** (çalışan işi çöpe atar). Paralellik istiyorsan **farklı worker'lara dağıt**, aynı worker'a mesaj yığma.
- **Gerçek doğrulama:** özelliği açıp test et; "var" demek yetmez.

### Kapsam dışı bulguyu karta çıkar

Bir kartı yürütürken kapsam dışında eksik veya hatalı bir şey görürsen mevcut işi
durdurma. Önce `list_tasks` ile aynı konuyu taşıyan açık kart var mı kontrol et;
yoksa `boardState: "pbi"` ve `tags: ["scope-out", "parent:<buKartınId>"]` ile ayrı
kart aç. Soy bağı için `dependencies` kullanma: bu alan kartı bağımlı olduğu kart
bitene kadar bloke eder; çoğu spin-off bağımsız çalışabilir. `dependencies` yalnız
gerçek blokaj varsa kullanılır. Ana kartın işini bitirmeden yan bulgunun peşine düşme.

## 4. Workflow desenleri — göreve göre seç ve **kombinle**

Aşağıdaki altı desen, yukarıdaki M1–M4 mekanikleri üstünde koştuğun **stratejilerdir**
(yeni araç gerektirmez). Münhasır değildirler — tek işte birkaçını zincirle
(ör. fan-out ile araştır → adversarial ile doğrula → sentezle).

| Desen | Ne yapar | Nasıl (mekanik) | Ne zaman |
|-------|----------|-----------------|----------|
| **Fanout-And-Synthesize** | Alt-görevlere böl, her dala bir worker, sonuçları birleştir | `spawn_worker` ×N (M2) veya `run_subagent` ×N (M1) → SEN sentezle | Derin araştırma: N kaynağı paralel tara → tek rapor |
| **Adversarial Verification** | Bir worker'ın çıktısını **ikinci bir worker** kırmaya/çürütmeye çalışır | Çıktıyı taze `spawn_worker(reviewer)`'a ver; "refute et, rubber-stamp etme" | İddiaları fact-check, kod/plan doğrulama |
| **Loop Until Done** | Durma koşulu sağlanana dek yeni worker spawn et | Notify geldikçe `spawn_worker`; tur sayısı sınırsız olduğundan durma koşulunu **sen** tanımla (bkz. §5) | "Yeni bulgu var mı? → devam" (Ralph-loop); GAN-loop skill'i de bunu yapar |
| **Classify-And-Act** | Görevi türüne göre doğru worker/yola yönlendir | Önce sınıflandır → `spawn_worker(target=profil)` ile doğru profile (explore/coder/reviewer) yönlendir | Karışık istekleri kategoriye ayırma |
| **Generate-And-Filter** | Çok seçenek üret, rubric + dedupe ile en iyileri süz | Fan-out ile N aday üret → SEN kendi bağlamında rubric'le ele | Beyin fırtınası: 10 fikir → en güçlü 3 |
| **Tournament** | Adaylar ikişerli yargılarla elenir → kazanan | Ardışık `spawn_worker(judge)` turları; coalescing biriktirir | En iyi tek çözümü seçmek |

Not: İlk üçü (Fanout / Adversarial / Loop) M2 döngüsüyle **doğrudan** eşleşir; son
üçü (Classify / Generate-Filter / Tournament) aynı araçlarla **prompt-seviyesinde**
kurulur.

**Kayıtlı recipe'ler (M5):** Bu 6 desen kutudan çıkan `coordinator-wf-*` recipe'leri
olarak saklıdır. Koordinatör Composer'ındaki **Workflow seçici** ile birini seçince
gövdesi bu sisteme enjekte edilir, önerilen worker hedefleri + stop condition +
`max_turns` uygulanır. Kendi recipe'ini `kind: coordinator-workflow` frontmatter'lı
bir skill olarak yazıp ekleyebilirsin.

## 5. Sınırlar (guardrail)

- `spawn_worker` hedefi var olan bir ajan **veya** bir profil (`explore`/`planner`/
  `coder`/`reviewer`/`validator`) olabilir; profil verilirse kalıcı, yeniden-kullanılabilir
  bir `worker:<profil>` ajanına otomatik materyalize edilir. **`planner`** kodu okuyup
  uygulanabilir bir plan (GOAL/FILES/STEPS/VERIFY/RISKS) döner, düzenleme yapmaz —
  planner → coder → validator zincirini hazır kurmak için
  `coordinator-wf-plan-dev-test` reçetesini kullan. **`validator`** kodu düzenlemeden
  test/typecheck/build/e2e çalıştırıp kompakt PASS/FAIL verdict döner — doğrulamayı ona
  delege et, diff'leri/logları kendi context'ine çekme. Testleri ve commit'i implementer
  worker yapar (commit yalnız validator PASS sonrası); sen sadece verdict okur, yönlendirirsin.
  Uzun bir worker çıktısı context'i şişirmesin diye cap'lenir ve tamamı bir artifact'a
  taşınıp bildirimde handle olarak geçer. (Anlık, senkron alt-görev
  için hâlâ `run_subagent` (M1) daha uygun.)
- **Ortak scratchpad:** koordinatör ve tüm worker'lar aynı paylaşılan dizini görür
  (context'te "Shared scratchpad" olarak verilir). Worker'lar arası kalıcı bulguları/
  planları her göreve tekrar yazmak yerine oraya küçük dosyalar (findings.md, plan.md)
  olarak yazın.
- **Gerçekte bağlayan iki sınır var:** koordinatör başına aktif worker sayısı
  (`CoordinatorMaxWorkers`, varsayılan 8, tavan 64) ve ağaç derinliği
  (`CoordinatorMaxDepth`, varsayılan 5, tavan 12). Bunlara takılan bir spawn
  **hata verir**, sessizce düz worker'a düşmez.
- **Otomatik tur sayısı ve ağaç geneli worker bütçesi artık sınırsızdır.**
  `normalize` her yükleme/kaydetmede `CoordinatorMaxTurns` ve
  `CoordinatorMaxSubtreeSessions` değerlerini **-1'e (sınırsız) sabitler**
  (`internal/settings/store.go:689` ve `:702`); ayarlar arayüzündeki iki girdi de
  kaldırılmıştır. Yani bu ikisi pratikte hiçbir zaman devreye girmez: notify
  döngüsü tur sayısı yüzünden durmaz, ağaç genelinde toplam worker bütçesi
  yüzünden spawn reddedilmez. Daha önce sonlu bir değer kaydedilmiş
  workspace'lerde de ilk okumada sınırsıza çevrilir.
- **Ağaç bütçesi kapalı olduğu için `Tree budget:` satırı da çıkmaz.** Bütçe
  değerlendirmesi sınırsızda (`<= 0`) hemen dönüyor
  (`evalCoordinatorTreeBudget`, `internal/agent/coordination.go:583`), dolayısıyla
  `spawn_worker` sonucunda `Tree budget: N/M live …` satırını **bekleme** ve
  spawn'ın ağaç bütçesi yüzünden reddedilmesini planlama. Fan-out'u sen
  sınırlarsın: `CoordinatorMaxWorkers` (eşzamanlı) + `CoordinatorMaxDepth`
  (derinlik) dışında duracak yer yok, o yüzden durma koşulunu görevin kendisine
  yaz.
- **Stall sert-halt:** düz metinde worker uydurmak (araç çağrısı olmadan) fantom-spawn
  stall'ına düşürür; nudge bütçesi bitip yargıç hâlâ stall doğrularsa koordinatör
  **otomatik-turlamayı bırakır** ve sana tek-seferlik `coordination` bildirimi gider.
  UI'da kırmızı **"Koordinatör durduruldu"** rozeti + **"Devam ettir"** butonu çıkar;
  gerçek bir koordinasyon aracı çağrısı da halt'ı temizler. Kaçınmak için §3 altın
  kuralına uy: worker'dan bahsetmeden ÖNCE `spawn_worker` çağır.

## 6. Derinlik — alt-koordinatörler

`spawn_worker(coordinator: true)` ile açtığın worker senin yetkilerini alır: görevini
kendi worker'larına bölebilir. Sınırsız derinlikte iç içe geçebilir.

- **Ne zaman:** alt-görev gerçekten bağımsız parçalara ayrılıyorsa ("şu 4 alt sistemi
  taşı", her biri kendi içinde birkaç dosya). **Varsayılan yapma** — her seviye tur,
  token ve gecikme çarpar; işi yapan düz bir worker, işi bir kez daha devreden bir
  alt-koordinatörden daima iyidir.
- **Bir alt-koordinatör dağıtım yaparken sana hiçbir şey göndermez.** Ondan tek bir
  mesaj alırsın: dalı bittiğinde gelen `<task-notification>`. O ana kadar durum
  bloğunda **DELEGATING** olarak görünür — bu bir durumdur, sonuç değildir. Boş boş
  bekleme, diğer işlerine bak.
- **Sen bir alt-koordinatörsen:** turunun bitmesi işinin bittiği anlamına gelmez.
  Sentezini tamamlayınca `report_to_coordinator(summary, status)` çağır — görevini
  yukarı kapatan tek şey budur. Tıkandıysan da `incomplete`/`failed` ile çağır;
  sessiz kalmak üstündeki tüm ağacı bekletir. Özeti **kendin yaz**: koordinatörün
  senin worker'larının oturumlarını okuyamaz.
- `send_to_worker` yalnız **kendi doğrudan** worker'larına gider; bir alt-koordinatörün
  worker'ları ona aittir. `list_workers(scope: "subtree")` ile tüm dalını görebilirsin.
- Bir alt-koordinatörü `stop_worker` ile durdurmak **tüm dalını** durdurur.
