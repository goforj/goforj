package atlas

import (
	"fmt"
	"os"

	"github.com/goforj/atlas/skills"
)

// MakeSkillCmd creates a project-owned Atlas skill.
type MakeSkillCmd struct {
	Name string `arg:"" help:"Skill name in lowercase kebab-case, such as checkout-rules"`
}

// NewMakeSkillCmd creates a MakeSkillCmd.
func NewMakeSkillCmd() *MakeSkillCmd {
	return &MakeSkillCmd{}
}

// Signature returns the Kong metadata for MakeSkillCmd.
func (*MakeSkillCmd) Signature() string {
	return `name:"atlas:make-skill" help:"Create a project-owned Atlas skill"`
}

// Run creates a project-owned Atlas skill template.
func (c *MakeSkillCmd) Run() error {
	path, err := skills.ScaffoldProjectSkill(".", c.Name)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Created Atlas project skill: %s\n", path)
	return nil
}
