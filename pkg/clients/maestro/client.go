package maestro

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/openshift-online/maestro/pkg/api/openapi"
	"github.com/openshift-online/maestro/pkg/client/cloudevents/grpcsource"
	"github.com/openshift/rosa-regional-platform-api/pkg/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	workv1client "open-cluster-management.io/api/client/work/clientset/versioned/typed/work/v1"
	grpcoptions "open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options/grpc"
)

const (
	consumersPath       = "/api/maestro/v1/consumers"
	resourceBundlesPath = "/api/maestro/v1/resource-bundles"

	// /api/maestro/v1/resource-bundles
)

// loggerAdapter adapts slog.Logger to OCM SDK logging.Logger interface
type loggerAdapter struct {
	logger *slog.Logger
}

func (l *loggerAdapter) DebugEnabled() bool {
	return true
}

func (l *loggerAdapter) InfoEnabled() bool {
	return true
}

func (l *loggerAdapter) WarnEnabled() bool {
	return true
}

func (l *loggerAdapter) ErrorEnabled() bool {
	return true
}

func (l *loggerAdapter) Debug(ctx context.Context, format string, args ...interface{}) {
	l.logger.DebugContext(ctx, fmt.Sprintf(format, args...))
}

func (l *loggerAdapter) Info(ctx context.Context, format string, args ...interface{}) {
	l.logger.InfoContext(ctx, fmt.Sprintf(format, args...))
}

func (l *loggerAdapter) Warn(ctx context.Context, format string, args ...interface{}) {
	l.logger.WarnContext(ctx, fmt.Sprintf(format, args...))
}

func (l *loggerAdapter) Error(ctx context.Context, format string, args ...interface{}) {
	l.logger.ErrorContext(ctx, fmt.Sprintf(format, args...))
}

func (l *loggerAdapter) Fatal(ctx context.Context, format string, args ...interface{}) {
	l.logger.ErrorContext(ctx, fmt.Sprintf(format, args...))
	os.Exit(1)
}

// Client provides access to the Maestro API
type Client struct {
	baseURL       string
	grpcBaseURL   string
	httpClient    *http.Client
	logger        *slog.Logger
	grpcOpts      *grpcoptions.GRPCOptions
	sourceID      string
	openapiClient *openapi.APIClient
	workClient    workv1client.WorkV1Interface
}

// NewClient creates a new Maestro client
func NewClient(cfg config.MaestroConfig, logger *slog.Logger) *Client {
	// Create OpenAPI client configuration
	openapiCfg := openapi.NewConfiguration()
	// Parse the base URL to extract host and scheme
	parsedURL, err := url.Parse(cfg.BaseURL)
	if err == nil {
		openapiCfg.Host = parsedURL.Host
		openapiCfg.Scheme = parsedURL.Scheme
	}
	openapiClient := openapi.NewAPIClient(openapiCfg)

	// Setup gRPC options
	grpcOpts := grpcoptions.NewGRPCOptions()

	// Parse the gRPC URL to extract just the host:port (without scheme)
	grpcURL := cfg.GRPCBaseURL
	parsedGRPC, err := url.Parse(cfg.GRPCBaseURL)
	if err != nil {
		logger.Error("failed to parse gRPC URL, using original value",
			"grpc_url", cfg.GRPCBaseURL,
			"error", err)
	} else if parsedGRPC.Host == "" {
		logger.Warn("parsed gRPC URL has empty host, using original value",
			"grpc_url", cfg.GRPCBaseURL)
	} else {
		// Successfully parsed and has a host - use the host:port portion
		grpcURL = parsedGRPC.Host
	}

	grpcOpts.Dialer = &grpcoptions.GRPCDialer{
		URL: grpcURL,
	}

	// Create the gRPC work client once during initialization
	// Wrap the logger to match OCM SDK interface
	adaptedLogger := &loggerAdapter{logger: logger}

	// Initialize the gRPC work client with background context
	workClient, err := grpcsource.NewMaestroGRPCSourceWorkClient(
		context.Background(),
		adaptedLogger,
		openapiClient,
		grpcOpts,
		"rosa-regional-platform-api", // Source ID
	)
	if err != nil {
		// Log the error but don't fail - the client can still be used for non-gRPC operations
		logger.Error("failed to create gRPC work client during initialization", "error", err)
		// workClient will be nil, and CreateManifestWork will handle this gracefully
	}

	return &Client{
		baseURL:       cfg.BaseURL,
		grpcBaseURL:   cfg.GRPCBaseURL,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		logger:        logger,
		grpcOpts:      grpcOpts,
		sourceID:      "rosa-regional-platform-api", // Default source ID
		openapiClient: openapiClient,
		workClient:    workClient,
	}
}

