#!/bin/sh
set -e

echo "Testing database connection..."

# Ждем, пока PostgreSQL станет доступным
until PGPASSWORD=$POSTGRES_PASSWORD psql -h postgres -U $POSTGRES_USER -d $POSTGRES_DB -c '\q'; do
  >&2 echo "PostgreSQL is unavailable - sleeping"
  sleep 1
done

echo "PostgreSQL is up - executing query"
PGPASSWORD=$POSTGRES_PASSWORD psql -h postgres -U $POSTGRES_USER -d $POSTGRES_DB -c "SELECT 'PostgreSQL connection successful' as status;"

echo "Database connection test completed" 