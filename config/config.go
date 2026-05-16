package config

import (
	"fmt"
	"os"
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

type Config struct {
	Server ServerConfig `yaml:"server"`
	Auth   AuthConfig   `yaml:"auth"`
	Store  StoreConfig  `yaml:"store"`
	Audit  AuditConfig  `yaml:"audit"`
	Log    LogConfig    `yaml:"log"`
}

type ServerConfig struct {
	Addr            string    `yaml:"addr"`
	BasePath        string    `yaml:"base_path"`
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
	Type   string   `yaml:"type"`
	Tokens []string `yaml:"tokens"`
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

const defaultPrefix = "scim_"

func defaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Addr:            ":8080",
			BasePath:        "/scim/v2",
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
