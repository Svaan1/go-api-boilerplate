package httpserver

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"

	"github.com/svaan1/go-api-boilerplate/internal/platform/config"
)

type clientIPKey struct{}

// ClientIP returns the resolved client IP, falling back to the direct peer.
func ClientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	if ip, ok := r.Context().Value(clientIPKey{}).(string); ok {
		return ip
	}
	return peerIP(r.RemoteAddr)
}

// ClientIdentity resolves client IP from trusted proxy headers and stores it in context.
func ClientIdentity(cfg config.Config) func(http.Handler) http.Handler {
	trusted := make([]netip.Prefix, 0, len(cfg.HTTP.TrustedProxies))
	validConfig := true
	for _, cidr := range cfg.HTTP.TrustedProxies {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			validConfig = false
			break
		}
		trusted = append(trusted, prefix.Masked())
	}
	if !validConfig {
		trusted = nil
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := peerIP(r.RemoteAddr)
			if validConfig && isTrusted(ip, trusted) {
				if chain, present, valid := forwardedChain(r.Header); present {
					if valid {
						ip = firstUntrusted(chain, trusted, ip)
					}
				} else if chain, present, valid := xForwardedForChain(r.Header); present && valid {
					ip = firstUntrusted(chain, trusted, ip)
				}
			}
			ctx := context.WithValue(r.Context(), clientIPKey{}, ip)
			*r = *r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
}

func peerIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return ""
	}
	addr, err := netip.ParseAddr(host)
	if err != nil || addr.Zone() != "" {
		return ""
	}
	return addr.Unmap().String()
}

func isTrusted(ip string, trusted []netip.Prefix) bool {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	for _, prefix := range trusted {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func firstUntrusted(chain []string, trusted []netip.Prefix, peer string) string {
	for i := len(chain) - 1; i >= 0; i-- {
		if !isTrusted(chain[i], trusted) {
			return chain[i]
		}
	}
	return peer
}

// forwardedChain parses every Forwarded element or rejects the complete header.
func forwardedChain(header http.Header) (chain []string, present, valid bool) {
	values, present := header[http.CanonicalHeaderKey("Forwarded")]
	if !present {
		return nil, false, false
	}
	if len(values) == 0 {
		return nil, true, false
	}
	for _, value := range values {
		elements, ok := splitOutsideQuotes(value, ',')
		if !ok || len(elements) == 0 {
			return nil, true, false
		}
		for _, element := range elements {
			params, ok := splitOutsideQuotes(element, ';')
			if !ok {
				return nil, true, false
			}
			var forValue string
			forSeen := false
			for _, param := range params {
				name, value, ok := strings.Cut(strings.TrimSpace(param), "=")
				if !ok || strings.TrimSpace(name) == "" {
					return nil, true, false
				}
				if strings.EqualFold(strings.TrimSpace(name), "for") {
					if forSeen {
						return nil, true, false
					}
					forSeen = true
					forValue, ok = unquoteForwardedValue(strings.TrimSpace(value))
					if !ok {
						return nil, true, false
					}
				}
			}
			if !forSeen {
				return nil, true, false
			}
			ip, ok := parseForwardedAddress(forValue)
			if !ok {
				return nil, true, false
			}
			chain = append(chain, ip)
		}
	}
	return chain, true, len(chain) > 0
}

func xForwardedForChain(header http.Header) (chain []string, present, valid bool) {
	values, present := header[http.CanonicalHeaderKey("X-Forwarded-For")]
	if !present {
		return nil, false, false
	}
	if len(values) == 0 {
		return nil, true, false
	}
	for _, value := range values {
		parts := strings.Split(value, ",")
		for _, part := range parts {
			addr, err := netip.ParseAddr(strings.TrimSpace(part))
			if err != nil || addr.Zone() != "" {
				return nil, true, false
			}
			chain = append(chain, addr.Unmap().String())
		}
	}
	return chain, true, len(chain) > 0
}

func parseForwardedAddress(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "unknown") || strings.HasPrefix(value, "_") {
		return "", false
	}
	// RFC 7239 brackets IPv6 literals when a node-port is present.
	if strings.HasPrefix(value, "[") {
		closeBracket := strings.IndexByte(value, ']')
		if closeBracket < 0 {
			return "", false
		}
		host := value[1:closeBracket]
		rest := value[closeBracket+1:]
		if rest != "" {
			if !strings.HasPrefix(rest, ":") || !validPort(rest[1:]) {
				return "", false
			}
		}
		addr, err := netip.ParseAddr(host)
		if err != nil || !addr.Is6() || addr.Zone() != "" {
			return "", false
		}
		return addr.String(), true
	}
	if strings.Contains(value, ":") {
		host, port, err := net.SplitHostPort(value)
		if err != nil || !validPort(port) {
			return "", false
		}
		addr, err := netip.ParseAddr(host)
		if err != nil || !addr.Is4() {
			return "", false
		}
		return addr.String(), true
	}
	addr, err := netip.ParseAddr(value)
	if err != nil || addr.Zone() != "" {
		return "", false
	}
	return addr.Unmap().String(), true
}

func validPort(port string) bool {
	if port == "" {
		return false
	}
	value, err := strconv.ParseUint(port, 10, 16)
	return err == nil && value > 0
}

func unquoteForwardedValue(value string) (string, bool) {
	if value == "" {
		return "", false
	}
	if value[0] != '"' {
		if strings.ContainsAny(value, " \t\"\\") {
			return "", false
		}
		return value, true
	}
	if len(value) < 2 || value[len(value)-1] != '"' {
		return "", false
	}
	// Escapes are unnecessary for IP literals and complicate safe parsing.
	inner := value[1 : len(value)-1]
	if strings.ContainsAny(inner, "\"\\") {
		return "", false
	}
	return inner, true
}

func splitOutsideQuotes(value string, delimiter byte) ([]string, bool) {
	var parts []string
	start := 0
	quoted := false
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '"':
			if i > 0 && value[i-1] == '\\' {
				return nil, false
			}
			quoted = !quoted
		case delimiter:
			if !quoted {
				part := strings.TrimSpace(value[start:i])
				if part == "" {
					return nil, false
				}
				parts = append(parts, part)
				start = i + 1
			}
		}
	}
	if quoted {
		return nil, false
	}
	part := strings.TrimSpace(value[start:])
	if part == "" {
		return nil, false
	}
	return append(parts, part), true
}
