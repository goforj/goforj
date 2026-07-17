package konghelp

import (
	"bytes"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/goforj/str/v2"
)

// Preview renders one of the real help formatters against an example Kong command tree.
func Preview(format string) string {
	format = str.Of(format).Trim().ToLower().String()
	parser, err := kong.New(
		helpPreviewCommandSurface(format),
		helpPreviewName(format),
		helpPreviewDescription(format),
	)
	if err != nil {
		return ""
	}
	ctx, err := kong.Trace(parser, []string{})
	if err != nil {
		return ""
	}
	var out bytes.Buffer
	switch format {
	case formatGuided:
		renderGuidedFormatter(&out, kong.HelpOptions{}, ctx)
	case formatExternalCLI:
		renderExternalCLIFormatter(&out, kong.HelpOptions{}, ctx)
	default:
		renderFrameworkFormatter(&out, kong.HelpOptions{}, ctx)
	}
	return strings.TrimRight(out.String(), "\n")
}

// helpPreviewCommandSurface uses framework-shaped commands only for the framework formatter preview.
func helpPreviewCommandSurface(format string) interface{} {
	if format == formatFramework || format == "" {
		return &helpFrameworkPreviewCLI{}
	}
	return &helpPreviewCLI{}
}

// helpPreviewName keeps each preview close to the kind of binary the formatter targets.
func helpPreviewName(format string) kong.Option {
	if format == formatFramework || format == "" {
		return kong.Name("app")
	}
	return kong.Name("tasks")
}

// helpPreviewDescription gives the preview enough context to show formatter-specific hierarchy.
func helpPreviewDescription(format string) kong.Option {
	if format == formatFramework || format == "" {
		return kong.Description("Application command surface")
	}
	return kong.Description("Track project tasks from the terminal")
}

// helpPreviewCLI models a small product CLI for external and guided formatter previews.
type helpPreviewCLI struct {
	Add  helpPreviewAddCmd  `cmd:"" help:"Add a task" group:"tasks"`
	List helpPreviewListCmd `cmd:"" help:"List open tasks" group:"tasks"`
	Done helpPreviewDoneCmd `cmd:"" help:"Mark a task complete" group:"tasks"`
}

// helpFrameworkPreviewCLI models GoForj-style category commands for the framework preview.
type helpFrameworkPreviewCLI struct {
	About         helpFrameworkPreviewCommand `cmd:"" name:"about" help:"Show app environment and services"`
	MakeCommand   helpFrameworkPreviewCommand `cmd:"" name:"make:command" help:"Create a new CLI command"`
	MakeMigration helpFrameworkPreviewCommand `cmd:"" name:"make:migration" help:"Create a new migration"`
	CacheShell    helpFrameworkPreviewCommand `cmd:"" name:"cache:shell" help:"Open a configured cache shell"`
	DBShell       helpFrameworkPreviewCommand `cmd:"" name:"db:shell" help:"Open a configured database shell"`
	Migrate       helpFrameworkPreviewCommand `cmd:"" name:"migrate" help:"Run database migrations"`
}

// helpFrameworkPreviewCommand keeps framework preview commands inert while Kong builds the command tree.
type helpFrameworkPreviewCommand struct{}

// Run satisfies Kong command execution for preview-only framework commands.
func (helpFrameworkPreviewCommand) Run() error { return nil }

// Help provides root examples so guided previews can show the examples-first layout.
func (helpPreviewCLI) Help() string {
	return strings.TrimSpace(`
Examples:
  tasks add "Review PR" --tag code
  tasks list --all
  tasks done 42
`)
}

// helpPreviewAddCmd gives command-specific help enough arguments and flags to demonstrate alignment.
type helpPreviewAddCmd struct {
	Title string `arg:"" help:"Task title"`
	Due   string `help:"Due date"`
	Tag   string `short:"t" help:"Task tag"`
}

// Run satisfies Kong command execution for the preview add command.
func (helpPreviewAddCmd) Run() error { return nil }

// Help provides command examples so selected-command previews exercise example rendering.
func (helpPreviewAddCmd) Help() string {
	return strings.TrimSpace(`
Examples:
  tasks add "Review PR"
  tasks add "Ship release notes" --due tomorrow --tag docs
`)
}

// helpPreviewListCmd supplies a simple flag-only command for preview command lists.
type helpPreviewListCmd struct {
	All bool `short:"a" help:"Include completed tasks"`
}

// Run satisfies Kong command execution for the preview list command.
func (helpPreviewListCmd) Run() error { return nil }

// helpPreviewDoneCmd supplies a simple positional command for preview command lists.
type helpPreviewDoneCmd struct {
	ID string `arg:"" help:"Task ID"`
}

// Run satisfies Kong command execution for the preview done command.
func (helpPreviewDoneCmd) Run() error { return nil }
