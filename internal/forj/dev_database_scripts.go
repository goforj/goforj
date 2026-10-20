package forj

import _ "embed"

// devMySQLReadinessScript rejects unauthenticated and temporary initialization servers before provisioning.
//
//go:embed dev_scripts/mysql_ready.sh
var devMySQLReadinessScript string

// devMySQLPreparationScript authenticates inside the container without putting passwords in host argv.
//
//go:embed dev_scripts/mysql_prepare.sh
var devMySQLPreparationScript string

// devPostgresPreparationScript preserves authenticated local TCP and visible SQL failures.
//
//go:embed dev_scripts/postgres_prepare.sh
var devPostgresPreparationScript string
