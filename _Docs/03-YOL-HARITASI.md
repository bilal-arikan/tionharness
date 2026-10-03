# TionHarness — Açık İşler ve Yol Haritası

> **Özet (2026-10-03):** Bu dosya kalan ve isteğe bağlı işleri tutar. İlk fazlar, masaüstü paketleme ve kamusal yayın tamamlandı; memory alt sistemi 2026-07-05'te kaldırıldı. Tamamlanan işlerin tarih/commit/kanıt dökümü [tarihsel yol haritasına](arsiv/03-YOL-HARITASI-TAMAMLANAN.md), değişiklik günlüğü [ilerlemeye](05-ILERLEME.md) ayrıldı. Aşağıdaki eski açık maddeler uygulanacak yeni görev için yeniden kaynak doğrulaması gerektirir; işaretleri sırf doküman temizliği nedeniyle değiştirilmemiştir.

İlke: **MVP ile başla, katman katman büyüt.** Her iş çalışan ve test edilebilir bir çıktı verir.

## Açık backlog

### Bağlam, bellek, trace

- [ ] **C2** — Compaction emniyet katmanı (`snip`) — 1M tampon var, düşük öncelik

### MCP & dağıtım

- [~] **D1** — MCP çoklu-transport: stdio + Streamable HTTP ✅ (+ hibrit `shared`/`scoped` kapsam); kalan: config kapsam-zinciri (local<user<project)

- [ ] **D3** — Server/Remote/Bridge (kapsam dışı, not olarak saklanır)

### API-native kalan UI boşlukları (2026-09-22, `arsiv/55-API-NATIVE-YOL-HARITASI.md`'den)

- [ ] Oturum bağlam önizlemesi (`session_context.go`) native-search modunda deferred kataloğu
  yansıtmıyor — "shipped tools" listesine `deferred` rozeti.

- [ ] Task budget'ın tur-meta'da gösterimi (otonom mesaj balonunda "bütçe: N token" rozeti).

- [ ] Flow-builder UI'da `outputSchema`/`jsonField` görsel düzenleyicisi (graf JSON'unda destekli).

### Insight ve maliyet açıkları (2026-09-22)

- [ ] Workspace-seviyesi insight taraması için in-app rapor artifact'i — eski
  `RenderAppFixReport` render'ı çağrısız kaldığı için kaldırıldı (2026-09-22), artifact session-scoped ([60-RETROSPEKTIF-TARAMA.md](60-RETROSPEKTIF-TARAMA.md) tek açık madde).

- [ ] [MALIYET-DUSURME-PLANI.md](MALIYET-DUSURME-PLANI.md) açık maddeleri: **#2** kodlama ajanlarında
  playwright/tarayıcı MCP satırlarını kapat, `climcp.WriteConfig` DisabledTools'a saygı; **#4** native
  döngüde tur-ortası `Prune`'u kaldır (claude-cli yolu zaten kalıcı); **#5** yapılandırılabilir TTL,
  varsayılan 5m; **#7** büyük araç sonuçlarını dosyaya yaz + özet; **#8** cache-kırılma dedektörü.

### 🟠 P1 — Çok-ajan mimarisine uyan

- [~] **CG-7 — Hooks + koşullu otomasyon + webhook** *(kısmî)*: (a) command hook'ları (olay→shell, timeout + fail-open) **✅ YAPILDI** — Faz P4 (`internal/agent/hooks.go`, bkz. `_Docs/18-HOOKS.md`); ajan da `create_hook`/`update_hook`/`delete_hook` ile yönetebilir (`_Docs/24-SELF-MANAGEMENT.md`). **Kalan:** (b) otomasyon koşulları (time/state/label gate); (c) webhook action (exp. backoff retry); prompt-hook'ları + rate limiter. `events` bus + `scheduler` ile örtüşür. *(craft v0.4.3, v0.7.5, v0.7.7)*

