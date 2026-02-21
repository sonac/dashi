# AGENTS.md

Guidance for coding agents operating in this repository.

## Project Snapshot
- Language/runtime: Go (`go 1.23`)
- Module: `dashi`
- Entrypoint: `cmd/server/main.go`
- Main app wiring: `internal/app/app.go`
- Storage: SQLite (`github.com/mattn/go-sqlite3`, CGO-dependent)
- Frontend: server-rendered templates + htmx fragments + small JS/CSS

## Repository Map
- `cmd/server`: startup, config load, logger init, shutdown signals
- `internal/app`: dependency graph and lifecycle
- `internal/web`: HTTP routes, handlers, templates, middleware
- `internal/db`: DB open/migrations/repository SQL
- `internal/collector`: host + container metrics collection
- `internal/logs`: Docker stream parsing and ingest workers
- `internal/alerts`: rule evaluation/state/notification flow
- `internal/notifier`: Telegram API client
- `internal/retention`: retention cleanup job
- `internal/models`: shared domain structs
- `web/templates`, `web/static`: UI templates/assets

## Build, Lint, Test Commands

Run all commands from repo root.

### Setup
- `go mod tidy`
- `mkdir -p data .gocache .gomodcache`
- `GOCACHE=$(pwd)/.gocache GOMODCACHE=$(pwd)/.gomodcache go mod tidy`

Taskfile shortcuts:
- `task setup-dev`
- `task dev`

### Run Locally
- `go run ./cmd/server`
- `GOCACHE=$(pwd)/.gocache GOMODCACHE=$(pwd)/.gomodcache go run ./cmd/server`

### Build
- `go build ./cmd/server`
- `CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o ./bin/dashi ./cmd/server`

### Test
- Full suite: `go test ./...`
- Verbose full suite: `go test -v ./...`
- No cache: `go test -count=1 ./...`

### Single Test (important)
- One package, one test:
  - `go test ./internal/logs -run '^TestParseDockerStream$' -v`
- Another package example:
  - `go test ./internal/docker -run '^TestNormalizeStats$' -v`
- Pattern in one package:
  - `go test ./internal/alerts -run 'TestCompare' -v`
- Full repo match (slower):
  - `go test ./... -run '^TestInferLevel$' -v`

### Lint / Static Analysis
No dedicated lint config (`golangci` config not present).
Use baseline checks:
- Format: `gofmt -w ./cmd ./internal`
- Vet: `go vet ./...`
- Optional if installed: `staticcheck ./...`

### Docker
- `docker compose up -d --build`
- `docker build -t dashi:local .`

## Go Style Guide

Follow existing conventions seen in `internal/*`.

### Imports
- Group imports: standard library, blank line, then `dashi/internal/...`.
- Keep imports explicit and one per line.
- Use blank imports only for required side effects (sqlite driver).

### Formatting
- Run `gofmt` on every touched Go file.
- Prefer early returns over deep nesting.
- Keep functions focused; extract helpers when branches grow.
- Keep comments minimal; only for non-obvious intent.

### Types
- Put shared domain data in `internal/models` structs.
- Use `map[string]any` only for flexible view/API payloads.
- Use pointers for optional fields (`*time.Time`, `*string`).
- Persist timestamps in UTC (`time.Now().UTC()`, `t.UTC()`).

### Naming
- Exported: `PascalCase`; unexported: `camelCase`.
- Constructors: `NewX(...)`.
- Verb-based method names: `Run`, `Tick`, `Evaluate`, `Reconcile`.
- Keep package names short and lowercase.

### Error Handling
- Return errors from lower layers (`db`, `docker`, parser/helpers).
- Wrap errors when adding context (`fmt.Errorf("...: %w", err)`).
- At service loops/orchestrators, log and continue on partial failures.
- Avoid panic for expected runtime failures.
- Ignore errors only when explicitly non-critical and safe.

### Logging
- Use `log/slog` structured logging.
- Keep message text concise and lowercase (existing pattern).
- Include useful keys (`err`, `id`, `container`, `rule_id`, etc.).
- Use module loggers via `logger.With("module", "...")`.

### Context
- `context.Context` is first parameter for I/O or long-running ops.
- Propagate request context through HTTP -> repo calls.
- Honor cancellation in worker loops and blocking operations.

### Database/SQL
- Keep SQL inside repository layer (`internal/db/repo.go`).
- Use `?` placeholders; do not interpolate untrusted input.
- Guard limits/defaults before query execution.
- Use transactions/prepared statements for batched inserts.

### HTTP Handlers
- Validate method/input early and return quickly.
- Use accurate status codes (`400`, `404`, `405`, `500`, `503`).
- Keep parsing close to handler usage.
- Reuse helpers for repeated logic (`writeJSON`, range parsing).

### Tests
- Place tests alongside code as `*_test.go`.
- Prefer table-driven tests when covering operator/branch matrices.
- Use `-run` with anchored regex for single-test execution.
- Keep tests deterministic and isolated from live Docker where possible.

## Frontend Conventions (existing codebase)
- Keep JS/CSS lightweight and framework-free unless asked.
- Preserve CSS variable/token approach in `web/static/style.css`.
- Preserve server-rendered template flow in `web/templates`.

## Agent Working Agreements
- Prefer small, targeted edits over broad refactors.
- Preserve route contracts unless task explicitly changes behavior.
- Avoid new dependencies unless clearly justified.
- Update tests/docs when behavior or commands change.

## Git Workflow Notes
- Primary branch: `master`.
- Remote repository: `git@github.com:sonac/dashi.git`.
- Keep commits focused and descriptive.
- Before pushing, run at least `go test ./...`.

## Cursor/Copilot Rules
Checked these locations:
- `.cursorrules`
- `.cursor/rules/`
- `.github/copilot-instructions.md`

Current status: none of these files exist in this repository.
If they are added later, treat them as repository-level instructions and update this file accordingly.
