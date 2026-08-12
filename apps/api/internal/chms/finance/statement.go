package finance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"remi-api/internal/chms/platform"
)

const (
	statementRunsCollection            = "chms_finance_statement_runs"
	documentDeliveriesCollection       = "chms_finance_document_deliveries"
	statementArtifactVersion     int64 = 1
)

type StatementSubject struct {
	Type     string      `json:"type" bson:"type"`
	ID       platform.ID `json:"id" bson:"id"`
	Name     string      `json:"name" bson:"name"`
	BranchID platform.ID `json:"branchId" bson:"branchId"`
}

type StatementAllocation struct {
	FundID platform.ID `json:"fundId" bson:"fundId"`
	Fund   string      `json:"fund" bson:"fund"`
	Amount int64       `json:"amountMinor" bson:"amountMinor"`
}

type StatementRow struct {
	ContributionID platform.ID           `json:"contributionId" bson:"contributionId"`
	ReceiptNumber  string                `json:"receiptNumber" bson:"receiptNumber"`
	ReceivedAt     time.Time             `json:"receivedAt" bson:"receivedAt"`
	BranchID       platform.ID           `json:"branchId" bson:"branchId"`
	Source         string                `json:"source" bson:"source"`
	Kind           string                `json:"kind" bson:"kind"`
	AmountMinor    int64                 `json:"amountMinor" bson:"amountMinor"`
	Allocations    []StatementAllocation `json:"allocations" bson:"allocations"`
}

type StatementRun struct {
	platform.ResourceEnvelope `bson:",inline"`
	ScopeKey                  string           `json:"scopeKey" bson:"scopeKey"`
	Subject                   StatementSubject `json:"subject" bson:"subject"`
	BranchScope               platform.ID      `json:"branchScope,omitempty" bson:"branchScope,omitempty"`
	PeriodID                  platform.ID      `json:"periodId,omitempty" bson:"periodId,omitempty"`
	Year                      int              `json:"year" bson:"year"`
	From                      time.Time        `json:"from" bson:"from"`
	To                        time.Time        `json:"to" bson:"to"`
	StatementVersion          int64            `json:"statementVersion" bson:"statementVersion"`
	ArtifactVersion           int64            `json:"artifactVersion" bson:"artifactVersion"`
	SourceHash                string           `json:"sourceHash" bson:"sourceHash"`
	ArtifactHash              string           `json:"artifactHash" bson:"artifactHash"`
	SupersedesID              platform.ID      `json:"supersedesId,omitempty" bson:"supersedesId,omitempty"`
	Rows                      []StatementRow   `json:"rows" bson:"rows"`
	TotalAmountMinor          int64            `json:"totalAmountMinor" bson:"totalAmountMinor"`
	Currency                  string           `json:"currency" bson:"currency"`
	GeneratedAt               time.Time        `json:"generatedAt" bson:"generatedAt"`
	DeliveryCount             int64            `json:"deliveryCount" bson:"-"`
}

type StatementInput struct {
	SubjectType string      `json:"subjectType"`
	SubjectID   platform.ID `json:"subjectId"`
	BranchID    platform.ID `json:"branchId"`
	PeriodID    platform.ID `json:"periodId"`
	Year        int         `json:"year"`
}

type StatementSubjectOption struct {
	Type     string      `json:"type"`
	ID       platform.ID `json:"id"`
	Name     string      `json:"name"`
	BranchID platform.ID `json:"branchId"`
}

type ReceiptSummary struct {
	ContributionID platform.ID           `json:"contributionId"`
	ReceiptNumber  string                `json:"receiptNumber"`
	ReceivedAt     time.Time             `json:"receivedAt"`
	BranchID       platform.ID           `json:"branchId"`
	Source         string                `json:"source"`
	Kind           string                `json:"kind"`
	AmountMinor    int64                 `json:"amountMinor"`
	Currency       string                `json:"currency"`
	Allocations    []StatementAllocation `json:"allocations"`
	EffectiveState string                `json:"effectiveState"`
}

type DocumentDelivery struct {
	ID             platform.ID `json:"id" bson:"_id"`
	OrganizationID platform.ID `json:"organizationId" bson:"organizationId"`
	DocumentType   string      `json:"documentType" bson:"documentType"`
	DocumentID     platform.ID `json:"documentId" bson:"documentId"`
	RequestKey     string      `json:"requestKey" bson:"requestKey"`
	Channel        string      `json:"channel" bson:"channel"`
	RecipientHint  string      `json:"recipientHint" bson:"recipientHint"`
	ArtifactHash   string      `json:"artifactHash" bson:"artifactHash"`
	State          string      `json:"state" bson:"state"`
	FailureReason  string      `json:"failureReason,omitempty" bson:"failureReason,omitempty"`
	RequestedBy    platform.ID `json:"requestedBy" bson:"requestedBy"`
	RequestedAt    time.Time   `json:"requestedAt" bson:"requestedAt"`
	DeliveredAt    *time.Time  `json:"deliveredAt,omitempty" bson:"deliveredAt,omitempty"`
}

