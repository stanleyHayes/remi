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

type VolunteerTeam struct {
	platform.ResourceEnvelope `bson:",inline"`
	Name                      string        `json:"name" bson:"name"`
	Description               string        `json:"description,omitempty" bson:"description,omitempty"`
	HomeBranchID              platform.ID   `json:"homeBranchId" bson:"homeBranchId"`
	LeaderPersonIDs           []platform.ID `json:"leaderPersonIds" bson:"leaderPersonIds"`
	Status                    string        `json:"status" bson:"status"`
}

type VolunteerTeamInput struct {
	Name            string        `json:"name"`
	Description     string        `json:"description"`
	HomeBranchID    platform.ID   `json:"homeBranchId"`
	LeaderPersonIDs []platform.ID `json:"leaderPersonIds"`
	Status          string        `json:"status"`
}

type PositionEligibility struct {
	MinimumAgeYears              int      `json:"minimumAgeYears,omitempty" bson:"minimumAgeYears,omitempty"`
	MembershipStages             []string `json:"membershipStages" bson:"membershipStages"`
	RequiredSkills               []string `json:"requiredSkills" bson:"requiredSkills"`
	BackgroundCheckRequired      bool     `json:"backgroundCheckRequired" bson:"backgroundCheckRequired"`
	BackgroundCheckMaxAgeDays    int      `json:"backgroundCheckMaxAgeDays,omitempty" bson:"backgroundCheckMaxAgeDays,omitempty"`
	SafeguardingTrainingRequired bool     `json:"safeguardingTrainingRequired" bson:"safeguardingTrainingRequired"`
}

type VolunteerPosition struct {
	platform.ResourceEnvelope `bson:",inline"`
	TeamID                    platform.ID         `json:"teamId" bson:"teamId"`
	Name                      string              `json:"name" bson:"name"`
	Description               string              `json:"description,omitempty" bson:"description,omitempty"`
	Eligibility               PositionEligibility `json:"eligibility" bson:"eligibility"`
	Status                    string              `json:"status" bson:"status"`
}

type VolunteerPositionInput struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Eligibility PositionEligibility `json:"eligibility"`
	Status      string              `json:"status"`
}

// ScreeningMetadata deliberately stores no report, narrative, identity
// document or adjudication detail. It is only scheduling eligibility metadata.
type ScreeningMetadata struct {
	BackgroundCheckStatus         string     `json:"backgroundCheckStatus" bson:"backgroundCheckStatus"`
	BackgroundCheckedAt           *time.Time `json:"backgroundCheckedAt,omitempty" bson:"backgroundCheckedAt,omitempty"`
	BackgroundCheckExpiresAt      *time.Time `json:"backgroundCheckExpiresAt,omitempty" bson:"backgroundCheckExpiresAt,omitempty"`
	BackgroundCheckReference      string     `json:"backgroundCheckReference,omitempty" bson:"backgroundCheckReference,omitempty"`
	SafeguardingTrainingAt        *time.Time `json:"safeguardingTrainingAt,omitempty" bson:"safeguardingTrainingAt,omitempty"`
	SafeguardingTrainingExpiresAt *time.Time `json:"safeguardingTrainingExpiresAt,omitempty" bson:"safeguardingTrainingExpiresAt,omitempty"`
}

type VolunteerProfile struct {
	platform.ResourceEnvelope `bson:",inline"`
	PersonID                  platform.ID       `json:"personId" bson:"personId"`
	Skills                    []string          `json:"skills" bson:"skills"`
	PreferredTeamIDs          []platform.ID     `json:"preferredTeamIds" bson:"preferredTeamIds"`
	PreferredPositionIDs      []platform.ID     `json:"preferredPositionIds" bson:"preferredPositionIds"`
	Status                    string            `json:"status" bson:"status"`
	Eligibility               ScreeningMetadata `json:"eligibility" bson:"eligibility"`
}

type VolunteerProfileInput struct {
	Skills               []string          `json:"skills"`
	PreferredTeamIDs     []platform.ID     `json:"preferredTeamIds"`
	PreferredPositionIDs []platform.ID     `json:"preferredPositionIds"`
	Status               string            `json:"status"`
	Eligibility          ScreeningMetadata `json:"eligibility"`
}

