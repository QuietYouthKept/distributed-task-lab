package main

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/auth"
)

func TestJWKSFixtureIssuesRoleTokenOverTrustedHTTPSAndRotatesKeys(t *testing.T) {
	const issuer, audience = "https://identity:8443", "tasklab-local"
	fixture, err := newJWKSFixture(issuer, audience, true)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(fixture.Handler())
	t.Cleanup(server.Close)

	caPath := filepath.Join(t.TempDir(), "fixture-ca.pem")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := auth.NewHTTPClientWithCAFile(caPath)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := auth.NewVerifier(auth.OIDCConfig{Issuer: issuer, Audience: audience, JWKSURL: server.URL + "/jwks",
		RoleClaim: "roles", RoleMappings: map[string]auth.Role{"fixture-reader": auth.RoleReader, "fixture-operator": auth.RoleOperator,
			"fixture-admin": auth.RoleAdmin}, HTTPClient: client, CacheTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	oldToken := requestFixtureToken(t, server.Client(), server.URL, "operator")
	principal, err := verifier.Verify(context.Background(), oldToken)
	if err != nil {
		t.Fatal(err)
	}
	if principal.Subject != "fixture-operator" || !principal.HasRole(auth.RoleOperator) || !principal.HasRole(auth.RoleReader) {
		t.Fatalf("unexpected operator principal: %+v", principal)
	}

	response, err := server.Client().Post(server.URL+"/test/rotate", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("rotate status=%d want=%d", response.StatusCode, http.StatusNoContent)
	}
	newToken := requestFixtureToken(t, server.Client(), server.URL, "reader")
	// Unknown-kid refreshes are throttled to one per five seconds. Poll with a
	// bounded deadline so this integration-style unit test honors that security
	// guard without relying on an unbounded retry or weakening the verifier.
	refreshDeadline := time.NewTimer(7 * time.Second)
	defer refreshDeadline.Stop()
	refreshPoll := time.NewTicker(50 * time.Millisecond)
	defer refreshPoll.Stop()
	for {
		principal, err = verifier.Verify(context.Background(), newToken)
		if err == nil {
			break
		}
		select {
		case <-refreshDeadline.C:
			t.Fatalf("verifier did not refresh JWKS for rotated key before deadline: %v", err)
		case <-refreshPoll.C:
		}
	}
	if principal.Subject != "fixture-reader" || !principal.HasRole(auth.RoleReader) || principal.HasRole(auth.RoleAdmin) {
		t.Fatalf("unexpected reader principal: %+v", principal)
	}
	if _, err := verifier.Verify(context.Background(), oldToken); err != nil {
		t.Fatalf("previous signing key should remain verifiable during local overlap: %v", err)
	}

	parts := strings.Split(oldToken, ".")
	if len(parts) != 3 || parts[2] == "" {
		t.Fatalf("unexpected JWT format with %d segments", len(parts))
	}
	parts[2] = map[bool]string{true: "A", false: "B"}[parts[2][0] == 'A'] + parts[2][1:]
	tampered := strings.Join(parts, ".")
	if _, err := verifier.Verify(context.Background(), tampered); err == nil {
		t.Fatal("tampered fixture token was accepted")
	}
}

func TestJWKSFixtureTokenIssuerIsOptInAndRolesAreFixed(t *testing.T) {
	fixture, err := newJWKSFixture("https://identity:8443", "tasklab-local", false)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(fixture.Handler())
	t.Cleanup(server.Close)
	for _, path := range []string{"/test/tokens/admin", "/test/rotate"} {
		response, err := server.Client().Post(server.URL+path, "application/json", strings.NewReader(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("disabled fixture endpoint %s returned %d", path, response.StatusCode)
		}
	}

	enabled, err := newJWKSFixture("https://identity:8443", "tasklab-local", true)
	if err != nil {
		t.Fatal(err)
	}
	server2 := httptest.NewTLSServer(enabled.Handler())
	t.Cleanup(server2.Close)
	response, err := server2.Client().Post(server2.URL+"/test/tokens/anything", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("unmapped role issuer returned %d", response.StatusCode)
	}
}

func TestJWKSFixtureRejectsInsecureOrUnboundedConfiguration(t *testing.T) {
	for _, test := range []struct{ issuer, audience string }{
		{"http://identity:8443", "aud"},
		{"https://identity:8443", ""},
		{"https://user:secret@identity:8443", "aud"},
		{"https://identity:8443?token=secret", "aud"},
		{"https://identity:8443", strings.Repeat("a", 513)},
	} {
		if _, err := newJWKSFixture(test.issuer, test.audience, false); err == nil {
			t.Fatalf("accepted invalid fixture configuration: issuer=%q audience=%q", test.issuer, test.audience)
		}
	}
	fixture, err := newJWKSFixture("https://identity:8443", "aud", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := fixture.mint("subject", "id", []string{"fixture-reader"}, maxFixtureTokenLifetime+time.Second); err == nil {
		t.Fatal("accepted unbounded test token lifetime")
	}
}

func requestFixtureToken(t *testing.T, client *http.Client, baseURL, role string) string {
	t.Helper()
	response, err := client.Post(baseURL+"/test/tokens/"+role, "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("token endpoint status=%d", response.StatusCode)
	}
	var value struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		t.Fatal(err)
	}
	if value.AccessToken == "" {
		t.Fatal("fixture returned empty token")
	}
	return value.AccessToken
}
