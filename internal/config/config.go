package config

import (
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

func (d DatabaseConfig) DSN() string {
	return "host=" + d.Host +
		" port=" + itoa(d.Port) +
		" user=" + d.User +
		" password=" + d.Password +
		" dbname=" + d.Name +
		" sslmode=" + d.SSLMode
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

// EdgeConfig controls distributed execution and edge node behavior.
type EdgeConfig struct {
	Enabled    bool          `mapstructure:"enabled"`
	NodeID     string        `mapstructure:"node_id"`     // unique identifier for this node
	Region     string        `mapstructure:"region"`      // e.g. "us-east-1", "eu-west-1"
	Role       string        `mapstructure:"role"`        // "control" or "edge"
	ControlURL string        `mapstructure:"control_url"` // URL of control plane (for edge nodes)
	SyncInterval time.Duration `mapstructure:"sync_interval"` // how often edge syncs with control
	HeartbeatInterval time.Duration `mapstructure:"heartbeat_interval"`
	FailoverTimeout   time.Duration `mapstructure:"failover_timeout"`
}

// SecurityConfig controls sandbox security hardening.
type SecurityConfig struct {
	MaxCodeSizeBytes  int           `mapstructure:"max_code_size_bytes"`
	MaxOutputBytes    int           `mapstructure:"max_output_bytes"`
	BlockedImports    []string      `mapstructure:"blocked_imports"`    // Go imports to block
	BlockedJSGlobals  []string      `mapstructure:"blocked_js_globals"` // JS globals to block
	NetworkDisabled   bool          `mapstructure:"network_disabled"`   // block all outbound network
	FSDisabled        bool          `mapstructure:"fs_disabled"`        // block filesystem access
	CodeSigningKey    string        `mapstructure:"code_signing_key"`   // HMAC key for code integrity
	RateLimitPerWorker int          `mapstructure:"rate_limit_per_worker"`
	AuditLog          bool          `mapstructure:"audit_log"`
}

// MetricsConfig controls Prometheus metrics exposure.
type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Path    string `mapstructure:"path"` // default "/metrics"
	Port    int    `mapstructure:"port"` // separate port for metrics, 0 = same as server
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
		cfg.Security.MaxCodeSizeBytes = 1 << 20 // 1MB
	}
	if cfg.Security.MaxOutputBytes == 0 {
		cfg.Security.MaxOutputBytes = 5 << 20 // 5MB
	}
	if cfg.Metrics.Path == "" {
		cfg.Metrics.Path = "/metrics"
	}

	return cfg, nil
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	s := ""
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}
