package agent_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/teslashibe/agent-setup/backend/internal/agent"
	"github.com/teslashibe/agent-setup/backend/internal/apperrors"
	dbmigrations "github.com/teslashibe/agent-setup/backend/internal/db/migrations"
	"github.com/teslashibe/agent-setup/backend/internal/teams"
)

var (
	testPool   *pgxpool.Pool
	userSeq    atomic.Int64
	sessionSeq atomic.Int64
)

// nextAntID returns a unique anthropic_session_id label for each test row to
// satisfy the schema's UNIQUE(anthropic_session_id) constraint.
func nextAntID(label string) string {
	return fmt.Sprintf("ant_%s_%d", label, sessionSeq.Add(1))
}

func TestMain(m *testing.M) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		fmt.Println("TEST_DATABASE_URL not set, skipping agent DB tests")
		os.Exit(0)
	}
	if err := migrateUp(url); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pool: %v\n", err)
		os.Exit(1)
	}
	testPool = pool
	defer pool.Close()
	if _, err := pool.Exec(context.Background(),
		`TRUNCATE teams, team_members, team_invites, agent_sessions, auth_codes, users RESTART IDENTITY CASCADE`,
	); err != nil {
		fmt.Fprintf(os.Stderr, "truncate: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func migrateUp(url string) error {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return err
	}
	defer db.Close()
	goose.SetBaseFS(dbmigrations.Files)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.UpContext(context.Background(), db, ".")
}

func freshUser(t *testing.T) (id, email, name string) {
	t.Helper()
	n := userSeq.Add(1)
	email = fmt.Sprintf("agent-user%d@test.local", n)
	name = fmt.Sprintf("Agent User %d", n)
	err := testPool.QueryRow(context.Background(), `
		INSERT INTO users (identity_key, email, name)
		VALUES ($1, $2, $3) RETURNING id::text`,
		"email|"+email, email, name,
	).Scan(&id)
	if err != nil {
		t.Fatalf("freshUser: %v", err)
	}
	return
}

// freshTeam creates a non-personal team owned by the given user and returns its ID.
func freshTeam(t *testing.T, ownerID string, label string) string {
	t.Helper()
	tsvc := teams.NewService(teams.NewStore(testPool), 50)
	team, err := tsvc.Create(context.Background(), ownerID, label)
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	return team.ID
}

func TestStore_CreateAndGetInTeam(t *testing.T) {
	if testPool == nil {
		t.Skip("no DB")
	}
	store := agent.NewStore(testPool)
	uid, _, _ := freshUser(t)
	teamID := freshTeam(t, uid, "Alpha")

	sess, err := store.CreateSession(context.Background(), teamID, uid, "First chat", nextAntID("first"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if sess.TeamID != teamID || sess.UserID != uid || sess.Title != "First chat" {
		t.Fatalf("unexpected session: %+v", sess)
	}

	got, err := store.GetSessionInTeam(context.Background(), teamID, sess.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != sess.ID {
		t.Fatalf("ID mismatch: %s vs %s", got.ID, sess.ID)
	}
}

// Sessions created in team A must be invisible from team B's lookups, even if
// the same user is a member of both. This prevents cross-team data bleed.
func TestStore_CrossTeamIsolation(t *testing.T) {
	if testPool == nil {
		t.Skip("no DB")
	}
	store := agent.NewStore(testPool)
	uid, _, _ := freshUser(t)
	teamA := freshTeam(t, uid, "TeamA")
	teamB := freshTeam(t, uid, "TeamB")

	sess, err := store.CreateSession(context.Background(), teamA, uid, "in A", nextAntID("inA"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.GetSessionInTeam(context.Background(), teamB, sess.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("expected ErrNotFound from teamB, got %v", err)
	}

	listB, err := store.ListSessionsInTeam(context.Background(), teamB, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(listB) != 0 {
		t.Fatalf("expected zero sessions in team B, got %d", len(listB))
	}

	listA, err := store.ListSessionsInTeam(context.Background(), teamA, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(listA) != 1 || listA[0].ID != sess.ID {
		t.Fatalf("expected one session in team A: %+v", listA)
	}
}

// ListSessionsInTeam with a userIDFilter scopes the result to that user.
// Without a filter, every session in the team is returned (admin "scope=all").
func TestStore_ListByUserAndAll(t *testing.T) {
	if testPool == nil {
		t.Skip("no DB")
	}
	store := agent.NewStore(testPool)
	owner, _, _ := freshUser(t)
	teamID := freshTeam(t, owner, "TeamMix")

	tsvc := teams.NewService(teams.NewStore(testPool), 50)
	member1, _, _ := freshUser(t)
	member2, _, _ := freshUser(t)
	for _, m := range []string{member1, member2} {
		if err := tsvc.Store().AddMember(context.Background(), teamID, m, teams.RoleMember); err != nil {
			t.Fatalf("add member: %v", err)
		}
	}

	mustCreate := func(uid, label string) {
		if _, err := store.CreateSession(context.Background(), teamID, uid, label, nextAntID(label)); err != nil {
			t.Fatalf("create %s: %v", label, err)
		}
	}
	mustCreate(owner, "owner-1")
	mustCreate(owner, "owner-2")
	mustCreate(member1, "m1-1")
	mustCreate(member2, "m2-1")

	mine, err := store.ListSessionsInTeam(context.Background(), teamID, member1)
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) != 1 || mine[0].UserID != member1 {
		t.Fatalf("member1 list: expected 1 own session, got %+v", mine)
	}

	all, err := store.ListSessionsInTeam(context.Background(), teamID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 {
		t.Fatalf("admin list: expected 4 sessions, got %d", len(all))
	}
}

// DeleteSessionInTeam refuses to delete a session that doesn't belong to the team.
func TestStore_DeleteScopedToTeam(t *testing.T) {
	if testPool == nil {
		t.Skip("no DB")
	}
	store := agent.NewStore(testPool)
	uid, _, _ := freshUser(t)
	teamA := freshTeam(t, uid, "DelA")
	teamB := freshTeam(t, uid, "DelB")

	sess, err := store.CreateSession(context.Background(), teamA, uid, "to delete", nextAntID("del"))
	if err != nil {
		t.Fatal(err)
	}

	// Wrong team → not found, no rows touched.
	if err := store.DeleteSessionInTeam(context.Background(), teamB, sess.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("expected ErrNotFound when deleting from wrong team, got %v", err)
	}
	if _, err := store.GetSessionInTeam(context.Background(), teamA, sess.ID); err != nil {
		t.Fatalf("expected session still present in teamA: %v", err)
	}

	// Correct team → delete succeeds.
	if err := store.DeleteSessionInTeam(context.Background(), teamA, sess.ID); err != nil {
		t.Fatalf("delete teamA: %v", err)
	}
	if _, err := store.GetSessionInTeam(context.Background(), teamA, sess.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}
