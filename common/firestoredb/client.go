package firestoredb

import (
	"context"
	"fmt"
	"os"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/option"
)

var Client *firestore.Client

// Initialize initializes the Firestore client
func Initialize(ctx context.Context) error {
	projectID := os.Getenv("FIREBASE_PROJECT_ID")
	if projectID == "" {
		return fmt.Errorf("FIREBASE_PROJECT_ID environment variable not set")
	}

	credPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if credPath == "" {
		return fmt.Errorf("GOOGLE_APPLICATION_CREDENTIALS environment variable not set")
	}

	opt := option.WithCredentialsFile(credPath)

	var err error
	Client, err = firestore.NewClient(ctx, projectID, opt)
	if err != nil {
		return fmt.Errorf("error initializing Firestore client: %w", err)
	}

	return nil
}

// GetClient returns the Firestore client
func GetClient() *firestore.Client {
	return Client
}

// Close closes the Firestore client
func Close() error {
	if Client != nil {
		return Client.Close()
	}
	return nil
}
