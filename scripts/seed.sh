#!/bin/bash
set -e

BASE_URL="${1:-http://localhost:8080}"

echo "Seeding dummy users via dummyLogin..."

# Call dummyLogin for admin - this ensures the dummy admin user exists
echo "Creating dummy admin..."
curl -s -X POST "${BASE_URL}/dummyLogin" \
  -H "Content-Type: application/json" \
  -d '{"role":"admin"}' | head -c 200
echo ""

# Call dummyLogin for user - this ensures the dummy user exists
echo "Creating dummy user..."
curl -s -X POST "${BASE_URL}/dummyLogin" \
  -H "Content-Type: application/json" \
  -d '{"role":"user"}' | head -c 200
echo ""

echo "Seed completed successfully."
