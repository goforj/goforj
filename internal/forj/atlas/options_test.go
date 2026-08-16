package atlas

import (
	"testing"

	"github.com/alecthomas/kong"
)

// TestUpdateCmdParsesTriStateSurfaces proves omitted, enabled, and disabled selections remain distinguishable at the CLI boundary.
func TestUpdateCmdParsesTriStateSurfaces(t *testing.T) {
	var root struct {
		Update UpdateCmd `cmd:"" name:"atlas:update"`
	}
	parser, err := kong.New(&root)
	if err != nil {
		t.Fatalf("kong.New(): %v", err)
	}
	if _, err := parser.Parse([]string{"atlas:update", "--guidelines=false", "--skills"}); err != nil {
		t.Fatalf("Parse(): %v", err)
	}
	if root.Update.Guidelines == nil || *root.Update.Guidelines {
		t.Fatalf("guidelines = %#v, want explicit false", root.Update.Guidelines)
	}
	if root.Update.Skills == nil || !*root.Update.Skills {
		t.Fatalf("skills = %#v, want explicit true", root.Update.Skills)
	}
	if root.Update.MCP != nil {
		t.Fatalf("MCP = %#v, want omitted", root.Update.MCP)
	}
}
