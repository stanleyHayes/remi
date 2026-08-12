package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"remi-api/internal/handlers/httpx"
	"remi-api/internal/middleware"
)

const (
	memberAccounts   = "chms_member_accounts"
	memberChallenges = "chms_member_auth_challenges"
	memberSessions   = "chms_member_sessions"
)

func (h *Handler) memberHash(value string) string {
	sum := sha256.Sum256([]byte(value + ":" + h.Cfg.JWTSecret + ":member-auth:v1"))
	return hex.EncodeToString(sum[:])
}

func memberName(names struct {
	Given     string `bson:"given"`
	Family    string `bson:"family"`
	Preferred string `bson:"preferred"`
}) string {
	first := strings.TrimSpace(names.Preferred)
	if first == "" {
		first = strings.TrimSpace(names.Given)
	}
	return strings.TrimSpace(first + " " + strings.TrimSpace(names.Family))
}

func memberEmail(points []struct {
	Type       string `bson:"type"`
	Value      string `bson:"value"`
	Normalized string `bson:"normalized"`
	Primary    bool   `bson:"primary"`
}) string {
	var fallback string
	for _, point := range points {
		if strings.ToLower(point.Type) != "email" {
			continue
		}
		value := strings.ToLower(strings.TrimSpace(point.Normalized))
		if value == "" {
			value = strings.ToLower(strings.TrimSpace(point.Value))
		}
		if fallback == "" {
			fallback = value
		}
		if point.Primary {
			return value
		}
	}
	return fallback
}

func memberPhone(points []struct {
	Type       string `bson:"type"`
	Value      string `bson:"value"`
	Normalized string `bson:"normalized"`
	Primary    bool   `bson:"primary"`
}) string {
	var fallback string
	for _, point := range points {
		if strings.ToLower(point.Type) != "phone" {
			continue
		}
		value := normalizeMemberIdentifier(point.Normalized)
		if value == "" {
			value = normalizeMemberIdentifier(point.Value)
		}
		if fallback == "" {
			fallback = value
		}
		if point.Primary {
			return value
		}
	}
	return fallback
}

func normalizeMemberIdentifier(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if strings.Contains(value, "@") {
		return value
	}
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, value)
	if strings.HasPrefix(digits, "0") && len(digits) == 10 {
		return "+233" + digits[1:]
	}
	if strings.HasPrefix(digits, "233") && len(digits) == 12 {
		return "+" + digits
	}
	if strings.HasPrefix(value, "+") && len(digits) >= 10 && len(digits) <= 15 {
		return "+" + digits
	}
	return ""
}

func (h *Handler) seededMemberDemo(account bson.M) bool {
	environment := strings.ToLower(strings.TrimSpace(h.Cfg.Environment))
	if environment == "production" || environment == "prod" {
		return false
	}
	demo, _ := account["demoAccount"].(bool)
	return demo
}

func (h *Handler) AdminInviteMember(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	personID := strings.TrimSpace(httpx.Str(body, "personId"))
	if personID == "" {
		httpx.Error(w, http.StatusBadRequest, "personId is required")
		return
	}
	var person struct {
		ID             string `bson:"_id"`
		OrganizationID string `bson:"organizationId"`
		Names          struct {
			Given     string `bson:"given"`
			Family    string `bson:"family"`
			Preferred string `bson:"preferred"`
		} `bson:"names"`
		ContactPoints []struct {
			Type       string `bson:"type"`
			Value      string `bson:"value"`
			Normalized string `bson:"normalized"`
			Primary    bool   `bson:"primary"`
		} `bson:"contactPoints"`
	}
	if err := h.DB.Collection("chms_people").FindOne(r.Context(), bson.M{"_id": personID, "organizationId": h.Cfg.CHMSOrganizationID, "archivedAt": bson.M{"$exists": false}}).Decode(&person); err != nil {
		if err == mongo.ErrNoDocuments {
			httpx.Error(w, http.StatusNotFound, "person not found")
		} else {
			httpx.Error(w, http.StatusInternalServerError, "database error")
		}
		return
	}
	email := memberEmail(person.ContactPoints)
	if email == "" || !strings.Contains(email, "@") {
		httpx.Error(w, http.StatusConflict, "the person needs a valid email before they can be invited")
		return
	}
	token := randHex(32)
	now, expires := time.Now().UTC(), time.Now().UTC().Add(72*time.Hour)
	phone := memberPhone(person.ContactPoints)
	update := bson.M{
		"$set":         bson.M{"name": memberName(person.Names), "email": email, "emailNormalized": email, "phoneNormalized": phone, "status": "invited", "invitationTokenHash": h.memberHash(token), "invitationExpiresAt": expires, "invitedAt": now, "updatedAt": now},
		"$setOnInsert": bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "personId": personID, "createdAt": now},
	}
	result, err := h.DB.Collection(memberAccounts).UpdateOne(r.Context(), bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "personId": personID, "status": bson.M{"$ne": "active"}}, update, options.UpdateOne().SetUpsert(true))
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			httpx.Error(w, http.StatusConflict, "this email is already linked to another member account")
		} else {
			httpx.Error(w, http.StatusInternalServerError, "could not create member invitation")
		}
		return
	}
	if result.MatchedCount == 0 && result.UpsertedID == nil {
		httpx.Error(w, http.StatusConflict, "this member account is already active")
		return
	}
	inviteURL := h.Cfg.MemberAppURL + "/invite/" + token
	if err := h.Email.Send(email, "Your invitation to My REMI", fmt.Sprintf(`<h2>Welcome to My REMI</h2><p>Your personal church workspace is ready.</p><p><a href="%s">Accept your invitation</a></p><p>This private link expires in 72 hours and can only be used once.</p>`, inviteURL)); err != nil {
		httpx.Error(w, http.StatusBadGateway, "invitation created but email delivery failed")
		return
	}
	response := bson.M{"personId": personID, "email": email, "status": "invited", "expiresAt": expires}
	if h.Cfg.ResendAPIKey == "" {
		response["demoInvitationUrl"] = inviteURL
	}
	httpx.JSON(w, http.StatusCreated, response)
}

