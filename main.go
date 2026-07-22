package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"mi-api-divisas/config"
	"mi-api-divisas/database"
	"mi-api-divisas/handlers"
	"mi-api-divisas/worker"
)

func main() {
	ctx := context.Background()
	cfg := config.Load()

	if cfg.DatabaseURL == "" {
		log.Fatal("Error: La variable DATABASE_URL no está configurada")
	}

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal("Error conectando a Postgres:", err)
	}
	defer pool.Close()

	if err := database.Migrate(ctx, pool); err != nil {
		log.Fatalf("Error creando tablas: %v", err)
	}

	go worker.Start(ctx, pool, cfg.DataURL)

	app := handlers.NewApp(pool)

	http.HandleFunc("/convert", app.HandleConvert)
	http.HandleFunc("/history", app.HandleHistory)
	http.HandleFunc("/latest", app.HandleLatest)
	http.HandleFunc("/rates/", app.HandleSingleRate)
	http.HandleFunc("/check", app.HandleCheck)

	fmt.Printf("🚀 API de Divisas Robustas iniciada en :%s\n", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, nil))
}
