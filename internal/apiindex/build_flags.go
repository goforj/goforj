package apiindex

import (
	"fmt"
	"sort"
	"strings"
)

// BuildTagsFromArgs extracts the final tag selection and rejects source modes indexing cannot mirror safely.
func BuildTagsFromArgs(args []string) ([]string, error) {
	value := ""
	found := false
	for index := 0; index < len(args); index++ {
		argument := strings.TrimSpace(args[index])
		if argument == "--" {
			break
		}
		switch {
		case argument == "-overlay" || argument == "--overlay" || strings.HasPrefix(argument, "-overlay=") || strings.HasPrefix(argument, "--overlay="):
			return nil, fmt.Errorf("go build %s is not supported because API indexing cannot mirror source overlays", argument)
		case argument == "-modfile" || argument == "--modfile" || strings.HasPrefix(argument, "-modfile=") || strings.HasPrefix(argument, "--modfile="):
			return nil, fmt.Errorf("go build %s is not supported because API indexing cannot mirror alternate module files", argument)
		case argument == "-race" || argument == "--race" || argument == "-msan" || argument == "--msan" || argument == "-asan" || argument == "--asan":
			return nil, fmt.Errorf("go build %s is not supported because API indexing cannot mirror implicit instrumentation build tags", argument)
		case argument == "-compiler" || argument == "--compiler" || strings.HasPrefix(argument, "-compiler=") || strings.HasPrefix(argument, "--compiler="):
			return nil, fmt.Errorf("go build %s is not supported because API indexing cannot mirror alternate compiler constraints", argument)
		case argument == "-tags" || argument == "--tags":
			if index+1 >= len(args) {
				return nil, fmt.Errorf("go build %s requires a tag list", argument)
			}
			index++
			value = args[index]
			found = true
		case strings.HasPrefix(argument, "-tags="):
			value = strings.TrimPrefix(argument, "-tags=")
			found = true
		case strings.HasPrefix(argument, "--tags="):
			value = strings.TrimPrefix(argument, "--tags=")
			found = true
		}
	}
	if !found {
		return nil, nil
	}
	tags := parseBuildTags(value)
	if len(tags) == 0 {
		return nil, fmt.Errorf("go build -tags requires at least one non-empty tag")
	}
	return tags, nil
}

// ValidateGOFLAGS rejects environment-selected inputs that indexing cannot mirror during source analysis.
func ValidateGOFLAGS(goFlags string) error {
	fields := strings.Fields(goFlags)
	for _, field := range fields {
		flag := field
		if strings.HasPrefix(flag, "--") {
			flag = "-" + strings.TrimPrefix(flag, "--")
		}
		switch {
		case flag == "-overlay" || strings.HasPrefix(flag, "-overlay="):
			return fmt.Errorf("GOFLAGS %s is not supported because API indexing cannot mirror source overlays", field)
		case flag == "-modfile" || strings.HasPrefix(flag, "-modfile="):
			return fmt.Errorf("GOFLAGS %s is not supported because API indexing cannot mirror alternate module files", field)
		case flag == "-race" || flag == "-msan" || flag == "-asan":
			return fmt.Errorf("GOFLAGS %s is not supported because API indexing cannot mirror implicit instrumentation build tags", field)
		case flag == "-compiler" || strings.HasPrefix(flag, "-compiler="):
			return fmt.Errorf("GOFLAGS %s is not supported because API indexing cannot mirror alternate compiler constraints", field)
		}
	}
	return nil
}

// parseBuildTags normalizes the comma- and whitespace-separated syntax accepted by the Go command.
func parseBuildTags(value string) []string {
	seen := map[string]struct{}{}
	for _, tag := range strings.FieldsFunc(strings.Trim(strings.TrimSpace(value), "'\""), func(character rune) bool {
		return character == ',' || character == ' ' || character == '\t'
	}) {
		if tag = strings.TrimSpace(tag); tag != "" {
			seen[tag] = struct{}{}
		}
	}
	tags := make([]string, 0, len(seen))
	for tag := range seen {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}
