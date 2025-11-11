#!/bin/bash
set -e

# This script runs during container initialization
# It grants monitoring privileges to the application user

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    -- Grant monitoring privileges for exporters
    GRANT pg_monitor TO "$POSTGRES_USER";

    -- Ensure the user can connect
    SELECT 'Database initialization complete' AS status;
EOSQL
