package stacks

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/goforj/goforj/internal/envfile"
)

// SQLiteDSNsKey carries private retained targets through existing Stack snapshots without colliding with named database settings.
const SQLiteDSNsKey = "FORJ_STACK_SQLITE_DSNS"

// sqliteDSNs decodes private metadata without including connection values in diagnostics.
func sqliteDSNs(values map[string]string) (map[string]string, error) {
	retained := map[string]string{}
	if strings.TrimSpace(values[SQLiteDSNsKey]) == "" {
		return retained, nil
	}
	if err := json.Unmarshal([]byte(values[SQLiteDSNsKey]), &retained); err != nil || retained == nil {
		return nil, fmt.Errorf("invalid private SQLite target metadata")
	}
	for key, value := range retained {
		if !envfile.IsValidKey(key) || !strings.HasSuffix(key, "_DRIVER") || strings.ContainsRune(value, '\x00') {
			return nil, fmt.Errorf("invalid private SQLite target metadata")
		}
	}
	return retained, nil
}

// storeSQLiteDSNs keeps retained connection strings in the same private value map as the profile that owns them.
func storeSQLiteDSNs(values, retained map[string]string) {
	if len(retained) == 0 {
		delete(values, SQLiteDSNsKey)
		return
	}
	data, _ := json.Marshal(retained)
	values[SQLiteDSNsKey] = string(data)
}
