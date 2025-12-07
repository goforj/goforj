package env

import "bytes"

// containsAny checks whether the byte slice contains any of the given substrings.
func containsAny(haystack []byte, subs ...string) bool {
	for _, s := range subs {
		if bytes.Contains(haystack, []byte(s)) {
			return true
		}
	}
	return false
}
