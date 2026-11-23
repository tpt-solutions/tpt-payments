'use client'

import * as React from 'react'

export interface ToastData {
  id: string
  title?: string
  description?: string
  variant?: 'default' | 'destructive'
}

type Listener = (toasts: ToastData[]) => void

const TOAST_LIFETIME_MS = 5000
let toasts: ToastData[] = []
const listeners = new Set<Listener>()
let counter = 0

function emit() {
  Array.from(listeners).forEach((listener) => listener(toasts))
}

export function dismiss(id: string) {
  toasts = toasts.filter((t) => t.id !== id)
  emit()
}

/** Show a toast. It dismisses itself after a few seconds. Requires a <Toaster /> in the layout. */
export function toast(input: Omit<ToastData, 'id'>) {
  counter += 1
  const id = `toast-${counter}`
  toasts = [...toasts, { ...input, id }].slice(-5)
  emit()
  setTimeout(() => dismiss(id), TOAST_LIFETIME_MS)
  return id
}

export function useToast() {
  const [state, setState] = React.useState<ToastData[]>(toasts)
  React.useEffect(() => {
    listeners.add(setState)
    return () => {
      listeners.delete(setState)
    }
  }, [])
  return { toasts: state, toast, dismiss }
}
