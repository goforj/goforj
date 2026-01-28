#!/usr/bin/env bash
set -euo pipefail

run_variant() {
  local variant="$1"
  local compose_cmd="docker compose"
  if command -v docker-compose >/dev/null 2>&1; then
    compose_cmd="docker-compose"
  fi
  local tmp_dir
  tmp_dir="$(mktemp -d)"
  echo "Running modelgen integration for ${variant} in ${tmp_dir}"

  cd "${tmp_dir}"
  case "${variant}" in
    mysql)
      printf '%s\n' \
        'project_name: IntegrationMySQL' \
        'module_name: github.com/test/project' \
        'updated_at: 2026-01-01 00:00:00 UTC' \
        'components:' \
        '  cli: true' \
        '  docker: true' \
        '  database_mysql: true' \
        > .goforj.yml
      ;;
    postgres)
      printf '%s\n' \
        'project_name: IntegrationPostgres' \
        'module_name: github.com/test/project' \
        'updated_at: 2026-01-01 00:00:00 UTC' \
        'components:' \
        '  cli: true' \
        '  docker: true' \
        '  database_postgres: true' \
        > .goforj.yml
      ;;
    sqlite)
      printf '%s\n' \
        'project_name: IntegrationSQLite' \
        'module_name: github.com/test/project' \
        'updated_at: 2026-01-01 00:00:00 UTC' \
        'components:' \
        '  cli: true' \
        '  docker: true' \
        '  database_sqlite: true' \
        > .goforj.yml
      ;;
    *)
      echo "Unknown variant: ${variant}" >&2
      exit 1
      ;;
  esac

  forj render

  if [[ -f docker-compose.yml || -f docker-compose.yaml ]]; then
    COMPOSE_PROJECT_NAME="goforj-integration-${variant}" ${compose_cmd} down -v --remove-orphans || true
    COMPOSE_PROJECT_NAME="goforj-integration-${variant}" ${compose_cmd} up -d
  fi

  case "${variant}" in
    mysql)
      echo "Waiting for mysql to be ready..."
      local mysql_ready="false"
      for _ in $(seq 1 30); do
        if COMPOSE_PROJECT_NAME="goforj-integration-${variant}" ${compose_cmd} exec -T mysql mysqladmin ping -uroot -proot >/dev/null 2>&1; then
          mysql_ready="true"
          break
        fi
        sleep 1
      done
      if [[ "${mysql_ready}" != "true" ]]; then
        echo "MySQL did not become ready in time." >&2
        exit 1
      fi
      DB_DRIVER=mysql \
        DB_HOST_INTEGRATION=127.0.0.1 \
        DB_PORT_INTEGRATION=3306 \
        DB_DATABASE_INTEGRATION=db \
        DB_USERNAME_INTEGRATION=user \
        DB_PASSWORD_INTEGRATION=password \
        forj test:integration -v
      ;;
    postgres)
      echo "Waiting for postgres to be ready..."
      local postgres_ready="false"
      for _ in $(seq 1 30); do
        if COMPOSE_PROJECT_NAME="goforj-integration-${variant}" ${compose_cmd} exec -T postgres pg_isready -U postgres >/dev/null 2>&1; then
          postgres_ready="true"
          break
        fi
        sleep 1
      done
      if [[ "${postgres_ready}" != "true" ]]; then
        echo "Postgres did not become ready in time." >&2
        exit 1
      fi
      DB_DRIVER=postgres \
        DB_HOST_INTEGRATION=127.0.0.1 \
        DB_PORT_INTEGRATION=5432 \
        DB_DATABASE_INTEGRATION=app \
        DB_USERNAME_INTEGRATION=postgres \
        DB_PASSWORD_INTEGRATION=postgres \
        forj test:integration -v
      ;;
    sqlite)
      DB_DRIVER=sqlite DB_DATABASE=./_data/sqlite/app.db forj test:integration -v
      ;;
  esac

  if [[ -f docker-compose.yml || -f docker-compose.yaml ]]; then
    COMPOSE_PROJECT_NAME="goforj-integration-${variant}" ${compose_cmd} down -v --remove-orphans || true
  fi

  rm -rf "${tmp_dir}"
}
if [[ $# -ne 1 ]]; then
  echo "usage: $0 <mysql|postgres|sqlite>" >&2
  exit 1
fi
run_variant "$1"
