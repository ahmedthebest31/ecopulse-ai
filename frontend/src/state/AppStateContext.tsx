import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import i18n, { setDocumentLanguage } from '../i18n'
import { AppStateContext, type AppStateContextValue } from './useAppState'
import { DEFAULT_STATE, GEMINI_KEY_STORAGE, STORAGE_KEY, type AppState } from './appStateDefaults'

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
    geminiKeySource:
      stored.geminiKeySource === 'system' || stored.geminiKeySource === 'custom'
        ? stored.geminiKeySource
        : base.geminiKeySource,
  }
}

function loadState(): AppState {
  let state = structuredClone(DEFAULT_STATE)
  let legacyKey = ''
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    if (raw) {
      const parsed: unknown = JSON.parse(raw)
      if (typeof parsed === 'object' && parsed !== null) {
        state = mergeWithDefaults(parsed as Partial<AppState>)
        legacyKey =
          typeof (parsed as Partial<AppState>).geminiCustomKey === 'string'
            ? ((parsed as Partial<AppState>).geminiCustomKey ?? '').trim()
            : ''
      }
    }
  } catch {
    state = structuredClone(DEFAULT_STATE)
  }

  // The custom Gemini key is stored in sessionStorage only. Migrate any
  // legacy localStorage copy on first load so the plaintext key disappears
  // from long-lived storage.
  let sessionKey = ''
  try {
    sessionKey = window.sessionStorage.getItem(GEMINI_KEY_STORAGE) ?? ''
  } catch {
    // sessionStorage unavailable; the key simply stays memory-only.
  }
  if (!sessionKey && legacyKey) {
    sessionKey = legacyKey
    try {
      window.sessionStorage.setItem(GEMINI_KEY_STORAGE, sessionKey)
    } catch {
      // Ignore: the in-memory copy still works for this tab.
    }
  }
  state.geminiCustomKey = sessionKey
  return state
}

export function AppStateProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AppState>(loadState)

  useEffect(() => {
    try {
      // Persist everything EXCEPT the custom Gemini key: it must never sit
      // in localStorage (XSS-exfiltratable, survives indefinitely).
      const persistable: Partial<AppState> = { ...state }
      delete persistable.geminiCustomKey
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify(persistable))
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
    // Keep the custom Gemini key in sessionStorage (its only persistent home).
    if (typeof patch.geminiCustomKey === 'string') {
      try {
        if (patch.geminiCustomKey.trim() === '') {
          window.sessionStorage.removeItem(GEMINI_KEY_STORAGE)
        } else {
          window.sessionStorage.setItem(GEMINI_KEY_STORAGE, patch.geminiCustomKey)
        }
      } catch {
        // sessionStorage unavailable; the in-memory state still works for this tab.
      }
    }
  }, [])

  const reset = useCallback(() => {
    try {
      window.sessionStorage.removeItem(GEMINI_KEY_STORAGE)
    } catch {
      // Ignore: resetting to defaults still succeeds in memory.
    }
    setState(structuredClone(DEFAULT_STATE))
  }, [])

  const value = useMemo<AppStateContextValue>(
    () => ({ state, update, reset }),
    [state, update, reset],
  )

  return <AppStateContext.Provider value={value}>{children}</AppStateContext.Provider>
}
