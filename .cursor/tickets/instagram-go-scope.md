# `instagram-go` — Instagram Web API Scraper SDK Scope

**Repo:** `github.com/teslashibe/instagram-go`  
**Package:** `github.com/teslashibe/instagram-go`  
**Mirrors:** `x-go` / `linkedin-go` / `reddit-go` conventions (stdlib only, zero prod deps)  
**Purpose:** Authenticated Go client for Instagram's private web/mobile API (`api/v1/*`). Covers profiles, posts, reels, stories, comments, followers/following, hashtags, locations, and search — giving full programmatic access to Instagram's content graph.

---

## Auth Verification

All endpoints confirmed working with:
- **User-Agent:** `Mozilla/5.0 ... Instagram 103.1.0.15.119 Android ...` (Instagram mobile UA required — Chrome UA returns `useragent mismatch`)
- **App-ID header:** `X-IG-App-ID: 936619743392459`
- **Required cookies:** `sessionid`, `csrftoken`, `ds_user_id`, `datr`, `mid`, `ig_did`
- **CSRF header:** `X-CSRFToken: <csrftoken value>` on all requests

---

## Confirmed Working Endpoints

All endpoints below were live-tested. Status `ok` was returned on every call.

### Authentication & Session

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/api/v1/accounts/current_user/?edit=true` | Returns full authenticated user profile (239 fields). Requires Instagram mobile UA. |

**`Cookies` struct fields:**
```
SessionID   string  // sessionid cookie — primary auth credential
CSRFToken   string  // csrftoken cookie — also sent as X-CSRFToken header
DSUserID    string  // ds_user_id — numeric user ID of logged-in account
Datr        string  // datr — device auth token
Mid         string  // mid — machine ID
IgDid       string  // ig_did — device ID
```

---

### Users / Profiles

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/api/v1/users/web_profile_info/?username={username}` | 62-field profile by username. Returns `data.user` object. Best for initial lookup. |
| `GET` | `/api/v1/users/{user_id}/info/` | 239-field full profile by numeric user ID. Superset of web_profile_info. |
| `GET` | `/api/v1/users/search/?q={query}&count={n}` | Username/name search. Returns `users[]` with pk, username, full_name. |
| `GET` | `/api/v1/discover/chaining/?target_id={user_id}` | Up to 80 suggested/related accounts for a given user. |

**Key `User` fields (sampled from `/info/` response — 239 total):**
```
pk / id              string    // numeric user ID
username             string
full_name            string
biography            string
biography_with_entities object // includes @mentions and hashtag entities
bio_links            []object  // external link(s) with title + URL
external_url         string
profile_pic_url      string
hd_profile_pic_url_info object // {url, width, height}
follower_count       int
following_count      int
media_count          int
is_verified          bool
is_business          bool
is_private           bool
category             string    // e.g. "Media/News Company"
category_id          int
city_name            string
zip                  string
address_street       string
instagram_location_id string
public_email         string
public_phone_number  string
contact_phone_number string
whatsapp_number      string
account_type         int       // 1=personal, 2=creator, 3=business
account_badges       []object
is_open_to_collab    bool
total_clips_count    int
has_highlight_reels  bool
highlight_reel_count int       // from web_profile_info
has_guides           bool
fan_club_info        object    // subscription info
mutual_followers_count int
profile_context      string    // "X and Y follow them"
pronouns             []string
```

---

