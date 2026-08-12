package finance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"remi-api/internal/chms/platform"
)

const (
	memberPaymentMethodsCollection  = "chms_member_payment_methods"
	recurringInstructionsCollection = "chms_recurring_giving_instructions"
)

type recurringAuthorization struct {
	InstructionID platform.ID
	Code          string
	Email         string
}

type MemberPaymentMethod struct {
	ID                     platform.ID             `json:"id" bson:"_id"`
	OrganizationID         platform.ID             `json:"-" bson:"organizationId"`
	PersonID               platform.ID             `json:"-" bson:"personId"`
	Provider               string                  `json:"provider" bson:"provider"`
	Signature              string                  `json:"-" bson:"signature"`
	EncryptedAuthorization platform.EncryptedValue `json:"-" bson:"encryptedAuthorization"`
	EncryptedEmail         platform.EncryptedValue `json:"-" bson:"encryptedEmail"`
	Channel                string                  `json:"channel" bson:"channel"`
	Brand                  string                  `json:"brand" bson:"brand"`
	CardType               string                  `json:"cardType" bson:"cardType"`
	Last4                  string                  `json:"last4" bson:"last4"`
	ExpiryMonth            string                  `json:"expiryMonth" bson:"expiryMonth"`
	ExpiryYear             string                  `json:"expiryYear" bson:"expiryYear"`
	Bank                   string                  `json:"bank" bson:"bank"`
	CountryCode            string                  `json:"countryCode" bson:"countryCode"`
	State                  string                  `json:"state" bson:"state"`
	CreatedAt              time.Time               `json:"createdAt" bson:"createdAt"`
	UpdatedAt              time.Time               `json:"updatedAt" bson:"updatedAt"`
	DisabledAt             *time.Time              `json:"disabledAt,omitempty" bson:"disabledAt,omitempty"`
}

type RecurringInstruction struct {
	ID              platform.ID      `json:"id" bson:"_id"`
	OrganizationID  platform.ID      `json:"-" bson:"organizationId"`
	PersonID        platform.ID      `json:"-" bson:"personId"`
	BranchID        platform.ID      `json:"branchId" bson:"branchId"`
	Donor           DonorAttribution `json:"donor" bson:"donor"`
	PaymentMethodID platform.ID      `json:"paymentMethodId" bson:"paymentMethodId"`
	FundID          platform.ID      `json:"fundId" bson:"fundId"`
	CampaignID      platform.ID      `json:"campaignId,omitempty" bson:"campaignId,omitempty"`
	PledgeID        platform.ID      `json:"pledgeId,omitempty" bson:"pledgeId,omitempty"`
	AmountMinor     int64            `json:"amountMinor" bson:"amountMinor"`
	Currency        string           `json:"currency" bson:"currency"`
	Frequency       string           `json:"frequency" bson:"frequency"`
	State           string           `json:"state" bson:"state"`
	NextChargeAt    time.Time        `json:"nextChargeAt" bson:"nextChargeAt"`
	LastIntentID    platform.ID      `json:"lastIntentId,omitempty" bson:"lastIntentId,omitempty"`
	ActionURL       string           `json:"actionUrl,omitempty" bson:"actionUrl,omitempty"`
	FailureCount    int32            `json:"failureCount" bson:"failureCount"`
	LastFailureCode string           `json:"lastFailureCode,omitempty" bson:"lastFailureCode,omitempty"`
	Version         int64            `json:"version" bson:"version"`
	CreatedAt       time.Time        `json:"createdAt" bson:"createdAt"`
	UpdatedAt       time.Time        `json:"updatedAt" bson:"updatedAt"`
	CancelledAt     *time.Time       `json:"cancelledAt,omitempty" bson:"cancelledAt,omitempty"`
	LeaseUntil      *time.Time       `json:"-" bson:"leaseUntil,omitempty"`
}

