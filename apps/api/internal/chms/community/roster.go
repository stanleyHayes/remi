package community

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"remi-api/internal/chms/platform"
)

type GroupMembership struct {
	platform.ResourceEnvelope `bson:",inline"`
	GroupID                   platform.ID `json:"groupId" bson:"groupId"`
	PersonID                  platform.ID `json:"personId" bson:"personId"`
	Role                      string      `json:"role" bson:"role"`
	Status                    string      `json:"status" bson:"status"`
	Source                    string      `json:"source" bson:"source"`
	RequestedAt               *time.Time  `json:"requestedAt,omitempty" bson:"requestedAt,omitempty"`
	InvitedAt                 *time.Time  `json:"invitedAt,omitempty" bson:"invitedAt,omitempty"`
	WaitlistedAt              *time.Time  `json:"waitlistedAt,omitempty" bson:"waitlistedAt,omitempty"`
	JoinedAt                  *time.Time  `json:"joinedAt,omitempty" bson:"joinedAt,omitempty"`
	EndedAt                   *time.Time  `json:"endedAt,omitempty" bson:"endedAt,omitempty"`
	LeaderNote                string      `json:"leaderNote,omitempty" bson:"leaderNote,omitempty"`
	DirectoryVisibility       string      `json:"directoryVisibility" bson:"directoryVisibility"`
	LastReason                string      `json:"lastReason,omitempty" bson:"lastReason,omitempty"`
}

// GroupMembershipEvent is the immutable lifecycle record for a roster entry.
// Operational leader notes remain on the current staff-only projection and are
// deliberately excluded from this history payload.
type GroupMembershipEvent struct {
	ID             platform.ID    `json:"id" bson:"_id"`
	OrganizationID platform.ID    `json:"organizationId" bson:"organizationId"`
	BranchID       platform.ID    `json:"branchId" bson:"branchId"`
	GroupID        platform.ID    `json:"groupId" bson:"groupId"`
	MembershipID   platform.ID    `json:"membershipId" bson:"membershipId"`
	PersonID       platform.ID    `json:"personId" bson:"personId"`
	FromStatus     string         `json:"fromStatus,omitempty" bson:"fromStatus,omitempty"`
	ToStatus       string         `json:"toStatus" bson:"toStatus"`
	Role           string         `json:"role" bson:"role"`
	Reason         string         `json:"reason,omitempty" bson:"reason,omitempty"`
	Actor          platform.Actor `json:"actor" bson:"actor"`
	RequestID      string         `json:"requestId,omitempty" bson:"requestId,omitempty"`
	OccurredAt     time.Time      `json:"occurredAt" bson:"occurredAt"`
}

type MembershipInput struct {
	PersonID            platform.ID `json:"personId"`
	Mode                string      `json:"mode"`
	Role                string      `json:"role"`
	Source              string      `json:"source"`
	LeaderNote          string      `json:"leaderNote"`
	DirectoryVisibility string      `json:"directoryVisibility"`
}
type MembershipTransition struct {
	Status              string `json:"status"`
	Role                string `json:"role"`
	LeaderNote          string `json:"leaderNote"`
	DirectoryVisibility string `json:"directoryVisibility"`
	Reason              string `json:"reason"`
}

