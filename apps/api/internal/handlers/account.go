package handlers

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"golang.org/x/crypto/bcrypt"
	"remi-api/internal/handlers/httpx"
	"remi-api/internal/middleware"
	"remi-api/internal/models"
)

func publicUser(user bson.M) bson.M {
	n := models.Normalize(user)
	return bson.M{"id": n["id"], "email": n["email"], "name": n["name"], "role": n["role"], "phone": n["phone"], "title": n["title"], "bio": n["bio"], "avatarUrl": n["avatarUrl"], "preferences": n["preferences"], "mfaEnabled": n["mfaEnabled"], "lastLoginAt": n["lastLoginAt"]}
}

func accountID(r *http.Request) (bson.ObjectID, bool) {
	c := middleware.ClaimsFrom(r)
	if c == nil {
		return bson.NilObjectID, false
	}
	id, err := bson.ObjectIDFromHex(c.UserID)
	return id, err == nil
}

func (h *Handler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := accountID(r)
	if !ok {
		httpx.Error(w, 401, "unauthorized")
		return
	}
	b, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, 400, "invalid JSON body")
		return
	}
	name, email := strings.TrimSpace(httpx.Str(b, "name")), strings.ToLower(strings.TrimSpace(httpx.Str(b, "email")))
	if name == "" || !strings.Contains(email, "@") {
		httpx.Error(w, 400, "name and a valid email are required")
		return
	}
	set := bson.M{"name": name, "email": email, "phone": httpx.Str(b, "phone"), "title": httpx.Str(b, "title"), "bio": httpx.Str(b, "bio"), "avatarUrl": httpx.Str(b, "avatarUrl"), "updatedAt": models.Now()}
	_, err = h.DB.Collection("users").UpdateOne(r.Context(), bson.M{"_id": id}, bson.M{"$set": set})
	if err != nil {
		httpx.Error(w, 409, "email is already in use")
		return
	}
	var u bson.M
	_ = h.DB.Collection("users").FindOne(r.Context(), bson.M{"_id": id}).Decode(&u)
	claims := middleware.ClaimsFrom(r)
	scope := scopeFromUser(u)
	if claims != nil && claims.MFAAt > 0 {
		token, _ := h.JWT.GenerateMFAAuthenticatedScoped(id.Hex(), email, name, fmt.Sprint(u["role"]), time.Unix(claims.MFAAt, 0), scope.BranchIDs, scope.MinistryIDs, scope.AssignedResourceIDs, scope.AccessVersion)
		httpx.JSON(w, 200, bson.M{"user": publicUser(u), "token": token})
		return
	}
	token, _ := h.JWT.GenerateScoped(id.Hex(), email, name, fmt.Sprint(u["role"]), scope.BranchIDs, scope.MinistryIDs, scope.AssignedResourceIDs, scope.AccessVersion)
	httpx.JSON(w, 200, bson.M{"user": publicUser(u), "token": token})
}

func (h *Handler) UpdatePassword(w http.ResponseWriter, r *http.Request) {
	id, ok := accountID(r)
	if !ok {
		httpx.Error(w, 401, "unauthorized")
		return
	}
	b, _ := httpx.Decode(r)
	current, next := httpx.Str(b, "currentPassword"), httpx.Str(b, "newPassword")
	if len(next) < 12 {
		httpx.Error(w, 400, "new password must be at least 12 characters")
		return
	}
	var u bson.M
	if h.DB.Collection("users").FindOne(r.Context(), bson.M{"_id": id}).Decode(&u) != nil || bcrypt.CompareHashAndPassword([]byte(fmt.Sprint(u["passwordHash"])), []byte(current)) != nil {
		httpx.Error(w, 400, "current password is incorrect")
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(next), bcrypt.DefaultCost)
	_, _ = h.DB.Collection("users").UpdateOne(r.Context(), bson.M{"_id": id}, bson.M{"$set": bson.M{"passwordHash": string(hash), "passwordChangedAt": models.Now()}})
	w.WriteHeader(204)
}

func (h *Handler) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	id, ok := accountID(r)
	if !ok {
		httpx.Error(w, 401, "unauthorized")
		return
	}
	b, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, 400, "invalid JSON body")
		return
	}
	prefs := bson.M{"density": httpx.Str(b, "density"), "theme": httpx.Str(b, "theme"), "emailDigest": b["emailDigest"], "reducedMotion": b["reducedMotion"], "timezone": httpx.Str(b, "timezone")}
	_, _ = h.DB.Collection("users").UpdateOne(r.Context(), bson.M{"_id": id}, bson.M{"$set": bson.M{"preferences": prefs}})
	httpx.JSON(w, 200, bson.M{"preferences": prefs})
}

func randomBase32(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return strings.TrimRight(base32.StdEncoding.EncodeToString(b), "=")
}
func totp(secret, code string, now time.Time) bool {
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	if err != nil {
		return false
	}
	for d := -1; d <= 1; d++ {
		counter := uint64(now.Unix()/30 + int64(d))
		msg := make([]byte, 8)
		binary.BigEndian.PutUint64(msg, counter)
		mac := hmac.New(sha1.New, raw)
		mac.Write(msg)
		sum := mac.Sum(nil)
		off := sum[len(sum)-1] & 15
		num := (uint32(sum[off])&127)<<24 | (uint32(sum[off+1])&255)<<16 | (uint32(sum[off+2])&255)<<8 | (uint32(sum[off+3]) & 255)
		if fmt.Sprintf("%06d", num%1000000) == code {
			return true
		}
	}
	return false
}
func hashCode(s string) string {
	x := sha256.Sum256([]byte(strings.ToUpper(strings.TrimSpace(s))))
	return hex.EncodeToString(x[:])
}

