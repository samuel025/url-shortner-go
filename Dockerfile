# Stage 1: Build the binary
FROM golang:alpine AS builder

WORKDIR /app

# Install ca-certificates and git
RUN apk --no-cache add ca-certificates git

# Cache Go dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build statically-linked Go binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/bin/api ./cmd/api

# Stage 2: Minimal runtime image
FROM alpine:3.20

# Install runtime dependencies and create non-root user
RUN apk --no-cache add ca-certificates tzdata && \
    adduser -D -g '' -u 10001 appuser

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/bin/api /app/api

USER appuser

EXPOSE 8080

CMD ["/app/api"]
