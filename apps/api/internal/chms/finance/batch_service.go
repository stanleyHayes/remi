package finance

import (
	"context"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"remi-api/internal/chms/platform"
)

type BatchDetail struct {
	Batch         *CountingBatch      `json:"batch"`
	Entries       []BatchEntry        `json:"entries"`
	Confirmations []CountConfirmation `json:"confirmations"`
	Events        []BatchEvent        `json:"events"`
}

func (s Service) allowedBatch(p platform.Principal, action string, branch platform.ID, recentMFA bool) bool {
	return s.Authorizer != nil && s.Authorizer.Authorize(p, platform.AccessRequest{Action: action, ResourceType: "finance-batch", OrganizationID: p.OrganizationID, BranchID: branch, FieldClasses: []platform.FieldClass{platform.FieldFinancial}, RequireRecentMFA: recentMFA, Now: s.now()}).Allowed
}
func batchDenied() error {
	return &platform.DomainError{Code: "not_found", Message: "Counting batch not found."}
}
func containsActor(values []platform.ID, id platform.ID) bool {
	for _, value := range values {
		if value == id {
			return true
		}
	}
	return false
}
func containsMethod(values []platform.ID, id platform.ID) bool {
	for _, value := range values {
		if value == id {
			return true
		}
	}
	return false
}
func hasRole(p platform.Principal, role string) bool {
	for _, value := range p.Roles {
		if value == role {
			return true
		}
	}
	return false
}
func (s Service) batchAudit(ctx context.Context, p platform.Principal, b *CountingBatch, action string, fields []string, reason, requestID string, now time.Time) error {
	return s.Platform.AppendAudit(ctx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: b.BranchID, Actor: p.Actor, Action: action, ResourceType: "counting-batch", ResourceID: b.ID, ChangedFields: fields, Outcome: "success", Reason: reason, RequestID: requestID, OccurredAt: now})
}
func (s Service) batchEvent(ctx context.Context, p platform.Principal, b *CountingBatch, sequence int64, event, reason string, now time.Time) error {
	return s.Repository.InsertBatchEvent(ctx, BatchEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: b.BranchID, BatchID: b.ID, Sequence: sequence, Type: event, Actor: p.Actor, Reason: reason, OccurredAt: now})
}

func (s Service) CreateBatch(ctx context.Context, p platform.Principal, input CountingBatchInput, requestID string) (*CountingBatch, error) {
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_batch", Message: err.Error()})
	}
	if !s.allowedBatch(p, "create", input.BranchID, false) {
		return nil, batchDenied()
	}
	campus, err := s.Repository.CampusExists(ctx, p.OrganizationID, input.BranchID)
	if err != nil {
		return nil, err
	}
	if !campus {
		return nil, platform.ValidationError(platform.FieldError{Path: "branchId", Code: "not_configured", Message: "Branch finance settings are not configured."})
	}
	for _, id := range input.ExpectedPaymentMethodIDs {
		method, e := s.Repository.FindActivePaymentMethod(ctx, p.OrganizationID, id)
		if e != nil {
			return nil, e
		}
		if method == nil || !map[string]bool{"cash": true, "cheque": true, "mobile-money": true, "bank-transfer": true, "in-kind": true}[method.Kind] {
			return nil, platform.ValidationError(platform.FieldError{Path: "expectedPaymentMethodIds", Code: "invalid", Message: "Every expected method must be an active offline tender."})
		}
	}
	for _, counterID := range input.CounterIDs {
		exists, e := s.Repository.StaffHasRole(ctx, counterID, "finance-counter", "finance-admin", "super-admin")
		if e != nil {
			return nil, e
		}
		if !exists {
			return nil, platform.ValidationError(platform.FieldError{Path: "counterIds", Code: "invalid_operator", Message: "Every counter must be an active finance counter or administrator."})
		}
	}
	now := s.now()
	value := CountingBatch{ResourceEnvelope: envelope(p, input.BranchID, now), ReceivedAt: input.ReceivedAt.UTC(), OccurrenceID: input.OccurrenceID, CounterIDs: input.CounterIDs, DualControlRequired: input.DualControlRequired, ExpectedPaymentMethodIDs: input.ExpectedPaymentMethodIDs, State: "open", EnteredTotal: platform.Money{Currency: "GHS"}, DeclaredTotal: platform.Money{Currency: "GHS"}, Variance: platform.Money{Currency: "GHS"}}
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if e := s.Repository.InsertBatch(tx, value); e != nil {
			return e
		}
		if e := s.batchEvent(tx, p, &value, 1, "batch.created", "", now); e != nil {
			return e
		}
		return s.batchAudit(tx, p, &value, "finance.batch.create", []string{"branch", "receivedAt", "counters", "methods"}, "", requestID, now)
	})
	if err != nil {
		return nil, err
	}
	return s.Repository.FindBatch(ctx, p.OrganizationID, value.ID)
}
func (s Service) StartBatch(ctx context.Context, p platform.Principal, id platform.ID, input BatchTransitionInput, requestID string) (*CountingBatch, error) {
	if err := platform.RequireExpectedVersion(input.ExpectedVersion); err != nil {
		return nil, err
	}
	batch, err := s.Repository.FindBatch(ctx, p.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if batch == nil || batch.State != "open" || !containsActor(batch.CounterIDs, p.Actor.ID) || !s.allowedBatch(p, "operate", batch.BranchID, false) {
		return nil, batchDenied()
	}
	now := s.now()
	set := bson.M{"state": "counting", "version": batch.Version + 1, "updatedAt": now, "updatedBy": p.Actor}
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if e := s.Repository.UpdateBatch(tx, p.OrganizationID, id, batch.Version, []string{"open"}, set); e != nil {
			return e
		}
		if e := s.batchEvent(tx, p, batch, batch.Version+1, "batch.counting-started", "", now); e != nil {
			return e
		}
		return s.batchAudit(tx, p, batch, "finance.batch.start", []string{"state"}, "", requestID, now)
	})
	if err != nil {
		return nil, err
	}
	return s.Repository.FindBatch(ctx, p.OrganizationID, id)
}

