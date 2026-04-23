# `threads-go` — Threads Web API Scraper SDK Scope

**Repo:** `github.com/teslashibe/threads-go`  
**Package:** `github.com/teslashibe/threads-go`  
**Mirrors:** `x-go` / `linkedin-go` / `instagram-go` conventions (stdlib only, zero prod deps)  
**Purpose:** Authenticated Go client for Threads' private REST API (`/api/v1/*`). Covers user profiles, thread posts, replies/conversations, social graph, hashtags, likers, search, and write actions (like, follow, post, reply, repost, delete, block, mute) — full programmatic read/write access to Threads' content graph.

---

## Auth Architecture — Two Distinct Modes

There are two separate auth mechanisms. **Read and write use different hosts, auth headers, and credential types.**

---

### Mode 1: Cookie Auth (Read operations)

**Host:** `https://www.threads.com/api/v1/`  
All read endpoints were live-tested against this host and returned HTTP 200.

- **User-Agent:** `Barcelona 289.0.0.14.109 Android` — **required**. Threads rejects Chrome/Safari UAs with `{"message":"useragent mismatch"}`. `Instagram {version} Android` also works.
- **App-ID header:** `X-IG-App-ID: 238260118697367`
- **Required cookies:** `sessionid`, `csrftoken`, `ds_user_id`, `mid`, `ig_did`
- **CSRF header:** `X-CSRFToken: <csrftoken value>` — must match the cookie on every request

**`Cookies` struct fields:**
```
SessionID  string  // sessionid — primary auth credential (URL-encoded)
CSRFToken  string  // csrftoken — also sent as X-CSRFToken header
DSUserID   string  // ds_user_id — numeric user ID of logged-in account
Mid        string  // mid — machine ID
IgDid      string  // ig_did — device ID
```

**How to obtain:** Export from browser dev tools (Application → Cookies → `.threads.com`) after logging in at threads.com. All five cookies are required; `sessionid` is the primary credential.

---

### Mode 2: Bearer Token Auth (Write operations)

**Host:** `https://i.instagram.com/api/v1/`  
Write operations go to Instagram's API backend, not threads.com. The write auth uses a Bearer token obtained via the **Bloks login flow** — separate from browser session cookies.

- **Authorization header:** `Authorization: Bearer IGT:2:{token}` — token is ~160 chars
- **User-Agent:** `Barcelona 289.0.0.77.109 Android`
- **Content-Type:** `application/x-www-form-urlencoded; charset=UTF-8`
- **No cookies required** for writes — the Bearer token is self-contained

**How to obtain the Bearer token:**

```bash
POST https://i.instagram.com/api/v1/bloks/apps/com.bloks.www.bloks.caa.login.async.send_login_request/
User-Agent: Barcelona 289.0.0.77.109 Android
Content-Type: application/x-www-form-urlencoded; charset=UTF-8

params={"client_input_params":{"password":"$PASSWORD","contact_point":"$USERNAME","device_id":"$DEVICE_ID"},"server_params":{"credential_type":"password","device_id":"$DEVICE_ID"}}
&bloks_versioning_id=00ba6fa565c3c707243ad976fa30a071a625f2a3d158d9412091176fe35027d8
```

- `$DEVICE_ID` — a random device identifier of shape `android-{13_random_chars}`
- The response is a large JSON blob. The token appears immediately after the string `Bearer IGT:2:` in the payload.
- Token is ~160 characters long and has an expiry (hours–days; store and refresh).
- **Bloks versioning ID** `00ba6fa565c3c707243ad976fa30a071a625f2a3d158d9412091176fe35027d8` is the Threads app's current versioning hash — may change with app updates.
- The Bloks login endpoint was confirmed live (HTTP 200) during testing.

**`Auth` struct fields for write client:**
```
Token     string  // IGT:2:... bearer token from Bloks login
UserID    string  // numeric user ID (from login response or separate lookup)
DeviceID  string  // android-{13chars} — stable per device
```

**`signed_body` format for write requests:**

Many write endpoints use a `signed_body` POST parameter. The format is:
```
signed_body=SIGNATURE.{url_encoded_json_payload}
```
`SIGNATURE` is the **literal string** "SIGNATURE" — there is no cryptographic signing required at the client level for this implementation. The JSON payload is URL-encoded using RFC 3986 safe chars (`!~*'()`).

