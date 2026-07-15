package runtime

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestLifecycle_StartOrder(t *testing.T) {
	manager := NewLifecycle(NewTimeouts())
	var calls []string

	manager.On(BeforeStartup, func(context.Context) error {
		calls = append(calls, "before")
		return nil
	})
	manager.On(Startup, func(context.Context) error {
		calls = append(calls, "startup")
		return nil
	})
	manager.On(AfterStartup, func(context.Context) error {
		calls = append(calls, "after")
		return nil
	})

	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	want := []string{"before", "startup", "after"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("unexpected hook order: got %v want %v", calls, want)
	}
}

func TestLifecycle_StartIsIdempotent(t *testing.T) {
	manager := NewLifecycle(NewTimeouts())
	calls := 0
	manager.On(Startup, func(context.Context) error {
		calls++
		return nil
	})

	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("first start failed: %v", err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("second start failed: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 startup call, got %d", calls)
	}
}

func TestLifecycle_StopOrderReversedWithinPhase(t *testing.T) {
	manager := NewLifecycle(NewTimeouts())
	var calls []string

	manager.On(Shutdown, func(context.Context) error {
		calls = append(calls, "first")
		return nil
	})
	manager.On(Shutdown, func(context.Context) error {
		calls = append(calls, "second")
		return nil
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	if err := manager.Stop(context.Background()); err != nil {
		t.Fatalf("stop failed: %v", err)
	}

	want := []string{"second", "first"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("unexpected shutdown order: got %v want %v", calls, want)
	}
}

func TestLifecycle_StopAggregatesErrors(t *testing.T) {
	manager := NewLifecycle(NewTimeouts())
	errA := errors.New("a")
	errB := errors.New("b")

	manager.On(Shutdown, func(context.Context) error { return errA })
	manager.On(Shutdown, func(context.Context) error { return errB })
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	err := manager.Stop(context.Background())
	if err == nil {
		t.Fatal("expected aggregated error")
	}
	if !errors.Is(err, errA) {
		t.Fatalf("expected aggregated error to include errA, got %v", err)
	}
	if !errors.Is(err, errB) {
		t.Fatalf("expected aggregated error to include errB, got %v", err)
	}
}

func TestLifecycle_StopWithoutStartIsNoop(t *testing.T) {
	manager := NewLifecycle(NewTimeouts())
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatalf("expected noop stop, got %v", err)
	}
}

// TestNewLifecycle_RequiresTimeouts verifies invalid wiring fails at construction rather than during shutdown.
func TestNewLifecycle_RequiresTimeouts(t *testing.T) {
	defer func() {
		got := recover()
		want := "runtime.NewLifecycle requires non-nil timeouts"
		if got != want {
			t.Fatalf("unexpected panic: got %v want %q", got, want)
		}
	}()

	NewLifecycle(nil)
}
