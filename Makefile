.PHONY: build test lint clean image image-push run generate help

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
	@echo "  test           - Run tests"
	@echo "  test-coverage  - Run tests with coverage"
	@echo "  lint           - Run linter"
	@echo "  clean          - Clean build artifacts"
	@echo "  image          - Build Docker image"
	@echo "  image-push     - Push Docker image"
	@echo "  run            - Run locally"
	@echo "  deps           - Download dependencies"
	@echo "  generate       - Generate OpenAPI code"
	@echo "  verify         - Verify go.mod is tidy"
	@echo "  all            - Run all checks (deps, lint, test, build)"

# Build the binary
build:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(BINARY_NAME) ./cmd/$(BINARY_NAME)

# Run tests
test:
	go test -v -race ./...

# Run tests with coverage
test-coverage:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

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

# Verify go.mod is tidy
verify:
	go mod tidy
	git diff --exit-code go.mod go.sum

# All checks
all: deps lint test build
