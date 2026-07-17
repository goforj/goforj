package backup

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/goforj/str/v2"
)

// SQLDialect contains only the SQL syntax needed by portable row transfer.
type SQLDialect interface {
	QuoteIdentifier(string) string
	Placeholder(int) string
	ListTables(context.Context, *sql.DB) ([]string, error)
	IdentityColumns(context.Context, SQLQueryer, string, []string) (map[string]bool, error)
	RestoreIdentity(context.Context, *sql.Tx, string, string, int64) error
}

// SQLQueryer is the shared query surface implemented by sql.DB and sql.Tx.
type SQLQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// NewSQLDialect returns the syntax adapter for a supported SQL driver.
func NewSQLDialect(driver string) (SQLDialect, error) {
	switch normalizeDriver(driver) {
	case "sqlite", "postgres", "mysql":
		return sqlDialect{name: normalizeDriver(driver)}, nil
	default:
		return nil, fmt.Errorf("unsupported SQL dialect %q", driver)
	}
}

// ExportPortable reads database tables into the driver-neutral archive model.
func ExportPortable(ctx context.Context, db *sql.DB, dialect SQLDialect, tables []string) (PortableArchive, error) {
	if db == nil {
		return PortableArchive{}, fmt.Errorf("database connection is required")
	}
	if dialect == nil {
		return PortableArchive{}, fmt.Errorf("SQL dialect is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	sort.Strings(tables)
	archive := PortableArchive{Version: 1}
	for _, tableName := range tables {
		if strings.TrimSpace(tableName) == "" || tableName == "migrations" {
			continue
		}
		table, err := exportTable(ctx, db, dialect, tableName)
		if err != nil {
			return PortableArchive{}, err
		}
		archive.Tables = append(archive.Tables, table)
	}
	return archive, nil
}

// ImportPortable inserts a portable archive into a target database transaction.
func ImportPortable(ctx context.Context, tx *sql.Tx, dialect SQLDialect, archive PortableArchive) error {
	if tx == nil {
		return fmt.Errorf("database transaction is required")
	}
	if dialect == nil {
		return fmt.Errorf("SQL dialect is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if archive.Version != 1 {
		return fmt.Errorf("unsupported portable archive version %d", archive.Version)
	}
	for _, table := range archive.Tables {
		columns, err := inspectTable(ctx, tx, dialect, table.Name)
		if err != nil {
			return err
		}
		if err := validatePortableColumns(table, columns); err != nil {
			return err
		}
		for _, row := range table.Rows {
			if err := validatePortableRow(table, columns, row); err != nil {
				return err
			}
			if err := insertPortableRow(ctx, tx, dialect, table, row); err != nil {
				return err
			}
		}
		for _, column := range table.Columns {
			if !column.AutoIncrement {
				continue
			}
			next := column.NextValue
			if next == 0 {
				next = 1
			}
			if err := dialect.RestoreIdentity(ctx, tx, table.Name, column.Name, next); err != nil {
				return err
			}
		}
	}
	return nil
}

// ValidateSchemaCompatibility checks a portable archive against target table metadata.
func ValidateSchemaCompatibility(archive PortableArchive, target []PortableTable) error {
	targetByName := map[string]PortableTable{}
	for _, table := range target {
		targetByName[table.Name] = table
	}
	for _, source := range archive.Tables {
		targetTable, ok := targetByName[source.Name]
		if !ok {
			return fmt.Errorf("portable archive is missing target table %s", source.Name)
		}
		if err := validatePortableColumns(source, targetTable.Columns); err != nil {
			return err
		}
	}
	return nil
}

// validatePortableRow rejects NULL values that the target schema cannot accept.
func validatePortableRow(table PortableTable, target []ColumnSpec, row PortableRow) error {
	columns := map[string]ColumnSpec{}
	for _, column := range target {
		columns[column.Name] = column
	}
	for name, value := range row {
		column, ok := columns[name]
		if ok && value.Type == "null" && !column.Nullable {
			return fmt.Errorf("portable table %s column %s contains NULL but target is required", table.Name, name)
		}
	}
	return nil
}

// exportTable reads metadata and rows for one table.
func exportTable(ctx context.Context, db *sql.DB, dialect SQLDialect, name string) (PortableTable, error) {
	rows, err := db.QueryContext(ctx, "SELECT * FROM "+dialect.QuoteIdentifier(name))
	if err != nil {
		return PortableTable{}, fmt.Errorf("read source table %s: %w", name, err)
	}
	defer rows.Close()
	columns, err := rows.ColumnTypes()
	if err != nil {
		return PortableTable{}, fmt.Errorf("inspect source table %s: %w", name, err)
	}
	result := PortableTable{Name: name, Columns: columnSpecs(columns)}
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(values))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return PortableTable{}, fmt.Errorf("scan source table %s: %w", name, err)
		}
		row := PortableRow{}
		for i, column := range columns {
			canonical, err := EncodeCanonical(values[i], result.Columns[i].Type)
			if err != nil {
				return PortableTable{}, fmt.Errorf("encode %s.%s: %w", name, column.Name(), err)
			}
			row[column.Name()] = canonical
		}
		result.Rows = append(result.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return PortableTable{}, fmt.Errorf("read source table %s: %w", name, err)
	}
	if err := rows.Close(); err != nil {
		return PortableTable{}, fmt.Errorf("close source table %s: %w", name, err)
	}
	identityColumns, err := dialect.IdentityColumns(ctx, db, name, columnNames(columns))
	if err != nil {
		return PortableTable{}, fmt.Errorf("inspect source identities %s: %w", name, err)
	}
	for i := range result.Columns {
		result.Columns[i].AutoIncrement = identityColumns[result.Columns[i].Name]
	}
	for i := range result.Columns {
		if !result.Columns[i].AutoIncrement {
			continue
		}
		max := int64(0)
		for _, row := range result.Rows {
			value, ok := row[result.Columns[i].Name]
			if !ok || value.Type != "integer" {
				continue
			}
			var number int64
			if _, err := fmt.Sscan(value.Value, &number); err == nil && number > max {
				max = number
			}
		}
		result.Columns[i].NextValue = max + 1
	}
	return result, nil
}

// inspectTable obtains target column metadata without requiring an ORM schema API.
func inspectTable(ctx context.Context, tx *sql.Tx, dialect SQLDialect, name string) ([]ColumnSpec, error) {
	rows, err := tx.QueryContext(ctx, "SELECT * FROM "+dialect.QuoteIdentifier(name)+" LIMIT 0")
	if err != nil {
		return nil, fmt.Errorf("inspect target table %s: %w", name, err)
	}
	columns, err := rows.ColumnTypes()
	if err != nil {
		return nil, fmt.Errorf("inspect target table %s: %w", name, err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close target table %s: %w", name, err)
	}
	result := columnSpecs(columns)
	identityColumns, err := dialect.IdentityColumns(ctx, tx, name, columnNames(columns))
	if err != nil {
		return nil, fmt.Errorf("inspect target identities %s: %w", name, err)
	}
	for i := range result {
		result[i].AutoIncrement = identityColumns[result[i].Name]
	}
	return result, nil
}

// insertPortableRow inserts one row using dialect-specific placeholders.
func insertPortableRow(ctx context.Context, tx *sql.Tx, dialect SQLDialect, table PortableTable, row PortableRow) error {
	columns := make([]string, 0, len(row))
	for column := range row {
		columns = append(columns, column)
	}
	sort.Strings(columns)
	quoted := make([]string, len(columns))
	placeholders := make([]string, len(columns))
	values := make([]any, len(columns))
	for i, column := range columns {
		quoted[i] = dialect.QuoteIdentifier(column)
		placeholders[i] = dialect.Placeholder(i + 1)
		value, err := DecodeCanonical(row[column])
		if err != nil {
			return fmt.Errorf("decode %s.%s: %w", table.Name, column, err)
		}
		values[i] = value
	}
	query := "INSERT INTO " + dialect.QuoteIdentifier(table.Name) + " (" + strings.Join(quoted, ", ") + ") VALUES (" + strings.Join(placeholders, ", ") + ")"
	if _, err := tx.ExecContext(ctx, query, values...); err != nil {
		return fmt.Errorf("insert portable row into %s: %w", table.Name, err)
	}
	return nil
}

// columnSpecs converts standard-library column metadata into portable metadata.
func columnSpecs(columns []*sql.ColumnType) []ColumnSpec {
	specs := make([]ColumnSpec, 0, len(columns))
	for _, column := range columns {
		nullable, _ := column.Nullable()
		specs = append(specs, ColumnSpec{Name: column.Name(), Type: strings.ToLower(column.DatabaseTypeName()), Nullable: nullable})
	}
	return specs
}

// columnNames extracts column names for dialect-specific identity inspection.
func columnNames(columns []*sql.ColumnType) []string {
	names := make([]string, 0, len(columns))
	for _, column := range columns {
		names = append(names, column.Name())
	}
	return names
}

// validatePortableColumns rejects missing or incompatible target columns before inserts.
func validatePortableColumns(table PortableTable, target []ColumnSpec) error {
	available := map[string]ColumnSpec{}
	for _, column := range target {
		available[column.Name] = column
	}
	for _, column := range table.Columns {
		targetColumn, ok := available[column.Name]
		if !ok {
			return fmt.Errorf("portable table %s is missing target column %s", table.Name, column.Name)
		}
		if !compatibleSQLTypes(column.Type, targetColumn.Type) {
			return fmt.Errorf("portable table %s column %s type %s is incompatible with target %s", table.Name, column.Name, column.Type, targetColumn.Type)
		}
	}
	return nil
}

// compatibleSQLTypes accepts equivalent vendor type families while rejecting unrelated types.
func compatibleSQLTypes(source string, target string) bool {
	return canonicalSQLType(source) == canonicalSQLType(target)
}

// canonicalSQLType maps database type names into the portable type families.
func canonicalSQLType(value string) string {
	value = str.Of(value).Trim().ToLower().String()
	switch {
	case strings.Contains(value, "bool"):
		return "boolean"
	case value == "tinyint" || strings.HasPrefix(value, "tinyint("):
		return "boolean"
	case strings.Contains(value, "int"):
		return "integer"
	case strings.Contains(value, "decimal"), strings.Contains(value, "numeric"), strings.Contains(value, "real"), strings.Contains(value, "double"), strings.Contains(value, "float"):
		return "decimal"
	case strings.Contains(value, "timestamp"), strings.Contains(value, "datetime"):
		return "timestamp"
	case value == "date":
		return "date"
	case strings.Contains(value, "json"):
		return "json"
	case strings.Contains(value, "blob"), strings.Contains(value, "binary"), strings.Contains(value, "bytea"):
		return "bytes"
	default:
		return "string"
	}
}

type sqlDialect struct{ name string }

// QuoteIdentifier quotes one identifier for the selected SQL dialect.
func (d sqlDialect) QuoteIdentifier(identifier string) string {
	if d.name == "mysql" {
		return "`" + strings.ReplaceAll(identifier, "`", "``") + "`"
	}
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

// Placeholder returns the parameter marker for one argument position.
func (d sqlDialect) Placeholder(position int) string {
	if d.name == "postgres" {
		return fmt.Sprintf("$%d", position)
	}
	return "?"
}

// ListTables discovers user tables for the selected SQL dialect.
func (d sqlDialect) ListTables(ctx context.Context, db *sql.DB) ([]string, error) {
	var query string
	switch d.name {
	case "sqlite":
		query = "SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'"
	case "mysql":
		query = "SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE()"
	case "postgres":
		query = "SELECT table_name FROM information_schema.tables WHERE table_schema = current_schema()"
	default:
		return nil, fmt.Errorf("unsupported SQL dialect %q", d.name)
	}
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tables := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if name != "migrations" {
			tables = append(tables, name)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tables, nil
}

// IdentityColumns discovers generated identity columns for one table.
func (d sqlDialect) IdentityColumns(ctx context.Context, queryer SQLQueryer, table string, columns []string) (map[string]bool, error) {
	identities := map[string]bool{}
	var query string
	var args []any
	switch d.name {
	case "sqlite":
		query = "PRAGMA table_info(" + d.QuoteIdentifier(table) + ")"
	case "mysql":
		query = "SELECT COLUMN_NAME FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND EXTRA LIKE '%auto_increment%'"
		args = []any{table}
	case "postgres":
		query = "SELECT column_name FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = $1 AND column_default LIKE 'nextval(%'"
		args = []any{table}
	default:
		return identities, nil
	}
	rows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if d.name == "sqlite" {
		for rows.Next() {
			var cid int
			var name, typeName string
			var notNull, primaryKey int
			var defaultValue any
			if err := rows.Scan(&cid, &name, &typeName, &notNull, &defaultValue, &primaryKey); err != nil {
				return nil, err
			}
			if primaryKey == 1 && strings.Contains(strings.ToLower(typeName), "int") {
				identities[name] = true
			}
		}
	} else {
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return nil, err
			}
			identities[name] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return identities, nil
}

// RestoreIdentity restores the next generated identity value for one table.
func (d sqlDialect) RestoreIdentity(ctx context.Context, tx *sql.Tx, table string, column string, next int64) error {
	if next < 1 {
		next = 1
	}
	switch d.name {
	case "sqlite":
		query := "INSERT OR REPLACE INTO sqlite_sequence (name, seq) VALUES (" + d.Placeholder(1) + ", " + d.Placeholder(2) + ")"
		if _, err := tx.ExecContext(ctx, query, table, next-1); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "no such table: sqlite_sequence") {
				return nil
			}
			return fmt.Errorf("restore SQLite identity %s.%s: %w", table, column, err)
		}
	case "mysql":
		query := "ALTER TABLE " + d.QuoteIdentifier(table) + " AUTO_INCREMENT = " + fmt.Sprint(next)
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("restore MySQL identity %s.%s: %w", table, column, err)
		}
	case "postgres":
		sequenceValue := next - 1
		called := "true"
		if next <= 1 {
			sequenceValue, called = 1, "false"
		}
		query := "SELECT setval(pg_get_serial_sequence(" + d.Placeholder(1) + ", " + d.Placeholder(2) + "), " + d.Placeholder(3) + ", " + called + ")"
		if _, err := tx.ExecContext(ctx, query, table, column, sequenceValue); err != nil {
			return fmt.Errorf("restore Postgres identity %s.%s: %w", table, column, err)
		}
	}
	return nil
}
