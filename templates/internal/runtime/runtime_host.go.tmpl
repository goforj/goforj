package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// Runtime is a hostable logical runtime for the app.
type Runtime interface {
	Identity() RuntimeIdentity
	Run(ctx context.Context) error
}

// RuntimeHost runs logical runtimes together with coordinated shutdown.
type RuntimeHost struct {
	runtimes []Runtime
}

// NewRuntimeHost creates a runtime host for the provided runtimes.
func NewRuntimeHost(runtimes ...Runtime) *RuntimeHost {
	filtered := make([]Runtime, 0, len(runtimes))
	for _, runtime := range runtimes {
		if runtime == nil {
			continue
		}
		filtered = append(filtered, runtime)
	}
	return &RuntimeHost{runtimes: filtered}
}

// Runtimes returns a copy of the configured runtimes.
func (h *RuntimeHost) Runtimes() []Runtime {
	if h == nil || len(h.runtimes) == 0 {
		return nil
	}
	out := make([]Runtime, len(h.runtimes))
	copy(out, h.runtimes)
	return out
}

// Run starts all configured runtimes and cancels siblings on the first failure.
func (h *RuntimeHost) Run(ctx context.Context) error {
	if h == nil || len(h.runtimes) == 0 {
		return fmt.Errorf("runtime host: no runtimes configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		identity RuntimeIdentity
		err      error
	}

	results := make(chan result, len(h.runtimes))
	var wg sync.WaitGroup
	for _, runtime := range h.runtimes {
		runtime := runtime
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- result{
				identity: runtime.Identity(),
				err:      runtime.Run(runCtx),
			}
		}()
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	var firstErr error
	for result := range results {
		if result.err == nil {
			continue
		}
		if runCtx.Err() != nil && errors.Is(result.err, context.Canceled) {
			continue
		}
		if errors.Is(ctx.Err(), context.Canceled) && errors.Is(result.err, context.Canceled) {
			continue
		}
		if firstErr == nil {
			firstErr = fmt.Errorf("runtime host: %s failed: %w", runtimeName(result.identity), result.err)
			cancel()
		}
	}

	if firstErr != nil {
		return firstErr
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return nil
	}
	return ctx.Err()
}

func runtimeName(identity RuntimeIdentity) string {
	if name := strings.TrimSpace(identity.Name); name != "" {
		return name
	}
	if label := strings.TrimSpace(identity.Label); label != "" {
		return label
	}
	return "runtime"
}
