package config

import "os"

func Value(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func MySQLDSN() string {
	return Value("CWATCH_DSN", "root@tcp(127.0.0.1:3306)/cwatch?parseTime=true&charset=utf8mb4&loc=UTC")
}
