package services

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"remi-api/internal/chms/finance"
)

func TestPaystackRawSignatureInitializeAndVerifyContract(t *testing.T) {
	secret := "paystack-unit-secret"
	raw := []byte(`{"event":"charge.success","data":{"reference":"REMI-1"}}`)
	hash := hmac.New(sha512.New, []byte(secret))
	_, _ = hash.Write(raw)
	signature := hex.EncodeToString(hash.Sum(nil))

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/transaction/initialize":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if r.Header.Get("Authorization") != "Bearer "+secret || body["reference"] != "REMI-1" || body["currency"] != "GHS" || body["amount"] != float64(12500) {
				t.Fatalf("initialize request headers/body: %v %v", r.Header, body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"status": true, "data": map[string]any{"authorization_url": "https://checkout.paystack.test/access", "reference": "REMI-1"}})
		case r.Method == http.MethodGet && r.URL.Path == "/transaction/verify/REMI-1":
			_, _ = w.Write([]byte(`{"status":true,"data":{"id":18446744073709551615,"status":"success","reference":"REMI-1","amount":12500,"currency":"GHS","channel":"mobile_money","paid_at":"2026-08-11T12:00:00Z","settlement":3090024}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := NewPaystackServiceWithClient(secret, server.URL, server.Client())
	if !service.VerifyWebhook(raw, signature) || service.VerifyWebhook(append(raw, ' '), signature) || service.VerifyWebhook(raw, "bad") {
		t.Fatal("Paystack raw-body signature verification was not exact")
	}
	initialized, err := service.InitializePayment(context.Background(), finance.ProviderInitializeRequest{AmountMinor: 12500, Currency: "GHS", Email: "giver@example.com", Reference: "REMI-1", Metadata: map[string]any{"intent_id": "intent-1"}})
	if err != nil || initialized.Reference != "REMI-1" || initialized.AuthorizationURL == "" {
		t.Fatalf("initialize=%+v err=%v", initialized, err)
	}
	verified, err := service.VerifyPayment(context.Background(), "REMI-1")
	if err != nil || verified.TransactionID != "18446744073709551615" || verified.Status != "success" || verified.AmountMinor != 12500 || verified.Currency != "GHS" || verified.Channel != "mobile_money" || verified.SettlementReference != "3090024" || !verified.PaidAt.Equal(time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("verified=%+v err=%v", verified, err)
	}
}
