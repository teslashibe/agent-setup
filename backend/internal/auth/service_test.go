package auth_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"

	"github.com/teslashibe/agent-setup/backend/internal/auth"
	dbmigrations "github.com/teslashibe/agent-setup/backend/internal/db/migrations"
)

var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		fmt.Println("TEST_DATABASE_URL not set; skipping auth DB tests")
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

func TestUpsertIdentity_isNewlyFlag(t *testing.T) {
	if pool == nil {
		t.Skip("no DB")
	}
	svc := auth.NewService(pool)
	ctx := context.Background()

	first, err := svc.UpsertIdentity(ctx, "email|fresh@example.com", "Fresh@Example.com", "")
	if err != nil {
		t.Fatal(err)
	}
	if !first.IsNewly {
		t.Fatal("first upsert should report IsNewly=true")
	}
	if first.User.Email != "fresh@example.com" {
		t.Fatalf("email should be lower-cased, got %q", first.User.Email)
	}
	if first.User.Name == "" {
		t.Fatal("display name should default to email-local-part")
	}

	second, err := svc.UpsertIdentity(ctx, "email|fresh@example.com", "fresh@example.com", "Updated")
	if err != nil {
		t.Fatal(err)
	}
	if second.IsNewly {
		t.Fatal("second upsert should report IsNewly=false")
	}
	if second.User.ID != first.User.ID {
		t.Fatal("upsert should preserve user id")
	}
	if second.User.Name != "Updated" {
		t.Fatalf("display name should update on conflict, got %q", second.User.Name)
	}
}
