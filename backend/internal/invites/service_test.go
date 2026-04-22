package invites_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"

	"github.com/teslashibe/agent-setup/backend/internal/apperrors"
	"github.com/teslashibe/agent-setup/backend/internal/auth"
	dbmigrations "github.com/teslashibe/agent-setup/backend/internal/db/migrations"
	"github.com/teslashibe/agent-setup/backend/internal/invites"
	"github.com/teslashibe/agent-setup/backend/internal/teams"
)

var (
	pool    *pgxpool.Pool
	userSeq atomic.Int64
)

type recordingSender struct {
	mu       sync.Mutex
	messages []sentMessage
	failNext bool
}

type sentMessage struct {
	To, Subject, Body string
}

func (r *recordingSender) Send(_ context.Context, to, subject, body string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failNext {
		r.failNext = false
		return errors.New("email gateway down")
	}
	r.messages = append(r.messages, sentMessage{to, subject, body})
	return nil
}

func (r *recordingSender) last() (sentMessage, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.messages) == 0 {
		return sentMessage{}, false
	}
	return r.messages[len(r.messages)-1], true
}

func TestMain(m *testing.M) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		fmt.Println("TEST_DATABASE_URL not set; skipping invites DB tests")
		os.Exit(0)
	}

	db, err := sql.Open("pgx", url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open: %v\n", err)
		os.Exit(1)
	}
	goose.SetBaseFS(dbmigrations.Files)
	if err := goose.SetDialect("postgres"); err != nil {
		fmt.Fprintf(os.Stderr, "dialect: %v\n", err)
		os.Exit(1)
	}
	if err := goose.UpContext(context.Background(), db, "."); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
	db.Close()

	p, err := pgxpool.New(context.Background(), url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pool: %v\n", err)
		os.Exit(1)
	}
	pool = p
	defer pool.Close()
	if _, err := pool.Exec(context.Background(),
		`TRUNCATE teams, team_members, team_invites, agent_sessions, auth_codes, users RESTART IDENTITY CASCADE`,
	); err != nil {
		fmt.Fprintf(os.Stderr, "truncate: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func setup(t *testing.T) (*invites.Service, *teams.Service, *auth.Service, *recordingSender) {
	t.Helper()
	if pool == nil {
		t.Skip("no DB")
	}
	authSvc := auth.NewService(pool)
	teamsSvc := teams.NewService(teams.NewStore(pool), 10)
	sender := &recordingSender{}
	svc := invites.NewService(invites.Config{
		AppName:   "Test App",
		AppURL:    "https://app.test",
		FromName:  "Test",
		InviteTTL: time.Hour,
	}, teamsSvc, authSvc, sender)
	return svc, teamsSvc, authSvc, sender
}

func makeUser(t *testing.T, authSvc *auth.Service, email string) auth.User {
	t.Helper()
	n := userSeq.Add(1)
	if email == "" {
		email = fmt.Sprintf("u%d@test.local", n)
	}
	res, err := authSvc.UpsertIdentity(t.Context(), "email|"+email, email, fmt.Sprintf("U%d", n))
	if err != nil {
		t.Fatal(err)
	}
	return res.User
}

func TestCreateAndSend_basics(t *testing.T) {
	svc, teamsSvc, authSvc, sender := setup(t)
	owner := makeUser(t, authSvc, "")
	if _, err := teamsSvc.EnsurePersonalTeam(t.Context(), owner.ID, owner.Name, owner.Email); err != nil {
		t.Fatal(err)
	}
	team, err := teamsSvc.Create(t.Context(), owner.ID, "Acme")
	if err != nil {
		t.Fatal(err)
	}

	inv, err := svc.CreateAndSend(t.Context(), team.ID, owner.ID, "joiner@test.local", teams.RoleMember)
	if err != nil {
		t.Fatal(err)
	}
	if inv.Token == "" {
		t.Fatal("token should be set on the returned invite")
	}
	msg, ok := sender.last()
	if !ok {
		t.Fatal("expected a sent email")
	}
	if msg.To != "joiner@test.local" {
		t.Fatalf("to: got %q", msg.To)
	}
	if msg.Subject == "" || msg.Body == "" {
		t.Fatal("subject/body should not be empty")
	}
}

func TestCreateAndSend_emailDispatchFailureRevokesInvite(t *testing.T) {
	svc, teamsSvc, authSvc, sender := setup(t)
	owner := makeUser(t, authSvc, "")
	team, _ := teamsSvc.Create(t.Context(), owner.ID, "Failing")
	sender.failNext = true

	if _, err := svc.CreateAndSend(t.Context(), team.ID, owner.ID, "lost@test.local", teams.RoleMember); err == nil {
		t.Fatal("expected error when email gateway fails")
	}

	// Resending now should succeed because the failed invite was revoked.
	if _, err := svc.CreateAndSend(t.Context(), team.ID, owner.ID, "lost@test.local", teams.RoleMember); err != nil {
		t.Fatalf("resend after rollback should succeed, got %v", err)
	}
}

func TestPreviewAndAccept(t *testing.T) {
	svc, teamsSvc, authSvc, _ := setup(t)
	owner := makeUser(t, authSvc, "")
	team, _ := teamsSvc.Create(t.Context(), owner.ID, "Workspace")

	inv, err := svc.CreateAndSend(t.Context(), team.ID, owner.ID, "ada@test.local", teams.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}

	preview, err := svc.Preview(t.Context(), inv.Token)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Team.ID != team.ID || preview.Role != teams.RoleAdmin {
		t.Fatalf("preview mismatch: %+v", preview)
	}
	if preview.Email != "ada@test.local" {
		t.Fatal("preview email should be the invite address")
	}

	accepter := makeUser(t, authSvc, "ada@test.local")
	gotTeam, gotRole, err := svc.AcceptByToken(t.Context(), accepter.ID, inv.Token)
	if err != nil {
		t.Fatal(err)
	}
	if gotTeam.ID != team.ID || gotRole != teams.RoleAdmin {
		t.Fatalf("accept mismatch: %+v %s", gotTeam, gotRole)
	}
}

func TestAccept_emailMismatchRejected(t *testing.T) {
	svc, teamsSvc, authSvc, _ := setup(t)
	owner := makeUser(t, authSvc, "")
	team, _ := teamsSvc.Create(t.Context(), owner.ID, "Locked")
	inv, _ := svc.CreateAndSend(t.Context(), team.ID, owner.ID, "intended@test.local", teams.RoleMember)

	other := makeUser(t, authSvc, "imposter@test.local")
	if _, _, err := svc.AcceptByToken(t.Context(), other.ID, inv.Token); !errors.Is(err, apperrors.ErrEmailMismatch) {
		t.Fatalf("want ErrEmailMismatch, got %v", err)
	}
}

func TestAccept_revokedInviteIsRejected(t *testing.T) {
	svc, teamsSvc, authSvc, _ := setup(t)
	owner := makeUser(t, authSvc, "")
	team, _ := teamsSvc.Create(t.Context(), owner.ID, "Revoke")
	inv, _ := svc.CreateAndSend(t.Context(), team.ID, owner.ID, "later@test.local", teams.RoleMember)

	if err := svc.Revoke(t.Context(), team.ID, inv.ID); err != nil {
		t.Fatal(err)
	}

	accepter := makeUser(t, authSvc, "later@test.local")
	if _, _, err := svc.AcceptByToken(t.Context(), accepter.ID, inv.Token); !errors.Is(err, apperrors.ErrInviteRevoked) {
		t.Fatalf("want ErrInviteRevoked, got %v", err)
	}
}

func TestSeatLimit_blocksInvite(t *testing.T) {
	authSvc := auth.NewService(pool)
	teamsSvc := teams.NewService(teams.NewStore(pool), 2)
	sender := &recordingSender{}
	svc := invites.NewService(invites.Config{AppName: "T", AppURL: "https://app.test", InviteTTL: time.Hour}, teamsSvc, authSvc, sender)

	owner := makeUser(t, authSvc, "")
	team, _ := teamsSvc.Create(t.Context(), owner.ID, "Capped")

	if _, err := svc.CreateAndSend(t.Context(), team.ID, owner.ID, "first@test.local", teams.RoleMember); err != nil {
		t.Fatal(err)
	}
	// owner is already a member; with max_seats=2 and one pending invite, the next invite should fail.
	if _, err := svc.CreateAndSend(t.Context(), team.ID, owner.ID, "second@test.local", teams.RoleMember); !errors.Is(err, apperrors.ErrSeatLimitReached) {
		t.Fatalf("want ErrSeatLimitReached, got %v", err)
	}
}

func TestPreview_unknownToken(t *testing.T) {
	svc, _, _, _ := setup(t)
	if _, err := svc.Preview(t.Context(), "totally-not-a-real-token"); !errors.Is(err, apperrors.ErrInviteNotFound) {
		t.Fatalf("want ErrInviteNotFound, got %v", err)
	}
}
