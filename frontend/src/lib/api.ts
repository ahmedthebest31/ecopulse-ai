import type {
  AnalyticsResult,
  GeminiStatusResult,
  ReportResult,
  TariffBreakdown,
  TariffRequest,
  TelemetryResponse,
} from '../types'

export const API_BASE_URL =
  (import.meta.env.VITE_API_BASE_URL as string | undefined) ?? 'http://localhost:8080/api'

export interface TelemetryParams {
  facilityId?: string
  start?: string
  end?: string
  limit?: number
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const { headers, ...rest } = init ?? {}
  const response = await fetch(`${API_BASE_URL}${path}`, {
    ...rest,
    headers: {
      'Content-Type': 'application/json',
      ...(headers ?? {}),
    },
  })
  if (!response.ok) {
    let detail = `HTTP ${response.status}`
    try {
      const body = (await response.json()) as { error?: string }
      if (body?.error) {
        detail = body.error
      }
    } catch {
      // non-JSON error body; keep the status-based message
    }
    throw new Error(detail)
  }
  return (await response.json()) as T
}

export function fetchTelemetry(params: TelemetryParams = {}): Promise<TelemetryResponse> {
  const search = new URLSearchParams()
  if (params.facilityId) {
    search.set('facility_id', params.facilityId)
  }
  if (params.start) {
    search.set('start', params.start)
  }
  if (params.end) {
    search.set('end', params.end)
  }
  if (params.limit !== undefined) {
    search.set('limit', String(params.limit))
  }
  const query = search.toString()
  return request<TelemetryResponse>(`/telemetry${query ? `?${query}` : ''}`)
}

export interface AnalyticsParams {
  spikeThresholdPercent?: number
  peakStart?: string
  peakEnd?: string
}

export function fetchAnalytics(params: AnalyticsParams = {}): Promise<AnalyticsResult> {
  const search = new URLSearchParams()
  if (params.spikeThresholdPercent !== undefined && params.spikeThresholdPercent > 0) {
    search.set('spike_threshold_percent', String(params.spikeThresholdPercent))
  }
  if (params.peakStart && params.peakEnd) {
    search.set('peak_start', params.peakStart)
    search.set('peak_end', params.peakEnd)
  }
  const query = search.toString()
  return request<AnalyticsResult>(`/analytics/spikes${query ? `?${query}` : ''}`)
}

export function postTariff(req: TariffRequest): Promise<TariffBreakdown> {
  return request<TariffBreakdown>('/tariff/calculate', {
    method: 'POST',
    body: JSON.stringify(req),
  })
}

export function postReport(locale: string, customKey?: string): Promise<ReportResult> {
  const headers: Record<string, string> = {}
  if (customKey && customKey.trim() !== '') {
    headers['x-goog-api-key'] = customKey.trim()
  }
  return request<ReportResult>('/report/summary', {
    method: 'POST',
    headers,
    body: JSON.stringify({ locale }),
  })
}

export function fetchGeminiStatus(): Promise<GeminiStatusResult> {
  return request<GeminiStatusResult>('/config/gemini-status')
}
