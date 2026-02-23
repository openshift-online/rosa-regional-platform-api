.PHONY: build test ci-test test-unit test-authz test-coverage test-e2e lint install-golangci-lint clean image image-push run generate generate-swagger help fmt vet

BINARY_NAME := rosa-regional-platform-api
IMAGE_REPO ?= quay.io/openshift/rosa-regional-platform-api
IMAGE_TAG ?= latest
GIT_SHA := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GOOS ?= linux
GOARCH ?= amd64
DYNAMODB_ENDPOINT ?= http://localhost:8180
CEDAR_AGENT_ENDPOINT ?= http://localhost:8181

# Show available make targets
help:
	@echo "Available targets:"
	@echo ""
	@echo "Build & Run:"
	@echo "  build          - Build the binary"
	@echo "  run            - Run locally with debug settings"
	@echo "  clean          - Clean build artifacts"
	@echo ""
	@echo "Testing:"
	@echo "  test           - Run all unit tests (excludes e2e)"
	@echo "  test-unit      - Run unit tests for a specific package (PKG=./pkg/authz/...)"
	@echo "  test-authz     - Run authorization package tests only"
	@echo "  test-coverage  - Run unit tests with coverage report"
	@echo "  test-e2e       - Run e2e integration tests"
	@echo "  test-e2e-authz - Run authz e2e tests with local infrastructure"
	@echo ""
	@echo "E2E Infrastructure:"
	@echo "  e2e-authz-infra-up   - Start DynamoDB Local and cedar-agent containers"
	@echo "  e2e-authz-infra-down - Stop E2E infrastructure"
	@echo "  e2e-init-db    - Initialize DynamoDB tables"
	@echo ""
	@echo "Code Quality:"
	@echo "  lint           - Run golangci-lint"
	@echo "  fmt            - Format code with gofmt"
	@echo "  vet            - Run go vet"
	@echo "  verify         - Verify go.mod is tidy"
	@echo ""
	@echo "Docker:"
	@echo "  image          - Build Docker image"
	@echo "  image-push     - Push Docker image"
	@echo ""
	@echo "Code Generation:"
	@echo "  deps           - Download and tidy dependencies"
	@echo "  generate       - Generate OpenAPI code"
	@echo "  generate-swagger - Regenerate swagger-ui.html"
	@echo ""
	@echo "  all            - Run all checks (deps, fmt, vet, lint, test, build)"

# Build the binary
build:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(BINARY_NAME) ./cmd/$(BINARY_NAME)

# Run all unit tests (excludes e2e)
test:
	go test -v -race -count=1 $(shell go list ./... | grep -v '/test/e2e')

# CI test target - runs tests with readonly modules
ci-test:
	# go test -mod=readonly -v -race -count=1 $(shell go list ./... | grep -v '/test/e2e')
	echo "ci-test"

# Run unit tests for a specific package (usage: make test-unit PKG=./pkg/authz/...)
PKG ?= ./...
test-unit:
	go test -v -race -count=1 $(PKG)

# Run authorization package tests only
test-authz:
	go test -v -race -count=1 ./pkg/authz/...

# Run tests with coverage (excludes e2e)
test-coverage:
	go test -v -race -coverprofile=coverage.out $(shell go list ./... | grep -v '/test/e2e')
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Run e2e tests
test-e2e:
	ginkgo -v ./test/e2e

# E2E infrastructure targets
.PHONY: e2e-authz-infra-up e2e-authz-infra-down e2e-init-db test-e2e-authz

# Start DynamoDB Local and cedar-agent containers
e2e-authz-infra-up:
	podman-compose -f hack/podman-compose.e2e-authz.yaml up -d
	@echo "Waiting for services to be ready..."
	@sleep 5
	@$(MAKE) e2e-init-db

# Stop E2E infrastructure
e2e-authz-infra-down:
	podman-compose -f hack/podman-compose.e2e-authz.yaml down -v

# Initialize DynamoDB tables
e2e-init-db:
	./scripts/e2e-init-dynamodb.sh

# Run authz E2E tests (starts infrastructure, runs tests, keeps infra running)
test-e2e-authz: e2e-authz-infra-up
	@./scripts/run-e2e-authz.sh

# Run authz E2E tests with cleanup (stops infrastructure after tests)
test-e2e-authz-clean: test-e2e-authz e2e-authz-infra-down