Example:
```python
payload = json.dumps({"caption": "Hello world", "publish_mode": "text_post", ...})
body = f"signed_body=SIGNATURE.{urllib.parse.quote(payload, safe='!~*()')}"
```

---

## Rate Limiting Observed

Threads uses behavioral rate limiting (no explicit `X-RateLimit-*` headers). During testing:
- After ~20–30 consecutive API calls, the session returns `403` with `{"message":"login_required","logout_reason":8}` — this is a **temporary suspension**, not permanent invalidation. `logout_reason: 8` = device fingerprint anomaly detected.
- Sessions recover within ~15–30 minutes of no further requests.
- Write endpoints (`POST /like/`, `POST /friendships/create/`) return 403 and require additional device signing (see notes) — **out of scope for V1 read-only SDK**.

**Recommendation:**
- Default minimum gap: **1.5s** between requests
- Exponential backoff on any `403` / `{"status":"fail"}`
- Surface `ErrRateLimited` and `ErrUnauthorized` as distinct sentinel errors

---

## Confirmed Working Endpoints

All endpoints below were live-tested. HTTP 200 and valid JSON was returned.

### Authentication & Current User

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/api/v1/accounts/current_user/?edit=true` | Full profile of the authenticated user. 40+ fields. Confirmed 200. |

---

### Users / Profiles

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/api/v1/users/{user_id}/info/` | Full profile by numeric user ID. Returns `user` object with 40+ fields. |
| `GET` | `/api/v1/users/{user_id}/info/?entry_point=profile&from_module=profile_page` | Extended profile variant — includes `text_app_is_low_like`, `text_app_media_reuse_enabled`, `current_catalog_id`, `ads_incentive_expiration_date`. |
| `GET` | `/api/v1/users/search/?q={query}&count={n}` | User search. Returns `num_results`, `users[]`, `has_more`. |

**Resolving username → user ID:** Use the user search endpoint; filter results for exact username match. The `pk` field is the stable numeric user ID to use in all subsequent calls.

**Key `User` fields (confirmed via `/info/` and `/accounts/current_user/`):**
```
pk / id / strong_id__   string   // numeric user ID
username                string
full_name               string
is_private              bool
is_verified             bool
profile_pic_url         string
hd_profile_pic_url_info object   // {url, width, height}
hd_profile_pic_versions []object // [{url, width, height}] — multiple sizes
text_app_biography      object   // {text_fragments: {fragments: []}} — Threads bio
fbid_v2                 string   // Meta-level user ID
has_nme_badge           bool
show_fb_link_on_profile bool
show_fb_page_link_on_profile bool
third_party_downloads_enabled int
eligible_for_text_app_activation_badge bool
feed_post_reshare_disabled bool
text_app_is_low_like    bool
hide_text_app_activation_badge_on_text_app bool
```

**Note on user lookup by username:** There is no `/api/v1/users/lookup/?username=` endpoint (405). Use search and exact-match the `username` field.

---

### Thread Posts

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/api/v1/text_feed/{user_id}/profile/` | A user's thread posts. Returns `threads[]` (each with `thread_items[]`) + `next_max_id` cursor. Confirmed for both public and authenticated user. |

**Pagination:** Response includes `next_max_id` string. Pass as `?max_id={cursor}` in the next request. When `next_max_id` is absent, no further pages exist.

**Response structure:**
```json
{
  "medias": [],
  "threads": [
    {
      "thread_items": [{ "post": { ... }, "thread_item_type": int, ... }],
      "thread_type": int,
      "id": "string"
    }
  ],
  "next_max_id": "cursor_string",
  "status": "ok"
}
```

**Key `Post` fields (confirmed via `thread_items[].post`):**
```
pk / id / strong_id__        string     // numeric post ID
fbid                         string     // Meta-level post ID
code                         string     // base64 post code — used in URL: /post/{code}
taken_at                     int64      // unix timestamp of post
media_type                   int        // 19 = text post; 1 = photo; 2 = video
product_type                 string     // "text_post"
like_count                   int
caption                      object     // {text: string, created_at, user}
text_post_app_info           object     // link preview, quoted post, etc.
user                         object     // partial User (pk, username, profile_pic_url, is_verified)
image_versions2              object     // media attachments (if any)
has_liked                    bool       // viewer's own like status
can_viewer_reshare           bool
like_and_view_counts_disabled bool
integrity_review_decision    string
caption_is_edited            bool
gen_ai_detection_method      string
community_notes_info         object
crosspost_metadata           object     // reposts / cross-app
permalink                    string     // canonical URL
organic_tracking_token       string
original_width               int
original_height              int
```

**Thread URL construction:** `https://www.threads.com/@{username}/post/{code}` where `code` is the `code` field on the post object.

