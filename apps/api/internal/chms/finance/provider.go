package finance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"remi-api/internal/chms/platform"
)

type ProviderInitializeRequest struct {
	AmountMinor int64
	Currency    string
	Email       string
	Reference   string
	CallbackURL string
	Metadata    map[string]any
}

type ProviderInitializeResult struct {
	AuthorizationURL string
	Reference        string
}

type ProviderVerification struct {
	Reference           string
	Status              string
	AmountMinor         int64
	Currency            string
	Channel             string
	TransactionID       string
	SettlementReference string
	PaidAt              time.Time
	Authorization       *ProviderAuthorization
}

type ProviderAuthorization struct {
	Code        string `json:"authorizationCode"`
	Signature   string `json:"signature"`
	Reusable    bool   `json:"reusable"`
	Channel     string `json:"channel"`
	CardType    string `json:"cardType"`
	Brand       string `json:"brand"`
	Last4       string `json:"last4"`
	ExpiryMonth string `json:"expiryMonth"`
	ExpiryYear  string `json:"expiryYear"`
	Bank        string `json:"bank"`
	CountryCode string `json:"countryCode"`
}

type ProviderChargeResult struct {
	Reference        string
	Status           string
	AuthorizationURL string
}

type RecurringPaymentProvider interface {
	ChargeAuthorization(context.Context, string, string, int64, string, string, map[string]any) (*ProviderChargeResult, error)
}

type PaymentProvider interface {
	Configured() bool
	VerifyWebhook(raw []byte, signature string) bool
	InitializePayment(context.Context, ProviderInitializeRequest) (*ProviderInitializeResult, error)
	VerifyPayment(context.Context, string) (*ProviderVerification, error)
}

type PaymentIntentInput struct {
	Email                  string           `json:"email"`
	AmountMinor            int64            `json:"amount"`
	Currency               string           `json:"currency,omitempty"`
	Category               string           `json:"category"`
	BranchID               platform.ID      `json:"branchId,omitempty"`
	FundID                 platform.ID      `json:"fundId,omitempty"`
	CampaignID             platform.ID      `json:"campaignId,omitempty"`
	PledgeID               platform.ID      `json:"pledgeId,omitempty"`
	PaymentMethodID        platform.ID      `json:"paymentMethodId,omitempty"`
	Donor                  DonorAttribution `json:"donor,omitempty"`
	Website                string           `json:"website,omitempty"`
	SavePaymentMethod      bool             `json:"savePaymentMethod,omitempty"`
	CallbackURL            string           `json:"-"`
	recurringAuthorization *recurringAuthorization
	memberPersonID         platform.ID
}

// CreateMemberPaystackIntent binds a public member request to its authenticated
// person. This prevents callers from saving a reusable authorization for a
// different donor, even if they forge the JSON body.
func (s Service) CreateMemberPaystackIntent(ctx context.Context, org, personID platform.ID, input PaymentIntentInput, requestID, idempotencyKey string) (*PaymentIntentResult, error) {
	if !personID.Valid() {
		return nil, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to give."}
	}
	input.memberPersonID = personID
	return s.CreatePaystackIntent(ctx, org, input, requestID, idempotencyKey)
}

func (i *PaymentIntentInput) NormalizeAndValidate() error {
	i.Email = strings.ToLower(strings.TrimSpace(i.Email))
	i.Currency = strings.ToUpper(strings.TrimSpace(i.Currency))
	if i.Currency == "" {
		i.Currency = "GHS"
	}
	i.Category = strings.TrimSpace(i.Category)
	if strings.TrimSpace(i.Website) != "" {
		return fmt.Errorf("invalid submission")
	}
	parsed, err := mail.ParseAddress(i.Email)
	if err != nil || !strings.EqualFold(parsed.Address, i.Email) || len(i.Email) > 254 {
		return fmt.Errorf("a valid email is required")
	}
	if i.AmountMinor <= 0 || i.AmountMinor > 100_000_000_00 {
		return fmt.Errorf("amount must be between GHS 0.01 and GHS 100,000,000")
	}
	if i.Currency != "GHS" {
		return fmt.Errorf("only GHS is supported")
	}
	if !i.FundID.Valid() && !i.CampaignID.Valid() && (len(i.Category) < 2 || len(i.Category) > 120) {
		return fmt.Errorf("category or fundId is required")
	}
	if i.Donor.Type == "" {
		i.Donor = DonorAttribution{Type: "unresolved"}
	}
	return i.Donor.NormalizeAndValidate()
}

