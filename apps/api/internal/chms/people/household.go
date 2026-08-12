package people

import (
	"errors"
	"strings"
	"time"

	"remi-api/internal/chms/platform"
)

type Household struct {
	platform.ResourceEnvelope `bson:",inline"`
	Name                      string         `json:"name" bson:"name"`
	HomeBranchID              platform.ID    `json:"homeBranchId" bson:"homeBranchId"`
	SharedContactPoints       []ContactPoint `json:"sharedContactPoints" bson:"sharedContactPoints"`
	SharedAddresses           []Address      `json:"sharedAddresses" bson:"sharedAddresses"`
	PrimaryContactPersonID    platform.ID    `json:"primaryContactPersonId,omitempty" bson:"primaryContactPersonId,omitempty"`
	StatementPreference       string         `json:"statementPreference" bson:"statementPreference"`
}

type HouseholdMembership struct {
	ID             platform.ID    `json:"id" bson:"_id"`
	OrganizationID platform.ID    `json:"organizationId" bson:"organizationId"`
	HouseholdID    platform.ID    `json:"householdId" bson:"householdId"`
	PersonID       platform.ID    `json:"personId" bson:"personId"`
	Role           string         `json:"role" bson:"role"`
	StartedAt      time.Time      `json:"startedAt" bson:"startedAt"`
	EndedAt        *time.Time     `json:"endedAt,omitempty" bson:"endedAt,omitempty"`
	EndReason      string         `json:"-" bson:"endReason,omitempty"`
	CreatedAt      time.Time      `json:"createdAt" bson:"createdAt"`
	CreatedBy      platform.Actor `json:"createdBy" bson:"createdBy"`
}

type HouseholdProfileMember struct {
	ID           platform.ID `json:"id"`
	Name         string      `json:"name"`
	Role         string      `json:"role,omitempty"`
	Relationship string      `json:"relationship,omitempty"`
	PhotoAssetID platform.ID `json:"photoAssetId,omitempty"`
}

type HouseholdProfile struct {
	ID               platform.ID              `json:"id"`
	Name             string                   `json:"name"`
	PrimaryContactID platform.ID              `json:"primaryContactId,omitempty"`
	Members          []HouseholdProfileMember `json:"members"`
	SharedAddress    string                   `json:"sharedAddress,omitempty"`
}

// Relationship is directional. Display labels for the inverse direction are
// configuration, not a duplicated record that can drift.
type Relationship struct {
	platform.ResourceEnvelope `bson:",inline"`
	FromPersonID              platform.ID `json:"fromPersonId" bson:"fromPersonId"`
	ToPersonID                platform.ID `json:"toPersonId" bson:"toPersonId"`
	Type                      string      `json:"type" bson:"type"`
	HouseholdID               platform.ID `json:"householdId,omitempty" bson:"householdId,omitempty"`
	StartedOn                 *string     `json:"startedOn,omitempty" bson:"startedOn,omitempty"`
	EndedAt                   *time.Time  `json:"endedAt,omitempty" bson:"endedAt,omitempty"`
	Visibility                string      `json:"visibility" bson:"visibility"`
	Source                    Source      `json:"source" bson:"source"`
}

type CreateHouseholdInput struct {
	OrganizationID         platform.ID            `json:"-"`
	HomeBranchID           platform.ID            `json:"homeBranchId"`
	Name                   string                 `json:"name"`
	SharedContactPoints    []ContactPoint         `json:"sharedContactPoints"`
	SharedAddresses        []Address              `json:"sharedAddresses"`
	PrimaryContactPersonID platform.ID            `json:"primaryContactPersonId"`
	StatementPreference    string                 `json:"statementPreference"`
	Members                []HouseholdMemberInput `json:"members"`
}
type UpdateHouseholdInput struct {
	ExpectedVersion        int64          `json:"expectedVersion"`
	HomeBranchID           platform.ID    `json:"homeBranchId"`
	Name                   string         `json:"name"`
	SharedContactPoints    []ContactPoint `json:"sharedContactPoints"`
	SharedAddresses        []Address      `json:"sharedAddresses"`
	PrimaryContactPersonID platform.ID    `json:"primaryContactPersonId"`
	StatementPreference    string         `json:"statementPreference"`
}

