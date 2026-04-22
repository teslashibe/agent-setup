# `agent-setup` — Android Notification Capture + Communication Rollup Scope

**Repo:** `github.com/teslashibe/agent-setup`
**Branch:** `feat/notification-capture`
**Affected packages:** `backend/internal/{notifications,mcp/platforms,config,db/migrations}`, `mobile/{modules/notification-capture,app,services}`
**Mirrors:** existing `agent-setup` conventions (Fiber + pgx + Goose, Expo Router + NativeWind, magic-link auth, MCP tool registry)
**Purpose:** Add passive Android notification capture to the Expo app so the Claude agent can produce daily communication rollups ("what happened" + "what needs to be done") from a user's real communication channels — texts, WhatsApp, email, Zillow, phone calls, etc. — without requiring per-platform API integrations.

---

## Goals

1. A thin **Expo Module** (Kotlin native + JS bridge) captures Android notifications passively via `NotificationListenerService`.
2. Captured notifications are batched and shipped to the Go backend over the existing authenticated API.
3. Notifications are stored in a **TimescaleDB hypertable** (time-series) for efficient time-range queries.
4. The Claude agent accesses notification data through **MCP tools** registered in the standard `platforms.All()` registry — no new wire protocol, no special orchestration.
5. The user configures which apps to monitor via a simple settings screen. Everything else is automatic.
6. **Zero behavior change required** from the end user. They keep using their existing apps. The system observes and summarises.

## Non-Goals (V1)

- iOS notification capture (iOS does not expose a `NotificationListenerService` equivalent without an app extension + heavy restrictions; out of scope for V1).
- macOS companion app (Phase 2 — `NSUserNotificationCenter` / Accessibility-based observer).
- Full message body capture beyond notification content (no SMS `READ_SMS` permission, no email IMAP integration — notification previews are sufficient for rollup quality).
- Real-time streaming to the agent (V1 is batch ingest → on-demand or scheduled rollup).
- Automatic action execution (e.g. "reply to Sarah for me") — V1 is read-only intelligence, not write-back.
- Play Store submission (V1 ships as a sideloaded APK for pilot users; Play Store compliance is a separate effort).

---

## Background — Why Notification Capture

The first pilot customer is a real estate agent in SF. Her pain: communication is fragmented across 6+ channels (SMS, WhatsApp, Zillow messages, email, phone calls, other real estate apps), and at the end of a busy day she can't reconstruct what happened or what needs follow-up. Dropped leads cost commissions.

The notification listener approach solves this without platform-specific integrations. Android's `NotificationListenerService` is a system-level API that receives every notification from every app once the user grants the "Notification Access" permission. A single permission replaces the need for Gmail API, WhatsApp Business API, Zillow API, carrier SMS APIs, etc. The data quality (sender, subject/title, message preview, timestamp, source app) is sufficient for daily rollup and action item extraction.

This is also the thinnest possible wedge into a larger product: once the user trusts the daily rollup, proactive features (missed response alerts, lead scoring, deal stage tracking) are incremental additions to the same data pipeline.

---

## Architecture Decisions

### 1. Expo Module, not a standalone native app

The notification capture lives inside the existing `agent-setup` Expo app as a local Expo Module. This keeps authentication, navigation, and the chat interface unified. The user opens one app, configures capture in settings, and asks the agent for their rollup in the same chat they already use.

**Rejected:** Separate native Android app that ships data to the same backend. Doubles the auth surface, doubles the app install, splits the UX for no benefit.

### 2. NotificationListenerService, not Accessibility Service

Android's `NotificationListenerService` is the narrowest permission that captures notification content from all apps. It requires no root, no special device admin, and no Play Store approval beyond a policy declaration. The user grants "Notification Access" once in system settings.

**Rejected:** Accessibility Service — captures richer screen content but triggers Play Store review flags, requires justification, and is disproportionate for V1's rollup-only use case. Can revisit for V2 "full screen observation" if needed.

**Rejected:** `READ_SMS` + IMAP + per-platform APIs — higher fidelity per channel but requires N integrations, each with its own auth and maintenance burden. The notification path captures all channels with one permission.

### 3. Batch ingest, not real-time streaming

The device batches captured notifications locally and flushes to the backend periodically (default: every 5 minutes) or on app foreground. This is simpler, more battery-friendly, and sufficient for end-of-day rollup.

**Rejected:** WebSocket / SSE push from device to backend. Adds complexity, battery drain, and connectivity edge cases for a feature that doesn't need sub-minute latency.

### 4. Notification data is stored per-user, accessed via MCP tools

Notifications land in a dedicated `notification_events` table, and the Claude agent reads them through MCP tools (`notifications_list`, `notifications_search`, etc.) registered in the standard `platforms.All()` registry. This means:

