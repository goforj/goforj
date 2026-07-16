package runtime

import (
	"context"
	"errors"
	"sync"
)

// Phase defines a lifecycle stage for hooks.
type Phase string

const (
	BeforeStartup  Phase = "before_startup"
	Startup        Phase = "startup"
	AfterStartup   Phase = "after_startup"
	BeforeShutdown Phase = "before_shutdown"
	Shutdown       Phase = "shutdown"
	AfterShutdown  Phase = "after_shutdown"
)

// Hook is a lifecycle callback.
type Hook func(ctx context.Context) error

// Lifecycle executes startup/shutdown hooks in a deterministic order.
type Lifecycle struct {
	mu       sync.Mutex
	hooks    map[Phase][]Hook
	started  bool
	timeouts *Timeouts
}

// NewLifecycle creates a lifecycle coordinator and panics when its required timeout policy is missing.
func NewLifecycle(timeouts *Timeouts) *Lifecycle {
	if timeouts == nil {
		panic("runtime.NewLifecycle requires non-nil timeouts")
	}

	return &Lifecycle{
		hooks:    make(map[Phase][]Hook),
		timeouts: timeouts,
	}
}

// Timeouts returns lifecycle timeout policy.
func (m *Lifecycle) Timeouts() *Timeouts {
	return m.timeouts
}

// On registers a hook for a phase.
func (m *Lifecycle) On(phase Phase, hook Hook) {
	if hook == nil {
		return
	}
	m.mu.Lock()
	m.hooks[phase] = append(m.hooks[phase], hook)
	m.mu.Unlock()
}

// Start runs startup phases once. Additional calls are no-op.
func (m *Lifecycle) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return nil
	}
	startHooks := m.cloneHooks([]Phase{BeforeStartup, Startup, AfterStartup})
	m.mu.Unlock()

	for _, hooks := range startHooks {
		for _, hook := range hooks {
			if err := hook(ctx); err != nil {
				return err
			}
		}
	}

	m.mu.Lock()
	m.started = true
	m.mu.Unlock()
	return nil
}

// Stop runs shutdown phases in reverse registration order. Additional calls are no-op.
func (m *Lifecycle) Stop(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return nil
	}
	stopHooks := m.cloneHooks([]Phase{BeforeShutdown, Shutdown, AfterShutdown})
	m.mu.Unlock()

	var joined error
	for _, hooks := range stopHooks {
		for i := len(hooks) - 1; i >= 0; i-- {
			if err := hooks[i](ctx); err != nil {
				joined = errors.Join(joined, err)
			}
		}
	}

	m.mu.Lock()
	m.started = false
	m.mu.Unlock()
	return joined
}

// cloneHooks isolates hook execution from registrations made while hooks are running.
func (m *Lifecycle) cloneHooks(phases []Phase) [][]Hook {
	cloned := make([][]Hook, 0, len(phases))
	for _, phase := range phases {
		hooks := m.hooks[phase]
		copied := make([]Hook, len(hooks))
		copy(copied, hooks)
		cloned = append(cloned, copied)
	}
	return cloned
}
