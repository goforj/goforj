package backup

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"

	_ "github.com/glebarez/go-sqlite"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// OpenedSQLConnection couples an open database handle with the dialect required to inspect it.
type OpenedSQLConnection struct {
	DB      *sql.DB
	Dialect SQLDialect
}

// OpenSQLConnection opens a standard-library SQL connection from framework metadata.
func OpenSQLConnection(ctx context.Context, connection Connection) (OpenedSQLConnection, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	driver := normalizeDriver(connection.Driver)
	dialect, err := NewSQLDialect(driver)
	if err != nil {
		return OpenedSQLConnection{}, err
	}
	sqlDriver, dsn, err := sqlConnectionDetails(connection)
	if err != nil {
		return OpenedSQLConnection{}, err
	}
	db, err := sql.Open(sqlDriver, dsn)
	if err != nil {
		return OpenedSQLConnection{}, fmt.Errorf("open %s connection: %w", driver, err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return OpenedSQLConnection{}, fmt.Errorf("ping %s connection: %w", driver, err)
	}
	return OpenedSQLConnection{DB: db, Dialect: dialect}, nil
}

// ListTables discovers application tables while excluding framework metadata tables.
func ListTables(ctx context.Context, db *sql.DB, dialect SQLDialect) ([]string, error) {
	if db == nil || dialect == nil {
		return nil, fmt.Errorf("database and SQL dialect are required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tables, err := dialect.ListTables(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("list database tables: %w", err)
	}
	return tables, nil
}

// sqlConnectionDetails builds driver-native DSNs without exposing them in output.
func sqlConnectionDetails(connection Connection) (string, string, error) {
	driver := normalizeDriver(connection.Driver)
	if connection.DSN != "" {
		if driver == "postgres" {
			return "pgx", connection.DSN, nil
		}
		return driver, connection.DSN, nil
	}
	switch driver {
	case "sqlite":
		path := connection.Database
		if path == "" {
			return "", "", fmt.Errorf("sqlite database path is required")
		}
		return "sqlite", path, nil
	case "mysql":
		if connection.Database == "" {
			return "", "", fmt.Errorf("mysql database is required")
		}
		host := connection.Host
		if host == "" {
			host = "127.0.0.1"
		}
		port := connection.Port
		if port == "" {
			port = "3306"
		}
		return "mysql", fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true", connection.Username, connection.Password, host, port, connection.Database), nil
	case "postgres":
		if connection.Database == "" {
			return "", "", fmt.Errorf("postgres database is required")
		}
		host := connection.Host
		if host == "" {
			host = "127.0.0.1"
		}
		port := connection.Port
		if port == "" {
			port = "5432"
		}
		values := url.Values{"sslmode": {"disable"}}
		postgresURL := url.URL{Scheme: "postgres", User: url.UserPassword(connection.Username, connection.Password), Host: net.JoinHostPort(host, port), Path: "/" + connection.Database, RawQuery: values.Encode()}
		return "pgx", postgresURL.String(), nil
	default:
		return "", "", fmt.Errorf("unsupported SQL driver %q", driver)
	}
}
