"""Render reviewed tool traces; purpose labels are supplied by trace inspection."""
import json
import statistics
import sys
from collections import Counter
from pathlib import Path

data = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
rows = data["results"]
lines = [
    "# Ayrıntılı araç kullanımı karşılaştırması",
    "",
    f"CLI: `{data['cliVersion']}`; düşünme: `{data['effort']}`. Önceki deneyden ayrı koşudur.",
    "",
    "Aynı üç görev, iki model ve iki taşıyıcı: 12 çağrı. Her görev sonunda sekiz",
    "bağımsız test çalıştırılır. Aşağıdaki amaç etiketleri kaydedilmiş komut ve",
    "araç olayları incelenerek eklenir; modelin kendi beyanı değildir.",
    "",
    "| Model | Yol | Başarı | Araç adımı | Araç hatası | Keşif | Okuma | Düzenleme | Doğrulama | Temizlik | Ortalama tur |",
    "|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|",
]
for model in ("gpt-6-astra", "gpt-6-sol"):
    for route in ("tionharness", "codex-cli"):
        group = [r for r in rows if r["model"] == model and r["route"] == route]
        if not group:
            continue
        trace = [t for r in group for t in r["trace"]]
        purposes = Counter(p for t in trace for p in t.get("purposes", ["incelenmedi"]))
        lines.append(f"| {model} | {route} | {sum(r['passed'] for r in group)}/{len(group)} | {len(trace)} | {sum(t['isError'] for t in trace)} | {purposes['keşif']} | {purposes['okuma']} | {purposes['düzenleme']} | {purposes['doğrulama']} | {purposes['temizlik']} | {statistics.mean(r['seconds'] for r in group):.2f} sn |")
lines += ["", "Amaç sütunları örtüşebilir: tek kabuk çağrısı hem listeleme hem dosya okuma yapabilir.", "", "## Kabuk araç süreleri", "", "| Model | Yol | Süresi ölçülen kabuk çağrısı | Toplam | Ortanca | En uzun |", "|---|---|---:|---:|---:|---:|"]
for model in ("gpt-6-astra", "gpt-6-sol"):
    for route in ("tionharness", "codex-cli"):
        shells = [t for r in rows if r["model"] == model and r["route"] == route for t in r["trace"] if t["tool"] == "shell"]
        measured = [t["durationMs"] for t in shells if t["durationMs"] is not None]
        if measured:
            lines.append(f"| {model} | {route} | {len(measured)}/{len(shells)} | {sum(measured)/1000:.3f} sn | {statistics.median(measured):.0f} ms | {max(measured)} ms |")
lines += [
    "",
    "## Çağrı sıraları",
    "",
    "Süreler araç başlangıç/bitiş olaylarının istemciye gelişinden ölçülür; saf CPU",
    "süresi değildir. `?` başlangıç ölçümünün bulunmadığını veya sağlayıcıda sıfır",
    "ile ayırt edilemediğini gösterir. Sıfır maliyet olarak yorumlanmaz. Araçların",
    "toplam süresini turdan çıkarmak saf model düşünme süresini vermez.",
    "",
]
for row in rows:
    lines += [f"### {row['task']} — {row['model']} — {row['route']}", "", f"Tur: {row['seconds']:.2f} sn; değerlendirme: {'geçti' if row['passed'] else 'başarısız'}.", "", "| Sıra | Araç | Amaç | Bitiş¹ | Süre | Hata |", "|---|---|---|---:|---:|---|"]
    for n, step in enumerate(row["trace"], 1):
        duration = "?" if step["durationMs"] is None else f"{step['durationMs']} ms"
        lines.append(f"| {n} | {step['tool']} | {step.get('summary', 'incelenmedi')} | {step['endMs']/1000:.2f} sn | {duration} | {'Evet' if step['isError'] else 'Hayır'} |")
    lines += [""]
lines += [
    "¹ Tur başlangıcından itibaren; araçlar bitiş sırasıyla gösterilir.",
    "",
    "## Kapsam ve ham kanıt",
    "",
    "Bu bir native dosya/kabuk deneyi; MCP, tarayıcı, worker, uzun oturum ve Codex",
    "Desktop karşılaştırması değildir. Her görev/model/yol bir kez çalıştırıldı.",
    "Cache, sunucu yükü ve farklı araç stratejileri gecikmeyi etkileyebilir.",
    "",
    "Yama olayları tam yama metnini vermeyebilir; boş alan eksik ölçümdür.",
    "Nihai solution.py içeriği ayrıca saklanır; bu, ara yamaların kaydı değildir.",
    "",
    "TionHarness çıktısı sağlayıcının normalleştirdiği ve boyutunu sınırladığı",
    "izdir; doğrudan CLI çıktısı native olay alanlarından alınır. Özellikle yama",
    "girdi/çıktı biçimleri aynı olmadığından karakter sayıları token maliyeti gibi",
    "karşılaştırılmaz. Araç başına token ölçümü yoktur; token bilgisi tur düzeyindedir.",
    "",
    "Aynı okuma komutunun düzenlemeden sonra tekrarlanması tek başına israf değildir.",
    "Hata sayacı başarısız durum/çıkış kodlarını ölçer; başarılı test sonucu araç",
    "hatasını silmez. Testler küçük örneklemi kapsar, genel kalite garantisi vermez.",
    "",
    "[Girdiler, çıktılar, nihai kod ve ham ölçümler](69-CODEX-TOOL-BENCHMARK-2026-09-28.json)",
    "· [Deney aracı](../scripts/codexbench/README.md)",
]
Path(sys.argv[2]).write_text("\n".join(lines) + "\n", encoding="utf-8")