func (h *Handler) GetMemberInvitation(w http.ResponseWriter, r *http.Request) {
	var account bson.M
	err := h.DB.Collection(memberAccounts).FindOne(r.Context(), bson.M{"invitationTokenHash": h.memberHash(chi.URLParam(r, "token")), "status": "invited", "invitationExpiresAt": bson.M{"$gt": time.Now().UTC()}}).Decode(&account)
	if err != nil {
		httpx.Error(w, http.StatusGone, "this invitation is invalid, expired or already used")
		return
	}
	httpx.JSON(w, http.StatusOK, bson.M{"name": account["name"], "email": account["email"], "expiresAt": account["invitationExpiresAt"]})
}

func (h *Handler) RedeemMemberInvitation(w http.ResponseWriter, r *http.Request) {
	var account bson.M
	filter := bson.M{"invitationTokenHash": h.memberHash(chi.URLParam(r, "token")), "status": "invited", "invitationExpiresAt": bson.M{"$gt": time.Now().UTC()}}
	if err := h.DB.Collection(memberAccounts).FindOne(r.Context(), filter).Decode(&account); err != nil {
		httpx.Error(w, http.StatusGone, "this invitation is invalid, expired or already used")
		return
	}
	now := time.Now().UTC()
	result, err := h.DB.Collection(memberAccounts).UpdateOne(r.Context(), filter, bson.M{"$set": bson.M{"status": "active", "emailVerifiedAt": now, "activatedAt": now, "updatedAt": now}, "$unset": bson.M{"invitationTokenHash": "", "invitationExpiresAt": ""}})
	if err != nil || result.ModifiedCount != 1 {
		httpx.Error(w, http.StatusGone, "this invitation is invalid, expired or already used")
		return
	}
	h.issueMemberSession(w, r, account, "Invitation link")
}

