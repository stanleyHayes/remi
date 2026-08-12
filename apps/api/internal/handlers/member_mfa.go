package handlers

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"remi-api/internal/chms/platform"
	"remi-api/internal/handlers/httpx"
)

func memberChannel(account bson.M, channel string) string {
	if channel == "sms" {
		return normalizeMemberIdentifier(fmt.Sprint(account["phoneNormalized"]))
	}
	return normalizeMemberIdentifier(fmt.Sprint(account["emailNormalized"]))
}

func maskMemberDestination(value, channel string) string {
	if channel == "email" {
		parts := strings.Split(value, "@")
		if len(parts) == 2 && len(parts[0]) > 1 {
			return parts[0][:1] + "•••@" + parts[1]
		}
	}
	if len(value) > 4 {
		return "••••" + value[len(value)-4:]
	}
	return "your verified contact"
}

func (h *Handler) startMemberMFAChallenge(w http.ResponseWriter, r *http.Request, account bson.M, channel, purpose, deviceName string) bool {
	destination := memberChannel(account, channel)
	if destination == "" {
		httpx.Error(w, http.StatusConflict, "a second verified contact method is required")
		return false
	}
	code := strings.TrimSpace(h.Cfg.SeedMemberOTP)
	var err error
	if !h.seededMemberDemo(account) || len(code) != 6 {
		code, err = secureDigits(6)
	}
	if err != nil {
		httpx.Error(w, 500, "could not create verification code")
		return false
	}
	now := time.Now().UTC()
	challenge := bson.M{"accountId": account["_id"], "channel": channel, "purpose": purpose, "codeHash": h.memberHash(code), "attempts": int32(0), "deviceName": deviceName, "createdAt": now, "expiresAt": now.Add(10 * time.Minute)}
	inserted, err := h.DB.Collection(memberChallenges).InsertOne(r.Context(), challenge)
	if err != nil {
		httpx.Error(w, 500, "could not create verification code")
		return false
	}
	var deliveryErr error
	if !h.seededMemberDemo(account) {
		if channel == "sms" {
			deliveryErr = h.SMS.Send(destination, fmt.Sprintf("%s is your My REMI security code. It expires in 10 minutes.", code))
		} else {
			deliveryErr = h.Email.Send(destination, "Your My REMI security code", fmt.Sprintf("<h2>%s is your security code</h2><p>It expires in 10 minutes and can be used once.</p>", code))
		}
	}
	if deliveryErr != nil {
		_, _ = h.DB.Collection(memberChallenges).DeleteOne(r.Context(), bson.M{"_id": inserted.InsertedID})
		httpx.Error(w, 502, "could not deliver verification code")
		return false
	}
	response := bson.M{"challengeId": inserted.InsertedID, "channel": channel, "destination": maskMemberDestination(destination, channel), "expiresIn": 600}
	if purpose == "mfa-sign-in" {
		response["mfaRequired"] = true
	}
	if h.seededMemberDemo(account) || (channel == "email" && h.Cfg.ResendAPIKey == "") || (channel == "sms" && h.Cfg.ArkeselAPIKey == "") {
		response["demoCode"] = code
	}
	httpx.JSON(w, http.StatusAccepted, response)
	return true
}

func (h *Handler) consumeMemberChallenge(ctx context.Context, idText, code, purpose string) (bson.M, bson.M, bool) {
	id, err := bson.ObjectIDFromHex(strings.TrimSpace(idText))
	if err != nil || len(strings.TrimSpace(code)) != 6 {
		return nil, nil, false
	}
	filter := bson.M{"_id": id, "purpose": purpose, "expiresAt": bson.M{"$gt": time.Now().UTC()}, "usedAt": bson.M{"$exists": false}, "attempts": bson.M{"$lt": 5}}
	var challenge bson.M
	if h.DB.Collection(memberChallenges).FindOne(ctx, filter).Decode(&challenge) != nil {
		return nil, nil, false
	}
	if subtle.ConstantTimeCompare([]byte(fmt.Sprint(challenge["codeHash"])), []byte(h.memberHash(strings.TrimSpace(code)))) != 1 {
		_, _ = h.DB.Collection(memberChallenges).UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$inc": bson.M{"attempts": 1}})
		return nil, nil, false
	}
	result, err := h.DB.Collection(memberChallenges).UpdateOne(ctx, filter, bson.M{"$set": bson.M{"usedAt": time.Now().UTC()}})
	if err != nil || result.ModifiedCount != 1 {
		return nil, nil, false
	}
	var account bson.M
	if h.DB.Collection(memberAccounts).FindOne(ctx, bson.M{"_id": challenge["accountId"], "status": "active"}).Decode(&account) != nil {
		return nil, nil, false
	}
	return account, challenge, true
}

func (h *Handler) VerifyMemberMFA(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, 400, "invalid JSON body")
		return
	}
	account, challenge, ok := h.consumeMemberChallenge(r.Context(), httpx.Str(body, "challengeId"), httpx.Str(body, "code"), "mfa-sign-in")
	if !ok {
		httpx.Error(w, 401, "the security code is invalid or expired")
		return
	}
	enabled, _ := account["mfaEnabled"].(bool)
	if !enabled {
		httpx.Error(w, 401, "multi-factor verification is no longer enabled")
		return
	}
	h.issueMemberSession(w, r, account, fmt.Sprint(challenge["deviceName"]))
}

