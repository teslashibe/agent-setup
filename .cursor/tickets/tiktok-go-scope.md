# `tiktok-go` — TikTok Web API Scraper SDK Scope

**Repo:** `github.com/teslashibe/tiktok-go`  
**Package:** `tiktok`  
**Mirrors:** `x-go` / `linkedin-go` / `reddit-go` / `hn-go` conventions (stdlib only, zero prod deps)  
**Purpose:** Authenticated Go client for TikTok's private web API (`www.tiktok.com/api/*`) and server-side rendered page scraping. Covers FYP feed, user profiles, video detail, live search, and with X-Bogus signing: user posts, comments, search, followers/following, liked videos, social actions, and hashtags.

---

## Auth Verification

All endpoints confirmed tested with session cookies extracted from an authenticated browser session.

**Required cookies:**
```
sessionid           // primary auth session ID
sid_tt              // session token (same value as sessionid)
tt_csrf_token       // CSRF token — also used as X-Tt-Csrf-Token header
msToken             // rotating token — updated via Set-Cookie on every response
ttwid               // web ID token
odin_tt             // device fingerprint token
sid_ucp_v1          // UCP session
uid_tt              // user ID token (hashed)
```

**Required headers on all API calls:**
```
User-Agent:   Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36
Referer:      https://www.tiktok.com/{page-context}
Accept:       application/json, text/plain, */*
```

**Standard query params on all `/api/*` calls:**
```
aid=1988
app_name=tiktok_web
device_platform=web_pc
browser_language=en-US
browser_platform=MacIntel
browser_name=Mozilla
region=US
language=en
os=mac
screen_height=1080
screen_width=1920
tz_name=America/New_York
msToken={rotating}
```

**`Cookies` struct fields:**
```go
SessionID      string  // sessionid — primary auth
SIDtt          string  // sid_tt — same value as sessionid
CSRFToken      string  // tt_csrf_token — also sent as X-Tt-Csrf-Token header
MsToken        string  // msToken — rotates on every response via Set-Cookie
TTWid          string  // ttwid — web tracking ID
OdinTT         string  // odin_tt — device fingerprint
SIDUcpV1       string  // sid_ucp_v1 — UCP session
UIDtt          string  // uid_tt — hashed user ID token
```

**msToken rotation:** TikTok sends a refreshed `msToken` via `Set-Cookie: msToken=...` on virtually every API response. The client **must** track and update this value between requests.

---

## X-Bogus Signing — Critical Implementation Note

TikTok requires an `X-Bogus` query parameter on most `/api/*` endpoints. Without it, the API returns HTTP 200 with an **empty body** (no error code, no JSON). This is TikTok's primary bot-detection mechanism for API calls.

**What X-Bogus is:**
- A per-request signature appended to the URL query string as `&X-Bogus={value}`
- Derived from: full URL query string bytes + User-Agent string + some constant seeds
- Output looks like: `DFSzswVYJeS7e4sDNq1tLdMN0` (25-char alphanumeric)

**What X-Bogus is NOT:**
- Not required on the FYP/recommend endpoint (our primary no-auth-needed feed)
- Not required on live search
- Not required for HTML page scraping (SSR routes)

**Implementation approach:**
The X-Bogus algorithm has been reverse-engineered by the open-source community (used in `pyktok`, `TikTokApi` Python library). The core is a deterministic bit-manipulation function over the query string bytes using a fixed charset. A Go port of this algorithm (< 100 lines) unlocks all Tier 2 endpoints. This is the **first and most critical implementation task** before any Tier 2 endpoint work.

---

## Endpoint Tiers

### Tier 1: Works Without X-Bogus (confirmed live ✅)

These work with cookies alone — no additional signing required.

---

### FYP / For You Page Feed

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/api/recommend/item_list/` | Paginated FYP feed. Returns `itemList[]` with full video structs. `cursor` param for pagination, `hasMore` for continuation. Confirmed returning 5–35 items per call. |

**Response envelope:**
```
statusCode   int       // 0 = success
status_code  int       // same, alternate key
hasMore      bool      // pagination continuation flag
itemList     []Item    // video items
```

---

### User Profile (HTML Page Scraping)

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/@{username}` | Full user profile + stats embedded in `__UNIVERSAL_DATA_FOR_REHYDRATION__` script tag. |

**Extraction:** Parse `<script id="__UNIVERSAL_DATA_FOR_REHYDRATION__">` JSON → `__DEFAULT_SCOPE__` → `webapp.user-detail` → `userInfo`.

