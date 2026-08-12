package reporting

import (
	"archive/zip"
	"bytes"
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

type Schedule struct {
	ID               platform.ID   `json:"id" bson:"_id"`
	OrganizationID   platform.ID   `json:"organizationId" bson:"organizationId"`
	OwnerID          platform.ID   `json:"ownerId" bson:"ownerId"`
	SavedViewID      platform.ID   `json:"savedViewId" bson:"savedViewId"`
	SavedViewVersion int64         `json:"savedViewVersion" bson:"savedViewVersion"`
	Name             string        `json:"name" bson:"name"`
	Cadence          string        `json:"cadence" bson:"cadence"`
	Format           string        `json:"format" bson:"format"`
	RecipientUserIDs []platform.ID `json:"recipientUserIds" bson:"recipientUserIds"`
	State            string        `json:"state" bson:"state"`
	NextRunAt        time.Time     `json:"nextRunAt" bson:"nextRunAt"`
	LastRunAt        *time.Time    `json:"lastRunAt,omitempty" bson:"lastRunAt,omitempty"`
	LastErrorCode    string        `json:"lastErrorCode,omitempty" bson:"lastErrorCode,omitempty"`
	Version          int64         `json:"version" bson:"version"`
	CreatedAt        time.Time     `json:"createdAt" bson:"createdAt"`
	UpdatedAt        time.Time     `json:"updatedAt" bson:"updatedAt"`
}

type ScheduleInput struct {
	SavedViewID      platform.ID   `json:"savedViewId"`
	SavedViewVersion int64         `json:"savedViewVersion"`
	Name             string        `json:"name"`
	Cadence          string        `json:"cadence"`
	Format           string        `json:"format"`
	RecipientUserIDs []platform.ID `json:"recipientUserIds"`
	NextRunAt        time.Time     `json:"nextRunAt"`
}

type BoardPackRun struct {
	ID                platform.ID  `json:"id" bson:"_id"`
	OrganizationID    platform.ID  `json:"organizationId" bson:"organizationId"`
	ScheduleID        platform.ID  `json:"scheduleId" bson:"scheduleId"`
	SavedViewID       platform.ID  `json:"savedViewId" bson:"savedViewId"`
	SavedViewVersion  int64        `json:"savedViewVersion" bson:"savedViewVersion"`
	DictionaryVersion string       `json:"dictionaryVersion" bson:"dictionaryVersion"`
	OwnerID           platform.ID  `json:"ownerId" bson:"ownerId"`
	BranchID          platform.ID  `json:"branchId" bson:"branchId"`
	Format            string       `json:"format" bson:"format"`
	Result            ReportResult `json:"result" bson:"result"`
	FileName          string       `json:"fileName" bson:"fileName"`
	ContentType       string       `json:"contentType" bson:"contentType"`
	Artifact          []byte       `json:"-" bson:"artifact"`
	ArtifactHash      string       `json:"artifactHash" bson:"artifactHash"`
	RowCount          int          `json:"rowCount" bson:"rowCount"`
	GeneratedAt       time.Time    `json:"generatedAt" bson:"generatedAt"`
}

type Delivery struct {
	ID              platform.ID `json:"id" bson:"_id"`
	OrganizationID  platform.ID `json:"organizationId" bson:"organizationId"`
	ScheduleID      platform.ID `json:"scheduleId" bson:"scheduleId"`
	RunID           platform.ID `json:"runId" bson:"runId"`
	RecipientUserID platform.ID `json:"recipientUserId" bson:"recipientUserId"`
	RecipientHint   string      `json:"recipientHint" bson:"recipientHint"`
	State           string      `json:"state" bson:"state"`
	ArtifactHash    string      `json:"artifactHash" bson:"artifactHash"`
	ExpiresAt       time.Time   `json:"expiresAt" bson:"expiresAt"`
	CreatedAt       time.Time   `json:"createdAt" bson:"createdAt"`
	DeliveredAt     *time.Time  `json:"deliveredAt,omitempty" bson:"deliveredAt,omitempty"`
	OpenedAt        *time.Time  `json:"openedAt,omitempty" bson:"openedAt,omitempty"`
}

type ApprovedRecipient struct {
	UserID platform.ID
	Email  string
	Name   string
}
type PrincipalResolver interface {
	ResolveReportingPrincipal(context.Context, platform.ID, platform.ID) (platform.Principal, bool, error)
}
type PrincipalResolverFunc func(context.Context, platform.ID, platform.ID) (platform.Principal, bool, error)

func (f PrincipalResolverFunc) ResolveReportingPrincipal(ctx context.Context, org, id platform.ID) (platform.Principal, bool, error) {
	return f(ctx, org, id)
}

type DeliverySender interface {
	Send(string, string, string) error
}

type ScheduleRepository interface {
	FindStaffRecipients(context.Context, platform.ID, []platform.ID) ([]ApprovedRecipient, error)
	InsertSchedule(context.Context, Schedule) error
	ListSchedules(context.Context, platform.ID, platform.ID) ([]Schedule, error)
	ClaimDueSchedule(context.Context, time.Time) (*Schedule, error)
	CompleteSchedule(context.Context, platform.ID, time.Time, time.Time) error
	PauseSchedule(context.Context, platform.ID, string, time.Time) error
	InsertBoardPackRun(context.Context, BoardPackRun) error
	InsertDelivery(context.Context, Delivery) error
	MarkDeliverySent(context.Context, platform.ID, time.Time) error
	MarkDeliveryFailed(context.Context, platform.ID, time.Time) error
	FindDelivery(context.Context, platform.ID, platform.ID, platform.ID) (*Delivery, error)
	FindBoardPackRun(context.Context, platform.ID, platform.ID) (*BoardPackRun, error)
	MarkDeliveryOpened(context.Context, platform.ID, time.Time) error
	MarkDeliveryExpired(context.Context, platform.ID, time.Time) error
}

func (s Service) CreateSchedule(ctx context.Context, p platform.Principal, organizationID platform.ID, input ScheduleInput, requestID string) (*Schedule, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Cadence = strings.ToLower(strings.TrimSpace(input.Cadence))
	input.Format = strings.ToLower(strings.TrimSpace(input.Format))
	if input.Name == "" || len(input.Name) > 100 || !map[string]bool{"weekly": true, "monthly": true}[input.Cadence] || !map[string]bool{"csv": true, "board-pack": true}[input.Format] || input.SavedViewVersion < 1 || len(input.RecipientUserIDs) == 0 || len(input.RecipientUserIDs) > 20 {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_schedule", Message: "Choose a saved view version, weekly or monthly cadence, CSV or board pack, and one to twenty approved recipients."})
	}
	now := s.now()
	if !input.NextRunAt.After(now) || input.NextRunAt.After(now.AddDate(1, 0, 0)) {
		return nil, platform.ValidationError(platform.FieldError{Path: "nextRunAt", Code: "invalid", Message: "Choose a first run within the next year."})
	}
	views, err := s.ListViews(ctx, p, organizationID)
	if err != nil {
		return nil, err
	}
	var view *SavedView
	for i := range views {
		if views[i].ID == input.SavedViewID {
			view = &views[i]
			break
		}
	}
	if view == nil || view.Version != input.SavedViewVersion {
		return nil, &platform.DomainError{Code: "conflict", Message: "The saved view changed; reload it before scheduling."}
	}
	if err := s.validateQuery(p, organizationID, view.Query); err != nil {
		return nil, err
	}
	repo, ok := s.Repository.(ScheduleRepository)
	if !ok {
		return nil, errors.New("schedule repository is unavailable")
	}
	recipients, err := repo.FindStaffRecipients(ctx, organizationID, uniqueIDs(input.RecipientUserIDs))
	if err != nil {
		return nil, err
	}
	if len(recipients) != len(uniqueIDs(input.RecipientUserIDs)) {
		return nil, platform.ValidationError(platform.FieldError{Path: "recipientUserIds", Code: "unapproved", Message: "Every recipient must be an active REMI staff account."})
	}
	schedule := Schedule{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: organizationID, OwnerID: p.Actor.ID, SavedViewID: view.ID, SavedViewVersion: view.Version, Name: input.Name, Cadence: input.Cadence, Format: input.Format, RecipientUserIDs: uniqueIDs(input.RecipientUserIDs), State: "active", NextRunAt: input.NextRunAt.UTC(), Version: 1, CreatedAt: now, UpdatedAt: now}
	if s.Evidence == nil {
		return nil, errors.New("schedule evidence store is unavailable")
	}
	err = s.Evidence.WithTransaction(ctx, func(tx context.Context) error {
		if err := repo.InsertSchedule(tx, schedule); err != nil {
			return err
		}
		return s.Evidence.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: organizationID, BranchID: view.Query.BranchID, Actor: p.Actor, Action: "report.schedule.create", ResourceType: "report-schedule", ResourceID: schedule.ID, ChangedFields: []string{"savedViewVersion", "cadence", "format", "recipients", "nextRunAt"}, Outcome: "success", RequestID: requestID, OccurredAt: now})
	})
	if err != nil {
		return nil, err
	}
	return &schedule, nil
}

