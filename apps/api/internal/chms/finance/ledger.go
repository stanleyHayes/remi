package finance

import (
	"fmt"
	"strings"
	"time"

	"remi-api/internal/chms/platform"
)

type DonorAttribution struct {
	Type        string      `json:"type" bson:"type"`
	PersonID    platform.ID `json:"personId,omitempty" bson:"personId,omitempty"`
	HouseholdID platform.ID `json:"householdId,omitempty" bson:"householdId,omitempty"`
}
type ContributionSplit struct {
	FundID platform.ID    `json:"fundId" bson:"fundId"`
	Amount platform.Money `json:"amount" bson:"amount"`
}
type ContributionLink struct {
	Type                   string      `json:"type,omitempty" bson:"type,omitempty"`
	ChainRootID            platform.ID `json:"chainRootId,omitempty" bson:"chainRootId,omitempty"`
	ReversesContributionID platform.ID `json:"reversesContributionId,omitempty" bson:"reversesContributionId,omitempty"`
	ReplacesContributionID platform.ID `json:"replacesContributionId,omitempty" bson:"replacesContributionId,omitempty"`
	ProviderAdjustsID      platform.ID `json:"providerAdjustsContributionId,omitempty" bson:"providerAdjustsContributionId,omitempty"`
	ProviderEventReference string      `json:"providerEventReference,omitempty" bson:"providerEventReference,omitempty"`
	Reason                 string      `json:"reason,omitempty" bson:"reason,omitempty"`
}
type Contribution struct {
	platform.ResourceEnvelope `bson:",inline"`
	ReceiptNumber             string              `json:"receiptNumber" bson:"receiptNumber"`
	ReceivedAt                time.Time           `json:"receivedAt" bson:"receivedAt"`
	PostedAt                  time.Time           `json:"postedAt" bson:"postedAt"`
	Donor                     DonorAttribution    `json:"donor" bson:"donor"`
	Source                    string              `json:"source" bson:"source"`
	PaymentMethodID           platform.ID         `json:"paymentMethodId" bson:"paymentMethodId"`
	PaymentMethodReference    string              `json:"paymentMethodReference,omitempty" bson:"paymentMethodReference,omitempty"`
	ProviderReference         string              `json:"providerReference,omitempty" bson:"providerReference,omitempty"`
	SourceReceiptNumber       string              `json:"sourceReceiptNumber,omitempty" bson:"sourceReceiptNumber,omitempty"`
	BatchID                   platform.ID         `json:"batchId,omitempty" bson:"batchId,omitempty"`
	CampaignID                platform.ID         `json:"campaignId,omitempty" bson:"campaignId,omitempty"`
	PledgeID                  platform.ID         `json:"pledgeId,omitempty" bson:"pledgeId,omitempty"`
	FiscalPeriodID            platform.ID         `json:"fiscalPeriodId" bson:"fiscalPeriodId"`
	Total                     platform.Money      `json:"total" bson:"total"`
	Splits                    []ContributionSplit `json:"splits" bson:"splits"`
	State                     string              `json:"state" bson:"state"`
	EffectiveState            string              `json:"effectiveState" bson:"-"`
	Provenance                string              `json:"provenance" bson:"provenance"`
	CommandKey                string              `json:"-" bson:"commandKey"`
	CommandHash               string              `json:"-" bson:"commandHash"`
	Link                      *ContributionLink   `json:"adjustment,omitempty" bson:"adjustment,omitempty"`
}
type ReceiptAllocation struct {
	ID             platform.ID `json:"id" bson:"_id"`
	OrganizationID platform.ID `json:"organizationId" bson:"organizationId"`
	SequenceID     platform.ID `json:"sequenceId" bson:"sequenceId"`
	ReceiptNumber  string      `json:"receiptNumber" bson:"receiptNumber"`
	ContributionID platform.ID `json:"contributionId" bson:"contributionId"`
	State          string      `json:"state" bson:"state"`
	Reason         string      `json:"reason,omitempty" bson:"reason,omitempty"`
	AllocatedAt    time.Time   `json:"allocatedAt" bson:"allocatedAt"`
}
type ContributionInput struct {
	ReceivedAt             time.Time           `json:"receivedAt"`
	BranchID               platform.ID         `json:"branchId"`
	Donor                  DonorAttribution    `json:"donor"`
	Source                 string              `json:"source"`
	PaymentMethodID        platform.ID         `json:"paymentMethodId"`
	PaymentMethodReference string              `json:"paymentMethodReference"`
	ProviderReference      string              `json:"providerReference"`
	SourceReceiptNumber    string              `json:"sourceReceiptNumber"`
	Total                  platform.Money      `json:"total"`
	Splits                 []ContributionSplit `json:"splits"`
	BatchID                platform.ID         `json:"batchId"`
	CampaignID             platform.ID         `json:"campaignId,omitempty"`
	PledgeID               platform.ID         `json:"pledgeId,omitempty"`
	PostingAction          string              `json:"postingAction"`
	Provenance             string              `json:"provenance"`
}
type AdjustmentInput struct {
	Type                   string              `json:"type"`
	Reason                 string              `json:"reason"`
	AmountMinor            int64               `json:"amountMinor,omitempty"`
	ProviderEventReference string              `json:"providerEventReference,omitempty"`
	ReplacementDonor       *DonorAttribution   `json:"replacementDonor,omitempty"`
	ReplacementSplits      []ContributionSplit `json:"replacementSplits,omitempty"`
}
type AdjustmentResult struct {
	Reversal    *Contribution `json:"reversal"`
	Replacement *Contribution `json:"replacement,omitempty"`
}

