package testkit

import (
	"os"
	"path/filepath"
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

func TestRenderedComposeStackEnvOverrides(t *testing.T) {
	stack := &RenderedComposeStack{
		services: map[string]*StartedContainer{
			"mysql": {Host: "127.0.0.1", Port: "33061"},
			"redis": {Host: "127.0.0.1", Port: "63791"},
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
		"REDIS_PORT=6379\n"), 0o644); err != nil {
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
    ports:
      - ${IP_ADDRESS:-0.0.0.0}:${REDIS_PORT}:6379
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
	for _, key := range []string{"DB_PORT", "REDIS_PORT"} {
		raw := values[key]
		if raw == "" {
			t.Fatalf("%s should be set", key)
		}
	}
}
