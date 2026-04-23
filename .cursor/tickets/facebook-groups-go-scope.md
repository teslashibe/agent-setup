# `facebook-go` — Groups Surface Scope

**Repo:** `github.com/teslashibe/facebook-go`  
**Package:** `github.com/teslashibe/facebook-go/groups`  
**Mirrors:** `linkedin-go` / `reddit-scraper` conventions (stdlib only, zero prod deps)  
**Purpose:** First surface of the `facebook-go` client — authenticated Go package for Facebook Groups, used by the GTM agent persona layer.

---

## Background & Reverse-Engineering Notes

Facebook Groups is backed by a single internal GraphQL endpoint. All operations —
reads and writes — are POST requests to `/api/graphql/` with form-encoded bodies.
Every request must carry a set of session tokens extracted from an initial page load
("session bootstrap"). The auth layer is cookie-based; the six cookies below give
full write access.

### Required Cookies (from browser session export)

| Cookie | httpOnly | Purpose |
|---|---|---|
| `sb` | ✓ | Browser session fingerprint |
| `datr` | ✓ | Device auth token (login device binding) |
| `c_user` | — | **User ID** (same as `__user` in every request) |
| `xs` | ✓ | **Session token** (most critical — acts like `li_at`) |
| `fr` | ✓ | Friend-request + ad tracking |
| `ps_l` / `ps_n` | ✓ | Presence / status |

### Session Bootstrap (one-time per process)

Perform a single authenticated `GET https://www.facebook.com/groups/feed/` and
extract the following tokens from the HTML response body using regex:

| Token | Regex | Used in |
|---|---|---|
| `fb_dtsg` | `"DTSGInitialData"[^}]*"token":"([^"]+)"` | Every GraphQL POST (CSRF) |
| `lsd` | `"LSD"[^}]*"token":"([^"]+)"` | `X-FB-LSD` header + form body |
| `__rev` | `"client_revision":(\d+)` | `__rev` + `__spin_r` form params |
| `__hs` | `"haste_session":"([^"]+)"` | `__hs` form param |
| `__hsi` | `"hsi":"(\d+)"` | `__hsi` form param |
| `__spin_t` | `"spin_t":(\d+)` | `__spin_t` form param |
| `jazoest` | computed (see below) | form body anti-bot |

**`jazoest` computation:**
```
sum = 0
for each rune r in fb_dtsg: sum += int(r)
jazoest = "2" + strconv.Itoa(sum)
```

### GraphQL Request Structure

```
POST https://www.facebook.com/api/graphql/
Content-Type: application/x-www-form-urlencoded

Headers (required):
  User-Agent:           Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) ...Chrome/131...
  Referer:              https://www.facebook.com/
  Origin:               https://www.facebook.com
  Accept:               */*
  Accept-Language:      en-US,en;q=0.9
  X-FB-Friendly-Name:   <query name e.g. GroupsCometGroupFeedQuery>
  X-FB-LSD:             <lsd token>
  X-ASBD-ID:            129477

Form body (all required):
  av            = <c_user / user ID>
  __user        = <c_user / user ID>
  __a           = 1
  __req         = <atomically incrementing hex counter per client, e.g. "1","2","a","b">
  __hs          = <haste_session>
  dpr           = 1
  __ccg         = EXCELLENT
  __rev         = <client_revision>
  __s           = <spin session string from HTML>
  __hsi         = <hsi>
  __comet_req   = 15
  fb_dtsg       = <dtsg token>
  jazoest       = <computed>
  lsd           = <lsd token>
  __spin_r      = <client_revision>
  __spin_b      = trunk
  __spin_t      = <spin_t>
  server_timestamps = true
  fb_api_caller_class   = RelayModern
  fb_api_req_friendly_name = <query name>
  variables     = <JSON string of query variables>
  doc_id        = <GraphQL document ID>
```

### Response Format

Every response body is prefixed with `for (;;);` (XSS guard). Strip this prefix
before JSON decoding. The shape is:

```json
{
  "data": { ... },
  "extensions": { "is_final": true }
}
```

