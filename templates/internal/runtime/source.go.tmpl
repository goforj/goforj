package runtime

import (
	"context"
	"os"
	"os/signal"

	"github.com/goforj/str/v2"
)

// Source identifies the current app-owned execution surface.
type Source string

const (
	SourceHTTP       Source = "http"
	SourceJobs       Source = "jobs"
	SourceScheduler  Source = "scheduler"
	SourceCLI        Source = "cli"
	SourceLighthouse Source = "lighthouse"
	SourceStartup    Source = "startup"
	SourceApp        Source = "app"
)

func (s Source) String() string {
	return string(s)
}

// NormalizeSource coerces a source string into its normalized form.
func NormalizeSource(value string) Source {
	switch str.Of(value).Trim().ToLower().String() {
	case string(SourceHTTP):
		return SourceHTTP
	case string(SourceJobs):
		return SourceJobs
	case string(SourceScheduler):
		return SourceScheduler
	case string(SourceCLI):
		return SourceCLI
	case string(SourceLighthouse):
		return SourceLighthouse
	case string(SourceStartup):
		return SourceStartup
	case string(SourceApp):
		return SourceApp
	default:
		return Source(str.Of(value).Trim().ToLower().String())
	}
}

type sourceContextKey struct{}

type sourceContextProvider interface {
	AppSource() Source
}

type sourceNameContextProvider interface {
	AppSourceName() string
}

// WithSource annotates a context with the logical source name.
func WithSource(ctx context.Context, source Source) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	source = NormalizeSource(source.String())
	if source == "" {
		return ctx
	}
	return context.WithValue(ctx, sourceContextKey{}, source)
}

// SourceFromContext extracts the logical source name from context.
func SourceFromContext(ctx context.Context) Source {
	if ctx == nil {
		return ""
	}
	if provider, ok := ctx.(sourceContextProvider); ok {
		return NormalizeSource(provider.AppSource().String())
	}
	if provider, ok := ctx.(sourceNameContextProvider); ok {
		return NormalizeSource(provider.AppSourceName())
	}
	source, _ := ctx.Value(sourceContextKey{}).(Source)
	return NormalizeSource(source.String())
}

// BackgroundSourceContext creates a background context tagged with the provided source.
func BackgroundSourceContext(source Source) context.Context {
	return WithSource(context.Background(), source)
}

// NotifyContextWithSource creates a signal-aware context tagged with the provided source.
func NotifyContextWithSource(parent context.Context, source Source, signals ...os.Signal) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return signal.NotifyContext(WithSource(parent, source), signals...)
}

// CLIContext creates a background CLI context tagged with the CLI source.
func CLIContext() context.Context {
	return BackgroundSourceContext(SourceCLI)
}

// CLINotifyContext creates a signal-aware CLI context.
func CLINotifyContext(parent context.Context, signals ...os.Signal) (context.Context, context.CancelFunc) {
	return NotifyContextWithSource(parent, SourceCLI, signals...)
}
