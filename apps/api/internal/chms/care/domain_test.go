package care

import "testing"

func validDefinitionInput() DefinitionInput {
	return DefinitionInput{BranchID: "accra", Name: "First visit follow-up", Purpose: "visitor-assimilation", InitialStageKey: "new", Stages: []Stage{{Key: "new", Name: "New", SLAMinutes: 1440}, {Key: "contacted", Name: "Contacted", SLAMinutes: 2880}, {Key: "closed", Name: "Closed", Terminal: true, OutcomeRequired: true}}, Transitions: []Transition{{From: "new", To: "contacted", Name: "Record contact"}, {From: "contacted", To: "closed", Name: "Close"}}, Tasks: []TaskTemplate{{Key: "welcome-call", StageKey: "new", Title: "Make welcome call", DueMinutes: 720}}}
}
func TestDefinitionValidationRejectsUnsafeGraphs(t *testing.T) {
	valid := validDefinitionInput()
	if err := valid.NormalizeAndValidate(); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*DefinitionInput)
	}{{"unknown initial", func(v *DefinitionInput) { v.InitialStageKey = "missing" }}, {"duplicate stage", func(v *DefinitionInput) { v.Stages = append(v.Stages, v.Stages[0]) }}, {"unknown transition", func(v *DefinitionInput) { v.Transitions[0].To = "missing" }}, {"duplicate task", func(v *DefinitionInput) { v.Tasks = append(v.Tasks, v.Tasks[0]) }}, {"negative SLA", func(v *DefinitionInput) { v.Stages[0].SLAMinutes = -1 }}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := validDefinitionInput()
			tt.mutate(&v)
			if err := v.NormalizeAndValidate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