Errors come as `{"errors": [{"message": "...", "severity": "CRITICAL"}]}`.

### Key GraphQL Operations & doc_ids

> ⚠️ `doc_id` values change with each Facebook code deploy. The package stores
> hardcoded defaults and lets callers override via `WithDocIDs(map[string]string)`,
> identical to how `linkedin-go` handles `queryID` rotation.

| Operation | Friendly Name | doc_id (approximate, verify on first run) |
|---|---|---|
| My Groups | `GroupsHomeNewQuery` | `7810301489006745` |
| Search Groups | `GroupSearchResultsPageQuery` | `8589379677810700` |
| Discover / Suggested | `GroupsDiscoverSuggestionsQuery` | `6705661989513295` |
| Group Info | `GroupsCometGroupPageQuery` | `4242218965874992` |
| Group Feed | `GroupsCometGroupFeedQuery` | `7265828503443504` |
| Group Feed (next page) | `GroupsCometGroupFeedPaginationQuery` | `8148491931922494` |
| Join Group | `GroupJoinMutation` | `4556533284398215` |
| Leave Group | `GroupLeaveMutation` | `5024190864356295` |
| Create Group | `GroupCreateMutation` | `6012044985567001` |
| Create Post | `ComposerStoryCreateMutation` | `7737694812917255` |
| Create Comment | `CommentCreateMutation` | `5418265591589706` |
| React to Post | `CometUFIFeedbackReactMutation` | `6996109963813789` |
| Get Comments | `CommentsListComponentPaginationQuery` | `9060882280617430` |
| Get Group Members | `GroupsCometMembersPageQuery` | `5100470406669740` |
| Search within Group | `CometGroupFeedSearchQuery` | `7491484044277512` |
| Get Post Detail | `CometSinglePostRouteQuery` | `6841468425963700` |

---

## File Structure

```
facebook-go/                        ← repo root
├── go.mod                          # module github.com/teslashibe/facebook-go
├── doc.go                          # top-level package doc (surfaces index)
├── README.md
└── groups/                         ← first surface
    ├── doc.go                      # package groups
    ├── facebook.go                 # Client, New(), Option funcs, Cookies type
    ├── session.go                  # sessionState, bootstrap(), token extraction
    ├── client.go                   # graphql(), do(), reqCounter, rate limiting, retry
    ├── groups.go                   # SearchGroups, DiscoverGroups, MyGroups, GetGroup,
    │                               #   JoinGroup, LeaveGroup, CreateGroup
    ├── feed.go                     # GetGroupFeed, GetGroupFeedPage (cursor pagination)
    ├── posts.go                    # GetPost, CreatePost
    ├── comments.go                 # GetPostComments, CreateComment, ReactToPost
    ├── members.go                  # GetGroupMembers, GetGroupMembersPage
    ├── trends.go                   # ScrapeGroupTrends → TrendReport
    ├── types.go                    # All exported domain types
    ├── internal.go                 # Unexported GraphQL response shapes
    └── errors.go                   # Sentinel errors
```

---

## Domain Types (`types.go`)

