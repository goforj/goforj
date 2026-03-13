//go:build integration_generator

package integration_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"database/sql"
	"fmt"
	"io"
	"log"
	"net"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/docker/go-connections/nat"
	"github.com/fsouza/fake-gcs-server/fakestorage"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/goforj/goforj/internal/generate"
	"github.com/goftp/server"
	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
	"github.com/pkg/sftp"
	testcontainers "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"golang.org/x/crypto/ssh"
)

var (
	sharedContainersMu sync.Mutex
	sharedContainers   = map[string]startedContainer{}
)

const cacheGeneratorIntegrationGoMod = `module example.com/cachegeneratorintegration

go 1.24

require (
	github.com/goforj/cache v0.1.5
	github.com/goforj/cache/cachecore v0.1.5
	github.com/goforj/cache/cachetest v0.1.5
	github.com/goforj/cache/driver/dynamocache v0.1.5
	github.com/goforj/cache/driver/memcachedcache v0.1.5
	github.com/goforj/cache/driver/mysqlcache v0.1.5
	github.com/goforj/cache/driver/natscache v0.1.5
	github.com/goforj/cache/driver/postgrescache v0.1.5
	github.com/goforj/cache/driver/rediscache v0.1.5
	github.com/goforj/cache/driver/sqlcore v0.1.5
	github.com/goforj/cache/driver/sqlitecache v0.1.5
	github.com/goforj/env/v2 v2.3.0
	github.com/goforj/str v1.2.0
)
`

const storageGeneratorIntegrationGoMod = `module example.com/storagegeneratorintegration

go 1.24

require (
	github.com/goforj/env/v2 v2.3.0
	github.com/goforj/storage v0.2.5
	github.com/goforj/storage/driver/ftpstorage v0.2.5
	github.com/goforj/storage/driver/gcsstorage v0.2.5
	github.com/goforj/storage/driver/localstorage v0.2.5
	github.com/goforj/storage/driver/memorystorage v0.2.5
	github.com/goforj/storage/driver/rclonestorage v0.2.5
	github.com/goforj/storage/driver/redisstorage v0.2.5
	github.com/goforj/storage/driver/s3storage v0.2.5
	github.com/goforj/storage/driver/sftpstorage v0.2.5
	github.com/goforj/str v1.2.0
)
`

const cacheGeneratorSmokeTestSource = `package cache

import "testing"

func TestGeneratedCacheSmoke(t *testing.T) {
	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	cases := []struct {
		name    string
		wantHit bool
	}{
		{name: "default", wantHit: true},
		{name: "file", wantHit: true},
		{name: "null", wantHit: false},
		{name: "redis", wantHit: true},
		{name: "memcached", wantHit: true},
		{name: "dynamo", wantHit: true},
		{name: "sqlite", wantHit: true},
		{name: "postgres", wantHit: true},
		{name: "mysql", wantHit: true},
		{name: "nats", wantHit: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var store = mgr.Default()
			if tc.name != "default" {
				store = mgr.mustStore(tc.name)
			}

			if err := store.Ready(); err != nil {
				t.Fatalf("Ready: %v", err)
			}

			key := "smoke/" + tc.name
			want := "value-" + tc.name
			if err := store.SetString(key, want, 0); err != nil {
				t.Fatalf("SetString: %v", err)
			}

			got, ok, err := store.GetString(key)
			if err != nil {
				t.Fatalf("GetString: %v", err)
			}
			if tc.wantHit {
				if !ok {
					t.Fatal("expected cache hit")
				}
				if got != want {
					t.Fatalf("GetString = %q, want %q", got, want)
				}
				t.Logf("driver %s passed", tc.name)
				return
			}
			if ok {
				t.Fatalf("expected cache miss, got %q", got)
			}
			t.Logf("driver %s passed", tc.name)
		})
	}
}
`

