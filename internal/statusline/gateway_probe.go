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

// resolvedCacheFile só resolve o caminho — não toca em disco. Quem grava
// (writeGatewayCache) é responsável por criar o diretório.
func (c GatewayConfig) resolvedCacheFile() string {
	if c.CacheFile != "" {
		return c.CacheFile
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "claude-statusline-gateway.json")
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
	ErrGatewayUnauthorized  = errors.New("gateway rejeitou o token: abra o Claude Code pra refazer o login")
	ErrGatewayUnreachable   = errors.New("gateway inacessível")
	ErrGatewayNoBudget      = errors.New("gateway respondeu sem budget pro usuário")
)

// GatewayProbeResult é o que o probe devolve: dados parseados, resposta crua
// (pro budget --json), se veio de cache stale e se a gravação do cache
// falhou (não impede o resultado — o probe segue fail-open).
type GatewayProbeResult struct {
	Usage           *GatewayUsage
	Raw             json.RawMessage
	FetchedAt       time.Time
	Stale           bool
	CacheWriteError error `json:"-"`
}

// gatewayDiskCache é o único cache (cada render é um processo novo). Nunca
// guarda o token, só dados de consumo. FailedAt/LastError formam o cache
// negativo: uma falha recente evita bater HTTP de novo a cada render
// enquanto o gateway estiver fora do ar. Um fetch bem-sucedido limpa os
// dois.
type gatewayDiskCache struct {
	FetchedAt int64           `json:"fetched_at"`
	Usage     *GatewayUsage   `json:"usage"`
	Raw       json.RawMessage `json:"raw,omitempty"`
	FailedAt  int64           `json:"failed_at,omitempty"`
	LastError string          `json:"last_error,omitempty"`
}

// ProbeGateway é a face fail-open: nil em qualquer erro, pro render.
func ProbeGateway(cfg GatewayConfig) *GatewayProbeResult {
	res, err := ProbeGatewayDetailed(cfg)
	if err != nil {
		return nil
	}
	return res
}

// ProbeGatewayDetailed busca /v1/usage com cache em disco e cache negativo
// (evita pagar o timeout inteiro em toda renderização enquanto o gateway
// estiver fora do ar):
//  1. cache de sucesso mais novo que TTL → devolve sem HTTP.
//  2. falha registrada há menos que TTL → sem HTTP: se ainda houver usage
//     preservado mais novo que StaleTTL, devolve stale; senão devolve o
//     erro tipado da última tentativa.
//  3. token ausente/expirado → cache stale (< StaleTTL) ou erro tipado.
//     Nunca grava cache negativo: token é problema local, não do gateway.
//  4. HTTP falha (401/403 vira ErrGatewayUnauthorized), resposta inválida
//     ou sem budget → grava cache negativo preservando o usage anterior
//     (se houver) e devolve stale ou o erro tipado.
//  5. OK → grava cache limpando qualquer marca de falha anterior.
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
	if res, err := negativeCacheHit(cachePath, cfg); res != nil || err != nil {
		return res, err
	}
	token, err := readAuth0Token(cfg.ResolvedTokenFile())
	if err != nil {
		return staleOr(cachePath, cfg.staleDuration(), err)
	}
	raw, err := fetchGatewayUsage(base, token, cfg.timeoutDuration())
	if err != nil {
		if !errors.Is(err, ErrGatewayUnauthorized) {
			err = fmt.Errorf("%w: %v", ErrGatewayUnreachable, err)
		}
		return failWithCache(cachePath, cfg.staleDuration(), err)
	}
	usage, err := ParseGatewayUsage(raw)
	if err != nil {
		return failWithCache(cachePath, cfg.staleDuration(), fmt.Errorf("%w: %v", ErrGatewayUnreachable, err))
	}
	if usage == nil {
		return failWithCache(cachePath, cfg.staleDuration(), ErrGatewayNoBudget)
	}
	res := &GatewayProbeResult{Usage: usage, Raw: raw, FetchedAt: time.Now()}
	entry := gatewayDiskCache{FetchedAt: res.FetchedAt.Unix(), Usage: res.Usage, Raw: res.Raw}
	if werr := writeGatewayCache(cachePath, entry); werr != nil {
		res.CacheWriteError = werr
	}
	return res, nil
}

