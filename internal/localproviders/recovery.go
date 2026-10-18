package localproviders

import (
	"encoding/json"
	"fmt"
)

// pendingChange retains both sides of an interrupted publication until the active environment selects one unambiguously.
type pendingChange struct {
	Before   map[string]string `json:"before,omitempty"`
	After    map[string]string `json:"after,omitempty"`
	Previous map[string]string `json:"previous,omitempty"`
	Applied  map[string]string `json:"applied,omitempty"`
}

// encodeState keeps recovery files readable while retaining absence separately from empty values.
func encodeState(value state) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// changedValues snapshots only touched assignments so unrelated credentials remain solely in the active environment.
func changedValues(before, after map[string]string) (map[string]string, map[string]string) {
	left, right := map[string]string{}, map[string]string{}
	for key := range changedKeys(before, after) {
		if value, present := before[key]; present {
			left[key] = value
		}
		if value, present := after[key]; present {
			right[key] = value
		}
	}
	return left, right
}

// matchesChange compares presence and value over exactly the assignments touched by a pending operation.
func matchesChange(current, expected, other map[string]string) bool {
	for key := range changedKeys(expected, other) {
		if !sameSetting(current, expected, key) {
			return false
		}
	}
	return true
}

// reconcilePending derives the active recovery point without modifying its raw optimistic-concurrency snapshot.
func (s *Session) reconcilePending() error {
	maps := []map[string]string{s.state.Previous, s.state.Applied}
	pending := s.state.Pending
	if pending != nil {
		maps = append(maps, pending.Before, pending.After, pending.Previous, pending.Applied)
	}
	for _, values := range maps {
		if err := s.Validate(values); err != nil {
			return fmt.Errorf("invalid local-provider restore point: %w", err)
		}
	}
	if pending == nil {
		return nil
	}
	if len(changedKeys(pending.Before, pending.After)) == 0 {
		return fmt.Errorf("invalid pending local-provider change: no changed assignments")
	}
	switch {
	case matchesChange(s.Current, pending.Before, pending.After):
		s.state.Version = 1
		s.state.Pending = nil
	case matchesChange(s.Current, pending.After, pending.Before):
		s.state = state{Version: 1, Previous: pending.Previous, Applied: pending.Applied}
		s.pendingCleanup = len(changedKeys(s.state.Previous, s.state.Applied)) == 0
	default:
		return fmt.Errorf(".env matches neither side of the pending local-provider change; recovery data is retained in %s; review the affected settings before retrying", stateName)
	}
	return nil
}
