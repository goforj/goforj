set -eu
export PGPASSWORD="$POSTGRES_PASSWORD"
for db in "$@"; do
    exists=$(psql -U "$POSTGRES_USER" -h "postgres" -d postgres -v ON_ERROR_STOP=1 -tAc "SELECT 1 FROM pg_database WHERE datname = '$db'")
    if [ "$exists" != "1" ]; then
        psql -U "$POSTGRES_USER" -h "postgres" -d postgres -v ON_ERROR_STOP=1 -c "CREATE DATABASE \"$db\";"
    fi
done
