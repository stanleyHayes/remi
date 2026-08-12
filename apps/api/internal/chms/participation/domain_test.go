package participation

import (
	"remi-api/internal/chms/platform"
	"testing"
	"time"
)

func TestDefinitionValidatesIANATimezoneAndRecurrence(t *testing.T) {
	input := DefinitionInput{HomeBranchID: "accra", Name: " Sunday Celebration ", Timezone: "Africa/Accra", DefaultDurationMinutes: 120, DefaultCapacity: 500, DefaultRoomIDs: []platform.ID{"hall", "hall"}, Recurrence: &RecurrencePattern{Frequency: "Weekly", DaysOfWeek: []string{"Sunday", "sunday"}, LocalStart: "09:00", StartsOn: "2026-08-16"}}
	if err := input.NormalizeAndValidate(); err != nil {
		t.Fatal(err)
	}
	if input.Name != "Sunday Celebration" || len(input.DefaultRoomIDs) != 1 || len(input.Recurrence.DaysOfWeek) != 1 {
		t.Fatalf("not normalized: %+v", input)
	}
}
func TestDefinitionRejectsInvalidTimezoneAndLocalTime(t *testing.T) {
	tests := []DefinitionInput{{HomeBranchID: "b", Name: "Service", Timezone: "Accra/Ghana", DefaultDurationMinutes: 60}, {HomeBranchID: "b", Name: "Service", Timezone: "Africa/Accra", DefaultDurationMinutes: 60, Recurrence: &RecurrencePattern{Frequency: "weekly", DaysOfWeek: []string{"sunday"}, LocalStart: "9am", StartsOn: "2026-08-16"}}}
	for _, input := range tests {
		if err := input.NormalizeAndValidate(); err == nil {
			t.Fatalf("accepted invalid input: %+v", input)
		}
	}
}
func TestOccurrenceUsesUTCAndStableImmutableKey(t *testing.T) {
	start := time.Date(2026, 8, 16, 9, 0, 0, 0, time.FixedZone("GMT", 0))
	input := OccurrenceInput{HomeBranchID: "accra", ServiceDefinitionID: "service-1", Name: "Sunday Celebration", StartsAt: start, EndsAt: start.Add(2 * time.Hour), Timezone: "Africa/Accra", Capacity: 500}
	if err := input.NormalizeAndValidate(); err != nil {
		t.Fatal(err)
	}
	one := OccurrenceKey("org", "service-1", input.StartsAt)
	two := OccurrenceKey("org", "service-1", input.StartsAt.In(time.FixedZone("other", 2*60*60)))
	if one != two || one == OccurrenceKey("org", "service-1", input.StartsAt.Add(time.Minute)) {
		t.Fatalf("bad occurrence keys: %s %s", one, two)
	}
}
func TestOccurrenceRejectsInvalidWindow(t *testing.T) {
	now := time.Now()
	input := OccurrenceInput{HomeBranchID: "b", Name: "Too long", StartsAt: now, EndsAt: now.Add(25 * time.Hour), Timezone: "Africa/Accra"}
	if err := input.NormalizeAndValidate(); err == nil {
		t.Fatal("accepted occurrence longer than 24 hours")
	}
}
func TestExpandRecurrenceUsesLocalCalendarAndDoesNotRewriteHistory(t *testing.T) {
	definition := ServiceDefinition{ResourceEnvelope: platform.ResourceEnvelope{ID: "service-1", OrganizationID: "org", BranchID: "accra"}, Name: "Sunday Celebration", HomeBranchID: "accra", Timezone: "Africa/Accra", DefaultDurationMinutes: 120, DefaultRoomIDs: []platform.ID{"hall"}, DefaultCapacity: 500, Recurrence: &RecurrencePattern{Frequency: "weekly", DaysOfWeek: []string{"sunday"}, LocalStart: "09:00", Interval: 1, StartsOn: "2026-08-16"}}
	items, err := ExpandRecurrence(definition, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("generated %d occurrences, want 3", len(items))
	}
	if items[0].StartsAt.Format(time.RFC3339) != "2026-08-16T09:00:00Z" || items[0].EndsAt.Sub(items[0].StartsAt) != 2*time.Hour {
		t.Fatalf("bad generated occurrence: %+v", items[0])
	}
	if OccurrenceKey("org", definition.ID, items[0].StartsAt) != OccurrenceKey("org", definition.ID, items[0].StartsAt) {
		t.Fatal("occurrence key is not stable")
	}
}

func TestAttendanceNormalizesSourceConfidenceAndTimes(t *testing.T) {
	checkIn := time.Date(2026, 8, 16, 9, 5, 0, 0, time.FixedZone("GMT+2", 2*60*60))
	input := AttendanceInput{Status: " Present ", Source: " ROSTER ", CheckedInAt: &checkIn, Guest: true}
	if err := input.NormalizeAndValidate(); err != nil {
		t.Fatal(err)
	}
	if input.Status != AttendancePresent || input.Source != "roster" || input.Confidence != 100 || input.CheckedInAt.Location() != time.UTC {
		t.Fatalf("attendance not normalized: %+v", input)
	}
}

func TestAttendanceRejectsInvalidStatusSourceConfidenceAndCheckout(t *testing.T) {
	now := time.Now().UTC()
	earlier := now.Add(-time.Hour)
	tests := []AttendanceInput{
		{Status: "maybe", Source: "operator", Confidence: 100},
		{Status: "present", Source: "facial-recognition", Confidence: 100},
		{Status: "present", Source: "operator", Confidence: 101},
		{Status: "absent", Source: "roster", Confidence: 100, CheckedInAt: &now},
		{Status: "present", Source: "operator", Confidence: 100, CheckedInAt: &now, CheckedOutAt: &earlier},
	}
	for _, input := range tests {
		if err := input.NormalizeAndValidate(); err == nil {
			t.Fatalf("accepted invalid attendance: %+v", input)
		}
	}
}

func TestHeadcountIsAggregateOnly(t *testing.T) {
	input := HeadcountInput{Category: " Main hall ", Count: 380, Source: "operator", ObservedAt: time.Now()}
	if err := input.NormalizeAndValidate(); err != nil {
		t.Fatal(err)
	}
	if input.Category != "main hall" || input.Confidence != 100 {
		t.Fatalf("headcount not normalized: %+v", input)
	}
}