func (h *Handler) RequestMemberOTP(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	identifier := normalizeMemberIdentifier(httpx.Str(body, "identifier"))
	generic := bson.M{"message": "If an active account matches, a sign-in code has been sent."}
	if identifier == "" {
		httpx.JSON(w, http.StatusAccepted, generic)
		return
	}
	lookup, channel := bson.M{"status": "active"}, "email"
	if strings.Contains(identifier, "@") {
		lookup["emailNormalized"] = identifier
	} else {
		lookup["phoneNormalized"], channel = identifier, "sms"
	}
	var account bson.M
	if err := h.DB.Collection(memberAccounts).FindOne(r.Context(), lookup).Decode(&account); err != nil {
		httpx.JSON(w, http.StatusAccepted, generic)
		return
	}
	code := strings.TrimSpace(h.Cfg.SeedMemberOTP)
	if !h.seededMemberDemo(account) || len(code) != 6 {
		code, err = secureDigits(6)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "could not create sign-in code")
			return
		}
	}
	now := time.Now().UTC()
	challenge := bson.M{"accountId": account["_id"], "channel": channel, "purpose": "sign-in", "codeHash": h.memberHash(code), "attempts": int32(0), "createdAt": now, "expiresAt": now.Add(10 * time.Minute)}
	inserted, err := h.DB.Collection(memberChallenges).InsertOne(r.Context(), challenge)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not create sign-in code")
		return
	}
	var deliveryErr error
	demo := h.seededMemberDemo(account)
	if !demo {
		if channel == "sms" {
			deliveryErr = h.SMS.Send(identifier, fmt.Sprintf("%s is your My REMI sign-in code. It expires in 10 minutes. Do not share it.", code))
		} else {
			deliveryErr = h.Email.Send(fmt.Sprint(account["email"]), "Your My REMI sign-in code", fmt.Sprintf(`<h2>%s is your sign-in code</h2><p>It expires in 10 minutes and can be used once.</p><p>If you did not request it, you can ignore this message.</p>`, code))
		}
	}
	if deliveryErr != nil {
		_, _ = h.DB.Collection(memberChallenges).DeleteOne(r.Context(), bson.M{"_id": inserted.InsertedID})
		httpx.Error(w, http.StatusBadGateway, "could not deliver sign-in code")
		return
	}
	generic["challengeId"] = inserted.InsertedID
	if demo || (channel == "email" && h.Cfg.ResendAPIKey == "") || (channel == "sms" && h.Cfg.ArkeselAPIKey == "") {
		generic["demoCode"] = code
	}
	httpx.JSON(w, http.StatusAccepted, generic)
}

func (h *Handler) VerifyMemberOTP(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	id, ok := httpx.ObjectID(strings.TrimSpace(httpx.Str(body, "challengeId")))
	code := strings.TrimSpace(httpx.Str(body, "code"))
	if !ok || len(code) != 6 {
		httpx.Error(w, http.StatusBadRequest, "challengeId and six-digit code are required")
		return
	}
	var challenge bson.M
	if err := h.DB.Collection(memberChallenges).FindOne(r.Context(), bson.M{"_id": id, "expiresAt": bson.M{"$gt": time.Now().UTC()}, "usedAt": bson.M{"$exists": false}, "attempts": bson.M{"$lt": 5}}).Decode(&challenge); err != nil {
		httpx.Error(w, http.StatusUnauthorized, "the sign-in code is invalid or expired")
		return
	}
	want, got := fmt.Sprint(challenge["codeHash"]), h.memberHash(code)
	if subtle.ConstantTimeCompare([]byte(want), []byte(got)) != 1 {
		_, _ = h.DB.Collection(memberChallenges).UpdateOne(r.Context(), bson.M{"_id": id}, bson.M{"$inc": bson.M{"attempts": 1}})
		httpx.Error(w, http.StatusUnauthorized, "the sign-in code is invalid or expired")
		return
	}
	result, err := h.DB.Collection(memberChallenges).UpdateOne(r.Context(), bson.M{"_id": id, "usedAt": bson.M{"$exists": false}}, bson.M{"$set": bson.M{"usedAt": time.Now().UTC()}})
	if err != nil || result.ModifiedCount != 1 {
		httpx.Error(w, http.StatusUnauthorized, "the sign-in code is invalid or expired")
		return
	}
	var account bson.M
	if err := h.DB.Collection(memberAccounts).FindOne(r.Context(), bson.M{"_id": challenge["accountId"], "status": "active"}).Decode(&account); err != nil {
		httpx.Error(w, http.StatusUnauthorized, "the member account is unavailable")
		return
	}
	if enabled, _ := account["mfaEnabled"].(bool); enabled {
		primary := fmt.Sprint(challenge["channel"])
		secondary := "email"
		if primary == "email" {
			secondary = "sms"
		}
		if h.startMemberMFAChallenge(w, r, account, secondary, "mfa-sign-in", strings.TrimSpace(httpx.Str(body, "deviceName"))) {
			return
		}
		return
	}
	h.issueMemberSession(w, r, account, strings.TrimSpace(httpx.Str(body, "deviceName")))
}

func (h *Handler) issueMemberSession(w http.ResponseWriter, r *http.Request, account bson.M, deviceName string) {
	accountID, ok := account["_id"].(bson.ObjectID)
	if !ok {
		httpx.Error(w, http.StatusInternalServerError, "invalid member account")
		return
	}
	if deviceName == "" {
		deviceName = "Web browser"
	}
	if len(deviceName) > 80 {
		deviceName = deviceName[:80]
	}
	refresh := randHex(32)
	now := time.Now().UTC()
	session := bson.M{"accountId": accountID, "personId": account["personId"], "organizationId": account["organizationId"], "refreshTokenHash": h.memberHash(refresh), "deviceName": deviceName, "createdAt": now, "lastSeenAt": now, "expiresAt": now.Add(30 * 24 * time.Hour)}
	inserted, err := h.DB.Collection(memberSessions).InsertOne(r.Context(), session)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not create member session")
		return
	}
	sessionID := inserted.InsertedID.(bson.ObjectID).Hex()
	access, err := h.JWT.GenerateMember(accountID.Hex(), fmt.Sprint(account["personId"]), sessionID, "", fmt.Sprint(account["email"]), fmt.Sprint(account["name"]))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not create access token")
		return
	}
	httpx.JSON(w, http.StatusOK, bson.M{"accessToken": access, "refreshToken": refresh, "expiresIn": 900, "member": memberAccountResponse(account)})
}

