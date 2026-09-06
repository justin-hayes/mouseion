package webapp

import (
	"net/url"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
)

func safeConnectionURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	parsed.User = nil
	query := parsed.Query()
	for key := range query {
		if sensitiveURLParameter(key) {
			query.Del(key)
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func connectionURLForEdit(connection domain.OpdsConnection) string {
	if connectionURLContainsCredentials(connection.URL) {
		return ""
	}
	return connection.URL
}

func connectionURLContainsCredentials(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return true
	}
	if parsed.User != nil {
		return true
	}
	for key := range parsed.Query() {
		if sensitiveURLParameter(key) {
			return true
		}
	}
	return false
}

func sensitiveURLParameter(key string) bool {
	parts := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(key)), func(r rune) bool {
		return r == '_' || r == '-' || r == '.' || r == ' '
	})
	for _, part := range parts {
		switch part {
		case "pass", "password", "passwd", "pwd", "passcode", "secret", "token", "auth", "authorization", "credential", "credentials", "session", "cookie", "signature", "sig", "key", "apikey", "accesskey":
			return true
		}
	}
	return false
}
