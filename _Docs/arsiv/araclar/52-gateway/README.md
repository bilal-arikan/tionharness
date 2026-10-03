# Faz 0 Spike — MCP Gateway (Doc 52)

Bu prototip ve göç yardımcıları tamamlanmış gateway çalışmasının tarihsel
araçlarıdır; günlük geliştirme veya aktif MCP sunucusu değildir. Yeni bir
protokol deneyi ya da eski TS gateway kurulumunun göçü gerektiğinde kullanılır.
Gerçek gateway uygulaması ve sonuçlar [52-MCP-GATEWAY.md](../../../52-MCP-GATEWAY.md)
belgesindedir.

Gateway deseninin claude-cli yolunda çalışıp çalışmadığını **gerçek claude-cli** ile ölçer.
Sonuçlar: `_Docs/52-MCP-GATEWAY.md` §3-E.

## Bileşenler
- `server/main.go` — stateful streamable-HTTP MCP server. GET SSE akışını açık tutar;
  `spike_grow` çağrılınca `spike_secret`'i ekleyip `notifications/tools/list_changed` push eder.
- `mcp-config.json` — claude `--mcp-config` (tek server: `spike`, `type:http`).

## Çalıştırma
```powershell
# Run from the repository root in a separate terminal.
Set-Location _Docs/arsiv/araclar/52-gateway/server
go build -o spikegw.exe .
.\spikegw.exe -addr 127.0.0.1:8791
```

Ayrı bir istemci terminalinde depo kökünden:

```powershell
Set-Location _Docs/arsiv/araclar/52-gateway
claude -p "call spike_grow then spike_secret and report the SECRET" `
  --mcp-config mcp-config.json --strict-mcp-config `
  --allowedTools "mcp__spike" --model claude-fable-5 `
  --output-format stream-json --verbose > run1.stream.json 2> run1.err.log
```

## Eski TS kurulumunu göç etme

`migrate-vps.py` verilen yapılandırmayı dönüştürür; `apply-migration.py` sonucu
hedef TionHarness örneğine uygular. Girdi yolları çağrıldığı terminale göredir;
mutlak yollarla verilirse arşiv konumundan bağımsız çalışır. `--ws` hedefleri
sınırlar; verilmezse uygulama örneğinin tüm workspace'leri hedeflenir. Uygulama
adımı aynı adlı mevcut sunucuları silip yeniden içeri alır; yalnız amaçlanan
göç için çalıştırılır.

Depo kökünden, hazırlanan eski yapılandırma dosyalarıyla:

```bash
python _Docs/arsiv/araclar/52-gateway/migrate-vps.py /absolute/path/config.json --secrets /absolute/path/secrets.json > /absolute/path/import.json
python _Docs/arsiv/araclar/52-gateway/apply-migration.py /absolute/path/import.json --base http://127.0.0.1:8090 --ws WS1
```

Arşiv taşımasında bu araçlar çalıştırılmadı. Eski `_spikes/52-gateway/`
altındaki Git tarafından yok sayılan deney çıktıları yerlerinde bırakıldı.

## Ölçülen (2026-07-06, claude 2.1.201)
- **Q1** aynı-tur list_changed çağrısı: ✅ (`SECRET=GATEWAY_OK_42` alındı)
- **Q2** wildcard (`mcp__spike`) sonradan gelen aracı kapsıyor: ✅
- **Q3** mid-turn list_changed cache'i silmiyor: ✅ (marjinal delta)
- Gözlem: claude araçları `ToolSearch select:` ile on-demand yükledi (HTTP defer artık çalışıyor).
