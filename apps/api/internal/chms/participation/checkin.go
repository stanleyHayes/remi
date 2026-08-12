package participation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"remi-api/internal/chms/platform"
)

type CheckinSession struct {
	platform.ResourceEnvelope `bson:",inline"`
	DeviceLabel               string          `json:"deviceLabel" bson:"deviceLabel"`
	OccurrenceIDs             []platform.ID   `json:"occurrenceIds" bson:"occurrenceIds"`
	State                     string          `json:"state" bson:"state"`
	ExpiresAt                 time.Time       `json:"expiresAt" bson:"expiresAt"`
	LastActivityAt            time.Time       `json:"lastActivityAt" bson:"lastActivityAt"`
	LastSequence              int64           `json:"lastSequence" bson:"lastSequence"`
	LockedAt                  *time.Time      `json:"lockedAt,omitempty" bson:"lockedAt,omitempty"`
	LockedBy                  *platform.Actor `json:"lockedBy,omitempty" bson:"lockedBy,omitempty"`
	LockReason                string          `json:"lockReason,omitempty" bson:"lockReason,omitempty"`
}

type CheckinSessionInput struct {
	BranchID      platform.ID   `json:"branchId"`
	DeviceLabel   string        `json:"deviceLabel"`
	OccurrenceIDs []platform.ID `json:"occurrenceIds"`
	ExpiresAt     time.Time     `json:"expiresAt"`
}

type CheckinCommand struct {
	ClientCommandID string      `json:"clientCommandId" bson:"clientCommandId"`
	LocalSequence   int64       `json:"localSequence" bson:"localSequence"`
	Type            string      `json:"type" bson:"type"`
	OccurrenceID    platform.ID `json:"occurrenceId" bson:"occurrenceId"`
	PersonID        platform.ID `json:"personId" bson:"personId"`
	ExpectedVersion int64       `json:"expectedVersion" bson:"expectedVersion"`
	Status          string      `json:"status,omitempty" bson:"status,omitempty"`
	Guest           bool        `json:"guest" bson:"guest"`
	Confidence      int         `json:"confidence" bson:"confidence"`
	CapturedAt      time.Time   `json:"capturedAt" bson:"capturedAt"`
}

type CheckinCommandResult struct {
	ClientCommandID string      `json:"clientCommandId" bson:"clientCommandId"`
	LocalSequence   int64       `json:"localSequence" bson:"localSequence"`
	Classification  string      `json:"classification" bson:"classification"`
	Code            string      `json:"code,omitempty" bson:"code,omitempty"`
	Message         string      `json:"message,omitempty" bson:"message,omitempty"`
	AttendanceID    platform.ID `json:"attendanceId,omitempty" bson:"attendanceId,omitempty"`
	Version         int64       `json:"version,omitempty" bson:"version,omitempty"`
}

type CheckinCommandReceipt struct {
	ID             platform.ID          `json:"id" bson:"_id"`
	OrganizationID platform.ID          `json:"organizationId" bson:"organizationId"`
	BranchID       platform.ID          `json:"branchId" bson:"branchId"`
	SessionID      platform.ID          `json:"sessionId" bson:"sessionId"`
	Fingerprint    string               `json:"-" bson:"fingerprint"`
	Result         CheckinCommandResult `json:"result" bson:"result"`
	CreatedAt      time.Time            `json:"createdAt" bson:"createdAt"`
	CompletedAt    *time.Time           `json:"completedAt,omitempty" bson:"completedAt,omitempty"`
}

type CheckinSyncRequest struct {
	Commands []CheckinCommand `json:"commands"`
}
type CheckinSyncResponse struct {
	Results []CheckinCommandResult `json:"results"`
}

func (in *CheckinSessionInput) NormalizeAndValidate(now time.Time) error {
	in.DeviceLabel = strings.TrimSpace(in.DeviceLabel)
	in.ExpiresAt = in.ExpiresAt.UTC()
	in.OccurrenceIDs = normalizeIDs(in.OccurrenceIDs)
	if !in.BranchID.Valid() || len(in.OccurrenceIDs) == 0 {
		return errors.New("branch and at least one occurrence are required")
	}
	if len(in.DeviceLabel) < 2 || len(in.DeviceLabel) > 100 {
		return errors.New("device label must be 2 to 100 characters")
	}
	if in.ExpiresAt.Before(now.Add(5*time.Minute)) || in.ExpiresAt.After(now.Add(72*time.Hour)) {
		return errors.New("session expiry must be between 5 minutes and 72 hours from now")
	}
	return nil
}

