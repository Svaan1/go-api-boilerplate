// Package config loads and validates runtime settings at process startup.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config contains immutable validated process settings.
type Config struct {
	Env           string        `mapstructure:"env"`
	HTTP          HTTP          `mapstructure:"http"`
	Database      Database      `mapstructure:"database"`
	Auth          Auth          `mapstructure:"auth"`
	CORS          CORS          `mapstructure:"cors"`
	RateLimit     RateLimit     `mapstructure:"rate_limit"`
	Observability Observability `mapstructure:"observability"`
	Shutdown      Shutdown      `mapstructure:"shutdown"`
}

// HTTP limits exposed network resources.
type HTTP struct {
	Address           string        `mapstructure:"address"`
	ReadHeaderTimeout time.Duration `mapstructure:"read_header_timeout"`
	ReadTimeout       time.Duration `mapstructure:"read_timeout"`
	WriteTimeout      time.Duration `mapstructure:"write_timeout"`
	IdleTimeout       time.Duration `mapstructure:"idle_timeout"`
	MaxHeaderBytes    int           `mapstructure:"max_header_bytes"`
	MaxBodyBytes      int64         `mapstructure:"max_body_bytes"`
	TrustedProxies    []string      `mapstructure:"trusted_proxies"`
	HSTS              bool          `mapstructure:"hsts"`
}

// Database configures bounded PostgreSQL connections.
type Database struct {
	URL               string        `mapstructure:"url"`
	MaxConnections    int32         `mapstructure:"max_connections"`
	MinConnections    int32         `mapstructure:"min_connections"`
	MaxLifetime       time.Duration `mapstructure:"max_lifetime"`
	MaxIdleTime       time.Duration `mapstructure:"max_idle_time"`
	HealthCheckPeriod time.Duration `mapstructure:"health_check_period"`
	PingTimeout       time.Duration `mapstructure:"ping_timeout"`
}

// Auth configures signature and registered-claim validation.
type Auth struct {
	HMACSecret string `mapstructure:"hmac_secret"`
	Issuer     string `mapstructure:"issuer"`
	Audience   string `mapstructure:"audience"`
	Algorithm  string `mapstructure:"algorithm"`
}

// CORS limits cross-origin browser requests.
type CORS struct {
	Origins     []string `mapstructure:"origins"`
	Methods     []string `mapstructure:"methods"`
	Headers     []string `mapstructure:"headers"`
	Credentials bool     `mapstructure:"credentials"`
}

// RateLimit bounds per-client requests and memory.
type RateLimit struct {
	RequestsPerSecond float64       `mapstructure:"requests_per_second"`
	Burst             int           `mapstructure:"burst"`
	MaxClients        int           `mapstructure:"max_clients"`
	IdleTTL           time.Duration `mapstructure:"idle_ttl"`
	CleanupInterval   time.Duration `mapstructure:"cleanup_interval"`
}

// Observability configures logging and Prometheus exposition.
type Observability struct {
	Debug       bool   `mapstructure:"debug"`
	Metrics     bool   `mapstructure:"metrics"`
	MetricsPath string `mapstructure:"metrics_path"`
}

// Shutdown sets bounded stop and dependency probes.
type Shutdown struct {
	Timeout          time.Duration `mapstructure:"timeout"`
	ReadinessTimeout time.Duration `mapstructure:"readiness_timeout"`
}

