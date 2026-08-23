import { useEffect, useState } from 'react'
import { fetchGeminiStatus } from './api'

export interface GeminiStatus {
  hasValidEnvKey: boolean
  loading: boolean
}

export function useGeminiStatus(): GeminiStatus {
  const [status, setStatus] = useState<GeminiStatus>({
    hasValidEnvKey: false,
    loading: true,
  })

  useEffect(() => {
    let cancelled = false
    fetchGeminiStatus()
      .then((result) => {
        if (!cancelled) {
          setStatus({ hasValidEnvKey: result.has_valid_env_key, loading: false })
        }
      })
      .catch(() => {
        if (!cancelled) {
          setStatus({ hasValidEnvKey: false, loading: false })
        }
      })
    return () => {
      cancelled = true
    }
  }, [])

  return status
}
