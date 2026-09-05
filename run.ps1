# run.ps1 - EcoPulse AI dev launcher for Windows.
$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$backendDir = Join-Path $root "backend-go"
$frontendDir = Join-Path $root "frontend"
$telemetryFile = Join-Path $root "data-generator\output\telemetry_data.json"
$logDir = Join-Path $root "logs"
$backendLog = Join-Path $logDir "backend.log"
$frontendLog = Join-Path $logDir "frontend.log"
$backendPort = 8080
$frontendPort = 5173
$backendUrl = "http://localhost:$backendPort"
$frontendUrl = "http://localhost:$frontendPort"

$script:spawned = @()
$script:shellExe = $null

function Test-PortOpen {
    param([int]$Port, [int]$TimeoutMs = 500)
    $client = New-Object System.Net.Sockets.TcpClient
    try {
        $async = $client.BeginConnect("127.0.0.1", $Port, $null, $null)
        if ($async.AsyncWaitHandle.WaitOne($TimeoutMs)) {
            $client.EndConnect($async)
            return $true
        }
    } catch {
        return $false
    } finally {
        $client.Close()
    }
    return $false
}

function Wait-ForPort {
    param([int]$Port, [int]$Seconds)
    $deadline = (Get-Date).AddSeconds($Seconds)
    while ((Get-Date) -lt $deadline) {
        if (Test-PortOpen $Port) { return $true }
        Start-Sleep -Milliseconds 750
    }
    return $false
}

function Get-LauncherShell {
    $pwsh = Get-Command pwsh -ErrorAction SilentlyContinue
    if ($pwsh) { return $pwsh.Source }
    Write-Host "WARNING: pwsh (PowerShell 7) is not installed; using Windows PowerShell 5.1 as the child shell instead." -ForegroundColor Yellow
    return (Get-Command powershell -ErrorAction Stop).Source
}

function Start-Server {
    param([string]$WorkingDir, [string]$Command, [string]$LogFile)
    $proc = Start-Process -FilePath $script:shellExe -WorkingDirectory $WorkingDir `
        -PassThru -NoNewWindow -RedirectStandardOutput $LogFile `
        -ArgumentList @("-NoProfile", "-NoLogo", "-Command", $Command)
    $script:spawned += $proc.Id
    return $proc
}

function Stop-SpawnedServers {
    foreach ($pidToKill in $script:spawned) {
        $stillRunning = Get-Process -Id $pidToKill -ErrorAction SilentlyContinue
        if (-not $stillRunning) {
            Write-Host "Process $pidToKill already stopped (for example, by Ctrl+C)." -ForegroundColor Gray
            continue
        }
        taskkill /PID $pidToKill /T /F 2>$null | Out-Null
        if ($LASTEXITCODE -eq 0) {
            Write-Host "Stopped process tree $pidToKill." -ForegroundColor Gray
        } else {
            Write-Host "WARNING: taskkill failed for process $pidToKill; kill it manually with: taskkill /PID $pidToKill /T /F" -ForegroundColor Yellow
        }
    }
}

Write-Host ""
Write-Host "EcoPulse AI dev launcher" -ForegroundColor Green
Write-Host "------------------------" -ForegroundColor Green

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Host "ERROR: Go is not installed or not on PATH. Install Go and retry." -ForegroundColor Red
    exit 1
}
if (-not (Get-Command pnpm -ErrorAction SilentlyContinue)) {
    Write-Host "ERROR: pnpm is not installed or not on PATH. Install it (or run corepack enable) and retry." -ForegroundColor Red
    exit 1
}
if (-not (Test-Path -LiteralPath $telemetryFile)) {
    Write-Host "ERROR: Telemetry dataset not found at:" -ForegroundColor Red
    Write-Host "  $telemetryFile" -ForegroundColor Red
    Write-Host "Generate it first with: cd data-generator; uv run python generator.py" -ForegroundColor Yellow
    exit 1
}

if (Test-PortOpen $backendPort) {
    Write-Host "ERROR: Port $backendPort is already in use. Close the existing backend first." -ForegroundColor Red
    exit 1
}
if (Test-PortOpen $frontendPort) {
    Write-Host "ERROR: Port $frontendPort is already in use. Close the existing frontend first." -ForegroundColor Red
    exit 1
}

try {
    New-Item -ItemType Directory -Force -Path $logDir | Out-Null
    $script:shellExe = Get-LauncherShell

    $backendCmd = "cmd /c `"go run cmd/server/main.go 2>&1`""
    $frontendCmd = "cmd /c `"pnpm dev 2>&1`""

    Write-Host "Starting Go backend on $backendUrl (in this terminal) ..." -ForegroundColor Cyan
    $backendShell = Start-Server $backendDir $backendCmd $backendLog

    Write-Host "Starting React frontend on $frontendUrl (in this terminal) ..." -ForegroundColor Cyan
    $frontendShell = Start-Server $frontendDir $frontendCmd $frontendLog

    Write-Host "Waiting for the Go backend to come online (up to 90s)..." -ForegroundColor Gray
    if (-not (Wait-ForPort $backendPort 90)) {
        throw "Go backend did not come online within 90 seconds. See $backendLog."
    }
    Write-Host "Go backend is up: $backendUrl" -ForegroundColor Green

    Write-Host "Waiting for the React frontend to come online (up to 60s)..." -ForegroundColor Gray
    if (-not (Wait-ForPort $frontendPort 60)) {
        throw "React frontend did not come online within 60 seconds. See $frontendLog."
    }
    Write-Host "React frontend is up: $frontendUrl" -ForegroundColor Green

    Start-Process $frontendUrl

    Write-Host ""
    Write-Host "Press Ctrl+C here to stop both servers." -ForegroundColor Yellow
    Write-Host "Both servers run invisibly in this terminal - no extra windows are opened." -ForegroundColor Gray
    Write-Host "Server output is redirected to:" -ForegroundColor Gray
    Write-Host "  $backendLog" -ForegroundColor Gray
    Write-Host "  $frontendLog" -ForegroundColor Gray
    Write-Host "Watch a live tail with: Get-Content -Wait $backendLog" -ForegroundColor Gray
    Write-Host "The launcher watches both servers and shuts everything down if either crashes." -ForegroundColor Gray

    while ($true) {
        Start-Sleep -Seconds 2
        if ($backendShell.HasExited) {
            throw "Go backend process exited unexpectedly. See $backendLog."
        }
        if (-not (Test-PortOpen $backendPort)) {
            throw "Go backend stopped responding on port $backendPort. See $backendLog."
        }
        if ($frontendShell.HasExited) {
            throw "React frontend process exited unexpectedly. See $frontendLog."
        }
        if (-not (Test-PortOpen $frontendPort)) {
            throw "React frontend stopped responding on port $frontendPort. See $frontendLog."
        }
    }
} finally {
    Write-Host "Stopping EcoPulse AI servers ..." -ForegroundColor Cyan
    Stop-SpawnedServers
    Write-Host "All servers stopped." -ForegroundColor Green
}