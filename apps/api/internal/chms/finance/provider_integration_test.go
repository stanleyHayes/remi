package finance

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"remi-api/internal/chms/platform"
)

type fakePaymentProvider struct {
	secret        string
	mu            sync.Mutex
	verifications map[string]ProviderVerification
	verifyErr     error
	charges       []fakeAuthorizationCharge
}

type fakeAuthorizationCharge struct {
	AuthorizationCode string
	Email             string
	Reference         string
	AmountMinor       int64
}

func (p *fakePaymentProvider) ChargeAuthorization(_ context.Context, code, email string, amount int64, _ string, reference string, _ map[string]any) (*ProviderChargeResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.charges = append(p.charges, fakeAuthorizationCharge{AuthorizationCode: code, Email: email, Reference: reference, AmountMinor: amount})
	return &ProviderChargeResult{Reference: reference, Status: "success"}, nil
}

func (p *fakePaymentProvider) Configured() bool { return p.secret != "" }
func (p *fakePaymentProvider) VerifyWebhook(raw []byte, signature string) bool {
	hash := hmac.New(sha512.New, []byte(p.secret))
	_, _ = hash.Write(raw)
	return hmac.Equal([]byte(hex.EncodeToString(hash.Sum(nil))), []byte(signature))
}
func (p *fakePaymentProvider) InitializePayment(_ context.Context, input ProviderInitializeRequest) (*ProviderInitializeResult, error) {
	return &ProviderInitializeResult{AuthorizationURL: "https://checkout.test/" + input.Reference, Reference: input.Reference}, nil
}
func (p *fakePaymentProvider) VerifyPayment(_ context.Context, reference string) (*ProviderVerification, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.verifyErr != nil {
		return nil, p.verifyErr
	}
	value, ok := p.verifications[reference]
	if !ok {
		return nil, fmt.Errorf("transaction not found")
	}
	return &value, nil
}
func (p *fakePaymentProvider) set(reference string, value ProviderVerification) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.verifications[reference] = value
}
func (p *fakePaymentProvider) sign(raw []byte) string {
	hash := hmac.New(sha512.New, []byte(p.secret))
	_, _ = hash.Write(raw)
	return hex.EncodeToString(hash.Sum(nil))
}

func configureOnlineGiving(t *testing.T, ctx context.Context, service Service, admin platform.Principal) (*PaymentMethod, *Fund) {
	t.Helper()
	start, end := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := service.SaveCampus(ctx, admin, "", CampusSettingsInput{BranchID: "accra", Name: "Accra Campus", Timezone: "Africa/Accra", Currency: "GHS"}, "provider-campus"); err != nil {
		t.Fatal(err)
	}
	method, err := service.SavePaymentMethod(ctx, admin, "", PaymentMethodInput{Code: "PAYSTACK", Name: "Paystack online", Kind: "card", Provider: "Paystack", Active: true}, "provider-method")
	if err != nil {
		t.Fatal(err)
	}
	fund, err := service.SaveFund(ctx, admin, "", FundInput{Code: "OFFERING", Name: "Offering", RestrictionType: "unrestricted", ActiveFrom: start}, "provider-fund")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.SavePeriod(ctx, admin, "", FiscalPeriodInput{Code: "FY2026", Name: "2026 fiscal year", StartsAt: start, EndsAt: end}, "provider-period"); err != nil {
		t.Fatal(err)
	}
	if _, err = service.SaveSequence(ctx, admin, "", ReceiptSequenceInput{Code: "MAIN2026", Prefix: "REMI", FiscalYear: 2026, Padding: 6, StartingNumber: 1}, "provider-sequence"); err != nil {
		t.Fatal(err)
	}
	return method, fund
}

