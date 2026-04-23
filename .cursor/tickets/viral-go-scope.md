# `x-viral-go` — X Algorithm Viral Scorer Scope

**Repo:** `github.com/teslashibe/x-viral-go`  
**Package:** `github.com/teslashibe/x-viral-go`  
**Mirrors:** `x-go` / `linkedin-go` / `reddit-go` conventions (stdlib only, zero prod deps)  
**Depends on:** `x-go` types (`Tweet`, `User`) are accepted as inputs but the dependency is optional — `viral-go` defines its own `Post` input type and accepts raw text  
**Purpose:** Go implementation of the X algorithm's weighted scoring formula. Scores draft posts for viral potential, provides structured feedback, and integrates with LLMs to iteratively rewrite posts for maximum algorithmic reach. An optimized Go equivalent of [willitgoviral.xyz](https://willitgoviral.xyz/) with LLM-in-the-loop iteration.

---

## Background & Algorithm Research

### Source Material

This package is derived from two sources:

1. **[xai-org/x-algorithm](https://github.com/xai-org/x-algorithm)** — X's open-source recommendation system (Rust + Python). The `home-mixer/scorers/weighted_scorer.rs` file defines the exact scoring formula: a weighted sum of 19 predicted engagement probabilities. The actual weight values are in a `params` module that was **excluded from the open-source release** ("Excluded from open source release for security reasons").

2. **Public algorithm analysis** — Engagement weight ratios derived from X's documentation, [AutoTweet's 2026 algorithm breakdown](https://www.autotweet.io/blog/x-algorithm-explained-2026), and empirical testing by growth researchers.

### The Scoring Formula (from `weighted_scorer.rs`)

X ranks every post using this formula:

```
Final Score = Σ (weight_i × P(action_i)) + offset
```

Where the 19 predicted actions and their **approximate relative weights** are:

| Action | Signal Name | Direction | Approx. Relative Weight |
|---|---|---|---|
| Reply | `reply_score` | Positive | 27.0 (highest) |
| Quote Tweet | `quote_score` | Positive | 13.0 |
| Retweet | `retweet_score` | Positive | 4.0 |
| Like/Favorite | `favorite_score` | Positive | 1.0 (baseline) |
| Share (generic) | `share_score` | Positive | 3.0 |
| Share via DM | `share_via_dm_score` | Positive | 2.5 |
| Share via Copy Link | `share_via_copy_link_score` | Positive | 2.0 |
| Video Quality View | `vqv_score` | Positive | 1.5 (video only) |
| Dwell (binary) | `dwell_score` | Positive | 1.0 |
| Dwell Time (continuous) | `dwell_time` | Positive | 0.5 |
| Click | `click_score` | Positive | 0.5 |
| Profile Click | `profile_click_score` | Positive | 0.5 |
| Photo Expand | `photo_expand_score` | Positive | 0.3 |
| Quoted Tweet Click | `quoted_click_score` | Positive | 0.3 |
| Follow Author | `follow_author_score` | Positive | 1.0 |
| Not Interested | `not_interested_score` | **Negative** | -74.0 |
| Block Author | `block_author_score` | **Negative** | -74.0 |
| Mute Author | `mute_author_score` | **Negative** | -74.0 |
| Report | `report_score` | **Negative** | -369.0 |

> **Note:** The relative weights above are approximations reconstructed from public research. The actual `params` module values are not published. These weights are configurable in `viral-go` via `WithWeights()`.

### What This Package Does Differently

The actual X algorithm runs a **Grok-based transformer** (Phoenix) to predict `P(action)` for each action type given a user's engagement history and the candidate post. That model is not available outside X's infrastructure.

Instead, `viral-go` takes the **author's perspective**: given a draft post, it estimates how algorithmically favorable the post is by analyzing the **content signals** that correlate with high `P(action)` values. This is the same approach used by [willitgoviral.xyz](https://willitgoviral.xyz/), but implemented as a composable Go library with LLM-in-the-loop optimization.

### Key Content Signals (2026)

From the algorithm source and public analysis, these content properties affect scoring:

| Category | Signal | Impact |
|---|---|---|
| **Engagement Triggers** | Questions / calls to action | ↑ P(reply) — the highest-weight signal |
| **Engagement Triggers** | Controversial/hot takes | ↑ P(reply), P(quote) |
| **Engagement Triggers** | Personal stories / vulnerability | ↑ P(reply), P(favorite) |
| **Format** | Thread hooks ("🧵", "A thread:") | ↑ P(dwell_time), P(click) |
| **Format** | Line breaks / whitespace structure | ↑ P(dwell), P(photo_expand) |
| **Format** | Optimal length (100–280 chars for punchy, 1000+ for dwell-time) | ↑ P(dwell_time) |
| **Format** | Native media (image/video mentioned) | ↑ P(photo_expand), P(vqv) |
| **Negative** | External links (http/https URLs) | ↓↓ Distribution penalty (40–70% reduction) |
| **Negative** | Hashtag stuffing (>3 hashtags) | ↓ Reduced NLP relevance |
| **Negative** | All-caps text (>30% uppercase) | ↑ P(not_interested), P(mute_author) |
| **Negative** | Spammy patterns (excessive emojis, "follow for follow") | ↑ P(report), P(block_author) |
| **Author** | Blue verified / Premium subscriber | 2–4x distribution boost |
| **Author** | Follower-to-following ratio >2:1 | Higher baseline authority |
| **Author** | Niche consistency | Better topic-audience matching |

---

## File Structure

```
viral-go/                                ← repo root
├── go.mod                               # module github.com/teslashibe/viral-go
├── doc.go                               # package viral — godoc overview
├── viral.go                             # Scorer, New(), Option funcs, default weights
├── score.go                             # Score(), ScorePost() — core scoring logic
├── signals.go                           # Signal extraction (content analysis, NLP heuristics)
├── weights.go                           # DefaultWeights, WeightSet, action weight constants
├── feedback.go                          # Feedback generation — structured improvement suggestions
├── optimize.go                          # Optimize() — LLM-in-the-loop iteration
├── llm.go                               # LLMProvider interface, prompt construction
├── types.go                             # All exported domain types
├── errors.go                            # Sentinel errors
├── integration_test.go                  # //go:build integration
└── README.md                            # Usage examples
```

---

## Domain Types (`types.go`)

```go
// Post is the input to the scorer. Either Text is required (for draft scoring)
// or a full post with engagement data (for retrospective analysis).
type Post struct {
    Text             string   `json:"text"`
    MediaType        Media    `json:"mediaType,omitempty"`        // None, Image, Video, Poll
    HasExternalLink  bool     `json:"hasExternalLink,omitempty"`
    IsReply          bool     `json:"isReply,omitempty"`
    IsQuote          bool     `json:"isQuote,omitempty"`
    IsThread         bool     `json:"isThread,omitempty"`
    Hashtags         []string `json:"hashtags,omitempty"`
    MentionedUsers   []string `json:"mentionedUsers,omitempty"`
}

// Media represents the type of media attached to a post.
type Media int

const (
    MediaNone  Media = iota
    MediaImage
    MediaVideo
    MediaPoll
    MediaGIF
)

// AuthorProfile provides optional author context that affects scoring.
type AuthorProfile struct {
    FollowersCount  int  `json:"followersCount"`
    FollowingCount  int  `json:"followingCount"`
    IsVerified      bool `json:"isVerified"`
    IsPremium       bool `json:"isPremium"`
    TweetCount      int  `json:"tweetCount,omitempty"`
    AccountAgeDays  int  `json:"accountAgeDays,omitempty"`
}

// ScoreResult is the output of scoring a single post.
type ScoreResult struct {
    Score           int              `json:"score"`            // 0–100 normalized viral score
    RawScore        float64          `json:"rawScore"`         // weighted sum before normalization
    ActionScores    ActionScores     `json:"actionScores"`     // per-action estimated probability
    Signals         []Signal         `json:"signals"`          // detected content signals
    Feedback        []Suggestion     `json:"feedback"`         // actionable improvement suggestions
    PositiveFactors []string         `json:"positiveFactors"`  // what's working
    NegativeFactors []string         `json:"negativeFactors"`  // what's hurting
}

// ActionScores holds estimated engagement probabilities for each action type.
// These are heuristic estimates, not ML predictions.
type ActionScores struct {
    Reply           float64 `json:"reply"`
    Quote           float64 `json:"quote"`
    Retweet         float64 `json:"retweet"`
    Favorite        float64 `json:"favorite"`
    Share           float64 `json:"share"`
    ShareViaDM      float64 `json:"shareViaDM"`
    ShareViaCopyLink float64 `json:"shareViaCopyLink"`
    VideoQualityView float64 `json:"videoQualityView"`
    Dwell           float64 `json:"dwell"`
    DwellTime       float64 `json:"dwellTime"`
    Click           float64 `json:"click"`
    ProfileClick    float64 `json:"profileClick"`
    PhotoExpand     float64 `json:"photoExpand"`
    QuotedClick     float64 `json:"quotedClick"`
    FollowAuthor    float64 `json:"followAuthor"`
    NotInterested   float64 `json:"notInterested"`
    BlockAuthor     float64 `json:"blockAuthor"`
    MuteAuthor      float64 `json:"muteAuthor"`
    Report          float64 `json:"report"`
}

// Signal is a detected content signal that affects scoring.
type Signal struct {
    Name        string  `json:"name"`        // e.g. "question_hook", "external_link"
    Category    string  `json:"category"`    // "engagement", "format", "negative", "author"
    Impact      float64 `json:"impact"`      // contribution to final score (can be negative)
    Description string  `json:"description"` // human-readable explanation
}

// Suggestion is a specific, actionable improvement recommendation.
type Suggestion struct {
    Priority    int    `json:"priority"`    // 1 = highest priority
    Category    string `json:"category"`    // matches Signal categories
    Action      string `json:"action"`      // what to do: "add", "remove", "rewrite", "restructure"
    Description string `json:"description"` // full suggestion text
    Impact      string `json:"impact"`      // "high", "medium", "low"
}

// OptimizeResult is the output of LLM-assisted optimization.
type OptimizeResult struct {
    Original      ScoreResult   `json:"original"`
    Iterations    []Iteration   `json:"iterations"`
    Best          Iteration     `json:"best"`
    TotalTokens   int           `json:"totalTokens,omitempty"`
}

// Iteration represents one LLM rewrite attempt.
type Iteration struct {
    Number      int         `json:"number"`
    Text        string      `json:"text"`      // the rewritten post text
    Score       ScoreResult `json:"score"`      // score of this iteration
    Reasoning   string      `json:"reasoning"`  // LLM's explanation of changes
}
```

---

## API Surface (`viral.go`, all exported methods on `*Scorer`)

```go
// Construction — import "github.com/teslashibe/viral-go"
func New(opts ...Option) *Scorer
// New creates a scorer with default weights and configuration.
// No auth required — this is a pure computation package.

// Scoring
func (s *Scorer) Score(text string) ScoreResult
// Score evaluates a plain text post and returns a viral score (0–100) with feedback.

func (s *Scorer) ScorePost(post Post) ScoreResult
// ScorePost evaluates a structured Post with media/format metadata for more accurate scoring.

func (s *Scorer) ScoreWithAuthor(post Post, author AuthorProfile) ScoreResult
// ScoreWithAuthor factors in author authority signals for a more complete score.

func (s *Scorer) ScoreBatch(posts []Post) []ScoreResult
// ScoreBatch scores multiple posts, useful for comparing draft variations.

// LLM-Assisted Optimization
func (s *Scorer) Optimize(ctx context.Context, text string, opts ...OptimizeOption) (*OptimizeResult, error)
// Optimize rewrites the post iteratively using an LLM to maximize viral score.
// Requires an LLMProvider to be configured via WithLLMProvider().

func (s *Scorer) OptimizePost(ctx context.Context, post Post, opts ...OptimizeOption) (*OptimizeResult, error)
// OptimizePost is like Optimize but with structured Post metadata.

// Prompt Generation (for external LLM use)
func (s *Scorer) GeneratePrompt(result ScoreResult) string
// GeneratePrompt produces a structured LLM prompt (like willitgoviral.xyz's "Copy Prompt"
// feature) that can be pasted into ChatGPT/Claude/Grok for manual optimization.
```

### Options

```go
// Scorer options
func WithWeights(w WeightSet) Option                   // override default action weights
func WithLLMProvider(p LLMProvider) Option              // required for Optimize()
func WithScoreNormalizer(fn func(float64) int) Option   // custom 0–100 normalization

// Optimize options
type OptimizeOption func(*optimizeOptions)
func WithMaxIterations(n int) OptimizeOption            // default 3
func WithTargetScore(score int) OptimizeOption          // stop early if score >= target (default 80)
func WithTone(tone string) OptimizeOption               // "professional", "casual", "provocative", etc.
func WithAudience(audience string) OptimizeOption       // target audience description
func WithConstraints(constraints ...string) OptimizeOption // e.g. "keep under 280 chars", "include @handle"
func WithPreserveIntent(preserve bool) OptimizeOption   // keep core message (default true)
```

---

## LLM Provider Interface (`llm.go`)

```go
// LLMProvider is the interface for LLM backends used by Optimize().
// Callers implement this to plug in their preferred LLM.
type LLMProvider interface {
    Complete(ctx context.Context, prompt string) (string, error)
}

// PromptBuilder constructs the system + user prompts for each optimization iteration.
// The prompt includes:
//   - The X algorithm scoring formula and weights
//   - The current post text and its score breakdown
//   - Specific signals detected and their impact
//   - The feedback/suggestions from the scorer
//   - Constraints from OptimizeOptions (tone, audience, etc.)
//   - Instruction to return JSON: {"text": "...", "reasoning": "..."}
```

This interface is intentionally minimal — it takes a prompt string and returns a completion string. This lets callers use any LLM backend:

```go
// Example: OpenAI
type openaiProvider struct { client *openai.Client }
func (o *openaiProvider) Complete(ctx context.Context, prompt string) (string, error) { ... }

// Example: Anthropic
type claudeProvider struct { client *anthropic.Client }
func (c *claudeProvider) Complete(ctx context.Context, prompt string) (string, error) { ... }

// Example: Ollama (local)
type ollamaProvider struct { endpoint string }
func (o *ollamaProvider) Complete(ctx context.Context, prompt string) (string, error) { ... }
```

---

## Scoring Engine (`signals.go` + `score.go`)

### Signal Detection

The scoring engine analyzes post text for content signals that correlate with high/low engagement. All analysis is heuristic/rule-based (no ML inference, no API calls):

| Signal | Detection Method | Affects Action |
|---|---|---|
| `question_hook` | Ends with `?`, contains question words | ↑ reply (+27x weight) |
| `call_to_action` | "reply", "let me know", "thoughts?", "what do you think" | ↑ reply |
| `controversial_take` | "unpopular opinion", "hot take", "controversial" | ↑ reply, quote |
| `personal_story` | First-person narrative markers ("I learned", "My experience") | ↑ reply, favorite |
| `thread_marker` | "🧵", "A thread:", "Thread:", numbered lists | ↑ dwell_time, click |
| `whitespace_structure` | Line breaks creating visual hierarchy | ↑ dwell |
| `optimal_length_short` | 100–280 chars | ↑ retweet (shareability) |
| `optimal_length_long` | >800 chars | ↑ dwell_time |
| `has_image` | MediaType == Image | ↑ photo_expand (2–3x engagement) |
| `has_video` | MediaType == Video | ↑ vqv (10x engagement) |
| `has_poll` | MediaType == Poll | ↑ reply |
| `has_numbers` | Statistics, data points ("87% of...", "3 reasons") | ↑ retweet, quote |
| `listicle_format` | Numbered items (1., 2., 3.) | ↑ dwell_time, share |
| `strong_opener` | First sentence is a hook (bold claim, surprising stat) | ↑ click, dwell |
| `external_link` | Contains http:// or https:// URLs | ↓↓ distribution penalty |
| `hashtag_stuffing` | >3 hashtags | ↓ NLP relevance |
| `excessive_caps` | >30% uppercase characters | ↑ not_interested, mute |
| `spam_patterns` | "follow for follow", "like and retweet", engagement bait | ↑ report, block |
| `excessive_emojis` | >5 emojis or >20% emoji density | ↑ not_interested |
| `empty_calories` | Generic motivational quotes, "GM" posts | Low engagement signal |
| `mention_heavy` | >3 @mentions | ↑ mute_author |
| `premium_boost` | Author.IsPremium == true | 2–4x distribution multiplier |
| `authority_ratio` | Followers/Following >2:1 | Higher baseline score |

### Score Computation

```
1. Extract signals from post text + metadata
2. For each of the 19 action types, estimate P(action) based on detected signals
3. Compute weighted sum: raw_score = Σ (weight_i × estimated_P(action_i))
4. Normalize to 0–100 scale
5. Generate feedback from signals with negative impact or missed opportunities
```

---

## Weight Configuration (`weights.go`)

```go
// WeightSet defines the weights for each action in the scoring formula.
// These mirror the structure in x-algorithm's home-mixer/scorers/weighted_scorer.rs.
type WeightSet struct {
    Favorite        float64 `json:"favorite"`
    Reply           float64 `json:"reply"`
    Retweet         float64 `json:"retweet"`
    PhotoExpand     float64 `json:"photoExpand"`
    Click           float64 `json:"click"`
    ProfileClick    float64 `json:"profileClick"`
    VideoQualityView float64 `json:"videoQualityView"`
    Share           float64 `json:"share"`
    ShareViaDM      float64 `json:"shareViaDM"`
    ShareViaCopyLink float64 `json:"shareViaCopyLink"`
    Dwell           float64 `json:"dwell"`
    Quote           float64 `json:"quote"`
    QuotedClick     float64 `json:"quotedClick"`
    DwellTime       float64 `json:"dwellTime"`
    FollowAuthor    float64 `json:"followAuthor"`
    NotInterested   float64 `json:"notInterested"`
    BlockAuthor     float64 `json:"blockAuthor"`
    MuteAuthor      float64 `json:"muteAuthor"`
    Report          float64 `json:"report"`
}

// DefaultWeights returns the best-estimate weights derived from public analysis
// of X's algorithm. These approximate the values in the excluded params module
// of xai-org/x-algorithm.
func DefaultWeights() WeightSet

// WeightsSum returns the sum of all positive weights (used for normalization).
func (w WeightSet) WeightsSum() float64

// NegativeWeightsSum returns the absolute sum of all negative weights.
func (w WeightSet) NegativeWeightsSum() float64
```

---

## Sentinel Errors (`errors.go`)

```go
var (
    ErrEmptyText       = errors.New("viral: post text is empty")
    ErrNoLLMProvider   = errors.New("viral: LLMProvider required for Optimize — use WithLLMProvider()")
    ErrLLMFailed       = errors.New("viral: LLM completion failed")
    ErrLLMParseFailed  = errors.New("viral: failed to parse LLM response as JSON")
    ErrMaxIterations   = errors.New("viral: reached max iterations without hitting target score")
    ErrContextCanceled = errors.New("viral: context canceled during optimization")
)
```

---

## User Stories & Acceptance Criteria

### US-1 · Score Plain Text Post
**As a GTM agent, I can score a draft post by text alone so I can quickly gauge viral potential before posting.**

- AC-1.1: `scorer.Score("Hello world")` returns a `ScoreResult` with `Score` in 0–100.
- AC-1.2: A well-crafted post with a question hook, personal story, and optimal length scores ≥70.
- AC-1.3: A post containing only "GM" or empty text scores ≤20.
- AC-1.4: `ScoreResult.Signals` lists every detected signal with its name, category, and impact.
- AC-1.5: `ScoreResult.Feedback` contains at least one `Suggestion` for any post scoring below 80.
- AC-1.6: `ScoreResult.PositiveFactors` and `NegativeFactors` are non-empty human-readable lists.
- AC-1.7: Empty text returns a `ScoreResult` with `Score == 0` (not an error — returns feedback suggesting adding content).

---

### US-2 · Score Structured Post with Metadata
**As a GTM agent, I can score a post with media type and format metadata so the scorer can account for visual content and format signals.**

- AC-2.1: `ScorePost(Post{Text: "...", MediaType: MediaVideo})` boosts the score via the `vqv_score` weight.
- AC-2.2: `Post{HasExternalLink: true}` applies the link penalty and surfaces it in `NegativeFactors`.
- AC-2.3: `Post{IsThread: true}` boosts dwell-time signals.
- AC-2.4: `Post{Hashtags: []string{"go", "golang", "dev", "code", "tech"}}` triggers `hashtag_stuffing` signal.
- AC-2.5: All `ActionScores` fields are populated with estimated probabilities in [0.0, 1.0].

---

### US-3 · Score with Author Context
**As a GTM agent, I can include author profile data so the scorer factors in authority and verification signals.**

- AC-3.1: `ScoreWithAuthor(post, AuthorProfile{IsPremium: true})` boosts the normalized score by the premium multiplier.
- AC-3.2: `AuthorProfile{FollowersCount: 10000, FollowingCount: 500}` (ratio 20:1) boosts the baseline.
- AC-3.3: `AuthorProfile{FollowersCount: 100, FollowingCount: 5000}` (ratio 0.02:1) does not boost.
- AC-3.4: Author signals appear in `ScoreResult.Signals` with category `"author"`.

---

### US-4 · Batch Scoring
**As a GTM agent, I can score multiple draft variations in one call so I can compare them side-by-side.**

- AC-4.1: `ScoreBatch([]Post{a, b, c})` returns `[]ScoreResult` of length 3.
- AC-4.2: Results are independent — scoring post A does not affect post B's score.
- AC-4.3: Posts are scored in order; indices correspond.

---

### US-5 · Custom Weights
**As a developer, I can override the default action weights so I can tune the scorer to match observed algorithm changes.**

- AC-5.1: `New(WithWeights(customWeights))` uses the provided weights instead of defaults.
- AC-5.2: Partial overrides (changing only `Reply` weight) are supported.
- AC-5.3: `DefaultWeights()` returns a copy of the built-in weights for inspection or modification.

---

### US-6 · LLM-Assisted Optimization
**As a GTM agent, I can have an LLM iteratively rewrite my post to maximize its viral score, so I get an optimized version ready to publish.**

- AC-6.1: `Optimize(ctx, text)` returns an `*OptimizeResult` with the original score and at least one iteration.
- AC-6.2: Each `Iteration` has the rewritten text, its score, and the LLM's reasoning.
- AC-6.3: `OptimizeResult.Best` is the iteration with the highest score.
- AC-6.4: `WithMaxIterations(5)` stops after 5 rewrites.
- AC-6.5: `WithTargetScore(90)` stops early when an iteration hits score ≥90.
- AC-6.6: `WithTone("professional")` constrains the LLM to maintain a professional voice.
- AC-6.7: `WithAudience("Go developers")` instructs the LLM to tailor content.
- AC-6.8: `WithConstraints("keep under 280 chars")` enforces hard constraints.
- AC-6.9: `WithPreserveIntent(true)` (default) ensures the core message survives rewrites.
- AC-6.10: Returns `ErrNoLLMProvider` if no provider is configured.
- AC-6.11: Respects `ctx` cancellation; returns partial result with `ErrContextCanceled`.
- AC-6.12: Each iteration's prompt includes the full score breakdown, detected signals, and feedback from the previous iteration.

---

### US-7 · Generate LLM Prompt
**As a GTM agent, I can generate a standalone LLM prompt from a score result so I can paste it into any chat interface (like willitgoviral.xyz's "Copy Prompt" feature).**

- AC-7.1: `GeneratePrompt(result)` returns a formatted string that includes: the post text, score, all signals, suggestions, and clear instructions for the LLM to rewrite the post.
- AC-7.2: The prompt is self-contained — it can be pasted into ChatGPT, Claude, or Grok with no additional context.
- AC-7.3: The prompt instructs the LLM to explain what changes it made and why.

---

### US-8 · Optimize Structured Post
**As a GTM agent, I can optimize a structured post with media/format metadata so the LLM is aware of the full context.**

- AC-8.1: `OptimizePost(ctx, Post{Text: "...", MediaType: MediaImage})` includes media context in the LLM prompt.
- AC-8.2: The LLM prompt tells the model not to suggest adding media if media is already present.
- AC-8.3: Thread-format posts get thread-aware optimization.

---

## Design Decisions

### 1. No External Dependencies

Like all `teslashibe` packages, `viral-go` uses only the Go standard library. The `LLMProvider` interface is caller-implemented — no OpenAI/Anthropic SDK dependency.

### 2. No ML Inference

The scoring is entirely heuristic/rule-based. We do NOT replicate the Grok transformer or any neural network. The signals are extracted using string matching, regex, and simple NLP heuristics (word counting, readability scoring). This keeps the package fast, deterministic, and portable.

### 3. Approximate Weights are Configurable

Since the actual X algorithm weights are not published, the defaults are best-effort approximations. `WithWeights()` lets callers tune them as new information emerges or the algorithm changes.

### 4. Scorer is Stateless and Concurrent

`Scorer` holds only configuration (weights, provider). `Score()` and `ScorePost()` are pure functions with no side effects. `Optimize()` is the only method that performs I/O (LLM calls) and requires a context.

### 5. Score Normalization Matches X's Approach

From `weighted_scorer.rs`, X's normalization handles negative scores by offsetting with the negative weights sum divided by total weights sum. The package replicates this logic with a configurable normalizer.

---

## Scoring Examples

### High-Scoring Post (expected ≥80)
```
I spent 6 months building a Go package that scores X posts using the actual algorithm weights.

Here's what I learned about what makes content go viral 🧵

1/ Replies are weighted 27x more than likes. If your post doesn't invite a response, you're leaving 90% of the algorithm's attention on the table.

What's the most counterintuitive thing you've learned about social media algorithms?
```

**Why it scores well:**
- Personal story opener ("I spent 6 months...") → ↑ P(reply), P(favorite)
- Thread marker ("🧵") → ↑ P(dwell_time), P(click)
- Specific data point ("27x more") → ↑ P(retweet), P(quote)
- Numbered format → ↑ P(dwell)
- Question hook at the end → ↑ P(reply) (highest-weight action)
- No external links → no penalty
- Optimal length (>280 chars, dwell-time territory)

### Low-Scoring Post (expected ≤25)
```
Check out my new blog post! https://myblog.com/post #tech #startup #coding #buildinpublic #growth
```

**Why it scores poorly:**
- External link → ↓↓ 40–70% distribution penalty
- Hashtag stuffing (5 hashtags) → ↓ NLP relevance
- No question/CTA → no reply signal
- Short, low-substance text → minimal dwell
- Engagement bait pattern ("Check out") → ↑ P(not_interested)

---

## Integration with `x-go`

While `viral-go` is a standalone package, it's designed to complement `x-go`:

```go
import (
    x "github.com/teslashibe/x-go"
    "github.com/teslashibe/viral-go"
)

// Score a draft before posting
scorer := viral.New()
result := scorer.Score("My draft post text...")

// Optimize with LLM
optimized, _ := scorer.Optimize(ctx, "My draft post text...",
    viral.WithTargetScore(85),
    viral.WithTone("professional"),
)

// Post the best version via x-go
client, _ := x.New(cookies)
tweet, _ := client.CreateTweet(ctx, optimized.Best.Text)

// Score an existing tweet from x-go (retrospective)
existingTweet, _ := client.GetTweet(ctx, tweetID)
post := viral.Post{
    Text:      existingTweet.Text,
    Hashtags:  existingTweet.Hashtags,
    IsReply:   existingTweet.IsReply,
    IsQuote:   existingTweet.IsQuote,
}
result = scorer.ScorePost(post)
```

---

## Out of Scope (v1)

- Real-time engagement data analysis (would require x-go read surface)
- A/B testing framework
- Scheduled posting
- Historical score tracking / analytics
- Multi-platform scoring (LinkedIn, Facebook, Reddit — future packages)
- Embedding-based semantic analysis (would add ML dependencies)
- Rate limiting or API calls for scoring (scoring is local computation only)

---

## Suggested Implementation Order

1. `errors.go` + `types.go` (foundation types)
2. `weights.go` (weight constants and `WeightSet`)
3. `signals.go` (signal detection — the core NLP/heuristic engine)
4. `score.go` (scoring pipeline — signal → action estimates → weighted sum → normalize)
5. `feedback.go` (suggestion generation from signals)
6. `viral.go` (Scorer construction, `Score`, `ScorePost`, `ScoreWithAuthor`, `ScoreBatch`)
7. `llm.go` (LLMProvider interface, prompt construction)
8. `optimize.go` (`Optimize`, `OptimizePost` — LLM iteration loop)
9. `doc.go` + `README.md`
10. Integration tests with real LLM provider
