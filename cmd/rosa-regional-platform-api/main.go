package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/rosa-regional-platform-api/pkg/clients/fleetdb"
	"github.com/openshift/rosa-regional-platform-api/pkg/config"
	"github.com/openshift/rosa-regional-platform-api/pkg/server"
)

var (
	// Config flags
	logLevel           string
	logFormat          string
	fleetDBClusterName string
	allowedAccounts    string
	dynamodbRegion     string
	dynamodbPrefix     string
	oidcIssuerBaseURL  string
	apiPort            int
	healthPort         int
	metricsPort        int
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "rosa-regional-platform-api",
	Short: "ROSA Regional Platform API",
	Long:  "Regional platform API for ROSA (Red Hat OpenShift Service on AWS)",
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the API server",
	RunE:  runServe,
}

func init() {
	serveCmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	serveCmd.Flags().StringVar(&logFormat, "log-format", "json", "Log format (json, text)")
	serveCmd.Flags().StringVar(&allowedAccounts, "allowed-accounts", "", "Comma-separated list of allowed AWS account IDs")
	serveCmd.Flags().StringVar(&fleetDBClusterName, "fleet-db-cluster-name", "", "EKS cluster name for fleet-db")
	serveCmd.Flags().StringVar(&dynamodbRegion, "dynamodb-region", "", "AWS region for DynamoDB (defaults to auto-detected region)")
	serveCmd.Flags().StringVar(&dynamodbPrefix, "dynamodb-prefix", "rosa", "Prefix for DynamoDB table names (default: rosa)")
	serveCmd.Flags().StringVar(&oidcIssuerBaseURL, "oidc-issuer-base-url", "", "Base URL for OIDC issuer (e.g. https://<cloudfront-domain>)")
	serveCmd.Flags().IntVar(&apiPort, "api-port", 8000, "API server port")
	serveCmd.Flags().IntVar(&healthPort, "health-port", 8080, "Health check server port")
	serveCmd.Flags().IntVar(&metricsPort, "metrics-port", 9090, "Metrics server port")

	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	// Create logger
	logger := createLogger(logLevel, logFormat)

	logger.Info("starting rosa-regional-platform-api",
		"log_level", logLevel,
		"log_format", logFormat,
	)

	// Detect AWS region from SDK default chain (IMDS, AWS_REGION env var, etc.)
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		return fmt.Errorf("failed to detect AWS region: %w", err)
	}
	if awsCfg.Region == "" {
		return fmt.Errorf("AWS region could not be detected from environment; set AWS_REGION")
	}
	logger.Info("detected AWS region", "region", awsCfg.Region)

	// Create config
	cfg := config.NewConfig()
	cfg.Logging.Level = logLevel
	cfg.Logging.Format = logFormat
	if fleetDBClusterName == "" {
		return fmt.Errorf("--fleet-db-cluster-name is required")
	}
	cfg.FleetDB.ClusterName = fleetDBClusterName
	cfg.FleetDB.AWSRegion = awsCfg.Region

	cfg.Regional.OIDCIssuerBaseURL = oidcIssuerBaseURL
	cfg.AllowedAccounts = parseAllowedAccounts(allowedAccounts)
	cfg.Server.APIPort = apiPort
	cfg.Server.HealthPort = healthPort
	cfg.Server.MetricsPort = metricsPort

	// Set DynamoDB region: --dynamodb-region if set, otherwise fall back to auto-detected region
	if dynamodbRegion != "" {
		cfg.Authz.AWSRegion = dynamodbRegion
		logger.Info("using DynamoDB region from flag", "region", dynamodbRegion)
	} else {
		cfg.Authz.AWSRegion = awsCfg.Region
	}

	// Set DynamoDB table name prefix
	if dynamodbPrefix != "" {
		cfg.Authz.AccountsTableName = dynamodbPrefix + "-authz-accounts"
		cfg.Authz.AdminsTableName = dynamodbPrefix + "-authz-admins"
		cfg.Authz.GroupsTableName = dynamodbPrefix + "-authz-groups"
		cfg.Authz.MembersTableName = dynamodbPrefix + "-authz-group-members"
		logger.Info("using DynamoDB table prefix", "prefix", dynamodbPrefix)
	}

	// Authz config from environment variables (for local development)
	if endpoint := os.Getenv("DYNAMODB_ENDPOINT"); endpoint != "" {
		cfg.Authz.DynamoDBEndpoint = endpoint
		logger.Info("using custom DynamoDB endpoint", "endpoint", endpoint)
	}
	if endpoint := os.Getenv("CEDAR_AGENT_ENDPOINT"); endpoint != "" {
		cfg.Authz.CedarAgentEndpoint = endpoint
		logger.Info("using cedar-agent for local AVP", "endpoint", endpoint)
	}
	if os.Getenv("AUTHZ_DISABLED") == "true" {
		cfg.Authz.Enabled = false
		logger.Info("authz disabled via environment variable")
	}

	// ZOA configuration from environment variables
	if os.Getenv("ZOA_ENABLED") == "true" {
		cfg.Zoa.Enabled = true
		cfg.Zoa.AWSRegion = dynamodbRegion
		if table := os.Getenv("ZOA_TABLE_NAME"); table != "" {
			cfg.Zoa.TableName = table
		} else if dynamodbPrefix != "" {
			cfg.Zoa.TableName = dynamodbPrefix + "-zoa-executions"
		}
		if bucket := os.Getenv("ZOA_BUCKET_NAME"); bucket != "" {
			cfg.Zoa.BucketName = bucket
		}
		if dir := os.Getenv("ZOA_TEMPLATES_DIR"); dir != "" {
			cfg.Zoa.TemplatesDir = dir
		} else {
			cfg.Zoa.TemplatesDir = "/etc/zoa/templates"
		}
		if dir := os.Getenv("ZOA_JOB_CONFIG_DIR"); dir != "" {
			cfg.Zoa.JobConfigDir = dir
		} else {
			cfg.Zoa.JobConfigDir = "/etc/zoa/job-config"
		}
		if auditTable := os.Getenv("ZOA_AUDIT_TABLE_NAME"); auditTable != "" {
			cfg.Zoa.AuditTableName = auditTable
		}
		logger.Info("ZOA trusted actions enabled",
			"table", cfg.Zoa.TableName,
			"bucket", cfg.Zoa.BucketName,
			"region", cfg.Zoa.AWSRegion,
			"templates_dir", cfg.Zoa.TemplatesDir,
			"job_config_dir", cfg.Zoa.JobConfigDir,
			"audit_table", cfg.Zoa.AuditTableName,
		)
	}

	// Create fleet-db client (reuses awsCfg from region detection above)
	fleetDBClient, err := fleetdb.NewClient(context.Background(), awsCfg, cfg.FleetDB.ClusterName, logger)
	if err != nil {
		return fmt.Errorf("failed to create fleet-db client: %w", err)
	}

	// Create in-cluster client for the RC (local cluster) — used by MC handler
	// to manage the hyperfleet-mc-config ConfigMap.
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	rcClient, err := ctrlclient.New(ctrl.GetConfigOrDie(), ctrlclient.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to create RC in-cluster client: %w", err)
	}

	// Create server
	srv, err := server.New(cfg, fleetDBClient, rcClient, logger)
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
		"fleet_db_cluster", cfg.FleetDB.ClusterName,
		"aws_region", cfg.FleetDB.AWSRegion,
		"allowed_accounts_count", len(cfg.AllowedAccounts),
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

func parseAllowedAccounts(accounts string) []string {
	if accounts == "" {
		return nil
	}
	var result []string
	for _, acc := range strings.Split(accounts, ",") {
		acc = strings.TrimSpace(acc)
		if acc != "" {
			result = append(result, acc)
		}
	}
	return result
}
