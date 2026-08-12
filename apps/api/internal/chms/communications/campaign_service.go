package communications

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"remi-api/internal/chms/platform"
)

func (s Service) campaignAllowed(p platform.Principal, action string, branch platform.ID) bool {
	return p.Actor.Type != platform.ActorMember && s.Authorizer.Authorize(p, platform.AccessRequest{Action: action, ResourceType: "communication-campaign", OrganizationID: p.OrganizationID, BranchID: branch, FieldClasses: []platform.FieldClass{platform.FieldPersonal}, Now: s.now()}).Allowed
}
func (s Service) templateAllowed(p platform.Principal, action string, branch platform.ID) bool {
	return p.Actor.Type != platform.ActorMember && s.Authorizer.Authorize(p, platform.AccessRequest{Action: action, ResourceType: "communication-template", OrganizationID: p.OrganizationID, BranchID: branch, FieldClasses: []platform.FieldClass{platform.FieldPersonal}, Now: s.now()}).Allowed
}

func (s Service) CreateTemplate(ctx context.Context, p platform.Principal, in TemplateInput, requestID string) (*Template, error) {
	if err := in.NormalizeAndValidate(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_template", Message: err.Error()})
	}
	if !s.templateAllowed(p, "create", in.BranchID) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot create templates for this branch."}
	}
	now := s.now()
	v := Template{ResourceEnvelope: platform.ResourceEnvelope{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: in.BranchID, SchemaVersion: CampaignSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: p.Actor, UpdatedAt: now, UpdatedBy: p.Actor}, Name: in.Name, Channel: in.Channel, Subject: in.Subject, Body: in.Body, State: "draft"}
	err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Repository.InsertTemplate(tx, v); err != nil {
			return err
		}
		return s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: v.BranchID, Actor: p.Actor, Action: "communications.template.create", ResourceType: "communication-template", ResourceID: v.ID, ChangedFields: []string{"name", "channel", "subject", "body", "state"}, Outcome: "success", RequestID: requestID, OccurredAt: now})
	})
	return &v, err
}
func (s Service) UpdateTemplate(ctx context.Context, p platform.Principal, id platform.ID, in TemplateInput, requestID string) (*Template, error) {
	if err := in.NormalizeAndValidate(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_template", Message: err.Error()})
	}
	v, err := s.Repository.FindTemplate(ctx, p.OrganizationID, id)
	if err != nil || v == nil {
		return nil, communicationNotFound(err, "Template not found.")
	}
	if !s.templateAllowed(p, "update", v.BranchID) || v.BranchID != in.BranchID {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot update this template."}
	}
	if v.Version != in.ExpectedVersion {
		return nil, platform.VersionConflict(in.ExpectedVersion)
	}
	v.Name, v.Channel, v.Subject, v.Body, v.State = in.Name, in.Channel, in.Subject, in.Body, "draft"
	v.PublishedAt = nil
	v.Version++
	v.UpdatedAt = s.now()
	v.UpdatedBy = p.Actor
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Repository.ReplaceTemplate(tx, *v, in.ExpectedVersion); err != nil {
			return err
		}
		return s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: v.BranchID, Actor: p.Actor, Action: "communications.template.update", ResourceType: "communication-template", ResourceID: v.ID, ChangedFields: []string{"name", "channel", "subject", "body", "state"}, Outcome: "success", RequestID: requestID, OccurredAt: v.UpdatedAt})
	})
	return v, err
}
func (s Service) PublishTemplate(ctx context.Context, p platform.Principal, id platform.ID, expected int64, requestID string) (*Template, error) {
	v, err := s.Repository.FindTemplate(ctx, p.OrganizationID, id)
	if err != nil || v == nil {
		return nil, communicationNotFound(err, "Template not found.")
	}
	if !s.templateAllowed(p, "approve", v.BranchID) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot publish this template."}
	}
	if v.Version != expected {
		return nil, platform.VersionConflict(expected)
	}
	now := s.now()
	v.State = "published"
	v.PublishedAt = &now
	v.Version++
	v.UpdatedAt = now
	v.UpdatedBy = p.Actor
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Repository.ReplaceTemplate(tx, *v, expected); err != nil {
			return err
		}
		return s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: v.BranchID, Actor: p.Actor, Action: "communications.template.publish", ResourceType: "communication-template", ResourceID: v.ID, ChangedFields: []string{"state", "publishedAt"}, Outcome: "success", RequestID: requestID, OccurredAt: now})
	})
	return v, err
}
func (s Service) ListTemplates(ctx context.Context, p platform.Principal, branch platform.ID) ([]Template, error) {
	if !s.templateAllowed(p, "read", branch) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot read templates for this branch."}
	}
	return s.Repository.ListTemplates(ctx, p.OrganizationID, branch)
}

