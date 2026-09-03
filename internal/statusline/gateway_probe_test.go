package statusline

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func writeTokenFile(t *testing.T, dir string, expiresAt int64) string {
	t.Helper()
	path := filepath.Join(dir, "auth0-token-cache.json")
	body := map[string]any{"access_token": "jwt-test", "expires_at": expiresAt}
	data, _ := json.Marshal(body)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func gatewayServer(t *testing.T, status int, body string, hits *int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(hits, 1)
		if r.URL.Path != "/v1/usage" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer jwt-test" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q", got)
		}
		if got := r.Header.Get("User-Agent"); !strings.HasPrefix(got, "claude-statusline/") {
			t.Errorf("User-Agent = %q, want prefix claude-statusline/", got)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func TestProbeGatewayDetailed(t *testing.T) {
	fixture, err := os.ReadFile("testdata/gateway_usage.json")
	if err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Hour).Unix()
	past := time.Now().Add(-time.Hour).Unix()

	t.Run("fetches once and serves fresh cache without http", func(t *testing.T) {
		var hits int32
		srv := gatewayServer(t, 200, string(fixture), &hits)
		defer srv.Close()
		dir := t.TempDir()
		cfg := GatewayConfig{
			BaseURL: srv.URL, TokenFile: writeTokenFile(t, dir, future),
			CacheFile: filepath.Join(dir, "cache.json"), TTL: "60s", StaleTTL: "1h", Timeout: "2s",
		}
		first, err := ProbeGatewayDetailed(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if first.Usage == nil || first.Usage.SpentBRLMicro != 73_530_000 || first.Stale {
			t.Fatalf("unexpected first result %+v", first)
		}
		second, err := ProbeGatewayDetailed(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if second.Usage.SpentBRLMicro != 73_530_000 || second.Stale {
			t.Fatalf("unexpected cached result %+v", second)
		}
		if got := atomic.LoadInt32(&hits); got != 1 {
			t.Fatalf("server hits = %d, want 1", got)
		}
		cached, _ := os.ReadFile(cfg.CacheFile)
		if len(cached) == 0 || strings.Contains(string(cached), "jwt-test") {
			t.Fatalf("cache must exist and never contain the token: %s", cached)
		}
	})

	t.Run("expired token never calls the gateway", func(t *testing.T) {
		var hits int32
		srv := gatewayServer(t, 200, string(fixture), &hits)
		defer srv.Close()
		dir := t.TempDir()
		cfg := GatewayConfig{BaseURL: srv.URL, TokenFile: writeTokenFile(t, dir, past), CacheFile: filepath.Join(dir, "cache.json")}
		_, err := ProbeGatewayDetailed(cfg)
		if !errors.Is(err, ErrGatewayTokenExpired) {
			t.Fatalf("err = %v, want ErrGatewayTokenExpired", err)
		}
		if got := atomic.LoadInt32(&hits); got != 0 {
			t.Fatalf("server hits = %d, want 0", got)
		}
	})

	t.Run("expired token does not write negative cache", func(t *testing.T) {
		var hits int32
		srv := gatewayServer(t, 200, string(fixture), &hits)
		defer srv.Close()
		dir := t.TempDir()
		cacheFile := filepath.Join(dir, "cache.json")
		cfg := GatewayConfig{BaseURL: srv.URL, TokenFile: writeTokenFile(t, dir, past), CacheFile: cacheFile}
		_, err := ProbeGatewayDetailed(cfg)
		if !errors.Is(err, ErrGatewayTokenExpired) {
			t.Fatalf("err = %v, want ErrGatewayTokenExpired", err)
		}
		if _, statErr := os.Stat(cacheFile); !os.IsNotExist(statErr) {
			t.Fatalf("expected no cache file, stat err = %v", statErr)
		}
	})

	t.Run("missing token file", func(t *testing.T) {
		dir := t.TempDir()
		cfg := GatewayConfig{BaseURL: "http://127.0.0.1:1", TokenFile: filepath.Join(dir, "nope.json"), CacheFile: filepath.Join(dir, "cache.json")}
		if _, err := ProbeGatewayDetailed(cfg); !errors.Is(err, ErrGatewayTokenMissing) {
			t.Fatalf("err = %v, want ErrGatewayTokenMissing", err)
		}
	})

	t.Run("not configured without base url", func(t *testing.T) {
		t.Setenv("ANTHROPIC_BASE_URL", "")
		cfg := GatewayConfig{TokenFile: writeTokenFile(t, t.TempDir(), future)}
		if _, err := ProbeGatewayDetailed(cfg); !errors.Is(err, ErrGatewayNotConfigured) {
			t.Fatalf("err = %v, want ErrGatewayNotConfigured", err)
		}
	})

	t.Run("base url falls back to env", func(t *testing.T) {
		var hits int32
		srv := gatewayServer(t, 200, string(fixture), &hits)
		defer srv.Close()
		t.Setenv("ANTHROPIC_BASE_URL", srv.URL+"/")
		dir := t.TempDir()
		cfg := GatewayConfig{TokenFile: writeTokenFile(t, dir, future), CacheFile: filepath.Join(dir, "cache.json")}
		res, err := ProbeGatewayDetailed(cfg)
		if err != nil || res.Usage == nil {
			t.Fatalf("res=%+v err=%v", res, err)
		}
	})

	t.Run("server error serves stale cache", func(t *testing.T) {
		var hits int32
		srv := gatewayServer(t, 500, `boom`, &hits)
		defer srv.Close()
		dir := t.TempDir()
		cacheFile := filepath.Join(dir, "cache.json")
		stale := gatewayDiskCache{FetchedAt: time.Now().Add(-5 * time.Minute).Unix(), Usage: &GatewayUsage{SpentBRLMicro: 42}}
		data, _ := json.Marshal(stale)
		_ = os.WriteFile(cacheFile, data, 0600)
		cfg := GatewayConfig{BaseURL: srv.URL, TokenFile: writeTokenFile(t, dir, future), CacheFile: cacheFile, TTL: "60s", StaleTTL: "1h"}
		res, err := ProbeGatewayDetailed(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if res.Usage.SpentBRLMicro != 42 || !res.Stale {
			t.Fatalf("expected stale cache, got %+v", res)
		}
	})

	t.Run("server error without cache is unreachable", func(t *testing.T) {
		var hits int32
		srv := gatewayServer(t, 500, `boom`, &hits)
		defer srv.Close()
		dir := t.TempDir()
		cfg := GatewayConfig{BaseURL: srv.URL, TokenFile: writeTokenFile(t, dir, future), CacheFile: filepath.Join(dir, "cache.json")}
		if _, err := ProbeGatewayDetailed(cfg); !errors.Is(err, ErrGatewayUnreachable) {
			t.Fatalf("err = %v, want ErrGatewayUnreachable", err)
		}
	})

	t.Run("response without budget", func(t *testing.T) {
		var hits int32
		srv := gatewayServer(t, 200, `{"entries":[]}`, &hits)
		defer srv.Close()
		dir := t.TempDir()
		cfg := GatewayConfig{BaseURL: srv.URL, TokenFile: writeTokenFile(t, dir, future), CacheFile: filepath.Join(dir, "cache.json")}
		if _, err := ProbeGatewayDetailed(cfg); !errors.Is(err, ErrGatewayNoBudget) {
			t.Fatalf("err = %v, want ErrGatewayNoBudget", err)
		}
	})

	t.Run("negative cache throttles retries after failure", func(t *testing.T) {
		var hits int32
		srv := gatewayServer(t, 500, `boom`, &hits)
		defer srv.Close()
		dir := t.TempDir()
		cfg := GatewayConfig{
			BaseURL: srv.URL, TokenFile: writeTokenFile(t, dir, future),
			CacheFile: filepath.Join(dir, "cache.json"), TTL: "60s", StaleTTL: "1h",
		}
		if _, err := ProbeGatewayDetailed(cfg); !errors.Is(err, ErrGatewayUnreachable) {
			t.Fatalf("first call err = %v, want ErrGatewayUnreachable", err)
		}
		if _, err := ProbeGatewayDetailed(cfg); !errors.Is(err, ErrGatewayUnreachable) {
			t.Fatalf("second call err = %v, want ErrGatewayUnreachable", err)
		}
		if got := atomic.LoadInt32(&hits); got != 1 {
			t.Fatalf("server hits = %d, want 1", got)
		}
	})

	t.Run("negative cache serves stale usage without http", func(t *testing.T) {
		var hits int32
		srv := gatewayServer(t, 200, string(fixture), &hits)
		defer srv.Close()
		dir := t.TempDir()
		cacheFile := filepath.Join(dir, "cache.json")
		entry := gatewayDiskCache{
			FetchedAt: time.Now().Add(-5 * time.Minute).Unix(),
			Usage:     &GatewayUsage{SpentBRLMicro: 99},
			FailedAt:  time.Now().Unix(),
			LastError: "gateway inacessível: HTTP 500",
		}
		data, _ := json.Marshal(entry)
		if err := os.WriteFile(cacheFile, data, 0600); err != nil {
			t.Fatal(err)
		}
		cfg := GatewayConfig{
			BaseURL: srv.URL, TokenFile: writeTokenFile(t, dir, future),
			CacheFile: cacheFile, TTL: "60s", StaleTTL: "1h",
		}
		res, err := ProbeGatewayDetailed(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if res.Usage == nil || res.Usage.SpentBRLMicro != 99 || !res.Stale {
			t.Fatalf("expected stale usage from negative cache, got %+v", res)
		}
		if got := atomic.LoadInt32(&hits); got != 0 {
			t.Fatalf("server hits = %d, want 0", got)
		}
	})

	t.Run("unauthorized maps to token error", func(t *testing.T) {
		var hits int32
		srv := gatewayServer(t, 401, `{"error":"unauthorized"}`, &hits)
		defer srv.Close()
		dir := t.TempDir()
		cfg := GatewayConfig{BaseURL: srv.URL, TokenFile: writeTokenFile(t, dir, future), CacheFile: filepath.Join(dir, "cache.json")}
		if _, err := ProbeGatewayDetailed(cfg); !errors.Is(err, ErrGatewayUnauthorized) {
			t.Fatalf("err = %v, want ErrGatewayUnauthorized", err)
		}
		if got := atomic.LoadInt32(&hits); got != 1 {
			t.Fatalf("server hits = %d, want 1", got)
		}
	})

	t.Run("corrupt cache falls through to http", func(t *testing.T) {
		var hits int32
		srv := gatewayServer(t, 200, string(fixture), &hits)
		defer srv.Close()
		dir := t.TempDir()
		cacheFile := filepath.Join(dir, "cache.json")
		if err := os.WriteFile(cacheFile, []byte(`{not json`), 0600); err != nil {
			t.Fatal(err)
		}
		cfg := GatewayConfig{BaseURL: srv.URL, TokenFile: writeTokenFile(t, dir, future), CacheFile: cacheFile}
		res, err := ProbeGatewayDetailed(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if res.Usage == nil || res.Usage.SpentBRLMicro != 73_530_000 {
			t.Fatalf("unexpected result %+v", res)
		}
		if got := atomic.LoadInt32(&hits); got != 1 {
			t.Fatalf("server hits = %d, want 1", got)
		}
		cached, err := os.ReadFile(cacheFile)
		if err != nil {
			t.Fatal(err)
		}
		var parsed gatewayDiskCache
		if err := json.Unmarshal(cached, &parsed); err != nil {
			t.Fatalf("cache file does not parse: %v", err)
		}
	})

	t.Run("cache dir is created and written atomically", func(t *testing.T) {
		var hits int32
		srv := gatewayServer(t, 200, string(fixture), &hits)
		defer srv.Close()
		dir := t.TempDir()
		cacheFile := filepath.Join(dir, "nested", "sub", "cache.json")
		cfg := GatewayConfig{BaseURL: srv.URL, TokenFile: writeTokenFile(t, dir, future), CacheFile: cacheFile}
		res, err := ProbeGatewayDetailed(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if res.CacheWriteError != nil {
			t.Fatalf("CacheWriteError = %v, want nil", res.CacheWriteError)
		}
		if _, err := os.Stat(cacheFile); err != nil {
			t.Fatalf("cache file not created: %v", err)
		}
		entries, err := os.ReadDir(filepath.Dir(cacheFile))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".tmp") {
				t.Fatalf("leftover tmp file: %s", e.Name())
			}
		}
	})

	t.Run("negative cache replays unauthorized", func(t *testing.T) {
		var hits int32
		srv := gatewayServer(t, 401, `{"error":"unauthorized"}`, &hits)
		defer srv.Close()
		dir := t.TempDir()
		cfg := GatewayConfig{
			BaseURL: srv.URL, TokenFile: writeTokenFile(t, dir, future),
			CacheFile: filepath.Join(dir, "cache.json"), TTL: "60s", StaleTTL: "1h",
		}
		if _, err := ProbeGatewayDetailed(cfg); !errors.Is(err, ErrGatewayUnauthorized) {
			t.Fatalf("first call err = %v, want ErrGatewayUnauthorized", err)
		}
		_, err := ProbeGatewayDetailed(cfg)
		if !errors.Is(err, ErrGatewayUnauthorized) {
			t.Fatalf("second call err = %v, want ErrGatewayUnauthorized", err)
		}
		if got := atomic.LoadInt32(&hits); got != 1 {
			t.Fatalf("server hits = %d, want 1", got)
		}
		if got := strings.Count(err.Error(), "refazer o login"); got != 1 {
			t.Fatalf("err.Error() = %q, want %q exactly once, got %d", err.Error(), "refazer o login", got)
		}
	})

	t.Run("negative cache replays no budget without duplicate text", func(t *testing.T) {
		var hits int32
		srv := gatewayServer(t, 200, `{"entries":[]}`, &hits)
		defer srv.Close()
		dir := t.TempDir()
		cfg := GatewayConfig{
			BaseURL: srv.URL, TokenFile: writeTokenFile(t, dir, future),
			CacheFile: filepath.Join(dir, "cache.json"), TTL: "60s", StaleTTL: "1h",
		}
		if _, err := ProbeGatewayDetailed(cfg); !errors.Is(err, ErrGatewayNoBudget) {
			t.Fatalf("first call err = %v, want ErrGatewayNoBudget", err)
		}
		_, err := ProbeGatewayDetailed(cfg)
		if !errors.Is(err, ErrGatewayNoBudget) {
			t.Fatalf("second call err = %v, want ErrGatewayNoBudget", err)
		}
		if got := atomic.LoadInt32(&hits); got != 1 {
			t.Fatalf("server hits = %d, want 1", got)
		}
		if got := strings.Count(err.Error(), "sem budget"); got != 1 {
			t.Fatalf("err.Error() = %q, want %q exactly once, got %d", err.Error(), "sem budget", got)
		}
	})

	t.Run("disabled", func(t *testing.T) {
		off := false
		if _, err := ProbeGatewayDetailed(GatewayConfig{Enabled: &off}); !errors.Is(err, ErrGatewayDisabled) {
			t.Fatalf("err = %v, want ErrGatewayDisabled", err)
		}
	})

	t.Run("slow gateway respects timeout", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(300 * time.Millisecond)
		}))
		defer srv.Close()
		dir := t.TempDir()
		cfg := GatewayConfig{BaseURL: srv.URL, TokenFile: writeTokenFile(t, dir, future), CacheFile: filepath.Join(dir, "cache.json"), Timeout: "50ms"}
		start := time.Now()
		_, err := ProbeGatewayDetailed(cfg)
		if !errors.Is(err, ErrGatewayUnreachable) {
			t.Fatalf("err = %v, want ErrGatewayUnreachable", err)
		}
		if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
			t.Fatalf("probe took %v, timeout not applied", elapsed)
		}
	})

	t.Run("fail-open wrapper returns nil on error", func(t *testing.T) {
		off := false
		if got := ProbeGateway(GatewayConfig{Enabled: &off}); got != nil {
			t.Fatalf("ProbeGateway = %+v, want nil", got)
		}
	})
}

func TestGatewayConfigured(t *testing.T) {
	dir := t.TempDir()
	token := writeTokenFile(t, dir, time.Now().Add(time.Hour).Unix())
	off := false
	cases := []struct {
		name string
		cfg  GatewayConfig
		env  string
		want bool
	}{
		{"url and token", GatewayConfig{BaseURL: "http://gw", TokenFile: token}, "", true},
		{"url from env", GatewayConfig{TokenFile: token}, "http://gw", true},
		{"no url", GatewayConfig{TokenFile: token}, "", false},
		{"no token file", GatewayConfig{BaseURL: "http://gw", TokenFile: filepath.Join(dir, "missing.json")}, "", false},
		{"disabled", GatewayConfig{Enabled: &off, BaseURL: "http://gw", TokenFile: token}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ANTHROPIC_BASE_URL", tc.env)
			if got := GatewayConfigured(tc.cfg); got != tc.want {
				t.Fatalf("GatewayConfigured = %v, want %v", got, tc.want)
			}
		})
	}
}