### Posts / Media

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/api/v1/feed/user/{user_id}/?count={n}&max_id={cursor}` | User's post feed. Paginated via `next_max_id`. Returns up to 12 per page. |
| `GET` | `/api/v1/media/{media_id}/info/` | Full metadata for a single post by numeric ID. |
| `GET` | `/api/v1/usertags/{user_id}/feed/?count={n}&max_id={cursor}` | Posts where the user has been tagged. Paginated. |

**Pagination:** `more_available: bool` + `next_max_id: string` on feed endpoints.

**`media_type` enum:**
```
1 = Photo
2 = Video  
8 = Carousel (album)
```
Reels have `product_type: "clips"`.

**Key `Post` fields (from `/info/` response):**
```
pk                   string    // numeric media ID
id                   string    // media ID (same as pk)
code                 string    // shortcode (e.g. "C6jijrpJ3cr") — used in /p/{code}/ URLs
taken_at             int64     // unix timestamp
media_type           int       // 1=photo, 2=video, 8=carousel
product_type         string    // "feed", "clips" (reels), "igtv"
like_count           int
comment_count        int
play_count           int       // reels/video only
ig_play_count        int       // reels/video only
video_duration       float64   // seconds, video/reels only
has_audio            bool
caption              object    // {text, created_at, user}
locations            []object  // tagged locations (note: plural, not `location`)
usertags             object    // {in: [{user, position}]}
top_likers           []string  // up to 3 usernames
has_liked            bool      // viewer's own like status
is_paid_partnership  bool
coauthor_producers   []object
carousel_media       []object  // if media_type=8; each item has image_versions2
image_versions2      object    // {candidates: [{url, width, height}]}
video_versions       []object  // [{type, url, width, height}]
video_dash_manifest  string    // MPEG-DASH manifest URL
clips_metadata       object    // reels-specific (audio, challenge, etc.)
music_metadata       object    // music track info
user                 object    // author (partial User)
```

---

### Reels (Clips)

| Method | Path | Notes |
|--------|------|-------|
| `POST` | `/api/v1/clips/user/` | User's reels feed. Body: `target_user_id={id}&page_size=12`. Paginated via `paging_info.max_id`. |

**Additional reel fields in `clips_metadata`:**
```
audio_type               string  // "original_sounds", "licensed_music"
original_sound_info      object  // if user-original audio
challenge_info           object  // linked hashtag challenge
audio_ranking_info       object
achievements_info        object
```

---

### Comments

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/api/v1/media/{media_id}/comments/?can_support_threading=true` | Post comments. Returns `comment_count` + `comments[]`. Paginated via `next_min_id` (cursor is a JSON object, base64-encoded for next call). |
| `GET` | `/api/v1/media/{media_id}/comments/{comment_id}/child_comments/` | Replies to a specific comment. Returns `child_comments[]`. |
| `GET` | `/api/v1/media/{media_id}/comment_likers/?comment_id={id}` | Users who liked a specific comment. Returns `users[]`. |

**Key `Comment` fields:**
```
pk                   string    // comment ID
text                 string
created_at           int64
like_count           int
comment_like_count   int
has_liked_comment    bool      // viewer status
user                 object    // partial User (pk, username, profile_pic_url, is_verified)
child_comment_count  int
preview_child_comments []object
```

---

### Likers

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/api/v1/media/{media_id}/likers/` | Returns up to 100 users who liked a post. No pagination cursor — capped at 100. |

**Returns:** `users[]` with `pk`, `username`, `full_name`, `profile_pic_url`, `is_verified`, `is_private`.

---

### Followers / Following

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/api/v1/friendships/{user_id}/followers/?count={n}&max_id={cursor}` | User's followers. Paginated via `next_max_id`. |
| `GET` | `/api/v1/friendships/{user_id}/following/?count={n}&max_id={cursor}` | Accounts the user follows. Paginated via `next_max_id`. |
| `GET` | `/api/v1/friendships/show/{user_id}/` | Friendship status between viewer and target. |
| `POST` | `/api/v1/friendships/show_many/` | Batch friendship status for multiple user IDs. Body: `user_ids={id1}%2C{id2}`. |

**`FriendshipStatus` fields:**
```
following            bool
followed_by          bool
blocking             bool
muting               bool
is_private           bool
incoming_request     bool
outgoing_request     bool
is_bestie            bool
is_feed_favorite     bool
is_restricted        bool
subscribed           bool
```

