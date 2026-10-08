package artifacts

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveLocalObjectKeyRejectsUnsafeReferences(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "safe.md"), []byte("safe"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveLocalObjectKey(root, "docs/safe.md"); err != nil {
		t.Fatalf("valid key: %v", err)
	}
	for _, key := range []string{"../secret", "/etc/passwd", "C:/Windows/win.ini", "docs\\safe.md", "docs/%2e%2e/secret", "docs/..", "docs\u2215safe.md"} {
		if _, err := ResolveLocalObjectKey(root, key); err == nil {
			t.Errorf("ResolveLocalObjectKey(%q) unexpectedly succeeded", key)
		}
	}
}

func TestResolveLocalObjectKeyRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows symlink creation requires elevated developer permissions")
	}
	root, outside := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.md"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(root, "link.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveLocalObjectKey(root, "link.md"); err == nil {
		t.Fatal("symlink escape unexpectedly succeeded")
	}
}
