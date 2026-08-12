package community

import (
	"testing"

	"remi-api/internal/chms/platform"
)

func TestGroupInputNormalizesOperationalControls(t *testing.T) {
	input := GroupInput{HomeBranchID: "accra", Name: "  Young Adults  ", Type: "COMMUNITY", LeaderPersonIDs: []platform.ID{"leader-2", "leader-1", "leader-1"}, Capacity: 24, Privacy: "request", Discoverability: "members", Status: "active", MeetingPattern: &MeetingPattern{Frequency: "weekly", Weekday: "Friday", LocalStart: "18:30", DurationMinutes: 90, Timezone: "Africa/Accra", Location: " Fellowship hall "}}
	if err := input.NormalizeAndValidate(); err != nil {
		t.Fatal(err)
	}
	if input.Name != "Young Adults" || len(input.LeaderPersonIDs) != 2 || input.MeetingPattern.Location != "Fellowship hall" {
		t.Fatalf("normalization failed: %+v", input)
	}
}
func TestGroupInputRejectsUnsafeVisibilityAndBadLifecycle(t *testing.T) {
	input := GroupInput{HomeBranchID: "accra", Name: "Private Care", Type: "support", Privacy: "invite-only", Discoverability: "public", Status: "active"}
	if err := input.NormalizeAndValidate(); err == nil {
		t.Fatal("public invite-only group accepted")
	}
	if err := validateTransition("closed", "active", ""); err == nil {
		t.Fatal("closed group reopened")
	}
	if err := validateTransition("active", "paused", ""); err == nil {
		t.Fatal("pause without reason accepted")
	}
}