func normalizeCheckinCommands(commands []CheckinCommand) error {
	if len(commands) == 0 || len(commands) > 500 {
		return errors.New("sync batch must contain 1 to 500 commands")
	}
	last := int64(0)
	seen := map[string]bool{}
	for i := range commands {
		command := &commands[i]
		command.ClientCommandID = strings.TrimSpace(command.ClientCommandID)
		command.Type = strings.ToLower(strings.TrimSpace(command.Type))
		command.Status = strings.ToLower(strings.TrimSpace(command.Status))
		command.CapturedAt = command.CapturedAt.UTC()
		if len(command.ClientCommandID) < 8 || len(command.ClientCommandID) > 128 || seen[command.ClientCommandID] {
			return errors.New("every command requires a unique 8 to 128 character client command ID")
		}
		seen[command.ClientCommandID] = true
		if command.LocalSequence <= last {
			return errors.New("commands must use strictly increasing local sequence numbers")
		}
		last = command.LocalSequence
		if command.Type != "check-in" && command.Type != "check-out" && command.Type != "mark" {
			return errors.New("command type must be check-in, check-out or mark")
		}
		if !command.OccurrenceID.Valid() || !command.PersonID.Valid() || command.CapturedAt.IsZero() {
			return errors.New("command occurrence, person and captured time are required")
		}
		if command.Confidence == 0 {
			command.Confidence = 100
		}
		if command.Confidence < 1 || command.Confidence > 100 {
			return errors.New("command confidence must be between 1 and 100")
		}
		if command.Type == "mark" && command.Status != AttendancePresent && command.Status != AttendanceAbsent && command.Status != AttendanceExcused {
			return errors.New("mark commands require present, absent or excused status")
		}
	}
	return nil
}

func checkinFingerprint(command CheckinCommand) string {
	encoded, _ := json.Marshal(command)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func (s Service) CreateCheckinSession(ctx context.Context, principal platform.Principal, input CheckinSessionInput, requestID string) (*CheckinSession, error) {
	now := s.now()
	if err := input.NormalizeAndValidate(now); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_checkin_session", Message: err.Error()})
	}
	if !s.allowed(principal, "create", "checkin-session", input.BranchID) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot authorize a check-in station in this branch."}
	}
	for _, id := range input.OccurrenceIDs {
		occurrence, err := s.Store.FindOccurrence(ctx, principal.OrganizationID, id)
		if err != nil {
			return nil, err
		}
		if occurrence == nil || occurrence.HomeBranchID != input.BranchID || occurrence.Status != "scheduled" {
			return nil, platform.ValidationError(platform.FieldError{Path: "occurrenceIds", Code: "invalid_occurrence", Message: "Every occurrence must be scheduled in the selected branch."})
		}
	}
	id := platform.ID(bson.NewObjectID().Hex())
	value := CheckinSession{ResourceEnvelope: platform.ResourceEnvelope{ID: id, OrganizationID: principal.OrganizationID, BranchID: input.BranchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: principal.Actor, UpdatedAt: now, UpdatedBy: principal.Actor}, DeviceLabel: input.DeviceLabel, OccurrenceIDs: input.OccurrenceIDs, State: "active", ExpiresAt: input.ExpiresAt, LastActivityAt: now}
	if err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.InsertCheckinSession(tx, value); err != nil {
			return err
		}
		return s.evidence(tx, principal, input.BranchID, "participation.checkin-session.created", "checkin-session", id, 1, []string{"deviceLabel", "occurrenceIds", "expiresAt"}, "", requestID, now)
	}); err != nil {
		return nil, fmt.Errorf("create check-in session: %w", err)
	}
	return &value, nil
}

