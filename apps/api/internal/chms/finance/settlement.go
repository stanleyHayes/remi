package finance

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"remi-api/internal/chms/platform"
)

const (
	settlementsCollection           = "chms_finance_settlements"
	reconciliationItemsCollection   = "chms_finance_reconciliation_items"
	periodControlRequestsCollection = "chms_finance_period_control_requests"
	periodControlEventsCollection   = "chms_finance_period_control_events"
)

type Settlement struct {
	platform.ResourceEnvelope `bson:",inline"`
	SourceType                string      `json:"sourceType" bson:"sourceType"`
	SourceName                string      `json:"sourceName" bson:"sourceName"`
	FileName                  string      `json:"fileName" bson:"fileName"`
	FileHash                  string      `json:"fileHash" bson:"fileHash"`
	PrivateAssetID            string      `json:"privateAssetId,omitempty" bson:"privateAssetId,omitempty"`
	PeriodID                  platform.ID `json:"periodId" bson:"periodId"`
	Reference                 string      `json:"reference" bson:"reference"`
	SettledAt                 time.Time   `json:"settledAt" bson:"settledAt"`
	Currency                  string      `json:"currency" bson:"currency"`
	GrossAmountMinor          int64       `json:"grossAmountMinor" bson:"grossAmountMinor"`
	FeeAmountMinor            int64       `json:"feeAmountMinor" bson:"feeAmountMinor"`
	OtherDeductionMinor       int64       `json:"otherDeductionMinor" bson:"otherDeductionMinor"`
	NetAmountMinor            int64       `json:"netAmountMinor" bson:"netAmountMinor"`
	MatchedGrossMinor         int64       `json:"matchedGrossMinor" bson:"-"`
	ApprovedVarianceMinor     int64       `json:"approvedVarianceMinor" bson:"-"`
	UnexplainedVarianceMinor  int64       `json:"unexplainedVarianceMinor" bson:"-"`
	ItemCount                 int64       `json:"itemCount" bson:"-"`
	UnresolvedCount           int64       `json:"unresolvedCount" bson:"-"`
	State                     string      `json:"state" bson:"state"`
	ReconciliationVersion     int64       `json:"reconciliationVersion" bson:"reconciliationVersion"`
	ImportReady               bool        `json:"-" bson:"importReady"`
}

type SettlementRowInput struct {
	SourceRowID    string    `json:"sourceRowId"`
	Reference      string    `json:"reference"`
	OccurredAt     time.Time `json:"occurredAt"`
	AmountMinor    int64     `json:"amountMinor"`
	FeeAmountMinor int64     `json:"feeAmountMinor"`
	Description    string    `json:"description"`
}
type SettlementInput struct {
	BranchID            platform.ID          `json:"branchId"`
	SourceType          string               `json:"sourceType"`
	SourceName          string               `json:"sourceName"`
	FileName            string               `json:"fileName"`
	FileHash            string               `json:"fileHash"`
	PrivateAssetID      string               `json:"privateAssetId"`
	PeriodID            platform.ID          `json:"periodId"`
	Reference           string               `json:"reference"`
	SettledAt           time.Time            `json:"settledAt"`
	Currency            string               `json:"currency"`
	GrossAmountMinor    int64                `json:"grossAmountMinor"`
	FeeAmountMinor      int64                `json:"feeAmountMinor"`
	OtherDeductionMinor int64                `json:"otherDeductionMinor"`
	NetAmountMinor      int64                `json:"netAmountMinor"`
	Rows                []SettlementRowInput `json:"rows"`
}

type ReconciliationItem struct {
	ID                   platform.ID `json:"id" bson:"_id"`
	OrganizationID       platform.ID `json:"organizationId" bson:"organizationId"`
	BranchID             platform.ID `json:"branchId" bson:"branchId"`
	SettlementID         platform.ID `json:"settlementId" bson:"settlementId"`
	SourceRowID          string      `json:"sourceRowId" bson:"sourceRowId"`
	Reference            string      `json:"reference" bson:"reference"`
	OccurredAt           time.Time   `json:"occurredAt" bson:"occurredAt"`
	AmountMinor          int64       `json:"amountMinor" bson:"amountMinor"`
	FeeAmountMinor       int64       `json:"feeAmountMinor" bson:"feeAmountMinor"`
	Description          string      `json:"description,omitempty" bson:"description,omitempty"`
	SuggestedTargetType  string      `json:"suggestedTargetType,omitempty" bson:"suggestedTargetType,omitempty"`
	SuggestedTargetID    platform.ID `json:"suggestedTargetId,omitempty" bson:"suggestedTargetId,omitempty"`
	SuggestedAmountMinor int64       `json:"suggestedAmountMinor,omitempty" bson:"suggestedAmountMinor,omitempty"`
	Confidence           string      `json:"confidence" bson:"confidence"`
	Evidence             []string    `json:"evidence" bson:"evidence"`
	Resolution           string      `json:"resolution" bson:"resolution"`
	MatchedTargetType    string      `json:"matchedTargetType,omitempty" bson:"matchedTargetType,omitempty"`
	MatchedTargetID      platform.ID `json:"matchedTargetId,omitempty" bson:"matchedTargetId,omitempty"`
	VarianceMinor        int64       `json:"varianceMinor" bson:"varianceMinor"`
	Reason               string      `json:"reason,omitempty" bson:"reason,omitempty"`
	OwnerID              platform.ID `json:"ownerId,omitempty" bson:"ownerId,omitempty"`
	ResolvedBy           platform.ID `json:"resolvedBy,omitempty" bson:"resolvedBy,omitempty"`
	ResolvedAt           *time.Time  `json:"resolvedAt,omitempty" bson:"resolvedAt,omitempty"`
	CreatedAt            time.Time   `json:"createdAt" bson:"createdAt"`
}

