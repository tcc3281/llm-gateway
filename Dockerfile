# Stage 1: Build binaries
FROM golang:1.22-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum* ./
RUN go mod download || true

COPY . .

# Generate go.sum and download all dependencies inside container
RUN go mod tidy

# Build both Gateway HTTP server and CLI binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /bin/gateway ./cmd/gateway
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /bin/llmgw ./cmd/cli


# Stage 2: Minimal runtime image
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /bin/gateway /app/gateway
COPY --from=builder /bin/llmgw /usr/local/bin/llmgw
COPY --from=builder /app/migrations /app/migrations

EXPOSE 8080 8081

CMD ["/app/gateway"]
