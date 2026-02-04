package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/trustlink/common/authmw"
	"github.com/trustlink/common/firebaseapp"
	"github.com/trustlink/common/firestoredb"
	"github.com/trustlink/common/httpx"
	"github.com/trustlink/common/log"
	"github.com/trustlink/common/rabbitmq"
	"go.uber.org/zap"
	"google.golang.org/api/iterator"
)

// Post represents a post in Firestore
type Post struct {
	ID                string    `firestore:"-" json:"id"`
	AuthorUID         string    `firestore:"authorUid" json:"authorUid"`
	AuthorDisplayName string    `firestore:"authorDisplayName" json:"authorDisplayName"`
	AuthorPhotoURL    string    `firestore:"authorPhotoUrl,omitempty" json:"authorPhotoUrl,omitempty"`
	Text              string    `firestore:"text" json:"text"`
	MediaURLs         []string  `firestore:"mediaUrls,omitempty" json:"mediaUrls,omitempty"`
	CreatedAt         time.Time `firestore:"createdAt" json:"createdAt"`
}

// CreatePostRequest represents the request body for creating a post
type CreatePostRequest struct {
	Text      string   `json:"text"`
	MediaURLs []string `json:"mediaUrls,omitempty"`
}

// PostCreatedEvent is published to RabbitMQ when a post is created
type PostCreatedEvent struct {
	PostID    string    `json:"postId"`
	AuthorUID string    `json:"authorUid"`
	CreatedAt time.Time `json:"createdAt"`
}

var rabbitConn *rabbitmq.Connection

func main() {
	ctx := context.Background()

	// Initialize logger
	env := getEnv("ENV", "dev")
	if err := log.Initialize(env); err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()

	// Initialize Firebase
	if err := firebaseapp.Initialize(ctx); err != nil {
		log.Fatal("Failed to initialize Firebase", zap.Error(err))
	}
	log.Info("Firebase initialized successfully")

	// Initialize Firestore
	if err := firestoredb.Initialize(ctx); err != nil {
		log.Fatal("Failed to initialize Firestore", zap.Error(err))
	}
	defer firestoredb.Close()
	log.Info("Firestore initialized successfully")

	// Initialize RabbitMQ
	rabbitURL := getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")
	var err error
	rabbitConn, err = rabbitmq.Connect(rabbitURL)
	if err != nil {
		log.Fatal("Failed to connect to RabbitMQ", zap.Error(err))
	}
	defer rabbitConn.Close()
	log.Info("RabbitMQ connected successfully")

	// Setup router
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// Health check
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.Success(w, map[string]string{"status": "ok", "service": "feed"})
	})

	// Protected routes
	r.Route("/v1/posts", func(r chi.Router) {
		r.Use(authmw.AuthMiddleware)
		r.Post("/", createPost)
		r.Get("/", getPosts)
	})

	// Start server
	port := getEnv("PORT", "8082")
	server := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info("Feed service starting", zap.String("port", port))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Server failed to start", zap.Error(err))
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down feed service...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown", zap.Error(err))
	}

	log.Info("Feed service stopped")
}

func createPost(w http.ResponseWriter, r *http.Request) {
	uid, ok := authmw.GetUserID(r.Context())
	if !ok {
		httpx.Unauthorized(w, "User ID not found in context")
		return
	}

	var req CreatePostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.BadRequest(w, "Invalid request body")
		return
	}

	if req.Text == "" {
		httpx.BadRequest(w, "Text is required")
		return
	}

	ctx := r.Context()
	client := firestoredb.GetClient()

	// Get user profile for denormalized data
	userDoc, err := client.Collection("users").Doc(uid).Get(ctx)
	if err != nil {
		log.Error("Failed to get user profile", zap.Error(err))
		httpx.InternalServerError(w, "Failed to get user profile")
		return
	}

	displayName := userDoc.Data()["displayName"].(string)
	photoURL := ""
	if url, ok := userDoc.Data()["photoUrl"].(string); ok {
		photoURL = url
	}

	// Create post
	now := time.Now()
	postID := uuid.New().String()
	post := Post{
		ID:                postID,
		AuthorUID:         uid,
		AuthorDisplayName: displayName,
		AuthorPhotoURL:    photoURL,
		Text:              req.Text,
		MediaURLs:         req.MediaURLs,
		CreatedAt:         now,
	}

	// Save to Firestore
	_, err = client.Collection("posts").Doc(postID).Set(ctx, post)
	if err != nil {
		log.Error("Failed to create post", zap.Error(err))
		httpx.InternalServerError(w, "Failed to create post")
		return
	}

	log.Info("Post created", zap.String("postId", postID), zap.String("authorUid", uid))

	// Publish event to RabbitMQ
	event := PostCreatedEvent{
		PostID:    postID,
		AuthorUID: uid,
		CreatedAt: now,
	}

	if err := rabbitConn.Publish(ctx, "post.created", event); err != nil {
		log.Error("Failed to publish post.created event", zap.Error(err))
		// Don't fail the request if event publishing fails
	}

	httpx.Created(w, post)
}

func getPosts(w http.ResponseWriter, r *http.Request) {
	// Get limit from query params
	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	ctx := r.Context()
	client := firestoredb.GetClient()

	// Query posts ordered by createdAt descending
	iter := client.Collection("posts").
		OrderBy("createdAt", firestore.Desc).
		Limit(limit).
		Documents(ctx)
	defer iter.Stop()

	var posts []Post
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Error("Failed to iterate posts", zap.Error(err))
			httpx.InternalServerError(w, "Failed to fetch posts")
			return
		}

		var post Post
		if err := doc.DataTo(&post); err != nil {
			log.Error("Failed to parse post", zap.Error(err))
			continue
		}

		post.ID = doc.Ref.ID
		posts = append(posts, post)
	}

	if posts == nil {
		posts = []Post{}
	}

	httpx.Success(w, map[string]interface{}{
		"posts": posts,
		"count": len(posts),
	})
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
