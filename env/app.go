package env

import (
	"flag"
	"os"
	"strings"
)

// environment helpers
const (
	Testing    = "testing"
	Local      = "local"
	Dev        = "dev"
	Staging    = "staging"
	Production = "production"
)

// IsAppEnvTesting checks if the current APP_ENV is testing or if we're inside running a test
func IsAppEnvTesting() bool {
	return os.Getenv("APP_ENV") == Testing ||
		flag.Lookup("test.v") != nil ||
		isTestSuffixFromArguments()
}

// isTestSuffixFromArguments checks if the test suffix is present in the command line arguments
func isTestSuffixFromArguments() bool {
	anyArgumentContainsTestSuffix := false

	for _, arg := range os.Args {
		if strings.HasSuffix(arg, ".test") || strings.HasSuffix(arg, "-test.run") {
			anyArgumentContainsTestSuffix = true
		}
	}

	return anyArgumentContainsTestSuffix
}

// GetAppEnv returns the current APP_ENV
func GetAppEnv() string {
	return os.Getenv("APP_ENV")
}

// IsAppEnv checks if the current APP_ENV matches any of the provided environments
func IsAppEnv(envs ...string) bool {
	current := os.Getenv("APP_ENV")
	for _, env := range envs {
		if current == env {
			return true
		}
	}
	return false
}

// IsAppEnvProduction checks if the current APP_ENV is production
func IsAppEnvProduction() bool {
	return IsAppEnv(Production)
}

// IsAppEnvStaging checks if the current APP_ENV is staging
func IsAppEnvStaging() bool {
	return IsAppEnv(Staging)
}

// IsAppEnvLocalOrStaging checks if the current APP_ENV is local or staging
func IsAppEnvLocalOrStaging() bool {
	return IsAppEnv(Local, Staging)
}

// IsAppEnvLocal checks if the current APP_ENV is local
func IsAppEnvLocal() bool {
	return IsAppEnv(Local)
}

// IsAppEnvDev checks if the current APP_ENV is dev
func IsAppEnvDev() bool {
	return IsAppEnv(Dev)
}

// IsAppEnvTestingOrLocal checks if the current APP_ENV is testing or local
func IsAppEnvTestingOrLocal() bool {
	return IsAppEnv(Testing, Local)
}
