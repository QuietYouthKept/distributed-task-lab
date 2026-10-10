// Command e2e-s3-proxy is a fault-injection reverse proxy used only by the
// disposable cross-container integration topology. It is not a production
// service and must never be exposed outside that isolated test network.
package main

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

var errDropResponse = errors.New("e2e fault proxy dropped upstream response")

type faultState struct {
	mu                 sync.Mutex
	objectPath         string
	putArmed           bool
	headArmed          bool
	putDropped         bool
	headDropped        bool
	putBarrierCount    int
	putBarrierArrivals int
	putBarrier         chan struct{}
	putBarrierReleased bool
}

type proxy struct {
	state     *faultState
	token     string
	upstream  *url.URL
	s3Handler http.Handler
}

type armRequest struct {
	// ObjectPath is the path-style S3 path without a leading slash, including
	// bucket and key, e.g. "bucket/prefix/object.json".
	ObjectPath      string `json:"object_path"`
	DropResponses   bool   `json:"drop_responses,omitempty"`
	PutBarrierCount int    `json:"put_barrier_count,omitempty"`
}

func main() {
	upstream, err := url.Parse(os.Getenv("E2E_S3_UPSTREAM"))
	if err != nil || upstream.Scheme != "http" || upstream.Host == "" || upstream.Path != "" {
		log.Fatal("E2E_S3_UPSTREAM must be an absolute HTTP URL without a path")
	}
	token := os.Getenv("E2E_S3_PROXY_TOKEN")
	if len(token) < 32 {
		log.Fatal("E2E_S3_PROXY_TOKEN must contain at least 32 characters")
	}
	listenS3 := envOr("E2E_S3_PROXY_LISTEN", ":9000")
	listenAdmin := envOr("E2E_S3_PROXY_ADMIN_LISTEN", ":9001")
	state := &faultState{}
	p := &proxy{state: state, token: token, upstream: upstream}

	reverse := httputil.NewSingleHostReverseProxy(upstream)
	director := reverse.Director
	reverse.Director = func(request *http.Request) {
		originalHost := request.Host
		director(request)
		// MinIO validates the SigV4 Host header as signed by the client. Keep
		// the proxy authority rather than rewriting it to the upstream host.
		request.Host = originalHost
	}
	reverse.ModifyResponse = func(response *http.Response) error {
		p.state.mu.Lock()
		defer p.state.mu.Unlock()
		if response.Request.URL.EscapedPath() != "/"+p.state.objectPath {
			return nil
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil
		}
		switch {
		case response.Request.Method == http.MethodPut && p.state.putArmed:
			p.state.putArmed = false
			p.state.headArmed = true
			p.state.putDropped = true
			return errDropResponse
		case response.Request.Method == http.MethodHead && p.state.headArmed:
			p.state.headDropped = true
			return errDropResponse
		default:
			return nil
		}
	}
	reverse.ErrorHandler = func(writer http.ResponseWriter, request *http.Request, proxyErr error) {
		if errors.Is(proxyErr, errDropResponse) {
			if hijacker, ok := writer.(http.Hijacker); ok {
				connection, _, err := hijacker.Hijack()
				if err == nil {
					_ = connection.Close()
					return
				}
			}
			return
		}
		http.Error(writer, "isolated e2e upstream failure", http.StatusBadGateway)
	}
	p.s3Handler = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !p.waitAtPutBarrier(writer, request) {
			return
		}
		reverse.ServeHTTP(writer, request)
	})

	admin := http.NewServeMux()
	admin.HandleFunc("POST /arm", p.arm)
	admin.HandleFunc("POST /disarm", p.disarm)
	admin.HandleFunc("GET /state", p.readState)
	go func() {
		if err := http.ListenAndServe(listenAdmin, admin); err != nil {
			log.Fatal("e2e fault proxy admin server failed")
		}
	}()
	if err := http.ListenAndServe(listenS3, p.s3Handler); err != nil {
		log.Fatal("e2e fault proxy S3 server failed")
	}
}

func (p *proxy) arm(writer http.ResponseWriter, request *http.Request) {
	if !p.authorized(request) {
		http.Error(writer, "unauthorized", http.StatusUnauthorized)
		return
	}
	var input armRequest
	if err := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 4096)).Decode(&input); err != nil || !validObjectPath(input.ObjectPath) || input.PutBarrierCount < 0 || input.PutBarrierCount > 2 {
		http.Error(writer, "invalid object path", http.StatusBadRequest)
		return
	}
	p.state.mu.Lock()
	p.state.objectPath = input.ObjectPath
	p.state.putArmed = input.DropResponses
	p.state.headArmed = false
	p.state.putDropped = false
	p.state.headDropped = false
	p.state.putBarrierCount = input.PutBarrierCount
	p.state.putBarrierArrivals = 0
	p.state.putBarrier = make(chan struct{})
	p.state.putBarrierReleased = input.PutBarrierCount == 0
	if p.state.putBarrierReleased {
		close(p.state.putBarrier)
	}
	p.state.mu.Unlock()
	writer.WriteHeader(http.StatusNoContent)
}

func (p *proxy) waitAtPutBarrier(writer http.ResponseWriter, request *http.Request) bool {
	p.state.mu.Lock()
	if request.Method != http.MethodPut || request.URL.EscapedPath() != "/"+p.state.objectPath || p.state.putBarrierCount == 0 {
		p.state.mu.Unlock()
		return true
	}
	p.state.putBarrierArrivals++
	if !p.state.putBarrierReleased && p.state.putBarrierArrivals >= p.state.putBarrierCount {
		close(p.state.putBarrier)
		p.state.putBarrierReleased = true
	}
	barrier := p.state.putBarrier
	p.state.mu.Unlock()

	select {
	case <-barrier:
		return true
	case <-request.Context().Done():
		return false
	case <-time.After(30 * time.Second):
		http.Error(writer, "timed out waiting for concurrent test writers", http.StatusGatewayTimeout)
		return false
	}
}

func (p *proxy) disarm(writer http.ResponseWriter, request *http.Request) {
	if !p.authorized(request) {
		http.Error(writer, "unauthorized", http.StatusUnauthorized)
		return
	}
	p.state.mu.Lock()
	p.state.putArmed = false
	p.state.headArmed = false
	p.state.putBarrierCount = 0
	if !p.state.putBarrierReleased && p.state.putBarrier != nil {
		close(p.state.putBarrier)
		p.state.putBarrierReleased = true
	}
	p.state.mu.Unlock()
	writer.WriteHeader(http.StatusNoContent)
}

func (p *proxy) readState(writer http.ResponseWriter, request *http.Request) {
	if !p.authorized(request) {
		http.Error(writer, "unauthorized", http.StatusUnauthorized)
		return
	}
	p.state.mu.Lock()
	state := struct {
		Armed              bool `json:"armed"`
		PutDropped         bool `json:"put_dropped"`
		HeadDropped        bool `json:"head_dropped"`
		PutBarrierArrivals int  `json:"put_barrier_arrivals"`
	}{p.state.putArmed || p.state.headArmed, p.state.putDropped, p.state.headDropped, p.state.putBarrierArrivals}
	p.state.mu.Unlock()
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(state)
}

func (p *proxy) authorized(request *http.Request) bool {
	provided := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
	return len(provided) == len(p.token) && subtle.ConstantTimeCompare([]byte(provided), []byte(p.token)) == 1
}

func validObjectPath(value string) bool {
	if value == "" || strings.HasPrefix(value, "/") || strings.ContainsAny(value, "\\?#%") {
		return false
	}
	parts := strings.Split(value, "/")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
