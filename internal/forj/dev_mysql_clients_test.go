package forj

import (
	"os/exec"
	"strings"
	"testing"
)

// TestDevMySQLClientCompatibility protects both official image environment families and client names.
func TestDevMySQLClientCompatibility(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell unavailable")
	}
	for _, test := range []struct {
		name, clients, password, user, diagnostic string
		environment                               []string
	}{
		{name: "mariadb", clients: "mariadb", password: "maria-root", user: "maria_app", environment: []string{"MARIADB_ROOT_PASSWORD=maria-root", "MARIADB_USER=maria_app"}},
		{name: "mysql", clients: "mysql", password: "mysql-root", user: "mysql_app", environment: []string{"MYSQL_ROOT_PASSWORD=mysql-root", "MYSQL_USER=mysql_app"}},
		{name: "legacy-mariadb", clients: "mariadb mysql", password: "legacy-root", user: "legacy_app", environment: []string{"MYSQL_ROOT_PASSWORD=legacy-root", "MYSQL_USER=legacy_app"}},
		{name: "mariadb-precedence", clients: "mariadb mysql", password: "maria-root", user: "maria_app", environment: []string{"MARIADB_ROOT_PASSWORD=maria-root", "MARIADB_USER=maria_app", "MYSQL_ROOT_PASSWORD=other-root", "MYSQL_USER=other_app"}},
		{name: "explicit-empty-password", clients: "mysql", user: "app", environment: []string{"MARIADB_ROOT_PASSWORD=", "MYSQL_ROOT_PASSWORD=other-root", "MYSQL_USER=app"}},
		{name: "missing-client", password: "root", user: "app", environment: []string{"MYSQL_ROOT_PASSWORD=root", "MYSQL_USER=app"}, diagnostic: "Neither mariadb nor mysql client is available"},
		{name: "missing-password", clients: "mysql", environment: []string{"MYSQL_USER=app"}, diagnostic: "Set MARIADB_ROOT_PASSWORD or MYSQL_ROOT_PASSWORD"},
	} {
		for _, preparation := range []bool{false, true} {
			name := test.name + "/readiness"
			script := devMySQLReadinessScript
			if preparation {
				name = test.name + "/preparation"
				script = devMySQLPreparationScript
			}
			t.Run(name, func(t *testing.T) {
				client := `record_client() {
    [ "$MYSQL_PWD" = "$EXPECTED_PASSWORD" ] || return 91
    [ "$1" = --protocol=socket ] || return 92
    for argument in "$@"; do
        case "$argument" in -p*|--password*) return 93 ;; esac
    done
    if [ "$CLIENT_OPERATION" = readiness ]; then
        [ "$#" = 7 ] && [ "$7" = 'SELECT @@skip_networking' ] || return 94
        printf '%s\n' 0
    else
        printf '%s\n' "$@"
        while IFS= read -r line; do printf '%s\n' "$line"; done
    fi
}
`
				for _, executable := range strings.Fields(test.clients) {
					// When both names exist, the MariaDB client must take precedence.
					if executable == "mysql" && strings.Contains(test.clients, "mariadb") {
						client += "mysql() { return 95; }\n"
					} else {
						client += executable + "() { record_client \"$@\"; }\n"
					}
				}
				operation := "readiness"
				if preparation {
					operation = "preparation"
				}
				command := exec.CommandContext(t.Context(), shell, "-c", client+script, "mysql-compatibility", "billing", "db")
				// An isolated PATH prevents an installed host client from hiding a missing fixture client.
				command.Env = append([]string{"PATH=" + t.TempDir(), "EXPECTED_PASSWORD=" + test.password, "CLIENT_OPERATION=" + operation}, test.environment...)
				output, err := command.CombinedOutput()
				if test.diagnostic != "" {
					if err == nil || !strings.Contains(string(output), test.diagnostic) {
						t.Fatalf("error=%v output=%s, want %s", err, output, test.diagnostic)
					}
					return
				}
				if err != nil {
					t.Fatalf("error=%v output=%s", err, output)
				}
				if preparation {
					for _, database := range []string{"billing", "db"} {
						if !strings.Contains(string(output), "GRANT ALL PRIVILEGES ON `"+database+"`.* TO '"+test.user+"'@'%';") {
							t.Fatalf("database grant missing or used wrong environment: %s", output)
						}
					}
					if strings.Count(string(output), "FLUSH PRIVILEGES;") != 1 {
						t.Fatalf("expected one privilege flush: %s", output)
					}
				}
			})
		}
	}
}

// TestDevMySQLPreparationRequiresApplicationUser rejects incomplete provisioning before executing SQL.
func TestDevMySQLPreparationRequiresApplicationUser(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell unavailable")
	}
	command := exec.CommandContext(t.Context(), shell, "-c", "mysql() { printf '%s\\n' 'unexpected SQL execution'; }\n"+devMySQLPreparationScript, "mysql-preparation", "db")
	command.Env = []string{"PATH=" + t.TempDir(), "MYSQL_ROOT_PASSWORD=fake"}
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "Set MARIADB_USER or MYSQL_USER") || strings.Contains(string(output), "unexpected SQL execution") {
		t.Fatalf("error=%v output=%s", err, output)
	}
}
