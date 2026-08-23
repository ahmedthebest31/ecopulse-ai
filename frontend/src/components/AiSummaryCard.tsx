import { useCallback, useEffect, useState } from 'react'
import { RefreshCw, Sparkles } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { postReport } from '../lib/api'
import { useAppState } from '../state/useAppState'
import type { AppLanguage } from '../i18n'
import type { ReportResult } from '../types'

export function AiSummaryCard() {
  const { t } = useTranslation()
  const { state } = useAppState()
  const [results, setResults] = useState<Partial<Record<AppLanguage, ReportResult>>>({})
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  const customKey =
    state.geminiKeySource === 'custom' && state.geminiCustomKey.trim() !== ''
      ? state.geminiCustomKey.trim()
      : undefined

  // Per-locale cache: switching language serves the stored summary instead of
  // silently re-calling Gemini and burning quota. The early-return path makes
  // no state updates, so the post-fetch effect re-run settles immediately.
  const load = useCallback(
    async (locale: AppLanguage, force = false) => {
      if (!force && results[locale]) {
        return
      }
      setLoading(true)
      setError(false)
      try {
        const response = await postReport(locale, customKey)
        setResults((prev) => ({ ...prev, [locale]: response }))
      } catch (err) {
        console.error('AI summary request failed', err)
        setError(true)
      } finally {
        setLoading(false)
      }
    },
    [customKey, results],
  )

  useEffect(() => {
    void Promise.resolve().then(() => void load(state.language))
  }, [load, state.language])

  const result = results[state.language]

  return (
    <section className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm dark:border-slate-800 dark:bg-slate-900">
      <div className="mb-2 flex items-center justify-between gap-3">
        <h2 className="flex items-center gap-2 text-base font-semibold text-slate-900 dark:text-slate-50">
          <Sparkles size={18} className="text-emerald-500" aria-hidden="true" />
          {t('dashboard.aiTitle')}
        </h2>
        <button
          type="button"
          onClick={() => {
            void load(state.language, true)
          }}
          disabled={loading}
          className="no-print inline-flex h-9 items-center gap-2 rounded-lg border border-slate-300 px-3 text-sm font-medium text-slate-700 transition-colors hover:bg-slate-100 disabled:cursor-not-allowed disabled:opacity-60 dark:border-slate-700 dark:text-slate-200 dark:hover:bg-slate-800"
        >
          <RefreshCw size={15} className={loading ? 'animate-spin' : ''} aria-hidden="true" />
          <span>{t('dashboard.aiRefresh')}</span>
        </button>
      </div>

      <div aria-live="polite">
        {loading && !result ? (
          <p className="text-sm text-slate-500 dark:text-slate-400">{t('dashboard.loading')}</p>
        ) : error ? (
          <p className="text-sm text-rose-700 dark:text-rose-400">{t('dashboard.aiError')}</p>
        ) : result ? (
          <div>
            <p className="whitespace-pre-line text-sm leading-relaxed text-slate-800 dark:text-slate-200">
              {result.summary}
            </p>
            <div className="mt-3 flex flex-wrap items-center gap-2 text-xs">
              <span className="inline-flex items-center gap-1.5 rounded-full bg-emerald-100 px-2.5 py-1 font-medium text-emerald-800 dark:bg-emerald-900/60 dark:text-emerald-200">
                {result.source === 'gemini' ? t('dashboard.aiSourceGemini') : t('dashboard.aiSourceFallback')}
              </span>
              {result.warning ? (
                <span className="text-amber-700 dark:text-amber-400">
                  {t('dashboard.aiWarning', { warning: result.warning })}
                </span>
              ) : null}
            </div>
          </div>
        ) : null}
      </div>
    </section>
  )
}
