package people

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"remi-api/internal/chms/platform"
)

const (
	mergeAliasesCollection = "chms_person_merge_aliases"
	mergeEventsCollection  = "chms_person_merge_events"
)

type DuplicateSignal struct {
	Type       string `json:"type" bson:"type"`
	Confidence int    `json:"confidence" bson:"confidence"`
	Hint       string `json:"hint" bson:"hint"`
}

type DuplicateCandidate struct {
	Person  PersonSummary     `json:"person"`
	Signals []DuplicateSignal `json:"signals"`
	Score   int               `json:"score"`
}

type MergeResolution struct {
	Names                    string `json:"names" bson:"names"`
	PhotoAssetID             string `json:"photoAssetId" bson:"photoAssetId"`
	DateOfBirth              string `json:"dateOfBirth" bson:"dateOfBirth"`
	Gender                   string `json:"gender" bson:"gender"`
	CommunicationPreferences string `json:"communicationPreferences" bson:"communicationPreferences"`
	Aliases                  string `json:"aliases" bson:"aliases"`
	ContactPoints            string `json:"contactPoints" bson:"contactPoints"`
	Addresses                string `json:"addresses" bson:"addresses"`
	Tags                     string `json:"tags" bson:"tags"`
	CustomFields             string `json:"customFields" bson:"customFields"`
}

func (r *MergeResolution) NormalizeAndValidate() error {
	scalars := []*string{&r.Names, &r.PhotoAssetID, &r.DateOfBirth, &r.Gender, &r.CommunicationPreferences}
	for _, value := range scalars {
		*value = strings.ToLower(strings.TrimSpace(*value))
		if *value == "" {
			*value = "canonical"
		}
		if *value != "canonical" && *value != "duplicate" {
			return errors.New("scalar merge choices must be canonical or duplicate")
		}
	}
	collections := []*string{&r.Aliases, &r.ContactPoints, &r.Addresses, &r.Tags, &r.CustomFields}
	for _, value := range collections {
		*value = strings.ToLower(strings.TrimSpace(*value))
		if *value == "" {
			*value = "combine"
		}
		if *value != "canonical" && *value != "duplicate" && *value != "combine" {
			return errors.New("collection merge choices must be canonical, duplicate or combine")
		}
	}
	return nil
}

type MergeInput struct {
	CanonicalPersonID        platform.ID     `json:"canonicalPersonId"`
	DuplicatePersonID        platform.ID     `json:"duplicatePersonId"`
	CanonicalExpectedVersion int64           `json:"canonicalExpectedVersion"`
	DuplicateExpectedVersion int64           `json:"duplicateExpectedVersion"`
	Resolution               MergeResolution `json:"resolution"`
	Reason                   string          `json:"reason"`
}

type PersonMergeEvent struct {
	ID                     platform.ID       `json:"id" bson:"_id"`
	OrganizationID         platform.ID       `json:"organizationId" bson:"organizationId"`
	CanonicalPersonID      platform.ID       `json:"canonicalPersonId" bson:"canonicalPersonId"`
	DuplicatePersonID      platform.ID       `json:"duplicatePersonId" bson:"duplicatePersonId"`
	CanonicalVersionBefore int64             `json:"canonicalVersionBefore" bson:"canonicalVersionBefore"`
	DuplicateVersionBefore int64             `json:"duplicateVersionBefore" bson:"duplicateVersionBefore"`
	CanonicalVersionAfter  int64             `json:"canonicalVersionAfter" bson:"canonicalVersionAfter"`
	Resolution             MergeResolution   `json:"resolution" bson:"resolution"`
	Signals                []DuplicateSignal `json:"signals" bson:"signals"`
	Reason                 string            `json:"reason" bson:"reason"`
	CreatedAt              time.Time         `json:"createdAt" bson:"createdAt"`
	CreatedBy              platform.Actor    `json:"createdBy" bson:"createdBy"`
	RequestID              string            `json:"requestId" bson:"requestId"`
}

