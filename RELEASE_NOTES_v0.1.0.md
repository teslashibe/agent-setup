# v0.1.0 — The seed lands

The first cut of `claude-agent-go` — an opinionated, MIT-licensed seed for shipping Claude-powered agent products in Go, end-to-end.

Hit **Use this template** on the repo to spin up a new client app that already has auth, a streaming agent backend, persistent sessions, an iOS / Android / Web client, and four cloud deploy targets ready to go.

## Highlights

- **Pure Go agent runtime** built directly on the official [`anthropic-sdk-go`](https://github.com/anthropics/anthropic-sdk-go) — no Node sidecar, no CLI subprocess, no Python.
- **One Expo codebase, three surfaces** — iOS, Android, and the Web all share `mobile/` via `react-native-web`.
- **Streaming chat out of the box** — Server-Sent Events from Fiber, consumed in the Expo app via `expo/fetch` and an async-generator helper.
- **Sessions that survive restarts** — every assistant turn, tool call, and tool result is persisted to a TimescaleDB hypertable and re-hydrated into the agent loop on the next message.
- **Magic-link auth shared across surfaces** — OTP + magic link, HS256 JWT, [`magiclink-auth-go`](https://github.com/teslashibe/magiclink-auth-go) on the backend, secure-store on mobile, localStorage on web.
- **Deploy anywhere** — first-class configs for Fly.io, Railway, GCP Cloud Run, and Kubernetes (kustomize).

## What's in the box

### Backend (`backend/`)

- Fiber v2 API with magic-link auth and JWT middleware
- `internal/agent/` — Anthropic client, tool registry, **tool-use loop**, SSE streaming, persistence
- TimescaleDB schema with `agent_messages` as a hypertable partitioned on `created_at`
- Goose v3 migrations (embedded SQL, run via `/bin/migrate up`)
- Multi-stage Dockerfile producing `/bin/server` + `/bin/migrate`
- Example tool: `get_current_time` — drop in your own behind a single interface

```go
type Tool interface {
    Name() string
    Description() string
    InputSchema() map[string]any
    Execute(ctx context.Context, input json.RawMessage) (any, error)
}
```

### Mobile + Web (`mobile/`)

- Expo SDK 55, Expo Router, NativeWind v4, dark theme
- `(auth)/welcome.tsx` — email → OTP → JWT, with resend cooldown
- `(app)/index.tsx` — Sessions list with pull-to-refresh and empty state
- `(app)/chat/[id].tsx` — Streaming chat with live `tool_use` / `tool_result` cards
- `services/agent.ts` — typed CRUD plus async-generator SSE consumer

### Deploy (`deploy/`)

| Target | File | Notes |
| --- | --- | --- |
| Fly.io | `deploy/fly.toml` | One-command deploy with `release_command` migrations |
| Railway | `deploy/railway.toml` | Push to GitHub → auto-deploy |
| GCP Cloud Run | `deploy/cloudrun.md` | Step-by-step including Cloud Run Jobs for migrations |
| Kubernetes | `deploy/k8s/` | Namespace + secret + migrate Job + Deployment + Service + kustomization |

### CI / CD

- `.github/workflows/ci.yml` — `go vet` + `go build` + `go test` on every push and PR
- `.github/workflows/docker.yml` — multi-arch image builds and pushes to `ghcr.io/teslashibe/agent-setup` on tags and `main`

## API surface

### Auth

| Method | Path | Description |
| --- | --- | --- |
| `POST` | `/auth/magic-link` | Send OTP + magic link to email |
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

### SSE event shape

```jsonc
{ "type": "text",        "text": "Sure, in Tokyo" }
{ "type": "tool_use",    "tool": "get_current_time", "tool_id": "toolu_01…", "input": {…} }
{ "type": "tool_result", "tool": "get_current_time", "tool_id": "toolu_01…", "output": {…} }
{ "type": "usage",       "usage": { "input_tokens": 142, "output_tokens": 18 } }
{ "type": "done" }
```

## Quickstart

```bash
gh repo create my-client-app --template teslashibe/agent-setup --public --clone
cd my-client-app
make setup                          # installs mobile deps, copies .env files
$EDITOR backend/.env                # set ANTHROPIC_API_KEY
make dev-all                        # API on :8080, Expo Web on :8081
```

Open [http://localhost:8081](http://localhost:8081), sign in (the dev sender prints the OTP to the API logs), tap **New**, ask *"What time is it in Tokyo?"* and watch the agent stream a tool call back.

## Known limitations

- The example `get_current_time` tool is intentionally tiny — real tools live in your fork.
- No MCP server wiring yet (it's planned; the agent loop interface is designed to drop it in cleanly).
- Streaming chunks are emitted at the **content-block** boundary, not token-by-token. Token-level streaming is on the v0.2 list.
- Mobile push notifications, file uploads, and image content blocks are out of scope for v0.1.
- Backend tests are scaffolded but not exhaustive — primarily a build / vet pipeline today.

## Roadmap (v0.2 candidates)

- Token-level streaming via `client.Messages.NewStreaming`
- Built-in MCP client wired into the tool registry
- File upload + image content blocks
- Optional Postgres-only mode (skip Timescale extension for environments that don't have it)
- Dependabot / Renovate config in the template
- Native binary release (`.tar.gz`) per OS/arch alongside the Docker image

## Architecture

See [#1 — Architecture: agent-setup as the seed](https://github.com/teslashibe/agent-setup/issues/1) for the layered model. TL;DR: this repo is a **GitHub Template**. Reusable code that needs to flow downstream lives in versioned packages, extracted only when a real second consumer needs them.

## Upgrade path for client forks

```bash
git remote add upstream https://github.com/teslashibe/agent-setup.git
git fetch upstream
git merge upstream/main          # or cherry-pick specific commits
go get -u github.com/teslashibe/magiclink-auth-go
cd mobile && npm update
```

## Credits

Built with [`anthropic-sdk-go`](https://github.com/anthropics/anthropic-sdk-go), [Fiber](https://github.com/gofiber/fiber), [Goose](https://github.com/pressly/goose), [TimescaleDB](https://github.com/timescale/timescaledb), [Expo](https://expo.dev), and [NativeWind](https://www.nativewind.dev).
