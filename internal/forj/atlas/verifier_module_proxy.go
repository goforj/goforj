package atlas

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	verifierModuleProxyHealthTimeout     = 2 * time.Second
	verifierModuleProxyReadHeaderTimeout = 5 * time.Second
	verifierModuleProxyReadTimeout       = 15 * time.Second
	verifierModuleProxyWriteTimeout      = 30 * time.Second
	verifierModuleProxyIdleTimeout       = 60 * time.Second
	verifierModuleProxyMaxHeaderBytes    = 8 << 10
	verifierModuleProxyMaxConnections    = 64
	verifierModuleProxyMaxRequests       = 32
	verifierModuleProxyUpstreamTimeout   = 1 * time.Minute
)

var verifierModuleProxyHealthClient = &http.Client{
	Timeout: verifierModuleProxyHealthTimeout,
	Transport: &http.Transport{
		Proxy:             nil,
		DisableKeepAlives: true,
	},
}

// verifierModuleProxy exposes the prepared base's immutable download cache without disclosing its filesystem path to verifier children.
// It is not an OS security boundary for unconfined same-UID processes: those processes can still inspect host state.
// On an authoritative host-isolated backend, the proxy grants only read access to regular module files.
type verifierModuleProxy struct {
	listener  net.Listener
	server    *http.Server
	url       string
	prefix    string
	root      string
	rootDir   *os.Root
	requests  chan struct{}
	upstreams []verifierModuleProxyUpstream
	client    *http.Client
	close     sync.Once
	closeErr  error
}

// newVerifierModuleProxy starts one authority-scoped loopback proxy for a trusted prepared module cache.
func newVerifierModuleProxy(environment []string) (*verifierModuleProxy, error) {
	root := filepath.Join(evaluationEnvironmentValue(environment, "GOMODCACHE"), "cache", "download")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create trusted module download cache: %w", err)
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("trusted module download cache must be a directory without symlinks")
	}
	prefixBytes := make([]byte, 16)
	if _, err := rand.Read(prefixBytes); err != nil {
		return nil, fmt.Errorf("generate module proxy prefix: %w", err)
	}
	proxy := &verifierModuleProxy{
		prefix:    hex.EncodeToString(prefixBytes),
		root:      root,
		upstreams: verifierModuleProxyUpstreams(environment),
		client: &http.Client{Timeout: verifierModuleProxyUpstreamTimeout, Transport: &http.Transport{
			MaxIdleConns:        verifierModuleProxyMaxConnections,
			MaxIdleConnsPerHost: verifierModuleProxyMaxConnections,
			IdleConnTimeout:     verifierModuleProxyIdleTimeout,
		}},
	}
	rootDir, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("anchor trusted module download cache: %w", err)
	}
	proxy.rootDir = rootDir
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, errors.Join(fmt.Errorf("listen for verifier module proxy: %w", err), rootDir.Close())
	}
	proxy.listener = newLimitedVerifierModuleProxyListener(listener, verifierModuleProxyMaxConnections)
	proxy.url = "http://" + listener.Addr().String() + "/" + proxy.prefix
	proxy.requests = make(chan struct{}, verifierModuleProxyMaxRequests)
	proxy.server = &http.Server{
		Handler:           http.HandlerFunc(proxy.serveHTTP),
		ErrorLog:          log.New(io.Discard, "", 0),
		ReadHeaderTimeout: verifierModuleProxyReadHeaderTimeout,
		ReadTimeout:       verifierModuleProxyReadTimeout,
		WriteTimeout:      verifierModuleProxyWriteTimeout,
		IdleTimeout:       verifierModuleProxyIdleTimeout,
		MaxHeaderBytes:    verifierModuleProxyMaxHeaderBytes,
	}
	go func() { _ = proxy.server.Serve(proxy.listener) }()
	if err := proxy.Healthy(context.Background()); err != nil {
		return proxy, errors.Join(err, proxy.Close())
	}
	return proxy, nil
}