type AvailabilityWindow struct {
	platform.ResourceEnvelope `bson:",inline"`
	PersonID                  platform.ID `json:"personId" bson:"personId"`
	StartsAt                  time.Time   `json:"startsAt" bson:"startsAt"`
	EndsAt                    time.Time   `json:"endsAt" bson:"endsAt"`
	State                     string      `json:"state" bson:"state"`
	Source                    string      `json:"source" bson:"source"`
	SourceKey                 string      `json:"sourceKey" bson:"sourceKey"`
}

type AvailabilityInput struct {
	StartsAt time.Time `json:"startsAt"`
	EndsAt   time.Time `json:"endsAt"`
	State    string    `json:"state"`
	Source   string    `json:"source"`
}

func (input *VolunteerTeamInput) normalize() error {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	input.LeaderPersonIDs = normalizeIDs(input.LeaderPersonIDs)
	if !input.HomeBranchID.Valid() {
		return errors.New("home branch is required")
	}
	if len(input.Name) < 2 || len(input.Name) > 150 || len(input.Description) > 2000 {
		return errors.New("team name or description is invalid")
	}
	if len(input.LeaderPersonIDs) > 10 {
		return errors.New("a team can have at most 10 leaders")
	}
	if input.Status == "" {
		input.Status = "active"
	}
	if input.Status != "active" && input.Status != "inactive" {
		return errors.New("team status must be active or inactive")
	}
	return nil
}

func (input *VolunteerPositionInput) normalize() error {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	if len(input.Name) < 2 || len(input.Name) > 150 || len(input.Description) > 2000 {
		return errors.New("position name or description is invalid")
	}
	if input.Status == "" {
		input.Status = "active"
	}
	if input.Status != "active" && input.Status != "inactive" {
		return errors.New("position status must be active or inactive")
	}
	input.Eligibility.RequiredSkills = normalizeSlugs(input.Eligibility.RequiredSkills, 30)
	input.Eligibility.MembershipStages = normalizeSlugs(input.Eligibility.MembershipStages, 20)
	if input.Eligibility.MinimumAgeYears < 0 || input.Eligibility.MinimumAgeYears > 100 {
		return errors.New("minimum age must be between 0 and 100")
	}
	if input.Eligibility.BackgroundCheckMaxAgeDays < 0 || input.Eligibility.BackgroundCheckMaxAgeDays > 3650 {
		return errors.New("background-check maximum age must be between 0 and 3650 days")
	}
	if input.Eligibility.BackgroundCheckRequired && input.Eligibility.BackgroundCheckMaxAgeDays == 0 {
		return errors.New("a required background check needs a maximum age")
	}
	return nil
}

func (input *VolunteerProfileInput) normalize() error {
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	if input.Status == "" {
		input.Status = "active"
	}
	if input.Status != "active" && input.Status != "paused" && input.Status != "inactive" {
		return errors.New("volunteer status must be active, paused or inactive")
	}
	input.Skills = normalizeSlugs(input.Skills, 50)
	input.PreferredTeamIDs = normalizeIDs(input.PreferredTeamIDs)
	input.PreferredPositionIDs = normalizeIDs(input.PreferredPositionIDs)
	if len(input.PreferredTeamIDs) > 20 || len(input.PreferredPositionIDs) > 40 {
		return errors.New("too many serving preferences")
	}
	status := strings.ToLower(strings.TrimSpace(input.Eligibility.BackgroundCheckStatus))
	if status == "" {
		status = "not-required"
	}
	if status != "not-required" && status != "pending" && status != "cleared" && status != "expired" && status != "restricted" {
		return errors.New("background-check status is invalid")
	}
	input.Eligibility.BackgroundCheckStatus = status
	input.Eligibility.BackgroundCheckReference = strings.TrimSpace(input.Eligibility.BackgroundCheckReference)
	if len(input.Eligibility.BackgroundCheckReference) > 120 {
		return errors.New("background-check reference is too long")
	}
	if input.Eligibility.BackgroundCheckExpiresAt != nil && input.Eligibility.BackgroundCheckedAt != nil && !input.Eligibility.BackgroundCheckExpiresAt.After(*input.Eligibility.BackgroundCheckedAt) {
		return errors.New("background-check expiry must follow the check date")
	}
	if status == "cleared" && (input.Eligibility.BackgroundCheckedAt == nil || input.Eligibility.BackgroundCheckExpiresAt == nil || input.Eligibility.BackgroundCheckReference == "") {
		return errors.New("cleared background checks require checked, expiry and reference metadata")
	}
	if input.Eligibility.SafeguardingTrainingExpiresAt != nil && input.Eligibility.SafeguardingTrainingAt != nil && !input.Eligibility.SafeguardingTrainingExpiresAt.After(*input.Eligibility.SafeguardingTrainingAt) {
		return errors.New("training expiry must follow the training date")
	}
	return nil
}

