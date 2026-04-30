export type StatuslineComponentMeta = {
  name: string
  label: string
  category: string
  description: string
  needs_history: boolean
  has_warn_at: boolean
}

export type StatuslineColor = { r: number; g: number; b: number }
export type StatuslineThemeSeg = { bg: StatuslineColor; fg: StatuslineColor }
export type StatuslineTheme = {
  name: string
  default: StatuslineThemeSeg
  segs: Record<string, StatuslineThemeSeg>
  status: { ok: StatuslineColor; warn: StatuslineColor; crit: StatuslineColor }
  muted: StatuslineColor
}
export type StatuslineThemesResp = {
  themes: StatuslineTheme[]
  styles: string[]
}

export type StatuslineLine = {
  components: string[]
  separator?: string
}

export type StatuslineComponentOpts = {
  warn_at?: number
  critical_at?: number
  format?: string
  hide?: boolean
}

export type StatuslineConfig = {
  theme: string
  style: string
  charset?: string
  auto_wrap?: boolean
  lines: StatuslineLine[]
  components?: Record<string, StatuslineComponentOpts>
  history?: { endpoint?: string; timeout?: string }
}

export type StatuslineMock = {
  cwd: string
  branch: string
  model: string
  context_pct: number
  cost_usd: number
  lines_added: number
  lines_removed: number
  rate_5h_pct: number
  rate_7d_pct: number
  vim_mode: '' | 'NORMAL' | 'INSERT'
  burn_rate_tpm: number
  cost_p90: number
  cost_today: number
  cluster_name: string
}