func webhookBody(t *testing.T, event string, data map[string]any) []byte {
	t.Helper()
	value, err := json.Marshal(map[string]any{"event": event, "data": data})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestPaystackIntentWebhookDuplicateReorderRefundAndChargebackLifecycle(t *testing.T) {
	ctx, repo, service, db, admin := financeTest(t)
	provider := &fakePaymentProvider{secret: "paystack-test-secret", verifications: map[string]ProviderVerification{}}
	service.Provider = provider
	method, fund := configureOnlineGiving(t, ctx, service, admin)

	create := func(key string, amount int64) *PaymentIntentResult {
		value, err := service.CreatePaystackIntent(ctx, "remi", PaymentIntentInput{Email: "giver@example.com", AmountMinor: amount, Category: "Offering"}, "intent-"+key, key)
		if err != nil {
			t.Fatalf("create intent %s: %v", key, err)
		}
		return value
	}
	intent := create("checkout-key-0001", 10_000)
	replay, err := service.CreatePaystackIntent(ctx, "remi", PaymentIntentInput{Email: "giver@example.com", AmountMinor: 10_000, Category: "Offering"}, "intent-replay", "checkout-key-0001")
	if err != nil || replay.ID != intent.ID || replay.AuthorizationURL != intent.AuthorizationURL {
		t.Fatalf("intent replay=%+v err=%v", replay, err)
	}
	if _, err = service.CreatePaystackIntent(ctx, "remi", PaymentIntentInput{Email: "giver@example.com", AmountMinor: 11_000, Category: "Offering"}, "intent-reuse", "checkout-key-0001"); err == nil {
		t.Fatal("changed checkout reused an idempotency key")
	}
	provider.set(intent.Reference, ProviderVerification{Reference: intent.Reference, Status: "success", AmountMinor: 10_000, Currency: "GHS", Channel: "card", TransactionID: "9000000001", SettlementReference: "settlement-1", PaidAt: time.Date(2026, 8, 11, 11, 30, 0, 0, time.UTC)})
	charge := webhookBody(t, "charge.success", map[string]any{"reference": intent.Reference, "amount": 10_000, "currency": "GHS"})
	if _, err = service.ReceivePaystackWebhook(ctx, "remi", charge, "bad-signature", "bad-signature"); err == nil {
		t.Fatal("invalid webhook signature was accepted")
	}
	var wg sync.WaitGroup
	wg.Add(12)
	errs := make(chan error, 12)
	for range 12 {
		go func() {
			defer wg.Done()
			_, receiveErr := service.ReceivePaystackWebhook(context.Background(), "remi", charge, provider.sign(charge), "charge-success")
			errs <- receiveErr
		}()
	}
	wg.Wait()
	close(errs)
	for receiveErr := range errs {
		if receiveErr != nil {
			t.Fatalf("concurrent charge webhook: %v", receiveErr)
		}
	}
	stored, err := repo.FindPaymentIntentByReference(ctx, "remi", intent.Reference)
	if err != nil || stored.State != "success" || !stored.ContributionID.Valid() || stored.SettlementReference != "settlement-1" {
		t.Fatalf("stored success=%+v err=%v", stored, err)
	}
	contributions, _ := repo.ListContributions(ctx, "remi", "accra", 50)
	if len(contributions) != 1 || contributions[0].ProviderReference != intent.Reference || contributions[0].ReceiptNumber != "REMI-2026-000001" {
		t.Fatalf("success contributions=%+v", contributions)
	}
	failedAfterSuccess := webhookBody(t, "charge.failed", map[string]any{"reference": intent.Reference, "status": "failed"})
	if _, err = service.ReceivePaystackWebhook(ctx, "remi", failedAfterSuccess, provider.sign(failedAfterSuccess), "late-failure"); err != nil {
		t.Fatal(err)
	}
	stored, _ = repo.FindPaymentIntentByReference(ctx, "remi", intent.Reference)
	if stored.State != "success" {
		t.Fatalf("late failure downgraded success: %+v", stored)
	}

	refunded := create("checkout-key-0002", 12_000)
	provider.set(refunded.Reference, ProviderVerification{Reference: refunded.Reference, Status: "success", AmountMinor: 12_000, Currency: "GHS", Channel: "mobile_money", TransactionID: "9000000002", PaidAt: time.Date(2026, 8, 11, 11, 40, 0, 0, time.UTC)})
	earlyRefund := webhookBody(t, "refund.processed", map[string]any{"transaction_reference": refunded.Reference, "refund_reference": "refund-early", "amount": "5000", "currency": "GHS"})
	if _, err = service.ReceivePaystackWebhook(ctx, "remi", earlyRefund, provider.sign(earlyRefund), "early-refund"); err != nil {
		t.Fatal(err)
	}
	charge2 := webhookBody(t, "charge.success", map[string]any{"reference": refunded.Reference, "amount": 12_000, "currency": "GHS"})
	if _, err = service.ReceivePaystackWebhook(ctx, "remi", charge2, provider.sign(charge2), "charge-two"); err != nil {
		t.Fatal(err)
	}
	if _, err = service.ReceivePaystackWebhook(ctx, "remi", earlyRefund, provider.sign(earlyRefund), "early-refund-replay"); err != nil {
		t.Fatal(err)
	}
	// Simulate a worker crash after the refund projection was updated but before
	// the inbox was acknowledged. Reclaiming the event must not count it twice.
	_, err = db.Collection(providerInboxCollection).UpdateOne(ctx,
		bson.M{"reference": refunded.Reference, "eventType": "refund.processed"},
		bson.M{"$set": bson.M{"state": "retry", "retryAt": time.Now().Add(-time.Minute)}, "$unset": bson.M{"processedAt": ""}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ReceivePaystackWebhook(ctx, "remi", earlyRefund, provider.sign(earlyRefund), "early-refund-recovery"); err != nil {
		t.Fatal(err)
	}
	stored, _ = repo.FindPaymentIntentByReference(ctx, "remi", refunded.Reference)
	if stored.RefundedAmountMinor != 5_000 {
		t.Fatalf("recovered refund counted twice: %+v", stored)
	}
	pendingLate := webhookBody(t, "refund.pending", map[string]any{"transaction_reference": refunded.Reference, "amount": "5000", "currency": "GHS"})
	if _, err = service.ReceivePaystackWebhook(ctx, "remi", pendingLate, provider.sign(pendingLate), "late-pending"); err != nil {
		t.Fatal(err)
	}
	finalRefund := webhookBody(t, "refund.processed", map[string]any{"transaction_reference": refunded.Reference, "refund_reference": "refund-final", "amount": "7000", "currency": "GHS"})
	if _, err = service.ReceivePaystackWebhook(ctx, "remi", finalRefund, provider.sign(finalRefund), "final-refund"); err != nil {
		t.Fatal(err)
	}
	stored, _ = repo.FindPaymentIntentByReference(ctx, "remi", refunded.Reference)
	if stored.State != "refunded" || stored.RefundState != "processed" || stored.RefundedAmountMinor != 12_000 {
		t.Fatalf("refund state=%+v", stored)
	}
	adjusted, _ := repo.SumProviderAdjustments(ctx, "remi", stored.ContributionID)
	if adjusted != -12_000 {
		t.Fatalf("provider adjustments=%d want -12000", adjusted)
	}

	chargeback := create("checkout-key-0003", 8_000)
	provider.set(chargeback.Reference, ProviderVerification{Reference: chargeback.Reference, Status: "success", AmountMinor: 8_000, Currency: "GHS", Channel: "card", TransactionID: "9000000003", PaidAt: time.Date(2026, 8, 11, 11, 45, 0, 0, time.UTC)})
	charge3 := webhookBody(t, "charge.success", map[string]any{"reference": chargeback.Reference, "amount": 8_000, "currency": "GHS"})
	_, _ = service.ReceivePaystackWebhook(ctx, "remi", charge3, provider.sign(charge3), "charge-three")
	dispute := webhookBody(t, "charge.dispute.create", map[string]any{"transaction": map[string]any{"reference": chargeback.Reference}, "amount": 8_000, "currency": "GHS"})
	_, _ = service.ReceivePaystackWebhook(ctx, "remi", dispute, provider.sign(dispute), "dispute-open")
	resolved := webhookBody(t, "charge.dispute.resolve", map[string]any{"transaction": map[string]any{"reference": chargeback.Reference}, "resolution": "merchant-accepted", "refund_amount": 8_000, "currency": "GHS"})
	_, _ = service.ReceivePaystackWebhook(ctx, "remi", resolved, provider.sign(resolved), "dispute-resolve")
	stored, _ = repo.FindPaymentIntentByReference(ctx, "remi", chargeback.Reference)
	if stored.State != "chargeback" || stored.ChargebackState != "resolved-debit" {
		t.Fatalf("chargeback state=%+v", stored)
	}

	mismatch := create("checkout-key-0004", 9_000)
	provider.set(mismatch.Reference, ProviderVerification{Reference: mismatch.Reference, Status: "success", AmountMinor: 9_001, Currency: "GHS", TransactionID: "9000000004", PaidAt: time.Now().UTC()})
	mismatchBody := webhookBody(t, "charge.success", map[string]any{"reference": mismatch.Reference, "amount": 9_000, "currency": "GHS"})
	_, _ = service.ReceivePaystackWebhook(ctx, "remi", mismatchBody, provider.sign(mismatchBody), "mismatch")
	mismatchStored, _ := repo.FindPaymentIntentByReference(ctx, "remi", mismatch.Reference)
	if mismatchStored.ContributionID.Valid() || mismatchStored.State != "pending" {
		t.Fatalf("mismatch posted a contribution: %+v", mismatchStored)
	}
	exceptions, _ := repo.ListProviderExceptions(ctx, "remi", 50)
	if len(exceptions) < 2 {
		t.Fatalf("exceptions=%+v", exceptions)
	}
	validInbox, _ := db.Collection(providerInboxCollection).CountDocuments(ctx, bson.M{"signatureValid": true})
	invalidInbox, _ := db.Collection(providerInboxCollection).CountDocuments(ctx, bson.M{"signatureValid": false})
	if validInbox < 10 || invalidInbox != 1 {
		t.Fatalf("inbox valid=%d invalid=%d", validInbox, invalidInbox)
	}
	var processedInbox bson.M
	if err = db.Collection(providerInboxCollection).FindOne(ctx, bson.M{"signatureValid": true, "state": "processed"}).Decode(&processedInbox); err != nil || len(processedInbox["rawBody"].(bson.Binary).Data) == 0 {
		t.Fatalf("durable raw inbox missing: %v %v", processedInbox, err)
	}
	_ = method
	_ = fund
}

func TestMemberSavedMethodAndRecurringChargeLifecycle(t *testing.T) {
	ctx, repo, service, db, admin := financeTest(t)
	provider := &fakePaymentProvider{secret: "member-recurring-secret", verifications: map[string]ProviderVerification{}}
	service.Provider = provider
	key := []byte("0123456789abcdef0123456789abcdef")
	cipher, err := platform.NewEnvelopeCipher("member-method-test-v1", key)
	if err != nil {
		t.Fatal(err)
	}
	service.Cipher = cipher
	_, fund := configureOnlineGiving(t, ctx, service, admin)
	if _, err = db.Collection("chms_people").InsertOne(ctx, bson.M{"_id": "person-member", "organizationId": "remi", "homeBranchId": "accra", "archivedAt": nil}); err != nil {
		t.Fatal(err)
	}

	intent, err := service.CreateMemberPaystackIntent(ctx, "remi", "person-member", PaymentIntentInput{Email: "member@example.com", AmountMinor: 12_500, FundID: fund.ID, BranchID: "accra", Donor: DonorAttribution{Type: "person", PersonID: "person-member"}, SavePaymentMethod: true}, "member-gift", "member-gift-1")
	if err != nil {
		t.Fatal(err)
	}
	provider.set(intent.Reference, ProviderVerification{Reference: intent.Reference, Status: "success", AmountMinor: 12_500, Currency: "GHS", Channel: "card", TransactionID: "member-transaction", PaidAt: time.Date(2026, 8, 11, 11, 30, 0, 0, time.UTC), Authorization: &ProviderAuthorization{Code: "AUTH_private_token", Signature: "SIG_member_card", Reusable: true, Channel: "card", Brand: "visa", Last4: "4081", ExpiryMonth: "12", ExpiryYear: "2030"}})
	body := webhookBody(t, "charge.success", map[string]any{"reference": intent.Reference, "amount": 12_500, "currency": "GHS"})
	_ = body
	if err = service.processPaystackEvent(ctx, "remi", "test-inbox", "test-hash", paystackWebhook{Event: "charge.success", Data: map[string]any{"reference": intent.Reference}}, intent.Reference, "member-gift-success"); err != nil {
		t.Fatalf("process member gift: %#v", err)
	}
	methods, err := service.ListMemberPaymentMethods(ctx, "remi", "person-member")
	if err != nil || len(methods) != 1 || methods[0].Last4 != "4081" || methods[0].EncryptedAuthorization.Ciphertext != "" {
		exceptions, _ := repo.ListProviderExceptions(ctx, "remi", 20)
		stored, _ := repo.FindPaymentIntentByReference(ctx, "remi", intent.Reference)
		t.Fatalf("safe methods=%+v err=%v intent=%+v exceptions=%+v", methods, err, stored, exceptions)
	}
	if other, _ := service.ListMemberPaymentMethods(ctx, "remi", "person-other"); len(other) != 0 {
		t.Fatalf("cross-member methods leaked: %+v", other)
	}

	instruction, err := service.CreateRecurringInstruction(ctx, "remi", "person-member", RecurringInstructionInput{BranchID: "accra", Donor: DonorAttribution{Type: "person", PersonID: "person-member"}, PaymentMethodID: methods[0].ID, FundID: fund.ID, AmountMinor: 5_000, Currency: "GHS", Frequency: "monthly", StartsAt: time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)}, "recurring-create")
	if err != nil {
		t.Fatal(err)
	}
	// Creation deliberately moves past starts to tomorrow; make it due to exercise the worker.
	_, err = repo.database.Collection(recurringInstructionsCollection).UpdateOne(ctx, bson.M{"_id": instruction.ID}, bson.M{"$set": bson.M{"nextChargeAt": time.Date(2026, 8, 11, 11, 0, 0, 0, time.UTC)}})
	if err != nil {
		t.Fatal(err)
	}
	processed, err := service.RunDueRecurring(ctx)
	if err != nil || processed != 1 {
		t.Fatalf("processed=%d err=%v", processed, err)
	}
	provider.mu.Lock()
	charges := append([]fakeAuthorizationCharge(nil), provider.charges...)
	provider.mu.Unlock()
	if len(charges) != 1 || charges[0].AuthorizationCode != "AUTH_private_token" || charges[0].Email != "member@example.com" {
		t.Fatalf("charges=%+v", charges)
	}
	if err = service.DisableMemberPaymentMethod(ctx, "remi", "person-member", methods[0].ID, "method-disable"); err != nil {
		t.Fatal(err)
	}
	instructions, _ := service.ListRecurringInstructions(ctx, "remi", "person-member")
	if len(instructions) != 1 || instructions[0].State != "cancelled" {
		t.Fatalf("instructions=%+v", instructions)
	}
}

func TestFundraisingCampaignPublicLifecycleAndPaymentAttribution(t *testing.T) {
	ctx, repo, service, _, admin := financeTest(t)
	provider := &fakePaymentProvider{secret: "campaign-provider-secret", verifications: map[string]ProviderVerification{}}
	service.Provider = provider
	_, fund := configureOnlineGiving(t, ctx, service, admin)
	campaign, err := service.SaveCampaign(ctx, admin, "", CampaignInput{Slug: "community-kitchen", Title: "Build the community kitchen", Summary: "Help us create a dignified community kitchen for families.", Story: "This shared kitchen will provide meals, training and practical care to families across our neighbourhood.", FundID: fund.ID, Goal: platform.Money{AmountMinor: 100_000, Currency: "GHS"}, StartsAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), EndsAt: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC), Status: "published", Featured: true}, "campaign-create")
	if err != nil {
		t.Fatal(err)
	}
	public, err := service.GetPublicCampaign(ctx, "remi", campaign.Slug)
	if err != nil || public.RaisedAmountMinor != 0 {
		t.Fatalf("public campaign=%+v err=%v", public, err)
	}
	intent, err := service.CreatePaystackIntent(ctx, "remi", PaymentIntentInput{Email: "campaign.giver@example.com", AmountMinor: 25_000, CampaignID: campaign.ID, Category: campaign.Title}, "campaign-intent", "campaign-checkout-0001")
	if err != nil {
		t.Fatal(err)
	}
	provider.set(intent.Reference, ProviderVerification{Reference: intent.Reference, Status: "success", AmountMinor: 25_000, Currency: "GHS", Channel: "card", TransactionID: "campaign-transaction-1", PaidAt: time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)})
	body := webhookBody(t, "charge.success", map[string]any{"reference": intent.Reference, "amount": 25_000, "currency": "GHS"})
	if _, err = service.ReceivePaystackWebhook(ctx, "remi", body, provider.sign(body), "campaign-success"); err != nil {
		t.Fatal(err)
	}
	public, err = service.GetPublicCampaign(ctx, "remi", campaign.Slug)
	if err != nil || public.RaisedAmountMinor != 25_000 || public.GiftCount != 1 {
		t.Fatalf("campaign totals=%+v err=%v", public, err)
	}
	stored, _ := repo.FindPaymentIntentByReference(ctx, "remi", intent.Reference)
	contribution, _ := repo.FindContribution(ctx, "remi", stored.ContributionID)
	if contribution == nil || contribution.CampaignID != campaign.ID {
		t.Fatalf("campaign attribution missing: %+v", contribution)
	}
}

