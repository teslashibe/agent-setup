# `redditviral-go` — Reddit Algorithm Viral Scorer Scope

**Repo:** `github.com/teslashibe/redditviral-go`  
**Package:** `github.com/teslashibe/redditviral-go`  
**Mirrors:** `viral-go` patterns (stdlib only, zero prod deps, LLM-in-the-loop)  
**Purpose:** Go implementation of Reddit's ranking algorithms (Hot, Best, Controversial, Rising). Scores draft posts/comments for viral potential, provides structured feedback, and integrates with LLMs to iteratively rewrite for maximum reach. The Reddit equivalent of `viral-go`.

---

## Background & Algorithm Research

### Source Material

1. **[reddit-archive/reddit `_sorts.pyx`](https://github.com/reddit-archive/reddit/blob/master/r2/r2/lib/db/_sorts.pyx)** — Reddit's actual open-source ranking formulas (Hot, Confidence/Best, Controversial, Rising, Q&A).
2. **Public algorithm analysis (2026)** — Engagement velocity research, subreddit-specific factors, time decay mechanics.

### The Four Ranking Formulas (from `_sorts.pyx`)

#### Hot Formula (default sort — gatekeeper for all organic traffic)

```python
score = ups - downs
order = log10(max(abs(score), 1))
sign  = 1 if score > 0 else (-1 if score < 0 else 0)
seconds = epoch_seconds(date) - 1134028003  # Reddit epoch: Dec 8, 2005
hot = sign * order + seconds / 45000
```

Key properties:
- **Logarithmic vote compression**: Going from 1→10 votes gives the same boost as 10→100 or 100→1000
- **Time is the dominant force**: Every 12.5 hours of age costs 1 full point (equivalent to needing 10x more votes)
- **First 10 upvotes ≈ next 100**: Early velocity matters exponentially more

#### Confidence/Best Formula (Wilson score interval)

```python
n = ups + downs
z = 1.281551565545  # 80% confidence
p = ups / n
confidence = (p + z²/2n - z√(p(1-p)/n + z²/4n²)) / (1 + z²/n)
```

Ranks by approval probability, not raw count. 10 upvotes / 0 downvotes beats 100 upvotes / 50 downvotes.

#### Controversial Formula

```python
magnitude = ups + downs
balance = min(ups, downs) / max(ups, downs)
controversial = magnitude ** balance
```

Rewards posts with high total engagement but nearly equal up/down ratios.

#### Rising Formula

```python
rising = upvotes / max(comment_count, 1)
```

Simple velocity signal: high upvotes relative to comment count.

### Key Reddit Ranking Signals (2026)

| Category | Signal | Impact |
|---|---|---|
| **Velocity** | Upvotes in first 30 minutes | Highest — determines trajectory |
| **Velocity** | First 10 upvotes ≈ next 100 (log compression) | Highest |
| **Time** | Post age (12.5h = 1 full ranking point lost) | Highest |
| **Engagement** | Comment count and depth | High (indirect — boosts visibility) |
| **Engagement** | OP responding to comments quickly | High (comment velocity signal) |
| **Ratio** | Upvote ratio (>90% = strong, <65% = penalized) | Moderate |
| **Title** | Compelling title (click-through rate) | Highest content signal |
| **Format** | Matching format to subreddit culture | High |
| **Account** | Karma, account age, participation history | Moderate (gating) |
| **Timing** | Posting when target audience is active | High |
| **Content** | Specificity, data, questions, value-first | High |
| **Negative** | Self-promotion, spam patterns, rule violations | Severe (removal) |

### What This Package Does

Since Reddit's ranking is formula-based (not ML-based like X), `redditviral-go` takes a dual approach:

1. **Content scoring**: Heuristic analysis of title + body text for signals that correlate with high upvote velocity and engagement
2. **Subreddit context**: Account for subreddit size, culture, and format preferences
3. **LLM optimization**: Iteratively rewrite titles and body text to maximize viral potential

---

## File Structure

```
redditviral-go/
├── go.mod
├── doc.go                    # package redditviral
├── redditviral.go            # Scorer, New(), Option funcs
├── score.go                  # Core scoring pipeline
├── signals.go                # Signal extraction (title, body, format analysis)
├── weights.go                # WeightSet, Reddit formula constants
├── feedback.go               # Suggestion generation
├── optimize.go               # LLM-in-the-loop iteration
├── llm.go                    # LLMProvider interface, prompt builder
├── types.go                  # Domain types
├── errors.go                 # Sentinel errors
├── redditviral_test.go       # Tests
└── README.md
```

---

## Domain Types (`types.go`)

```go
type PostType int

const (
    PostText PostType = iota
    PostLink
    PostImage
    PostVideo
    PostPoll
    PostCrosspost
)

type SubredditSize int

const (
    SubredditMicro  SubredditSize = iota // <50K
    SubredditSmall                       // 50K-200K
    SubredditMedium                      // 200K-500K
    SubredditLarge                       // 500K-2M
    SubredditMega                        // 2M+
)

type Post struct {
    Title          string       `json:"title"`
    Body           string       `json:"body,omitempty"`
    PostType       PostType     `json:"postType,omitempty"`
    URL            string       `json:"url,omitempty"`
    Subreddit      string       `json:"subreddit,omitempty"`
    SubredditSize  SubredditSize `json:"subredditSize,omitempty"`
    Flair          string       `json:"flair,omitempty"`
    IsNSFW         bool         `json:"isNSFW,omitempty"`
    IsSpoiler      bool         `json:"isSpoiler,omitempty"`
}

type AuthorProfile struct {
    TotalKarma     int  `json:"totalKarma"`
    LinkKarma      int  `json:"linkKarma"`
    CommentKarma   int  `json:"commentKarma"`
    AccountAgeDays int  `json:"accountAgeDays"`
    IsGold         bool `json:"isGold"`
    HasVerifiedEmail bool `json:"hasVerifiedEmail"`
}

type ScoreResult struct {
    Score           int          `json:"score"`
    RawScore        float64      `json:"rawScore"`
    TitleScore      float64      `json:"titleScore"`
    BodyScore       float64      `json:"bodyScore"`
    Signals         []Signal     `json:"signals"`
    Feedback        []Suggestion `json:"feedback"`
    PositiveFactors []string     `json:"positiveFactors"`
    NegativeFactors []string     `json:"negativeFactors"`
}

type Signal struct {
    Name        string  `json:"name"`
    Category    string  `json:"category"`
    Impact      float64 `json:"impact"`
    Description string  `json:"description"`
}

type Suggestion struct {
    Priority    int    `json:"priority"`
    Category    string `json:"category"`
    Action      string `json:"action"`
    Description string `json:"description"`
    Impact      string `json:"impact"`
}

type OptimizeResult struct {
    Original   ScoreResult `json:"original"`
    Iterations []Iteration `json:"iterations"`
    Best       Iteration   `json:"best"`
}

type Iteration struct {
    Number    int         `json:"number"`
    Title     string      `json:"title"`
    Body      string      `json:"body"`
    Score     ScoreResult `json:"score"`
    Reasoning string      `json:"reasoning"`
}
```

---

## API Surface

```go
func New(opts ...Option) *Scorer

func (s *Scorer) Score(title string) ScoreResult
func (s *Scorer) ScorePost(post Post) ScoreResult
func (s *Scorer) ScoreWithAuthor(post Post, author AuthorProfile) ScoreResult
func (s *Scorer) ScoreBatch(posts []Post) []ScoreResult

func (s *Scorer) Optimize(ctx context.Context, title string, opts ...OptimizeOption) (*OptimizeResult, error)
func (s *Scorer) OptimizePost(ctx context.Context, post Post, opts ...OptimizeOption) (*OptimizeResult, error)
func (s *Scorer) GeneratePrompt(result ScoreResult) string

// Options
func WithWeights(w WeightSet) Option
func WithLLMProvider(p LLMProvider) Option
func WithScoreNormalizer(fn func(float64) int) Option

// Optimize options
func WithMaxIterations(n int) OptimizeOption
func WithTargetScore(score int) OptimizeOption
func WithTone(tone string) OptimizeOption
func WithAudience(audience string) OptimizeOption
func WithConstraints(constraints ...string) OptimizeOption
func WithPreserveIntent(preserve bool) OptimizeOption
func WithSubreddit(subreddit string) OptimizeOption
```

---

## Scoring Signals

### Title Signals (highest impact — title is ~80% of Reddit success)

| Signal | Detection | Impact |
|---|---|---|
| `question_title` | Title ends with `?` or contains question words | +8.0 |
| `specific_numbers` | "3 reasons", "87% of", "$50K" | +5.0 |
| `personal_experience` | "I", "My", "I just", first-person markers | +4.0 |
| `how_to` | "How to", "Guide to", "Step by step" | +3.0 |
| `emotional_hook` | Surprise, curiosity, controversy markers | +4.0 |
| `optimal_title_length` | 60-120 characters (sweet spot) | +2.0 |
| `brackets_tags` | [OC], [Serious], [Discussion], etc. | +1.5 |
| `specificity` | Specific nouns/proper nouns vs vague language | +2.0 |
| `title_too_short` | <20 characters | -3.0 |
| `title_too_long` | >300 characters | -2.0 |
| `clickbait_title` | "You won't believe", "SHOCKING" | -5.0 |
| `all_caps_title` | >30% uppercase | -4.0 |

### Body/Content Signals

| Signal | Detection | Impact |
|---|---|---|
| `tldr_present` | Contains TL;DR or TLDR section | +2.0 |
| `formatted_body` | Headers, bullets, paragraphs | +2.5 |
| `optimal_body_length` | 500-2000 chars (text posts) | +2.0 |
| `source_links` | Contains references/citations | +1.5 |
| `story_arc` | Narrative structure | +3.0 |
| `value_first` | Key info in first paragraph | +2.0 |
| `wall_of_text` | >2000 chars, no formatting | -3.0 |
| `self_promotion` | "my website", "my product", promo language | -8.0 |
| `external_link_text` | Link post to low-quality domain | -2.0 |
| `too_short_body` | Text post with <50 chars body | -2.0 |

### Format/Meta Signals

| Signal | Detection | Impact |
|---|---|---|
| `image_post` | PostType == Image | +2.0 (subreddit-dependent) |
| `video_post` | PostType == Video | +1.0 |
| `crosspost` | PostType == Crosspost | -1.0 |
| `nsfw_flag` | IsNSFW == true | -3.0 (reduced reach) |

### Account Signals

| Signal | Detection | Impact |
|---|---|---|
| `high_karma` | TotalKarma > 10000 | +2.0 |
| `low_karma` | TotalKarma < 100 | -2.0 |
| `new_account` | AccountAgeDays < 30 | -4.0 |
| `verified_email` | HasVerifiedEmail == true | +1.0 |

---

## Suggested Implementation Order

1. `errors.go` + `types.go`
2. `weights.go` (Reddit formula constants)
3. `signals.go` (title + body signal detection)
4. `score.go` (scoring pipeline)
5. `feedback.go`
6. `redditviral.go` (public API)
7. `llm.go` + `optimize.go`
8. `doc.go` + `README.md`
9. Tests
10. Tag v0.1.0
