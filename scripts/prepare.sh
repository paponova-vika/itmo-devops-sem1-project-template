#!/bin/bash

# prepare.sh
# This script installs dependencies and prepares the PostgreSQL database.

set -e

echo "Installing necessary packages..."
sudo apt-get update
sudo apt-get install -y golang-go postgresql postgresql-contrib unzip curl

echo "Starting PostgreSQL service..."
sudo service postgresql start

echo "Configuring PostgreSQL for external access..."

# Update postgresql.conf to listen on all interfaces
PG_CONF="/etc/postgresql/$(ls /etc/postgresql)/main/postgresql.conf"
sudo sed -i "s/^#listen_addresses = 'localhost'/listen_addresses = '*'/g" $PG_CONF

# Update pg_hba.conf to allow connections from any IP (or restrict to your network)
PG_HBA="/etc/postgresql/$(ls /etc/postgresql)/main/pg_hba.conf"
echo "host    all             all             0.0.0.0/0               md5" | sudo tee -a $PG_HBA
echo "host    all             all             ::/0                    md5" | sudo tee -a $PG_HBA

# Restart PostgreSQL to apply changes
sudo service postgresql restart

echo "Setting up database, user, and table..."

# Create database, user, table, and grant permissions
sudo -u postgres psql <<EOF
CREATE DATABASE "project-sem-1";
CREATE USER validator WITH PASSWORD 'val1dat0r";
GRANT ALL PRIVILEGES ON DATABASE "project-sem-1" TO validator;

\c "project-sem-1"

-- Grant privileges on schema public
ALTER SCHEMA public OWNER TO validator;
GRANT ALL ON SCHEMA public TO validator;

-- Create the table
CREATE TABLE IF NOT EXISTS prices (
    id SERIAL PRIMARY KEY,
    product_name TEXT NOT NULL,
    category TEXT NOT NULL,
    price NUMERIC NOT NULL,
    creation_date DATE NOT NULL
);

-- Grant privileges on the table and sequences
GRANT ALL PRIVILEGES ON TABLE prices TO validator;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO validator;
EOF

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
