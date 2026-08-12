package community

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"remi-api/internal/chms/platform"
)

const groupsCollection = "chms_groups"
const groupMembershipsCollection = "chms_group_memberships"
const groupMembershipEventsCollection = "chms_group_membership_events"
const groupMeetingsCollection = "chms_group_meetings"
const groupAttendanceCollection = "chms_group_attendance"
const volunteerTeamsCollection = "chms_volunteer_teams"
const volunteerPositionsCollection = "chms_volunteer_positions"
const volunteerProfilesCollection = "chms_volunteer_profiles"
const volunteerAvailabilityCollection = "chms_volunteer_availability"
const servicePlansCollection = "chms_service_plans"
const volunteerRotationsCollection = "chms_volunteer_rotations"
const assignmentsCollection = "chms_assignments"
const assignmentEventsCollection = "chms_assignment_events"
const communicationHandoffsCollection = "chms_communication_handoffs"

type Store interface {
	InsertGroup(context.Context, Group) error
	FindGroup(context.Context, platform.ID, platform.ID) (*Group, error)
	ListGroups(context.Context, platform.ID, platform.ID, bool) ([]Group, error)
	UpdateGroup(context.Context, platform.ID, platform.ID, int64, GroupInput, time.Time, platform.Actor) error
	InsertMembership(context.Context, GroupMembership) error
	InsertMembershipEvent(context.Context, GroupMembershipEvent) error
	FindMembership(context.Context, platform.ID, platform.ID, platform.ID) (*GroupMembership, error)
	FindMembershipByID(context.Context, platform.ID, platform.ID) (*GroupMembership, error)
	ListMemberships(context.Context, platform.ID, platform.ID, bool) ([]GroupMembership, error)
	ListMembershipEvents(context.Context, platform.ID, platform.ID) ([]GroupMembershipEvent, error)
	UpdateMembership(context.Context, platform.ID, platform.ID, int64, MembershipTransition, time.Time, platform.Actor) error
	ReserveGroupSeat(context.Context, platform.ID, platform.ID, int) (bool, error)
	ReleaseGroupSeat(context.Context, platform.ID, platform.ID) error
	InsertMeeting(context.Context, GroupMeeting) error
	FindMeeting(context.Context, platform.ID, platform.ID) (*GroupMeeting, error)
	ListMeetings(context.Context, platform.ID, platform.ID, time.Time, time.Time) ([]GroupMeeting, error)
	InsertMeetingAttendance(context.Context, GroupMeetingAttendance) error
	FindMeetingAttendance(context.Context, platform.ID, platform.ID, platform.ID) (*GroupMeetingAttendance, error)
	UpdateMeetingAttendance(context.Context, platform.ID, platform.ID, platform.ID, int64, MeetingAttendanceInput, time.Time, platform.Actor) error
	ListMeetingAttendance(context.Context, platform.ID, platform.ID) ([]GroupMeetingAttendance, error)
	InsertVolunteerTeam(context.Context, VolunteerTeam) error
	FindVolunteerTeam(context.Context, platform.ID, platform.ID) (*VolunteerTeam, error)
	ListVolunteerTeams(context.Context, platform.ID, platform.ID, bool) ([]VolunteerTeam, error)
	UpdateVolunteerTeam(context.Context, platform.ID, platform.ID, int64, VolunteerTeamInput, time.Time, platform.Actor) error
	InsertVolunteerPosition(context.Context, VolunteerPosition) error
	FindVolunteerPosition(context.Context, platform.ID, platform.ID) (*VolunteerPosition, error)
	ListVolunteerPositions(context.Context, platform.ID, platform.ID, bool) ([]VolunteerPosition, error)
	UpdateVolunteerPosition(context.Context, platform.ID, platform.ID, int64, VolunteerPositionInput, time.Time, platform.Actor) error
	UpsertVolunteerProfile(context.Context, VolunteerProfile, int64) error
	FindVolunteerProfile(context.Context, platform.ID, platform.ID) (*VolunteerProfile, error)
	InsertAvailability(context.Context, AvailabilityWindow) error
	FindAvailabilityBySourceKey(context.Context, platform.ID, platform.ID, string) (*AvailabilityWindow, error)
	ListAvailability(context.Context, platform.ID, platform.ID, time.Time, time.Time) ([]AvailabilityWindow, error)
	InsertServicePlan(context.Context, ServicePlan) error
	FindServicePlan(context.Context, platform.ID, platform.ID) (*ServicePlan, error)
	ListServicePlans(context.Context, platform.ID, platform.ID, time.Time, time.Time) ([]ServicePlan, error)
	UpdateServicePlan(context.Context, platform.ID, platform.ID, int64, ServicePlanTransition, time.Time, platform.Actor) error
	InsertVolunteerRotation(context.Context, VolunteerRotation) error
	FindVolunteerRotation(context.Context, platform.ID, platform.ID) (*VolunteerRotation, error)
	ListVolunteerRotations(context.Context, platform.ID, platform.ID, bool) ([]VolunteerRotation, error)
	InsertAssignment(context.Context, VolunteerAssignment) error
	FindAssignment(context.Context, platform.ID, platform.ID) (*VolunteerAssignment, error)
	ListAssignments(context.Context, platform.ID, platform.ID) ([]VolunteerAssignment, error)
	ListPersonAssignments(context.Context, platform.ID, platform.ID, time.Time, time.Time) ([]VolunteerAssignment, error)
	ListPersonAssignmentsOverlapping(context.Context, platform.ID, platform.ID, time.Time, time.Time, platform.ID) ([]VolunteerAssignment, error)
	UpdateAssignment(context.Context, platform.ID, platform.ID, int64, AssignmentUpdate, time.Time, platform.Actor) error
	InsertAssignmentEvent(context.Context, AssignmentEvent) error
	ListAssignmentEvents(context.Context, platform.ID, platform.ID) ([]AssignmentEvent, error)
}
type MongoRepository struct{ database *mongo.Database }

