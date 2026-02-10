# Build stage
FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git ca-certificates
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /wkr ./cmd/server

# Runtime stage — needs Go for compiling Go workers, Deno for JS/TS workers
FROM golang:1.25-alpine
RUN apk add --no-cache ca-certificates tzdata deno
RUN addgroup -S cubis && adduser -S cubis -G cubis
# Pre-create cache dirs writable by cubis user
RUN mkdir -p /tmp/cubis-cache && chown cubis:cubis /tmp/cubis-cache
WORKDIR /app
COPY --from=builder /wkr .
COPY config.yml .
COPY config-edge.yml .
RUN chown -R cubis:cubis /app
USER cubis
EXPOSE 8080
ENTRYPOINT ["./wkr"]
