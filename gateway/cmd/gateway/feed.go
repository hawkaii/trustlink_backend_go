package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/google/uuid"
	"github.com/trustlink/common/authmw"
	"github.com/trustlink/common/firestoredb"
	"github.com/trustlink/common/httpx"
	"github.com/trustlink/common/log"
	"go.uber.org/zap"
	"google.golang.org/api/iterator"
)

// Post represents a post in Firestore
type Post struct {
	ID                string   `firestore:"-" json:"id"`
	AuthorUID         string   `firestore:"authorUid" json:"authorUid"`
	AuthorDisplayName string   `firestore:"authorDisplayName" json:"authorDisplayName"`
	AuthorPhotoURL    string   `firestore:"authorPhotoUrl,omitempty" json:"authorPhotoUrl,omitempty"`
	Text              string   `firestore:"text" json:"text"`
	MediaURLs         []string `firestore:"mediaUrls,omitempty" json:"mediaUrls,omitempty"`
	CreatedAt         string   `firestore:"createdAt" json:"createdAt"`
}

// CreatePostRequest represents the request body for creating a post
type CreatePostRequest struct {
	Text      string   `json:"text"`
	MediaURLs []string `json:"mediaUrls,omitempty"`
}

func handleCreatePost(w http.ResponseWriter, r *http.Request) {
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
	postID := uuid.New().String()
	now := time.Now().Format(time.RFC3339)
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

	httpx.Created(w, post)
}

func handleGetPosts(w http.ResponseWriter, r *http.Request) {
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
