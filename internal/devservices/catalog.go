// Package devservices defines the optional local tools published through Docker Compose profiles.
package devservices

import (
	"sort"
	"strings"

	"github.com/goforj/goforj/project"
)

// Key identifies one built-in developer service independently from the infrastructure it provides.
type Key string

const (
	// KeyRedis identifies the Redis developer service.
	KeyRedis Key = "redis"
	// KeyRustFS identifies the RustFS S3-compatible developer service.
	KeyRustFS Key = "rustfs"
	// KeyOpenSearch identifies the standalone OpenSearch developer service.
	KeyOpenSearch Key = "opensearch"
)

// Definition describes one optional Compose profile and the infrastructure capability it provides.
type Definition struct {
	Key      Key
	Label    string
	Profile  string
	Provides project.ServiceKey
	Order    int
}

var catalog = []Definition{
	{Key: KeyRedis, Label: "Redis", Profile: "redis", Provides: project.ServiceRedis, Order: 10},
	{Key: KeyRustFS, Label: "RustFS", Profile: "rustfs", Provides: project.ServiceStorageS3, Order: 20},
	{Key: KeyOpenSearch, Label: "OpenSearch", Profile: "opensearch", Order: 30},
}

// Catalog returns a defensive copy in stable presentation and reconciliation order.
func Catalog() []Definition {
	definitions := make([]Definition, len(catalog))
	copy(definitions, catalog)
	sort.SliceStable(definitions, func(left, right int) bool {
		return definitions[left].Order < definitions[right].Order
	})
	return definitions
}

// DefinitionByKey returns one developer-service definition by its exact stable key.
func DefinitionByKey(key Key) (Definition, bool) {
	for _, definition := range Catalog() {
		if definition.Key == key {
			return definition, true
		}
	}
	return Definition{}, false
}

// DefinitionByProfile returns one developer-service definition by its exact Compose profile token.
func DefinitionByProfile(profile string) (Definition, bool) {
	profile = strings.TrimSpace(profile)
	for _, definition := range Catalog() {
		if definition.Profile == profile {
			return definition, true
		}
	}
	return Definition{}, false
}

// Enabled returns known definitions selected by exact profile tokens in catalog order.
func Enabled(value string) []Definition {
	tokens := make(map[string]struct{})
	for _, token := range strings.Split(value, ",") {
		token = strings.TrimSpace(token)
		if token != "" {
			tokens[token] = struct{}{}
		}
	}

	enabled := make([]Definition, 0, len(tokens))
	for _, definition := range Catalog() {
		if _, ok := tokens[definition.Profile]; ok {
			enabled = append(enabled, definition)
		}
	}
	return enabled
}