func TestPaymentIntentDemoFallbackIsExplicitAndFailsClosedOtherwise(t *testing.T) {
	ctx, repo, service, _, admin := financeTest(t)
	configureOnlineGiving(t, ctx, service, admin)

	_, err := service.CreatePaystackIntent(ctx, "remi", PaymentIntentInput{Email: "giver@example.com", AmountMinor: 10_000, Category: "Offering"}, "provider-disabled", "checkout-provider-disabled")
	var domain *platform.DomainError
	if !errors.As(err, &domain) || domain.Code != "provider_unavailable" {
		t.Fatalf("unconfigured production-style service error=%v", err)
	}
	failed, findErr := repo.FindPaymentIntentByClientKey(ctx, "remi", "checkout-provider-disabled")
	if findErr != nil || failed == nil || failed.State != "failed" || failed.FailureCode != "provider_not_configured" {
		t.Fatalf("failed intent=%+v err=%v", failed, findErr)
	}

	service.AllowProviderDemo = true
	demo, err := service.CreatePaystackIntent(ctx, "remi", PaymentIntentInput{Email: "demo@example.com", AmountMinor: 10_000, Category: "Offering"}, "provider-demo", "checkout-provider-demo")
	if err != nil || !demo.Demo || !strings.HasPrefix(demo.AuthorizationURL, "/give/demo-success?") {
		t.Fatalf("explicit demo=%+v err=%v", demo, err)
	}
}

