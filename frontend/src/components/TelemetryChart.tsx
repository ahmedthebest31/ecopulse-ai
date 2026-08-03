import { useMemo } from 'react'
import {
  CartesianGrid,
  ComposedChart,
  Legend,
  Line,
  ResponsiveContainer,
  Scatter,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import { useTranslation } from 'react-i18next'
import type { TelemetryRecord } from '../types'
import { formatHourLabel, formatNumber } from '../lib/format'
import type { AppLanguage } from '../i18n'

interface ChartPoint {
  time: string
  power: number
  baseline: number
  spike: number | null
  micro: number | null
}

interface TelemetryChartProps {
  records: TelemetryRecord[]
  facilityId: string
  onFacilityChange: (facilityId: string) => void
  timezone: string
  language: AppLanguage
  isDark: boolean
}

function buildChartData(records: TelemetryRecord[], facilityId: string): ChartPoint[] {
  if (facilityId !== 'all') {
    return records
      .filter((record) => record.facility_id === facilityId)
      .map((record) => ({
        time: record.timestamp,
        power: record.power_kw,
        baseline: record.baseline_kw,
        spike: record.anomaly_flag === 'forced_spike' ? record.power_kw : null,
        micro: record.anomaly_flag === 'micro_surge' ? record.power_kw : null,
      }))
  }
  const byTime = new Map<string, ChartPoint>()
  for (const record of records) {
    let point = byTime.get(record.timestamp)
    if (!point) {
      point = { time: record.timestamp, power: 0, baseline: 0, spike: null, micro: null }
      byTime.set(record.timestamp, point)
    }
    point.power += record.power_kw
    point.baseline += record.baseline_kw
    if (record.anomaly_flag === 'forced_spike') {
      point.spike = point.power
    }
    if (record.anomaly_flag === 'micro_surge') {
      point.micro = point.power
    }
  }
  return [...byTime.values()]
}

export function TelemetryChart({
  records,
  facilityId,
  onFacilityChange,
  timezone,
  language,
  isDark,
}: TelemetryChartProps) {
  const { t } = useTranslation()

  const facilities = useMemo(() => {
    const names = new Map<string, string>()
    for (const record of records) {
      if (!names.has(record.facility_id)) {
        names.set(record.facility_id, record.facility_name)
      }
    }
    return [...names.entries()]
  }, [records])

  const data = useMemo(
    () => buildChartData(records, facilityId),
    [records, facilityId],
  )

  const tooltipStyle = {
    backgroundColor: isDark ? '#0f172a' : '#ffffff',
    border: isDark ? '1px solid #334155' : '1px solid #cbd5e1',
    color: isDark ? '#f1f5f9' : '#0f172a',
    fontSize: '13px',
  }

  return (
    <div>
      <div className="mb-3">
        <label className="block text-sm font-medium text-slate-700 dark:text-slate-300" htmlFor="facility-select">
          {t('dashboard.filterFacility')}
        </label>
        <select
          id="facility-select"
          value={facilityId}
          onChange={(event) => onFacilityChange(event.target.value)}
          className="mt-1 w-full max-w-xs rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 focus:border-emerald-500 focus:outline-none focus:ring-2 focus:ring-emerald-500 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-100"
        >
          <option value="all">{t('dashboard.facilityAll')}</option>
          {facilities.map(([id, name]) => (
            <option key={id} value={id}>
              {name} ({id})
            </option>
          ))}
        </select>
      </div>

      <div dir="ltr" role="img" aria-label={`${t('dashboard.chartTitle')}. ${data.length} minutes of power data`}>
        <ResponsiveContainer width="100%" height={320}>
          <ComposedChart data={data} margin={{ top: 8, right: 8, bottom: 8, left: 0 }}>
            <CartesianGrid strokeDasharray="3 3" stroke={isDark ? '#334155' : '#cbd5e1'} />
            <XAxis
              dataKey="time"
              tickFormatter={(value: string) => formatHourLabel(value, timezone, language)}
              interval={60}
              stroke={isDark ? '#94a3b8' : '#475569'}
              tick={{ fontSize: 12 }}
            />
            <YAxis
              stroke={isDark ? '#94a3b8' : '#475569'}
              tick={{ fontSize: 12 }}
              tickFormatter={(value: number) => formatNumber(value, language, 0)}
            />
            <Tooltip
              contentStyle={tooltipStyle}
              formatter={(value, name) => [formatNumber(Number(value), language, 2), String(name)]}
              labelFormatter={(value) => formatHourLabel(String(value), timezone, language)}
            />
            <Legend wrapperStyle={{ fontSize: '13px' }} />
            <Line
              type="monotone"
              dataKey="power"
              name={t('dashboard.chartActual')}
              stroke="#10b981"
              strokeWidth={2}
              dot={false}
              isAnimationActive={false}
            />
            <Line
              type="monotone"
              dataKey="baseline"
              name={t('dashboard.chartBaseline')}
              stroke="#94a3b8"
              strokeDasharray="4 4"
              dot={false}
              isAnimationActive={false}
            />
            <Scatter
              dataKey="spike"
              name={t('dashboard.chartSpike')}
              fill="#dc2626"
              isAnimationActive={false}
            />
            <Scatter
              dataKey="micro"
              name={t('dashboard.chartMicro')}
              fill="#f59e0b"
              isAnimationActive={false}
            />
          </ComposedChart>
        </ResponsiveContainer>
      </div>
    </div>
  )
}
