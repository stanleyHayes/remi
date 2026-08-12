package finance

import (
	"bytes"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"remi-api/internal/chms/platform"
)

func TestContributionSplitNormalizationGeneratedProperties(t *testing.T) {
	random := rand.New(rand.NewSource(20260811))
	for example := 0; example < 750; example++ {
		count := 1 + random.Intn(12)
		total := int64(count + random.Intn(5_000_000))
		remaining := total
		splits := make([]ContributionSplit, 0, count)
		for index := 0; index < count; index++ {
			amount := remaining
			if index < count-1 {
				minimumAfter := int64(count - index - 1)
				amount = 1 + random.Int63n(remaining-minimumAfter)
			}
			remaining -= amount
			splits = append(splits, ContributionSplit{FundID: platform.ID("fund-" + string(rune('A'+index))), Amount: platform.Money{AmountMinor: amount, Currency: "GHS"}})
		}
		balanced := append([]ContributionSplit(nil), splits...)
		if err := normalizeContributionSplits(balanced, "GHS", total, false); err != nil {
			t.Fatalf("balanced example %d rejected: total=%d splits=%+v err=%v", example, total, splits, err)
		}
		unbalanced := append([]ContributionSplit(nil), splits...)
		unbalanced[len(unbalanced)-1].Amount.AmountMinor++
		if err := normalizeContributionSplits(unbalanced, "GHS", total, false); err == nil {
			t.Fatalf("unbalanced example %d accepted", example)
		}
		wrongCurrency := append([]ContributionSplit(nil), splits...)
		wrongCurrency[random.Intn(len(wrongCurrency))].Amount.Currency = "USD"
		if err := normalizeContributionSplits(wrongCurrency, "GHS", total, false); err == nil {
			t.Fatalf("mixed-currency example %d accepted", example)
		}
		if len(splits) > 1 {
			duplicate := append([]ContributionSplit(nil), splits...)
			duplicate[1].FundID = duplicate[0].FundID
			if err := normalizeContributionSplits(duplicate, "GHS", total, false); err == nil {
				t.Fatalf("duplicate fund example %d accepted", example)
			}
		}
	}
}

