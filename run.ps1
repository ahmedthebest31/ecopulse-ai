# run.ps1 - Launch EcoPulse AI for local development and manual testing.
# Starts the Go backend and the React (Vite) frontend in separate windows,
# waits until both are actually online, then opens the dashboard in the
# default browser. Press Ctrl+C here to stop both servers.
$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$backendDir = Join-Path $root "backend-go"
$frontendDir = Join-Path $root "frontend"
$telemetryFile = Join-Path $root "data-generator\output\telemetry_data.json"
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
    param([int]$Port, [int]$Seconds, [string]$Label)
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
}

function Stop-SpawnedServers {
    foreach ($pidToKill in $script:spawned) {
        try {
            taskkill /PID $pidToKill /T /F 2>$null | Out-Null
        } catch {
            Write-Host "Could not stop process $pidToKill. Close its window manually." -ForegroundColor Yellow
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
    Write-Host "Starting Go backend on $backendUrl ..." -ForegroundColor Cyan
    Start-ServerWindow $backendDir "go run cmd/server/main.go"

    Write-Host "Starting React frontend on $frontendUrl ..." -ForegroundColor Cyan
    Start-ServerWindow $frontendDir "pnpm dev"

    Write-Host "Waiting for the Go backend to come online (up to 90s)..." -ForegroundColor Gray
    if (-not (Wait-ForPort $backendPort 90 "Go backend")) {
        throw "Go backend did not come online within 90 seconds. Check its window for errors."
    }
    Write-Host "Go backend is up: $backendUrl" -ForegroundColor Green

    Write-Host "Waiting for the React frontend to come online (up to 60s)..." -ForegroundColor Gray
    if (-not (Wait-ForPort $frontendPort 60 "React frontend")) {
        throw "React frontend did not come online within 60 seconds. Check its window for errors."
    }
    Write-Host "React frontend is up: $frontendUrl" -ForegroundColor Green

    Start-Process $frontendUrl

    Write-Host ""
    Write-Host "Press Ctrl+C here to stop both servers." -ForegroundColor Yellow
    Write-Host "You can also close the two server windows manually." -ForegroundColor Gray

    while ($true) {
        Start-Sleep -Seconds 3600
    }
} finally {
    Write-Host "Stopping EcoPulse AI servers ..." -ForegroundColor Cyan
    Stop-SpawnedServers
    Write-Host "All servers stopped." -ForegroundColor Green
}
