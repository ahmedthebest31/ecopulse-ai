import { useTranslation } from 'react-i18next'
import type { GeminiKeySource } from '../state/appStateDefaults'

const GEMINI_KEY_URL = 'https://aistudio.google.com/app/apikey'

interface GeminiKeyFieldProps {
  hasValidEnvKey: boolean
  source: GeminiKeySource
  customKey: string
  onChange: (patch: { source: GeminiKeySource; customKey: string }) => void
}

const labelClass = 'block text-sm font-medium text-slate-700 dark:text-slate-300'

const inputClass =
  'w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 focus:border-emerald-500 focus:outline-none focus:ring-2 focus:ring-emerald-500 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-100'

export function GeminiKeyField({ hasValidEnvKey, source, customKey, onChange }: GeminiKeyFieldProps) {
  const { t } = useTranslation()
  const effectiveSource: GeminiKeySource = hasValidEnvKey ? source : 'custom'

  const pickSource = (next: GeminiKeySource) => {
    onChange({ source: hasValidEnvKey ? next : 'custom', customKey })
  }

  const updateKey = (next: string) => {
    onChange({ source: 'custom', customKey: next })
  }

  return (
    <fieldset>
      <legend className={labelClass}>{t('wizard.geminiLabel')}</legend>

      {hasValidEnvKey ? (
        <div className="mt-2 grid grid-cols-1 gap-3 sm:grid-cols-2">
          <label
            className={`flex cursor-pointer items-center gap-2 rounded-lg border p-3 text-sm ${
              effectiveSource === 'system'
                ? 'border-emerald-500 bg-emerald-50 dark:bg-emerald-900/30'
                : 'border-slate-300 dark:border-slate-700'
            }`}
          >
            <input
              type="radio"
              name="gemini-key-source"
              value="system"
              checked={effectiveSource === 'system'}
              onChange={() => pickSource('system')}
              className="h-4 w-4 accent-emerald-600"
            />
            <span className="font-medium text-slate-800 dark:text-slate-200">
              {t('wizard.geminiUseSystem')}
            </span>
          </label>
          <label
            className={`flex cursor-pointer items-center gap-2 rounded-lg border p-3 text-sm ${
              effectiveSource === 'custom'
                ? 'border-emerald-500 bg-emerald-50 dark:bg-emerald-900/30'
                : 'border-slate-300 dark:border-slate-700'
            }`}
          >
            <input
              type="radio"
              name="gemini-key-source"
              value="custom"
              checked={effectiveSource === 'custom'}
              onChange={() => pickSource('custom')}
              className="h-4 w-4 accent-emerald-600"
            />
            <span className="font-medium text-slate-800 dark:text-slate-200">
              {t('wizard.geminiUseCustom')}
            </span>
          </label>
        </div>
      ) : null}

      {hasValidEnvKey && effectiveSource === 'system' ? (
        <p className="mt-2 text-xs text-slate-500 dark:text-slate-400">{t('wizard.geminiHint')}</p>
      ) : (
        <div className="mt-2 max-w-lg">
          <label htmlFor="gemini-custom-key" className={labelClass}>
            {t('wizard.geminiCustomLabel')}
          </label>
          <div className="mt-1 flex flex-wrap items-center gap-2">
            <input
              id="gemini-custom-key"
              type="password"
              autoComplete="off"
              value={customKey}
              onChange={(event) => updateKey(event.target.value)}
              className={inputClass}
            />
            <a
              href={GEMINI_KEY_URL}
              target="_blank"
              rel="noopener noreferrer"
              className="shrink-0 text-sm text-emerald-700 underline hover:text-emerald-800 dark:text-emerald-400 dark:hover:text-emerald-300"
            >
              {GEMINI_KEY_URL}
            </a>
          </div>
        </div>
      )}
    </fieldset>
  )
}
