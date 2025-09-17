package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/synesthesie/backend/internal/config"
)

type SMSService struct {
	cfg    *config.Config
	client *http.Client
}

type clickSendMessage struct {
	Source   string `json:"source"`
	Body     string `json:"body"`
	To       string `json:"to"`
	From     string `json:"from,omitempty"`
	Schedule int64  `json:"schedule,omitempty"`
}

type clickSendPayload struct {
	Messages []clickSendMessage `json:"messages"`
}

func NewSMSService(cfg *config.Config) *SMSService {
	return &SMSService{
		cfg:    cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *SMSService) SendSMS(to, body string) error {
	if !s.cfg.SMSVerificationEnabled {
		return nil
	}
	switch strings.ToLower(s.cfg.SMSProvider) {
	case "seven":
		return s.sendViaSeven(to, body)
	case "clicksend":
		return s.sendViaClickSend(to, body)
	default:
		return s.sendViaSeven(to, body)
	}
}

// seven.io API v1: POST https://gateway.seven.io/api/sms
// Header: X-Api-Key: <key>
// Form: to=<E164>&text=<msg>&from=<id>
func (s *SMSService) sendViaSeven(to, body string) error {
	if s.cfg.SevenAPIKey == "" {
		return fmt.Errorf("seven api key missing")
	}
	form := url.Values{}
	form.Set("to", to)
	form.Set("text", body)
	if s.cfg.SMSFrom != "" {
		form.Set("from", s.cfg.SMSFrom)
	}
	req, err := http.NewRequest("POST", "https://gateway.seven.io/api/sms", bytes.NewBufferString(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Api-Key", s.cfg.SevenAPIKey)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("seven send failed: %d", resp.StatusCode)
	}
	return nil
}

// ClickSend fallback (legacy)
func (s *SMSService) sendViaClickSend(to, body string) error {
	if s.cfg.ClickSendUsername == "" || s.cfg.ClickSendAPIKey == "" {
		return fmt.Errorf("sms provider not configured")
	}
	msg := clickSendMessage{Source: "api", Body: body, To: to, From: s.cfg.ClickSendFrom}
	payload := clickSendPayload{Messages: []clickSendMessage{msg}}
	b, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", "https://rest.clicksend.com/v3/sms/send", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(s.cfg.ClickSendUsername, s.cfg.ClickSendAPIKey)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("sms send failed with status %d", resp.StatusCode)
	}
	return nil
}
