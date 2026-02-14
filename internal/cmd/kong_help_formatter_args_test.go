package cmd

import (
	"os"
	"testing"
)

func TestMaintainerHelpEnabledFromArgs(t *testing.T) {
	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })

	origEnv, hadEnv := os.LookupEnv("FORJ_DEV")
	t.Cleanup(func() {
		if hadEnv {
			_ = os.Setenv("FORJ_DEV", origEnv)
		} else {
			_ = os.Unsetenv("FORJ_DEV")
		}
	})
	_ = os.Unsetenv("FORJ_DEV")

	cases := []struct {
		name string
		args []string
		want bool
	}{
		{name: "no toggle", args: []string{"forj"}, want: false},
		{name: "dev flag", args: []string{"forj", "--dev"}, want: true},
		{name: "dev flag explicit true", args: []string{"forj", "--dev=true"}, want: true},
		{name: "x alias", args: []string{"forj", "--x"}, want: true},
		{name: "x alias explicit true", args: []string{"forj", "--x=true"}, want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			os.Args = tc.args
			got := maintainerHelpEnabled()
			if got != tc.want {
				t.Fatalf("maintainerHelpEnabled() = %v, want %v (args=%v)", got, tc.want, tc.args)
			}
		})
	}
}