func (input *AvailabilityInput) normalize() error {
	input.StartsAt = input.StartsAt.UTC()
	input.EndsAt = input.EndsAt.UTC()
	input.State = strings.ToLower(strings.TrimSpace(input.State))
	input.Source = strings.ToLower(strings.TrimSpace(input.Source))
	if input.StartsAt.IsZero() || !input.EndsAt.After(input.StartsAt) || input.EndsAt.Sub(input.StartsAt) > 366*24*time.Hour {
		return errors.New("availability requires a valid window no longer than 366 days")
	}
	if input.State != "available" && input.State != "preferred" && input.State != "unavailable" {
		return errors.New("availability state must be available, preferred or unavailable")
	}
	if input.Source == "" {
		input.Source = "staff"
	}
	if input.Source != "staff" && input.Source != "member" && input.Source != "import" {
		return errors.New("availability source is invalid")
	}
	return nil
}

func normalizeSlugs(values []string, limit int) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, raw := range values {
		value := strings.ToLower(strings.TrimSpace(raw))
		value = strings.Join(strings.Fields(value), "-")
		if value == "" || len(value) > 60 || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	if len(result) > limit {
		return result[:limit]
	}
	return result
}

func (s Service) CreateVolunteerTeam(ctx context.Context, principal platform.Principal, input VolunteerTeamInput, requestID string) (*VolunteerTeam, error) {
	if err := input.normalize(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_team", Message: err.Error()})
	}
	if !s.allowedVolunteer(principal, "create", input.HomeBranchID, platform.FieldOperational) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot create volunteer teams in this branch."}
	}
	if err := s.validateLeaders(ctx, principal.OrganizationID, input.LeaderPersonIDs); err != nil {
		return nil, err
	}
	now := s.now()
	value := VolunteerTeam{ResourceEnvelope: platform.ResourceEnvelope{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: input.HomeBranchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: principal.Actor, UpdatedAt: now, UpdatedBy: principal.Actor}, Name: input.Name, Description: input.Description, HomeBranchID: input.HomeBranchID, LeaderPersonIDs: input.LeaderPersonIDs, Status: input.Status}
	if err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.InsertVolunteerTeam(tx, value); err != nil {
			return err
		}
		return s.volunteerEvidence(tx, principal, value.HomeBranchID, value.ID, value.Version, "community.volunteer-team.created", requestID)
	}); err != nil {
		return nil, fmt.Errorf("create volunteer team: %w", err)
	}
	return &value, nil
}

func (s Service) ListVolunteerTeams(ctx context.Context, principal platform.Principal, branchID platform.ID, includeInactive bool) ([]VolunteerTeam, error) {
	if !branchID.Valid() {
		return nil, platform.ValidationError(platform.FieldError{Path: "branchId", Code: "required", Message: "Choose a branch."})
	}
	if !s.allowedVolunteer(principal, "read", branchID, platform.FieldOperational) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot read volunteer teams in this branch."}
	}
	return s.Store.ListVolunteerTeams(ctx, principal.OrganizationID, branchID, includeInactive)
}