type ReconciliationResolutionInput struct {
	Action     string      `json:"action"`
	TargetType string      `json:"targetType"`
	TargetID   platform.ID `json:"targetId"`
	Reason     string      `json:"reason"`
	OwnerID    platform.ID `json:"ownerId"`
}

type ReconciliationOwner struct {
	ID    platform.ID `json:"id"`
	Name  string      `json:"name"`
	Email string      `json:"email"`
	Role  string      `json:"role"`
}

type PeriodControlRequest struct {
	ID             platform.ID `json:"id" bson:"_id"`
	OrganizationID platform.ID `json:"organizationId" bson:"organizationId"`
	PeriodID       platform.ID `json:"periodId" bson:"periodId"`
	Action         string      `json:"action" bson:"action"`
	State          string      `json:"state" bson:"state"`
	Reason         string      `json:"reason" bson:"reason"`
	RequestedBy    platform.ID `json:"requestedBy" bson:"requestedBy"`
	RequestedAt    time.Time   `json:"requestedAt" bson:"requestedAt"`
	ApprovedBy     platform.ID `json:"approvedBy,omitempty" bson:"approvedBy,omitempty"`
	ApprovedAt     *time.Time  `json:"approvedAt,omitempty" bson:"approvedAt,omitempty"`
	Snapshot       bson.M      `json:"snapshot" bson:"snapshot"`
}
type PeriodControlInput struct {
	Action          string      `json:"action"`
	RequestID       platform.ID `json:"requestId"`
	Reason          string      `json:"reason"`
	ExpectedVersion int64       `json:"expectedVersion"`
}
type PeriodControlResult struct {
	Period   *FiscalPeriod         `json:"period"`
	Request  *PeriodControlRequest `json:"request"`
	Snapshot bson.M                `json:"snapshot"`
}

func (i *SettlementInput) NormalizeAndValidate() error {
	i.SourceType = strings.ToLower(strings.TrimSpace(i.SourceType))
	i.SourceName = strings.TrimSpace(i.SourceName)
	i.FileName = strings.TrimSpace(i.FileName)
	i.FileHash = strings.ToLower(strings.TrimSpace(i.FileHash))
	i.PrivateAssetID = strings.TrimSpace(i.PrivateAssetID)
	i.Reference = strings.TrimSpace(i.Reference)
	i.Currency = strings.ToUpper(strings.TrimSpace(i.Currency))
	if !i.BranchID.Valid() || !i.PeriodID.Valid() || !map[string]bool{"paystack": true, "bank": true, "deposit": true}[i.SourceType] || len(i.SourceName) < 2 || len(i.Reference) < 2 || i.SettledAt.IsZero() {
		return fmt.Errorf("branch, period, supported source, name, reference and settlement date are required")
	}
	if len(i.FileHash) != 64 || len(i.FileName) < 3 || len(i.FileName) > 160 || len(i.PrivateAssetID) < 3 || len(i.PrivateAssetID) > 240 {
		return fmt.Errorf("a SHA-256 file hash, private asset reference and safe file name are required")
	}
	if i.Currency != "GHS" || i.GrossAmountMinor <= 0 || i.FeeAmountMinor < 0 || i.OtherDeductionMinor < 0 || i.FeeAmountMinor > i.GrossAmountMinor || i.OtherDeductionMinor > i.GrossAmountMinor-i.FeeAmountMinor || i.NetAmountMinor != i.GrossAmountMinor-i.FeeAmountMinor-i.OtherDeductionMinor {
		return fmt.Errorf("settlement money must be balanced positive GHS minor units")
	}
	if len(i.Rows) < 1 || len(i.Rows) > 5000 {
		return fmt.Errorf("one to 5000 settlement rows are required")
	}
	seen := map[string]bool{}
	var total, fees int64
	for index := range i.Rows {
		row := &i.Rows[index]
		row.SourceRowID = strings.TrimSpace(row.SourceRowID)
		row.Reference = strings.TrimSpace(row.Reference)
		row.Description = strings.TrimSpace(row.Description)
		if row.SourceRowID == "" || seen[row.SourceRowID] || row.OccurredAt.IsZero() || row.AmountMinor <= 0 || row.FeeAmountMinor < 0 || len(row.Reference) > 160 || len(row.Description) > 500 {
			return fmt.Errorf("rows require unique source IDs, dates and positive amounts")
		}
		if row.AmountMinor > math.MaxInt64-total || row.FeeAmountMinor > math.MaxInt64-fees {
			return fmt.Errorf("row totals exceed supported GHS minor units")
		}
		seen[row.SourceRowID] = true
		total += row.AmountMinor
		fees += row.FeeAmountMinor
	}
	if total != i.GrossAmountMinor || fees != i.FeeAmountMinor {
		return fmt.Errorf("row amounts and fees must equal settlement totals")
	}
	return nil
}

