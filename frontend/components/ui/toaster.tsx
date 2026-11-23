'use client'

import { cn } from '@/lib/utils'
import { dismiss, useToast } from './use-toast'

/** Renders the toasts created by toast(). Mount once, in the root layout. */
export function Toaster() {
  const { toasts } = useToast()
  return (
    <div
      aria-live="polite"
      className="pointer-events-none fixed bottom-4 right-4 z-[100] flex w-full max-w-sm flex-col gap-2"
    >
      {toasts.map((t) => (
        <div
          key={t.id}
          role="status"
          className={cn(
            'pointer-events-auto rounded-md border bg-background p-4 text-foreground shadow-lg',
            t.variant === 'destructive' && 'border-destructive text-destructive',
          )}
        >
          <div className="flex items-start justify-between gap-2">
            <div className="space-y-1">
              {t.title && <p className="text-sm font-semibold">{t.title}</p>}
              {t.description && <p className="text-sm text-muted-foreground">{t.description}</p>}
            </div>
            <button
              type="button"
              aria-label="Dismiss notification"
              onClick={() => dismiss(t.id)}
              className="text-sm opacity-70 hover:opacity-100"
            >
              <span aria-hidden="true">×</span>
            </button>
          </div>
        </div>
      ))}
    </div>
  )
}
