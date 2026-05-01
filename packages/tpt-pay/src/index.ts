/**
 * TPT Pay JavaScript SDK
 * @module @tpt/pay
 */

import * as CryptoJS from 'crypto-js';

export interface TPTPayConfig {
  publishableKey: string;
  apiBaseUrl?: string;
  version?: string;
}

export interface PaymentIntent {
  id: string;
  amount: number;
  currency: string;
  status: string;
  client_secret: string;
  metadata?: Record<string, string>;
  created_at: number;
}

export interface Refund {
  id: string;
  payment_intent_id: string;
  amount: number;
  currency: string;
  status: string;
  reason?: string;
  created_at: number;
}

export interface Subscription {
  id: string;
  customer_id: string;
  price_id: string;
  status: string;
  current_period_start: number;
  current_period_end: number;
  cancel_at_period_end: boolean;
  created_at: number;
}

export interface Invoice {
  id: string;
  number: string;
  status: string;
  total: number;
  currency: string;
  hosted_url: string;
  due_at: number;
  created_at: number;
}

export interface CreatePaymentIntentOptions {
  amount: number;
  currency: string;
  metadata?: Record<string, string>;
}

export interface ConfirmPaymentOptions {
  payment_method: string;
  return_url?: string;
}

/**
 * Main TPTPay client class
 */
export class TPTPay {
  private publishableKey: string;
  private apiBaseUrl: string;
  private version: string;

  constructor(config: TPTPayConfig) {
    this.publishableKey = config.publishableKey;
    this.apiBaseUrl = config.apiBaseUrl || 'https://api.tptpay.com/v1';
    this.version = config.version || '2024-06-20';
  }

  /**
   * Payment Intents API
   */
  public paymentIntents = {
    /**
     * Create a new PaymentIntent
     */
    create: async (options: CreatePaymentIntentOptions): Promise<PaymentIntent> => {
      return this.request<PaymentIntent>('/payment_intents', 'POST', options);
    },

    /**
     * Retrieve an existing PaymentIntent
     */
    retrieve: async (id: string): Promise<PaymentIntent> => {
      return this.request<PaymentIntent>(`/payment_intents/${id}`, 'GET');
    },

    /**
     * Confirm a PaymentIntent
     */
    confirm: async (id: string, options: ConfirmPaymentOptions): Promise<PaymentIntent> => {
      return this.request<PaymentIntent>(`/payment_intents/${id}/confirm`, 'POST', options);
    },

    /**
     * Cancel a PaymentIntent
     */
    cancel: async (id: string): Promise<PaymentIntent> => {
      return this.request<PaymentIntent>(`/payment_intents/${id}/cancel`, 'POST');
    }
  };

  /**
   * Refunds API
   */
  public refunds = {
    /**
     * Create a new Refund
     */
    create: async (paymentIntentId: string, amount: number, reason?: string): Promise<Refund> => {
      return this.request<Refund>('/refunds', 'POST', {
        payment_intent_id: paymentIntentId,
        amount,
        reason
      });
    },

    /**
     * Retrieve an existing Refund
     */
    retrieve: async (id: string): Promise<Refund> => {
      return this.request<Refund>(`/refunds/${id}`, 'GET');
    },

    /**
     * List all Refunds
     */
    list: async (params?: { limit?: number; starting_after?: string }): Promise<{ data: Refund[], has_more: boolean }> => {
      const query = params ? '?' + new URLSearchParams(params as any).toString() : '';
      return this.request<{ data: Refund[], has_more: boolean }>(`/refunds${query}`, 'GET');
    }
  };

  /**
   * Subscriptions API
   */
  public subscriptions = {
    /**
     * Create a new Subscription
     */
    create: async (customerId: string, priceId: string, options?: { trial_days?: number }): Promise<Subscription> => {
      return this.request<Subscription>('/subscriptions', 'POST', {
        customer_id: customerId,
        price_id: priceId,
        ...options
      });
    },

    /**
     * Cancel a Subscription
     */
    cancel: async (id: string, atPeriodEnd: boolean = false): Promise<Subscription> => {
      return this.request<Subscription>(`/subscriptions/${id}/cancel`, 'POST', {
        at_period_end: atPeriodEnd
      });
    },

    /**
     * Retrieve an existing Subscription
     */
    retrieve: async (id: string): Promise<Subscription> => {
      return this.request<Subscription>(`/subscriptions/${id}`, 'GET');
    }
  };

  /**
   * Invoices API
   */
  public invoices = {
    /**
     * Retrieve an existing Invoice
     */
    retrieve: async (id: string): Promise<Invoice> => {
      return this.request<Invoice>(`/invoices/${id}`, 'GET');
    },

    /**
     * Pay an Invoice
     */
    pay: async (id: string, paymentMethodId?: string): Promise<Invoice> => {
      return this.request<Invoice>(`/invoices/${id}/pay`, 'POST', {
        payment_method: paymentMethodId
      });
    }
  };

  /**
   * Webhook signature verification
   */
  public webhooks = {
    /**
     * Verify webhook signature
     */
    constructEvent: (payload: string, signatureHeader: string, secret: string, tolerance: number = 300): any => {
      const parts = signatureHeader.split(',');
      
      let timestamp: number = 0;
      let signature: string = '';

      for (const part of parts) {
        const [key, value] = part.split('=', 2);
        if (key === 't') timestamp = parseInt(value, 10);
        if (key === 'v1') signature = value;
      }

      if (timestamp === 0 || !signature) {
        throw new Error('Invalid signature header format');
      }

      const now = Math.floor(Date.now() / 1000);
      if (now - timestamp > tolerance) {
        throw new Error('Webhook timestamp outside tolerance window');
      }

      const signedPayload = `${timestamp}.${payload}`;
      const expectedSignature = CryptoJS.HmacSHA256(signedPayload, secret).toString(CryptoJS.enc.Hex);

      if (!this.secureCompare(signature, expectedSignature)) {
        throw new Error('Invalid webhook signature');
      }

      return JSON.parse(payload);
    }
  };

  private async request<T>(path: string, method: string, body?: any): Promise<T> {
    const headers: Record<string, string> = {
      'Authorization': `Bearer ${this.publishableKey}`,
      'TPT-Version': this.version,
      'Content-Type': 'application/json',
    };

    const options: RequestInit = {
      method,
      headers,
      credentials: 'same-origin'
    };

    if (body && method !== 'GET') {
      options.body = JSON.stringify(body);
    }

    const response = await fetch(`${this.apiBaseUrl}${path}`, options);

    if (!response.ok) {
      const error = await response.json().catch(() => ({ message: 'Request failed' }));
      throw new Error(error.message || `Request failed with status ${response.status}`);
    }

    return response.json() as Promise<T>;
  }

  private secureCompare(a: string, b: string): boolean {
    if (a.length !== b.length) return false;
    
    let result = 0;
    for (let i = 0; i < a.length; i++) {
      result |= a.charCodeAt(i) ^ b.charCodeAt(i);
    }
    
    return result === 0;
  }
}

export default TPTPay;