type StatementMailer interface {
	SendAttachment(to, subject, html, filename string, content []byte) error
}

func (input *StatementInput) normalize() error {
	input.SubjectType = strings.ToLower(strings.TrimSpace(input.SubjectType))
	if !map[string]bool{"person": true, "household": true}[input.SubjectType] || !input.SubjectID.Valid() {
		return fmt.Errorf("person or household subject is required")
	}
	if input.PeriodID.Valid() == (input.Year > 0) {
		return fmt.Errorf("choose exactly one fiscal period or calendar year")
	}
	if input.Year > 0 && (input.Year < 2000 || input.Year > 2200) {
		return fmt.Errorf("year is outside the supported range")
	}
	return nil
}

func (s Service) statementRange(ctx context.Context, org platform.ID, input StatementInput) (time.Time, time.Time, platform.ID, int, error) {
	if input.PeriodID.Valid() {
		period, err := s.Repository.FindFiscalPeriod(ctx, org, input.PeriodID)
		if err != nil || period == nil {
			return time.Time{}, time.Time{}, "", 0, settlementDenied()
		}
		return period.StartsAt.UTC(), period.EndsAt.UTC(), period.ID, period.StartsAt.Year(), nil
	}
	return time.Date(input.Year, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(input.Year+1, 1, 1, 0, 0, 0, 0, time.UTC), "", input.Year, nil
}

func (s Service) canAccessStatement(ctx context.Context, p platform.Principal, subject StatementSubject, issuing bool) (bool, error) {
	if p.Actor.Type == platform.ActorMember {
		return s.Repository.MemberCanAccessStatementSubject(ctx, p.OrganizationID, p.Actor.ID, subject.Type, subject.ID)
	}
	if issuing {
		return s.allowed(p, "create", subject.BranchID) && s.allowedLedger(p, "read", subject.BranchID, false), nil
	}
	return s.allowedLedger(p, "read", subject.BranchID, false), nil
}

func (s Service) GenerateStatement(ctx context.Context, p platform.Principal, input StatementInput, requestID string) (*StatementRun, error) {
	if err := input.normalize(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_statement", Message: err.Error()})
	}
	subject, err := s.Repository.FindStatementSubject(ctx, p.OrganizationID, input.SubjectType, input.SubjectID)
	if err != nil || subject == nil {
		return nil, settlementDenied()
	}
	allowed, err := s.canAccessStatement(ctx, p, *subject, true)
	if err != nil {
		return nil, err
	}
	if !allowed || (p.Actor.Type != platform.ActorMember && (!input.BranchID.Valid() || input.BranchID != subject.BranchID)) {
		return nil, settlementDenied()
	}
	from, to, periodID, year, err := s.statementRange(ctx, p.OrganizationID, input)
	if err != nil {
		return nil, err
	}
	branch := input.BranchID
	if p.Actor.Type == platform.ActorMember {
		branch = ""
	}
	rows, err := s.Repository.BuildStatementRows(ctx, p.OrganizationID, *subject, branch, from, to)
	if err != nil {
		return nil, err
	}
	scopePayload := struct {
		SubjectType string      `json:"subjectType"`
		SubjectID   platform.ID `json:"subjectId"`
		BranchID    platform.ID `json:"branchId,omitempty"`
		PeriodID    platform.ID `json:"periodId,omitempty"`
		Year        int         `json:"year"`
		From        time.Time   `json:"from"`
		To          time.Time   `json:"to"`
	}{subject.Type, subject.ID, branch, periodID, year, from, to}
	scopeBytes, _ := json.Marshal(scopePayload)
	scopeDigest := sha256.Sum256(scopeBytes)
	scopeKey := hex.EncodeToString(scopeDigest[:])
	sourceBytes, _ := json.Marshal(struct {
		Subject StatementSubject `json:"subject"`
		Rows    []StatementRow   `json:"rows"`
	}{Subject: *subject, Rows: rows})
	sourceDigest := sha256.Sum256(sourceBytes)
	sourceHash := hex.EncodeToString(sourceDigest[:])
	if existing, e := s.Repository.FindStatementBySource(ctx, p.OrganizationID, scopeKey, sourceHash); e != nil {
		return nil, e
	} else if existing != nil {
		return s.Repository.DecorateStatement(ctx, existing)
	}
	latest, err := s.Repository.FindLatestStatement(ctx, p.OrganizationID, scopeKey)
	if err != nil {
		return nil, err
	}
	version := int64(1)
	var supersedes platform.ID
	if latest != nil {
		version, supersedes = latest.StatementVersion+1, latest.ID
	}
	var total int64
	for _, row := range rows {
		if (row.AmountMinor > 0 && total > math.MaxInt64-row.AmountMinor) || (row.AmountMinor < 0 && total < math.MinInt64-row.AmountMinor) {
			return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "money_overflow", Message: "Statement total exceeds the supported money range."})
		}
		total += row.AmountMinor
	}
	now := s.now()
	run := StatementRun{ResourceEnvelope: envelope(p, subject.BranchID, now), ScopeKey: scopeKey, Subject: *subject, BranchScope: branch, PeriodID: periodID, Year: year, From: from, To: to, StatementVersion: version, ArtifactVersion: statementArtifactVersion, SourceHash: sourceHash, SupersedesID: supersedes, Rows: rows, TotalAmountMinor: total, Currency: "GHS", GeneratedAt: now}
	pdf := renderStatementPDF(&run)
	artifactDigest := sha256.Sum256(pdf)
	run.ArtifactHash = hex.EncodeToString(artifactDigest[:])
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if insertErr := s.Repository.Insert(tx, statementRunsCollection, run); insertErr != nil {
			return insertErr
		}
		if auditErr := s.audit(tx, p, subject.BranchID, "finance.statement.generate", "statement-run", run.ID, []string{"subject", "range", "sourceHash", "artifactHash", "version"}, requestID, now); auditErr != nil {
			return auditErr
		}
		return s.Platform.EnqueueEvent(tx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: subject.BranchID, Type: "finance.statement.generated", EventVersion: 1, AggregateType: "statement-run", AggregateID: run.ID, AggregateVersion: version, Actor: p.Actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: bson.M{"statementId": run.ID, "subjectType": subject.Type, "rowCount": len(rows)}}, State: "pending", AvailableAt: now})
	})
	if err != nil {
		if existing, findErr := s.Repository.FindStatementBySource(ctx, p.OrganizationID, scopeKey, sourceHash); findErr == nil && existing != nil {
			return s.Repository.DecorateStatement(ctx, existing)
		}
		return nil, err
	}
	return &run, nil
}

