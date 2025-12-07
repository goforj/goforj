package env

import (
	"bytes"
	"os"
)

var (
	// These are shims that tests override.
	statFile = os.Stat
	readFile = os.ReadFile
	getEnv   = os.Getenv
)

const (
	fileDockerSock = "/var/run/docker.sock"
	fileDockerEnv  = "/.dockerenv"
	fileCgroup     = "/proc/1/cgroup"
)

// IsDocker reports whether the current process is running in a Docker container.
func IsDocker() bool {
	// Check /.dockerenv
	if _, err := statFile("/.dockerenv"); err == nil {
		return true
	}

	// Check cgroup
	cgroup, err := readFile(fileCgroup)
	if err == nil {
		if bytes.Contains(cgroup, []byte("docker")) ||
			bytes.Contains(cgroup, []byte("containerd")) ||
			bytes.Contains(cgroup, []byte("podman")) {
			return true
		}
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

	if !bytes.Contains(cgroup, []byte("docker")) &&
		!bytes.Contains(cgroup, []byte("kubepods")) &&
		!bytes.Contains(cgroup, []byte("containerd")) {
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
	if err == nil {
		if bytes.Contains(cgroup, []byte("container")) ||
			bytes.Contains(cgroup, []byte("kubepods")) ||
			bytes.Contains(cgroup, []byte("libpod")) ||
			bytes.Contains(cgroup, []byte("containerd")) {
			return true
		}
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
	if err == nil {
		if bytes.Contains(cgroup, []byte("kubepods")) {
			return true
		}
	}

	return false
}
