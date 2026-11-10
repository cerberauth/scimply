package config

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration wraps time.Duration for YAML unmarshaling from strings like "5s".
type Duration struct{ time.Duration }

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	dur, err := time.ParseDuration(value.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", value.Value, err)
	}
	d.Duration = dur
	return nil
}

// DefaultBasePath is where the SCIM endpoints are served unless configured.
const DefaultBasePath = "/scim/v2"

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Auth      AuthConfig      `yaml:"auth"`
	Store     StoreConfig     `yaml:"store"`
	Audit     AuditConfig     `yaml:"audit"`
	Log       LogConfig       `yaml:"log"`
	Telemetry TelemetryConfig `yaml:"telemetry"`
}

type TelemetryConfig struct {
	Endpoint string            `yaml:"endpoint"`
	Headers  map[string]string `yaml:"headers"`
}

type ServerConfig struct {
	Addr     string `yaml:"addr"`
	BasePath string `yaml:"base_path"`
	// BaseURL is the public absolute URL of the SCIM root used in Location
	// headers and meta.location (e.g. "https://idp.example.com/scim/v2").
	// When empty it is derived from the request Host header.
	BaseURL         string    `yaml:"base_url"`
	ReadTimeout     Duration  `yaml:"read_timeout"`
	WriteTimeout    Duration  `yaml:"write_timeout"`
	ShutdownTimeout Duration  `yaml:"shutdown_timeout"`
	DefaultPageSize int       `yaml:"default_page_size"`
	MaxPageSize     int       `yaml:"max_page_size"`
	MaxBulkOps      int       `yaml:"max_bulk_ops"`
	TLS             TLSConfig `yaml:"tls"`
}

