// POST /api/checkout: creates a payment intent. The secret key stays on the server.
import { createCheckout } from '../../../lib/checkout';
import { getClient } from '../../../lib/tpt';

export async function POST(request) {
  let body = null;
  try {
    body = await request.json();
  } catch {
    return Response.json({ error: 'body must be JSON' }, { status: 400 });
  }
  let client;
  try {
    client = getClient();
  } catch (err) {
    console.error(err.message);
    return Response.json({ error: 'payments are not configured' }, { status: 503 });
  }
  const { status, body: out } = await createCheckout(client, body);
  return Response.json(out, { status });
}
