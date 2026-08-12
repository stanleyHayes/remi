package communications

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"remi-api/internal/chms/people"
	"remi-api/internal/chms/platform"
)

type UnitOfWork interface {
	WithTransaction(context.Context, func(context.Context) error) error
	AppendAudit(context.Context, platform.AuditEvent) error
	EnqueueEvent(context.Context, platform.OutboxRecord) error
}
type Service struct {
	Repository    *Repository
	Platform      UnitOfWork
	Authorizer    platform.Authorizer
	Sender        Sender
	OptOuts       *OptOutCodec
	PublicWebURL  string
	WebhookSecret string
	Now           func() time.Time
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func (s Service) allowed(p platform.Principal, action string, branch platform.ID, fields []platform.FieldClass) bool {
	return p.Actor.Type != platform.ActorMember && s.Authorizer.Authorize(p, platform.AccessRequest{Action: action, ResourceType: "communication-audience", OrganizationID: p.OrganizationID, BranchID: branch, FieldClasses: fields, Now: s.now()}).Allowed
}

func (s Service) Create(ctx context.Context, p platform.Principal, in SaveInput, requestID string) (*Audience, error) {
	if err := in.NormalizeAndValidate(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_audience", Message: err.Error()})
	}
	if !s.allowed(p, "create", in.BranchID, []platform.FieldClass{platform.FieldPersonal}) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot create communication audiences for this branch."}
	}
	if err := s.validateSegments(ctx, p, in.BranchID, in.SegmentIDs); err != nil {
		return nil, err
	}
	now := s.now()
	v := Audience{ResourceEnvelope: platform.ResourceEnvelope{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: in.BranchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: p.Actor, UpdatedAt: now, UpdatedBy: p.Actor}, Name: in.Name, Description: in.Description, SegmentIDs: in.SegmentIDs, ExcludedPersonIDs: in.ExcludedPersonIDs, Purpose: in.Purpose, Channel: in.Channel, OwnerID: p.Actor.ID}
	err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Repository.InsertAudience(tx, v); err != nil {
			return err
		}
		if err := s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: v.BranchID, Actor: p.Actor, Action: "communications.audience.create", ResourceType: "communication-audience", ResourceID: v.ID, ChangedFields: []string{"name", "segmentIds", "excludedPersonIds", "purpose", "channel"}, Outcome: "success", RequestID: requestID, OccurredAt: now}); err != nil {
			return err
		}
		return s.Platform.EnqueueEvent(tx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: v.BranchID, Type: "communications.audience.created", EventVersion: 1, AggregateType: "communication-audience", AggregateID: v.ID, AggregateVersion: 1, Actor: p.Actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: map[string]any{"audienceId": v.ID, "segmentCount": len(v.SegmentIDs)}}, State: "pending", AvailableAt: now})
	})
	return &v, err
}
func (s Service) Update(ctx context.Context, p platform.Principal, id platform.ID, in SaveInput, requestID string) (*Audience, error) {
	if err := in.NormalizeAndValidate(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_audience", Message: err.Error()})
	}
	cur, err := s.Repository.FindAudience(ctx, p.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if cur == nil {
		return nil, &platform.DomainError{Code: "not_found", Message: "Audience not found."}
	}
	if cur.OwnerID != p.Actor.ID || cur.BranchID != in.BranchID || !s.allowed(p, "update", cur.BranchID, []platform.FieldClass{platform.FieldPersonal}) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "Only the audience owner can update it."}
	}
	if cur.Version != in.ExpectedVersion {
		return nil, platform.VersionConflict(in.ExpectedVersion)
	}
	if err := s.validateSegments(ctx, p, in.BranchID, in.SegmentIDs); err != nil {
		return nil, err
	}
	v := *cur
	v.Name, v.Description, v.SegmentIDs, v.ExcludedPersonIDs, v.Purpose, v.Channel = in.Name, in.Description, in.SegmentIDs, in.ExcludedPersonIDs, in.Purpose, in.Channel
	v.Version++
	v.UpdatedAt = s.now()
	v.UpdatedBy = p.Actor
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Repository.ReplaceAudience(tx, v, in.ExpectedVersion); err != nil {
			return err
		}
		return s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: v.BranchID, Actor: p.Actor, Action: "communications.audience.update", ResourceType: "communication-audience", ResourceID: v.ID, ChangedFields: []string{"name", "segmentIds", "excludedPersonIds", "purpose", "channel"}, Outcome: "success", RequestID: requestID, OccurredAt: v.UpdatedAt})
	})
	return &v, err
}
func (s Service) List(ctx context.Context, p platform.Principal, branch platform.ID) ([]Audience, error) {
	if !s.allowed(p, "read", branch, []platform.FieldClass{platform.FieldPersonal}) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot read audiences for this branch."}
	}
	return s.Repository.ListAudiences(ctx, p.OrganizationID, branch)
}
func (s Service) validateSegments(ctx context.Context, p platform.Principal, branch platform.ID, ids []platform.ID) error {
	for _, id := range ids {
		seg, err := s.Repository.FindSegment(ctx, p.OrganizationID, id)
		if err != nil {
			return err
		}
		if seg == nil || seg.BranchID != branch || (p.Actor.Type != platform.ActorSystem && seg.Visibility != "organization" && seg.OwnerID != p.Actor.ID) {
			return &platform.DomainError{Code: "forbidden", Message: "An audience segment is unavailable or outside your scope."}
		}
	}
	return nil
}

