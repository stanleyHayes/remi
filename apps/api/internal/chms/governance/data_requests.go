package governance

import (
	"context"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"remi-api/internal/chms/platform"
)

const requestCollection = "chms_data_requests"
const historyCollection = "chms_data_request_history"

type DataRequest struct {
	ID             platform.ID `bson:"_id" json:"id"`
	OrganizationID platform.ID `bson:"organizationId" json:"-"`
	PersonID       platform.ID `bson:"personId" json:"personId"`
	BranchID       platform.ID `bson:"branchId,omitempty" json:"branchId,omitempty"`
	Type           string      `bson:"type" json:"type"`
	Details        string      `bson:"details" json:"details"`
	Status         string      `bson:"status" json:"status"`
	Priority       string      `bson:"priority,omitempty" json:"priority"`
	AssigneeID     platform.ID `bson:"assigneeId,omitempty" json:"assigneeId,omitempty"`
	OutcomeCode    string      `bson:"outcomeCode,omitempty" json:"outcomeCode,omitempty"`
	MemberResponse string      `bson:"memberResponse,omitempty" json:"memberResponse,omitempty"`
	DueAt          time.Time   `bson:"dueAt,omitempty" json:"dueAt,omitempty"`
	Version        int64       `bson:"version,omitempty" json:"version"`
	CreatedAt      time.Time   `bson:"createdAt" json:"createdAt"`
	UpdatedAt      time.Time   `bson:"updatedAt" json:"updatedAt"`
}

type Transition struct {
	ID             platform.ID    `bson:"_id" json:"id"`
	OrganizationID platform.ID    `bson:"organizationId" json:"-"`
	RequestID      platform.ID    `bson:"requestId" json:"requestId"`
	From           string         `bson:"from" json:"from"`
	To             string         `bson:"to" json:"to"`
	Reason         string         `bson:"reason" json:"-"`
	MemberResponse string         `bson:"memberResponse,omitempty" json:"memberResponse,omitempty"`
	OutcomeCode    string         `bson:"outcomeCode,omitempty" json:"outcomeCode,omitempty"`
	Actor          platform.Actor `bson:"actor" json:"-"`
	OccurredAt     time.Time      `bson:"occurredAt" json:"occurredAt"`
}

type TransitionInput struct {
	ExpectedVersion int64       `json:"expectedVersion"`
	Status          string      `json:"status"`
	Priority        string      `json:"priority"`
	AssigneeID      platform.ID `json:"assigneeId"`
	Reason          string      `json:"reason"`
	OutcomeCode     string      `json:"outcomeCode"`
	MemberResponse  string      `json:"memberResponse"`
}

type Store interface {
	List(context.Context, platform.ID, platform.ID, string, int64) ([]DataRequest, error)
	Find(context.Context, platform.ID, platform.ID) (*DataRequest, error)
	History(context.Context, platform.ID, platform.ID) ([]Transition, error)
	Transition(context.Context, *DataRequest, Transition, TransitionInput) (*DataRequest, error)
}

type Evidence interface {
	AppendAudit(context.Context, platform.AuditEvent) error
}

type Service struct {
	Store      Store
	Evidence   Evidence
	Authorizer platform.Authorizer
	Now        func() time.Time
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s Service) authorize(p platform.Principal, action string, branch platform.ID, mfa bool) error {
	if s.Authorizer == nil || !s.Authorizer.Authorize(p, platform.AccessRequest{Action: action, ResourceType: "data-request", OrganizationID: p.OrganizationID, BranchID: branch, FieldClasses: []platform.FieldClass{platform.FieldOperational, platform.FieldPersonal}, RequireRecentMFA: mfa, Now: s.now()}).Allowed {
		return &platform.DomainError{Code: "forbidden", Message: "You cannot manage this privacy-request scope."}
	}
	return nil
}

func (s Service) List(ctx context.Context, p platform.Principal, branch platform.ID, status string) ([]DataRequest, error) {
	if err := s.authorize(p, "read", branch, false); err != nil {
		return nil, err
	}
	status = strings.ToLower(strings.TrimSpace(status))
	if status != "" && !validStatus(status) {
		return nil, platform.ValidationError(platform.FieldError{Path: "status", Code: "invalid", Message: "Choose a supported request status."})
	}
	return s.Store.List(ctx, p.OrganizationID, branch, status, 100)
}

func (s Service) Get(ctx context.Context, p platform.Principal, id platform.ID) (*DataRequest, []Transition, error) {
	v, err := s.Store.Find(ctx, p.OrganizationID, id)
	if err != nil || v == nil {
		return nil, nil, &platform.DomainError{Code: "not_found", Message: "Privacy request not found."}
	}
	if err = s.authorize(p, "read", v.BranchID, false); err != nil {
		return nil, nil, err
	}
	history, err := s.Store.History(ctx, p.OrganizationID, id)
	return v, history, err
}

var transitions = map[string]map[string]bool{"received": {"triaged": true, "withdrawn": true}, "triaged": {"in-fulfilment": true}, "in-fulfilment": {"awaiting-approval": true}, "awaiting-approval": {"completed": true, "partially-completed": true, "declined": true}}

func validStatus(v string) bool {
	return v == "received" || v == "triaged" || v == "in-fulfilment" || v == "awaiting-approval" || v == "completed" || v == "partially-completed" || v == "declined" || v == "withdrawn"
}
func terminal(v string) bool {
	return v == "completed" || v == "partially-completed" || v == "declined"
}

