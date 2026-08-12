package people

import (
	"context"
	"testing"
	"time"

	"remi-api/internal/chms/platform"
)

type fakeHouseholdStore struct {
	people             map[platform.ID]bool
	households         map[platform.ID]bool
	household          *Household
	memberships        []HouseholdMembership
	relationships      []Relationship
	endedMemberships   []platform.ID
	relationship       *Relationship
	endedRelationships []platform.ID
}

func (f *fakeHouseholdStore) InsertHousehold(_ context.Context, h Household) error {
	f.household = &h
	if f.households == nil {
		f.households = map[platform.ID]bool{}
	}
	f.households[h.ID] = true
	return nil
}
func (f *fakeHouseholdStore) InsertMembership(_ context.Context, m HouseholdMembership) error {
	f.memberships = append(f.memberships, m)
	return nil
}
func (f *fakeHouseholdStore) InsertRelationship(_ context.Context, r Relationship) error {
	f.relationships = append(f.relationships, r)
	f.relationship = &f.relationships[len(f.relationships)-1]
	return nil
}
func (f *fakeHouseholdStore) PersonExists(_ context.Context, _ platform.ID, id platform.ID) (bool, error) {
	return f.people[id], nil
}
func (f *fakeHouseholdStore) HouseholdExists(_ context.Context, _ platform.ID, id platform.ID) (bool, error) {
	return f.households[id], nil
}
func (f *fakeHouseholdStore) EndMembership(_ context.Context, _ platform.ID, id platform.ID, _ time.Time, _ string) error {
	f.endedMemberships = append(f.endedMemberships, id)
	return nil
}
func (f *fakeHouseholdStore) FindRelationship(context.Context, platform.ID, platform.ID) (*Relationship, error) {
	return f.relationship, nil
}
func (f *fakeHouseholdStore) EndRelationship(_ context.Context, _ platform.ID, id platform.ID, _ int64, _ time.Time, _ time.Time, _ platform.Actor) error {
	f.endedRelationships = append(f.endedRelationships, id)
	return nil
}
func (f *fakeHouseholdStore) FindHouseholdByID(context.Context, platform.ID, platform.ID) (*Household, error) {
	return f.household, nil
}
func (f *fakeHouseholdStore) UpdateHousehold(_ context.Context, _ platform.ID, _ platform.ID, expectedVersion int64, input UpdateHouseholdInput, updatedAt time.Time, actor platform.Actor) error {
	if f.household == nil {
		return nil
	}
	f.household.Version = expectedVersion + 1
	f.household.HomeBranchID = input.HomeBranchID
	f.household.BranchID = input.HomeBranchID
	f.household.Name = input.Name
	f.household.UpdatedAt = updatedAt
	f.household.UpdatedBy = actor
	return nil
}
func (f *fakeHouseholdStore) FindActiveMembership(_ context.Context, _ platform.ID, householdID, personID platform.ID) (*HouseholdMembership, error) {
	for i := range f.memberships {
		if f.memberships[i].HouseholdID == householdID && f.memberships[i].PersonID == personID && f.memberships[i].EndedAt == nil {
			return &f.memberships[i], nil
		}
	}
	return nil, nil
}

func TestHouseholdServiceCreatesHouseholdMembersAndEvidenceAtomically(t *testing.T) {
	now := time.Date(2026, 8, 11, 13, 0, 0, 0, time.UTC)
	repo := &fakeHouseholdStore{people: map[platform.ID]bool{"p1": true, "p2": true}, households: map[platform.ID]bool{}}
	evidence := &fakePlatform{}
	service := HouseholdService{Repository: repo, Platform: evidence, Now: func() time.Time { return now }}
	input := CreateHouseholdInput{OrganizationID: "org", HomeBranchID: "branch", Name: "Mensah Household", PrimaryContactPersonID: "p1", StatementPreference: "household", Members: []HouseholdMemberInput{{PersonID: "p1", Role: "adult"}, {PersonID: "p2", Role: "child"}}}
	household, err := service.Create(context.Background(), input, platform.Actor{Type: platform.ActorStaff, ID: "staff"}, "request-household")
	if err != nil {
		t.Fatal(err)
	}
	if household == nil || household.Version != 1 || len(repo.memberships) != 2 || len(evidence.audits) != 1 || len(evidence.events) != 1 {
		t.Fatalf("incomplete household create: household=%+v members=%d audits=%d events=%d", household, len(repo.memberships), len(evidence.audits), len(evidence.events))
	}
	if evidence.events[0].Type != "people.household.created" {
		t.Fatalf("unexpected event %s", evidence.events[0].Type)
	}
}

