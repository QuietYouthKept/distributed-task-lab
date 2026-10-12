package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestNewHTTPClientWithCAFileTrustsMountedLocalCA(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	certificate := server.TLS.Certificates[0].Certificate[0]
	path := filepath.Join(t.TempDir(), "local-ca.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate}), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := NewHTTPClientWithCAFile(path)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("client should trust the explicitly mounted local CA: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("unexpected status %d", response.StatusCode)
	}
}

func TestNewHTTPClientWithCAFileRejectsInvalidPEMWithoutLeakingPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-ca.pem")
	if err := os.WriteFile(path, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := NewHTTPClientWithCAFile(path)
	if err == nil || strings.Contains(err.Error(), path) {
		t.Fatalf("invalid trust bundle should fail without leaking its path, got %v", err)
	}
}

func TestVerifierValidatesIdentityAndMapsOnlyTrustedRoles(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	server := jwksServer(t, &privateKey.PublicKey, "key-1")
	defer server.Close()
	verifier, err := NewVerifier(OIDCConfig{
		Issuer: "https://issuer.example", Audience: "task-api", JWKSURL: server.URL,
		RoleMappings:     map[string]Role{"ops-group": RoleOperator, "admins": RoleAdmin},
		ServiceClientIDs: map[string]struct{}{"worker-client": {}},
	})
	if err != nil {
		t.Fatal(err)
	}
	claims := oidcClaims{Roles: []string{"ops-group", "attacker-admin"}, AZP: "worker-client", RegisteredClaims: jwt.RegisteredClaims{
		Issuer: "https://issuer.example", Subject: "service:worker-17", Audience: jwt.ClaimStrings{"task-api"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)), NotBefore: jwt.NewNumericDate(time.Now().Add(-time.Second)),
	}}
	raw := signedToken(t, privateKey, "key-1", claims)
	p, err := verifier.Verify(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if p.Subject != "service:worker-17" || p.Kind != KindService || !p.HasRole(RoleOperator) || p.HasRole(RoleAdmin) || !p.HasRole(RoleReader) {
		t.Fatalf("unexpected principal: %+v", p)
	}
}

func TestAdminRoleInheritsOperatorAndReader(t *testing.T) {
	p := Principal{Subject: "admin", Roles: map[Role]struct{}{RoleAdmin: {}}}
	if !p.HasRole(RoleReader) || !p.HasRole(RoleOperator) || !p.IsAdmin() {
		t.Fatal("admin role should include operator and reader permissions")
	}
}

func TestVerifierRejectsUntrustedClaimsAndAlgorithms(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	server := jwksServer(t, &privateKey.PublicKey, "key-1")
	defer server.Close()
	verifier, err := NewVerifier(OIDCConfig{Issuer: "https://issuer.example", Audience: "task-api", JWKSURL: server.URL, RoleMappings: map[string]Role{"admin": RoleAdmin}})
	if err != nil {
		t.Fatal(err)
	}
	base := oidcClaims{Roles: []string{"admin"}, RegisteredClaims: jwt.RegisteredClaims{
		Issuer: "https://issuer.example", Subject: "alice", Audience: jwt.ClaimStrings{"task-api"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
	}}
	tests := []struct {
		name string
		edit func(*oidcClaims)
	}{
		{"wrong issuer", func(c *oidcClaims) { c.Issuer = "https://evil.example" }},
		{"wrong audience", func(c *oidcClaims) { c.Audience = jwt.ClaimStrings{"other-api"} }},
		{"expired", func(c *oidcClaims) { c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute)) }},
		{"not yet valid", func(c *oidcClaims) { c.NotBefore = jwt.NewNumericDate(time.Now().Add(time.Minute)) }},
		{"missing subject", func(c *oidcClaims) { c.Subject = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := base
			claims.Roles = append([]string(nil), base.Roles...)
			tt.edit(&claims)
			if _, err := verifier.Verify(context.Background(), signedToken(t, privateKey, "key-1", claims)); err == nil {
				t.Fatal("expected token to be rejected")
			}
		})
	}
	forged := signedTokenWithMethod(t, jwt.SigningMethodHS256, []byte("secret"), "key-1", base)
	if _, err := verifier.Verify(context.Background(), forged); err == nil {
		t.Fatal("HS256 token was accepted")
	}
	wrongSigner, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.Verify(context.Background(), signedToken(t, wrongSigner, "key-1", base)); err == nil {
		t.Fatal("token with a forged signature was accepted")
	}
}