- The agent discovers notification tools the same way it discovers LinkedIn or Reddit tools — via `tools/list` on the MCP server.
- No special orchestration, no scheduled cron, no separate "rollup service." The user asks "What happened today?" in the chat, and the agent calls the tools.
- The system prompt teaches the agent how to structure a rollup from notification data.

**Rejected:** Dedicated rollup endpoint that runs Claude server-side on a schedule and sends a push notification. Cleaner UX for the "6pm daily summary" use case, but adds a new execution path outside the managed agent system. Can be added later as a thin cron that creates a session and sends a prompt — the MCP tools are the same either way.

### 5. No credentials needed — internal platform binding

Unlike every other platform in `platforms.All()`, notification capture doesn't need user-supplied credentials. The data is pushed by the user's own device, already authenticated via their JWT. The platform binding uses `nullValidator` (like `xviral` or `codegen`) and the `NewClient` constructor receives the database pool + user ID, not a credential blob.

### 6. App allowlist is stored on-device, not on the server

The list of apps the user chooses to monitor (e.g. "Gmail, WhatsApp, Messages, Zillow") is stored locally on the device (AsyncStorage / SecureStore). The backend receives notifications only for allowed apps — filtering happens at the source. This keeps the privacy posture simple: the server never sees notifications from apps the user didn't explicitly opt in.

**Rejected:** Server-side allowlist. Adds a round-trip on every notification and a settings sync problem. The device is the right place for this gate.

---

## Domain Model

### New table

```sql
-- +goose Up

CREATE TABLE notification_events (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    app_package  TEXT NOT NULL,                  -- e.g. "com.google.android.gm"
    app_label    TEXT NOT NULL,                  -- human-readable, e.g. "Gmail"
    title        TEXT,                           -- notification title (sender name, email subject, etc.)
    content      TEXT,                           -- notification body (message preview)
    category     TEXT,                           -- Android notification category if available (msg, email, call, etc.)
    captured_at  TIMESTAMPTZ NOT NULL,           -- device-side timestamp
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

SELECT create_hypertable('notification_events', 'captured_at');

CREATE INDEX idx_notif_user_time ON notification_events (user_id, captured_at DESC);
CREATE INDEX idx_notif_user_app  ON notification_events (user_id, app_package, captured_at DESC);
CREATE INDEX idx_notif_content   ON notification_events USING gin (to_tsvector('english', coalesce(title, '') || ' ' || coalesce(content, '')));

-- +goose Down
DROP TABLE IF EXISTS notification_events;
```

**Why TimescaleDB hypertable:** Every query is time-ranged ("give me today's notifications," "what happened in the last 4 hours"). Hypertable chunking gives efficient range scans and automatic chunk-based retention (e.g. drop data older than 90 days) without manual partition management.

**Why GIN full-text index:** The `notifications_search` MCP tool needs to find notifications by keyword ("find messages from Sarah about the Sunset listing"). Postgres `to_tsvector` + `ts_query` is sufficient for V1; no external search engine needed.

### Go types (`backend/internal/notifications/model.go`)

```go
package notifications

import "time"

type Event struct {
    ID         int64     `json:"id"`
    UserID     string    `json:"user_id"`
    AppPackage string    `json:"app_package"`
    AppLabel   string    `json:"app_label"`
    Title      string    `json:"title,omitempty"`
    Content    string    `json:"content,omitempty"`
    Category   string    `json:"category,omitempty"`
    CapturedAt time.Time `json:"captured_at"`
    CreatedAt  time.Time `json:"created_at"`
}

type BatchInput struct {
    Events []EventInput `json:"events"`
}

type EventInput struct {
    AppPackage string    `json:"app_package"`
    AppLabel   string    `json:"app_label"`
    Title      string    `json:"title,omitempty"`
    Content    string    `json:"content,omitempty"`
    Category   string    `json:"category,omitempty"`
    CapturedAt time.Time `json:"captured_at"`
}
```

---

## API Surface

### Notification ingest

| Method | Path | Auth | Description |
|---|---|---|---|
| `POST` | `/api/notifications/batch` | Bearer JWT | Receive a batch of notification events from the device. Body: `BatchInput`. Deduplicates on `(user_id, app_package, captured_at, title)` using `ON CONFLICT DO NOTHING`. Returns `{ "accepted": <count> }`. |

Rate limit: **60 requests/minute per user** (at 5-minute flush intervals, this is 12x headroom). Configurable via `NOTIFICATIONS_INGEST_RATE_LIMIT`.

