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
	"github.com/shopspring/decimal" // Instalar con: go get github.com/shopspring/decimal
)

// --- Estructuras de la API Externa ---
type APIResponse struct {
	Success   bool               `json:"success"`
	Timestamp int64              `json:"timestamp"`
	Quotes    map[string]float64 `json:"quotes"`
}

type App struct {
	DB *pgxpool.Pool
}

func main() {
	ctx := context.Background()
	
	// 1. Obtener conexión desde variable de entorno de Dokploy
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

	// 2. Crear tablas automáticamente
	app.initDatabase(ctx)

	// 3. Iniciar Worker (1 vez al día)
	go app.startDailyWorker(ctx)

	// 4. Rutas
	http.HandleFunc("/convert", app.handleConvert)
	http.HandleFunc("/history", app.handleHistory)

	fmt.Println("🚀 API de Divisas Robustas iniciada en :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// initDatabase crea las tablas si no existen
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

// startDailyWorker se ejecuta cada 24 horas
func (app *App) startDailyWorker(ctx context.Context) {
	// Ejecutar inmediatamente al arrancar por primera vez
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

	// Usar una transacción para insertar todo en bloque
	tx, err := app.DB.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)

	for pair, rate := range data.Quotes {
		code := strings.TrimPrefix(pair, "USD")
		if code == "" || len(code) != 3 { continue }

		// 1. Actualizar tabla principal
		tx.Exec(ctx, `INSERT INTO exchange_rates (currency_code, rate_to_base, updated_at)
			VALUES ($1, $2, NOW()) ON CONFLICT (currency_code) 
			DO UPDATE SET rate_to_base = EXCLUDED.rate_to_base, updated_at = NOW();`, code, rate)

		// 2. Guardar en histórico
		tx.Exec(ctx, `INSERT INTO rate_history (currency_code, rate) VALUES ($1, $2);`, code, rate)
	}

	tx.Commit(ctx)
	log.Println("✅ Sincronización terminada.")
}

func (app *App) handleConvert(w http.ResponseWriter, r *http.Request) {
	from := strings.ToUpper(r.URL.Query().Get("from"))
	to := strings.ToUpper(r.URL.Query().Get("to"))
	amount, _ := decimal.NewFromString(r.URL.Query().Get("amount"))

	if amount.IsZero() { amount = decimal.NewFromInt(1) }

	var rateFrom, rateTo decimal.Decimal
	
	err1 := app.DB.QueryRow(r.Context(), "SELECT rate_to_base FROM exchange_rates WHERE currency_code=$1", from).Scan(&rateFrom)
	err2 := app.DB.QueryRow(r.Context(), "SELECT rate_to_base FROM exchange_rates WHERE currency_code=$1", to).Scan(&rateTo)

	if err1 != nil || err2 != nil {
		http.Error(w, "Divisa no encontrada", 404)
		return
	}

	// Cálculo: (Cantidad * Rate_Destino) / Rate_Origen
	result := amount.Mul(rateTo).Div(rateFrom)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"from": from, "to": to, "amount": amount, "result": result.Round(4),
	})
}

func (app *App) handleHistory(w http.ResponseWriter, r *http.Request) {
    // Aquí podrías filtrar por r.URL.Query().Get("start") y "end"
    // Pero por brevedad, devolvemos los últimos 10 del historial
    rows, _ := app.DB.Query(r.Context(), "SELECT currency_code, rate, recorded_at FROM rate_history ORDER BY recorded_at DESC LIMIT 10")
    defer rows.Close()

    var results []map[string]interface{}
    for rows.Next() {
        var code string; var rate float64; var date time.Time
        rows.Scan(&code, &rate, &date)
        results = append(results, map[string]interface{}{"code": code, "rate": rate, "date": date})
    }
    json.NewEncoder(w).Encode(results)
}

// handleLatest devuelve todos los tipos de cambio actuales
func (app *App) handleLatest(w http.ResponseWriter, r *http.Request) {
	rows, err := app.DB.Query(r.Context(), "SELECT currency_code, rate_to_base, updated_at FROM exchange_rates")
	if err != nil {
		http.Error(w, "Error en la base de datos", 500)
		return
	}
	defer rows.Close()

	rates := make(map[string]interface{})
	for rows.Next() {
		var code string
		var rate decimal.Decimal
		var updated time.Time
		rows.Scan(&code, &rate, &updated)
		rates[code] = map[string]interface{}{
			"rate": rate.Round(6),
			"updated_at": updated,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"base": "USD",
		"rates": rates,
	})
}

// handleSingleRate devuelve el precio de una sola moneda especificada en la URL
func (app *App) handleSingleRate(w http.ResponseWriter, r *http.Request) {
	code := strings.ToUpper(strings.TrimPrefix(r.URL.Path, "/rates/"))
	if len(code) != 3 {
		http.Error(w, "Código de moneda inválido", 400)
		return
	}

	var rate decimal.Decimal
	var updated time.Time
	err := app.DB.QueryRow(r.Context(), "SELECT rate_to_base, updated_at FROM exchange_rates WHERE currency_code=$1", code).Scan(&rate, &updated)
	
	if err != nil {
		http.Error(w, "Moneda no encontrada", 404)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code": code,
		"rate": rate.Round(6),
		"base": "USD",
		"updated_at": updated,
	})
}