func (s Service) ListStatements(ctx context.Context, p platform.Principal, subjectType string, subjectID platform.ID) ([]StatementRun, error) {
	subject, err := s.Repository.FindStatementSubject(ctx, p.OrganizationID, subjectType, subjectID)
	if err != nil || subject == nil {
		return nil, settlementDenied()
	}
	allowed, err := s.canAccessStatement(ctx, p, *subject, false)
	if err != nil || !allowed {
		return nil, settlementDenied()
	}
	return s.Repository.ListStatements(ctx, p.OrganizationID, subjectType, subjectID)
}

func (s Service) SearchStatementSubjects(ctx context.Context, p platform.Principal, query string) ([]StatementSubjectOption, error) {
	if p.Actor.Type != platform.ActorStaff {
		return nil, settlementDenied()
	}
	query = strings.TrimSpace(query)
	if len([]rune(query)) < 2 || len([]rune(query)) > 80 {
		return nil, platform.ValidationError(platform.FieldError{Path: "q", Code: "invalid", Message: "Enter 2 to 80 characters to find a statement recipient."})
	}
	values, err := s.Repository.SearchStatementSubjects(ctx, p.OrganizationID, query)
	if err != nil {
		return nil, err
	}
	visible := make([]StatementSubjectOption, 0, len(values))
	for _, value := range values {
		if s.allowedLedger(p, "read", value.BranchID, false) {
			visible = append(visible, value)
		}
	}
	return visible, nil
}

func (s Service) GetStatement(ctx context.Context, p platform.Principal, id platform.ID) (*StatementRun, error) {
	run, err := s.Repository.FindStatement(ctx, p.OrganizationID, id)
	if err != nil || run == nil {
		return nil, settlementDenied()
	}
	allowed, err := s.canAccessStatement(ctx, p, run.Subject, false)
	if err != nil || !allowed {
		return nil, settlementDenied()
	}
	return s.Repository.DecorateStatement(ctx, run)
}

