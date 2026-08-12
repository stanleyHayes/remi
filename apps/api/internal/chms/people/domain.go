package people

import (
	"errors"
	"net/mail"
	"regexp"
	"sort"
	"strings"
	"time"

	"remi-api/internal/chms/platform"
)

const CurrentSchemaVersion = 1

var nonPhone = regexp.MustCompile(`[^0-9+]`)

type Names struct {
	Given     string `json:"given" bson:"given"`
	Middle    string `json:"middle,omitempty" bson:"middle,omitempty"`
	Family    string `json:"family,omitempty" bson:"family,omitempty"`
	Preferred string `json:"preferred,omitempty" bson:"preferred,omitempty"`
}
type PartialDate struct {
	Value     string `json:"value" bson:"value"`
	Precision string `json:"precision" bson:"precision"`
}
type ContactPoint struct {
	Type       string     `json:"type" bson:"type"`
	Value      string     `json:"value" bson:"value"`
	Normalized string     `json:"-" bson:"normalized"`
	Primary    bool       `json:"primary" bson:"primary"`
	VerifiedAt *time.Time `json:"verifiedAt,omitempty" bson:"verifiedAt,omitempty"`
}
type Address struct {
	Label   string `json:"label,omitempty" bson:"label,omitempty"`
	Line1   string `json:"line1" bson:"line1"`
	Line2   string `json:"line2,omitempty" bson:"line2,omitempty"`
	City    string `json:"city,omitempty" bson:"city,omitempty"`
	Region  string `json:"region,omitempty" bson:"region,omitempty"`
	Country string `json:"country" bson:"country"`
	Primary bool   `json:"primary" bson:"primary"`
}
type CommunicationPreferences struct {
	Email    bool `json:"email" bson:"email"`
	SMS      bool `json:"sms" bson:"sms"`
	WhatsApp bool `json:"whatsapp" bson:"whatsapp"`
	Phone    bool `json:"phone" bson:"phone"`
}
type Source struct {
	Type          string `json:"type" bson:"type"`
	Reference     string `json:"reference,omitempty" bson:"reference,omitempty"`
	NoticeVersion string `json:"noticeVersion,omitempty" bson:"noticeVersion,omitempty"`
}

type Person struct {
	platform.ResourceEnvelope `bson:",inline"`
	PersonNumber              string                   `json:"personNumber" bson:"personNumber"`
	Names                     Names                    `json:"names" bson:"names"`
	Aliases                   []string                 `json:"aliases" bson:"aliases"`
	PhotoAssetID              platform.ID              `json:"photoAssetId,omitempty" bson:"photoAssetId,omitempty"`
	DateOfBirth               *PartialDate             `json:"dateOfBirth,omitempty" bson:"dateOfBirth,omitempty"`
	Gender                    *string                  `json:"gender,omitempty" bson:"gender,omitempty"`
	ContactPoints             []ContactPoint           `json:"contactPoints" bson:"contactPoints"`
	Addresses                 []Address                `json:"addresses" bson:"addresses"`
	HomeBranchID              platform.ID              `json:"homeBranchId" bson:"homeBranchId"`
	MembershipStage           string                   `json:"membershipStage" bson:"membershipStage"`
	Tags                      []string                 `json:"tags" bson:"tags"`
	CustomFields              map[string]string        `json:"customFields" bson:"customFields"`
	CommunicationPreferences  CommunicationPreferences `json:"communicationPreferences" bson:"communicationPreferences"`
	Source                    Source                   `json:"source" bson:"source"`
}

type CreateInput struct {
	OrganizationID           platform.ID
	HomeBranchID             platform.ID
	Names                    Names
	Aliases                  []string
	PhotoAssetID             platform.ID
	DateOfBirth              *PartialDate
	Gender                   *string
	ContactPoints            []ContactPoint
	Addresses                []Address
	MembershipStage          string
	Tags                     []string
	CustomFields             map[string]string
	CommunicationPreferences CommunicationPreferences
	Source                   Source
}

type UpdateInput struct {
	ExpectedVersion          int64                    `json:"expectedVersion"`
	HomeBranchID             platform.ID              `json:"homeBranchId"`
	Names                    Names                    `json:"names"`
	Aliases                  []string                 `json:"aliases"`
	PhotoAssetID             platform.ID              `json:"photoAssetId"`
	DateOfBirth              *PartialDate             `json:"dateOfBirth"`
	Gender                   *string                  `json:"gender"`
	ContactPoints            []ContactPoint           `json:"contactPoints"`
	Addresses                []Address                `json:"addresses"`
	MembershipStage          string                   `json:"membershipStage"`
	Tags                     []string                 `json:"tags"`
	CustomFields             map[string]string        `json:"customFields"`
	CommunicationPreferences CommunicationPreferences `json:"communicationPreferences"`
	Source                   Source                   `json:"source"`
}

