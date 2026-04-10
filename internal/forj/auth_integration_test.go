//go:build integration

package forj

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
	testcontainers "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type authDatabaseFixture struct {
	host      string
	port      string
	username  string
	password  string
	driver    string
	container testcontainers.Container
	stop      func()
}

type authDatabaseFixtureState struct {
	once    sync.Once
	fixture *authDatabaseFixture
	err     error
}

var authDatabaseFixtures = map[string]*authDatabaseFixtureState{
	"mysql":    {},
	"postgres": {},
}

type authRenderedIntegrationCase struct {
	name       string
	moduleName string
	driver     string
	components project.Components
}

func cleanupAuthDatabaseFixtures() {
	for _, state := range authDatabaseFixtures {
		if state != nil && state.fixture != nil && state.fixture.stop != nil {
			state.fixture.stop()
		}
	}
}

func TestGeneratedAuthRenderedIntegration(t *testing.T) {
	if _, err := exec.LookPath("wire"); err != nil {
		t.Skip("wire is required for rendered app integration tests")
	}

	for _, tc := range []authRenderedIntegrationCase{
		{
			name:       "sqlite",
			moduleName: "example.com/authsqlite",
			driver:     "sqlite",
			components: project.Components{
				WebAPI:         true,
				Auth:           true,
				DatabaseSQLite: true,
			},
		},
		{
			name:       "mysql",
			moduleName: "example.com/authmysqlfull",
			driver:     "mysql",
			components: project.Components{
				WebAPI:        true,
				Auth:          true,
				DatabaseMySQL: true,
			},
		},
		{
			name:       "postgres",
			moduleName: "example.com/authpostgresfull",
			driver:     "postgres",
			components: project.Components{
				WebAPI:           true,
				Auth:             true,
				DatabasePostgres: true,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			projectDir := renderAuthIntegrationApp(t, tc)
			setupRenderedAuthEnv(t, projectDir)
			configureRenderedAuthDatabase(t, projectDir, tc.driver)
			runRenderedAuthPackageTests(t, projectDir, tc.driver)
			handle, baseURL := startRenderedAuthApp(t, projectDir)
			defer stopProcAsync(t, "auth-api", handle, time.Second)
			runRenderedAuthAppAssertions(t, baseURL)
		})
	}
}

func renderAuthIntegrationApp(t *testing.T, tc authRenderedIntegrationCase) string {
	t.Helper()

	projectDir := t.TempDir()
	renderEnv := map[string]string{}
	if driver := tc.components.DatabaseDriver(); driver != "" {
		renderEnv["DB_DRIVER"] = driver
		renderEnv["DB_SUPPORTED_DRIVERS"] = driver
	}
	renderProjectWithForj(t, projectDir, project.Config{
		ProjectName:  "AuthApp",
		GoModuleName: tc.moduleName,
		UpdatedAt:    "2026-04-09 00:00:00 UTC",
		Render: project.RenderConfig{
			Components: tc.components,
		},
	}, renderEnv)

	return projectDir
}

func runRenderedAuthPackageTests(t *testing.T, projectDir, driver string) {
	t.Helper()
	runRenderedAuthCommand(
		t,
		projectDir,
		"go test ./internal/auth",
		[]string{"go", "test", "./internal/auth", "-tags=integration," + driver, "-count=1"},
		integrationGoProcessEnv(map[string]string{
			"DB_DRIVER":            driver,
			"DB_SUPPORTED_DRIVERS": driver,
		}),
	)
}

func startRenderedAuthApp(t *testing.T, projectDir string) (*procHandle, string) {
	t.Helper()

	runRenderedAuthCommand(
		t,
		projectDir,
		"go build",
		[]string{"go", "build", "-o", "./bin/app", "."},
		integrationGoProcessEnv(nil),
	)
	runRenderedAuthCommand(
		t,
		projectDir,
		"migrate",
		[]string{"./bin/app", "migrate"},
		integrationProcessEnv(),
	)

	addr := findFreeAddr(t)
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}
	baseURL := "http://127.0.0.1:" + port
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		ctxRun, cancelRun := context.WithCancel(context.Background())
		cmd := exec.CommandContext(ctxRun, "./bin/app", "http:serve", "--port", port)
		cmd.Dir = projectDir
		cmd.Env = integrationProcessEnv()
		handle := &procHandle{name: "api", cmd: cmd, cancel: cancelRun}
		cmd.Stdout = &handle.stdout
		cmd.Stderr = &handle.stderr
		if err := handle.Start(); err != nil {
			cancelRun()
			t.Fatalf("start auth app: %v", err)
		}

		err := waitForAuthProbeEndpointReady(handle, baseURL+"/-/health", http.StatusOK, 15*time.Second)
		if err == nil {
			return handle, baseURL
		}
		lastErr = err
		stopProcAsync(t, "auth-api-retry", handle, time.Second)
		if !isTransientMySQLStartupError(err) || attempt == 3 {
			t.Fatalf("process exited before probe became ready: %v", err)
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("start auth app: %v", lastErr)
	return nil, ""
}

