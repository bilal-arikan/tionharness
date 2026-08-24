# TionHarness — HTTP API uçtan-uca smoke testi.
#
# _Docs\33-DIS-AJAN-OTOMASYONU.md'deki "A. HTTP API Yolu" akışını baştan sona
# doğrular: sağlık → CORS/no-auth → hata sözleşmesi → workspaces → agents →
# oturum aç → working-dir round-trip → GERÇEK sohbet turu (SSE, canlı LLM) →
# çok-turlu bağlam sürekliliği → permission override turu → flow run-stream
# (transform, LLM'siz) → schedule "run now" (otonom teslim) → flow branch routing
# (contains/equals/regex) → branch default arm → parallel fan-out+join (LLM) →
# kalıcılık → temizlik.
# Bir dış ajanın TionHarness'yu API ile sürebildiğini kanıtlar ve regresyonları yakalar.
#
# Önkoşul: TionHarness sunucusu çalışıyor olmalı (varsayılan dev portu :8090).
#
# Kullanım:
#   .\scripts\e2e-smoke.ps1                       # default ws, ilk ajan, varsayılan port
#   .\scripts\e2e-smoke.ps1 -BaseUrl http://127.0.0.1:8095
#   .\scripts\e2e-smoke.ps1 -Workspace WS3 -AgentId AGT3
#   .\scripts\e2e-smoke.ps1 -KeepSession          # test oturumunu silme (incelemek için)
#   .\scripts\e2e-smoke.ps1 -SkipLLM              # canlı LLM turlarını atla (hızlı/ucuz)

param(
    [string]$BaseUrl = "http://127.0.0.1:8090",
    [string]$Workspace = "",          # boş → sunucunun default workspace'i
    [string]$AgentId = "",            # boş → workspace'in ilk (en yeni) ajanı
    [switch]$KeepSession,
    [switch]$SkipLLM
)

$ErrorActionPreference = "Stop"
$script:pass = 0
$script:fail = 0
$script:skip = 0

function Test-Step {
    param([string]$Name, [scriptblock]$Body)
    try {
        $detail = & $Body
        if ($detail -eq "__SKIP__") {
            $script:skip++
            Write-Host ("[SKIP] {0}" -f $Name) -ForegroundColor DarkGray
            return
        }
        $script:pass++
        Write-Host ("[PASS] {0}{1}" -f $Name, $(if ($detail) { " — $detail" } else { "" })) -ForegroundColor Green
    } catch {
        $script:fail++
        Write-Host ("[FAIL] {0} — {1}" -f $Name, $_.Exception.Message) -ForegroundColor Red
    }
}

# Workspace-kapsamlı REST yardımcıları (X-Workspace-Id header'ı).
$headers = @{ "Content-Type" = "application/json" }
if ($Workspace) { $headers["X-Workspace-Id"] = $Workspace }

# -Depth 10: iç içe gövdeler (flow graph → nodes → node alanları) varsayılan
# derinlik 2'de string'e kırpılır → geçersiz JSON. 10 tüm çağrılar için güvenli.
#
# UTF-8 gövde: gövde -Body'ye STRING olarak verilirse Windows PowerShell 5.1
# Invoke-RestMethod onu charset'siz application/json icin Latin-1/ANSI encode eder →
# Turkce karakterler bozulur (mojibake). Bunu önlemek icin JSON'u her zaman UTF-8
# BAYT dizisine cevirip öyle gönderiyoruz (byte[] gövde ham gönderilir). Curl-Raw
# zaten ayni nedenle UTF-8 temp dosya kullanir.
function ConvertTo-Utf8Body { param($Obj) [System.Text.Encoding]::UTF8.GetBytes(($Obj | ConvertTo-Json -Compress -Depth 10)) }
function Api-Get  { param($Path) Invoke-RestMethod -Uri "$BaseUrl$Path" -Headers $headers -TimeoutSec 15 }
function Api-Post { param($Path, $Obj) Invoke-RestMethod -Uri "$BaseUrl$Path" -Method Post -Headers $headers -Body (ConvertTo-Utf8Body $Obj) -TimeoutSec 30 }
function Api-Put  { param($Path, $Obj) Invoke-RestMethod -Uri "$BaseUrl$Path" -Method Put -Headers $headers -Body (ConvertTo-Utf8Body $Obj) -TimeoutSec 15 }
function Api-Del  { param($Path) Invoke-RestMethod -Uri "$BaseUrl$Path" -Method Delete -Headers $headers -TimeoutSec 15 }