Confirmed fields in `userInfo.user`:
```
id, shortId, uniqueId, nickname
avatarLarger, avatarMedium, avatarThumb
signature, createTime, verified, secUid
privateAccount, secret, ftc, roomId
commentSetting, duetSetting, stitchSetting, downloadSetting
relation, openFavorite, ttSeller
isADVirtual, isEmbedBanned, canExpPlaylist
uniqueIdModifyTime, nickNameModifyTime
recommendReason, nowInvitationCardUrl
commerceUserInfo, profileTab, followingVisibility
profileEmbedPermission, language, eventList
suggestAccountBind, isOrganization, HasPromoteEntry, UserStoryStatus
```

Confirmed fields in `userInfo.stats`:
```
followerCount, followingCount, heart, heartCount
videoCount, diggCount, friendCount
```

Also available from `webapp.app-context` on **own** profile page (logged-in user):
```
user.secUid, user.uid, user.uniqueId, user.nickName
user.hasLivePermission, user.hasIMPermission, user.hasSearchPermission
user.storeRegion, user.ageGateRegion, user.analyticsOn
user.proAccountInfo.status
csrfToken
```

---

### Video Detail (HTML Page Scraping)

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/@{username}/video/{videoId}` | Full video + author + music + challenges. Embedded in `webapp.video-detail` → `itemInfo.itemStruct`. |

**Extraction:** Same `__UNIVERSAL_DATA_FOR_REHYDRATION__` approach → `__DEFAULT_SCOPE__` → `webapp.video-detail` → `itemInfo.itemStruct`.

Confirmed `itemStruct` fields (full set from live response):
```
id, desc, createTime, scheduleTime
duetEnabled, stitchEnabled, shareEnabled, forFriend
digged, collected (auth-state flags for the current viewer)
isAd, privateItem, officalItem, originalItem, secret
itemCommentStatus, isReviewing, indexEnabled
diversificationId, diversificationLabels, channelTags
textLanguage, textTranslatable
suggestedWords, videoSuggestWordsList
locationCreated, contentLocation
AIGCDescription, IsAigc, CategoryType
backendSourceEventTracking
```

---

### Live Search

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/api/search/live/full/` | Search live rooms by keyword. Works without X-Bogus. Returns `data[]` with `live_info.raw_data` (JSON-encoded live room struct). |

**Response envelope:**
```
status_code  int
data         []LiveResult
has_more     bool
cursor       string
```

**`raw_data` (parsed) fields:**
```
id, id_str, title, status
user_count (current viewers), like_count
owner.id, owner.nickname, owner.display_id, owner.sec_uid
owner.bio_description, owner.avatar_thumb/medium/large
owner.follow_info, owner.pay_grade, owner.badge_list
stats.total_user, stats.enter_count, stats.share_count, stats.comment_count
stream_url.rtmp_pull_url, stream_url.flv_pull_url
stream_url.candidate_resolution, stream_url.stream_size_width/height
stream_url.live_core_sdk_data
cover.url_list[]
hashtag.id, hashtag.title, hashtag.image
live_room_mode, age_restricted
start_time
```

---

## Tier 2: Requires X-Bogus (confirmed returning empty body without it)

Once X-Bogus signing is implemented, all of these become available.

---

### Users

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/api/user/detail/` | User detail by `uniqueId` or `secUid`. Full user struct + stats in `userInfo`. Needs X-Bogus. |
| `GET` | `/api/user/follower/list/` | Paginated follower list by `secUid`. Params: `count`, `minCursor`. Needs X-Bogus. |
| `GET` | `/api/user/following/list/` | Paginated following list by `secUid`. Params: `count`, `minCursor`. Needs X-Bogus. |

---

### Videos / Posts

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/api/post/item_list/` | Paginated user post list by `secUid`. Params: `count`, `cursor`. Needs X-Bogus. |
| `GET` | `/api/like/item_list/` | Paginated liked videos by `secUid`. Params: `count`, `cursor`. Needs X-Bogus. |
| `GET` | `/api/follow/item_list/` | Following feed (videos from accounts the viewer follows). Params: `count`. Needs X-Bogus. |
| `GET` | `/api/trending/feed/` | Trending/explore feed. Params: `count`, `region`. Needs X-Bogus. |
| `GET` | `/api/collection/item_list/` | Saved/collected videos for authenticated user. Params: `count`, `cursor`. Needs X-Bogus. |

---

