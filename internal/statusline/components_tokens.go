package statusline

// sessionTokens lê os contadores da sessão: current_usage quando o Claude
// Code manda (turno atual), senão os totais acumulados.
func sessionTokens(in *Input) (input, output, cache int64) {
	cur := in.Context.Current
	input = int64(cur.InputTokens)
	if input == 0 {
		input = int64(in.Context.TotalInputTokens)
	}
	output = int64(cur.OutputTokens)
	if output == 0 {
		output = int64(in.Context.TotalOutputTokens)
	}
	return input, output, int64(cur.CacheReadInputTokens)
}

// tokenComp é o molde dos 4 chips: label fixo + valor formatado. value
// devolve (n, visible); visible=false esconde o chip.
type tokenComp struct {
	name  string
	label string
	value func(input, output, cache int64) (int64, bool)
}

func (t tokenComp) Name() string { return t.name }
func (t tokenComp) Render(c *RenderCtx, opts ComponentOpts) Segment {
	n, visible := t.value(sessionTokens(c.In))
	if !visible {
		return Segment{}
	}
	seg := c.Theme.SegOf(t.name)
	return Segment{Name: t.name, Text: opts.LabelPrefix + t.label + ": " + FormatTokens(n), FG: seg.FG, BG: seg.BG}
}

func init() {
	Register(tokenComp{name: "tokens_in", label: "In", value: func(in, _, _ int64) (int64, bool) { return in, true }})
	Register(tokenComp{name: "tokens_out", label: "Out", value: func(_, out, _ int64) (int64, bool) { return out, true }})
	Register(tokenComp{name: "tokens_total", label: "Total", value: func(in, out, _ int64) (int64, bool) { return in + out, true }})
	Register(tokenComp{name: "tokens_cache", label: "Cache", value: func(_, _, cache int64) (int64, bool) { return cache, cache > 0 }})
}