func (s Service) CreateCampaign(ctx context.Context, p platform.Principal, in CampaignInput, requestID string) (*Campaign, error) {
	if err := in.NormalizeAndValidate(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_campaign", Message: err.Error()})
	}
	if !s.campaignAllowed(p, "create", in.BranchID) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot create campaigns for this branch."}
	}
	a, err := s.Repository.FindAudience(ctx, p.OrganizationID, in.AudienceID)
	if err != nil || a == nil {
		return nil, communicationNotFound(err, "Audience not found.")
	}
	t, err := s.Repository.FindTemplate(ctx, p.OrganizationID, in.TemplateID)
	if err != nil || t == nil {
		return nil, communicationNotFound(err, "Template not found.")
	}
	if a.BranchID != in.BranchID || t.BranchID != in.BranchID || a.Channel != t.Channel {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "channel_mismatch", Message: "Audience and template must share the branch and channel."})
	}
	now := s.now()
	v := Campaign{ResourceEnvelope: platform.ResourceEnvelope{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: in.BranchID, SchemaVersion: CampaignSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: p.Actor, UpdatedAt: now, UpdatedBy: p.Actor}, Name: in.Name, AudienceID: a.ID, AudienceVersion: a.Version, TemplateID: t.ID, TemplateVersion: t.Version, Purpose: a.Purpose, Channel: a.Channel, State: "draft"}
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Repository.InsertCampaign(tx, v); err != nil {
			return err
		}
		return s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: v.BranchID, Actor: p.Actor, Action: "communications.campaign.create", ResourceType: "communication-campaign", ResourceID: v.ID, ChangedFields: []string{"audienceId", "audienceVersion", "templateId", "templateVersion", "purpose", "channel", "state"}, Outcome: "success", RequestID: requestID, OccurredAt: now})
	})
	return &v, err
}
func (s Service) SubmitCampaign(ctx context.Context, p platform.Principal, id platform.ID, expected int64, requestID string) (*Campaign, error) {
	return s.transitionCampaign(ctx, p, id, expected, "draft", "pending-approval", "submit", requestID)
}
func (s Service) ApproveCampaign(ctx context.Context, p platform.Principal, id platform.ID, expected int64, requestID string) (*Campaign, error) {
	v, err := s.Repository.FindCampaign(ctx, p.OrganizationID, id)
	if err != nil || v == nil {
		return nil, communicationNotFound(err, "Campaign not found.")
	}
	if !s.campaignAllowed(p, "approve", v.BranchID) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot approve this campaign."}
	}
	if p.Actor.ID == v.CreatedBy.ID {
		return nil, &platform.DomainError{Code: "forbidden", Message: "Campaign creators cannot approve their own campaign."}
	}
	if v.State != "pending-approval" || v.Version != expected {
		return nil, &platform.DomainError{Code: "invalid_transition", Message: "Only the current pending campaign version can be approved."}
	}
	a, _ := s.Repository.FindAudience(ctx, p.OrganizationID, v.AudienceID)
	t, _ := s.Repository.FindTemplate(ctx, p.OrganizationID, v.TemplateID)
	if a == nil || t == nil || a.Version != v.AudienceVersion || t.Version != v.TemplateVersion || t.State != "published" {
		return nil, &platform.DomainError{Code: "conflict", Message: "Audience or template changed, or the template is not published. Create a fresh campaign version."}
	}
	now := s.now()
	v.State = "approved"
	v.ApprovedAt = &now
	v.ApprovedBy = p.Actor
	v.Version++
	v.UpdatedAt = now
	v.UpdatedBy = p.Actor
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Repository.ReplaceCampaign(tx, *v, expected); err != nil {
			return err
		}
		return s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: v.BranchID, Actor: p.Actor, Action: "communications.campaign.approve", ResourceType: "communication-campaign", ResourceID: v.ID, ChangedFields: []string{"state", "approvedAt", "approvedBy"}, Outcome: "success", RequestID: requestID, OccurredAt: now})
	})
	return v, err
}
func (s Service) ScheduleCampaign(ctx context.Context, p platform.Principal, id platform.ID, in ScheduleInput, requestID string) (*Campaign, error) {
	if err := in.NormalizeAndValidate(s.now()); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_schedule", Message: err.Error()})
	}
	v, err := s.Repository.FindCampaign(ctx, p.OrganizationID, id)
	if err != nil || v == nil {
		return nil, communicationNotFound(err, "Campaign not found.")
	}
	if !s.campaignAllowed(p, "operate", v.BranchID) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot schedule this campaign."}
	}
	if v.State != "approved" || v.Version != in.ExpectedVersion {
		return nil, &platform.DomainError{Code: "invalid_transition", Message: "Only the current approved campaign can be scheduled."}
	}
	if s.Sender == nil || !s.Sender.Configured(v.Channel) {
		return nil, &platform.DomainError{Code: "provider_unavailable", Message: "The selected channel provider is not configured."}
	}
	now := s.now()
	v.State = "scheduled"
	v.ScheduledAt = &in.ScheduledAt
	v.Timezone = in.Timezone
	v.Version++
	v.UpdatedAt = now
	v.UpdatedBy = p.Actor
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Repository.ReplaceCampaign(tx, *v, in.ExpectedVersion); err != nil {
			return err
		}
		if err := s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: v.BranchID, Actor: p.Actor, Action: "communications.campaign.schedule", ResourceType: "communication-campaign", ResourceID: v.ID, ChangedFields: []string{"state", "scheduledAt", "timezone"}, Outcome: "success", RequestID: requestID, OccurredAt: now}); err != nil {
			return err
		}
		return s.Platform.EnqueueEvent(tx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: v.BranchID, Type: "communications.campaign.scheduled", EventVersion: 1, AggregateType: "communication-campaign", AggregateID: v.ID, AggregateVersion: v.Version, Actor: p.Actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: map[string]any{"campaignId": v.ID, "availableAt": in.ScheduledAt}}, State: "pending", AvailableAt: now})
	})
	return v, err
}
func (s Service) transitionCampaign(ctx context.Context, p platform.Principal, id platform.ID, expected int64, from, to, action, requestID string) (*Campaign, error) {
	v, err := s.Repository.FindCampaign(ctx, p.OrganizationID, id)
	if err != nil || v == nil {
		return nil, communicationNotFound(err, "Campaign not found.")
	}
	if !s.campaignAllowed(p, "update", v.BranchID) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot update this campaign."}
	}
	if v.State != from || v.Version != expected {
		return nil, &platform.DomainError{Code: "invalid_transition", Message: "Campaign state or version changed."}
	}
	now := s.now()
	v.State = to
	if to == "pending-approval" {
		v.ApprovalRequestedAt = &now
	}
	v.Version++
	v.UpdatedAt = now
	v.UpdatedBy = p.Actor
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Repository.ReplaceCampaign(tx, *v, expected); err != nil {
			return err
		}
		return s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: v.BranchID, Actor: p.Actor, Action: "communications.campaign." + action, ResourceType: "communication-campaign", ResourceID: v.ID, ChangedFields: []string{"state"}, Outcome: "success", RequestID: requestID, OccurredAt: now})
	})
	return v, err
}
func (s Service) ListCampaigns(ctx context.Context, p platform.Principal, branch platform.ID) ([]Campaign, error) {
	if !s.campaignAllowed(p, "read", branch) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot read campaigns for this branch."}
	}
	return s.Repository.ListCampaigns(ctx, p.OrganizationID, branch)
}
func (s Service) GetCampaign(ctx context.Context, p platform.Principal, id platform.ID) (*Campaign, []Delivery, error) {
	v, err := s.Repository.FindCampaign(ctx, p.OrganizationID, id)
	if err != nil || v == nil {
		return nil, nil, communicationNotFound(err, "Campaign not found.")
	}
	if !s.campaignAllowed(p, "read", v.BranchID) {
		return nil, nil, &platform.DomainError{Code: "forbidden", Message: "You cannot read this campaign."}
	}
	deliveries, err := s.Repository.ListDeliveries(ctx, p.OrganizationID, id)
	return v, deliveries, err
}

