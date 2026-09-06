<#
.SYNOPSIS
  AI Identity / 稳定性 只读监控（一次性脚手架，非项目主代码）。

.DESCRIPTION
  在你使用 Claude 期间挂着它，回答两个问题：
    1) 出口 IP 全程有没有变？（神圣不可变身份的硬保证）
    2) 长输出断流时，到底发生了什么？（IP 变了 / mihomo 报错 / 还是客户端·上游侧）

  只读：
    - 经命名管道 verge-mihomo 读 mihomo 的 /connections（不改任何配置）。
    - 经 mixed-port 采样 ipinfo 看出口 IP（唯一外部流量，低频）。
    - 读 service 日志尾部，实时抓 AI 链路错误（403 / reset / eof / timeout ...）。
  绝不修改 Clash 配置，绝不更改最终出口身份，绝不切换节点。

  日志可靠性（为长期挂机 + 事后查证设计）：
    - 屏幕上看到的每一行（含每秒状态行）都即时写入日志文件。
    - 逐行刷盘（Add-Content，无内存缓冲）：Ctrl+C / 关终端 / 脚本崩，已写内容都不丢。
    - 时间戳带完整日期；文件固定 UTF-8。
    - 每个循环都被 try/catch 包住：单次异常只记一行并继续，监控不会静默死掉。

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File .\scripts\monitor-ai-identity.ps1
  powershell -ExecutionPolicy Bypass -File .\scripts\monitor-ai-identity.ps1 -DurationMinutes 480
  powershell -ExecutionPolicy Bypass -File .\scripts\monitor-ai-identity.ps1 -Once
#>
param(
  [int]$PollSeconds      = 3,
  [int]$IpSampleSeconds  = 90,
  [int]$DurationMinutes  = 0,                         # 0 = 一直跑到 Ctrl+C
  [string]$IpInfoUrl     = "https://ipinfo.io/json",
  [string]$MixedPort     = "127.0.0.1:7889",
  [string]$PipeName      = "verge-mihomo",
  [string]$ClashDir      = "$env:APPDATA\io.github.clash-verge-rev.clash-verge-rev",
  [string]$LogDir        = "",                        # 留空 = 当前目录
  [int]$NotableLifeSec   = 15,                        # 连接存活≥此秒数才算"实打实的流"
  [int]$NotableDownKB    = 256,                       # 或下行≥此KB
  [switch]$Once
)

# ---------- 控制器 secret：只用于 Bearer，绝不打印 ----------
$script:secret = ''
$runtimeCfg = Join-Path $ClashDir 'clash-verge.yaml'
if (Test-Path -LiteralPath $runtimeCfg) {
  foreach ($ln in Get-Content -LiteralPath $runtimeCfg) {
    if ($ln -match '^secret\s*:\s*(.*)$') { $script:secret = $matches[1].Trim().Trim('"').Trim("'") }
  }
}

# ---------- 日志（逐行即时刷盘，UTF-8，带日期）----------
$startTime = Get-Date
if (-not $LogDir) { $LogDir = (Get-Location).Path }
$logFile = Join-Path $LogDir ("ai-identity-monitor-{0:yyyyMMdd-HHmmss}.log" -f $startTime)

function Log([string]$line) {
  $msg = ("[{0:yyyy-MM-dd HH:mm:ss}] {1}" -f (Get-Date), $line)
  Write-Host $msg
  # 逐行追加并立即关闭句柄 = 即时刷盘，进程崩溃也不丢已写行
  try { Add-Content -LiteralPath $logFile -Value $msg -Encoding UTF8 } catch { }
}

# ---------- HTTP over named pipe（已实测：mihomo v1.19.x 走 chunked）----------
function Invoke-PipeHttp {
  param([string]$Path, [string]$Auth)
  $pipe = New-Object System.IO.Pipes.NamedPipeClientStream('.', $PipeName, [System.IO.Pipes.PipeDirection]::InOut)
  try {
    $pipe.Connect(2000)
    $req = "GET $Path HTTP/1.1`r`nHost: localhost`r`n"
    if ($Auth) { $req += "Authorization: Bearer $Auth`r`n" }
    $req += "Connection: close`r`n`r`n"
    $b = [System.Text.Encoding]::ASCII.GetBytes($req)
    $pipe.Write($b, 0, $b.Length); $pipe.Flush()
    $ms = New-Object System.IO.MemoryStream
    $buf = New-Object byte[] 8192
    while (($n = $pipe.Read($buf, 0, $buf.Length)) -gt 0) { $ms.Write($buf, 0, $n) }
    return [System.Text.Encoding]::UTF8.GetString($ms.ToArray())
  } finally { $pipe.Dispose() }
}

