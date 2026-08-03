import { createContext, useContext } from 'react'
import type { AppState } from './appStateDefaults'

export interface AppStateContextValue {
  state: AppState
  update: (patch: Partial<AppState>) => void
  reset: () => void
}

export const AppStateContext = createContext<AppStateContextValue | null>(null)

export function useAppState(): AppStateContextValue {
  const ctx = useContext(AppStateContext)
  if (!ctx) {
    throw new Error('useAppState must be used within an AppStateProvider')
  }
  return ctx
}