```go
// Cookies holds the Facebook session cookies from a browser export.
type Cookies struct {
    SB       string // sb
    DATR     string // datr
    CUser    string // c_user (user ID)
    XS       string // xs (session token)
    FR       string // fr
    PSL      string // ps_l
    PSN      string // ps_n
}

// Group is the canonical group metadata model.
type Group struct {
    ID          string     `json:"id"`
    Name        string     `json:"name"`
    URL         string     `json:"url"`
    Description string     `json:"description,omitempty"`
    MemberCount int        `json:"memberCount,omitempty"`
    Privacy     Privacy    `json:"privacy"`         // PUBLIC | CLOSED | SECRET
    CoverURL    string     `json:"coverUrl,omitempty"`
    AdminIDs    []string   `json:"adminIds,omitempty"`
    Joined      bool       `json:"joined"`
    PendingJoin bool       `json:"pendingJoin"`
    CreatedAt   time.Time  `json:"createdAt,omitempty"`
}

type Privacy string
const (
    PrivacyPublic Privacy = "PUBLIC"
    PrivacyClosed Privacy = "CLOSED"
    PrivacySecret Privacy = "SECRET"
)

// Post represents a group post / story.
type Post struct {
    ID          string     `json:"id"`
    FeedbackID  string     `json:"feedbackId"`   // needed for comment/react mutations
    GroupID     string     `json:"groupId"`
    AuthorID    string     `json:"authorId"`
    AuthorName  string     `json:"authorName"`
    Message     string     `json:"message"`
    Attachments []string   `json:"attachments,omitempty"` // image/link URLs
    ReactionCount int      `json:"reactionCount"`
    CommentCount  int      `json:"commentCount"`
    ShareCount    int      `json:"shareCount"`
    CreatedAt   time.Time  `json:"createdAt"`
    UpdatedAt   time.Time  `json:"updatedAt,omitempty"`
}

// Comment represents a comment on a post.
type Comment struct {
    ID         string    `json:"id"`
    FeedbackID string    `json:"feedbackId"`
    AuthorID   string    `json:"authorId"`
    AuthorName string    `json:"authorName"`
    Message    string    `json:"message"`
    ReactionCount int   `json:"reactionCount"`
    CreatedAt  time.Time `json:"createdAt"`
}

// Member is a group member.
type Member struct {
    ID     string `json:"id"`
    Name   string `json:"name"`
    URL    string `json:"url"`
    IsAdmin bool  `json:"isAdmin"`
}

// GroupSearchResult is a lightweight result from SearchGroups.
type GroupSearchResult struct {
    ID          string  `json:"id"`
    Name        string  `json:"name"`
    URL         string  `json:"url"`
    MemberCount int     `json:"memberCount"`
    Privacy     Privacy `json:"privacy"`
    CoverURL    string  `json:"coverUrl,omitempty"`
}

// FeedPage is one page of group feed results with a cursor for the next page.
type FeedPage struct {
    Posts      []Post `json:"posts"`
    NextCursor string `json:"nextCursor,omitempty"` // empty when no more pages
    HasNext    bool   `json:"hasNext"`
}

// CommentPage is one page of comments with a cursor for the next page.
type CommentPage struct {
    Comments   []Comment `json:"comments"`
    NextCursor string    `json:"nextCursor,omitempty"`
    HasNext    bool      `json:"hasNext"`
}

// MemberPage is one page of group members.
type MemberPage struct {
    Members    []Member `json:"members"`
    NextCursor string   `json:"nextCursor,omitempty"`
    HasNext    bool     `json:"hasNext"`
}

// ReactionType enumerates supported post reactions.
type ReactionType string
const (
    ReactionLike    ReactionType = "LIKE"
    ReactionLove    ReactionType = "LOVE"
    ReactionCare    ReactionType = "CARE"
    ReactionHaha    ReactionType = "HAHA"
    ReactionWow     ReactionType = "WOW"
    ReactionSad     ReactionType = "SAD"
    ReactionAngry   ReactionType = "ANGRY"
)

// TrendReport is the output of ScrapeGroupTrends.
type TrendReport struct {
    GroupID       string            `json:"groupId"`
    PostsAnalyzed int               `json:"postsAnalyzed"`
    TopKeywords   []KeywordFreq     `json:"topKeywords"`
    TopHashtags   []KeywordFreq     `json:"topHashtags"`
    AvgEngagement float64           `json:"avgEngagement"` // (reactions+comments+shares)/posts
    PeakHours     []int             `json:"peakHours"`     // UTC hour → post count ranking
    SentimentScore float64          `json:"sentimentScore"`// naive: +1 positive, -1 negative keywords
    ActiveAuthors []AuthorActivity  `json:"activeAuthors"`
}

type KeywordFreq struct {
    Term  string `json:"term"`
    Count int    `json:"count"`
}

type AuthorActivity struct {
    AuthorID   string `json:"authorId"`
    AuthorName string `json:"authorName"`
    PostCount  int    `json:"postCount"`
}
```

---

## API Surface (`groups/facebook.go`, all exported methods on `*Client`)