func (h *Handler) GetMemberMFA(w http.ResponseWriter, r *http.Request) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, 403, "member access required")
		return
	}
	id, err := bson.ObjectIDFromHex(claims.UserID)
	if err != nil {
		httpx.Error(w, 401, "invalid member session")
		return
	}
	var account bson.M
	if h.DB.Collection(memberAccounts).FindOne(r.Context(), bson.M{"_id": id, "status": "active"}).Decode(&account) != nil {
		httpx.Error(w, 401, "invalid member session")
		return
	}
	enabled, _ := account["mfaEnabled"].(bool)
	httpx.JSON(w, 200, bson.M{"enabled": enabled, "emailAvailable": memberChannel(account, "email") != "", "smsAvailable": memberChannel(account, "sms") != "", "email": maskMemberDestination(memberChannel(account, "email"), "email"), "sms": maskMemberDestination(memberChannel(account, "sms"), "sms"), "enabledAt": account["mfaEnabledAt"]})
}

func (h *Handler) memberAccountForRequest(w http.ResponseWriter, r *http.Request) (bson.M, bool) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, 403, "member access required")
		return nil, false
	}
	id, err := bson.ObjectIDFromHex(claims.UserID)
	if err != nil {
		httpx.Error(w, 401, "invalid member session")
		return nil, false
	}
	var account bson.M
	if h.DB.Collection(memberAccounts).FindOne(r.Context(), bson.M{"_id": id, "status": "active"}).Decode(&account) != nil {
		httpx.Error(w, 401, "invalid member session")
		return nil, false
	}
	return account, true
}

func (h *Handler) StartMemberMFA(w http.ResponseWriter, r *http.Request) {
	account, ok := h.memberAccountForRequest(w, r)
	if !ok {
		return
	}
	if memberChannel(account, "email") == "" || memberChannel(account, "sms") == "" {
		httpx.Error(w, 409, "add and verify both an email address and mobile number before enabling two-step verification")
		return
	}
	body, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, 400, "invalid JSON body")
		return
	}
	channel := strings.ToLower(strings.TrimSpace(httpx.Str(body, "channel")))
	if channel != "email" && channel != "sms" {
		httpx.Error(w, 400, "choose email or SMS verification")
		return
	}
	h.startMemberMFAChallenge(w, r, account, channel, "mfa-setup", "")
}

func (h *Handler) ConfirmMemberMFA(w http.ResponseWriter, r *http.Request) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, 403, "member access required")
		return
	}
	body, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, 400, "invalid JSON body")
		return
	}
	account, _, valid := h.consumeMemberChallenge(r.Context(), httpx.Str(body, "challengeId"), httpx.Str(body, "code"), "mfa-setup")
	if !valid || fmt.Sprint(account["personId"]) != claims.PersonID {
		httpx.Error(w, 401, "the security code is invalid or expired")
		return
	}
	now := time.Now().UTC()
	result, err := h.DB.Collection(memberAccounts).UpdateOne(r.Context(), bson.M{"_id": account["_id"], "status": "active"}, bson.M{"$set": bson.M{"mfaEnabled": true, "mfaEnabledAt": now, "updatedAt": now}})
	if err != nil || result.ModifiedCount != 1 {
		httpx.Error(w, 500, "two-step verification could not be enabled")
		return
	}
	h.appendMemberMFAAudit(r, claims.PersonID, "member.mfa.enable", []string{"mfaEnabled", "mfaEnabledAt"})
	httpx.JSON(w, 200, bson.M{"enabled": true, "enabledAt": now})
}

func (h *Handler) RequestDisableMemberMFA(w http.ResponseWriter, r *http.Request) {
	account, ok := h.memberAccountForRequest(w, r)
	if !ok {
		return
	}
	enabled, _ := account["mfaEnabled"].(bool)
	if !enabled {
		httpx.Error(w, 409, "two-step verification is not enabled")
		return
	}
	body, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, 400, "invalid JSON body")
		return
	}
	channel := strings.ToLower(strings.TrimSpace(httpx.Str(body, "channel")))
	if channel != "email" && channel != "sms" {
		httpx.Error(w, 400, "choose email or SMS verification")
		return
	}
	h.startMemberMFAChallenge(w, r, account, channel, "mfa-disable", "")
}

func (h *Handler) DisableMemberMFA(w http.ResponseWriter, r *http.Request) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, 403, "member access required")
		return
	}
	body, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, 400, "invalid JSON body")
		return
	}
	account, _, valid := h.consumeMemberChallenge(r.Context(), httpx.Str(body, "challengeId"), httpx.Str(body, "code"), "mfa-disable")
	if !valid || fmt.Sprint(account["personId"]) != claims.PersonID {
		httpx.Error(w, 401, "the security code is invalid or expired")
		return
	}
	now := time.Now().UTC()
	result, err := h.DB.Collection(memberAccounts).UpdateOne(r.Context(), bson.M{"_id": account["_id"], "status": "active", "mfaEnabled": true}, bson.M{"$set": bson.M{"mfaEnabled": false, "mfaDisabledAt": now, "updatedAt": now}, "$unset": bson.M{"mfaEnabledAt": ""}})
	if err != nil || result.ModifiedCount != 1 {
		httpx.Error(w, 500, "two-step verification could not be disabled")
		return
	}
	h.appendMemberMFAAudit(r, claims.PersonID, "member.mfa.disable", []string{"mfaEnabled", "mfaDisabledAt"})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) appendMemberMFAAudit(r *http.Request, personID, action string, fields []string) {
	store, err := platform.NewMongoPlatformStore(h.DB)
	if err != nil {
		return
	}
	_ = store.AppendAudit(r.Context(), platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: platform.ID(h.Cfg.CHMSOrganizationID), Actor: platform.Actor{Type: platform.ActorMember, ID: platform.ID(personID)}, Action: action, ResourceType: "member-account", ResourceID: platform.ID(personID), SubjectIDs: []platform.ID{platform.ID(personID)}, ChangedFields: fields, Outcome: "success", RequestID: platform.RequestIDFrom(r.Context()), OccurredAt: time.Now().UTC()})
}
