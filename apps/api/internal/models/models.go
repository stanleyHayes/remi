package models

import (
	"regexp"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Content statuses used across content collections.
const (
	StatusAIDraft   = "ai-draft"
	StatusInReview  = "in-review"
	StatusApproved  = "approved"
	StatusPublished = "published"
)

// PublicStatuses is what unauthenticated endpoints are allowed to return.
var PublicStatuses = []string{StatusPublished, StatusApproved}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify converts a display string to a URL-safe slug.
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// Now returns the current UTC time.
func Now() time.Time { return time.Now().UTC() }

// Normalize converts a Mongo document into the JSON shape the API contract
// expects: _id becomes id (hex string) and dates become RFC3339 strings.
func Normalize(doc bson.M) bson.M {
	out := bson.M{}
	for k, v := range doc {
		switch k {
		case "_id":
			if oid, ok := v.(bson.ObjectID); ok {
				out["id"] = oid.Hex()
			} else {
				out["id"] = v
			}
		case "passwordHash", "token", "mfaSecret", "mfaPendingSecret", "mfaRecoveryCodes":
			// never leak
			continue
		default:
			out[k] = normalizeValue(v)
		}
	}
	return out
}

func NormalizeAll(docs []bson.M) []bson.M {
	out := make([]bson.M, 0, len(docs))
	for _, d := range docs {
		out = append(out, Normalize(d))
	}
	return out
}

func normalizeValue(v any) any {
	switch t := v.(type) {
	case bson.DateTime:
		return t.Time().UTC().Format(time.RFC3339)
	case time.Time:
		return t.UTC().Format(time.RFC3339)
	case bson.ObjectID:
		return t.Hex()
	case bson.M:
		return Normalize(t)
	case bson.A:
		arr := make([]any, len(t))
		for i, e := range t {
			arr[i] = normalizeValue(e)
		}
		return arr
	default:
		return v
	}
}
