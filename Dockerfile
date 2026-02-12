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

# Runtime stage — needs Go for compiling Go workers, Deno for JS/TS workers
FROM golang:1.25-alpine
RUN apk add --no-cache ca-certificates tzdata deno \
    && addgroup -S cubis && adduser -S cubis -G cubis \
    && mkdir -p /tmp/cubis-cache && chown cubis:cubis /tmp/cubis-cache
WORKDIR /app
COPY --from=builder --chown=cubis:cubis --chmod=0755 /wkr .
COPY --chown=cubis:cubis config.yml .
COPY --chown=cubis:cubis config-edge.yml .
USER cubis
EXPOSE 8080
ENTRYPOINT ["./wkr"]
