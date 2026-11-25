// Webhook handling, separate from the route file. The signature is checked over the exact bytes
// received, before anything is parsed or trusted.
'use strict';

const { constructEvent, SignatureVerificationError } = require('@tpt/pay/webhooks');

/**
 * Verifies a delivery and hands the event to onEvent. Returns { status, body }:
 *   400 when the signature check fails (no reason is given to the sender),
 *   500 when onEvent throws, so TPT Pay sends the event again,
 *   200 when the event was handled.
 */
function handleWebhook({ raw, signature, secret, onEvent, log = console }) {
  let event;
  try {
    event = constructEvent(raw, signature || '', secret);
  } catch (err) {
    if (err instanceof SignatureVerificationError) {
      return { status: 400, body: { error: 'signature check failed' } };
    }
    throw err;
  }
  try {
    onEvent(event);
  } catch (err) {
    log.error('webhook handler failed for', event.id, err);
    return { status: 500, body: { error: 'handler failed' } };
  }
  return { status: 200, body: { received: true } };
}

module.exports = { handleWebhook };
