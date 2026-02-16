# Build stage
FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git ca-certificates
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY config.yml config-edge.yml ./
COPY internal ./internal
COPY cmd ./cmd
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /wkr ./cmd/server

# Runtime stage — needs Go for compiling Go workers, Deno for JS/TS workers, Python for Python workers, Rust for Rust workers
FROM golang:1.25-alpine
COPY --from=ghcr.io/astral-sh/uv:latest /uv /usr/local/bin/uv
RUN apk add --no-cache ca-certificates tzdata deno python3 py3-pip \
    rust cargo cmake make g++ gcc musl-dev python3-dev \
    && addgroup -S cubis && adduser -S cubis -G cubis \
    && mkdir -p /tmp/cubis-cache && chown cubis:cubis /tmp/cubis-cache
WORKDIR /app
COPY --from=builder --chown=root:root --chmod=755 /wkr .
COPY --chown=root:root --chmod=644 config.yml .
COPY --chown=root:root --chmod=644 config-edge.yml .
USER cubis
EXPOSE 8080
CMD ["/app/wkr"]
