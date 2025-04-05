#!/bin/sh
set -e

echo "Ожидание готовности PostgreSQL..."
sleep 5

for i in $(seq 1 5); do
  if pg_isready -h postgres -U ${POSTGRES_USER} -d ${POSTGRES_DB}; then
    echo "PostgreSQL сервер доступен!"
    exit 0
  fi
  echo "Попытка подключения к PostgreSQL не удалась, повторяю... ($i/5)"
  sleep 2
done

echo "PostgreSQL сервер не доступен"
exit 1