// URL returns the authority-scoped module proxy URL for verifier child environments.
func (proxy *verifierModuleProxy) URL() string { return proxy.url }

// Healthy verifies that the loopback endpoint remains available at a treatment boundary.
func (proxy *verifierModuleProxy) Healthy(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, verifierModuleProxyHealthTimeout)
		defer cancel()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, proxy.url+"/health", nil)
	if err != nil {
		return err
	}
	response, err := verifierModuleProxyHealthClient.Do(request)
	if err != nil {
		return fmt.Errorf("check verifier module proxy: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("check verifier module proxy: status %d", response.StatusCode)
	}
	return nil
}

// Close stops the loopback endpoint once all authority-owned verifiers have stopped.
func (proxy *verifierModuleProxy) Close() error {
	if proxy == nil {
		return nil
	}
	proxy.close.Do(func() { proxy.closeErr = errors.Join(proxy.server.Close(), proxy.rootDir.Close()) })
	return proxy.closeErr
}

// serveHTTP limits the proxy to GET and HEAD requests for regular, non-symlinked files below the trusted download root.
func (proxy *verifierModuleProxy) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	select {
	case proxy.requests <- struct{}{}:
		defer func() { <-proxy.requests }()
	default:
		http.Error(writer, "service unavailable", http.StatusServiceUnavailable)
		return
	}
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		writer.Header().Set("Allow", "GET, HEAD")
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if request.URL.Path == "/"+proxy.prefix+"/health" {
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	prefix := "/" + proxy.prefix + "/"
	if !strings.HasPrefix(request.URL.EscapedPath(), prefix) {
		http.NotFound(writer, request)
		return
	}
	relative, err := url.PathUnescape(strings.TrimPrefix(request.URL.EscapedPath(), prefix))
	if err != nil || relative == "" || !filepath.IsLocal(filepath.FromSlash(relative)) {
		http.NotFound(writer, request)
		return
	}
	file, err := proxy.openRegular(filepath.FromSlash(relative))
	if err != nil {
		proxy.forwardUpstream(writer, request, relative, strings.TrimPrefix(request.URL.EscapedPath(), prefix))
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(writer, request)
		return
	}
	writer.Header().Set("Content-Length", fmt.Sprint(info.Size()))
	if request.Method == http.MethodGet {
		_, _ = io.Copy(writer, file)
	}
}

// verifierModuleProxyUpstream retains one declared HTTP endpoint and the fallback semantics following it.
type verifierModuleProxyUpstream struct {
	endpoint      *url.URL
	fallbackOnAny bool
}

// verifierModuleProxyUpstreams freezes the declared HTTP(S) GOPROXY prefix for authority-owned cold-cache requests.
func verifierModuleProxyUpstreams(environment []string) []verifierModuleProxyUpstream {
	value := strings.TrimSpace(evaluationEnvironmentValue(environment, "GOPROXY"))
	upstreams := make([]verifierModuleProxyUpstream, 0, 2)
	for value != "" {
		entry := value
		separator := byte(0)
		if index := strings.IndexAny(value, ",|"); index >= 0 {
			entry = value[:index]
			separator = value[index]
			value = value[index+1:]
		} else {
			value = ""
		}
		entry = strings.TrimSpace(entry)
		if entry == "direct" || entry == "off" {
			break
		}
		candidate, err := url.Parse(entry)
		if err == nil && (candidate.Scheme == "http" || candidate.Scheme == "https") && candidate.Host != "" {
			upstreams = append(upstreams, verifierModuleProxyUpstream{endpoint: candidate, fallbackOnAny: separator == '|'})
		}
	}
	return upstreams
}

