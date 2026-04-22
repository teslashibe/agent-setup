package notifications

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is the pgx-backed persistence layer for notification_events.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore wires a Store to a pgx pool.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// InsertBatch inserts every event in the batch under userID, deduplicating
// on (user_id, app_package, captured_at, title) via ON CONFLICT DO NOTHING.
// Returns the number of rows actually inserted.
//
// The dedup key matches uq_notif_event_dedup in the migration.
func (s *Store) InsertBatch(ctx context.Context, userID string, events []EventInput) (int, error) {
	if len(events) == 0 {
		return 0, nil
	}
	const q = `
		INSERT INTO notification_events
			(user_id, app_package, app_label, title, content, category, captured_at)
		VALUES
			($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT ON CONSTRAINT uq_notif_event_dedup DO NOTHING`

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var accepted int
	for _, ev := range events {
		ct := ev.CapturedAt
		if ct.IsZero() {
			ct = time.Now().UTC()
		}
		tag, err := tx.Exec(ctx, q,
			userID,
			strings.TrimSpace(ev.AppPackage),
			strings.TrimSpace(ev.AppLabel),
			strings.TrimSpace(ev.Title),
			strings.TrimSpace(ev.Content),
			strings.TrimSpace(ev.Category),
			ct.UTC(),
		)
		if err != nil {
			return 0, fmt.Errorf("insert: %w", err)
		}
		accepted += int(tag.RowsAffected())
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return accepted, nil
}

// List returns events for userID matching opts in reverse chronological
// order (newest first). Empty/nil filters are treated as "no constraint".
func (s *Store) List(ctx context.Context, userID string, opts ListOpts) ([]Event, error) {
	args := []any{userID}
	clauses := []string{"user_id = $1"}
	if opts.Since != nil {
		args = append(args, opts.Since.UTC())
		clauses = append(clauses, fmt.Sprintf("captured_at >= $%d", len(args)))
	}
	if opts.Until != nil {
		args = append(args, opts.Until.UTC())
		clauses = append(clauses, fmt.Sprintf("captured_at <= $%d", len(args)))
	}
	if app := strings.TrimSpace(opts.AppPackage); app != "" {
		args = append(args, app)
		clauses = append(clauses, fmt.Sprintf("app_package = $%d", len(args)))
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	args = append(args, limit)
	q := `SELECT id, user_id, app_package, app_label, title, content, category, captured_at, created_at
	        FROM notification_events
	       WHERE ` + strings.Join(clauses, " AND ") + `
	    ORDER BY captured_at DESC, id DESC
	       LIMIT $` + fmt.Sprintf("%d", len(args))
	return s.queryEvents(ctx, q, args...)
}

// Search runs a full-text query (Postgres plainto_tsquery) against title +
// content, restricted by the same opts as List. Empty query string falls
// through to List.
func (s *Store) Search(ctx context.Context, userID, query string, opts ListOpts) ([]Event, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return s.List(ctx, userID, opts)
	}
	args := []any{userID, q}
	clauses := []string{
		"user_id = $1",
		"to_tsvector('english', coalesce(title,'') || ' ' || coalesce(content,'')) @@ plainto_tsquery('english', $2)",
	}
	if opts.Since != nil {
		args = append(args, opts.Since.UTC())
		clauses = append(clauses, fmt.Sprintf("captured_at >= $%d", len(args)))
	}
	if opts.Until != nil {
		args = append(args, opts.Until.UTC())
		clauses = append(clauses, fmt.Sprintf("captured_at <= $%d", len(args)))
	}
	if app := strings.TrimSpace(opts.AppPackage); app != "" {
		args = append(args, app)
		clauses = append(clauses, fmt.Sprintf("app_package = $%d", len(args)))
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	args = append(args, limit)
	sql := `SELECT id, user_id, app_package, app_label, title, content, category, captured_at, created_at
	          FROM notification_events
	         WHERE ` + strings.Join(clauses, " AND ") + `
	      ORDER BY captured_at DESC, id DESC
	         LIMIT $` + fmt.Sprintf("%d", len(args))
	return s.queryEvents(ctx, sql, args...)
}

// GroupThreads clusters events into Thread rows. With GroupBy=="contact"
// (default) it groups by (app_package, title) which approximates "messages
// from this contact in this app". With GroupBy=="app" it collapses to one
// row per app — useful as a coarse landscape view.
func (s *Store) GroupThreads(ctx context.Context, userID string, opts ThreadOpts) ([]Thread, error) {
	args := []any{userID}
	clauses := []string{"user_id = $1"}
	if opts.Since != nil {
		args = append(args, opts.Since.UTC())
		clauses = append(clauses, fmt.Sprintf("captured_at >= $%d", len(args)))
	}
	if opts.Until != nil {
		args = append(args, opts.Until.UTC())
		clauses = append(clauses, fmt.Sprintf("captured_at <= $%d", len(args)))
	}
	if app := strings.TrimSpace(opts.AppPackage); app != "" {
		args = append(args, app)
		clauses = append(clauses, fmt.Sprintf("app_package = $%d", len(args)))
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	args = append(args, limit)

	groupBy := strings.ToLower(strings.TrimSpace(opts.GroupBy))
	if groupBy == "" {
		groupBy = "contact"
	}

	var sql string
	switch groupBy {
	case "app":
		sql = `SELECT '' AS contact, app_label, app_package, count(*)::int, min(captured_at), max(captured_at),
		              (array_agg(content ORDER BY captured_at DESC))[1] AS preview
		         FROM notification_events
		        WHERE ` + strings.Join(clauses, " AND ") + `
		     GROUP BY app_package, app_label
		     ORDER BY max(captured_at) DESC
		        LIMIT $` + fmt.Sprintf("%d", len(args))
	default: // "contact"
		sql = `SELECT title AS contact, app_label, app_package, count(*)::int, min(captured_at), max(captured_at),
		              (array_agg(content ORDER BY captured_at DESC))[1] AS preview
		         FROM notification_events
		        WHERE ` + strings.Join(clauses, " AND ") + `
		     GROUP BY app_package, app_label, title
		     ORDER BY max(captured_at) DESC
		        LIMIT $` + fmt.Sprintf("%d", len(args))
	}

	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Thread
	for rows.Next() {
		var t Thread
		var preview *string
		if err := rows.Scan(&t.Contact, &t.AppLabel, &t.AppPackage, &t.MessageCount, &t.FirstAt, &t.LastAt, &preview); err != nil {
			return nil, err
		}
		if preview != nil {
			t.Preview = *preview
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListApps returns the distinct apps that have sent notifications for the
// user, with row counts and last-seen timestamps. Powers both the
// notifications_apps MCP tool and the mobile settings screen's "captured
// apps" stat.
func (s *Store) ListApps(ctx context.Context, userID string, since, until *time.Time) ([]AppSummary, error) {
	args := []any{userID}
	clauses := []string{"user_id = $1"}
	if since != nil {
		args = append(args, since.UTC())
		clauses = append(clauses, fmt.Sprintf("captured_at >= $%d", len(args)))
	}
	if until != nil {
		args = append(args, until.UTC())
		clauses = append(clauses, fmt.Sprintf("captured_at <= $%d", len(args)))
	}
	sql := `SELECT app_package, max(app_label), count(*)::int, max(captured_at)
	          FROM notification_events
	         WHERE ` + strings.Join(clauses, " AND ") + `
	      GROUP BY app_package
	      ORDER BY max(captured_at) DESC`
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AppSummary
	for rows.Next() {
		var a AppSummary
		if err := rows.Scan(&a.AppPackage, &a.AppLabel, &a.Count, &a.LastAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// PendingActions returns events that match action-item heuristics
// (questions, time-sensitive keywords, missed calls). Ranking happens in
// the service layer so the SQL stays a simple WHERE-or filter.
func (s *Store) PendingActions(ctx context.Context, userID string, opts ActionOpts) ([]Event, error) {
	args := []any{userID}
	clauses := []string{"user_id = $1"}
	if opts.Since != nil {
		args = append(args, opts.Since.UTC())
		clauses = append(clauses, fmt.Sprintf("captured_at >= $%d", len(args)))
	}
	if opts.Until != nil {
		args = append(args, opts.Until.UTC())
		clauses = append(clauses, fmt.Sprintf("captured_at <= $%d", len(args)))
	}
	// Heuristic OR: contains '?', or matches a urgency keyword, or category=call.
	clauses = append(clauses, `(
		content ILIKE '%?%'
		OR content ~* '(deadline|expires|by tomorrow|showing at|offer|closing|inspection|ASAP|urgent|tonight|today)'
		OR title ~* '(missed call|missed)'
		OR category = 'call'
	)`)
	limit := opts.Limit
	if limit <= 0 {
		limit = 100
	}
	args = append(args, limit)
	sql := `SELECT id, user_id, app_package, app_label, title, content, category, captured_at, created_at
	          FROM notification_events
	         WHERE ` + strings.Join(clauses, " AND ") + `
	      ORDER BY captured_at DESC
	         LIMIT $` + fmt.Sprintf("%d", len(args))
	return s.queryEvents(ctx, sql, args...)
}

func (s *Store) queryEvents(ctx context.Context, sql string, args ...any) ([]Event, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

func scanEvents(rows pgx.Rows) ([]Event, error) {
	var out []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(
			&e.ID, &e.UserID, &e.AppPackage, &e.AppLabel,
			&e.Title, &e.Content, &e.Category, &e.CapturedAt, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
