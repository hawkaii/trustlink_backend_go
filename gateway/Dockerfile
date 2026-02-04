# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy common module first
COPY common/ ./common/
WORKDIR /app/common
RUN go mod download

# Copy gateway service
WORKDIR /app
COPY gateway/go.mod gateway/go.sum ./gateway/
WORKDIR /app/gateway
RUN go mod download

COPY gateway/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /gateway ./cmd/gateway

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /gateway .

EXPOSE 8080

CMD ["./gateway"]
