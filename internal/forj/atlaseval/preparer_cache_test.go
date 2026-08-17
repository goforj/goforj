package atlaseval

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/goforj/goforj/internal/scenarios"
)

// TestPreparationCacheCapacityFollowsEffectiveWorkers keeps filtered suites from retaining the eight-worker maximum.
func TestPreparationCacheCapacityFollowsEffectiveWorkers(t *testing.T) {
	for requested, want := range map[int]int{-1: 1, 1: 1, 4: 4, 8: 8, 20: 8} {
		if got := newPreparationCacheWithCapacity(requested).capacity; got != want {
			t.Fatalf("capacity(%d) = %d, want %d", requested, got, want)
		}
	}
}

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

// TestPreparationCacheRetriesWaiterAfterItsPublishedBaseIsEvicted keeps delayed same-plan callers from cloning a closed base.
func TestPreparationCacheRetriesWaiterAfterItsPublishedBaseIsEvicted(t *testing.T) {
	cache := newPreparationCache()
	first := &scenarios.PreparedScenario{}
	second := &scenarios.PreparedScenario{}
	paused := make(chan struct{}, 1)
	resume := make(chan struct{})
	cache.afterFlight = func() {
		paused <- struct{}{}
		<-resume
	}
	closed := make(chan *scenarios.PreparedScenario, 1)
	var closeOnce sync.Once
	cache.closeBase = func(base *scenarios.PreparedScenario) error {
		closeOnce.Do(func() { closed <- base })
		return nil
	}
	ownerStarted := make(chan struct{}, 1)
	allowOwner := make(chan struct{})

	owner := make(chan struct {
		base    *scenarios.PreparedScenario
		release func()
		err     error
	}, 1)
	go func() {
		base, release, err := cache.acquire(context.Background(), "plan", func() (*scenarios.PreparedScenario, error) {
			ownerStarted <- struct{}{}
			<-allowOwner
			return first, nil
		})
		owner <- struct {
			base    *scenarios.PreparedScenario
			release func()
			err     error
		}{base, release, err}
	}()
	<-ownerStarted

	waiter := make(chan struct {
		base    *scenarios.PreparedScenario
		release func()
		err     error
	}, 1)
	materializations := 0
	go func() {
		base, release, err := cache.acquire(context.Background(), "plan", func() (*scenarios.PreparedScenario, error) {
			materializations++
			return second, nil
		})
		waiter <- struct {
			base    *scenarios.PreparedScenario
			release func()
			err     error
		}{base, release, err}
	}()
	waitForPreparationCacheActive(t, cache, 2)
	close(allowOwner)
	ownerResult := <-owner
	if ownerResult.err != nil || ownerResult.base != first {
		t.Fatalf("owner acquire() = %v, %v", ownerResult.base, ownerResult.err)
	}
	<-paused
	ownerResult.release()
	for worker := range MaximumRetainedPreparationBases {
		key := fmt.Sprintf("other-%d", worker)
		_, release, err := cache.acquire(context.Background(), key, func() (*scenarios.PreparedScenario, error) {
			return &scenarios.PreparedScenario{}, nil
		})
		if err != nil {
			t.Fatalf("acquire(%q): %v", key, err)
		}
		release()
	}
	if base := <-closed; base != first {
		t.Fatalf("closed base = %p, want first base %p", base, first)
	}
	close(resume)
	waiterResult := <-waiter
	if waiterResult.err != nil || waiterResult.base != second {
		t.Fatalf("waiter acquire() = %v, %v", waiterResult.base, waiterResult.err)
	}
	if materializations != 1 {
		t.Fatalf("waiter materializations = %d, want 1", materializations)
	}
	waiterResult.release()
}

