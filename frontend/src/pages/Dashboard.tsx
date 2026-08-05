import { useCallback, useEffect, useMemo, useState } from 'react'
import { Cloud, Leaf, Printer, Receipt, TrendingUp } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { fetchAnalytics, fetchTelemetry, API_BASE_URL } from '../lib/api'
import { calculateTariff } from '../lib/tariff'
import { formatNumber, treesEquivalent } from '../lib/format'
import { useAppState } from '../state/useAppState'
import type { AnalyticsResult, TelemetryRecord } from '../types'
import { AiSummaryCard } from '../components/AiSummaryCard'
import { AnomalyTable } from '../components/AnomalyTable'
import { KpiCards, type KpiItem } from '../components/KpiCards'
import { TelemetryChart } from '../components/TelemetryChart'
import type { HeaderStatus } from '../components/Header'

interface DashboardProps {
  onStatusChange: (status: HeaderStatus, alertCount: number) => void
}

export function Dashboard({ onStatusChange }: DashboardProps) {
  const { t } = useTranslation()
  const { state } = useAppState()
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [records, setRecords] = useState<TelemetryRecord[]>([])
  const [analytics, setAnalytics] = useState<AnalyticsResult | null>(null)
  const [facilityId, setFacilityId] = useState('all')
  const [reloadKey, setReloadKey] = useState(0)

  const load = useCallback(async () => {
    try {
      const [telemetry, analysis] = await Promise.all([fetchTelemetry(), fetchAnalytics()])
      setRecords(telemetry.records)
      setAnalytics(analysis)
    } catch {
      setError(t('dashboard.error', { url: API_BASE_URL }))
    } finally {
      setLoading(false)
    }
  }, [t])

  useEffect(() => {
    void Promise.resolve().then(load)
  }, [load, reloadKey])

  useEffect(() => {
    if (error || records.length === 0) {
      onStatusChange('offline', 0)
      return
    }
    const alertCount = (analytics?.critical_spikes.length ?? 0) + (analytics?.maintenance_alerts.length ?? 0)
    onStatusChange(alertCount > 0 ? 'warning' : 'ok', alertCount)
  }, [error, records.length, analytics, onStatusChange])

  const kpis = useMemo<KpiItem[]>(() => {
    if (!analytics) {
      return []
    }
    const bill = calculateTariff({
      kwh: analytics.total_energy_kwh,
      mode: state.tariffMode,
      flat_rate_egp: state.flatRate,
      tiers: state.tiers,
      usd_per_egp: state.usdPerEGP,
    })
    const trees = Math.ceil(treesEquivalent(analytics.total_carbon_kg))
    const modeLabel = state.tariffMode === 'flat' ? t('dashboard.modeFlat') : t('dashboard.modeTiered')
    return [
      {
        icon: TrendingUp,
        label: t('dashboard.kpiConsumption'),
        sub: t('dashboard.kpiConsumptionSub'),
        value: `${formatNumber(analytics.total_energy_kwh, state.language)} ${t('app.kwhUnit')}`,
        accent: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/60 dark:text-emerald-300',
      },
      {
        icon: Receipt,
        label: t('dashboard.kpiBill'),
        sub: t('dashboard.kpiBillSub', { mode: modeLabel }),
        value: `${formatNumber(bill.total_cost_egp, state.language)} ${t('app.egp')} / ${formatNumber(bill.total_cost_usd, state.language)} ${t('app.usd')}`,
        accent: 'bg-teal-100 text-teal-700 dark:bg-teal-900/60 dark:text-teal-300',
      },
      {
        icon: Cloud,
        label: t('dashboard.kpiCo2'),
        sub: t('dashboard.kpiCo2Sub'),
        value: `${formatNumber(analytics.total_carbon_kg, state.language)} ${t('app.kgUnit')}`,
        accent: 'bg-sky-100 text-sky-700 dark:bg-sky-900/60 dark:text-sky-300',
      },
      {
        icon: Leaf,
        label: t('dashboard.kpiTrees'),
        sub: t('dashboard.kpiTreesSub', { count: formatNumber(trees, state.language, 0) }),
        value: formatNumber(trees, state.language, 0),
        accent: 'bg-lime-100 text-lime-700 dark:bg-lime-900/60 dark:text-lime-300',
      },
    ]
  }, [analytics, state, t])

  const isDark = state.theme === 'dark'

  const printReport = () => {
    const originalTitle = document.title
    const now = new Date()
    const year = now.getFullYear()
    const month = String(now.getMonth() + 1).padStart(2, '0')
    const day = String(now.getDate()).padStart(2, '0')
    document.title = `EcoPulse_AI_Report_${year}-${month}-${day}`
    const restoreTitle = () => {
      document.title = originalTitle
      window.removeEventListener('afterprint', restoreTitle)
    }
    window.addEventListener('afterprint', restoreTitle)
    window.print()
  }

  if (loading) {
    return (
      <p className="py-12 text-center text-slate-600 dark:text-slate-400" role="status">
        {t('dashboard.loading')}
      </p>
    )
  }

  if (error) {
    return (
      <div className="rounded-xl border border-rose-200 bg-rose-50 p-6 text-center dark:border-rose-900 dark:bg-rose-950/40" role="alert">
        <p className="text-sm text-rose-800 dark:text-rose-300">{error}</p>
        <button
          type="button"
          onClick={() => {
            setLoading(true)
            setError(null)
            setReloadKey((value) => value + 1)
          }}
          className="no-print mt-4 inline-flex h-9 items-center rounded-lg bg-emerald-600 px-4 text-sm font-medium text-white transition-colors hover:bg-emerald-700"
        >
          {t('dashboard.retry')}
        </button>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div className="no-print flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-50">
          {t('app.brand')} · {t('app.tagline')}
        </h1>
        <button
          type="button"
          onClick={printReport}
          className="inline-flex h-9 items-center gap-2 rounded-lg border border-slate-300 px-3 text-sm font-medium text-slate-700 transition-colors hover:bg-slate-100 dark:border-slate-700 dark:text-slate-200 dark:hover:bg-slate-800"
        >
          <Printer size={15} aria-hidden="true" />
          <span>{t('app.print')}</span>
          <span className="sr-only">{t('app.printHint')}</span>
        </button>
      </div>

      <KpiCards items={kpis} />

      <div className="grid grid-cols-1 gap-6 xl:grid-cols-3">
        <section className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm dark:border-slate-800 dark:bg-slate-900 xl:col-span-2">
          <div className="mb-3">
            <h2 className="text-base font-semibold text-slate-900 dark:text-slate-50">
              {t('dashboard.chartTitle')}
            </h2>
            <p className="text-sm text-slate-600 dark:text-slate-400">{t('dashboard.chartSub')}</p>
          </div>
          <TelemetryChart
            records={records}
            facilityId={facilityId}
            onFacilityChange={setFacilityId}
            timezone={state.timezone}
            language={state.language}
            isDark={isDark}
          />
        </section>

        <AiSummaryCard />
      </div>

      <AnomalyTable
        records={records}
        timezone={state.timezone}
        timeFormat={state.timeFormat}
        language={state.language}
      />
    </div>
  )
}
