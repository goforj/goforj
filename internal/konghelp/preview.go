package konghelp

import (
	"bytes"
	"strings"

	"github.com/goforj/str/v2"

	"github.com/alecthomas/kong"
)

// Preview renders one of the real help formatters against an example Kong command tree.
func Preview(format string) string {
	format = str.Of(format).Trim().ToLower().String()
	parser, err := kong.New(
		previewCommandSurface(format),
		previewName(format),
		previewDescription(format),
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

// previewCommandSurface uses framework-shaped commands only for the framework formatter preview.
func previewCommandSurface(format string) interface{} {
	if format == formatFramework || format == "" {
		return &frameworkPreviewCLI{}
	}
	return &previewCLI{}
}

// previewName keeps each preview close to the kind of binary the formatter targets.
func previewName(format string) kong.Option {
	if format == formatFramework || format == "" {
		return kong.Name("app")
	}
	return kong.Name("tasks")
}

// previewDescription gives the preview enough context to show formatter-specific hierarchy.
func previewDescription(format string) kong.Option {
	if format == formatFramework || format == "" {
		return kong.Description("Application command surface")
	}
	return kong.Description("Track project tasks from the terminal")
}

// previewCLI models a small product CLI for external and guided formatter previews.
type previewCLI struct {
	Add  previewAddCmd  `cmd:"" help:"Add a task" group:"tasks"`
	List previewListCmd `cmd:"" help:"List open tasks" group:"tasks"`
	Done previewDoneCmd `cmd:"" help:"Mark a task complete" group:"tasks"`
}

// frameworkPreviewCLI models GoForj-style category commands for the framework preview.
type frameworkPreviewCLI struct {
	About         frameworkPreviewCommand `cmd:"" name:"about" help:"Show app environment and services"`
	MakeCommand   frameworkPreviewCommand `cmd:"" name:"make:command" help:"Create a new CLI command"`
	MakeMigration frameworkPreviewCommand `cmd:"" name:"make:migration" help:"Create a new migration"`
	CacheShell    frameworkPreviewCommand `cmd:"" name:"cache:shell" help:"Open a configured cache shell"`
	DBShell       frameworkPreviewCommand `cmd:"" name:"db:shell" help:"Open a configured database shell"`
	RedisShell    frameworkPreviewCommand `cmd:"" name:"redis:shell" help:"Open the configured Redis shell"`
	Migrate       frameworkPreviewCommand `cmd:"" name:"migrate" help:"Run database migrations"`
}

// frameworkPreviewCommand keeps framework preview commands inert while Kong builds the command tree.
type frameworkPreviewCommand struct{}

// Run satisfies Kong command execution for preview-only framework commands.
func (frameworkPreviewCommand) Run() error { return nil }

// Help provides root examples so guided previews can show the examples-first layout.
func (previewCLI) Help() string {
	return strings.TrimSpace(`
Examples:
  tasks add "Review PR" --tag code
  tasks list --all
  tasks done 42
`)
}

// previewAddCmd gives command-specific help enough arguments and flags to demonstrate alignment.
type previewAddCmd struct {
	Title string `arg:"" help:"Task title"`
	Due   string `help:"Due date"`
	Tag   string `short:"t" help:"Task tag"`
}

// Run satisfies Kong command execution for the preview add command.
func (previewAddCmd) Run() error { return nil }

// Help provides command examples so selected-command previews exercise example rendering.
func (previewAddCmd) Help() string {
	return strings.TrimSpace(`
Examples:
  tasks add "Review PR"
  tasks add "Ship release notes" --due tomorrow --tag docs
`)
}

// previewListCmd supplies a simple flag-only command for preview command lists.
type previewListCmd struct {
	All bool `short:"a" help:"Include completed tasks"`
}

// Run satisfies Kong command execution for the preview list command.
func (previewListCmd) Run() error { return nil }

// previewDoneCmd supplies a simple positional command for preview command lists.
type previewDoneCmd struct {
	ID string `arg:"" help:"Task ID"`
}

// Run satisfies Kong command execution for the preview done command.
func (previewDoneCmd) Run() error { return nil }