```go
// Auth / bootstrap — import "github.com/teslashibe/facebook-go/groups"
func New(cookies Cookies, opts ...Option) (*Client, error)
// New performs session bootstrap immediately, returning err if auth fails.

// Groups — discovery & management
func (c *Client) SearchGroups(ctx context.Context, query string, opts ...SearchOption) ([]GroupSearchResult, error)
func (c *Client) DiscoverGroups(ctx context.Context) ([]GroupSearchResult, error)
func (c *Client) MyGroups(ctx context.Context) ([]Group, error)
func (c *Client) GetGroup(ctx context.Context, groupID string) (*Group, error)
func (c *Client) JoinGroup(ctx context.Context, groupID string) error
func (c *Client) LeaveGroup(ctx context.Context, groupID string) error
func (c *Client) CreateGroup(ctx context.Context, params CreateGroupParams) (*Group, error)

// Feed — reading posts
func (c *Client) GetGroupFeed(ctx context.Context, groupID string) (FeedPage, error)
func (c *Client) GetGroupFeedPage(ctx context.Context, groupID, cursor string) (FeedPage, error)
func (c *Client) GetPost(ctx context.Context, postID string) (*Post, error)

// Posts — writing
func (c *Client) CreatePost(ctx context.Context, groupID, message string, opts ...PostOption) (*Post, error)

// Comments
func (c *Client) GetPostComments(ctx context.Context, feedbackID string) (CommentPage, error)
func (c *Client) GetPostCommentsPage(ctx context.Context, feedbackID, cursor string) (CommentPage, error)
func (c *Client) CreateComment(ctx context.Context, feedbackID, message string) (*Comment, error)
func (c *Client) ReactToPost(ctx context.Context, feedbackID string, reaction ReactionType) error

// Members
func (c *Client) GetGroupMembers(ctx context.Context, groupID string) (MemberPage, error)
func (c *Client) GetGroupMembersPage(ctx context.Context, groupID, cursor string) (MemberPage, error)

// Trends
func (c *Client) ScrapeGroupTrends(ctx context.Context, groupID string, opts ...TrendOption) (*TrendReport, error)
// ScrapeGroupTrends paginates through the feed collecting posts up to opts.MaxPosts
// (default 200), then computes the TrendReport in-memory.
```

### Options & Params

```go
// Client options
func WithUserAgent(ua string) Option
func WithDocIDs(overrides map[string]string) Option  // key = friendly name
func WithRetry(maxAttempts int, base time.Duration) Option
func WithHTTPClient(hc *http.Client) Option
func WithProxy(proxyURL string) Option
func WithMinRequestGap(d time.Duration) Option       // default 800ms

// SearchGroups options
type SearchOption func(*searchOptions)
func WithSearchLocation(cityOrRegion string) SearchOption
func WithSearchLimit(n int) SearchOption             // default 20

// CreateGroup params
type CreateGroupParams struct {
    Name        string
    Privacy     Privacy  // default CLOSED
    Description string
}

// CreatePost options
type PostOption func(*postOptions)
func WithPostAttachmentURL(url string) PostOption

// Trend options
type TrendOption func(*trendOptions)
func WithTrendMaxPosts(n int) TrendOption            // default 200
func WithTrendTopN(n int) TrendOption                // top N keywords, default 20
func WithTrendStopWords(words []string) TrendOption  // additional stop words to filter
```

---

## User Stories & Acceptance Criteria

### US-1 · Auth & Session Bootstrap
**As a developer, I can construct a `Client` from a cookie struct so the session is
validated immediately and all subsequent calls are authenticated.**

- AC-1.1: `New(cookies)` performs a `GET /groups/feed/` to extract `fb_dtsg`, `lsd`, `__rev`, `__hs`, `__hsi`, `__spin_t`. Returns `ErrUnauthorized` if any required token is absent.
- AC-1.2: `Cookies.CUser` is validated as non-empty; returns `ErrInvalidAuth` if blank.
- AC-1.3: Cookie jar is populated with all six cookies before the bootstrap request.
- AC-1.4: Session tokens are stored in a thread-safe `sessionState` struct, refreshed automatically on `ErrSessionExpired`.
- AC-1.5: `__req` counter is an `atomic.Uint64`, formatted as lowercase hex.
- AC-1.6: `jazoest` is recomputed when `fb_dtsg` changes.

