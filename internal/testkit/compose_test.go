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
		"DB_USERNAME=user\n"+
		"DB_PASSWORD=password\n"+
		"DB_ROOT_PASSWORD=root\n"+
		"IP_ADDRESS=0.0.0.0\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".env.host"), []byte(""+
		"DB_HOST=localhost\n"+
		"DB_PORT=3306\n"), 0o644); err != nil {
		t.Fatalf("write .env.host: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(`
services:
  mysql:
    build:
      context: ./containers/mariadb
      args:
        - INNODB_BUFFER_POOL_SIZE=${INNODB_BUFFER_POOL_SIZE:-512MB}
    ports:
      - ${IP_ADDRESS:-0.0.0.0}:3306:3306
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
	if got := mysql.Ports[0]; got != "0.0.0.0:3306:3306" {
		t.Fatalf("port mapping = %q, want %q", got, "0.0.0.0:3306:3306")
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
	if port != "5432/tcp" {
		t.Fatalf("container port = %q, want %q", port, "5432/tcp")
	}
}
