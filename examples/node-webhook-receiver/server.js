// Webhook receiver for TPT Pay, using only Node's http module and @tpt/pay.
//
// The signature is checked over the raw request bytes before anything is parsed. Deliveries
// that fail verification get 400 and are not passed to the handler, so the retry policy on
// the server side treats them as permanent failures.

const http = require('node:http');
const { constructEvent, SignatureVerificationError } = require('@tpt/pay/webhooks');

const MAX_BODY_BYTES = 1024 * 1024; // matches the core service's 1 MB request limit

function readRawBody(req, limit) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    let size = 0;
    req.on('data', (chunk) => {
      size += chunk.length;
      if (size > limit) {
        reject(new Error('body too large'));
        req.destroy();
        return;
      }
      chunks.push(chunk);
    });
    req.on('end', () => resolve(Buffer.concat(chunks)));
    req.on('error', reject);
  });
}

/**
 * Build a request handler.
 * @param {object} options
 * @param {string} options.secret  the endpoint's signing secret (whsec_...)
 * @param {(event: object) => Promise<void> | void} options.onEvent  called once per verified event
 */
function createWebhookHandler({ secret, onEvent }) {
  if (!secret) throw new Error('secret is required');
  if (typeof onEvent !== 'function') throw new Error('onEvent must be a function');

  return async function handle(req, res) {
    if (req.method !== 'POST') {
      res.writeHead(405, { Allow: 'POST' }).end();
      return;
    }

    let raw;
    try {
      raw = await readRawBody(req, MAX_BODY_BYTES);
    } catch {
      res.writeHead(413).end();
      return;
    }

    let event;
    try {
      event = constructEvent(raw, req.headers['tpt-signature'] || '', secret);
    } catch (err) {
      if (err instanceof SignatureVerificationError) {
        // Do not say why. The reason helps an attacker more than it helps the sender.
        res.writeHead(400).end();
        return;
      }
      throw err;
    }

    try {
      await onEvent(event);
    } catch (err) {
      // Non-2xx asks the sender to retry. Log the detail here, not in the response body.
      console.error('webhook handler failed for', event.id, err);
      res.writeHead(500).end();
      return;
    }

    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ received: true }));
  };
}

module.exports = { createWebhookHandler };

// Run directly: WEBHOOK_SECRET=whsec_... PORT=4242 node server.js
if (require.main === module) {
  const secret = process.env.WEBHOOK_SECRET;
  if (!secret) {
    console.error('WEBHOOK_SECRET is required');
    process.exit(1);
  }
  const handler = createWebhookHandler({
    secret,
    onEvent: (event) => {
      console.log('received', event.type, event.id || '');
    },
  });
  const port = Number(process.env.PORT || 4242);
  http.createServer((req, res) => {
    if (req.url !== '/webhook') {
      res.writeHead(404).end();
      return;
    }
    handler(req, res);
  }).listen(port, () => console.log(`listening on http://localhost:${port}/webhook`));
}
