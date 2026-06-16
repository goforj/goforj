package atlas

import (
	"fmt"
	"os"

	"github.com/goforj/atlas/skills"
)

// ListSkillsCmd lists Atlas skills.
type ListSkillsCmd struct{}

// NewListSkillsCmd creates a ListSkillsCmd.
func NewListSkillsCmd() *ListSkillsCmd {
	return &ListSkillsCmd{}
}

// Signature returns the Kong metadata for ListSkillsCmd.
func (*ListSkillsCmd) Signature() string {
	return `name:"atlas:list-skills" help:"List Atlas skills"`
}

// Run prints built-in Atlas skill names.
func (*ListSkillsCmd) Run() error {
	fmt.Fprintln(os.Stdout, "Built-in skills:")
	for _, name := range skills.Names() {
		fmt.Fprintf(os.Stdout, "  %s\n", name)
	}

	projectSkills, err := skills.ProjectSkills(".")
	if err != nil {
		return err
	}
	if len(projectSkills) == 0 {
		return nil
	}
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "Project skills:")
	for _, skill := range projectSkills {
		fmt.Fprintf(os.Stdout, "  %s  %s\n", skill.Name, skill.Path)
	}
	return nil
}