func (s Service) ImportSettlement(ctx context.Context, p platform.Principal, input SettlementInput, requestID string) (*Settlement, error) {
	// Normalize an owned copy so concurrent idempotent callers cannot race on
	// the request's shared slice backing array.
	input.Rows = append([]SettlementRowInput(nil), input.Rows...)
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_settlement", Message: err.Error()})
	}
	if !s.allowed(p, "create", input.BranchID) {
		return nil, settlementDenied()
	}
	period, err := s.Repository.FindFiscalPeriod(ctx, p.OrganizationID, input.PeriodID)
	if err != nil || period == nil {
		return nil, settlementDenied()
	}
	if period.Status != "open" {
		return nil, platform.ValidationError(platform.FieldError{Path: "periodId", Code: "period_closed", Message: "Reopen the fiscal period through the controlled approval workflow before importing settlement evidence."})
	}
	if input.SettledAt.Before(period.StartsAt) || !input.SettledAt.Before(period.EndsAt) {
		return nil, platform.ValidationError(platform.FieldError{Path: "settledAt", Code: "outside_period", Message: "Settlement date must fall inside its fiscal period."})
	}
	if existing, e := s.Repository.FindSettlementByHash(ctx, p.OrganizationID, input.SourceType, input.FileHash); e != nil {
		return nil, e
	} else if existing != nil {
		return s.Repository.GetSettlement(ctx, p.OrganizationID, existing.ID)
	}
	now := s.now()
	value := Settlement{ResourceEnvelope: envelope(p, input.BranchID, now), SourceType: input.SourceType, SourceName: input.SourceName, FileName: input.FileName, FileHash: input.FileHash, PrivateAssetID: input.PrivateAssetID, PeriodID: input.PeriodID, Reference: input.Reference, SettledAt: input.SettledAt.UTC(), Currency: input.Currency, GrossAmountMinor: input.GrossAmountMinor, FeeAmountMinor: input.FeeAmountMinor, OtherDeductionMinor: input.OtherDeductionMinor, NetAmountMinor: input.NetAmountMinor, State: "imported", ReconciliationVersion: 1, ImportReady: false}
	items := make([]ReconciliationItem, 0, len(input.Rows))
	for _, row := range input.Rows {
		item := ReconciliationItem{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: input.BranchID, SettlementID: value.ID, SourceRowID: row.SourceRowID, Reference: row.Reference, OccurredAt: row.OccurredAt.UTC(), AmountMinor: row.AmountMinor, FeeAmountMinor: row.FeeAmountMinor, Description: row.Description, Confidence: "none", Resolution: "unresolved", CreatedAt: now}
		if e := s.suggestSettlementMatch(ctx, p.OrganizationID, input.SourceType, &item); e != nil {
			return nil, e
		}
		items = append(items, item)
	}
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if e := s.Repository.Insert(tx, settlementsCollection, value); e != nil {
			return e
		}
		if e := s.Repository.InsertReconciliationItems(tx, items); e != nil {
			return e
		}
		if e := s.audit(tx, p, input.BranchID, "finance.settlement.import", "settlement", value.ID, []string{"source", "fileHash", "period", "totals", "rows"}, requestID, now); e != nil {
			return e
		}
		if e := s.Platform.EnqueueEvent(tx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: input.BranchID, Type: "finance.settlement.imported", EventVersion: 1, AggregateType: "settlement", AggregateID: value.ID, AggregateVersion: 1, Actor: p.Actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: bson.M{"rowCount": len(items), "periodId": input.PeriodID}}, State: "pending", AvailableAt: now}); e != nil {
			return e
		}
		_, e := s.Repository.database.Collection(settlementsCollection).UpdateOne(tx, bson.M{"_id": value.ID, "organizationId": p.OrganizationID}, bson.M{"$set": bson.M{"importReady": true}})
		return e
	})
	if err != nil {
		// The unique source/hash index is the final concurrency guard. A racing
		// replay should receive the original decorated settlement, not a storage
		// error, after the winning transaction commits.
		var domainErr *platform.DomainError
		if mongo.IsDuplicateKeyError(err) || (errors.As(err, &domainErr) && domainErr.Code == "conflict") {
			for attempt := 0; attempt < 25; attempt++ {
				existing, replayErr := s.Repository.FindSettlementByHash(ctx, p.OrganizationID, input.SourceType, input.FileHash)
				if replayErr != nil {
					return nil, replayErr
				}
				if existing != nil {
					return s.Repository.GetSettlement(ctx, p.OrganizationID, existing.ID)
				}
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(10 * time.Millisecond):
				}
			}
		}
		return nil, err
	}
	return s.Repository.GetSettlement(ctx, p.OrganizationID, value.ID)
}

