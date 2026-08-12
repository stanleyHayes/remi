package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"remi-api/internal/chms/care"
	"remi-api/internal/chms/communications"
	"remi-api/internal/chms/community"
	"remi-api/internal/chms/engagement"
	"remi-api/internal/chms/finance"
	"remi-api/internal/chms/governance"
	chmsimports "remi-api/internal/chms/imports"
	"remi-api/internal/chms/participation"
	"remi-api/internal/chms/people"
	"remi-api/internal/chms/platform"
	"remi-api/internal/chms/reporting"
	"remi-api/internal/middleware"
)

type PersonReader interface {
	FindByID(context.Context, platform.ID, platform.ID) (*people.Person, error)
}
type HouseholdReader interface {
	FindProfileByPersonID(context.Context, platform.ID, platform.ID) (*people.HouseholdProfile, error)
	FindProfileByHouseholdID(context.Context, platform.ID, platform.ID) (*people.HouseholdProfile, error)
	FindHouseholdByID(context.Context, platform.ID, platform.ID) (*people.Household, error)
	FindActiveMembership(context.Context, platform.ID, platform.ID, platform.ID) (*people.HouseholdMembership, error)
	FindRelationship(context.Context, platform.ID, platform.ID) (*people.Relationship, error)
	ListHouseholds(context.Context, platform.ID, platform.ID, int64) ([]people.Household, error)
}
type JourneyReader interface {
	ListMembershipEvents(context.Context, platform.ID, platform.ID, int64) ([]people.MembershipEvent, error)
}

// Services contains only the already-authorized domain operations exposed by
// this first people/household HTTP slice.
type Services struct {
	Search           people.SearchService
	People           people.Service
	HouseholdService people.HouseholdService
	Segments         people.SegmentService
	Merge            people.MergeService
	Bulk             people.BulkService
	Imports          chmsimports.Service
	Participation    participation.Service
	Community        community.Service
	Communications   communications.Service
	Engagement       engagement.Service
	Care             care.Service
	Finance          finance.Service
	Reporting        reporting.Service
	Audit            platform.AuditService
	Governance       governance.Service
	Repository       PersonReader
	Households       HouseholdReader
	Journey          JourneyReader
	Authorizer       platform.Authorizer
}

type Handler struct {
	services         Services
	organizationID   platform.ID
	resolvePrincipal func(*http.Request) (platform.Principal, bool)
}

