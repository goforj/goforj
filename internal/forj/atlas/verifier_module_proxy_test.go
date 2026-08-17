package atlas

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestVerifierModuleProxyServesOnlyTrustedRegularFiles verifies the proxy's Go module protocol surface without exposing its backing path.
func TestVerifierModuleProxyServesOnlyTrustedRegularFiles(t *testing.T) {
	cache := t.TempDir()
	download := filepath.Join(cache, "cache", "download")
	artifact := filepath.Join(download, "example.com", "module", "@v", "v1.0.0.mod")
	if err := os.MkdirAll(filepath.Dir(artifact), 0o700); err != nil {
		t.Fatal(err)
	}
	want := []byte("module example.com/module\n")
	if err := os.WriteFile(artifact, want, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(artifact, filepath.Join(download, "linked.mod")); err != nil {
		t.Fatal(err)
	}
	proxy, err := newVerifierModuleProxy([]string{"GOMODCACHE=" + cache})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = proxy.Close() })
	if strings.Contains(proxy.URL(), cache) || strings.Contains(proxy.URL(), "file:") {
		t.Fatalf("proxy URL exposes backing cache: %q", proxy.URL())
	}
	if proxy.server.ReadHeaderTimeout != verifierModuleProxyReadHeaderTimeout ||
		proxy.server.ReadTimeout != verifierModuleProxyReadTimeout ||
		proxy.server.WriteTimeout != verifierModuleProxyWriteTimeout ||
		proxy.server.IdleTimeout != verifierModuleProxyIdleTimeout ||
		proxy.server.MaxHeaderBytes != verifierModuleProxyMaxHeaderBytes {
		t.Fatalf("proxy server limits = %#v, want configured bounded timeouts and headers", proxy.server)
	}
	if cap(proxy.requests) != verifierModuleProxyMaxRequests {
		t.Fatalf("proxy request capacity = %d, want %d", cap(proxy.requests), verifierModuleProxyMaxRequests)
	}
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		request, err := http.NewRequest(method, proxy.URL()+"/example.com/module/@v/v1.0.0.mod", nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			t.Fatalf("%s status = %d", method, response.StatusCode)
		}
		body := make([]byte, len(want))
		count, _ := response.Body.Read(body)
		response.Body.Close()
		if method == http.MethodGet && string(body[:count]) != string(want) {
			t.Fatalf("GET body = %q, want %q", body[:count], want)
		}
	}
	for _, suffix := range []string{"/../outside", "/%2e%2e/outside", "/example.com", "/linked.mod"} {
		response, err := http.Get(proxy.URL() + suffix)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("GET %q status = %d, want 404", suffix, response.StatusCode)
		}
	}
	request, err := http.NewRequest(http.MethodPost, proxy.URL()+"/example.com/module/@v/v1.0.0.mod", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want %d", response.StatusCode, http.StatusMethodNotAllowed)
	}
}

// TestVerifierModuleProxyRejectsRequestsPastItsActiveLimit keeps slow clients from consuming unbounded handler work.
func TestVerifierModuleProxyRejectsRequestsPastItsActiveLimit(t *testing.T) {
	proxy := &verifierModuleProxy{prefix: "authority", requests: make(chan struct{}, 1)}
	proxy.requests <- struct{}{}
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/authority/anything", nil)
	response := httptest.NewRecorder()
	proxy.serveHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}

// TestVerifierModuleProxyForwardsColdMissesWithoutExposingUpstream verifies that verifier children use only the authority-scoped endpoint.
func TestVerifierModuleProxyForwardsColdMissesWithoutExposingUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/example.com/module/@v/v1.0.0.mod" {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "text/plain")
		_, _ = writer.Write([]byte("module example.com/module\n"))
	}))
	t.Cleanup(upstream.Close)
	proxy, err := newVerifierModuleProxy([]string{"GOMODCACHE=" + t.TempDir(), "GOPROXY=" + upstream.URL + ",direct"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = proxy.Close() })
	if strings.Contains(proxy.URL(), upstream.URL) {
		t.Fatalf("proxy URL exposes upstream: %q", proxy.URL())
	}
	response, err := verifierModuleProxyHealthClient.Get(proxy.URL() + "/example.com/module/@v/v1.0.0.mod")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("cold miss status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "module example.com/module\n" {
		t.Fatalf("cold miss body = %q", body)
	}
}

// TestVerifierModuleProxyPreservesDeclaredHTTPFallback keeps cold verification compatible with ordinary GOPROXY chains without exposing them to children.
func TestVerifierModuleProxyPreservesDeclaredHTTPFallback(t *testing.T) {
	first := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(first.Close)
	second := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("fallback"))
	}))
	t.Cleanup(second.Close)
	proxy, err := newVerifierModuleProxy([]string{"GOMODCACHE=" + t.TempDir(), "GOPROXY=" + first.URL + "," + second.URL + ",direct"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = proxy.Close() })
	response, err := verifierModuleProxyHealthClient.Get(proxy.URL() + "/example.com/module/@v/v1.0.0.mod")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || string(body) != "fallback" {
		t.Fatalf("fallback response = %d %q", response.StatusCode, body)
	}
}

// TestVerifierModuleProxyHealthCloseAndConcurrentReads verifies authority lifecycle and parallel verifier access.
func TestVerifierModuleProxyHealthCloseAndConcurrentReads(t *testing.T) {
	cache := t.TempDir()
	artifact := filepath.Join(cache, "cache", "download", "module", "@v", "v1.mod")
	if err := os.MkdirAll(filepath.Dir(artifact), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifact, []byte("module module\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	proxy, err := newVerifierModuleProxy([]string{"GOMODCACHE=" + cache})
	if err != nil {
		t.Fatal(err)
	}
	if err := proxy.Healthy(context.Background()); err != nil {
		t.Fatalf("Healthy(): %v", err)
	}
	var group sync.WaitGroup
	for range 16 {
		group.Add(1)
		go func() {
			defer group.Done()
			response, err := http.Get(proxy.URL() + "/module/@v/v1.mod")
			if err != nil {
				t.Errorf("concurrent read: %v", err)
				return
			}
			response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Errorf("concurrent read status = %d", response.StatusCode)
			}
		}()
	}
	group.Wait()
	if err := proxy.Close(); err != nil {
		t.Fatalf("Close(): %v", err)
	}
	if err := proxy.Healthy(context.Background()); err == nil {
		t.Fatal("Healthy() after Close() succeeded")
	}
}

// TestVerifierModuleProxyHealthBypassesDownloadSaturation keeps a live shared authority from looking failed while verifier downloads occupy its bounded request slots.
func TestVerifierModuleProxyHealthBypassesDownloadSaturation(t *testing.T) {
	proxy, err := newVerifierModuleProxy([]string{"GOMODCACHE=" + t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = proxy.Close() })
	for range cap(proxy.requests) {
		proxy.requests <- struct{}{}
	}
	t.Cleanup(func() {
		for range cap(proxy.requests) {
			<-proxy.requests
		}
	})
	if err := proxy.Healthy(context.Background()); err != nil {
		t.Fatalf("Healthy() during download saturation: %v", err)
	}
	response, err := verifierModuleProxyHealthClient.Get(proxy.URL() + "/example.com/module/@v/v1.0.0.mod")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("saturated download status = %d, want %d", response.StatusCode, http.StatusServiceUnavailable)
	}
}
