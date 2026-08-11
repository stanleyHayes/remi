package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// EmailService sends via Resend when configured, otherwise logs to stdout.
type EmailService struct {
	apiKey string
	from   string
	client *http.Client
}

func NewEmailService(apiKey, from string) *EmailService {
	return &EmailService{
		apiKey: apiKey,
		from:   from,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (e *EmailService) Send(to, subject, html string) error {
	if e.apiKey == "" {
		log.Printf("[email:demo] to=%s subject=%q\n%s", to, subject, html)
		return nil
	}
	payload := map[string]any{
		"from":    e.from,
		"to":      []string{to},
		"subject": subject,
		"html":    html,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("resend returned %s", resp.Status)
	}
	return nil
}