func New(services Services, organizationID platform.ID) *Handler {
	return &Handler{services: services, organizationID: organizationID}
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(platform.RequestIDMiddleware)
	r.Get("/people", h.listPeople)
	r.Post("/people", h.createPerson)
	r.Get("/people/{personId}", h.getPerson)
	r.Patch("/people/{personId}", h.updatePerson)
	r.Post("/people/{personId}/archive", h.archivePerson)
	r.Post("/people/{personId}/restore", h.restorePerson)
	r.Post("/people/{personId}/membership-transitions", h.transitionMembership)
	r.Post("/people/{personId}/membership-transitions/{eventId}/reverse", h.reverseMembership)
	r.Post("/households", h.createHousehold)
	r.Get("/households", h.listHouseholds)
	r.Get("/households/{householdId}", h.getHousehold)
	r.Patch("/households/{householdId}", h.updateHousehold)
	r.Post("/households/{householdId}/members", h.addHouseholdMember)
	r.Delete("/households/{householdId}/members/{personId}", h.removeHouseholdMember)
	r.Post("/relationships", h.createRelationship)
	r.Post("/relationships/{relationshipId}/end", h.endRelationship)
	r.Get("/people/{personId}/duplicate-candidates", h.duplicateCandidates)
	r.Post("/people/merge", h.mergePeople)
	r.Get("/people-segments", h.listSegments)
	r.Post("/people-segments", h.createSegment)
	r.Put("/people-segments/{segmentId}", h.updateSegment)
	r.Delete("/people-segments/{segmentId}", h.archiveSegment)
	r.Get("/communication-audiences", h.listCommunicationAudiences)
	r.Post("/communication-audiences", h.createCommunicationAudience)
	r.Put("/communication-audiences/{audienceId}", h.updateCommunicationAudience)
	r.Get("/communication-audiences/{audienceId}/preview", h.previewCommunicationAudience)
	r.Post("/communication-audiences/{audienceId}/exports", h.exportCommunicationAudience)
	r.Get("/communication-audiences/{audienceId}/exports", h.listCommunicationAudienceExports)
	r.Get("/communication-templates", h.listCommunicationTemplates)
	r.Post("/communication-templates", h.createCommunicationTemplate)
	r.Put("/communication-templates/{templateId}", h.updateCommunicationTemplate)
	r.Post("/communication-templates/{templateId}/publish", h.publishCommunicationTemplate)
	r.Get("/communication-campaigns", h.listCommunicationCampaigns)
	r.Post("/communication-campaigns", h.createCommunicationCampaign)
	r.Get("/communication-campaigns/{campaignId}", h.getCommunicationCampaign)
	r.Post("/communication-campaigns/{campaignId}/submit", h.submitCommunicationCampaign)
	r.Post("/communication-campaigns/{campaignId}/approve", h.approveCommunicationCampaign)
	r.Post("/communication-campaigns/{campaignId}/schedule", h.scheduleCommunicationCampaign)
	r.Post("/people/bulk/preview", h.previewBulk)
	r.Post("/people/bulk/previews/{previewId}/start", h.startBulk)
	r.Post("/people/bulk/jobs/{jobId}/run", h.runBulk)
	r.Post("/people/bulk/jobs/{jobId}/undo", h.undoBulk)
	r.Post("/imports", h.stageImport)
	r.Put("/imports/{runId}/mapping", h.mapImport)
	r.Post("/imports/{runId}/validate", h.validateImport)
	r.Post("/imports/{runId}/commit", h.commitImport)
	r.Post("/imports/{runId}/rollback", h.rollbackImport)
	r.Get("/service-definitions", h.listServiceDefinitions)
	r.Post("/service-definitions", h.createServiceDefinition)
	r.Get("/service-definitions/{definitionId}", h.getServiceDefinition)
	r.Patch("/service-definitions/{definitionId}", h.updateServiceDefinition)
	r.Post("/service-definitions/{definitionId}/occurrences:generate", h.generateOccurrences)
	r.Get("/occurrences", h.listOccurrences)
	r.Post("/occurrences", h.createOccurrence)
	r.Get("/occurrences/{occurrenceId}", h.getOccurrence)
	r.Patch("/occurrences/{occurrenceId}", h.updateOccurrence)
	r.Post("/occurrences/{occurrenceId}/cancel", h.cancelOccurrence)
	r.Get("/occurrences/{occurrenceId}/attendance", h.listAttendance)
	r.Put("/occurrences/{occurrenceId}/attendance/{personId}", h.recordAttendance)
	r.Post("/occurrences/{occurrenceId}/attendance-corrections", h.correctAttendance)
	r.Get("/occurrences/{occurrenceId}/attendance/{personId}/events", h.listAttendanceEvents)
	r.Get("/occurrences/{occurrenceId}/headcounts", h.listHeadcounts)
	r.Post("/occurrences/{occurrenceId}/headcounts", h.createHeadcount)
	r.Patch("/occurrences/{occurrenceId}/headcounts/{headcountId}", h.updateHeadcount)
	r.Post("/occurrences/{occurrenceId}/lock-attendance", h.lockAttendance)
	r.Get("/occurrences/{occurrenceId}/attendance-control", h.getAttendanceControl)
	r.Get("/attendance-periods", h.listAttendancePeriods)
	r.Post("/attendance-periods/close", h.closeAttendancePeriod)
	r.Get("/attendance-analytics", h.getAttendanceAnalytics)
	r.Get("/reporting/metrics", h.getReportingMetrics)
	r.Post("/reporting/query", h.runReportingQuery)
	r.Get("/reporting/saved-views", h.listReportingViews)
	r.Post("/reporting/saved-views", h.createReportingView)
	r.Put("/reporting/saved-views/{viewId}", h.updateReportingView)
	r.Get("/reporting/export-runs", h.listReportingExports)
	r.Post("/reporting/export-runs", h.createReportingExport)
	r.Get("/reporting/export-runs/{exportId}", h.getReportingExport)
	r.Get("/reporting/export-runs/{exportId}/download", h.downloadReportingExport)
	r.Get("/reporting/schedules", h.listReportingSchedules)
	r.Post("/reporting/schedules", h.createReportingSchedule)
	r.Get("/reporting/deliveries/{deliveryId}", h.getReportingDelivery)
	r.Get("/reporting/deliveries/{deliveryId}/download", h.downloadReportingDelivery)
	r.Get("/audit-events", h.listAuditEvents)
	r.Post("/audit-events/export", h.exportAuditEvents)
	r.Get("/data-requests", h.listDataRequests)
	r.Get("/data-requests/{requestId}", h.getDataRequest)
	r.Post("/data-requests/{requestId}/transitions", h.transitionDataRequest)
	r.Post("/checkin/sessions", h.createCheckinSession)
	r.Get("/checkin/sessions/{sessionId}", h.getCheckinSession)
	r.Post("/checkin/sessions/{sessionId}/commands:sync", h.syncCheckinCommands)
	r.Post("/checkin/sessions/{sessionId}/lock", h.lockCheckinSession)
	r.Get("/checkin/sessions/{sessionId}/households", h.lookupCheckinHouseholds)
	r.Post("/checkin/sessions/{sessionId}/guests", h.createCheckinGuest)
	r.Post("/guardian-authorizations", h.createGuardianAuthorization)
	r.Post("/checkin/sessions/{sessionId}/children", h.checkinChild)
	r.Post("/checkin/{attendanceId}/pickup", h.pickupChild)
	r.Post("/child-checkins/{checkinId}/incidents", h.createSafeguardingIncident)
	r.Get("/groups", h.listGroups)
	r.Get("/leader-workspace", h.getLeaderWorkspace)
	r.Get("/leader-workspace/groups/{groupId}", h.getLeaderGroupWorkspace)
	r.Get("/leader-workspace/teams/{teamId}", h.getLeaderTeamWorkspace)
	r.Get("/community-analytics", h.getCommunityAnalytics)
	r.Get("/engagement/rules", h.listEngagementRules)
	r.Post("/engagement/rules", h.createEngagementRule)
	r.Get("/engagement/rules/{ruleId}", h.getEngagementRule)
	r.Patch("/engagement/rules/{ruleId}", h.updateEngagementRule)
	r.Post("/engagement/rules/{ruleId}/publish", h.publishEngagementRule)
	r.Post("/engagement/rules/{ruleId}/generate", h.generateEngagementSignals)
	r.Get("/engagement/signals", h.listEngagementSignals)
	r.Get("/engagement/cohorts", h.getEngagementCohorts)
	r.Get("/engagement/safety-readiness", h.getEngagementSafetyReadiness)
	r.Post("/engagement/safety-reviews", h.submitEngagementSafetyReview)
	r.Get("/engagement/signals/{signalId}", h.getEngagementSignal)
	r.Get("/engagement/signals/{signalId}/review", h.getEngagementReview)
	r.Post("/engagement/signals/{signalId}/assign", h.assignEngagementSignal)
	r.Post("/engagement/signals/{signalId}/snooze", h.snoozeEngagementSignal)
	r.Post("/engagement/signals/{signalId}/suppress", h.suppressEngagementSignal)
	r.Post("/engagement/signals/{signalId}/contact-events", h.recordEngagementContact)
	r.Post("/engagement/signals/{signalId}/resolve", h.resolveEngagementSignal)
	r.Post("/groups", h.createGroup)
	r.Get("/groups/{groupId}", h.getGroup)
	r.Patch("/groups/{groupId}", h.updateGroup)
	r.Get("/groups/{groupId}/members", h.listGroupMembers)
	r.Post("/groups/{groupId}/members", h.createGroupMember)
	r.Get("/groups/{groupId}/members/{personId}/history", h.listGroupMemberHistory)
	r.Patch("/groups/{groupId}/members/{personId}", h.transitionGroupMember)
	r.Delete("/groups/{groupId}/members/{personId}", h.endGroupMember)
	r.Get("/groups/{groupId}/meetings", h.listGroupMeetings)
	r.Post("/groups/{groupId}/meetings", h.createGroupMeeting)
	r.Get("/groups/{groupId}/meetings/{meetingId}/attendance", h.listGroupMeetingAttendance)
	r.Put("/groups/{groupId}/meetings/{meetingId}/attendance/{personId}", h.recordGroupMeetingAttendance)
	r.Post("/groups/{groupId}/communication-handoffs", h.requestGroupCommunication)
	r.Get("/teams", h.listVolunteerTeams)
	r.Post("/teams", h.createVolunteerTeam)
	r.Get("/teams/{teamId}", h.getVolunteerTeam)
	r.Patch("/teams/{teamId}", h.updateVolunteerTeam)
	r.Get("/teams/{teamId}/positions", h.listVolunteerPositions)
	r.Post("/teams/{teamId}/positions", h.createVolunteerPosition)
	r.Patch("/teams/{teamId}/positions/{positionId}", h.updateVolunteerPosition)
	r.Get("/people/{personId}/volunteer-profile", h.getVolunteerProfile)
	r.Put("/people/{personId}/volunteer-profile", h.putVolunteerProfile)
	r.Get("/people/{personId}/availability", h.listVolunteerAvailability)
	r.Post("/people/{personId}/availability", h.addVolunteerAvailability)
	r.Get("/service-plans", h.listServicePlans)
	r.Post("/service-plans", h.createServicePlan)
	r.Get("/service-plans/{planId}", h.getServicePlan)
	r.Patch("/service-plans/{planId}/status", h.transitionServicePlan)
	r.Get("/teams/{teamId}/rotations", h.listVolunteerRotations)
	r.Post("/teams/{teamId}/rotations", h.createVolunteerRotation)
	r.Get("/service-plans/{planId}/assignments", h.listVolunteerAssignments)
	r.Post("/service-plans/{planId}/assignments:preview", h.previewVolunteerAssignment)
	r.Post("/service-plans/{planId}/assignments", h.createVolunteerAssignment)
	r.Patch("/assignments/{assignmentId}/response", h.respondVolunteerAssignment)
	r.Post("/assignments/{assignmentId}/substitute", h.substituteVolunteerAssignment)
	r.Post("/assignments/{assignmentId}/reminders", h.recordVolunteerReminder)
	r.Post("/assignments/{assignmentId}/no-show", h.markVolunteerNoShow)
	r.Get("/assignments/{assignmentId}/events", h.listVolunteerAssignmentEvents)
	r.Get("/workflow-definitions", h.listWorkflowDefinitions)
	r.Post("/workflow-definitions", h.createWorkflowDefinition)
	r.Get("/workflow-definitions/{definitionId}", h.getWorkflowDefinition)
	r.Patch("/workflow-definitions/{definitionId}", h.updateWorkflowDefinition)
	r.Post("/workflow-definitions/{definitionId}/publish", h.publishWorkflowDefinition)
	r.Get("/workflow-instances", h.listWorkflowInstances)
	r.Post("/workflow-instances", h.startWorkflowInstance)
	r.Get("/workflow-instances/{instanceId}", h.getWorkflowInstance)
	r.Post("/workflow-instances/{instanceId}/transitions", h.transitionWorkflowInstance)
	r.Post("/workflow-instances/{instanceId}/reassign", h.reassignWorkflowInstance)
	r.Post("/tasks/{taskId}/complete", h.completeWorkflowTask)
	r.Post("/tasks/{taskId}/reminders", h.remindWorkflowTask)
	r.Get("/assimilation/progress", h.listAssimilationProgress)
	r.Post("/assimilation/attendance-triggers", h.recordAssimilationAttendance)
	r.Get("/care-cases", h.listCareCases)
	r.Post("/care-cases", h.createCareCase)
	r.Get("/care-cases/{caseId}", h.getCareCase)
	r.Post("/care-cases/{caseId}/assign", h.assignCareCase)
	r.Post("/care-cases/{caseId}/close", h.closeCareCase)
	r.Get("/care-cases/{caseId}/notes", h.listCareNotes)
	r.Post("/care-cases/{caseId}/notes", h.addCareNote)
	r.Post("/care-cases/{caseId}/contact-events", h.addCareContactEvent)
	r.Get("/finance/funds", h.listFinanceFunds)
	r.Post("/finance/funds", h.createFinanceFund)
	r.Patch("/finance/funds/{id}", h.updateFinanceFund)
	r.Post("/finance/funds/{id}/deactivate", h.deactivateFinanceFund)
	r.Get("/finance/campaigns", h.listFinanceCampaigns)
	r.Post("/finance/campaigns", h.createFinanceCampaign)
	r.Patch("/finance/campaigns/{id}", h.updateFinanceCampaign)
	r.Get("/finance/pledges", h.listFinancePledges)
	r.Post("/finance/pledges", h.createFinancePledge)
	r.Get("/finance/pledges/{id}", h.getFinancePledge)
	r.Patch("/finance/pledges/{id}", h.updateFinancePledge)
	r.Post("/finance/pledges/{id}/reminders", h.remindFinancePledge)
	r.Get("/finance/settlements", h.listFinanceSettlements)
	r.Post("/finance/settlements", h.importFinanceSettlement)
	r.Get("/finance/settlements/{id}", h.getFinanceSettlement)
	r.Get("/finance/settlements/{id}/items", h.listFinanceReconciliationItems)
	r.Get("/finance/reconciliation-owners", h.listFinanceReconciliationOwners)
	r.Post("/finance/reconciliation-items/{id}/resolve", h.resolveFinanceReconciliationItem)
	r.Get("/finance/payment-methods", h.listFinancePaymentMethods)
	r.Post("/finance/payment-methods", h.createFinancePaymentMethod)
	r.Patch("/finance/payment-methods/{id}", h.updateFinancePaymentMethod)
	r.Get("/finance/campuses", h.listFinanceCampuses)
	r.Post("/finance/campuses", h.createFinanceCampus)
	r.Patch("/finance/campuses/{id}", h.updateFinanceCampus)
	r.Get("/finance/periods", h.listFinancePeriods)
	r.Post("/finance/periods", h.createFinancePeriod)
	r.Patch("/finance/periods/{id}", h.updateFinancePeriod)
	r.Post("/finance/periods/{id}/close", h.closeFinancePeriod)
	r.Post("/finance/periods/{id}/reopen", h.reopenFinancePeriod)
	r.Get("/finance/period-control-requests", h.listFinancePeriodControlRequests)
	r.Get("/finance/receipt-sequences", h.listFinanceSequences)
	r.Post("/finance/receipt-sequences", h.createFinanceSequence)
	r.Patch("/finance/receipt-sequences/{id}", h.updateFinanceSequence)
	r.Get("/finance/account-mappings", h.listFinanceMappings)
	r.Post("/finance/account-mappings", h.createFinanceMapping)
	r.Patch("/finance/account-mappings/{id}", h.updateFinanceMapping)
	r.Get("/finance/contributions", h.listFinanceContributions)
	r.Get("/finance/payment-intents", h.listFinancePaymentIntents)
	r.Get("/finance/provider-exceptions", h.listFinanceProviderExceptions)
	r.Post("/finance/contributions", h.postFinanceContribution)
	r.Get("/finance/contributions/{id}", h.getFinanceContribution)
	r.Post("/finance/contributions/{id}/adjustments", h.adjustFinanceContribution)
	r.Post("/finance/contributions/{id}/attribution", h.correctFinanceContributionAttribution)
	r.Get("/finance/reports/summary", h.getFinanceReport)
	r.Get("/finance/export-runs", h.listFinanceExportRuns)
	r.Post("/finance/export-runs", h.createFinanceExportRun)
	r.Get("/finance/export-runs/{id}", h.getFinanceExportRun)
	r.Get("/finance/export-runs/{id}/download", h.downloadFinanceExportRun)
	r.Get("/finance/statements", h.listFinanceStatements)
	r.Get("/finance/statement-subjects", h.searchFinanceStatementSubjects)
	r.Post("/finance/statements", h.generateFinanceStatement)
	r.Get("/finance/statements/{id}", h.getFinanceStatement)
	r.Get("/finance/statements/{id}/pdf", h.downloadFinanceStatement)
	r.Post("/finance/statements/{id}/deliveries", h.deliverFinanceStatement)
	r.Get("/finance/receipts", h.listFinanceReceipts)
	r.Get("/finance/receipts/{id}/pdf", h.downloadFinanceReceipt)
	r.Post("/finance/receipts/{id}/deliveries", h.deliverFinanceReceipt)
	r.Get("/finance/batches", h.listFinanceBatches)
	r.Post("/finance/batches", h.createFinanceBatch)
	r.Get("/finance/batches/{id}", h.getFinanceBatch)
	r.Post("/finance/batches/{id}/entries", h.createFinanceBatchEntry)
	r.Patch("/finance/batches/{id}/entries/{entryId}", h.updateFinanceBatchEntry)
	r.Post("/finance/batches/{id}/confirmations", h.confirmFinanceBatch)
	r.Post("/finance/batches/{id}/transitions", h.transitionFinanceBatch)
	return r
}