type PaymentIntent struct {
	platform.ResourceEnvelope `bson:",inline"`
	Provider                  string                  `json:"provider" bson:"provider"`
	Reference                 string                  `json:"reference" bson:"reference"`
	ClientRequestKey          string                  `json:"-" bson:"clientRequestKey"`
	RequestHash               string                  `json:"-" bson:"requestHash"`
	State                     string                  `json:"state" bson:"state"`
	EmailHash                 string                  `json:"-" bson:"emailHash"`
	EmailMasked               string                  `json:"emailMasked" bson:"emailMasked"`
	Category                  string                  `json:"category" bson:"category"`
	CampaignID                platform.ID             `json:"campaignId,omitempty" bson:"campaignId,omitempty"`
	PledgeID                  platform.ID             `json:"pledgeId,omitempty" bson:"pledgeId,omitempty"`
	Donor                     DonorAttribution        `json:"donor" bson:"donor"`
	PaymentMethodID           platform.ID             `json:"paymentMethodId" bson:"paymentMethodId"`
	Total                     platform.Money          `json:"total" bson:"total"`
	Splits                    []ContributionSplit     `json:"splits" bson:"splits"`
	AuthorizationURL          string                  `json:"-" bson:"authorizationUrl,omitempty"`
	SavePaymentMethod         bool                    `json:"-" bson:"savePaymentMethod,omitempty"`
	AuthorizationEmail        platform.EncryptedValue `json:"-" bson:"authorizationEmail,omitempty"`
	RecurringInstructionID    platform.ID             `json:"recurringInstructionId,omitempty" bson:"recurringInstructionId,omitempty"`
	ProviderStatus            string                  `json:"providerStatus,omitempty" bson:"providerStatus,omitempty"`
	ProviderTransactionID     string                  `json:"providerTransactionId,omitempty" bson:"providerTransactionId,omitempty"`
	Channel                   string                  `json:"channel,omitempty" bson:"channel,omitempty"`
	SettlementReference       string                  `json:"settlementReference,omitempty" bson:"settlementReference,omitempty"`
	ContributionID            platform.ID             `json:"contributionId,omitempty" bson:"contributionId,omitempty"`
	RefundState               string                  `json:"refundState,omitempty" bson:"refundState,omitempty"`
	RefundedAmountMinor       int64                   `json:"refundedAmountMinor,omitempty" bson:"refundedAmountMinor,omitempty"`
	AppliedProviderEvents     []string                `json:"-" bson:"appliedProviderEvents,omitempty"`
	ChargebackState           string                  `json:"chargebackState,omitempty" bson:"chargebackState,omitempty"`
	FailureCode               string                  `json:"failureCode,omitempty" bson:"failureCode,omitempty"`
	LastProviderEventAt       *time.Time              `json:"lastProviderEventAt,omitempty" bson:"lastProviderEventAt,omitempty"`
}

type PaymentIntentResult struct {
	ID               platform.ID `json:"id"`
	Reference        string      `json:"reference"`
	AuthorizationURL string      `json:"authorizationUrl"`
	State            string      `json:"state"`
	Demo             bool        `json:"demo,omitempty"`
}

type ProviderInbox struct {
	ID             platform.ID `json:"id" bson:"_id"`
	OrganizationID platform.ID `json:"organizationId" bson:"organizationId"`
	Provider       string      `json:"provider" bson:"provider"`
	BodyHash       string      `json:"bodyHash" bson:"bodyHash"`
	RawBody        []byte      `json:"-" bson:"rawBody,omitempty"`
	SignatureValid bool        `json:"signatureValid" bson:"signatureValid"`
	EventType      string      `json:"eventType,omitempty" bson:"eventType,omitempty"`
	Reference      string      `json:"reference,omitempty" bson:"reference,omitempty"`
	State          string      `json:"state" bson:"state"`
	Attempts       int32       `json:"attempts" bson:"attempts"`
	LastErrorCode  string      `json:"lastErrorCode,omitempty" bson:"lastErrorCode,omitempty"`
	ReceivedAt     time.Time   `json:"receivedAt" bson:"receivedAt"`
	ClaimedAt      *time.Time  `json:"claimedAt,omitempty" bson:"claimedAt,omitempty"`
	ProcessedAt    *time.Time  `json:"processedAt,omitempty" bson:"processedAt,omitempty"`
	NextAttemptAt  *time.Time  `json:"nextAttemptAt,omitempty" bson:"nextAttemptAt,omitempty"`
}

