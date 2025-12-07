package env

import (
	"os"
	"testing"
)

// Save real implementations so we can restore them.
var (
	realStatFile = statFile
	realReadFile = readFile
	realGetEnv   = getEnv
)

// Reset all shims after each test.
func reset() {
	statFile = realStatFile
	readFile = realReadFile
	getEnv = realGetEnv
}

// Helper for tests.
func mockEnv(vars map[string]string) {
	getEnv = func(key string) string {
		return vars[key]
	}
}

func TestIsDocker_DockerenvExists(t *testing.T) {
	defer reset()

	statFile = func(path string) (os.FileInfo, error) {
		if path == "/.dockerenv" {
			return nil, nil // simulate exists
		}
		return nil, os.ErrNotExist
	}

	readFile = func(path string) ([]byte, error) {
		return nil, os.ErrNotExist
	}

	if !IsDocker() {
		t.Fatal("expected IsDocker() == true when /.dockerenv exists")
	}
}

func TestIsDocker_CgroupDetectsContainer(t *testing.T) {
	defer reset()

	statFile = func(path string) (os.FileInfo, error) {
		return nil, os.ErrNotExist // no /.dockerenv
	}

	readFile = func(path string) ([]byte, error) {
		return []byte("0::/docker/12345"), nil
	}

	if !IsDocker() {
		t.Fatal("expected IsDocker() == true from docker cgroup")
	}
}

func TestIsDocker_False(t *testing.T) {
	defer reset()

	statFile = func(path string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	}

	readFile = func(path string) ([]byte, error) {
		return []byte("0::/user.slice"), nil
	}

	if IsDocker() {
		t.Fatal("expected IsDocker() == false when no signals")
	}
}

func TestIsDind_True(t *testing.T) {
	defer reset()

	statFile = func(path string) (os.FileInfo, error) {
		switch path {
		case fileDockerEnv:
			return nil, nil // must have /.dockerenv
		case fileDockerSock:
			return nil, nil // must have docker.sock
		}
		return nil, os.ErrNotExist
	}

	// DinD inner often has host-like cgroups
	readFile = func(path string) ([]byte, error) {
		return []byte("0::/user.slice"), nil
	}

	if !IsDockerInDocker() {
		t.Fatal("expected IsDockerInDocker() == true for DinD inner")
	}
}

func TestIsDind_FalseWhenNormalContainer(t *testing.T) {
	defer reset()

	statFile = func(path string) (os.FileInfo, error) {
		if path == "/var/run/docker.sock" {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}

	readFile = func(path string) ([]byte, error) {
		return []byte("0::/docker/12345"), nil
	}

	if IsDockerInDocker() {
		t.Fatal("expected IsDockerInDocker() == false in real container cgroup")
	}
}

func TestIsDockerHost_True(t *testing.T) {
	defer reset()

	statFile = func(path string) (os.FileInfo, error) {
		if path == "/var/run/docker.sock" {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}

	readFile = func(path string) ([]byte, error) {
		return []byte("0::/user.slice"), nil
	}

	if !IsDockerHost() {
		t.Fatal("expected IsDockerHost() == true")
	}
}

func TestIsDockerHost_FalseWhenNamespaced(t *testing.T) {
	defer reset()

	statFile = func(path string) (os.FileInfo, error) {
		if path == "/var/run/docker.sock" {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}

	readFile = func(path string) ([]byte, error) {
		return []byte("0::/docker/xyz"), nil
	}

	if IsDockerHost() {
		t.Fatal("expected IsDockerHost() == false in container namespace")
	}
}

func TestIsKubernetes_EnvVar(t *testing.T) {
	defer reset()

	mockEnv(map[string]string{
		"KUBERNETES_SERVICE_HOST": "10.0.0.1",
	})

	if !IsKubernetes() {
		t.Fatal("expected IsKubernetes() == true via env")
	}
}

func TestIsKubernetes_Cgroup(t *testing.T) {
	defer reset()

	mockEnv(nil)

	readFile = func(path string) ([]byte, error) {
		return []byte("0::/kubepods/besteffort"), nil
	}

	if !IsKubernetes() {
		t.Fatal("expected IsKubernetes() == true via cgroup")
	}
}

func TestIsKubernetes_False(t *testing.T) {
	defer reset()

	mockEnv(nil)

	readFile = func(path string) ([]byte, error) {
		return []byte("0::/user.slice"), nil
	}

	if IsKubernetes() {
		t.Fatal("expected false when no kube signal")
	}
}

func TestIsContainer_Docker(t *testing.T) {
	defer reset()

	statFile = func(path string) (os.FileInfo, error) {
		if path == "/.dockerenv" {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}

	if !IsContainer() {
		t.Fatal("expected true because IsDocker() is true")
	}
}

func TestIsContainer_ContainerCgroup(t *testing.T) {
	defer reset()

	statFile = func(path string) (os.FileInfo, error) { return nil, os.ErrNotExist }

	readFile = func(path string) ([]byte, error) {
		return []byte("0::/containerd/xyz"), nil
	}

	if !IsContainer() {
		t.Fatal("expected true from containerd cgroup")
	}
}

func TestIsContainer_Kubernetes(t *testing.T) {
	defer reset()

	mockEnv(map[string]string{
		"KUBERNETES_SERVICE_HOST": "10.0.0.1",
	})

	if !IsContainer() {
		t.Fatal("expected IsContainer() == true in kube")
	}
}

func TestIsContainer_False(t *testing.T) {
	defer reset()

	statFile = func(path string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	}

	readFile = func(path string) ([]byte, error) {
		return []byte("0::/user.slice"), nil
	}

	mockEnv(nil)

	if IsContainer() {
		t.Fatal("expected false when no container signals")
	}
}
