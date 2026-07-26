# TionSwarm — Ajan Rehberi

Bu dosya, TionSwarm deposunda çalışan ajanlar için tekrar eden friction'dan
türetilmiş kısa kurallar içerir. Terminal: PowerShell veya Git-Bash.

## Grep/ripgrep kullanımı

Buradaki yerleşik `Grep` aracı **ripgrep** sözdizimi kullanır (POSIX `grep`
değil). Gerçek yaşanan hatalardan çıkarılmış kurallar:

- **Literal süslü/parantez karakterlerini kaçır.** Regex meta karakteri olan
  `{}` `()` `[]` işaretlerini düz metin ararken ters bölü ile kaçır.
  Örnek: bir Go interface aramak için `interface\{\}` yaz, `interface{}` değil.

- **Alternation için düz `|` kullan.** Shell alışkanlığıyla `a\|b` yazma —
  ripgrep bunu "regex parse error" olarak reddeder. Grep aracına `pattern`
  değerini `error|warning|fatal` biçiminde, kaçırılmamış boru ile ver.

- **`output_mode` yalnızca şu üç değeri kabul eder:** `content` |
  `files_with_matches` | `count`. Başka bir değer (ör. `text`, `lines`) geçersiz
  ve reddedilir.

- **Sabit dosya yolu vermeden önce Glob ile doğrula.** Grep'e var olmayan bir
  `path` verirsen sonuç boş/yanıltıcı döner ve hata da vermez. Önce
  `Glob` ile (ör. `internal/db/store_*.go`) dosyanın gerçekten var olduğunu
  teyit et, sonra o yolu Grep'e geç.

## internal/db dizin haritası

`internal/db` paketi tek bir dosya-tabanlı store'dur (`DB` tipi, `store.go` ve
`db.go`). Her varlık ayrı bir `store_*.go` dosyasında sahiplenilir. Aşağıdaki
liste yalnızca depoda **gerçekten var olan** dosyaları içerir.

| Dosya | Sahiplendiği alan |
|-------|-------------------|
| `store.go` | Agents, **Sessions**, **Messages** ve önyükleme (boot) yüklemesi — çekirdek store |
| `db.go` | `DB` tipi, açılış, dizin sabitleri, kilit/persist yardımcıları |
| `store_artifact.go` | Artifacts; oturum başına tekil "onaylanan planlar" rolling artifact |
| `store_task.go` | Kanban board görevleri (task kartları) ve board değişiklik olayları |
| `store_flow.go` | Flows ve flow run'ları |
| `store_run.go` | Çalışan run'ların (running) listelenmesi |
| `store_schedule.go` | Schedules (zamanlanmış çalıştırmalar) |
| `store_hook.go` | Hook yapılandırmaları |
| `store_mcp.go` | MCP sunucu yapılandırmaları |
| `store_tools.go` | Workspace düzeyi araç etkinleştirme/görünürlük yapılandırması (`tools-config.json`) |
| `store_automation.go` | Otomasyon; etiket (tags) normalizasyonu yardımcıları |
| `store_search.go` | Oturumlar arası mesaj araması (`SearchHit`, `SearchOpts`) |
| `store_usage.go` | Ajan/gün bazlı LLM kullanım (usage) rollup'ı ve çağrı taksonomisi |
| `store_session_usage.go` | Oturum ömrü boyunca LLM kullanım rollup'ı (`SessionUsage`) |
| `store_lessons.go` | Workspace düzeyi hata→ders (lessons) JSONL store'u (`lessons.jsonl`) |

İlgili model tanımları ayrı `models_*.go` dosyalarındadır (ör.
`models_task.go`, `models_flow.go`, `models_hook.go`, `models_mcp.go`,
`models_artifact.go`, `models_automation.go`, `models_attachment.go`). Ana
`Usage`/`Session`/`Agent` modelleri `models.go` içindedir. `store_*_test.go`
dosyaları ilgili store dosyalarının testleridir.