# curl ile ham HTTP — Invoke-WebRequest bazı yanıtlarda (auth/redirect) NonInteractive
# modda takılır; curl deterministiktir. Dönüş: @{ Code; Body }.
function Curl-Raw {
    param([string]$Method, [string]$Path, [string]$Body, [string[]]$ExtraHeaders)
    $a = @("-s", "-w", "`n__HTTP_CODE__%{http_code}", "-X", $Method, "$BaseUrl$Path")
    foreach ($h in $ExtraHeaders) { $a += @("-H", $h) }
    $tmp = $null
    if ($PSBoundParameters.ContainsKey('Body')) {
        $a += @("-H", "Content-Type: application/json")
        if ($Workspace) { $a += @("-H", "X-Workspace-Id: $Workspace") }
        # BOM'suz UTF-8 gövde (Go JSON decoder BOM'u reddeder).
        $tmp = [System.IO.Path]::GetTempFileName()
        [System.IO.File]::WriteAllText($tmp, $Body, (New-Object System.Text.UTF8Encoding($false)))
        $a += @("--data", "@$tmp")
    } elseif ($Workspace) {
        $a += @("-H", "X-Workspace-Id: $Workspace")
    }
    $out = (& curl.exe @a) -join "`n"
    if ($tmp) { Remove-Item $tmp -ErrorAction SilentlyContinue }
    $code = ""
    $m = [regex]::Match($out, '__HTTP_CODE__(\d+)\s*$')
    if ($m.Success) { $code = $m.Groups[1].Value; $out = $out.Substring(0, $m.Index) }
    return @{ Code = $code; Body = $out }
}

# Bir gerçek sohbet turu çalıştırır; reply text'ini döndürür. done yoksa/hata
# event'inde throw eder. permissionMode tek-tur kapı override'ı (read-only|ask|auto).
function Invoke-ChatTurn {
    param([string]$Message, [string]$PermissionMode = "")
    $obj = @{ sessionId = $script:sessionId; message = $Message; agentIds = @($script:AgentId) }
    if ($PermissionMode) { $obj["permissionMode"] = $PermissionMode }
    $r = Curl-Raw -Method POST -Path "/api/chat/stream" -Body ($obj | ConvertTo-Json -Compress)
    $raw = $r.Body
    if ($raw -match '"error"\s*:\s*"') { throw "error event/yanit: $raw" }
    if ($raw -notmatch 'event:\s*done') { throw "terminal 'done' event'i yok (tur tamamlanmadi)" }
    $m = [regex]::Match($raw, '"replyMessage"\s*:\s*\{.*?"text"\s*:\s*"((?:[^"\\]|\\.)*)"')
    if (-not $m.Success) { throw "reply event'inde text bulunamadi" }
    $txt = $m.Groups[1].Value
    if (-not $txt.Trim()) { throw "LLM bos cevap dondu" }
    return $txt
}

