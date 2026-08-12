package httpapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"remi-api/internal/chms/finance"
	"remi-api/internal/chms/platform"
	"remi-api/internal/middleware"
	"remi-api/internal/services"
	"remi-api/internal/testsupport"
)

type httpPaymentProvider struct {
	secret       string
	verification finance.ProviderVerification
}

func (p *httpPaymentProvider) Configured() bool { return true }
func (p *httpPaymentProvider) VerifyWebhook(raw []byte, signature string) bool {
	hash := hmac.New(sha512.New, []byte(p.secret))
	_, _ = hash.Write(raw)
	return hmac.Equal([]byte(hex.EncodeToString(hash.Sum(nil))), []byte(signature))
}
func (p *httpPaymentProvider) InitializePayment(_ context.Context, input finance.ProviderInitializeRequest) (*finance.ProviderInitializeResult, error) {
	return &finance.ProviderInitializeResult{AuthorizationURL: "https://checkout.test/" + input.Reference, Reference: input.Reference}, nil
}
func (p *httpPaymentProvider) VerifyPayment(_ context.Context, reference string) (*finance.ProviderVerification, error) {
	value := p.verification
	value.Reference = reference
	return &value, nil
}
func (p *httpPaymentProvider) sign(raw []byte) string {
	hash := hmac.New(sha512.New, []byte(p.secret))
	_, _ = hash.Write(raw)
	return hex.EncodeToString(hash.Sum(nil))
}

func TestPaystackPublicHTTPAndScopedFinanceInspection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(testsupport.MongoURI()))
	if err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	if err = client.Ping(ctx, nil); err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	db := client.Database("remi_provider_http_" + bson.NewObjectID().Hex())
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_ = db.Drop(cleanup)
		_ = client.Disconnect(cleanup)
	})
	repository, _ := finance.NewRepository(db)
	if err = repository.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	store, _ := platform.NewMongoPlatformStore(db)
	if err = store.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	provider := &httpPaymentProvider{secret: "http-paystack-secret", verification: finance.ProviderVerification{Status: "success", AmountMinor: 15_000, Currency: "GHS", Channel: "card", TransactionID: "888001", SettlementReference: "settlement-http", PaidAt: now}}
	authorizer := platform.GrantAuthorizer{}
	financeService := finance.Service{Repository: repository, Platform: store, Authorizer: authorizer, Provider: provider, Now: func() time.Time { return now }}
	mfa := now.Add(-time.Minute)
	admin := platform.Principal{Actor: platform.Actor{Type: platform.ActorStaff, ID: "finance-admin"}, OrganizationID: "org-http", Roles: []string{"super-admin"}, MFAConfirmedAt: &mfa, Grants: []platform.Grant{{Action: "*", Resource: "finance-config", BranchIDs: []platform.ID{"*"}, FieldClasses: []platform.FieldClass{platform.FieldFinancial}}}}
	if _, err = financeService.SaveCampus(ctx, admin, "", finance.CampusSettingsInput{BranchID: "accra", Name: "Accra Campus", Timezone: "Africa/Accra", Currency: "GHS"}, "campus"); err != nil {
		t.Fatal(err)
	}
	if _, err = financeService.SavePaymentMethod(ctx, admin, "", finance.PaymentMethodInput{Code: "PAYSTACK", Name: "Paystack online", Kind: "card", Provider: "Paystack", Active: true}, "method"); err != nil {
		t.Fatal(err)
	}
	if _, err = financeService.SaveFund(ctx, admin, "", finance.FundInput{Code: "OFFERING", Name: "Offering", RestrictionType: "unrestricted", ActiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}, "fund"); err != nil {
		t.Fatal(err)
	}
	if _, err = financeService.SavePeriod(ctx, admin, "", finance.FiscalPeriodInput{Code: "FY2026", Name: "2026 fiscal year", StartsAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), EndsAt: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)}, "period"); err != nil {
		t.Fatal(err)
	}
	if _, err = financeService.SaveSequence(ctx, admin, "", finance.ReceiptSequenceInput{Code: "MAIN2026", Prefix: "REMI", FiscalYear: 2026, Padding: 6, StartingNumber: 1}, "sequence"); err != nil {
		t.Fatal(err)
	}
	handler := New(Services{Finance: financeService, Authorizer: authorizer}, "org-http")
	jwt := services.NewJWTService("provider-http-secret-with-at-least-32-bytes")
	financeToken, _ := jwt.Generate("finance-reader", "finance@example.com", "Finance", "finance-auditor")
	editorToken, _ := jwt.Generate("editor-reader", "editor@example.com", "Editor", "editor")
	router := chi.NewRouter()
	router.Use(platform.RequestIDMiddleware)
	router.Post("/api/payments/paystack/intents", handler.CreatePaystackIntent)
	router.Post("/api/webhooks/paystack", handler.ReceivePaystackWebhook)
	router.With(middleware.Auth(jwt)).Mount("/api/chms/v1", handler.Routes())
	server := httptest.NewServer(router)
	defer server.Close()

	status, intent := integrationJSONWithKey(t, server.URL+"/api/payments/paystack/intents", http.MethodPost, "", "public-checkout-http", map[string]any{"email": "giver@example.com", "amount": 15_000, "category": "Offering"})
	if status != http.StatusCreated || intent["authorizationUrl"] == "" || intent["reference"] == "" {
		t.Fatalf("intent status=%d body=%v", status, intent)
	}
	reference := intent["reference"].(string)
	webhook, _ := json.Marshal(map[string]any{"event": "charge.success", "data": map[string]any{"reference": reference, "amount": 15_000, "currency": "GHS"}})
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/webhooks/paystack", bytes.NewReader(webhook))
	req.Header.Set("X-Paystack-Signature", "invalid")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad signature status=%d", response.StatusCode)
	}
	req, _ = http.NewRequest(http.MethodPost, server.URL+"/api/webhooks/paystack", bytes.NewReader(webhook))
	req.Header.Set("X-Paystack-Signature", provider.sign(webhook))
	response, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("valid webhook status=%d", response.StatusCode)
	}
	status, list := integrationJSON(t, server.URL+"/api/chms/v1/finance/payment-intents?branchId=accra", http.MethodGet, financeToken, nil)
	if status != http.StatusOK || len(list["items"].([]any)) != 1 || list["items"].([]any)[0].(map[string]any)["state"] != "success" {
		t.Fatalf("finance intent list status=%d body=%v", status, list)
	}
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/finance/payment-intents?branchId=accra", http.MethodGet, editorToken, nil)
	if status != http.StatusNotFound {
		t.Fatalf("editor inferred finance intents status=%d", status)
	}
}
