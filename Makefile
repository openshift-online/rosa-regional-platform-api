.PHONY: build test test-coverage test-e2e lint clean image image-push run generate generate-swagger help

BINARY_NAME := rosa-regional-frontend-api
IMAGE_REPO ?= quay.io/openshift/rosa-regional-frontend-api
IMAGE_TAG ?= latest
GIT_SHA := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GOOS ?= linux
GOARCH ?= amd64

# Show available make targets
help:
	@echo "Available targets:"
	@echo "  build          - Build the binary"
	@echo "  test           - Run unit tests (excludes e2e)"
	@echo "  test-coverage  - Run unit tests with coverage (excludes e2e)"
	@echo "  test-e2e       - Run e2e integration/functional tests"
	@echo "  lint           - Run linter"
	@echo "  clean          - Clean build artifacts"
	@echo "  image          - Build Docker image"
	@echo "  image-push     - Push Docker image"
	@echo "  run            - Run locally"
	@echo "  deps           - Download dependencies"
	@echo "  generate       - Generate OpenAPI code"
	@echo "  generate-swagger - Regenerate openapi/swagger-ui.html from openapi.yaml"
	@echo "  verify         - Verify go.mod is tidy"
	@echo "  all            - Run all checks (deps, lint, test, build)"

# Build the binary
build:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(BINARY_NAME) ./cmd/$(BINARY_NAME)

# Run tests (excludes e2e)
test:
	go test -v -race $(shell go list ./... | grep -v '/test/e2e')

# Run tests with coverage (excludes e2e)
test-coverage:
	go test -v -race -coverprofile=coverage.out $(shell go list ./... | grep -v '/test/e2e')
	go tool cover -html=coverage.out -o coverage.html

# Run e2e tests
# go test -v ./test/e2e
test-e2e:
	ginkgo -v ./test/e2e

# Run linter
lint:
	golangci-lint run ./...

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
		echo '  <title>ROSA Regional Frontend API - Swagger UI</title>'; \
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
all: deps lint test build
