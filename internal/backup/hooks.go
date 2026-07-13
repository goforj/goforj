package backup

import "context"

// HookEvent identifies a lifecycle point around a backup operation.
type HookEvent string

const (
	// HookBeforeCreate runs before backup artifacts are created.
	HookBeforeCreate HookEvent = "before-create"
	// HookAfterCreate runs after a backup manifest has been written.
	HookAfterCreate HookEvent = "after-create"
	// HookBeforeRestore runs before destructive restore work begins.
	HookBeforeRestore HookEvent = "before-restore"
	// HookAfterRestore runs after restore work completes.
	HookAfterRestore HookEvent = "after-restore"
)

// Hook handles one optional backup lifecycle event.
type Hook func(context.Context, HookEvent) error

// HookRegistry stores explicit optional backup hooks.
type HookRegistry struct {
	BeforeCreate  []Hook
	AfterCreate   []Hook
	BeforeRestore []Hook
	AfterRestore  []Hook
}

// Run executes hooks for an event in registration order.
func (r HookRegistry) Run(ctx context.Context, event HookEvent) error {
	hooks := r.AfterCreate
	switch event {
	case HookBeforeCreate:
		hooks = r.BeforeCreate
	case HookBeforeRestore:
		hooks = r.BeforeRestore
	case HookAfterRestore:
		hooks = r.AfterRestore
	}
	for _, hook := range hooks {
		if err := hook(ctx, event); err != nil {
			return err
		}
	}
	return nil
}