// TestPreparationCacheRetainsOneBasePerConcurrentPairWorker keeps every plan reusable after a fully occupied worker wave.
func TestPreparationCacheRetainsOneBasePerConcurrentPairWorker(t *testing.T) {
	cache := newPreparationCache()
	started := make(chan struct{}, MaximumRetainedPreparationBases)
	allowMaterialization := make(chan struct{})
	releases := make(chan func(), MaximumRetainedPreparationBases)
	completed := make(chan error, MaximumRetainedPreparationBases)
	var lock sync.Mutex
	materializations := map[string]int{}
	for worker := range MaximumRetainedPreparationBases {
		key := fmt.Sprintf("plan-%d", worker)
		go func() {
			_, release, err := cache.acquire(context.Background(), key, func() (*scenarios.PreparedScenario, error) {
				lock.Lock()
				materializations[key]++
				lock.Unlock()
				started <- struct{}{}
				<-allowMaterialization
				return &scenarios.PreparedScenario{}, nil
			})
			if err == nil {
				releases <- release
			}
			completed <- err
		}()
	}
	for range MaximumRetainedPreparationBases {
		<-started
	}
	close(allowMaterialization)
	for range MaximumRetainedPreparationBases {
		if err := <-completed; err != nil {
			t.Fatal(err)
		}
	}
	for range MaximumRetainedPreparationBases {
		(<-releases)()
	}
	for worker := range MaximumRetainedPreparationBases {
		key := fmt.Sprintf("plan-%d", worker)
		_, release, err := cache.acquire(context.Background(), key, func() (*scenarios.PreparedScenario, error) {
			lock.Lock()
			materializations[key]++
			lock.Unlock()
			return &scenarios.PreparedScenario{}, nil
		})
		if err != nil {
			t.Fatalf("reuse acquire(%q): %v", key, err)
		}
		release()
	}
	lock.Lock()
	defer lock.Unlock()
	for key, count := range materializations {
		if count != 1 {
			t.Fatalf("materializations for %q = %d, want 1", key, count)
		}
	}
}

// TestPreparationCacheBoundsIdleDistinctPlans preserves recent repeated-trial reuse without retaining every plan encountered by a suite.
func TestPreparationCacheBoundsIdleDistinctPlans(t *testing.T) {
	cache := newPreparationCache()
	materializations := 0
	acquire := func(key string) {
		base, release, err := cache.acquire(context.Background(), key, func() (*scenarios.PreparedScenario, error) {
			materializations++
			return &scenarios.PreparedScenario{}, nil
		})
		if err != nil || base == nil {
			t.Fatalf("acquire(%q) = %v, %v", key, base, err)
		}
		release()
	}
	for _, key := range []string{"plan-a", "plan-b", "plan-c", "plan-d", "plan-e", "plan-f", "plan-g", "plan-h", "plan-i"} {
		acquire(key)
	}
	cache.mu.Lock()
	retained := len(cache.bases)
	_, firstRetained := cache.bases["plan-a"]
	cache.mu.Unlock()
	if retained != MaximumRetainedPreparationBases || firstRetained {
		t.Fatalf("retained bases = %d, plan-a retained = %t", retained, firstRetained)
	}
	acquire("plan-b")
	if materializations != 9 {
		t.Fatalf("materializations = %d, want reuse of retained plan-b", materializations)
	}
	acquire("plan-a")
	if materializations != 10 {
		t.Fatalf("materializations = %d, want evicted plan-a rematerialized", materializations)
	}
}

// TestPreparationCacheEvictsAfterHeldBasesRelease keeps concurrent clones safe while returning to the retained-base bound promptly.
func TestPreparationCacheEvictsAfterHeldBasesRelease(t *testing.T) {
	cache := newPreparationCache()
	const workers = 4
	releases := make([]func(), 0, workers)
	for _, key := range []string{"plan-a", "plan-b", "plan-c", "plan-d"} {
		base, release, err := cache.acquire(context.Background(), key, func() (*scenarios.PreparedScenario, error) {
			return &scenarios.PreparedScenario{}, nil
		})
		if err != nil || base == nil {
			t.Fatalf("acquire(%q) = %v, %v", key, base, err)
		}
		releases = append(releases, release)
	}
	cache.mu.Lock()
	held := len(cache.bases)
	cache.mu.Unlock()
	if held != workers {
		t.Fatalf("held bases = %d, want all active worker bases retained", held)
	}
	for _, release := range releases {
		release()
	}
	cache.mu.Lock()
	retained := len(cache.bases)
	cache.mu.Unlock()
	if retained != workers {
		t.Fatalf("retained bases after worker releases = %d, want %d", retained, workers)
	}
}

