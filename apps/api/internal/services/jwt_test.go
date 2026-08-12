package services

import (
	"testing"
	"time"
)

func TestMFAAuthenticatedSessionCarriesConfirmationTime(t *testing.T) {
	service := NewJWTService("jwt-test-secret-with-at-least-32-bytes")
	confirmed := time.Now().UTC().Truncate(time.Second)
	token, err := service.GenerateMFAAuthenticated("user-1", "finance@example.com", "Finance", "super-admin", confirmed)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := service.Validate(token)
	if err != nil || claims.MFAAt != confirmed.Unix() {
		t.Fatalf("claims=%+v err=%v", claims, err)
	}
}

func TestScopedStaffSessionCarriesOnlyAssignedScopes(t *testing.T) {
	service := NewJWTService("jwt-test-secret-with-at-least-32-bytes")
	token, err := service.GenerateScoped("user-1", "pastor@example.com", "Pastor", "pastor", []string{"branch-1"}, []string{"care"}, []string{"case-1"}, 3)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := service.Validate(token)
	if err != nil {
		t.Fatal(err)
	}
	if len(claims.BranchIDs) != 1 || claims.BranchIDs[0] != "branch-1" || len(claims.MinistryIDs) != 1 || claims.MinistryIDs[0] != "care" || len(claims.AssignedResourceIDs) != 1 || claims.AssignedResourceIDs[0] != "case-1" {
		t.Fatalf("scopes were not preserved: %+v", claims)
	}
	if claims.AccessVersion == nil || *claims.AccessVersion != 3 {
		t.Fatalf("access version=%v", claims.AccessVersion)
	}
}
