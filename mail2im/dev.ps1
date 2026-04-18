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
$frontendProc = $null

# Kill leftover dev processes from previous runs
Get-Process -Name "node" -ErrorAction SilentlyContinue | Where-Object {
    $_.MainWindowTitle -eq "" -and $_.Path -and $_.Path -match "frontend-react"
} | Stop-Process -Force -ErrorAction SilentlyContinue

# Prepare log directory (clean up stale files from previous runs)
$logDir = Join-Path (Get-Location) ".dev-logs"
Remove-Item $logDir -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Path $logDir -Force | Out-Null

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
    # No tray: start backend with go run (output to log file for tailing)
    Write-Host "`n[Backend] Starting..." -ForegroundColor Yellow
    $backendJob = Start-Job -ScriptBlock {
        param($root, $ldflags, $port, $goproxy, $logFile)
        $env:PORT = $port
        $env:GOPROXY = $goproxy
        Set-Location "$root\backend"
        & go run -ldflags $ldflags cmd/server/main.go 2>&1 | Tee-Object -FilePath $logFile
    } -ArgumentList (Get-Location).Path, $ldflags, $Port, $env:GOPROXY, (Join-Path (Get-Location) ".dev-logs\backend.log")
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

# Show default test credentials
Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  Default Account (first run)" -ForegroundColor Cyan
Write-Host "  Username: admin" -ForegroundColor White
Write-Host "  Password: admin (or set via setup)" -ForegroundColor White
Write-Host "========================================" -ForegroundColor Cyan

# --- Start Frontend ---
# Ensure node_modules exists
if (-not (Test-Path "frontend-react\node_modules")) {
    Write-Host "`n[Frontend] Installing dependencies..." -ForegroundColor Yellow
    Push-Location frontend-react
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

# Clean up stale log files (kill leftover processes if locked)
$feLOG = Join-Path $logDir "frontend.log"
$beLOG = Join-Path $logDir "backend.log"
Remove-Item $feLOG -Force -ErrorAction SilentlyContinue
Remove-Item $beLOG -Force -ErrorAction SilentlyContinue
New-Item -ItemType File -Path $feLOG -Force | Out-Null
New-Item -ItemType File -Path $beLOG -Force | Out-Null

$feWorkDir = Join-Path (Get-Location) "frontend-react"
$feErrLog = Join-Path $logDir "frontend-err.log"
$frontendProc = Start-Process -FilePath "cmd.exe" `
    -ArgumentList "/c `"cd /d $feWorkDir && pnpm run dev --port $FrontendPort --host 0.0.0.0`"" `
    -RedirectStandardOutput $feLOG -RedirectStandardError $feErrLog `
    -PassThru -WindowStyle Hidden

Write-Host "[Frontend] http://localhost:$FrontendPort" -ForegroundColor Green
Write-Host "`nPress Ctrl+C to stop all services.`n" -ForegroundColor DarkGray

# --- Tail logs + handle exit ---
$feLastLine = 0
$beLastLine = 0
try {
    while ($true) {
        # Tail backend log (NoTray mode writes to $beLOG)
        if (Test-Path $beLOG) {
            $lines = Get-Content $beLOG -ErrorAction SilentlyContinue
            if ($lines -and $lines.Count -gt $beLastLine) {
                $lines[$beLastLine..($lines.Count - 1)] | ForEach-Object {
                    if ($_.Trim()) { Write-Host "[BE] $_" -ForegroundColor DarkCyan }
                }
                $beLastLine = $lines.Count
            }
        }
        # Tail frontend log
        if (Test-Path $feLOG) {
            $lines = Get-Content $feLOG -ErrorAction SilentlyContinue
            if ($lines -and $lines.Count -gt $feLastLine) {
                $lines[$feLastLine..($lines.Count - 1)] | ForEach-Object {
                    if ($_.Trim()) { Write-Host "[FE] $_" -ForegroundColor DarkMagenta }
                }
                $feLastLine = $lines.Count
            }
        }
        # Check if processes died
        $backendAlive = if ($trayProc) { -not $trayProc.HasExited } elseif ($backendJob) { $backendJob.State -eq "Running" } else { $false }
        $frontendAlive = -not $frontendProc.HasExited
        if (-not $backendAlive -and -not $frontendAlive) {
            Write-Host "`nAll processes exited." -ForegroundColor Yellow
            break
        }
        Start-Sleep -Milliseconds 500
    }
} finally {
    Write-Host "`nStopping services..." -ForegroundColor Yellow
    # Kill process trees (taskkill /T kills child processes too)
    if ($trayProc -and -not $trayProc.HasExited) {
        & taskkill /F /T /PID $trayProc.Id 2>$null | Out-Null
    }
    if ($backendJob) {
        Stop-Job $backendJob -ErrorAction SilentlyContinue
        Remove-Job $backendJob -Force -ErrorAction SilentlyContinue
    }
    if ($frontendProc -and -not $frontendProc.HasExited) {
        & taskkill /F /T /PID $frontendProc.Id 2>$null | Out-Null
    }
    # Wait briefly for processes to release file handles
    Start-Sleep -Milliseconds 500
    # Clean up
    Remove-Item ".\mail2im-tray.exe" -ErrorAction SilentlyContinue
    Remove-Item $logDir -Recurse -Force -ErrorAction SilentlyContinue
    Write-Host "Done." -ForegroundColor Green
}