func TestHouseholdServiceRejectsUnknownOrInvalidPrimaryMember(t *testing.T) {
	service := HouseholdService{Repository: &fakeHouseholdStore{people: map[platform.ID]bool{"p1": true}}, Platform: &fakePlatform{}}
	input := CreateHouseholdInput{OrganizationID: "org", HomeBranchID: "branch", Name: "House", PrimaryContactPersonID: "missing", Members: []HouseholdMemberInput{{PersonID: "p1", Role: "adult"}}}
	if _, err := service.Create(context.Background(), input, platform.Actor{Type: platform.ActorStaff, ID: "staff"}, "request"); err == nil {
		t.Fatal("expected primary-member validation")
	}
	input.PrimaryContactPersonID = "p1"
	input.Members = append(input.Members, HouseholdMemberInput{PersonID: "missing", Role: "adult"})
	if _, err := service.Create(context.Background(), input, platform.Actor{Type: platform.ActorStaff, ID: "staff"}, "request"); err == nil {
		t.Fatal("expected unknown member rejection")
	}
}

func TestHouseholdMemberHistoryUsesAddAndEndEvents(t *testing.T) {
	now := time.Date(2026, 8, 11, 13, 0, 0, 0, time.UTC)
	repo := &fakeHouseholdStore{people: map[platform.ID]bool{"p1": true}, households: map[platform.ID]bool{"h1": true}}
	evidence := &fakePlatform{}
	service := HouseholdService{Repository: repo, Platform: evidence, Now: func() time.Time { return now }}
	membership, err := service.AddMember(context.Background(), "org", "h1", HouseholdMemberInput{PersonID: "p1", Role: "friend"}, time.Time{}, platform.Actor{Type: platform.ActorStaff, ID: "staff"}, "add")
	if err != nil {
		t.Fatal(err)
	}
	if membership.StartedAt != now || len(repo.memberships) != 1 || evidence.events[0].Type != "people.household.member.added" {
		t.Fatalf("bad add: %+v", membership)
	}
	if err := service.EndMember(context.Background(), "org", "h1", membership.ID, "Moved to another household", platform.Actor{Type: platform.ActorStaff, ID: "staff"}, "end"); err != nil {
		t.Fatal(err)
	}
	if len(repo.endedMemberships) != 1 || evidence.events[1].Type != "people.household.member.ended" {
		t.Fatal("membership end did not preserve history/evidence")
	}
}

func TestDirectionalRelationshipCreationAndEndAreVersioned(t *testing.T) {
	now := time.Date(2026, 8, 11, 13, 0, 0, 0, time.UTC)
	repo := &fakeHouseholdStore{people: map[platform.ID]bool{"guardian": true, "child": true}, households: map[platform.ID]bool{"home": true}}
	evidence := &fakePlatform{}
	service := HouseholdService{Repository: repo, Platform: evidence, Now: func() time.Time { return now }}
	relationship, err := service.CreateRelationship(context.Background(), "org", Relationship{FromPersonID: "guardian", ToPersonID: "child", Type: " Guardian ", HouseholdID: "home", Visibility: "restricted"}, platform.Actor{Type: platform.ActorStaff, ID: "staff"}, "create-rel")
	if err != nil {
		t.Fatal(err)
	}
	if relationship.Type != "guardian" || relationship.Version != 1 || len(repo.relationships) != 1 {
		t.Fatalf("bad relationship: %+v", relationship)
	}
	repo.relationship = relationship
	ended, err := service.EndRelationship(context.Background(), "org", relationship.ID, 1, "Guardianship record corrected", platform.Actor{Type: platform.ActorStaff, ID: "staff"}, "end-rel")
	if err != nil {
		t.Fatal(err)
	}
	if ended.EndedAt == nil || ended.Version != 2 || len(repo.endedRelationships) != 1 || evidence.events[1].Type != "people.relationship.ended" {
		t.Fatalf("bad relationship end: %+v", ended)
	}
}