const storageGeneratorSmokeTestSource = `package storage

import "testing"

func TestGeneratedStorageSmoke(t *testing.T) {
	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	names := []string{
		"default",
		"memory",
		"redis",
		"ftp",
		"sftp",
		"s3",
		"gcs",
		"rclone",
	}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			disk := mgr.Default()
			if name != "default" {
				disk = mgr.mustDisk(name)
			}

			path := "smoke/" + name + ".txt"
			want := []byte("value-" + name)
			if err := disk.Put(path, want); err != nil {
				t.Fatalf("Put: %v", err)
			}
			got, err := disk.Get(path)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if string(got) != string(want) {
				t.Fatalf("Get = %q, want %q", got, want)
			}
			exists, err := disk.Exists(path)
			if err != nil {
				t.Fatalf("Exists: %v", err)
			}
			if !exists {
				t.Fatal("expected stored object to exist")
			}
			t.Logf("driver %s passed", name)
		})
	}
}
`

func TestMain(m *testing.M) {
	code := m.Run()
	terminateSharedContainers()
	os.Exit(code)
}

func TestGenerateCacheFilesIntegrationSmoke(t *testing.T) {
	ctx := context.Background()
	redis := startRedisContainer(t, ctx)
	memcached := startMemcachedContainer(t, ctx)
	dynamo := startDynamoDBContainer(t, ctx)
	postgres := startPostgresContainer(t, ctx)
	mysql := startMySQLContainer(t, ctx)
	nats := startNATSContainer(t, ctx)

	fileDir := filepath.Join(t.TempDir(), "cache-file")
	sqlitePath := filepath.Join(t.TempDir(), "cache.sqlite3")

	envs := map[string]string{
		"CACHE_DRIVER":                  "memory",
		"CACHE_DEFAULT_TTL_SECONDS":     "60",
		"CACHE_FILE_DRIVER":             "file",
		"CACHE_FILE_FILE_DIR":           fileDir,
		"CACHE_NULL_DRIVER":             "null",
		"CACHE_REDIS_DRIVER":            "redis",
		"CACHE_REDIS_ADDR":              redis.addr,
		"CACHE_MEMCACHED_DRIVER":        "memcached",
		"CACHE_MEMCACHED_ADDRESSES":     memcached.addr,
		"CACHE_DYNAMO_DRIVER":           "dynamodb",
		"CACHE_DYNAMO_REGION":           "us-east-1",
		"CACHE_DYNAMO_ENDPOINT":         dynamo.endpoint,
		"CACHE_DYNAMO_TABLE":            "cache_entries",
		"CACHE_SQLITE_DRIVER":           "sqlite",
		"CACHE_SQLITE_DSN":              sqlitePath,
		"CACHE_SQLITE_TABLE":            "cache_entries",
		"CACHE_POSTGRES_DRIVER":         "postgres",
		"CACHE_POSTGRES_DSN":            postgres.dsn,
		"CACHE_POSTGRES_TABLE":          "cache_entries",
		"CACHE_MYSQL_DRIVER":            "mysql",
		"CACHE_MYSQL_DSN":               mysql.dsn,
		"CACHE_MYSQL_TABLE":             "cache_entries",
		"CACHE_NATS_DRIVER":             "nats",
		"CACHE_NATS_URL":                nats.url,
		"CACHE_NATS_BUCKET":             "CACHE_GENERATOR_SMOKE",
		"CACHE_NATS_STORAGE":            "memory",
		"CACHE_NATS_HISTORY":            "1",
		"CACHE_NATS_REPLICAS":           "1",
		"CACHE_NATS_MAX_BYTES":          "1048576",
		"CACHE_NATS_MAX_VALUE_SIZE":     "4096",
		"CACHE_NATS_BUCKET_TTL":         "false",
		"CACHE_NATS_BUCKET_TTL_SECONDS": "0",
		"CACHE_NATS_DESCRIPTION":        "generator smoke bucket",
		"CACHE_NATS_COMPRESSED":         "false",
	}
	setEnv(t, envs)

	root := newTempModule(t, ".tmp-cache-generator-integration-*")
	mkdirAll(t, filepath.Join(root, "internal", "cache"))
	writeFile(t, filepath.Join(root, "go.mod"), cacheGeneratorIntegrationGoMod)
	writeFile(t, filepath.Join(root, "internal", "cache", "manager.go"), string(loadCacheManagerFixture(t)))

	if written, err := generate.GenerateCacheFiles(root); err != nil {
		t.Fatalf("GenerateCacheFiles: %v", err)
	} else if written == 0 {
		t.Fatal("expected generated cache files to be written")
	}
	writeFile(t, filepath.Join(root, "internal", "cache", "generator_smoke_test.go"), cacheGeneratorSmokeTestSource)

	t.Log("running cache generated smoke test")
	runGoCommand(t, root, envs, "test", "-mod=mod", "./internal/cache", "-run", "TestGeneratedCacheSmoke", "-count=1", "-v")
}