func runRenderedAuthAppAssertions(t *testing.T, baseURL string) {
	t.Helper()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	client := &http.Client{
		Timeout: 2 * time.Second,
		Jar:     jar,
	}

	assertStatus(t, client, http.MethodGet, baseURL+"/api/v1/hello", nil, http.StatusUnauthorized)
	assertStatus(t, client, http.MethodGet, baseURL+"/api/v1/auth/me", nil, http.StatusUnauthorized)

	assertJSONStatus(t, client, http.MethodPost, baseURL+"/api/v1/auth/login", map[string]any{
		"login":    "admin",
		"password": "wrong",
	}, http.StatusUnauthorized)

	loginResp := assertJSONStatus(t, client, http.MethodPost, baseURL+"/api/v1/auth/login", map[string]any{
		"login":    "admin",
		"password": "admin",
	}, http.StatusOK)
	if !strings.Contains(loginResp, `"username":"admin"`) {
		t.Fatalf("login response missing admin user:\n%s", loginResp)
	}

	meResp := assertJSONStatus(t, client, http.MethodGet, baseURL+"/api/v1/auth/me", nil, http.StatusOK)
	if !strings.Contains(meResp, `"username":"admin"`) {
		t.Fatalf("me response missing admin user:\n%s", meResp)
	}

	helloResp := assertStatusBody(t, client, http.MethodGet, baseURL+"/api/v1/hello", nil, http.StatusOK)
	if strings.TrimSpace(helloResp) != "Hello, World!" {
		t.Fatalf("hello body = %q, want %q", strings.TrimSpace(helloResp), "Hello, World!")
	}

	beforeAccess := authCookieValue(t, jar, baseURL, "auth_access")
	if beforeAccess == "" {
		t.Fatal("expected auth_access cookie after login")
	}

	time.Sleep(3 * time.Second)

	assertStatus(t, client, http.MethodGet, baseURL+"/api/v1/hello", nil, http.StatusOK)
	afterAccess := authCookieValue(t, jar, baseURL, "auth_access")
	if afterAccess == "" {
		t.Fatal("expected refreshed auth_access cookie")
	}
	if afterAccess == beforeAccess {
		t.Fatal("expected auth_access cookie to rotate after expiry/refresh")
	}

	assertJSONStatus(t, client, http.MethodPost, baseURL+"/api/v1/auth/logout", nil, http.StatusOK)
	assertStatus(t, client, http.MethodGet, baseURL+"/api/v1/hello", nil, http.StatusUnauthorized)
}

func runRenderedAuthCommand(t *testing.T, projectDir, name string, args []string, env []string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = projectDir
	cmd.Env = env

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s failed: %v\n%s", name, err, out.String())
	}
}

func setupRenderedAuthEnv(t *testing.T, projectDir string) {
	t.Helper()
	for _, kv := range []struct {
		key   string
		value string
	}{
		{"AUTH_ACCESS_TTL", "2s"},
		{"AUTH_REFRESH_TTL", "30m"},
		{"AUTH_COOKIE_SECURE", "false"},
		{"AUTH_BOOTSTRAP_USERNAME", "admin"},
		{"AUTH_BOOTSTRAP_PASSWORD", "admin"},
	} {
		if err := testkit.ReplaceOrAppendEnvValues([]string{filepath.Join(projectDir, ".env")}, map[string]string{kv.key: kv.value}); err != nil {
			t.Fatalf("set %s: %v", kv.key, err)
		}
	}
}

