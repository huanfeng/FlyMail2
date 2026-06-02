#requires -Version 7.0
<#
.SYNOPSIS
    FlyMail Windows 开发脚本（菜单式，后台托管前后端进程）。
.DESCRIPTION
    一站式进行 flymail 的前后端「后台运行/停止/重启」、构建、测试、数据库管理。
    开发服务以后台进程方式运行（不再弹新窗口），由本脚本统一启停，PID 记录在 .dev/，
    日志统一写入 logs/。即使关闭本菜单，服务仍在后台运行，可再次运行本脚本查看状态/停止。
    用法：
      .\dev.ps1            # 进入交互菜单
      .\dev.ps1 up         # 直接启动前后端（别名见 $Aliases）
      .\dev.ps1 2          # 直接执行菜单项 2
.NOTES
    后端：flymail/backend（module flymail，无 CGO，glebarez 纯 Go SQLite），默认仅监听 127.0.0.1。
    前端：flymail/frontend（React + Vite + pnpm，dev 代理 /api -> 后端）。
    日志：logs/flymail.log（后端应用日志，按大小轮转）、logs/frontend.out.log（前端 Vite）、
          logs/backend.err.log / frontend.err.log（进程错误/崩溃）。
#>

param(
    [Parameter(Position = 0)]
    [string]$Action = ''
)

$ErrorActionPreference = 'Stop'

# ---------- 路径与配置 ----------
$Root          = $PSScriptRoot
$Backend       = Join-Path $Root 'backend'
$Frontend      = Join-Path $Root 'frontend'
$DataDir       = Join-Path $Backend 'data'
$DistDir       = Join-Path $Backend 'web\dist'
$BinDir        = Join-Path $Root 'bin'
$BinPath       = Join-Path $BinDir 'flymail.exe'
$DevBackendExe = Join-Path $BinDir 'flymail-dev.exe'
$LogDir        = Join-Path $Root 'logs'
$DevDir        = Join-Path $Root '.dev'

$BackendPort  = if ($env:FLYMAIL_SERVER_PORT) { $env:FLYMAIL_SERVER_PORT } else { '8080' }
# FlyMail 专属前端开发端口（避开 Vite 默认 5173，减少与其它项目冲突）。
$FrontendPort = if ($env:FLYMAIL_WEB_PORT) { $env:FLYMAIL_WEB_PORT } else { '5390' }

# ---------- 统一环境 ----------
$env:CGO_ENABLED = '0'                       # 无 CGO（glebarez 纯 Go SQLite）
if (-not $env:GOPROXY) { $env:GOPROXY = 'https://goproxy.cn,direct' }
$env:FLYMAIL_WEB_PORT   = $FrontendPort       # 传给 vite，确保 strictPort 生效
$env:FLYMAIL_LOG_DIR    = $LogDir             # 后端日志统一落到 logs/
$env:FLYMAIL_SERVER_HOST = '127.0.0.1'        # 仅监听本机，避免防火墙弹窗

$pwshExe = (Get-Command pwsh -ErrorAction SilentlyContinue).Source
if (-not $pwshExe) { $pwshExe = 'powershell' }

# ---------- 工具函数 ----------
function Write-Title($text) { Write-Host "`n==== $text ====" -ForegroundColor Cyan }
function Write-Ok($text)    { Write-Host "✓ $text" -ForegroundColor Green }
function Write-Warn($text)  { Write-Host "! $text" -ForegroundColor Yellow }
function Write-Err($text)   { Write-Host "✗ $text" -ForegroundColor Red }

function Assert-Tool($name, $hint) {
    if (-not (Get-Command $name -ErrorAction SilentlyContinue)) {
        Write-Err "未找到 $name。$hint"
        return $false
    }
    return $true
}

function Ensure-Dirs {
    New-Item -ItemType Directory -Force -Path $LogDir | Out-Null
    New-Item -ItemType Directory -Force -Path $DevDir | Out-Null
}

function Ensure-AdminHint {
    if (-not (Test-Path (Join-Path $DataDir 'flymail.db'))) {
        Write-Warn '尚未初始化数据库/管理员，登录前请先执行「数据库初始化」'
    }
}

