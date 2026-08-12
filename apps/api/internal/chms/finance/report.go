package finance

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"remi-api/internal/chms/platform"
)

type FinanceReportInput struct {
	BranchID platform.ID `json:"branchId"`
	PeriodID platform.ID `json:"periodId,omitempty"`
	StartsAt time.Time   `json:"startsAt,omitempty"`
	EndsAt   time.Time   `json:"endsAt,omitempty"`
}

type FinanceReportMeta struct {
	MetricVersion   string      `json:"metricVersion"`
	AsOf            time.Time   `json:"asOf"`
	SourceWatermark string      `json:"sourceWatermark"`
	Timezone        string      `json:"timezone"`
	BranchID        platform.ID `json:"branchId"`
	PeriodID        platform.ID `json:"periodId,omitempty"`
	StartsAt        time.Time   `json:"startsAt"`
	EndsAt          time.Time   `json:"endsAt"`
	Caveats         []string    `json:"caveats"`
}

type FundMovementRow struct {
	FundID      platform.ID `json:"fundId"`
	FundCode    string      `json:"fundCode"`
	FundName    string      `json:"fundName"`
	AmountMinor int64       `json:"amountMinor"`
	GiftCount   int         `json:"giftCount"`
}
type GivingTrendRow struct {
	Date        string `json:"date"`
	AmountMinor int64  `json:"amountMinor"`
	GiftCount   int    `json:"giftCount"`
}
type DepositReconciliationRow struct {
	SettlementID             platform.ID `json:"settlementId"`
	Reference                string      `json:"reference"`
	SettledAt                time.Time   `json:"settledAt"`
	State                    string      `json:"state"`
	GrossAmountMinor         int64       `json:"grossAmountMinor"`
	NetAmountMinor           int64       `json:"netAmountMinor"`
	MatchedGrossMinor        int64       `json:"matchedGrossMinor"`
	UnexplainedVarianceMinor int64       `json:"unexplainedVarianceMinor"`
	UnresolvedCount          int64       `json:"unresolvedCount"`
}
type PledgeProgressRow struct {
	FundID               platform.ID `json:"fundId"`
	FundName             string      `json:"fundName"`
	PledgeCount          int         `json:"pledgeCount"`
	TargetAmountMinor    int64       `json:"targetAmountMinor"`
	FulfilledAmountMinor int64       `json:"fulfilledAmountMinor"`
	RemainingAmountMinor int64       `json:"remainingAmountMinor"`
}
type FinanceReportTotals struct {
	NetContributionMinor int64 `json:"netContributionMinor"`
	ContributionCount    int   `json:"contributionCount"`
	AdjustmentCount      int   `json:"adjustmentCount"`
	AnonymousCount       int   `json:"anonymousCount"`
	UnresolvedCount      int   `json:"unresolvedCount"`
	SettlementGrossMinor int64 `json:"settlementGrossMinor"`
	SettlementNetMinor   int64 `json:"settlementNetMinor"`
	UnexplainedMinor     int64 `json:"unexplainedMinor"`
	PledgeTargetMinor    int64 `json:"pledgeTargetMinor"`
	PledgeFulfilledMinor int64 `json:"pledgeFulfilledMinor"`
}
type FinanceReport struct {
	Meta                  FinanceReportMeta          `json:"meta"`
	Totals                FinanceReportTotals        `json:"totals"`
	FundMovement          []FundMovementRow          `json:"fundMovement"`
	GivingTrend           []GivingTrendRow           `json:"givingTrend"`
	DepositReconciliation []DepositReconciliationRow `json:"depositReconciliation"`
	PledgeProgress        []PledgeProgressRow        `json:"pledgeProgress"`
}

