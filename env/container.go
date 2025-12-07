package env

import (
	"os"
)

var (
	// These are shims that tests override.
	statFile = os.Stat
	readFile = os.ReadFile
	getEnv   = os.Getenv
)

const (
	// files
	fileDockerSock = "/var/run/docker.sock"
	fileDockerEnv  = "/.dockerenv"
	fileCgroup     = "/proc/1/cgroup"

	// cgroup names
	cgroupContainer      = "container"
	cgroupNameDocker     = "docker"
	cgroupNameKube       = "kubepods"
	cgroupNameContainerd = "containerd"
	cgroupNamePodman     = "podman"
	cgroupNameLibpod     = "libpod"
)

// IsDocker reports whether the current process is running in a Docker container.
func IsDocker() bool {
	// Check /.dockerenv
	if _, err := statFile(fileDockerEnv); err == nil {
		return true
	}

	// Check cgroup
	cgroup, err := readFile(fileCgroup)
	if err == nil && containsAny(cgroup, cgroupNameDocker, cgroupNameContainerd, cgroupNamePodman) {
		return true
	}

	return false
}

// IsDockerInDocker reports whether we are inside a Docker-in-Docker environment.
func IsDockerInDocker() bool {
	// If /.dockerenv does not exist → not a Docker *container* at all.
	if _, err := statFile(fileDockerEnv); err != nil {
		return false
	}

	// If docker.sock exists → this IS an inner DinD container.
	if _, err := statFile(fileDockerSock); err == nil {
		return true
	}

	return false
}

// IsDockerHost reports whether this container behaves like a Docker host.
func IsDockerHost() bool {
	if _, err := statFile(fileDockerSock); err != nil {
		return false
	}

	cgroup, err := readFile(fileCgroup)
	if err != nil {
		return false
	}

	// Docker host should *not* have container-scoped cgroups
	if !containsAny(cgroup, cgroupNameDocker, cgroupNameKube, cgroupNameContainerd) {
		return true
	}

	return false
}

// IsContainer detects any container runtime.
func IsContainer() bool {
	if IsDocker() {
		return true
	}

	cgroup, err := readFile(fileCgroup)
	if err == nil && containsAny(cgroup,
		cgroupContainer,
		cgroupNameKube,
		cgroupNameLibpod,
		cgroupNameContainerd,
	) {
		return true
	}

	if getEnv("KUBERNETES_SERVICE_HOST") != "" {
		return true
	}

	return false
}

// IsKubernetes reports whether running inside Kubernetes.
func IsKubernetes() bool {
	if getEnv("KUBERNETES_SERVICE_HOST") != "" {
		return true
	}

	cgroup, err := readFile(fileCgroup)
	if err == nil && containsAny(cgroup, cgroupNameKube) {
		return true
	}

	return false
}
