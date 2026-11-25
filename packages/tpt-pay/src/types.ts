/**
 * Resource shapes as returned by the TPT Pay API. Amounts are integers in the currency's
 * smallest unit (cents for USD). Timestamps are Unix seconds.
 */

export type PaymentIntentStatus =
  | 'requires_payment_method'
  | 'requires_confirmation'
  | 'requires_action'
  | 'processing'
  | 'requires_capture'
  | 'succeeded'
  | 'canceled';

export interface PaymentIntent {
  id: string;
  object: 'payment_intent';
  amount: number;
  currency: string;
  /** IDs of the merchant flag rules that matched at creation. Always present. */
  risk_flags?: string[];
  status: PaymentIntentStatus;
  capture_method: 'automatic' | 'manual';
  confirmation_method: string;
  created: number;
  livemode: boolean;
  payment_method?: string;
  metadata?: Record<string, string>;
  amount_captured?: number;
  canceled_at?: number;
  cancellation_reason?: string;
  ledger_transaction_id?: string;
}

export interface CreatePaymentIntentParams {
  amount: number;
  currency: string;
  capture_method?: 'automatic' | 'manual';
  confirmation_method?: 'automatic' | 'manual';
  metadata?: Record<string, string>;
}

export interface ConfirmPaymentIntentParams {
  payment_method: string;
}

export interface CapturePaymentIntentParams {
  amount_to_capture?: number;
}

export interface CancelPaymentIntentParams {
  cancellation_reason?: string;
}

/** Options accepted by every write call. */
export interface RequestOptions {
  /** Idempotency key for this call. Generated automatically when omitted. */
  idempotencyKey?: string;
}

/**
 * Resources below are part of the planned Stripe-compatible surface. The core service does
 * not serve them yet (see the compatibility report), so calls will fail with 404.
 */
export interface Refund {
  id: string;
  object: 'refund';
  payment_intent: string;
  amount: number;
  currency: string;
  status: string;
  created: number;
}

/** A recurring charge. Test mode only until a live billing policy is decided. */
export interface Subscription {
  id: string;
  object: 'subscription';
  /** active: charges succeed. past_due: the last charge failed and is being retried. canceled: no more charges. */
  status: 'active' | 'past_due' | 'canceled';
  amount: number;
  currency: string;
  interval: 'week' | 'month' | 'year';
  payment_method: string;
  /** Unix time. The period the next charge pays for starts here. */
  current_period_start: number;
  current_period_end: number;
  cancel_at_period_end: boolean;
  cancel_reason?: 'requested' | 'payment_failed' | 'canceled_at_period_end';
  canceled_at?: number;
  metadata?: Record<string, string>;
  livemode: boolean;
  created: number;
}

export interface CreateSubscriptionParams {
  amount: number;
  currency: string;
  interval: 'week' | 'month' | 'year';
  payment_method: string;
  metadata?: Record<string, string>;
}

export interface ListSubscriptionsParams {
  limit?: number;
  starting_after?: string;
}

export interface SubscriptionList {
  object: 'list';
  url: string;
  has_more: boolean;
  data: Subscription[];
}

export interface Invoice {
  id: string;
  object: 'invoice';
  status: string;
  amount_due: number;
  currency: string;
  created: number;
}


/** A failure window in sandbox time. The end is exclusive. */
export interface TestClockOutage {
  from: string;
  to: string;
  /** If set, Authorize returns this card decline inside the window. */
  decline_code?: string;
}

/** The sandbox clock and its scheduled outages. */
export interface TestClock {
  object: 'test_clock';
  frozen_time: string;
  outages: TestClockOutage[];
}

/** Exactly one of seconds or frozen_time. */
export type AdvanceTestClockParams = { seconds: number; frozen_time?: never } | { frozen_time: string; seconds?: never };