---

### Thread Detail & Replies

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/api/v1/text_feed/{thread_id}/replies/` | Thread conversation context. Returns root post + up to 3 reply threads per page with a downward cursor. |
| `GET` | `/api/v1/text_feed/{thread_id}/replies/?count={n}` | Same with explicit reply count per page. |

**Response structure (thread context):**
```json
{
  "target_post_id":   "numeric post ID",
  "containing_thread": {
    "thread_items": [ ... ],   // the root post + any parent posts in the thread
    "thread_type": int,
    "id": "string"
  },
  "reply_threads": [ ... ],    // top-level replies, each is a thread_items[]
  "sibling_threads": [ ... ],  // threads at the same level (same parent)
  "paging_tokens": {
    "downwards": "base64_cursor"
  },
  "downwards_thread_will_continue": bool,
  "is_subscribed_to_target_post": bool,
  "is_author_of_root_post": bool,
  "target_post_reply_placeholder": string,
  "show_unavailable_replies_disclaimer": bool
}
```

**Pagination:** Pass `paging_tokens.downwards` as `?paging_token={cursor}` for subsequent pages of replies.

**`thread_item` fields:**
```
post                         object   // full Post object (see above)
thread_item_type             int
is_contextual                bool
line_type                    string
should_show_replies_cta      bool
can_inline_expand_below      bool
view_replies_cta_string      string
reply_facepile_users         []object // small avatars preview
reply_to_author              object   // partial User
should_show_permalink_upsell bool
is_parent_unavailable        bool
parent_post_unavailable_reason string
```

---

### Likers

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/api/v1/media/{media_id}/likers/` | Returns `users[]` and `user_count`. The `users[]` list is a sample (confirmed empty for private posts, sample for public). `user_count` is authoritative. |

**Response:**
```json
{ "users": [], "user_count": 2776, "status": "ok" }
```

---

### Social Graph

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/api/v1/friendships/{user_id}/followers/?count={n}&max_id={cursor}` | Paginated follower list. Returns `users[]`, `has_more`, `next_max_id`, `follow_ranking_token`. |
| `GET` | `/api/v1/friendships/{user_id}/following/?count={n}&max_id={cursor}` | Paginated following list. Same response shape. |
| `GET` | `/api/v1/friendships/show/{user_id}/` | Friendship status between authenticated user and target. |
| `GET` | `/api/v1/friendships/pending/?count={n}` | Incoming follow requests for private accounts. Returns `users[]`, `friend_requests`, `next_max_id`. |

**`FriendshipStatus` fields (confirmed via `/friendships/show/`):**
```
following                    bool
followed_by                  bool
blocking                     bool
muting                       bool
is_private                   bool
is_restricted                bool
incoming_request             bool
outgoing_request             bool
is_bestie                    bool
is_feed_favorite             bool
is_eligible_to_subscribe     bool
is_blocking_reel             bool
is_muting_reel               bool
is_muting_notes              bool
is_muting_media_notes        bool
is_muting_media_reposts      bool
text_post_app_pre_following  bool
```

**Follower list `User` fields (subset):**
```
pk, username, full_name, profile_pic_url, is_verified, is_private,
has_onboarded_to_text_post_app, third_party_downloads_enabled
```

---

### Hashtags

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/api/v1/tags/search/?q={query}` | Hashtag search. Returns `results[]` with id, name, media_count, formatted_media_count, search_result_subtitle. |
| `GET` | `/api/v1/tags/{tag_name}/info/` | Hashtag metadata. Returns id, name, media_count, formatted_media_count, is_trending, subtitle. |

**Note:** Hashtag post feed (`/tags/{name}/sections/`, `/tags/{name}/ranked_sections/`) returned 404 / 405 in testing. These appear Threads-specific and may require a different request format or are GraphQL-only. Treat as **out of scope V1**.

