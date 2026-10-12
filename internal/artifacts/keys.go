package artifacts

import (
	"path"
	"strings"
)

// ValidateKey accepts only normalized, relative object keys. Keys are never
// interpreted as URLs or paths outside the configured store root/prefix.
func ValidateKey(key string) error {
	if key == "" || len(key) > 1024 || strings.TrimSpace(key) != key || strings.ContainsAny(key, "\\:%") || strings.Contains(key, "//") || strings.HasPrefix(key, "/") {
		return ErrInvalidKey
	}
	for _, segment := range strings.Split(key, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return ErrInvalidKey
		}
	}
	if path.Clean(key) != key {
		return ErrInvalidKey
	}
	return nil
}
