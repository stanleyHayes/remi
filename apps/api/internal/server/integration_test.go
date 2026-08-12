// Integration tests for the REMI API. They run the real router against a
// dedicated remi_test database on a real MongoDB (dropped after the run).
//
// Without MongoDB they skip; with REQUIRE_INFRA=1 they fail instead, so CI
// cannot go green having run nothing. Locally: `make up` first.
package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"golang.org/x/crypto/bcrypt"

	"remi-api/internal/chms/community"
	"remi-api/internal/chms/consent"
	chmshttp "remi-api/internal/chms/httpapi"
	"remi-api/internal/chms/platform"
	"remi-api/internal/config"
	"remi-api/internal/handlers"
	"remi-api/internal/models"
	"remi-api/internal/server"
	"remi-api/internal/services"
	"remi-api/internal/testsupport"
)

const testDBName = "remi_test"

var (
	connOnce  sync.Once
	connErr   error
	testMongo *mongo.Client
	testDB    *mongo.Database
)

func TestMain(m *testing.M) {
	code := m.Run()
	if testMongo != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = testDB.Drop(ctx)
		_ = testMongo.Disconnect(ctx)
	}
	os.Exit(code)
}

// setup connects to MongoDB (once per run) and returns the test database and
// a server running the real route table against it.
func setup(t *testing.T) (*mongo.Database, *httptest.Server) {
	t.Helper()
	connOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		client, err := mongo.Connect(options.Client().ApplyURI(testsupport.MongoURI()))
		if err != nil {
			connErr = err
			return
		}
		if err := client.Ping(ctx, nil); err != nil {
			connErr = fmt.Errorf("ping mongo: %w", err)
			return
		}
		testMongo = client
		testDB = client.Database(testDBName)
	})
	if connErr != nil {
		testsupport.SkipOrFail(t, "MongoDB", connErr)
	}

	cfg := &config.Config{
		Port:               "0",
		JWTSecret:          "integration-test-secret",
		CORSOrigins:        []string{"http://localhost:3010"},
		EmailFrom:          "REMI Test <test@remi.church>",
		NotifyEmail:        "pastor@remi.church",
		AdminAppURL:        "http://localhost:3011",
		MemberAppURL:       "http://localhost:3012",
		CHMSOrganizationID: "remi",
		SeedMemberOTP:      "260811",
		// No Paystack key: giving runs in demo mode.
	}
	h := handlers.New(testDB, cfg)
	srv := httptest.NewServer(server.NewRouter(h, cfg.CORSOrigins))
	t.Cleanup(srv.Close)
	return testDB, srv
}

func TestMemberInvitationPasswordlessSessionLifecycle(t *testing.T) {
	database, srv := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, collection := range []string{"chms_people", "chms_member_accounts", "chms_member_auth_challenges", "chms_member_sessions"} {
		_, _ = database.Collection(collection).DeleteMany(ctx, bson.M{})
	}
	personID := "member-person-1"
	_, err := database.Collection("chms_people").InsertOne(ctx, bson.M{
		"_id": personID, "organizationId": "remi", "names": bson.M{"given": "Ama", "family": "Mensah", "preferred": "Ama"},
		"contactPoints": bson.A{bson.M{"type": "email", "value": "ama.member@test.remi", "normalized": "ama.member@test.remi", "primary": true}, bson.M{"type": "phone", "value": "0244000000", "normalized": "+233244000000", "primary": true}},
	})
	if err != nil {
		t.Fatalf("seed member person: %v", err)
	}
	adminToken, err := handlers.New(database, &config.Config{JWTSecret: "integration-test-secret"}).JWT.Generate("admin", "admin@test.remi", "Admin", "editor")
	if err != nil {
		t.Fatalf("admin token: %v", err)
	}

	status, invitation := do(t, http.MethodPost, srv.URL+"/api/admin/member-invitations", adminToken, bson.M{"personId": personID})
	if status != http.StatusCreated {
		t.Fatalf("invite member: got %d (%v)", status, invitation)
	}
	inviteURL := fmt.Sprint(invitation["demoInvitationUrl"])
	parts := strings.Split(inviteURL, "/invite/")
	if len(parts) != 2 || parts[1] == "" {
		t.Fatalf("missing invitation URL: %v", invitation)
	}
	inviteToken := parts[1]
	status, _ = do(t, http.MethodGet, srv.URL+"/api/member-auth/invitations/"+inviteToken, "", nil)
	if status != http.StatusOK {
		t.Fatalf("inspect member invitation: got %d", status)
	}
	status, redeemed := do(t, http.MethodPost, srv.URL+"/api/member-auth/invitations/"+inviteToken+"/redeem", "", bson.M{})
	if status != http.StatusOK || fmt.Sprint(redeemed["accessToken"]) == "" || fmt.Sprint(redeemed["refreshToken"]) == "" {
		t.Fatalf("redeem member invitation: got %d (%v)", status, redeemed)
	}
	status, _ = do(t, http.MethodPost, srv.URL+"/api/member-auth/invitations/"+inviteToken+"/redeem", "", bson.M{})
	if status != http.StatusGone {
		t.Fatalf("reused member invitation: got %d", status)
	}
	access := fmt.Sprint(redeemed["accessToken"])
	status, me := do(t, http.MethodGet, srv.URL+"/api/member/me", access, nil)
	if status != http.StatusOK || me["personId"] != personID {
		t.Fatalf("member me: got %d (%v)", status, me)
	}

	status, challenge := do(t, http.MethodPost, srv.URL+"/api/member-auth/otp/request", "", bson.M{"identifier": "ama.member@test.remi"})
	if status != http.StatusAccepted || fmt.Sprint(challenge["challengeId"]) == "" || fmt.Sprint(challenge["demoCode"]) == "" {
		t.Fatalf("request member OTP: got %d (%v)", status, challenge)
	}
	status, verified := do(t, http.MethodPost, srv.URL+"/api/member-auth/otp/verify", "", bson.M{"challengeId": fmt.Sprint(challenge["challengeId"]), "code": fmt.Sprint(challenge["demoCode"]), "deviceName": "Integration browser"})
	if status != http.StatusOK {
		t.Fatalf("verify member OTP: got %d (%v)", status, verified)
	}
	status, _ = do(t, http.MethodPost, srv.URL+"/api/member-auth/otp/verify", "", bson.M{"challengeId": fmt.Sprint(challenge["challengeId"]), "code": fmt.Sprint(challenge["demoCode"])})
	if status != http.StatusUnauthorized {
		t.Fatalf("reused member OTP: got %d", status)
	}
	status, smsChallenge := do(t, http.MethodPost, srv.URL+"/api/member-auth/otp/request", "", bson.M{"identifier": "0244000000"})
	if status != http.StatusAccepted || fmt.Sprint(smsChallenge["challengeId"]) == "" || fmt.Sprint(smsChallenge["demoCode"]) == "" {
		t.Fatalf("request member SMS OTP: got %d (%v)", status, smsChallenge)
	}
	status, smsVerified := do(t, http.MethodPost, srv.URL+"/api/member-auth/otp/verify", "", bson.M{"challengeId": fmt.Sprint(smsChallenge["challengeId"]), "code": fmt.Sprint(smsChallenge["demoCode"]), "deviceName": "Member phone"})
	if status != http.StatusOK || fmt.Sprint(smsVerified["accessToken"]) == "" {
		t.Fatalf("verify member SMS OTP: got %d (%v)", status, smsVerified)
	}

	oldRefresh := fmt.Sprint(verified["refreshToken"])
	status, refreshed := do(t, http.MethodPost, srv.URL+"/api/member-auth/refresh", "", bson.M{"refreshToken": oldRefresh})
	if status != http.StatusOK || fmt.Sprint(refreshed["refreshToken"]) == oldRefresh {
		t.Fatalf("rotate member session: got %d (%v)", status, refreshed)
	}
	status, _ = do(t, http.MethodPost, srv.URL+"/api/member-auth/refresh", "", bson.M{"refreshToken": oldRefresh})
	if status != http.StatusUnauthorized {
		t.Fatalf("reused refresh token: got %d", status)
	}
	status, _ = do(t, http.MethodPost, srv.URL+"/api/member-auth/logout", "", bson.M{"refreshToken": fmt.Sprint(refreshed["refreshToken"])})
	if status != http.StatusNoContent {
		t.Fatalf("member logout: got %d", status)
	}
	status, _ = do(t, http.MethodPost, srv.URL+"/api/member-auth/refresh", "", bson.M{"refreshToken": fmt.Sprint(refreshed["refreshToken"])})
	if status != http.StatusUnauthorized {
		t.Fatalf("refresh after logout: got %d", status)
	}
}

