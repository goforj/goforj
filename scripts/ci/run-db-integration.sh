#!/usr/bin/env bash
set -euo pipefail

run_variant() {
  local variant="$1"
  local compose_cmd="docker compose"
  if command -v docker-compose >/dev/null 2>&1; then
    compose_cmd="docker-compose"
  fi
  local shared_root=""
  if [[ -n "${RUNNER_TEMP:-}" && -d "${RUNNER_TEMP}" ]]; then
    shared_root="${RUNNER_TEMP}"
  elif [[ -n "${GITHUB_WORKSPACE:-}" && -d "${GITHUB_WORKSPACE}" ]]; then
    shared_root="${GITHUB_WORKSPACE}"
  elif [[ -d "/runner" ]]; then
    shared_root="/runner"
  else
    shared_root="/tmp"
    echo "Warning: using ${shared_root} for temp dir; this may not be host-mounted for docker socket runners." >&2
  fi
  local tmp_dir
  tmp_dir="$(mktemp -d -p "${shared_root}")"
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
      docker run --rm \
        -v "${GOMODCACHE:-$HOME/go/pkg/mod}:/go/pkg/mod" \
        -v "${GOCACHE:-$HOME/.cache/go-build}:/root/.cache/go-build" \
        --network "goforj-integration-${variant}_backend" \
        -v "${tmp_dir}:/app" \
        -w /app \
        -e DB_DRIVER=mysql \
        -e DB_HOST=mysql \
        -e DB_PORT=3306 \
        -e DB_DATABASE=db \
        -e DB_USERNAME=user \
        -e DB_PASSWORD=password \
        golang:1.23 \
        go test ./internal/modelgen -tags=integration,mysql -v
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
      docker run --rm \
        -v "${GOMODCACHE:-$HOME/go/pkg/mod}:/go/pkg/mod" \
        -v "${GOCACHE:-$HOME/.cache/go-build}:/root/.cache/go-build" \
        --network "goforj-integration-${variant}_backend" \
        -v "${tmp_dir}:/app" \
        -w /app \
        -e DB_DRIVER=postgres \
        -e DB_HOST=postgres \
        -e DB_PORT=5432 \
        -e DB_DATABASE=app \
        -e DB_USERNAME=postgres \
        -e DB_PASSWORD=postgres \
        golang:1.23 \
        go test ./internal/modelgen -tags=integration,postgres -v
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
