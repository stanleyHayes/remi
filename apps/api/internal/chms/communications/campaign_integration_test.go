package communications

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
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

type capturedSender struct{ messages []string }

func (s *capturedSender) Configured(channel string) bool { return channel == "email" }
func (s *capturedSender) Send(_ context.Context, channel, destination, subject, body string) (ProviderReceipt, error) {
	s.messages = append(s.messages, body)
	return ProviderReceipt{Provider: "resend", Reference: "email-1", AcceptedAt: time.Now().UTC()}, nil
}

func campaignPrincipal(id string) platform.Principal {
	return platform.Principal{
		Actor:          platform.Actor{Type: platform.ActorStaff, ID: platform.ID(id)},
		OrganizationID: "org-1",
		Grants: []platform.Grant{
			{Action: "*", Resource: "communication-audience", BranchIDs: []platform.ID{"accra"}, FieldClasses: []platform.FieldClass{platform.FieldPersonal}},
			{Action: "*", Resource: "communication-template", BranchIDs: []platform.ID{"accra"}, FieldClasses: []platform.FieldClass{platform.FieldPersonal}},
			{Action: "*", Resource: "communication-campaign", BranchIDs: []platform.ID{"accra"}, FieldClasses: []platform.FieldClass{platform.FieldPersonal}},
		},
	}
}

func TestCampaignApprovalDispatchEventsAndOptOut(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(testsupport.MongoURI()))
	if err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	if err = client.Ping(ctx, nil); err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	db := client.Database("remi_campaign_lifecycle_test")
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = db.Drop(cleanup)
		_ = client.Disconnect(cleanup)
	})
	repo, _ := NewRepository(db)
	if err = repo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	evidence := &evidenceStore{}
	now := time.Date(2026, 8, 12, 14, 0, 0, 0, time.UTC)
	sender := &capturedSender{}
	codec, _ := NewOptOutCodec([]byte("01234567890123456789012345678901"))
	svc := Service{Repository: repo, Platform: evidence, Authorizer: platform.GrantAuthorizer{}, Sender: sender, OptOuts: codec, PublicWebURL: "https://remi.vercel.app", WebhookSecret: "webhook-secret", Now: func() time.Time { return now }}
	owner, approver := campaignPrincipal("creator"), campaignPrincipal("approver")
	segment := people.SavedSegment{ResourceEnvelope: platform.ResourceEnvelope{ID: "seg-private", OrganizationID: "org-1", BranchID: "accra"}, Name: "Private members", Filter: people.PersonSearchFilter{BranchID: "accra", MembershipStages: []string{"member"}}, Visibility: "private", OwnerID: "creator"}
	_, _ = db.Collection("chms_people_segments").InsertOne(ctx, segment)
	verified := now.Add(-time.Hour)
	person := people.Person{ResourceEnvelope: platform.ResourceEnvelope{ID: "person-1", OrganizationID: "org-1", BranchID: "accra"}, Names: people.Names{Given: "Ama", Family: "Mensah"}, HomeBranchID: "accra", MembershipStage: "member", ContactPoints: []people.ContactPoint{{Type: "email", Normalized: "ama@example.com", VerifiedAt: &verified}}}
	_, _ = db.Collection("chms_people").InsertOne(ctx, person)
	_, _ = db.Collection("chms_consent_projections").InsertOne(ctx, bson.M{"_id": "consent-1", "organizationId": "org-1", "branchId": "accra", "personId": "person-1", "purpose": "church-updates", "channel": "email", "state": "granted"})
	audience, err := svc.Create(ctx, owner, SaveInput{Name: "Members", BranchID: "accra", SegmentIDs: []platform.ID{"seg-private"}, Purpose: "church-updates", Channel: "email"}, "audience")
	if err != nil {
		t.Fatal(err)
	}
	template, err := svc.CreateTemplate(ctx, owner, TemplateInput{Name: "Weekly", BranchID: "accra", Channel: "email", Subject: "Hello {{first_name}}", Body: "Update for {{first_name}}. Unsubscribe: {{unsubscribe_url}}"}, "template")
	if err != nil {
		t.Fatal(err)
	}
	template, err = svc.PublishTemplate(ctx, owner, template.ID, template.Version, "publish")
	if err != nil {
		t.Fatal(err)
	}
	campaign, err := svc.CreateCampaign(ctx, owner, CampaignInput{Name: "Weekly update", BranchID: "accra", AudienceID: audience.ID, TemplateID: template.ID}, "campaign")
	if err != nil {
		t.Fatal(err)
	}
	campaign, err = svc.SubmitCampaign(ctx, owner, campaign.ID, campaign.Version, "submit")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.ApproveCampaign(ctx, owner, campaign.ID, campaign.Version, "self-approve"); err == nil {
		t.Fatal("creator approved own campaign")
	}
	campaign, err = svc.ApproveCampaign(ctx, approver, campaign.ID, campaign.Version, "approve")
	if err != nil {
		t.Fatal(err)
	}
	campaign, err = svc.ScheduleCampaign(ctx, owner, campaign.ID, ScheduleInput{ExpectedVersion: campaign.Version, ScheduledAt: now, Timezone: "Africa/Accra"}, "schedule")
	if err != nil {
		t.Fatal(err)
	}
	processed, err := svc.RunDue(ctx)
	if err != nil || processed != 1 || len(sender.messages) != 1 {
		t.Fatalf("dispatch processed=%d sends=%d err=%v", processed, len(sender.messages), err)
	}
	if !strings.Contains(sender.messages[0], "https://remi.vercel.app/unsubscribe?token=") {
		t.Fatalf("missing preference link %q", sender.messages[0])
	}
	var inbox Delivery
	if err = db.Collection(deliveriesCollection).FindOne(ctx, bson.M{"organizationId": "org-1", "personId": "person-1"}).Decode(&inbox); err != nil {
		t.Fatal(err)
	}
	if inbox.InboxSubject != "Hello Ama" || !strings.Contains(inbox.InboxBody, "Update for Ama") || inbox.Purpose != "church-updates" || strings.Contains(inbox.InboxBody, "https://") {
		t.Fatalf("delivery inbox snapshot is not immutable/member-safe: %+v", inbox)
	}
	processed, err = svc.RunDue(ctx)
	if err != nil || processed != 0 || len(sender.messages) != 1 {
		t.Fatalf("dispatch replay sent again processed=%d sends=%d err=%v", processed, len(sender.messages), err)
	}
	event := ProviderEventInput{ProviderEventID: "event-1", Reference: "email-1", Type: "delivered", OccurredAt: now}
	raw, _ := json.Marshal(event)
	mac := hmac.New(sha256.New, []byte("webhook-secret"))
	_, _ = mac.Write(raw)
	signature := hex.EncodeToString(mac.Sum(nil))
	recorded, err := svc.RecordProviderEvent(ctx, "resend", raw, signature)
	if err != nil || !recorded {
		t.Fatalf("event recorded=%v err=%v", recorded, err)
	}
	recorded, err = svc.RecordProviderEvent(ctx, "resend", raw, signature)
	if err != nil || recorded {
		t.Fatalf("duplicate event recorded=%v err=%v", recorded, err)
	}
	marker := "/unsubscribe?token="
	tokenPart := strings.Split(strings.Split(sender.messages[0], marker)[1], " ")[0]
	token, err := url.QueryUnescape(tokenPart)
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.OptOut(ctx, token); err != nil {
		t.Fatal(err)
	}
	preview, _, err := svc.Preview(ctx, owner, audience.ID, 50)
	if err != nil || preview.RecipientCount != 0 || preview.ExclusionCounts["suppressed"] != 1 {
		t.Fatalf("opt-out not live preview=%+v err=%v", preview, err)
	}
}