func (s Service) Move(ctx context.Context, p platform.Principal, id platform.ID, input TransitionInput, requestID string) (*DataRequest, error) {
	v, err := s.Store.Find(ctx, p.OrganizationID, id)
	if err != nil || v == nil {
		return nil, &platform.DomainError{Code: "not_found", Message: "Privacy request not found."}
	}
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	input.Priority = strings.ToLower(strings.TrimSpace(input.Priority))
	input.Reason = strings.TrimSpace(input.Reason)
	input.MemberResponse = strings.TrimSpace(input.MemberResponse)
	input.OutcomeCode = strings.ToLower(strings.TrimSpace(input.OutcomeCode))
	if err = s.authorize(p, "operate", v.BranchID, terminal(input.Status)); err != nil {
		return nil, err
	}
	if input.ExpectedVersion < 1 {
		return nil, &platform.DomainError{Code: "precondition_required", Message: "Load the current request version before changing it."}
	}
	if !transitions[v.Status][input.Status] {
		return nil, &platform.DomainError{Code: "invalid_transition", Message: "That privacy-request transition is not allowed."}
	}
	if len(input.Reason) < 10 || len(input.Reason) > 1000 {
		return nil, platform.ValidationError(platform.FieldError{Path: "reason", Code: "invalid", Message: "Record a reason between 10 and 1,000 characters."})
	}
	if input.Status == "triaged" && (input.AssigneeID.Valid() == false || (input.Priority != "routine" && input.Priority != "urgent")) {
		return nil, platform.ValidationError(platform.FieldError{Path: "assigneeId", Code: "required", Message: "Choose an assignee and priority when triaging."})
	}
	if terminal(input.Status) {
		if p.Actor.ID == v.AssigneeID {
			return nil, &platform.DomainError{Code: "forbidden", Message: "A different reviewer must approve the final decision."}
		}
		if input.OutcomeCode == "" || len(input.MemberResponse) < 10 {
			return nil, platform.ValidationError(platform.FieldError{Path: "memberResponse", Code: "required", Message: "Choose an outcome and provide a plain-language member response."})
		}
	}
	now := s.now()
	transition := Transition{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, RequestID: id, From: v.Status, To: input.Status, Reason: input.Reason, MemberResponse: input.MemberResponse, OutcomeCode: input.OutcomeCode, Actor: p.Actor, OccurredAt: now}
	updated, err := s.Store.Transition(ctx, v, transition, input)
	if err != nil {
		return nil, err
	}
	if s.Evidence != nil {
		err = s.Evidence.AppendAudit(ctx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: v.BranchID, Actor: p.Actor, Action: "governance.data-request.transition", ResourceType: "data-request", ResourceID: id, SubjectIDs: []platform.ID{v.PersonID}, ChangedFields: []string{"status", "version"}, BeforeVersion: v.Version, AfterVersion: updated.Version, Outcome: "success", Reason: input.Reason, RequestID: requestID, OccurredAt: now})
	}
	return updated, err
}

type MongoStore struct{ DB *mongo.Database }

func (m MongoStore) List(ctx context.Context, org, branch platform.ID, status string, limit int64) ([]DataRequest, error) {
	filter := bson.M{"organizationId": org}
	if branch.Valid() {
		filter["branchId"] = branch
	}
	if status != "" {
		filter["status"] = status
	}
	cur, err := m.DB.Collection(requestCollection).Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(limit))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []DataRequest
	err = cur.All(ctx, &out)
	return out, err
}
func (m MongoStore) Find(ctx context.Context, org, id platform.ID) (*DataRequest, error) {
	var v DataRequest
	err := m.DB.Collection(requestCollection).FindOne(ctx, bson.M{"_id": id, "organizationId": org}).Decode(&v)
	if err != nil {
		return nil, err
	}
	if v.Version < 1 {
		v.Version = 1
	}
	return &v, nil
}
func (m MongoStore) History(ctx context.Context, org, id platform.ID) ([]Transition, error) {
	cur, err := m.DB.Collection(historyCollection).Find(ctx, bson.M{"organizationId": org, "requestId": id}, options.Find().SetSort(bson.D{{Key: "occurredAt", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []Transition
	err = cur.All(ctx, &out)
	return out, err
}
func (m MongoStore) Transition(ctx context.Context, current *DataRequest, event Transition, input TransitionInput) (*DataRequest, error) {
	session, err := m.DB.Client().StartSession()
	if err != nil {
		return nil, err
	}
	defer session.EndSession(ctx)
	var updated DataRequest
	_, err = session.WithTransaction(ctx, func(tx context.Context) (any, error) {
		set := bson.M{"status": input.Status, "updatedAt": event.OccurredAt}
		if input.Priority != "" {
			set["priority"] = input.Priority
		}
		if input.AssigneeID.Valid() {
			set["assigneeId"] = input.AssigneeID
		}
		if input.OutcomeCode != "" {
			set["outcomeCode"] = input.OutcomeCode
		}
		if input.MemberResponse != "" {
			set["memberResponse"] = input.MemberResponse
		}
		result, updateErr := m.DB.Collection(requestCollection).UpdateOne(tx, bson.M{"_id": current.ID, "organizationId": current.OrganizationID, "$or": bson.A{bson.M{"version": input.ExpectedVersion}, bson.M{"version": bson.M{"$exists": false}, "status": "received", "$expr": bson.M{"$eq": bson.A{input.ExpectedVersion, int64(1)}}}}}, bson.M{"$set": set, "$inc": bson.M{"version": 1}})
		if updateErr != nil {
			return nil, updateErr
		}
		if result.ModifiedCount != 1 {
			return nil, platform.VersionConflict(current.Version)
		}
		if _, updateErr = m.DB.Collection(historyCollection).InsertOne(tx, event); updateErr != nil {
			return nil, updateErr
		}
		return nil, m.DB.Collection(requestCollection).FindOne(tx, bson.M{"_id": current.ID, "organizationId": current.OrganizationID}).Decode(&updated)
	})
	return &updated, err
}