func (s Service) ListSchedules(ctx context.Context, p platform.Principal, organizationID platform.ID) ([]Schedule, error) {
	repo, ok := s.Repository.(ScheduleRepository)
	if !ok {
		return nil, errors.New("schedule repository is unavailable")
	}
	return repo.ListSchedules(ctx, organizationID, p.Actor.ID)
}

func (s Service) ProcessDueSchedule(ctx context.Context) (bool, error) {
	repo, ok := s.Repository.(ScheduleRepository)
	if !ok || s.Principals == nil {
		return false, nil
	}
	now := s.now()
	schedule, err := repo.ClaimDueSchedule(ctx, now)
	if err != nil || schedule == nil {
		return false, err
	}
	p, active, err := s.Principals.ResolveReportingPrincipal(ctx, schedule.OrganizationID, schedule.OwnerID)
	if err != nil {
		return true, err
	}
	if !active {
		_ = repo.PauseSchedule(ctx, schedule.ID, "owner_inactive", now)
		return true, nil
	}
	view, err := s.Repository.FindSavedView(ctx, schedule.OrganizationID, schedule.OwnerID, schedule.SavedViewID)
	if err != nil || view == nil || view.Version != schedule.SavedViewVersion {
		_ = repo.PauseSchedule(ctx, schedule.ID, "saved_view_changed", now)
		return true, err
	}
	result, err := s.RunReport(ctx, p, schedule.OrganizationID, view.Query)
	if err != nil {
		_ = repo.PauseSchedule(ctx, schedule.ID, "permission_or_query_changed", now)
		return true, nil
	}
	artifact, fileName, contentType, rowCount, err := boardPackArtifact(schedule.Format, *result, *schedule, now)
	if err != nil {
		return true, err
	}
	digest := sha256.Sum256(artifact)
	hash := hex.EncodeToString(digest[:])
	run := BoardPackRun{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: schedule.OrganizationID, ScheduleID: schedule.ID, SavedViewID: view.ID, SavedViewVersion: view.Version, DictionaryVersion: result.DictionaryVersion, OwnerID: schedule.OwnerID, BranchID: view.Query.BranchID, Format: schedule.Format, Result: *result, FileName: fileName, ContentType: contentType, Artifact: artifact, ArtifactHash: hash, RowCount: rowCount, GeneratedAt: now}
	if err = repo.InsertBoardPackRun(ctx, run); err != nil {
		return true, err
	}
	requestID := "schedule:" + string(schedule.ID) + ":" + now.Format(time.RFC3339Nano)
	if s.Evidence != nil {
		_ = s.Evidence.AppendAudit(ctx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: schedule.OrganizationID, BranchID: view.Query.BranchID, Actor: p.Actor, Action: "report.schedule.run", ResourceType: "board-pack-run", ResourceID: run.ID, ChangedFields: []string{"savedViewVersion", "dictionaryVersion", "query", "artifactHash", "rowCount"}, Outcome: "success", RequestID: requestID, OccurredAt: now})
	}
	recipients, err := repo.FindStaffRecipients(ctx, schedule.OrganizationID, schedule.RecipientUserIDs)
	if err != nil {
		return true, err
	}
	for _, recipient := range recipients {
		delivery := Delivery{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: schedule.OrganizationID, ScheduleID: schedule.ID, RunID: run.ID, RecipientUserID: recipient.UserID, RecipientHint: maskRecipient(recipient.Email), State: "pending", ArtifactHash: hash, ExpiresAt: now.Add(7 * 24 * time.Hour), CreatedAt: now}
		if err = repo.InsertDelivery(ctx, delivery); err != nil {
			return true, err
		}
		link := strings.TrimRight(s.AdminAppURL, "/") + "/reports?delivery=" + string(delivery.ID)
		sendErr := error(nil)
		if s.Sender == nil {
			sendErr = errors.New("report delivery sender unavailable")
		} else {
			sendErr = s.Sender.Send(recipient.Email, "Your REMI report is ready", fmt.Sprintf(`<h2>%s</h2><p>A governed REMI report is ready for %s.</p><p><a href="%s">Sign in to view and download</a></p><p>Access expires in seven days and requires your REMI account.</p>`, schedule.Name, recipient.Name, link))
		}
		state, outcome := "delivered", "success"
		if sendErr != nil {
			state, outcome = "failed", "failure"
			_ = repo.MarkDeliveryFailed(ctx, delivery.ID, now)
		} else {
			_ = repo.MarkDeliverySent(ctx, delivery.ID, now)
		}
		if s.Evidence != nil {
			_ = s.Evidence.AppendAudit(ctx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: schedule.OrganizationID, BranchID: view.Query.BranchID, Actor: p.Actor, Action: "report.delivery." + state, ResourceType: "report-delivery", ResourceID: delivery.ID, SubjectIDs: []platform.ID{recipient.UserID}, ChangedFields: []string{"state", "artifactHash", "expiresAt", "recipientUserId"}, Outcome: outcome, RequestID: requestID + ":" + string(recipient.UserID), OccurredAt: now})
		}
	}
	next := nextSchedule(schedule.Cadence, schedule.NextRunAt)
	if !next.After(now) {
		next = nextSchedule(schedule.Cadence, now)
	}
	return true, repo.CompleteSchedule(ctx, schedule.ID, now, next)
}