# Bir graph'ı geçici flow olarak oluşturur, run-stream (SSE) ile koşar, ham gövdeyi
# döndürür; sonra flow'u VE koşunun ürettiği transcript oturumunu siler (workspace
# temiz kalsın). LLM'siz node'lar (transform/branch/delay) için deterministiktir.
function Run-FlowGraph {
    param([hashtable]$Graph, [string]$InputText = "x", [string]$Name = "E2E flow")
    $flow = Api-Post "/api/flows" @{ name = $Name; graph = $Graph }
    if (-not $flow.id) { throw "flow id dönmedi" }
    $before = @{}; (Api-Get "/api/sessions") | ForEach-Object { $before[$_.id] = $true }
    try {
        $r = Curl-Raw -Method POST -Path "/api/flows/$($flow.id)/run-stream" -Body (@{ input = $InputText } | ConvertTo-Json -Compress)
        return $r.Body
    } finally {
        Api-Del "/api/flows/$($flow.id)" | Out-Null
        if (-not $KeepSession) {
            foreach ($s in (Api-Get "/api/sessions")) {
                if (-not $before.ContainsKey($s.id)) { Api-Del "/api/sessions/$($s.id)" | Out-Null }
            }
        }
    }
}

Write-Host "==> TionHarness E2E smoke testi — $BaseUrl (ws='$(if($Workspace){$Workspace}else{'<default>'})', llm=$(if($SkipLLM){'kapali'}else{'acik'}))" -ForegroundColor Cyan

# 1) Sağlık
Test-Step "GET /health" {
    $h = Api-Get "/health"
    if ($h.status -ne "ok") { throw "status='$($h.status)' (beklenen 'ok')" }
    "service=$($h.service) v$($h.version) workspaces=$($h.workspaces)"
}

# 2) CORS / no-auth — tarayıcı içinden çağrılabilirlik (doc 33 §A.1)
Test-Step "OPTIONS preflight (CORS açık, auth yok)" {
    $a = @("-s", "-i", "-X", "OPTIONS", "$BaseUrl/api/agents", "-H", "Origin: http://example.com")
    $out = (& curl.exe @a) -join "`n"
    if ($out -notmatch '204') { throw "204 No Content beklenirdi" }
    if ($out -notmatch 'Access-Control-Allow-Origin:\s*\*') { throw "ACAO '*' header'i yok" }
    "204 + ACAO=*"
}

# 3) Hata sözleşmesi — eksik alan 400, geçersiz oturum 404, ikisi de {error} JSON
Test-Step "Hata sözleşmesi (400 eksik alan, 404 geçersiz oturum)" {
    $bad = Curl-Raw -Method POST -Path "/api/chat/stream" -Body '{"message":"x"}'
    if ($bad.Code -ne "400") { throw "eksik sessionId icin 400 beklendi, gelen $($bad.Code)" }
    if ($bad.Body -notmatch '"error"') { throw "400 govdesi {error} degil: $($bad.Body)" }
    $nf = Curl-Raw -Method GET -Path "/api/sessions/SESnope/workdir"
    if ($nf.Code -ne "404") { throw "gecersiz oturum icin 404 beklendi, gelen $($nf.Code)" }
    if ($nf.Body -notmatch '"error"') { throw "404 govdesi {error} degil: $($nf.Body)" }
    "400+404 ikisi de {error}"
}

# 4) Workspaces listesi
Test-Step "GET /api/workspaces" {
    $ws = Api-Get "/api/workspaces"
    if (-not $ws -or $ws.Count -lt 1) { throw "hiç workspace yok" }
    "$($ws.Count) workspace"
}

# 5) Agents listesi (+ test ajanını seç)
Test-Step "GET /api/agents" {
    $agents = Api-Get "/api/agents"
    if (-not $agents -or $agents.Count -lt 1) { throw "hiç ajan yok; önce bir ajan oluştur" }
    if (-not $AgentId) { $script:AgentId = $agents[0].id }
    $a = $agents | Where-Object { $_.id -eq $script:AgentId } | Select-Object -First 1
    if (-not $a) { throw "ajan '$script:AgentId' bulunamadı" }
    "test ajanı=$($a.id) ($($a.name), $($a.provider)/$($a.model))"
}

# 6) Oturum aç
$script:sessionId = $null
Test-Step "POST /api/sessions" {
    $s = Api-Post "/api/sessions" @{ agentId = $script:AgentId; title = "E2E smoke" }
    if (-not $s.id) { throw "oturum id dönmedi" }
    $script:sessionId = $s.id
    "session=$($s.id)"
}

