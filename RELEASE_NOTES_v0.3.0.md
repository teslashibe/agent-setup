# v0.3.0 — Production-ready Claude Managed Agents seed

This is the first release we'd actually ship to a paying client. The repo is now a
single-purpose, opinionated, deeply-tested template for building autonomous
Claude-powered products end-to-end.

It supersedes the v0.1.x and v0.2.x line. Everything from those releases has
been consolidated, validated, and where appropriate, replaced.

---

## What this repo is

A **GitHub Template Repository** that lets you spin up a complete Claude-agent
product for a client in roughly a day:

- **Go backend** (Fiber v2) with magic-link auth and a TimescaleDB session store
- **Expo app** (iOS, Android, and web from one codebase) with a streaming chat UI
- **Claude Managed Agents** doing the actual agent work — bash, web search, file
  ops, and code execution come for free; you just write a system prompt
- **Four deploy targets** wired up: Fly.io, Railway, GCP Cloud Run, Kubernetes
- **One DB migration** — we're in dev, the schema lives in `00001_init.sql`
- **~1,150 lines of Go** — every line earns its keep

---

## The big shift since v0.1.x: Claude Managed Agents

v0.1 shipped a hand-rolled tool-use loop on top of the Messages API. That's
been replaced entirely by [Claude Managed Agents](https://platform.claude.com/docs/en/managed-agents/overview),
Anthropic's managed agent runtime.

| Before (v0.1.x) | After (v0.3.0) |
|---|---|
| Hand-rolled tool-use loop in Go | Anthropic runs the loop |
| Custom `tools.go` registry, schemas, per-tool Go code | `agent_toolset_20260401` — bash, web, files free |
| `agent_messages` Timescale hypertable | Anthropic stores event history server-side |
| One `Messages.Create` call per agent turn | Stream from `/v1/sessions/:id/stream` |
| ~400 lines of agent loop code | ~190 lines of session/stream/history glue |

What that means in practice:
- You don't define tools in Go anymore. Tools come from the Anthropic
  toolset, MCP servers, or are added via the Anthropic Console.
- Conversation history isn't persisted in your DB — it lives with Anthropic
  and is fetched via the events list API on demand.
- Bash, web search, web fetch, file read/write/glob/grep, and code execution
  are all available to every agent automatically.

---

## What's in the box

### Backend (`backend/`)

```
cmd/
  server/      Fiber API entrypoint (~150 lines)
  migrate/     Goose runner: up / down / status / reset (~60 lines)
  provision/   One-time: create Anthropic Agent + Environment (~70 lines)
internal/
  agent/       service, store, handler, model — Managed Agents glue
  apperrors/   Typed errors + Fiber ErrorHandler + UserID helper
  auth/        Magic-link auth: service, middleware, handler
  config/      All env vars in one place
  db/migrations/ 00001_init.sql (the only schema file)
```

**Fiber idioms throughout.** Handlers return `error`; the centralized
`apperrors.FiberHandler` (wired via `fiber.Config{ErrorHandler}`) maps
typed errors to JSON responses. No `apperrors.Handle(c, err)` wrapping.

**DRY persistence helpers.** `scanSession` + `sessionFields` const in
`agent/store.go`; `scanUser` + `selectUserBy(column, value)` in
`auth/service.go`. Each user/session scan and field list lives in
exactly one place.

**Rate limiting on `/run`** scoped per authenticated user (10 req/60s by
default, env-configurable). Other endpoints unaffected.

### Mobile + Web (`mobile/`)

Expo SDK 55. One codebase serves iOS, Android, and the web via
`react-native-web`.

- **`(auth)/welcome.tsx`** — email → 6-digit OTP → JWT, with resend cooldown
- **`(app)/index.tsx`** — sessions list with pull-to-refresh, swipe to delete
- **`(app)/chat/[id].tsx`** — streaming chat, renders `tool_use` and
  `tool_result` blocks inline as they arrive
- **`services/agent.ts`** — typed CRUD plus an async-generator that consumes
  the SSE stream via `expo/fetch`

Token storage is config-driven via `EXPO_PUBLIC_APP_SLUG` so each client
fork uses its own secure-store key without conflicts.

### Deploy (`deploy/`)

| Target | File | Notes |
|---|---|---|
| Fly.io | `deploy/fly.toml` | One-command deploy with `release_command` migrations |
| Railway | `deploy/railway.toml` | Push to GitHub → auto-deploy |
| GCP Cloud Run | `deploy/cloudrun.md` | Step-by-step including Cloud Run Jobs for migrations |
| Kubernetes | `deploy/k8s/` | Namespace + secret + migrate Job + Deployment + Service + kustomization |

### CI / CD

- `.github/workflows/ci.yml` — `go vet` + `go build` + `go test` + `tsc --noEmit` on every push and PR
- `.github/workflows/docker.yml` — multi-arch image builds and pushes to `ghcr.io/teslashibe/agent-setup` on tags and `main`

---

## API surface

### Auth

| Method | Path | Description |
|---|---|---|
| `POST` | `/auth/magic-link` | Send OTP + magic link to email |
| `POST` | `/auth/verify` | Exchange OTP for JWT |
| `GET` | `/auth/verify?token=…` | Magic-link click handler |
| `POST` | `/auth/login` | **Dev only** — issue JWT directly |

### Agent (require `Authorization: Bearer <jwt>`)

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/me` | Current user |
| `POST` | `/api/agent/sessions` | Create a session (provisions Anthropic session under the hood) |
| `GET` | `/api/agent/sessions` | List your sessions |
| `GET` | `/api/agent/sessions/:id` | Get one session |
| `DELETE` | `/api/agent/sessions/:id` | Delete a session |
| `GET` | `/api/agent/sessions/:id/messages` | Replay full chat history (from Anthropic event list) |
| `POST` | `/api/agent/sessions/:id/run` | Send a message; **streams SSE** of agent events |

### SSE event shape

```jsonc
{ "type": "tool_use",    "tool": "web_search", "tool_id": "sevt_…" }
{ "type": "tool_result", "tool_id": "sevt_…",  "is_error": false }
{ "type": "text",        "text": "Sure, here's what I found…" }
{ "type": "done" }
{ "type": "error",       "error": "..." }
```

---

## Quickstart for a new client

```bash
# 1. Fork the template
gh repo create teslashibe/<client-name> --template teslashibe/agent-setup --private --clone
cd <client-name>

# 2. Bootstrap
make setup

# 3. Configure backend/.env
$EDITOR backend/.env
#   ANTHROPIC_API_KEY=sk-ant-...
#   JWT_SECRET=$(openssl rand -hex 32)
#   AGENT_SYSTEM_PROMPT="You are <Client>'s expert..."

# 4. Provision the client's Anthropic Agent
make managed-agents-provision
# paste ANTHROPIC_AGENT_ID and ANTHROPIC_ENVIRONMENT_ID into backend/.env

# 5. Brand the mobile app (mobile/app.config.ts, mobile/.env.example,
#    mobile/tailwind.config.js)

# 6. Run locally
make dev-all          # API on :8080, Expo Web on :8081

# 7. Deploy (Fly.io example)
fly launch
fly secrets set \
  ANTHROPIC_API_KEY=... \
  ANTHROPIC_AGENT_ID=... \
  ANTHROPIC_ENVIRONMENT_ID=... \
  JWT_SECRET=... \
  RESEND_API_KEY=... \
  DATABASE_URL=...
fly deploy
```

See [README.md](https://github.com/teslashibe/agent-setup/blob/main/README.md) for the full step-by-step.

---

## Refactor history (this release supersedes)

This release captures the entire arc since v0.1.0:

- **v0.1.0** — Initial release: Fiber + Goose + TimescaleDB + Expo + hand-rolled Messages API tool-use loop
- **v0.1.1** — Dockerfile fixed to match local Go toolchain
- **v0.1.2** — End-to-end tested live against Anthropic API
- **v0.1.3** — Per-user rate limiting on `/run`; backlog issues filed for auto-title, delete session, EAS build
- **v0.2.0** — Migrated to Claude Managed Agents; deleted hand-rolled tool loop and `agent_messages` table; closed auto-title and delete-session issues
- **v0.2.1** — Removed 4 packages (`bootstrap/`, `db/db.go`, `httputil/`, `auth/model.go`); 6 migrations consolidated into 1
- **v0.2.2** — Idiomatic Fiber refactor: handlers return `error`, centralized `ErrorHandler`, DRY scan helpers
- **v0.3.0** *(this release)* — Documentation rewrite focused on the per-client deployment workflow

The codebase is now ~1,150 lines of Go for a complete Claude agent product
with magic-link auth, sessions, streaming, history, rate limiting, and four
cloud deploy targets.

---

## Known limitations

- **EAS build config not yet included** — see [#4](https://github.com/teslashibe/agent-setup/issues/4). For now, Expo Web + dev-server `npm run start` cover all needs short of App Store / Play Store submission.
- **Email delivery in dev mode prints to logs** — set `RESEND_API_KEY` in production or no users will receive their OTP.
- **Anthropic Managed Agents is in beta** (`managed-agents-2026-04-01` header). Anthropic notes "behaviors may be refined between releases." We pin the SDK version in `go.mod` and verify each upgrade.

---

## What we explicitly chose NOT to do

- **No monorepo or workspaces** — adds tooling complexity for two apps
- **No git submodules** — annoying and we'd regret them
- **No premature package extraction** — `magiclink-auth-go` is the only shared lib until a real second consumer needs more
- **No custom MCP server wrapper** — clients can wire MCP via the Anthropic Agent config
- **No image / file upload pipeline** — add per-client when needed; the Managed Agent already has file ops in its container

---

## Credits

Built on [Claude Managed Agents](https://platform.claude.com/docs/en/managed-agents/overview),
[`anthropic-sdk-go`](https://github.com/anthropics/anthropic-sdk-go),
[Fiber](https://github.com/gofiber/fiber),
[Goose](https://github.com/pressly/goose),
[TimescaleDB](https://github.com/timescale/timescaledb),
[Expo](https://expo.dev),
[NativeWind](https://www.nativewind.dev),
and [`magiclink-auth-go`](https://github.com/teslashibe/magiclink-auth-go).