type PersonMergeAlias struct {
	ID                platform.ID `json:"id" bson:"_id"`
	OrganizationID    platform.ID `json:"organizationId" bson:"organizationId"`
	AliasPersonID     platform.ID `json:"aliasPersonId" bson:"aliasPersonId"`
	CanonicalPersonID platform.ID `json:"canonicalPersonId" bson:"canonicalPersonId"`
	MergeEventID      platform.ID `json:"mergeEventId" bson:"mergeEventId"`
	CreatedAt         time.Time   `json:"createdAt" bson:"createdAt"`
}

type MergeStore interface {
	FindByID(context.Context, platform.ID, platform.ID) (*Person, error)
	FindDuplicatePeople(context.Context, platform.ID, platform.ID, []ContactPoint, Source) ([]Person, error)
	CommitPersonMerge(context.Context, Person, Person, int64, int64, PersonMergeAlias, PersonMergeEvent, time.Time, platform.Actor) error
}

type MergeService struct {
	Store      MergeStore
	Platform   UnitOfWork
	Authorizer platform.Authorizer
	Now        func() time.Time
}

func (s MergeService) Candidates(ctx context.Context, principal platform.Principal, personID platform.ID) ([]DuplicateCandidate, error) {
	if s.Store == nil || s.Authorizer == nil {
		return nil, errors.New("merge service is not configured")
	}
	person, err := s.Store.FindByID(ctx, principal.OrganizationID, personID)
	if err != nil {
		return nil, err
	}
	if person == nil || person.ArchivedAt != nil {
		return nil, &platform.DomainError{Code: "not_found", Message: "Person not found."}
	}
	now := s.now()
	if !s.allowed(principal, "read", person.HomeBranchID, now) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot inspect duplicate candidates for this person."}
	}
	matches, err := s.Store.FindDuplicatePeople(ctx, principal.OrganizationID, person.ID, person.ContactPoints, person.Source)
	if err != nil {
		return nil, err
	}
	candidates := make([]DuplicateCandidate, 0, len(matches))
	for _, match := range matches {
		if !s.allowed(principal, "read", match.HomeBranchID, now) {
			continue
		}
		signals := duplicateSignals(*person, match)
		if len(signals) == 0 {
			continue
		}
		score := 0
		for _, signal := range signals {
			score += signal.Confidence
		}
		if score > 100 {
			score = 100
		}
		candidates = append(candidates, DuplicateCandidate{Person: personSummary(match), Signals: signals, Score: score})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].Person.ID < candidates[j].Person.ID
		}
		return candidates[i].Score > candidates[j].Score
	})
	return candidates, nil
}

