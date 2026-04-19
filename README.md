# claude-agent-go

An open-source boilerplate for building Claude-powered agent backends in **Go**.

Cloud-deployable, batteries-included, opinionated.

## Stack

| Layer | Technology |
| --- | --- |
| Language | Go 1.23 |
| HTTP | [Fiber v2](https://github.com/gofiber/fiber) |
| LLM | [`anthropic-sdk-go`](https://github.com/anthropics/anthropic-sdk-go) (official) with a hand-rolled tool-use loop |
| Database | [TimescaleDB](https://github.com/timescale/timescaledb) (Postgres 16 + hypertables for messages) |
| DB driver | [`pgx/v5`](https://github.com/jackc/pgx) |
| Migrations | [Goose v3](https://github.com/pressly/goose) (embedded SQL) |
| Auth | [`magiclink-auth-go`](https://github.com/teslashibe/magiclink-auth-go) — OTP + magic link, HS256 JWT |
| Streaming | Server-Sent Events (SSE) for agent runs |
| Container | Multi-stage Dockerfile (distroless-style alpine) |
| Orchestration | docker-compose for local; Fly.io / Cloud Run / Railway / K8s for prod |

## Why this exists

Anthropic does not ship an official "Agent SDK" for Go (only TypeScript and Python). This repo gives you the same shape — sessions, tool-use loop, streaming, persistence — built on top of the official `anthropic-sdk-go` client. No Node.js sidecar, no CLI subprocess, no surprises in production.

## Repository layout

```text
agent-setup/
├── backend/
│   ├── cmd/
│   │   ├── server/        # Fiber API entrypoint
│   │   └── migrate/       # Goose migration runner (embedded SQL)
│   ├── internal/
│   │   ├── agent/         # Anthropic client + tool-use loop + SSE + persistence
│   │   ├── apperrors/
│   │   ├── auth/          # Magic-link + JWT middleware
│   │   ├── bootstrap/
│   │   ├── config/
│   │   ├── db/
│   │   │   └── migrations/  # *.sql goose files (embedded)
│   │   └── httputil/
│   └── Dockerfile
├── deploy/
│   ├── fly.toml
│   ├── railway.toml
│   ├── cloudrun.md
│   └── k8s/
├── .github/workflows/
├── docker-compose.yml
└── Makefile
```

## Quickstart

### 1) Prerequisites

- Go 1.23+
- Docker + Docker Compose
- An `ANTHROPIC_API_KEY` ([console.anthropic.com](https://console.anthropic.com))

### 2) Setup

```bash
make setup
# edit backend/.env and set ANTHROPIC_API_KEY=sk-ant-...
```

### 3) Run

```bash
make up      # full stack in Docker
# or
make dev     # API on host, Timescale + migrations in Docker
```

The API will be on [http://localhost:8080](http://localhost:8080).

### 4) Get a dev token

```bash
make token
# returns { "jwt": "..." }
```

## API

### Auth

| Method | Path | Description |
| --- | --- | --- |
| `POST` | `/auth/magic-link` | Send OTP + magic link to email |
| `POST` | `/auth/verify` | Exchange OTP for JWT |
| `GET` | `/auth/verify?token=...` | Magic-link click handler |
| `POST` | `/auth/login` | **Dev only** — issue JWT directly |

### Agent (all require `Authorization: Bearer <jwt>`)

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/me` | Current user |
| `POST` | `/api/agent/sessions` | Create a new session |
| `GET` | `/api/agent/sessions` | List your sessions |
| `GET` | `/api/agent/sessions/:id/messages` | Replay a session |
| `POST` | `/api/agent/sessions/:id/run` | Send a message; **streams SSE** of agent events |

### Example: chat with the agent

```bash
TOKEN=$(make -s token | jq -r .jwt)

# Create a session
SESSION=$(curl -s -X POST http://localhost:8080/api/agent/sessions \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"title":"hello world"}' | jq -r .id)

# Stream a run
curl -N -X POST http://localhost:8080/api/agent/sessions/$SESSION/run \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"message":"What time is it in Tokyo?"}'
```

## Adding tools

Tools live in `backend/internal/agent/tools.go`. Implement the `Tool` interface and register it in `DefaultRegistry`:

```go
type Tool interface {
    Name() string
    Description() string
    InputSchema() map[string]any
    Execute(ctx context.Context, input json.RawMessage) (any, error)
}
```

A `get_current_time` example tool ships with the boilerplate.

## Adding migrations

```bash
make migrate-create NAME=add_widgets
# writes backend/internal/db/migrations/<timestamp>_add_widgets.sql
make migrate
```

Migration files use the [Goose](https://github.com/pressly/goose) annotated SQL format.

## Cloud deployment

The `deploy/` folder contains starter configs for the four most common targets:

| Target | Config | Notes |
| --- | --- | --- |
| **Fly.io** | `deploy/fly.toml` | Easiest. `fly launch --no-deploy && fly secrets set ANTHROPIC_API_KEY=... && fly deploy` |
| **Railway** | `deploy/railway.toml` | Push to GitHub → auto-deploy |
| **GCP Cloud Run** | `deploy/cloudrun.md` | Container image + Cloud SQL Postgres with Timescale extension |
| **Kubernetes** | `deploy/k8s/` | Plain manifests — `kubectl apply -k deploy/k8s` |

For managed Timescale, use [Timescale Cloud](https://www.timescale.com/cloud) and set `DATABASE_URL` accordingly.

## License

MIT — see [LICENSE](./LICENSE).