# 保证 web/dist 存在最小占位（dev 后端 embed 需要；正式 UI 由 Vite 提供）。
function Ensure-DistPlaceholder {
    $index = Join-Path $DistDir 'index.html'
    if (-not (Test-Path $index)) {
        New-Item -ItemType Directory -Force -Path $DistDir | Out-Null
        Set-Content -Path $index -Encoding utf8 -Value '<!doctype html><title>FlyMail dev</title>开发模式请访问 Vite 前端端口。'
        Write-Warn "web/dist 缺失，已生成占位 index.html（开发 UI 走 Vite，正式请执行「完整构建」）"
    }
}

# ---------- 进程管理 ----------
function Get-PidFile($name) { Join-Path $DevDir "$name.pid" }

# 返回正在运行的进程对象；若 pid 文件失效则清理并返回 $null。
function Get-DevProcess($name) {
    $pf = Get-PidFile $name
    if (-not (Test-Path $pf)) { return $null }
    $procId = (Get-Content $pf -ErrorAction SilentlyContinue | Select-Object -First 1)
    if (-not $procId) { return $null }
    $p = Get-Process -Id ([int]$procId) -ErrorAction SilentlyContinue
    if (-not $p) { Remove-Item $pf -ErrorAction SilentlyContinue; return $null }
    return $p
}

function Stop-DevService($name, $label) {
    $p = Get-DevProcess $name
    if (-not $p) { Write-Warn "$label 未在运行"; return }
    # /T 连同子进程一并结束（pnpm->node、go 子进程等）。
    taskkill /PID $p.Id /T /F 2>$null | Out-Null
    Remove-Item (Get-PidFile $name) -ErrorAction SilentlyContinue
    Write-Ok "$label 已停止 (PID $($p.Id))"
}

function Start-Backend {
    if (-not (Assert-Tool 'go' '请先安装 Go')) { return }
    if (Get-DevProcess 'backend') { Write-Warn '后端已在运行（用「重启后端」回收）'; return }
    Ensure-Dirs
    Ensure-AdminHint
    Ensure-DistPlaceholder
    Write-Title "编译后端 (go build -> flymail-dev.exe)"
    New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
    Push-Location $Backend
    try { go build -o $DevBackendExe . } finally { Pop-Location }
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path $DevBackendExe)) { Write-Err '后端编译失败，未启动'; return }
    $out = Join-Path $LogDir 'backend.out.log'
    $err = Join-Path $LogDir 'backend.err.log'
    $p = Start-Process -FilePath $DevBackendExe `
        -ArgumentList 'server', '--data-dir', $DataDir `
        -WorkingDirectory $Backend -WindowStyle Hidden -PassThru `
        -RedirectStandardOutput $out -RedirectStandardError $err
    $p.Id | Set-Content (Get-PidFile 'backend')
    Write-Ok "后端已启动 (PID $($p.Id))   http://127.0.0.1:$BackendPort"
    Write-Host "  应用日志: $(Join-Path $LogDir 'flymail.log')" -ForegroundColor DarkGray
}

function Start-Frontend {
    if (-not (Assert-Tool 'pnpm' '请先安装 pnpm：npm i -g pnpm')) { return }
    if (Get-DevProcess 'frontend') { Write-Warn '前端已在运行（用「重启前端」回收）'; return }
    Ensure-Dirs
    if (-not (Test-Path (Join-Path $Frontend 'node_modules'))) {
        Write-Warn '未检测到 node_modules，建议先执行「安装前端依赖」'
    }
    $out = Join-Path $LogDir 'frontend.out.log'
    $err = Join-Path $LogDir 'frontend.err.log'
    # 用 pwsh 包一层以兼容 pnpm 的 .cmd/.ps1 shim；子进程(node)的输出经继承句柄重定向到文件。
    $inner = "Set-Location '$Frontend'; `$env:FLYMAIL_WEB_PORT='$FrontendPort'; pnpm dev"
    $p = Start-Process -FilePath $pwshExe `
        -ArgumentList '-NoProfile', '-NoLogo', '-Command', $inner `
        -WindowStyle Hidden -PassThru `
        -RedirectStandardOutput $out -RedirectStandardError $err
    $p.Id | Set-Content (Get-PidFile 'frontend')
    Write-Ok "前端已启动 (PID $($p.Id))   http://localhost:$FrontendPort"
    Write-Host "  日志: $out" -ForegroundColor DarkGray
}

function Start-All {
    Write-Title '启动开发服务（后端 + 前端，后台运行）'
    Start-Backend
    Start-Sleep -Milliseconds 500
    Start-Frontend
    Write-Host "`n后端: http://127.0.0.1:$BackendPort    前端: http://localhost:$FrontendPort" -ForegroundColor Magenta
}

