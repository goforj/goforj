package backup

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/goforj/str/v2"

	_ "github.com/glebarez/go-sqlite"
)

// Connection describes the safe connection metadata required by a backup strategy.
type Connection struct {
	Name     string
	Driver   string
	DSN      string
	Database string
	Host     string
	Port     string
	Username string
	Password string
}

// Strategy creates and restores one native database artifact.
type Strategy interface {
	// Name defines the name behavior required from implementations.
	Name() string
	// Backup defines the backup behavior required from implementations.
	Backup(context.Context, Connection, string) error
	// Restore defines the restore behavior required from implementations.
	Restore(context.Context, Connection, string) error
}

// NativeStrategy returns the native strategy for a supported database driver.
func NativeStrategy(driver string) (Strategy, error) {
	switch str.Of(driver).Trim().ToLower().String() {
	case "sqlite", "sqlite3":
		return sqliteStrategy{}, nil
	case "mysql", "mariadb":
		return mysqlStrategy{}, nil
	case "postgres", "postgresql", "pgx":
		return postgresStrategy{}, nil
	default:
		return nil, fmt.Errorf("unsupported backup database driver %q", driver)
	}
}

// runTool runs an external database tool without exposing connection secrets.
func runTool(ctx context.Context, name string, args []string, env []string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run %s: %w", name, err)
	}
	return nil
}

// artifactPath creates the parent directory and returns a safe artifact path.
func artifactPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("backup artifact path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("create backup artifact directory: %w", err)
	}
	return path, nil
}

type sqliteStrategy struct{}

// Name identifies the SQLite native backup strategy.
func (sqliteStrategy) Name() string { return "sqlite-vacuum-into" }

