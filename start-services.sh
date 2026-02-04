#!/bin/bash

# Load environment variables
export GOOGLE_APPLICATION_CREDENTIALS=/home/hawkaii/code/projects/trustlink/trustlink-1bae8-firebase-adminsdk-fbsvc-8f2395f569.json
export FIREBASE_PROJECT_ID=trustlink-1bae8
export GATEWAY_PORT=8080
export PROFILE_SERVICE_URL=http://localhost:8081
export FEED_SERVICE_URL=http://localhost:8082
export CONNECTIONS_SERVICE_URL=http://localhost:8083
export PROFILE_SERVICE_PORT=8081
export FEED_SERVICE_PORT=8082
export CONNECTIONS_SERVICE_PORT=8083
export NOTIFICATION_SERVICE_PORT=8084
export RABBITMQ_URL=amqp://guest:guest@localhost:5672/

echo "Starting Gateway Service on port $GATEWAY_PORT..."
cd /home/hawkaii/code/projects/trustlink/backend/gateway
nohup go run cmd/gateway/main.go > gateway.log 2>&1 &
GATEWAY_PID=$!
echo "Gateway started with PID: $GATEWAY_PID"

sleep 2

echo "Starting Profile Service on port $PROFILE_SERVICE_PORT..."
cd /home/hawkaii/code/projects/trustlink/backend/profile-service
nohup go run cmd/profile-service/main.go > profile.log 2>&1 &
PROFILE_PID=$!
echo "Profile Service started with PID: $PROFILE_PID"

sleep 2

echo ""
echo "Services started successfully!"
echo "Gateway PID: $GATEWAY_PID"
echo "Profile Service PID: $PROFILE_PID"
echo ""
echo "View logs:"
echo "  Gateway: tail -f /home/hawkaii/code/projects/trustlink/backend/gateway/gateway.log"
echo "  Profile: tail -f /home/hawkaii/code/projects/trustlink/backend/profile-service/profile.log"
echo ""
echo "Stop services:"
echo "  kill $GATEWAY_PID $PROFILE_PID"
