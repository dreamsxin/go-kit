package httpsecurity

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func validHeaderName(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char > 127 || !isTokenChar(byte(char)) {
			return false
		}
	}
	return true
}

func validMethod(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		if !isTokenChar(value[i]) {
			return false
		}
	}
	return value == strings.ToUpper(value)
}

func isTokenChar(char byte) bool {
	if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' {
		return true
	}
	return strings.ContainsRune("!#$%&'*+-.^_`|~", rune(char))
}

func validHeaderValue(value string) bool {
	return !strings.ContainsAny(value, "\r\n")
}

func normalizedOrigin(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "null" {
		return value, nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid origin %q", value)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("invalid origin scheme %q", parsed.Scheme)
	}
	if parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("origin must not contain credentials, path, query, or fragment")
	}
	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host), nil
}

func addVary(header http.Header, values ...string) {
	existing := make(map[string]struct{})
	for _, line := range header.Values("Vary") {
		for _, value := range strings.Split(line, ",") {
			existing[http.CanonicalHeaderKey(strings.TrimSpace(value))] = struct{}{}
		}
	}
	for _, value := range values {
		value = http.CanonicalHeaderKey(value)
		if _, ok := existing[value]; ok {
			continue
		}
		header.Add("Vary", value)
		existing[value] = struct{}{}
	}
}
