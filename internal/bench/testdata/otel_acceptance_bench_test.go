// Package http benchmarks the generated runtime using GoForj Web workload contracts.
package http

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"example.com/otelperf/internal/inspects"
	"example.com/otelperf/internal/logger"
	"example.com/otelperf/internal/runtime"
	"github.com/goforj/web"
)

// acceptancePublisher exercises source capture without retaining per-request history.
type acceptancePublisher struct{}

// Publish discards finished captures like a bounded outbound sink after delivery.
func (acceptancePublisher) Publish(inspects.Record) {}

// BenchmarkOTelAcceptance measures the published Web workloads inside the generated HTTP runtime.
func BenchmarkOTelAcceptance(b *testing.B) {
	for _, mode := range []string{"off", "gated", "lighthouse", "otlp"} {
		b.Run(mode, func(b *testing.B) {
			for _, workload := range []string{"static_text", "path_param_json", "middleware_chain", "live_plain_text"} {
				b.Run(workload, func(b *testing.B) {
					handler := acceptanceHandler(b, mode, workload)
					path := "/benchmark/static"
					if workload == "path_param_json" {
						path = "/benchmark/users/42"
					}
					req := httptest.NewRequest(http.MethodGet, path, nil)
					check := httptest.NewRecorder()
					handler.ServeHTTP(check, req)
					if check.Code != 200 {
						b.Fatalf("invalid workload: %d", check.Code)
					}
					if workload == "path_param_json" {
						var result struct {
							ID string `json:"id"`
						}
						if err := json.Unmarshal(check.Body.Bytes(), &result); err != nil || result.ID != "42" {
							b.Fatal("path parameter JSON contract failed")
						}
					} else if check.Body.String() != "Hello, benchmark!" {
						b.Fatal("text response contract failed")
					}
					if workload == "live_plain_text" {
						acceptanceLive(b, handler, path)
						return
					}
					if workload == "middleware_chain" && check.Header().Get("X-Benchmark") != "ready" {
						b.Fatal("middleware did not execute")
					}
					writer := &acceptanceWriter{header: make(http.Header)}
					b.ReportAllocs()
					b.ResetTimer()
					for range b.N {
						clear(writer.header)
						writer.status = 0
						handler.ServeHTTP(writer, req)
					}
					b.StopTimer()
					b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "requests/s")
				})
			}
		})
	}
}

// acceptanceHandler keeps runtime flags and endpoint work identical across base and PR renders.
func acceptanceHandler(b *testing.B, mode, workload string) http.Handler {
	b.Helper()
	for _, key := range []string{"OTEL_TRACES_EXPORTER", "OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "OTEL_TRACES_SAMPLER", "OTEL_SDK_DISABLED"} {
		b.Setenv(key, "")
	}
	b.Setenv("APP_ENV", "production")
	b.Setenv("HTTP_ACCESS_LOG_ENABLED", "false")
	b.Setenv("METRICS_HTTP_ENABLED", "false")
	b.Setenv("LIGHTHOUSE_INSPECT_ENABLED", "false")
	if mode == "lighthouse" || mode == "gated" {
		b.Setenv("LIGHTHOUSE_INSPECT_ENABLED", "true")
	}
	manager := inspects.NewManager()
	manager.SetPublisher(acceptancePublisher{})
	manager.SetPublishOnly(true)
	if mode == "gated" {
		manager.SetCaptureGate(func() bool { return false })
	}
	var received atomic.Int64
	if mode == "otlp" {
		configure, ok := any(manager).(interface{ ConfigureTelemetry(context.Context) error })
		if !ok {
			b.Skip("OTLP was not supported by the base revision")
		}
		collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			payload, err := io.ReadAll(r.Body)
			if err != nil {
				b.Error(err)
			}
			received.Add(int64(acceptanceSpanCount(payload, 0)))
			w.Header().Set("Content-Type", "application/x-protobuf")
		}))
		b.Cleanup(collector.Close)
		b.Setenv("OTEL_TRACES_EXPORTER", "otlp")
		b.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector.URL)
		if err := configure.ConfigureTelemetry(context.Background()); err != nil {
			b.Fatal(err)
		}
	}
	if closer, ok := any(manager).(interface{ Shutdown(context.Context) error }); ok {
		expected := b.N + 1
		if workload == "live_plain_text" {
			expected++
		}
		b.Cleanup(func() {
			if err := closer.Shutdown(context.Background()); err != nil {
				b.Error(err)
			}
			if mode == "otlp" {
				b.ReportMetric(float64(received.Load())*100/float64(expected), "exported_pct")
			}
		})
	}
	var middleware []web.Middleware
	if workload == "middleware_chain" {
		middleware = []web.Middleware{
			func(next web.Handler) web.Handler {
				return func(c web.Context) error { c.Set("benchmark-value", "ready"); return next(c) }
			},
			func(next web.Handler) web.Handler {
				return func(c web.Context) error { c.SetHeader("X-Benchmark", "ready"); return next(c) }
			},
			func(next web.Handler) web.Handler {
				return func(c web.Context) error {
					if c.Get("benchmark-value") != "ready" {
						return fmt.Errorf("missing middleware state")
					}
					return next(c)
				}
			},
		}
	}
	routePath := "/benchmark/static"
	handler := web.Handler(func(c web.Context) error { return c.Text(200, "Hello, benchmark!") })
	if workload == "path_param_json" {
		routePath = "/benchmark/users/:id"
		handler = func(c web.Context) error {
			return c.JSON(200, struct {
				ID string `json:"id"`
			}{ID: c.Param("id")})
		}
	}
	server := NewServer(logger.NewSilentLogger(), runtime.NewTimeouts(), []web.RouteGroup{web.NewRouteGroup("", []web.Route{web.NewRoute(http.MethodGet, routePath, handler)}, middleware...)}, nil, nil, newTestMaintenanceState(b), manager, nil)
	result, err := server.bootstrap("")
	if err != nil {
		b.Fatal(err)
	}
	return result
}

