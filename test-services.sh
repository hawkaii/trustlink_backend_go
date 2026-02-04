#!/bin/bash

# Test TrustLink Backend Services
echo "Testing TrustLink Backend Services"
echo "==================================="
echo ""

# Test Gateway Health
echo "1. Testing Gateway Health..."
HEALTH_RESPONSE=$(curl -s http://localhost:8080/healthz)
echo "   Response: $HEALTH_RESPONSE"
echo ""

# Test Profile Service without auth (should get 401)
echo "2. Testing Profile Endpoint without auth (should get 401)..."
PROFILE_NO_AUTH=$(curl -s -w "\nHTTP_CODE:%{http_code}" http://localhost:8080/v1/profile/me)
echo "   Response: $PROFILE_NO_AUTH"
echo ""

# Instructions for testing with Firebase token
echo "3. To test with Firebase authentication:"
echo "   a. Create a user in Firebase Console or use Flutter app"
echo "   b. Get the ID token"
echo "   c. Test with: curl -H \"Authorization: Bearer <token>\" http://localhost:8080/v1/profile/me"
echo ""

echo "Services Status:"
echo "================"
ps aux | grep -E "(gateway|profile-service)" | grep "go run" | grep -v grep | while read -r line; do
    echo "  ✓ $(echo "$line" | awk '{print $11, $12, $13}')"
done
echo ""

echo "Log Files:"
echo "=========="
echo "  Gateway: /home/hawkaii/code/projects/trustlink/backend/gateway/gateway.log"
echo "  Profile: /home/hawkaii/code/projects/trustlink/backend/profile-service/profile.log"
