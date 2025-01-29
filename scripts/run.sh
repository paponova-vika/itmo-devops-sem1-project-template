#!/bin/bash

set -e

echo "Building the application..."
cd main
go mod tidy
go build -o app main.go


echo "Starting the application in background..."
nohup ./app > app.log 2>&1 &

echo "Application started successfully in background."
echo "Logs are being written to app.log"