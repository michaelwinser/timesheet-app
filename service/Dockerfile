# Web build stage
FROM node:20-alpine AS web-builder

WORKDIR /build

# Copy package files
COPY web/package.json web/package-lock.json* ./

# Install dependencies
RUN npm install

# Copy source code
COPY web/ ./

# Build the app
RUN npm run build

# Go build stage
FROM golang:1.24-alpine AS go-builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY service/go.mod service/go.sum ./
RUN go mod download

# Copy source code
COPY service/ ./

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/server

# Runtime stage
FROM alpine:3.20

WORKDIR /app

# Install CA certificates for HTTPS calls
RUN apk add --no-cache ca-certificates

# Copy binary from builder
COPY --from=go-builder /server /app/server

# Copy API spec for serving
COPY docs/v2/api-spec.yaml /app/api-spec.yaml

# Copy web assets from web builder
COPY --from=web-builder /build/build /app/static

EXPOSE 8080

# Configure paths
ENV API_SPEC_PATH=/app/api-spec.yaml
ENV STATIC_DIR=/app/static

CMD ["/app/server"]
