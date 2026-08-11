package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// PaystackService initialises transactions against Paystack when a secret key
// is configured.
type PaystackService struct {
	secretKey string
	client    *http.Client
}

type PaystackInitResult struct {
	AuthorizationURL string `json:"authorizationUrl"`
	Reference        string `json:"reference"`
}

func NewPaystackService(secretKey string) *PaystackService {
	return &PaystackService{
		secretKey: secretKey,
		client:    &http.Client{Timeout: 15 * time.Second},
	}
}

func (p *PaystackService) Configured() bool { return p.secretKey != "" }

func (p *PaystackService) Initialize(amountPesewas int64, email, reference, category string) (*PaystackInitResult, error) {
	payload := map[string]any{
		"amount":    amountPesewas,
		"email":     email,
		"reference": reference,
		"metadata":  map[string]any{"category": category},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, "https://api.paystack.co/transaction/initialize", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.secretKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var out struct {
		Status  bool   `json:"status"`
		Message string `json:"message"`
		Data    struct {
			AuthorizationURL string `json:"authorization_url"`
			Reference        string `json:"reference"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if !out.Status {
		return nil, fmt.Errorf("paystack: %s", out.Message)
	}
	return &PaystackInitResult{AuthorizationURL: out.Data.AuthorizationURL, Reference: out.Data.Reference}, nil
}