func TestGenerateStorageFilesIntegrationSmoke(t *testing.T) {
	ctx := context.Background()
	redis := startRedisContainer(t, ctx)
	ftpServer := startEmbeddedFTPServer(t)
	sftpServer := startEmbeddedSFTPServer(t)
	s3Server := startFakeS3Server(t)
	gcsServer := startFakeGCSServer(t)
	rcloneRoot := t.TempDir()
	defaultRoot := filepath.Join(t.TempDir(), "default-storage")
	envs := map[string]string{
		"STORAGE_DRIVER":                        "local",
		"STORAGE_ROOT":                          defaultRoot,
		"STORAGE_MEMORY_DRIVER":                 "memory",
		"STORAGE_REDIS_DRIVER":                  "redis",
		"STORAGE_REDIS_ADDR":                    redis.addr,
		"STORAGE_FTP_DRIVER":                    "ftp",
		"STORAGE_FTP_HOST":                      ftpServer.host,
		"STORAGE_FTP_PORT":                      ftpServer.port,
		"STORAGE_FTP_USER":                      ftpServer.user,
		"STORAGE_FTP_PASSWORD":                  ftpServer.password,
		"STORAGE_SFTP_DRIVER":                   "sftp",
		"STORAGE_SFTP_HOST":                     sftpServer.host,
		"STORAGE_SFTP_PORT":                     sftpServer.port,
		"STORAGE_SFTP_USER":                     sftpServer.user,
		"STORAGE_SFTP_PASSWORD":                 sftpServer.password,
		"STORAGE_SFTP_INSECURE_IGNORE_HOST_KEY": "true",
		"STORAGE_S3_DRIVER":                     "s3",
		"STORAGE_S3_BUCKET":                     s3Server.bucket,
		"STORAGE_S3_ENDPOINT":                   s3Server.endpoint,
		"STORAGE_S3_REGION":                     "us-east-1",
		"STORAGE_S3_ACCESS_KEY_ID":              "access",
		"STORAGE_S3_SECRET_ACCESS_KEY":          "secret",
		"STORAGE_S3_USE_PATH_STYLE":             "true",
		"STORAGE_GCS_DRIVER":                    "gcs",
		"STORAGE_GCS_BUCKET":                    gcsServer.bucket,
		"STORAGE_GCS_ENDPOINT":                  gcsServer.endpoint,
		"STORAGE_RCLONE_DRIVER":                 "rclone",
		"STORAGE_RCLONE_REMOTE":                 "localdisk:" + rcloneRoot,
		"STORAGE_RCLONE_RCLONE_CONFIG_DATA":     "[localdisk]\ntype = local\n",
	}
	setEnv(t, envs)

	root := newTempModule(t, ".tmp-storage-generator-integration-*")
	mkdirAll(t, filepath.Join(root, "internal", "storage"))
	writeFile(t, filepath.Join(root, "go.mod"), storageGeneratorIntegrationGoMod)
	writeFile(t, filepath.Join(root, "internal", "storage", "manager.go"), string(loadStorageManagerFixture(t)))

	if written, err := generate.GenerateStorageFiles(root); err != nil {
		t.Fatalf("GenerateStorageFiles: %v", err)
	} else if written == 0 {
		t.Fatal("expected generated storage files to be written")
	}
	writeFile(t, filepath.Join(root, "internal", "storage", "generator_smoke_test.go"), storageGeneratorSmokeTestSource)

	t.Log("running storage generated smoke test")
	runGoCommand(t, root, envs, "test", "-mod=mod", "./internal/storage", "-run", "TestGeneratedStorageSmoke", "-count=1", "-v")
}

