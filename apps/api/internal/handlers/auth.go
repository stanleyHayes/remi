package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"golang.org/x/crypto/bcrypt"

	"remi-api/internal/handlers/httpx"
	"remi-api/internal/middleware"
)

func invitationHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (h *Handler) GetInvitation(w http.ResponseWriter, r *http.Request) {
	var user bson.M
	err := h.DB.Collection("users").FindOne(r.Context(), bson.M{"invitationTokenHash": invitationHash(chi.URLParam(r, "token")), "invitationStatus": "pending", "invitationExpiresAt": bson.M{"$gt": time.Now()}}).Decode(&user)
	if err == mongo.ErrNoDocuments {
		httpx.Error(w, http.StatusGone, "this invitation is invalid, expired or already used")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusOK, bson.M{"email": user["email"], "role": user["role"], "expiresAt": user["invitationExpiresAt"]})
}

func (h *Handler) AcceptInvitation(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	name, password := strings.TrimSpace(httpx.Str(body, "name")), httpx.Str(body, "password")
	if name == "" {
		httpx.Error(w, http.StatusBadRequest, "your name is required")
		return
	}
	if len(password) < 12 {
		httpx.Error(w, http.StatusBadRequest, "password must be at least 12 characters")
		return
	}
	filter := bson.M{"invitationTokenHash": invitationHash(chi.URLParam(r, "token")), "invitationStatus": "pending", "invitationExpiresAt": bson.M{"$gt": time.Now()}}
	var user bson.M
	if err := h.DB.Collection("users").FindOne(r.Context(), filter).Decode(&user); err != nil {
		httpx.Error(w, http.StatusGone, "this invitation is invalid, expired or already used")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not secure password")
		return
	}
	now := time.Now()
	result, err := h.DB.Collection("users").UpdateOne(r.Context(), filter, bson.M{"$set": bson.M{"name": name, "passwordHash": string(hash), "invitationStatus": "accepted", "activatedAt": now, "updatedAt": now}, "$unset": bson.M{"invitationTokenHash": "", "invitationExpiresAt": ""}})
	if err != nil || result.ModifiedCount != 1 {
		httpx.Error(w, http.StatusGone, "this invitation is invalid, expired or already used")
		return
	}
	oid, _ := user["_id"].(bson.ObjectID)
	email, _ := user["email"].(string)
	role, _ := user["role"].(string)
	scope := scopeFromUser(user)
	token, err := h.JWT.GenerateScoped(oid.Hex(), email, name, role, scope.BranchIDs, scope.MinistryIDs, scope.AssignedResourceIDs, scope.AccessVersion)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not issue session")
		return
	}
	httpx.JSON(w, http.StatusOK, bson.M{"token": token, "user": bson.M{"id": oid.Hex(), "email": email, "name": name, "role": role}})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	email, password := strings.ToLower(strings.TrimSpace(httpx.Str(body, "email"))), httpx.Str(body, "password")
	if email == "" || password == "" {
		httpx.Error(w, http.StatusBadRequest, "email and password are required")
		return
	}

	var user bson.M
	err = h.DB.Collection("users").FindOne(r.Context(), bson.M{"email": email}).Decode(&user)
	if err == mongo.ErrNoDocuments {
		httpx.Error(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if status, _ := user["invitationStatus"].(string); status == "pending" {
		httpx.Error(w, http.StatusForbidden, "accept your email invitation before signing in")
		return
	}
	hash, _ := user["passwordHash"].(string)
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		httpx.Error(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	oid, _ := user["_id"].(bson.ObjectID)
	if enabled, _ := user["mfaEnabled"].(bool); enabled {
		challenge, err := h.JWT.GenerateMFAChallenge(oid.Hex())
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "could not issue MFA challenge")
			return
		}
		httpx.JSON(w, http.StatusOK, bson.M{"mfaRequired": true, "challengeToken": challenge})
		return
	}
	name, _ := user["name"].(string)
	role, _ := user["role"].(string)
	scope := scopeFromUser(user)
	token, err := h.JWT.GenerateScoped(oid.Hex(), email, name, role, scope.BranchIDs, scope.MinistryIDs, scope.AssignedResourceIDs, scope.AccessVersion)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	httpx.JSON(w, http.StatusOK, bson.M{
		"token": token,
		"user":  bson.M{"id": oid.Hex(), "email": email, "name": name, "role": role},
	})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFrom(r)
	if claims == nil {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	oid, err := bson.ObjectIDFromHex(claims.UserID)
	if err != nil {
		httpx.Error(w, http.StatusUnauthorized, "invalid session")
		return
	}
	var user bson.M
	if err := h.DB.Collection("users").FindOne(r.Context(), bson.M{"_id": oid}).Decode(&user); err != nil {
		httpx.Error(w, http.StatusUnauthorized, "user no longer exists")
		return
	}
	httpx.JSON(w, http.StatusOK, bson.M{"user": publicUser(user)})
}
