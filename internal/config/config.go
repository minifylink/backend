package config

import (
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	Env string `envconfig:"ENV" default:"local"`
	PostgresConfig
	HTTPServer
}

type PostgresConfig struct {
	Host     string `envconfig:"POSTGRES_HOST" default:"postgres"`
	Port     string `envconfig:"POSTGRES_PORT" default:"5432"`
	Username string `envconfig:"POSTGRES_USER" default:"postgres"`
	Password string `envconfig:"POSTGRES_PASSWORD" default:"postgres"`
	DBName   string `envconfig:"POSTGRES_DB" default:"shortener"`
	SSLMode  string `envconfig:"POSTGRES_SSL_MODE" default:"disable"`
}

type HTTPServer struct {
	Address     string        `envconfig:"HTTP_SERVER_ADDRESS" default:"0.0.0.0:8082"`
	Timeout     time.Duration `envconfig:"HTTP_SERVER_TIMEOUT" default:"4s"`
	IdleTimeout time.Duration `envconfig:"HTTP_SERVER_IDLE_TIMEOUT" default:"60s"`
}

func LoadEnv() {
	envFile := os.Getenv("ENV_FILE")
	if envFile == "" {
		envFile = ".env"
	}

	err := godotenv.Load(envFile)
	if err != nil {
		log.Printf("Warning: could not load .env file: %v", err)
	}
}

func MustLoad() *Config {
	LoadEnv()

	var cfg Config

	err := envconfig.Process("", &cfg)
	if err != nil {
		log.Fatalf("Failed to process config: %v", err)
	}

	return &cfg
}