func (s Service) suggestSettlementMatch(ctx context.Context, org platform.ID, source string, item *ReconciliationItem) error {
	if source == "paystack" {
		contribution, err := s.Repository.FindContributionByProviderReference(ctx, org, item.BranchID, item.Reference, item.OccurredAt)
		if err != nil {
			return err
		}
		if contribution != nil {
			effective, effectiveErr := s.Repository.EffectiveContributionAmount(ctx, org, contribution.ID)
			if effectiveErr != nil {
				return effectiveErr
			}
			item.SuggestedTargetType, item.SuggestedTargetID, item.SuggestedAmountMinor = "contribution", contribution.ID, contribution.Total.AmountMinor
			item.SuggestedAmountMinor = effective
			item.Evidence = []string{"provider-reference"}
			item.Confidence = "reference"
			if effective == item.AmountMinor {
				item.Confidence = "exact"
				item.Evidence = append(item.Evidence, "amount", "currency")
			}
		}
	}
	if source == "deposit" {
		batch, err := s.Repository.FindBatchByIDAndTotal(ctx, org, item.Reference, item.AmountMinor)
		if err != nil {
			return err
		}
		if batch != nil {
			item.SuggestedTargetType, item.SuggestedTargetID, item.SuggestedAmountMinor = "batch", batch.ID, batch.EnteredTotal.AmountMinor
			item.Confidence = "exact"
			item.Evidence = []string{"deposit-reference", "amount", "currency"}
		}
	}
	return nil
}

func (s Service) ListSettlements(ctx context.Context, p platform.Principal) ([]Settlement, error) {
	if !s.allowed(p, "read", "") {
		return nil, settlementDenied()
	}
	return s.Repository.ListSettlements(ctx, p.OrganizationID)
}
func (s Service) GetSettlement(ctx context.Context, p platform.Principal, id platform.ID) (*Settlement, error) {
	value, err := s.Repository.GetSettlement(ctx, p.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if value == nil || !s.allowed(p, "read", value.BranchID) {
		return nil, settlementDenied()
	}
	return value, nil
}
func (s Service) ListReconciliationItems(ctx context.Context, p platform.Principal, settlementID platform.ID) ([]ReconciliationItem, error) {
	settlement, err := s.GetSettlement(ctx, p, settlementID)
	if err != nil {
		return nil, err
	}
	return s.Repository.ListReconciliationItems(ctx, p.OrganizationID, settlement.ID)
}

func (s Service) ListReconciliationOwners(ctx context.Context, p platform.Principal) ([]ReconciliationOwner, error) {
	if !s.allowed(p, "read", "") {
		return nil, settlementDenied()
	}
	return s.Repository.ListActiveFinanceStaff(ctx)
}

func (s Service) ResolveReconciliationItem(ctx context.Context, p platform.Principal, id platform.ID, input ReconciliationResolutionInput, requestID string) (*ReconciliationItem, error) {
	input.Action = strings.ToLower(strings.TrimSpace(input.Action))
	input.TargetType = strings.ToLower(strings.TrimSpace(input.TargetType))
	input.Reason = strings.TrimSpace(input.Reason)
	if !map[string]bool{"match": true, "approve-variance": true, "assign-exception": true}[input.Action] {
		return nil, invalid(fmt.Errorf("unsupported resolution action"))
	}
	item, err := s.Repository.FindReconciliationItem(ctx, p.OrganizationID, id)
	if err != nil || item == nil || item.Resolution != "unresolved" || !s.allowed(p, "update", item.BranchID) {
		return nil, settlementDenied()
	}
	if input.Action == "approve-variance" && !s.allowedLedger(p, "approve", item.BranchID, true) {
		return nil, settlementDenied()
	}
	now := s.now()
	set := bson.M{"resolution": input.Action, "resolvedBy": p.Actor.ID, "resolvedAt": now}
	changed := []string{"resolution"}
	if input.Action == "assign-exception" {
		if !input.OwnerID.Valid() || len(input.Reason) < 10 {
			return nil, invalid(fmt.Errorf("owned exception requires a detailed reason"))
		}
		ownerExists, e := s.Repository.StaffHasRole(ctx, input.OwnerID, "super-admin", "editor", "viewer", "finance-counter", "finance-admin", "finance-approver", "finance-auditor")
		if e != nil {
			return nil, e
		}
		if !ownerExists {
			return nil, platform.ValidationError(platform.FieldError{Path: "ownerId", Code: "invalid_owner", Message: "Exception owner must be an active staff user."})
		}
		set["ownerId"], set["reason"] = input.OwnerID, input.Reason
		changed = append(changed, "owner", "reason")
	} else {
		if !map[string]bool{"contribution": true, "batch": true}[input.TargetType] || !input.TargetID.Valid() {
			return nil, invalid(fmt.Errorf("match target is required"))
		}
		eligible, e := s.Repository.ReconciliationTargetEligible(ctx, p.OrganizationID, item.SettlementID, input.TargetType, input.TargetID)
		if e != nil {
			return nil, e
		}
		if !eligible {
			return nil, platform.ValidationError(platform.FieldError{Path: "targetId", Code: "outside_settlement_scope", Message: "Match target must belong to the settlement branch and fiscal period."})
		}
		targetAmount, e := s.Repository.ReconciliationTargetAmount(ctx, p.OrganizationID, input.TargetType, input.TargetID)
		if e != nil {
			return nil, e
		}
		if targetAmount == nil {
			return nil, settlementDenied()
		}
		variance := item.AmountMinor - *targetAmount
		if variance != 0 && (input.Action != "approve-variance" || len(input.Reason) < 10) {
			return nil, platform.ValidationError(platform.FieldError{Path: "reason", Code: "variance_reason_required", Message: "A detailed reason is required to approve a non-zero variance."})
		}
		if variance == 0 && input.Action == "approve-variance" {
			return nil, invalid(fmt.Errorf("exact matches do not require variance approval"))
		}
		set["matchedTargetType"], set["matchedTargetId"], set["varianceMinor"], set["reason"] = input.TargetType, input.TargetID, variance, input.Reason
		changed = append(changed, "match", "variance", "reason")
	}
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if e := s.Repository.ResolveReconciliationItem(tx, p.OrganizationID, id, set); e != nil {
			return e
		}
		if input.Action != "assign-exception" {
			if e := s.Repository.MarkReconciliationTarget(tx, p.OrganizationID, input.TargetType, input.TargetID, now, p.Actor); e != nil {
				return e
			}
		}
		if e := s.Repository.RefreshSettlementState(tx, p.OrganizationID, item.SettlementID, now, p.Actor); e != nil {
			return e
		}
		return s.audit(tx, p, item.BranchID, "finance.reconciliation."+input.Action, "reconciliation-item", id, changed, requestID, now)
	})
	if err != nil {
		return nil, err
	}
	return s.Repository.FindReconciliationItem(ctx, p.OrganizationID, id)
}

