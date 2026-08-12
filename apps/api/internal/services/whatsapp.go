package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// WhatsAppService sends free-form text through Meta's Cloud API. Production
// operators remain responsible for approved-template/session policy; REMI
// never falls back to simulated sends when this transport is unconfigured.
type WhatsAppService struct {
	accessToken   string
	phoneNumberID string
	graphVersion  string
	client        *http.Client
}

func NewWhatsAppService(accessToken, phoneNumberID, graphVersion string) *WhatsAppService {
	if strings.TrimSpace(graphVersion) == "" {
		graphVersion = "v23.0"
	}
	return &WhatsAppService{accessToken: strings.TrimSpace(accessToken), phoneNumberID: strings.TrimSpace(phoneNumberID), graphVersion: strings.Trim(strings.TrimSpace(graphVersion), "/"), client: &http.Client{Timeout: 12 * time.Second}}
}

func (s *WhatsAppService) Configured() bool { return s.accessToken != "" && s.phoneNumberID != "" }

func (s *WhatsAppService) SendTracked(to, message string) (string, error) {
	if !s.Configured() {
		return "", fmt.Errorf("whatsapp provider is not configured")
	}
	payload, err := json.Marshal(map[string]any{"messaging_product": "whatsapp", "recipient_type": "individual", "to": to, "type": "text", "text": map[string]any{"preview_url": false, "body": message}})
	if err != nil {
		return "", err
	}
	endpoint := fmt.Sprintf("https://graph.facebook.com/%s/%s/messages", url.PathEscape(s.graphVersion), url.PathEscape(s.phoneNumberID))
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+s.accessToken)
	req.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("whatsapp returned %s", response.Status)
	}
	var result struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil || len(result.Messages) == 0 || result.Messages[0].ID == "" {
		return "", fmt.Errorf("whatsapp returned an invalid receipt")
	}
	return result.Messages[0].ID, nil
}