func (s Service) StatementPDF(ctx context.Context, p platform.Principal, id platform.ID) ([]byte, *StatementRun, error) {
	run, err := s.GetStatement(ctx, p, id)
	if err != nil {
		return nil, nil, err
	}
	pdf := renderStatementPDF(run)
	digest := sha256.Sum256(pdf)
	if hex.EncodeToString(digest[:]) != run.ArtifactHash {
		return nil, nil, fmt.Errorf("statement artifact hash mismatch")
	}
	return pdf, run, nil
}

func renderStatementPDF(run *StatementRun) []byte {
	lines := []string{
		fmt.Sprintf("Statement for: %s", run.Subject.Name),
		fmt.Sprintf("Period: %s to %s", run.From.Format("02 Jan 2006"), run.To.Add(-time.Nanosecond).Format("02 Jan 2006")),
		fmt.Sprintf("Version: %d  Currency: %s", run.StatementVersion, run.Currency),
		"",
	}
	for _, row := range run.Rows {
		lines = append(lines, fmt.Sprintf("%s  %-18s  %-12s  GHS %s", row.ReceivedAt.Format("02 Jan 2006"), row.ReceiptNumber, row.Kind, formatMinor(row.AmountMinor)))
		for _, allocation := range row.Allocations {
			lines = append(lines, fmt.Sprintf("    %s  GHS %s", allocation.Fund, formatMinor(allocation.Amount)))
		}
	}
	lines = append(lines, "", "Statement total: GHS "+formatMinor(run.TotalAmountMinor), "This informational statement reflects posted gifts and approved corrections in REMI.")
	return renderTextPDF("REMI Giving Statement", lines)
}

func formatMinor(amount int64) string {
	negative := amount < 0
	whole, cents := amount/100, amount%100
	if whole < 0 {
		whole = -whole
	}
	if cents < 0 {
		cents = -cents
	}
	value := strconv.FormatInt(whole, 10) + "." + fmt.Sprintf("%02d", cents)
	if negative {
		return "-" + value
	}
	return value
}

func (s Service) ListReceipts(ctx context.Context, p platform.Principal, subjectType string, subjectID, periodID platform.ID, year int) ([]ReceiptSummary, error) {
	input := StatementInput{SubjectType: subjectType, SubjectID: subjectID, PeriodID: periodID, Year: year}
	if err := input.normalize(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_receipt_filter", Message: err.Error()})
	}
	subject, err := s.Repository.FindStatementSubject(ctx, p.OrganizationID, subjectType, subjectID)
	if err != nil || subject == nil {
		return nil, settlementDenied()
	}
	allowed, err := s.canAccessStatement(ctx, p, *subject, false)
	if err != nil || !allowed {
		return nil, settlementDenied()
	}
	from, to, _, _, err := s.statementRange(ctx, p.OrganizationID, input)
	if err != nil {
		return nil, err
	}
	rows, err := s.Repository.BuildStatementRows(ctx, p.OrganizationID, *subject, "", from, to)
	if err != nil {
		return nil, err
	}
	values := make([]ReceiptSummary, 0, len(rows))
	originalIDs := make([]platform.ID, 0, len(rows))
	for _, row := range rows {
		if row.Kind == "gift" {
			originalIDs = append(originalIDs, row.ContributionID)
		}
	}
	reversals, err := s.Repository.FindReversals(ctx, p.OrganizationID, originalIDs)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		state := "posted"
		if row.Kind != "gift" {
			state = "adjustment"
		} else if reversal, found := reversals[row.ContributionID]; found && reversal.Link != nil {
			state = "corrected"
			if reversal.Link.Type == "reversal" {
				state = "reversed"
			}
		}
		values = append(values, ReceiptSummary{ContributionID: row.ContributionID, ReceiptNumber: row.ReceiptNumber, ReceivedAt: row.ReceivedAt, BranchID: row.BranchID, Source: row.Source, Kind: row.Kind, AmountMinor: row.AmountMinor, Currency: "GHS", Allocations: row.Allocations, EffectiveState: state})
	}
	return values, nil
}