func (in *UpdateHouseholdInput) normalizeAndValidate(organizationID platform.ID) error {
	if err := platform.RequireExpectedVersion(in.ExpectedVersion); err != nil {
		return err
	}
	members := []HouseholdMemberInput{}
	if in.PrimaryContactPersonID.Valid() {
		members = append(members, HouseholdMemberInput{PersonID: in.PrimaryContactPersonID, Role: "primary-contact"})
	}
	candidate := CreateHouseholdInput{OrganizationID: organizationID, HomeBranchID: in.HomeBranchID, Name: in.Name, SharedContactPoints: in.SharedContactPoints, SharedAddresses: in.SharedAddresses, PrimaryContactPersonID: in.PrimaryContactPersonID, StatementPreference: in.StatementPreference, Members: members}
	if err := candidate.NormalizeAndValidate(); err != nil {
		return err
	}
	in.HomeBranchID, in.Name, in.SharedContactPoints, in.SharedAddresses, in.StatementPreference = candidate.HomeBranchID, candidate.Name, candidate.SharedContactPoints, candidate.SharedAddresses, candidate.StatementPreference
	return nil
}

type HouseholdMemberInput struct {
	PersonID platform.ID `json:"personId"`
	Role     string      `json:"role"`
}

func (in *CreateHouseholdInput) NormalizeAndValidate() error {
	if !in.OrganizationID.Valid() || !in.HomeBranchID.Valid() {
		return errors.New("organization and home branch are required")
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 150 {
		return errors.New("household name must be 1 to 150 characters")
	}
	for i := range in.SharedContactPoints {
		if err := normalizeContact(&in.SharedContactPoints[i]); err != nil {
			return err
		}
	}
	if err := onePrimaryContactPerType(in.SharedContactPoints); err != nil {
		return err
	}
	for i := range in.SharedAddresses {
		in.SharedAddresses[i].Line1 = strings.TrimSpace(in.SharedAddresses[i].Line1)
		in.SharedAddresses[i].Country = strings.ToUpper(strings.TrimSpace(in.SharedAddresses[i].Country))
		if in.SharedAddresses[i].Line1 == "" || len(in.SharedAddresses[i].Country) != 2 {
			return errors.New("shared addresses require line1 and two-letter country code")
		}
	}
	if in.StatementPreference == "" {
		in.StatementPreference = "individual"
	}
	if in.StatementPreference != "individual" && in.StatementPreference != "household" {
		return errors.New("statement preference must be individual or household")
	}
	seen := map[platform.ID]bool{}
	primaryFound := !in.PrimaryContactPersonID.Valid()
	for i := range in.Members {
		in.Members[i].Role = strings.TrimSpace(in.Members[i].Role)
		if err := ValidateHouseholdMembership("pending", in.Members[i].PersonID, in.Members[i].Role); err != nil {
			return err
		}
		if seen[in.Members[i].PersonID] {
			return errors.New("a person can appear only once in initial household members")
		}
		seen[in.Members[i].PersonID] = true
		if in.Members[i].PersonID == in.PrimaryContactPersonID {
			primaryFound = true
		}
	}
	if !primaryFound {
		return errors.New("primary contact must be an initial household member")
	}
	return nil
}

func ValidateHouseholdMembership(householdID, personID platform.ID, role string) error {
	if !householdID.Valid() || !personID.Valid() {
		return errors.New("household and person are required")
	}
	role = strings.TrimSpace(role)
	if role == "" || len(role) > 80 {
		return errors.New("household role must be 1 to 80 characters")
	}
	return nil
}
func ValidateRelationship(from, to platform.ID, relationshipType, visibility string) error {
	if !from.Valid() || !to.Valid() || from == to {
		return errors.New("relationship requires two different people")
	}
	relationshipType = strings.TrimSpace(relationshipType)
	if relationshipType == "" || len(relationshipType) > 80 {
		return errors.New("relationship type must be 1 to 80 characters")
	}
	if visibility == "" {
		visibility = "staff"
	}
	if visibility != "staff" && visibility != "member-visible" && visibility != "restricted" {
		return errors.New("invalid relationship visibility")
	}
	return nil
}

func NormalizeRelationshipType(value string) string { return strings.ToLower(strings.TrimSpace(value)) }
