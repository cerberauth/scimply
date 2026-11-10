package server

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel/metric"

	"github.com/cerberauth/scimply/audit"
	"github.com/cerberauth/scimply/schema"
	"github.com/cerberauth/scimply/store"
)

type AuthFunc func(r *http.Request) (bool, error)

type config struct {
	store           store.ResourceStore
	registry        *schema.Registry
	basePath        string
	auditLogger     audit.Logger
	logger          *slog.Logger
	authFunc        AuthFunc
	spConfig        *schema.ServiceProviderConfig
	maxBulkOps      int
	defaultPageSize int
	maxPageSize     int
	opsCounter      metric.Int64Counter
	passwordHasher  PasswordHasher
}

type Option func(*config)

// PasswordHasher transforms a clear-text password into the value persisted
// by the store (e.g. a bcrypt or argon2 hash).
type PasswordHasher func(ctx context.Context, password string) (string, error)

func WithStore(s store.ResourceStore) Option {
	return func(c *config) {
		c.store = s
	}
}

func WithSchemaRegistry(reg *schema.Registry) Option {
	return func(c *config) {
		c.registry = reg
	}
}

func WithBasePath(path string) Option {
	return func(c *config) {
		path = strings.TrimRight(path, "/")
		c.basePath = path
	}
}

func WithAuditLogger(l audit.Logger) Option {
	return func(c *config) {
		c.auditLogger = l
	}
}

func WithLogger(l *slog.Logger) Option {
	return func(c *config) {
		c.logger = l
	}
}

func WithAuthFunc(fn AuthFunc) Option {
	return func(c *config) {
		c.authFunc = fn
	}
}

func WithServiceProviderConfig(spc *schema.ServiceProviderConfig) Option {
	return func(c *config) {
		c.spConfig = spc
	}
}

func WithMaxBulkOps(n int) Option {
	return func(c *config) {
		c.maxBulkOps = n
	}
}

func WithDefaultPageSize(n int) Option {
	return func(c *config) {
		c.defaultPageSize = n
	}
}

func WithMaxPageSize(n int) Option {
	return func(c *config) {
		c.maxPageSize = n
	}
}

func WithBearerTokenAuth(fn func(token string) (bool, error)) Option {
	return WithAuthFunc(func(r *http.Request) (bool, error) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			return false, nil
		}
		const prefix = "Bearer "
		if !strings.HasPrefix(authHeader, prefix) {
			return false, nil
		}
		token := strings.TrimPrefix(authHeader, prefix)
		return fn(token)
	})
}

// WithPasswordHasher hashes the "password" attribute before it reaches the
// store on create, replace and patch (RFC 7643 §4.1.1).
func WithPasswordHasher(fn PasswordHasher) Option {
	return func(c *config) {
		c.passwordHasher = fn
	}
}