func configureRenderedAuthDatabase(t *testing.T, projectDir, driver string) {
	t.Helper()

	setEnv := func(key, value string) {
		if err := testkit.ReplaceOrAppendEnvValues(
			[]string{filepath.Join(projectDir, ".env"), filepath.Join(projectDir, ".env.host")},
			map[string]string{key: value},
		); err != nil {
			t.Fatalf("set %s: %v", key, err)
		}
	}

	switch driver {
	case "sqlite":
		setEnv("DB_DRIVER", "sqlite")
		setEnv("DB_SUPPORTED_DRIVERS", "sqlite")
		setEnv("DB_DATABASE", filepath.Join(projectDir, "storage", "auth-integration.db"))
		return
	case "mysql":
		fixture := sharedAuthDatabaseFixture(t, "mysql")
		setAuthDatabaseEnv(t, setEnv, fixture)
		return
	case "postgres":
		fixture := sharedAuthDatabaseFixture(t, "postgres")
		setAuthDatabaseEnv(t, setEnv, fixture)
		return
	default:
		t.Fatalf("unsupported rendered auth driver %q", driver)
		return
	}
}

func setAuthDatabaseEnv(t *testing.T, setEnv func(string, string), fixture *authDatabaseFixture) {
	t.Helper()

	database := resetRenderedAuthDatabase(t, fixture)
	setEnv("DB_DRIVER", fixture.driver)
	setEnv("DB_SUPPORTED_DRIVERS", fixture.driver)
	setEnv("DB_HOST", fixture.host)
	setEnv("DB_PORT", fixture.port)
	setEnv("DB_DATABASE", database)
	setEnv("DB_USERNAME", fixture.username)
	setEnv("DB_PASSWORD", fixture.password)
}

func resetRenderedAuthDatabase(t *testing.T, fixture *authDatabaseFixture) string {
	t.Helper()

	switch fixture.driver {
	case "mysql":
		if err := resetRenderedMySQLAuthDatabase(fixture); err != nil {
			t.Fatalf("reset mysql auth database: %v", err)
		}
	case "postgres":
		if err := resetRenderedPostgresAuthDatabase(fixture); err != nil {
			t.Fatalf("reset postgres auth database: %v", err)
		}
	case "sqlite":
		return ""
	default:
		t.Fatalf("unsupported auth fixture driver %q", fixture.driver)
	}
	return "app"
}

func resetRenderedMySQLAuthDatabase(fixture *authDatabaseFixture) error {
	return runAuthContainerCommand(
		fixture,
		[]string{
			"sh", "-lc",
			`mysql -h 127.0.0.1 -uapp -p"$MYSQL_PASSWORD" app -e 'DROP TABLE IF EXISTS auth_sessions; DROP TABLE IF EXISTS users; DROP TABLE IF EXISTS migrations;'`,
		},
	)
}

func resetRenderedPostgresAuthDatabase(fixture *authDatabaseFixture) error {
	return runAuthContainerCommand(
		fixture,
		[]string{
			"sh", "-lc",
			`psql -U postgres -d postgres -v ON_ERROR_STOP=1 -tc "SELECT 1 FROM pg_database WHERE datname = 'app'" | grep -q 1 || psql -U postgres -d postgres -v ON_ERROR_STOP=1 -c 'CREATE DATABASE app'; psql -U postgres -d app -v ON_ERROR_STOP=1 -c 'DROP TABLE IF EXISTS auth_sessions, users, migrations CASCADE;'`,
		},
	)
}

func runAuthContainerCommand(fixture *authDatabaseFixture, cmd []string) error {
	deadline := time.Now().Add(20 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		exitCode, output, err := fixture.container.Exec(context.Background(), cmd)
		if err != nil {
			lastErr = err
			time.Sleep(250 * time.Millisecond)
			continue
		}
		if exitCode == 0 {
			return nil
		}
		body, _ := io.ReadAll(output)
		text := strings.TrimSpace(string(body))
		lastErr = fmt.Errorf("%s exec exit code %d: %s", fixture.driver, exitCode, text)
		if !isTransientAuthContainerResetError(text) {
			return lastErr
		}
		time.Sleep(250 * time.Millisecond)
	}
	return lastErr
}

func isTransientAuthContainerResetError(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "can't connect to mysql server") ||
		strings.Contains(lower, "database system is shutting down") ||
		strings.Contains(lower, "terminating connection due to administrator command") ||
		strings.Contains(lower, "server closed the connection unexpectedly")
}

func sharedAuthDatabaseFixture(t *testing.T, driver string) *authDatabaseFixture {
	t.Helper()

	state, ok := authDatabaseFixtures[driver]
	if !ok {
		t.Fatalf("unsupported shared auth database fixture %q", driver)
	}
	state.once.Do(func() {
		state.fixture, state.err = startSharedAuthDatabaseFixture(driver)
	})
	if state.err != nil {
		t.Fatalf("start %s auth database fixture: %v", driver, state.err)
	}
	return state.fixture
}

