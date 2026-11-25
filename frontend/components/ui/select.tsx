'use client'

import * as React from 'react'
import { cn } from '@/lib/utils'

interface SelectContextValue {
  value: string
  onValueChange: (value: string) => void
  open: boolean
  setOpen: (open: boolean) => void
  /** Labels of the items, keyed by value, so SelectValue can show the chosen item's text. */
  labels: Record<string, string>
  registerLabel: (value: string, label: string) => void
  /** Links the trigger (aria-controls) to the listbox it opens. */
  listboxId: string
}

const SelectContext = React.createContext<SelectContextValue | null>(null)

function useSelect() {
  const ctx = React.useContext(SelectContext)
  if (!ctx) throw new Error('Select components must be used inside <Select>')
  return ctx
}

export interface SelectProps {
  value?: string
  defaultValue?: string
  onValueChange?: (value: string) => void
  children?: React.ReactNode
}

export function Select({ value, defaultValue = '', onValueChange, children }: SelectProps) {
  const [internal, setInternal] = React.useState(defaultValue)
  const [open, setOpen] = React.useState(false)
  const [labels, setLabels] = React.useState<Record<string, string>>({})
  const current = value ?? internal
  const listboxId = React.useId()

  const handleChange = React.useCallback(
    (next: string) => {
      if (value === undefined) setInternal(next)
      onValueChange?.(next)
    },
    [value, onValueChange],
  )

  const registerLabel = React.useCallback((v: string, label: string) => {
    setLabels((prev) => (prev[v] === label ? prev : { ...prev, [v]: label }))
  }, [])

  return (
    <SelectContext.Provider
      value={{ value: current, onValueChange: handleChange, open, setOpen, labels, registerLabel, listboxId }}
    >
      <div className="relative">{children}</div>
    </SelectContext.Provider>
  )
}

export const SelectTrigger = React.forwardRef<HTMLButtonElement, React.ButtonHTMLAttributes<HTMLButtonElement>>(
  ({ className, children, ...props }, ref) => {
    const { open, setOpen, listboxId } = useSelect()
    return (
      <button
        ref={ref}
        type="button"
        role="combobox"
        aria-expanded={open}
        aria-controls={listboxId}
        aria-haspopup="listbox"
        onClick={() => setOpen(!open)}
        className={cn(
          'flex h-10 w-full items-center justify-between rounded-md border border-input bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50',
          className,
        )}
        {...props}
      >
        {children}
        <span aria-hidden="true" className="ml-2 opacity-50">▾</span>
      </button>
    )
  },
)
SelectTrigger.displayName = 'SelectTrigger'

export function SelectValue({ placeholder }: { placeholder?: string }) {
  const { value, labels } = useSelect()
  const text = value ? labels[value] ?? value : placeholder ?? ''
  return <span className={cn(!value && 'text-muted-foreground')}>{text}</span>
}

/**
 * The list is always mounted, hidden while closed, so each SelectItem can register its label
 * before the user opens the list.
 */
export function SelectContent({ className, children }: React.HTMLAttributes<HTMLDivElement>) {
  const { open, setOpen, listboxId } = useSelect()

  React.useEffect(() => {
    if (!open) return
    const onDown = () => setOpen(false)
    document.addEventListener('mousedown', onDown)
    return () => document.removeEventListener('mousedown', onDown)
  }, [open, setOpen])

  return (
    <div
      id={listboxId}
      role="listbox"
      hidden={!open}
      onMouseDown={(e) => e.stopPropagation()}
      className={cn(
        'absolute left-0 top-full z-50 mt-1 max-h-64 w-full min-w-[8rem] overflow-y-auto rounded-md border bg-popover p-1 text-popover-foreground shadow-md',
        className,
      )}
    >
      {children}
    </div>
  )
}

export interface SelectItemProps extends Omit<React.HTMLAttributes<HTMLDivElement>, 'value'> {
  value: string
}

export function SelectItem({ value, children, className, ...props }: SelectItemProps) {
  const { value: selected, onValueChange, setOpen, registerLabel } = useSelect()

  React.useEffect(() => {
    if (typeof children === 'string' || typeof children === 'number') {
      registerLabel(value, String(children))
    }
  }, [value, children, registerLabel])

  const active = selected === value
  return (
    <div
      role="option"
      aria-selected={active}
      tabIndex={-1}
      onClick={() => {
        onValueChange(value)
        setOpen(false)
      }}
      className={cn(
        'relative flex w-full cursor-pointer select-none items-center rounded-sm py-1.5 pl-3 pr-2 text-sm outline-none hover:bg-accent hover:text-accent-foreground',
        active && 'font-medium',
        className,
      )}
      {...props}
    >
      {children}
    </div>
  )
}
