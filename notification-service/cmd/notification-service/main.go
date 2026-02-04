package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/trustlink/common/firebaseapp"
	"github.com/trustlink/common/firestoredb"
	"github.com/trustlink/common/log"
	"github.com/trustlink/common/rabbitmq"
	"go.uber.org/zap"
)

// PostCreatedEvent from feed service
type PostCreatedEvent struct {
	PostID    string    `json:"postId"`
	AuthorUID string    `json:"authorUid"`
	CreatedAt time.Time `json:"createdAt"`
}

// ConnectionEvent from connections service
type ConnectionEvent struct {
	FromUID   string    `json:"fromUid"`
	ToUID     string    `json:"toUid"`
	CreatedAt time.Time `json:"createdAt"`
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize logger
	env := getEnv("ENV", "dev")
	if err := log.Initialize(env); err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()

	// Initialize Firebase (needed for FCM later)
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
	rabbitConn, err := rabbitmq.Connect(rabbitURL)
	if err != nil {
		log.Fatal("Failed to connect to RabbitMQ", zap.Error(err))
	}
	defer rabbitConn.Close()
	log.Info("RabbitMQ connected successfully")

	// Start consuming events
	err = rabbitConn.Consume(ctx, rabbitmq.ConsumeOptions{
		QueueName: "notification-service",
		RoutingKeys: []string{
			"post.created",
			"connection.requested",
			"connection.accepted",
		},
		Handler: handleEvent,
	})
	if err != nil {
		log.Fatal("Failed to start consuming", zap.Error(err))
	}

	log.Info("Notification service started")

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down notification service...")
	cancel()

	// Give time for graceful shutdown
	time.Sleep(2 * time.Second)
	log.Info("Notification service stopped")
}

func handleEvent(body []byte) error {
	// Parse generic event to determine type
	var eventType struct {
		PostID  string `json:"postId,omitempty"`
		FromUID string `json:"fromUid,omitempty"`
	}

	if err := json.Unmarshal(body, &eventType); err != nil {
		log.Error("Failed to parse event", zap.Error(err))
		return err
	}

	// Route to appropriate handler based on fields present
	if eventType.PostID != "" {
		return handlePostCreated(body)
	} else if eventType.FromUID != "" {
		return handleConnectionEvent(body)
	}

	log.Warn("Unknown event type", zap.ByteString("body", body))
	return nil
}

func handlePostCreated(body []byte) error {
	var event PostCreatedEvent
	if err := json.Unmarshal(body, &event); err != nil {
		log.Error("Failed to parse post.created event", zap.Error(err))
		return err
	}

	log.Info("Handling post.created event",
		zap.String("postId", event.PostID),
		zap.String("authorUid", event.AuthorUID))

	// TODO: Implement FCM notification logic
	// 1. Query connections of the author
	// 2. Get FCM tokens for connected users
	// 3. Send push notifications via FCM

	return nil
}

func handleConnectionEvent(body []byte) error {
	var event ConnectionEvent
	if err := json.Unmarshal(body, &event); err != nil {
		log.Error("Failed to parse connection event", zap.Error(err))
		return err
	}

	log.Info("Handling connection event",
		zap.String("fromUid", event.FromUID),
		zap.String("toUid", event.ToUID))

	// TODO: Implement FCM notification logic
	// 1. Get FCM tokens for target user (toUid)
	// 2. Send push notification via FCM

	return nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
