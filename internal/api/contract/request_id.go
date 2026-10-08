package contract

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"unicode"

	"github.com/google/uuid"
)

type requestIDKey struct{}

func WithRequestID(ctx context.Context, value string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, value)
}

func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey{}).(string)
	return value
}

// NormalizeRequestID accepts bounded opaque correlation IDs. Invalid caller
// values are replaced instead of appearing in logs or response headers.
func NormalizeRequestID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 16 && len(value) <= 128 {
		valid := true
		for _, r := range value {
			if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.') {
				valid = false
				break
			}
		}
		if valid {
			return value
		}
	}
	return uuid.NewString()
}

func jsonEncoder(w io.Writer, value any) error { return json.NewEncoder(w).Encode(value) }
