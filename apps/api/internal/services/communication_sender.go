package services

import (
	"context"
	"fmt"
	"time"

	"remi-api/internal/chms/communications"
)

type CommunicationSender struct {
	Email    *EmailService
	SMS      *SMSService
	WhatsApp *WhatsAppService
}

func (s CommunicationSender) Configured(channel string) bool {
	switch channel {
	case "email":
		return s.Email != nil && s.Email.Configured()
	case "sms":
		return s.SMS != nil && s.SMS.Configured()
	case "whatsapp":
		return s.WhatsApp != nil && s.WhatsApp.Configured()
	default:
		return false
	}
}

func (s CommunicationSender) Send(_ context.Context, channel, destination, subject, body string) (communications.ProviderReceipt, error) {
	var provider, reference string
	var err error
	switch channel {
	case "email":
		provider = "resend"
		if s.Email == nil {
			err = fmt.Errorf("email provider is unavailable")
		} else {
			reference, err = s.Email.SendTracked(destination, subject, body)
		}
	case "sms":
		provider = "arkesel"
		if s.SMS == nil {
			err = fmt.Errorf("sms provider is unavailable")
		} else {
			reference, err = s.SMS.SendTracked(destination, body)
		}
	case "whatsapp":
		provider = "meta-whatsapp"
		if s.WhatsApp == nil {
			err = fmt.Errorf("whatsapp provider is unavailable")
		} else {
			reference, err = s.WhatsApp.SendTracked(destination, body)
		}
	default:
		err = fmt.Errorf("unsupported communication channel")
	}
	if err != nil {
		return communications.ProviderReceipt{}, err
	}
	return communications.ProviderReceipt{Provider: provider, Reference: reference, AcceptedAt: time.Now().UTC()}, nil
}
