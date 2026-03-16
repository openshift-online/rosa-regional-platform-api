package hyperfleet

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/openshift/rosa-regional-platform-api/pkg/config"
)

// Client provides access to the Hyperfleet API as a passthrough
type Client struct {
	baseURL    string
	httpClient *http.Client
	logger     *slog.Logger
}

// NewClient creates a new Hyperfleet client
func NewClient(cfg config.HyperfleetConfig, logger *slog.Logger) *Client {
	return &Client{
		baseURL: cfg.BaseURL,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		logger: logger,
	}
}

// ProxyRequest proxies an HTTP request to the Hyperfleet API
func (c *Client) ProxyRequest(ctx context.Context, method, path string, body io.Reader, queryParams url.Values) (*http.Response, error) {
	// Build the full URL
	fullURL := c.baseURL + path
	if len(queryParams) > 0 {
		fullURL = fullURL + "?" + queryParams.Encode()
	}

	c.logger.Debug("proxying request to hyperfleet", "method", method, "path", path, "url", fullURL)

	// Create the request
	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")

	// Execute the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	c.logger.Debug("received response from hyperfleet", "status", resp.StatusCode)

	return resp, nil
}
