#Requires -Version 7.0
#Requires -RunAsAdministrator
# Manual real-network comparison: installed Throne, then the fork, then Throne restored. Started by hand, see realnet_check.cmd.
param(
    [string]$AppDir = (Join-Path (Split-Path $PSScriptRoot -Parent) 'deployment\windows-amd64'),
    [string]$DataDir = (Join-Path $env:LOCALAPPDATA 'Poros-dev\realnet-data'),
    [string]$LogRoot = (Join-Path $env:LOCALAPPDATA 'Poros-dev\realnet'),
    [string]$ThronePath = 'C:\Program Files\Throne\Throne.exe',
    [int]$IdleSeconds = 60,
    [long]$DownloadBytes = 100000000,
    [string[]]$Redact = @()
)

$ErrorActionPreference = 'Stop'
$runDir = Join-Path $LogRoot (Get-Date -Format 'yyyy-MM-dd_HH-mm-ss')
New-Item -ItemType Directory -Force $runDir | Out-Null

$probeUrls = @(
    'https://www.google.com/generate_204',
    'https://cp.cloudflare.com/generate_204',
    'https://www.youtube.com',
    'https://api.anthropic.com',
    'https://claude.ai',
    'https://chatgpt.com',
    'https://github.com',
    'https://ya.ru',
    'https://vk.com',
    'https://www.gosuslugi.ru'
)

function Write-Step([string]$Text) {
    Write-Host "`n=== $Text" -ForegroundColor Cyan
    Add-Content (Join-Path $runDir 'steps.log') "$(Get-Date -Format 'HH:mm:ss') $Text"
}

function Protect-Text([string]$Text) {
    foreach ($item in $Redact) { $Text = $Text.Replace($item, '<redacted>') }
    $Text = $Text -replace '(?i)\b(vless|vmess|trojan|ssr?|hysteria2?|hy2|tuic|anytls|wg|wireguard|socks5?|naive\+https|throne|poros)://\S+', '$1://<redacted>'
    $Text = $Text -replace '(?i)https?://\S*(/sub|sub/|token|key=|auth)\S*', '<redacted-url>'
    $Text = $Text -replace '(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b', '<uuid>'
    $Text = $Text -replace '(?i)\b(password|passwd|secret|token|uuid|private_key|public_key|short_id|psk)\b(["'':= ]+)[^\s"'',]+', '$1$2<redacted>'
    $Text = $Text -replace '\b[A-Za-z0-9+/_-]{32,}={0,2}', '<long-token>'
    $Text -replace '\b(\d{1,3})\.(\d{1,3})\.\d{1,3}\.\d{1,3}\b', {
        $first = [int]$_.Groups[1].Value
        $second = [int]$_.Groups[2].Value
        $private = $first -in 0, 10, 127, 255 -or ($first -eq 192 -and $second -eq 168) -or ($first -eq 172 -and $second -in 16..31) -or
            ($first -eq 169 -and $second -eq 254) -or ($first -eq 100 -and $second -in 64..127)
        if ($private) { $_.Value } else { "$first.$second.x.x" }
    }
}