func TestVerifierRefreshesJWKSForRotatedKey(t *testing.T) {
	keyA, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keyB, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var current atomic.Pointer[rsa.PublicKey]
	current.Store(&keyA.PublicKey)
	var kid atomic.Value
	kid.Store("a")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jwksPayload(current.Load(), kid.Load().(string)))
	}))
	defer server.Close()
	verifier, err := NewVerifier(OIDCConfig{Issuer: "https://issuer.example", Audience: "task-api", JWKSURL: server.URL, RoleMappings: map[string]Role{}})
	if err != nil {
		t.Fatal(err)
	}
	claims := oidcClaims{RegisteredClaims: jwt.RegisteredClaims{Issuer: "https://issuer.example", Subject: "alice", Audience: jwt.ClaimStrings{"task-api"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute))}}
	if _, err := verifier.Verify(context.Background(), signedToken(t, keyA, "a", claims)); err != nil {
		t.Fatal(err)
	}
	current.Store(&keyB.PublicKey)
	kid.Store("b")
	verifier.refreshMu.Lock()
	verifier.lastRefreshAttempt = time.Now().Add(-jwksRefreshMinInterval)
	verifier.refreshMu.Unlock()
	if _, err := verifier.Verify(context.Background(), signedToken(t, keyB, "b", claims)); err != nil {
		t.Fatalf("rotation refresh: %v", err)
	}
}

