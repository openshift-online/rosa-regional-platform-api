# Build stage
FROM golang:1.25-alpine AS builder

# Build arguments for OS and architecture support
ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-w -s" \
    -o rosa-regional-frontend-api \
    ./cmd/rosa-regional-frontend-api

# Runtime stage
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /app/rosa-regional-frontend-api /app/rosa-regional-frontend-api

# Expose ports
EXPOSE 8000 8080 9090

# Run as non-root user
USER nonroot:nonroot

ENTRYPOINT ["/app/rosa-regional-frontend-api"]
CMD ["serve"]