func (in *UpdateInput) normalizeAndValidate(organizationID platform.ID) error {
	if err := platform.RequireExpectedVersion(in.ExpectedVersion); err != nil {
		return err
	}
	editable := CreateInput{OrganizationID: organizationID, HomeBranchID: in.HomeBranchID, Names: in.Names, Aliases: in.Aliases, PhotoAssetID: in.PhotoAssetID, DateOfBirth: in.DateOfBirth, Gender: in.Gender, ContactPoints: in.ContactPoints, Addresses: in.Addresses, MembershipStage: in.MembershipStage, Tags: in.Tags, CustomFields: in.CustomFields, CommunicationPreferences: in.CommunicationPreferences}
	if err := editable.NormalizeAndValidate(); err != nil {
		return err
	}
	in.HomeBranchID, in.Names, in.Aliases, in.PhotoAssetID = editable.HomeBranchID, editable.Names, editable.Aliases, editable.PhotoAssetID
	in.DateOfBirth, in.Gender, in.ContactPoints, in.Addresses = editable.DateOfBirth, editable.Gender, editable.ContactPoints, editable.Addresses
	in.MembershipStage, in.Tags, in.CustomFields, in.CommunicationPreferences = editable.MembershipStage, editable.Tags, editable.CustomFields, editable.CommunicationPreferences
	return nil
}

func (in *CreateInput) NormalizeAndValidate() error {
	if !in.OrganizationID.Valid() {
		return errors.New("organization is required")
	}
	if !in.HomeBranchID.Valid() {
		return errors.New("home branch is required")
	}
	in.Names.Given = strings.TrimSpace(in.Names.Given)
	in.Names.Middle = strings.TrimSpace(in.Names.Middle)
	in.Names.Family = strings.TrimSpace(in.Names.Family)
	in.Names.Preferred = strings.TrimSpace(in.Names.Preferred)
	if in.Names.Given == "" {
		return errors.New("given name is required")
	}
	if len(in.Names.Given) > 100 || len(in.Names.Middle) > 100 || len(in.Names.Family) > 100 || len(in.Names.Preferred) > 100 {
		return errors.New("name parts must not exceed 100 characters")
	}
	if in.MembershipStage == "" {
		in.MembershipStage = "guest"
	}
	if !allowedStage(in.MembershipStage) {
		return errors.New("invalid membership stage")
	}
	if in.DateOfBirth != nil {
		if err := validatePartialDate(*in.DateOfBirth); err != nil {
			return err
		}
	}
	for i := range in.ContactPoints {
		if err := normalizeContact(&in.ContactPoints[i]); err != nil {
			return err
		}
	}
	if err := onePrimaryContactPerType(in.ContactPoints); err != nil {
		return err
	}
	for i := range in.Addresses {
		in.Addresses[i].Line1 = strings.TrimSpace(in.Addresses[i].Line1)
		in.Addresses[i].Country = strings.ToUpper(strings.TrimSpace(in.Addresses[i].Country))
		if in.Addresses[i].Line1 == "" || len(in.Addresses[i].Country) != 2 {
			return errors.New("addresses require line1 and two-letter country code")
		}
	}
	in.Tags = normalizeTags(in.Tags)
	if len(in.CustomFields) > 50 {
		return errors.New("custom fields must not exceed 50 entries")
	}
	for key, value := range in.CustomFields {
		if strings.TrimSpace(key) == "" || len(key) > 64 || len(value) > 500 {
			return errors.New("invalid custom field")
		}
	}
	return nil
}

func allowedStage(stage string) bool {
	switch stage {
	case "guest", "returning-guest", "regular-attendee", "member", "inactive", "transferred", "deceased":
		return true
	}
	return false
}
func validatePartialDate(value PartialDate) error {
	layouts := map[string]string{"year": "2006", "month": "2006-01", "day": "2006-01-02"}
	layout, ok := layouts[value.Precision]
	if !ok {
		return errors.New("date precision must be year, month, or day")
	}
	if _, err := time.Parse(layout, value.Value); err != nil {
		return errors.New("date value does not match precision")
	}
	return nil
}
func normalizeContact(point *ContactPoint) error {
	point.Type = strings.ToLower(strings.TrimSpace(point.Type))
	point.Value = strings.TrimSpace(point.Value)
	switch point.Type {
	case "email":
		address, err := mail.ParseAddress(point.Value)
		if err != nil || !strings.Contains(address.Address, "@") {
			return errors.New("invalid email contact")
		}
		point.Normalized = strings.ToLower(address.Address)
	case "mobile", "phone", "whatsapp":
		normalized := nonPhone.ReplaceAllString(point.Value, "")
		if strings.HasPrefix(normalized, "0") {
			normalized = "+233" + strings.TrimPrefix(normalized, "0")
		}
		if !strings.HasPrefix(normalized, "+") || len(normalized) < 9 || len(normalized) > 16 {
			return errors.New("invalid phone contact")
		}
		point.Normalized = normalized
	default:
		return errors.New("unsupported contact type")
	}
	return nil
}
func onePrimaryContactPerType(points []ContactPoint) error {
	seen := map[string]bool{}
	for _, point := range points {
		if point.Primary && seen[point.Type] {
			return errors.New("only one primary contact is allowed per type")
		}
		if point.Primary {
			seen[point.Type] = true
		}
	}
	return nil
}
func normalizeTags(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" && !seen[value] && len(value) <= 50 {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}
