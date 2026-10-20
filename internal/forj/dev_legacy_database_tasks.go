package forj

import (
	"strings"

	"github.com/goforj/goforj/project"
)

// These exact historical identities keep owner-written commands out of compatibility handling.
const (
	legacyMySQLDevWaitCommand       = "docker-compose exec -T mysql sh -c 'while ! mariadb-admin ping -h \"mysql\" --silent; do sleep .5; done; mariadb -h \"mysql\" -uroot -p\"$MARIADB_ROOT_PASSWORD\" -e \"CREATE DATABASE IF NOT EXISTS \\`$MARIADB_DATABASE\\`;\"'"
	legacySocketMySQLDevWaitCommand = "docker-compose exec -T mysql sh -c 'set -e; attempt=0; until mariadb-admin ping --protocol=socket --connect-timeout=1 --silent; do attempt=$((attempt + 1)); if [ \"$attempt\" -ge 60 ]; then echo \"MySQL did not become ready after 60 attempts\" >&2; mariadb-admin ping --protocol=socket --connect-timeout=1; exit 1; fi; sleep .5; done; mariadb --protocol=socket -uroot -p\"$MARIADB_ROOT_PASSWORD\" -e \"CREATE DATABASE IF NOT EXISTS \\`$MARIADB_DATABASE\\`;\"'"
	legacyPostgresDevWaitCommand    = "docker-compose exec -T postgres sh -c 'export PGPASSWORD=\"$POSTGRES_PASSWORD\"; until pg_isready -h \"postgres\" -p 5432; do sleep .5; done; for db in \"$POSTGRES_DB\"; do psql -U \"$POSTGRES_USER\" -h \"postgres\" -d postgres -v ON_ERROR_STOP=1 -tc \"SELECT 1 FROM pg_database WHERE datname = '\\''$db'\\''\" | grep -q 1 || psql -U \"$POSTGRES_USER\" -h \"postgres\" -d postgres -v ON_ERROR_STOP=1 -c \"CREATE DATABASE \\\"$db\\\";\"; done'"
)

// legacyDevDatabaseTaskService recognizes only unmodified generated readiness tasks.
func legacyDevDatabaseTaskService(task project.DevTask) string {
	if strings.TrimSpace(task.Name) != "Waiting for Database to be ready" {
		return ""
	}
	switch normalizeDevComposeExecutable(task.Cmd) {
	case legacyMySQLDevWaitCommand, legacySocketMySQLDevWaitCommand, strings.ReplaceAll(legacyMySQLDevWaitCommand, "mariadb", "mysql"):
		return "mysql"
	case legacyPostgresDevWaitCommand:
		return "postgres"
	default:
		return ""
	}
}
