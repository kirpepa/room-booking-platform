#!/bin/bash
set -e

BASE_URL="${1:-http://localhost:8080}"

echo "Seeding TEST_TASK_MODE users via dummyLogin..."

curl --fail-with-body --silent --show-error -o /dev/null -X POST "${BASE_URL}/dummyLogin" \
  -H "Content-Type: application/json" \
  -d '{"role":"admin"}'

curl --fail-with-body --silent --show-error -o /dev/null -X POST "${BASE_URL}/dummyLogin" \
  -H "Content-Type: application/json" \
  -d '{"role":"user"}'

echo "Seed completed successfully."
