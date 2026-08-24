# TionHarness — UTF-8-as-CP1254 mojibake onarim araci (one-shot).
#
# KOK NEDEN: TionHarness'in depolama/Go I/O yolu UTF-8 temizdir. Bozulma, govdeyi
# double-encode eden bir ISTEMCI'den girer (Windows PowerShell 5.1 Invoke-RestMethod
# Turkce/ANSI govdeyi CP1254 olarak gonderir). Sonuc: dogru UTF-8 baytlar (orn "i"
# = C4 B1) CP1254 olarak cozulup tekrar UTF-8 kaydedilir ("Ä±" = C3 84 C2 B1).
#
# Bu arac stored knowledge (*.json "content") ve session (*.jsonl "text") alanlarini
# tarar ve double-encoding'i GUVENLE tersine cevirir: yalniz tersine cevirme
# (CP1254-bytes -> UTF-8-decode) GECERLI UTF-8 verir VE round-trip orijinal bozuk
# metni geri uretirse uygular. Aksi halde dokunmaz (yanlis pozitif yok).
#
# Kullanim:
#   .\scripts\repair-encoding.ps1                      # DRY-RUN: tum workspace'leri tarar, sadece raporlar
#   .\scripts\repair-encoding.ps1 -StoreRoot 'C:\Users\user\.tionharness\workspaces'
#   .\scripts\repair-encoding.ps1 -Apply               # gercekten yazar (once .bak-encfix yedegi alir)
#   .\scripts\repair-encoding.ps1 -Workspace WS5 -Apply

param(
    [string]$StoreRoot = "$env:USERPROFILE\.tionharness\workspaces",
    [string]$Workspace = "",          # bos -> tum workspace'ler
    [switch]$Apply
)

$ErrorActionPreference = "Stop"
$cp1254 = [System.Text.Encoding]::GetEncoding(1254)
$utf8Strict = New-Object System.Text.UTF8Encoding($false, $true)  # BOM yok, gecersizde throw
$utf8NoBom  = New-Object System.Text.UTF8Encoding($false, $false)

# Rapor icin kisa onizleme.
function Trunc { param([string]$s) if ($null -eq $s) { return "" }; $s = $s -replace "`n", " "; if ($s.Length -gt 60) { $s.Substring(0, 60) + "..." } else { $s } }

# Bir string icindeki double-encoding'i SPAN bazli (ftfy-benzeri) tersine cevirir.
# Yalniz, CP1254 baytlari GECERLI bir UTF-8 cok-baytli dizi olusturan kosulari
# yeniden kurar; geri kalan her sey (ASCII, temiz cok-baytli Turkce karakterler,
# matematik sembolleri) BIREBIR korunur. Bu sayede KARISIK satirlar (bozuk Q +
# temiz A) da guvenle duzeltilir. Degisiklik olmazsa $null doner.
#
# Neden guvenli: temiz bir 'i' (U+0131) CP1254'te 0xFD'ye gider -> gecerli UTF-8
# lead degil -> dokunulmaz; '×' (U+00D7) -> 0xD7 lead degil -> dokunulmaz; '-'
# (U+2212) CP1254'te yok -> dokunulmaz. Sadece C3/C4/C5..+continuation gibi gercek
# mojibake dizileri yeniden cozulur.
function Repair-String {
    param([string]$s)
    if ([string]::IsNullOrEmpty($s)) { return $null }
    $arr = $s.ToCharArray()
    $n = $arr.Length
    $sb = New-Object System.Text.StringBuilder
    $changed = $false
    $i = 0
    while ($i -lt $n) {
        $c = $arr[$i]
        if ([int]$c -lt 0x80) { [void]$sb.Append($c); $i++; continue }
        $b0a = $cp1254.GetBytes([string]$c)
        # Tek bayta gitmiyorsa veya CP1254'te yoksa (replacement '?') -> orijinali koru.
        if ($b0a.Length -ne 1 -or $b0a[0] -eq 0x3F) { [void]$sb.Append($c); $i++; continue }
        $b0 = $b0a[0]
        # YALNIZ gercek metin-mojibake'inin lead baytlari kabul edilir:
        #   C3 = Latin-1 Supplement (ç ö ü Ç Ö Ü ...), C4/C5 = Latin Extended-A (ğ ı ş İ ...),
        #   C2 = Â/NBSP/sembol blogu, E2 = genel noktalama (— ' " ... via E2 80/84/...).
        # Bu, standalone Turkce harflerin CP1254 baytlariyla (ç=E7 ö=F6 ü=FC i=FD s=FE g=F0
        # Ç=C7 Ö=D6 Ü=DC ...) yanlis tetiklenmeyi onler: hicbiri bu sette degil.
        $len = 0
        if     ($b0 -eq 0xC2 -or $b0 -eq 0xC3 -or $b0 -eq 0xC4 -or $b0 -eq 0xC5) { $len = 2 }
        elseif ($b0 -eq 0xE2) { $len = 3 }
        else   { [void]$sb.Append($c); $i++; continue }   # metin-mojibake lead bayti degil
        if ($i + $len -gt $n) { [void]$sb.Append($c); $i++; continue }
        $bytes = New-Object byte[] $len
        $bytes[0] = $b0
        $ok = $true
        for ($k = 1; $k -lt $len; $k++) {
            $bk = $cp1254.GetBytes([string]$arr[$i + $k])
            if ($bk.Length -ne 1 -or $bk[0] -lt 0x80 -or $bk[0] -gt 0xBF) { $ok = $false; break }
            $bytes[$k] = $bk[0]
        }
        if (-not $ok) { [void]$sb.Append($c); $i++; continue }
        try { $dec = $utf8Strict.GetString($bytes) } catch { [void]$sb.Append($c); $i++; continue }
        [void]$sb.Append($dec); $i += $len; $changed = $true
    }
    if (-not $changed) { return $null }
    return $sb.ToString()
}