func (s Service) ReceiptPDF(ctx context.Context, p platform.Principal, contributionID platform.ID) ([]byte, *ReceiptSummary, error) {
	contribution, err := s.Repository.FindContribution(ctx, p.OrganizationID, contributionID)
	if err != nil || contribution == nil || contribution.State != "posted" {
		return nil, nil, ledgerDenied()
	}
	subjectType, subjectID := contribution.Donor.Type, contribution.Donor.PersonID
	if subjectType == "household" {
		subjectID = contribution.Donor.HouseholdID
	}
	if !subjectID.Valid() {
		return nil, nil, ledgerDenied()
	}
	subject, err := s.Repository.FindStatementSubject(ctx, p.OrganizationID, subjectType, subjectID)
	if err != nil || subject == nil {
		return nil, nil, ledgerDenied()
	}
	allowed, err := s.canAccessStatement(ctx, p, *subject, false)
	if err != nil || !allowed {
		return nil, nil, ledgerDenied()
	}
	rows, err := s.Repository.StatementRowsForContributions(ctx, p.OrganizationID, []Contribution{*contribution})
	if err != nil || len(rows) != 1 {
		return nil, nil, err
	}
	row := rows[0]
	effectiveState := "adjustment"
	if contribution.Link == nil {
		effectiveState = contribution.State
		reversal, findErr := s.Repository.FindReversal(ctx, p.OrganizationID, contribution.ID)
		if findErr != nil {
			return nil, nil, findErr
		}
		if reversal != nil && reversal.Link != nil {
			effectiveState = "corrected"
			if reversal.Link.Type == "reversal" {
				effectiveState = "reversed"
			}
		}
	}
	receipt := &ReceiptSummary{ContributionID: row.ContributionID, ReceiptNumber: row.ReceiptNumber, ReceivedAt: row.ReceivedAt, BranchID: row.BranchID, Source: row.Source, Kind: row.Kind, AmountMinor: row.AmountMinor, Currency: "GHS", Allocations: row.Allocations, EffectiveState: effectiveState}
	lines := []string{fmt.Sprintf("Receipt number: %s", receipt.ReceiptNumber), fmt.Sprintf("Received: %s", receipt.ReceivedAt.Format("02 Jan 2006")), fmt.Sprintf("For: %s", subject.Name), fmt.Sprintf("Type: %s", receipt.Kind), "Amount: GHS " + formatMinor(receipt.AmountMinor), ""}
	for _, allocation := range receipt.Allocations {
		lines = append(lines, fmt.Sprintf("%s: GHS %s", allocation.Fund, formatMinor(allocation.Amount)))
	}
	lines = append(lines, "", "This receipt records a posted gift or approved correction in REMI.")
	return renderTextPDF("REMI Contribution Receipt", lines), receipt, nil
}

func (s Service) DeliverStatement(ctx context.Context, p platform.Principal, id platform.ID, requestKey string) (*DocumentDelivery, error) {
	pdf, run, err := s.StatementPDF(ctx, p, id)
	if err != nil {
		return nil, err
	}
	return s.deliverDocument(ctx, p, "statement", run.ID, run.Subject, run.ArtifactHash, fmt.Sprintf("REMI giving statement - %d", run.Year), fmt.Sprintf("REMI-statement-%d-v%d.pdf", run.Year, run.StatementVersion), pdf, requestKey)
}

func (s Service) DeliverReceipt(ctx context.Context, p platform.Principal, id platform.ID, requestKey string) (*DocumentDelivery, error) {
	pdf, receipt, err := s.ReceiptPDF(ctx, p, id)
	if err != nil {
		return nil, err
	}
	subjectType, subjectID := "person", platform.ID("")
	contribution, findErr := s.Repository.FindContribution(ctx, p.OrganizationID, id)
	if findErr != nil {
		return nil, findErr
	}
	if contribution != nil {
		subjectType, subjectID = contribution.Donor.Type, contribution.Donor.PersonID
		if subjectType == "household" {
			subjectID = contribution.Donor.HouseholdID
		}
	}
	subject, err := s.Repository.FindStatementSubject(ctx, p.OrganizationID, subjectType, subjectID)
	if err != nil || subject == nil {
		return nil, ledgerDenied()
	}
	digest := sha256.Sum256(pdf)
	return s.deliverDocument(ctx, p, "receipt", receipt.ContributionID, *subject, hex.EncodeToString(digest[:]), "REMI contribution receipt "+receipt.ReceiptNumber, "REMI-receipt-"+receipt.ReceiptNumber+".pdf", pdf, requestKey)
}

