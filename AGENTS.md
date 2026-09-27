# TionHarness — Codex Notları

Bu dosya Codex CLI içindir. Genel proje kuralları `CLAUDE.md`'dedir; ikisi çelişirse
`CLAUDE.md` esastır.

## Kod Arama Katmanları

Global `~/.codex/AGENTS.md` içinde hem `codebase-memory-mcp` hem `zvec-grep` blokları
var ve birbirinden habersizler ("her zaman graph tercih et" vs. "anlamsal için zg").
Bu repoda geçerli sıralama aşağıdakidir:

- **Adı bilmiyorsun, niyeti biliyorsun** → `zvec_grep_search`.
  Doğal dil sorgusu; dosya:satır + snippet döner. Markdown/JSON/YAML de indexli.
- **Sembol adı elinde, bağlantısını istiyorsun** → `codebase-memory-mcp`
  (`project` = `C-Users-Bilal-Desktop-Projects-TionHarness`). Caller/callee,
  impact analizi, bağımlılık zinciri. Vektör araması bunu veremez.
- **Birebir string / dosya deseni** → `zvec_grep_rg` veya native grep. En hızlısı.

Tipik akış: konum bilinmiyorsa `zvec_grep_search` ile bul, sembol çıkınca
`codebase-memory-mcp` ile ilişkilendir. Adı zaten biliyorsan doğrudan grep.

`zvec_grep_search` aracı oturumda yoksa veya sonuçlar yetersizse grep'e düş.
Boş arama sonucu tek başına indeksin bulunmadığını göstermez; sunucunun
tazelik/indeks durumunu esas al. Yeni indeks oluşturmak açık kullanıcı onayı ister.

## Kabuk

Git Bash zorunlu; POSIX sözdizimi, Windows yolları `/c/...`.
`rtk` hook olarak kuruludur, komut başına elle `rtk` ekleme.
