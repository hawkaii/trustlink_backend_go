# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy common module first
COPY common/ ./common/
WORKDIR /app/common
RUN go mod download

# Copy notification service
WORKDIR /app
COPY notification-service/go.mod notification-service/go.sum ./notification-service/
WORKDIR /app/notification-service
RUN go mod download

COPY notification-service/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /notification-service ./cmd/notification-service

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /notification-service .

EXPOSE 8084

CMD ["./notification-service"]
