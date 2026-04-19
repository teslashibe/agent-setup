package bootstrap

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/teslashibe/agent-setup/backend/internal/config"
	"github.com/teslashibe/agent-setup/backend/internal/db"
)

type Core struct {
	Cfg  config.Config
	Pool *pgxpool.Pool
}

func Init(ctx context.Context) (*Core, error) {
	_ = godotenv.Load()

	cfg := config.Load()
	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}

	return &Core{
		Cfg:  cfg,
		Pool: pool,
	}, nil
}
