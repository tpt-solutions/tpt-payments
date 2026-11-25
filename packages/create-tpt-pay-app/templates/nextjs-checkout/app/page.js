'use client';

// A minimal checkout form. It creates a payment intent on the server and shows the result. Card
// details are not collected here: a real page would use TPT Pay's hosted or embedded payment form.
import { useState } from 'react';

export default function Checkout() {
  const [amount, setAmount] = useState('2000');
  const [currency, setCurrency] = useState('usd');
  const [result, setResult] = useState(null);
  const [busy, setBusy] = useState(false);

  async function submit(event) {
    event.preventDefault();
    setBusy(true);
    setResult(null);
    try {
      const res = await fetch('/api/checkout', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ amount: Number(amount), currency, order_id: `order-${Date.now()}` }),
      });
      const body = await res.json();
      setResult({ ok: res.ok, text: res.ok ? `Payment ${body.id} is ${body.status}.` : body.error });
    } catch {
      setResult({ ok: false, text: 'Could not reach the server. Try again.' });
    } finally {
      setBusy(false);
    }
  }

  return (
    <main>
      <h1>Checkout</h1>
      <form onSubmit={submit}>
        <label>
          Amount (minor units, for example 2000 for 20.00)
          <input
            type="number"
            min="1"
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            style={{ display: 'block', width: '100%', margin: '0.5rem 0 1rem', padding: '0.5rem' }}
          />
        </label>
        <label>
          Currency
          <input
            value={currency}
            onChange={(e) => setCurrency(e.target.value)}
            maxLength={3}
            style={{ display: 'block', width: '6rem', margin: '0.5rem 0 1rem', padding: '0.5rem' }}
          />
        </label>
        <button type="submit" disabled={busy} style={{ padding: '0.6rem 1.2rem' }}>
          {busy ? 'Starting payment…' : 'Pay'}
        </button>
      </form>
      {result && (
        <p role="status" style={{ color: result.ok ? 'seagreen' : 'crimson' }}>
          {result.text}
        </p>
      )}
    </main>
  );
}