type FinanceExportInput struct {
	Kind string `json:"kind"`
	FinanceReportInput
}
type FinanceExportRun struct {
	ID               platform.ID    `json:"id" bson:"_id"`
	OrganizationID   platform.ID    `json:"organizationId" bson:"organizationId"`
	BranchID         platform.ID    `json:"branchId" bson:"branchId"`
	Kind             string         `json:"kind" bson:"kind"`
	Status           string         `json:"status" bson:"status"`
	Report           FinanceReport  `json:"report" bson:"report"`
	FileName         string         `json:"fileName" bson:"fileName"`
	ContentType      string         `json:"contentType" bson:"contentType"`
	Artifact         []byte         `json:"-" bson:"artifact"`
	ArtifactHash     string         `json:"artifactHash" bson:"artifactHash"`
	RowCount         int            `json:"rowCount" bson:"rowCount"`
	TotalAmountMinor int64          `json:"totalAmountMinor" bson:"totalAmountMinor"`
	CreatedBy        platform.Actor `json:"createdBy" bson:"createdBy"`
	CreatedAt        time.Time      `json:"createdAt" bson:"createdAt"`
	CompletedAt      *time.Time     `json:"completedAt,omitempty" bson:"completedAt,omitempty"`
}

func (i *FinanceReportInput) normalize(ctx context.Context, repository *Repository, organizationID platform.ID) error {
	if !i.BranchID.Valid() {
		return fmt.Errorf("branchId is required")
	}
	if i.PeriodID.Valid() {
		period, err := repository.FindFiscalPeriod(ctx, organizationID, i.PeriodID)
		if err != nil || period == nil {
			return fmt.Errorf("fiscal period was not found")
		}
		i.StartsAt, i.EndsAt = period.StartsAt, period.EndsAt
	}
	i.StartsAt, i.EndsAt = i.StartsAt.UTC(), i.EndsAt.UTC()
	if i.StartsAt.IsZero() || !i.EndsAt.After(i.StartsAt) || i.EndsAt.Sub(i.StartsAt) > 5*366*24*time.Hour {
		return fmt.Errorf("a valid reporting range of at most five years is required")
	}
	return nil
}

