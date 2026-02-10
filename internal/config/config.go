package config

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	App      AppConfig      `mapstructure:"app"`
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Auth     AuthConfig     `mapstructure:"auth"`
	Runtime  RuntimeConfig  `mapstructure:"runtime"`
	Edge     EdgeConfig     `mapstructure:"edge"`
	Security SecurityConfig `mapstructure:"security"`
	Metrics  MetricsConfig  `mapstructure:"metrics"`
	Log      LogConfig      `mapstructure:"log"`
	Sentry   SentryConfig   `mapstructure:"sentry"`
}

type AppConfig struct {
	Name    string `mapstructure:"name"`
	Version string `mapstructure:"version"`
	Env     string `mapstructure:"env"`
}

type ServerConfig struct {
	Host         string        `mapstructure:"host"`
	Port         int           `mapstructure:"port"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	IdleTimeout  time.Duration `mapstructure:"idle_timeout"`
	BodyLimit    int           `mapstructure:"body_limit"`
	CORSOrigins  []string      `mapstructure:"cors_origins"` // empty = ["*"] in dev, blocked in prod
}

type DatabaseConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	User            string        `mapstructure:"user"`
	Password        string        `mapstructure:"password"`
	Name            string        `mapstructure:"name"`
	SSLMode         string        `mapstructure:"sslmode"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	AutoMigrate     bool          `mapstructure:"auto_migrate"`
}

// DSN returns a properly escaped PostgreSQL connection string.
func (d DatabaseConfig) DSN() string {
	// Use url.QueryEscape for values that may contain special characters
	return "host=" + d.Host +
		" port=" + strconv.Itoa(d.Port) +
		" user=" + d.User +
		" password='" + strings.ReplaceAll(d.Password, "'", "\\'") + "'" +
		" dbname=" + d.Name +
		" sslmode=" + d.SSLMode
}

// URI returns a PostgreSQL connection URI (alternative format).
func (d DatabaseConfig) URI() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		url.QueryEscape(d.User),
		url.QueryEscape(d.Password),
		d.Host, d.Port, d.Name, d.SSLMode)
}

type AuthConfig struct {
	JWTSecret    string        `mapstructure:"jwt_secret"`
	JWTExpiry    time.Duration `mapstructure:"jwt_expiry"`
	APIKeyHeader string        `mapstructure:"api_key_header"`
}

type RuntimeConfig struct {
	MaxExecutionTime     time.Duration `mapstructure:"max_execution_time"`
	MaxMemoryMB          int           `mapstructure:"max_memory_mb"`
	MaxConcurrentWorkers int           `mapstructure:"max_concurrent_workers"`
	SandboxEnabled       bool          `mapstructure:"sandbox_enabled"`
	EncryptionKey        string        `mapstructure:"encryption_key"`
}

type EdgeConfig struct {
	Enabled           bool          `mapstructure:"enabled"`
	NodeID            string        `mapstructure:"node_id"`
	Region            string        `mapstructure:"region"`
	Role              string        `mapstructure:"role"`
	ControlURL        string        `mapstructure:"control_url"`
	InternalSecret    string        `mapstructure:"internal_secret"` // shared secret for inter-node auth
	SyncInterval      time.Duration `mapstructure:"sync_interval"`
	HeartbeatInterval time.Duration `mapstructure:"heartbeat_interval"`
	FailoverTimeout   time.Duration `mapstructure:"failover_timeout"`
}

type SecurityConfig struct {
	MaxCodeSizeBytes   int      `mapstructure:"max_code_size_bytes"`
	MaxOutputBytes     int      `mapstructure:"max_output_bytes"`
	BlockedImports     []string `mapstructure:"blocked_imports"`
	BlockedJSGlobals   []string `mapstructure:"blocked_js_globals"`
	NetworkDisabled    bool     `mapstructure:"network_disabled"`
	FSDisabled         bool     `mapstructure:"fs_disabled"`
	CodeSigningKey     string   `mapstructure:"code_signing_key"`
	RateLimitPerWorker int      `mapstructure:"rate_limit_per_worker"`
	AuditLog           bool     `mapstructure:"audit_log"`
}

type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Path    string `mapstructure:"path"`
	Port    int    `mapstructure:"port"`
}

type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

type SentryConfig struct {
	Enabled          bool    `mapstructure:"enabled"`
	DSN              string  `mapstructure:"dsn"`
	Environment      string  `mapstructure:"environment"`
	TracesSampleRate float64 `mapstructure:"traces_sample_rate"`
	Debug            bool    `mapstructure:"debug"`
}

func Load(path string) (*Config, error) {
	viper.SetConfigFile(path)
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	cfg := &Config{}
	if err := viper.Unmarshal(cfg); err != nil {
		return nil, err
	}

	// Reject default secrets in non-development environments
	if cfg.App.Env != "" && cfg.App.Env != "development" {
		if cfg.Auth.JWTSecret == "change-me-in-production-use-env-var" {
			return nil, fmt.Errorf("jwt_secret must be changed in %s environment", cfg.App.Env)
		}
		if strings.Contains(cfg.Runtime.EncryptionKey, "change-me") {
			return nil, fmt.Errorf("encryption_key must be changed in %s environment", cfg.App.Env)
		}
	}

	// Defaults
	if cfg.Edge.NodeID == "" {
		cfg.Edge.NodeID = "node-1"
	}
	if cfg.Edge.Region == "" {
		cfg.Edge.Region = "default"
	}
	if cfg.Edge.Role == "" {
		cfg.Edge.Role = "control"
	}
	if cfg.Edge.SyncInterval == 0 {
		cfg.Edge.SyncInterval = 30 * time.Second
	}
	if cfg.Edge.HeartbeatInterval == 0 {
		cfg.Edge.HeartbeatInterval = 10 * time.Second
	}
	if cfg.Edge.FailoverTimeout == 0 {
		cfg.Edge.FailoverTimeout = 30 * time.Second
	}
	if cfg.Security.MaxCodeSizeBytes == 0 {
		cfg.Security.MaxCodeSizeBytes = 1 << 20
	}
	if cfg.Security.MaxOutputBytes == 0 {
		cfg.Security.MaxOutputBytes = 5 << 20
	}
	if cfg.Metrics.Path == "" {
		cfg.Metrics.Path = "/metrics"
	}
	if len(cfg.Server.CORSOrigins) == 0 {
		cfg.Server.CORSOrigins = []string{"*"}
	}

	return cfg, nil
}