func (s Service) validateBatchEntry(ctx context.Context, p platform.Principal, batch *CountingBatch, input *BatchEntryInput) error {
	if err := input.NormalizeAndValidate(); err != nil {
		return platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_batch_entry", Message: err.Error()})
	}
	if !containsMethod(batch.ExpectedPaymentMethodIDs, input.PaymentMethodID) {
		return platform.ValidationError(platform.FieldError{Path: "paymentMethodId", Code: "unexpected", Message: "Payment method is not enabled for this batch."})
	}
	method, err := s.Repository.FindActivePaymentMethod(ctx, p.OrganizationID, input.PaymentMethodID)
	if err != nil {
		return err
	}
	if method == nil || !methodMatchesSource(method.Kind, input.Source) {
		return platform.ValidationError(platform.FieldError{Path: "paymentMethodId", Code: "source_mismatch", Message: "Payment method does not match entry source."})
	}
	exists, err := s.Repository.DonorExists(ctx, p.OrganizationID, input.Donor)
	if err != nil {
		return err
	}
	if !exists {
		return platform.ValidationError(platform.FieldError{Path: "donor", Code: "not_found", Message: "Donor record does not exist."})
	}
	for _, split := range input.Splits {
		fund, e := s.Repository.FindActiveFund(ctx, p.OrganizationID, split.FundID, batch.ReceivedAt)
		if e != nil {
			return e
		}
		if fund == nil {
			return platform.ValidationError(platform.FieldError{Path: "splits", Code: "inactive_fund", Message: "Every split requires a fund active on the received date."})
		}
	}
	period, err := s.Repository.FindPeriodForDate(ctx, p.OrganizationID, batch.ReceivedAt)
	if err != nil {
		return err
	}
	if period == nil {
		return platform.ValidationError(platform.FieldError{Path: "receivedAt", Code: "no_open_period", Message: "Batch received date must be in an open fiscal period."})
	}
	return nil
}