func (h *Handler) StartMFA(w http.ResponseWriter, r *http.Request) {
	id, ok := accountID(r)
	if !ok {
		httpx.Error(w, 401, "unauthorized")
		return
	}
	secret := randomBase32(20)
	_, _ = h.DB.Collection("users").UpdateOne(r.Context(), bson.M{"_id": id}, bson.M{"$set": bson.M{"mfaPendingSecret": secret}})
	c := middleware.ClaimsFrom(r)
	uri := "otpauth://totp/" + url.PathEscape("REMI:"+c.Email) + "?secret=" + secret + "&issuer=REMI&period=30&digits=6"
	httpx.JSON(w, 200, bson.M{"secret": secret, "otpauthUrl": uri})
}
func (h *Handler) ConfirmMFA(w http.ResponseWriter, r *http.Request) {
	id, ok := accountID(r)
	if !ok {
		httpx.Error(w, 401, "unauthorized")
		return
	}
	b, _ := httpx.Decode(r)
	var u bson.M
	_ = h.DB.Collection("users").FindOne(r.Context(), bson.M{"_id": id}).Decode(&u)
	secret := fmt.Sprint(u["mfaPendingSecret"])
	if !totp(secret, httpx.Str(b, "code"), time.Now()) {
		httpx.Error(w, 400, "verification code is invalid")
		return
	}
	plain := make([]string, 8)
	hashed := make([]string, 8)
	for i := range plain {
		plain[i] = strings.ToUpper(randomBase32(5))
		hashed[i] = hashCode(plain[i])
	}
	_, _ = h.DB.Collection("users").UpdateOne(r.Context(), bson.M{"_id": id}, bson.M{"$set": bson.M{"mfaEnabled": true, "mfaSecret": secret, "mfaRecoveryCodes": hashed}, "$unset": bson.M{"mfaPendingSecret": ""}})
	httpx.JSON(w, 200, bson.M{"recoveryCodes": plain})
}
func (h *Handler) DisableMFA(w http.ResponseWriter, r *http.Request) {
	id, ok := accountID(r)
	if !ok {
		httpx.Error(w, 401, "unauthorized")
		return
	}
	b, _ := httpx.Decode(r)
	var u bson.M
	_ = h.DB.Collection("users").FindOne(r.Context(), bson.M{"_id": id}).Decode(&u)
	if bcrypt.CompareHashAndPassword([]byte(fmt.Sprint(u["passwordHash"])), []byte(httpx.Str(b, "password"))) != nil {
		httpx.Error(w, 400, "password is incorrect")
		return
	}
	_, _ = h.DB.Collection("users").UpdateOne(r.Context(), bson.M{"_id": id}, bson.M{"$set": bson.M{"mfaEnabled": false}, "$unset": bson.M{"mfaSecret": "", "mfaRecoveryCodes": "", "mfaPendingSecret": ""}})
	w.WriteHeader(204)
}
func (h *Handler) VerifyMFA(w http.ResponseWriter, r *http.Request) {
	b, _ := httpx.Decode(r)
	claims, err := h.JWT.ValidateMFAChallenge(httpx.Str(b, "challengeToken"))
	if err != nil {
		httpx.Error(w, 401, "MFA challenge expired")
		return
	}
	id, err := bson.ObjectIDFromHex(claims.UserID)
	if err != nil {
		httpx.Error(w, 401, "invalid challenge")
		return
	}
	var u bson.M
	if h.DB.Collection("users").FindOne(r.Context(), bson.M{"_id": id}).Decode(&u) != nil {
		httpx.Error(w, 401, "invalid challenge")
		return
	}
	code := strings.TrimSpace(httpx.Str(b, "code"))
	valid := totp(fmt.Sprint(u["mfaSecret"]), code, time.Now())
	if !valid {
		if arr, ok := u["mfaRecoveryCodes"].(bson.A); ok {
			for _, v := range arr {
				if fmt.Sprint(v) == hashCode(code) {
					valid = true
					_, _ = h.DB.Collection("users").UpdateOne(r.Context(), bson.M{"_id": id}, bson.M{"$pull": bson.M{"mfaRecoveryCodes": v}})
					break
				}
			}
		}
	}
	if !valid {
		httpx.Error(w, 401, "verification code is invalid")
		return
	}
	email, name, role := fmt.Sprint(u["email"]), fmt.Sprint(u["name"]), fmt.Sprint(u["role"])
	scope := scopeFromUser(u)
	token, _ := h.JWT.GenerateMFAAuthenticatedScoped(id.Hex(), email, name, role, time.Now().UTC(), scope.BranchIDs, scope.MinistryIDs, scope.AssignedResourceIDs, scope.AccessVersion)
	_, _ = h.DB.Collection("users").UpdateOne(r.Context(), bson.M{"_id": id}, bson.M{"$set": bson.M{"lastLoginAt": models.Now()}})
	httpx.JSON(w, 200, bson.M{"token": token, "user": publicUser(u)})
}

var _ = strconv.Itoa
