package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// SMSService sends transactional messages through Arkesel's Ghana-focused V2
// REST API. With no key configured it logs in development, matching the email
// transport's safe local-demo behaviour.
type SMSService struct {
	apiKey string
	sender string
	client *http.Client
}

func NewSMSService(apiKey, sender string) *SMSService {
	sender = strings.TrimSpace(sender)
	if sender == "" {
		sender = "REMI"
	}
	if len(sender) > 11 {
		sender = sender[:11]
	}
	return &SMSService{apiKey: strings.TrimSpace(apiKey), sender: sender, client: &http.Client{Timeout: 10 * time.Second}}
}

func (s *SMSService) Send(to, message string) error {
	_, err := s.SendTracked(to, message)
	return err
}

func (s *SMSService) Configured() bool { return strings.TrimSpace(s.apiKey) != "" }

func (s *SMSService) SendTracked(to, message string) (string, error) {
	if s.apiKey == "" {
		log.Printf("[sms:demo] to=%s message=%q", to, message)
		return "", nil
	}
	payload, _ := json.Marshal(map[string]any{"sender": s.sender, "message": message, "recipients": []string{to}})
	req, err := http.NewRequest(http.MethodPost, "https://sms.arkesel.com/api/v2/sms/send", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("api-key", s.apiKey)
	req.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("arkesel returned %s", response.Status)
	}
	var result map[string]any
	_ = json.NewDecoder(response.Body).Decode(&result)
	for _, key := range []string{"id", "message_id", "request_id"} {
		if value, ok := result[key].(string); ok && strings.TrimSpace(value) != "" {
			return value, nil
		}
	}
	if value := strings.TrimSpace(response.Header.Get("X-Request-ID")); value != "" {
		return value, nil
	}
	return fmt.Sprintf("arkesel-%d", time.Now().UTC().UnixNano()), nil
}
