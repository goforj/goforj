package project

import "testing"

func TestDevWatchMatchersEmptyTreatsUnknownFieldsAsConfiguration(t *testing.T) {
	if !(DevWatchMatchers{}).Empty() {
		t.Fatal("zero matcher should be empty")
	}
	matchers := DevWatchMatchers{Extra: map[string]any{"future_matcher": true}}
	if matchers.Empty() {
		t.Fatal("matcher with preserved future fields was classified as empty")
	}
}
