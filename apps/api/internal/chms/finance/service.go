package finance

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"remi-api/internal/chms/platform"
)

type Platform interface {
	WithTransaction(context.Context, func(context.Context) error) error
	AppendAudit(context.Context, platform.AuditEvent) error
	EnqueueEvent(context.Context, platform.OutboxRecord) error
}
type Service struct {
	Repository *Repository
	Platform   Platform
	Authorizer platform.Authorizer
	Provider   PaymentProvider
	// AllowProviderDemo permits the local no-provider checkout redirect. It
	// must never be enabled by a production bootstrap.
	AllowProviderDemo   bool
	ReminderEligibility ReminderEligibility
	Mailer              StatementMailer
	Cipher              *platform.EnvelopeCipher
	MemberAppURL        string
	Now                 func() time.Time
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func (s Service) allowed(p platform.Principal, action string, branch platform.ID) bool {
	if s.Authorizer == nil {
		return false
	}
	return s.Authorizer.Authorize(p, platform.AccessRequest{Action: action, ResourceType: "finance-config", OrganizationID: p.OrganizationID, BranchID: branch, FieldClasses: []platform.FieldClass{platform.FieldFinancial}, Now: s.now()}).Allowed
}
func denied() error {
	return &platform.DomainError{Code: "not_found", Message: "Finance configuration not found."}
}
func invalid(err error) error {
	return platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_finance_configuration", Message: err.Error()})
}
func envelope(p platform.Principal, branch platform.ID, now time.Time) platform.ResourceEnvelope {
	return platform.ResourceEnvelope{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: branch, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: p.Actor, UpdatedAt: now, UpdatedBy: p.Actor}
}
func (s Service) audit(ctx context.Context, p platform.Principal, branch platform.ID, action, resource string, id platform.ID, fields []string, requestID string, now time.Time) error {
	return s.Platform.AppendAudit(ctx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: branch, Actor: p.Actor, Action: action, ResourceType: resource, ResourceID: id, ChangedFields: fields, Outcome: "success", RequestID: requestID, OccurredAt: now})
}
func (s Service) transact(ctx context.Context, p platform.Principal, branch platform.ID, action, resource, collection string, id platform.ID, expected int64, set bson.M, value any, fields []string, requestID string) error {
	now := s.now()
	return s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if expected == 0 {
			if err := s.Repository.Insert(tx, collection, value); err != nil {
				return err
			}
		} else {
			set["version"] = expected + 1
			set["updatedAt"] = now
			set["updatedBy"] = p.Actor
			if err := s.Repository.Update(tx, collection, p.OrganizationID, id, expected, set); err != nil {
				return err
			}
		}
		return s.audit(tx, p, branch, action, resource, id, fields, requestID, now)
	})
}

func (s Service) ListFunds(ctx context.Context, p platform.Principal) ([]Fund, error) {
	if !s.allowed(p, "read", "") {
		return nil, denied()
	}
	return s.Repository.ListFunds(ctx, p.OrganizationID)
}
func (s Service) SaveFund(ctx context.Context, p platform.Principal, id platform.ID, i FundInput, requestID string) (*Fund, error) {
	if err := i.NormalizeAndValidate(); err != nil {
		return nil, invalid(err)
	}
	if !s.allowed(p, map[bool]string{true: "update", false: "create"}[id.Valid()], "") {
		return nil, denied()
	}
	now := s.now()
	if i.SuccessorFundID.Valid() {
		if i.SuccessorFundID == id {
			return nil, invalid(fmt.Errorf("a fund cannot succeed itself"))
		}
		exists, err := s.Repository.FundExists(ctx, p.OrganizationID, i.SuccessorFundID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, platform.ValidationError(platform.FieldError{Path: "successorFundId", Code: "not_found", Message: "Successor fund does not exist."})
		}
	}
	value := Fund{ResourceEnvelope: envelope(p, "", now), Code: i.Code, Name: i.Name, Description: i.Description, RestrictionType: i.RestrictionType, ActiveFrom: i.ActiveFrom, ActiveUntil: i.ActiveUntil, SuccessorFundID: i.SuccessorFundID}
	expected := int64(0)
	if id.Valid() {
		if err := platform.RequireExpectedVersion(i.ExpectedVersion); err != nil {
			return nil, err
		}
		current, err := s.Repository.FindFund(ctx, p.OrganizationID, id)
		if err != nil || current == nil {
			return nil, denied()
		}
		value.ResourceEnvelope = current.ResourceEnvelope
		value.ID = id
		expected = i.ExpectedVersion
	}
	set := bson.M{"code": value.Code, "name": value.Name, "description": value.Description, "restrictionType": value.RestrictionType, "activeFrom": value.ActiveFrom, "activeUntil": value.ActiveUntil, "successorFundId": value.SuccessorFundID}
	if err := s.transact(ctx, p, "", map[bool]string{true: "finance.fund.update", false: "finance.fund.create"}[id.Valid()], "fund", fundsCollection, value.ID, expected, set, value, []string{"code", "name", "restrictionType", "activeDates", "successor"}, requestID); err != nil {
		return nil, err
	}
	return s.Repository.FindFund(ctx, p.OrganizationID, value.ID)
}

