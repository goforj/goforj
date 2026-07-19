package managedenv

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

// TestCaptureSnapshotsMultipleValues verifies the private marker carries names without duplicating values.
func TestCaptureSnapshotsMultipleValues(t *testing.T) {
	t.Setenv("HARBOR_EMPTY", "")
	t.Setenv("IP_ADDRESS", "127.18.0.11")
	t.Setenv(MetadataKey, "HARBOR_EMPTY,IP_ADDRESS")

	set, err := Capture()
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	if got, want := set.Keys(), []string{"HARBOR_EMPTY", "IP_ADDRESS"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Keys() = %#v, want %#v", got, want)
	}
	if value, ok := set.Lookup("HARBOR_EMPTY"); !ok || value != "" {
		t.Fatalf("Lookup(HARBOR_EMPTY) = %q, %v; want present-empty", value, ok)
	}
	if value, ok := set.Lookup("IP_ADDRESS"); !ok || value != "127.18.0.11" {
		t.Fatalf("Lookup(IP_ADDRESS) = %q, %v", value, ok)
	}
	if _, ok := os.LookupEnv(MetadataKey); ok {
		t.Fatalf("%s remained in the process environment", MetadataKey)
	}
}

// TestCaptureRejectsMalformedMetadata verifies invalid launcher contracts fail before dotenv can hide them.
func TestCaptureRejectsMalformedMetadata(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty", raw: "", want: "at least one"},
		{name: "surrounding whitespace", raw: " IP_ADDRESS", want: "surrounding whitespace"},
		{name: "empty entry", raw: "IP_ADDRESS,", want: "empty environment key"},
		{name: "invalid first character", raw: "1P_ADDRESS", want: "invalid environment key"},
		{name: "invalid punctuation", raw: "IP-ADDRESS", want: "invalid environment key"},
		{name: "duplicate", raw: "IP_ADDRESS,IP_ADDRESS", want: "duplicate environment key"},
		{name: "case-fold duplicate", raw: "Harbor_Token,HARBOR_TOKEN", want: "duplicate environment key"},
		{name: "unsorted", raw: "IP_ADDRESS,HARBOR_TOKEN", want: "must be sorted"},
		{name: "metadata self", raw: MetadataKey, want: "reserved environment key"},
		{name: "internal namespace", raw: "FORJ_INTERNAL_OTHER", want: "reserved environment key"},
		{name: "framework app selector", raw: "FORJ_APP", want: "reserved environment key"},
		{name: "case-fold framework selector", raw: "forj_app", want: "reserved environment key"},
		{name: "plain-mode control", raw: "FORJ_DEV_PLAIN", want: "reserved environment key"},
		{name: "application layer selector", raw: "APP_ENV", want: "reserved environment key"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(MetadataKey, test.raw)
			_, err := Capture()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Capture() error = %v, want containing %q", err, test.want)
			}
			if _, ok := os.LookupEnv(MetadataKey); ok {
				t.Fatalf("%s remained after rejected metadata", MetadataKey)
			}
		})
	}
}

// TestCaptureRejectsMissingManagedValue verifies names cannot silently resolve to absent values.
func TestCaptureRejectsMissingManagedValue(t *testing.T) {
	const key = "HARBOR_MISSING_MANAGED_VALUE"
	previous, present := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if present {
			_ = os.Setenv(key, previous)
			return
		}
		_ = os.Unsetenv(key)
	})
	t.Setenv(MetadataKey, key)

	_, err := Capture()
	if err == nil || !strings.Contains(err.Error(), "is not present") {
		t.Fatalf("Capture() error = %v, want missing value", err)
	}
}

// TestApplyRestoresManagedValues verifies dotenv conflicts cannot outlive the reload boundary.
func TestApplyRestoresManagedValues(t *testing.T) {
	t.Setenv("HARBOR_TOKEN", "launcher-token")
	t.Setenv("IP_ADDRESS", "127.18.0.12")
	t.Setenv(MetadataKey, "HARBOR_TOKEN,IP_ADDRESS")
	set, err := Capture()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARBOR_TOKEN", "dotenv-token")
	t.Setenv("IP_ADDRESS", "0.0.0.0")
	t.Setenv(MetadataKey, "SHOULD_NOT_SURVIVE")

	if err := set.Apply(); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if got := os.Getenv("HARBOR_TOKEN"); got != "launcher-token" {
		t.Fatalf("HARBOR_TOKEN = %q", got)
	}
	if got := os.Getenv("IP_ADDRESS"); got != "127.18.0.12" {
		t.Fatalf("IP_ADDRESS = %q", got)
	}
	if _, ok := os.LookupEnv(MetadataKey); ok {
		t.Fatalf("%s survived Apply()", MetadataKey)
	}
}