func (s Service) GetDeliveryArtifact(ctx context.Context, p platform.Principal, organizationID, id platform.ID) (*Delivery, *BoardPackRun, error) {
	repo, ok := s.Repository.(ScheduleRepository)
	if !ok {
		return nil, nil, errors.New("schedule repository is unavailable")
	}
	delivery, err := repo.FindDelivery(ctx, organizationID, p.Actor.ID, id)
	if err != nil || delivery == nil {
		return nil, nil, &platform.DomainError{Code: "not_found", Message: "Report delivery not found."}
	}
	now := s.now()
	requestID := "delivery:" + string(delivery.ID) + ":" + now.Format(time.RFC3339Nano)
	if !delivery.ExpiresAt.After(now) {
		_ = repo.MarkDeliveryExpired(ctx, delivery.ID, now)
		if s.Evidence != nil {
			_ = s.Evidence.AppendAudit(ctx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: organizationID, Actor: p.Actor, Action: "report.delivery.expired", ResourceType: "report-delivery", ResourceID: delivery.ID, Outcome: "denied", RequestID: requestID, OccurredAt: now})
		}
		return nil, nil, &platform.DomainError{Code: "expired", Message: "This report access has expired."}
	}
	run, err := repo.FindBoardPackRun(ctx, organizationID, delivery.RunID)
	if err != nil || run == nil {
		return nil, nil, &platform.DomainError{Code: "not_found", Message: "Report delivery not found."}
	}
	if err = s.validateQuery(p, organizationID, run.Result.Query); err != nil {
		return nil, nil, &platform.DomainError{Code: "not_found", Message: "Report delivery not found."}
	}
	digest := sha256.Sum256(run.Artifact)
	if run.ArtifactHash == "" || run.ArtifactHash != delivery.ArtifactHash || hex.EncodeToString(digest[:]) != run.ArtifactHash {
		return nil, nil, &platform.DomainError{Code: "conflict", Message: "The report artifact failed its integrity check."}
	}
	if delivery.OpenedAt == nil {
		_ = repo.MarkDeliveryOpened(ctx, delivery.ID, now)
		if s.Evidence != nil {
			_ = s.Evidence.AppendAudit(ctx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: organizationID, BranchID: run.BranchID, Actor: p.Actor, Action: "report.delivery.opened", ResourceType: "report-delivery", ResourceID: delivery.ID, ChangedFields: []string{"openedAt"}, Outcome: "success", RequestID: requestID, OccurredAt: now})
		}
	}
	return delivery, run, nil
}

