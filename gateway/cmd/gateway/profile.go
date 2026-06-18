package main

import (
	"encoding/json"
	"net/http"
	"time"

	"cloud.google.com/go/firestore"
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

func handleGetProfile(w http.ResponseWriter, r *http.Request) {
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
		if status.Code(err) == codes.NotFound {
			// Auto-create minimal profile from Firebase Auth
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

func handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
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
