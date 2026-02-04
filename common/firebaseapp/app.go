package firebaseapp

import (
	"context"
	"fmt"
	"os"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"google.golang.org/api/option"
)

var (
	App        *firebase.App
	AuthClient *auth.Client
)

// Initialize initializes the Firebase Admin SDK
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
	config := &firebase.Config{
		ProjectID: projectID,
	}

	var err error
	App, err = firebase.NewApp(ctx, config, opt)
	if err != nil {
		return fmt.Errorf("error initializing Firebase app: %w", err)
	}

	AuthClient, err = App.Auth(ctx)
	if err != nil {
		return fmt.Errorf("error initializing Firebase Auth client: %w", err)
	}

	return nil
}

// GetAuthClient returns the Firebase Auth client
func GetAuthClient() *auth.Client {
	return AuthClient
}
