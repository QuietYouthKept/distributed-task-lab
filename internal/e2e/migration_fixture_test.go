package e2e

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/artifacts"
)

// TestMigrationArtifactFixture is an explicit helper for the Wave 4 migration
// evidence script. It writes or verifies a real S3 object and is inert during
// normal test runs.
func TestMigrationArtifactFixture(t *testing.T) {
	if os.Getenv("W4_MIGRATION_FIXTURE") != "1" {
		t.Skip("W4_MIGRATION_FIXTURE=1 is required for migration evidence")
	}
	endpoint, bucket := os.Getenv("MINIO_ENDPOINT"), os.Getenv("MINIO_BUCKET")
	access, secret := os.Getenv("MINIO_ACCESS_KEY"), os.Getenv("MINIO_SECRET_KEY")
	for name, value := range map[string]string{"MINIO_ENDPOINT": endpoint, "MINIO_BUCKET": bucket, "MINIO_ACCESS_KEY": access, "MINIO_SECRET_KEY": secret} {
		if value == "" {
			t.Fatalf("%s is required", name)
		}
	}
	store, err := artifacts.NewS3Store(artifacts.S3Config{
		Endpoint: endpoint, Bucket: bucket, Prefix: os.Getenv("ARTIFACT_S3_PREFIX"),
		AccessKey: access, SecretKey: secret, Region: "us-east-1", AllowHTTP: true,
		AllowBucketCreate: true, MaxBytes: 1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatal(err)
	}
	key := os.Getenv("W4_MIGRATION_ARTIFACT_KEY")
	if key == "" {
		t.Fatal("W4_MIGRATION_ARTIFACT_KEY is required")
	}
	switch os.Getenv("W4_MIGRATION_ARTIFACT_MODE") {
	case "put-source":
		body := []byte("Wave 4 migration fixture source.\n\nSecond paragraph verifies deterministic chunk output.\n")
		info, err := store.PutIfAbsent(ctx, key, body, "text/markdown")
		if err != nil {
			t.Fatal(err)
		}
		if info.SHA256 != digest(body) {
			t.Fatalf("source checksum mismatch: store=%s local=%s", info.SHA256, digest(body))
		}
		t.Logf("MIGRATION_ARTIFACT_SOURCE key=%s sha256=%s size=%d", info.Key, info.SHA256, info.Size)
	case "verify-output":
		body, info, err := store.Get(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if info.SHA256 != digest(body) {
			t.Fatalf("output checksum mismatch: store=%s local=%s", info.SHA256, digest(body))
		}
		if want := os.Getenv("W4_MIGRATION_ARTIFACT_SHA256"); want != "" && info.SHA256 != want {
			t.Fatalf("output checksum=%s, want %s", info.SHA256, want)
		}
		t.Logf("MIGRATION_ARTIFACT_OUTPUT key=%s sha256=%s size=%d", info.Key, info.SHA256, info.Size)
	default:
		t.Fatal(fmt.Errorf("unsupported W4_MIGRATION_ARTIFACT_MODE %q", os.Getenv("W4_MIGRATION_ARTIFACT_MODE")))
	}
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
