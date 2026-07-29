# Stage 1: Build binary
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install git and ca-certificates
RUN apk add --no-cache git ca-certificates

# Copy dependencies manifest first for efficient caching
COPY go.mod go.sum* ./
RUN go mod download

# Copy application source
COPY . .

# Build statically linked binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o collector ./cmd/collector

# Stage 2: Minimal runtime image
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/collector /app/collector

# Create non-root user
RUN addgroup -S finops && adduser -S finops -G finops
USER finops

EXPOSE 8080

ENTRYPOINT ["/app/collector"]