type ProviderException struct {
	ID             platform.ID `json:"id" bson:"_id"`
	OrganizationID platform.ID `json:"organizationId" bson:"organizationId"`
	Provider       string      `json:"provider" bson:"provider"`
	InboxID        platform.ID `json:"inboxId" bson:"inboxId"`
	IntentID       platform.ID `json:"intentId,omitempty" bson:"intentId,omitempty"`
	Reference      string      `json:"reference,omitempty" bson:"reference,omitempty"`
	Code           string      `json:"code" bson:"code"`
	State          string      `json:"state" bson:"state"`
	CreatedAt      time.Time   `json:"createdAt" bson:"createdAt"`
}

type WebhookReceipt struct {
	Accepted  bool        `json:"accepted"`
	Duplicate bool        `json:"duplicate"`
	InboxID   platform.ID `json:"inboxId,omitempty"`
}

type paystackWebhook struct {
	Event string         `json:"event"`
	Data  map[string]any `json:"data"`
}

func emailFingerprint(email string) (string, string) {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	parts := strings.Split(email, "@")
	masked := "***"
	if len(parts) == 2 {
		prefix := []rune(parts[0])
		if len(prefix) > 0 {
			masked = string(prefix[0]) + "***@" + parts[1]
		}
	}
	return hex.EncodeToString(sum[:]), masked
}

func providerSystemPrincipal(org, branch platform.ID) platform.Principal {
	return platform.Principal{Actor: platform.Actor{Type: platform.ActorSystem, ID: "paystack-webhook"}, OrganizationID: org, Roles: []string{"payment-provider"}, Grants: []platform.Grant{{Action: "create", Resource: "finance-ledger", BranchIDs: []platform.ID{branch}, FieldClasses: []platform.FieldClass{platform.FieldFinancial}}, {Action: "approve", Resource: "finance-ledger", BranchIDs: []platform.ID{branch}, FieldClasses: []platform.FieldClass{platform.FieldFinancial}}}}
}

