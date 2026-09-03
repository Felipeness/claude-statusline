package statusline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Version é injetada pelo main (ldflags). Vai no User-Agent dos probes.
var Version = "dev"

// GatewayConfig controla o probe do LLM Gateway da empresa (Kong com budget
// por usuário). Habilitado por padrão; só age quando há base URL (config ou
// env ANTHROPIC_BASE_URL) e o cache de token do Auth0 existe.
type GatewayConfig struct {
	Enabled   *bool  `toml:"enabled" json:"enabled,omitempty"`
	BaseURL   string `toml:"base_url" json:"base_url"`     // vazio = env ANTHROPIC_BASE_URL
	TokenFile string `toml:"token_file" json:"token_file"` // vazio = ~/.claude/auth0-token-cache.json
	CacheFile string `toml:"cache_file" json:"cache_file"` // vazio = ~/.cache/claude-statusline-gateway.json
	TTL       string `toml:"ttl" json:"ttl"`               // default 60s
	StaleTTL  string `toml:"stale_ttl" json:"stale_ttl"`   // default 1h
	Timeout   string `toml:"timeout" json:"timeout"`       // default 4s
}

func (c GatewayConfig) IsEnabled() bool { return c.Enabled == nil || *c.Enabled }

func (c GatewayConfig) ResolvedBaseURL() string {
	base := c.BaseURL
	if base == "" {
		base = os.Getenv("ANTHROPIC_BASE_URL")
	}
	return strings.TrimRight(base, "/")
}

func (c GatewayConfig) ResolvedTokenFile() string {
	if c.TokenFile != "" {
		return c.TokenFile
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "auth0-token-cache.json")
}

func (c GatewayConfig) resolvedCacheFile() string {
	if c.CacheFile != "" {
		return c.CacheFile
	}
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".cache")
	_ = os.MkdirAll(dir, 0700)
	return filepath.Join(dir, "claude-statusline-gateway.json")
}

func (c GatewayConfig) ttlDuration() time.Duration   { return parseProbeDur(c.TTL, 60*time.Second) }
func (c GatewayConfig) staleDuration() time.Duration { return parseProbeDur(c.StaleTTL, time.Hour) }
func (c GatewayConfig) timeoutDuration() time.Duration {
	return parseProbeDur(c.Timeout, 4*time.Second)
}

// GatewayConfigured diz se a sessão parece rodar via gateway: probe ligado,
// base URL conhecida e cache de token do Auth0 presente em disco.
func GatewayConfigured(cfg GatewayConfig) bool {
	if !cfg.IsEnabled() || cfg.ResolvedBaseURL() == "" {
		return false
	}
	_, err := os.Stat(cfg.ResolvedTokenFile())
	return err == nil
}

// Erros tipados pro subcomando budget explicar o que falta. O render usa
// ProbeGateway, que engole tudo (fail-open).
var (
	ErrGatewayDisabled      = errors.New("probe do gateway desligado no config")
	ErrGatewayNotConfigured = errors.New("gateway não configurado: ANTHROPIC_BASE_URL ausente")
	ErrGatewayTokenMissing  = errors.New("token do Auth0 ausente: abra o Claude Code pra fazer login no gateway")
	ErrGatewayTokenExpired  = errors.New("token do Auth0 expirado: abra o Claude Code pra renovar")
	ErrGatewayUnreachable   = errors.New("gateway inacessível")
	ErrGatewayNoBudget      = errors.New("gateway respondeu sem budget pro usuário")
)

// GatewayProbeResult é o que o probe devolve: dados parseados, resposta crua
// (pro budget --json) e se veio de cache stale.
type GatewayProbeResult struct {
	Usage     *GatewayUsage
	Raw       json.RawMessage
	FetchedAt time.Time
	Stale     bool
}

// gatewayDiskCache é o único cache (cada render é um processo novo). Nunca
// guarda o token, só dados de consumo.
type gatewayDiskCache struct {
	FetchedAt int64           `json:"fetched_at"`
	Usage     *GatewayUsage   `json:"usage"`
	Raw       json.RawMessage `json:"raw,omitempty"`
}

