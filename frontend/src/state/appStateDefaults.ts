import type { AppLanguage } from '../i18n'
import type { Tier } from '../types'

export const TREES_ABSORPTION_KG_PER_YEAR = 21.77

export type Theme = 'dark' | 'light'
export type TimeFormat = '12h' | '24h'
export type TariffMode = 'flat' | 'tiered'
export type GeminiKeySource = 'system' | 'custom'

export interface AppState {
  configured: boolean
  theme: Theme
  language: AppLanguage
  timezone: string
  timeFormat: TimeFormat
  tariffMode: TariffMode
  flatRate: number
  tiers: Tier[]
  usdPerEGP: number
  spikeThresholdPercent: number
  peakStart: string
  peakEnd: string
  carbonFactor: number
  email: string
  webhookUrl: string
  geminiKeySource: GeminiKeySource
  geminiCustomKey: string
}

export const DEFAULT_EGYPTIAN_TIERS: Tier[] = [
  { name: 'Tier 1', lower_kwh: 0, upper_kwh: 50, rate_egp: 0.68 },
  { name: 'Tier 2', lower_kwh: 50, upper_kwh: 100, rate_egp: 0.78 },
  { name: 'Tier 3', lower_kwh: 100, upper_kwh: 200, rate_egp: 0.95 },
  { name: 'Tier 4', lower_kwh: 200, upper_kwh: 350, rate_egp: 1.55 },
  { name: 'Tier 5', lower_kwh: 350, upper_kwh: 650, rate_egp: 1.95 },
  { name: 'Tier 6', lower_kwh: 650, upper_kwh: 1000, rate_egp: 2.1 },
  { name: 'Tier 7', lower_kwh: 1000, upper_kwh: 0, rate_egp: 2.23 },
]

export const DEFAULT_USD_PER_EGP = 48.5

export const DEFAULT_STATE: AppState = {
  configured: false,
  theme: 'dark',
  language: 'en',
  timezone: 'Africa/Cairo',
  timeFormat: '24h',
  tariffMode: 'tiered',
  flatRate: 2.5,
  tiers: DEFAULT_EGYPTIAN_TIERS,
  usdPerEGP: DEFAULT_USD_PER_EGP,
  spikeThresholdPercent: 30,
  peakStart: '18:00',
  peakEnd: '22:00',
  carbonFactor: 0.85,
  email: '',
  webhookUrl: '',
  geminiKeySource: 'system',
  geminiCustomKey: '',
}

export const STORAGE_KEY = 'ecopulse.settings.v1'

// The custom Gemini key lives in sessionStorage only (cleared when the browser
// session ends), never in localStorage: any XSS on the page could otherwise
// exfiltrate a long-lived key. AppStateContext migrates legacy localStorage
// copies into sessionStorage on startup.
export const GEMINI_KEY_STORAGE = 'ecopulse.gemini.custom-key'