type GroupMeeting struct {
	platform.ResourceEnvelope `bson:",inline"`
	GroupID                   platform.ID `json:"groupId" bson:"groupId"`
	Topic                     string      `json:"topic" bson:"topic"`
	StartsAt                  time.Time   `json:"startsAt" bson:"startsAt"`
	EndsAt                    time.Time   `json:"endsAt" bson:"endsAt"`
	Timezone                  string      `json:"timezone" bson:"timezone"`
	Location                  string      `json:"location,omitempty" bson:"location,omitempty"`
	Status                    string      `json:"status" bson:"status"`
}
type MeetingInput struct {
	Topic    string    `json:"topic"`
	StartsAt time.Time `json:"startsAt"`
	EndsAt   time.Time `json:"endsAt"`
	Timezone string    `json:"timezone"`
	Location string    `json:"location"`
}
type GroupMeetingAttendance struct {
	platform.ResourceEnvelope `bson:",inline"`
	GroupID                   platform.ID `json:"groupId" bson:"groupId"`
	MeetingID                 platform.ID `json:"meetingId" bson:"meetingId"`
	PersonID                  platform.ID `json:"personId" bson:"personId"`
	Status                    string      `json:"status" bson:"status"`
	Confidence                int         `json:"confidence" bson:"confidence"`
	Reason                    string      `json:"reason,omitempty" bson:"reason,omitempty"`
}
type MeetingAttendanceInput struct {
	Status     string `json:"status"`
	Confidence int    `json:"confidence"`
	Reason     string `json:"reason"`
}

func (input *MembershipInput) normalize() error {
	input.Mode = strings.ToLower(strings.TrimSpace(input.Mode))
	input.Role = strings.ToLower(strings.TrimSpace(input.Role))
	input.Source = strings.ToLower(strings.TrimSpace(input.Source))
	input.LeaderNote = strings.TrimSpace(input.LeaderNote)
	input.DirectoryVisibility = strings.ToLower(strings.TrimSpace(input.DirectoryVisibility))
	if !input.PersonID.Valid() {
		return errors.New("person is required")
	}
	if input.Mode != "apply" && input.Mode != "invite" && input.Mode != "add" {
		return errors.New("mode must be apply, invite or add")
	}
	if input.Role == "" {
		input.Role = "member"
	}
	if input.Role != "member" && input.Role != "participant" && input.Role != "facilitator" {
		return errors.New("role must be member, participant or facilitator")
	}
	if input.DirectoryVisibility == "" {
		input.DirectoryVisibility = "hidden"
	}
	if input.DirectoryVisibility != "hidden" && input.DirectoryVisibility != "members" {
		return errors.New("directory visibility must be hidden or members")
	}
	if input.Source == "" {
		input.Source = "staff"
	}
	if len(input.Source) > 80 || len(input.LeaderNote) > 500 {
		return errors.New("membership metadata is too long")
	}
	return nil
}
func (input *MembershipTransition) normalize() error {
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	input.Role = strings.ToLower(strings.TrimSpace(input.Role))
	input.LeaderNote = strings.TrimSpace(input.LeaderNote)
	input.DirectoryVisibility = strings.ToLower(strings.TrimSpace(input.DirectoryVisibility))
	input.Reason = strings.TrimSpace(input.Reason)
	if input.Status != "active" && input.Status != "waitlisted" && input.Status != "declined" && input.Status != "ended" {
		return errors.New("status must be active, waitlisted, declined or ended")
	}
	if input.Role == "" {
		input.Role = "member"
	}
	if input.Role != "member" && input.Role != "participant" && input.Role != "facilitator" {
		return errors.New("membership role is invalid")
	}
	if input.DirectoryVisibility == "" {
		input.DirectoryVisibility = "hidden"
	}
	if input.DirectoryVisibility != "hidden" && input.DirectoryVisibility != "members" {
		return errors.New("directory visibility must be hidden or members")
	}
	if len(input.LeaderNote) > 500 || len(input.Reason) > 500 {
		return errors.New("membership metadata is too long")
	}
	if (input.Status == "declined" || input.Status == "ended") && len(input.Reason) < 3 {
		return errors.New("declining or ending membership requires a reason")
	}
	return nil
}
func (input *MeetingInput) normalize() error {
	input.Topic = strings.TrimSpace(input.Topic)
	input.Timezone = strings.TrimSpace(input.Timezone)
	input.Location = strings.TrimSpace(input.Location)
	input.StartsAt = input.StartsAt.UTC()
	input.EndsAt = input.EndsAt.UTC()
	if len(input.Topic) < 2 || len(input.Topic) > 150 {
		return errors.New("meeting topic must be 2 to 150 characters")
	}
	if _, err := time.LoadLocation(input.Timezone); err != nil {
		return errors.New("meeting timezone is invalid")
	}
	if input.StartsAt.IsZero() || !input.EndsAt.After(input.StartsAt) || input.EndsAt.Sub(input.StartsAt) > 24*time.Hour {
		return errors.New("meeting requires a valid window no longer than 24 hours")
	}
	if len(input.Location) > 250 {
		return errors.New("meeting location is too long")
	}
	return nil
}
func (input *MeetingAttendanceInput) normalize() error {
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	input.Reason = strings.TrimSpace(input.Reason)
	if input.Status != "present" && input.Status != "absent" && input.Status != "excused" {
		return errors.New("status must be present, absent or excused")
	}
	if input.Confidence == 0 {
		input.Confidence = 100
	}
	if input.Confidence < 1 || input.Confidence > 100 {
		return errors.New("confidence must be 1 to 100")
	}
	if len(input.Reason) > 500 {
		return errors.New("attendance reason is too long")
	}
	return nil
}

