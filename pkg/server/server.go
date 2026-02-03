package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/openshift/rosa-regional-frontend-api/pkg/clients/maestro"
	"github.com/openshift/rosa-regional-frontend-api/pkg/config"
	apphandlers "github.com/openshift/rosa-regional-frontend-api/pkg/handlers"
	"github.com/openshift/rosa-regional-frontend-api/pkg/middleware"
)

// Server represents the API server
type Server struct {
	cfg           *config.Config
	logger        *slog.Logger
	apiServer     *http.Server
	grpcServer    *http.Server
	healthServer  *http.Server
	metricsServer *http.Server
	healthHandler *apphandlers.HealthHandler
}

// New creates a new Server instance
func New(cfg *config.Config, logger *slog.Logger) (*Server, error) {
	// Create Maestro client
	maestroClient := maestro.NewClient(cfg.Maestro, logger)

	// Create handlers
	healthHandler := apphandlers.NewHealthHandler()
	mgmtClusterHandler := apphandlers.NewManagementClusterHandler(maestroClient, logger)
	resourceBundleHandler := apphandlers.NewResourceBundleHandler(maestroClient, logger)
	workHandler := apphandlers.NewWorkHandler(maestroClient, logger)

	// Create authorization middleware
	authMiddleware := middleware.NewAuthorization(cfg.AllowedAccounts, logger)

	// Create API router
	apiRouter := mux.NewRouter()
	apiRouter.Use(middleware.Identity)

	// Management cluster routes (require allowed account)
	mgmtRouter := apiRouter.PathPrefix("/api/v0/management_clusters").Subrouter()
	mgmtRouter.Use(authMiddleware.RequireAllowedAccount)
	mgmtRouter.HandleFunc("", mgmtClusterHandler.Create).Methods(http.MethodPost)
	mgmtRouter.HandleFunc("", mgmtClusterHandler.List).Methods(http.MethodGet)
	mgmtRouter.HandleFunc("/{id}", mgmtClusterHandler.Get).Methods(http.MethodGet)

	// Resource bundle routes (require allowed account)
	rbRouter := apiRouter.PathPrefix("/api/v0/resource_bundles").Subrouter()
	rbRouter.Use(authMiddleware.RequireAllowedAccount)
	rbRouter.HandleFunc("", resourceBundleHandler.List).Methods(http.MethodGet)

	// Work routes (require allowed account)
	workRouter := apiRouter.PathPrefix("/api/v0/work").Subrouter()
	workRouter.Use(authMiddleware.RequireAllowedAccount)
	workRouter.HandleFunc("", workHandler.Create).Methods(http.MethodPost)

	// Health routes on API server (no auth required)
	apiRouter.HandleFunc("/api/v0/live", healthHandler.Liveness).Methods(http.MethodGet)
	apiRouter.HandleFunc("/api/v0/ready", healthHandler.Readiness).Methods(http.MethodGet)

	// Add CORS and logging
	apiHandler := handlers.CORS(
		handlers.AllowedOrigins([]string{"*"}),
		handlers.AllowedMethods([]string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete}),
		handlers.AllowedHeaders([]string{"Content-Type", "Authorization"}),
	)(apiRouter)

	// Create health router
	healthRouter := mux.NewRouter()
	healthRouter.HandleFunc("/healthz", healthHandler.Liveness).Methods(http.MethodGet)
	healthRouter.HandleFunc("/readyz", healthHandler.Readiness).Methods(http.MethodGet)

	// Create metrics router
	metricsRouter := mux.NewRouter()
	metricsRouter.Handle("/metrics", promhttp.Handler()).Methods(http.MethodGet)

	return &Server{
		cfg:    cfg,
		logger: logger,
		apiServer: &http.Server{
			Addr:         fmt.Sprintf("%s:%d", cfg.Server.APIBindAddress, cfg.Server.APIPort),
			Handler:      apiHandler,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
		},
		healthServer: &http.Server{
			Addr:         fmt.Sprintf("%s:%d", cfg.Server.HealthBindAddress, cfg.Server.HealthPort),
			Handler:      healthRouter,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		},
		metricsServer: &http.Server{
			Addr:         fmt.Sprintf("%s:%d", cfg.Server.MetricsBindAddress, cfg.Server.MetricsPort),
			Handler:      metricsRouter,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		},
		healthHandler: healthHandler,
	}, nil
}

// Run starts all servers and blocks until context is cancelled
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 3)

	// Start health server
	go func() {
		s.logger.Info("starting health server", "addr", s.healthServer.Addr)
		if err := s.healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("health server error: %w", err)
		}
	}()

	// Start metrics server
	go func() {
		s.logger.Info("starting metrics server", "addr", s.metricsServer.Addr)
		if err := s.metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("metrics server error: %w", err)
		}
	}()

	// Start API server
	go func() {
		s.logger.Info("starting API server", "addr", s.apiServer.Addr)
		if err := s.apiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("API server error: %w", err)
		}
	}()

	// Wait for context cancellation or error
	select {
	case <-ctx.Done():
		s.logger.Info("shutting down servers")
		return s.shutdown()
	case err := <-errCh:
		return err
	}
}

func (s *Server) shutdown() error {
	// Mark as not ready to stop receiving traffic
	s.healthHandler.SetReady(false)

	// Give load balancers time to detect we're not ready
	time.Sleep(5 * time.Second)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.Server.ShutdownTimeout)
	defer cancel()

	// Shutdown servers in order
	if err := s.apiServer.Shutdown(shutdownCtx); err != nil {
		s.logger.Error("failed to shutdown API server", "error", err)
	}

	if err := s.metricsServer.Shutdown(shutdownCtx); err != nil {
		s.logger.Error("failed to shutdown metrics server", "error", err)
	}

	if err := s.healthServer.Shutdown(shutdownCtx); err != nil {
		s.logger.Error("failed to shutdown health server", "error", err)
	}

	s.logger.Info("all servers stopped")
	return nil
}
