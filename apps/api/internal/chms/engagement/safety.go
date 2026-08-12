package engagement

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"remi-api/internal/chms/platform"
)

var safetyRoles = []string{"product", "pastoral", "privacy"}

func (s Service) SafetyReadiness(ctx context.Context, p platform.Principal, branch platform.ID) (*SafetyReadiness, error) {
	if !branch.Valid() {
		return nil, platform.ValidationError(platform.FieldError{Path: "branchId", Code: "required", Message: "Choose a branch."})
	}
	if !s.allowed(p, "read", branch) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot read retention safety evidence for this branch."}
	}
	rules, fingerprint, policies, err := s.safetyPolicySet(ctx, p.OrganizationID, branch)
	if err != nil {
		return nil, err
	}
	reviews, err := s.Store.ListSafetyReviews(ctx, p.OrganizationID, branch)
	if err != nil {
		return nil, err
	}
	result := &SafetyReadiness{MetricVersion: "retention-v1", BranchID: branch, PolicyFingerprint: fingerprint, Policies: policies, RequiredRoles: append([]string{}, safetyRoles...), EligibleRoles: []string{}, RoleStatus: map[string]string{}, ReleaseState: "awaiting-human-review", Reviews: reviews, GeneratedAt: s.now()}
	for _, role := range safetyRoles {
		result.RoleStatus[role] = "pending"
		if reviewerMatchesAll(rules, role, p.Actor.ID) {
			result.EligibleRoles = append(result.EligibleRoles, role)
		}
	}
	seen := map[string]bool{}
	for _, review := range reviews {
		if review.PolicyFingerprint != fingerprint || seen[review.Role] {
			continue
		}
		seen[review.Role] = true
		result.RoleStatus[review.Role] = review.Decision
	}
	approved := true
	for _, role := range safetyRoles {
		if result.RoleStatus[role] != "approved" {
			approved = false
		}
		if result.RoleStatus[role] == "changes-required" {
			result.ReleaseState = "changes-required"
		}
	}
	if approved {
		result.ReleaseState = "approved"
	}
	return result, nil
}

func (s Service) SubmitSafetyReview(ctx context.Context, p platform.Principal, input SafetyReviewInput, requestID string) (*SafetyReadiness, error) {
	input.Role, input.Decision, input.Findings = strings.ToLower(strings.TrimSpace(input.Role)), strings.ToLower(strings.TrimSpace(input.Decision)), strings.TrimSpace(input.Findings)
	if !input.BranchID.Valid() || !contains(safetyRoles, input.Role) || (input.Decision != "approved" && input.Decision != "changes-required") || len(input.Findings) < 20 || len(input.Findings) > 2000 {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_review", Message: "Choose a branch, reviewer role and decision, then record findings of 20 to 2,000 characters."})
	}
	if input.Decision == "approved" && !input.Checklist.complete() {
		return nil, platform.ValidationError(platform.FieldError{Path: "checklist", Code: "incomplete", Message: "Every safety statement must be confirmed before approval."})
	}
	if !s.allowed(p, "approve", input.BranchID) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot approve retention safety evidence for this branch."}
	}
	rules, fingerprint, _, err := s.safetyPolicySet(ctx, p.OrganizationID, input.BranchID)
	if err != nil {
		return nil, err
	}
	if !reviewerMatchesAll(rules, input.Role, p.Actor.ID) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "Only the named reviewer for every published policy in this branch may submit this role's decision."}
	}
	now := s.now()
	review := SafetyReview{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: input.BranchID, MetricVersion: "retention-v1", PolicyFingerprint: fingerprint, Role: input.Role, Decision: input.Decision, Findings: input.Findings, Checklist: input.Checklist, Actor: p.Actor, RequestID: requestID, ReviewedAt: now}
	if err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if insertErr := s.Store.InsertSafetyReview(tx, review); insertErr != nil {
			return insertErr
		}
		return s.evidence(tx, p, input.BranchID, "engagement.safety-review."+input.Decision, "retention-safety-review", review.ID, 1, requestID, now)
	}); err != nil {
		return nil, err
	}
	return s.SafetyReadiness(ctx, p, input.BranchID)
}

func (s Service) safetyPolicySet(ctx context.Context, organizationID, branch platform.ID) ([]Rule, string, []SafetyPolicyReference, error) {
	values, err := s.Store.ListRules(ctx, organizationID, branch)
	if err != nil {
		return nil, "", nil, err
	}
	rules := []Rule{}
	for _, rule := range values {
		if rule.Status == "published" && rule.MetricVersion == "retention-v1" {
			rules = append(rules, rule)
		}
	}
	if len(rules) == 0 {
		return nil, "", nil, &platform.DomainError{Code: "conflict", Message: "Publish an approved branch retention policy before safety review."}
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].ID < rules[j].ID })
	hash := sha256.New()
	policies := make([]SafetyPolicyReference, 0, len(rules))
	for _, rule := range rules {
		_, _ = fmt.Fprintf(hash, "%s:%d:%s|", rule.ID, rule.Version, rule.UpdatedAt.UTC().Format("20060102T150405.000000000Z"))
		policies = append(policies, SafetyPolicyReference{ID: rule.ID, Version: rule.Version, Name: rule.Name, Kind: rule.Kind, PublishedAt: rule.PublishedAt})
	}
	return rules, hex.EncodeToString(hash.Sum(nil)), policies, nil
}

func reviewerMatchesAll(rules []Rule, role string, actor platform.ID) bool {
	if !actor.Valid() || len(rules) == 0 {
		return false
	}
	for _, rule := range rules {
		var expected platform.ID
		switch role {
		case "product":
			expected = rule.Approval.ProductOwnerID
		case "pastoral":
			expected = rule.Approval.PastoralApproverID
		case "privacy":
			expected = rule.Approval.PrivacyApproverID
		}
		if expected != actor {
			return false
		}
	}
	return true
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