func TestMemberMFAAndHouseholdDelegationLifecycle(t *testing.T) {
	database, srv := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, collection := range []string{"chms_people", "chms_member_accounts", "chms_member_auth_challenges", "chms_member_sessions", "chms_households", "chms_household_memberships", "chms_household_access_delegations", "chms_audit_events"} {
		_, _ = database.Collection(collection).DeleteMany(ctx, bson.M{})
	}
	accountA, accountB := bson.NewObjectID(), bson.NewObjectID()
	personA, personB, householdID := "member-mfa-a", "member-mfa-b", "household-mfa-1"
	_, err := database.Collection("chms_people").InsertMany(ctx, []any{
		bson.M{"_id": personA, "organizationId": "remi", "names": bson.M{"given": "Ama", "family": "Owusu"}, "dateOfBirth": bson.M{"value": "1990-05-10", "precision": "day"}, "archivedAt": nil},
		bson.M{"_id": personB, "organizationId": "remi", "names": bson.M{"given": "Kojo", "family": "Owusu"}, "dateOfBirth": bson.M{"value": "1988-02-03", "precision": "day"}, "archivedAt": nil},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = database.Collection("chms_member_accounts").InsertMany(ctx, []any{
		bson.M{"_id": accountA, "organizationId": "remi", "personId": personA, "name": "Ama Owusu", "email": "ama.mfa@test.remi", "emailNormalized": "ama.mfa@test.remi", "phoneNormalized": "+233244111111", "status": "active", "demoAccount": true},
		bson.M{"_id": accountB, "organizationId": "remi", "personId": personB, "name": "Kojo Owusu", "email": "kojo.mfa@test.remi", "emailNormalized": "kojo.mfa@test.remi", "phoneNormalized": "+233244222222", "status": "active", "demoAccount": true},
	})
	_, _ = database.Collection("chms_households").InsertOne(ctx, bson.M{"_id": householdID, "organizationId": "remi", "name": "Owusu household"})
	_, _ = database.Collection("chms_household_memberships").InsertMany(ctx, []any{
		bson.M{"_id": "membership-a", "organizationId": "remi", "householdId": householdID, "personId": personA, "role": "primary", "endedAt": nil},
		bson.M{"_id": "membership-b", "organizationId": "remi", "householdId": householdID, "personId": personB, "role": "adult", "endedAt": nil},
	})
	h := handlers.New(database, &config.Config{JWTSecret: "integration-test-secret"})
	tokenA, _ := h.JWT.GenerateMember(accountA.Hex(), personA, bson.NewObjectID().Hex(), householdID, "ama.mfa@test.remi", "Ama Owusu")
	tokenB, _ := h.JWT.GenerateMember(accountB.Hex(), personB, bson.NewObjectID().Hex(), householdID, "kojo.mfa@test.remi", "Kojo Owusu")

	status, setupChallenge := do(t, http.MethodPost, srv.URL+"/api/member/mfa/setup", tokenA, bson.M{"channel": "sms"})
	if status != http.StatusAccepted || fmt.Sprint(setupChallenge["demoCode"]) != "260811" {
		t.Fatalf("start member MFA: got %d (%v)", status, setupChallenge)
	}
	status, enabled := do(t, http.MethodPost, srv.URL+"/api/member/mfa/confirm", tokenA, bson.M{"challengeId": fmt.Sprint(setupChallenge["challengeId"]), "code": "260811"})
	if status != http.StatusOK || enabled["enabled"] != true {
		t.Fatalf("confirm member MFA: got %d (%v)", status, enabled)
	}

	status, primary := do(t, http.MethodPost, srv.URL+"/api/member-auth/otp/request", "", bson.M{"identifier": "ama.mfa@test.remi"})
	if status != http.StatusAccepted {
		t.Fatalf("request primary factor: %d", status)
	}
	status, second := do(t, http.MethodPost, srv.URL+"/api/member-auth/otp/verify", "", bson.M{"challengeId": fmt.Sprint(primary["challengeId"]), "code": "260811", "deviceName": "Protected browser"})
	if status != http.StatusAccepted || second["mfaRequired"] != true || second["accessToken"] != nil {
		t.Fatalf("MFA must withhold session: got %d (%v)", status, second)
	}
	status, signedIn := do(t, http.MethodPost, srv.URL+"/api/member-auth/mfa/verify", "", bson.M{"challengeId": fmt.Sprint(second["challengeId"]), "code": "260811"})
	if status != http.StatusOK || fmt.Sprint(signedIn["accessToken"]) == "" {
		t.Fatalf("verify second factor: got %d (%v)", status, signedIn)
	}
	status, _ = do(t, http.MethodPost, srv.URL+"/api/member-auth/mfa/verify", "", bson.M{"challengeId": fmt.Sprint(second["challengeId"]), "code": "260811"})
	if status != http.StatusUnauthorized {
		t.Fatalf("reused MFA challenge: got %d", status)
	}

	status, listed := do(t, http.MethodGet, srv.URL+"/api/member/household-delegations", tokenA, nil)
	candidates, _ := listed["candidates"].([]any)
	if status != http.StatusOK || len(candidates) != 1 {
		t.Fatalf("delegation candidates: got %d (%v)", status, listed)
	}
	status, created := do(t, http.MethodPost, srv.URL+"/api/member/household-delegations", tokenA, bson.M{"delegatePersonId": personB, "fields": bson.A{"profile", "statements"}, "expiresAt": time.Now().UTC().Add(90 * 24 * time.Hour)})
	if status != http.StatusCreated {
		t.Fatalf("create delegation: got %d (%v)", status, created)
	}
	delegationID := fmt.Sprint(created["_id"])
	status, received := do(t, http.MethodGet, srv.URL+"/api/member/household-delegations", tokenB, nil)
	items, _ := received["items"].([]any)
	if status != http.StatusOK || len(items) != 1 {
		t.Fatalf("recipient delegation view: got %d (%v)", status, received)
	}
	status, _ = do(t, http.MethodDelete, srv.URL+"/api/member/household-delegations/"+delegationID, tokenB, nil)
	if status != http.StatusNotFound {
		t.Fatalf("delegate must not revoke grant: got %d", status)
	}
	status, _ = do(t, http.MethodDelete, srv.URL+"/api/member/household-delegations/"+delegationID, tokenA, nil)
	if status != http.StatusNoContent {
		t.Fatalf("grantor revoke delegation: got %d", status)
	}

	status, disableChallenge := do(t, http.MethodPost, srv.URL+"/api/member/mfa/disable/request", tokenA, bson.M{"channel": "email"})
	if status != http.StatusAccepted {
		t.Fatalf("request MFA disable: got %d (%v)", status, disableChallenge)
	}
	status, _ = do(t, http.MethodDelete, srv.URL+"/api/member/mfa", tokenA, bson.M{"challengeId": fmt.Sprint(disableChallenge["challengeId"]), "code": "260811"})
	if status != http.StatusNoContent {
		t.Fatalf("disable MFA: got %d", status)
	}
	count, _ := database.Collection("chms_audit_events").CountDocuments(ctx, bson.M{"actor.id": personA, "action": bson.M{"$in": bson.A{"member.mfa.enable", "member.mfa.disable", "member.household-delegation.create", "member.household-delegation.revoke"}}})
	if count != 4 {
		t.Fatalf("security audit evidence: got %d events, want 4", count)
	}
}

func TestMemberParticipationIsSelfScopedAndPrivacySafe(t *testing.T) {
	database, srv := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	collections := []string{"chms_people", "chms_member_accounts", "chms_households", "chms_household_memberships", "events", "event_registrations", "chms_event_registration_payments", "chms_groups", "chms_group_memberships", "chms_group_membership_events", "chms_service_occurrences", "chms_attendance", "chms_group_meetings", "chms_group_attendance", "chms_group_meeting_responses", "chms_group_meeting_response_events", "chms_communication_handoffs", "chms_audit_events"}
	for _, collection := range collections {
		_, _ = database.Collection(collection).DeleteMany(ctx, bson.M{})
	}
	now := time.Now().UTC()
	memberID, childID, otherID, leaderID, hiddenID := "participation-member", "participation-child", "participation-other", "participation-leader", "participation-hidden"
	accountID, otherAccountID := bson.NewObjectID(), bson.NewObjectID()
	_, _ = database.Collection("chms_people").InsertMany(ctx, []any{
		bson.M{"_id": memberID, "organizationId": "remi", "homeBranchId": "branch-member", "names": bson.M{"given": "Ama", "family": "Boateng"}, "archivedAt": nil},
		bson.M{"_id": childID, "organizationId": "remi", "homeBranchId": "branch-member", "names": bson.M{"given": "Esi", "family": "Boateng"}, "dateOfBirth": bson.M{"value": "2015-04-02"}, "archivedAt": nil},
		bson.M{"_id": otherID, "organizationId": "remi", "homeBranchId": "branch-other", "names": bson.M{"given": "Other", "family": "Member"}, "archivedAt": nil},
		bson.M{"_id": leaderID, "organizationId": "remi", "homeBranchId": "branch-member", "names": bson.M{"given": "Kojo", "family": "Leader"}, "contactPoints": bson.A{bson.M{"type": "email", "value": "private-leader@test.remi"}}, "archivedAt": nil},
		bson.M{"_id": hiddenID, "organizationId": "remi", "homeBranchId": "branch-member", "names": bson.M{"given": "Hidden", "family": "Person"}, "contactPoints": bson.A{bson.M{"type": "phone", "value": "+233200000000"}}, "archivedAt": nil},
	})
	_, _ = database.Collection("chms_member_accounts").InsertMany(ctx, []any{
		bson.M{"_id": accountID, "organizationId": "remi", "personId": memberID, "name": "Ama Boateng", "email": "ama.participation@test.remi", "emailNormalized": "ama.participation@test.remi", "status": "active"},
		bson.M{"_id": otherAccountID, "organizationId": "remi", "personId": otherID, "name": "Other Member", "email": "other.participation@test.remi", "emailNormalized": "other.participation@test.remi", "status": "active"},
	})
	_, _ = database.Collection("chms_households").InsertOne(ctx, bson.M{"_id": "participation-household", "organizationId": "remi", "name": "Boateng household"})
	_, _ = database.Collection("chms_household_memberships").InsertMany(ctx, []any{
		bson.M{"_id": "participation-membership-a", "organizationId": "remi", "householdId": "participation-household", "personId": memberID, "role": "primary", "endedAt": nil},
		bson.M{"_id": "participation-membership-b", "organizationId": "remi", "householdId": "participation-household", "personId": childID, "role": "child", "endedAt": nil},
	})
	eventID := bson.NewObjectID()
	_, _ = database.Collection("events").InsertOne(ctx, bson.M{"_id": eventID, "title": "Family Gathering", "slug": "family-gathering", "contentStatus": "published", "registrationEnabled": true, "capacity": int32(2), "registeredCount": int32(0), "startAt": now.Add(7 * 24 * time.Hour), "endAt": now.Add(7*24*time.Hour + 2*time.Hour), "location": "REMI Hall", "childPrecheckEnabled": true, "registrationQuestions": bson.A{bson.M{"id": "meal", "label": "Meal choice", "type": "choice", "required": true, "options": bson.A{"Jollof", "Waakye"}}, bson.M{"id": "accessibility", "label": "Accessibility support", "type": "boolean", "required": false}}})
	waitlistEventID := bson.NewObjectID()
	_, _ = database.Collection("events").InsertOne(ctx, bson.M{"_id": waitlistEventID, "title": "Full Workshop", "slug": "full-workshop", "contentStatus": "published", "registrationEnabled": true, "waitlistEnabled": true, "capacity": int32(1), "registeredCount": int32(1), "startAt": now.Add(10 * 24 * time.Hour), "endAt": now.Add(10*24*time.Hour + 2*time.Hour), "location": "Studio"})
	paidEventID := bson.NewObjectID()
	_, _ = database.Collection("events").InsertOne(ctx, bson.M{"_id": paidEventID, "title": "Family Retreat", "slug": "family-retreat", "contentStatus": "published", "registrationEnabled": true, "paymentRequired": true, "priceMinor": int64(12500), "currency": "GHS", "capacity": int32(2), "registeredCount": int32(0), "startAt": now.Add(12 * 24 * time.Hour), "endAt": now.Add(13 * 24 * time.Hour), "location": "Retreat Centre"})
	expiringEventID := bson.NewObjectID()
	_, _ = database.Collection("events").InsertOne(ctx, bson.M{"_id": expiringEventID, "title": "Paid Seminar", "slug": "paid-seminar", "contentStatus": "published", "registrationEnabled": true, "paymentRequired": true, "priceMinor": int64(5000), "currency": "GHS", "capacity": int32(1), "registeredCount": int32(0), "startAt": now.Add(14 * 24 * time.Hour), "endAt": now.Add(14*24*time.Hour + 2*time.Hour), "location": "Studio"})
	paidWaitlistEventID := bson.NewObjectID()
	_, _ = database.Collection("events").InsertOne(ctx, bson.M{"_id": paidWaitlistEventID, "title": "Paid Waitlist", "slug": "paid-waitlist", "contentStatus": "published", "registrationEnabled": true, "paymentRequired": true, "priceMinor": int64(7500), "currency": "GHS", "waitlistEnabled": true, "capacity": int32(1), "registeredCount": int32(1), "startAt": now.Add(15 * 24 * time.Hour)})
	paidOccupantID := bson.NewObjectID()
	_, _ = database.Collection("event_registrations").InsertOne(ctx, bson.M{"_id": paidOccupantID, "organizationId": "remi", "eventId": paidWaitlistEventID.Hex(), "personId": childID, "registeredByPersonId": memberID, "name": "Esi Boateng", "state": "confirmed", "activeKey": true, "createdAt": now})
	groups := []any{
		bson.M{"_id": "participation-open", "organizationId": "remi", "homeBranchId": "branch-member", "name": "Young Families", "type": "community", "description": "Families growing together", "status": "active", "discoverability": "members", "privacy": "open", "capacity": 10, "activeMemberCount": 2, "version": 1, "leaderPersonIds": bson.A{leaderID}},
		bson.M{"_id": "participation-private", "organizationId": "remi", "homeBranchId": "branch-member", "name": "Private Care", "type": "support", "status": "active", "discoverability": "staff", "privacy": "invite-only", "capacity": 10, "activeMemberCount": 0, "version": 1, "leaderNote": "must never leak"},
		bson.M{"_id": "participation-other-branch", "organizationId": "remi", "homeBranchId": "branch-other", "name": "Other Branch", "type": "community", "status": "active", "discoverability": "members", "privacy": "open", "capacity": 10, "activeMemberCount": 0, "version": 1},
	}
	_, _ = database.Collection("chms_groups").InsertMany(ctx, groups)
	_, _ = database.Collection("chms_group_memberships").InsertMany(ctx, []any{
		bson.M{"_id": "participation-leader-membership", "organizationId": "remi", "branchId": "branch-member", "groupId": "participation-open", "personId": leaderID, "role": "facilitator", "status": "active", "directoryVisibility": "members", "version": 1},
		bson.M{"_id": "participation-hidden-membership", "organizationId": "remi", "branchId": "branch-member", "groupId": "participation-open", "personId": hiddenID, "role": "member", "status": "active", "directoryVisibility": "hidden", "leaderNote": "private roster note", "version": 1},
	})
	_, _ = database.Collection("chms_group_meetings").InsertMany(ctx, []any{
		bson.M{"_id": "participation-future-meeting", "organizationId": "remi", "branchId": "branch-member", "groupId": "participation-open", "topic": "Family Circle", "startsAt": now.Add(72 * time.Hour), "endsAt": now.Add(74 * time.Hour), "timezone": "Africa/Accra", "location": "Community Hall", "status": "scheduled"},
		bson.M{"_id": "participation-private-meeting", "organizationId": "remi", "branchId": "branch-member", "groupId": "participation-private", "topic": "Private Care Meeting", "startsAt": now.Add(72 * time.Hour), "endsAt": now.Add(74 * time.Hour), "timezone": "Africa/Accra", "location": "Private Room", "status": "scheduled"},
	})
	_, _ = database.Collection("chms_service_occurrences").InsertMany(ctx, []any{
		bson.M{"_id": "participation-occurrence-present", "organizationId": "remi", "homeBranchId": "branch-member", "name": "Sunday Celebration", "startsAt": now.Add(-14 * 24 * time.Hour), "endsAt": now.Add(-14*24*time.Hour + 2*time.Hour), "timezone": "Africa/Accra"},
		bson.M{"_id": "participation-occurrence-absent", "organizationId": "remi", "homeBranchId": "branch-member", "name": "Private Absence", "startsAt": now.Add(-7 * 24 * time.Hour), "endsAt": now.Add(-7*24*time.Hour + 2*time.Hour), "timezone": "Africa/Accra"},
	})
	_, _ = database.Collection("chms_attendance").InsertMany(ctx, []any{
		bson.M{"_id": "participation-attendance-present", "organizationId": "remi", "occurrenceId": "participation-occurrence-present", "personId": memberID, "status": "present", "source": "operator", "confidence": 72, "reason": "private correction", "updatedAt": now.Add(-14 * 24 * time.Hour)},
		bson.M{"_id": "participation-attendance-absent", "organizationId": "remi", "occurrenceId": "participation-occurrence-absent", "personId": memberID, "status": "absent", "source": "operator", "confidence": 100, "reason": "private absence", "updatedAt": now.Add(-7 * 24 * time.Hour)},
	})
	jwt := handlers.New(database, &config.Config{JWTSecret: "integration-test-secret"}).JWT
	memberToken, _ := jwt.GenerateMember(accountID.Hex(), memberID, bson.NewObjectID().Hex(), "participation-household", "ama.participation@test.remi", "Ama Boateng")
	otherToken, _ := jwt.GenerateMember(otherAccountID.Hex(), otherID, bson.NewObjectID().Hex(), "", "other.participation@test.remi", "Other Member")

	status, initial := do(t, http.MethodGet, srv.URL+"/api/member/participation", memberToken, nil)
	if status != http.StatusOK {
		t.Fatalf("participation read: got %d (%v)", status, initial)
	}
	encoded, _ := json.Marshal(initial)
	for _, forbidden := range []string{"Private Care", "Other Branch", "Private Absence", "private correction", "private absence", "leaderNote", "retention"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("member projection leaked %q: %s", forbidden, encoded)
		}
	}
	attendance := initial["attendance"].([]any)
	availableGroups := initial["groups"].(map[string]any)["discoverable"].([]any)
	people := initial["registrationPeople"].([]any)
	if len(attendance) != 1 || len(availableGroups) != 1 || len(people) != 2 {
		t.Fatalf("projection scope mismatch: attendance=%d groups=%d people=%d", len(attendance), len(availableGroups), len(people))
	}

	status, _ = do(t, http.MethodPost, srv.URL+"/api/member/registrations", memberToken, bson.M{"eventId": eventID.Hex(), "personIds": bson.A{memberID}})
	if status != http.StatusBadRequest {
		t.Fatalf("required event answer must be enforced: got %d", status)
	}
	status, created := do(t, http.MethodPost, srv.URL+"/api/member/registrations", memberToken, bson.M{"eventId": eventID.Hex(), "personIds": bson.A{memberID, childID}, "answers": bson.M{"meal": "Jollof", "accessibility": false}})
	if status != http.StatusCreated || len(created["items"].([]any)) != 2 {
		t.Fatalf("household registration: got %d (%v)", status, created)
	}
	selfRegistrationID := created["items"].([]any)[0].(map[string]any)["id"].(string)
	calendarReq, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/member/registrations/"+selfRegistrationID+"/calendar.ics", nil)
	calendarReq.Header.Set("Authorization", "Bearer "+memberToken)
	calendarResponse, calendarErr := http.DefaultClient.Do(calendarReq)
	if calendarErr != nil {
		t.Fatal(calendarErr)
	}
	calendarBody, _ := io.ReadAll(calendarResponse.Body)
	calendarResponse.Body.Close()
	if calendarResponse.StatusCode != http.StatusOK || !strings.Contains(string(calendarBody), "BEGIN:VCALENDAR") || !strings.Contains(string(calendarBody), "Family Gathering") {
		t.Fatalf("owned calendar export failed status=%d body=%s", calendarResponse.StatusCode, calendarBody)
	}
	status, waitlisted := do(t, http.MethodPost, srv.URL+"/api/member/registrations", memberToken, bson.M{"eventId": waitlistEventID.Hex(), "personIds": bson.A{memberID}})
	if status != http.StatusCreated || waitlisted["state"] != "waitlisted" {
		t.Fatalf("full event waitlist: got %d (%v)", status, waitlisted)
	}
	status, _ = do(t, http.MethodPost, srv.URL+"/api/member/registrations", otherToken, bson.M{"eventId": eventID.Hex(), "personIds": bson.A{otherID}, "answers": bson.M{"meal": "Waakye"}})
	if status != http.StatusConflict {
		t.Fatalf("capacity must reject extra registration: got %d", status)
	}
	var childRegistration bson.M
	if err := database.Collection("event_registrations").FindOne(ctx, bson.M{"eventId": eventID.Hex(), "personId": childID, "state": "confirmed"}).Decode(&childRegistration); err != nil {
		t.Fatal(err)
	}
	childRegistrationID := childRegistration["_id"].(bson.ObjectID).Hex()
	status, _ = do(t, http.MethodDelete, srv.URL+"/api/member/registrations/"+childRegistrationID, otherToken, nil)
	if status != http.StatusNotFound {
		t.Fatalf("unrelated cancellation must be concealed: got %d", status)
	}
	status, _ = do(t, http.MethodDelete, srv.URL+"/api/member/registrations/"+childRegistrationID, memberToken, nil)
	if status != http.StatusNoContent {
		t.Fatalf("household registration cancellation: got %d", status)
	}
	status, paidBooking := do(t, http.MethodPost, srv.URL+"/api/member/registrations", memberToken, bson.M{"eventId": paidEventID.Hex(), "personIds": bson.A{memberID, childID}})
	if status != http.StatusCreated || paidBooking["state"] != "pending-payment" || int(paidBooking["payment"].(map[string]any)["amountMinor"].(float64)) != 25000 {
		t.Fatalf("paid event seat hold: got %d (%v)", status, paidBooking)
	}
	paidRegistrationID := paidBooking["items"].([]any)[0].(map[string]any)["id"].(string)
	status, _ = do(t, http.MethodPost, srv.URL+"/api/member/registrations/"+paidRegistrationID+"/payment", otherToken, nil)
	if status != http.StatusNotFound {
		t.Fatalf("unrelated payment checkout must be concealed: got %d", status)
	}
	status, checkout := do(t, http.MethodPost, srv.URL+"/api/member/registrations/"+paidRegistrationID+"/payment", memberToken, nil)
	if status != http.StatusCreated || checkout["state"] != "initialized" || checkout["demo"] != true || !strings.Contains(fmt.Sprint(checkout["authorizationUrl"]), "registration_payment=") {
		t.Fatalf("paid event checkout: got %d (%v)", status, checkout)
	}
	paymentID := checkout["id"].(string)
	status, _ = do(t, http.MethodPost, srv.URL+"/api/member/registration-payments/"+paymentID+"/confirm", otherToken, nil)
	if status != http.StatusNotFound {
		t.Fatalf("unrelated payment confirmation must be concealed: got %d", status)
	}
	status, confirmedPayment := do(t, http.MethodPost, srv.URL+"/api/member/registration-payments/"+paymentID+"/confirm", memberToken, nil)
	if status != http.StatusOK || confirmedPayment["state"] != "paid" {
		t.Fatalf("paid event confirmation: got %d (%v)", status, confirmedPayment)
	}
	status, replayedPayment := do(t, http.MethodPost, srv.URL+"/api/member/registration-payments/"+paymentID+"/confirm", memberToken, nil)
	if status != http.StatusOK || replayedPayment["state"] != "paid" {
		t.Fatalf("payment confirmation replay: got %d (%v)", status, replayedPayment)
	}
	confirmedPlaces, _ := database.Collection("event_registrations").CountDocuments(ctx, bson.M{"bookingId": paidBooking["payment"].(map[string]any)["bookingId"], "state": "confirmed"})
	if confirmedPlaces != 2 {
		t.Fatalf("payment must confirm the complete booking exactly once: got %d", confirmedPlaces)
	}
	status, expiringBooking := do(t, http.MethodPost, srv.URL+"/api/member/registrations", memberToken, bson.M{"eventId": expiringEventID.Hex(), "personIds": bson.A{memberID}})
	if status != http.StatusCreated || expiringBooking["state"] != "pending-payment" {
		t.Fatalf("expiring paid booking: got %d (%v)", status, expiringBooking)
	}
	_, _ = database.Collection("event_registrations").UpdateMany(ctx, bson.M{"bookingId": expiringBooking["payment"].(map[string]any)["bookingId"]}, bson.M{"$set": bson.M{"paymentExpiresAt": now.Add(-time.Minute)}})
	status, _ = do(t, http.MethodGet, srv.URL+"/api/member/participation", memberToken, nil)
	if status != http.StatusOK {
		t.Fatalf("participation refresh after expiry: got %d", status)
	}
	var expiredEvent bson.M
	_ = database.Collection("events").FindOne(ctx, bson.M{"_id": expiringEventID}).Decode(&expiredEvent)
	if fmt.Sprint(expiredEvent["registeredCount"]) != "0" {
		t.Fatalf("expired payment must release its seat: %v", expiredEvent)
	}
	status, paidWaitlist := do(t, http.MethodPost, srv.URL+"/api/member/registrations", memberToken, bson.M{"eventId": paidWaitlistEventID.Hex(), "personIds": bson.A{memberID}})
	if status != http.StatusCreated || paidWaitlist["state"] != "waitlisted" {
		t.Fatalf("paid event waitlist: got %d (%v)", status, paidWaitlist)
	}
	status, _ = do(t, http.MethodDelete, srv.URL+"/api/member/registrations/"+paidOccupantID.Hex(), memberToken, nil)
	if status != http.StatusNoContent {
		t.Fatalf("cancel paid occupant: got %d", status)
	}
	var promotedPaid bson.M
	promotedErr := database.Collection("event_registrations").FindOne(ctx, bson.M{"eventId": paidWaitlistEventID.Hex(), "personId": memberID}).Decode(&promotedPaid)
	promotedExpiry, promotedExpiryOK := promotedPaid["paymentExpiresAt"].(bson.DateTime)
	if promotedErr != nil || promotedPaid["state"] != "pending-payment" || !promotedExpiryOK || promotedExpiry.Time().Before(now) {
		t.Fatalf("paid waitlist promotion must require a fresh payment: %v err=%v", promotedPaid, promotedErr)
	}
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/transaction/initialize") {
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			_, _ = fmt.Fprintf(w, `{"status":true,"data":{"authorization_url":"https://checkout.test/pay","reference":%q}}`, payload["reference"])
			return
		}
		_, _ = fmt.Fprint(w, `{"status":true,"data":{"id":42,"status":"success","reference":"WRONG-REFERENCE","amount":12500,"currency":"GHS","channel":"card","paid_at":"2026-08-12T00:00:00Z"}}`)
	}))
	defer providerServer.Close()
	providerConfig := &config.Config{Environment: "test", JWTSecret: "integration-test-secret", CORSOrigins: []string{"http://localhost:3012"}, MemberAppURL: "http://localhost:3012", CHMSOrganizationID: "remi", PaystackSecretKey: "sk_test_member_event"}
	providerHandler := handlers.New(database, providerConfig)
	providerHandler.Paystack = services.NewPaystackServiceWithClient(providerConfig.PaystackSecretKey, providerServer.URL, providerServer.Client())
	providerAPI := httptest.NewServer(server.NewRouter(providerHandler, providerConfig.CORSOrigins))
	defer providerAPI.Close()
	mismatchEventID := bson.NewObjectID()
	_, _ = database.Collection("events").InsertOne(ctx, bson.M{"_id": mismatchEventID, "title": "Provider Match Test", "slug": "provider-match-test", "contentStatus": "published", "registrationEnabled": true, "paymentRequired": true, "priceMinor": int64(12500), "currency": "GHS", "capacity": int32(1), "registeredCount": int32(0), "startAt": now.Add(16 * 24 * time.Hour)})
	status, mismatchBooking := do(t, http.MethodPost, providerAPI.URL+"/api/member/registrations", memberToken, bson.M{"eventId": mismatchEventID.Hex(), "personIds": bson.A{memberID}})
	if status != http.StatusCreated {
		t.Fatalf("provider mismatch booking: got %d (%v)", status, mismatchBooking)
	}
	mismatchRegistrationID := mismatchBooking["items"].([]any)[0].(map[string]any)["id"].(string)
	status, mismatchCheckout := do(t, http.MethodPost, providerAPI.URL+"/api/member/registrations/"+mismatchRegistrationID+"/payment", memberToken, nil)
	if status != http.StatusCreated || mismatchCheckout["demo"] != false {
		t.Fatalf("provider checkout: got %d (%v)", status, mismatchCheckout)
	}
	status, _ = do(t, http.MethodPost, providerAPI.URL+"/api/member/registration-payments/"+fmt.Sprint(mismatchCheckout["id"])+"/confirm", memberToken, nil)
	if status != http.StatusConflict {
		t.Fatalf("provider reference mismatch must fail closed: got %d", status)
	}

	status, joined := do(t, http.MethodPost, srv.URL+"/api/member/groups/participation-open/membership", memberToken, nil)
	if status != http.StatusCreated || joined["status"] != "active" {
		t.Fatalf("join discoverable group: got %d (%v)", status, joined)
	}
	status, _ = do(t, http.MethodPost, srv.URL+"/api/member/groups/participation-private/membership", memberToken, nil)
	if status != http.StatusNotFound {
		t.Fatalf("invite-only group must be concealed: got %d", status)
	}
	status, _ = do(t, http.MethodPost, srv.URL+"/api/member/groups/participation-other-branch/membership", memberToken, nil)
	if status != http.StatusNotFound {
		t.Fatalf("cross-branch group must be concealed: got %d", status)
	}
	status, _ = do(t, http.MethodDelete, srv.URL+"/api/member/groups/participation-open/membership", memberToken, nil)
	if status != http.StatusNoContent {
		t.Fatalf("leave own group: got %d", status)
	}
	status, rejoined := do(t, http.MethodPost, srv.URL+"/api/member/groups/participation-open/membership", memberToken, nil)
	if status != http.StatusCreated || rejoined["status"] != "active" {
		t.Fatalf("rejoin ended group: got %d (%v)", status, rejoined)
	}
	eventCount, _ := database.Collection("chms_group_membership_events").CountDocuments(ctx, bson.M{"groupId": "participation-open", "personId": memberID})
	if eventCount != 3 {
		t.Fatalf("group lifecycle event chain: got %d, want 3", eventCount)
	}
	status, pathway := do(t, http.MethodPost, srv.URL+"/api/member/pathway-requests", memberToken, bson.M{"type": "baptism", "note": "I would like to prepare."})
	if status != http.StatusCreated || pathway["state"] != "requested" {
		t.Fatalf("pathway request: got %d (%v)", status, pathway)
	}
	status, replayedPathway := do(t, http.MethodPost, srv.URL+"/api/member/pathway-requests", memberToken, bson.M{"type": "baptism"})
	if status != http.StatusOK || replayedPathway["id"] != pathway["id"] {
		t.Fatalf("pathway replay: got %d (%v)", status, replayedPathway)
	}
	status, precheck := do(t, http.MethodPost, srv.URL+"/api/member/child-prechecks", memberToken, bson.M{"eventId": eventID.Hex(), "childPersonId": childID})
	if status != http.StatusCreated || len(fmt.Sprint(precheck["securityCode"])) != 8 {
		t.Fatalf("guardian pre-check: got %d (%v)", status, precheck)
	}
	status, _ = do(t, http.MethodPost, srv.URL+"/api/member/child-prechecks", otherToken, bson.M{"eventId": eventID.Hex(), "childPersonId": childID})
	if status != http.StatusNotFound {
		t.Fatalf("unrelated guardian pre-check must be concealed: got %d", status)
	}
	attendanceCount, _ := database.Collection("chms_attendance").CountDocuments(ctx, bson.M{"personId": childID})
	if attendanceCount != 0 {
		t.Fatalf("pre-check must not create attendance: got %d", attendanceCount)
	}
	status, community := do(t, http.MethodGet, srv.URL+"/api/member/groups/participation-open/community", memberToken, nil)
	if status != http.StatusOK {
		t.Fatalf("member community read: got %d (%v)", status, community)
	}
	communityJSON, _ := json.Marshal(community)
	for _, forbidden := range []string{"Hidden Person", "private roster note", "private-leader@test.remi", "+233200000000", "Private Care Meeting", "confidence", "reason"} {
		if strings.Contains(string(communityJSON), forbidden) {
			t.Fatalf("community projection leaked %q: %s", forbidden, communityJSON)
		}
	}
	if len(community["roster"].([]any)) != 2 || len(community["leaders"].([]any)) != 1 || len(community["meetings"].([]any)) != 1 {
		t.Fatalf("community projection mismatch: %v", community)
	}
	status, response := do(t, http.MethodPut, srv.URL+"/api/member/groups/participation-open/meetings/participation-future-meeting/response", memberToken, bson.M{"status": "going", "expectedVersion": 0})
	if status != http.StatusOK || response["status"] != "going" {
		t.Fatalf("meeting response: got %d (%v)", status, response)
	}
	status, _ = do(t, http.MethodPut, srv.URL+"/api/member/groups/participation-open/meetings/participation-future-meeting/response", memberToken, bson.M{"status": "maybe", "expectedVersion": 0})
	if status != http.StatusConflict {
		t.Fatalf("stale meeting response must conflict: got %d", status)
	}
	status, visibility := do(t, http.MethodPut, srv.URL+"/api/member/groups/participation-open/directory-visibility", memberToken, bson.M{"visibility": "members", "expectedVersion": 3})
	if status != http.StatusOK || visibility["visibility"] != "members" {
		t.Fatalf("group visibility: got %d (%v)", status, visibility)
	}
	status, handoff := do(t, http.MethodPost, srv.URL+"/api/member/groups/participation-open/leader-messages", memberToken, bson.M{"channel": "email", "message": "Could a leader help me with the next meeting?"})
	if status != http.StatusAccepted || handoff["state"] != "pending-consent-review" {
		t.Fatalf("leader handoff: got %d (%v)", status, handoff)
	}
	var storedHandoff bson.M
	if err := database.Collection("chms_communication_handoffs").FindOne(ctx, bson.M{"requestedByPersonId": memberID}).Decode(&storedHandoff); err != nil {
		t.Fatal(err)
	}
	storedJSON, _ := json.Marshal(storedHandoff)
	if strings.Contains(string(storedJSON), "private-leader@test.remi") || storedHandoff["state"] != "pending-consent-review" {
		t.Fatalf("handoff stored a destination or unsafe state: %v", storedHandoff)
	}
	status, _ = do(t, http.MethodGet, srv.URL+"/api/member/groups/participation-open/community", otherToken, nil)
	if status != http.StatusNotFound {
		t.Fatalf("unrelated community must be concealed: got %d", status)
	}
	count, _ := database.Collection("chms_audit_events").CountDocuments(ctx, bson.M{"actor.id": memberID, "action": bson.M{"$in": bson.A{"member.registration.create", "member.registration.cancel", "member.group-membership.join", "member.group-membership.leave"}}})
	if count != 13 {
		t.Fatalf("member participation audit evidence: got %d, want 13", count)
	}
}