func TestPledgeAttributionProgressRefundAndConsentAwareReminder(t *testing.T) {
	ctx, repo, service, database, admin := financeTest(t)
	provider := &fakePaymentProvider{secret: "pledge-provider-secret", verifications: map[string]ProviderVerification{}}
	service.Provider = provider
	service.ReminderEligibility = func(_ context.Context, _ platform.Principal, _, _ platform.ID, purpose, channel string) (bool, string, error) {
		return purpose == "giving-communications" && channel == "email", "eligible", nil
	}
	_, fund := configureOnlineGiving(t, ctx, service, admin)
	_, _ = database.Collection("chms_people").InsertOne(ctx, bson.M{"_id": "pledge-person", "organizationId": "remi", "homeBranchId": "accra", "archivedAt": nil})
	campaign, err := service.SaveCampaign(ctx, admin, "", CampaignInput{Slug: "youth-centre", Title: "Build the youth centre", Summary: "Create a welcoming place for young people to grow.", Story: "This centre will provide safe rooms for mentoring, worship, learning and practical community care.", FundID: fund.ID, Goal: platform.Money{AmountMinor: 500_000, Currency: "GHS"}, StartsAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), EndsAt: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC), Status: "published"}, "pledge-campaign")
	if err != nil {
		t.Fatal(err)
	}
	donor := DonorAttribution{Type: "person", PersonID: "pledge-person"}
	pledge, err := service.SavePledge(ctx, admin, "", PledgeInput{BranchID: "accra", Donor: donor, FundID: fund.ID, CampaignID: campaign.ID, Target: platform.Money{AmountMinor: 100_000, Currency: "GHS"}, Schedule: PledgeSchedule{Frequency: "monthly", InstallmentAmount: platform.Money{AmountMinor: 25_000, Currency: "GHS"}, StartsAt: time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)}, Reminder: PledgeReminderPreference{OptedIn: true, Channels: []string{"email"}, Cadence: "monthly", RecipientPersonID: "pledge-person"}, State: "active"}, "pledge-create")
	if err != nil {
		t.Fatal(err)
	}
	intent, err := service.CreatePaystackIntent(ctx, "remi", PaymentIntentInput{Email: "pledger@example.com", AmountMinor: 25_000, CampaignID: campaign.ID, PledgeID: pledge.ID, Donor: donor}, "pledge-intent", "pledge-checkout-0001")
	if err != nil {
		t.Fatal(err)
	}
	provider.set(intent.Reference, ProviderVerification{Reference: intent.Reference, Status: "success", AmountMinor: 25_000, Currency: "GHS", Channel: "card", TransactionID: "pledge-transaction", PaidAt: time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)})
	charge := webhookBody(t, "charge.success", map[string]any{"reference": intent.Reference, "amount": 25_000, "currency": "GHS"})
	if _, err = service.ReceivePaystackWebhook(ctx, "remi", charge, provider.sign(charge), "pledge-charge"); err != nil {
		t.Fatal(err)
	}
	pledge, err = repo.FindPledge(ctx, "remi", pledge.ID)
	if err != nil || pledge.FulfilledAmountMinor != 25_000 || pledge.RemainingAmountMinor != 75_000 {
		t.Fatalf("pledge after charge=%+v err=%v", pledge, err)
	}
	refund := webhookBody(t, "refund.processed", map[string]any{"transaction_reference": intent.Reference, "refund_reference": "pledge-refund", "amount": "5000", "currency": "GHS"})
	if _, err = service.ReceivePaystackWebhook(ctx, "remi", refund, provider.sign(refund), "pledge-refund"); err != nil {
		t.Fatal(err)
	}
	pledge, err = repo.FindPledge(ctx, "remi", pledge.ID)
	if err != nil || pledge.FulfilledAmountMinor != 20_000 || pledge.RemainingAmountMinor != 80_000 {
		t.Fatalf("pledge after refund=%+v err=%v", pledge, err)
	}
	decision, err := service.SendPledgeReminder(ctx, admin, pledge.ID, PledgeReminderInput{Channel: "email"}, "pledge-reminder-1")
	if err != nil || decision.Decision != "queued" {
		t.Fatalf("reminder=%+v err=%v", decision, err)
	}
	decision, err = service.SendPledgeReminder(ctx, admin, pledge.ID, PledgeReminderInput{Channel: "email"}, "pledge-reminder-2")
	if err != nil || decision.Decision != "suppressed" || decision.Reason != "frequency-cap" {
		t.Fatalf("frequency cap=%+v err=%v", decision, err)
	}
	member := platform.Principal{Actor: platform.Actor{Type: platform.ActorMember, ID: "pledge-person"}, OrganizationID: "remi", Roles: []string{"member"}}
	memberItems, err := service.ListPledges(ctx, member)
	if err != nil || len(memberItems) != 1 || memberItems[0].ID != pledge.ID {
		t.Fatalf("member pledges=%+v err=%v", memberItems, err)
	}
	memberInput := PledgeInput{BranchID: pledge.BranchID, Donor: pledge.Donor, FundID: pledge.FundID, CampaignID: pledge.CampaignID, Target: pledge.Target, Schedule: pledge.Schedule, Reminder: pledge.Reminder, State: "paused", ExpectedVersion: pledge.Version}
	paused, err := service.SavePledge(ctx, member, pledge.ID, memberInput, "member-pause")
	if err != nil || paused.State != "paused" {
		t.Fatalf("member pause=%+v err=%v", paused, err)
	}
	other := platform.Principal{Actor: platform.Actor{Type: platform.ActorMember, ID: "other-person"}, OrganizationID: "remi", Roles: []string{"member"}}
	if _, err = service.GetPledge(ctx, other, pledge.ID); err == nil {
		t.Fatal("another member read a private pledge")
	}
}