$roots = @()
if ($Workspace) { $roots = @(Join-Path $StoreRoot $Workspace) }
else { $roots = Get-ChildItem -Path $StoreRoot -Directory -ErrorAction SilentlyContinue | Select-Object -ExpandProperty FullName }

$totalFiles = 0; $changedFiles = 0; $changedFields = 0

foreach ($wsRoot in $roots) {
    $store = Join-Path $wsRoot 'store'
    if (-not (Test-Path $store)) { continue }

    # 1) knowledge/*.json  -> "content" alani
    Get-ChildItem -Path (Join-Path $store 'knowledge') -Filter '*.json' -File -ErrorAction SilentlyContinue | ForEach-Object {
        $totalFiles++
        $obj = [System.IO.File]::ReadAllText($_.FullName, $utf8NoBom) | ConvertFrom-Json
        $fix = Repair-String $obj.content
        if ($fix -ne $null) {
            $changedFields++; $changedFiles++
            Write-Host ("[FIX] {0}  '{1}' -> '{2}'" -f $_.Name, (Trunc $obj.content), (Trunc $fix)) -ForegroundColor Yellow
            if ($Apply) {
                Copy-Item $_.FullName "$($_.FullName).bak-encfix" -Force
                $obj.content = $fix
                # Embedding, ESKI bozuk icerikten hesaplanmis term-vektorudur (orn
                # "kaã":1). Null'la -> Go yuklemede unmarshalVector(nil) doner ve
                # DUZELTILMIS icerikten yeniden hesaplar (memory.Recall backfill).
                if ($obj.PSObject.Properties.Name -contains 'embedding') { $obj.embedding = $null }
                $json = $obj | ConvertTo-Json -Depth 10 -Compress:$false
                [System.IO.File]::WriteAllText($_.FullName, $json, $utf8NoBom)
            }
        }
    }

    # 2) sessions/*/session.jsonl -> her satirda "text" alani
    Get-ChildItem -Path (Join-Path $store 'sessions') -Directory -ErrorAction SilentlyContinue | ForEach-Object {
        $jf = Join-Path $_.FullName 'session.jsonl'
        if (-not (Test-Path $jf)) { return }
        $totalFiles++
        $lines = [System.IO.File]::ReadAllLines($jf, $utf8NoBom)
        $fileChanged = $false
        $outLines = New-Object System.Collections.Generic.List[string]
        foreach ($line in $lines) {
            if ([string]::IsNullOrWhiteSpace($line)) { $outLines.Add($line); continue }
            # Mojibake, ham satirda LITERAL UTF-8 bayt olarak durur; JSON yapisal
            # karakterleri (ASCII < 0x80) Repair-String tarafindan dokunulmaz. Bu yuzden
            # satirin TAMAMINA uygulamak guvenlidir ve JSON round-trip (alan kaybi/
            # yeniden-siralama/escaping farki) riskini tamamen ortadan kaldirir — yalniz
            # mojibake karakterleri degisir, yapinin geri kalani BIREBIR korunur.
            $fix = Repair-String $line
            if ($fix -ne $null) {
                $changedFields++; $fileChanged = $true
                $o = $line | ConvertFrom-Json
                $of = $fix | ConvertFrom-Json   # gosterim icin duzeltilmis text'i cikar
                Write-Host ("[FIX] {0}/session.jsonl  '{1}' -> '{2}'" -f $_.Name, (Trunc $o.text), (Trunc $of.text)) -ForegroundColor Yellow
                $outLines.Add($fix)
            } else {
                $outLines.Add($line)   # degismeyen satir BIREBIR korunur
            }
        }
        if ($fileChanged) {
            $changedFiles++
            if ($Apply) {
                Copy-Item $jf "$jf.bak-encfix" -Force
                # JSONL: her satir + trailing newline (Go appendMessageLocked ile uyumlu)
                [System.IO.File]::WriteAllText($jf, ([string]::Join("`n", $outLines) + "`n"), $utf8NoBom)
            }
        }
    }
}

Write-Host ""
$mode = if ($Apply) { "APPLIED" } else { "DRY-RUN (yazilmadi; -Apply ile uygula)" }
Write-Host ("==> {0}: {1} alan duzeltildi, {2} dosya etkilendi ({3} dosya tarandi)" -f $mode, $changedFields, $changedFiles, $totalFiles) -ForegroundColor Cyan
