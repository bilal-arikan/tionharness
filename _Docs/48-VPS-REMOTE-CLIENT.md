# 48 — VPS Uzak Sunucu + Mobil İnce İstemci

> **Durum:** FİZİBİLİTE / TASARIM 📐 (2026-07-04). Henüz uygulanmadı.
> **Amaç:** TionSwarm backend'ini bir VPS'te (7/24 Linux) çalıştırıp, telefondan
> **ince bir istemci** (WebView APK / PWA) ile erişmek. Workspaceler ve tüm
> dosyalar VPS diskinde yaşar; telefon yalnızca uzak bir kullanıcı arayüzüdür.
> **Kapsam kararı (bu doküman):** tek kullanıcı · güvenlik = **VPN (Tailscale/WireGuard)** ·
> dosya ihtiyacı = **önizleme + indirme + text/prompt editleme** (harici editör/senkron YOK).

İlgili dokümanlar: `32-NATIVE-PENCERE` (Windows WebView2 masaüstü — bunun Android/uzak eşi),
`26-CALISMA-DIZINI` (WorkingDir + `fs/browse`), `08-DEPOLAMA` (dosya-tabanlı store),
`33-DIS-AJAN-OTOMASYONU` (API eksiksizliği + auth-yok/CORS-wildcard notu), `34-YEDEKLEME`.

Alternatif yön: **on-device Android** (Go backend telefonda) — bu dokümanın reddettiği
yol. Neden reddedildiği §2'de; özeti: Android'de subprocess yasağı claude-cli/Bash/
`transform_data`/stdio-MCP'yi kırar. VPS modeli bunların **hiçbirini** kırmaz.

---

## 1. Motivasyon

Kullanıcı TionSwarm'ya telefondan erişmek istiyor. İki mimari değerlendirildi:

1. **On-device (telefonda Go server):** Android'in `execve` (W^X) kısıtı yüzünden
   rastgele subprocess spawn yasak → claude-cli (Node), `Bash`/`PowerShell`,
   `transform_data` (python/node/bun), stdio-MCP **kırılır**. Ayrıca Doze/battery
   optimizasyonu scheduler/otonomiyi öldürür. Köklü yeniden mimari gerektirir.
2. **VPS + ince istemci (bu doküman):** Backend gerçek bir Linux makinesinde
   **değişmeden** çalışır; telefon HTTPS/SSE ile bağlanan bir kabuk. Subprocess
   sorunu yok, otonomi 7/24 gerçekten çalışır, workspace depolaması normal FS.

Karar: **VPS modeli.** Çekirdek TionSwarm'ya neredeyse dokunmadan, asıl iş
**güvenlik + ince istemci + mobil UI cilası**.

### 1.1 Mimari

```
  Telefon (ince APK / PWA)          VPS (Linux, 7/24)
  ┌───────────────────────┐        ┌──────────────────────────────┐
  │ WebView → https://vps │══VPN══▶│ TionSwarm binary (DEĞİŞMEDEN)   │
  │ (Tailscale ağında)    │  SSE   │  ├─ API 138+ endpoint         │
  └───────────────────────┘        │  ├─ claude-cli / Bash / py    │
  Masaüstü tarayıcı ──── aynı VPS ─│  ├─ stdio + HTTP MCP          │
                                   │  └─ scheduler/cron/otonomi ✅  │
                                   │ Disk: store/ + artifacts/ +   │
                                   │       workspaceler + JSONL    │
                                   └──────────────────────────────┘
```

`workspaceler ve temel dosyalar VPS'te durur` = TionSwarm'nun bugünkü **dosya-tabanlı
depolaması** (`08-DEPOLAMA`) VPS diskinde. Ekstra "bağlama/link/mount" mekanizması
YOK — dosyalar zaten orada yaşar, ajanlar bugünkü gibi fs araçlarıyla editler.

---

## 2. On-device vs VPS — neyin çalıştığı

