# Codex CLI araç izinleri — 2026-09-28

> **Özet:** Kurulu Codex CLI 0.157.1, Claude'un `--allowedTools` ve
> `--disallowedTools` bayraklarını sunmuyor. MCP araçları için tam adlarla
> `enabled_tools` / `disabled_tools`, native kabuk için `features.shell_tool`
> kullanılabiliyor. TionHarness bu karşılıkları artık uyguluyor. Bunların ilk
> hangi sürümde geldiği bu incelemede belirlenmedi.

## Uygulanan davranış

| Alan | Codex yolu |
|---|---|
| Harici MCP sunucusu | Mevcut sunucu kapısı korunur; izin verilmeyen sunucu bağlanmaz. |
| Tekil MCP aracı | Ajanın `server__tool` adı `enabled_tools = ["tool"]` alanına çevrilir. |
| Tekil yasak | `BlockedTools` ve etkin `ToolOverrides` yasakları `disabled_tools` olur; yasak önceliklidir. |
| Altyapı sunucusu istisnası | İzin listesi muafiyeti korunur; açık yasak yine uygulanır. |
| Interaction köprüsü | Core/extended için yalnız turun izinli araçları listelenir; sunucudaki çağrı kontrolü ayrıca sürer. |
| Native shell | Interaction endpoint'i ve TionHarness shell köprüsü etkin, `NativeShell` kapalıysa `shell_tool=false`. |
| Native shell açık | Kullanıcının açık tercihiyle Codex'in `exec_command` / `write_stdin` araçları korunur. |
| Köprü yok | Native shell mevcut davranışını korur. |
| Yeni tur / hatalı izin belgesi | Önceki turun MCP sunucuları temizlenir; eski izinler tekrar kullanılmaz. |

`server*` / `server__*` gibi tüm sunucuyu kapsayan kurallar mevcut kapıda
çözülür. **Sunucu içi araç jokerleri desteklenmez:** `server__read*` şeklindeki
kısıt tam araç adlarına çevrilemiyorsa ilgili sunucu kapatılır ve
`codex_mcp_tool_policy` debug hatası üretilir. İzin listesinde zaten tüm sunucuyu
açan bir kural varsa gereksiz dar izin jokeri dikkate alınmaz. Yasak jokeri için
böyle bir istisna yoktur. İkinci bir stdio sunucusu yalnız katalog keşfi için
başlatılmaz.

Bu değişiklik tam Claude paritesi değildir. Codex'e genel bir native araç
izin listesi eklenmedi; `apply_patch` yerleşik aracı korunur. Native shell açıkken
komutlar TionHarness shell onayından geçmez. Harici MCP ve plugin araçlarına
genel, etkileşimli TionHarness onayı eklenmedi; burada tanımlanan filtreler
doğrudan yapılandırılan MCP sunucuları içindir. Plugin araçları ayrıca kendi
yapılandırmalarıyla yönetilir. MCP filtreleri bir işletim sistemi sandbox'ı
değildir.

## Doğrulama

- Gerçek `codex.exe 0.157.1`, geçici çalışma dizini ve geçici `CODEX_HOME`.
- `--strict-config` ile gerçek yapılandırma ayrıştırıcısı.
- Yerel Responses ve MCP taklit sunucuları; gerçek model API'si, giriş anahtarı
  veya hesap kotası kullanılmaz. Astra/Sol model kimliğiyle CLI araç davranışı
  test edilir; model zekâsı veya başarı oranı ölçülmez.
- Model yanıtı kontrollü biçimde `ALL_TOOLS` kataloğunu okur ve hem okuma hem
  yazma aracını çağırmayı dener. Böylece yalnız prompt görünürlüğü değil, gerçek
  dispatch sonucu da kontrol edilir.

| Senaryo | Sonuç (Astra + Sol) |
|---|---|
| Kısıtsız referans | Üç MCP aracı görünür; okuma ve yazma çağrıları ulaşır. |
| Native shell kapalı | `exec_command` / `write_stdin` yok; `apply_patch` var. |
| Tam adla izin | Yalnız izinli okuma aracı görünür ve çalışır. |
| Tam adla yasak | Yazma aracı görünmez ve çağrısı sunucuya ulaşmaz. |
| Aynı araca izin + yasak | Yasak kazanır; çağrı ulaşmaz. |
| Boş izin listesi | Hiçbir MCP aracı çalışmaz. |
| Codex'e doğrudan izin jokeri | Joker genişletilmez; hiçbir eşleşme oluşmaz. |
| Codex'e doğrudan yasak jokeri | Joker genişletilmez; yazma açık kalır. TionHarness bu nedenle böyle bir sunucuyu bağlamaz. |

**16/16 senaryo geçti.** Go regresyonları ayrıca izin çevirisini, legacy/override
önceliğini, muaf sunucuları, desteklenmeyen jokerlerde kapalı kalmayı, köprü
bağlantısını, shell seçeneğini, hatalı belgeleri ve tur değişiminde temizliği
denetler. Tam proje geçidi `scripts/test.sh full` geçti: tüm Go paketleri,
143 frontend dosyasında 1026 test, bağımlılık yönü ve diff kontrolü başarılı.

Makine tarafından okunabilir sonuçlar:
[16 senaryonun kaydı](69-CODEX-ARAC-IZINLERI-2026-09-28.json).

Tekrarlanabilir test: [probe_policy.py](../scripts/codexbench/probe_policy.py).
Komut ve kapsam: [benchmark rehberi](../scripts/codexbench/README.md).

## Kaynaklar

- [OpenAI yapılandırma referansı](https://learn.chatgpt.com/docs/config-file/config-reference)
- [OpenAI MCP rehberi](https://learn.chatgpt.com/docs/extend/mcp)
- Yerel CLI `exec --help` ve yukarıdaki davranış testi.
