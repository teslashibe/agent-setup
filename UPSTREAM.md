# Upstream

This repo is a downstream of [`teslashibe/template-app`](https://github.com/teslashibe/template-app).

```
template-app  ←  upstream (base: auth, UI, Go backend patterns)
     ↓
agent-setup   ←  this repo (adds: Claude agent loop, sessions, streaming chat)
     ↓
client-repos  ←  forked per client via "Use this template"
```

## Pulling upstream updates

```bash
make sync-upstream   # git fetch upstream && git merge upstream/main
```

Shared files (UI components, auth provider, backend middleware) merge cleanly.
Agent-specific files don't exist in `template-app` so they're never touched.

The only files that may need conflict resolution are the ones where both repos
deliberately diverge:

| File | Diverges because |
|---|---|
| `backend/go.mod` | agent-setup adds `anthropic-sdk-go` |
| `backend/internal/config/config.go` | agent-setup adds `ANTHROPIC_*` env vars |
| `backend/cmd/server/main.go` | agent-setup adds agent routes |
| `docker-compose.yml` | agent-setup uses TimescaleDB instead of plain Postgres |
| `mobile/app/(app)/index.tsx` | different home screen (sessions list vs placeholder) |
| `mobile/app/(app)/_layout.tsx` | different tabs (Chats/Settings vs Home/Settings) |
| `README.md` | different content |

Resolve those five-ish conflicts, commit, done.