func (s Service) CreatePaystackIntent(ctx context.Context, org platform.ID, input PaymentIntentInput, requestID, idempotencyKey string) (*PaymentIntentResult, error) {
	if !org.Valid() || s.Repository == nil || s.Platform == nil {
		return nil, errors.New("payment intent service is not configured")
	}
	if err := platform.ValidateIdempotencyKey(idempotencyKey); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "Idempotency-Key", Code: "invalid", Message: err.Error()})
	}
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_payment_intent", Message: err.Error()})
	}
	hash, err := commandHash(input)
	if err != nil {
		return nil, err
	}
	if existing, e := s.Repository.FindPaymentIntentByClientKey(ctx, org, idempotencyKey); e != nil {
		return nil, e
	} else if existing != nil {
		if existing.RequestHash != hash {
			return nil, &platform.DomainError{Code: "idempotency_key_reused", Message: "This checkout key was already used for different payment details."}
		}
		return &PaymentIntentResult{ID: existing.ID, Reference: existing.Reference, AuthorizationURL: existing.AuthorizationURL, State: existing.State, Demo: s.Provider == nil || !s.Provider.Configured()}, nil
	}
	branch, err := s.Repository.ResolvePaymentBranch(ctx, org, input.BranchID)
	if err != nil {
		return nil, err
	}
	if !branch.Valid() {
		return nil, platform.ValidationError(platform.FieldError{Path: "branchId", Code: "not_configured", Message: "Online giving is not configured for a branch."})
	}
	if input.CampaignID.Valid() {
		campaign, campaignErr := s.Repository.FindCampaign(ctx, org, input.CampaignID)
		if campaignErr != nil {
			return nil, campaignErr
		}
		now := s.now()
		if campaign == nil || campaign.Status != "published" || now.Before(campaign.StartsAt) || now.After(campaign.EndsAt) {
			return nil, platform.ValidationError(platform.FieldError{Path: "campaignId", Code: "not_available", Message: "That fundraising campaign is not accepting gifts."})
		}
		input.FundID, input.Category = campaign.FundID, campaign.Title
	}
	if input.PledgeID.Valid() {
		pledge, pledgeErr := s.Repository.FindPledge(ctx, org, input.PledgeID)
		if pledgeErr != nil {
			return nil, pledgeErr
		}
		if pledge == nil || pledge.State != "active" || pledge.Donor != input.Donor || pledge.FundID != input.FundID || (pledge.CampaignID.Valid() && pledge.CampaignID != input.CampaignID) {
			return nil, platform.ValidationError(platform.FieldError{Path: "pledgeId", Code: "attribution_mismatch", Message: "Pledge must be active and match the donor, fund and campaign."})
		}
	}
	fund, err := s.Repository.ResolvePublicFund(ctx, org, input.FundID, input.Category, s.now())
	if err != nil {
		return nil, err
	}
	if fund == nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "category", Code: "not_configured", Message: "That giving category is not currently available."})
	}
	method, err := s.Repository.ResolvePaystackMethod(ctx, org, input.PaymentMethodID)
	if err != nil {
		return nil, err
	}
	if method == nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "paymentMethodId", Code: "not_configured", Message: "Paystack giving is not configured."})
	}
	if period, e := s.Repository.FindPeriodForDate(ctx, org, s.now()); e != nil {
		return nil, e
	} else if period == nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "no_open_period", Message: "Online giving is temporarily unavailable while the finance period is closed."})
	}
	if sequence, e := s.Repository.FindSequenceForYear(ctx, org, s.now().Year()); e != nil {
		return nil, e
	} else if sequence == nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "no_receipt_sequence", Message: "Online giving receipt configuration is incomplete."})
	}
	now := s.now()
	reference := "REMI-" + now.Format("20060102") + "-" + bson.NewObjectID().Hex()
	emailHash, emailMasked := emailFingerprint(input.Email)
	actor := platform.Actor{Type: platform.ActorSystem, ID: "public-giving"}
	intent := PaymentIntent{ResourceEnvelope: platform.ResourceEnvelope{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: org, BranchID: branch, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: actor, UpdatedAt: now, UpdatedBy: actor}, Provider: "paystack", Reference: reference, ClientRequestKey: idempotencyKey, RequestHash: hash, State: "pending", EmailHash: emailHash, EmailMasked: emailMasked, Category: fund.Name, CampaignID: input.CampaignID, PledgeID: input.PledgeID, Donor: input.Donor, PaymentMethodID: method.ID, Total: platform.Money{AmountMinor: input.AmountMinor, Currency: input.Currency}, Splits: []ContributionSplit{{FundID: fund.ID, Amount: platform.Money{AmountMinor: input.AmountMinor, Currency: input.Currency}}}, SavePaymentMethod: input.SavePaymentMethod}
	if input.SavePaymentMethod {
		if s.Cipher == nil || !input.memberPersonID.Valid() || input.Donor.Type != "person" || input.Donor.PersonID != input.memberPersonID {
			return nil, platform.ValidationError(platform.FieldError{Path: "savePaymentMethod", Code: "member_required", Message: "Sign in as the attributed member to save a payment method."})
		}
		intent.AuthorizationEmail, err = s.Cipher.Encrypt([]byte(input.Email), []byte(string(org)+":"+string(intent.ID)+":authorization-email"))
		if err != nil {
			return nil, err
		}
	}
	if input.recurringAuthorization != nil {
		intent.RecurringInstructionID = input.recurringAuthorization.InstructionID
	}
	if err = s.Repository.InsertPaymentIntent(ctx, intent); err != nil {
		if replay, replayErr := s.Repository.FindPaymentIntentByClientKey(ctx, org, idempotencyKey); replayErr == nil && replay != nil && replay.RequestHash == hash {
			return &PaymentIntentResult{ID: replay.ID, Reference: replay.Reference, AuthorizationURL: replay.AuthorizationURL, State: replay.State, Demo: s.Provider == nil || !s.Provider.Configured()}, nil
		}
		return nil, err
	}
	if s.Provider == nil || !s.Provider.Configured() {
		if !s.AllowProviderDemo {
			_ = s.Repository.UpdatePaymentIntent(ctx, org, intent.ID, 1, bson.M{"state": "failed", "failureCode": "provider_not_configured", "version": int64(2), "updatedAt": now})
			return nil, &platform.DomainError{Code: "provider_unavailable", Message: "Online giving is temporarily unavailable."}
		}
		intent.AuthorizationURL = "/give/demo-success?ref=" + reference
		if err = s.Repository.UpdatePaymentIntent(ctx, org, intent.ID, 1, bson.M{"authorizationUrl": intent.AuthorizationURL, "version": int64(2), "updatedAt": now}); err != nil {
			return nil, err
		}
		return &PaymentIntentResult{ID: intent.ID, Reference: reference, AuthorizationURL: intent.AuthorizationURL, State: "pending", Demo: true}, nil
	}
	if input.recurringAuthorization != nil {
		recurringProvider, ok := s.Provider.(RecurringPaymentProvider)
		if !ok {
			_ = s.Repository.UpdatePaymentIntent(ctx, org, intent.ID, 1, bson.M{"state": "failed", "failureCode": "recurring_not_supported", "version": int64(2), "updatedAt": now})
			return nil, &platform.DomainError{Code: "provider_unavailable", Message: "Recurring giving is temporarily unavailable."}
		}
		charged, chargeErr := recurringProvider.ChargeAuthorization(ctx, input.recurringAuthorization.Code, input.recurringAuthorization.Email, input.AmountMinor, input.Currency, reference, map[string]any{"intent_id": intent.ID, "recurring_instruction_id": input.recurringAuthorization.InstructionID, "organization_id": org})
		if chargeErr != nil || charged == nil || charged.Reference != reference {
			_ = s.Repository.UpdatePaymentIntent(ctx, org, intent.ID, 1, bson.M{"state": "failed", "failureCode": "recurring_charge_failed", "version": int64(2), "updatedAt": now})
			return nil, &platform.DomainError{Code: "provider_unavailable", Message: "The scheduled gift could not be submitted."}
		}
		if strings.TrimSpace(charged.AuthorizationURL) != "" {
			intent.AuthorizationURL = charged.AuthorizationURL
		}
		if err = s.Repository.UpdatePaymentIntent(ctx, org, intent.ID, 1, bson.M{"authorizationUrl": intent.AuthorizationURL, "providerStatus": charged.Status, "version": int64(2), "updatedAt": now}); err != nil {
			return nil, err
		}
		return &PaymentIntentResult{ID: intent.ID, Reference: reference, AuthorizationURL: intent.AuthorizationURL, State: "pending"}, nil
	}
	initialized, err := s.Provider.InitializePayment(ctx, ProviderInitializeRequest{AmountMinor: input.AmountMinor, Currency: input.Currency, Email: input.Email, Reference: reference, CallbackURL: input.CallbackURL, Metadata: map[string]any{"intent_id": intent.ID, "category": fund.Code, "campaign_id": input.CampaignID, "pledge_id": input.PledgeID, "organization_id": org}})
	if err != nil {
		_ = s.Repository.UpdatePaymentIntent(ctx, org, intent.ID, 1, bson.M{"state": "failed", "failureCode": "provider_initialize_failed", "version": int64(2), "updatedAt": now})
		return nil, &platform.DomainError{Code: "provider_unavailable", Message: "The payment provider could not start checkout. Please try again."}
	}
	if initialized.Reference != reference || strings.TrimSpace(initialized.AuthorizationURL) == "" {
		_ = s.Repository.UpdatePaymentIntent(ctx, org, intent.ID, 1, bson.M{"state": "failed", "failureCode": "provider_reference_mismatch", "version": int64(2), "updatedAt": now})
		return nil, &platform.DomainError{Code: "dependency_unavailable", Message: "The payment provider returned an invalid checkout response."}
	}
	intent.AuthorizationURL = initialized.AuthorizationURL
	if err = s.Repository.UpdatePaymentIntent(ctx, org, intent.ID, 1, bson.M{"authorizationUrl": initialized.AuthorizationURL, "version": int64(2), "updatedAt": now}); err != nil {
		return nil, err
	}
	return &PaymentIntentResult{ID: intent.ID, Reference: reference, AuthorizationURL: initialized.AuthorizationURL, State: "pending"}, nil
}

