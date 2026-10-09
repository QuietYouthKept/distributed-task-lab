package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const maxJWKSBytes = 1 << 20
const maxTokenBytes = 16 << 10

type OIDCConfig struct {
	Issuer           string
	Audience         string
	JWKSURL          string
	RoleClaim        string
	RoleMappings     map[string]Role
	ServiceClientIDs map[string]struct{}
	CacheTTL         time.Duration
	HTTPClient       *http.Client
	ClockSkew        time.Duration
}

type oidcClaims struct {
	Roles    []string `json:"roles"`
	Groups   []string `json:"groups"`
	ClientID string   `json:"client_id"`
	AZP      string   `json:"azp"`
	jwt.RegisteredClaims
}

type jwksDocument struct {
	Keys []struct {
		KID string `json:"kid"`
		KTY string `json:"kty"`
		USE string `json:"use"`
		ALG string `json:"alg"`
		N   string `json:"n"`
		E   string `json:"e"`
	} `json:"keys"`
}

type Verifier struct {
	config OIDCConfig

	mu        sync.Mutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
}

func NewVerifier(config OIDCConfig) (*Verifier, error) {
	if strings.TrimSpace(config.Issuer) == "" || strings.TrimSpace(config.Audience) == "" || strings.TrimSpace(config.JWKSURL) == "" {
		return nil, errors.New("OIDC issuer, audience, and JWKS URL are required")
	}
	if config.RoleClaim == "" {
		config.RoleClaim = "roles"
	}
	if config.CacheTTL <= 0 {
		config.CacheTTL = 5 * time.Minute
	}
	if config.ClockSkew < 0 || config.ClockSkew > 2*time.Minute {
		return nil, errors.New("OIDC clock skew must be between 0 and 2 minutes")
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 5 * time.Second}
	}
	clientCopy := *config.HTTPClient
	clientCopy.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	config.HTTPClient = &clientCopy
	return &Verifier{config: config, keys: make(map[string]*rsa.PublicKey)}, nil
}

func (v *Verifier) Verify(ctx context.Context, raw string) (Principal, error) {
	if len(raw) == 0 || len(raw) > maxTokenBytes {
		return Principal{}, errors.New("token is empty or too large")
	}
	claims := &oidcClaims{}
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithIssuer(v.config.Issuer),
		jwt.WithAudience(v.config.Audience),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(v.config.ClockSkew),
	)
	token, err := parser.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodRS256 {
			return nil, fmt.Errorf("unsupported signing algorithm")
		}
		kid, ok := token.Header["kid"].(string)
		if !ok || kid == "" || len(kid) > 256 {
			return nil, errors.New("token key id is missing or invalid")
		}
		return v.keyFor(ctx, kid)
	})
	if err != nil || token == nil || !token.Valid {
		return Principal{}, errors.New("token verification failed")
	}
	if claims.Subject == "" || len(claims.Subject) > 512 {
		return Principal{}, errors.New("token subject is missing or invalid")
	}
	roles, err := v.mappedRoles(claims)
	if err != nil {
		return Principal{}, err
	}
	if _, ok := roles[RoleReader]; !ok {
		// Every accepted identity receives read access, but no write role is
		// inferred. Elevated roles still require an explicit trusted mapping.
		roles[RoleReader] = struct{}{}
	}
	kind := KindHuman
	clientID := claims.AZP
	if clientID == "" {
		clientID = claims.ClientID
	}
	if _, trustedService := v.config.ServiceClientIDs[clientID]; clientID != "" && trustedService {
		kind = KindService
	}
	return Principal{Subject: claims.Subject, Kind: kind, Roles: roles}, nil
}

func (v *Verifier) mappedRoles(claims *oidcClaims) (map[Role]struct{}, error) {
	var values []string
	switch v.config.RoleClaim {
	case "roles":
		values = claims.Roles
	case "groups":
		values = claims.Groups
	default:
		return nil, fmt.Errorf("unsupported configured role claim %q", v.config.RoleClaim)
	}
	roles := make(map[Role]struct{})
	for _, value := range values {
		role, ok := v.config.RoleMappings[value]
		if !ok {
			continue
		}
		switch role {
		case RoleReader, RoleOperator, RoleAdmin:
			roles[role] = struct{}{}
		default:
			return nil, fmt.Errorf("configured role mapping contains unsupported role")
		}
	}
	return roles, nil
}

func (v *Verifier) keyFor(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	key, exists := v.keys[kid]
	if exists && time.Since(v.fetchedAt) < v.config.CacheTTL {
		return key, nil
	}
	if err := v.refresh(ctx); err != nil {
		return nil, err
	}
	key, exists = v.keys[kid]
	if !exists {
		return nil, errors.New("token key id is not trusted")
	}
	return key, nil
}

func (v *Verifier) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.config.JWKSURL, nil)
	if err != nil {
		return fmt.Errorf("create JWKS request: %w", err)
	}
	resp, err := v.config.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch JWKS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch JWKS: unexpected HTTP status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxJWKSBytes+1))
	if err != nil {
		return fmt.Errorf("read JWKS: %w", err)
	}
	if len(data) > maxJWKSBytes {
		return errors.New("JWKS response is too large")
	}
	var document jwksDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("decode JWKS: %w", err)
	}
	keys := make(map[string]*rsa.PublicKey, len(document.Keys))
	for _, item := range document.Keys {
		if item.KID == "" || item.KTY != "RSA" || (item.USE != "" && item.USE != "sig") || (item.ALG != "" && item.ALG != jwt.SigningMethodRS256.Alg()) {
			continue
		}
		key, err := rsaKey(item.N, item.E)
		if err == nil {
			keys[item.KID] = key
		}
	}
	if len(keys) == 0 {
		return errors.New("JWKS contains no supported RS256 signing keys")
	}
	v.keys = keys
	v.fetchedAt = time.Now()
	return nil
}

func rsaKey(modulus, exponent string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(modulus)
	if err != nil || len(nBytes) < 256 || len(nBytes) > 512 {
		return nil, errors.New("invalid RSA modulus")
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(exponent)
	if err != nil || len(eBytes) == 0 || len(eBytes) > 4 {
		return nil, errors.New("invalid RSA exponent")
	}
	e := 0
	for _, b := range eBytes {
		e = e<<8 | int(b)
	}
	if e < 3 || e%2 == 0 {
		return nil, errors.New("invalid RSA exponent")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}, nil
}
