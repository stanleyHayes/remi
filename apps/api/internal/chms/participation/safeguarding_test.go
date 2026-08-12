package participation

import (
	"crypto/sha256"
	"strings"
	"testing"
	"time"
)

func TestPickupCodesAreRandomHashedAndConstantTimeVerifiable(t *testing.T) {
	secret := sha256.Sum256([]byte("safeguarding-unit-test-secret"))
	manager, err := NewPickupCodeManager(secret[:])
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for range 200 {
		code, hash, err := manager.Generate()
		if err != nil {
			t.Fatal(err)
		}
		if len(code) != 6 || len(hash) != 64 || hash == code || seen[code] {
			t.Fatalf("unsafe pickup code code=%q hash=%q", code, hash)
		}
		for _, character := range code {
			if !strings.ContainsRune(pickupAlphabet, character) {
				t.Fatalf("code contains ambiguous character %q", character)
			}
		}
		if !manager.Verify(strings.ToLower(code), hash) || manager.Verify("AAAAAA", hash) {
			t.Fatal("pickup code verification failed")
		}
		seen[code] = true
	}
}

func TestGuardianAuthorizationRequiresDistinctPeopleAndValidWindow(t *testing.T) {
	now := time.Now().UTC()
	bad := []GuardianAuthorizationInput{
		{BranchID: "branch", ChildPersonID: "same", GuardianPersonID: "same", Relationship: "parent"},
		{BranchID: "branch", ChildPersonID: "child", GuardianPersonID: "guardian", Relationship: "", ValidFrom: now},
		{BranchID: "branch", ChildPersonID: "child", GuardianPersonID: "guardian", Relationship: "parent", ValidFrom: now, ValidUntil: timePointerSafeguarding(now.Add(-time.Hour))},
	}
	for _, input := range bad {
		if err := input.normalize(now); err == nil {
			t.Fatalf("accepted invalid authorization: %+v", input)
		}
	}
}

func timePointerSafeguarding(value time.Time) *time.Time { return &value }
