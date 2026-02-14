package cmd

import (
	"os"
	"testing"
)

func TestMaintainerHelpEnabledFromEnv(t *testing.T) {
	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })
	os.Args = []string{"forj"}

	origEnv, hadEnv := os.LookupEnv("FORJ_DEV")
	t.Cleanup(func() {
		if hadEnv {
			_ = os.Setenv("FORJ_DEV", origEnv)
		} else {
			_ = os.Unsetenv("FORJ_DEV")
		}
	})

	_ = os.Setenv("FORJ_DEV", "1")
	if !maintainerHelpEnabled() {
		t.Fatalf("expected maintainerHelpEnabled() from FORJ_DEV=1")
	}

	_ = os.Setenv("FORJ_DEV", "0")
	if maintainerHelpEnabled() {
		t.Fatalf("expected maintainerHelpEnabled() false from FORJ_DEV=0")
	}
}