// MemberRoutes exposes only self-scoped member operations. It must be mounted
// separately from Routes, which is a staff-only administrative surface.
func (h *Handler) MemberRoutes() http.Handler {
	r := chi.NewRouter()
	r.Use(platform.RequestIDMiddleware)
	r.Get("/serving/assignments", h.listOwnVolunteerAssignments)
	r.Patch("/serving/assignments/{assignmentId}/response", h.respondOwnVolunteerAssignment)
	r.Get("/finance/pledges", h.listOwnFinancePledges)
	r.Post("/finance/pledges", h.createOwnFinancePledge)
	r.Get("/finance/pledges/{id}", h.getOwnFinancePledge)
	r.Patch("/finance/pledges/{id}", h.updateOwnFinancePledge)
	r.Get("/finance/statements", h.listOwnFinanceStatements)
	r.Post("/finance/statements", h.generateOwnFinanceStatement)
	r.Get("/finance/statements/{id}", h.getOwnFinanceStatement)
	r.Get("/finance/statements/{id}/pdf", h.downloadOwnFinanceStatement)
	r.Post("/finance/statements/{id}/deliveries", h.deliverOwnFinanceStatement)
	r.Get("/finance/receipts", h.listOwnFinanceReceipts)
	r.Get("/finance/receipts/{id}/pdf", h.downloadOwnFinanceReceipt)
	r.Post("/finance/receipts/{id}/deliveries", h.deliverOwnFinanceReceipt)
	r.Get("/finance/campaigns", h.listOwnFinanceCampaigns)
	r.Post("/finance/giving-intents", h.createOwnGivingIntent)
	r.Get("/finance/payment-methods", h.listOwnPaymentMethods)
	r.Delete("/finance/payment-methods/{id}", h.disableOwnPaymentMethod)
	r.Get("/finance/recurring-instructions", h.listOwnRecurringInstructions)
	r.Post("/finance/recurring-instructions", h.createOwnRecurringInstruction)
	r.Patch("/finance/recurring-instructions/{id}", h.updateOwnRecurringInstruction)
	return r
}