| Konu | On-device Android | **VPS + ince istemci** |
|------|-------------------|------------------------|
| claude-cli (Node) | ❌ Node yok | ✅ Linux'ta çalışır |
| `Bash` / `PowerShell`(→sh) | ❌ shell yok | ✅ |
| `transform_data` (py/node/bun) | ❌ runtime yok | ✅ |
| stdio MCP (subprocess) | ❌ spawn kısıtı | ✅ |
| HTTP MCP | ✅ | ✅ |
| Scheduler / cron / otonomi / coordinator | ⚠️ Doze öldürür | ✅ **güvenilir 7/24** |
| Workspace depolama | scoped storage'a hapis | ✅ normal Linux FS |
| API provider (anthropic/openrouter/…) | ✅ (tek çalışan) | ✅ |
| Yeniden mimari yükü | Yüksek | **Neredeyse yok** |

**Sonuç:** VPS modelinde tool matrisinde hiçbir şey kırılmaz. On-device'ın tek
avantajı "internetsiz/tam offline" idi — kullanıcı senaryosunda gerekli değil.

---

## 3. Güvenlik — tek gerçek engel (karar: VPN)

Bugün API'de **auth YOK, CORS wildcard** (`33-DIS-AJAN-OTOMASYONU`) — bilinçliydi
çünkü hep `127.0.0.1`'de. Public VPS'e **çıplak** koymak felaket: internetteki
herkes ajanları sürer, API token'larını (parayı) yakar, dosyaları okur/siler.

İki yol vardı; **bu kapsam için VPN seçildi:**

### 3.1 Seçilen: VPN (Tailscale / WireGuard) — kod yazmadan güvenli

- VPS'i özel bir ağa (Tailscale tailnet ya da WireGuard) koy; telefon da o ağda.
- TionSwarm yine **`127.0.0.1` ya da tailnet arayüzüne** bind eder → **public'e hiç
  çıkmaz.** "auth-yok/CORS-wildcard" felsefesi olduğu gibi korunur, **TionSwarm'ya
  auth kodu eklemeye gerek kalmaz.**
- Tek kullanıcı + kişisel kullanım için en pratik, en az efor.
- Bind adresi: `TIONSWARM_ADDR=<tailscale-ip>:8080` (veya `127.0.0.1` + Tailscale
  Serve ile TLS terminasyonu). Tailscale zaten uçtan-uca şifreli → ek TLS opsiyonel.

### 3.2 Reddedilen (bu kapsamda): reverse proxy + auth

- nginx/caddy ile TLS + token/mTLS auth, `0.0.0.0` bind arkada. TionSwarm'ya bir
  **auth katmanı** eklemeyi gerektirir (bugün yok) — orta çaplı iş.
- Yalnızca **çok-cihaz paylaşımı / gerçek public erişim** gerekince mantıklı.
  Tek kullanıcıda gereksiz karmaşa → **ertelendi** (bkz. §7 Gelecek).

> **Not:** VPN seçimi, TionSwarm kodunda auth değişikliği gerektirmeyen tek yol.
> Bu, bu projeyi "backend'e dokunmadan" hedefine kilitliyor.

---

## 4. Dosya erişimi — önizleme / indirme / editleme

Kullanıcı ihtiyacı: **klasör gezme + dosya/artifact önizleme + indirme + text
dosyası ve editlenebilir prompt'ları düzenleme.** İyi haber: büyük kısmı API'de
**zaten var**; telefonda çözülecek şey render + birkaç küçük endpoint.

### 4.1 Zaten var olan (yeniden kullanılacak)

| İhtiyaç | Mevcut API / sistem | Kaynak |
|---------|---------------------|--------|
| Klasör gezme | `GET /api/fs/browse` | `26-CALISMA-DIZINI` |
| Oturum cwd seç | `GET/PUT /api/sessions/{id}/workdir` | `26` |
| Dosya oku/yaz/edit | `Read`/`Write`/`Edit` araçları + fs kilitsiz | `sandbox.go` |
| Artifact listeleme/içerik | `workspace/artifacts/<sessionId>/` + artifact API | Artifact sistemi |
| Binary/medya artifact | `kind=image/video/audio/file` + `sourcePath` (base64 gömmeden) | Artifact sistemi |
| Editlenebilir prompt'lar | agent soul, skill body, flow prompt, workspace instructions, compact.md, core memory, oturum hedefi (9 call-site, `PromptEditor`) | `common/PromptEditor.tsx` |

