import type { LucideIcon } from 'lucide-react'

export interface KpiItem {
  icon: LucideIcon
  label: string
  sub: string
  value: string
  accent: string
}

export function KpiCards({ items }: { items: KpiItem[] }) {
  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
      {items.map((item) => {
        const Icon = item.icon
        return (
          <section
            key={item.label}
            className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm dark:border-slate-800 dark:bg-slate-900"
          >
            <div className="flex items-start justify-between gap-2">
              <div>
                <h3 className="text-sm font-medium text-slate-600 dark:text-slate-400">
                  {item.label}
                </h3>
                <p className="mt-1 text-2xl font-semibold text-slate-900 dark:text-slate-50">
                  {item.value}
                </p>
                <p className="mt-1 text-xs text-slate-500 dark:text-slate-400">{item.sub}</p>
              </div>
              <span
                className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-lg ${item.accent}`}
                aria-hidden="true"
              >
                <Icon size={20} />
              </span>
            </div>
          </section>
        )
      })}
    </div>
  )
}
