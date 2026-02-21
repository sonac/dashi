# Dashi

Dashi is a self-hosted home server observability dashboard written in Go with htmx.

## Features

- Host metrics: CPU, memory, network traffic, disk usage, load, uptime
- Docker metrics per container
- Docker log ingestion and service grouping
- Alert rules with cooldown/hysteresis
- Telegram notifications
- htmx dashboard fragments + JSON APIs
- SQLite persistence and retention cleanup

## Run locally

```bash
go mod tidy
go run ./cmd/server
```

Then open `http://localhost:8080`.

## Docker

```bash
docker compose up -d --build
```

To ensure Docker API/log access works from inside the Dashi container, export host socket settings before starting:

```bash
export DOCKER_SOCKET=/var/run/docker.sock
export DOCKER_GID=$(stat -c '%g' "$DOCKER_SOCKET")
docker compose up -d --build
```

On macOS, use:

```bash
export DOCKER_GID=$(stat -f '%g' "$DOCKER_SOCKET")
```

Mounts:

- `${DOCKER_SOCKET:-/var/run/docker.sock}` to same path in container
- `./data` to `/data` for SQLite

After startup, verify Docker connectivity via `GET /readyz` (returns `ready` only if DB and Docker are reachable).

## Environment variables

- `APP_ADDR` (default `:8080`)
- `APP_DATA_DIR` (default `./data`)
- `APP_DB_PATH` (default `$APP_DATA_DIR/app.db`)
- `APP_RETENTION_DAYS` (default `14`)
- `DOCKER_SOCKET` (default `/var/run/docker.sock`)
- `TELEGRAM_BOT_TOKEN`
- `TELEGRAM_CHAT_ID`

## Health

- `GET /healthz`
- `GET /readyz`
