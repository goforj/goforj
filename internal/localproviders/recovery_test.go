package localproviders

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// recoveryFixture isolates the original credential-bearing switch without unrelated App topology.
func recoveryFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	put(t, root, ".goforj.yml", "project_name: recovery\nmodule_name: example.org/recovery\nrender:\n  components: [database_mysql,cache]\n")
	put(t, root, ".env", "DB_DRIVER=mysql\nDB_DSN=synthetic-recovery-secret\nDB_SUPPORTED_DRIVERS=mysql\nCACHE_DRIVER=redis\nCACHE_SUPPORTED_DRIVERS=memory,redis\nCOMPOSE_PROFILES=mysql,redis,custom\nOWNER_PRIVATE=unchanged-owner-secret\n")
	return root
}

// recoveryPlan uses the same production paths for first switches, repeated selections, and restoration.
func recoveryPlan(t *testing.T, s *Session, operation string) map[string]string {
	t.Helper()
	var values map[string]string
	var err error
	switch operation {
	case "first":
		values, err = s.Portable()
	case "repeat":
		values, err = s.Drivers(map[string]string{"DB_DRIVER": "mysql"})
	case "restore":
		values, err = s.Previous()
	default:
		t.Fatal("unknown recovery operation")
	}
	if err != nil {
		t.Fatal(err)
	}
	return values
}

// runRecoveryOperation drives the actual public API after fault injection is installed at the filesystem boundary.
func runRecoveryOperation(t *testing.T, s *Session, operation string) error {
	t.Helper()
	if operation == "restore" {
		return s.Restore()
	}
	return s.Apply(recoveryPlan(t, s, operation))
}

// TestRecoveryInterruptionProcess exits without running defers at a selected real publication boundary.
func TestRecoveryInterruptionProcess(t *testing.T) {
	root := os.Getenv("FORJ_RECOVERY_TEST_ROOT")
	if root == "" {
		return
	}
	operation, boundary := os.Getenv("FORJ_RECOVERY_TEST_OPERATION"), os.Getenv("FORJ_RECOVERY_TEST_BOUNDARY")
	s := openTest(t, root)
	backups := 0
	s.rename = func(from, to string) error {
		phase := ""
		switch filepath.Base(to) {
		case ".gitignore":
			phase = "ignore"
		case stateName:
			backups++
			phase = "pending"
			if backups == 2 {
				phase = "final"
			}
		case ".env":
			phase = "env"
		}
		if boundary == "before-"+phase {
			os.Exit(73)
		}
		err := os.Rename(from, to)
		if err == nil && boundary == "after-"+phase {
			os.Exit(73)
		}
		return err
	}
	if err := runRecoveryOperation(t, s, operation); err != nil {
		t.Fatal(err)
	}
	t.Fatal("interruption boundary was not reached")
}