### Notification query (REST, for the mobile app's own UI if desired)

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/api/notifications` | Bearer JWT | List recent notifications for the authenticated user. Query params: `since` (RFC3339), `until` (RFC3339), `app` (filter by app_package), `q` (full-text search), `limit` (default 50, max 200). |
| `GET` | `/api/notifications/apps` | Bearer JWT | List distinct apps that have sent notifications, with count per app. Used by the settings screen to show "these apps are being captured." |

These REST endpoints are optional — the agent uses MCP tools, not REST — but they're useful for a future mobile "notification log" screen and for debugging.

---

## MCP Tools

Notification tools are registered in the MCP registry alongside platform tools. The agent discovers them via `tools/list` and calls them like any other tool. Tool names are prefixed `notifications_` following the existing `{platform}_{action}` convention.

### Tool definitions

| Tool name | Input | Output | Description |
|---|---|---|---|
| `notifications_list` | `since?: string` (RFC3339), `until?: string`, `app_package?: string`, `limit?: int` (default 50) | `PageOf[Event]` | List notifications in reverse chronological order, optionally filtered by time range and app. Primary tool for building rollups. |
| `notifications_search` | `query: string`, `since?: string`, `until?: string`, `app_package?: string`, `limit?: int` | `PageOf[Event]` | Full-text search across notification title + content. "Find all messages from Sarah about the Sunset listing." |
| `notifications_threads` | `since?: string`, `until?: string`, `app_package?: string`, `group_by?: string` ("contact" \| "app", default "contact") | `[]Thread` | Group notifications into conversation-like threads by extracting contact names from titles and clustering by app + contact + time proximity. Returns threads with message count, last message time, and preview. |
| `notifications_apps` | `since?: string`, `until?: string` | `[]AppSummary` | List all apps that sent notifications in the time range, with count and latest notification time per app. Meta-tool for the agent to understand what data is available. |
| `notifications_pending_actions` | `since?: string`, `until?: string` | `[]ActionItem` | Heuristic extraction of likely action items: unanswered messages (notification with no outbound reply within N hours), questions detected in content, time-sensitive keywords ("deadline," "offer expires," "showing at"). Returns ranked items with source notification reference. |

### Thread and ActionItem types

```go
type Thread struct {
    Contact      string  `json:"contact"`
    AppLabel     string  `json:"app_label"`
    AppPackage   string  `json:"app_package"`
    MessageCount int     `json:"message_count"`
    FirstAt      string  `json:"first_at"`
    LastAt       string  `json:"last_at"`
    Preview      string  `json:"preview"`
}

type AppSummary struct {
    AppPackage string `json:"app_package"`
    AppLabel   string `json:"app_label"`
    Count      int    `json:"count"`
    LastAt     string `json:"last_at"`
}

type ActionItem struct {
    Priority   string `json:"priority"`        // "high", "medium", "low"
    Summary    string `json:"summary"`
    Contact    string `json:"contact,omitempty"`
    AppLabel   string `json:"app_label"`
    CapturedAt string `json:"captured_at"`
    Reason     string `json:"reason"`           // "unanswered", "question", "time_sensitive", "follow_up"
}
```

### Provider pattern

The notification MCP tools live in `backend/internal/notifications/mcp/` (not a separate `*-go` repo, since there's no external API client to wrap). The `Provider` implements `mcptool.Provider` with `Platform() == "notifications"`.

```go
// backend/internal/notifications/mcp/mcp.go
package notificationsmcp

import "github.com/teslashibe/mcptool"

type Provider struct{}

func (Provider) Platform() string { return "notifications" }

func (Provider) Tools() []mcptool.Tool {
    return append(append(append(append(
        listTools,
        searchTools...),
        threadTools...),
        appTools...),
        actionTools...,
    )
}
```

### Client shape

Unlike platform bindings where `NewClient` builds an API client from credentials, the notification "client" is a thin wrapper around the store that's scoped to the current user. User ID is resolved from the MCP request context (JWT in path), same as other platforms.

```go
// backend/internal/notifications/mcp/client.go
type Client struct {
    Store  *notifications.Store
    UserID string
}
```

The `NewClient` in `platforms.go` constructs this from the request context:

```go
func Notifications(pool *pgxpool.Pool) Plugin {
    return Plugin{
        Binding: mcp.PlatformBinding{
            Provider: notificationsmcp.Provider{},
            NewClient: func(ctx context.Context, _ json.RawMessage) (any, error) {
                userID := mcp.UserIDFromContext(ctx)
                return &notificationsmcp.Client{
                    Store:  notifications.NewStore(pool),
                    UserID: userID,
                }, nil
            },
        },
        Validator: nullValidator{platform: "notifications"},
    }
}
```

**Note:** This requires threading `userID` through to `NewClient` via context. Currently `Server.CallTool` already has `userID` from the JWT resolution. Verify that `NewClient` receives a context carrying the user ID — if not, the context propagation from `transport.go → server.go → NewClient` needs a small plumbing addition. Check `mcp.Server.CallTool` to confirm `userID` is available on the context passed to `NewClient`.

---

## Android Notification Capture Module

### Expo Module structure

```
mobile/modules/notification-capture/
├── expo-module.config.json
├── src/
│   ├── index.ts                              # re-export
│   └── NotificationCaptureModule.ts          # JS bridge: start/stop, getApps, setAllowlist, flush
└── android/
    ├── build.gradle.kts                      # Expo Module Gradle config
    └── src/main/
        ├── AndroidManifest.xml               # NotificationListenerService declaration
        └── java/com/teslashibe/notificationcapture/
            ├── NotificationCaptureModule.kt  # Expo Module definition — exposes JS API
            ├── NotificationCaptureService.kt # NotificationListenerService impl — captures notifications
            └── NotificationStore.kt          # Local SQLite buffer for batching before upload
