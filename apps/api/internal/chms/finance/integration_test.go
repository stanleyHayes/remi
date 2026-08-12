package finance

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

func financeTest(t *testing.T) (context.Context, *Repository, Service, *mongo.Database, platform.Principal) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	client, err := mongo.Connect(options.Client().ApplyURI(testsupport.MongoURI()))
	if err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	if err = client.Ping(ctx, nil); err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	db := client.Database("remi_finance_test_" + bson.NewObjectID().Hex())
	t.Cleanup(func() {
		c, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_ = db.Drop(c)
		_ = client.Disconnect(c)
	})
	repo, _ := NewRepository(db)
	if err = repo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	store, _ := platform.NewMongoPlatformStore(db)
	if err = store.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	service := Service{Repository: repo, Platform: store, Authorizer: platform.GrantAuthorizer{}, Now: func() time.Time { return time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC) }}
	mfa := time.Date(2026, 8, 11, 11, 58, 0, 0, time.UTC)
	principal := platform.Principal{Actor: platform.Actor{Type: platform.ActorStaff, ID: "finance-admin"}, OrganizationID: "remi", Roles: []string{"super-admin"}, MFAConfirmedAt: &mfa, Grants: []platform.Grant{{Action: "*", Resource: "finance-config", BranchIDs: []platform.ID{"*"}, FieldClasses: []platform.FieldClass{platform.FieldFinancial}}, {Action: "*", Resource: "finance-ledger", BranchIDs: []platform.ID{"*"}, FieldClasses: []platform.FieldClass{platform.FieldFinancial}}, {Action: "*", Resource: "finance-batch", BranchIDs: []platform.ID{"*"}, FieldClasses: []platform.FieldClass{platform.FieldFinancial}}}}
	return ctx, repo, service, db, principal
}

