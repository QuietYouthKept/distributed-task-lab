package observability

import (
	"regexp"
	"sort"
	"strings"
)

var (
	bearerCredential = regexp.MustCompile(`(?i)(\bbearer\s+)[A-Za-z0-9._~+/-]+=*`)
	urlCredential    = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://[^/\s:@]+:)[^@/\s]+(@)`)
	keyCredential    = regexp.MustCompile(`(?i)\b(password|passwd|token|secret|api[_-]?key|access[_-]?key)=([^&\s,;]+)`)
)

func sensitiveAttribute(key string) bool {
	key = strings.ToLower(key)
	key = strings.NewReplacer("_", "", "-", "", ".", "").Replace(key)
	for _, marker := range []string{"password", "passwd", "secret", "token", "authorization", "credential", "apikey", "accesskey"} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}

func redactText(value string, secrets []string) string {
	value = bearerCredential.ReplaceAllString(value, "${1}[REDACTED]")
	value = urlCredential.ReplaceAllString(value, "${1}[REDACTED]${2}")
	value = keyCredential.ReplaceAllString(value, "${1}=[REDACTED]")
	ordered := append([]string(nil), secrets...)
	sort.Slice(ordered, func(i, j int) bool { return len(ordered[i]) > len(ordered[j]) })
	for _, secret := range ordered {
		if len(secret) >= 4 {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	return value
}
