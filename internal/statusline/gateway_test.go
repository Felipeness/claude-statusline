package statusline

import (
	"os"
	"testing"
	"time"
)

func TestParseGatewayUsage(t *testing.T) {
	fixture, err := os.ReadFile("testdata/gateway_usage.json")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		raw     string
		want    *GatewayUsage
		wantErr bool
	}{
		{
			name: "full fixture",
			raw:  string(fixture),
			want: &GatewayUsage{
				SpentBRLMicro: 73_530_000, LimitBRLMicro: 520_000_000, BaseLimitBRLMicro: 600_000_000,
				Tokens: 9_700_000, WindowEnd: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC).Unix(),
				Scope: "user", CalendarPeriod: "monthly",
			},
		},
		{name: "empty entries", raw: `{"entries":[]}`, want: nil},
		{name: "entry without budget", raw: `{"entries":[{"scope":"user"}]}`, want: nil},
		{
			name: "falls back to limit when effectiveLimit missing",
			raw:  `{"entries":[{"budget":{"limit":{"brlLimitMicro":100000000},"spend":{"offerBrlMicro":5000000}}}]}`,
			want: &GatewayUsage{SpentBRLMicro: 5_000_000, LimitBRLMicro: 100_000_000, BaseLimitBRLMicro: 100_000_000},
		},
		{
			name: "explicit zero effectiveLimit stays zero",
			raw:  `{"entries":[{"budget":{"limit":{"brlLimitMicro":100000000},"effectiveLimit":{"brlLimitMicro":0},"exceeded":true}}]}`,
			want: &GatewayUsage{LimitBRLMicro: 0, BaseLimitBRLMicro: 100_000_000, Exceeded: true},
		},
		{
			name: "date-only window end and scope inside budget",
			raw:  `{"entries":[{"budget":{"scope":"license","calendarPeriod":"weekly","window":{"end":"2026-09-08"},"exceeded":true}}]}`,
			want: &GatewayUsage{WindowEnd: time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC).Unix(), Exceeded: true, Scope: "license", CalendarPeriod: "weekly"},
		},
		{
			name: "float micro values are rounded",
			raw:  `{"entries":[{"budget":{"limit":{"brlLimitMicro":100.6},"spend":{"offerBrlMicro":2.4,"tokens":10.5}}}]}`,
			want: &GatewayUsage{SpentBRLMicro: 2, LimitBRLMicro: 101, BaseLimitBRLMicro: 101, Tokens: 11},
		},
		{name: "invalid json", raw: `{`, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseGatewayUsage([]byte(tc.raw))
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == nil {
				if got != nil {
					t.Fatalf("want nil, got %+v", got)
				}
				return
			}
			if got == nil || *got != *tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestGatewayUsagePct(t *testing.T) {
	cases := []struct {
		name string
		g    *GatewayUsage
		want float64
	}{
		{"nil receiver", nil, 0},
		{"zero limit", &GatewayUsage{SpentBRLMicro: 10}, 0},
		{"fourteen percent", &GatewayUsage{SpentBRLMicro: 73_530_000, LimitBRLMicro: 520_000_000}, 14.140384615384615},
		{"over limit", &GatewayUsage{SpentBRLMicro: 600, LimitBRLMicro: 500}, 120},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.g.Pct(); got != tc.want {
				t.Fatalf("Pct() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestGatewayUsageResetDate(t *testing.T) {
	g := &GatewayUsage{WindowEnd: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC).Unix()}
	if got := g.ResetDate(); got != "01/10" {
		t.Fatalf("ResetDate() = %q, want 01/10", got)
	}
	if got := (&GatewayUsage{}).ResetDate(); got != "" {
		t.Fatalf("ResetDate() on zero = %q, want empty", got)
	}
}
