package statusline

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// oauthUsageEndpoint é o endpoint que o Claude Code usa pra mostrar
// "you've used X% of your 5h block". Requer Bearer token + anthropic-beta.
const oauthUsageEndpoint = "https://api.anthropic.com/api/oauth/usage"

// OAuthProbeConfig liga o probe automatico do endpoint de usage.
// Util pra preencher rate_5h/rate_7d quando o stdin do Claude Code vem
// com rate_limits=null (acontece quando a sessao roda em API key mode).
type OAuthProbeConfig struct {
	Enabled   bool    `toml:"enabled" json:"enabled"`
	TTL       string  `toml:"ttl" json:"ttl"`             // ex: "30s"
	Threshold float64 `toml:"threshold" json:"threshold"` // % usada pelo auth_mode (default 90)
	Timeout   string  `toml:"timeout" json:"timeout"`     // ex: "3s"
	UserAgent string  `toml:"user_agent" json:"user_agent"`
}

func (c OAuthProbeConfig) ttlDuration() time.Duration {
	return parseProbeDur(c.TTL, 30*time.Second)
}

func (c OAuthProbeConfig) timeoutDuration() time.Duration {
	return parseProbeDur(c.Timeout, 3*time.Second)
}

func (c OAuthProbeConfig) thresholdValue() float64 {
	if c.Threshold <= 0 {
		return 90
	}
	return c.Threshold
}

func parseProbeDur(s string, dflt time.Duration) time.Duration {
	if s == "" {
		return dflt
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return dflt
	}
	return d
}

// oauthUsageResponse espelha o payload de /api/oauth/usage.
type oauthUsageResponse struct {
	FiveHour *struct {
		Utilization float64 `json:"utilization"`
		ResetsAt    string  `json:"resets_at"` // ISO 8601 com microssegundos
	} `json:"five_hour"`
	SevenDay *struct {
		Utilization float64 `json:"utilization"`
		ResetsAt    string  `json:"resets_at"`
	} `json:"seven_day"`
}

// ProbeResult e o que o probe devolve.
type ProbeResult struct {
	FiveHour  *RateLimitWindow
	SevenDay  *RateLimitWindow
	FetchedAt time.Time
}

type probeCache struct {
	mu   sync.Mutex
	last *ProbeResult
}

var defaultProbeCache = &probeCache{}

// ProbeOAuth bate em /api/oauth/usage com cache em memoria. Devolve nil
// (sem erro) quando nao ha credenciais OAuth ou o probe falha — fail-open
// pra nao quebrar o render.
func ProbeOAuth(cfg OAuthProbeConfig) *ProbeResult {
	if !cfg.Enabled {
		return nil
	}
	defaultProbeCache.mu.Lock()
	defer defaultProbeCache.mu.Unlock()

	if defaultProbeCache.last != nil &&
		time.Since(defaultProbeCache.last.FetchedAt) < cfg.ttlDuration() {
		return defaultProbeCache.last
	}

	token, err := readOAuthToken()
	if err != nil || token == "" {
		return nil
	}

	resp, err := fetchOAuthUsage(token, cfg)
	if err != nil {
		return nil
	}

	result := buildProbeResult(resp)
	defaultProbeCache.last = result
	return result
}

func readOAuthToken() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(home, ".claude", ".credentials.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var wrapper struct {
		ClaudeAiOauth struct {
			AccessToken string `json:"accessToken"`
		} `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return "", err
	}
	return wrapper.ClaudeAiOauth.AccessToken, nil
}

func fetchOAuthUsage(token string, cfg OAuthProbeConfig) (*oauthUsageResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeoutDuration())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, oauthUsageEndpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	ua := cfg.UserAgent
	if ua == "" {
		ua = "claude-statusline/native-probe"
	}
	req.Header.Set("User-Agent", ua)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("usage endpoint: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var out oauthUsageResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func buildProbeResult(r *oauthUsageResponse) *ProbeResult {
	out := &ProbeResult{FetchedAt: time.Now()}
	if r.FiveHour != nil {
		out.FiveHour = &RateLimitWindow{
			UsedPercentage: r.FiveHour.Utilization,
			ResetsAt:       parseIsoEpoch(r.FiveHour.ResetsAt),
		}
	}
	if r.SevenDay != nil {
		out.SevenDay = &RateLimitWindow{
			UsedPercentage: r.SevenDay.Utilization,
			ResetsAt:       parseIsoEpoch(r.SevenDay.ResetsAt),
		}
	}
	return out
}

// parseIsoEpoch tenta varios formatos ISO 8601 (com/sem microssegundos,
// com/sem timezone explicito). Devolve 0 quando nao consegue parsear.
func parseIsoEpoch(s string) int64 {
	if s == "" {
		return 0
	}
	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.999999-07:00",
		"2006-01-02T15:04:05.999999Z07:00",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t.Unix()
		}
	}
	return 0
}

// MergeProbeIntoInput sobrepoe rate_limits do probe quando o stdin nao
// trouxe esses dados. Em modo OAuth, o Claude Code envia rate_limits no
// stdin (sobrepoe nao acontece). Em modo API key, vem null (probe enche).
func MergeProbeIntoInput(in *Input, p *ProbeResult) {
	if p == nil || in == nil {
		return
	}
	if in.RateLimits == nil {
		in.RateLimits = &RateLimits{}
	}
	if in.RateLimits.FiveHour == nil && p.FiveHour != nil {
		in.RateLimits.FiveHour = p.FiveHour
	}
	if in.RateLimits.SevenDay == nil && p.SevenDay != nil {
		in.RateLimits.SevenDay = p.SevenDay
	}
}
