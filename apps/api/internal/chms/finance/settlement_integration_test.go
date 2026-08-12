package finance

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"remi-api/internal/chms/platform"
)

func TestSettlementImportMatchingVarianceCloseAndReopen(t *testing.T) {
	ctx, _, service, database, admin := financeTest(t)
	method, fund := configureOnlineGiving(t, ctx, service, admin)
	_, _ = database.Collection("chms_people").InsertOne(ctx, bson.M{"_id": "settlement-donor", "organizationId": "remi", "homeBranchId": "accra", "archivedAt": nil})
	donor := DonorAttribution{Type: "person", PersonID: "settlement-donor"}
	post := func(reference, key string, amount int64) *Contribution {
		value, err := service.PostContribution(ctx, admin, ContributionInput{ReceivedAt: time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC), BranchID: "accra", Donor: donor, Source: "online", PaymentMethodID: method.ID, ProviderReference: reference, Total: platform.Money{AmountMinor: amount, Currency: "GHS"}, Splits: []ContributionSplit{{FundID: fund.ID, Amount: platform.Money{AmountMinor: amount, Currency: "GHS"}}}, PostingAction: "post", Provenance: "verified-provider"}, "post-"+key, "settlement-post-"+key)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	exact := post("provider-exact", "exact", 25_000)
	variance := post("provider-variance", "variance", 10_000)
	periods, err := service.ListPeriods(ctx, admin)
	if err != nil || len(periods) != 1 {
		t.Fatalf("periods=%+v err=%v", periods, err)
	}
	period := periods[0]
	input := SettlementInput{BranchID: "accra", SourceType: "paystack", SourceName: "Paystack Ghana", FileName: "settlement-2026-08-10.csv", FileHash: strings.Repeat("a", 64), PrivateAssetID: "remi/finance/settlements/private-a", PeriodID: period.ID, Reference: "PST-2026-08-10", SettledAt: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC), Currency: "GHS", GrossAmountMinor: 34_500, FeeAmountMinor: 500, NetAmountMinor: 34_000, Rows: []SettlementRowInput{{SourceRowID: "row-1", Reference: "provider-exact", OccurredAt: time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC), AmountMinor: 25_000, FeeAmountMinor: 300}, {SourceRowID: "row-2", Reference: "provider-variance", OccurredAt: time.Date(2026, 8, 10, 8, 5, 0, 0, time.UTC), AmountMinor: 9_500, FeeAmountMinor: 200}}}
	settlement, err := service.ImportSettlement(ctx, admin, input, "settlement-import")
	if err != nil {
		t.Fatal(err)
	}
	replay, err := service.ImportSettlement(ctx, admin, input, "settlement-replay")
	if err != nil || replay.ID != settlement.ID {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	items, err := service.ListReconciliationItems(ctx, admin, settlement.ID)
	if err != nil || len(items) != 2 || items[0].Confidence != "exact" || items[0].SuggestedTargetID != exact.ID || items[1].Confidence != "reference" || items[1].SuggestedTargetID != variance.ID {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	if _, err = service.ResolveReconciliationItem(ctx, admin, items[0].ID, ReconciliationResolutionInput{Action: "assign-exception", OwnerID: "missing-owner", Reason: "This exception needs an accountable finance owner."}, "resolve-invalid-owner"); err == nil {
		t.Fatal("inactive exception owner was accepted")
	}
	if _, err = service.ResolveReconciliationItem(ctx, admin, items[0].ID, ReconciliationResolutionInput{Action: "match", TargetType: "contribution", TargetID: exact.ID}, "resolve-exact"); err != nil {
		t.Fatal(err)
	}
	if _, err = service.ResolveReconciliationItem(ctx, admin, items[1].ID, ReconciliationResolutionInput{Action: "match", TargetType: "contribution", TargetID: variance.ID}, "resolve-missing-reason"); err == nil {
		t.Fatal("non-zero variance matched without approval and reason")
	}
	reconciler := platform.Principal{Actor: platform.Actor{Type: platform.ActorStaff, ID: "finance-reconciler"}, OrganizationID: "remi", Roles: []string{"finance-admin"}, Grants: []platform.Grant{{Action: "update", Resource: "finance-config", BranchIDs: []platform.ID{"accra"}, FieldClasses: []platform.FieldClass{platform.FieldFinancial}}}}
	if _, err = service.ResolveReconciliationItem(ctx, reconciler, items[1].ID, ReconciliationResolutionInput{Action: "approve-variance", TargetType: "contribution", TargetID: variance.ID, Reason: "Paystack export records a processor-side GHS 5 adjustment."}, "resolve-variance-without-approval"); err == nil {
		t.Fatal("variance was approved without finance-ledger approval and recent MFA")
	}
	if _, err = service.ResolveReconciliationItem(ctx, admin, items[1].ID, ReconciliationResolutionInput{Action: "approve-variance", TargetType: "contribution", TargetID: variance.ID, Reason: "Paystack export records a processor-side GHS 5 adjustment."}, "resolve-variance"); err != nil {
		t.Fatal(err)
	}
	settlement, err = service.GetSettlement(ctx, admin, settlement.ID)
	if err != nil || settlement.State != "reconciled" || settlement.MatchedGrossMinor != 35_000 || settlement.ApprovedVarianceMinor != -500 || settlement.UnexplainedVarianceMinor != 0 {
		t.Fatalf("reconciled=%+v err=%v", settlement, err)
	}
	request, err := service.ControlPeriod(ctx, admin, period.ID, "close", PeriodControlInput{Action: "request", Reason: "All August settlements and provider exceptions have been reviewed."}, "close-request")
	if err != nil || request.Request == nil {
		t.Fatalf("close request=%+v err=%v", request, err)
	}
	mfa := time.Date(2026, 8, 11, 11, 59, 0, 0, time.UTC)
	approver := platform.Principal{Actor: platform.Actor{Type: platform.ActorStaff, ID: "finance-approver"}, OrganizationID: "remi", Roles: []string{"finance-approver"}, MFAConfirmedAt: &mfa, Grants: []platform.Grant{{Action: "approve", Resource: "finance-ledger", BranchIDs: []platform.ID{"*"}, FieldClasses: []platform.FieldClass{platform.FieldFinancial}}, {Action: "read", Resource: "finance-config", BranchIDs: []platform.ID{"*"}, FieldClasses: []platform.FieldClass{platform.FieldFinancial}}}}
	closed, err := service.ControlPeriod(ctx, approver, period.ID, "close", PeriodControlInput{Action: "approve", RequestID: request.Request.ID, Reason: "Independent review confirms zero unexplained reconciliation variance.", ExpectedVersion: period.Version}, "close-approve")
	if err != nil || closed.Period.Status != "closed" {
		t.Fatalf("close=%+v err=%v", closed, err)
	}
	if _, err = service.PostContribution(ctx, admin, ContributionInput{ReceivedAt: time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC), BranchID: "accra", Donor: donor, Source: "online", PaymentMethodID: method.ID, ProviderReference: "closed-period", Total: platform.Money{AmountMinor: 1_000, Currency: "GHS"}, Splits: []ContributionSplit{{FundID: fund.ID, Amount: platform.Money{AmountMinor: 1_000, Currency: "GHS"}}}, PostingAction: "post"}, "closed-post", "closed-post-key"); err == nil {
		t.Fatal("closed period accepted a backdated contribution")
	}
	closedImport := input
	closedImport.FileHash = strings.Repeat("c", 64)
	closedImport.Reference = "PST-CLOSED-PERIOD"
	if _, err = service.ImportSettlement(ctx, admin, closedImport, "closed-settlement-import"); err == nil {
		t.Fatal("closed period accepted new settlement evidence")
	}
	reopenRequest, err := service.ControlPeriod(ctx, admin, period.ID, "reopen", PeriodControlInput{Action: "request", Reason: "A documented late settlement requires controlled period correction."}, "reopen-request")
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := service.ControlPeriod(ctx, approver, period.ID, "reopen", PeriodControlInput{Action: "approve", RequestID: reopenRequest.Request.ID, Reason: "Independent approval granted after impact review and alerting.", ExpectedVersion: closed.Period.Version}, "reopen-approve")
	if err != nil || reopened.Period.Status != "open" {
		t.Fatalf("reopen=%+v err=%v", reopened, err)
	}
	controls, _ := database.Collection(periodControlEventsCollection).CountDocuments(ctx, bson.M{"organizationId": "remi", "periodId": period.ID})
	if controls != 2 {
		t.Fatalf("period control events=%d", controls)
	}
}

