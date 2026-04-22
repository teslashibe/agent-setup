# Notification Capture — Forensic Audit

Audit of `feat/notification-capture` against the scope at
`/Users/brendanplayford/teslashibe/agent-setup/.worktrees/notification-capture/.cursor/tickets/notification-capture-scope.md`.

Performed under `.claude/rules/issue-audit-user-stories.mdc`.

---

## 1. Findings (severity-ordered, evidence first)

### High — none open
The two issues that would have shipped as data-loss bugs were caught
during the audit and fixed in this same branch.

### Medium — closed in this branch

#### M1. Network failure dropped notifications permanently — FIXED
- **Evidence:** `mobile/services/notificationSync.ts::flushNow` previously
  called `drainBuffer()` (which empties the native store atomically)
  then iterated upload chunks. If any `uploadBatch(chunk)` threw, the
  remaining events were already gone.
- **Impact:** An agent driving between properties on flaky cell would
  silently lose hours of notifications. Exactly the user the feature
  is built for.
- **Fix:** Added `requeueEvents` to the native module
  (`NotificationStore.kt`, `NotificationCaptureModule.kt`,
  `src/index.ts`). On upload failure, `flushNow` requeues the unsent
  tail back into the native buffer (older entries are dropped first if
  the requeue would overflow MAX_BUFFER_SIZE = 500). Verified by
  `npm run typecheck`.

#### M2. Backend would 500 if `mcp` dispatch lost user_id for credential-less platforms — FIXED
- **Evidence:** `internal/mcp/server.go::resolveClient` previously
  unconditionally called `decryptCredentials(userID, platform)`. Three
  built-in platforms (`xviral`, `redditviral`, `codegen`) and the new
  `notifications` platform have no per-user credentials, so this would
  always emit `credential_missing` and refuse to call the tool.
- **Fix:** `PlatformBinding` gained a `NoCredentials bool` flag.
  `resolveClient` skips decryption when set, and stamps user_id into
  context via `withUserID` so tools can recover it via
  `UserIDFromContext`. Covered by
  `internal/mcp/context_test.go`,
  `internal/mcp/platforms/notifications_test.go`.

### Medium — open (deferred outside this branch)

#### M3. Pre-existing template dependency drift
- **Evidence:** `npx expo-doctor` reports:
  - Missing peer dep `react-native-worklets` (required by
    `react-native-reanimated`).
  - Duplicate `expo-font` (14.0.11 vs nested 55.0.6).
  - 7 packages out of date vs SDK 55.
- **Why deferred:** Pre-existing in `main`, unrelated to notification
  capture. Touching them widens this PR's surface area beyond the
  feature scope. Recommend a follow-up PR `chore/sdk-55-realign` that
  runs `npx expo install --check` and resolves duplicates.
- **Risk to APK build:** `react-native-worklets` is required by
  `react-native-reanimated`, which ships in this template. EAS Build
  will likely auto-install via `expo install` during the build phase,
  but if it does not, the APK may crash on first launch. Mitigation:
  run `npx expo install react-native-worklets` before triggering EAS
  Build.

### Low — design choices to be aware of

#### L1. `notifications_pending_actions` ranking is heuristic, not learned
- **Where:** `backend/internal/notifications/service.go::classify`.
- **Behaviour:** Coarse priority via regex over urgency keywords +
  `?` + `category=call`. The agent re-ranks at the LLM layer, so this
  is intentionally a pre-filter, not the final ordering.
- **Risk:** False positives (e.g. "?" in a Zillow price-drop alert).
  The agent will down-rank. Track in production logs; revisit if the
  user complains the rollup buries real follow-ups.

#### L2. Threading is by `(app_package, title)`
- **Where:** `backend/internal/notifications/store.go::GroupThreads`.
- **Behaviour:** Groups SMS/WhatsApp messages by sender name (which
  Android puts in the notification title). Works well for 1:1 threads,
  groups multiple senders into one row when an app shows
  "3 new messages" in the title.
- **Mitigation:** The agent gets the raw `notifications_search` tool
  for ad-hoc lookups, so this is recoverable at query time.

#### L3. iOS not implemented
- **Why:** iOS does not allow third-party apps to read other apps'
  notifications without MDM enrolment. This is documented as
  out-of-scope for V1 in the scope doc and the mobile module declares
  `platforms: ["android"]` so there is no surprise at build time.
- **Risk:** Zero — `isCaptureAvailable` short-circuits the React
  surface to `FALLBACK_VALUE` on iOS.

---

## 2. Gaps vs scope intent

