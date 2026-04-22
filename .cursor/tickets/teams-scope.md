# `agent-setup` — Teams (Owner / Admin / Member) Scope

**Repo:** `github.com/teslashibe/agent-setup`
**Affected packages:** `backend/internal/{auth,agent,teams,invites,config}`, `mobile/{app,services,providers,components}`
**Mirrors:** existing `agent-setup` conventions (Fiber + pgx + Goose, Expo Router + NativeWind, magic-link auth, Resend email)
**Purpose:** Add first-class team workspaces (owner / admin / member) to the `agent-setup` template so any client repo created from it ships with multi-user collaboration out of the box. Email invites go through Resend; invite acceptance and all auth stay magic-link driven.

---

## Goals

1. Every authenticated user belongs to at least one **team** (auto-created "Personal" team on first login).
2. Three roles per team: **owner**, **admin**, **member**, with a clear permission matrix.
3. **Invite flow** is admin/owner-initiated, sent via **Resend**, and accepted via a single magic link (no separate "accept invite" password flow).
4. The existing **agent session** model is extended to be team-scoped without breaking the current single-user UX (everything continues to work for a solo user with no setup).
5. Backend and frontend ship together — no half-built UI.
6. **Feature-flaggable**: a single `TEAMS_ENABLED=true` toggle controls whether team UI/API is exposed. Default ON in the template.

## Non-Goals (V1)

- Per-team Anthropic Agent + Environment IDs (V1 keeps the global `ANTHROPIC_AGENT_ID`; per-team override lands in V1.1).
- Cross-team session sharing or moving sessions between teams.
- Granular per-resource ACLs beyond the three roles.
- SSO / SAML / SCIM provisioning.
- Audit log UI (events are still logged server-side; UI lands later).
- Billing / seat enforcement (a `max_seats` column ships, but no Stripe wiring).

---

## Background — Why Teams in the Template

`agent-setup` is the seed for client deliverables. Today it ships single-user. Almost every client conversation hits the same wall within a week of launch: *"We need to add a teammate."* Putting teams in the template means that wall never gets hit — the moment a client wants collaboration, it's already there, behind a config flag.

The pattern below is deliberately **Linear / Notion / Slack-style**: a user always has a "Personal" team (single-member, owner is the user), and may belong to additional teams via invitation. Sessions live inside a team. There is no concept of a session that does not belong to a team.

---

## Architecture Decisions

These are the load-bearing choices. Each is followed by the alternative considered and why it was rejected. All are open to override in the **Decision Points** section at the bottom.

### 1. Teams are the primary tenant boundary

Sessions belong to a **(team, user)** pair, not just a user. Migrating the existing `agent_sessions` table:

- Add `team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE`.
- Backfill: for every existing user, create a "Personal" team and set `team_id` on all of their sessions to that team's ID.
- After backfill, drop the nullability and add a composite index `(team_id, user_id, updated_at DESC)`.

**Rejected:** Sessions stay user-owned and teams are a sharing layer on top. Cleaner integration into the current model but produces two parallel permission systems (user-owned vs team-shared) which is exactly the surface area we want to eliminate.

### 2. One owner per team, transferable

Exactly one row in `team_members` per team has `role = 'owner'`. Ownership transfer is a single endpoint that swaps roles atomically inside a transaction.

**Rejected:** Multiple owners (Notion-style). Simpler to reason about for a template; teams that want shared admin power use the **admin** role.

### 3. Member visibility: "see your own + you can see public team activity"

Within a team:

