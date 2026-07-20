package httpsecurity

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultCSRFCookie  = "csrf_token"
	defaultCSRFHeader  = "X-CSRF-Token"
	csrfNonceBytes     = 32
	minCSRFSecretBytes = 32
)

// CSRFConfig defines signed double-submit-cookie protection for browser routes
// that authenticate with cookies. Secret must contain at least 32 bytes.
type CSRFConfig struct {
	Secret         []byte
	CookieName     string
	HeaderName     string
	TrustedOrigins []string
	RequireOrigin  bool
	SecureCookie   bool
	SameSite       http.SameSite
	CookieMaxAge   time.Duration
}

type csrfPolicy struct {
	secret         []byte
	cookieName     string
	headerName     string
	trustedOrigins map[string]struct{}
	requireOrigin  bool
	secureCookie   bool
	sameSite       http.SameSite
	cookieMaxAge   time.Duration
}

// NewCSRF creates signed double-submit-cookie middleware. Safe requests ensure
// a valid token cookie exists; unsafe requests require the same valid token in
// the configured header.
func NewCSRF(config CSRFConfig) (Middleware, error) {
	if len(config.Secret) < minCSRFSecretBytes {
		return nil, fmt.Errorf("httpsecurity: CSRF secret must contain at least %d bytes", minCSRFSecretBytes)
	}
	cookieName := strings.TrimSpace(config.CookieName)
	if cookieName == "" {
		cookieName = defaultCSRFCookie
	}
	if !validCookieName(cookieName) {
		return nil, fmt.Errorf("httpsecurity: invalid CSRF cookie name %q", cookieName)
	}
	headerName := http.CanonicalHeaderKey(strings.TrimSpace(config.HeaderName))
	if headerName == "" {
		headerName = defaultCSRFHeader
	}
	if !validHeaderName(headerName) {
		return nil, fmt.Errorf("httpsecurity: invalid CSRF header name %q", headerName)
	}
	if config.CookieMaxAge < 0 {
		return nil, fmt.Errorf("httpsecurity: CSRF cookie max age cannot be negative")
	}
	sameSite := config.SameSite
	if sameSite == 0 {
		sameSite = http.SameSiteLaxMode
	}
	if sameSite < http.SameSiteDefaultMode || sameSite > http.SameSiteNoneMode {
		return nil, fmt.Errorf("httpsecurity: invalid CSRF SameSite mode")
	}
	if sameSite == http.SameSiteNoneMode && !config.SecureCookie {
		return nil, fmt.Errorf("httpsecurity: SameSite=None requires a secure CSRF cookie")
	}
	if strings.HasPrefix(cookieName, "__Host-") && !config.SecureCookie {
		return nil, fmt.Errorf("httpsecurity: __Host- CSRF cookie requires Secure")
	}
	if strings.HasPrefix(cookieName, "__Secure-") && !config.SecureCookie {
		return nil, fmt.Errorf("httpsecurity: __Secure- CSRF cookie requires Secure")
	}
	origins := make(map[string]struct{}, len(config.TrustedOrigins))
	for _, origin := range config.TrustedOrigins {
		normalized, err := normalizedOrigin(origin)
		if err != nil || normalized == "null" {
			return nil, fmt.Errorf("httpsecurity: CSRF invalid trusted origin %q", origin)
		}
		origins[normalized] = struct{}{}
	}
	policy := &csrfPolicy{
		secret:         append([]byte(nil), config.Secret...),
		cookieName:     cookieName,
		headerName:     headerName,
		trustedOrigins: origins,
		requireOrigin:  config.RequireOrigin,
		secureCookie:   config.SecureCookie,
		sameSite:       sameSite,
		cookieMaxAge:   config.CookieMaxAge,
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			if isSafeMethod(request.Method) {
				cookie, err := request.Cookie(policy.cookieName)
				if err != nil || !policy.validToken(cookie.Value) {
					token, err := policy.newToken()
					if err != nil {
						http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
						return
					}
					policy.setCookie(w, token)
				}
				next.ServeHTTP(w, request)
				return
			}
			if !policy.validRequestOrigin(request) {
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
				return
			}
			cookie, err := request.Cookie(policy.cookieName)
			headerToken := request.Header.Get(policy.headerName)
			if err != nil || !policy.validToken(cookie.Value) || !policy.validToken(headerToken) ||
				subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(headerToken)) != 1 {
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, request)
		})
	}, nil
}

func (policy *csrfPolicy) newToken() (string, error) {
	nonce := make([]byte, csrfNonceBytes)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	encodedNonce := base64.RawURLEncoding.EncodeToString(nonce)
	signature := policy.sign(encodedNonce)
	return encodedNonce + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (policy *csrfPolicy) validToken(token string) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return false
	}
	nonce, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || len(nonce) != csrfNonceBytes {
		return false
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	expected := policy.sign(parts[0])
	return hmac.Equal(signature, expected)
}

func (policy *csrfPolicy) sign(value string) []byte {
	mac := hmac.New(sha256.New, policy.secret)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}

func (policy *csrfPolicy) setCookie(w http.ResponseWriter, token string) {
	cookie := &http.Cookie{
		Name:     policy.cookieName,
		Value:    token,
		Path:     "/",
		Secure:   policy.secureCookie,
		HttpOnly: false,
		SameSite: policy.sameSite,
	}
	if policy.cookieMaxAge > 0 {
		cookie.MaxAge = int(policy.cookieMaxAge / time.Second)
		cookie.Expires = time.Now().Add(policy.cookieMaxAge)
	}
	http.SetCookie(w, cookie)
}

func (policy *csrfPolicy) validRequestOrigin(request *http.Request) bool {
	value := strings.TrimSpace(request.Header.Get("Origin"))
	if value == "" {
		if referer := strings.TrimSpace(request.Referer()); referer != "" {
			parsed, err := url.Parse(referer)
			if err != nil || parsed.Scheme == "" || parsed.Host == "" {
				return false
			}
			value = parsed.Scheme + "://" + parsed.Host
		}
	}
	if value == "" {
		return !policy.requireOrigin
	}
	normalized, err := normalizedOrigin(value)
	if err != nil || normalized == "null" {
		return false
	}
	if normalized == effectiveRequestOrigin(request) {
		return true
	}
	_, ok := policy.trustedOrigins[normalized]
	return ok
}

func effectiveRequestOrigin(request *http.Request) string {
	return strings.ToLower(effectiveScheme(request) + "://" + request.Host)
}

func validCookieName(value string) bool {
	return validHeaderName(value)
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}
