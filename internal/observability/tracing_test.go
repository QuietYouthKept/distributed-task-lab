package observability

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"go.opentelemetry.io/otel"
	collecttrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

func TestSetupTracingExportsSpanToOTLPHTTPReceiver(t *testing.T) {
	var mu sync.Mutex
	var requests int
	var received *collecttrace.ExportTraceServiceRequest
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/traces" {
			t.Errorf("OTLP request = %s %s", r.Method, r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read OTLP body: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		export := &collecttrace.ExportTraceServiceRequest{}
		if err := proto.Unmarshal(body, export); err != nil {
			t.Errorf("decode OTLP protobuf: %v", err)
			http.Error(w, "bad protobuf", http.StatusBadRequest)
			return
		}
		mu.Lock()
		requests++
		received = export
		mu.Unlock()
		w.Header().Set("Content-Type", "application/x-protobuf")
		_, _ = w.Write([]byte{0x0a, 0x00})
	}))
	defer receiver.Close()

	t.Setenv("OTEL_ENABLED", "true")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", receiver.URL)
	shutdown, err := SetupTracing(context.Background(), "phase1-test-service")
	if err != nil {
		t.Fatalf("setup tracing: %v", err)
	}
	_, span := otel.Tracer("phase1-test").Start(context.Background(), "e2e.receiver.assertion")
	span.End()
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("flush tracing provider: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if requests == 0 || received == nil {
		t.Fatal("OTLP receiver did not receive an exported trace request")
	}
	spanCount := 0
	serviceFound := false
	for _, resourceSpans := range received.ResourceSpans {
		for _, attribute := range resourceSpans.Resource.Attributes {
			if attribute.Key == "service.name" && attribute.Value.GetStringValue() == "phase1-test-service" {
				serviceFound = true
			}
		}
		for _, scopeSpans := range resourceSpans.ScopeSpans {
			for _, exportedSpan := range scopeSpans.Spans {
				spanCount++
				if exportedSpan.Name != "e2e.receiver.assertion" {
					t.Errorf("exported span name = %q", exportedSpan.Name)
				}
			}
		}
	}
	if !serviceFound || spanCount != 1 {
		t.Fatalf("receiver payload service_found=%v span_count=%d", serviceFound, spanCount)
	}
}