type RecurringInstructionInput struct {
	BranchID        platform.ID      `json:"branchId"`
	Donor           DonorAttribution `json:"donor"`
	PaymentMethodID platform.ID      `json:"paymentMethodId"`
	FundID          platform.ID      `json:"fundId"`
	CampaignID      platform.ID      `json:"campaignId,omitempty"`
	PledgeID        platform.ID      `json:"pledgeId,omitempty"`
	AmountMinor     int64            `json:"amountMinor"`
	Currency        string           `json:"currency"`
	Frequency       string           `json:"frequency"`
	StartsAt        time.Time        `json:"startsAt"`
}

func (s Service) captureMemberPaymentMethod(ctx context.Context, intent *PaymentIntent, authorization ProviderAuthorization, requestID string) error {
	if s.Repository == nil || s.Platform == nil || s.Cipher == nil || intent == nil || !intent.Donor.PersonID.Valid() || !authorization.Reusable || strings.TrimSpace(authorization.Code) == "" || strings.TrimSpace(authorization.Signature) == "" {
		return errors.New("reusable member authorization is incomplete")
	}
	email, err := s.Cipher.Decrypt(intent.AuthorizationEmail, []byte(string(intent.OrganizationID)+":"+string(intent.ID)+":authorization-email"))
	if err != nil {
		return err
	}
	now := s.now()
	collection := s.Repository.database.Collection(memberPaymentMethodsCollection)
	var existing MemberPaymentMethod
	findErr := collection.FindOne(ctx, bson.M{"organizationId": intent.OrganizationID, "personId": intent.Donor.PersonID, "provider": "paystack", "signature": authorization.Signature}).Decode(&existing)
	id := platform.ID(bson.NewObjectID().Hex())
	createdAt := now
	if findErr == nil {
		id, createdAt = existing.ID, existing.CreatedAt
	} else if findErr != mongo.ErrNoDocuments {
		return findErr
	}
	rawAuthorization, err := json.Marshal(authorization)
	if err != nil {
		return err
	}
	aad := string(intent.OrganizationID) + ":" + string(intent.Donor.PersonID) + ":" + string(id)
	encryptedAuthorization, err := s.Cipher.Encrypt(rawAuthorization, []byte(aad+":authorization"))
	if err != nil {
		return err
	}
	encryptedEmail, err := s.Cipher.Encrypt(email, []byte(aad+":email"))
	if err != nil {
		return err
	}
	method := MemberPaymentMethod{ID: id, OrganizationID: intent.OrganizationID, PersonID: intent.Donor.PersonID, Provider: "paystack", Signature: authorization.Signature, EncryptedAuthorization: encryptedAuthorization, EncryptedEmail: encryptedEmail, Channel: authorization.Channel, Brand: authorization.Brand, CardType: authorization.CardType, Last4: authorization.Last4, ExpiryMonth: authorization.ExpiryMonth, ExpiryYear: authorization.ExpiryYear, Bank: authorization.Bank, CountryCode: authorization.CountryCode, State: "active", CreatedAt: createdAt, UpdatedAt: now}
	return s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if _, updateErr := s.Repository.database.Collection(memberPaymentMethodsCollection).ReplaceOne(tx, bson.M{"_id": id, "organizationId": intent.OrganizationID, "personId": intent.Donor.PersonID}, method, options.Replace().SetUpsert(true)); updateErr != nil {
			return updateErr
		}
		return s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: intent.OrganizationID, BranchID: intent.BranchID, Actor: platform.Actor{Type: platform.ActorSystem, ID: "paystack-webhook"}, Action: "finance.member-payment-method.verified", ResourceType: "member-payment-method", ResourceID: id, SubjectIDs: []platform.ID{intent.Donor.PersonID}, ChangedFields: []string{"provider", "signature", "maskedDetails", "state"}, Outcome: "success", RequestID: requestID, OccurredAt: now})
	})
}

