# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy common module first
COPY common/ ./common/
WORKDIR /app/common
RUN go mod download

# Copy feed service
WORKDIR /app
COPY feed-service/go.mod feed-service/go.sum ./feed-service/
WORKDIR /app/feed-service
RUN go mod download

COPY feed-service/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /feed-service ./cmd/feed-service

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /feed-service .

EXPOSE 8082

CMD ["./feed-service"]
