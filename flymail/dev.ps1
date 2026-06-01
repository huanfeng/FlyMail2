#requires -Version 7.0
<#
.SYNOPSIS
    FlyMail Windows 开发脚本（菜单式）。
.DESCRIPTION
    一站式进行 flymail 的前端/后端编译、开发热重载、测试、数据库管理与打包部署。
    用法：
      .\dev.ps1            # 进入交互菜单
      .\dev.ps1 7          # 直接执行菜单项 7（完整构建）
      .\dev.ps1 build      # 也可用别名直达（见 $Actions）
.NOTES
    后端：flymail/backend（module flymail，无 CGO，glebarez 纯 Go SQLite）
    前端：flymail/frontend（React + Vite + pnpm，dev 代理 /api -> 后端）
#>

param(
    [Parameter(Position = 0)]
    [string]$Action = ''
)

$ErrorActionPreference = 'Stop'

# ---------- 路径与配置 ----------
$Root        = $PSScriptRoot
$Backend     = Join-Path $Root 'backend'
$Frontend    = Join-Path $Root 'frontend'
$DataDir     = Join-Path $Backend 'data'
$DistDir     = Join-Path $Backend 'web\dist'
$BinDir      = Join-Path $Root 'bin'
$BinPath     = Join-Path $BinDir 'flymail.exe'

$BackendPort  = if ($env:FLYMAIL_SERVER_PORT) { $env:FLYMAIL_SERVER_PORT } else { '8080' }
# FlyMail 专属前端开发端口（避开 Vite 默认 5173，减少与其它项目冲突）。
# 与 frontend/vite.config.ts 的 server.port 保持一致；可用 FLYMAIL_WEB_PORT 覆盖。
$FrontendPort = if ($env:FLYMAIL_WEB_PORT) { $env:FLYMAIL_WEB_PORT } else { '5390' }
$env:FLYMAIL_WEB_PORT = $FrontendPort   # 传给 vite，确保 strictPort 生效

# 统一 Go 环境：无 CGO（glebarez 纯 Go），国内代理兜底
$env:CGO_ENABLED = '0'
if (-not $env:GOPROXY) { $env:GOPROXY = 'https://goproxy.cn,direct' }

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

# 在新 PowerShell 窗口中启动长驻进程，保持菜单可用
function Start-InNewWindow($title, $workDir, $command) {
    $pwshCmd = Get-Command pwsh -ErrorAction SilentlyContinue
    $pwshExe = if ($pwshCmd) { $pwshCmd.Source } else { 'powershell' }
    $full = "`$host.UI.RawUI.WindowTitle = '$title'; Set-Location '$workDir'; `$env:CGO_ENABLED='0'; `$env:GOPROXY='$($env:GOPROXY)'; `$env:FLYMAIL_WEB_PORT='$FrontendPort'; $command"
    Start-Process $pwshExe -ArgumentList '-NoExit', '-Command', $full | Out-Null
    Write-Ok "已在新窗口启动：$title"
}

# ---------- 各操作 ----------
function Invoke-FrontendInstall {
    Write-Title '安装前端依赖 (pnpm install)'
    if (-not (Assert-Tool 'pnpm' '请先安装 pnpm：npm i -g pnpm')) { return }
    Push-Location $Frontend
    try { pnpm install } finally { Pop-Location }
    Write-Ok '前端依赖安装完成'
}

function Invoke-FrontendDev {
    Write-Title "前端开发 (Vite 热重载, http://localhost:$FrontendPort, 代理 /api -> :$BackendPort)"
    if (-not (Assert-Tool 'pnpm' '请先安装 pnpm')) { return }
    if (-not (Test-Path (Join-Path $Frontend 'node_modules'))) {
        Write-Warn '未检测到 node_modules，建议先执行菜单项 1 安装依赖'
    }
    Start-InNewWindow 'FlyMail 前端(vite)' $Frontend 'pnpm dev'
}

