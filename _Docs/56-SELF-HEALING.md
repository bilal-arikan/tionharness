# 56 — Kendi Kendini Onaran Oturum Akışları (Self-Healing)

> Durum: **Tamamlandı** (2026-07-07). İlham: external-context-agent projesinin
> `error_classifier` / `tool_guardrails` / `message_sanitization` / stuck-loop
> desenleri, TionSwarm'ın mevcut saf-`decideRecovery` mimarisine uyarlandı.
> Kapsam: **native tool-use döngüsü** (anthropic/minimax). claude-cli kendi
> döngüsünü sürer — Faz A/B/C ona uygulanmaz; Faz D (oturum-seviyesi) her iki
> yol için geçerlidir.

## Bileşenler

### Faz A — Provider hata sınıflandırıcı + sınırlı retry
- `internal/agent/errclass.go`: `errClass` taksonomisi — `rate_limit`,
  `overloaded`, `server_error`, `timeout` (retry-edilebilir); `context_overflow`
  (compact yolu); `auth`, `billing`, `cancelled`, `unknown` (terminal).
  `classifyProviderError` muhafazakâr string/sentinel eşleme yapar; emin
  olunamayan her hata `unknown` = terminal (asla körlemesine retry edilmez).
- `decideRecovery` yeni `contProviderRetry` continuation'ı döner: aynı istek,
  jitter'lı üstel backoff (1s→30s cap, ±%25), `sleepCtx` ile iptal-duyarlı.
- Bütçe: `loopState.providerRetries` < `maxProviderRetries`
  (ayar `maxProviderRetries`, varsayılan 2, clamp 0..5, 0 = kapalı).
- İptal (`context.Canceled`) artık `termCancelled` ile biter (önceden
  `provider_error` görünüyordu).

### Faz B — Tool-loop guardrail (döngü tespiti)
- `internal/agent/toolguard.go`: yan-etkisiz saf kontrolcü (`check`/`observe`),
  tur başına `completeTracedInner`'da kurulur. Üç sayaç:
  | Sayaç | Uyarı | Blok/Halt |
  |---|---|---|
  | Exact-failure (aynı tool + aynı arg-hash hatası) | 2 | 5 (blok) |
  | Same-tool-failure (aynı tool ardışık hata) | 3 | 8 (tur halt) |
  | No-progress (idempotent aynı çağrı başarılı tekrar) | 2 | 5 (blok) |
- İdempotentlik hardcoded liste değil: `tools.Classify(name) == RiskRead`
  (MCP araçları otomatik kapsanır).
- **Uyarılar** (`toolGuardWarnings`, vars. açık): eyleme dönük İngilizce hint
  başarısız tool_result'ın SONUNA eklenir ("diagnose before retrying, try a
  different tool, don't fall back to text-only").
- **Hard stop** (`toolGuardHardStop`, vars. KAPALI): blok → tool çalışmadan
  sentetik `IsError` result (`StepError(reason:guardrail_block)`); halt →
  batch'in sonuçları yazıldıktan sonra kontrollü tur sonu
  (`StepRecovery(reason:guardrail_halt)`, hata DEĞİL).
- Hook/izin redleri sayaçlara girmez (hermes paritesi: policy denial ≠ tool
  failure).

### Faz C — Mesaj dizisi onarımı
- `internal/conversation/repair.go` `RepairSequence(msgs)`: her provider çağrısı
  öncesi (toolloop iterasyon başı) tur-içi geçmişte tool_use↔tool_result
  eşleşme değişmezlerini zorlar; ihlaller provider 400'ü olarak turu öldürmek
  yerine onarılır:
  1. duplicate tool_result → ilki kalır;
  2. sahipsiz tool_result → düşer (mesajda başka şey yoksa mesaj düşer);
  3. cevapsız assistant tool_use → RawContent yoksa komşu assistant batch ile
     birleştirilir, yoksa sentetik "cancelled" tool_result üretilir;
  4. kısmi batch (bazı sonuçlar kayıp) → eksikler sentetik doldurulur.
- Saf + idempotent; sağlam dizi aynı backing array ile döner (bedava).
  **Sessiz düzeltme yok**: her onarım log + `debug.jsonl`'e `repair` olayı.
- Not: kalıcı geçmiş (`session.jsonl` → `toProviderMessages`) yalnız Role+Text
  taşır; tool blokları yalnız tur-içi yaşar — onarımın doğru yeri bu yüzden
  toolloop'tur, `Prepare` değil.

### Faz D — Kalıcı stuck sayacı + otonom askıya alma
- `db.Session.StuckTurns`: ardışık kötü turlar (tur hatası VEYA guardrail
  halt); temiz tur sıfırlar. Restart'a dayanıklı (session JSON'unda).
- Eşik (`stuckTurnThreshold`, vars. 3, clamp 0..20, 0 = kapalı) aşılınca:
  - otomatik **`stuck` etiketi** (`agent.TagStuck`, autotag add-only) →
    etiket-otomasyonla onarım ajanı bağlanabilir (46-ETIKET-OTOMASYON);
  - `completeTracedInner` başındaki `stuckGate` sonraki **otonom** turları
    reddeder (manuel chat hiç kapılanmaz — kurtarma yolu tam da odur).
- Gate'in kendi reddi sayacı BÜYÜTMEZ (`stuckGuardMarker` dışlaması).
- `RemoveSessionTags` ile `stuck` etiketi kaldırılınca sayaç da sıfırlanır
  (fixer çözdü → otonomi geri açılır).

## Gözlemlenebilirlik
- Yeni `debug.jsonl` olay türleri: `repair` (rule `Name`'de) ve `guardrail`
  (`warn`/`block`/`halt`/`stuck_gate` `Name`'de, tool/detay `Detail`'de).
- Yeni adım reason'ları: `provider_retry` (StepRecovery), `guardrail_block`/
  `guardrail_halt`. UI mevcut StepRecovery/StepError kartlarıyla gösterir.

## Ayarlar (settings.json → Tunables)
| Ayar | Varsayılan | Etki |
|---|---|---|
| `maxProviderRetries` | 2 (0..5) | geçici provider hatası retry bütçesi |
| `toolGuardWarnings` | true | hint ekleme |
| `toolGuardHardStop` | false | blok/halt devre kesici |
| `stuckTurnThreshold` | 3 (0..20) | stuck etiketi + otonom gate eşiği |

## Testler
- `errclass_test.go` (sınıflandırma/retryable/backoff/sleepCtx + provider-retry
  karar tabloları), `toolguard_test.go` (uyar/blok/halt/reset/no-progress),
  `conversation/repair_test.go` (9 onarım senaryosu + idempotentlik),
  `stuck_test.go` (sayaç/etiket/gate/untag-reset).

## Kapsam dışı / sıradaki adımlar
- **Hata→ders döngüsü** (hermes `background_review` karşılığı): tur-sonu
  reflektör fork'u dersleri skill/artifact'a yazar; `stuck`/`tool-error`
  etiketleri + otomasyon altyapısı zemin hazır. Ayrı plan.
- Guardrail eşiklerinin settings'e açılması (şimdilik sabit default'lar).
- Frontend Ayarlar UI'ına yeni alanların eklenmesi (API/DTO hazır).
