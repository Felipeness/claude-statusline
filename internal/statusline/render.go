package statusline

import (
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"
)

// Render é a entry point: lê config, parseia stdin, busca history (best-effort),
// itera lines/components, devolve string ANSI pronta pra stdout.
func Render(in *Input, cfg *Config) string {
	hist := FetchHistory(
		cfg.History.Endpoint,
		in.SessionID,
		in.Workspace.ProjectDir,
		cfg.History.TimeoutDuration(),
	)
	return RenderWith(in, cfg, hist)
}

// RenderWith é a forma testável: caller fornece o HistoryData (ou nil) ao
// invés de buscar do daemon. Studio web usa essa pra ter controle total
// sobre os dados na preview.
func RenderWith(in *Input, cfg *Config, hist *HistoryData) string {
	theme := GetTheme(cfg.Theme)
	ctx := &RenderCtx{
		In:      in,
		History: hist,
		Theme:   theme,
		Now:     time.Now(),
	}

	width := terminalWidth()

	var lines []string
	for _, line := range cfg.Lines {
		segs := collectSegments(line.Components, ctx, cfg.Components)
		if len(segs) == 0 {
			continue
		}
		sep := line.Separator
		if sep == "" {
			sep = " │ "
		}
		render := func(s []Segment) string {
			switch cfg.Style {
			case "powerline":
				return renderPowerline(s)
			case "capsule":
				return renderCapsule(s)
			default:
				return renderPlain(s, sep, theme)
			}
		}
		out := render(segs)
		// Auto-wrap: se a linha visivel excede a largura, dropa segments
		// do FIM ate caber. Mantem ao menos 1 segment.
		if cfg.AutoWrap && width > 0 && len(segs) > 1 {
			for VisibleLen(out) > width && len(segs) > 1 {
				segs = segs[:len(segs)-1]
				out = render(segs)
			}
		}
		lines = append(lines, out)
	}
	return strings.Join(lines, "\n")
}

// terminalWidth tenta descobrir colunas. O statusline roda como subprocess
// do Claude Code com stdin/stdout/stderr pipados (sem TTY) e normalmente
// sem env COLUMNS, entao a ordem e:
//   1. env COLUMNS (Claude Code as vezes propaga)
//   2. controlling terminal direto (/dev/tty no Unix, CONIN$ no Windows)
//   3. fallback nos fds stderr/stdin
// Devolve 0 quando nada funciona (= sem truncate, comportamento antigo).
func terminalWidth() int {
	if v := os.Getenv("COLUMNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	if w := widthFromControllingTTY(); w > 0 {
		return w
	}
	for _, fd := range []int{int(os.Stderr.Fd()), int(os.Stdin.Fd())} {
		if w, _, err := term.GetSize(fd); err == nil && w > 0 {
			return w
		}
	}
	return 0
}

// collectSegments resolve cada nome em Segment via registry, dropa vazios.
func collectSegments(names []string, ctx *RenderCtx, perComp map[string]ComponentOpts) []Segment {
	out := make([]Segment, 0, len(names))
	for _, name := range names {
		comp := Get(name)
		if comp == nil {
			continue
		}
		opts := perComp[name]
		if opts.Hide {
			continue
		}
		seg := comp.Render(ctx, opts)
		if seg.Empty() {
			continue
		}
		out = append(out, seg)
	}
	return out
}

// renderPlain — separator entre segments, cores in-line. Usa só FG (BG ignorado).
func renderPlain(segs []Segment, sep string, theme *Theme) string {
	var b strings.Builder
	sepCol := theme.Muted
	for i, s := range segs {
		if i > 0 {
			b.WriteString(sepCol.FG())
			b.WriteString(sep)
			b.WriteString(Reset)
		}
		if s.Bold {
			b.WriteString(Bold)
		}
		b.WriteString(s.FG.FG())
		b.WriteString(s.Text)
		b.WriteString(Reset)
	}
	return b.String()
}

// renderPowerline — segmentos em pílulas com BG, transição via arrow glyph.
// Padrão herdado do Owloops/claude-powerline:
//   reset → next.BG → prev.BG-as-FG → arrow → next segment
const powerlineArrow = "" //

func renderPowerline(segs []Segment) string {
	var b strings.Builder
	for i, s := range segs {
		bg := s.BG
		if bg.Empty() {
			bg = Color{R: 45, G: 45, B: 45} // graphite default
		}
		// segment body
		b.WriteString(bg.BG())
		b.WriteString(s.FG.FG())
		if s.Bold {
			b.WriteString(Bold)
		}
		b.WriteString(" " + s.Text + " ")
		// transition
		b.WriteString(Reset)
		if i < len(segs)-1 {
			next := segs[i+1].BG
			if next.Empty() {
				next = bg
			}
			b.WriteString(next.BG())
			b.WriteString(bg.FG()) // prev BG vira FG do arrow
			b.WriteString(powerlineArrow)
			b.WriteString(Reset)
		} else {
			// tail
			b.WriteString(bg.FG())
			b.WriteString(powerlineArrow)
			b.WriteString(Reset)
		}
	}
	return b.String()
}

// renderCapsule — pílulas independentes com bordas arredondadas, sem
// transição entre elas. Usa  e .
const (
	capsuleLeft  = "" //
	capsuleRight = "" //
)

func renderCapsule(segs []Segment) string {
	var b strings.Builder
	for i, s := range segs {
		if i > 0 {
			b.WriteString(" ")
		}
		bg := s.BG
		if bg.Empty() {
			bg = Color{R: 45, G: 45, B: 45}
		}
		b.WriteString(bg.FG())
		b.WriteString(capsuleLeft)
		b.WriteString(Reset)
		b.WriteString(bg.BG())
		b.WriteString(s.FG.FG())
		if s.Bold {
			b.WriteString(Bold)
		}
		b.WriteString(" " + s.Text + " ")
		b.WriteString(Reset)
		b.WriteString(bg.FG())
		b.WriteString(capsuleRight)
		b.WriteString(Reset)
	}
	return b.String()
}
