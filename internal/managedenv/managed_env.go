// Package managedenv preserves launcher-owned development environment values across dotenv reloads.
package managedenv

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

// MetadataKey names the private launcher-to-forj metadata entry that identifies managed values.
const MetadataKey = "FORJ_INTERNAL_MANAGED_ENV_KEYS"

// reservedKeys keeps lifecycle controls owned by GoForj instead of external launchers.
var reservedKeys = map[string]struct{}{
	"APP_ENV":             {},
	MetadataKey:           {},
	"FORJ_APP":            {},
	"FORJ_BUILD_PROGRESS": {},
	"FORJ_COMMAND_PREFIX": {},
	"FORJ_DEV_PLAIN":      {},
}

// Set is the immutable snapshot of launcher-owned values captured before dotenv loading.
type Set struct {
	keys   []string
	values map[string]string
}

// Capture validates and removes the private metadata entry before snapshotting its listed values.
func Capture() (Set, error) {
	raw, present := os.LookupEnv(MetadataKey)
	if !present {
		return Set{}, nil
	}

	set, captureErr := capturePresent(raw)
	if unsetErr := os.Unsetenv(MetadataKey); unsetErr != nil {
		unsetErr = fmt.Errorf("remove managed environment metadata: %w", unsetErr)
		if captureErr != nil {
			return Set{}, errors.Join(captureErr, unsetErr)
		}
		return Set{}, unsetErr
	}
	return set, captureErr
}

// Len reports how many launcher-owned values were captured.
func (s Set) Len() int {
	return len(s.keys)
}

// Keys returns the captured keys in their validated canonical order.
func (s Set) Keys() []string {
	return append([]string(nil), s.keys...)
}

// Lookup returns one captured launcher-owned value, including present-empty values.
func (s Set) Lookup(key string) (string, bool) {
	value, ok := s.values[key]
	return value, ok
}

// Apply removes private metadata and restores launcher-owned values after dotenv loading.
func (s Set) Apply() error {
	if err := os.Unsetenv(MetadataKey); err != nil {
		return fmt.Errorf("remove managed environment metadata: %w", err)
	}
	for _, key := range s.keys {
		if err := os.Setenv(key, s.values[key]); err != nil {
			return fmt.Errorf("restore managed environment %s: %w", key, err)
		}
	}
	return nil
}

// CommandEnvironment forces launcher-owned values over command-specific configuration.
func (s Set) CommandEnvironment(base map[string]string) map[string]string {
	containsMetadata := false
	for key := range base {
		if strings.EqualFold(key, MetadataKey) {
			containsMetadata = true
			break
		}
	}
	if len(s.keys) == 0 && !containsMetadata {
		return base
	}
	merged := make(map[string]string, len(base)+len(s.keys))
	for key, value := range base {
		if s.managesFolded(key) {
			continue
		}
		merged[key] = value
	}
	for _, key := range s.keys {
		merged[key] = s.values[key]
	}
	for key := range merged {
		if strings.EqualFold(key, MetadataKey) {
			delete(merged, key)
		}
	}
	return merged
}

// AppEnvironment marks ordinary managed values for one generated App to restore after its own dotenv loading.
func (s Set) AppEnvironment(base map[string]string, derived map[string]string) (map[string]string, error) {
	merged := s.CommandEnvironment(base)
	if len(s.keys) == 0 {
		return merged, nil
	}
	keys := append([]string(nil), s.keys...)
	seen := make(map[string]struct{}, len(keys)+len(derived))
	for _, key := range keys {
		seen[strings.ToUpper(key)] = struct{}{}
	}
	for key, value := range derived {
		if !isStrictEnvironmentKey(key) || isReservedKey(key) {
			return nil, fmt.Errorf("derived managed environment contains invalid key %q", key)
		}
		foldedKey := strings.ToUpper(key)
		if _, explicitlyManaged := seen[foldedKey]; explicitlyManaged {
			continue
		}
		for configuredKey := range merged {
			if strings.EqualFold(configuredKey, key) {
				delete(merged, configuredKey)
			}
		}
		merged[key] = value
		keys = append(keys, key)
		seen[foldedKey] = struct{}{}
	}
	sort.Slice(keys, func(left, right int) bool {
		return strings.ToUpper(keys[left]) < strings.ToUpper(keys[right])
	})
	merged[MetadataKey] = strings.Join(keys, ",")
	return merged, nil
}

// managesFolded prevents differently cased command configuration from winning on Windows.
func (s Set) managesFolded(key string) bool {
	for _, managedKey := range s.keys {
		if strings.EqualFold(key, managedKey) {
			return true
		}
	}
	return false
}

// capturePresent validates and snapshots a marker that the caller guarantees is present.
func capturePresent(raw string) (Set, error) {
	keys, err := parseKeys(raw)
	if err != nil {
		return Set{}, err
	}
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		value, ok := os.LookupEnv(key)
		if !ok {
			return Set{}, fmt.Errorf("%s lists %q, but that environment value is not present", MetadataKey, key)
		}
		values[key] = value
	}
	return Set{keys: keys, values: values}, nil
}

// parseKeys enforces a deterministic metadata representation before any dotenv mutation occurs.
func parseKeys(raw string) ([]string, error) {
	if raw == "" {
		return nil, fmt.Errorf("%s must list at least one environment key", MetadataKey)
	}
	if strings.TrimSpace(raw) != raw {
		return nil, fmt.Errorf("%s must not contain surrounding whitespace", MetadataKey)
	}
	keys := strings.Split(raw, ",")
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if key == "" {
			return nil, fmt.Errorf("%s contains an empty environment key", MetadataKey)
		}
		if !isStrictEnvironmentKey(key) {
			return nil, fmt.Errorf("%s contains invalid environment key %q", MetadataKey, key)
		}
		if isReservedKey(key) {
			return nil, fmt.Errorf("%s contains reserved environment key %q", MetadataKey, key)
		}
		foldedKey := strings.ToUpper(key)
		if _, ok := seen[foldedKey]; ok {
			return nil, fmt.Errorf("%s contains duplicate environment key %q", MetadataKey, key)
		}
		seen[foldedKey] = struct{}{}
	}
	if !sort.SliceIsSorted(keys, func(left, right int) bool {
		return strings.ToUpper(keys[left]) < strings.ToUpper(keys[right])
	}) {
		return nil, fmt.Errorf("%s environment keys must be sorted", MetadataKey)
	}
	return keys, nil
}

// isStrictEnvironmentKey accepts the portable shell identifier grammar shared by supported hosts.
func isStrictEnvironmentKey(key string) bool {
	for index, char := range key {
		if char == '_' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || index > 0 && char >= '0' && char <= '9' {
			continue
		}
		return false
	}
	return key != ""
}

// isReservedKey keeps framework control-plane entries outside launcher-managed application state.
func isReservedKey(key string) bool {
	foldedKey := strings.ToUpper(key)
	if strings.HasPrefix(foldedKey, "FORJ_INTERNAL_") {
		return true
	}
	_, ok := reservedKeys[foldedKey]
	return ok
}