func memberAccountResponse(account bson.M) bson.M {
	enabled, _ := account["mfaEnabled"].(bool)
	return bson.M{"id": account["_id"], "personId": account["personId"], "name": account["name"], "email": account["email"], "status": account["status"], "mfaEnabled": enabled}
}

func (h *Handler) RefreshMemberSession(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	refresh := strings.TrimSpace(httpx.Str(body, "refreshToken"))
	var session bson.M
	filter := bson.M{"refreshTokenHash": h.memberHash(refresh), "expiresAt": bson.M{"$gt": time.Now().UTC()}, "revokedAt": bson.M{"$exists": false}}
	if refresh == "" || h.DB.Collection(memberSessions).FindOne(r.Context(), filter).Decode(&session) != nil {
		httpx.Error(w, http.StatusUnauthorized, "invalid or expired member session")
		return
	}
	var account bson.M
	if err := h.DB.Collection(memberAccounts).FindOne(r.Context(), bson.M{"_id": session["accountId"], "status": "active"}).Decode(&account); err != nil {
		httpx.Error(w, http.StatusUnauthorized, "invalid or expired member session")
		return
	}
	rotated := randHex(32)
	now := time.Now().UTC()
	result, err := h.DB.Collection(memberSessions).UpdateOne(r.Context(), filter, bson.M{"$set": bson.M{"refreshTokenHash": h.memberHash(rotated), "lastSeenAt": now}})
	if err != nil || result.ModifiedCount != 1 {
		httpx.Error(w, http.StatusUnauthorized, "invalid or expired member session")
		return
	}
	sessionID := session["_id"].(bson.ObjectID).Hex()
	access, err := h.JWT.GenerateMember(account["_id"].(bson.ObjectID).Hex(), fmt.Sprint(account["personId"]), sessionID, fmt.Sprint(session["activeHouseholdId"]), fmt.Sprint(account["email"]), fmt.Sprint(account["name"]))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not refresh member session")
		return
	}
	httpx.JSON(w, http.StatusOK, bson.M{"accessToken": access, "refreshToken": rotated, "expiresIn": 900})
}

func (h *Handler) LogoutMemberSession(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	refresh := strings.TrimSpace(httpx.Str(body, "refreshToken"))
	if refresh != "" {
		_, _ = h.DB.Collection(memberSessions).UpdateOne(r.Context(), bson.M{"refreshTokenHash": h.memberHash(refresh), "revokedAt": bson.M{"$exists": false}}, bson.M{"$set": bson.M{"revokedAt": time.Now().UTC()}})
	}
	w.WriteHeader(http.StatusNoContent)
}

func requireMember(r *http.Request) bool {
	claims := middleware.ClaimsFrom(r)
	return claims != nil && claims.Role == "member" && claims.PersonID != "" && claims.SessionID != ""
}

func (h *Handler) MemberMe(w http.ResponseWriter, r *http.Request) {
	if !requireMember(r) {
		httpx.Error(w, http.StatusForbidden, "member access required")
		return
	}
	claims := middleware.ClaimsFrom(r)
	id, err := bson.ObjectIDFromHex(claims.UserID)
	if err != nil {
		httpx.Error(w, http.StatusUnauthorized, "invalid member session")
		return
	}
	var account bson.M
	if h.DB.Collection(memberAccounts).FindOne(r.Context(), bson.M{"_id": id, "status": "active"}).Decode(&account) != nil {
		httpx.Error(w, http.StatusUnauthorized, "invalid member session")
		return
	}
	response := memberAccountResponse(account)
	response["activeHouseholdId"] = claims.HouseholdID
	httpx.JSON(w, http.StatusOK, response)
}