func (s Service) LockCheckinSession(ctx context.Context, principal platform.Principal, sessionID platform.ID, reason, requestID string) (*CheckinSession, error) {
	if err := platform.ValidateReason(reason); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "reason", Code: "invalid_reason", Message: err.Error()})
	}
	session, err := s.Store.FindCheckinSession(ctx, principal.OrganizationID, sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil || !s.canOperateSession(principal, *session) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Check-in session not found."}
	}
	if session.State != "active" {
		return nil, &platform.DomainError{Code: "conflict", Message: "Check-in session is already locked."}
	}
	now := s.now()
	if err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.LockCheckinSession(tx, principal.OrganizationID, sessionID, session.Version, now, strings.TrimSpace(reason), principal.Actor); err != nil {
			return err
		}
		return s.evidence(tx, principal, session.BranchID, "participation.checkin-session.locked", "checkin-session", sessionID, session.Version+1, []string{"state", "lockedAt"}, reason, requestID, now)
	}); err != nil {
		return nil, fmt.Errorf("lock check-in session: %w", err)
	}
	return s.Store.FindCheckinSession(ctx, principal.OrganizationID, sessionID)
}

func (s Service) GetCheckinSession(ctx context.Context, principal platform.Principal, sessionID platform.ID) (*CheckinSession, error) {
	session, err := s.Store.FindCheckinSession(ctx, principal.OrganizationID, sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil || !s.canOperateSession(principal, *session) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Check-in session not found."}
	}
	return session, nil
}

func (s Service) SyncCheckinCommands(ctx context.Context, principal platform.Principal, sessionID platform.ID, request CheckinSyncRequest, requestID string) (*CheckinSyncResponse, error) {
	if err := normalizeCheckinCommands(request.Commands); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "commands", Code: "invalid_commands", Message: err.Error()})
	}
	session, err := s.Store.FindCheckinSession(ctx, principal.OrganizationID, sessionID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	if session == nil || !s.canOperateSession(principal, *session) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Check-in session not found."}
	}
	if session.State != "active" || !session.ExpiresAt.After(now) {
		return nil, &platform.DomainError{Code: "checkin_session_locked", Message: "This check-in station is locked or expired."}
	}
	authorized := map[platform.ID]bool{}
	for _, id := range session.OccurrenceIDs {
		authorized[id] = true
	}
	results := make([]CheckinCommandResult, 0, len(request.Commands))
	for _, command := range request.Commands {
		fingerprint := checkinFingerprint(command)
		processing := CheckinCommandReceipt{}
		existing, findErr := s.Store.FindCheckinCommandReceipt(ctx, principal.OrganizationID, sessionID, command.ClientCommandID)
		if findErr != nil {
			return nil, findErr
		}
		if existing != nil {
			if existing.Fingerprint != fingerprint {
				results = append(results, CheckinCommandResult{ClientCommandID: command.ClientCommandID, LocalSequence: command.LocalSequence, Classification: "rejected", Code: "idempotency_reused", Message: "Client command ID was reused with different content."})
				continue
			}
			if existing.Result.Classification == "processing" {
				results = append(results, CheckinCommandResult{ClientCommandID: command.ClientCommandID, LocalSequence: command.LocalSequence, Classification: "retry", Code: "processing", Message: "Command processing has not completed."})
				continue
			}
			if existing.Result.Classification == "retry" {
				reclaimed, reclaimErr := s.Store.ReclaimCheckinCommand(ctx, principal.OrganizationID, existing.ID, now)
				if reclaimErr != nil {
					return nil, reclaimErr
				}
				if !reclaimed {
					results = append(results, CheckinCommandResult{ClientCommandID: command.ClientCommandID, LocalSequence: command.LocalSequence, Classification: "retry", Code: "concurrent_retry", Message: "Command retry is already being processed."})
					break
				}
				processing = *existing
				processing.Result.Classification = "processing"
			} else {
				duplicate := existing.Result
				duplicate.Classification = "duplicate"
				results = append(results, duplicate)
				continue
			}
		}
		if !processing.ID.Valid() {
			processing = CheckinCommandReceipt{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: session.BranchID, SessionID: sessionID, Fingerprint: fingerprint, Result: CheckinCommandResult{ClientCommandID: command.ClientCommandID, LocalSequence: command.LocalSequence, Classification: "processing"}, CreatedAt: now}
			claimed, claimErr := s.Store.ClaimCheckinCommand(ctx, processing)
			if claimErr != nil {
				return nil, claimErr
			}
			if !claimed {
				results = append(results, CheckinCommandResult{ClientCommandID: command.ClientCommandID, LocalSequence: command.LocalSequence, Classification: "retry", Code: "concurrent_replay", Message: "Command is already being processed."})
				break
			}
		}
		if command.LocalSequence <= session.LastSequence {
			result := CheckinCommandResult{ClientCommandID: command.ClientCommandID, LocalSequence: command.LocalSequence, Classification: "conflict", Code: "sequence_replayed", Message: "Command sequence is older than the station checkpoint."}
			if err := s.finishCheckinCommand(ctx, principal, sessionID, processing.ID, command.LocalSequence, result, now); err != nil {
				return nil, err
			}
			results = append(results, result)
			continue
		}
		if !authorized[command.OccurrenceID] {
			result := CheckinCommandResult{ClientCommandID: command.ClientCommandID, LocalSequence: command.LocalSequence, Classification: "rejected", Code: "occurrence_not_authorized", Message: "Occurrence is not authorized for this station."}
			if err := s.finishCheckinCommand(ctx, principal, sessionID, processing.ID, command.LocalSequence, result, now); err != nil {
				return nil, err
			}
			if command.LocalSequence > session.LastSequence {
				session.LastSequence = command.LocalSequence
			}
			results = append(results, result)
			continue
		}
		result := s.applyCheckinCommand(ctx, principal, *session, command, requestID)
		if err := s.finishCheckinCommand(ctx, principal, sessionID, processing.ID, command.LocalSequence, result, now); err != nil {
			return nil, err
		}
		if command.LocalSequence > session.LastSequence {
			session.LastSequence = command.LocalSequence
		}
		results = append(results, result)
		if result.Classification == "retry" {
			break
		}
	}
	return &CheckinSyncResponse{Results: results}, nil
}