func (s Service) SaveBatchEntry(ctx context.Context, p platform.Principal, batchID, entryID platform.ID, input BatchEntryInput, requestID string) (*BatchEntry, error) {
	batch, err := s.Repository.FindBatch(ctx, p.OrganizationID, batchID)
	if err != nil {
		return nil, err
	}
	if batch == nil || batch.State != "counting" || !containsActor(batch.CounterIDs, p.Actor.ID) || !s.allowedBatch(p, "operate", batch.BranchID, false) {
		return nil, batchDenied()
	}
	confirmations, err := s.Repository.ListConfirmations(ctx, p.OrganizationID, batchID)
	if err != nil {
		return nil, err
	}
	if len(confirmations) > 0 {
		return nil, &platform.DomainError{Code: "locked", Message: "Reset count confirmations before editing entries."}
	}
	if err = s.validateBatchEntry(ctx, p, batch, &input); err != nil {
		return nil, err
	}
	now := s.now()
	value := BatchEntry{ResourceEnvelope: envelope(p, batch.BranchID, now), BatchID: batchID, Donor: input.Donor, Source: input.Source, PaymentMethodID: input.PaymentMethodID, PaymentMethodReference: input.PaymentMethodReference, Total: input.Total, Splits: input.Splits, Provenance: input.Provenance}
	creating := !entryID.Valid()
	if !creating {
		if err = platform.RequireExpectedVersion(input.ExpectedVersion); err != nil {
			return nil, err
		}
		current, e := s.Repository.FindBatchEntry(ctx, p.OrganizationID, batchID, entryID)
		if e != nil {
			return nil, e
		}
		if current == nil {
			return nil, batchDenied()
		}
		value.ResourceEnvelope = current.ResourceEnvelope
		value.ID = entryID
		value.Version = current.Version + 1
	}
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if creating {
			if e := s.Repository.InsertBatchEntry(tx, value); e != nil {
				return e
			}
		} else {
			set := bson.M{"donor": value.Donor, "source": value.Source, "paymentMethodId": value.PaymentMethodID, "paymentMethodReference": value.PaymentMethodReference, "total": value.Total, "splits": value.Splits, "provenance": value.Provenance, "version": value.Version, "updatedAt": now, "updatedBy": p.Actor}
			if e := s.Repository.UpdateBatchEntry(tx, p.OrganizationID, batchID, entryID, input.ExpectedVersion, set); e != nil {
				return e
			}
		}
		total, count, e := s.Repository.BatchEntryTotals(tx, p.OrganizationID, batchID)
		if e != nil {
			return e
		}
		set := bson.M{"enteredTotal": platform.Money{AmountMinor: total, Currency: "GHS"}, "entryCount": count, "version": batch.Version + 1, "updatedAt": now, "updatedBy": p.Actor}
		if e = s.Repository.UpdateBatch(tx, p.OrganizationID, batchID, batch.Version, []string{"counting"}, set); e != nil {
			return e
		}
		event := "batch.entry-created"
		if !creating {
			event = "batch.entry-updated"
		}
		if e = s.batchEvent(tx, p, batch, batch.Version+1, event, "", now); e != nil {
			return e
		}
		return s.batchAudit(tx, p, batch, "finance."+event, []string{"entries", "enteredTotal"}, "", requestID, now)
	})
	if err != nil {
		return nil, err
	}
	return s.Repository.FindBatchEntry(ctx, p.OrganizationID, batchID, value.ID)
}

func normalizeTenderMap(values []TenderTotal) map[platform.ID]int64 {
	result := map[platform.ID]int64{}
	for _, value := range values {
		result[value.PaymentMethodID] = value.Amount.AmountMinor
	}
	return result
}
func sameTenderTotals(a, b []TenderTotal) bool {
	left, right := normalizeTenderMap(a), normalizeTenderMap(b)
	if len(left) != len(right) {
		return false
	}
	for id, value := range left {
		if right[id] != value {
			return false
		}
	}
	return true
}
func (s Service) validateConfirmationMethods(ctx context.Context, p platform.Principal, batch *CountingBatch, input CountConfirmationInput) error {
	if len(input.TenderTotals) != len(batch.ExpectedPaymentMethodIDs) {
		return platform.ValidationError(platform.FieldError{Path: "tenderTotals", Code: "incomplete", Message: "Declare every expected payment method, including zero totals."})
	}
	totals := normalizeTenderMap(input.TenderTotals)
	cashDenominations := map[platform.ID]int64{}
	for _, row := range input.Denominations {
		method, err := s.Repository.FindActivePaymentMethod(ctx, p.OrganizationID, row.PaymentMethodID)
		if err != nil {
			return err
		}
		if method == nil || method.Kind != "cash" {
			return platform.ValidationError(platform.FieldError{Path: "denominations", Code: "cash_only", Message: "Denominations are only valid for active cash methods."})
		}
		cashDenominations[row.PaymentMethodID] += row.TotalMinor
	}
	for _, id := range batch.ExpectedPaymentMethodIDs {
		if _, ok := totals[id]; !ok {
			return platform.ValidationError(platform.FieldError{Path: "tenderTotals", Code: "unexpected", Message: "Tender methods must match the batch."})
		}
		method, err := s.Repository.FindActivePaymentMethod(ctx, p.OrganizationID, id)
		if err != nil {
			return err
		}
		if method == nil {
			return platform.ValidationError(platform.FieldError{Path: "tenderTotals", Code: "inactive", Message: "Tender method is no longer active."})
		}
		if method.Kind == "cash" && cashDenominations[id] != totals[id] {
			return platform.ValidationError(platform.FieldError{Path: "denominations", Code: "cash_mismatch", Message: "Cash denominations must equal the declared cash total."})
		}
	}
	return nil
}