func (s Service) CreateMembership(ctx context.Context, principal platform.Principal, groupID platform.ID, input MembershipInput, requestID string) (*GroupMembership, error) {
	if err := input.normalize(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_membership", Message: err.Error()})
	}
	group, err := s.GetGroup(ctx, principal, groupID)
	if err != nil {
		return nil, err
	}
	if group.Status != "active" {
		return nil, &platform.DomainError{Code: "conflict", Message: "Only active groups can accept roster changes."}
	}
	if !s.canLeadGroup(principal, *group, "update") {
		return nil, &platform.DomainError{Code: "not_found", Message: "Group not found."}
	}
	if _, active, resolveErr := s.People.ResolvePersonReference(ctx, principal.OrganizationID, input.PersonID); resolveErr != nil {
		return nil, resolveErr
	} else if !active {
		return nil, platform.ValidationError(platform.FieldError{Path: "personId", Code: "invalid_person", Message: "Choose an active person."})
	}
	existing, err := s.Store.FindMembership(ctx, principal.OrganizationID, groupID, input.PersonID)
	if err != nil {
		return nil, err
	}
	if existing != nil && existing.Status != "ended" && existing.Status != "declined" {
		return nil, &platform.DomainError{Code: "conflict", Message: "This person already has a current group membership."}
	}
	now := s.now()
	status := map[string]string{"apply": "applied", "invite": "invited", "add": "active"}[input.Mode]
	if existing != nil {
		transition := MembershipTransition{Status: status, Role: input.Role, LeaderNote: input.LeaderNote, DirectoryVisibility: input.DirectoryVisibility, Reason: "Rejoined group"}
		return s.transitionMembership(ctx, principal, *group, *existing, transition, requestID, true)
	}
	membership := GroupMembership{ResourceEnvelope: platform.ResourceEnvelope{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: group.HomeBranchID, SchemaVersion: 1, Version: 1, CreatedAt: now, CreatedBy: principal.Actor, UpdatedAt: now, UpdatedBy: principal.Actor}, GroupID: groupID, PersonID: input.PersonID, Role: input.Role, Status: status, Source: input.Source, LeaderNote: input.LeaderNote, DirectoryVisibility: input.DirectoryVisibility}
	if status == "applied" {
		membership.RequestedAt = &now
	}
	if status == "invited" {
		membership.InvitedAt = &now
	}
	if status == "active" {
		membership.JoinedAt = &now
	}
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if status == "active" {
			reserved, reserveErr := s.Store.ReserveGroupSeat(tx, principal.OrganizationID, groupID, group.Capacity)
			if reserveErr != nil {
				return reserveErr
			}
			if !reserved {
				membership.Status = "waitlisted"
				membership.JoinedAt = nil
				membership.WaitlistedAt = &now
			}
		}
		if err := s.Store.InsertMembership(tx, membership); err != nil {
			return err
		}
		if err := s.Store.InsertMembershipEvent(tx, s.membershipEvent(principal, membership, "", membership.Status, "", requestID, now)); err != nil {
			return err
		}
		return s.rosterEvidence(tx, principal, *group, membership, "community.group-membership.created", requestID)
	})
	if err != nil {
		return nil, fmt.Errorf("create group membership: %w", err)
	}
	return &membership, nil
}