// Backup creates a consistent SQLite backup through SQLite's native VACUUM INTO operation.
func (s sqliteStrategy) Backup(ctx context.Context, conn Connection, artifact string) error {
	path, err := artifactPath(artifact)
	if err != nil {
		return err
	}
	database := conn.DSN
	if database == "" {
		database = conn.Database
	}
	if database == "" {
		return fmt.Errorf("sqlite backup requires a database path")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	db, err := sql.Open("sqlite", database)
	if err != nil {
		return fmt.Errorf("open SQLite database: %w", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping SQLite database: %w", err)
	}
	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		return fmt.Errorf("backup SQLite database: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("protect SQLite backup: %w", err)
	}
	return nil
}

// Restore restores a SQLite artifact through a native temporary copy and replacement.
func (s sqliteStrategy) Restore(ctx context.Context, conn Connection, artifact string) error {
	database := conn.DSN
	if database == "" {
		database = conn.Database
	}
	if database == "" {
		return fmt.Errorf("sqlite restore requires a database path")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if filepath.Clean(database) == filepath.Clean(artifact) {
		return fmt.Errorf("sqlite restore source and target must differ")
	}
	targetDir := filepath.Dir(database)
	temp, err := os.CreateTemp(targetDir, ".goforj-sqlite-restore-*")
	if err != nil {
		return fmt.Errorf("create SQLite restore temporary file: %w", err)
	}
	tempPath := temp.Name()
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close SQLite restore temporary file: %w", err)
	}
	defer os.Remove(tempPath)
	_ = os.Remove(tempPath)
	source, err := sql.Open("sqlite", artifact)
	if err != nil {
		return fmt.Errorf("open SQLite backup: %w", err)
	}
	if err := source.PingContext(ctx); err != nil {
		_ = source.Close()
		return fmt.Errorf("ping SQLite backup: %w", err)
	}
	if _, err := source.ExecContext(ctx, "VACUUM INTO ?", tempPath); err != nil {
		_ = source.Close()
		return fmt.Errorf("prepare SQLite restore: %w", err)
	}
	if err := source.Close(); err != nil {
		return fmt.Errorf("close SQLite backup: %w", err)
	}
	if err := replaceSQLiteDatabase(tempPath, database); err != nil {
		return err
	}
	return nil
}

// replaceSQLiteDatabase replaces a target file only after the native restore is complete.
func replaceSQLiteDatabase(source string, target string) error {
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove SQLite target: %w", err)
	}
	if err := os.Rename(source, target); err != nil {
		return fmt.Errorf("replace SQLite target: %w", err)
	}
	return nil
}

type mysqlStrategy struct{}

// Name identifies the MySQL native backup strategy.
func (mysqlStrategy) Name() string { return "mysqldump" }

// Backup creates a logical MySQL or MariaDB dump.
func (s mysqlStrategy) Backup(ctx context.Context, conn Connection, artifact string) error {
	path, err := artifactPath(artifact)
	if err != nil {
		return err
	}
	args := mysqlArgs(conn)
	args = append(args, "--single-transaction", "--quick", "--routines", "--triggers", "--events", "--hex-blob", conn.Database)
	tool, err := firstAvailableTool("mysqldump", "mariadb-dump")
	if err != nil {
		return err
	}
	return runRedirectedTool(ctx, tool, args, mysqlEnv(conn), path)
}

// Restore restores a MySQL or MariaDB logical dump.
func (s mysqlStrategy) Restore(ctx context.Context, conn Connection, artifact string) error {
	args := mysqlArgs(conn)
	args = append(args, conn.Database)
	tool, err := firstAvailableTool("mysql", "mariadb")
	if err != nil {
		return err
	}
	return runInputTool(ctx, tool, args, mysqlEnv(conn), artifact)
}

// mysqlArgs builds non-secret arguments shared by MySQL dump and restore.
func mysqlArgs(conn Connection) []string {
	args := []string{}
	if conn.Host != "" {
		// Mapped container ports must use TCP; MySQL clients otherwise treat localhost as a Unix socket.
		args = append(args, "--protocol", "TCP")
		args = append(args, "--host", conn.Host)
	}
	if conn.Port != "" {
		args = append(args, "--port", conn.Port)
	}
	if conn.Username != "" {
		args = append(args, "--user", conn.Username)
	}
	return args
}

// mysqlEnv supplies the password through the client environment instead of arguments.
func mysqlEnv(conn Connection) []string {
	if conn.Password == "" {
		return nil
	}
	return []string{"MYSQL_PWD=" + conn.Password}
}

type postgresStrategy struct{}

// Name identifies the Postgres native backup strategy.
func (postgresStrategy) Name() string { return "pg_dump" }

// Backup creates a custom-format Postgres dump.
func (s postgresStrategy) Backup(ctx context.Context, conn Connection, artifact string) error {
	path, err := artifactPath(artifact)
	if err != nil {
		return err
	}
	args := []string{"--format=custom", "--file", path}
	args = append(args, postgresArgs(conn)...)
	args = append(args, conn.Database)
	return runTool(ctx, "pg_dump", args, postgresEnv(conn))
}

// Restore restores a custom-format Postgres dump.
func (s postgresStrategy) Restore(ctx context.Context, conn Connection, artifact string) error {
	args := []string{"--clean", "--if-exists"}
	args = append(args, postgresArgs(conn)...)
	args = append(args, "--dbname", conn.Database, artifact)
	return runTool(ctx, "pg_restore", args, postgresEnv(conn))
}

// postgresArgs builds non-secret arguments shared by Postgres dump and restore.
func postgresArgs(conn Connection) []string {
	args := []string{}
	if conn.Host != "" {
		args = append(args, "--host", conn.Host)
	}
	if conn.Port != "" {
		args = append(args, "--port", conn.Port)
	}
	if conn.Username != "" {
		args = append(args, "--username", conn.Username)
	}
	return args
}

// postgresEnv supplies the password through the client environment instead of arguments.
func postgresEnv(conn Connection) []string {
	if conn.Password == "" {
		return nil
	}
	return []string{"PGPASSWORD=" + conn.Password}
}

// runRedirectedTool runs a native tool with stdout redirected to an artifact.
func runRedirectedTool(ctx context.Context, name string, args []string, env []string, output string) error {
	file, err := openPrivateOutput(output)
	if err != nil {
		return fmt.Errorf("create %s output: %w", name, err)
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = file
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		runErr := fmt.Errorf("run %s: %w: %s", name, err, strings.TrimSpace(stderr.String()))
		if closeErr := file.Close(); closeErr != nil {
			return errors.Join(runErr, fmt.Errorf("close %s output: %w", name, closeErr))
		}
		return runErr
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s output: %w", name, err)
	}
	return nil
}

// runInputTool runs a native tool with stdin read from an artifact.
func runInputTool(ctx context.Context, name string, args []string, env []string, input string) error {
	file, err := os.Open(input)
	if err != nil {
		return fmt.Errorf("open %s input: %w", name, err)
	}
	defer file.Close()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = file
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run %s: %w: %s", name, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// firstAvailableTool selects the first installed native database client.
func firstAvailableTool(names ...string) (string, error) {
	for _, name := range names {
		if _, err := exec.LookPath(name); err == nil {
			return name, nil
		}
	}
	return "", fmt.Errorf("none of the required database tools are installed: %s", strings.Join(names, ", "))
}