func TestDualControlCountingBatchVarianceApprovalAndAtomicPosting(t *testing.T) {
	ctx, repo, service, db, admin := financeTest(t)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, user := range []bson.M{{"_id": "counter-1", "role": "finance-counter", "invitationStatus": "accepted"}, {"_id": "counter-2", "role": "finance-counter", "invitationStatus": "accepted"}, {"_id": "approver-1", "role": "finance-approver", "invitationStatus": "accepted"}} {
		_, _ = db.Collection("users").InsertOne(ctx, user)
	}
	_, _ = db.Collection("chms_people").InsertOne(ctx, bson.M{"_id": "cash-donor", "organizationId": "remi", "homeBranchId": "accra", "archivedAt": nil})
	_, err := service.SaveCampus(ctx, admin, "", CampusSettingsInput{BranchID: "accra", Name: "Accra Campus", Timezone: "Africa/Accra", Currency: "GHS"}, "campus")
	if err != nil {
		t.Fatal(err)
	}
	method, err := service.SavePaymentMethod(ctx, admin, "", PaymentMethodInput{Code: "CASH", Name: "Cash", Kind: "cash", Active: true}, "method")
	if err != nil {
		t.Fatal(err)
	}
	fund, err := service.SaveFund(ctx, admin, "", FundInput{Code: "OFFERING", Name: "General Offering", RestrictionType: "unrestricted", ActiveFrom: start}, "fund")
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.SavePeriod(ctx, admin, "", FiscalPeriodInput{Code: "FY2026", Name: "2026 fiscal year", StartsAt: start, EndsAt: end}, "period")
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.SaveSequence(ctx, admin, "", ReceiptSequenceInput{Code: "MAIN2026", Prefix: "REMI", FiscalYear: 2026, Padding: 6, StartingNumber: 1}, "sequence")
	if err != nil {
		t.Fatal(err)
	}
	batch, err := service.CreateBatch(ctx, admin, CountingBatchInput{BranchID: "accra", ReceivedAt: time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC), CounterIDs: []platform.ID{"counter-1", "counter-2"}, DualControlRequired: true, ExpectedPaymentMethodIDs: []platform.ID{method.ID}}, "batch-create")
	if err != nil {
		t.Fatal(err)
	}
	counterGrant := []platform.Grant{{Action: "read", Resource: "finance-batch", BranchIDs: []platform.ID{"accra"}, FieldClasses: []platform.FieldClass{platform.FieldFinancial}}, {Action: "operate", Resource: "finance-batch", BranchIDs: []platform.ID{"accra"}, FieldClasses: []platform.FieldClass{platform.FieldFinancial}}}
	counter1 := platform.Principal{Actor: platform.Actor{Type: platform.ActorStaff, ID: "counter-1"}, OrganizationID: "remi", Roles: []string{"finance-counter"}, Grants: counterGrant}
	counter2 := platform.Principal{Actor: platform.Actor{Type: platform.ActorStaff, ID: "counter-2"}, OrganizationID: "remi", Roles: []string{"finance-counter"}, Grants: counterGrant}
	batch, err = service.StartBatch(ctx, counter1, batch.ID, BatchTransitionInput{ExpectedVersion: 1}, "start")
	if err != nil || batch.State != "counting" {
		t.Fatalf("start=%+v err=%v", batch, err)
	}
	entryInput := BatchEntryInput{Donor: DonorAttribution{Type: "person", PersonID: "cash-donor"}, Source: "cash", PaymentMethodID: method.ID, Total: platform.Money{AmountMinor: 10000, Currency: "GHS"}, Splits: []ContributionSplit{{FundID: fund.ID, Amount: platform.Money{AmountMinor: 10000, Currency: "GHS"}}}, Provenance: "sunday-count"}
	entry, err := service.SaveBatchEntry(ctx, counter1, batch.ID, "", entryInput, "entry")
	if err != nil {
		t.Fatal(err)
	}
	confirmation := CountConfirmationInput{ExpectedVersion: 3, TenderTotals: []TenderTotal{{PaymentMethodID: method.ID, Amount: platform.Money{AmountMinor: 9000, Currency: "GHS"}}}, Denominations: []DenominationCount{{PaymentMethodID: method.ID, ValueMinor: 5000, Count: 1}, {PaymentMethodID: method.ID, ValueMinor: 2000, Count: 2}}, Attachments: []PrivateAttachment{{PublicID: "remi/finance/batches/count-sheet-1", ResourceType: "image", Label: "Signed count sheet"}}}
	batch, err = service.ConfirmBatchCount(ctx, counter1, batch.ID, confirmation, "confirm-1")
	if err != nil || batch.State != "counting" || batch.ConfirmationCount != 1 {
		t.Fatalf("first confirmation=%+v err=%v", batch, err)
	}
	entryInput.ExpectedVersion = entry.Version
	if _, err = service.SaveBatchEntry(ctx, counter1, batch.ID, entry.ID, entryInput, "locked-entry"); err == nil {
		t.Fatal("entry changed after first independent confirmation")
	}
	mismatch := confirmation
	mismatch.ExpectedVersion = 4
	mismatch.TenderTotals = []TenderTotal{{PaymentMethodID: method.ID, Amount: platform.Money{AmountMinor: 8000, Currency: "GHS"}}}
	mismatch.Denominations = []DenominationCount{{PaymentMethodID: method.ID, ValueMinor: 2000, Count: 4}}
	if _, err = service.ConfirmBatchCount(ctx, counter2, batch.ID, mismatch, "mismatch"); err == nil {
		t.Fatal("mismatched independent count was accepted")
	}
	confirmation.ExpectedVersion = 4
	batch, err = service.ConfirmBatchCount(ctx, counter2, batch.ID, confirmation, "confirm-2")
	if err != nil || batch.State != "counted" || batch.Variance.AmountMinor != 1000 {
		t.Fatalf("counted=%+v err=%v", batch, err)
	}
	if _, err = service.ApproveBatch(ctx, counter1, batch.ID, BatchTransitionInput{ExpectedVersion: 5, Reason: "counter cannot approve"}, "self-approve"); err == nil {
		t.Fatal("counter approved own batch")
	}
	mfa := time.Date(2026, 8, 11, 11, 59, 0, 0, time.UTC)
	approver := platform.Principal{Actor: platform.Actor{Type: platform.ActorStaff, ID: "approver-1"}, OrganizationID: "remi", Roles: []string{"finance-approver"}, MFAConfirmedAt: &mfa, Grants: []platform.Grant{{Action: "read", Resource: "finance-batch", BranchIDs: []platform.ID{"accra"}, FieldClasses: []platform.FieldClass{platform.FieldFinancial}}, {Action: "approve", Resource: "finance-batch", BranchIDs: []platform.ID{"accra"}, FieldClasses: []platform.FieldClass{platform.FieldFinancial}}}}
	if _, err = service.ApproveBatch(ctx, approver, batch.ID, BatchTransitionInput{ExpectedVersion: 5}, "approve-no-reason"); err == nil {
		t.Fatal("variance approved without reason")
	}
	batch, err = service.ApproveBatch(ctx, approver, batch.ID, BatchTransitionInput{ExpectedVersion: 5, Reason: "physical count reviewed and documented as GHS 10 short"}, "approve")
	if err != nil || batch.State != "approved" {
		t.Fatalf("approved=%+v err=%v", batch, err)
	}
	batch, err = service.PostBatch(ctx, approver, batch.ID, BatchTransitionInput{ExpectedVersion: 6}, "post")
	if err != nil || batch.State != "posted" {
		t.Fatalf("posted=%+v err=%v", batch, err)
	}
	contributions, err := repo.ListContributions(ctx, "remi", "accra", 20)
	if err != nil || len(contributions) != 1 || contributions[0].BatchID != batch.ID || contributions[0].ReceiptNumber != "REMI-2026-000001" {
		t.Fatalf("contributions=%+v err=%v", contributions, err)
	}
	if _, err = service.PostBatch(ctx, approver, batch.ID, BatchTransitionInput{ExpectedVersion: 6}, "duplicate-post"); err == nil {
		t.Fatal("posted batch was posted twice")
	}
	detail, err := service.GetBatch(ctx, counter1, batch.ID)
	if err != nil || len(detail.Confirmations) != 2 || len(detail.Events) != 7 {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
	otherCounter := counter1
	otherCounter.Actor.ID = "other-counter"
	if _, err = service.GetBatch(ctx, otherCounter, batch.ID); err == nil {
		t.Fatal("unassigned counter read batch donor detail")
	}
	outbox, _ := db.Collection("chms_outbox").CountDocuments(ctx, bson.M{"type": "finance.contribution.posted"})
	if outbox != 1 {
		t.Fatalf("outbox=%d", outbox)
	}
}