**`Hashtag` fields (confirmed):**
```
id                    string   // "17843756752021523"
name                  string   // "ai"
media_count           int      // 53117151
formatted_media_count string   // "53.1M"
is_trending           bool
subtitle              string   // nullable
allow_muting_story    bool
follow_button_text    string   // nullable
show_follow_drop_down bool
hide_use_hashtag_button bool
```

---

### Liked Posts (authenticated user only)

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/api/v1/feed/liked/?count={n}` | Posts liked by the authenticated user. Returns `items[]` (full post objects), `num_results`, `more_available`. |

---

### Home Timeline Feed (write-auth only)

| Method | Path | Notes |
|--------|------|-------|
| `POST` | `/api/v1/feed/text_post_app_timeline/` | Authenticated user's "For You" home feed. Uses Bearer token auth on `i.instagram.com`. Body: `pagination_source=text_post_feed_threads`. Paginated via `max_id`. |

**Note:** This endpoint requires Bearer token auth (Mode 2). It was NOT accessible with cookie auth in testing.

---

### Notifications

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/api/v1/text_feed/text_app_notifications/` | Activity notifications for the authenticated user. Uses Bearer token auth. |

**Query params:**
```
feed_type=all
mark_as_seen=false
selected_filter=text_post_app_replies   // or: text_post_app_mentions | verified
timezone_offset={tz_offset_seconds}
max_id={cursor}                          // for pagination
pagination_first_record_timestamp={ts}  // for pagination
```

---

### Recommended Users

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/api/v1/text_feed/recommended_users/` | Suggested accounts for the authenticated user. Uses Bearer token auth. Paginated via `max_id`. |

---

### User Replies Feed

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/api/v1/text_feed/{user_id}/profile/replies` | All reply posts by a user (replies-tab). Uses Bearer token auth. Paginated via `max_id`. |

---

## Write Endpoints

All write operations use **Bearer token auth (Mode 2)** on `https://i.instagram.com/api/v1/`. They require the `Bearer IGT:2:{token}` Authorization header obtained via the Bloks login flow. The `X-CSRFToken` header and cookies are **not** used for writes.

Many write endpoints use the `signed_body=SIGNATURE.{url_encoded_json}` POST body format (see Auth section above).

---

### Content Creation

| Method | Path | Body params | Notes |
|--------|------|-------------|-------|
| `POST` | `/media/configure_text_only_post/` | `signed_body` | Create a text-only thread post. |
| `POST` | `/media/configure_text_post_app_feed/` | `signed_body` | Create a thread post with a single image attachment. |
| `POST` | `/media/configure_text_post_app_sidecar/` | `signed_body` | Create a thread post with multiple images (carousel/sidecar). |
| `POST` | `https://www.instagram.com/rupload_igphoto/{upload_name}` | binary body | Upload image before posting. Returns `upload_id` for use in configure call. |

**Text post `signed_body` JSON payload fields:**
```json
{
  "publish_mode":        "text_post",     // required for text-only
  "caption":             "Post text here",
  "text_post_app_info": {
    "reply_control":     0,               // 0=everyone, 1=accounts you follow, 2=mentioned only
    "reply_id":          "POST_ID",       // omit for new post; set for replies
    "link_attachment_url": "https://...", // omit if no URL attachment
    "quoted_post_id":    "POST_ID"        // omit unless quoting a post
  },
  "timezone_offset":     "0",
  "source_type":         "4",
  "_uid":                "USER_ID",
  "device_id":           "android-DEVICE_ID",
  "upload_id":           "TIMESTAMP_MS",  // int(time.now().unix_ms())
  "device": {
    "manufacturer":      "OnePlus",
    "model":             "ONEPLUS+A3003",
    "android_version":   26,
    "android_release":   "8.1.0"
  }
}
```

**Image post** — same payload plus:
```json
{
  "upload_id":           "ID_FROM_RUPLOAD_RESPONSE",
  "scene_capture_type":  ""
}
```

**Sidecar (multi-image)** — same payload plus:
```json
{
  "client_sidecar_id": "TIMESTAMP_MS",
  "children_metadata": [
    {"upload_id": "ID1", "source_type": "4", "timezone_offset": "0", "scene_capture_type": ""},
    {"upload_id": "ID2", "source_type": "4", "timezone_offset": "0", "scene_capture_type": ""}
  ]
}
```

