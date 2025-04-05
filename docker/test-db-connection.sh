#!/bin/bash
set -e

echo "Проверка подключения к PostgreSQL..."

POSTGRES_USER=$(docker exec postgres env | grep POSTGRES_USER | cut -d= -f2 || echo "postgres")
POSTGRES_PASSWORD=$(docker exec postgres env | grep POSTGRES_PASSWORD | cut -d= -f2)
POSTGRES_DB=$(docker exec postgres env | grep POSTGRES_DB | cut -d= -f2 || echo "shortener")

echo "Используется: User=$POSTGRES_USER, DB=$POSTGRES_DB"

for i in $(seq 1 5); do
  echo "Попытка подключения к PostgreSQL ($i/5)..."

  if PGPASSWORD="$POSTGRES_PASSWORD" pg_isready -h localhost -U "$POSTGRES_USER"; then
    echo "PostgreSQL доступен"
    exit 0
  fi

  echo "PostgreSQL пока недоступен, ожидание..."
  sleep 3
done

echo "Не удалось подключиться к PostgreSQL после 5 попыток"
exit 1