func (s Service) DeactivateFund(ctx context.Context, p platform.Principal, id platform.ID, input DeactivateFundInput, requestID string) (*Fund, error) {
	if err := platform.RequireExpectedVersion(input.ExpectedVersion); err != nil {
		return nil, err
	}
	if err := platform.ValidateReason(input.Reason); err != nil {
		return nil, invalid(err)
	}
	if !s.allowed(p, "update", "") {
		return nil, denied()
	}
	current, err := s.Repository.FindFund(ctx, p.OrganizationID, id)
	if err != nil || current == nil {
		return nil, denied()
	}
	if current.Version != input.ExpectedVersion {
		return nil, platform.VersionConflict(current.Version)
	}
	if input.SuccessorFundID == id {
		return nil, invalid(fmt.Errorf("a fund cannot succeed itself"))
	}
	if input.SuccessorFundID.Valid() {
		exists, e := s.Repository.FundExists(ctx, p.OrganizationID, input.SuccessorFundID)
		if e != nil {
			return nil, e
		}
		if !exists {
			return nil, platform.ValidationError(platform.FieldError{Path: "successorFundId", Code: "not_found", Message: "Successor fund does not exist."})
		}
	}
	now := s.now()
	set := bson.M{"activeUntil": now, "successorFundId": input.SuccessorFundID, "version": current.Version + 1, "updatedAt": now, "updatedBy": p.Actor}
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if e := s.Repository.Update(tx, fundsCollection, p.OrganizationID, id, current.Version, set); e != nil {
			return e
		}
		return s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, Actor: p.Actor, Action: "finance.fund.deactivate", ResourceType: "fund", ResourceID: id, ChangedFields: []string{"activeUntil", "successor"}, Outcome: "success", Reason: input.Reason, RequestID: requestID, OccurredAt: now})
	})
	if err != nil {
		return nil, err
	}
	return s.Repository.FindFund(ctx, p.OrganizationID, id)
}
func (s Service) ListPaymentMethods(ctx context.Context, p platform.Principal) ([]PaymentMethod, error) {
	if !s.allowed(p, "read", "") {
		return nil, denied()
	}
	return s.Repository.ListPaymentMethods(ctx, p.OrganizationID)
}
func (s Service) SavePaymentMethod(ctx context.Context, p platform.Principal, id platform.ID, i PaymentMethodInput, requestID string) (*PaymentMethod, error) {
	if err := i.NormalizeAndValidate(); err != nil {
		return nil, invalid(err)
	}
	action := "create"
	if id.Valid() {
		action = "update"
	}
	if !s.allowed(p, action, "") {
		return nil, denied()
	}
	now := s.now()
	value := PaymentMethod{ResourceEnvelope: envelope(p, "", now), Code: i.Code, Name: i.Name, Kind: i.Kind, Provider: i.Provider, Active: i.Active}
	expected := int64(0)
	if id.Valid() {
		if err := platform.RequireExpectedVersion(i.ExpectedVersion); err != nil {
			return nil, err
		}
		current, err := s.Repository.FindPaymentMethod(ctx, p.OrganizationID, id)
		if err != nil || current == nil {
			return nil, denied()
		}
		value.ResourceEnvelope = current.ResourceEnvelope
		value.ID = id
		expected = i.ExpectedVersion
	}
	set := bson.M{"code": value.Code, "name": value.Name, "kind": value.Kind, "provider": value.Provider, "active": value.Active}
	if err := s.transact(ctx, p, "", "finance.payment-method."+action, "payment-method", paymentMethodsCollection, value.ID, expected, set, value, []string{"code", "name", "kind", "provider", "active"}, requestID); err != nil {
		return nil, err
	}
	return s.Repository.FindPaymentMethod(ctx, p.OrganizationID, value.ID)
}
func (s Service) ListCampuses(ctx context.Context, p platform.Principal) ([]CampusSettings, error) {
	if !s.allowed(p, "read", "") {
		return nil, denied()
	}
	return s.Repository.ListCampusSettings(ctx, p.OrganizationID)
}
func (s Service) SaveCampus(ctx context.Context, p platform.Principal, id platform.ID, i CampusSettingsInput, requestID string) (*CampusSettings, error) {
	if err := i.NormalizeAndValidate(); err != nil {
		return nil, invalid(err)
	}
	action := "create"
	if id.Valid() {
		action = "update"
	}
	if !s.allowed(p, action, i.BranchID) {
		return nil, denied()
	}
	now := s.now()
	value := CampusSettings{ResourceEnvelope: envelope(p, i.BranchID, now), Name: i.Name, Timezone: i.Timezone, Currency: i.Currency}
	expected := int64(0)
	if id.Valid() {
		if err := platform.RequireExpectedVersion(i.ExpectedVersion); err != nil {
			return nil, err
		}
		current, err := s.Repository.FindCampusSettings(ctx, p.OrganizationID, id)
		if err != nil || current == nil || current.BranchID != i.BranchID {
			return nil, denied()
		}
		value.ResourceEnvelope = current.ResourceEnvelope
		value.ID = id
		expected = i.ExpectedVersion
	}
	set := bson.M{"branchId": i.BranchID, "name": value.Name, "timezone": value.Timezone, "currency": value.Currency}
	if err := s.transact(ctx, p, i.BranchID, "finance.campus."+action, "finance-campus", campusSettingsCollection, value.ID, expected, set, value, []string{"branch", "name", "timezone", "currency"}, requestID); err != nil {
		return nil, err
	}
	return s.Repository.FindCampusSettings(ctx, p.OrganizationID, value.ID)
}
func (s Service) ListPeriods(ctx context.Context, p platform.Principal) ([]FiscalPeriod, error) {
	if !s.allowed(p, "read", "") {
		return nil, denied()
	}
	return s.Repository.ListFiscalPeriods(ctx, p.OrganizationID)
}
func (s Service) SavePeriod(ctx context.Context, p platform.Principal, id platform.ID, i FiscalPeriodInput, requestID string) (*FiscalPeriod, error) {
	if err := i.NormalizeAndValidate(); err != nil {
		return nil, invalid(err)
	}
	action := "create"
	if id.Valid() {
		action = "update"
	}
	if !s.allowed(p, action, "") {
		return nil, denied()
	}
	now := s.now()
	value := FiscalPeriod{ResourceEnvelope: envelope(p, "", now), Code: i.Code, Name: i.Name, StartsAt: i.StartsAt, EndsAt: i.EndsAt, Status: "open"}
	expected := int64(0)
	if id.Valid() {
		if err := platform.RequireExpectedVersion(i.ExpectedVersion); err != nil {
			return nil, err
		}
		current, e := s.Repository.FindFiscalPeriod(ctx, p.OrganizationID, id)
		if e != nil || current == nil {
			return nil, denied()
		}
		if current.Status != "open" {
			return nil, &platform.DomainError{Code: "immutable", Message: "Closed fiscal periods cannot be edited."}
		}
		value.ResourceEnvelope = current.ResourceEnvelope
		value.ID = id
		expected = i.ExpectedVersion
	}
	set := bson.M{"code": value.Code, "name": value.Name, "startsAt": value.StartsAt, "endsAt": value.EndsAt, "status": value.Status}
	err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if e := s.Repository.LockPeriodSchedule(tx, p.OrganizationID, now); e != nil {
			return e
		}
		overlap, e := s.Repository.HasOverlappingPeriod(tx, p.OrganizationID, id, i.StartsAt, i.EndsAt)
		if e != nil {
			return e
		}
		if overlap {
			return platform.ValidationError(platform.FieldError{Path: "startsAt", Code: "period_overlap", Message: "Fiscal periods cannot overlap."})
		}
		if expected == 0 {
			if e = s.Repository.Insert(tx, fiscalPeriodsCollection, value); e != nil {
				return e
			}
		} else {
			set["version"] = expected + 1
			set["updatedAt"] = now
			set["updatedBy"] = p.Actor
			if e = s.Repository.Update(tx, fiscalPeriodsCollection, p.OrganizationID, value.ID, expected, set); e != nil {
				return e
			}
		}
		return s.audit(tx, p, "", "finance.period."+action, "fiscal-period", value.ID, []string{"code", "name", "dates"}, requestID, now)
	})
	if err != nil {
		return nil, err
	}
	return s.Repository.FindFiscalPeriod(ctx, p.OrganizationID, value.ID)
}
func (s Service) ListSequences(ctx context.Context, p platform.Principal) ([]ReceiptSequence, error) {
	if !s.allowed(p, "read", "") {
		return nil, denied()
	}
	return s.Repository.ListReceiptSequences(ctx, p.OrganizationID)
}
func (s Service) SaveSequence(ctx context.Context, p platform.Principal, id platform.ID, i ReceiptSequenceInput, requestID string) (*ReceiptSequence, error) {
	if err := i.NormalizeAndValidate(); err != nil {
		return nil, invalid(err)
	}
	action := "create"
	if id.Valid() {
		action = "update"
	}
	if !s.allowed(p, action, "") {
		return nil, denied()
	}
	now := s.now()
	value := ReceiptSequence{ResourceEnvelope: envelope(p, "", now), Code: i.Code, Prefix: i.Prefix, FiscalYear: i.FiscalYear, Padding: i.Padding, NextNumber: i.StartingNumber}
	expected := int64(0)
	if id.Valid() {
		if err := platform.RequireExpectedVersion(i.ExpectedVersion); err != nil {
			return nil, err
		}
		current, e := s.Repository.FindReceiptSequence(ctx, p.OrganizationID, id)
		if e != nil || current == nil {
			return nil, denied()
		}
		if i.StartingNumber < current.NextNumber {
			return nil, invalid(fmt.Errorf("startingNumber cannot move backwards"))
		}
		value.ResourceEnvelope = current.ResourceEnvelope
		value.ID = id
		expected = i.ExpectedVersion
	}
	set := bson.M{"code": value.Code, "prefix": value.Prefix, "fiscalYear": value.FiscalYear, "padding": value.Padding, "nextNumber": value.NextNumber}
	if err := s.transact(ctx, p, "", "finance.receipt-sequence."+action, "receipt-sequence", receiptSequencesCollection, value.ID, expected, set, value, []string{"code", "prefix", "fiscalYear", "padding", "nextNumber"}, requestID); err != nil {
		return nil, err
	}
	return s.Repository.FindReceiptSequence(ctx, p.OrganizationID, value.ID)
}
func (s Service) ListMappings(ctx context.Context, p platform.Principal) ([]AccountMapping, error) {
	if !s.allowed(p, "read", "") {
		return nil, denied()
	}
	return s.Repository.ListAccountMappings(ctx, p.OrganizationID)
}
func (s Service) SaveMapping(ctx context.Context, p platform.Principal, id platform.ID, i AccountMappingInput, requestID string) (*AccountMapping, error) {
	if err := i.NormalizeAndValidate(); err != nil {
		return nil, invalid(err)
	}
	action := "create"
	if id.Valid() {
		action = "update"
	}
	if !s.allowed(p, action, i.BranchID) {
		return nil, denied()
	}
	exists, err := s.Repository.FundExists(ctx, p.OrganizationID, i.FundID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, platform.ValidationError(platform.FieldError{Path: "fundId", Code: "not_found", Message: "Fund does not exist."})
	}
	now := s.now()
	value := AccountMapping{ResourceEnvelope: envelope(p, i.BranchID, now), FundID: i.FundID, Category: i.Category, ExternalAccountCode: i.ExternalAccountCode, ExternalDimensionCode: i.ExternalDimensionCode, EffectiveFrom: i.EffectiveFrom, EffectiveUntil: i.EffectiveUntil}
	expected := int64(0)
	if id.Valid() {
		if err := platform.RequireExpectedVersion(i.ExpectedVersion); err != nil {
			return nil, err
		}
		current, e := s.Repository.FindAccountMapping(ctx, p.OrganizationID, id)
		if e != nil || current == nil || current.BranchID != i.BranchID {
			return nil, denied()
		}
		value.ResourceEnvelope = current.ResourceEnvelope
		value.ID = id
		expected = i.ExpectedVersion
	}
	set := bson.M{"branchId": i.BranchID, "fundId": value.FundID, "category": value.Category, "externalAccountCode": value.ExternalAccountCode, "externalDimensionCode": value.ExternalDimensionCode, "effectiveFrom": value.EffectiveFrom, "effectiveUntil": value.EffectiveUntil}
	if err = s.transact(ctx, p, i.BranchID, "finance.account-mapping."+action, "account-mapping", accountMappingsCollection, value.ID, expected, set, value, []string{"branch", "fund", "category", "externalAccountCode", "externalDimensionCode", "effectiveDates"}, requestID); err != nil {
		return nil, err
	}
	return s.Repository.FindAccountMapping(ctx, p.OrganizationID, value.ID)
}