// acceptanceLive measures serial HTTP/1.1 keep-alive requests independently from in-process dispatch.
func acceptanceLive(b *testing.B, handler http.Handler, path string) {
	server := httptest.NewServer(handler)
	defer server.Close()
	transport := &http.Transport{MaxIdleConns: 1, MaxIdleConnsPerHost: 1, MaxConnsPerHost: 1, DisableCompression: true, ForceAttemptHTTP2: false}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}
	request, err := http.NewRequest(http.MethodGet, server.URL+path, nil)
	if err != nil {
		b.Fatal(err)
	}
	perform := func() {
		response, err := client.Do(request)
		if err != nil {
			b.Fatal(err)
		}
		_, err = io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != 200 {
			b.Fatalf("HTTP workload failed: %v", err)
		}
	}
	perform()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		perform()
	}
	b.StopTimer()
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "requests/s")
}

// acceptanceWriter reuses response storage to isolate handler dispatch cost.
type acceptanceWriter struct {
	header http.Header
	status int
}

// Header returns the resettable response header map.
func (w *acceptanceWriter) Header() http.Header { return w.header }

// WriteHeader preserves the first response status.
func (w *acceptanceWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

// Write discards bytes without allocating response storage.
func (w *acceptanceWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	return len(body), nil
}

// WriteString supports the router's direct text response path.
func (w *acceptanceWriter) WriteString(body string) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	return len(body), nil
}

// acceptanceSpanCount counts standard OTLP ResourceSpans/ScopeSpans/Spans without changing either module's dependency graph.
func acceptanceSpanCount(payload []byte, depth int) int {
	count := 0
	for len(payload) > 0 {
		tag, size := binary.Uvarint(payload)
		if size <= 0 {
			return count
		}
		payload = payload[size:]
		switch tag & 7 {
		case 0:
			_, size = binary.Uvarint(payload)
			if size <= 0 {
				return count
			}
			payload = payload[size:]
		case 1:
			if len(payload) < 8 {
				return count
			}
			payload = payload[8:]
		case 2:
			length, prefix := binary.Uvarint(payload)
			if prefix <= 0 || length > uint64(len(payload)-prefix) {
				return count
			}
			field := payload[prefix : prefix+int(length)]
			target := uint64(2)
			if depth == 0 {
				target = 1
			}
			if tag>>3 == target {
				if depth == 2 {
					count++
				} else {
					count += acceptanceSpanCount(field, depth+1)
				}
			}
			payload = payload[prefix+int(length):]
		case 5:
			if len(payload) < 4 {
				return count
			}
			payload = payload[4:]
		default:
			return count
		}
	}
	return count
}
