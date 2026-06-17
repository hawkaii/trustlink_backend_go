package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/storage"
	"github.com/trustlink/common/httpx"
	"github.com/trustlink/common/log"
	"go.uber.org/zap"
	"google.golang.org/api/option"
)

type PresignRequest struct {
	FileName    string `json:"fileName"`
	ContentType string `json:"contentType"`
}

type PresignResponse struct {
	PutURL    string `json:"putUrl"`
	PublicURL string `json:"publicUrl"`
	FileName  string `json:"fileName"`
}

var gcsClient *storage.Client
var gcsBucket string

func initGCS(ctx context.Context) error {
	bucket := os.Getenv("GCS_BUCKET")
	if bucket == "" {
		log.Warn("GCS_BUCKET not set — upload presign endpoint will be unavailable")
		return nil
	}

	creds := os.Getenv("GCS_CREDENTIALS")
	var client *storage.Client
	var err error

	if creds != "" {
		client, err = storage.NewClient(ctx, option.WithCredentialsFile(creds))
	} else {
		client, err = storage.NewClient(ctx)
	}

	if err != nil {
		return fmt.Errorf("failed to create GCS client: %w", err)
	}

	gcsClient = client
	gcsBucket = bucket
	log.Info("GCS client initialized", zap.String("bucket", bucket))
	return nil
}

func handlePresignUpload(w http.ResponseWriter, r *http.Request) {
	if gcsClient == nil {
		httpx.BadRequest(w, "GCS not configured — set GCS_BUCKET and GCS_CREDENTIALS env vars")
		return
	}

	var req PresignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.BadRequest(w, "Invalid request body")
		return
	}

	if req.FileName == "" {
		httpx.BadRequest(w, "fileName is required")
		return
	}

	objectName := fmt.Sprintf("uploads/%d/%s", time.Now().Unix(), req.FileName)
	contentType := req.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	ctx := r.Context()
	putURL, err := generateSignedPutURL(ctx, gcsBucket, objectName, contentType)
	if err != nil {
		log.Error("Failed to generate signed URL", zap.Error(err))
		httpx.InternalServerError(w, "Failed to generate upload URL")
		return
	}

	publicURL := fmt.Sprintf("https://storage.googleapis.com/%s/%s", gcsBucket, objectName)

	resp := PresignResponse{
		PutURL:    putURL,
		PublicURL: publicURL,
		FileName:  objectName,
	}

	httpx.Success(w, resp)
}

func generateSignedPutURL(ctx context.Context, bucket, object, contentType string) (string, error) {
	opts := &storage.SignedURLOptions{
		Method:         http.MethodPut,
		ContentType:    contentType,
		Expires:        time.Now().Add(15 * time.Minute),
		Scheme:         storage.SigningSchemeV4,
	}

	return gcsClient.Bucket(bucket).SignedURL(object, opts)
}