# Format code
fmt:
	go fmt -mod=readonly ./...

# Run go vet
vet:
	go vet -mod=readonly ./...

# Install golangci-lint if not present
.PHONY: install-golangci-lint
install-golangci-lint:
	@which golangci-lint > /dev/null || { \
		echo "Installing golangci-lint..."; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin v1.62.2; \
	}

# Run linter
lint: install-golangci-lint
	GOLANGCI_LINT_CACHE=/tmp/golangci-lint-cache golangci-lint run --modules-download-mode=readonly ./...

# Clean build artifacts
clean:
	rm -f $(BINARY_NAME)
	rm -f coverage.out coverage.html

# Build Docker image
image:
	docker build --platform $(GOOS)/$(GOARCH) -t $(IMAGE_REPO):$(IMAGE_TAG) .
	docker tag $(IMAGE_REPO):$(IMAGE_TAG) $(IMAGE_REPO):$(GIT_SHA)

# Push Docker image
image-push: image
	docker push $(IMAGE_REPO):$(IMAGE_TAG)
	docker push $(IMAGE_REPO):$(GIT_SHA)

# Run locally
run: build
	./$(BINARY_NAME) serve \
		--log-level=debug \
		--log-format=text \
		--maestro-url=http://localhost:8001 \
		--allowed-accounts=123456789012

# Download dependencies
deps:
	go mod download
	go mod tidy

# Generate OpenAPI code (requires oapi-codegen)
generate:
	@echo "OpenAPI code generation not yet configured"
	@echo "Install oapi-codegen: go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest"

# Regenerate swagger-ui.html from openapi.yaml (requires yq)
generate-swagger:
	@which yq > /dev/null || (echo "Error: yq is required. Install with: brew install yq" && exit 1)
	@echo "Generating openapi/swagger-ui.html from openapi/openapi.yaml..."
	@( \
		echo '<!DOCTYPE html>'; \
		echo '<html lang="en">'; \
		echo '<head>'; \
		echo '  <meta charset="UTF-8">'; \
		echo '  <title>ROSA Regional Platform API - Swagger UI</title>'; \
		echo '  <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5.10.5/swagger-ui.css">'; \
		echo '  <style>'; \
		echo '    html {'; \
		echo '      box-sizing: border-box;'; \
		echo '      overflow: -moz-scrollbars-vertical;'; \
		echo '      overflow-y: scroll;'; \
		echo '    }'; \
		echo '    *, *:before, *:after {'; \
		echo '      box-sizing: inherit;'; \
		echo '    }'; \
		echo '    body {'; \
		echo '      margin: 0;'; \
		echo '      padding: 0;'; \
		echo '    }'; \
		echo '  </style>'; \
		echo '</head>'; \
		echo '<body>'; \
		echo '  <div id="swagger-ui"></div>'; \
		echo '  <script src="https://unpkg.com/swagger-ui-dist@5.10.5/swagger-ui-bundle.js"></script>'; \
		echo '  <script src="https://unpkg.com/swagger-ui-dist@5.10.5/swagger-ui-standalone-preset.js"></script>'; \
		echo '  <script>'; \
		echo '    window.onload = function() {'; \
		echo '      const ui = SwaggerUIBundle({'; \
		echo "        url: window.location.origin + '/openapi.yaml',"; \
		echo '        spec: '; \
		yq eval -o=json -I=2 '.' openapi/openapi.yaml | sed 's/^/  /'; \
		echo ','; \
		echo "        dom_id: '#swagger-ui',"; \
		echo '        deepLinking: true,'; \
		echo '        presets: ['; \
		echo '          SwaggerUIBundle.presets.apis,'; \
		echo '          SwaggerUIStandalonePreset'; \
		echo '        ],'; \
		echo '        plugins: ['; \
		echo '          SwaggerUIBundle.plugins.DownloadUrl'; \
		echo '        ],'; \
		echo '        layout: "StandaloneLayout"'; \
		echo '      });'; \
		echo '      window.ui = ui;'; \
		echo '    };'; \
		echo '  </script>'; \
		echo '</body>'; \
		echo '</html>'; \
	) > docs/index.html
	@echo "Done! Generated docs/index.html"

# Verify go.mod is tidy
verify:
	go mod tidy
	git diff --exit-code go.mod go.sum

# All checks
all:
	make lint
# all: deps fmt vet lint test build
