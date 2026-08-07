package api

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestDisruptionBudgetModelRejectsEmptyReasons(t *testing.T) {
	emptyReasons := []types.String{}
	model := DisruptionBudgetModel{
		Nodes:   types.StringValue("10%"),
		Reasons: &emptyReasons,
	}

	if _, err := model.values(context.Background()); err == nil || !strings.Contains(err.Error(), "at least one") {
		t.Fatalf("values() error = %v, want empty reasons error", err)
	}
}

func TestDisruptionBudgetModelRejectsInvalidSchedule(t *testing.T) {
	for _, schedule := range []string{"daily", "@every 1h", "TZ=UTC 0 2 * * *"} {
		model := DisruptionBudgetModel{
			Nodes:    types.StringValue("10%"),
			Schedule: types.StringValue(schedule),
			Duration: types.StringValue("1h"),
		}

		if _, err := model.values(context.Background()); err == nil || !strings.Contains(err.Error(), "invalid schedule") {
			t.Fatalf("values() schedule %q error = %v, want invalid schedule error", schedule, err)
		}
	}
}

func TestDisruptionBudgetModelAcceptsSupportedSchedules(t *testing.T) {
	for _, schedule := range []string{"@daily", "0 2 * * *"} {
		model := DisruptionBudgetModel{
			Nodes:    types.StringValue("10%"),
			Schedule: types.StringValue(schedule),
			Duration: types.StringValue("1h"),
		}
		if _, err := model.values(context.Background()); err != nil {
			t.Fatalf("values() schedule %q error = %v", schedule, err)
		}
	}
}

func TestDisruptionBudgetModelRejectsPaddedManagedValues(t *testing.T) {
	tests := []struct {
		name     string
		nodes    string
		schedule string
		duration string
		want     string
	}{
		{name: "nodes", nodes: " 10% ", schedule: "@daily", duration: "1h", want: "nodes"},
		{name: "schedule", nodes: "10%", schedule: " @daily ", duration: "1h", want: "schedule"},
		{name: "duration", nodes: "10%", schedule: "@daily", duration: " 1h ", want: "duration"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := DisruptionBudgetModel{
				Nodes:    types.StringValue(test.nodes),
				Schedule: types.StringValue(test.schedule),
				Duration: types.StringValue(test.duration),
			}
			if _, err := model.values(context.Background()); err == nil || !strings.Contains(err.Error(), test.want+" cannot contain") {
				t.Fatalf("values() error = %v, want padded %s error", err, test.want)
			}
		})
	}
}