func (s MergeService) Merge(ctx context.Context, principal platform.Principal, input MergeInput, requestID string) (*PersonMergeEvent, error) {
	if s.Store == nil || s.Platform == nil || s.Authorizer == nil {
		return nil, errors.New("merge service is not configured")
	}
	if !input.CanonicalPersonID.Valid() || !input.DuplicatePersonID.Valid() || input.CanonicalPersonID == input.DuplicatePersonID {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_people", Message: "Choose two different people to merge."})
	}
	if err := platform.RequireExpectedVersion(input.CanonicalExpectedVersion); err != nil {
		return nil, err
	}
	if err := platform.RequireExpectedVersion(input.DuplicateExpectedVersion); err != nil {
		return nil, err
	}
	if err := input.Resolution.NormalizeAndValidate(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "resolution", Code: "invalid_resolution", Message: err.Error()})
	}
	if err := platform.ValidateReason(input.Reason); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "reason", Code: "invalid_reason", Message: err.Error()})
	}
	canonical, err := s.Store.FindByID(ctx, principal.OrganizationID, input.CanonicalPersonID)
	if err != nil {
		return nil, err
	}
	duplicate, err := s.Store.FindByID(ctx, principal.OrganizationID, input.DuplicatePersonID)
	if err != nil {
		return nil, err
	}
	if canonical == nil || duplicate == nil || canonical.ArchivedAt != nil || duplicate.ArchivedAt != nil {
		return nil, &platform.DomainError{Code: "not_found", Message: "Both active people are required for a merge."}
	}
	now := s.now()
	if !s.allowed(principal, "merge", canonical.HomeBranchID, now) || !s.allowed(principal, "merge", duplicate.HomeBranchID, now) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You need merge access to both people and branches."}
	}
	signals := duplicateSignals(*canonical, *duplicate)
	if len(signals) == 0 {
		return nil, &platform.DomainError{Code: "no_duplicate_signal", Message: "The records do not share an approved duplicate signal."}
	}
	merged, err := resolvePersonMerge(*canonical, *duplicate, input.Resolution)
	if err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "resolution", Code: "invalid_result", Message: err.Error()})
	}
	merged.Version, merged.UpdatedAt, merged.UpdatedBy = input.CanonicalExpectedVersion+1, now, principal.Actor
	mergeID := platform.ID(bson.NewObjectID().Hex())
	alias := PersonMergeAlias{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, AliasPersonID: duplicate.ID, CanonicalPersonID: canonical.ID, MergeEventID: mergeID, CreatedAt: now}
	event := PersonMergeEvent{ID: mergeID, OrganizationID: principal.OrganizationID, CanonicalPersonID: canonical.ID, DuplicatePersonID: duplicate.ID, CanonicalVersionBefore: input.CanonicalExpectedVersion, DuplicateVersionBefore: input.DuplicateExpectedVersion, CanonicalVersionAfter: merged.Version, Resolution: input.Resolution, Signals: signals, Reason: strings.TrimSpace(input.Reason), CreatedAt: now, CreatedBy: principal.Actor, RequestID: requestID}
	if err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.CommitPersonMerge(tx, merged, *duplicate, input.CanonicalExpectedVersion, input.DuplicateExpectedVersion, alias, event, now, principal.Actor); err != nil {
			return err
		}
		if err := s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: canonical.HomeBranchID, Actor: principal.Actor, Action: "people.person.merge", ResourceType: "person", ResourceID: canonical.ID, SubjectIDs: []platform.ID{canonical.ID, duplicate.ID}, ChangedFields: []string{"identity", "contacts", "addresses", "tags", "customFields", "mergeAlias"}, Outcome: "success", Reason: input.Reason, RequestID: requestID, OccurredAt: now}); err != nil {
			return err
		}
		return s.Platform.EnqueueEvent(tx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: canonical.HomeBranchID, Type: "people.person.merged", EventVersion: 1, AggregateType: "person", AggregateID: canonical.ID, AggregateVersion: merged.Version, Actor: principal.Actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: map[string]any{"canonicalPersonId": canonical.ID, "aliasPersonId": duplicate.ID, "mergeEventId": event.ID}}, State: "pending", AvailableAt: now})
	}); err != nil {
		return nil, fmt.Errorf("merge people: %w", err)
	}
	return &event, nil
}

func (s MergeService) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func (s MergeService) allowed(principal platform.Principal, action string, branchID platform.ID, now time.Time) bool {
	return s.Authorizer.Authorize(principal, platform.AccessRequest{Action: action, ResourceType: "person-merge", OrganizationID: principal.OrganizationID, BranchID: branchID, FieldClasses: []platform.FieldClass{platform.FieldPersonal}, Now: now}).Allowed
}

func duplicateSignals(left, right Person) []DuplicateSignal {
	signals := []DuplicateSignal{}
	seen := map[string]bool{}
	for _, a := range left.ContactPoints {
		for _, b := range right.ContactPoints {
			if a.Type == b.Type && a.Normalized != "" && a.Normalized == b.Normalized {
				key := "contact:" + a.Type + ":" + a.Normalized
				if seen[key] {
					continue
				}
				seen[key] = true
				confidence := 90
				if a.Type == "email" {
					confidence = 100
				}
				signals = append(signals, DuplicateSignal{Type: a.Type, Confidence: confidence, Hint: maskedHint(a.Normalized)})
			}
		}
	}
	if left.Source.Type != "" && left.Source.Type == right.Source.Type && left.Source.Reference != "" && left.Source.Reference == right.Source.Reference {
		signals = append(signals, DuplicateSignal{Type: "external-id", Confidence: 100, Hint: maskedHint(left.Source.Reference)})
	}
	sort.Slice(signals, func(i, j int) bool {
		if signals[i].Confidence == signals[j].Confidence {
			return signals[i].Type < signals[j].Type
		}
		return signals[i].Confidence > signals[j].Confidence
	})
	return signals
}
func maskedHint(value string) string {
	runes := []rune(value)
	if len(runes) <= 4 {
		return "••••"
	}
	return "••••" + string(runes[len(runes)-4:])
}
func personSummary(person Person) PersonSummary {
	return PersonSummary{ID: person.ID, Version: person.Version, PersonNumber: person.PersonNumber, Names: person.Names, PhotoAssetID: person.PhotoAssetID, HomeBranchID: person.HomeBranchID, MembershipStage: person.MembershipStage, Tags: person.Tags, ContactPoints: person.ContactPoints, UpdatedAt: person.UpdatedAt}
}

