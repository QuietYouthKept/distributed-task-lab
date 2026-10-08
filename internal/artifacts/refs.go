// Package artifacts validates references supplied to worker adapters.
package artifacts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveLocalObjectKey resolves an object-style key below root without
// allowing callers to select an arbitrary worker filesystem path. It rejects
// absolute paths, traversal, URL-encoded separators, alternate separators,
// and symlink escapes.
func ResolveLocalObjectKey(root, key string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("artifact root is required")
	}
	if err := validateObjectKey(key); err != nil {
		return "", err
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve artifact root: %w", err)
	}
	path := filepath.Join(resolvedRoot, filepath.FromSlash(key))
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve object key: %w", err)
	}
	relative, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("object key resolves outside configured artifact root")
	}
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return "", fmt.Errorf("stat object key: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("object key must reference a file")
	}
	return resolvedPath, nil
}

func validateObjectKey(key string) error {
	key = strings.TrimSpace(key)
	if key == "" || len(key) > 1024 {
		return fmt.Errorf("object_key must be between 1 and 1024 characters")
	}
	if filepath.IsAbs(key) || strings.Contains(key, "\\") || strings.Contains(key, ":") || strings.Contains(key, "%") || strings.ContainsAny(key, "\u2215\uff0f\u29f8") {
		return fmt.Errorf("object_key must be a relative slash-separated key")
	}
	for _, segment := range strings.Split(key, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("object_key contains an invalid path segment")
		}
	}
	return nil
}
