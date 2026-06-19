package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	// Ensure a clean environment so defaults apply.
	for _, k := range []string{"APP_ENV", "APP_BASE_URL", "APP_HTTP_ADDR", "APP_LOG_LEVEL",
		"APP_LOG_FORMAT", "APP_DATABASE_PATH", "APP_SESSION_SECRET", "APP_TRUSTED_PROXY_CIDRS",
		"APP_CSRF_COOKIE_NAME", "APP_CSRF_HEADER_NAME", "APP_SETUP_ENABLED", "APP_SETUP_TOKEN"} {
		t.Setenv(k, "")
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.App != "development" {
		t.Fatalf("App = %q, want development", cfg.App)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("HTTPAddr = %q", cfg.HTTPAddr)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("LogLevel = %q", cfg.LogLevel)
	}
	if cfg.LogFormat != "text" {
		t.Fatalf("LogFormat = %q, want text in development", cfg.LogFormat)
	}
	if cfg.ReadTimeout != 15*time.Second {
		t.Fatalf("ReadTimeout = %v", cfg.ReadTimeout)
	}
	if cfg.CSRFCookieName != "homepage_csrf" {
		t.Fatalf("CSRFCookieName = %q", cfg.CSRFCookieName)
	}
	if cfg.CSRFHeaderName != "X-CSRF-Token" {
		t.Fatalf("CSRFHeaderName = %q", cfg.CSRFHeaderName)
	}
	if !cfg.SetupEnabled {
		t.Fatal("setup should be enabled by default in development")
	}
}

func TestLoadHonoursEnv(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_LOG_FORMAT", "json")
	t.Setenv("APP_HTTP_ADDR", ":9000")
	t.Setenv("APP_SESSION_SECRET", "0123456789abcdef0123456789abcdef") // 32 bytes
	t.Setenv("APP_TRUSTED_PROXY_CIDRS", "10.0.0.0/8, 172.16.0.0/12")
	t.Setenv("APP_CSRF_COOKIE_NAME", "csrf_cookie")
	t.Setenv("APP_CSRF_HEADER_NAME", "X-Test-CSRF")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.IsProduction() {
		t.Fatal("expected production")
	}
	if cfg.HTTPAddr != ":9000" {
		t.Fatalf("HTTPAddr = %q", cfg.HTTPAddr)
	}
	if cfg.LogFormat != "json" {
		t.Fatalf("LogFormat = %q", cfg.LogFormat)
	}
	if len(cfg.TrustedProxyCIDRs) != 2 {
		t.Fatalf("trusted proxies = %v", cfg.TrustedProxyCIDRs)
	}
	if cfg.SessionSecret == "" {
		t.Fatal("session secret not loaded")
	}
	if cfg.CSRFCookieName != "csrf_cookie" || cfg.CSRFHeaderName != "X-Test-CSRF" {
		t.Fatalf("csrf config not loaded: cookie=%q header=%q", cfg.CSRFCookieName, cfg.CSRFHeaderName)
	}
	if cfg.SetupEnabled {
		t.Fatal("setup should be disabled by default in production")
	}
}

func TestProductionSetupCanBeTokenProtected(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_SESSION_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("APP_SETUP_ENABLED", "true")
	t.Setenv("APP_SETUP_TOKEN", "setup-token-123456")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.SetupEnabled || cfg.SetupToken != "setup-token-123456" {
		t.Fatalf("setup token config not loaded: enabled=%v token=%q", cfg.SetupEnabled, cfg.SetupToken)
	}
}

func TestLoadValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
	}{
		{"bad env", map[string]string{"APP_ENV": "staging"}},
		{"bad log level", map[string]string{"APP_LOG_LEVEL": "trace"}},
		{"bad log format", map[string]string{"APP_LOG_FORMAT": "yaml"}},
		{"bad trusted proxy cidr", map[string]string{"APP_TRUSTED_PROXY_CIDRS": "10.0.0.0/8, not-a-cidr"}},
		{"empty csrf cookie", map[string]string{"APP_CSRF_COOKIE_NAME": "   "}},
		{"empty csrf header", map[string]string{"APP_CSRF_HEADER_NAME": "   "}},
		{"production without secret", map[string]string{"APP_ENV": "production"}},
		{"production weak secret", map[string]string{"APP_ENV": "production", "APP_SESSION_SECRET": "short"}},
		{"production setup enabled without token", map[string]string{"APP_ENV": "production", "APP_SESSION_SECRET": "0123456789abcdef0123456789abcdef", "APP_SETUP_ENABLED": "true"}},
		{"production setup enabled weak token", map[string]string{"APP_ENV": "production", "APP_SESSION_SECRET": "0123456789abcdef0123456789abcdef", "APP_SETUP_ENABLED": "true", "APP_SETUP_TOKEN": "short"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			if _, err := Load(); err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
		})
	}
}
