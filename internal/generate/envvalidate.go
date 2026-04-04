package generate

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/goforj/env/v2"
	"github.com/goforj/str"
)

type primitiveEnvContract struct {
	Prefix        string
	DefaultDriver string
	RootKeys      []string
	CommonKeys    map[string]struct{}
	DriverKeys    map[string]map[string]struct{}
	ChildNames    func(scope env.Scope) []string
	AllowInactiveRootKeys bool
}

func validatePrimitiveEnv(contract primitiveEnvContract) error {
	rootKeySet := makeSet(contract.RootKeys...)
	scope := env.WithPrefix(contract.Prefix)
	childNames := contract.ChildNames(scope)
	knownChildren := makeSet(childNames...)
	supportedDrivers, err := parseSupportedDrivers(contract.Prefix, contract.DriverKeys)
	if err != nil {
		return err
	}

	var problems []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(key, contract.Prefix+"_") {
			continue
		}
		if key == contract.Prefix+"_SUPPORTED_DRIVERS" {
			continue
		}

		trimmed := strings.TrimPrefix(key, contract.Prefix+"_")
		child, rootKey, ok := splitScopedEnvKey(trimmed, contract.RootKeys)
		if !ok {
			problems = append(problems, fmt.Sprintf("%s is not a supported %s env var", key, strings.ToLower(contract.Prefix)))
			continue
		}
		if child != "" {
			if _, ok := knownChildren[child]; !ok {
				problems = append(problems, fmt.Sprintf("%s does not match a valid %s scope", key, strings.ToLower(contract.Prefix)))
				continue
			}
		}

		driverScope := scope
		if child != "" {
			driverScope = scope.Child(child)
		}
		driver := str.Of(driverScope.Get("DRIVER", contract.DefaultDriver)).TrimSpace().ToLower().String()
		if driver == "" {
			driver = contract.DefaultDriver
		}
		if supportedDrivers != nil {
			if _, ok := supportedDrivers[driver]; !ok {
				if child == "" {
					problems = append(problems, fmt.Sprintf("%s selects driver %q not enabled by %s_SUPPORTED_DRIVERS", contract.Prefix+"_DRIVER", driver, contract.Prefix))
				} else {
					problems = append(problems, fmt.Sprintf("%s selects driver %q not enabled by %s_SUPPORTED_DRIVERS", contract.Prefix+"_"+child+"_DRIVER", driver, contract.Prefix))
				}
				continue
			}
		}
		allowedKeys, err := allowedPrimitiveKeys(contract, driver)
		if err != nil {
			if child == "" {
				problems = append(problems, fmt.Sprintf("%s selects unsupported driver %q", contract.Prefix+"_DRIVER", driver))
			} else {
				problems = append(problems, fmt.Sprintf("%s selects unsupported driver %q", contract.Prefix+"_"+child+"_DRIVER", driver))
			}
			continue
		}
		if _, ok := rootKeySet[rootKey]; !ok {
			problems = append(problems, fmt.Sprintf("%s is not a supported %s env var", key, strings.ToLower(contract.Prefix)))
			continue
		}
		if _, ok := allowedKeys[rootKey]; ok {
			continue
		}
		if child == "" && contract.AllowInactiveRootKeys {
			continue
		}
		problems = append(problems, fmt.Sprintf("%s is not supported for %s driver %q", key, strings.ToLower(contract.Prefix), driver))
	}

	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return fmt.Errorf("invalid %s env:\n- %s", strings.ToLower(contract.Prefix), strings.Join(problems, "\n- "))
}

func allowedPrimitiveKeys(contract primitiveEnvContract, driver string) (map[string]struct{}, error) {
	allowed := make(map[string]struct{}, len(contract.CommonKeys)+1)
	for key := range contract.CommonKeys {
		allowed[key] = struct{}{}
	}

	driverKeys, ok := contract.DriverKeys[driver]
	if !ok {
		return nil, fmt.Errorf("unsupported driver %q", driver)
	}
	for key := range driverKeys {
		allowed[key] = struct{}{}
	}
	return allowed, nil
}

func splitScopedEnvKey(value string, rootKeys []string) (child string, rootKey string, ok bool) {
	for _, rootKey := range rootKeys {
		if value == rootKey {
			return "", rootKey, true
		}
	}
	for _, rootKey := range rootKeys {
		suffix := "_" + rootKey
		if !strings.HasSuffix(value, suffix) {
			continue
		}
		child = strings.TrimSuffix(value, suffix)
		child = str.Of(child).TrimSpace().ToUpper().String()
		if child == "" {
			return "", "", false
		}
		return child, rootKey, true
	}
	return "", "", false
}

func makeSet(values ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func parseSupportedDrivers(prefix string, knownDrivers map[string]map[string]struct{}) (map[string]struct{}, error) {
	raw := str.Of(env.WithPrefix(prefix).Get("SUPPORTED_DRIVERS", "")).TrimSpace().ToLower().String()
	if raw == "" {
		return nil, nil
	}
	set := map[string]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		driver := str.Of(part).TrimSpace().ToLower().String()
		if driver == "" {
			continue
		}
		if _, ok := knownDrivers[driver]; !ok {
			return nil, fmt.Errorf("%s_SUPPORTED_DRIVERS includes unsupported driver %q", prefix, driver)
		}
		set[driver] = struct{}{}
	}
	if len(set) == 0 {
		return nil, nil
	}
	return set, nil
}

func supportedDrivers(prefix string, knownDrivers map[string]map[string]struct{}, fallback []string) ([]string, error) {
	set, err := parseSupportedDrivers(prefix, knownDrivers)
	if err != nil {
		return nil, err
	}
	if set != nil {
		return sortStrings(set), nil
	}

	out := map[string]struct{}{}
	for _, driver := range fallback {
		driver = str.Of(driver).TrimSpace().ToLower().String()
		if driver == "" {
			continue
		}
		if _, ok := knownDrivers[driver]; !ok {
			continue
		}
		out[driver] = struct{}{}
	}
	return sortStrings(out), nil
}

func sortStrings(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for driver := range set {
		out = append(out, driver)
	}
	sort.Strings(out)
	return out
}