func (s Service) GetVolunteerTeam(ctx context.Context, principal platform.Principal, id platform.ID) (*VolunteerTeam, error) {
	value, err := s.Store.FindVolunteerTeam(ctx, principal.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if value == nil || !s.canLeadTeam(principal, *value, "read") {
		return nil, &platform.DomainError{Code: "not_found", Message: "Volunteer team not found."}
	}
	return value, nil
}

func (s Service) UpdateVolunteerTeam(ctx context.Context, principal platform.Principal, id platform.ID, expectedVersion int64, input VolunteerTeamInput, requestID string) (*VolunteerTeam, error) {
	if err := platform.RequireExpectedVersion(expectedVersion); err != nil {
		return nil, err
	}
	if err := input.normalize(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_team", Message: err.Error()})
	}
	current, err := s.Store.FindVolunteerTeam(ctx, principal.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if current == nil || !s.allowedVolunteer(principal, "update", current.HomeBranchID, platform.FieldOperational) || !s.allowedVolunteer(principal, "update", input.HomeBranchID, platform.FieldOperational) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Volunteer team not found."}
	}
	if err = s.validateLeaders(ctx, principal.OrganizationID, input.LeaderPersonIDs); err != nil {
		return nil, err
	}
	now := s.now()
	if err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.UpdateVolunteerTeam(tx, principal.OrganizationID, id, expectedVersion, input, now, principal.Actor); err != nil {
			return err
		}
		return s.volunteerEvidence(tx, principal, input.HomeBranchID, id, expectedVersion+1, "community.volunteer-team.updated", requestID)
	}); err != nil {
		return nil, fmt.Errorf("update volunteer team: %w", err)
	}
	return s.Store.FindVolunteerTeam(ctx, principal.OrganizationID, id)
}

func (s Service) CreateVolunteerPosition(ctx context.Context, principal platform.Principal, teamID platform.ID, input VolunteerPositionInput, requestID string) (*VolunteerPosition, error) {
	if err := input.normalize(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_position", Message: err.Error()})
	}
	team, err := s.Store.FindVolunteerTeam(ctx, principal.OrganizationID, teamID)
	if err != nil {
		return nil, err
	}
	if team == nil || !s.canLeadTeam(principal, *team, "update") {
		return nil, &platform.DomainError{Code: "not_found", Message: "Volunteer team not found."}
	}
	now := s.now()
	value := VolunteerPosition{ResourceEnvelope: platform.ResourceEnvelope{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: team.HomeBranchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: principal.Actor, UpdatedAt: now, UpdatedBy: principal.Actor}, TeamID: teamID, Name: input.Name, Description: input.Description, Eligibility: input.Eligibility, Status: input.Status}
	if err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.InsertVolunteerPosition(tx, value); err != nil {
			return err
		}
		return s.volunteerEvidence(tx, principal, value.BranchID, value.ID, value.Version, "community.volunteer-position.created", requestID)
	}); err != nil {
		return nil, fmt.Errorf("create volunteer position: %w", err)
	}
	return &value, nil
}

func (s Service) ListVolunteerPositions(ctx context.Context, principal platform.Principal, teamID platform.ID, includeInactive bool) ([]VolunteerPosition, error) {
	team, err := s.Store.FindVolunteerTeam(ctx, principal.OrganizationID, teamID)
	if err != nil {
		return nil, err
	}
	if team == nil || !s.canLeadTeam(principal, *team, "read") {
		return nil, &platform.DomainError{Code: "not_found", Message: "Volunteer team not found."}
	}
	return s.Store.ListVolunteerPositions(ctx, principal.OrganizationID, teamID, includeInactive)
}

func (s Service) UpdateVolunteerPosition(ctx context.Context, principal platform.Principal, teamID, positionID platform.ID, expectedVersion int64, input VolunteerPositionInput, requestID string) (*VolunteerPosition, error) {
	if err := platform.RequireExpectedVersion(expectedVersion); err != nil {
		return nil, err
	}
	if err := input.normalize(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_position", Message: err.Error()})
	}
	team, err := s.Store.FindVolunteerTeam(ctx, principal.OrganizationID, teamID)
	if err != nil {
		return nil, err
	}
	current, err := s.Store.FindVolunteerPosition(ctx, principal.OrganizationID, positionID)
	if err != nil {
		return nil, err
	}
	if team == nil || current == nil || current.TeamID != teamID || !s.canLeadTeam(principal, *team, "update") {
		return nil, &platform.DomainError{Code: "not_found", Message: "Volunteer position not found."}
	}
	now := s.now()
	if err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.UpdateVolunteerPosition(tx, principal.OrganizationID, positionID, expectedVersion, input, now, principal.Actor); err != nil {
			return err
		}
		return s.volunteerEvidence(tx, principal, team.HomeBranchID, positionID, expectedVersion+1, "community.volunteer-position.updated", requestID)
	}); err != nil {
		return nil, fmt.Errorf("update volunteer position: %w", err)
	}
	return s.Store.FindVolunteerPosition(ctx, principal.OrganizationID, positionID)
}