func resolvePersonMerge(canonical, duplicate Person, resolution MergeResolution) (Person, error) {
	result := canonical
	if resolution.Names == "duplicate" {
		result.Names = duplicate.Names
	}
	if resolution.PhotoAssetID == "duplicate" {
		result.PhotoAssetID = duplicate.PhotoAssetID
	}
	if resolution.DateOfBirth == "duplicate" {
		result.DateOfBirth = duplicate.DateOfBirth
	}
	if resolution.Gender == "duplicate" {
		result.Gender = duplicate.Gender
	}
	if resolution.CommunicationPreferences == "duplicate" {
		result.CommunicationPreferences = duplicate.CommunicationPreferences
	}
	result.Aliases = chooseStrings(canonical.Aliases, duplicate.Aliases, resolution.Aliases)
	result.ContactPoints = chooseContacts(canonical.ContactPoints, duplicate.ContactPoints, resolution.ContactPoints)
	result.Addresses = chooseAddresses(canonical.Addresses, duplicate.Addresses, resolution.Addresses)
	result.Tags = chooseStrings(canonical.Tags, duplicate.Tags, resolution.Tags)
	result.CustomFields = chooseMap(canonical.CustomFields, duplicate.CustomFields, resolution.CustomFields)
	validation := CreateInput{OrganizationID: result.OrganizationID, HomeBranchID: result.HomeBranchID, Names: result.Names, Aliases: result.Aliases, PhotoAssetID: result.PhotoAssetID, DateOfBirth: result.DateOfBirth, Gender: result.Gender, ContactPoints: result.ContactPoints, Addresses: result.Addresses, MembershipStage: result.MembershipStage, Tags: result.Tags, CustomFields: result.CustomFields, CommunicationPreferences: result.CommunicationPreferences, Source: result.Source}
	if err := validation.NormalizeAndValidate(); err != nil {
		return Person{}, err
	}
	result.Names, result.Aliases, result.ContactPoints, result.Addresses, result.Tags, result.CustomFields = validation.Names, validation.Aliases, validation.ContactPoints, validation.Addresses, validation.Tags, validation.CustomFields
	return result, nil
}
func chooseStrings(a, b []string, mode string) []string {
	if mode == "canonical" {
		return append([]string(nil), a...)
	}
	if mode == "duplicate" {
		return append([]string(nil), b...)
	}
	seen := map[string]bool{}
	out := []string{}
	for _, values := range [][]string{a, b} {
		for _, value := range values {
			key := strings.ToLower(strings.TrimSpace(value))
			if key != "" && !seen[key] {
				seen[key] = true
				out = append(out, value)
			}
		}
	}
	return out
}
func chooseContacts(a, b []ContactPoint, mode string) []ContactPoint {
	if mode == "canonical" {
		return append([]ContactPoint(nil), a...)
	}
	if mode == "duplicate" {
		return append([]ContactPoint(nil), b...)
	}
	seen := map[string]bool{}
	out := []ContactPoint{}
	primary := map[string]bool{}
	for _, values := range [][]ContactPoint{a, b} {
		for _, value := range values {
			key := value.Type + ":" + value.Normalized
			if seen[key] {
				continue
			}
			seen[key] = true
			if value.Primary && primary[value.Type] {
				value.Primary = false
			}
			if value.Primary {
				primary[value.Type] = true
			}
			out = append(out, value)
		}
	}
	return out
}
func chooseAddresses(a, b []Address, mode string) []Address {
	if mode == "canonical" {
		return append([]Address(nil), a...)
	}
	if mode == "duplicate" {
		return append([]Address(nil), b...)
	}
	out := append([]Address(nil), a...)
	seen := map[string]bool{}
	primary := false
	for _, value := range out {
		seen[addressKey(value)] = true
		if value.Primary {
			primary = true
		}
	}
	for _, value := range b {
		if seen[addressKey(value)] {
			continue
		}
		if value.Primary && primary {
			value.Primary = false
		}
		if value.Primary {
			primary = true
		}
		out = append(out, value)
	}
	return out
}
func addressKey(a Address) string {
	return strings.ToLower(strings.TrimSpace(a.Line1 + "|" + a.Line2 + "|" + a.City + "|" + a.Region + "|" + a.Country))
}
func chooseMap(a, b map[string]string, mode string) map[string]string {
	out := map[string]string{}
	if mode != "duplicate" {
		for k, v := range a {
			out[k] = v
		}
	}
	if mode != "canonical" {
		for k, v := range b {
			if _, exists := out[k]; !exists || mode == "duplicate" {
				out[k] = v
			}
		}
	}
	return out
}