func (s Service) deliverDocument(ctx context.Context, p platform.Principal, documentType string, documentID platform.ID, subject StatementSubject, artifactHash, mailSubject, filename string, pdf []byte, requestKey string) (*DocumentDelivery, error) {
	requestKey = strings.TrimSpace(requestKey)
	if err := platform.ValidateIdempotencyKey(requestKey); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "Idempotency-Key", Code: "invalid", Message: err.Error()})
	}
	if s.Mailer == nil {
		return nil, &platform.DomainError{Code: "dependency_unavailable", Message: "Email delivery is not configured."}
	}
	if existing, err := s.Repository.FindDocumentDelivery(ctx, p.OrganizationID, documentType, documentID, requestKey); err != nil {
		return nil, err
	} else if existing != nil {
		return existing, nil
	}
	email, err := s.Repository.VerifiedStatementEmail(ctx, p.OrganizationID, subject)
	if err != nil {
		return nil, err
	}
	if email == "" {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "verified_email_required", Message: "A verified member email is required for private document delivery."})
	}
	now := s.now()
	delivery := DocumentDelivery{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, DocumentType: documentType, DocumentID: documentID, RequestKey: requestKey, Channel: "email", RecipientHint: maskEmail(email), ArtifactHash: artifactHash, State: "sending", RequestedBy: p.Actor.ID, RequestedAt: now}
	if err = s.Repository.Insert(ctx, documentDeliveriesCollection, delivery); err != nil {
		if existing, findErr := s.Repository.FindDocumentDelivery(ctx, p.OrganizationID, documentType, documentID, requestKey); findErr == nil && existing != nil {
			return existing, nil
		}
		return nil, err
	}
	if err = s.Mailer.SendAttachment(email, mailSubject, "<p>Your private REMI giving document is attached. If you did not request it, contact the church finance team.</p>", filename, pdf); err != nil {
		_ = s.Repository.CompleteDocumentDelivery(ctx, p.OrganizationID, delivery.ID, "failed", err.Error(), now)
		delivery.State, delivery.FailureReason = "failed", "delivery provider rejected the message"
		return &delivery, &platform.DomainError{Code: "dependency_unavailable", Message: "The private document could not be delivered."}
	}
	deliveredAt := s.now()
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if updateErr := s.Repository.CompleteDocumentDelivery(tx, p.OrganizationID, delivery.ID, "delivered", "", deliveredAt); updateErr != nil {
			return updateErr
		}
		return s.audit(tx, p, subject.BranchID, "finance."+documentType+".deliver", documentType, documentID, []string{"channel", "artifactHash", "recipientHint"}, requestKey, deliveredAt)
	})
	if err != nil {
		return nil, err
	}
	delivery.State, delivery.DeliveredAt = "delivered", &deliveredAt
	return &delivery, nil
}

func maskEmail(value string) string {
	parts := strings.Split(value, "@")
	if len(parts) != 2 || len(parts[0]) < 2 {
		return "verified email"
	}
	return parts[0][:1] + "***@" + parts[1]
}

func (r *Repository) FindStatementSubject(ctx context.Context, org platform.ID, subjectType string, id platform.ID) (*StatementSubject, error) {
	if subjectType == "person" {
		var value struct {
			ID       platform.ID                               `bson:"_id"`
			BranchID platform.ID                               `bson:"homeBranchId"`
			Names    struct{ Given, Family, Preferred string } `bson:"names"`
		}
		err := r.database.Collection("chms_people").FindOne(ctx, bson.M{"_id": id, "organizationId": org, "archivedAt": nil}).Decode(&value)
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(value.Names.Preferred + " " + value.Names.Family)
		if strings.TrimSpace(value.Names.Preferred) == "" {
			name = strings.TrimSpace(value.Names.Given + " " + value.Names.Family)
		}
		return &StatementSubject{Type: "person", ID: id, Name: name, BranchID: value.BranchID}, nil
	}
	var value struct {
		ID                  platform.ID `bson:"_id"`
		Name                string      `bson:"name"`
		BranchID            platform.ID `bson:"homeBranchId"`
		StatementPreference string      `bson:"statementPreference"`
	}
	err := r.database.Collection("chms_households").FindOne(ctx, bson.M{"_id": id, "organizationId": org, "archivedAt": nil}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &StatementSubject{Type: "household", ID: id, Name: value.Name, BranchID: value.BranchID}, nil
}

func (r *Repository) SearchStatementSubjects(ctx context.Context, org platform.ID, query string) ([]StatementSubjectOption, error) {
	pattern := bson.Regex{Pattern: regexp.QuoteMeta(strings.TrimSpace(query)), Options: "i"}
	values := make([]StatementSubjectOption, 0, 20)
	peopleCursor, err := r.database.Collection("chms_people").Find(ctx, bson.M{"organizationId": org, "archivedAt": nil, "$or": bson.A{bson.M{"personNumber": pattern}, bson.M{"names.given": pattern}, bson.M{"names.family": pattern}, bson.M{"names.preferred": pattern}}}, options.Find().SetProjection(bson.M{"names": 1, "homeBranchId": 1}).SetLimit(12))
	if err != nil {
		return nil, err
	}
	defer peopleCursor.Close(ctx)
	for peopleCursor.Next(ctx) {
		var person struct {
			ID       platform.ID `bson:"_id"`
			BranchID platform.ID `bson:"homeBranchId"`
			Names    struct {
				Given, Family, Preferred string
			} `bson:"names"`
		}
		if err = peopleCursor.Decode(&person); err != nil {
			return nil, err
		}
		name := strings.TrimSpace(person.Names.Preferred + " " + person.Names.Family)
		if person.Names.Preferred == "" {
			name = strings.TrimSpace(person.Names.Given + " " + person.Names.Family)
		}
		values = append(values, StatementSubjectOption{Type: "person", ID: person.ID, Name: name, BranchID: person.BranchID})
	}
	householdCursor, err := r.database.Collection("chms_households").Find(ctx, bson.M{"organizationId": org, "archivedAt": nil, "name": pattern}, options.Find().SetProjection(bson.M{"name": 1, "homeBranchId": 1}).SetLimit(8))
	if err != nil {
		return nil, err
	}
	defer householdCursor.Close(ctx)
	for householdCursor.Next(ctx) {
		var household struct {
			ID       platform.ID `bson:"_id"`
			Name     string      `bson:"name"`
			BranchID platform.ID `bson:"homeBranchId"`
		}
		if err = householdCursor.Decode(&household); err != nil {
			return nil, err
		}
		values = append(values, StatementSubjectOption{Type: "household", ID: household.ID, Name: household.Name, BranchID: household.BranchID})
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].Name == values[j].Name {
			return values[i].ID < values[j].ID
		}
		return strings.ToLower(values[i].Name) < strings.ToLower(values[j].Name)
	})
	return values, nil
}