function ConvertFrom-Chunked {
  param([string]$Body)
  $sb = New-Object System.Text.StringBuilder; $i = 0
  while ($i -lt $Body.Length) {
    $nl = $Body.IndexOf("`r`n", $i); if ($nl -lt 0) { break }
    $hex = (($Body.Substring($i, $nl - $i)) -split ';')[0].Trim(); if (-not $hex) { break }
    $size = 0; try { $size = [Convert]::ToInt32($hex, 16) } catch { break }
    if ($size -eq 0) { break }
    $start = $nl + 2; if ($start + $size -gt $Body.Length) { $size = $Body.Length - $start }
    [void]$sb.Append($Body.Substring($start, $size)); $i = $start + $size + 2
  }
  return $sb.ToString()
}

function Get-Connections {
  try {
    $resp = Invoke-PipeHttp -Path '/connections' -Auth $script:secret
    $parts = $resp -split "`r`n`r`n", 2
    $hdr = $parts[0]; $body = $parts[1]
    if ($hdr -notmatch 'HTTP/1\.\d 200') { $script:lastConnError = (($hdr -split "`r`n")[0]); return $null }
    if ($hdr -match '(?i)transfer-encoding:\s*chunked') { $body = ConvertFrom-Chunked $body }
    $j = $body | ConvertFrom-Json
    return @($j.connections)
  } catch { $script:lastConnError = $_.Exception.Message; return $null }
}

# ---------- 状态 ----------
$script:baselineIP  = $null
$script:baselineOrg = ''
$script:ipChanged   = $false
$script:ipSamples   = 0
$script:ipMismatch  = 0
$script:ipFails     = 0
$script:lastIpSample = (Get-Date).AddSeconds(-99999)
$seen        = @{}                                   # id -> {host, proc, start, down}
$activePrev  = @{}
$notableEnds = 0
$script:nonStaticHits = 0
$script:aiErrorsSeen  = 0
$reportedErr = New-Object System.Collections.Generic.HashSet[string]
$serviceLog  = Join-Path $ClashDir 'logs\service\service_latest.log'

function Sample-ExitIP {
  param([string]$Why)
  $script:lastIpSample = Get-Date
  try {
    $r = Invoke-RestMethod -Uri $IpInfoUrl -Proxy ("http://" + $MixedPort) -TimeoutSec 15
    $script:ipSamples++
    $ip = $r.ip; $org = $r.org; $cc = $r.country
    if (-not $script:baselineIP) {
      $script:baselineIP = $ip; $script:baselineOrg = $org
      Log "BASELINE 出口 IP = $ip  (org=$org country=$cc)  —— 后续都与此比对"
    } elseif ($ip -ne $script:baselineIP) {
      $script:ipChanged = $true; $script:ipMismatch++
      Log "!!! 出口 IP 变了 ($Why)：baseline=$($script:baselineIP) now=$ip (org=$org)  ==> 身份事故，立即停用并排查 IPRoyal"
    } else {
      Log "出口 IP 正常 ($Why)：$ip"
    }
  } catch {
    $script:ipFails++
    Log "IP 采样失败 ($Why)：$($_.Exception.Message)  ==> IPRoyal 可能在拒绝(403)或链路中断"
  }
}

function Scan-LogErrors {
  if (-not (Test-Path -LiteralPath $serviceLog)) { return }
  foreach ($ln in (Get-Content -LiteralPath $serviceLog -Tail 40)) {
    if ($ln -match '(?i)level=(warning|error)' -and
        $ln -match '(?i)(claude|anthropic|openai|chatgpt|codex|cursor|AI-Static|IPRoyal-Static)' -and
        $ln -match '(?i)(err code: \d+|can not connect remote|connection reset|broken pipe|\bEOF\b|i/o timeout|502 bad gateway|connection refused|10054|10060)') {
      $key = $ln.Substring(0, [Math]::Min(90, $ln.Length))
      if (-not $reportedErr.Contains($key)) {
        [void]$reportedErr.Add($key); $script:aiErrorsSeen++
        $clean = ((($ln -replace '(\d{1,3}\.){3}\d{1,3}', '<ip>') -replace '\s+', ' ').Trim())
        Log "!! mihomo AI链路错误：$clean"
      }
    }
  }
}

