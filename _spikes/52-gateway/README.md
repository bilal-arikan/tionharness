# Faz 0 Spike — MCP Gateway (Doc 52)

Gateway deseninin claude-cli yolunda çalışıp çalışmadığını **gerçek claude-cli** ile ölçer.
Sonuçlar: `_Docs/52-MCP-GATEWAY.md` §3-E.

## Bileşenler
- `server/main.go` — stateful streamable-HTTP MCP server. GET SSE akışını açık tutar;
  `spike_grow` çağrılınca `spike_secret`'i ekleyip `notifications/tools/list_changed` push eder.
- `mcp-config.json` — claude `--mcp-config` (tek server: `spike`, `type:http`).

## Çalıştırma
```powershell
cd server; go build -o spikegw.exe .; .\spikegw.exe -addr 127.0.0.1:8791   # ayrı pencere
cd ..
claude -p "call spike_grow then spike_secret and report the SECRET" `
  --mcp-config mcp-config.json --strict-mcp-config `
  --allowedTools "mcp__spike" --model claude-fable-5 `
  --output-format stream-json --verbose > run1.stream.json 2> run1.err.log
```

## Ölçülen (2026-07-06, claude 2.1.201)
- **Q1** aynı-tur list_changed çağrısı: ✅ (`SECRET=GATEWAY_OK_42` alındı)
- **Q2** wildcard (`mcp__spike`) sonradan gelen aracı kapsıyor: ✅
- **Q3** mid-turn list_changed cache'i silmiyor: ✅ (marjinal delta)
- Gözlem: claude araçları `ToolSearch select:` ile on-demand yükledi (HTTP defer artık çalışıyor).
