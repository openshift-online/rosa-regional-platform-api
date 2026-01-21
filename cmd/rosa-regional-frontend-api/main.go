package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/openshift/rosa-regional-frontend-api/pkg/config"
	"github.com/openshift/rosa-regional-frontend-api/pkg/server"
)

var (
	// Config flags
	logLevel         string
	logFormat        string
	maestroURL       string
	dynamoDBEndpoint string
	dynamoDBRegion   string
	dynamoDBTable    string
	apiPort          int
	healthPort       int
	metricsPort      int
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "rosa-regional-frontend-api",
	Short: "ROSA Regional Frontend API",
	Long:  "Regional frontend API for ROSA (Red Hat OpenShift Service on AWS)",
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the API server",
	RunE:  runServe,
}

func init() {
	serveCmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	serveCmd.Flags().StringVar(&logFormat, "log-format", "json", "Log format (json, text)")
	serveCmd.Flags().StringVar(&maestroURL, "maestro-url", "http://maestro:8000", "Maestro service base URL")
	serveCmd.Flags().StringVar(&dynamoDBEndpoint, "dynamodb-endpoint", "", "DynamoDB endpoint (for local development)")
	serveCmd.Flags().StringVar(&dynamoDBRegion, "dynamodb-region", "us-east-1", "DynamoDB region")
	serveCmd.Flags().StringVar(&dynamoDBTable, "dynamodb-table", "rosa-customer-accounts", "DynamoDB table name")
	serveCmd.Flags().IntVar(&apiPort, "api-port", 8000, "API server port")
	serveCmd.Flags().IntVar(&healthPort, "health-port", 8080, "Health check server port")
	serveCmd.Flags().IntVar(&metricsPort, "metrics-port", 9090, "Metrics server port")

	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	// Create logger
	logger := createLogger(logLevel, logFormat)

	logger.Info("starting rosa-regional-frontend-api",
		"log_level", logLevel,
		"log_format", logFormat,
	)

	// Create config
	cfg := config.NewConfig()
	cfg.Logging.Level = logLevel
	cfg.Logging.Format = logFormat
	cfg.Maestro.BaseURL = maestroURL
	cfg.DynamoDB.Endpoint = dynamoDBEndpoint
	cfg.DynamoDB.Region = dynamoDBRegion
	cfg.DynamoDB.TableName = dynamoDBTable
	cfg.Server.APIPort = apiPort
	cfg.Server.HealthPort = healthPort
	cfg.Server.MetricsPort = metricsPort

	// Create server
	srv, err := server.New(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	// Setup signal handling
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Run server
	logger.Info("server configuration",
		"api_port", cfg.Server.APIPort,
		"health_port", cfg.Server.HealthPort,
		"metrics_port", cfg.Server.MetricsPort,
		"maestro_url", cfg.Maestro.BaseURL,
		"dynamodb_region", cfg.DynamoDB.Region,
		"dynamodb_table", cfg.DynamoDB.TableName,
	)

	if err := srv.Run(ctx); err != nil {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

func createLogger(level, format string) *slog.Logger {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	var handler slog.Handler
	if format == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}