**Image upload headers (rupload):**
```
User-Agent:               Barcelona 289.0.0.77.109 Android
Authorization:            Bearer IGT:2:{token}
X-Instagram-Rupload-Params: {"media_type":1,"upload_id":"ID","sticker_burnin_params":"[]","image_compression":"{...}","xsharing_user_ids":"[]","retry_context":"{...}","IG-FB-Xpost-entry-point-v2":"feed"}
X_FB_PHOTO_WATERFALL_ID:  {uuid4}
X-Entity-Type:            image/jpeg
X-Entity-Name:            {upload_name}
X-Entity-Length:          {byte_length}
Offset:                   0
Content-Type:             application/octet-stream
```

---

### Post Interactions

| Method | Path | Body params | Notes |
|--------|------|-------------|-------|
| `POST` | `/media/{post_id}_{user_id}/like/` | (empty) | Like a post. The path combines `post_id` + `_` + your `user_id`. |
| `POST` | `/media/{post_id}_{user_id}/unlike/` | (empty) | Remove a like from a post. |
| `POST` | `/media/{post_id}_{user_id}/delete/?media_type=TEXT_POST` | (empty) | Delete your own post. Query param `media_type=TEXT_POST` is required. |
| `POST` | `/repost/create_repost/` | `media_id={post_id}` | Repost a thread to your profile. |
| `POST` | `/repost/delete_text_app_repost/` | `original_media_id={post_id}` | Undo a repost. |

**Like/unlike path note:** The path uses a compound ID: `{post_pk}_{viewer_user_id}`. Example: `POST /media/3870872187813562164_75472043478/like/`

---

### Social Graph (Write)

| Method | Path | Body params | Notes |
|--------|------|-------------|-------|
| `POST` | `/friendships/create/{user_id}/` | (empty) | Follow a user. |
| `POST` | `/friendships/destroy/{user_id}/` | (empty) | Unfollow a user. |
| `POST` | `/friendships/block/{user_id}/` | `signed_body` | Block a user. |
| `POST` | `/friendships/unblock/{user_id}/` | `signed_body` | Unblock a user. |
| `POST` | `/friendships/mute_posts_or_story_from_follow/` | `signed_body` | Mute a user's posts (stays following). |
| `POST` | `/friendships/unmute_posts_or_story_from_follow/` | `signed_body` | Unmute a user's posts. |
| `POST` | `/restrict_action/restrict_many/` | `signed_body` | Restrict a user. |
| `POST` | `/restrict_action/unrestrict/` | `signed_body` | Unrestrict a user. |

**Block `signed_body` JSON:**
```json
{
  "user_id":               "TARGET_USER_ID",
  "surface":               "ig_text_feed_timeline",
  "is_auto_block_enabled": "true"
}
```

**Mute/unmute `signed_body` JSON:**
```json
{
  "target_posts_author_id": "TARGET_USER_ID",
  "container_module":       "ig_text_feed_timeline"
}
```

**Unblock/unrestrict `signed_body` JSON:**
```json
{
  "user_id":          "TARGET_USER_ID",
  "container_module": "ig_text_feed_timeline"
}
```

**Restrict `signed_body` JSON:**
```json
{
  "user_ids":         "TARGET_USER_ID",
  "container_module": "ig_text_feed_timeline"
}
```

---

### Write Response Shape

All write endpoints return `{"status": "ok"}` on success or `{"status": "fail", "message": "..."}` on error. Content creation endpoints return additional data:

**Configure text post response (success):**
```json
{
  "media": { /* full Post object */ },
  "upload_id": "string",
  "status": "ok"
}
```

**Repost response (success):**
```json
{
  "repost_fbid": "string",
  "status": "ok"
}
```

---

## Endpoints Confirmed Not Available / Redirected

| Path | Status | Notes |
|------|--------|-------|
| `GET /api/v1/feed/timeline/` | 405 | Home "For You" feed via cookie auth — use `POST i.instagram.com/api/v1/feed/text_post_app_timeline/` with Bearer token instead |
| `GET /api/v1/text_feed/following_v2/` | 404 | Following feed — not at this path |
| `GET /api/v1/text_feed/keyword_search/` | 404 | Post keyword search — not on REST path |
| `GET /api/v1/tags/{name}/ranked_sections/` | 404 | Tag post feed |
| `GET /api/v1/media/{id}/info/` | 500 | Single-post lookup via REST — use `/text_feed/{id}/replies/` for post + context instead |
| `GET /api/v1/media/{id}/replies/` | 404 | Instagram-style replies — use `/text_feed/{id}/replies/` |
| `GET /api/v1/media/{id}/reposters/` | 404 | Reposters list |
| `GET /api/v1/news/inbox/` | 500 | Notifications via cookie auth — use `/text_feed/text_app_notifications/` with Bearer token |
| `GET /api/v1/text_feed/{user_id}/profile_replies/` | 404 | User's reply history — not at this path; correct path is `/text_feed/{user_id}/profile/replies` |