func (r *MongoRepository) FindDuplicatePeople(ctx context.Context, organizationID, excludeID platform.ID, contacts []ContactPoint, source Source) ([]Person, error) {
	clauses := bson.A{}
	values := []string{}
	for _, contact := range contacts {
		if contact.Normalized != "" {
			values = append(values, contact.Normalized)
		}
	}
	if len(values) > 0 {
		clauses = append(clauses, bson.M{"contactPoints.normalized": bson.M{"$in": values}})
	}
	if source.Type != "" && source.Reference != "" {
		clauses = append(clauses, bson.M{"source.type": source.Type, "source.reference": source.Reference})
	}
	if len(clauses) == 0 {
		return nil, nil
	}
	cursor, err := r.collection.Find(ctx, bson.M{"organizationId": organizationID, "_id": bson.M{"$ne": excludeID}, "archivedAt": nil, "$or": clauses}, options.Find().SetLimit(50))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var people []Person
	if err := cursor.All(ctx, &people); err != nil {
		return nil, err
	}
	return people, nil
}

func (r *MongoRepository) CommitPersonMerge(ctx context.Context, canonical, duplicate Person, canonicalExpected, duplicateExpected int64, alias PersonMergeAlias, event PersonMergeEvent, now time.Time, actor platform.Actor) error {
	canonicalResult, err := r.collection.ReplaceOne(ctx, bson.M{"_id": canonical.ID, "organizationId": canonical.OrganizationID, "version": canonicalExpected, "archivedAt": nil}, canonical)
	if err != nil {
		return err
	}
	if canonicalResult.MatchedCount != 1 {
		return platform.VersionConflict(canonicalExpected)
	}
	duplicateResult, err := r.collection.UpdateOne(ctx, bson.M{"_id": duplicate.ID, "organizationId": duplicate.OrganizationID, "version": duplicateExpected, "archivedAt": nil}, bson.M{"$set": bson.M{"archivedAt": now, "archivedBy": actor, "archiveReason": "merged-record", "mergedIntoPersonId": canonical.ID, "updatedAt": now, "updatedBy": actor}, "$inc": bson.M{"version": 1}})
	if err != nil {
		return err
	}
	if duplicateResult.MatchedCount != 1 {
		return platform.VersionConflict(duplicateExpected)
	}
	if _, err := r.collection.Database().Collection(mergeAliasesCollection).InsertOne(ctx, alias); err != nil {
		return err
	}
	if _, err := r.collection.Database().Collection(mergeEventsCollection).InsertOne(ctx, event); err != nil {
		return err
	}
	return nil
}

var _ MergeStore = (*MongoRepository)(nil)
