package testkit

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestLoadRenderedComposeInterpolatesEnvAndHostOverrides(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, ".env"), []byte(""+
		"DB_DATABASE=db\n"+
		"DB_HOST=localhost\n"+
		"DB_PORT=43061\n"+
		"DB_USERNAME=user\n"+
		"DB_PASSWORD=password\n"+
		"DB_ROOT_PASSWORD=root\n"+
		"IP_ADDRESS=0.0.0.0\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(`
services:
  mysql:
    build:
      context: ./containers/mariadb
      args:
        - INNODB_BUFFER_POOL_SIZE=${INNODB_BUFFER_POOL_SIZE:-512MB}
    ports:
      - ${IP_ADDRESS:-0.0.0.0}:${DB_PORT:-3306}:3306
    environment:
      - MARIADB_DATABASE=${DB_DATABASE}
      - MARIADB_USER=${DB_USERNAME}
      - MARIADB_PASSWORD=${DB_PASSWORD}
      - MARIADB_ROOT_PASSWORD=${DB_ROOT_PASSWORD}
`), 0o644); err != nil {
		t.Fatalf("write docker-compose.yml: %v", err)
	}

	model, err := loadRenderedCompose(projectDir)
	if err != nil {
		t.Fatalf("load rendered compose: %v", err)
	}
	mysql, ok := model.Services["mysql"]
	if !ok {
		t.Fatal("expected mysql service")
	}
	if mysql.Build == nil {
		t.Fatal("expected mysql build config")
	}
	if got := mysql.Build.Args["INNODB_BUFFER_POOL_SIZE"]; got != "512MB" {
		t.Fatalf("build arg = %q, want %q", got, "512MB")
	}
	if got := mysql.Environment["MARIADB_DATABASE"]; got != "db" {
		t.Fatalf("database env = %q, want %q", got, "db")
	}
	if got := mysql.Environment["MARIADB_USER"]; got != "user" {
		t.Fatalf("username env = %q, want %q", got, "user")
	}
	if got := mysql.Ports[0]; got != "0.0.0.0:43061:3306" {
		t.Fatalf("port mapping = %q, want %q", got, "0.0.0.0:43061:3306")
	}
}

func TestLoadRenderedComposeInterpolatesNestedDefaultPorts(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, ".env"), []byte("IP_ADDRESS=0.0.0.0\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(`
services:
  mysql:
    ports:
      - ${IP_ADDRESS:-0.0.0.0}:${DB_MYSQL_PORT:-${DB_PORT:-3306}}:3306
  postgres:
    ports:
      - ${IP_ADDRESS:-0.0.0.0}:${DB_POSTGRES_PORT:-${DB_PORT:-5432}}:5432
`), 0o644); err != nil {
		t.Fatalf("write docker-compose.yml: %v", err)
	}

	model, err := loadRenderedCompose(projectDir)
	if err != nil {
		t.Fatalf("load rendered compose: %v", err)
	}
	if got := model.Services["mysql"].Ports[0]; got != "0.0.0.0:3306:3306" {
		t.Fatalf("mysql port mapping = %q, want %q", got, "0.0.0.0:3306:3306")
	}
	if got := model.Services["postgres"].Ports[0]; got != "0.0.0.0:5432:5432" {
		t.Fatalf("postgres port mapping = %q, want %q", got, "0.0.0.0:5432:5432")
	}
}

// TestLoadRenderedComposeHonorsExactProfiles verifies dormant services stay excluded until an exact profile token enables them.
func TestLoadRenderedComposeHonorsExactProfiles(t *testing.T) {
	tests := []struct {
		name     string
		profiles string
		want     string
	}{
		{name: "profile assignment absent", want: "mysql"},
		{name: "partial token", profiles: "redis-debug", want: "mysql"},
		{name: "exact token", profiles: "redis", want: "mysql,redis"},
		{name: "multiple exact tokens", profiles: "rustfs,opensearch", want: "mysql,opensearch,rustfs"},
		{name: "alternate service profile", profiles: "storage", want: "mysql,rustfs"},
		{name: "wildcard", profiles: "*", want: "mysql,opensearch,redis,rustfs"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectDir := t.TempDir()
			environment := "IP_ADDRESS=0.0.0.0\n"
			if test.profiles != "" {
				environment += "COMPOSE_PROFILES=" + test.profiles + "\n"
			}
			if err := os.WriteFile(filepath.Join(projectDir, ".env"), []byte(environment), 0o644); err != nil {
				t.Fatalf("write .env: %v", err)
			}
			if err := os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(`
services:
  mysql:
    ports:
      - 3306:3306
  redis:
    profiles: [redis]
    ports:
      - 6379:6379
  rustfs:
    profiles: [rustfs, storage]
    ports:
      - 9000:9000
  opensearch:
    profiles: [opensearch]
    ports:
      - 9200:9200
`), 0o644); err != nil {
				t.Fatalf("write docker-compose.yml: %v", err)
			}

			model, err := loadRenderedCompose(projectDir)
			if err != nil {
				t.Fatalf("load rendered compose: %v", err)
			}
			names := make([]string, 0, len(model.Services))
			for name := range model.Services {
				names = append(names, name)
			}
			sort.Strings(names)
			if got := strings.Join(names, ","); got != test.want {
				t.Fatalf("active services = %q, want %q", got, test.want)
			}
		})
	}
}

// TestLoadRenderedComposeHonorsProcessProfilePrecedence matches Compose when a test invocation overrides project dotenv state.
func TestLoadRenderedComposeHonorsProcessProfilePrecedence(t *testing.T) {
	projectDir := t.TempDir()
	t.Setenv("COMPOSE_PROFILES", "rustfs")
	if err := os.WriteFile(filepath.Join(projectDir, ".env"), []byte("COMPOSE_PROFILES=redis\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(`
services:
  mysql:
    image: mariadb:11
  redis:
    profiles: [redis]
    image: redis:7.4
  rustfs:
    profiles: [rustfs]
    image: rustfs/rustfs:1.0.0-beta.10
`), 0o644); err != nil {
		t.Fatalf("write docker-compose.yml: %v", err)
	}

	model, err := loadRenderedCompose(projectDir)
	if err != nil {
		t.Fatalf("load rendered compose: %v", err)
	}
	names := make([]string, 0, len(model.Services))
	for name := range model.Services {
		names = append(names, name)
	}
	sort.Strings(names)
	if got := strings.Join(names, ","); got != "mysql,rustfs" {
		t.Fatalf("process-selected services = %q, want process profiles to override .env", got)
	}
}

func TestRenderedComposeStackEnvOverrides(t *testing.T) {
	stack := &RenderedComposeStack{
		services: map[string]*StartedContainer{
			"mysql":  {Host: "127.0.0.1", Port: "33061"},
			"redis":  {Host: "127.0.0.1", Port: "63791"},
			"rustfs": {Host: "127.0.0.1", Port: "49000"},
		},
	}

	overrides := stack.EnvOverrides()
	if got := overrides["DB_HOST"]; got != "127.0.0.1" {
		t.Fatalf("DB_HOST = %q, want %q", got, "127.0.0.1")
	}
	if got := overrides["DB_PORT"]; got != "33061" {
		t.Fatalf("DB_PORT = %q, want %q", got, "33061")
	}
	if got := overrides["REDIS_PORT"]; got != "63791" {
		t.Fatalf("REDIS_PORT = %q, want %q", got, "63791")
	}
	if got := overrides["STORAGE_ENDPOINT"]; got != "http://127.0.0.1:49000" {
		t.Fatalf("STORAGE_ENDPOINT = %q, want %q", got, "http://127.0.0.1:49000")
	}
}

// TestComposeServiceEnabledForRenderedTests honors the explicit standalone-tool exclusion marker.
func TestComposeServiceEnabledForRenderedTests(t *testing.T) {
	disabled := false
	enabled := true
	if !composeServiceEnabledForRenderedTests(composeService{}) {
		t.Fatal("ordinary Compose service was excluded from rendered tests")
	}
	if composeServiceEnabledForRenderedTests(composeService{RenderedTest: &disabled}) {
		t.Fatal("explicitly excluded Compose service remained enabled for rendered tests")
	}
	if !composeServiceEnabledForRenderedTests(composeService{RenderedTest: &enabled}) {
		t.Fatal("explicitly enabled Compose service was excluded from rendered tests")
	}
}

func TestComposeServiceContainerPort(t *testing.T) {
	port, err := composeServiceContainerPort(composeService{
		Ports: []string{"0.0.0.0:5432:5432"},
	})
	if err != nil {
		t.Fatalf("composeServiceContainerPort returned error: %v", err)
	}
	if got := string(port.Container); got != "5432/tcp" {
		t.Fatalf("container port = %q, want %q", got, "5432/tcp")
	}
	if got := port.Binding.HostPort; got != "5432" {
		t.Fatalf("host port = %q, want %q", got, "5432")
	}
}

func TestPrepareRenderedComposeTestEnvRemovesEnvHostAndAllocatesPorts(t *testing.T) {
	projectDir := t.TempDir()
	t.Setenv("FORJ_INTEGRATION_PORT_RANGE_START", "47000")
	t.Setenv("FORJ_INTEGRATION_PORT_RANGE_END", "47010")
	if err := os.WriteFile(filepath.Join(projectDir, ".env"), []byte(""+
		"DB_HOST=localhost\n"+
		"DB_PORT=3306\n"+
		"REDIS_HOST=localhost\n"+
		"REDIS_PORT=6379\n"+
		"RUSTFS_API_PORT=9000\n"+
		"STORAGE_ENDPOINT=http://rustfs:9000\n"+
		"COMPOSE_PROFILES=redis,rustfs\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env.host"), []byte("DB_PORT=9999\n"), 0o644); err != nil {
		t.Fatalf("write .env.host: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(`
services:
  mysql:
    ports:
      - ${IP_ADDRESS:-0.0.0.0}:${DB_PORT}:3306
  redis:
    profiles: [redis]
    ports:
      - ${IP_ADDRESS:-0.0.0.0}:${REDIS_PORT}:6379
  rustfs:
    profiles: [rustfs]
    ports:
      - ${DEV_SERVICE_IP_ADDRESS:-127.0.0.1}:${RUSTFS_API_PORT}:9000
`), 0o644); err != nil {
		t.Fatalf("write docker-compose.yml: %v", err)
	}

	if err := prepareRenderedComposeTestEnv(projectDir); err != nil {
		t.Fatalf("prepareRenderedComposeTestEnv: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, ".env.host")); !os.IsNotExist(err) {
		t.Fatalf(".env.host should be removed during test prep, got err=%v", err)
	}
	values, err := ParseEnvFiles(filepath.Join(projectDir, ".env"))
	if err != nil {
		t.Fatalf("ParseEnvFiles(.env): %v", err)
	}
	originalPorts := map[string]string{"DB_PORT": "3306", "REDIS_PORT": "6379", "RUSTFS_API_PORT": "9000"}
	for _, key := range []string{"DB_PORT", "REDIS_PORT", "RUSTFS_API_PORT"} {
		raw := values[key]
		if raw == "" || raw == originalPorts[key] {
			t.Fatalf("%s should receive an allocated port, got %q", key, raw)
		}
	}
	if got := values["STORAGE_ENDPOINT"]; got != "http://localhost:"+values["RUSTFS_API_PORT"] {
		t.Fatalf("STORAGE_ENDPOINT = %q, want RustFS test endpoint on allocated port %q", got, values["RUSTFS_API_PORT"])
	}
}

// TestRenderedComposeDefaultPortRangeAvoidsEphemeralPorts protects explicit bindings from Docker's automatic host-port allocator.
func TestRenderedComposeDefaultPortRangeAvoidsEphemeralPorts(t *testing.T) {
	t.Setenv("FORJ_INTEGRATION_PORT_RANGE_START", "")
	t.Setenv("FORJ_INTEGRATION_PORT_RANGE_END", "")

	start, end := renderedComposePortRange()
	if start < 1024 || end >= 32768 {
		t.Fatalf("default rendered Compose port range = %d-%d, want non-privileged ports below the common Linux ephemeral range", start, end)
	}
	if width := end - start + 1; width < 1000 {
		t.Fatalf("default rendered Compose port range width = %d, want at least 1000 ports", width)
	}
}

// TestPrepareRenderedComposeTestEnvSkipsInactiveProfiles keeps partial profile names from reserving ports for dormant services.
func TestPrepareRenderedComposeTestEnvSkipsInactiveProfiles(t *testing.T) {
	projectDir := t.TempDir()
	t.Setenv("FORJ_INTEGRATION_PORT_RANGE_START", "47100")
	t.Setenv("FORJ_INTEGRATION_PORT_RANGE_END", "47110")
	if err := os.WriteFile(filepath.Join(projectDir, ".env"), []byte(""+
		"DB_PORT=3306\n"+
		"REDIS_PORT=6379\n"+
		"COMPOSE_PROFILES=redis-debug\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(`
services:
  mysql:
    ports:
      - ${IP_ADDRESS:-0.0.0.0}:${DB_PORT}:3306
  redis:
    profiles: [redis]
    ports:
      - ${IP_ADDRESS:-0.0.0.0}:${REDIS_PORT}:6379
`), 0o644); err != nil {
		t.Fatalf("write docker-compose.yml: %v", err)
	}

	if err := prepareRenderedComposeTestEnv(projectDir); err != nil {
		t.Fatalf("prepareRenderedComposeTestEnv: %v", err)
	}
	values, err := ParseEnvFiles(filepath.Join(projectDir, ".env"))
	if err != nil {
		t.Fatalf("ParseEnvFiles(.env): %v", err)
	}
	if got := values["DB_PORT"]; got == "" || got == "3306" {
		t.Fatalf("DB_PORT should receive an allocated port, got %q", got)
	}
	if got := values["REDIS_PORT"]; got != "6379" {
		t.Fatalf("REDIS_PORT = %q, want dormant profile value %q", got, "6379")
	}
}
