// Subscription billing example: start, read and cancel subscriptions, and react to subscription
// webhooks. The price comes from a plan table on the server; the browser chooses a plan, never an
// amount. Subscriptions are test mode only in TPT Pay for now, so use a test key.

'use strict';

const express = require('express');
const { TPTPay, TPTPayError } = require('@tpt/pay');
const { constructEvent, SignatureVerificationError } = require('@tpt/pay/webhooks');

const MAX_WEBHOOK_BYTES = 1024 * 1024;

// The plans this application sells. Amounts are in minor units (cents), set here and nowhere else.
const PLANS = {
  pro: { amount: 1500, currency: 'usd', interval: 'month' },
  team: { amount: 4900, currency: 'usd', interval: 'month' },
};

function createApp({ tpt, webhookSecret, onEvent = () => {} }) {
  const app = express();

  // The webhook route needs the exact bytes that were signed, so it reads a raw body. It must come
  // before express.json(), which would otherwise consume the body first.
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
      if (event.type.startsWith('subscription.')) {
        // A non-2xx response asks TPT Pay to send the event again, so only fail on a real problem.
        onEvent(event);
      }
    } catch (err) {
      console.error('subscription handler failed for', event.id, err);
      return res.status(500).json({ error: 'handler failed' });
    }
    return res.json({ received: true });
  });

  app.use(express.json({ limit: '16kb' }));

  app.post('/api/subscriptions', async (req, res, next) => {
    const plan = PLANS[req.body?.plan];
    if (!plan) {
      return res.status(400).json({ error: 'plan must be one of: ' + Object.keys(PLANS).join(', ') });
    }
    const paymentMethod = req.body?.payment_method;
    if (typeof paymentMethod !== 'string' || paymentMethod === '') {
      return res.status(400).json({ error: 'payment_method is required' });
    }
    try {
      const sub = await tpt.subscriptions.create({
        amount: plan.amount,
        currency: plan.currency,
        interval: plan.interval,
        payment_method: paymentMethod,
        // Your customer reference, so each webhook can be matched back to the account.
        metadata: { customer_ref: String(req.body.customer_ref ?? '') },
      });
      return res.json(summary(sub));
    } catch (err) {
      return next(err);
    }
  });

  app.get('/api/subscriptions/:id', async (req, res, next) => {
    try {
      return res.json(summary(await tpt.subscriptions.retrieve(req.params.id)));
    } catch (err) {
      return next(err);
    }
  });

  app.post('/api/subscriptions/:id/cancel', async (req, res, next) => {
    const atPeriodEnd = req.body?.at_period_end === true;
    try {
      return res.json(summary(await tpt.subscriptions.cancel(req.params.id, atPeriodEnd)));
    } catch (err) {
      return next(err);
    }
  });

  // Errors from the API keep their status and code, so the browser can show a decline or a 404.
  app.use((err, req, res, _next) => {
    if (err instanceof TPTPayError && err.status && err.status < 500) {
      return res.status(err.status).json({ error: { code: err.code || null, message: err.message } });
    }
    console.error('subscription request failed:', err);
    return res.status(502).json({ error: 'subscription request could not be completed' });
  });

  app.use((req, res) => res.status(404).json({ error: 'not found' }));

  return app;
}

// The fields the browser needs. The full object stays on the server.
function summary(sub) {
  return {
    id: sub.id,
    status: sub.status,
    current_period_end: sub.current_period_end,
    cancel_at_period_end: sub.cancel_at_period_end,
  };
}

module.exports = { createApp, PLANS };

if (require.main === module) {
  const apiKey = process.env.TPT_SECRET_KEY;
  const webhookSecret = process.env.TPT_WEBHOOK_SECRET;
  if (!apiKey || !webhookSecret) {
    console.error('set TPT_SECRET_KEY and TPT_WEBHOOK_SECRET');
    process.exit(1);
  }
  const port = Number(process.env.PORT || 3000);
  const onEvent = (event) => console.log('subscription event', event.type, event.data.object.id);
  createApp({ tpt: new TPTPay({ apiKey }), webhookSecret, onEvent }).listen(port, () => {
    console.log(`listening on http://localhost:${port}`);
  });
}
