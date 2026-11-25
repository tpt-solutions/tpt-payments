# Next.js checkout

A checkout with Next.js (App Router) and TPT Pay:

- `app/page.js` is a form that asks the server for a payment intent. Card details are not collected
  here; a real page would use TPT Pay's hosted or embedded payment form.
- `app/api/checkout/route.js` creates the payment intent with the secret key, which stays on the server.
- `app/api/webhook/route.js` receives signed deliveries. The signature is checked over the raw body
  before anything is parsed, and a handler failure returns 500 so the event is sent again.

The logic lives in `lib/` and is tested without starting Next.js.

## Run it

```bash
npm install
cp .env.example .env.local   # then fill in TPT_SECRET_KEY and TPT_WEBHOOK_SECRET
npm run dev
```

Open http://localhost:3000. Create a webhook endpoint that points at `/api/webhook` (through a
tunnel if the app runs on your machine) to receive events.

The build uses webpack (`next build --webpack`). Turbopack cannot follow the `file:` link to
`@tpt/pay` on Windows, where npm writes that link with a POSIX path.

## Test

```bash
npm test
```

The tests cover input checks, the order reference, the generic upstream error, signed and tampered
webhooks, and handler failures. They use a fake client and make no network calls.