---

### Stories

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/api/v1/feed/reels_tray/` | Home stories tray — all unseen stories for followed accounts. Returns `tray[]` with user + story metadata. |
| `GET` | `/api/v1/feed/user/{user_id}/story/` | Stories for a specific user. Returns `reel.items[]`. Empty if no active story. |
| `GET` | `/api/v1/highlights/{user_id}/highlights_tray/` | User's saved story highlights. Returns `tray[]` with `id`, `title`, `media_count`. |

**`StoryItem` fields** (subset of media fields):
```
pk, taken_at, media_type, expiring_at
image_versions2 / video_versions
has_audio, video_duration
story_link_stickers    []object  // link stickers
story_hashtag_stickers []object
story_location_stickers []object
```

---

### Hashtags

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/api/v1/tags/web_info/?tag_name={hashtag}` | Hashtag metadata + embedded top/recent media sections. Single request gives both info AND first page of media. |
| `POST` | `/api/v1/tags/{hashtag}/sections/` | Paginated hashtag media. Body: `tab=ranked` or `tab=recent` + `max_id={cursor}`. |

**`Hashtag` fields (from `web_info`):**
```
id                   string    // numeric hashtag ID
name                 string
media_count          int       // e.g. 91,101,543
formatted_media_count string   // "91.1M posts"
profile_pic_url      string    // representative image
is_trending          bool
subtitle             string
top                  object    // {sections[], next_max_id, more_available}
recent               object    // {sections[], next_max_id, more_available}
```

**Hashtag media sections** return `sections[]` where each section has:
```
layout_type          string    // "media_grid"
layout_content.medias []object // each with nested `media` object (full post fields)
```

---

### Locations

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/api/v1/location_search/?search_query={q}&timestamp={ts}` | Search locations by name. Returns `venues[]` with id, name, lat, lng. |
| `GET` | `/api/v1/locations/{location_id}/info/` | Location metadata by numeric ID. |
| `POST` | `/api/v1/locations/{location_id}/sections/` | Paginated media for a location. Body: `tab=ranked` or `tab=recent`. |

**`Location` fields:**
```
pk / external_id     string    // numeric location ID
name                 string
lat                  float64
lng                  float64
address              string
city                 string
short_name           string
facebook_places_id   string
```

---

### Search

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/api/v1/web/search/topsearch/?context=blended&query={q}&include_reel=true` | Blended search across users, hashtags, and places. |
| `GET` | `/api/v1/users/search/?q={query}&count={n}` | User-only search. Returns `users[]` with pk, username, full_name. |

**`TopSearch` response structure:**
```
users      []{ user: User, position: int }
hashtags   []{ hashtag: { id, name, media_count, profile_pic_url }, position: int }
places     []{ place: { location: Location, subtitle: string, title: string }, position: int }
```

---

### Explore

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/api/v1/discover/topical_explore/?is_prefetch=true&module=explore_popular&use_sectional_payload=true` | Explore page. Returns `sectional_items[]` with layout types and media. |

---

### Home Timeline

| Method | Path | Notes |
|--------|------|-------|
| `POST` | `/api/v1/feed/timeline/` | Authenticated user's home feed. Body: `is_pull_to_refresh=0&feed_view_info=[]`. Returns `feed_items[]` (mix of `media_or_ad` and suggested content). Paginated via `next_max_id`. |

---

## Pagination Pattern

All list endpoints use **cursor-based pagination** with `max_id`:

```
GET /api/v1/feed/user/{id}/?count=12
→ { items: [...], more_available: true, next_max_id: "3878755669196673665_787132" }

