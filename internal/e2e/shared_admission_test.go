package e2e

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSharedIngressAdmissionAcrossIndependentAPIContainers(t *testing.T) {
	apiA, apiB := os.Getenv("E2E_API_A_URL"), os.Getenv("E2E_API_B_URL")
	token := os.Getenv("E2E_API_TOKEN")
	if apiA == "" || apiB == "" || token == "" {
		t.Skip("set E2E_API_A_URL, E2E_API_B_URL and E2E_API_TOKEN from the isolated admission runner")
	}
	burst, err := strconv.Atoi(os.Getenv("E2E_SHARED_RATE_LIMIT_BURST"))
	if err != nil || burst < 1 {
		t.Fatal("E2E_SHARED_RATE_LIMIT_BURST must be a positive integer")
	}
	requests := burst * 4
	start := make(chan struct{})
	var allowed, limited, unexpected atomic.Int64
	var wg sync.WaitGroup
	client := &http.Client{Timeout: 10 * time.Second}
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			base := apiA
			if i%2 != 0 {
				base = apiB
			}
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, base+"/v1/tasks?limit=1", nil)
			if err != nil {
				unexpected.Add(1)
				return
			}
			req.Header.Set("Authorization", "Bearer "+token)
			resp, err := client.Do(req)
			if err != nil {
				unexpected.Add(1)
				return
			}
			defer resp.Body.Close()
			switch resp.StatusCode {
			case http.StatusOK:
				allowed.Add(1)
			case http.StatusTooManyRequests:
				if resp.Header.Get("Retry-After") == "" {
					unexpected.Add(1)
				} else {
					limited.Add(1)
				}
			default:
				unexpected.Add(1)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	if got := allowed.Load(); got != int64(burst) {
		t.Fatalf("shared burst across two API containers: expected %d admitted requests, got %d (429=%d unexpected=%d)", burst, got, limited.Load(), unexpected.Load())
	}
	if got := limited.Load(); got != int64(requests-burst) {
		t.Fatalf("expected %d rate-limited responses, got %d", requests-burst, got)
	}
	if got := unexpected.Load(); got != 0 {
		t.Fatalf("unexpected HTTP/network outcomes: %d", got)
	}
	t.Logf("two independent API containers shared one PostgreSQL bucket: admitted=%d limited=%d", allowed.Load(), limited.Load())
}

func TestSharedAdmissionRunnerConfiguration(t *testing.T) {
	if os.Getenv("E2E_REQUIRE_SHARED_ADMISSION") != "true" {
		t.Skip("isolated admission runner not enabled")
	}
	for _, name := range []string{"E2E_API_A_URL", "E2E_API_B_URL", "E2E_API_TOKEN"} {
		if os.Getenv(name) == "" {
			t.Fatal(fmt.Sprintf("%s is required by the isolated admission runner", name))
		}
	}
}