func (s Service) ReceivePaystackWebhook(ctx context.Context, org platform.ID, raw []byte, signature, requestID string) (*WebhookReceipt, error) {
	if len(raw) == 0 || len(raw) > 1<<20 {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_body", Message: "Webhook body is empty or too large."})
	}
	sum := sha256.Sum256(raw)
	bodyHash := hex.EncodeToString(sum[:])
	now := s.now()
	valid := s.Provider != nil && s.Provider.Configured() && s.Provider.VerifyWebhook(raw, signature)
	inbox := ProviderInbox{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: org, Provider: "paystack", BodyHash: bodyHash, SignatureValid: valid, State: "received", ReceivedAt: now}
	if valid {
		inbox.RawBody = append([]byte(nil), raw...)
	} else {
		inbox.State = "rejected"
		inbox.LastErrorCode = "invalid_signature"
	}
	stored, inserted, err := s.Repository.InsertProviderInbox(ctx, inbox)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, &platform.DomainError{Code: "unauthenticated", Message: "Invalid Paystack signature."}
	}
	if !inserted && stored.State == "processed" {
		return &WebhookReceipt{Accepted: true, Duplicate: true, InboxID: stored.ID}, nil
	}
	claimed, err := s.Repository.ClaimProviderInbox(ctx, stored.ID, now)
	if err != nil {
		return nil, err
	}
	if !claimed {
		return &WebhookReceipt{Accepted: true, Duplicate: true, InboxID: stored.ID}, nil
	}
	var event paystackWebhook
	if err = json.Unmarshal(raw, &event); err != nil || strings.TrimSpace(event.Event) == "" {
		s.recordProviderException(ctx, org, stored.ID, "", "malformed_event", "")
		_ = s.Repository.CompleteProviderInbox(ctx, stored.ID, "exception", "malformed_event", "", "", now)
		return &WebhookReceipt{Accepted: true, InboxID: stored.ID}, nil
	}
	event.Event = strings.ToLower(strings.TrimSpace(event.Event))
	reference := providerString(event.Data, "reference", "transaction_reference")
	if reference == "" {
		reference = nestedProviderString(event.Data, "transaction", "reference")
	}
	if err = s.processPaystackEvent(ctx, org, stored.ID, bodyHash, event, reference, requestID); err != nil {
		var domain *platform.DomainError
		if !errors.As(err, &domain) || domain.Code == "provider_unavailable" || domain.Code == "dependency_unavailable" || domain.Code == "version_conflict" || domain.Code == "command_in_progress" {
			next := now.Add(time.Minute)
			code := "dependency_unavailable"
			if domain != nil {
				code = domain.Code
			}
			_ = s.Repository.RetryProviderInbox(ctx, stored.ID, code, next)
			return &WebhookReceipt{Accepted: true, InboxID: stored.ID}, nil
		}
		s.recordProviderException(ctx, org, stored.ID, reference, "processing_failed", "")
		_ = s.Repository.CompleteProviderInbox(ctx, stored.ID, "exception", "processing_failed", event.Event, reference, now)
		return &WebhookReceipt{Accepted: true, InboxID: stored.ID}, nil
	}
	_ = s.Repository.CompleteProviderInbox(ctx, stored.ID, "processed", "", event.Event, reference, now)
	return &WebhookReceipt{Accepted: true, Duplicate: !inserted, InboxID: stored.ID}, nil
}

