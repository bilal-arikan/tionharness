# 49 — Mobil / responsive UI: uygulama öncesi analiz ve plan (§1–§6)

> Arşiv: `49-MOBIL-RESPONSIVE-UI.md` dosyasından taşınan, tamamlanmış plan/tasarım metni. Güncel durum için asıl dokümana bak.

## 1. Motivasyon

`48-VPS-REMOTE-CLIENT` telefondan (PWA/WebView) uzak TionHarness'e bağlanmayı
tarifliyor. Ama mevcut UI **yatay masaüstü düzeni** varsayıyor: soldan sağa
`NavRail → Sidebar → main → DetailPanel` diye 3-4 sütun yan yana diziliyor.
360-420px genişlikteki bir telefonda bu düzen ezilir, sütunlar okunamaz hale
gelir. Uzak istemci ancak UI **tek-sütun + navigasyonla katmanlı** dikey akışa
indirgenirse işe yarar.

**İyi haber:** temel altyapı hazır — `index.html` zaten doğru viewport meta'sını
taşıyor (`width=device-width, initial-scale=1.0`, `index.html:6`) ve Tailwind v4
breakpoint'leri mevcut; yalnızca **kullanılmıyorlar**.

### 1.1 Genel bulgu: uygulama şu an "responsive değil"

`sm:`/`md:`/`lg:`/`xl:` breakpoint'leri tüm `frontend/src` içinde **yalnız ~27
kez, 13 dosyada** kullanılmış (çoğu tekil "etiketi küçük ekranda gizle" dokunuşu,
ör. `App.tsx:1077` `labelClassName="hidden sm:inline"`). Yani bilinçli bir
responsive tasarım **yok**; düzen sabit-genişlik + yatay flex üzerine kurulu.

---

## 2. Mevcut durum tespiti (portrait'te kırılanlar)

