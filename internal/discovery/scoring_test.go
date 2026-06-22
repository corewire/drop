package discovery

import (
	"testing"
	"time"

	dropv1alpha1 "github.com/corewire/drop/api/v1alpha1"
)

func TestWorktimeWeighter_Weight(t *testing.T) {
	strategy := &dropv1alpha1.ScoringStrategy{
		Type: dropv1alpha1.ScoringStrategyWorktime,
		Worktime: &dropv1alpha1.WorktimeStrategy{
			Timezone: "UTC",
			Windows: []dropv1alpha1.WorktimeWindow{
				{StartHour: 9, EndHour: 17, Weight: "1.0"},
				{StartHour: 6, EndHour: 9, Weight: "0.3"},
				{StartHour: 17, EndHour: 19, Weight: "0.3"},
			},
		},
	}

	weighter, err := NewScoreWeighter(strategy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		name string
		hour int
		want float64
	}{
		{"peak hours 9am", 9, 1.0},
		{"peak hours 12pm", 12, 1.0},
		{"peak hours 16pm", 16, 1.0},
		{"early morning 6am", 6, 0.3},
		{"early morning 8am", 8, 0.3},
		{"evening 17pm", 17, 0.3},
		{"evening 18pm", 18, 0.3},
		{"night 0am", 0, 0.0},
		{"night 3am", 3, 0.0},
		{"night 5am", 5, 0.0},
		{"late night 20pm", 20, 0.0},
		{"late night 23pm", 23, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := time.Date(2025, 1, 15, tt.hour, 30, 0, 0, time.UTC)
			got := weighter.Weight(ts)
			if got != tt.want {
				t.Errorf("Weight at hour %d = %f, want %f", tt.hour, got, tt.want)
			}
		})
	}
}

func TestWorktimeWeighter_Timezone(t *testing.T) {
	strategy := &dropv1alpha1.ScoringStrategy{
		Type: dropv1alpha1.ScoringStrategyWorktime,
		Worktime: &dropv1alpha1.WorktimeStrategy{
			Timezone: "Europe/Berlin",
			Windows: []dropv1alpha1.WorktimeWindow{
				{StartHour: 9, EndHour: 17, Weight: "1.0"},
			},
		},
	}

	weighter, err := NewScoreWeighter(strategy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 10:00 UTC = 11:00 Berlin (CET, winter) — should be in window
	ts := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	if got := weighter.Weight(ts); got != 1.0 {
		t.Errorf("Weight at 10:00 UTC (11:00 Berlin) = %f, want 1.0", got)
	}

	// 07:00 UTC = 08:00 Berlin (CET, winter) — should be outside window
	ts = time.Date(2025, 1, 15, 7, 0, 0, 0, time.UTC)
	if got := weighter.Weight(ts); got != 0.0 {
		t.Errorf("Weight at 07:00 UTC (08:00 Berlin) = %f, want 0.0", got)
	}
}

func TestNewScoreWeighter_Nil(t *testing.T) {
	weighter, err := NewScoreWeighter(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if weighter != nil {
		t.Error("expected nil weighter for nil strategy")
	}
}

func TestAggregateRangeValues_WithWorktime(t *testing.T) {
	strategy := &dropv1alpha1.ScoringStrategy{
		Type: dropv1alpha1.ScoringStrategyWorktime,
		Worktime: &dropv1alpha1.WorktimeStrategy{
			Timezone: "UTC",
			Windows: []dropv1alpha1.WorktimeWindow{
				{StartHour: 9, EndHour: 17, Weight: "1.0"},
				{StartHour: 0, EndHour: 9, Weight: "0.0"},
			},
		},
	}

	weighter, err := NewScoreWeighter(strategy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Timestamps: 10:00 (weight 1.0), 03:00 (weight 0.0), 12:00 (weight 1.0)
	values := [][]interface{}{
		{float64(time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC).Unix()), "100"},
		{float64(time.Date(2025, 1, 15, 3, 0, 0, 0, time.UTC).Unix()), "200"},
		{float64(time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC).Unix()), "50"},
	}

	method := dropv1alpha1.AggregationSum
	// Expected: 100*1.0 + 200*0.0 + 50*1.0 = 150
	got := aggregateRangeValues(values, &method, weighter)
	if got != 150 {
		t.Errorf("aggregateRangeValues with worktime weighting = %d, want 150", got)
	}

	// Without weighter: 100 + 200 + 50 = 350
	got = aggregateRangeValues(values, &method, nil)
	if got != 350 {
		t.Errorf("aggregateRangeValues without weighting = %d, want 350", got)
	}
}
