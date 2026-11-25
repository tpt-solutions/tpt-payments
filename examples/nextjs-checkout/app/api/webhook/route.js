// POST /api/webhook: receives signed deliveries from TPT Pay. The raw body is read so the signature
// is checked over the exact bytes sent. (App Router route handlers do not parse the body for us.)
import { handleWebhook } from '../../../lib/webhook';
import { webhookSecret } from '../../../lib/tpt';

const MAX_BYTES = 1024 * 1024;

export async function POST(request) {
  const declared = Number(request.headers.get('content-length') || 0);
  if (declared > MAX_BYTES) return Response.json({ error: 'body too large' }, { status: 413 });
  const raw = Buffer.from(await request.arrayBuffer());
  if (raw.length > MAX_BYTES) return Response.json({ error: 'body too large' }, { status: 413 });

  let secret;
  try {
    secret = webhookSecret();
  } catch (err) {
    console.error(err.message);
    return Response.json({ error: 'webhooks are not configured' }, { status: 503 });
  }
  const { status, body } = handleWebhook({
    raw,
    signature: request.headers.get('TPT-Signature'),
    secret,
    onEvent: (event) => {
      // Handle the event here: mark the order paid on payment_intent.succeeded, and so on.
      console.log('event', event.type, event.id);
    },
  });
  return Response.json(body, { status });
}
