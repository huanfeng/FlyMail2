# Mail2IM Development Environment Launcher (Windows)
# Usage: .\dev.ps1 [-Port 8080] [-FrontendPort 8008] [-NoTray]

param(
    [int]$Port = 8080,
    [int]$FrontendPort = 8008,
    [switch]$NoTray
)

$ErrorActionPreference = "Stop"

# Environment
$env:PORT = $Port
$env:CORS_ORIGINS = "http://localhost:$FrontendPort"
$env:VITE_API_BASE_URL = "/api"
$env:API_HOST = "http://localhost:$Port"
$env:GOPROXY = "https://goproxy.cn,direct"

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  Mail2IM Development Environment"       -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "Backend  : http://localhost:$Port"
Write-Host "Frontend : http://localhost:$FrontendPort"
Write-Host "Tray     : $(if ($NoTray) { 'disabled' } else { 'enabled' })"
Write-Host "========================================" -ForegroundColor Cyan

$ldflags = ""
try {
    $version = git describe --tags --always --dirty 2>$null
    if (-not $version) { $version = "dev" }
    $commit = git rev-parse --short HEAD 2>$null
    if (-not $commit) { $commit = "unknown" }
    $date = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
    $ldflags = "-X 'main.AppVersion=$version' -X 'main.GitCommit=$commit' -X 'main.BuildTime=$date'"
} catch {
    $ldflags = "-X 'main.AppVersion=dev'"
}

$trayProc = $null
$backendJob = $null
$frontendJob = $null

# --- Build & start backend (with or without tray) ---
if (-not $NoTray) {
    # Build tray binary (includes the server)
    Write-Host "`n[Tray] Building..." -ForegroundColor Yellow
    Push-Location backend
    $buildOutput = & go build -ldflags $ldflags -o ../mail2im-tray.exe ./cmd/tray 2>&1
    Pop-Location
    if ($LASTEXITCODE -ne 0) {
        Write-Host "[Error] Tray build failed:" -ForegroundColor Red
        Write-Host $buildOutput
        exit 1
    }
    Write-Host "[Tray] Build OK" -ForegroundColor Green

    # Start tray process (auto-starts server)
    Write-Host "[Tray] Starting (server on :$Port)..." -ForegroundColor Yellow
    $env:MAIL2IM_FRONTEND_PORT = $FrontendPort
    $trayProc = Start-Process -FilePath ".\mail2im-tray.exe" -PassThru -WindowStyle Hidden
} else {
    # No tray: start backend with go run
    Write-Host "`n[Backend] Starting..." -ForegroundColor Yellow
    $backendJob = Start-Job -ScriptBlock {
        param($root, $ldflags, $port, $goproxy)
        $env:PORT = $port
        $env:GOPROXY = $goproxy
        Set-Location "$root\backend"
        & go run -ldflags $ldflags cmd/server/main.go 2>&1
    } -ArgumentList (Get-Location).Path, $ldflags, $Port, $env:GOPROXY
}

# Wait for backend to be ready
Write-Host "[Backend] Waiting for port $Port..." -ForegroundColor Yellow
$maxRetries = 30
$ready = $false
for ($i = 0; $i -lt $maxRetries; $i++) {
    Start-Sleep -Seconds 1
    try {
        $tcp = New-Object System.Net.Sockets.TcpClient
        $tcp.Connect("127.0.0.1", $Port)
        $tcp.Close()
        $ready = $true
        break
    } catch {
        # Check if process/job died
        if ($trayProc -and $trayProc.HasExited) {
            Write-Host "[Error] Tray process exited unexpectedly (code $($trayProc.ExitCode))" -ForegroundColor Red
            exit 1
        }
        if ($backendJob -and ($backendJob.State -eq "Failed" -or $backendJob.State -eq "Completed")) {
            Write-Host "[Error] Backend exited unexpectedly:" -ForegroundColor Red
            Receive-Job $backendJob
            Remove-Job $backendJob -Force
            exit 1
        }
    }
}

if (-not $ready) {
    Write-Host "[Error] Backend failed to start within $maxRetries seconds." -ForegroundColor Red
    if ($trayProc) { Stop-Process -Id $trayProc.Id -Force -ErrorAction SilentlyContinue }
    if ($backendJob) { Stop-Job $backendJob; Remove-Job $backendJob -Force }
    exit 1
}
Write-Host "[Backend] Ready!" -ForegroundColor Green

# --- Start Frontend ---
# Ensure node_modules exists
if (-not (Test-Path "frontend\node_modules")) {
    Write-Host "`n[Frontend] Installing dependencies..." -ForegroundColor Yellow
    Push-Location frontend
    & pnpm install
    if ($LASTEXITCODE -ne 0) {
        Write-Host "[Error] pnpm install failed" -ForegroundColor Red
        Pop-Location
        exit 1
    }
    Pop-Location
    Write-Host "[Frontend] Dependencies installed" -ForegroundColor Green
}

Write-Host "`n[Frontend] Starting (pnpm dev)..." -ForegroundColor Yellow
$frontendJob = Start-Job -ScriptBlock {
    param($root, $frontendPort, $apiHost)
    $env:API_HOST = $apiHost
    Set-Location "$root\frontend"
    & pnpm run dev -- --port $frontendPort --host 0.0.0.0 2>&1
} -ArgumentList (Get-Location).Path, $FrontendPort, $env:API_HOST

Write-Host "[Frontend] http://localhost:$FrontendPort" -ForegroundColor Green
Write-Host "`nPress Ctrl+C to stop all services.`n" -ForegroundColor DarkGray

# --- Stream output + handle exit ---
try {
    while ($true) {
        # Print backend output (only in NoTray mode)
        if ($backendJob -and $backendJob.HasMoreData) {
            Receive-Job $backendJob | ForEach-Object {
                Write-Host "[BE] $_" -ForegroundColor DarkCyan
            }
        }
        # Print frontend output
        if ($frontendJob.HasMoreData) {
            Receive-Job $frontendJob | ForEach-Object {
                Write-Host "[FE] $_" -ForegroundColor DarkMagenta
            }
        }
        # Check if processes died
        $backendAlive = if ($trayProc) { -not $trayProc.HasExited } elseif ($backendJob) { $backendJob.State -eq "Running" } else { $false }
        $frontendAlive = $frontendJob.State -eq "Running"
        if (-not $backendAlive -and -not $frontendAlive) {
            Write-Host "`nAll processes exited." -ForegroundColor Yellow
            break
        }
        Start-Sleep -Milliseconds 500
    }
} finally {
    Write-Host "`nStopping services..." -ForegroundColor Yellow
    if ($trayProc -and -not $trayProc.HasExited) {
        Stop-Process -Id $trayProc.Id -Force -ErrorAction SilentlyContinue
    }
    if ($backendJob) {
        Stop-Job $backendJob -ErrorAction SilentlyContinue
        Remove-Job $backendJob -Force -ErrorAction SilentlyContinue
    }
    Stop-Job $frontendJob -ErrorAction SilentlyContinue
    Remove-Job $frontendJob -Force -ErrorAction SilentlyContinue
    # Clean up tray binary
    Remove-Item ".\mail2im-tray.exe" -ErrorAction SilentlyContinue
    Write-Host "Done." -ForegroundColor Green
}