// forwardUpstream serves a valid cache miss through the authority-owned client without revealing upstream details to a verifier child.
func (proxy *verifierModuleProxy) forwardUpstream(writer http.ResponseWriter, request *http.Request, relativePath, escapedPath string) {
	if len(proxy.upstreams) == 0 || proxy.client == nil {
		http.NotFound(writer, request)
		return
	}
	for index, upstream := range proxy.upstreams {
		target := *upstream.endpoint
		target.Path = strings.TrimRight(target.Path, "/") + "/" + strings.TrimLeft(relativePath, "/")
		target.RawPath = strings.TrimRight(target.EscapedPath(), "/") + "/" + strings.TrimLeft(escapedPath, "/")
		upstreamRequest, err := http.NewRequestWithContext(request.Context(), request.Method, target.String(), nil)
		if err != nil {
			http.NotFound(writer, request)
			return
		}
		response, err := proxy.client.Do(upstreamRequest)
		last := index == len(proxy.upstreams)-1
		if err != nil {
			if upstream.fallbackOnAny && !last {
				continue
			}
			http.NotFound(writer, request)
			return
		}
		fallback := !last && (upstream.fallbackOnAny || response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusGone)
		if fallback {
			_ = response.Body.Close()
			continue
		}
		defer response.Body.Close()
		if contentType := response.Header.Get("Content-Type"); contentType != "" {
			writer.Header().Set("Content-Type", contentType)
		}
		if response.ContentLength >= 0 {
			writer.Header().Set("Content-Length", fmt.Sprint(response.ContentLength))
		}
		writer.WriteHeader(response.StatusCode)
		if request.Method == http.MethodGet {
			_, _ = io.Copy(writer, response.Body)
		}
		return
	}
	http.NotFound(writer, request)
}

// limitedVerifierModuleProxyListener bounds connection descriptors held by loopback clients.
type limitedVerifierModuleProxyListener struct {
	net.Listener
	connections chan struct{}
	done        chan struct{}
	close       sync.Once
}

// newLimitedVerifierModuleProxyListener limits concurrent accepted connections to limit.
func newLimitedVerifierModuleProxyListener(listener net.Listener, limit int) *limitedVerifierModuleProxyListener {
	return &limitedVerifierModuleProxyListener{Listener: listener, connections: make(chan struct{}, limit), done: make(chan struct{})}
}

// Accept waits for capacity before accepting a connection so excess clients cannot consume unbounded descriptors.
func (listener *limitedVerifierModuleProxyListener) Accept() (net.Conn, error) {
	select {
	case listener.connections <- struct{}{}:
	case <-listener.done:
		return nil, net.ErrClosed
	}
	connection, err := listener.Listener.Accept()
	if err != nil {
		<-listener.connections
		return nil, err
	}
	return &limitedVerifierModuleProxyConnection{Conn: connection, release: func() { <-listener.connections }}, nil
}

// Close releases an Accept waiting for connection capacity before closing the underlying listener.
func (listener *limitedVerifierModuleProxyListener) Close() error {
	listener.close.Do(func() { close(listener.done) })
	return listener.Listener.Close()
}

// limitedVerifierModuleProxyConnection returns its listener capacity exactly once when closed.
type limitedVerifierModuleProxyConnection struct {
	net.Conn
	release func()
	once    sync.Once
}

// Close releases the listener capacity after closing the underlying connection.
func (connection *limitedVerifierModuleProxyConnection) Close() error {
	err := connection.Conn.Close()
	connection.once.Do(connection.release)
	return err
}

// openRegular rejects every symlinked path component before opening the requested module artifact.
func (proxy *verifierModuleProxy) openRegular(relative string) (*os.File, error) {
	path := proxy.root
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		path = filepath.Join(path, component)
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("invalid module proxy path")
		}
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("module proxy target is not regular")
	}
	file, err := proxy.rootDir.Open(relative)
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		_ = file.Close()
		return nil, errors.New("module proxy target changed while opening")
	}
	return file, nil
}

// evaluationEnvironmentValue returns the final value for a named entry in an explicit process environment.
func evaluationEnvironmentValue(environment []string, name string) string {
	for index := len(environment) - 1; index >= 0; index-- {
		key, value, ok := strings.Cut(environment[index], "=")
		if ok && key == name {
			return value
		}
	}
	return ""
}
