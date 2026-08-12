package finance

import (
	"fmt"
	"strings"
	"time"

	"remi-api/internal/chms/platform"
)

type CountingBatch struct {
	platform.ResourceEnvelope `bson:",inline"`
	ReceivedAt                time.Time      `json:"receivedAt" bson:"receivedAt"`
	OccurrenceID              platform.ID    `json:"occurrenceId,omitempty" bson:"occurrenceId,omitempty"`
	CounterIDs                []platform.ID  `json:"counterIds" bson:"counterIds"`
	DualControlRequired       bool           `json:"dualControlRequired" bson:"dualControlRequired"`
	ExpectedPaymentMethodIDs  []platform.ID  `json:"expectedPaymentMethodIds" bson:"expectedPaymentMethodIds"`
	State                     string         `json:"state" bson:"state"`
	EntryCount                int64          `json:"entryCount" bson:"entryCount"`
	EnteredTotal              platform.Money `json:"enteredTotal" bson:"enteredTotal"`
	DeclaredTotal             platform.Money `json:"declaredTotal" bson:"declaredTotal"`
	Variance                  platform.Money `json:"variance" bson:"variance"`
	ConfirmationCount         int            `json:"confirmationCount" bson:"confirmationCount"`
	ApprovedBy                platform.ID    `json:"approvedBy,omitempty" bson:"approvedBy,omitempty"`
	ApprovedAt                *time.Time     `json:"approvedAt,omitempty" bson:"approvedAt,omitempty"`
	PostedBy                  platform.ID    `json:"postedBy,omitempty" bson:"postedBy,omitempty"`
	PostedAt                  *time.Time     `json:"postedAt,omitempty" bson:"postedAt,omitempty"`
}
type BatchEntry struct {
	platform.ResourceEnvelope `bson:",inline"`
	BatchID                   platform.ID         `json:"batchId" bson:"batchId"`
	Donor                     DonorAttribution    `json:"donor" bson:"donor"`
	Source                    string              `json:"source" bson:"source"`
	PaymentMethodID           platform.ID         `json:"paymentMethodId" bson:"paymentMethodId"`
	PaymentMethodReference    string              `json:"paymentMethodReference,omitempty" bson:"paymentMethodReference,omitempty"`
	Total                     platform.Money      `json:"total" bson:"total"`
	Splits                    []ContributionSplit `json:"splits" bson:"splits"`
	Provenance                string              `json:"provenance,omitempty" bson:"provenance,omitempty"`
}
type TenderTotal struct {
	PaymentMethodID platform.ID    `json:"paymentMethodId" bson:"paymentMethodId"`
	Amount          platform.Money `json:"amount" bson:"amount"`
}
type DenominationCount struct {
	PaymentMethodID platform.ID `json:"paymentMethodId" bson:"paymentMethodId"`
	ValueMinor      int64       `json:"valueMinor" bson:"valueMinor"`
	Count           int64       `json:"count" bson:"count"`
	TotalMinor      int64       `json:"totalMinor" bson:"totalMinor"`
}
type PrivateAttachment struct {
	PublicID     string `json:"publicId" bson:"publicId"`
	ResourceType string `json:"resourceType" bson:"resourceType"`
	Label        string `json:"label,omitempty" bson:"label,omitempty"`
}
type CountConfirmation struct {
	ID             platform.ID         `json:"id" bson:"_id"`
	OrganizationID platform.ID         `json:"organizationId" bson:"organizationId"`
	BranchID       platform.ID         `json:"branchId" bson:"branchId"`
	BatchID        platform.ID         `json:"batchId" bson:"batchId"`
	CounterID      platform.ID         `json:"counterId" bson:"counterId"`
	TenderTotals   []TenderTotal       `json:"tenderTotals" bson:"tenderTotals"`
	Denominations  []DenominationCount `json:"denominations" bson:"denominations"`
	Attachments    []PrivateAttachment `json:"attachments" bson:"attachments"`
	DeclaredTotal  platform.Money      `json:"declaredTotal" bson:"declaredTotal"`
	ConfirmedAt    time.Time           `json:"confirmedAt" bson:"confirmedAt"`
}
type BatchEvent struct {
	ID             platform.ID    `json:"id" bson:"_id"`
	OrganizationID platform.ID    `json:"organizationId" bson:"organizationId"`
	BranchID       platform.ID    `json:"branchId" bson:"branchId"`
	BatchID        platform.ID    `json:"batchId" bson:"batchId"`
	Sequence       int64          `json:"sequence" bson:"sequence"`
	Type           string         `json:"type" bson:"type"`
	Actor          platform.Actor `json:"actor" bson:"actor"`
	Reason         string         `json:"reason,omitempty" bson:"reason,omitempty"`
	OccurredAt     time.Time      `json:"occurredAt" bson:"occurredAt"`
}