function Get-OwnedProcess([string]$Directory) {
    Get-Process | Where-Object { $_.Path -and $_.Path.StartsWith("$Directory\", [StringComparison]::OrdinalIgnoreCase) }
}

function Save-NetworkState([string]$Label) {
    $tunnels = Get-NetAdapter | Where-Object InterfaceDescription -like '*sing-tun*'
    $text = @(
        "tunnel adapters: $(($tunnels | ForEach-Object { "$($_.Name) [$($_.Status)]" }) -join ', ')",
        '--- adapters', (Get-NetAdapter | Where-Object Status -eq 'Up' | Format-Table Name, InterfaceDescription, LinkSpeed -AutoSize | Out-String -Width 160),
        '--- ipv4 routes per interface', (Get-NetRoute -AddressFamily IPv4 | Group-Object InterfaceAlias | Sort-Object Count -Descending | Format-Table Count, Name -AutoSize | Out-String -Width 160),
        '--- default routes', (Get-NetRoute -AddressFamily IPv4 -DestinationPrefix '0.0.0.0/0' -ErrorAction SilentlyContinue | Format-Table NextHop, InterfaceAlias, RouteMetric -AutoSize | Out-String -Width 160),
        '--- dns servers', (Get-DnsClientServerAddress -AddressFamily IPv4 | Where-Object ServerAddresses | Format-Table InterfaceAlias, ServerAddresses -AutoSize | Out-String -Width 160),
        '--- system proxy', (Get-ItemProperty 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings' | Format-List ProxyEnable, ProxyServer | Out-String)
    ) -join "`n"
    Set-Content (Join-Path $runDir "network-$Label.txt") (Protect-Text $text)
}

function Invoke-Probes([string]$Label) {
    $rows = foreach ($url in $probeUrls) {
        $format = '%{http_code} %{time_namelookup} %{time_connect} %{time_appconnect} %{time_total}'
        $output = curl.exe --noproxy '*' -s -o NUL --max-time 15 -w $format $url
        $code, $dns, $connect, $tls, $total = "$output" -split ' '
        [pscustomobject]@{ stage = $Label; url = $url; http = $code; dns_s = $dns; connect_s = $connect; tls_s = $tls; total_s = $total; exitCode = $LASTEXITCODE }
    }
    $rows | Export-Csv (Join-Path $runDir 'probes.csv') -Append -NoTypeInformation
    $rows | Format-Table url, http, total_s, exitCode -AutoSize | Out-Host

    $exit = curl.exe --noproxy '*' -s --max-time 15 https://api.ipify.org
    $exit6 = curl.exe --noproxy '*' -6 -s --max-time 10 https://api64.ipify.org
    $hash = if ($exit) { (Get-FileHash -Algorithm SHA256 -InputStream ([IO.MemoryStream]::new([Text.Encoding]::UTF8.GetBytes("$exit")))).Hash.Substring(0, 12) } else { 'none' }
    [pscustomobject]@{ stage = $Label; exitIpHash = $hash; exitIpMasked = (Protect-Text "$exit"); ipv6Reachable = [bool]$exit6 } |
        Export-Csv (Join-Path $runDir 'exit-ip.csv') -Append -NoTypeInformation
}

function Measure-Processes([string]$Label, [string]$Segment, [string]$Directory, [scriptblock]$Until) {
    $cores = [Environment]::ProcessorCount
    $previous = @{}
    $clock = [Diagnostics.Stopwatch]::StartNew()
    $lastTick = 0.0
    $rows = [Collections.Generic.List[object]]::new()
    while (-not (& $Until $clock.Elapsed.TotalSeconds)) {
        $now = $clock.Elapsed.TotalSeconds
        foreach ($process in Get-OwnedProcess $Directory) {
            $cpu = $process.TotalProcessorTime.TotalSeconds
            if ($previous.ContainsKey($process.Id)) {
                $rows.Add([pscustomobject]@{
                    stage = $Label; segment = $Segment; t_s = [math]::Round($now, 1); process = $process.ProcessName
                    ws_mb = [math]::Round($process.WorkingSet64 / 1MB, 1); private_mb = [math]::Round($process.PrivateMemorySize64 / 1MB, 1)
                    cpu_pct_all_cores = [math]::Round(($cpu - $previous[$process.Id]) / ($now - $lastTick) / $cores * 100, 3)
                })
            }
            $previous[$process.Id] = $cpu
        }
        $lastTick = $now
        Start-Sleep -Seconds 2
    }
    $rows | Export-Csv (Join-Path $runDir 'metrics.csv') -Append -NoTypeInformation
}

function Invoke-Stage([string]$Label, [string]$Directory) {
    Write-Step "$Label`: состояние сети и проверка сайтов"
    Save-NetworkState $Label
    Invoke-Probes $Label

    Write-Step "$Label`: простой $IdleSeconds с, сверни окно в трей и не трогай его"
    Measure-Processes $Label 'idle' $Directory { param($t) $t -ge $IdleSeconds }

    Write-Step "$Label`: загрузка $([math]::Round($DownloadBytes / 1MB)) МБ"
    $speedFile = Join-Path $runDir "speed-$Label.txt"
    $arguments = "--noproxy * -s -o NUL --max-time 120 -w `"%{http_code} %{speed_download} %{time_total}`" `"https://speed.cloudflare.com/__down?bytes=$DownloadBytes`""
    $download = Start-Process curl.exe -ArgumentList $arguments -NoNewWindow -PassThru -RedirectStandardOutput $speedFile
    Measure-Processes $Label 'download' $Directory { param($t) $download.HasExited }
    Get-Content $speedFile | Out-Host
}

function Save-ForkLogs {
    $logs = Join-Path $DataDir 'logs'
    if (-not (Test-Path $logs)) { $logs = Join-Path $DataDir 'config\logs' }
    if (-not (Test-Path $logs)) { Write-Step "логи форка не найдены в $DataDir"; return }
    foreach ($file in Get-ChildItem $logs -Filter '*.log') {
        $tail = (Get-Content $file.FullName -Tail 3000) -join "`n"
        Set-Content (Join-Path $runDir "fork-$($file.Name)") (Protect-Text $tail)
    }
}

function Write-Summary {
    $metrics = Import-Csv (Join-Path $runDir 'metrics.csv')
    $summary = $metrics | Group-Object stage, segment, process | ForEach-Object {
        $private = $_.Group | ForEach-Object { [double]$_.private_mb } | Measure-Object -Average -Maximum
        $cpu = $_.Group | ForEach-Object { [double]$_.cpu_pct_all_cores } | Measure-Object -Average -Maximum
        $first = $_.Group[0]
        [pscustomobject]@{
            stage = $first.stage; segment = $first.segment; process = $first.process
            private_avg_mb = [math]::Round($private.Average, 1); private_max_mb = [math]::Round($private.Maximum, 1)
            cpu_avg_pct = [math]::Round($cpu.Average, 3); cpu_max_pct = [math]::Round($cpu.Maximum, 3)
        }
    }
    $summary | Export-Csv (Join-Path $runDir 'summary.csv') -NoTypeInformation
    $summary | Format-Table -AutoSize | Out-Host
}

$throneDir = Split-Path $ThronePath -Parent
$exe = Join-Path $AppDir 'Poros.exe'
if (-not (Test-Path $exe)) { throw "В $AppDir нет собранного приложения" }

try {
    Save-NetworkState 'before'
    if (Get-OwnedProcess $throneDir) {
        Invoke-Stage 'throne' $throneDir
        Write-Step 'Закрой Throne через трей (Выход). Жду завершения процессов'
        while (Get-OwnedProcess $throneDir) { Start-Sleep -Seconds 1 }
    } else {
        Write-Step 'Throne не запущен, замер установленной версии пропущен'
    }
    Save-NetworkState 'without-vpn'

    Write-Step "Запускаю форк: $exe"
    New-Item -ItemType Directory -Force $DataDir | Out-Null
    Start-Process $exe -ArgumentList '-appdata', "`"$DataDir`""
    Read-Host 'Подключись в форке в режиме TUN и нажми Enter'
    Invoke-Stage 'fork' $AppDir

    Write-Step 'Закрой форк через трей (Выход). Жду завершения процессов'
    while (Get-OwnedProcess $AppDir) { Start-Sleep -Seconds 1 }
    Save-NetworkState 'after-fork'
    Save-ForkLogs
    Write-Summary
}
finally {
    if ((Test-Path $ThronePath) -and -not (Get-OwnedProcess $throneDir)) {
        Write-Step 'Возвращаю Throne'
        foreach ($process in Get-OwnedProcess $AppDir) { Stop-Process -Id $process.Id -Force }
        Start-Process $ThronePath
    }
    Write-Host "`nЛоги: $runDir" -ForegroundColor Green
}