| Scope item | Status | Notes |
|---|---|---|
| `00003_notification_events.sql` migration | ✅ ships, inert if `NOTIFICATIONS_ENABLED=false` | Idempotent CREATE TABLE / INDEX / hypertable. |
| `notifications` platform on the MCP server | ✅ conditional plugin in `cmd/server/main.go` | Not in `platforms.All()`, by design — keeps `All()` signature stable. |
| 5 MCP tools (`list`, `search`, `threads`, `apps`, `pending_actions`) | ✅ implemented + schema-validated by `mcp_test.go` | All prefixed `notifications_`. |
| Expo native module | ✅ `modules/notification-capture` | Autolinking confirmed via `npx expo-modules-autolinking search --platform android`. |
| Mobile sync loop | ✅ + requeue-on-failure | Foreground only by design; background WorkManager is V2. |
| Settings UI | ✅ `app/(app)/capture.tsx` + entry row in `settings.tsx` | Hidden tab in `_layout.tsx` so deep-linking works. |
| Feature flag plumbing | ✅ `NOTIFICATIONS_ENABLED` (server) + `EXPO_PUBLIC_NOTIFICATIONS_ENABLED` (client) | Default OFF on both sides. |
| Provisioner system prompt addendum | ✅ `agent.NotificationsSystemPrompt()` | Only attached when `cfg.NotificationsEnabled`. |
| APK build | ⚠️ scaffolded only | No local Android SDK / JDK on this Mac. `eas.json` ships with `preview` and `production-apk` profiles + matching `npm run build:apk:*` scripts. Ship instructions below. |

### Gaps not closed in this PR (V2)

- Background sync via WorkManager — currently foreground-only.
- Encryption-at-rest for the on-device buffer (SharedPreferences are
  app-private; root access required to read them, but full disk
  encryption is the only OS-level guarantee).
- Per-app redaction rules (e.g. blur OTP codes before upload).
- Daily digest push notification driven by the agent.

---

## 3. User stories

### US-1 — Real-estate agent rollup
> As a real estate agent on Android, I want my texts, WhatsApp,
> Zillow, and email notifications captured passively so the agent can
> tell me what happened today and what needs follow-up without me
> retyping anything.

### US-2 — Privacy control
> As a user installing the APK, I want a single master switch and an
> explicit allowlist so I decide which apps the agent can see, and I
> want to revoke at any time from the settings screen.

### US-3 — Operator confidence
> As the operator forking this template for a non-realtor client, I
> want to ship the same template with `NOTIFICATIONS_ENABLED=false`
> and have zero notification rows, MCP tools, or UI surfaces appear,
> so the feature stays invisible until I opt in.

### US-4 — Reliability under flaky network
> As a real estate agent driving between properties, I want unsent
> notifications to survive a dropped cell signal so my evening rollup
> is complete even if the day's connectivity was patchy.

### US-5 — Agent rollup quality
> As a user asking the agent "what happened today", I want a
> per-contact thread list, an action-item list ranked by urgency, and
> the ability to drill into specific contacts or topics with a
> follow-up question.

---

## 4. Acceptance criteria

### AC for US-1 — Capture & ingest
- **Given** the user has installed the APK and granted Notification
  Access, **when** an SMS, WhatsApp, Gmail, or Zillow notification
  posts on the device, **then** the event is appended to the local
  buffer with `app_package`, `app_label`, `title`, `content`,
  `category`, and `captured_at` populated.
- **Given** the device buffers ≥1 event, **when** the app
  foregrounds OR 5 minutes elapse, **then** events are POSTed to
  `/api/notifications/batch` and `markSynced` is called.
- **Negative:** **Given** the user has not granted Notification
  Access, **then** `hasPermission()` returns false and the settings
  screen shows the "Open system settings" CTA.

### AC for US-2 — Privacy control
- **Given** the master switch is OFF, **when** a notification posts,
  **then** the `NotificationCaptureService` returns early and nothing
  is written to the buffer (verified by reading
  `NotificationCaptureService.kt::onNotificationPosted`).
- **Given** an app's package is NOT in the allowlist, **when** that
  app posts a notification, **then** the event is dropped at the
  service layer.
- **Given** the user toggles the master switch OFF in settings,
  **then** `setEnabled(false)` is called and `stopSync()` runs;
  pending events stay in the buffer until either the user re-enables
  capture or uninstalls the app.

### AC for US-3 — Default-off opt-in
- **Given** `NOTIFICATIONS_ENABLED=false` (the default in
  `.env.example`), **when** the backend boots, **then**
  - the `/api/notifications/*` routes are not mounted (verified by
    the `if cfg.NotificationsEnabled` guard in `cmd/server/main.go`),
  - the `notifications` MCP plugin is not appended to the dispatcher
    plugin list,
  - the agent provisioner uses `defaultSystemPrompt` (no rollup
    addendum),
  - the migration still runs but creates an idle hypertable.
