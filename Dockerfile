# Build stage
FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git ca-certificates
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /cubis-wkr ./cmd/server

# Runtime stage
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata deno
RUN addgroup -S cubis && adduser -S cubis -G cubis
WORKDIR /app
COPY --from=builder /cubis-wkr .
COPY config.yml .
RUN chown -R cubis:cubis /app
USER cubis
EXPOSE 8080
ENTRYPOINT ["./cubis-wkr"]
