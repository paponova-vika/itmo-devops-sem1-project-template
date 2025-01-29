#!/bin/bash

# prepare.sh
# This script installs dependencies and prepares the PostgreSQL database.

set -e

PGHOST="localhost"
PGPORT=5432
PGUSER="validator"
PGPASSWORD="val1dat0r"
DBNAME="project-sem-1"

echo "Checking if PostgreSQL is running..."
if pg_isready -q -h "$PGHOST" -p "$PGPORT" -U "$PGUSER"; then
    echo "PostgreSQL is already running. Skipping startup."
else
    echo "PostgreSQL is not running. Installing and starting..."
    sudo apt-get update
    sudo apt-get install -y golang-go postgresql postgresql-contrib unzip curl
    sudo service postgresql start
fi

echo "Checking if database 'project-sem-1' exists..."
DB_EXISTS=$(PGPASSWORD="$PGPASSWORD" psql -U "$PGUSER" -h "$PGHOST" -p "$PGPORT" -d "$DBNAME" -tAc "SELECT 1 FROM pg_database WHERE datname='project-sem-1'")

if [ "$DB_EXISTS" == "1" ]; then
    echo "Database 'project-sem-1' already exists. Skipping creation."
else
    echo "Creating database 'project-sem-1'..."
    psql -U "$PGUSER" -h "$PGHOST" -p "$PGPORT" -d "$DBNAME" <<EOF
    CREATE DATABASE "project-sem-1";
    CREATE USER validator WITH PASSWORD 'val1dat0r';
    GRANT ALL PRIVILEGES ON DATABASE "project-sem-1" TO validator;
EOF
fi

echo "Checking if table 'prices' exists..."
TABLE_EXISTS=$(PGPASSWORD="$PGPASSWORD" psql -U "$PGUSER" -h "$PGHOST" -p "$PGPORT" -d "$DBNAME" -d "project-sem-1" -tAc "SELECT to_regclass('public.prices')")

if [ "$TABLE_EXISTS" == "public.prices" ]; then
    echo "Table 'prices' already exists. Skipping creation."
else
    echo "Creating table 'prices'..."
    PGPASSWORD="$PGPASSWORD" psql -U "$PGUSER" -h "$PGHOST" -p "$PGPORT" -d "$DBNAME" <<EOF
    ALTER SCHEMA public OWNER TO validator;
    GRANT ALL ON SCHEMA public TO validator;

    CREATE TABLE prices (
        id SERIAL PRIMARY KEY,
        product_name TEXT NOT NULL,
        category TEXT NOT NULL,
        price NUMERIC NOT NULL,
        creation_date DATE NOT NULL
    );

    GRANT ALL PRIVILEGES ON TABLE prices TO validator;
    GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO validator;
EOF
fi

echo "Checking if application is already running..."
APP_RUNNING=$(pgrep -f "./app")

if [ -n "$APP_RUNNING" ]; then
    echo "Application is already running (PID: $APP_RUNNING). Skipping startup."
    exit 0
fi

echo "Installing Go dependencies..."
if [ -f go.mod ]; then
    go mod tidy
else
    go mod init project-sem-1
fi

echo "Building the application..."
go build -o app .

echo "PostgreSQL is now accessible externally and locally with proper permissions."
echo "Preparation complete."
