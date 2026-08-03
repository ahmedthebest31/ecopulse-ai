import { useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { Check, ChevronLeft, ChevronRight, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import i18n, { setDocumentLanguage } from '../../i18n'
import { useAppState } from '../../state/useAppState'
import type { AppState } from '../../state/appStateDefaults'
import type { Tier } from '../../types'

interface WizardProps {
  open: boolean
  onClose: () => void
}

const STEP_COUNT = 4

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
const WEBHOOK_RE = /^https?:\/\/.+/i

const inputClass =
  'w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 focus:border-emerald-500 focus:outline-none focus:ring-2 focus:ring-emerald-500 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-100'

const labelClass = 'block text-sm font-medium text-slate-700 dark:text-slate-300'

export function Wizard({ open, onClose }: WizardProps) {
  if (!open) {
    return null
  }
  return <WizardDialog onClose={onClose} />
}

function WizardDialog({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation()
  const { state, update } = useAppState()
  const [draft, setDraft] = useState<AppState>(state)
  const [step, setStep] = useState(0)
  const [error, setError] = useState<string | null>(null)
  const [saved, setSaved] = useState(false)
  const dialogRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    dialogRef.current?.focus()
  }, [])

  useEffect(() => {
    setDocumentLanguage(draft.language)
    void i18n.changeLanguage(draft.language)
  }, [draft.language])

  useEffect(() => {
    return () => {
      setDocumentLanguage(state.language)
      void i18n.changeLanguage(state.language)
    }
  }, [state.language])

  const updateDraft = (patch: Partial<AppState>) => {
    setDraft((prev) => ({ ...prev, ...patch }))
  }

  const setTierUpper = (index: number, value: number) => {
    const next = draft.tiers.map((tier, i) => {
      if (i === index) {
        return { ...tier, upper_kwh: value }
      }
      if (i === index + 1) {
        return { ...tier, lower_kwh: value }
      }
      return tier
    })
    updateDraft({ tiers: next })
  }

  const setTierRate = (index: number, value: number) => {
    const next = draft.tiers.map((tier, i) =>
      i === index ? { ...tier, rate_egp: value } : tier,
    )
    updateDraft({ tiers: next })
  }

  const validateStep = (current: number): string | null => {
    if (current === 1) {
      if (draft.tariffMode === 'flat') {
        if (draft.flatRate <= 0) {
          return t('wizard.errorRates')
        }
      } else {
        let prevUpper = 0
        const count = draft.tiers.length
        for (let i = 0; i < count; i++) {
          const tier = draft.tiers[i]
          if (tier.rate_egp < 0) {
            return t('wizard.errorRates')
          }
          const isLast = i === count - 1
          if (!isLast && (tier.upper_kwh <= 0 || tier.upper_kwh <= prevUpper)) {
            return t('wizard.errorRates')
          }
          if (tier.upper_kwh > 0) {
            prevUpper = tier.upper_kwh
          }
        }
      }
      if (draft.usdPerEGP <= 0) {
        return t('wizard.errorRates')
      }
    }
    if (current === 2) {
      if (draft.spikeThresholdPercent < 0 || draft.carbonFactor <= 0) {
        return t('wizard.errorRates')
      }
    }
    if (current === 3) {
      if (draft.email.trim() !== '' && !EMAIL_RE.test(draft.email.trim())) {
        return t('wizard.errorEmail')
      }
      if (draft.webhookUrl.trim() !== '' && !WEBHOOK_RE.test(draft.webhookUrl.trim())) {
        return t('wizard.errorWebhook')
      }
    }
    return null
  }

  const goNext = () => {
    const validation = validateStep(step)
    if (validation) {
      setError(validation)
      return
    }
    setError(null)
    setStep((value) => Math.min(value + 1, STEP_COUNT - 1))
  }

  const goBack = () => {
    setError(null)
    setStep((value) => Math.max(value - 1, 0))
  }

  const finish = () => {
    const validation = validateStep(step)
    if (validation) {
      setError(validation)
      return
    }
    setError(null)
    update({ ...draft, configured: true })
    setSaved(true)
    window.setTimeout(onClose, 800)
  }

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Escape') {
      event.preventDefault()
      onClose()
      return
    }
    if (event.key === 'Tab') {
      const focusables = dialogRef.current?.querySelectorAll<HTMLElement>(
        'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
      )
      if (!focusables || focusables.length === 0) {
        return
      }
      const first = focusables[0]
      const last = focusables[focusables.length - 1]
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault()
        last.focus()
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault()
        first.focus()
      }
    }
  }

  const stepTitles = [
    t('wizard.s1Title'),
    t('wizard.s2Title'),
    t('wizard.s3Title'),
    t('wizard.s4Title'),
  ]

  const isLastStep = step === STEP_COUNT - 1

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4"
      role="dialog"
      aria-modal="true"
      aria-labelledby="wizard-title"
    >
      <div className="absolute inset-0 bg-slate-950/60" aria-hidden="true" onClick={onClose} />
      <div
        ref={dialogRef}
        tabIndex={-1}
        onKeyDown={handleKeyDown}
        className="relative max-h-[90vh] w-full max-w-2xl overflow-y-auto rounded-xl border border-slate-200 bg-white shadow-2xl outline-none dark:border-slate-700 dark:bg-slate-900"
      >
        <div className="flex items-start justify-between gap-3 border-b border-slate-200 px-5 py-4 dark:border-slate-700">
          <div>
            <h2 id="wizard-title" className="text-lg font-semibold text-slate-900 dark:text-slate-50">
              {t('wizard.title')}
            </h2>
            <p className="text-sm text-slate-600 dark:text-slate-400">{t('wizard.subtitle')}</p>
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label={t('wizard.close')}
            className="inline-flex h-9 w-9 items-center justify-center rounded-lg text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-700 dark:text-slate-400 dark:hover:bg-slate-800"
          >
            <X size={18} aria-hidden="true" />
          </button>
        </div>

        <nav aria-label={t('wizard.title')} className="flex flex-wrap gap-2 border-b border-slate-200 px-5 py-3 dark:border-slate-700">
          {stepTitles.map((title, index) => {
            const isActive = index === step
            const isDone = index < step
            return (
              <button
                key={title}
                type="button"
                onClick={() => {
                  if (index < step) {
                    setError(null)
                    setStep(index)
                  }
                }}
                disabled={index > step}
                aria-current={isActive ? 'step' : undefined}
                className={`inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors ${
                  isActive
                    ? 'bg-emerald-500 text-white'
                    : isDone
                      ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/60 dark:text-emerald-200'
                      : 'text-slate-500 dark:text-slate-400 disabled:opacity-60'
                }`}
              >
                {isDone ? (
                  <Check size={14} aria-hidden="true" />
                ) : (
                  <span aria-hidden="true">{index + 1}</span>
                )}
                {title}
              </button>
            )
          })}
        </nav>

        <div
          className="px-5 py-4"
          role="region"
          aria-label={`${stepTitles[step]} - ${t('wizard.step', { current: step + 1, total: STEP_COUNT })}`}
        >
          <p className="mb-4 text-sm text-slate-500 dark:text-slate-400">
            {t('wizard.step', { current: step + 1, total: STEP_COUNT })} · {stepTitles[step]}
          </p>

          {step === 0 && (
            <div className="space-y-5">
              <fieldset>
                <legend className={labelClass}>{t('wizard.languageLabel')}</legend>
                <div className="mt-2 grid grid-cols-1 gap-3 sm:grid-cols-2">
                  {(['en', 'ar'] as const).map((language) => (
                    <label
                      key={language}
                      className={`flex cursor-pointer items-center gap-2 rounded-lg border p-3 text-sm ${
                        draft.language === language
                          ? 'border-emerald-500 bg-emerald-50 dark:bg-emerald-900/30'
                          : 'border-slate-300 dark:border-slate-700'
                      }`}
                    >
                      <input
                        type="radio"
                        name="language"
                        value={language}
                        checked={draft.language === language}
                        onChange={() => updateDraft({ language })}
                        className="h-4 w-4 accent-emerald-600"
                      />
                      <span className="font-medium text-slate-800 dark:text-slate-200">
                        {language === 'en' ? 'English' : 'العربية'}
                      </span>
                    </label>
                  ))}
                </div>
              </fieldset>

              <fieldset>
                <legend className={labelClass}>{t('wizard.timezoneLabel')}</legend>
                <div className="mt-2">
                  <select
                    value={draft.timezone}
                    onChange={(event) => updateDraft({ timezone: event.target.value })}
                    className={inputClass}
                  >
                    <option value="Africa/Cairo">Africa/Cairo (GMT+3)</option>
                    <option value="UTC">UTC (GMT+0)</option>
                    <option value="Etc/GMT-3">GMT+3 (fixed)</option>
                  </select>
                </div>
              </fieldset>

              <fieldset>
                <legend className={labelClass}>{t('wizard.timeFormatLabel')}</legend>
                <div className="mt-2 grid grid-cols-1 gap-3 sm:grid-cols-2">
                  {(['24h', '12h'] as const).map((format) => (
                    <label
                      key={format}
                      className={`flex cursor-pointer items-center gap-2 rounded-lg border p-3 text-sm ${
                        draft.timeFormat === format
                          ? 'border-emerald-500 bg-emerald-50 dark:bg-emerald-900/30'
                          : 'border-slate-300 dark:border-slate-700'
                      }`}
                    >
                      <input
                        type="radio"
                        name="timeFormat"
                        value={format}
                        checked={draft.timeFormat === format}
                        onChange={() => updateDraft({ timeFormat: format })}
                        className="h-4 w-4 accent-emerald-600"
                      />
                      <span className="font-medium text-slate-800 dark:text-slate-200">
                        {format === '24h' ? t('wizard.time24h') : t('wizard.time12h')}
                      </span>
                    </label>
                  ))}
                </div>
              </fieldset>
            </div>
          )}

          {step === 1 && (
            <div className="space-y-5">
              <fieldset>
                <legend className={labelClass}>{t('wizard.tariffModeLabel')}</legend>
                <div className="mt-2 grid grid-cols-1 gap-3 sm:grid-cols-2">
                  {(['flat', 'tiered'] as const).map((mode) => (
                    <label
                      key={mode}
                      className={`flex cursor-pointer items-start gap-2 rounded-lg border p-3 text-sm ${
                        draft.tariffMode === mode
                          ? 'border-emerald-500 bg-emerald-50 dark:bg-emerald-900/30'
                          : 'border-slate-300 dark:border-slate-700'
                      }`}
                    >
                      <input
                        type="radio"
                        name="tariffMode"
                        value={mode}
                        checked={draft.tariffMode === mode}
                        onChange={() => updateDraft({ tariffMode: mode })}
                        className="mt-0.5 h-4 w-4 accent-emerald-600"
                      />
                      <span>
                        <span className="block font-medium text-slate-800 dark:text-slate-200">
                          {mode === 'flat' ? t('wizard.modeFlat') : t('wizard.modeTiered')}
                        </span>
                        <span className="block text-xs text-slate-500 dark:text-slate-400">
                          {mode === 'flat' ? t('wizard.modeFlatHint') : t('wizard.modeTieredHint')}
                        </span>
                      </span>
                    </label>
                  ))}
                </div>
              </fieldset>

              {draft.tariffMode === 'flat' ? (
                <div className="max-w-xs">
                  <label htmlFor="flat-rate" className={labelClass}>
                    {t('wizard.flatRateLabel')}
                  </label>
                  <input
                    id="flat-rate"
                    type="number"
                    min="0"
                    step="0.01"
                    value={draft.flatRate}
                    onChange={(event) => updateDraft({ flatRate: Number(event.target.value) })}
                    className={`mt-1 ${inputClass}`}
                  />
                </div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <caption className="sr-only">{t('wizard.s2Title')}</caption>
                    <thead>
                      <tr>
                        <th scope="col" className="px-2 py-1.5 text-start text-xs font-semibold uppercase text-slate-600 dark:text-slate-400">
                          {t('wizard.tierName')}
                        </th>
                        <th scope="col" className="px-2 py-1.5 text-start text-xs font-semibold uppercase text-slate-600 dark:text-slate-400">
                          {t('wizard.tierRange')}
                        </th>
                        <th scope="col" className="px-2 py-1.5 text-start text-xs font-semibold uppercase text-slate-600 dark:text-slate-400">
                          {t('wizard.tierUpper')}
                        </th>
                        <th scope="col" className="px-2 py-1.5 text-start text-xs font-semibold uppercase text-slate-600 dark:text-slate-400">
                          {t('wizard.tierRate')}
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {draft.tiers.map((tier: Tier, index: number) => {
                        const isLast = index === draft.tiers.length - 1
                        const rangeLabel = isLast
                          ? `${tier.lower_kwh}+`
                          : index === 0
                            ? `0-${tier.upper_kwh}`
                            : `${tier.lower_kwh + 1}-${tier.upper_kwh}`
                        return (
                          <tr key={tier.name} className="border-t border-slate-200 dark:border-slate-700">
                            <td className="px-2 py-2 text-slate-800 dark:text-slate-200">{tier.name}</td>
                            <td className="px-2 py-2 text-slate-600 dark:text-slate-400">{rangeLabel}</td>
                            <td className="px-2 py-2">
                              <input
                                type="number"
                                min="0"
                                step="1"
                                disabled={isLast}
                                aria-label={`${tier.name} ${t('wizard.tierUpper')}`}
                                value={isLast ? '' : tier.upper_kwh}
                                onChange={(event) =>
                                  setTierUpper(index, event.target.value === '' ? 0 : Number(event.target.value))
                                }
                                className={`w-24 rounded-lg border border-slate-300 bg-white px-2 py-1.5 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-slate-100 disabled:opacity-50 ${inputClass}`}
                              />
                            </td>
                            <td className="px-2 py-2">
                              <input
                                type="number"
                                min="0"
                                step="0.01"
                                aria-label={`${tier.name} ${t('wizard.tierRate')}`}
                                value={tier.rate_egp}
                                onChange={(event) =>
                                  setTierRate(index, event.target.value === '' ? 0 : Number(event.target.value))
                                }
                                className={`w-24 rounded-lg border border-slate-300 bg-white px-2 py-1.5 text-sm dark:border-slate-700 dark:bg-slate-800 dark:text-slate-100 ${inputClass}`}
                              />
                            </td>
                          </tr>
                        )
                      })}
                    </tbody>
                  </table>
                </div>
              )}

              <div className="max-w-xs">
                <label htmlFor="usd-rate" className={labelClass}>
                  {t('wizard.usdPerEGP')}
                </label>
                <input
                  id="usd-rate"
                  type="number"
                  min="0"
                  step="0.01"
                  value={draft.usdPerEGP}
                  onChange={(event) => updateDraft({ usdPerEGP: Number(event.target.value) })}
                  className={`mt-1 ${inputClass}`}
                />
              </div>
            </div>
          )}

          {step === 2 && (
            <div className="space-y-5">
              <div className="max-w-xs">
                <label htmlFor="spike-threshold" className={labelClass}>
                  {t('wizard.spikeThreshold')}
                </label>
                <input
                  id="spike-threshold"
                  type="number"
                  min="0"
                  max="500"
                  step="1"
                  value={draft.spikeThresholdPercent}
                  onChange={(event) =>
                    updateDraft({ spikeThresholdPercent: Number(event.target.value) })
                  }
                  className={`mt-1 ${inputClass}`}
                />
                <p className="mt-1 text-xs text-slate-500 dark:text-slate-400">
                  {t('wizard.spikeThresholdHint')}
                </p>
              </div>

              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div>
                  <label htmlFor="peak-start" className={labelClass}>
                    {t('wizard.peakStart')}
                  </label>
                  <input
                    id="peak-start"
                    type="time"
                    value={draft.peakStart}
                    onChange={(event) => updateDraft({ peakStart: event.target.value })}
                    className={`mt-1 ${inputClass}`}
                  />
                </div>
                <div>
                  <label htmlFor="peak-end" className={labelClass}>
                    {t('wizard.peakEnd')}
                  </label>
                  <input
                    id="peak-end"
                    type="time"
                    value={draft.peakEnd}
                    onChange={(event) => updateDraft({ peakEnd: event.target.value })}
                    className={`mt-1 ${inputClass}`}
                  />
                </div>
              </div>

              <div className="max-w-xs">
                <label htmlFor="carbon-factor" className={labelClass}>
                  {t('wizard.carbonFactor')}
                </label>
                <input
                  id="carbon-factor"
                  type="number"
                  min="0"
                  step="0.01"
                  value={draft.carbonFactor}
                  onChange={(event) => updateDraft({ carbonFactor: Number(event.target.value) })}
                  className={`mt-1 ${inputClass}`}
                />
              </div>
            </div>
          )}

          {step === 3 && (
            <div className="space-y-5">
              <div className="max-w-sm">
                <label htmlFor="notification-email" className={labelClass}>
                  {t('wizard.notificationEmail')}
                  <span className="text-xs font-normal text-slate-500 dark:text-slate-400">
                    {' '}
                    ({t('wizard.notificationEmailHint')})
                  </span>
                </label>
                <input
                  id="notification-email"
                  type="email"
                  autoComplete="email"
                  value={draft.email}
                  onChange={(event) => updateDraft({ email: event.target.value })}
                  className={`mt-1 ${inputClass}`}
                />
              </div>

              <div className="max-w-sm">
                <label htmlFor="notification-webhook" className={labelClass}>
                  {t('wizard.notificationWebhook')}
                  <span className="text-xs font-normal text-slate-500 dark:text-slate-400">
                    {' '}
                    ({t('wizard.notificationWebhookHint')})
                  </span>
                </label>
                <input
                  id="notification-webhook"
                  type="url"
                  autoComplete="url"
                  value={draft.webhookUrl}
                  onChange={(event) => updateDraft({ webhookUrl: event.target.value })}
                  className={`mt-1 ${inputClass}`}
                />
              </div>
            </div>
          )}
        </div>

        <div aria-live="polite">
          {error ? (
            <p role="alert" className="mx-5 mb-3 rounded-lg bg-rose-100 px-3 py-2 text-sm text-rose-800 dark:bg-rose-900/40 dark:text-rose-200">
              {error}
            </p>
          ) : null}
          {saved ? (
            <p role="status" className="mx-5 mb-3 rounded-lg bg-emerald-100 px-3 py-2 text-sm text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-200">
              {t('wizard.success')}
            </p>
          ) : null}
        </div>

        <div className="flex items-center justify-between gap-3 border-t border-slate-200 px-5 py-4 dark:border-slate-700">
          <button
            type="button"
            onClick={onClose}
            className="inline-flex h-9 items-center rounded-lg px-3 text-sm font-medium text-slate-600 transition-colors hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-800"
          >
            {t('wizard.cancel')}
          </button>
          <div className="flex items-center gap-2">
            {step > 0 ? (
              <button
                type="button"
                onClick={goBack}
                className="inline-flex h-9 items-center gap-1.5 rounded-lg border border-slate-300 px-3 text-sm font-medium text-slate-700 transition-colors hover:bg-slate-100 dark:border-slate-700 dark:text-slate-200 dark:hover:bg-slate-800"
              >
                <ChevronLeft size={16} aria-hidden="true" />
                {t('wizard.back')}
              </button>
            ) : null}
            {isLastStep ? (
              <button
                type="button"
                onClick={finish}
                className="inline-flex h-9 items-center gap-1.5 rounded-lg bg-emerald-600 px-4 text-sm font-medium text-white transition-colors hover:bg-emerald-700"
              >
                <Check size={16} aria-hidden="true" />
                {t('wizard.finish')}
              </button>
            ) : (
              <button
                type="button"
                onClick={goNext}
                className="inline-flex h-9 items-center gap-1.5 rounded-lg bg-emerald-600 px-4 text-sm font-medium text-white transition-colors hover:bg-emerald-700"
              >
                {t('wizard.next')}
                <ChevronRight size={16} aria-hidden="true" />
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