func TestGeneratedLedgerMatrixPreservesImmutabilityAndReportReconciliation(t *testing.T) {
	ctx, repository, service, db, principal := financeTest(t)
	startsAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	endsAt := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := service.SaveCampus(ctx, principal, "", CampusSettingsInput{BranchID: "accra", Name: "Accra Campus", Timezone: "Africa/Accra", Currency: "GHS"}, "matrix-campus"); err != nil {
		t.Fatal(err)
	}
	method, err := service.SavePaymentMethod(ctx, principal, "", PaymentMethodInput{Code: "CARD", Name: "Card", Kind: "card", Provider: "Paystack", Active: true}, "matrix-method")
	if err != nil {
		t.Fatal(err)
	}
	funds := make([]Fund, 4)
	for index := range funds {
		value, saveErr := service.SaveFund(ctx, principal, "", FundInput{Code: "FUND_" + string(rune('A'+index)), Name: "Generated fund " + string(rune('A'+index)), RestrictionType: "unrestricted", ActiveFrom: startsAt}, "matrix-fund")
		if saveErr != nil {
			t.Fatal(saveErr)
		}
		funds[index] = *value
	}
	period, err := service.SavePeriod(ctx, principal, "", FiscalPeriodInput{Code: "MATRIX2026", Name: "Generated matrix year", StartsAt: startsAt, EndsAt: endsAt}, "matrix-period")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.SaveSequence(ctx, principal, "", ReceiptSequenceInput{Code: "MATRIXSEQ", Prefix: "MATRIX", FiscalYear: 2026, Padding: 6, StartingNumber: 1}, "matrix-sequence"); err != nil {
		t.Fatal(err)
	}
	random := rand.New(rand.NewSource(259))
	expectedNet := int64(0)
	expectedFunds := map[platform.ID]int64{}
	corrections := 0
	const giftCount = 32
	for index := 0; index < giftCount; index++ {
		total := int64(100 + random.Intn(250_000))
		splitCount := 1 + random.Intn(len(funds))
		permutation := random.Perm(len(funds))[:splitCount]
		remaining := total
		splits := make([]ContributionSplit, 0, splitCount)
		for splitIndex, fundIndex := range permutation {
			amount := remaining
			if splitIndex < splitCount-1 {
				minimumAfter := int64(splitCount - splitIndex - 1)
				amount = 1 + random.Int63n(remaining-minimumAfter)
			}
			remaining -= amount
			splits = append(splits, ContributionSplit{FundID: funds[fundIndex].ID, Amount: platform.Money{AmountMinor: amount, Currency: "GHS"}})
			expectedFunds[funds[fundIndex].ID] += amount
		}
		input := ContributionInput{ReceivedAt: time.Date(2026, time.Month(2+(index%7)), 1+(index%25), 9, 0, 0, 0, time.UTC), BranchID: "accra", Donor: DonorAttribution{Type: "anonymous"}, Source: "online", PaymentMethodID: method.ID, ProviderReference: fmt.Sprintf("matrix-provider-%03d", index), Total: platform.Money{AmountMinor: total, Currency: "GHS"}, Splits: splits, PostingAction: "post", Provenance: "generated-property-matrix"}
		command := fmt.Sprintf("matrix-post-command-%03d", index)
		posted, postErr := service.PostContribution(ctx, principal, input, "matrix-post", command)
		if postErr != nil {
			t.Fatalf("post %d: %#v", index, postErr)
		}
		replayed, replayErr := service.PostContribution(ctx, principal, input, "matrix-replay", command)
		if replayErr != nil || replayed.ID != posted.ID {
			t.Fatalf("post replay %d drifted: first=%s replay=%v err=%v", index, posted.ID, replayed, replayErr)
		}
		expectedNet += total
		before, _ := repository.FindContribution(ctx, principal.OrganizationID, posted.ID)
		beforeBytes, _ := bson.Marshal(before)
		if index%4 == 0 {
			corrections++
			for _, split := range splits {
				expectedFunds[split.FundID] -= split.Amount.AmountMinor
			}
			replacementFund := funds[(permutation[0]+1)%len(funds)].ID
			expectedFunds[replacementFund] += total
			adjustment := AdjustmentInput{Type: "correction", Reason: "Generated property correction with reviewed fund evidence", ReplacementSplits: []ContributionSplit{{FundID: replacementFund, Amount: platform.Money{AmountMinor: total, Currency: "GHS"}}}}
			adjustmentKey := fmt.Sprintf("matrix-adjustment-%03d", index)
			first, adjustErr := service.AdjustContribution(ctx, principal, posted.ID, adjustment, "matrix-adjust", adjustmentKey)
			if adjustErr != nil || first.Replacement == nil {
				t.Fatalf("adjustment %d: result=%+v err=%v", index, first, adjustErr)
			}
			again, replayAdjustErr := service.AdjustContribution(ctx, principal, posted.ID, adjustment, "matrix-adjust-replay", adjustmentKey)
			if replayAdjustErr != nil || again.Reversal.ID != first.Reversal.ID || again.Replacement == nil || again.Replacement.ID != first.Replacement.ID {
				t.Fatalf("adjustment replay %d drifted: first=%+v replay=%+v err=%v", index, first, again, replayAdjustErr)
			}
		}
		after, _ := repository.FindContribution(ctx, principal.OrganizationID, posted.ID)
		afterBytes, _ := bson.Marshal(after)
		if !bytes.Equal(beforeBytes, afterBytes) {
			t.Fatalf("posted original %d mutated after replay/correction", index)
		}
	}
	report, err := service.BuildFinanceReport(ctx, principal, FinanceReportInput{BranchID: "accra", PeriodID: period.ID})
	if err != nil {
		t.Fatal(err)
	}
	if report.Totals.NetContributionMinor != expectedNet || report.Totals.ContributionCount != giftCount+corrections*2 || report.Totals.AdjustmentCount != corrections*2 {
		t.Fatalf("report totals diverged: got=%+v expectedNet=%d gifts=%d corrections=%d", report.Totals, expectedNet, giftCount, corrections)
	}
	for _, row := range report.FundMovement {
		if row.AmountMinor != expectedFunds[row.FundID] {
			t.Fatalf("fund %s movement=%d expected=%d", row.FundID, row.AmountMinor, expectedFunds[row.FundID])
		}
		delete(expectedFunds, row.FundID)
	}
	for fundID, amount := range expectedFunds {
		if amount != 0 {
			t.Fatalf("fund %s expected movement %d missing from report", fundID, amount)
		}
	}
	ledgerRows, err := repository.ReportContributions(ctx, principal.OrganizationID, "accra", startsAt, endsAt)
	if err != nil {
		t.Fatal(err)
	}
	ledgerTotal := int64(0)
	for _, row := range ledgerRows {
		ledgerTotal += row.Total.AmountMinor
	}
	if ledgerTotal != report.Totals.NetContributionMinor {
		t.Fatalf("report=%d ledger=%d", report.Totals.NetContributionMinor, ledgerTotal)
	}
	readOnly := principal
	readOnly.Actor.ID = "matrix-auditor"
	readOnly.Grants = []platform.Grant{{Action: "read", Resource: "finance-ledger", BranchIDs: []platform.ID{"accra"}, FieldClasses: []platform.FieldClass{platform.FieldFinancial}}}
	if _, err = service.BuildFinanceReport(ctx, readOnly, FinanceReportInput{BranchID: "accra", PeriodID: period.ID}); err != nil {
		t.Fatalf("finance auditor could not read aggregate report: %v", err)
	}
	if _, err = service.CreateFinanceExport(ctx, readOnly, FinanceExportInput{Kind: "accounting-csv", FinanceReportInput: FinanceReportInput{BranchID: "accra", PeriodID: period.ID}}, "matrix-auditor-export"); err == nil {
		t.Fatal("read-only finance auditor exported data")
	}
	wrongBranch := readOnly
	wrongBranch.Grants[0].BranchIDs = []platform.ID{"kumasi"}
	if _, err = service.BuildFinanceReport(ctx, wrongBranch, FinanceReportInput{BranchID: "accra", PeriodID: period.ID}); err == nil {
		t.Fatal("cross-branch finance report was disclosed")
	}
	withoutMFA := principal
	withoutMFA.MFAConfirmedAt = nil
	if _, err = service.CreateFinanceExport(ctx, withoutMFA, FinanceExportInput{Kind: "accounting-csv", FinanceReportInput: FinanceReportInput{BranchID: "accra", PeriodID: period.ID}}, "matrix-no-mfa"); err == nil {
		t.Fatal("finance export succeeded without recent MFA")
	}
	if count, _ := db.Collection(contributionsCollection).CountDocuments(ctx, bson.M{"organizationId": principal.OrganizationID}); count != int64(giftCount+corrections*2) {
		t.Fatalf("unexpected contribution count=%d", count)
	}
}