// TestRecoverySurvivesEveryPublicationBoundary checks old-or-new state, credential retention, retry, and private-file protection after hard exits.
func TestRecoverySurvivesEveryPublicationBoundary(t *testing.T) {
	for _, operation := range []string{"first", "repeat", "restore"} {
		boundaries := []string{"before-pending", "after-pending", "before-env", "after-env", "before-final", "after-final"}
		if operation == "first" {
			boundaries = append([]string{"before-ignore", "after-ignore"}, boundaries...)
		}
		for _, boundary := range boundaries {
			t.Run(operation+"/"+boundary, func(t *testing.T) {
				root := recoveryFixture(t)
				runLocalGit(t, root, "init", "-q")
				original := clone(openTest(t, root).Current)
				if operation != "first" {
					s := openTest(t, root)
					if err := s.Apply(recoveryPlan(t, s, "first")); err != nil {
						t.Fatal(err)
					}
				}
				s := openTest(t, root)
				before := clone(s.Current)
				after := recoveryPlan(t, s, operation)
				command := exec.Command(os.Args[0], "-test.run=^TestRecoveryInterruptionProcess$", "-test.count=1")
				command.Env = append(os.Environ(), "FORJ_RECOVERY_TEST_ROOT="+root, "FORJ_RECOVERY_TEST_OPERATION="+operation, "FORJ_RECOVERY_TEST_BOUNDARY="+boundary)
				output, err := command.CombinedOutput()
				var failure *exec.ExitError
				if !errors.As(err, &failure) || failure.ExitCode() != 73 {
					t.Fatalf("expected hard exit at %s: %v\n%s", boundary, err, output)
				}
				active, err := os.ReadFile(filepath.Join(root, ".env"))
				if err != nil {
					t.Fatal(err)
				}
				backup, err := os.ReadFile(filepath.Join(root, stateName))
				if err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
				if !bytes.Contains(active, []byte("synthetic-recovery-secret")) && !bytes.Contains(backup, []byte("synthetic-recovery-secret")) {
					t.Fatal("interruption lost the saved service credential")
				}
				if bytes.Contains(backup, []byte("unchanged-owner-secret")) {
					t.Fatal("recovery copied unrelated credentials")
				}
				recovered := openTest(t, root)
				published := boundary == "after-env" || boundary == "before-final" || boundary == "after-final"
				expected := before
				if published {
					expected = after
				}
				if !reflect.DeepEqual(recovered.Current, expected) {
					t.Fatal("reconciliation selected the wrong publication boundary")
				}
				wantPrevious := operation != "first" || published
				if operation == "restore" && boundary == "after-final" {
					wantPrevious = false
				}
				if recovered.HasPrevious() != wantPrevious {
					t.Fatal("reconciliation selected the wrong restore point")
				}
				if wantPrevious {
					previous, err := recovered.Previous()
					if err != nil || !reflect.DeepEqual(previous, original) {
						t.Fatalf("reconciled restore point does not match original settings: %v", err)
					}
				}
				if _, err := os.Stat(filepath.Join(root, ".gitignore")); err == nil {
					assertLocalSecretUntracked(t, root, "synthetic-recovery-secret")
					leftovers, _ := filepath.Glob(filepath.Join(root, ".env.local-providers.tmp-*"))
					for _, name := range leftovers {
						assertFileMode(t, root, filepath.Base(name), 0600)
					}
				}
				if _, err := os.Stat(filepath.Join(root, lockName)); err != nil {
					t.Fatal("hard exit did not retain the exclusive lock")
				}
				// The child has exited, so removing this stale lock models the documented manual recovery step.
				if err := os.Remove(filepath.Join(root, lockName)); err != nil {
					t.Fatal(err)
				}
				active = append(active, []byte("OWNER_LATER=keep\n")...)
				put(t, root, ".env", string(active))
				recovered = openTest(t, root)
				if recovered.HasPrevious() {
					if err := recovered.Restore(); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := recovered.Apply(recoveryPlan(t, recovered, "first")); err != nil {
						t.Fatal(err)
					}
					if err := openTest(t, root).Restore(); err != nil {
						t.Fatal(err)
					}
				}
				final := openTest(t, root)
				if !reflect.DeepEqual(final.Current, original) || final.HasPrevious() {
					t.Fatal("recovery did not return to the original settings")
				}
				active, err = os.ReadFile(filepath.Join(root, ".env"))
				if err != nil || !bytes.Contains(active, []byte("OWNER_LATER=keep")) {
					t.Fatal("recovery lost the unrelated later edit")
				}
				backup, err = os.ReadFile(filepath.Join(root, stateName))
				if err != nil || bytes.Contains(backup, []byte("synthetic-recovery-secret")) {
					t.Fatal("successful restoration retained private recovery values")
				}
			})
		}
	}
}

// TestFinalizationFailureRetainsRecoverableCommit keeps the pending record after the active environment has already changed.
func TestFinalizationFailureRetainsRecoverableCommit(t *testing.T) {
	for _, operation := range []string{"first", "repeat", "restore"} {
		t.Run(operation, func(t *testing.T) {
			root := recoveryFixture(t)
			original := clone(openTest(t, root).Current)
			if operation != "first" {
				s := openTest(t, root)
				if err := s.Apply(recoveryPlan(t, s, "first")); err != nil {
					t.Fatal(err)
				}
			}
			s := openTest(t, root)
			after := recoveryPlan(t, s, operation)
			backups := 0
			s.rename = func(from, to string) error {
				if filepath.Base(to) == stateName {
					backups++
					if backups == 2 {
						return errors.New("injected cleanup failure")
					}
				}
				return os.Rename(from, to)
			}
			if err := runRecoveryOperation(t, s, operation); err == nil || !strings.Contains(err.Error(), "settings were updated") {
				t.Fatalf("expected committed cleanup failure, got %v", err)
			}
			recovered := openTest(t, root)
			if !reflect.DeepEqual(recovered.Current, after) || !recovered.HasPrevious() {
				t.Fatal("cleanup failure lost the active state or restore action")
			}
			if err := recovered.Restore(); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(openTest(t, root).Current, original) {
				t.Fatal("cleanup failure could not recover original settings")
			}
		})
	}
}