func NewMongoRepository(database *mongo.Database) (*MongoRepository, error) {
	if database == nil {
		return nil, errors.New("community database is required")
	}
	return &MongoRepository{database: database}, nil
}
func (r *MongoRepository) EnsureIndexes(ctx context.Context) error {
	definitions := []struct {
		name   string
		models []mongo.IndexModel
	}{
		{groupsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "homeBranchId", Value: 1}, {Key: "ministryId", Value: 1}, {Key: "status", Value: 1}, {Key: "name", Value: 1}}}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "leaderPersonIds", Value: 1}, {Key: "status", Value: 1}}}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "discoverability", Value: 1}, {Key: "privacy", Value: 1}, {Key: "status", Value: 1}}}}},
		{groupMembershipsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "groupId", Value: 1}, {Key: "personId", Value: 1}}, Options: options.Index().SetUnique(true)}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "groupId", Value: 1}, {Key: "status", Value: 1}, {Key: "requestedAt", Value: 1}}}}},
		{groupMembershipEventsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "membershipId", Value: 1}, {Key: "occurredAt", Value: 1}, {Key: "_id", Value: 1}}}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "groupId", Value: 1}, {Key: "personId", Value: 1}, {Key: "occurredAt", Value: -1}}}}},
		{groupMeetingsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "groupId", Value: 1}, {Key: "startsAt", Value: -1}, {Key: "_id", Value: 1}}}}},
		{groupAttendanceCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "meetingId", Value: 1}, {Key: "personId", Value: 1}}, Options: options.Index().SetUnique(true)}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "groupId", Value: 1}, {Key: "personId", Value: 1}, {Key: "updatedAt", Value: -1}}}}},
		{volunteerTeamsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "homeBranchId", Value: 1}, {Key: "status", Value: 1}, {Key: "name", Value: 1}}}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "leaderPersonIds", Value: 1}, {Key: "status", Value: 1}}}}},
		{volunteerPositionsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "teamId", Value: 1}, {Key: "status", Value: 1}, {Key: "name", Value: 1}}}}},
		{volunteerProfilesCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "personId", Value: 1}}, Options: options.Index().SetUnique(true)}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "skills", Value: 1}, {Key: "status", Value: 1}}}}},
		{volunteerAvailabilityCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "personId", Value: 1}, {Key: "startsAt", Value: 1}, {Key: "endsAt", Value: 1}}}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "personId", Value: 1}, {Key: "sourceKey", Value: 1}}, Options: options.Index().SetUnique(true)}}},
		{servicePlansCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "occurrenceId", Value: 1}}, Options: options.Index().SetUnique(true)}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "branchId", Value: 1}, {Key: "startsAt", Value: 1}, {Key: "status", Value: 1}}}}},
		{volunteerRotationsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "teamId", Value: 1}, {Key: "status", Value: 1}, {Key: "name", Value: 1}}}}},
		{assignmentsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "planId", Value: 1}, {Key: "positionId", Value: 1}, {Key: "slot", Value: 1}}, Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"status": bson.M{"$in": []string{"invited", "accepted", "substitute-requested", "no-show"}}})}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "personId", Value: 1}, {Key: "startsAt", Value: 1}, {Key: "endsAt", Value: 1}, {Key: "status", Value: 1}}}}},
		{assignmentEventsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "assignmentId", Value: 1}, {Key: "occurredAt", Value: 1}, {Key: "_id", Value: 1}}}}},
		{communicationHandoffsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "groupId", Value: 1}, {Key: "createdAt", Value: -1}}}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "state", Value: 1}, {Key: "createdAt", Value: 1}}}}},
	}
	for _, definition := range definitions {
		if _, err := r.database.Collection(definition.name).Indexes().CreateMany(ctx, definition.models); err != nil {
			return fmt.Errorf("create %s indexes: %w", definition.name, err)
		}
	}
	return nil
}

func (r *MongoRepository) InsertCommunicationHandoff(ctx context.Context, value GroupCommunicationHandoff) error {
	_, err := r.database.Collection(communicationHandoffsCollection).InsertOne(ctx, value)
	if err != nil {
		return fmt.Errorf("insert communication handoff: %w", err)
	}
	return nil
}

