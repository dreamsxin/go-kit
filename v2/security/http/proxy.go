package httpsecurity

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
)

type clientIPKey struct{}
type schemeKey struct{}

// TrustedProxyConfig defines which direct peers may supply forwarding
// headers. Header names default to X-Forwarded-For and X-Forwarded-Proto.
type TrustedProxyConfig struct {
	TrustedProxies       []string
	ForwardedForHeader   string
	ForwardedProtoHeader string
}

// NewTrustedProxy resolves the effective client IP and scheme. Forwarding
// headers are ignored unless the direct peer matches TrustedProxies.
func NewTrustedProxy(config TrustedProxyConfig) (Middleware, error) {
	networks, err := parseNetworks(config.TrustedProxies)
	if err != nil {
		return nil, fmt.Errorf("httpsecurity: trusted proxy: %w", err)
	}
	forwardedFor := http.CanonicalHeaderKey(strings.TrimSpace(config.ForwardedForHeader))
	if forwardedFor == "" {
		forwardedFor = "X-Forwarded-For"
	}
	forwardedProto := http.CanonicalHeaderKey(strings.TrimSpace(config.ForwardedProtoHeader))
	if forwardedProto == "" {
		forwardedProto = "X-Forwarded-Proto"
	}
	if !validHeaderName(forwardedFor) || !validHeaderName(forwardedProto) {
		return nil, fmt.Errorf("httpsecurity: invalid forwarding header name")
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			peer, err := remoteIP(request.RemoteAddr)
			if err != nil {
				http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
				return
			}
			client := peer
			scheme := requestScheme(request)
			if containsIP(networks, peer) {
				if value := request.Header.Get(forwardedFor); value != "" {
					client, err = forwardedClientIP(value, peer, networks)
					if err != nil {
						http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
						return
					}
				}
				if value := request.Header.Get(forwardedProto); value != "" {
					scheme, err = forwardedScheme(value)
					if err != nil {
						http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
						return
					}
				}
			}
			ctx := context.WithValue(request.Context(), clientIPKey{}, cloneIP(client))
			ctx = context.WithValue(ctx, schemeKey{}, scheme)
			next.ServeHTTP(w, request.WithContext(ctx))
		})
	}, nil
}

// ClientIPFromContext returns a copy of the effective client IP installed by
// NewTrustedProxy.
func ClientIPFromContext(ctx context.Context) (net.IP, bool) {
	ip, ok := ctx.Value(clientIPKey{}).(net.IP)
	if !ok || ip == nil {
		return nil, false
	}
	return cloneIP(ip), true
}

// SchemeFromContext returns the trusted effective request scheme.
func SchemeFromContext(ctx context.Context) (string, bool) {
	scheme, ok := ctx.Value(schemeKey{}).(string)
	return scheme, ok && scheme != ""
}

func effectiveClientIP(request *http.Request) (net.IP, error) {
	if ip, ok := ClientIPFromContext(request.Context()); ok {
		return ip, nil
	}
	return remoteIP(request.RemoteAddr)
}

func effectiveScheme(request *http.Request) string {
	if scheme, ok := SchemeFromContext(request.Context()); ok {
		return scheme
	}
	return requestScheme(request)
}

func requestScheme(request *http.Request) string {
	if request.TLS != nil {
		return "https"
	}
	return "http"
}

func remoteIP(remoteAddr string) (net.IP, error) {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err != nil {
		host = strings.TrimSpace(remoteAddr)
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		return nil, fmt.Errorf("invalid remote address %q", remoteAddr)
	}
	return ip, nil
}

func forwardedClientIP(value string, peer net.IP, trusted []*net.IPNet) (net.IP, error) {
	parts := strings.Split(value, ",")
	client := peer
	for i := len(parts) - 1; i >= 0; i-- {
		candidate := net.ParseIP(strings.TrimSpace(parts[i]))
		if candidate == nil {
			return nil, fmt.Errorf("invalid forwarded client IP")
		}
		client = candidate
		if !containsIP(trusted, candidate) {
			break
		}
	}
	return client, nil
}

func forwardedScheme(value string) (string, error) {
	parts := strings.Split(value, ",")
	scheme := strings.ToLower(strings.TrimSpace(parts[len(parts)-1]))
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("invalid forwarded scheme")
	}
	return scheme, nil
}

func parseNetworks(values []string) ([]*net.IPNet, error) {
	networks := make([]*net.IPNet, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, fmt.Errorf("empty IP network")
		}
		if ip := net.ParseIP(value); ip != nil {
			bits := 128
			if ip.To4() != nil {
				ip = ip.To4()
				bits = 32
			}
			networks = append(networks, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			continue
		}
		_, network, err := net.ParseCIDR(value)
		if err != nil {
			return nil, fmt.Errorf("invalid IP network %q", value)
		}
		networks = append(networks, network)
	}
	return networks, nil
}

func containsIP(networks []*net.IPNet, ip net.IP) bool {
	for _, network := range networks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func cloneIP(ip net.IP) net.IP {
	return append(net.IP(nil), ip...)
}
