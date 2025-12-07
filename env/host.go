package env

// IsHostEnvironment reports whether the process is running *outside* any
// container or orchestrated runtime.
//
// Being a Docker host does NOT count as being in a container.
func IsHostEnvironment() bool {
	return !IsContainer() &&
		!IsDockerInDocker()
}
