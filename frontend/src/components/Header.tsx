import { Moon, Settings2, Sun, Zap } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useAppState } from '../state/useAppState'

export type HeaderStatus = 'ok' | 'warning' | 'offline'

interface HeaderProps {
  status: HeaderStatus
  alertCount: number
  onOpenSettings: () => void
}

export function Header({ status, alertCount, onOpenSettings }: HeaderProps) {
  const { t } = useTranslation()
  const { state, update } = useAppState()
  const isDark = state.theme === 'dark'
  const isArabic = state.language === 'ar'

  const statusLabel =
    status === 'ok'
      ? t('app.statusOperational')
      : status === 'warning'
        ? t('app.statusAttention')
        : t('app.statusOffline')

  const statusBadge =
    status === 'ok'
      ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/60 dark:text-emerald-200'
      : status === 'warning'
        ? 'bg-amber-100 text-amber-800 dark:bg-amber-900/60 dark:text-amber-200'
        : 'bg-rose-100 text-rose-800 dark:bg-rose-900/60 dark:text-rose-200'

  return (
    <header className="no-print sticky top-0 z-40 border-b border-slate-200 bg-white/90 backdrop-blur dark:border-slate-800 dark:bg-slate-900/90">
      <div className="mx-auto flex max-w-7xl flex-wrap items-center gap-x-4 gap-y-2 px-4 py-3">
        <div className="flex items-center gap-2">
          <span
            className="flex h-9 w-9 items-center justify-center rounded-lg bg-emerald-500 text-white"
            aria-hidden="true"
          >
            <Zap size={20} />
          </span>
          <div>
            <p className="text-base font-semibold leading-tight text-slate-900 dark:text-slate-50">
              {t('app.brand')}
            </p>
            <p className="text-xs leading-tight text-slate-600 dark:text-slate-400">
              {t('app.tagline')}
            </p>
          </div>
        </div>

        <span
          className={`inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-sm font-medium ${statusBadge}`}
        >
          <span aria-hidden="true">●</span>
          {statusLabel}
          {status === 'warning' && alertCount > 0 ? ` (${alertCount})` : ''}
        </span>

        <div className="ms-auto flex flex-wrap items-center gap-1.5">
          <div className="flex rounded-lg border border-slate-300 p-0.5 dark:border-slate-700" role="group" aria-label={t('app.language')}>
            <button
              type="button"
              aria-pressed={!isArabic}
              onClick={() => update({ language: 'en' })}
              className={`rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${
                !isArabic
                  ? 'bg-emerald-500 text-white'
                  : 'text-slate-600 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-800'
              }`}
            >
              EN
            </button>
            <button
              type="button"
              aria-pressed={isArabic}
              onClick={() => update({ language: 'ar' })}
              className={`rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${
                isArabic
                  ? 'bg-emerald-500 text-white'
                  : 'text-slate-600 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-800'
              }`}
            >
              العربية
            </button>
          </div>

          <button
            type="button"
            aria-pressed={isDark}
            aria-label={isDark ? t('app.themeLight') : t('app.themeDark')}
            onClick={() => update({ theme: isDark ? 'light' : 'dark' })}
            className="inline-flex h-9 items-center gap-2 rounded-lg border border-slate-300 px-3 text-sm font-medium text-slate-700 transition-colors hover:bg-slate-100 dark:border-slate-700 dark:text-slate-200 dark:hover:bg-slate-800"
          >
            {isDark ? <Sun size={16} aria-hidden="true" /> : <Moon size={16} aria-hidden="true" />}
            <span className="sr-only">{isDark ? t('app.themeLight') : t('app.themeDark')}</span>
          </button>

          <span className="inline-flex h-9 items-center gap-1.5 rounded-lg border border-slate-300 px-3 text-sm text-slate-600 dark:border-slate-700 dark:text-slate-400">
            <span aria-hidden="true">GMT+3</span>
            <span className="sr-only">{t('app.tagline')} · GMT+3</span>
          </span>

          <button
            type="button"
            onClick={onOpenSettings}
            className="inline-flex h-9 items-center gap-2 rounded-lg border border-slate-300 px-3 text-sm font-medium text-slate-700 transition-colors hover:bg-slate-100 dark:border-slate-700 dark:text-slate-200 dark:hover:bg-slate-800"
          >
            <Settings2 size={16} aria-hidden="true" />
            <span>{t('app.openSettings')}</span>
          </button>
        </div>
      </div>
    </header>
  )
}
