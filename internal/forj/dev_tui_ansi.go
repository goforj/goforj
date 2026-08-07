package forj

import (
	"regexp"
	"strings"
)

var ansiCSI = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
var ansiOSC = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)
var ansiSingleEscape = regexp.MustCompile(`\x1b(?:[@-Z\\-_]|[78])`)
var ansiC0Controls = regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1a\x1c-\x1f\x7f]`)

// splitANSITail centralizes split ansitail behavior so callers follow the same contract.
func splitANSITail(raw string) (string, string) {
	last := strings.LastIndex(raw, "\x1b[")
	if last == -1 {
		return raw, ""
	}
	seq := raw[last:]
	for i := 2; i < len(seq); i++ {
		b := seq[i]
		if b >= 0x40 && b <= 0x7e {
			return raw, ""
		}
	}
	return raw[:last], seq
}

// sanitizeCSI centralizes sanitize csi behavior so callers follow the same contract.
func sanitizeCSI(input string) string {
	input = ansiOSC.ReplaceAllString(input, "")
	input = ansiSingleEscape.ReplaceAllString(input, "")
	input = ansiCSI.ReplaceAllStringFunc(input, func(seq string) string {
		if strings.HasSuffix(seq, "m") {
			return seq
		}
		return ""
	})
	return ansiC0Controls.ReplaceAllString(input, "")
}
