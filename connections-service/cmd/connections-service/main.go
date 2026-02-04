package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sort"
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
	"github.com/trustlink/common/rabbitmq"
	"go.uber.org/zap"
	"google.golang.org/api/iterator"
)

// RelationshipStatus represents the status of a connection
type RelationshipStatus string

const (
	StatusRequested RelationshipStatus = "requested"
	StatusAccepted  RelationshipStatus = "accepted"
	StatusRejected  RelationshipStatus = "rejected"
)

// Relationship represents a connection between two users
type Relationship struct {
	ID        string             `firestore:"-" json:"id"`
	FromUID   string             `firestore:"fromUid" json:"fromUid"`
	ToUID     string             `firestore:"toUid" json:"toUid"`
	Status    RelationshipStatus `firestore:"status" json:"status"`
	CreatedAt time.Time          `firestore:"createdAt" json:"createdAt"`
	UpdatedAt time.Time          `firestore:"updatedAt" json:"updatedAt"`
}

// ConnectionRequestRequest represents a connection request
type ConnectionRequestRequest struct {
	TargetUID string `json:"targetUid"`
}

// ConnectionActionRequest represents accept/reject actions
type ConnectionActionRequest struct {
	FromUID string `json:"fromUid"`
}

// ConnectionEvent is published to RabbitMQ
type ConnectionEvent struct {
	FromUID   string    `json:"fromUid"`
	ToUID     string    `json:"toUid"`
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
		httpx.Success(w, map[string]string{"status": "ok", "service": "connections"})
	})

	// Protected routes
	r.Route("/v1/connections", func(r chi.Router) {
		r.Use(authmw.AuthMiddleware)
		r.Post("/request", requestConnection)
		r.Post("/accept", acceptConnection)
		r.Post("/reject", rejectConnection)
		r.Get("/", getConnections)
	})

	// Start server
	port := getEnv("PORT", "8083")
	server := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info("Connections service starting", zap.String("port", port))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Server failed to start", zap.Error(err))
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down connections service...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown", zap.Error(err))
	}

	log.Info("Connections service stopped")
}

func requestConnection(w http.ResponseWriter, r *http.Request) {
	uid, ok := authmw.GetUserID(r.Context())
	if !ok {
		httpx.Unauthorized(w, "User ID not found in context")
		return
	}

	var req ConnectionRequestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.BadRequest(w, "Invalid request body")
		return
	}

	if req.TargetUID == "" {
		httpx.BadRequest(w, "targetUid is required")
		return
	}

	if req.TargetUID == uid {
		httpx.BadRequest(w, "Cannot connect with yourself")
		return
	}

	ctx := r.Context()
	client := firestoredb.GetClient()

	// Create deterministic relationship ID
	relationshipID := createRelationshipID(uid, req.TargetUID)

	now := time.Now()
	relationship := Relationship{
		ID:        relationshipID,
		FromUID:   uid,
		ToUID:     req.TargetUID,
		Status:    StatusRequested,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Save to Firestore
	_, err := client.Collection("relationships").Doc(relationshipID).Set(ctx, relationship)
	if err != nil {
		log.Error("Failed to create connection request", zap.Error(err))
		httpx.InternalServerError(w, "Failed to create connection request")
		return
	}

	log.Info("Connection requested",
		zap.String("fromUid", uid),
		zap.String("toUid", req.TargetUID))

	// Publish event
	event := ConnectionEvent{
		FromUID:   uid,
		ToUID:     req.TargetUID,
		CreatedAt: now,
	}

	if err := rabbitConn.Publish(ctx, "connection.requested", event); err != nil {
		log.Error("Failed to publish connection.requested event", zap.Error(err))
	}

	httpx.Created(w, relationship)
}

func acceptConnection(w http.ResponseWriter, r *http.Request) {
	uid, ok := authmw.GetUserID(r.Context())
	if !ok {
		httpx.Unauthorized(w, "User ID not found in context")
		return
	}

	var req ConnectionActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.BadRequest(w, "Invalid request body")
		return
	}

	if req.FromUID == "" {
		httpx.BadRequest(w, "fromUid is required")
		return
	}

	ctx := r.Context()
	client := firestoredb.GetClient()

	relationshipID := createRelationshipID(req.FromUID, uid)
	docRef := client.Collection("relationships").Doc(relationshipID)

	// Update status
	now := time.Now()
	_, err := docRef.Update(ctx, []firestore.Update{
		{Path: "status", Value: string(StatusAccepted)},
		{Path: "updatedAt", Value: now},
	})
	if err != nil {
		log.Error("Failed to accept connection", zap.Error(err))
		httpx.InternalServerError(w, "Failed to accept connection")
		return
	}

	log.Info("Connection accepted",
		zap.String("fromUid", req.FromUID),
		zap.String("toUid", uid))

	// Publish event
	event := ConnectionEvent{
		FromUID:   req.FromUID,
		ToUID:     uid,
		CreatedAt: now,
	}

	if err := rabbitConn.Publish(ctx, "connection.accepted", event); err != nil {
		log.Error("Failed to publish connection.accepted event", zap.Error(err))
	}

	// Fetch and return updated relationship
	doc, err := docRef.Get(ctx)
	if err != nil {
		log.Error("Failed to get updated relationship", zap.Error(err))
		httpx.InternalServerError(w, "Failed to get updated relationship")
		return
	}

	var relationship Relationship
	if err := doc.DataTo(&relationship); err != nil {
		log.Error("Failed to parse relationship", zap.Error(err))
		httpx.InternalServerError(w, "Failed to parse relationship")
		return
	}

	relationship.ID = doc.Ref.ID
	httpx.Success(w, relationship)
}