type TLSConfig struct {
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

// AuthConfig controls request authentication.
// Type: "none" (default) | "bearer_token"
type AuthConfig struct {
	Type string `yaml:"type"`
	// Tokens lists accepted bearer tokens in plain text. Prefer TokenEnv or
	// TokenFiles to keep secrets out of the config file.
	Tokens []string `yaml:"tokens"`
	// TokenEnv lists environment variables, each holding one accepted token.
	TokenEnv []string `yaml:"token_env"`
	// TokenFiles lists files, each holding one accepted token (surrounding
	// whitespace is trimmed), e.g. mounted secrets.
	TokenFiles []string `yaml:"token_files"`
}

// ResolveTokens returns every accepted bearer token from Tokens, TokenEnv
// and TokenFiles. A referenced environment variable that is unset or empty,
// or a file that is unreadable or empty, is an error.
func (a AuthConfig) ResolveTokens() ([]string, error) {
	tokens := make([]string, 0, len(a.Tokens)+len(a.TokenEnv)+len(a.TokenFiles))
	for _, t := range a.Tokens {
		if t != "" {
			tokens = append(tokens, t)
		}
	}
	for _, name := range a.TokenEnv {
		v := strings.TrimSpace(os.Getenv(name))
		if v == "" {
			return nil, fmt.Errorf("auth token environment variable %q is not set", name)
		}
		tokens = append(tokens, v)
	}
	for _, path := range a.TokenFiles {
		data, err := os.ReadFile(path) //nolint:gosec // G304: path comes from the operator's config file.
		if err != nil {
			return nil, fmt.Errorf("read auth token file: %w", err)
		}
		v := strings.TrimSpace(string(data))
		if v == "" {
			return nil, fmt.Errorf("auth token file %q is empty", path)
		}
		tokens = append(tokens, v)
	}
	return tokens, nil
}

// StoreConfig selects the backend store.
// Type: "memory" (default) | "postgres" | "mysql" | "mongodb" | "scim"
type StoreConfig struct {
	Type     string         `yaml:"type"`
	Postgres PostgresConfig `yaml:"postgres"`
	MySQL    MySQLConfig    `yaml:"mysql"`
	MongoDB  MongoDBConfig  `yaml:"mongodb"`
	SCIM     SCIMConfig     `yaml:"scim"`
}

type PostgresConfig struct {
	DSN         string   `yaml:"dsn"`
	TablePrefix string   `yaml:"table_prefix"`
	MaxConns    int32    `yaml:"max_conns"`
	MinConns    int32    `yaml:"min_conns"`
	ConnTimeout Duration `yaml:"conn_timeout"`
	AutoMigrate bool     `yaml:"auto_migrate"`
}

type MySQLConfig struct {
	DSN         string `yaml:"dsn"`
	TablePrefix string `yaml:"table_prefix"`
	MaxConns    int    `yaml:"max_conns"`
	AutoMigrate bool   `yaml:"auto_migrate"`
}

type MongoDBConfig struct {
	URI              string   `yaml:"uri"`
	Database         string   `yaml:"database"`
	CollectionPrefix string   `yaml:"collection_prefix"`
	Timeout          Duration `yaml:"timeout"`
	AutoMigrate      bool     `yaml:"auto_migrate"`
}

// SCIMConfig configures the SCIM proxy connector.
// Version: "v2" (default) | "v1"
type SCIMConfig struct {
	BaseURL string   `yaml:"base_url"`
	Token   string   `yaml:"token"`
	Timeout Duration `yaml:"timeout"`
	Version string   `yaml:"version"`
}

// AuditConfig controls the audit event log.
// Type: "none" (default) | "json"
// Output: "stdout" (default) | "stderr" | file path
type AuditConfig struct {
	Type   string `yaml:"type"`
	Output string `yaml:"output"`
}

// LogConfig controls structured application logging.
// Level: "debug" | "info" (default) | "warn" | "error"
// Format: "json" (default) | "text"
type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// Load reads a YAML config file and returns a Config with defaults applied.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	cfg := defaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

// CustomizedCount returns how many settings in the config file at path differ
// from the defaults, counting each value once (server.tls.cert_file is one, not
// two). It reports false when the file can't be read, so a caller can leave the
// number out. Only the count leaves this function: never a key or a value.
func CustomizedCount(path string) (int, bool) {
	cfg, err := Load(path)
	if err != nil {
		return 0, false
	}
	return countDifferences(reflect.ValueOf(defaultConfig()).Elem(), reflect.ValueOf(cfg).Elem()), true
}

// countDifferences walks two values of the same type and counts the leaf values
// that differ.
func countDifferences(def, got reflect.Value) int {
	if def.Kind() == reflect.Struct {
		n := 0
		for i := 0; i < def.NumField(); i++ {
			n += countDifferences(def.Field(i), got.Field(i))
		}
		return n
	}
	if reflect.DeepEqual(def.Interface(), got.Interface()) {
		return 0
	}
	return 1
}

const defaultPrefix = "scim_"

func defaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Addr:            ":8080",
			BasePath:        DefaultBasePath,
			ReadTimeout:     Duration{5 * time.Second},
			WriteTimeout:    Duration{10 * time.Second},
			ShutdownTimeout: Duration{30 * time.Second},
			DefaultPageSize: 100,
			MaxPageSize:     1000,
			MaxBulkOps:      100,
		},
		Auth: AuthConfig{
			Type: "none",
		},
		Store: StoreConfig{
			Type: "memory",
			Postgres: PostgresConfig{
				TablePrefix: defaultPrefix,
				MaxConns:    10,
				MinConns:    2,
				ConnTimeout: Duration{30 * time.Second},
			},
			MySQL: MySQLConfig{
				TablePrefix: defaultPrefix,
				MaxConns:    10,
			},
			MongoDB: MongoDBConfig{
				Database:         "scim",
				CollectionPrefix: defaultPrefix,
				Timeout:          Duration{30 * time.Second},
			},
			SCIM: SCIMConfig{
				Timeout: Duration{30 * time.Second},
				Version: "v2",
			},
		},
		Audit: AuditConfig{
			Type:   "none",
			Output: "stdout",
		},
		Log: LogConfig{
			Level:  "info",
			Format: "json",
		},
	}
}
