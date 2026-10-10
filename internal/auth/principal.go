// Package auth verifies service-issued identities and carries trusted
// principals through request contexts. Caller-controlled task fields are never
// used to construct a Principal.
package auth

import "context"

type Role string

const (
	RoleReader   Role = "reader"
	RoleOperator Role = "operator"
	RoleAdmin    Role = "admin"
)

type Kind string

const (
	KindHuman   Kind = "human"
	KindService Kind = "service"
)

type Principal struct {
	Subject string
	Kind    Kind
	Roles   map[Role]struct{}
}

func (p Principal) HasRole(role Role) bool {
	_, ok := p.Roles[role]
	if ok {
		return true
	}
	if role == RoleReader {
		_, ok = p.Roles[RoleOperator]
		if ok {
			return true
		}
		_, ok = p.Roles[RoleAdmin]
	}
	if role == RoleOperator {
		_, ok = p.Roles[RoleAdmin]
	}
	return ok
}

func (p Principal) IsAdmin() bool { return p.HasRole(RoleAdmin) }

type contextKey struct{}

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, contextKey{}, principal)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(contextKey{}).(Principal)
	return p, ok && p.Subject != ""
}

// TokenVerifier is implemented by the OIDC/JWKS verifier and allows router
// tests to inject controlled identities without weakening production auth.
type TokenVerifier interface {
	Verify(context.Context, string) (Principal, error)
}
