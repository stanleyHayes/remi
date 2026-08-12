package finance

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"io"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"remi-api/internal/chms/platform"
)

func TestFinanceReportReconcilesLedgerAndPersistsPrivateExports(t *testing.T) {
	ctx, _, service, db, principal := financeTest(t)
	startsAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	endsAt := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	_, _ = db.Collection("chms_people").InsertOne(ctx, bson.M{"_id": "report-person", "organizationId": "remi", "homeBranchId": "accra", "archivedAt": nil})
	if _, err := service.SaveCampus(ctx, principal, "", CampusSettingsInput{BranchID: "accra", Name: "Accra Campus", Timezone: "Africa/Accra", Currency: "GHS"}, "report-campus"); err != nil {
		t.Fatal(err)
	}
	method, err := service.SavePaymentMethod(ctx, principal, "", PaymentMethodInput{Code: "CARD", Name: "Card", Kind: "card", Provider: "Paystack", Active: true}, "report-method")
	if err != nil {
		t.Fatal(err)
	}
	fund, err := service.SaveFund(ctx, principal, "", FundInput{Code: "GENERAL", Name: "General fund", RestrictionType: "unrestricted", ActiveFrom: startsAt}, "report-fund")
	if err != nil {
		t.Fatal(err)
	}
	period, err := service.SavePeriod(ctx, principal, "", FiscalPeriodInput{Code: "FY2026", Name: "2026 fiscal year", StartsAt: startsAt, EndsAt: endsAt}, "report-period")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.SaveSequence(ctx, principal, "", ReceiptSequenceInput{Code: "REPORT2026", Prefix: "RPT", FiscalYear: 2026, Padding: 6, StartingNumber: 1}, "report-sequence"); err != nil {
		t.Fatal(err)
	}
	if _, err = service.SaveMapping(ctx, principal, "", AccountMappingInput{BranchID: "accra", FundID: fund.ID, Category: "contribution", ExternalAccountCode: "=unsafe-ledger-code", ExternalDimensionCode: "+unsafe-dimension", EffectiveFrom: startsAt}, "report-mapping"); err != nil {
		t.Fatal(err)
	}
	posted, err := service.PostContribution(ctx, principal, ContributionInput{ReceivedAt: time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC), BranchID: "accra", Donor: DonorAttribution{Type: "person", PersonID: "report-person"}, Source: "online", PaymentMethodID: method.ID, ProviderReference: "report-provider-reference", Total: platform.Money{AmountMinor: 12345, Currency: "GHS"}, Splits: []ContributionSplit{{FundID: fund.ID, Amount: platform.Money{AmountMinor: 12345, Currency: "GHS"}}}, PostingAction: "post", Provenance: "report-test"}, "report-post", "report-post-command")
	if err != nil {
		t.Fatal(err)
	}
	if posted.ReceiptNumber == "" {
		t.Fatal("posted contribution did not allocate a receipt")
	}
	report, err := service.BuildFinanceReport(ctx, principal, FinanceReportInput{BranchID: "accra", PeriodID: period.ID})
	if err != nil {
		t.Fatal(err)
	}
	if report.Totals.NetContributionMinor != 12345 || report.Totals.ContributionCount != 1 || len(report.FundMovement) != 1 || report.FundMovement[0].AmountMinor != 12345 || len(report.GivingTrend) != 1 {
		t.Fatalf("report did not reconcile to ledger: %+v", report)
	}
	if report.Meta.MetricVersion == "" || report.Meta.SourceWatermark == "" || report.Meta.Timezone != "Africa/Accra" {
		t.Fatalf("report provenance missing: %+v", report.Meta)
	}
	firstWatermark := report.Meta.SourceWatermark
	replayed, err := service.BuildFinanceReport(ctx, principal, FinanceReportInput{BranchID: "accra", PeriodID: period.ID})
	if err != nil || replayed.Meta.SourceWatermark != firstWatermark || replayed.Totals != report.Totals {
		t.Fatalf("report regeneration drifted: first=%+v replay=%+v err=%v", report.Meta, replayed.Meta, err)
	}
	csvRun, err := service.CreateFinanceExport(ctx, principal, FinanceExportInput{Kind: "accounting-csv", FinanceReportInput: FinanceReportInput{BranchID: "accra", PeriodID: period.ID}}, "report-csv")
	if err != nil {
		t.Fatal(err)
	}
	if csvRun.Status != "completed" || csvRun.RowCount != 1 || csvRun.TotalAmountMinor != 12345 || len(csvRun.ArtifactHash) != 64 {
		t.Fatalf("csv export metadata invalid: %+v", csvRun)
	}
	reader := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(csvRun.Artifact, []byte{0xEF, 0xBB, 0xBF})))
	records, err := reader.ReadAll()
	if err != nil || len(records) != 2 || records[1][9] != "'=unsafe-ledger-code" || records[1][10] != "'+unsafe-dimension" {
		t.Fatalf("csv formula protection missing: records=%v err=%v", records, err)
	}
	packageRun, err := service.CreateFinanceExport(ctx, principal, FinanceExportInput{Kind: "audit-package", FinanceReportInput: FinanceReportInput{BranchID: "accra", PeriodID: period.ID}}, "report-package")
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(packageRun.Artifact), int64(len(packageRun.Artifact)))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, file := range archive.File {
		names[file.Name] = true
		body, openErr := file.Open()
		if openErr != nil {
			t.Fatal(openErr)
		}
		_, _ = io.Copy(io.Discard, body)
		_ = body.Close()
	}
	if !names["manifest.json"] || !names["report.json"] || !names["accounting.csv"] {
		t.Fatalf("audit package incomplete: %v", names)
	}
	stored, err := service.GetFinanceExport(ctx, principal, packageRun.ID)
	if err != nil || !bytes.Equal(stored.Artifact, packageRun.Artifact) {
		t.Fatalf("private artifact did not round trip: err=%v", err)
	}
	audits, _ := db.Collection("chms_audit_events").CountDocuments(ctx, bson.M{"action": "finance.export.create"})
	if audits != 2 {
		t.Fatalf("export audit count=%d", audits)
	}
	noExport := principal
	noExport.Actor.ID = "read-only-finance"
	noExport.Grants = []platform.Grant{{Action: "read", Resource: "finance-ledger", BranchIDs: []platform.ID{"accra"}, FieldClasses: []platform.FieldClass{platform.FieldFinancial}}}
	if _, err = service.CreateFinanceExport(context.Background(), noExport, FinanceExportInput{Kind: "accounting-csv", FinanceReportInput: FinanceReportInput{BranchID: "accra", PeriodID: period.ID}}, "denied-export"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("read-only principal exported finance data: %v", err)
	}
}
