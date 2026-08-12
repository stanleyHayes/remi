package services

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"remi-api/internal/chms/finance"
)

// PaystackService initialises transactions against Paystack when a secret key
// is configured.
type PaystackService struct {
	secretKey string
	client    *http.Client
	baseURL   string
}

type PaystackInitResult struct {
	AuthorizationURL string `json:"authorizationUrl"`
	Reference        string `json:"reference"`
}

func NewPaystackService(secretKey string) *PaystackService {
	return &PaystackService{
		secretKey: secretKey,
		client:    &http.Client{Timeout: 15 * time.Second},
		baseURL:   "https://api.paystack.co",
	}
}

func (p *PaystackService) Configured() bool { return p.secretKey != "" }

func NewPaystackServiceWithClient(secretKey, baseURL string, client *http.Client) *PaystackService {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &PaystackService{secretKey: secretKey, client: client, baseURL: strings.TrimRight(baseURL, "/")}
}

func (p *PaystackService) VerifyWebhook(raw []byte, signature string) bool {
	if p.secretKey == "" || len(raw) == 0 {
		return false
	}
	want := hmac.New(sha512.New, []byte(p.secretKey))
	_, _ = want.Write(raw)
	provided, err := hexSignature(signature)
	return err == nil && hmac.Equal(want.Sum(nil), provided)
}

func hexSignature(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if len(value) != sha512.Size*2 {
		return nil, fmt.Errorf("invalid signature length")
	}
	result := make([]byte, sha512.Size)
	for i := 0; i < len(result); i++ {
		part, err := strconv.ParseUint(value[i*2:i*2+2], 16, 8)
		if err != nil {
			return nil, err
		}
		result[i] = byte(part)
	}
	return result, nil
}

func (p *PaystackService) InitializePayment(ctx context.Context, input finance.ProviderInitializeRequest) (*finance.ProviderInitializeResult, error) {
	payload := map[string]any{"amount": input.AmountMinor, "currency": input.Currency, "email": input.Email, "reference": input.Reference, "metadata": input.Metadata}
	if strings.TrimSpace(input.CallbackURL) != "" {
		payload["callback_url"] = strings.TrimSpace(input.CallbackURL)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/transaction/initialize", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.secretKey)
	req.Header.Set("Content-Type", "application/json")
	var out struct {
		Status  bool   `json:"status"`
		Message string `json:"message"`
		Data    struct {
			AuthorizationURL string `json:"authorization_url"`
			Reference        string `json:"reference"`
		} `json:"data"`
	}
	if err = p.doJSON(req, &out); err != nil {
		return nil, err
	}
	if !out.Status {
		return nil, fmt.Errorf("paystack: %s", out.Message)
	}
	return &finance.ProviderInitializeResult{AuthorizationURL: out.Data.AuthorizationURL, Reference: out.Data.Reference}, nil
}

func (p *PaystackService) VerifyPayment(ctx context.Context, reference string) (*finance.ProviderVerification, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/transaction/verify/"+url.PathEscape(reference), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.secretKey)
	var out struct {
		Status  bool   `json:"status"`
		Message string `json:"message"`
		Data    struct {
			ID            json.Number `json:"id"`
			Status        string      `json:"status"`
			Reference     string      `json:"reference"`
			Amount        int64       `json:"amount"`
			Currency      string      `json:"currency"`
			Channel       string      `json:"channel"`
			PaidAt        time.Time   `json:"paid_at"`
			Settlement    any         `json:"settlement"`
			Authorization struct {
				Code        string `json:"authorization_code"`
				Signature   string `json:"signature"`
				Reusable    bool   `json:"reusable"`
				Channel     string `json:"channel"`
				CardType    string `json:"card_type"`
				Brand       string `json:"brand"`
				Last4       string `json:"last4"`
				ExpiryMonth string `json:"exp_month"`
				ExpiryYear  string `json:"exp_year"`
				Bank        string `json:"bank"`
				CountryCode string `json:"country_code"`
			} `json:"authorization"`
		} `json:"data"`
	}
	if err = p.doJSON(req, &out); err != nil {
		return nil, err
	}
	if !out.Status {
		return nil, fmt.Errorf("paystack: %s", out.Message)
	}
	settlement := ""
	if out.Data.Settlement != nil {
		settlement = strings.TrimSpace(fmt.Sprint(out.Data.Settlement))
		if settlement == "<nil>" {
			settlement = ""
		}
	}
	verification := &finance.ProviderVerification{Reference: out.Data.Reference, Status: strings.ToLower(out.Data.Status), AmountMinor: out.Data.Amount, Currency: strings.ToUpper(out.Data.Currency), Channel: strings.ToLower(out.Data.Channel), TransactionID: out.Data.ID.String(), SettlementReference: settlement, PaidAt: out.Data.PaidAt}
	if strings.TrimSpace(out.Data.Authorization.Code) != "" {
		verification.Authorization = &finance.ProviderAuthorization{Code: strings.TrimSpace(out.Data.Authorization.Code), Signature: strings.TrimSpace(out.Data.Authorization.Signature), Reusable: out.Data.Authorization.Reusable, Channel: strings.ToLower(strings.TrimSpace(out.Data.Authorization.Channel)), CardType: strings.TrimSpace(out.Data.Authorization.CardType), Brand: strings.TrimSpace(out.Data.Authorization.Brand), Last4: strings.TrimSpace(out.Data.Authorization.Last4), ExpiryMonth: strings.TrimSpace(out.Data.Authorization.ExpiryMonth), ExpiryYear: strings.TrimSpace(out.Data.Authorization.ExpiryYear), Bank: strings.TrimSpace(out.Data.Authorization.Bank), CountryCode: strings.ToUpper(strings.TrimSpace(out.Data.Authorization.CountryCode))}
	}
	return verification, nil
}

func (p *PaystackService) ChargeAuthorization(ctx context.Context, authorizationCode, email string, amountMinor int64, currency, reference string, metadata map[string]any) (*finance.ProviderChargeResult, error) {
	payload := map[string]any{"authorization_code": authorizationCode, "email": email, "amount": amountMinor, "currency": currency, "reference": reference, "metadata": metadata, "queue": true}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/transaction/charge_authorization", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.secretKey)
	req.Header.Set("Content-Type", "application/json")
	var out struct {
		Status  bool   `json:"status"`
		Message string `json:"message"`
		Data    struct {
			Status           string `json:"status"`
			Reference        string `json:"reference"`
			AuthorizationURL string `json:"authorization_url"`
		} `json:"data"`
	}
	if err = p.doJSON(req, &out); err != nil {
		return nil, err
	}
	if !out.Status {
		return nil, fmt.Errorf("paystack: %s", out.Message)
	}
	return &finance.ProviderChargeResult{Reference: out.Data.Reference, Status: strings.ToLower(strings.TrimSpace(out.Data.Status)), AuthorizationURL: strings.TrimSpace(out.Data.AuthorizationURL)}, nil
}

func (p *PaystackService) doJSON(req *http.Request, out any) error {
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("paystack HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 1<<20))
	decoder.UseNumber()
	return decoder.Decode(out)
}

func (p *PaystackService) Initialize(amountPesewas int64, email, reference, category string) (*PaystackInitResult, error) {
	result, err := p.InitializePayment(context.Background(), finance.ProviderInitializeRequest{AmountMinor: amountPesewas, Currency: "GHS", Email: email, Reference: reference, Metadata: map[string]any{"category": category}})
	if err != nil {
		return nil, err
	}
	return &PaystackInitResult{AuthorizationURL: result.AuthorizationURL, Reference: result.Reference}, nil
}
