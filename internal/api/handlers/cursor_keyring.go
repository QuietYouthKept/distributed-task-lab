package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	rotatingCursorVersion = 2
	cursorFutureClockSkew = 30 * time.Second
	maxRotatingCursorTTL  = 7 * 24 * time.Hour
)

var cursorKeyIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

type CursorVerificationKey struct {
	ID          string
	Key         []byte
	AcceptUntil time.Time
}

// CursorKeyRingConfig is only used by the explicitly opt-in rotation path.
// The default constructor continues to issue and accept the compatible v1
// cursor format indefinitely, as before.
type CursorKeyRingConfig struct {
	ActiveKeyID         string
	ActiveKey           []byte
	PreviousKeys        []CursorVerificationKey
	TTL                 time.Duration
	LegacyV1Key         []byte
	LegacyV1AcceptUntil time.Time
}

type rotatingCursorCodec struct {
	activeID  string
	activeKey []byte
	previous  map[string]CursorVerificationKey
	ttl       time.Duration
	legacy    *cursorCodec
	legacyEnd time.Time
	now       func() time.Time
}

type rotatingTaskCursor struct {
	Version   int    `json:"v"`
	KeyID     string `json:"kid"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
	CreatedAt string `json:"created_at"`
	ID        string `json:"id"`
	Filter    string `json:"filter"`
	PageSize  int    `json:"page_size"`
	Subject   string `json:"subject"`
}

func newRotatingCursorCodec(config CursorKeyRingConfig) (*rotatingCursorCodec, error) {
	if !cursorKeyIDPattern.MatchString(config.ActiveKeyID) || len(config.ActiveKey) < 32 {
		return nil, fmt.Errorf("cursor key ring requires a valid active key ID and at least 32 key bytes")
	}
	if config.TTL < time.Second || config.TTL > maxRotatingCursorTTL {
		return nil, fmt.Errorf("cursor key ring TTL must be between 1 second and 7 days")
	}
	codec := &rotatingCursorCodec{activeID: config.ActiveKeyID, activeKey: append([]byte(nil), config.ActiveKey...),
		previous: make(map[string]CursorVerificationKey, len(config.PreviousKeys)), ttl: config.TTL, now: time.Now}
	for _, key := range config.PreviousKeys {
		if !cursorKeyIDPattern.MatchString(key.ID) || key.ID == config.ActiveKeyID || len(key.Key) < 32 || key.AcceptUntil.IsZero() {
			return nil, fmt.Errorf("cursor previous key requires a distinct valid ID, at least 32 key bytes, and accept-until time")
		}
		if _, exists := codec.previous[key.ID]; exists {
			return nil, fmt.Errorf("cursor key ring contains duplicate previous key IDs")
		}
		key.Key = append([]byte(nil), key.Key...)
		codec.previous[key.ID] = key
	}
	if len(config.LegacyV1Key) > 0 {
		legacy, err := newCursorCodec(config.LegacyV1Key)
		if err != nil || config.LegacyV1AcceptUntil.IsZero() {
			return nil, fmt.Errorf("legacy v1 cursor acceptance requires a 32-byte key and finite accept-until time")
		}
		codec.legacy = legacy
		codec.legacyEnd = config.LegacyV1AcceptUntil
	} else if !config.LegacyV1AcceptUntil.IsZero() {
		return nil, fmt.Errorf("legacy v1 cursor accept-until requires its signing key")
	}
	return codec, nil
}

func (c *rotatingCursorCodec) encode(cursor taskCursor) string {
	now := c.now()
	rotating := rotatingTaskCursor{Version: rotatingCursorVersion, KeyID: c.activeID,
		IssuedAt: now.Unix(), ExpiresAt: now.Add(c.ttl).Unix(), CreatedAt: cursor.CreatedAt,
		ID: cursor.ID, Filter: cursor.Filter, PageSize: cursor.PageSize, Subject: cursor.Subject}
	raw, _ := json.Marshal(rotating)
	payload := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, c.activeKey)
	_, _ = mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (c *rotatingCursorCodec) decode(token, expectedFilter string, pageSize int, subject string) (time.Time, string, error) {
	if len(token) == 0 || len(token) > 2048 {
		return time.Time{}, "", errInvalidCursor
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return time.Time{}, "", errInvalidCursor
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return time.Time{}, "", errInvalidCursor
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, "", errInvalidCursor
	}
	var header struct {
		Version int    `json:"v"`
		KeyID   string `json:"kid"`
	}
	if json.Unmarshal(raw, &header) != nil {
		return time.Time{}, "", errInvalidCursor
	}
	now := c.now()
	var key []byte
	var previousUntil time.Time
	if header.Version == 1 && c.legacy != nil && !now.After(c.legacyEnd) {
		key = c.legacy.key
	} else if header.Version == rotatingCursorVersion {
		if header.KeyID == c.activeID {
			key = c.activeKey
		} else if previous, ok := c.previous[header.KeyID]; ok && !now.After(previous.AcceptUntil) {
			key, previousUntil = previous.Key, previous.AcceptUntil
		} else {
			return time.Time{}, "", errInvalidCursor
		}
	} else {
		return time.Time{}, "", errInvalidCursor
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(parts[0]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return time.Time{}, "", errInvalidCursor
	}
	if header.Version == 1 {
		var cursor taskCursor
		if json.Unmarshal(raw, &cursor) != nil || cursor.Version != 1 || !cursor.matches(expectedFilter, pageSize, subject) {
			return time.Time{}, "", errInvalidCursor
		}
		createdAt, err := time.Parse(time.RFC3339Nano, cursor.CreatedAt)
		if err != nil {
			return time.Time{}, "", errInvalidCursor
		}
		return createdAt, cursor.ID, nil
	}
	var cursor rotatingTaskCursor
	if json.Unmarshal(raw, &cursor) != nil || cursor.Version != rotatingCursorVersion || !cursor.matches(expectedFilter, pageSize, subject) {
		return time.Time{}, "", errInvalidCursor
	}
	issuedAt, expiresAt := time.Unix(cursor.IssuedAt, 0), time.Unix(cursor.ExpiresAt, 0)
	if issuedAt.After(now.Add(cursorFutureClockSkew)) || !expiresAt.After(now) || !expiresAt.After(issuedAt) || expiresAt.Sub(issuedAt) > c.ttl {
		return time.Time{}, "", errInvalidCursor
	}
	if !previousUntil.IsZero() && now.After(previousUntil) {
		return time.Time{}, "", errInvalidCursor
	}
	createdAt, err := time.Parse(time.RFC3339Nano, cursor.CreatedAt)
	if err != nil {
		return time.Time{}, "", errInvalidCursor
	}
	return createdAt, cursor.ID, nil
}

func (c taskCursor) matches(filter string, pageSize int, subject string) bool {
	return c.Version == 1 && c.ID != "" && c.Filter == filter && c.PageSize == pageSize && c.Subject == subject
}

func (c rotatingTaskCursor) matches(filter string, pageSize int, subject string) bool {
	return c.ID != "" && c.Filter == filter && c.PageSize == pageSize && c.Subject == subject
}
