package main

import (
	"context"
	"database/sql"
	"log"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"

	dbmigrations "github.com/teslashibe/agent-setup/backend/internal/db/migrations"
)

func main() {
	_ = godotenv.Load()

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		log.Fatal("DATABASE_URL is required")
	}

	cmd := "up"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	db, err := sql.Open("pgx", url)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	goose.SetBaseFS(dbmigrations.Files)
	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	switch cmd {
	case "up":
		if err := goose.UpContext(ctx, db, "."); err != nil {
			log.Fatalf("up: %v", err)
		}
	case "down":
		if err := goose.DownContext(ctx, db, "."); err != nil {
			log.Fatalf("down: %v", err)
		}
	case "status":
		if err := goose.StatusContext(ctx, db, "."); err != nil {
			log.Fatalf("status: %v", err)
		}
	case "reset":
		if err := goose.ResetContext(ctx, db, "."); err != nil {
			log.Fatalf("reset: %v", err)
		}
	default:
		log.Fatalf("unknown command %q — use: up, down, status, reset", cmd)
	}
}
