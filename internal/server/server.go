// Package server é o HTTP backend mínimo do Studio web. 5 endpoints:
//   GET  /api/components — catálogo dos 16 components
//   GET  /api/themes — 5 themes + 3 styles com cores RGB
//   GET  /api/presets — 3 presets canônicos
//   GET  /api/config — config atual (TOML → JSON)
//   POST /api/config — salva config nova
//   POST /api/render — recebe {config, mock_input, mock_history} → {ansi, html}
package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/felipeness/claude-statusline/internal/statusline"
)

type Server struct {
	ConfigPath string
	Static     http.Handler
}

func Run(s *Server, listen string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/components", s.handleComponents)
	mux.HandleFunc("/api/themes", s.handleThemes)
	mux.HandleFunc("/api/presets", s.handlePresets)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/render", s.handleRender)
	if s.Static != nil {
		mux.Handle("/", s.Static)
	}
	return http.ListenAndServe(listen, withCORS(mux))
}

func withCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func (s *Server) handleComponents(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, statusline.Metas())
}

type colorOut struct {
	R uint8 `json:"r"`
	G uint8 `json:"g"`
	B uint8 `json:"b"`
}
type segOut struct {
	BG colorOut `json:"bg"`
	FG colorOut `json:"fg"`
}
type themeOut struct {
	Name    string            `json:"name"`
	Default segOut            `json:"default"`
	Segs    map[string]segOut `json:"segs"`
	Status  struct {
		OK   colorOut `json:"ok"`
		Warn colorOut `json:"warn"`
		Crit colorOut `json:"crit"`
	} `json:"status"`
	Muted colorOut `json:"muted"`
}

func (s *Server) handleThemes(w http.ResponseWriter, r *http.Request) {
	out := make([]themeOut, 0, len(statusline.ThemeNames))
	for _, name := range statusline.ThemeNames {
		t := statusline.Themes[name]
		if t == nil {
			continue
		}
		o := themeOut{
			Name:    t.Name,
			Default: segOut{
				BG: colorOut{t.Default.BG.R, t.Default.BG.G, t.Default.BG.B},
				FG: colorOut{t.Default.FG.R, t.Default.FG.G, t.Default.FG.B},
			},
			Muted: colorOut{t.Muted.R, t.Muted.G, t.Muted.B},
			Segs:  map[string]segOut{},
		}
		o.Status.OK = colorOut{t.Status.OK.R, t.Status.OK.G, t.Status.OK.B}
		o.Status.Warn = colorOut{t.Status.Warn.R, t.Status.Warn.G, t.Status.Warn.B}
		o.Status.Crit = colorOut{t.Status.Crit.R, t.Status.Crit.G, t.Status.Crit.B}
		for k, v := range t.Segs {
			o.Segs[k] = segOut{
				BG: colorOut{v.BG.R, v.BG.G, v.BG.B},
				FG: colorOut{v.FG.R, v.FG.G, v.FG.B},
			}
		}
		out = append(out, o)
	}
	writeJSON(w, 200, map[string]any{"themes": out, "styles": statusline.StyleNames})
}

func (s *Server) handlePresets(w http.ResponseWriter, r *http.Request) {
	out := map[string]*statusline.Config{}
	for _, name := range statusline.PresetNames {
		if cfg := statusline.Presets[name]; cfg != nil {
			out[name] = cfg
		}
	}
	writeJSON(w, 200, map[string]any{
		"names":   statusline.PresetNames,
		"presets": out,
	})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, err := statusline.LoadConfig(s.ConfigPath)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, cfg)
	case http.MethodPost:
		var cfg statusline.Config
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeErr(w, 400, "invalid json: "+err.Error())
			return
		}
		if cfg.Theme == "" {
			cfg.Theme = "graphite"
		}
		if cfg.Style == "" {
			cfg.Style = "plain"
		}
		if err := os.MkdirAll(filepath.Dir(s.ConfigPath), 0755); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if err := statusline.SaveConfig(s.ConfigPath, &cfg); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"status": "saved", "path": s.ConfigPath})
	default:
		writeErr(w, 405, "method not allowed")
	}
}

type renderRequest struct {
	Config      *statusline.Config      `json:"config"`
	MockInput   *statusline.Input       `json:"mock_input"`
	MockHistory *statusline.HistoryData `json:"mock_history"`
}

func (s *Server) handleRender(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "POST required")
		return
	}
	var req renderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "invalid json: "+err.Error())
		return
	}
	if req.Config == nil {
		writeErr(w, 400, "config required")
		return
	}
	if req.MockInput == nil {
		req.MockInput = defaultMockInput()
	}
	out := statusline.RenderWith(req.MockInput, req.Config, req.MockHistory)
	writeJSON(w, 200, map[string]string{
		"ansi": out,
		"html": statusline.AnsiToHTML(out),
	})
}

func defaultMockInput() *statusline.Input {
	return &statusline.Input{
		CWD:       "/Users/dev/projects/my-app",
		SessionID: "preview-mock",
		Model: statusline.ModelInfo{
			DisplayName: "Opus 4.7",
			ID:          "claude-opus-4-7",
		},
		Workspace: statusline.Workspace{
			CurrentDir: "/Users/dev/projects/my-app",
			ProjectDir: "/Users/dev/projects/my-app",
		},
		Context: statusline.ContextWindow{
			UsedPercentage:    42,
			TotalInputTokens:  18432,
			TotalOutputTokens: 4521,
		},
		Cost: statusline.CostInfo{
			TotalCostUSD:      0.32,
			TotalLinesAdded:   45,
			TotalLinesRemoved: 12,
		},
		RateLimits: &statusline.RateLimits{
			FiveHour: &statusline.RateLimitWindow{UsedPercentage: 73},
			SevenDay: &statusline.RateLimitWindow{UsedPercentage: 18},
		},
		Worktree: &statusline.WorktreeInfo{Branch: "feat/CC-1234-statusline"},
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
