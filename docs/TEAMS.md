# Teams

Multi-tenant collaboration on top of the agent-setup template, with a strict
Owner > Admin > Member role hierarchy and magic-link invite flow.

The full design rationale lives in
[`.cursor/tickets/teams-scope.md`](../.cursor/tickets/teams-scope.md). This
file is the operator/integrator handbook: how the pieces fit, where the
boundaries are, what to set in env, and how to test.

## TL;DR

- Every user gets a `Personal` team auto-created on first sign-in.
- Other teams are opt-in. Creator becomes owner.
- Roles: `owner` (1 per team) > `admin` (many) > `member`.
- All `/api/agent/*` requests are scoped to an active team via the
  `X-Team-ID` request header (defaults to the caller's personal team).
- Invitations are email-bound and travel through the existing magic-link
  flow — the recipient never sees a separate "create password" path.
- `TEAMS_ENABLED=false` strips the routes but keeps personal teams (so
  agent endpoints still work in single-tenant deployments).

## Roles

| Capability                                 | Owner | Admin | Member |
| ------------------------------------------ | :---: | :---: | :----: |
| Read team metadata                          |  ✅   |  ✅   |   ✅   |
| Create / read own agent sessions            |  ✅   |  ✅   |   ✅   |
| Read **other members'** agent sessions      |  ✅   |  ✅   |   ❌   |
| Invite members                              |  ✅   |  ✅   |   ❌   |
| Invite admins                                |  ✅   |  ❌   |   ❌   |
| Change member role                          |  ✅   |  ❌   |   ❌   |
| Remove member                               |  ✅   |  ✅\* |   ❌   |
| Remove another admin                        |  ✅   |  ❌   |   ❌   |
| Rename team                                 |  ✅   |  ✅   |   ❌   |
| Transfer ownership                          |  ✅   |  ❌   |   ❌   |
| Delete team                                 |  ✅   |  ❌   |   ❌   |

\* Admins can remove members but never other admins or the owner.

A handler that requires a minimum role uses `teams.RequireRole(min)`. The
fine-grained "admins can act on members but not other admins" check lives in
`teams.Service.canActOn` so it stays consistent across HTTP and any future
non-HTTP callers.

## Data model

```
users ──┐
        │
        ├──< team_members (team_id, user_id, role) >── teams
        │                                              │
        ├──< team_invites (token, email, role) ────────┘
        │
        └──< agent_sessions (team_id, user_id) ────────┘
```

- `teams.is_personal = true` rows are 1-per-user (enforced by partial unique
  index `idx_teams_personal_per_user`).
- Exactly one `team_members.role = 'owner'` per team (partial unique index
  `idx_team_members_one_owner`).
- One pending invite per `(team_id, lower(email))` (partial unique index
  `idx_team_invites_pending_email`). Re-inviting after a revoke or accept is
  always allowed.
- `agent_sessions.team_id` is `NOT NULL` (existing rows backfilled to each
  user's freshly-created personal team in the migration).

## Active team selection

The mobile app tracks an "active team" client-side. On every authenticated
request, `services/api.ts` stamps `X-Team-ID` with that team's id (unless the
caller opts out via `skipTeamHeader: true` for endpoints like `/api/me` and
`/api/teams` listing that operate on the user identity).

Server-side, `teams.Middleware.RequireTeam` resolves the active team:

1. If `X-Team-ID` is present → look up that membership, 403 if not a member.
2. Otherwise → the caller's personal team.

The `/api/teams/:teamID/*` routes use a separate `RequireTeamFromParam` so the
team is unambiguously identified by the path; the header is ignored there.

## Invite lifecycle

```
                   ┌──────────────────────────────────────────────┐
                   │ Admin clicks "Invite teammate"               │
                   │   POST /api/teams/:id/invites                │
                   │   { email, role }                            │
                   └──────────────────────────────────────────────┘
                                    │
                                    ▼
        Insert team_invites row (token = 32 random bytes, base64url),
        send branded email via Resend, return Invite (token redacted).
        On send failure → row is auto-revoked so admin can retry.
                                    │
                                    ▼
                  ┌─────────────────┴─────────────────┐
                  │                                   │
       Recipient clicks link.                Recipient enters email
       Public landing page deep-links        on welcome screen with
       to agentapp://invites/accept          ?invite_token attached.
       (or shows fallback HTML).
                  │                                   │
                  ▼                                   ▼
       Mobile /invites/accept screen.       Magic-link verify endpoint
       Three branches:                      (POST /auth/verify) reads
                                            invite_token from auth_codes
       a) signed-out → forward to           and auto-accepts.
          /(auth)/welcome with email
          + invite_token. Magic-link
          flow auto-accepts on verify.

       b) signed-in with right email →
          POST /api/invites/accept,
          switch active team, land in
          /(app)/teams/:id.

       c) signed-in with wrong email →
          explain + offer "sign out
          and continue".
```

Key safety properties:

- Invite emails are case-insensitive but the **acceptance check is strict**:
  the JWT email must match the invite address. Forwarding the link doesn't
  let someone else accept.
- Tokens are **single-use**. Once accepted or revoked the row is terminal;
  re-inviting issues a new token.
- The token is **only included in the API response** when first created or
  resent. `GET /api/teams/:id/invites` returns a redacted list — admins who
  need the link again call `POST /:id/resend`.
- Acceptance honours seat limits. Existing pending invites count against the
  cap so a flurry of invites can't blow past `max_seats`.

## Magic-link bridge (the important sketch)

We deliberately did not fork `magiclink-auth-go`. Instead we wrap its
`CodeStore` interface in a small adapter that knows about invites:

```
POST /auth/magic-link        ┌──────────────────────────────────────┐
   { email, invite_token }  →│ sendMagicLinkHandler                 │
                             │  • previewInvite(token) → assert      │
                             │    email matches                      │
                             │  • codeStore.SetPendingInvite(email,  │
                             │    invite_token)                      │
                             │  • magiclink.Send(...)                │
                             └──────────────────────────────────────┘
                                            │
                                            ▼
                             magiclink.Service.Create() calls
                             codeStore.Create(email, code, token, exp)
                             → adapter pulls invite_token from sync.Map
                               and persists it on auth_codes.invite_token

POST /auth/verify            ┌──────────────────────────────────────┐
   { email, code,            │ verifyCodeHandler                    │
     invite_token? }       → │  • magiclink.VerifyCode → JWT         │
                             │  • lookupInviteByToken / byEmail      │
                             │  • invites.AcceptByToken(...)          │
                             │  • return { jwt, invite_error? }      │
                             └──────────────────────────────────────┘
```

Errors during invite acceptance are non-fatal: login still succeeds, but the
response carries `invite_error` so the UI can warn the user.

## Disabling the feature

Set `TEAMS_ENABLED=false`:

- `/api/teams` and `/api/invites` and `/invites/accept` routes are not
  mounted.
- Personal teams are still auto-created on sign-in (so `agent_sessions` keeps
  working — the schema requires `team_id`).
- The mobile app gracefully handles the missing routes (the Teams tab will
  surface an empty state).

## Testing

```bash
# unit + integration suites (require TEST_DATABASE_URL)
cd backend
TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5434/teams_test?sslmode=disable" \
  go test ./internal/...

# typecheck mobile
cd mobile
npm run typecheck
```

The teams suite covers personal-team idempotency, role transitions,
ownership transfer, seat limits, slug uniqueness, and cross-team isolation.
The invites suite covers preview / accept happy path, email mismatch,
revoked / expired, and rollback on email-send failure. The agent store has
explicit cross-team isolation tests so regressions there fail loudly.

## Smoke test (end-to-end)

```bash
make up                              # full stack
TOK=$(curl -s -X POST http://localhost:8080/auth/login \
  -H 'content-type: application/json' \
  -d '{"email":"owner@local.test","name":"Owen"}' | jq -r .token)

# List memberships — personal team auto-created
curl -s -H "Authorization: Bearer $TOK" http://localhost:8080/api/teams/

# Create a team
TEAM=$(curl -s -X POST -H "Authorization: Bearer $TOK" \
  -H 'content-type: application/json' -d '{"name":"Acme Inc"}' \
  http://localhost:8080/api/teams/ | jq -r .team.id)

# Invite (without RESEND_API_KEY the link is logged to stderr)
curl -s -X POST -H "Authorization: Bearer $TOK" \
  -H 'content-type: application/json' \
  -d '{"email":"new@local.test","role":"member"}' \
  http://localhost:8080/api/teams/$TEAM/invites
```