### 4.2 Eklenecek (küçük)

1. **Dosya indirme endpoint'i** — verili yoldaki (workspace-altı/artifact) binary'yi
   `Content-Disposition: attachment` ile stream eden `GET /api/files/download?path=…`
   (ya da artifact-id bazlı). Telefon "İndir" butonuyla cihaza kaydeder.
   - Güvenlik: yol normalize + path-traversal guard (VPN arkasında olsa da).
2. **Mobil dosya/artifact önizleme ekranı** — WebView'de:
   - text/kod → mevcut kod bloğu + diff renderer,
   - md → Markdown render, image/pdf → inline önizleme (Craft'taki `*-preview`
     mantığının mobil eşi). Zaten `CodeBlock.tsx` yetenekleri var.
3. **Text/prompt editörü (mobil):** `PromptEditor` mobil-dostu düzende; dosya için
   `Read`→göster→düzenle→`Write` (freshness guard korunur, `readtracker.go`).

### 4.3 KAPSAM DIŞI (bilinçli)

- **Harici editörle düzenleme / klasör senkronu** (SSHFS, Syncthing, rclone) — bu
  kapsamda YOK. Kullanıcı ihtiyacı "önizle/indir/text+prompt editle" ile karşılanır.
  Gerçek offline ayna gerekirse §7'de opsiyonel olarak durur.
- Telefonun VPS diskini **mount** etmesi gerekmez; her şey API'den gezilir/önizlenir.

---

## 5. İnce istemci (APK / PWA)

İki eşdeğer seçenek — ikisi de çekirdeğe dokunmaz:

1. **PWA (en kolay):** Mevcut React SPA'ya `manifest.json` + service worker eklenir;
   telefonda "Ana ekrana ekle" ile kurulur. VPS URL'ini (tailnet) açar. APK derleme
   derdi yok, Play Store gerekmez.
2. **İnce WebView APK:** Kotlin bir `WebView` Activity, `https://<tailnet-host>`'a
   yönlenir (Windows WebView2 masaüstünün — `32-NATIVE-PENCERE` — Android eşi).
   - `network_security_config.xml`: tailnet host'una (gerekiyorsa cleartext/özel CA)
     izin. Tailscale Serve TLS veriyorsa düz HTTPS yeter.
   - Avantaj: native paylaşım, bildirim, dosya indirme entegrasyonu.

Öneri: **önce PWA** (hızlı doğrulama), memnun kalınırsa WebView APK'ya sarılır.

---

## 6. Bonus: otonomi bu modelde gerçek olur

VPS 7/24 ayakta → `robfig/cron` scheduler, `schedule_wake`, etiket-tetikli
otomasyon döngüleri (`46-ETIKET-OTOMASYON`), coordinator/worker (`47`) **güvenilir**
çalışır (Android Doze derdi yok). Senaryo: "telefonu kapatsam da ajanlarım VPS'te
işi sürdürür, sabah bildirimle sonucu görürüm." Masaüstü bildirimleri push/e-posta
kanalına maplenebilir (gelecek iş).

---

## 7. Faz planı

| Faz | İş | Çekirdek dokunuşu |
|-----|-----|-------------------|
| **F0** | VPS'e TionSwarm kur (`go build` linux/amd64) + claude-cli login (izole config home) + Tailscale kur, tailnet bind | Yok (deploy) |
| **F1** | PWA: `manifest.json` + service worker; telefondan tailnet URL'ini aç, uçtan uca sohbet doğrula | Frontend |
| **F2** | Dosya indirme endpoint'i (`/api/files/download`, path guard) + mobil önizleme ekranı (text/md/image/pdf/diff) | Küçük backend + frontend |
| **F3** | Mobil-dostu editör: `PromptEditor` mobil düzeni + dosya Read→Write akışı (freshness guard) | Frontend |
| **F4** | Mobil UI cilası: composer, artifact listesi, `fs/browse` gezgini küçük ekran düzeni | Frontend |
| **F5 (ops.)** | İnce WebView APK (Kotlin) — PWA yetmezse | Yeni Android modülü |
| **Gelecek** | Reverse proxy + auth (çok-cihaz/public), Syncthing klasör aynası, push bildirim | Ayrı doküman |

