package atlaseval

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/goforj/goforj/internal/scenarios"
)

// TestPreparationCacheMaterializesDistinctPlansConcurrently keeps unrelated evaluations from queueing behind one global cache lock.
func TestPreparationCacheMaterializesDistinctPlansConcurrently(t *testing.T) {
	cache := newPreparationCache()
	started := make(chan string, 2)
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	done := make(chan error, 2)
	for _, key := range []string{"plan-a", "plan-b"} {
		go func(key string) {
			base, finish, err := cache.acquire(context.Background(), key, func() (*scenarios.PreparedScenario, error) {
				started <- key
				<-release
				return &scenarios.PreparedScenario{}, nil
			})
			if err == nil && base != nil {
				finish()
			}
			done <- err
		}(key)
	}
	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("distinct plans did not enter materialization concurrently")
		}
	}
	releaseOnce.Do(func() { close(release) })
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}

// TestPreparationCacheSingleFlightsIdenticalPlans keeps one immutable base shared by concurrent same-plan callers.
func TestPreparationCacheSingleFlightsIdenticalPlans(t *testing.T) {
	cache := newPreparationCache()
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var count int
	var lock sync.Mutex
	done := make(chan error, 2)
	for range 2 {
		go func() {
			base, finish, err := cache.acquire(context.Background(), "plan", func() (*scenarios.PreparedScenario, error) {
				lock.Lock()
				count++
				lock.Unlock()
				started <- struct{}{}
				<-release
				return &scenarios.PreparedScenario{}, nil
			})
			if err == nil && base != nil {
				finish()
			}
			done <- err
		}()
	}
	<-started
	waitForPreparationCacheActive(t, cache, 2)
	close(release)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	lock.Lock()
	defer lock.Unlock()
	if count != 1 {
		t.Fatalf("materializations = %d, want 1", count)
	}
}

// TestPreparationCacheCanceledWaiterReturnsBeforeMaterializationCompletes keeps canceled same-plan callers from cloning late.
func TestPreparationCacheCanceledWaiterReturnsBeforeMaterializationCompletes(t *testing.T) {
	cache := newPreparationCache()
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	ownerDone := make(chan error, 1)
	go func() {
		base, finish, err := cache.acquire(context.Background(), "plan", func() (*scenarios.PreparedScenario, error) {
			started <- struct{}{}
			<-release
			return &scenarios.PreparedScenario{}, nil
		})
		if err == nil && base != nil {
			finish()
		}
		ownerDone <- err
	}()
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	waiterDone := make(chan error, 1)
	go func() {
		_, _, err := cache.acquire(ctx, "plan", func() (*scenarios.PreparedScenario, error) {
			return nil, errors.New("waiter materialized a shared plan")
		})
		waiterDone <- err
	}()
	waitForPreparationCacheActive(t, cache, 2)
	cancel()
	if err := <-waiterDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("waiter error = %v, want context cancellation", err)
	}
	close(release)
	if err := <-ownerDone; err != nil {
		t.Fatal(err)
	}
}

// TestPreparationCacheCloseWaitsForInFlightMaterialization keeps cleanup from racing a running base builder.
func TestPreparationCacheCloseWaitsForInFlightMaterialization(t *testing.T) {
	cache := newPreparationCache()
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	ownerDone := make(chan error, 1)
	go func() {
		_, _, err := cache.acquire(context.Background(), "plan", func() (*scenarios.PreparedScenario, error) {
			started <- struct{}{}
			<-release
			return nil, errors.New("materialization failed")
		})
		ownerDone <- err
	}()
	<-started
	closed := make(chan error, 1)
	go func() { closed <- (&Preparer{cache: cache}).Close(context.Background()) }()
	waitForPreparationCacheClosed(t, cache)
	select {
	case err := <-closed:
		t.Fatalf("Close() returned before materialization completed: %v", err)
	default:
	}
	close(release)
	if err := <-ownerDone; err == nil {
		t.Fatal("owner materialization succeeded")
	}
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
}

// TestPreparerCloseCanRetryAfterCanceledWait keeps an interrupted shutdown from permanently abandoning retained bases.
func TestPreparerCloseCanRetryAfterCanceledWait(t *testing.T) {
	cache := newPreparationCache()
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	ownerDone := make(chan error, 1)
	go func() {
		_, _, err := cache.acquire(context.Background(), "plan", func() (*scenarios.PreparedScenario, error) {
			started <- struct{}{}
			<-release
			return nil, errors.New("materialization stopped")
		})
		ownerDone <- err
	}()
	<-started
	preparer := &Preparer{cache: cache}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := preparer.Close(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Close() error = %v, want context cancellation", err)
	}
	close(release)
	if err := <-ownerDone; err == nil {
		t.Fatal("owner materialization succeeded")
	}
	if err := preparer.Close(context.Background()); err != nil {
		t.Fatalf("retry Close(): %v", err)
	}
	select {
	case <-cache.cleanupDone:
	default:
		t.Fatal("retry Close() did not finish cleanup")
	}
}

// TestPreparerClosePreservesUnownedEnvironmentRoot keeps cleanup limited to paths claimed by NewPreparer.
func TestPreparerClosePreservesUnownedEnvironmentRoot(t *testing.T) {
	baseRoot := t.TempDir()
	sentinel := filepath.Join(baseRoot, ".environment", "sentinel")
	if err := os.MkdirAll(filepath.Dir(sentinel), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sentinel, []byte("owned elsewhere"), 0o600); err != nil {
		t.Fatal(err)
	}
	preparer := &Preparer{BaseRoot: baseRoot, cache: newPreparationCache()}
	if err := preparer.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if body, err := os.ReadFile(sentinel); err != nil || string(body) != "owned elsewhere" {
		t.Fatalf("unowned environment root changed: body=%q err=%v", body, err)
	}
}

// waitForPreparationCacheActive waits until every expected caller has entered the cache rather than relying on goroutine timing.
func waitForPreparationCacheActive(t *testing.T, cache *preparationCache, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		cache.mu.Lock()
		active := cache.active
		cache.mu.Unlock()
		if active == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("active cache users = %d, want %d", active, want)
		}
		runtime.Gosched()
	}
}

// waitForPreparationCacheClosed waits until Close has established the rejection boundary before checking that it remains blocked.
func waitForPreparationCacheClosed(t *testing.T, cache *preparationCache) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		cache.mu.Lock()
		closed := cache.closed
		cache.mu.Unlock()
		if closed {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("preparation cache did not enter the closed state")
		}
		runtime.Gosched()
	}
}
