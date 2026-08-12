package handlers

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type staffScope struct {
	BranchIDs           []string
	MinistryIDs         []string
	AssignedResourceIDs []string
	AccessVersion       int64
}

func scopeFromUser(user bson.M) staffScope {
	role := strings.TrimSpace(fmt.Sprint(user["role"]))
	if role == "super-admin" {
		return staffScope{BranchIDs: []string{"*"}, MinistryIDs: []string{"*"}, AccessVersion: int64FromBSON(user["accessVersion"])}
	}
	return staffScope{
		BranchIDs:           stringsFromBSON(user["branchIds"]),
		MinistryIDs:         stringsFromBSON(user["ministryIds"]),
		AssignedResourceIDs: stringsFromBSON(user["assignedResourceIds"]),
		AccessVersion:       int64FromBSON(user["accessVersion"]),
	}
}

func int64FromBSON(value any) int64 {
	switch number := value.(type) {
	case int64:
		return number
	case int32:
		return int64(number)
	case int:
		return int64(number)
	case float64:
		return int64(number)
	default:
		return 0
	}
}

func stringsFromBSON(value any) []string {
	items := []string{}
	switch values := value.(type) {
	case bson.A:
		for _, item := range values {
			items = appendValidScope(items, item)
		}
	case []string:
		for _, item := range values {
			items = appendValidScope(items, item)
		}
	case []any:
		for _, item := range values {
			items = appendValidScope(items, item)
		}
	}
	return items
}

func appendValidScope(items []string, raw any) []string {
	value := strings.TrimSpace(fmt.Sprint(raw))
	if value == "" || value == "<nil>" {
		return items
	}
	for _, existing := range items {
		if existing == value {
			return items
		}
	}
	return append(items, value)
}