func (r *Repository) MemberCanAccessStatementSubject(ctx context.Context, org, member platform.ID, subjectType string, subjectID platform.ID) (bool, error) {
	if subjectType == "person" {
		return member == subjectID, nil
	}
	count, err := r.database.Collection("chms_household_memberships").CountDocuments(ctx, bson.M{"organizationId": org, "householdId": subjectID, "personId": member, "endedAt": nil})
	if err != nil || count != 1 {
		return false, err
	}
	count, err = r.database.Collection("chms_households").CountDocuments(ctx, bson.M{"_id": subjectID, "organizationId": org, "primaryContactPersonId": member, "statementPreference": "household", "archivedAt": nil})
	return count == 1, err
}

func (r *Repository) BuildStatementRows(ctx context.Context, org platform.ID, subject StatementSubject, branch platform.ID, from, to time.Time) ([]StatementRow, error) {
	filter := bson.M{"organizationId": org, "state": "posted", "receivedAt": bson.M{"$gte": from, "$lt": to}}
	filter["donor."+subject.Type+"Id"] = subject.ID
	if branch.Valid() {
		filter["branchId"] = branch
	}
	cursor, err := r.database.Collection(contributionsCollection).Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "receivedAt", Value: 1}, {Key: "receiptNumber", Value: 1}, {Key: "_id", Value: 1}}).SetLimit(10000))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	values := []Contribution{}
	if err = cursor.All(ctx, &values); err != nil {
		return nil, err
	}
	return r.StatementRowsForContributions(ctx, org, values)
}

func (r *Repository) StatementRowsForContributions(ctx context.Context, org platform.ID, values []Contribution) ([]StatementRow, error) {
	fundIDs := []platform.ID{}
	seen := map[platform.ID]bool{}
	for _, contribution := range values {
		for _, split := range contribution.Splits {
			if !seen[split.FundID] {
				seen[split.FundID], fundIDs = true, append(fundIDs, split.FundID)
			}
		}
	}
	funds := map[platform.ID]string{}
	if len(fundIDs) > 0 {
		cursor, err := r.database.Collection(fundsCollection).Find(ctx, bson.M{"organizationId": org, "_id": bson.M{"$in": fundIDs}}, options.Find().SetProjection(bson.M{"name": 1}))
		if err != nil {
			return nil, err
		}
		defer cursor.Close(ctx)
		for cursor.Next(ctx) {
			var fund struct {
				ID   platform.ID `bson:"_id"`
				Name string      `bson:"name"`
			}
			if err = cursor.Decode(&fund); err != nil {
				return nil, err
			}
			funds[fund.ID] = fund.Name
		}
	}
	rows := make([]StatementRow, 0, len(values))
	for _, contribution := range values {
		kind := "gift"
		if contribution.Link != nil {
			kind = contribution.Link.Type
		}
		allocations := make([]StatementAllocation, 0, len(contribution.Splits))
		for _, split := range contribution.Splits {
			name := funds[split.FundID]
			if name == "" {
				name = "Historical fund"
			}
			allocations = append(allocations, StatementAllocation{FundID: split.FundID, Fund: name, Amount: split.Amount.AmountMinor})
		}
		sort.Slice(allocations, func(i, j int) bool { return allocations[i].FundID < allocations[j].FundID })
		rows = append(rows, StatementRow{ContributionID: contribution.ID, ReceiptNumber: contribution.ReceiptNumber, ReceivedAt: contribution.ReceivedAt.UTC(), BranchID: contribution.BranchID, Source: contribution.Source, Kind: kind, AmountMinor: contribution.Total.AmountMinor, Allocations: allocations})
	}
	return rows, nil
}

