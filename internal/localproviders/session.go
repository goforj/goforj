package localproviders

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/goforj/goforj/internal/generate"
	"github.com/goforj/goforj/project"
)

const stateName = ".env.local-providers.backup"
const lockName = ".env.local-providers.lock"
const ignoreRules = "\n# GoForj local-provider recovery files\n/.env\n.env.local-providers.*\n"

// state retains one reversible change, including the distinction between absent and empty settings.
type state struct {
	Version  int               `json:"version"`
	Previous map[string]string `json:"previous,omitempty"`
	Applied  map[string]string `json:"applied,omitempty"`
}

// Session holds the project configuration reviewed before a local-provider switch.
type Session struct {
	Root          string
	Current       map[string]string
	Resources     []Resource
	config        *project.Config
	env           document
	files         map[string]file
	state         state
	databaseNames []string
}

// Open reads project-owned configuration without changing the process environment.
func Open(root string) (*Session, error) {
	config, err := project.LoadProjectConfigAt(root)
	if err != nil {
		return nil, err
	}
	s := &Session{Root: root, config: config, files: map[string]file{}}
	for _, name := range []string{".env", ".env.example", stateName, ".gitignore"} {
		f, err := readFile(root, name)
		if err != nil {
			return nil, err
		}
		s.files[name] = f
	}
	if !s.files[".env"].exists {
		return nil, fmt.Errorf(".env is missing; run forj env:init first")
	}
	s.env, err = parseDocument(s.files[".env"].before)
	if err != nil {
		return nil, fmt.Errorf("read .env: %w", err)
	}
	s.Current = managedValues(config, s.env.values)
	inventory := clone(s.env.values)
	if s.files[".env.example"].exists {
		example, err := parseDocument(s.files[".env.example"].before)
		if err != nil {
			return nil, fmt.Errorf("read .env.example: %w", err)
		}
		for key, value := range example.values {
			if _, ok := inventory[key]; !ok {
				inventory[key] = value
			}
		}
	}
	s.Resources = resources(root, config, inventory)
	s.databaseNames = generate.ResourceNames(root, inventory)[project.ResourceDatabase]
	if err := s.Validate(s.Current); err != nil {
		return nil, fmt.Errorf("invalid resource configuration in .env: %w", err)
	}
	if s.files[stateName].exists {
		if err := json.Unmarshal(s.files[stateName].before, &s.state); err != nil {
			return nil, fmt.Errorf("invalid local-provider restore point")
		}
		if s.state.Version != 1 {
			return nil, fmt.Errorf("unsupported local-provider restore point version")
		}
		for _, values := range []map[string]string{s.state.Previous, s.state.Applied} {
			if err := s.Validate(values); err != nil {
				return nil, fmt.Errorf("invalid local-provider restore point: %w", err)
			}
		}
	}
	return s, nil
}

// HasPrevious reports whether a local-provider change can be restored.
func (s *Session) HasPrevious() bool { return len(changedKeys(s.state.Previous, s.state.Applied)) > 0 }

// Previous restores only keys changed by the switch and rejects subsequent edits to those keys.
func (s *Session) Previous() (map[string]string, error) {
	if !s.HasPrevious() {
		return nil, fmt.Errorf("no previous local-provider configuration to restore")
	}
	values := clone(s.Current)
	for key := range changedKeys(s.state.Previous, s.state.Applied) {
		if !sameSetting(s.Current, s.state.Applied, key) {
			return nil, fmt.Errorf("%s changed since the local-provider switch; restore cannot overwrite it", key)
		}
		if value, ok := s.state.Previous[key]; ok {
			values[key] = value
		} else {
			delete(values, key)
		}
	}
	return values, s.Validate(values)
}

// Apply keeps one original restore point across reviewed provider changes.
func (s *Session) Apply(values map[string]string) error {
	if err := s.Validate(values); err != nil {
		return err
	}
	if len(changedKeys(s.Current, values)) == 0 {
		return nil
	}
	original := s.Current
	if s.HasPrevious() {
		var err error
		original, err = s.Previous()
		if err != nil {
			return err
		}
	}
	previous, applied := map[string]string{}, map[string]string{}
	for key := range changedKeys(original, values) {
		if value, present := original[key]; present {
			previous[key] = value
		}
		if value, present := values[key]; present {
			applied[key] = value
		}
	}
	return s.publish(values, state{Version: 1, Previous: previous, Applied: applied})
}