// CreateConsumer creates a new consumer in Maestro
func (c *Client) CreateConsumer(ctx context.Context, req *ConsumerCreateRequest) (*Consumer, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+consumersPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	c.logger.Debug("creating consumer in Maestro", "name", req.Name)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		var apiErr Error
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Reason != "" {
			return nil, &apiErr
		}
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(respBody))
	}

	var consumer Consumer
	if err := json.Unmarshal(respBody, &consumer); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	c.logger.Debug("consumer created", "id", consumer.ID, "name", consumer.Name)

	return &consumer, nil
}

// ListConsumers lists consumers from Maestro with pagination
func (c *Client) ListConsumers(ctx context.Context, page, size int) (*ConsumerList, error) {
	u, err := url.Parse(c.baseURL + consumersPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	q := u.Query()
	if page > 0 {
		q.Set("page", strconv.Itoa(page))
	}
	if size > 0 {
		q.Set("size", strconv.Itoa(size))
	}
	u.RawQuery = q.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.logger.Debug("listing consumers from Maestro", "page", page, "size", size)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var apiErr Error
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Reason != "" {
			return nil, &apiErr
		}
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(respBody))
	}

	var list ConsumerList
	if err := json.Unmarshal(respBody, &list); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	c.logger.Debug("consumers listed", "total", list.Total)

	return &list, nil
}

// GetConsumer retrieves a consumer by ID from Maestro
func (c *Client) GetConsumer(ctx context.Context, id string) (*Consumer, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+consumersPath+"/"+id, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.logger.Debug("getting consumer from Maestro", "id", id)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		var apiErr Error
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Reason != "" {
			return nil, &apiErr
		}
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(respBody))
	}

	var consumer Consumer
	if err := json.Unmarshal(respBody, &consumer); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	c.logger.Debug("consumer retrieved", "id", consumer.ID, "name", consumer.Name)

	return &consumer, nil
}

// ListResourceBundles lists resource bundles from Maestro with pagination and optional filters
func (c *Client) ListResourceBundles(ctx context.Context, page, size int, search, orderBy, fields string) (*ResourceBundleList, error) {
	u, err := url.Parse(c.baseURL + resourceBundlesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	q := u.Query()
	if page > 0 {
		q.Set("page", strconv.Itoa(page))
	}
	if size > 0 {
		q.Set("size", strconv.Itoa(size))
	}
	if search != "" {
		q.Set("search", search)
	}
	if orderBy != "" {
		q.Set("orderBy", orderBy)
	}
	if fields != "" {
		q.Set("fields", fields)
	}
	u.RawQuery = q.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.logger.Debug("listing resource bundles from Maestro", "page", page, "size", size, "search", search)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var apiErr Error
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Reason != "" {
			return nil, &apiErr
		}
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(respBody))
	}

	var list ResourceBundleList
	if err := json.Unmarshal(respBody, &list); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	c.logger.Debug("resource bundles listed", "total", list.Total)

	return &list, nil
}

// CreateManifestWork creates a ManifestWork resource in Maestro via gRPC
func (c *Client) CreateManifestWork(ctx context.Context, clusterName string, manifestWork *workv1.ManifestWork) (*workv1.ManifestWork, error) {
	c.logger.Debug("creating manifestwork via gRPC", "cluster", clusterName, "work_name", manifestWork.Name)

	// Check if workClient was initialized successfully
	if c.workClient == nil {
		return nil, fmt.Errorf("gRPC work client not initialized")
	}

	// Create the ManifestWork using the reusable client interface
	result, err := c.workClient.ManifestWorks(clusterName).Create(ctx, manifestWork, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create manifestwork: %w", err)
	}

	c.logger.Debug("manifestwork created", "cluster", clusterName, "work_name", result.Name, "uid", result.UID)

	return result, nil
}
