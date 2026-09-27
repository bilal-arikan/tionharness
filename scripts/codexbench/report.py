"""Render the measured sample without extrapolating to Desktop or billing."""
import json
import statistics
import sys
from pathlib import Path

data = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
rows = data["results"]
lines = [
    "# TionHarness–Codex CLI karşılaştırmalı deney",
    "",
    f"CLI: `{data['cliVersion']}`. Düşünme seviyesi: `{data['effort']}`.",
    "",
    "Bu sonuçlar üç küçük Python göreviyle yapılan taşıyıcı deneyidir. Codex Desktop",
    "veya TionHarness'in tam uygulaması ölçülmedi. Her görev/model/taşıyıcı yalnız",
    "bir kez çalıştırıldı; istatistiksel üstünlük veya genel başarı oranı çıkarılamaz.",
    "",
    "| Model | Yol | Başarılı görev | Ortalama süre | Ortanca süre | Toplam girdi¹ | Cache okuma² | Çıktı |",
    "|---|---|---:|---:|---:|---:|---:|---:|",
]
for model in ("gpt-6-astra", "gpt-6-sol"):
    for route in ("tionharness", "codex-cli"):
        group = [r for r in rows if r["model"] == model and r["route"] == route]
        if not group:
            continue
        total = sum(r["usage"]["inputTokens"] + r["usage"].get("cacheReadTokens", 0) + r["usage"].get("cacheWriteTokens", 0) for r in group)
        cache = sum(r["usage"].get("cacheReadTokens", 0) for r in group)
        output = sum(r["usage"]["outputTokens"] for r in group)
        mean = statistics.mean(r["seconds"] for r in group)
        median = statistics.median(r["seconds"] for r in group)
        lines.append(f"| {model} | {route} | {sum(r['passed'] for r in group)}/{len(group)} | {mean:.2f} sn | {median:.2f} sn | {total:,} | {cache:,} | {output:,} |")
lines += [
    "",
    "¹ Çağrı içindeki tüm model adımlarının toplamıdır; tek bağlam büyüklüğü değildir.",
    "² Cache okuma toplam girdinin alt kümesidir; ikinci kez eklenmez. Çıktı düşünmeyi",
    "zaten içerir. Bunlar abonelik kotası veya gerçek fatura ölçümü değildir.",
    "",
    "## Tekil denemeler",
    "",
    "| Görev | Model | Yol | Süre | Araç adımı³ | Test sonucu |",
    "|---|---|---|---:|---:|---|",
]
for row in rows:
    lines.append(f"| {row['task']} | {row['model']} | {row['route']} | {row['seconds']:.2f} sn | {row['tools']} | {'Geçti' if row['passed'] else 'Başarısız'} |")
lines += [
    "",
    "³ Tamamlanan araç olaylarıdır; sağlayıcı izlerinin sınıflandırması farklı olabilir.",
    "",
    "## Yöntem ve sınırlar",
    "",
    "Her görev sekiz bağımsız unittest yöntemiyle değerlendirildi. Değerlendirici",
    "model tamamlandıktan sonra eklendi. Temiz klasörler, aynı hesap, CLI, görev",
    "talimatları ve düşünme seviyesi kullanıldı. Sıra dönüşümlüydü; sunucu cache'i",
    "sıfırlanmadı. Başarısız görevlere dışarıdan düzeltme veya tekrar hakkı verilmedi.",
    "",
    "Ölçüm native dosya/kabuk araçlarını içerir; MCP, plugin, worker, çok turlu",
    "oturum, compaction ve Desktop araç ekosistemini içermez. Süre CLI başlatma,",
    "model ve araç döngüsünü kapsar, bağımsız değerlendiriciyi kapsamaz. Cache ve",
    "araç stratejisi değişebildiğinden süre/token farkı yalnız taşıyıcı maliyeti",
    "olarak yorumlanamaz. Uzun veya zor depo görevlerinde sonuç değişebilir.",
    "",
    "Tekrar üretme: [deney aracı](../scripts/codexbench/README.md).",
    "Ham ölçümler: [JSON sonuçları](69-CODEX-BENCHMARK-2026-09-28.json).",
]
Path(sys.argv[2]).write_text("\n".join(lines) + "\n", encoding="utf-8")
