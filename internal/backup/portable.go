package backup

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"
)

// CanonicalValue is a database-neutral representation of one SQL value.
type CanonicalValue struct {
	Type  string `json:"type"`
	Value string `json:"value,omitempty"`
}

// PortableRow stores one table row by column name.
type PortableRow map[string]CanonicalValue

// PortableTable stores the portable rows for one table.
type PortableTable struct {
	Name    string        `json:"name"`
	Columns []ColumnSpec  `json:"columns"`
	Rows    []PortableRow `json:"rows"`
}

// ColumnSpec describes a portable column contract.
type ColumnSpec struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	Nullable      bool   `json:"nullable"`
	AutoIncrement bool   `json:"auto_increment,omitempty"`
	NextValue     int64  `json:"next_value,omitempty"`
}

// PortableArchive is the in-memory representation of a portable data archive.
type PortableArchive struct {
	Version              int             `json:"version"`
	SchemaFingerprint    string          `json:"schema_fingerprint,omitempty"`
	MigrationFingerprint string          `json:"migration_fingerprint,omitempty"`
	Tables               []PortableTable `json:"tables"`
}

// MigrationFingerprint returns a stable hash for an ordered migration identity set.
func MigrationFingerprint(names []string) string {
	ordered := append([]string(nil), names...)
	sort.Strings(ordered)
	return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(strings.Join(ordered, "\x00"))))
}

// ValidateMigrationFingerprint verifies that an archive matches the target migration contract.
func ValidateMigrationFingerprint(archive PortableArchive, expected string) error {
	if expected == "" || archive.MigrationFingerprint == "" {
		return nil
	}
	if archive.MigrationFingerprint != expected {
		return fmt.Errorf("portable archive migration fingerprint mismatch: expected %s, got %s", expected, archive.MigrationFingerprint)
	}
	return nil
}

// EncodeCanonical converts a database driver value into a canonical value.
func EncodeCanonical(value any, databaseType string) (CanonicalValue, error) {
	if value == nil {
		return CanonicalValue{Type: "null"}, nil
	}
	typeName := strings.ToLower(strings.TrimSpace(databaseType))
	canonicalType := canonicalSQLType(typeName)
	switch typed := value.(type) {
	case []byte:
		if canonicalType == "json" {
			return canonicalJSON(typed)
		}
		if canonicalType == "decimal" {
			text := string(typed)
			if _, ok := new(big.Rat).SetString(text); !ok {
				return CanonicalValue{}, fmt.Errorf("invalid decimal value %q", text)
			}
			return CanonicalValue{Type: "decimal", Value: text}, nil
		}
		if canonicalType == "boolean" {
			switch string(typed) {
			case "0", "false":
				return CanonicalValue{Type: "boolean", Value: "false"}, nil
			case "1", "true":
				return CanonicalValue{Type: "boolean", Value: "true"}, nil
			}
		}
		return CanonicalValue{Type: "bytes", Value: base64.StdEncoding.EncodeToString(typed)}, nil
	case bool:
		return CanonicalValue{Type: "boolean", Value: fmt.Sprintf("%t", typed)}, nil
	case time.Time:
		return CanonicalValue{Type: "timestamp", Value: typed.UTC().Format(time.RFC3339Nano)}, nil
	case int64, int32, int16, int8, int:
		if canonicalType == "boolean" {
			return CanonicalValue{Type: "boolean", Value: fmt.Sprintf("%t", fmt.Sprint(typed) != "0")}, nil
		}
		return CanonicalValue{Type: "integer", Value: fmt.Sprint(typed)}, nil
	case uint64, uint32, uint16, uint8, uint:
		if canonicalType == "boolean" {
			return CanonicalValue{Type: "boolean", Value: fmt.Sprintf("%t", fmt.Sprint(typed) != "0")}, nil
		}
		return CanonicalValue{Type: "integer", Value: fmt.Sprint(typed)}, nil
	case float64, float32:
		return CanonicalValue{Type: "decimal", Value: fmt.Sprint(typed)}, nil
	case string:
		if canonicalType == "json" {
			return canonicalJSON([]byte(typed))
		}
		if canonicalType == "decimal" {
			if _, ok := new(big.Rat).SetString(typed); !ok {
				return CanonicalValue{}, fmt.Errorf("invalid decimal value %q", typed)
			}
			return CanonicalValue{Type: "decimal", Value: typed}, nil
		}
		return CanonicalValue{Type: "string", Value: typed}, nil
	default:
		return CanonicalValue{}, fmt.Errorf("unsupported database value type %T", value)
	}
}

// DecodeCanonical converts a canonical value into a driver-neutral Go value.
func DecodeCanonical(value CanonicalValue) (any, error) {
	switch value.Type {
	case "null":
		return nil, nil
	case "string", "decimal", "timestamp", "date", "json":
		return value.Value, nil
	case "integer":
		var number int64
		if _, err := fmt.Sscan(value.Value, &number); err != nil {
			return nil, fmt.Errorf("decode integer: %w", err)
		}
		return number, nil
	case "boolean":
		return value.Value == "true", nil
	case "bytes":
		decoded, err := base64.StdEncoding.DecodeString(value.Value)
		if err != nil {
			return nil, fmt.Errorf("decode bytes: %w", err)
		}
		return decoded, nil
	default:
		return nil, fmt.Errorf("unsupported canonical value type %q", value.Type)
	}
}

// MarshalArchive encodes a portable archive with a stable version marker.
func MarshalArchive(archive PortableArchive) ([]byte, error) {
	if archive.Version == 0 {
		archive.Version = 1
	}
	if archive.SchemaFingerprint == "" {
		archive.SchemaFingerprint = SchemaFingerprint(archive)
	}
	data, err := json.MarshalIndent(archive, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode portable archive: %w", err)
	}
	return append(data, '\n'), nil
}

// SchemaFingerprint returns a stable hash of portable table and column metadata.
func SchemaFingerprint(archive PortableArchive) string {
	data := bytes.Buffer{}
	for _, table := range archive.Tables {
		data.WriteString(table.Name)
		for _, column := range table.Columns {
			data.WriteString("\x00")
			data.WriteString(column.Name)
			data.WriteString("\x00")
			data.WriteString(column.Type)
			data.WriteString(fmt.Sprintf("\x00%t", column.Nullable))
		}
	}
	return fmt.Sprintf("sha256:%x", sha256.Sum256(data.Bytes()))
}

// UnmarshalArchive decodes and validates a portable archive.
func UnmarshalArchive(data []byte) (PortableArchive, error) {
	var archive PortableArchive
	if err := json.Unmarshal(bytes.TrimSpace(data), &archive); err != nil {
		return PortableArchive{}, fmt.Errorf("decode portable archive: %w", err)
	}
	if archive.Version != 1 {
		return PortableArchive{}, fmt.Errorf("unsupported portable archive version %d", archive.Version)
	}
	if archive.SchemaFingerprint != "" && archive.SchemaFingerprint != SchemaFingerprint(archive) {
		return PortableArchive{}, fmt.Errorf("portable archive schema fingerprint mismatch")
	}
	return archive, nil
}

// canonicalJSON validates and compacts JSON before storing it in the archive.
func canonicalJSON(data []byte) (CanonicalValue, error) {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return CanonicalValue{}, fmt.Errorf("invalid JSON value: %w", err)
	}
	compact, err := json.Marshal(value)
	if err != nil {
		return CanonicalValue{}, fmt.Errorf("normalize JSON value: %w", err)
	}
	return CanonicalValue{Type: "json", Value: string(compact)}, nil
}
