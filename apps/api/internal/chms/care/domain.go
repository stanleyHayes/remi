package care

import (
	"errors"
	"strings"
	"time"

	"remi-api/internal/chms/platform"
)

const CurrentSchemaVersion = 1

type Stage struct {
	Key             string `json:"key" bson:"key"`
	Name            string `json:"name" bson:"name"`
	SLAMinutes      int    `json:"slaMinutes" bson:"slaMinutes"`
	Terminal        bool   `json:"terminal" bson:"terminal"`
	OutcomeRequired bool   `json:"outcomeRequired" bson:"outcomeRequired"`
}
type Transition struct {
	From string `json:"from" bson:"from"`
	To   string `json:"to" bson:"to"`
	Name string `json:"name" bson:"name"`
}
type TaskTemplate struct {
	Key        string `json:"key" bson:"key"`
	StageKey   string `json:"stageKey" bson:"stageKey"`
	Title      string `json:"title" bson:"title"`
	DueMinutes int    `json:"dueMinutes" bson:"dueMinutes"`
}
type Definition struct {
	platform.ResourceEnvelope `bson:",inline"`
	Name                      string         `json:"name" bson:"name"`
	Description               string         `json:"description,omitempty" bson:"description,omitempty"`
	Purpose                   string         `json:"purpose" bson:"purpose"`
	DefinitionVersion         int            `json:"definitionVersion" bson:"definitionVersion"`
	SupersedesDefinitionID    platform.ID    `json:"supersedesDefinitionId,omitempty" bson:"supersedesDefinitionId,omitempty"`
	Status                    string         `json:"status" bson:"status"`
	InitialStageKey           string         `json:"initialStageKey" bson:"initialStageKey"`
	Stages                    []Stage        `json:"stages" bson:"stages"`
	Transitions               []Transition   `json:"transitions" bson:"transitions"`
	Tasks                     []TaskTemplate `json:"tasks" bson:"tasks"`
	PublishedAt               *time.Time     `json:"publishedAt,omitempty" bson:"publishedAt,omitempty"`
}
type DefinitionInput struct {
	BranchID               platform.ID    `json:"branchId"`
	Name                   string         `json:"name"`
	Description            string         `json:"description"`
	Purpose                string         `json:"purpose"`
	InitialStageKey        string         `json:"initialStageKey"`
	Stages                 []Stage        `json:"stages"`
	Transitions            []Transition   `json:"transitions"`
	Tasks                  []TaskTemplate `json:"tasks"`
	SupersedesDefinitionID platform.ID    `json:"supersedesDefinitionId"`
}

func (in *DefinitionInput) NormalizeAndValidate() error {
	in.Name, in.Description, in.Purpose, in.InitialStageKey = strings.TrimSpace(in.Name), strings.TrimSpace(in.Description), strings.TrimSpace(in.Purpose), strings.ToLower(strings.TrimSpace(in.InitialStageKey))
	if !in.BranchID.Valid() || in.Name == "" || len(in.Name) > 150 || in.Purpose == "" || len(in.Purpose) > 100 || len(in.Stages) < 1 || len(in.Stages) > 30 {
		return errors.New("branch, name, purpose and 1 to 30 stages are required")
	}
	stageKeys := map[string]bool{}
	for i := range in.Stages {
		in.Stages[i].Key, in.Stages[i].Name = strings.ToLower(strings.TrimSpace(in.Stages[i].Key)), strings.TrimSpace(in.Stages[i].Name)
		if in.Stages[i].Key == "" || in.Stages[i].Name == "" || stageKeys[in.Stages[i].Key] || in.Stages[i].SLAMinutes < 0 || in.Stages[i].SLAMinutes > 525600 {
			return errors.New("stages require unique keys, names and valid SLA minutes")
		}
		stageKeys[in.Stages[i].Key] = true
	}
	if !stageKeys[in.InitialStageKey] {
		return errors.New("initial stage must reference a stage")
	}
	seenTransitions := map[string]bool{}
	for i := range in.Transitions {
		in.Transitions[i].From, in.Transitions[i].To, in.Transitions[i].Name = strings.ToLower(strings.TrimSpace(in.Transitions[i].From)), strings.ToLower(strings.TrimSpace(in.Transitions[i].To)), strings.TrimSpace(in.Transitions[i].Name)
		key := in.Transitions[i].From + ">" + in.Transitions[i].To
		if !stageKeys[in.Transitions[i].From] || !stageKeys[in.Transitions[i].To] || in.Transitions[i].From == in.Transitions[i].To || in.Transitions[i].Name == "" || seenTransitions[key] {
			return errors.New("transitions must connect different known stages exactly once")
		}
		seenTransitions[key] = true
	}
	seenTasks := map[string]bool{}
	for i := range in.Tasks {
		in.Tasks[i].Key, in.Tasks[i].StageKey, in.Tasks[i].Title = strings.ToLower(strings.TrimSpace(in.Tasks[i].Key)), strings.ToLower(strings.TrimSpace(in.Tasks[i].StageKey)), strings.TrimSpace(in.Tasks[i].Title)
		if in.Tasks[i].Key == "" || in.Tasks[i].Title == "" || !stageKeys[in.Tasks[i].StageKey] || seenTasks[in.Tasks[i].Key] || in.Tasks[i].DueMinutes < 0 {
			return errors.New("task templates require unique keys, titles, known stages and valid due minutes")
		}
		seenTasks[in.Tasks[i].Key] = true
	}
	return nil
}