func (s Service) BuildFinanceReport(ctx context.Context, p platform.Principal, input FinanceReportInput) (*FinanceReport, error) {
	if err := input.normalize(ctx, s.Repository, p.OrganizationID); err != nil {
		return nil, invalid(err)
	}
	if !s.allowedLedger(p, "read", input.BranchID, false) {
		return nil, denied()
	}
	contributions, err := s.Repository.ReportContributions(ctx, p.OrganizationID, input.BranchID, input.StartsAt, input.EndsAt)
	if err != nil {
		return nil, err
	}
	settlements, err := s.Repository.ListSettlements(ctx, p.OrganizationID)
	if err != nil {
		return nil, err
	}
	pledges, err := s.Repository.ListPledges(ctx, p.OrganizationID, "")
	if err != nil {
		return nil, err
	}
	funds, err := s.Repository.ListFunds(ctx, p.OrganizationID)
	if err != nil {
		return nil, err
	}
	campuses, _ := s.Repository.ListCampusSettings(ctx, p.OrganizationID)
	fundByID := map[platform.ID]Fund{}
	for _, fund := range funds {
		fundByID[fund.ID] = fund
	}
	report := &FinanceReport{Meta: FinanceReportMeta{MetricVersion: "finance-1.0.0", AsOf: s.now(), Timezone: "Africa/Accra", BranchID: input.BranchID, PeriodID: input.PeriodID, StartsAt: input.StartsAt, EndsAt: input.EndsAt, Caveats: []string{"Contribution totals include immutable posted adjustments in the selected range.", "Pledge progress is intention tracking and must not be treated as debt or member ranking."}}, FundMovement: []FundMovementRow{}, GivingTrend: []GivingTrendRow{}, DepositReconciliation: []DepositReconciliationRow{}, PledgeProgress: []PledgeProgressRow{}}
	for _, campus := range campuses {
		if campus.BranchID == input.BranchID && campus.Timezone != "" {
			report.Meta.Timezone = campus.Timezone
			break
		}
	}
	fundRows := map[platform.ID]*FundMovementRow{}
	trendRows := map[string]*GivingTrendRow{}
	watermark := input.StartsAt
	sourceKeys := make([]string, 0, len(contributions)+len(settlements)+len(pledges))
	location, _ := time.LoadLocation(report.Meta.Timezone)
	if location == nil {
		location = time.UTC
	}
	for _, contribution := range contributions {
		report.Totals.NetContributionMinor += contribution.Total.AmountMinor
		report.Totals.ContributionCount++
		if contribution.Link != nil {
			report.Totals.AdjustmentCount++
		}
		if contribution.Donor.Type == "anonymous" {
			report.Totals.AnonymousCount++
		}
		if contribution.Donor.Type == "unresolved" {
			report.Totals.UnresolvedCount++
		}
		date := contribution.ReceivedAt.In(location).Format("2006-01-02")
		trend := trendRows[date]
		if trend == nil {
			trend = &GivingTrendRow{Date: date}
			trendRows[date] = trend
		}
		trend.AmountMinor += contribution.Total.AmountMinor
		trend.GiftCount++
		for _, split := range contribution.Splits {
			row := fundRows[split.FundID]
			if row == nil {
				fund := fundByID[split.FundID]
				row = &FundMovementRow{FundID: split.FundID, FundCode: fund.Code, FundName: fund.Name}
				fundRows[split.FundID] = row
			}
			row.AmountMinor += split.Amount.AmountMinor
			row.GiftCount++
		}
		if contribution.UpdatedAt.After(watermark) {
			watermark = contribution.UpdatedAt
		}
		sourceKeys = append(sourceKeys, string(contribution.ID)+":"+strconv.FormatInt(contribution.Version, 10))
	}
	for _, settlement := range settlements {
		if settlement.BranchID != input.BranchID || settlement.SettledAt.Before(input.StartsAt) || !settlement.SettledAt.Before(input.EndsAt) {
			continue
		}
		report.Totals.SettlementGrossMinor += settlement.GrossAmountMinor
		report.Totals.SettlementNetMinor += settlement.NetAmountMinor
		report.Totals.UnexplainedMinor += settlement.UnexplainedVarianceMinor
		report.DepositReconciliation = append(report.DepositReconciliation, DepositReconciliationRow{SettlementID: settlement.ID, Reference: settlement.Reference, SettledAt: settlement.SettledAt, State: settlement.State, GrossAmountMinor: settlement.GrossAmountMinor, NetAmountMinor: settlement.NetAmountMinor, MatchedGrossMinor: settlement.MatchedGrossMinor, UnexplainedVarianceMinor: settlement.UnexplainedVarianceMinor, UnresolvedCount: settlement.UnresolvedCount})
		if settlement.UpdatedAt.After(watermark) {
			watermark = settlement.UpdatedAt
		}
		sourceKeys = append(sourceKeys, string(settlement.ID)+":"+strconv.FormatInt(settlement.ReconciliationVersion, 10))
	}
	pledgeRows := map[platform.ID]*PledgeProgressRow{}
	for _, pledge := range pledges {
		if pledge.BranchID != input.BranchID || pledge.CreatedAt.After(input.EndsAt) {
			continue
		}
		row := pledgeRows[pledge.FundID]
		if row == nil {
			row = &PledgeProgressRow{FundID: pledge.FundID, FundName: fundByID[pledge.FundID].Name}
			pledgeRows[pledge.FundID] = row
		}
		row.PledgeCount++
		row.TargetAmountMinor += pledge.Target.AmountMinor
		row.FulfilledAmountMinor += pledge.FulfilledAmountMinor
		row.RemainingAmountMinor += pledge.RemainingAmountMinor
		report.Totals.PledgeTargetMinor += pledge.Target.AmountMinor
		report.Totals.PledgeFulfilledMinor += pledge.FulfilledAmountMinor
		if pledge.UpdatedAt.After(watermark) {
			watermark = pledge.UpdatedAt
		}
		sourceKeys = append(sourceKeys, string(pledge.ID)+":"+strconv.FormatInt(pledge.Version, 10))
	}
	for _, row := range fundRows {
		report.FundMovement = append(report.FundMovement, *row)
	}
	for _, row := range trendRows {
		report.GivingTrend = append(report.GivingTrend, *row)
	}
	for _, row := range pledgeRows {
		report.PledgeProgress = append(report.PledgeProgress, *row)
	}
	sort.Slice(report.FundMovement, func(i, j int) bool { return report.FundMovement[i].FundName < report.FundMovement[j].FundName })
	sort.Slice(report.GivingTrend, func(i, j int) bool { return report.GivingTrend[i].Date < report.GivingTrend[j].Date })
	sort.Slice(report.DepositReconciliation, func(i, j int) bool {
		return report.DepositReconciliation[i].SettledAt.Before(report.DepositReconciliation[j].SettledAt)
	})
	sort.Slice(report.PledgeProgress, func(i, j int) bool { return report.PledgeProgress[i].FundName < report.PledgeProgress[j].FundName })
	sort.Strings(sourceKeys)
	hash := sha256.Sum256([]byte(strings.Join(sourceKeys, "\n")))
	report.Meta.SourceWatermark = watermark.UTC().Format(time.RFC3339Nano) + ":" + hex.EncodeToString(hash[:])
	return report, nil
}

