---
name: "Koordinatör (Çoklu-Ajan)"
description: "Bir koordinatör oturumunun paralel worker'ları async yönetmesi: spawn_worker ile fan-out, <task-notification> ile geri bildirim, send_to_worker ile devam, stop_worker ile durdurma. Araştırma→sentez→uygulama→doğrulama döngüsü. M1 (run_subagent sync), M3 (send_message peer) ve M4 (flow) yöntemleriyle ne zaman hangisini seçeceğini de açıklar."
when_to_use: "Uzun veya çok-fazlı bir işi birden çok ajana paralel dağıtıp sonuçları tek merkezden sentezlemek istediğinde; koordinatör turlar boyunca canlı kalıp fan-out + sentez + doğrulama yapmalı"
icon: "🧭"
color: "#0ea5e9"
access: shared
auto_summary: false
---
# Koordinatör Modu — Çoklu-Ajan Koordinasyonu

Bu skill, TionSwarm'daki **koordinatör/worker** desenini (M2) ve onunla birlikte
kullanılabilen diğer koordinasyon yöntemlerini öğretir. Ayrıntılı tasarım:
`_Docs/47-KOORDINATOR-COKLU-AJAN.md`.

> **Koordinatör olmak için** oturumun `Role = "coordinator"` olmalıdır (Composer'daki
> Koordinatör rozeti veya oturum ayarından). Yalnız o zaman `spawn_worker` /
> `send_to_worker` / `stop_worker` / `list_workers` araçları görünür.

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

- **Her mesajın kullanıcıya.** `<task-notification>`'lar iç sinyaldir; onlara teşekkür etme.
- **Fan-out süper gücün.** Bağımsız worker'ları tek turda başlat, sonra turu bitir. Sonuçları **tahmin etme/uydurma** — bildirim gelince yeni tur açılır.
- **Sentezi SEN yap.** "Based on your findings" YASAK — bulguları oku, dosya:satır içeren net spec yaz.
- **Yazma-ağır işleri sıraya koy.** Aynı dosya kümesine aynı anda iki worker yazmasın; araştırma paralel serbest.
- **Worker görevleri self-contained olmalı** — worker senin konuşmanı görmez; dosya yolu, satır, hata mesajı, "bitti" tanımı ver.
- **Continue vs. spawn:** bağlam örtüşmesi yüksek → `send_to_worker`; düşük/temiz gerek → `spawn_worker`; doğrulama → her zaman taze `spawn_worker`.
- **Gerçek doğrulama:** özelliği açıp test et; "var" demek yetmez.

## 4. Workflow desenleri — göreve göre seç ve **kombinle**

Aşağıdaki altı desen, yukarıdaki M1–M4 mekanikleri üstünde koştuğun **stratejilerdir**
(yeni araç gerektirmez). Münhasır değildirler — tek işte birkaçını zincirle
(ör. fan-out ile araştır → adversarial ile doğrula → sentezle).

| Desen | Ne yapar | Nasıl (mekanik) | Ne zaman |
|-------|----------|-----------------|----------|
| **Fanout-And-Synthesize** | Alt-görevlere böl, her dala bir worker, sonuçları birleştir | `spawn_worker` ×N (M2) veya `run_subagent` ×N (M1) → SEN sentezle | Derin araştırma: N kaynağı paralel tara → tek rapor |
| **Adversarial Verification** | Bir worker'ın çıktısını **ikinci bir worker** kırmaya/çürütmeye çalışır | Çıktıyı taze `spawn_worker(reviewer)`'a ver; "refute et, rubber-stamp etme" | İddiaları fact-check, kod/plan doğrulama |
| **Loop Until Done** | Durma koşulu sağlanana dek yeni worker spawn et | Notify geldikçe `spawn_worker`; `CoordinatorMaxTurns` guard'ı | "Yeni bulgu var mı? → devam" (Ralph-loop); GAN-loop skill'i de bunu yapar |
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

- `spawn_worker` hedefi var olan bir ajan **veya** bir profil (`explore`/`coder`/
  `reviewer`) olabilir; profil verilirse kalıcı, yeniden-kullanılabilir bir
  `worker:<profil>` ajanına otomatik materyalize edilir. (Anlık, senkron alt-görev
  için hâlâ `run_subagent` (M1) daha uygun.)
- **Ortak scratchpad:** koordinatör ve tüm worker'lar aynı paylaşılan dizini görür
  (context'te "Shared scratchpad" olarak verilir). Worker'lar arası kalıcı bulguları/
  planları her göreve tekrar yazmak yerine oraya küçük dosyalar (findings.md, plan.md)
  olarak yazın.
- Worker oturumları koordinasyon araçlarını **göremez** → worker worker spawn edemez (recursion engeli).
- Koordinatör başına aktif worker sayısı (`CoordinatorMaxWorkers`) ve otomatik koordinatör
  tur sayısı (`CoordinatorMaxTurns`) sınırlıdır; limit dolunca yeni bildirimler kaydedilir
  ama otomatik tur tetiklenmez (manuel devam edebilirsin).
