package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFeatureGenerationValidatesConfiguration rejects unsupported builds and method collisions before publication.
func TestFeatureGenerationValidatesConfiguration(t *testing.T) {
	for _, test := range []struct {
		name   string
		values map[string]string
	}{
		{"unknown driver", map[string]string{"FEATURE_DRIVER": "hosted"}},
		{"method collision", map[string]string{"FEATURE_INSTANCES_DRIVER": "memory"}},
		{"unknown build driver", map[string]string{"FEATURE_SUPPORTED_DRIVERS": "memory,hosted"}},
		{"inactive child option", map[string]string{"FEATURE_CHECKOUT_DRIVER": "memory", "FEATURE_CHECKOUT_PATH": "wrong"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			if _, err := generateFeatureFiles(fixtureGenerationInput(root, test.values)); err == nil {
				t.Fatal("invalid feature contract accepted")
			}
			if _, err := os.Stat(filepath.Join(root, FeatureManagerPath)); !os.IsNotExist(err) {
				t.Fatal("invalid generation published output")
			}
		})
	}
}

// TestFeatureGenerationRejectsMalformedOwnership avoids generating a wider resource scope from invalid Project configuration.
func TestFeatureGenerationRejectsMalformedOwnership(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte("apps: [invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := generateFeatureFiles(fixtureGenerationInput(root, nil)); err == nil {
		t.Fatal("malformed App ownership accepted")
	}
}

// TestGeneratedFeatureManagerExercisesNativeLifecycle validates generated behavior against the released native API.
func TestGeneratedFeatureManagerExercisesNativeLifecycle(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte("render:\n  components: [cli, feature]\napps:\n  billing:\n    components: [cli, feature]\n  reports:\n    components: [cli]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	input := fixtureGenerationInput(root, map[string]string{"FEATURE_SUPPORTED_DRIVERS": "memory,file", "FEATURE_CHECKOUT_DRIVER": "file", "FEATURE_CHECKOUT_PATH": "checkout.json", "BILLING_FEATURE_BILLING_ONLY_DRIVER": "memory", "REPORTS_FEATURE_UNRELATED_DRIVER": "file"})
	if count, err := generateFeatureFiles(input); err != nil || count != 2 {
		t.Fatalf("generate: count=%d, err=%v", count, err)
	}
	if count, err := generateFeatureFiles(input); err != nil || count != 0 {
		t.Fatalf("regenerate drift: count=%d, err=%v", count, err)
	}
	accessors, err := os.ReadFile(filepath.Join(root, FeatureAccessorsPath))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(accessors), "BillingOnly()") || strings.Contains(string(accessors), "Unrelated()") {
		t.Fatalf("App ownership: %s", accessors)
	}
	writeFixtureGoMod(t, root, fixtureGoModSpec{module: "example.com/featurefixtures", required: []string{"github.com/goforj/feature", "github.com/goforj/env/v2", "github.com/fsnotify/fsnotify"}})
	if err := os.WriteFile(filepath.Join(root, "internal/features/manager_test.go"), []byte(featureManagerRuntimeTests), 0600); err != nil {
		t.Fatal(err)
	}
	runFixtureGoModTidy(t, root, nil)
	runFixtureGoTest(t, root, "./internal/features", "", map[string]string{"GOFLAGS": "-race"})
}