// ProbeGateway é a face fail-open: nil em qualquer erro, pro render.
func ProbeGateway(cfg GatewayConfig) *GatewayProbeResult {
	res, err := ProbeGatewayDetailed(cfg)
	if err != nil {
		return nil
	}
	return res
}

// ProbeGatewayDetailed busca /v1/usage com cache em disco:
//  1. cache mais novo que TTL → devolve sem HTTP
//  2. token ausente/expirado → cache stale (< StaleTTL) ou erro tipado
//  3. HTTP falha ou resposta inválida → cache stale ou ErrGatewayUnreachable
//  4. OK → grava cache e devolve
func ProbeGatewayDetailed(cfg GatewayConfig) (*GatewayProbeResult, error) {
	if !cfg.IsEnabled() {
		return nil, ErrGatewayDisabled
	}
	base := cfg.ResolvedBaseURL()
	if base == "" {
		return nil, ErrGatewayNotConfigured
	}
	cachePath := cfg.resolvedCacheFile()
	if fresh := readGatewayCache(cachePath, cfg.ttlDuration()); fresh != nil {
		return fresh, nil
	}
	token, err := readAuth0Token(cfg.ResolvedTokenFile())
	if err != nil {
		return staleOr(cachePath, cfg.staleDuration(), err)
	}
	raw, err := fetchGatewayUsage(base, token, cfg.timeoutDuration())
	if err != nil {
		return staleOr(cachePath, cfg.staleDuration(), fmt.Errorf("%w: %v", ErrGatewayUnreachable, err))
	}
	usage, err := ParseGatewayUsage(raw)
	if err != nil {
		return staleOr(cachePath, cfg.staleDuration(), fmt.Errorf("%w: %v", ErrGatewayUnreachable, err))
	}
	if usage == nil {
		return nil, ErrGatewayNoBudget
	}
	res := &GatewayProbeResult{Usage: usage, Raw: raw, FetchedAt: time.Now()}
	writeGatewayCache(cachePath, res)
	return res, nil
}

func staleOr(cachePath string, staleTTL time.Duration, err error) (*GatewayProbeResult, error) {
	if stale := readGatewayCache(cachePath, staleTTL); stale != nil {
		stale.Stale = true
		return stale, nil
	}
	return nil, err
}

type auth0TokenCache struct {
	AccessToken string  `json:"access_token"`
	ExpiresAt   float64 `json:"expires_at"` // epoch segundos (ms tolerado)
}

func readAuth0Token(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", ErrGatewayTokenMissing
	}
	var cache auth0TokenCache
	if err := json.Unmarshal(data, &cache); err != nil || cache.AccessToken == "" {
		return "", ErrGatewayTokenMissing
	}
	expires := cache.ExpiresAt
	if expires > 1e12 { // veio em milissegundos
		expires /= 1000
	}
	if expires > 0 && float64(time.Now().Unix()) >= expires {
		return "", ErrGatewayTokenExpired
	}
	return cache.AccessToken, nil
}

func fetchGatewayUsage(base, token string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/usage", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "claude-statusline/"+Version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

func readGatewayCache(path string, maxAge time.Duration) *GatewayProbeResult {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var entry gatewayDiskCache
	if err := json.Unmarshal(data, &entry); err != nil || entry.Usage == nil {
		return nil
	}
	fetched := time.Unix(entry.FetchedAt, 0)
	if time.Since(fetched) > maxAge {
		return nil
	}
	return &GatewayProbeResult{Usage: entry.Usage, Raw: entry.Raw, FetchedAt: fetched}
}

func writeGatewayCache(path string, res *GatewayProbeResult) {
	data, err := json.Marshal(gatewayDiskCache{FetchedAt: res.FetchedAt.Unix(), Usage: res.Usage, Raw: res.Raw})
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0600)
}
