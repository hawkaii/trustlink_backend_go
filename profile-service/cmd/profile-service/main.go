package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/trustlink/common/authmw"
	"github.com/trustlink/common/firebaseapp"
	"github.com/trustlink/common/firestoredb"
	"github.com/trustlink/common/httpx"
	"github.com/trustlink/common/log"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// User represents a user profile in Firestore
type User struct {
	UID         string    `firestore:"-" json:"uid"`
	DisplayName string    `firestore:"displayName" json:"displayName"`
	Username    string    `firestore:"username" json:"username"`
	Email       string    `firestore:"email" json:"email"`
	PhotoURL    string    `firestore:"photoUrl,omitempty" json:"photoUrl,omitempty"`
	Profession  string    `firestore:"profession,omitempty" json:"profession,omitempty"`
	Birthday    string    `firestore:"birthday,omitempty" json:"birthday,omitempty"`
	Gender      string    `firestore:"gender,omitempty" json:"gender,omitempty"`
	Location    string    `firestore:"location,omitempty" json:"location,omitempty"`
	Bio         string    `firestore:"bio,omitempty" json:"bio,omitempty"`
	CreatedAt   time.Time `firestore:"createdAt" json:"createdAt"`
	UpdatedAt   time.Time `firestore:"updatedAt" json:"updatedAt"`
}

// UpdateProfileRequest represents the request body for profile updates
type UpdateProfileRequest struct {
	DisplayName *string `json:"displayName,omitempty"`
	Username    *string `json:"username,omitempty"`
	PhotoURL    *string `json:"photoUrl,omitempty"`
	Profession  *string `json:"profession,omitempty"`
	Birthday    *string `json:"birthday,omitempty"`
	Gender      *string `json:"gender,omitempty"`
	Location    *string `json:"location,omitempty"`
	Bio         *string `json:"bio,omitempty"`
}

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

	// Setup router
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// Health check
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.Success(w, map[string]string{"status": "ok", "service": "profile"})
	})

	// Protected routes
	r.Route("/v1/profile", func(r chi.Router) {
		r.Use(authmw.AuthMiddleware)
		r.Get("/me", getProfile)
		r.Patch("/me", updateProfile)
	})

	// Start server
	port := getEnv("PORT", "8081")
	server := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info("Profile service starting", zap.String("port", port))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Server failed to start", zap.Error(err))
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down profile service...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown", zap.Error(err))
	}

	log.Info("Profile service stopped")
}

func getProfile(w http.ResponseWriter, r *http.Request) {
	uid, ok := authmw.GetUserID(r.Context())
	if !ok {
		httpx.Unauthorized(w, "User ID not found in context")
		return
	}

	ctx := r.Context()
	client := firestoredb.GetClient()
	docRef := client.Collection("users").Doc(uid)

	doc, err := docRef.Get(ctx)
	if err != nil {
		// If user doesn't exist, create a minimal profile
		if status.Code(err) == codes.NotFound {
			log.Info("User not found, creating profile", zap.String("uid", uid))

			// Get user info from Firebase Auth
			authClient := firebaseapp.GetAuthClient()
			userRecord, err := authClient.GetUser(ctx, uid)
			if err != nil {
				log.Error("Failed to get user from Auth", zap.Error(err))
				httpx.InternalServerError(w, "Failed to create profile")
				return
			}

			now := time.Now()
			user := User{
				UID:         uid,
				Email:       userRecord.Email,
				DisplayName: userRecord.DisplayName,
				PhotoURL:    userRecord.PhotoURL,
				CreatedAt:   now,
				UpdatedAt:   now,
			}

			// Create user document
			_, err = docRef.Set(ctx, user)
			if err != nil {
				log.Error("Failed to create user document", zap.Error(err))
				httpx.InternalServerError(w, "Failed to create profile")
				return
			}

			user.UID = uid
			httpx.Success(w, user)
			return
		}

		log.Error("Failed to get user document", zap.Error(err))
		httpx.InternalServerError(w, "Failed to get profile")
		return
	}

	var user User
	if err := doc.DataTo(&user); err != nil {
		log.Error("Failed to parse user document", zap.Error(err))
		httpx.InternalServerError(w, "Failed to parse profile")
		return
	}

	user.UID = uid
	httpx.Success(w, user)
}

func updateProfile(w http.ResponseWriter, r *http.Request) {
	uid, ok := authmw.GetUserID(r.Context())
	if !ok {
		httpx.Unauthorized(w, "User ID not found in context")
		return
	}

	var req UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.BadRequest(w, "Invalid request body")
		return
	}

	ctx := r.Context()
	client := firestoredb.GetClient()
	docRef := client.Collection("users").Doc(uid)

	// Build update map
	updates := []firestore.Update{
		{Path: "updatedAt", Value: time.Now()},
	}

	if req.DisplayName != nil {
		updates = append(updates, firestore.Update{Path: "displayName", Value: *req.DisplayName})
	}
	if req.Username != nil {
		updates = append(updates, firestore.Update{Path: "username", Value: *req.Username})
	}
	if req.PhotoURL != nil {
		updates = append(updates, firestore.Update{Path: "photoUrl", Value: *req.PhotoURL})
	}
	if req.Profession != nil {
		updates = append(updates, firestore.Update{Path: "profession", Value: *req.Profession})
	}
	if req.Birthday != nil {
		updates = append(updates, firestore.Update{Path: "birthday", Value: *req.Birthday})
	}
	if req.Gender != nil {
		updates = append(updates, firestore.Update{Path: "gender", Value: *req.Gender})
	}
	if req.Location != nil {
		updates = append(updates, firestore.Update{Path: "location", Value: *req.Location})
	}
	if req.Bio != nil {
		updates = append(updates, firestore.Update{Path: "bio", Value: *req.Bio})
	}

	// Update document
	_, err := docRef.Update(ctx, updates)
	if err != nil {
		log.Error("Failed to update user document", zap.Error(err))
		httpx.InternalServerError(w, "Failed to update profile")
		return
	}

	log.Info("Profile updated", zap.String("uid", uid))

	// Fetch and return updated profile
	doc, err := docRef.Get(ctx)
	if err != nil {
		log.Error("Failed to get updated user document", zap.Error(err))
		httpx.InternalServerError(w, "Failed to get updated profile")
		return
	}

	var user User
	if err := doc.DataTo(&user); err != nil {
		log.Error("Failed to parse user document", zap.Error(err))
		httpx.InternalServerError(w, "Failed to parse profile")
		return
	}

	user.UID = uid
	httpx.Success(w, user)
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