func (s Service) CreateFinanceExport(ctx context.Context, p platform.Principal, input FinanceExportInput, requestID string) (*FinanceExportRun, error) {
	input.Kind = strings.ToLower(strings.TrimSpace(input.Kind))
	if input.Kind != "accounting-csv" && input.Kind != "audit-package" {
		return nil, invalid(fmt.Errorf("kind must be accounting-csv or audit-package"))
	}
	if !s.allowedLedger(p, "export", input.BranchID, true) {
		return nil, denied()
	}
	report, err := s.BuildFinanceReport(ctx, p, input.FinanceReportInput)
	if err != nil {
		return nil, err
	}
	contributions, err := s.Repository.ReportContributions(ctx, p.OrganizationID, input.BranchID, report.Meta.StartsAt, report.Meta.EndsAt)
	if err != nil {
		return nil, err
	}
	mappings, err := s.Repository.ListAccountMappings(ctx, p.OrganizationID)
	if err != nil {
		return nil, err
	}
	csvBody, rowCount, err := accountingCSV(contributions, mappings)
	if err != nil {
		return nil, err
	}
	artifact, contentType, extension := csvBody, "text/csv; charset=utf-8", "csv"
	if input.Kind == "audit-package" {
		artifact, err = auditPackage(report, csvBody)
		if err != nil {
			return nil, err
		}
		contentType, extension = "application/zip", "zip"
	}
	now := s.now()
	digest := sha256.Sum256(artifact)
	run := &FinanceExportRun{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: input.BranchID, Kind: input.Kind, Status: "completed", Report: *report, FileName: fmt.Sprintf("remi-finance-%s-%s.%s", input.Kind, now.Format("20060102-150405"), extension), ContentType: contentType, Artifact: artifact, ArtifactHash: hex.EncodeToString(digest[:]), RowCount: rowCount, TotalAmountMinor: report.Totals.NetContributionMinor, CreatedBy: p.Actor, CreatedAt: now, CompletedAt: &now}
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Repository.Insert(tx, financeExportRunsCollection, run); err != nil {
			return err
		}
		return s.audit(tx, p, input.BranchID, "finance.export.create", "finance-export", run.ID, []string{"kind", "scope", "sourceWatermark", "artifactHash", "rowCount", "total"}, requestID, now)
	})
	if err != nil {
		return nil, err
	}
	return run, nil
}

func (s Service) ListFinanceExports(ctx context.Context, p platform.Principal, branch platform.ID) ([]FinanceExportRun, error) {
	if !branch.Valid() || !s.allowedLedger(p, "read", branch, false) {
		return nil, denied()
	}
	return s.Repository.ListFinanceExportRuns(ctx, p.OrganizationID, branch)
}
func (s Service) GetFinanceExport(ctx context.Context, p platform.Principal, id platform.ID) (*FinanceExportRun, error) {
	run, err := s.Repository.FindFinanceExportRun(ctx, p.OrganizationID, id)
	if err != nil || run == nil || !s.allowedLedger(p, "read", run.BranchID, false) {
		return nil, denied()
	}
	return run, nil
}