func (s Service) processPaystackEvent(ctx context.Context, org, inboxID platform.ID, bodyHash string, event paystackWebhook, reference, requestID string) error {
	intent, err := s.Repository.FindPaymentIntentByReference(ctx, org, reference)
	if err != nil {
		return err
	}
	if intent == nil {
		s.recordProviderException(ctx, org, inboxID, reference, "unknown_reference", "")
		return nil
	}
	switch event.Event {
	case "charge.success":
		if intent.ContributionID.Valid() {
			return nil
		}
		verified, e := s.Provider.VerifyPayment(ctx, reference)
		if e != nil {
			return &platform.DomainError{Code: "provider_unavailable", Message: "Paystack verification is temporarily unavailable."}
		}
		if verified.Status != "success" || verified.Reference != intent.Reference || verified.AmountMinor != intent.Total.AmountMinor || strings.ToUpper(verified.Currency) != intent.Total.Currency {
			s.recordProviderException(ctx, org, inboxID, reference, "payment_mismatch", intent.ID)
			return nil
		}
		receivedAt := verified.PaidAt
		if receivedAt.IsZero() {
			receivedAt = s.now()
		}
		principal := providerSystemPrincipal(org, intent.BranchID)
		contribution, e := s.PostContribution(ctx, principal, ContributionInput{ReceivedAt: receivedAt, BranchID: intent.BranchID, Donor: intent.Donor, Source: "online", PaymentMethodID: intent.PaymentMethodID, PaymentMethodReference: verified.TransactionID, ProviderReference: intent.Reference, CampaignID: intent.CampaignID, PledgeID: intent.PledgeID, Total: intent.Total, Splits: intent.Splits, PostingAction: "post", Provenance: "paystack-webhook"}, requestID, "paystack-charge:"+intent.Reference)
		if e != nil {
			return e
		}
		if intent.SavePaymentMethod && intent.Donor.Type == "person" && verified.Authorization != nil && verified.Authorization.Reusable {
			if captureErr := s.captureMemberPaymentMethod(ctx, intent, *verified.Authorization, requestID); captureErr != nil {
				s.recordProviderException(ctx, org, inboxID, reference, "authorization_capture_failed", intent.ID)
			}
		}
		if e = s.Repository.AdvancePaymentIntent(ctx, org, intent.ID, bson.M{"state": "success", "providerStatus": "success", "providerTransactionId": verified.TransactionID, "channel": verified.Channel, "settlementReference": verified.SettlementReference, "contributionId": contribution.ID, "lastProviderEventAt": s.now(), "failureCode": ""}); e != nil {
			return e
		}
		if intent.RecurringInstructionID.Valid() {
			_, e = s.Repository.database.Collection(recurringInstructionsCollection).UpdateOne(ctx, bson.M{"_id": intent.RecurringInstructionID, "organizationId": org, "lastIntentId": intent.ID, "state": "needs-action"}, bson.M{"$set": bson.M{"state": "active", "actionUrl": "", "failureCount": 0, "lastFailureCode": "", "updatedAt": s.now()}, "$inc": bson.M{"version": 1}})
		}
		return e
	case "charge.failed", "bank.transfer.rejected":
		if intent.State == "success" || intent.ContributionID.Valid() {
			return nil
		}
		if e := s.Repository.AdvancePaymentIntent(ctx, org, intent.ID, bson.M{"state": "failed", "providerStatus": providerString(event.Data, "status"), "failureCode": event.Event, "lastProviderEventAt": s.now()}); e != nil {
			return e
		}
		var recurringErr error
		if intent.RecurringInstructionID.Valid() {
			_, recurringErr = s.Repository.database.Collection(recurringInstructionsCollection).UpdateOne(ctx, bson.M{"_id": intent.RecurringInstructionID, "organizationId": org}, bson.M{"$set": bson.M{"state": "paused", "lastFailureCode": event.Event, "actionUrl": "", "updatedAt": s.now()}, "$inc": bson.M{"failureCount": 1, "version": 1}})
		}
		return recurringErr
	case "refund.pending", "refund.processing", "refund.needs-attention", "refund.failed":
		return s.Repository.AdvancePaymentIntentRefundState(ctx, org, intent.ID, strings.TrimPrefix(event.Event, "refund."), s.now())
	case "refund.processed":
		if !intent.ContributionID.Valid() {
			return &platform.DomainError{Code: "dependency_unavailable", Message: "The successful charge has not been posted yet."}
		}
		amount, e := providerInt64(event.Data, "amount")
		if e != nil || amount <= 0 || strings.ToUpper(providerString(event.Data, "currency")) != intent.Total.Currency {
			s.recordProviderException(ctx, org, inboxID, reference, "refund_mismatch", intent.ID)
			return nil
		}
		providerEventReference := providerString(event.Data, "refund_reference")
		if providerEventReference == "" {
			providerEventReference = "paystack-refund:" + bodyHash
		}
		_, e = s.AdjustContribution(ctx, providerSystemPrincipal(org, intent.BranchID), intent.ContributionID, AdjustmentInput{Type: "provider-refund", Reason: "Processed Paystack refund", AmountMinor: amount, ProviderEventReference: providerEventReference}, requestID, "paystack-refund:"+providerEventReference)
		if e != nil {
			return e
		}
		return s.Repository.ApplyPaymentIntentRefund(ctx, org, intent.ID, providerEventReference, amount, s.now())
	case "charge.dispute.create", "charge.dispute.remind":
		if strings.HasPrefix(intent.ChargebackState, "resolved-") {
			return nil
		}
		s.recordProviderException(ctx, org, inboxID, reference, "chargeback_open", intent.ID)
		return s.Repository.AdvancePaymentIntent(ctx, org, intent.ID, bson.M{"chargebackState": "open", "lastProviderEventAt": s.now()})
	case "charge.dispute.resolve":
		resolution := strings.ToLower(providerString(event.Data, "resolution", "status"))
		refundAmount, _ := providerInt64(event.Data, "refund_amount", "amount")
		if refundAmount > 0 && intent.ContributionID.Valid() {
			eventRef := "paystack-chargeback:" + bodyHash
			if _, e := s.AdjustContribution(ctx, providerSystemPrincipal(org, intent.BranchID), intent.ContributionID, AdjustmentInput{Type: "provider-chargeback", Reason: "Resolved Paystack chargeback", AmountMinor: refundAmount, ProviderEventReference: eventRef}, requestID, eventRef); e != nil {
				return e
			}
			return s.Repository.AdvancePaymentIntent(ctx, org, intent.ID, bson.M{"state": "chargeback", "chargebackState": "resolved-debit", "lastProviderEventAt": s.now()})
		}
		return s.Repository.AdvancePaymentIntent(ctx, org, intent.ID, bson.M{"chargebackState": "resolved-" + resolution, "lastProviderEventAt": s.now()})
	default:
		return nil
	}
}

