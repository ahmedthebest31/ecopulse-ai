import { useMemo, useState } from 'react'
import {
  flexRender,
  getCoreRowModel,
  getSortedRowModel,
  useReactTable,
  type ColumnDef,
  type SortingState,
} from '@tanstack/react-table'
import { ChevronDown, ChevronUp, Download } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { AnomalyFlag, TelemetryRecord } from '../types'
import { formatDateTime, formatNumber } from '../lib/format'
import type { AppLanguage } from '../i18n'
import type { TimeFormat } from '../state/appStateDefaults'

interface AnomalyRow {
  id: string
  timestamp: string
  facility_id: string
  facility_name: string
  equipment_id: string
  type: AnomalyFlag
  power_kw: number
  ratio: number
  severity: string
  carbon_kg: number
  is_peak_hour: boolean
}

interface AnomalyTableProps {
  records: TelemetryRecord[]
  timezone: string
  timeFormat: TimeFormat
  language: AppLanguage
}

function buildRows(records: TelemetryRecord[]): AnomalyRow[] {
  return records
    .filter((record) => record.anomaly_flag !== 'none')
    .map((record) => ({
      id: `${record.facility_id}-${record.timestamp}`,
      timestamp: record.timestamp,
      facility_id: record.facility_id,
      facility_name: record.facility_name,
      equipment_id: record.equipment_id || '—',
      type: record.anomaly_flag as AnomalyFlag,
      power_kw: record.power_kw,
      ratio: record.ratio_to_baseline,
      severity: record.anomaly_severity || 'info',
      carbon_kg: record.carbon_kg,
      is_peak_hour: record.is_peak_hour,
    }))
}

