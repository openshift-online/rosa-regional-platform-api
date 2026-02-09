package config

import (
	"time"

	"github.com/openshift/rosa-regional-platform-api/pkg/authz"
)

type Config struct {
	Server          ServerConfig
	Maestro         MaestroConfig
	Logging         LoggingConfig
	Authz           *authz.Config
	AllowedAccounts []string
}

type ServerConfig struct {
	APIBindAddress     string
	APIPort            int
	GRPCBindAddress    string
	GRPCPort           int
	HealthBindAddress  string
	HealthPort         int
	MetricsBindAddress string
	MetricsPort        int
	ShutdownTimeout    time.Duration
}

type MaestroConfig struct {
	BaseURL     string
	GRPCBaseURL string
	Timeout     time.Duration
}

type LoggingConfig struct {
	Level  string
	Format string
}

func NewConfig() *Config {
	return &Config{
		Server: ServerConfig{
			APIBindAddress:     "0.0.0.0",
			APIPort:            8000,
			GRPCBindAddress:    "0.0.0.0",
			GRPCPort:           8090,
			HealthBindAddress:  "0.0.0.0",
			HealthPort:         8080,
			MetricsBindAddress: "0.0.0.0",
			MetricsPort:        9090,
			ShutdownTimeout:    30 * time.Second,
		},
		Maestro: MaestroConfig{
			BaseURL:     "http://maestro:8000",
			GRPCBaseURL: "maestro-grpc.maestro-server:8090",
			Timeout:     30 * time.Second,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
		Authz: authz.DefaultConfig(),
	}
}