### Comments

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/api/comment/list/` | Paginated comment list by `aweme_id` (video ID). Params: `count`, `cursor`. Needs X-Bogus. |
| `GET` | `/api/comment/list/reply/` | Paginated replies to a comment. Params: `comment_id`, `count`, `cursor`. Needs X-Bogus. |

---

### Search

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/api/search/general/full/` | Mixed results (videos + users + hashtags). Params: `keyword`, `count`, `cursor`. Needs X-Bogus. |
| `GET` | `/api/search/item/full/` | Video-only search. Params: `keyword`, `count`, `cursor`. Needs X-Bogus. |
| `GET` | `/api/search/user/full/` | User-only search. Params: `keyword`, `count`, `cursor`. Needs X-Bogus. |

Note: `/api/search/live/full/` works **without** X-Bogus (Tier 1).

---

### Hashtags / Challenges

| Method | Path | Notes |
|--------|------|-------|
| `GET` | `/api/challenge/detail/` | Hashtag metadata by `challengeName`. Returns challenge info + stats. Needs X-Bogus. |
| `GET` | `/api/challenge/item_list/` | Paginated videos under a hashtag by `challengeID`. Params: `count`, `cursor`. Needs X-Bogus. |

---

### Social Actions (Authenticated)

| Method | Path | Notes |
|--------|------|-------|
| `POST` | `/api/commit/item/digg/` | Like/unlike a video. Body: `aweme_id`, `type` (1=like, 0=unlike). Needs X-Bogus + CSRF. |
| `POST` | `/api/commit/follow/user/` | Follow/unfollow a user. Body: `user_id`, `type` (1=follow, 0=unfollow). Needs X-Bogus + CSRF. |
| `POST` | `/api/commit/item/collect/` | Save/collect a video. Body: `aweme_id`, `type`. Needs X-Bogus + CSRF. |

---

## Data Models

### `User`
```go
type User struct {
    ID               string
    ShortID          string
    UniqueID         string   // @handle
    Nickname         string
    Signature        string   // bio
    AvatarLarger     string
    AvatarMedium     string
    AvatarThumb      string
    CreateTime       int64
    Verified         bool
    SecUID           string   // used as cursor/ID on API calls
    PrivateAccount   bool
    Secret           bool
    FTC              bool     // "from the community"
    Relation         int      // 0=none, 1=following, 2=follower, 3=mutual
    OpenFavorite     bool
    CommentSetting   int
    DuetSetting      int
    StitchSetting    int
    DownloadSetting  int
    IsADVirtual      bool
    IsEmbedBanned    bool
    CanExpPlaylist    bool
    TtSeller         bool
    RoomID           string   // non-empty if user is currently live
    UniqueIDModifyTime     int64
    NickNameModifyTime     int64
    RecommendReason        string
    NowInvitationCardURL   string
    SuggestAccountBind     bool
    UserStoryStatus        int
}
```

### `UserStats`
```go
type UserStats struct {
    FollowerCount   int64
    FollowingCount  int64
    Heart           int64    // total likes received (may overflow int32)
    HeartCount      int64
    VideoCount      int64
    DiggCount       int64
    FriendCount     int64
}
```

### `Video` (Item)
```go
type Video struct {
    ID                string
    Desc              string
    CreateTime        int64
    ScheduleTime      int64
    Author            Author
    Music             Music
    Stats             VideoStats
    StatsV2           VideoStatsV2   // string-typed counts
    AuthorStats       UserStats
    AuthorStatsV2     UserStats
    VideoDetail       VideoDetail
    Challenges        []Challenge
    TextExtra         []TextExtra    // hashtags, @mentions in desc
    Contents          []Content      // parsed description segments
    POI               *POI           // location tag
    Digged            bool           // viewer has liked this
    Collected         bool           // viewer has saved this
    ShareEnabled      bool
    DuetEnabled       bool
    StitchEnabled     bool
    DuetDisplay       int
    StitchDisplay     int
    ForFriend         bool
    PrivateItem       bool
    IsAd              bool
    OfficalItem       bool
    OriginalItem      bool
    IsReviewing       bool
    Secret            bool
    ItemCommentStatus int
    IndexEnabled      bool
    CategoryType      int
    DiversificationID int
    TextLanguage      string
    TextTranslatable  bool
    BackendSourceEventTracking string
    SuggestedWords    []string
    VideoSuggestWordsList *SuggestWordsList
    AIGCDescription   string
    IsAIGC            bool
    ItemControl       ItemControl
    IsHDBitrate       bool
}
```

