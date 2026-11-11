package main

import (
	"flag"
	"io"
	"testing"
)

// TestSpikeArgumentsRejectIncompleteRuns keeps invalid CLI input from starting renders or modifying a trial.
func TestSpikeArgumentsRejectIncompleteRuns(t *testing.T) {
	for _, arguments := range [][]string{
		nil, {"unknown"}, {"reference"}, {"reference", "--forj", "forj"},
		{"prepare", "--forj", "forj"}, {"auth", "--forj", "forj"},
		{"evaluate"}, {"evaluate", "one", "two"}, {"prepare", "--forj", "forj", "--source", "app", "--revision="},
		{"reference", "--forj", "forj", "--project", "app", "unexpected"}, {"reference", "--unknown"},
	} {
		if _, err := parseSpikeOptions(arguments, io.Discard); err == nil {
			t.Fatalf("accepted incomplete arguments: %v", arguments)
		}
	}
}

// TestSpikeArgumentsRetainSelections verifies each supported experiment reaches the intended input contract.
func TestSpikeArgumentsRetainSelections(t *testing.T) {
	for _, arguments := range [][]string{
		{"reference", "--forj", "forj", "--project", "one", "--project", "two", "--from-renderer", "old"},
		{"prepare", "--forj", "forj", "--source", "app"},
		{"evaluate", "workspace"}, {"auth", "--forj", "forj", "--ditracker", "app"},
	} {
		options, err := parseSpikeOptions(arguments, io.Discard)
		if err != nil || options.command != arguments[0] {
			t.Fatalf("arguments=%v options=%+v err=%v", arguments, options, err)
		}
		if options.command == "reference" && (len(options.projects) != 2 || options.comparison != "old") {
			t.Fatalf("lost renderer or project selections: %+v", options)
		}
	}
	if _, err := parseSpikeOptions([]string{"reference", "--help"}, io.Discard); err != flag.ErrHelp {
		t.Fatalf("help error=%v", err)
	}
}