// TestPreparationCacheEvictionFailurePoisonsLaterAcquisition prevents continued base growth after deletion makes cache accounting unreliable.
func TestPreparationCacheEvictionFailurePoisonsLaterAcquisition(t *testing.T) {
	cache := newPreparationCache()
	evictionErr := errors.New("remove evicted base")
	cache.closeBase = func(*scenarios.PreparedScenario) error { return evictionErr }
	acquire := func(key string) {
		t.Helper()
		base, release, err := cache.acquire(context.Background(), key, func() (*scenarios.PreparedScenario, error) {
			return &scenarios.PreparedScenario{}, nil
		})
		if err != nil || base == nil {
			t.Fatalf("acquire(%q) = %v, %v", key, base, err)
		}
		release()
	}
	for _, key := range []string{"plan-a", "plan-b", "plan-c", "plan-d", "plan-e", "plan-f", "plan-g", "plan-h"} {
		acquire(key)
	}

	materialized := false
	_, _, err := cache.acquire(context.Background(), "plan-i", func() (*scenarios.PreparedScenario, error) {
		materialized = true
		return &scenarios.PreparedScenario{}, nil
	})
	if !errors.Is(err, ErrPreparationCachePoisoned) || !errors.Is(err, evictionErr) {
		t.Fatalf("poisoning acquire() error = %v, want typed eviction failure", err)
	}
	if !materialized {
		t.Fatal("poisoning acquire did not reach the eviction boundary")
	}

	materialized = false
	_, _, err = cache.acquire(context.Background(), "plan-j", func() (*scenarios.PreparedScenario, error) {
		materialized = true
		return &scenarios.PreparedScenario{}, nil
	})
	if !errors.Is(err, ErrPreparationCachePoisoned) || !errors.Is(err, evictionErr) {
		t.Fatalf("later acquire() error = %v, want typed eviction failure", err)
	}
	if materialized {
		t.Fatal("later acquire materialized after eviction failure")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = cache.acquire(ctx, "plan-j", func() (*scenarios.PreparedScenario, error) {
		t.Fatal("canceled acquire materialized")
		return nil, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled acquire() error = %v, want context cancellation", err)
	}
	if err := (&Preparer{cache: cache}).Close(context.Background()); !errors.Is(err, evictionErr) {
		t.Fatalf("Close() error = %v, want eviction failure", err)
	}
}

// TestPreparationCacheWaitForEvictionReportsConcurrentPoison keeps a prepared clone from escaping while another release is deleting shared state.
func TestPreparationCacheWaitForEvictionReportsConcurrentPoison(t *testing.T) {
	cache := newPreparationCacheWithCapacity(1)
	started := make(chan struct{})
	allowFailure := make(chan struct{})
	evictionErr := errors.New("remove evicted base")
	cache.closeBase = func(*scenarios.PreparedScenario) error {
		close(started)
		<-allowFailure
		return evictionErr
	}
	acquire := func(key string) func() {
		t.Helper()
		_, release, err := cache.acquire(context.Background(), key, func() (*scenarios.PreparedScenario, error) {
			return &scenarios.PreparedScenario{}, nil
		})
		if err != nil {
			t.Fatalf("acquire(%q): %v", key, err)
		}
		return release
	}
	releaseA := acquire("plan-a")
	releaseB := acquire("plan-b")
	t.Cleanup(releaseB)
	go releaseA()
	<-started
	done := make(chan error, 1)
	go func() { done <- cache.waitForEviction(context.Background()) }()
	select {
	case err := <-done:
		t.Fatalf("waitForEviction returned before deletion completed: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(allowFailure)
	if err := <-done; !errors.Is(err, ErrPreparationCachePoisoned) || !errors.Is(err, evictionErr) {
		t.Fatalf("waitForEviction() error = %v, want typed eviction failure", err)
	}
}

// TestPreparationCacheReleasesPublishedBaseAfterCancellation prevents a post-materialization cancellation from pinning an otherwise idle base.
func TestPreparationCacheReleasesPublishedBaseAfterCancellation(t *testing.T) {
	cache := newPreparationCache()
	ctx, cancel := context.WithCancel(context.Background())
	_, _, err := cache.acquire(ctx, "plan", func() (*scenarios.PreparedScenario, error) {
		cancel()
		return &scenarios.PreparedScenario{}, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("acquire() error = %v, want context cancellation", err)
	}
	cache.mu.Lock()
	users := cache.baseUsers["plan"]
	cache.mu.Unlock()
	if users != 0 {
		t.Fatalf("published canceled base users = %d, want 0", users)
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
