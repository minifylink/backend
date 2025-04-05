#!/bin/bash
set -e

echo "Initializing database: $POSTGRES_DB with user: $POSTGRES_USER"

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname postgres <<-EOSQL
  DO \$\$
  BEGIN
    IF NOT EXISTS (SELECT FROM pg_database WHERE datname = '$POSTGRES_DB') THEN
      CREATE DATABASE "$POSTGRES_DB";
    END IF;
  END
  \$\$;
EOSQL

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
  CREATE TABLE IF NOT EXISTS links (
    id SERIAL PRIMARY KEY,
    original_link TEXT NOT NULL,
    short_id VARCHAR(20) NOT NULL UNIQUE,
    created_at TIMESTAMP NOT NULL DEFAULT now()
  );

  CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    telegram_id BIGINT NOT NULL UNIQUE,
    created_at TIMESTAMP NOT NULL DEFAULT now()
  );

  CREATE TABLE IF NOT EXISTS analytics (
    id SERIAL PRIMARY KEY,
    link_id INTEGER NOT NULL REFERENCES links(id) ON DELETE CASCADE,
    timestamp TIMESTAMP NOT NULL DEFAULT now(),
    country VARCHAR(50),
    device VARCHAR(50),
    browser VARCHAR(50)
  );
EOSQL

echo "Database initialization completed successfully"