---

### US-2 · Search Groups
**As a GTM agent, I can search for Facebook Groups by keyword so I can discover target communities.**

- AC-2.1: `SearchGroups(ctx, "crypto traders")` returns `[]GroupSearchResult` with ID, Name, URL, MemberCount, Privacy for each result.
- AC-2.2: Results include both public and closed groups (not secret groups, which are invisible to non-members).
- AC-2.3: `WithSearchLocation("San Francisco")` filters results geographically.
- AC-2.4: `WithSearchLimit(50)` returns up to 50 results in a single call (single page from the API).
- AC-2.5: Returns `ErrNotFound` when zero results match the query.
- AC-2.6: Friendly name sent to API: `GroupSearchResultsPageQuery`.

---

### US-3 · Discover Groups
**As a GTM agent, I can browse Facebook's recommended groups so I can find communities I haven't thought of.**

- AC-3.1: `DiscoverGroups(ctx)` returns Facebook's personalized group suggestions for the authenticated user.
- AC-3.2: Returns at least the first page of suggestions (≥ 5 groups).
- AC-3.3: Friendly name sent to API: `GroupsDiscoverSuggestionsQuery`.

---

### US-4 · My Groups
**As a GTM agent, I can list all groups I have joined so I can manage my community presence.**

- AC-4.1: `MyGroups(ctx)` returns all groups the authenticated user is a member of.
- AC-4.2: Each `Group` in the result has `Joined: true`.
- AC-4.3: Includes groups where membership is pending approval (`PendingJoin: true`).
- AC-4.4: Friendly name sent to API: `GroupsHomeNewQuery`.

---

### US-5 · Get Group Info
**As a GTM agent, I can retrieve metadata about any public or joined group so I can qualify it before engaging.**

- AC-5.1: `GetGroup(ctx, groupID)` returns a populated `*Group`.
- AC-5.2: MemberCount, Privacy, Description, CoverURL, and AdminIDs are populated when available.
- AC-5.3: Returns `ErrNotFound` for nonexistent group IDs.
- AC-5.4: Returns `ErrForbidden` for secret groups the user is not a member of.
- AC-5.5: Friendly name: `GroupsCometGroupPageQuery`.

---

### US-6 · Join Group
**As a GTM agent, I can join a public or closed group so I can participate in the community.**

- AC-6.1: `JoinGroup(ctx, groupID)` returns `nil` on success.
- AC-6.2: For **public** groups, membership is immediate and the `Group.Joined` field will be `true` afterwards.
- AC-6.3: For **closed** groups, a join request is sent; subsequent `GetGroup` returns `PendingJoin: true`.
- AC-6.4: Calling `JoinGroup` on an already-joined group returns `ErrAlreadyMember`.
- AC-6.5: Friendly name: `GroupJoinMutation`.

---

### US-7 · Leave Group
**As a GTM agent, I can leave a group so I can clean up memberships.**

- AC-7.1: `LeaveGroup(ctx, groupID)` returns `nil` on success.
- AC-7.2: Returns `ErrNotMember` if the user is not a member of the group.
- AC-7.3: Friendly name: `GroupLeaveMutation`.

---

### US-8 · Create Group
**As a GTM agent, I can create a new Facebook Group so I can build a community around my brand/persona.**

- AC-8.1: `CreateGroup(ctx, params)` returns a `*Group` with the server-assigned `ID`.
- AC-8.2: `params.Privacy` defaults to `PrivacyClosed` if not specified.
- AC-8.3: `params.Name` must be non-empty; returns `ErrInvalidParams` otherwise.
- AC-8.4: Friendly name: `GroupCreateMutation`.

---

### US-9 · Get Group Feed
**As a GTM agent, I can retrieve the most recent posts from a group so I can understand current discussions.**