- [ ] **CG-8 — Otomasyon/flow geçmişi cap + compaction**: `flow_runs` sınırla (örn. 20/flow, 1000 global) + periyodik compaction. *(craft v0.7.8)*

### 🟡 P2 — Sağlamlaştırma (file-based depolama)

- [ ] **CG-10 — Config oto-onarım**: başlangıçta bozuk config/`~/.claude.json` (boş/BOM/invalid) tespit+onarım+backup; Windows file-lock retry. *(craft v0.2.33)*

- [ ] **CG-11 — Volatile-context kuralını koru**: tarih/saat/session-state eklenirse **yalnız `SystemDynamic`'e veya user-mesaj kuyruğuna** (statik cache prefix'e değil). Mevcut iki-parçalı tasarım zaten doğru — regression'a karşı not. *(craft v0.10.2)* ✅ tasarım uyumlu

### 🟢 P3 — Güvenlik (web UI / remote açılırsa)

- [ ] **CG-12 — URL şema blocklist** *(düşük risk — kontrol edildi)*: `Markdown.tsx` `isExternal` yalnız `http(s)` eşliyor; `mailto:`/`#`/`http(s)` → `<a href>`, **diğer tüm şemalar** (`javascript:`/`file:`/`data:`/`vscode:`…) `onOpenFile` (yerel dosya) dalına düşüyor → doğrudan `href` XSS yok, ama keyfi şema string'i callback'e gidiyor. Açık allowlist/blocklist + `onOpenFile`'da şema doğrulaması ekle. *(craft v0.8.12, v0.9.6)*

- [ ] **CG-13 — Shell sandbox escape**: `find -exec` vb. engelle (shell açıkken). *(craft v0.5.0)* — **Faz P3 ile birlikte**

- [ ] **CG-14 — Remote açılırsa**: WebUI auth (argon2id + JWT + rate limiter) · WS TLS zorunlu (`wss://`) · upload/artifact yollarında path-traversal sanitize doğrula · CI'da `go mod verify`. *(craft v0.8.2, v0.7.0, v0.3.2, v0.8.0)*

### 🔵 P4 — UI/UX & ekosistem (opsiyonel)

- [ ] **CG-15 — Render blokları**: Mermaid native · HTML/PDF/image/markdown preview · datatable/spreadsheet + `transform_data`. Artifact sistemine eklenebilir. *(craft v0.3.0, v0.4.2, v0.4.6, v0.9.6)*

- [ ] **CG-17 — Mini agents** (hafif prompt + hızlı model profili). *(craft v0.3.1)*

- [ ] **CG-18 — Session labels + auto-label + batch işlemler**. *(craft v0.2.27, v0.4.6)*

- [ ] **CG-20 — Doküman araçları** (`markitdown`/`pdf-tool`/`xlsx-tool`) attachment işleme için. *(craft v0.6.0)*

- [ ] **CG-21 — Messaging gateway** (Telegram/WhatsApp/Lark): response mode enum + subprocess izolasyon + **erişim kontrol** (güvenlik kritik). Not: Connectors fazı kapsam dışıydı. *(craft v0.8.10, v0.9.1)*

### Provider ekosistemi genişletme (2026-06-18)

- [ ] **SC-2 — Generic CLI factory** (CLI ailesi): referans projenin `streamGenericCliChat` deseni (binary spawn + stdout satır-stream, JSON parse yok) ile yapısal çıktısı olmayan onlarca coding-CLI'yi tek handler + veri listesiyle ekle. Yeni `kind_genericcli.go` + `[]genericCLI{id,label,binary}`. **CLI işi — CLI fazı açılınca, SC-1'den sonra.**

### the external agent-Agent incelemesinden — Kendini-geliştiren ajan özellikleri (2026-06-19)

