# TPT Pay merchant dashboard (frontend)

> **Status: prototype.** The dashboard and developer portal run, but most pages show mock data
> and are not connected to the API yet. Replacing the mock data is tracked in
> [todo §8](../todo.md). Do not use this app with real merchant accounts.

Next.js app. Setup for the whole platform is in the [root README](../README.md).

## Authentication (prototype)

Sign-in uses one prototype account whose credentials come from the environment. Nothing is
committed to source, and every auth route returns `503` until the settings are present.

1. Copy `.env.example` to `.env.local`.
2. Set `JWT_SECRET` to at least 32 random characters.
3. Set `DEMO_USER_EMAIL`, `DEMO_USER_TOTP_SECRET`, and a password hash:

   ```bash
   node -e "const c=require('crypto');const s=c.randomBytes(16);c.scrypt(process.argv[1],s,32,(e,k)=>console.log('scrypt\$'+s.toString('hex')+'\$'+k.toString('hex')))" 'your-password'
   ```

What the app enforces:

- Dashboard routes (`/dashboard`, `/api-keys`, `/transactions`, `/webhooks`, `/settings`) redirect
  to sign-in unless the session signature verifies (`middleware.ts`).
- Login and 2FA are rate-limited per client (10 attempts per 15 minutes). The limit is in-process,
  so a multi-instance deployment needs a shared store.
- A pre-auth token is single-use: each one allows one TOTP attempt.
- Passwords use scrypt with a per-password random salt, compared in constant time.

Known gaps before real use: no user database, no account lockout, and the rate limiter trusts
the first `X-Forwarded-For` hop, so run it behind a proxy that sets that header.