---

## Pagination Pattern

All list endpoints use **cursor-based pagination** with `max_id`:

```
GET /api/v1/text_feed/{user_id}/profile/?count=10
→ { "threads": [...], "next_max_id": "3870872187813562164_63055343223" }

GET /api/v1/text_feed/{user_id}/profile/?count=10&max_id=3870872187813562164_63055343223
→ next page
```

Replies use a different token key:
```
GET /api/v1/text_feed/{thread_id}/replies/?count=5
→ { "paging_tokens": { "downwards": "GBZkb3dud..." } }

GET /api/v1/text_feed/{thread_id}/replies/?count=5&paging_token=GBZkb3dud...
→ next page of replies
```

---

## SDK Design

### Package Structure (mirrors `x-go` / `instagram-go`)

```
threads-go/
  threads.go          // package doc, New(), NewWithAuth(), Client, Option, Cookies, Auth
  client.go           // HTTP plumbing, do(), retry loop, rate-gap — handles both auth modes
  errors.go           // ErrInvalidAuth, ErrNotFound, ErrRateLimited, ErrUnauthorized
  models.go           // User, Thread, Post, ThreadContext, Hashtag, FriendshipStatus, etc.
  auth.go             // Login() — Bloks auth flow to get Bearer token
  profile.go          // GetProfile(), SearchUsers(), GetMe()
  feed.go             // GetUserThreads(), GetTimeline(), GetLikedPosts()
  thread.go           // GetThreadContext(), GetThreadReplies()
  replies.go          // GetUserReplies()
  likers.go           // GetLikers()
  followers.go        // GetFollowers(), GetFollowing(), GetFriendshipStatus(), GetPendingRequests()
  hashtags.go         // SearchHashtags(), GetHashtag()
  notifications.go    // GetNotifications(), GetRecommendedUsers()
  post.go             // Post(), PostReply(), PostWithImage(), PostWithImages()
  actions.go          // Like(), Unlike(), Repost(), DeleteRepost(), Delete()
  social.go           // Follow(), Unfollow(), Block(), Unblock(), Mute(), Unmute(), Restrict(), Unrestrict()
  go.mod
  examples/
    get_profile/main.go
    get_threads/main.go
    get_replies/main.go
    search_users/main.go
    post_thread/main.go
    like_follow/main.go
  integration_test.go
  README.md
```

### `Cookies` Struct (read-only client)

```go
type Cookies struct {
    SessionID string `json:"sessionid"`   // primary session credential (URL-encoded)
    CSRFToken string `json:"csrftoken"`   // CSRF token — also sent as X-CSRFToken header
    DSUserID  string `json:"ds_user_id"`  // numeric user ID of logged-in account
    Mid       string `json:"mid"`         // machine ID
    IgDid     string `json:"ig_did"`      // device ID
}
```

### `Auth` Struct (write-capable client)

```go
type Auth struct {
    Token    string  // Bearer IGT:2:... obtained via Login()
    UserID   string  // numeric user ID
    DeviceID string  // android-{13chars} — stable per session
}
```

### Client Constructors

```go
// Read-only client — uses cookie auth on www.threads.com
New(cookies Cookies, opts ...Option) (*Client, error)

// Write-capable client — uses Bearer token on i.instagram.com
NewWithAuth(auth Auth, opts ...Option) (*Client, error)

// Obtain auth via Bloks login (returns Auth for use with NewWithAuth)
Login(ctx context.Context, username, password, deviceID string) (*Auth, error)
```

### Client Options

```go
WithUserAgent(ua string) Option           // override default Barcelona UA
WithHTTPClient(hc *http.Client) Option
WithProxy(proxyURL string) Option
WithMinRequestGap(d time.Duration) Option // default 1.5s
WithRetry(maxAttempts int, base time.Duration) Option
WithAppID(appID string) Option            // override X-IG-App-ID (default 238260118697367)
```

