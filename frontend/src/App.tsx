import { useCallback, useState } from 'react'
import { Header, type HeaderStatus } from './components/Header'
import { Wizard } from './components/wizard/Wizard'
import { Dashboard } from './pages/Dashboard'
import { Settings } from './pages/Settings'
import { AppStateProvider } from './state/AppStateContext'
import { useAppState } from './state/useAppState'

type Page = 'dashboard' | 'settings'

function Shell() {
  const { state } = useAppState()
  const [wizardOpen, setWizardOpen] = useState(() => state.configured === false)
  const [page, setPage] = useState<Page>('dashboard')
  const [status, setStatus] = useState<HeaderStatus>('ok')
  const [alertCount, setAlertCount] = useState(0)

  const handleStatusChange = useCallback((next: HeaderStatus, count: number) => {
    setStatus(next)
    setAlertCount(count)
  }, [])

  const openWizard = useCallback(() => setWizardOpen(true), [])
  const closeWizard = useCallback(() => setWizardOpen(false), [])

  return (
    <>
      <Header status={status} alertCount={alertCount} onOpenSettings={() => setPage('settings')} />
      <main className="mx-auto max-w-7xl px-4 py-6">
        {page === 'settings' ? (
          <Settings onBack={() => setPage('dashboard')} onRunWizard={openWizard} />
        ) : (
          <Dashboard onStatusChange={handleStatusChange} />
        )}
      </main>
      <Wizard open={wizardOpen} onClose={closeWizard} />
    </>
  )
}

export default function App() {
  return (
    <AppStateProvider>
      <Shell />
    </AppStateProvider>
  )
}
