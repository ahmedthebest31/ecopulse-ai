# Stage 1: Go API server (stdlib only, static binary)
FROM golang:1.26-alpine AS backend-build
WORKDIR /src/backend-go
COPY backend-go/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/ecopulse-server ./cmd/server

# Stage 2: regenerate the deterministic seed-42 telemetry dataset
FROM python:3.12-alpine AS telemetry
WORKDIR /src/data-generator
COPY data-generator/ ./
RUN python generator.py --seed 42

# Stage 3: React/Vite dashboard
FROM node:22-alpine AS frontend-build
WORKDIR /app
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN corepack enable && corepack prepare pnpm@11.25.0 --activate && pnpm install --frozen-lockfile
COPY frontend/ ./
ARG VITE_API_BASE_URL=/api
ENV VITE_API_BASE_URL=${VITE_API_BASE_URL}
RUN pnpm build

# Runtime: nginx serves the dashboard and proxies /api/ to the Go backend
FROM nginx:1.27-alpine
RUN apk add --no-cache ca-certificates && rm -f /etc/nginx/conf.d/default.conf
COPY docker/nginx.conf /etc/nginx/conf.d/default.conf
COPY docker/entrypoint.sh /usr/local/bin/ecopulse-entrypoint
RUN chmod +x /usr/local/bin/ecopulse-entrypoint

WORKDIR /opt/ecopulse/backend-go
COPY --from=backend-build /out/ecopulse-server ./
COPY --from=telemetry /src/data-generator/output/telemetry_data.json /opt/ecopulse/data-generator/output/telemetry_data.json
COPY --from=frontend-build /app/dist /usr/share/nginx/html

ENV HOST=127.0.0.1 PORT=8080 TELEMETRY_DATA_PATH=../data-generator/output/telemetry_data.json

EXPOSE 80
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD wget -q -O /dev/null http://127.0.0.1/api/health || exit 1
ENTRYPOINT ["/usr/local/bin/ecopulse-entrypoint"]