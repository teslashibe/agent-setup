package notifications

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Service wraps Store with business logic that doesn't belong in raw SQL:
// action-item ranking, payload normalisation, and limit clamping.
type Service struct {
	store *Store
	cfg   ServiceConfig
}

// ServiceConfig is the runtime knob set used by the service. Defaults are
// applied via WithDefaults so callers can pass a zero-value struct.
type ServiceConfig struct {
	// DefaultPageSize bounds list/search results when the caller omits Limit.
	DefaultPageSize int
	// MaxPageSize is the hard cap on Limit regardless of caller request.
	MaxPageSize int
	// ReplyWindowHrs is the unanswered-message threshold for action items.
	ReplyWindowHrs int
}

// WithDefaults fills in safe defaults for any zero field.
func (c ServiceConfig) WithDefaults() ServiceConfig {
	if c.DefaultPageSize <= 0 {
		c.DefaultPageSize = 50
	}
	if c.MaxPageSize <= 0 {
		c.MaxPageSize = 200
	}
	if c.ReplyWindowHrs <= 0 {
		c.ReplyWindowHrs = 2
	}
	return c
}

// NewService constructs a Service.
func NewService(store *Store, cfg ServiceConfig) *Service {
	return &Service{store: store, cfg: cfg.WithDefaults()}
}

// IngestBatch validates, normalises, and persists a batch of EventInput
// rows. Empty input is accepted (returns 0). Inputs with empty
// app_package are skipped silently — the device occasionally fires
// "system" notifications without one.
func (s *Service) IngestBatch(ctx context.Context, userID string, in BatchInput) (BatchResult, error) {
	clean := make([]EventInput, 0, len(in.Events))
	for _, ev := range in.Events {
		if strings.TrimSpace(ev.AppPackage) == "" {
			continue
		}
		clean = append(clean, ev)
	}
	n, err := s.store.InsertBatch(ctx, userID, clean)
	if err != nil {
		return BatchResult{}, err
	}
	return BatchResult{Accepted: n}, nil
}

// List clamps limit and forwards to the store.
func (s *Service) List(ctx context.Context, userID string, opts ListOpts) ([]Event, error) {
	opts.Limit = s.clampLimit(opts.Limit)
	return s.store.List(ctx, userID, opts)
}

// Search clamps limit and forwards to the store.
func (s *Service) Search(ctx context.Context, userID, query string, opts ListOpts) ([]Event, error) {
	opts.Limit = s.clampLimit(opts.Limit)
	return s.store.Search(ctx, userID, query, opts)
}

// GroupThreads clamps limit and forwards to the store.
func (s *Service) GroupThreads(ctx context.Context, userID string, opts ThreadOpts) ([]Thread, error) {
	opts.Limit = s.clampLimit(opts.Limit)
	return s.store.GroupThreads(ctx, userID, opts)
}

// ListApps forwards to the store; no clamping (app cardinality is naturally
// small).
func (s *Service) ListApps(ctx context.Context, userID string, since, until *time.Time) ([]AppSummary, error) {
	return s.store.ListApps(ctx, userID, since, until)
}

// PendingActions queries candidate rows and ranks them. The store does the
// SQL filter; the service applies prioritisation + dedup.
func (s *Service) PendingActions(ctx context.Context, userID string, opts ActionOpts) ([]ActionItem, error) {
	if opts.ReplyWindowHrs <= 0 {
		opts.ReplyWindowHrs = s.cfg.ReplyWindowHrs
	}
	opts.Limit = s.clampLimit(opts.Limit)
	rows, err := s.store.PendingActions(ctx, userID, opts)
	if err != nil {
		return nil, err
	}
	out := make([]ActionItem, 0, len(rows))
	for _, e := range rows {
		out = append(out, classify(e))
	}
	sort.SliceStable(out, func(i, j int) bool {
		return priorityRank(out[i].Priority) < priorityRank(out[j].Priority)
	})
	return out, nil
}

func (s *Service) clampLimit(limit int) int {
	if limit <= 0 {
		return s.cfg.DefaultPageSize
	}
	if limit > s.cfg.MaxPageSize {
		return s.cfg.MaxPageSize
	}
	return limit
}

// urgencyRE matches the time-sensitive keyword set used by classify and the
// store's WHERE clause. Kept in one place so the two can't drift.
var urgencyRE = regexp.MustCompile(`(?i)\b(deadline|expires|by tomorrow|showing at|offer|closing|inspection|asap|urgent|tonight|today)\b`)

// missedCallRE matches phone-dialer notifications.
var missedCallRE = regexp.MustCompile(`(?i)\bmissed\b`)

// classify turns an Event into a ranked ActionItem. The agent re-ranks
// these with its own reasoning, but having a coarse priority + reason in
// the data lets the agent skim quickly.
func classify(e Event) ActionItem {
	item := ActionItem{
		Priority:   "low",
		Summary:    summarise(e),
		Contact:    e.Title,
		AppLabel:   e.AppLabel,
		AppPackage: e.AppPackage,
		CapturedAt: e.CapturedAt,
		EventID:    e.ID,
	}
	switch {
	case e.Category == "call" || missedCallRE.MatchString(e.Title):
		item.Priority = "high"
		item.Reason = "missed_call"
	case urgencyRE.MatchString(e.Content) || urgencyRE.MatchString(e.Title):
		item.Priority = "high"
		item.Reason = "time_sensitive"
	case strings.Contains(e.Content, "?"):
		item.Priority = "medium"
		item.Reason = "question"
	default:
		item.Priority = "low"
		item.Reason = "follow_up"
	}
	return item
}

func summarise(e Event) string {
	t := strings.TrimSpace(e.Title)
	c := strings.TrimSpace(e.Content)
	switch {
	case t != "" && c != "":
		return t + ": " + c
	case t != "":
		return t
	case c != "":
		return c
	default:
		return e.AppLabel
	}
}

func priorityRank(p string) int {
	switch p {
	case "high":
		return 0
	case "medium":
		return 1
	default:
		return 2
	}
}
