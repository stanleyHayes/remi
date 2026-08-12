package httpapi

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	chmsimports "remi-api/internal/chms/imports"
	"remi-api/internal/chms/people"
	"remi-api/internal/chms/platform"
)

var peopleImportTargets = map[string]bool{"names.given": true, "names.middle": true, "names.family": true, "names.preferred": true, "email": true, "mobile": true, "whatsapp": true, "dateOfBirth": true, "gender": true, "address.line1": true, "address.city": true, "address.region": true, "membershipStage": true, "tags": true, "externalId": true}

func peopleImportSchema(branchID platform.ID) chmsimports.ValidationSchema {
	return chmsimports.ValidationSchema{AllowedTargets: peopleImportTargets, RequiredTargets: []string{"names.given"}, Validate: func(_ context.Context, row map[string]string) []chmsimports.RowError {
		input := personInputFromImport(branchID, "validation", row)
		if err := input.NormalizeAndValidate(); err != nil {
			return []chmsimports.RowError{{Field: "record", Code: "invalid_person", Message: err.Error()}}
		}
		return nil
	}}
}

type peopleImportCommitter struct {
	repository people.Repository
	platform   people.UnitOfWork
}
type peopleImportRollbacker struct{ repository people.Repository }

func (r peopleImportRollbacker) Rollback(ctx context.Context, run chmsimports.ImportRun, row chmsimports.ImportRow, reason string) *chmsimports.RowError {
	now := time.Now().UTC()
	actor := platform.Actor{Type: platform.ActorImport, ID: run.ID}
	if err := r.repository.SetArchive(ctx, run.OrganizationID, row.CanonicalResourceID, row.ResourceVersionAtCommit, &now, &actor, reason, now, actor); err != nil {
		return &chmsimports.RowError{Field: "record", Code: "rollback_conflict", Message: "The person changed after import and was not archived."}
	}
	return nil
}

func (c peopleImportCommitter) Commit(ctx context.Context, run chmsimports.ImportRun, row chmsimports.ImportRow) (platform.ID, int64, error) {
	input := personInputFromImport(run.BranchID, string(run.ID), row.Mapped)
	input.OrganizationID = run.OrganizationID
	if err := input.NormalizeAndValidate(); err != nil {
		return "", 0, err
	}
	duplicates, err := c.repository.FindDuplicateIDs(ctx, run.OrganizationID, input.ContactPoints)
	if err != nil {
		return "", 0, err
	}
	if len(duplicates) > 0 {
		return "", 0, &platform.DomainError{Code: "duplicate_candidate", Message: fmt.Sprintf("Row %d matches an existing person.", row.RowNumber), Details: map[string]any{"candidateIds": duplicates}}
	}
	now := time.Now().UTC()
	actor := platform.Actor{Type: platform.ActorImport, ID: run.ID}
	id := platform.ID(bson.NewObjectID().Hex())
	person := people.Person{ResourceEnvelope: platform.ResourceEnvelope{ID: id, OrganizationID: run.OrganizationID, BranchID: run.BranchID, SchemaVersion: people.CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: actor, UpdatedAt: now, UpdatedBy: actor}, PersonNumber: "P-" + bson.NewObjectID().Hex()[12:], Names: input.Names, PhotoAssetID: input.PhotoAssetID, DateOfBirth: input.DateOfBirth, Gender: input.Gender, ContactPoints: input.ContactPoints, Addresses: input.Addresses, HomeBranchID: run.BranchID, MembershipStage: input.MembershipStage, Tags: input.Tags, CustomFields: input.CustomFields, CommunicationPreferences: input.CommunicationPreferences, Source: input.Source}
	if err := c.repository.Insert(ctx, person); err != nil {
		return "", 0, err
	}
	requestID := "import-" + string(run.ID)
	if err := c.platform.AppendAudit(ctx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: run.OrganizationID, BranchID: run.BranchID, Actor: actor, Action: "people.person.import", ResourceType: "person", ResourceID: id, ChangedFields: []string{"identity", "contacts", "membershipStage"}, Outcome: "success", RequestID: requestID, OccurredAt: now}); err != nil {
		return "", 0, err
	}
	if err := c.platform.EnqueueEvent(ctx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: run.OrganizationID, BranchID: run.BranchID, Type: "people.person.imported", EventVersion: 1, AggregateType: "person", AggregateID: id, AggregateVersion: 1, Actor: actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: map[string]any{"personId": id, "importRunId": run.ID, "rowNumber": row.RowNumber}}, State: "pending", AvailableAt: now}); err != nil {
		return "", 0, err
	}
	return id, 1, nil
}

func personInputFromImport(branchID platform.ID, reference string, row map[string]string) people.CreateInput {
	contacts := []people.ContactPoint{}
	for _, kind := range []string{"email", "mobile", "whatsapp"} {
		if value := strings.TrimSpace(row[kind]); value != "" {
			contacts = append(contacts, people.ContactPoint{Type: kind, Value: value, Primary: true})
		}
	}
	addresses := []people.Address{}
	if line := strings.TrimSpace(row["address.line1"]); line != "" {
		addresses = append(addresses, people.Address{Line1: line, City: row["address.city"], Region: row["address.region"], Country: "GH", Primary: true})
	}
	stage := strings.TrimSpace(row["membershipStage"])
	if stage == "" {
		stage = "guest"
	}
	custom := map[string]string{}
	if external := strings.TrimSpace(row["externalId"]); external != "" {
		custom["externalId"] = external
	}
	tags := []string{}
	for _, tag := range strings.Split(row["tags"], ",") {
		if strings.TrimSpace(tag) != "" {
			tags = append(tags, tag)
		}
	}
	var dob *people.PartialDate
	if value := strings.TrimSpace(row["dateOfBirth"]); value != "" {
		precision := "day"
		if len(value) == 4 {
			precision = "year"
		} else if len(value) == 7 {
			precision = "month"
		}
		dob = &people.PartialDate{Value: value, Precision: precision}
	}
	var gender *string
	if value := strings.TrimSpace(row["gender"]); value != "" {
		gender = &value
	}
	return people.CreateInput{HomeBranchID: branchID, Names: people.Names{Given: row["names.given"], Middle: row["names.middle"], Family: row["names.family"], Preferred: row["names.preferred"]}, DateOfBirth: dob, Gender: gender, ContactPoints: contacts, Addresses: addresses, MembershipStage: stage, Tags: tags, CustomFields: custom, Source: people.Source{Type: "csv-import", Reference: reference, NoticeVersion: "member-intake-v1"}}
}