| Bileşen | Sorun | Dosya:satır |
|---------|-------|-------------|
| **Kök layout** | `flex h-full` → NavRail + Sidebar + main + DetailPanel **yan yana** sütunlar; dar ekranda ezilir | `App.tsx:992` |
| **NavRail** | Sabit `w-52` (açık) / `w-14` (dar) dikey rail, **daima görünür**, tam yükseklik; portrait'te kalıcı yer kaplar | `NavRail.tsx:174-176` |
| **SessionsSidebar** | `flex h-full shrink-0` + JS ile sürüklenen `style={{ width }}` — ikinci kalıcı sütun; chat'te hep açık | `SessionsSidebar.tsx:204-208` |
| **SessionDetailPanel** | Sağdaki üçüncü sütun (`detailOpen` ile), main'i daha da daraltır | `App.tsx:1276-1287` |
| **AgentRoster / MemoryPanel** | `memory` view'ında sol roster + panel = yine iki sütun | `App.tsx:1033-1041` |
| **Header** | `px-6 py-3` + sağda ChatMeters + klasör butonları; dar ekranda taşar | `App.tsx:1052-1102` |
| **ModalOverlay** | `fixed inset-0 flex items-center justify-center p-4` → **ortalanmış** dialog; child kendi `max-w`'sini koyar → telefonda full-screen sheet değil, sıkışık kutu | `common/ModalOverlay.tsx:26-27` |
| **FlowCanvas (React Flow)** | Serbest zoom/pan grafik + node inspector; dokunmatik/dar ekranda düzenlenemez | `flow/FlowCanvas.tsx`, `panels/FlowsPanel.tsx` |
| **NetworkPanel / MemoryGraphView (vis-network)** | Fizik-tabanlı geniş grafik; portrait'te kullanılamaz | `panels/NetworkPanel.tsx`, `graph/MemoryGraphView.tsx` |
| **TaskBoard (Kanban)** | Yatay sütun dizisi; telefonda yan yana kolonlar sığmaz | `panels/TaskBoard.tsx` |
| **Sabit genişlikli kutular** | Çok sayıda `w-[…]`/`min-w-[…]` (modallar, kartlar, pill'ler) — dar ekranda taşma riski | ~93 dosyada desen (denetlenecek) |
| **Composer** | Kart + araç çubuğu + dropdownlar; dar ekranda buton satırı sıkışır (temel işlev çalışır ama ergonomi zayıf) | `chat/Composer.tsx` |

> Not: `~93 dosya` sayısı geniş bir grep deseninin sonucu (`min-w-0` gibi masum
> kullanımları da içerir). Gerçek kırılma noktaları F-fazlarında dosya-dosya
> denetlenip daraltılmalı; tablo **kesin** olanları listeler.

### 2.1 Mimari kök neden

`App.tsx` render ağacı tek bir yatay şeritte 4 kardeş tutuyor:

```
<div flex h-full>
  <NavRail/>              (sabit genişlik, her zaman)
  {chat && <SessionsSidebar/>}   (sabit/resizable genişlik)
  <main flex-1/>         (içerik)
  {chat && detailOpen && <SessionDetailPanel/>}  (sağ sütun)
</div>
```

Masaüstünde ideal; telefonda **üç kalıcı sütun içeriğe yer bırakmaz.** Çözüm:
dar ekranda bu kardeşleri **üst üste (stack) + navigasyonla değiştirilen tekil
görünümlere** indirgemek.

---

## 3. Tasarım yaklaşımı

### 3.1 Breakpoint stratejisi — `md` eşiği

Tailwind `md` (768px) eşiği "telefon vs masaüstü" sınırı olur:
- **`< md` (mobil/portrait):** tek sütun, alt tab bar, sürgülü paneller (drawer),
  full-screen sheet modallar.
- **`>= md` (masaüstü/tablet yatay):** bugünkü çok-sütunlu düzen **aynen korunur**.

Yaklaşım **mobile-first ekleme**: mevcut masaüstü sınıflarını `md:` ile
etiketleyip, altına mobil varsayılanları koymak (masaüstü davranışını bozmadan).
Örn. NavRail: `class="... w-52"` → `class="... hidden md:flex md:w-52"`.

### 3.2 NavRail → alt yatay-kaydırılabilir nav bar (KARAR)

**Seçilen tasarım:** `< md`'de dikey rail gizlenir (`hidden md:flex`), yerine
**altta tek şeritte yatay kaydırılabilir bir nav bar** gelir — tüm view'lar
(16) + Workspace + Ayarlar aynı çubukta, taşanlar **yatay scroll** ile erişilir.
"En sık 4 tab + Daha fazla drawer" hibritine gerek kalmaz; hiçbir hedef gizli
değil. `NAV` dizisi (`NavRail.tsx`) **export edilip** hem masaüstü rail hem mobil
bar tarafından tüketilir (tek kaynak).

- `fixed inset-x-0 bottom-0` + `overflow-x-auto` + scrollbar gizli + safe-area
  padding (`env(safe-area-inset-bottom)`).
- Her öğe: ikon + küçük etiket (dikey), aktiflik accent-soft tint, busy/unread/
  dirty noktaları (rail ile aynı sinyaller).
- Kod: `components/MobileNavBar.tsx` (`md:hidden`).
- **Not (F-sonraki):** çoklu-workspace **switcher** henüz mobil barda yok — ilk
  öğe `Workspace` view'ına gider. Workspace *değiştirme* (birden çok ws arası) için
  ayrı bir mobil switcher/drawer eklenmeli.

### 3.3 Çok-panelli görünümleri tek-panele indir

- **Chat (`< md`):** SessionsSidebar ve SessionDetailPanel **kalıcı sütun olmaktan
  çıkar**, sürgülü overlay (drawer) olur. Varsayılan görünüm = transcript + composer
  tam genişlik. Üstte "☰ oturumlar" ve "ⓘ detay" butonları drawer'ları açar.
- **Memory/Agents:** roster, aynı şekilde açılır liste (drawer) veya ayrı "geri"li
  alt-ekran (master-detail navigasyon yığını).
- Kalıp: `< md`'de aynı anda **tek panel**; ötekiler drawer/stack.

### 3.4 Composer / chat mobil düzeni

- Transcript alanı tam genişlik, `px` küçült (`px-6`→`px-3` mobilde).
- Composer araç çubuğu: butonlar tek satıra sığmazsa ikincil aksiyonlar "…"
  menüsüne toplanır; textarea min yükseklik korunur. Klavye açılınca `dvh` birimi
  (dynamic viewport height) ile composer görünür kalsın (`100dvh` layout).
- `env(safe-area-inset-*)` ile çentik/altbar boşlukları.

### 3.5 Modallar → full-screen sheet

- `ModalOverlay` (`common/ModalOverlay.tsx`) `< md`'de **tam ekran sheet** olsun:
  `items-center justify-center p-4` → mobilde `items-end` + `w-full h-full`/`max-h`
  bottom-sheet ya da full-screen. Tek dosyada değişiklik → **tüm modallar** (task
  formu, artifact önizleme, workspace oluştur, context modalları) birden kazanır.
- `ArtifactPreviewModal` full-screen önizleme (48'in F2 dosya önizleme hedefiyle
  örtüşür).

### 3.6 Grafik/canvas görünümleri — "salt-görüntüle / uyar"

`48 §8/madde 4` ile tutarlı: FlowCanvas (React Flow), NetworkPanel &
MemoryGraphView (vis-network), TaskBoard (Kanban) portrait'te **birinci sınıf
düzenleme hedefi değil**.
- **Karar:** `< md`'de bu ekranlar **salt-görüntüle** (pan/zoom serbest ama
  düzenleme kısıtlı) + üstte "en iyi masaüstünde" bilgi çubuğu; ya da liste-tabanlı
  fallback (ör. flow node'ları düz liste, board sütunları dikey yığın). İlk sürümde
  **görüntüle + uyarı** yeterli; liste-fallback ikinci öncelik.

---

## 4. Bileşen bazında yapılacaklar (öncelik sıralı)

1. **Kök layout responsive shell** (`App.tsx:992`) — `< md` tek sütun + panelleri
   drawer/stack'e taşıyan koşullu render. En kritik, her şeyin önkoşulu.
2. **NavRail alt tab bar + drawer** (`NavRail.tsx`) — birincil navigasyon.
3. **Chat drawer'ları** — SessionsSidebar + SessionDetailPanel'i overlay'e çevir
   (`App.tsx:1012-1030`, `1276-1287`, `SessionsSidebar.tsx:204`).
4. **ModalOverlay full-screen sheet** (`common/ModalOverlay.tsx`) — tek dosya, geniş
   etki.
5. **Header sıkıştırma** (`App.tsx:1052`) — meters/butonları mobilde ikon-only.
6. **Composer mobil ergonomi** (`chat/Composer.tsx`) + `100dvh`/safe-area layout.
7. **Grafik ekranları görüntüle+uyarı** (Flows/Network/Memory graph/Board).
8. **Sabit-genişlik denetimi** — kalan `w-[…]`/`min-w-[…]` taşmalarını dosya-dosya
   `max-w-full`/responsive'e çevir.

---

## 5. Faz planı

| Faz | İş | Kapsam |
|-----|-----|--------|
| **F1 — Shell + Sohbet** | Kök responsive shell (`< md` tek sütun) + NavRail alt tab bar/drawer + SessionsSidebar & DetailPanel drawer + Composer/transcript tam genişlik + `100dvh`/safe-area. **En kritik akış (sohbet) uçtan uca telefonda kullanılır.** | En büyük değer |
| **F2 — Modallar + Header** | `ModalOverlay` full-screen sheet + header ikon-only sıkıştırma + ArtifactPreviewModal full-screen (48-F2 ile örtüşür) | Orta |
| **F3 — Liste ekranları** | Agents/Memory/Schedules/Skills/Tools/Market/Budget/Logs panellerinde iki-sütun→tek-sütun + kart taşmalarını düzelt | Orta |
| **F4 — Grafik/canvas** | Flows/Network/MemoryGraph/TaskBoard için "görüntüle + uyarı" (ops. liste-fallback) | Düşük |
| **F5 — Cila** | Kalan sabit-genişlik denetimi, dokunmatik hedef boyutları (min 44px), erişilebilirlik, gerçek cihaz testi | Süregelen |

En kritik akış (sohbet) **F1'de** biter → 48'in PWA doğrulaması (F1) ile birlikte
telefondan gerçek kullanım mümkün olur.

---

## 6. Açık kararlar / riskler

1. **Navigasyon deseni — KARARLAŞTI:** altta yatay-kaydırılabilir tam nav bar
   (tüm view'lar tek şeritte, taşanlar scroll ile). Bkz. §3.2. Açık kalan: mobilde
   çoklu-**workspace switcher** (şimdilik yok).
2. **Masaüstü regresyonu:** mobile-first ekleme yaparken `md:` etiketlemesi eksik
   kalırsa masaüstü düzeni bozulabilir → her bileşende `>= md` görünümü snapshot'la
   doğrula. Mümkünse Playwright ile iki viewport (390px + 1440px) görsel test.
3. **SessionsSidebar resize mantığı:** `style={{ width }}` (JS drag) mobilde
   anlamsız → `< md`'de devre dışı bırakılıp full-width drawer'a geçilmeli.
4. **Grafik kütüphaneleri:** React Flow / vis-network dokunmatik jestleri sınırlı;
   liste-fallback yatırımı ne kadar? İlk sürümde **uyarı yeterli** kararı.
5. **`dvh` desteği:** iOS Safari eski sürümlerinde `dvh` davranışı; `100dvh` +
   fallback `100vh` gerekebilir.
6. **Test yüzeyi:** gerçek cihaz + tarayıcı devtools portrait emülasyonu; `48`'in
   VPN/PWA kurulumunda telefondan canlı doğrulama en sağlıklısı.
7. **Kapsam sırası:** Bu doküman UI-only; `48`'in dosya indirme/önizleme
   endpoint'leri (F2) bağımsız ilerleyebilir ama F2 modallarıyla görsel olarak
   buluşur.

---
