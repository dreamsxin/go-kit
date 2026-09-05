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
	"strconv"
	"strings"
	"time"
)

const (
	defaultCSRFCookie  = "csrf_token"
	defaultCSRFHeader  = "X-CSRF-Token"
	csrfNonceBytes     = 32
	minCSRFSecretBytes = 32

	// DefaultCSRFTokenTTL is how long a minted token stays valid when
	// CSRFConfig.TokenTTL is zero.
	DefaultCSRFTokenTTL = 12 * time.Hour

	// csrfClockSkew is the tolerance for a token that appears to come from the
	// future, which happens when a fleet's clocks disagree.
	csrfClockSkew = time.Minute
)

// CSRFConfig defines signed double-submit-cookie protection for browser routes
// that authenticate with cookies. Secret must contain at least 32 bytes.
//
// SessionID is required: a token is signed together with the session it was
// minted for, so it authorizes that session alone. Return the value that
// identifies the caller's session — the session cookie is the usual answer —
// and report false when the request carries no session. An unsafe request
// without a session is refused; a safe request without one is served without a
// token, and the token is minted on the first request after sign-in.
//
// Set AssumeHTTPS when TLS terminates upstream and this process serves
// plaintext, so that the same-origin comparison against the browser's Origin
// header holds behind a load balancer.
type CSRFConfig struct {
	Secret         []byte
	SessionID      func(*http.Request) (string, bool)
	TokenTTL       time.Duration
	AssumeHTTPS    bool
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
	sessionID      func(*http.Request) (string, bool)
	tokenTTL       time.Duration
	assumeHTTPS    bool
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
	if config.SessionID == nil {
		return nil, fmt.Errorf("httpsecurity: CSRF requires a SessionID function so a token authorizes one session")
	}
	if config.TokenTTL < 0 {
		return nil, fmt.Errorf("httpsecurity: CSRF token TTL cannot be negative")
	}
	tokenTTL := config.TokenTTL
	if tokenTTL == 0 {
		tokenTTL = DefaultCSRFTokenTTL
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
		sessionID:      config.SessionID,
		tokenTTL:       tokenTTL,
		assumeHTTPS:    config.AssumeHTTPS,
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
			session, hasSession := policy.sessionID(request)
			if isSafeMethod(request.Method) {
				if hasSession && !policy.hasValidCookie(request, session) {
					token, err := policy.newToken(session, time.Now())
					if err != nil {
						http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
						return
					}
					// A minted token belongs to one session, so an intermediary
					// must not store this response for anyone else.
					w.Header().Set("Cache-Control", "no-store")
					addVary(w.Header(), "Cookie")
					policy.setCookie(w, token)
				}
				next.ServeHTTP(w, request)
				return
			}
			if !hasSession || !policy.validRequestOrigin(request) {
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
				return
			}
			cookie, err := request.Cookie(policy.cookieName)
			headerToken := request.Header.Get(policy.headerName)
			now := time.Now()
			if err != nil || !policy.validToken(cookie.Value, session, now) || !policy.validToken(headerToken, session, now) ||
				subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(headerToken)) != 1 {
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, request)
		})
	}, nil
}

func (policy *csrfPolicy) hasValidCookie(request *http.Request, session string) bool {
	cookie, err := request.Cookie(policy.cookieName)
	if err != nil {
		return false
	}
	return policy.validToken(cookie.Value, session, time.Now())
}

func (policy *csrfPolicy) newToken(session string, now time.Time) (string, error) {
	nonce := make([]byte, csrfNonceBytes)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	encodedNonce := base64.RawURLEncoding.EncodeToString(nonce)
	issuedAt := strconv.FormatInt(now.Unix(), 10)
	signature := policy.sign(signingInput(encodedNonce, issuedAt, session))
	return encodedNonce + "." + issuedAt + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

// validToken reports whether token was minted by this policy, for this session,
// within the configured lifetime.
func (policy *csrfPolicy) validToken(token, session string, now time.Time) bool {
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		return false
	}
	nonce, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || len(nonce) != csrfNonceBytes {
		return false
	}
	issuedAtSeconds, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return false
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	expected := policy.sign(signingInput(parts[0], parts[1], session))
	if !hmac.Equal(signature, expected) {
		return false
	}
	issuedAt := time.Unix(issuedAtSeconds, 0)
	if now.Sub(issuedAt) > policy.tokenTTL {
		return false
	}
	return issuedAt.Sub(now) <= csrfClockSkew
}

// signingInput binds a token to its session and its issue time. The nonce is a
// fixed-width base64 value and the timestamp is digits, so the session — which
// an application may format however it likes — is unambiguous as the last field.
func signingInput(encodedNonce, issuedAt, session string) string {
	return encodedNonce + "." + issuedAt + "." + session
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
	if normalized == effectiveRequestOrigin(request, policy.assumeHTTPS) {
		return true
	}
	_, ok := policy.trustedOrigins[normalized]
	return ok
}

func effectiveRequestOrigin(request *http.Request, assumeHTTPS bool) string {
	return strings.ToLower(resolvedScheme(request, assumeHTTPS) + "://" + request.Host)
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