func boardPackArtifact(format string, result ReportResult, schedule Schedule, now time.Time) ([]byte, string, string, int, error) {
	csvBody, rows, err := reportCSV(result)
	if err != nil {
		return nil, "", "", 0, err
	}
	stem := "remi-" + safeFileName(schedule.Name) + "-" + now.Format("20060102")
	if format == "csv" {
		return csvBody, stem + ".csv", "text/csv; charset=utf-8", rows, nil
	}
	buffer := &bytes.Buffer{}
	writer := zip.NewWriter(buffer)
	manifest, _ := json.MarshalIndent(map[string]any{"scheduleId": schedule.ID, "savedViewId": schedule.SavedViewID, "savedViewVersion": schedule.SavedViewVersion, "dictionaryVersion": result.DictionaryVersion, "query": result.Query, "generatedAt": result.GeneratedAt, "rowCount": rows}, "", "  ")
	for name, body := range map[string][]byte{"report.csv": csvBody, "manifest.json": manifest} {
		entry, createErr := writer.Create(name)
		if createErr != nil {
			return nil, "", "", 0, createErr
		}
		if _, createErr = entry.Write(body); createErr != nil {
			return nil, "", "", 0, createErr
		}
	}
	if err = writer.Close(); err != nil {
		return nil, "", "", 0, err
	}
	return buffer.Bytes(), stem + ".zip", "application/zip", rows, nil
}
func nextSchedule(cadence string, from time.Time) time.Time {
	if cadence == "weekly" {
		return from.AddDate(0, 0, 7)
	}
	return from.AddDate(0, 1, 0)
}
func uniqueIDs(values []platform.ID) []platform.ID {
	seen := map[platform.ID]bool{}
	out := []platform.ID{}
	for _, id := range values {
		if id.Valid() && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
func maskRecipient(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 || parts[0] == "" {
		return "approved staff"
	}
	return parts[0][:1] + "***@" + parts[1]
}
func safeFileName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var out strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			out.WriteRune(r)
		} else if out.Len() > 0 && !strings.HasSuffix(out.String(), "-") {
			out.WriteByte('-')
		}
	}
	result := strings.Trim(out.String(), "-")
	if result == "" {
		return "board-pack"
	}
	return result
}