### `VideoDetail`
```go
type VideoDetail struct {
    ID              string
    Height          int
    Width           int
    Duration        int
    Ratio           string    // "720p", "1080p"
    Cover           string    // URL
    OriginCover     string
    DynamicCover    string
    PlayAddr        string    // CDN stream URL
    DownloadAddr    string
    ShareCover      []string
    ReflowCover     string
    Bitrate         int
    EncodedType     string
    Format          string
    VideoQuality    string
    EncodeUserTag   string
    CodecType       string
    Definition      string
    Size            int64
    VideoID         string
    VQScore         float64
    BitrateInfo     []BitrateInfo
    SubtitleInfos   []SubtitleInfo
    ZoomCover       map[string]string
    VolumeInfo      VolumeInfo
    PlayAddrStruct  PlayAddrStruct
    CLAInfo         CLAInfo
}
```

### `VideoStats`
```go
type VideoStats struct {
    DiggCount    int64   // likes
    ShareCount   int64
    CommentCount int64
    PlayCount    int64
    CollectCount int64   // saves (note: returned as string in statsV2)
}

type VideoStatsV2 struct {   // string-typed version returned alongside Stats
    DiggCount    string
    ShareCount   string
    CommentCount string
    PlayCount    string
    RepostCount  string
    CollectCount string
}
```

### `Author` (compact User on video objects)
```go
type Author struct {
    ID               string
    UniqueID         string
    Nickname         string
    Signature        string
    SecUID           string
    AvatarLarger     string
    AvatarMedium     string
    AvatarThumb      string
    Verified         bool
    PrivateAccount   bool
    Relation         int
    OpenFavorite     bool
    CommentSetting   int
    DuetSetting      int
    StitchSetting    int
    DownloadSetting  int
    FTC              bool
    IsADVirtual      bool
    IsEmbedBanned    bool
    RoomID           string
    UserStoryStatus  int
}
```

### `Music`
```go
type Music struct {
    ID                string
    Title             string
    AuthorName        string
    PlayURL           string
    CoverLarge        string
    CoverMedium       string
    CoverThumb        string
    Duration          int
    Original          bool    // true = original sound (not licensed track)
    Private           bool
    IsCopyrighted     bool
    IsCommerceMusic   bool
    IsUnlimitedMusic  bool
    ShootDuration     int
    ScheduleSearchTime int64
    Collected         bool
    PreciseDuration   float64
}
```

### `Challenge` (Hashtag)
```go
type Challenge struct {
    ID              string
    Title           string   // hashtag name (no #)
    Desc            string
    ProfileLarger   string
    ProfileMedium   string
    ProfileThumb    string
    CoverLarger     string
    CoverMedium     string
    CoverThumb      string
    IsCommerce      bool
}

type ChallengeStats struct {
    VideoCount int64
    ViewCount  int64
}
```

### `POI` (Location)
```go
type POI struct {
    ID               string
    Name             string
    Address          string
    City             string
    Province         string
    Country          string
    CityCode         string
    CountryCode      string
    FatherPOIID      string
    FatherPOIName    string
    TTTypeCode       string
    TTTypeNameTiny   string    // "City", "Restaurant", etc.
    TTTypeNameMedium string
    TTTypeNameSuper  string    // "Place and Address"
    Type             int
    TypeCode         string
}
```

### `LiveRoom`
```go
type LiveRoom struct {
    ID              int64
    IDStr           string
    Title           string
    Status          int       // 2 = currently live
    UserCount       int       // current concurrent viewers
    LikeCount       int64
    StartTime       int64
    Cover           string    // URL from url_list[0]
    Owner           LiveOwner
    Stats           LiveStats
    Hashtag         LiveHashtag
    StreamURL       LiveStreamURL
    LiveRoomMode    int
    AgeRestricted   bool
}

type LiveOwner struct {
    ID              string
    IDStr           string
    Nickname        string
    DisplayID       string    // @handle
    SecUID          string
    BioDescription  string
    AvatarThumb     string
    AvatarMedium    string
    AvatarLarge     string
    FollowInfo      FollowInfo
    PayGrade        PayGrade
    BadgeList       []Badge
}

type LiveStats struct {
    TotalUser    int
    EnterCount   int
    ShareCount   int
    CommentCount int
}

type LiveStreamURL struct {
    RTMPPullURL          string
    FLVPullURL           map[string]string   // resolution → URL
    CandidateResolution  []string
    StreamSizeWidth      int
    StreamSizeHeight     int
}
```

### `TextExtra` (inline entities in desc)
```go
type TextExtra struct {
    Start         int
    End           int
    Type          int     // 1=hashtag, 2=mention, 3=URL
    HashtagName   string
    HashtagID     string
    UserID        string
    UserUniqueID  string
    SecUID        string
}
```

### `ItemControl`
```go
type ItemControl struct {
    CanRepost bool
}
```

---

