package runtime

import (
	"context"
)

// RuntimeIdentity describes a logical source hosted by the app.
type RuntimeIdentity struct {
	Name  string // http, scheduler, jobs
	Label string // http, scheduler, jobs
}

// RuntimeFunc adapts a runtime identity and function into a Runtime.
type RuntimeFunc struct {
	RuntimeIdentity RuntimeIdentity
	RunFn           func(ctx context.Context) error
}

// Identity returns the logical runtime identity.
func (r RuntimeFunc) Identity() RuntimeIdentity {
	return r.RuntimeIdentity
}

// Run executes the adapted runtime function.
func (r RuntimeFunc) Run(ctx context.Context) error {
	if r.RunFn == nil {
		return nil
	}
	return r.RunFn(ctx)
}
