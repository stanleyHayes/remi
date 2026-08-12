package consent

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"remi-api/internal/chms/platform"
	"remi-api/internal/testsupport"
)

func TestConsentProjectionWithdrawalAndSuppressionPrecedence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(testsupport.MongoURI()))
	if err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	if err = client.Ping(ctx, nil); err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	db := client.Database("remi_consent_test")
	t.Cleanup(func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer dropCancel()
		_ = db.Drop(dropCtx)
		_ = client.Disconnect(dropCtx)
	})
	repository, _ := NewRepository(db)
	if err = repository.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	store, _ := platform.NewMongoPlatformStore(db)
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	service := Service{Repository: repository, Platform: store, Authorizer: platform.GrantAuthorizer{}, Now: func() time.Time { return now }}
	member := platform.Principal{Actor: platform.Actor{Type: platform.ActorMember, ID: "person-1"}, OrganizationID: "org-1"}
	choice := Choice{Purpose: "church-updates", Channel: "email", State: "granted", NoticeVersion: "communications-2026-01", EvidenceReference: "member-settings", Source: "member-self-service", ExpectedVersion: 0}
	projection, err := service.Apply(ctx, member, "accra", "person-1", choice, "grant")
	if err != nil || projection.Version != 1 {
		t.Fatalf("projection=%+v err=%v", projection, err)
	}
	decision, err := service.Evaluate(ctx, member, "accra", "person-1", "church-updates", "email")
	if err != nil || !decision.Eligible || decision.Decision != "eligible" {
		t.Fatalf("decision=%+v err=%v", decision, err)
	}
	staff := platform.Principal{Actor: platform.Actor{Type: platform.ActorStaff, ID: "admin-1"}, OrganizationID: "org-1", Grants: []platform.Grant{{Action: "*", Resource: "consent", BranchIDs: []platform.ID{"accra"}, FieldClasses: []platform.FieldClass{platform.FieldPersonal}}}}
	if _, err = service.ApplySuppression(ctx, staff, "accra", "person-1", SuppressionInput{Channel: "email", Reason: "hard-bounce", Source: "email-provider", State: "active"}, "suppress"); err != nil {
		t.Fatalf("apply suppression: %v", err)
	}
	decision, err = service.Evaluate(ctx, member, "accra", "person-1", "church-updates", "email")
	if err != nil || decision.Eligible || decision.Decision != "suppressed" {
		t.Fatalf("hard-bounce decision=%+v err=%v", decision, err)
	}
	choice.State, choice.ExpectedVersion = "withdrawn", 1
	projection, err = service.Apply(ctx, member, "accra", "person-1", choice, "withdraw")
	if err != nil || projection.State != "withdrawn" {
		t.Fatalf("withdraw=%+v err=%v", projection, err)
	}
	choice.State, choice.ExpectedVersion = "granted", 2
	projection, err = service.Apply(ctx, member, "accra", "person-1", choice, "regrant")
	if err != nil || projection.State != "granted" {
		t.Fatalf("regrant=%+v err=%v", projection, err)
	}
	decision, _ = service.Evaluate(ctx, member, "accra", "person-1", "church-updates", "email")
	if decision.Eligible || decision.Decision != "suppressed" {
		t.Fatalf("regrant bypassed provider suppression: %+v", decision)
	}
	other := platform.Principal{Actor: platform.Actor{Type: platform.ActorMember, ID: "person-2"}, OrganizationID: "org-1"}
	if _, err = service.List(ctx, other, "accra", "person-1"); err == nil {
		t.Fatal("member read another person's consent")
	}
	events, _ := db.Collection(eventsCollection).CountDocuments(ctx, bson.M{"personId": "person-1"})
	if events != 3 {
		t.Fatalf("events=%d want 3", events)
	}
	audits, _ := db.Collection("chms_audit_events").CountDocuments(ctx, bson.M{"resourceType": "consent"})
	if audits != 3 {
		t.Fatalf("audits=%d want 3", audits)
	}
}

func TestConcurrentInitialConsentCreatesOneProjection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(testsupport.MongoURI()))
	if err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	if err = client.Ping(ctx, nil); err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	db := client.Database("remi_consent_race_test")
	t.Cleanup(func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer dropCancel()
		_ = db.Drop(dropCtx)
		_ = client.Disconnect(dropCtx)
	})
	repository, _ := NewRepository(db)
	_ = repository.EnsureIndexes(ctx)
	store, _ := platform.NewMongoPlatformStore(db)
	service := Service{Repository: repository, Platform: store, Authorizer: platform.GrantAuthorizer{}}
	member := platform.Principal{Actor: platform.Actor{Type: platform.ActorMember, ID: "person-race"}, OrganizationID: "org-race"}
	choice := Choice{Purpose: "event-reminders", Channel: "sms", State: "granted", NoticeVersion: "communications-2026-01", Source: "member-self-service"}
	var wait sync.WaitGroup
	wait.Add(12)
	successes := 0
	var lock sync.Mutex
	for range 12 {
		go func() {
			defer wait.Done()
			if _, applyErr := service.Apply(context.Background(), member, "accra", "person-race", choice, "race"); applyErr == nil {
				lock.Lock()
				successes++
				lock.Unlock()
			}
		}()
	}
	wait.Wait()
	count, _ := db.Collection(projectionsCollection).CountDocuments(ctx, bson.M{"personId": "person-race"})
	if successes != 1 || count != 1 {
		t.Fatalf("successes=%d projections=%d", successes, count)
	}
}