func TestMemberServingWorkspaceIsSelfScopedAndLifecycleSafe(t *testing.T) {
	database, srv := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, collection := range []string{"chms_people", "chms_member_accounts", "chms_volunteer_teams", "chms_volunteer_positions", "chms_volunteer_profiles", "chms_volunteer_availability", "chms_service_plans", "chms_assignments", "chms_assignment_events", "chms_serving_checkins", "chms_audit_events"} {
		_, _ = database.Collection(collection).DeleteMany(ctx, bson.M{})
	}
	now := time.Now().UTC()
	memberID, otherID := "serving-member", "serving-other"
	accountID, otherAccountID := bson.NewObjectID(), bson.NewObjectID()
	_, _ = database.Collection("chms_people").InsertMany(ctx, []any{
		bson.M{"_id": memberID, "organizationId": "remi", "homeBranchId": "branch-serving", "names": bson.M{"given": "Ama", "family": "Server"}, "archivedAt": nil},
		bson.M{"_id": otherID, "organizationId": "remi", "homeBranchId": "branch-other", "names": bson.M{"given": "Other", "family": "Server"}, "archivedAt": nil},
	})
	_, _ = database.Collection("chms_member_accounts").InsertMany(ctx, []any{
		bson.M{"_id": accountID, "organizationId": "remi", "personId": memberID, "email": "ama.serving@test.remi", "status": "active"},
		bson.M{"_id": otherAccountID, "organizationId": "remi", "personId": otherID, "email": "other.serving@test.remi", "status": "active"},
	})
	_, _ = database.Collection("chms_volunteer_teams").InsertMany(ctx, []any{
		bson.M{"_id": "serving-team", "organizationId": "remi", "homeBranchId": "branch-serving", "name": "Hospitality", "description": "Welcome people well", "status": "active"},
		bson.M{"_id": "other-team", "organizationId": "remi", "homeBranchId": "branch-other", "name": "Private Team", "status": "active"},
	})
	_, _ = database.Collection("chms_volunteer_positions").InsertMany(ctx, []any{
		bson.M{"_id": "serving-position", "organizationId": "remi", "branchId": "branch-serving", "teamId": "serving-team", "name": "Welcome host", "status": "active", "eligibility": bson.M{"backgroundCheckRequired": true}},
		bson.M{"_id": "other-position", "organizationId": "remi", "branchId": "branch-other", "teamId": "other-team", "name": "Private Position", "status": "active"},
	})
	_, _ = database.Collection("chms_volunteer_profiles").InsertOne(ctx, bson.M{"_id": "serving-profile", "organizationId": "remi", "branchId": "branch-serving", "personId": memberID, "skills": bson.A{"hospitality"}, "preferredTeamIds": bson.A{"serving-team"}, "preferredPositionIds": bson.A{"serving-position"}, "status": "active", "eligibility": bson.M{"backgroundCheckStatus": "cleared", "backgroundCheckReference": "PRIVATE-CHECK-123"}, "version": int64(1)})
	_, _ = database.Collection("chms_service_plans").InsertOne(ctx, bson.M{"_id": "serving-plan", "organizationId": "remi", "branchId": "branch-serving", "name": "Sunday Celebration", "startsAt": now.Add(-time.Hour), "endsAt": now.Add(time.Hour), "status": "published"})
	_, _ = database.Collection("chms_assignments").InsertMany(ctx, []any{
		bson.M{"_id": "serving-current", "organizationId": "remi", "branchId": "branch-serving", "personId": memberID, "teamId": "serving-team", "positionId": "serving-position", "planId": "serving-plan", "startsAt": now.Add(-time.Hour), "endsAt": now.Add(time.Hour), "status": "accepted", "version": int64(2), "reminder": bson.M{"sentCount": 1, "lastSentAt": now.Add(-24 * time.Hour)}, "noShowReason": "must not leak", "overrideReason": "private override"},
		bson.M{"_id": "serving-other-assignment", "organizationId": "remi", "branchId": "branch-other", "personId": otherID, "teamId": "other-team", "positionId": "other-position", "planId": "serving-plan", "startsAt": now.Add(-time.Hour), "endsAt": now.Add(time.Hour), "status": "accepted", "version": int64(1), "noShowReason": "other private"},
	})
	jwt := handlers.New(database, &config.Config{JWTSecret: "integration-test-secret"}).JWT
	memberToken, _ := jwt.GenerateMember(accountID.Hex(), memberID, bson.NewObjectID().Hex(), "", "ama.serving@test.remi", "Ama Server")
	otherToken, _ := jwt.GenerateMember(otherAccountID.Hex(), otherID, bson.NewObjectID().Hex(), "", "other.serving@test.remi", "Other Server")
	status, workspace := do(t, http.MethodGet, srv.URL+"/api/member/serving-workspace", memberToken, nil)
	if status != http.StatusOK {
		t.Fatalf("serving workspace: got %d (%v)", status, workspace)
	}
	encoded, _ := json.Marshal(workspace)
	for _, forbidden := range []string{"PRIVATE-CHECK-123", "backgroundCheck", "noShowReason", "private override", "Private Team", "Private Position", "other private"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("serving projection leaked %q: %s", forbidden, encoded)
		}
	}
	if len(workspace["assignments"].([]any)) != 1 || len(workspace["teams"].([]any)) != 1 {
		t.Fatalf("serving projection scope mismatch: %v", workspace)
	}
	status, updated := do(t, http.MethodPut, srv.URL+"/api/member/serving-workspace/preferences", memberToken, bson.M{"expectedVersion": 1, "skills": bson.A{"Prayer", "Hospitality"}, "preferredTeamIds": bson.A{"serving-team"}, "preferredPositionIds": bson.A{"serving-position"}, "status": "paused"})
	if status != http.StatusOK || updated["status"] != "paused" {
		t.Fatalf("update serving preferences: got %d (%v)", status, updated)
	}
	var preserved bson.M
	_ = database.Collection("chms_volunteer_profiles").FindOne(ctx, bson.M{"_id": "serving-profile"}).Decode(&preserved)
	if !strings.Contains(fmt.Sprint(preserved["eligibility"]), "PRIVATE-CHECK-123") {
		t.Fatalf("member update erased staff eligibility: %v", preserved)
	}
	status, window := do(t, http.MethodPost, srv.URL+"/api/member/serving-workspace/availability", memberToken, bson.M{"startsAt": now.Add(24 * time.Hour), "endsAt": now.Add(48 * time.Hour), "state": "unavailable"})
	if status != http.StatusCreated {
		t.Fatalf("add member availability: got %d (%v)", status, window)
	}
	status, _ = do(t, http.MethodDelete, srv.URL+"/api/member/serving-workspace/availability/"+fmt.Sprint(window["id"]), otherToken, nil)
	if status != http.StatusNotFound {
		t.Fatalf("other member availability cancellation must be concealed: got %d", status)
	}
	status, checked := do(t, http.MethodPost, srv.URL+"/api/member/serving-workspace/assignments/serving-current/check-in", memberToken, nil)
	if status != http.StatusCreated {
		t.Fatalf("serving check-in: got %d (%v)", status, checked)
	}
	status, replay := do(t, http.MethodPost, srv.URL+"/api/member/serving-workspace/assignments/serving-current/check-in", memberToken, nil)
	if status != http.StatusOK || fmt.Sprint(replay["assignmentId"]) != "serving-current" {
		t.Fatalf("check-in replay: got %d (%v)", status, replay)
	}
	status, substitute := do(t, http.MethodPost, srv.URL+"/api/member/serving-workspace/assignments/serving-current/substitute-request", memberToken, bson.M{"expectedVersion": 2, "reason": "Family responsibility"})
	if status != http.StatusAccepted || substitute["status"] != "substitute-requested" {
		t.Fatalf("substitute request: got %d (%v)", status, substitute)
	}
	status, _ = do(t, http.MethodPost, srv.URL+"/api/member/serving-workspace/assignments/serving-current/substitute-request", otherToken, bson.M{"expectedVersion": 3, "reason": "malicious"})
	if status != http.StatusNotFound {
		t.Fatalf("other member substitute request must be concealed: got %d", status)
	}
	events, _ := database.Collection("chms_assignment_events").CountDocuments(ctx, bson.M{"assignmentId": "serving-current", "toStatus": "substitute-requested"})
	if events != 1 {
		t.Fatalf("substitute lifecycle event count: got %d", events)
	}
}

