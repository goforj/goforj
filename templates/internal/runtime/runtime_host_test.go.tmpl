package runtime

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRuntimeHostRunFailsWhenEmpty(t *testing.T) {
	host := NewRuntimeHost()

	err := host.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for empty runtime host")
	}
}

func TestRuntimeHostRunCancelsSiblingsOnFailure(t *testing.T) {
	errBoom := errors.New("boom")
	started := make(chan struct{}, 2)
	canceled := make(chan string, 1)

	host := NewRuntimeHost(
		stubRuntime{
			identity: RuntimeIdentity{Name: "http", Label: "http"},
			run: func(context.Context) error {
				started <- struct{}{}
				return errBoom
			},
		},
		stubRuntime{
			identity: RuntimeIdentity{Name: "jobs", Label: "jobs"},
			run: func(ctx context.Context) error {
				started <- struct{}{}
				<-ctx.Done()
				canceled <- "jobs"
				return ctx.Err()
			},
		},
	)

	err := host.Run(context.Background())
	if !errors.Is(err, errBoom) {
		t.Fatalf("expected host error to include %v, got %v", errBoom, err)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("expected both runtimes to start")
		}
	}
	select {
	case got := <-canceled:
		if got != "jobs" {
			t.Fatalf("expected jobs cancellation, got %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("expected sibling runtime to be canceled")
	}
}

func TestRuntimeHostRunReturnsNilOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	canceled := make(chan struct{}, 1)

	host := NewRuntimeHost(
		stubRuntime{
			identity: RuntimeIdentity{Name: "http", Label: "http"},
			run: func(ctx context.Context) error {
				<-ctx.Done()
				canceled <- struct{}{}
				return ctx.Err()
			},
		},
	)

	done := make(chan error, 1)
	go func() {
		done <- host.Run(ctx)
	}()
	cancel()

	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("expected runtime cancellation")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected nil host error on graceful cancellation, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("expected host run to complete")
	}
}

func TestWithSourceStoresNormalizedSourceName(t *testing.T) {
	ctx := WithSource(context.Background(), Source(" HTTP "))

	if got := SourceFromContext(ctx); got != SourceHTTP {
		t.Fatalf("expected source name %q, got %q", SourceHTTP, got)
	}
}

func TestNormalizeSourceSupportsLighthouseSource(t *testing.T) {
	if got := NormalizeSource(" LIGHTHOUSE "); got != SourceLighthouse {
		t.Fatalf("expected source name %q, got %q", SourceLighthouse, got)
	}
}

func TestSourceFromContextReturnsEmptyWhenUnset(t *testing.T) {
	if got := SourceFromContext(context.Background()); got != "" {
		t.Fatalf("expected empty source name, got %q", got)
	}
}

func TestRuntimeHostRuntimesReturnsCopy(t *testing.T) {
	host := NewRuntimeHost(stubRuntime{identity: RuntimeIdentity{Name: "http"}})

	runtimes := host.Runtimes()
	if len(runtimes) != 1 {
		t.Fatalf("expected one runtime, got %d", len(runtimes))
	}
	runtimes[0] = nil

	if got := host.Runtimes(); len(got) != 1 || got[0] == nil {
		t.Fatal("expected runtime host to retain original runtimes")
	}
}

func TestRuntimeHostFiltersNilRuntimes(t *testing.T) {
	host := NewRuntimeHost(nil, stubRuntime{identity: RuntimeIdentity{Name: "http"}})

	if got := len(host.Runtimes()); got != 1 {
		t.Fatalf("expected one runtime after filtering nils, got %d", got)
	}
}

type stubRuntime struct {
	identity RuntimeIdentity
	run      func(ctx context.Context) error
}

func (r stubRuntime) Identity() RuntimeIdentity {
	return r.identity
}

func (r stubRuntime) Run(ctx context.Context) error {
	if r.run == nil {
		return nil
	}
	return r.run(ctx)
}