func (s Service) ListMemberPaymentMethods(ctx context.Context, org, personID platform.ID) ([]MemberPaymentMethod, error) {
	if s.Repository == nil || !org.Valid() || !personID.Valid() {
		return nil, errors.New("member payment-method service is unavailable")
	}
	cursor, err := s.Repository.database.Collection(memberPaymentMethodsCollection).Find(ctx, bson.M{"organizationId": org, "personId": personID, "state": "active"}, options.Find().SetProjection(bson.M{"encryptedAuthorization": 0, "encryptedEmail": 0, "signature": 0}).SetSort(bson.D{{Key: "updatedAt", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var values []MemberPaymentMethod
	if err = cursor.All(ctx, &values); err != nil {
		return nil, err
	}
	return values, nil
}

func (s Service) DisableMemberPaymentMethod(ctx context.Context, org, personID, methodID platform.ID, requestID string) error {
	if s.Repository == nil || s.Platform == nil || !org.Valid() || !personID.Valid() || !methodID.Valid() {
		return &platform.DomainError{Code: "not_found", Message: "Payment method not found."}
	}
	now := s.now()
	return s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		result, err := s.Repository.database.Collection(memberPaymentMethodsCollection).UpdateOne(tx, bson.M{"_id": methodID, "organizationId": org, "personId": personID, "state": "active"}, bson.M{"$set": bson.M{"state": "disabled", "disabledAt": now, "updatedAt": now}})
		if err != nil {
			return err
		}
		if result.ModifiedCount != 1 {
			return &platform.DomainError{Code: "not_found", Message: "Payment method not found."}
		}
		_, err = s.Repository.database.Collection(recurringInstructionsCollection).UpdateMany(tx, bson.M{"organizationId": org, "personId": personID, "paymentMethodId": methodID, "state": bson.M{"$in": bson.A{"active", "needs-action"}}}, bson.M{"$set": bson.M{"state": "cancelled", "cancelledAt": now, "updatedAt": now}, "$inc": bson.M{"version": 1}})
		if err != nil {
			return err
		}
		return s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: org, Actor: platform.Actor{Type: platform.ActorMember, ID: personID}, Action: "finance.member-payment-method.disabled", ResourceType: "member-payment-method", ResourceID: methodID, SubjectIDs: []platform.ID{personID}, ChangedFields: []string{"state", "disabledAt"}, Outcome: "success", RequestID: requestID, OccurredAt: now})
	})
}

func (s Service) CreateRecurringInstruction(ctx context.Context, org, personID platform.ID, input RecurringInstructionInput, requestID string) (*RecurringInstruction, error) {
	input.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))
	input.Frequency = strings.ToLower(strings.TrimSpace(input.Frequency))
	if input.Currency == "" {
		input.Currency = "GHS"
	}
	if !org.Valid() || !personID.Valid() || !input.BranchID.Valid() || !input.PaymentMethodID.Valid() || !input.FundID.Valid() || input.AmountMinor < 100 || input.AmountMinor > 100_000_000_00 || input.Currency != "GHS" || (input.Frequency != "weekly" && input.Frequency != "monthly" && input.Frequency != "quarterly") {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_recurring_instruction", Message: "Choose a payment method, fund, amount and weekly, monthly or quarterly rhythm."})
	}
	if err := input.Donor.NormalizeAndValidate(); err != nil || (input.Donor.Type != "person" && input.Donor.Type != "household") {
		return nil, platform.ValidationError(platform.FieldError{Path: "donor", Code: "invalid", Message: "Choose your own profile or eligible household."})
	}
	var method MemberPaymentMethod
	if s.Repository.database.Collection(memberPaymentMethodsCollection).FindOne(ctx, bson.M{"_id": input.PaymentMethodID, "organizationId": org, "personId": personID, "state": "active"}).Decode(&method) != nil {
		return nil, &platform.DomainError{Code: "not_found", Message: "Payment method not found."}
	}
	if fund, err := s.Repository.ResolvePublicFund(ctx, org, input.FundID, "", s.now()); err != nil || fund == nil {
		if err != nil {
			return nil, err
		}
		return nil, platform.ValidationError(platform.FieldError{Path: "fundId", Code: "not_available", Message: "That fund is not accepting gifts."})
	}
	now := s.now()
	startsAt := input.StartsAt.UTC()
	if startsAt.IsZero() || startsAt.Before(now.Add(5*time.Minute)) {
		startsAt = now.Add(24 * time.Hour)
	}
	value := &RecurringInstruction{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: org, PersonID: personID, BranchID: input.BranchID, Donor: input.Donor, PaymentMethodID: input.PaymentMethodID, FundID: input.FundID, CampaignID: input.CampaignID, PledgeID: input.PledgeID, AmountMinor: input.AmountMinor, Currency: input.Currency, Frequency: input.Frequency, State: "active", NextChargeAt: startsAt, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if _, err := s.Repository.database.Collection(recurringInstructionsCollection).InsertOne(tx, value); err != nil {
			return err
		}
		return s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: org, BranchID: input.BranchID, Actor: platform.Actor{Type: platform.ActorMember, ID: personID}, Action: "finance.recurring-instruction.created", ResourceType: "recurring-giving-instruction", ResourceID: value.ID, SubjectIDs: []platform.ID{personID}, ChangedFields: []string{"donor", "paymentMethodId", "fundId", "amountMinor", "frequency", "nextChargeAt", "state"}, Outcome: "success", RequestID: requestID, OccurredAt: now})
	}); err != nil {
		return nil, err
	}
	return value, nil
}

