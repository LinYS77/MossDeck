// Package config loads application configuration from environment variables.
//
// Configuration is environment-driven (12-factor style) so the same binary
// can run in development and production without code changes. Empty values
// are treated as unset and fall back to defaults, which keeps hand-edited
// .env files forgiving. Unknown keys are ignored; invalid values for known
// keys surface as an explicit error at startup instead of a confusing
// runtime failure.
package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all configuration resolved at startup.
type Config struct {
	// App is the deployment flavour: "development" or "production".
	App string `json:"app"`
	// BaseURL is the canonical public origin, e.g. "https://home.example.com".
	// It has no trailing slash and is reserved for URL building in later tasks.
	BaseURL string `json:"baseUrl"`

	HTTPAddr     string        `json:"httpAddr"`
	ReadTimeout  time.Duration `json:"readTimeout"`
	WriteTimeout time.Duration `json:"writeTimeout"`

	// LogLevel is one of debug, info, warn, error.
	LogLevel string `json:"logLevel"`
	// LogFormat is "json" or "text".
	LogFormat string `json:"logFormat"`

	// DatabasePath is the filesystem path to the SQLite database file.
	DatabasePath string `json:"databasePath"`

	// StaticDir is the path to the built frontend directory (index.html + assets).
	// When non-empty, the server serves the SPA for all non-API GET requests.
	// Empty (default) = API-only mode (use with Vite dev proxy).
	StaticDir string `json:"staticDir"`

	// SessionSecret is reserved for future signed/encrypted session payloads.
	// Today's sessions are opaque random tokens stored by hash, so this secret
	// is not yet consumed; it remains a startup guard so production never runs
	// without a strong value once it is needed.
	SessionSecret string `json:"-"`

	// TrustedProxyCIDRs defines the reverse-proxy networks whose forwarded
	// headers (X-Forwarded-For / X-Real-IP) the client-IP resolver trusts.
	// Empty (the default) means NO proxy is trusted: client IPs are always the
	// TCP peer, so forged headers cannot rotate apparent IPs to bypass per-IP
	// rate limiting. For a public deployment behind a reverse proxy, set this
	// to the proxy's CIDR (e.g. 10.0.0.0/8) and have the proxy overwrite
	// X-Forwarded-For.
	TrustedProxyCIDRs []string `json:"trustedProxyCidrs"`

	// --- Auth / session ---

	// SessionTTL is how long a login session remains valid.
	SessionTTL time.Duration `json:"sessionTtl"`
	// BcryptCost is the bcrypt cost used when hashing passwords.
	BcryptCost int `json:"bcryptCost"`

	// --- Cookie ---

	// CookieName is the session cookie name.
	CookieName string `json:"cookieName"`
	// CookieSecure sets the cookie Secure attribute. Defaults to true in
	// production (HTTPS only).
	CookieSecure bool `json:"cookieSecure"`
	// CookieSameSite is "lax", "strict", or "none".
	CookieSameSite string `json:"cookieSameSite"`

	// CSRFCookieName is the non-HttpOnly double-submit cookie name.
	CSRFCookieName string `json:"csrfCookieName"`

	// SetupEnabled controls the one-time owner password setup endpoint. It defaults
	// to enabled only in development. In production it may be enabled only when
	// protected by APP_SETUP_TOKEN.
	SetupEnabled bool `json:"setupEnabled"`
	// SetupToken protects first-run setup on a publicly reachable deployment.
	SetupToken string `json:"-"`
	// CSRFHeaderName is the request header unsafe methods must echo.
	CSRFHeaderName string `json:"csrfHeaderName"`

	// --- Login rate limiting ---

	// LoginMaxFailures is the number of failed login attempts allowed per
	// client IP within LoginWindow before the key is throttled.
	LoginMaxFailures int `json:"loginMaxFailures"`
	// LoginWindow is the sliding failure-count window.
	LoginWindow time.Duration `json:"loginWindow"`
}

// Load reads configuration from the environment, applying defaults and
// validating values. It returns an error describing the first problem found.
func Load() (Config, error) {
	app := getenv("APP_ENV", "development")
	c := Config{
		App:               app,
		BaseURL:           strings.TrimRight(getenv("APP_BASE_URL", "http://localhost:8080"), "/"),
		HTTPAddr:          getenv("APP_HTTP_ADDR", ":8080"),
		ReadTimeout:       getduration("APP_READ_TIMEOUT", 15*time.Second),
		WriteTimeout:      getduration("APP_WRITE_TIMEOUT", 15*time.Second),
		LogLevel:          strings.ToLower(getenv("APP_LOG_LEVEL", "info")),
		LogFormat:         strings.ToLower(getenv("APP_LOG_FORMAT", defaultLogFormat(app))),
		DatabasePath:      getenv("APP_DATABASE_PATH", "./data/homepage.db"),
		StaticDir:         getenv("APP_STATIC_DIR", ""),
		SessionSecret:     getenv("APP_SESSION_SECRET", ""),
		TrustedProxyCIDRs: getlist("APP_TRUSTED_PROXY_CIDRS"),

		SessionTTL:       getduration("APP_SESSION_TTL", 30*24*time.Hour),
		BcryptCost:       getenvint("APP_BCRYPT_COST", 12),
		CookieName:       getenv("APP_COOKIE_NAME", "homepage_session"),
		CookieSecure:     getenvbool("APP_COOKIE_SECURE", app == "production"),
		CookieSameSite:   strings.ToLower(getenv("APP_COOKIE_SAMESITE", "lax")),
		CSRFCookieName:   strings.TrimSpace(getenv("APP_CSRF_COOKIE_NAME", "homepage_csrf")),
		SetupEnabled:     getenvbool("APP_SETUP_ENABLED", app != "production"),
		SetupToken:       strings.TrimSpace(getenv("APP_SETUP_TOKEN", "")),
		CSRFHeaderName:   strings.TrimSpace(getenv("APP_CSRF_HEADER_NAME", "X-CSRF-Token")),
		LoginMaxFailures: getenvint("APP_LOGIN_MAX_FAILURES", 5),
		LoginWindow:      getduration("APP_LOGIN_WINDOW", 15*time.Minute),
	}

	if err := validate(c); err != nil {
		return Config{}, err
	}
	return c, nil
}