func (r *MongoRepository) LoadCommunityAnalytics(ctx context.Context, organizationID, branchID platform.ID, from, to time.Time) (CommunityAnalyticsDataset, error) {
	data := CommunityAnalyticsDataset{Groups: []Group{}, Memberships: []GroupMembership{}, Meetings: []GroupMeeting{}, Attendance: []GroupMeetingAttendance{}, Teams: []VolunteerTeam{}, Positions: []VolunteerPosition{}, Plans: []ServicePlan{}, Assignments: []VolunteerAssignment{}}
	load := func(collection string, filter any, sortOrder bson.D, target any) error {
		cursor, err := r.database.Collection(collection).Find(ctx, filter, options.Find().SetSort(sortOrder))
		if err != nil {
			return fmt.Errorf("load community analytics %s: %w", collection, err)
		}
		defer cursor.Close(ctx)
		if err = cursor.All(ctx, target); err != nil {
			return fmt.Errorf("decode community analytics %s: %w", collection, err)
		}
		return nil
	}
	if err := load(groupsCollection, bson.M{"organizationId": organizationID, "homeBranchId": branchID}, bson.D{{Key: "name", Value: 1}}, &data.Groups); err != nil {
		return data, err
	}
	groupIDs := make([]platform.ID, 0, len(data.Groups))
	for _, group := range data.Groups {
		groupIDs = append(groupIDs, group.ID)
	}
	if len(groupIDs) > 0 {
		if err := load(groupMembershipsCollection, bson.M{"organizationId": organizationID, "groupId": bson.M{"$in": groupIDs}}, bson.D{{Key: "groupId", Value: 1}}, &data.Memberships); err != nil {
			return data, err
		}
		if err := load(groupMeetingsCollection, bson.M{"organizationId": organizationID, "groupId": bson.M{"$in": groupIDs}, "startsAt": bson.M{"$gte": from, "$lt": to}}, bson.D{{Key: "startsAt", Value: 1}}, &data.Meetings); err != nil {
			return data, err
		}
		meetingIDs := make([]platform.ID, 0, len(data.Meetings))
		for _, meeting := range data.Meetings {
			meetingIDs = append(meetingIDs, meeting.ID)
		}
		if len(meetingIDs) > 0 {
			if err := load(groupAttendanceCollection, bson.M{"organizationId": organizationID, "meetingId": bson.M{"$in": meetingIDs}}, bson.D{{Key: "meetingId", Value: 1}}, &data.Attendance); err != nil {
				return data, err
			}
		}
	}
	if err := load(volunteerTeamsCollection, bson.M{"organizationId": organizationID, "homeBranchId": branchID}, bson.D{{Key: "name", Value: 1}}, &data.Teams); err != nil {
		return data, err
	}
	teamIDs := make([]platform.ID, 0, len(data.Teams))
	for _, team := range data.Teams {
		teamIDs = append(teamIDs, team.ID)
	}
	if len(teamIDs) > 0 {
		if err := load(volunteerPositionsCollection, bson.M{"organizationId": organizationID, "teamId": bson.M{"$in": teamIDs}}, bson.D{{Key: "teamId", Value: 1}}, &data.Positions); err != nil {
			return data, err
		}
		if err := load(assignmentsCollection, bson.M{"organizationId": organizationID, "teamId": bson.M{"$in": teamIDs}, "startsAt": bson.M{"$lt": to}, "endsAt": bson.M{"$gt": from}}, bson.D{{Key: "startsAt", Value: 1}}, &data.Assignments); err != nil {
			return data, err
		}
	}
	if err := load(servicePlansCollection, bson.M{"organizationId": organizationID, "branchId": branchID, "startsAt": bson.M{"$lt": to}, "endsAt": bson.M{"$gt": from}}, bson.D{{Key: "startsAt", Value: 1}}, &data.Plans); err != nil {
		return data, err
	}
	return data, nil
}
func (r *MongoRepository) InsertGroup(ctx context.Context, value Group) error {
	_, err := r.database.Collection(groupsCollection).InsertOne(ctx, value)
	if err != nil {
		return fmt.Errorf("insert group: %w", err)
	}
	return nil
}
func (r *MongoRepository) FindGroup(ctx context.Context, organizationID, id platform.ID) (*Group, error) {
	var value Group
	err := r.database.Collection(groupsCollection).FindOne(ctx, bson.M{"_id": id, "organizationId": organizationID}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find group: %w", err)
	}
	return &value, nil
}
func (r *MongoRepository) ListGroups(ctx context.Context, organizationID, branchID platform.ID, includeClosed bool) ([]Group, error) {
	filter := bson.M{"organizationId": organizationID, "homeBranchId": branchID}
	if !includeClosed {
		filter["status"] = bson.M{"$ne": "closed"}
	}
	cursor, err := r.database.Collection(groupsCollection).Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "status", Value: 1}, {Key: "name", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	defer cursor.Close(ctx)
	var values []Group
	if err = cursor.All(ctx, &values); err != nil {
		return nil, err
	}
	if values == nil {
		values = []Group{}
	}
	return values, nil
}
func (r *MongoRepository) ListGroupsLedBy(ctx context.Context, organizationID, personID platform.ID) ([]Group, error) {
	cursor, err := r.database.Collection(groupsCollection).Find(ctx, bson.M{"organizationId": organizationID, "leaderPersonIds": personID, "status": bson.M{"$ne": "closed"}}, options.Find().SetSort(bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list led groups: %w", err)
	}
	defer cursor.Close(ctx)
	values := []Group{}
	if err = cursor.All(ctx, &values); err != nil {
		return nil, err
	}
	return values, nil
}
func (r *MongoRepository) UpdateGroup(ctx context.Context, organizationID, id platform.ID, expectedVersion int64, input GroupInput, now time.Time, actor platform.Actor) error {
	set := bson.M{"name": input.Name, "type": input.Type, "description": input.Description, "homeBranchId": input.HomeBranchID, "branchId": input.HomeBranchID, "ministryId": input.MinistryID, "leaderPersonIds": input.LeaderPersonIDs, "capacity": input.Capacity, "meetingPattern": input.MeetingPattern, "privacy": input.Privacy, "discoverability": input.Discoverability, "status": input.Status, "updatedAt": now, "updatedBy": actor}
	if input.Status == "closed" {
		set["closedAt"] = now
		set["closureReason"] = input.Reason
	} else {
		set["closedAt"] = nil
		set["closureReason"] = ""
	}
	result, err := r.database.Collection(groupsCollection).UpdateOne(ctx, bson.M{"_id": id, "organizationId": organizationID, "version": expectedVersion}, bson.M{"$set": set, "$inc": bson.M{"version": 1}})
	if err != nil {
		return fmt.Errorf("update group: %w", err)
	}
	if result.MatchedCount != 1 {
		return platform.VersionConflict(expectedVersion)
	}
	return nil
}

func (r *MongoRepository) InsertMembership(ctx context.Context, value GroupMembership) error {
	_, err := r.database.Collection(groupMembershipsCollection).InsertOne(ctx, value)
	if mongo.IsDuplicateKeyError(err) {
		return &platform.DomainError{Code: "conflict", Message: "This person already has a group membership."}
	}
	if err != nil {
		return fmt.Errorf("insert group membership: %w", err)
	}
	return nil
}
func (r *MongoRepository) InsertMembershipEvent(ctx context.Context, value GroupMembershipEvent) error {
	_, err := r.database.Collection(groupMembershipEventsCollection).InsertOne(ctx, value)
	if err != nil {
		return fmt.Errorf("insert group membership event: %w", err)
	}
	return nil
}
func (r *MongoRepository) FindMembership(ctx context.Context, organizationID, groupID, personID platform.ID) (*GroupMembership, error) {
	var value GroupMembership
	err := r.database.Collection(groupMembershipsCollection).FindOne(ctx, bson.M{"organizationId": organizationID, "groupId": groupID, "personId": personID}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find group membership: %w", err)
	}
	return &value, nil
}
func (r *MongoRepository) FindMembershipByID(ctx context.Context, organizationID, id platform.ID) (*GroupMembership, error) {
	var value GroupMembership
	err := r.database.Collection(groupMembershipsCollection).FindOne(ctx, bson.M{"_id": id, "organizationId": organizationID}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find group membership: %w", err)
	}
	return &value, nil
}
func (r *MongoRepository) ListMemberships(ctx context.Context, organizationID, groupID platform.ID, includeInactive bool) ([]GroupMembership, error) {
	filter := bson.M{"organizationId": organizationID, "groupId": groupID}
	if !includeInactive {
		filter["status"] = bson.M{"$nin": []string{"declined", "ended"}}
	}
	cursor, err := r.database.Collection(groupMembershipsCollection).Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "status", Value: 1}, {Key: "createdAt", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list group memberships: %w", err)
	}
	defer cursor.Close(ctx)
	var values []GroupMembership
	if err = cursor.All(ctx, &values); err != nil {
		return nil, err
	}
	if values == nil {
		values = []GroupMembership{}
	}
	return values, nil
}
func (r *MongoRepository) ListMembershipEvents(ctx context.Context, organizationID, membershipID platform.ID) ([]GroupMembershipEvent, error) {
	cursor, err := r.database.Collection(groupMembershipEventsCollection).Find(ctx, bson.M{"organizationId": organizationID, "membershipId": membershipID}, options.Find().SetSort(bson.D{{Key: "occurredAt", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list group membership events: %w", err)
	}
	defer cursor.Close(ctx)
	var values []GroupMembershipEvent
	if err = cursor.All(ctx, &values); err != nil {
		return nil, err
	}
	if values == nil {
		values = []GroupMembershipEvent{}
	}
	return values, nil
}
func (r *MongoRepository) UpdateMembership(ctx context.Context, organizationID, id platform.ID, expectedVersion int64, input MembershipTransition, now time.Time, actor platform.Actor) error {
	set := bson.M{"status": input.Status, "role": input.Role, "leaderNote": input.LeaderNote, "directoryVisibility": input.DirectoryVisibility, "lastReason": input.Reason, "updatedAt": now, "updatedBy": actor}
	switch input.Status {
	case "applied":
		set["requestedAt"] = now
		set["invitedAt"] = nil
		set["waitlistedAt"] = nil
		set["joinedAt"] = nil
		set["endedAt"] = nil
	case "invited":
		set["invitedAt"] = now
		set["requestedAt"] = nil
		set["waitlistedAt"] = nil
		set["joinedAt"] = nil
		set["endedAt"] = nil
	case "active":
		set["joinedAt"] = now
		set["waitlistedAt"] = nil
		set["endedAt"] = nil
	case "waitlisted":
		set["waitlistedAt"] = now
	case "ended", "declined":
		set["endedAt"] = now
	}
	result, err := r.database.Collection(groupMembershipsCollection).UpdateOne(ctx, bson.M{"_id": id, "organizationId": organizationID, "version": expectedVersion}, bson.M{"$set": set, "$inc": bson.M{"version": 1}})
	if err != nil {
		return fmt.Errorf("update group membership: %w", err)
	}
	if result.MatchedCount != 1 {
		return platform.VersionConflict(expectedVersion)
	}
	return nil
}
func (r *MongoRepository) ReserveGroupSeat(ctx context.Context, organizationID, groupID platform.ID, capacity int) (bool, error) {
	filter := bson.M{"_id": groupID, "organizationId": organizationID, "status": "active"}
	if capacity > 0 {
		filter["$or"] = []bson.M{{"activeMemberCount": bson.M{"$lt": capacity}}, {"activeMemberCount": bson.M{"$exists": false}}}
	}
	result, err := r.database.Collection(groupsCollection).UpdateOne(ctx, filter, bson.M{"$inc": bson.M{"activeMemberCount": 1}})
	if err != nil {
		return false, fmt.Errorf("reserve group seat: %w", err)
	}
	return result.ModifiedCount == 1, nil
}
func (r *MongoRepository) ReleaseGroupSeat(ctx context.Context, organizationID, groupID platform.ID) error {
	result, err := r.database.Collection(groupsCollection).UpdateOne(ctx, bson.M{"_id": groupID, "organizationId": organizationID, "activeMemberCount": bson.M{"$gt": 0}}, bson.M{"$inc": bson.M{"activeMemberCount": -1}})
	if err != nil {
		return fmt.Errorf("release group seat: %w", err)
	}
	if result.MatchedCount != 1 {
		return &platform.DomainError{Code: "conflict", Message: "Group seat projection is already empty."}
	}
	return nil
}
func (r *MongoRepository) InsertMeeting(ctx context.Context, value GroupMeeting) error {
	_, err := r.database.Collection(groupMeetingsCollection).InsertOne(ctx, value)
	if err != nil {
		return fmt.Errorf("insert group meeting: %w", err)
	}
	return nil
}
func (r *MongoRepository) FindMeeting(ctx context.Context, organizationID, id platform.ID) (*GroupMeeting, error) {
	var value GroupMeeting
	err := r.database.Collection(groupMeetingsCollection).FindOne(ctx, bson.M{"_id": id, "organizationId": organizationID}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find group meeting: %w", err)
	}
	return &value, nil
}
func (r *MongoRepository) ListMeetings(ctx context.Context, organizationID, groupID platform.ID, from, to time.Time) ([]GroupMeeting, error) {
	cursor, err := r.database.Collection(groupMeetingsCollection).Find(ctx, bson.M{"organizationId": organizationID, "groupId": groupID, "startsAt": bson.M{"$gte": from, "$lt": to}}, options.Find().SetSort(bson.D{{Key: "startsAt", Value: -1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list group meetings: %w", err)
	}
	defer cursor.Close(ctx)
	var values []GroupMeeting
	if err = cursor.All(ctx, &values); err != nil {
		return nil, err
	}
	if values == nil {
		values = []GroupMeeting{}
	}
	return values, nil
}
func (r *MongoRepository) InsertMeetingAttendance(ctx context.Context, value GroupMeetingAttendance) error {
	_, err := r.database.Collection(groupAttendanceCollection).InsertOne(ctx, value)
	if mongo.IsDuplicateKeyError(err) {
		return platform.VersionConflict(0)
	}
	if err != nil {
		return fmt.Errorf("insert group attendance: %w", err)
	}
	return nil
}
func (r *MongoRepository) FindMeetingAttendance(ctx context.Context, organizationID, meetingID, personID platform.ID) (*GroupMeetingAttendance, error) {
	var value GroupMeetingAttendance
	err := r.database.Collection(groupAttendanceCollection).FindOne(ctx, bson.M{"organizationId": organizationID, "meetingId": meetingID, "personId": personID}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find group attendance: %w", err)
	}
	return &value, nil
}
func (r *MongoRepository) UpdateMeetingAttendance(ctx context.Context, organizationID, meetingID, personID platform.ID, expectedVersion int64, input MeetingAttendanceInput, now time.Time, actor platform.Actor) error {
	result, err := r.database.Collection(groupAttendanceCollection).UpdateOne(ctx, bson.M{"organizationId": organizationID, "meetingId": meetingID, "personId": personID, "version": expectedVersion}, bson.M{"$set": bson.M{"status": input.Status, "confidence": input.Confidence, "reason": input.Reason, "updatedAt": now, "updatedBy": actor}, "$inc": bson.M{"version": 1}})
	if err != nil {
		return fmt.Errorf("update group attendance: %w", err)
	}
	if result.MatchedCount != 1 {
		return platform.VersionConflict(expectedVersion)
	}
	return nil
}
func (r *MongoRepository) ListMeetingAttendance(ctx context.Context, organizationID, meetingID platform.ID) ([]GroupMeetingAttendance, error) {
	cursor, err := r.database.Collection(groupAttendanceCollection).Find(ctx, bson.M{"organizationId": organizationID, "meetingId": meetingID}, options.Find().SetSort(bson.D{{Key: "personId", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list group attendance: %w", err)
	}
	defer cursor.Close(ctx)
	var values []GroupMeetingAttendance
	if err = cursor.All(ctx, &values); err != nil {
		return nil, err
	}
	if values == nil {
		values = []GroupMeetingAttendance{}
	}
	return values, nil
}

func (r *MongoRepository) InsertVolunteerTeam(ctx context.Context, value VolunteerTeam) error {
	_, err := r.database.Collection(volunteerTeamsCollection).InsertOne(ctx, value)
	if err != nil {
		return fmt.Errorf("insert volunteer team: %w", err)
	}
	return nil
}
func (r *MongoRepository) FindVolunteerTeam(ctx context.Context, organizationID, id platform.ID) (*VolunteerTeam, error) {
	var value VolunteerTeam
	err := r.database.Collection(volunteerTeamsCollection).FindOne(ctx, bson.M{"_id": id, "organizationId": organizationID}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find volunteer team: %w", err)
	}
	return &value, nil
}
func (r *MongoRepository) ListVolunteerTeams(ctx context.Context, organizationID, branchID platform.ID, includeInactive bool) ([]VolunteerTeam, error) {
	filter := bson.M{"organizationId": organizationID, "homeBranchId": branchID}
	if !includeInactive {
		filter["status"] = "active"
	}
	cursor, err := r.database.Collection(volunteerTeamsCollection).Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list volunteer teams: %w", err)
	}
	defer cursor.Close(ctx)
	var values []VolunteerTeam
	if err = cursor.All(ctx, &values); err != nil {
		return nil, err
	}
	if values == nil {
		values = []VolunteerTeam{}
	}
	return values, nil
}
func (r *MongoRepository) ListVolunteerTeamsLedBy(ctx context.Context, organizationID, personID platform.ID) ([]VolunteerTeam, error) {
	cursor, err := r.database.Collection(volunteerTeamsCollection).Find(ctx, bson.M{"organizationId": organizationID, "leaderPersonIds": personID, "status": "active"}, options.Find().SetSort(bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list led volunteer teams: %w", err)
	}
	defer cursor.Close(ctx)
	values := []VolunteerTeam{}
	if err = cursor.All(ctx, &values); err != nil {
		return nil, err
	}
	return values, nil
}
func (r *MongoRepository) UpdateVolunteerTeam(ctx context.Context, organizationID, id platform.ID, expectedVersion int64, input VolunteerTeamInput, now time.Time, actor platform.Actor) error {
	result, err := r.database.Collection(volunteerTeamsCollection).UpdateOne(ctx, bson.M{"_id": id, "organizationId": organizationID, "version": expectedVersion}, bson.M{"$set": bson.M{"name": input.Name, "description": input.Description, "homeBranchId": input.HomeBranchID, "branchId": input.HomeBranchID, "leaderPersonIds": input.LeaderPersonIDs, "status": input.Status, "updatedAt": now, "updatedBy": actor}, "$inc": bson.M{"version": 1}})
	if err != nil {
		return fmt.Errorf("update volunteer team: %w", err)
	}
	if result.MatchedCount != 1 {
		return platform.VersionConflict(expectedVersion)
	}
	return nil
}
func (r *MongoRepository) InsertVolunteerPosition(ctx context.Context, value VolunteerPosition) error {
	_, err := r.database.Collection(volunteerPositionsCollection).InsertOne(ctx, value)
	if err != nil {
		return fmt.Errorf("insert volunteer position: %w", err)
	}
	return nil
}
func (r *MongoRepository) FindVolunteerPosition(ctx context.Context, organizationID, id platform.ID) (*VolunteerPosition, error) {
	var value VolunteerPosition
	err := r.database.Collection(volunteerPositionsCollection).FindOne(ctx, bson.M{"_id": id, "organizationId": organizationID}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find volunteer position: %w", err)
	}
	return &value, nil
}
func (r *MongoRepository) ListVolunteerPositions(ctx context.Context, organizationID, teamID platform.ID, includeInactive bool) ([]VolunteerPosition, error) {
	filter := bson.M{"organizationId": organizationID, "teamId": teamID}
	if !includeInactive {
		filter["status"] = "active"
	}
	cursor, err := r.database.Collection(volunteerPositionsCollection).Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list volunteer positions: %w", err)
	}
	defer cursor.Close(ctx)
	var values []VolunteerPosition
	if err = cursor.All(ctx, &values); err != nil {
		return nil, err
	}
	if values == nil {
		values = []VolunteerPosition{}
	}
	return values, nil
}
func (r *MongoRepository) UpdateVolunteerPosition(ctx context.Context, organizationID, id platform.ID, expectedVersion int64, input VolunteerPositionInput, now time.Time, actor platform.Actor) error {
	result, err := r.database.Collection(volunteerPositionsCollection).UpdateOne(ctx, bson.M{"_id": id, "organizationId": organizationID, "version": expectedVersion}, bson.M{"$set": bson.M{"name": input.Name, "description": input.Description, "eligibility": input.Eligibility, "status": input.Status, "updatedAt": now, "updatedBy": actor}, "$inc": bson.M{"version": 1}})
	if err != nil {
		return fmt.Errorf("update volunteer position: %w", err)
	}
	if result.MatchedCount != 1 {
		return platform.VersionConflict(expectedVersion)
	}
	return nil
}
func (r *MongoRepository) UpsertVolunteerProfile(ctx context.Context, value VolunteerProfile, expectedVersion int64) error {
	filter := bson.M{"organizationId": value.OrganizationID, "personId": value.PersonID}
	if expectedVersion > 0 {
		filter["version"] = expectedVersion
	}
	update := bson.M{"$set": bson.M{"branchId": value.BranchID, "skills": value.Skills, "preferredTeamIds": value.PreferredTeamIDs, "preferredPositionIds": value.PreferredPositionIDs, "status": value.Status, "eligibility": value.Eligibility, "updatedAt": value.UpdatedAt, "updatedBy": value.UpdatedBy}, "$setOnInsert": bson.M{"_id": value.ID, "organizationId": value.OrganizationID, "personId": value.PersonID, "schemaVersion": value.SchemaVersion, "createdAt": value.CreatedAt, "createdBy": value.CreatedBy}, "$inc": bson.M{"version": 1}}
	result, err := r.database.Collection(volunteerProfilesCollection).UpdateOne(ctx, filter, update, options.UpdateOne().SetUpsert(expectedVersion == 0))
	if mongo.IsDuplicateKeyError(err) || (err == nil && expectedVersion > 0 && result.MatchedCount != 1) {
		return platform.VersionConflict(expectedVersion)
	}
	if err != nil {
		return fmt.Errorf("upsert volunteer profile: %w", err)
	}
	return nil
}
func (r *MongoRepository) FindVolunteerProfile(ctx context.Context, organizationID, personID platform.ID) (*VolunteerProfile, error) {
	var value VolunteerProfile
	err := r.database.Collection(volunteerProfilesCollection).FindOne(ctx, bson.M{"organizationId": organizationID, "personId": personID}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find volunteer profile: %w", err)
	}
	return &value, nil
}
func (r *MongoRepository) InsertAvailability(ctx context.Context, value AvailabilityWindow) error {
	_, err := r.database.Collection(volunteerAvailabilityCollection).InsertOne(ctx, value)
	if mongo.IsDuplicateKeyError(err) {
		return &platform.DomainError{Code: "conflict", Message: "This availability entry has already been recorded."}
	}
	if err != nil {
		return fmt.Errorf("insert availability: %w", err)
	}
	return nil
}
func (r *MongoRepository) FindAvailabilityBySourceKey(ctx context.Context, organizationID, personID platform.ID, sourceKey string) (*AvailabilityWindow, error) {
	var value AvailabilityWindow
	err := r.database.Collection(volunteerAvailabilityCollection).FindOne(ctx, bson.M{"organizationId": organizationID, "personId": personID, "sourceKey": sourceKey}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find availability by source key: %w", err)
	}
	return &value, nil
}
func (r *MongoRepository) ListAvailability(ctx context.Context, organizationID, personID platform.ID, from, to time.Time) ([]AvailabilityWindow, error) {
	cursor, err := r.database.Collection(volunteerAvailabilityCollection).Find(ctx, bson.M{"organizationId": organizationID, "personId": personID, "startsAt": bson.M{"$lt": to}, "endsAt": bson.M{"$gt": from}}, options.Find().SetSort(bson.D{{Key: "startsAt", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list availability: %w", err)
	}
	defer cursor.Close(ctx)
	var values []AvailabilityWindow
	if err = cursor.All(ctx, &values); err != nil {
		return nil, err
	}
	if values == nil {
		values = []AvailabilityWindow{}
	}
	return values, nil
}

func (r *MongoRepository) InsertServicePlan(ctx context.Context, value ServicePlan) error {
	_, err := r.database.Collection(servicePlansCollection).InsertOne(ctx, value)
	if mongo.IsDuplicateKeyError(err) {
		return &platform.DomainError{Code: "conflict", Message: "This occurrence already has a service plan."}
	}
	if err != nil {
		return fmt.Errorf("insert service plan: %w", err)
	}
	return nil
}

func (r *MongoRepository) FindServicePlan(ctx context.Context, organizationID, id platform.ID) (*ServicePlan, error) {
	var value ServicePlan
	err := r.database.Collection(servicePlansCollection).FindOne(ctx, bson.M{"_id": id, "organizationId": organizationID}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find service plan: %w", err)
	}
	return &value, nil
}

func (r *MongoRepository) ListServicePlans(ctx context.Context, organizationID, branchID platform.ID, from, to time.Time) ([]ServicePlan, error) {
	cursor, err := r.database.Collection(servicePlansCollection).Find(ctx, bson.M{"organizationId": organizationID, "branchId": branchID, "startsAt": bson.M{"$lt": to}, "endsAt": bson.M{"$gt": from}}, options.Find().SetSort(bson.D{{Key: "startsAt", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list service plans: %w", err)
	}
	defer cursor.Close(ctx)
	values := []ServicePlan{}
	if err := cursor.All(ctx, &values); err != nil {
		return nil, err
	}
	return values, nil
}

func (r *MongoRepository) UpdateServicePlan(ctx context.Context, organizationID, id platform.ID, expectedVersion int64, transition ServicePlanTransition, now time.Time, actor platform.Actor) error {
	set := bson.M{"status": transition.Status, "lastReason": transition.Reason, "updatedAt": now, "updatedBy": actor}
	switch transition.Status {
	case "published":
		set["publishedAt"] = now
	case "locked":
		set["lockedAt"] = now
	case "completed":
		set["completedAt"] = now
	case "cancelled":
		set["cancelledAt"] = now
	}
	result, err := r.database.Collection(servicePlansCollection).UpdateOne(ctx, bson.M{"_id": id, "organizationId": organizationID, "version": expectedVersion}, bson.M{"$set": set, "$inc": bson.M{"version": 1}})
	if err != nil {
		return fmt.Errorf("update service plan: %w", err)
	}
	if result.MatchedCount != 1 {
		return platform.VersionConflict(expectedVersion)
	}
	return nil
}

func (r *MongoRepository) InsertVolunteerRotation(ctx context.Context, value VolunteerRotation) error {
	_, err := r.database.Collection(volunteerRotationsCollection).InsertOne(ctx, value)
	if err != nil {
		return fmt.Errorf("insert volunteer rotation: %w", err)
	}
	return nil
}

func (r *MongoRepository) FindVolunteerRotation(ctx context.Context, organizationID, id platform.ID) (*VolunteerRotation, error) {
	var value VolunteerRotation
	err := r.database.Collection(volunteerRotationsCollection).FindOne(ctx, bson.M{"_id": id, "organizationId": organizationID}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find volunteer rotation: %w", err)
	}
	return &value, nil
}

func (r *MongoRepository) ListVolunteerRotations(ctx context.Context, organizationID, teamID platform.ID, includeInactive bool) ([]VolunteerRotation, error) {
	filter := bson.M{"organizationId": organizationID, "teamId": teamID}
	if !includeInactive {
		filter["status"] = "active"
	}
	cursor, err := r.database.Collection(volunteerRotationsCollection).Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list volunteer rotations: %w", err)
	}
	defer cursor.Close(ctx)
	values := []VolunteerRotation{}
	if err := cursor.All(ctx, &values); err != nil {
		return nil, err
	}
	return values, nil
}

func (r *MongoRepository) InsertAssignment(ctx context.Context, value VolunteerAssignment) error {
	_, err := r.database.Collection(assignmentsCollection).InsertOne(ctx, value)
	if mongo.IsDuplicateKeyError(err) {
		return &platform.DomainError{Code: "conflict", Message: "This serving slot is already assigned."}
	}
	if err != nil {
		return fmt.Errorf("insert volunteer assignment: %w", err)
	}
	return nil
}

func (r *MongoRepository) FindAssignment(ctx context.Context, organizationID, id platform.ID) (*VolunteerAssignment, error) {
	var value VolunteerAssignment
	err := r.database.Collection(assignmentsCollection).FindOne(ctx, bson.M{"_id": id, "organizationId": organizationID}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find volunteer assignment: %w", err)
	}
	return &value, nil
}

func (r *MongoRepository) ListAssignments(ctx context.Context, organizationID, planID platform.ID) ([]VolunteerAssignment, error) {
	cursor, err := r.database.Collection(assignmentsCollection).Find(ctx, bson.M{"organizationId": organizationID, "planId": planID}, options.Find().SetSort(bson.D{{Key: "positionId", Value: 1}, {Key: "slot", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list volunteer assignments: %w", err)
	}
	defer cursor.Close(ctx)
	values := []VolunteerAssignment{}
	if err := cursor.All(ctx, &values); err != nil {
		return nil, err
	}
	return values, nil
}

func (r *MongoRepository) ListPersonAssignments(ctx context.Context, organizationID, personID platform.ID, from, to time.Time) ([]VolunteerAssignment, error) {
	cursor, err := r.database.Collection(assignmentsCollection).Find(ctx, bson.M{"organizationId": organizationID, "personId": personID, "startsAt": bson.M{"$lt": to}, "endsAt": bson.M{"$gt": from}}, options.Find().SetSort(bson.D{{Key: "startsAt", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list person assignments: %w", err)
	}
	defer cursor.Close(ctx)
	values := []VolunteerAssignment{}
	if err := cursor.All(ctx, &values); err != nil {
		return nil, err
	}
	return values, nil
}

func (r *MongoRepository) ListTeamAssignments(ctx context.Context, organizationID, teamID platform.ID, from, to time.Time) ([]VolunteerAssignment, error) {
	cursor, err := r.database.Collection(assignmentsCollection).Find(ctx, bson.M{"organizationId": organizationID, "teamId": teamID, "startsAt": bson.M{"$lt": to}, "endsAt": bson.M{"$gt": from}}, options.Find().SetSort(bson.D{{Key: "startsAt", Value: 1}, {Key: "positionId", Value: 1}, {Key: "slot", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list team assignments: %w", err)
	}
	defer cursor.Close(ctx)
	values := []VolunteerAssignment{}
	if err := cursor.All(ctx, &values); err != nil {
		return nil, err
	}
	return values, nil
}

func (r *MongoRepository) ListPersonAssignmentsOverlapping(ctx context.Context, organizationID, personID platform.ID, from, to time.Time, excludeID platform.ID) ([]VolunteerAssignment, error) {
	filter := bson.M{"organizationId": organizationID, "personId": personID, "startsAt": bson.M{"$lt": to}, "endsAt": bson.M{"$gt": from}, "status": bson.M{"$in": []string{"invited", "accepted", "substitute-requested"}}}
	if excludeID.Valid() {
		filter["_id"] = bson.M{"$ne": excludeID}
	}
	cursor, err := r.database.Collection(assignmentsCollection).Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "startsAt", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list overlapping assignments: %w", err)
	}
	defer cursor.Close(ctx)
	values := []VolunteerAssignment{}
	if err := cursor.All(ctx, &values); err != nil {
		return nil, err
	}
	return values, nil
}

func (r *MongoRepository) UpdateAssignment(ctx context.Context, organizationID, id platform.ID, expectedVersion int64, update AssignmentUpdate, now time.Time, actor platform.Actor) error {
	set := bson.M{"status": update.Status, "responseReasonCode": update.ResponseReasonCode, "respondedAt": update.RespondedAt, "replacedByAssignmentId": update.ReplacedByAssignmentID, "reminder": update.Reminder, "noShowReason": update.NoShowReason, "updatedAt": now, "updatedBy": actor}
	result, err := r.database.Collection(assignmentsCollection).UpdateOne(ctx, bson.M{"_id": id, "organizationId": organizationID, "version": expectedVersion}, bson.M{"$set": set, "$inc": bson.M{"version": 1}})
	if err != nil {
		return fmt.Errorf("update volunteer assignment: %w", err)
	}
	if result.MatchedCount != 1 {
		return platform.VersionConflict(expectedVersion)
	}
	return nil
}

func (r *MongoRepository) InsertAssignmentEvent(ctx context.Context, value AssignmentEvent) error {
	_, err := r.database.Collection(assignmentEventsCollection).InsertOne(ctx, value)
	if err != nil {
		return fmt.Errorf("insert assignment event: %w", err)
	}
	return nil
}

func (r *MongoRepository) ListAssignmentEvents(ctx context.Context, organizationID, assignmentID platform.ID) ([]AssignmentEvent, error) {
	cursor, err := r.database.Collection(assignmentEventsCollection).Find(ctx, bson.M{"organizationId": organizationID, "assignmentId": assignmentID}, options.Find().SetSort(bson.D{{Key: "occurredAt", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list assignment events: %w", err)
	}
	defer cursor.Close(ctx)
	values := []AssignmentEvent{}
	if err := cursor.All(ctx, &values); err != nil {
		return nil, err
	}
	return values, nil
}