func TestSettlementImportRejectsDuplicateRowsAndUnbalancedFees(t *testing.T) {
	input := SettlementInput{BranchID: "accra", SourceType: "bank", SourceName: "Church bank", FileName: "bank.csv", FileHash: strings.Repeat("b", 64), PrivateAssetID: "private/bank", PeriodID: "period", Reference: "statement", SettledAt: time.Now(), Currency: "GHS", GrossAmountMinor: 100, FeeAmountMinor: 10, NetAmountMinor: 90, Rows: []SettlementRowInput{{SourceRowID: "same", OccurredAt: time.Now(), AmountMinor: 50, FeeAmountMinor: 4}, {SourceRowID: "same", OccurredAt: time.Now(), AmountMinor: 50, FeeAmountMinor: 5}}}
	if err := input.NormalizeAndValidate(); err == nil {
		t.Fatal("duplicate rows and unbalanced row fees were accepted")
	}
	overflow := input
	overflow.GrossAmountMinor = math.MaxInt64
	overflow.FeeAmountMinor = 0
	overflow.NetAmountMinor = math.MaxInt64
	overflow.Rows = []SettlementRowInput{{SourceRowID: "one", OccurredAt: time.Now(), AmountMinor: math.MaxInt64}, {SourceRowID: "two", OccurredAt: time.Now(), AmountMinor: 1}}
	if err := overflow.NormalizeAndValidate(); err == nil || !strings.Contains(err.Error(), "exceed") {
		t.Fatalf("overflow was not rejected explicitly: %v", err)
	}
}

