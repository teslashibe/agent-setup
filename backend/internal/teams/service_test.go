package teams_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"

	"github.com/teslashibe/agent-setup/backend/internal/apperrors"
	dbmigrations "github.com/teslashibe/agent-setup/backend/internal/db/migrations"
	"github.com/teslashibe/agent-setup/backend/internal/teams"
)

var (
	testPool   *pgxpool.Pool
	userSeq    atomic.Int64
)

func TestMain(m *testing.M) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		fmt.Println("TEST_DATABASE_URL not set, skipping teams DB tests")
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
	email = fmt.Sprintf("user%d@test.local", n)
	name = fmt.Sprintf("User %d", n)
	err := testPool.QueryRow(context.Background(), `
		INSERT INTO users (identity_key, email, name)
		VALUES ($1, $2, $3)
		RETURNING id::text`,
		"email|"+email, email, name,
	).Scan(&id)
	if err != nil {
		t.Fatalf("freshUser: %v", err)
	}
	return
}

func newSvc(t *testing.T) *teams.Service {
	t.Helper()
	if testPool == nil {
		t.Skip("TEST_DATABASE_URL not set")
	}
	return teams.NewService(teams.NewStore(testPool), 5)
}

func TestEnsurePersonalTeam_idempotent(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	uid, email, name := freshUser(t)

	first, err := svc.EnsurePersonalTeam(ctx, uid, name, email)
	if err != nil {
		t.Fatal(err)
	}
	if !first.IsPersonal {
		t.Fatal("expected personal team")
	}
	second, err := svc.EnsurePersonalTeam(ctx, uid, name, email)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("personal team should be stable across calls: %s vs %s", first.ID, second.ID)
	}
}

func TestCreate_validation(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	uid, _, _ := freshUser(t)

	if _, err := svc.Create(ctx, uid, "  "); err == nil {
		t.Fatal("expected error for empty name")
	}
	if _, err := svc.Create(ctx, uid, longString(81)); err == nil {
		t.Fatal("expected error for over-long name")
	}

	team, err := svc.Create(ctx, uid, "Acme")
	if err != nil {
		t.Fatal(err)
	}
	if team.IsPersonal {
		t.Fatal("explicit Create should never produce a personal team")
	}
	if team.Slug == "" {
		t.Fatal("expected slug to be assigned")
	}
}

func TestCreate_slugCollisionDisambiguates(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	uid, _, _ := freshUser(t)

	a, err := svc.Create(ctx, uid, "Hello World")
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.Create(ctx, uid, "Hello World")
	if err != nil {
		t.Fatal(err)
	}
	if a.Slug == b.Slug {
		t.Fatalf("expected unique slugs, got %q twice", a.Slug)
	}
}