// TestPendingStateRejectsMixedAndInvalidChanges preserves evidence when no complete publication state matches.
func TestPendingStateRejectsMixedAndInvalidChanges(t *testing.T) {
	for _, scenario := range []string{"empty", "invalid-key", "mixed", "absent-empty"} {
		t.Run(scenario, func(t *testing.T) {
			root := recoveryFixture(t)
			pending := &pendingChange{Before: map[string]string{"DB_DRIVER": "mysql", "CACHE_DRIVER": "redis"}, After: map[string]string{"DB_DRIVER": "sqlite", "CACHE_DRIVER": "memory"}}
			switch scenario {
			case "empty":
				pending.After = clone(pending.Before)
			case "invalid-key":
				pending.After["APP_KEY"] = "synthetic-invalid-secret"
			case "mixed":
				put(t, root, ".env", "DB_DRIVER=sqlite\nCACHE_DRIVER=redis\n")
			case "absent-empty":
				pending.Before = map[string]string{}
				pending.After = map[string]string{"DB_DSN": ""}
				put(t, root, ".env", "DB_DSN=unexpected\n")
			}
			data, err := encodeState(state{Version: 2, Pending: pending})
			if err != nil {
				t.Fatal(err)
			}
			put(t, root, stateName, string(data))
			if _, err := Open(root); err == nil || strings.Contains(err.Error(), "synthetic-invalid-secret") {
				t.Fatalf("accepted unsafe pending state or exposed a value: %v", err)
			}
			actual, err := os.ReadFile(filepath.Join(root, stateName))
			if err != nil || !bytes.Equal(data, actual) {
				t.Fatal("failed reconciliation changed the backup")
			}
		})
	}
}

// TestPendingStateDistinguishesAbsentAndEmpty selects the correct side without treating inheritance as an explicit empty value.
func TestPendingStateDistinguishesAbsentAndEmpty(t *testing.T) {
	for _, present := range []bool{false, true} {
		t.Run(fmt.Sprint(present), func(t *testing.T) {
			root := recoveryFixture(t)
			s := openTest(t, root)
			delete(s.Current, "DB_DSN")
			if present {
				s.Current["DB_DSN"] = ""
			}
			s.state = state{Version: 2, Pending: &pendingChange{Before: map[string]string{}, After: map[string]string{"DB_DSN": ""}}}
			if err := s.reconcilePending(); err != nil {
				t.Fatal(err)
			}
			if s.pendingCleanup != present {
				t.Fatal("recovery conflated absent and empty DSNs")
			}
		})
	}
}

// TestRecoveryFinalizationPreservesConcurrentEdits keeps the journal when cleanup observes a newer active environment.
func TestRecoveryFinalizationPreservesConcurrentEdits(t *testing.T) {
	for _, managed := range []bool{false, true} {
		t.Run(fmt.Sprint(managed), func(t *testing.T) {
			root := recoveryFixture(t)
			s := openTest(t, root)
			var committed []byte
			s.rename = func(from, to string) error {
				if err := os.Rename(from, to); err != nil {
					return err
				}
				if filepath.Base(to) == ".env" {
					var err error
					committed, err = os.ReadFile(to)
					if err != nil {
						return err
					}
					changed := append(bytes.Clone(committed), []byte("OWNER_LATER=concurrent\n")...)
					if managed {
						changed = []byte(strings.ReplaceAll(string(committed), "DB_DSN=\n", "DB_DSN=concurrent-secret\n"))
					}
					return os.WriteFile(to, changed, 0600)
				}
				return nil
			}
			if err := runRecoveryOperation(t, s, "first"); err == nil || !strings.Contains(err.Error(), "settings were updated") {
				t.Fatalf("expected concurrent cleanup failure, got %v", err)
			}
			backup, err := os.ReadFile(filepath.Join(root, stateName))
			if err != nil || !bytes.Contains(backup, []byte("synthetic-recovery-secret")) {
				t.Fatal("concurrent cleanup failure lost recovery data")
			}
			if managed {
				if _, err := Open(root); err == nil || !strings.Contains(err.Error(), "neither side") {
					t.Fatalf("accepted mixed controlled settings: %v", err)
				}
				put(t, root, ".env", string(committed))
			}
			recovered := openTest(t, root)
			if err := recovered.Restore(); err != nil {
				t.Fatal(err)
			}
			active, err := os.ReadFile(filepath.Join(root, ".env"))
			if err != nil || !bytes.Contains(active, []byte("synthetic-recovery-secret")) {
				t.Fatal("recovery failed after concurrent cleanup edit")
			}
			if !managed && !bytes.Contains(active, []byte("OWNER_LATER=concurrent")) {
				t.Fatal("recovery lost unrelated concurrent settings")
			}
		})
	}
}

