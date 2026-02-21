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

Mounts:

- `/var/run/docker.sock` (read-only)
- `./data` to `/data` for SQLite

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