func TestChangeRole_authz(t *testing.T) {
	svc := newSvc(t)
	store := svc.Store()
	ctx := context.Background()

	ownerID, _, _ := freshUser(t)
	adminID, _, _ := freshUser(t)
	memberID, _, _ := freshUser(t)
	otherAdminID, _, _ := freshUser(t)

	team, err := svc.Create(ctx, ownerID, "Acme")
	if err != nil {
		t.Fatal(err)
	}
	for _, uid := range []string{adminID, memberID, otherAdminID} {
		if err := store.AddMember(ctx, team.ID, uid, teams.RoleMember); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.UpdateMemberRole(ctx, team.ID, adminID, teams.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateMemberRole(ctx, team.ID, otherAdminID, teams.RoleAdmin); err != nil {
		t.Fatal(err)
	}

	t.Run("admin can promote a member to admin", func(t *testing.T) {
		err := svc.ChangeRole(ctx, adminID, team.ID, memberID, teams.RoleAdmin)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.UpdateMemberRole(ctx, team.ID, memberID, teams.RoleMember); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("admin cannot change another admin's role", func(t *testing.T) {
		err := svc.ChangeRole(ctx, adminID, team.ID, otherAdminID, teams.RoleMember)
		if !errors.Is(err, apperrors.ErrInsufficientRole) {
			t.Fatalf("want ErrInsufficientRole, got %v", err)
		}
	})

	t.Run("member cannot change anyone's role", func(t *testing.T) {
		err := svc.ChangeRole(ctx, memberID, team.ID, otherAdminID, teams.RoleMember)
		if !errors.Is(err, apperrors.ErrInsufficientRole) {
			t.Fatalf("want ErrInsufficientRole, got %v", err)
		}
	})

	t.Run("nobody can demote the owner via change-role", func(t *testing.T) {
		err := svc.ChangeRole(ctx, adminID, team.ID, ownerID, teams.RoleMember)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("change-role rejects setting the owner role", func(t *testing.T) {
		err := svc.ChangeRole(ctx, ownerID, team.ID, memberID, teams.RoleOwner)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestRemoveMember_rules(t *testing.T) {
	svc := newSvc(t)
	store := svc.Store()
	ctx := context.Background()

	ownerID, _, _ := freshUser(t)
	adminID, _, _ := freshUser(t)
	memberID, _, _ := freshUser(t)
	team, err := svc.Create(ctx, ownerID, "Remove-me")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddMember(ctx, team.ID, adminID, teams.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if err := store.AddMember(ctx, team.ID, memberID, teams.RoleMember); err != nil {
		t.Fatal(err)
	}

	if err := svc.RemoveMember(ctx, ownerID, team.ID, ownerID); err == nil {
		t.Fatal("owner cannot remove themselves via remove-member")
	}
	if err := svc.RemoveMember(ctx, adminID, team.ID, ownerID); !errors.Is(err, apperrors.ErrCannotRemoveOwner) {
		t.Fatalf("want ErrCannotRemoveOwner, got %v", err)
	}
	if err := svc.RemoveMember(ctx, memberID, team.ID, adminID); !errors.Is(err, apperrors.ErrInsufficientRole) {
		t.Fatalf("want ErrInsufficientRole, got %v", err)
	}
	if err := svc.RemoveMember(ctx, adminID, team.ID, memberID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetMembership(ctx, team.ID, memberID); !errors.Is(err, apperrors.ErrNotTeamMember) {
		t.Fatalf("want ErrNotTeamMember, got %v", err)
	}
}

func TestTransferOwnership_atomic(t *testing.T) {
	svc := newSvc(t)
	store := svc.Store()
	ctx := context.Background()

	ownerID, _, _ := freshUser(t)
	successorID, _, _ := freshUser(t)
	team, err := svc.Create(ctx, ownerID, "Hand off")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddMember(ctx, team.ID, successorID, teams.RoleMember); err != nil {
		t.Fatal(err)
	}

	if err := svc.TransferOwnership(ctx, ownerID, team.ID, successorID); err != nil {
		t.Fatal(err)
	}

	original, err := store.GetMembership(ctx, team.ID, ownerID)
	if err != nil {
		t.Fatal(err)
	}
	if original.Role != teams.RoleAdmin {
		t.Fatalf("former owner should be admin, got %q", original.Role)
	}
	successor, err := store.GetMembership(ctx, team.ID, successorID)
	if err != nil {
		t.Fatal(err)
	}
	if successor.Role != teams.RoleOwner {
		t.Fatalf("successor should be owner, got %q", successor.Role)
	}

	var ownerCount int
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM team_members WHERE team_id = $1 AND role = 'owner'`, team.ID,
	).Scan(&ownerCount); err != nil {
		t.Fatal(err)
	}
	if ownerCount != 1 {
		t.Fatalf("expected exactly one owner after transfer, got %d", ownerCount)
	}
}

func TestLeave_rules(t *testing.T) {
	svc := newSvc(t)
	store := svc.Store()
	ctx := context.Background()

	uid, email, name := freshUser(t)
	personal, err := svc.EnsurePersonalTeam(ctx, uid, name, email)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Leave(ctx, uid, personal.ID); !errors.Is(err, apperrors.ErrCannotLeavePersonal) {
		t.Fatalf("want ErrCannotLeavePersonal, got %v", err)
	}

	team, err := svc.Create(ctx, uid, "Leavable")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Leave(ctx, uid, team.ID); err == nil {
		t.Fatal("sole owner cannot leave")
	}

	memberID, _, _ := freshUser(t)
	if err := store.AddMember(ctx, team.ID, memberID, teams.RoleMember); err != nil {
		t.Fatal(err)
	}
	if err := svc.Leave(ctx, memberID, team.ID); err != nil {
		t.Fatal(err)
	}
}

func TestEnforceSeatLimit(t *testing.T) {
	svc := teams.NewService(teams.NewStore(testPool), 3)
	ctx := context.Background()
	ownerID, _, _ := freshUser(t)
	team, err := svc.Create(ctx, ownerID, "SeatCap")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.EnforceSeatLimit(ctx, team.ID, 0); err != nil {
		t.Fatalf("expected current count to fit, got %v", err)
	}
	if err := svc.EnforceSeatLimit(ctx, team.ID, 2); err != nil {
		t.Fatalf("expected count(1)+pending(2) <= max(3) to fit, got %v", err)
	}
	if err := svc.EnforceSeatLimit(ctx, team.ID, 3); !errors.Is(err, apperrors.ErrSeatLimitReached) {
		t.Fatalf("count(1)+pending(3) > max(3) should reject, got %v", err)
	}
}

func TestConsumeInvite_doubleSpendIsRejected(t *testing.T) {
	svc := newSvc(t)
	store := svc.Store()
	ctx := context.Background()

	ownerID, _, _ := freshUser(t)
	team, err := svc.Create(ctx, ownerID, "InviteTest")
	if err != nil {
		t.Fatal(err)
	}

	tok, err := teams.NewInviteToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateInvite(ctx, team.ID, ownerID, "joiner@test.local", teams.RoleMember, tok, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	joinerID, _, _ := freshUser(t)
	if _, err := store.ConsumeInvite(ctx, tok, joinerID, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ConsumeInvite(ctx, tok, joinerID, time.Now()); !errors.Is(err, apperrors.ErrInviteAlreadyAccepted) {
		t.Fatalf("want ErrInviteAlreadyAccepted, got %v", err)
	}
}

func TestConsumeInvite_expired(t *testing.T) {
	svc := newSvc(t)
	store := svc.Store()
	ctx := context.Background()
	ownerID, _, _ := freshUser(t)
	team, err := svc.Create(ctx, ownerID, "ExpiredInvite")
	if err != nil {
		t.Fatal(err)
	}
	tok, _ := teams.NewInviteToken()
	if _, err := store.CreateInvite(ctx, team.ID, ownerID, "late@test.local", teams.RoleMember, tok, time.Now().Add(-1*time.Hour)); err != nil {
		t.Fatal(err)
	}
	joinerID, _, _ := freshUser(t)
	if _, err := store.ConsumeInvite(ctx, tok, joinerID, time.Now()); !errors.Is(err, apperrors.ErrInviteExpired) {
		t.Fatalf("want ErrInviteExpired, got %v", err)
	}
}

func TestCreateInvite_dedupesPending(t *testing.T) {
	svc := newSvc(t)
	store := svc.Store()
	ctx := context.Background()
	ownerID, _, _ := freshUser(t)
	team, _ := svc.Create(ctx, ownerID, "Dedupe")
	tok1, _ := teams.NewInviteToken()
	if _, err := store.CreateInvite(ctx, team.ID, ownerID, "x@test.local", teams.RoleMember, tok1, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	tok2, _ := teams.NewInviteToken()
	if _, err := store.CreateInvite(ctx, team.ID, ownerID, "X@test.local", teams.RoleMember, tok2, time.Now().Add(time.Hour)); !errors.Is(err, apperrors.ErrInvitePending) {
		t.Fatalf("want ErrInvitePending, got %v", err)
	}
}

func longString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}
