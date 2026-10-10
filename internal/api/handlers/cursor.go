package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var errInvalidCursor = errors.New("invalid cursor")

type taskCursor struct {
	Version   int    `json:"v"`
	CreatedAt string `json:"created_at"`
	ID        string `json:"id"`
	Filter    string `json:"filter"`
	PageSize  int    `json:"page_size"`
	Subject   string `json:"subject"`
}

type cursorCodec struct{ key []byte }

func newCursorCodec(key []byte) (*cursorCodec, error) {
	if len(key) < 32 {
		return nil, fmt.Errorf("cursor signing key must contain at least 32 bytes")
	}
	return &cursorCodec{key: append([]byte(nil), key...)}, nil
}

func (c *cursorCodec) encode(cursor taskCursor) string {
	raw, _ := json.Marshal(cursor)
	payload := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, c.key)
	_, _ = mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (c *cursorCodec) decode(token string, expectedFilter string, pageSize int, subject string) (time.Time, string, error) {
	if len(token) == 0 || len(token) > 2048 {
		return time.Time{}, "", errInvalidCursor
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return time.Time{}, "", errInvalidCursor
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, "", errInvalidCursor
	}
	mac := hmac.New(sha256.New, c.key)
	_, _ = mac.Write([]byte(parts[0]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return time.Time{}, "", errInvalidCursor
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return time.Time{}, "", errInvalidCursor
	}
	var cursor taskCursor
	if err := json.Unmarshal(raw, &cursor); err != nil || cursor.Version != 1 || cursor.Filter != expectedFilter || cursor.PageSize != pageSize || cursor.Subject != subject || cursor.ID == "" {
		return time.Time{}, "", errInvalidCursor
	}
	createdAt, err := time.Parse(time.RFC3339Nano, cursor.CreatedAt)
	if err != nil {
		return time.Time{}, "", errInvalidCursor
	}
	return createdAt, cursor.ID, nil
}

func taskFilterFingerprint(status, taskType, queue, subject string) string {
	mac := sha256.Sum256([]byte(strings.Join([]string{status, taskType, queue, subject}, "\x00")))
	return base64.RawURLEncoding.EncodeToString(mac[:])
}
