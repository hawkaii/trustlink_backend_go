# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy common module first
COPY common/ ./common/
WORKDIR /app/common
RUN go mod download

# Copy profile service
WORKDIR /app
COPY profile-service/go.mod profile-service/go.sum ./profile-service/
WORKDIR /app/profile-service
RUN go mod download

COPY profile-service/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /profile-service ./cmd/profile-service

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /profile-service .

EXPOSE 8081

CMD ["./profile-service"]