```

### expo-module.config.json

```json
{
  "platforms": ["android"],
  "android": {
    "modules": ["com.teslashibe.notificationcapture.NotificationCaptureModule"]
  }
}
```

### Expo app config plugin addition

`mobile/app.config.ts` — add the local module to `plugins`:

```ts
plugins: [
  "expo-router",
  "expo-web-browser",
  "expo-secure-store",
  "./modules/notification-capture",
],
```

### Android NotificationListenerService

The core Kotlin class:

```kotlin
// NotificationCaptureService.kt
class NotificationCaptureService : NotificationListenerService() {

    override fun onNotificationPosted(sbn: StatusBarNotification) {
        val pkg = sbn.packageName
        if (!isAllowlisted(pkg)) return
        if (sbn.isOngoing) return    // skip persistent/ongoing notifications

        val extras = sbn.notification.extras
        val event = NotificationEvent(
            appPackage = pkg,
            appLabel   = getAppLabel(pkg),
            title      = extras.getCharSequence(Notification.EXTRA_TITLE)?.toString(),
            content    = extras.getCharSequence(Notification.EXTRA_TEXT)?.toString(),
            category   = sbn.notification.category,
            capturedAt = Instant.ofEpochMilli(sbn.postTime).toString(),
        )

        NotificationStore.insert(event)
    }
}
```

Key behaviors:

- **Allowlist filtering** happens immediately in `onNotificationPosted` — non-allowlisted apps are discarded before storage.
- **Ongoing notifications** (music players, navigation, etc.) are skipped.
- **Deduplication:** StatusBarNotification has a `key` field. The local store deduplicates on `(key, postTime)` to handle notification updates without creating duplicate events.
- **Local buffer:** `NotificationStore` is a simple SQLite table on device that accumulates events between flushes.

### JS bridge API

```ts
// NotificationCaptureModule.ts
import { NativeModule, requireNativeModule } from 'expo-modules-core';

interface NotificationEvent {
  app_package: string;
  app_label: string;
  title: string | null;
  content: string | null;
  category: string | null;
  captured_at: string;  // ISO8601
}

interface NotificationCaptureModule extends NativeModule {
  isServiceEnabled(): boolean;
  openNotificationAccessSettings(): void;
  getInstalledApps(): Promise<Array<{ package: string; label: string }>>;
  setAllowlist(packages: string[]): void;
  getAllowlist(): string[];
  getPendingCount(): number;
  flush(): Promise<NotificationEvent[]>;  // returns events and clears local buffer
  setEnabled(enabled: boolean): void;
}

export default requireNativeModule<NotificationCaptureModule>('NotificationCapture');
```

### Flush and upload flow

The mobile app runs a flush loop:

```ts
// services/notifications.ts
const FLUSH_INTERVAL_MS = 5 * 60 * 1000; // 5 minutes

async function flushNotifications(): Promise<void> {
    const events = await NotificationCaptureModule.flush();
    if (events.length === 0) return;
    await api.post('/api/notifications/batch', { events });
}
```

This runs:
- On a `setInterval` when the app is in the foreground.
- On `AppState` change to "active" (when the app is foregrounded).
- Before the user opens the chat screen (so the agent has the latest data).

**No background task scheduler in V1.** Expo's managed workflow doesn't support arbitrary background tasks without `expo-task-manager` + `expo-background-fetch`, which are coarse-grained (minimum 15-minute intervals, OS-controlled). For V1, foreground-only flush is sufficient — the user opens the app to ask for their rollup, which triggers a flush. Background upload is a V2 enhancement.

---

## Mobile UI

### New screen: `app/(app)/capture.tsx`

A settings screen for notification capture configuration. Accessible from the Settings tab.

**Layout:**

1. **Status banner** — shows whether Notification Access is granted. If not, a prominent button: "Enable Notification Access" → calls `NotificationCaptureModule.openNotificationAccessSettings()` which opens the Android system settings page.

2. **Master toggle** — "Capture notifications" on/off. Calls `setEnabled(boolean)`.

3. **App selector** — List of installed apps (from `getInstalledApps()`), each with a toggle. Defaults to all OFF; user opts in per app. Common communication apps are surfaced at the top with suggested labels:
   - Messages (com.google.android.apps.messaging)
   - WhatsApp (com.whatsapp)
   - Gmail (com.google.android.gm)
   - Outlook (com.microsoft.office.outlook)
   - Phone (com.google.android.dialer)
   - Zillow (com.zillow.android.zillowmap)

   The rest appear alphabetically below. User's selections are persisted to AsyncStorage.

4. **Stats footer** — "X notifications captured today · Last sync: Y minutes ago"

### Updated screen: `app/(app)/settings.tsx`

Add a row: "Notification Capture" with a chevron → navigates to `/(app)/capture`.

### Updated layout: `app/(app)/_layout.tsx`

Add `capture` as a hidden route (like `chat` and `platforms` — navigable but not a visible tab):

```ts
<Tabs.Screen name="capture" options={{ href: null }} />
```

---

## Agent System Prompt Addition

The provisioner's system prompt (`provision.go` `defaultSystemPrompt`) is extended to teach the agent about notification tools:

```
You also have access to the user's captured device notifications via tools prefixed
with notifications_. These contain communication activity from the user's phone — texts,
emails, WhatsApp messages, app notifications, missed calls, etc.