func (s Service) ConfirmBatchCount(ctx context.Context, p platform.Principal, id platform.ID, input CountConfirmationInput, requestID string) (*CountingBatch, error) {
	declared, err := input.NormalizeAndValidate()
	if err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_count_confirmation", Message: err.Error()})
	}
	batch, err := s.Repository.FindBatch(ctx, p.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if batch == nil || batch.State != "counting" || batch.Version != input.ExpectedVersion || !containsActor(batch.CounterIDs, p.Actor.ID) || !s.allowedBatch(p, "operate", batch.BranchID, false) {
		return nil, batchDenied()
	}
	if err = s.validateConfirmationMethods(ctx, p, batch, input); err != nil {
		return nil, err
	}
	entriesTotal, entryCount, err := s.Repository.BatchEntryTotals(ctx, p.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if entryCount < 1 {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "empty_batch", Message: "At least one contribution entry is required."})
	}
	confirmations, err := s.Repository.ListConfirmations(ctx, p.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	for _, confirmation := range confirmations {
		if confirmation.CounterID == p.Actor.ID {
			return nil, &platform.DomainError{Code: "conflict", Message: "This counter already confirmed the count."}
		}
		if !sameTenderTotals(confirmation.TenderTotals, input.TenderTotals) || confirmation.DeclaredTotal.AmountMinor != declared.AmountMinor {
			return nil, platform.ValidationError(platform.FieldError{Path: "tenderTotals", Code: "counter_mismatch", Message: "Independent counter totals do not match."})
		}
	}
	required := 1
	if batch.DualControlRequired {
		required = 2
	}
	nextCount := len(confirmations) + 1
	nextState := "counting"
	if nextCount >= required {
		nextState = "counted"
	}
	now := s.now()
	confirmation := CountConfirmation{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: batch.BranchID, BatchID: id, CounterID: p.Actor.ID, TenderTotals: input.TenderTotals, Denominations: input.Denominations, Attachments: input.Attachments, DeclaredTotal: declared, ConfirmedAt: now}
	variance := platform.Money{AmountMinor: entriesTotal - declared.AmountMinor, Currency: "GHS"}
	set := bson.M{"state": nextState, "entryCount": entryCount, "enteredTotal": platform.Money{AmountMinor: entriesTotal, Currency: "GHS"}, "declaredTotal": declared, "variance": variance, "confirmationCount": nextCount, "version": batch.Version + 1, "updatedAt": now, "updatedBy": p.Actor}
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if e := s.Repository.InsertConfirmation(tx, confirmation); e != nil {
			return e
		}
		if e := s.Repository.UpdateBatch(tx, p.OrganizationID, id, batch.Version, []string{"counting"}, set); e != nil {
			return e
		}
		event := "batch.count-confirmed"
		if nextState == "counted" {
			event = "batch.counted"
		}
		if e := s.batchEvent(tx, p, batch, batch.Version+1, event, "", now); e != nil {
			return e
		}
		return s.batchAudit(tx, p, batch, "finance."+event, []string{"state", "declaredTotal", "variance", "confirmations"}, "", requestID, now)
	})
	if err != nil {
		return nil, err
	}
	return s.Repository.FindBatch(ctx, p.OrganizationID, id)
}

func (s Service) ResetBatchCount(ctx context.Context, p platform.Principal, id platform.ID, input BatchTransitionInput, requestID string) (*CountingBatch, error) {
	if err := platform.RequireExpectedVersion(input.ExpectedVersion); err != nil {
		return nil, err
	}
	if err := platform.ValidateReason(input.Reason); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "reason", Code: "invalid", Message: err.Error()})
	}
	batch, err := s.Repository.FindBatch(ctx, p.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if batch == nil || (batch.State != "counting" && batch.State != "counted") || !s.allowedBatch(p, "update", batch.BranchID, false) {
		return nil, batchDenied()
	}
	now := s.now()
	set := bson.M{"state": "counting", "confirmationCount": 0, "declaredTotal": platform.Money{Currency: "GHS"}, "variance": platform.Money{Currency: "GHS"}, "version": batch.Version + 1, "updatedAt": now, "updatedBy": p.Actor}
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if e := s.Repository.DeleteConfirmations(tx, p.OrganizationID, id); e != nil {
			return e
		}
		if e := s.Repository.UpdateBatch(tx, p.OrganizationID, id, batch.Version, []string{"counting", "counted"}, set); e != nil {
			return e
		}
		if e := s.batchEvent(tx, p, batch, batch.Version+1, "batch.count-reset", input.Reason, now); e != nil {
			return e
		}
		return s.batchAudit(tx, p, batch, "finance.batch.count-reset", []string{"state", "confirmations"}, input.Reason, requestID, now)
	})
	if err != nil {
		return nil, err
	}
	return s.Repository.FindBatch(ctx, p.OrganizationID, id)
}

