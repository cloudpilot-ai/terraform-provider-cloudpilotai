package api

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/robfig/cron/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DisruptionBudgetModel struct {
	Nodes    types.String    `tfsdk:"nodes"`
	Reasons  *[]types.String `tfsdk:"reasons"`
	Schedule types.String    `tfsdk:"schedule"`
	Duration types.String    `tfsdk:"duration"`
}

type disruptionBudgetValues struct {
	nodes    string
	reasons  []string
	schedule *string
	duration *metav1.Duration
}

const maxDisruptionBudgets = 50

var (
	disruptionBudgetNodesPattern         = regexp.MustCompile(`^((100|[0-9]{1,2})%|[0-9]+)$`)
	disruptionBudgetScheduleMacroPattern = regexp.MustCompile(`^@(annually|yearly|monthly|weekly|daily|midnight|hourly)$`)
)

func validateDisruptionBudgetCount(count int) error {
	if count == 0 {
		return fmt.Errorf("node_disruption_budgets must contain at least one budget")
	}
	if count > maxDisruptionBudgets {
		return fmt.Errorf("node_disruption_budgets must contain at most %d budgets", maxDisruptionBudgets)
	}
	return nil
}

func (m DisruptionBudgetModel) values(_ context.Context) (disruptionBudgetValues, error) {
	nodes := m.Nodes.ValueString()
	if nodes != strings.TrimSpace(nodes) {
		return disruptionBudgetValues{}, fmt.Errorf("nodes cannot contain leading or trailing whitespace")
	}
	values := disruptionBudgetValues{nodes: nodes}
	if values.nodes == "" {
		return values, fmt.Errorf("nodes is required")
	}
	if !disruptionBudgetNodesPattern.MatchString(values.nodes) {
		return values, fmt.Errorf("nodes must be a non-negative integer or percentage from 0%% to 100%%")
	}

	if m.Reasons != nil {
		if len(*m.Reasons) == 0 {
			return values, fmt.Errorf("reasons must contain at least one value when configured")
		}
		values.reasons = make([]string, 0, len(*m.Reasons))
		for _, reason := range *m.Reasons {
			if reason.IsNull() || reason.IsUnknown() {
				return values, fmt.Errorf("reasons cannot contain null or unknown values")
			}
			reasonValue := reason.ValueString()
			switch reasonValue {
			case "Empty", "Underutilized", "Drifted":
				values.reasons = append(values.reasons, reasonValue)
			default:
				return values, fmt.Errorf("unsupported reason %q", reasonValue)
			}
		}
	}

	hasSchedule := !m.Schedule.IsNull() && !m.Schedule.IsUnknown()
	hasDuration := !m.Duration.IsNull() && !m.Duration.IsUnknown()
	if hasSchedule != hasDuration {
		return values, fmt.Errorf("schedule and duration must be configured together")
	}
	if hasSchedule {
		schedule := m.Schedule.ValueString()
		if schedule != strings.TrimSpace(schedule) {
			return values, fmt.Errorf("schedule cannot contain leading or trailing whitespace")
		}
		if schedule == "" {
			return values, fmt.Errorf("schedule cannot be empty")
		}
		if strings.HasPrefix(schedule, "@") {
			if !disruptionBudgetScheduleMacroPattern.MatchString(schedule) {
				return values, fmt.Errorf("invalid schedule: unsupported macro %q", schedule)
			}
		} else if len(strings.Fields(schedule)) != 5 {
			return values, fmt.Errorf("invalid schedule: expected exactly 5 fields")
		}
		if _, err := cron.ParseStandard(schedule); err != nil {
			return values, fmt.Errorf("invalid schedule: %w", err)
		}
		durationValue := m.Duration.ValueString()
		if durationValue != strings.TrimSpace(durationValue) {
			return values, fmt.Errorf("duration cannot contain leading or trailing whitespace")
		}
		duration, err := time.ParseDuration(durationValue)
		if err != nil {
			return values, fmt.Errorf("invalid duration: %w", err)
		}
		if duration <= 0 || duration%time.Minute != 0 {
			return values, fmt.Errorf("duration must be a positive whole number of minutes or hours")
		}
		values.schedule = &schedule
		values.duration = &metav1.Duration{Duration: duration}
	}

	return values, nil
}

func newDisruptionBudgetModel(nodes string, reasons []string, schedule *string, duration *metav1.Duration) DisruptionBudgetModel {
	model := DisruptionBudgetModel{
		Nodes:    types.StringValue(nodes),
		Schedule: types.StringNull(),
		Duration: types.StringNull(),
	}
	if len(reasons) > 0 {
		terraformReasons := make([]types.String, 0, len(reasons))
		for _, reason := range reasons {
			terraformReasons = append(terraformReasons, types.StringValue(reason))
		}
		model.Reasons = &terraformReasons
	}
	if schedule != nil {
		model.Schedule = types.StringValue(*schedule)
	}
	if duration != nil {
		model.Duration = types.StringValue(NormalizeDuration(duration.Duration.String()))
	}
	return model
}