func (h *Handler) respondOwnVolunteerAssignment(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFrom(r)
	if claims == nil || claims.Role != "member" || !platform.ID(claims.PersonID).Valid() {
		platform.WriteError(w, r, &platform.DomainError{Code: "forbidden", Message: "Member access is required."})
		return
	}
	var input community.AssignmentResponseInput
	if err := platform.DecodeJSON(w, r, &input, 1<<20); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	principal := platform.Principal{Actor: platform.Actor{Type: platform.ActorMember, ID: platform.ID(claims.PersonID)}, OrganizationID: h.organizationID, Roles: []string{"member"}}
	value, err := h.services.Community.RespondAssignment(r.Context(), principal, platform.ID(chi.URLParam(r, "assignmentId")), input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (h *Handler) listOwnVolunteerAssignments(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	now := time.Now().UTC()
	from, err := parseOptionalTime(r.URL.Query().Get("from"), now.Add(-24*time.Hour))
	if err != nil {
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "from", Code: "invalid_time", Message: "Use an ISO-8601 date and time."}))
		return
	}
	to, err := parseOptionalTime(r.URL.Query().Get("to"), now.Add(180*24*time.Hour))
	if err != nil {
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "to", Code: "invalid_time", Message: "Use an ISO-8601 date and time."}))
		return
	}
	values, err := h.services.Community.ListOwnAssignments(r.Context(), principal, from, to)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (h *Handler) listPeople(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	limit, err := strconv.Atoi(defaultString(r.URL.Query().Get("limit"), "50"))
	if err != nil {
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "limit", Code: "invalid_limit", Message: "Enter a valid limit."}))
		return
	}
	request := people.PersonSearchRequest{Filter: people.PersonSearchFilter{Query: r.URL.Query().Get("q"), BranchID: platform.ID(r.URL.Query().Get("branchId")), MembershipStages: r.URL.Query()["membershipStage"], Tags: r.URL.Query()["tag"], Archived: r.URL.Query().Get("archived") == "true"}, Cursor: r.URL.Query().Get("cursor"), Limit: limit, IncludeContacts: r.URL.Query().Get("includeContacts") == "true"}
	page, searchErr := h.services.Search.Search(r.Context(), principal, request)
	if searchErr != nil {
		platform.WriteError(w, r, searchErr)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) createPerson(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input people.CreateInput
	if err := platform.DecodeJSON(w, r, &input, 1<<20); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	input.OrganizationID = h.organizationID
	decision := h.services.Authorizer.Authorize(principal, platform.AccessRequest{Action: "create", ResourceType: "person", OrganizationID: h.organizationID, BranchID: input.HomeBranchID, FieldClasses: []platform.FieldClass{platform.FieldPersonal}, Now: time.Now().UTC()})
	if !decision.Allowed {
		platform.WriteError(w, r, &platform.DomainError{Code: "forbidden", Message: "You do not have access to create people in this branch."})
		return
	}
	created, duplicates, err := h.services.People.Create(r.Context(), input, principal.Actor, platform.RequestIDFrom(r.Context()))
	if err != nil {
		if domain, yes := err.(*platform.DomainError); yes && domain.Code == "duplicate_candidate" {
			domain.Details = map[string]any{"candidateIds": duplicates}
		}
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("ETag", strconv.Quote(strconv.FormatInt(created.Version, 10)))
	writeJSON(w, http.StatusCreated, created)
}

func (h *Handler) getPerson(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	person, err := h.services.Repository.FindByID(r.Context(), h.organizationID, platform.ID(chi.URLParam(r, "personId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if person == nil {
		platform.WriteError(w, r, &platform.DomainError{Code: "not_found", Message: "Person not found."})
		return
	}
	decision := h.services.Authorizer.Authorize(principal, platform.AccessRequest{Action: "read", ResourceType: "person", OrganizationID: h.organizationID, BranchID: person.HomeBranchID, ResourceID: person.ID, FieldClasses: []platform.FieldClass{platform.FieldPersonal}, Now: time.Now().UTC()})
	if !decision.Allowed {
		platform.WriteError(w, r, &platform.DomainError{Code: "not_found", Message: "Person not found."})
		return
	}
	w.Header().Set("ETag", strconv.Quote(strconv.FormatInt(person.Version, 10)))
	response := map[string]any{"person": person, "permissions": permissionsFor(principal)}
	if h.services.Households != nil && h.allowed(principal, "read", "household", person.HomeBranchID, platform.FieldPersonal) {
		household, householdErr := h.services.Households.FindProfileByPersonID(r.Context(), h.organizationID, person.ID)
		if householdErr != nil {
			platform.WriteError(w, r, householdErr)
			return
		}
		if household != nil {
			response["household"] = household
		}
	}
	if h.services.Journey != nil && h.allowed(principal, "read", "person", person.HomeBranchID, platform.FieldPersonal) {
		journey, journeyErr := h.services.Journey.ListMembershipEvents(r.Context(), h.organizationID, person.ID, 100)
		if journeyErr != nil {
			platform.WriteError(w, r, journeyErr)
			return
		}
		response["journey"] = journey
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) updatePerson(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	personID := platform.ID(chi.URLParam(r, "personId"))
	current, err := h.services.Repository.FindByID(r.Context(), h.organizationID, personID)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if current == nil {
		platform.WriteError(w, r, &platform.DomainError{Code: "not_found", Message: "Person not found."})
		return
	}
	var input people.UpdateInput
	if err := platform.DecodeJSON(w, r, &input, 1<<20); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if input.ExpectedVersion == 0 {
		input.ExpectedVersion = parseIfMatch(r.Header.Get("If-Match"))
	}
	for _, branch := range []platform.ID{current.HomeBranchID, input.HomeBranchID} {
		if !h.allowed(principal, "update", "person", branch, platform.FieldPersonal) {
			platform.WriteError(w, r, &platform.DomainError{Code: "not_found", Message: "Person not found."})
			return
		}
	}
	updated, err := h.services.People.Update(r.Context(), h.organizationID, personID, input, principal.Actor, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("ETag", strconv.Quote(strconv.FormatInt(updated.Version, 10)))
	writeJSON(w, http.StatusOK, updated)
}

type archiveRequest struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	Reason          string `json:"reason"`
}

func (h *Handler) archivePerson(w http.ResponseWriter, r *http.Request) { h.setArchive(w, r, true) }
func (h *Handler) restorePerson(w http.ResponseWriter, r *http.Request) { h.setArchive(w, r, false) }
func (h *Handler) setArchive(w http.ResponseWriter, r *http.Request, archived bool) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	personID := platform.ID(chi.URLParam(r, "personId"))
	current, err := h.services.Repository.FindByID(r.Context(), h.organizationID, personID)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if current == nil {
		platform.WriteError(w, r, &platform.DomainError{Code: "not_found", Message: "Person not found."})
		return
	}
	action := "restore"
	if archived {
		action = "archive"
	}
	if !h.allowed(principal, action, "person", current.HomeBranchID, platform.FieldPersonal) {
		platform.WriteError(w, r, &platform.DomainError{Code: "not_found", Message: "Person not found."})
		return
	}
	var input archiveRequest
	if err := platform.DecodeJSON(w, r, &input, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if input.ExpectedVersion == 0 {
		input.ExpectedVersion = parseIfMatch(r.Header.Get("If-Match"))
	}
	updated, err := h.services.People.SetArchived(r.Context(), h.organizationID, personID, input.ExpectedVersion, archived, input.Reason, principal.Actor, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("ETag", strconv.Quote(strconv.FormatInt(updated.Version, 10)))
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) createHousehold(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input people.CreateHouseholdInput
	if err := platform.DecodeJSON(w, r, &input, 1<<20); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	input.OrganizationID = h.organizationID
	if !h.allowed(principal, "create", "household", input.HomeBranchID, platform.FieldPersonal) {
		platform.WriteError(w, r, &platform.DomainError{Code: "forbidden", Message: "You do not have access to create a household in this branch."})
		return
	}
	for _, member := range input.Members {
		person, err := h.services.Repository.FindByID(r.Context(), h.organizationID, member.PersonID)
		if err != nil {
			platform.WriteError(w, r, err)
			return
		}
		if person == nil || !h.allowed(principal, "update", "person", person.HomeBranchID, platform.FieldPersonal) {
			platform.WriteError(w, r, &platform.DomainError{Code: "not_found", Message: "A household member was not found."})
			return
		}
	}
	household, err := h.services.HouseholdService.Create(r.Context(), input, principal.Actor, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("ETag", strconv.Quote(strconv.FormatInt(household.Version, 10)))
	writeJSON(w, http.StatusCreated, household)
}
func (h *Handler) listHouseholds(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	branchID := platform.ID(r.URL.Query().Get("branchId"))
	if !h.allowed(principal, "read", "household", branchID, platform.FieldPersonal) {
		platform.WriteError(w, r, &platform.DomainError{Code: "forbidden", Message: "You cannot read households in this branch."})
		return
	}
	limit, _ := strconv.ParseInt(defaultString(r.URL.Query().Get("limit"), "50"), 10, 64)
	items, err := h.services.Households.ListHouseholds(r.Context(), h.organizationID, branchID, limit)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
func (h *Handler) updateHousehold(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	id := platform.ID(chi.URLParam(r, "householdId"))
	current, err := h.services.Households.FindHouseholdByID(r.Context(), h.organizationID, id)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if current == nil {
		platform.WriteError(w, r, &platform.DomainError{Code: "not_found", Message: "Household not found."})
		return
	}
	var input people.UpdateHouseholdInput
	if err := platform.DecodeJSON(w, r, &input, 256<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if input.ExpectedVersion == 0 {
		input.ExpectedVersion = parseIfMatch(r.Header.Get("If-Match"))
	}
	for _, branch := range []platform.ID{current.HomeBranchID, input.HomeBranchID} {
		if !h.allowed(principal, "update", "household", branch, platform.FieldPersonal) {
			platform.WriteError(w, r, &platform.DomainError{Code: "not_found", Message: "Household not found."})
			return
		}
	}
	updated, err := h.services.HouseholdService.Update(r.Context(), h.organizationID, id, input, principal.Actor, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("ETag", strconv.Quote(strconv.FormatInt(updated.Version, 10)))
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) getHousehold(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	id := platform.ID(chi.URLParam(r, "householdId"))
	household, err := h.services.Households.FindHouseholdByID(r.Context(), h.organizationID, id)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if household == nil || !h.allowed(principal, "read", "household", household.HomeBranchID, platform.FieldPersonal) {
		platform.WriteError(w, r, &platform.DomainError{Code: "not_found", Message: "Household not found."})
		return
	}
	profile, err := h.services.Households.FindProfileByHouseholdID(r.Context(), h.organizationID, id)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("ETag", strconv.Quote(strconv.FormatInt(household.Version, 10)))
	writeJSON(w, http.StatusOK, map[string]any{"household": household, "profile": profile})
}

type addMemberRequest struct {
	PersonID  platform.ID `json:"personId"`
	Role      string      `json:"role"`
	StartedAt time.Time   `json:"startedAt"`
}

func (h *Handler) addHouseholdMember(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	householdID := platform.ID(chi.URLParam(r, "householdId"))
	household, err := h.services.Households.FindHouseholdByID(r.Context(), h.organizationID, householdID)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if household == nil || !h.allowed(principal, "update", "household", household.HomeBranchID, platform.FieldPersonal) {
		platform.WriteError(w, r, &platform.DomainError{Code: "not_found", Message: "Household not found."})
		return
	}
	var input addMemberRequest
	if err := platform.DecodeJSON(w, r, &input, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	person, err := h.services.Repository.FindByID(r.Context(), h.organizationID, input.PersonID)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if person == nil || !h.allowed(principal, "update", "person", person.HomeBranchID, platform.FieldPersonal) {
		platform.WriteError(w, r, &platform.DomainError{Code: "not_found", Message: "Person not found."})
		return
	}
	membership, err := h.services.HouseholdService.AddMember(r.Context(), h.organizationID, householdID, people.HouseholdMemberInput{PersonID: input.PersonID, Role: input.Role}, input.StartedAt, principal.Actor, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, membership)
}

type removeMemberRequest struct {
	Reason string `json:"reason"`
}

func (h *Handler) removeHouseholdMember(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	householdID := platform.ID(chi.URLParam(r, "householdId"))
	personID := platform.ID(chi.URLParam(r, "personId"))
	household, err := h.services.Households.FindHouseholdByID(r.Context(), h.organizationID, householdID)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	person, personErr := h.services.Repository.FindByID(r.Context(), h.organizationID, personID)
	if personErr != nil {
		platform.WriteError(w, r, personErr)
		return
	}
	if household == nil || person == nil || !h.allowed(principal, "update", "household", household.HomeBranchID, platform.FieldPersonal) || !h.allowed(principal, "update", "person", person.HomeBranchID, platform.FieldPersonal) {
		platform.WriteError(w, r, &platform.DomainError{Code: "not_found", Message: "Household membership not found."})
		return
	}
	membership, err := h.services.Households.FindActiveMembership(r.Context(), h.organizationID, householdID, personID)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if membership == nil {
		platform.WriteError(w, r, &platform.DomainError{Code: "not_found", Message: "Household membership not found."})
		return
	}
	var input removeMemberRequest
	if err := platform.DecodeJSON(w, r, &input, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if err := h.services.HouseholdService.EndMember(r.Context(), h.organizationID, householdID, membership.ID, input.Reason, principal.Actor, platform.RequestIDFrom(r.Context())); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) createRelationship(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input people.Relationship
	if err := platform.DecodeJSON(w, r, &input, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	fields := []platform.FieldClass{platform.FieldPersonal}
	if input.Visibility == "restricted" {
		fields = append(fields, platform.FieldSensitiveMinistry)
	}
	for _, id := range []platform.ID{input.FromPersonID, input.ToPersonID} {
		person, err := h.services.Repository.FindByID(r.Context(), h.organizationID, id)
		if err != nil {
			platform.WriteError(w, r, err)
			return
		}
		if person == nil || !h.allowed(principal, "update", "person", person.HomeBranchID, fields...) {
			platform.WriteError(w, r, &platform.DomainError{Code: "not_found", Message: "A related person was not found."})
			return
		}
	}
	relationship, err := h.services.HouseholdService.CreateRelationship(r.Context(), h.organizationID, input, principal.Actor, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("ETag", strconv.Quote(strconv.FormatInt(relationship.Version, 10)))
	writeJSON(w, http.StatusCreated, relationship)
}

type endRelationshipRequest struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	Reason          string `json:"reason"`
}

func (h *Handler) endRelationship(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	relationshipID := platform.ID(chi.URLParam(r, "relationshipId"))
	relationship, err := h.services.Households.FindRelationship(r.Context(), h.organizationID, relationshipID)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if relationship == nil {
		platform.WriteError(w, r, &platform.DomainError{Code: "not_found", Message: "Relationship not found."})
		return
	}
	fields := []platform.FieldClass{platform.FieldPersonal}
	if relationship.Visibility == "restricted" {
		fields = append(fields, platform.FieldSensitiveMinistry)
	}
	for _, id := range []platform.ID{relationship.FromPersonID, relationship.ToPersonID} {
		person, personErr := h.services.Repository.FindByID(r.Context(), h.organizationID, id)
		if personErr != nil {
			platform.WriteError(w, r, personErr)
			return
		}
		if person == nil || !h.allowed(principal, "update", "person", person.HomeBranchID, fields...) {
			platform.WriteError(w, r, &platform.DomainError{Code: "not_found", Message: "Relationship not found."})
			return
		}
	}
	var input endRelationshipRequest
	if err := platform.DecodeJSON(w, r, &input, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if input.ExpectedVersion == 0 {
		input.ExpectedVersion = parseIfMatch(r.Header.Get("If-Match"))
	}
	ended, err := h.services.HouseholdService.EndRelationship(r.Context(), h.organizationID, relationshipID, input.ExpectedVersion, input.Reason, principal.Actor, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("ETag", strconv.Quote(strconv.FormatInt(ended.Version, 10)))
	writeJSON(w, http.StatusOK, ended)
}

func (h *Handler) duplicateCandidates(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	items, err := h.services.Merge.Candidates(r.Context(), principal, platform.ID(chi.URLParam(r, "personId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
func (h *Handler) mergePeople(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input people.MergeInput
	if err := platform.DecodeJSON(w, r, &input, 256<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	event, err := h.services.Merge.Merge(r.Context(), principal, input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, event)
}
func (h *Handler) listSegments(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	items, err := h.services.Segments.List(r.Context(), principal, platform.ID(r.URL.Query().Get("branchId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
func (h *Handler) createSegment(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input people.SaveSegmentInput
	if err := platform.DecodeJSON(w, r, &input, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	segment, err := h.services.Segments.Create(r.Context(), principal, input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("ETag", strconv.Quote(strconv.FormatInt(segment.Version, 10)))
	writeJSON(w, http.StatusCreated, segment)
}

type updateSegmentRequest struct {
	ExpectedVersion int64                     `json:"expectedVersion"`
	Name            string                    `json:"name"`
	Description     string                    `json:"description"`
	Filter          people.PersonSearchFilter `json:"filter"`
	Visibility      string                    `json:"visibility"`
}

func (h *Handler) updateSegment(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input updateSegmentRequest
	if err := platform.DecodeJSON(w, r, &input, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if input.ExpectedVersion == 0 {
		input.ExpectedVersion = parseIfMatch(r.Header.Get("If-Match"))
	}
	segment, err := h.services.Segments.Update(r.Context(), principal, platform.ID(chi.URLParam(r, "segmentId")), input.ExpectedVersion, people.SaveSegmentInput{Name: input.Name, Description: input.Description, Filter: input.Filter, Visibility: input.Visibility}, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("ETag", strconv.Quote(strconv.FormatInt(segment.Version, 10)))
	writeJSON(w, http.StatusOK, segment)
}
func (h *Handler) archiveSegment(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	version := parseIfMatch(r.Header.Get("If-Match"))
	if version == 0 {
		version, _ = strconv.ParseInt(r.URL.Query().Get("expectedVersion"), 10, 64)
	}
	if err := h.services.Segments.Archive(r.Context(), principal, platform.ID(chi.URLParam(r, "segmentId")), version, platform.RequestIDFrom(r.Context())); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type bulkPreviewRequest struct {
	Filter people.PersonSearchFilter `json:"filter"`
	Action people.BulkAction         `json:"action"`
}

func (h *Handler) previewBulk(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input bulkPreviewRequest
	if err := platform.DecodeJSON(w, r, &input, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	preview, err := h.services.Bulk.Preview(r.Context(), principal, input.Filter, input.Action, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, preview)
}
func (h *Handler) startBulk(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	job, err := h.services.Bulk.Start(r.Context(), principal, platform.ID(chi.URLParam(r, "previewId")), platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}
func (h *Handler) runBulk(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	chunk, _ := strconv.Atoi(defaultString(r.URL.Query().Get("chunkSize"), "100"))
	job, err := h.services.Bulk.RunChunk(r.Context(), principal, platform.ID(chi.URLParam(r, "jobId")), chunk)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}
func (h *Handler) undoBulk(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	chunk, _ := strconv.Atoi(defaultString(r.URL.Query().Get("chunkSize"), "100"))
	job, err := h.services.Bulk.UndoChunk(r.Context(), principal, platform.ID(chi.URLParam(r, "jobId")), chunk)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (h *Handler) stageImport(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 21<<20)
	if err := r.ParseMultipartForm(21 << 20); err != nil {
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "file", Code: "body_too_large", Message: "Choose a CSV file up to 20 MB."}))
		return
	}
	branchID := platform.ID(r.FormValue("branchId"))
	if !h.allowed(principal, "create", "import-run", branchID, platform.FieldPersonal) {
		platform.WriteError(w, r, &platform.DomainError{Code: "forbidden", Message: "You cannot import people into this branch."})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "file", Code: "required", Message: "Choose a CSV file."}))
		return
	}
	defer file.Close()
	run, err := h.services.Imports.StageCSV(r.Context(), chmsimports.StageInput{OrganizationID: h.organizationID, BranchID: branchID, EntityType: defaultString(r.FormValue("entityType"), "people"), SourceType: defaultString(r.FormValue("sourceType"), "csv"), SourceName: defaultString(r.FormValue("sourceName"), header.Filename), Reader: file}, principal.Actor, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("ETag", strconv.Quote(strconv.FormatInt(run.Version, 10)))
	writeJSON(w, http.StatusCreated, run)
}

type mappingRequest struct {
	ExpectedVersion int64             `json:"expectedVersion"`
	Version         int               `json:"version"`
	Fields          map[string]string `json:"fields"`
}

func (h *Handler) mapImport(w http.ResponseWriter, r *http.Request) {
	principal, run, ok := h.importRun(w, r, "update")
	if !ok {
		return
	}
	var input mappingRequest
	if err := platform.DecodeJSON(w, r, &input, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if input.ExpectedVersion == 0 {
		input.ExpectedVersion = parseIfMatch(r.Header.Get("If-Match"))
	}
	updated, err := h.services.Imports.ConfigureMapping(r.Context(), h.organizationID, run.ID, input.ExpectedVersion, chmsimports.MappingInput{Version: input.Version, Fields: input.Fields}, peopleImportSchema(run.BranchID), principal.Actor, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("ETag", strconv.Quote(strconv.FormatInt(updated.Version, 10)))
	writeJSON(w, http.StatusOK, updated)
}
func (h *Handler) validateImport(w http.ResponseWriter, r *http.Request) {
	principal, run, ok := h.importRun(w, r, "update")
	if !ok {
		return
	}
	var body struct {
		ExpectedVersion int64 `json:"expectedVersion"`
	}
	if err := platform.DecodeJSON(w, r, &body, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if body.ExpectedVersion > 0 && body.ExpectedVersion != run.Version {
		platform.WriteError(w, r, platform.VersionConflict(run.Version))
		return
	}
	updated, err := h.services.Imports.ValidateDryRun(r.Context(), h.organizationID, run.ID, peopleImportSchema(run.BranchID), principal.Actor, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}
func (h *Handler) commitImport(w http.ResponseWriter, r *http.Request) {
	principal, run, ok := h.importRun(w, r, "update")
	if !ok {
		return
	}
	var body struct {
		ExpectedVersion int64 `json:"expectedVersion"`
	}
	if err := platform.DecodeJSON(w, r, &body, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if body.ExpectedVersion > 0 && body.ExpectedVersion != run.Version {
		platform.WriteError(w, r, platform.VersionConflict(run.Version))
		return
	}
	updated, err := h.services.Imports.CommitValidated(r.Context(), h.organizationID, run.ID, peopleImportCommitter{repository: h.services.People.Repository, platform: h.services.People.Platform}, principal.Actor, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}
func (h *Handler) rollbackImport(w http.ResponseWriter, r *http.Request) {
	principal, run, ok := h.importRun(w, r, "archive")
	if !ok {
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if err := platform.DecodeJSON(w, r, &body, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	updated, err := h.services.Imports.RollbackCommitted(r.Context(), h.organizationID, run.ID, peopleImportRollbacker{repository: h.services.People.Repository}, body.Reason, principal.Actor, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}
func (h *Handler) importRun(w http.ResponseWriter, r *http.Request, action string) (platform.Principal, *chmsimports.ImportRun, bool) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return principal, nil, false
	}
	run, err := h.services.Imports.Repository.FindRun(r.Context(), h.organizationID, platform.ID(chi.URLParam(r, "runId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return principal, nil, false
	}
	if run == nil || !h.allowed(principal, action, "import-run", run.BranchID, platform.FieldPersonal) {
		platform.WriteError(w, r, &platform.DomainError{Code: "not_found", Message: "Import run not found."})
		return principal, nil, false
	}
	return principal, run, true
}

type transitionRequest struct {
	ExpectedVersion int64       `json:"expectedVersion"`
	ToStage         string      `json:"toStage"`
	EffectiveAt     time.Time   `json:"effectiveAt"`
	ReasonCode      string      `json:"reasonCode"`
	NoteReference   platform.ID `json:"noteReference"`
}

func (h *Handler) transitionMembership(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	personID := platform.ID(chi.URLParam(r, "personId"))
	person, err := h.services.Repository.FindByID(r.Context(), h.organizationID, personID)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if person == nil || !h.allowed(principal, "update", "person", person.HomeBranchID, platform.FieldPersonal) {
		platform.WriteError(w, r, &platform.DomainError{Code: "not_found", Message: "Person not found."})
		return
	}
	var input transitionRequest
	if err := platform.DecodeJSON(w, r, &input, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if input.ExpectedVersion == 0 {
		input.ExpectedVersion = parseIfMatch(r.Header.Get("If-Match"))
	}
	event, err := h.services.People.TransitionMembership(r.Context(), h.organizationID, personID, input.ExpectedVersion, people.MembershipTransitionInput{ToStage: input.ToStage, EffectiveAt: input.EffectiveAt, ReasonCode: input.ReasonCode, NoteReference: input.NoteReference}, principal.Actor, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, event)
}

type reverseRequest struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	ReasonCode      string `json:"reasonCode"`
}

func (h *Handler) reverseMembership(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	personID := platform.ID(chi.URLParam(r, "personId"))
	person, err := h.services.Repository.FindByID(r.Context(), h.organizationID, personID)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if person == nil || !h.allowed(principal, "update", "person", person.HomeBranchID, platform.FieldPersonal) {
		platform.WriteError(w, r, &platform.DomainError{Code: "not_found", Message: "Person not found."})
		return
	}
	var input reverseRequest
	if err := platform.DecodeJSON(w, r, &input, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if input.ExpectedVersion == 0 {
		input.ExpectedVersion = parseIfMatch(r.Header.Get("If-Match"))
	}
	event, err := h.services.People.ReverseMembershipTransition(r.Context(), h.organizationID, personID, platform.ID(chi.URLParam(r, "eventId")), input.ExpectedVersion, input.ReasonCode, principal.Actor, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, event)
}

func (h *Handler) createServicePlan(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input community.ServicePlanInput
	if err := platform.DecodeJSON(w, r, &input, 256<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.services.Community.CreateServicePlan(r.Context(), principal, input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("ETag", strconv.Quote(strconv.FormatInt(value.Version, 10)))
	writeJSON(w, http.StatusCreated, value)
}

func (h *Handler) getServicePlan(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	value, err := h.services.Community.GetServicePlan(r.Context(), principal, platform.ID(chi.URLParam(r, "planId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("ETag", strconv.Quote(strconv.FormatInt(value.Version, 10)))
	writeJSON(w, http.StatusOK, value)
}

func (h *Handler) listServicePlans(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	from, fromErr := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
	to, toErr := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
	if fromErr != nil || toErr != nil {
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "range", Code: "invalid_range", Message: "from and to must be RFC3339 timestamps."}))
		return
	}
	values, err := h.services.Community.ListServicePlans(r.Context(), principal, platform.ID(r.URL.Query().Get("branchId")), from, to)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, values)
}

func (h *Handler) transitionServicePlan(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input struct {
		ExpectedVersion int64  `json:"expectedVersion"`
		Status          string `json:"status"`
		Reason          string `json:"reason"`
	}
	if err := platform.DecodeJSON(w, r, &input, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if input.ExpectedVersion == 0 {
		input.ExpectedVersion = parseIfMatch(r.Header.Get("If-Match"))
	}
	value, err := h.services.Community.TransitionServicePlan(r.Context(), principal, platform.ID(chi.URLParam(r, "planId")), input.ExpectedVersion, community.ServicePlanTransition{Status: input.Status, Reason: input.Reason}, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("ETag", strconv.Quote(strconv.FormatInt(value.Version, 10)))
	writeJSON(w, http.StatusOK, value)
}

func (h *Handler) createVolunteerRotation(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input community.VolunteerRotationInput
	if err := platform.DecodeJSON(w, r, &input, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	input.TeamID = platform.ID(chi.URLParam(r, "teamId"))
	value, err := h.services.Community.CreateVolunteerRotation(r.Context(), principal, input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (h *Handler) listVolunteerRotations(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	values, err := h.services.Community.ListVolunteerRotations(r.Context(), principal, platform.ID(chi.URLParam(r, "teamId")), r.URL.Query().Get("includeInactive") == "true")
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, values)
}

func (h *Handler) previewVolunteerAssignment(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input community.AssignmentInput
	if err := platform.DecodeJSON(w, r, &input, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.services.Community.PreviewAssignment(r.Context(), principal, platform.ID(chi.URLParam(r, "planId")), input)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (h *Handler) createVolunteerAssignment(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input community.AssignmentInput
	if err := platform.DecodeJSON(w, r, &input, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.services.Community.CreateAssignment(r.Context(), principal, platform.ID(chi.URLParam(r, "planId")), input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (h *Handler) listVolunteerAssignments(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	values, err := h.services.Community.ListAssignments(r.Context(), principal, platform.ID(chi.URLParam(r, "planId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, values)
}

func (h *Handler) respondVolunteerAssignment(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input community.AssignmentResponseInput
	if err := platform.DecodeJSON(w, r, &input, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if input.ExpectedVersion == 0 {
		input.ExpectedVersion = parseIfMatch(r.Header.Get("If-Match"))
	}
	value, err := h.services.Community.RespondAssignment(r.Context(), principal, platform.ID(chi.URLParam(r, "assignmentId")), input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (h *Handler) substituteVolunteerAssignment(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input community.SubstituteInput
	if err := platform.DecodeJSON(w, r, &input, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if input.ExpectedVersion == 0 {
		input.ExpectedVersion = parseIfMatch(r.Header.Get("If-Match"))
	}
	value, err := h.services.Community.SubstituteAssignment(r.Context(), principal, platform.ID(chi.URLParam(r, "assignmentId")), input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (h *Handler) recordVolunteerReminder(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input community.ReminderInput
	if err := platform.DecodeJSON(w, r, &input, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if input.ExpectedVersion == 0 {
		input.ExpectedVersion = parseIfMatch(r.Header.Get("If-Match"))
	}
	value, err := h.services.Community.RecordAssignmentReminder(r.Context(), principal, platform.ID(chi.URLParam(r, "assignmentId")), input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (h *Handler) markVolunteerNoShow(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input community.NoShowInput
	if err := platform.DecodeJSON(w, r, &input, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if input.ExpectedVersion == 0 {
		input.ExpectedVersion = parseIfMatch(r.Header.Get("If-Match"))
	}
	value, err := h.services.Community.MarkAssignmentNoShow(r.Context(), principal, platform.ID(chi.URLParam(r, "assignmentId")), input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (h *Handler) listVolunteerAssignmentEvents(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	values, err := h.services.Community.ListAssignmentEvents(r.Context(), principal, platform.ID(chi.URLParam(r, "assignmentId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, values)
}

func (h *Handler) allowed(principal platform.Principal, action, resource string, branch platform.ID, fields ...platform.FieldClass) bool {
	return h.services.Authorizer.Authorize(principal, platform.AccessRequest{Action: action, ResourceType: resource, OrganizationID: h.organizationID, BranchID: branch, FieldClasses: fields, Now: time.Now().UTC()}).Allowed
}

func (h *Handler) principal(r *http.Request) (platform.Principal, bool) {
	if h.resolvePrincipal != nil {
		return h.resolvePrincipal(r)
	}
	claims := middleware.ClaimsFrom(r)
	if claims == nil || strings.TrimSpace(claims.UserID) == "" {
		return platform.Principal{}, false
	}
	if strings.HasPrefix(claims.Role, "finance-") {
		actorID := platform.ID(claims.UserID)
		actionsByResource := map[string][]string{}
		switch claims.Role {
		case "finance-counter":
			actionsByResource["finance-batch"] = []string{"read", "operate"}
		case "finance-admin":
			actionsByResource["finance-config"] = []string{"read", "create", "update"}
			actionsByResource["finance-batch"] = []string{"read", "create", "update", "operate"}
			actionsByResource["finance-ledger"] = []string{"read", "create", "export"}
		case "finance-approver":
			actionsByResource["finance-config"] = []string{"read"}
			actionsByResource["finance-batch"] = []string{"read", "approve"}
			actionsByResource["finance-ledger"] = []string{"read", "approve", "export"}
		case "finance-auditor":
			actionsByResource["finance-config"] = []string{"read"}
			actionsByResource["finance-batch"] = []string{"read"}
			actionsByResource["finance-ledger"] = []string{"read"}
		}
		grants := []platform.Grant{}
		for resource, actions := range actionsByResource {
			for _, action := range actions {
				grants = append(grants, platform.Grant{Action: action, Resource: resource, BranchIDs: claimIDs(claims.BranchIDs), FieldClasses: []platform.FieldClass{platform.FieldFinancial}})
			}
		}
		principal := platform.Principal{Actor: platform.Actor{Type: platform.ActorStaff, ID: actorID}, OrganizationID: h.organizationID, Roles: []string{claims.Role}, Grants: grants, AssignedResourceIDs: claimIDs(claims.AssignedResourceIDs)}
		if claims.MFAAt > 0 {
			confirmed := time.Unix(claims.MFAAt, 0).UTC()
			principal.MFAConfirmedAt = &confirmed
		}
		return principal, actorID.Valid()
	}
	if claims.Role == "auditor" || claims.Role == "data-protection-supervisor" {
		principal, recognized := StaffPrincipalForRoleScoped(claims.Role, platform.ID(claims.UserID), h.organizationID, claims.BranchIDs, claims.MinistryIDs, claims.AssignedResourceIDs)
		if !recognized {
			return platform.Principal{}, false
		}
		if claims.MFAAt > 0 {
			confirmed := time.Unix(claims.MFAAt, 0).UTC()
			principal.MFAConfirmedAt = &confirmed
		}
		return principal, true
	}
	if roleGrants, recognized := operationalRoleGrants(claims.Role); recognized {
		actorID := platform.ID(claims.UserID)
		roleGrants = applyClaimScopes(roleGrants, claims.BranchIDs, claims.MinistryIDs)
		principal := platform.Principal{Actor: platform.Actor{Type: platform.ActorStaff, ID: actorID}, OrganizationID: h.organizationID, Roles: []string{claims.Role}, Grants: roleGrants, AssignedResourceIDs: claimIDs(claims.AssignedResourceIDs)}
		if claims.MFAAt > 0 {
			confirmed := time.Unix(claims.MFAAt, 0).UTC()
			principal.MFAConfirmedAt = &confirmed
		}
		return principal, actorID.Valid()
	}
	fields := []platform.FieldClass{platform.FieldOperational, platform.FieldPersonal}
	actions := []string{"read", "search"}
	if claims.Role == "editor" || claims.Role == "super-admin" {
		actions = append(actions, "create", "update", "archive", "restore", "bulk-update", "cancel", "record", "operate")
	}
	if claims.Role == "super-admin" {
		fields = append(fields, platform.FieldSensitiveMinistry, platform.FieldChildSafeguarding, platform.FieldFinancial)
		actions = append(actions, "merge", "approve", "pickup", "export")
	}
	grants := make([]platform.Grant, 0, len(actions))
	for _, action := range actions {
		grants = append(grants, platform.Grant{Action: action, Resource: "*", BranchIDs: claimIDs(claims.BranchIDs), FieldClasses: fields})
	}
	actorType, actorID := platform.ActorStaff, platform.ID(claims.UserID)
	if claims.Role == "member" {
		actorType, actorID = platform.ActorMember, platform.ID(claims.PersonID)
		grants = nil
	}
	principal := platform.Principal{Actor: platform.Actor{Type: actorType, ID: actorID}, OrganizationID: h.organizationID, Roles: []string{claims.Role}, Grants: grants, AssignedResourceIDs: claimIDs(claims.AssignedResourceIDs)}
	if claims.MFAAt > 0 {
		confirmed := time.Unix(claims.MFAAt, 0).UTC()
		principal.MFAConfirmedAt = &confirmed
	}
	return principal, actorID.Valid()
}

func claimIDs(values []string) []platform.ID {
	ids := make([]platform.ID, 0, len(values))
	for _, value := range values {
		id := platform.ID(strings.TrimSpace(value))
		if id.Valid() {
			ids = append(ids, id)
		}
	}
	return ids
}

func applyClaimScopes(grants []platform.Grant, branches, ministries []string) []platform.Grant {
	branchIDs, ministryIDs := claimIDs(branches), claimIDs(ministries)
	ministryScoped := map[string]bool{"group": true, "volunteer": true, "volunteer-assignment": true, "care-case": true, "workflow": true, "communication-campaign": true}
	for i := range grants {
		grants[i].BranchIDs = append([]platform.ID(nil), branchIDs...)
		if ministryScoped[grants[i].Resource] {
			grants[i].MinistryIDs = append([]platform.ID(nil), ministryIDs...)
		}
	}
	return grants
}

func operationalRoleGrants(role string) ([]platform.Grant, bool) {
	fields := []platform.FieldClass{platform.FieldOperational, platform.FieldPersonal}
	read := []string{"read", "search"}
	mutate := []string{"create", "update", "archive", "restore", "record", "operate", "cancel"}
	resources, actions := []string{}, read
	switch role {
	case "pastor":
		fields = append(fields, platform.FieldSensitiveMinistry)
		resources, actions = []string{"person", "household", "relationship", "attendance", "group", "volunteer", "engagement", "care-case", "workflow"}, append(read, "create", "update", "operate", "record")
	case "branch-admin":
		resources, actions = []string{"*"}, append(read, mutate...)
	case "membership-admin":
		resources, actions = []string{"person", "household", "relationship", "people-segment", "import", "attendance"}, append(read, mutate...)
	case "group-admin":
		resources, actions = []string{"group", "volunteer", "communication-campaign"}, append(read, mutate...)
	case "volunteer-coordinator":
		resources, actions = []string{"group", "volunteer", "volunteer-assignment"}, append(read, mutate...)
	default:
		return nil, false
	}
	grants := make([]platform.Grant, 0, len(resources)*len(actions))
	for _, resource := range resources {
		for _, action := range actions {
			grants = append(grants, platform.Grant{Action: action, Resource: resource, BranchIDs: []platform.ID{"*"}, FieldClasses: fields})
		}
	}
	return grants, true
}

// StaffPrincipalForRole rebuilds current grants for background work. It uses
// the same role vocabulary as request authentication so scheduled reports are
// reauthorized instead of retaining permissions captured when created.
func StaffPrincipalForRole(role string, actorID, organizationID platform.ID) (platform.Principal, bool) {
	return StaffPrincipalForRoleScoped(role, actorID, organizationID, []string{"*"}, []string{"*"}, nil)
}

func StaffPrincipalForRoleScoped(role string, actorID, organizationID platform.ID, branches, ministries, assignments []string) (platform.Principal, bool) {
	if !actorID.Valid() || !organizationID.Valid() {
		return platform.Principal{}, false
	}
	if strings.HasPrefix(role, "finance-") {
		actionsByResource := map[string][]string{}
		switch role {
		case "finance-counter":
			actionsByResource["finance-batch"] = []string{"read", "operate"}
		case "finance-admin":
			actionsByResource["finance-config"] = []string{"read", "create", "update"}
			actionsByResource["finance-batch"] = []string{"read", "create", "update", "operate"}
			actionsByResource["finance-ledger"] = []string{"read", "create", "export"}
		case "finance-approver":
			actionsByResource["finance-config"] = []string{"read"}
			actionsByResource["finance-batch"] = []string{"read", "approve"}
			actionsByResource["finance-ledger"] = []string{"read", "approve", "export"}
		case "finance-auditor":
			actionsByResource["finance-config"] = []string{"read"}
			actionsByResource["finance-batch"] = []string{"read"}
			actionsByResource["finance-ledger"] = []string{"read"}
		default:
			return platform.Principal{}, false
		}
		grants := []platform.Grant{}
		for resource, actions := range actionsByResource {
			for _, action := range actions {
				grants = append(grants, platform.Grant{Action: action, Resource: resource, BranchIDs: claimIDs(branches), FieldClasses: []platform.FieldClass{platform.FieldFinancial}})
			}
		}
		return platform.Principal{Actor: platform.Actor{Type: platform.ActorStaff, ID: actorID}, OrganizationID: organizationID, Roles: []string{role}, Grants: grants, AssignedResourceIDs: claimIDs(assignments)}, true
	}
	if role == "auditor" || role == "data-protection-supervisor" {
		grantBranches := claimIDs(branches)
		if role == "data-protection-supervisor" {
			grantBranches = []platform.ID{"*"}
		}
		grants := []platform.Grant{
			{Action: "read", Resource: "audit-event", BranchIDs: grantBranches, FieldClasses: []platform.FieldClass{platform.FieldOperational}},
			{Action: "export", Resource: "audit-event", BranchIDs: grantBranches, FieldClasses: []platform.FieldClass{platform.FieldOperational}},
			{Action: "read", Resource: "data-request", BranchIDs: grantBranches, FieldClasses: []platform.FieldClass{platform.FieldOperational, platform.FieldPersonal}},
		}
		if role == "data-protection-supervisor" {
			grants = append(grants, platform.Grant{Action: "operate", Resource: "data-request", BranchIDs: grantBranches, FieldClasses: []platform.FieldClass{platform.FieldOperational, platform.FieldPersonal}})
		}
		return platform.Principal{Actor: platform.Actor{Type: platform.ActorStaff, ID: actorID}, OrganizationID: organizationID, Roles: []string{role}, Grants: grants}, true
	}
	if grants, ok := operationalRoleGrants(role); ok {
		grants = applyClaimScopes(grants, branches, ministries)
		return platform.Principal{Actor: platform.Actor{Type: platform.ActorStaff, ID: actorID}, OrganizationID: organizationID, Roles: []string{role}, Grants: grants, AssignedResourceIDs: claimIDs(assignments)}, true
	}
	if role != "viewer" && role != "editor" && role != "super-admin" {
		return platform.Principal{}, false
	}
	fields := []platform.FieldClass{platform.FieldOperational, platform.FieldPersonal}
	actions := []string{"read", "search"}
	if role == "editor" || role == "super-admin" {
		actions = append(actions, "create", "update", "archive", "restore", "bulk-update", "cancel", "record", "operate")
	}
	if role == "super-admin" {
		fields = append(fields, platform.FieldSensitiveMinistry, platform.FieldChildSafeguarding, platform.FieldFinancial)
		actions = append(actions, "merge", "approve", "pickup", "export")
	}
	grants := make([]platform.Grant, 0, len(actions))
	for _, action := range actions {
		grants = append(grants, platform.Grant{Action: action, Resource: "*", BranchIDs: claimIDs(branches), FieldClasses: fields})
	}
	return platform.Principal{Actor: platform.Actor{Type: platform.ActorStaff, ID: actorID}, OrganizationID: organizationID, Roles: []string{role}, Grants: grants, AssignedResourceIDs: claimIDs(assignments)}, true
}

func permissionsFor(principal platform.Principal) map[string]any {
	role := "viewer"
	if len(principal.Roles) > 0 {
		role = principal.Roles[0]
	}
	sections := []string{"identity", "household", "journey", "attendance", "groups", "serving"}
	actions := []string{}
	if role == "editor" || role == "super-admin" || role == "branch-admin" || role == "membership-admin" {
		actions = []string{"edit", "archive", "transition-membership"}
	}
	if role == "super-admin" {
		sections = append(sections, "care", "giving")
		actions = append(actions, "merge")
	}
	if role == "pastor" {
		sections = append(sections, "care")
		actions = append(actions, "edit-care")
	}
	return map[string]any{"sections": sections, "actions": actions}
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
func parseOptionalTime(value string, fallback time.Time) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	return time.Parse(time.RFC3339, value)
}
func parseIfMatch(value string) int64 {
	value = strings.Trim(strings.TrimSpace(value), "\"")
	parsed, _ := strconv.ParseInt(value, 10, 64)
	return parsed
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
