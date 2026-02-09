# E2E Tests

End-to-end integration and functional tests for the ROSA Regional Platform API.

## Prerequisites

```bash
go install github.com/onsi/ginkgo/v2/ginkgo@latest
```

## Running Tests

```bash
# Run with ginkgo
cd test/e2e
ginkgo -v

# Run with go test
cd test/e2e
go test -v
```

## Environment Variables

- `E2E_BASE_URL`: Base URL of the API server (default: `http://localhost:8000`)
- `E2E_TOKEN`: Authentication token for API requests

## Note

These are integration/functional tests, separate from unit tests in `pkg/`.