function Stop-All {
    Write-Title '停止开发服务'
    Stop-DevService 'frontend' '前端'
    Stop-DevService 'backend'  '后端'
}

function Restart-All  { Stop-All;                       Start-Sleep -Milliseconds 500; Start-All }
function Restart-Backend  { Stop-DevService 'backend'  '后端'; Start-Sleep -Milliseconds 400; Start-Backend }
function Restart-Frontend { Stop-DevService 'frontend' '前端'; Start-Sleep -Milliseconds 400; Start-Frontend }

function Show-Status {
    Write-Title '开发服务状态'
    foreach ($s in @(
            @{ n = 'backend';  l = '后端'; port = $BackendPort },
            @{ n = 'frontend'; l = '前端'; port = $FrontendPort })) {
        $p = Get-DevProcess $s.n
        if ($p) {
            $listening = $false
            try {
                $listening = [bool](Get-NetTCPConnection -State Listen -LocalPort ([int]$s.port) -ErrorAction SilentlyContinue)
            } catch {}
            $portInfo = if ($listening) { ":$($s.port) 监听中" } else { ":$($s.port) 端口未监听?" }
            Write-Ok ("{0}: 运行中  PID {1}  {2}" -f $s.l, $p.Id, $portInfo)
        } else {
            Write-Warn ("{0}: 未运行" -f $s.l)
        }
    }
    Write-Host "日志目录: $LogDir" -ForegroundColor DarkGray
}

function Show-Logs {
    Write-Title '日志末尾快照'
    $files = [ordered]@{
        '后端应用 (flymail.log)' = Join-Path $LogDir 'flymail.log'
        '前端 (frontend.out.log)' = Join-Path $LogDir 'frontend.out.log'
        '后端错误 (backend.err.log)' = Join-Path $LogDir 'backend.err.log'
    }
    foreach ($k in $files.Keys) {
        $f = $files[$k]
        Write-Host "`n--- $k ---" -ForegroundColor Cyan
        if (Test-Path $f) { Get-Content $f -Tail 30 } else { Write-Host '(暂无)' -ForegroundColor DarkGray }
    }
}

function Watch-Logs {
    $target = Read-Host '实时跟踪哪个? (b=后端应用 / f=前端，回车=后端)'
    $file = switch ($target) {
        'f' { Join-Path $LogDir 'frontend.out.log' }
        default { Join-Path $LogDir 'flymail.log' }
    }
    if (-not (Test-Path $file)) { New-Item -ItemType File -Force -Path $file | Out-Null }
    Write-Ok "在新窗口跟踪：$file（关闭该窗口即可，不影响服务）"
    Start-Process $pwshExe -ArgumentList '-NoExit', '-NoProfile', '-Command', "Get-Content -Path '$file' -Wait -Tail 50" | Out-Null
}

# ---------- 构建 / 测试 / 数据库 ----------
function Invoke-FrontendInstall {
    Write-Title '安装前端依赖 (pnpm install)'
    if (-not (Assert-Tool 'pnpm' '请先安装 pnpm：npm i -g pnpm')) { return }
    Push-Location $Frontend
    try { pnpm install } finally { Pop-Location }
    Write-Ok '前端依赖安装完成'
}

function Invoke-BuildFrontend {
    Write-Title '构建前端 (-> backend/web/dist)'
    if (-not (Assert-Tool 'pnpm' '请先安装 pnpm')) { return }
    Push-Location $Frontend
    try {
        if (-not (Test-Path (Join-Path $Frontend 'node_modules'))) { pnpm install }
        pnpm build
    } finally { Pop-Location }
    if (Test-Path (Join-Path $DistDir 'index.html')) { Write-Ok "前端已构建到 $DistDir" }
    else { Write-Err '前端构建产物未生成，请检查 vite.config 的 outDir' }
}

