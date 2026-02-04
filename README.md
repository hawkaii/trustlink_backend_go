# TrustLink Backend

Go microservices backend for TrustLink platform using Firebase (Auth + Firestore) and RabbitMQ for event-driven architecture.

## Architecture

- **Gateway** (`:8080`) - API gateway with CORS and routing
- **Profile Service** (`:8081`) - User profile management
- **Feed Service** (`:8082`) - Posts and feed management
- **Connections Service** (`:8083`) - User connections/relationships
- **Notification Service** - Event consumer (no HTTP port)

### Technologies

- **Go 1.21+**
- **Firebase Admin SDK** - Authentication & Firestore
- **RabbitMQ** - Event bus
- **Chi Router** - HTTP routing
- **Zap** - Structured logging

## Prerequisites

1. **Go 1.21 or higher**
   ```bash
   go version
   ```

2. **Docker & Docker Compose** (for RabbitMQ)
   ```bash
   docker --version
   docker-compose --version
   ```

3. **Firebase Project**
   - Create a Firebase project at https://console.firebase.google.com
   - Enable Firebase Authentication
   - Enable Cloud Firestore
   - Download service account key:
     - Go to Project Settings > Service Accounts
     - Click "Generate new private key"
     - Save as `serviceAccountKey.json` (keep this file OUTSIDE the repo)

## Setup

### 1. Environment Configuration

Copy the example environment file:

```bash
cp .env.example .env
```

Edit `.env` and set:

```env
ENV=dev
PORT=8080

# Firebase Configuration
FIREBASE_PROJECT_ID=your-firebase-project-id
GOOGLE_APPLICATION_CREDENTIALS=/absolute/path/to/serviceAccountKey.json

# CORS (for Flutter dev)
CORS_ALLOWED_ORIGINS=http://localhost:3000,http://10.0.2.2:8080

# RabbitMQ
RABBITMQ_URL=amqp://guest:guest@localhost:5672/

# Service Ports
GATEWAY_PORT=8080
PROFILE_SERVICE_PORT=8081
FEED_SERVICE_PORT=8082
CONNECTIONS_SERVICE_PORT=8083
NOTIFICATION_SERVICE_PORT=8084
```

### 2. Start RabbitMQ

```bash
docker-compose up -d
```

Verify RabbitMQ is running:
- Management UI: http://localhost:15672 (guest/guest)

### 3. Install Dependencies

```bash
cd common && go mod download && cd ..
cd gateway && go mod download && cd ..
cd profile-service && go mod download && cd ..
cd feed-service && go mod download && cd ..
cd connections-service && go mod download && cd ..
cd notification-service && go mod download && cd ..
```

Or install all at once:

```bash
for dir in common gateway profile-service feed-service connections-service notification-service; do
  (cd $dir && go mod download)
done
```

### 4. Run Services

#### Option A: Run individually (recommended for development)

In separate terminals:

```bash
# Terminal 1: Gateway
cd gateway && go run cmd/gateway/main.go

# Terminal 2: Profile Service
cd profile-service && go run cmd/profile-service/main.go

# Terminal 3: Feed Service
cd feed-service && go run cmd/feed-service/main.go

# Terminal 4: Connections Service
cd connections-service && go run cmd/connections-service/main.go

# Terminal 5: Notification Service
cd notification-service && go run cmd/notification-service/main.go
```

#### Option B: Use a process manager (tmux/tmuxinator)

Create a `tmuxinator.yml`:

```yaml
name: trustlink
root: ~/code/projects/trustlink/backend

windows:
  - services:
      layout: tiled
      panes:
        - gateway:
cd gateway && go run cmd/gateway/main.go
        - profile:
            cd profile-service && go run cmd/profile-service/main.go
        - feed:
            cd feed-service && go run cmd/feed-service/main.go
        - connections:
            cd connections-service && go run cmd/connections-service/main.go
        - notifications:
            cd notification-service && go run cmd/notification-service/main.go
```

## API Endpoints

### Gateway (http://localhost:8080)

#### Health Check
- `GET /healthz` - Gateway health check

### Profile Service

#### Protected Endpoints (require Firebase ID token)
- `GET /v1/profile/me` - Get current user profile (creates if not exists)
- `PATCH /v1/profile/me` - Update current user profile

**Example PATCH Request:**
```json
{
  "displayName": "John Doe",
  "username": "johndoe",
  "profession": "Software Engineer",
  "location": "San Francisco, CA",
  "bio": "Passionate about building great products"
}
```

### Feed Service

#### Protected Endpoints
- `POST /v1/posts` - Create a new post
- `GET /v1/posts?limit=20` - Get latest posts

**Example POST Request:**
```json
{
  "text": "Hello, TrustLink!",
  "mediaUrls": ["https://example.com/image.jpg"]
}
```

### Connections Service

