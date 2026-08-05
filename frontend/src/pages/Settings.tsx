import { ArrowLeft, RotateCcw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useAppState } from '../state/useAppState'
import type { Tier } from '../types'
import { GeminiKeyField } from '../components/GeminiKeyField'
import { useGeminiStatus } from '../lib/geminiStatus'
import { isValidEmail, isValidWebhookUrl } from '../lib/validation'

interface SettingsProps {
  onBack: () => void
  onRunWizard: () => void
}

const inputClass =
  'w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 focus:border-emerald-500 focus:outline-none focus:ring-2 focus:ring-emerald-500 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-100'

const labelClass = 'block text-sm font-medium text-slate-700 dark:text-slate-300'

const sectionClass = 'rounded-xl border border-slate-200 bg-white p-4 shadow-sm dark:border-slate-800 dark:bg-slate-900'

const sectionTitleClass = 'text-base font-semibold text-slate-900 dark:text-slate-50'

const radioClass = (selected: boolean) =>
  `flex cursor-pointer items-center gap-2 rounded-lg border p-3 text-sm ${
    selected
      ? 'border-emerald-500 bg-emerald-50 dark:bg-emerald-900/30'
      : 'border-slate-300 dark:border-slate-700'
  }`

function errorBorder(hasError: boolean) {
  return hasError
    ? ' border-rose-500 focus:border-rose-500 focus:ring-rose-500 dark:border-rose-500'
    : ''
}