func (s Service) ListRecurringInstructions(ctx context.Context, org, personID platform.ID) ([]RecurringInstruction, error) {
	cursor, err := s.Repository.database.Collection(recurringInstructionsCollection).Find(ctx, bson.M{"organizationId": org, "personId": personID}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var values []RecurringInstruction
	if err = cursor.All(ctx, &values); err != nil {
		return nil, err
	}
	return values, nil
}

func (s Service) UpdateRecurringInstructionState(ctx context.Context, org, personID, id platform.ID, expectedVersion int64, state, requestID string) (*RecurringInstruction, error) {
	state = strings.ToLower(strings.TrimSpace(state))
	if state != "active" && state != "paused" && state != "cancelled" {
		return nil, platform.ValidationError(platform.FieldError{Path: "state", Code: "invalid", Message: "Choose active, paused or cancelled."})
	}
	if expectedVersion < 1 {
		return nil, platform.RequireExpectedVersion(expectedVersion)
	}
	now := s.now()
	set := bson.M{"state": state, "updatedAt": now, "lastFailureCode": "", "actionUrl": ""}
	if state == "cancelled" {
		set["cancelledAt"] = now
	}
	var value RecurringInstruction
	if err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		result := s.Repository.database.Collection(recurringInstructionsCollection).FindOneAndUpdate(tx, bson.M{"_id": id, "organizationId": org, "personId": personID, "version": expectedVersion, "state": bson.M{"$ne": "cancelled"}}, bson.M{"$set": set, "$inc": bson.M{"version": 1}}, options.FindOneAndUpdate().SetReturnDocument(options.After))
		if err := result.Decode(&value); err != nil {
			if err == mongo.ErrNoDocuments {
				return &platform.DomainError{Code: "version_conflict", Message: "Recurring instruction changed; refresh and try again."}
			}
			return err
		}
		return s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: org, BranchID: value.BranchID, Actor: platform.Actor{Type: platform.ActorMember, ID: personID}, Action: "finance.recurring-instruction." + state, ResourceType: "recurring-giving-instruction", ResourceID: id, SubjectIDs: []platform.ID{personID}, ChangedFields: []string{"state"}, Outcome: "success", RequestID: requestID, OccurredAt: now})
	}); err != nil {
		return nil, err
	}
	return &value, nil
}

func nextRecurringTime(from time.Time, frequency string) time.Time {
	switch frequency {
	case "weekly":
		return from.AddDate(0, 0, 7)
	case "quarterly":
		return from.AddDate(0, 3, 0)
	default:
		return from.AddDate(0, 1, 0)
	}
}

