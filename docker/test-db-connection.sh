#!/bin/sh
set -e

echo "Ожидание готовности PostgreSQL..."
sleep 5

for i in $(seq 1 30); do
  if PGPASSWORD=${POSTGRES_PASSWORD} psql -h postgres -U ${POSTGRES_USER} -d ${POSTGRES_DB} -c "SELECT 1" >/dev/null 2>&1; then
    echo "Подключение к PostgreSQL успешно!"
    exit 0
  fi
  echo "Попытка подключения к Postgres не удалась, повторяю... ($i/30)"
  sleep 2
done

echo "Postgres не доступен"
exit 1