func (d *DonorAttribution) NormalizeAndValidate() error {
	d.Type = strings.ToLower(strings.TrimSpace(d.Type))
	switch d.Type {
	case "person":
		if !d.PersonID.Valid() || d.HouseholdID.Valid() {
			return fmt.Errorf("person donor requires only personId")
		}
	case "household":
		if !d.HouseholdID.Valid() || d.PersonID.Valid() {
			return fmt.Errorf("household donor requires only householdId")
		}
	case "anonymous", "unresolved":
		if d.PersonID.Valid() || d.HouseholdID.Valid() {
			return fmt.Errorf("anonymous or unresolved donor cannot include identity")
		}
	default:
		return fmt.Errorf("unsupported donor type")
	}
	return nil
}
func normalizeContributionSplits(splits []ContributionSplit, currency string, expected int64, allowNegative bool) error {
	if len(splits) < 1 || len(splits) > 100 {
		return fmt.Errorf("one to 100 fund splits are required")
	}
	seen := map[platform.ID]bool{}
	sum := platform.Money{Currency: currency}
	for idx := range splits {
		split := &splits[idx]
		if !split.FundID.Valid() || seen[split.FundID] {
			return fmt.Errorf("split funds must be present and unique")
		}
		seen[split.FundID] = true
		split.Amount.Currency = strings.ToUpper(strings.TrimSpace(split.Amount.Currency))
		if split.Amount.Currency != currency {
			return fmt.Errorf("split currency must match total")
		}
		if (!allowNegative && split.Amount.AmountMinor <= 0) || (allowNegative && split.Amount.AmountMinor >= 0) {
			return fmt.Errorf("split amount sign is invalid")
		}
		next, err := sum.Add(split.Amount)
		if err != nil {
			return err
		}
		sum = next
	}
	if sum.AmountMinor != expected {
		return fmt.Errorf("split sum must equal contribution total")
	}
	return nil
}
func (i *ContributionInput) NormalizeAndValidate() error {
	i.Source = strings.ToLower(strings.TrimSpace(i.Source))
	i.PaymentMethodReference = strings.TrimSpace(i.PaymentMethodReference)
	i.ProviderReference = strings.TrimSpace(i.ProviderReference)
	i.SourceReceiptNumber = strings.TrimSpace(i.SourceReceiptNumber)
	i.PostingAction = strings.ToLower(strings.TrimSpace(i.PostingAction))
	i.Provenance = strings.TrimSpace(i.Provenance)
	i.Total.Currency = strings.ToUpper(strings.TrimSpace(i.Total.Currency))
	if i.ReceivedAt.IsZero() || !i.BranchID.Valid() {
		return fmt.Errorf("receivedAt and branchId are required")
	}
	if err := i.Donor.NormalizeAndValidate(); err != nil {
		return err
	}
	if !map[string]bool{"online": true, "cash": true, "cheque": true, "mobile-money": true, "bank-transfer": true, "in-kind": true, "import": true}[i.Source] {
		return fmt.Errorf("unsupported contribution source")
	}
	if !i.PaymentMethodID.Valid() {
		return fmt.Errorf("paymentMethodId is required")
	}
	if i.PostingAction != "post" {
		return fmt.Errorf("postingAction must be post; staged batches use the counting workflow")
	}
	if i.Total.Currency != "GHS" || i.Total.AmountMinor <= 0 {
		return fmt.Errorf("total must be a positive GHS amount")
	}
	if err := normalizeContributionSplits(i.Splits, i.Total.Currency, i.Total.AmountMinor, false); err != nil {
		return err
	}
	if map[string]bool{"cash": true, "cheque": true, "mobile-money": true, "in-kind": true}[i.Source] && !i.BatchID.Valid() {
		return fmt.Errorf("offline tender requires batchId")
	}
	if i.Source != "import" && i.SourceReceiptNumber != "" {
		return fmt.Errorf("sourceReceiptNumber is only valid for imported history")
	}
	if len(i.PaymentMethodReference) > 160 || len(i.ProviderReference) > 160 || len(i.SourceReceiptNumber) > 80 || len(i.Provenance) > 200 {
		return fmt.Errorf("references exceed maximum length")
	}
	return nil
}
func (i *AdjustmentInput) NormalizeAndValidate() error {
	i.Type = strings.ToLower(strings.TrimSpace(i.Type))
	i.Reason = strings.TrimSpace(i.Reason)
	i.ProviderEventReference = strings.TrimSpace(i.ProviderEventReference)
	providerAdjustment := i.Type == "provider-refund" || i.Type == "provider-chargeback"
	if i.Type != "reversal" && i.Type != "correction" && i.Type != "attribution-correction" && !providerAdjustment {
		return fmt.Errorf("unsupported adjustment type")
	}
	if err := platform.ValidateReason(i.Reason); err != nil {
		return err
	}
	if (i.Type == "reversal" || providerAdjustment) && (i.ReplacementDonor != nil || len(i.ReplacementSplits) > 0) {
		return fmt.Errorf("reversal cannot include replacement values")
	}
	if providerAdjustment {
		if i.AmountMinor <= 0 || len(i.ProviderEventReference) < 3 || len(i.ProviderEventReference) > 160 {
			return fmt.Errorf("provider adjustment requires a positive amount and provider event reference")
		}
		return nil
	}
	if i.AmountMinor != 0 || i.ProviderEventReference != "" {
		return fmt.Errorf("provider adjustment fields are not valid for a manual correction")
	}
	if i.Type != "reversal" && i.ReplacementDonor == nil && len(i.ReplacementSplits) == 0 {
		return fmt.Errorf("correction requires replacement donor or splits")
	}
	if i.ReplacementDonor != nil {
		return i.ReplacementDonor.NormalizeAndValidate()
	}
	return nil
}