## Domains

| Domain | Purpose |
|--------|---------|
| `https://www.tiktok.com` | Primary: all `/api/*` + page scraping |
| `https://im-api.tiktok.com` | IM/DM messaging API |
| `https://webcast.us.tiktok.com` | Live stream WebSocket/API |
| `https://location.tiktokw.us` | Location/geo services |
| `https://mcs.tiktokw.us` | TEA analytics beacon (one-way, fire-and-forget) |
| `https://verification.tiktokw.us` | Captcha/verification |
| `https://starling.tiktokv.us` | A/B config |

---

## HTML Page Scraping Pattern

TikTok SSR pages embed a full `__UNIVERSAL_DATA_FOR_REHYDRATION__` JSON blob in every page. This is the **zero-X-Bogus** path for getting structured data.

**Script tag:**
```html
<script id="__UNIVERSAL_DATA_FOR_REHYDRATION__" type="application/json">
  {...}
</script>
```

**Navigation:**
```
Root
└── __DEFAULT_SCOPE__
    ├── webapp.app-context       // logged-in user info, csrfToken, region
    ├── webapp.biz-context       // domains, feature flags, nav config
    ├── webapp.user-detail       // /@username page → userInfo.user + userInfo.stats
    ├── webapp.video-detail      // /video/{id} page → itemInfo.itemStruct (full Video)
    ├── webapp.i18n-translation  // i18n strings
    ├── seo.abtest               // SEO video IDs: vidList[]
    └── webapp.a-b               // A/B experiment config
```

**Pages with embedded data:**
| URL | Scope Key | Data |
|-----|-----------|------|
| `/@{username}` | `webapp.user-detail` | User + Stats |
| `/@{username}/video/{id}` | `webapp.video-detail` | Full Video struct |
| `/foryou`, `/explore`, `/tag/*` | None (client-loaded) | App context only |

---

## Pagination Pattern

**Cursor-based (most endpoints):**
```
Request:  cursor=0   → first page
Response: hasMore=true, itemList=[...], (max cursor = len(items))
Request:  cursor=5   → second page (use prev count as cursor)
```

**MinCursor-based (followers/following):**
```
Request:  minCursor=0, maxCursor=0
Response: minCursor={next}, maxCursor={next}, hasMore=true
```

---

## Go Package Structure

```
tiktok-go/
├── go.mod                          // module github.com/teslashibe/tiktok-go, go 1.21, NO external deps
├── tiktok.go                       // Client, Cookies, New(), Option, baseURL const, common params
├── client.go                       // apiGET/apiPOST, pageGET, setHeaders, msToken rotation, doRequest
├── xbogus.go                       // X-Bogus signing algorithm (Go port of reverse-engineered JS)
├── types.go                        // All exported types: User, Video, Music, LiveRoom, etc.
├── feed.go                         // ForYouFeed(), ForYouPage iterator
├── profile.go                      // GetUser() via HTML scrape, GetUserByID() via API
├── video.go                        // GetVideo() via HTML scrape, GetUserVideos() via API
├── search.go                       // SearchVideos(), SearchUsers(), SearchLive() (Tier 1), SearchGeneral()
├── hashtag.go                      // GetHashtag(), GetHashtagVideos()
├── comments.go                     // GetComments(), GetReplies()
├── social.go                       // Follow/Unfollow, Like/Unlike, Collect
├── live.go                         // SearchLive(), GetLiveRoom()
├── iterator.go                     // FeedIterator, VideoIterator, CommentIterator, Checkpoint
├── errors.go                       // ErrInvalidAuth, ErrRateLimited, ErrXBogus, HTTPError
├── messaging.go                    // DM: GetConversations, GetMessages, SendMessage (Phase 5, needs X-Bogus)
├── examples/
│   ├── fyp/main.go                 // Stream FYP feed
│   ├── get_user/main.go            // Fetch user profile
│   ├── get_video/main.go           // Fetch video detail
│   ├── post_comment/main.go        // Post and reply to comments (no X-Bogus needed)
│   └── search_live/main.go         // Search live rooms
└── integration_test.go             // //go:build integration
```

---

## Implementation Phases

### Phase 1: Foundation (Tier 1 — no X-Bogus)
1. `tiktok.go` — `Client`, `Cookies`, `New`, `Option` (WithHTTPClient, WithTimeout, WithRateLimit, WithUserAgent)
2. `client.go` — `apiGET`, `pageGET`, `setHeaders`, msToken rotation (read `Set-Cookie` on every response)
3. `types.go` — `User`, `UserStats`, `Video`, `VideoDetail`, `VideoStats`, `Music`, `Author`, `Challenge`, `POI`, `TextExtra`, `ItemControl`
4. `feed.go` — `ForYouFeed(count, cursor)` → `FeedPage{Items, HasMore, Cursor}`; `FeedIterator`
5. `profile.go` — `GetUser(username)` via `/@username` HTML scrape + SIGI parser
6. `video.go` — `GetVideo(username, videoID)` via HTML scrape

