package handlers

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRotatingCursorKeyBoundariesAndBindings(t *testing.T) {
	base := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	oldKey := []byte(strings.Repeat("o", 32))
	newKey := []byte(strings.Repeat("n", 32))
	cursor := taskCursor{Version: 1, CreatedAt: base.Add(-time.Minute).Format(time.RFC3339Nano), ID: "task-1", Filter: "filters", PageSize: 25, Subject: "principal-1"}
	oldIssuer, err := newRotatingCursorCodec(CursorKeyRingConfig{ActiveKeyID: "old", ActiveKey: oldKey, TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	oldIssuer.now = func() time.Time { return base }
	oldToken := oldIssuer.encode(cursor)

	codec, err := newRotatingCursorCodec(CursorKeyRingConfig{ActiveKeyID: "new", ActiveKey: newKey,
		PreviousKeys: []CursorVerificationKey{{ID: "old", Key: oldKey, AcceptUntil: base.Add(45 * time.Second)}}, TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	codec.now = func() time.Time { return base.Add(30 * time.Second) }
	gotTime, gotID, err := codec.decode(oldToken, "filters", 25, "principal-1")
	if err != nil || gotID != cursor.ID || !gotTime.Equal(base.Add(-time.Minute)) {
		t.Fatalf("previous key overlap decode = %s/%s/%v", gotTime, gotID, err)
	}
	for _, tc := range []struct {
		filter  string
		size    int
		subject string
	}{{"other", 25, "principal-1"}, {"filters", 10, "principal-1"}, {"filters", 25, "principal-2"}} {
		if _, _, err := codec.decode(oldToken, tc.filter, tc.size, tc.subject); err == nil {
			t.Fatalf("previous cursor accepted changed binding %+v", tc)
		}
	}
	codec.now = func() time.Time { return base.Add(46 * time.Second) }
	if _, _, err := codec.decode(oldToken, "filters", 25, "principal-1"); err == nil {
		t.Fatal("previous key accepted after its overlap cutoff")
	}

	codec.now = func() time.Time { return base }
	newToken := codec.encode(cursor)
	secondInstance, err := newRotatingCursorCodec(CursorKeyRingConfig{ActiveKeyID: "new", ActiveKey: newKey,
		PreviousKeys: []CursorVerificationKey{{ID: "old", Key: oldKey, AcceptUntil: base.Add(45 * time.Second)}}, TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	secondInstance.now = func() time.Time { return base.Add(time.Second) }
	if _, id, err := secondInstance.decode(newToken, "filters", 25, "principal-1"); err != nil || id != cursor.ID {
		t.Fatalf("second instance could not validate active-key cursor: id=%q err=%v", id, err)
	}
	codec.now = func() time.Time { return base.Add(time.Hour) }
	if _, _, err := codec.decode(newToken, "filters", 25, "principal-1"); err == nil {
		t.Fatal("expired active-key cursor was accepted")
	}
	futureIssuer, err := newRotatingCursorCodec(CursorKeyRingConfig{ActiveKeyID: "new", ActiveKey: newKey, TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	futureIssuer.now = func() time.Time { return base.Add(31 * time.Second) }
	futureToken := futureIssuer.encode(cursor)
	codec.now = func() time.Time { return base }
	if _, _, err := codec.decode(futureToken, "filters", 25, "principal-1"); err == nil {
		t.Fatal("cursor issued beyond allowed future clock skew was accepted")
	}
	parts := strings.Split(newToken, ".")
	tampered := "A" + parts[0][1:] + "." + parts[1]
	codec.now = func() time.Time { return base.Add(time.Minute) }
	if _, _, err := codec.decode(tampered, "filters", 25, "principal-1"); err == nil {
		t.Fatal("tampered rotating cursor was accepted")
	}

	unknownIssuer, err := newRotatingCursorCodec(CursorKeyRingConfig{ActiveKeyID: "unknown", ActiveKey: []byte(strings.Repeat("u", 32)), TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	unknownIssuer.now = func() time.Time { return base }
	unknown := unknownIssuer.encode(cursor)
	codec.now = func() time.Time { return base.Add(time.Second) }
	if _, _, err := codec.decode(unknown, "filters", 25, "principal-1"); err == nil {
		t.Fatal("cursor with unknown key ID was accepted")
	}
}

func TestRotatingCursorLegacyV1CutoffAndConcurrentUse(t *testing.T) {
	base := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	legacyKey := []byte(strings.Repeat("l", 32))
	legacy, err := newCursorCodec(legacyKey)
	if err != nil {
		t.Fatal(err)
	}
	cursor := taskCursor{Version: 1, CreatedAt: base.Format(time.RFC3339Nano), ID: "task-legacy", Filter: "f", PageSize: 5, Subject: "user"}
	legacyToken := legacy.encode(cursor)
	codec, err := newRotatingCursorCodec(CursorKeyRingConfig{ActiveKeyID: "current", ActiveKey: []byte(strings.Repeat("c", 32)),
		TTL: time.Minute, LegacyV1Key: legacyKey, LegacyV1AcceptUntil: base.Add(30 * time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	codec.now = func() time.Time { return base.Add(10 * time.Second) }
	if _, id, err := codec.decode(legacyToken, "f", 5, "user"); err != nil || id != cursor.ID {
		t.Fatalf("legacy v1 compatibility = %q, %v", id, err)
	}
	codec.now = func() time.Time { return base.Add(31 * time.Second) }
	if _, _, err := codec.decode(legacyToken, "f", 5, "user"); err == nil {
		t.Fatal("legacy v1 cursor accepted after its explicit cutoff")
	}

	codec.now = func() time.Time { return base }
	token := codec.encode(cursor)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, id, err := codec.decode(token, "f", 5, "user"); err != nil || id != cursor.ID {
				t.Errorf("concurrent cursor decode = %q, %v", id, err)
			}
		}()
	}
	wg.Wait()
}

func TestRotatingCursorConfigRejectsUnboundedOrAmbiguousKeys(t *testing.T) {
	base := CursorKeyRingConfig{ActiveKeyID: "active", ActiveKey: []byte(strings.Repeat("a", 32)), TTL: time.Minute}
	tests := []CursorKeyRingConfig{
		{ActiveKeyID: "active", ActiveKey: []byte("short"), TTL: time.Minute},
		{ActiveKeyID: "active", ActiveKey: base.ActiveKey, TTL: 8 * 24 * time.Hour},
		{ActiveKeyID: "active", ActiveKey: base.ActiveKey, TTL: time.Minute,
			PreviousKeys: []CursorVerificationKey{{ID: "active", Key: base.ActiveKey, AcceptUntil: time.Now().Add(time.Minute)}}},
		{ActiveKeyID: "active", ActiveKey: base.ActiveKey, TTL: time.Minute,
			LegacyV1Key: base.ActiveKey},
	}
	for i, config := range tests {
		if _, err := newRotatingCursorCodec(config); err == nil {
			t.Errorf("invalid key ring config %d unexpectedly succeeded", i)
		}
	}
}
