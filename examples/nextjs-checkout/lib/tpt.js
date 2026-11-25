// The TPT Pay client for the server, built once from the environment.
'use strict';

const { TPTPay } = require('@tpt/pay');

let client = null;

/** Returns the shared client. Throws if the secret key is not configured. */
function getClient() {
  if (!client) {
    const apiKey = process.env.TPT_SECRET_KEY;
    if (!apiKey) throw new Error('TPT_SECRET_KEY is not set (see .env.example)');
    client = new TPTPay({ apiKey, apiBaseUrl: process.env.TPT_API_BASE || undefined });
  }
  return client;
}

function webhookSecret() {
  const secret = process.env.TPT_WEBHOOK_SECRET;
  if (!secret) throw new Error('TPT_WEBHOOK_SECRET is not set (see .env.example)');
  return secret;
}

module.exports = { getClient, webhookSecret };