function Invoke-BuildBackend {
    Write-Title '构建后端二进制 (无 CGO, 含 go embed)'
    if (-not (Assert-Tool 'go' '请先安装 Go')) { return }
    if (-not (Test-Path (Join-Path $DistDir 'index.html'))) {
        Write-Warn 'backend/web/dist 缺少前端产物，embed 可能为空。建议先执行「构建前端」或「完整构建」'
    }
    New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
    Push-Location $Backend
    try { go build -o $BinPath . } finally { Pop-Location }
    if (Test-Path $BinPath) { Write-Ok "已生成：$BinPath ($([math]::Round((Get-Item $BinPath).Length/1MB,1)) MB)" }
}

function Invoke-BuildAll {
    Write-Title '完整构建（前端 -> 后端，产出单二进制）'
    Invoke-BuildFrontend
    Invoke-BuildBackend
    Write-Ok '完整构建完成：前端已 embed 进单一可执行文件'
}

function Invoke-Test {
    Write-Title '运行后端测试 (go test ./...)'
    if (-not (Assert-Tool 'go' '请先安装 Go')) { return }
    Push-Location $Backend
    try { go test ./... } finally { Pop-Location }
}

function Invoke-Lint {
    Write-Title '代码检查 (go vet + gofmt -l)'
    if (-not (Assert-Tool 'go' '请先安装 Go')) { return }
    Push-Location $Backend
    try {
        go vet ./...
        $unformatted = gofmt -l .
        if ($unformatted) { Write-Warn "以下文件未格式化（可用 gofmt -w 修复）：`n$unformatted" }
        else { Write-Ok 'gofmt 检查通过' }
    } finally { Pop-Location }
}

function Invoke-DBInit {
    Write-Title '数据库初始化（创建管理员账户）'
    if (-not (Assert-Tool 'go' '请先安装 Go')) { return }
    $user = Read-Host '管理员用户名 (默认 admin)'
    if ([string]::IsNullOrWhiteSpace($user)) { $user = 'admin' }
    $pass = Read-Host '管理员密码 (开发默认 admin，直接回车即用)'
    if ([string]::IsNullOrWhiteSpace($pass)) { $pass = 'admin'; Write-Warn '使用开发默认密码 admin，生产请用「重置管理员密码」修改' }
    Push-Location $Backend
    try { go run . db init --admin-user $user --admin-pass $pass --data-dir $DataDir } finally { Pop-Location }
}

function Invoke-ResetPassword {
    Write-Title '重置管理员密码'
    if (-not (Assert-Tool 'go' '请先安装 Go')) { return }
    $user = Read-Host '管理员用户名 (默认 admin)'
    if ([string]::IsNullOrWhiteSpace($user)) { $user = 'admin' }
    $pass = Read-Host '新密码'
    if ([string]::IsNullOrWhiteSpace($pass)) { Write-Err '密码不能为空'; return }
    Push-Location $Backend
    try { go run . db reset-admin-password --admin-user $user --admin-pass $pass --data-dir $DataDir } finally { Pop-Location }
}

function Invoke-RunBinary {
    Write-Title "运行已构建的二进制 (server, :$BackendPort)"
    if (-not (Test-Path $BinPath)) { Write-Err "未找到 $BinPath，请先执行「构建后端二进制」或「完整构建」"; return }
    & $BinPath server --data-dir $DataDir
}

function Invoke-Clean {
    Write-Title '清理'
    Stop-All
    if (Test-Path $BinDir) { Remove-Item -Recurse -Force $BinDir; Write-Ok '已删除 bin/' }
    $ansLog = Read-Host '是否清空 logs/ 日志目录？(y/N)'
    if ($ansLog -eq 'y' -or $ansLog -eq 'Y') {
        if (Test-Path $LogDir) { Remove-Item -Recurse -Force $LogDir; Write-Ok '已清空 logs/' }
    }
    $ans = Read-Host '是否同时清空数据目录 data/（数据库与附件，不可恢复）？(y/N)'
    if ($ans -eq 'y' -or $ans -eq 'Y') {
        if (Test-Path $DataDir) { Remove-Item -Recurse -Force $DataDir; Write-Ok '已清空 data/' }
    } else { Write-Host '保留 data/' }
    Write-Warn '注意：未删除 backend/web/dist（go embed 需要它存在；如需刷新请执行「构建前端」）'
}

