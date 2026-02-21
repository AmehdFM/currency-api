package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// --- 1. ESTRUCTURAS ---

type APIResponse struct {
	Success   bool               `json:"success"`
	Timestamp int64              `json:"timestamp"`
	Quotes    map[string]float64 `json:"quotes"`
}

type App struct {
	DB *pgxpool.Pool
}

// --- 2. MAIN & CONFIGURACIÓN ---

func main() {
	ctx := context.Background()

	// Conexión a Base de Datos
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		log.Fatal("Error: La variable DATABASE_URL no está configurada")
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		log.Fatal("Error conectando a Postgres:", err)
	}
	defer pool.Close()

	app := App{DB: pool}

	// Inicialización
	app.initDatabase(ctx)
	go app.startDailyWorker(ctx)

	// Definición de Rutas (Endpoints)
	http.HandleFunc("/convert", app.handleConvert)
	http.HandleFunc("/history", app.handleHistory)
	http.HandleFunc("/latest", app.handleLatest)
	http.HandleFunc("/rates/", app.handleSingleRate)
	http.HandleFunc("/check", app.handleCheck)

	fmt.Println("🚀 API de Divisas Robustas iniciada en :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// --- 3. LÓGICA DE BASE DE DATOS Y WORKER ---

func (app *App) initDatabase(ctx context.Context) {
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
		if _, err := app.DB.Exec(ctx, q); err != nil {
			log.Fatalf("Error creando tablas: %v", err)
		}
	}
	log.Println("✅ Base de datos verificada/creada.")
}

func (app *App) startDailyWorker(ctx context.Context) {
	app.updateRates(ctx)
	ticker := time.NewTicker(24 * time.Hour)
	for {
		select {
		case <-ticker.C:
			app.updateRates(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (app *App) updateRates(ctx context.Context) {
	log.Println("🔄 Sincronizando datos con proveedor externo...")
	url := os.Getenv("DATA_URL")

	resp, err := http.Get(url)
	if err != nil {
		log.Println("Error de red:", err)
		return
	}
	defer resp.Body.Close()

	var data APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		log.Println("Error JSON:", err)
		return
	}

	tx, err := app.DB.Begin(ctx)
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

// --- 4. HANDLERS (ENDPOINTS) ---

func (app *App) handleConvert(w http.ResponseWriter, r *http.Request) {
	from := strings.ToUpper(r.URL.Query().Get("from"))
	to := strings.ToUpper(r.URL.Query().Get("to"))
	amount, _ := decimal.NewFromString(r.URL.Query().Get("amount"))

	if amount.IsZero() {
		amount = decimal.NewFromInt(1)
	}

	var rateFrom, rateTo decimal.Decimal
	err1 := app.DB.QueryRow(r.Context(), "SELECT rate_to_base FROM exchange_rates WHERE currency_code=$1", from).Scan(&rateFrom)
	err2 := app.DB.QueryRow(r.Context(), "SELECT rate_to_base FROM exchange_rates WHERE currency_code=$1", to).Scan(&rateTo)

	if err1 != nil || err2 != nil {
		http.Error(w, "Divisa no encontrada", 404)
		return
	}

	result := amount.Mul(rateTo).Div(rateFrom)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"from": from, "to": to, "amount": amount, "result": result.Round(4),
	})
}

func (app *App) handleHistory(w http.ResponseWriter, r *http.Request) {
	code := strings.ToUpper(r.URL.Query().Get("code"))
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")

	if len(code) != 3 {
		http.Error(w, "Se requiere código de moneda (parámetro 'code')", 400)
		return
	}

	query := "SELECT rate, recorded_at FROM rate_history WHERE currency_code = $1"
	args := []interface{}{code}
	argCount := 2

	if start != "" {
		query += fmt.Sprintf(" AND recorded_at >= $%d", argCount)
		args = append(args, start)
		argCount++
	}
	if end != "" {
		query += fmt.Sprintf(" AND recorded_at <= $%d", argCount)
		args = append(args, end)
		argCount++
	}

	query += " ORDER BY recorded_at DESC LIMIT 100"
	rows, err := app.DB.Query(r.Context(), query, args...)
	if err != nil {
		http.Error(w, "Error interno", 500)
		return
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var rate decimal.Decimal
		var date time.Time
		rows.Scan(&rate, &date)
		results = append(results, map[string]interface{}{
			"code": code, "rate": rate.Round(6), "date": date.Format(time.RFC3339),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func (app *App) handleLatest(w http.ResponseWriter, r *http.Request) {
	rows, err := app.DB.Query(r.Context(), "SELECT currency_code, rate_to_base, updated_at FROM exchange_rates")
	if err != nil {
		http.Error(w, "Error DB", 500)
		return
	}
	defer rows.Close()

	rates := make(map[string]interface{})
	for rows.Next() {
		var code string
		var rate decimal.Decimal
		var updated time.Time
		rows.Scan(&code, &rate, &updated)
		rates[code] = map[string]interface{}{"rate": rate.Round(6), "updated_at": updated}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"base": "USD", "rates": rates})
}

func (app *App) handleSingleRate(w http.ResponseWriter, r *http.Request) {
	code := strings.ToUpper(strings.TrimPrefix(r.URL.Path, "/rates/"))
	if len(code) != 3 {
		http.Error(w, "Código inválido", 400)
		return
	}

	var rate decimal.Decimal
	var updated time.Time
	err := app.DB.QueryRow(r.Context(), "SELECT rate_to_base, updated_at FROM exchange_rates WHERE currency_code=$1", code).Scan(&rate, &updated)

	if err != nil {
		http.Error(w, "No encontrado", 404)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"code": code, "rate": rate.Round(6), "base": "USD", "updated_at": updated})
}

func (app *App) handleCheck(w http.ResponseWriter, r *http.Request) {
	err := app.DB.Ping(r.Context())
	w.Header().Set("Content-Type", "application/json")

	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "database": "disconnected"})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "available", "database": "connected", "timestamp": time.Now().Format(time.RFC3339)})
}