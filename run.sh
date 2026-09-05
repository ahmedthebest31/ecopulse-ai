#!/usr/bin/env bash
# EcoPulse AI dev launcher for Linux/macOS.
set -Eeuo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
backend_dir="$root/backend-go"
frontend_dir="$root/frontend"
telemetry_file="$root/data-generator/output/telemetry_data.json"
log_dir="$root/logs"
backend_log="$log_dir/backend.log"
frontend_log="$log_dir/frontend.log"
backend_port=8080
frontend_port=5173
backend_url="http://localhost:$backend_port"
frontend_url="http://localhost:$frontend_port"

pgids=()
names=()

die() {
  echo "ERROR: $*" >&2
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "Required command not found: $1. Install it and retry."
}

port_open() {
  local port="$1"
  if command -v nc >/dev/null 2>&1; then
    nc -z -w1 127.0.0.1 "$port" >/dev/null 2>&1
  else
    timeout 1 bash -c "exec 3<>/dev/tcp/127.0.0.1/$port" 2>/dev/null
  fi
}

wait_for_port() {
  local port="$1" seconds="$2" deadline now
  deadline=$(($(date +%s) + seconds))
  while :; do
    now=$(date +%s)
    [ "$now" -ge "$deadline" ] && return 1
    port_open "$port" && return 0
    sleep 1
  done
}

start_server() {
  local name="$1" dir="$2" log_file="$3"
  shift 3
  local pgid
  pgid="$(cd "$dir" && { setsid "$@" >"$log_file" 2>&1 & echo $!; })"
  pgids+=("$pgid")
  names+=("$name")
  printf '%s' "$pgid"
}

stop_servers() {
  local i pgid
  for ((i = 0; i < ${#pgids[@]}; i++)); do
    pgid="${pgids[$i]}"
    if kill -0 "-$pgid" 2>/dev/null; then
      kill "-$pgid" 2>/dev/null || true
      sleep 1
      if kill -0 "-$pgid" 2>/dev/null; then
        kill -9 "-$pgid" 2>/dev/null || true
      else
        echo "Stopped process group ${pgid} (${names[$i]})."
      fi
    else
      echo "Process group ${pgid} (${names[$i]}) already stopped."
    fi
  done
}

trap stop_servers EXIT

echo ""
echo "EcoPulse AI dev launcher"
echo "------------------------"

require_cmd go
require_cmd pnpm
command -v setsid >/dev/null 2>&1 || die "Required command not found: setsid (util-linux). Install it and retry."
[ -f "$telemetry_file" ] || die "Telemetry dataset not found at: $telemetry_file (generate it with: cd data-generator && uv run python generator.py)"

port_open "$backend_port" && die "Port $backend_port is already in use. Close the existing backend first."
port_open "$frontend_port" && die "Port $frontend_port is already in use. Close the existing frontend first."

mkdir -p "$log_dir"

start_server backend "$backend_dir" "$backend_log" go run cmd/server/main.go >/dev/null
backend_pgid="${pgids[${#pgids[@]} - 1]}"

start_server frontend "$frontend_dir" "$frontend_log" pnpm dev
frontend_pgid="${pgids[${#pgids[@]} - 1]}"

echo "Starting Go backend on $backend_url (in this terminal) ..."
echo "Starting React frontend on $frontend_url (in this terminal) ..."

echo "Waiting for the Go backend to come online (up to 90s)..."
wait_for_port "$backend_port" 90 || die "Go backend did not come online within 90 seconds. See $backend_log."
echo "Go backend is up: $backend_url"

echo "Waiting for the React frontend to come online (up to 60s)..."
wait_for_port "$frontend_port" 60 || die "React frontend did not come online within 60 seconds. See $frontend_log."
echo "React frontend is up: $frontend_url"

if command -v xdg-open >/dev/null 2>&1 && [ -n "${DISPLAY:-}" ]; then
  xdg-open "$frontend_url" >/dev/null 2>&1 || true
elif command -v wslview >/dev/null 2>&1; then
  wslview "$frontend_url" >/dev/null 2>&1 || true
else
  echo "Dashboard: $frontend_url"
fi

echo ""
echo "Press Ctrl+C here to stop both servers."
echo "Both servers run invisibly in this terminal - no extra windows are opened."
echo "Server output is redirected to:"
echo "  $backend_log"
echo "  $frontend_log"
echo "Watch a live tail with: tail -f $backend_log"
echo "The launcher watches both servers and shuts everything down if either crashes."

while :; do
  sleep 2
  if ! kill -0 "-$backend_pgid" 2>/dev/null || ! port_open "$backend_port"; then
    die "Go backend went down. See $backend_log."
  fi
  if ! kill -0 "-$frontend_pgid" 2>/dev/null || ! port_open "$frontend_port"; then
    die "React frontend went down. See $frontend_log."
  fi
done