func (s Service) PutVolunteerProfile(ctx context.Context, principal platform.Principal, personID platform.ID, expectedVersion int64, input VolunteerProfileInput, requestID string) (*VolunteerProfile, error) {
	if err := input.normalize(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_volunteer_profile", Message: err.Error()})
	}
	branchID, active, err := s.People.ResolvePersonReference(ctx, principal.OrganizationID, personID)
	if err != nil {
		return nil, err
	}
	if !active || !s.allowedVolunteer(principal, "update", branchID, platform.FieldSensitiveMinistry) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Volunteer profile not found."}
	}
	current, err := s.Store.FindVolunteerProfile(ctx, principal.OrganizationID, personID)
	if err != nil {
		return nil, err
	}
	if (current == nil && expectedVersion != 0) || (current != nil && current.Version != expectedVersion) {
		return nil, platform.VersionConflict(expectedVersion)
	}
	for _, positionID := range input.PreferredPositionIDs {
		position, findErr := s.Store.FindVolunteerPosition(ctx, principal.OrganizationID, positionID)
		if findErr != nil {
			return nil, findErr
		}
		if position == nil || position.BranchID != branchID {
			return nil, platform.ValidationError(platform.FieldError{Path: "preferredPositionIds", Code: "invalid_position", Message: "Every preferred position must belong to the volunteer's branch."})
		}
	}
	for _, teamID := range input.PreferredTeamIDs {
		team, findErr := s.Store.FindVolunteerTeam(ctx, principal.OrganizationID, teamID)
		if findErr != nil {
			return nil, findErr
		}
		if team == nil || team.HomeBranchID != branchID {
			return nil, platform.ValidationError(platform.FieldError{Path: "preferredTeamIds", Code: "invalid_team", Message: "Every preferred team must belong to the volunteer's branch."})
		}
	}
	now := s.now()
	value := VolunteerProfile{ResourceEnvelope: platform.ResourceEnvelope{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: branchID, SchemaVersion: CurrentSchemaVersion, Version: expectedVersion + 1, CreatedAt: now, CreatedBy: principal.Actor, UpdatedAt: now, UpdatedBy: principal.Actor}, PersonID: personID, Skills: input.Skills, PreferredTeamIDs: input.PreferredTeamIDs, PreferredPositionIDs: input.PreferredPositionIDs, Status: input.Status, Eligibility: input.Eligibility}
	if current != nil {
		value.ID, value.CreatedAt, value.CreatedBy = current.ID, current.CreatedAt, current.CreatedBy
	}
	if err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.UpsertVolunteerProfile(tx, value, expectedVersion); err != nil {
			return err
		}
		return s.volunteerEvidence(tx, principal, branchID, value.ID, value.Version, "community.volunteer-profile.updated", requestID)
	}); err != nil {
		return nil, fmt.Errorf("put volunteer profile: %w", err)
	}
	return s.Store.FindVolunteerProfile(ctx, principal.OrganizationID, personID)
}

func (s Service) GetVolunteerProfile(ctx context.Context, principal platform.Principal, personID platform.ID) (*VolunteerProfile, error) {
	branchID, active, err := s.People.ResolvePersonReference(ctx, principal.OrganizationID, personID)
	if err != nil {
		return nil, err
	}
	if !active || !s.allowedVolunteer(principal, "read", branchID, platform.FieldSensitiveMinistry) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Volunteer profile not found."}
	}
	value, err := s.Store.FindVolunteerProfile(ctx, principal.OrganizationID, personID)
	if err != nil {
		return nil, err
	}
	if value == nil {
		return nil, &platform.DomainError{Code: "not_found", Message: "Volunteer profile not found."}
	}
	return value, nil
}