func (s Service) RunDueRecurring(ctx context.Context) (int, error) {
	if s.Repository == nil || s.Cipher == nil || s.Provider == nil || !s.Provider.Configured() {
		return 0, nil
	}
	now, processed := s.now(), 0
	for processed < 25 {
		var instruction RecurringInstruction
		err := s.Repository.database.Collection(recurringInstructionsCollection).FindOneAndUpdate(ctx, bson.M{"state": "active", "nextChargeAt": bson.M{"$lte": now}, "$or": bson.A{bson.M{"leaseUntil": bson.M{"$exists": false}}, bson.M{"leaseUntil": bson.M{"$lte": now}}}}, bson.M{"$set": bson.M{"leaseUntil": now.Add(2 * time.Minute), "updatedAt": now}}, options.FindOneAndUpdate().SetSort(bson.D{{Key: "nextChargeAt", Value: 1}}).SetReturnDocument(options.After)).Decode(&instruction)
		if err == mongo.ErrNoDocuments {
			break
		}
		if err != nil {
			return processed, err
		}
		processed++
		var method MemberPaymentMethod
		if err = s.Repository.database.Collection(memberPaymentMethodsCollection).FindOne(ctx, bson.M{"_id": instruction.PaymentMethodID, "organizationId": instruction.OrganizationID, "personId": instruction.PersonID, "state": "active"}).Decode(&method); err != nil {
			s.failRecurring(ctx, instruction, "payment_method_unavailable", now)
			continue
		}
		aad := string(instruction.OrganizationID) + ":" + string(instruction.PersonID) + ":" + string(method.ID)
		rawAuth, authErr := s.Cipher.Decrypt(method.EncryptedAuthorization, []byte(aad+":authorization"))
		rawEmail, emailErr := s.Cipher.Decrypt(method.EncryptedEmail, []byte(aad+":email"))
		var authorization ProviderAuthorization
		if authErr != nil || emailErr != nil || json.Unmarshal(rawAuth, &authorization) != nil || !authorization.Reusable || authorization.Code == "" {
			s.failRecurring(ctx, instruction, "authorization_unavailable", now)
			continue
		}
		input := PaymentIntentInput{Email: string(rawEmail), AmountMinor: instruction.AmountMinor, Currency: instruction.Currency, FundID: instruction.FundID, CampaignID: instruction.CampaignID, PledgeID: instruction.PledgeID, Donor: instruction.Donor, recurringAuthorization: &recurringAuthorization{InstructionID: instruction.ID, Code: authorization.Code, Email: string(rawEmail)}}
		key := fmt.Sprintf("recurring-%s-%d", instruction.ID, instruction.NextChargeAt.Unix())
		intent, intentErr := s.CreatePaystackIntent(ctx, instruction.OrganizationID, input, "recurring-worker", key)
		if intentErr != nil {
			s.failRecurring(ctx, instruction, "provider_submit_failed", now)
			continue
		}
		set := bson.M{"nextChargeAt": nextRecurringTime(instruction.NextChargeAt, instruction.Frequency), "lastIntentId": intent.ID, "failureCount": 0, "lastFailureCode": "", "leaseUntil": nil, "updatedAt": now}
		if intent.AuthorizationURL != "" {
			set["state"], set["actionUrl"] = "needs-action", intent.AuthorizationURL
		}
		_, _ = s.Repository.database.Collection(recurringInstructionsCollection).UpdateOne(ctx, bson.M{"_id": instruction.ID, "leaseUntil": bson.M{"$gt": now}}, bson.M{"$set": set, "$inc": bson.M{"version": 1}})
	}
	return processed, nil
}

func (s Service) failRecurring(ctx context.Context, value RecurringInstruction, code string, now time.Time) {
	state := "active"
	if value.FailureCount+1 >= 3 {
		state = "paused"
	}
	_, _ = s.Repository.database.Collection(recurringInstructionsCollection).UpdateOne(ctx, bson.M{"_id": value.ID, "organizationId": value.OrganizationID}, bson.M{"$set": bson.M{"state": state, "lastFailureCode": code, "leaseUntil": nil, "nextChargeAt": now.Add(6 * time.Hour), "updatedAt": now}, "$inc": bson.M{"failureCount": 1, "version": 1}})
}
