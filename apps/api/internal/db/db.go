package db

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

// Connect opens a MongoDB client and returns the database named in the URI path.
func Connect(ctx context.Context, uri string) (*mongo.Client, *mongo.Database, error) {
	name := dbName(uri)
	if name == "" {
		return nil, nil, fmt.Errorf("MONGODB_URI must include a database name, e.g. mongodb://host:27017/remi")
	}

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, nil, err
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		return nil, nil, fmt.Errorf("ping mongo: %w", err)
	}
	return client, client.Database(name), nil
}

// EnsureInitialAdmin applies the versioned initial-admin migration once. It
// creates a missing account or repairs the pre-migration demo account, then
// records bootstrapVersion so later password changes survive every restart.
func EnsureInitialAdmin(ctx context.Context, database *mongo.Database, email, password string) (bool, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || password == "" {
		return false, nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return false, fmt.Errorf("hash initial admin password: %w", err)
	}
	users := database.Collection("users")
	var existing bson.M
	err = users.FindOne(ctx, bson.M{"email": email}).Decode(&existing)
	if err != nil && err != mongo.ErrNoDocuments {
		return false, fmt.Errorf("find initial admin: %w", err)
	}
	if err == nil {
		versioned := false
		switch version := existing["bootstrapVersion"].(type) {
		case int32:
			versioned = version >= 1
		case int64:
			versioned = version >= 1
		case int:
			versioned = version >= 1
		}
		if versioned {
			return false, nil
		}
		_, err = users.UpdateOne(ctx, bson.M{"_id": existing["_id"]}, bson.M{"$set": bson.M{
			"passwordHash": string(hash), "invitationStatus": "accepted",
			"bootstrapVersion": int32(1), "updatedAt": time.Now(),
		}})
		if err != nil {
			return false, fmt.Errorf("repair initial admin: %w", err)
		}
		return true, nil
	}
	now := time.Now()
	_, err = users.InsertOne(ctx, bson.M{
		"email": email, "name": "REMI Administrator", "role": "super-admin",
		"passwordHash": string(hash), "invitationStatus": "accepted",
		"bootstrapVersion": int32(1), "createdAt": now, "updatedAt": now,
	})
	if err != nil {
		return false, fmt.Errorf("create initial admin: %w", err)
	}
	_, _ = users.Indexes().CreateOne(ctx,
		mongo.IndexModel{Keys: bson.D{{Key: "email", Value: 1}}, Options: options.Index().SetUnique(true)})
	return true, nil
}