func settlementDenied() error {
	return &platform.DomainError{Code: "not_found", Message: "Settlement or reconciliation item not found."}
}

func (r *Repository) FindSettlementByHash(ctx context.Context, org platform.ID, source, hash string) (*Settlement, error) {
	var value Settlement
	err := r.database.Collection(settlementsCollection).FindOne(ctx, bson.M{"organizationId": org, "sourceType": source, "fileHash": hash, "$or": bson.A{bson.M{"importReady": true}, bson.M{"importReady": bson.M{"$exists": false}}}}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &value, nil
}
func (r *Repository) GetSettlement(ctx context.Context, org, id platform.ID) (*Settlement, error) {
	value, err := findOne[Settlement](ctx, r, settlementsCollection, org, id)
	if err != nil || value == nil {
		return value, err
	}
	if err = r.decorateSettlement(ctx, value); err != nil {
		return nil, err
	}
	return value, nil
}
func (r *Repository) ListSettlements(ctx context.Context, org platform.ID) ([]Settlement, error) {
	cursor, err := r.database.Collection(settlementsCollection).Find(ctx, bson.M{"organizationId": org, "archivedAt": nil}, options.Find().SetSort(bson.D{{Key: "settledAt", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	values := []Settlement{}
	if err = cursor.All(ctx, &values); err != nil {
		return nil, err
	}
	for index := range values {
		if err = r.decorateSettlement(ctx, &values[index]); err != nil {
			return nil, err
		}
	}
	return values, nil
}
func (r *Repository) decorateSettlement(ctx context.Context, value *Settlement) error {
	items, err := r.ListReconciliationItems(ctx, value.OrganizationID, value.ID)
	if err != nil {
		return err
	}
	value.ItemCount = int64(len(items))
	for _, item := range items {
		if item.Resolution == "unresolved" {
			value.UnresolvedCount++
		}
		if item.Resolution == "match" || item.Resolution == "approve-variance" {
			value.MatchedGrossMinor += item.AmountMinor - item.VarianceMinor
			value.ApprovedVarianceMinor += item.VarianceMinor
		}
	}
	value.UnexplainedVarianceMinor = value.GrossAmountMinor - value.MatchedGrossMinor - value.ApprovedVarianceMinor
	return nil
}
func (r *Repository) InsertReconciliationItems(ctx context.Context, items []ReconciliationItem) error {
	values := make([]any, len(items))
	for index := range items {
		values[index] = items[index]
	}
	_, err := r.database.Collection(reconciliationItemsCollection).InsertMany(ctx, values)
	return err
}
func (r *Repository) ListReconciliationItems(ctx context.Context, org, settlement platform.ID) ([]ReconciliationItem, error) {
	cursor, err := r.database.Collection(reconciliationItemsCollection).Find(ctx, bson.M{"organizationId": org, "settlementId": settlement}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	values := []ReconciliationItem{}
	err = cursor.All(ctx, &values)
	return values, err
}
func (r *Repository) FindReconciliationItem(ctx context.Context, org, id platform.ID) (*ReconciliationItem, error) {
	var value ReconciliationItem
	err := r.database.Collection(reconciliationItemsCollection).FindOne(ctx, bson.M{"_id": id, "organizationId": org}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &value, err
}
func (r *Repository) ResolveReconciliationItem(ctx context.Context, org, id platform.ID, set bson.M) error {
	result, err := r.database.Collection(reconciliationItemsCollection).UpdateOne(ctx, bson.M{"_id": id, "organizationId": org, "resolution": "unresolved"}, bson.M{"$set": set})
	if err != nil {
		return err
	}
	if result.ModifiedCount != 1 {
		return platform.VersionConflict(1)
	}
	return nil
}
func (r *Repository) FindContributionByProviderReference(ctx context.Context, org, branch platform.ID, reference string, occurredAt time.Time) (*Contribution, error) {
	var value Contribution
	err := r.database.Collection(contributionsCollection).FindOne(ctx, bson.M{"organizationId": org, "branchId": branch, "providerReference": reference, "state": "posted", "receivedAt": bson.M{"$gte": occurredAt.AddDate(0, 0, -31), "$lt": occurredAt.AddDate(0, 0, 31)}}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &value, err
}
func (r *Repository) FindBatchByIDAndTotal(ctx context.Context, org platform.ID, reference string, total int64) (*CountingBatch, error) {
	var value CountingBatch
	err := r.database.Collection(countingBatchesCollection).FindOne(ctx, bson.M{"_id": platform.ID(reference), "organizationId": org, "state": bson.M{"$in": bson.A{"posted", "deposited"}}, "enteredTotal.amountMinor": total}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &value, err
}
func (r *Repository) ReconciliationTargetAmount(ctx context.Context, org platform.ID, targetType string, id platform.ID) (*int64, error) {
	if targetType == "contribution" {
		value, err := r.EffectiveContributionAmount(ctx, org, id)
		if err != nil {
			return nil, err
		}
		if value == 0 {
			if exists, findErr := r.FindContribution(ctx, org, id); findErr != nil || exists == nil {
				return nil, findErr
			}
		}
		return &value, nil
	}
	var row struct {
		Total        platform.Money `bson:"total"`
		EnteredTotal platform.Money `bson:"enteredTotal"`
	}
	collection := contributionsCollection
	if targetType == "batch" {
		collection = countingBatchesCollection
	}
	err := r.database.Collection(collection).FindOne(ctx, bson.M{"_id": id, "organizationId": org}).Decode(&row)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	value := row.Total.AmountMinor
	if targetType == "batch" {
		value = row.EnteredTotal.AmountMinor
	}
	return &value, nil
}

func (r *Repository) ReconciliationTargetEligible(ctx context.Context, org, settlementID platform.ID, targetType string, id platform.ID) (bool, error) {
	settlement, err := r.GetSettlement(ctx, org, settlementID)
	if err != nil || settlement == nil {
		return false, err
	}
	period, err := r.FindFiscalPeriod(ctx, org, settlement.PeriodID)
	if err != nil || period == nil {
		return false, err
	}
	filter := bson.M{"_id": id, "organizationId": org, "branchId": settlement.BranchID}
	collection := contributionsCollection
	if targetType == "contribution" {
		filter["receivedAt"] = bson.M{"$gte": period.StartsAt, "$lt": period.EndsAt}
		filter["state"] = "posted"
	} else {
		collection = countingBatchesCollection
		filter["receivedAt"] = bson.M{"$gte": period.StartsAt, "$lt": period.EndsAt}
		filter["state"] = bson.M{"$in": bson.A{"posted", "deposited"}}
	}
	count, err := r.database.Collection(collection).CountDocuments(ctx, filter)
	return count == 1, err
}

func (r *Repository) EffectiveContributionAmount(ctx context.Context, org, id platform.ID) (int64, error) {
	value, err := r.FindContribution(ctx, org, id)
	if err != nil || value == nil {
		return 0, err
	}
	adjustments, err := r.SumProviderAdjustments(ctx, org, id)
	if err != nil {
		return 0, err
	}
	return value.Total.AmountMinor + adjustments, nil
}
func (r *Repository) RefreshSettlementState(ctx context.Context, org, settlement platform.ID, now time.Time, actor platform.Actor) error {
	unresolved, err := r.database.Collection(reconciliationItemsCollection).CountDocuments(ctx, bson.M{"organizationId": org, "settlementId": settlement, "resolution": "unresolved"})
	if err != nil {
		return err
	}
	exceptions, err := r.database.Collection(reconciliationItemsCollection).CountDocuments(ctx, bson.M{"organizationId": org, "settlementId": settlement, "resolution": "assign-exception"})
	if err != nil {
		return err
	}
	state := "review"
	if unresolved == 0 && exceptions == 0 {
		state = "reconciled"
	}
	_, err = r.database.Collection(settlementsCollection).UpdateOne(ctx, bson.M{"_id": settlement, "organizationId": org}, bson.M{"$set": bson.M{"state": state, "updatedAt": now, "updatedBy": actor}, "$inc": bson.M{"version": 1, "reconciliationVersion": 1}})
	return err
}

func (r *Repository) MarkReconciliationTarget(ctx context.Context, org platform.ID, targetType string, id platform.ID, now time.Time, actor platform.Actor) error {
	if targetType != "batch" {
		return nil
	}
	result, err := r.database.Collection(countingBatchesCollection).UpdateOne(ctx, bson.M{"_id": id, "organizationId": org, "state": bson.M{"$in": bson.A{"posted", "deposited"}}}, bson.M{"$set": bson.M{"state": "reconciled", "updatedAt": now, "updatedBy": actor}, "$inc": bson.M{"version": 1}})
	if err != nil {
		return err
	}
	if result.ModifiedCount != 1 {
		return settlementDenied()
	}
	return nil
}

func (s Service) ControlPeriod(ctx context.Context, p platform.Principal, periodID platform.ID, control string, input PeriodControlInput, requestID string) (*PeriodControlResult, error) {
	control = strings.ToLower(strings.TrimSpace(control))
	input.Action = strings.ToLower(strings.TrimSpace(input.Action))
	input.Reason = strings.TrimSpace(input.Reason)
	if !map[string]bool{"close": true, "reopen": true}[control] || !map[string]bool{"request": true, "approve": true}[input.Action] || len(input.Reason) < 10 {
		return nil, invalid(fmt.Errorf("control action and detailed reason are required"))
	}
	period, err := s.Repository.FindFiscalPeriod(ctx, p.OrganizationID, periodID)
	if err != nil || period == nil {
		return nil, denied()
	}
	expectedStatus := map[string]string{"close": "open", "reopen": "closed"}[control]
	if period.Status != expectedStatus {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_period_state", Message: "Fiscal period is not eligible for this action."})
	}
	now := s.now()
	if input.Action == "request" {
		if !s.allowed(p, "update", "") {
			return nil, denied()
		}
		snapshot, e := s.Repository.PeriodCloseSnapshot(ctx, p.OrganizationID, period)
		if e != nil {
			return nil, e
		}
		value := PeriodControlRequest{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, PeriodID: period.ID, Action: control, State: "pending", Reason: input.Reason, RequestedBy: p.Actor.ID, RequestedAt: now, Snapshot: snapshot}
		e = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
			if insertErr := s.Repository.Insert(tx, periodControlRequestsCollection, value); insertErr != nil {
				return insertErr
			}
			return s.audit(tx, p, "", "finance.period."+control+".request", "fiscal-period", period.ID, []string{"controlRequest", "snapshot"}, requestID, now)
		})
		if e != nil {
			return nil, e
		}
		return &PeriodControlResult{Period: period, Request: &value, Snapshot: snapshot}, nil
	}
	if !input.RequestID.Valid() || !s.allowedLedger(p, "approve", "", true) {
		return nil, denied()
	}
	pending, e := s.Repository.FindPeriodControlRequest(ctx, p.OrganizationID, input.RequestID)
	if e != nil || pending == nil || pending.PeriodID != period.ID || pending.Action != control || pending.State != "pending" || pending.RequestedBy == p.Actor.ID {
		return nil, denied()
	}
	if err = platform.RequireExpectedVersion(input.ExpectedVersion); err != nil {
		return nil, err
	}
	if period.Version != input.ExpectedVersion {
		return nil, platform.VersionConflict(period.Version)
	}
	snapshot, e := s.Repository.PeriodCloseSnapshot(ctx, p.OrganizationID, period)
	if e != nil {
		return nil, e
	}
	if control == "close" && snapshotInt(snapshot, "blockingCount") > 0 {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "period_not_reconciled", Message: "Resolve every batch, settlement, provider and reconciliation exception before closing the period."})
	}
	newStatus := map[string]string{"close": "closed", "reopen": "open"}[control]
	approvedAt := now
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		current, e := s.Repository.FindFiscalPeriod(tx, p.OrganizationID, period.ID)
		if e != nil {
			return e
		}
		if current == nil || current.Version != period.Version || current.Status != expectedStatus {
			return platform.VersionConflict(period.Version)
		}
		if e = s.Repository.Update(tx, fiscalPeriodsCollection, p.OrganizationID, period.ID, period.Version, bson.M{"status": newStatus, "version": period.Version + 1, "updatedAt": now, "updatedBy": p.Actor}); e != nil {
			return e
		}
		if e = s.Repository.ApprovePeriodControlRequest(tx, p.OrganizationID, pending.ID, p.Actor.ID, approvedAt, snapshot); e != nil {
			return e
		}
		event := bson.M{"_id": platform.ID(bson.NewObjectID().Hex()), "organizationId": p.OrganizationID, "periodId": period.ID, "requestId": pending.ID, "action": control, "fromStatus": expectedStatus, "toStatus": newStatus, "reason": input.Reason, "requestedBy": pending.RequestedBy, "approvedBy": p.Actor.ID, "snapshot": snapshot, "occurredAt": now}
		if e = s.Repository.Insert(tx, periodControlEventsCollection, event); e != nil {
			return e
		}
		if e = s.audit(tx, p, "", "finance.period."+control+".approve", "fiscal-period", period.ID, []string{"status", "controlRequest", "snapshot"}, requestID, now); e != nil {
			return e
		}
		return s.Platform.EnqueueEvent(tx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, Type: "finance.period." + control + "d", EventVersion: 1, AggregateType: "fiscal-period", AggregateID: period.ID, AggregateVersion: period.Version + 1, Actor: p.Actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: bson.M{"controlRequestId": pending.ID, "previousStatus": expectedStatus, "newStatus": newStatus}}, State: "pending", AvailableAt: now})
	})
	if err != nil {
		return nil, err
	}
	updated, e := s.Repository.FindFiscalPeriod(ctx, p.OrganizationID, period.ID)
	if e != nil {
		return nil, e
	}
	pending.State, pending.ApprovedBy, pending.ApprovedAt = "approved", p.Actor.ID, &approvedAt
	return &PeriodControlResult{Period: updated, Request: pending, Snapshot: snapshot}, nil
}

func snapshotInt(value bson.M, key string) int64 {
	switch number := value[key].(type) {
	case int:
		return int64(number)
	case int32:
		return int64(number)
	case int64:
		return number
	case float64:
		return int64(number)
	}
	return 0
}
func (r *Repository) FindPeriodControlRequest(ctx context.Context, org, id platform.ID) (*PeriodControlRequest, error) {
	var value PeriodControlRequest
	err := r.database.Collection(periodControlRequestsCollection).FindOne(ctx, bson.M{"_id": id, "organizationId": org}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &value, err
}
func (s Service) ListPeriodControlRequests(ctx context.Context, p platform.Principal, periodID platform.ID) ([]PeriodControlRequest, error) {
	if !s.allowed(p, "read", "") {
		return nil, denied()
	}
	return s.Repository.ListPeriodControlRequests(ctx, p.OrganizationID, periodID)
}

// ListPeriodControlRequests returns requests for one period, or every period in
// the organization when periodID is empty. The configuration screen needs the
// organization-wide view to badge each period with its pending request.
func (r *Repository) ListPeriodControlRequests(ctx context.Context, org, periodID platform.ID) ([]PeriodControlRequest, error) {
	filter := bson.M{"organizationId": org}
	if periodID.Valid() {
		filter["periodId"] = periodID
	}
	cursor, err := r.database.Collection(periodControlRequestsCollection).Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "requestedAt", Value: -1}}).SetLimit(50))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	values := []PeriodControlRequest{}
	err = cursor.All(ctx, &values)
	return values, err
}
func (r *Repository) ApprovePeriodControlRequest(ctx context.Context, org, id, approver platform.ID, at time.Time, snapshot bson.M) error {
	result, err := r.database.Collection(periodControlRequestsCollection).UpdateOne(ctx, bson.M{"_id": id, "organizationId": org, "state": "pending"}, bson.M{"$set": bson.M{"state": "approved", "approvedBy": approver, "approvedAt": at, "approvalSnapshot": snapshot}})
	if err != nil {
		return err
	}
	if result.ModifiedCount != 1 {
		return platform.VersionConflict(1)
	}
	return nil
}
func (r *Repository) PeriodCloseSnapshot(ctx context.Context, org platform.ID, period *FiscalPeriod) (bson.M, error) {
	count := func(collection string, filter bson.M) (int64, error) {
		filter["organizationId"] = org
		return r.database.Collection(collection).CountDocuments(ctx, filter)
	}
	settlements, err := count(settlementsCollection, bson.M{"periodId": period.ID, "state": bson.M{"$ne": "reconciled"}})
	if err != nil {
		return nil, err
	}
	batches, err := count(countingBatchesCollection, bson.M{"receivedAt": bson.M{"$gte": period.StartsAt, "$lt": period.EndsAt}, "state": bson.M{"$nin": bson.A{"reconciled", "voided"}}})
	if err != nil {
		return nil, err
	}
	intents, err := count(paymentIntentsCollection, bson.M{"createdAt": bson.M{"$gte": period.StartsAt, "$lt": period.EndsAt}, "state": bson.M{"$in": bson.A{"pending", "failed", "chargeback"}}})
	if err != nil {
		return nil, err
	}
	exceptions, err := count(providerExceptionsCollection, bson.M{"createdAt": bson.M{"$gte": period.StartsAt, "$lt": period.EndsAt}, "state": bson.M{"$ne": "resolved"}})
	if err != nil {
		return nil, err
	}
	settlementIDs := []platform.ID{}
	if err = r.database.Collection(settlementsCollection).Distinct(ctx, "_id", bson.M{"organizationId": org, "periodId": period.ID}).Decode(&settlementIDs); err != nil {
		return nil, err
	}
	reconciliation := int64(0)
	unmatchedContributions := int64(0)
	if len(settlementIDs) > 0 {
		reconciliation, err = count(reconciliationItemsCollection, bson.M{"settlementId": bson.M{"$in": settlementIDs}, "resolution": "unresolved"})
		if err != nil {
			return nil, err
		}
		matchedContributionIDs := []platform.ID{}
		if err = r.database.Collection(reconciliationItemsCollection).Distinct(ctx, "matchedTargetId", bson.M{"organizationId": org, "settlementId": bson.M{"$in": settlementIDs}, "matchedTargetType": "contribution", "resolution": bson.M{"$in": bson.A{"match", "approve-variance"}}}).Decode(&matchedContributionIDs); err != nil {
			return nil, err
		}
		filter := bson.M{"receivedAt": bson.M{"$gte": period.StartsAt, "$lt": period.EndsAt}, "source": "online", "state": "posted"}
		if len(matchedContributionIDs) > 0 {
			filter["_id"] = bson.M{"$nin": matchedContributionIDs}
		}
		unmatchedContributions, err = count(contributionsCollection, filter)
		if err != nil {
			return nil, err
		}
	} else {
		unmatchedContributions, err = count(contributionsCollection, bson.M{"receivedAt": bson.M{"$gte": period.StartsAt, "$lt": period.EndsAt}, "source": "online", "state": "posted"})
		if err != nil {
			return nil, err
		}
	}
	return bson.M{"settlementBlockers": settlements, "batchBlockers": batches, "providerIntentBlockers": intents, "providerExceptionBlockers": exceptions, "reconciliationBlockers": reconciliation, "unmatchedContributionBlockers": unmatchedContributions, "blockingCount": settlements + batches + intents + exceptions + reconciliation + unmatchedContributions, "sourceWatermark": time.Now().UTC()}, nil
}