- [ ] **HA-2 — Kendini-geliştiren prosedürel skill + skill hub** *(yüksek değer — ayırt edici)*:
  harici ajanin en özgün yanı: ajan zor bir görevi tamamladıktan sonra **kendi prosedürel skill'ini
  otonom yazar** ve tekrar kullanımla **iyileştirir** (procedural memory); skill'ler
  [agentskills.io](https://agentskills.io) merkezi hub'ında paylaşılır. TionHarness'te skill sistemi
  (dosya-tabanlı, global/workspace tier, `create_skill`/`delete_skill`) + market (HarnessPack v1)
  **zaten var** — eksik olan **otonom skill üretimi** (görev sonrası ajanın deneyimden skill
  damıtması) ve **skill'in zamanla iyileşmesi** (kullanım geri-bildirimiyle revizyon). Mevcut
  self-management skill araçları bunun temelini oluşturuyor; üzerine
  "görev-sonrası skill-damıtma" hook'u + agentskills.io uyumlu içe/dışa aktarım eklenir.
  İlişkili: market (`_Docs/21-MARKET.md`), self-management (`_Docs/24-SELF-MANAGEMENT.md`).

> **Not:** HA-2 TionHarness'in mevcut alt sistemlerinin (skill sistemi, market) **üzerine**
> kurulabilir; sıfırdan değil. harici ajanin diğer güçlü yanları (mesajlaşma
> gateway → **CG-21**; çoklu çalıştırma backend'i Docker/SSH/Modal → kapsam dışı/D3) ayrı maddelerde.

### Faz R — Çok-ajan yarış & kurtarma guard'ları (oturum analizinden, 2026-06-19)

- [ ] **RG-1 — Entity versioning + CAS** *(1. dalga, S-M, yüksek değer)*: `db` entity'lerine `Version int`; her `mutate*Locked` bump'lar; yazım araç/API'sinde opsiyonel `If-Match` → bayat sürüm reddedilir. Belgelenmiş **"son yazan kazanır"** veri-kaybını kapatır. Temel: atomik `*.tmp`→`rename` + `store.go mutateSessionLocked` deseni (zaten var).

- [ ] **RG-2 — Workspace git-lock + provenance-scoped staging** *(2. dalga, M, yüksek değer)*: workspace başına tek-yazar git kilidi + ajan yalnız dokunduğu/`created_by` path'leri stage eder (oturumda elle Python ile yapılanın ürünleşmiş hali). Temel: `created_by` provenance (`_Docs/24-SELF-MANAGEMENT.md`) + `builtin_shell.go` git çağrıları.

- [ ] **RG-3 — Kaynak kira (lease) registry** *(2. dalga, M, orta değer)*: ajan port/temp-dir ister, runtime boş olanı verir, `stop`'ta otomatik bırakır. `:8090` çakışması bir daha olmaz; TionHarness'in kendi açılış preflight'ı için de kullanılır. Yer: `agent/runtime.go` yaşam döngüsü.

- [ ] **RG-4 — Tur yan-etki defteri → otomatik teardown** *(1. dalga, M, yüksek değer)*: tur başına spawn edilen PID / geçici workspace / temp dosya kaydı; tur biter veya çökerse otomatik teardown. Agent'ın elle yaptığı "test ws sil, sunucu durdur" işini garantiye alır. Temel: `agent/recovery.go` (A1) + `db/inflight.go` sidecar deseni.

- [ ] **RG-5 — Boot orphan reconcile genişletmesi** *(2. dalga, S-M, orta değer)*: açılışta takılı child süreç + sahipsiz temp workspace temizliği. Şu an yalnız tur (`inflight.go`) ve flow (`ResumeRunningFlows`, `flow.go`) resume ediliyor; aynı boot yoluna süreç/kaynak reconcile eklenir.

## Sonraya bırakılan masaüstü fikri

Sistem tepsisine küçülme, durum göstergesi ve hızlı menü ilk yol haritasında
önerilmiştir. Henüz bu belge temizliğinde uygulandığı iddia edilmez; aday
kütüphane ve platform riskleri [tarihsel kayıtta](arsiv/03-YOL-HARITASI-TAMAMLANAN.md)
korunur. Connectors ve pano dispatcher'ı kapsam dışı kararlar olarak aynı kayıtta tutulur.
