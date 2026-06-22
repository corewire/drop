package discovery

import (
	"fmt"
	"strconv"
	"time"

	dropv1alpha1 "github.com/corewire/drop/api/v1alpha1"
)

// ScoreWeighter applies a time-based weight to data point values during aggregation.
type ScoreWeighter interface {
	// Weight returns the multiplier for a data point at the given timestamp.
	Weight(t time.Time) float64
}

// NewScoreWeighter builds a ScoreWeighter from the API scoring strategy config.
// Returns nil if strategy is nil (no weighting applied).
func NewScoreWeighter(strategy *dropv1alpha1.ScoringStrategy) (ScoreWeighter, error) {
	if strategy == nil {
		return nil, nil
	}
	switch strategy.Type {
	case dropv1alpha1.ScoringStrategyWorktime:
		if strategy.Worktime == nil {
			return nil, fmt.Errorf("worktime config is required when type=worktime")
		}
		return newWorktimeWeighter(strategy.Worktime)
	default:
		return nil, fmt.Errorf("unsupported scoring strategy type: %s", strategy.Type)
	}
}

// worktimeWeighter weights data points based on time-of-day windows.
type worktimeWeighter struct {
	windows []worktimeWindow
	loc     *time.Location
}

type worktimeWindow struct {
	startHour int
	endHour   int
	weight    float64
}

func newWorktimeWeighter(cfg *dropv1alpha1.WorktimeStrategy) (*worktimeWeighter, error) {
	tz := cfg.Timezone
	if tz == "" {
		tz = "UTC"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, fmt.Errorf("loading timezone %q: %w", tz, err)
	}

	windows := make([]worktimeWindow, 0, len(cfg.Windows))
	for _, w := range cfg.Windows {
		weight, err := strconv.ParseFloat(w.Weight, 64)
		if err != nil {
			return nil, fmt.Errorf("parsing weight %q: %w", w.Weight, err)
		}
		windows = append(windows, worktimeWindow{
			startHour: int(w.StartHour),
			endHour:   int(w.EndHour),
			weight:    weight,
		})
	}

	return &worktimeWeighter{
		windows: windows,
		loc:     loc,
	}, nil
}

// Weight returns the multiplier for the given timestamp based on configured windows.
// Returns 0 if the timestamp doesn't fall within any window.
func (w *worktimeWeighter) Weight(t time.Time) float64 {
	hour := t.In(w.loc).Hour()
	for _, win := range w.windows {
		if hour >= win.startHour && hour < win.endHour {
			return win.weight
		}
	}
	return 0
}