export function Settings({ onBack, onRunWizard }: SettingsProps) {
  const { t } = useTranslation()
  const { state, update } = useAppState()
  const { hasValidEnvKey } = useGeminiStatus()

  const emailError =
    state.email.trim() !== '' && !isValidEmail(state.email) ? t('wizard.errorEmail') : null
  const webhookError =
    state.webhookUrl.trim() !== '' && !isValidWebhookUrl(state.webhookUrl)
      ? t('wizard.errorWebhook')
      : null

  const setTierUpper = (index: number, value: number) => {
    const next = state.tiers.map((tier, i) => {
      if (i === index) {
        return { ...tier, upper_kwh: value }
      }
      if (i === index + 1) {
        return { ...tier, lower_kwh: value }
      }
      return tier
    })
    update({ tiers: next })
  }

  const setTierRate = (index: number, value: number) => {
    const next = state.tiers.map((tier, i) =>
      i === index ? { ...tier, rate_egp: value } : tier,
    )
    update({ tiers: next })
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-50">
            {t('settings.title')}
          </h1>
          <p className="text-sm text-slate-600 dark:text-slate-400">{t('settings.subtitle')}</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <button
            type="button"
            onClick={onRunWizard}
            className="inline-flex h-9 items-center gap-2 rounded-lg bg-emerald-600 px-3 text-sm font-medium text-white transition-colors hover:bg-emerald-700"
          >
            <RotateCcw size={15} aria-hidden="true" />
            <span>{t('settings.runWizard')}</span>
          </button>
          <button
            type="button"
            onClick={onBack}
            className="inline-flex h-9 items-center gap-2 rounded-lg border border-slate-300 px-3 text-sm font-medium text-slate-700 transition-colors hover:bg-slate-100 dark:border-slate-700 dark:text-slate-200 dark:hover:bg-slate-800"
          >
            <ArrowLeft size={15} className="rtl:rotate-180" aria-hidden="true" />
            <span>{t('settings.back')}</span>
          </button>
        </div>
      </div>

      <p role="status" className="text-xs text-slate-500 dark:text-slate-400">
        {t('settings.autoSaved')}
      </p>

      <section className={sectionClass} aria-labelledby="settings-regional-title">
        <h2 id="settings-regional-title" className={sectionTitleClass}>
          {t('settings.sectionRegional')}
        </h2>
        <div className="mt-4 space-y-5">
          <fieldset>
            <legend className={labelClass}>{t('wizard.languageLabel')}</legend>
            <div className="mt-2 grid grid-cols-1 gap-3 sm:grid-cols-2">
              {(['en', 'ar'] as const).map((language) => (
                <label key={language} className={radioClass(state.language === language)}>
                  <input
                    type="radio"
                    name="settings-language"
                    value={language}
                    checked={state.language === language}
                    onChange={() => update({ language })}
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
            <div className="mt-2 max-w-xs">
              <select
                value={state.timezone}
                onChange={(event) => update({ timezone: event.target.value })}
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
                <label key={format} className={radioClass(state.timeFormat === format)}>
                  <input
                    type="radio"
                    name="settings-time-format"
                    value={format}
                    checked={state.timeFormat === format}
                    onChange={() => update({ timeFormat: format })}
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
      </section>

      <section className={sectionClass} aria-labelledby="settings-tariff-title">
        <h2 id="settings-tariff-title" className={sectionTitleClass}>
          {t('settings.sectionTariff')}
        </h2>
        <div className="mt-4 space-y-5">
          <fieldset>
            <legend className={labelClass}>{t('wizard.tariffModeLabel')}</legend>
            <div className="mt-2 grid grid-cols-1 gap-3 sm:grid-cols-2">
              {(['flat', 'tiered'] as const).map((mode) => (
                <label key={mode} className={radioClass(state.tariffMode === mode)}>
                  <input
                    type="radio"
                    name="settings-tariff-mode"
                    value={mode}
                    checked={state.tariffMode === mode}
                    onChange={() => update({ tariffMode: mode })}
                    className="h-4 w-4 accent-emerald-600"
                  />
                  <span className="font-medium text-slate-800 dark:text-slate-200">
                    {mode === 'flat' ? t('wizard.modeFlat') : t('wizard.modeTiered')}
                  </span>
                </label>
              ))}
            </div>
          </fieldset>

          {state.tariffMode === 'flat' ? (
            <div className="max-w-xs">
              <label htmlFor="settings-flat-rate" className={labelClass}>
                {t('wizard.flatRateLabel')}
              </label>
              <input
                id="settings-flat-rate"
                type="number"
                min="0"
                step="0.01"
                value={state.flatRate}
                onChange={(event) => update({ flatRate: Number(event.target.value) })}
                className={`mt-1 ${inputClass}`}
              />
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <caption className="sr-only">{t('settings.sectionTariff')}</caption>
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
                  {state.tiers.map((tier: Tier, index: number) => {
                    const isLast = index === state.tiers.length - 1
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
            <label htmlFor="settings-usd-rate" className={labelClass}>
              {t('wizard.usdPerEGP')}
            </label>
            <input
              id="settings-usd-rate"
              type="number"
              min="0"
              step="0.01"
              value={state.usdPerEGP}
              onChange={(event) => update({ usdPerEGP: Number(event.target.value) })}
              className={`mt-1 ${inputClass}`}
            />
          </div>
        </div>
      </section>

      <section className={sectionClass} aria-labelledby="settings-thresholds-title">
        <h2 id="settings-thresholds-title" className={sectionTitleClass}>
          {t('settings.sectionThresholds')}
        </h2>
        <div className="mt-4 space-y-5">
          <div className="max-w-xs">
            <label htmlFor="settings-spike-threshold" className={labelClass}>
              {t('wizard.spikeThreshold')}
            </label>
            <input
              id="settings-spike-threshold"
              type="number"
              min="0"
              max="500"
              step="1"
              value={state.spikeThresholdPercent}
              onChange={(event) =>
                update({ spikeThresholdPercent: Number(event.target.value) })
              }
              className={`mt-1 ${inputClass}`}
            />
            <p className="mt-1 text-xs text-slate-500 dark:text-slate-400">
              {t('wizard.spikeThresholdHint')}
            </p>
          </div>

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div>
              <label htmlFor="settings-peak-start" className={labelClass}>
                {t('wizard.peakStart')}
              </label>
              <input
                id="settings-peak-start"
                type="time"
                value={state.peakStart}
                onChange={(event) => update({ peakStart: event.target.value })}
                className={`mt-1 ${inputClass}`}
              />
            </div>
            <div>
              <label htmlFor="settings-peak-end" className={labelClass}>
                {t('wizard.peakEnd')}
              </label>
              <input
                id="settings-peak-end"
                type="time"
                value={state.peakEnd}
                onChange={(event) => update({ peakEnd: event.target.value })}
                className={`mt-1 ${inputClass}`}
              />
            </div>
          </div>

          <div className="max-w-xs">
            <label htmlFor="settings-carbon-factor" className={labelClass}>
              {t('wizard.carbonFactor')}
            </label>
            <input
              id="settings-carbon-factor"
              type="number"
              min="0"
              step="0.01"
              value={state.carbonFactor}
              onChange={(event) => update({ carbonFactor: Number(event.target.value) })}
              className={`mt-1 ${inputClass}`}
            />
          </div>
        </div>
      </section>

      <section className={sectionClass} aria-labelledby="settings-notifications-title">
        <h2 id="settings-notifications-title" className={sectionTitleClass}>
          {t('settings.sectionNotifications')}
        </h2>
        <div className="mt-4 space-y-5">
          <div className="max-w-sm">
            <label htmlFor="settings-notification-email" className={labelClass}>
              {t('wizard.notificationEmail')}
              <span className="text-xs font-normal text-slate-500 dark:text-slate-400">
                {' '}
                ({t('wizard.notificationEmailHint')})
              </span>
            </label>
            <input
              id="settings-notification-email"
              type="email"
              autoComplete="email"
              value={state.email}
              onChange={(event) => update({ email: event.target.value })}
              aria-invalid={emailError ? true : undefined}
              aria-describedby={emailError ? 'settings-email-error' : undefined}
              className={`mt-1 ${inputClass}${errorBorder(emailError !== null)}`}
            />
            {emailError ? (
              <p
                id="settings-email-error"
                role="alert"
                className="mt-1 text-xs font-medium text-rose-600 dark:text-rose-400"
              >
                {emailError}
              </p>
            ) : null}
          </div>

          <div className="max-w-sm">
            <label htmlFor="settings-notification-webhook" className={labelClass}>
              {t('wizard.notificationWebhook')}
              <span className="text-xs font-normal text-slate-500 dark:text-slate-400">
                {' '}
                ({t('wizard.notificationWebhookHint')})
              </span>
            </label>
            <input
              id="settings-notification-webhook"
              type="url"
              autoComplete="url"
              value={state.webhookUrl}
              onChange={(event) => update({ webhookUrl: event.target.value })}
              aria-invalid={webhookError ? true : undefined}
              aria-describedby={webhookError ? 'settings-webhook-error' : undefined}
              className={`mt-1 ${inputClass}${errorBorder(webhookError !== null)}`}
            />
            {webhookError ? (
              <p
                id="settings-webhook-error"
                role="alert"
                className="mt-1 text-xs font-medium text-rose-600 dark:text-rose-400"
              >
                {webhookError}
              </p>
            ) : null}
          </div>
        </div>
      </section>

      <section className={sectionClass} aria-labelledby="settings-integrations-title">
        <h2 id="settings-integrations-title" className={sectionTitleClass}>
          {t('settings.sectionIntegrations')}
        </h2>
        <div className="mt-4">
          <GeminiKeyField
            hasValidEnvKey={hasValidEnvKey}
            source={state.geminiKeySource}
            customKey={state.geminiCustomKey}
            onChange={(patch) =>
              update({
                geminiKeySource: patch.source,
                geminiCustomKey: patch.customKey,
              })
            }
          />
        </div>
      </section>
    </div>
  )
}