// Restore consumes the single restore point without changing unrelated settings or retaining old credentials.
func (s *Session) Restore() error {
	values, err := s.Previous()
	if err != nil {
		return err
	}
	return s.publish(values, state{Version: 1})
}

// publish updates only changed assignments so existing comments and untouched resource values survive.
func (s *Session) publish(values map[string]string, next state) error {
	env, err := s.reviewedDocument(values)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	updates := map[string][]byte{stateName: append(data, '\n'), ".env": env}
	return s.write(updates)
}

// reviewedDocument verifies that interpolation in preserved assignments cannot change the reviewed configuration.
func (s *Session) reviewedDocument(values map[string]string) ([]byte, error) {
	changed := changedKeys(s.Current, values)
	replacements := map[string]string{}
	expected := clone(s.env.values)
	for key := range changed {
		if value, ok := values[key]; ok {
			replacements[key] = value
			expected[key] = value
		} else {
			delete(expected, key)
		}
	}
	raw := s.env.replace(func(key string) bool { return changed[key] }, replacements)
	actual, err := parseDocument(raw)
	if err != nil {
		return nil, fmt.Errorf("check proposed .env: %w", err)
	}
	for _, key := range keys(changedKeys(expected, actual.values)) {
		return nil, fmt.Errorf("%s resolves differently in the proposed .env; replace dependent dotenv references with literal values before switching or restoring drivers", key)
	}
	return raw, nil
}

// read captures mutation targets for optimistic concurrency checks.
func (s *Session) read(name string) (file, error) {
	if f, ok := s.files[name]; ok {
		return f, nil
	}
	f, err := readFile(s.Root, name)
	if err == nil {
		s.files[name] = f
	}
	return f, err
}

// write protects recovery files in Git and publishes the backup before changing the active environment.
func (s *Session) write(updates map[string][]byte) error {
	lock, err := os.OpenFile(filepath.Join(s.Root, lockName), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("another local-provider operation may be running (%s): %w", lockName, err)
	}
	_ = lock.Close()
	defer os.Remove(filepath.Join(s.Root, lockName))
	for _, original := range s.files {
		if err := checkFile(s.Root, original); err != nil {
			return err
		}
	}
	ignore := s.files[".gitignore"]
	if !bytes.HasSuffix(ignore.before, []byte(ignoreRules)) {
		updates[".gitignore"] = append(bytes.Clone(ignore.before), []byte(ignoreRules)...)
	}
	var files []file
	for _, name := range keys(updates) {
		f, err := s.read(name)
		if err != nil {
			return err
		}
		if isPrivateFile(name) {
			if err := verifyPrivateFileUntracked(s.Root, name); err != nil {
				return err
			}
		}
		f.after = updates[name]
		files = append(files, f)
	}
	return commit(s.Root, files, os.Rename)
}

// changedKeys records value changes without treating an absent setting as an explicitly empty one.
func changedKeys(before, after map[string]string) map[string]bool {
	result := map[string]bool{}
	for _, values := range []map[string]string{before, after} {
		for key := range values {
			if !sameSetting(before, after, key) {
				result[key] = true
			}
		}
	}
	return result
}

// sameSetting compares both the presence and value of one assignment.
func sameSetting(left, right map[string]string, key string) bool {
	a, foundA := left[key]
	b, foundB := right[key]
	return foundA == foundB && a == b
}

// equalValues compares complete configurations including explicitly empty assignments.
func equalValues(left, right map[string]string) bool { return len(changedKeys(left, right)) == 0 }

// Overrides identifies existing runtime layers whose precedence can override the proposed local settings.
func (s *Session) Overrides() ([]string, error) {
	var result []string
	for _, name := range []string{".env.local", ".env.staging", ".env.production", ".env.host", ".env.testing"} {
		f, err := nearestRuntimeLayer(s.Root, name)
		if err != nil {
			return nil, err
		}
		if !f.exists {
			continue
		}
		d, err := parseDocument(f.before)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f.name, err)
		}
		managed := managedValues(s.config, d.values)
		if len(managed) > 0 {
			result = append(result, f.name+": "+strings.Join(keys(managed), ", "))
		}
	}
	var inherited []string
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if ok && managedKey(s.config, key) {
			inherited = append(inherited, key)
		}
	}
	if len(inherited) > 0 {
		sort.Strings(inherited)
		result = append(result, "process environment: "+strings.Join(inherited, ", "))
	}
	return result, nil
}