type startedContainer struct {
	container testcontainers.Container
	host      string
	port      string
	addr      string
	endpoint  string
	url       string
	dsn       string
}

type startedFTPServer struct {
	server   *server.Server
	host     string
	port     string
	user     string
	password string
}

type startedSFTPServer struct {
	listener net.Listener
	host     string
	port     string
	user     string
	password string
}

type startedS3Server struct {
	server   *httptest.Server
	endpoint string
	bucket   string
}

type startedGCSServer struct {
	server   *fakestorage.Server
	endpoint string
	bucket   string
}

func startRedisContainer(t *testing.T, ctx context.Context) startedContainer {
	t.Helper()
	return startSharedContainer(t, ctx, "redis", testcontainers.ContainerRequest{
		Image:        "redis:7-alpine",
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor:   wait.ForListeningPort("6379/tcp").WithStartupTimeout(30 * time.Second),
	}, "6379/tcp")
}

func startMemcachedContainer(t *testing.T, ctx context.Context) startedContainer {
	t.Helper()
	return startGenericContainer(t, ctx, testcontainers.ContainerRequest{
		Image:        "memcached:1.6-alpine",
		ExposedPorts: []string{"11211/tcp"},
		WaitingFor:   wait.ForListeningPort("11211/tcp").WithStartupTimeout(30 * time.Second),
	}, "11211/tcp")
}

func startDynamoDBContainer(t *testing.T, ctx context.Context) startedContainer {
	t.Helper()
	started := startGenericContainer(t, ctx, testcontainers.ContainerRequest{
		Image:        "amazon/dynamodb-local:2.5.4",
		ExposedPorts: []string{"8000/tcp"},
		WaitingFor:   wait.ForListeningPort("8000/tcp").WithStartupTimeout(30 * time.Second),
	}, "8000/tcp")
	started.endpoint = "http://" + started.addr
	return started
}

func startPostgresContainer(t *testing.T, ctx context.Context) startedContainer {
	t.Helper()
	started := startGenericContainer(t, ctx, testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_DB":       "app",
			"POSTGRES_USER":     "app",
			"POSTGRES_PASSWORD": "secret",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").WithStartupTimeout(60 * time.Second),
	}, "5432/tcp")
	started.dsn = fmt.Sprintf("postgres://app:secret@%s/app?sslmode=disable", started.addr)
	return started
}

func startMySQLContainer(t *testing.T, ctx context.Context) startedContainer {
	t.Helper()
	started := startGenericContainer(t, ctx, testcontainers.ContainerRequest{
		Image:        "mysql:8.4",
		ExposedPorts: []string{"3306/tcp"},
		Env: map[string]string{
			"MYSQL_DATABASE":      "app",
			"MYSQL_USER":          "app",
			"MYSQL_PASSWORD":      "secret",
			"MYSQL_ROOT_PASSWORD": "rootsecret",
		},
		WaitingFor: wait.ForLog("ready for connections").WithStartupTimeout(90 * time.Second),
	}, "3306/tcp")
	started.dsn = fmt.Sprintf("app:secret@tcp(%s)/app?parseTime=true", started.addr)
	waitForMySQLReady(t, started.dsn)
	return started
}