### Core Types

```go
// User — profile data
type User struct {
    ID             string
    Username       string
    FullName       string
    IsPrivate      bool
    IsVerified     bool
    ProfilePicURL  string
    Biography      string    // from text_app_biography
    FbIDV2         string
}

// Post — a single thread post (media_type=19 for text, 1=photo, 2=video)
type Post struct {
    ID           string
    Code         string      // base64 code — for URL: /post/{code}
    TakenAt      time.Time
    Author       User
    Caption      string      // post text body
    LikeCount    int
    Permalink    string
    MediaType    int         // 19=text, 1=photo, 2=video
    HasLiked     bool        // viewer's own like status
    TextPostAppInfo TextPostAppInfo  // quoted post, link preview, etc.
    ImageVersions   []Image   // if media attached
}

// Thread — a thread item in a profile or conversation
type Thread struct {
    Items      []ThreadItem
    ThreadType int
    ID         string
}

// ThreadItem — one post within a thread
type ThreadItem struct {
    Post                Post
    ThreadItemType      int
    LineType            string
    ReplyFacepileUsers  []User
    ReplyToAuthor       *User
    ShouldShowRepliesCTA bool
    ViewRepliesCTAString string
}

// ThreadContext — full conversation context for a post
type ThreadContext struct {
    TargetPostID            string
    ContainingThread        Thread     // root post + parents
    ReplyThreads            []Thread   // top-level replies
    SiblingThreads          []Thread   // threads at the same depth
    PagingTokenDownwards    string     // cursor for more replies
    WillContinue            bool       // more replies exist
    IsSubscribed            bool
    IsAuthorOfRootPost      bool
}

// FriendshipStatus — relationship between viewer and a user
type FriendshipStatus struct {
    Following              bool
    FollowedBy             bool
    Blocking               bool
    Muting                 bool
    IsPrivate              bool
    IsRestricted           bool
    IncomingRequest        bool
    OutgoingRequest        bool
    IsBestie               bool
    IsFeedFavorite         bool
    MutingReposts          bool
}

// Hashtag
type Hashtag struct {
    ID           string
    Name         string
    MediaCount   int
    FormattedMediaCount string
    IsTrending   bool
}

// PostOptions — options for creating a new post
type PostOptions struct {
    Caption        string
    ReplyToID      string   // parent post ID for replies
    QuotedPostID   string   // post ID to quote
    LinkURL        string   // link attachment
    ImagePaths     []string // local paths or HTTP URLs
    ReplyControl   int      // 0=everyone, 1=following, 2=mentioned
    TimezoneOffset int      // UTC offset in seconds
}

// Page — generic paginated result
type Page[T any] struct {
    Items      []T
    NextCursor string
    HasNext    bool
}
```

### Method Surface Area

```go
// Auth
Login(ctx, username, password, deviceID string) (*Auth, error)
c.Me(ctx context.Context) (*User, error)

// Profiles
c.GetProfile(ctx context.Context, userID string) (*User, error)
c.SearchUsers(ctx context.Context, query string, count int) ([]User, error)

// Thread posts (profile feed)
c.GetUserThreads(ctx context.Context, userID string) *ThreadIterator      // paginated

// Thread replies tab
c.GetUserReplies(ctx context.Context, userID string) *ThreadIterator      // paginated; requires Bearer auth

// Thread detail + replies
c.GetThreadContext(ctx context.Context, threadID string, count int) (*ThreadContext, error)
c.GetThreadReplies(ctx context.Context, threadID, cursor string, count int) (*ThreadContext, error)

// Likers
c.GetLikers(ctx context.Context, mediaID string) ([]User, int, error)     // users, user_count

// Social graph (read)
c.GetFollowers(ctx context.Context, userID string) *UserIterator           // paginated
c.GetFollowing(ctx context.Context, userID string) *UserIterator           // paginated
c.GetFriendshipStatus(ctx context.Context, userID string) (*FriendshipStatus, error)
c.GetPendingRequests(ctx context.Context) *UserIterator                    // incoming follow requests

// Hashtags
c.SearchHashtags(ctx context.Context, query string) ([]Hashtag, error)
c.GetHashtag(ctx context.Context, name string) (*Hashtag, error)

// Feeds (authenticated)
c.GetLikedPosts(ctx context.Context, count int) ([]Post, bool, error)     // requires cookie auth
c.GetTimeline(ctx context.Context) *PostIterator                           // requires Bearer auth

// Notifications & discovery (requires Bearer auth)
c.GetNotifications(ctx context.Context, filter string) *NotificationIterator
c.GetRecommendedUsers(ctx context.Context) *UserIterator

// ── Write methods — require NewWithAuth / Bearer token ──

// Post creation
c.Post(ctx context.Context, opts PostOptions) (*Post, error)              // text, image, reply, quote

// Post interactions
c.Like(ctx context.Context, postID string) error
c.Unlike(ctx context.Context, postID string) error
c.Repost(ctx context.Context, postID string) error
c.DeleteRepost(ctx context.Context, postID string) error
c.Delete(ctx context.Context, postID string) error

// Social graph (write)
c.Follow(ctx context.Context, userID string) error
c.Unfollow(ctx context.Context, userID string) error
c.Block(ctx context.Context, userID string) error
c.Unblock(ctx context.Context, userID string) error
c.Mute(ctx context.Context, userID string) error
c.Unmute(ctx context.Context, userID string) error
c.Restrict(ctx context.Context, userID string) error
c.Unrestrict(ctx context.Context, userID string) error
```