func (s Service) finishCheckinCommand(ctx context.Context, principal platform.Principal, sessionID, receiptID platform.ID, sequence int64, result CheckinCommandResult, now time.Time) error {
	if err := s.Store.CompleteCheckinCommand(ctx, principal.OrganizationID, receiptID, result, now); err != nil {
		return err
	}
	if result.Classification == "retry" {
		return nil
	}
	return s.Store.AdvanceCheckinSession(ctx, principal.OrganizationID, sessionID, sequence, now, principal.Actor)
}

func (s Service) applyCheckinCommand(ctx context.Context, principal platform.Principal, session CheckinSession, command CheckinCommand, requestID string) CheckinCommandResult {
	input := AttendanceInput{Status: command.Status, Source: "offline", Confidence: command.Confidence, StationID: session.ID, Guest: command.Guest, SyncCommandID: command.ClientCommandID}
	if command.Type == "check-in" {
		input.Status = AttendancePresent
		input.CheckedInAt = &command.CapturedAt
	}
	if command.Type == "check-out" {
		input.Status = AttendancePresent
		input.CheckedOutAt = &command.CapturedAt
		if current, _ := s.Store.FindAttendance(ctx, principal.OrganizationID, command.OccurrenceID, command.PersonID); current != nil {
			input.CheckedInAt = current.CheckedInAt
		}
	}
	attendance, err := s.RecordAttendance(ctx, principal, command.OccurrenceID, command.PersonID, command.ExpectedVersion, input, requestID)
	result := CheckinCommandResult{ClientCommandID: command.ClientCommandID, LocalSequence: command.LocalSequence}
	if err == nil {
		result.Classification = "applied"
		result.AttendanceID = attendance.ID
		result.Version = attendance.Version
		return result
	}
	result.Message = err.Error()
	result.Classification = "retry"
	result.Code = "dependency_error"
	var domain *platform.DomainError
	if errors.As(err, &domain) {
		result.Code = domain.Code
		switch domain.Code {
		case "version_conflict", "attendance_locked", "period_closed":
			result.Classification = "conflict"
		case "validation_failed", "not_found", "forbidden", "precondition_required":
			result.Classification = "rejected"
		}
	}
	return result
}

func (s Service) canOperateSession(principal platform.Principal, session CheckinSession) bool {
	if !s.allowed(principal, "operate", "checkin-session", session.BranchID) {
		return false
	}
	if session.CreatedBy.ID == principal.Actor.ID {
		return true
	}
	for _, role := range principal.Roles {
		if role == "super-admin" {
			return true
		}
	}
	return false
}