### Phase 2: X-Bogus + Search
7. `xbogus.go` — Port X-Bogus signing algorithm (< 100 lines Go)
8. `search.go` — `SearchLive(keyword, count)` (Tier 1, no X-Bogus); `SearchVideos`, `SearchUsers`, `SearchGeneral` (Tier 2, with X-Bogus)
9. `live.go` — `LiveRoom`, `LiveOwner`, `LiveStats` types; `SearchLive` consolidation

### Phase 3: User Content
10. `video.go` (extend) — `GetUserVideos(secUID, count, cursor)` → user post list
11. `profile.go` (extend) — `GetFollowers(secUID, count, cursor)`, `GetFollowing(secUID, count, cursor)`, `GetLikedVideos(secUID, count, cursor)`
12. `hashtag.go` — `GetHashtag(name)`, `GetHashtagVideos(challengeID, count, cursor)`
13. `comments.go` — `GetComments(videoID, count, cursor)`, `GetReplies(commentID, count, cursor)`

### Phase 4: Social Actions + Polish
14. `social.go` — `LikeVideo`, `UnlikeVideo`, `FollowUser`, `UnfollowUser`, `CollectVideo`, `BlockUser`, `MuteUser`, `RepostVideo`
15. `iterator.go` — `FeedIterator`, `VideoIterator`, `CommentIterator` with `Checkpoint` for resuming
16. `errors.go` — All error sentinels
17. Examples + integration tests

### Phase 5: IM + Video Upload (advanced)
18. `messaging.go` — `GetConversations`, `GetMessages`, `SendDM` via `im-api.tiktok.com` REST + WebSocket listener skeleton
19. Video upload flow — `InitUpload`, chunk upload to CDN, `CommitUpload`, `CreatePost`

---

## Write Endpoints — Full Surface

All write endpoints were live-tested. The table below documents confirmed status, required parameters, and response shape.

### Tier Key
- ✅ **Works without X-Bogus** — confirmed live response
- ⚠️ **Needs X-Bogus** — returns empty body (200) without it
- 🔗 **Needs X-Bogus + Referer match** — returns `{"status_code":0,"status_msg":"url doesn't match"}` without it

All write endpoints use:
- `POST` method
- `Content-Type: application/x-www-form-urlencoded; charset=UTF-8`
- `X-Secsdk-Csrf-Version: 1.2.8`
- `X-Tt-Csrf-Token: {csrfToken}` header
- Standard query params (`aid`, `app_name`, `device_platform`, `msToken`, …) on the URL

---

### Comments ✅ (confirmed working without X-Bogus)

| Method | Path | Body Params | Notes |
|--------|------|-------------|-------|
| `POST` | `/api/comment/publish/` | `aweme_id`, `text`, `text_extra` (JSON array), `channel_id=0` | Post top-level comment. Returns full `comment` object. |
| `POST` | `/api/comment/publish/` | `aweme_id`, `text`, `text_extra`, `channel_id=0`, `comment_id` (parent), `reply_comment_id` (parent), `reply_type=2` | Reply to a comment. Same response shape. |
| `POST` | `/api/comment/delete/` | `aweme_id`, `comment_id` | Delete own comment. Needs X-Bogus (returns status_code:4 without). |
| `POST` | `/api/comment/digg/` | `aweme_id`, `comment_id`, `type` (1=like, 0=unlike) | Like/unlike a comment. Needs X-Bogus (empty body without). |
| `POST` | `/api/comment/top/` | `aweme_id`, `comment_id`, `action` (1=pin, 0=unpin) | Pin comment (video owner only). Needs Referer match. |

**`comment` response object** (confirmed from live POST):
```go
type Comment struct {
    AwemeID            string    // video this comment belongs to
    CID                string    // comment ID
    CreateTime         int64
    DiggCount          int
    Status             int       // 2 = published, 4 = under review
    Text               string
    TextExtra          []TextExtra  // parsed @mentions, hashtags
    ReplyID            string    // parent comment ID ("0" if top-level)
    ReplyToReplyID     string    // grandparent comment ID
    ReplyComment       []Comment // direct replies (empty on fresh post)
    UserDigged         int       // 0 = viewer hasn't liked, 1 = has liked
    ImageList          []Image   // nil unless image comment
    LabelList          []Label   // moderation labels (usually nil)
    CommentPostItemIDs []string  // related video IDs
    User               CommentUser
}
```