func (r *Repository) ReportContributions(ctx context.Context, org, branch platform.ID, startsAt, endsAt time.Time) ([]Contribution, error) {
	filter := bson.M{"organizationId": org, "branchId": branch, "state": "posted", "archivedAt": nil, "receivedAt": bson.M{"$gte": startsAt, "$lt": endsAt}}
	cursor, err := r.database.Collection(contributionsCollection).Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "receivedAt", Value: 1}, {Key: "receiptNumber", Value: 1}}).SetLimit(10000))
	if err != nil {
		return nil, fmt.Errorf("report contributions: %w", err)
	}
	defer cursor.Close(ctx)
	values := []Contribution{}
	if err = cursor.All(ctx, &values); err != nil {
		return nil, fmt.Errorf("decode report contributions: %w", err)
	}
	return values, nil
}
func (r *Repository) ListFinanceExportRuns(ctx context.Context, org, branch platform.ID) ([]FinanceExportRun, error) {
	cursor, err := r.database.Collection(financeExportRunsCollection).Find(ctx, bson.M{"organizationId": org, "branchId": branch}, options.Find().SetProjection(bson.M{"artifact": 0}).SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(100))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	values := []FinanceExportRun{}
	return values, cursor.All(ctx, &values)
}
func (r *Repository) FindFinanceExportRun(ctx context.Context, org, id platform.ID) (*FinanceExportRun, error) {
	return findOne[FinanceExportRun](ctx, r, financeExportRunsCollection, org, id)
}

func accountingCSV(contributions []Contribution, mappings []AccountMapping) ([]byte, int, error) {
	buffer := &bytes.Buffer{}
	buffer.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(buffer)
	if err := writer.Write([]string{"receipt_number", "received_at", "branch_id", "donor_type", "source", "payment_method_id", "fund_id", "amount_minor", "currency", "external_account_code", "external_dimension_code", "adjustment_type", "chain_root_id"}); err != nil {
		return nil, 0, err
	}
	rows := 0
	for _, contribution := range contributions {
		for _, split := range contribution.Splits {
			mapping := mappingAt(mappings, contribution.BranchID, split.FundID, contribution.ReceivedAt)
			adjustmentType, chainRoot := "", ""
			if contribution.Link != nil {
				adjustmentType, chainRoot = contribution.Link.Type, string(contribution.Link.ChainRootID)
			}
			values := []string{contribution.ReceiptNumber, contribution.ReceivedAt.Format(time.RFC3339), string(contribution.BranchID), contribution.Donor.Type, contribution.Source, string(contribution.PaymentMethodID), string(split.FundID), strconv.FormatInt(split.Amount.AmountMinor, 10), split.Amount.Currency, mapping.ExternalAccountCode, mapping.ExternalDimensionCode, adjustmentType, chainRoot}
			for index := range values {
				values[index] = safeCSV(values[index])
			}
			if err := writer.Write(values); err != nil {
				return nil, 0, err
			}
			rows++
		}
	}
	writer.Flush()
	return buffer.Bytes(), rows, writer.Error()
}
func mappingAt(values []AccountMapping, branch, fund platform.ID, at time.Time) AccountMapping {
	var selected AccountMapping
	for _, value := range values {
		if value.BranchID != branch || value.Category != "contribution" || (value.FundID.Valid() && value.FundID != fund) || value.EffectiveFrom.After(at) || (value.EffectiveUntil != nil && !at.Before(*value.EffectiveUntil)) {
			continue
		}
		if selected.ID == "" || value.EffectiveFrom.After(selected.EffectiveFrom) || (value.FundID == fund && selected.FundID != fund) {
			selected = value
		}
	}
	return selected
}
func safeCSV(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + value
	}
	return value
}
func auditPackage(report *FinanceReport, csvBody []byte) ([]byte, error) {
	buffer := &bytes.Buffer{}
	archive := zip.NewWriter(buffer)
	reportBody, _ := json.MarshalIndent(report, "", "  ")
	manifestBody, _ := json.MarshalIndent(map[string]any{"metricVersion": report.Meta.MetricVersion, "sourceWatermark": report.Meta.SourceWatermark, "asOf": report.Meta.AsOf, "branchId": report.Meta.BranchID, "periodId": report.Meta.PeriodID, "startsAt": report.Meta.StartsAt, "endsAt": report.Meta.EndsAt, "netContributionMinor": report.Totals.NetContributionMinor, "reconciliationUnexplainedMinor": report.Totals.UnexplainedMinor}, "", "  ")
	for name, body := range map[string][]byte{"report.json": reportBody, "accounting.csv": csvBody, "manifest.json": manifestBody} {
		file, err := archive.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err = file.Write(body); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