func (s Service) Preview(ctx context.Context, p platform.Principal, id platform.ID, limit int) (*Preview, []Recipient, error) {
	a, err := s.Repository.FindAudience(ctx, p.OrganizationID, id)
	if err != nil || a == nil {
		if err == nil {
			err = &platform.DomainError{Code: "not_found", Message: "Audience not found."}
		}
		return nil, nil, err
	}
	if !s.allowed(p, "read", a.BranchID, []platform.FieldClass{platform.FieldPersonal}) {
		return nil, nil, &platform.DomainError{Code: "forbidden", Message: "You cannot preview this audience."}
	}
	if err := s.validateSegments(ctx, p, a.BranchID, a.SegmentIDs); err != nil {
		return nil, nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	counts := map[platform.ID]int{}
	peopleByID := map[platform.ID]people.Person{}
	candidateRows := 0
	for _, sid := range a.SegmentIDs {
		seg, findErr := s.Repository.FindSegment(ctx, p.OrganizationID, sid)
		if findErr != nil {
			return nil, nil, findErr
		}
		if seg == nil {
			return nil, nil, &platform.DomainError{Code: "conflict", Message: "An audience source segment is no longer available."}
		}
		rows, e := s.Repository.ResolveSegment(ctx, p.OrganizationID, seg.Filter)
		if e != nil {
			return nil, nil, e
		}
		for _, person := range rows {
			candidateRows++
			counts[person.ID]++
			peopleByID[person.ID] = person
		}
	}
	exclude := map[platform.ID]bool{}
	for _, pid := range a.ExcludedPersonIDs {
		exclude[pid] = true
	}
	ids := make([]string, 0, len(peopleByID))
	for pid := range peopleByID {
		ids = append(ids, string(pid))
	}
	sort.Strings(ids)
	result := &Preview{AudienceID: a.ID, AudienceVersion: a.Version, Purpose: a.Purpose, Channel: a.Channel, CandidateCount: candidateRows, ExclusionCounts: map[string]int{}, Recipients: []Recipient{}, ConsentEvaluated: s.now()}
	all := []Recipient{}
	destSeen := map[string]bool{}
	for _, raw := range ids {
		person := peopleByID[platform.ID(raw)]
		reason := ""
		if counts[person.ID] > 1 {
			result.ExclusionCounts["duplicate-segment-membership"] += counts[person.ID] - 1
		}
		if exclude[person.ID] {
			reason = "manual-exclusion"
		} else if isMinor(person, result.ConsentEvaluated) {
			reason = "minor"
		}
		dest := ""
		if reason == "" {
			dest = destination(person, a.Channel)
			if dest == "" {
				reason = "no-verified-contact"
			}
		}
		if reason == "" {
			decision, eligible, e := s.Repository.ConsentState(ctx, p.OrganizationID, person.ID, a.Purpose, a.Channel)
			if e != nil {
				return nil, nil, e
			}
			if !eligible {
				reason = decision
			}
		}
		if reason == "" && destSeen[dest] {
			reason = "duplicate-destination"
		}
		if reason != "" {
			result.ExclusionCounts[reason]++
			continue
		}
		destSeen[dest] = true
		name := strings.TrimSpace(person.Names.Preferred + " " + person.Names.Family)
		if strings.TrimSpace(person.Names.Preferred) == "" {
			name = strings.TrimSpace(person.Names.Given + " " + person.Names.Family)
		}
		all = append(all, Recipient{PersonID: person.ID, DisplayName: name, Destination: dest, Masked: mask(dest, a.Channel), SegmentCount: counts[person.ID]})
	}
	result.RecipientCount = len(all)
	result.Truncated = len(all) > limit
	if len(all) > limit {
		result.Recipients = all[:limit]
	} else {
		result.Recipients = all
	}
	return result, all, nil
}
func (s Service) Export(ctx context.Context, p platform.Principal, id platform.ID, reason, requestID string, w io.Writer) (*ExportRecord, error) {
	reason = strings.TrimSpace(reason)
	if len(reason) < 10 || len(reason) > 500 {
		return nil, platform.ValidationError(platform.FieldError{Path: "reason", Code: "invalid_reason", Message: "Give a 10 to 500 character export purpose."})
	}
	preview, recipients, err := s.Preview(ctx, p, id, 200)
	if err != nil {
		return nil, err
	}
	a, _ := s.Repository.FindAudience(ctx, p.OrganizationID, id)
	if !s.allowed(p, "export", a.BranchID, []platform.FieldClass{platform.FieldPersonal}) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot export recipient contact data."}
	}
	now := s.now()
	record := ExportRecord{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: a.BranchID, AudienceID: id, AudienceVersion: preview.AudienceVersion, RecipientCount: len(recipients), Purpose: a.Purpose, Channel: a.Channel, Reason: reason, Actor: p.Actor, RequestID: requestID, ExportedAt: now}
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Repository.InsertExport(tx, record); err != nil {
			return err
		}
		return s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: a.BranchID, Actor: p.Actor, Action: "communications.audience.export", ResourceType: "communication-audience", ResourceID: id, ChangedFields: []string{"recipient-contact-export"}, Outcome: "success", Reason: reason, RequestID: requestID, OccurredAt: now})
	})
	if err != nil {
		return nil, err
	}
	csvw := csv.NewWriter(w)
	_ = csvw.Write([]string{"person_id", "display_name", a.Channel})
	for _, r := range recipients {
		_ = csvw.Write([]string{string(r.PersonID), r.DisplayName, r.Destination})
	}
	csvw.Flush()
	if err := csvw.Error(); err != nil {
		return nil, err
	}
	return &record, nil
}
func (s Service) ListExports(ctx context.Context, p platform.Principal, id platform.ID) ([]ExportRecord, error) {
	a, err := s.Repository.FindAudience(ctx, p.OrganizationID, id)
	if err != nil || a == nil {
		if err == nil {
			err = &platform.DomainError{Code: "not_found", Message: "Audience not found."}
		}
		return nil, err
	}
	if !s.allowed(p, "read", a.BranchID, []platform.FieldClass{platform.FieldPersonal}) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot inspect exports."}
	}
	return s.Repository.ListExports(ctx, p.OrganizationID, id)
}
func destination(p people.Person, channel string) string {
	types := map[string][]string{"email": {"email"}, "sms": {"mobile", "phone"}, "whatsapp": {"whatsapp", "mobile"}, "phone": {"phone", "mobile"}, "push": {"push"}}[channel]
	for _, wanted := range types {
		for _, c := range p.ContactPoints {
			if c.Type == wanted && c.VerifiedAt != nil && c.Normalized != "" {
				return c.Normalized
			}
		}
	}
	return ""
}
func isMinor(p people.Person, now time.Time) bool {
	if p.DateOfBirth == nil || len(p.DateOfBirth.Value) < 4 {
		return false
	}
	var year int
	if _, err := fmt.Sscanf(p.DateOfBirth.Value[:4], "%d", &year); err != nil {
		return false
	}
	if year > now.Year()-18 {
		return true
	}
	if year < now.Year()-18 {
		return false
	}
	// Exact dates in the threshold year are minors until their birthday;
	// coarser year-only records are conservatively treated as adults.
	if p.DateOfBirth.Precision == "day" {
		var born time.Time
		if parsed, err := time.Parse("2006-01-02", p.DateOfBirth.Value); err == nil {
			born = parsed
			return now.Before(born.AddDate(18, 0, 0))
		}
	}
	if p.DateOfBirth.Precision == "month" {
		var born time.Time
		if parsed, err := time.Parse("2006-01", p.DateOfBirth.Value); err == nil {
			born = parsed
			return now.Before(born.AddDate(18, 0, 0))
		}
	}
	return false
}
func mask(v, channel string) string {
	if channel == "email" {
		parts := strings.Split(v, "@")
		if len(parts) == 2 && len(parts[0]) > 0 {
			return string(parts[0][0]) + "•••@" + parts[1]
		}
	}
	if len(v) > 4 {
		return "••••" + v[len(v)-4:]
	}
	return "••••"
}
