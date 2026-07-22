package database

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Connect(ctx context.Context, connStr string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return nil, err
	}
	return pool, nil
}

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS exchange_rates (
			currency_code CHAR(3) PRIMARY KEY,
			rate_to_base DECIMAL(18, 8) NOT NULL,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS rate_history (
			id SERIAL PRIMARY KEY,
			currency_code CHAR(3) NOT NULL,
			rate DECIMAL(18, 8) NOT NULL,
			recorded_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_history_lookup ON rate_history(currency_code, recorded_at);`,
	}

	for _, q := range queries {
		if _, err := pool.Exec(ctx, q); err != nil {
			return err
		}
	}
	log.Println("✅ Base de datos verificada/creada.")
	return nil
}
