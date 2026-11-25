# TPT Pay merchant dashboard (frontend)

> **Status: prototype.** The dashboard and developer portal run, but most pages show mock data
> and are not connected to the API yet. Replacing the mock data is tracked in
> [todo §8](../todo.md). Do not use this app with real merchant accounts.

Next.js app. Setup for the whole platform is in the [root README](../README.md).

## Authentication

Dashboard users live in the core's database (`dashboard_users`), not in this app. The core owns the
password hashes (PBKDF2-SHA256), the TOTP secrets (encrypted), and the lockout. This server calls the
core's internal routes with a service token. The browser never calls them. Every auth route returns
`503` until the settings are present.

1. Copy `.env.example` to `.env.local` and set `JWT_SECRET` (at least 32 characters).
2. Start the core with the same `DASHBOARD_SERVICE_TOKEN` as this app, and a `DASHBOARD_TOTP_KEY`
   (`openssl rand -base64 32`). Set `TPT_CORE_URL` to the core's address.
3. Create a user, then enroll a second factor. The password is read from standard input:

   ```bash
   echo 'a long passphrase' | dashuser create --merchant-email you@example.com \
     --email owner@example.com --name "Owner"
   dashuser enroll-totp --email owner@example.com   # prints an otpauth:// URI once
   ```

Transactions are read from the payments API through server routes under `/api/tpt/`. Set
`TPT_API_BASE` (the API base URL) and `TPT_DASHBOARD_SECRET_KEY` (a secret key; `sk_test_` while
developing). The key stays on the server. The routes check the session themselves, because the
dashboard middleware matches pages, not `/api`.

Note: `next.config.js` rewrites every other `/api/*` path to `http://localhost:8080/v1`. Routes
under `/api/tpt/` are static files and take precedence over that rewrite, but a dynamic
`[id]` route under `/api` would not be reached, which is why the retrieve route takes `?id=`.

What the app enforces:

- Dashboard routes (`/dashboard`, `/api-keys`, `/transactions`, `/webhooks`, `/settings`) redirect
  to sign-in unless the session signature verifies (`middleware.ts`).
- Login and 2FA are rate-limited per client (10 attempts per 15 minutes). The limit is in-process,
  so a multi-instance deployment needs a shared store.
- A pre-auth token is single-use: each one allows one TOTP attempt.
- Passwords are PBKDF2-SHA256 (600,000 iterations) in the core, with a per-password random salt.
- Five wrong passwords, or five wrong codes, lock that factor for 15 minutes. The count is shared by every
  server instance, because it is stored in the core's database.
- A second-factor code can be used once. A replayed code is refused.

Known gaps before real use: the per-client rate limit is in-process, and it trusts the first
`X-Forwarded-For` hop, so run it behind a proxy that sets that header. Users are created with the
`dashuser` command only; there is no sign-up or password-reset flow.
