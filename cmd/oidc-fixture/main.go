// Command oidc-fixture provides a local-only HTTPS JWKS and test-token issuer.
// It is a protocol fixture, not a general-purpose identity provider.
package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const maxFixtureTokenLifetime = 15 * time.Minute

type fixtureClaims struct {
	Roles []string `json:"roles"`
	jwt.RegisteredClaims
}

type fixtureJWK struct {
	KeyType  string `json:"kty"`
	Use      string `json:"use"`
	Alg      string `json:"alg"`
	KeyID    string `json:"kid"`
	Modulus  string `json:"n"`
	Exponent string `json:"e"`
}

type fixtureKey struct {
	id      string
	private *rsa.PrivateKey
}

type jwksFixture struct {
	mu            sync.RWMutex
	issuer        string
	audience      string
	tokenIssuer   bool
	active        fixtureKey
	previous      *fixtureKey
	tokenLifetime time.Duration
	now           func() time.Time
}

func newJWKSFixture(issuer, audience string, tokenIssuer bool) (*jwksFixture, error) {
	parsedIssuer, err := url.Parse(issuer)
	if err != nil || parsedIssuer.Scheme != "https" || parsedIssuer.Host == "" || parsedIssuer.User != nil ||
		parsedIssuer.RawQuery != "" || parsedIssuer.Fragment != "" || strings.TrimSpace(issuer) != issuer ||
		strings.TrimSpace(audience) == "" || len(audience) > 512 {
		return nil, errors.New("fixture issuer must be HTTPS and audience is required")
	}
	key, err := generateFixtureKey()
	if err != nil {
		return nil, err
	}
	return &jwksFixture{issuer: strings.TrimRight(issuer, "/"), audience: audience, tokenIssuer: tokenIssuer,
		active: key, tokenLifetime: 5 * time.Minute, now: time.Now}, nil
}

func generateFixtureKey() (fixtureKey, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fixtureKey{}, fmt.Errorf("generate fixture signing key: %w", err)
	}
	var idBytes [12]byte
	if _, err := rand.Read(idBytes[:]); err != nil {
		return fixtureKey{}, fmt.Errorf("generate fixture key ID: %w", err)
	}
	return fixtureKey{id: base64.RawURLEncoding.EncodeToString(idBytes[:]), private: key}, nil
}

func (f *jwksFixture) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /jwks", f.serveJWKS)
	if f.tokenIssuer {
		mux.HandleFunc("POST /test/tokens/{role}", f.serveToken)
		mux.HandleFunc("POST /test/rotate", f.rotateKey)
	}
	return mux
}

func (f *jwksFixture) serveJWKS(w http.ResponseWriter, _ *http.Request) {
	f.mu.RLock()
	keys := []fixtureJWK{toJWK(f.active)}
	if f.previous != nil {
		keys = append(keys, toJWK(*f.previous))
	}
	f.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"keys": keys})
}

func (f *jwksFixture) serveToken(w http.ResponseWriter, r *http.Request) {
	role := r.PathValue("role")
	if role != "reader" && role != "operator" && role != "admin" {
		http.Error(w, "unknown fixture identity", http.StatusNotFound)
		return
	}
	token, expiresAt, err := f.mint("fixture-"+role, "fixture-"+role, []string{"fixture-" + role}, f.tokenLifetime)
	if err != nil {
		http.Error(w, "could not mint fixture token", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"access_token": token, "token_type": "Bearer", "expires_at": expiresAt.UTC().Format(time.RFC3339)})
}

func (f *jwksFixture) rotateKey(w http.ResponseWriter, _ *http.Request) {
	key, err := generateFixtureKey()
	if err != nil {
		http.Error(w, "could not rotate fixture key", http.StatusInternalServerError)
		return
	}
	f.mu.Lock()
	previous := f.active
	f.active, f.previous = key, &previous
	f.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (f *jwksFixture) mint(subject, identity string, roles []string, lifetime time.Duration) (string, time.Time, error) {
	if lifetime <= 0 || lifetime > maxFixtureTokenLifetime {
		return "", time.Time{}, errors.New("fixture token lifetime must be greater than zero and at most fifteen minutes")
	}
	f.mu.RLock()
	key := f.active
	f.mu.RUnlock()
	now := f.now().UTC()
	expiresAt := now.Add(lifetime)
	claims := fixtureClaims{Roles: append([]string(nil), roles...), RegisteredClaims: jwt.RegisteredClaims{
		Issuer: f.issuer, Subject: subject, Audience: jwt.ClaimStrings{f.audience},
		IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(expiresAt),
		ID: identity + "-" + key.id,
	}}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = key.id
	signed, err := token.SignedString(key.private)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign fixture token: %w", err)
	}
	return signed, expiresAt, nil
}

func toJWK(key fixtureKey) fixtureJWK {
	return fixtureJWK{KeyType: "RSA", Use: "sig", Alg: jwt.SigningMethodRS256.Alg(), KeyID: key.id,
		Modulus:  base64.RawURLEncoding.EncodeToString(key.private.PublicKey.N.Bytes()),
		Exponent: base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.private.PublicKey.E)).Bytes())}
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if os.Getenv("APP_PROFILE") != "staging-sim" {
		logger.Error("OIDC fixture is restricted to APP_PROFILE=staging-sim")
		os.Exit(2)
	}
	issuer := os.Getenv("OIDC_FIXTURE_ISSUER")
	if issuer == "" {
		issuer = "https://identity:8443"
	}
	audience := os.Getenv("OIDC_FIXTURE_AUDIENCE")
	if audience == "" {
		logger.Error("OIDC_FIXTURE_AUDIENCE is required")
		os.Exit(2)
	}
	cert, key := os.Getenv("OIDC_FIXTURE_TLS_CERT_FILE"), os.Getenv("OIDC_FIXTURE_TLS_KEY_FILE")
	if cert == "" || key == "" {
		logger.Error("OIDC fixture TLS certificate and key paths are required")
		os.Exit(2)
	}
	issueTokens := os.Getenv("OIDC_FIXTURE_ENABLE_TEST_ISSUER") == "true"
	fixture, err := newJWKSFixture(issuer, audience, issueTokens)
	if err != nil {
		logger.Error("invalid OIDC fixture configuration", "error", err)
		os.Exit(2)
	}
	server := &http.Server{Addr: "0.0.0.0:8443", Handler: fixture.Handler(), ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 15 * time.Second, MaxHeaderBytes: 8 << 10}
	logger.Info("local HTTPS JWKS fixture started", "issuer", fixture.issuer, "test_token_issuer_enabled", issueTokens,
		"identity_mode", "local_jwks_protocol_fixture_not_full_oidc")
	stopped := make(chan os.Signal, 1)
	signal.Notify(stopped, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stopped
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()
	if err := server.ListenAndServeTLS(cert, key); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("OIDC fixture stopped unexpectedly", "error", err)
		os.Exit(1)
	}
}
