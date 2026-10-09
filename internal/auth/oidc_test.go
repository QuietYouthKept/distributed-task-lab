package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

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
	if _, err := verifier.Verify(context.Background(), signedToken(t, keyB, "b", claims)); err != nil {
		t.Fatalf("rotation refresh: %v", err)
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