func waitForMySQLReady(t *testing.T, dsn string) {
	t.Helper()

	_ = mysqlDriver.SetLogger(&mysqlDriver.NopLogger{})
	defer func() {
		_ = mysqlDriver.SetLogger(log.New(os.Stderr, "[mysql] ", log.Ldate|log.Ltime))
	}()

	deadline := time.Now().Add(30 * time.Second)
	for {
		db, err := sql.Open("mysql", dsn)
		if err == nil {
			err = db.Ping()
			_ = db.Close()
		}
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("wait for mysql ready: %v", err)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func startNATSContainer(t *testing.T, ctx context.Context) startedContainer {
	t.Helper()
	started := startGenericContainer(t, ctx, testcontainers.ContainerRequest{
		Image:        "nats:2.10-alpine",
		Cmd:          []string{"-js"},
		ExposedPorts: []string{"4222/tcp"},
		WaitingFor:   wait.ForListeningPort("4222/tcp").WithStartupTimeout(30 * time.Second),
	}, "4222/tcp")
	started.url = "nats://" + started.addr
	return started
}

func startGenericContainer(t *testing.T, ctx context.Context, req testcontainers.ContainerRequest, port string) startedContainer {
	t.Helper()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("start container %s: %v", req.Image, err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Fatalf("terminate container %s: %v", req.Image, err)
		}
	})

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("container host %s: %v", req.Image, err)
	}
	mapped, err := container.MappedPort(ctx, nat.Port(port))
	if err != nil {
		t.Fatalf("container mapped port %s: %v", req.Image, err)
	}
	return startedContainer{
		container: container,
		host:      host,
		port:      mapped.Port(),
		addr:      net.JoinHostPort(host, mapped.Port()),
	}
}

func startSharedContainer(t *testing.T, ctx context.Context, key string, req testcontainers.ContainerRequest, port string) startedContainer {
	t.Helper()

	sharedContainersMu.Lock()
	started, ok := sharedContainers[key]
	sharedContainersMu.Unlock()
	if ok {
		return started
	}

	started = startGenericContainerWithoutCleanup(t, ctx, req, port)

	sharedContainersMu.Lock()
	defer sharedContainersMu.Unlock()
	if existing, ok := sharedContainers[key]; ok {
		_ = started.container.Terminate(context.Background())
		return existing
	}
	sharedContainers[key] = started
	return started
}

func startGenericContainerWithoutCleanup(t *testing.T, ctx context.Context, req testcontainers.ContainerRequest, port string) startedContainer {
	t.Helper()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("start container %s: %v", req.Image, err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(context.Background())
		t.Fatalf("container host %s: %v", req.Image, err)
	}
	mapped, err := container.MappedPort(ctx, nat.Port(port))
	if err != nil {
		_ = container.Terminate(context.Background())
		t.Fatalf("container mapped port %s: %v", req.Image, err)
	}
	return startedContainer{
		container: container,
		host:      host,
		port:      mapped.Port(),
		addr:      net.JoinHostPort(host, mapped.Port()),
	}
}

func terminateSharedContainers() {
	sharedContainersMu.Lock()
	defer sharedContainersMu.Unlock()

	for key, started := range sharedContainers {
		if err := started.container.Terminate(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "terminate shared container %s: %v\n", key, err)
		}
	}
}

func startEmbeddedFTPServer(t *testing.T) startedFTPServer {
	t.Helper()
	root := t.TempDir()
	port := pickPort(t)
	serverOpts := &server.ServerOpts{
		Factory:  &ftpDriverFactory{root: root},
		Port:     port,
		Hostname: "127.0.0.1",
		Auth:     &server.SimpleAuth{Name: "anonymous", Password: "anonymous"},
	}
	ftpServer := server.NewServer(serverOpts)
	go func() {
		_ = ftpServer.ListenAndServe()
	}()
	t.Cleanup(func() {
		_ = ftpServer.Shutdown()
	})
	time.Sleep(200 * time.Millisecond)
	return startedFTPServer{
		server:   ftpServer,
		host:     "127.0.0.1",
		port:     fmt.Sprintf("%d", port),
		user:     "anonymous",
		password: "anonymous",
	}
}

