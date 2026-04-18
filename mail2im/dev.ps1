# Mail2IM Development Environment Launcher (Windows)
# Usage: .\dev.ps1 [-Port 8080] [-FrontendPort 8008]

param(
    [int]$Port = 8080,
    [int]$FrontendPort = 8008
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
Write-Host "API Proxy: /api -> http://localhost:$Port"
Write-Host "========================================" -ForegroundColor Cyan

# --- Build flags ---
$version = git describe --tags --always --dirty 2>$null
if (-not $version) { $version = "dev" }
$commit = git rev-parse --short HEAD 2>$null
if (-not $commit) { $commit = "unknown" }
$date = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
$ldflags = "-X 'main.AppVersion=$version' -X 'main.GitCommit=$commit' -X 'main.BuildTime=$date'"

# --- Start Backend ---
Write-Host "`n[Backend] Starting..." -ForegroundColor Yellow
$backendJob = Start-Job -ScriptBlock {
    param($root, $ldflags, $port, $goproxy)
    $env:PORT = $port
    $env:GOPROXY = $goproxy
    Set-Location "$root\backend"
    & go run -ldflags $ldflags cmd/server/main.go 2>&1
} -ArgumentList (Get-Location).Path, $ldflags, $Port, $env:GOPROXY

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
        # Check if job died
        if ($backendJob.State -eq "Failed" -or $backendJob.State -eq "Completed") {
            Write-Host "[Error] Backend process exited unexpectedly:" -ForegroundColor Red
            Receive-Job $backendJob
            Remove-Job $backendJob -Force
            exit 1
        }
    }
}

if (-not $ready) {
    Write-Host "[Error] Backend failed to start within $maxRetries seconds." -ForegroundColor Red
    Stop-Job $backendJob; Remove-Job $backendJob -Force
    exit 1
}
Write-Host "[Backend] Ready!" -ForegroundColor Green

# --- Start Frontend ---
Write-Host "`n[Frontend] Starting (pnpm dev)..." -ForegroundColor Yellow
$frontendJob = Start-Job -ScriptBlock {
    param($root, $frontendPort, $apiHost)
    $env:API_HOST = $apiHost
    Set-Location "$root\frontend"
    & pnpm dev --port $frontendPort --host 0.0.0.0 2>&1
} -ArgumentList (Get-Location).Path, $FrontendPort, $env:API_HOST

Write-Host "[Frontend] http://localhost:$FrontendPort" -ForegroundColor Green
Write-Host "`nPress Ctrl+C to stop all services.`n" -ForegroundColor DarkGray

# --- Stream output + handle exit ---
try {
    while ($true) {
        # Print backend output
        if ($backendJob.HasMoreData) {
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
        # Check if jobs died
        if ($backendJob.State -ne "Running" -and $frontendJob.State -ne "Running") {
            Write-Host "`nBoth processes exited." -ForegroundColor Yellow
            break
        }
        Start-Sleep -Milliseconds 500
    }
} finally {
    Write-Host "`nStopping services..." -ForegroundColor Yellow
    Stop-Job $backendJob -ErrorAction SilentlyContinue
    Stop-Job $frontendJob -ErrorAction SilentlyContinue
    Remove-Job $backendJob -Force -ErrorAction SilentlyContinue
    Remove-Job $frontendJob -Force -ErrorAction SilentlyContinue
    Write-Host "Done." -ForegroundColor Green
}