GET /api/v1/feed/user/{id}/?count=12&max_id=3878755669196673665_787132
→ next page
```

POST-based endpoints (hashtag sections, clips, locations) use `max_id` in the request body.

The comments endpoint uses a JSON cursor object as `next_min_id`.

---

## Rate Limiting

No rate limit headers were observed in testing (`X-RateLimit-*` not present). Instagram uses behavioral rate limiting. Recommended:
- Minimum 1–2s between requests
- Back off on HTTP 429 or `{"message": "Please wait a few minutes before you try again.", "status": "fail"}`
- On `{"message": "useragent mismatch", "status": "fail"}` — switch to Instagram mobile UA

---

## SDK Design

### Package Structure (mirrors `x-go` / `linkedin-go`)

```
instagram-go/
  instagram.go          // package doc, New(), Client, Option, Cookies
  client.go             // HTTP plumbing, do(), retry loop, rate-gap
  errors.go             // ErrInvalidAuth, ErrNotFound, ErrRateLimited, etc.
  models.go             // User, Post, Comment, Hashtag, Location, etc.
  profile.go            // GetProfile(username), GetProfileByID(id)
  feed.go               // GetUserPosts(), GetReels(), GetTaggedPosts()
  comments.go           // GetComments(), GetCommentReplies(), GetCommentLikers()
  followers.go          // GetFollowers(), GetFollowing(), GetFriendshipStatus()
  stories.go            // GetStories(), GetHighlights(), GetStoriesTray()
  hashtags.go           // GetHashtag(), GetHashtagPosts()
  locations.go          // SearchLocations(), GetLocation(), GetLocationPosts()
  search.go             // Search(), SearchUsers()
  timeline.go           // GetTimeline()
  explore.go            // GetExplore()
  iterator.go           // page iterator (same pattern as x-go)
  go.mod
  examples/
    get_profile/main.go
    get_posts/main.go
    search/main.go
  integration_test.go
  README.md
```

### `Cookies` Struct

```go
type Cookies struct {
    SessionID string `json:"sessionid"`  // primary session credential
    CSRFToken string `json:"csrftoken"`  // CSRF token (also sent as header)
    DSUserID  string `json:"ds_user_id"` // numeric user ID
    Datr      string `json:"datr"`       // device auth token
    Mid       string `json:"mid"`        // machine ID
    IgDid     string `json:"ig_did"`     // device ID (optional)
}
```

### `Client` Options (mirrors x-go pattern)

```go
New(cookies Cookies, opts ...Option) (*Client, error)

WithUserAgent(ua string) Option          // override default Instagram mobile UA
WithHTTPClient(hc *http.Client) Option
WithProxy(proxyURL string) Option
WithMinRequestGap(d time.Duration) Option  // default 1.5s
WithRetry(maxAttempts int, base time.Duration) Option
WithAppID(appID string) Option           // override X-IG-App-ID (default 936619743392459)
```

### Core Types

```go
// User — profile data
type User struct {
    ID             string
    Username       string
    FullName       string
    Biography      string
    BioLinks       []BioLink
    ExternalURL    string
    ProfilePicURL  string
    FollowerCount  int
    FollowingCount int
    MediaCount     int
    IsVerified     bool
    IsBusiness     bool
    IsPrivate      bool
    Category       string
    City           string
    PublicEmail    string
    IsOpenToCollab bool
    TotalClipsCount int
}

// Post — photo, video, or carousel
type Post struct {
    ID           string
    Shortcode    string      // for instagram.com/p/{shortcode}/
    MediaType    MediaType   // Photo=1, Video=2, Carousel=8
    ProductType  string      // "feed", "clips", "igtv"
    TakenAt      time.Time
    Author       User
    Caption      string
    LikeCount    int
    CommentCount int
    PlayCount    int         // reels/video
    VideoDuration float64   // seconds
    Images       []Image     // all resolutions
    Videos       []Video
    CarouselItems []Post     // if Carousel
    Location     *Location
    TaggedUsers  []TaggedUser
    MusicTrack   *MusicTrack
    IsSponsored  bool
}

