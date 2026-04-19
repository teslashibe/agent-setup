package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"

	"github.com/teslashibe/agent-setup/backend/internal/config"
	dbmigrations "github.com/teslashibe/agent-setup/backend/internal/db/migrations"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	command := "up"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	dbConn, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer dbConn.Close()

	goose.SetBaseFS(dbmigrations.Files)
	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatalf("set dialect: %v", err)
	}

	ctx := context.Background()

	switch command {
	case "up":
		if len(os.Args) >= 3 {
			steps, err := strconv.Atoi(os.Args[2])
			if err != nil || steps <= 0 {
				log.Fatalf("invalid up steps %q", os.Args[2])
			}
			if err := goose.UpByOneContext(ctx, dbConn, "."); err != nil {
				log.Fatalf("goose up by one: %v", err)
			}
			for i := 1; i < steps; i++ {
				if err := goose.UpByOneContext(ctx, dbConn, "."); err != nil {
					log.Fatalf("goose up by one: %v", err)
				}
			}
			fmt.Printf("applied up %d step(s)\n", steps)
			return
		}
		if err := goose.UpContext(ctx, dbConn, "."); err != nil {
			log.Fatalf("goose up: %v", err)
		}
		fmt.Println("migrations up complete")
	case "down":
		if err := goose.DownContext(ctx, dbConn, "."); err != nil {
			log.Fatalf("goose down: %v", err)
		}
		fmt.Println("rolled back one migration")
	case "status":
		if err := goose.StatusContext(ctx, dbConn, "."); err != nil {
			log.Fatalf("goose status: %v", err)
		}
	case "version":
		v, err := goose.GetDBVersionContext(ctx, dbConn)
		if err != nil {
			log.Fatalf("goose version: %v", err)
		}
		fmt.Printf("version=%d\n", v)
	case "reset":
		if err := goose.ResetContext(ctx, dbConn, "."); err != nil {
			log.Fatalf("goose reset: %v", err)
		}
	default:
		log.Fatalf("unknown command %q (supported: up, down, status, version, reset)", command)
	}
}