# 7) Working-directory round-trip (oturum-başına cwd, doc 26)
Test-Step "PUT+GET /api/sessions/{id}/workdir (cwd round-trip)" {
    if (-not $script:sessionId) { throw "oturum yok" }
    $repo = (Resolve-Path "$PSScriptRoot\..").Path     # TionHarness repo kökü (mevcut + git)
    $set = Api-Put "/api/sessions/$($script:sessionId)/workdir" @{ dir = $repo }
    if (-not $set.exists) { throw "set sonrası exists=false ($repo)" }
    $got = Api-Get "/api/sessions/$($script:sessionId)/workdir"
    if ($got.dir -ne $set.dir) { throw "GET dir uyuşmuyor" }
    # Reset → effective workspace default'a düşmeli
    $reset = Api-Put "/api/sessions/$($script:sessionId)/workdir" @{ dir = "" }
    if ($reset.dir -ne "") { throw "reset sonrası dir boş değil" }
    "set(git=$($set.isGitRepo)) → get → reset OK"
}

# 8) GERÇEK sohbet turu (SSE) — canlı LLM çağrısı
$marker = "TionHarness smoke OK"
Test-Step "POST /api/chat/stream (gerçek LLM turu)" {
    if (-not $script:sessionId) { throw "oturum yok (önceki adım başarısız)" }
    if ($SkipLLM) { return "__SKIP__" }
    $reply = Invoke-ChatTurn "Reply with exactly this phrase and nothing else: $marker"
    "done OK, reply='$reply'"
}

# 9) Çok-turlu bağlam sürekliliği — 2. tur önceki turdaki bilgiyi hatırlamalı
Test-Step "Çok-turlu bağlam sürekliliği (recall)" {
    if (-not $script:sessionId) { throw "oturum yok" }
    if ($SkipLLM) { return "__SKIP__" }
    Invoke-ChatTurn "Remember this codeword for later: ZEPHYR-7. Just acknowledge with OK." | Out-Null
    $reply = Invoke-ChatTurn "What was the codeword I told you? Reply with only the codeword."
    if ($reply -notmatch 'ZEPHYR-?7') { throw "2. tur codeword'u hatirlamadi: '$reply'" }
    "codeword hatirlandi: '$reply'"
}

# 10) permissionMode tek-tur override (read-only) — tur yine tamamlanmalı
Test-Step "permissionMode=read-only override turu" {
    if (-not $script:sessionId) { throw "oturum yok" }
    if ($SkipLLM) { return "__SKIP__" }
    $reply = Invoke-ChatTurn -Message "Reply with the single word: READY" -PermissionMode "read-only"
    "read-only tur tamamlandi, reply='$reply'"
}

# 11) Flow (orchestration) — transform node ile LLM'siz, deterministik koşu (doc 7).
#     POST /api/flows → POST /api/flows/{id}/run-stream (SSE node+reply) → DELETE.
Test-Step "Flow run-stream (transform node, LLM'siz)" {
    $graph = @{
        start = "t1"
        nodes = @(@{ id = "t1"; type = "transform"; title = "Echo"; template = "flow-ok: {{input}}"; next = "" })
    }
    $raw = Run-FlowGraph -Graph $graph -InputText "PING" -Name "E2E smoke flow"
    if ($raw -match 'event:\s*error') { throw "flow error event: $raw" }
    if ($raw -notmatch 'event:\s*node')  { throw "node event'i yok" }
    if ($raw -notmatch 'event:\s*reply') { throw "terminal reply event'i yok" }
    if ($raw -notmatch '"status"\s*:\s*"success"') { throw "run.status=success degil" }
    if ($raw -notmatch 'flow-ok: PING') { throw "transform çıktısı (flow-ok: PING) yok" }
    "node+reply OK, status=success, output='flow-ok: PING'"
}

