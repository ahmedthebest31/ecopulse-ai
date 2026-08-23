# run.ps1 - Launch EcoPulse AI for local development and manual testing.
# Starts the Go backend and the React (Vite) frontend in separate windows,
# waits until both are actually online, then opens the dashboard in the
# default browser. Both servers mirror their output into logs\*.log.
# A watchdog polls both children every 2 seconds: if either server stops
# responding on its port or its window exits, the launcher reports which one
# failed, tears down everything it spawned, and exits nonzero.
# Press Ctrl+C here to stop both servers.
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

function Start-ServerWindow {
    param([string]$WorkingDir, [string]$Command)
    $proc = Start-Process powershell -WorkingDirectory $WorkingDir -PassThru `
        -ArgumentList @("-NoExit", "-Command", $Command)
    $script:spawned += $proc.Id
    return $proc
}

function Stop-SpawnedServers {
    foreach ($pidToKill in $script:spawned) {
        taskkill /PID $pidToKill /T /F 2>$null | Out-Null
        if ($LASTEXITCODE -eq 0) {
            Write-Host "Stopped process tree $pidToKill." -ForegroundColor Gray
        } else {
            Write-Host "WARNING: taskkill exited with code $LASTEXITCODE for process $pidToKill; it may have already exited. Close any leftover window manually." -ForegroundColor Yellow
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

    # Tee each child's combined output into logs\*.log while keeping the live
    # window visible. The merge happens inside cmd.exe so PowerShell only ever
    # sees plain text lines (no ErrorRecord framing noise in the logs).
    $backendCmd = "cmd /c `"go run cmd/server/main.go 2>&1`" | Tee-Object -FilePath '$backendLog'"
    $frontendCmd = "cmd /c `"pnpm dev 2>&1`" | Tee-Object -FilePath '$frontendLog'"

    Write-Host "Starting Go backend on $backendUrl ..." -ForegroundColor Cyan
    $backendShell = Start-ServerWindow $backendDir $backendCmd

    Write-Host "Starting React frontend on $frontendUrl ..." -ForegroundColor Cyan
    $frontendShell = Start-ServerWindow $frontendDir $frontendCmd

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
    Write-Host "You can also close the two server windows manually." -ForegroundColor Gray
    Write-Host "Server output is mirrored to:" -ForegroundColor Gray
    Write-Host "  $backendLog" -ForegroundColor Gray
    Write-Host "  $frontendLog" -ForegroundColor Gray
    Write-Host "The launcher watches both servers and shuts everything down if either crashes." -ForegroundColor Gray

    # Watchdog: detect a crashed/exited child within one poll interval instead
    # of sleeping blindly. Any failure throws so the finally block below stops
    # BOTH servers and the script exits nonzero.
    while ($true) {
        Start-Sleep -Seconds 2
        if ($backendShell.HasExited) {
            throw "Go backend window exited unexpectedly. See $backendLog."
        }
        if (-not (Test-PortOpen $backendPort)) {
            throw "Go backend stopped responding on port $backendPort. See $backendLog."
        }
        if ($frontendShell.HasExited) {
            throw "React frontend window exited unexpectedly. See $frontendLog."
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