func TestImmutableContributionPostingIdempotencyAndCorrection(t *testing.T) {
	ctx, repo, service, db, p := financeTest(t)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	_, _ = db.Collection("chms_people").InsertOne(ctx, bson.M{"_id": "person-1", "organizationId": "remi", "homeBranchId": "accra", "archivedAt": nil})
	if _, err := service.SaveCampus(ctx, p, "", CampusSettingsInput{BranchID: "accra", Name: "Accra Campus", Timezone: "Africa/Accra", Currency: "GHS"}, "campus"); err != nil {
		t.Fatal(err)
	}
	method, err := service.SavePaymentMethod(ctx, p, "", PaymentMethodInput{Code: "CARD", Name: "Card", Kind: "card", Provider: "Paystack", Active: true}, "method")
	if err != nil {
		t.Fatal(err)
	}
	fund, err := service.SaveFund(ctx, p, "", FundInput{Code: "TITHE", Name: "Tithes", RestrictionType: "unrestricted", ActiveFrom: start}, "fund")
	if err != nil {
		t.Fatal(err)
	}
	missions, err := service.SaveFund(ctx, p, "", FundInput{Code: "MISSIONS", Name: "Missions", RestrictionType: "temporarily-restricted", ActiveFrom: start}, "missions")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.SavePeriod(ctx, p, "", FiscalPeriodInput{Code: "FY2026", Name: "2026 fiscal year", StartsAt: start, EndsAt: end}, "period"); err != nil {
		t.Fatal(err)
	}
	if _, err = service.SaveSequence(ctx, p, "", ReceiptSequenceInput{Code: "MAIN2026", Prefix: "REMI", FiscalYear: 2026, Padding: 6, StartingNumber: 1}, "sequence"); err != nil {
		t.Fatal(err)
	}
	input := ContributionInput{ReceivedAt: time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC), BranchID: "accra", Donor: DonorAttribution{Type: "person", PersonID: "person-1"}, Source: "online", PaymentMethodID: method.ID, ProviderReference: "paystack-ref-1", Total: platform.Money{AmountMinor: 10000, Currency: "GHS"}, Splits: []ContributionSplit{{FundID: fund.ID, Amount: platform.Money{AmountMinor: 7000, Currency: "GHS"}}, {FundID: missions.ID, Amount: platform.Money{AmountMinor: 3000, Currency: "GHS"}}}, PostingAction: "post", Provenance: "paystack-webhook"}
	posted, err := service.PostContribution(ctx, p, input, "post-request", "post-command-0001")
	if err != nil || posted.ReceiptNumber != "REMI-2026-000001" || posted.State != "posted" {
		t.Fatalf("posted=%+v err=%v", posted, err)
	}
	replayed, err := service.PostContribution(ctx, p, input, "post-replay", "post-command-0001")
	if err != nil || replayed.ID != posted.ID || replayed.ReceiptNumber != posted.ReceiptNumber {
		t.Fatalf("replay=%+v err=%v", replayed, err)
	}
	changed := input
	changed.Total.AmountMinor = 11000
	changed.Splits = []ContributionSplit{{FundID: fund.ID, Amount: platform.Money{AmountMinor: 11000, Currency: "GHS"}}}
	if _, err = service.PostContribution(ctx, p, changed, "reuse", "post-command-0001"); err == nil {
		t.Fatal("idempotency key reuse with changed payload was accepted")
	}
	unbalanced := input
	unbalanced.ProviderReference = "paystack-ref-2"
	unbalanced.Splits = []ContributionSplit{{FundID: fund.ID, Amount: platform.Money{AmountMinor: 9000, Currency: "GHS"}}}
	if _, err = service.PostContribution(ctx, p, unbalanced, "unbalanced", "post-command-0002"); err == nil {
		t.Fatal("unbalanced split was accepted")
	}
	adjustment := AdjustmentInput{Type: "correction", Reason: "correct donor designation after reviewed request", ReplacementSplits: []ContributionSplit{{FundID: missions.ID, Amount: platform.Money{AmountMinor: 10000, Currency: "GHS"}}}}
	result, err := service.AdjustContribution(ctx, p, posted.ID, adjustment, "adjust-request", "adjust-command-0001")
	if err != nil || result.Reversal.Total.AmountMinor != -10000 || result.Replacement == nil || result.Replacement.Total.AmountMinor != 10000 {
		t.Fatalf("adjustment=%+v err=%v", result, err)
	}
	original, err := repo.FindContribution(ctx, p.OrganizationID, posted.ID)
	if err != nil || original.Total.AmountMinor != 10000 || len(original.Splits) != 2 || original.Version != 1 {
		t.Fatalf("original mutated: %+v err=%v", original, err)
	}
	detail, err := service.GetContribution(ctx, p, posted.ID)
	if err != nil || detail.EffectiveState != "corrected" {
		t.Fatalf("effective detail=%+v err=%v", detail, err)
	}
	replayedAdjustment, err := service.AdjustContribution(ctx, p, posted.ID, adjustment, "adjust-replay", "adjust-command-0001")
	if err != nil || replayedAdjustment.Reversal.ID != result.Reversal.ID {
		t.Fatalf("adjustment replay=%+v err=%v", replayedAdjustment, err)
	}
	if _, err = service.AdjustContribution(ctx, p, posted.ID, AdjustmentInput{Type: "reversal", Reason: "second reversal must not be possible"}, "second-adjust", "adjust-command-0002"); err == nil {
		t.Fatal("second adjustment was accepted")
	}
	items, err := service.ListContributions(ctx, p, "accra", 20)
	if err != nil || len(items) != 3 {
		t.Fatalf("items=%d err=%v", len(items), err)
	}
	allocations, _ := db.Collection(receiptAllocationsCollection).CountDocuments(ctx, bson.M{})
	if allocations != 3 {
		t.Fatalf("allocations=%d want 3", allocations)
	}
	outbox, _ := db.Collection("chms_outbox").CountDocuments(ctx, bson.M{"organizationId": "remi"})
	if outbox != 2 {
		t.Fatalf("outbox=%d want 2", outbox)
	}
}

