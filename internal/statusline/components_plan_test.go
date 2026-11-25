package statusline

import (
	"testing"
	"time"
)

func TestPlanComponents(t *testing.T) {
	now := time.Date(2026, 11, 25, 19, 39, 0, 0, time.UTC) // quarta
	sameDayReset := now.Add(3*time.Hour + 21*time.Minute).Unix()
	nextSundayReset := now.Add(3*24*time.Hour + 14*time.Hour).Unix()

	full := &Input{
		Effort:     &EffortInfo{Level: "medium"},
		RateLimits: &RateLimits{SevenDay: &RateLimitWindow{UsedPercentage: 48, ResetsAt: nextSundayReset}},
	}
	full.Cost.TotalDurationMS = (83 * time.Minute).Milliseconds()
	full.Context.TotalInputTokens = 227_000
	full.Context.ContextWindowSize = 1_000_000
	full.Context.UsedPercentage = 23

	fiveHour := &Input{RateLimits: &RateLimits{FiveHour: &RateLimitWindow{UsedPercentage: 55, ResetsAt: sameDayReset}}}
	expired := &Input{RateLimits: &RateLimits{SevenDay: &RateLimitWindow{UsedPercentage: 91, ResetsAt: now.Add(-time.Minute).Unix()}}}

	cases := []struct {
		name string
		comp string
		in   *Input
		opts ComponentOpts
		want string
	}{
		{"effort level", "effort", full, ComponentOpts{}, "esforço medium"},
		{"effort hidden without level", "effort", &Input{}, ComponentOpts{}, ""},
		{"session time", "session_time", full, ComponentOpts{}, "sessão 1h23m"},
		{"session time hidden at zero", "session_time", &Input{}, ComponentOpts{}, ""},
		{"context tokens", "context_tokens", full, ComponentOpts{}, "227.0k/1.0M"},
		{"context tokens hidden without window size", "context_tokens", &Input{}, ComponentOpts{}, ""},
		{"context pct label prefix", "context_pct", full, ComponentOpts{LabelPrefix: "ctx "}, "ctx ▓░░░░░ 23%"},
		{"weekly reset on another day shows weekday", "rate_7d", full, ComponentOpts{}, "semana ▓▓░░░░ 48% reseta dom 09:39 (em 3d14h)"},
		{"5h reset today omits weekday", "rate_5h", fiveHour, ComponentOpts{}, "5h ▓▓▓░░░ 55% reseta 23:00 (em 3h21m)"},
		{"past reset drops the suffix", "rate_7d", expired, ComponentOpts{}, "semana ▓▓▓▓▓░ 91%"},
		{"window the plan does not send is hidden", "rate_5h", full, ComponentOpts{}, ""},
		{"no rate limits at all", "rate_7d", &Input{}, ComponentOpts{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &RenderCtx{In: tc.in, Theme: GetTheme("graphite"), Now: now}
			if seg := Get(tc.comp).Render(ctx, tc.opts); seg.Text != tc.want {
				t.Fatalf("%s text = %q, want %q", tc.comp, seg.Text, tc.want)
			}
		})
	}
}

func TestPresetsReferenceKnownComponents(t *testing.T) {
	for _, name := range PresetNames {
		cfg := Presets[name]
		if cfg == nil {
			t.Fatalf("preset %q listed in PresetNames but missing", name)
		}
		for _, line := range cfg.Lines {
			for _, comp := range line.Components {
				if Get(comp) == nil || componentMetas[comp].Name == "" {
					t.Fatalf("preset %q references component %q without registry or meta", name, comp)
				}
			}
		}
	}
}

func TestPlanPresetKeepsContextThresholds(t *testing.T) {
	opts := Presets["plan"].Components["context_pct"]
	if opts.LabelPrefix != "ctx " || opts.WarnAt != 50 || opts.CriticalAt != 80 {
		t.Fatalf("context_pct opts = %+v", opts)
	}
}