### Iterator Pattern (mirrors `x-go`)

```go
type ThreadIterator struct { /* unexported */ }

func (it *ThreadIterator) Next(ctx context.Context) ([]Thread, error)
func (it *ThreadIterator) HasMore() bool
func (it *ThreadIterator) Cursor() string
```

---

## Key Notes & Caveats

1. **Two distinct auth modes.** Read operations use browser session cookies on `www.threads.com`. Write operations use a Bearer `IGT:2:` token from the Bloks login flow on `i.instagram.com`. They are separate credential sets; the SDK needs both constructors.

2. **User-Agent is critical.** `Barcelona 289.0.0.14.109 Android` (reads) and `Barcelona 289.0.0.77.109 Android` (writes/Bloks) are required. Chrome/macOS UAs return `{"message":"useragent mismatch","status":"fail"}` with HTTP 400.

3. **App-ID `238260118697367` is Threads' web app ID**, confirmed from HTML source. Pass as `X-IG-App-ID` header on read requests. Write requests use Bearer token instead.

4. **`signed_body` is `SIGNATURE.{payload}` — not an actual HMAC.** The literal string "SIGNATURE" is used as the prefix. The JSON payload is URL-encoded with safe chars `!~*'()`. No cryptographic signing is needed client-side.

5. **Write endpoint host is `i.instagram.com`, not `www.threads.com`.** The Threads and Instagram backends share the same write API.

6. **Like/unlike path uses compound ID:** `{post_pk}_{viewer_user_id}`. Both components must be numeric. Example: `POST /media/3870872187813562164_75472043478/like/`.

7. **Bloks versioning ID** `00ba6fa565c3c707243ad976fa30a071a625f2a3d158d9412091176fe35027d8` is the current Threads app hash embedded in the binary. It may change with app updates — treat as a configurable constant.

8. **Username → user ID resolution.** Use `GET /api/v1/users/search/?q={username}&count=5` and match `username` field exactly to get `pk`. No direct username-to-ID endpoint exists.

9. **Thread ID vs. code.** The `pk` (e.g. `3870872187813562164`) is used in API calls. The `code` field (e.g. `DW4Gb79kQc0`) is the URL slug used in `threads.com/@user/post/{code}`.

10. **`media_type=19`** identifies a Threads text post. Regular photo posts have `media_type=1`.

11. **Rate limiting is aggressive.** After ~20–30 sequential requests, Threads suspends the session temporarily (`logout_reason: 8`). Always enforce a minimum gap (1.5s default), add jitter, and back off exponentially on 403.

12. **Session cookie expiry.** The `sessionid` cookie has a long TTL (months) from the browser, but the server may invalidate it earlier if unusual activity is detected. The Bearer token from Bloks login also has an expiry and must be refreshed.

---

## Out of Scope (V1)

- Keyword / post content search (no confirmed REST path)
- Tag / hashtag post feeds
- Following feed (distinct from For You timeline)
- GraphQL mutations (`doc_id`s rotate with every deploy)
- Image upload to non-Instagram CDN
- Direct messaging / threads inbox