const featureManagerRuntimeTests = `package features

import (
    "context"
    "errors"
    "os"
    "path/filepath"
    "sync"
    "sync/atomic"
    "testing"
    "time"

    "github.com/fsnotify/fsnotify"
    "github.com/goforj/feature"
)

// configureFeatureFixture supplies safe native defaults for each isolated runtime probe.
func configureFeatureFixture(t *testing.T) string {
    t.Helper()
    path := filepath.Join(t.TempDir(), "flags.json")
    if err := os.WriteFile(path, []byte("{\"schema\":1,\"flags\":{\"checkout-v2\":{\"default\":true}}}"), 0600); err != nil { t.Fatal(err) }
    for key, value := range map[string]string{"FEATURE_DRIVER":"memory", "FEATURE_DOCUMENT":"{\"schema\":1,\"flags\":{}}", "FEATURE_CHECKOUT_DRIVER":"file", "FEATURE_CHECKOUT_PATH":path, "FEATURE_CHECKOUT_REFRESH_INTERVAL":"10ms", "FEATURE_INITIALIZATION_TIMEOUT":"5s", "FEATURE_REFRESH_INTERVAL":"30s"} { t.Setenv(key,value) }
    return path
}

// TestManagedFeatureMemoryWithoutDocument starts the default evaluator without an inline environment definition.
func TestManagedFeatureMemoryWithoutDocument(t *testing.T) {
    configureFeatureFixture(t)
    t.Setenv("FEATURE_DOCUMENT", "")
    if err := os.Unsetenv("FEATURE_DOCUMENT"); err != nil { t.Fatal(err) }
    manager, err := NewManager()
    if err != nil { t.Fatal(err) }
    defer manager.Close()
    snapshot, err := manager.Default().Snapshot()
    if err != nil { t.Fatal(err) }
    if snapshot.Enabled(context.Background(), "not-configured") { t.Fatal("empty memory evaluator enabled an undefined flag") }
    if err := manager.ReadinessChecks()[0].Check(context.Background()); err != nil { t.Fatal(err) }
}

// TestManagedFeatureSnapshotsAndHealth exercises valid and invalid refresh with real source files.
func TestManagedFeatureSnapshotsAndHealth(t *testing.T) {
    path := configureFeatureFixture(t)
    manager, err := NewManager()
    if err != nil { t.Fatal(err) }
    defer manager.Close()
    ctx := context.Background()
    snapshot, err := manager.Checkout().Snapshot()
    if err != nil { t.Fatal(err) }
    if !snapshot.Enabled(ctx,"checkout-v2") { t.Fatal("initial document was not loaded synchronously") }
    if err := os.WriteFile(path, []byte("invalid"), 0600); err != nil { t.Fatal(err) }
    if err := manager.Refresh(ctx); !errors.Is(err,feature.ErrInvalidDocument) { t.Fatalf("invalid refresh: %v",err) }
    if !manager.Checkout().Enabled(ctx,"checkout-v2") { t.Fatal("invalid refresh discarded valid snapshot") }
    if err := manager.ReadinessChecks()[1].Check(ctx); err != nil { t.Fatalf("retained configuration became unready: %v",err) }
    if !errors.Is(manager.Instances()[1].Status.LastError,feature.ErrInvalidDocument) { t.Fatal("refresh freshness failure was hidden") }
    if err := os.WriteFile(path, []byte("{\"schema\":1,\"flags\":{\"checkout-v2\":{\"default\":false}}}"), 0600); err != nil { t.Fatal(err) }
    if err := manager.Refresh(ctx); err != nil { t.Fatal(err) }
    if manager.Checkout().Enabled(ctx,"checkout-v2") || !snapshot.Enabled(ctx,"checkout-v2") { t.Fatal("captured and live revisions were mixed") }
    if err := manager.ReadinessChecks()[1].Check(ctx); err != nil { t.Fatal(err) }
    if len(manager.Instances()) != 2 || manager.Default() == manager.Checkout() { t.Fatal("named evaluator collapsed into default") }
    canceled, cancel := context.WithCancel(ctx); cancel()
    if err := manager.ReadinessChecks()[0].Check(canceled); !errors.Is(err,context.Canceled) { t.Fatal(err) }
}

// TestManagedFeatureStartupErrors keeps invalid deployment policy from reaching request execution.
func TestManagedFeatureStartupErrors(t *testing.T) {
    for _, test := range []struct{key,value string}{
        {"FEATURE_DRIVER","hosted"}, {"FEATURE_DOCUMENT","invalid"},
        {"FEATURE_CHECKOUT_PATH",""}, {"FEATURE_CHECKOUT_PATH","/does-not-exist/feature.json"},
        {"FEATURE_REFRESH_INTERVAL","bad"}, {"FEATURE_REFRESH_INTERVAL","1ms"},
        {"FEATURE_INITIALIZATION_TIMEOUT","bad"}, {"FEATURE_INITIALIZATION_TIMEOUT","0s"},
    } {
        t.Run(test.key+test.value,func(t *testing.T) {
            configureFeatureFixture(t); t.Setenv(test.key,test.value)
            if _,err := NewManager(); err == nil { t.Fatal("invalid startup accepted") }
        })
    }
    configureFeatureFixture(t)
    if _,err := NewManagerForApp("MISSING"); err == nil { t.Fatal("unknown App accepted") }
    if _,err := NewManagerForApp("REPORTS"); err == nil { t.Fatal("disabled App initialized") }
}

// TestManagedFeatureAppOwnership does not initialize resources registered only by another App.
func TestManagedFeatureAppOwnership(t *testing.T) {
    configureFeatureFixture(t)
    t.Setenv("BILLING_FEATURE_BILLING_ONLY_DOCUMENT","{\"schema\":1,\"flags\":{\"billing\":{\"default\":true}}}")
    billing, err := NewManagerForApp("BILLING")
    if err != nil { t.Fatal(err) }; defer billing.Close()
    if !billing.BillingOnly().Enabled(context.Background(),"billing") || len(billing.Instances()) != 3 { t.Fatal("App overlay not loaded") }
    base,err := NewManager(); if err != nil { t.Fatal(err) }; defer base.Close()
    if len(base.Instances()) != 2 { t.Fatal("named App resource leaked into default App") }
    defer func(){ if recover() == nil { t.Fatal("cross-App accessor silently accepted") } }()
    base.BillingOnly()
}

// TestManagedFeatureLifecycle joins workers even when startup callback context expires.
func TestManagedFeatureLifecycle(t *testing.T) {
    path := configureFeatureFixture(t)
    manager,err := NewManager(); if err != nil { t.Fatal(err) }
    canceled,cancel := context.WithCancel(context.Background()); cancel()
    if err := manager.Start(canceled); !errors.Is(err,context.Canceled) { t.Fatal("canceled startup accepted") }
    startup,finishStartup := context.WithCancel(context.Background())
    if err := manager.Start(startup); err != nil { t.Fatal(err) }
    if err := manager.Start(startup); err != nil { t.Fatal(err) }
    finishStartup()
    if err := os.WriteFile(path, []byte("{\"schema\":1,\"flags\":{}}"),0600); err != nil { t.Fatal(err) }
    deadline := time.After(3*time.Second)
    tick := time.NewTicker(5*time.Millisecond); defer tick.Stop()
    for manager.Checkout().Enabled(context.Background(),"checkout-v2") {
        select { case <-deadline: t.Fatal("refresh stopped with startup callback context"); case <-tick.C: }
    }
    var callers sync.WaitGroup
    for range 8 { callers.Add(1); go func(){ defer callers.Done(); _ = manager.Close() }() }
    callers.Wait()
    if err := manager.Start(context.Background()); err == nil { t.Fatal("closed manager restarted") }
    if err := manager.Refresh(context.Background()); err == nil { t.Fatal("closed manager refreshed") }
    if err := manager.ReadinessChecks()[0].Check(context.Background()); err == nil { t.Fatal("closed manager was ready") }
    neverStarted,err := NewManager(); if err != nil { t.Fatal(err) }
    if err := neverStarted.Close(); err != nil { t.Fatal(err) }
}

// TestUninitializedFeatureReadiness rejects unusable state without treating stale valid configuration as unavailable.
func TestUninitializedFeatureReadiness(t *testing.T) {
    manager := &Manager{resources: []managedEvaluator{{name:"default",client:&feature.Client{}}}}
    if err := manager.ReadinessChecks()[0].Check(context.Background()); err == nil { t.Fatal("uninitialized configuration was ready") }
}

// TestManagedFeatureInvalidInitialDocument keeps a malformed file from publishing an initial evaluator.
func TestManagedFeatureInvalidInitialDocument(t *testing.T) {
    path := configureFeatureFixture(t)
    if err := os.WriteFile(path, []byte("invalid"), 0600); err != nil { t.Fatal(err) }
    if _, err := NewManager(); !errors.Is(err, feature.ErrInvalidDocument) { t.Fatalf("invalid initial file: %v",err) }
}

// TestManagedFeatureWhitespaceDrivers preserves the generator's defaulting policy at runtime.
func TestManagedFeatureWhitespaceDrivers(t *testing.T) {
    configureFeatureFixture(t)
    for _, key := range []string{"FEATURE_DRIVER", "FEATURE_CHECKOUT_DRIVER", "BILLING_FEATURE_BILLING_ONLY_DRIVER"} { t.Setenv(key, " \t ") }
    for _, prefix := range []string{"", "BILLING"} {
        manager, err := NewManagerForApp(prefix)
        if err != nil { t.Fatal(err) }
        for _, instance := range manager.Instances() { if instance.Driver != "memory" { t.Fatalf("whitespace driver did not default: %#v", instance) } }
        if err := manager.Close(); err != nil { t.Fatal(err) }
    }
}

// waitFeatureCondition waits for asynchronous publication without depending on one backend's event timing.
func waitFeatureCondition(t *testing.T, condition func() bool) {
    t.Helper()
    deadline := time.NewTimer(3*time.Second); defer deadline.Stop()
    tick := time.NewTicker(5*time.Millisecond); defer tick.Stop()
    for !condition() { select { case <-deadline.C: t.Fatal("feature refresh did not converge"); case <-tick.C: } }
}

// writeFeatureDefinition replaces complete files so editor-style saves can be tested repeatedly.
func writeFeatureDefinition(t *testing.T, path string, enabled bool) {
    t.Helper()
    document := "{\"schema\":1,\"flags\":{\"checkout-v2\":{\"default\":false}}}"
    if enabled { document = "{\"schema\":1,\"flags\":{\"checkout-v2\":{\"default\":true}}}" }
    temp := path+".next"
    if err := os.WriteFile(temp, []byte(document),0600); err != nil { t.Fatal(err) }
    if err := os.Rename(temp,path); err != nil { t.Fatal(err) }
}

// TestManagedFeatureFileNotifications observes writes and repeated atomic replacement before the safety interval.
func TestManagedFeatureFileNotifications(t *testing.T) {
    path := configureFeatureFixture(t)
    t.Setenv("FEATURE_CHECKOUT_REFRESH_INTERVAL","1h")
    manager,err := NewManager(); if err != nil { t.Fatal(err) }; defer manager.Close()
    if err := manager.Start(context.Background()); err != nil { t.Fatal(err) }
    initial := manager.Instances()[1].Status.LastSuccess
    waitFeatureCondition(t,func() bool { return manager.Instances()[1].Status.LastSuccess.After(initial) })
    if err := os.WriteFile(path,[]byte("invalid"),0600); err != nil { t.Fatal(err) }
    waitFeatureCondition(t,func() bool { return errors.Is(manager.Instances()[1].Status.LastError,feature.ErrInvalidDocument) })
    if !manager.Checkout().Enabled(context.Background(),"checkout-v2") { t.Fatal("invalid event discarded valid state") }
    for _, enabled := range []bool{false,true,false} {
        writeFeatureDefinition(t,path,enabled)
        waitFeatureCondition(t,func() bool { return manager.Instances()[1].Status.LastError == nil && manager.Checkout().Enabled(context.Background(),"checkout-v2") == enabled })
    }
    if err := os.Remove(path); err != nil { t.Fatal(err) }
    waitFeatureCondition(t,func() bool { return errors.Is(manager.Instances()[1].Status.LastError,feature.ErrSource) })
    writeFeatureDefinition(t,path,true)
    waitFeatureCondition(t,func() bool { return manager.Instances()[1].Status.LastError == nil && manager.Checkout().Enabled(context.Background(),"checkout-v2") })
    if err := manager.Close(); err != nil { t.Fatal(err) }
    last := manager.Instances()[1].Status.LastAttempt
    writeFeatureDefinition(t,path,false)
    time.Sleep(100*time.Millisecond)
    if manager.Instances()[1].Status.LastAttempt != last { t.Fatal("refresh continued after shutdown") }
}

// TestFeatureFileRefreshFallback keeps refreshing when a parent cannot be watched.
func TestFeatureFileRefreshFallback(t *testing.T) {
    var loads atomic.Int64
    client,err := feature.Open(context.Background(),feature.SourceFunc(func(context.Context)(feature.Document,error) {
        loads.Add(1)
        return feature.Decode([]byte("{\"schema\":1,\"flags\":{}}"))
    }))
    if err != nil { t.Fatal(err) }
    resource := managedEvaluator{client:client,path:filepath.Join(t.TempDir(),"missing","flags.json"),interval:10*time.Millisecond}
    if watcher := watchFeatureDirectory(resource.path); watcher != nil { _ = watcher.Close(); t.Fatal("missing parent was watched") }
    ctx,cancel := context.WithCancel(context.Background()); defer cancel()
    done := make(chan struct{})
    go func(){ defer close(done); runFeatureFileRefresh(ctx,resource,nil) }()
    waitFeatureCondition(t,func() bool { return loads.Load() >= 4 })
    cancel()
    select { case <-done: case <-time.After(time.Second): t.Fatal("fallback worker did not stop") }
}

// TestFeatureFileWatcherFailure reconciles after notification failure and channel closure.
func TestFeatureFileWatcherFailure(t *testing.T) {
    for _, mode := range []string{"error","closed"} { t.Run(mode,func(t *testing.T) {
        path := configureFeatureFixture(t)
        manager,err := NewManager(); if err != nil { t.Fatal(err) }; defer manager.Close()
        resource := manager.resources[1]
        resource.interval = 20*time.Millisecond
        watcher := watchFeatureDirectory(path)
        if watcher == nil { t.Fatal("cannot watch test directory") }
        // Own the injected error channel so the backend cannot close it concurrently with a test send.
        if mode == "error" { watcher.Errors = make(chan error,1); watcher.Errors <- fsnotify.ErrEventOverflow }
        ctx,cancel := context.WithCancel(context.Background()); defer cancel()
        done := make(chan struct{})
        go func(){ defer close(done); runFeatureFileRefresh(ctx,resource,watcher) }()
        if mode == "closed" { _ = watcher.Close() }
        writeFeatureDefinition(t,path,false)
        waitFeatureCondition(t,func() bool { return !manager.Checkout().Enabled(context.Background(),"checkout-v2") })
        cancel()
        select { case <-done: case <-time.After(time.Second): t.Fatal("recovered worker did not stop") }
    }) }
}

// TestManagedFeatureMountedSymlink follows a projected directory swap without waiting for polling.
func TestManagedFeatureMountedSymlink(t *testing.T) {
    path := configureFeatureFixture(t)
    parent := filepath.Dir(path)
    for _, version := range []string{"one","two"} {
        if err := os.Mkdir(filepath.Join(parent,version),0700); err != nil { t.Fatal(err) }
        writeFeatureDefinition(t,filepath.Join(parent,version,"flags.json"),version=="one")
    }
    if err := os.Symlink("one",filepath.Join(parent,"..data")); err != nil { t.Skipf("symlinks unavailable: %v",err) }
    if err := os.Remove(path); err != nil { t.Fatal(err) }
    if err := os.Symlink(filepath.Join("..data","flags.json"),path); err != nil { t.Fatal(err) }
    t.Setenv("FEATURE_CHECKOUT_REFRESH_INTERVAL","1h")
    manager,err := NewManager(); if err != nil { t.Fatal(err) }; defer manager.Close()
    if err := manager.Start(context.Background()); err != nil { t.Fatal(err) }
    initial := manager.Instances()[1].Status.LastSuccess
    waitFeatureCondition(t,func() bool { return manager.Instances()[1].Status.LastSuccess.After(initial) })
    if err := os.Symlink("two",filepath.Join(parent,"..next")); err != nil { t.Fatal(err) }
    if err := os.Rename(filepath.Join(parent,"..next"),filepath.Join(parent,"..data")); err != nil { t.Fatal(err) }
    waitFeatureCondition(t,func() bool { return !manager.Checkout().Enabled(context.Background(),"checkout-v2") })
}

// TestManagedFeatureParentReplacement reacquires a directory watch after the watched directory is replaced.
func TestManagedFeatureParentReplacement(t *testing.T) {
    path := configureFeatureFixture(t)
    t.Setenv("FEATURE_CHECKOUT_REFRESH_INTERVAL","100ms")
    manager,err := NewManager(); if err != nil { t.Fatal(err) }; defer manager.Close()
    if err := manager.Start(context.Background()); err != nil { t.Fatal(err) }
    parent := filepath.Dir(path)
    if err := os.Rename(parent,parent+".old"); err != nil { t.Fatal(err) }
    t.Cleanup(func(){ _ = os.RemoveAll(parent+".old") })
    if err := os.Mkdir(parent,0700); err != nil { t.Fatal(err) }
    writeFeatureDefinition(t,path,false)
    waitFeatureCondition(t,func() bool { return !manager.Checkout().Enabled(context.Background(),"checkout-v2") })
    writeFeatureDefinition(t,path,true)
    waitFeatureCondition(t,func() bool { return manager.Checkout().Enabled(context.Background(),"checkout-v2") })
}

// TestFeatureNotificationDebounce coalesces event bursts and ignores unrelated writes and metadata noise.
func TestFeatureNotificationDebounce(t *testing.T) {
    var loads atomic.Int64
    client,err := feature.Open(context.Background(),feature.SourceFunc(func(context.Context)(feature.Document,error) {
        loads.Add(1)
        return feature.Decode([]byte("{\"schema\":1,\"flags\":{}}"))
    }))
    if err != nil { t.Fatal(err) }
    path := filepath.Join(t.TempDir(),"flags.json")
    watcher := watchFeatureDirectory(path)
    if watcher == nil { t.Fatal("cannot watch test directory") }
    // Own the event channel to exercise platform-independent debounce behavior without backend noise.
    events := make(chan fsnotify.Event,32)
    watcher.Events = events
    ctx,cancel := context.WithCancel(context.Background()); defer cancel()
    done := make(chan struct{})
    go func(){ defer close(done); runFeatureFileRefresh(ctx,managedEvaluator{client:client,path:path,interval:time.Hour},watcher) }()
    defer func(){ cancel(); <-done }()
    waitFeatureCondition(t,func() bool { return loads.Load()==2 })
    events <- fsnotify.Event{Name:path,Op:fsnotify.Chmod}
    events <- fsnotify.Event{Name:path+".other",Op:fsnotify.Write}
    time.Sleep(100*time.Millisecond)
    if loads.Load()!=2 { t.Fatal("unrelated events caused a refresh") }
    for range 20 { events <- fsnotify.Event{Name:path,Op:fsnotify.Write} }
    waitFeatureCondition(t,func() bool { return loads.Load()>=3 })
    time.Sleep(100*time.Millisecond)
    if loads.Load()!=3 { t.Fatalf("event burst caused %d loads",loads.Load()) }
}
`
