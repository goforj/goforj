package envfile_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/envfile"
)

// TestLookupUsesDotenvSemantics verifies exports, quotes, interpolation, and final-assignment precedence remain parser-owned.
func TestLookupUsesDotenvSemantics(t *testing.T) {
	lines := strings.Split("HOST=cache.internal\nexport CACHE_URL=redis://${HOST}:6379\nCACHE_URL='redis://override:6379' # owner override", "\n")
	got, found := envfile.Lookup(lines, "CACHE_URL")
	if !found || got != "redis://override:6379" {
		t.Fatalf("Lookup() = %q, %t, want final quoted value", got, found)
	}
}

// TestLookupFallsBackAroundMalformedLines verifies an unrelated syntax error cannot hide a concrete owner assignment.
func TestLookupFallsBackAroundMalformedLines(t *testing.T) {
	lines := []string{"BROKEN='unterminated", "CACHE_DRIVER=memory", "# CACHE_DRIVER=file", "CACHE_DRIVER=redis"}
	got, found := envfile.Lookup(lines, "CACHE_DRIVER")
	if !found || got != "redis" {
		t.Fatalf("Lookup() = %q, %t, want final raw assignment", got, found)
	}
}

// TestLookupFallbackPreservesDotenvValues verifies recovery still decodes quotes and references between valid assignments.
func TestLookupFallbackPreservesDotenvValues(t *testing.T) {
	lines := []string{
		"HOST=cache.internal",
		"BROKEN='unterminated",
		`CACHE_URL="redis://${HOST}:6379"`,
	}
	got, found := envfile.Lookup(lines, "CACHE_URL")
	if !found || got != "redis://cache.internal:6379" {
		t.Fatalf("Lookup() = %q, %t, want decoded interpolated value", got, found)
	}
}

// TestSetFinalPreservesPrecedenceAndTerminalNewline verifies updates target the controlling assignment without changing newline ownership.
func TestSetFinalPreservesPrecedenceAndTerminalNewline(t *testing.T) {
	lines := []string{"CACHE_DRIVER=file", "export CACHE_DRIVER='memory'", ""}
	got := envfile.SetFinal(lines, "CACHE_DRIVER", "redis")
	want := []string{"CACHE_DRIVER=file", "CACHE_DRIVER=redis", ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SetFinal() = %#v, want %#v", got, want)
	}

	got = envfile.SetFinal([]string{"CACHE_DRIVER=memory", ""}, "QUEUE_DRIVER", "workerpool")
	want = []string{"CACHE_DRIVER=memory", "QUEUE_DRIVER=workerpool", ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SetFinal() append = %#v, want %#v", got, want)
	}
}

// TestAssignmentParsersKeepSemanticAndSourceScanningDistinct verifies operational parsing ignores comments while raw scanning can find commented placeholders.
func TestAssignmentParsersKeepSemanticAndSourceScanningDistinct(t *testing.T) {
	key, value, ok := envfile.ParseAssignment(" export CACHE_DRIVER = 'memory' # portable")
	if !ok || key != "CACHE_DRIVER" || value != "memory" {
		t.Fatalf("ParseAssignment() = %q, %q, %t", key, value, ok)
	}
	if _, _, ok := envfile.ParseAssignment("# CACHE_DRIVER=redis"); ok {
		t.Fatal("ParseAssignment() accepted a commented placeholder")
	}
	key, ok = envfile.ScanKey(" # export CACHE_DRIVER = redis # optional")
	if !ok || key != "CACHE_DRIVER" {
		t.Fatalf("ScanKey() = %q, %t", key, ok)
	}
	key, ok = envfile.ScanKey("CUSTOM_TOKEN: yaml-style-secret")
	if !ok || key != "CUSTOM_TOKEN" {
		t.Fatalf("ScanKey() YAML assignment = %q, %t", key, ok)
	}
}

// TestEncodeValueRoundTripsLiteralSecretCharacters verifies stored input cannot become interpolation or another assignment.
func TestEncodeValueRoundTripsLiteralSecretCharacters(t *testing.T) {
	want := "prefix $TOKEN `command`\nINJECTED=true"
	line := "SECRET=" + envfile.EncodeValue(want)
	key, got, ok := envfile.ParseAssignment(line)
	if !ok || key != "SECRET" || got != want {
		t.Fatalf("encoded assignment = %q; parsed %q, %q, %t", line, key, got, ok)
	}
}

// TestIsValidKeyUsesPortableDotenvNames verifies keys are safe to pass through every supported environment parser.
func TestIsValidKeyUsesPortableDotenvNames(t *testing.T) {
	for _, test := range []struct {
		key  string
		want bool
	}{
		{key: "APP_KEY", want: true},
		{key: "_PRIVATE", want: true},
		{key: "9INVALID", want: false},
		{key: "BAD-KEY", want: false},
		{key: "", want: false},
	} {
		if got := envfile.IsValidKey(test.key); got != test.want {
			t.Errorf("IsValidKey(%q) = %t, want %t", test.key, got, test.want)
		}
	}
}
