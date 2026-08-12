package platform

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"remi-api/internal/testsupport"
)

func TestMongoAuditQueryIsolatesOrganizationAndBranchAndPersistsExportEvidence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(testsupport.MongoURI()))
	if err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	if err = client.Ping(ctx, nil); err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	database := client.Database("remi_audit_query_test")
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_ = database.Drop(cleanup)
		_ = client.Disconnect(cleanup)
	})
	store, _ := NewMongoPlatformStore(database)
	if err = store.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 12, 6, 30, 0, 0, time.UTC)
	seed := []any{
		AuditEvent{ID: "audit-branch-1", OrganizationID: "org-1", BranchID: "branch-1", Actor: Actor{Type: ActorStaff, ID: "staff-1"}, Action: "people.update", ResourceType: "person", ResourceID: "person-1", ChangedFields: []string{"name"}, Outcome: "success", RequestID: "request-1", OccurredAt: now.Add(-time.Hour)},
		AuditEvent{ID: "audit-branch-2", OrganizationID: "org-1", BranchID: "branch-2", Actor: Actor{Type: ActorStaff, ID: "staff-2"}, Action: "attendance.record", ResourceType: "attendance", ResourceID: "attendance-2", Outcome: "success", RequestID: "request-2", OccurredAt: now.Add(-2 * time.Hour)},
		AuditEvent{ID: "audit-other-org", OrganizationID: "org-2", BranchID: "branch-1", Actor: Actor{Type: ActorStaff, ID: "staff-secret"}, Action: "secret.other-org", ResourceType: "person", ResourceID: "person-secret", Outcome: "success", Reason: "other organization marker", RequestID: "request-secret", OccurredAt: now.Add(-3 * time.Hour)},
	}
	if _, err = database.Collection(auditCollection).InsertMany(ctx, seed); err != nil {
		t.Fatal(err)
	}
	service := AuditService{Store: store, Evidence: store, Authorizer: GrantAuthorizer{}, Now: func() time.Time { return now }}
	query := AuditQuery{BranchID: "branch-1", StartsAt: now.Add(-24 * time.Hour), EndsAt: now, Limit: 100}
	principal := auditPrincipal([]ID{"branch-1"}, now)
	page, err := service.List(ctx, principal, query, "mongo-list")
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != "audit-branch-1" {
		t.Fatalf("scoped page=%+v err=%v", page, err)
	}
	artifact, err := service.Export(ctx, principal, query, "Approved branch assurance review", "mongo-export")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(artifact.Bytes), "other organization marker") || strings.Contains(string(artifact.Bytes), "audit-branch-2") || artifact.RowCount != 1 {
		t.Fatalf("cross-scope export: %s", artifact.Bytes)
	}
	count, err := database.Collection(auditCollection).CountDocuments(ctx, bson.M{"organizationId": "org-1", "action": "audit.export", "requestId": "mongo-export"})
	if err != nil || count != 1 {
		t.Fatalf("export evidence count=%d err=%v", count, err)
	}
}