func startEmbeddedSFTPServer(t *testing.T) startedSFTPServer {
	t.Helper()
	root := t.TempDir()
	signer, err := generateSigner()
	if err != nil {
		t.Fatalf("generate signer: %v", err)
	}
	serverConfig := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if c.User() == "test" && string(pass) == "test" {
				return nil, nil
			}
			return nil, fmt.Errorf("permission denied")
		},
	}
	serverConfig.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen sftp: %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
	})
	go acceptSFTPConnections(t, listener, serverConfig, root)
	time.Sleep(100 * time.Millisecond)
	return startedSFTPServer{
		listener: listener,
		host:     "127.0.0.1",
		port:     fmt.Sprintf("%d", listener.Addr().(*net.TCPAddr).Port),
		user:     "test",
		password: "test",
	}
}

func startFakeS3Server(t *testing.T) startedS3Server {
	t.Helper()
	fake := gofakes3.New(s3mem.New())
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen fake s3: %v", err)
	}
	server := httptest.NewUnstartedServer(fake.Server())
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)

	const bucket = "generator-smoke"
	ensureS3Bucket(t, server.URL, bucket)
	return startedS3Server{
		server:   server,
		endpoint: server.URL,
		bucket:   bucket,
	}
}

func startFakeGCSServer(t *testing.T) startedGCSServer {
	t.Helper()
	host := "127.0.0.1"
	port := uint16(pickPort(t))
	server, err := fakestorage.NewServerWithOptions(fakestorage.Options{
		Scheme:     "http",
		Host:       host,
		Port:       port,
		PublicHost: fmt.Sprintf("%s:%d", host, port),
	})
	if err != nil {
		t.Fatalf("start fake gcs server: %v", err)
	}
	t.Cleanup(server.Stop)

	const bucket = "generator-smoke"
	server.CreateBucketWithOpts(fakestorage.CreateBucketOpts{Name: bucket})
	return startedGCSServer{
		server:   server,
		endpoint: server.URL(),
		bucket:   bucket,
	}
}

func ensureS3Bucket(t *testing.T, endpoint, bucket string) {
	t.Helper()
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{URL: endpoint, HostnameImmutable: true}, nil
			},
		)),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("access", "secret", "")),
	)
	if err != nil {
		t.Fatalf("load fake s3 config: %v", err)
	}
	client := awss3.NewFromConfig(cfg, func(o *awss3.Options) { o.UsePathStyle = true })
	if _, err := client.CreateBucket(context.Background(), &awss3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		t.Fatalf("create fake s3 bucket: %v", err)
	}
}

func acceptSFTPConnections(t *testing.T, listener net.Listener, cfg *ssh.ServerConfig, root string) {
	t.Helper()
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			_, chans, reqs, err := ssh.NewServerConn(c, cfg)
			if err != nil {
				_ = c.Close()
				return
			}
			go ssh.DiscardRequests(reqs)
			handleSFTPChannels(t, chans, root)
		}(conn)
	}
}

func handleSFTPChannels(t *testing.T, chans <-chan ssh.NewChannel, root string) {
	t.Helper()
	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}
		channel, requests, err := newChannel.Accept()
		if err != nil {
			continue
		}
		go func(in <-chan *ssh.Request) {
			for req := range in {
				if req.Type != "subsystem" {
					req.Reply(false, nil)
					continue
				}
				var payload struct {
					Name string
				}
				if err := ssh.Unmarshal(req.Payload, &payload); err != nil || payload.Name != "sftp" {
					req.Reply(false, nil)
					continue
				}
				req.Reply(true, nil)
				sftpServer, err := sftp.NewServer(channel, sftp.WithServerWorkingDirectory(root))
				if err != nil {
					return
				}
				if err := sftpServer.Serve(); err == io.EOF {
					_ = sftpServer.Close()
				}
				return
			}
		}(requests)
	}
}

func generateSigner() (ssh.Signer, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	return ssh.NewSignerFromKey(key)
}

type ftpDriverFactory struct {
	root string
}

func (f *ftpDriverFactory) NewDriver() (server.Driver, error) {
	return &ftpMemDriver{root: f.root, perm: server.NewSimplePerm("user", "group")}, nil
}