- AC-9.1: `GetGroupFeed(ctx, groupID)` returns the first page of posts as a `FeedPage`.
- AC-9.2: Each `Post` includes: ID, FeedbackID, AuthorName, Message, ReactionCount, CommentCount, ShareCount, CreatedAt.
- AC-9.3: `FeedPage.HasNext` is `true` when more posts exist; `FeedPage.NextCursor` is set.
- AC-9.4: `GetGroupFeedPage(ctx, groupID, cursor)` fetches the next page using the cursor.
- AC-9.5: Returns `ErrForbidden` for groups where the user is not a member and privacy is CLOSED or SECRET.
- AC-9.6: Friendly names: `GroupsCometGroupFeedQuery` (first page), `GroupsCometGroupFeedPaginationQuery` (subsequent).

---

### US-10 · Scrape Historical Posts
**As a GTM agent, I can iterate through old group posts with cursor pagination so I can analyze historical trends.**

- AC-10.1: Repeated calls to `GetGroupFeedPage` with the cursor from the previous response yields all available posts.
- AC-10.2: When no more posts exist, `FeedPage.HasNext == false` and `NextCursor == ""`.
- AC-10.3: The client respects `WithMinRequestGap` between paginated calls.

---

### US-11 · Create Post in Group
**As a GTM agent, I can publish a text post to a group so I can engage with the community.**

- AC-11.1: `CreatePost(ctx, groupID, message)` returns a `*Post` with the server-assigned `ID`.
- AC-11.2: `WithPostAttachmentURL(url)` appends a link attachment to the post.
- AC-11.3: `message` must not be empty; returns `ErrInvalidParams` otherwise.
- AC-11.4: Returns `ErrForbidden` if the user is not a member of the group.
- AC-11.5: Friendly name: `ComposerStoryCreateMutation`.

---

### US-12 · Create Comment
**As a GTM agent, I can comment on a group post so I can join specific conversations.**

- AC-12.1: `CreateComment(ctx, feedbackID, message)` returns a `*Comment`.
- AC-12.2: `feedbackID` is the `Post.FeedbackID` field (not the post ID).
- AC-12.3: Returns `ErrNotFound` for nonexistent feedbackIDs.
- AC-12.4: Friendly name: `CommentCreateMutation`.

---

### US-13 · React to Post
**As a GTM agent, I can react to posts (Like, Love, Care, etc.) so I can engage passively without writing content.**

- AC-13.1: `ReactToPost(ctx, feedbackID, ReactionLike)` posts a Like reaction and returns `nil`.
- AC-13.2: All seven `ReactionType` constants work correctly.
- AC-13.3: Calling on an already-reacted post changes the reaction type (no error).
- AC-13.4: Friendly name: `CometUFIFeedbackReactMutation`.

---

### US-14 · Get Post Comments
**As a GTM agent, I can read all comments on a post so I can understand community sentiment.**

- AC-14.1: `GetPostComments(ctx, feedbackID)` returns first-page comments with AuthorName, Message, CreatedAt.
- AC-14.2: `GetPostCommentsPage(ctx, feedbackID, cursor)` paginates through all comments.
- AC-14.3: Friendly name: `CommentsListComponentPaginationQuery`.

---

### US-15 · Get Group Members
**As a GTM agent, I can list group members so I can identify influencers and active participants.**

- AC-15.1: `GetGroupMembers(ctx, groupID)` returns first page of members with ID, Name, URL, IsAdmin.
- AC-15.2: `GetGroupMembersPage` paginates through all members.
- AC-15.3: Returns `ErrForbidden` for groups where the member list is hidden.
- AC-15.4: Friendly name: `GroupsCometMembersPageQuery`.

---

### US-16 · Scrape Group Trends
**As a GTM agent, I can analyze a group's recent posts for trending topics, keywords, and engagement patterns so I can identify pain points and craft targeted messaging.**

