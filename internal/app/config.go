package app

import "os"

type Config struct {
	Addr          string
	DatabasePath  string
	MigrationsDir string
}

func LoadConfig() Config {
	return Config{
		Addr:          env("ADDR", ":8080"),
		DatabasePath:  env("DATABASE_PATH", "data/app.db"),
		MigrationsDir: env("MIGRATIONS_DIR", "migrations"),
	}
}

func env(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
