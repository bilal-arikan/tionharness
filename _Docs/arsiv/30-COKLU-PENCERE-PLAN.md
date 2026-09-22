# 30 — Çoklu pencere: tamamlanmış plan gövdesi

> Arşiv: `30-COKLU-PENCERE.md` dosyasından taşınan, tamamlanmış plan/tasarım metni. Güncel durum için asıl dokümana bak.

## Sorun (bugün)

`frontend/.../WorkspaceSwitcher.tsx` `openInNewWindow` → `window.open(url, '_blank')`.
Tarayıcıda aynı app'in yeni sekmesi açılır (çalışır). Ama WebView2 masaüstü uygulaması
`window.open`'ı handle etmediğinden WebView2 varsayılanı URL'yi **sistem tarayıcısında (Edge)**
açar → üstte adres çubuğu + kopuk bağlam; composer ajan seçili olmadığından mesaj gönderilemez.

## Kod değişiklikleri

### Backend — `cmd/tionharness-desktop`

- **`main.go`**: başta `if url := os.Getenv("TIONHARNESS_WEBVIEW_URL"); url != "" { runSecondary(url); return }`.
  - `runSecondary(url)`: loopback doğrula (`http://127.0.0.1:` öneki — güvenlik), `setDPIAware`,
    `LockOSThread`, WebView2 penceresi (aynı boyut/başlık), title bar temasını **HTTP'den** çek
    (`GET <base>/api/settings` → preset) `applyTitleBar`, `Navigate(url)`, `Run()`.
  - Birincil yol (mevcut): `app.Bootstrap` + Pencere; ek olarak `w.Bind("tionharnessOpenWindow", ...)`.
- **`openwindow_windows.go` (yeni)**: `spawnWindow(baseURL, route string)` →
  `exe, _ := os.Executable(); cmd := exec.Command(exe); cmd.Env = append(os.Environ(),
  "TIONHARNESS_WEBVIEW_URL="+baseURL+"/#"+route); cmd.Start()`.
- **Title bar — connect-only varyantı**: `app.App.Appearance()` yok; küçük bir HTTP yardımcı
  `fetchAppearance(base) (preset, theme string)` (`GET /api/settings`, mevcut DTO `themePreset`/
  `theme` döner) + aynı `watchTitleBar` mantığı HTTP poll ile. (Basit v1: yalnız açılışta uygula.)

### Frontend — `WorkspaceSwitcher.tsx`

```ts
function openInNewWindow(id: string) {
  const route = buildRoute({ workspaceId: id, view: 'chat', id: null })
  const w = window as any
  if (w.chrome?.webview && typeof w.tionharnessOpenWindow === 'function') {
    w.tionharnessOpenWindow(route)          // native: yeni TionHarness penceresi (ayrı süreç)
    return
  }
  // Tarayıcı/dev: eski davranış (aynı app, yeni sekme)
  window.open(`${location.origin}${location.pathname}#${route}`, '_blank', 'noopener')
}
```

(Opsiyonel: `lib/desktop.ts` `isDesktop()` / `openNativeWindow(route)` yardımcılarıyla
soyutla, ileride başka yerlerde de kullanılsın.)

## Doğrulama planı

1. `go build ./...`/`vet` yeşil; başsız `tionharness` ve birincil masaüstü davranışı değişmedi.
2. Canlı: masaüstünü aç → WS2'yi "yeni pencerede aç" → **ayrı bir native TionHarness penceresi**
   açılır (Edge değil, adres çubuğu yok), WS2/chat yüklenir, **mesaj gönderilebilir**.
3. Başlık çubuğu ikincil pencerede de temaya uygun.
4. İkincil pencereyi kapat → birincil etkilenmez. Tarayıcı/dev modunda eski davranış korunur.
5. Güvenlik: `TIONHARNESS_WEBVIEW_URL=http://evil.com` ile başlat → reddedilir.

## Uygulama adımları (sıra)

1. `cmd/tionharness-desktop/main.go`: connect-only dalı + `runSecondary` + `w.Bind("tionharnessOpenWindow")`.
2. `openwindow_windows.go`: `spawnWindow` (düz `exec.Command` ile self-exec; `proc.Command` değil).
3. Title bar connect-only: `fetchAppearance(base)` HTTP yardımcı.
4. Frontend `WorkspaceSwitcher.openInNewWindow`: WebView2 köprüsü + fallback (+ ops. `lib/desktop.ts`).
5. Build (UI + desktop) + canlı doğrulama (yukarıdaki 5 madde).
6. README/`32-NATIVE-PENCERE.md` güncelle, `05-ILERLEME.md` girdisi.

## Tahmini efor

Orta. En ince kısım: connect-only başlık çubuğu teması (HTTP'den okuma) ve Bind callback'inin
doğru thread'de süreç başlatması (Dispatch gerekmez; başlatma saf `os/exec`, UI thread'i bloklamaz).
v1 "açılışta tema + birincil kapanınca ikincil otomatik kapanmaz" sınırlarıyla küçük tutulabilir.
