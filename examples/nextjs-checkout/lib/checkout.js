// Checkout logic, separate from the Next.js route files so it can be tested without starting Next.
'use strict';

const CURRENCY = /^[a-z]{3}$/i;
const MAX_AMOUNT = 10_000_000; // a sanity cap for the example, in minor units

/** Checks the browser's request. Returns { ok: true, value } or { ok: false, error }. */
function parseCheckout(body) {
  if (!body || typeof body !== 'object' || Array.isArray(body)) {
    return { ok: false, error: 'body must be a JSON object' };
  }
  const { amount, currency = 'usd', order_id: orderId = '' } = body;
  if (!Number.isInteger(amount) || amount <= 0 || amount > MAX_AMOUNT) {
    return { ok: false, error: `amount must be a whole number from 1 to ${MAX_AMOUNT} in minor units` };
  }
  if (typeof currency !== 'string' || !CURRENCY.test(currency)) {
    return { ok: false, error: 'currency must be a three-letter code, such as usd' };
  }
  if (typeof orderId !== 'string' || orderId.length > 64) {
    return { ok: false, error: 'order_id must be a string of at most 64 characters' };
  }
  return { ok: true, value: { amount, currency: currency.toLowerCase(), orderId } };
}

/**
 * Creates a payment intent through the client. Returns { status, body } so the route can send it.
 * Upstream detail is logged by the caller and never returned to the browser.
 */
async function createCheckout(tpt, body, log = console) {
  const parsed = parseCheckout(body);
  if (!parsed.ok) return { status: 400, body: { error: parsed.error } };
  try {
    const intent = await tpt.paymentIntents.create({
      amount: parsed.value.amount,
      currency: parsed.value.currency,
      capture_method: 'automatic',
      // Your order reference, so the webhook can be matched back to the order.
      metadata: { order_id: parsed.value.orderId },
    });
    return { status: 200, body: { id: intent.id, status: intent.status } };
  } catch (err) {
    log.error('create payment intent failed:', err && err.message);
    return { status: 502, body: { error: 'payment could not be started' } };
  }
}

module.exports = { parseCheckout, createCheckout, MAX_AMOUNT };
