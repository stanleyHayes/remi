package services

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
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
	_, err := e.send(to, subject, html, nil)
	return err
}

func (e *EmailService) Configured() bool { return strings.TrimSpace(e.apiKey) != "" }

// SendTracked returns Resend's durable email identifier for delivery-event
// correlation. Campaign delivery never treats the local demo logger as a
// configured provider.
func (e *EmailService) SendTracked(to, subject, html string) (string, error) {
	return e.send(to, subject, html, nil)
}

// SendAttachment sends a private binary artifact without logging its contents
// in demo mode. Resend accepts attachment content as base64.
func (e *EmailService) SendAttachment(to, subject, html, filename string, content []byte) error {
	filename = strings.TrimSpace(filename)
	if filename == "" || strings.ContainsAny(filename, "\r\n") {
		return fmt.Errorf("attachment filename is invalid")
	}
	if len(content) == 0 || len(content) > 10<<20 {
		return fmt.Errorf("attachment size is outside the supported range")
	}
	attachment := map[string]string{"filename": filename, "content": base64.StdEncoding.EncodeToString(content)}
	_, err := e.send(to, subject, html, []map[string]string{attachment})
	return err
}

func (e *EmailService) send(to, subject, html string, attachments []map[string]string) (string, error) {
	if e.apiKey == "" {
		if len(attachments) > 0 {
			log.Printf("[email:demo] to=%s subject=%q attachments=%d filename=%q", to, subject, len(attachments), attachments[0]["filename"])
		} else {
			log.Printf("[email:demo] to=%s subject=%q\n%s", to, subject, html)
		}
		return "", nil
	}
	payload := map[string]any{
		"from":    e.from,
		"to":      []string{to},
		"subject": subject,
		"html":    html,
	}
	if len(attachments) > 0 {
		payload["attachments"] = attachments
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("resend returned %s", resp.Status)
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || strings.TrimSpace(result.ID) == "" {
		return "", fmt.Errorf("resend returned an invalid receipt")
	}
	return result.ID, nil
}