function Invoke-BackendDev {
    Write-Title "后端开发运行 (go run server, http://localhost:$BackendPort)"
    if (-not (Assert-Tool 'go' '请先安装 Go')) { return }
    Ensure-AdminHint
    Start-InNewWindow 'FlyMail 后端(server)' $Backend "go run . server --data-dir `"$DataDir`""
}

function Invoke-FullDev {
    Write-Title '全栈开发（后端 + 前端 同时，各占一个新窗口）'
    Invoke-BackendDev
    Start-Sleep -Milliseconds 500
    Invoke-FrontendDev
    Write-Host "后端: http://localhost:$BackendPort   前端: http://localhost:$FrontendPort" -ForegroundColor Magenta
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
        Write-Warn 'backend/web/dist 缺少前端产物，embed 可能为空。建议先执行菜单项 5 或 7'
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
    $pass = Read-Host '管理员密码'
    if ([string]::IsNullOrWhiteSpace($pass)) { Write-Err '密码不能为空'; return }
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
    if (-not (Test-Path $BinPath)) { Write-Err "未找到 $BinPath，请先执行菜单项 6 或 7 构建"; return }
    & $BinPath server --data-dir $DataDir
}

function Invoke-Clean {
    Write-Title '清理'
    if (Test-Path $BinDir) { Remove-Item -Recurse -Force $BinDir; Write-Ok '已删除 bin/' }
    $ans = Read-Host "是否同时清空数据目录 data/（数据库与附件，不可恢复）？(y/N)"
    if ($ans -eq 'y' -or $ans -eq 'Y') {
        if (Test-Path $DataDir) { Remove-Item -Recurse -Force $DataDir; Write-Ok '已清空 data/' }
    } else { Write-Host '保留 data/' }
    Write-Warn '注意：未删除 backend/web/dist（go embed 需要它存在；如需刷新请执行菜单项 5 重新构建前端）'
}

function Ensure-AdminHint {
    if (-not (Test-Path (Join-Path $DataDir 'flymail.db'))) {
        Write-Warn '尚未初始化数据库/管理员，登录前请先执行菜单项 10（数据库初始化）'
    }
}

# ---------- 菜单分发 ----------
$Actions = [ordered]@{
    '1'  = @{ Name = '安装前端依赖 (pnpm install)';            Fn = ${function:Invoke-FrontendInstall} }
    '2'  = @{ Name = '前端开发 (Vite 热重载)            [新窗口]'; Fn = ${function:Invoke-FrontendDev} }
    '3'  = @{ Name = '后端开发 (go run server :8080)     [新窗口]'; Fn = ${function:Invoke-BackendDev} }
    '4'  = @{ Name = '全栈开发 (后端 + 前端)            [新窗口x2]'; Fn = ${function:Invoke-FullDev} }
    '5'  = @{ Name = '构建前端 (-> backend/web/dist)';         Fn = ${function:Invoke-BuildFrontend} }
    '6'  = @{ Name = '构建后端二进制 (无 CGO, 含 embed)';      Fn = ${function:Invoke-BuildBackend} }
    '7'  = @{ Name = '完整构建 (前端->后端, 单二进制)';        Fn = ${function:Invoke-BuildAll} }
    '8'  = @{ Name = '运行后端测试 (go test ./...)';           Fn = ${function:Invoke-Test} }
    '9'  = @{ Name = '代码检查 (go vet + gofmt)';              Fn = ${function:Invoke-Lint} }
    '10' = @{ Name = '数据库初始化 (建管理员)';                Fn = ${function:Invoke-DBInit} }
    '11' = @{ Name = '重置管理员密码';                         Fn = ${function:Invoke-ResetPassword} }
    '12' = @{ Name = '运行已构建的二进制 (server)';            Fn = ${function:Invoke-RunBinary} }
    '13' = @{ Name = '清理 (bin / data)';                      Fn = ${function:Invoke-Clean} }
}

# 别名（支持 .\dev.ps1 build 等）
$Aliases = @{
    'install' = '1'; 'fe' = '2'; 'be' = '3'; 'dev' = '4';
    'buildfe' = '5'; 'buildbe' = '6'; 'build' = '7';
    'test' = '8'; 'lint' = '9'; 'dbinit' = '10'; 'resetpw' = '11';
    'run' = '12'; 'clean' = '13'
}

function Show-Menu {
    Write-Host "`n========== FlyMail 开发菜单 ==========" -ForegroundColor Cyan
    Write-Host (" 后端 :$BackendPort   前端 :$FrontendPort   CGO_ENABLED=0") -ForegroundColor DarkGray
    foreach ($key in $Actions.Keys) {
        '{0,3}) {1}' -f $key, $Actions[$key].Name | Write-Host
    }
    Write-Host '  0) 退出'
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
    # 直达模式
    if ($Action -eq '0' -or $Action -eq 'exit') { return }
    Invoke-Choice $Action
    return
}

# 交互菜单模式
while ($true) {
    Show-Menu
    $choice = Read-Host '请选择'
    if ($choice -eq '0' -or $choice -eq 'q' -or $choice -eq 'exit') { Write-Host '再见'; break }
    Invoke-Choice $choice
    Write-Host "`n按回车继续..." -ForegroundColor DarkGray
    [void](Read-Host)
}