// Comment
type Comment struct {
    ID           string
    Text         string
    CreatedAt    time.Time
    Author       User
    LikeCount    int
    ReplyCount   int
    Replies      []Comment
}

// Hashtag
type Hashtag struct {
    ID           string
    Name         string
    MediaCount   int
    ProfilePicURL string
    IsTrending   bool
}

// Location
type Location struct {
    ID      string
    Name    string
    Lat     float64
    Lng     float64
    Address string
    City    string
}

// FriendshipStatus
type FriendshipStatus struct {
    Following       bool
    FollowedBy      bool
    Blocking        bool
    Muting          bool
    IsPrivate       bool
    IsRestricted    bool
    IsBestie        bool
    IsFeedFavorite  bool
    Subscribed      bool
}

// Page — generic paginated result
type Page[T any] struct {
    Items     []T
    NextCursor string
    HasNext   bool
}
```

---

## Method Surface Area

```go
// Auth
c.Me(ctx) (*User, error)

// Profiles
c.GetProfile(ctx, username string) (*User, error)
c.GetProfileByID(ctx, userID string) (*User, error)
c.SearchUsers(ctx, query string, count int) ([]User, error)
c.GetSuggestedUsers(ctx, targetUserID string) ([]User, error)

// Posts
c.GetPosts(ctx, userID string) *PostIterator          // paginated
c.GetPost(ctx, mediaID string) (*Post, error)
c.GetTaggedPosts(ctx, userID string) *PostIterator

// Reels
c.GetReels(ctx, userID string) *PostIterator           // paginated

// Comments
c.GetComments(ctx, mediaID string) *CommentIterator    // paginated
c.GetCommentReplies(ctx, mediaID, commentID string) ([]Comment, error)
c.GetCommentLikers(ctx, mediaID, commentID string) ([]User, error)

// Likers
c.GetLikers(ctx, mediaID string) ([]User, error)       // capped at 100

// Social graph
c.GetFollowers(ctx, userID string) *UserIterator        // paginated
c.GetFollowing(ctx, userID string) *UserIterator        // paginated
c.GetFriendshipStatus(ctx, userID string) (*FriendshipStatus, error)
c.GetFriendshipStatuses(ctx, userIDs []string) (map[string]FriendshipStatus, error)

// Stories
c.GetStoriesTray(ctx) ([]StoryReel, error)             // home feed stories
c.GetUserStories(ctx, userID string) ([]StoryItem, error)
c.GetHighlights(ctx, userID string) ([]Highlight, error)

// Hashtags
c.GetHashtag(ctx, name string) (*Hashtag, error)
c.GetHashtagPosts(ctx, name, tab string) *PostIterator  // tab="ranked"|"recent"

// Locations
c.SearchLocations(ctx, query string) ([]Location, error)
c.GetLocation(ctx, locationID string) (*Location, error)
c.GetLocationPosts(ctx, locationID, tab string) *PostIterator

// Search
c.Search(ctx, query string) (*SearchResult, error)     // blended: users+hashtags+places

// Feeds
c.GetTimeline(ctx) *PostIterator                       // home feed
c.GetExplore(ctx) ([]ExploreSection, error)

// --- Write actions ---

// Social graph (Tier 1 — confirmed 200 in testing)
c.Follow(ctx, userID string) (*FriendshipStatus, error)
c.Unfollow(ctx, userID string) (*FriendshipStatus, error)

// Social graph (Tier 2 — correct paths, require mobile-app session or careful pacing)
c.Block(ctx, userID string) (*FriendshipStatus, error)
c.Unblock(ctx, userID string) (*FriendshipStatus, error)
c.Mute(ctx, userID string) error
c.Unmute(ctx, userID string) error
c.SetBesties(ctx, add []string, remove []string) error  // close friends