func TestSeededMemberUsesFixedOTPOnlyOutsideProduction(t *testing.T) {
	database, srv := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, collection := range []string{"chms_member_accounts", "chms_member_auth_challenges"} {
		_, _ = database.Collection(collection).DeleteMany(ctx, bson.M{})
	}
	accountID, _ := bson.ObjectIDFromHex("66b8f9500000000000000099")
	_, err := database.Collection("chms_member_accounts").InsertOne(ctx, bson.M{
		"_id": accountID, "organizationId": "remi", "personId": "seeded-member-test",
		"name": "Seeded Member", "email": "seeded.member@test.remi",
		"emailNormalized": "seeded.member@test.remi", "status": "active", "demoAccount": true,
	})
	if err != nil {
		t.Fatalf("seed demo member: %v", err)
	}

	status, challenge := do(t, http.MethodPost, srv.URL+"/api/member-auth/otp/request", "", bson.M{"identifier": "seeded.member@test.remi"})
	if status != http.StatusAccepted || fmt.Sprint(challenge["demoCode"]) != "260811" {
		t.Fatalf("seeded member OTP: got %d (%v), want fixed local code", status, challenge)
	}
	status, verified := do(t, http.MethodPost, srv.URL+"/api/member-auth/otp/verify", "", bson.M{
		"challengeId": fmt.Sprint(challenge["challengeId"]), "code": "260811",
	})
	if status != http.StatusOK || fmt.Sprint(verified["accessToken"]) == "" {
		t.Fatalf("verify seeded member OTP: got %d (%v)", status, verified)
	}
}

