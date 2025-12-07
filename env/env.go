package env

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Get returns the environment variable for the given key,
// or the provided fallback if empty.
//
// Example:
//
//    dbHost := env.Get("DB_HOST", "localhost")
//
func Get(key, fallback string) string {
	val := os.Getenv(key)
	if len(val) == 0 {
		return fallback
	}
	return val
}

// GetInt parses an int from an environment variable with a fallback.
//
// Example:
//
//    port := env.GetInt("PORT", "8080")
//
func GetInt(key, fallback string) int {
	val := Get(key, fallback)
	ret, err := strconv.Atoi(val)
	if err != nil {
		panic(err)
	}
	return ret
}

// GetInt64 parses an int64 from an environment variable.
//
// Example:
//
//    size := env.GetInt64("MAX_SIZE", "1024")
//
func GetInt64(key, fallback string) int64 {
	val := Get(key, fallback)
	ret, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		panic(err)
	}
	return ret
}

// GetUint parses an unsigned int from environment variables.
//
// Example:
//
//    workers := env.GetUint("WORKERS", "4")
//
func GetUint(key, fallback string) uint {
	val := Get(key, fallback)
	i, err := strconv.ParseUint(val, 10, 32)
	if err != nil {
		panic(err)
	}
	return uint(i)
}

// GetUint64 parses an unsigned 64-bit int from environment variables.
//
// Example:
//
//    maxItems := env.GetUint64("MAX_ITEMS", "5000")
//
func GetUint64(key, fallback string) uint64 {
	val := Get(key, fallback)
	i, err := strconv.ParseUint(val, 10, 64)
	if err != nil {
		panic(err)
	}
	return i
}

// GetFloat parses a float64 from an environment variable.
//
// Example:
//
//    threshold := env.GetFloat("THRESHOLD", "0.75")
//
func GetFloat(key, fallback string) float64 {
	val := Get(key, fallback)
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		panic(err)
	}
	return f
}

// GetBool parses a boolean from an environment variable.
//
// Accepted values: true/false, 1/0, t/f, TRUE/FALSE.
//
// Example:
//
//    debug := env.GetBool("DEBUG", "false")
//
func GetBool(key, fallback string) bool {
	val := Get(key, fallback)
	ret, err := strconv.ParseBool(val)
	if err != nil {
		panic(err)
	}
	return ret
}

// GetDuration parses a Go duration string (e.g. "5s", "10m", "1h").
//
// Example:
//
//    timeout := env.GetDuration("HTTP_TIMEOUT", "5s")
//
func GetDuration(key, fallback string) time.Duration {
	val := Get(key, fallback)
	d, err := time.ParseDuration(val)
	if err != nil {
		panic(err)
	}
	return d
}

// GetSlice splits a comma-separated string into a []string.
//
// Example:
//
//    peers := env.GetSlice("PEERS", "10.0.0.1,10.0.0.2")
//    // → []string{"10.0.0.1", "10.0.0.2"}
//
func GetSlice(key, fallback string) []string {
	val := Get(key, fallback)
	if val == "" {
		return []string{}
	}

	parts := strings.Split(val, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// GetMap parses key=value pairs separated by commas.
//
// Example:
//
//    limits := env.GetMap("LIMITS", "read=10,write=5")
//    // → map[string]string{"read":"10", "write":"5"}
//
func GetMap(key, fallback string) map[string]string {
	val := Get(key, fallback)
	m := map[string]string{}

	if strings.TrimSpace(val) == "" {
		return m
	}

	pairs := strings.Split(val, ",")
	for _, p := range pairs {
		kv := strings.SplitN(strings.TrimSpace(p), "=", 2)
		if len(kv) == 2 {
			m[kv[0]] = kv[1]
		}
	}

	return m
}

// GetEnum ensures the environment variable's value is in the allowed list.
//
// Example:
//
//    env := env.GetEnum("APP_ENV", "dev", []string{"dev","staging","prod"})
//
func GetEnum(key, fallback string, allowed []string) string {
	val := Get(key, fallback)
	for _, a := range allowed {
		if val == a {
			return val
		}
	}
	panic("env: invalid enum value for " + key + ": " + val)
}

// MustGet returns the value of key or panics if missing.
//
// Example:
//
//    secret := env.MustGet("API_SECRET")
//
func MustGet(key string) string {
	val := os.Getenv(key)
	if val == "" {
		panic("env variable missing: " + key)
	}
	return val
}

// MustGetInt panics if the value is missing or not an int.
//
// Example:
//
//    port := env.MustGetInt("PORT")
//
func MustGetInt(key string) int {
	return GetInt(key, "")
}

// MustGetBool panics if missing or invalid.
//
// Example:
//
//    enabled := env.MustGetBool("FEATURE_ENABLED")
//
func MustGetBool(key string) bool {
	return GetBool(key, "")
}