func (s Service) TransitionMembership(ctx context.Context, principal platform.Principal, groupID, membershipID platform.ID, expectedVersion int64, input MembershipTransition, requestID string) (*GroupMembership, error) {
	if err := platform.RequireExpectedVersion(expectedVersion); err != nil {
		return nil, err
	}
	if err := input.normalize(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_membership_transition", Message: err.Error()})
	}
	group, err := s.GetGroup(ctx, principal, groupID)
	if err != nil {
		return nil, err
	}
	membership, err := s.Store.FindMembershipByID(ctx, principal.OrganizationID, membershipID)
	if err != nil {
		return nil, err
	}
	if membership == nil || membership.GroupID != groupID || !s.canLeadGroup(principal, *group, "update") {
		return nil, &platform.DomainError{Code: "not_found", Message: "Group membership not found."}
	}
	if membership.Version != expectedVersion {
		return nil, platform.VersionConflict(expectedVersion)
	}
	return s.transitionMembership(ctx, principal, *group, *membership, input, requestID, false)
}
func (s Service) transitionMembership(ctx context.Context, principal platform.Principal, group Group, current GroupMembership, input MembershipTransition, requestID string, rejoin bool) (*GroupMembership, error) {
	if !rejoin {
		allowed := map[string]map[string]bool{"applied": {"active": true, "waitlisted": true, "declined": true}, "invited": {"active": true, "declined": true}, "waitlisted": {"active": true, "declined": true, "ended": true}, "active": {"ended": true}}
		if !allowed[current.Status][input.Status] {
			return nil, platform.ValidationError(platform.FieldError{Path: "status", Code: "invalid_transition", Message: "This membership transition is not allowed."})
		}
	}
	now := s.now()
	transition := input
	err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if input.Status == "active" && current.Status != "active" {
			reserved, reserveErr := s.Store.ReserveGroupSeat(tx, principal.OrganizationID, group.ID, group.Capacity)
			if reserveErr != nil {
				return reserveErr
			}
			if !reserved {
				transition.Status = "waitlisted"
			}
		}
		if current.Status == "active" && transition.Status != "active" {
			if err := s.Store.ReleaseGroupSeat(tx, principal.OrganizationID, group.ID); err != nil {
				return err
			}
		}
		if err := s.Store.UpdateMembership(tx, principal.OrganizationID, current.ID, current.Version, transition, now, principal.Actor); err != nil {
			return err
		}
		updated := current
		updated.Status = transition.Status
		updated.Role = transition.Role
		updated.DirectoryVisibility = transition.DirectoryVisibility
		updated.Version++
		if err := s.Store.InsertMembershipEvent(tx, s.membershipEvent(principal, updated, current.Status, transition.Status, transition.Reason, requestID, now)); err != nil {
			return err
		}
		return s.rosterEvidence(tx, principal, group, updated, "community.group-membership.transitioned", requestID)
	})
	if err != nil {
		return nil, fmt.Errorf("transition group membership: %w", err)
	}
	return s.Store.FindMembershipByID(ctx, principal.OrganizationID, current.ID)
}
func (s Service) ListMembershipHistory(ctx context.Context, principal platform.Principal, groupID, personID platform.ID) ([]GroupMembershipEvent, error) {
	group, err := s.GetGroup(ctx, principal, groupID)
	if err != nil {
		return nil, err
	}
	if !s.canLeadGroup(principal, *group, "read") {
		return nil, &platform.DomainError{Code: "not_found", Message: "Group not found."}
	}
	membership, err := s.Store.FindMembership(ctx, principal.OrganizationID, groupID, personID)
	if err != nil {
		return nil, err
	}
	if membership == nil {
		return nil, &platform.DomainError{Code: "not_found", Message: "Group membership not found."}
	}
	return s.Store.ListMembershipEvents(ctx, principal.OrganizationID, membership.ID)
}
func (s Service) ListMemberships(ctx context.Context, principal platform.Principal, groupID platform.ID, includeInactive bool) ([]GroupMembership, error) {
	group, err := s.GetGroup(ctx, principal, groupID)
	if err != nil {
		return nil, err
	}
	if !s.canLeadGroup(principal, *group, "read") {
		return nil, &platform.DomainError{Code: "not_found", Message: "Group not found."}
	}
	return s.Store.ListMemberships(ctx, principal.OrganizationID, groupID, includeInactive)
}