type CountingBatchInput struct {
	BranchID                 platform.ID   `json:"branchId"`
	ReceivedAt               time.Time     `json:"receivedAt"`
	OccurrenceID             platform.ID   `json:"occurrenceId"`
	CounterIDs               []platform.ID `json:"counterIds"`
	DualControlRequired      bool          `json:"dualControlRequired"`
	ExpectedPaymentMethodIDs []platform.ID `json:"expectedPaymentMethodIds"`
}
type BatchEntryInput struct {
	Donor                  DonorAttribution    `json:"donor"`
	Source                 string              `json:"source"`
	PaymentMethodID        platform.ID         `json:"paymentMethodId"`
	PaymentMethodReference string              `json:"paymentMethodReference"`
	Total                  platform.Money      `json:"total"`
	Splits                 []ContributionSplit `json:"splits"`
	Provenance             string              `json:"provenance"`
	ExpectedVersion        int64               `json:"expectedVersion"`
}
type CountConfirmationInput struct {
	ExpectedVersion int64               `json:"expectedVersion"`
	TenderTotals    []TenderTotal       `json:"tenderTotals"`
	Denominations   []DenominationCount `json:"denominations"`
	Attachments     []PrivateAttachment `json:"attachments"`
}
type BatchTransitionInput struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	Reason          string `json:"reason"`
}

func uniqueIDs(values []platform.ID) bool {
	seen := map[platform.ID]bool{}
	for _, value := range values {
		if !value.Valid() || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}
func (i *CountingBatchInput) NormalizeAndValidate() error {
	if !i.BranchID.Valid() || i.ReceivedAt.IsZero() {
		return fmt.Errorf("branchId and receivedAt are required")
	}
	if !uniqueIDs(i.CounterIDs) || len(i.CounterIDs) < 1 || len(i.CounterIDs) > 2 {
		return fmt.Errorf("one or two distinct counters are required")
	}
	if i.DualControlRequired && len(i.CounterIDs) != 2 {
		return fmt.Errorf("dual control requires exactly two counters")
	}
	if !uniqueIDs(i.ExpectedPaymentMethodIDs) || len(i.ExpectedPaymentMethodIDs) < 1 || len(i.ExpectedPaymentMethodIDs) > 10 {
		return fmt.Errorf("one to ten distinct payment methods are required")
	}
	return nil
}
func (i *BatchEntryInput) NormalizeAndValidate() error {
	i.Source = strings.ToLower(strings.TrimSpace(i.Source))
	i.PaymentMethodReference = strings.TrimSpace(i.PaymentMethodReference)
	i.Provenance = strings.TrimSpace(i.Provenance)
	if err := i.Donor.NormalizeAndValidate(); err != nil {
		return err
	}
	if !map[string]bool{"cash": true, "cheque": true, "mobile-money": true, "bank-transfer": true, "in-kind": true}[i.Source] {
		return fmt.Errorf("batch entry requires an offline contribution source")
	}
	if !i.PaymentMethodID.Valid() {
		return fmt.Errorf("paymentMethodId is required")
	}
	i.Total.Currency = strings.ToUpper(strings.TrimSpace(i.Total.Currency))
	if i.Total.Currency != "GHS" || i.Total.AmountMinor <= 0 {
		return fmt.Errorf("total must be a positive GHS amount")
	}
	if err := normalizeContributionSplits(i.Splits, "GHS", i.Total.AmountMinor, false); err != nil {
		return err
	}
	if len(i.PaymentMethodReference) > 160 || len(i.Provenance) > 200 {
		return fmt.Errorf("entry references exceed maximum length")
	}
	return nil
}
func (i *CountConfirmationInput) NormalizeAndValidate() (platform.Money, error) {
	if err := platform.RequireExpectedVersion(i.ExpectedVersion); err != nil {
		return platform.Money{}, err
	}
	if len(i.TenderTotals) < 1 || len(i.TenderTotals) > 10 {
		return platform.Money{}, fmt.Errorf("one to ten tender totals are required")
	}
	seen := map[platform.ID]bool{}
	total := platform.Money{Currency: "GHS"}
	for idx := range i.TenderTotals {
		line := &i.TenderTotals[idx]
		line.Amount.Currency = strings.ToUpper(strings.TrimSpace(line.Amount.Currency))
		if !line.PaymentMethodID.Valid() || seen[line.PaymentMethodID] || line.Amount.Currency != "GHS" || line.Amount.AmountMinor < 0 {
			return platform.Money{}, fmt.Errorf("tender totals must use unique methods and non-negative GHS amounts")
		}
		seen[line.PaymentMethodID] = true
		next, err := total.Add(line.Amount)
		if err != nil {
			return platform.Money{}, err
		}
		total = next
	}
	if len(i.Denominations) > 100 {
		return platform.Money{}, fmt.Errorf("too many denomination rows")
	}
	for idx := range i.Denominations {
		row := &i.Denominations[idx]
		if !row.PaymentMethodID.Valid() || row.ValueMinor <= 0 || row.Count < 0 || row.ValueMinor > 1_000_000_00 {
			return platform.Money{}, fmt.Errorf("invalid denomination row")
		}
		if row.Count > 0 && row.ValueMinor > int64(^uint64(0)>>1)/row.Count {
			return platform.Money{}, fmt.Errorf("denomination total overflow")
		}
		row.TotalMinor = row.ValueMinor * row.Count
	}
	for idx := range i.Attachments {
		a := &i.Attachments[idx]
		a.PublicID = strings.TrimSpace(a.PublicID)
		a.ResourceType = strings.ToLower(strings.TrimSpace(a.ResourceType))
		a.Label = strings.TrimSpace(a.Label)
		if !strings.HasPrefix(a.PublicID, "remi/finance/") || (a.ResourceType != "image" && a.ResourceType != "raw") || len(a.Label) > 80 {
			return platform.Money{}, fmt.Errorf("attachments must be private finance asset references")
		}
	}
	if len(i.Attachments) > 20 {
		return platform.Money{}, fmt.Errorf("too many attachments")
	}
	return total, nil
}
