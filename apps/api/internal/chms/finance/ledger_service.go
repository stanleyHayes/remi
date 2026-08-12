package finance

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"remi-api/internal/chms/platform"
)

type idempotencyPlatform interface {
	Begin(context.Context, string, string, string, time.Time, *time.Time) (platform.IdempotencyRecord, bool, error)
	Complete(context.Context, string, string, string, int, []byte, platform.ID) error
	Release(context.Context, string, string, string) error
}

func (s Service) allowedLedger(p platform.Principal, action string, branch platform.ID, recentMFA bool) bool {
	if p.Actor.Type == platform.ActorSystem && len(p.Roles) == 1 && p.Roles[0] == "payment-provider" && (action == "create" || action == "approve") {
		return true
	}
	return s.Authorizer != nil && s.Authorizer.Authorize(p, platform.AccessRequest{Action: action, ResourceType: "finance-ledger", OrganizationID: p.OrganizationID, BranchID: branch, FieldClasses: []platform.FieldClass{platform.FieldFinancial}, RequireRecentMFA: recentMFA, Now: s.now()}).Allowed
}
func ledgerDenied() error {
	return &platform.DomainError{Code: "not_found", Message: "Contribution not found."}
}
func commandHash(value any) (string, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return platform.RequestHash(body), nil
}
func (s Service) beginIdempotent(ctx context.Context, scope, key, hash string) (platform.IdempotencyRecord, bool, error) {
	if err := platform.ValidateIdempotencyKey(key); err != nil {
		return platform.IdempotencyRecord{}, false, platform.ValidationError(platform.FieldError{Path: "Idempotency-Key", Code: "invalid", Message: err.Error()})
	}
	store, ok := s.Platform.(idempotencyPlatform)
	if !ok {
		return platform.IdempotencyRecord{}, true, nil
	}
	expiry := s.now().Add(24 * time.Hour)
	return store.Begin(ctx, scope, key, hash, s.now(), &expiry)
}
func (s Service) finishIdempotent(ctx context.Context, scope, key, hash string, status int, body []byte, id platform.ID) error {
	if store, ok := s.Platform.(idempotencyPlatform); ok {
		return store.Complete(ctx, scope, key, hash, status, body, id)
	}
	return nil
}
func (s Service) releaseIdempotent(ctx context.Context, scope, key, hash string) {
	if store, ok := s.Platform.(idempotencyPlatform); ok {
		_ = store.Release(ctx, scope, key, hash)
	}
}
func replayContribution(record platform.IdempotencyRecord) (*Contribution, error) {
	if record.State == "completed" && len(record.ResponseBody) > 0 {
		var value Contribution
		if err := json.Unmarshal(record.ResponseBody, &value); err != nil {
			return nil, err
		}
		return &value, nil
	}
	return nil, &platform.DomainError{Code: "command_in_progress", Message: "This contribution command is already processing."}
}