# 12) Schedule "run now" — POST /api/schedules/{id}/run senkron fırlatır, enabled'dan
#     bağımsız (doc 24). Devre-dışı + uzak cron ile oluştur → manuel tetik → satırda
#     lastDeliveryStatus=success bekle → temizle. Teslim ajana otonom tur açar (LLM).
Test-Step "Schedule run-now (manuel tetik, otonom teslim)" {
    if ($SkipLLM) { return "__SKIP__" }
    # Tetik penceresinde oluşan otonom oturumu sonra temizlemek için öncesini al.
    $before = @{}; (Api-Get "/api/sessions") | ForEach-Object { $before[$_.id] = $true }
    $sch = Api-Post "/api/schedules" @{
        agentId = $script:AgentId
        cronExpr = "0 0 1 1 *"     # yılda bir (Oca 1) — test sırasında kendiliğinden fırlamaz
        prompt = "Reply with the single word: PONG."
        enabled = $false           # otomatik fırlamasın; run-now enabled'dan bağımsız
    }
    if (-not $sch.id) { throw "schedule id dönmedi" }
    try {
        $row = Api-Post "/api/schedules/$($sch.id)/run" @{}
        if ($row.lastDeliveryStatus -ne "success") {
            throw "lastDeliveryStatus='$($row.lastDeliveryStatus)' (hata: $($row.lastDeliveryError))"
        }
        if (-not $row.lastRunAt -or $row.lastRunAt -le 0) { throw "lastRunAt set edilmedi" }
        # Teslimden doğan otonom oturum(lar)ı temizle.
        $cleaned = 0
        if (-not $KeepSession) {
            foreach ($s in (Api-Get "/api/sessions")) {
                if (-not $before.ContainsKey($s.id)) { Api-Del "/api/sessions/$($s.id)" | Out-Null; $cleaned++ }
            }
        }
        "lastDeliveryStatus=success, lastRunAt set; otonom oturum temizlendi=$cleaned"
    } finally {
        Api-Del "/api/schedules/$($sch.id)" | Out-Null
    }
}

# 13) Flow branch routing — son çıktıya göre dallanma (contains/equals/regex).
#     LLM'siz: transform kaynak değeri üretir, branch yönlendirir, seçilen arm
#     belirgin token basar. Her mod ayrı flow ile (engine.go evalBranch/branchArmMatches).
Test-Step "Flow branch routing (contains/equals/regex)" {
    $cases = @(
        @{ mode = "contains"; value = "hello world"; pattern = "world" },
        @{ mode = "equals";   value = "yes";         pattern = "yes" },
        @{ mode = "regex";    value = "order-42";    pattern = "order-\d+" }
    )
    foreach ($c in $cases) {
        $graph = @{
            start = "src"
            nodes = @(
                @{ id = "src"; type = "transform"; title = "Source"; template = $c.value; next = "br" },
                @{ id = "br"; type = "branch"; title = "Route"; matchMode = $c.mode; branches = @(
                    @{ contains = $c.pattern; next = "hit" },
                    @{ contains = ""; next = "miss" }
                ) },
                @{ id = "hit";  type = "transform"; title = "Hit";  template = "ROUTED-HIT";  next = "" },
                @{ id = "miss"; type = "transform"; title = "Miss"; template = "ROUTED-MISS"; next = "" }
            )
        }
        $raw = Run-FlowGraph -Graph $graph -InputText $c.value -Name "E2E branch $($c.mode)"
        if ($raw -match 'event:\s*error') { throw "$($c.mode): error event" }
        if ($raw -notmatch '"status"\s*:\s*"success"') { throw "$($c.mode): status success degil" }
        if ($raw -notmatch 'ROUTED-HIT') { throw "$($c.mode): hit arm'a yönlenmedi" }
        if ($raw -match 'ROUTED-MISS')   { throw "$($c.mode): yanlışlıkla default arm'a düştü" }
    }
    "3 mod da hit arm'a yönlendi: contains, equals, regex"
}