func TestFinanceFoundationLifecycleAndInvariants(t *testing.T) {
	ctx, repo, service, db, p := financeTest(t)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	fund, err := service.SaveFund(ctx, p, "", FundInput{Code: "TITHE", Name: "Tithes", RestrictionType: "unrestricted", ActiveFrom: start}, "fund-create")
	if err != nil || fund.Version != 1 {
		t.Fatalf("fund=%+v err=%v", fund, err)
	}
	if _, err = service.SavePaymentMethod(ctx, p, "", PaymentMethodInput{Code: "MOMO", Name: "Mobile money", Kind: "mobile-money", Provider: "Paystack", Active: true}, "method-create"); err != nil {
		t.Fatal(err)
	}
	campus, err := service.SaveCampus(ctx, p, "", CampusSettingsInput{BranchID: "accra", Name: "Accra Campus", Timezone: "Africa/Accra", Currency: "GHS"}, "campus-create")
	if err != nil || campus.BranchID != "accra" {
		t.Fatalf("campus=%+v err=%v", campus, err)
	}
	period, err := service.SavePeriod(ctx, p, "", FiscalPeriodInput{Code: "FY2026", Name: "2026 fiscal year", StartsAt: start, EndsAt: end}, "period-create")
	if err != nil || period.Status != "open" {
		t.Fatalf("period=%+v err=%v", period, err)
	}
	if _, err = service.SavePeriod(ctx, p, "", FiscalPeriodInput{Code: "OVERLAP", Name: "Overlapping year", StartsAt: start.AddDate(0, 6, 0), EndsAt: end.AddDate(0, 6, 0)}, "overlap"); err == nil {
		t.Fatal("overlapping fiscal period was accepted")
	}
	sequence, err := service.SaveSequence(ctx, p, "", ReceiptSequenceInput{Code: "MAIN2026", Prefix: "REMI", FiscalYear: 2026, Padding: 6, StartingNumber: 1}, "sequence-create")
	if err != nil {
		t.Fatal(err)
	}
	mapping, err := service.SaveMapping(ctx, p, "", AccountMappingInput{BranchID: "accra", FundID: fund.ID, Category: "contribution", ExternalAccountCode: "4100", ExternalDimensionCode: "ACCRA", EffectiveFrom: start}, "mapping-create")
	if err != nil || mapping.FundID != fund.ID {
		t.Fatalf("mapping=%+v err=%v", mapping, err)
	}
	fund.Name = "Tithes and offerings"
	updated, err := service.SaveFund(ctx, p, fund.ID, FundInput{Code: fund.Code, Name: fund.Name, RestrictionType: fund.RestrictionType, ActiveFrom: fund.ActiveFrom, ExpectedVersion: 1}, "fund-update")
	if err != nil || updated.Version != 2 {
		t.Fatalf("updated=%+v err=%v", updated, err)
	}
	if _, err = service.SaveFund(ctx, p, fund.ID, FundInput{Code: fund.Code, Name: "Stale update", RestrictionType: fund.RestrictionType, ActiveFrom: fund.ActiveFrom, ExpectedVersion: 1}, "stale"); err == nil {
		t.Fatal("stale update was accepted")
	}
	deactivated, err := service.DeactivateFund(ctx, p, fund.ID, DeactivateFundInput{ExpectedVersion: 2, Reason: "replaced by approved annual fund"}, "fund-deactivate")
	if err != nil || deactivated.ActiveUntil == nil || deactivated.Version != 3 {
		t.Fatalf("deactivated=%+v err=%v", deactivated, err)
	}
	viewer := platform.Principal{Actor: platform.Actor{Type: platform.ActorStaff, ID: "viewer"}, OrganizationID: "remi", Grants: []platform.Grant{{Action: "read", Resource: "finance-config", BranchIDs: []platform.ID{"*"}, FieldClasses: []platform.FieldClass{platform.FieldOperational}}}}
	if _, err = service.ListFunds(ctx, viewer); err == nil {
		t.Fatal("non-finance viewer read configuration")
	}
	first, err := repo.ReserveReceipt(ctx, p.OrganizationID, sequence.ID, time.Now().UTC(), p.Actor)
	if err != nil || first != "REMI-2026-000001" {
		t.Fatalf("receipt=%s err=%v", first, err)
	}
	second, err := repo.ReserveReceipt(ctx, p.OrganizationID, sequence.ID, time.Now().UTC(), p.Actor)
	if err != nil || second != "REMI-2026-000002" {
		t.Fatalf("second receipt=%s err=%v", second, err)
	}
	audits, _ := db.Collection("chms_audit_events").CountDocuments(ctx, bson.M{"organizationId": "remi"})
	if audits != 8 {
		t.Fatalf("audits=%d want 8", audits)
	}
}

func TestReceiptSequenceConcurrentAllocationIsUnique(t *testing.T) {
	ctx, repo, service, _, p := financeTest(t)
	sequence, err := service.SaveSequence(ctx, p, "", ReceiptSequenceInput{Code: "RACE2026", Prefix: "REMI", FiscalYear: 2026, Padding: 6, StartingNumber: 1}, "sequence-create")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	values := map[string]bool{}
	successes := 0
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			number, e := repo.ReserveReceipt(context.Background(), p.OrganizationID, sequence.ID, time.Now().UTC(), p.Actor)
			if e == nil {
				mu.Lock()
				values[number] = true
				successes++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if successes != 12 || len(values) != 12 {
		t.Fatalf("successes=%d unique=%d", successes, len(values))
	}
	current, _ := repo.FindReceiptSequence(ctx, p.OrganizationID, sequence.ID)
	if current.NextNumber != int64(successes+1) {
		t.Fatalf("next=%d successes=%d", current.NextNumber, successes)
	}
}
