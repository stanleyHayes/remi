package finance

import (
	"math"
	"testing"
	"time"

	"remi-api/internal/chms/platform"
)

func validContributionInput() ContributionInput {
	return ContributionInput{ReceivedAt: time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC), BranchID: "accra", Donor: DonorAttribution{Type: "anonymous"}, Source: "online", PaymentMethodID: "card", Total: platform.Money{AmountMinor: 10000, Currency: "GHS"}, Splits: []ContributionSplit{{FundID: "general", Amount: platform.Money{AmountMinor: 10000, Currency: "GHS"}}}, PostingAction: "post"}
}
func TestContributionInputProtectsIdentityTenderAndBalance(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ContributionInput)
	}{
		{"anonymous identity", func(i *ContributionInput) { i.Donor.PersonID = "person-1" }},
		{"unbalanced split", func(i *ContributionInput) { i.Splits[0].Amount.AmountMinor = 9999 }},
		{"offline without batch", func(i *ContributionInput) { i.Source = "cash" }},
		{"non GHS", func(i *ContributionInput) { i.Total.Currency = "USD"; i.Splits[0].Amount.Currency = "USD" }},
		{"import receipt on live gift", func(i *ContributionInput) { i.SourceReceiptNumber = "legacy-1" }},
		{"overflowing splits", func(i *ContributionInput) {
			i.Total.AmountMinor = math.MaxInt64
			i.Splits = []ContributionSplit{{FundID: "one", Amount: platform.Money{AmountMinor: math.MaxInt64, Currency: "GHS"}}, {FundID: "two", Amount: platform.Money{AmountMinor: 1, Currency: "GHS"}}}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validContributionInput()
			test.mutate(&input)
			if err := input.NormalizeAndValidate(); err == nil {
				t.Fatal("invalid contribution was accepted")
			}
		})
	}
}
func TestDonorAttributionFiniteIdentityRules(t *testing.T) {
	valid := []DonorAttribution{{Type: "person", PersonID: "p1"}, {Type: "household", HouseholdID: "h1"}, {Type: "anonymous"}, {Type: "unresolved"}}
	for _, value := range valid {
		copy := value
		if err := copy.NormalizeAndValidate(); err != nil {
			t.Fatalf("%+v: %v", value, err)
		}
	}
}
