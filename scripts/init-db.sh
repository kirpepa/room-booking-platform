#!/bin/bash
set -e

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    CREATE DATABASE auth_db;
    CREATE DATABASE room_db;
    CREATE DATABASE availability_db;
    CREATE DATABASE booking_db;
EOSQL
