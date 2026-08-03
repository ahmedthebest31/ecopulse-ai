import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import i18n, { setDocumentLanguage } from '../i18n'
import { AppStateContext, type AppStateContextValue } from './useAppState'
import { DEFAULT_STATE, STORAGE_KEY, type AppState } from './appStateDefaults'

function mergeWithDefaults(stored: Partial<AppState>): AppState {
  const base = structuredClone(DEFAULT_STATE)
  return {
    ...base,
    ...stored,
    theme: stored.theme === 'light' || stored.theme === 'dark' ? stored.theme : base.theme,
    language: stored.language === 'en' || stored.language === 'ar' ? stored.language : base.language,
    timeFormat: stored.timeFormat === '12h' || stored.timeFormat === '24h' ? stored.timeFormat : base.timeFormat,
    tariffMode: stored.tariffMode === 'flat' || stored.tariffMode === 'tiered' ? stored.tariffMode : base.tariffMode,
    tiers: Array.isArray(stored.tiers) && stored.tiers.length > 0 ? stored.tiers : base.tiers,
  }
}

function loadState(): AppState {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    if (!raw) {
      return structuredClone(DEFAULT_STATE)
    }
    const parsed: unknown = JSON.parse(raw)
    if (typeof parsed !== 'object' || parsed === null) {
      return structuredClone(DEFAULT_STATE)
    }
    return mergeWithDefaults(parsed as Partial<AppState>)
  } catch {
    return structuredClone(DEFAULT_STATE)
  }
}

export function AppStateProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AppState>(loadState)

  useEffect(() => {
    try {
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify(state))
    } catch {
      // localStorage can be unavailable (private mode); state still works in memory.
    }
  }, [state])

  useEffect(() => {
    document.documentElement.classList.toggle('dark', state.theme === 'dark')
  }, [state.theme])

  useEffect(() => {
    setDocumentLanguage(state.language)
    void i18n.changeLanguage(state.language)
  }, [state.language])

  const update = useCallback((patch: Partial<AppState>) => {
    setState((prev) => ({ ...prev, ...patch }))
  }, [])

  const reset = useCallback(() => {
    setState(structuredClone(DEFAULT_STATE))
  }, [])

  const value = useMemo<AppStateContextValue>(
    () => ({ state, update, reset }),
    [state, update, reset],
  )

  return <AppStateContext.Provider value={value}>{children}</AppStateContext.Provider>
}
