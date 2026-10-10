package handlers

import (
	"strings"
	"testing"
	"time"
)

func TestCursorCodecBindsSignatureFiltersPageSizeAndPrincipal(t *testing.T) {
	codec, err := newCursorCodec([]byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	createdAt := time.Date(2026, 10, 9, 10, 11, 12, 123456789, time.UTC)
	cursor := taskCursor{Version: 1, CreatedAt: createdAt.Format(time.RFC3339Nano), ID: "task-1", Filter: "filter-a", PageSize: 25, Subject: "user-a"}
	token := codec.encode(cursor)
	gotTime, gotID, err := codec.decode(token, "filter-a", 25, "user-a")
	if err != nil || !gotTime.Equal(createdAt) || gotID != "task-1" {
		t.Fatalf("decode = (%s, %q, %v)", gotTime, gotID, err)
	}
	for _, tc := range []struct {
		filter  string
		size    int
		subject string
	}{
		{"filter-b", 25, "user-a"}, {"filter-a", 10, "user-a"}, {"filter-a", 25, "user-b"},
	} {
		if _, _, err := codec.decode(token, tc.filter, tc.size, tc.subject); err == nil {
			t.Fatalf("cursor unexpectedly accepted mismatched query/principal: %+v", tc)
		}
	}
	parts := strings.Split(token, ".")
	first := "A"
	if strings.HasPrefix(parts[1], first) {
		first = "B"
	}
	tampered := parts[0] + "." + first + parts[1][1:]
	if _, _, err := codec.decode(tampered, "filter-a", 25, "user-a"); err == nil {
		t.Fatal("tampered cursor was accepted")
	}
}