func (s Service) RunDue(ctx context.Context) (int, error) {
	if s.Sender == nil {
		return 0, errors.New("communication sender is not configured")
	}
	due, err := s.Repository.ListDueCampaigns(ctx, s.now(), 20)
	if err != nil {
		return 0, err
	}
	done := 0
	for _, campaign := range due {
		claimed, claimErr := s.Repository.ClaimCampaign(ctx, campaign.OrganizationID, campaign.ID, campaign.Version, s.now())
		if claimErr != nil {
			return done, claimErr
		}
		if !claimed {
			continue
		}
		if dispatchErr := s.dispatch(ctx, campaign); dispatchErr != nil {
			return done, dispatchErr
		}
		done++
	}
	return done, nil
}
func (s Service) dispatch(ctx context.Context, campaign Campaign) error {
	system := platform.Principal{Actor: platform.Actor{Type: platform.ActorSystem, ID: "communication-worker"}, OrganizationID: campaign.OrganizationID, Grants: []platform.Grant{{Action: "read", Resource: "communication-audience", BranchIDs: []platform.ID{campaign.BranchID}, FieldClasses: []platform.FieldClass{platform.FieldPersonal}}}}
	preview, recipients, err := s.Preview(ctx, system, campaign.AudienceID, 200)
	if err != nil {
		return s.Repository.FinalizeCampaign(ctx, campaign.OrganizationID, campaign.ID, 0, 0, 1, 0, s.now())
	}
	template, err := s.Repository.FindTemplate(ctx, campaign.OrganizationID, campaign.TemplateID)
	if err != nil || template == nil || template.Version != campaign.TemplateVersion || campaign.AudienceVersion != preview.AudienceVersion {
		return s.Repository.FinalizeCampaign(ctx, campaign.OrganizationID, campaign.ID, 0, 0, 1, 0, s.now())
	}
	accepted, failed, suppressed := 0, 0, 0
	for _, recipient := range recipients {
		now := s.now()
		inboxSubject, inboxBody := renderTemplate(*template, recipient.DisplayName, "REMI", "")
		delivery := Delivery{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: campaign.OrganizationID, BranchID: campaign.BranchID, CampaignID: campaign.ID, PersonID: recipient.PersonID, Channel: campaign.Channel, DestinationHint: recipient.Masked, InboxSubject: inboxSubject, InboxBody: inboxBody, Purpose: campaign.Purpose, State: "queued", CreatedAt: now, UpdatedAt: now}
		saved, created, saveErr := s.Repository.UpsertDelivery(ctx, delivery)
		if saveErr != nil {
			return saveErr
		}
		if !created {
			continue
		}
		allowed, reason, policyErr := s.Repository.EvaluateMemberDeliveryPolicy(ctx, campaign.OrganizationID, recipient.PersonID, now)
		if policyErr != nil {
			failed++
			_ = s.Repository.UpdateDelivery(ctx, saved.ID, "failed", "", "", "preference_policy_failed", s.now())
			continue
		}
		if !allowed {
			suppressed++
			_ = s.Repository.UpdateDelivery(ctx, saved.ID, "suppressed", "", "", reason, s.now())
			continue
		}
		unsubscribeURL := ""
		if s.OptOuts != nil && strings.TrimSpace(s.PublicWebURL) != "" {
			token, tokenErr := s.OptOuts.Sign(OptOutClaims{OrganizationID: campaign.OrganizationID, BranchID: campaign.BranchID, PersonID: recipient.PersonID, CampaignID: campaign.ID, Purpose: campaign.Purpose, Channel: campaign.Channel, ExpiresAt: now.AddDate(1, 0, 0)})
			if tokenErr != nil {
				failed++
				_ = s.Repository.UpdateDelivery(ctx, saved.ID, "failed", "", "", "optout_token_failed", s.now())
				continue
			}
			unsubscribeURL = strings.TrimRight(s.PublicWebURL, "/") + "/unsubscribe?token=" + url.QueryEscape(token)
		}
		if campaign.Channel == "email" && unsubscribeURL == "" {
			failed++
			_ = s.Repository.UpdateDelivery(ctx, saved.ID, "failed", "", "", "optout_unavailable", s.now())
			continue
		}
		subject, body := renderTemplate(*template, recipient.DisplayName, "REMI", unsubscribeURL)
		receipt, sendErr := s.Sender.Send(ctx, campaign.Channel, recipient.Destination, subject, body)
		if sendErr != nil {
			failed++
			_ = s.Repository.UpdateDelivery(ctx, saved.ID, "failed", "", "", "provider_rejected", s.now())
			continue
		}
		accepted++
		_ = s.Repository.UpdateDelivery(ctx, saved.ID, "accepted", receipt.Provider, receipt.Reference, "", s.now())
	}
	return s.Repository.FinalizeCampaign(ctx, campaign.OrganizationID, campaign.ID, len(recipients), accepted, failed, suppressed, s.now())
}
func renderTemplate(t Template, name, church, unsubscribeURL string) (string, string) {
	first := strings.Fields(name)
	firstName := "Member"
	if len(first) > 0 {
		firstName = first[0]
	}
	replace := strings.NewReplacer("{{first_name}}", firstName, "{{church_name}}", church, "{{unsubscribe_url}}", unsubscribeURL)
	subject, body := replace.Replace(t.Subject), replace.Replace(t.Body)
	if t.Channel != "email" && !strings.Contains(strings.ToUpper(body), "STOP") {
		body += "\nReply STOP to opt out."
	}
	return subject, body
}
func communicationNotFound(err error, message string) error {
	if err != nil {
		return err
	}
	return &platform.DomainError{Code: "not_found", Message: message}
}
