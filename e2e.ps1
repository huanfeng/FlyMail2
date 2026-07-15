# FlyMail 后端 E2E（Windows 本机，GreenMail standalone JAR，无需 Docker）
# 用法:
#   ./e2e.ps1                # 全量跑
#   ./e2e.ps1 -Run TestProbe # 只跑匹配的测试
#   ./e2e.ps1 -KeepUp        # 跑完保留 GreenMail 进程（反复迭代时更快）
# 前置: Java 11+（GreenMail 2.x 要求）、Go。
param(
    [string]$Run = "",
    [switch]$KeepUp
)

$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

$GreenMailVersion = "2.1.8"
$JarDir = Join-Path $PSScriptRoot ".dev/greenmail"
$Jar = Join-Path $JarDir "greenmail-standalone-$GreenMailVersion.jar"
$JarUrl = "https://repo1.maven.org/maven2/com/icegreen/greenmail-standalone/$GreenMailVersion/greenmail-standalone-$GreenMailVersion.jar"

# 端口: 邮件端口用 GreenMail 默认; API 用 18080(本机 8080 常被 IDE 占用)
$SmtpPort = 3025
$ImapPort = 3143
$ApiPort = 18080
$ReadinessUrl = "http://127.0.0.1:$ApiPort/api/service/readiness"

function Test-Ready {
    try {
        $r = Invoke-WebRequest -Uri $ReadinessUrl -UseBasicParsing -TimeoutSec 2
        return $r.StatusCode -eq 200
    } catch { return $false }
}

# 1. JAR 缓存
if (-not (Test-Path $Jar)) {
    New-Item -ItemType Directory -Force $JarDir | Out-Null
    Write-Host "[e2e] downloading GreenMail $GreenMailVersion ..."
    Invoke-WebRequest -Uri $JarUrl -OutFile $Jar
}

# 2. 起 GreenMail（若已就绪则复用，配合 -KeepUp 快速迭代）
$greenmail = $null
if (Test-Ready) {
    Write-Host "[e2e] reusing running GreenMail (API :$ApiPort)"
} else {
    Write-Host "[e2e] starting GreenMail (SMTP :$SmtpPort IMAP :$ImapPort API :$ApiPort) ..."
    $greenmail = Start-Process -FilePath "java" -PassThru -WindowStyle Hidden `
        -RedirectStandardOutput (Join-Path $JarDir "greenmail.log") `
        -RedirectStandardError (Join-Path $JarDir "greenmail.err.log") `
        -ArgumentList @(
            "-Dgreenmail.setup.test.all",
            "-Dgreenmail.hostname=127.0.0.1",
            "-Dgreenmail.auth.disabled",
            "-Dgreenmail.api.hostname=127.0.0.1",
            "-Dgreenmail.api.port=$ApiPort",
            "-jar", $Jar
        )
    $deadline = (Get-Date).AddSeconds(30)
    while (-not (Test-Ready)) {
        if ((Get-Date) -gt $deadline) {
            if ($greenmail) { Stop-Process -Id $greenmail.Id -Force -Confirm:$false }
            Write-Error "[e2e] GreenMail not ready in 30s, see $JarDir/greenmail*.log"
        }
        Start-Sleep -Milliseconds 500
    }
    Write-Host "[e2e] GreenMail ready"
}

# 3. 跑测试
$env:E2E_GREENMAIL = "1"
$env:GREENMAIL_HOST = "127.0.0.1"
$env:GREENMAIL_SMTP_PORT = "$SmtpPort"
$env:GREENMAIL_IMAP_PORT = "$ImapPort"
$env:GREENMAIL_API_PORT = "$ApiPort"

$testArgs = @("test", "./internal/e2e/...", "-p", "1", "-count=1", "-timeout", "300s", "-v")
if ($Run -ne "") { $testArgs += @("-run", $Run) }

Push-Location (Join-Path $PSScriptRoot "flymail/backend")
& go @testArgs
$rc = $LASTEXITCODE
Pop-Location

# 4. 收尾
if ($greenmail -and -not $KeepUp) {
    Write-Host "[e2e] stopping GreenMail"
    Stop-Process -Id $greenmail.Id -Force -Confirm:$false
} elseif ($KeepUp) {
    Write-Host "[e2e] KeepUp: GreenMail left running (API :$ApiPort)"
}
exit $rc
