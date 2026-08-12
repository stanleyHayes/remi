package community

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"remi-api/internal/chms/platform"
)

type LeaderWorkspace struct {
	Groups      []LeaderGroupSummary  `json:"groups"`
	Teams       []LeaderTeamSummary   `json:"teams"`
	Assignments []VolunteerAssignment `json:"assignments"`
	GeneratedAt time.Time             `json:"generatedAt"`
}

type LeaderGroupSummary struct {
	Group            Group          `json:"group"`
	ActiveMembers    int            `json:"activeMembers"`
	PendingMembers   int            `json:"pendingMembers"`
	UpcomingMeetings []GroupMeeting `json:"upcomingMeetings"`
}

type LeaderTeamSummary struct {
	Team      VolunteerTeam       `json:"team"`
	Positions []VolunteerPosition `json:"positions"`
}

type leaderWorkspaceStore interface {
	ListGroupsLedBy(context.Context, platform.ID, platform.ID) ([]Group, error)
	ListVolunteerTeamsLedBy(context.Context, platform.ID, platform.ID) ([]VolunteerTeam, error)
	ListTeamAssignments(context.Context, platform.ID, platform.ID, time.Time, time.Time) ([]VolunteerAssignment, error)
}

type GroupCommunicationInput struct {
	Purpose string `json:"purpose"`
	Channel string `json:"channel"`
	Message string `json:"message"`
}

type GroupCommunicationHandoff struct {
	ID                platform.ID    `json:"id" bson:"_id"`
	OrganizationID    platform.ID    `json:"organizationId" bson:"organizationId"`
	BranchID          platform.ID    `json:"branchId" bson:"branchId"`
	GroupID           platform.ID    `json:"groupId" bson:"groupId"`
	AudiencePersonIDs []platform.ID  `json:"audiencePersonIds" bson:"audiencePersonIds"`
	Purpose           string         `json:"purpose" bson:"purpose"`
	Channel           string         `json:"channel" bson:"channel"`
	Message           string         `json:"message" bson:"message"`
	State             string         `json:"state" bson:"state"`
	CreatedBy         platform.Actor `json:"createdBy" bson:"createdBy"`
	CreatedAt         time.Time      `json:"createdAt" bson:"createdAt"`
}

type leaderCommunicationStore interface {
	InsertCommunicationHandoff(context.Context, GroupCommunicationHandoff) error
}

func (input *GroupCommunicationInput) normalize() error {
	input.Purpose, input.Channel, input.Message = strings.ToLower(strings.TrimSpace(input.Purpose)), strings.ToLower(strings.TrimSpace(input.Channel)), strings.TrimSpace(input.Message)
	if input.Purpose != "group-operations" && input.Purpose != "church-updates" {
		return errors.New("purpose must be group-operations or church-updates")
	}
	if input.Channel != "email" && input.Channel != "sms" && input.Channel != "whatsapp" {
		return errors.New("channel must be email, sms or whatsapp")
	}
	if len(input.Message) < 10 || len(input.Message) > 2000 {
		return errors.New("message must be 10 to 2000 characters")
	}
	return nil
}

// RequestGroupCommunication creates a consent-gated handoff, never a direct
// send. Destinations are intentionally absent; the communications worker must
// resolve current consent, suppressions and verified contact data at execution.
func (s Service) RequestGroupCommunication(ctx context.Context, principal platform.Principal, groupID platform.ID, input GroupCommunicationInput, requestID string) (*GroupCommunicationHandoff, error) {
	if err := input.normalize(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_communication", Message: err.Error()})
	}
	group, err := s.GetGroup(ctx, principal, groupID)
	if err != nil {
		return nil, err
	}
	if !s.canLeadGroup(principal, *group, "update") {
		return nil, &platform.DomainError{Code: "not_found", Message: "Group not found."}
	}
	store, ok := s.Store.(leaderCommunicationStore)
	if !ok {
		return nil, fmt.Errorf("leader communication store is unavailable")
	}
	members, err := s.Store.ListMemberships(ctx, principal.OrganizationID, groupID, false)
	if err != nil {
		return nil, err
	}
	audience := []platform.ID{}
	for _, member := range members {
		if member.Status == "active" {
			audience = append(audience, member.PersonID)
		}
	}
	sort.Slice(audience, func(i, j int) bool { return audience[i] < audience[j] })
	now := s.now()
	value := GroupCommunicationHandoff{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: group.HomeBranchID, GroupID: groupID, AudiencePersonIDs: audience, Purpose: input.Purpose, Channel: input.Channel, Message: input.Message, State: "pending-consent-review", CreatedBy: principal.Actor, CreatedAt: now}
	if err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if saveErr := store.InsertCommunicationHandoff(tx, value); saveErr != nil {
			return saveErr
		}
		if auditErr := s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: group.HomeBranchID, Actor: principal.Actor, Action: "community.group.communication-requested", ResourceType: "group", ResourceID: groupID, ChangedFields: []string{"purpose", "channel", "audience", "state"}, Outcome: "success", RequestID: requestID, OccurredAt: now}); auditErr != nil {
			return auditErr
		}
		return s.Platform.EnqueueEvent(tx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: group.HomeBranchID, Type: "community.group.communication-requested", EventVersion: 1, AggregateType: "communication-handoff", AggregateID: value.ID, AggregateVersion: 1, Actor: principal.Actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: map[string]any{"handoffId": value.ID, "groupId": groupID, "purpose": value.Purpose, "channel": value.Channel, "audienceCount": len(audience)}}, State: "pending", AvailableAt: now})
	}); err != nil {
		return nil, fmt.Errorf("request group communication: %w", err)
	}
	return &value, nil
}

