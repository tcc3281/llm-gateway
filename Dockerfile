# Stage 1: Build React frontend
FROM node:20-alpine AS web-builder
WORKDIR /app
COPY web/package*.json ./web/
RUN cd web && npm ci
COPY web ./web
RUN cd web && npm run build

# Stage 2: Build Go binaries
FROM golang:1.22-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum* ./
RUN go mod download || true
COPY . .
# Copy the compiled frontend dist from web-builder stage
COPY --from=web-builder /app/internal/web/dist ./internal/web/dist
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /bin/gateway ./cmd/gateway
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /bin/proxy ./cmd/proxy
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /bin/control-plane ./cmd/control-plane
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /bin/worker ./cmd/worker

# Stage 3: Minimal runtime image
FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /bin/gateway /app/gateway
COPY --from=builder /bin/proxy /app/proxy
COPY --from=builder /bin/control-plane /app/control-plane
COPY --from=builder /bin/worker /app/worker
COPY --from=builder /app/migrations /app/migrations
EXPOSE 8080 8081 8082
CMD ["/app/gateway"]