func (s Service) CreateMeeting(ctx context.Context, principal platform.Principal, groupID platform.ID, input MeetingInput, requestID string) (*GroupMeeting, error) {
	if err := input.normalize(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_meeting", Message: err.Error()})
	}
	group, err := s.GetGroup(ctx, principal, groupID)
	if err != nil {
		return nil, err
	}
	if !s.canLeadGroup(principal, *group, "update") {
		return nil, &platform.DomainError{Code: "not_found", Message: "Group not found."}
	}
	if group.Status != "active" {
		return nil, &platform.DomainError{Code: "conflict", Message: "Only active groups can schedule meetings."}
	}
	now := s.now()
	value := GroupMeeting{ResourceEnvelope: platform.ResourceEnvelope{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: group.HomeBranchID, SchemaVersion: 1, Version: 1, CreatedAt: now, CreatedBy: principal.Actor, UpdatedAt: now, UpdatedBy: principal.Actor}, GroupID: groupID, Topic: input.Topic, StartsAt: input.StartsAt, EndsAt: input.EndsAt, Timezone: input.Timezone, Location: input.Location, Status: "scheduled"}
	if err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.InsertMeeting(tx, value); err != nil {
			return err
		}
		return s.rosterEvidence(tx, principal, *group, value, "community.group-meeting.created", requestID)
	}); err != nil {
		return nil, fmt.Errorf("create group meeting: %w", err)
	}
	return &value, nil
}
func (s Service) membershipEvent(principal platform.Principal, membership GroupMembership, fromStatus, toStatus, reason, requestID string, occurredAt time.Time) GroupMembershipEvent {
	return GroupMembershipEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: membership.BranchID, GroupID: membership.GroupID, MembershipID: membership.ID, PersonID: membership.PersonID, FromStatus: fromStatus, ToStatus: toStatus, Role: membership.Role, Reason: reason, Actor: principal.Actor, RequestID: requestID, OccurredAt: occurredAt}
}
func (s Service) ListMeetings(ctx context.Context, principal platform.Principal, groupID platform.ID, from, to time.Time) ([]GroupMeeting, error) {
	group, err := s.GetGroup(ctx, principal, groupID)
	if err != nil {
		return nil, err
	}
	if from.IsZero() || to.IsZero() || !to.After(from) || to.Sub(from) > 366*24*time.Hour {
		return nil, platform.ValidationError(platform.FieldError{Path: "range", Code: "invalid_range", Message: "Choose a valid meeting range no longer than 366 days."})
	}
	return s.Store.ListMeetings(ctx, principal.OrganizationID, group.ID, from.UTC(), to.UTC())
}
func (s Service) RecordMeetingAttendance(ctx context.Context, principal platform.Principal, groupID, meetingID, personID platform.ID, expectedVersion int64, input MeetingAttendanceInput, requestID string) (*GroupMeetingAttendance, error) {
	if err := input.normalize(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_attendance", Message: err.Error()})
	}
	group, err := s.GetGroup(ctx, principal, groupID)
	if err != nil {
		return nil, err
	}
	meeting, err := s.Store.FindMeeting(ctx, principal.OrganizationID, meetingID)
	if err != nil {
		return nil, err
	}
	if meeting == nil || meeting.GroupID != groupID || !s.canLeadGroup(principal, *group, "update") {
		return nil, &platform.DomainError{Code: "not_found", Message: "Group meeting not found."}
	}
	membership, err := s.Store.FindMembership(ctx, principal.OrganizationID, groupID, personID)
	if err != nil {
		return nil, err
	}
	if membership == nil || membership.Status != "active" {
		return nil, platform.ValidationError(platform.FieldError{Path: "personId", Code: "not_active_member", Message: "Attendance can only be recorded for an active roster member."})
	}
	if s.now().After(meeting.EndsAt) && len(input.Reason) < 3 {
		return nil, platform.ValidationError(platform.FieldError{Path: "reason", Code: "required", Message: "Late attendance corrections require a reason."})
	}
	current, err := s.Store.FindMeetingAttendance(ctx, principal.OrganizationID, meetingID, personID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	if current == nil {
		if expectedVersion != 0 {
			return nil, platform.VersionConflict(expectedVersion)
		}
		value := GroupMeetingAttendance{ResourceEnvelope: platform.ResourceEnvelope{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: group.HomeBranchID, SchemaVersion: 1, Version: 1, CreatedAt: now, CreatedBy: principal.Actor, UpdatedAt: now, UpdatedBy: principal.Actor}, GroupID: groupID, MeetingID: meetingID, PersonID: personID, Status: input.Status, Confidence: input.Confidence, Reason: input.Reason}
		if err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
			if err := s.Store.InsertMeetingAttendance(tx, value); err != nil {
				return err
			}
			return s.rosterEvidence(tx, principal, *group, value, "community.group-attendance.recorded", requestID)
		}); err != nil {
			return nil, err
		}
		return &value, nil
	}
	if expectedVersion != current.Version {
		return nil, platform.VersionConflict(expectedVersion)
	}
	if err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.UpdateMeetingAttendance(tx, principal.OrganizationID, meetingID, personID, expectedVersion, input, now, principal.Actor); err != nil {
			return err
		}
		updated := *current
		updated.Version++
		updated.Status = input.Status
		return s.rosterEvidence(tx, principal, *group, updated, "community.group-attendance.corrected", requestID)
	}); err != nil {
		return nil, err
	}
	return s.Store.FindMeetingAttendance(ctx, principal.OrganizationID, meetingID, personID)
}
func (s Service) ListMeetingAttendance(ctx context.Context, principal platform.Principal, groupID, meetingID platform.ID) ([]GroupMeetingAttendance, error) {
	group, err := s.GetGroup(ctx, principal, groupID)
	if err != nil {
		return nil, err
	}
	meeting, err := s.Store.FindMeeting(ctx, principal.OrganizationID, meetingID)
	if err != nil {
		return nil, err
	}
	if meeting == nil || meeting.GroupID != group.ID {
		return nil, &platform.DomainError{Code: "not_found", Message: "Group meeting not found."}
	}
	return s.Store.ListMeetingAttendance(ctx, principal.OrganizationID, meetingID)
}
func (s Service) rosterEvidence(ctx context.Context, principal platform.Principal, group Group, resource any, eventType, requestID string) error {
	id := group.ID
	version := group.Version
	switch value := resource.(type) {
	case GroupMembership:
		id = value.ID
		version = value.Version
	case GroupMeeting:
		id = value.ID
		version = value.Version
	case GroupMeetingAttendance:
		id = value.ID
		version = value.Version
	}
	now := s.now()
	if err := s.Platform.AppendAudit(ctx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: group.HomeBranchID, Actor: principal.Actor, Action: eventType, ResourceType: "group", ResourceID: id, ChangedFields: []string{"status", "role", "schedule", "attendance"}, Outcome: "success", RequestID: requestID, OccurredAt: now}); err != nil {
		return err
	}
	return s.Platform.EnqueueEvent(ctx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: group.HomeBranchID, Type: eventType, EventVersion: 1, AggregateType: "group", AggregateID: group.ID, AggregateVersion: version, Actor: principal.Actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: map[string]any{"groupId": group.ID, "resourceId": id}}, State: "pending", AvailableAt: now})
}
