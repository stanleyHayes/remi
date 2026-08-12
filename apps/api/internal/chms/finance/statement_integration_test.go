package finance

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"remi-api/internal/chms/platform"
)

type recordingStatementMailer struct {
	mu       sync.Mutex
	messages []recordedStatementMessage
}

type recordedStatementMessage struct {
	to, subject, filename string
	content               []byte
}

func (m *recordingStatementMailer) SendAttachment(to, subject, _ string, filename string, content []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, recordedStatementMessage{to: to, subject: subject, filename: filename, content: append([]byte(nil), content...)})
	return nil
}

func TestDeterministicStatementsCorrectionsDeliveryAndMemberPrivacy(t *testing.T) {
	ctx, repo, service, database, admin := financeTest(t)
	mailer := &recordingStatementMailer{}
	service.Mailer = mailer
	method, fund := configureOnlineGiving(t, ctx, service, admin)
	now := service.now()

	for _, person := range []bson.M{
		{"_id": "statement-person", "organizationId": "remi", "homeBranchId": "accra", "archivedAt": nil, "names": bson.M{"given": "Ama", "family": "Mensah", "preferred": "Ama"}},
		{"_id": "other-person", "organizationId": "remi", "homeBranchId": "accra", "archivedAt": nil, "names": bson.M{"given": "Kojo", "family": "Mensah"}},
	} {
		if _, err := database.Collection("chms_people").InsertOne(ctx, person); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.Collection("chms_households").InsertOne(ctx, bson.M{"_id": "mensah-household", "organizationId": "remi", "homeBranchId": "accra", "archivedAt": nil, "name": "Mensah household", "primaryContactPersonId": "statement-person", "statementPreference": "household"}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Collection("chms_household_memberships").InsertOne(ctx, bson.M{"_id": "membership-1", "organizationId": "remi", "householdId": "mensah-household", "personId": "statement-person", "role": "primary-contact", "startedAt": now, "createdAt": now}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Collection("chms_member_accounts").InsertOne(ctx, bson.M{"_id": "account-1", "organizationId": "remi", "personId": "statement-person", "email": "ama@example.com", "status": "active", "emailVerifiedAt": now}); err != nil {
		t.Fatal(err)
	}

	post := func(key string, donor DonorAttribution, amount int64) *Contribution {
		value, err := service.PostContribution(ctx, admin, ContributionInput{ReceivedAt: time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC), BranchID: "accra", Donor: donor, Source: "online", PaymentMethodID: method.ID, ProviderReference: "statement-" + key, Total: platform.Money{AmountMinor: amount, Currency: "GHS"}, Splits: []ContributionSplit{{FundID: fund.ID, Amount: platform.Money{AmountMinor: amount, Currency: "GHS"}}}, PostingAction: "post", Provenance: "verified-provider"}, "statement-post-"+key, "statement-key-"+key)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	personGift := post("person", DonorAttribution{Type: "person", PersonID: "statement-person"}, 12_345)
	householdGift := post("household", DonorAttribution{Type: "household", HouseholdID: "mensah-household"}, 50_000)
	_ = post("anonymous", DonorAttribution{Type: "anonymous"}, 99_999)

	input := StatementInput{SubjectType: "person", SubjectID: "statement-person", BranchID: "accra", Year: 2026}
	first, err := service.GenerateStatement(ctx, admin, input, "statement-generate-1")
	if err != nil || first.StatementVersion != 1 || len(first.Rows) != 1 || first.TotalAmountMinor != 12_345 || first.Rows[0].ContributionID != personGift.ID {
		t.Fatalf("first statement=%+v err=%v", first, err)
	}
	firstPDF, _, err := service.StatementPDF(ctx, admin, first.ID)
	if err != nil || !strings.HasPrefix(string(firstPDF), "%PDF-1.4") {
		t.Fatalf("statement pdf prefix=%q err=%v", string(firstPDF[:min(8, len(firstPDF))]), err)
	}
	digest := sha256.Sum256(firstPDF)
	if hex.EncodeToString(digest[:]) != first.ArtifactHash {
		t.Fatal("stored statement artifact hash did not reproduce")
	}
	replayed, err := service.GenerateStatement(ctx, admin, input, "statement-generate-replay")
	if err != nil || replayed.ID != first.ID || replayed.ArtifactHash != first.ArtifactHash {
		t.Fatalf("statement replay=%+v err=%v", replayed, err)
	}

	adjusted, err := service.AdjustContribution(ctx, admin, personGift.ID, AdjustmentInput{Type: "correction", Reason: "Correct the reviewed fund designation for the member statement.", ReplacementSplits: []ContributionSplit{{FundID: fund.ID, Amount: platform.Money{AmountMinor: 12_345, Currency: "GHS"}}}}, "statement-correction", "statement-correction-key")
	if err != nil || adjusted.Replacement == nil {
		t.Fatalf("correction=%+v err=%v", adjusted, err)
	}
	second, err := service.GenerateStatement(ctx, admin, input, "statement-generate-2")
	if err != nil || second.StatementVersion != 2 || second.SupersedesID != first.ID || second.ArtifactHash == first.ArtifactHash || len(second.Rows) != 3 || second.TotalAmountMinor != 12_345 {
		t.Fatalf("second statement=%+v err=%v", second, err)
	}
	storedFirst, err := repo.FindStatement(ctx, "remi", first.ID)
	if err != nil || storedFirst.ArtifactHash != first.ArtifactHash || len(storedFirst.Rows) != 1 {
		t.Fatalf("prior snapshot mutated=%+v err=%v", storedFirst, err)
	}

	receipts, err := service.ListReceipts(ctx, admin, "person", "statement-person", "", 2026)
	if err != nil || len(receipts) != 3 || receipts[0].EffectiveState != "corrected" {
		t.Fatalf("receipts=%+v err=%v", receipts, err)
	}
	member := platform.Principal{Actor: platform.Actor{Type: platform.ActorMember, ID: "statement-person"}, OrganizationID: "remi", Roles: []string{"member"}}
	if _, err = service.GetStatement(ctx, member, second.ID); err != nil {
		t.Fatalf("member could not read own statement: %v", err)
	}
	other := platform.Principal{Actor: platform.Actor{Type: platform.ActorMember, ID: "other-person"}, OrganizationID: "remi", Roles: []string{"member"}}
	if _, err = service.GetStatement(ctx, other, second.ID); err == nil {
		t.Fatal("another member read a private statement")
	}
	householdInput := StatementInput{SubjectType: "household", SubjectID: "mensah-household", Year: 2026}
	householdStatement, err := service.GenerateStatement(ctx, member, householdInput, "household-statement")
	if err != nil || len(householdStatement.Rows) != 1 || householdStatement.Rows[0].ContributionID != householdGift.ID {
		t.Fatalf("household statement=%+v err=%v", householdStatement, err)
	}
	if _, err = service.GenerateStatement(ctx, other, householdInput, "household-statement-denied"); err == nil {
		t.Fatal("non-primary household member generated a household statement")
	}

	delivery, err := service.DeliverStatement(ctx, member, second.ID, "statement-delivery-0001")
	if err != nil || delivery.State != "delivered" || delivery.RecipientHint != "a***@example.com" || len(mailer.messages) != 1 {
		t.Fatalf("delivery=%+v messages=%+v err=%v", delivery, mailer.messages, err)
	}
	replayedDelivery, err := service.DeliverStatement(ctx, member, second.ID, "statement-delivery-0001")
	if err != nil || replayedDelivery.ID != delivery.ID || len(mailer.messages) != 1 {
		t.Fatalf("delivery replay=%+v messages=%d err=%v", replayedDelivery, len(mailer.messages), err)
	}
	receiptPDF, receipt, err := service.ReceiptPDF(ctx, member, personGift.ID)
	if err != nil || receipt.EffectiveState != "corrected" || !strings.HasPrefix(string(receiptPDF), "%PDF-1.4") {
		t.Fatalf("receipt=%+v err=%v", receipt, err)
	}
}

func TestStatementFormattingUsesExactMinorUnits(t *testing.T) {
	values := map[int64]string{0: "0.00", 1: "0.01", -1: "-0.01", 100: "1.00", -12345: "-123.45", int64(^uint64(0) >> 1): "92233720368547758.07"}
	for input, want := range values {
		if got := formatMinor(input); got != want {
			t.Fatalf("formatMinor(%d)=%q want %q", input, got, want)
		}
	}
}

var _ StatementMailer = (*recordingStatementMailer)(nil)