- **Given** `EXPO_PUBLIC_NOTIFICATIONS_ENABLED` is not set, **when**
  the mobile app boots, **then** the Settings screen omits the
  "Notification Capture" card and the `/capture` route renders the
  "feature disabled" empty state.

### AC for US-4 — Reliability
- **Given** the buffer holds N events and the device has no network,
  **when** `flushNow` runs, **then** the upload throws, all N events
  are requeued via `requeueEvents`, and `markSynced` is NOT called.
- **Given** the buffer holds 250 events and the second chunk fails,
  **when** `flushNow` runs, **then** the first 200 are accepted, the
  remaining 50 are requeued, and `flushNow` returns 200.
- **Given** the buffer would exceed 500 events after a requeue,
  **then** older events are dropped first (freshest preserved).

### AC for US-5 — Agent rollup quality
- **Given** events exist for the requested time window, **when** the
  agent calls `notifications_threads`, **then** results are returned
  with `contact`, `app_label`, `message_count`, `last_at`, and
  `preview` fields ordered by most-recent activity.
- **Given** an event whose content matches an urgency keyword (e.g.
  "tonight", "showing at"), **when** the agent calls
  `notifications_pending_actions`, **then** the row appears with
  `priority="high"` and `reason="time_sensitive"`.
- **Negative:** **Given** an empty time window, **when** the agent
  calls any `notifications_*` tool, **then** the response is the
  empty result for that shape (`{events:[],count:0}` /
  `{threads:[]}` / `{actions:[]}`), not an error.

---

## 5. Risks and follow-up actions

### Pre-merge / pre-ship

- **R1 (medium).** APK build cannot be exercised on this Mac (no JDK,
  no Android SDK).
  - Mitigation: `expo prebuild --platform android` was dry-run and
    succeeded; `eas.json` ships APK profiles. The remaining step is
    cloud-build via `eas build --platform android --profile preview`
    after `eas login`.
- **R2 (medium).** Pre-existing `expo-doctor` failures (M3 above).
  Recommend running `npx expo install react-native-worklets` before
  the first APK build.
- **R3 (low).** Database migration `00003` requires the
  `timescaledb` extension. The previous two migrations already do, so
  any environment that boots the existing template has it. New
  deployments should verify `CREATE EXTENSION IF NOT EXISTS
  timescaledb;` runs in `00001_init.sql`.

### Post-ship operational

- **R4.** Tail `notification_events` cardinality. If a single user
  exceeds ~1M rows in a month, set up the Timescale retention policy
  the scope doc calls out (drop chunks > 90 days).
- **R5.** Watch the agent's tool-call traces for `pending_actions`
  false positives. If the heuristic misranks > 20% of items, replace
  the regex pre-filter with an LLM call against the last hour's
  events.
- **R6.** SharedPreferences buffer is plaintext. If the user enrols
  in MDM or roots their phone the buffer is readable. If we ship to
  enterprise clients, swap to `EncryptedSharedPreferences` (Jetpack
  Security).

### Ship checklist (run in order from `mobile/`)

1. `npm install`
2. `npx expo install react-native-worklets` *(addresses R2)*
3. `eas login` *(if not already)*
4. `eas build --platform android --profile preview --non-interactive`
   — produces a sideloadable APK URL when complete (~15 min cloud build).
5. Email/AirDrop the APK to the real estate agent, walk them through
   enabling Notification Access in Settings → Apps → Special access.
6. Backend: set `NOTIFICATIONS_ENABLED=true` on the agent's instance,
   apply migration 00003, restart server.
7. Watch `notification_events` row count grow as the device flushes.

---

## 6. Verification matrix

| Gate | Command | Result |
|---|---|---|
| Backend `go vet` | `cd backend && go vet ./...` | ✅ no diagnostics |
| Backend tests | `cd backend && go test ./internal/notifications/... ./internal/mcp/...` | ✅ all packages pass |
| Backend build | `cd backend && go build ./...` | ✅ |
| Mobile typecheck | `cd mobile && npm run typecheck` | ✅ |
| Native android scaffold | `cd mobile && npx expo prebuild --platform android --no-install --clean` | ✅ |
| Autolink discovery | `cd mobile && npx expo-modules-autolinking search --platform android` | ✅ `notification-capture` listed |
| APK build | `cd mobile && eas build --platform android --profile preview` | ⏸ requires `eas login` (no local SDK on this Mac) |
| Expo doctor | `cd mobile && npx expo-doctor` | ⚠ 3 pre-existing failures (R2/M3) |
