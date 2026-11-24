// Express checkout example: create, confirm and read payment intents, and receive verified
// webhooks. The application is built by createApp(), which takes its TPT Pay client and webhook
// secret as arguments, so the tests can run it against a fake client with no network.

'use strict';

const express = require('express');
const { TPTPay, TPTPayError } = require('@tpt/pay');
const { constructEvent, SignatureVerificationError } = require('@tpt/pay/webhooks');

const MAX_WEBHOOK_BYTES = 1024 * 1024;

function createApp({ tpt, webhookSecret }) {
  const app = express();

  // The webhook route needs the exact bytes that were signed, so it reads a raw body. It must
  // come before express.json(), which would otherwise consume the body first.
  app.post('/webhook', express.raw({ type: '*/*', limit: MAX_WEBHOOK_BYTES }), (req, res) => {
    let event;
    try {
      event = constructEvent(req.body, req.get('TPT-Signature') || '', webhookSecret);
    } catch (err) {
      if (err instanceof SignatureVerificationError) {
        // No reason in the body: the sender learns nothing about why the check failed.
        return res.status(400).json({ error: 'signature check failed' });
      }
      throw err;
    }
    try {
      // Handle the event here: mark the order paid on payment_intent.succeeded, and so on.
      console.log('event', event.type, event.id);
    } catch (err) {
      // A non-2xx response asks TPT Pay to send the event again.
      console.error('webhook handler failed for', event.id, err);
      return res.status(500).json({ error: 'handler failed' });
    }
    return res.json({ received: true });
  });

  app.use(express.json({ limit: '16kb' }));

  app.post('/api/checkout', async (req, res, next) => {
    const amount = req.body?.amount;
    if (!Number.isInteger(amount) || amount <= 0) {
      return res.status(400).json({ error: 'amount must be a positive integer in minor units' });
    }
    try {
      const intent = await tpt.paymentIntents.create({
        amount,
        currency: (req.body.currency || 'usd').toLowerCase(),
        capture_method: 'automatic',
        // Your order reference, so the webhook can be matched back to the order.
        metadata: { order_id: String(req.body.order_id ?? '') },
      });
      return res.json({ id: intent.id, status: intent.status });
    } catch (err) {
      return next(err);
    }
  });

  app.post('/api/checkout/:id/confirm', async (req, res, next) => {
    const paymentMethod = req.body?.payment_method;
    if (typeof paymentMethod !== 'string' || paymentMethod === '') {
      return res.status(400).json({ error: 'payment_method is required' });
    }
    try {
      const intent = await tpt.paymentIntents.confirm(req.params.id, { payment_method: paymentMethod });
      return res.json({ id: intent.id, status: intent.status });
    } catch (err) {
      return next(err);
    }
  });

  app.get('/api/checkout/:id', async (req, res, next) => {
    try {
      const intent = await tpt.paymentIntents.retrieve(req.params.id);
      return res.json({ id: intent.id, status: intent.status });
    } catch (err) {
      return next(err);
    }
  });

  // Errors from the API keep their status and code, so the browser can show a decline.
  app.use((err, req, res, _next) => {
    if (err instanceof TPTPayError && err.status && err.status < 500) {
      return res.status(err.status).json({ error: { code: err.code || null, message: err.message } });
    }
    console.error('payment request failed:', err);
    return res.status(502).json({ error: 'payment could not be completed' });
  });

  app.use((req, res) => res.status(404).json({ error: 'not found' }));

  return app;
}

module.exports = { createApp };

if (require.main === module) {
  const apiKey = process.env.TPT_SECRET_KEY;
  const webhookSecret = process.env.TPT_WEBHOOK_SECRET;
  if (!apiKey || !webhookSecret) {
    console.error('set TPT_SECRET_KEY and TPT_WEBHOOK_SECRET');
    process.exit(1);
  }
  const port = Number(process.env.PORT || 3000);
  createApp({ tpt: new TPTPay({ apiKey }), webhookSecret }).listen(port, () => {
    console.log(`listening on http://localhost:${port}`);
  });
}
