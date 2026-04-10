package forj

import (
	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/internal/testkit"
)

func startMySQLTestcontainer(silent bool, testEnv map[string]string) (func(), error) {
	return testkit.StartMySQLTestcontainer(integrationLogf(silent), testEnv)
}

func startPostgresTestcontainer(silent bool, testEnv map[string]string) (func(), error) {
	return testkit.StartPostgresTestcontainer(integrationLogf(silent), testEnv)
}

func startRedisTestcontainer(silent bool, testEnv map[string]string) (func(), error) {
	return testkit.StartRedisTestcontainer(integrationLogf(silent), testEnv)
}

func integrationLogf(silent bool) func(string, ...any) {
	if silent {
		return nil
	}
	return console.Infof
}
