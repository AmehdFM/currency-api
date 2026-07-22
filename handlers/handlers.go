package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

type App struct {
	DB *pgxpool.Pool
}

func NewApp(db *pgxpool.Pool) *App {
	return &App{DB: db}
}

func (app *App) HandleConvert(w http.ResponseWriter, r *http.Request) {
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

func (app *App) HandleHistory(w http.ResponseWriter, r *http.Request) {
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

func (app *App) HandleLatest(w http.ResponseWriter, r *http.Request) {
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

func (app *App) HandleSingleRate(w http.ResponseWriter, r *http.Request) {
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

func (app *App) HandleCheck(w http.ResponseWriter, r *http.Request) {
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