func TestConcurrentSettlementImportReplaysWinningTransaction(t *testing.T) {
	ctx, _, service, _, admin := financeTest(t)
	configureOnlineGiving(t, ctx, service, admin)
	periods, err := service.ListPeriods(ctx, admin)
	if err != nil || len(periods) != 1 {
		t.Fatalf("periods=%+v err=%v", periods, err)
	}
	input := SettlementInput{BranchID: "accra", SourceType: "bank", SourceName: "Church bank", FileName: "concurrent.csv", FileHash: strings.Repeat("d", 64), PrivateAssetID: "private/concurrent", PeriodID: periods[0].ID, Reference: "BANK-CONCURRENT", SettledAt: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC), Currency: "GHS", GrossAmountMinor: 1_000, NetAmountMinor: 1_000, Rows: []SettlementRowInput{{SourceRowID: "row-one", Reference: "bank-one", OccurredAt: time.Date(2026, 8, 10, 11, 0, 0, 0, time.UTC), AmountMinor: 1_000}}}
	const workers = 6
	results := make(chan *Settlement, workers)
	errors := make(chan error, workers)
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			value, importErr := service.ImportSettlement(ctx, admin, input, fmt.Sprintf("concurrent-%d", index))
			results <- value
			errors <- importErr
		}(index)
	}
	wait.Wait()
	close(results)
	close(errors)
	for importErr := range errors {
		if importErr != nil {
			t.Fatalf("concurrent replay failed: %v", importErr)
		}
	}
	var id platform.ID
	for value := range results {
		if value == nil || value.ItemCount != 1 {
			t.Fatalf("replay was not decorated: %+v", value)
		}
		if !id.Valid() {
			id = value.ID
		} else if value.ID != id {
			t.Fatalf("duplicate settlement ids: %s and %s", id, value.ID)
		}
	}
}