**`CommentUser`** (the `user` field on a comment — 100+ fields, superset of `Author`):

Key fields confirmed present:
```
uid, unique_id, nickname, signature, sec_uid, region, language
avatar_larger, avatar_medium, avatar_thumb, avatar_uri
verified, is_block, is_mute, follow_status, follower_status, friends_status
follower_count, following_count, aweme_count, total_favorited, favoriting_count
comment_setting, download_setting, duet_setting, stitch_setting
has_email, has_facebook_token, has_twitter_token, has_youtube_token, has_insights
twitter_id, twitter_name, ins_id, youtube_channel_id
account_labels, account_region, authority_status, custom_verify
commerce_user_level, with_commerce_entry, with_shop_entry
advance_feature_item_order, advanced_feature_info
bold_fields, cover_url, white_cover_url, video_icon
search_highlight, type_label, user_tags, user_spark_info
is_discipline_member, is_star, is_ad_fake, is_phone_binded
```

---

### Video Social Actions ⚠️ (need X-Bogus)

| Method | Path | Body Params | Response Fields | Notes |
|--------|------|-------------|-----------------|-------|
| `POST` | `/api/commit/item/digg/` | `aweme_id`, `type` (1=like, 0=unlike) | `status_code`, `digg_status` (1/0) | Like / Unlike a video |
| `POST` | `/api/commit/item/collect/` | `aweme_id`, `type` (1=save, 0=unsave) | `status_code` | Save / Unsave a video |
| `POST` | `/api/repost/` | `aweme_id` | `status_code`, `repost_item` | Repost video to own profile |
| `POST` | `/api/repost/delete/` | `aweme_id` | `status_code` | Delete own repost |

---

### User Social Actions

| Method | Path | Body Params | X-Bogus? | Notes |
|--------|------|-------------|----------|-------|
| `POST` | `/api/commit/follow/user/` | `user_id`, `type` (1=follow, 0=unfollow), `from=0`, `from_pre=0` | ⚠️ | Follow / Unfollow |
| `POST` | `/api/commit/follow/block/` | `user_id`, `type` (1=block, 0=unblock) | 🔗 | Block / Unblock |
| `POST` | `/api/commit/mute/user/` | `user_id`, `type` (1=mute, 0=unmute) | 🔗 | Mute / Unmute |

---

### Content Moderation

| Method | Path | Body Params | X-Bogus? | Notes |
|--------|------|-------------|----------|-------|
| `POST` | `/api/commit/report/aweme/` | `object_id` (video ID), `reason_id`, `type=1`, `report_type=1` | 🔗 | Report video |
| `POST` | `/api/commit/report/user/` | `object_id` (user ID), `reason_id`, `type=2` | 🔗 | Report user |

---

### Own Post Management ⚠️ / 🔗

| Method | Path | Body Params | X-Bogus? | Notes |
|--------|------|-------------|----------|-------|
| `POST` | `/api/post/delete/` | `aweme_id`, `channel_id=0` | 🔗 | Delete own video post |

---

### Video Upload (Multi-step, X-Bogus required)

TikTok video upload is a multi-step flow:

1. **Init upload** — `POST /api/post/upload/init/` — returns an upload URL + upload token
2. **Upload chunks** — `PUT` to signed CDN URL (Akamai/ByteDance CDN) in chunks
3. **Commit upload** — `POST /api/post/upload/commit/` with upload token + video metadata
4. **Create post** — `POST /api/post/create/` with `video_id`, `caption`, `hashtag_ids[]`, `poi_id`, privacy settings

All steps require X-Bogus. The upload CDN domain is separate from `www.tiktok.com` (typically `upload.tiktokcdn.com` or a signed regional URL). Full implementation requires X-Bogus before this can be tested end-to-end.

---

### Direct Messages / IM

IM uses a separate domain (`im-api.tiktok.com`) with its own REST API, plus a WebSocket connection for real-time delivery.

| Domain | Endpoint | Notes |
|--------|----------|-------|
| `https://im-api.tiktok.com` | REST API for conversations & messages | Confirmed domain from page source. Specific paths need X-Bogus to activate — 404s on cold probe. |
| `wss://im-ws.tiktok.com/ws/v2` | WebSocket for real-time DM delivery | Used for push notifications of new messages. |
| `https://www.tiktok.com/api/im/get_conversations/` | Conversation list | Needs X-Bogus (returns "url doesn't match"). |
| `https://www.tiktok.com/api/im/message_list/` | Messages in a conversation | Needs X-Bogus. |
| `https://www.tiktok.com/api/im/msg/send/` | Send a DM | Needs X-Bogus. |
| `https://www.tiktok.com/api/im/unread_count/` | Unread DM count | Needs X-Bogus. |