func (r *Repository) FindStatementBySource(ctx context.Context, org platform.ID, scopeKey, sourceHash string) (*StatementRun, error) {
	var value StatementRun
	err := r.database.Collection(statementRunsCollection).FindOne(ctx, bson.M{"organizationId": org, "scopeKey": scopeKey, "sourceHash": sourceHash}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &value, err
}

func (r *Repository) FindLatestStatement(ctx context.Context, org platform.ID, scopeKey string) (*StatementRun, error) {
	var value StatementRun
	err := r.database.Collection(statementRunsCollection).FindOne(ctx, bson.M{"organizationId": org, "scopeKey": scopeKey}, options.FindOne().SetSort(bson.D{{Key: "statementVersion", Value: -1}})).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &value, err
}

func (r *Repository) FindStatement(ctx context.Context, org, id platform.ID) (*StatementRun, error) {
	return findOne[StatementRun](ctx, r, statementRunsCollection, org, id)
}

func (r *Repository) DecorateStatement(ctx context.Context, value *StatementRun) (*StatementRun, error) {
	count, err := r.database.Collection(documentDeliveriesCollection).CountDocuments(ctx, bson.M{"organizationId": value.OrganizationID, "documentType": "statement", "documentId": value.ID, "state": "delivered"})
	if err != nil {
		return nil, err
	}
	value.DeliveryCount = count
	return value, nil
}

func (r *Repository) ListStatements(ctx context.Context, org platform.ID, subjectType string, subjectID platform.ID) ([]StatementRun, error) {
	cursor, err := r.database.Collection(statementRunsCollection).Find(ctx, bson.M{"organizationId": org, "subject.type": subjectType, "subject.id": subjectID}, options.Find().SetSort(bson.D{{Key: "generatedAt", Value: -1}}).SetLimit(100))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	values := []StatementRun{}
	if err = cursor.All(ctx, &values); err != nil {
		return nil, err
	}
	for index := range values {
		if _, err = r.DecorateStatement(ctx, &values[index]); err != nil {
			return nil, err
		}
	}
	return values, nil
}

func (r *Repository) VerifiedStatementEmail(ctx context.Context, org platform.ID, subject StatementSubject) (string, error) {
	personID := subject.ID
	if subject.Type == "household" {
		var household struct {
			Primary platform.ID `bson:"primaryContactPersonId"`
		}
		if err := r.database.Collection("chms_households").FindOne(ctx, bson.M{"_id": subject.ID, "organizationId": org, "statementPreference": "household"}).Decode(&household); err != nil {
			return "", err
		}
		personID = household.Primary
	}
	var account struct {
		Email string `bson:"email"`
	}
	err := r.database.Collection("chms_member_accounts").FindOne(ctx, bson.M{"organizationId": org, "personId": personID, "status": "active", "emailVerifiedAt": bson.M{"$exists": true}}, options.FindOne().SetProjection(bson.M{"email": 1})).Decode(&account)
	if err == mongo.ErrNoDocuments {
		return "", nil
	}
	return strings.ToLower(strings.TrimSpace(account.Email)), err
}

func (r *Repository) FindDocumentDelivery(ctx context.Context, org platform.ID, documentType string, documentID platform.ID, requestKey string) (*DocumentDelivery, error) {
	var value DocumentDelivery
	err := r.database.Collection(documentDeliveriesCollection).FindOne(ctx, bson.M{"organizationId": org, "documentType": documentType, "documentId": documentID, "requestKey": requestKey}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &value, err
}

func (r *Repository) CompleteDocumentDelivery(ctx context.Context, org, id platform.ID, state, failure string, at time.Time) error {
	set := bson.M{"state": state}
	if state == "delivered" {
		set["deliveredAt"] = at
	} else {
		set["failureReason"] = failure
	}
	result, err := r.database.Collection(documentDeliveriesCollection).UpdateOne(ctx, bson.M{"_id": id, "organizationId": org, "state": "sending"}, bson.M{"$set": set})
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return platform.VersionConflict(1)
	}
	return nil
}
