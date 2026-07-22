package worker

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"mi-api-divisas/models"
)

func Start(ctx context.Context, pool *pgxpool.Pool, dataURL string) {
	updateRates(ctx, pool, dataURL)

	ticker := time.NewTicker(24 * time.Hour)
	for {
		select {
		case <-ticker.C:
			updateRates(ctx, pool, dataURL)
		case <-ctx.Done():
			return
		}
	}
}

func updateRates(ctx context.Context, pool *pgxpool.Pool, dataURL string) {
	log.Println("🔄 Sincronizando datos con proveedor externo...")

	resp, err := http.Get(dataURL)
	if err != nil {
		log.Println("Error de red:", err)
		return
	}
	defer resp.Body.Close()

	var data models.APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		log.Println("Error JSON:", err)
		return
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)

	for pair, rate := range data.Quotes {
		code := strings.TrimPrefix(pair, "USD")
		if code == "" || len(code) != 3 {
			continue
		}

		tx.Exec(ctx, `INSERT INTO exchange_rates (currency_code, rate_to_base, updated_at)
			VALUES ($1, $2, NOW()) ON CONFLICT (currency_code) 
			DO UPDATE SET rate_to_base = EXCLUDED.rate_to_base, updated_at = NOW();`, code, rate)

		tx.Exec(ctx, `INSERT INTO rate_history (currency_code, rate) VALUES ($1, $2);`, code, rate)
	}

	tx.Commit(ctx)
	log.Println("✅ Sincronización terminada.")
}
