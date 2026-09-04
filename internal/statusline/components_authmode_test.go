package statusline

import (
	"testing"
	"time"
)

func TestAuthModeRendersGateway(t *testing.T) {
	theme := GetTheme("graphite")
	cases := []struct {
		name   string
		mode   string
		want   string
		wantFG Color
	}{
		{"gateway", "gateway", "[Gateway]", theme.SegOf("auth_mode").FG},
		{"oauth", "oauth", "[OAuth]", theme.SegOf("auth_mode").FG},
		{"api key", "api_key", "[API key]", theme.Status.Warn},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &RenderCtx{In: &Input{AuthMode: tc.mode}, Theme: theme, Now: time.Now()}
			seg := Get("auth_mode").Render(ctx, ComponentOpts{})
			if seg.Text != tc.want || seg.FG != tc.wantFG || !seg.Bold {
				t.Fatalf("got %+v, want text %q fg %+v bold", seg, tc.want, tc.wantFG)
			}
		})
	}
}