// TestCommandEnvironmentForcesOnlyManagedKeys verifies configured command env cannot replace launcher ownership.
func TestCommandEnvironmentForcesOnlyManagedKeys(t *testing.T) {
	t.Setenv("HARBOR_TOKEN", "launcher-token")
	t.Setenv("IP_ADDRESS", "127.18.0.13")
	t.Setenv(MetadataKey, "HARBOR_TOKEN,IP_ADDRESS")
	set, err := Capture()
	if err != nil {
		t.Fatal(err)
	}
	base := map[string]string{
		"harbor_token":  "configured-token",
		"IP_ADDRESS":    "0.0.0.0",
		"API_HTTP_PORT": "3000",
		MetadataKey:     "DO_NOT_FORWARD",
	}

	got := set.CommandEnvironment(base)
	if got["HARBOR_TOKEN"] != "launcher-token" || got["IP_ADDRESS"] != "127.18.0.13" {
		t.Fatalf("CommandEnvironment() = %#v", got)
	}
	if got["API_HTTP_PORT"] != "3000" {
		t.Fatalf("API_HTTP_PORT = %q, want configured port", got["API_HTTP_PORT"])
	}
	if _, ok := got[MetadataKey]; ok {
		t.Fatalf("CommandEnvironment() forwarded %s", MetadataKey)
	}
	if base["IP_ADDRESS"] != "0.0.0.0" {
		t.Fatalf("CommandEnvironment() mutated base: %#v", base)
	}
	if _, ok := got["harbor_token"]; ok {
		t.Fatalf("CommandEnvironment() kept a case-folded managed conflict: %#v", got)
	}
}

// TestCommandEnvironmentStripsConfiguredMetadataWithoutManagedValues verifies projects cannot leak the private marker.
func TestCommandEnvironmentStripsConfiguredMetadataWithoutManagedValues(t *testing.T) {
	base := map[string]string{
		"FORJ_internal_managed_env_keys": "SHOULD_NOT_LEAK",
		"API_HTTP_PORT":                  "3000",
	}
	got := (Set{}).CommandEnvironment(base)
	if _, ok := got["FORJ_internal_managed_env_keys"]; ok {
		t.Fatalf("CommandEnvironment() leaked configured metadata: %#v", got)
	}
	if got["API_HTTP_PORT"] != "3000" {
		t.Fatalf("CommandEnvironment() changed ordinary config: %#v", got)
	}
	if base["FORJ_internal_managed_env_keys"] == "" {
		t.Fatalf("CommandEnvironment() mutated the project map: %#v", base)
	}
}

// TestAppEnvironmentAddsConsumableMetadata verifies only generated App children receive the private names marker.
func TestAppEnvironmentAddsConsumableMetadata(t *testing.T) {
	t.Setenv("HARBOR_TOKEN", "comma,value\nsecond line")
	t.Setenv("IP_ADDRESS", "127.18.0.14")
	t.Setenv(MetadataKey, "HARBOR_TOKEN,IP_ADDRESS")
	set, err := Capture()
	if err != nil {
		t.Fatal(err)
	}
	got, err := set.AppEnvironment(
		map[string]string{"API_HTTP_HOST": "0.0.0.0", "API_HTTP_PORT": "3000"},
		map[string]string{"API_HTTP_HOST": "127.18.0.14"},
	)
	if err != nil {
		t.Fatalf("AppEnvironment() error = %v", err)
	}
	if got[MetadataKey] != "API_HTTP_HOST,HARBOR_TOKEN,IP_ADDRESS" {
		t.Fatalf("%s = %q", MetadataKey, got[MetadataKey])
	}
	if got["HARBOR_TOKEN"] != "comma,value\nsecond line" || got["IP_ADDRESS"] != "127.18.0.14" {
		t.Fatalf("AppEnvironment() lost ordinary managed values: %#v", got)
	}
	if got["API_HTTP_HOST"] != "127.18.0.14" || got["API_HTTP_PORT"] != "3000" {
		t.Fatalf("AppEnvironment() changed host/port mapping: %#v", got)
	}
}

// TestAbsentMarkerPreservesCommandEnvironment verifies ordinary forj dev behavior remains byte-for-byte unmodified.
func TestAbsentMarkerPreservesCommandEnvironment(t *testing.T) {
	previous, present := os.LookupEnv(MetadataKey)
	if err := os.Unsetenv(MetadataKey); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if present {
			_ = os.Setenv(MetadataKey, previous)
		}
	})
	set, err := Capture()
	if err != nil {
		t.Fatal(err)
	}
	if set.Len() != 0 {
		t.Fatalf("Len() = %d", set.Len())
	}
	base := map[string]string{"IP_ADDRESS": "configured"}
	got := set.CommandEnvironment(base)
	got["IP_ADDRESS"] = "same-map"
	if base["IP_ADDRESS"] != "same-map" {
		t.Fatal("zero Set cloned or replaced the command environment")
	}
}