// EnsureMemberAuthIndexes applies the member identity invariants. TTL indexes
// remove expired one-time challenges and sessions; hashes are unique so an
// invitation or refresh token cannot ambiguously resolve to multiple records.
func EnsureMemberAuthIndexes(ctx context.Context, database *mongo.Database) error {
	definitions := []struct {
		collection string
		models     []mongo.IndexModel
	}{
		{"chms_member_accounts", []mongo.IndexModel{
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "personId", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "emailNormalized", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "phoneNormalized", Value: 1}}, Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"phoneNormalized": bson.M{"$gt": ""}})},
			{Keys: bson.D{{Key: "invitationTokenHash", Value: 1}}, Options: options.Index().SetUnique(true).SetSparse(true)},
		}},
		{"chms_member_auth_challenges", []mongo.IndexModel{
			{Keys: bson.D{{Key: "expiresAt", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
			{Keys: bson.D{{Key: "accountId", Value: 1}, {Key: "createdAt", Value: -1}}},
		}},
		{"chms_member_sessions", []mongo.IndexModel{
			{Keys: bson.D{{Key: "refreshTokenHash", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "expiresAt", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
			{Keys: bson.D{{Key: "accountId", Value: 1}, {Key: "revokedAt", Value: 1}, {Key: "lastSeenAt", Value: -1}}},
		}},
		{"chms_consent_events", []mongo.IndexModel{
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "personId", Value: 1}, {Key: "purpose", Value: 1}, {Key: "channel", Value: 1}, {Key: "occurredAt", Value: -1}}},
		}},
		{"chms_data_requests", []mongo.IndexModel{
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "personId", Value: 1}, {Key: "createdAt", Value: -1}}},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "status", Value: 1}, {Key: "createdAt", Value: 1}}},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "branchId", Value: 1}, {Key: "status", Value: 1}, {Key: "dueAt", Value: 1}}},
		}},
		{"chms_data_request_history", []mongo.IndexModel{
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "requestId", Value: 1}, {Key: "occurredAt", Value: 1}}},
		}},
		{"chms_household_access_delegations", []mongo.IndexModel{
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "grantorPersonId", Value: 1}, {Key: "state", Value: 1}, {Key: "expiresAt", Value: 1}}},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "delegatePersonId", Value: 1}, {Key: "state", Value: 1}, {Key: "expiresAt", Value: 1}}},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "grantorPersonId", Value: 1}, {Key: "delegatePersonId", Value: 1}, {Key: "state", Value: 1}}, Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"state": "active"})},
		}},
		{"event_registrations", []mongo.IndexModel{
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "personId", Value: 1}, {Key: "createdAt", Value: -1}}},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "eventId", Value: 1}, {Key: "personId", Value: 1}, {Key: "state", Value: 1}}, Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"state": "confirmed", "personId": bson.M{"$type": "string"}})},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "eventId", Value: 1}, {Key: "personId", Value: 1}, {Key: "activeKey", Value: 1}}, Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"activeKey": true, "personId": bson.M{"$type": "string"}})},
		}},
		{"chms_event_registration_payments", []mongo.IndexModel{
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "bookingId", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "reference", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "registeredByPersonId", Value: 1}, {Key: "createdAt", Value: -1}}},
		}},
		{"chms_group_meeting_responses", []mongo.IndexModel{
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "meetingId", Value: 1}, {Key: "personId", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "groupId", Value: 1}, {Key: "personId", Value: 1}, {Key: "updatedAt", Value: -1}}},
		}},
		{"chms_group_meeting_response_events", []mongo.IndexModel{
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "meetingId", Value: 1}, {Key: "personId", Value: 1}, {Key: "occurredAt", Value: 1}}},
		}},
		{"chms_serving_checkins", []mongo.IndexModel{
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "assignmentId", Value: 1}, {Key: "personId", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "personId", Value: 1}, {Key: "checkedInAt", Value: -1}}},
		}},
		{"chms_member_pathway_requests", []mongo.IndexModel{
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "personId", Value: 1}, {Key: "type", Value: 1}, {Key: "state", Value: 1}, {Key: "createdAt", Value: -1}}},
		}},
		{"chms_member_child_prechecks", []mongo.IndexModel{
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "eventId", Value: 1}, {Key: "childPersonId", Value: 1}, {Key: "guardianPersonId", Value: 1}, {Key: "state", Value: 1}}, Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"state": "prepared"})},
			{Keys: bson.D{{Key: "expiresAt", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
		}},
		{"chms_member_care_requests", []mongo.IndexModel{
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "personId", Value: 1}, {Key: "createdAt", Value: -1}}, Options: options.Index().SetPartialFilterExpression(bson.M{"personId": bson.M{"$type": "string"}})},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "state", Value: 1}, {Key: "createdAt", Value: 1}}},
		}},
		{"chms_member_pastoral_appointments", []mongo.IndexModel{
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "personId", Value: 1}, {Key: "createdAt", Value: -1}}},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "state", Value: 1}, {Key: "createdAt", Value: 1}}},
		}},
		{"chms_member_content_saves", []mongo.IndexModel{
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "personId", Value: 1}, {Key: "contentType", Value: 1}, {Key: "contentId", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "personId", Value: 1}, {Key: "saved", Value: 1}, {Key: "updatedAt", Value: -1}}},
		}},
		{"chms_member_communication_preferences", []mongo.IndexModel{
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "personId", Value: 1}}, Options: options.Index().SetUnique(true)},
		}},
		{"chms_member_inbox_states", []mongo.IndexModel{
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "personId", Value: 1}, {Key: "deliveryId", Value: 1}}, Options: options.Index().SetUnique(true)},
		}},
		{"chms_member_group_messages", []mongo.IndexModel{
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "groupId", Value: 1}, {Key: "state", Value: 1}, {Key: "createdAt", Value: -1}}},
		}},
		{"chms_member_client_events", []mongo.IndexModel{
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "type", Value: 1}, {Key: "route", Value: 1}, {Key: "recordedAt", Value: -1}}},
			{Keys: bson.D{{Key: "expiresAt", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
		}},
	}
	for _, definition := range definitions {
		if _, err := database.Collection(definition.collection).Indexes().CreateMany(ctx, definition.models); err != nil {
			return fmt.Errorf("ensure %s indexes: %w", definition.collection, err)
		}
	}
	return nil
}

func dbName(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(u.Path, "/")
}