func TestMemberHomeIsCompleteAndSelfScoped(t *testing.T) {
	database, srv := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	now := time.Now().UTC()
	personID, otherPersonID := "home-member-1", "home-member-other"
	branchID := bson.NewObjectID()
	eventID := bson.NewObjectID()
	collections := []string{"chms_people", "chms_service_occurrences", "chms_assignments", "chms_volunteer_teams", "chms_volunteer_positions", "chms_groups", "chms_group_memberships", "event_registrations", "events", "announcements", "branches"}
	for _, collection := range collections {
		_, _ = database.Collection(collection).DeleteMany(ctx, bson.M{})
	}
	_, err := database.Collection("chms_people").InsertMany(ctx, []any{
		bson.M{"_id": personID, "organizationId": "remi", "names": bson.M{"given": "Ama", "family": "Mensah", "preferred": "Ama"}, "homeBranchId": branchID.Hex(), "membershipStage": "member", "archivedAt": nil},
		bson.M{"_id": otherPersonID, "organizationId": "remi", "names": bson.M{"given": "Private", "family": "Member"}, "homeBranchId": branchID.Hex(), "membershipStage": "member", "archivedAt": nil},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = database.Collection("branches").InsertOne(ctx, bson.M{"_id": branchID, "name": "Accra Central", "contentStatus": "published"})
	_, _ = database.Collection("chms_service_occurrences").InsertOne(ctx, bson.M{"_id": "home-occurrence", "organizationId": "remi", "homeBranchId": branchID.Hex(), "name": "Sunday Celebration", "startsAt": now.Add(48 * time.Hour), "endsAt": now.Add(50 * time.Hour), "timezone": "Africa/Accra", "status": "scheduled"})
	_, _ = database.Collection("chms_volunteer_teams").InsertOne(ctx, bson.M{"_id": "home-team", "organizationId": "remi", "name": "Hospitality"})
	_, _ = database.Collection("chms_volunteer_positions").InsertOne(ctx, bson.M{"_id": "home-position", "organizationId": "remi", "name": "Welcome host"})
	_, _ = database.Collection("chms_assignments").InsertMany(ctx, []any{
		bson.M{"_id": "home-assignment", "organizationId": "remi", "personId": personID, "teamId": "home-team", "positionId": "home-position", "planId": "home-plan", "startsAt": now.Add(48 * time.Hour), "endsAt": now.Add(50 * time.Hour), "status": "invited", "slot": 1, "version": 1},
		bson.M{"_id": "other-assignment", "organizationId": "remi", "personId": otherPersonID, "teamId": "home-team", "positionId": "home-position", "planId": "other-plan", "startsAt": now.Add(48 * time.Hour), "endsAt": now.Add(50 * time.Hour), "status": "accepted", "slot": 2, "version": 1, "noShowReason": "must remain private"},
	})
	_, _ = database.Collection("chms_groups").InsertMany(ctx, []any{
		bson.M{"_id": "home-group", "organizationId": "remi", "name": "East Legon Community", "type": "community", "status": "active", "meetingPattern": bson.M{"weekday": "thursday", "localStart": "19:00"}},
		bson.M{"_id": "private-group", "organizationId": "remi", "name": "Private Other Group", "type": "support", "status": "active"},
	})
	_, _ = database.Collection("chms_group_memberships").InsertMany(ctx, []any{
		bson.M{"_id": "home-membership", "organizationId": "remi", "groupId": "home-group", "personId": personID, "role": "member", "status": "active", "joinedAt": now.Add(-30 * 24 * time.Hour)},
		bson.M{"_id": "other-membership", "organizationId": "remi", "groupId": "private-group", "personId": otherPersonID, "role": "member", "status": "active", "joinedAt": now.Add(-20 * 24 * time.Hour)},
	})
	_, _ = database.Collection("events").InsertOne(ctx, bson.M{"_id": eventID, "title": "Welcome Lunch", "slug": "welcome-lunch", "startAt": now.Add(72 * time.Hour), "contentStatus": "published", "location": "Fellowship Hall"})
	_, _ = database.Collection("event_registrations").InsertMany(ctx, []any{
		bson.M{"_id": bson.NewObjectID(), "eventId": eventID.Hex(), "email": "ama.member@test.remi", "normalizedEmail": "ama.member@test.remi", "createdAt": now.Add(-time.Hour)},
		bson.M{"_id": bson.NewObjectID(), "eventId": eventID.Hex(), "email": "private@test.remi", "normalizedEmail": "private@test.remi", "createdAt": now.Add(-time.Hour)},
	})
	_, _ = database.Collection("announcements").InsertMany(ctx, []any{
		bson.M{"_id": bson.NewObjectID(), "title": "Community update", "body": "Visible public news", "contentStatus": "published", "publishAt": now.Add(-time.Hour), "expiresAt": nil},
		bson.M{"_id": bson.NewObjectID(), "title": "Internal draft", "body": "Never expose", "contentStatus": "draft", "publishAt": now.Add(-time.Hour), "expiresAt": nil},
	})

	jwt := handlers.New(database, &config.Config{JWTSecret: "integration-test-secret"}).JWT
	memberToken, err := jwt.GenerateMember("home-account", personID, "home-session", "", "ama.member@test.remi", "Ama")
	if err != nil {
		t.Fatal(err)
	}
	status, home := do(t, http.MethodGet, srv.URL+"/api/member/home", memberToken, nil)
	if status != http.StatusOK {
		t.Fatalf("member home status=%d body=%v", status, home)
	}
	if home["member"].(map[string]any)["name"] != "Ama" || home["nextGathering"].(map[string]any)["name"] != "Sunday Celebration" {
		t.Fatalf("member or gathering projection=%v", home)
	}
	if len(home["serving"].([]any)) != 1 || len(home["groups"].([]any)) != 1 || len(home["registrations"].([]any)) != 1 || len(home["announcements"].([]any)) != 1 {
		t.Fatalf("member home was incomplete or crossed a member boundary: %v", home)
	}
	raw, _ := bson.MarshalExtJSON(home, false, false)
	if strings.Contains(string(raw), "Private Other Group") || strings.Contains(string(raw), "must remain private") || strings.Contains(string(raw), "Internal draft") {
		t.Fatalf("member home leaked private data: %s", raw)
	}
	adminToken, _ := jwt.Generate("admin", "admin@test.remi", "Admin", "super-admin")
	status, _ = do(t, http.MethodGet, srv.URL+"/api/member/home", adminToken, nil)
	if status != http.StatusForbidden {
		t.Fatalf("staff token entered member home: %d", status)
	}
}

func TestMemberProfilePrivacyIsSelfScopedAndAudited(t *testing.T) {
	database, srv := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, collection := range []string{"chms_people", "chms_member_accounts", "chms_consent_events", "chms_data_requests", "chms_audit_events", "chms_outbox", "chms_households", "chms_household_memberships"} {
		_, _ = database.Collection(collection).DeleteMany(ctx, bson.M{})
	}
	now := time.Now().UTC()
	personID, otherID := "profile-member-1", "profile-member-other"
	person := bson.M{
		"_id": personID, "organizationId": "remi", "schemaVersion": 1, "version": 1,
		"personNumber": "REMI-1001", "names": bson.M{"given": "Ama", "family": "Mensah", "preferred": "Ama"},
		"aliases": bson.A{}, "contactPoints": bson.A{bson.M{"type": "email", "value": "ama.profile@test.remi", "normalized": "ama.profile@test.remi", "primary": true}},
		"addresses": bson.A{}, "homeBranchId": "accra-central", "branchId": "accra-central", "membershipStage": "member",
		"tags": bson.A{"choir"}, "customFields": bson.M{"staff.note": "preserve-me"},
		"communicationPreferences": bson.M{"email": true, "sms": false, "whatsapp": false, "phone": false},
		"source":                   bson.M{"type": "admin"}, "createdAt": now, "updatedAt": now, "archivedAt": nil,
	}
	other := bson.M{
		"_id": otherID, "organizationId": "remi", "schemaVersion": 1, "version": 1,
		"personNumber": "REMI-1002", "names": bson.M{"given": "Kojo", "family": "Owusu"}, "aliases": bson.A{},
		"contactPoints": bson.A{bson.M{"type": "email", "value": "kojo.private@test.remi", "normalized": "kojo.private@test.remi", "primary": true}},
		"addresses":     bson.A{}, "homeBranchId": "accra-central", "membershipStage": "member", "tags": bson.A{}, "customFields": bson.M{},
		"communicationPreferences": bson.M{}, "source": bson.M{"type": "admin"}, "createdAt": now, "updatedAt": now, "archivedAt": nil,
	}
	if _, err := database.Collection("chms_people").InsertMany(ctx, []any{person, other}); err != nil {
		t.Fatal(err)
	}
	_, _ = database.Collection("chms_member_accounts").InsertOne(ctx, bson.M{"_id": "profile-account", "organizationId": "remi", "personId": personID, "emailNormalized": "ama.profile@test.remi", "status": "active"})
	jwt := handlers.New(database, &config.Config{JWTSecret: "integration-test-secret"}).JWT
	memberToken, err := jwt.GenerateMember("profile-account", personID, "profile-session", "", "ama.profile@test.remi", "Ama")
	if err != nil {
		t.Fatal(err)
	}
	status, profile := do(t, http.MethodGet, srv.URL+"/api/member/profile", memberToken, nil)
	if status != http.StatusOK || profile["person"].(map[string]any)["id"] != personID {
		t.Fatalf("member profile: status=%d body=%v", status, profile)
	}
	update := bson.M{
		"expectedVersion":          1,
		"names":                    bson.M{"given": "Ama", "family": "Mensah", "preferred": "Mimi"},
		"contactPoints":            bson.A{bson.M{"type": "email", "value": "ama.profile@test.remi", "primary": true}, bson.M{"type": "mobile", "value": "0244000000", "primary": true}},
		"addresses":                bson.A{bson.M{"line1": "14 Community Road", "city": "Accra", "region": "Greater Accra", "country": "GH", "primary": true}},
		"communicationPreferences": bson.M{"email": true, "sms": true, "whatsapp": false, "phone": false},
		"emergencyContact":         bson.M{"name": "Esi Mensah", "relationship": "sister", "phone": "0200000000"},
		"directoryVisibility":      "members",
	}
	status, updated := do(t, http.MethodPatch, srv.URL+"/api/member/profile", memberToken, update)
	if status != http.StatusOK {
		t.Fatalf("update member profile: status=%d body=%v", status, updated)
	}
	var stored bson.M
	if err := database.Collection("chms_people").FindOne(ctx, bson.M{"_id": personID}).Decode(&stored); err != nil {
		t.Fatal(err)
	}
	custom := bson.M{}
	for _, field := range stored["customFields"].(bson.D) {
		custom[field.Key] = field.Value
	}
	if stored["homeBranchId"] != "accra-central" || stored["membershipStage"] != "member" || custom["staff.note"] != "preserve-me" || custom["member.directoryVisibility"] != "members" {
		t.Fatalf("profile update changed staff-owned data or lost member preferences: %v", stored)
	}
	if count, _ := database.Collection("chms_consent_events").CountDocuments(ctx, bson.M{"personId": personID, "channel": "sms", "state": "granted"}); count != 1 {
		t.Fatalf("sms consent events=%d, want 1", count)
	}
	if count, _ := database.Collection("chms_audit_events").CountDocuments(ctx, bson.M{"resourceId": personID, "actor.type": "member"}); count == 0 {
		t.Fatal("member profile update did not append an audit event")
	}
	if count, _ := database.Collection("chms_outbox").CountDocuments(ctx, bson.M{"aggregateId": personID}); count == 0 {
		t.Fatal("member profile update did not enqueue an outbox event")
	}
	status, _ = do(t, http.MethodPatch, srv.URL+"/api/member/profile", memberToken, update)
	if status != http.StatusPreconditionFailed {
		t.Fatalf("stale member profile update status=%d, want 412", status)
	}
	update["expectedVersion"] = 2
	update["contactPoints"] = bson.A{bson.M{"type": "email", "value": "different@test.remi", "primary": true}}
	status, _ = do(t, http.MethodPatch, srv.URL+"/api/member/profile", memberToken, update)
	if status != http.StatusBadRequest {
		t.Fatalf("login email removal status=%d, want 400", status)
	}
	status, request := do(t, http.MethodPost, srv.URL+"/api/member/data-requests", memberToken, bson.M{"type": "access", "details": "Please prepare a copy."})
	if status != http.StatusCreated || request["personId"] != personID {
		t.Fatalf("create own data request: status=%d body=%v", status, request)
	}
	_, _ = database.Collection("chms_data_requests").InsertOne(ctx, bson.M{"_id": "private-request", "organizationId": "remi", "personId": otherID, "type": "deletion", "status": "received", "createdAt": now})
	status, requests := doList(t, srv.URL+"/api/member/data-requests", memberToken)
	if status != http.StatusOK || len(requests) != 1 || requests[0]["type"] != "access" {
		t.Fatalf("data request list crossed member boundary: status=%d body=%v", status, requests)
	}
	householdID := "profile-household"
	_, _ = database.Collection("chms_households").InsertOne(ctx, bson.M{"_id": householdID, "organizationId": "remi", "schemaVersion": 1, "version": 1, "name": "Mensah Household", "homeBranchId": "accra-central", "sharedContactPoints": bson.A{}, "sharedAddresses": bson.A{}, "primaryContactPersonId": personID, "statementPreference": "individual", "createdAt": now, "updatedAt": now, "archivedAt": nil})
	_, _ = database.Collection("chms_household_memberships").InsertMany(ctx, []any{
		bson.M{"_id": "profile-household-primary", "organizationId": "remi", "householdId": householdID, "personId": personID, "role": "primary-contact", "startedAt": now, "endedAt": nil, "createdAt": now},
		bson.M{"_id": "profile-household-member", "organizationId": "remi", "householdId": householdID, "personId": otherID, "role": "member", "startedAt": now, "endedAt": nil, "createdAt": now},
	})
	householdUpdate := bson.M{"expectedVersion": 1, "name": "Mensah Family", "sharedContactPoints": bson.A{}, "sharedAddresses": bson.A{bson.M{"line1": "14 Community Road", "city": "Accra", "region": "Greater Accra", "country": "GH", "primary": true}}}
	status, household := do(t, http.MethodPatch, srv.URL+"/api/member/household", memberToken, householdUpdate)
	if status != http.StatusOK || household["name"] != "Mensah Family" {
		t.Fatalf("primary contact household update: status=%d body=%v", status, household)
	}
	status, _ = do(t, http.MethodPatch, srv.URL+"/api/member/household", memberToken, householdUpdate)
	if status != http.StatusPreconditionFailed {
		t.Fatalf("stale household update status=%d, want 412", status)
	}
	otherToken, _ := jwt.GenerateMember("other-account", otherID, "other-session", householdID, "kojo.private@test.remi", "Kojo")
	householdUpdate["expectedVersion"] = 2
	status, _ = do(t, http.MethodPatch, srv.URL+"/api/member/household", otherToken, householdUpdate)
	if status != http.StatusForbidden {
		t.Fatalf("ordinary household member update status=%d, want 403", status)
	}
	adminToken, _ := jwt.Generate("admin", "admin@test.remi", "Admin", "super-admin")
	status, _ = do(t, http.MethodGet, srv.URL+"/api/member/profile", adminToken, nil)
	if status != http.StatusForbidden {
		t.Fatalf("staff token entered member profile: %d", status)
	}
}

// do issues one JSON request and returns the status and decoded body.
func do(t *testing.T, method, url, token string, body any) (int, bson.M) {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer res.Body.Close()
	out := bson.M{}
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}

// doList issues a GET and decodes the bare-array response the list endpoints
// return.
func doList(t *testing.T, url, token string) (int, []bson.M) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer res.Body.Close()
	out := []bson.M{}
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func TestHealth(t *testing.T) {
	_, srv := setup(t)
	status, body := do(t, http.MethodGet, srv.URL+"/health", "", nil)
	if status != http.StatusOK {
		t.Fatalf("health: got %d, want 200", status)
	}
	if body["status"] != "ok" {
		t.Fatalf("health: got body %v", body)
	}
}

// seedUser upserts a login with a known password.
func seedUser(t *testing.T, db *mongo.Database, email, password, role string) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	c, cancel := ctx()
	defer cancel()
	_, err = db.Collection("users").UpdateOne(c,
		bson.M{"email": email},
		bson.M{"$set": bson.M{
			"email": email, "name": "Integration " + role,
			"role": role, "passwordHash": string(hash),
		}, "$setOnInsert": bson.M{"createdAt": models.Now()}},
		options.UpdateOne().SetUpsert(true))
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
}

func login(t *testing.T, srv *httptest.Server, email, password string) string {
	t.Helper()
	status, body := do(t, http.MethodPost, srv.URL+"/api/auth/login", "",
		bson.M{"email": email, "password": password})
	if status != http.StatusOK {
		t.Fatalf("login %s: got %d (%v), want 200", email, status, body)
	}
	token, _ := body["token"].(string)
	if token == "" {
		t.Fatalf("login %s: no token in response %v", email, body)
	}
	return token
}

func TestLogin(t *testing.T) {
	db, srv := setup(t)
	seedUser(t, db, "editor@test.remi", "correct-horse", "editor")

	t.Run("good credentials issue a token", func(t *testing.T) {
		token := login(t, srv, "editor@test.remi", "correct-horse")
		status, body := do(t, http.MethodGet, srv.URL+"/api/auth/me", token, nil)
		if status != http.StatusOK {
			t.Fatalf("me: got %d, want 200", status)
		}
		user, _ := body["user"].(map[string]any)
		if user["email"] != "editor@test.remi" || user["role"] != "editor" {
			t.Fatalf("me: unexpected user %v", user)
		}
	})

	t.Run("bad credentials are rejected", func(t *testing.T) {
		status, body := do(t, http.MethodPost, srv.URL+"/api/auth/login", "",
			bson.M{"email": "editor@test.remi", "password": "wrong"})
		if status != http.StatusUnauthorized {
			t.Fatalf("bad password: got %d (%v), want 401", status, body)
		}
		status, _ = do(t, http.MethodPost, srv.URL+"/api/auth/login", "",
			bson.M{"email": "nobody@test.remi", "password": "whatever"})
		if status != http.StatusUnauthorized {
			t.Fatalf("unknown user: got %d, want 401", status)
		}
	})
}

func TestAdminInvitationRoundTrip(t *testing.T) {
	db, srv := setup(t)
	seedUser(t, db, "inviter@test.remi", "inviter-pass", "super-admin")
	adminToken := login(t, srv, "inviter@test.remi", "inviter-pass")

	status, invited := do(t, http.MethodPost, srv.URL+"/api/admin/users", adminToken, bson.M{
		"email": "new-teammate@test.remi", "role": "editor",
	})
	if status != http.StatusCreated {
		t.Fatalf("invite: got %d (%v), want 201", status, invited)
	}
	inviteURL, _ := invited["demoInvitationUrl"].(string)
	parts := strings.Split(inviteURL, "/")
	inviteToken := parts[len(parts)-1]
	if inviteToken == "" {
		t.Fatalf("invite: missing demo invitation URL in %v", invited)
	}

	status, invitation := do(t, http.MethodGet, srv.URL+"/api/auth/invitations/"+inviteToken, "", nil)
	if status != http.StatusOK || invitation["email"] != "new-teammate@test.remi" {
		t.Fatalf("inspect invitation: got %d (%v), want 200", status, invitation)
	}

	status, accepted := do(t, http.MethodPost, srv.URL+"/api/auth/invitations/"+inviteToken+"/accept", "", bson.M{
		"name": "New Teammate", "password": "a-secure-passphrase-2026",
	})
	if status != http.StatusOK || accepted["token"] == "" {
		t.Fatalf("accept invitation: got %d (%v), want session", status, accepted)
	}
	status, _ = do(t, http.MethodGet, srv.URL+"/api/auth/invitations/"+inviteToken, "", nil)
	if status != http.StatusGone {
		t.Fatalf("reused invitation: got %d, want 410", status)
	}
	login(t, srv, "new-teammate@test.remi", "a-secure-passphrase-2026")

	status, users := doList(t, srv.URL+"/api/admin/users", adminToken)
	if status != http.StatusOK {
		t.Fatalf("list users: got %d, want 200", status)
	}
	for _, user := range users {
		if _, leaked := user["passwordHash"]; leaked {
			t.Fatal("users API leaked passwordHash")
		}
		if _, leaked := user["invitationTokenHash"]; leaked {
			t.Fatal("users API leaked invitationTokenHash")
		}
	}
}

func TestOperationalInvitationRequiresExplicitBranchScope(t *testing.T) {
	db, srv := setup(t)
	_, _ = db.Collection("branches").UpdateOne(context.Background(), bson.M{"slug": "branch-1"}, bson.M{"$set": bson.M{"slug": "branch-1", "name": "Branch One"}}, options.UpdateOne().SetUpsert(true))
	_, _ = db.Collection("ministries").UpdateOne(context.Background(), bson.M{"slug": "pastoral-care"}, bson.M{"$set": bson.M{"slug": "pastoral-care", "name": "Pastoral Care"}}, options.UpdateOne().SetUpsert(true))
	seedUser(t, db, "scope-admin@test.remi", "scope-admin-pass", "super-admin")
	adminToken := login(t, srv, "scope-admin@test.remi", "scope-admin-pass")
	status, _ := do(t, http.MethodPost, srv.URL+"/api/admin/users", adminToken, bson.M{"email": "pastor-unscoped@test.remi", "role": "pastor"})
	if status != http.StatusBadRequest {
		t.Fatalf("unscoped operational invitation status=%d, want 400", status)
	}
	status, invited := do(t, http.MethodPost, srv.URL+"/api/admin/users", adminToken, bson.M{"email": "pastor-scoped@test.remi", "role": "pastor", "branchIds": bson.A{"branch-1"}, "ministryIds": bson.A{"pastoral-care"}})
	if status != http.StatusCreated {
		t.Fatalf("scoped invitation status=%d body=%v", status, invited)
	}
	var stored bson.M
	if err := db.Collection("users").FindOne(context.Background(), bson.M{"email": "pastor-scoped@test.remi"}).Decode(&stored); err != nil {
		t.Fatal(err)
	}
	got, ok := stored["branchIds"].(bson.A)
	if !ok || len(got) != 1 || fmt.Sprint(got[0]) != "branch-1" {
		t.Fatalf("stored branch scopes=%v", stored["branchIds"])
	}
}

func TestScopeChangeRevokesExistingStaffSessionAndAudits(t *testing.T) {
	db, srv := setup(t)
	ownerEmail := fmt.Sprintf("scope-owner-%d@test.remi", time.Now().UnixNano())
	pastorEmail := fmt.Sprintf("scope-pastor-%d@test.remi", time.Now().UnixNano())
	seedUser(t, db, ownerEmail, "scope-owner-pass", "super-admin")
	seedUser(t, db, pastorEmail, "scope-pastor-pass", "pastor")
	ctx := context.Background()
	_, _ = db.Collection("branches").UpdateOne(ctx, bson.M{"slug": "scope-branch-1"}, bson.M{"$set": bson.M{"slug": "scope-branch-1", "name": "Scope Branch One"}}, options.UpdateOne().SetUpsert(true))
	_, _ = db.Collection("branches").UpdateOne(ctx, bson.M{"slug": "scope-branch-2"}, bson.M{"$set": bson.M{"slug": "scope-branch-2", "name": "Scope Branch Two"}}, options.UpdateOne().SetUpsert(true))
	var pastor bson.M
	if err := db.Collection("users").FindOne(ctx, bson.M{"email": pastorEmail}).Decode(&pastor); err != nil {
		t.Fatal(err)
	}
	pastorID := pastor["_id"].(bson.ObjectID)
	_, err := db.Collection("users").UpdateOne(ctx, bson.M{"_id": pastorID}, bson.M{"$set": bson.M{"branchIds": bson.A{"scope-branch-1"}, "ministryIds": bson.A{}, "accessVersion": int64(0), "invitationStatus": "accepted"}})
	if err != nil {
		t.Fatal(err)
	}
	oldToken := login(t, srv, pastorEmail, "scope-pastor-pass")
	if status, _ := do(t, http.MethodGet, srv.URL+"/api/auth/me", oldToken, nil); status != http.StatusOK {
		t.Fatalf("current scoped session status=%d", status)
	}
	adminToken := login(t, srv, ownerEmail, "scope-owner-pass")
	status, result := do(t, http.MethodPatch, srv.URL+"/api/admin/users/"+pastorID.Hex()+"/scopes", adminToken, bson.M{"branchIds": bson.A{"scope-branch-2"}, "ministryIds": bson.A{}, "expectedAccessVersion": int64(0), "reason": "Pastoral assignment transferred to branch two"})
	if status != http.StatusOK || fmt.Sprint(result["accessVersion"]) != "1" {
		t.Fatalf("scope update status=%d body=%v", status, result)
	}
	if status, _ = do(t, http.MethodGet, srv.URL+"/api/auth/me", oldToken, nil); status != http.StatusUnauthorized {
		t.Fatalf("old token status=%d, want 401", status)
	}
	newToken := login(t, srv, pastorEmail, "scope-pastor-pass")
	claims, err := handlers.New(db, &config.Config{JWTSecret: "integration-test-secret"}).JWT.Validate(newToken)
	if err != nil || claims.AccessVersion == nil || *claims.AccessVersion != 1 || len(claims.BranchIDs) != 1 || claims.BranchIDs[0] != "scope-branch-2" {
		t.Fatalf("new claims=%+v err=%v", claims, err)
	}
	if count, _ := db.Collection("chms_audit_events").CountDocuments(ctx, bson.M{"resourceId": pastorID.Hex(), "action": "staff.access-scope.update", "reason": "Pastoral assignment transferred to branch two"}); count != 1 {
		t.Fatalf("scope audit count=%d", count)
	}
	status, _ = do(t, http.MethodPatch, srv.URL+"/api/admin/users/"+pastorID.Hex()+"/scopes", adminToken, bson.M{"branchIds": bson.A{"scope-branch-1"}, "expectedAccessVersion": int64(0), "reason": "Stale change must not overwrite current access"})
	if status != http.StatusConflict {
		t.Fatalf("stale scope update status=%d", status)
	}
}

func TestRoleChangeRequiresRecentMFARevokesSessionsAndAudits(t *testing.T) {
	db, srv := setup(t)
	ctx := context.Background()
	ownerEmail := fmt.Sprintf("role-owner-%d@test.remi", time.Now().UnixNano())
	targetEmail := fmt.Sprintf("role-target-%d@test.remi", time.Now().UnixNano())
	seedUser(t, db, ownerEmail, "role-owner-pass", "super-admin")
	seedUser(t, db, targetEmail, "role-target-pass", "editor")
	_, _ = db.Collection("branches").UpdateOne(ctx, bson.M{"slug": "role-branch"}, bson.M{"$set": bson.M{"slug": "role-branch", "name": "Role Branch"}}, options.UpdateOne().SetUpsert(true))

	var owner, target bson.M
	if err := db.Collection("users").FindOne(ctx, bson.M{"email": ownerEmail}).Decode(&owner); err != nil {
		t.Fatal(err)
	}
	if err := db.Collection("users").FindOne(ctx, bson.M{"email": targetEmail}).Decode(&target); err != nil {
		t.Fatal(err)
	}
	ownerID := owner["_id"].(bson.ObjectID)
	targetID := target["_id"].(bson.ObjectID)
	_, err := db.Collection("users").UpdateOne(ctx, bson.M{"_id": ownerID}, bson.M{"$set": bson.M{"accessVersion": int64(0), "invitationStatus": "accepted"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Collection("users").UpdateOne(ctx, bson.M{"_id": targetID}, bson.M{"$set": bson.M{"accessVersion": int64(0), "invitationStatus": "accepted"}})
	if err != nil {
		t.Fatal(err)
	}

	ordinaryOwnerToken := login(t, srv, ownerEmail, "role-owner-pass")
	status, body := do(t, http.MethodPatch, srv.URL+"/api/admin/users/"+targetID.Hex()+"/scopes", ordinaryOwnerToken, bson.M{"role": "pastor", "branchIds": bson.A{"role-branch"}, "ministryIds": bson.A{}, "expectedAccessVersion": int64(0), "reason": "Assign pastoral operations responsibility"})
	if status != http.StatusForbidden || !strings.Contains(fmt.Sprint(body["error"]), "MFA") {
		t.Fatalf("role change without MFA status=%d body=%v", status, body)
	}

	mfaToken, err := services.NewJWTService("integration-test-secret").GenerateMFAAuthenticatedScoped(ownerID.Hex(), ownerEmail, "Role Owner", "super-admin", time.Now().UTC(), []string{"*"}, []string{"*"}, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	oldTargetToken := login(t, srv, targetEmail, "role-target-pass")
	status, body = do(t, http.MethodPatch, srv.URL+"/api/admin/users/"+targetID.Hex()+"/scopes", mfaToken, bson.M{"role": "pastor", "branchIds": bson.A{"role-branch"}, "ministryIds": bson.A{}, "expectedAccessVersion": int64(0), "reason": "Assign pastoral operations responsibility"})
	if status != http.StatusOK || fmt.Sprint(body["role"]) != "pastor" || fmt.Sprint(body["accessVersion"]) != "1" {
		t.Fatalf("MFA role change status=%d body=%v", status, body)
	}
	if status, _ = do(t, http.MethodGet, srv.URL+"/api/auth/me", oldTargetToken, nil); status != http.StatusUnauthorized {
		t.Fatalf("old target token status=%d, want 401", status)
	}
	if count, _ := db.Collection("chms_audit_events").CountDocuments(ctx, bson.M{"resourceId": targetID.Hex(), "action": "staff.access-role.update", "reason": "Assign pastoral operations responsibility"}); count != 1 {
		t.Fatalf("role audit count=%d", count)
	}

	status, body = do(t, http.MethodPatch, srv.URL+"/api/admin/users/"+ownerID.Hex()+"/scopes", mfaToken, bson.M{"role": "viewer", "branchIds": bson.A{}, "ministryIds": bson.A{}, "expectedAccessVersion": int64(0), "reason": "Attempt to alter own administrator role"})
	if status != http.StatusConflict || !strings.Contains(fmt.Sprint(body["error"]), "own role") {
		t.Fatalf("self role change status=%d body=%v", status, body)
	}
}

func TestGroupRoutesEnforcePersistedMinistryScope(t *testing.T) {
	db, baseServer := setup(t)
	baseServer.Close()
	cfg := &config.Config{JWTSecret: "integration-test-secret", CHMSOrganizationID: "remi"}
	h := handlers.New(db, cfg)
	repository, err := community.NewMongoRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	store, err := platform.NewMongoPlatformStore(db)
	if err != nil {
		t.Fatal(err)
	}
	chmsHandler := chmshttp.New(chmshttp.Services{Community: community.Service{Store: repository, Platform: store, Authorizer: platform.GrantAuthorizer{}}, Authorizer: platform.GrantAuthorizer{}}, "remi")
	srv := httptest.NewServer(server.NewRouterWithCHMS(h, chmsHandler, nil))
	t.Cleanup(srv.Close)
	ctx := context.Background()
	email := fmt.Sprintf("group-scope-%d@test.remi", time.Now().UnixNano())
	suffix := fmt.Sprint(time.Now().UnixNano())
	youthGroupID, worshipGroupID := "route-youth-"+suffix, "route-worship-"+suffix
	seedUser(t, db, email, "group-scope-pass", "group-admin")
	var user bson.M
	if err := db.Collection("users").FindOne(ctx, bson.M{"email": email}).Decode(&user); err != nil {
		t.Fatal(err)
	}
	userID := user["_id"].(bson.ObjectID)
	_, err = db.Collection("users").UpdateOne(ctx, bson.M{"_id": userID}, bson.M{"$set": bson.M{"branchIds": bson.A{"scope-branch"}, "ministryIds": bson.A{"youth"}, "accessVersion": int64(0), "invitationStatus": "accepted"}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	groups := []any{
		bson.M{"_id": youthGroupID, "organizationId": "remi", "branchId": "scope-branch", "homeBranchId": "scope-branch", "ministryId": "youth", "schemaVersion": 1, "version": int64(1), "name": "Youth Circle", "type": "youth", "leaderPersonIds": bson.A{}, "capacity": 20, "activeMemberCount": 0, "privacy": "request", "discoverability": "members", "status": "active", "createdAt": now, "updatedAt": now},
		bson.M{"_id": worshipGroupID, "organizationId": "remi", "branchId": "scope-branch", "homeBranchId": "scope-branch", "ministryId": "worship", "schemaVersion": 1, "version": int64(1), "name": "Worship Team", "type": "ministry", "leaderPersonIds": bson.A{}, "capacity": 20, "activeMemberCount": 0, "privacy": "request", "discoverability": "members", "status": "active", "createdAt": now, "updatedAt": now},
	}
	if _, err = db.Collection("chms_groups").InsertMany(ctx, groups); err != nil {
		t.Fatal(err)
	}
	token, err := services.NewJWTService("integration-test-secret").GenerateScoped(userID.Hex(), email, "Group Operator", "group-admin", []string{"scope-branch"}, []string{"youth"}, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	status, body := do(t, http.MethodGet, srv.URL+"/api/chms/v1/groups?branchId=scope-branch", token, nil)
	items, ok := body["items"].([]any)
	if status != http.StatusOK || !ok || len(items) != 1 {
		t.Fatalf("ministry list status=%d body=%v", status, body)
	}
	item, _ := items[0].(map[string]any)
	if fmt.Sprint(item["id"]) != youthGroupID {
		t.Fatalf("ministry list leaked or omitted group: %v", items)
	}
	status, _ = do(t, http.MethodGet, srv.URL+"/api/chms/v1/groups/"+worshipGroupID, token, nil)
	if status != http.StatusNotFound {
		t.Fatalf("cross-ministry detail status=%d, want 404", status)
	}
	status, _ = do(t, http.MethodGet, srv.URL+"/api/chms/v1/groups/"+youthGroupID, token, nil)
	if status != http.StatusOK {
		t.Fatalf("own-ministry detail status=%d", status)
	}
}

func insertSermon(t *testing.T, db *mongo.Database, slug, series, status string) {
	t.Helper()
	c, cancel := ctx()
	defer cancel()
	_, err := db.Collection("sermons").InsertOne(c, bson.M{
		"title": "Sermon " + slug, "slug": slug, "series": series,
		"preacher": "Test Preacher", "date": models.Now(),
		"contentStatus": status, "createdAt": models.Now(), "updatedAt": models.Now(),
	})
	if err != nil {
		t.Fatalf("insert sermon: %v", err)
	}
}

func TestSermonsListAndSeriesFilter(t *testing.T) {
	db, srv := setup(t)
	insertSermon(t, db, "it-hearing-1", "IT Hearing God", models.StatusPublished)
	insertSermon(t, db, "it-hearing-2", "IT Hearing God", models.StatusPublished)
	insertSermon(t, db, "it-prayer-1", "IT Prayer Life", models.StatusPublished)

	status, body := do(t, http.MethodGet, srv.URL+"/api/sermons", "", nil)
	if status != http.StatusOK {
		t.Fatalf("list sermons: got %d, want 200", status)
	}
	if total, _ := body["total"].(float64); total < 3 {
		t.Fatalf("list sermons: total %v, want >= 3", body["total"])
	}

	status, body = do(t, http.MethodGet, srv.URL+"/api/sermons?series=IT+Hearing+God", "", nil)
	if status != http.StatusOK {
		t.Fatalf("series filter: got %d, want 200", status)
	}
	if total, _ := body["total"].(float64); total != 2 {
		t.Fatalf("series filter: total %v, want exactly 2", body["total"])
	}
	items, _ := body["items"].([]any)
	for _, item := range items {
		m, _ := item.(map[string]any)
		if m["series"] != "IT Hearing God" {
			t.Fatalf("series filter leaked a sermon from %v", m["series"])
		}
	}
}

func TestPublicContentFiltering(t *testing.T) {
	db, srv := setup(t)
	c, cancel := ctx()
	defer cancel()
	for _, m := range []bson.M{
		{"title": "IT Visible Ministry", "slug": "it-visible-ministry", "name": "IT Visible Ministry",
			"contentStatus": models.StatusPublished, "createdAt": models.Now(), "updatedAt": models.Now()},
		{"title": "IT Draft Ministry", "slug": "it-draft-ministry", "name": "IT Draft Ministry",
			"contentStatus": models.StatusAIDraft, "createdAt": models.Now(), "updatedAt": models.Now()},
	} {
		if _, err := db.Collection("ministries").InsertOne(c, m); err != nil {
			t.Fatalf("insert ministry: %v", err)
		}
	}

	status, list := doList(t, srv.URL+"/api/ministries", "")
	if status != http.StatusOK {
		t.Fatalf("list ministries: got %d, want 200", status)
	}
	var visible, draft bool
	for _, m := range list {
		switch m["slug"] {
		case "it-visible-ministry":
			visible = true
		case "it-draft-ministry":
			draft = true
		}
	}
	if !visible {
		t.Fatal("published ministry missing from public list")
	}
	if draft {
		t.Fatal("ai-draft ministry leaked into the public list")
	}

	// The draft is also unreachable by slug.
	status, _ = do(t, http.MethodGet, srv.URL+"/api/ministries/it-draft-ministry", "", nil)
	if status != http.StatusNotFound {
		t.Fatalf("draft by slug: got %d, want 404", status)
	}
}

func TestPublicBranchUsesStableOperationalIdentifier(t *testing.T) {
	database, srv := setup(t)
	c, cancel := ctx()
	defer cancel()
	contentID := bson.NewObjectID()
	_, err := database.Collection("branches").InsertOne(c, bson.M{"_id": contentID, "title": "IT Operational Branch", "name": "IT Operational Branch", "slug": "it-operational-branch", "contentStatus": models.StatusPublished, "createdAt": models.Now(), "updatedAt": models.Now()})
	if err != nil {
		t.Fatal(err)
	}
	status, list := doList(t, srv.URL+"/api/branches", "")
	if status != http.StatusOK {
		t.Fatalf("branches status=%d", status)
	}
	for _, branch := range list {
		if branch["slug"] == "it-operational-branch" {
			if branch["id"] != "it-operational-branch" || branch["operationalId"] != "it-operational-branch" || branch["contentId"] != contentID.Hex() {
				t.Fatalf("branch identifiers=%v", branch)
			}
			return
		}
	}
	t.Fatal("operational branch missing")
}

func TestAdminContentCRUDRoundTrip(t *testing.T) {
	db, srv := setup(t)
	seedUser(t, db, "crud@test.remi", "crud-pass", "editor")
	token := login(t, srv, "crud@test.remi", "crud-pass")

	// Create
	status, body := do(t, http.MethodPost, srv.URL+"/api/admin/announcements", token, bson.M{
		"title":     "IT Announcement",
		"body":      "Created by the integration test.",
		"publishAt": models.Now().Add(-time.Hour),
	})
	if status != http.StatusCreated {
		t.Fatalf("create: got %d (%v), want 201", status, body)
	}
	id, _ := body["id"].(string)
	if id == "" {
		t.Fatalf("create: no id in %v", body)
	}

	// Update
	status, body = do(t, http.MethodPut, srv.URL+"/api/admin/announcements/"+id, token, bson.M{
		"title": "IT Announcement (updated)",
	})
	if status != http.StatusOK || body["title"] != "IT Announcement (updated)" {
		t.Fatalf("update: got %d (%v), want 200 with new title", status, body)
	}

	// Status transition
	status, body = do(t, http.MethodPatch, srv.URL+"/api/admin/content/announcements/"+id+"/status", token, bson.M{
		"contentStatus": models.StatusInReview,
	})
	if status != http.StatusOK || body["contentStatus"] != models.StatusInReview {
		t.Fatalf("status transition: got %d (%v), want 200 in-review", status, body)
	}

	// While in-review it must not appear publicly.
	status, publicList := doList(t, srv.URL+"/api/announcements", "")
	if status != http.StatusOK {
		t.Fatalf("public announcements: got %d, want 200", status)
	}
	for _, m := range publicList {
		if title, _ := m["title"].(string); title == "IT Announcement (updated)" {
			t.Fatal("in-review announcement leaked into the public list")
		}
	}

	// Delete
	status, _ = do(t, http.MethodDelete, srv.URL+"/api/admin/announcements/"+id, token, nil)
	if status != http.StatusOK {
		t.Fatalf("delete: got %d, want 200", status)
	}
	c, cancel := ctx()
	defer cancel()
	count, err := db.Collection("announcements").CountDocuments(c, bson.M{"title": "IT Announcement (updated)"})
	if err != nil || count != 0 {
		t.Fatalf("delete: count %d err %v, want 0", count, err)
	}
}

func TestPrayerFormAndHoneypot(t *testing.T) {
	db, srv := setup(t)
	c, cancel := ctx()
	defer cancel()
	before, err := db.Collection("prayer_requests").CountDocuments(c, bson.M{})
	if err != nil {
		t.Fatalf("count prayer_requests: %v", err)
	}

	// A genuine submission is stored.
	status, _ := do(t, http.MethodPost, srv.URL+"/api/forms/prayer", "", bson.M{
		"name": "IT Member", "email": "member@test.remi", "request": "Please pray for the integration test.",
	})
	if status != http.StatusCreated {
		t.Fatalf("prayer submit: got %d, want 201", status)
	}

	// A filled honeypot is silently accepted but stores nothing.
	status, _ = do(t, http.MethodPost, srv.URL+"/api/forms/prayer", "", bson.M{
		"request": "spam", "website": "http://spam.example",
	})
	if status != http.StatusCreated {
		t.Fatalf("honeypot submit: got %d, want 201", status)
	}

	after, err := db.Collection("prayer_requests").CountDocuments(c, bson.M{})
	if err != nil {
		t.Fatalf("count prayer_requests: %v", err)
	}
	if after-before != 1 {
		t.Fatalf("prayer_requests grew by %d, want exactly 1 (honeypot must store nothing)", after-before)
	}

	status, _ = do(t, http.MethodPost, srv.URL+"/api/forms/prayer", "", bson.M{"name": "Must be discarded", "email": "discard@test.remi", "request": "Anonymous prayer", "identityMode": "anonymous", "visibility": "pastors-only", "linkToProfile": true, "linkConsent": true})
	if status != http.StatusCreated {
		t.Fatalf("anonymous prayer: got %d", status)
	}
	var anonymous bson.M
	if err = db.Collection("prayer_requests").FindOne(c, bson.M{"request": "Anonymous prayer"}).Decode(&anonymous); err != nil {
		t.Fatal(err)
	}
	if anonymous["name"] != nil || anonymous["email"] != nil || anonymous["personId"] != nil || anonymous["profileLinkStatus"] != "not-requested" {
		t.Fatalf("anonymous request retained identity or linkage: %v", anonymous)
	}

	status, _ = do(t, http.MethodPost, srv.URL+"/api/forms/prayer", "", bson.M{"name": "Public Member", "email": "public@test.remi", "request": "Please verify my profile", "identityMode": "identified", "visibility": "prayer-team", "linkToProfile": true, "linkConsent": true})
	if status != http.StatusCreated {
		t.Fatalf("public link request: got %d", status)
	}
	var pending bson.M
	if err = db.Collection("prayer_requests").FindOne(c, bson.M{"request": "Please verify my profile"}).Decode(&pending); err != nil {
		t.Fatal(err)
	}
	if pending["personId"] != nil || pending["profileLinkStatus"] != "pending-verification" || pending["profileLinkConsent"] == nil {
		t.Fatalf("public request bypassed verification or lost consent: %v", pending)
	}
}

func TestMemberPrayerRequiresExplicitSelfLinkConsent(t *testing.T) {
	db, srv := setup(t)
	c, cancel := ctx()
	defer cancel()
	_, _ = db.Collection("prayer_requests").DeleteMany(c, bson.M{"source": "member-app"})
	jwt := handlers.New(db, &config.Config{JWTSecret: "integration-test-secret"}).JWT
	memberToken, err := jwt.GenerateMember("prayer-account", "person-prayer", "prayer-session", "", "member.prayer@test.remi", "Akosua")
	if err != nil {
		t.Fatal(err)
	}
	staffToken, _ := jwt.Generate("staff", "staff@test.remi", "Staff", "editor")
	status, _ := do(t, http.MethodPost, srv.URL+"/api/member/prayer-requests", staffToken, bson.M{"request": "Staff must not use member prayer"})
	if status != http.StatusForbidden {
		t.Fatalf("staff member-prayer status=%d", status)
	}
	status, result := do(t, http.MethodPost, srv.URL+"/api/member/prayer-requests", memberToken, bson.M{"request": "Linked member prayer", "identityMode": "identified", "visibility": "pastors-only", "linkToProfile": true, "linkConsent": true})
	if status != http.StatusCreated || result["linkedToProfile"] != true {
		t.Fatalf("linked member prayer status=%d body=%v", status, result)
	}
	var linked bson.M
	if err = db.Collection("prayer_requests").FindOne(c, bson.M{"request": "Linked member prayer"}).Decode(&linked); err != nil {
		t.Fatal(err)
	}
	if linked["personId"] != "person-prayer" || linked["profileLinkStatus"] != "linked" || linked["profileLinkConsent"] == nil {
		t.Fatalf("member linkage evidence missing: %v", linked)
	}
	status, result = do(t, http.MethodPost, srv.URL+"/api/member/prayer-requests", memberToken, bson.M{"request": "Anonymous member prayer", "identityMode": "anonymous", "visibility": "pastors-only", "linkToProfile": true, "linkConsent": true})
	if status != http.StatusCreated || result["linkedToProfile"] != false {
		t.Fatalf("anonymous member prayer status=%d body=%v", status, result)
	}
	var anonymous bson.M
	_ = db.Collection("prayer_requests").FindOne(c, bson.M{"request": "Anonymous member prayer"}).Decode(&anonymous)
	if anonymous["personId"] != nil || anonymous["name"] != nil || anonymous["email"] != nil {
		t.Fatalf("anonymous member prayer retained identity: %v", anonymous)
	}
}

func TestMemberCareContentPrivacyAcknowledgementAndEncryptedNotes(t *testing.T) {
	db, srv := setup(t)
	c, cancel := ctx()
	defer cancel()
	for _, collection := range []string{"prayer_requests", "chms_member_care_requests", "chms_member_pastoral_appointments", "chms_member_content_saves", "sermons", "announcements", "settings", "chms_audit_events"} {
		_, _ = db.Collection(collection).DeleteMany(c, bson.M{})
	}
	now := time.Now().UTC()
	sermonID := bson.NewObjectID()
	_, _ = db.Collection("sermons").InsertMany(c, []any{
		bson.M{"_id": sermonID, "title": "Grace for the week", "slug": "grace-for-the-week", "description": "A published message", "contentStatus": models.StatusPublished, "date": now},
		bson.M{"_id": bson.NewObjectID(), "title": "Draft staff message", "slug": "draft-staff-message", "contentStatus": "draft", "date": now},
	})
	_, _ = db.Collection("announcements").InsertOne(c, bson.M{"_id": bson.NewObjectID(), "title": "Community lunch", "body": "Join us after service.", "contentStatus": models.StatusPublished, "publishAt": now.Add(-time.Hour)})
	_, _ = db.Collection("settings").InsertOne(c, bson.M{"isLive": true, "livestreamUrl": "https://video.example/live"})
	j := handlers.New(db, &config.Config{JWTSecret: "integration-test-secret"}).JWT
	member, _ := j.GenerateMember("care-account", "care-person", "care-session", "", "care.member@test.remi", "Esi Care")
	other, _ := j.GenerateMember("other-care-account", "other-care-person", "other-care-session", "", "other.care@test.remi", "Other Member")
	staff, _ := j.Generate("care-staff", "staff@test.remi", "Staff", "editor")

	status, _ := do(t, http.MethodGet, srv.URL+"/api/member/care-content", staff, nil)
	if status != http.StatusForbidden {
		t.Fatalf("staff care-content status=%d", status)
	}
	status, anonymous := do(t, http.MethodPost, srv.URL+"/api/member/care-requests", member, bson.M{"category": "emotional-support", "summary": "Please pray privately", "details": "I would appreciate private support this week.", "identityMode": "anonymous", "visibility": "pastors-only"})
	if status != http.StatusCreated || anonymous["trackable"] != false {
		t.Fatalf("anonymous care status=%d body=%v", status, anonymous)
	}
	var anonymousStored bson.M
	_ = db.Collection("chms_member_care_requests").FindOne(c, bson.M{"summary": "Please pray privately"}).Decode(&anonymousStored)
	if anonymousStored["personId"] != nil || anonymousStored["name"] != nil || anonymousStored["email"] != nil {
		t.Fatalf("anonymous care retained identity: %v", anonymousStored)
	}
	status, identified := do(t, http.MethodPost, srv.URL+"/api/member/care-requests", member, bson.M{"category": "spiritual-guidance", "summary": "Need some guidance", "details": "I would value a pastoral conversation about my next step.", "identityMode": "identified", "visibility": "care-team"})
	if status != http.StatusCreated || identified["trackable"] != true {
		t.Fatalf("identified care status=%d body=%v", status, identified)
	}
	careID := fmt.Sprint(identified["id"])
	_, _ = db.Collection("chms_member_care_requests").UpdateOne(c, bson.M{"_id": careID}, bson.M{"$set": bson.M{"state": "assigned", "assignedTo": "restricted-pastor", "staffNote": "must never serialize"}})
	status, _ = do(t, http.MethodPost, srv.URL+"/api/member/pastoral-appointments", member, bson.M{"topic": "I would like to discuss family life", "preferredWindow": "weekday-evening", "contactChannel": "email"})
	if status != http.StatusCreated {
		t.Fatalf("appointment status=%d", status)
	}
	status, saved := do(t, http.MethodPut, srv.URL+"/api/member/saved-content/sermon/"+sermonID.Hex(), member, bson.M{"saved": true, "note": "A private reflection only I should see"})
	if status != http.StatusOK || saved["saved"] != true {
		t.Fatalf("save status=%d body=%v", status, saved)
	}
	var rawSave bson.M
	_ = db.Collection("chms_member_content_saves").FindOne(c, bson.M{"personId": "care-person"}).Decode(&rawSave)
	rawJSON, _ := bson.MarshalExtJSON(rawSave, false, false)
	if strings.Contains(string(rawJSON), "A private reflection") || rawSave["encryptedNote"] == nil {
		t.Fatalf("private note not encrypted: %s", rawJSON)
	}

	status, workspace := do(t, http.MethodGet, srv.URL+"/api/member/care-content", member, nil)
	if status != http.StatusOK {
		t.Fatalf("workspace status=%d body=%v", status, workspace)
	}
	encoded, _ := json.Marshal(workspace)
	text := string(encoded)
	for _, forbidden := range []string{"Draft staff message", "restricted-pastor", "must never serialize"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("workspace leaked %q: %s", forbidden, text)
		}
	}
	if !strings.Contains(text, "Grace for the week") || !strings.Contains(text, "Community lunch") || !strings.Contains(text, "A private reflection only I should see") || !strings.Contains(text, "acknowledged") {
		t.Fatalf("workspace missing safe member data: %s", text)
	}
	status, otherWorkspace := do(t, http.MethodGet, srv.URL+"/api/member/care-content", other, nil)
	if status != http.StatusOK {
		t.Fatalf("other workspace status=%d", status)
	}
	otherJSON, _ := json.Marshal(otherWorkspace)
	if strings.Contains(string(otherJSON), "Need some guidance") || strings.Contains(string(otherJSON), "A private reflection") {
		t.Fatalf("cross-member data leaked: %s", otherJSON)
	}
}

func TestMemberDirectoryInboxGroupMessagesAndPreferences(t *testing.T) {
	database, srv := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	collections := []string{"chms_people", "chms_household_memberships", "chms_groups", "chms_group_memberships", "chms_member_communication_preferences", "chms_member_inbox_states", "chms_member_group_messages", "chms_communication_deliveries", "chms_communication_campaigns", "chms_communication_templates", "chms_audit_events"}
	for _, collection := range collections {
		_, _ = database.Collection(collection).DeleteMany(ctx, bson.M{})
	}
	now := time.Now().UTC()
	people := []any{
		bson.M{"_id": "directory-viewer", "organizationId": "remi", "homeBranchId": "accra", "names": bson.M{"given": "Ama", "family": "Mensah"}, "dateOfBirth": bson.M{"value": "1990-01-01"}, "contactPoints": bson.A{}, "customFields": bson.M{"member.directoryVisibility": "branch"}, "archivedAt": nil},
		bson.M{"_id": "directory-visible", "organizationId": "remi", "homeBranchId": "accra", "names": bson.M{"given": "Esi", "family": "Owusu"}, "dateOfBirth": bson.M{"value": "1992-03-04"}, "contactPoints": bson.A{bson.M{"type": "email", "value": "esi@example.com", "primary": true}, bson.M{"type": "mobile", "value": "0244000000", "primary": true}}, "customFields": bson.M{"member.directoryVisibility": "branch"}, "archivedAt": nil},
		bson.M{"_id": "directory-hidden", "organizationId": "remi", "homeBranchId": "accra", "names": bson.M{"given": "Hidden", "family": "Adult"}, "dateOfBirth": bson.M{"value": "1991-01-01"}, "contactPoints": bson.A{}, "customFields": bson.M{"member.directoryVisibility": "hidden"}, "archivedAt": nil},
		bson.M{"_id": "directory-minor", "organizationId": "remi", "homeBranchId": "accra", "names": bson.M{"given": "Young", "family": "Member"}, "dateOfBirth": bson.M{"value": now.AddDate(-10, 0, 0).Format("2006-01-02")}, "contactPoints": bson.A{}, "customFields": bson.M{"member.directoryVisibility": "branch"}, "archivedAt": nil},
		bson.M{"_id": "directory-other", "organizationId": "remi", "homeBranchId": "kumasi", "names": bson.M{"given": "Other", "family": "Branch"}, "dateOfBirth": bson.M{"value": "1990-01-01"}, "contactPoints": bson.A{}, "customFields": bson.M{"member.directoryVisibility": "branch"}, "archivedAt": nil},
	}
	if _, err := database.Collection("chms_people").InsertMany(ctx, people); err != nil {
		t.Fatal(err)
	}
	_, _ = database.Collection("chms_member_communication_preferences").InsertOne(ctx, bson.M{"_id": "directory-pref", "organizationId": "remi", "personId": "directory-visible", "quietEnabled": true, "quietStart": "21:00", "quietEnd": "07:00", "timezone": "Africa/Accra", "dailyCap": 3, "directoryFields": bson.A{"email"}, "version": 1, "createdAt": now, "updatedAt": now})
	_, _ = database.Collection("chms_groups").InsertOne(ctx, bson.M{"_id": "directory-group", "organizationId": "remi", "name": "East Legon Circle", "status": "active", "archivedAt": nil})
	_, _ = database.Collection("chms_group_memberships").InsertMany(ctx, []any{
		bson.M{"_id": "membership-viewer", "organizationId": "remi", "groupId": "directory-group", "personId": "directory-viewer", "status": "active", "directoryVisibility": "members", "endedAt": nil},
		bson.M{"_id": "membership-visible", "organizationId": "remi", "groupId": "directory-group", "personId": "directory-visible", "status": "active", "directoryVisibility": "members", "endedAt": nil},
		bson.M{"_id": "membership-hidden", "organizationId": "remi", "groupId": "directory-group", "personId": "directory-hidden", "status": "active", "directoryVisibility": "hidden", "endedAt": nil},
		bson.M{"_id": "membership-minor", "organizationId": "remi", "groupId": "directory-group", "personId": "directory-minor", "status": "active", "directoryVisibility": "members", "endedAt": nil},
	})
	templateID, campaignID, deliveryID := "directory-template", "directory-campaign", platform.ID(bson.NewObjectID().Hex())
	_, _ = database.Collection("chms_communication_templates").InsertOne(ctx, bson.M{"_id": templateID, "organizationId": "remi", "subject": "Mutable legacy subject", "body": "Mutable legacy body"})
	_, _ = database.Collection("chms_communication_campaigns").InsertOne(ctx, bson.M{"_id": campaignID, "organizationId": "remi", "templateId": templateID, "purpose": "church-updates"})
	_, _ = database.Collection("chms_communication_deliveries").InsertOne(ctx, bson.M{"_id": deliveryID, "organizationId": "remi", "campaignId": campaignID, "personId": "directory-viewer", "channel": "email", "state": "accepted", "inboxSubject": "Sunday at REMI", "inboxBody": "Hello Ama. Service begins at nine.", "purpose": "church-updates", "updatedAt": now})
	jwt := handlers.New(database, &config.Config{JWTSecret: "integration-test-secret", CHMSOrganizationID: "remi"}).JWT
	viewer, _ := jwt.GenerateMember("directory-account", "directory-viewer", "directory-session", "", "ama@test.remi", "Ama Mensah")
	hidden, _ := jwt.GenerateMember("hidden-account", "directory-hidden", "hidden-session", "", "hidden@test.remi", "Hidden Adult")

	status, branch := do(t, http.MethodGet, srv.URL+"/api/member/directory?scope=branch", viewer, nil)
	encoded, _ := json.Marshal(branch)
	if status != http.StatusOK || !strings.Contains(string(encoded), "Esi Owusu") || strings.Contains(string(encoded), "0244000000") || strings.Contains(string(encoded), "Hidden Adult") || strings.Contains(string(encoded), "Young Member") || strings.Contains(string(encoded), "Other Branch") {
		t.Fatalf("branch privacy status=%d body=%s", status, encoded)
	}
	status, group := do(t, http.MethodGet, srv.URL+"/api/member/directory?scope=group&groupId=directory-group", viewer, nil)
	groupJSON, _ := json.Marshal(group)
	if status != http.StatusOK || !strings.Contains(string(groupJSON), "Esi Owusu") || strings.Contains(string(groupJSON), "Hidden Adult") || strings.Contains(string(groupJSON), "Young Member") {
		t.Fatalf("group privacy status=%d body=%s", status, groupJSON)
	}
	status, workspace := do(t, http.MethodGet, srv.URL+"/api/member/communication-workspace", viewer, nil)
	workspaceJSON, _ := json.Marshal(workspace)
	if status != http.StatusOK || !strings.Contains(string(workspaceJSON), "Sunday at REMI") || strings.Contains(string(workspaceJSON), "Mutable legacy") {
		t.Fatalf("inbox snapshot status=%d body=%s", status, workspaceJSON)
	}
	status, _ = do(t, http.MethodPatch, srv.URL+"/api/member/inbox/"+string(deliveryID), viewer, bson.M{"action": "read"})
	if status != http.StatusNoContent {
		t.Fatalf("read state status=%d", status)
	}
	status, _ = do(t, http.MethodPut, srv.URL+"/api/member/communication-preferences", viewer, bson.M{"quietEnabled": true, "quietStart": "22:00", "quietEnd": "06:30", "timezone": "Africa/Accra", "dailyCap": 2, "directoryFields": bson.A{"email", "mobile"}, "expectedVersion": 0})
	if status != http.StatusOK {
		t.Fatalf("preference update status=%d", status)
	}
	status, _ = do(t, http.MethodPut, srv.URL+"/api/member/communication-preferences", viewer, bson.M{"quietEnabled": true, "quietStart": "22:00", "quietEnd": "06:30", "timezone": "Africa/Accra", "dailyCap": 2, "directoryFields": bson.A{"email"}, "expectedVersion": 0})
	if status != http.StatusConflict {
		t.Fatalf("stale preference status=%d", status)
	}
	status, posted := do(t, http.MethodPost, srv.URL+"/api/member/groups/directory-group/messages", viewer, bson.M{"message": "See you at our Thursday gathering."})
	if status != http.StatusCreated {
		t.Fatalf("group post status=%d body=%v", status, posted)
	}
	status, _ = do(t, http.MethodPost, srv.URL+"/api/member/groups/directory-group/messages", hidden, bson.M{"message": "This must be rejected."})
	if status != http.StatusForbidden {
		t.Fatalf("hidden author status=%d", status)
	}
	status, conversation := do(t, http.MethodGet, srv.URL+"/api/member/groups/directory-group/messages", viewer, nil)
	conversationJSON, _ := json.Marshal(conversation)
	if status != http.StatusOK || !strings.Contains(string(conversationJSON), "Thursday gathering") {
		t.Fatalf("conversation status=%d body=%s", status, conversationJSON)
	}
}

func TestMemberClientObservabilityIsAuthenticatedFiniteAndIdentityFree(t *testing.T) {
	database, srv := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _ = database.Collection("chms_member_client_events").DeleteMany(ctx, bson.M{})
	jwt := handlers.New(database, &config.Config{JWTSecret: "integration-test-secret", CHMSOrganizationID: "remi"}).JWT
	member, _ := jwt.GenerateMember("telemetry-account", "telemetry-person", "telemetry-session", "", "private.member@test.remi", "Private Member")
	staff, _ := jwt.Generate("telemetry-staff", "staff@test.remi", "Staff", "super-admin")
	payload := bson.M{"type": "route-error", "route": "giving", "digest": "safe_digest_42", "online": true}
	status, _ := do(t, http.MethodPost, srv.URL+"/api/member/client-events", "", payload)
	if status != http.StatusUnauthorized {
		t.Fatalf("anonymous telemetry status=%d", status)
	}
	status, _ = do(t, http.MethodPost, srv.URL+"/api/member/client-events", staff, payload)
	if status != http.StatusForbidden {
		t.Fatalf("staff telemetry status=%d", status)
	}
	status, _ = do(t, http.MethodPost, srv.URL+"/api/member/client-events", member, payload)
	if status != http.StatusNoContent {
		t.Fatalf("member telemetry status=%d", status)
	}
	for _, rejected := range []bson.M{
		{"type": "route-error", "route": "giving", "message": "gift amount GHS 500"},
		{"type": "route-error", "route": "/people/private-member", "digest": "safe"},
		{"type": "route-error", "route": "care", "digest": "contains spaces and private words"},
		{"type": "navigation-slow", "route": "home", "durationBucket": "exactly-4387ms"},
	} {
		status, _ = do(t, http.MethodPost, srv.URL+"/api/member/client-events", member, rejected)
		if status != http.StatusBadRequest {
			t.Fatalf("unsafe telemetry accepted status=%d payload=%v", status, rejected)
		}
	}
	var stored bson.M
	if err := database.Collection("chms_member_client_events").FindOne(ctx, bson.M{"type": "route-error"}).Decode(&stored); err != nil {
		t.Fatal(err)
	}
	encoded, _ := bson.MarshalExtJSON(stored, false, false)
	for _, forbidden := range []string{"telemetry-person", "private.member", "Private Member", "gift amount", "message", "token"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("client event persisted forbidden value %q: %s", forbidden, encoded)
		}
	}
	if stored["expiresAt"] == nil || stored["route"] != "giving" || stored["digest"] != "safe_digest_42" {
		t.Fatalf("client event retention/projection invalid: %v", stored)
	}
}

func TestMemberConsentCentreLifecycleAndAuthorization(t *testing.T) {
	db, srv := setup(t)
	c, cancel := ctx()
	defer cancel()
	for _, collection := range []string{"chms_consent_events", "chms_consent_projections", "chms_suppressions"} {
		_, _ = db.Collection(collection).DeleteMany(c, bson.M{})
	}
	repository, _ := consent.NewRepository(db)
	if err := repository.EnsureIndexes(c); err != nil {
		t.Fatal(err)
	}
	_, err := db.Collection("chms_people").UpdateOne(c, bson.M{"_id": "consent-person"}, bson.M{"$set": bson.M{"organizationId": "remi", "homeBranchId": "accra", "names": bson.M{"given": "Esi"}}}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		t.Fatal(err)
	}
	jwt := handlers.New(db, &config.Config{JWTSecret: "integration-test-secret"}).JWT
	memberToken, _ := jwt.GenerateMember("consent-account", "consent-person", "consent-session", "", "esi@test.remi", "Esi")
	staffToken, _ := jwt.Generate("staff-consent", "staff@test.remi", "Staff", "editor")
	status, body := do(t, http.MethodGet, srv.URL+"/api/member/consents", memberToken, nil)
	if status != http.StatusOK || len(body["items"].([]any)) != 0 {
		t.Fatalf("initial consent status=%d body=%v", status, body)
	}
	choice := bson.M{"purpose": "church-updates", "channel": "email", "state": "granted", "expectedVersion": 0, "evidenceReference": "privacy-centre"}
	status, body = do(t, http.MethodPut, srv.URL+"/api/member/consents", memberToken, choice)
	if status != http.StatusOK || body["state"] != "granted" || body["version"] != float64(1) {
		t.Fatalf("grant status=%d body=%v", status, body)
	}
	status, body = do(t, http.MethodGet, srv.URL+"/api/member/communication-eligibility?purpose=church-updates&channel=email", memberToken, nil)
	if status != http.StatusOK || body["eligible"] != true || body["decision"] != "eligible" {
		t.Fatalf("eligible status=%d body=%v", status, body)
	}
	choice["state"], choice["expectedVersion"] = "withdrawn", 1
	status, body = do(t, http.MethodPut, srv.URL+"/api/member/consents", memberToken, choice)
	if status != http.StatusOK || body["state"] != "withdrawn" {
		t.Fatalf("withdraw status=%d body=%v", status, body)
	}
	status, body = do(t, http.MethodGet, srv.URL+"/api/member/communication-eligibility?purpose=church-updates&channel=email", memberToken, nil)
	if status != http.StatusOK || body["eligible"] != false || body["decision"] != "withdrawn" {
		t.Fatalf("withdraw decision status=%d body=%v", status, body)
	}
	status, _ = do(t, http.MethodGet, srv.URL+"/api/member/consents", staffToken, nil)
	if status != http.StatusForbidden {
		t.Fatalf("staff member-consent status=%d", status)
	}
	status, body = do(t, http.MethodGet, srv.URL+"/api/admin/people/consent-person/consents", staffToken, nil)
	if status != http.StatusOK || len(body["items"].([]any)) != 1 {
		t.Fatalf("staff consent read status=%d body=%v", status, body)
	}
	status, body = do(t, http.MethodPost, srv.URL+"/api/admin/people/consent-person/suppressions", staffToken, bson.M{"channel": "email", "reason": "hard-bounce", "source": "email-provider", "state": "active", "expectedVersion": 0})
	if status != http.StatusOK || body["state"] != "active" {
		t.Fatalf("staff suppression status=%d body=%v", status, body)
	}
	choice["state"], choice["expectedVersion"] = "granted", 2
	status, _ = do(t, http.MethodPut, srv.URL+"/api/member/consents", memberToken, choice)
	if status != http.StatusOK {
		t.Fatalf("regrant status=%d", status)
	}
	status, body = do(t, http.MethodGet, srv.URL+"/api/member/communication-eligibility?purpose=church-updates&channel=email", memberToken, nil)
	if status != http.StatusOK || body["eligible"] != false || body["decision"] != "suppressed" {
		t.Fatalf("provider suppression bypassed status=%d body=%v", status, body)
	}
	events, _ := db.Collection("chms_consent_events").CountDocuments(c, bson.M{"personId": "consent-person"})
	suppressions, _ := db.Collection("chms_suppressions").CountDocuments(c, bson.M{"personId": "consent-person", "state": "active"})
	if events != 3 || suppressions != 1 {
		t.Fatalf("events=%d active suppressions=%d", events, suppressions)
	}
}

func TestGivingInitializeDemoMode(t *testing.T) {
	_, srv := setup(t)
	status, body := do(t, http.MethodPost, srv.URL+"/api/giving/initialize", "", bson.M{
		"email": "giver@test.remi", "amount": 5000, "category": "Tithe",
	})
	if status != http.StatusOK {
		t.Fatalf("giving initialize: got %d (%v), want 200", status, body)
	}
	if body["demo"] != true {
		t.Fatalf("giving initialize: demo %v, want true (no Paystack key configured)", body["demo"])
	}
	if ref, _ := body["reference"].(string); ref == "" {
		t.Fatalf("giving initialize: no reference in %v", body)
	}
}

func TestRBACViewerCannotWrite(t *testing.T) {
	db, srv := setup(t)
	seedUser(t, db, "viewer@test.remi", "viewer-pass", "viewer")
	token := login(t, srv, "viewer@test.remi", "viewer-pass")

	// Viewers may read admin lists...
	status, _ := do(t, http.MethodGet, srv.URL+"/api/admin/announcements", token, nil)
	if status != http.StatusOK {
		t.Fatalf("viewer read: got %d, want 200", status)
	}

	// ...but every write is forbidden.
	for _, attempt := range []struct{ method, path string }{
		{http.MethodPost, "/api/admin/announcements"},
		{http.MethodPut, "/api/admin/settings"},
		{http.MethodPatch, "/api/admin/content/announcements/000000000000000000000000/status"},
	} {
		status, body := do(t, attempt.method, srv.URL+attempt.path, token, bson.M{"title": "x"})
		if status != http.StatusForbidden {
			t.Fatalf("viewer %s %s: got %d (%v), want 403", attempt.method, attempt.path, status, body)
		}
	}

	// And no token at all is unauthorized.
	status, _ = do(t, http.MethodPost, srv.URL+"/api/admin/announcements", "", bson.M{"title": "x"})
	if status != http.StatusUnauthorized {
		t.Fatalf("anonymous write: got %d, want 401", status)
	}
}
