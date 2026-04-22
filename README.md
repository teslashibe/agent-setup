# claude-agent-go

> The seed for shipping Claude-powered agent products to clients.
> Built on **[Claude Managed Agents](https://platform.claude.com/docs/en/managed-agents/overview)**.
> One repo. One auth flow. iOS + Android + Web from the same codebase.

[![ci](https://github.com/teslashibe/agent-setup/actions/workflows/ci.yml/badge.svg)](https://github.com/teslashibe/agent-setup/actions/workflows/ci.yml)
[![docker](https://github.com/teslashibe/agent-setup/actions/workflows/docker.yml/badge.svg)](https://github.com/teslashibe/agent-setup/actions/workflows/docker.yml)
[![license: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](./LICENSE)

> **Use this template** → spin up a new client repo. Customize. Deploy. Done in a day.

## Stack

| Layer | Tech |
| --- | --- |
| Agent runtime | [Claude Managed Agents](https://platform.claude.com/docs/en/managed-agents/overview) — Anthropic hosts the long-running agent harness, runs the tool-use loop, and provides bash/web/file tools in a managed container |
| Anthropic client | [`anthropic-sdk-go`](https://github.com/anthropics/anthropic-sdk-go) — used **only** for Beta endpoints (`Beta.Agents`, `Beta.Sessions`, `Beta.Sessions.Events`). No `Messages` API, no hand-rolled tool loop. |
| API | Go 1.25 · [Fiber v2](https://github.com/gofiber/fiber) · [pgx/v5](https://github.com/jackc/pgx) |
| Database | [TimescaleDB](https://github.com/timescale/timescaledb) (Postgres 16) · [Goose](https://github.com/pressly/goose) migrations |
| Auth | [`magiclink-auth-go`](https://github.com/teslashibe/magiclink-auth-go) — OTP + magic link, HS256 JWT |
| Streaming | Server-Sent Events for agent runs |
| Mobile + Web | [Expo SDK 55](https://expo.dev) — iOS, Android, **and Web** from one codebase |
| UI | NativeWind v4, shadcn-style primitives, dark theme |
| Container | Multi-stage Alpine Dockerfile, image published to GHCR |

---

## Deploying a new client

This is the workflow you, the developer, follow to ship a new client product.

### 1) Create the client repo from this template

```bash
gh repo create teslashibe/<client-name> \
  --template teslashibe/agent-setup \
  --private --clone

cd <client-name>
```

You now have a complete working app. Everything below is per-client customization.

### 2) Bootstrap dev

```bash
make setup
```

This copies `.env.example → backend/.env` and `npm install`s the mobile app.

Open `backend/.env` and set:

```bash
ANTHROPIC_API_KEY=sk-ant-...
JWT_SECRET=$(openssl rand -hex 32)
RESEND_API_KEY=re_...           # for production magic-link emails
```

### 3) Provision the client's Anthropic Agent

Each client gets their **own Anthropic Agent + Environment**. This is what defines the AI's persona, model, and available tools.

Edit `backend/.env` and set the system prompt:

```bash
AGENT_SYSTEM_PROMPT="You are <Client>'s expert assistant. You help with X, Y, Z..."
```

Then run:

```bash
make managed-agents-provision
```

This prints two IDs — paste them into `backend/.env`:

```bash
ANTHROPIC_AGENT_ID=agent_011...
ANTHROPIC_ENVIRONMENT_ID=env_01...
```

The Agent now has bash, web search/fetch, file ops, and code execution available out of the box. To customize tools further, update the Agent in the [Anthropic Console](https://platform.claude.com/) — or re-run `provision` with a different system prompt and replace the IDs.

### 4) Brand the app

Edit `mobile/app.config.ts`:

```typescript
name: "Client Name",
slug: "client-name",
scheme: "clientapp",
ios:     { bundleIdentifier: "com.client.app" },
android: { package:          "com.client.app" },
```

Edit `mobile/.env.example`:
```bash
EXPO_PUBLIC_APP_SLUG=clientapp
```

Adjust theme colors in `mobile/tailwind.config.js` and `mobile/theme/tokens.ts`.

### 5) Run locally

```bash
make dev-all       # API on :8080, Expo Web on :8081
# or:
make up            # full stack in Docker
make dev-mobile    # Expo iOS/Android dev server (scan QR with Expo Go)
```

Sign in: enter any email at the welcome screen → check the API logs for the OTP → enter it.

### 6) Deploy backend

This repo doesn't ship cloud-specific configs — deploy it however your infrastructure works.

The container image builds automatically on every push to `main` and on tags via `.github/workflows/docker.yml`, publishing to:

```
ghcr.io/<your-org>/<client-name>:main-<timestamp>-<sha>
```

What needs to be provided in your deployment environment:

**Required environment variables** (see `backend/.env.example` for the full list):
- `ANTHROPIC_API_KEY`, `ANTHROPIC_AGENT_ID`, `ANTHROPIC_ENVIRONMENT_ID`
- `JWT_SECRET` (`openssl rand -hex 32`)
- `DATABASE_URL`
- `RESEND_API_KEY`, `AUTH_EMAIL_FROM`
- `APP_URL`, `CORS_ALLOWED_ORIGINS`

**Run order:**
1. `/bin/migrate up` — runs Goose migrations (one-shot job per deploy)
2. `/bin/server` — long-running API process

**Database:** any Postgres 16+ with the TimescaleDB extension installed. [Timescale Cloud](https://www.timescale.com/cloud) is the easiest managed option.

### 7) Ship the mobile app

For TestFlight / Play Store, EAS Build is the simplest path. See [issue #4](https://github.com/teslashibe/agent-setup/issues/4) — `eas.json` will land in v0.3.x.

For now, **Expo Web** at the deployed URL works on iOS Safari and Android Chrome with no app store needed.

---

## Repository layout

```text
agent-setup/
├── backend/
│   ├── cmd/
│   │   ├── server/        # Fiber API entrypoint
│   │   ├── migrate/       # Goose runner (up/down/status/reset)
│   │   └── provision/     # One-time: create Anthropic Agent + Environment
│   └── internal/
│       ├── agent/         # Managed Agents service, store, handler, model
│       ├── apperrors/     # Typed errors + Fiber ErrorHandler + UserID helper
│       ├── auth/          # Magic-link auth: service, middleware, handler
│       ├── teams/         # Team + membership + role middleware
│       ├── invites/       # Invite service, email render, public landing
│       ├── config/        # All env vars in one place
│       └── db/migrations/ # 00001_init.sql, 00002_teams.sql
├── mobile/                # Expo Router app — iOS, Android, Web
│   ├── app/(auth)/        # Magic-link sign-in (also handles invite flow)
│   ├── app/(app)/         # Sessions list + streaming chat + Teams tab
│   ├── app/invites/       # /invites/accept deep-link landing
│   ├── components/ui/     # NativeWind primitives
│   ├── providers/         # AuthSessionProvider, TeamsProvider
│   └── services/          # api.ts, auth.ts, agent.ts, teams.ts
├── .github/workflows/     # ci.yml + docker.yml (builds → ghcr.io)
├── docker-compose.yml
└── Makefile
```

---

## API

### Auth

| Method | Path | Description |
| --- | --- | --- |
| `POST` | `/auth/magic-link` | Send OTP + magic link |
| `POST` | `/auth/verify` | Exchange OTP for JWT |
| `GET` | `/auth/verify?token=…` | Magic-link click handler |
| `POST` | `/auth/login` | **Dev only** — issue JWT directly |

### Agent (require `Authorization: Bearer <jwt>`)

All `/api/agent/*` calls are scoped to the active team. Pass `X-Team-ID:
<team-uuid>` to choose a team; if omitted, the caller's personal team is used.

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/me` | Current user |
| `POST` | `/api/agent/sessions` | Create a session in the active team |
| `GET` | `/api/agent/sessions?scope=mine\|all` | List sessions; `scope=all` is admin+ only |
| `GET` | `/api/agent/sessions/:id` | Get one session (admins+ can read any in team) |
| `DELETE` | `/api/agent/sessions/:id` | Delete (owner of session OR admin+ in team) |
| `GET` | `/api/agent/sessions/:id/messages` | Replay full chat history (from Anthropic) |
| `POST` | `/api/agent/sessions/:id/run` | Send a message; **streams SSE** of agent events |

### Teams (require `Authorization: Bearer <jwt>`, see [docs/TEAMS.md](./docs/TEAMS.md))

Every user gets a `Personal` team auto-created on first login; other teams are
opt-in. Roles are `owner` > `admin` > `member`.

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/teams` | List the caller's memberships |
| `POST` | `/api/teams` | Create a team (caller becomes owner) |
| `GET` | `/api/teams/:teamID` | Get team metadata + caller role |
| `PATCH` | `/api/teams/:teamID` | Rename team (admin+) |
| `DELETE` | `/api/teams/:teamID` | Delete a non-personal team (owner only) |
| `GET` | `/api/teams/:teamID/members` | List members |
| `PATCH` | `/api/teams/:teamID/members/:userID` | Change role (owner only) |
| `DELETE` | `/api/teams/:teamID/members/:userID` | Remove member (admin+) |
| `DELETE` | `/api/teams/:teamID/members/me` | Leave a team (not allowed for personal) |
| `POST` | `/api/teams/:teamID/transfer-ownership` | Transfer to another member (owner only) |
| `GET` | `/api/teams/:teamID/invites` | List pending invites (admin+) |
| `POST` | `/api/teams/:teamID/invites` | Create invite + send email (admin+) |
| `POST` | `/api/teams/:teamID/invites/:id/resend` | Resend the email (admin+) |
| `DELETE` | `/api/teams/:teamID/invites/:id` | Revoke pending invite (admin+) |
| `GET` | `/api/invites/preview?token=…` | **Unauthenticated** preview for landing page |
| `POST` | `/api/invites/accept` | Accept invite (caller email must match) |
| `GET` | `/invites/accept?token=…` | **Public** HTML landing → deep-links into mobile app |

### Platform credentials (require `Authorization: Bearer <jwt>`)

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/platforms` | List every platform + connection status |
| `GET` | `/api/platforms/:platform/credentials` | Connection metadata (no secret returned) |
| `POST` / `PUT` | `/api/platforms/:platform/credentials` | Upsert encrypted credential |
| `DELETE` | `/api/platforms/:platform/credentials` | Disconnect platform |

### MCP server

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/mcp/v1/health` | Liveness (no auth) |
| `POST` | `/api/mcp/v1` | JSON-RPC 2.0 (header JWT) — `initialize`, `tools/list`, `tools/call` |
| `POST` | `/mcp/u/:token/v1` | Same surface, JWT in URL path (used by Anthropic Managed Agents) |

---

## MCP layer (Model Context Protocol)

`agent-setup` ships an in-process MCP server that exposes **374 tools across 14 platforms** (LinkedIn, X, Reddit, Hacker News, Facebook, Instagram, TikTok, Threads, Product Hunt, Nextdoor, ElevenLabs, Codegen/Claude Code, X Viral Scoring, Reddit Viral Scoring). Each tool wraps a method on the matching scraper package.

The Anthropic Managed Agent for each user is provisioned with `/mcp/u/<jwt>/v1` as its only MCP server, so any tool the agent calls runs through Fiber and operates on that user's encrypted credentials.

Token-efficiency is enforced server-side: every response is shaped to ≤50 items per array, ≤800 chars per string, and ≤64 KB total.

| Doc | What it covers |
| --- | --- |
| [`docs/mcp-architecture.md`](./docs/mcp-architecture.md) | Components, request lifecycle, drift-prevention, K8s notes |
| [`docs/credentials-setup.md`](./docs/credentials-setup.md) | How operators provision the encryption key, how users paste cookies/tokens |
| [`docs/mcp-inventory.md`](./docs/mcp-inventory.md) | Generated table of every registered tool (run `go run ./cmd/mcp-inventory` to refresh) |
| [`.cursor/rules/mcp-tool-conventions.mdc`](./.cursor/rules/mcp-tool-conventions.mdc) | The rule any AI agent follows when adding/auditing MCP tools |

### SSE event shape

```jsonc
{ "type": "tool_use",    "tool": "web_search", "tool_id": "sevt_…" }
{ "type": "tool_result", "tool_id": "sevt_…",  "is_error": false }
{ "type": "text",        "text": "Sure, here's what I found…" }
{ "type": "done" }
{ "type": "error",       "error": "..." }
```

---

## Configuration

All runtime config is environment variables. Defaults are in [`backend/internal/config/config.go`](./backend/internal/config/config.go).

| Variable | Required | Purpose |
| --- | --- | --- |
| `DATABASE_URL` | yes | Postgres / TimescaleDB connection string |
| `JWT_SECRET` | yes (prod) | HS256 signing secret — `openssl rand -hex 32` |
| `ANTHROPIC_API_KEY` | yes | Anthropic API key |
| `ANTHROPIC_AGENT_ID` | yes | From `make managed-agents-provision` |
| `ANTHROPIC_ENVIRONMENT_ID` | yes | From `make managed-agents-provision` |
| `AGENT_SYSTEM_PROMPT` | provision-only | The agent's persona |
| `AGENT_RUN_RATE_LIMIT` | no (default 10) | Requests per window per user on `/run` |
| `AGENT_RUN_RATE_WINDOW_SECONDS` | no (default 60) | Rate limit window |
| `RESEND_API_KEY` | for prod email | If unset, OTP codes log to stdout (dev mode) |
| `AUTH_EMAIL_FROM` | for prod email | The "From" address for magic-link emails |
| `APP_URL` | yes (prod) | Public URL of the deployed API |
| `CORS_ALLOWED_ORIGINS` | yes (prod) | Comma-separated list of allowed origins |
| `MOBILE_APP_SCHEME` | for deep links | URL scheme registered in `app.config.ts` |
| `PORT` | no (default 8080) | API listen port |
| `TEAMS_ENABLED` | no (default `true`) | Set `false` to disable `/api/teams` + invites |
| `TEAMS_DEFAULT_MAX_SEATS` | no (default `25`) | Seat cap for newly-created teams |
| `TEAMS_INVITE_TTL_HOURS` | no (default `168`) | Invite expiry; default = 7 days |
| `TEAMS_INVITE_FROM_NAME` | no (default `Agent App`) | Display name on invite emails |

---

## Common tasks

```bash
# Run the API + web together (host)
make dev-all

# Full stack in Docker
make up
make logs
make down

# Database
make db-shell                # psql in the postgres container
make db-reset                # destroy + recreate + migrate

# Migrations (we're in dev; edit 00001_init.sql in place + db-reset)
make migrate

# Backend tests + typecheck
make test
make lint
make typecheck               # mobile tsc

# Build everything
make build

# Get a dev JWT
make token
```

---

## Architecture

This is the **single template** — `template-app` was archived in favour of this repo. See [issue #1](https://github.com/teslashibe/agent-setup/issues/1) for full notes.

```
magiclink-auth-go    ← Go module, shared auth library
        ↓
agent-setup          ← THE template (this repo)
        ↓  "Use this template"
client repos         ← one per client, customized
```

Updates to the seed flow downstream by `git merge upstream/main` in client repos. Anthropic Agent + Environment are provisioned per-client and live in the client's Anthropic account.

---

## Open issues / roadmap

- [#2](https://github.com/teslashibe/agent-setup/issues/2) — Auto-generate session title from first message ✅ done in v0.2.0
- [#3](https://github.com/teslashibe/agent-setup/issues/3) — `DELETE /api/agent/sessions/:id` ✅ done in v0.2.0
- [#4](https://github.com/teslashibe/agent-setup/issues/4) — EAS build config for App Store / Play Store

---

## License

MIT — see [LICENSE](./LICENSE).