# ---------- 菜单分发 ----------
$Actions = [ordered]@{
    '1'  = @{ Name = '安装前端依赖 (pnpm install)';            Fn = ${function:Invoke-FrontendInstall} }
    '2'  = @{ Name = '启动开发服务 (后端+前端, 后台)';         Fn = ${function:Start-All} }
    '3'  = @{ Name = '停止开发服务';                           Fn = ${function:Stop-All} }
    '4'  = @{ Name = '重启开发服务 (前后端)';                  Fn = ${function:Restart-All} }
    '5'  = @{ Name = '重启后端';                               Fn = ${function:Restart-Backend} }
    '6'  = @{ Name = '重启前端';                               Fn = ${function:Restart-Frontend} }
    '7'  = @{ Name = '查看状态';                               Fn = ${function:Show-Status} }
    '8'  = @{ Name = '查看日志 (末尾快照)';                    Fn = ${function:Show-Logs} }
    '9'  = @{ Name = '实时跟踪日志 (新窗口查看器)';            Fn = ${function:Watch-Logs} }
    '10' = @{ Name = '构建前端 (-> backend/web/dist)';         Fn = ${function:Invoke-BuildFrontend} }
    '11' = @{ Name = '构建后端二进制 (无 CGO, 含 embed)';      Fn = ${function:Invoke-BuildBackend} }
    '12' = @{ Name = '完整构建 (前端->后端, 单二进制)';        Fn = ${function:Invoke-BuildAll} }
    '13' = @{ Name = '运行后端测试 (go test ./...)';           Fn = ${function:Invoke-Test} }
    '14' = @{ Name = '代码检查 (go vet + gofmt)';              Fn = ${function:Invoke-Lint} }
    '15' = @{ Name = '数据库初始化 (建管理员)';                Fn = ${function:Invoke-DBInit} }
    '16' = @{ Name = '重置管理员密码';                         Fn = ${function:Invoke-ResetPassword} }
    '17' = @{ Name = '运行已构建的二进制 (server, 前台)';      Fn = ${function:Invoke-RunBinary} }
    '18' = @{ Name = '清理 (bin / logs / data)';               Fn = ${function:Invoke-Clean} }
}

# 别名（支持 .\dev.ps1 up 等）
$Aliases = @{
    'install' = '1'
    'up' = '2'; 'start' = '2'
    'down' = '3'; 'stop' = '3'
    'restart' = '4'; 're' = '4'
    'rebe' = '5'; 'refe' = '6'
    'status' = '7'; 'st' = '7'
    'logs' = '8'; 'watch' = '9'; 'tail' = '9'
    'buildfe' = '10'; 'buildbe' = '11'; 'build' = '12'
    'test' = '13'; 'lint' = '14'
    'dbinit' = '15'; 'resetpw' = '16'
    'run' = '17'; 'clean' = '18'
}

function Show-Menu {
    Write-Host "`n========== FlyMail 开发菜单 ==========" -ForegroundColor Cyan
    Write-Host (" 后端 127.0.0.1:$BackendPort   前端 :$FrontendPort   日志 logs/   CGO=0") -ForegroundColor DarkGray
    $beState = if (Get-DevProcess 'backend') { '运行中' } else { '停止' }
    $feState = if (Get-DevProcess 'frontend') { '运行中' } else { '停止' }
    Write-Host (" 当前：后端[{0}] 前端[{1}]" -f $beState, $feState) -ForegroundColor DarkGray
    foreach ($key in $Actions.Keys) {
        '{0,3}) {1}' -f $key, $Actions[$key].Name | Write-Host
    }
    Write-Host '  0) 退出菜单（后台服务保持运行）'
    Write-Host '=====================================' -ForegroundColor Cyan
}

function Invoke-Choice($choice) {
    if ($Aliases.ContainsKey($choice)) { $choice = $Aliases[$choice] }
    if ($Actions.Contains($choice)) {
        try { & $Actions[$choice].Fn }
        catch { Write-Err $_.Exception.Message }
    } else {
        Write-Warn "无效选项：$choice"
    }
}

# ---------- 入口 ----------
if ($Action) {
    if ($Action -eq '0' -or $Action -eq 'exit') { return }
    Invoke-Choice $Action
    return
}

while ($true) {
    Show-Menu
    $choice = Read-Host '请选择'
    if ($choice -eq '0' -or $choice -eq 'q' -or $choice -eq 'exit') { Write-Host '退出菜单（后台服务仍在运行，可再次运行本脚本管理）'; break }
    Invoke-Choice $choice
    Write-Host "`n按回车继续..." -ForegroundColor DarkGray
    [void](Read-Host)
}