- A **member** sees only sessions where `user_id = self`.
- An **admin** sees all sessions in the team (read-only on others' sessions for V1 — no posting).
- An **owner** sees all sessions in the team (read-only on others' sessions for V1).

This matches the agent-chat use case where conversations frequently contain personal context the user does not want broadcast to peers, while still giving admins enough visibility to support / audit.

**Rejected:** "All members see all sessions by default." Too leaky for an AI chat product. Session-level sharing (a `share` toggle per session) is on the roadmap but out of scope for V1.

### 4. Invite acceptance flows through the existing magic-link path

When the invitee clicks the email link, they land on the API. Two cases:

- **Already authenticated** (JWT present) → consume invite, add to team, redirect to app.
- **Not authenticated** → bounce them through the standard `/auth/magic-link` flow with the `invite_token` carried as a query param. After verification, the same handler that issues the JWT also consumes the invite and adds the user to the team.

**Rejected:** Separate "create password / accept invite" page. We would have to build new UI and a new code path just to glue auth to invite consumption — magic-link already gives us a one-click path, we just need to plumb the token through.

### 5. Resend is the only email transport

The existing `magiclink-auth-go/resend.Sender` already integrates Resend for OTP/magic-link emails. The new **invite email** uses the same Resend client — we instantiate one shared `*resend.Sender` in `cmd/server/main.go` and inject it into both the magic-link service and a new `invites.Service`. No second email integration.

When `RESEND_API_KEY` is empty (dev mode), invite emails log to stdout in the same shape that magic-link OTPs already do — no surprises locally.

### 6. Anthropic Agent stays global in V1

Every team uses the same `ANTHROPIC_AGENT_ID` / `ANTHROPIC_ENVIRONMENT_ID`. Per-team agent overrides are a clean follow-up (just three new nullable columns on `teams`) but explicitly out of scope for V1.

### 7. Active team context is server-resolved per request

The mobile client sends the active team via header `X-Team-ID` on `/api/*` requests. The auth middleware resolves the (user, team) pair, validates membership, and attaches both the team and the role to the Fiber locals. Endpoints then guard on role through a small helper.

**Rejected:** Putting the active team into the JWT. Forces a token reissue on every team switch and complicates revocation.

---

## Permission Matrix

| Capability | Owner | Admin | Member |
|---|:---:|:---:|:---:|
| Create session in team | ✅ | ✅ | ✅ |
| Send messages in *own* session | ✅ | ✅ | ✅ |
| List *own* sessions | ✅ | ✅ | ✅ |
| List *all* team sessions (read-only on others) | ✅ | ✅ | ❌ |
| Delete own session | ✅ | ✅ | ✅ |
| Delete other members' sessions | ✅ | ✅ | ❌ |
| Invite new members | ✅ | ✅ | ❌ |
| Revoke pending invites | ✅ | ✅ | ❌ |
| Change member role (member ↔ admin) | ✅ | ✅ | ❌ |
| Remove member from team | ✅ | ✅ | ❌ |
| Remove other admin from team | ✅ | ❌ | ❌ |
| Rename team | ✅ | ✅ | ❌ |
| Transfer ownership | ✅ | ❌ | ❌ |
| Delete team | ✅ | ❌ | ❌ |

Special rules:

- A team always has exactly one owner. An owner cannot be removed or have their role changed; they must transfer first.
- An admin cannot demote / remove another admin or owner.
- A user cannot leave a team if they are the sole owner — they must transfer ownership first or delete the team.
- A user's "Personal" team is auto-created on signup. They cannot leave or delete it (it gets cleaned up when the user is deleted via cascade).

---

## Domain Model

### New tables

```sql
CREATE TABLE teams (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL,
    slug         TEXT NOT NULL UNIQUE,           -- url-friendly, lowercase, dash-separated
    is_personal  BOOLEAN NOT NULL DEFAULT FALSE, -- true for the auto-created per-user team
    max_seats    INT NOT NULL DEFAULT 25,        -- soft cap, enforced at invite-send time
    created_by   UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_teams_slug ON teams(slug);

CREATE TYPE team_role AS ENUM ('owner', 'admin', 'member');

CREATE TABLE team_members (
    team_id    UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       team_role NOT NULL,
    joined_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (team_id, user_id)
);
CREATE INDEX idx_team_members_user ON team_members(user_id);

-- Exactly one owner per team, enforced at the DB level
CREATE UNIQUE INDEX idx_team_members_one_owner
    ON team_members(team_id) WHERE role = 'owner';

CREATE TABLE team_invites (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id      UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    email        TEXT NOT NULL,
    role         team_role NOT NULL CHECK (role IN ('admin', 'member')),
    token        TEXT NOT NULL UNIQUE,
    invited_by   UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at   TIMESTAMPTZ NOT NULL,
    accepted_at  TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_team_invites_token ON team_invites(token);
CREATE INDEX idx_team_invites_team ON team_invites(team_id, accepted_at, revoked_at);

-- Prevent duplicate active invites for the same email within a team
CREATE UNIQUE INDEX idx_team_invites_pending_email
    ON team_invites(team_id, lower(email))
    WHERE accepted_at IS NULL AND revoked_at IS NULL;
```

### Modified tables

```sql
-- Add team scope to existing sessions
ALTER TABLE agent_sessions
    ADD COLUMN team_id UUID REFERENCES teams(id) ON DELETE CASCADE;

-- Backfill: every existing user gets a "Personal" team they own,
-- and all of their sessions move into it. (Run inside the migration.)
-- See "Migration & Backwards Compatibility" below for the SQL.

ALTER TABLE agent_sessions
    ALTER COLUMN team_id SET NOT NULL;

DROP INDEX IF EXISTS idx_agent_sessions_user;
CREATE INDEX idx_agent_sessions_team_user
    ON agent_sessions(team_id, user_id, updated_at DESC);
CREATE INDEX idx_agent_sessions_team
    ON agent_sessions(team_id, updated_at DESC);
```

### Go types (`backend/internal/teams/model.go`)

```go
package teams

import "time"

type Role string

const (
    RoleOwner  Role = "owner"
    RoleAdmin  Role = "admin"
    RoleMember Role = "member"
)

type Team struct {
    ID         string    `json:"id"`
    Name       string    `json:"name"`
    Slug       string    `json:"slug"`
    IsPersonal bool      `json:"is_personal"`
    MaxSeats   int       `json:"max_seats"`
    CreatedBy  string    `json:"created_by"`
    CreatedAt  time.Time `json:"created_at"`
    UpdatedAt  time.Time `json:"updated_at"`
}

type Member struct {
    TeamID   string    `json:"team_id"`
    UserID   string    `json:"user_id"`
    Email    string    `json:"email"`
    Name     string    `json:"name"`
    Role     Role      `json:"role"`
    JoinedAt time.Time `json:"joined_at"`
}

type Invite struct {
    ID         string     `json:"id"`
    TeamID     string     `json:"team_id"`
    Email      string     `json:"email"`
    Role       Role       `json:"role"`
    InvitedBy  string     `json:"invited_by"`
    ExpiresAt  time.Time  `json:"expires_at"`
    AcceptedAt *time.Time `json:"accepted_at,omitempty"`
    RevokedAt  *time.Time `json:"revoked_at,omitempty"`
    CreatedAt  time.Time  `json:"created_at"`
}

// Membership pairs a Team with the caller's role inside it.
type Membership struct {
    Team Team `json:"team"`
    Role Role `json:"role"`
}
```

---

## API Surface

All endpoints are under `/api/teams/...` and require `Authorization: Bearer <jwt>` unless noted. The active team header `X-Team-ID` is honored on `/api/agent/...` for session scoping (see "Auth middleware changes" below).

### Team lifecycle

| Method | Path | Role | Description |
|---|---|---|---|
| `GET` | `/api/teams` | any | List teams the caller is a member of, with their role in each. |
| `POST` | `/api/teams` | n/a | Create a new team. Caller becomes the owner. Body: `{ "name": "Acme" }`. |
| `GET` | `/api/teams/:teamID` | member+ | Get team details. |
| `PATCH` | `/api/teams/:teamID` | admin+ | Update name. |
| `DELETE` | `/api/teams/:teamID` | owner | Delete the team (and cascade all sessions/members/invites). Cannot delete a personal team. |
| `POST` | `/api/teams/:teamID/transfer-ownership` | owner | Body: `{ "to_user_id": "..." }`. Atomically swaps roles. |

### Members

| Method | Path | Role | Description |
|---|---|---|---|
| `GET` | `/api/teams/:teamID/members` | member+ | List members. |
| `PATCH` | `/api/teams/:teamID/members/:userID` | admin+ | Body: `{ "role": "admin" | "member" }`. Cannot target the owner. |
| `DELETE` | `/api/teams/:teamID/members/:userID` | admin+ | Remove member. Admins cannot remove other admins or the owner. |
| `DELETE` | `/api/teams/:teamID/members/me` | any | Leave the team (forbidden if sole owner of a non-personal team). |

### Invites

| Method | Path | Role | Description |
|---|---|---|---|
| `GET` | `/api/teams/:teamID/invites` | admin+ | List pending invites. |
| `POST` | `/api/teams/:teamID/invites` | admin+ | Body: `{ "email": "...", "role": "admin"\|"member" }`. Generates token, persists row, sends Resend email. |
| `POST` | `/api/teams/:teamID/invites/:inviteID/resend` | admin+ | Resends the email (does not rotate the token unless expired). |
| `DELETE` | `/api/teams/:teamID/invites/:inviteID` | admin+ | Revoke pending invite. |
| `GET` | `/api/invites/:token` | public | Inspect an invite (returns `{ team_name, role, email, expired }`). Used by mobile to render a "join Acme as admin" preview before forcing auth. |
| `POST` | `/api/invites/:token/accept` | required | Accept an invite as the currently authenticated user. Returns the team membership. |

### Updated agent endpoints

The shape of `/api/agent/sessions/*` does not change — but the **scope** does:

- All session reads are scoped to the **active team**, resolved from `X-Team-ID` header (falls back to the user's personal team if absent).
- Session writes (`POST`, `DELETE`, `run`) check membership + role for the active team.
- New optional query param `?scope=mine|all` on `GET /api/agent/sessions`. `mine` is the default for all roles; `all` is permitted only for admins/owners.

### Public web endpoint (browser landing)

| Method | Path | Description |
|---|---|---|
| `GET` | `/invites/accept?token=...` | HTML page. If user is authenticated (cookie/header), accepts and redirects to deep link. If not, kicks off magic-link flow with `invite_token` carried in state. |

---

## Auth Middleware Changes

`backend/internal/auth/middleware.go` gains a second middleware that runs after `RequireAuth`:

```go
func (m *Middleware) RequireTeam(svc *teams.Service) fiber.Handler {
    return func(c *fiber.Ctx) error {
        userID := apperrors.UserID(c)
        teamID := strings.TrimSpace(c.Get("X-Team-ID"))

        // If no team header, default to the user's personal team.
        membership, err := svc.ResolveActive(c.UserContext(), userID, teamID)
        if err != nil {
            return err
        }
        c.Locals("team_id", membership.Team.ID)
        c.Locals("team_role", string(membership.Role))
        return c.Next()
    }
}

// RequireRole is a small per-route gate.
func RequireRole(min teams.Role) fiber.Handler {
    return func(c *fiber.Ctx) error {
        role := teams.Role(c.Locals("team_role").(string))
        if !role.AtLeast(min) {
            return apperrors.ErrForbidden
        }
        return c.Next()
    }
}
```

Wiring in `cmd/server/main.go`:

```go
api := app.Group("/api", authMW.RequireAuth())

// Team management (no team context required — operates on path :teamID)
teams.NewHandler(teamSvc, inviteSvc).Mount(api)

// Anything that operates on agent state needs an active team
agentAPI := api.Group("", authMW.RequireTeam(teamSvc))
agent.NewHandler(agentSvc).Mount(agentAPI, runLimiter)
```

---

## Backend Implementation Plan

### New packages

```
backend/internal/
├── teams/
│   ├── model.go          # Team, Member, Invite, Role, Membership
│   ├── store.go          # pgx queries
│   ├── service.go        # business logic, role checks
│   ├── handler.go        # Fiber handlers, thin
│   └── service_test.go   # role hierarchy, invariants
└── invites/
    ├── email.go          # Resend renderer for invite emails
    ├── service.go        # CreateInvite, AcceptInvite, ResendInvite, RevokeInvite
    └── handler.go        # /api/invites/:token/accept handler
```

### `teams.Service` API (sketch)

```go
func (s *Service) CreatePersonal(ctx context.Context, user auth.User) (Team, error)
func (s *Service) Create(ctx context.Context, ownerID, name string) (Team, error)
func (s *Service) ListForUser(ctx context.Context, userID string) ([]Membership, error)
func (s *Service) ResolveActive(ctx context.Context, userID, requestedTeamID string) (Membership, error)
func (s *Service) UpdateName(ctx context.Context, teamID, name string) (Team, error)
func (s *Service) Delete(ctx context.Context, teamID string) error
func (s *Service) TransferOwnership(ctx context.Context, teamID, fromUserID, toUserID string) error

func (s *Service) ListMembers(ctx context.Context, teamID string) ([]Member, error)
func (s *Service) ChangeRole(ctx context.Context, teamID, actorID, targetID string, newRole Role) error
func (s *Service) RemoveMember(ctx context.Context, teamID, actorID, targetID string) error
func (s *Service) Leave(ctx context.Context, teamID, userID string) error
```

### `invites.Service` API

```go
type Service struct {
    pool   *pgxpool.Pool
    teams  *teams.Service
    auth   *auth.Service
    email  magiclink.EmailSender   // shared Resend client
    cfg    Config                   // AppURL, FromAddress, AppName, InviteTTL
    now    func() time.Time
}

func (s *Service) Create(ctx context.Context, teamID, invitedBy, email string, role teams.Role) (teams.Invite, error)
func (s *Service) Resend(ctx context.Context, inviteID string) (teams.Invite, error)
func (s *Service) Revoke(ctx context.Context, inviteID string) error
func (s *Service) Inspect(ctx context.Context, token string) (InvitePreview, error)
func (s *Service) Accept(ctx context.Context, token, userID string) (teams.Team, teams.Role, error)
```

### Hook points in existing code

- `auth.Service.UpsertIdentity` — on first-time insert, also call `teams.Service.CreatePersonal` inside the same transaction so a brand-new user lands in a workspace immediately.
- `agent.Store.CreateSession` — gain `teamID` argument, write to `team_id` column.
- `agent.Store.ListSessions` / `GetSession` / `DeleteSession` — accept `teamID` and the caller's role, return team-scoped results filtered by role.
- `cmd/server/magiclink.go` — extend `userStoreAdapter.UpsertUser` to invoke the personal-team bootstrap.

### Rate limits

- Existing `/api/agent/sessions/:id/run` limiter stays per-user.
- New limiter on `POST /api/teams/:teamID/invites`: **10 invites / hour per team**, configurable via `TEAMS_INVITE_RATE_LIMIT`.
- New limiter on `POST /api/invites/:token/accept`: **5 attempts / minute per IP** to slow token enumeration.

### Errors (extend `apperrors`)

```go
var (
    ErrTeamNotFound          = New(http.StatusNotFound,    "team not found")
    ErrNotTeamMember         = New(http.StatusForbidden,   "not a member of this team")
    ErrInsufficientRole      = New(http.StatusForbidden,   "your role does not permit this action")
    ErrCannotRemoveOwner     = New(http.StatusBadRequest,  "owner cannot be removed; transfer ownership first")
    ErrCannotLeavePersonal   = New(http.StatusBadRequest,  "cannot leave your personal team")
    ErrCannotDeletePersonal  = New(http.StatusBadRequest,  "cannot delete your personal team")
    ErrSeatLimitReached      = New(http.StatusBadRequest,  "team has reached its seat limit")
    ErrInviteNotFound        = New(http.StatusNotFound,    "invite not found")
    ErrInviteExpired         = New(http.StatusGone,        "invite expired")
    ErrInviteAlreadyAccepted = New(http.StatusConflict,    "invite already accepted")
    ErrInviteRevoked         = New(http.StatusGone,        "invite revoked")
    ErrEmailMismatch         = New(http.StatusForbidden,   "this invite was sent to a different email")
)
```

---

## Resend Invite Email

Reuses `magiclink-auth-go/resend.Sender`. The renderer lives in `internal/invites/email.go` and follows the same dark-theme aesthetic as the existing OTP email so the brand stays consistent.

```go
// internal/invites/email.go
type InviteEmail struct {
    AppName    string
    AppURL     string
    TeamName   string
    InviterName string
    Role       teams.Role
    AcceptURL  string  // {AppURL}/invites/accept?token=...
    ExpiresAt  time.Time
}

func renderInviteEmail(d InviteEmail) (subject, htmlBody string)
```

Subject format: `"{InviterName} invited you to join {TeamName} on {AppName}"`.

Body structure (HTML, table-based for email-client compatibility):
- Header: app name
- Headline: "You've been invited"
- Card: team name, role badge, inviter name
- Primary CTA button: "Accept invitation" → `AcceptURL`
- Fine print: "Expires in 7 days. If you weren't expecting this, you can safely ignore this email."

Dev mode (no `RESEND_API_KEY`): the subject + body get logged to stdout, identical to today's `devEmailSender` for OTP. The `AcceptURL` is plainly visible in the log so a dev can copy-paste it into a browser.

---

## Magic-Link Integration for Invite Acceptance

The whole point of "everything magic-link driven" is that the invitee never has to set a password and never has to learn a second flow. The integration boils down to two cases.

### Case A — Invitee already has an account, currently authenticated

```
Invite email → click → /invites/accept?token=TKN (browser)
  ↓
Backend reads JWT cookie/header
  ↓
invites.Accept(ctx, token, userID)  →  team_members row inserted
  ↓
Render success page → deep-link to mobile app: agentapp://teams/<teamID>
```

### Case B — Invitee not signed in (most common)

```
Invite email → click → /invites/accept?token=TKN
  ↓
Backend sees no auth, calls invites.Inspect(ctx, token) → renders a small page:

   "You've been invited to join <Team Name> as <role>.
    Sign in with <invite.email> to accept."

  [ Send magic link ]   ← button POSTs to /auth/magic-link
                          with body { email, invite_token: TKN }
  ↓
magiclink.Send delivers the OTP email as today
  ↓
User enters OTP in browser (or clicks the magic link)
  ↓
/auth/verify is hit; on success, magiclink-auth-go returns the JWT.
  ↓
Backend extension: if invite_token is present in the verify request,
call invites.Accept(ctx, invite_token, userID) before returning the
HTML success page. Page deep-links into the app on the team route.
```

### Why this works without forking `magiclink-auth-go`

`magiclink-auth-go` already supports passing arbitrary state through verification — we just need to:

1. Accept an `invite_token` field on the JSON body of `POST /auth/magic-link`, persist it on the `auth_codes` row (new column), and read it back on verify.
2. After `magicSvc.VerifyCode` / `VerifyToken` succeeds, look up the stored `invite_token` and call `invitesSvc.Accept`.
3. If acceptance fails, **still return the JWT** (the user authenticated successfully) but include an `invite_error` field so the client can surface the failure non-fatally.

Schema delta:

```sql
ALTER TABLE auth_codes
    ADD COLUMN invite_token TEXT;
```

The existing `codeStoreAdapter` in `cmd/server/magiclink.go` is updated to read/write that column.

### Email matching

Invites are bound to the email they were sent to. On acceptance, the invitee's authenticated email must match (case-insensitive) the invite's email, otherwise `ErrEmailMismatch`. This prevents "forward an invite to a friend" from accidentally giving the friend access.

If the invitee genuinely needs to use a different email, an admin can revoke and re-issue.

---

## Frontend Implementation Plan

The mobile app already has the routing scaffold (`(auth)`, `(app)`), an `AuthSessionProvider`, and a `FloatingTabBar` with both native bottom-bar and web sidebar variants. Teams plug in cleanly on top.

### New providers

`mobile/providers/TeamsProvider.tsx` — sits inside `AuthSessionProvider`:

```ts
type TeamsContextValue = {
  teams: Membership[];
  activeTeam: Team | null;
  activeRole: Role | null;
  isLoading: boolean;
  switchTeam: (teamID: string) => Promise<void>;
  refresh: () => Promise<void>;
};
```

Active team ID is persisted (SecureStore on native, localStorage on web) under `${APP_SLUG}_active_team`. On boot, the provider reads it back and sends it on every API request.

The active team header is plumbed by extending `services/api.ts` with a global `setActiveTeamProvider(() => teamID | null)`, mirroring the existing `setAccessTokenProvider` pattern. The provider hooks itself in via a `useEffect` exactly as `AuthSessionProvider` already does.

### New service module

`mobile/services/teams.ts`:

```ts
export type Role = "owner" | "admin" | "member";

export type Team = { id: string; name: string; slug: string; is_personal: boolean; max_seats: number; created_at: string; updated_at: string };
export type Membership = { team: Team; role: Role };
export type Member = { team_id: string; user_id: string; email: string; name: string; role: Role; joined_at: string };
export type Invite = { id: string; team_id: string; email: string; role: Role; expires_at: string; accepted_at?: string; revoked_at?: string; created_at: string };

export async function listTeams(): Promise<Membership[]>
export async function createTeam(name: string): Promise<Team>
export async function updateTeam(id: string, name: string): Promise<Team>
export async function deleteTeam(id: string): Promise<void>
export async function transferOwnership(teamID: string, toUserID: string): Promise<void>

export async function listMembers(teamID: string): Promise<Member[]>
export async function changeRole(teamID: string, userID: string, role: Role): Promise<void>
export async function removeMember(teamID: string, userID: string): Promise<void>
export async function leaveTeam(teamID: string): Promise<void>

export async function listInvites(teamID: string): Promise<Invite[]>
export async function sendInvite(teamID: string, email: string, role: "admin" | "member"): Promise<Invite>
export async function resendInvite(teamID: string, inviteID: string): Promise<Invite>
export async function revokeInvite(teamID: string, inviteID: string): Promise<void>
export async function acceptInvite(token: string): Promise<{ team: Team; role: Role }>
export async function inspectInvite(token: string): Promise<{ team_name: string; role: Role; email: string; expired: boolean }>
```

### New routes

```
mobile/app/(app)/
├── teams/
│   ├── _layout.tsx          # Stack: index → [id] → [id]/members → [id]/invites
│   ├── index.tsx            # List of teams + "Create team" CTA
│   ├── new.tsx              # Form: team name → POST /api/teams → switch into it
│   └── [id]/
│       ├── _layout.tsx
│       ├── index.tsx        # Team overview: name, role, member count, danger zone
│       ├── members.tsx      # List members; admin+ can change role / remove
│       └── invites.tsx      # Pending invites; admin+ can send / revoke / resend

mobile/app/(auth)/
└── invite.tsx               # Lands from deep link agentapp://invites/accept?token=...
                              # Uses inspectInvite() to render preview, then forwards
                              # to the standard sign-in flow with invite_token in state.
```

### Updated routes

`mobile/app/(app)/_layout.tsx` — add a third tab `teams`. Keep `chat` hidden as today.

`mobile/app/(app)/index.tsx` (Sessions list) — query is now scoped to the active team. For `admin`+ roles, show a `<Picker>` for `scope: mine | all` at the top. Header gains a small badge showing the active team name.

`mobile/components/FloatingTabBar.tsx` (Web sidebar variant) — replace the static "Claude Agent Go" header with a **team switcher dropdown**:

- Dropdown trigger shows current team name + role badge.
- Open shows: list of memberships, divider, "Create team", "Manage teams".
- Selecting a team calls `useTeams().switchTeam(id)` which updates the provider, re-issues a `refresh()` on the active screens via React Query / focus effects.

Native variant gets the team switcher inside Settings as a tappable row, given screen real estate.

### New UI primitives

Two small additions to `components/ui/`:

- `Select.tsx` — accessible single-select dropdown (cross-platform). Used for role pickers and team switcher.
- `Dialog.tsx` — modal dialog (cross-platform). Used for confirm-destructive actions (remove member, delete team, transfer ownership).

Plus a `RoleBadge` cosmetic in `components/RoleBadge.tsx` that wraps `Badge` with role-appropriate variant + label.

### Error / forbidden UX

The middleware returns `403 ErrInsufficientRole` with the structured error JSON the client already handles via `APIError`. The `TeamsProvider` exposes a `can(action: TeamAction): boolean` helper computed locally from the active role, so screens hide buttons rather than show-and-fail. This keeps the UI honest without an extra round-trip.

---

## Configuration

New env vars (additive to today's `backend/.env.example`):

```bash
# Teams
TEAMS_ENABLED=true                       # gates /api/teams/* routes & frontend UI
TEAMS_INVITE_TTL_HOURS=168               # 7 days
TEAMS_INVITE_RATE_LIMIT=10               # invites per hour per team
TEAMS_DEFAULT_MAX_SEATS=25               # initial cap on new teams
```

When `TEAMS_ENABLED=false`:

- Routes return 404.
- Personal team is still auto-created on signup (so existing user-scoped session behaviour keeps working transparently).
- Frontend reads `EXPO_PUBLIC_TEAMS_ENABLED` (mirror) and hides Teams tab + switcher.

`backend/internal/config/config.go` adds these as typed fields on `Config`. `mobile/config.ts` adds `TEAMS_ENABLED = process.env.EXPO_PUBLIC_TEAMS_ENABLED !== "false"`.

---

## Migration & Backwards Compatibility

A new migration file `backend/internal/db/migrations/00002_teams.sql` does the schema work above plus a transactional backfill:

```sql
-- +goose Up
-- +goose StatementBegin

-- (table + index DDL from "Domain Model" above goes here)

-- Backfill: create a Personal team for every existing user.
WITH new_teams AS (
    INSERT INTO teams (name, slug, is_personal, created_by)
    SELECT
        COALESCE(NULLIF(name, ''), 'Personal') || '''s Workspace',
        lower(regexp_replace(split_part(email, '@', 1), '[^a-z0-9]+', '-', 'g'))
            || '-' || substr(id::text, 1, 8),
        TRUE,
        id
    FROM users
    RETURNING id, created_by
)
INSERT INTO team_members (team_id, user_id, role)
SELECT id, created_by, 'owner' FROM new_teams;

-- Move every existing session into its owner's personal team.
UPDATE agent_sessions s
SET team_id = t.id
FROM teams t
WHERE t.created_by = s.user_id
  AND t.is_personal = TRUE;

ALTER TABLE agent_sessions ALTER COLUMN team_id SET NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE agent_sessions DROP COLUMN team_id;
DROP TABLE IF EXISTS team_invites;
DROP TABLE IF EXISTS team_members;
DROP TABLE IF EXISTS teams;
DROP TYPE IF EXISTS team_role;
ALTER TABLE auth_codes DROP COLUMN IF EXISTS invite_token;
-- +goose StatementEnd
```

The migration is **idempotent on a fresh DB** and **safe on an existing one** because:

1. New tables don't conflict with anything.
2. The backfill only operates on rows that exist; if there are no users yet, it's a no-op.
3. The `auth_codes.invite_token` column is added separately in the same migration.

---

## Implementation Order

Suggested commit/PR sequence — each item independently testable:

1. **Schema + migration** (`00002_teams.sql`) + Goose smoke test.
2. **`internal/teams` package** — model, store, service (no handler yet); unit tests for role hierarchy + "exactly one owner" + "cannot leave personal".
3. **`auth` integration** — bootstrap personal team on `UpsertIdentity`; update `magiclink.go` adapter.
4. **`teams` HTTP handler** + `RequireTeam` / `RequireRole` middleware + wiring in `cmd/server/main.go`.
5. **`internal/invites` package** — service + email renderer + handler. Reuse the existing Resend sender. Add `auth_codes.invite_token` column + adapter changes.
6. **Update `internal/agent`** — store + service + handler accept `team_id`; `?scope=mine|all` query.
7. **Mobile `services/teams.ts`** + `TeamsProvider` + `setActiveTeamProvider` plumbing in `services/api.ts`.
8. **Mobile UI** — Teams tab routes, web-sidebar team switcher, sessions list scoping, role-gated affordances.
9. **Mobile `(auth)/invite.tsx` deep-link landing** + plumbing through `AuthSessionProvider` so an unauthenticated invite click ends up logged in *and* joined.
10. **Update `README.md`** — new Teams section under "API", short callout in "Deploying a new client", new env vars in the config table.
11. **Tag v0.3.0**.

Estimated effort, end to end: **5–7 working days** for a single engineer (backend ~3, frontend ~2.5, polish/tests/docs ~1).

---

## Testing Plan

### Backend

- Unit tests in `teams/service_test.go`:
  - Role hierarchy comparator (`Role.AtLeast`).
  - "Exactly one owner" invariant when creating, transferring, demoting.
  - "Cannot leave personal team."
  - "Admin cannot remove owner / other admin."
- Integration tests against a real Postgres (using the existing `make dev-db` flow) for the invite acceptance flow:
  - Create invite → click token → unauthenticated path → magic-link OTP → joined.
  - Email mismatch returns `403 ErrEmailMismatch`.
  - Expired invite returns `410 ErrInviteExpired`.
  - Revoked invite returns `410 ErrInviteRevoked`.
  - Reusing an accepted token returns `409 ErrInviteAlreadyAccepted`.
- Concurrency: two simultaneous accepts of the same token — exactly one succeeds. Enforced via a `SELECT … FOR UPDATE` inside the accept transaction.

### Frontend

- `npm run typecheck` clean across the new screens and services.
- Manual smoke tests (cross-platform — iOS, Android, Web):
  - Create team → invite member → accept invite from a second browser/account → both see the team in the switcher.
  - Switch team → sessions list updates.
  - Remove member → their JWT keeps working but next request to that team's endpoints returns 403 → app gracefully falls back to a different team.
  - Owner transfer round-trip.
  - Delete team → all sessions vanish; user lands on personal team.

---

## Out of Scope (V1)

- Per-team Anthropic Agent / Environment / system-prompt overrides (V1.1; a clean three-column add to `teams` + a small `agent.Service` lookup).
- Session sharing / handoff across teams or users.
- Granular ACLs (e.g., per-session "can view" lists).
- SSO / SCIM provisioning.
- Audit log UI.
- Stripe / billing / paid seat enforcement (the `max_seats` column is honored softly).
- Usage analytics dashboards (per-team token consumption graphs).
- Public / shareable team join links (V1 is invite-by-email only — the security model is much simpler when invites are email-bound).
- Domain-based auto-join ("anyone with `@acme.com` joins the Acme team").

---

## Decision Points (please confirm or override)

These are the load-bearing assumptions baked into the plan above. If any are wrong, the rest of the plan adjusts cleanly — they're isolated to specific sections.

1. **Personal team auto-create.** Every user gets one on signup, can't leave or delete it. *Alternative: no personal team; users must be invited to use the product.* Confirm or override.
2. **Member visibility.** Members see only their own sessions; admins/owners see all sessions in the team (read-only on others'). *Alternative: all members see all sessions.*
3. **Single owner per team.** Ownership is transferable; admins are the way to share power. *Alternative: multi-owner like Notion.*
4. **Anthropic Agent stays global in V1.** ✅ **Confirmed.** Each client deployment has its own `ANTHROPIC_AGENT_ID` (provisioned by `make managed-agents-provision`); all teams within that deployment share it. Per-team agents land in V1.1 if a customer needs them — additive (3 nullable columns on `teams` + a small lookup in `agent.Service`).
5. **Invite expiry default 7 days.** Configurable via `TEAMS_INVITE_TTL_HOURS`. Confirm the default.
6. **Email-bound invites.** The accepting user's email must match the invite. No "forward to a friend." *Alternative: any signed-in user can accept by token.*
7. **Default max seats per team: 25.** Soft cap, only enforced at invite-send time. Confirm number.
8. **Feature flag `TEAMS_ENABLED` defaults to `true`** in the template. *Alternative: default off, opt-in per client.*
9. **Active team via `X-Team-ID` header.** Cleanly separates team context from JWT. *Alternative: include team in JWT — cleaner request shape but uglier revocation.*
10. **Single Resend integration.** The existing magic-link Resend client also sends invite emails — no separate `RESEND_INVITE_API_KEY`. Confirm.

---

## Open Questions (need answers before commit 1)

- **Mobile deep-link scheme** for invite acceptance: today's `MOBILE_APP_SCHEME` defaults to `agentapp`. Should the invite landing use `agentapp://invites/accept?token=...` or a universal-link / app-link domain? (Universal links require `apple-app-site-association` + `assetlinks.json`, which is out of scope for this template; sticking with the custom scheme keeps parity with the existing magic-link flow.)
- **Personal team naming.** Plan above uses `"<email-local>'s Workspace"`. Alternatives: `"<Name>'s Workspace"`, `"Personal"`, or let the user pick on first run.
- **Invite preview without auth** (`GET /api/invites/:token`) leaks the team name and inviter email to anyone holding the token. Acceptable? (It's a 256-bit token in the URL, so practical risk is low, but it's worth naming.)