func TestVerifierRateLimitsUnknownKIDRefreshes(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jwksPayload(&privateKey.PublicKey, "known"))
	}))
	defer server.Close()
	verifier, err := NewVerifier(OIDCConfig{
		Issuer: "https://issuer.example", Audience: "task-api", JWKSURL: server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.keyFor(context.Background(), "known"); err != nil {
		t.Fatal(err)
	}
	for _, kid := range []string{"random-1", "random-2", "random-3"} {
		if _, err := verifier.keyFor(context.Background(), kid); err == nil {
			t.Fatalf("unknown KID %q unexpectedly trusted", kid)
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("JWKS requests after a burst of unknown KIDs = %d, want 1", got)
	}

	verifier.refreshMu.Lock()
	verifier.lastRefreshAttempt = time.Now().Add(-jwksRefreshMinInterval)
	verifier.refreshMu.Unlock()
	if _, err := verifier.keyFor(context.Background(), "rotated-key"); err == nil {
		t.Fatal("unknown rotated KID unexpectedly trusted")
	}
	if _, err := verifier.keyFor(context.Background(), "another-random-kid"); err == nil {
		t.Fatal("second unknown KID unexpectedly trusted")
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("JWKS requests after cooldown = %d, want exactly one refresh", got)
	}
}

func TestVerifierKnownCachedKeyRemainsAvailableDuringJWKSRefresh(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	refreshStarted := make(chan struct{})
	releaseRefresh := make(chan struct{})
	var releaseOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) > 1 {
			close(refreshStarted)
			<-releaseRefresh
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jwksPayload(&privateKey.PublicKey, "known"))
	}))
	defer server.Close()
	defer releaseOnce.Do(func() { close(releaseRefresh) })
	verifier, err := NewVerifier(OIDCConfig{
		Issuer: "https://issuer.example", Audience: "task-api", JWKSURL: server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.keyFor(context.Background(), "known"); err != nil {
		t.Fatal(err)
	}
	verifier.refreshMu.Lock()
	verifier.lastRefreshAttempt = time.Now().Add(-jwksRefreshMinInterval)
	verifier.refreshMu.Unlock()
	unknownDone := make(chan error, 1)
	go func() {
		_, err := verifier.keyFor(context.Background(), "unknown")
		unknownDone <- err
	}()
	select {
	case <-refreshStarted:
	case <-time.After(time.Second):
		t.Fatal("unknown KID did not initiate the allowed refresh")
	}

	knownDone := make(chan error, 1)
	go func() {
		_, err := verifier.keyFor(context.Background(), "known")
		knownDone <- err
	}()
	select {
	case err := <-knownDone:
		if err != nil {
			t.Fatalf("cached known key failed during refresh: %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("known cached key was blocked by a network JWKS refresh")
	}
	releaseOnce.Do(func() { close(releaseRefresh) })
	if err := <-unknownDone; err == nil {
		t.Fatal("unknown KID unexpectedly trusted")
	}
}

func TestVerifierFailsClosedWhenJWKSIsUnavailableAfterCacheExpiry(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var unavailable atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if unavailable.Load() {
			http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jwksPayload(&privateKey.PublicKey, "key-1"))
	}))
	defer server.Close()
	verifier, err := NewVerifier(OIDCConfig{
		Issuer: "https://issuer.example", Audience: "task-api", JWKSURL: server.URL,
		CacheTTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	claims := oidcClaims{RegisteredClaims: jwt.RegisteredClaims{
		Issuer: "https://issuer.example", Subject: "alice", Audience: jwt.ClaimStrings{"task-api"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
	}}
	raw := signedToken(t, privateKey, "key-1", claims)
	if _, err := verifier.Verify(context.Background(), raw); err != nil {
		t.Fatalf("initial JWKS verification: %v", err)
	}

	unavailable.Store(true)
	verifier.mu.Lock()
	verifier.fetchedAt = time.Now().Add(-2 * time.Minute)
	verifier.mu.Unlock()
	verifier.refreshMu.Lock()
	verifier.lastRefreshAttempt = time.Now().Add(-jwksRefreshMinInterval)
	verifier.refreshMu.Unlock()
	if _, err := verifier.Verify(context.Background(), raw); err == nil {
		t.Fatal("verification accepted an expired JWKS cache during issuer outage")
	}
}

func TestVerifierHonorsJWKSHTTPTimeout(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(80 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jwksPayload(&privateKey.PublicKey, "key-1"))
	}))
	defer server.Close()
	verifier, err := NewVerifier(OIDCConfig{
		Issuer: "https://issuer.example", Audience: "task-api", JWKSURL: server.URL,
		HTTPClient: &http.Client{Timeout: 10 * time.Millisecond},
	})
	if err != nil {
		t.Fatal(err)
	}
	claims := oidcClaims{RegisteredClaims: jwt.RegisteredClaims{
		Issuer: "https://issuer.example", Subject: "alice", Audience: jwt.ClaimStrings{"task-api"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
	}}
	if _, err := verifier.Verify(context.Background(), signedToken(t, privateKey, "key-1", claims)); err == nil {
		t.Fatal("verification succeeded despite the configured JWKS HTTP timeout")
	}
}

func jwksServer(t *testing.T, key *rsa.PublicKey, kid string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jwksPayload(key, kid))
	}))
}

func jwksPayload(key *rsa.PublicKey, kid string) map[string]any {
	return map[string]any{"keys": []map[string]string{{"kid": kid, "kty": "RSA", "use": "sig", "alg": "RS256", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())}}}
}

func signedToken(t *testing.T, key *rsa.PrivateKey, kid string, claims oidcClaims) string {
	t.Helper()
	return signedTokenWithMethod(t, jwt.SigningMethodRS256, key, kid, claims)
}

func signedTokenWithMethod(t *testing.T, method jwt.SigningMethod, key any, kid string, claims oidcClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(method, claims)
	token.Header["kid"] = kid
	raw, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
