# claude-agent-go

> The seed for shipping Claude-powered agent products.
> One repo, one auth flow, one database, three surfaces: **Go API · iOS · Android · Web.**

[![ci](https://github.com/teslashibe/agent-setup/actions/workflows/ci.yml/badge.svg)](https://github.com/teslashibe/agent-setup/actions/workflows/ci.yml)
[![docker](https://github.com/teslashibe/agent-setup/actions/workflows/docker.yml/badge.svg)](https://github.com/teslashibe/agent-setup/actions/workflows/docker.yml)
[![license: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](./LICENSE)

> **Use this template** → clone the seed and ship a client agent product end-to-end.

## Stack

| Layer | Technology |
| --- | --- |
| Backend | Go 1.23, [Fiber v2](https://github.com/gofiber/fiber), [pgx/v5](https://github.com/jackc/pgx) |
| LLM | [`anthropic-sdk-go`](https://github.com/anthropics/anthropic-sdk-go) v1.37 with a hand-rolled tool-use loop |
| Database | [TimescaleDB](https://github.com/timescale/timescaledb) (Postgres 16 + hypertable for messages) |
| Migrations | [Goose v3](https://github.com/pressly/goose) (embedded SQL) |
| Auth | [`magiclink-auth-go`](https://github.com/teslashibe/magiclink-auth-go) — OTP + magic link, HS256 JWT |
| Streaming | Server-Sent Events for agent runs |
| Mobile + Web | [Expo SDK 55](https://expo.dev) (iOS, Android, **and Web** from one codebase) |
| UI | NativeWind v4 + Tailwind, shadcn-style primitives |
| Container | Multi-stage Dockerfile (Alpine) |
| Cloud | Fly.io · Railway · GCP Cloud Run · Kubernetes |

## Why this exists

Anthropic does not ship an official **Agent SDK** for Go (only TypeScript and Python). This repo gives you the same shape — sessions, tool-use loop, streaming, persistence — built directly on the official `anthropic-sdk-go`. No Node sidecar, no CLI subprocess.

It's also a complete product seed: the same backend serves an **Expo app that runs on iOS, Android, and the Web** with shared auth and a streaming chat UI.

## Repository layout

```text
agent-setup/
├── backend/
│   ├── cmd/
│   │   ├── server/        # Fiber API entrypoint
│   │   └── migrate/       # Goose migration runner (embedded SQL)
│   ├── internal/
│   │   ├── agent/         # Anthropic client + tool-use loop + SSE + persistence
│   │   ├── apperrors/, auth/, bootstrap/, config/, db/, httputil/
│   │   └── db/migrations/   # *.sql goose files (embedded)
│   └── Dockerfile
├── mobile/                # Expo app — iOS, Android, AND web
│   ├── app/               # expo-router routes
│   │   ├── (auth)/        # magic-link sign-in
│   │   └── (app)/
│   │       ├── index.tsx        # Sessions list
│   │       ├── chat/[id].tsx    # Streaming chat (SSE)
│   │       └── settings.tsx
│   ├── components/, providers/, services/, theme/
│   └── app.config.ts
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
- Node.js 20+
- Docker + Docker Compose
- An `ANTHROPIC_API_KEY` ([console.anthropic.com](https://console.anthropic.com))

### 2) Setup

```bash
make setup
# edit backend/.env, set ANTHROPIC_API_KEY=sk-ant-...
```

### 3) Run

```bash
make dev-all      # API on :8080 + Expo Web on :8081
# or:
make up           # full stack in Docker
make dev-mobile   # Expo iOS/Android dev server
```

Then open:

- **API:** [http://localhost:8080/health](http://localhost:8080/health)
- **Web app:** [http://localhost:8081](http://localhost:8081)
- **Mobile:** scan the Expo QR with Expo Go

### 4) Use it

1. Open the web/mobile app → enter any email → check the API logs for the OTP code (dev sender prints it) → sign in.
2. Tap **New** to create a chat.
3. Send a message like *"What time is it in Tokyo?"* — watch the streaming response and the `get_current_time` tool call render in real-time.

## API

### Auth

| Method | Path | Description |
| --- | --- | --- |
| `POST` | `/auth/magic-link` | Send OTP + magic link |
| `POST` | `/auth/verify` | Exchange OTP for JWT |
| `GET` | `/auth/verify?token=…` | Magic-link click handler |
| `POST` | `/auth/login` | **Dev only** — issue JWT directly |

### Agent (require `Authorization: Bearer <jwt>`)

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/me` | Current user |
| `POST` | `/api/agent/sessions` | Create a session |
| `GET` | `/api/agent/sessions` | List your sessions |
| `GET` | `/api/agent/sessions/:id/messages` | Replay a session |
| `POST` | `/api/agent/sessions/:id/run` | Send a message; **streams SSE** |

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
make migrate
```

## Cloud deployment

| Target | Config | Notes |
| --- | --- | --- |
| **Fly.io** | `deploy/fly.toml` | `fly launch && fly secrets set ANTHROPIC_API_KEY=… && fly deploy` |
| **Railway** | `deploy/railway.toml` | Push to GitHub → auto-deploy |
| **GCP Cloud Run** | `deploy/cloudrun.md` | Image + Timescale Cloud or self-managed Postgres |
| **Kubernetes** | `deploy/k8s/` | `kubectl apply -k deploy/k8s` |

For managed TimescaleDB, use [Timescale Cloud](https://www.timescale.com/cloud) and set `DATABASE_URL`.

## Architecture

See [issue #1](https://github.com/teslashibe/agent-setup/issues/1) for the layered architecture and roadmap. TL;DR: this repo is a **GitHub Template Repository**. Fork it per client, customize, ship. Reusable code that needs to flow downstream lives in versioned packages (Go modules + npm packages) — extracted only when a real second consumer needs them.

## License

MIT — see [LICENSE](./LICENSE).
