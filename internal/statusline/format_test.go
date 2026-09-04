package statusline

import "testing"

func TestFormatBRLMicro(t *testing.T) {
	cases := []struct {
		name  string
		micro int64
		want  string
	}{
		{"zero", 0, "R$ 0,00"},
		{"cents", 73_530_000, "R$ 73,53"},
		{"thousands separator", 1_520_000_000, "R$ 1.520,00"},
		{"millions", 1_234_567_890_000, "R$ 1.234.567,89"},
		{"rounds half up", 999_999, "R$ 1,00"},
		{"rounds down", 4_999, "R$ 0,00"},
		{"negative", -73_530_000, "-R$ 73,53"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatBRLMicro(tc.micro); got != tc.want {
				t.Fatalf("FormatBRLMicro(%d) = %q, want %q", tc.micro, got, tc.want)
			}
		})
	}
}

func TestFormatTokens(t *testing.T) {
	cases := []struct {
		name string
		n    int64
		want string
	}{
		{"units", 999, "999"},
		{"exact thousand", 1_000, "1.0k"},
		{"thousands", 38_630, "38.6k"},
		{"millions", 9_700_000, "9.7M"},
		{"zero", 0, "0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatTokens(tc.n); got != tc.want {
				t.Fatalf("FormatTokens(%d) = %q, want %q", tc.n, got, tc.want)
			}
		})
	}
}