**IM request format (POST body likely JSON, not form-encoded):**
```go
// Inferred from IM API patterns (requires X-Bogus to confirm live)
type SendMessageRequest struct {
    ReceiverID  string      `json:"receiver_id"`
    ContentType int         `json:"type"`         // 1 = text
    Content     IMContent   `json:"content"`
}

type IMContent struct {
    Text string `json:"text"`
}
```

---

### Notifications

| Method | Path | Body Params | X-Bogus? | Notes |
|--------|------|-------------|----------|-------|
| `GET` | `/api/notice/detail/` | `count`, `cursor`, notice type filter | ⚠️ | Fetch notification list |
| `POST` | `/api/notice/mark_read/` | `notice_ids` (or `all`), `notice_type` | 🔗 | Mark notifications read |

---

## Write Error Response Shape

| Pattern | Example | Meaning |
|---------|---------|---------|
| Empty body (0 bytes) | `HTTP 200, body: ""` | Missing X-Bogus signature |
| `{"status_code":0,"status_msg":"url doesn't match"}` | collect, block, mute | X-Bogus or Referer validation failed |
| `{"status_code":4,"status_msg":"Server is currently unavailable..."}` | delete comment without X-Bogus | X-Bogus required |
| `{"status_code":0,"status_msg":"Comment sent successfully","comment":{...}}` | POST comment ✅ | Success |

---

## Rate Limiting

TikTok does not return `X-Ratelimit-*` headers. Behavior observed:
- API calls work immediately at low frequency
- Aggressive polling triggers captcha/bot detection (not studied at rate limit threshold)
- Recommended: 500ms–1s minimum gap between authenticated API calls
- `msToken` rotation is mandatory — stale msToken likely accelerates bans

Implement a `MinRequestGap` option (default 500ms) similar to `reddit-go`.

---

## Key Implementation Notes

1. **msToken must rotate:** Every response sets a new `Set-Cookie: msToken=...`. The client must update its internal msToken and use the latest value on the next request. Failure to do this will result in auth errors within minutes.

2. **secUid vs userId:** Most `/api/*` endpoints prefer `secUid` (the long opaque string). `userId` (numeric) is available but less commonly accepted. Always prefer `secUid` obtained from `/@username` page scrape.

3. **FYP is infinite:** The `/api/recommend/item_list/` endpoint works indefinitely with sequential cursors (0, 5, 10, ...). `hasMore` is always `true`. Implement as an unbounded iterator.

4. **Video playAddr is CDN-signed:** The `playAddr` URLs contain temporary signatures (`x-expires`, `x-signature`). They are valid for a limited window (typically 1 hour). Do not cache raw URLs.

5. **StatsV2 strings:** TikTok returns stats in both `stats` (int) and `statsV2` (string) formats. Model both. Use int where possible; parse strings as int64 with fallback.

6. **Heart vs HeartCount overflow:** `UserStats.heart` is a raw int that can exceed int32 max (e.g., Khaby Lame: 2,600,000,000). Use `int64` throughout for all count fields.

7. **Live search pagination:** `cursor` is a string token (not an integer offset) in live search responses.

8. **X-Bogus on POST:** Social action endpoints (like, follow, collect, block, mute) require X-Bogus on the URL query string, not in the POST body.

9. **Comment write is a Tier 1 action:** `POST /api/comment/publish/` and replies both work without X-Bogus. The response includes a 100+ field `user` object — parse carefully. Delete/like comment do require X-Bogus.

10. **Three distinct write failure modes:** (a) empty body = missing X-Bogus, (b) `"url doesn't match"` = Referer or X-Bogus URL validation failed, (c) `status_code: 4` = endpoint reached but auth check failed. Each requires different remediation.

11. **IM is a separate domain:** `im-api.tiktok.com` for REST + `wss://im-ws.tiktok.com/ws/v2` for real-time. IM endpoints on `www.tiktok.com/api/im/*` are proxies that need X-Bogus. Implement IM as a Phase 5 item after X-Bogus is stable.

12. **Video upload is multi-step:** init → chunk upload to CDN → commit → create post. Requires X-Bogus at each step. The CDN upload URL is signed and short-lived. Treat as a Phase 5 feature.
