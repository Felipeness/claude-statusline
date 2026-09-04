package statusline

import (
	"strings"
	"testing"
	"time"
)

func tokensCtx(in *Input) *RenderCtx {
	return &RenderCtx{In: in, Theme: GetTheme("graphite"), Now: time.Now()}
}

func TestSessionTokenComponents(t *testing.T) {
	withCurrent := &Input{}
	withCurrent.Context.Current.InputTokens = 3
	withCurrent.Context.Current.OutputTokens = 436
	withCurrent.Context.Current.CacheReadInputTokens = 38_630
	withCurrent.Context.TotalInputTokens = 99_999

	totalsOnly := &Input{}
	totalsOnly.Context.TotalInputTokens = 18_432
	totalsOnly.Context.TotalOutputTokens = 4_521

	cases := []struct {
		name string
		comp string
		in   *Input
		want string
	}{
		{"in from current usage", "tokens_in", withCurrent, "In: 3"},
		{"out from current usage", "tokens_out", withCurrent, "Out: 436"},
		{"total sums in and out", "tokens_total", withCurrent, "Total: 439"},
		{"cache formatted", "tokens_cache", withCurrent, "Cache: 38.6k"},
		{"in falls back to totals", "tokens_in", totalsOnly, "In: 18.4k"},
		{"out falls back to totals", "tokens_out", totalsOnly, "Out: 4.5k"},
		{"total from totals", "tokens_total", totalsOnly, "Total: 23.0k"},
		{"cache hidden at zero", "tokens_cache", totalsOnly, ""},
		{"in visible at zero", "tokens_in", &Input{}, "In: 0"},
		{"total visible at zero", "tokens_total", &Input{}, "Total: 0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seg := Get(tc.comp).Render(tokensCtx(tc.in), ComponentOpts{})
			if strings.TrimSpace(seg.Text) != tc.want {
				t.Fatalf("%s text = %q, want %q", tc.comp, seg.Text, tc.want)
			}
		})
	}
}

func TestSessionTokenMetas(t *testing.T) {
	for _, name := range []string{"tokens_in", "tokens_out", "tokens_total", "tokens_cache"} {
		meta, ok := componentMetas[name]
		if !ok || meta.Category != "context" {
			t.Fatalf("meta %s = %+v (ok=%v)", name, meta, ok)
		}
	}
}
