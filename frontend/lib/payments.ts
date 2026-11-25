// Types and display helpers for payment intents, shared by the transaction pages.

// The shape the core API returns for a payment intent (payment.Wire in the core service).
export interface PaymentIntent {
  id: string
  object: "payment_intent"
  amount: number // minor units
  currency: string
  status: IntentStatus
  created: number // Unix seconds
  livemode: boolean
  capture_method?: string
  confirmation_method?: string
  payment_method?: string
  amount_captured?: number
  ledger_transaction_id?: string
  canceled_at?: number
  cancellation_reason?: string
  metadata?: Record<string, string>
}

export type IntentStatus =
  | "requires_payment_method"
  | "requires_confirmation"
  | "requires_action"
  | "processing"
  | "requires_capture"
  | "succeeded"
  | "canceled"

export const STATUS_LABELS: Record<IntentStatus, string> = {
  requires_payment_method: "Awaiting payment method",
  requires_confirmation: "Awaiting confirmation",
  requires_action: "Action required",
  processing: "Processing",
  requires_capture: "Authorized",
  succeeded: "Succeeded",
  canceled: "Canceled",
}

// minorDigits is the number of decimal places in the currency's minor unit (2 for USD, 0 for JPY).
export function minorDigits(currency: string): number {
  try {
    return new Intl.NumberFormat("en", { style: "currency", currency: currency.toUpperCase() })
      .resolvedOptions().maximumFractionDigits ?? 2
  } catch {
    return 2
  }
}

// formatMinor shows an amount in minor units using the currency's own decimal places, so JPY
// (no minor unit) and USD (2 places) are both shown correctly.
export function formatMinor(amount: number, currency: string): string {
  const code = currency.toUpperCase()
  try {
    const fmt = new Intl.NumberFormat("en", { style: "currency", currency: code })
    return fmt.format(amount / 10 ** minorDigits(code))
  } catch {
    return `${amount} ${code}`
  }
}
