// Package reporting owns the canonical definitions used by dashboards and
// exports. A metric is not publishable until its source, scope, grain,
// timezone, exclusions, freshness and reconciliation rule are explicit.
package reporting

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"remi-api/internal/chms/platform"
)

type Definition struct {
	ID                 string              `json:"id"`
	Version            string              `json:"version"`
	Name               string              `json:"name"`
	Description        string              `json:"description"`
	Owner              string              `json:"owner"`
	Domain             string              `json:"domain"`
	Unit               string              `json:"unit"`
	Formula            string              `json:"formula"`
	Sources            []string            `json:"sources"`
	Grain              string              `json:"grain"`
	Scope              string              `json:"scope"`
	Timezone           string              `json:"timezone"`
	Exclusions         []string            `json:"exclusions"`
	FreshnessMinutes   int                 `json:"freshnessMinutes"`
	ReconciliationRule string              `json:"reconciliationRule"`
	PrivacyRule        string              `json:"privacyRule"`
	RequiredResource   string              `json:"-"`
	FieldClass         platform.FieldClass `json:"-"`
}

type Catalog struct {
	Version     string       `json:"version"`
	GeneratedAt time.Time    `json:"generatedAt"`
	Items       []Definition `json:"items"`
}

var definitions = []Definition{
	{ID: "people.active", Version: "1.0.0", Name: "Active people", Description: "Named, non-archived people currently in an active membership stage.", Owner: "Membership lead", Domain: "people", Unit: "people", Formula: "count(distinct person.id)", Sources: []string{"chms_people"}, Grain: "person", Scope: "organization and optional home branch", Timezone: "branch timezone", Exclusions: []string{"archived people", "visitor records outside an active membership stage", "anonymous attendance"}, FreshnessMinutes: 15, ReconciliationRule: "Equals the branch-scoped people register under the same as-of timestamp and lifecycle filter.", PrivacyRule: "Aggregate only; no person identifiers in dashboard output.", RequiredResource: "person", FieldClass: platform.FieldPersonal},
	{ID: "households.active", Version: "1.0.0", Name: "Active households", Description: "Households with at least one current membership relation.", Owner: "Membership lead", Domain: "people", Unit: "households", Formula: "count(distinct household.id with current membership)", Sources: []string{"chms_households", "chms_household_memberships"}, Grain: "household", Scope: "organization and optional branch", Timezone: "branch timezone", Exclusions: []string{"ended memberships", "archived households", "empty imported shells"}, FreshnessMinutes: 30, ReconciliationRule: "Every counted household must resolve to one or more current household membership rows.", PrivacyRule: "Suppress household composition from aggregate responses.", RequiredResource: "household", FieldClass: platform.FieldPersonal},
	{ID: "attendance.confirmed_people", Version: "1.0.0", Name: "Confirmed attendance", Description: "Unique named people with effective present attendance in the selected occurrence window.", Owner: "Service operations lead", Domain: "attendance", Unit: "people", Formula: "count(distinct person.id where effective status = present)", Sources: []string{"chms_occurrences", "chms_attendance_records", "chms_attendance_events"}, Grain: "person per reporting window", Scope: "branch, service definition and occurrence window", Timezone: "occurrence timezone", Exclusions: []string{"cancelled occurrences", "anonymous headcount", "absent or excused records", "superseded corrections"}, FreshnessMinutes: 15, ReconciliationRule: "Unique people never exceeds effective present rows; named plus anonymous attendance reconciles separately to approved headcount.", PrivacyRule: "Small cohorts below five are suppressed outside operational attendance views.", RequiredResource: "attendance", FieldClass: platform.FieldOperational},
	{ID: "attendance.headcount", Version: "1.0.0", Name: "Approved headcount", Description: "Latest approved total headcount for completed occurrences.", Owner: "Service operations lead", Domain: "attendance", Unit: "attendances", Formula: "sum(latest effective approved headcount.total)", Sources: []string{"chms_occurrences", "chms_headcounts"}, Grain: "occurrence", Scope: "branch and occurrence window", Timezone: "occurrence timezone", Exclusions: []string{"cancelled occurrences", "superseded headcounts", "draft or rejected counts"}, FreshnessMinutes: 15, ReconciliationRule: "For each occurrence, reported headcount equals the latest effective approved record and is compared with named attendance without forcing equality.", PrivacyRule: "Aggregate operational value only.", RequiredResource: "attendance", FieldClass: platform.FieldOperational},
	{ID: "groups.active_connections", Version: "1.0.0", Name: "Active group connections", Description: "People with a current active membership in a non-archived group.", Owner: "Groups lead", Domain: "groups", Unit: "connections", Formula: "count(distinct group.id + person.id current active membership)", Sources: []string{"chms_groups", "chms_group_memberships"}, Grain: "person-group connection", Scope: "organization, branch and ministry", Timezone: "branch timezone", Exclusions: []string{"applications", "invitations", "waitlist", "ended memberships", "archived groups"}, FreshnessMinutes: 30, ReconciliationRule: "Equals the sum of current active roster rows across included groups after deduplicating person-group pairs.", PrivacyRule: "Cohorts below five are suppressed in comparative reports.", RequiredResource: "group", FieldClass: platform.FieldOperational},
	{ID: "serving.fill_rate", Version: "1.0.0", Name: "Serving fill rate", Description: "Share of required service-plan positions filled by accepted or completed assignments.", Owner: "Volunteer operations lead", Domain: "serving", Unit: "percent", Formula: "100 * filled required slots / required slots", Sources: []string{"chms_service_plans", "chms_volunteer_positions", "chms_volunteer_assignments"}, Grain: "service plan", Scope: "branch, ministry and team", Timezone: "service-plan timezone", Exclusions: []string{"cancelled plans", "optional positions", "declined or cancelled assignments"}, FreshnessMinutes: 15, ReconciliationRule: "Filled slots plus open slots equals required slots for every included plan.", PrivacyRule: "No screening, safeguarding or volunteer health fields.", RequiredResource: "volunteer-assignment", FieldClass: platform.FieldOperational},
	{ID: "care.open_cases", Version: "1.0.0", Name: "Open care cases", Description: "Care cases in an actionable non-terminal state.", Owner: "Pastoral care lead", Domain: "care", Unit: "cases", Formula: "count(case.id where state not in closed,cancelled)", Sources: []string{"chms_care_cases"}, Grain: "care case", Scope: "authorized branch and assignment scope", Timezone: "branch timezone", Exclusions: []string{"closed cases", "cancelled cases", "member prayer requests not promoted to a case"}, FreshnessMinutes: 5, ReconciliationRule: "Equals the authorized care queue under the same state and assignment filters.", PrivacyRule: "Counts only; categories, notes, subjects and assignees are excluded from executive output.", RequiredResource: "care-case", FieldClass: platform.FieldSensitiveMinistry},
	{ID: "retention.open_observations", Version: "retention-v1", Name: "Open pastoral observations", Description: "Current evidence-backed engagement observations awaiting human review.", Owner: "Pastoral follow-up lead", Domain: "retention", Unit: "observations", Formula: "count(signal.id where state is open and not expired)", Sources: []string{"chms_engagement_signals", "chms_engagement_rules"}, Grain: "evidence observation", Scope: "branch and published policy", Timezone: "published policy timezone", Exclusions: []string{"expired observations", "resolved, suppressed or snoozed observations", "finance data"}, FreshnessMinutes: 30, ReconciliationRule: "Equals the review queue for the same policy version, evidence watermark and as-of time.", PrivacyRule: "Never a member score; no ranking, faith inference or finance source.", RequiredResource: "engagement", FieldClass: platform.FieldSensitiveMinistry},
	{ID: "giving.net_posted", Version: "finance-1.0.0", Name: "Net posted giving", Description: "Net immutable posted contribution value after approved corrections in the selected range.", Owner: "Finance lead", Domain: "finance", Unit: "minor currency units", Formula: "sum(posted contribution total + immutable approved adjustments)", Sources: []string{"chms_finance_contributions", "chms_finance_adjustments"}, Grain: "posted contribution", Scope: "organization, branch, fund and fiscal period", Timezone: "campus timezone", Exclusions: []string{"draft batches", "unverified payment intents", "pledges", "reversed contributions beyond their effective net value"}, FreshnessMinutes: 5, ReconciliationRule: "Equals finance report netContributionMinor and reconciles by fund to the immutable contribution ledger.", PrivacyRule: "Financial permission required; aggregate output contains no donor identity.", RequiredResource: "finance-ledger", FieldClass: platform.FieldFinancial},
	{ID: "giving.unexplained_variance", Version: "finance-1.0.0", Name: "Unexplained settlement variance", Description: "Absolute unresolved variance remaining across imported settlements.", Owner: "Finance lead", Domain: "finance", Unit: "minor currency units", Formula: "sum(abs(settlement.unexplainedVarianceMinor))", Sources: []string{"chms_finance_settlements", "chms_finance_reconciliation_items"}, Grain: "settlement", Scope: "organization, branch and settlement window", Timezone: "campus timezone", Exclusions: []string{"resolved variance", "unposted provider previews"}, FreshnessMinutes: 5, ReconciliationRule: "Each settlement gross equals matched gross plus explained and unexplained variance under the same import version.", PrivacyRule: "Financial permission required; provider secrets and payer identity excluded.", RequiredResource: "finance-ledger", FieldClass: platform.FieldFinancial},
	{ID: "communications.delivery_rate", Version: "1.0.0", Name: "Communication delivery rate", Description: "Share of attempted external deliveries accepted or delivered by the provider.", Owner: "Communications lead", Domain: "communications", Unit: "percent", Formula: "100 * delivered-or-accepted / attempted", Sources: []string{"chms_communication_deliveries"}, Grain: "delivery attempt", Scope: "organization, branch, campaign, purpose and channel", Timezone: "campaign timezone", Exclusions: []string{"consent-suppressed recipients", "quiet-hour deferrals", "daily-cap deferrals", "test deliveries"}, FreshnessMinutes: 15, ReconciliationRule: "Attempted equals accepted, delivered, failed and pending provider outcomes without double-counting retries.", PrivacyRule: "No destination, message body, provider payload or recipient identity.", RequiredResource: "communication-campaign", FieldClass: platform.FieldOperational},
	{ID: "data_quality.open_exceptions", Version: "1.0.0", Name: "Open data-quality exceptions", Description: "Unresolved duplicate, invalid-contact, orphan-relation and unresolved-donor exceptions.", Owner: "Data steward", Domain: "data-quality", Unit: "exceptions", Formula: "count(exception.id where state = open)", Sources: []string{"chms_import_issues", "chms_duplicate_candidates", "chms_finance_reconciliation_items"}, Grain: "exception", Scope: "organization, branch and source run", Timezone: "UTC", Exclusions: []string{"resolved exceptions", "acknowledged false positives", "rolled-back import runs"}, FreshnessMinutes: 30, ReconciliationRule: "Equals the open exception queues by type and source run; the total is the exact sum of displayed categories.", PrivacyRule: "Counts and safe categories only; raw source rows and personal values require their owning permission.", RequiredResource: "import", FieldClass: platform.FieldOperational},
}