func (s Service) AddAvailability(ctx context.Context, principal platform.Principal, personID platform.ID, input AvailabilityInput, requestID, sourceKey string) (*AvailabilityWindow, error) {
	if err := input.normalize(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_availability", Message: err.Error()})
	}
	branchID, active, err := s.People.ResolvePersonReference(ctx, principal.OrganizationID, personID)
	if err != nil {
		return nil, err
	}
	if !active || !s.allowedVolunteer(principal, "update", branchID, platform.FieldOperational) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Volunteer not found."}
	}
	now := s.now()
	value := AvailabilityWindow{ResourceEnvelope: platform.ResourceEnvelope{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: branchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: principal.Actor, UpdatedAt: now, UpdatedBy: principal.Actor}, PersonID: personID, StartsAt: input.StartsAt, EndsAt: input.EndsAt, State: input.State, Source: input.Source, SourceKey: strings.TrimSpace(sourceKey)}
	if value.SourceKey == "" {
		value.SourceKey = string(value.ID)
	}
	if existing, findErr := s.Store.FindAvailabilityBySourceKey(ctx, principal.OrganizationID, personID, value.SourceKey); findErr != nil {
		return nil, findErr
	} else if existing != nil {
		if !sameAvailability(*existing, value) {
			return nil, &platform.DomainError{Code: "idempotency_key_reused", Message: "This idempotency key was already used for different availability data."}
		}
		return existing, nil
	}
	if err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.InsertAvailability(tx, value); err != nil {
			return err
		}
		return s.volunteerEvidence(tx, principal, branchID, value.ID, value.Version, "community.volunteer-availability.created", requestID)
	}); err != nil {
		if existing, findErr := s.Store.FindAvailabilityBySourceKey(ctx, principal.OrganizationID, personID, value.SourceKey); findErr == nil && existing != nil {
			return existing, nil
		}
		return nil, fmt.Errorf("add volunteer availability: %w", err)
	}
	return &value, nil
}

func sameAvailability(left, right AvailabilityWindow) bool {
	return left.PersonID == right.PersonID && left.StartsAt.Equal(right.StartsAt) && left.EndsAt.Equal(right.EndsAt) && left.State == right.State && left.Source == right.Source
}

func (s Service) ListAvailability(ctx context.Context, principal platform.Principal, personID platform.ID, from, to time.Time) ([]AvailabilityWindow, error) {
	branchID, active, err := s.People.ResolvePersonReference(ctx, principal.OrganizationID, personID)
	if err != nil {
		return nil, err
	}
	if !active || !s.allowedVolunteer(principal, "read", branchID, platform.FieldOperational) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Volunteer not found."}
	}
	if from.IsZero() || to.IsZero() || !to.After(from) || to.Sub(from) > 2*366*24*time.Hour {
		return nil, platform.ValidationError(platform.FieldError{Path: "range", Code: "invalid_range", Message: "Choose a valid availability range no longer than two years."})
	}
	return s.Store.ListAvailability(ctx, principal.OrganizationID, personID, from.UTC(), to.UTC())
}

func (s Service) allowedVolunteer(principal platform.Principal, action string, branchID platform.ID, fields ...platform.FieldClass) bool {
	return s.Authorizer.Authorize(principal, platform.AccessRequest{Action: action, ResourceType: "volunteer", OrganizationID: principal.OrganizationID, BranchID: branchID, FieldClasses: fields, Now: s.now()}).Allowed
}

func (s Service) volunteerEvidence(ctx context.Context, principal platform.Principal, branchID, resourceID platform.ID, version int64, eventType, requestID string) error {
	now := s.now()
	if err := s.Platform.AppendAudit(ctx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: branchID, Actor: principal.Actor, Action: eventType, ResourceType: "volunteer", ResourceID: resourceID, ChangedFields: []string{"team", "position", "skills", "eligibility", "availability", "preferences", "status"}, Outcome: "success", RequestID: requestID, OccurredAt: now}); err != nil {
		return err
	}
	return s.Platform.EnqueueEvent(ctx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: branchID, Type: eventType, EventVersion: 1, AggregateType: "volunteer", AggregateID: resourceID, AggregateVersion: version, Actor: principal.Actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: map[string]any{"resourceId": resourceID}}, State: "pending", AvailableAt: now})
}