// Media actions (Tier 2 — require mobile-app session origin or careful pacing)
c.Like(ctx, mediaID string) error
c.Unlike(ctx, mediaID string) error
c.Save(ctx, mediaID string) error
c.Unsave(ctx, mediaID string) error
c.PostComment(ctx, mediaID, text, replyToCommentID string) (*Comment, error)
c.DeleteComment(ctx, mediaID, commentID string) error
c.LikeComment(ctx, mediaID, commentID string) error
c.UnlikeComment(ctx, mediaID, commentID string) error
c.MarkStorySeen(ctx, userID, mediaID string) error
```

---

## Write Endpoints

Write actions split into two behavioural tiers discovered through live testing:

- **Tier 1 – Confirmed 200 OK**: follow and unfollow returned 200 and a full `friendship_status` body.  
- **Tier 2 – Correct path, 302 under rate limit**: all other write endpoints returned `302 → /accounts/login/` once Instagram's behavioural write rate-limiter activated. The paths are correct and well-documented; they require careful pacing (≥5s between writes, exponential back-off on 302).

### Social Graph Writes

All body parameters sent as `application/x-www-form-urlencoded`.

| Method | Path | Body params | Notes |
|--------|------|------------|-------|
| `POST` | `/api/v1/friendships/create/{user_id}/` | _(empty)_ | Follow a public account. For private accounts, sends a follow request (`outgoing_request: true`). Returns `friendship_status`. **Confirmed 200.** |
| `POST` | `/api/v1/friendships/destroy/{user_id}/` | _(empty)_ | Unfollow, or withdraw a pending follow request. Returns `friendship_status`. **Confirmed 200.** |
| `POST` | `/api/v1/friendships/block/{user_id}/` | _(empty)_ | Block a user. Returns `friendship_status`. Path confirmed, rate-limited in testing. |
| `POST` | `/api/v1/friendships/unblock/{user_id}/` | _(empty)_ | Unblock a user. Returns `friendship_status`. |
| `POST` | `/api/v1/friendships/mute_posts_or_story_from_follow/` | `target_posts_author_id={user_id}` | Mute a followed account's posts and/or stories. |
| `POST` | `/api/v1/friendships/unmute_posts_or_story_from_follow/` | `target_posts_author_id={user_id}` | Unmute. |
| `POST` | `/api/v1/friendships/set_besties/` | `add=[{id}]&remove=[]&source=settings` | Add/remove from Close Friends list. |

**Follow response body:**
```json
{
  "friendship_status": {
    "following": true,
    "is_bestie": false,
    "is_feed_favorite": false,
    "is_private": false,
    "is_restricted": false,
    "incoming_request": false,
    "outgoing_request": false,
    "followed_by": false,
    "muting": false,
    "blocking": false,
    "is_eligible_to_subscribe": false,
    "subscribed": false
  },
  "previous_following": true,
  "error": null,
  "status": "ok"
}
```

---

### Media Action Writes

All `POST`, body as `application/x-www-form-urlencoded`. Paths confirmed correct; endpoints require either a mobile-app session or will 302 when write rate-limit triggers.

| Method | Path | Body params | Notes |
|--------|------|------------|-------|
| `POST` | `/api/v1/media/{media_id}/like/` | `module_name=feed_timeline&d=0` | Like a post. `d=0` = timeline, `d=1` = profile grid. |
| `POST` | `/api/v1/media/{media_id}/unlike/` | `module_name=feed_timeline` | Remove a like. |
| `POST` | `/api/v1/media/{media_id}/save/` | `added_collection_ids=[]` | Bookmark/save a post. Optionally add to a collection. |
| `POST` | `/api/v1/media/{media_id}/unsave/` | _(empty)_ | Remove bookmark. |
| `POST` | `/api/v1/media/{media_id}/comment/` | `comment_text={text}&replied_to_comment_id=` | Post a comment. Set `replied_to_comment_id` for replies. Returns the new `Comment` object. |
| `DELETE` | `/api/v1/media/{media_id}/comment/{comment_id}/` | _(empty)_ | Delete own comment. |
| `POST` | `/api/v1/media/{media_id}/comment_like/` | `comment_id={id}` | Like a comment. |
| `POST` | `/api/v1/media/{media_id}/comment_unlike/` | `comment_id={id}` | Unlike a comment. |
| `POST` | `/api/v1/media/seen/` | `reels[{user_id}][]={media_id}_{user_id}` | Mark a story item as seen. |

**Comment response body** (on `POST /comment/`):
```json
{
  "comment": {
    "pk": "18333281527223547",
    "text": "...",
    "created_at": 1776171604,
    "user": { "pk": "188023731", "username": "...", "profile_pic_url": "..." },
    "status": "Active",
    "like_count": 0,
    "child_comment_count": 0
  },
  "status": "ok"
}
```

---

### Write Rate Limiting — Key Findings

Discovered through live session testing:

1. **Writes use a separate rate limiter from reads.** The same session that serves unlimited reads will 302 all writes once the write rate limiter activates. Read endpoints continued returning 200 throughout all write testing.

2. **The write rate limiter activates fast under programmatic conditions.** Follow + unfollow on a single account worked (2 successful writes), then subsequent write attempts 302'd — including re-following the same account. Estimated window: **~2–5 writes per minute** before soft block activates.

3. **302 response clears `sessionid` in `Set-Cookie`.** When rate-limited, Instagram returns `Set-Cookie: sessionid=""; Max-Age=0` across all domains. This is a **cookie-jar trap** — if using a jar-based HTTP client, the session will be wiped. When passing cookies via explicit header (Go's `http.Header`), the session survives and reads continue working.

4. **Media write endpoints may require mobile app session origin.** Like, comment, save, and comment_like all 302'd on the very first attempt (before the rate limiter would plausibly have triggered), suggesting these endpoints may additionally verify the session originated from a native app install rather than a web browser login. Follow/unfollow are more permissive in this regard.

5. **Recommended write pacing:** Minimum **5–10s between writes**, randomised. Exponential back-off (starting at 60s) on any 302 response to a write endpoint.

---

## Out of Scope (V1)

- Post creation (photo/video upload) — requires multipart upload to `rupload.qe.instagram.com`
- Stories creation/upload
- Instagram Shopping / product tagging
- IGTV (long-form video — rare, being deprecated)
- Live video
- Broadcast channels
- Threads integration

---

## Notes & Caveats

1. **User-Agent is critical.** The Instagram mobile UA (`Instagram 103.1.0.15.119 Android ...`) is required. Chrome/Safari UAs return `{"message": "useragent mismatch"}`.

2. **`X-IG-App-ID: 936619743392459`** must be sent on every request. This is the web app's registered app ID.

3. **Shortcode vs. media ID.** The public URL uses a `shortcode` (e.g. `C6jijrpJ3cr`), but all API endpoints use a numeric `media_id`. The `code` field on the Post response gives the shortcode for URL construction. GraphQL (`/graphql/query/` with `doc_id=10015901848480474`) resolves shortcode → full post but requires additional request parameters.

4. **Followers are not paginating fully for public accounts.** The followers endpoint returns up to ~49 results per page for large accounts (natgeo: 275M followers). Use `next_max_id` for pagination. The `max_id` cursor resets between sessions.

5. **Comment cursor is complex.** The `next_min_id` cursor for comments is a JSON object, not a plain string. It must be passed as-is (URL-encoded) in the next request.

6. **Stories expire.** Story items have `expiring_at` timestamp (24 hours from `taken_at`). Tray items include `seen` timestamp.

7. **Carousel posts.** `media_type=8` posts have a `carousel_media[]` array where each item is a full media object (type 1 or 2).

8. **Rate limiting.** No explicit headers; Instagram uses server-side behavioral limiting. Recommend 1.5s default gap between requests, exponential backoff on 429 or `status: fail` with "wait a few minutes" message.
