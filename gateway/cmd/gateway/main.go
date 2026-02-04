package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
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

	// Setup router
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// CORS is handled by Caddy reverse proxy in production
	// Uncomment for local development without Caddy
	// r.Use(cors.Handler(cors.Options{
	// 	AllowedOrigins:   getAllowedOrigins(),
	// 	AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
	// 	AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
	// 	ExposedHeaders:   []string{"Link"},
	// 	AllowCredentials: true,
	// 	MaxAge:           300,
	// }))

	// Health check
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.Success(w, map[string]string{"status": "ok"})
	})

	// Service proxy routes
	profileURL := getServiceURL("PROFILE_SERVICE_URL", "http://localhost:8081")
	feedURL := getServiceURL("FEED_SERVICE_URL", "http://localhost:8082")
	connectionsURL := getServiceURL("CONNECTIONS_SERVICE_URL", "http://localhost:8083")

	r.Route("/v1", func(r chi.Router) {
		r.Handle("/profile/*", createProxy(profileURL))
		r.Handle("/posts/*", createProxy(feedURL))
		r.Handle("/posts", createProxy(feedURL))
		r.Handle("/connections/*", createProxy(connectionsURL))
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

func createProxy(targetURL string) http.Handler {
	target, _ := url.Parse(targetURL)
	proxy := httputil.NewSingleHostReverseProxy(target)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Don't strip prefix, just forward the request as-is
		r.URL.Host = target.Host
		r.URL.Scheme = target.Scheme
		r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
		r.Host = target.Host

		log.Debug("Proxying request",
			zap.String("original_path", r.URL.Path),
			zap.String("target", targetURL))

		proxy.ServeHTTP(w, r)
	})
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getServiceURL(envKey, fallback string) string {
	return getEnv(envKey, fallback)
}

func getAllowedOrigins() []string {
	origins := getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000,http://10.0.2.2:8080")
	return strings.Split(origins, ",")
}