function Do-Cycle {
  $conns = Get-Connections
  if ($null -eq $conns) {
    Log "读取 /connections 失败：$script:lastConnError"
    return
  }
  $ai = @($conns | Where-Object {
    $_.metadata.host -match '(?i)claude|anthropic|openai|chatgpt|codex|cursor' -or
    $_.metadata.processPath -match '(?i)claude|chatgpt|codex|cursor|openai'
  })
  $cur = @{}; $nonStatic = @()
  foreach ($c in $ai) {
    $cur[$c.id] = $true
    $isStatic = ($c.chains -contains 'IPRoyal-Static') -and ($c.chains -contains 'AI-Static')
    if (-not $isStatic) { $nonStatic += $c }
    if (-not $seen.ContainsKey($c.id)) {
      $proc = if ($c.metadata.processPath) { Split-Path $c.metadata.processPath -Leaf } else { $c.metadata.process }
      $seen[$c.id] = [pscustomobject]@{ host = $c.metadata.host; proc = $proc; start = $c.start; down = [int64]$c.download }
    } else { $seen[$c.id].down = [int64]$c.download }
  }
  # 断流检测：上一轮在、这一轮没了的连接
  foreach ($id in @($activePrev.Keys)) {
    if (-not $cur.ContainsKey($id)) {
      $info = $seen[$id]
      $life = -1; try { $life = [int]((Get-Date) - [datetime]$info.start).TotalSeconds } catch { }
      $downKB = [int]($info.down / 1KB)
      if ($life -ge $NotableLifeSec -or $downKB -ge $NotableDownKB) {
        $script:notableEnds = $script:notableEnds + 1
        Log "连接结束 host=$($info.host) proc=$($info.proc) 存活=${life}s 下行=${downKB}KB （正常完成或断流——和你客户端看到的时刻对照）"
        if (((Get-Date) - $script:lastIpSample).TotalSeconds -ge 20) { Sample-ExitIP "断流后补采" }
      }
    }
  }
  foreach ($c in $nonStatic) {
    $script:nonStaticHits = $script:nonStaticHits + 1
    Log "!!! 非静态 AI 连接 host=$($c.metadata.host) chains=[$($c.chains -join ' / ')]  ==> 泄漏(RED)"
  }
  $script:activePrev = $cur
  $allStatic = if ($ai.Count -gt 0) { ($nonStatic.Count -eq 0) } else { $true }
  $ipShown = if ($script:baselineIP) { $script:baselineIP } else { '?' }
  # 每秒状态行也写入文件（屏幕=磁盘）
  Log ("AI连接={0} 全静态={1} 出口IP={2} 连接结束={3} AI错误={4}" -f `
    $ai.Count, $(if ($allStatic) { 'YES' } else { 'NO!' }), $ipShown, $script:notableEnds, $script:aiErrorsSeen)
  Scan-LogErrors
}

# ---------- 主流程 ----------
Write-Host "== AI Identity 只读监控 =="
Write-Host "   日志(每行即时落盘，崩溃/Ctrl+C 不丢已写内容): $logFile"
Write-Host "   只读 /connections + 低频采样出口 IP + 扫描 mihomo 错误；不改配置、不动身份。"
if (-not $script:secret) { Write-Host "   警告：未从 clash-verge.yaml 读到 controller secret，/connections 可能 401。" }
Log "monitor 启动  poll=${PollSeconds}s  ip-sample=${IpSampleSeconds}s  duration=${DurationMinutes}min  once=$Once"
Sample-ExitIP "启动基线"

try {
  if ($Once) {
    Do-Cycle
  } else {
    while ($true) {
      if ($DurationMinutes -gt 0 -and ((Get-Date) - $startTime).TotalMinutes -ge $DurationMinutes) { break }
      try {
        Do-Cycle
        if (((Get-Date) - $script:lastIpSample).TotalSeconds -ge $IpSampleSeconds) { Sample-ExitIP "周期采样" }
      } catch {
        Log "监控循环异常(已忽略并继续)：$($_.Exception.Message)"
      }
      Start-Sleep -Seconds $PollSeconds
    }
  }
} finally {
  $dur = "{0:hh\:mm\:ss}" -f ((Get-Date) - $startTime)
  Log "===== 会话小结 ====="
  Log "时长: $dur"
  Log "基线出口 IP: $($script:baselineIP)  (org=$($script:baselineOrg))"
  Log ("会话期出口 IP 是否变化: " + $(if ($script:ipChanged) { 'YES  ==> 出现过身份事故！' } else { 'NO  （身份保持）' }))
  Log "IP 采样: $($script:ipSamples) 次 (不匹配=$($script:ipMismatch), 采样失败=$($script:ipFails))"
  Log "mihomo AI 链路错误: $($script:aiErrorsSeen) 条"
  Log "非静态 AI 连接(泄漏): $($script:nonStaticHits) 次"
  Log "实打实的连接结束事件: $notableEnds 次"
  Log "时间线日志: $logFile"
}
