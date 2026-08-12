package finance

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"remi-api/internal/chms/platform"
)

const CurrentSchemaVersion = 1

var codePattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{1,31}$`)

type Fund struct {
	platform.ResourceEnvelope `bson:",inline"`
	Code                      string      `json:"code" bson:"code"`
	Name                      string      `json:"name" bson:"name"`
	Description               string      `json:"description,omitempty" bson:"description,omitempty"`
	RestrictionType           string      `json:"restrictionType" bson:"restrictionType"`
	ActiveFrom                time.Time   `json:"activeFrom" bson:"activeFrom"`
	ActiveUntil               *time.Time  `json:"activeUntil,omitempty" bson:"activeUntil,omitempty"`
	SuccessorFundID           platform.ID `json:"successorFundId,omitempty" bson:"successorFundId,omitempty"`
}

type PaymentMethod struct {
	platform.ResourceEnvelope `bson:",inline"`
	Code                      string `json:"code" bson:"code"`
	Name                      string `json:"name" bson:"name"`
	Kind                      string `json:"kind" bson:"kind"`
	Provider                  string `json:"provider,omitempty" bson:"provider,omitempty"`
	Active                    bool   `json:"active" bson:"active"`
}

type CampusSettings struct {
	platform.ResourceEnvelope `bson:",inline"`
	Name                      string `json:"name" bson:"name"`
	Timezone                  string `json:"timezone" bson:"timezone"`
	Currency                  string `json:"currency" bson:"currency"`
}

type FiscalPeriod struct {
	platform.ResourceEnvelope `bson:",inline"`
	Code                      string    `json:"code" bson:"code"`
	Name                      string    `json:"name" bson:"name"`
	StartsAt                  time.Time `json:"startsAt" bson:"startsAt"`
	EndsAt                    time.Time `json:"endsAt" bson:"endsAt"`
	Status                    string    `json:"status" bson:"status"`
}

type ReceiptSequence struct {
	platform.ResourceEnvelope `bson:",inline"`
	Code                      string `json:"code" bson:"code"`
	Prefix                    string `json:"prefix" bson:"prefix"`
	FiscalYear                int    `json:"fiscalYear" bson:"fiscalYear"`
	Padding                   int    `json:"padding" bson:"padding"`
	NextNumber                int64  `json:"nextNumber" bson:"nextNumber"`
}

type AccountMapping struct {
	platform.ResourceEnvelope `bson:",inline"`
	FundID                    platform.ID `json:"fundId,omitempty" bson:"fundId,omitempty"`
	Category                  string      `json:"category" bson:"category"`
	ExternalAccountCode       string      `json:"externalAccountCode" bson:"externalAccountCode"`
	ExternalDimensionCode     string      `json:"externalDimensionCode,omitempty" bson:"externalDimensionCode,omitempty"`
	EffectiveFrom             time.Time   `json:"effectiveFrom" bson:"effectiveFrom"`
	EffectiveUntil            *time.Time  `json:"effectiveUntil,omitempty" bson:"effectiveUntil,omitempty"`
}

type FundInput struct {
	Code, Name, Description, RestrictionType string
	ActiveFrom                               time.Time
	ActiveUntil                              *time.Time
	SuccessorFundID                          platform.ID
	ExpectedVersion                          int64
}
type DeactivateFundInput struct {
	ExpectedVersion int64       `json:"expectedVersion"`
	Reason          string      `json:"reason"`
	SuccessorFundID platform.ID `json:"successorFundId"`
}
type PaymentMethodInput struct {
	Code, Name, Kind, Provider string
	Active                     bool
	ExpectedVersion            int64
}
type CampusSettingsInput struct {
	BranchID                 platform.ID
	Name, Timezone, Currency string
	ExpectedVersion          int64
}
type FiscalPeriodInput struct {
	Code, Name       string
	StartsAt, EndsAt time.Time
	ExpectedVersion  int64
}
type ReceiptSequenceInput struct {
	Code, Prefix        string
	FiscalYear, Padding int
	StartingNumber      int64
	ExpectedVersion     int64
}
type AccountMappingInput struct {
	BranchID, FundID                                     platform.ID
	Category, ExternalAccountCode, ExternalDimensionCode string
	EffectiveFrom                                        time.Time
	EffectiveUntil                                       *time.Time
	ExpectedVersion                                      int64
}

func normalizeCode(value string) string { return strings.ToUpper(strings.TrimSpace(value)) }
func validateCode(path, value string) error {
	if !codePattern.MatchString(value) {
		return fmt.Errorf("%s must contain 2-32 uppercase letters, numbers, hyphens or underscores", path)
	}
	return nil
}
func validName(value string) bool { n := len(strings.TrimSpace(value)); return n >= 2 && n <= 120 }
func validRestriction(v string) bool {
	return map[string]bool{"unrestricted": true, "temporarily-restricted": true, "board-designated": true}[v]
}
func validMethodKind(v string) bool {
	return map[string]bool{"cash": true, "cheque": true, "mobile-money": true, "card": true, "bank-transfer": true, "in-kind": true}[v]
}
func validMappingCategory(v string) bool {
	return map[string]bool{"contribution": true, "cash": true, "cheque": true, "mobile-money": true, "card": true, "bank-transfer": true, "in-kind": true, "fee": true, "refund": true}[v]
}

func (i *FundInput) NormalizeAndValidate() error {
	i.Code, i.Name, i.Description = normalizeCode(i.Code), strings.TrimSpace(i.Name), strings.TrimSpace(i.Description)
	i.RestrictionType = strings.ToLower(strings.TrimSpace(i.RestrictionType))
	if err := validateCode("code", i.Code); err != nil {
		return err
	}
	if !validName(i.Name) {
		return fmt.Errorf("name must be between 2 and 120 characters")
	}
	if !validRestriction(i.RestrictionType) {
		return fmt.Errorf("unsupported restriction type")
	}
	if i.ActiveFrom.IsZero() {
		return fmt.Errorf("activeFrom is required")
	}
	if i.ActiveUntil != nil && !i.ActiveUntil.After(i.ActiveFrom) {
		return fmt.Errorf("activeUntil must follow activeFrom")
	}
	return nil
}
func (i *PaymentMethodInput) NormalizeAndValidate() error {
	i.Code, i.Name, i.Kind, i.Provider = normalizeCode(i.Code), strings.TrimSpace(i.Name), strings.ToLower(strings.TrimSpace(i.Kind)), strings.TrimSpace(i.Provider)
	if err := validateCode("code", i.Code); err != nil {
		return err
	}
	if !validName(i.Name) || !validMethodKind(i.Kind) {
		return fmt.Errorf("valid name and supported payment kind are required")
	}
	return nil
}
func (i *CampusSettingsInput) NormalizeAndValidate() error {
	i.Name, i.Timezone, i.Currency = strings.TrimSpace(i.Name), strings.TrimSpace(i.Timezone), strings.ToUpper(strings.TrimSpace(i.Currency))
	if !i.BranchID.Valid() || !validName(i.Name) {
		return fmt.Errorf("branchId and name are required")
	}
	if _, err := time.LoadLocation(i.Timezone); err != nil {
		return fmt.Errorf("timezone must be an IANA timezone")
	}
	if i.Currency != "GHS" {
		return fmt.Errorf("only GHS is currently supported")
	}
	return nil
}
func (i *FiscalPeriodInput) NormalizeAndValidate() error {
	i.Code, i.Name = normalizeCode(i.Code), strings.TrimSpace(i.Name)
	if err := validateCode("code", i.Code); err != nil {
		return err
	}
	if !validName(i.Name) || i.StartsAt.IsZero() || !i.EndsAt.After(i.StartsAt) {
		return fmt.Errorf("valid name, startsAt and endsAt are required")
	}
	return nil
}
func (i *ReceiptSequenceInput) NormalizeAndValidate() error {
	i.Code, i.Prefix = normalizeCode(i.Code), strings.TrimSpace(i.Prefix)
	if err := validateCode("code", i.Code); err != nil {
		return err
	}
	if i.FiscalYear < 2000 || i.FiscalYear > 2200 || i.Padding < 4 || i.Padding > 12 || i.StartingNumber < 1 {
		return fmt.Errorf("valid fiscalYear, padding 4-12 and positive startingNumber are required")
	}
	if len(i.Prefix) < 2 || len(i.Prefix) > 32 {
		return fmt.Errorf("prefix must be between 2 and 32 characters")
	}
	return nil
}
func (i *AccountMappingInput) NormalizeAndValidate() error {
	i.Category, i.ExternalAccountCode, i.ExternalDimensionCode = strings.ToLower(strings.TrimSpace(i.Category)), strings.TrimSpace(i.ExternalAccountCode), strings.TrimSpace(i.ExternalDimensionCode)
	if !i.BranchID.Valid() || !validMappingCategory(i.Category) || i.ExternalAccountCode == "" || len(i.ExternalAccountCode) > 80 || i.EffectiveFrom.IsZero() {
		return fmt.Errorf("branchId, supported category, external account code and effectiveFrom are required")
	}
	if i.EffectiveUntil != nil && !i.EffectiveUntil.After(i.EffectiveFrom) {
		return fmt.Errorf("effectiveUntil must follow effectiveFrom")
	}
	return nil
}