// GetLeaderWorkspace is ownership-scoped by construction. It does not grant a
// branch-wide directory read and deliberately excludes leader notes, screening
// metadata and contact details from its aggregate projection.
func (s Service) GetLeaderWorkspace(ctx context.Context, principal platform.Principal, from, to time.Time) (*LeaderWorkspace, error) {
	if principal.Actor.Type != platform.ActorMember || !principal.Actor.ID.Valid() {
		return nil, &platform.DomainError{Code: "forbidden", Message: "A linked member identity is required for the leader workspace."}
	}
	from, to = from.UTC(), to.UTC()
	if from.IsZero() || to.IsZero() || !to.After(from) || to.Sub(from) > 366*24*time.Hour {
		return nil, platform.ValidationError(platform.FieldError{Path: "range", Code: "invalid_range", Message: "Choose a valid workspace range no longer than 366 days."})
	}
	store, ok := s.Store.(leaderWorkspaceStore)
	if !ok {
		return nil, fmt.Errorf("leader workspace store is unavailable")
	}
	groups, err := store.ListGroupsLedBy(ctx, principal.OrganizationID, principal.Actor.ID)
	if err != nil {
		return nil, err
	}
	teams, err := store.ListVolunteerTeamsLedBy(ctx, principal.OrganizationID, principal.Actor.ID)
	if err != nil {
		return nil, err
	}
	result := &LeaderWorkspace{Groups: []LeaderGroupSummary{}, Teams: []LeaderTeamSummary{}, Assignments: []VolunteerAssignment{}, GeneratedAt: s.now()}
	for _, group := range groups {
		members, loadErr := s.Store.ListMemberships(ctx, principal.OrganizationID, group.ID, false)
		if loadErr != nil {
			return nil, loadErr
		}
		meetings, loadErr := s.Store.ListMeetings(ctx, principal.OrganizationID, group.ID, from, to)
		if loadErr != nil {
			return nil, loadErr
		}
		summary := LeaderGroupSummary{Group: group, UpcomingMeetings: meetings}
		for _, member := range members {
			switch member.Status {
			case "active":
				summary.ActiveMembers++
			case "applied", "invited", "waitlisted":
				summary.PendingMembers++
			}
		}
		result.Groups = append(result.Groups, summary)
	}
	for _, team := range teams {
		positions, loadErr := s.Store.ListVolunteerPositions(ctx, principal.OrganizationID, team.ID, false)
		if loadErr != nil {
			return nil, loadErr
		}
		result.Teams = append(result.Teams, LeaderTeamSummary{Team: team, Positions: positions})
	}
	result.Assignments, err = s.Store.ListPersonAssignments(ctx, principal.OrganizationID, principal.Actor.ID, from, to)
	if err != nil {
		return nil, err
	}
	sort.Slice(result.Groups, func(i, j int) bool { return result.Groups[i].Group.Name < result.Groups[j].Group.Name })
	sort.Slice(result.Teams, func(i, j int) bool { return result.Teams[i].Team.Name < result.Teams[j].Team.Name })
	return result, nil
}

func (s Service) GetOwnedLeaderGroup(ctx context.Context, principal platform.Principal, groupID platform.ID) (*Group, error) {
	if principal.Actor.Type != platform.ActorMember || !principal.Actor.ID.Valid() {
		return nil, &platform.DomainError{Code: "not_found", Message: "Group not found."}
	}
	group, err := s.Store.FindGroup(ctx, principal.OrganizationID, groupID)
	if err != nil {
		return nil, err
	}
	if group == nil || !ownsID(group.LeaderPersonIDs, principal.Actor.ID) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Group not found."}
	}
	return group, nil
}

func (s Service) GetOwnedLeaderTeam(ctx context.Context, principal platform.Principal, teamID platform.ID) (*VolunteerTeam, error) {
	if principal.Actor.Type != platform.ActorMember || !principal.Actor.ID.Valid() {
		return nil, &platform.DomainError{Code: "not_found", Message: "Volunteer team not found."}
	}
	team, err := s.Store.FindVolunteerTeam(ctx, principal.OrganizationID, teamID)
	if err != nil {
		return nil, err
	}
	if team == nil || !ownsID(team.LeaderPersonIDs, principal.Actor.ID) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Volunteer team not found."}
	}
	return team, nil
}

func (s Service) ListOwnedLeaderTeamAssignments(ctx context.Context, principal platform.Principal, teamID platform.ID, from, to time.Time) ([]VolunteerAssignment, error) {
	if _, err := s.GetOwnedLeaderTeam(ctx, principal, teamID); err != nil {
		return nil, err
	}
	from, to = from.UTC(), to.UTC()
	if from.IsZero() || to.IsZero() || !to.After(from) || to.Sub(from) > 366*24*time.Hour {
		return nil, platform.ValidationError(platform.FieldError{Path: "range", Code: "invalid_range", Message: "Choose a valid schedule range no longer than 366 days."})
	}
	store, ok := s.Store.(leaderWorkspaceStore)
	if !ok {
		return nil, fmt.Errorf("leader workspace store is unavailable")
	}
	return store.ListTeamAssignments(ctx, principal.OrganizationID, teamID, from, to)
}

func ownsID(ids []platform.ID, id platform.ID) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}

func (s Service) canLeadGroup(principal platform.Principal, group Group, action string) bool {
	return s.allowed(principal, action, group.HomeBranchID, group.MinistryID) || (principal.Actor.Type == platform.ActorMember && ownsID(group.LeaderPersonIDs, principal.Actor.ID))
}

func (s Service) canLeadTeam(principal platform.Principal, team VolunteerTeam, action string) bool {
	return s.allowedVolunteer(principal, action, team.HomeBranchID, platform.FieldOperational) || (principal.Actor.Type == platform.ActorMember && ownsID(team.LeaderPersonIDs, principal.Actor.ID))
}