func Definitions() []Definition {
	items := append([]Definition(nil), definitions...)
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items
}

func Validate(items []Definition) error {
	if len(items) == 0 {
		return fmt.Errorf("metric dictionary is empty")
	}
	seen := map[string]bool{}
	for _, item := range items {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Version) == "" || strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.Description) == "" || strings.TrimSpace(item.Owner) == "" || strings.TrimSpace(item.Domain) == "" || strings.TrimSpace(item.Unit) == "" || strings.TrimSpace(item.Formula) == "" || len(item.Sources) == 0 || strings.TrimSpace(item.Grain) == "" || strings.TrimSpace(item.Scope) == "" || strings.TrimSpace(item.Timezone) == "" || len(item.Exclusions) == 0 || item.FreshnessMinutes < 1 || strings.TrimSpace(item.ReconciliationRule) == "" || strings.TrimSpace(item.PrivacyRule) == "" || strings.TrimSpace(item.RequiredResource) == "" || item.FieldClass == "" {
			return fmt.Errorf("metric %q is incomplete", item.ID)
		}
		if seen[item.ID] {
			return fmt.Errorf("duplicate metric %q", item.ID)
		}
		seen[item.ID] = true
	}
	return nil
}

func init() {
	if err := Validate(definitions); err != nil {
		panic(err)
	}
}