func rejectConnection(w http.ResponseWriter, r *http.Request) {
	uid, ok := authmw.GetUserID(r.Context())
	if !ok {
		httpx.Unauthorized(w, "User ID not found in context")
		return
	}

	var req ConnectionActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.BadRequest(w, "Invalid request body")
		return
	}

	if req.FromUID == "" {
		httpx.BadRequest(w, "fromUid is required")
		return
	}

	ctx := r.Context()
	client := firestoredb.GetClient()

	relationshipID := createRelationshipID(req.FromUID, uid)
	docRef := client.Collection("relationships").Doc(relationshipID)

	// Update status
	_, err := docRef.Update(ctx, []firestore.Update{
		{Path: "status", Value: string(StatusRejected)},
		{Path: "updatedAt", Value: time.Now()},
	})
	if err != nil {
		log.Error("Failed to reject connection", zap.Error(err))
		httpx.InternalServerError(w, "Failed to reject connection")
		return
	}

	log.Info("Connection rejected",
		zap.String("fromUid", req.FromUID),
		zap.String("toUid", uid))

	// Fetch and return updated relationship
	doc, err := docRef.Get(ctx)
	if err != nil {
		log.Error("Failed to get updated relationship", zap.Error(err))
		httpx.InternalServerError(w, "Failed to get updated relationship")
		return
	}

	var relationship Relationship
	if err := doc.DataTo(&relationship); err != nil {
		log.Error("Failed to parse relationship", zap.Error(err))
		httpx.InternalServerError(w, "Failed to parse relationship")
		return
	}

	relationship.ID = doc.Ref.ID
	httpx.Success(w, relationship)
}

func getConnections(w http.ResponseWriter, r *http.Request) {
	uid, ok := authmw.GetUserID(r.Context())
	if !ok {
		httpx.Unauthorized(w, "User ID not found in context")
		return
	}

	status := r.URL.Query().Get("status")
	if status == "" {
		status = string(StatusAccepted)
	}

	ctx := r.Context()
	client := firestoredb.GetClient()

	// Query connections where user is either fromUid or toUid
	var relationships []Relationship

	// Query where user is fromUid
	iter1 := client.Collection("relationships").
		Where("fromUid", "==", uid).
		Where("status", "==", status).
		Documents(ctx)
	defer iter1.Stop()

	for {
		doc, err := iter1.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Error("Failed to iterate relationships", zap.Error(err))
			httpx.InternalServerError(w, "Failed to fetch connections")
			return
		}

		var rel Relationship
		if err := doc.DataTo(&rel); err != nil {
			log.Error("Failed to parse relationship", zap.Error(err))
			continue
		}

		rel.ID = doc.Ref.ID
		relationships = append(relationships, rel)
	}

	// Query where user is toUid
	iter2 := client.Collection("relationships").
		Where("toUid", "==", uid).
		Where("status", "==", status).
		Documents(ctx)
	defer iter2.Stop()

	for {
		doc, err := iter2.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Error("Failed to iterate relationships", zap.Error(err))
			httpx.InternalServerError(w, "Failed to fetch connections")
			return
		}

		var rel Relationship
		if err := doc.DataTo(&rel); err != nil {
			log.Error("Failed to parse relationship", zap.Error(err))
			continue
		}

		rel.ID = doc.Ref.ID
		relationships = append(relationships, rel)
	}

	if relationships == nil {
		relationships = []Relationship{}
	}

	httpx.Success(w, map[string]interface{}{
		"connections": relationships,
		"count":       len(relationships),
	})
}

func createRelationshipID(uid1, uid2 string) string {
	// Create deterministic ID by sorting UIDs
	uids := []string{uid1, uid2}
	sort.Strings(uids)
	return uids[0] + "_" + uids[1]
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
