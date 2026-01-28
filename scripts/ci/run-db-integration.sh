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
  elif [[ -n "${PWD:-}" && -d "${PWD}" ]]; then
    shared_root="${PWD}"
  elif [[ -d "/runner" ]]; then
    shared_root="/runner"
  else
    shared_root="/tmp"
    echo "Warning: using ${shared_root} for temp dir; this may not be host-mounted for docker socket runners." >&2
  fi
  local tmp_dir
  tmp_dir="$(mktemp -d -p "${shared_root}")"
  local runner_mount_source=""
  local runner_mount_root=""
  if [[ -r /proc/self/mountinfo ]]; then
    runner_mount_source="$(awk '$5=="/runner"{for (i=1;i<=NF;i++) if ($i=="-"){print $(i+2); exit}}' /proc/self/mountinfo)"
    runner_mount_root="$(awk '$5=="/runner"{print $4; exit}' /proc/self/mountinfo)"
  fi
  if [[ -z "${runner_mount_source}" ]]; then
    runner_mount_source="/runner"
  fi
  if [[ -z "${runner_mount_root}" ]]; then
    runner_mount_root="/"
  fi
  local host_tmp_dir="${tmp_dir}"
  if [[ "${tmp_dir}" == /runner/* ]]; then
    local suffix="${tmp_dir#/runner}"
    if [[ "${runner_mount_source}" == /* && "${runner_mount_source}" != /dev/* ]]; then
      host_tmp_dir="${runner_mount_source}${suffix}"
    else
      if [[ "${runner_mount_root}" == "/" ]]; then
        host_tmp_dir="${suffix}"
      else
        host_tmp_dir="${runner_mount_root}${suffix}"
      fi
    fi
  fi
  echo "Running modelgen integration for ${variant} in ${tmp_dir} (host: ${host_tmp_dir}; source: ${runner_mount_source}; root: ${runner_mount_root})"

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
      wait_for_service_port "${compose_cmd}" "goforj-integration-${variant}" "goforj-integration-${variant}_backend" mysql 3306 "MySQL" || exit 1
      docker run --rm \
        -v goforj-go-mod-cache:/go/pkg/mod \
        -v goforj-go-build-cache:/root/.cache/go-build \
        --network "goforj-integration-${variant}_backend" \
        -v "${host_tmp_dir}:/app" \
        -w /app \
        -e DB_DRIVER=mysql \
        -e DB_HOST=mysql \
        -e DB_PORT=3306 \
        -e DB_DATABASE=db \
        -e DB_USERNAME=user \
        -e DB_PASSWORD=password \
        -e DB_HOST_IN_DOCKER=true \
        golang:1.25 \
        sh -c "go test ./internal/modelgen -tags=integration,mysql -v && go test ./internal/migrations -tags=integration,mysql -v"
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
      wait_for_service_port "${compose_cmd}" "goforj-integration-${variant}" "goforj-integration-${variant}_backend" postgres 5432 "Postgres" || exit 1
      docker run --rm \
        -v goforj-go-mod-cache:/go/pkg/mod \
        -v goforj-go-build-cache:/root/.cache/go-build \
        --network "goforj-integration-${variant}_backend" \
        -v "${host_tmp_dir}:/app" \
        -w /app \
        -e DB_DRIVER=postgres \
        -e DB_HOST=postgres \
        -e DB_PORT=5432 \
        -e DB_DATABASE=app \
        -e DB_USERNAME=postgres \
        -e DB_PASSWORD=postgres \
        -e DB_HOST_IN_DOCKER=true \
        golang:1.25 \
        sh -c "go test ./internal/modelgen -tags=integration,postgres -v && go test ./internal/migrations -tags=integration,postgres -v"
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

wait_for_service_port() {
  local compose_cmd="$1"
  local project="$2"
  local network="$3"
  local host="$4"
  local port="$5"
  local label="$6"
  for _ in $(seq 1 30); do
    if docker run --rm --network "${network}" busybox sh -c "nc -z ${host} ${port}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "${label} port ${host}:${port} did not become reachable." >&2
  return 1
}
if [[ $# -ne 1 ]]; then
  echo "usage: $0 <mysql|postgres|sqlite>" >&2
  exit 1
fi
run_variant "$1"