// TestFailedActivePublicationRetainsJournalAfterConcurrentEdit protects first-switch credentials when returned-error rollback cannot trust the active file.
func TestFailedActivePublicationRetainsJournalAfterConcurrentEdit(t *testing.T) {
	root := recoveryFixture(t)
	runLocalGit(t, root, "init", "-q")
	s := openTest(t, root)
	s.rename = func(from, to string) error {
		if filepath.Base(to) == ".env" {
			put(t, root, ".env", "DB_DRIVER=sqlite\nDB_DSN=concurrent-secret\nCACHE_DRIVER=memory\n")
			return errors.New("injected rename failure after concurrent edit")
		}
		return os.Rename(from, to)
	}
	if err := runRecoveryOperation(t, s, "first"); err == nil {
		t.Fatal("accepted failed active publication")
	}
	backup, err := os.ReadFile(filepath.Join(root, stateName))
	if err != nil || !bytes.Contains(backup, []byte("synthetic-recovery-secret")) {
		t.Fatal("rollback deleted the only recovery copy")
	}
	if _, err := os.Stat(filepath.Join(root, ".gitignore")); err != nil {
		t.Fatal("rollback removed private-file protection")
	}
	assertLocalSecretUntracked(t, root, "synthetic-recovery-secret")
	if _, err := Open(root); err == nil {
		t.Fatal("recovery accepted ambiguous active edits")
	}
}

// TestRecoveredSessionKeepsRawBackupConcurrencySnapshot rejects later changes to the pending bytes even after in-memory reconciliation.
func TestRecoveredSessionKeepsRawBackupConcurrencySnapshot(t *testing.T) {
	root := recoveryFixture(t)
	s := openTest(t, root)
	backups := 0
	s.rename = func(from, to string) error {
		if filepath.Base(to) == stateName {
			backups++
			if backups == 2 {
				return errors.New("injected cleanup failure")
			}
		}
		return os.Rename(from, to)
	}
	if err := runRecoveryOperation(t, s, "first"); err == nil {
		t.Fatal("expected cleanup failure")
	}
	recovered := openTest(t, root)
	backup, err := os.ReadFile(filepath.Join(root, stateName))
	if err != nil {
		t.Fatal(err)
	}
	put(t, root, stateName, string(append(backup, ' ')))
	if err := recovered.Restore(); err == nil || !strings.Contains(err.Error(), "changed while") {
		t.Fatalf("lost raw pending concurrency baseline: %v", err)
	}
}

// TestInterruptedRestoreKeepsAbsentAndEmptySettings preserves the distinction through actual process termination and cleanup retries.
func TestInterruptedRestoreKeepsAbsentAndEmptySettings(t *testing.T) {
	for _, present := range []bool{false, true} {
		for _, boundary := range []string{"after-pending", "after-env", "before-final"} {
			t.Run(fmt.Sprint(present)+"/"+boundary, func(t *testing.T) {
				root := recoveryFixture(t)
				raw := "DB_DRIVER=mysql\nCACHE_DRIVER=redis\n"
				if present {
					raw += "DB_DSN=\n"
				}
				put(t, root, ".env", raw)
				s := openTest(t, root)
				original := clone(s.Current)
				if err := s.Apply(recoveryPlan(t, s, "first")); err != nil {
					t.Fatal(err)
				}
				command := exec.Command(os.Args[0], "-test.run=^TestRecoveryInterruptionProcess$", "-test.count=1")
				command.Env = append(os.Environ(), "FORJ_RECOVERY_TEST_ROOT="+root, "FORJ_RECOVERY_TEST_OPERATION=restore", "FORJ_RECOVERY_TEST_BOUNDARY="+boundary)
				output, err := command.CombinedOutput()
				var failure *exec.ExitError
				if !errors.As(err, &failure) || failure.ExitCode() != 73 {
					t.Fatalf("expected hard exit: %v\n%s", err, output)
				}
				if err := os.Remove(filepath.Join(root, lockName)); err != nil {
					t.Fatal(err)
				}
				recovered := openTest(t, root)
				if err := recovered.Restore(); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(openTest(t, root).Current, original) {
					t.Fatal("recovery changed assignment presence or value")
				}
			})
		}
	}
}

// TestRecoveryFormatRejectsUnsafeVersionCombinations prevents older binaries from silently discarding a pending transaction.
func TestRecoveryFormatRejectsUnsafeVersionCombinations(t *testing.T) {
	for _, data := range []string{
		`{"version":2}`,
		`{"version":1,"pending":{"before":{"DB_DSN":""},"after":{}}}`,
		`{"version":3}`,
	} {
		t.Run(data, func(t *testing.T) {
			root := recoveryFixture(t)
			put(t, root, stateName, data)
			if _, err := Open(root); err == nil {
				t.Fatal("accepted mismatched recovery format")
			}
			actual, err := os.ReadFile(filepath.Join(root, stateName))
			if err != nil || string(actual) != data {
				t.Fatal("unsupported format changed recovery data")
			}
		})
	}
}