function toCsv(rows: AnomalyRow[], headers: string[]): string {
  const escape = (value: string): string => {
    if (/[",\n]/.test(value)) {
      return `"${value.replace(/"/g, '""')}"`
    }
    return value
  }
  const lines = [
    headers.join(','),
    ...rows.map((row) =>
      [
        row.timestamp,
        row.facility_name,
        row.facility_id,
        row.equipment_id,
        row.type,
        row.power_kw.toFixed(3),
        row.ratio.toFixed(3),
        row.severity,
        row.carbon_kg.toFixed(3),
        row.is_peak_hour ? '1' : '0',
      ]
        .map(escape)
        .join(','),
    ),
  ]
  return `\uFEFF${lines.join('\n')}`
}

export function AnomalyTable({ records, timezone, timeFormat, language }: AnomalyTableProps) {
  const { t } = useTranslation()
  const [sorting, setSorting] = useState<SortingState>([{ id: 'timestamp', desc: true }])
  const [facilityFilter, setFacilityFilter] = useState('all')
  const [typeFilter, setTypeFilter] = useState('all')
  const [severityFilter, setSeverityFilter] = useState('all')

  const allRows = useMemo(() => buildRows(records), [records])

  const facilities = useMemo(() => {
    const names = new Map<string, string>()
    for (const record of records) {
      if (!names.has(record.facility_id)) {
        names.set(record.facility_id, record.facility_name)
      }
    }
    return [...names.entries()]
  }, [records])

  const filteredRows = useMemo(() => {
    return allRows.filter(
      (row) =>
        (facilityFilter === 'all' || row.facility_id === facilityFilter) &&
        (typeFilter === 'all' || row.type === typeFilter) &&
        (severityFilter === 'all' || row.severity === severityFilter),
    )
  }, [allRows, facilityFilter, typeFilter, severityFilter])

  const columns = useMemo<ColumnDef<AnomalyRow>[]>(() => {
    const labelSeverity = (severity: string): string => {
      switch (severity) {
        case 'critical':
          return t('dashboard.severityCritical')
        case 'warning':
          return t('dashboard.severityWarning')
        case 'info':
          return t('dashboard.severityInfo')
        default:
          return severity
      }
    }

    const labelType = (type: string): string =>
      type === 'forced_spike' ? t('dashboard.typeSpike') : t('dashboard.typeMicroSurge')

    return [
      {
        accessorKey: 'timestamp',
        header: t('dashboard.colTime'),
        cell: (info) => formatDateTime(info.getValue<string>(), timezone, timeFormat, language),
      },
      {
        accessorKey: 'facility_name',
        header: t('dashboard.colFacility'),
        cell: (info) => {
          const row = info.row.original
          return `${info.getValue<string>()} (${row.facility_id})`
        },
      },
      { accessorKey: 'equipment_id', header: t('dashboard.colEquipment') },
      {
        accessorKey: 'type',
        header: t('dashboard.colType'),
        cell: (info) => {
          const type = info.getValue<string>()
          const isSpike = type === 'forced_spike'
          return (
            <span
              className={`inline-flex rounded-full px-2.5 py-0.5 text-xs font-medium ${
                isSpike
                  ? 'bg-rose-100 text-rose-800 dark:bg-rose-900/60 dark:text-rose-200'
                  : 'bg-amber-100 text-amber-800 dark:bg-amber-900/60 dark:text-amber-200'
              }`}
            >
              {labelType(type)}
            </span>
          )
        },
      },
      {
        accessorKey: 'power_kw',
        header: t('dashboard.colPower'),
        cell: (info) => formatNumber(info.getValue<number>(), language, 3),
      },
      {
        accessorKey: 'ratio',
        header: t('dashboard.colRatio'),
        cell: (info) => formatNumber(info.getValue<number>(), language, 2),
      },
      {
        accessorKey: 'severity',
        header: t('dashboard.colSeverity'),
        cell: (info) => {
          const severity = info.getValue<string>()
          const cls =
            severity === 'critical'
              ? 'bg-rose-100 text-rose-800 dark:bg-rose-900/60 dark:text-rose-200'
              : severity === 'warning'
                ? 'bg-amber-100 text-amber-800 dark:bg-amber-900/60 dark:text-amber-200'
                : 'bg-sky-100 text-sky-800 dark:bg-sky-900/60 dark:text-sky-200'
          return (
            <span className={`inline-flex rounded-full px-2.5 py-0.5 text-xs font-medium ${cls}`}>
              {labelSeverity(severity)}
            </span>
          )
        },
      },
      {
        accessorKey: 'carbon_kg',
        header: t('dashboard.colCarbon'),
        cell: (info) => formatNumber(info.getValue<number>(), language, 2),
      },
      {
        accessorKey: 'is_peak_hour',
        header: t('dashboard.colPeak'),
        cell: (info) => (info.getValue<boolean>() ? t('dashboard.yes') : t('dashboard.no')),
      },
    ]
  }, [t, timezone, timeFormat, language])

  const table = useReactTable({
    data: filteredRows,
    columns,
    state: { sorting },
    onSortingChange: setSorting,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
  })

  const exportCsv = () => {
    const headers = [
      t('dashboard.colTime'),
      t('dashboard.colFacility'),
      'ID',
      t('dashboard.colEquipment'),
      t('dashboard.colType'),
      t('dashboard.colPower'),
      t('dashboard.colRatio'),
      t('dashboard.colSeverity'),
      t('dashboard.colCarbon'),
      t('dashboard.colPeak'),
    ]
    const csv = toCsv(filteredRows, headers)
    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = 'ecopulse-anomalies.csv'
    document.body.appendChild(anchor)
    anchor.click()
    anchor.remove()
    URL.revokeObjectURL(url)
  }

  const selectClass =
    'rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 focus:border-emerald-500 focus:outline-none focus:ring-2 focus:ring-emerald-500 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-100'

  return (
    <section className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm dark:border-slate-800 dark:bg-slate-900">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold text-slate-900 dark:text-slate-50">
            {t('dashboard.anomaliesTitle')}
          </h2>
          <p className="text-sm text-slate-600 dark:text-slate-400">
            {t('dashboard.anomaliesSub', { count: filteredRows.length })}
          </p>
        </div>
        <button
          type="button"
          onClick={exportCsv}
          className="no-print inline-flex h-9 items-center gap-2 rounded-lg border border-slate-300 px-3 text-sm font-medium text-slate-700 transition-colors hover:bg-slate-100 dark:border-slate-700 dark:text-slate-200 dark:hover:bg-slate-800"
        >
          <Download size={15} aria-hidden="true" />
          <span>{t('dashboard.csvExport')}</span>
        </button>
      </div>

      <div className="no-print mb-3 flex flex-wrap gap-3">
        <label className="block text-sm">
          <span className="mb-1 block font-medium text-slate-700 dark:text-slate-300">
            {t('dashboard.filterFacility')}
          </span>
          <select
            value={facilityFilter}
            onChange={(event) => setFacilityFilter(event.target.value)}
            className={selectClass}
          >
            <option value="all">{t('dashboard.facilityAll')}</option>
            {facilities.map(([id, name]) => (
              <option key={id} value={id}>
                {name} ({id})
              </option>
            ))}
          </select>
        </label>

        <label className="block text-sm">
          <span className="mb-1 block font-medium text-slate-700 dark:text-slate-300">
            {t('dashboard.filterType')}
          </span>
          <select
            value={typeFilter}
            onChange={(event) => setTypeFilter(event.target.value)}
            className={selectClass}
          >
            <option value="all">{t('dashboard.typeAll')}</option>
            <option value="forced_spike">{t('dashboard.typeSpike')}</option>
            <option value="micro_surge">{t('dashboard.typeMicroSurge')}</option>
          </select>
        </label>

        <label className="block text-sm">
          <span className="mb-1 block font-medium text-slate-700 dark:text-slate-300">
            {t('dashboard.filterSeverity')}
          </span>
          <select
            value={severityFilter}
            onChange={(event) => setSeverityFilter(event.target.value)}
            className={selectClass}
          >
            <option value="all">{t('dashboard.severityAll')}</option>
            <option value="critical">{t('dashboard.severityCritical')}</option>
            <option value="warning">{t('dashboard.severityWarning')}</option>
            <option value="info">{t('dashboard.severityInfo')}</option>
          </select>
        </label>
      </div>

      {filteredRows.length === 0 ? (
        <p className="py-6 text-center text-sm text-slate-500 dark:text-slate-400">
          {t('dashboard.anomaliesEmpty')}
        </p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full border-collapse text-sm">
            <caption className="sr-only">{t('dashboard.anomaliesTitle')}</caption>
            <thead>
              {table.getHeaderGroups().map((headerGroup) => (
                <tr key={headerGroup.id}>
                  {headerGroup.headers.map((header) => {
                    const sortState = header.column.getIsSorted()
                    const canSort = header.column.getCanSort()
                    return (
                      <th
                        key={header.id}
                        scope="col"
                        aria-sort={sortState === 'asc' ? 'ascending' : sortState === 'desc' ? 'descending' : 'none'}
                        className="border-b border-slate-200 px-3 py-2 text-start text-xs font-semibold uppercase tracking-wide text-slate-600 dark:border-slate-700 dark:text-slate-400"
                      >
                        {canSort ? (
                          <button
                            type="button"
                            onClick={header.column.getToggleSortingHandler()}
                            className="inline-flex items-center gap-1 hover:text-emerald-600 dark:hover:text-emerald-400"
                          >
                            {flexRender(header.column.columnDef.header, header.getContext())}
                            {sortState === 'asc' ? (
                              <ChevronUp size={14} aria-hidden="true" />
                            ) : sortState === 'desc' ? (
                              <ChevronDown size={14} aria-hidden="true" />
                            ) : (
                              <ChevronDown size={14} className="opacity-40" aria-hidden="true" />
                            )}
                          </button>
                        ) : (
                          flexRender(header.column.columnDef.header, header.getContext())
                        )}
                      </th>
                    )
                  })}
                </tr>
              ))}
            </thead>
            <tbody>
              {table.getRowModel().rows.map((row) => (
                <tr
                  key={row.id}
                  className="border-b border-slate-100 last:border-b-0 hover:bg-slate-50 dark:border-slate-800 dark:hover:bg-slate-800/50"
                >
                  {row.getVisibleCells().map((cell) => (
                    <td key={cell.id} className="px-3 py-2 text-slate-800 dark:text-slate-200">
                      {flexRender(cell.column.columnDef.cell, cell.getContext())}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}
