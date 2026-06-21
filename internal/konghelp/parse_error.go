package konghelp

import (
	"fmt"
	"strings"

	"github.com/alecthomas/kong"
)

// CommandParseError adds the first command Help example to parse errors.
func CommandParseError(parser *kong.Kong, command string, err error) error {
	if err == nil {
		return nil
	}
	example := commandExample(parser, command)
	if example == "" {
		return err
	}
	return fmt.Errorf("%w\nexample: %s", err, example)
}

// commandExample extracts the first declared example so parse errors can offer a useful next attempt.
func commandExample(parser *kong.Kong, command string) string {
	if parser == nil || parser.Model == nil || parser.Model.Node == nil || command == "" {
		return ""
	}
	node := findCommandNode(parser.Model.Node, command)
	if node == nil {
		return ""
	}
	return FirstExampleFromDetail(node.Detail)
}

// findCommandNode walks nested Kong commands because preboot command aliases can live below the root.
func findCommandNode(node *kong.Node, command string) *kong.Node {
	if node.Type == kong.CommandNode && (node.Name == command || stringSliceContains(node.Aliases, command)) {
		return node
	}
	for _, child := range node.Children {
		if child.Type == kong.CommandNode && (child.Name == command || stringSliceContains(child.Aliases, command)) {
			return child
		}
		if found := findCommandNode(child, command); found != nil {
			return found
		}
	}
	return nil
}

// FirstExampleFromDetail extracts the first entry under an Examples section.
func FirstExampleFromDetail(detail string) string {
	inExamples := false
	for _, line := range strings.Split(detail, "\n") {
		clean := strings.TrimSpace(line)
		if clean == "" {
			continue
		}
		if strings.EqualFold(strings.TrimSuffix(clean, ":"), "examples") {
			inExamples = true
			continue
		}
		if inExamples {
			return clean
		}
	}
	return ""
}

// stringSliceContains avoids pulling in a broader helper package for this tiny alias check.
func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