func (s Service) recordProviderException(ctx context.Context, org, inboxID platform.ID, reference, code string, intentID platform.ID) {
	_ = s.Repository.InsertProviderException(ctx, ProviderException{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: org, Provider: "paystack", InboxID: inboxID, IntentID: intentID, Reference: reference, Code: code, State: "open", CreatedAt: s.now()})
}

func providerString(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := data[key]; ok && value != nil {
			text := strings.TrimSpace(fmt.Sprint(value))
			if text != "" && text != "<nil>" {
				return text
			}
		}
	}
	return ""
}

func nestedProviderString(data map[string]any, parent, key string) string {
	value, ok := data[parent].(map[string]any)
	if !ok {
		return ""
	}
	return providerString(value, key)
}

func providerInt64(data map[string]any, keys ...string) (int64, error) {
	for _, key := range keys {
		value, ok := data[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case float64:
			if typed != float64(int64(typed)) {
				return 0, fmt.Errorf("%s must be an integer", key)
			}
			return int64(typed), nil
		case json.Number:
			return typed.Int64()
		case string:
			return strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		}
	}
	return 0, fmt.Errorf("amount is missing")
}

func (s Service) ListPaymentIntents(ctx context.Context, p platform.Principal, branch platform.ID, limit int64) ([]PaymentIntent, error) {
	if !branch.Valid() {
		return nil, platform.ValidationError(platform.FieldError{Path: "branchId", Code: "required", Message: "A branch scope is required."})
	}
	if !s.allowedLedger(p, "read", branch, false) {
		return nil, ledgerDenied()
	}
	return s.Repository.ListPaymentIntents(ctx, p.OrganizationID, branch, limit)
}

func (s Service) ListProviderExceptions(ctx context.Context, p platform.Principal, limit int64) ([]ProviderException, error) {
	if !s.allowedLedger(p, "read", "", false) {
		return nil, ledgerDenied()
	}
	return s.Repository.ListProviderExceptions(ctx, p.OrganizationID, limit)
}
