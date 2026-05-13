package config

import (
	"time"

	"github.com/openshift/rosa-regional-platform-api/pkg/authz"
)

type Config struct {
	Server          ServerConfig
	Maestro         MaestroConfig
	Hyperfleet      HyperfleetConfig
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
	// AllowedOrigins is the list of origins permitted for CORS preflight
	// requests. FedRAMP SC-08 prohibits wildcard ("*") CORS origins on
	// production API endpoints; set this to the exact frontend origin(s).
	AllowedOrigins []string
}

type MaestroConfig struct {
	BaseURL     string
	GRPCBaseURL string
	Timeout     time.Duration
}

type HyperfleetConfig struct {
	BaseURL string
	Timeout time.Duration
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
			// AllowedOrigins must be set to the actual console/UI origin before
			// deployment; an empty slice disables CORS preflight responses.
			AllowedOrigins: []string{},
		},
		Maestro: MaestroConfig{
			BaseURL:     "http://maestro:8000",
			GRPCBaseURL: "maestro-grpc.maestro-server:8090",
			Timeout:     30 * time.Second,
		},
		Hyperfleet: HyperfleetConfig{
			BaseURL: "http://hyperfleet-api.hyperfleet-system:8000",
			Timeout: 30 * time.Second,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
		Authz: authz.DefaultConfig(),
	}
}