func TestMemberDeliveryPolicyQuietHoursAndDailyCap(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(testsupport.MongoURI()))
	if err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	if err = client.Ping(ctx, nil); err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	db := client.Database("remi_member_delivery_policy_test")
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_ = db.Drop(cleanup)
		_ = client.Disconnect(cleanup)
	})
	repo, _ := NewRepository(db)
	now := time.Date(2026, 8, 12, 22, 15, 0, 0, time.UTC)
	_, err = db.Collection("chms_member_communication_preferences").InsertOne(ctx, bson.M{"_id": "preference-1", "organizationId": "org-1", "personId": "person-1", "quietEnabled": true, "quietStart": "21:00", "quietEnd": "07:00", "timezone": "Africa/Accra", "dailyCap": 2})
	if err != nil {
		t.Fatal(err)
	}
	allowed, reason, err := repo.EvaluateMemberDeliveryPolicy(ctx, "org-1", "person-1", now)
	if err != nil || allowed || reason != "quiet_hours" {
		t.Fatalf("quiet hours allowed=%v reason=%q err=%v", allowed, reason, err)
	}
	_, err = db.Collection("chms_member_communication_preferences").UpdateOne(ctx, bson.M{"_id": "preference-1"}, bson.M{"$set": bson.M{"quietEnabled": false}})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 2; index++ {
		_, err = db.Collection(deliveriesCollection).InsertOne(ctx, bson.M{"_id": platform.ID(bson.NewObjectID().Hex()), "organizationId": "org-1", "campaignId": platform.ID(bson.NewObjectID().Hex()), "personId": "person-1", "state": "accepted", "updatedAt": now.Add(-time.Duration(index+1) * time.Hour)})
		if err != nil {
			t.Fatal(err)
		}
	}
	allowed, reason, err = repo.EvaluateMemberDeliveryPolicy(ctx, "org-1", "person-1", now)
	if err != nil || allowed || reason != "daily_cap" {
		t.Fatalf("daily cap allowed=%v reason=%q err=%v", allowed, reason, err)
	}
	allowed, reason, err = repo.EvaluateMemberDeliveryPolicy(ctx, "org-1", "person-without-preferences", now)
	if err != nil || !allowed || reason != "" {
		t.Fatalf("default policy allowed=%v reason=%q err=%v", allowed, reason, err)
	}
}

func TestOptOutCodecRejectsTamperingAndExpiry(t *testing.T) {
	codec, _ := NewOptOutCodec([]byte("01234567890123456789012345678901"))
	now := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	token, err := codec.Sign(OptOutClaims{OrganizationID: "org", BranchID: "branch", PersonID: "person", CampaignID: "campaign", Purpose: "church-updates", Channel: "email", ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = codec.Verify(token, now); err != nil {
		t.Fatal(err)
	}
	if _, err = codec.Verify(token+"x", now); err == nil {
		t.Fatal("tampered token accepted")
	}
	if _, err = codec.Verify(token, now.Add(2*time.Hour)); err == nil {
		t.Fatal("expired token accepted")
	}
}
