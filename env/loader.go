package env

import (
	"fmt"
	"github.com/joho/godotenv"
	"os"
	"runtime"
)

// MaxDirectorySeekLevels is the number of directory
// levels a .env file needs to be searched in
const MaxDirectorySeekLevels int = 10

const (
	runtimeDarwin  = "darwin"
	runtimeWindows = "windows"

	// file
	fileEnv        = ".env"
	fileEnvHost    = ".env.host"
	envFileTesting = ".env.testing"
)

// envLoaded is a flag to check if the environment file has been loaded
var envLoaded = false

// LoadEnvFileIfExists loads environment file .env locally
// loads .env.testing if invoked from the context of a test file
// loads .env.host if invoked from the context of MacOS which references variables to communicate back to the docker network
func LoadEnvFileIfExists() error {
	if runtime.GOOS == runtimeDarwin || runtime.GOOS == runtimeWindows {
		_ = os.Setenv("APP_ENV", Local)
	}

	if !envLoaded {
		// use dev or testing envs depending on the environment
		envLoadMsg := ""
		envFile := fileEnv
		if IsAppEnvTesting() {
			envFile = envFileTesting
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

		// search for global .env.host
		// we're likely talking from host -> container network
		// used from IDEs
		if IsHostEnvironment() || IsDockerInDocker() {
			env := fileEnvHost
			if loadEnvFile(env) {
				if GetInt("APP_DEBUG", "0") > 0 {
					fmt.Println(fmt.Sprintf("Loaded environment [env] APP_ENV [%v] ENV_FILE [%v]", os.Getenv("APP_ENV"), env))
				}
			}
		}
	}

	return nil
}

// IsEnvLoaded checks if the environment file has been loaded
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