---

## 8. Açık kararlar / riskler

1. ~~**Tailscale bind detayı:**~~ **KARARLAŞTI (2026-07-24): `127.0.0.1` + Tailscale Serve.**
   Otomatik Let's Encrypt sertifikası + tailnet host adı verdiği için doğrudan tailnet-IP
   bind'ine tercih edildi. Hazır script: **`scripts\tailscale-serve.ps1`** — `dev.ps1` gibi
   **ön planda** koşar (backend çıktısı terminale akar; Ctrl+C süreç ağacını indirir ve
   `-KeepServe` verilmedikçe serve yapılandırmasını kaldırır).

   ```powershell
   .\scripts\tailscale-serve.ps1              # gerekiyorsa derle, koş, serve et
   .\scripts\tailscale-serve.ps1 -Build       # önce zorla yeniden derle
   .\scripts\tailscale-serve.ps1 -Port 5174   # loopback portunu değiştir (vars. 5174)
   .\scripts\tailscale-serve.ps1 -Reset       # yalnız serve yapılandırmasını sök
   ```

   **Neden HTTPS şart:** tarayıcılar mikrofona (`getUserMedia` / Web Speech) yalnız
   **güvenli bağlamda** izin verir → telefondan sesli girdi ancak `https://` ile çalışır
   (bkz. `07-CHAT-UX.md` STT). Trafik tailnet **içinde** kalır (`serve`, `funnel` DEĞİL)
   → auth'suz backend asla public olmaz. Varsayılan port 5174, `dev.ps1` (5173/8090) ve
   unity-mcp (8080) ile çakışmaz. **Tek seferlik ön koşul:** tailnet admin konsolunda
   "HTTPS Certificates" açık olmalı (`https://login.tailscale.com/admin/dns`).
2. **claude-cli VPS'te kimlik:** izole config home + `claude setup-token` (oauth) ya
   da `ANTHROPIC_API_KEY` (`claudeCliAuthKind`/`Token`, `03` maddesi). VPS'te login
   akışı headless olacağından **setup-token** önerilir.
   - **In-app tarayıcı girişi (ClaudeAuthDialog):** "Otomatik (loopback)" alt-modu
     backend'de efemer `127.0.0.1:<port>` dinleyici açıp `redirect_uri`'yi
     `http://localhost:<port>/callback` yapar → dönüş viewer'ın **kendi** makinesine
     düşer, VPS'e değil. Bu yüzden uzak erişim (`window.location.hostname` localhost
     değil) tespit edilince dialog varsayılan olarak **"Elle kod"** akışına geçer
     (`redirect_uri = platform.claude.com/oauth/code/callback`; kodu kopyala-yapıştır,
     her yerden çalışır). Otomatik moda geçilirse uyarı bandı gösterilir.
3. **İndirme endpoint güvenliği:** VPN arkasında olsa da path-traversal guard şart;
   yalnız workspace-altı + artifact yollarına izin.
4. **Mobil düzen kapsamı:** flow canvas (React Flow) + ilişki grafiği (vis-network
   ~515KB) küçük ekranda ikinci sınıf — mobilde salt-görüntüle ya da gizle.
5. **Yedek/kalıcılık:** VPS diski tek doğruluk kaynağı → `34-YEDEKLEME` periyodik zip
   + VPS snapshot/off-site kopya önerilir (APK'da veri tutulmuyor).
6. **VPS maliyeti/uptime:** her zaman-açık küçük instance yeter; otonomi token
   maliyeti mobilde daha görünmez → global **otonomi-pause** freni + Tasarruf Merkezi
   takibi yakından izlenmeli. *(Per-ajan `daily_token_limit` guardrail'i 2026-07-01'de
   kaldırıldı; bugün yalnız kullanım takibi var.)*