// Load layers defaults, stage file, and APP_ environment variables, then validates.
func Load() (Config, error) {
	v := viper.New()
	v.SetEnvPrefix("APP")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	defaults := map[string]any{
		"env": "development", "http.address": "127.0.0.1:8080", "http.read_header_timeout": "5s",
		"http.read_timeout": "15s", "http.write_timeout": "30s",
		"http.idle_timeout": "60s", "http.max_header_bytes": 1048576,
		"http.max_body_bytes": 1048576, "http.trusted_proxies": []string{},
		"http.hsts": false, "database.url": "", "database.max_connections": 10,
		"database.min_connections": 0, "database.max_lifetime": "30m",
		"database.max_idle_time": "5m", "database.health_check_period": "1m",
		"database.ping_timeout": "5s", "auth.hmac_secret": "",
		"auth.issuer": "go-api-boilerplate", "auth.audience": "go-api",
		"auth.algorithm": "HS256", "cors.origins": []string{},
		"cors.methods":     []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
		"cors.headers":     []string{"Authorization", "Content-Type", "X-Request-ID"},
		"cors.credentials": false, "rate_limit.requests_per_second": 10.0,
		"rate_limit.burst": 20, "rate_limit.max_clients": 10000,
		"rate_limit.idle_ttl": "10m", "rate_limit.cleanup_interval": "1m",
		"observability.debug": false, "observability.metrics": true,
		"observability.metrics_path": "/metrics", "shutdown.timeout": "10s",
		"shutdown.readiness_timeout": "2s",
	}
	for key, value := range defaults {
		v.SetDefault(key, value)
	}
	stage := v.GetString("env")
	if !slices.Contains([]string{"development", "test", "staging", "production"}, stage) {
		return Config{}, fmt.Errorf("invalid app env %q", stage)
	}
	v.SetConfigFile("config/" + stage + ".yaml")
	if err := v.ReadInConfig(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("reading stage configuration: %w", err)
	}
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("decoding configuration: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate rejects unsafe or unusable settings before resources start.
func (c Config) Validate() error {
	if !slices.Contains([]string{"development", "test", "staging", "production"}, c.Env) {
		return errors.New("invalid app env")
	}
	host, port, err := net.SplitHostPort(c.HTTP.Address)
	if err != nil || port == "" || host == "" {
		return errors.New("invalid http address")
	}
	if parsed, err := net.LookupPort("tcp", port); err != nil || parsed < 1 {
		return errors.New("invalid http port")
	}
	h := c.HTTP
	if h.ReadHeaderTimeout <= 0 || h.ReadTimeout <= 0 || h.WriteTimeout <= 0 || h.IdleTimeout <= 0 ||
		h.MaxHeaderBytes <= 0 || h.MaxBodyBytes <= 0 {
		return errors.New("http limits must be positive")
	}
	for _, cidr := range h.TrustedProxies {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return errors.New("invalid trusted proxy cidr")
		}
	}
	d := c.Database
	if d.URL == "" {
		return errors.New("APP_DATABASE_URL is required")
	}
	if parsed, err := url.Parse(d.URL); err != nil || parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return errors.New("invalid database url")
	}
	if d.MaxConnections < 1 || d.MinConnections < 0 || d.MinConnections > d.MaxConnections ||
		d.MaxLifetime <= 0 || d.MaxIdleTime <= 0 || d.HealthCheckPeriod <= 0 || d.PingTimeout <= 0 {
		return errors.New("invalid database pool settings")
	}
	if len(c.Auth.HMACSecret) < 32 {
		return errors.New("APP_AUTH_HMAC_SECRET must contain at least 32 bytes")
	}
	if c.Auth.Algorithm != "HS256" || c.Auth.Issuer == "" || c.Auth.Audience == "" {
		return errors.New("invalid auth claims or algorithm")
	}
	for _, origin := range c.CORS.Origins {
		if origin == "*" {
			if c.CORS.Credentials {
				return errors.New("wildcard cors origin cannot allow credentials")
			}
			continue
		}
		parsed, err := url.Parse(origin)
		if err != nil || !slices.Contains([]string{"http", "https"}, parsed.Scheme) ||
			parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.User != nil {
			return errors.New("invalid cors origin")
		}
	}
	for _, method := range c.CORS.Methods {
		if !slices.Contains([]string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD"}, method) {
			return errors.New("invalid cors method")
		}
	}
	for _, header := range c.CORS.Headers {
		if header == "" || strings.ContainsAny(header, " \t\r\n:") {
			return errors.New("invalid cors header")
		}
	}
	r := c.RateLimit
	if r.RequestsPerSecond <= 0 || r.Burst <= 0 || r.MaxClients <= 0 || r.IdleTTL <= 0 || r.CleanupInterval <= 0 {
		return errors.New("invalid rate limit settings")
	}
	path := c.Observability.MetricsPath
	if !strings.HasPrefix(path, "/") || path == "/" ||
		strings.ContainsAny(path, "?#") || path == "/healthz" || path == "/readyz" ||
		path == "/v1" || strings.HasPrefix(path, "/v1/") {
		return errors.New("invalid metrics path")
	}
	if c.Shutdown.Timeout <= 0 || c.Shutdown.ReadinessTimeout <= 0 {
		return errors.New("shutdown timeouts must be positive")
	}
	if c.Env == "production" && c.Observability.Debug {
		return errors.New("debug logging is forbidden in production")
	}
	return nil
}

// Logger uses source-aware text and optional debug in development; JSON info elsewhere.
func (c Config) Logger() *slog.Logger {
	options := &slog.HandlerOptions{Level: slog.LevelInfo}
	if c.Env == "development" {
		if c.Observability.Debug {
			options.Level = slog.LevelDebug
		}
		options.AddSource = true
		return slog.New(slog.NewTextHandler(os.Stderr, options))
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, options))
}