#### Protected Endpoints
- `POST /v1/connections/request` - Send connection request
- `POST /v1/connections/accept` - Accept connection request
- `POST /v1/connections/reject` - Reject connection request
- `GET /v1/connections?status=accepted` - List connections

**Example Request Connection:**
```json
{
  "targetUid": "firebase-uid-of-target-user"
}
```

**Example Accept/Reject:**
```json
{
  "fromUid": "firebase-uid-who-sent-request"
}
```

## Authentication

All protected endpoints require a Firebase ID token in the `Authorization` header:

```
Authorization: Bearer <firebase-id-token>
```

The Flutter app should obtain this token after Firebase Auth sign-in:

```dart
final token = await FirebaseAuth.instance.currentUser!.getIdToken();
```

## Firestore Data Model

### Collections

#### `users/{uid}`
```json
{
  "displayName": "string",
  "username": "string",
  "email": "string",
  "photoUrl": "string (optional)",
  "profession": "string (optional)",
  "birthday": "string (optional)",
  "gender": "string (optional)",
  "location": "string (optional)",
  "bio": "string (optional)",
  "createdAt": "timestamp",
  "updatedAt": "timestamp"
}
```

#### `posts/{postId}`
```json
{
  "authorUid": "string",
  "authorDisplayName": "string",
  "authorPhotoUrl": "string (optional)",
  "text": "string",
  "mediaUrls": ["string"],
  "createdAt": "timestamp"
}
```

#### `relationships/{relationshipId}`
```json
{
  "fromUid": "string",
  "toUid": "string",
  "status": "requested|accepted|rejected",
  "createdAt": "timestamp",
  "updatedAt": "timestamp"
}
```

## RabbitMQ Events

### Exchange: `trustlink.events` (topic)

### Routing Keys

#### `post.created`
```json
{
  "postId": "string",
  "authorUid": "string",
  "createdAt": "timestamp"
}
```

#### `connection.requested`
```json
{
  "fromUid": "string",
  "toUid": "string",
  "createdAt": "timestamp"
}
```

#### `connection.accepted`
```json
{
  "fromUid": "string",
  "toUid": "string",
  "createdAt": "timestamp"
}
```

## Testing

### Manual Testing with cURL

#### 1. Get a Firebase ID token

Use the Flutter app to sign in, or use Firebase REST API:

```bash
# Sign up
curl -X POST \
  'https://identitytoolkit.googleapis.com/v1/accounts:signUp?key=YOUR_API_KEY' \
  -H 'Content-Type: application/json' \
  -d '{
    "email": "test@example.com",
    "password": "password123",
    "returnSecureToken": true
  }'

# Extract idToken from response
```

#### 2. Test endpoints

```bash
# Get profile
curl -X GET http://localhost:8080/v1/profile/me \
  -H "Authorization: Bearer YOUR_ID_TOKEN"

# Create post
curl -X POST http://localhost:8080/v1/posts \
  -H "Authorization: Bearer YOUR_ID_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"text": "Hello, TrustLink!"}'

# Get posts
curl -X GET http://localhost:8080/v1/posts \
  -H "Authorization: Bearer YOUR_ID_TOKEN"
```

## Troubleshooting

### Common Issues

1. **"FIREBASE_PROJECT_ID environment variable not set"**
   - Ensure `.env` file exists and has correct values
   - Export environment variables: `source .env` (or use direnv)

2. **"Failed to connect to RabbitMQ"**
   - Ensure RabbitMQ is running: `docker-compose ps`
   - Check RabbitMQ logs: `docker-compose logs rabbitmq`

3. **401 Unauthorized errors**
   - Verify Firebase ID token is valid and not expired
   - Check that GOOGLE_APPLICATION_CREDENTIALS points to correct file
   - Verify Firebase project ID matches

4. **CORS errors from Flutter**
   - Add Android emulator IP to CORS_ALLOWED_ORIGINS: `http://10.0.2.2:8080`
   - For iOS simulator, add: `http://localhost:8080`

## Development Tips

### Hot Reload with Air

Install Air for hot reload:

```bash
go install github.com/cosmtrek/air@latest
```

Create `.air.toml` in each service directory, then run:

```bash
air
```

### Viewing Logs

All services use structured logging (Zap). In development mode, logs are colorized.

### Database Indexes

For production, create Firestore indexes:

```
Collection: posts
- createdAt (Descending)

Collection: relationships
- fromUid (Ascending), status (Ascending)
- toUid (Ascending), status (Ascending)
```

## Next Steps

1. **Integrate Flutter app with Firebase Auth** (see `../TrustLink/README.md`)
2. **Implement FCM push notifications** in notification-service
3. **Add pagination** to feed and connections endpoints
4. **Implement search** functionality
5. **Add rate limiting** to prevent abuse
6. **Set up CI/CD** pipeline
7. **Deploy to Cloud Run** or Kubernetes

## License

MIT
