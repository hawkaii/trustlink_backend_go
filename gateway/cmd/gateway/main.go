package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/trustlink/common/authmw"
	"github.com/trustlink/common/firebaseapp"
	"github.com/trustlink/common/firestoredb"
	"github.com/trustlink/common/httpx"
	"github.com/trustlink/common/log"
	"go.uber.org/zap"
)

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

	// Initialize GCS (optional — upload presign endpoint only)
	if err := initGCS(ctx); err != nil {
		log.Warn("GCS initialization skipped", zap.Error(err))
	}

	// Setup router
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// CORS — enabled for development (Caddy handles this in prod)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   getAllowedOrigins(),
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Health check
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.Success(w, map[string]string{"status": "ok"})
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		httpx.Success(w, map[string]string{"status": "ready"})
	})

	// API routes
	r.Route("/v1", func(r chi.Router) {
		// Upload presign endpoint
		r.Post("/uploads/presign", handlePresignUpload)

		// Profile routes (protected)
		r.Route("/profile", func(r chi.Router) {
			r.Use(authmw.AuthMiddleware)
			r.Get("/me", handleGetProfile)
			r.Patch("/me", handleUpdateProfile)
		})

		// Post routes (protected)
		r.Route("/posts", func(r chi.Router) {
			r.Use(authmw.AuthMiddleware)
			r.Post("/", handleCreatePost)
			r.Get("/", handleGetPosts)
		})
	})

	// Start server
	port := getEnv("PORT", "8080")
	server := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		log.Info("Gateway starting", zap.String("port", port))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Server failed to start", zap.Error(err))
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down gateway...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown", zap.Error(err))
	}

	log.Info("Gateway stopped")
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getAllowedOrigins() []string {
	origins := getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000,http://10.0.2.2:8080,http://35.253.120.86:8080")
	return strings.Split(origins, ",")
}