type ftpMemDriver struct {
	root string
	perm server.Perm
}

func (d *ftpMemDriver) Init(*server.Conn) {}

func (d *ftpMemDriver) Stat(p string) (server.FileInfo, error) {
	info, err := os.Stat(d.abs(p))
	if err != nil {
		return nil, err
	}
	return ftpFileInfo{FileInfo: info}, nil
}

func (d *ftpMemDriver) ChangeDir(p string) error {
	info, err := os.Stat(d.abs(p))
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return os.ErrInvalid
	}
	return nil
}

func (d *ftpMemDriver) ListDir(p string, cb func(server.FileInfo) error) error {
	entries, err := os.ReadDir(d.abs(p))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if err := cb(ftpFileInfo{FileInfo: info}); err != nil {
			return err
		}
	}
	return nil
}

func (d *ftpMemDriver) DeleteDir(p string) error {
	return os.RemoveAll(d.abs(p))
}

func (d *ftpMemDriver) DeleteFile(p string) error {
	return os.Remove(d.abs(p))
}

func (d *ftpMemDriver) Rename(from, to string) error {
	return os.Rename(d.abs(from), d.abs(to))
}

func (d *ftpMemDriver) MakeDir(p string) error {
	return os.MkdirAll(d.abs(p), 0o755)
}

func (d *ftpMemDriver) GetFile(p string, _ int64) (int64, io.ReadCloser, error) {
	file, err := os.Open(d.abs(p))
	if err != nil {
		return 0, nil, err
	}
	info, _ := file.Stat()
	return info.Size(), file, nil
}

func (d *ftpMemDriver) PutFile(p string, r io.Reader, _ bool) (int64, error) {
	full := d.abs(p)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return 0, err
	}
	file, err := os.Create(full)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	return io.Copy(file, r)
}

func (d *ftpMemDriver) abs(p string) string {
	if p == "" || p == "." {
		return d.root
	}
	return filepath.Join(d.root, p)
}

type ftpFileInfo struct {
	os.FileInfo
}

func (f ftpFileInfo) Owner() string { return "user" }
func (f ftpFileInfo) Group() string { return "group" }

func pickPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("pick port: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func setEnv(t *testing.T, envs map[string]string) {
	t.Helper()
	for key, value := range envs {
		t.Setenv(key, value)
	}
}

func newTempModule(t *testing.T, pattern string) string {
	t.Helper()
	root, err := os.MkdirTemp(repoRoot(t), pattern)
	if err != nil {
		t.Fatalf("mkdir temp module: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(root)
	})
	return root
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runGoCommand(t *testing.T, dir string, envs map[string]string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GOCACHE=/tmp/gocache",
		"GOMODCACHE=/tmp/gomodcache",
		"GOWORK=off",
	)
	for key, value := range envs {
		cmd.Env = append(cmd.Env, key+"="+value)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe for go %s: %v", strings.Join(args, " "), err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe for go %s: %v", strings.Join(args, " "), err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start go %s: %v", strings.Join(args, " "), err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(os.Stdout, stdout)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(os.Stderr, stderr)
	}()

	err = cmd.Wait()
	wg.Wait()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("go %s timed out after 5m", strings.Join(args, " "))
	}
	if err != nil {
		t.Fatalf("go %s failed: %v", strings.Join(args, " "), err)
	}
}

func loadCacheManagerFixture(t *testing.T) []byte {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve test file path")
	}
	fixturePath := filepath.Join(filepath.Dir(currentFile), "..", "internal", "forj", "internal", "cache", "manager.go")
	content, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read cache manager fixture: %v", err)
	}
	return content
}

func loadStorageManagerFixture(t *testing.T) []byte {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve test file path")
	}
	fixturePath := filepath.Join(filepath.Dir(currentFile), "..", "internal", "forj", "internal", "storage", "manager.go")
	content, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read storage manager fixture: %v", err)
	}
	return content
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve test file path")
	}
	return filepath.Join(filepath.Dir(currentFile), "..")
}
