package apiindex

import (
	"reflect"
	"testing"
)

// TestBuildTagsFromArgsMatchesGoFlagPrecedence verifies the last command-line tag value wins.
func TestBuildTagsFromArgsMatchesGoFlagPrecedence(t *testing.T) {
	tags, err := BuildTagsFromArgs([]string{"-tags=old,shared", "-trimpath", "--tags", "new shared", "./cmd/app"})
	if err != nil {
		t.Fatalf("parse go build tags: %v", err)
	}
	if want := []string{"new", "shared"}; !reflect.DeepEqual(tags, want) {
		t.Fatalf("go build tags = %v, want %v", tags, want)
	}
	if tags, err := BuildTagsFromArgs([]string{"-trimpath", "./cmd/app"}); err != nil || tags != nil {
		t.Fatalf("tag-free go build selection = %v, %v, want nil", tags, err)
	}
}

// TestBuildTagsFromArgsRejectsUnmirroredSourceFlags prevents indexing a different source surface than the final binary.
func TestBuildTagsFromArgsRejectsUnmirroredSourceFlags(t *testing.T) {
	for _, arguments := range [][]string{
		{"-overlay", "overlay.json"},
		{"--overlay=overlay.json"},
		{"-modfile", "alternate.mod"},
		{"--modfile=alternate.mod"},
		{"-race"},
		{"--msan"},
		{"-asan"},
		{"-compiler=gccgo"},
	} {
		if _, err := BuildTagsFromArgs(arguments); err == nil {
			t.Fatalf("source-affecting arguments %v were accepted", arguments)
		}
	}
}

// TestValidateGOFLAGSRejectsUnmirroredInputs keeps analysis from starting under a divergent source environment.
func TestValidateGOFLAGSRejectsUnmirroredInputs(t *testing.T) {
	for _, goFlags := range []string{
		"-overlay=overlay.json",
		"--overlay=overlay.json",
		"-modfile alternate.mod",
		"--modfile alternate.mod",
		"-race",
		"--race",
		"-msan",
		"--msan",
		"-asan",
		"--asan",
		"-compiler=gccgo",
		"--compiler=gccgo",
	} {
		if err := ValidateGOFLAGS(goFlags); err == nil {
			t.Fatalf("GOFLAGS %q were accepted", goFlags)
		}
	}
	if err := ValidateGOFLAGS("-trimpath -tags=dev"); err != nil {
		t.Fatalf("supported GOFLAGS were rejected: %v", err)
	}
}