# 14) Flow branch default (else) arm — hiçbir arm tutmazsa boş-Contains default'a düşer.
Test-Step "Flow branch default arm (no-match → else)" {
    $graph = @{
        start = "src"
        nodes = @(
            @{ id = "src"; type = "transform"; title = "Source"; template = "abc"; next = "br" },
            @{ id = "br"; type = "branch"; title = "Route"; matchMode = "contains"; branches = @(
                @{ contains = "zzz"; next = "hit" },
                @{ contains = ""; next = "miss" }
            ) },
            @{ id = "hit";  type = "transform"; title = "Hit";  template = "ROUTED-HIT";  next = "" },
            @{ id = "miss"; type = "transform"; title = "Miss"; template = "ROUTED-MISS"; next = "" }
        )
    }
    $raw = Run-FlowGraph -Graph $graph -InputText "abc" -Name "E2E branch default"
    if ($raw -notmatch '"status"\s*:\s*"success"') { throw "status success degil" }
    if ($raw -notmatch 'ROUTED-MISS') { throw "default (else) arm'a yönlenmedi" }
    if ($raw -match 'ROUTED-HIT')     { throw "yanlışlıkla hit arm'a düştü" }
    "no-match → default (else) arm OK"
}

# 15) Flow parallel fan-out + join — iki agent node eşzamanlı koşar, çıktıları
#     [title]\nout formatında birleşir (engine.go runParallel). Gerçek LLM (2 çağrı).
Test-Step "Flow parallel fan-out + join (LLM)" {
    if ($SkipLLM) { return "__SKIP__" }
    $graph = @{
        start = "fan"
        nodes = @(
            @{ id = "fan"; type = "parallel"; title = "Fan"; parallel = @("a1", "a2"); joinNext = "" },
            @{ id = "a1"; type = "agent"; title = "A1"; agentId = $script:AgentId; prompt = "Reply with exactly one word: ALPHA"; next = "" },
            @{ id = "a2"; type = "agent"; title = "A2"; agentId = $script:AgentId; prompt = "Reply with exactly one word: BETA"; next = "" }
        )
    }
    $raw = Run-FlowGraph -Graph $graph -InputText "go" -Name "E2E parallel"
    if ($raw -match 'event:\s*error') { throw "error event: $raw" }
    if ($raw -notmatch '"status"\s*:\s*"success"') { throw "status success degil" }
    if ($raw -notmatch 'ALPHA') { throw "a1 (ALPHA) çıktısı join'de yok" }
    if ($raw -notmatch 'BETA')  { throw "a2 (BETA) çıktısı join'de yok" }
    "2 paralel agent join edildi: ALPHA + BETA"
}

# 16) Kalıcılık — kullanıcı + asistan mesajları diske yazıldı mı
Test-Step "GET /api/sessions/{id}/messages (kalıcılık)" {
    if (-not $script:sessionId) { throw "oturum yok" }
    $msgs = Api-Get "/api/sessions/$($script:sessionId)/messages"
    if ($SkipLLM) { return "LLM atlandı; $($msgs.Count) mesaj" }
    $roles = ($msgs | ForEach-Object { $_.role })
    if ($roles -notcontains "user")      { throw "kullanıcı mesajı kalıcılaşmadı" }
    if ($roles -notcontains "assistant") { throw "asistan mesajı kalıcılaşmadı" }
    "$($msgs.Count) mesaj (4 tur: user+assistant çiftleri)"
}

# 17) Temizlik
Test-Step "DELETE /api/sessions/{id} (temizlik)" {
    if (-not $script:sessionId) { throw "oturum yok" }
    if ($KeepSession) { return "atlandı (-KeepSession)" }
    Api-Del "/api/sessions/$($script:sessionId)" | Out-Null
    "silindi=$($script:sessionId)"
}

# Özet
$total = $script:pass + $script:fail
Write-Host ""
$tail = if ($script:skip -gt 0) { " ($($script:skip) atlandı)" } else { "" }
Write-Host ("==> Sonuç: {0}/{1} geçti{2}" -f $script:pass, $total, $tail) -ForegroundColor $(if ($script:fail -eq 0) { "Green" } else { "Red" })
if ($script:fail -gt 0) { exit 1 }
