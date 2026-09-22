# 03 — Yol Haritası: Kaldırılan Bölümler (Arşiv)

> **Özet (2026-09-22):** `03-YOL-HARITASI.md`'den 2026-09-22 doküman temizliğinde taşınan
> tarihsel bölümler: Faz 6 Memory, bellek maddeleri C3/C5/C6 ve HA-1. Hepsi 2026-07-05'te
> kaldırılan memory alt sistemine (Reflect, lexical cosine recall, `human` çekirdek bloğu)
> dayanıyordu; bu özellikler artık kodda yoktur. Yalnız tarihsel referanstır.

## Faz 6 — Memory ✅ → **KALDIRILDI (2026-07-05)**
> ⚠️ Memory alt sistemi (journal recall + core memory + hafıza grafiği + ilgili tool/API/UI/veri)
> projeden **tamamen çıkarıldı**. Aşağısı tarihsel kayıttır; bu özellikler artık yoktur.
- [x] ~~`internal/memory`: doküman + journal + reflection~~ — paket depodan **silindi**
- [x] Recall: **embedding yerine saf Go lexical cosine** (anahtarsız/çevrimdışı; embedding ileride takılabilir)
- [x] Dream cycle (`Reflect`) — journal'ı provider'a özetletip reflection üret
- **Çıktı:** Hatırlayan, yansıtan ajanlar; sohbet+göreve otomatik enjeksiyon. ✅

> ⚠️ **Bellek (C3/C5/C6) — KALDIRILDI (2026-07-05):** memory alt sistemi tamamen çıkarıldı;
> aşağıdaki bellek maddeleri artık geçersiz tarihsel kayıttır. Bağlam maddeleri (compaction/handoff) geçerli.
- ~~**C5 / C6 / C3**~~ — recency+importance ağırlıklı recall, MemGPT tarzı
  self-editing çekirdek bellek ve memdir benzeri `memory_write` indeksleme.
  **Hepsi 2026-07-05'te düştü** (memory alt sistemi kaldırıldı). Tarihsel tasarım:
  [`arsiv/31-MEMGPT-CORE-MEMORY.md`](31-MEMGPT-CORE-MEMORY.md).

- [ ] **HA-1 — Gelişmiş hafıza: tam-metin arama + LLM özet + kullanıcı modelleme** *(yüksek değer)*:
  the external agent hafızası üç katman taşıyor — (a) **FTS5 tam-metin arama** oturumlar üzerinde (TionHarness'te
  mevcut **CG-16** ile örtüşür; Go tarafında ripgrep veya bleve/saf-Go ters-indeks ile, DB-siz
  felsefeye uygun), (b) **LLM-destekli özetleme** ile çapraz-oturum recall (TionHarness'te `Reflect`
  dream-cycle + rolling summary kısmen var; oturumlar-arası kalıcı özet indeksine genişletilir),
  (c) **Honcho-benzeri kullanıcı modelleme** — etkileşimlerden kalıcı kullanıcı profili çıkarma
  (tercihler/bağlam/davranış). TionHarness'in mevcut lexical-cosine recall'ı (Faz 6) bunun altyapısı;
  üzerine kalıcı kullanıcı-profili entity'si + oto-güncelleme eklenir. İlişkili: **CG-16**, **C3** (memory_write).
  > ✅ **(c) kullanıcı modelleme TAMAMLANDI (2026-06-23):** `human` çekirdek bloğu dream-cycle'a
  > piggyback eden bir geçişle journal'dan otomatik doldurulur (C6 / `31-MEMGPT-CORE-MEMORY.md` Parça 4b).
  > Kalan: (a) FTS5 tam-metin arama + (b) çapraz-oturum kalıcı özet indeksi.