func startSharedAuthDatabaseFixture(driver string) (*authDatabaseFixture, error) {
	switch driver {
	case "mysql":
		return startSharedMySQLAuthFixture()
	case "postgres":
		return startSharedPostgresAuthFixture()
	default:
		return nil, os.ErrInvalid
	}
}

func startSharedMySQLAuthFixture() (*authDatabaseFixture, error) {
	started, err := testkit.StartTestcontainer(
		nil,
		testcontainers.ContainerRequest{
			Image:        "mysql:8.4",
			ExposedPorts: []string{"3306/tcp"},
			Env: map[string]string{
				"MYSQL_DATABASE":      "app",
				"MYSQL_USER":          "app",
				"MYSQL_PASSWORD":      "secret",
				"MYSQL_ROOT_PASSWORD": "rootsecret",
			},
			WaitingFor: wait.ForLog("ready for connections").WithStartupTimeout(90 * time.Second),
		},
		"3306/tcp",
		60*time.Second,
		"MySQL auth",
	)
	if err != nil {
		return nil, err
	}
	return &authDatabaseFixture{
		host:      started.Host,
		port:      started.Port,
		username:  "app",
		password:  "secret",
		driver:    "mysql",
		container: started.Container,
		stop:      started.Stop,
	}, nil
}

func startSharedPostgresAuthFixture() (*authDatabaseFixture, error) {
	started, err := testkit.StartTestcontainer(
		nil,
		testcontainers.ContainerRequest{
			Image:        "postgres:16-alpine",
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_DB":       "postgres",
				"POSTGRES_USER":     "postgres",
				"POSTGRES_PASSWORD": "secret",
			},
			WaitingFor: wait.ForLog("database system is ready to accept connections").WithStartupTimeout(60 * time.Second),
		},
		"5432/tcp",
		60*time.Second,
		"Postgres auth",
	)
	if err != nil {
		return nil, err
	}
	return &authDatabaseFixture{
		host:      started.Host,
		port:      started.Port,
		username:  "postgres",
		password:  "secret",
		driver:    "postgres",
		container: started.Container,
		stop:      started.Stop,
	}, nil
}

func assertStatus(t *testing.T, client *http.Client, method, url string, body any, want int) {
	t.Helper()
	resp, respBody := doJSONRequest(t, client, method, url, body)
	if resp.StatusCode != want {
		t.Fatalf("%s %s status = %d, want %d\nbody:\n%s", method, url, resp.StatusCode, want, respBody)
	}
}

func assertStatusBody(t *testing.T, client *http.Client, method, url string, body any, want int) string {
	t.Helper()
	resp, respBody := doJSONRequest(t, client, method, url, body)
	if resp.StatusCode != want {
		t.Fatalf("%s %s status = %d, want %d\nbody:\n%s", method, url, resp.StatusCode, want, respBody)
	}
	return respBody
}

func assertJSONStatus(t *testing.T, client *http.Client, method, url string, body any, want int) string {
	t.Helper()
	respBody := assertStatusBody(t, client, method, url, body, want)
	if !json.Valid([]byte(respBody)) {
		t.Fatalf("%s %s body is not valid JSON:\n%s", method, url, respBody)
	}
	return respBody
}

func doJSONRequest(t *testing.T, client *http.Client, method, url string, body any) (*http.Response, string) {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s failed: %v", method, url, err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return resp, string(responseBody)
}

func authCookieValue(t *testing.T, jar http.CookieJar, baseURL, name string) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, baseURL, nil)
	if err != nil {
		t.Fatalf("new cookie inspection request: %v", err)
	}
	for _, cookie := range jar.Cookies(req.URL) {
		if cookie.Name == name {
			return cookie.Value
		}
	}
	return ""
}

func waitForAuthProbeEndpointReady(proc *procHandle, url string, wantStatus int, timeout time.Duration) error {
	client := &http.Client{Timeout: 300 * time.Millisecond}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if proc != nil {
			if err := proc.ExitError(); err != nil {
				return err
			}
		}
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == wantStatus {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if proc != nil {
		return fmt.Errorf("probe %s did not return status %d before timeout\n%s", url, wantStatus, proc.Output())
	}
	return fmt.Errorf("probe %s did not return status %d before timeout", url, wantStatus)
}

func isTransientMySQLStartupError(err error) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	return strings.Contains(text, "driver: bad connection") ||
		strings.Contains(text, "unexpected EOF")
}
