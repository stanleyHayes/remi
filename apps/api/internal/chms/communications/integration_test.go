package communications

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"remi-api/internal/chms/people"
	"remi-api/internal/chms/platform"
	"remi-api/internal/testsupport"
)

type evidenceStore struct {
	audits []platform.AuditEvent
	events []platform.OutboxRecord
}

func (s *evidenceStore) WithTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}
func (s *evidenceStore) AppendAudit(_ context.Context, v platform.AuditEvent) error {
	s.audits = append(s.audits, v)
	return nil
}
func (s *evidenceStore) EnqueueEvent(_ context.Context, v platform.OutboxRecord) error {
	s.events = append(s.events, v)
	return nil
}
func audiencePrincipal(id string) platform.Principal {
	return platform.Principal{Actor: platform.Actor{Type: platform.ActorStaff, ID: platform.ID(id)}, OrganizationID: "org-1", Grants: []platform.Grant{{Action: "*", Resource: "communication-audience", BranchIDs: []platform.ID{"accra"}, FieldClasses: []platform.FieldClass{platform.FieldPersonal}}}}
}

func TestAudiencePreviewRechecksConsentSuppressionsSafetyAndDuplicates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(testsupport.MongoURI()))
	if err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	if err = client.Ping(ctx, nil); err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	db := client.Database("remi_communications_test")
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = db.Drop(c)
		_ = client.Disconnect(c)
	})
	repo, _ := NewRepository(db)
	if err = repo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	evidence := &evidenceStore{}
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	svc := Service{Repository: repo, Platform: evidence, Authorizer: platform.GrantAuthorizer{}, Now: func() time.Time { return now }}
	p := audiencePrincipal("owner")
	segments := []people.SavedSegment{{ResourceEnvelope: platform.ResourceEnvelope{ID: "seg-members", OrganizationID: "org-1", BranchID: "accra"}, Name: "Members", Filter: people.PersonSearchFilter{BranchID: "accra", MembershipStages: []string{"member"}}, Visibility: "organization", OwnerID: "owner"}, {ResourceEnvelope: platform.ResourceEnvelope{ID: "seg-choir", OrganizationID: "org-1", BranchID: "accra"}, Name: "Choir", Filter: people.PersonSearchFilter{BranchID: "accra", Tags: []string{"choir"}}, Visibility: "private", OwnerID: "owner"}}
	for _, v := range segments {
		if _, err = db.Collection("chms_people_segments").InsertOne(ctx, v); err != nil {
			t.Fatal(err)
		}
	}
	verified := now.Add(-time.Hour)
	persons := []people.Person{
		{ResourceEnvelope: platform.ResourceEnvelope{ID: "eligible", OrganizationID: "org-1", BranchID: "accra"}, Names: people.Names{Given: "Ama", Family: "Able"}, HomeBranchID: "accra", MembershipStage: "member", Tags: []string{"choir"}, ContactPoints: []people.ContactPoint{{Type: "email", Normalized: "ama@example.com", VerifiedAt: &verified}}},
		{ResourceEnvelope: platform.ResourceEnvelope{ID: "withdrawn", OrganizationID: "org-1", BranchID: "accra"}, Names: people.Names{Given: "Esi"}, HomeBranchID: "accra", MembershipStage: "member", ContactPoints: []people.ContactPoint{{Type: "email", Normalized: "esi@example.com", VerifiedAt: &verified}}},
		{ResourceEnvelope: platform.ResourceEnvelope{ID: "minor", OrganizationID: "org-1", BranchID: "accra"}, Names: people.Names{Given: "Kofi"}, HomeBranchID: "accra", MembershipStage: "member", DateOfBirth: &people.PartialDate{Value: "2012-01-01", Precision: "day"}, ContactPoints: []people.ContactPoint{{Type: "email", Normalized: "kofi@example.com", VerifiedAt: &verified}}},
		{ResourceEnvelope: platform.ResourceEnvelope{ID: "unverified", OrganizationID: "org-1", BranchID: "accra"}, Names: people.Names{Given: "Jo"}, HomeBranchID: "accra", MembershipStage: "member", ContactPoints: []people.ContactPoint{{Type: "email", Normalized: "jo@example.com"}}},
		{ResourceEnvelope: platform.ResourceEnvelope{ID: "duplicate-destination", OrganizationID: "org-1", BranchID: "accra"}, Names: people.Names{Given: "Amaa"}, HomeBranchID: "accra", MembershipStage: "member", ContactPoints: []people.ContactPoint{{Type: "email", Normalized: "ama@example.com", VerifiedAt: &verified}}},
	}
	for _, v := range persons {
		if _, err = db.Collection("chms_people").InsertOne(ctx, v); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"eligible", "minor", "unverified", "duplicate-destination"} {
		_, _ = db.Collection("chms_consent_projections").InsertOne(ctx, bson.M{"_id": "consent-" + id, "organizationId": "org-1", "personId": id, "purpose": "church-updates", "channel": "email", "state": "granted"})
	}
	_, _ = db.Collection("chms_consent_projections").InsertOne(ctx, bson.M{"_id": "consent-withdrawn", "organizationId": "org-1", "personId": "withdrawn", "purpose": "church-updates", "channel": "email", "state": "withdrawn"})
	a, err := svc.Create(ctx, p, SaveInput{Name: "Weekly members", BranchID: "accra", SegmentIDs: []platform.ID{"seg-members", "seg-choir"}, Purpose: "church-updates", Channel: "email"}, "create")
	if err != nil {
		t.Fatal(err)
	}
	preview, all, err := svc.Preview(ctx, p, a.ID, 50)
	if err != nil {
		t.Fatal(err)
	}
	if preview.CandidateCount != 6 || preview.RecipientCount != 1 || len(all) != 1 || all[0].Destination != "ama@example.com" {
		t.Fatalf("unsafe preview=%+v recipients=%+v", preview, all)
	}
	if preview.ExclusionCounts["duplicate-segment-membership"] != 1 || preview.ExclusionCounts["withdrawn"] != 1 || preview.ExclusionCounts["minor"] != 1 || preview.ExclusionCounts["no-verified-contact"] != 1 || preview.ExclusionCounts["duplicate-destination"] != 1 {
		t.Fatalf("bad exclusions=%+v", preview.ExclusionCounts)
	}
	// A withdrawal after save must immediately remove the recipient.
	_, _ = db.Collection("chms_consent_projections").UpdateMany(ctx, bson.M{"personId": bson.M{"$in": bson.A{"eligible", "duplicate-destination"}}}, bson.M{"$set": bson.M{"state": "withdrawn"}})
	preview, _, err = svc.Preview(ctx, p, a.ID, 50)
	if err != nil || preview.RecipientCount != 0 || preview.ExclusionCounts["withdrawn"] != 3 {
		t.Fatalf("withdrawal not live preview=%+v err=%v", preview, err)
	}
	_, _ = db.Collection("chms_consent_projections").UpdateMany(ctx, bson.M{"personId": bson.M{"$in": bson.A{"eligible", "duplicate-destination"}}}, bson.M{"$set": bson.M{"state": "granted"}})
	var csv bytes.Buffer
	record, err := svc.Export(ctx, p, a.ID, "Approved weekly member update", "export-request", &csv)
	if err != nil {
		t.Fatal(err)
	}
	if record.RecipientCount != 1 || !strings.Contains(csv.String(), "ama@example.com") {
		t.Fatalf("bad export=%+v csv=%q", record, csv.String())
	}
	if len(evidence.audits) != 2 || evidence.audits[1].Action != "communications.audience.export" || evidence.audits[1].Reason == "" {
		t.Fatalf("missing audit %+v", evidence.audits)
	}
	member := platform.Principal{Actor: platform.Actor{Type: platform.ActorMember, ID: "eligible"}, OrganizationID: "org-1"}
	if _, _, err = svc.Preview(ctx, member, a.ID, 50); err == nil {
		t.Fatal("member previewed staff audience")
	}
	other := audiencePrincipal("other")
	if _, err = svc.Update(ctx, other, a.ID, SaveInput{Name: "Hijack", BranchID: "accra", SegmentIDs: []platform.ID{"seg-members"}, Purpose: "church-updates", Channel: "email", ExpectedVersion: 1}, "hijack"); err == nil {
		t.Fatal("non-owner updated audience")
	}
}
