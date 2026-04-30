import type {
  StatuslineComponentMeta,
  StatuslineConfig,
  StatuslineThemesResp,
} from './types'

async function get<T>(path: string): Promise<T> {
  const res = await fetch(path)
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`)
  return (await res.json()) as T
}

async function postJSON<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`)
  return (await res.json()) as T
}

export const api = {
  components: () => get<StatuslineComponentMeta[]>('/api/components'),
  themes: () => get<StatuslineThemesResp>('/api/themes'),
  presets: () =>
    get<{ names: string[]; presets: Record<string, StatuslineConfig> }>('/api/presets'),
  configGet: () => get<StatuslineConfig>('/api/config'),
  configSave: (cfg: StatuslineConfig) =>
    postJSON<{ status: string; path: string }>('/api/config', cfg),
  render: (cfg: StatuslineConfig, mockInput?: unknown, mockHistory?: unknown) =>
    postJSON<{ ansi: string; html: string }>('/api/render', {
      config: cfg,
      mock_input: mockInput,
      mock_history: mockHistory,
    }),
}
