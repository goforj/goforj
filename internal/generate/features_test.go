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
		{"invalid accessor", map[string]string{"FEATURE_123_DRIVER": "memory"}},
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

// TestFeatureGenerationPreservesExistingArtifacts rejects invalid accessors before changing either output.
func TestFeatureGenerationPreservesExistingArtifacts(t *testing.T) {
	root := t.TempDir()
	input := fixtureGenerationInput(root, nil)
	if _, err := generateFeatureFiles(input); err != nil {
		t.Fatal(err)
	}
	originals := map[string]string{}
	for _, path := range []string{FeatureManagerPath, FeatureAccessorsPath} {
		contents, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		originals[path] = string(contents)
	}
	invalid := fixtureGenerationInput(root, map[string]string{"FEATURE_123_DRIVER": "memory", "FEATURE_SUPPORTED_DRIVERS": "memory"})
	if count, err := generateFeatureFiles(invalid); err == nil || count != 0 {
		t.Fatalf("invalid accessor: count=%d err=%v", count, err)
	}
	for path, original := range originals {
		contents, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		if string(contents) != original {
			t.Fatalf("invalid generation changed %s", path)
		}
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
	writeFixtureGoMod(t, root, fixtureGoModSpec{module: "example.com/featurefixtures", required: []string{"github.com/goforj/feature", "github.com/goforj/env/v2", "github.com/fsnotify/fsnotify", "golang.org/x/text"}})
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
    "runtime"
    "strings"
    "sync"
    "sync/atomic"
    "testing"
    "time"

    "github.com/fsnotify/fsnotify"
    "github.com/goforj/feature"
)

// featureFixtureDirectory removes platform temp-directory aliases so ordinary paths exercise healthy watches.
func featureFixtureDirectory(t *testing.T) string {
    t.Helper()
    path, err := filepath.EvalSymlinks(t.TempDir())
    if err != nil { t.Fatal(err) }
    return path
}

// featureSymlinkInterval retains event-only assertions where the backend supports reliable link replacement.
func featureSymlinkInterval(t *testing.T) {
    t.Helper()
    interval := "1h"
    if runtime.GOOS != "linux" && runtime.GOOS != "windows" { interval = "20ms" }
    t.Setenv("FEATURE_CHECKOUT_REFRESH_INTERVAL",interval)
}

// configureFeatureFixture supplies safe native defaults for each isolated runtime probe.
func configureFeatureFixture(t *testing.T) string {
    t.Helper()
    path := filepath.Join(featureFixtureDirectory(t), "flags.json")
    if err := os.WriteFile(path, []byte("{\"schema\":1,\"flags\":{\"checkout-v2\":{\"default\":true}}}"), 0600); err != nil { t.Fatal(err) }
    for key, value := range map[string]string{"FEATURE_DRIVER":"memory", "FEATURE_DOCUMENT":"{\"schema\":1,\"flags\":{}}", "FEATURE_CHECKOUT_DRIVER":"file", "FEATURE_CHECKOUT_PATH":path, "FEATURE_CHECKOUT_REFRESH_INTERVAL":"10ms", "FEATURE_INITIALIZATION_TIMEOUT":"5s", "FEATURE_REFRESH_INTERVAL":"30s"} { t.Setenv(key,value) }
    return path
}

// TestManagedFeatureMemoryWithoutDocument starts the default evaluator without an inline environment definition.
func TestManagedFeatureMemoryWithoutDocument(t *testing.T) {
    path := configureFeatureFixture(t)
    if err := os.Remove(path); err != nil { t.Fatal(err) }
    t.Setenv("FEATURE_CHECKOUT_DRIVER", "memory")
    t.Setenv("FEATURE_DOCUMENT", "")
    if err := os.Unsetenv("FEATURE_DOCUMENT"); err != nil { t.Fatal(err) }
    manager, err := NewManager()
    if err != nil { t.Fatal(err) }
    defer manager.Close()
    if err := manager.Start(context.Background()); err != nil { t.Fatal(err) }
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
    initial := manager.Instances()[1].Status.LastSuccess
    if err := manager.Start(context.Background()); err != nil { t.Fatal(err) }
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
    resource := managedEvaluator{client:client,path:filepath.Join(featureFixtureDirectory(t),"missing","flags.json"),interval:10*time.Millisecond}
    if watcher := watchFeatureDirectory(resource.path); watcher != nil { _ = watcher.Close(); t.Fatal("missing parent was watched") }
    ctx,cancel := context.WithCancel(context.Background()); defer cancel()
    done := make(chan struct{})
    go func(){ defer close(done); runFeatureFileRefresh(ctx,resource,nil) }()
    waitFeatureCondition(t,func() bool { return loads.Load() >= 4 })
    if err := os.Mkdir(filepath.Dir(resource.path),0700); err != nil { t.Fatal(err) }
    time.Sleep(100*time.Millisecond)
    recovered := loads.Load()
    time.Sleep(100*time.Millisecond)
    if loads.Load()!=recovered { t.Fatal("polling continued after watch recovery") }
    writeFeatureDefinition(t,resource.path,true)
    waitFeatureCondition(t,func() bool { return loads.Load()>recovered })
    cancel()
    select { case <-done: case <-time.After(time.Second): t.Fatal("fallback worker did not stop") }
}

// TestFeatureFileWatcherFailure reconciles after notification failure and channel closure.
func TestFeatureFileWatcherFailure(t *testing.T) {
    for _, mode := range []string{"error","closed","overflow"} { t.Run(mode,func(t *testing.T) {
        path := configureFeatureFixture(t)
        manager,err := NewManager(); if err != nil { t.Fatal(err) }; defer manager.Close()
        resource := manager.resources[1]
        resource.interval = 20*time.Millisecond
        watcher := watchFeatureDirectory(path)
        if watcher == nil { t.Fatal("cannot watch test directory") }
        // Own the injected error channel so the backend cannot close it concurrently with a test send.
        if mode == "error" || mode == "overflow" {
            watcher.Errors = make(chan error,1)
            if mode == "overflow" { resource.interval = time.Hour; watcher.Errors <- fsnotify.ErrEventOverflow } else { watcher.Errors <- errors.New("watch unavailable") }
        }
        ctx,cancel := context.WithCancel(context.Background()); defer cancel()
        done := make(chan struct{})
        initial := manager.Instances()[1].Status.LastSuccess
        go func(){ defer close(done); runFeatureFileRefresh(ctx,resource,watcher) }()
        if mode == "closed" { _ = watcher.Close() }
        waitFeatureCondition(t,func() bool { return manager.Instances()[1].Status.LastSuccess.After(initial) })
        writeFeatureDefinition(t,path,false)
        waitFeatureCondition(t,func() bool { return !manager.Checkout().Enabled(context.Background(),"checkout-v2") })
        if mode == "overflow" {
            writeFeatureDefinition(t,path,true)
            waitFeatureCondition(t,func() bool { return manager.Checkout().Enabled(context.Background(),"checkout-v2") })
        }
        cancel()
        select { case <-done: case <-time.After(time.Second): t.Fatal("recovered worker did not stop") }
    }) }
}

// TestManagedFeatureMountedSymlink follows a projected directory swap without waiting for polling.
func TestManagedFeatureMountedSymlink(t *testing.T) {
    path := configureFeatureFixture(t)
    parent := filepath.Dir(path)
    if err := os.Mkdir(filepath.Join(parent,"one"),0700); err != nil { t.Fatal(err) }
    writeFeatureDefinition(t,filepath.Join(parent,"one","flags.json"),true)
    if err := os.Symlink("one",filepath.Join(parent,"..data")); err != nil { t.Skipf("symlinks unavailable: %v",err) }
    if err := os.Remove(path); err != nil { t.Fatal(err) }
    if err := os.Symlink(filepath.Join("..data","flags.json"),path); err != nil { t.Fatal(err) }
    featureSymlinkInterval(t)
    manager,err := NewManager(); if err != nil { t.Fatal(err) }; defer manager.Close()
    initial := manager.Instances()[1].Status.LastSuccess
    if err := manager.Start(context.Background()); err != nil { t.Fatal(err) }
    waitFeatureCondition(t,func() bool { return manager.Instances()[1].Status.LastSuccess.After(initial) })
    // Projected mounts publish a new revision directory before replacing their data symlink.
    if err := os.Mkdir(filepath.Join(parent,"two"),0700); err != nil { t.Fatal(err) }
    writeFeatureDefinition(t,filepath.Join(parent,"two","flags.json"),false)
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

// TestFeatureWatchRegistrationOrder keeps ancestor controls active throughout descendant registration.
func TestFeatureWatchRegistrationOrder(t *testing.T) {
    path := configureFeatureFixture(t)
    ancestor := filepath.Join(filepath.Dir(path),"revision")
    parent := filepath.Join(ancestor,"nested")
    if err := os.MkdirAll(parent,0700); err != nil { t.Fatal(err) }
    path = filepath.Join(parent,"flags.json")
    writeFeatureDefinition(t,path,true)
    t.Setenv("FEATURE_CHECKOUT_PATH",path)
    manager,err := NewManager(); if err != nil { t.Fatal(err) }; defer manager.Close()
    directories,_,err := featureWatchTopology(path); if err != nil { t.Fatal(err) }
    for index,directory := range directories {
        if index == 0 { continue }
        previous := directories[index-1]
        if featureWatchDepth(previous) > featureWatchDepth(directory) || (featureWatchDepth(previous) == featureWatchDepth(directory) && previous >= directory) { t.Fatal("directory controls are not ordered from ancestors to descendants") }
    }
    registered := map[string]bool{}
    replaced := false
    watcher := registerFeatureDirectory(path,func(backend *fsnotify.Watcher,directory string) error {
        if directory != filepath.Dir(directory) && !registered[filepath.Dir(directory)] { t.Fatalf("parent control missing before %s",directory) }
        if err := backend.Add(directory); err != nil { return err }
        registered[directory] = true
        if directory == parent {
            replaced = true
            if err := os.Rename(ancestor,ancestor+".old"); err != nil { t.Fatal(err) }
            if err := os.MkdirAll(parent,0700); err != nil { t.Fatal(err) }
            writeFeatureDefinition(t,path,true)
        }
        return nil
    })
    if !replaced { t.Fatal("registration did not exercise ancestor replacement") }
    resource := manager.resources[1]; resource.interval = 20*time.Millisecond
    if watcher != nil { resource.interval = time.Hour }
    ctx,cancel := context.WithCancel(context.Background())
    done := make(chan struct{})
    initial := manager.Instances()[1].Status.LastSuccess
    go func(){ defer close(done); runFeatureFileRefresh(ctx,resource,watcher) }()
    defer func(){ cancel(); <-done }()
    waitFeatureCondition(t,func() bool { return manager.Instances()[1].Status.LastSuccess.After(initial) })
    for _,enabled := range []bool{false,true} {
        writeFeatureDefinition(t,path,enabled)
        waitFeatureCondition(t,func() bool { return manager.Checkout().Enabled(context.Background(),"checkout-v2") == enabled })
    }
}

// TestManagedFeatureAncestorReplacement rebinds unchanged watch paths after any ancestor is replaced.
func TestManagedFeatureAncestorReplacement(t *testing.T) {
    configureFeatureFixture(t)
    base := featureFixtureDirectory(t)
    ancestor := filepath.Join(base,"revision")
    parent := filepath.Join(ancestor,"nested")
    if err := os.MkdirAll(parent,0700); err != nil { t.Fatal(err) }
    path := filepath.Join(parent,"flags.json")
    writeFeatureDefinition(t,path,true)
    t.Setenv("FEATURE_CHECKOUT_PATH",path)
    t.Setenv("FEATURE_CHECKOUT_REFRESH_INTERVAL","1h")
    manager,err := NewManager(); if err != nil { t.Fatal(err) }; defer manager.Close()
    initial := manager.Instances()[1].Status.LastSuccess
    if err := manager.Start(context.Background()); err != nil { t.Fatal(err) }
    waitFeatureCondition(t,func() bool { return manager.Instances()[1].Status.LastSuccess.After(initial) })
    for _, directory := range []string{ancestor,parent} {
        if err := os.Rename(directory,directory+".old"); err != nil { t.Fatal(err) }
        if err := os.MkdirAll(parent,0700); err != nil { t.Fatal(err) }
        writeFeatureDefinition(t,path,false)
        waitFeatureCondition(t,func() bool { return !manager.Checkout().Enabled(context.Background(),"checkout-v2") })
        writeFeatureDefinition(t,path,true)
        waitFeatureCondition(t,func() bool { return manager.Checkout().Enabled(context.Background(),"checkout-v2") })
    }
}

// TestManagedFeaturePathCase follows native lookup semantics for files and replaceable ancestors.
func TestManagedFeaturePathCase(t *testing.T) {
    for _, mode := range []string{"file","ancestor"} { t.Run(mode,func(t *testing.T) {
        configureFeatureFixture(t)
        ancestor := filepath.Join(featureFixtureDirectory(t),"revision")
        parent := filepath.Join(ancestor,"nested")
        if err := os.MkdirAll(parent,0700); err != nil { t.Fatal(err) }
        path := filepath.Join(parent,"flags.json")
        writeFeatureDefinition(t,path,true)
        configured := filepath.Join(parent,"FLAGS.JSON")
        if mode == "ancestor" { configured = filepath.Join(filepath.Dir(ancestor),"REVISION","NESTED","FLAGS.JSON") }
        if _,err := os.Stat(configured); err != nil { t.Skip("case-sensitive filesystem") }
        t.Setenv("FEATURE_CHECKOUT_PATH",configured)
        t.Setenv("FEATURE_CHECKOUT_REFRESH_INTERVAL","1h")
        manager,err := NewManager(); if err != nil { t.Fatal(err) }; defer manager.Close()
        initial := manager.Instances()[1].Status.LastSuccess
        if err := manager.Start(context.Background()); err != nil { t.Fatal(err) }
        waitFeatureCondition(t,func() bool { return manager.Instances()[1].Status.LastSuccess.After(initial) })
        writeFeatureDefinition(t,path,false)
        waitFeatureCondition(t,func() bool { return !manager.Checkout().Enabled(context.Background(),"checkout-v2") })
        if err := os.Rename(ancestor,ancestor+".old"); err != nil { t.Fatal(err) }
        if err := os.MkdirAll(parent,0700); err != nil { t.Fatal(err) }
        writeFeatureDefinition(t,path,true)
        waitFeatureCondition(t,func() bool { return manager.Checkout().Enabled(context.Background(),"checkout-v2") })
        writeFeatureDefinition(t,path,false)
        waitFeatureCondition(t,func() bool { return !manager.Checkout().Enabled(context.Background(),"checkout-v2") })
        if err := os.Remove(path); err != nil { t.Fatal(err) }
        waitFeatureCondition(t,func() bool { return manager.Instances()[1].Status.LastError != nil })
        writeFeatureDefinition(t,path,true)
        waitFeatureCondition(t,func() bool { return manager.Instances()[1].Status.LastError == nil && manager.Checkout().Enabled(context.Background(),"checkout-v2") })
    }) }
}

// TestManagedFeatureUnicodeSpelling retains notifications for native normalization-equivalent file and ancestor names.
func TestManagedFeatureUnicodeSpelling(t *testing.T) {
    for _,mode := range []string{"file","ancestor"} { t.Run(mode,func(t *testing.T) {
        configureFeatureFixture(t)
        base := featureFixtureDirectory(t)
        parent := filepath.Join(base,"caf\u00e9")
        if err := os.Mkdir(parent,0700); err != nil { t.Fatal(err) }
        path := filepath.Join(parent,"caf\u00e9.json")
        writeFeatureDefinition(t,path,true)
        configured := filepath.Join(parent,"cafe\u0301.json")
        if mode == "ancestor" { configured = filepath.Join(base,"cafe\u0301","cafe\u0301.json") }
        if _,err := os.Stat(configured); err != nil { t.Skip("normalization-sensitive filesystem") }
        // An unrelated hardlink must not supply the canonical spelling of a replaceable entry.
        if err := os.Link(path,filepath.Join(parent,"aaa-unrelated.json")); err != nil { t.Fatal(err) }
        watcher := watchFeatureDirectory(configured)
        if watcher == nil { t.Fatal("native Unicode spelling disabled notification refresh") }
        _ = watcher.Close()
        t.Setenv("FEATURE_CHECKOUT_PATH",configured)
        t.Setenv("FEATURE_CHECKOUT_REFRESH_INTERVAL","1h")
        manager,err := NewManager(); if err != nil { t.Fatal(err) }; defer manager.Close()
        initial := manager.Instances()[1].Status.LastSuccess
        if err := manager.Start(context.Background()); err != nil { t.Fatal(err) }
        waitFeatureCondition(t,func() bool { return manager.Instances()[1].Status.LastSuccess.After(initial) })
        replacement := filepath.Join(parent,"replacement.json")
        writeFeatureDefinition(t,replacement,false)
        if err := os.Rename(replacement,path); err != nil { t.Fatal(err) }
        waitFeatureCondition(t,func() bool { return !manager.Checkout().Enabled(context.Background(),"checkout-v2") })
        if err := os.Remove(path); err != nil { t.Fatal(err) }
        waitFeatureCondition(t,func() bool { return manager.Instances()[1].Status.LastError != nil })
        writeFeatureDefinition(t,path,true)
        waitFeatureCondition(t,func() bool { return manager.Instances()[1].Status.LastError == nil && manager.Checkout().Enabled(context.Background(),"checkout-v2") })
    }) }
}

// TestFeatureNormalizationDistinctPaths preserves separate normalization variants on filesystems that distinguish them.
func TestFeatureNormalizationDistinctPaths(t *testing.T) {
    configureFeatureFixture(t)
    parent := featureFixtureDirectory(t)
    path,other := filepath.Join(parent,"caf\u00e9.json"),filepath.Join(parent,"cafe\u0301.json")
    if !featureWatchEquivalent(path,other) { t.Fatal("Unicode normalization candidates were not recognized") }
    writeFeatureDefinition(t,path,true)
    if _,err := os.Stat(other); err == nil { t.Skip("normalization-insensitive filesystem") }
    writeFeatureDefinition(t,other,false)
    _,controls,err := featureWatchTopology(path); if err != nil { t.Fatal(err) }
    if !controls[path] { t.Fatal("configured source missing from topology") }
    if _,ok := controls[other]; ok { t.Fatal("distinct normalization entry became a source control") }
    watcher := &featureWatcher{controls:controls}
    if _,relevant := watcher.featureWatchEvent(other); relevant { t.Fatal("distinct normalization event matched the source") }
}

// TestFeatureCaseDistinctPaths avoids confusing separate files on case-sensitive filesystems.
func TestFeatureCaseDistinctPaths(t *testing.T) {
    path := configureFeatureFixture(t)
    other := filepath.Join(filepath.Dir(path),strings.ToUpper(filepath.Base(path)))
    if _,err := os.Stat(other); err == nil { t.Skip("case-insensitive filesystem") }
    writeFeatureDefinition(t,other,false)
    directories,controls,err := featureWatchTopology(path); if err != nil { t.Fatal(err) }
    if len(directories) == 0 || !controls[path] { t.Fatal("configured source missing from topology") }
    if _,ok := controls[other]; ok { t.Fatal("distinct case-sensitive entry became a source control") }
    watcher := &featureWatcher{controls:controls}
    if _,relevant := watcher.featureWatchEvent(other); relevant { t.Fatal("distinct case-sensitive event matched the source") }
    actual,err := os.Lstat(path); if err != nil { t.Fatal(err) }
    canonical,err := featureWatchComponent(filepath.Dir(path),filepath.Base(path),actual)
    if err != nil || canonical != path { t.Fatalf("canonical source = %q, error = %v",canonical,err) }
}

// TestManagedFeaturePermissionRecovery retries failed sources when file or ancestor access is restored.
func TestManagedFeaturePermissionRecovery(t *testing.T) {
    if runtime.GOOS == "windows" { t.Skip("POSIX permission semantics required") }
    for _, mode := range []string{"file","parent","ancestor"} { t.Run(mode,func(t *testing.T) {
        configureFeatureFixture(t)
        ancestor := filepath.Join(featureFixtureDirectory(t),"revision")
        parent := filepath.Join(ancestor,"nested")
        if err := os.MkdirAll(parent,0700); err != nil { t.Fatal(err) }
        path := filepath.Join(parent,"flags.json")
        writeFeatureDefinition(t,path,true)
        t.Setenv("FEATURE_CHECKOUT_PATH",path)
        t.Setenv("FEATURE_CHECKOUT_REFRESH_INTERVAL","1h")
        manager,err := NewManager(); if err != nil { t.Fatal(err) }; defer manager.Close()
        initial := manager.Instances()[1].Status.LastSuccess
        if err := manager.Start(context.Background()); err != nil { t.Fatal(err) }
        waitFeatureCondition(t,func() bool { return manager.Instances()[1].Status.LastSuccess.After(initial) })
        file,err := os.OpenFile(path,os.O_WRONLY,0600); if err != nil { t.Fatal(err) }; defer file.Close()
        blocked, restored := path,os.FileMode(0600)
        if mode == "parent" { blocked,restored = parent,0700 }
        if mode == "ancestor" { blocked,restored = ancestor,0700 }
        defer os.Chmod(blocked,restored)
        if err := os.Chmod(blocked,0200); err != nil { t.Fatal(err) }
        if probe,err := os.Open(path); err == nil { _ = probe.Close(); t.Skip("process bypasses filesystem permissions") }
        if _,err := file.Write([]byte("{\"schema\":1,\"flags\":{\"checkout-v2\":{\"default\":false}}}")); err != nil { t.Fatal(err) }
        if err := file.Sync(); err != nil { t.Fatal(err) }
        waitFeatureCondition(t,func() bool { return manager.Instances()[1].Status.LastError != nil })
        if err := os.Chmod(blocked,restored); err != nil { t.Fatal(err) }
        waitFeatureCondition(t,func() bool { return manager.Instances()[1].Status.LastError == nil && !manager.Checkout().Enabled(context.Background(),"checkout-v2") })
        writeFeatureDefinition(t,path,true)
        waitFeatureCondition(t,func() bool { return manager.Checkout().Enabled(context.Background(),"checkout-v2") })
    }) }
}

// TestFeatureNotificationDebounce avoids polling healthy watches and coalesces events while ignoring metadata noise.
func TestFeatureNotificationDebounce(t *testing.T) {
    var loads atomic.Int64
    client,err := feature.Open(context.Background(),feature.SourceFunc(func(context.Context)(feature.Document,error) {
        loads.Add(1)
        return feature.Decode([]byte("{\"schema\":1,\"flags\":{}}"))
    }))
    if err != nil { t.Fatal(err) }
    path := filepath.Join(featureFixtureDirectory(t),"flags.json")
    watcher := watchFeatureDirectory(path)
    if watcher == nil { t.Fatal("cannot watch test directory") }
    // Own the event channel to exercise platform-independent debounce behavior without backend noise.
    events := make(chan fsnotify.Event,128)
    watcher.Events = events
    ctx,cancel := context.WithCancel(context.Background()); defer cancel()
    done := make(chan struct{})
    go func(){ defer close(done); runFeatureFileRefresh(ctx,managedEvaluator{client:client,path:path,interval:10*time.Millisecond},watcher) }()
    defer func(){ cancel(); <-done }()
    waitFeatureCondition(t,func() bool { return loads.Load()==2 })
    events <- fsnotify.Event{Name:path,Op:fsnotify.Chmod}
    events <- fsnotify.Event{Name:path+".other",Op:fsnotify.Write}
    for directory := filepath.Dir(path); ; directory = filepath.Dir(directory) {
        events <- fsnotify.Event{Name:filepath.Join(directory,"unrelated"),Op:fsnotify.Create|fsnotify.Remove}
        events <- fsnotify.Event{Name:directory,Op:fsnotify.Write|fsnotify.Chmod}
        if directory == filepath.Dir(directory) { break }
    }
    time.Sleep(100*time.Millisecond)
    if loads.Load()!=2 { t.Fatal("unrelated events caused a refresh") }
    for range 20 { events <- fsnotify.Event{Name:path,Op:fsnotify.Write} }
    waitFeatureCondition(t,func() bool { return loads.Load()>=3 })
    time.Sleep(100*time.Millisecond)
    if loads.Load()!=3 { t.Fatalf("event burst caused %d loads",loads.Load()) }
}
// TestFeatureNotificationChurn bounds refresh latency even while unrelated entries keep changing.
func TestFeatureNotificationChurn(t *testing.T) {
    path := configureFeatureFixture(t)
    t.Setenv("FEATURE_CHECKOUT_REFRESH_INTERVAL", "1h")
    manager, err := NewManager(); if err != nil { t.Fatal(err) }; defer manager.Close()
    initial := manager.Instances()[1].Status.LastSuccess
    if err := manager.Start(context.Background()); err != nil { t.Fatal(err) }
    waitFeatureCondition(t, func() bool { return manager.Instances()[1].Status.LastSuccess.After(initial) })
    stop, done := make(chan struct{}), make(chan struct{})
    go func() {
        defer close(done)
        tick := time.NewTicker(5*time.Millisecond); defer tick.Stop()
        noise := path+".noise"
        for { select {
        case <-stop: return
        case <-tick.C: _ = os.WriteFile(noise, []byte("noise"),0600); _ = os.Remove(noise)
        } }
    }()
    defer func(){ close(stop); <-done }()
    writeFeatureDefinition(t,path,false)
    waitFeatureCondition(t, func() bool { return !manager.Checkout().Enabled(context.Background(),"checkout-v2") })
}

// TestFeatureSymlinkTargets rebinds repeated parent swaps and follows direct external and nested targets.
func TestFeatureSymlinkTargets(t *testing.T) {
    for _, mode := range []string{"parent", "direct", "nested", "same-directory", "relative-parent"} { t.Run(mode, func(t *testing.T) {
        path := configureFeatureFixture(t)
        parent := filepath.Dir(path)
        targets := []string{filepath.Join(featureFixtureDirectory(t),"flags.json"), filepath.Join(featureFixtureDirectory(t),"flags.json"), filepath.Join(featureFixtureDirectory(t),"flags.json")}
        if mode == "same-directory" { targets = []string{filepath.Join(parent,"one.json"),filepath.Join(parent,"two.json"),filepath.Join(parent,"three.json")} }
        for _, target := range targets { writeFeatureDefinition(t,target,true) }
        if err := os.Remove(path); err != nil { t.Fatal(err) }
        control := path
        linkTarget := targets[0]
        if mode == "parent" {
            control = filepath.Join(parent,"current")
            path = filepath.Join(control,"flags.json")
            linkTarget = filepath.Dir(targets[0])
        } else if mode == "nested" {
            control = filepath.Join(featureFixtureDirectory(t),"intermediate")
            if err := os.Symlink(control,path); err != nil { t.Skipf("symlinks unavailable: %v",err) }
        }
        if mode == "relative-parent" {
            relative,err := filepath.Rel(filepath.Dir(control),linkTarget); if err != nil { t.Fatal(err) }; linkTarget = relative
        }
        if err := os.Symlink(linkTarget,control); err != nil { t.Skipf("symlinks unavailable: %v",err) }
        t.Setenv("FEATURE_CHECKOUT_PATH",path)
        featureSymlinkInterval(t)
        watcher := watchFeatureDirectory(path)
        supported := runtime.GOOS == "linux" || runtime.GOOS == "windows"
        if watcher != nil { _ = watcher.Close() }
        if (watcher != nil) != supported { t.Fatal("symlink watching availability does not match reliable backend coverage") }
        manager, err := NewManager(); if err != nil { t.Fatal(err) }; defer manager.Close()
        initial := manager.Instances()[1].Status.LastSuccess
        if err := manager.Start(context.Background()); err != nil { t.Fatal(err) }
        waitFeatureCondition(t,func() bool { return manager.Instances()[1].Status.LastSuccess.After(initial) })
        for i, enabled := range []bool{false,true} {
            target := targets[i+1]
            writeFeatureDefinition(t,target,enabled)
            destination := target
            if mode == "parent" { destination = filepath.Dir(target) }
            if mode == "relative-parent" {
                relative,err := filepath.Rel(filepath.Dir(control),destination); if err != nil { t.Fatal(err) }; destination = relative
            }
            if err := os.Symlink(destination,control+".next"); err != nil { t.Fatal(err) }
            if err := os.Rename(control+".next",control); err != nil { t.Fatal(err) }
            waitFeatureCondition(t,func() bool { return manager.Checkout().Enabled(context.Background(),"checkout-v2") == enabled })
        }
        if err := os.WriteFile(targets[2],[]byte("{\"schema\":1,\"flags\":{}}"),0600); err != nil { t.Fatal(err) }
        waitFeatureCondition(t,func() bool { return !manager.Checkout().Enabled(context.Background(),"checkout-v2") })
        writeFeatureDefinition(t,targets[2],true)
        waitFeatureCondition(t,func() bool { return manager.Checkout().Enabled(context.Background(),"checkout-v2") })
    }) }
}

// TestFeatureMemoryHasNoRefreshWorker keeps immutable sources outside the file refresh lifecycle.
func TestFeatureMemoryHasNoRefreshWorker(t *testing.T) {
    var loads atomic.Int64
    client, err := feature.Open(context.Background(),feature.SourceFunc(func(context.Context)(feature.Document,error) {
        loads.Add(1)
        return feature.Decode([]byte("{\"schema\":1,\"flags\":{}}"))
    }))
    if err != nil { t.Fatal(err) }
    manager := &Manager{resources: []managedEvaluator{{driver:"memory", client:client, interval:10*time.Millisecond}}}
    defer manager.Close()
    if err := manager.Start(context.Background()); err != nil { t.Fatal(err) }
    time.Sleep(100*time.Millisecond)
    if loads.Load() != 1 { t.Fatal("memory source started a refresh worker") }
}

// TestFeatureUnresolvableSymlinks keeps incomplete or cyclic topology from masquerading as healthy watching.
func TestFeatureUnresolvableSymlinks(t *testing.T) {
    parent := featureFixtureDirectory(t)
    link := filepath.Join(parent,"current")
    if err := os.Symlink("current",link); err != nil { t.Skipf("symlinks unavailable: %v",err) }
    if watcher := watchFeatureDirectory(filepath.Join(link,"flags.json")); watcher != nil { _ = watcher.Close(); t.Fatal("cyclic path was watched") }
    if err := os.Remove(link); err != nil { t.Fatal(err) }
    if err := os.Symlink("missing",link); err != nil { t.Fatal(err) }
    if watcher := watchFeatureDirectory(filepath.Join(link,"flags.json")); watcher != nil { _ = watcher.Close(); t.Fatal("missing symlink parent was watched") }
}

// TestFeatureOverflowRebindsSymlinkTargets recovers directory topology when the swap notification was lost.
func TestFeatureOverflowRebindsSymlinkTargets(t *testing.T) {
    if runtime.GOOS != "linux" && runtime.GOOS != "windows" { t.Skip("symlink notification coverage requires fallback on this backend") }
    path := configureFeatureFixture(t)
    first, second := filepath.Join(featureFixtureDirectory(t),"flags.json"), filepath.Join(featureFixtureDirectory(t),"flags.json")
    writeFeatureDefinition(t,first,true)
    writeFeatureDefinition(t,second,false)
    if err := os.Remove(path); err != nil { t.Fatal(err) }
    if err := os.Symlink(first,path); err != nil { t.Skipf("symlinks unavailable: %v",err) }
    manager,err := NewManager(); if err != nil { t.Fatal(err) }; defer manager.Close()
    resource := manager.resources[1]; resource.interval = time.Hour
    watcher := watchFeatureDirectory(path)
    if watcher == nil { t.Fatal("cannot watch test symlink") }
    // Suppress swap notifications to reproduce an overflow that lost the topology change.
    watcher.Events = make(chan fsnotify.Event)
    watcher.Errors = make(chan error,1)
    ctx,cancel := context.WithCancel(context.Background())
    done := make(chan struct{})
    initial := manager.Instances()[1].Status.LastSuccess
    go func(){ defer close(done); runFeatureFileRefresh(ctx,resource,watcher) }()
    defer func(){ cancel(); <-done }()
    waitFeatureCondition(t,func() bool { return manager.Instances()[1].Status.LastSuccess.After(initial) })
    if err := os.Symlink(second,path+".next"); err != nil { t.Fatal(err) }
    if err := os.Rename(path+".next",path); err != nil { t.Fatal(err) }
    watcher.Errors <- fsnotify.ErrEventOverflow
    waitFeatureCondition(t,func() bool { return !manager.Checkout().Enabled(ctx,"checkout-v2") })
    writeFeatureDefinition(t,second,true)
    waitFeatureCondition(t,func() bool { return manager.Checkout().Enabled(ctx,"checkout-v2") })
}

`