func (h *Handler) ListMemberSessions(w http.ResponseWriter, r *http.Request) {
	if !requireMember(r) {
		httpx.Error(w, http.StatusForbidden, "member access required")
		return
	}
	claims := middleware.ClaimsFrom(r)
	accountID, _ := bson.ObjectIDFromHex(claims.UserID)
	cursor, err := h.DB.Collection(memberSessions).Find(r.Context(), bson.M{"accountId": accountID, "revokedAt": bson.M{"$exists": false}, "expiresAt": bson.M{"$gt": time.Now().UTC()}}, options.Find().SetSort(bson.D{{Key: "lastSeenAt", Value: -1}}).SetProjection(bson.M{"refreshTokenHash": 0}))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	defer cursor.Close(r.Context())
	var sessions []bson.M
	if err := cursor.All(r.Context(), &sessions); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	for _, session := range sessions {
		if id, ok := session["_id"].(bson.ObjectID); ok {
			session["current"] = id.Hex() == claims.SessionID
		}
	}
	httpx.JSON(w, http.StatusOK, sessions)
}

func (h *Handler) ListMemberHouseholds(w http.ResponseWriter, r *http.Request) {
	if !requireMember(r) {
		httpx.Error(w, http.StatusForbidden, "member access required")
		return
	}
	claims := middleware.ClaimsFrom(r)
	cursor, err := h.DB.Collection("chms_household_memberships").Find(r.Context(), bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "personId": claims.PersonID, "endedAt": bson.M{"$exists": false}}, options.Find().SetProjection(bson.M{"householdId": 1, "role": 1}))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	defer cursor.Close(r.Context())
	var memberships []bson.M
	if err := cursor.All(r.Context(), &memberships); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	items := make([]bson.M, 0, len(memberships))
	for _, membership := range memberships {
		householdID := fmt.Sprint(membership["householdId"])
		var household bson.M
		if err := h.DB.Collection("chms_households").FindOne(r.Context(), bson.M{"_id": householdID, "organizationId": h.Cfg.CHMSOrganizationID}, options.FindOne().SetProjection(bson.M{"name": 1})).Decode(&household); err != nil {
			continue
		}
		items = append(items, bson.M{"id": householdID, "name": household["name"], "role": membership["role"], "current": householdID == claims.HouseholdID})
	}
	httpx.JSON(w, http.StatusOK, items)
}

func (h *Handler) RevokeMemberSession(w http.ResponseWriter, r *http.Request) {
	if !requireMember(r) {
		httpx.Error(w, http.StatusForbidden, "member access required")
		return
	}
	claims := middleware.ClaimsFrom(r)
	accountID, _ := bson.ObjectIDFromHex(claims.UserID)
	sessionID, err := bson.ObjectIDFromHex(chi.URLParam(r, "sessionId"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "session not found")
		return
	}
	result, err := h.DB.Collection(memberSessions).UpdateOne(r.Context(), bson.M{"_id": sessionID, "accountId": accountID, "revokedAt": bson.M{"$exists": false}}, bson.M{"$set": bson.M{"revokedAt": time.Now().UTC()}})
	if err != nil || result.ModifiedCount != 1 {
		httpx.Error(w, http.StatusNotFound, "session not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) SwitchMemberHousehold(w http.ResponseWriter, r *http.Request) {
	if !requireMember(r) {
		httpx.Error(w, http.StatusForbidden, "member access required")
		return
	}
	body, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	householdID := strings.TrimSpace(httpx.Str(body, "householdId"))
	claims := middleware.ClaimsFrom(r)
	if householdID != "" {
		membershipFilter := bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "personId": claims.PersonID, "householdId": householdID, "endedAt": bson.M{"$exists": false}}
		if err := h.DB.Collection("chms_household_memberships").FindOne(r.Context(), membershipFilter).Err(); err != nil {
			httpx.Error(w, http.StatusNotFound, "household not found")
			return
		}
	}
	sessionID, _ := bson.ObjectIDFromHex(claims.SessionID)
	accountID, _ := bson.ObjectIDFromHex(claims.UserID)
	if _, err := h.DB.Collection(memberSessions).UpdateOne(r.Context(), bson.M{"_id": sessionID, "accountId": accountID, "revokedAt": bson.M{"$exists": false}}, bson.M{"$set": bson.M{"activeHouseholdId": householdID, "lastSeenAt": time.Now().UTC()}}); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not switch household")
		return
	}
	access, err := h.JWT.GenerateMember(claims.UserID, claims.PersonID, claims.SessionID, householdID, claims.Email, claims.Name)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not switch household")
		return
	}
	httpx.JSON(w, http.StatusOK, bson.M{"accessToken": access, "expiresIn": 900, "activeHouseholdId": householdID})
}

func secureDigits(length int) (string, error) {
	var b strings.Builder
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		b.WriteByte(byte('0' + n.Int64()))
	}
	return b.String(), nil
}