func (s Service) validatePosting(ctx context.Context, p platform.Principal, input ContributionInput) (*FiscalPeriod, *ReceiptSequence, error) {
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_contribution", Message: err.Error()})
	}
	if input.BatchID.Valid() {
		return nil, nil, platform.ValidationError(platform.FieldError{Path: "batchId", Code: "batch_workflow_required", Message: "Offline contributions must be posted by the controlled counting-batch workflow."})
	}
	if !s.allowedLedger(p, "create", input.BranchID, true) {
		return nil, nil, ledgerDenied()
	}
	campus, err := s.Repository.CampusExists(ctx, p.OrganizationID, input.BranchID)
	if err != nil {
		return nil, nil, err
	}
	if !campus {
		return nil, nil, platform.ValidationError(platform.FieldError{Path: "branchId", Code: "not_configured", Message: "Branch finance settings are not configured."})
	}
	donor, err := s.Repository.DonorExists(ctx, p.OrganizationID, input.Donor)
	if err != nil {
		return nil, nil, err
	}
	if !donor {
		return nil, nil, platform.ValidationError(platform.FieldError{Path: "donor", Code: "not_found", Message: "Donor record does not exist."})
	}
	method, err := s.Repository.FindActivePaymentMethod(ctx, p.OrganizationID, input.PaymentMethodID)
	if err != nil {
		return nil, nil, err
	}
	if method == nil {
		return nil, nil, platform.ValidationError(platform.FieldError{Path: "paymentMethodId", Code: "inactive", Message: "Payment method is not active."})
	}
	if !methodMatchesSource(method.Kind, input.Source) {
		return nil, nil, platform.ValidationError(platform.FieldError{Path: "paymentMethodId", Code: "source_mismatch", Message: "Payment method does not match contribution source."})
	}
	for _, split := range input.Splits {
		fund, e := s.Repository.FindActiveFund(ctx, p.OrganizationID, split.FundID, input.ReceivedAt)
		if e != nil {
			return nil, nil, e
		}
		if fund == nil {
			return nil, nil, platform.ValidationError(platform.FieldError{Path: "splits", Code: "inactive_fund", Message: "Every split requires a fund active on the received date."})
		}
	}
	if input.PledgeID.Valid() {
		pledge, e := s.Repository.FindPledge(ctx, p.OrganizationID, input.PledgeID)
		if e != nil {
			return nil, nil, e
		}
		if pledge == nil || pledge.State != "active" || pledge.Donor != input.Donor || len(input.Splits) != 1 || pledge.FundID != input.Splits[0].FundID || (pledge.CampaignID.Valid() && pledge.CampaignID != input.CampaignID) {
			return nil, nil, platform.ValidationError(platform.FieldError{Path: "pledgeId", Code: "attribution_mismatch", Message: "Pledge must be active and match the donor, fund and campaign."})
		}
	}
	period, err := s.Repository.FindPeriodForDate(ctx, p.OrganizationID, input.ReceivedAt)
	if err != nil {
		return nil, nil, err
	}
	if period == nil {
		return nil, nil, platform.ValidationError(platform.FieldError{Path: "receivedAt", Code: "no_open_period", Message: "Received date must fall within an open fiscal period."})
	}
	sequence, err := s.Repository.FindSequenceForYear(ctx, p.OrganizationID, input.ReceivedAt.Year())
	if err != nil {
		return nil, nil, err
	}
	if sequence == nil {
		return nil, nil, platform.ValidationError(platform.FieldError{Path: "receivedAt", Code: "no_receipt_sequence", Message: "Receipt sequence is not configured for this fiscal year."})
	}
	return period, sequence, nil
}
func methodMatchesSource(kind, source string) bool {
	if kind == source {
		return true
	}
	if source == "online" {
		return kind == "card" || kind == "mobile-money" || kind == "bank-transfer"
	}
	if source == "import" {
		return true
	}
	return false
}
func (s Service) PostContribution(ctx context.Context, p platform.Principal, input ContributionInput, requestID, idempotencyKey string) (*Contribution, error) {
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_contribution", Message: err.Error()})
	}
	if !s.allowedLedger(p, "create", input.BranchID, true) {
		return nil, ledgerDenied()
	}
	hash, err := commandHash(input)
	if err != nil {
		return nil, err
	}
	scope := "finance.contribution.post:" + string(p.OrganizationID)
	record, acquired, err := s.beginIdempotent(ctx, scope, idempotencyKey, hash)
	if err != nil {
		return nil, err
	}
	if !acquired {
		return replayContribution(record)
	}
	if existing, e := s.Repository.FindContributionByCommand(ctx, p.OrganizationID, idempotencyKey); e != nil {
		s.releaseIdempotent(ctx, scope, idempotencyKey, hash)
		return nil, e
	} else if existing != nil {
		if existing.CommandHash != hash {
			s.releaseIdempotent(ctx, scope, idempotencyKey, hash)
			return nil, &platform.DomainError{Code: "idempotency_key_reused", Message: "This idempotency key was already used for a different request."}
		}
		decorateEffectiveState(existing, nil)
		body, _ := json.Marshal(existing)
		if e = s.finishIdempotent(ctx, scope, idempotencyKey, hash, 201, body, existing.ID); e != nil {
			return nil, e
		}
		return existing, nil
	}
	period, _, err := s.validatePosting(ctx, p, input)
	if err != nil {
		s.releaseIdempotent(ctx, scope, idempotencyKey, hash)
		return nil, err
	}
	now := s.now()
	value := Contribution{ResourceEnvelope: envelope(p, input.BranchID, now), ReceivedAt: input.ReceivedAt.UTC(), PostedAt: now, Donor: input.Donor, Source: input.Source, PaymentMethodID: input.PaymentMethodID, PaymentMethodReference: input.PaymentMethodReference, ProviderReference: input.ProviderReference, SourceReceiptNumber: input.SourceReceiptNumber, BatchID: input.BatchID, CampaignID: input.CampaignID, PledgeID: input.PledgeID, FiscalPeriodID: period.ID, Total: input.Total, Splits: input.Splits, State: "posted", EffectiveState: "posted", Provenance: input.Provenance, CommandKey: idempotencyKey, CommandHash: hash}
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		validatedPeriod, validatedSequence, e := s.validatePosting(tx, p, input)
		if e != nil {
			return e
		}
		value.FiscalPeriodID = validatedPeriod.ID
		receipt, e := s.Repository.ReserveReceipt(tx, p.OrganizationID, validatedSequence.ID, now, p.Actor)
		if e != nil {
			return e
		}
		value.ReceiptNumber = receipt
		allocation := ReceiptAllocation{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, SequenceID: validatedSequence.ID, ReceiptNumber: receipt, ContributionID: value.ID, State: "posted", AllocatedAt: now}
		if e = s.Repository.InsertReceiptAllocation(tx, allocation); e != nil {
			return e
		}
		if e = s.Repository.InsertContribution(tx, value); e != nil {
			return e
		}
		if e = s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: input.BranchID, Actor: p.Actor, Action: "finance.contribution.post", ResourceType: "contribution", ResourceID: value.ID, SubjectIDs: donorSubjectIDs(value.Donor), ChangedFields: []string{"donor", "source", "paymentMethod", "total", "splits", "period", "receipt"}, Outcome: "success", RequestID: requestID, OccurredAt: now}); e != nil {
			return e
		}
		if e = s.Platform.EnqueueEvent(tx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: input.BranchID, Type: "finance.contribution.posted", EventVersion: 1, AggregateType: "contribution", AggregateID: value.ID, AggregateVersion: 1, Actor: p.Actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: bson.M{"contributionId": value.ID, "fiscalPeriodId": value.FiscalPeriodID}}, State: "pending", AvailableAt: now}); e != nil {
			return e
		}
		body, _ := json.Marshal(value)
		return s.finishIdempotent(tx, scope, idempotencyKey, hash, 201, body, value.ID)
	})
	if err != nil {
		s.releaseIdempotent(ctx, scope, idempotencyKey, hash)
		return nil, err
	}
	return &value, nil
}
func donorSubjectIDs(d DonorAttribution) []platform.ID {
	if d.PersonID.Valid() {
		return []platform.ID{d.PersonID}
	}
	if d.HouseholdID.Valid() {
		return []platform.ID{d.HouseholdID}
	}
	return nil
}
func (s Service) GetContribution(ctx context.Context, p platform.Principal, id platform.ID) (*Contribution, error) {
	value, err := s.Repository.FindContribution(ctx, p.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if value == nil || !s.allowedLedger(p, "read", value.BranchID, false) {
		return nil, ledgerDenied()
	}
	reversal, err := s.Repository.FindReversal(ctx, p.OrganizationID, value.ID)
	if err != nil {
		return nil, err
	}
	decorateEffectiveState(value, reversal)
	return value, nil
}
func (s Service) ListContributions(ctx context.Context, p platform.Principal, branch platform.ID, limit int64) ([]Contribution, error) {
	if !branch.Valid() {
		return nil, platform.ValidationError(platform.FieldError{Path: "branchId", Code: "required", Message: "A branch scope is required."})
	}
	if !s.allowedLedger(p, "read", branch, false) {
		return nil, ledgerDenied()
	}
	values, err := s.Repository.ListContributions(ctx, p.OrganizationID, branch, limit)
	if err != nil {
		return nil, err
	}
	ids := make([]platform.ID, 0, len(values))
	for idx := range values {
		if values[idx].Link == nil {
			ids = append(ids, values[idx].ID)
		}
	}
	reversals, err := s.Repository.FindReversals(ctx, p.OrganizationID, ids)
	if err != nil {
		return nil, err
	}
	for idx := range values {
		var reversal *Contribution
		if found, ok := reversals[values[idx].ID]; ok {
			copy := found
			reversal = &copy
		}
		decorateEffectiveState(&values[idx], reversal)
	}
	return values, nil
}
func decorateEffectiveState(value *Contribution, reversal *Contribution) {
	if value.Link != nil {
		value.EffectiveState = "adjustment"
		return
	}
	value.EffectiveState = value.State
	if reversal != nil && reversal.Link != nil {
		if reversal.Link.Type == "reversal" {
			value.EffectiveState = "reversed"
		} else {
			value.EffectiveState = "corrected"
		}
	}
}

func (s Service) AdjustContribution(ctx context.Context, p platform.Principal, originalID platform.ID, input AdjustmentInput, requestID, idempotencyKey string) (*AdjustmentResult, error) {
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_adjustment", Message: err.Error()})
	}
	providerAdjustment := input.Type == "provider-refund" || input.Type == "provider-chargeback"
	if providerAdjustment && p.Actor.Type != platform.ActorSystem {
		return nil, ledgerDenied()
	}
	original, err := s.Repository.FindContribution(ctx, p.OrganizationID, originalID)
	if err != nil {
		return nil, err
	}
	if original == nil || original.State != "posted" || (original.Link != nil && original.Link.ReversesContributionID.Valid()) || !s.allowedLedger(p, "approve", original.BranchID, true) {
		return nil, ledgerDenied()
	}
	hash, err := commandHash(input)
	if err != nil {
		return nil, err
	}
	scope := "finance.contribution.adjust:" + string(originalID)
	record, acquired, err := s.beginIdempotent(ctx, scope, idempotencyKey, hash)
	if err != nil {
		return nil, err
	}
	if !acquired {
		if record.State == "completed" {
			var value AdjustmentResult
			if e := json.Unmarshal(record.ResponseBody, &value); e != nil {
				return nil, e
			}
			return &value, nil
		}
		return nil, &platform.DomainError{Code: "command_in_progress", Message: "This adjustment command is already processing."}
	}
	if existing, e := s.Repository.FindContributionByCommand(ctx, p.OrganizationID, idempotencyKey+":reversal"); e != nil {
		s.releaseIdempotent(ctx, scope, idempotencyKey, hash)
		return nil, e
	} else if existing != nil {
		if existing.CommandHash != hash {
			s.releaseIdempotent(ctx, scope, idempotencyKey, hash)
			return nil, &platform.DomainError{Code: "idempotency_key_reused", Message: "This idempotency key was already used for a different request."}
		}
		replacement, _ := s.Repository.FindContributionByCommand(ctx, p.OrganizationID, idempotencyKey+":replacement")
		result := &AdjustmentResult{Reversal: existing, Replacement: replacement}
		body, _ := json.Marshal(result)
		if e = s.finishIdempotent(ctx, scope, idempotencyKey, hash, 201, body, existing.ID); e != nil {
			return nil, e
		}
		return result, nil
	}
	existingReversal, err := s.Repository.FindReversal(ctx, p.OrganizationID, originalID)
	if err != nil {
		s.releaseIdempotent(ctx, scope, idempotencyKey, hash)
		return nil, err
	}
	if existingReversal != nil && !providerAdjustment {
		s.releaseIdempotent(ctx, scope, idempotencyKey, hash)
		return nil, &platform.DomainError{Code: "immutable", Message: "This contribution already has a reversal or correction."}
	}
	now := s.now()
	adjustmentAmount := original.Total.AmountMinor
	if providerAdjustment {
		adjustmentAmount = input.AmountMinor
		alreadyAdjusted, e := s.Repository.SumProviderAdjustments(ctx, p.OrganizationID, original.ID)
		if e != nil {
			s.releaseIdempotent(ctx, scope, idempotencyKey, hash)
			return nil, e
		}
		remaining := original.Total.AmountMinor + alreadyAdjusted
		if adjustmentAmount > remaining {
			s.releaseIdempotent(ctx, scope, idempotencyKey, hash)
			return nil, platform.ValidationError(platform.FieldError{Path: "amountMinor", Code: "exceeds_remaining", Message: "Provider adjustment exceeds the remaining contribution amount."})
		}
	}
	period, err := s.Repository.FindPeriodForDate(ctx, p.OrganizationID, now)
	if err != nil || period == nil {
		s.releaseIdempotent(ctx, scope, idempotencyKey, hash)
		if err != nil {
			return nil, err
		}
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "no_open_period", Message: "An open current fiscal period is required for adjustments."})
	}
	sequence, err := s.Repository.FindSequenceForYear(ctx, p.OrganizationID, now.Year())
	if err != nil || sequence == nil {
		s.releaseIdempotent(ctx, scope, idempotencyKey, hash)
		if err != nil {
			return nil, err
		}
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "no_receipt_sequence", Message: "A current receipt sequence is required for adjustments."})
	}
	root := original.ID
	if original.Link != nil && original.Link.ChainRootID.Valid() {
		root = original.Link.ChainRootID
	}
	link := &ContributionLink{Type: input.Type, ChainRootID: root, ReversesContributionID: original.ID, Reason: input.Reason}
	if providerAdjustment {
		link.ReversesContributionID = ""
		link.ProviderAdjustsID = original.ID
		link.ProviderEventReference = input.ProviderEventReference
	}
	reversal := Contribution{ResourceEnvelope: envelope(p, original.BranchID, now), ReceivedAt: now, PostedAt: now, Donor: original.Donor, Source: "adjustment", PaymentMethodID: original.PaymentMethodID, CampaignID: original.CampaignID, PledgeID: original.PledgeID, FiscalPeriodID: period.ID, Total: platform.Money{AmountMinor: -adjustmentAmount, Currency: original.Total.Currency}, State: "posted", EffectiveState: "adjustment", Provenance: "paystack-webhook", CommandKey: idempotencyKey + ":reversal", CommandHash: hash, Link: link}
	remainingAllocation := adjustmentAmount
	for idx, split := range original.Splits {
		amount, allocationErr := proportionalMinor(adjustmentAmount, split.Amount.AmountMinor, original.Total.AmountMinor)
		if allocationErr != nil {
			s.releaseIdempotent(ctx, scope, idempotencyKey, hash)
			return nil, platform.ValidationError(platform.FieldError{Path: "amountMinor", Code: "overflow", Message: allocationErr.Error()})
		}
		if idx == len(original.Splits)-1 {
			amount = remainingAllocation
		}
		remainingAllocation -= amount
		reversal.Splits = append(reversal.Splits, ContributionSplit{FundID: split.FundID, Amount: platform.Money{AmountMinor: -amount, Currency: split.Amount.Currency}})
	}
	result := &AdjustmentResult{Reversal: &reversal}
	if input.Type != "reversal" && !providerAdjustment {
		donor := original.Donor
		if input.ReplacementDonor != nil {
			donor = *input.ReplacementDonor
		}
		splits := original.Splits
		if len(input.ReplacementSplits) > 0 {
			splits = input.ReplacementSplits
		}
		if err = normalizeContributionSplits(splits, original.Total.Currency, original.Total.AmountMinor, false); err != nil {
			s.releaseIdempotent(ctx, scope, idempotencyKey, hash)
			return nil, platform.ValidationError(platform.FieldError{Path: "replacementSplits", Code: "unbalanced", Message: err.Error()})
		}
		exists, e := s.Repository.DonorExists(ctx, p.OrganizationID, donor)
		if e != nil || !exists {
			s.releaseIdempotent(ctx, scope, idempotencyKey, hash)
			if e != nil {
				return nil, e
			}
			return nil, platform.ValidationError(platform.FieldError{Path: "replacementDonor", Code: "not_found", Message: "Replacement donor does not exist."})
		}
		for _, split := range splits {
			fund, e := s.Repository.FindActiveFund(ctx, p.OrganizationID, split.FundID, now)
			if e != nil || fund == nil {
				s.releaseIdempotent(ctx, scope, idempotencyKey, hash)
				if e != nil {
					return nil, e
				}
				return nil, platform.ValidationError(platform.FieldError{Path: "replacementSplits", Code: "inactive_fund", Message: "Replacement fund is not active."})
			}
		}
		replacement := Contribution{ResourceEnvelope: envelope(p, original.BranchID, now), ReceivedAt: now, PostedAt: now, Donor: donor, Source: "adjustment", PaymentMethodID: original.PaymentMethodID, CampaignID: original.CampaignID, PledgeID: original.PledgeID, FiscalPeriodID: period.ID, Total: original.Total, Splits: splits, State: "posted", EffectiveState: "adjustment", Provenance: "adjustment", CommandKey: idempotencyKey + ":replacement", CommandHash: hash, Link: &ContributionLink{Type: input.Type, ChainRootID: root, ReplacesContributionID: original.ID, Reason: input.Reason}}
		result.Replacement = &replacement
	}
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		currentOriginal, e := s.Repository.FindContribution(tx, p.OrganizationID, original.ID)
		if e != nil {
			return e
		}
		if currentOriginal == nil || currentOriginal.Version != original.Version {
			return platform.VersionConflict(original.Version)
		}
		alreadyReversed, e := s.Repository.FindReversal(tx, p.OrganizationID, original.ID)
		if e != nil {
			return e
		}
		if alreadyReversed != nil && !providerAdjustment {
			return &platform.DomainError{Code: "immutable", Message: "This contribution already has a reversal or correction."}
		}
		if providerAdjustment {
			alreadyAdjusted, e := s.Repository.SumProviderAdjustments(tx, p.OrganizationID, original.ID)
			if e != nil {
				return e
			}
			if input.AmountMinor > original.Total.AmountMinor+alreadyAdjusted {
				return platform.ValidationError(platform.FieldError{Path: "amountMinor", Code: "exceeds_remaining", Message: "Provider adjustment exceeds the remaining contribution amount."})
			}
		}
		currentPeriod, e := s.Repository.FindPeriodForDate(tx, p.OrganizationID, now)
		if e != nil {
			return e
		}
		if currentPeriod == nil {
			return platform.ValidationError(platform.FieldError{Path: "$", Code: "no_open_period", Message: "An open current fiscal period is required for adjustments."})
		}
		currentSequence, e := s.Repository.FindSequenceForYear(tx, p.OrganizationID, now.Year())
		if e != nil {
			return e
		}
		if currentSequence == nil {
			return platform.ValidationError(platform.FieldError{Path: "$", Code: "no_receipt_sequence", Message: "A current receipt sequence is required for adjustments."})
		}
		if result.Replacement != nil {
			exists, e := s.Repository.DonorExists(tx, p.OrganizationID, result.Replacement.Donor)
			if e != nil {
				return e
			}
			if !exists {
				return platform.ValidationError(platform.FieldError{Path: "replacementDonor", Code: "not_found", Message: "Replacement donor does not exist."})
			}
			for _, split := range result.Replacement.Splits {
				fund, e := s.Repository.FindActiveFund(tx, p.OrganizationID, split.FundID, now)
				if e != nil {
					return e
				}
				if fund == nil {
					return platform.ValidationError(platform.FieldError{Path: "replacementSplits", Code: "inactive_fund", Message: "Replacement fund is not active."})
				}
			}
		}
		values := []*Contribution{result.Reversal}
		if result.Replacement != nil {
			values = append(values, result.Replacement)
		}
		for _, value := range values {
			value.FiscalPeriodID = currentPeriod.ID
			receipt, e := s.Repository.ReserveReceipt(tx, p.OrganizationID, currentSequence.ID, now, p.Actor)
			if e != nil {
				return e
			}
			value.ReceiptNumber = receipt
			if e = s.Repository.InsertReceiptAllocation(tx, ReceiptAllocation{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, SequenceID: currentSequence.ID, ReceiptNumber: receipt, ContributionID: value.ID, State: "posted", AllocatedAt: now}); e != nil {
				return fmt.Errorf("insert adjustment receipt allocation: %w", e)
			}
			if e = s.Repository.InsertContribution(tx, *value); e != nil {
				return fmt.Errorf("insert adjustment contribution: %w", e)
			}
		}
		if e := s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: original.BranchID, Actor: p.Actor, Action: "finance.contribution." + input.Type, ResourceType: "contribution", ResourceID: original.ID, SubjectIDs: donorSubjectIDs(original.Donor), ChangedFields: []string{"adjustmentLink", "reversal", "replacement"}, Outcome: "success", Reason: input.Reason, RequestID: requestID, OccurredAt: now}); e != nil {
			return e
		}
		if e := s.Platform.EnqueueEvent(tx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: original.BranchID, Type: "finance.contribution.adjusted", EventVersion: 1, AggregateType: "contribution", AggregateID: original.ID, AggregateVersion: original.Version, Actor: p.Actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: bson.M{"originalId": original.ID, "reversalId": result.Reversal.ID}}, State: "pending", AvailableAt: now}); e != nil {
			return e
		}
		body, _ := json.Marshal(result)
		return s.finishIdempotent(tx, scope, idempotencyKey, hash, 201, body, result.Reversal.ID)
	})
	if err != nil {
		s.releaseIdempotent(ctx, scope, idempotencyKey, hash)
		return nil, err
	}
	return result, nil
}

func proportionalMinor(amount, part, total int64) (int64, error) {
	if amount < 0 || part < 0 || total <= 0 {
		return 0, fmt.Errorf("provider adjustment allocation is invalid")
	}
	value := new(big.Int).Mul(big.NewInt(amount), big.NewInt(part))
	value.Quo(value, big.NewInt(total))
	if !value.IsInt64() {
		return 0, fmt.Errorf("provider adjustment allocation exceeds supported money range")
	}
	return value.Int64(), nil
}

func (s Service) CorrectAttribution(ctx context.Context, p platform.Principal, originalID platform.ID, donor DonorAttribution, reason, requestID, idempotencyKey string) (*AdjustmentResult, error) {
	return s.AdjustContribution(ctx, p, originalID, AdjustmentInput{Type: "attribution-correction", Reason: strings.TrimSpace(reason), ReplacementDonor: &donor}, requestID, idempotencyKey)
}