func (s Service) ApproveBatch(ctx context.Context, p platform.Principal, id platform.ID, input BatchTransitionInput, requestID string) (*CountingBatch, error) {
	if err := platform.RequireExpectedVersion(input.ExpectedVersion); err != nil {
		return nil, err
	}
	batch, err := s.Repository.FindBatch(ctx, p.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if batch == nil || batch.State != "counted" || containsActor(batch.CounterIDs, p.Actor.ID) || !s.allowedBatch(p, "approve", batch.BranchID, false) {
		return nil, batchDenied()
	}
	reason := strings.TrimSpace(input.Reason)
	if batch.Variance.AmountMinor != 0 {
		if err = platform.ValidateReason(reason); err != nil {
			return nil, platform.ValidationError(platform.FieldError{Path: "reason", Code: "variance_reason_required", Message: "A reviewed variance reason is required."})
		}
	}
	now := s.now()
	set := bson.M{"state": "approved", "approvedBy": p.Actor.ID, "approvedAt": now, "version": batch.Version + 1, "updatedAt": now, "updatedBy": p.Actor}
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if e := s.Repository.UpdateBatch(tx, p.OrganizationID, id, batch.Version, []string{"counted"}, set); e != nil {
			return e
		}
		if e := s.batchEvent(tx, p, batch, batch.Version+1, "batch.approved", reason, now); e != nil {
			return e
		}
		return s.batchAudit(tx, p, batch, "finance.batch.approve", []string{"state", "approver"}, reason, requestID, now)
	})
	if err != nil {
		return nil, err
	}
	return s.Repository.FindBatch(ctx, p.OrganizationID, id)
}

