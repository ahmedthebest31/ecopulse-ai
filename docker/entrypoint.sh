#!/bin/sh
set -eu

cd /opt/ecopulse/backend-go
./ecopulse-server &
backend_pid=$!
echo "EcoPulse AI backend started (PID ${backend_pid})."

attempt=0
while [ "${attempt}" -lt 30 ]; do
    if wget -q -O /dev/null "http://127.0.0.1:8080/api/health"; then
        break
    fi
    attempt=$((attempt + 1))
    sleep 1
done

if [ "${attempt}" -eq 30 ]; then
    echo "ERROR: backend did not pass /api/health within 30 seconds." >&2
    kill "${backend_pid}" 2>/dev/null || true
    exit 1
fi
echo "Backend health check passed. Starting nginx."

on_term() {
    echo "Shutting down EcoPulse AI ..."
    kill -TERM "${backend_pid}" 2>/dev/null || true
    kill -TERM "${nginx_pid}" 2>/dev/null || true
    exit 0
}
trap on_term TERM INT

nginx -g "daemon off;" &
nginx_pid=$!
wait "${nginx_pid}"