package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/svaan1/go-api-boilerplate/internal/config"
)

func TestClientIdentity(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		trusted    []string
		forwarded  string
		xff        string
		want       string
	}{
		{
			name:       "ignores forged XFF from untrusted peer",
			remoteAddr: "198.51.100.8:4321",
			trusted:    []string{"10.0.0.0/8"},
			xff:        "203.0.113.99",
			want:       "198.51.100.8",
		},
		{
			name:       "walks multi-hop chain to first untrusted address",
			remoteAddr: "10.0.0.3:443",
			trusted:    []string{"10.0.0.0/8"},
			xff:        "198.51.100.7, 10.0.0.1, 10.0.0.2",
			want:       "198.51.100.7",
		},
		{
			name:       "Forwarded takes precedence over XFF",
			remoteAddr: "10.0.0.3:443",
			trusted:    []string{"10.0.0.0/8"},
			forwarded:  `for=192.0.2.44, for=10.0.0.2`,
			xff:        "203.0.113.99",
			want:       "192.0.2.44",
		},
		{
			name:       "malformed XFF falls back to peer",
			remoteAddr: "10.0.0.3:443",
			trusted:    []string{"10.0.0.0/8"},
			xff:        "192.0.2.44, not-an-ip",
			want:       "10.0.0.3",
		},
		{
			name:       "malformed Forwarded falls back to peer, not XFF",
			remoteAddr: "10.0.0.3:443",
			trusted:    []string{"10.0.0.0/8"},
			forwarded:  `for=192.0.2.44, for=unknown`,
			xff:        "203.0.113.99",
			want:       "10.0.0.3",
		},
		{
			name:       "parses quoted IPv6 Forwarded node with port",
			remoteAddr: "10.0.0.3:443",
			trusted:    []string{"10.0.0.0/8"},
			forwarded:  `for="[2001:db8::1]:1234", for=10.0.0.2`,
			want:       "2001:db8::1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tt.remoteAddr
			if tt.forwarded != "" {
				r.Header.Set("Forwarded", tt.forwarded)
			}
			if tt.xff != "" {
				r.Header.Set("X-Forwarded-For", tt.xff)
			}

			var fromHandler string
			handler := ClientIdentity(config.Config{HTTP: config.HTTP{TrustedProxies: tt.trusted}})(http.HandlerFunc(func(_ http.ResponseWriter, got *http.Request) {
				fromHandler = ClientIP(got)
			}))
			handler.ServeHTTP(httptest.NewRecorder(), r)

			if fromHandler != tt.want {
				t.Fatalf("ClientIP in handler = %q, want %q", fromHandler, tt.want)
			}
			if got := ClientIP(r); got != tt.want {
				t.Fatalf("ClientIP on original request after middleware = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClientIPUsesRemotePeerWithoutMiddleware(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "[2001:db8::2]:4567"

	if got, want := ClientIP(r), "2001:db8::2"; got != want {
		t.Fatalf("ClientIP = %q, want %q", got, want)
	}
}