type Instance struct {
	platform.ResourceEnvelope `bson:",inline"`
	DefinitionID              platform.ID `json:"definitionId" bson:"definitionId"`
	DefinitionVersion         int         `json:"definitionVersion" bson:"definitionVersion"`
	SubjectType               string      `json:"subjectType" bson:"subjectType"`
	SubjectID                 platform.ID `json:"subjectId" bson:"subjectId"`
	StageKey                  string      `json:"stageKey" bson:"stageKey"`
	OwnerID                   platform.ID `json:"ownerId" bson:"ownerId"`
	DueAt                     *time.Time  `json:"dueAt,omitempty" bson:"dueAt,omitempty"`
	State                     string      `json:"state" bson:"state"`
	OutcomeCode               string      `json:"outcomeCode,omitempty" bson:"outcomeCode,omitempty"`
	OutcomeSummary            string      `json:"outcomeSummary,omitempty" bson:"outcomeSummary,omitempty"`
	CompletedAt               *time.Time  `json:"completedAt,omitempty" bson:"completedAt,omitempty"`
}
type StartInput struct {
	DefinitionID platform.ID `json:"definitionId"`
	SubjectType  string      `json:"subjectType"`
	SubjectID    platform.ID `json:"subjectId"`
	OwnerID      platform.ID `json:"ownerId"`
}
type TransitionInput struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	ToStageKey      string `json:"toStageKey"`
	OutcomeCode     string `json:"outcomeCode"`
	OutcomeSummary  string `json:"outcomeSummary"`
	AutomationKey   string `json:"automationKey"`
}
type Task struct {
	platform.ResourceEnvelope `bson:",inline"`
	InstanceID                platform.ID `json:"instanceId" bson:"instanceId"`
	TemplateKey               string      `json:"templateKey" bson:"templateKey"`
	StageKey                  string      `json:"stageKey" bson:"stageKey"`
	Title                     string      `json:"title" bson:"title"`
	AssigneeID                platform.ID `json:"assigneeId" bson:"assigneeId"`
	DueAt                     *time.Time  `json:"dueAt,omitempty" bson:"dueAt,omitempty"`
	Status                    string      `json:"status" bson:"status"`
	Outcome                   string      `json:"outcome,omitempty" bson:"outcome,omitempty"`
	CompletedAt               *time.Time  `json:"completedAt,omitempty" bson:"completedAt,omitempty"`
	ReminderCount             int         `json:"reminderCount" bson:"reminderCount"`
	LastReminderAt            *time.Time  `json:"lastReminderAt,omitempty" bson:"lastReminderAt,omitempty"`
}
type Event struct {
	ID             platform.ID    `json:"id" bson:"_id"`
	OrganizationID platform.ID    `json:"organizationId" bson:"organizationId"`
	InstanceID     platform.ID    `json:"instanceId" bson:"instanceId"`
	Type           string         `json:"type" bson:"type"`
	FromStage      string         `json:"fromStage,omitempty" bson:"fromStage,omitempty"`
	ToStage        string         `json:"toStage,omitempty" bson:"toStage,omitempty"`
	Actor          platform.Actor `json:"actor" bson:"actor"`
	Reason         string         `json:"reason,omitempty" bson:"reason,omitempty"`
	OccurredAt     time.Time      `json:"occurredAt" bson:"occurredAt"`
}
