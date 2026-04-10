package testkit

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/docker/go-connections/nat"
	testcontainers "github.com/testcontainers/testcontainers-go"
	tclog "github.com/testcontainers/testcontainers-go/log"
	"github.com/testcontainers/testcontainers-go/wait"
)

type Logf func(format string, args ...any)

type StartedContainer struct {
	Host      string
	Port      string
	Container testcontainers.Container
	stop      func()
}

type quietTestcontainersLogger struct{}

func (quietTestcontainersLogger) Printf(string, ...any) {}

func init() {
	tclog.SetDefault(quietTestcontainersLogger{})
}

func (c *StartedContainer) Stop() {
	if c != nil && c.stop != nil {
		c.stop()
	}
}

func StartTestcontainer(
	logf Logf,
	request testcontainers.ContainerRequest,
	portSpec string,
	readyTimeout time.Duration,
	readyLabel string,
) (*StartedContainer, error) {
	if logf != nil {
		if request.FromDockerfile.Context != "" {
			logf("Building %s container image", readyLabel)
		}
		logf("Starting %s container", readyLabel)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: request,
		Started:          true,
		Logger:           quietTestcontainersLogger{},
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("start %s testcontainer: %w", readyLabel, err)
	}
	host, port, err := mappedContainerEndpoint(ctx, container, portSpec)
	if err != nil {
		_ = container.Terminate(context.Background())
		cancel()
		return nil, err
	}
	if err := WaitForTCPReadyAddress(host, port, readyTimeout); err != nil {
		_ = container.Terminate(context.Background())
		cancel()
		return nil, err
	}
	if logf != nil {
		logf("%s container ready at %s:%s", readyLabel, host, port)
	}
	return &StartedContainer{
		Host:      host,
		Port:      port,
		Container: container,
		stop: func() {
			_ = container.Terminate(context.Background())
			cancel()
		},
	}, nil
}

func StartMySQLTestcontainer(logf Logf, testEnv map[string]string) (func(), error) {
	started, err := StartTestcontainer(
		logf,
		testcontainers.ContainerRequest{
			Image:        "mysql:8.4",
			ExposedPorts: []string{"3306/tcp"},
			Env: map[string]string{
				"MYSQL_DATABASE":      "db",
				"MYSQL_USER":          "user",
				"MYSQL_PASSWORD":      "password",
				"MYSQL_ROOT_PASSWORD": "root",
			},
			WaitingFor: wait.ForLog("ready for connections").WithStartupTimeout(90 * time.Second),
		},
		"3306/tcp",
		60*time.Second,
		"MySQL",
	)
	if err != nil {
		return nil, err
	}
	if err := WaitForContainerExecSuccess(
		started.Container,
		[]string{
			"sh", "-lc",
			`mysql -h 127.0.0.1 -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" "$MYSQL_DATABASE" -e 'SELECT 1'`,
		},
		60*time.Second,
	); err != nil {
		started.Stop()
		return nil, fmt.Errorf("wait for mysql authenticated readiness: %w", err)
	}
	if testEnv != nil {
		testEnv["DB_HOST"] = started.Host
		testEnv["DB_PORT"] = started.Port
		testEnv["DB_HOST_INTEGRATION"] = started.Host
		testEnv["DB_PORT_INTEGRATION"] = started.Port
	}
	return started.Stop, nil
}

func StartPostgresTestcontainer(logf Logf, testEnv map[string]string) (func(), error) {
	started, err := StartTestcontainer(
		logf,
		testcontainers.ContainerRequest{
			Image:        "postgres:16-alpine",
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_DB":       "app",
				"POSTGRES_USER":     "postgres",
				"POSTGRES_PASSWORD": "postgres",
			},
			WaitingFor: wait.ForLog("database system is ready to accept connections").WithStartupTimeout(60 * time.Second),
		},
		"5432/tcp",
		60*time.Second,
		"Postgres",
	)
	if err != nil {
		return nil, err
	}
	if testEnv != nil {
		testEnv["DB_HOST"] = started.Host
		testEnv["DB_PORT"] = started.Port
		testEnv["DB_HOST_INTEGRATION"] = started.Host
		testEnv["DB_PORT_INTEGRATION"] = started.Port
	}
	return started.Stop, nil
}

func StartRedisTestcontainer(logf Logf, testEnv map[string]string) (func(), error) {
	started, err := StartTestcontainer(
		logf,
		testcontainers.ContainerRequest{
			Image:        "redis:7.4",
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor: wait.ForAll(
				wait.ForListeningPort("6379/tcp"),
				wait.ForLog("Ready to accept connections"),
			).WithStartupTimeout(60 * time.Second),
		},
		"6379/tcp",
		60*time.Second,
		"Redis",
	)
	if err != nil {
		return nil, err
	}
	if testEnv != nil {
		testEnv["REDIS_HOST"] = started.Host
		testEnv["REDIS_PORT"] = started.Port
	}
	return started.Stop, nil
}

func WaitForTCPReadyAddress(host, port string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	address := net.JoinHostPort(host, port)
	var lastErr error
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 2*time.Second)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}
	if lastErr != nil {
		return fmt.Errorf("tcp endpoint %s not ready: %w", address, lastErr)
	}
	return fmt.Errorf("tcp endpoint %s not ready", address)
}

func WaitForContainerExecSuccess(container testcontainers.Container, cmd []string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		exitCode, output, err := container.Exec(context.Background(), cmd)
		if err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if exitCode == 0 {
			if output != nil {
				_, _ = io.Copy(io.Discard, output)
			}
			return nil
		}
		body := ""
		if output != nil {
			data, _ := io.ReadAll(output)
			body = strings.TrimSpace(string(data))
		}
		if body == "" {
			body = fmt.Sprintf("exit code %d", exitCode)
		}
		lastErr = fmt.Errorf("%s", body)
		time.Sleep(500 * time.Millisecond)
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("container command did not succeed before timeout")
}

func mappedContainerEndpoint(ctx context.Context, container testcontainers.Container, portSpec string) (string, string, error) {
	host, err := container.Host(ctx)
	if err != nil {
		return "", "", fmt.Errorf("resolve testcontainer host: %w", err)
	}
	mappedPort, err := container.MappedPort(ctx, nat.Port(portSpec))
	if err != nil {
		return "", "", fmt.Errorf("resolve testcontainer port %s: %w", portSpec, err)
	}
	return host, mappedPort.Port(), nil
}