- AC-16.1: `ScrapeGroupTrends(ctx, groupID)` paginates through up to `WithTrendMaxPosts(200)` posts.
- AC-16.2: `TrendReport.TopKeywords` returns the top-N (default 20) unigrams/bigrams by frequency, filtered for stop words (English stop list bundled in package).
- AC-16.3: `TrendReport.TopHashtags` extracts `#hashtag` tokens from post messages.
- AC-16.4: `TrendReport.AvgEngagement` is `(sum of reactions + comments + shares) / postsAnalyzed`.
- AC-16.5: `TrendReport.PeakHours` is a slice of UTC hours (0–23) sorted by post count descending.
- AC-16.6: `TrendReport.ActiveAuthors` lists authors sorted by `PostCount` descending; top-10 by default.
- AC-16.7: `WithTrendStopWords([]string{"buy", "sell"})` adds domain-specific stop words on top of the bundled list.
- AC-16.8: The function respects `ctx` cancellation; returns partial report if cancelled mid-scrape with a wrapped `ErrPartialResult`.

---

## Client Transport & Rate-Limiting

| Concern | Behaviour |
|---|---|
| **Request gap** | Min 800ms between requests (leaky-bucket, same as reddit-scraper) |
| **Retry** | 3 attempts, 500ms exponential base; retries on 429 and 5xx only |
| **429 handling** | Honour `Retry-After` header; if absent, back off 60s |
| **Session refresh** | On 401/`ErrSessionExpired`, re-bootstrap once then retry |
| **`for (;;);` strip** | Strip before every JSON decode |
| **Proxy support** | `WithProxy("http://host:port")` wraps transport |
| **Context** | All methods accept `context.Context`; cancel propagates immediately |

---

## Sentinel Errors (`errors.go`)

```go
var (
    ErrInvalidAuth    = errors.New("facebook: missing or empty required cookie (xs or c_user)")
    ErrUnauthorized   = errors.New("facebook: authentication failed (session expired or invalid)")
    ErrForbidden      = errors.New("facebook: access denied to this resource")
    ErrNotFound       = errors.New("facebook: resource not found")
    ErrRateLimited    = errors.New("facebook: rate limited")
    ErrAlreadyMember  = errors.New("facebook: already a member of this group")
    ErrNotMember      = errors.New("facebook: not a member of this group")
    ErrInvalidParams  = errors.New("facebook: invalid or missing required parameters")
    ErrPartialResult  = errors.New("facebook: context cancelled; partial result returned")
    ErrRequestFailed  = errors.New("facebook: HTTP request failed")
    ErrSessionExpired = errors.New("facebook: session expired")
)
```

---

## Implementation Notes

### doc_id Discovery
When a `doc_id` returns a GraphQL error like `"The document with ID X has been deleted"`,
the client should log a warning and return `ErrNotFound` with a hint to use `WithDocIDs()`
to supply updated IDs. The caller (agent) is responsible for refreshing doc_ids by
loading the relevant Facebook page and grepping the JS bundles:

```bash
curl -s -b 'xs=...;c_user=...' https://www.facebook.com/groups/GROUPID/ \
  | rg -o '"GroupsCometGroupFeedQuery","id":"(\d+)"' | head -3
```

### Thread Safety
`Client` is fully concurrent. All shared mutable state (`sessionState`, `reqCounter`)
is protected by mutex or atomics.

### No Reflection Parsing
Internal GraphQL responses are decoded into typed structs (not `map[string]interface{}`).
This improves correctness and keeps allocations low.

### Stop-Word List
A minimal English stop-word list (~200 words) is embedded as a `var` slice in `trends.go`.
No external file or embed directive needed.

---

## Out of Scope (v1)

- Marketplace listings
- Facebook Events
- Facebook Pages (as opposed to Groups)
- Messenger / inbox
- Facebook Ads API
- Video/Reels within groups
- Story reactions (separate endpoint family)

---

## Suggested Implementation Order

1. `errors.go` + `types.go` (foundation)
2. `session.go` + `facebook.go` (auth bootstrap)
3. `client.go` (HTTP + GraphQL layer)
4. `groups.go` US-2 through US-8 (read then write)
5. `feed.go` US-9, US-10
6. `posts.go` + `comments.go` US-11 through US-14
7. `members.go` US-15
8. `trends.go` US-16
9. Integration tests against live session
