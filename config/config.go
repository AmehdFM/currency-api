package config

import "os"

type Config struct {
	DatabaseURL string
	DataURL     string
	Port        string
}

func Load() Config {
	return Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		DataURL:     os.Getenv("DATA_URL"),
		Port:        getEnv("PORT", "8080"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
