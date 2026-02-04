# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy common module first
COPY common/ ./common/
WORKDIR /app/common
RUN go mod download

# Copy connections service
WORKDIR /app
COPY connections-service/go.mod connections-service/go.sum ./connections-service/
WORKDIR /app/connections-service
RUN go mod download

COPY connections-service/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /connections-service ./cmd/connections-service

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /connections-service .

EXPOSE 8083

CMD ["./connections-service"]
