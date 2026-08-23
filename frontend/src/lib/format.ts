import { TREES_ABSORPTION_KG_PER_YEAR } from '../state/appStateDefaults'
import type { AppLanguage } from '../i18n'
import type { TimeFormat } from '../state/appStateDefaults'

export function intlLocale(language: AppLanguage): string {
  return language === 'ar' ? 'ar-EG-u-nu-latn' : 'en-US'
}

export function formatNumber(value: number, language: AppLanguage, digits = 2): string {
  return new Intl.NumberFormat(intlLocale(language), {
    maximumFractionDigits: digits,
  }).format(value)
}

export function formatTime(
  timestamp: string,
  timezone: string,
  timeFormat: TimeFormat,
  language: AppLanguage,
): string {
  const date = new Date(timestamp)
  return new Intl.DateTimeFormat(intlLocale(language), {
    timeZone: timezone,
    hour: '2-digit',
    minute: '2-digit',
    hour12: timeFormat === '12h',
  }).format(date)
}

export function formatDateTime(
  timestamp: string,
  timezone: string,
  timeFormat: TimeFormat,
  language: AppLanguage,
): string {
  const date = new Date(timestamp)
  return new Intl.DateTimeFormat(intlLocale(language), {
    timeZone: timezone,
    month: 'short',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: timeFormat === '12h',
  }).format(date)
}

export function formatHourLabel(
  timestamp: string,
  timezone: string,
  language: AppLanguage,
): string {
  const date = new Date(timestamp)
  return new Intl.DateTimeFormat(intlLocale(language), {
    timeZone: timezone,
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(date)
}

export function timezoneOffsetLabel(timezone: string): string {
  try {
    const parts = new Intl.DateTimeFormat('en-US', {
      timeZone: timezone,
      timeZoneName: 'shortOffset',
    }).formatToParts(new Date())
    const offsetPart = parts.find((part) => part.type === 'timeZoneName')
    if (offsetPart?.value) {
      return offsetPart.value
    }
  } catch {
    // Unknown timezone identifier; fall through to the raw name.
  }
  return timezone
}

export function treesEquivalent(carbonKg: number): number {
  return carbonKg / TREES_ABSORPTION_KG_PER_YEAR
}