func (s Service) PostBatch(ctx context.Context, p platform.Principal, id platform.ID, input BatchTransitionInput, requestID string) (*CountingBatch, error) {
	if err := platform.RequireExpectedVersion(input.ExpectedVersion); err != nil {
		return nil, err
	}
	batch, err := s.Repository.FindBatch(ctx, p.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if batch == nil || batch.State != "approved" || containsActor(batch.CounterIDs, p.Actor.ID) || !s.allowedBatch(p, "approve", batch.BranchID, true) {
		return nil, batchDenied()
	}
	entries, err := s.Repository.ListBatchEntries(ctx, p.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if len(entries) < 1 || len(entries) > 500 || int64(len(entries)) != batch.EntryCount {
		return nil, platform.ValidationError(platform.FieldError{Path: "entries", Code: "invalid_count", Message: "Batch must contain one to 500 locked entries."})
	}
	now := s.now()
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		current, e := s.Repository.FindBatch(tx, p.OrganizationID, id)
		if e != nil {
			return e
		}
		if current == nil || current.State != "approved" || current.Version != batch.Version {
			return platform.VersionConflict(batch.Version)
		}
		period, e := s.Repository.FindPeriodForDate(tx, p.OrganizationID, batch.ReceivedAt)
		if e != nil {
			return e
		}
		if period == nil {
			return platform.ValidationError(platform.FieldError{Path: "receivedAt", Code: "no_open_period", Message: "Batch period is not open."})
		}
		sequence, e := s.Repository.FindSequenceForYear(tx, p.OrganizationID, batch.ReceivedAt.Year())
		if e != nil {
			return e
		}
		if sequence == nil {
			return platform.ValidationError(platform.FieldError{Path: "receivedAt", Code: "no_receipt_sequence", Message: "Receipt sequence is not configured."})
		}
		for _, entry := range entries {
			inputEntry := BatchEntryInput{Donor: entry.Donor, Source: entry.Source, PaymentMethodID: entry.PaymentMethodID, PaymentMethodReference: entry.PaymentMethodReference, Total: entry.Total, Splits: entry.Splits, Provenance: entry.Provenance}
			if e = s.validateBatchEntry(tx, p, batch, &inputEntry); e != nil {
				return e
			}
			commandKey := "batch:" + string(id) + ":" + string(entry.ID)
			if existing, e := s.Repository.FindContributionByCommand(tx, p.OrganizationID, commandKey); e != nil {
				return e
			} else if existing != nil {
				return &platform.DomainError{Code: "conflict", Message: "A batch entry was already posted."}
			}
			value := Contribution{ResourceEnvelope: envelope(p, batch.BranchID, now), ReceivedAt: batch.ReceivedAt, PostedAt: now, Donor: entry.Donor, Source: entry.Source, PaymentMethodID: entry.PaymentMethodID, PaymentMethodReference: entry.PaymentMethodReference, BatchID: id, FiscalPeriodID: period.ID, Total: entry.Total, Splits: entry.Splits, State: "posted", EffectiveState: "posted", Provenance: entry.Provenance, CommandKey: commandKey, CommandHash: platform.RequestHash([]byte(commandKey))}
			receipt, e := s.Repository.ReserveReceipt(tx, p.OrganizationID, sequence.ID, now, p.Actor)
			if e != nil {
				return e
			}
			value.ReceiptNumber = receipt
			if e = s.Repository.InsertReceiptAllocation(tx, ReceiptAllocation{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, SequenceID: sequence.ID, ReceiptNumber: receipt, ContributionID: value.ID, State: "posted", AllocatedAt: now}); e != nil {
				return e
			}
			if e = s.Repository.InsertContribution(tx, value); e != nil {
				return e
			}
			if e = s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: batch.BranchID, Actor: p.Actor, Action: "finance.contribution.post", ResourceType: "contribution", ResourceID: value.ID, SubjectIDs: donorSubjectIDs(value.Donor), ChangedFields: []string{"batch", "donor", "source", "total", "splits", "period", "receipt"}, Outcome: "success", RequestID: requestID, OccurredAt: now}); e != nil {
				return e
			}
			if e = s.Platform.EnqueueEvent(tx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: batch.BranchID, Type: "finance.contribution.posted", EventVersion: 1, AggregateType: "contribution", AggregateID: value.ID, AggregateVersion: 1, Actor: p.Actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: bson.M{"contributionId": value.ID, "batchId": id}}, State: "pending", AvailableAt: now}); e != nil {
				return e
			}
		}
		set := bson.M{"state": "posted", "postedBy": p.Actor.ID, "postedAt": now, "version": batch.Version + 1, "updatedAt": now, "updatedBy": p.Actor}
		if e = s.Repository.UpdateBatch(tx, p.OrganizationID, id, batch.Version, []string{"approved"}, set); e != nil {
			return e
		}
		if e = s.batchEvent(tx, p, batch, batch.Version+1, "batch.posted", strings.TrimSpace(input.Reason), now); e != nil {
			return e
		}
		return s.batchAudit(tx, p, batch, "finance.batch.post", []string{"state", "contributions", "receipts"}, strings.TrimSpace(input.Reason), requestID, now)
	})
	if err != nil {
		return nil, err
	}
	return s.Repository.FindBatch(ctx, p.OrganizationID, id)
}

func (s Service) GetBatch(ctx context.Context, p platform.Principal, id platform.ID) (*BatchDetail, error) {
	batch, err := s.Repository.FindBatch(ctx, p.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if batch == nil || !s.allowedBatch(p, "read", batch.BranchID, false) || (hasRole(p, "finance-counter") && !containsActor(batch.CounterIDs, p.Actor.ID)) {
		return nil, batchDenied()
	}
	entries, err := s.Repository.ListBatchEntries(ctx, p.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	confirmations, err := s.Repository.ListConfirmations(ctx, p.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	events, err := s.Repository.ListBatchEvents(ctx, p.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	return &BatchDetail{Batch: batch, Entries: entries, Confirmations: confirmations, Events: events}, nil
}
func (s Service) ListBatches(ctx context.Context, p platform.Principal, branch platform.ID, limit int64) ([]CountingBatch, error) {
	if !branch.Valid() {
		return nil, platform.ValidationError(platform.FieldError{Path: "branchId", Code: "required", Message: "A branch scope is required."})
	}
	if !s.allowedBatch(p, "read", branch, false) {
		return nil, batchDenied()
	}
	counterID := platform.ID("")
	if hasRole(p, "finance-counter") {
		counterID = p.Actor.ID
	}
	return s.Repository.ListBatches(ctx, p.OrganizationID, branch, counterID, limit)
}