// negativeCacheHit devolve (nil, nil) quando não há falha recente registrada
// (segue o fluxo normal). Quando há, evita HTTP: usage ainda dentro de
// StaleTTL vira resultado stale, senão devolve o erro tipado da última
// tentativa.
func negativeCacheHit(cachePath string, cfg GatewayConfig) (*GatewayProbeResult, error) {
	entry, ok := readGatewayCacheEntry(cachePath)
	if !ok || entry.FailedAt == 0 {
		return nil, nil
	}
	if time.Since(time.Unix(entry.FailedAt, 0)) >= cfg.ttlDuration() {
		return nil, nil
	}
	if entry.Usage != nil {
		fetched := time.Unix(entry.FetchedAt, 0)
		if time.Since(fetched) < cfg.staleDuration() {
			return &GatewayProbeResult{Usage: entry.Usage, Raw: entry.Raw, FetchedAt: fetched, Stale: true}, nil
		}
	}
	sentinel := ErrGatewayUnreachable
	if entry.LastError == ErrGatewayNoBudget.Error() {
		sentinel = ErrGatewayNoBudget
	}
	return nil, fmt.Errorf("%w: %s", sentinel, entry.LastError)
}

// staleOr devolve o cache stale (< staleTTL) quando existe, senão err.
// Usada pra falhas locais (token ausente/expirado) que não devem gravar
// cache negativo.
func staleOr(cachePath string, staleTTL time.Duration, err error) (*GatewayProbeResult, error) {
	if stale := readGatewayCache(cachePath, staleTTL); stale != nil {
		stale.Stale = true
		return stale, nil
	}
	return nil, err
}

// failWithCache grava o cache negativo (preservando usage/raw/fetchedAt
// anteriores, se houver) e devolve o cache stale quando ainda utilizável,
// senão err. A gravação é best-effort: falha nela não muda o resultado
// devolvido pro caller, que já é um erro de qualquer forma.
func failWithCache(cachePath string, staleTTL time.Duration, err error) (*GatewayProbeResult, error) {
	recordGatewayFailure(cachePath, err)
	return staleOr(cachePath, staleTTL, err)
}

func recordGatewayFailure(cachePath string, err error) {
	entry := gatewayDiskCache{FailedAt: time.Now().Unix(), LastError: err.Error()}
	if prev, ok := readGatewayCacheEntry(cachePath); ok {
		entry.Usage = prev.Usage
		entry.Raw = prev.Raw
		entry.FetchedAt = prev.FetchedAt
	}
	_ = writeGatewayCache(cachePath, entry)
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
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("%w (HTTP %d)", ErrGatewayUnauthorized, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

// readGatewayCacheEntry lê o cache bruto, sem checar TTL nem exigir usage.
// Usada pelo cache negativo e pela gravação (pra preservar campos antigos).
func readGatewayCacheEntry(path string) (*gatewayDiskCache, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var entry gatewayDiskCache
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false
	}
	return &entry, true
}

func readGatewayCache(path string, maxAge time.Duration) *GatewayProbeResult {
	entry, ok := readGatewayCacheEntry(path)
	if !ok || entry.Usage == nil {
		return nil
	}
	fetched := time.Unix(entry.FetchedAt, 0)
	if time.Since(fetched) > maxAge {
		return nil
	}
	return &GatewayProbeResult{Usage: entry.Usage, Raw: entry.Raw, FetchedAt: fetched}
}

// writeGatewayCache grava atomicamente: escreve num arquivo temporário no
// mesmo diretório e troca por rename (evita leitor ver arquivo truncado).
// Cria o diretório se preciso.
func writeGatewayCache(path string, entry gatewayDiskCache) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal cache: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".claude-statusline-gateway-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp cache file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write temp cache file: %w", err)
	}
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("chmod temp cache file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp cache file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename cache file: %w", err)
	}
	return nil
}