// IsProduction reports whether the process runs in production mode.
func (c Config) IsProduction() bool { return c.App == "production" }

func validate(c Config) error {
	if c.App != "development" && c.App != "production" {
		return fmt.Errorf("invalid APP_ENV %q: want development or production", c.App)
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("invalid APP_LOG_LEVEL %q: want debug|info|warn|error", c.LogLevel)
	}
	if c.LogFormat != "json" && c.LogFormat != "text" {
		return fmt.Errorf("invalid APP_LOG_FORMAT %q: want json or text", c.LogFormat)
	}
	if c.HTTPAddr == "" {
		return fmt.Errorf("APP_HTTP_ADDR must not be empty")
	}
	if c.DatabasePath == "" {
		return fmt.Errorf("APP_DATABASE_PATH must not be empty")
	}
	if c.SessionTTL <= 0 {
		return fmt.Errorf("APP_SESSION_TTL must be a positive duration")
	}
	if c.BcryptCost < bcryptMinCost || c.BcryptCost > bcryptMaxCost {
		return fmt.Errorf("APP_BCRYPT_COST must be between %d and %d", bcryptMinCost, bcryptMaxCost)
	}
	if c.CookieName == "" {
		return fmt.Errorf("APP_COOKIE_NAME must not be empty")
	}
	if strings.TrimSpace(c.CSRFCookieName) == "" {
		return fmt.Errorf("APP_CSRF_COOKIE_NAME must not be empty")
	}
	if strings.TrimSpace(c.CSRFHeaderName) == "" {
		return fmt.Errorf("APP_CSRF_HEADER_NAME must not be empty")
	}
	if c.IsProduction() && c.SetupEnabled && len(c.SetupToken) < 16 {
		return fmt.Errorf("APP_SETUP_ENABLED=true in production requires APP_SETUP_TOKEN of at least 16 bytes")
	}
	switch c.CookieSameSite {
	case "lax", "strict", "none":
	default:
		return fmt.Errorf("APP_COOKIE_SAMESITE must be lax, strict, or none")
	}
	// SameSite=None requires Secure or browsers reject the cookie.
	if c.CookieSameSite == "none" && !c.CookieSecure {
		return fmt.Errorf("APP_COOKIE_SAMESITE=none requires APP_COOKIE_SECURE=true")
	}
	if c.LoginMaxFailures < 1 {
		return fmt.Errorf("APP_LOGIN_MAX_FAILURES must be at least 1")
	}
	if c.LoginWindow <= 0 {
		return fmt.Errorf("APP_LOGIN_WINDOW must be a positive duration")
	}
	for _, cidr := range c.TrustedProxyCIDRs {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		// Accept both CIDR notation and a bare IP (validated as a single host).
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			if net.ParseIP(cidr) == nil {
				return fmt.Errorf("invalid APP_TRUSTED_PROXY_CIDRS entry %q: must be a CIDR or IP", cidr)
			}
		}
	}
	// Forward-looking guard: surface a missing/waek session secret early in
	// production even though auth does not consume it yet. This prevents a
	// silently insecure deployment once auth lands.
	if c.IsProduction() && len(c.SessionSecret) < 32 {
		return fmt.Errorf("production requires APP_SESSION_SECRET of at least 32 bytes")
	}
	if c.StaticDir != "" {
		fi, err := os.Stat(c.StaticDir)
		if err != nil {
			return fmt.Errorf("APP_STATIC_DIR %q: %w", c.StaticDir, err)
		}
		if !fi.IsDir() {
			return fmt.Errorf("APP_STATIC_DIR %q is not a directory", c.StaticDir)
		}
	}
	return nil
}

func defaultLogFormat(app string) string {
	if app == "production" {
		return "json"
	}
	return "text"
}

const (
	bcryptMinCost = 4
	bcryptMaxCost = 31
)

func getenv(key, def string) string {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	return v
}

func getenvbool(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

func getenvint(key string, def int) int {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return n
}

func getduration(key string, def time.Duration) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

// getlist splits a comma-separated env value, trimming whitespace and blanks.
func getlist(key string) []string {
	v, ok := os.LookupEnv(key)
	if !ok {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