When the user asks for a "rollup," "summary," "what happened today," or "what do I need
to do," use these tools to build a structured response:

1. Call notifications_apps to understand which channels had activity.
2. Call notifications_threads to group activity by contact and conversation.
3. Call notifications_pending_actions to surface items that may need follow-up.

Structure your rollup as:
- **What happened** — organised by contact or conversation, most important first.
- **What needs attention** — action items ranked by urgency, with the source message quoted.

Keep summaries concise. Quote specific message content when it adds clarity (e.g. "Sarah
asked: 'Can you send the comps for 742 Evergreen?'"). Flag time-sensitive items prominently.
```

---

## Backend Implementation Plan

### New packages

```
backend/internal/
├── notifications/
│   ├── model.go          # Event, BatchInput, EventInput, Thread, AppSummary, ActionItem
│   ├── store.go          # pgx queries: InsertBatch, List, Search, GroupThreads, ListApps
│   ├── service.go        # Business logic: dedup, full-text query building, action extraction
│   ├── handler.go        # Fiber: POST /api/notifications/batch, GET /api/notifications, GET .../apps
│   └── mcp/
│       ├── mcp.go        # Provider implementation
│       ├── client.go     # Client (store wrapper scoped to user)
│       ├── list.go       # notifications_list tool
│       ├── search.go     # notifications_search tool
│       ├── threads.go    # notifications_threads tool
│       ├── apps.go       # notifications_apps tool
│       └── actions.go    # notifications_pending_actions tool
```

### Store API

```go
type Store struct {
    pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store

func (s *Store) InsertBatch(ctx context.Context, userID string, events []EventInput) (int, error)
func (s *Store) List(ctx context.Context, userID string, opts ListOpts) ([]Event, error)
func (s *Store) Search(ctx context.Context, userID, query string, opts ListOpts) ([]Event, error)
func (s *Store) GroupThreads(ctx context.Context, userID string, opts ThreadOpts) ([]Thread, error)
func (s *Store) ListApps(ctx context.Context, userID string, since, until time.Time) ([]AppSummary, error)
func (s *Store) PendingActions(ctx context.Context, userID string, opts ActionOpts) ([]ActionItem, error)

type ListOpts struct {
    Since      *time.Time
    Until      *time.Time
    AppPackage string
    Limit      int
}

type ThreadOpts struct {
    Since      *time.Time
    Until      *time.Time
    AppPackage string
    GroupBy    string // "contact" | "app"
}

type ActionOpts struct {
    Since *time.Time
    Until *time.Time
}
```

### Thread grouping logic (`store.go`)

Threads are grouped by extracting the contact name from the notification `title` field (most messaging apps put the sender name in the title) and clustering by `(app_package, title, time_bucket)`. The SQL:

```sql
SELECT
    app_package,
    app_label,
    title AS contact,
    count(*) AS message_count,
    min(captured_at) AS first_at,
    max(captured_at) AS last_at,
    (array_agg(content ORDER BY captured_at DESC))[1] AS preview
FROM notification_events
WHERE user_id = $1
  AND captured_at >= $2
  AND captured_at <= $3
GROUP BY app_package, app_label, title
ORDER BY max(captured_at) DESC;
```

This is a rough heuristic — "title" in most messaging apps is the contact name, but in email it's the subject line. The agent's natural language processing smooths over these inconsistencies when producing the rollup.

### Action item extraction (`store.go` / `service.go`)

V1 action extraction is heuristic, not LLM-based (keeps it fast and free of extra API calls):

1. **Unanswered messages:** Notifications from messaging apps where no outbound notification from the same app followed within a configurable window (default 2 hours). Heuristic: if the user's own outbound message would trigger a "message sent" notification, its absence implies no reply.
2. **Questions:** Content containing `?` from known communication apps.
3. **Time-sensitive keywords:** Content matching patterns like `deadline`, `expires`, `by tomorrow`, `showing at`, `offer`, `closing`, `inspection`, `ASAP`, `urgent`.
4. **Missed calls:** Notifications from the phone dialer app with category `call` or title matching "Missed call."

Priority ranking: missed calls and time-sensitive keywords → unanswered questions → other unanswered messages.

---

## Platform Registration

### `platforms.go` addition

```go
func Notifications(pool *pgxpool.Pool) Plugin {
    return Plugin{
        Binding: mcp.PlatformBinding{
            Provider: notificationsmcp.Provider{},
            NewClient: func(ctx context.Context, _ json.RawMessage) (any, error) {
                userID := mcp.UserIDFromContext(ctx)
                if userID == "" {
                    return nil, errors.New("notifications: user ID not available in context")
                }
                return &notificationsmcp.Client{
                    Store:  notifications.NewStore(pool),
                    UserID: userID,
                }, nil
            },
        },
        Validator: nullValidator{platform: "notifications"},
    }
}
```

### `All()` addition

```go
func All(pool *pgxpool.Pool) []Plugin {
    return []Plugin{
        LinkedIn(),
        X(),
        // ... existing platforms ...
        Codegen(),
        Notifications(pool),  // new — no credentials, uses DB directly
    }
}
```

**No breaking change to `All()`.** `Notifications(pool)` is **not** added to `All()`. Instead, it is conditionally appended in `cmd/server/main.go` only when `NOTIFICATIONS_ENABLED=true`:

```go
plugins := platforms.All()
if cfg.NotificationsEnabled {
    plugins = append(plugins, platforms.Notifications(pool))
}
```

This keeps `All()` zero-argument (no signature change), keeps the notification dependency out of forks that don't need it, and follows the same pattern as the `TEAMS_ENABLED` gate on team routes. Forks that never set the flag never touch this code path.

### Context propagation for `userID`

`mcp.Server.CallTool` currently receives `userID` as a parameter and uses it for credential lookup. The `NewClient` function receives `ctx` and `raw` (credential JSON). To make `userID` available in `NewClient`'s context, add it to the context in `Server.CallTool` before calling `NewClient`:

```go
// In server.go CallTool, before NewClient:
ctx = withUserID(ctx, userID)
```

This is a one-line addition. Define `withUserID` / `UserIDFromContext` as context key helpers in the `mcp` package.

---

## Configuration

New env vars (additive to `backend/.env.example`):

```bash
# Notification Capture
NOTIFICATIONS_ENABLED=true                          # gates /api/notifications/* routes and MCP tools
NOTIFICATIONS_INGEST_RATE_LIMIT=60                  # max batch uploads per minute per user
NOTIFICATIONS_RETENTION_DAYS=90                     # TimescaleDB chunk retention (0 = keep forever)
NOTIFICATIONS_ACTION_REPLY_WINDOW_HOURS=2           # how long before an unreplied message is flagged
NOTIFICATIONS_DEFAULT_PAGE_SIZE=50                  # default limit for list/search queries
NOTIFICATIONS_MAX_PAGE_SIZE=200                     # hard cap on limit
```

When `NOTIFICATIONS_ENABLED=false` (the default):
- `/api/notifications/*` routes are not mounted (not 404 — the routes simply don't exist).
- Notification MCP tools are not registered — the agent never sees them in `tools/list`.
- The mobile capture settings screen shows "Notification capture is not available for this deployment."
- The `Notifications(pool)` plugin is not appended to the platform list.

**Migration note:** The `00003_notification_events.sql` migration ships in the template and runs on all forks (Goose runs all migrations in order). The table is created but remains empty and adds zero overhead for forks that don't enable the feature. This is the same pattern as `teams` and `team_invites` tables which exist in every fork regardless of `TEAMS_ENABLED`. If a fork wants to explicitly skip it, they can delete the migration file before first deploy — but there's no cost to leaving it.

`backend/internal/config/config.go` adds these as typed fields on `Config`. `NOTIFICATIONS_ENABLED` defaults to `false` in `Load()` so existing forks are unaffected.

---

## Mobile Implementation Plan

### New service module: `services/notifications.ts`

```ts
export interface NotificationEvent {
    app_package: string;
    app_label: string;
    title: string | null;
    content: string | null;
    category: string | null;
    captured_at: string;
}

export interface AppSummary {
    app_package: string;
    app_label: string;
    count: number;
    last_at: string;
}

// Ingest
export async function uploadBatch(events: NotificationEvent[]): Promise<{ accepted: number }>;

// Query (optional — agent uses MCP tools, but useful for a future log screen)
export async function listNotifications(opts?: {
    since?: string;
    until?: string;
    app?: string;
    q?: string;
    limit?: number;
}): Promise<NotificationEvent[]>;

export async function listCapturedApps(): Promise<AppSummary[]>;
```

### Flush manager: `services/notificationSync.ts`

```ts
// Manages the flush interval and app lifecycle hooks.
// Started by the capture settings screen or on app boot if capture is enabled.

export function startSync(): void;   // begins setInterval + AppState listener
export function stopSync(): void;    // clears interval
export function flushNow(): Promise<void>;  // immediate flush (called before opening chat)
```

### New provider: `providers/NotificationCaptureProvider.tsx`

Wraps the native module state and sync manager. Sits inside `AuthSessionProvider` (needs JWT for uploads). Exposes:

```ts
type NotificationCaptureContextValue = {
    isAvailable: boolean;     // Android only
    isEnabled: boolean;       // master toggle
    hasPermission: boolean;   // Notification Access granted
    allowlist: string[];      // app packages being monitored
    pendingCount: number;     // events in local buffer
    lastSyncAt: Date | null;
    setEnabled: (enabled: boolean) => void;
    setAllowlist: (packages: string[]) => void;
    openPermissionSettings: () => void;
    flushNow: () => Promise<void>;
};
```

On iOS / web, `isAvailable` is `false` and all other fields are no-ops.

---

## Privacy and Security

### Data in transit
All notification data is sent over HTTPS to the backend, authenticated with the user's JWT. Same transport security as every other API call.

### Data at rest
Notification content is stored in plaintext in Postgres (not encrypted at rest beyond disk-level encryption). This is consistent with how agent session messages are stored today. If a deployment requires field-level encryption, the same `CREDENTIALS_ENCRYPTION_KEY` + `crypto.go` pattern from the credentials package can be applied — but this is out of scope for V1.

### Data retention
`NOTIFICATIONS_RETENTION_DAYS` controls automatic cleanup via TimescaleDB's `drop_chunks()` policy:

```sql
SELECT add_retention_policy('notification_events', INTERVAL '90 days');
```

This is set up in the migration or as a post-migration step.

### User control
- The user explicitly grants Notification Access in Android system settings.
- The user explicitly selects which apps to monitor.
- The user can disable capture at any time via the in-app toggle.
- The user can revoke Notification Access in system settings, which immediately stops all capture.

### Pilot deployment
V1 is a sideloaded APK for a single pilot user. No Play Store review, no broad distribution. The privacy posture is a direct trust relationship with a known user.

---

## Implementation Order

Suggested commit/PR sequence — each item independently testable:

1. **Schema + migration** (`00003_notification_events.sql`) + Goose smoke test.
2. **`internal/notifications` package** — model, store (`InsertBatch`, `List`, `Search`, `GroupThreads`, `ListApps`, `PendingActions`), service. Unit tests for dedup, full-text search, thread grouping, action extraction heuristics.
3. **`internal/notifications/handler.go`** — `POST /api/notifications/batch`, `GET /api/notifications`, `GET /api/notifications/apps`. Wiring in `cmd/server/main.go` under the auth middleware.
4. **Context propagation** — Add `UserIDFromContext` / `withUserID` to `internal/mcp` package. Update `Server.CallTool` to inject user ID into context before `NewClient`.
5. **`internal/notifications/mcp/`** — Provider, Client, all five tool definitions (`list`, `search`, `threads`, `apps`, `pending_actions`). Follow `mcptool.Define[*Client, Input]` pattern.
6. **Platform registration** — Add `Notifications(pool)` to `platforms.go`, update `All()` signature, update call site in `main.go`.
7. **System prompt update** — Add notification-aware instructions to `provision.go` `defaultSystemPrompt`.
8. **Expo Module: native Kotlin** — `NotificationCaptureService.kt`, `NotificationCaptureModule.kt`, `NotificationStore.kt`, `AndroidManifest.xml`, `build.gradle.kts`.
9. **Expo Module: JS bridge** — `NotificationCaptureModule.ts`, `index.ts`, `expo-module.config.json`.
10. **Mobile services** — `services/notifications.ts`, `services/notificationSync.ts`.
11. **Mobile provider** — `providers/NotificationCaptureProvider.tsx`, wire into `app/_layout.tsx`.
12. **Mobile UI** — `app/(app)/capture.tsx` settings screen, settings row in `app/(app)/settings.tsx`.
13. **App config** — Update `app.config.ts` plugins array.
14. **Integration test** — End-to-end: mock notification → local store → flush → backend ingest → MCP tool query → verify rollup data.
15. **README update** — New "Notification Capture" section.

Estimated effort: **5–7 working days** (backend 2–3, native module 2, mobile UI/integration 1–2).

---

## Testing Plan

### Backend

- **Store unit tests:**
  - `InsertBatch` deduplication: same `(user_id, app_package, captured_at, title)` inserted twice → only one row.
  - `List` with time range filters.
  - `Search` full-text: "Sarah Sunset listing" matches a notification with those words in title/content.
  - `GroupThreads` clusters correctly by contact.
  - `PendingActions` extracts questions, missed calls, time-sensitive keywords.

- **Handler tests:**
  - Batch upload with valid JWT → 200 + accepted count.
  - Batch upload without auth → 401.
  - Batch upload with empty events array → 200, accepted: 0.
  - Rate limiting: exceed 60 requests/minute → 429.

- **MCP tool tests:**
  - Register notification tools in a test registry → `tools/list` includes them.
  - `tools/call` `notifications_list` → returns stored events.
  - `tools/call` `notifications_search` with query → returns matching events.

### Mobile

- `npm run typecheck` clean across new modules and screens.
- Manual smoke test on Android (physical device or emulator):
  - Install APK → grant Notification Access → select apps → send a test SMS → verify notification captured → open chat → ask "what happened?" → agent produces rollup.
  - Disable capture → send SMS → verify no new notifications captured.
  - Revoke Notification Access in system settings → verify app shows "permission not granted" banner.

### Integration

- Full loop: Android emulator with notification injection (`adb shell service call notification`) → flush → backend query → MCP tool response verification.

---

## Out of Scope (V1)

- iOS notification capture (no equivalent API without app extension + restricted background modes).
- macOS companion app.
- Full SMS body capture (requires `READ_SMS` permission + additional privacy considerations).
- Email IMAP integration (notification previews are sufficient for V1 rollup quality).
- Background sync via `expo-background-fetch` (foreground-only flush for V1).
- Scheduled daily rollup push notification (the user asks the agent; there's no autonomous trigger).
- Write-back actions ("reply to Sarah for me").
- Play Store submission and policy compliance review.
- Field-level encryption of notification content at rest.
- Multi-device notification deduplication (V1 assumes one device per user).
- Web/desktop notification capture (browser Notification API is too restricted).

---

## Decision Points (confirm or override)

1. **Expo Module vs bare native app.** Plan uses a local Expo Module inside the existing app. *Alternative: separate native Android app with its own auth.* Confirm.
2. **NotificationListenerService only (no AccessibilityService, no READ_SMS).** Notification previews are sufficient for rollup. *Alternative: add READ_SMS for full message bodies.* Confirm.
3. **Batch ingest with foreground-only flush.** No background sync in V1. *Alternative: add `expo-background-fetch` for 15-minute background flushes.* Confirm.
4. **App allowlist stored on-device.** Server never sees notifications from non-allowlisted apps. *Alternative: server-side allowlist with sync.* Confirm.
5. **MCP tools for agent access (no dedicated rollup endpoint/cron).** The user triggers rollups by asking the agent. *Alternative: scheduled cron that auto-generates a daily rollup and sends a push notification.* Confirm.
6. **Heuristic action extraction (no LLM calls in the tool).** `pending_actions` uses keyword/pattern matching, not Claude. The agent's own reasoning handles nuance. *Alternative: call Claude inside the tool for higher-quality extraction.* Confirm.
7. **Conditional registration outside `All()`.** `Notifications(pool)` is appended in `main.go` only when `NOTIFICATIONS_ENABLED=true`, not added to `All()`. *Alternative: add to `All()` unconditionally and let `nullValidator` handle the rest.* Confirm.
8. **90-day retention default.** Configurable via `NOTIFICATIONS_RETENTION_DAYS`. *Alternative: no auto-retention (keep everything).* Confirm.
9. **No credential requirement.** Notification tools use `nullValidator` — no entry in the Platform Connections UI. *Alternative: show a "Device" entry in connections with an on/off toggle.* Confirm.
10. **Single device per user.** No dedup logic for the same notification arriving from multiple devices. *Alternative: dedup on `(user_id, app_package, captured_at, title, content)` which handles multi-device naturally.* Confirm.

---

## Open Questions (need answers before commit 1)

- **Notification content truncation.** Android truncates notification text in `EXTRA_TEXT` (typically 1-2 lines for messaging apps). For longer messages, `EXTRA_BIG_TEXT` or `EXTRA_TEXT_LINES` may contain more. Should the service attempt to read expanded notification styles (`BigTextStyle`, `InboxStyle`, `MessagingStyle`)? This is more code but significantly better content quality for email and group chats.
- **Outbound message detection.** The `pending_actions` tool flags "unanswered" messages, but detecting outbound replies via notifications is unreliable (not all apps generate "message sent" notifications). Should we accept this limitation in V1, or add `READ_SMS` specifically for SMS outbound detection?
- **Team scoping.** Should notification data be team-scoped (like sessions) or user-scoped (always private)? Recommendation: user-scoped only — notification data is inherently personal and should not be visible to team admins.
- **Pilot user onboarding.** Should we build a one-time setup wizard (grant permission → select apps → confirm) or just the settings screen? The wizard is better UX for a non-technical user but more UI work.
