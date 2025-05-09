package env

import (
	"errors"
	"flag"
	"fmt"
	"github.com/joho/godotenv"
	"os"
	"runtime"
	"strings"
)

// MaxDirectorySeekLevels is the number of directory
// levels a .env file needs to be searched in
const MaxDirectorySeekLevels int = 10

var envLoaded = false

// LoadEnvFileIfExists loads environment file .env locally
// loads .env.testing if invoked from the context of a test file
// loads .env.host if invoked from the context of MacOS which references variables to communicate back to the docker network
func LoadEnvFileIfExists() error {
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		_ = os.Setenv("APP_ENV", "local")
	}

	if !envLoaded {
		// use dev or testing envs depending on the environment
		envLoadMsg := ""
		envFile := ".env"
		if IsAppEnvTesting() {
			envFile = ".env.testing"
		}

		// load top-level .env
		if loadEnvFile(envFile) {
			envLoadMsg = fmt.Sprintf("[LoadEnv] APP_ENV [%v] ENV_FILE [%v]", os.Getenv("APP_ENV"), envFile)
		}

		// display env: [LoadEnv] APP_ENV [local] ENV_FILE [.env]
		if GetInt("APP_DEBUG", "0") >= 3 {
			fmt.Println(envLoadMsg)
		}

		envLoaded = true

		// check if running in docker
		isInDocker := true
		if _, err := os.Stat("/.dockerenv"); errors.Is(err, os.ErrNotExist) {
			isInDocker = false
		}

		// search for global .env.host
		// we're likely talking from host -> container network
		// used from IDEs
		if !isInDocker {
			env := ".env.host"
			if loadEnvFile(env) {
				if GetInt("APP_DEBUG", "0") > 0 {
					fmt.Println(fmt.Sprintf("Loaded environment [env] APP_ENV [%v] ENV_FILE [%v]", os.Getenv("APP_ENV"), env))
				}
			}
		}
	}

	return nil
}

func IsEnvLoaded() bool {
	return envLoaded
}

// searches for .env file through directory traversal
// loads .env file if found
func loadEnvFile(envFile string) bool {
	var path string
	found := false
	for i := 0; i < MaxDirectorySeekLevels; i++ {
		if _, err := os.Stat(path + envFile); err == nil {
			path += envFile
			found = true
			break
		}
		path += "../"
	}

	if found {
		if err := godotenv.Overload(path); err != nil {
			panic(err)
		}
	}

	return found
}

// environment helpers
const (
	AppEnvTesting    = "testing"
	AppEnvLocal      = "local"
	AppEnvDev        = "dev" // effectively the same as (local)
	AppEnvStaging    = "staging"
	AppEnvProduction = "production"
)

func GetAppEnv() string {
	return os.Getenv("APP_ENV")
}

func IsAppEnvDev() bool {
	return os.Getenv("APP_ENV") == AppEnvDev
}

func IsAppEnvLocal() bool {
	return os.Getenv("APP_ENV") == AppEnvLocal ||
		os.Getenv("APP_ENV") == AppEnvDev
}

func IsAppEnvTesting() bool {
	return os.Getenv("APP_ENV") == AppEnvTesting ||
		flag.Lookup("test.v") != nil ||
		isTestSuffixFromArguments()
}

func IsAppEnvProduction() bool {
	return os.Getenv("APP_ENV") == AppEnvProduction
}

func isTestSuffixFromArguments() bool {
	anyArgumentContainsTestSuffix := false

	for _, arg := range os.Args {
		if strings.HasSuffix(arg, ".test") || strings.HasSuffix(arg, "-test.run") {
			anyArgumentContainsTestSuffix = true
		}
	}

	return anyArgumentContainsTestSuffix
}

const (
	AppWebserver = "web"
	AppCli       = "cli"
)

func IsAppModeWebserver() bool {
	return os.Getenv("APP_MODE") == AppWebserver
}

func IsAppModeCli() bool {
	return os.Getenv("APP_MODE") == AppCli
}

func SetAppModeCli() {
	_ = os.Setenv("APP_MODE", AppCli)
}

func SetAppModeWebserver() {
	_ = os.Setenv("APP_MODE", AppWebserver)
}
